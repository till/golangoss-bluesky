package content

import (
	"context"
	"strings"

	gh "github.com/google/go-github/v39/github"
)

// larrySearchFix wraps larry's GitHub search client to repair a query-encoding
// bug in upstream (which is unmaintained).
//
// larry joins qualifiers with literal '+' characters (e.g. "a+language:go").
// go-github URL-encodes those as %2B, so GitHub receives a single fuzzy text
// term and silently drops the language qualifier — returning repos in any language.
//
// Spaces, which go-github encodes as '+' on the wire, are the actual GitHub search
// separator.
type larrySearchFix struct {
	inner interface {
		Repositories(ctx context.Context, query string, opt *gh.SearchOptions) (*gh.RepositoriesSearchResult, *gh.Response, error)
	}
}

func (f larrySearchFix) Repositories(ctx context.Context, query string, opt *gh.SearchOptions) (*gh.RepositoriesSearchResult, *gh.Response, error) {
	return f.inner.Repositories(ctx, strings.ReplaceAll(query, "+", " "), opt)
}
