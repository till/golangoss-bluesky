package gitlab

import (
	"context"
	"fmt"
	"math/rand/v2"
)

// randAlphabet is the pool of single-char queries we send as `search`.
// GitLab matches `search` against project name and path, so any char here
// will surface a different slice of public projects.
const RandAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

var randOrderBy = []string{"star_count", "last_activity_at", "created_at"}

// randIntN is package-level so tests can swap in a deterministic source.
var RandIntN = rand.IntN

// RandomProject returns one randomly-picked public project in the given
// language. It does this by sending a random single-char `search` query
// and a random `order_by`, then picking one project from the response.
//
// Retries up to maxRetries-1 additional times with different chars if the
// page comes back empty. Returns (nil, nil) if no project is found after
// all retries.
func (c *Client) RandomProject(ctx context.Context, language string) (*Project, error) {
	const maxRetries = 3
	const perPage = 100

	tried := make(map[byte]bool, maxRetries)
	for range maxRetries {
		var ch byte
		for {
			ch = RandAlphabet[RandIntN(len(RandAlphabet))]
			if !tried[ch] {
				tried[ch] = true
				break
			}
		}
		orderBy := randOrderBy[RandIntN(len(randOrderBy))]

		results, err := c.searchWithOrder(ctx, SearchOptions{
			Language: language,
			Query:    string(ch),
			PerPage:  perPage,
		}, orderBy)
		if err != nil {
			return nil, fmt.Errorf("random project: %w", err)
		}
		if len(results) == 0 {
			continue
		}
		pick := results[RandIntN(len(results))]
		return &pick, nil
	}
	return nil, nil
}
