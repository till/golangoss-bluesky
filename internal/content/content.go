package content

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ezeoleaf/larry/config"
	"github.com/ezeoleaf/larry/provider/github"
	"github.com/till/golangoss-bluesky/internal/bluesky"
)

var (
	provider github.Provider
)

func Start() error {
	if _, status := os.LookupEnv("GITHUB_TOKEN"); !status {
		panic("No github token")
	}

	cfg := config.Config{
		Language: "go",
	}

	cacheClient := &CacheClientProcess{}

	provider = github.NewProvider(os.Getenv("GITHUB_TOKEN"), cfg, cacheClient)
	return nil
}

func Do(ctx context.Context, c bluesky.Client) error {
	p, err := provider.GetContentToPublish()
	if err != nil {
		return fmt.Errorf("could not get content: %v", err)
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
				hashTags = e
				continue
			}

			if strings.Contains(e, "⭐️ ") {
				stargazers = e
				continue
			}

		}
	}

	return c.Post(ctx, bluesky.PostRecord(*p.Title, *p.URL, author, stargazers, hashTags))
}
