// Package gitlab is a thin wrapper around the official GitLab Go SDK
// that searches gitlab.com for public projects in a given language.
package gitlab

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go"
)

// Project is the subset of fields we care about for a search result.
type Project struct {
	ID          int64
	Name        string
	PathWithNS  string
	WebURL      string
	Description string
	Stars       int64
	Topics      []string
}

// SearchOptions controls a single page of results.
type SearchOptions struct {
	// Language is matched against GitLab's per-project language detection
	// (case-insensitive). Empty disables the filter.
	Language string
	// Query is a free-text search; empty means no text filter.
	Query string
	// MinStars filters out projects with fewer than this many stars.
	// Applied client-side after fetching the page.
	MinStars int64
	// PerPage caps the page size; <=0 means SDK default.
	PerPage int
	// Page is 1-indexed.
	Page int
}

// Client wraps a gitlab.Client. Token may be empty for unauthenticated
// access to public projects (rate-limited).
type Client struct {
	gl *gl.Client
}

// New builds a client against gitlab.com.
// httpClient is optional; pass nil for the default.
func New(token string, httpClient *http.Client) (*Client, error) {
	opts := []gl.ClientOptionFunc{}
	if httpClient != nil {
		opts = append(opts, gl.WithHTTPClient(httpClient))
	}
	c, err := gl.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("new gitlab client: %w", err)
	}
	return &Client{gl: c}, nil
}

// NewWithBaseURL is like New but targets a self-hosted GitLab.
// Useful for tests against an httptest server.
func NewWithBaseURL(token, baseURL string, httpClient *http.Client) (*Client, error) {
	opts := []gl.ClientOptionFunc{gl.WithBaseURL(baseURL)}
	if httpClient != nil {
		opts = append(opts, gl.WithHTTPClient(httpClient))
	}
	c, err := gl.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("new gitlab client: %w", err)
	}
	return &Client{gl: c}, nil
}

// Search returns one page of public projects matching opts,
// ordered by star_count descending.
func (c *Client) Search(ctx context.Context, opts SearchOptions) ([]Project, error) {
	return c.searchWithOrder(ctx, opts, "star_count")
}

func (c *Client) searchWithOrder(ctx context.Context, opts SearchOptions, orderBy string) ([]Project, error) {
	list := &gl.ListProjectsOptions{
		Visibility: new(gl.PublicVisibility),
		OrderBy:    new(orderBy),
		Sort:       new("desc"),
	}
	if opts.Language != "" {
		list.WithProgrammingLanguage = new(opts.Language)
	}
	if opts.Query != "" {
		list.Search = new(opts.Query)
	}
	if opts.PerPage > 0 {
		list.PerPage = int64(opts.PerPage)
	}
	if opts.Page > 0 {
		list.Page = int64(opts.Page)
	}

	raw, _, err := c.gl.Projects.ListProjects(list, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	out := make([]Project, 0, len(raw))
	for _, p := range raw {
		if p.StarCount < opts.MinStars {
			continue
		}
		out = append(out, Project{
			ID:          p.ID,
			Name:        p.Name,
			PathWithNS:  p.PathWithNamespace,
			WebURL:      p.WebURL,
			Description: p.Description,
			Stars:       p.StarCount,
			Topics:      p.Topics,
		})
	}
	return out, nil
}
