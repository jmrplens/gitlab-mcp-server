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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
)

// HTTPEnvOverlay holds the HTTP-relevant settings found in the environment.
// A nil field means the variable was absent or empty, and the caller keeps
// whatever the flag layer resolved.
type HTTPEnvOverlay struct {
	GitLabURL         *string
	SkipTLSVerify     *bool
	ToolSurface       *string
	MetaTools         *bool
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
// applies. An invalid value is an error rather than a silent fallback, so a
// typo in a deployment manifest fails at startup instead of quietly running
// with a default the operator did not choose.
func LoadHTTPEnvOverlay() (*HTTPEnvOverlay, error) {
	overlay := &HTTPEnvOverlay{}
	for _, load := range []func(*HTTPEnvOverlay) error{
		loadOverlaySurface,
		loadOverlayBooleans,
		loadOverlayLimits,
		loadOverlayAuthAndRate,
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
	// TOOL_SURFACE and META_TOOLS resolve together: the deprecated selector is
	// only consulted when the canonical one is absent.
	if envPresent("TOOL_SURFACE") || envPresent("META_TOOLS") {
		surface, metaTools, err := ParseToolSurface(Getenv("TOOL_SURFACE"), Getenv("META_TOOLS"))
		if err != nil {
			return err
		}
		o.ToolSurface, o.MetaTools = &surface, &metaTools
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
	if envPresent("GITLAB_TIER") || envPresent("GITLAB_ENTERPRISE") {
		tier, explicit, err := resolveTierEnv(os.Getenv("GITLAB_TIER"), os.Getenv("GITLAB_ENTERPRISE"))
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
		{"GITLAB_SKIP_TLS_VERIFY", &o.SkipTLSVerify},
		{"GITLAB_READ_ONLY", &o.ReadOnly},
		{"GITLAB_SAFE_MODE", &o.SafeMode},
		{"EMBEDDED_RESOURCES", &o.EmbeddedResources},
		{"GITLAB_IGNORE_SCOPES", &o.IgnoreScopes},
	} {
		if !envPresent(b.name) {
			continue
		}
		value, err := parseBool(Getenv(b.name), false)
		if err != nil {
			return fmt.Errorf("invalid %s value: %w", b.name, err)
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

	durations := []struct {
		name     string
		fallback time.Duration
		maxValue time.Duration
		zeroOK   bool
		target   **time.Duration
	}{
		{"SESSION_TIMEOUT", DefaultSessionTimeout, MaxSessionTimeout, false, &o.SessionTimeout},
		{"POOL_IDLE_TIMEOUT", DefaultPoolIdleTimeout, MaxPoolIdleTimeout, true, &o.PoolIdleTimeout},
		{"SESSION_REVALIDATE_INTERVAL", DefaultRevalidateInterval, MaxRevalidateInterval, true, &o.RevalidateInterval},
		{"ACTION_TIMEOUT", DefaultActionTimeout, MaxActionTimeout, true, &o.ActionTimeout},
		{"DRAIN_DELAY", DefaultDrainDelay, MaxDrainDelay, true, &o.DrainDelay},
	}
	for _, d := range durations {
		if !envPresent(d.name) {
			continue
		}
		var (
			value time.Duration
			err   error
		)
		if d.zeroOK {
			value, err = parseDisableableDurationEnv(d.name, d.fallback, d.maxValue)
		} else {
			value, err = parseBoundedDurationEnv(d.name, d.fallback, d.maxValue)
		}
		if err != nil {
			return err
		}
		*d.target = &value
	}
	return nil
}

func loadOverlayAuthAndRate(o *HTTPEnvOverlay) error {
	if envPresent("AUTH_MODE") {
		auth, err := loadAuthEnv()
		if err != nil {
			return err
		}
		o.AuthMode = &auth.mode
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
		auth, err := loadAuthEnv()
		if err != nil {
			return err
		}
		o.OAuthCacheTTL = &auth.oauthCacheTTL
	}
	// Carried verbatim, as the flag's own value is: the flag layer splits the
	// comma-separated list. Without this the documented variable was inert in
	// the one mode where OAuth exists.
	if envPresent("OAUTH_CLIENT_UID") {
		value := TrimmedGetenv("OAUTH_CLIENT_UID")
		o.OAuthClientUID = &value
	}
	if envPresent("RATE_LIMIT_RPS") {
		value, err := parseFloatNonNegative(Getenv("RATE_LIMIT_RPS"), 0)
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
	return nil
}
