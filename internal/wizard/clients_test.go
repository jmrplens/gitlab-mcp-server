// clients_test.go contains unit tests for MCP client detection and
// configuration path resolution.
package wizard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateEntry_VSCode(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:    "/usr/bin/gitlab-mcp-server",
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "glpat-abc123",
		SkipTLSVerify: true,
		MetaTools:     true,
	}

	entry := GenerateEntry(ClientVSCode, cfg)

	if entry["type"] != "stdio" {
		t.Errorf("VS Code type: got %q, want %q", entry["type"], "stdio")
	}
	if entry["command"] != cfg.BinaryPath {
		t.Errorf("command mismatch")
	}

	// VS Code should have envFile field
	envFile, ok := entry["envFile"].(string)
	if !ok || envFile == "" {
		t.Error("VS Code entry should have envFile field")
	}

	// Env should contain preferences but NOT secrets
	env := entry["env"].(map[string]string)
	if _, hasToken := env["GITLAB_TOKEN"]; hasToken {
		t.Error("GITLAB_TOKEN should not be in env (it's in envFile)")
	}
	if _, hasURL := env["GITLAB_URL"]; hasURL {
		t.Error("GITLAB_URL should not be in env (it's in envFile)")
	}
	if env["META_TOOLS"] != "true" {
		t.Error("META_TOOLS should be in env")
	}
}

func TestGenerateEntry_CopilotCLI(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-abc123",
	}

	entry := GenerateEntry(ClientCopilotCLI, cfg)

	if entry["type"] != "stdio" {
		t.Errorf("Copilot CLI type: got %q, want %q", entry["type"], "stdio")
	}
	args, ok := entry["args"].([]string)
	if !ok || len(args) != 0 {
		t.Errorf("args: got %v, want empty []string", entry["args"])
	}
	tools, ok := entry["tools"].([]string)
	if !ok || len(tools) != 1 || tools[0] != "*" {
		t.Errorf("tools: got %v, want [*]", entry["tools"])
	}
}

func TestGenerateEntry_OpenCode(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-abc123",
	}

	entry := GenerateEntry(ClientOpenCode, cfg)

	if entry["type"] != "local" {
		t.Errorf("OpenCode type: got %q, want %q", entry["type"], "local")
	}
	cmd, ok := entry["command"].([]string)
	if !ok || len(cmd) != 1 {
		t.Errorf("command should be array: got %v", entry["command"])
	}
	if entry["enabled"] != true {
		t.Error("enabled should be true")
	}
	if _, hasEnv := entry["env"]; hasEnv {
		t.Error("OpenCode should use 'environment', not 'env'")
	}
	if _, hasEnvironment := entry["environment"]; !hasEnvironment {
		t.Error("OpenCode should have 'environment' key")
	}
}

func TestGenerateEntry_ClaudeDesktop(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-abc123",
	}

	entry := GenerateEntry(ClientClaudeDesktop, cfg)

	if _, hasType := entry["type"]; hasType {
		t.Error("Claude Desktop should not have 'type' field")
	}
	if entry["command"] != cfg.BinaryPath {
		t.Errorf("command mismatch")
	}
}

func TestRootKey_Mapping(t *testing.T) {
	tests := []struct {
		client ClientID
		want   string
	}{
		{ClientVSCode, "servers"},
		{ClientOpenCode, "mcp"},
		{ClientCrush, "mcp"},
		{ClientZed, "context_servers"},
		{ClientClaudeDesktop, "mcpServers"},
		{ClientClaudeCode, "mcpServers"},
		{ClientCursor, "mcpServers"},
		{ClientWindsurf, "mcpServers"},
		{ClientCopilotCLI, "mcpServers"},
	}
	for _, tt := range tests {
		t.Run(string(tt.client), func(t *testing.T) {
			got := RootKey(tt.client)
			if got != tt.want {
				t.Errorf("RootKey(%s) = %q, want %q", tt.client, got, tt.want)
			}
		})
	}
}

func TestAllClients_Count(t *testing.T) {
	clients := AllClients()
	if len(clients) != 10 {
		t.Errorf("got %d clients, want 10", len(clients))
	}
}

func TestMergeServerEntry_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	entry := map[string]any{"command": "/bin/test", "env": map[string]string{"A": "B"}}
	if err := MergeServerEntry(path, "servers", "gitlab", entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}

	servers, ok := result["servers"].(map[string]any)
	if !ok {
		t.Fatal("missing 'servers' key")
	}
	if _, ok = servers["gitlab"]; !ok {
		t.Fatal("missing 'gitlab' entry")
	}
}

func TestMergeServerEntry_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	existing := `{"mcpServers": {"other-server": {"command": "other"}}, "authToken": "secret"}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"command": "/bin/test"}
	if err := MergeServerEntry(path, "mcpServers", "gitlab", entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	var result map[string]any
	json.Unmarshal(data, &result)

	// Should preserve authToken
	if result["authToken"] != "secret" {
		t.Error("authToken was not preserved")
	}

	// Should preserve other-server
	servers := result["mcpServers"].(map[string]any)
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server was not preserved")
	}
	if _, ok := servers["gitlab"]; !ok {
		t.Error("gitlab entry was not added")
	}
}

func TestMergeServerEntry_OverwritesExistingGitlab(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	existing := `{"mcpServers": {"gitlab": {"command": "old"}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"command": "/bin/new"}
	if err := MergeServerEntry(path, "mcpServers", "gitlab", entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}

	servers := result["mcpServers"].(map[string]any)
	gitlab := servers["gitlab"].(map[string]any)
	if gitlab["command"] != "/bin/new" {
		t.Errorf("got %v, want /bin/new", gitlab["command"])
	}
}

func TestMergeServerEntry_JSONC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	// VS Code-style JSONC with comments and trailing commas
	jsonc := `{
  // MCP server configuration
  "servers": {
    "other": {
      "type": "stdio",
      "command": "other-binary",
    },
  },
}`
	if err := os.WriteFile(path, []byte(jsonc), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := map[string]any{"type": "stdio", "command": "/bin/gitlab"}
	if err := MergeServerEntry(path, "servers", "gitlab", entry); err != nil {
		t.Fatalf("MergeServerEntry on JSONC file should not fail: %v", err)
	}

	data, _ := os.ReadFile(path)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}

	servers := result["servers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("existing 'other' entry was not preserved")
	}
	if _, ok := servers["gitlab"]; !ok {
		t.Error("'gitlab' entry was not added")
	}
}

func TestEnvMap_Defaults(t *testing.T) {
	cfg := ServerConfig{
		GitLabURL:     "https://example.com",
		GitLabToken:   "token",
		SkipTLSVerify: false,
		MetaTools:     false,
		AutoUpdate:    false,
		LogLevel:      "info",
	}

	env := envMap(cfg)
	if env["GITLAB_URL"] != "https://example.com" {
		t.Error("GITLAB_URL mismatch")
	}
	if _, ok := env["GITLAB_SKIP_TLS_VERIFY"]; ok {
		t.Error("GITLAB_SKIP_TLS_VERIFY should not be set when false")
	}
	if _, ok := env["META_TOOLS"]; ok {
		t.Error("META_TOOLS should not be set when false")
	}
	if _, ok := env["YOLO_MODE"]; ok {
		t.Error("YOLO_MODE should not be set when false")
	}
	if _, ok := env["LOG_LEVEL"]; ok {
		t.Error("LOG_LEVEL should not be set for default 'info'")
	}
}

func TestEnvMap_YoloMode(t *testing.T) {
	cfg := ServerConfig{
		GitLabURL:   "https://example.com",
		GitLabToken: "token",
		YoloMode:    true,
	}
	env := envMap(cfg)
	if env["YOLO_MODE"] != "true" {
		t.Error("YOLO_MODE should be set when enabled")
	}
}

func TestAllClients_DefaultSelected(t *testing.T) {
	clients := AllClients()
	var defaultNames []string
	for _, c := range clients {
		if c.DefaultSelected {
			defaultNames = append(defaultNames, string(c.ID))
		}
	}
	if len(defaultNames) != 2 {
		t.Errorf("expected 2 default-selected clients, got %d: %v", len(defaultNames), defaultNames)
	}
}

func TestGenerateEntry_Crush(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-abc123",
	}

	entry := GenerateEntry(ClientCrush, cfg)

	if entry["type"] != "stdio" {
		t.Errorf("Crush type: got %q, want %q", entry["type"], "stdio")
	}
	if _, ok := entry["command"].(string); !ok {
		t.Errorf("Crush command should be string, got %T", entry["command"])
	}
	if _, hasEnv := entry["env"]; !hasEnv {
		t.Error("Crush should have 'env' key")
	}
}

func TestGenerateEntry_Zed(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-abc123",
	}

	entry := GenerateEntry(ClientZed, cfg)

	if _, hasType := entry["type"]; hasType {
		t.Error("Zed should not have 'type' field")
	}
	if entry["command"] != cfg.BinaryPath {
		t.Errorf("command mismatch")
	}
}

// TestEnvMap_AllFlagsEnabled verifies envMap includes all optional env vars
// when every boolean flag is true and log level is non-default.
func TestEnvMap_AllFlagsEnabled(t *testing.T) {
	cfg := ServerConfig{
		GitLabURL:     "https://example.com",
		GitLabToken:   "tok",
		SkipTLSVerify: true,
		MetaTools:     true,
		AutoUpdate:    true,
		YoloMode:      true,
		LogLevel:      "debug",
	}
	env := envMap(cfg)

	expected := map[string]string{
		"GITLAB_URL":             "https://example.com",
		"GITLAB_TOKEN":           "tok",
		"GITLAB_SKIP_TLS_VERIFY": "true",
		"META_TOOLS":             "true",
		"AUTO_UPDATE":            "true",
		"YOLO_MODE":              "true",
		"LOG_LEVEL":              "debug",
	}
	for k, want := range expected {
		got, ok := env[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// TestTokenCreationURL verifies the token creation URL is constructed
// correctly, including trimming trailing slashes.
func TestTokenCreationURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSufx string
	}{
		{"no trailing slash", "https://gitlab.com", "/-/user_settings/personal_access_tokens?name=gitlab-mcp-server&scopes=api"},
		{"trailing slash", "https://gitlab.com/", "/-/user_settings/personal_access_tokens?name=gitlab-mcp-server&scopes=api"},
		{"custom url", "https://custom.dev", "/-/user_settings/personal_access_tokens?name=gitlab-mcp-server&scopes=api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenCreationURL(tt.input)
			if got == "" {
				t.Fatal("returned empty URL")
			}
			wantPrefix := "https://"
			if got[:8] != wantPrefix {
				t.Errorf("URL should start with https://, got %q", got)
			}
			if len(got) < len(tt.wantSufx) {
				t.Fatalf("URL too short: %q", got)
			}
			suffix := got[len(got)-len(tt.wantSufx):]
			if suffix != tt.wantSufx {
				t.Errorf("URL suffix = %q, want %q", suffix, tt.wantSufx)
			}
		})
	}
}

// TestRestartHint_UnknownClient verifies that an unrecognized client ID
// returns a fallback hint containing the client ID.
func TestRestartHint_UnknownClient(t *testing.T) {
	hint := RestartHint("unknown-client")
	if hint == "" {
		t.Fatal("expected non-empty hint for unknown client")
	}
	if hint != "restart unknown-client" {
		t.Errorf("hint = %q, want %q", hint, "restart unknown-client")
	}
}

// TestGenerateEntry_AllClients_HaveCommand verifies every client produces an
// entry with at least a "command" key of the expected type.
func TestGenerateEntry_AllClients_HaveCommand(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-xxx",
	}

	clients := AllClients()
	for _, c := range clients {
		t.Run(string(c.ID), func(t *testing.T) {
			entry := GenerateEntry(c.ID, cfg)
			cmd, ok := entry["command"]
			if !ok {
				t.Fatal("entry missing 'command' key")
			}

			switch c.ID {
			case ClientOpenCode:
				// OpenCode uses []string for command
				var arr []string
				arr, ok = cmd.([]string)
				if !ok || len(arr) == 0 {
					t.Errorf("OpenCode command should be []string, got %T", cmd)
				}
			default:
				// All others use string
				if _, ok = cmd.(string); !ok {
					t.Errorf("command should be string, got %T", cmd)
				}
			}
		})
	}
}

// TestAllClients_UniqueIDs verifies there are no duplicate client IDs.
func TestAllClients_UniqueIDs(t *testing.T) {
	clients := AllClients()
	seen := make(map[ClientID]bool)
	for _, c := range clients {
		if seen[c.ID] {
			t.Errorf("duplicate client ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

// TestAllClients_ConfigPathsOrDisplayOnly verifies each client has either a
// config path or is marked DisplayOnly.
func TestAllClients_ConfigPathsOrDisplayOnly(t *testing.T) {
	for _, c := range AllClients() {
		t.Run(string(c.ID), func(t *testing.T) {
			if !c.DisplayOnly && c.ConfigPath == "" {
				t.Errorf("client %s has no ConfigPath and is not DisplayOnly", c.ID)
			}
		})
	}
}

// TestGenerateEntry_NoSecretsInEnv verifies that writable (non-display-only)
// clients do NOT have GITLAB_TOKEN or GITLAB_URL in their env map.
func TestGenerateEntry_NoSecretsInEnv(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:    "/usr/bin/gitlab-mcp-server",
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "glpat-secret",
		SkipTLSVerify: true,
		MetaTools:     true,
	}

	writableClients := []ClientID{
		ClientVSCode, ClientClaudeDesktop, ClientClaudeCode,
		ClientCursor, ClientWindsurf, ClientCopilotCLI,
		ClientOpenCode, ClientCrush, ClientZed,
	}

	for _, id := range writableClients {
		t.Run(string(id), func(t *testing.T) {
			entry := GenerateEntry(id, cfg)

			// Find the env map (may be under "env" or "environment")
			var env map[string]string
			var e map[string]string
			var ok bool
			if e, ok = entry["env"].(map[string]string); ok {
				env = e
			} else if e, ok = entry["environment"].(map[string]string); ok {
				env = e
			}
			if env == nil {
				t.Fatal("entry has no env/environment map")
			}

			if _, has := env["GITLAB_TOKEN"]; has {
				t.Error("GITLAB_TOKEN must not be in client env (should be in env file)")
			}
			if _, has := env["GITLAB_URL"]; has {
				t.Error("GITLAB_URL must not be in client env (should be in env file)")
			}
			if _, has := env["GITLAB_SKIP_TLS_VERIFY"]; has {
				t.Error("GITLAB_SKIP_TLS_VERIFY must not be in client env (should be in env file)")
			}
		})
	}
}

// TestGenerateEntry_JetBrains_HasSecrets verifies JetBrains (display-only)
// includes secrets in env since it cannot reference env files.
func TestGenerateEntry_JetBrains_HasSecrets(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:    "/usr/bin/gitlab-mcp-server",
		GitLabURL:     "https://gitlab.example.com",
		GitLabToken:   "glpat-secret",
		SkipTLSVerify: true,
	}

	entry := GenerateEntry(ClientJetBrains, cfg)
	env := entry["env"].(map[string]string)

	if env["GITLAB_URL"] != cfg.GitLabURL {
		t.Error("JetBrains should have GITLAB_URL in env")
	}
	if env["GITLAB_TOKEN"] != cfg.GitLabToken {
		t.Error("JetBrains should have GITLAB_TOKEN in env")
	}
	if env["GITLAB_SKIP_TLS_VERIFY"] != "true" {
		t.Error("JetBrains should have GITLAB_SKIP_TLS_VERIFY in env")
	}
}

// TestGenerateEntry_VSCode_HasEnvFile verifies VS Code has envFile field.
func TestGenerateEntry_VSCode_HasEnvFile(t *testing.T) {
	cfg := ServerConfig{
		BinaryPath:  "/usr/bin/gitlab-mcp-server",
		GitLabURL:   "https://gitlab.example.com",
		GitLabToken: "glpat-abc123",
	}

	entry := GenerateEntry(ClientVSCode, cfg)

	envFile, ok := entry["envFile"].(string)
	if !ok {
		t.Fatal("VS Code entry should have envFile field")
	}
	if envFile != "${userHome}/"+EnvFileName {
		t.Errorf("envFile = %q, want %q", envFile, "${userHome}/"+EnvFileName)
	}
}

// TestEnvMapPreferences_NoSecrets verifies envMapPreferences never includes secrets.
func TestEnvMapPreferences_NoSecrets(t *testing.T) {
	cfg := ServerConfig{
		GitLabURL:     "https://example.com",
		GitLabToken:   "glpat-secret",
		SkipTLSVerify: true,
		MetaTools:     true,
		AutoUpdate:    true,
		YoloMode:      true,
		LogLevel:      "debug",
	}

	env := envMapPreferences(cfg)

	secrets := []string{"GITLAB_URL", "GITLAB_TOKEN", "GITLAB_SKIP_TLS_VERIFY"}
	for _, key := range secrets {
		if _, has := env[key]; has {
			t.Errorf("envMapPreferences should not contain %s", key)
		}
	}

	if env["META_TOOLS"] != "true" {
		t.Error("missing META_TOOLS")
	}
	if env["LOG_LEVEL"] != "debug" {
		t.Error("missing LOG_LEVEL")
	}
}
