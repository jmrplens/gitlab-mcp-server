// http_overlay.go layers environment variables underneath the HTTP-mode CLI
// flags.
//
// HTTP mode used to build its whole configuration from flags and never consult
// the environment, so every documented HTTP environment variable was inert:
// the flag default won even when the operator had exported a value and passed
// no flag. This file supplies the middle layer of the intended precedence —
// an explicitly passed flag, then the environment, then the built-in default.
//
// Only values actually present in the environment are reported. That
// distinction is the whole point: the existing loaders substitute defaults for
// absent variables, which would make "exported the default" indistinguishable
// from "exported nothing" and let the overlay overwrite a flag default with an
// identical value for the wrong reason.

package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// HTTPEnvOverlay holds the HTTP-relevant settings found in the environment.
// A nil field means the variable was absent or empty, and the caller keeps
// whatever the flag layer resolved, except for UploadMaxFileSize, whose flag
// writes the variable and leaves the flag layer nothing to keep.
type HTTPEnvOverlay struct {
	GitLabURL         *string
	SkipTLSVerify     *bool
	ToolSurface       *string
	CapabilitySurface *string
	MetaParamSchema   *string

	Tier         *edition.Tier
	TierExplicit bool

	ReadOnly          *bool
	SafeMode          *bool
	EmbeddedResources *bool
	IgnoreScopes      *bool
	ExcludeTools      *string

	MaxHTTPClients     *int
	SessionTimeout     *time.Duration
	PoolIdleTimeout    *time.Duration
	RevalidateInterval *time.Duration
	ActionTimeout      *time.Duration
	DrainDelay         *time.Duration

	AuthMode       *string
	PublicURL      *string
	TrustedOrigins *string
	OAuthCacheTTL  *time.Duration
	OAuthClientUID *string

	RateLimitRPS   *float64
	RateLimitBurst *int

	// The two authentication budgets. Each is a pair, and each half is
	// overlaid on its own: an operator who exports only a window means the
	// window, with the other half left at whatever the flag or the default
	// says.
	AuthFailureLimit       *int
	AuthFailureWindow      *time.Duration
	AuthDistinctTokenLimit *int
	AuthDistinctWindow     *time.Duration

	// The bucket each client address draws on for the discovery documents
	// (register row RTC-008), each half overlaid on its own like a budget's.
	DiscoveryRateLimitRPS   *float64
	DiscoveryRateLimitBurst *int

	// UploadMaxFileSize is the largest local file a tool accepts (register
	// row RQB-007). --upload-max-file-size writes the variable rather than a
	// field of the flag layer, so the value read here is the flag's when it
	// was passed. It is the one field whose nil the caller does not answer
	// with the flag layer's value, since that layer holds none: the caller
	// sets [DefaultMaxFileSize].
	UploadMaxFileSize *int64

	// The operator's statement that something in front of a plain HTTP
	// listener off loopback encrypts the hop, which turns the startup warning
	// about that listener into a line at INFO. A container sets it here
	// because the image's command already names --http-addr, and any argument
	// after the image name replaces that command wholesale.
	AllowPlaintextHTTP *bool
}

// envPresent reports whether name is set to a non-empty value, under either
// spelling.
//
// This is the gate every read below sits behind, so it has to agree with
// [Getenv] about which name counts. A presence check that looked only at the
// unprefixed spelling would leave an operator who migrated to GITLAB_MCP_ with
// an HTTP deployment that reads none of their settings.
func envPresent(name string) bool {
	return TrimmedGetenv(name) != ""
}

// LoadHTTPEnvOverlay reads the environment variables that have an HTTP flag
// counterpart, validating each with the same parser and bounds the stdio path
// applies to the settings both transports have. An invalid value is an error
// rather than a silent fallback, so a typo in a deployment manifest fails at
// startup instead of quietly running with a default the operator did not
// choose. The settings only HTTP mode has are read here alone
// ([httpOnlyNames]).
func LoadHTTPEnvOverlay() (*HTTPEnvOverlay, error) {
	overlay := &HTTPEnvOverlay{}
	for _, load := range []func(*HTTPEnvOverlay) error{
		loadOverlaySurface,
		loadOverlayBooleans,
		loadOverlayLimits,
		loadOverlayAuthAndRate,
		loadOverlayDiscoveryRate,
		loadOverlayUploadLimit,
	} {
		if err := load(overlay); err != nil {
			return nil, err
		}
	}
	return overlay, nil
}

func loadOverlaySurface(o *HTTPEnvOverlay) error {
	if envPresent("GITLAB_URL") {
		value := strings.TrimRight(strings.TrimSpace(os.Getenv("GITLAB_URL")), "/")
		o.GitLabURL = &value
	}
	if envPresent("TOOL_SURFACE") {
		surface, err := ParseToolSurface(Getenv("TOOL_SURFACE"))
		if err != nil {
			return err
		}
		o.ToolSurface = &surface
	}
	if envPresent("CAPABILITY_SURFACE") {
		value, err := parseCapabilitySurface(Getenv("CAPABILITY_SURFACE"), DefaultCapabilitySurface)
		if err != nil {
			return fmt.Errorf("invalid CAPABILITY_SURFACE value: %w", err)
		}
		o.CapabilitySurface = &value
	}
	if envPresent("META_PARAM_SCHEMA") {
		value, err := parseMetaParamSchema(Getenv("META_PARAM_SCHEMA"), DefaultMetaParamSchema)
		if err != nil {
			return fmt.Errorf("invalid META_PARAM_SCHEMA value: %w", err)
		}
		o.MetaParamSchema = &value
	}
	if envPresent("TIER") {
		tier, explicit, err := parseTierEnv(Getenv("TIER"))
		if err != nil {
			return err
		}
		o.Tier, o.TierExplicit = &tier, explicit
	}
	if envPresent("EXCLUDE_TOOLS") {
		value := Getenv("EXCLUDE_TOOLS")
		o.ExcludeTools = &value
	}
	return nil
}

func loadOverlayBooleans(o *HTTPEnvOverlay) error {
	for _, b := range []struct {
		name   string
		target **bool
	}{
		{"SKIP_TLS_VERIFY", &o.SkipTLSVerify},
		{"READ_ONLY", &o.ReadOnly},
		{"SAFE_MODE", &o.SafeMode},
		{"EMBEDDED_RESOURCES", &o.EmbeddedResources},
		{"IGNORE_SCOPES", &o.IgnoreScopes},
		{"ALLOW_PLAINTEXT_HTTP", &o.AllowPlaintextHTTP},
	} {
		if !envPresent(b.name) {
			continue
		}
		value, err := parseBool(Getenv(b.name), false)
		if err != nil {
			return fmt.Errorf("invalid %s%s value: %w", EnvPrefix, b.name, err)
		}
		*b.target = &value
	}
	return nil
}

func loadOverlayLimits(o *HTTPEnvOverlay) error {
	if envPresent("MAX_HTTP_CLIENTS") {
		value, err := parseInt(Getenv("MAX_HTTP_CLIENTS"), DefaultMaxHTTPClients)
		if err != nil {
			return fmt.Errorf("invalid MAX_HTTP_CLIENTS value: %w", err)
		}
		o.MaxHTTPClients = &value
	}

	// A zero turns each of these off, as it does to the flag of the same name:
	// the session timeout's used to be refused here (issue 958).
	durations := []struct {
		name     string
		fallback time.Duration
		maxValue time.Duration
		target   **time.Duration
	}{
		{"SESSION_TIMEOUT", DefaultSessionTimeout, MaxSessionTimeout, &o.SessionTimeout},
		{"POOL_IDLE_TIMEOUT", DefaultPoolIdleTimeout, MaxPoolIdleTimeout, &o.PoolIdleTimeout},
		{"SESSION_REVALIDATE_INTERVAL", DefaultRevalidateInterval, MaxRevalidateInterval, &o.RevalidateInterval},
		{"ACTION_TIMEOUT", DefaultActionTimeout, MaxActionTimeout, &o.ActionTimeout},
		{"DRAIN_DELAY", DefaultDrainDelay, MaxDrainDelay, &o.DrainDelay},
	}
	for _, d := range durations {
		if !envPresent(d.name) {
			continue
		}
		value, err := parseDisableableDurationEnv(d.name, d.fallback, d.maxValue)
		if err != nil {
			return err
		}
		*d.target = &value
	}
	return nil
}

func loadOverlayAuthAndRate(o *HTTPEnvOverlay) error {
	// AUTH_MODE is passed through unparsed as well: validateHTTPAuthConfig
	// owns the check, since it needs the resolved mode to judge the rest.
	if envPresent("AUTH_MODE") {
		value := Getenv("AUTH_MODE")
		o.AuthMode = &value
	}
	// PUBLIC_URL and TRUSTED_ORIGINS are passed through unparsed: both are
	// validated later against the fully resolved configuration (public-url
	// only has to be https when auth-mode ends up being oauth), and parsing
	// them twice would let the two checks disagree.
	if envPresent("PUBLIC_URL") {
		value := TrimmedGetenv("PUBLIC_URL")
		o.PublicURL = &value
	}
	if envPresent("TRUSTED_ORIGINS") {
		value := TrimmedGetenv("TRUSTED_ORIGINS")
		o.TrustedOrigins = &value
	}
	if envPresent("OAUTH_CACHE_TTL") {
		value, err := parseDuration(Getenv("OAUTH_CACHE_TTL"), DefaultOAuthCacheTTL)
		if err != nil {
			return fmt.Errorf("invalid OAUTH_CACHE_TTL value: %w", err)
		}
		o.OAuthCacheTTL = &value
	}
	// Carried verbatim, as the flag's own value is: the flag layer splits the
	// comma-separated list. Without this the documented variable was inert in
	// the one mode where OAuth exists.
	if envPresent("OAUTH_CLIENT_UID") {
		value := TrimmedGetenv("OAUTH_CLIENT_UID")
		o.OAuthClientUID = &value
	}
	if envPresent("RATE_LIMIT_RPS") {
		value, err := parseFloatNonNegative(Getenv("RATE_LIMIT_RPS"), tenancy.ToolCallRateEnvDefault) // register row RTC-001
		if err != nil {
			return fmt.Errorf("invalid RATE_LIMIT_RPS value: %w", err)
		}
		o.RateLimitRPS = &value
	}
	if envPresent("RATE_LIMIT_BURST") {
		value, err := parseIntNonNegative(Getenv("RATE_LIMIT_BURST"), DefaultRateLimitBurst)
		if err != nil {
			return fmt.Errorf("invalid RATE_LIMIT_BURST value: %w", err)
		}
		o.RateLimitBurst = &value
	}
	return o.readAuthBudgetEnv()
}

// loadOverlayDiscoveryRate overlays the two settings of the bucket each client
// address draws on for the discovery documents (register row RTC-008), each
// held to the bounds the flags are held to, so an invalid value fails startup
// naming the variable rather than falling back in silence. Zero is a value
// like any other here, and switches the bucket off.
//
// Only HTTP mode reads them: a stdio server publishes no document, so it has
// nothing for them to bound.
func loadOverlayDiscoveryRate(o *HTTPEnvOverlay) error {
	if envPresent("DISCOVERY_RATE_LIMIT_RPS") {
		value, err := parseFloatNonNegative(Getenv("DISCOVERY_RATE_LIMIT_RPS"), DefaultDiscoveryRateLimitRPS)
		if err != nil {
			return fmt.Errorf("invalid %sDISCOVERY_RATE_LIMIT_RPS value: %w", EnvPrefix, err)
		}
		if value > MaxDiscoveryRateLimitRPS {
			return fmt.Errorf("%sDISCOVERY_RATE_LIMIT_RPS exceeds maximum of %d (got %g)", EnvPrefix, MaxDiscoveryRateLimitRPS, value)
		}
		o.DiscoveryRateLimitRPS = &value
	}
	if envPresent("DISCOVERY_RATE_LIMIT_BURST") {
		value, err := parseIntNonNegative(Getenv("DISCOVERY_RATE_LIMIT_BURST"), DefaultDiscoveryRateLimitBurst)
		if err != nil {
			return fmt.Errorf("invalid %sDISCOVERY_RATE_LIMIT_BURST value: %w", EnvPrefix, err)
		}
		if value > MaxDiscoveryRateLimitBurst {
			return fmt.Errorf("%sDISCOVERY_RATE_LIMIT_BURST exceeds maximum of %d (got %d)", EnvPrefix, MaxDiscoveryRateLimitBurst, value)
		}
		o.DiscoveryRateLimitBurst = &value
	}
	return nil
}

// loadOverlayUploadLimit overlays the upload limit (register row RQB-007)
// through the reader stdio's [Load] calls, so a value that does not parse,
// one that is not positive and one above the ceiling refuse startup in both
// transports with the same words, and a blank one is unset in both. HTTP mode
// used to read it outside the overlay, warning and using the default for the
// first two and the maximum for the third (issue 1170).
func loadOverlayUploadLimit(o *HTTPEnvOverlay) error {
	if !envPresent("UPLOAD_MAX_FILE_SIZE") {
		return nil
	}
	value, err := UploadMaxFileSizeFromEnv()
	if err != nil {
		return err
	}
	o.UploadMaxFileSize = &value
	return nil
}

// readAuthBudgetEnv overlays the four authentication-budget settings, each
// validated with the bound its flag is held to, so a value exported without
// its flag is honored and an invalid one fails startup rather than falling
// back in silence. This is their one reader: a stdio server has no
// authentication gate and reads none of them (issue 1171).
func (o *HTTPEnvOverlay) readAuthBudgetEnv() error {
	if envPresent("AUTH_FAILURE_LIMIT") {
		value, err := parseIntNonNegative(Getenv("AUTH_FAILURE_LIMIT"), DefaultAuthFailureLimit)
		if err != nil {
			return fmt.Errorf("invalid AUTH_FAILURE_LIMIT value: %w", err)
		}
		if value > MaxAuthFailureLimit {
			return fmt.Errorf("AUTH_FAILURE_LIMIT exceeds maximum of %d (got %d)", MaxAuthFailureLimit, value)
		}
		o.AuthFailureLimit = &value
	}
	if envPresent("AUTH_FAILURE_WINDOW") {
		value, err := parseDisableableDurationEnv("AUTH_FAILURE_WINDOW", DefaultAuthFailureWindow, MaxAuthFailureWindow)
		if err != nil {
			return err
		}
		o.AuthFailureWindow = &value
	}
	if envPresent("AUTH_DISTINCT_TOKEN_LIMIT") {
		value, err := parseIntNonNegative(Getenv("AUTH_DISTINCT_TOKEN_LIMIT"), DefaultAuthDistinctTokenLimit)
		if err != nil {
			return fmt.Errorf("invalid AUTH_DISTINCT_TOKEN_LIMIT value: %w", err)
		}
		if value > MaxAuthDistinctTokenLimit {
			return fmt.Errorf("AUTH_DISTINCT_TOKEN_LIMIT exceeds maximum of %d (got %d)", MaxAuthDistinctTokenLimit, value)
		}
		o.AuthDistinctTokenLimit = &value
	}
	if envPresent("AUTH_DISTINCT_TOKEN_WINDOW") {
		value, err := parseDisableableDurationEnv("AUTH_DISTINCT_TOKEN_WINDOW", DefaultAuthDistinctWindow, MaxAuthDistinctWindow)
		if err != nil {
			return err
		}
		o.AuthDistinctWindow = &value
	}
	return nil
}
