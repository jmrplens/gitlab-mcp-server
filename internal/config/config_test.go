// config_test.go contains unit tests for the config package.
// Tests verify [Load] behavior with valid configuration, missing required
// fields, and invalid boolean environment variable values.
package config

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
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
	testCustomRepo     = "custom/group/project"
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

// TestLoad_MetaToolsInvalid verifies that [Load] returns an error when
// META_TOOLS contains an unsupported tool surface value.
func TestLoad_MetaToolsInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("TOOL_SURFACE", "")
	t.Setenv("META_TOOLS", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid META_TOOLS, got nil")
	}
}

// TestLoad_MetaToolsDynamic verifies that META_TOOLS=dynamic selects the
// low-token dynamic tool surface while preserving legacy MetaTools truthiness.
func TestLoad_MetaToolsDynamic(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("META_TOOLS", "dynamic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.ToolSurface != ToolSurfaceDynamic {
		t.Fatalf("ToolSurface = %q, want %q", cfg.ToolSurface, ToolSurfaceDynamic)
	}
	if !cfg.MetaTools {
		t.Fatal("MetaTools = false, want true for dynamic mode")
	}
}

// TestLoad_DefaultToolSurface verifies the empty selector path uses the
// low-token dynamic surface while preserving legacy MetaTools truthiness.
func TestLoad_DefaultToolSurface(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("TOOL_SURFACE", "")
	t.Setenv("META_TOOLS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.ToolSurface != ToolSurfaceDynamic {
		t.Fatalf("ToolSurface = %q, want %q", cfg.ToolSurface, ToolSurfaceDynamic)
	}
	if !cfg.MetaTools {
		t.Fatal("MetaTools = false, want true for dynamic mode")
	}
}

// TestLegacyMetaToolsSelectorInUse verifies that legacy selector detection is
// limited to configurations that set META_TOOLS without TOOL_SURFACE.
func TestLegacyMetaToolsSelectorInUse(t *testing.T) {
	tests := []struct {
		name             string
		toolSurfaceValue string
		metaToolsValue   string
		want             bool
	}{
		{name: "empty values", want: false},
		{name: "legacy only", metaToolsValue: "false", want: true},
		{name: "canonical only", toolSurfaceValue: "individual", want: false},
		{name: "canonical wins", toolSurfaceValue: "dynamic", metaToolsValue: "false", want: false},
		{name: "whitespace legacy only", metaToolsValue: " dynamic ", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LegacyMetaToolsSelectorInUse(tt.toolSurfaceValue, tt.metaToolsValue)
			if got != tt.want {
				t.Fatalf("LegacyMetaToolsSelectorInUse() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLegacyMetaToolsReplacement verifies that legacy META_TOOLS spellings map
// to their canonical TOOL_SURFACE replacements.
func TestLegacyMetaToolsReplacement(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "true", want: ToolSurfaceMeta},
		{value: "false", want: ToolSurfaceIndividual},
		{value: "dynamic", want: ToolSurfaceDynamic},
		{value: "notabool", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := LegacyMetaToolsReplacement(tt.value); got != tt.want {
				t.Fatalf("LegacyMetaToolsReplacement(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestLoad_ToolSurfaceOverridesMetaTools verifies that TOOL_SURFACE is the
// explicit catalog-mode knob when both new and legacy settings are present.
func TestLoad_ToolSurfaceOverridesMetaTools(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("META_TOOLS", "false")
	t.Setenv("TOOL_SURFACE", "dynamic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if cfg.ToolSurface != ToolSurfaceDynamic {
		t.Fatalf("ToolSurface = %q, want %q", cfg.ToolSurface, ToolSurfaceDynamic)
	}
	if !cfg.MetaTools {
		t.Fatal("MetaTools = false, want true for dynamic mode")
	}
}

// TestLoad_ToolSurfaceInvalid verifies that Load rejects unsupported explicit
// tool surface values before falling back to META_TOOLS.
func TestLoad_ToolSurfaceInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("META_TOOLS", "true")
	t.Setenv("TOOL_SURFACE", "not-a-surface")

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
			t.Setenv("TOOL_SURFACE", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf(fmtLoadUnexpected, err)
			}
			if cfg.ToolSurface != tt.want {
				t.Fatalf("ToolSurface = %q, want %q", cfg.ToolSurface, tt.want)
			}
			if !cfg.MetaTools {
				t.Fatal("MetaTools = false, want true for dynamic candidate mode")
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
			t.Setenv("CAPABILITY_SURFACE", value)

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
	t.Setenv("CAPABILITY_SURFACE", "everything")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid CAPABILITY_SURFACE, got nil")
	}
}

// TestEffectiveCapabilitySurface verifies that empty capability settings resolve
// to the full surface while explicit minimal settings are preserved.
func TestEffectiveCapabilitySurface(t *testing.T) {
	if got := EffectiveCapabilitySurface(""); got != CapabilitySurfaceFull {
		t.Fatalf("EffectiveCapabilitySurface(empty) = %q, want %q", got, CapabilitySurfaceFull)
	}
	if got := EffectiveCapabilitySurface(CapabilitySurfaceMinimal); got != CapabilitySurfaceMinimal {
		t.Fatalf("EffectiveCapabilitySurface(minimal) = %q, want %q", got, CapabilitySurfaceMinimal)
	}
}

// TestLoad_MetaParamSchemaDefault verifies that [Load] defaults
// MetaParamSchema to "opaque" when the env var is unset.
func TestLoad_MetaParamSchemaDefault(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	// Pin META_PARAM_SCHEMA to empty so a value loaded from a developer's
	// .env file cannot override the default-case assertion below.
	t.Setenv("META_PARAM_SCHEMA", "")

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
			t.Setenv("META_PARAM_SCHEMA", tc.input)

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
	t.Setenv("META_PARAM_SCHEMA", "verbose")

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
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "5GB")

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
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "2147483648")

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
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "notanumber")

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

// TestLoad_MaxHTTPClients verifies MAX_HTTP_CLIENTS env var parsing.
func TestLoad_MaxHTTPClients(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("MAX_HTTP_CLIENTS", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.MaxHTTPClients != DefaultMaxHTTPClients {
			t.Errorf("MaxHTTPClients = %d, want %d", cfg.MaxHTTPClients, DefaultMaxHTTPClients)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("MAX_HTTP_CLIENTS", "50")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.MaxHTTPClients != 50 {
			t.Errorf("MaxHTTPClients = %d, want 50", cfg.MaxHTTPClients)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("MAX_HTTP_CLIENTS", "not-a-number")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid MAX_HTTP_CLIENTS")
		}
	})
}

// TestLoad_SessionTimeout verifies SESSION_TIMEOUT env var parsing.
func TestLoad_SessionTimeout(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("SESSION_TIMEOUT", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.SessionTimeout != DefaultSessionTimeout {
			t.Errorf("SessionTimeout = %v, want %v", cfg.SessionTimeout, DefaultSessionTimeout)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("SESSION_TIMEOUT", "1h")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.SessionTimeout != time.Hour {
			t.Errorf("SessionTimeout = %v, want 1h", cfg.SessionTimeout)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("SESSION_TIMEOUT", "not-a-duration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid SESSION_TIMEOUT")
		}
	})
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
				GitLabURL:      tt.url,
				GitLabToken:    "test-token",
				MaxHTTPClients: 1,
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

// TestValidate_UploadMaxFileSizeBound verifies that excessively large
// UPLOAD_MAX_FILE_SIZE values are rejected.
func TestValidate_UploadMaxFileSizeBound(t *testing.T) {
	cfg := &Config{
		GitLabURL:         "https://gitlab.example.com",
		GitLabToken:       "test-token",
		MaxHTTPClients:    1,
		UploadMaxFileSize: MaxFileSize + 1,
	}
	err := cfg.validate()
	if err == nil {
		t.Fatal("validate() expected error for oversized UPLOAD_MAX_FILE_SIZE")
	}
}

// TestValidate_MaxHTTPClientsBound verifies that MAX_HTTP_CLIENTS
// beyond the upper bound are rejected.
func TestValidate_MaxHTTPClientsBound(t *testing.T) {
	cfg := &Config{
		GitLabURL:      "https://gitlab.example.com",
		GitLabToken:    "test-token",
		MaxHTTPClients: MaxHTTPClients + 1,
	}
	err := cfg.validate()
	if err == nil {
		t.Fatal("validate() expected error for oversized MAX_HTTP_CLIENTS")
	}
}

// TestValidate_MaxHTTPClientsZero verifies that zero MAX_HTTP_CLIENTS is rejected.
func TestValidate_MaxHTTPClientsZero(t *testing.T) {
	cfg := &Config{
		GitLabURL:      "https://gitlab.example.com",
		GitLabToken:    "test-token",
		MaxHTTPClients: 0,
	}
	err := cfg.validate()
	if err == nil {
		t.Fatal("validate() expected error for zero MAX_HTTP_CLIENTS")
	}
}

// TestValidate_AcceptableMaxValues verifies that values at the exact
// upper bound are accepted.
func TestValidate_AcceptableMaxValues(t *testing.T) {
	cfg := &Config{
		GitLabURL:         "https://gitlab.example.com",
		GitLabToken:       "test-token",
		UploadMaxFileSize: MaxFileSize,
		MaxHTTPClients:    MaxHTTPClients,
		RateLimitRPS:      MaxRateLimitRPS,
		RateLimitBurst:    MaxRateLimitBurst,
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
			cfg := &Config{GitLabURL: "https://gitlab.example.com", GitLabToken: "test-token", MaxHTTPClients: 1, RateLimitBurst: DefaultRateLimitBurst}
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
		MetaTools:         true,
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
	if snapshot.GitLabURL != cfg.GitLabURL || !snapshot.MetaTools || !snapshot.Enterprise() || !snapshot.ReadOnly || !snapshot.SafeMode {
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

// TestLoad_InvalidMetaTools verifies that Load returns an error when
// META_TOOLS has an invalid tool surface value.
func TestLoad_InvalidMetaTools(t *testing.T) {
	t.Setenv("META_TOOLS", "notabool")
	t.Setenv("TOOL_SURFACE", "")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid META_TOOLS")
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
			t.Setenv("GITLAB_ENTERPRISE", "")
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

// TestLoad_DeprecatedEnterpriseEnv verifies the deprecated GITLAB_ENTERPRISE env
// var is honored for back-compat when GITLAB_MCP_TIER is unset (true→ultimate,
// false→free, both explicit), and that GITLAB_MCP_TIER takes precedence over it.
func TestLoad_DeprecatedEnterpriseEnv(t *testing.T) {
	tests := []struct {
		name         string
		tier         string
		enterprise   string
		wantTier     edition.Tier
		wantExplicit bool
	}{
		{name: "enterprise true maps to ultimate", enterprise: "true", wantTier: edition.Ultimate, wantExplicit: true},
		{name: "enterprise false maps to free", enterprise: "false", wantTier: edition.Free, wantExplicit: true},
		{name: "GITLAB_MCP_TIER wins over enterprise", tier: "premium", enterprise: "true", wantTier: edition.Premium, wantExplicit: true},
		{name: "both unset detects", wantTier: edition.Free, wantExplicit: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", "https://gitlab.example.com")
			t.Setenv("GITLAB_TOKEN", "test")
			t.Setenv("GITLAB_MCP_TIER", tc.tier)
			t.Setenv("GITLAB_ENTERPRISE", tc.enterprise)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Tier != tc.wantTier || cfg.TierExplicit != tc.wantExplicit {
				t.Errorf("Tier=%v Explicit=%v, want %v %v", cfg.Tier, cfg.TierExplicit, tc.wantTier, tc.wantExplicit)
			}
		})
	}
	if !LegacyEnterpriseEnvInUse("", "true") {
		t.Error("LegacyEnterpriseEnvInUse(unset tier, enterprise set) should be true")
	}
	if LegacyEnterpriseEnvInUse("premium", "true") {
		t.Error("LegacyEnterpriseEnvInUse should be false when GITLAB_MCP_TIER is set")
	}
}

// TestLoad_InvalidReadOnly verifies that Load returns an error when
// GITLAB_MCP_READ_ONLY has an invalid boolean value.
func TestLoad_InvalidReadOnly(t *testing.T) {
	t.Setenv("GITLAB_MCP_READ_ONLY", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_READ_ONLY")
	}
}

// TestLoad_InvalidUploadMaxFileSize verifies that Load returns an error
// when UPLOAD_MAX_FILE_SIZE has an invalid value.
func TestLoad_InvalidUploadMaxFileSize(t *testing.T) {
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "notanumber")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid UPLOAD_MAX_FILE_SIZE")
	}
}

// TestLoad_InvalidMaxHTTPClients verifies that Load rejects non-integer MAX_HTTP_CLIENTS.
func TestLoad_InvalidMaxHTTPClients(t *testing.T) {
	t.Setenv("MAX_HTTP_CLIENTS", "abc")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "5242880")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid MAX_HTTP_CLIENTS")
	}
}

// TestLoad_InvalidSessionTimeout verifies that Load rejects invalid SESSION_TIMEOUT.
func TestLoad_InvalidSessionTimeout(t *testing.T) {
	t.Setenv("SESSION_TIMEOUT", "notaduration")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "5242880")
	t.Setenv("MAX_HTTP_CLIENTS", "100")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SESSION_TIMEOUT")
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

// TestValidate_AuthMode verifies that validate accepts valid AUTH_MODE values
// and rejects invalid ones.
func TestValidate_AuthMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "empty is valid", mode: "", wantErr: false},
		{name: "legacy is valid", mode: "legacy", wantErr: false},
		{name: "oauth is valid", mode: "oauth", wantErr: false},
		{name: "invalid value", mode: "saml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				GitLabURL:      "https://gitlab.example.com",
				GitLabToken:    "test-token",
				MaxHTTPClients: 1,
				AuthMode:       tt.mode,
				// oauth mode requires the RFC 9728 resource identifier.
				PublicURL: "https://mcp.example.com",
			}
			err := cfg.validate()
			if tt.wantErr && err == nil {
				t.Errorf("validate() for AuthMode %q expected error, got nil", tt.mode)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate() for AuthMode %q unexpected error: %v", tt.mode, err)
			}
		})
	}
}

// TestValidate_OAuthCacheTTL verifies that validate enforces min/max bounds
// on OAuthCacheTTL when it is non-zero.
func TestValidate_OAuthCacheTTL(t *testing.T) {
	tests := []struct {
		name    string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "zero is valid (disabled)", ttl: 0, wantErr: false},
		{name: "at minimum", ttl: MinOAuthCacheTTL, wantErr: false},
		{name: "at maximum", ttl: MaxOAuthCacheTTL, wantErr: false},
		{name: "between bounds", ttl: 30 * time.Minute, wantErr: false},
		{name: "below minimum", ttl: 30 * time.Second, wantErr: true},
		{name: "above maximum", ttl: 3 * time.Hour, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				GitLabURL:      "https://gitlab.example.com",
				GitLabToken:    "test-token",
				MaxHTTPClients: 1,
				OAuthCacheTTL:  tt.ttl,
			}
			err := cfg.validate()
			if tt.wantErr && err == nil {
				t.Errorf("validate() for OAuthCacheTTL %v expected error, got nil", tt.ttl)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate() for OAuthCacheTTL %v unexpected error: %v", tt.ttl, err)
			}
		})
	}
}

// TestLoad_AuthMode verifies AUTH_MODE env var parsing and defaults.
func TestLoad_AuthMode(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run("default is legacy", func(t *testing.T) {
		t.Setenv("AUTH_MODE", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AuthMode != "legacy" {
			t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "legacy")
		}
	})

	t.Run("explicit oauth", func(t *testing.T) {
		t.Setenv("AUTH_MODE", "oauth")
		t.Setenv("PUBLIC_URL", "https://mcp.example.com")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AuthMode != "oauth" {
			t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "oauth")
		}
	})
}

// TestLoad_OAuthCacheTTL verifies OAUTH_CACHE_TTL env var parsing.
func TestLoad_OAuthCacheTTL(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("OAUTH_CACHE_TTL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.OAuthCacheTTL != DefaultOAuthCacheTTL {
			t.Errorf("OAuthCacheTTL = %v, want %v", cfg.OAuthCacheTTL, DefaultOAuthCacheTTL)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("OAUTH_CACHE_TTL", "30m")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.OAuthCacheTTL != 30*time.Minute {
			t.Errorf("OAuthCacheTTL = %v, want 30m", cfg.OAuthCacheTTL)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("OAUTH_CACHE_TTL", "not-a-duration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid OAUTH_CACHE_TTL")
		}
	})
}

// TestLoad_InvalidSafeMode verifies that Load returns an error when
// GITLAB_MCP_SAFE_MODE has an invalid boolean value.
func TestLoad_InvalidSafeMode(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_MCP_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
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
	t.Setenv("EMBEDDED_RESOURCES", "notabool")

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
		{name: "empty gitlab url", cfg: Config{GitLabToken: "test-token", MaxHTTPClients: 1}},
		{name: "invalid tool surface", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", MaxHTTPClients: 1, ToolSurface: "unknown"}},
		{name: "invalid capability surface", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", MaxHTTPClients: 1, CapabilitySurface: "compact"}},
		{name: "negative rate limit", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", MaxHTTPClients: 1, RateLimitRPS: -1}},
		{name: "missing burst for positive rate limit", cfg: Config{GitLabURL: testGitLabURL, GitLabToken: "test-token", MaxHTTPClients: 1, RateLimitRPS: 1, RateLimitBurst: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.validate(); err == nil {
				t.Fatal("validate() expected error, got nil")
			}
		})
	}
}

// TestEffectiveToolSurface verifies explicit and legacy tool-surface resolution.
func TestEffectiveToolSurface(t *testing.T) {
	tests := []struct {
		name        string
		metaTools   bool
		toolSurface string
		want        string
	}{
		{name: "explicit dynamic", metaTools: false, toolSurface: ToolSurfaceDynamic, want: ToolSurfaceDynamic},
		{name: "legacy meta", metaTools: true, want: ToolSurfaceMeta},
		{name: "legacy individual", metaTools: false, want: ToolSurfaceIndividual},
		{name: "unknown falls back to meta", metaTools: true, toolSurface: "unknown", want: ToolSurfaceMeta},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveToolSurface(tt.metaTools, tt.toolSurface); got != tt.want {
				t.Errorf("EffectiveToolSurface() = %q, want %q", got, tt.want)
			}
		})
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
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_MCP_READ_ONLY", "false")
	t.Setenv("GITLAB_MCP_SAFE_MODE", "false")
	t.Setenv("GITLAB_MCP_IGNORE_SCOPES", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_MCP_IGNORE_SCOPES")
	}
}

// TestLoad_SessionTimeoutExceedsMax verifies that Load rejects a SESSION_TIMEOUT
// value that exceeds the maximum allowed duration.
func TestLoad_SessionTimeoutExceedsMax(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("SESSION_TIMEOUT", "25h")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for SESSION_TIMEOUT exceeding maximum")
	}
}

// TestLoad_RevalidateInterval verifies SESSION_REVALIDATE_INTERVAL env var
// parsing: default, custom, invalid, and exceeds-max scenarios.
func TestLoad_RevalidateInterval(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("SESSION_REVALIDATE_INTERVAL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.RevalidateInterval != DefaultRevalidateInterval {
			t.Errorf("RevalidateInterval = %v, want %v", cfg.RevalidateInterval, DefaultRevalidateInterval)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("SESSION_REVALIDATE_INTERVAL", "5m")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.RevalidateInterval != 5*time.Minute {
			t.Errorf("RevalidateInterval = %v, want 5m", cfg.RevalidateInterval)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("SESSION_REVALIDATE_INTERVAL", "notaduration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid SESSION_REVALIDATE_INTERVAL")
		}
	})

	t.Run("exceeds maximum", func(t *testing.T) {
		t.Setenv("SESSION_REVALIDATE_INTERVAL", "25h")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for SESSION_REVALIDATE_INTERVAL exceeding maximum")
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
	t.Setenv("EXCLUDE_TOOLS", "gitlab_create_issue, gitlab_delete_project")

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
				t.Setenv("RATE_LIMIT_RPS", tt.envVal)
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
				t.Setenv("RATE_LIMIT_BURST", tt.envVal)
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
	t.Setenv("RATE_LIMIT_RPS", "10")
	t.Setenv("RATE_LIMIT_BURST", "0")

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

// TestResolveTierEnv_InvalidEnterpriseValue verifies a non-boolean
// GITLAB_ENTERPRISE value yields the explicit invalid-value error instead of
// silently defaulting a tier.
func TestResolveTierEnv_InvalidEnterpriseValue(t *testing.T) {
	_, _, err := resolveTierEnv("", "maybe")
	if err == nil || !strings.Contains(err.Error(), "invalid GITLAB_ENTERPRISE value") {
		t.Errorf("resolveTierEnv err = %v, want invalid GITLAB_ENTERPRISE error", err)
	}
}

// TestLoad_DisableableDurations_AcceptZero verifies that the two settings
// documented as "0 disables" actually accept 0 from the environment.
//
// Both route through parseDisableableDurationEnv rather than the shared
// parseDuration, which rejects any non-positive value. Without that split the
// documented way to switch these off failed at startup, while the equivalent
// CLI flags accepted it. SESSION_TIMEOUT is covered as a control: it documents
// no zero behavior, so zero must still be an error there.
func TestLoad_DisableableDurations_AcceptZero(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run("POOL_IDLE_TIMEOUT=0 disables idle eviction", func(t *testing.T) {
		t.Setenv("POOL_IDLE_TIMEOUT", "0")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.PoolIdleTimeout != 0 {
			t.Errorf("PoolIdleTimeout = %v, want 0", cfg.PoolIdleTimeout)
		}
	})

	t.Run("SESSION_REVALIDATE_INTERVAL=0 disables revalidation", func(t *testing.T) {
		t.Setenv("SESSION_REVALIDATE_INTERVAL", "0")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.RevalidateInterval != 0 {
			t.Errorf("RevalidateInterval = %v, want 0", cfg.RevalidateInterval)
		}
	})

	t.Run("zero stays invalid where it is not documented", func(t *testing.T) {
		t.Setenv("SESSION_TIMEOUT", "0")
		if _, err := Load(); err == nil {
			t.Error("Load() error = nil, want an error for SESSION_TIMEOUT=0")
		}
	})
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

// TestLoad_PoolIdleTimeoutInvalid verifies that Load surfaces a wrapped error
// when POOL_IDLE_TIMEOUT cannot be parsed as a duration or exceeds
// MaxPoolIdleTimeout. This exercises the loadLimitEnv error path distinct
// from the POOL_IDLE_TIMEOUT=0 "disabled" case already covered by
// TestLoad_DisableableDurations_AcceptZero: a non-zero, invalid value must
// still fail startup rather than silently falling back to the default.
func TestLoad_PoolIdleTimeoutInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run("unparseable duration", func(t *testing.T) {
		t.Setenv("POOL_IDLE_TIMEOUT", "notaduration")
		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error for unparseable POOL_IDLE_TIMEOUT")
		}
		if !strings.Contains(err.Error(), "POOL_IDLE_TIMEOUT") {
			t.Errorf("Load() error = %v, want it to mention POOL_IDLE_TIMEOUT", err)
		}
	})

	t.Run("exceeds maximum", func(t *testing.T) {
		t.Setenv("POOL_IDLE_TIMEOUT", "48h")
		_, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error for POOL_IDLE_TIMEOUT exceeding MaxPoolIdleTimeout")
		}
		if !strings.Contains(err.Error(), "exceeds maximum") {
			t.Errorf("Load() error = %v, want it to mention 'exceeds maximum'", err)
		}
	})
}

// TestValidate_OAuthModeRejectsInvalidPublicURLThroughValidate verifies that
// c.validate() itself (not just validatePublicURL in isolation) surfaces the
// RFC 9728 error when AuthMode is "oauth" and PublicURL is empty or
// malformed. This exercises the return path inside validateModeEnums that
// propagates validatePublicURL's error, which a direct call to
// validatePublicURL (see TestValidate_OAuthRequiresPublicURL) does not touch.
func TestValidate_OAuthModeRejectsInvalidPublicURLThroughValidate(t *testing.T) {
	cfg := &Config{
		GitLabURL:      testGitLabURL,
		GitLabToken:    testGitLabToken,
		MaxHTTPClients: 1,
		AuthMode:       "oauth",
		PublicURL:      "",
	}
	err := cfg.validate()
	if err == nil {
		t.Fatal("validate() error = nil, want error for oauth mode without PublicURL")
	}
	if !strings.Contains(err.Error(), "--public-url") {
		t.Errorf("validate() error = %v, want it to mention --public-url", err)
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
	// oauth mode is rejected without a public URL, so that case names the
	// companion setting it needs rather than being dropped: auth mode is one of
	// the readers this test exists to cover.
	for _, tc := range []struct {
		name      string
		env       string
		value     string
		field     string
		alsoEnv   string
		alsoValue string
	}{
		{name: "tool surface", env: "TOOL_SURFACE", value: ToolSurfaceMeta, field: "ToolSurface"},
		{name: "capability surface", env: "CAPABILITY_SURFACE", value: CapabilitySurfaceMinimal, field: "CapabilitySurface"},
		{name: "meta param schema", env: "META_PARAM_SCHEMA", value: MetaParamSchemaFull, field: "MetaParamSchema"},
		{
			name: "auth mode", env: "AUTH_MODE", value: "oauth", field: "AuthMode",
			alsoEnv: "PUBLIC_URL", alsoValue: "https://mcp.example.com",
		},
		{name: "public url", env: "PUBLIC_URL", value: "https://mcp.example.com", field: "PublicURL"},
		{name: "excluded tools", env: "EXCLUDE_TOOLS", value: "gitlab_issue_delete", field: "ExcludeTools"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetDeprecatedEnvUses()
			t.Cleanup(resetDeprecatedEnvUses)
			t.Setenv("GITLAB_URL", testGitLabURL)
			t.Setenv("GITLAB_TOKEN", testGitLabToken)
			t.Setenv(EnvPrefix+tc.env, tc.value)
			if tc.alsoEnv != "" {
				t.Setenv(EnvPrefix+tc.alsoEnv, tc.alsoValue)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() with %s set: %v", EnvPrefix+tc.env, err)
			}

			got := loadedSetting(cfg, tc.field)
			if got != tc.value {
				t.Errorf("%s = %q, want %q", EnvPrefix+tc.env, got, tc.value)
			}
			if warnings := DeprecatedEnvWarnings(); len(warnings) != 0 {
				t.Errorf("loading with only prefixed names warned: %v", warnings)
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
	case "AuthMode":
		return cfg.AuthMode
	case "PublicURL":
		return cfg.PublicURL
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
// It answers what Load answers, the deprecated GITLAB_ENTERPRISE fallback
// included, so the two cannot drift into disagreeing about which catalog a
// deployment serves.
func TestTierFromEnv_ResolvesTheTierWithoutTheRestOfAConfiguration(t *testing.T) {
	cases := []struct {
		name         string
		tier         string
		enterprise   string
		wantTier     edition.Tier
		wantExplicit bool
		wantErr      bool
	}{
		{name: "neither set detects free", wantTier: edition.Free},
		{name: "the tier variable", tier: "ultimate", wantTier: edition.Ultimate, wantExplicit: true},
		{name: "the deprecated enterprise flag", enterprise: "true", wantTier: edition.Ultimate, wantExplicit: true},
		{name: "the tier variable wins over it", tier: "premium", enterprise: "true", wantTier: edition.Premium, wantExplicit: true},
		{name: "an unknown tier is refused", tier: "platinum", wantErr: true},
		{name: "an unparseable enterprise flag is refused", enterprise: "maybe", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvPrefix+"TIER", tc.tier)
			t.Setenv("GITLAB_TIER", "")
			t.Setenv("GITLAB_ENTERPRISE", tc.enterprise)

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

// TestLoad_ActionTimeoutAndDrainDelayInvalid verifies the two limits parsed
// last in the limit block refuse a value outside their range through Load,
// which is the path a deployment actually takes.
func TestLoad_ActionTimeoutAndDrainDelayInvalid(t *testing.T) {
	cases := []struct {
		name  string
		env   string
		value string
	}{
		{name: "action timeout above its maximum", env: "ACTION_TIMEOUT", value: "48h"},
		{name: "action timeout unparseable", env: "ACTION_TIMEOUT", value: "soon"},
		{name: "drain delay above its maximum", env: "DRAIN_DELAY", value: "1h"},
		{name: "drain delay unparseable", env: "DRAIN_DELAY", value: "later"},
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
