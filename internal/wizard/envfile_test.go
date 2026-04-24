// envfile_test.go contains unit tests for .env file reading, writing,
// and credential management.
package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEnvFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	cfg := ServerConfig{
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "glpat-abc123",
		SkipTLSVerify: true,
	}

	got, err := writeEnvFileToPath(path, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "GITLAB_URL=https://gitlab.example.com") {
		t.Error("missing GITLAB_URL")
	}
	if !strings.Contains(content, "GITLAB_TOKEN=glpat-abc123") {
		t.Error("missing GITLAB_TOKEN")
	}
	if !strings.Contains(content, "GITLAB_SKIP_TLS_VERIFY=true") {
		t.Error("missing GITLAB_SKIP_TLS_VERIFY")
	}
}

func TestWriteEnvFile_OmitsSkipTLS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	cfg := ServerConfig{
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "glpat-abc123",
		SkipTLSVerify: false,
	}

	if _, err := writeEnvFileToPath(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "GITLAB_SKIP_TLS_VERIFY") {
		t.Error("should not contain GITLAB_SKIP_TLS_VERIFY when false")
	}
}

func TestEnvFilePath_InHome(t *testing.T) {
	path := EnvFilePath()
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) {
		t.Errorf("EnvFilePath() = %q, should start with %q", path, home)
	}
	if !strings.HasSuffix(path, EnvFileName) {
		t.Errorf("EnvFilePath() = %q, should end with %q", path, EnvFileName)
	}
}

func TestLoadExistingConfigFromPath_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "# gitlab-mcp-server config\n" +
		"GITLAB_URL=https://gitlab.example.com\n" +
		"GITLAB_TOKEN=glpat-abc123def456\n" +
		"GITLAB_SKIP_TLS_VERIFY=true\n"

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if !found {
		t.Fatal("expected found=true for valid env file")
	}
	if cfg.GitLabURL != "https://gitlab.example.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://gitlab.example.com")
	}
	if cfg.GitLabToken != "glpat-abc123def456" {
		t.Errorf("GitLabToken = %q, want %q", cfg.GitLabToken, "glpat-abc123def456")
	}
	if !cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify should be true")
	}
}

func TestLoadExistingConfigFromPath_FileNotExists(t *testing.T) {
	_, found := loadExistingConfigFromPath(filepath.Join(t.TempDir(), "nonexistent.env"))
	if found {
		t.Error("expected found=false for nonexistent file")
	}
}

func TestLoadExistingConfigFromPath_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	if err := os.WriteFile(path, []byte(""), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, found := loadExistingConfigFromPath(path)
	if found {
		t.Error("expected found=false for empty file")
	}
}

func TestLoadExistingConfigFromPath_OnlyComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "# Just comments\n# Another comment\n\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, found := loadExistingConfigFromPath(path)
	if found {
		t.Error("expected found=false for file with only comments")
	}
}

func TestLoadExistingConfigFromPath_SkipTLSFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\nGITLAB_SKIP_TLS_VERIFY=false\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if !found {
		t.Fatal("expected found=true")
	}
	if cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify should be false")
	}
}

func TestLoadExistingConfigFromPath_SensibleDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, _ := loadExistingConfigFromPath(path)
	if !cfg.MetaTools {
		t.Error("MetaTools should default to true")
	}
	if !cfg.AutoUpdate {
		t.Error("AutoUpdate should default to true")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

// TestWriteEnvFile_Wrapper verifies the public WriteEnvFile function
// delegates to writeEnvFileToPath with the path from EnvFilePath().
// We override HOME to avoid writing to the real home directory.
func TestWriteEnvFile_Wrapper(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := ServerConfig{
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-wrapper-test",
	}

	path, err := WriteEnvFile(cfg)
	if err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}
	if path == "" {
		t.Fatal("WriteEnvFile returned empty path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !strings.Contains(string(data), "glpat-wrapper-test") {
		t.Error("written file does not contain expected token")
	}
}
