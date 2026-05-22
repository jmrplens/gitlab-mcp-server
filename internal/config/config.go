// Package config loads, normalizes, and validates runtime configuration for the
// GitLab MCP server.
//
// Configuration comes from environment variables, .env files, and CLI flags in
// cmd/server. The package centralizes defaults and bounds for stdio mode, HTTP
// mode, OAuth token verification, auto-update behavior, upload limits, safe
// mode, read-only mode, rate limiting, tool surfaces, capability surfaces, and
// meta-tool schema detail.
//
// # Validation Model
//
// Loaders keep user-facing configuration forgiving while preserving hard bounds
// that protect runtime behavior: URL values are parsed and normalized, duration
// and size fields are clamped to documented limits, and invalid enum values are
// reported before server registration begins.
package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
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
	MaxHTTPClients            = 10000
	MaxSessionTimeout         = 24 * time.Hour
	MaxRevalidateInterval     = 24 * time.Hour
)

// OAuth defaults.
const (
	DefaultOAuthCacheTTL = 15 * time.Minute
	MinOAuthCacheTTL     = 1 * time.Minute
	MaxOAuthCacheTTL     = 2 * time.Hour
)

// Auto-update defaults.
const (
	DefaultAutoUpdateRepo     = "jmrplens/gitlab-mcp-server"
	DefaultAutoUpdateInterval = 1 * time.Hour
	DefaultAutoUpdateTimeout  = 60 * time.Second
	MinAutoUpdateTimeout      = 5 * time.Second
	MaxAutoUpdateTimeout      = 10 * time.Minute
)

// DefaultRateLimitBurst is the bucket size used when rps > 0 and the operator
// did not set RATE_LIMIT_BURST explicitly.
const (
	DefaultRateLimitBurst = 40
	MaxRateLimitRPS       = 1000
	MaxRateLimitBurst     = 10000
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
	GitLabURL            string
	GitLabToken          string
	SkipTLSVerify        bool
	DisableRetries       bool // Disable GitLab client retries for unit tests.
	MetaTools            bool
	ToolSurface          string
	CapabilitySurface    string
	Enterprise           bool
	AutoDetectEnterprise bool
	ReadOnly             bool
	SafeMode             bool

	EmbeddedResources bool // Append EmbeddedResource content blocks to get_* tool results (default true)

	UploadMaxFileSize int64

	MaxHTTPClients     int           // Maximum unique tokens in the server pool (HTTP mode only)
	SessionTimeout     time.Duration // Idle MCP session timeout (HTTP mode only)
	RevalidateInterval time.Duration // Token re-validation interval (HTTP mode only)

	AutoUpdate         string        // Auto-update mode: "true" (auto), "check" (log-only), "false" (disabled)
	AutoUpdateRepo     string        // GitLab project path for update checks
	AutoUpdateInterval time.Duration // How often to check for updates (HTTP mode)
	AutoUpdateTimeout  time.Duration // Timeout for startup/background update checks

	AuthMode      string        // Auth mode for HTTP: "legacy" (default) or "oauth"
	OAuthCacheTTL time.Duration // OAuth token cache TTL (HTTP mode, oauth auth mode)

	TrustedProxyHeader string   // HTTP header with real client IP (e.g. X-Forwarded-For, X-Real-IP)
	ExcludeTools       []string // Tool names to exclude from registration (comma-separated via EXCLUDE_TOOLS)
	IgnoreScopes       bool     // When true, skip PAT scope detection and register all tools

	RateLimitRPS   float64 // Per-server tools/call rate limit in requests/second (0 = disabled)
	RateLimitBurst int     // Token-bucket burst size when RateLimitRPS > 0

	// MetaParamSchema controls how meta-tool input schemas advertise the
	// shape of the `params` object. Allowed values: "opaque" (default),
	// "compact", "full". See [DefaultMetaParamSchema] and constants.
	MetaParamSchema string
}

// ServerConfig is an immutable configuration snapshot used to build one MCP
// server instance for a specific GitLab URL and credential principal.
type ServerConfig struct {
	GitLabURL         string
	MetaTools         bool
	ToolSurface       string
	CapabilitySurface string
	Enterprise        bool
	ReadOnly          bool
	SafeMode          bool
	ExcludeTools      []string
	TokenScopes       []string
	RateLimitRPS      float64
	RateLimitBurst    int
	MetaParamSchema   string
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
		Enterprise:        c.Enterprise,
		ReadOnly:          c.ReadOnly,
		SafeMode:          c.SafeMode,
		ExcludeTools:      slices.Clone(c.ExcludeTools),
		RateLimitRPS:      c.RateLimitRPS,
		RateLimitBurst:    c.RateLimitBurst,
		MetaParamSchema:   c.MetaParamSchema,
	}
}

// EnvFileName is the name of the env file where the setup wizard stores secrets.
const EnvFileName = ".gitlab-mcp-server.env"

// DefaultGitLabURL is the GitLab instance used when GITLAB_URL is unset.
const DefaultGitLabURL = "https://gitlab.com"

// Load reads configuration from environment variables.
// It attempts to load a .env file from the current directory first, then
// falls back to ~/.gitlab-mcp-server.env (written by the setup wizard) for
// secrets not provided via the environment or CWD .env.
func Load() (*Config, error) {
	_ = godotenv.Load()

	// Fallback: load secrets from the wizard-generated env file in $HOME.
	// godotenv does not overwrite variables already set, so explicit env
	// vars and CWD .env values take precedence.
	if home, err := os.UserHomeDir(); err == nil {
		_ = godotenv.Load(filepath.Join(home, EnvFileName))
	}

	bools, err := loadBooleanEnv()
	if err != nil {
		return nil, err
	}

	toolSurface, metaTools, err := ParseToolSurface(os.Getenv("TOOL_SURFACE"), os.Getenv("META_TOOLS"))
	if err != nil {
		return nil, err
	}

	capabilitySurface, err := parseCapabilitySurface(os.Getenv("CAPABILITY_SURFACE"), DefaultCapabilitySurface)
	if err != nil {
		return nil, fmt.Errorf("invalid CAPABILITY_SURFACE value: %w", err)
	}

	limits, err := loadLimitEnv()
	if err != nil {
		return nil, err
	}

	updates, err := loadAutoUpdateEnv()
	if err != nil {
		return nil, err
	}
	auth, err := loadAuthEnv()
	if err != nil {
		return nil, err
	}

	rateLimitRPS, err := parseFloatNonNegative(os.Getenv("RATE_LIMIT_RPS"), 0)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_RPS value: %w", err)
	}
	rateLimitBurst, err := parseIntNonNegative(os.Getenv("RATE_LIMIT_BURST"), DefaultRateLimitBurst)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_BURST value: %w", err)
	}

	metaParamSchema, err := parseMetaParamSchema(os.Getenv("META_PARAM_SCHEMA"), DefaultMetaParamSchema)
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
		Enterprise:         bools.enterprise,
		ReadOnly:           bools.readOnly,
		SafeMode:           bools.safeMode,
		EmbeddedResources:  bools.embeddedResources,
		UploadMaxFileSize:  limits.maxFileSize,
		MaxHTTPClients:     limits.maxHTTPClients,
		SessionTimeout:     limits.sessionTimeout,
		RevalidateInterval: limits.revalidateInterval,
		AutoUpdate:         updates.mode,
		AutoUpdateRepo:     updates.repo,
		AutoUpdateInterval: updates.interval,
		AutoUpdateTimeout:  updates.timeout,
		AuthMode:           auth.mode,
		OAuthCacheTTL:      auth.oauthCacheTTL,
		ExcludeTools:       ParseCSV(os.Getenv("EXCLUDE_TOOLS")),
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
	enterprise        bool
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
}

type autoUpdateEnv struct {
	mode     string
	repo     string
	interval time.Duration
	timeout  time.Duration
}

type authEnv struct {
	mode          string
	oauthCacheTTL time.Duration
}

func loadBooleanEnv() (booleanEnv, error) {
	values := booleanEnv{}
	var err error
	if values.skipTLS, err = parseEnvBool("GITLAB_SKIP_TLS_VERIFY", false); err != nil {
		return booleanEnv{}, err
	}
	if values.enterprise, err = parseEnvBool("GITLAB_ENTERPRISE", false); err != nil {
		return booleanEnv{}, err
	}
	if values.readOnly, err = parseEnvBool("GITLAB_READ_ONLY", false); err != nil {
		return booleanEnv{}, err
	}
	if values.safeMode, err = parseEnvBool("GITLAB_SAFE_MODE", false); err != nil {
		return booleanEnv{}, err
	}
	if values.embeddedResources, err = parseEnvBool("EMBEDDED_RESOURCES", true); err != nil {
		return booleanEnv{}, err
	}
	if values.ignoreScopes, err = parseEnvBool("GITLAB_IGNORE_SCOPES", false); err != nil {
		return booleanEnv{}, err
	}
	return values, nil
}

func parseEnvBool(name string, defaultValue bool) (bool, error) {
	value, err := parseBool(os.Getenv(name), defaultValue)
	if err != nil {
		return false, fmt.Errorf("invalid %s value: %w", name, err)
	}
	return value, nil
}

func loadLimitEnv() (limitEnv, error) {
	values := limitEnv{}
	var err error
	if values.maxFileSize, err = parseSize(os.Getenv("UPLOAD_MAX_FILE_SIZE"), DefaultMaxFileSize); err != nil {
		return limitEnv{}, fmt.Errorf("invalid UPLOAD_MAX_FILE_SIZE value: %w", err)
	}
	if values.maxHTTPClients, err = parseInt(os.Getenv("MAX_HTTP_CLIENTS"), DefaultMaxHTTPClients); err != nil {
		return limitEnv{}, fmt.Errorf("invalid MAX_HTTP_CLIENTS value: %w", err)
	}
	if values.sessionTimeout, err = parseBoundedDurationEnv("SESSION_TIMEOUT", DefaultSessionTimeout, MaxSessionTimeout); err != nil {
		return limitEnv{}, err
	}
	if values.revalidateInterval, err = parseBoundedDurationEnv("SESSION_REVALIDATE_INTERVAL", DefaultRevalidateInterval, MaxRevalidateInterval); err != nil {
		return limitEnv{}, err
	}
	return values, nil
}

func parseBoundedDurationEnv(name string, defaultValue, maxValue time.Duration) (time.Duration, error) {
	value, err := parseDuration(os.Getenv(name), defaultValue)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value: %w", name, err)
	}
	if value > maxValue {
		return 0, fmt.Errorf("%s %s exceeds maximum of %s", name, value, maxValue)
	}
	return value, nil
}

func loadAutoUpdateEnv() (autoUpdateEnv, error) {
	values := autoUpdateEnv{mode: os.Getenv("AUTO_UPDATE"), repo: os.Getenv("AUTO_UPDATE_REPO")}
	if values.mode == "" {
		values.mode = "true"
	}
	if values.repo == "" {
		values.repo = DefaultAutoUpdateRepo
	}
	var err error
	if values.interval, err = parseDuration(os.Getenv("AUTO_UPDATE_INTERVAL"), DefaultAutoUpdateInterval); err != nil {
		return autoUpdateEnv{}, fmt.Errorf("invalid AUTO_UPDATE_INTERVAL value: %w", err)
	}
	if values.timeout, err = parseDuration(os.Getenv("AUTO_UPDATE_TIMEOUT"), DefaultAutoUpdateTimeout); err != nil {
		return autoUpdateEnv{}, fmt.Errorf("invalid AUTO_UPDATE_TIMEOUT value: %w", err)
	}
	return values, nil
}

func loadAuthEnv() (authEnv, error) {
	mode := os.Getenv("AUTH_MODE")
	if mode == "" {
		mode = "legacy"
	}
	oauthCacheTTL, err := parseDuration(os.Getenv("OAUTH_CACHE_TTL"), DefaultOAuthCacheTTL)
	if err != nil {
		return authEnv{}, fmt.Errorf("invalid OAUTH_CACHE_TTL value: %w", err)
	}
	return authEnv{mode: mode, oauthCacheTTL: oauthCacheTTL}, nil
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
	if c.AuthMode != "" && c.AuthMode != "legacy" && c.AuthMode != "oauth" {
		return fmt.Errorf("AUTH_MODE must be 'legacy' or 'oauth' (got %q)", c.AuthMode)
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
	if err := validateDurationRange("AUTO_UPDATE_TIMEOUT", c.AutoUpdateTimeout, MinAutoUpdateTimeout, MaxAutoUpdateTimeout); err != nil {
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
