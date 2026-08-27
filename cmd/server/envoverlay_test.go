// envoverlay_test.go verifies the HTTP-mode configuration precedence:
// an explicitly passed flag, then the environment, then the built-in default.
package main

import (
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

	t.Run("environment fills an unpassed flag", func(t *testing.T) {
		hcfg := newOverlayConfig()
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
			PublicURL:      &envPublicURL,
			TrustedOrigins: &envOrigins,
		})
		if hcfg.publicURL != envPublicURL {
			t.Errorf("publicURL = %q, want the environment value %q", hcfg.publicURL, envPublicURL)
		}
		if hcfg.trustedOrigins != envOrigins {
			t.Errorf("trustedOrigins = %q, want the environment value %q", hcfg.trustedOrigins, envOrigins)
		}
	})

	t.Run("passed flag beats the environment", func(t *testing.T) {
		hcfg := newOverlayConfig("public-url", "trusted-origins")
		hcfg.publicURL = "https://flag.example.com"
		hcfg.trustedOrigins = "https://flag-origin.example"
		applyHTTPEnvOverlay(hcfg, &config.HTTPEnvOverlay{
			PublicURL:      &envPublicURL,
			TrustedOrigins: &envOrigins,
		})
		if hcfg.publicURL != "https://flag.example.com" {
			t.Errorf("publicURL = %q, want the flag value", hcfg.publicURL)
		}
		if hcfg.trustedOrigins != "https://flag-origin.example" {
			t.Errorf("trustedOrigins = %q, want the flag value", hcfg.trustedOrigins)
		}
	})
}
