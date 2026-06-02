package gitlab_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/till/golangoss-bluesky/internal/gitlab"
)

const samplePage = `[
  {"id":1,"name":"alpha","path_with_namespace":"a/alpha","web_url":"https://gitlab.com/a/alpha","description":"first","star_count":42,"topics":["go","cli"]},
  {"id":2,"name":"beta","path_with_namespace":"b/beta","web_url":"https://gitlab.com/b/beta","description":"second","star_count":3,"topics":[]}
]`

func TestSearch_PassesFiltersAndMapsFields(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v4/projects", r.URL.Path)
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(samplePage))
	}))
	t.Cleanup(srv.Close)

	c, err := gitlab.NewWithBaseURL("", srv.URL+"/api/v4", srv.Client())
	require.NoError(t, err)

	got, err := c.Search(t.Context(), gitlab.SearchOptions{
		Language: "Go",
		Query:    "bot",
		PerPage:  25,
		Page:     1,
	})
	require.NoError(t, err)

	require.Equal(t, "Go", gotQuery.Get("with_programming_language"))
	require.Equal(t, "bot", gotQuery.Get("search"))
	require.Equal(t, "public", gotQuery.Get("visibility"))
	require.Equal(t, "star_count", gotQuery.Get("order_by"))
	require.Equal(t, "desc", gotQuery.Get("sort"))
	require.Equal(t, "25", gotQuery.Get("per_page"))

	require.Len(t, got, 2)
	require.Equal(t, "a/alpha", got[0].PathWithNS)
	require.Equal(t, int64(42), got[0].Stars)
	require.Equal(t, []string{"go", "cli"}, got[0].Topics)
	require.Equal(t, "https://gitlab.com/b/beta", got[1].WebURL)
}

func TestSearch_MinStarsFiltersClientSide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(samplePage))
	}))
	t.Cleanup(srv.Close)

	c, err := gitlab.NewWithBaseURL("", srv.URL+"/api/v4", srv.Client())
	require.NoError(t, err)

	got, err := c.Search(t.Context(), gitlab.SearchOptions{MinStars: 10})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "a/alpha", got[0].PathWithNS)
}

func TestSearch_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c, err := gitlab.NewWithBaseURL("", srv.URL+"/api/v4", srv.Client())
	require.NoError(t, err)

	_, err = c.Search(t.Context(), gitlab.SearchOptions{Language: "Go"})
	require.Error(t, err)
}
