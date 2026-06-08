// envfile_test.go contains unit tests for .env file reading, writing,
// and credential management.
package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteEnvFile_CreatesFile verifies writeEnvFileToPath creates the .env
// file at the requested path and writes GITLAB_URL, GITLAB_TOKEN, and
// GITLAB_SKIP_TLS_VERIFY lines when SkipTLSVerify is true.
func TestWriteEnvFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	cfg := ServerConfig{
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "test-token-abc123",
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
	if !strings.Contains(content, "GITLAB_TOKEN=test-token-abc123") {
		t.Error("missing GITLAB_TOKEN")
	}
	if !strings.Contains(content, "GITLAB_SKIP_TLS_VERIFY=true") {
		t.Error("missing GITLAB_SKIP_TLS_VERIFY")
	}
}

// TestWriteEnvFile_WritesFalseSkipTLS verifies writeEnvFileToPath records
// GITLAB_SKIP_TLS_VERIFY=false so the generated env file is explicit.
func TestWriteEnvFile_WritesFalseSkipTLS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	cfg := ServerConfig{
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "test-token-abc123",
		SkipTLSVerify: false,
	}

	if _, err := writeEnvFileToPath(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "GITLAB_SKIP_TLS_VERIFY=false") {
		t.Error("should contain GITLAB_SKIP_TLS_VERIFY=false when false")
	}
}

// TestWriteEnvFile_WritesAdvancedOptions verifies every advanced wizard
// setting selected by a user is persisted to the generated env file.
func TestWriteEnvFile_WritesAdvancedOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	cfg := ServerConfig{
		GitLabURL:         "https://gitlab.example.com",
		GitLabToken:       "test-token-abc123",
		SkipTLSVerify:     true,
		MetaTools:         true,
		ToolSurface:       "dynamic",
		CapabilitySurface: "minimal",
		MetaParamSchema:   "compact",
		Enterprise:        true,
		ReadOnly:          true,
		SafeMode:          true,
		EmbeddedResources: false,
		ExcludeTools:      "gitlab_admin,gitlab_runner",
		IgnoreScopes:      true,
		UploadMaxFileSize: "500MB",
		AutoUpdate:        true,
		AutoUpdateMode:    "check",
		AutoUpdateRepo:    "example/gitlab-mcp-server",
		AutoUpdateTimeout: "90s",
		RateLimitRPS:      "3.5",
		RateLimitBurst:    "12",
		LogLevel:          "debug",
		YoloMode:          true,
	}

	if _, err := writeEnvFileToPath(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	expected := []string{
		"GITLAB_URL=https://gitlab.example.com",
		"GITLAB_TOKEN=test-token-abc123",
		"GITLAB_SKIP_TLS_VERIFY=true",
		"TOOL_SURFACE=dynamic",
		"CAPABILITY_SURFACE=minimal",
		"META_PARAM_SCHEMA=compact",
		"GITLAB_ENTERPRISE=true",
		"GITLAB_READ_ONLY=true",
		"GITLAB_SAFE_MODE=true",
		"EMBEDDED_RESOURCES=false",
		"EXCLUDE_TOOLS=gitlab_admin,gitlab_runner",
		"GITLAB_IGNORE_SCOPES=true",
		"UPLOAD_MAX_FILE_SIZE=500MB",
		"AUTO_UPDATE=check",
		"AUTO_UPDATE_REPO=example/gitlab-mcp-server",
		"AUTO_UPDATE_TIMEOUT=90s",
		"RATE_LIMIT_RPS=3.5",
		"RATE_LIMIT_BURST=12",
		"LOG_LEVEL=debug",
		"YOLO_MODE=true",
	}
	for _, line := range expected {
		if !strings.Contains(content, line+"\n") {
			t.Errorf("env file missing line %q\ncontent:\n%s", line, content)
		}
	}
	if strings.Contains(content, "META_TOOLS=") {
		t.Fatalf("env file should not write deprecated META_TOOLS\ncontent:\n%s", content)
	}
}

// TestEnvFilePath_InHome verifies EnvFilePath returns a path located inside
// the user's home directory and ending with the EnvFileName suffix.
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

// TestLoadExistingConfigFromPath_ValidFile verifies loadExistingConfigFromPath
// parses GITLAB_URL, GITLAB_TOKEN, and GITLAB_SKIP_TLS_VERIFY from a valid
// env file and returns found=true.
func TestLoadExistingConfigFromPath_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "# gitlab-mcp-server config\n" +
		"GITLAB_URL=https://gitlab.example.com\n" +
		"GITLAB_TOKEN=test-token-abc123def456\n" +
		"GITLAB_SKIP_TLS_VERIFY=true\n" +
		"TOOL_SURFACE=dynamic\n" +
		"CAPABILITY_SURFACE=minimal\n" +
		"META_PARAM_SCHEMA=full\n" +
		"GITLAB_ENTERPRISE=true\n" +
		"GITLAB_READ_ONLY=true\n" +
		"GITLAB_SAFE_MODE=true\n" +
		"EMBEDDED_RESOURCES=false\n" +
		"EXCLUDE_TOOLS=gitlab_admin\n" +
		"GITLAB_IGNORE_SCOPES=true\n" +
		"UPLOAD_MAX_FILE_SIZE=500MB\n" +
		"AUTO_UPDATE=check\n" +
		"AUTO_UPDATE_REPO=example/repo\n" +
		"AUTO_UPDATE_TIMEOUT=90s\n" +
		"RATE_LIMIT_RPS=3.5\n" +
		"RATE_LIMIT_BURST=12\n" +
		"LOG_LEVEL=debug\n" +
		"YOLO_MODE=true\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if !found {
		t.Fatal("expected found=true for valid env file")
	}
	if cfg.GitLabURL != "https://gitlab.example.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://gitlab.example.com")
	}
	if cfg.GitLabToken != "test-token-abc123def456" {
		t.Errorf("GitLabToken = %q, want %q", cfg.GitLabToken, "test-token-abc123def456")
	}
	if !cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify should be true")
	}
	if cfg.ToolSurface != "dynamic" || cfg.CapabilitySurface != "minimal" || cfg.MetaParamSchema != "full" {
		t.Errorf("catalog options not parsed: %#v", cfg)
	}
	if !cfg.Enterprise || !cfg.ReadOnly || !cfg.SafeMode || cfg.EmbeddedResources || !cfg.IgnoreScopes || !cfg.YoloMode {
		t.Errorf("boolean advanced options not parsed: %#v", cfg)
	}
	if cfg.ExcludeTools != "gitlab_admin" || cfg.UploadMaxFileSize != "500MB" {
		t.Errorf("text advanced options not parsed: %#v", cfg)
	}
	if cfg.AutoUpdateMode != "check" || cfg.AutoUpdateRepo != "example/repo" || cfg.RateLimitRPS != "3.5" {
		t.Errorf("mode advanced options not parsed: %#v", cfg)
	}
}

// TestLoadExistingConfigFromPath_LegacyMetaToolsDynamic verifies legacy
// META_TOOLS dynamic values are read for compatibility when TOOL_SURFACE is absent.
func TestLoadExistingConfigFromPath_LegacyMetaToolsDynamic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\n" +
		"GITLAB_TOKEN=test-token-abc123def456\n" +
		"META_TOOLS=dynamic\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if !found {
		t.Fatal("expected found=true")
	}
	if cfg.ToolSurface != "dynamic" || !cfg.MetaTools {
		t.Fatalf("legacy META_TOOLS dynamic parsed as ToolSurface=%q MetaTools=%v", cfg.ToolSurface, cfg.MetaTools)
	}
}

// TestLoadExistingConfigFromPath_FileNotExists verifies loadExistingConfigFromPath
// returns found=false when the target path does not exist.
func TestLoadExistingConfigFromPath_FileNotExists(t *testing.T) {
	_, found := loadExistingConfigFromPath(filepath.Join(t.TempDir(), "nonexistent.env"))
	if found {
		t.Error("expected found=false for nonexistent file")
	}
}

// TestLoadExistingConfigFromPath_EmptyFile verifies loadExistingConfigFromPath
// returns found=false when the file exists but is empty.
func TestLoadExistingConfigFromPath_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, found := loadExistingConfigFromPath(path)
	if found {
		t.Error("expected found=false for empty file")
	}
}

// TestLoadExistingConfigFromPath_OnlyComments verifies loadExistingConfigFromPath
// returns found=false when the file contains only comments and blank lines.
func TestLoadExistingConfigFromPath_OnlyComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "# Just comments\n# Another comment\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, found := loadExistingConfigFromPath(path)
	if found {
		t.Error("expected found=false for file with only comments")
	}
}

// TestLoadExistingConfigFromPath_LinesWithoutEquals verifies that malformed
// lines lacking the '=' separator are silently skipped, while well-formed
// entries on the same file are still parsed correctly.
func TestLoadExistingConfigFromPath_LinesWithoutEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\n" +
		"GITLAB_TOKEN=test-token-abcdef1234567890\n" +
		"this-line-has-no-equals-sign\n" +
		"ANOTHER_BAD_LINE\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if !found {
		t.Fatal("expected found=true for file with valid entries alongside malformed lines")
	}
	if cfg.GitLabURL != "https://gitlab.example.com" {
		t.Errorf("GitLabURL = %q, want %q", cfg.GitLabURL, "https://gitlab.example.com")
	}
	if cfg.GitLabToken != "test-token-abcdef1234567890" {
		t.Errorf("GitLabToken = %q, want %q", cfg.GitLabToken, "test-token-abcdef1234567890")
	}
}

// TestLoadExistingConfigFromPath_OnlyLinesWithoutEquals verifies that an env
// file containing only malformed lines (no '=' separator) returns found=false
// since no parseable vars are extracted.
func TestLoadExistingConfigFromPath_OnlyLinesWithoutEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "no-equals-here\nanother-bad-line\nyet-another\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if found {
		t.Error("expected found=false when all lines lack '=' separator")
	}
	_ = cfg
}

// TestLoadExistingConfigFromPath_InvalidToolSurfaceFallback verifies that an
// invalid TOOL_SURFACE value triggers the toolSurfaceFromEnv fallback path
// that derives the tool surface from META_TOOLS (or its default).
func TestLoadExistingConfigFromPath_InvalidToolSurfaceFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\n" +
		"GITLAB_TOKEN=test-token-abcdef1234567890\n" +
		"TOOL_SURFACE=not-a-valid-value\n" +
		"META_TOOLS=false\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if !found {
		t.Fatal("expected found=true for valid env file with invalid TOOL_SURFACE")
	}
	if cfg.MetaTools {
		t.Error("MetaTools should be false (META_TOOLS=false), exercising the fallback branch")
	}
}

// TestToolSurfaceFromEnv_InvalidValueFallsBackToMetaTools verifies the
// toolSurfaceFromEnv function returns a non-empty surface and a sensible
// metaTools flag when ParseToolSurface rejects the TOOL_SURFACE value.
func TestToolSurfaceFromEnv_InvalidValueFallsBackToMetaTools(t *testing.T) {
	surface, metaTools := toolSurfaceFromEnv(map[string]string{
		"TOOL_SURFACE": "garbage-value",
		"META_TOOLS":   "false",
	})
	if surface == "" {
		t.Error("expected non-empty surface from fallback")
	}
	if metaTools {
		t.Error("metaTools = true, want false from META_TOOLS=false fallback")
	}
}

// TestLoadExistingConfigFromPath_OnlyPreferences verifies preference-only env
// files are parsed but do not count as an existing GitLab connection.
func TestLoadExistingConfigFromPath_OnlyPreferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "LOG_LEVEL=debug\nTOOL_SURFACE=dynamic\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	cfg, found := loadExistingConfigFromPath(path)
	if found {
		t.Error("expected found=false when URL and token are absent")
	}
	if cfg.LogLevel != "debug" || cfg.ToolSurface != "dynamic" {
		t.Errorf("preferences not parsed: %#v", cfg)
	}
}

// TestLoadExistingConfigFromPath_SkipTLSFalse verifies loadExistingConfigFromPath
// parses GITLAB_SKIP_TLS_VERIFY=false into SkipTLSVerify=false.
func TestLoadExistingConfigFromPath_SkipTLSFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\nGITLAB_SKIP_TLS_VERIFY=false\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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

// TestLoadExistingConfigFromPath_SensibleDefaults verifies loadExistingConfigFromPath
// applies default values (MetaTools=true, AutoUpdate=true, LogLevel="info") when
// those fields are absent from the env file.
func TestLoadExistingConfigFromPath_SensibleDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)

	content := "GITLAB_URL=https://gitlab.example.com\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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
		GitLabToken: "test-token-wrapper-test",
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
	if !strings.Contains(string(data), "test-token-wrapper-test") {
		t.Error("written file does not contain expected token")
	}
}
