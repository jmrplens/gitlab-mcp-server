package gitlab

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
	"golang.org/x/oauth2"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/telemetry"
)

// Client wraps the official GitLab API client with project-specific configuration.
// It includes connection resilience: when GitLab is unreachable at startup, the
// server enters degraded mode and automatically recovers when connectivity is restored.
type Client struct {
	inner   *gl.Client
	baseURL string

	// tier holds the resolved GitLab licensing tier (Free/Premium/Ultimate).
	// IsEnterprise() derives the legacy Premium/Ultimate notion from it. Stored
	// atomically for lock-free reads in the hot path, and as an int64 because
	// edition.Tier is an int: int to int64 is a widening on every platform Go
	// supports, so the store is a plain conversion rather than a narrowing
	// that would have to be argued safe. Used to select EE-specific API
	// queries (e.g. GraphQL branch rules with approval rules, code owner
	// approval, external status checks).
	tier atomic.Int64

	// bearerAuth selects the auth scheme for the raw probes this client
	// makes outside the SDK (health/version, credential check): true sends
	// "Authorization: Bearer" (oauth HTTP mode, where gloas- tokens are
	// only valid as Bearer), false sends PRIVATE-TOKEN (PAT clients).
	bearerAuth bool

	// unbound marks the credential-less client from [NewUnboundClient], which
	// refuses every request. Handlers on a shared server capture it and resolve
	// the caller's own through [Client.For], so a handler that still holds this
	// one is a request nothing could attribute to a credential. See
	// [Client.IsUnbound].
	unbound bool

	// Connection resilience: lazy initialization with rate-limited recovery.
	healthURL    string       // Direct API URL for health checks (bypasses SDK)
	token        string       // Token for health check authentication
	healthClient *http.Client // Raw HTTP client without resilience wrapper

	// maxResponse caps how many bytes of one decompressed response body this
	// client will read. See [DefaultMaxResponseBytes].
	maxResponse atomic.Int64

	// destination is what this client may open a connection to. It is read
	// per request by [destinationTransport] and enforced at the dialer, so it
	// covers the first hop and every redirect alike. See [destinationPolicy]
	// and ADR-0022.
	destination atomic.Pointer[destinationPolicy]

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

	// onUnauthorized is told when GitLab answers a call made with this
	// client's credential with 401, and what the 401 said. The server pool
	// uses it to stop serving a credential GitLab has refused, instead of
	// serving it until the periodic check notices. See [Client.SetOnUnauthorized].
	onUnauthorized atomic.Pointer[func(UnauthorizedAnswer)]
	// unauthorizedOnce makes a 401 naming the credential reach the hook once:
	// the verdict is final, so a second one would only repeat it.
	unauthorizedOnce sync.Once
}

// SetOnUnauthorized registers fn to run when GitLab answers a call made with
// this client's credential with 401, and says what the 401 named. fn runs on
// the goroutine that made the call, so it must be cheap and must not block; a
// nil fn clears it.
//
// The two answers arrive differently, because they mean different things. A
// 401 naming the credential ([UnauthorizedCredential]) is GitLab's verdict on
// it, and reaches fn once for the client's lifetime. A 401 naming nothing
// ([UnauthorizedUnexplained]) reaches fn every time: it may be a permission
// refusal of one call, which says nothing about the next, so it must neither
// stand for the credential's verdict nor use up the one delivery that verdict
// gets. Deciding how often to act on it is fn's business.
//
// Only the SDK's requests are reported. The raw probes this client makes
// outside the SDK are not, since their callers act on the answer themselves.
func (c *Client) SetOnUnauthorized(fn func(UnauthorizedAnswer)) {
	if fn == nil {
		c.onUnauthorized.Store(nil)
		return
	}
	c.onUnauthorized.Store(&fn)
}

// notifyUnauthorized tells the registered callback what a 401 named: once for
// a 401 naming the credential, every time for one naming nothing.
func (c *Client) notifyUnauthorized(answer UnauthorizedAnswer) {
	fn := c.onUnauthorized.Load()
	if fn == nil {
		return
	}
	if answer == UnauthorizedCredential {
		c.unauthorizedOnce.Do(func() { (*fn)(answer) })
		return
	}
	(*fn)(answer)
}

// initCooldown is the minimum interval between lazy re-initialization attempts
// to prevent thundering herd on a recovering GitLab instance.
const initCooldown = 30 * time.Second

// healthTimeout is the HTTP timeout for direct health check requests
// used during initialization (bypasses the SDK transport chain).
const healthTimeout = 10 * time.Second

// versionAPIPath is the GitLab Version API endpoint used for health and
// edition probes.
const versionAPIPath = "/api/v4/version"

// namespacePlanPageSize and namespacePlanMaxPages bound the namespace listing
// the tier probe reads.
//
// One page was not enough: a caller whose only paid namespace sat past the
// hundredth resolved Free, and nothing said so. The probe now pages, and two
// things keep that from becoming a cost on every credential. It stops at the
// first Ultimate, since no later namespace can raise the answer, so the case
// this exists for is usually settled by the first page. And it stops after
// namespacePlanMaxPages either way, because an account may administer
// thousands and refining a tier is not worth walking all of them; a caller
// past that bound resolves lower than they should, which the warning and the
// explicit tier both answer.
const (
	namespacePlanPageSize = 100
	namespacePlanMaxPages = 10
)

// GitLabDotComHost is the canonical host for GitLab SaaS-only features.
const GitLabDotComHost = "gitlab.com"

// SetTier records the resolved GitLab licensing tier for this client.
func (c *Client) SetTier(t edition.Tier) { c.tier.Store(int64(t)) }

// Tier returns the resolved GitLab licensing tier for this client.
func (c *Client) Tier() edition.Tier { return edition.Tier(c.tier.Load()) }

// SetEnterprise marks the client as connected to a Premium/Ultimate instance.
// It sets the tier to Premium when v is true and Free when v is false; callers
// needing the Premium/Ultimate distinction should use [Client.SetTier].
func (c *Client) SetEnterprise(v bool) {
	if v {
		c.SetTier(edition.Premium)
		return
	}
	c.SetTier(edition.Free)
}

// IsEnterprise reports whether the GitLab instance is Premium/Ultimate, derived
// from the resolved tier (tier >= Premium).
func (c *Client) IsEnterprise() bool { return c.Tier().IsEnterprise() }

// IsGitLabDotCom reports whether the client is configured for GitLab.com.
func (c *Client) IsGitLabDotCom() bool {
	if c == nil {
		return false
	}
	return IsGitLabDotComURL(c.baseURL)
}

// IsGitLabDotComURL reports whether rawURL targets the canonical GitLab.com host.
func IsGitLabDotComURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Hostname(), GitLabDotComHost)
}

// NewClient creates an authenticated GitLab client from the provided configuration.
// When cfg.SkipTLSVerify is true, TLS certificate verification is disabled (for self-signed certs).
// The client includes a resilience transport that enables automatic recovery
// when GitLab becomes available after being unreachable at startup.
func NewClient(cfg *config.Config) (*Client, error) {
	pools := buildDestinationPools(cfg.SkipTLSVerify)

	c := &Client{
		baseURL:   cfg.GitLabURL,
		healthURL: strings.TrimRight(cfg.GitLabURL, "/") + versionAPIPath,
		token:     cfg.GitLabToken,
	}
	c.SetTier(cfg.Tier)
	c.maxResponse.Store(DefaultMaxResponseBytes)
	c.destination.Store(newDestinationPolicy(cfg.GitLabURL, false, allowPrivateInstances()))
	c.healthClient = newHealthClient(pools, cfg.GitLabURL, c)

	sdkHTTPClient := &http.Client{
		Transport:     apiTransport(pools, c),
		CheckRedirect: credentialSafeRedirect(cfg.GitLabURL),
	}

	options := []gl.ClientOptionFunc{
		gl.WithBaseURL(cfg.GitLabURL),
		gl.WithHTTPClient(sdkHTTPClient),
	}
	options = append(options, retryOptions()...)
	if cfg.DisableRetries {
		options = append(options, gl.WithoutRetries())
	}

	inner, err := gl.NewClient(
		cfg.GitLabToken,
		options...,
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
	return NewClientWithTokenRetries(baseURL, token, skipTLSVerify, false)
}

// NewClientWithTokenRetries is [NewClientWithToken] with the retry policy
// exposed: disableRetries turns off client-go's retryablehttp wrapper
// (RetryMax 5, linear backoff), which unit tests need — a pooled client
// probing a mock that answers 5xx otherwise sleeps through the whole
// credential-check deadline instead of reading the answer it already has.
func NewClientWithTokenRetries(baseURL, token string, skipTLSVerify, disableRetries bool) (*Client, error) {
	return newTokenClient(baseURL, token, skipTLSVerify, disableRetries, maxRetryBackoff)
}

// newTokenClient is the body of [NewClientWithTokenRetries] with the backoff
// ceiling passed in rather than read from [maxRetryBackoff]. See
// [retryOptionsWithCeiling] for why that seam exists; every caller outside a
// test passes the package constant.
func newTokenClient(baseURL, token string, skipTLSVerify, disableRetries bool, retryCeiling time.Duration) (*Client, error) {
	pools := buildDestinationPools(skipTLSVerify)

	c := &Client{
		baseURL:   baseURL,
		healthURL: strings.TrimRight(baseURL, "/") + versionAPIPath,
		token:     token,
	}
	c.maxResponse.Store(DefaultMaxResponseBytes)
	c.destination.Store(newDestinationPolicy(baseURL, false, allowPrivateInstances()))
	c.healthClient = newHealthClient(pools, baseURL, c)

	sdkHTTPClient := &http.Client{
		Transport:     apiTransport(pools, c),
		CheckRedirect: credentialSafeRedirect(baseURL),
	}

	options := []gl.ClientOptionFunc{
		gl.WithBaseURL(baseURL),
		gl.WithHTTPClient(sdkHTTPClient),
	}
	options = append(options, retryOptionsWithCeiling(retryCeiling)...)
	if disableRetries {
		options = append(options, gl.WithoutRetries())
	}

	inner, err := gl.NewClient(
		token,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client: %w", err)
	}

	c.inner = inner
	return c, nil
}

// NewOAuthClientWithToken creates a GitLab client that authenticates with
// "Authorization: Bearer" instead of the PRIVATE-TOKEN header. The server
// pool uses it in oauth HTTP mode, where every credential arrives as a
// Bearer token: an OAuth access token (gloas-...) is ONLY valid as Bearer —
// GitLab rejects it in PRIVATE-TOKEN — while personal access tokens are
// valid in both schemes, so forwarding exactly as received is correct for
// every token kind the mode admits.
func NewOAuthClientWithToken(baseURL, token string, skipTLSVerify bool) (*Client, error) {
	pools := buildDestinationPools(skipTLSVerify)

	c := &Client{
		baseURL:    baseURL,
		healthURL:  strings.TrimRight(baseURL, "/") + versionAPIPath,
		token:      token,
		bearerAuth: true,
	}
	c.maxResponse.Store(DefaultMaxResponseBytes)
	c.destination.Store(newDestinationPolicy(baseURL, false, allowPrivateInstances()))
	c.healthClient = newHealthClient(pools, baseURL, c)

	sdkHTTPClient := &http.Client{
		Transport:     apiTransport(pools, c),
		CheckRedirect: credentialSafeRedirect(baseURL),
	}

	options := []gl.ClientOptionFunc{
		gl.WithBaseURL(baseURL),
		gl.WithHTTPClient(sdkHTTPClient),
	}
	options = append(options, retryOptions()...)

	inner, err := gl.NewAuthSourceClient(
		gl.OAuthTokenSource{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})},
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab oauth client: %w", err)
	}

	c.inner = inner
	return c, nil
}

// HTTPTransport returns the GitLab HTTP transport configured with the same TLS
// policy used by authenticated GitLab clients.
//
// A request made through it carries no destination policy, so the dialer
// applies tier A and nothing else, as it always has: a caller that borrows the
// transport chose its own destination (ADR-0022). Without skipped
// verification it is the permissive half of [sharedDestinationPools], which is
// the pool such a request belongs in: a connection it finds there leads to an
// address tier A allows, and one it opens is one any request routed there
// would have been allowed to dial as well.
func HTTPTransport(skipTLSVerify bool) http.RoundTripper {
	return buildBaseTransport(skipTLSVerify)
}

// Ping validates connectivity and authentication by calling the GitLab version endpoint.
// Returns the GitLab version string on success.
// Callers should wrap ctx with context.WithTimeout to bound the network round-trip.
func (c *Client) Ping(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// client-go builds every request from context.Background() and only
	// WithContext replaces it, so without this the caller's deadline bounds
	// the check above and nothing else.
	v, _, err := c.inner.Version.GetVersion(gl.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("gitlab ping failed: %w", err)
	}
	if v == nil || v.Version == "" {
		return "", errors.New("gitlab ping failed: empty version in response")
	}
	return v.Version, nil
}

// CurrentUserInfo holds the identity of the authenticated GitLab user.
type CurrentUserInfo struct {
	UserID   int
	Username string
}

// CurrentUser returns the identity of the authenticated GitLab user.
// It calls the /user API endpoint and returns both the numeric ID and username.
func (c *Client) CurrentUser(ctx context.Context) (*CurrentUserInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	u, _, err := c.inner.Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("fetching current user: %w", err)
	}
	return &CurrentUserInfo{UserID: int(u.ID), Username: u.Username}, nil
}

// CurrentUsername returns the username of the authenticated GitLab user.
//
// Deprecated: Use [Client.CurrentUser] which returns both ID and username.
func (c *Client) CurrentUsername(ctx context.Context) (string, error) {
	info, err := c.CurrentUser(ctx)
	if err != nil {
		return "", err
	}
	return info.Username, nil
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

	versionInfo, err := c.versionDirect(ctx)
	if err != nil {
		return "", err
	}
	if versionInfo.Enterprise != nil {
		c.SetEnterprise(*versionInfo.Enterprise)
	}

	c.initialized.Store(true)
	return versionInfo.Version, nil
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
		slog.DebugContext(ctx, "lazy re-initialization failed", "error", err)
		return
	}
	c.needsLazyInit.Store(false)
	slog.InfoContext(ctx, "gitlab client recovered. Lazy initialization succeeded")
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
func (c *Client) pingDirect(ctx context.Context) error {
	_, err := c.versionDirect(ctx)
	return err
}

// DetectEnterprise updates the client edition flag from /api/v4/version when
// GitLab exposes it, returning fallback when the field is absent or detection
// fails.
func (c *Client) DetectEnterprise(ctx context.Context, fallback bool) bool {
	versionInfo, err := c.versionDirect(ctx)
	if err != nil {
		slog.WarnContext(ctx, "failed to detect GitLab edition, using configured enterprise mode", "error", err, "fallback", fallback)
		c.SetEnterprise(fallback)
		return fallback
	}
	if versionInfo.Enterprise == nil {
		slog.DebugContext(ctx, "GitLab version endpoint did not report edition, using configured enterprise mode", "version", versionInfo.Version, "fallback", fallback)
		c.SetEnterprise(fallback)
		return fallback
	}
	c.SetEnterprise(*versionInfo.Enterprise)
	slog.InfoContext(ctx, "detected GitLab edition", "version", versionInfo.Version, "enterprise", *versionInfo.Enterprise)
	return *versionInfo.Enterprise
}

// DetectTier resolves the GitLab licensing tier and stores it on the client.
//
// It asks two questions, because neither one answers for every deployment:
//
//   - GET /license, which carries the instance's own plan and is **admin-only
//     on self-managed**. It is the authoritative answer where it is available.
//   - GET /namespaces, which carries a plan per namespace and is readable by
//     any token, for the namespaces the caller administers. On GitLab.com that
//     plan is the subscription; on self-managed it is "default" whatever the
//     instance is licensed for, since a subscription is a GitLab.com concept.
//
// Measured on 2026-09-22 against a licensed EE 19.3.1 and against GitLab.com:
// a non-admin token is refused /license with 403 on both, /namespaces reports
// "default" for admin and non-admin alike on self-managed, and reports the real
// plan on GitLab.com for each namespace the caller administers. So the second
// question rescues every GitLab.com caller, which is the population that used
// to fall back to Free unconditionally, and rescues nobody on self-managed.
//
// Where neither answers, the tier is [edition.Free] as before. That is right on
// a CE instance and wrong on a licensed one the caller cannot read the license
// of, and the two are told apart by the edition rather than guessed at: an
// enterprise build that could not be resolved gets a warning naming the flag
// that settles it, and a CE build stays silent because Free is the truth there.
func (c *Client) DetectTier(ctx context.Context) edition.Tier {
	if tier, ok := c.tierFromLicense(ctx); ok {
		c.SetTier(tier)
		return tier
	}
	if tier, ok := c.tierFromNamespaces(ctx); ok {
		c.SetTier(tier)
		return tier
	}

	c.warnUnresolvedTier(ctx)
	c.SetTier(edition.Free)
	return edition.Free
}

// tierFromLicense reads the instance license, which only an administrator may
// do on a self-managed instance and nobody may do on GitLab.com.
func (c *Client) tierFromLicense(ctx context.Context) (edition.Tier, bool) {
	lic, _, err := c.inner.License.GetLicense(gl.WithContext(ctx))
	if err != nil {
		slog.DebugContext(ctx, "the instance license is not readable with this token, trying the namespace plan",
			"error", err)
		return edition.Free, false
	}
	if lic == nil || strings.TrimSpace(lic.Plan) == "" {
		slog.DebugContext(ctx, "GitLab license reported no plan, trying the namespace plan")
		return edition.Free, false
	}
	tier := edition.TierFromPlan(lic.Plan)
	slog.InfoContext(ctx, "detected GitLab tier from the instance license", "plan", lic.Plan, "tier", tier.String())
	return tier, true
}

// tierFromNamespaces reads the plan of the namespaces the caller administers
// and answers with the highest paid one.
//
// **The highest rather than the caller's own**, and the asymmetry is the same
// one ADR-0018 records for token scopes: a tier resolved too low silently
// removes tools, and the caller cannot tell the difference between a capability
// this deployment lacks and one it was not given, while a tier resolved too
// high surfaces as GitLab's own refusal on the one call that needed it. A
// GitLab.com account whose personal namespace is Free and who works in an
// Ultimate group is the case this exists for.
//
// **"default" is the one plan that answers nothing**, and telling it apart from
// "free" is what keeps the warning honest. Measured on 2026-09-22: GitLab.com
// reports "free" for a namespace on the free plan, and self-managed reports
// "default" for every namespace whatever the instance is licensed for. So a
// namespace saying "free" has answered the question, and its caller is served
// Free with no warning; a namespace saying "default" has not, and an enterprise
// build falls through to one. Reading both as Free would warn every GitLab.com
// account on the free plan, on every startup, about a tier that is correct.
//
// **It pages**, under the two bounds namespacePlanPageSize and
// namespacePlanMaxPages describe: reading only the first page resolved Free for
// a caller whose paid namespace sat past the hundredth. A page that fails after
// an earlier one answered keeps that answer rather than discarding it.
func (c *Client) tierFromNamespaces(ctx context.Context) (edition.Tier, bool) {
	opts := &gl.ListNamespacesOptions{}
	opts.PerPage = namespacePlanPageSize
	opts.Page = 1

	best, found := edition.Free, false
	// No bound in the loop header: the page cap below is what ends the walk,
	// and it has to be the one that does, since it is where the walk says it
	// stopped short. A second copy of the bound up here could never be reached
	// first, which is a condition that reads as a limit and decides nothing.
	for page := 1; ; page++ {
		namespaces, resp, err := c.inner.Namespaces.ListNamespaces(opts, gl.WithContext(ctx))
		if err != nil {
			// A page that fails after an earlier one answered keeps what it
			// answered: the tier found so far is a fact, and discarding it
			// would resolve lower than the caller has already been shown to
			// hold.
			slog.DebugContext(ctx, "could not list namespaces to resolve the tier", "page", page, "error", err)
			break
		}

		for _, ns := range namespaces {
			if ns == nil || !namespacePlanAnswers(ns.Plan) {
				continue
			}
			tier := edition.TierFromPlan(ns.Plan)
			if !found || tier > best {
				best, found = tier, true
				slog.DebugContext(ctx, "a namespace reports a plan", "namespace", ns.FullPath, "plan", ns.Plan, "tier", tier.String())
			}
		}

		// Nothing a later page carries can raise the answer past the highest
		// tier there is, so stop asking.
		if best == edition.Ultimate {
			break
		}
		next := nextNamespacePage(resp)
		if next <= 0 {
			break
		}
		if page == namespacePlanMaxPages {
			slog.DebugContext(ctx, "stopped reading namespaces at the page bound; a paid plan past it is not seen",
				"pages", namespacePlanMaxPages, "per_page", namespacePlanPageSize)
			break
		}
		opts.Page = next
	}

	if found {
		slog.InfoContext(ctx, "detected GitLab tier from the namespace plan", "tier", best.String())
	}
	return best, found
}

// nextNamespacePage is the page GitLab says follows resp, or zero when it names
// none.
//
// client-go hands back a response with every nil error, so the SDK never
// brings a nil one here. The check stays so that a change on its side ends the
// walk instead of panicking in the middle of resolving a tier, and it lives in
// a function of its own because that is the one place a test can hand it nil.
func nextNamespacePage(resp *gl.Response) int64 {
	if resp == nil {
		return 0
	}
	return resp.NextPage
}

// namespacePlanAnswers reports whether a namespace's plan says anything about
// the tier.
//
// Everything does except the empty string, which a namespace the caller does
// not administer carries, and "default", which is what self-managed reports for
// every namespace because a subscription is a GitLab.com concept. Both mean the
// question went unanswered rather than that the answer is Free.
func namespacePlanAnswers(plan string) bool {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "", "default":
		return false
	default:
		return true
	}
}

// warnUnresolvedTier says so when falling back to Free may be wrong, and stays
// quiet when it cannot be.
//
// On a CE build Free is the truth and a warning every startup would be noise
// an operator learns to ignore. On an enterprise build it may well be a
// licensed instance whose license this token cannot read, and the surface the
// caller gets is smaller than the one they are paying for, which is worth one
// line naming the setting that fixes it.
func (c *Client) warnUnresolvedTier(ctx context.Context) {
	if !c.DetectEnterprise(ctx, false) {
		slog.DebugContext(ctx, "no license and no paid namespace plan on a CE instance; the tier is free")
		return
	}
	slog.WarnContext(ctx, "could not determine the licensing tier of this enterprise instance, serving the Free tool surface; "+
		"the license is readable only by an administrator, and a namespace plan is reported only on GitLab.com. "+
		"Set GITLAB_MCP_TIER (or --tier in HTTP mode) to premium or ultimate if this instance is licensed")
}

// gitLabVersionInfo captures the subset of /api/v4/version needed for health
// checks and GitLab edition detection.
type gitLabVersionInfo struct {
	Version    string `json:"version"`
	Enterprise *bool  `json:"enterprise"`
}

// setAuthHeader applies this client's auth scheme to a raw request.
func (c *Client) setAuthHeader(req *http.Request) {
	if c.bearerAuth {
		req.Header.Set("Authorization", "Bearer "+c.token)
		return
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
}

// versionDirect queries the GitLab Version API through the raw health client.
// It bypasses the resilient SDK wrapper so edition detection can run during
// client initialization and degraded-mode recovery. The URL it asks is
// healthURL, derived once from the normalized base URL the operator
// configured, never from a request.
func (c *Client) versionDirect(ctx context.Context) (*gitLabVersionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.healthURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating health request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.healthClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("gitlab ping: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var versionInfo gitLabVersionInfo
	if err = json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return nil, fmt.Errorf("gitlab ping: decoding version: %w", err)
	}
	if versionInfo.Version == "" {
		return nil, errors.New("gitlab ping failed: empty version in response")
	}

	return &versionInfo, nil
}

// CredentialVerdict is what GitLab answered when asked whether it accepts a
// credential. See [Client.CheckCredential].
type CredentialVerdict uint8

const (
	// CredentialUnanswered means no verdict was obtained: a transport error, a
	// timeout, a 404 from a stubbed instance, a 5xx. It is the zero value,
	// because a question nobody answered must never read as either answer.
	CredentialUnanswered CredentialVerdict = iota
	// CredentialAccepted means GitLab answered the probe with a 2xx.
	CredentialAccepted
	// CredentialRefused means GitLab answered the probe with 401 or 403.
	CredentialRefused
)

// CheckCredential asks GitLab whether it accepts this credential, and reports
// which of the three answers it got.
//
// Three and not two, because two callers need different halves of them.
// Admission ([Client.CredentialRejected]) needs only to know whether GitLab
// refused, and admits on anything else so that an instance outage is not a
// total denial of service. The pool's confirmation of a 401 that named no
// cause needs to know whether GitLab accepted: keeping an entry and recording
// that its credential was just checked is only honest on a real answer, and a
// 500 read as "accepted" would push back the credential-age ceiling on the
// strength of a question GitLab never answered. The pool's periodic
// revalidation reads all three, through [Client.CheckCredentialDetail] so it
// can log what each was read from: it evicts on a refusal, stamps the entry on
// an acceptance, and counts anything else as no verdict, since treating a
// briefly unreachable GitLab as a refusal would evict every tenant at once.
//
// It issues GET /api/v4/user through the raw health client rather than the SDK
// on purpose: client-go wraps requests in retryablehttp with RetryMax 5 and a
// linear backoff, which turns a refused connection or a struggling instance
// into seconds of stalling. A liveness question about a credential should be
// asked once and answered fast. The health client also does not report its
// 401s to the unauthorized hook, which is what lets the pool ask this question
// about a 401, or on its revalidation sweep, without the answer raising
// another.
//
// The probe URL is built from the normalized base URL the operator configured,
// never from a request.
func (c *Client) CheckCredential(ctx context.Context) CredentialVerdict {
	return c.CheckCredentialDetail(ctx).Verdict
}

// CredentialCheck is one credential probe's verdict together with what it was
// read from, for a caller that has to say why no verdict was reached.
type CredentialCheck struct {
	// Verdict is what [Client.CheckCredential] reports.
	Verdict CredentialVerdict
	// Status is the HTTP status GitLab answered the probe with, and 0 when no
	// response arrived.
	Status int
	// Err is nil when GitLab answered with a verdict. Otherwise it says why
	// there was none: the error that stopped the request from being built or
	// answered (a refused connection, a TLS failure, a timeout), or, when a
	// response arrived with a status that is neither verdict, that status.
	Err error
}

// CheckCredentialDetail is [Client.CheckCredential] with the cause kept.
//
// The verdict alone serves admission and the pool's confirmation of a 401,
// which act on it and log nothing about the probe. The pool's periodic
// revalidation logs every round that reaches no verdict, and a warning that
// cannot say whether GitLab answered 500, 429 or a redirect, or never
// answered at all, gives an operator who sees it on every round nothing to go
// on.
func (c *Client) CheckCredentialDetail(ctx context.Context) CredentialCheck {
	probeURL := strings.TrimRight(c.baseURL, "/") + "/api/v4/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, http.NoBody)
	if err != nil {
		return CredentialCheck{Err: fmt.Errorf("build the credential probe: %w", err)}
	}
	c.setAuthHeader(req)

	resp, err := c.healthClient.Do(req)
	if err != nil {
		return CredentialCheck{Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))

	check := CredentialCheck{Verdict: credentialVerdictFor(resp.StatusCode), Status: resp.StatusCode}
	if check.Verdict == CredentialUnanswered {
		check.Err = fmt.Errorf("the credential probe was answered HTTP %d, which is neither an acceptance nor a refusal", resp.StatusCode)
	}
	return check
}

// credentialVerdictFor reads the status the credential probe was answered
// with. Only an explicit 401 or 403 refuses and only a 2xx accepts; every
// other status is no verdict at all.
//
// Written as two ifs rather than a tagless switch, because a case expression
// carries no statement counter of its own: the mutation gate reports every
// mutant in one as not covered, and the boundaries of the 2xx range are
// exactly where a mutant is worth seeing killed.
func credentialVerdictFor(status int) CredentialVerdict {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return CredentialRefused
	}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return CredentialAccepted
	}
	return CredentialUnanswered
}

// CredentialRejected reports whether GitLab actively refuses this credential.
//
// It is [Client.CheckCredential] read by admission: only an explicit 401 or
// 403 counts as a rejection, and every other outcome, a transport error, a 404
// from a stubbed instance, a 5xx, means no verdict was obtained and is
// reported as false, so callers fail open.
func (c *Client) CredentialRejected(ctx context.Context) bool {
	return c.CheckCredential(ctx) == CredentialRefused
}

// newHealthClient builds the raw HTTP client used for the version, credential
// and edition probes that bypass the SDK.
//
// It carries the same redirect policy as the SDK client, and needs it more:
// this client sets PRIVATE-TOKEN or Authorization by hand in [Client.setAuthHeader]
// and runs at startup, before any tool call, so an instance answering
// /api/v4/version with a redirect collects the credential before the server
// has served a single request.
//
// The response ceiling is here for the same reason and not because these
// bodies are large: the version probe is the first request the process makes
// and it runs again on every SDK call while degraded, so it is the earliest
// point at which a configured instance gets to answer with whatever it likes.
// It is the client's own ceiling rather than a private one, so
// [Client.SetMaxResponseBytes] means the same thing everywhere.
//
// A 401 it is answered with is not reported to the unauthorized hook, unlike
// one the SDK is answered with: see [responseLimitTransport.reportsUnauthorized].
func newHealthClient(pools destinationPools, baseURL string, c *Client) *http.Client {
	return &http.Client{
		Transport:     &responseLimitTransport{base: &destinationTransport{pools: pools, client: c}, client: c},
		Timeout:       healthTimeout,
		CheckRedirect: credentialSafeRedirect(baseURL),
	}
}

// responseHeaderTimeout bounds the wait for an upstream's response headers.
//
// [http.Client.Timeout] would be the obvious knob and is the wrong one here:
// it covers the body as well, so any ceiling low enough to be useful against a
// stalled instance also kills a legitimate artifact or export download that is
// streaming fine. A response-header timeout separates the two — an upstream
// that accepts the connection and never answers is bounded, a slow but
// progressing download is not — which is the case that otherwise pins a
// goroutine and a socket with no upper bound at all, since client-go's clients
// carry no timeout and net/http's default transport sets none.
//
// Sixty seconds is above anything GitLab takes to produce headers (its own
// worker timeout is of that order) and far below "forever".
const responseHeaderTimeout = 60 * time.Second

// sharedDestinationPools is the pair of verifying transports every client
// shares.
//
// They are built once because the connection pool lives in the transport: a
// per-client transport would give each of up to --max-http-clients pool
// entries its own idle-connection set. They are clones rather than
// [http.DefaultTransport] itself because setting ResponseHeaderTimeout on that
// would change the behavior of every other package in the process. There are
// two rather than one for the reason [destinationPools] gives: a single pool
// let a request the destination guard refuses reuse a connection a request it
// permits had opened.
var sharedDestinationPools = sync.OnceValue(func() destinationPools {
	return destinationPools{
		permissive: newBaseTransport(nil),
		strict:     newBaseTransport(nil),
	}
})

// newBaseTransport clones net/http's default transport and applies this
// package's timeouts, optionally replacing the TLS configuration.
//
// The dialer is this package's own so that [guardDestination] can run as
// ControlContext: it fires after resolution and once per candidate address,
// which is the only position from which a name that resolves somewhere else
// than it claims can be judged by what it resolved to. Its timeouts restate
// net/http's defaults, because replacing DialContext replaces the dialer that
// carried them. It is reached through [guardedDial], which is the only place
// that can tell the guard a dial is to a proxy rather than to a destination.
//
// Proxy is kept as the clone carries it, which for net/http's own default is
// [http.ProxyFromEnvironment]. [destinationTransport] asks that same field
// which proxy a request goes through, so for a deterministic function, which
// ProxyFromEnvironment is since it reads the environment once per process,
// the router and the transport read one answer. [proxyDialAddress] says what
// is and is not covered when the function is not deterministic.
func newBaseTransport(tlsConfig *tls.Config) *http.Transport {
	var t *http.Transport
	if def, ok := http.DefaultTransport.(*http.Transport); ok {
		t = def.Clone()
	} else {
		t = &http.Transport{}
	}
	t.ResponseHeaderTimeout = responseHeaderTimeout
	t.DialContext = guardedDial(baseDialer())
	if tlsConfig != nil {
		t.TLSClientConfig = tlsConfig
	}
	return t
}

// baseDialer is the dialer every transport this package builds dials through.
//
// It is a function of its own rather than a literal inside [newBaseTransport]
// because a dialer reached only as the method value `DialContext` cannot be
// asked what it was configured with: neither its timeouts nor the hook it
// carries are readable back through the transport, so nothing could assert
// either and a mutation of either went unnoticed.
//
// The timeouts restate net/http's own defaults deliberately. Setting
// [http.Transport.DialContext] replaces the dialer that carried them, so
// leaving them out does not inherit them, it removes them.
func baseDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:        30 * time.Second,
		KeepAlive:      30 * time.Second,
		ControlContext: guardDestination,
	}
}

// buildBaseTransport returns the base HTTP round tripper with optional TLS
// configuration, for a caller that makes its own requests with no destination
// policy (see [HTTPTransport]). When skipTLSVerify is true, TLS certificate
// verification is disabled to support self-signed certificates in development
// environments.
func buildBaseTransport(skipTLSVerify bool) http.RoundTripper {
	if skipTLSVerify {
		return newBaseTransport(insecureTLSConfig())
	}
	return sharedDestinationPools().permissive
}

// buildDestinationPools returns the pair of transports one client routes its
// requests between.
//
// A client that skips certificate verification gets a pair of its own, as it
// always got a transport of its own, because the shared pair verifies. The pair
// is split for the same reason the shared one is: a client's policy answers
// differently for its own instance and for a hop away from it, and the pool
// is what a reused connection is judged by.
func buildDestinationPools(skipTLSVerify bool) destinationPools {
	if skipTLSVerify {
		return destinationPools{
			permissive: newBaseTransport(insecureTLSConfig()),
			strict:     newBaseTransport(insecureTLSConfig()),
		}
	}
	return sharedDestinationPools()
}

// insecureTLSConfig is the TLS configuration of a transport whose operator
// asked for certificate verification to be skipped.
func insecureTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // G402: user-configured opt-in for self-signed certificates via GITLAB_MCP_SKIP_TLS_VERIFY
	}
}

// apiTransport is the full chain the GitLab SDK client speaks through.
//
// Two things happen here and only one of them is telemetry.
//
// The span is the visible half: without it a tool call is a single opaque
// duration, and an operator cannot tell one slow GitLab call from eleven fast
// paginated ones. It is unconditional, because the OpenTelemetry API is a no-op
// when no SDK is installed, so a flag would only add a branch that can disagree
// with whether telemetry is running.
//
// The boundary is the half that matters more and is easy to forget.
// propagation.Baggage.Inject writes a caller's baggage into any outgoing
// request whenever the context carries some, and this server's global
// propagator includes Baggage. So the moment this transport participates in
// tracing, an MCP client's baggage would ride outward to the customer's GitLab
// instance under this server's credential unless something clears it. Not
// forwarding is an action, not a default, and this is where the action goes:
// at the seam between a context that came from a caller and a request we make.
// The trace context itself is untouched, so a distributed trace still joins up.
//
// The health client deliberately does NOT go through here. It probes on a timer
// rather than in response to anything, so its spans would have no parent and
// would arrive steadily forever, burying the calls somebody actually asked for
// under an unbounded stream of liveness checks.
//
// The response ceiling is the innermost wrapper for the same kind of reason:
// what it must bound is the body net/http has already decompressed, so it
// belongs on the far side of the base transport rather than anywhere that
// happens to see the wire bytes. The capture sits just outside it, so the
// body a handler reads a field from under [WithResponseCapture] is the
// bounded one.
//
// The destination stamp is one layer further in still, because what it must
// name is the request as the dialer will see it: each redirect hop reaches
// this chain as a request of its own, and the stamp says whether that hop's
// URL is still the instance's own host. The same layer chooses which of the
// two pools serves the hop, for the reason [destinationPools] gives.
func apiTransport(pools destinationPools, c *Client) http.RoundTripper {
	return mcpotel.NewTransport(&outboundBoundaryTransport{
		base: &dotUnescapeTransport{
			base: &resilienceTransport{
				base: &captureTransport{
					base: &responseLimitTransport{
						base:                &destinationTransport{pools: pools, client: c},
						client:              c,
						reportsUnauthorized: true,
					},
				},
				client: c,
			},
		},
	})
}

// outboundBoundaryTransport strips a caller's baggage from every request this
// client makes. See [instrumented] for why this is not the default.
type outboundBoundaryTransport struct {
	base http.RoundTripper
}

func (t *outboundBoundaryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req.WithContext(telemetry.OutboundContext(req.Context())))
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
