package content

import (
	"context"
	"fmt"
	"strings"
	"time"

	gh "github.com/google/go-github/v39/github"
)

// larrySearchFix wraps larry's GitHub search client to repair a query-encoding
// bug in upstream (which is unmaintained) and to narrow the search.
//
// larry joins qualifiers with literal '+' characters (e.g. "a+language:go").
// go-github URL-encodes those as %2B, so GitHub receives a single fuzzy text
// term and silently drops the language qualifier — returning repos in any language.
//
// Spaces, which go-github encodes as '+' on the wire, are the actual GitHub search
// separator.
//
// We also append `archived:false` and `pushed:>=<one year ago>` so the search
// skips dead projects. Larry's client-side archived check spins forever when all
// candidates are archived, and it never checks last-push time.
type larrySearchFix struct {
	inner interface {
		Repositories(ctx context.Context, query string, opt *gh.SearchOptions) (*gh.RepositoriesSearchResult, *gh.Response, error)
	}
	now func() time.Time
}

// roughly one year
const activeWithin = 365 * 24 * time.Hour

func (f larrySearchFix) Repositories(ctx context.Context, query string, opt *gh.SearchOptions) (*gh.RepositoriesSearchResult, *gh.Response, error) {
	now := time.Now
	if f.now != nil {
		now = f.now
	}
	pushedSince := now().UTC().Add(-activeWithin).Format("2006-01-02")

	q := fmt.Sprintf("%s archived:false pushed:>=%s",
		strings.ReplaceAll(query, "+", " "),
		pushedSince,
	)
	return f.inner.Repositories(ctx, q, opt)
}
