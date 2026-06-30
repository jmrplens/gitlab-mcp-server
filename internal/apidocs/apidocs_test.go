package apidocs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain makes backoff/spacing instant so retry tests run fast.
func TestMain(m *testing.M) {
	sleepFn = func(time.Duration) {}
	os.Exit(m.Run())
}

// newServer returns an httptest server serving "<area>.md" bodies and counting
// hits, plus the parsed base URL ending in "/".
func newServer(t *testing.T, body string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestFetch_DownloadsThenServesFreshCache(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newServer(t, "# branches doc")
	f := New(dir, Options{BaseURL: srv.URL + "/"})
	f.cacheDir = dir // isolate cache to temp dir

	got, err := f.Fetch("branches")
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	if got != "# branches doc" {
		t.Fatalf("body = %q", got)
	}
	if *hits != 1 {
		t.Fatalf("hits after first fetch = %d, want 1", *hits)
	}

	// Second fetch: cache is fresh, no new download.
	if _, err2 := f.Fetch("branches"); err2 != nil {
		t.Fatalf("second Fetch: %v", err2)
	}
	if *hits != 1 {
		t.Fatalf("hits after cached fetch = %d, want 1 (served from cache)", *hits)
	}
}

func TestFetch_StaleCacheTriggersRedownload(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newServer(t, "fresh")
	f := New(dir, Options{BaseURL: srv.URL + "/", MaxAge: time.Hour})
	f.cacheDir = dir

	// Seed a stale cache file (2h old, MaxAge 1h).
	cachePath := filepath.Join(dir, "branches.md")
	if err := os.WriteFile(cachePath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatal(err)
	}

	got, err := f.Fetch("branches")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got != "fresh" || *hits != 1 {
		t.Fatalf("stale cache not refreshed: body=%q hits=%d", got, *hits)
	}
}

func TestFetch_RefreshForcesDownload(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newServer(t, "v2")
	cachePath := filepath.Join(dir, "branches.md")
	if err := os.WriteFile(cachePath, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := New(dir, Options{BaseURL: srv.URL + "/", Refresh: true})
	f.cacheDir = dir

	got, err := f.Fetch("branches")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got != "v2" || *hits != 1 {
		t.Fatalf("refresh did not force download: body=%q hits=%d", got, *hits)
	}
}

func TestFetch_OfflineServesCachedAnyAgeAndErrorsOnMiss(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newServer(t, "should-not-be-fetched")
	f := New(dir, Options{BaseURL: srv.URL + "/", Offline: true})
	f.cacheDir = dir

	// Stale cache is still served in offline mode.
	cachePath := filepath.Join(dir, "branches.md")
	if err := os.WriteFile(cachePath, []byte("cached"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	_ = os.Chtimes(cachePath, old, old)

	got, err := f.Fetch("branches")
	if err != nil || got != "cached" {
		t.Fatalf("offline cached: body=%q err=%v", got, err)
	}
	// Cache miss offline is an error, no network hit.
	if _, missErr := f.Fetch("missing"); missErr == nil {
		t.Fatal("offline miss: want error")
	}
	if *hits != 0 {
		t.Fatalf("offline made %d network hits, want 0", *hits)
	}
}

func TestFetch_RetriesOn429ThenSucceeds(t *testing.T) {
	dir := t.TempDir()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0") // 0 -> ignored, fast backoff
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	f := New(dir, Options{BaseURL: srv.URL + "/"})
	f.cacheDir = dir

	got, err := f.Fetch("branches")
	if err != nil {
		t.Fatalf("Fetch with one 429: %v", err)
	}
	if got != "ok" || hits < 2 {
		t.Fatalf("did not retry past 429: body=%q hits=%d", got, hits)
	}
}

func TestFetch_NetworkErrorFallsBackToStaleCache(t *testing.T) {
	dir := t.TempDir()
	// Closed server -> connection refused.
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL + "/"
	srv.Close()

	cachePath := filepath.Join(dir, "branches.md")
	if err := os.WriteFile(cachePath, []byte("stale-but-usable"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	_ = os.Chtimes(cachePath, old, old)

	f := New(dir, Options{BaseURL: base, MaxAge: time.Hour})
	f.cacheDir = dir

	got, err := f.Fetch("branches")
	if err != nil || got != "stale-but-usable" {
		t.Fatalf("stale fallback: body=%q err=%v", got, err)
	}
}

func TestCacheDir_UnderRepoRoot(t *testing.T) {
	got := CacheDir(filepath.Join("repo", "root"))
	want := filepath.Join("repo", "root", ".cache", "gitlab-api-docs")
	if got != want {
		t.Fatalf("CacheDir = %q, want %q", got, want)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("5"); got != 5*time.Second {
		t.Errorf("delta-seconds: got %v, want 5s", got)
	}
	if got := parseRetryAfter("  10 "); got != 10*time.Second {
		t.Errorf("padded: got %v, want 10s", got)
	}
	for _, v := range []string{"", "0", "-3", "garbage"} {
		if got := parseRetryAfter(v); got != 0 {
			t.Errorf("parseRetryAfter(%q) = %v, want 0", v, got)
		}
	}
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 31*time.Second {
		t.Errorf("http-date future: got %v, want ~30s", got)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Errorf("http-date past: got %v, want 0", got)
	}
}

func TestBackoffDelay(t *testing.T) {
	if got := backoffDelay(1, 3*time.Second); got < 3*time.Second || got > 3*time.Second+baseSpacing {
		t.Errorf("retry-after honored: got %v", got)
	}
	if got := backoffDelay(1, 10*time.Minute); got > maxBackoff+baseSpacing {
		t.Errorf("retry-after cap: got %v, want <= %v", got, maxBackoff+baseSpacing)
	}
	if d := backoffDelay(1, 0); d < time.Second || d >= time.Second+time.Second/2 {
		t.Errorf("attempt 1 backoff: %v", d)
	}
	if d := backoffDelay(3, 0); d < 4*time.Second || d >= 6*time.Second {
		t.Errorf("attempt 3 backoff: %v", d)
	}
}
