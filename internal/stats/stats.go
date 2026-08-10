// Package stats serves bot health metrics over HTTP: uptime, memory,
// GC counters and a summary of the S3 cache bucket.
package stats

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/minio/minio-go/v7"
)

const (
	// s3CacheTTL is how long S3 usage is cached before we re-list the bucket.
	// Keeps the page fast under refresh spam without hiding real changes for long.
	s3CacheTTL = 30 * time.Second

	shutdownTimeout = 5 * time.Second

	// recentLimit caps how many recent cache entries we display.
	recentLimit = 50
)

// S3Stats summarizes the state of the cache bucket.
type S3Stats struct {
	Objects   int
	TotalSize int64
	Recent    []CacheEntry
	Err       error
}

// CacheEntry describes one cached object.
type CacheEntry struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// S3Provider fetches cache-bucket stats. Kept as a function type so tests
// can inject a fake without spinning up a minio client.
type S3Provider func(ctx context.Context) S3Stats

// MinioProvider returns an S3Provider backed by a minio client.
func MinioProvider(mc *minio.Client, bucket string) S3Provider {
	return func(ctx context.Context) S3Stats {
		var out S3Stats
		opts := minio.ListObjectsOptions{Recursive: true}
		for obj := range mc.ListObjects(ctx, bucket, opts) {
			if obj.Err != nil {
				out.Err = obj.Err
				return out
			}
			out.Objects++
			out.TotalSize += obj.Size
			out.Recent = append(out.Recent, CacheEntry{
				Key:          obj.Key,
				Size:         obj.Size,
				LastModified: obj.LastModified,
			})
		}
		sort.Slice(out.Recent, func(i, j int) bool {
			return out.Recent[i].LastModified.After(out.Recent[j].LastModified)
		})
		if len(out.Recent) > recentLimit {
			out.Recent = out.Recent[:recentLimit]
		}
		return out
	}
}

// Server exposes bot health metrics over HTTP.
type Server struct {
	addr      string
	startTime time.Time
	s3        S3Provider

	mu       sync.Mutex
	cachedAt time.Time
	cached   S3Stats
}

// NewServer builds a stats server. addr follows net.Listen conventions
// (e.g. ":8080"). Pass nil for s3 to disable the cache section.
func NewServer(addr string, s3 S3Provider) *Server {
	return &Server{
		addr:      addr,
		startTime: time.Now(),
		s3:        s3,
	}
}

// ListenAndServe blocks until ctx is cancelled or the server errors.
// A ctx cancel triggers a graceful shutdown and returns nil.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.HandleStats)

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(sctx)
		close(shutdownDone)
	}()

	err := srv.ListenAndServe()
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// HandleStats renders the stats page.
func (s *Server) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var cache S3Stats
	if s.s3 != nil {
		cache = s.fetchS3(r.Context())
	} else {
		cache.Err = errors.New("s3 not configured")
	}

	data := viewData{
		Uptime:      time.Since(s.startTime).Round(time.Second).String(),
		Started:     s.startTime.UTC().Format(time.RFC3339),
		Goroutines:  runtime.NumGoroutine(),
		Alloc:       humanize.IBytes(m.Alloc),
		Sys:         humanize.IBytes(m.Sys),
		NumGC:       m.NumGC,
		PauseTotal:  time.Duration(m.PauseTotalNs).Round(time.Millisecond).String(),
		LastGC:      lastGC(m.LastGC),
		CacheCount:  cache.Objects,
		CacheSize:   humanize.IBytes(uint64(cache.TotalSize)),
		Recent:      toViewEntries(cache.Recent),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if cache.Err != nil {
		data.CacheError = cache.Err.Error()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := statsTmpl.Execute(w, data); err != nil {
		slog.ErrorContext(r.Context(), "render stats", "error", err)
	}
}

func (s *Server) fetchS3(ctx context.Context) S3Stats {
	s.mu.Lock()
	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < s3CacheTTL {
		c := s.cached
		s.mu.Unlock()
		return c
	}
	s.mu.Unlock()

	out := s.s3(ctx)

	s.mu.Lock()
	s.cachedAt = time.Now()
	s.cached = out
	s.mu.Unlock()

	return out
}

func lastGC(nanos uint64) string {
	if nanos == 0 {
		return "never"
	}
	return time.Unix(0, int64(nanos)).UTC().Format(time.RFC3339)
}

type viewData struct {
	Uptime      string
	Started     string
	Goroutines  int
	Alloc       string
	Sys         string
	NumGC       uint32
	LastGC      string
	PauseTotal  string
	CacheCount  int
	CacheSize   string
	CacheError  string
	Recent      []recentEntry
	GeneratedAt string
}

type recentEntry struct {
	Key  string
	Size string
	Age  string
}

func toViewEntries(entries []CacheEntry) []recentEntry {
	out := make([]recentEntry, 0, len(entries))
	now := time.Now()
	for _, e := range entries {
		out = append(out, recentEntry{
			Key:  e.Key,
			Size: humanize.IBytes(uint64(e.Size)),
			Age:  humanize.RelTime(e.LastModified, now, "ago", "from now"),
		})
	}
	return out
}

var statsTmpl = template.Must(template.New("stats").Parse(pageHTML))

const pageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>golangoss-bluesky stats</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 720px; margin: 2rem auto; padding: 0 1rem; color: #222; }
    h1 { font-size: 1.4rem; margin-bottom: .25rem; }
    h2 { font-size: 1.05rem; margin-top: 1.5rem; }
    dl { display: grid; grid-template-columns: max-content 1fr; column-gap: 1rem; row-gap: .25rem; }
    dt { font-weight: 600; }
    table { border-collapse: collapse; width: 100%; margin-top: .5rem; font-size: .9rem; }
    th, td { text-align: left; padding: .25rem .5rem; border-bottom: 1px solid #eee; }
    .num { text-align: right; font-variant-numeric: tabular-nums; }
    .error { color: #b00020; }
    footer { margin-top: 2rem; color: #666; font-size: .85rem; }
  </style>
</head>
<body>
  <h1>golangoss-bluesky stats</h1>

  <h2>Process</h2>
  <dl>
    <dt>Uptime</dt><dd>{{.Uptime}}</dd>
    <dt>Started</dt><dd>{{.Started}}</dd>
    <dt>Goroutines</dt><dd>{{.Goroutines}}</dd>
  </dl>

  <h2>Memory</h2>
  <dl>
    <dt>Allocated</dt><dd>{{.Alloc}}</dd>
    <dt>System</dt><dd>{{.Sys}}</dd>
  </dl>

  <h2>GC</h2>
  <dl>
    <dt>Cycles</dt><dd>{{.NumGC}}</dd>
    <dt>Last GC</dt><dd>{{.LastGC}}</dd>
    <dt>Pause total</dt><dd>{{.PauseTotal}}</dd>
  </dl>

  <h2>Cache (S3)</h2>
  {{- if .CacheError}}
  <p class="error">Error: {{.CacheError}}</p>
  {{- end}}
  <dl>
    <dt>Cached links</dt><dd>{{.CacheCount}}</dd>
    <dt>Total size</dt><dd>{{.CacheSize}}</dd>
  </dl>

  {{- if .Recent}}
  <table>
    <thead><tr><th>Key</th><th class="num">Size</th><th>Modified</th></tr></thead>
    <tbody>
    {{- range .Recent}}
      <tr><td>{{.Key}}</td><td class="num">{{.Size}}</td><td>{{.Age}}</td></tr>
    {{- end}}
    </tbody>
  </table>
  {{- end}}

  <footer>Generated {{.GeneratedAt}}</footer>
</body>
</html>
`
