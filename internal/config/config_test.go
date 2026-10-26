// config_test.go contains unit tests for the config package.
// Tests verify [Load] behavior with valid configuration, missing required
// fields, and invalid boolean environment variable values.
package config

import (
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// Test fixtures used across configuration tests.
const (
	testGitLabURL      = "https://gitlab.example.com"
	testGitLabToken    = "test-token-abc"
	fmtLoadUnexpected  = "Load() unexpected error: %v"
	fmtLoadErr         = "Load() error: %v"
	testHTTPExampleURL = "http://example.com"
	subtestDefault     = "default value"
	subtestCustom      = "custom value"
	subtestInvalid     = "invalid value"
)

// TestMain redirects HOME and USERPROFILE to a throwaway temp directory for the
// duration of the package tests so config loading never reads or writes the real
// user home, then removes the directory and propagates the test exit code.
func TestMain(m *testing.M) {
	homeDir, err := os.MkdirTemp("", "gitlab-mcp-config-test-home-")
	if err != nil {
		panic(err)
	}

	if setErr := os.Setenv("HOME", homeDir); setErr != nil {
		panic(setErr)
	}
	if setErr := os.Setenv("USERPROFILE", homeDir); setErr != nil {
		panic(setErr)
	}

	code := m.Run()
	_ = os.RemoveAll(homeDir)
	os.Exit(code)
}

// TestLoad_ValidConfig verifies that [Load] returns a fully populated [Config]
// when all required environment variables are set with valid values.
func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}

	if cfg.GitLabURL != testGitLabURL {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, testGitLabURL)
	}
	if cfg.GitLabToken != testGitLabToken {
		t.Errorf("GitLabToken = %q, want %q", cfg.GitLabToken, testGitLabToken)
	}
	if cfg.SkipTLSVerify != false {
		t.Errorf("SkipTLSVerify = %v, want false", cfg.SkipTLSVerify)
	}
}

// TestLoad_DefaultGitLabURLWhenUnset verifies that [Load] uses GitLab.com when
// GITLAB_URL is not configured.
func TestLoad_DefaultGitLabURLWhenUnset(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "whitespace", value: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", tt.value)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)

			cfg, err := Load()
			if err != nil {
				t.Fatalf(fmtLoadUnexpected, err)
			}
			if cfg.GitLabURL != DefaultGitLabURL {
				t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, DefaultGitLabURL)
			}
		})
	}
}

// TestLoad_InvalidExplicitGitLabURL verifies that malformed explicit
// GITLAB_URL values are rejected instead of being replaced by the default.
func TestLoad_InvalidExplicitGitLabURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "missing scheme", value: "gitlab.example.com", wantErr: "must use http:// or https://"},
		{name: "unsupported scheme", value: "ftp://gitlab.example.com", wantErr: "must use http:// or https://"},
		{name: "missing host", value: "https://", wantErr: "must include a host"},
		{name: "malformed URL", value: "http://[::1", wantErr: "is not a valid URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", tt.value)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() expected error for invalid GITLAB_URL, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Load() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestLoad_MissingToken verifies that [Load] returns an error when GITLAB_TOKEN
// is empty, since it is a required configuration field.
func TestLoad_MissingToken(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error when GITLAB_TOKEN is empty, got nil")
	}
}

// TestLoad_SkipTLSVerifyTrue verifies that [Load] correctly parses
// GITLAB_MCP_SKIP_TLS_VERIFY="true" and sets [Config.SkipTLSVerify] to true.
func TestLoad_SkipTLSVerifyTrue(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if !cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify = false, want true")
	}
}

// TestLoad_SkipTLSVerifyInvalid verifies that [Load] returns an error when
// GITLAB_MCP_SKIP_TLS_VERIFY contains a non-boolean string.
func TestLoad_SkipTLSVerifyInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid GITLAB_MCP_SKIP_TLS_VERIFY, got nil")
	}
}

// TestLoad_DefaultToolSurface verifies the empty selector path uses the
// low-token dynamic surface.
func TestLoad_DefaultToolSurface(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_TOOL_SURFACE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.ToolSurface != ToolSurfaceDynamic {
		t.Fatalf("ToolSurface = %q, want %q", cfg.ToolSurface, ToolSurfaceDynamic)
	}
}

// TestLoad_ToolSurfaceInvalid verifies that Load rejects an unsupported
// tool surface value.
func TestLoad_ToolSurfaceInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_TOOL_SURFACE", "not-a-surface")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid TOOL_SURFACE, got nil")
	}
}

// TestLoad_ToolSurfaceDynamicCandidates verifies that dynamic selector aliases
// are accepted as explicit low-token surface spellings.
func TestLoad_ToolSurfaceDynamicCandidates(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "DYNAMIC", want: ToolSurfaceDynamic},
		{value: " dynamic ", want: ToolSurfaceDynamic},
		{value: "low-token", want: ToolSurfaceDynamic},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv("GITLAB_MCP_TOOL_SURFACE", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf(fmtLoadUnexpected, err)
			}
			if cfg.ToolSurface != tt.want {
				t.Fatalf("ToolSurface = %q, want %q", cfg.ToolSurface, tt.want)
			}
		})
	}
}

// TestLoad_CapabilitySurfaceMinimal verifies that CAPABILITY_SURFACE selects
// the non-default low-token resource and prompt surface.
func TestLoad_CapabilitySurfaceMinimal(t *testing.T) {
	tests := []string{"minimal", "MINIMAL", " minimal "}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv("GITLAB_MCP_CAPABILITY_SURFACE", value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf(fmtLoadUnexpected, err)
			}
			if cfg.CapabilitySurface != CapabilitySurfaceMinimal {
				t.Fatalf("CapabilitySurface = %q, want %q", cfg.CapabilitySurface, CapabilitySurfaceMinimal)
			}
		})
	}
}

// TestLoad_CapabilitySurfaceInvalid verifies unsupported capability surfaces
// are rejected during environment configuration loading.
func TestLoad_CapabilitySurfaceInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_CAPABILITY_SURFACE", "everything")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid CAPABILITY_SURFACE, got nil")
	}
}

// TestEffectiveCapabilitySurface verifies that an unset or unrecognized
// capability setting resolves to the default surface while each canonical
// value is preserved.
//
// The full arm is asserted explicitly rather than left to the default: both
// return the same string, so a switch that had lost the arm would still
// answer correctly for "full" and wrongly for anything else the arm ever
// grows to cover.
func TestEffectiveCapabilitySurface(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty falls back to the default", input: "", want: DefaultCapabilitySurface},
		{name: "unrecognized falls back to the default", input: "compact", want: DefaultCapabilitySurface},
		{name: "the canonical full is preserved", input: CapabilitySurfaceFull, want: CapabilitySurfaceFull},
		{name: "the canonical minimal is preserved", input: CapabilitySurfaceMinimal, want: CapabilitySurfaceMinimal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveCapabilitySurface(tt.input); got != tt.want {
				t.Errorf("EffectiveCapabilitySurface(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateSurfaces_AcceptEveryCanonicalValue verifies the two validators
// a configuration passes through accept each surface they document and refuse
// anything else by name.
//
// Only one arm of each switch was ever taken, so a validator that had lost
// the individual or the full arm would have refused a documented
// configuration at startup with the list of values it had just rejected.
func TestValidateSurfaces_AcceptEveryCanonicalValue(t *testing.T) {
	t.Run("tool surface", func(t *testing.T) {
		for _, surface := range []string{"", ToolSurfaceMeta, ToolSurfaceIndividual, ToolSurfaceDynamic} {
			t.Run(surface, func(t *testing.T) {
				if err := validateToolSurface(surface); err != nil {
					t.Errorf("validateToolSurface(%q) = %v, want it accepted", surface, err)
				}
			})
		}
		err := validateToolSurface("individual-tools")
		if err == nil {
			t.Fatal("validateToolSurface(alias) = nil, want a refusal: validation runs on the resolved surface, not the operator's spelling")
		}
		if !strings.Contains(err.Error(), "individual-tools") {
			t.Errorf("error %q does not name the value it refused", err)
		}
	})

	t.Run("capability surface", func(t *testing.T) {
		for _, surface := range []string{"", CapabilitySurfaceFull, CapabilitySurfaceMinimal} {
			t.Run(surface, func(t *testing.T) {
				if err := validateCapabilitySurface(surface); err != nil {
					t.Errorf("validateCapabilitySurface(%q) = %v, want it accepted", surface, err)
				}
			})
		}
		err := validateCapabilitySurface("minimum")
		if err == nil {
			t.Fatal("validateCapabilitySurface(alias) = nil, want a refusal")
		}
		if !strings.Contains(err.Error(), "minimum") {
			t.Errorf("error %q does not name the value it refused", err)
		}
	})
}

// TestLoad_MetaParamSchemaDefault verifies that [Load] defaults
// MetaParamSchema to "opaque" when the env var is unset.
func TestLoad_MetaParamSchemaDefault(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	// Pin META_PARAM_SCHEMA to empty so a value loaded from a developer's
	// .env file cannot override the default-case assertion below.
	t.Setenv("GITLAB_MCP_META_PARAM_SCHEMA", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.MetaParamSchema != MetaParamSchemaOpaque {
		t.Errorf("MetaParamSchema = %q, want %q", cfg.MetaParamSchema, MetaParamSchemaOpaque)
	}
}

// TestLoad_MetaParamSchemaValid verifies that [Load] accepts the three
// documented values for META_PARAM_SCHEMA, case-insensitively.
func TestLoad_MetaParamSchemaValid(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"opaque", MetaParamSchemaOpaque},
		{"COMPACT", MetaParamSchemaCompact},
		{"Full", MetaParamSchemaFull},
		{" full ", MetaParamSchemaFull},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv("GITLAB_MCP_META_PARAM_SCHEMA", tc.input)

			cfg, err := Load()
			if err != nil {
				t.Fatalf(fmtLoadUnexpected, err)
			}
			if cfg.MetaParamSchema != tc.want {
				t.Errorf("MetaParamSchema = %q, want %q", cfg.MetaParamSchema, tc.want)
			}
		})
	}
}

// TestLoad_MetaParamSchemaInvalid verifies that [Load] rejects
// META_PARAM_SCHEMA values outside the allowed set.
func TestLoad_MetaParamSchemaInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_META_PARAM_SCHEMA", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid META_PARAM_SCHEMA, got nil")
	}
}

// Transport and HTTP addr are now CLI flags, not env vars.

// TestLoad_UploadDefaults verifies upload config defaults when env vars are unset.
func TestLoad_UploadDefaults(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.UploadMaxFileSize != DefaultMaxFileSize {
		t.Errorf("UploadMaxFileSize = %d, want %d", cfg.UploadMaxFileSize, DefaultMaxFileSize)
	}
}

// TestLoad_UploadHumanFriendlySizes verifies parsing of human-friendly size values.
func TestLoad_UploadHumanFriendlySizes(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_UPLOAD_MAX_FILE_SIZE", "5GB")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.UploadMaxFileSize != 5*1024*1024*1024 {
		t.Errorf("UploadMaxFileSize = %d, want %d", cfg.UploadMaxFileSize, int64(5*1024*1024*1024))
	}
}

// TestLoad_UploadRawBytes verifies parsing of raw byte values.
func TestLoad_UploadRawBytes(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_UPLOAD_MAX_FILE_SIZE", "2147483648")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.UploadMaxFileSize != 2147483648 {
		t.Errorf("UploadMaxFileSize = %d, want 2147483648", cfg.UploadMaxFileSize)
	}
}

// TestLoad_UploadInvalidSize verifies parseSize returns error for non-numeric input.
func TestLoad_UploadInvalidSize(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_UPLOAD_MAX_FILE_SIZE", "notanumber")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid size value")
	}
}

// TestParseSize_CaseInsensitive verifies parseSize handles case variations.
func TestParseSize_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"10mb", 10 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"10Mb", 10 * 1024 * 1024},
		{"2gb", 2 * 1024 * 1024 * 1024},
		{"512kb", 512 * 1024},
		{"1024", 1024},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseSize(tt.input, 0)
			if err != nil {
				t.Fatalf("parseSize(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseSize_RefusesASizeOfNothing verifies that a size of zero, and a
// negative one, are refused rather than accepted as a ceiling.
//
// A ceiling of zero would refuse every upload and every local file read with
// "too large", which reads as a broken server rather than as a setting the
// operator wrote. Only non-numeric input was refused before, so the boundary
// between "no ceiling was asked for" and "a ceiling of nothing" was never
// tested at all.
func TestParseSize_RefusesASizeOfNothing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    int64
	}{
		{name: "zero", input: "0", wantErr: true},
		{name: "zero with a unit", input: "0MB", wantErr: true},
		{name: "negative", input: "-1", wantErr: true},
		{name: "one byte is a ceiling, however small", input: "1", want: 1},
		{name: "empty falls back to the default", input: "", want: 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSize(tt.input, 4096)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSize(%q) = %d, want a refusal", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSize(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidate_RateLimitBurstOfOne_IsAccepted verifies the smallest burst a
// rate-limited deployment may configure.
//
// A burst is a token bucket's depth, so one is the smallest that admits any
// request at all, and it is the value an operator writes to say "no burst,
// just the rate". Only zero and the ceiling were asserted, which leaves the
// bound free to drift onto the one value between them.
func TestValidate_RateLimitBurstOfOne_IsAccepted(t *testing.T) {
	cfg := &Config{
		GitLabURL:      testGitLabURL,
		GitLabToken:    testGitLabToken,
		RateLimitRPS:   10,
		RateLimitBurst: 1,
	}
	if err := cfg.validate(); err != nil {
		t.Errorf("validate() refused a burst of 1 beside a positive RPS: %v", err)
	}

	cfg.RateLimitBurst = 0
	if err := cfg.validate(); err == nil {
		t.Error("validate() accepted a burst of 0 beside a positive RPS, which admits nothing")
	}
}

// TestParseInt verifies parseInt handles valid values, defaults, and errors.
func TestParseInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		def     int
		want    int
		wantErr bool
	}{
		{"empty returns default", "", 42, 42, false},
		{"valid integer", "10", 0, 10, false},
		{"whitespace trimmed", "  25  ", 0, 25, false},
		{"zero is rejected", "0", 1, 0, true},
		{"negative is rejected", "-5", 1, 0, true},
		{"non-numeric is rejected", "abc", 1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt(tt.input, tt.def)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseInt(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseDuration verifies parseDuration handles valid durations, defaults, and errors.
func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		def     time.Duration
		want    time.Duration
		wantErr bool
	}{
		{"empty returns default", "", 10 * time.Minute, 10 * time.Minute, false},
		{"valid duration", "5m", 0, 5 * time.Minute, false},
		{"hours", "2h", 0, 2 * time.Hour, false},
		{"whitespace trimmed", "  30s  ", 0, 30 * time.Second, false},
		{"zero is rejected", "0s", time.Minute, 0, true},
		{"negative is rejected", "-1m", time.Minute, 0, true},
		{"invalid format", "notaduration", time.Minute, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input, tt.def)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// httpOnlyValuesAStartRefuses gives every setting only HTTP mode has a value
// an HTTP start refuses, where the setting has one: oauth without the public
// URL it requires, a URL ending in a slash, an origin that is not one, and a
// value each parser or bound refuses for the rest. OAUTH_CLIENT_UID takes any
// list, so it is given one. A stdio start used to refuse most of these values
// too, on settings it never used (issue 1171).
var httpOnlyValuesAStartRefuses = map[string]string{
	"ALLOW_PLAINTEXT_HTTP":       "maybe",
	"AUTH_DISTINCT_TOKEN_LIMIT":  "many",
	"AUTH_DISTINCT_TOKEN_WINDOW": "soon",
	"AUTH_FAILURE_LIMIT":         "-1",
	"AUTH_FAILURE_WINDOW":        "25h",
	"AUTH_MODE":                  AuthModeOAuth,
	"DISCOVERY_RATE_LIMIT_BURST": "-1",
	"DISCOVERY_RATE_LIMIT_RPS":   "NaN",
	"DRAIN_DELAY":                "1h",
	"MAX_HTTP_CLIENTS":           "0",
	"OAUTH_CACHE_TTL":            "0",
	"OAUTH_CLIENT_UID":           "12ab,34cd",
	"POOL_IDLE_TIMEOUT":          "-1m",
	"PUBLIC_URL":                 "https://mcp.example.com/",
	"SESSION_TIMEOUT":            "forever",
	"TRUSTED_ORIGINS":            "not an origin",
}

// loadWithNothingSet is what [Load] returns from an environment that sets no
// setting this project defines, the configuration every case below compares
// against.
func loadWithNothingSet(t *testing.T) Config {
	t.Helper()
	clearOverlayEnv(t)
	t.Setenv(EnvFileVar, "")
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	return *cfg
}

// TestLoad_HTTPOnlySettings_AreNotRead pins what a stdio server makes of a
// setting only HTTP mode has: nothing, whatever its value (issue 1171). Each is
// set, alone and then all of them together, to a value an HTTP start refuses,
// and Load must succeed and load exactly what it loads with none of them set.
//
// It used to parse most of them and refuse to start, so a variable the same
// shell exported for an HTTP deployment beside it stopped a stdio process
// that would never have used it: GITLAB_MCP_AUTH_MODE=oauth without a public
// URL did, naming --public-url, and so did GITLAB_MCP_MAX_HTTP_CLIENTS=0. The
// ones it parsed and accepted still landed in fields nothing on stdio read.
func TestLoad_HTTPOnlySettings_AreNotRead(t *testing.T) {
	if got := slices.Sorted(maps.Keys(httpOnlyValuesAStartRefuses)); !slices.Equal(got, httpOnlyNames) {
		t.Fatalf("this test gives values to %v, and the HTTP-only settings are %v; give each one a value", got, httpOnlyNames)
	}
	for _, name := range httpOnlyNames {
		t.Run(name, func(t *testing.T) {
			want := loadWithNothingSet(t)
			t.Setenv(EnvPrefix+name, httpOnlyValuesAStartRefuses[name])
			assertLoadsAsIfUnset(t, want, EnvPrefix+name)
		})
	}
	t.Run("all of them", func(t *testing.T) {
		want := loadWithNothingSet(t)
		for name, value := range httpOnlyValuesAStartRefuses {
			t.Setenv(EnvPrefix+name, value)
		}
		assertLoadsAsIfUnset(t, want, "every HTTP-only setting")
	})
}

// assertLoadsAsIfUnset holds Load to the configuration it loads when nothing
// is set, with what was set named in the failure.
func assertLoadsAsIfUnset(t *testing.T, want Config, set string) {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with %s set refused to start: %v", set, err)
	}
	if !reflect.DeepEqual(*cfg, want) {
		t.Errorf("Load() with %s set loaded %+v, want what it loads with nothing set, %+v", set, *cfg, want)
	}
}

// TestHTTPOnlyNames_AreTheSettingsOnlyTheOverlayReads holds [httpOnlyNames]
// to the two readers, by what each does rather than by a second list: a
// setting is on it exactly when [LoadHTTPEnvOverlay] reads it and [Load] does
// not. A setting HTTP mode gains would otherwise be read by neither stdio nor
// the line that says stdio ignores it, and one stdio starts reading would be
// named at startup as ignored while it configures the run.
//
// Each setting this project defines is set, alone, to one value no parser
// accepts, and a reader takes notice of it when it refuses the value or
// reports something it does not report with nothing set. The second is what
// catches a reader that keeps the value as written and so accepts it, such as
// the overlay's AUTH_MODE, PUBLIC_URL, TRUSTED_ORIGINS and OAUTH_CLIENT_UID
// and Load's EXCLUDE_TOOLS. The list is also held to name only settings this
// project defines, in name order, which is the order [HTTPOnlyEnvUses]
// answers in.
func TestHTTPOnlyNames_AreTheSettingsOnlyTheOverlayReads(t *testing.T) {
	if !slices.IsSorted(httpOnlyNames) {
		t.Errorf("httpOnlyNames is not in name order: %v", httpOnlyNames)
	}
	for _, name := range httpOnlyNames {
		if !slices.Contains(prefixedNames, name) {
			t.Errorf("httpOnlyNames lists %s, which is not a setting this project defines", name)
		}
	}
	for _, name := range PrefixedEnvNames() {
		t.Run(name, func(t *testing.T) {
			overlayReads, loadReads := readersOf(t, name)
			want := overlayReads && !loadReads
			if listed := slices.Contains(httpOnlyNames, name); listed != want {
				t.Errorf("%s: on httpOnlyNames %v, want %v, since the HTTP overlay reads it %v and Load reads it %v",
					name, listed, want, overlayReads, loadReads)
			}
		})
	}
}

// readersOf reports which of the two readers take notice of a setting set to a
// value no parser accepts: by refusing it, or by reporting what they do not
// report with nothing set.
func readersOf(t *testing.T, name string) (overlay, load bool) {
	t.Helper()
	baseline := loadWithNothingSet(t)
	baseOverlay, err := LoadHTTPEnvOverlay()
	if err != nil {
		t.Fatalf("LoadHTTPEnvOverlay() with nothing set: %v", err)
	}

	t.Setenv(EnvPrefix+name, "unreadable-1171")
	got, err := LoadHTTPEnvOverlay()
	overlay = err != nil || !reflect.DeepEqual(got, baseOverlay)
	cfg, err := Load()
	load = err != nil || !reflect.DeepEqual(*cfg, baseline)
	return overlay, load
}

// TestHTTPOnlyEnvUses_NamesWhatTheEnvironmentSets covers what a stdio run is
// handed to name: the HTTP-only settings set to something, under the name an
// operator exports, in name order, and none of the settings stdio reads. A
// value of blanks is unset, as it is to the overlay, and the bare spelling a
// setting had before 3.1.0 is nobody's setting here: [RetiredEnvUses] answers
// for it.
func TestHTTPOnlyEnvUses_NamesWhatTheEnvironmentSets(t *testing.T) {
	every := make([]string, 0, len(httpOnlyNames))
	for _, name := range httpOnlyNames {
		every = append(every, EnvPrefix+name)
	}
	cases := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{name: "nothing set"},
		{
			name: "only settings stdio reads",
			env:  map[string]string{EnvPrefix + "TOOL_SURFACE": ToolSurfaceMeta, EnvPrefix + "SESSION_REVALIDATE_INTERVAL": "5m"},
		},
		{
			name: "two of them beside a setting stdio reads, and a blank one",
			env: map[string]string{
				EnvPrefix + "SESSION_TIMEOUT":  "  ",
				EnvPrefix + "MAX_HTTP_CLIENTS": "0",
				EnvPrefix + "AUTH_MODE":        AuthModeOAuth,
				EnvPrefix + "TOOL_SURFACE":     ToolSurfaceMeta,
			},
			want: []string{EnvPrefix + "AUTH_MODE", EnvPrefix + "MAX_HTTP_CLIENTS"},
		},
		{name: "a retired bare spelling", env: map[string]string{"AUTH_MODE": AuthModeOAuth}},
		{name: "every one of them", env: prefixedValues(httpOnlyValuesAStartRefuses), want: every},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearOverlayEnv(t)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if got := HTTPOnlyEnvUses(); !slices.Equal(got, tc.want) {
				t.Errorf("HTTPOnlyEnvUses() = %v, want %v", got, tc.want)
			}
		})
	}
}

// prefixedValues keys each value by the setting's name as an operator exports
// it.
func prefixedValues(values map[string]string) map[string]string {
	prefixed := make(map[string]string, len(values))
	for name, value := range values {
		prefixed[EnvPrefix+name] = value
	}
	return prefixed
}

// TestValidate_URLFormat verifies that GITLAB_URL must have a valid scheme and host.
func TestValidate_URLFormat(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://gitlab.example.com", wantErr: false},
		{name: "valid http", url: "http://gitlab.local", wantErr: false},
		{name: "valid with port", url: "https://gitlab.example.com:8443", wantErr: false},
		{name: "valid with path", url: "https://gitlab.example.com/api", wantErr: false},
		{name: "missing scheme", url: "gitlab.example.com", wantErr: true},
		{name: "ftp scheme", url: "ftp://gitlab.example.com", wantErr: true},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: true},
		{name: "no host", url: "https://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				GitLabURL:   tt.url,
				GitLabToken: "test-token",
			}
			err := cfg.validate()
			if tt.wantErr && err == nil {
				t.Errorf("validate() for URL %q expected error, got nil", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate() for URL %q unexpected error: %v", tt.url, err)
			}
		})
	}
}

// TestValidate_AcceptableMaxValues verifies that the rate limit's values at
// their exact upper bound are accepted. The upload limit's ceiling is not
// validate's to judge (issue 1170): its reader accepts the ceiling itself,
// which TestUploadMaxFileSizeFromEnv_RefusesWhatEitherTransportRefuses holds.
func TestValidate_AcceptableMaxValues(t *testing.T) {
	cfg := &Config{
		GitLabURL:      "https://gitlab.example.com",
		GitLabToken:    "test-token",
		RateLimitRPS:   MaxRateLimitRPS,
		RateLimitBurst: MaxRateLimitBurst,
	}
	err := cfg.validate()
	if err != nil {
		t.Errorf("validate() unexpected error for max values: %v", err)
	}
}

// TestValidate_RateLimitBounds verifies that unreasonable rate limit values are rejected.
func TestValidate_RateLimitBounds(t *testing.T) {
	tests := []struct {
		name  string
		patch func(*Config)
	}{
		{name: "rps above maximum", patch: func(cfg *Config) { cfg.RateLimitRPS = MaxRateLimitRPS + 1 }},
		{name: "burst above maximum", patch: func(cfg *Config) { cfg.RateLimitBurst = MaxRateLimitBurst + 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{GitLabURL: "https://gitlab.example.com", GitLabToken: "test-token", RateLimitBurst: DefaultRateLimitBurst}
			tt.patch(cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("validate() expected rate limit bound error, got nil")
			}
		})
	}
}

// TestServerConfig_CopiesServerScopedFields verifies ServerConfig returns an
// immutable per-server snapshot and defensively copies slice-backed fields.
func TestServerConfig_CopiesServerScopedFields(t *testing.T) {
	if (*Config)(nil).ServerConfig() == nil {
		t.Fatal("nil Config should return an empty ServerConfig snapshot")
	}

	cfg := &Config{
		GitLabURL:         "https://gitlab.example.com",
		Tier:              edition.Ultimate,
		TierExplicit:      true,
		ReadOnly:          true,
		SafeMode:          true,
		ExcludeTools:      []string{"gitlab_create_project"},
		RateLimitRPS:      2.5,
		RateLimitBurst:    7,
		MetaParamSchema:   MetaParamSchemaFull,
		ToolSurface:       ToolSurfaceDynamic,
		CapabilitySurface: CapabilitySurfaceMinimal,
	}

	snapshot := cfg.ServerConfig()
	if snapshot.GitLabURL != cfg.GitLabURL || !snapshot.Enterprise() || !snapshot.ReadOnly || !snapshot.SafeMode {
		t.Fatalf("ServerConfig snapshot does not preserve boolean/url fields: %+v", snapshot)
	}
	if snapshot.Tier != edition.Ultimate || !snapshot.TierExplicit {
		t.Fatalf("ServerConfig snapshot does not preserve tier fields: %+v", snapshot)
	}
	if snapshot.RateLimitRPS != cfg.RateLimitRPS || snapshot.RateLimitBurst != cfg.RateLimitBurst {
		t.Fatalf("ServerConfig rate limit fields = (%v, %d), want (%v, %d)", snapshot.RateLimitRPS, snapshot.RateLimitBurst, cfg.RateLimitRPS, cfg.RateLimitBurst)
	}
	if snapshot.MetaParamSchema != MetaParamSchemaFull {
		t.Fatalf("MetaParamSchema = %q, want %q", snapshot.MetaParamSchema, MetaParamSchemaFull)
	}
	if snapshot.ToolSurface != ToolSurfaceDynamic {
		t.Fatalf("ToolSurface = %q, want %q", snapshot.ToolSurface, ToolSurfaceDynamic)
	}
	if snapshot.CapabilitySurface != CapabilitySurfaceMinimal {
		t.Fatalf("CapabilitySurface = %q, want %q", snapshot.CapabilitySurface, CapabilitySurfaceMinimal)
	}
	if !slices.Equal(snapshot.ExcludeTools, cfg.ExcludeTools) {
		t.Fatalf("ExcludeTools = %v, want %v", snapshot.ExcludeTools, cfg.ExcludeTools)
	}

	cfg.ExcludeTools[0] = "changed_in_source"
	if snapshot.ExcludeTools[0] != "gitlab_create_project" {
		t.Fatalf("snapshot ExcludeTools changed after source mutation: %v", snapshot.ExcludeTools)
	}
	snapshot.ExcludeTools[0] = "changed_in_snapshot"
	if cfg.ExcludeTools[0] != "changed_in_source" {
		t.Fatalf("source ExcludeTools changed after snapshot mutation: %v", cfg.ExcludeTools)
	}
}

// TestLoad_InvalidSkipTLS verifies that Load returns an error when
// GITLAB_MCP_SKIP_TLS_VERIFY has an invalid boolean value.
func TestLoad_InvalidSkipTLS(t *testing.T) {
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_SKIP_TLS_VERIFY")
	}
}

// TestLoad_InvalidTier verifies that Load returns an error when GITLAB_MCP_TIER
// holds an unrecognized value.
func TestLoad_InvalidTier(t *testing.T) {
	t.Setenv("GITLAB_MCP_TIER", "platinum")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_TIER")
	}
}

// TestLoad_TierResolution verifies that GITLAB_MCP_TIER resolves to the expected
// tier and explicit flag, including the unset (detect) case.
func TestLoad_TierResolution(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		set          bool
		wantTier     edition.Tier
		wantExplicit bool
		wantEnt      bool
	}{
		{name: "unset detects free", set: false, wantTier: edition.Free, wantExplicit: false, wantEnt: false},
		{name: "free", value: "free", set: true, wantTier: edition.Free, wantExplicit: true, wantEnt: false},
		{name: "ce maps to free", value: "ce", set: true, wantTier: edition.Free, wantExplicit: true, wantEnt: false},
		{name: "premium", value: "premium", set: true, wantTier: edition.Premium, wantExplicit: true, wantEnt: true},
		{name: "ultimate", value: "ultimate", set: true, wantTier: edition.Ultimate, wantExplicit: true, wantEnt: true},
		{name: "case insensitive", value: "Premium", set: true, wantTier: edition.Premium, wantExplicit: true, wantEnt: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", "https://gitlab.example.com")
			t.Setenv("GITLAB_TOKEN", "test")
			if tc.set {
				t.Setenv("GITLAB_MCP_TIER", tc.value)
			} else {
				t.Setenv("GITLAB_MCP_TIER", "")
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Tier != tc.wantTier {
				t.Errorf("Tier = %v, want %v", cfg.Tier, tc.wantTier)
			}
			if cfg.TierExplicit != tc.wantExplicit {
				t.Errorf("TierExplicit = %v, want %v", cfg.TierExplicit, tc.wantExplicit)
			}
			if cfg.Enterprise() != tc.wantEnt {
				t.Errorf("Enterprise() = %v, want %v", cfg.Enterprise(), tc.wantEnt)
			}
		})
	}
}

// TestLoad_RetiredEnterpriseEnv_IsIgnored verifies that GITLAB_ENTERPRISE, the
// tier selector v2 deprecated in favor of GITLAB_MCP_TIER, no longer decides
// anything: a configuration that sets it and nothing else detects the tier
// from the instance, as one that sets neither does.
func TestLoad_RetiredEnterpriseEnv_IsIgnored(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_TIER", "")
	t.Setenv("GITLAB_ENTERPRISE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Tier != edition.Free || cfg.TierExplicit {
		t.Errorf("Tier=%v Explicit=%v, want the detected default (free, not explicit)", cfg.Tier, cfg.TierExplicit)
	}
}

// TestLoad_InvalidReadOnly verifies that Load returns an error when
// GITLAB_MCP_READ_ONLY has an invalid boolean value.
func TestLoad_InvalidReadOnly(t *testing.T) {
	t.Setenv("GITLAB_MCP_READ_ONLY", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_READ_ONLY")
	}
}

// TestLoad_InvalidUploadMaxFileSize verifies that Load returns an error
// when UPLOAD_MAX_FILE_SIZE has an invalid value.
func TestLoad_InvalidUploadMaxFileSize(t *testing.T) {
	t.Setenv("GITLAB_MCP_UPLOAD_MAX_FILE_SIZE", "notanumber")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid UPLOAD_MAX_FILE_SIZE")
	}
}

// TestParseSize_InvalidSuffix verifies parseSize rejects invalid numeric strings
// that are not plain numbers or known suffixes.
func TestParseSize_InvalidSuffix(t *testing.T) {
	_, err := parseSize("50TB", 0)
	if err == nil {
		t.Fatal("expected error for unsupported suffix TB")
	}
}

// TestParseSize_NegativeValue verifies parseSize rejects negative values.
func TestParseSize_NegativeValue(t *testing.T) {
	_, err := parseSize("-10MB", 0)
	if err == nil {
		t.Fatal("expected error for negative size")
	}
}

// TestParseSize_ASizeNoByteCountHolds_IsRefused verifies that a size whose
// byte count does not fit in an int64 is refused rather than wrapped (issue
// 1170). The count is the number times its unit, and the product used to
// wrap: 17179869185GB is 2^64 plus 1 GiB bytes, which came back as 1 GiB, a
// limit far below the one written that the ceiling then had no reason to
// refuse, and 8589934592GB came back as the most negative int64, which is no
// limit at all to toolutil. The largest count of each unit that fits is a
// size like any other, and the ceiling judges it.
func TestParseSize_ASizeNoByteCountHolds_IsRefused(t *testing.T) {
	for _, tc := range []struct {
		input   string
		want    int64
		refused bool
	}{
		{input: "17179869185GB", refused: true},
		{input: "8589934592GB", refused: true},
		{input: "8796093022208MB", refused: true},
		{input: "9007199254740992KB", refused: true},
		{input: "8589934591GB", want: 8589934591 << 30},
		{input: "8796093022207MB", want: 8796093022207 << 20},
		{input: "9007199254740991KB", want: 9007199254740991 << 10},
		{input: "9223372036854775807", want: 9223372036854775807},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseSize(tc.input, 0)
			if tc.refused {
				if err == nil || !strings.Contains(err.Error(), "does not fit") {
					t.Errorf("parseSize(%q) = %d, %v; want a refusal saying the byte count does not fit", tc.input, got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("parseSize(%q) = %d, %v; want %d", tc.input, got, err, tc.want)
			}
		})
	}
}

// TestUploadMaxFileSizeFromEnv_RefusesWhatEitherTransportRefuses holds the
// one reader of the upload limit to its bounds (register row RQB-007, issue
// 1170). Stdio's Load and HTTP mode's overlay both read the setting through
// it, so the ceiling is applied there rather than by Validate, which only
// stdio calls: HTTP mode used to warn and use the default for a value it
// could not parse, and the maximum for one above it. Each refusal names the
// variable in full, which is also the variable --upload-max-file-size writes.
//
// A blank value is no value, as it is to the overlay, which reads one as
// absent and leaves the limit at its default: the reader used to refuse it as
// a size that did not parse, so the same exported line stopped a stdio server
// and started an HTTP one. A padded size is read trimmed, as every variable is.
func TestUploadMaxFileSizeFromEnv_RefusesWhatEitherTransportRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        int64
		says        string
	}{
		{name: "unset is the default", want: DefaultMaxFileSize},
		{name: "blank is the default", value: "   ", want: DefaultMaxFileSize},
		{name: "a padded size", value: " 50MB ", want: 50 << 20},
		{name: "a size with a unit", value: "50MB", want: 50 << 20},
		{name: "the ceiling itself", value: "1024GB", want: MaxFileSize},
		{name: "a value that does not parse", value: "the-size-of-a-cat", says: `invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value: invalid size "the-size-of-a-cat"`},
		{name: "zero", value: "0", says: `invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value: size must be positive, got "0"`},
		{name: "one byte above the ceiling", value: strconv.FormatInt(MaxFileSize+1, 10), says: "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB (got 1099511627777 bytes)"},
		{name: "a unit above the ceiling", value: "1025GB", says: "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB"},
		{name: "a size no byte count holds", value: "17179869185GB", says: "invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvPrefix+"UPLOAD_MAX_FILE_SIZE", tc.value)
			got, err := UploadMaxFileSizeFromEnv()
			if tc.says != "" {
				if err == nil || !strings.Contains(err.Error(), tc.says) {
					t.Errorf("UploadMaxFileSizeFromEnv() = %d, %v; want a refusal carrying %q", got, err, tc.says)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("UploadMaxFileSizeFromEnv() = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

// TestLoad_AnUploadSizeAboveTheCeilingOrNoByteCountHolds_RefusesStdio is the
// stdio half of the test above, through Load: a value the reader refuses ends
// the start with the reader's own words, the variable named in full, and a
// size that wrapped to one below the ceiling no longer starts a server under
// a limit nobody wrote.
func TestLoad_AnUploadSizeAboveTheCeilingOrNoByteCountHolds_RefusesStdio(t *testing.T) {
	for _, tc := range []struct{ value, says string }{
		{value: "1025GB", says: "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE exceeds maximum of 1 TB"},
		{value: "17179869185GB", says: "invalid GITLAB_MCP_UPLOAD_MAX_FILE_SIZE value"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv(EnvPrefix+"UPLOAD_MAX_FILE_SIZE", tc.value)
			cfg, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Load() = %+v, %v; want a refusal carrying %q", cfg, err, tc.says)
			}
		})
	}
}

// TestLoad_InvalidSafeMode verifies that Load returns an error when
// GITLAB_MCP_SAFE_MODE has an invalid boolean value.
func TestLoad_InvalidSafeMode(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	t.Setenv("GITLAB_MCP_SAFE_MODE", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_SAFE_MODE")
	}
}

// TestLoad_InvalidEmbeddedResources verifies invalid EMBEDDED_RESOURCES values
// are rejected during environment loading.
func TestLoad_InvalidEmbeddedResources(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_EMBEDDED_RESOURCES", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid EMBEDDED_RESOURCES")
	}
}

// TestValidate_DirectErrorBranches verifies validation-only branches that are
// normally guarded earlier by environment parsers.
func TestValidate_DirectErrorBranches(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "empty gitlab url", cfg: Config{GitLabToken: "test-token"}},
		{name: "invalid tool surface", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", ToolSurface: "unknown"}},
		{name: "invalid capability surface", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", CapabilitySurface: "compact"}},
		{name: "negative rate limit", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", RateLimitRPS: -1}},
		{name: "missing burst for positive rate limit", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", RateLimitRPS: 1, RateLimitBurst: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.validate(); err == nil {
				t.Fatal("validate() expected error, got nil")
			}
		})
	}
}

// TestEffectiveToolSurface verifies that a snapshot's surface is the one it
// names, and that anything else answers the default rather than a guess.
func TestEffectiveToolSurface(t *testing.T) {
	tests := []struct {
		name        string
		toolSurface string
		want        string
	}{
		{name: "dynamic", toolSurface: ToolSurfaceDynamic, want: ToolSurfaceDynamic},
		{name: "meta", toolSurface: ToolSurfaceMeta, want: ToolSurfaceMeta},
		{name: "individual", toolSurface: ToolSurfaceIndividual, want: ToolSurfaceIndividual},
		{name: "empty takes the default", want: DefaultToolSurface},
		{name: "unknown takes the default", toolSurface: "unknown", want: DefaultToolSurface},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveToolSurface(tt.toolSurface); got != tt.want {
				t.Errorf("EffectiveToolSurface() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseToolSurfaceValue_EverySpellingAnOperatorMayWrite verifies each
// accepted spelling of the tool-surface selector resolves to the surface it
// names, and that an unrecognized one is refused by name.
//
// The selector accepts nine spellings across three surfaces, and only four of
// them were ever evaluated: the three canonical names plus one alias. An arm
// quietly deleted from any of the three cases would have turned a documented
// value into a startup error, and nothing here would have noticed. They are
// cheap to pin because the function is pure, and they are the values operators
// copy out of the documentation.
//
// The boolean spellings this table also carried are gone with META_TOOLS: a
// tool surface named "true" meant the meta catalog only because the deprecated
// selector was a boolean.
func TestParseToolSurfaceValue_EverySpellingAnOperatorMayWrite(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "the canonical meta", value: ToolSurfaceMeta, want: ToolSurfaceMeta},
		{name: "meta-tools", value: "meta-tools", want: ToolSurfaceMeta},
		{name: "metatools", value: "metatools", want: ToolSurfaceMeta},

		{name: "the canonical individual", value: ToolSurfaceIndividual, want: ToolSurfaceIndividual},
		{name: "individual-tools", value: "individual-tools", want: ToolSurfaceIndividual},
		{name: "tools", value: "tools", want: ToolSurfaceIndividual},

		{name: "the canonical dynamic", value: ToolSurfaceDynamic, want: ToolSurfaceDynamic},
		{name: "dynamic-tools", value: "dynamic-tools", want: ToolSurfaceDynamic},
		{name: "low-token", value: "low-token", want: ToolSurfaceDynamic},

		{name: "case and surrounding space are ignored", value: "  META  ", want: ToolSurfaceMeta},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToolSurfaceValue(tt.value, "TOOL_SURFACE")
			if err != nil {
				t.Fatalf("parseToolSurfaceValue(%q) unexpected error: %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("parseToolSurfaceValue(%q) = %q, want %q", tt.value, got, tt.want)
			}

			// The same spelling must survive the exported entry point.
			mode, err := ParseToolSurface(tt.value)
			if err != nil {
				t.Fatalf("ParseToolSurface(%q) unexpected error: %v", tt.value, err)
			}
			if mode != tt.want {
				t.Errorf("ParseToolSurface(%q) = %q, want %q", tt.value, mode, tt.want)
			}
		})
	}
}

// TestParseToolSurfaceValue_UnknownIsRefusedByName verifies the error names
// the setting and the value, since an operator reading it has only the log to
// work out which of the two selectors they mistyped.
func TestParseToolSurfaceValue_UnknownIsRefusedByName(t *testing.T) {
	_, err := parseToolSurfaceValue("indivdual", "TOOL_SURFACE")
	if err == nil {
		t.Fatal("parseToolSurfaceValue(typo) = nil error, want a refusal")
	}
	for _, want := range []string{"TOOL_SURFACE", "indivdual"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		})
	}
	if _, parseErr := ParseToolSurface("indivdual"); parseErr == nil {
		t.Error("ParseToolSurface(typo) = nil error, want the same refusal")
	}
}

// TestParseCapabilitySurface_Aliases verifies capability surface aliases and errors.
func TestParseCapabilitySurface_Aliases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty uses default", input: "", want: CapabilitySurfaceMinimal},
		{name: "default alias", input: "default", want: CapabilitySurfaceFull},
		{name: "low token alias", input: "low-token", want: CapabilitySurfaceMinimal},
		{name: "invalid", input: "compact", wantErr: true},
		// The canonical spellings and the remaining alias: only the aliases
		// were exercised, so the two arms an operator is most likely to
		// write were the untested ones.
		{name: "the canonical full", input: CapabilitySurfaceFull, want: CapabilitySurfaceFull},
		{name: "the canonical minimal", input: CapabilitySurfaceMinimal, want: CapabilitySurfaceMinimal},
		{name: "minimum alias", input: "minimum", want: CapabilitySurfaceMinimal},
		{name: "case and surrounding space are ignored", input: "  FULL  ", want: CapabilitySurfaceFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCapabilitySurface(tt.input, CapabilitySurfaceMinimal)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseCapabilitySurface() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCapabilitySurface() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseCapabilitySurface() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseFloatNonNegative_NonFinite verifies NaN and Infinity are rejected.
func TestParseFloatNonNegative_NonFinite(t *testing.T) {
	for _, input := range []string{"NaN", "+Inf"} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseFloatNonNegative(input, 0); err == nil {
				t.Fatal("parseFloatNonNegative() expected error, got nil")
			}
		})
	}
}

// TestLoad_InvalidIgnoreScopes verifies that Load returns an error when
// GITLAB_MCP_IGNORE_SCOPES has an invalid boolean value.
func TestLoad_InvalidIgnoreScopes(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	t.Setenv("GITLAB_MCP_SAFE_MODE", "false")
	t.Setenv("GITLAB_MCP_IGNORE_SCOPES", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_IGNORE_SCOPES")
	}
}

// TestLoad_RevalidateInterval verifies SESSION_REVALIDATE_INTERVAL env var
// parsing: default, custom, invalid, and exceeds-max scenarios.
func TestLoad_RevalidateInterval(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.RevalidateInterval != DefaultRevalidateInterval {
			t.Errorf("RevalidateInterval = %v, want %v", cfg.RevalidateInterval, DefaultRevalidateInterval)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "5m")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.RevalidateInterval != 5*time.Minute {
			t.Errorf("RevalidateInterval = %v, want 5m", cfg.RevalidateInterval)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "notaduration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid SESSION_REVALIDATE_INTERVAL")
		}
	})

	t.Run("exceeds maximum", func(t *testing.T) {
		t.Setenv("GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", "25h")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for SESSION_REVALIDATE_INTERVAL exceeding maximum")
		}
	})

	// The maximum itself is a value the operator may set. Only "over" and
	// "well under" were asserted, so a bound that had drifted from > to >=
	// would refuse the documented ceiling and no test would say so.
	t.Run("the maximum itself is accepted", func(t *testing.T) {
		t.Setenv("GITLAB_MCP_SESSION_REVALIDATE_INTERVAL", MaxRevalidateInterval.String())
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() refused the documented maximum: %v", err)
		}
		if cfg.RevalidateInterval != MaxRevalidateInterval {
			t.Errorf("RevalidateInterval = %v, want %v", cfg.RevalidateInterval, MaxRevalidateInterval)
		}
	})
}

// TestParseCSV_Scenarios verifies ParseCSV handles various input patterns
// including empty strings, single values, multiple values, and whitespace.
func TestParseCSV_Scenarios(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty string", input: "", want: nil},
		{name: "single value", input: "tool_a", want: []string{"tool_a"}},
		{name: "multiple values", input: "tool_a,tool_b,tool_c", want: []string{"tool_a", "tool_b", "tool_c"}},
		{name: "whitespace trimmed", input: " tool_a , tool_b ", want: []string{"tool_a", "tool_b"}},
		{name: "empty tokens filtered", input: "tool_a,,tool_b,", want: []string{"tool_a", "tool_b"}},
		{name: "only commas", input: ",,,", want: []string{}},
		{name: "spaces only tokens", input: " , , ", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCSV(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("ParseCSV(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseCSV(%q) returned %d items, want %d", tt.input, len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("ParseCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestLoad_ExcludeTools verifies that EXCLUDE_TOOLS is parsed into
// Config.ExcludeTools via ParseCSV.
func TestLoad_ExcludeTools(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_EXCLUDE_TOOLS", "gitlab_create_issue, gitlab_delete_project")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadErr, err)
	}
	if len(cfg.ExcludeTools) != 2 {
		t.Fatalf("ExcludeTools has %d items, want 2", len(cfg.ExcludeTools))
	}
	if cfg.ExcludeTools[0] != "gitlab_create_issue" {
		t.Errorf("ExcludeTools[0] = %q, want %q", cfg.ExcludeTools[0], "gitlab_create_issue")
	}
	if cfg.ExcludeTools[1] != "gitlab_delete_project" {
		t.Errorf("ExcludeTools[1] = %q, want %q", cfg.ExcludeTools[1], "gitlab_delete_project")
	}
}

// TestParseIntNonNegative verifies parseIntNonNegative handles empty strings
// (default), valid non-negative integers, negative values (error), and
// invalid strings (error).
func TestParseIntNonNegative(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		defaultVal int
		want       int
		wantErr    bool
	}{
		{"empty returns default", "", 40, 40, false},
		{"zero is valid", "0", 40, 0, false},
		{"positive value", "10", 0, 10, false},
		{"whitespace trimmed", " 5 ", 0, 5, false},
		{"negative value", "-1", 0, 0, true},
		{"invalid string", "abc", 0, 0, true},
		{"float string rejected", "1.5", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseIntNonNegative(tt.input, tt.defaultVal)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseIntNonNegative(%q, %d) error = %v, wantErr %v", tt.input, tt.defaultVal, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseIntNonNegative(%q, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

// TestParseFloatNonNegative verifies parseFloatNonNegative handles empty
// strings (default), valid non-negative floats, zero (valid, disables
// feature), negative values (error), and invalid strings (error).
func TestParseFloatNonNegative(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		defaultVal float64
		want       float64
		wantErr    bool
	}{
		{"empty returns default", "", 0, 0, false},
		{"zero is valid", "0", 5.0, 0, false},
		{"positive integer string", "10", 0, 10, false},
		{"positive float", "2.5", 0, 2.5, false},
		{"whitespace trimmed", " 3.14 ", 0, 3.14, false},
		{"negative value", "-0.1", 0, 0, true},
		{"invalid string", "abc", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseFloatNonNegative(tt.input, tt.defaultVal)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFloatNonNegative(%q, %g) error = %v, wantErr %v", tt.input, tt.defaultVal, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseFloatNonNegative(%q, %g) = %g, want %g", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

// TestLoad_RateLimitRPS verifies RATE_LIMIT_RPS env var parsing and defaults.
func TestLoad_RateLimitRPS(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    float64
		wantErr bool
	}{
		{subtestDefault, "", 0, false},
		{subtestCustom, "5.5", 5.5, false},
		{subtestInvalid, "not-a-number", 0, true},
		{"negative rejected", "-1", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testHTTPExampleURL)
			t.Setenv("GITLAB_TOKEN", "test")
			if tt.envVal != "" {
				t.Setenv("GITLAB_MCP_RATE_LIMIT_RPS", tt.envVal)
			}
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf(fmtLoadErr, err)
			}
			if cfg.RateLimitRPS != tt.want {
				t.Errorf("RateLimitRPS = %g, want %g", cfg.RateLimitRPS, tt.want)
			}
		})
	}
}

// TestLoad_RateLimitBurst verifies RATE_LIMIT_BURST env var parsing and defaults.
func TestLoad_RateLimitBurst(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    int
		wantErr bool
	}{
		{subtestDefault, "", DefaultRateLimitBurst, false},
		{subtestCustom, "100", 100, false},
		{subtestInvalid, "xyz", 0, true},
		{"negative rejected", "-5", 0, true},
		{"zero is valid", "0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testHTTPExampleURL)
			t.Setenv("GITLAB_TOKEN", "test")
			if tt.envVal != "" {
				t.Setenv("GITLAB_MCP_RATE_LIMIT_BURST", tt.envVal)
			}
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf(fmtLoadErr, err)
			}
			if cfg.RateLimitBurst != tt.want {
				t.Errorf("RateLimitBurst = %d, want %d", cfg.RateLimitBurst, tt.want)
			}
		})
	}
}

// TestValidate_RateLimitBurstRequiredWithRPS verifies that a positive
// RATE_LIMIT_RPS combined with a zero RATE_LIMIT_BURST fails validation.
func TestValidate_RateLimitBurstRequiredWithRPS(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_RATE_LIMIT_RPS", "10")
	t.Setenv("GITLAB_MCP_RATE_LIMIT_BURST", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for RPS > 0 with burst = 0, got nil")
	}
}

// TestConfig_InstanceURLs_DerivesTheListFromTheSingleURL pins the invariant
// that keeps the two instance fields from disagreeing.
//
// GitLabURLs is the full list and GitLabURL its first entry, but only the flag
// layer fills both. Every other constructor sets GitLabURL alone, and reading
// the slice directly there yields "no instance fixed" — which resolves to the
// public GitLab and sends the request to an instance nobody configured. That
// is not hypothetical: it broke an HTTP-mode test the moment the request gate
// started reading the list.
func TestConfig_InstanceURLs_DerivesTheListFromTheSingleURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		want []string
	}{
		{name: "nil config", cfg: nil},
		{name: "nothing configured", cfg: &Config{}},
		{
			name: "only the single URL is set",
			cfg:  &Config{GitLabURL: "https://gitlab.example.com"},
			want: []string{"https://gitlab.example.com"},
		},
		{
			name: "the list wins when both are set",
			cfg: &Config{
				GitLabURL:  "https://gitlab.com",
				GitLabURLs: []string{"https://gitlab.com", "https://gitlab.example.com"},
			},
			want: []string{"https://gitlab.com", "https://gitlab.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.InstanceURLs(); !slices.Equal(got, tt.want) {
				t.Errorf("InstanceURLs() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConfigEnterprise_NilReceivers verifies the nil-receiver guards of
// Config.Enterprise and ServerConfig.Enterprise return false instead of
// panicking, so callers can probe unset configurations safely.
func TestConfigEnterprise_NilReceivers(t *testing.T) {
	var c *Config
	if c.Enterprise() {
		t.Error("nil *Config Enterprise() = true, want false")
	}
	var s *ServerConfig
	if s.Enterprise() {
		t.Error("nil *ServerConfig Enterprise() = true, want false")
	}
}

// zeroOffDuration is a duration variable whose zero turns its setting off, with
// the field each of the two readers fills from it: [Load] for stdio, nil for a
// setting only HTTP mode has, which stdio does not read, and
// [LoadHTTPEnvOverlay] for HTTP mode.
type zeroOffDuration struct {
	name    string
	loaded  func(*Config) time.Duration
	overlay func(*HTTPEnvOverlay) *time.Duration
}

// zeroOffDurations are every duration variable a zero turns off. The session
// timeout is the one issue 958 added: its variable used to refuse the zero its
// flag reads as off (issue 1171, item 1).
func zeroOffDurations() []zeroOffDuration {
	return []zeroOffDuration{
		{
			"SESSION_TIMEOUT",
			nil,
			func(o *HTTPEnvOverlay) *time.Duration { return o.SessionTimeout },
		},
		{
			"POOL_IDLE_TIMEOUT",
			nil,
			func(o *HTTPEnvOverlay) *time.Duration { return o.PoolIdleTimeout },
		},
		{
			"SESSION_REVALIDATE_INTERVAL",
			func(c *Config) time.Duration { return c.RevalidateInterval },
			func(o *HTTPEnvOverlay) *time.Duration { return o.RevalidateInterval },
		},
		{
			"ACTION_TIMEOUT",
			func(c *Config) time.Duration { return c.ActionTimeout },
			func(o *HTTPEnvOverlay) *time.Duration { return o.ActionTimeout },
		},
		{
			"DRAIN_DELAY",
			nil,
			func(o *HTTPEnvOverlay) *time.Duration { return o.DrainDelay },
		},
		{
			"AUTH_FAILURE_WINDOW",
			nil,
			func(o *HTTPEnvOverlay) *time.Duration { return o.AuthFailureWindow },
		},
		{
			"AUTH_DISTINCT_TOKEN_WINDOW",
			nil,
			func(o *HTTPEnvOverlay) *time.Duration { return o.AuthDistinctWindow },
		},
	}
}

// zeroDurationSpellings are ways an operator writes a zero duration: the bare
// digit the variables documented first, and the Go duration forms the flag of
// the same name accepts, which the variables used to refuse (issue 1171, item
// 3), padded too, since every variable is read trimmed.
var zeroDurationSpellings = []string{"0", "0s", "0ms", "0h0m0s", " 0s "}

// TestLoad_AZeroDuration_TurnsItsSettingOffHoweverWritten pins what zero means
// for every duration variable that has a zero to mean, at the stdio reader's
// environment layer: the setting is off, written as the bare digit or as a Go
// duration (issue 958). A negative duration is still refused, since zero
// already says off and a minus sign is a typo. The settings only HTTP mode has
// are not read here at all (issue 1171), which
// TestLoad_HTTPOnlySettings_AreNotRead holds.
func TestLoad_AZeroDuration_TurnsItsSettingOffHoweverWritten(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	for _, setting := range zeroOffDurations() {
		if setting.loaded == nil {
			continue
		}
		t.Run(setting.name, func(t *testing.T) {
			for _, spelling := range zeroDurationSpellings {
				t.Run(strconv.Quote(spelling), func(t *testing.T) {
					t.Setenv(EnvPrefix+setting.name, spelling)
					cfg, err := Load()
					if err != nil {
						t.Fatalf("Load() with %s=%q error = %v, want the setting off", setting.name, spelling, err)
					}
					if got := setting.loaded(cfg); got != 0 {
						t.Errorf("%s=%q loaded as %v, want 0", setting.name, spelling, got)
					}
				})
			}
			t.Run("negative", func(t *testing.T) {
				t.Setenv(EnvPrefix+setting.name, "-1s")
				if _, err := Load(); err == nil {
					t.Errorf("Load() with %s=-1s error = nil, want a refusal", setting.name)
				}
			})
		})
	}
}

// TestLoad_ABlankDuration_IsUnsetOnBothTransports pins what a blank duration
// variable means to both readers: nothing was set, so the setting keeps its
// default. HTTP mode's overlay has always read a blank value as absent, and
// stdio's reader refused it as a duration that did not parse, so the same
// exported line started an HTTP server and stopped a stdio one. The action
// timeout's row states that a malformed value refuses startup on both
// transports (RQB-005), which held only once both agreed on what a value is.
func TestLoad_ABlankDuration_IsUnsetOnBothTransports(t *testing.T) {
	for _, setting := range zeroOffDurations() {
		t.Run(setting.name, func(t *testing.T) {
			clearOverlayEnv(t)
			t.Setenv("GITLAB_URL", testHTTPExampleURL)
			t.Setenv("GITLAB_TOKEN", "test")
			var unset *Config
			if setting.loaded != nil {
				var err error
				if unset, err = Load(); err != nil {
					t.Fatalf("Load() with %s unset error = %v", setting.name, err)
				}
			}
			t.Setenv(EnvPrefix+setting.name, "   ")
			if setting.loaded != nil {
				cfg, err := Load()
				if err != nil {
					t.Fatalf("Load() with a blank %s error = %v, want its default", setting.name, err)
				}
				if got, want := setting.loaded(cfg), setting.loaded(unset); got != want {
					t.Errorf("a blank %s loaded as %v, want the default %v", setting.name, got, want)
				}
			}
			overlay, err := LoadHTTPEnvOverlay()
			if err != nil {
				t.Fatalf("LoadHTTPEnvOverlay() with a blank %s error = %v, want it absent", setting.name, err)
			}
			if got := setting.overlay(overlay); got != nil {
				t.Errorf("a blank %s overlaid as %v, want nil", setting.name, *got)
			}
		})
	}
}

// TestValidate_OAuthRequiresPublicURL verifies oauth mode refuses to start
// without a public URL, and enforces the RFC 9728 identifier constraints on
// the one given.
func TestValidate_OAuthRequiresPublicURL(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		wantErr   bool
	}{
		{"missing", "", true},
		{"http non-loopback", "http://mcp.example.com", true},
		{"trailing slash", "https://mcp.example.com/", true},
		{"fragment", "https://mcp.example.com#frag", true},
		{"https origin", "https://mcp.example.com", false},
		{"https with path", "https://mcp.example.com/gitlab", false},
		{"loopback http", "http://localhost:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePublicURL(tt.publicURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePublicURL(%q) error = %v, wantErr %t", tt.publicURL, err, tt.wantErr)
			}
		})
	}
}

// TestValidateMetadataURL_RejectsWhatTheFlagsPromise checks the URLs this
// deployment publishes in its protected-resource metadata.
//
// The flag descriptions say https, and nothing enforced it. These links are
// followed by consent screens and directories, so a bad value does not fail
// where an operator would see it: it produces a document a client rejects, or
// renders a link nobody can use, long after startup succeeded.
//
// http on a loopback host is allowed, matching --public-url, so a developer can
// point one at a local page while trying things out.
func TestValidateMetadataURL_RejectsWhatTheFlagsPromise(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty is fine; the field is simply omitted", value: ""},
		{name: "an https URL", value: "https://example.com/privacy"},
		{name: "https with a path and query", value: "https://example.com/a/b?c=d"},
		{name: "http on loopback, for local development", value: "http://127.0.0.1:8080/privacy"},
		{name: "http on localhost", value: "http://localhost:8080/privacy"},

		{name: "plain http on a public host", value: "http://example.com/privacy", wantErr: true},
		{name: "not a URL at all", value: "not a url", wantErr: true},
		{name: "a path with no host", value: "/privacy", wantErr: true},
		{name: "a scheme that is not http(s)", value: "ftp://example.com/privacy", wantErr: true},
		{name: "a mailto link", value: "mailto:someone@example.com", wantErr: true},
		{name: "unparseable", value: "https://[::1/privacy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetadataURL("--resource-policy-uri", tt.value)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateMetadataURL(%q) = nil, want an error", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateMetadataURL(%q) = %v, want nil", tt.value, err)
			}
			if err != nil && !strings.Contains(err.Error(), "--resource-policy-uri") {
				t.Errorf("err = %v, want it to name the flag the operator set", err)
			}
		})
	}
}

// TestLoad_PrefixedNamesReachTheConfig verifies that a setting exported under
// its GITLAB_MCP_ name arrives in the loaded [Config].
//
// Wiring each reader is not the same as each reader being wired: these values
// pass through parsers, presence gates and grouping structs on the way, and a
// name converted at the wrong end of one of those would still read only the
// deprecated spelling. Asserting on the resulting Config is what covers the
// whole path, and the absence of a deprecation warning is what proves the
// prefixed name was the one used rather than an old one left over.
func TestLoad_PrefixedNamesReachTheConfig(t *testing.T) {
	// The authentication mode and the public URL were cases here too, until
	// stdio stopped reading the settings only HTTP mode has (issue 1171); the
	// HTTP overlay's own test of prefixed names covers them now.
	for _, tc := range []struct {
		name  string
		env   string
		value string
		field string
	}{
		{name: "tool surface", env: "TOOL_SURFACE", value: ToolSurfaceMeta, field: "ToolSurface"},
		{name: "capability surface", env: "CAPABILITY_SURFACE", value: CapabilitySurfaceMinimal, field: "CapabilitySurface"},
		{name: "meta param schema", env: "META_PARAM_SCHEMA", value: MetaParamSchemaFull, field: "MetaParamSchema"},
		{name: "excluded tools", env: "EXCLUDE_TOOLS", value: "gitlab_issue_delete", field: "ExcludeTools"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv(EnvPrefix+tc.env, tc.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() with %s set: %v", EnvPrefix+tc.env, err)
			}

			got := loadedSetting(cfg, tc.field)
			if got != tc.value {
				t.Errorf("%s = %q, want %q", EnvPrefix+tc.env, got, tc.value)
			}
			refuse, warn := RetiredEnvUses()
			if len(refuse) != 0 || len(warn) != 0 {
				t.Errorf("loading with only prefixed names reported retired ones: refuse=%v warn=%v", refuse, warn)
			}
		})
	}
}

// loadedSetting reads one string-valued setting by name, so the case table can
// stay data instead of carrying an accessor closure per case.
func loadedSetting(cfg *Config, field string) string {
	switch field {
	case "ToolSurface":
		return cfg.ToolSurface
	case "CapabilitySurface":
		return cfg.CapabilitySurface
	case "MetaParamSchema":
		return cfg.MetaParamSchema
	case "ExcludeTools":
		return strings.Join(cfg.ExcludeTools, ",")
	default:
		return "<unknown field " + field + ">"
	}
}

// TestParseTierFlag_MirrorsTheEnvironmentParser verifies the exported flag
// parser HTTP mode uses gives the same three answers the environment parser
// does: unset means detect, a known name is explicit, and anything else is
// refused naming the accepted values.
func TestParseTierFlag_MirrorsTheEnvironmentParser(t *testing.T) {
	cases := []struct {
		name         string
		value        string
		wantTier     edition.Tier
		wantExplicit bool
		wantErr      bool
	}{
		{name: "unset detects", value: "", wantTier: edition.Free},
		{name: "blank detects", value: "   ", wantTier: edition.Free},
		{name: "premium is explicit", value: "premium", wantTier: edition.Premium, wantExplicit: true},
		{name: "ce is free and explicit", value: "ce", wantTier: edition.Free, wantExplicit: true},
		{name: "unknown is refused", value: "platinum", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, explicit, err := ParseTierFlag(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTierFlag(%q) accepted a tier that does not exist", tc.value)
				}
				if !strings.Contains(err.Error(), "premium") {
					t.Errorf("error %q does not list the accepted values", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTierFlag(%q): %v", tc.value, err)
			}
			if tier != tc.wantTier || explicit != tc.wantExplicit {
				t.Errorf("ParseTierFlag(%q) = (%v, %v), want (%v, %v)", tc.value, tier, explicit, tc.wantTier, tc.wantExplicit)
			}
		})
	}
}

// TestTierFromEnv_ResolvesTheTierWithoutTheRestOfAConfiguration verifies the
// exported environment resolver, which exists for --tool-search: it inspects
// the catalog offline, so it must reach the tier without [Load] demanding the
// GitLab URL and token.
//
// It answers what Load answers, so the two cannot drift into disagreeing
// about which catalog a deployment serves.
func TestTierFromEnv_ResolvesTheTierWithoutTheRestOfAConfiguration(t *testing.T) {
	cases := []struct {
		name         string
		tier         string
		wantTier     edition.Tier
		wantExplicit bool
		wantErr      bool
	}{
		{name: "unset detects free", wantTier: edition.Free},
		{name: "the tier variable", tier: "ultimate", wantTier: edition.Ultimate, wantExplicit: true},
		{name: "an unknown tier is refused", tier: "platinum", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvPrefix+"TIER", tc.tier)
			t.Setenv("GITLAB_TIER", "")

			tier, explicit, err := TierFromEnv()
			if tc.wantErr {
				if err == nil {
					t.Fatal("TierFromEnv() = nil error, want the value refused")
				}
				return
			}
			if err != nil {
				t.Fatalf("TierFromEnv(): %v", err)
			}
			if tier != tc.wantTier || explicit != tc.wantExplicit {
				t.Errorf("TierFromEnv() = (%v, %v), want (%v, %v)", tier, explicit, tc.wantTier, tc.wantExplicit)
			}
		})
	}
}

// TestIsLoopbackGitLabURL_NamesOnlyThisMachine verifies the loopback test
// the cleartext exemptions rest on: the three spellings of this machine are
// loopback, a public host and an unparseable value are not.
func TestIsLoopbackGitLabURL_NamesOnlyThisMachine(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "localhost", raw: "http://localhost:8080", want: true},
		{name: "ipv4 loopback", raw: "http://127.0.0.1", want: true},
		{name: "ipv6 loopback", raw: "http://[::1]:3000/api", want: true},
		{name: "public host", raw: "https://gitlab.example.com", want: false},
		{name: "no host", raw: "gitlab.example.com", want: false},
		{name: "unparseable", raw: "http://[::1", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLoopbackGitLabURL(tc.raw); got != tc.want {
				t.Errorf("IsLoopbackGitLabURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestValidateOAuthGitLabURL_RequiresHTTPSOffTheMachine verifies bearer tokens
// are never forwarded in cleartext to an instance elsewhere: https anywhere
// passes, http passes only on loopback, and a value with no host is refused
// as not being a URL at all.
func TestValidateOAuthGitLabURL_RequiresHTTPSOffTheMachine(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "https", raw: "https://gitlab.example.com"},
		{name: "http on loopback", raw: "http://127.0.0.1:8929"},
		{name: "http elsewhere", raw: "http://gitlab.example.com", wantErr: "cleartext"},
		{name: "not a url", raw: "gitlab.example.com", wantErr: "not an absolute URL"},
		{name: "unparseable", raw: "https://[::1", wantErr: "not an absolute URL"},
		// Neither https nor http: the loopback exemption must not be
		// reached at all, or a scheme this server cannot speak would be
		// accepted for any host that happens to be local.
		{name: "a scheme that is neither http nor https", raw: "ftp://gitlab.example.com", wantErr: "cleartext"},
		{name: "a foreign scheme on loopback", raw: "ftp://127.0.0.1:8929", wantErr: "cleartext"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOAuthGitLabURL(tc.raw)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("ValidateOAuthGitLabURL(%q) = %v, want it accepted", tc.raw, err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("ValidateOAuthGitLabURL(%q) accepted a URL that puts a token on the wire", tc.raw)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not say %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidatePublicURL_ExportedPathAndTheNonURL verifies the exported
// wrapper the HTTP flag path calls reaches the same checks as the loader,
// including the one a value with no host trips: it is refused as not being
// an absolute URL rather than passing because it has no fragment or slash.
func TestValidatePublicURL_ExportedPathAndTheNonURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "accepted", raw: "https://mcp.example.com"},
		{name: "no host", raw: "mcp.example.com", wantErr: "not an absolute URL"},
		{name: "unparseable", raw: "https://[::1", wantErr: "not an absolute URL"},
		{name: "empty", raw: "", wantErr: "requires --public-url"},
		// A scheme that is neither https nor http short-circuits before the
		// loopback exemption, so an identifier no client can dereference is
		// refused wherever it points.
		{name: "a scheme that is neither http nor https", raw: "ftp://mcp.example.com", wantErr: "must use https"},
		{name: "a foreign scheme on loopback", raw: "ftp://127.0.0.1:8080", wantErr: "must use https"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePublicURL(tc.raw)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("ValidatePublicURL(%q) = %v, want it accepted", tc.raw, err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("ValidatePublicURL(%q) accepted an unusable identifier", tc.raw)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not say %q", err, tc.wantErr)
			}
		})
	}
}

// TestLoad_ActionTimeoutInvalid verifies the limit parsed last in the limit
// block refuses a value outside its range through Load, which is the path a
// deployment actually takes. The drain delay was a case here too, until stdio
// stopped reading it with the rest of the settings only HTTP mode has (issue
// 1171).
func TestLoad_ActionTimeoutInvalid(t *testing.T) {
	cases := []struct {
		name  string
		env   string
		value string
	}{
		{name: "action timeout above its maximum", env: "ACTION_TIMEOUT", value: "48h"},
		{name: "action timeout unparseable", env: "ACTION_TIMEOUT", value: "soon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv(EnvPrefix+tc.env, tc.value)
			if _, err := Load(); err == nil {
				t.Errorf("Load() accepted %s=%q", EnvPrefix+tc.env, tc.value)
			}
		})
	}
}

// TestUploadSizeConstants_HoldTheDefaultBelowTheCeilingItNames states what the
// two upload-size constants are, which nothing else in this package does.
//
// Every other assertion about them reads a loaded Config back against the same
// constant that filled it, so one that collapsed would move both sides of the
// comparison and fail nothing. The direction it would collapse in is the
// dangerous one: toolutil reads a non-positive limit as no limit at all, so a
// DefaultMaxFileSize that lost its multiplication would switch the
// out-of-the-box upload ceiling off with the suite still green. MaxFileSize is
// the figure UploadMaxFileSizeFromEnv names when it refuses ("exceeds maximum
// of 1 TB"), so one that moved would make that sentence a lie.
//
// The wanted sizes are spelled as shifts rather than as the product the
// declaration uses, so the two sides cannot move together.
func TestUploadSizeConstants_HoldTheDefaultBelowTheCeilingItNames(t *testing.T) {
	t.Parallel()

	const (
		twoGiB = int64(2) << 30
		oneTiB = int64(1) << 40
	)
	defaultSize, ceiling := int64(DefaultMaxFileSize), int64(MaxFileSize)

	if defaultSize != twoGiB {
		t.Errorf("DefaultMaxFileSize = %d bytes, want %d: the environment table publishes it as 2GB",
			defaultSize, twoGiB)
	}
	if ceiling != oneTiB {
		t.Errorf("MaxFileSize = %d bytes, want %d: the reader refuses anything above it by naming 1 TB",
			ceiling, oneTiB)
	}
	if defaultSize <= 0 {
		t.Errorf("DefaultMaxFileSize = %d: toolutil reads a non-positive limit as no limit at all, "+
			"so a deployment that configured nothing would accept a file of any size", defaultSize)
	}
	if defaultSize > ceiling {
		t.Errorf("DefaultMaxFileSize = %d is above MaxFileSize = %d: the configuration a deployment "+
			"gets without asking for one would be refused by its own validation", defaultSize, ceiling)
	}
}

// TestDurationConstants_ArePublishedAsTheDocumentationStatesThem pins every
// duration this package declares.
//
// It exists for the reason the size test above does. The load tests compare a
// parsed Config against the very constant that supplied its default, and the
// bound tests hand a maximum back to the parser that enforces it, so a constant
// that collapsed to zero would move both sides of every one of them. Zero is
// what this arithmetic collapses to, and the parser here has no floor to catch
// it: parseDisableableDurationEnv only refuses a value above the maximum, and
// reads zero as the operator asking for no bound at
// all — so a DefaultActionTimeout that lost its multiplication would leave
// every handler unbounded and say nothing, which is the opposite of what the
// constant is for.
//
// The wanted durations are spelled in a smaller unit than each declaration
// uses, so the two sides cannot move together.
func TestDurationConstants_ArePublishedAsTheDocumentationStatesThem(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "DefaultSessionTimeout", got: DefaultSessionTimeout, want: 1800 * time.Second},
		{name: "MaxSessionTimeout", got: MaxSessionTimeout, want: 1440 * time.Minute},
		{name: "DefaultRevalidateInterval", got: DefaultRevalidateInterval, want: 900 * time.Second},
		{name: "MaxRevalidateInterval", got: MaxRevalidateInterval, want: 1440 * time.Minute},
		{name: "DefaultPoolIdleTimeout", got: DefaultPoolIdleTimeout, want: 60 * time.Minute},
		{name: "MaxPoolIdleTimeout", got: MaxPoolIdleTimeout, want: 1440 * time.Minute},
		{name: "DefaultActionTimeout", got: DefaultActionTimeout, want: 3900 * time.Second},
		{name: "MaxActionTimeout", got: MaxActionTimeout, want: 1440 * time.Minute},
		{name: "DefaultDrainDelay", got: DefaultDrainDelay, want: 0},
		{name: "MaxDrainDelay", got: MaxDrainDelay, want: 300 * time.Second},
		{name: "DefaultOAuthCacheTTL", got: DefaultOAuthCacheTTL, want: 900 * time.Second},
		{name: "MinOAuthCacheTTL", got: MinOAuthCacheTTL, want: 60 * time.Second},
		{name: "MaxOAuthCacheTTL", got: MaxOAuthCacheTTL, want: 120 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("%s = %s, want %s: the environment and flag tables publish that figure",
					tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestDurationConstants_KeepEveryDefaultInsideItsOwnBound asserts the shape of
// each pair rather than either figure in it, so a deliberate change to a
// published default still has to leave the deployment that asks for nothing
// working.
//
// A default above its own ceiling is not a lint: the start that reads a setting
// runs its default through the same bound check an operator's value goes
// through, so such a pair refuses startup for a deployment that set nothing at
// all. On stdio that is Load, for the two of these stdio has (the revalidation
// interval and the action timeout). In HTTP mode, whose flags default to these
// constants, it is cmd/server's validateHTTPDurationConfig for the first five
// and its validateOAuthCacheTTL for the TTL of an oauth deployment.
//
// The two assertions after the loop are the bounds that are not a plain
// default-and-maximum pair: the OAuth TTL has a floor as well, and the action
// timeout has to outlast the longest wait an action offers, which is the
// 3600-second ceiling a pipeline wait imposes on itself. internal/toolutil
// pins that second one against the real constant, which this package cannot
// import; here it is the number the declaration's own comment names.
func TestDurationConstants_KeepEveryDefaultInsideItsOwnBound(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   time.Duration
		ceiling time.Duration
	}{
		{name: "session timeout", value: DefaultSessionTimeout, ceiling: MaxSessionTimeout},
		{name: "revalidate interval", value: DefaultRevalidateInterval, ceiling: MaxRevalidateInterval},
		{name: "pool idle timeout", value: DefaultPoolIdleTimeout, ceiling: MaxPoolIdleTimeout},
		{name: "action timeout", value: DefaultActionTimeout, ceiling: MaxActionTimeout},
		{name: "drain delay", value: DefaultDrainDelay, ceiling: MaxDrainDelay},
		{name: "oauth cache TTL", value: DefaultOAuthCacheTTL, ceiling: MaxOAuthCacheTTL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.ceiling <= 0 {
				t.Errorf("%s maximum = %s: a non-positive ceiling refuses every value an operator "+
					"could set, its own default included", tc.name, tc.ceiling)
			}
			if tc.value > tc.ceiling {
				t.Errorf("%s default = %s is above its maximum of %s: the start that reads it puts "+
					"the default through the same bound check, so a deployment that set nothing would not start",
					tc.name, tc.value, tc.ceiling)
			}
		})
	}

	if DefaultOAuthCacheTTL < MinOAuthCacheTTL {
		t.Errorf("DefaultOAuthCacheTTL = %s is below MinOAuthCacheTTL = %s: cmd/server's "+
			"validateOAuthCacheTTL refuses the default an oauth deployment never overrides",
			DefaultOAuthCacheTTL, MinOAuthCacheTTL)
	}
	if longestWait := 3600 * time.Second; DefaultActionTimeout <= longestWait {
		t.Errorf("DefaultActionTimeout = %s, want more than the %s a pipeline wait caps itself at: "+
			"the deadline starts before the handler does, so a default at or under that ceiling "+
			"cancels an action that was about to return on its own",
			DefaultActionTimeout, longestWait)
	}
}
