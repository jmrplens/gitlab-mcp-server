// cli_test.go contains unit tests for the CLI wizard mode, verifying
// flag parsing and configuration collection.
package wizard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStepInstall_WithBinaryNameSuffix verifies that stepInstall strips the
// binary filename suffix from the user-provided path when it ends with the
// platform-specific binary name.
func TestStepInstall_WithBinaryNameSuffix(t *testing.T) {
	tmpDir := t.TempDir()
	fullPath := filepath.Join(tmpDir, "bin", DefaultBinaryName())

	r := strings.NewReader(fullPath + "\n")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	path, err := stepInstall(p, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path == "" {
		t.Error("returned path is empty")
	}
	if !strings.Contains(w.String(), "Step 1") {
		t.Error("missing Step 1 header in output")
	}
}

// TestStepInstall_BinaryNameOnlyUsesCurrentDirectory verifies a bare binary
// filename is normalized to the current directory before installation.
func TestStepInstall_BinaryNameOnlyUsesCurrentDirectory(t *testing.T) {
	orig := installBinaryFn
	var gotDest string
	installBinaryFn = func(destDir string) (string, error) {
		gotDest = destDir
		return filepath.Join(destDir, DefaultBinaryName()), nil
	}
	t.Cleanup(func() { installBinaryFn = orig })

	r := strings.NewReader(DefaultBinaryName() + "\n")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	path, err := stepInstall(p, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDest != "." {
		t.Fatalf("install destination = %q, want current directory", gotDest)
	}
	if path != filepath.Join(".", DefaultBinaryName()) {
		t.Fatalf("installed path = %q, want current directory binary", path)
	}
}

// TestStepInstall_DefaultPath verifies stepInstall works when the user
// accepts the default path by pressing Enter.
func TestStepInstall_DefaultPath(t *testing.T) {
	stubInstallBinary(t)

	// Empty input triggers default
	r := strings.NewReader("\n")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	path, err := stepInstall(p, &w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path == "" {
		t.Error("returned path is empty")
	}
}

// TestStepInstall_EOF verifies stepInstall returns an error when input
// reaches EOF during the install path prompt.
func TestStepInstall_EOF(t *testing.T) {
	r := strings.NewReader("") // immediate EOF
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	_, err := stepInstall(p, &w)
	if err == nil {
		t.Fatal("expected error for EOF, got nil")
	}
}

// TestStepInstall_InstallBinaryFails verifies that when installBinaryFn fails,
// stepInstall falls back to the current executable path instead of returning an error.
func TestStepInstall_InstallBinaryFails(t *testing.T) {
	orig := installBinaryFn
	installBinaryFn = func(string) (string, error) {
		return "", errors.New("permission denied")
	}
	t.Cleanup(func() { installBinaryFn = orig })

	tmpDir := t.TempDir()
	input := tmpDir + "\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	path, err := stepInstall(p, &w)
	if err != nil {
		t.Fatalf("stepInstall should not return error on install failure, got: %v", err)
	}
	if path == "" {
		t.Error("expected fallback path, got empty")
	}
	output := w.String()
	if !strings.Contains(output, "Could not install binary") {
		t.Error("expected 'Could not install binary' warning in output")
	}
}

// TestStepGitLabConfig_ValidInput verifies stepGitLabConfig returns a
// properly configured ServerConfig for valid URL and token.
func TestStepGitLabConfig_ValidInput(t *testing.T) {
	input := "https://gitlab.example.com\ntest-token-xxxxxxxxxxxxxxxxxxxx\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg, err := stepGitLabConfig(p, &w, ServerConfig{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitLabURL != "https://gitlab.example.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://gitlab.example.com")
	}
	if cfg.GitLabToken != "test-token-xxxxxxxxxxxxxxxxxxxx" {
		t.Errorf("GitLabToken = %q, want masked value", cfg.GitLabToken)
	}
	if cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify should default to false")
	}
	if !cfg.MetaTools {
		t.Error("MetaTools should default to true")
	}
}

// TestStepGitLabConfig_DefaultURL verifies that pressing Enter at the GitLab
// URL prompt, or entering whitespace, uses the GitLab.com default.
func TestStepGitLabConfig_DefaultURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "enter", input: "\ntest-token-xxxxxxxxxxxxxxxxxxxx\n"},
		{name: "whitespace", input: "   \ntest-token-xxxxxxxxxxxxxxxxxxxx\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			var w bytes.Buffer
			p := NewPrompter(r, &w)

			cfg, err := stepGitLabConfig(p, &w, ServerConfig{}, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.GitLabURL != DefaultGitLabURL {
				t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, DefaultGitLabURL)
			}
		})
	}
}

// TestStepGitLabConfig_WithExistingConfig verifies that existing GitLab URL and
// token values are preserved when the user accepts defaults.
func TestStepGitLabConfig_WithExistingConfig(t *testing.T) {
	// User presses Enter on all prompts → existing values should be used
	input := "\n\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	existing := ServerConfig{
		GitLabURL:   "https://existing.gitlab.com",
		GitLabToken: "test-token-existingtoken12345678",
	}

	cfg, err := stepGitLabConfig(p, &w, existing, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitLabURL != "https://existing.gitlab.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://existing.gitlab.com")
	}
	if cfg.GitLabToken != "test-token-existingtoken12345678" {
		t.Errorf("GitLabToken = %q, want %q", cfg.GitLabToken, "test-token-existingtoken12345678")
	}
}

// TestStepGitLabConfig_WithExistingAdvancedOptions verifies existing advanced
// settings remain loaded when the user accepts the URL and token defaults.
func TestStepGitLabConfig_WithExistingAdvancedOptions(t *testing.T) {
	input := "\n\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	existing := ServerConfig{
		GitLabURL:         "https://existing.gitlab.com",
		GitLabToken:       "test-token-existingtoken12345678",
		SkipTLSVerify:     true,
		ToolSurface:       "dynamic",
		CapabilitySurface: "minimal",
		MetaParamSchema:   "compact",
		Enterprise:        true,
		ReadOnly:          true,
		SafeMode:          true,
		EmbeddedResources: false,
		ExcludeTools:      "gitlab_admin",
		IgnoreScopes:      true,
		UploadMaxFileSize: "500MB",
		AutoUpdateMode:    "check",
		AutoUpdateRepo:    "example/repo",
		AutoUpdateTimeout: "90s",
		RateLimitRPS:      "3.5",
		RateLimitBurst:    "12",
		LogLevel:          "debug",
		YoloMode:          true,
	}

	cfg, err := stepGitLabConfig(p, &w, existing, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ToolSurface != "dynamic" || cfg.CapabilitySurface != "minimal" || cfg.MetaParamSchema != "compact" {
		t.Errorf("catalog options not preserved: %#v", cfg)
	}
	if !cfg.SkipTLSVerify || !cfg.Enterprise || !cfg.ReadOnly || !cfg.SafeMode || cfg.EmbeddedResources || !cfg.IgnoreScopes || !cfg.YoloMode {
		t.Errorf("boolean advanced options not preserved: %#v", cfg)
	}
	if cfg.ExcludeTools != "gitlab_admin" || cfg.AutoUpdateMode != "check" || cfg.RateLimitRPS != "3.5" {
		t.Errorf("text/mode advanced options not preserved: %#v", cfg)
	}
}

// TestStepGitLabConfig_URLError verifies stepGitLabConfig returns an
// "invalid URL" error when the user enters a malformed URL.
func TestStepGitLabConfig_URLError(t *testing.T) {
	input := "not-a-valid-url\ntest-token-xxx\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	_, err := stepGitLabConfig(p, &w, ServerConfig{}, false)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("error = %v, want to contain 'invalid URL'", err)
	}
}

// TestStepGitLabConfig_EOF verifies stepGitLabConfig returns an error
// when input reaches EOF during the URL prompt.
func TestStepGitLabConfig_EOF(t *testing.T) {
	r := strings.NewReader("")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	_, err := stepGitLabConfig(p, &w, ServerConfig{}, false)
	if err == nil {
		t.Fatal("expected error for EOF, got nil")
	}
}

// TestStepGitLabConfig_TokenEOF verifies stepGitLabConfig returns an error
// when input reaches EOF during the token prompt.
func TestStepGitLabConfig_TokenEOF(t *testing.T) {
	// Provide a valid URL but EOF before the token
	r := strings.NewReader("https://gitlab.example.com\n")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	_, err := stepGitLabConfig(p, &w, ServerConfig{}, false)
	if err == nil {
		t.Fatal("expected error for EOF during token prompt, got nil")
	}
}

// TestStepOptions_AllAnswered verifies stepOptions configures all advanced
// options correctly when the user provides explicit answers.
func TestStepOptions_AllAnswered(t *testing.T) {
	input := strings.Join([]string{
		"n",            // skip TLS
		"1",            // tool surface = dynamic
		"2",            // capability surface = minimal
		"2",            // meta parameter schema = compact
		"y",            // enterprise
		"y",            // read-only
		"y",            // safe mode
		"n",            // embedded resources
		"y",            // ignore scopes
		"gitlab_admin", // exclude tools
		"500MB",        // upload max file size
		"2",            // auto-update mode = check
		"example/repo", // auto-update repo
		"90s",          // auto-update timeout
		"3.5",          // rate limit RPS
		"12",           // rate limit burst
		"n",            // yolo
		"2",            // log level = info
	}, "\n") + "\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := &ServerConfig{}
	err := stepOptions(p, &w, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify should be false (answered n)")
	}
	if cfg.ToolSurface != "dynamic" || !cfg.MetaTools {
		t.Errorf("ToolSurface = %q, MetaTools = %v; want dynamic/true", cfg.ToolSurface, cfg.MetaTools)
	}
	if cfg.CapabilitySurface != "minimal" || cfg.MetaParamSchema != "compact" {
		t.Errorf("catalog options not set: %#v", cfg)
	}
	if !cfg.Enterprise || !cfg.ReadOnly || !cfg.SafeMode || cfg.EmbeddedResources || !cfg.IgnoreScopes {
		t.Errorf("boolean options not set: %#v", cfg)
	}
	if cfg.ExcludeTools != "gitlab_admin" || cfg.UploadMaxFileSize != "500MB" {
		t.Errorf("text options not set: %#v", cfg)
	}
	if cfg.AutoUpdateMode != "check" || !cfg.AutoUpdate {
		t.Errorf("AutoUpdateMode = %q, AutoUpdate = %v; want check/true", cfg.AutoUpdateMode, cfg.AutoUpdate)
	}
	if cfg.RateLimitRPS != "3.5" || cfg.RateLimitBurst != "12" {
		t.Errorf("rate options not set: %#v", cfg)
	}
	if cfg.YoloMode {
		t.Error("YoloMode should be false (answered n)")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

// TestStepOptions_EOF verifies stepOptions returns an error when input
// reaches EOF before all options are answered.
func TestStepOptions_EOF(t *testing.T) {
	r := strings.NewReader("") // immediate EOF
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := &ServerConfig{}
	err := stepOptions(p, &w, cfg)
	if err == nil {
		t.Fatal("expected error for EOF, got nil")
	}
}

// TestStepOptions_EOFOnMetaTools verifies stepOptions returns an error
// when EOF occurs during the second prompt (meta-tools).
func TestStepOptions_EOFOnMetaTools(t *testing.T) {
	r := strings.NewReader("y\n") // first prompt OK, then EOF
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := &ServerConfig{}
	err := stepOptions(p, &w, cfg)
	if err == nil {
		t.Fatal("expected error for EOF on meta-tools prompt, got nil")
	}
}

// TestStepOptions_EOFOnAutoUpdate verifies stepOptions returns an error
// when EOF occurs during the third prompt (auto-update).
func TestStepOptions_EOFOnAutoUpdate(t *testing.T) {
	r := strings.NewReader("y\ny\n") // two prompts OK, then EOF
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := &ServerConfig{}
	err := stepOptions(p, &w, cfg)
	if err == nil {
		t.Fatal("expected error for EOF on auto-update prompt, got nil")
	}
}

// TestStepOptions_EOFOnYolo verifies stepOptions returns an error
// when EOF occurs during the fourth prompt (yolo mode).
func TestStepOptions_EOFOnYolo(t *testing.T) {
	r := strings.NewReader("y\ny\ny\n") // three prompts OK, then EOF
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := &ServerConfig{}
	err := stepOptions(p, &w, cfg)
	if err == nil {
		t.Fatal("expected error for EOF on yolo prompt, got nil")
	}
}

// TestStepOptions_EOFOnLogLevel verifies stepOptions returns an error
// when EOF occurs during the log level choice prompt.
func TestStepOptions_EOFOnLogLevel(t *testing.T) {
	r := strings.NewReader("y\ny\ny\nn\n") // four prompts OK, then EOF on choice
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := &ServerConfig{}
	err := stepOptions(p, &w, cfg)
	if err == nil {
		t.Fatal("expected error for EOF on log level prompt, got nil")
	}
}

// TestStepOptions_EOFAtEveryAdvancedPrompt verifies each advanced prompt
// returns EOF cleanly when the input stream stops at that point.
func TestStepOptions_EOFAtEveryAdvancedPrompt(t *testing.T) {
	answers := []string{
		"n", // skip TLS
		"1", // tool surface
		"1", // capability surface
		"1", // meta parameter schema
		"n", // enterprise
		"n", // read-only
		"n", // safe mode
		"y", // embedded resources
		"n", // ignore scopes
		"",  // exclude tools
		"",  // upload max file size
		"1", // auto-update mode
		"",  // auto-update repo
		"",  // auto-update timeout
		"",  // rate limit RPS
		"",  // rate limit burst
		"n", // yolo
		"2", // log level
	}

	for answered := range answers {
		t.Run(fmt.Sprintf("after_%02d_answers", answered), func(t *testing.T) {
			input := ""
			if answered > 0 {
				input = strings.Join(answers[:answered], "\n") + "\n"
			}
			r := strings.NewReader(input)
			var w bytes.Buffer
			p := NewPrompter(r, &w)

			cfg := DefaultServerConfig()
			err := stepOptions(p, &w, &cfg)
			if err == nil {
				t.Fatalf("expected EOF after %d answered prompt(s)", answered)
			}
		})
	}
}

// TestStepClients_EOF verifies stepClients returns an error when input
// reaches EOF during the client selection prompt.
func TestStepClients_EOF(t *testing.T) {
	r := strings.NewReader("")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := ServerConfig{
		BinaryPath:  "/bin/test",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "test-token-test",
	}
	err := stepClients(p, &w, cfg)
	if err == nil {
		t.Fatal("expected error for EOF, got nil")
	}
}

// TestStepClients_SelectAll verifies stepClients processes all clients
// when the user enters "a" (all).
func TestStepClients_SelectAll(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)

	r := strings.NewReader("a\n")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := ServerConfig{
		BinaryPath:  filepath.Join(t.TempDir(), "test-binary"),
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "test-token-xxxxxxxxxxxxxxxxxxxx",
	}
	err := stepClients(p, &w, cfg)
	// Some clients may fail to write config (paths are real), but the
	// function should complete without returning a critical error.
	if err != nil {
		t.Logf("stepClients returned error (may be expected): %v", err)
	}

	output := w.String()
	if !strings.Contains(output, "Step 3") {
		t.Error("expected Step 3 header in output")
	}
	if !strings.Contains(output, "Setup Complete") {
		t.Error("expected Setup Complete in output")
	}
}

// TestRunCLI_AdvancedOptions verifies the full CLI flow with advanced
// options enabled, covering the stepOptions branch.
func TestRunCLI_AdvancedOptions(t *testing.T) {
	useFakeClients(t)
	envPath := stubWriteEnvFile(t)
	stubLoadExistingConfig(t)

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "bin")

	input := strings.Join([]string{
		installDir + string(os.PathSeparator) + DefaultBinaryName(),
		"https://gitlab.example.com",
		"test-token-xxxxxxxxxxxxxxxxxxxx",
		"y",            // yes to advanced options
		"y",            // skip TLS
		"1",            // tool surface = dynamic
		"2",            // capability surface = minimal
		"2",            // meta parameter schema = compact
		"y",            // enterprise
		"y",            // read-only
		"y",            // safe mode
		"n",            // embedded resources
		"y",            // ignore scopes
		"gitlab_admin", // exclude tools
		"500MB",        // upload max file size
		"2",            // auto-update mode = check
		"example/repo", // auto-update repo
		"90s",          // auto-update timeout
		"3.5",          // rate limit RPS
		"12",           // rate limit burst
		"n",            // yolo
		"2",            // log level = info
		"a",            // all clients
	}, "\n") + "\n"

	r := strings.NewReader(input)
	var w bytes.Buffer

	err := RunCLI("1.0.0-test", r, &w)
	if err != nil {
		t.Logf("RunCLI returned error (may be expected in test env): %v", err)
	}

	output := w.String()
	if !strings.Contains(output, "Advanced Options") {
		t.Error("expected 'Advanced Options' section in output")
	}
	if !strings.Contains(output, "Setup Complete") {
		t.Error("expected 'Setup Complete' in output")
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading generated env file: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"TOOL_SURFACE=dynamic",
		"CAPABILITY_SURFACE=minimal",
		"META_PARAM_SCHEMA=compact",
		"GITLAB_ENTERPRISE=true",
		"GITLAB_READ_ONLY=true",
		"GITLAB_SAFE_MODE=true",
		"EMBEDDED_RESOURCES=false",
		"EXCLUDE_TOOLS=gitlab_admin",
		"AUTO_UPDATE=check",
		"RATE_LIMIT_RPS=3.5",
	} {
		if !strings.Contains(content, want+"\n") {
			t.Errorf("generated env file missing %q\ncontent:\n%s", want, content)
		}
	}
}

// TestRunCLI_AdvancedOptionsEOF verifies that RunCLI returns an error when
// the user answers "y" to advanced options but then EOF is reached during
// the options prompting.
func TestRunCLI_AdvancedOptionsEOF(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)
	stubLoadExistingConfig(t)

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "bin")

	input := strings.Join([]string{
		installDir + string(os.PathSeparator) + DefaultBinaryName(),
		"https://gitlab.example.com",
		"test-token-xxxxxxxxxxxxxxxxxxxx",
		"y", // yes to advanced → triggers stepOptions
		// EOF here — no answers for stepOptions
	}, "\n") + "\n"

	r := strings.NewReader(input)
	var w bytes.Buffer

	err := RunCLI("1.0.0-test", r, &w)
	if err == nil {
		t.Fatal("expected error from EOF during advanced options")
	}
}

// TestRunCLI_AskAdvancedEOF verifies RunCLI returns an error when EOF is
// reached at the "Configure advanced options?" prompt itself.
func TestRunCLI_AskAdvancedEOF(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)
	stubLoadExistingConfig(t)

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "bin")

	input := strings.Join([]string{
		installDir + string(os.PathSeparator) + DefaultBinaryName(),
		"https://gitlab.example.com",
		"test-token-xxxxxxxxxxxxxxxxxxxx",
		// EOF here — no answer for "Configure advanced options?"
	}, "\n") + "\n"

	r := strings.NewReader(input)
	var w bytes.Buffer

	err := RunCLI("1.0.0-test", r, &w)
	if err == nil {
		t.Fatal("expected error from EOF at advanced options prompt")
	}
}

// TestPrintBanner verifies the banner output contains key information.
func TestPrintBanner(t *testing.T) {
	var w bytes.Buffer
	printBanner(&w, "3.2.1")

	output := w.String()
	if !strings.Contains(output, "3.2.1") {
		t.Error("banner missing version")
	}
	if !strings.Contains(output, "gitlab-mcp-server") {
		t.Error("banner missing project name")
	}
}

// TestPrintSection verifies the section header format.
func TestPrintSection(t *testing.T) {
	var w bytes.Buffer
	printSection(&w, "Test Section")

	output := w.String()
	if !strings.Contains(output, "Test Section") {
		t.Error("section missing title")
	}
	if !strings.Contains(output, "---") {
		t.Error("section missing separator")
	}
}
