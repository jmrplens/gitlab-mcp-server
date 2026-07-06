// Package apidocs is a shared fetcher for the GitLab API reference docs (the
// doc/api/<area>.md files in the gitlab-org/gitlab monorepo) used as a source of
// truth alongside the client-go SDK by the audit utilities.
//
// Docs are cached on disk in a single shared, gitignored directory
// (.cache/gitlab-api-docs/ under the repo root). A cached doc is reused while it
// is younger than MaxAge (7 days by default); older or missing docs are
// re-downloaded. Callers can force a full refresh or pin to offline (cached
// only). Downloads are polite: a server Retry-After is honored, otherwise
// exponential backoff with jitter is used, and successful fetches are spaced so
// a full sweep does not trip GitLab's raw rate limiter.
package apidocs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const (
	// DefaultBaseURL is the raw GitLab API reference doc root. Each area maps to
	// <DefaultBaseURL><area>.md.
	DefaultBaseURL = "https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/api/"

	// DefaultUserDocBaseURL is the raw doc root one level above the API
	// reference. Some endpoint families carry their only licensing-tier badge on
	// a user-facing page (e.g. merge request dependencies), so tier auditors need
	// to fetch pages relative to doc/ as well as doc/api/.
	DefaultUserDocBaseURL = "https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/"

	// DefaultMaxAge is how long a cached doc is considered fresh before a
	// re-download is attempted.
	DefaultMaxAge = 7 * 24 * time.Hour

	// cacheDirName is the shared, gitignored on-disk cache location relative to
	// the repo root.
	cacheDirRel = ".cache/gitlab-api-docs"

	// Fetch tuning. GitLab's raw endpoint rate-limits bursts with HTTP 429
	// (often carrying a Retry-After header).
	maxAttempts = 6
	baseSpacing = 500 * time.Millisecond
	maxBackoff  = 60 * time.Second
)

// sleepCtx waits for d or until ctx is cancelled, returning ctx.Err() on
// cancellation. It is indirected so tests can make retries instant.
var sleepCtx = func(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// CacheDir returns the shared, gitignored API-doc cache directory for the given
// repository root.
func CacheDir(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(cacheDirRel))
}

// Fetcher downloads and caches GitLab API reference docs. The zero value is not
// usable; construct one with New so defaults are applied.
type Fetcher struct {
	cacheDir string
	baseURL  string
	maxAge   time.Duration
	refresh  bool
	offline  bool
	strict   bool
	client   *http.Client
}

// Options configures a Fetcher.
type Options struct {
	// Refresh forces a re-download even when the cached copy is still fresh.
	Refresh bool
	// Offline never hits the network: cached docs are returned regardless of
	// age, and a cache miss is an error.
	Offline bool
	// Strict disables the stale-cache fallback: when a download fails (e.g. an
	// upstream 404/410 removal), the error is returned instead of serving an
	// older cached copy. Authoritative callers such as -validate-docs set this
	// so a deleted doc is not masked by a previously cached body.
	Strict bool
	// MaxAge overrides the freshness window; 0 uses DefaultMaxAge.
	MaxAge time.Duration
	// BaseURL overrides the doc root (used by tests); empty uses DefaultBaseURL.
	BaseURL string
	// CacheDir overrides the on-disk cache location; empty uses the shared
	// CacheDir(repoRoot). Useful for tests and callers that want an isolated cache.
	CacheDir string
	// Client overrides the HTTP client; nil uses a 30s-timeout default.
	Client *http.Client
}

// New returns a Fetcher writing to the shared cache under repoRoot.
func New(repoRoot string, opts Options) *Fetcher {
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = CacheDir(repoRoot)
	}
	f := &Fetcher{
		cacheDir: cacheDir,
		baseURL:  opts.BaseURL,
		maxAge:   opts.MaxAge,
		refresh:  opts.Refresh,
		offline:  opts.Offline,
		strict:   opts.Strict,
		client:   opts.Client,
	}
	if f.baseURL == "" {
		f.baseURL = DefaultBaseURL
	}
	if f.maxAge <= 0 {
		f.maxAge = DefaultMaxAge
	}
	if f.client == nil {
		f.client = &http.Client{Timeout: 30 * time.Second}
	}
	return f
}

// Fetch returns the markdown for one doc area (e.g. "branches"). It serves a
// cached copy when present and fresh (younger than MaxAge) unless Refresh is
// set; otherwise it downloads, caches, and returns the doc. In Offline mode it
// returns the cached copy at any age and never downloads. The context cancels an
// in-flight download and aborts the retry backoff.
func (f *Fetcher) Fetch(ctx context.Context, area string) (string, error) {
	cachePath := filepath.Join(f.cacheDir, filepath.FromSlash(area)+".md")

	if cached, ok := f.cachedIfUsable(cachePath); ok {
		return cached, nil
	}
	if f.offline {
		return "", fmt.Errorf("apidocs: %s not cached and offline", area)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return "", fmt.Errorf("apidocs: create cache dir: %w", err)
	}

	body, err := f.download(ctx, area)
	if err != nil {
		// In strict mode (authoritative validation) a download failure must
		// surface so a removed upstream doc is not masked by a cached copy.
		if !f.strict {
			// Otherwise fall back to a stale cached copy rather than failing the
			// whole audit when the network is flaky and an older doc is on disk.
			if stale, statErr := os.ReadFile(cachePath); statErr == nil { //#nosec G304 -- cache path is derived, not user input
				cmdutil.Progressf("apidocs: %v; using stale cached %s", err, area)
				return string(stale), nil
			}
		}
		return "", err
	}
	if writeErr := os.WriteFile(cachePath, body, 0o600); writeErr != nil {
		return "", fmt.Errorf("apidocs: write cache %s: %w", cachePath, writeErr)
	}
	return string(body), nil
}

// cachedIfUsable returns the cached doc when it may be served without a
// download: Offline serves any age; otherwise the copy must be fresh and
// Refresh unset.
func (f *Fetcher) cachedIfUsable(cachePath string) (string, bool) {
	info, err := os.Stat(cachePath)
	if err != nil {
		return "", false
	}
	if f.offline {
		data, readErr := os.ReadFile(cachePath) //#nosec G304 -- cache path is derived, not user input
		return string(data), readErr == nil
	}
	if f.refresh {
		return "", false
	}
	if time.Since(info.ModTime()) >= f.maxAge {
		return "", false
	}
	data, readErr := os.ReadFile(cachePath) //#nosec G304 -- cache path is derived, not user input
	return string(data), readErr == nil
}

// download fetches one area with retry, honoring Retry-After and falling back to
// jittered exponential backoff. A cancelled context aborts the wait between
// attempts and returns promptly.
func (f *Fetcher) download(ctx context.Context, area string) ([]byte, error) {
	url := f.baseURL + area + ".md"
	cmdutil.Progressf("apidocs: fetching %s", area)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		body, status, retryAfter, err := fetchOnce(ctx, f.client, url)
		switch {
		case err != nil:
			lastErr = err
		case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
			lastErr = fmt.Errorf("HTTP %d for %s", status, url)
		case status != http.StatusOK:
			return nil, fmt.Errorf("HTTP %d for %s", status, url)
		default:
			// Gentle spacing so a full sweep does not trip the rate limiter.
			// A cancel here is harmless — the body is already in hand.
			_ = sleepCtx(ctx, baseSpacing)
			return body, nil
		}
		if attempt == maxAttempts {
			break
		}
		wait := backoffDelay(attempt, retryAfter)
		cmdutil.Progressf("apidocs: %s: %v; retry %d/%d in %s", area, lastErr, attempt+1, maxAttempts, wait.Round(time.Millisecond))
		if sleepErr := sleepCtx(ctx, wait); sleepErr != nil {
			return nil, fmt.Errorf("apidocs: %s retry aborted: %w", area, errors.Join(lastErr, sleepErr))
		}
	}
	return nil, fmt.Errorf("apidocs: giving up on %s after %d attempts: %w", area, maxAttempts, lastErr)
}

// backoffDelay computes the wait before the next attempt. A server Retry-After
// is honored first (capped at maxBackoff); otherwise exponential backoff
// (1s, 2s, 4s, …) with full jitter avoids synchronized retries.
func backoffDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, maxBackoff) + jitter(baseSpacing)
	}
	exp := min(time.Duration(1<<min(attempt-1, 5))*time.Second, maxBackoff)
	return exp + jitter(exp/2)
}

// jitter returns a random duration in [0, span) (0 when span <= 0).
func jitter(span time.Duration) time.Duration {
	if span <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(span))) //#nosec G404 -- backoff jitter, not security-sensitive
}

// fetchOnce performs a single GET and returns the body, status, and parsed
// Retry-After delay (0 when absent/unparsable).
func fetchOnce(ctx context.Context, client *http.Client, url string) (body []byte, status int, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, 0, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()
	retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, retryAfter, nil
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, retryAfter, err
	}
	return body, resp.StatusCode, retryAfter, nil
}

// parseRetryAfter interprets a Retry-After header, either a non-negative
// delta-seconds integer or an HTTP-date. Returns 0 when empty/unparsable so the
// caller falls back to exponential backoff.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, convErr := strconv.Atoi(v); convErr == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, parseErr := http.ParseTime(v); parseErr == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
