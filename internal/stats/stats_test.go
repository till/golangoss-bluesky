package stats_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/till/golangoss-bluesky/internal/stats"
)

func TestHandleStats_RendersHTMLWithCacheSummary(t *testing.T) {
	fakeS3 := func(ctx context.Context) stats.S3Stats {
		return stats.S3Stats{
			Objects:   3,
			TotalSize: 2048,
			Recent: []stats.CacheEntry{
				{Key: "repo:12345", Size: 1024, LastModified: time.Now().Add(-2 * time.Minute)},
				{Key: "repo:67890", Size: 1024, LastModified: time.Now().Add(-5 * time.Minute)},
			},
		}
	}
	srv := stats.NewServer(":0", fakeS3)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.HandleStats(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	body := w.Body.String()
	require.Contains(t, body, "golangoss-bluesky stats")
	require.Contains(t, body, "Cached links")
	require.Contains(t, body, "repo:12345")
	require.Contains(t, body, "repo:67890")
}

func TestHandleStats_SurfacesS3Error(t *testing.T) {
	srv := stats.NewServer(":0", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.HandleStats(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "s3 not configured")
}

func TestHandleStats_NotFoundForUnknownPath(t *testing.T) {
	srv := stats.NewServer(":0", nil)
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	w := httptest.NewRecorder()
	srv.HandleStats(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
