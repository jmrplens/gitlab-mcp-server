package apidocs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// realSleepCtx keeps the production sleep so TestSleepCtx can exercise it
// after TestMain swaps the package variable for the instant version.
var realSleepCtx func(ctx context.Context, d time.Duration) error

// TestMain makes backoff/spacing instant so retry tests run fast, while still
// honoring cancellation so the context-aware paths stay covered.
func TestMain(m *testing.M) {
	realSleepCtx = sleepCtx
	sleepCtx = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
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

// newStatusServer returns an httptest server answering every request with the
// given status and no body, plus the count of requests that reached it. The
// count is what says how often a status was retried.
func newStatusServer(t *testing.T, status int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestFetch_DownloadsThenServesFreshCache(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newServer(t, "# branches doc")
	f := New(dir, Options{BaseURL: srv.URL + "/"})
	f.cacheDir = dir // isolate cache to temp dir

	got, err := f.Fetch(context.Background(), "branches")
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
	if _, err2 := f.Fetch(context.Background(), "branches"); err2 != nil {
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

	got, err := f.Fetch(context.Background(), "branches")
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

	got, err := f.Fetch(context.Background(), "branches")
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

	got, err := f.Fetch(context.Background(), "branches")
	if err != nil || got != "cached" {
		t.Fatalf("offline cached: body=%q err=%v", got, err)
	}
	// Cache miss offline is an error, no network hit.
	if _, missErr := f.Fetch(context.Background(), "missing"); missErr == nil {
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

	got, err := f.Fetch(context.Background(), "branches")
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

	got, err := f.Fetch(context.Background(), "branches")
	if err != nil || got != "stale-but-usable" {
		t.Fatalf("stale fallback: body=%q err=%v", got, err)
	}
}

func TestFetch_StrictVsLenientOn404WithStaleCache(t *testing.T) {
	// Server always 404s (doc removed upstream).
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	seed := func(dir string) {
		cachePath := filepath.Join(dir, "branches.md")
		if err := os.WriteFile(cachePath, []byte("cached body"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-30 * 24 * time.Hour) // stale, forces a download attempt
		_ = os.Chtimes(cachePath, old, old)
	}

	// Lenient (default): a 404 with a stale cache falls back to the cached body.
	lenientDir := t.TempDir()
	seed(lenientDir)
	lenient := New(lenientDir, Options{BaseURL: srv.URL + "/", MaxAge: time.Hour, CacheDir: lenientDir})
	if got, err := lenient.Fetch(context.Background(), "branches"); err != nil || got != "cached body" {
		t.Fatalf("lenient: got %q err %v, want cached fallback", got, err)
	}

	// Strict: the 404 surfaces as an error instead of masking the removal.
	strictDir := t.TempDir()
	seed(strictDir)
	strict := New(strictDir, Options{BaseURL: srv.URL + "/", MaxAge: time.Hour, CacheDir: strictDir, Strict: true})
	if _, err := strict.Fetch(context.Background(), "branches"); err == nil {
		t.Fatal("strict: want error on 404 despite stale cache, got nil")
	}
}

func TestJitter_RangeAndNonPositive(t *testing.T) {
	if got := jitter(0); got != 0 {
		t.Errorf("jitter(0) = %v, want 0", got)
	}
	if got := jitter(-5 * time.Second); got != 0 {
		t.Errorf("jitter(negative) = %v, want 0", got)
	}
	span := 100 * time.Millisecond
	for range 50 {
		if got := jitter(span); got < 0 || got >= span {
			t.Fatalf("jitter(%v) = %v, want [0,%v)", span, got, span)
		}
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
		t.Run(v, func(t *testing.T) {
			if got := parseRetryAfter(v); got != 0 {
				t.Errorf("parseRetryAfter(%q) = %v, want 0", v, got)
			}
		})
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

// TestRequest_Spacing_IsTheOptionTheCallerGave verifies what a successful
// download waits for afterwards, which is the one thing that keeps a 250-page
// sweep under GitLab's raw rate limiter and the one thing a caller fetching
// from an httptest handler in its own process has no use for.
//
// A caller that names no spacing gets baseSpacing, so nothing that reaches
// gitlab.com loses the pause by saying nothing; a caller that asks for none
// gets a non-positive duration, which sleepCtx returns from at once. The
// duration is read off the seam rather than measured with a clock: the point
// is which value the fetcher passes, and a test that measured it would have
// to spend it.
func TestRequest_Spacing_IsTheOptionTheCallerGave(t *testing.T) {
	tests := []struct {
		name string
		opt  time.Duration
		want time.Duration
	}{
		{name: "unset keeps the rate-limiter pause", opt: 0, want: baseSpacing},
		{name: "negative removes it", opt: -1, want: -1},
		{name: "a caller may shorten it", opt: time.Millisecond, want: time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newServer(t, "# doc")
			var got time.Duration
			restore := sleepCtx
			sleepCtx = func(ctx context.Context, d time.Duration) error {
				got = d
				return ctx.Err()
			}
			t.Cleanup(func() { sleepCtx = restore })

			f := New(t.TempDir(), Options{BaseURL: srv.URL + "/", CacheDir: t.TempDir(), Spacing: tt.opt})
			if _, err := f.Fetch(t.Context(), "branches"); err != nil {
				t.Fatalf("Fetch() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("spacing after a successful fetch = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSleepCtx_Scenarios_WaitsOrHonorsCancellation verifies the production
// sleep (the one TestMain replaces for the other tests): a non-positive
// duration returns at once with the context's state, a short wait elapses,
// and a long wait is cut short by cancellation.
func TestSleepCtx_Scenarios_WaitsOrHonorsCancellation(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name    string
		ctx     context.Context
		d       time.Duration
		wantErr error
	}{
		{name: "zero duration returns immediately", ctx: context.Background(), d: 0, wantErr: nil},
		{name: "negative duration reports cancellation", ctx: cancelled, d: -time.Second, wantErr: context.Canceled},
		{name: "short wait elapses", ctx: context.Background(), d: time.Millisecond, wantErr: nil},
		{name: "long wait aborted by cancellation", ctx: cancelled, d: time.Hour, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			err := realSleepCtx(tt.ctx, tt.d)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("sleepCtx() error = %v, want %v", err, tt.wantErr)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Errorf("sleepCtx() took %v, want a prompt return", elapsed)
			}
		})
	}
}

// TestFetch_AStatusTheServerKeepsReturning_DecidesHowOftenItIsAsked verifies
// the retry classification by the one thing it changes that a caller can see:
// how many times GitLab is asked.
//
// A 429 and anything from 500 up are transient, so they are retried until the
// attempts run out and the error says so; every other non-200 is the endpoint's
// answer and is returned from the first request, because retrying a 404 five
// more times delays a 250-page sweep by nothing useful and tells the caller
// nothing it did not know after the first. 499 is here for the boundary: it is
// the largest status that is still not a server error, and a comparison that
// slipped by one would retry it.
//
// The counts are the assertion rather than the error text alone, since the
// loop gives up with the same message however many requests it made.
func TestFetch_AStatusTheServerKeepsReturning_DecidesHowOftenItIsAsked(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		wantRequests int32
		wantErr      string
	}{
		{name: "too many requests is retried to the last attempt", status: http.StatusTooManyRequests, wantRequests: maxAttempts, wantErr: "giving up on branches after 6 attempts"},
		{name: "an internal server error is retried to the last attempt", status: http.StatusInternalServerError, wantRequests: maxAttempts, wantErr: "giving up on branches after 6 attempts"},
		{name: "an unavailable service is retried to the last attempt", status: http.StatusServiceUnavailable, wantRequests: maxAttempts, wantErr: "giving up on branches after 6 attempts"},
		{name: "the status just below a server error is answered at once", status: 499, wantRequests: 1, wantErr: "HTTP 499"},
		{name: "a page that is gone is answered at once", status: http.StatusNotFound, wantRequests: 1, wantErr: "HTTP 404"},
		{name: "a refusal is answered at once", status: http.StatusForbidden, wantRequests: 1, wantErr: "HTTP 403"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, hits := newStatusServer(t, tt.status)
			f := New(t.TempDir(), Options{BaseURL: srv.URL + "/", CacheDir: t.TempDir()})

			got, err := f.Fetch(context.Background(), "branches")

			if err == nil {
				t.Fatalf("Fetch() = %q, nil; want an error: nothing was cached to fall back to", got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Fetch() error = %q, want it to name %q", err, tt.wantErr)
			}
			if asked := atomic.LoadInt32(hits); asked != tt.wantRequests {
				t.Errorf("GitLab was asked %d time(s), want %d", asked, tt.wantRequests)
			}
		})
	}
}

// TestFetch_OfflineWithACacheItCannotRead_Fails verifies that an offline run
// whose cache entry exists but cannot be read refuses rather than serving an
// empty document. A cache entry that stats and will not read is what a half
// written file or a directory of the same name looks like, and answering a
// caller with "" and no error would report the area as documenting nothing.
func TestFetch_OfflineWithACacheItCannotRead_Fails(t *testing.T) {
	cache := t.TempDir()
	if err := os.Mkdir(filepath.Join(cache, "branches.md"), 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	srv, hits := newServer(t, "must not be fetched")
	f := New(t.TempDir(), Options{BaseURL: srv.URL + "/", CacheDir: cache, Offline: true})

	got, err := f.Fetch(context.Background(), "branches")

	if err == nil {
		t.Fatalf("Fetch() = %q, nil; want the offline refusal", got)
	}
	if !strings.Contains(err.Error(), "not cached and offline") {
		t.Errorf("Fetch() error = %q, want it to say the area is not cached", err)
	}
	if asked := atomic.LoadInt32(hits); asked != 0 {
		t.Errorf("offline made %d network request(s), want 0", asked)
	}
}

// TestRequest_TheRetryLine_NamesTheAttemptStillToCome verifies the progress a
// waiting operator reads: a line written before a backoff counts the attempt
// about to be made, not the one that just failed, so the last line of a run
// that gives up reads "retry 6/6" and never "retry 5/6" or "retry 0/6".
func TestRequest_TheRetryLine_NamesTheAttemptStillToCome(t *testing.T) {
	var lines []string
	restore := progressf
	progressf = func(message string, args ...any) { lines = append(lines, fmt.Sprintf(message, args...)) }
	t.Cleanup(func() { progressf = restore })

	srv, _ := newStatusServer(t, http.StatusTooManyRequests)
	f := New(t.TempDir(), Options{BaseURL: srv.URL + "/", CacheDir: t.TempDir()})

	if _, err := f.Fetch(context.Background(), "branches"); err == nil {
		t.Fatal("Fetch() error = nil, want the giving-up error")
	}

	var retries []string
	for _, line := range lines {
		if strings.Contains(line, "retry ") {
			retries = append(retries, line)
		}
	}
	if len(retries) != maxAttempts-1 {
		t.Fatalf("progress wrote %d retry line(s) %q, want %d: one before every backoff but not after the last attempt", len(retries), retries, maxAttempts-1)
	}
	for i, line := range retries {
		want := fmt.Sprintf("retry %d/%d in ", i+2, maxAttempts)
		if !strings.Contains(line, want) {
			t.Errorf("retry line %d = %q, want it to name %q", i+1, line, want)
		}
	}
}

// TestSleepCtx_APositiveDuration_IsSpentBeforeItReturns verifies that the pause
// between attempts is actually taken. It is the only thing keeping a 250-page
// sweep under GitLab's raw rate limiter, and a sleep that returned the
// context's state without waiting would read as a success at every call site
// while retrying a 429 as fast as the network allows.
func TestSleepCtx_APositiveDuration_IsSpentBeforeItReturns(t *testing.T) {
	const wait = 60 * time.Millisecond

	start := time.Now()
	err := realSleepCtx(context.Background(), wait)
	if err != nil {
		t.Fatalf("sleepCtx() error = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < wait/2 {
		t.Errorf("sleepCtx(%v) returned after %v, want it to have waited", wait, elapsed)
	}
}

// TestNew_EmptyOptions_UsesDefaults verifies the zero Options fill in the
// documented base URL, freshness window and HTTP client.
//
// The window and the client's timeout are checked against the durations the
// documentation states rather than against the package's own names, because a
// fetcher whose client never times out hangs a 250-page sweep on one
// unanswered connection, and a freshness window that is not the documented
// week either re-downloads GitLab's whole doc tree on every run or serves a
// page that moved on months ago.
func TestNew_EmptyOptions_UsesDefaults(t *testing.T) {
	f := New(t.TempDir(), Options{})
	if f.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", f.baseURL, DefaultBaseURL)
	}
	if f.maxAge != DefaultMaxAge {
		t.Errorf("maxAge = %v, want %v", f.maxAge, DefaultMaxAge)
	}
	if f.maxAge != 7*24*time.Hour {
		t.Errorf("maxAge = %v, want the documented week", f.maxAge)
	}
	if f.client == nil {
		t.Fatal("client = nil, want the default HTTP client")
	}
	if f.client.Timeout != 30*time.Second {
		t.Errorf("client timeout = %v, want 30s: a fetcher with no timeout waits forever on one unanswered connection", f.client.Timeout)
	}
}

// TestNew_TheOptionsGiven_AreUsed verifies that every override a caller states
// survives New, the HTTP client above all: a caller supplying its own client
// is supplying its own transport and timeout, and replacing it with the
// default would send the audit's requests through neither.
func TestNew_TheOptionsGiven_AreUsed(t *testing.T) {
	client := &http.Client{Timeout: 3 * time.Second}
	cache := t.TempDir()
	f := New(t.TempDir(), Options{
		BaseURL:  "https://example.test/doc/api/",
		AreasURL: "https://example.test/tree",
		MaxAge:   time.Minute,
		Spacing:  -1,
		CacheDir: cache,
		Client:   client,
	})

	if f.client != client {
		t.Errorf("client = %v, want the one the caller supplied", f.client)
	}
	if f.baseURL != "https://example.test/doc/api/" {
		t.Errorf("baseURL = %q, want the caller's", f.baseURL)
	}
	if f.areasURL != "https://example.test/tree" {
		t.Errorf("areasURL = %q, want the caller's", f.areasURL)
	}
	if f.maxAge != time.Minute {
		t.Errorf("maxAge = %v, want the caller's minute", f.maxAge)
	}
	if f.spacing != -1 {
		t.Errorf("spacing = %v, want the negative value that removes the pause", f.spacing)
	}
	if f.cacheDir != cache {
		t.Errorf("cacheDir = %q, want %q", f.cacheDir, cache)
	}
}

// TestFetch_UnusableCache_ReturnsError verifies the two cache write failures
// are reported instead of being masked by a successful download: a cache
// directory that cannot be created because its parent is a file, and a cache
// entry that is a directory. Neither depends on permission bits.
func TestFetch_UnusableCache_ReturnsError(t *testing.T) {
	srv, _ := newServer(t, "fresh body")
	tests := []struct {
		name     string
		cacheDir func(t *testing.T) string
		wantErr  string
	}{
		{
			name: "cache directory parent is a file",
			cacheDir: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				blocker := filepath.Join(dir, "blocker")
				if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
					t.Fatalf("write blocker: %v", err)
				}
				return filepath.Join(blocker, "cache")
			},
			wantErr: "create cache dir",
		},
		{
			name: "cache entry is a directory",
			cacheDir: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := os.Mkdir(filepath.Join(dir, "branches.md"), 0o750); err != nil {
					t.Fatalf("mkdir cache entry: %v", err)
				}
				return dir
			},
			wantErr: "write cache",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New(t.TempDir(), Options{BaseURL: srv.URL + "/", CacheDir: tt.cacheDir(t)})
			_, err := f.Fetch(context.Background(), "branches")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Fetch() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestDownload_CancelledContext_ReturnsRetryAborted verifies that a request
// failing under an already-cancelled context does not spin through the
// retry loop: the backoff wait observes the cancellation and the error
// names both the request failure and the abort.
func TestDownload_CancelledContext_ReturnsRetryAborted(t *testing.T) {
	srv, hits := newServer(t, "never read")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f := New(t.TempDir(), Options{BaseURL: srv.URL + "/"})
	_, err := f.download(ctx, "branches")
	if err == nil || !strings.Contains(err.Error(), "branches retry aborted") || !errors.Is(err, context.Canceled) {
		t.Fatalf("download() error = %v, want the retry-aborted error wrapping context.Canceled", err)
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Errorf("server hits = %d, want 0 for a request cancelled before it was sent", *hits)
	}
}

// TestFetchOnce_Failures_ReturnErrors verifies the two fetchOnce failures
// that are not HTTP statuses: a URL the request constructor rejects, and a
// response whose body ends before its declared Content-Length, which the
// body read reports as an unexpected EOF.
func TestFetchOnce_Failures_ReturnErrors(t *testing.T) {
	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	t.Cleanup(truncated.Close)
	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{name: "unparsable url", url: "::not-a-url", wantStatus: 0},
		{name: "body shorter than content length", url: truncated.URL + "/branches.md", wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, status, _, err := fetchOnce(context.Background(), truncated.Client(), tt.url)
			if err == nil {
				t.Fatalf("fetchOnce() = %q, %d, nil; want an error", body, status)
			}
			if status != tt.wantStatus {
				t.Errorf("fetchOnce() status = %d, want %d", status, tt.wantStatus)
			}
		})
	}
}
