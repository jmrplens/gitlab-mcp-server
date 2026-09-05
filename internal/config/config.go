package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
)

// DefaultMaxFileSize and MaxFileSize define the default and upper bound for
// GitLab upload payload sizes.
const (
	DefaultMaxFileSize = 2 * 1024 * 1024 * 1024    // 2 GB
	MaxFileSize        = 1024 * 1024 * 1024 * 1024 // 1 TB upper bound
)

// HTTP pool defaults.
const (
	DefaultMaxHTTPClients     = 100
	DefaultSessionTimeout     = 30 * time.Minute
	DefaultRevalidateInterval = 15 * time.Minute
	DefaultPoolIdleTimeout    = 1 * time.Hour
	// DefaultActionTimeout bounds one action's handler: it ends one that would
	// otherwise park until its client gave up, and never cuts a legitimate
	// call. It is the longest wait any action offers, a pipeline wait's
	// 3600-second ceiling, plus five minutes for the calls around it, since
	// the deadline starts before the handler does and a default equal to the
	// wait would end it a moment before it returned on its own. toolutil pins
	// the inequality in a test, since this package cannot import the constant
	// it must exceed.
	DefaultActionTimeout = 65 * time.Minute
	// DefaultDrainDelay is zero: on SIGTERM the listener closes at once, as
	// it always has. A deployment behind a balancer that polls /health sets
	// it to at least one probe interval, so the 503 the endpoint answers
	// while draining is seen before the close is.
	DefaultDrainDelay     = 0 * time.Second
	MaxHTTPClients        = 10000
	MaxSessionTimeout     = 24 * time.Hour
	MaxRevalidateInterval = 24 * time.Hour
	MaxPoolIdleTimeout    = 24 * time.Hour
	MaxActionTimeout      = 24 * time.Hour
	// MaxDrainDelay caps the announcement: a delay longer than this holds a
	// stopping process open past what any supervisor waits.
	MaxDrainDelay = 5 * time.Minute
)

// OAuth defaults.
const (
	DefaultOAuthCacheTTL = 15 * time.Minute
	MinOAuthCacheTTL     = 1 * time.Minute
	MaxOAuthCacheTTL     = 2 * time.Hour
)

// DefaultRateLimitBurst is the bucket size used when rps > 0 and the operator
// did not set RATE_LIMIT_BURST explicitly.
//
// DefaultHTTPRateLimitRPS is the tool-call limit an HTTP deployment gets unless
// the operator says otherwise. The specification requires a server exposing
// tools to rate limit their invocation, and an HTTP deployment is the shared
// one: every call it forwards is charged to its own egress address, so one
// looping client's volume lands on every other tenant and on the instance's
// limits. The number itself is a judgement call, not a spec value — far above
// any human-driven session, and still a bound on a retry loop. Stdio keeps 0:
// a single-user local process has no co-tenant to protect, so a limiter there
// only costs latency. Explicit 0 remains the opt-out in both.
const (
	DefaultRateLimitBurst   = 40
	DefaultHTTPRateLimitRPS = 10
	MaxRateLimitRPS         = 1000
	MaxRateLimitBurst       = 10000
)

// Meta-tool param schema modes.
const (
	// MetaParamSchemaOpaque keeps the legacy `params: object` envelope.
	// This is the default and produces the smallest tools/list payload.
	MetaParamSchemaOpaque = "opaque"
	// MetaParamSchemaCompact emits a discriminated `oneOf` per action with
	// descriptions and $defs stripped to reduce size.
	MetaParamSchemaCompact = "compact"
	// MetaParamSchemaFull emits a discriminated `oneOf` per action with the
	// complete reflected JSON Schema for each action's params.
	MetaParamSchemaFull = "full"
	// DefaultMetaParamSchema is the default mode applied when neither the
	// META_PARAM_SCHEMA env var nor the --meta-param-schema flag is set.
	DefaultMetaParamSchema = MetaParamSchemaOpaque
)

// Tool surface modes select which tool catalog is exposed by the server.
const (
	// ToolSurfaceMeta exposes the current domain meta-tool catalog.
	ToolSurfaceMeta = "meta"
	// ToolSurfaceIndividual exposes the full individual tool catalog.
	ToolSurfaceIndividual = "individual"
	// ToolSurfaceDynamic exposes the default low-token find/execute catalog.
	ToolSurfaceDynamic = "dynamic"
	// DefaultToolSurface selects the low-token find/execute catalog by default.
	DefaultToolSurface = ToolSurfaceDynamic
)

// DefaultSocketMode is the permission mode a unix listening socket is created
// with: owner and group may connect, nobody else. A reverse proxy therefore
// reaches the server by sharing a group with it, which is the grant an
// operator makes deliberately — 0666 would let every local account reach an
// endpoint whose whole point is that it is not exposed.
const DefaultSocketMode os.FileMode = 0o660

// Authentication modes select how HTTP mode authenticates a request.
const (
	// AuthModeLegacy accepts a GitLab personal access token supplied per
	// request, in either the PRIVATE-TOKEN header or Authorization: Bearer.
	AuthModeLegacy = "legacy"
	// AuthModeOAuth accepts only Authorization: Bearer, verified against the
	// GitLab instance, and advertises discovery through RFC 9728.
	AuthModeOAuth = "oauth"
	// DefaultAuthMode preserves the static-token behavior for deployments
	// that set no mode at all.
	DefaultAuthMode = AuthModeLegacy
)

// Capability surface modes select which non-tool MCP capabilities are exposed.
const (
	// CapabilitySurfaceFull exposes the current resource and prompt catalog.
	CapabilitySurfaceFull = "full"
	// CapabilitySurfaceMinimal exposes only capabilities required for dynamic use.
	CapabilitySurfaceMinimal = "minimal"
	// DefaultCapabilitySurface preserves the existing resource and prompt catalog.
	DefaultCapabilitySurface = CapabilitySurfaceFull
)

// Config holds all configuration values for the MCP server.
type Config struct {
	GitLabURL string
	// GitLabURLs is the full set of instances an HTTP deployment publishes,
	// in the order they were configured; GitLabURL is its first entry.
	//
	// More than one turns the per-request GITLAB-URL header from a free
	// choice into a selection among published instances, which is what makes
	// it safe in oauth mode: the server validates the bearer token against
	// the instance it is about to use, so a caller naming a host of their own
	// would be handed the token. Empty or single-valued behaves exactly as
	// GitLabURL alone always has.
	GitLabURLs        []string
	GitLabToken       string
	SkipTLSVerify     bool
	DisableRetries    bool // Disable GitLab client retries for unit tests.
	MetaTools         bool
	ToolSurface       string
	CapabilitySurface string

	// Tier is the resolved GitLab licensing tier for this configuration.
	// When TierExplicit is false it holds the conservative default
	// (edition.Free); callers detect the real tier from the instance license.
	Tier edition.Tier
	// TierExplicit reports whether the tier was set explicitly via GITLAB_MCP_TIER
	// (stdio) or --tier (HTTP). When true, no instance license check is
	// performed and Tier is used verbatim. When false, the tier is detected
	// per instance, falling back to edition.Free.
	TierExplicit bool

	ReadOnly bool
	SafeMode bool

	EmbeddedResources bool // Append EmbeddedResource content blocks to get_* tool results (default true)

	UploadMaxFileSize int64

	MaxHTTPClients     int           // Maximum unique tokens in the server pool (HTTP mode only)
	SessionTimeout     time.Duration // Idle MCP session timeout (HTTP mode only)
	RevalidateInterval time.Duration // Token re-validation interval (HTTP mode only)
	// ActionTimeout bounds one action's handler in both transports: a call
	// still running after this long is cancelled and answered with the
	// deadline error. 0 disables the bound.
	ActionTimeout time.Duration
	// DrainDelay is how long, after SIGTERM, the HTTP listener stays open
	// answering /health with 503 draining before it closes, so a balancer
	// takes the instance out of rotation first. 0 closes at once.
	DrainDelay time.Duration
	// PoolIdleTimeout is how long a pooled per-token-and-URL server entry may go unused
	// before it is reclaimed; 0 keeps entries until the pool's size bound
	// evicts them (HTTP mode only).
	PoolIdleTimeout time.Duration

	// Stateless enables sessionless streamable HTTP (SEP-2567, protocol
	// 2026-07-28): the server neither reads nor sets Mcp-Session-Id, every
	// POST is self-contained, and GET/DELETE return 405 (HTTP mode only).
	Stateless bool
	// JSONResponse returns application/json bodies instead of
	// text/event-stream (SSE) for streamable responses (HTTP mode only).
	JSONResponse bool
	// MaxRequestBodyBytes caps the size of incoming streamable HTTP request
	// bodies. 0 uses the SDK default (4 MiB); negatives are rejected at
	// validation (HTTP mode only).
	//
	// The default stays at the SDK's 4 MiB rather than being cut to 256 KiB or
	// 1 MiB, which a hardening review proposed against the cost of parsing a
	// deeply nested body. That defense now lives where the shape of the attack
	// is, in the inbound JSON depth cap: a smaller byte budget only narrows the
	// window, since a quarter of a megabyte still spells a hundred thousand
	// levels of nesting, and a wide-but-shallow body of the full size
	// unmarshals in tens of milliseconds either way. Cutting it would break the
	// documented inline-upload path, where content_base64 is the only way a
	// remote caller can send a file at all, for every deployment that never
	// configured the flag. An operator who wants a smaller ceiling still has
	// the flag.
	MaxRequestBodyBytes int64

	// TLSCertFile and TLSKeyFile enable TLS on the listener itself, for a
	// deployment where the proxy does not share a machine with the server
	// and the hop between them crosses a network. They are set together or
	// not at all (HTTP mode only).
	TLSCertFile string
	TLSKeyFile  string
	// SocketMode is the permission bits applied to a unix socket named by
	// the listen address. 0 means the default, [DefaultSocketMode].
	SocketMode os.FileMode

	AuthMode      string        // Auth mode for HTTP: "legacy" (default) or "oauth"
	OAuthCacheTTL time.Duration // OAuth token cache TTL (HTTP mode, oauth auth mode)
	// OAuthClientUIDs pins the OAuth applications whose tokens this
	// deployment admits, by GitLab application uid. Empty — the default —
	// admits any credential the instance accepts.
	//
	// It is the only recipient check available: GitLab's authorization server
	// publishes no resource_indicators_supported, so RFC 8707 audience
	// restriction cannot be used, and the specification's "or otherwise verify
	// that they are the intended recipient" is met by comparing the
	// application a token was minted for. It is a set because --gitlab-url is
	// repeatable and every published instance has its own application.
	//
	// Off by default because turning it on refuses personal access tokens
	// outright: a PAT belongs to no application, and it is a supported
	// credential here.
	OAuthClientUIDs []string
	// PublicURL is the externally reachable origin of this deployment
	// (scheme://host[:port][/path], no trailing slash). Required in oauth
	// mode: RFC 9728 defines the protected-resource identifier as an https
	// URL, and deriving it from the bind address produces host-less or
	// wrong-origin identifiers behind any TLS-terminating proxy.
	PublicURL string

	// ResourceDocumentation is the RFC 9728 resource_documentation URL the
	// protected-resource metadata advertises. Empty means this project's
	// own OAuth setup guide. An operator running their own deployment
	// points it at a page describing their own OAuth application, which is
	// the only sanctioned way to lead a client to a client ID: RFC 9728
	// defines no field for one.
	ResourceDocumentation string

	// ResourcePolicyURI and ResourceTermsURI are the RFC 9728
	// resource_policy_uri and resource_tos_uri fields. Both are omitted from
	// the metadata document when empty, which is the default: they describe a
	// specific deployment's undertakings about the data reached with the
	// tokens it accepts, and publishing a link to a page that does not exist
	// would put a dead link on a consent screen.
	ResourcePolicyURI string
	ResourceTermsURI  string

	TrustedProxyHeader string   // HTTP header with real client IP (e.g. X-Forwarded-For, X-Real-IP)
	TrustedProxies     []string // Addresses or CIDR ranges of the proxies TrustedProxyHeader is believed from; required with it
	// TrustedOrigins are absolute origins (scheme://host[:port]) allowed to
	// make cross-origin browser requests. Empty by default: the server
	// refuses every cross-origin browser POST, and only a listed origin (or
	// the PublicURL origin, seeded automatically) is exempted. A trusted
	// origin is validation, not a bypass — the DNS-rebinding MUST is still
	// met for every origin not on the list.
	TrustedOrigins []string
	ExcludeTools   []string // Tool names to exclude from registration (comma-separated via EXCLUDE_TOOLS)
	IgnoreScopes   bool     // When true, skip PAT scope detection and register all tools

	RateLimitRPS   float64 // Per-server tools/call rate limit in requests/second (0 = disabled)
	RateLimitBurst int     // Token-bucket burst size when RateLimitRPS > 0

	// MetaParamSchema controls how meta-tool input schemas advertise the
	// shape of the `params` object. Allowed values: "opaque" (default),
	// "compact", "full". See [DefaultMetaParamSchema] and constants.
	MetaParamSchema string
}

// InstanceURLs returns the GitLab instances this configuration publishes.
//
// GitLabURLs is the full list and GitLabURL its first entry, but only the
// flag layer fills both. Every other constructor — a test, a stdio load, any
// caller that predates the list — sets GitLabURL alone, and reading the slice
// directly there yields "no instance fixed", which resolves to the public
// GitLab and sends the request somewhere nobody asked for. Deriving one from
// the other here means the two cannot disagree.
func (c *Config) InstanceURLs() []string {
	if c == nil {
		return nil
	}
	if len(c.GitLabURLs) > 0 {
		return c.GitLabURLs
	}
	if c.GitLabURL == "" {
		return nil
	}
	return []string{c.GitLabURL}
}

// Enterprise reports whether the resolved tier is an Enterprise (Premium or
// Ultimate) tier. It derives the legacy binary "enterprise" notion from the
// 3-tier model so positional gating continues to behave identically.
func (c *Config) Enterprise() bool {
	if c == nil {
		return false
	}
	return c.Tier.IsEnterprise()
}

// ServerConfig is an immutable configuration snapshot used to build one MCP
// server instance for a specific GitLab URL and credential principal.
type ServerConfig struct {
	GitLabURL         string
	MetaTools         bool
	ToolSurface       string
	CapabilitySurface string
	// Tier is the resolved GitLab licensing tier for this pool entry. When the
	// owning Config did not set the tier explicitly, the pool detects it per
	// instance before building the server.
	Tier edition.Tier
	// TierExplicit mirrors Config.TierExplicit: when true the tier is used
	// verbatim and the pool performs no per-instance license detection.
	TierExplicit bool
	ReadOnly     bool
	// ReadOnlyFromTokenScope records that ReadOnly was not asked for by the
	// operator but derived from the credential: this token cannot write, so a
	// read-only surface was built for it. The two causes need different words
	// when a withheld action is asked for — "reauthorize with a wider scope"
	// versus "this deployment does not write" — and only the first is
	// something the caller can act on.
	ReadOnlyFromTokenScope bool
	SafeMode               bool
	ExcludeTools           []string
	TokenScopes            []string
	RateLimitRPS           float64
	RateLimitBurst         int
	MetaParamSchema        string
	// Stateless mirrors Config.Stateless. It reaches the server because a
	// sessionless transport cannot carry a server-initiated notification
	// outside an open request, which decides whether the legacy
	// resources/subscribe path can be honored at all.
	Stateless bool
}

// ServerConfig returns the server-scoped subset of Config. Callers may enrich
// the returned snapshot with detected per-principal data before creating a
// concrete MCP server instance.
func (c *Config) ServerConfig() *ServerConfig {
	if c == nil {
		return &ServerConfig{}
	}
	return &ServerConfig{
		GitLabURL:         c.GitLabURL,
		MetaTools:         c.MetaTools,
		ToolSurface:       c.ToolSurface,
		CapabilitySurface: c.CapabilitySurface,
		Tier:              c.Tier,
		TierExplicit:      c.TierExplicit,
		ReadOnly:          c.ReadOnly,
		SafeMode:          c.SafeMode,
		ExcludeTools:      slices.Clone(c.ExcludeTools),
		RateLimitRPS:      c.RateLimitRPS,
		RateLimitBurst:    c.RateLimitBurst,
		MetaParamSchema:   c.MetaParamSchema,
		Stateless:         c.Stateless,
	}
}

// Enterprise reports whether this server's resolved tier is an Enterprise
// (Premium or Ultimate) tier, deriving the legacy binary notion from the tier.
func (s *ServerConfig) Enterprise() bool {
	if s == nil {
		return false
	}
	return s.Tier.IsEnterprise()
}

// DefaultGitLabURL is the GitLab instance used when GITLAB_URL is unset.
const DefaultGitLabURL = "https://gitlab.com"

// Load reads configuration from environment variables, after populating that
// environment from the dotenv files [LoadEnvFiles] accepts: the file
// [EnvFileVar] names and ~/.gitlab-mcp-server.env, in that order, neither of
// them overwriting a variable the client already passed.
func Load() (*Config, error) {
	LoadEnvFiles()

	bools, err := loadBooleanEnv()
	if err != nil {
		return nil, err
	}

	tier, tierExplicit, err := resolveTierEnv(Getenv("TIER"), os.Getenv("GITLAB_ENTERPRISE"))
	if err != nil {
		return nil, err
	}

	toolSurface, metaTools, err := ParseToolSurface(Getenv("TOOL_SURFACE"), Getenv("META_TOOLS"))
	if err != nil {
		return nil, err
	}

	capabilitySurface, err := parseCapabilitySurface(Getenv("CAPABILITY_SURFACE"), DefaultCapabilitySurface)
	if err != nil {
		return nil, fmt.Errorf("invalid CAPABILITY_SURFACE value: %w", err)
	}

	limits, err := loadLimitEnv()
	if err != nil {
		return nil, err
	}
	auth, err := loadAuthEnv()
	if err != nil {
		return nil, err
	}

	rateLimitRPS, err := parseFloatNonNegative(Getenv("RATE_LIMIT_RPS"), 0)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_RPS value: %w", err)
	}
	rateLimitBurst, err := parseIntNonNegative(Getenv("RATE_LIMIT_BURST"), DefaultRateLimitBurst)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_BURST value: %w", err)
	}

	metaParamSchema, err := parseMetaParamSchema(Getenv("META_PARAM_SCHEMA"), DefaultMetaParamSchema)
	if err != nil {
		return nil, fmt.Errorf("invalid META_PARAM_SCHEMA value: %w", err)
	}

	cfg := &Config{
		GitLabURL:          gitLabURLFromEnv(),
		GitLabToken:        os.Getenv("GITLAB_TOKEN"),
		SkipTLSVerify:      bools.skipTLS,
		MetaTools:          metaTools,
		ToolSurface:        toolSurface,
		CapabilitySurface:  capabilitySurface,
		Tier:               tier,
		TierExplicit:       tierExplicit,
		ReadOnly:           bools.readOnly,
		SafeMode:           bools.safeMode,
		EmbeddedResources:  bools.embeddedResources,
		UploadMaxFileSize:  limits.maxFileSize,
		MaxHTTPClients:     limits.maxHTTPClients,
		SessionTimeout:     limits.sessionTimeout,
		RevalidateInterval: limits.revalidateInterval,
		PoolIdleTimeout:    limits.poolIdleTimeout,
		ActionTimeout:      limits.actionTimeout,
		DrainDelay:         limits.drainDelay,
		AuthMode:           auth.mode,
		PublicURL:          auth.publicURL,
		OAuthCacheTTL:      auth.oauthCacheTTL,
		OAuthClientUIDs:    auth.oauthClientUIDs,
		ExcludeTools:       ParseCSV(Getenv("EXCLUDE_TOOLS")),
		IgnoreScopes:       bools.ignoreScopes,
		RateLimitRPS:       rateLimitRPS,
		RateLimitBurst:     rateLimitBurst,
		MetaParamSchema:    metaParamSchema,
	}

	if validateErr := cfg.validate(); validateErr != nil {
		return nil, validateErr
	}

	return cfg, nil
}

type booleanEnv struct {
	skipTLS           bool
	readOnly          bool
	safeMode          bool
	embeddedResources bool
	ignoreScopes      bool
}

type limitEnv struct {
	maxFileSize        int64
	maxHTTPClients     int
	sessionTimeout     time.Duration
	revalidateInterval time.Duration
	poolIdleTimeout    time.Duration
	actionTimeout      time.Duration
	drainDelay         time.Duration
}

type authEnv struct {
	mode            string
	oauthCacheTTL   time.Duration
	publicURL       string
	oauthClientUIDs []string
}

func loadBooleanEnv() (booleanEnv, error) {
	values := booleanEnv{}
	var err error
	if values.skipTLS, err = parseEnvBool("SKIP_TLS_VERIFY", false); err != nil {
		return booleanEnv{}, err
	}
	if values.readOnly, err = parseEnvBool("READ_ONLY", false); err != nil {
		return booleanEnv{}, err
	}
	if values.safeMode, err = parseEnvBool("SAFE_MODE", false); err != nil {
		return booleanEnv{}, err
	}
	if values.embeddedResources, err = parseEnvBool("EMBEDDED_RESOURCES", true); err != nil {
		return booleanEnv{}, err
	}
	if values.ignoreScopes, err = parseEnvBool("IGNORE_SCOPES", false); err != nil {
		return booleanEnv{}, err
	}
	return values, nil
}

// parseTierEnv resolves the GITLAB_MCP_TIER value into a tier and an "explicit"
// flag. An empty value yields (edition.Free, false) so the caller knows to
// detect the tier from the instance license. A non-empty value must be one of
// free/ce/premium/ultimate (case-insensitive); it yields (tier, true), meaning
// no license check is performed. An unrecognized value is an error.
func parseTierEnv(value string) (tier edition.Tier, explicit bool, err error) {
	if strings.TrimSpace(value) == "" {
		return edition.Free, false, nil
	}
	parsed, ok := edition.ParseTier(value)
	if !ok {
		return edition.Free, false, fmt.Errorf(
			"invalid %sTIER value: expected free, ce, premium, or ultimate, got %q", EnvPrefix, value,
		)
	}
	return parsed, true, nil
}

// resolveTierEnv resolves the effective tier from GITLAB_MCP_TIER (or its old
// spelling GITLAB_MCP_TIER) and the DEPRECATED GITLAB_ENTERPRISE env vars. The tier
// wins. When it is unset, the deprecated GITLAB_ENTERPRISE is honored for
// back-compat with existing configs: true → ultimate, false → free (both
// explicit, so no license check). When neither is set, returns
// (edition.Free, false) so the caller detects from the license.
func resolveTierEnv(tierValue, enterpriseValue string) (tier edition.Tier, explicit bool, err error) {
	tier, explicit, err = parseTierEnv(tierValue)
	if err != nil || explicit {
		return tier, explicit, err
	}
	if raw := strings.TrimSpace(enterpriseValue); raw != "" {
		enabled, perr := strconv.ParseBool(raw)
		if perr != nil {
			return edition.Free, false, fmt.Errorf(
				"invalid GITLAB_ENTERPRISE value: expected true or false, got %q", raw,
			)
		}
		if enabled {
			return edition.Ultimate, true, nil
		}
		return edition.Free, true, nil
	}
	return edition.Free, false, nil
}

// LegacyEnterpriseEnvInUse reports whether the DEPRECATED GITLAB_ENTERPRISE env
// var is the active tier source (the tier unset, GITLAB_ENTERPRISE set), so the
// caller can emit a one-time deprecation warning pointing users to
// GITLAB_MCP_TIER.
func LegacyEnterpriseEnvInUse(tierValue, enterpriseValue string) bool {
	return strings.TrimSpace(tierValue) == "" && strings.TrimSpace(enterpriseValue) != ""
}

// ParseTierFlag resolves a CLI --tier flag value into a tier and an "explicit"
// flag, mirroring [parseTierEnv]. It is exported for cmd/server HTTP-mode flag
// handling.
func ParseTierFlag(value string) (tier edition.Tier, explicit bool, err error) {
	return parseTierEnv(value)
}

func parseEnvBool(name string, defaultValue bool) (bool, error) {
	value, err := parseBool(Getenv(name), defaultValue)
	if err != nil {
		return false, fmt.Errorf("invalid %s%s value: %w", EnvPrefix, name, err)
	}
	return value, nil
}

func loadLimitEnv() (limitEnv, error) {
	values := limitEnv{}
	var err error
	if values.maxFileSize, err = UploadMaxFileSizeFromEnv(); err != nil {
		return limitEnv{}, fmt.Errorf("invalid UPLOAD_MAX_FILE_SIZE value: %w", err)
	}
	if values.maxHTTPClients, err = parseInt(Getenv("MAX_HTTP_CLIENTS"), DefaultMaxHTTPClients); err != nil {
		return limitEnv{}, fmt.Errorf("invalid MAX_HTTP_CLIENTS value: %w", err)
	}
	if values.sessionTimeout, err = parseBoundedDurationEnv("SESSION_TIMEOUT", DefaultSessionTimeout, MaxSessionTimeout); err != nil {
		return limitEnv{}, err
	}
	if values.poolIdleTimeout, err = parseDisableableDurationEnv("POOL_IDLE_TIMEOUT", DefaultPoolIdleTimeout, MaxPoolIdleTimeout); err != nil {
		return limitEnv{}, err
	}
	if values.revalidateInterval, err = parseDisableableDurationEnv("SESSION_REVALIDATE_INTERVAL", DefaultRevalidateInterval, MaxRevalidateInterval); err != nil {
		return limitEnv{}, err
	}
	if values.actionTimeout, err = parseDisableableDurationEnv("ACTION_TIMEOUT", DefaultActionTimeout, MaxActionTimeout); err != nil {
		return limitEnv{}, err
	}
	if values.drainDelay, err = parseDisableableDurationEnv("DRAIN_DELAY", DefaultDrainDelay, MaxDrainDelay); err != nil {
		return limitEnv{}, err
	}
	return values, nil
}

// parseDisableableDurationEnv parses a bounded duration whose zero value is a
// meaningful setting rather than an error: both POOL_IDLE_TIMEOUT and
// SESSION_REVALIDATE_INTERVAL document "0" as "turn this off", and their
// consumers already treat a non-positive value that way. The shared
// [parseDuration] rejects "0" outright, so routing these through
// [parseBoundedDurationEnv] made a documented setting fail at startup.
func parseDisableableDurationEnv(name string, defaultValue, maxValue time.Duration) (time.Duration, error) {
	if TrimmedGetenv(name) == "0" {
		return 0, nil
	}
	return parseBoundedDurationEnv(name, defaultValue, maxValue)
}

func parseBoundedDurationEnv(name string, defaultValue, maxValue time.Duration) (time.Duration, error) {
	value, err := parseDuration(Getenv(name), defaultValue)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value: %w", name, err)
	}
	if value > maxValue {
		return 0, fmt.Errorf("%s %s exceeds maximum of %s", name, value, maxValue)
	}
	return value, nil
}

func loadAuthEnv() (authEnv, error) {
	mode := Getenv("AUTH_MODE")
	if mode == "" {
		mode = "legacy"
	}
	oauthCacheTTL, err := parseDuration(Getenv("OAUTH_CACHE_TTL"), DefaultOAuthCacheTTL)
	if err != nil {
		return authEnv{}, fmt.Errorf("invalid OAUTH_CACHE_TTL value: %w", err)
	}
	return authEnv{
		mode:            mode,
		oauthCacheTTL:   oauthCacheTTL,
		publicURL:       TrimmedGetenv("PUBLIC_URL"),
		oauthClientUIDs: ParseCSV(Getenv("OAUTH_CLIENT_UID")),
	}, nil
}

func gitLabURLFromEnv() string {
	gitLabURL := strings.TrimSpace(os.Getenv("GITLAB_URL"))
	if gitLabURL == "" {
		return DefaultGitLabURL
	}
	return gitLabURL
}

// validate checks that all required configuration fields are present and valid.
func (c *Config) validate() error {
	if err := c.validateURLAndToken(); err != nil {
		return err
	}
	if err := c.validateLimits(); err != nil {
		return err
	}
	if err := c.validateModeEnums(); err != nil {
		return err
	}
	return c.validateDurationsAndRates()
}

func (c *Config) validateURLAndToken() error {
	if c.GitLabURL == "" {
		return errors.New("GITLAB_URL cannot be empty")
	}
	u, err := url.Parse(c.GitLabURL)
	if err != nil {
		return fmt.Errorf("GITLAB_URL is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("GITLAB_URL must use http:// or https:// scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("GITLAB_URL must include a host")
	}
	if c.GitLabToken == "" {
		return errors.New("GITLAB_TOKEN is required")
	}
	return nil
}

func (c *Config) validateLimits() error {
	if c.UploadMaxFileSize > MaxFileSize {
		return fmt.Errorf("UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB (got %d bytes)", c.UploadMaxFileSize)
	}
	if c.MaxHTTPClients <= 0 {
		return fmt.Errorf("MAX_HTTP_CLIENTS must be positive (got %d)", c.MaxHTTPClients)
	}
	if c.MaxHTTPClients > MaxHTTPClients {
		return fmt.Errorf("MAX_HTTP_CLIENTS exceeds maximum of %d (got %d)", MaxHTTPClients, c.MaxHTTPClients)
	}
	return nil
}

func (c *Config) validateModeEnums() error {
	if c.AuthMode != "" && c.AuthMode != AuthModeLegacy && c.AuthMode != AuthModeOAuth {
		return fmt.Errorf("AUTH_MODE must be 'legacy' or 'oauth' (got %q)", c.AuthMode)
	}
	if c.AuthMode == AuthModeOAuth {
		if err := validatePublicURL(c.PublicURL); err != nil {
			return err
		}
	}
	if err := validateToolSurface(c.ToolSurface); err != nil {
		return err
	}
	return validateCapabilitySurface(c.CapabilitySurface)
}

func validateToolSurface(toolSurface string) error {
	if toolSurface == "" {
		return nil
	}
	switch toolSurface {
	case ToolSurfaceMeta, ToolSurfaceIndividual, ToolSurfaceDynamic:
		return nil
	default:
		return fmt.Errorf("TOOL_SURFACE must be one of %s (got %q)", validToolSurfaceList(), toolSurface)
	}
}

func validateCapabilitySurface(capabilitySurface string) error {
	if capabilitySurface == "" {
		return nil
	}
	switch capabilitySurface {
	case CapabilitySurfaceFull, CapabilitySurfaceMinimal:
		return nil
	default:
		return fmt.Errorf("CAPABILITY_SURFACE must be %q or %q (got %q)", CapabilitySurfaceFull, CapabilitySurfaceMinimal, capabilitySurface)
	}
}

func (c *Config) validateDurationsAndRates() error {
	if err := validateDurationRange("OAUTH_CACHE_TTL", c.OAuthCacheTTL, MinOAuthCacheTTL, MaxOAuthCacheTTL); err != nil {
		return err
	}
	if c.RateLimitRPS < 0 {
		return fmt.Errorf("RATE_LIMIT_RPS must be >= 0 (got %g)", c.RateLimitRPS)
	}
	if c.RateLimitRPS > MaxRateLimitRPS {
		return fmt.Errorf("RATE_LIMIT_RPS exceeds maximum of %g (got %g)", float64(MaxRateLimitRPS), c.RateLimitRPS)
	}
	if c.RateLimitRPS > 0 && c.RateLimitBurst < 1 {
		return fmt.Errorf("RATE_LIMIT_BURST must be >= 1 when RATE_LIMIT_RPS > 0 (got %d)", c.RateLimitBurst)
	}
	if c.RateLimitBurst > MaxRateLimitBurst {
		return fmt.Errorf("RATE_LIMIT_BURST exceeds maximum of %d (got %d)", MaxRateLimitBurst, c.RateLimitBurst)
	}
	return nil
}

func validateDurationRange(name string, value, minValue, maxValue time.Duration) error {
	if value == 0 {
		return nil
	}
	if value < minValue {
		return fmt.Errorf("%s %s is below minimum of %s", name, value, minValue)
	}
	if value > maxValue {
		return fmt.Errorf("%s %s exceeds maximum of %s", name, value, maxValue)
	}
	return nil
}

// parseBool parses a string as a boolean, returning defaultValue when s is empty.
// Returns an error if s is non-empty and not a valid boolean representation.
func parseBool(s string, defaultValue bool) (bool, error) {
	if s == "" {
		return defaultValue, nil
	}
	return strconv.ParseBool(s)
}

// EffectiveToolSurface returns the canonical tool surface for legacy and new
// configuration snapshots. Empty ToolSurface values are derived from MetaTools
// so older tests and callers keep their current behavior.
func EffectiveToolSurface(metaTools bool, toolSurface string) string {
	switch toolSurface {
	case ToolSurfaceMeta, ToolSurfaceIndividual, ToolSurfaceDynamic:
		return toolSurface
	}
	if metaTools {
		return ToolSurfaceMeta
	}
	return ToolSurfaceIndividual
}

// EffectiveCapabilitySurface returns the canonical capability surface.
func EffectiveCapabilitySurface(capabilitySurface string) string {
	switch capabilitySurface {
	case CapabilitySurfaceFull, CapabilitySurfaceMinimal:
		return capabilitySurface
	default:
		return DefaultCapabilitySurface
	}
}

// ParseToolSurface resolves the explicit TOOL_SURFACE value and legacy
// META_TOOLS value into a canonical tool surface and compatible MetaTools bool.
func ParseToolSurface(toolSurfaceValue, metaToolsValue string) (mode string, metaTools bool, err error) {
	if strings.TrimSpace(toolSurfaceValue) != "" {
		resolvedMode, parseErr := parseToolSurfaceValue(toolSurfaceValue, "TOOL_SURFACE")
		if parseErr != nil {
			return "", false, parseErr
		}
		// MetaTools keeps its legacy meaning for callers that only need to know
		// whether the selected surface is not the individual-tool catalog.
		return resolvedMode, resolvedMode != ToolSurfaceIndividual, nil
	}

	if strings.TrimSpace(metaToolsValue) == "" {
		return DefaultToolSurface, true, nil
	}
	resolvedMode, parseErr := parseToolSurfaceValue(metaToolsValue, "META_TOOLS")
	if parseErr != nil {
		return "", false, parseErr
	}
	return resolvedMode, resolvedMode != ToolSurfaceIndividual, nil
}

// LegacyMetaToolsSelectorInUse reports whether a configuration relies on the
// deprecated META_TOOLS selector instead of the canonical TOOL_SURFACE selector.
func LegacyMetaToolsSelectorInUse(toolSurfaceValue, metaToolsValue string) bool {
	return strings.TrimSpace(toolSurfaceValue) == "" && strings.TrimSpace(metaToolsValue) != ""
}

// LegacyMetaToolsReplacement returns the canonical TOOL_SURFACE value that
// corresponds to a legacy META_TOOLS value. It returns an empty string when the
// legacy value is invalid.
func LegacyMetaToolsReplacement(metaToolsValue string) string {
	mode, err := parseToolSurfaceValue(metaToolsValue, "META_TOOLS")
	if err != nil {
		return ""
	}
	return mode
}

func parseToolSurfaceValue(value, name string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "true", "t", "1", "yes", "y", ToolSurfaceMeta, "meta-tools", "metatools":
		return ToolSurfaceMeta, nil
	case "false", "f", "0", "no", "n", ToolSurfaceIndividual, "individual-tools", "tools":
		return ToolSurfaceIndividual, nil
	case ToolSurfaceDynamic, "dynamic-tools", "low-token":
		return ToolSurfaceDynamic, nil
	default:
		return "", fmt.Errorf("invalid %s value: expected true, false, or one of %s, got %q", name, validToolSurfaceList(), value)
	}
}

func validToolSurfaceList() string {
	return fmt.Sprintf("%q, %q, or %q", ToolSurfaceMeta, ToolSurfaceIndividual, ToolSurfaceDynamic)
}

func parseCapabilitySurface(s, defaultValue string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return defaultValue, nil
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case CapabilitySurfaceFull, "default":
		return CapabilitySurfaceFull, nil
	case CapabilitySurfaceMinimal, "minimum", "low-token":
		return CapabilitySurfaceMinimal, nil
	default:
		return "", fmt.Errorf("expected %q or %q, got %q", CapabilitySurfaceFull, CapabilitySurfaceMinimal, s)
	}
}

// parseMetaParamSchema validates the META_PARAM_SCHEMA setting. It accepts
// "opaque", "compact" or "full" (case-insensitive). Returns defaultValue when
// s is empty and an error when s is non-empty and unrecognized.
func parseMetaParamSchema(s, defaultValue string) (string, error) {
	if s == "" {
		return defaultValue, nil
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case MetaParamSchemaOpaque:
		return MetaParamSchemaOpaque, nil
	case MetaParamSchemaCompact:
		return MetaParamSchemaCompact, nil
	case MetaParamSchemaFull:
		return MetaParamSchemaFull, nil
	default:
		return "", fmt.Errorf("expected one of %q, %q, %q, got %q",
			MetaParamSchemaOpaque, MetaParamSchemaCompact, MetaParamSchemaFull, s)
	}
}

// parseSize parses a human-friendly size string (e.g. "50MB", "10mb", "2GB",
// "1024") into bytes. Supported suffixes: KB, MB, GB (case-insensitive).
// Returns defaultValue when s is empty.
func parseSize(s string, defaultValue int64) (int64, error) {
	if s == "" {
		return defaultValue, nil
	}

	upper := strings.TrimSpace(strings.ToUpper(s))

	multiplier := int64(1)
	numStr := upper

	switch {
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(upper, "GB")
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(upper, "MB")
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1024
		numStr = strings.TrimSuffix(upper, "KB")
	}

	numStr = strings.TrimSpace(numStr)
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive, got %q", s)
	}

	return n * multiplier, nil
}

// parseInt parses a string as an integer, returning defaultValue when s is empty.
func parseInt(s string, defaultValue int) (int, error) {
	if s == "" {
		return defaultValue, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("value must be positive, got %d", n)
	}
	return n, nil
}

// parseIntNonNegative parses an integer where 0 is permitted (useful for
// "disabled by default" knobs). Returns defaultValue when s is empty.
func parseIntNonNegative(s string, defaultValue int) (int, error) {
	if s == "" {
		return defaultValue, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("value must be >= 0, got %d", n)
	}
	return n, nil
}

// parseFloatNonNegative parses a non-negative float, returning defaultValue
// when s is empty. Used for rate-per-second knobs where 0 disables the
// feature.
func parseFloatNonNegative(s string, defaultValue float64) (float64, error) {
	if s == "" {
		return defaultValue, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float %q: %w", s, err)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("value must be a finite number, got %g", f)
	}
	if f < 0 {
		return 0, fmt.Errorf("value must be >= 0, got %g", f)
	}
	return f, nil
}

// parseDuration parses a string as a [time.Duration], returning defaultValue when s is empty.
func parseDuration(s string, defaultValue time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %s", d)
	}
	return d, nil
}

// ParseCSV splits a comma-separated string into trimmed, non-empty tokens.
func ParseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isLoopbackHost reports whether a hostname names this machine.
//
// Three separate rules draw an exemption here, and they must draw it in the
// same place: cleartext is tolerated for --gitlab-url and --public-url, and
// --skip-tls-verify is tolerated for --gitlab-url. All three are the same
// judgement, that a credential traveling without a verified peer does not
// leave the host.
func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// IsLoopbackGitLabURL reports whether a GitLab base URL names this machine.
func IsLoopbackGitLabURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// ValidateOAuthGitLabURL requires the GitLab instance URL to use https in
// oauth mode. Bearer tokens are forwarded upstream on every call, so a
// cleartext instance URL would put a live credential on the wire
// (CWE-319). http is tolerated only for loopback hosts, matching the
// exemption ValidatePublicURL makes for local development.
func ValidateOAuthGitLabURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("--gitlab-url %q is not an absolute URL", raw)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("--auth-mode=oauth requires an https --gitlab-url (got %q): bearer tokens are forwarded to the instance on every call, and http would transmit them in cleartext. http is allowed only for loopback development", raw)
	}
	return nil
}

// ValidateMetadataURL checks a URL this deployment publishes in its
// protected-resource metadata.
//
// These are links a consent screen or a directory follows, so a value that is
// not an absolute https URL produces a document clients reject or render with a
// link nobody can use — and it fails at that point rather than at startup,
// where an operator would see it. The flags say https; this is what makes that
// true.
//
// http is allowed on a loopback host, matching --public-url, so a developer can
// point these at a local page while trying things out.
func ValidateMetadataURL(flag, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	// Hostname rather than Host: "https://:443/x" has an authority and no host
	// in it, and publishing that is a link no client can follow.
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("%s %q is not an absolute URL", flag, raw)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("%s %q must be an https URL (http is accepted only for loopback hosts)", flag, raw)
	}
	return nil
}

// ValidatePublicURL enforces the RFC 9728 constraints on the advertised
// protected-resource identifier. Exported for the HTTP flag path, which
// assembles its Config directly instead of going through Load.
func ValidatePublicURL(raw string) error {
	return validatePublicURL(raw)
}

// validatePublicURL enforces the RFC 9728 constraints on the advertised
// protected-resource identifier: present, absolute, https (http only for
// loopback development hosts), no fragment, no trailing slash.
func validatePublicURL(raw string) error {
	if raw == "" {
		return errors.New("oauth mode requires --public-url: the RFC 9728 resource identifier must be the externally reachable https origin, which cannot be derived from the bind address")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("--public-url %q is not an absolute URL", raw)
	}
	if u.Fragment != "" {
		return fmt.Errorf("--public-url %q must not carry a fragment (RFC 9728 section 1.2)", raw)
	}
	if strings.HasSuffix(u.Path, "/") {
		return fmt.Errorf("--public-url %q must not end in a slash", raw)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("--public-url %q must use https (RFC 9728 section 1.2; http is allowed only for loopback development)", raw)
	}
	return nil
}

// UploadMaxFileSizeFromEnv resolves the upload limit from the environment, or
// returns the default when it is unset.
//
// Exported because HTTP mode builds its configuration from flags rather than
// through Load, and the flag for this setting works by writing the environment
// variable. Without one function both paths call, the limit applied depended on
// the transport: raised on stdio, ignored on HTTP.
func UploadMaxFileSizeFromEnv() (int64, error) {
	return parseSize(Getenv("UPLOAD_MAX_FILE_SIZE"), DefaultMaxFileSize)
}
