// http_overlay_test.go verifies the environment layer that sits underneath the
// HTTP-mode CLI flags.
package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
)

// TestLoadHTTPEnvOverlay_AbsentVariablesReportNothing verifies the property the
// whole overlay rests on: a variable that is not set must leave its field nil.
// The underlying loaders substitute defaults for absent variables, so if the
// overlay reported those it would be indistinguishable from an operator having
// exported the default, and it would overwrite the flag layer for the wrong
// reason.
func TestLoadHTTPEnvOverlay_AbsentVariablesReportNothing(t *testing.T) {
	clearOverlayEnv(t)

	overlay, err := LoadHTTPEnvOverlay()
	if err != nil {
		t.Fatalf("LoadHTTPEnvOverlay() error: %v", err)
	}

	nilFields := map[string]bool{
		"GitLabURL":          overlay.GitLabURL == nil,
		"SkipTLSVerify":      overlay.SkipTLSVerify == nil,
		"ToolSurface":        overlay.ToolSurface == nil,
		"MetaTools":          overlay.MetaTools == nil,
		"CapabilitySurface":  overlay.CapabilitySurface == nil,
		"MetaParamSchema":    overlay.MetaParamSchema == nil,
		"Tier":               overlay.Tier == nil,
		"ReadOnly":           overlay.ReadOnly == nil,
		"SafeMode":           overlay.SafeMode == nil,
		"EmbeddedResources":  overlay.EmbeddedResources == nil,
		"IgnoreScopes":       overlay.IgnoreScopes == nil,
		"ExcludeTools":       overlay.ExcludeTools == nil,
		"MaxHTTPClients":     overlay.MaxHTTPClients == nil,
		"SessionTimeout":     overlay.SessionTimeout == nil,
		"PoolIdleTimeout":    overlay.PoolIdleTimeout == nil,
		"RevalidateInterval": overlay.RevalidateInterval == nil,
		"ActionTimeout":      overlay.ActionTimeout == nil,
		"AuthMode":           overlay.AuthMode == nil,
		"PublicURL":          overlay.PublicURL == nil,
		"TrustedOrigins":     overlay.TrustedOrigins == nil,
		"OAuthCacheTTL":      overlay.OAuthCacheTTL == nil,
		"RateLimitRPS":       overlay.RateLimitRPS == nil,
		"RateLimitBurst":     overlay.RateLimitBurst == nil,
	}
	for field, isNil := range nilFields {
		t.Run(field, func(t *testing.T) {
			if !isNil {
				t.Errorf("%s was reported despite its variable being unset", field)
			}
		})
	}
	if overlay.TierExplicit {
		t.Error("TierExplicit = true with no tier variable set")
	}
}

// TestLoadHTTPEnvOverlay_PresentVariablesAreParsed verifies that each variable
// is read, validated by the same parser the stdio path uses, and reported.
func TestLoadHTTPEnvOverlay_PresentVariablesAreParsed(t *testing.T) {
	for _, tt := range presentVariableCases() {
		t.Run(tt.name, func(t *testing.T) {
			clearOverlayEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			overlay, err := LoadHTTPEnvOverlay()
			if err != nil {
				t.Fatalf("LoadHTTPEnvOverlay() error: %v", err)
			}
			tt.assert(t, overlay)
		})
	}
}

// presentVariableCase is one variable (or group of related variables) set in
// the environment together with what the overlay is expected to report.
type presentVariableCase struct {
	name   string
	env    map[string]string
	assert func(*testing.T, *HTTPEnvOverlay)
}

// presentVariableCases holds the table separately from the test body so the
// runner stays small enough to read at a glance.
func presentVariableCases() []presentVariableCase {
	return []presentVariableCase{
		{
			name: "gitlab url loses its trailing slash",
			env:  map[string]string{"GITLAB_URL": "https://gitlab.example.com/"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "GitLabURL", o.GitLabURL, "https://gitlab.example.com")
			},
		},
		{
			// Without this, a compose deployment that sets AUTH_MODE=oauth
			// with no flags cannot start at all: the overlay reads the mode
			// but not the resource identifier oauth mode requires.
			name: "public url and trusted origins reach the overlay",
			env: map[string]string{
				"PUBLIC_URL":      " https://mcp.example.com/gitlab ",
				"TRUSTED_ORIGINS": " https://claude.ai,https://inspector.example ",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "PublicURL", o.PublicURL, "https://mcp.example.com/gitlab")
				assertStr(t, "TrustedOrigins", o.TrustedOrigins, "https://claude.ai,https://inspector.example")
			},
		},
		{
			name: "tool surface resolves the canonical selector",
			env:  map[string]string{"TOOL_SURFACE": ToolSurfaceMeta},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "ToolSurface", o.ToolSurface, ToolSurfaceMeta)
				if o.MetaTools == nil || !*o.MetaTools {
					t.Error("MetaTools should accompany a meta surface")
				}
			},
		},
		{
			name: "the deprecated selector still resolves a surface",
			env:  map[string]string{"META_TOOLS": "false"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "ToolSurface", o.ToolSurface, ToolSurfaceIndividual)
			},
		},
		{
			name: "capability surface",
			env:  map[string]string{"CAPABILITY_SURFACE": CapabilitySurfaceMinimal},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "CapabilitySurface", o.CapabilitySurface, CapabilitySurfaceMinimal)
			},
		},
		{
			name: "meta param schema",
			env:  map[string]string{"META_PARAM_SCHEMA": MetaParamSchemaCompact},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "MetaParamSchema", o.MetaParamSchema, MetaParamSchemaCompact)
			},
		},
		{
			name: "an explicit tier is pinned",
			env:  map[string]string{"GITLAB_TIER": "ultimate"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.Tier == nil || *o.Tier != edition.Ultimate || !o.TierExplicit {
					t.Errorf("Tier = %v explicit = %v, want ultimate pinned", o.Tier, o.TierExplicit)
				}
			},
		},
		{
			name: "the deprecated enterprise flag still resolves a tier",
			env:  map[string]string{"GITLAB_ENTERPRISE": "true"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.Tier == nil || *o.Tier != edition.Ultimate {
					t.Errorf("Tier = %v, want ultimate", o.Tier)
				}
			},
		},
		{
			name: "exclude tools is carried verbatim for the flag layer to split",
			env:  map[string]string{"EXCLUDE_TOOLS": "gitlab_runner,gitlab_geo"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "ExcludeTools", o.ExcludeTools, "gitlab_runner,gitlab_geo")
			},
		},
		{
			name: "booleans",
			env: map[string]string{
				"GITLAB_SKIP_TLS_VERIFY": "true",
				"GITLAB_READ_ONLY":       "true",
				"GITLAB_SAFE_MODE":       "true",
				"EMBEDDED_RESOURCES":     "false",
				"GITLAB_IGNORE_SCOPES":   "true",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertBool(t, "SkipTLSVerify", o.SkipTLSVerify, true)
				assertBool(t, "ReadOnly", o.ReadOnly, true)
				assertBool(t, "SafeMode", o.SafeMode, true)
				assertBool(t, "EmbeddedResources", o.EmbeddedResources, false)
				assertBool(t, "IgnoreScopes", o.IgnoreScopes, true)
			},
		},
		{
			name: "pool limits",
			env: map[string]string{
				"MAX_HTTP_CLIENTS":            "250",
				"SESSION_TIMEOUT":             "10m",
				"POOL_IDLE_TIMEOUT":           "6h",
				"SESSION_REVALIDATE_INTERVAL": "5m",
				"ACTION_TIMEOUT":              "20m",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.MaxHTTPClients == nil || *o.MaxHTTPClients != 250 {
					t.Errorf("MaxHTTPClients = %v, want 250", o.MaxHTTPClients)
				}
				assertDur(t, "SessionTimeout", o.SessionTimeout, 10*time.Minute)
				assertDur(t, "PoolIdleTimeout", o.PoolIdleTimeout, 6*time.Hour)
				assertDur(t, "RevalidateInterval", o.RevalidateInterval, 5*time.Minute)
				assertDur(t, "ActionTimeout", o.ActionTimeout, 20*time.Minute)
			},
		},
		{
			name: "zero disables the two settings documented as disableable",
			env: map[string]string{
				"POOL_IDLE_TIMEOUT":           "0",
				"SESSION_REVALIDATE_INTERVAL": "0",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertDur(t, "PoolIdleTimeout", o.PoolIdleTimeout, 0)
				assertDur(t, "RevalidateInterval", o.RevalidateInterval, 0)
			},
		},
		{
			name: "auth and rate limiting",
			env: map[string]string{
				"AUTH_MODE":        "oauth",
				"OAUTH_CACHE_TTL":  "30m",
				"RATE_LIMIT_RPS":   "12.5",
				"RATE_LIMIT_BURST": "80",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "AuthMode", o.AuthMode, "oauth")
				assertDur(t, "OAuthCacheTTL", o.OAuthCacheTTL, 30*time.Minute)
				if o.RateLimitRPS == nil || *o.RateLimitRPS != 12.5 {
					t.Errorf("RateLimitRPS = %v, want 12.5", o.RateLimitRPS)
				}
				if o.RateLimitBurst == nil || *o.RateLimitBurst != 80 {
					t.Errorf("RateLimitBurst = %v, want 80", o.RateLimitBurst)
				}
			},
		},
		{
			name: "whitespace counts as absent",
			env:  map[string]string{"AUTH_MODE": "   "},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.AuthMode != nil {
					t.Errorf("AuthMode = %v, want nil for a blank value", *o.AuthMode)
				}
			},
		},
	}
}

// TestLoadHTTPEnvOverlay_InvalidValuesFailLoudly verifies that a malformed
// value is an error rather than a silent fallback, and that the message names
// the variable so a typo in a deployment manifest is self-diagnosing.
func TestLoadHTTPEnvOverlay_InvalidValuesFailLoudly(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantMsg string
	}{
		{"TOOL_SURFACE", "bogus", "TOOL_SURFACE"},
		{"META_TOOLS", "bogus", "META_TOOLS"},
		{"CAPABILITY_SURFACE", "bogus", "CAPABILITY_SURFACE"},
		{"META_PARAM_SCHEMA", "bogus", "META_PARAM_SCHEMA"},
		{"GITLAB_TIER", "bogus", "GITLAB_TIER"},
		{"GITLAB_ENTERPRISE", "bogus", "GITLAB_ENTERPRISE"},
		{"GITLAB_SKIP_TLS_VERIFY", "bogus", "GITLAB_SKIP_TLS_VERIFY"},
		{"GITLAB_READ_ONLY", "bogus", "GITLAB_READ_ONLY"},
		{"MAX_HTTP_CLIENTS", "bogus", "MAX_HTTP_CLIENTS"},
		{"SESSION_TIMEOUT", "bogus", "SESSION_TIMEOUT"},
		{"POOL_IDLE_TIMEOUT", "bogus", "POOL_IDLE_TIMEOUT"},
		{"SESSION_REVALIDATE_INTERVAL", "bogus", "SESSION_REVALIDATE_INTERVAL"},
		{"OAUTH_CACHE_TTL", "bogus", "OAUTH_CACHE_TTL"},
		{"RATE_LIMIT_RPS", "bogus", "RATE_LIMIT_RPS"},
		{"RATE_LIMIT_BURST", "bogus", "RATE_LIMIT_BURST"},
		{"POOL_IDLE_TIMEOUT", "48h", "exceeds maximum"},
		{"ACTION_TIMEOUT", "bogus", "ACTION_TIMEOUT"},
		{"ACTION_TIMEOUT", "48h", "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			clearOverlayEnv(t)
			t.Setenv(tt.name, tt.value)

			overlay, err := LoadHTTPEnvOverlay()
			if err == nil {
				t.Fatalf("LoadHTTPEnvOverlay() error = nil, want an error; overlay = %+v", overlay)
			}
			if overlay != nil {
				t.Errorf("overlay = %+v, want nil alongside the error", overlay)
			}
			// The message has to be self-diagnosing: a bare strconv error
			// would leave an operator hunting for which variable is wrong.
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantMsg)
			}
		})
	}
}

// TestLoadHTTPEnvOverlay_AuthModeIsValidatedDownstream documents a deliberate
// split: the overlay carries AUTH_MODE through unparsed because
// validateHTTPAuthConfig owns that check, and it needs the resolved value to
// also verify that oauth was given a fixed --gitlab-url. Rejecting it here
// would duplicate the rule in two places.
func TestLoadHTTPEnvOverlay_AuthModeIsValidatedDownstream(t *testing.T) {
	clearOverlayEnv(t)
	t.Setenv("AUTH_MODE", "bogus")

	overlay, err := LoadHTTPEnvOverlay()
	if err != nil {
		t.Fatalf("LoadHTTPEnvOverlay() error: %v", err)
	}
	assertStr(t, "AuthMode", overlay.AuthMode, "bogus")
}

// TestLoadHTTPEnvOverlay_AuthModeSurfacesCacheTTLErrors verifies the branch
// where AUTH_MODE is present and OAUTH_CACHE_TTL is not parseable. Both read
// through the same loader, so an invalid TTL has to fail the load even when it
// is the mode that triggered it.
func TestLoadHTTPEnvOverlay_AuthModeSurfacesCacheTTLErrors(t *testing.T) {
	clearOverlayEnv(t)
	t.Setenv("AUTH_MODE", "oauth")
	t.Setenv("OAUTH_CACHE_TTL", "bogus")

	if _, err := LoadHTTPEnvOverlay(); err == nil {
		t.Error("LoadHTTPEnvOverlay() error = nil, want the invalid TTL to fail the load")
	}
}

// clearOverlayEnv unsets every variable the overlay reads, so a case only sees
// what it sets and the developer's own environment cannot leak into a result.
func clearOverlayEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"GITLAB_URL", "GITLAB_SKIP_TLS_VERIFY", "TOOL_SURFACE", "META_TOOLS",
		"CAPABILITY_SURFACE", "META_PARAM_SCHEMA", "GITLAB_TIER", "GITLAB_ENTERPRISE",
		"GITLAB_READ_ONLY", "GITLAB_SAFE_MODE", "EMBEDDED_RESOURCES",
		"GITLAB_IGNORE_SCOPES", "EXCLUDE_TOOLS", "MAX_HTTP_CLIENTS",
		"SESSION_TIMEOUT", "POOL_IDLE_TIMEOUT", "SESSION_REVALIDATE_INTERVAL",
		"ACTION_TIMEOUT",
		"AUTH_MODE", "PUBLIC_URL", "TRUSTED_ORIGINS", "OAUTH_CACHE_TTL",
		"RATE_LIMIT_RPS", "RATE_LIMIT_BURST",
	} {
		// t.Setenv registers the restore; unsetting afterwards makes the
		// variable genuinely absent rather than present-and-empty, which is
		// the state these tests claim to exercise.
		//
		// Both spellings: the prefixed one wins when both are set, so a
		// GITLAB_MCP_* value exported by the developer would override the
		// bare-name fixture a case sets and fail it for a reason outside the
		// test.
		for _, spelling := range []string{name, EnvPrefix + name} {
			t.Setenv(spelling, "")
			if err := os.Unsetenv(spelling); err != nil {
				t.Fatalf("unset %s: %v", spelling, err)
			}
		}
	}
}

func assertStr(t *testing.T, field string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %q", field, want)
	}
	if *got != want {
		t.Errorf("%s = %q, want %q", field, *got, want)
	}
}

func assertBool(t *testing.T, field string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", field, want)
	}
	if *got != want {
		t.Errorf("%s = %v, want %v", field, *got, want)
	}
}

func assertDur(t *testing.T, field string, got *time.Duration, want time.Duration) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", field, want)
	}
	if *got != want {
		t.Errorf("%s = %v, want %v", field, *got, want)
	}
}

// TestLoadHTTPEnvOverlay_PrefixedNamesArePresent verifies that HTTP mode's
// environment layer sees a prefixed variable at all.
//
// Every read in this file sits behind [envPresent], so the gate decides
// whether a value is read rather than the reader does. A gate looking only at
// the deprecated spelling would leave a migrated HTTP deployment falling back
// to flag defaults for every setting, with nothing reporting that its
// environment had been skipped.
func TestLoadHTTPEnvOverlay_PrefixedNamesArePresent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   string
		value string
		field string
	}{
		{name: "tool surface", env: "TOOL_SURFACE", value: ToolSurfaceIndividual, field: "ToolSurface"},
		{name: "capability surface", env: "CAPABILITY_SURFACE", value: CapabilitySurfaceMinimal, field: "CapabilitySurface"},
		{name: "trusted origins", env: "TRUSTED_ORIGINS", value: "https://claude.ai", field: "TrustedOrigins"},
		{name: "auth mode", env: "AUTH_MODE", value: "oauth", field: "AuthMode"},
		{name: "public url", env: "PUBLIC_URL", value: "https://mcp.example.com", field: "PublicURL"},
		{name: "pool idle timeout", env: "POOL_IDLE_TIMEOUT", value: "2h0m0s", field: "PoolIdleTimeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvPrefix+tc.env, tc.value)

			overlay, err := LoadHTTPEnvOverlay()
			if err != nil {
				t.Fatalf("LoadHTTPEnvOverlay() with %s set: %v", EnvPrefix+tc.env, err)
			}
			if got := overlaySetting(overlay, tc.field); got != tc.value {
				t.Errorf("%s = %s, want %q", EnvPrefix+tc.env, got, tc.value)
			}
		})
	}
}

// overlaySetting renders one overlay field by name, keeping the case table
// data rather than a closure per case. An absent field reads as a marker so it
// cannot be mistaken for a value that was read and found empty.
func overlaySetting(o *HTTPEnvOverlay, field string) string {
	const unset = "<unset>"
	switch field {
	case "ToolSurface":
		if o.ToolSurface == nil {
			return unset
		}
		return *o.ToolSurface
	case "CapabilitySurface":
		if o.CapabilitySurface == nil {
			return unset
		}
		return *o.CapabilitySurface
	case "TrustedOrigins":
		if o.TrustedOrigins == nil {
			return unset
		}
		return *o.TrustedOrigins
	case "AuthMode":
		if o.AuthMode == nil {
			return unset
		}
		return *o.AuthMode
	case "PublicURL":
		if o.PublicURL == nil {
			return unset
		}
		return *o.PublicURL
	case "PoolIdleTimeout":
		if o.PoolIdleTimeout == nil {
			return unset
		}
		return o.PoolIdleTimeout.String()
	default:
		return "<unknown field " + field + ">"
	}
}
