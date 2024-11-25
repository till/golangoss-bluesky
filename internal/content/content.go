package content

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/ezeoleaf/larry/cache"
	"github.com/ezeoleaf/larry/config"
	"github.com/ezeoleaf/larry/provider/github"
	"github.com/till/golangoss-bluesky/internal/bluesky"
)

var (
	provider           github.Provider
	ErrCouldNotContent = errors.New("could not get content")
)

func Start(token string, cacheClient cache.Client) error {
	cfg := config.Config{
		Language: "go",
	}

	provider = github.NewProvider(token, cfg, cacheClient)
	return nil
}

func Do(ctx context.Context, c bluesky.Client) error {
	p, err := provider.GetContentToPublish()
	if err != nil {
		slog.Error("error fetching content", slog.Any("err", err))
		return ErrCouldNotContent
	}

	if p == nil {
		slog.Debug("nothing found")
		return nil
	}

	var author, stargazers, hashTags string

	if len(p.ExtraData) > 0 {
		for _, e := range p.ExtraData {
			if strings.Contains(e, "Author: @") {
				author = strings.Replace(e, "Author: ", "", 1)
			}

			if strings.Contains(e, "#") {
				hashTags = strings.TrimSpace(e)
				continue
			}

			if strings.Contains(e, "⭐️ ") {
				stargazers = e
				continue
			}
		}
	}

	return c.Post(ctx, bluesky.PostRecord(*p.Title, *p.Subtitle, *p.URL, author, stargazers, hashTags))
}
