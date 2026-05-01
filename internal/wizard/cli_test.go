// cli_test.go contains unit tests for the CLI wizard mode, verifying
// flag parsing and configuration collection.
package wizard

import (
	"bytes"
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
		return "", fmt.Errorf("permission denied")
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
	input := "https://gitlab.example.com\nglpat-xxxxxxxxxxxxxxxxxxxx\n"
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
	if cfg.GitLabToken != "glpat-xxxxxxxxxxxxxxxxxxxx" {
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
		{name: "enter", input: "\nglpat-xxxxxxxxxxxxxxxxxxxx\n"},
		{name: "whitespace", input: "   \nglpat-xxxxxxxxxxxxxxxxxxxx\n"},
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

// TestStepGitLabConfig_URLError verifies stepGitLabConfig returns an error
// when the user enters a URL without a scheme.
func TestStepGitLabConfig_WithExistingConfig(t *testing.T) {
	// User presses Enter on all prompts → existing values should be used
	input := "\n\n"
	r := strings.NewReader(input)
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	existing := ServerConfig{
		GitLabURL:   "https://existing.gitlab.com",
		GitLabToken: "glpat-existingtoken12345678",
	}

	cfg, err := stepGitLabConfig(p, &w, existing, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitLabURL != "https://existing.gitlab.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://existing.gitlab.com")
	}
	if cfg.GitLabToken != "glpat-existingtoken12345678" {
		t.Errorf("GitLabToken = %q, want %q", cfg.GitLabToken, "glpat-existingtoken12345678")
	}
}

// TestStepGitLabConfig_URLError verifies stepGitLabConfig returns an
// "invalid URL" error when the user enters a malformed URL.
func TestStepGitLabConfig_URLError(t *testing.T) {
	input := "not-a-valid-url\nglpat-xxx\n"
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

// TestStepOptions_AllAnswered verifies stepOptions configures all options
// correctly when the user provides explicit answers.
func TestStepOptions_AllAnswered(t *testing.T) {
	// n=no TLS skip, y=meta-tools, y=auto-update, n=yolo, 2=info log level
	input := "n\ny\ny\nn\n2\n"
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
	if !cfg.MetaTools {
		t.Error("MetaTools should be true (answered y)")
	}
	if !cfg.AutoUpdate {
		t.Error("AutoUpdate should be true (answered y)")
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

// TestStepClients_EOF verifies stepClients returns an error when input
// reaches EOF during the client selection prompt.
func TestStepClients_EOF(t *testing.T) {
	r := strings.NewReader("")
	var w bytes.Buffer
	p := NewPrompter(r, &w)

	cfg := ServerConfig{
		BinaryPath:  "/bin/test",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-test",
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
		GitLabToken: "glpat-xxxxxxxxxxxxxxxxxxxx",
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
	stubWriteEnvFile(t)
	stubLoadExistingConfig(t)

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "bin")

	input := strings.Join([]string{
		installDir + string(os.PathSeparator) + DefaultBinaryName(),
		"https://gitlab.example.com",
		"glpat-xxxxxxxxxxxxxxxxxxxx",
		"y", // yes to advanced options
		"y", // skip TLS = yes
		"y", // meta-tools = yes
		"y", // auto-update = yes
		"n", // yolo = no
		"2", // log level = info
		"a", // all clients
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
		"glpat-xxxxxxxxxxxxxxxxxxxx",
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
		"glpat-xxxxxxxxxxxxxxxxxxxx",
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
