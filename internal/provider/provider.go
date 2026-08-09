// Package provider picks a GitHub repo to post about. It runs a filtered
// search (language, non-archived, recently pushed), skips repos we've already
// seen via the shared cache, and enriches the pick with the owner's GitHub
// login plus Bluesky handle when they've linked one on their GitHub profile.
package provider

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/google/go-github/v90/github"
	"github.com/till/golangoss-bluesky/internal/cache"
	"github.com/till/golangoss-bluesky/internal/config"
)

// Content is what the provider hands back to the poster.
type Content struct {
	Title       string
	Description string
	URL         string
	Stars       int
	Hashtag     string
	Author      Author
}

// Author is the repo owner. BlueskyHandle is set when the owner listed a
// bsky.app profile in their GitHub social accounts; otherwise empty.
type Author struct {
	GitHubLogin   string
	BlueskyHandle string
}

type Provider struct {
	Config      config.Config
	CacheClient cache.ClientS3

	GitHubSearchClient *github.SearchService
	GitHubUserClient   *github.UsersService
}

func NewProvider(apiKey string, cfg config.Config, cacheClient cache.ClientS3) (Provider, error) {
	slog.Info("New Github Provider")
	p := Provider{Config: cfg, CacheClient: cacheClient}

	gh, err := github.NewClient(github.WithAuthToken(apiKey))
	if err != nil {
		return p, err
	}

	p.GitHubSearchClient = gh.Search
	p.GitHubUserClient = gh.Users

	return p, nil
}

// GetContentToPublish picks a random uncached repo matching the query and
// enriches it with the owner's social handles. Returns (nil, nil) when every
// candidate on the fetched page is already cached.
func (p Provider) GetContentToPublish(ctx context.Context) (*Content, error) {
	res, _, err := p.GitHubSearchClient.Repositories(ctx, p.buildQuery(), &github.SearchOptions{
		Sort:        "updated",
		Order:       "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("github search: %w", err)
	}
	if len(res.Repositories) == 0 {
		return nil, nil
	}

	for _, idx := range rand.Perm(len(res.Repositories)) {
		repo := res.Repositories[idx]
		if repo.ID == nil {
			continue
		}
		key := fmt.Sprintf("repo:%d", *repo.ID)

		seen, err := p.isSeen(ctx, key)
		if err != nil {
			return nil, err
		}
		if seen {
			continue
		}
		if err := p.markSeen(ctx, key); err != nil {
			return nil, err
		}
		return p.toContent(ctx, repo), nil
	}
	return nil, nil
}

func (p Provider) buildQuery() string {
	query := strings.Builder{}

	if p.Config.Language != "" {
		query.WriteString("language:")
		query.WriteString(p.Config.Language)
	}

	if !p.Config.Archived {
		if query.Len() > 0 {
			query.WriteString(" ")
		}
		query.WriteString("archived:false")
	}

	if query.Len() > 0 {
		query.WriteString(" ")
	}
	query.WriteString("pushed:>=")
	query.WriteString(p.Config.PushedSince.Format("2006-01-02"))

	return query.String()
}

func (p Provider) isSeen(ctx context.Context, key string) (bool, error) {
	_, err := p.CacheClient.Get(ctx, key)
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (p Provider) markSeen(ctx context.Context, key string) error {
	return p.CacheClient.Set(ctx, key, true, 0)
}

func (p Provider) toContent(ctx context.Context, repo *github.Repository) *Content {
	lang := p.Config.Language
	if lang == "" {
		lang = "go"
	}
	c := &Content{
		Title:       repo.GetName(),
		Description: repo.GetDescription(),
		URL:         repo.GetHTMLURL(),
		Stars:       repo.GetStargazersCount(),
		Hashtag:     "#" + strings.ToLower(lang),
	}
	if login := repo.GetOwner().GetLogin(); login != "" {
		c.Author = p.fetchAuthor(ctx, login)
	}
	return c
}

func (p Provider) fetchAuthor(ctx context.Context, login string) Author {
	a := Author{GitHubLogin: login}

	accounts, _, err := p.GitHubUserClient.ListUserSocialAccounts(ctx, login, nil)
	if err != nil {
		slog.WarnContext(ctx, "social accounts fetch failed", "login", login, "err", err)
		return a
	}
	for _, sa := range accounts {
		if h := extractBlueskyHandle(sa.GetURL()); h != "" {
			a.BlueskyHandle = h
			return a
		}
	}
	return a
}

// extractBlueskyHandle returns the handle portion of a bsky.app profile URL,
// or "" if the URL isn't a bsky profile.
func extractBlueskyHandle(url string) string {
	const prefix = "https://bsky.app/profile/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	handle := strings.TrimSuffix(strings.TrimPrefix(url, prefix), "/")
	if handle == "" || strings.ContainsAny(handle, "/?#") {
		return ""
	}
	return handle
}
