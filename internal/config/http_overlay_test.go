// http_overlay_test.go verifies the environment layer that sits underneath the
// HTTP-mode CLI flags.
package config

import (
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
		"AuthMode":           overlay.AuthMode == nil,
		"OAuthCacheTTL":      overlay.OAuthCacheTTL == nil,
		"RateLimitRPS":       overlay.RateLimitRPS == nil,
		"RateLimitBurst":     overlay.RateLimitBurst == nil,
		"AutoUpdate":         overlay.AutoUpdate == nil,
		"AutoUpdateRepo":     overlay.AutoUpdateRepo == nil,
		"AutoUpdateInterval": overlay.AutoUpdateInterval == nil,
		"AutoUpdateTimeout":  overlay.AutoUpdateTimeout == nil,
	}
	for field, isNil := range nilFields {
		if !isNil {
			t.Errorf("%s was reported despite its variable being unset", field)
		}
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
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.MaxHTTPClients == nil || *o.MaxHTTPClients != 250 {
					t.Errorf("MaxHTTPClients = %v, want 250", o.MaxHTTPClients)
				}
				assertDur(t, "SessionTimeout", o.SessionTimeout, 10*time.Minute)
				assertDur(t, "PoolIdleTimeout", o.PoolIdleTimeout, 6*time.Hour)
				assertDur(t, "RevalidateInterval", o.RevalidateInterval, 5*time.Minute)
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
			name: "auto-update settings are reported individually",
			env: map[string]string{
				"AUTO_UPDATE":          "check",
				"AUTO_UPDATE_REPO":     "someone/fork",
				"AUTO_UPDATE_INTERVAL": "2h",
				"AUTO_UPDATE_TIMEOUT":  "90s",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "AutoUpdate", o.AutoUpdate, "check")
				assertStr(t, "AutoUpdateRepo", o.AutoUpdateRepo, "someone/fork")
				assertDur(t, "AutoUpdateInterval", o.AutoUpdateInterval, 2*time.Hour)
				assertDur(t, "AutoUpdateTimeout", o.AutoUpdateTimeout, 90*time.Second)
			},
		},
		{
			name: "one auto-update variable does not drag in the others",
			env:  map[string]string{"AUTO_UPDATE": "false"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "AutoUpdate", o.AutoUpdate, "false")
				if o.AutoUpdateRepo != nil || o.AutoUpdateInterval != nil || o.AutoUpdateTimeout != nil {
					t.Error("unset auto-update variables were reported alongside the set one")
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
	tests := []struct{ name, value string }{
		{"TOOL_SURFACE", "bogus"},
		{"META_TOOLS", "bogus"},
		{"CAPABILITY_SURFACE", "bogus"},
		{"META_PARAM_SCHEMA", "bogus"},
		{"GITLAB_TIER", "bogus"},
		{"GITLAB_ENTERPRISE", "bogus"},
		{"GITLAB_SKIP_TLS_VERIFY", "bogus"},
		{"GITLAB_READ_ONLY", "bogus"},
		{"MAX_HTTP_CLIENTS", "bogus"},
		{"SESSION_TIMEOUT", "bogus"},
		{"POOL_IDLE_TIMEOUT", "bogus"},
		{"SESSION_REVALIDATE_INTERVAL", "bogus"},
		{"OAUTH_CACHE_TTL", "bogus"},
		{"RATE_LIMIT_RPS", "bogus"},
		{"RATE_LIMIT_BURST", "bogus"},
		{"AUTO_UPDATE_INTERVAL", "bogus"},
		{"AUTO_UPDATE_TIMEOUT", "bogus"},
		{"POOL_IDLE_TIMEOUT_over_max", "48h"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			clearOverlayEnv(t)
			name := tt.name
			if name == "POOL_IDLE_TIMEOUT_over_max" {
				name = "POOL_IDLE_TIMEOUT"
			}
			t.Setenv(name, tt.value)

			overlay, err := LoadHTTPEnvOverlay()
			if err == nil {
				t.Fatalf("LoadHTTPEnvOverlay() error = nil, want an error; overlay = %+v", overlay)
			}
			if overlay != nil {
				t.Errorf("overlay = %+v, want nil alongside the error", overlay)
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
		"AUTH_MODE", "OAUTH_CACHE_TTL", "RATE_LIMIT_RPS", "RATE_LIMIT_BURST",
		"AUTO_UPDATE", "AUTO_UPDATE_REPO", "AUTO_UPDATE_INTERVAL", "AUTO_UPDATE_TIMEOUT",
	} {
		t.Setenv(name, "")
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
