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
