// testhelpers_test.go provides shared test helpers for the wizard package
// tests, including temporary directory setup and fixture creation.

package wizard

import (
	"path/filepath"
	"testing"
)

// stubLoadExistingConfig overrides loadExistingConfigFn to return an empty
// config with hasExisting=false. Call this to prevent tests from reading
// the real home directory env file.
func stubLoadExistingConfig(t *testing.T) {
	t.Helper()
	orig := loadExistingConfigFn
	loadExistingConfigFn = func() (ServerConfig, bool) {
		return ServerConfig{}, false
	}
	t.Cleanup(func() { loadExistingConfigFn = orig })
}

// stubLoadExistingConfigWith overrides loadExistingConfigFn to return the
// given config as if it had been loaded from an existing env file.
func stubLoadExistingConfigWith(t *testing.T, cfg ServerConfig) {
	t.Helper()
	orig := loadExistingConfigFn
	loadExistingConfigFn = func() (ServerConfig, bool) {
		return cfg, true
	}
	t.Cleanup(func() { loadExistingConfigFn = orig })
}

// useFakeClients overrides allClientsFn to return clients with config paths
// in a temp directory. Restores the original function when the test finishes.
// Returns the temp directory used for config files.
func useFakeClients(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	orig := allClientsFn
	allClientsFn = func() []ClientInfo {
		return []ClientInfo{
			{ID: ClientVSCode, Name: "VS Code (test)", ConfigPath: filepath.Join(tmpDir, "vscode-mcp.json"), DefaultSelected: true},
			{ID: ClientClaudeDesktop, Name: "Claude Desktop (test)", ConfigPath: filepath.Join(tmpDir, "claude-desktop.json")},
			{ID: ClientClaudeCode, Name: "Claude Code (test)", ConfigPath: filepath.Join(tmpDir, "claude-code.json")},
			{ID: ClientCursor, Name: "Cursor (test)", ConfigPath: filepath.Join(tmpDir, "cursor-mcp.json")},
			{ID: ClientWindsurf, Name: "Windsurf (test)", ConfigPath: filepath.Join(tmpDir, "windsurf-mcp.json")},
			{ID: ClientJetBrains, Name: "JetBrains IDEs (test)", DisplayOnly: true},
			{ID: ClientCopilotCLI, Name: "Copilot CLI (test)", ConfigPath: filepath.Join(tmpDir, "copilot-mcp.json"), DefaultSelected: true},
			{ID: ClientOpenCode, Name: "OpenCode (test)", ConfigPath: filepath.Join(tmpDir, "opencode-mcp.json")},
			{ID: ClientCrush, Name: "Crush (test)", ConfigPath: filepath.Join(tmpDir, "crush-mcp.json")},
			{ID: ClientZed, Name: "Zed (test)", ConfigPath: filepath.Join(tmpDir, "zed-settings.json")},
		}
	}
	t.Cleanup(func() { allClientsFn = orig })
	return tmpDir
}

// stubWriteEnvFile overrides writeEnvFileFn to write to a temp directory
// instead of the real home directory. Returns the path of the env file.
func stubWriteEnvFile(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, EnvFileName)
	orig := writeEnvFileFn
	writeEnvFileFn = func(cfg ServerConfig) (string, error) {
		return writeEnvFileToPath(envPath, cfg)
	}
	t.Cleanup(func() { writeEnvFileFn = orig })
	return envPath
}

// stubPickDirectory overrides pickDirectoryFn with a function that returns
// the given path without opening any OS dialog.
func stubPickDirectory(t *testing.T, path string, err error) {
	t.Helper()
	orig := pickDirectoryFn
	pickDirectoryFn = func(string) (string, error) { return path, err }
	t.Cleanup(func() { pickDirectoryFn = orig })
}

// stubInstallBinary overrides installBinaryFn to copy the binary into a temp
// directory instead of the real install location. This prevents tests from
// overwriting the production binary.
func stubInstallBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	orig := installBinaryFn
	installBinaryFn = func(destDir string) (string, error) {
		return installBinaryImpl(tmpDir)
	}
	t.Cleanup(func() { installBinaryFn = orig })
	return tmpDir
}

// stubGetInstalledVersion overrides getInstalledVersionFn to return the given
// version string without executing a real binary.
func stubGetInstalledVersion(t *testing.T, version string) {
	t.Helper()
	orig := getInstalledVersionFn
	getInstalledVersionFn = func() string { return version }
	t.Cleanup(func() { getInstalledVersionFn = orig })
}
