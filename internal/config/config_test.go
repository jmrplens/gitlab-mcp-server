// config_test.go contains unit tests for the config package.
// Tests verify [Load] behavior with valid configuration, missing required
// fields, and invalid boolean environment variable values.

package config

import (
	"testing"
	"time"
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
	fmtAutoUpdateWant  = "AutoUpdate = %q, want %q"
	testCustomRepo     = "custom/group/project"
)

// TestLoad_ValidConfig verifies that [Load] returns a fully populated [Config]
// when all required environment variables are set with valid values.
func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "")

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

// TestLoad_MissingURL verifies that [Load] returns an error when GITLAB_URL
// is empty, since it is a required configuration field.
func TestLoad_MissingURL(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", testGitLabToken)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error when GITLAB_URL is empty, got nil")
	}
}

// TestLoad_MissingToken verifies that [Load] returns an error when GITLAB_TOKEN
// is empty, since it is a required configuration field.
func TestLoad_MissingToken(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error when GITLAB_TOKEN is empty, got nil")
	}
}

// TestLoad_SkipTLSVerifyTrue verifies that [Load] correctly parses
// GITLAB_SKIP_TLS_VERIFY="true" and sets [Config.SkipTLSVerify] to true.
func TestLoad_SkipTLSVerifyTrue(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf(fmtLoadUnexpected, err)
	}
	if !cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify = false, want true")
	}
}

// TestLoad_SkipTLSVerifyInvalid verifies that [Load] returns an error when
// GITLAB_SKIP_TLS_VERIFY contains a non-boolean string.
func TestLoad_SkipTLSVerifyInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid GITLAB_SKIP_TLS_VERIFY, got nil")
	}
}

// TestLoad_MetaToolsInvalid verifies that [Load] returns an error when
// META_TOOLS contains a non-boolean string.
func TestLoad_MetaToolsInvalid(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid META_TOOLS, got nil")
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

// TestLoad_AutoUpdate verifies AUTO_UPDATE env var parsing and defaults.
func TestLoad_AutoUpdate(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run("default value is true", func(t *testing.T) {
		t.Setenv("AUTO_UPDATE", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdate != "true" {
			t.Errorf(fmtAutoUpdateWant, cfg.AutoUpdate, "true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		t.Setenv("AUTO_UPDATE", "false")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdate != "false" {
			t.Errorf(fmtAutoUpdateWant, cfg.AutoUpdate, "false")
		}
	})

	t.Run("check mode", func(t *testing.T) {
		t.Setenv("AUTO_UPDATE", "check")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdate != "check" {
			t.Errorf(fmtAutoUpdateWant, cfg.AutoUpdate, "check")
		}
	})
}

// TestLoad_AutoUpdateRepo verifies AUTO_UPDATE_REPO env var parsing and default.
func TestLoad_AutoUpdateRepo(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_REPO", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdateRepo != DefaultAutoUpdateRepo {
			t.Errorf("AutoUpdateRepo = %q, want %q", cfg.AutoUpdateRepo, DefaultAutoUpdateRepo)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_REPO", testCustomRepo)
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdateRepo != testCustomRepo {
			t.Errorf("AutoUpdateRepo = %q, want %q", cfg.AutoUpdateRepo, testCustomRepo)
		}
	})
}

// TestLoad_AutoUpdateInterval verifies AUTO_UPDATE_INTERVAL env var parsing.
func TestLoad_AutoUpdateInterval(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_INTERVAL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdateInterval != DefaultAutoUpdateInterval {
			t.Errorf("AutoUpdateInterval = %v, want %v", cfg.AutoUpdateInterval, DefaultAutoUpdateInterval)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_INTERVAL", "30m")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdateInterval != 30*time.Minute {
			t.Errorf("AutoUpdateInterval = %v, want 30m", cfg.AutoUpdateInterval)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_INTERVAL", "not-a-duration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid AUTO_UPDATE_INTERVAL")
		}
	})
}

// TestLoad_AutoUpdateTimeout verifies AUTO_UPDATE_TIMEOUT env var parsing, default, and bounds.
func TestLoad_AutoUpdateTimeout(t *testing.T) {
	t.Setenv("GITLAB_URL", testHTTPExampleURL)
	t.Setenv("GITLAB_TOKEN", "test")

	t.Run(subtestDefault, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_TIMEOUT", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdateTimeout != DefaultAutoUpdateTimeout {
			t.Errorf("AutoUpdateTimeout = %v, want %v", cfg.AutoUpdateTimeout, DefaultAutoUpdateTimeout)
		}
	})

	t.Run(subtestCustom, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_TIMEOUT", "90s")
		cfg, err := Load()
		if err != nil {
			t.Fatalf(fmtLoadErr, err)
		}
		if cfg.AutoUpdateTimeout != 90*time.Second {
			t.Errorf("AutoUpdateTimeout = %v, want 90s", cfg.AutoUpdateTimeout)
		}
	})

	t.Run(subtestInvalid, func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_TIMEOUT", "not-a-duration")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid AUTO_UPDATE_TIMEOUT")
		}
	})

	t.Run("below_minimum", func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_TIMEOUT", "1s")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for AUTO_UPDATE_TIMEOUT below minimum")
		}
	})

	t.Run("above_maximum", func(t *testing.T) {
		t.Setenv("AUTO_UPDATE_TIMEOUT", "15m")
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for AUTO_UPDATE_TIMEOUT above maximum")
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
	}
	err := cfg.validate()
	if err != nil {
		t.Errorf("validate() unexpected error for max values: %v", err)
	}
}

// TestLoad_InvalidSkipTLS verifies that Load returns an error when
// GITLAB_SKIP_TLS_VERIFY has an invalid boolean value.
func TestLoad_InvalidSkipTLS(t *testing.T) {
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_SKIP_TLS_VERIFY")
	}
}

// TestLoad_InvalidMetaTools verifies that Load returns an error when
// META_TOOLS has an invalid boolean value.
func TestLoad_InvalidMetaTools(t *testing.T) {
	t.Setenv("META_TOOLS", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid META_TOOLS")
	}
}

// TestLoad_InvalidEnterprise verifies that Load returns an error when
// GITLAB_ENTERPRISE has an invalid boolean value.
func TestLoad_InvalidEnterprise(t *testing.T) {
	t.Setenv("GITLAB_ENTERPRISE", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_ENTERPRISE")
	}
}

// TestLoad_InvalidReadOnly verifies that Load returns an error when
// GITLAB_READ_ONLY has an invalid boolean value.
func TestLoad_InvalidReadOnly(t *testing.T) {
	t.Setenv("GITLAB_READ_ONLY", "notabool")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_READ_ONLY")
	}
}

// TestLoad_InvalidUploadMaxFileSize verifies that Load returns an error
// when UPLOAD_MAX_FILE_SIZE has an invalid value.
func TestLoad_InvalidUploadMaxFileSize(t *testing.T) {
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "notanumber")
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "test")
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_READ_ONLY", "false")
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
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_READ_ONLY", "false")
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
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_READ_ONLY", "false")
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

// TestValidate_AutoUpdateTimeout verifies that validate enforces min/max bounds
// on AutoUpdateTimeout when it is non-zero (covers HTTP-mode direct construction).
func TestValidate_AutoUpdateTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{name: "zero is valid (uses default)", timeout: 0, wantErr: false},
		{name: "at minimum", timeout: MinAutoUpdateTimeout, wantErr: false},
		{name: "at maximum", timeout: MaxAutoUpdateTimeout, wantErr: false},
		{name: "between bounds", timeout: 2 * time.Minute, wantErr: false},
		{name: "below minimum", timeout: 1 * time.Second, wantErr: true},
		{name: "above maximum", timeout: 15 * time.Minute, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				GitLabURL:         "https://gitlab.example.com",
				GitLabToken:       "test-token",
				MaxHTTPClients:    1,
				AutoUpdateTimeout: tt.timeout,
			}
			err := cfg.validate()
			if tt.wantErr && err == nil {
				t.Errorf("validate() for AutoUpdateTimeout %v expected error, got nil", tt.timeout)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate() for AutoUpdateTimeout %v unexpected error: %v", tt.timeout, err)
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
// GITLAB_SAFE_MODE has an invalid boolean value.
func TestLoad_InvalidSafeMode(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_READ_ONLY", "false")
	t.Setenv("GITLAB_SAFE_MODE", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_SAFE_MODE")
	}
}

// TestLoad_InvalidIgnoreScopes verifies that Load returns an error when
// GITLAB_IGNORE_SCOPES has an invalid boolean value.
func TestLoad_InvalidIgnoreScopes(t *testing.T) {
	t.Setenv("GITLAB_URL", testGitLabURL)
	t.Setenv("GITLAB_TOKEN", testGitLabToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "false")
	t.Setenv("META_TOOLS", "true")
	t.Setenv("GITLAB_ENTERPRISE", "false")
	t.Setenv("GITLAB_READ_ONLY", "false")
	t.Setenv("GITLAB_SAFE_MODE", "false")
	t.Setenv("GITLAB_IGNORE_SCOPES", "notabool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITLAB_IGNORE_SCOPES")
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
