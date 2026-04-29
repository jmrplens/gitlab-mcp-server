// Package config loads and validates server configuration from environment variables.
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

// Upload size defaults.
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

// Rate-limit defaults.
const (
	// DefaultRateLimitBurst is the bucket size used when rps > 0 and the
	// operator did not set RATE_LIMIT_BURST explicitly.
	DefaultRateLimitBurst = 40
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

// Config holds all configuration values for the MCP server.
type Config struct {
	GitLabURL            string
	GitLabToken          string
	SkipTLSVerify        bool
	MetaTools            bool
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
	AutoUpdateTimeout  time.Duration // Timeout for pre-start update check (stdio mode)

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
	GitLabURL       string
	MetaTools       bool
	Enterprise      bool
	ReadOnly        bool
	SafeMode        bool
	ExcludeTools    []string
	TokenScopes     []string
	RateLimitRPS    float64
	RateLimitBurst  int
	MetaParamSchema string
}

// ServerConfig returns the server-scoped subset of Config. Callers may enrich
// the returned snapshot with detected per-principal data before creating a
// concrete MCP server instance.
func (c *Config) ServerConfig() *ServerConfig {
	if c == nil {
		return &ServerConfig{}
	}
	return &ServerConfig{
		GitLabURL:       c.GitLabURL,
		MetaTools:       c.MetaTools,
		Enterprise:      c.Enterprise,
		ReadOnly:        c.ReadOnly,
		SafeMode:        c.SafeMode,
		ExcludeTools:    slices.Clone(c.ExcludeTools),
		RateLimitRPS:    c.RateLimitRPS,
		RateLimitBurst:  c.RateLimitBurst,
		MetaParamSchema: c.MetaParamSchema,
	}
}

// EnvFileName is the name of the env file where the setup wizard stores secrets.
const EnvFileName = ".gitlab-mcp-server.env"

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

	skipTLS, err := parseBool(os.Getenv("GITLAB_SKIP_TLS_VERIFY"), false)
	if err != nil {
		return nil, fmt.Errorf("invalid GITLAB_SKIP_TLS_VERIFY value: %w", err)
	}

	metaTools, err := parseBool(os.Getenv("META_TOOLS"), true)
	if err != nil {
		return nil, fmt.Errorf("invalid META_TOOLS value: %w", err)
	}

	enterprise, err := parseBool(os.Getenv("GITLAB_ENTERPRISE"), false)
	if err != nil {
		return nil, fmt.Errorf("invalid GITLAB_ENTERPRISE value: %w", err)
	}

	readOnly, err := parseBool(os.Getenv("GITLAB_READ_ONLY"), false)
	if err != nil {
		return nil, fmt.Errorf("invalid GITLAB_READ_ONLY value: %w", err)
	}

	safeMode, err := parseBool(os.Getenv("GITLAB_SAFE_MODE"), false)
	if err != nil {
		return nil, fmt.Errorf("invalid GITLAB_SAFE_MODE value: %w", err)
	}

	embeddedResources, err := parseBool(os.Getenv("EMBEDDED_RESOURCES"), true)
	if err != nil {
		return nil, fmt.Errorf("invalid EMBEDDED_RESOURCES value: %w", err)
	}

	ignoreScopes, err := parseBool(os.Getenv("GITLAB_IGNORE_SCOPES"), false)
	if err != nil {
		return nil, fmt.Errorf("invalid GITLAB_IGNORE_SCOPES value: %w", err)
	}

	maxFileSize, err := parseSize(os.Getenv("UPLOAD_MAX_FILE_SIZE"), DefaultMaxFileSize)
	if err != nil {
		return nil, fmt.Errorf("invalid UPLOAD_MAX_FILE_SIZE value: %w", err)
	}

	maxHTTPClients, err := parseInt(os.Getenv("MAX_HTTP_CLIENTS"), DefaultMaxHTTPClients)
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_HTTP_CLIENTS value: %w", err)
	}

	sessionTimeout, err := parseDuration(os.Getenv("SESSION_TIMEOUT"), DefaultSessionTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_TIMEOUT value: %w", err)
	}
	if sessionTimeout > MaxSessionTimeout {
		return nil, fmt.Errorf("SESSION_TIMEOUT %s exceeds maximum of %s", sessionTimeout, MaxSessionTimeout)
	}

	revalidateInterval, err := parseDuration(os.Getenv("SESSION_REVALIDATE_INTERVAL"), DefaultRevalidateInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_REVALIDATE_INTERVAL value: %w", err)
	}
	if revalidateInterval > MaxRevalidateInterval {
		return nil, fmt.Errorf("SESSION_REVALIDATE_INTERVAL %s exceeds maximum of %s", revalidateInterval, MaxRevalidateInterval)
	}

	autoUpdateRepo := os.Getenv("AUTO_UPDATE_REPO")
	if autoUpdateRepo == "" {
		autoUpdateRepo = DefaultAutoUpdateRepo
	}

	autoUpdateInterval, err := parseDuration(os.Getenv("AUTO_UPDATE_INTERVAL"), DefaultAutoUpdateInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid AUTO_UPDATE_INTERVAL value: %w", err)
	}

	autoUpdateTimeout, err := parseDuration(os.Getenv("AUTO_UPDATE_TIMEOUT"), DefaultAutoUpdateTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid AUTO_UPDATE_TIMEOUT value: %w", err)
	}

	autoUpdate := os.Getenv("AUTO_UPDATE")
	if autoUpdate == "" {
		autoUpdate = "true"
	}

	authMode := os.Getenv("AUTH_MODE")
	if authMode == "" {
		authMode = "legacy"
	}

	oauthCacheTTL, err := parseDuration(os.Getenv("OAUTH_CACHE_TTL"), DefaultOAuthCacheTTL)
	if err != nil {
		return nil, fmt.Errorf("invalid OAUTH_CACHE_TTL value: %w", err)
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
		GitLabURL:          os.Getenv("GITLAB_URL"),
		GitLabToken:        os.Getenv("GITLAB_TOKEN"),
		SkipTLSVerify:      skipTLS,
		MetaTools:          metaTools,
		Enterprise:         enterprise,
		ReadOnly:           readOnly,
		SafeMode:           safeMode,
		EmbeddedResources:  embeddedResources,
		UploadMaxFileSize:  maxFileSize,
		MaxHTTPClients:     maxHTTPClients,
		SessionTimeout:     sessionTimeout,
		RevalidateInterval: revalidateInterval,
		AutoUpdate:         autoUpdate,
		AutoUpdateRepo:     autoUpdateRepo,
		AutoUpdateInterval: autoUpdateInterval,
		AutoUpdateTimeout:  autoUpdateTimeout,
		AuthMode:           authMode,
		OAuthCacheTTL:      oauthCacheTTL,
		ExcludeTools:       ParseCSV(os.Getenv("EXCLUDE_TOOLS")),
		IgnoreScopes:       ignoreScopes,
		RateLimitRPS:       rateLimitRPS,
		RateLimitBurst:     rateLimitBurst,
		MetaParamSchema:    metaParamSchema,
	}

	if err = cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required configuration fields are present and valid.
func (c *Config) validate() error {
	if c.GitLabURL == "" {
		return errors.New("GITLAB_URL is required")
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
	if c.UploadMaxFileSize > MaxFileSize {
		return fmt.Errorf("UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB (got %d bytes)", c.UploadMaxFileSize)
	}
	if c.MaxHTTPClients <= 0 {
		return fmt.Errorf("MAX_HTTP_CLIENTS must be positive (got %d)", c.MaxHTTPClients)
	}
	if c.MaxHTTPClients > MaxHTTPClients {
		return fmt.Errorf("MAX_HTTP_CLIENTS exceeds maximum of %d (got %d)", MaxHTTPClients, c.MaxHTTPClients)
	}
	if c.AuthMode != "" && c.AuthMode != "legacy" && c.AuthMode != "oauth" {
		return fmt.Errorf("AUTH_MODE must be 'legacy' or 'oauth' (got %q)", c.AuthMode)
	}
	if c.OAuthCacheTTL != 0 {
		if c.OAuthCacheTTL < MinOAuthCacheTTL {
			return fmt.Errorf("OAUTH_CACHE_TTL %s is below minimum of %s", c.OAuthCacheTTL, MinOAuthCacheTTL)
		}
		if c.OAuthCacheTTL > MaxOAuthCacheTTL {
			return fmt.Errorf("OAUTH_CACHE_TTL %s exceeds maximum of %s", c.OAuthCacheTTL, MaxOAuthCacheTTL)
		}
	}
	if c.AutoUpdateTimeout != 0 {
		if c.AutoUpdateTimeout < MinAutoUpdateTimeout {
			return fmt.Errorf("AUTO_UPDATE_TIMEOUT %s is below minimum of %s", c.AutoUpdateTimeout, MinAutoUpdateTimeout)
		}
		if c.AutoUpdateTimeout > MaxAutoUpdateTimeout {
			return fmt.Errorf("AUTO_UPDATE_TIMEOUT %s exceeds maximum of %s", c.AutoUpdateTimeout, MaxAutoUpdateTimeout)
		}
	}
	if c.RateLimitRPS < 0 {
		return fmt.Errorf("RATE_LIMIT_RPS must be >= 0 (got %g)", c.RateLimitRPS)
	}
	if c.RateLimitRPS > 0 && c.RateLimitBurst < 1 {
		return fmt.Errorf("RATE_LIMIT_BURST must be >= 1 when RATE_LIMIT_RPS > 0 (got %d)", c.RateLimitBurst)
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
