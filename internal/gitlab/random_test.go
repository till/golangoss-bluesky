package gitlab_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/till/golangoss-bluesky/internal/gitlab"
)

// fakeRand returns a sequence of ints, looping if exhausted.
func fakeRand(seq ...int) func(int) int {
	i := 0
	return func(n int) int {
		v := seq[i%len(seq)]
		i++
		return v % n
	}
}

func TestRandomProject_PassesRandomCharAndPicksOne(t *testing.T) {
	// Sequence: 0 -> first char ('a'), 0 -> first order_by ('star_count'),
	// 1 -> second project in the response.
	prev := gitlab.RandIntN
	gitlab.RandIntN = fakeRand(0, 0, 1)
	t.Cleanup(func() { gitlab.RandIntN = prev })

	var seenQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQueries = append(seenQueries, r.URL.Query().Get("search"))
		require.Equal(t, "Go", r.URL.Query().Get("with_programming_language"))
		require.Equal(t, "star_count", r.URL.Query().Get("order_by"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(samplePage))
	}))
	t.Cleanup(srv.Close)

	c, err := gitlab.NewWithBaseURL("", srv.URL+"/api/v4", srv.Client())
	require.NoError(t, err)

	got, err := c.RandomProject(t.Context(), "Go")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, []string{"a"}, seenQueries)
	require.Equal(t, "b/beta", got.PathWithNS)
}

func TestRandomProject_RetriesOnEmptyPage(t *testing.T) {
	// First two chars miss ('a','b'), third hits ('c').
	// Order indices: 0,0,0 (always star_count). Final pick: 0.
	prev := gitlab.RandIntN
	gitlab.RandIntN = fakeRand(0, 0, 1, 0, 2, 0, 0)
	t.Cleanup(func() { gitlab.RandIntN = prev })

	var seenQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		seenQueries = append(seenQueries, q)
		w.Header().Set("Content-Type", "application/json")
		if q == "c" {
			_, _ = w.Write([]byte(samplePage))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	c, err := gitlab.NewWithBaseURL("", srv.URL+"/api/v4", srv.Client())
	require.NoError(t, err)

	got, err := c.RandomProject(t.Context(), "Go")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, []string{"a", "b", "c"}, seenQueries)
	require.Equal(t, "a/alpha", got.PathWithNS)
}

func TestRandomProject_NilAfterMaxRetries(t *testing.T) {
	prev := gitlab.RandIntN
	gitlab.RandIntN = fakeRand(0, 0, 1, 0, 2, 0)
	t.Cleanup(func() { gitlab.RandIntN = prev })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	c, err := gitlab.NewWithBaseURL("", srv.URL+"/api/v4", srv.Client())
	require.NoError(t, err)

	got, err := c.RandomProject(t.Context(), "Go")
	require.NoError(t, err)
	require.Nil(t, got)
}
