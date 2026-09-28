// http_overlay_test.go verifies the environment layer that sits underneath the
// HTTP-mode CLI flags.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
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
		"DrainDelay":         overlay.DrainDelay == nil,
		"AuthMode":           overlay.AuthMode == nil,
		"PublicURL":          overlay.PublicURL == nil,
		"TrustedOrigins":     overlay.TrustedOrigins == nil,
		"OAuthCacheTTL":      overlay.OAuthCacheTTL == nil,
		"OAuthClientUID":     overlay.OAuthClientUID == nil,
		"RateLimitRPS":       overlay.RateLimitRPS == nil,
		"RateLimitBurst":     overlay.RateLimitBurst == nil,

		"AuthFailureLimit":       overlay.AuthFailureLimit == nil,
		"AuthFailureWindow":      overlay.AuthFailureWindow == nil,
		"AuthDistinctTokenLimit": overlay.AuthDistinctTokenLimit == nil,
		"AuthDistinctWindow":     overlay.AuthDistinctWindow == nil,
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
	cases := []presentVariableCase{
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
				"GITLAB_MCP_PUBLIC_URL":      " https://mcp.example.com/gitlab ",
				"GITLAB_MCP_TRUSTED_ORIGINS": " https://claude.ai,https://inspector.example ",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "PublicURL", o.PublicURL, "https://mcp.example.com/gitlab")
				assertStr(t, "TrustedOrigins", o.TrustedOrigins, "https://claude.ai,https://inspector.example")
			},
		},
		{
			// The audit of the reference pages found this variable documented
			// for HTTP mode and read by nothing there: the overlay never
			// carried it, so only the flag admitted an application.
			name: "oauth client uid reaches the overlay verbatim",
			env:  map[string]string{"GITLAB_MCP_OAUTH_CLIENT_UID": " 12ab, 34cd "},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "OAuthClientUID", o.OAuthClientUID, "12ab, 34cd")
			},
		},
		{
			name: "tool surface resolves the canonical selector",
			env:  map[string]string{"GITLAB_MCP_TOOL_SURFACE": ToolSurfaceMeta},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "ToolSurface", o.ToolSurface, ToolSurfaceMeta)
			},
		},
		{
			name: "capability surface",
			env:  map[string]string{"GITLAB_MCP_CAPABILITY_SURFACE": CapabilitySurfaceMinimal},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "CapabilitySurface", o.CapabilitySurface, CapabilitySurfaceMinimal)
			},
		},
		{
			name: "meta param schema",
			env:  map[string]string{"GITLAB_MCP_META_PARAM_SCHEMA": MetaParamSchemaCompact},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "MetaParamSchema", o.MetaParamSchema, MetaParamSchemaCompact)
			},
		},
		{
			name: "an explicit tier is pinned",
			env:  map[string]string{"GITLAB_MCP_TIER": "ultimate"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.Tier == nil || *o.Tier != edition.Ultimate || !o.TierExplicit {
					t.Errorf("Tier = %v explicit = %v, want ultimate pinned", o.Tier, o.TierExplicit)
				}
			},
		},
		{
			name: "the retired enterprise flag resolves nothing",
			env:  map[string]string{"GITLAB_ENTERPRISE": "true"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.Tier != nil {
					t.Errorf("Tier = %v, want the overlay to leave it unset", *o.Tier)
				}
			},
		},
		{
			name: "exclude tools is carried verbatim for the flag layer to split",
			env:  map[string]string{"GITLAB_MCP_EXCLUDE_TOOLS": "gitlab_runner,gitlab_geo"},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertStr(t, "ExcludeTools", o.ExcludeTools, "gitlab_runner,gitlab_geo")
			},
		},
		{
			name: "booleans",
			env: map[string]string{
				"GITLAB_MCP_SKIP_TLS_VERIFY":    "true",
				"GITLAB_MCP_READ_ONLY":          "true",
				"GITLAB_MCP_SAFE_MODE":          "true",
				"GITLAB_MCP_EMBEDDED_RESOURCES": "false",
				"GITLAB_MCP_IGNORE_SCOPES":      "true",
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
				"GITLAB_MCP_MAX_HTTP_CLIENTS":            "250",
				"GITLAB_MCP_SESSION_TIMEOUT":             "10m",
				"GITLAB_MCP_POOL_IDLE_TIMEOUT":           "6h",
				"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL": "5m",
				"GITLAB_MCP_ACTION_TIMEOUT":              "20m",
				"GITLAB_MCP_DRAIN_DELAY":                 "5s",
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
				assertDur(t, "DrainDelay", o.DrainDelay, 5*time.Second)
			},
		},
		{
			name: "zero disables the two settings documented as disableable",
			env: map[string]string{
				"GITLAB_MCP_POOL_IDLE_TIMEOUT":           "0",
				"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL": "0",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertDur(t, "PoolIdleTimeout", o.PoolIdleTimeout, 0)
				assertDur(t, "RevalidateInterval", o.RevalidateInterval, 0)
			},
		},
		{
			name: "an explicit zero drain delay closes the listener at once",
			env: map[string]string{
				"GITLAB_MCP_DRAIN_DELAY": "0",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertDur(t, "DrainDelay", o.DrainDelay, 0)
			},
		},
		{
			name: "auth and rate limiting",
			env: map[string]string{
				"GITLAB_MCP_AUTH_MODE":        "oauth",
				"GITLAB_MCP_OAUTH_CACHE_TTL":  "30m",
				"GITLAB_MCP_RATE_LIMIT_RPS":   "12.5",
				"GITLAB_MCP_RATE_LIMIT_BURST": "80",
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
			env:  map[string]string{"GITLAB_MCP_AUTH_MODE": "   "},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				if o.AuthMode != nil {
					t.Errorf("AuthMode = %v, want nil for a blank value", *o.AuthMode)
				}
			},
		},
	}
	return append(cases, authBudgetPresentCases()...)
}

// authBudgetPresentCases are the authentication budgets' rows of the table
// above, kept apart so neither list grows past what a reader takes in at once.
// Each value and bound of one setting is held by
// TestReadAuthBudgetEnv_EachValue_AgreesWithTheStdioReader; these rows are
// about the four settings together.
func authBudgetPresentCases() []presentVariableCase {
	return []presentVariableCase{
		{
			// Four different values, so a budget read from its neighbor's
			// variable, or reported in its neighbor's field, cannot pass.
			name: "authentication budgets",
			env: map[string]string{
				"GITLAB_MCP_AUTH_FAILURE_LIMIT":         "4",
				"GITLAB_MCP_AUTH_FAILURE_WINDOW":        "30s",
				"GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT":  "9",
				"GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW": "5m",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertInt(t, "AuthFailureLimit", o.AuthFailureLimit, 4)
				assertDur(t, "AuthFailureWindow", o.AuthFailureWindow, 30*time.Second)
				assertInt(t, "AuthDistinctTokenLimit", o.AuthDistinctTokenLimit, 9)
				assertDur(t, "AuthDistinctWindow", o.AuthDistinctWindow, 5*time.Minute)
			},
		},
		{
			// The struct's promise: each half of a budget is overlaid on its
			// own, so a window exported alone leaves the limit beside it to the
			// flag or its default instead of reporting one nobody set.
			name: "an authentication budget's window is overlaid without its limit",
			env: map[string]string{
				"GITLAB_MCP_AUTH_FAILURE_WINDOW":        "2m",
				"GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW": "20m",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertDur(t, "AuthFailureWindow", o.AuthFailureWindow, 2*time.Minute)
				assertDur(t, "AuthDistinctWindow", o.AuthDistinctWindow, 20*time.Minute)
				if o.AuthFailureLimit != nil || o.AuthDistinctTokenLimit != nil {
					t.Errorf("limits = %v and %v, want both unreported when only the windows were set",
						o.AuthFailureLimit, o.AuthDistinctTokenLimit)
				}
			},
		},
		{
			name: "an authentication budget's limit is overlaid without its window",
			env: map[string]string{
				"GITLAB_MCP_AUTH_FAILURE_LIMIT":        "3",
				"GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT": "70",
			},
			assert: func(t *testing.T, o *HTTPEnvOverlay) {
				t.Helper()
				assertInt(t, "AuthFailureLimit", o.AuthFailureLimit, 3)
				assertInt(t, "AuthDistinctTokenLimit", o.AuthDistinctTokenLimit, 70)
				if o.AuthFailureWindow != nil || o.AuthDistinctWindow != nil {
					t.Errorf("windows = %v and %v, want both unreported when only the limits were set",
						o.AuthFailureWindow, o.AuthDistinctWindow)
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
		// wantMsg is the setting's name rather than the variable's for the
		// parsers shared with the flag layer: --tool-surface reaches the same
		// message, and naming an environment variable there would be wrong.
		{"GITLAB_MCP_TOOL_SURFACE", "bogus", "TOOL_SURFACE"},
		{"GITLAB_MCP_CAPABILITY_SURFACE", "bogus", "CAPABILITY_SURFACE"},
		{"GITLAB_MCP_META_PARAM_SCHEMA", "bogus", "META_PARAM_SCHEMA"},
		// The renamed switches used to be listed twice here, once under each
		// spelling, because both were read. 3.1.0 reads one, so the retired
		// spelling of each is gone from this table: a bogus value under it is
		// not an invalid value any more, it is a variable nobody consults.
		{"GITLAB_MCP_TIER", "bogus", "GITLAB_MCP_TIER"},
		{"GITLAB_MCP_SKIP_TLS_VERIFY", "bogus", "GITLAB_MCP_SKIP_TLS_VERIFY"},
		{"GITLAB_MCP_READ_ONLY", "bogus", "GITLAB_MCP_READ_ONLY"},
		{"GITLAB_MCP_SAFE_MODE", "bogus", "GITLAB_MCP_SAFE_MODE"},
		{"GITLAB_MCP_IGNORE_SCOPES", "bogus", "GITLAB_MCP_IGNORE_SCOPES"},
		{"GITLAB_MCP_MAX_HTTP_CLIENTS", "bogus", "MAX_HTTP_CLIENTS"},
		{"GITLAB_MCP_SESSION_TIMEOUT", "bogus", "SESSION_TIMEOUT"},
		{"GITLAB_MCP_POOL_IDLE_TIMEOUT", "bogus", "POOL_IDLE_TIMEOUT"},
		{"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "bogus", "SESSION_REVALIDATE_INTERVAL"},
		{"GITLAB_MCP_OAUTH_CACHE_TTL", "bogus", "OAUTH_CACHE_TTL"},
		{"GITLAB_MCP_RATE_LIMIT_RPS", "bogus", "RATE_LIMIT_RPS"},
		{"GITLAB_MCP_RATE_LIMIT_BURST", "bogus", "RATE_LIMIT_BURST"},
		{"GITLAB_MCP_POOL_IDLE_TIMEOUT", "48h", "exceeds maximum"},
		{"GITLAB_MCP_ACTION_TIMEOUT", "bogus", "ACTION_TIMEOUT"},
		{"GITLAB_MCP_ACTION_TIMEOUT", "48h", "exceeds maximum"},
		{"GITLAB_MCP_DRAIN_DELAY", "bogus", "DRAIN_DELAY"},
		{"GITLAB_MCP_DRAIN_DELAY", "10m", "exceeds maximum"},
		// The four authentication budgets, each refused both ways: a value
		// that does not parse, and one past the bound the stdio path holds it
		// to. Until these rows no case here set any of the four, so the
		// overlay's refusals of them ran in no test at all.
		{"GITLAB_MCP_AUTH_FAILURE_LIMIT", "bogus", "AUTH_FAILURE_LIMIT"},
		{"GITLAB_MCP_AUTH_FAILURE_LIMIT", "100001", "AUTH_FAILURE_LIMIT exceeds maximum"},
		{"GITLAB_MCP_AUTH_FAILURE_WINDOW", "bogus", "AUTH_FAILURE_WINDOW"},
		{"GITLAB_MCP_AUTH_FAILURE_WINDOW", "25h", "AUTH_FAILURE_WINDOW 25h0m0s exceeds maximum"},
		{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT", "bogus", "AUTH_DISTINCT_TOKEN_LIMIT"},
		{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT", "100001", "AUTH_DISTINCT_TOKEN_LIMIT exceeds maximum"},
		{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW", "bogus", "AUTH_DISTINCT_TOKEN_WINDOW"},
		{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW", "25h", "AUTH_DISTINCT_TOKEN_WINDOW 25h0m0s exceeds maximum"},
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
	t.Setenv("GITLAB_MCP_AUTH_MODE", "bogus")

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
	t.Setenv("GITLAB_MCP_AUTH_MODE", "oauth")
	t.Setenv("GITLAB_MCP_OAUTH_CACHE_TTL", "bogus")

	if _, err := LoadHTTPEnvOverlay(); err == nil {
		t.Error("LoadHTTPEnvOverlay() error = nil, want the invalid TTL to fail the load")
	}
}

// TestReadAuthBudgetEnv_EachValue_AgreesWithTheStdioReader holds the promise
// readAuthBudgetEnv makes: every authentication-budget variable is read with
// the parser and the bound the stdio path applies, so one exported value with
// something in it starts both transports or refuses both, with the same words.
//
// A value of only whitespace is outside that promise and outside this table.
// The overlay counts it as absent (envPresent trims first, which
// TestLoadHTTPEnvOverlay_PresentVariablesAreParsed pins), while the stdio
// parsers test for the empty string before they trim and so refuse it. Every
// typed setting of the package shares that divergence, and which of the two
// readings is right is a decision still to be made, so nothing here asserts
// either reading for the stdio half.
//
// The two readers are written out separately, which is how the HTTP half came
// to be read by code no test ran. Each case sets one variable and asks both,
// so a bound that drifts in either copy fails here, including one that starts
// refusing the published maximum itself: every limit is tried at exactly its
// maximum and one above, and every window at its maximum and one second over.
func TestReadAuthBudgetEnv_EachValue_AgreesWithTheStdioReader(t *testing.T) {
	for _, tc := range authBudgetAgreementCases() {
		t.Run(tc.setting.env+"="+tc.value, func(t *testing.T) {
			clearOverlayEnv(t)
			t.Setenv(EnvPrefix+tc.setting.env, tc.value)

			stdio, stdioErr := loadAuthBudgetEnv()
			overlay, httpErr := LoadHTTPEnvOverlay()
			if tc.refuse {
				assertRefusedAlike(t, tc.setting.env, stdioErr, httpErr)
				return
			}
			if stdioErr != nil || httpErr != nil {
				t.Fatalf("stdio error = %v, HTTP error = %v; want both to accept %s=%q",
					stdioErr, httpErr, EnvPrefix+tc.setting.env, tc.value)
			}
			if got := tc.setting.stdio(stdio); got != tc.want {
				t.Errorf("stdio read %s, want %s", got, tc.want)
			}
			if got := tc.setting.http(overlay); got != tc.want {
				t.Errorf("HTTP read %s, want %s", got, tc.want)
			}
		})
	}
}

// authBudgetSetting is one authentication-budget variable and where each
// reader reports it, rendered as text so a limit and a window compare alike.
type authBudgetSetting struct {
	env   string
	stdio func(authBudgetEnv) string
	http  func(*HTTPEnvOverlay) string
}

// authBudgetAgreementCase is one value of one setting, and what both readers
// have to make of it.
type authBudgetAgreementCase struct {
	setting authBudgetSetting
	value   string
	want    string
	refuse  bool
}

// authBudgetAgreementCases tries each setting on both sides of its own bound:
// zero, an ordinary value and the maximum are accepted, and one step past the
// maximum, a negative value and a value that does not parse are refused.
func authBudgetAgreementCases() []authBudgetAgreementCase {
	limits := []struct {
		setting  authBudgetSetting
		maxValue int
	}{
		{
			setting: authBudgetSetting{
				env:   "AUTH_FAILURE_LIMIT",
				stdio: func(b authBudgetEnv) string { return strconv.Itoa(b.failureLimit) },
				http:  func(o *HTTPEnvOverlay) string { return overlayReading(o.AuthFailureLimit) },
			},
			maxValue: MaxAuthFailureLimit,
		},
		{
			setting: authBudgetSetting{
				env:   "AUTH_DISTINCT_TOKEN_LIMIT",
				stdio: func(b authBudgetEnv) string { return strconv.Itoa(b.distinctLimit) },
				http:  func(o *HTTPEnvOverlay) string { return overlayReading(o.AuthDistinctTokenLimit) },
			},
			maxValue: MaxAuthDistinctTokenLimit,
		},
	}
	windows := []struct {
		setting  authBudgetSetting
		maxValue time.Duration
	}{
		{
			setting: authBudgetSetting{
				env:   "AUTH_FAILURE_WINDOW",
				stdio: func(b authBudgetEnv) string { return fmt.Sprint(b.failureWindow) },
				http:  func(o *HTTPEnvOverlay) string { return overlayReading(o.AuthFailureWindow) },
			},
			maxValue: MaxAuthFailureWindow,
		},
		{
			setting: authBudgetSetting{
				env:   "AUTH_DISTINCT_TOKEN_WINDOW",
				stdio: func(b authBudgetEnv) string { return fmt.Sprint(b.distinctWindow) },
				http:  func(o *HTTPEnvOverlay) string { return overlayReading(o.AuthDistinctWindow) },
			},
			maxValue: MaxAuthDistinctWindow,
		},
	}

	var cases []authBudgetAgreementCase
	for _, l := range limits {
		top := strconv.Itoa(l.maxValue)
		cases = append(cases,
			authBudgetAgreementCase{setting: l.setting, value: "0", want: "0"},
			authBudgetAgreementCase{setting: l.setting, value: "7", want: "7"},
			authBudgetAgreementCase{setting: l.setting, value: top, want: top},
			authBudgetAgreementCase{setting: l.setting, value: strconv.Itoa(l.maxValue + 1), refuse: true},
			authBudgetAgreementCase{setting: l.setting, value: "-1", refuse: true},
			authBudgetAgreementCase{setting: l.setting, value: "ten", refuse: true},
		)
	}
	for _, w := range windows {
		top := w.maxValue.String()
		cases = append(cases,
			authBudgetAgreementCase{setting: w.setting, value: "0", want: "0s"},
			authBudgetAgreementCase{setting: w.setting, value: "45s", want: "45s"},
			authBudgetAgreementCase{setting: w.setting, value: top, want: top},
			authBudgetAgreementCase{setting: w.setting, value: (w.maxValue + time.Second).String(), refuse: true},
			authBudgetAgreementCase{setting: w.setting, value: "-1s", refuse: true},
			authBudgetAgreementCase{setting: w.setting, value: "soon", refuse: true},
		)
	}
	return cases
}

// overlayReading renders an overlay field the way the stdio reader's value is
// rendered, and an unreported one as a marker no reading can equal.
func overlayReading[T int | time.Duration](v *T) string {
	if v == nil {
		return "<unset>"
	}
	return fmt.Sprint(*v)
}

// assertRefusedAlike holds the two readers to one refusal of name: both
// refuse, in the same words, and the words name the variable.
func assertRefusedAlike(t *testing.T, name string, stdioErr, httpErr error) {
	t.Helper()
	if stdioErr == nil || httpErr == nil {
		t.Fatalf("stdio error = %v, HTTP error = %v; want both to refuse %s", stdioErr, httpErr, name)
	}
	if stdioErr.Error() != httpErr.Error() {
		t.Errorf("the two transports refuse in different words:\n stdio: %s\n HTTP:  %s", stdioErr, httpErr)
	}
	if !strings.Contains(httpErr.Error(), name) {
		t.Errorf("error = %q, want it to name %s", httpErr, name)
	}
}

// clearOverlayEnv unsets every variable the overlay reads, so a case only sees
// what it sets and the developer's own environment cannot leak into a result.
func clearOverlayEnv(t *testing.T) {
	t.Helper()
	// Derived from the list rather than restated, so a setting added there
	// cannot leave a value of the developer's own standing in a case that
	// claims the variable is unset. The hand-written list this replaced still
	// named META_TOOLS and GITLAB_ENTERPRISE, which 3.0.0 removed.
	for _, name := range append(PrefixedEnvNames(), "GITLAB_URL") {
		// t.Setenv registers the restore; unsetting afterwards makes the
		// variable genuinely absent rather than present-and-empty, which is
		// the state these tests claim to exercise.
		//
		// Both spellings: only the prefixed one is read, and the retired one
		// is cleared because a case that asserts nothing was reported would
		// otherwise be answered by a report about the developer's shell.
		for _, spelling := range []string{RetiredEnvName(name), EnvPrefix + name} {
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

func assertInt(t *testing.T, field string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %d", field, want)
	}
	if *got != want {
		t.Errorf("%s = %d, want %d", field, *got, want)
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
