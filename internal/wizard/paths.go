package wizard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	appName      = "gitlab-mcp-server"
	configDirXDG = ".config"
	crushFile    = "crush.json"
	settingsFile = "settings.json"
)

// DefaultInstallDir returns the platform-standard directory for installing binaries.
//   - Windows: %LOCALAPPDATA%\gitlab-mcp-server
//   - macOS/Linux: ~/.local/bin
func DefaultInstallDir() string {
	home, _ := os.UserHomeDir()
	return defaultInstallDirFor(runtime.GOOS, os.Getenv, home)
}

// defaultInstallDirFor is the platform-parameterized implementation of
// DefaultInstallDir, extracted so every GOOS branch is testable on any host.
func defaultInstallDirFor(goos string, getenv func(string) string, home string) string {
	switch goos {
	case "windows":
		if dir := getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appName)
		}
		return filepath.Join(home, "AppData", "Local", appName)
	default:
		return filepath.Join(home, ".local", "bin")
	}
}

// DefaultBinaryName returns the binary name for the current platform.
func DefaultBinaryName() string {
	return defaultBinaryNameFor(runtime.GOOS)
}

// defaultBinaryNameFor is the platform-parameterized implementation of
// DefaultBinaryName, extracted so both GOOS branches are testable on any host.
func defaultBinaryNameFor(goos string) string {
	if goos == "windows" {
		return appName + ".exe"
	}
	return appName
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	return expandPathWithHome(path, home, err)
}

// expandPathWithHome joins a ~-prefixed path with the resolved home directory,
// propagating any home-resolution error. Extracted so the error branch is
// testable without depending on the host environment.
func expandPathWithHome(path, home string, homeErr error) (string, error) {
	if homeErr != nil {
		return "", homeErr
	}
	return filepath.Join(home, path[1:]), nil
}

// configDir returns the platform-specific user config directory for a given app.
func configDir(app string) string {
	home, _ := os.UserHomeDir()
	return configDirFor(runtime.GOOS, os.Getenv, home, app)
}

// configDirFor is the platform-parameterized implementation of configDir,
// extracted so every GOOS branch is testable on any host.
func configDirFor(goos string, getenv func(string) string, home, app string) string {
	switch goos {
	case "windows":
		if dir := getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, app)
		}
		return filepath.Join(home, "AppData", "Roaming", app)
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", app)
	default:
		if dir := getenv("XDG_CONFIG_HOME"); dir != "" {
			return filepath.Join(dir, app)
		}
		return filepath.Join(home, configDirXDG, app)
	}
}

// vsCodeConfigPath returns the path to VS Code's user-level mcp.json.
func vsCodeConfigPath() string {
	return filepath.Join(configDir("Code"), "User", "mcp.json")
}

// claudeDesktopConfigPath returns the path to Claude Desktop's config.
func claudeDesktopConfigPath() string {
	return filepath.Join(configDir("Claude"), "claude_desktop_config.json")
}

// claudeCodeConfigPath returns the path to Claude Code's config.
func claudeCodeConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// cursorConfigPath returns the path to Cursor's MCP config.
func cursorConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cursor", "mcp.json")
}

// windsurfConfigPath returns the path to Windsurf (Codeium) MCP config.
func windsurfConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
}

// copilotCLIConfigPath returns the path to Copilot CLI MCP config.
func copilotCLIConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".copilot", "mcp-config.json")
}

// openCodeConfigPath returns the path to OpenCode's global config.
func openCodeConfigPath() string {
	return filepath.Join(configDir("opencode"), "opencode.json")
}

// crushConfigPath returns the path to Crush (Charm) global config.
func crushConfigPath() string {
	home, _ := os.UserHomeDir()
	return crushConfigPathFor(runtime.GOOS, os.Getenv, home)
}

// crushConfigPathFor is the platform-parameterized implementation of
// crushConfigPath, extracted so every GOOS branch is testable on any host.
func crushConfigPathFor(goos string, getenv func(string) string, home string) string {
	switch goos {
	case "windows":
		if dir := getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "crush", crushFile)
		}
		return filepath.Join(home, "AppData", "Local", "crush", crushFile)
	default:
		return filepath.Join(configDirFor(goos, getenv, home, "crush"), crushFile)
	}
}

// EnvFileName is the name of the env file where secrets are stored.
const EnvFileName = ".gitlab-mcp-server.env"

// EnvFilePath returns the path to the env file in the user's home directory.
func EnvFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, EnvFileName)
}

// zedConfigPath returns the path to Zed's settings file.
func zedConfigPath() string {
	home, _ := os.UserHomeDir()
	return zedConfigPathFor(runtime.GOOS, os.Getenv, home)
}

// zedConfigPathFor is the platform-parameterized implementation of
// zedConfigPath, extracted so every GOOS branch is testable on any host.
func zedConfigPathFor(goos string, getenv func(string) string, home string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, configDirXDG, "zed", settingsFile)
	case "windows":
		if dir := getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, "Zed", settingsFile)
		}
		return filepath.Join(home, "AppData", "Roaming", "Zed", settingsFile)
	default:
		if dir := getenv("XDG_CONFIG_HOME"); dir != "" {
			return filepath.Join(dir, "zed", settingsFile)
		}
		return filepath.Join(home, configDirXDG, "zed", settingsFile)
	}
}
