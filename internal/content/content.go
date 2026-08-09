package content

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/till/golangoss-bluesky/internal/bluesky"
	"github.com/till/golangoss-bluesky/internal/cache"
	"github.com/till/golangoss-bluesky/internal/config"
	ghprovider "github.com/till/golangoss-bluesky/internal/provider"
	"github.com/till/golangoss-bluesky/internal/utils"
)

var (
	prov *ghprovider.Provider

	// ErrCouldNotContent is returned when content cannot be fetched
	ErrCouldNotContent = errors.New("could not get content")
)

// activeWithin is the max age of a repo's last push for it to be considered
// alive. Anything older is filtered out at the search layer.
const activeWithin = 365 * 24 * time.Hour

// Start bootstraps the content provider
func Start(token string, cacheClient cache.ClientS3) error {
	cfg := config.Config{
		Language:    "go",
		Archived:    false,
		PushedSince: time.Now().UTC().Add(-activeWithin),
	}

	p, err := ghprovider.NewProvider(token, cfg, cacheClient)
	if err != nil {
		return err
	}
	prov = &p
	return nil
}

// Do gets content from the provider and posts it to bluesky
func Do(ctx context.Context, c bluesky.Client) error {
	item, err := prov.GetContentToPublish(ctx)
	if err != nil {
		utils.LogError(fmt.Errorf("error fetching content: %w", err))
		return ErrCouldNotContent
	}

	if item == nil {
		slog.Debug("nothing found")
		return nil
	}

	author := ""
	if item.Author.GitHubLogin != "" {
		author = "@" + item.Author.GitHubLogin
	}
	stargazers := fmt.Sprintf("⭐️ %d", item.Stars)

	return c.Post(ctx, bluesky.PostRecord(
		item.Title,
		item.Description,
		item.URL,
		author,
		stargazers,
		item.Hashtag,
	))
}
