// env_overlay_test.go verifies the HTTP-mode configuration precedence:
// an explicitly passed flag, then the environment, then the built-in default.
package main

import (
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
)

// newOverlayConfig returns an httpConfig carrying the flag defaults, with the
// named flags marked as explicitly passed.
func newOverlayConfig(setFlags ...string) *httpConfig {
	hcfg := &httpConfig{
		sessionTimeout:     config.DefaultSessionTimeout,
		poolIdleTimeout:    config.DefaultPoolIdleTimeout,
		revalidateInterval: config.DefaultRevalidateInterval,
		maxHTTPClients:     config.DefaultMaxHTTPClients,
		capabilitySurface:  config.DefaultCapabilitySurface,
		setFlags:           map[string]bool{},
	}
	for _, name := range setFlags {
		hcfg.setFlags[name] = true
	}
	return hcfg
}

// TestApplyHTTPEnvOverlay_FlagBeatsEnvironment verifies the top of the
// precedence order: a flag the operator actually passed is never replaced by
// the environment. This is the property that keeps a deployment's command line
// authoritative over stray variables inherited from an image or a shell.
func TestApplyHTTPEnvOverlay_FlagBeatsEnvironment(t *testing.T) {
	hcfg := newOverlayConfig("pool-idle-timeout", "session-timeout", "read-only")
	hcfg.poolIdleTimeout = 15 * time.Minute
	hcfg.sessionTimeout = 5 * time.Minute
	hcfg.readOnly = true

	envPool := 6 * time.Hour
	envSession := 2 * time.Hour
	envReadOnly := false
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
		PoolIdleTimeout: &envPool,
		SessionTimeout:  &envSession,
		ReadOnly:        &envReadOnly,
	})

	if hcfg.poolIdleTimeout != 15*time.Minute {
		t.Errorf("poolIdleTimeout = %v, want the flag value 15m", hcfg.poolIdleTimeout)
	}
	if hcfg.sessionTimeout != 5*time.Minute {
		t.Errorf("sessionTimeout = %v, want the flag value 5m", hcfg.sessionTimeout)
	}
	if !hcfg.readOnly {
		t.Error("readOnly = false, want the flag value true")
	}
}

// TestApplyHTTPEnvOverlay_EnvironmentBeatsDefault verifies the middle of the
// order: when no flag was passed, the environment supplies the value instead
// of the built-in default. Before this layer existed the flag default always
// won, which made every documented HTTP environment variable inert.
func TestApplyHTTPEnvOverlay_EnvironmentBeatsDefault(t *testing.T) {
	hcfg := newOverlayConfig()

	envPool := 6 * time.Hour
	envClients := 250
	envSurface := config.ToolSurfaceMeta
	envReadOnly := true
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
		PoolIdleTimeout: &envPool,
		MaxHTTPClients:  &envClients,
		ToolSurface:     &envSurface,
		ReadOnly:        &envReadOnly,
	})

	if hcfg.poolIdleTimeout != 6*time.Hour {
		t.Errorf("poolIdleTimeout = %v, want the environment value 6h", hcfg.poolIdleTimeout)
	}
	if hcfg.maxHTTPClients != 250 {
		t.Errorf("maxHTTPClients = %d, want the environment value 250", hcfg.maxHTTPClients)
	}
	if hcfg.toolSurface != config.ToolSurfaceMeta {
		t.Errorf("toolSurface = %q, want the environment value %q", hcfg.toolSurface, config.ToolSurfaceMeta)
	}
	if !hcfg.readOnly {
		t.Error("readOnly = false, want the environment value true")
	}
}

// TestApplyHTTPEnvOverlay_AbsentEnvironmentKeepsDefault verifies the bottom of
// the order: a nil overlay field means the variable was absent, so whatever the
// flag layer resolved survives untouched.
func TestApplyHTTPEnvOverlay_AbsentEnvironmentKeepsDefault(t *testing.T) {
	hcfg := newOverlayConfig()
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{})

	if hcfg.poolIdleTimeout != config.DefaultPoolIdleTimeout {
		t.Errorf("poolIdleTimeout = %v, want the default %v", hcfg.poolIdleTimeout, config.DefaultPoolIdleTimeout)
	}
	if hcfg.maxHTTPClients != config.DefaultMaxHTTPClients {
		t.Errorf("maxHTTPClients = %d, want the default %d", hcfg.maxHTTPClients, config.DefaultMaxHTTPClients)
	}
}

// TestApplyHTTPEnvOverlay_TierOnlyPins verifies that the environment can pin
// the tier but never un-pin it. A non-explicit tier must leave detection in
// place, because clearing tierSet would silently switch a deployment from its
// configured tier to per-instance detection.
func TestApplyHTTPEnvOverlay_TierOnlyPins(t *testing.T) {
	t.Run("explicit environment tier pins it", func(t *testing.T) {
		hcfg := newOverlayConfig()
		tier := edition.Ultimate
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{Tier: &tier, TierExplicit: true})

		if !hcfg.tierSet || hcfg.tier != tier.String() {
			t.Errorf("tier = %q set = %v, want %q pinned", hcfg.tier, hcfg.tierSet, tier)
		}
	})

	t.Run("non-explicit environment tier leaves detection alone", func(t *testing.T) {
		hcfg := newOverlayConfig()
		tier := edition.Free
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{Tier: &tier, TierExplicit: false})

		if hcfg.tierSet {
			t.Error("tierSet = true, want detection preserved")
		}
	})

	t.Run("the tier flag wins over the environment", func(t *testing.T) {
		hcfg := newOverlayConfig("tier")
		hcfg.tier, hcfg.tierSet = "premium", true
		tier := edition.Ultimate
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{Tier: &tier, TierExplicit: true})

		if hcfg.tier != "premium" {
			t.Errorf("tier = %q, want the flag value premium", hcfg.tier)
		}
	})
}

// TestApplyHTTPEnvOverlay_SurfaceFlagBeatsDeprecatedEnv verifies that an
// explicit --tool-surface is not overridden by a stale META_TOOLS variable.
// The deprecated selector may only apply when the operator chose no surface at
// all on the command line.
func TestApplyHTTPEnvOverlay_SurfaceFlagBeatsDeprecatedEnv(t *testing.T) {
	hcfg := newOverlayConfig("tool-surface")
	hcfg.toolSurface = config.ToolSurfaceDynamic

	metaTools := true
	applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{MetaTools: &metaTools})

	if hcfg.metaToolsSet {
		t.Error("metaToolsSet = true, want the explicit --tool-surface to win over META_TOOLS")
	}
}

// TestApplyHTTPEnvOverlay_NilInputsAreNoOps verifies the guard clauses, so a
// caller that has no overlay cannot panic the startup path.
func TestApplyHTTPEnvOverlay_NilInputsAreNoOps(t *testing.T) {
	applyHTTPEnvOverlay(nil, &config.HTTPEnvOverlay{})

	hcfg := newOverlayConfig()
	applyHTTPEnvOverlay(hcfg, nil)
	if hcfg.poolIdleTimeout != config.DefaultPoolIdleTimeout {
		t.Errorf("poolIdleTimeout = %v, want it untouched", hcfg.poolIdleTimeout)
	}
}

// TestApplyHTTPEnvOverlay_OAuthOriginSettingsFollowPrecedence verifies both
// halves of the precedence rule for the two settings a containerised OAuth
// deployment can only supply through the environment. Before they were
// overlaid, AUTH_MODE=oauth reached the configuration but PUBLIC_URL did not,
// so such a deployment failed at startup demanding a flag it had no way to
// pass.
func TestApplyHTTPEnvOverlay_OAuthOriginSettingsFollowPrecedence(t *testing.T) {
	envPublicURL := "https://env.example.com/gitlab"
	envOrigins := "https://env-origin.example"
	// The application allow-list joined the overlay late: the variable was
	// documented for HTTP mode and read by nothing there, so only the flag
	// admitted an application.
	envClientUID := "12ab,34cd"

	t.Run("environment fills an unpassed flag", func(t *testing.T) {
		hcfg := newOverlayConfig()
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
			PublicURL:      &envPublicURL,
			TrustedOrigins: &envOrigins,
			OAuthClientUID: &envClientUID,
		})
		if hcfg.publicURL != envPublicURL {
			t.Errorf("publicURL = %q, want the environment value %q", hcfg.publicURL, envPublicURL)
		}
		if hcfg.trustedOrigins != envOrigins {
			t.Errorf("trustedOrigins = %q, want the environment value %q", hcfg.trustedOrigins, envOrigins)
		}
		if hcfg.oauthClientUID != envClientUID {
			t.Errorf("oauthClientUID = %q, want the environment value %q", hcfg.oauthClientUID, envClientUID)
		}
	})

	t.Run("passed flag beats the environment", func(t *testing.T) {
		hcfg := newOverlayConfig("public-url", "trusted-origins", "oauth-client-uid")
		hcfg.publicURL = "https://flag.example.com"
		hcfg.trustedOrigins = "https://flag-origin.example"
		hcfg.oauthClientUID = "flag-uid"
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
			PublicURL:      &envPublicURL,
			TrustedOrigins: &envOrigins,
			OAuthClientUID: &envClientUID,
		})
		if hcfg.publicURL != "https://flag.example.com" {
			t.Errorf("publicURL = %q, want the flag value", hcfg.publicURL)
		}
		if hcfg.trustedOrigins != "https://flag-origin.example" {
			t.Errorf("trustedOrigins = %q, want the flag value", hcfg.trustedOrigins)
		}
		if hcfg.oauthClientUID != "flag-uid" {
			t.Errorf("oauthClientUID = %q, want the flag value", hcfg.oauthClientUID)
		}
	})
}

// TestApplyHTTPEnvOverlay_SettingsWithoutTheirOwnTest covers the overlay
// entries that no other case exercises: the instance list, the deprecated
// boolean surface selector, and the two rate-limit numbers.
//
// GITLAB_URL is the one that is not a plain assignment. The environment carries
// one string where the flag can be repeated, so a comma-separated value has to
// spell the same list — and it replaces whatever the flag layer left rather
// than appending to it, or a value from a previous parse would survive
// underneath the operator's own.
func TestApplyHTTPEnvOverlay_SettingsWithoutTheirOwnTest(t *testing.T) {
	t.Parallel()

	t.Run("the instance list comes from one comma-separated value", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig()
		hcfg.gitlabURLs = repeatedFlag{"https://left-over.example.com"}
		value := "https://gitlab.com, https://gitlab.example.com"

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{GitLabURL: &value})

		want := repeatedFlag{"https://gitlab.com", "https://gitlab.example.com"}
		if !slices.Equal(hcfg.gitlabURLs, want) {
			t.Errorf("gitlabURLs = %v, want %v", hcfg.gitlabURLs, want)
		}
	})

	t.Run("a passed flag keeps the instance list", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig("gitlab-url")
		hcfg.gitlabURLs = repeatedFlag{"https://from-the-flag.example.com"}
		value := "https://from-the-environment.example.com"

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{GitLabURL: &value})

		if !slices.Equal(hcfg.gitlabURLs, repeatedFlag{"https://from-the-flag.example.com"}) {
			t.Errorf("gitlabURLs = %v, want the flag value kept", hcfg.gitlabURLs)
		}
	})

	t.Run("the deprecated boolean selector is honored and marked", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig()
		metaTools := true

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{MetaTools: &metaTools})

		if !hcfg.metaTools {
			t.Error("metaTools = false, want the deprecated environment selector honored")
		}
		// Marked as set, because the flag layer reads it only when it was
		// stated: an unmarked value cannot be told from the flag's default and
		// would silently select the meta surface for everyone.
		if !hcfg.metaToolsSet {
			t.Error("metaToolsSet = false, want the value recorded as stated")
		}
	})

	t.Run("the rate limit comes from the environment", func(t *testing.T) {
		t.Parallel()

		hcfg := newOverlayConfig()
		rps, burst := 42.5, 99

		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{RateLimitRPS: &rps, RateLimitBurst: &burst})

		if hcfg.rateLimitRPS != rps {
			t.Errorf("rateLimitRPS = %v, want %v", hcfg.rateLimitRPS, rps)
		}
		if hcfg.rateLimitBurst != burst {
			t.Errorf("rateLimitBurst = %d, want %d", hcfg.rateLimitBurst, burst)
		}
	})
}
