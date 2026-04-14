// Package gitlab provides a wrapper around the GitLab REST API v4 client.
package gitlab

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Client wraps the official GitLab API client with project-specific configuration.
// It includes connection resilience: when GitLab is unreachable at startup, the
// server enters degraded mode and automatically recovers when connectivity is restored.
type Client struct {
	inner *gl.Client

	// enterprise indicates whether the GitLab instance is Premium/Ultimate.
	// Used to select EE-specific API queries (e.g. GraphQL branch rules with
	// approval rules, code owner approval, external status checks).
	enterprise bool

	// Connection resilience: lazy initialization with rate-limited recovery.
	healthURL    string       // Direct API URL for health checks (bypasses SDK)
	token        string       // Token for health check authentication
	healthClient *http.Client // Raw HTTP client without resilience wrapper

	// initialized tracks whether Initialize() completed successfully.
	// Uses atomic.Bool for lock-free reads in the hot path (EnsureInitialized).
	initialized atomic.Bool
	// needsLazyInit is set when startup Initialize() fails, enabling
	// EnsureInitialized to attempt recovery on the next API call.
	needsLazyInit atomic.Bool
	// initMu serializes lazy re-initialization attempts.
	initMu sync.Mutex
	// lastInitAttempt prevents thundering herd on a recovering GitLab instance.
	lastInitAttempt time.Time
}

// initCooldown is the minimum interval between lazy re-initialization attempts
// to prevent thundering herd on a recovering GitLab instance.
const initCooldown = 30 * time.Second

// healthTimeout is the HTTP timeout for direct health check requests
// used during initialization (bypasses the SDK transport chain).
const healthTimeout = 10 * time.Second

// SetEnterprise marks the client as connected to a Premium/Ultimate instance.
func (c *Client) SetEnterprise(v bool) { c.enterprise = v }

// IsEnterprise reports whether the GitLab instance is Premium/Ultimate.
func (c *Client) IsEnterprise() bool { return c.enterprise }

// NewClient creates an authenticated GitLab client from the provided configuration.
// When cfg.SkipTLSVerify is true, TLS certificate verification is disabled (for self-signed certs).
// The client includes a resilience transport that enables automatic recovery
// when GitLab becomes available after being unreachable at startup.
func NewClient(cfg *config.Config) (*Client, error) {
	base := buildBaseTransport(cfg.SkipTLSVerify)

	c := &Client{
		healthURL:    strings.TrimRight(cfg.GitLabURL, "/") + "/api/v4/version",
		token:        cfg.GitLabToken,
		healthClient: &http.Client{Transport: base, Timeout: healthTimeout},
		enterprise:   cfg.Enterprise,
	}

	sdkHTTPClient := &http.Client{
		Transport: &dotUnescapeTransport{
			base: &resilienceTransport{base: base, client: c},
		},
	}

	inner, err := gl.NewClient(
		cfg.GitLabToken,
		gl.WithBaseURL(cfg.GitLabURL),
		gl.WithHTTPClient(sdkHTTPClient),
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client: %w", err)
	}

	c.inner = inner
	return c, nil
}

// NewClientWithToken creates an authenticated GitLab client with explicit
// parameters. Unlike [NewClient], it does not require a full [config.Config]
// and is designed for use in the server pool where each client has a
// unique token but shares the same base URL and TLS settings.
// The client includes a resilience transport that enables automatic recovery
// when GitLab becomes available after being unreachable.
func NewClientWithToken(baseURL, token string, skipTLSVerify bool) (*Client, error) {
	base := buildBaseTransport(skipTLSVerify)

	c := &Client{
		healthURL:    strings.TrimRight(baseURL, "/") + "/api/v4/version",
		token:        token,
		healthClient: &http.Client{Transport: base, Timeout: healthTimeout},
	}

	sdkHTTPClient := &http.Client{
		Transport: &dotUnescapeTransport{
			base: &resilienceTransport{base: base, client: c},
		},
	}

	inner, err := gl.NewClient(
		token,
		gl.WithBaseURL(baseURL),
		gl.WithHTTPClient(sdkHTTPClient),
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client: %w", err)
	}

	c.inner = inner
	return c, nil
}

// Ping validates connectivity and authentication by calling the GitLab version endpoint.
// Returns the GitLab version string on success.
// Callers should wrap ctx with context.WithTimeout to bound the network round-trip.
func (c *Client) Ping(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	v, _, err := c.inner.Version.GetVersion()
	if err != nil {
		return "", fmt.Errorf("gitlab ping failed: %w", err)
	}
	if v == nil || v.Version == "" {
		return "", errors.New("gitlab ping failed: empty version in response")
	}
	return v.Version, nil
}

// CurrentUsername returns the username of the authenticated GitLab user.
// It calls the /user API endpoint and returns the username field.
// Callers should wrap ctx with context.WithTimeout to bound the network round-trip.
func (c *Client) CurrentUsername(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	u, _, err := c.inner.Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("fetching current user: %w", err)
	}
	return u.Username, nil
}

// GL returns the underlying gitlab client for use in tool handlers.
func (c *Client) GL() *gl.Client {
	return c.inner
}

// Initialize validates GitLab connectivity via a direct HTTP health check
// (bypassing the SDK transport chain to avoid recursion). On success it
// marks the client as initialized and returns the GitLab version string.
func (c *Client) Initialize(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	ver, err := c.pingDirect(ctx)
	if err != nil {
		return "", err
	}

	c.initialized.Store(true)
	return ver, nil
}

// EnsureInitialized attempts lazy re-initialization if the client was not
// initialized at startup (e.g. GitLab was down). This allows automatic
// recovery when GitLab becomes available again. Thread-safe via initMu.
// Includes a 30-second cooldown between attempts to avoid hammering GitLab.
// Called automatically by [resilienceTransport] on every SDK request.
func (c *Client) EnsureInitialized(ctx context.Context) {
	if !c.needsLazyInit.Load() {
		return
	}

	c.initMu.Lock()
	defer c.initMu.Unlock()

	// Double-check after acquiring lock.
	if c.initialized.Load() {
		return
	}

	// Rate limit: at most one attempt per cooldown period.
	if time.Since(c.lastInitAttempt) < initCooldown {
		return
	}
	c.lastInitAttempt = time.Now()

	if _, err := c.Initialize(ctx); err != nil {
		slog.Debug("lazy re-initialization failed", "error", err)
		return
	}
	c.needsLazyInit.Store(false)
	slog.Info("gitlab client recovered — lazy initialization succeeded")
}

// EnableLazyInit enables lazy re-initialization on subsequent API calls.
// Called when startup Initialize() fails so that the server can recover
// automatically when GitLab becomes available again.
func (c *Client) EnableLazyInit() { c.needsLazyInit.Store(true) }

// IsInitialized returns true if Initialize() completed successfully.
func (c *Client) IsInitialized() bool { return c.initialized.Load() }

// MarkInitialized sets the initialized flag without running the full
// Initialize flow. Intended for test setups where the client is preconfigured
// with a token or mock credentials.
func (c *Client) MarkInitialized() { c.initialized.Store(true) }

// pingDirect performs a raw HTTP GET to /api/v4/version using the dedicated
// health client, bypassing the SDK transport chain entirely. This prevents
// recursion when called from [EnsureInitialized] inside [resilienceTransport].
func (c *Client) pingDirect(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.healthURL, http.NoBody) //#nosec G704 -- healthURL is built from admin-configured GITLAB_URL, not user input
	if err != nil {
		return "", fmt.Errorf("creating health request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.healthClient.Do(req) //#nosec G704 -- request URL derived from admin config
	if err != nil {
		return "", fmt.Errorf("gitlab ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("gitlab ping: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var v struct {
		Version string `json:"version"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("gitlab ping: decoding version: %w", err)
	}
	if v.Version == "" {
		return "", errors.New("gitlab ping failed: empty version in response")
	}

	return v.Version, nil
}

// buildBaseTransport returns the base HTTP round tripper with optional TLS
// configuration. When skipTLSVerify is true, TLS certificate verification is
// disabled to support self-signed certificates in development environments.
func buildBaseTransport(skipTLSVerify bool) http.RoundTripper {
	if skipTLSVerify {
		return &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //#nosec G402 -- intentional for self-signed certificates, controlled by GITLAB_SKIP_TLS_VERIFY
			},
		}
	}
	return http.DefaultTransport
}

// resilienceTransport wraps an [http.RoundTripper] and calls
// [Client.EnsureInitialized] before each request. This enables transparent
// recovery when GitLab becomes available after being unreachable at startup.
// The overhead in normal operation is a single atomic read (fast path).
type resilienceTransport struct {
	base   http.RoundTripper
	client *Client
}

// RoundTrip calls [Client.EnsureInitialized] for automatic recovery, then
// delegates to the base transport.
func (t *resilienceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.client.EnsureInitialized(req.Context())
	return t.base.RoundTrip(req)
}

// dotUnescapeTransport reverses the percent-encoding of dots (%2E → .) in URL
// paths. The gitlab client-go/v2 library's PathEscape intentionally encodes
// dots, but some GitLab instances (behind certain reverse proxies or WAFs)
// reject %2E-encoded URLs with 403 Forbidden.
type dotUnescapeTransport struct {
	base http.RoundTripper
}

// RoundTrip replaces percent-encoded dots (%2E) with literal dots in the
// request URL path, then delegates to the base transport.
func (t *dotUnescapeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.RawPath != "" {
		req.URL.RawPath = strings.ReplaceAll(req.URL.RawPath, "%2E", ".")
	}
	return t.base.RoundTrip(req)
}
