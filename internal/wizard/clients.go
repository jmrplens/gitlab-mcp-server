package wizard

import (
	"fmt"
	"strings"
)

// ClientID identifies a supported MCP client.
type ClientID string

const (
	ClientVSCode        ClientID = "vscode"
	ClientClaudeDesktop ClientID = "claude-desktop"
	ClientClaudeCode    ClientID = "claude-code"
	ClientCursor        ClientID = "cursor"
	ClientWindsurf      ClientID = "windsurf"
	ClientJetBrains     ClientID = "jetbrains"
	ClientCopilotCLI    ClientID = "copilot-cli"
	ClientOpenCode      ClientID = "opencode"
	ClientCrush         ClientID = "crush"
	ClientZed           ClientID = "zed"
)

// DefaultGitLabURL is pre-filled in UI modes as a convenience default.
const DefaultGitLabURL = ""

// TokenCreationURL returns the GitLab URL for creating a personal access token
// with the "api" scope pre-selected, which is required for full MCP functionality.
func TokenCreationURL(gitlabURL string) string {
	return strings.TrimRight(gitlabURL, "/") + "/-/user_settings/personal_access_tokens?name=gitlab-mcp-server&scopes=api"
}

// ServerConfig holds the user's configuration values for the MCP server.
type ServerConfig struct {
	BinaryPath    string
	GitLabURL     string
	GitLabToken   string
	SkipTLSVerify bool
	MetaTools     bool
	AutoUpdate    bool
	LogLevel      string
	YoloMode      bool
}

// ClientInfo describes an MCP client and how to configure it.
type ClientInfo struct {
	ID              ClientID
	Name            string
	ConfigPath      string // resolved config file path ("" for display-only clients)
	DisplayOnly     bool   // true for clients where we print JSON instead of writing a file
	DefaultSelected bool   // true if this client should be pre-selected in the wizard
}

// allClientsFn is the function used internally to get the client list.
// Tests can swap this to return clients with temp config paths.
var allClientsFn = AllClients

// AllClients returns the list of supported MCP clients with resolved config paths.
func AllClients() []ClientInfo {
	return []ClientInfo{
		{ID: ClientVSCode, Name: "VS Code (GitHub Copilot)", ConfigPath: vsCodeConfigPath(), DefaultSelected: true},
		{ID: ClientClaudeDesktop, Name: "Claude Desktop", ConfigPath: claudeDesktopConfigPath()},
		{ID: ClientClaudeCode, Name: "Claude Code (CLI)", ConfigPath: claudeCodeConfigPath()},
		{ID: ClientCursor, Name: "Cursor", ConfigPath: cursorConfigPath()},
		{ID: ClientWindsurf, Name: "Windsurf (Codeium)", ConfigPath: windsurfConfigPath()},
		{ID: ClientJetBrains, Name: "JetBrains IDEs", DisplayOnly: true},
		{ID: ClientCopilotCLI, Name: "Copilot CLI", ConfigPath: copilotCLIConfigPath(), DefaultSelected: true},
		{ID: ClientOpenCode, Name: "OpenCode", ConfigPath: openCodeConfigPath()},
		{ID: ClientCrush, Name: "Crush (Charm)", ConfigPath: crushConfigPath()},
		{ID: ClientZed, Name: "Zed", ConfigPath: zedConfigPath()},
	}
}

// envMapPreferences builds the non-secret environment variables (feature toggles and preferences).
func envMapPreferences(cfg ServerConfig) map[string]string {
	env := make(map[string]string)
	if cfg.MetaTools {
		env["META_TOOLS"] = "true"
	}
	if cfg.AutoUpdate {
		env["AUTO_UPDATE"] = "true"
	}
	if cfg.YoloMode {
		env["YOLO_MODE"] = "true"
	}
	if cfg.LogLevel != "" && cfg.LogLevel != "info" {
		env["LOG_LEVEL"] = cfg.LogLevel
	}
	return env
}

// envMap builds the full environment variable map for a server configuration.
// Used for display-only clients (JetBrains) that cannot reference an env file.
func envMap(cfg ServerConfig) map[string]string {
	env := envMapPreferences(cfg)
	env["GITLAB_URL"] = cfg.GitLabURL
	env["GITLAB_TOKEN"] = cfg.GitLabToken
	if cfg.SkipTLSVerify {
		env["GITLAB_SKIP_TLS_VERIFY"] = "true"
	}
	return env
}

// envFileRef returns the envFile path using VS Code's ${userHome} variable
// for portability, so the JSON config works regardless of the actual home dir.
func envFileRef() string {
	return "${userHome}/" + EnvFileName
}

// GenerateEntry returns the JSON-compatible map structure for the "gitlab"
// server entry, specific to the given client.
// Secrets (GITLAB_URL, GITLAB_TOKEN, GITLAB_SKIP_TLS_VERIFY) are NOT included
// in client configs — they live in the env file. VS Code uses native envFile
// support; other clients rely on the server loading the env file at startup.
// JetBrains (display-only) still uses the full env map since it cannot load files.
func GenerateEntry(clientID ClientID, cfg ServerConfig) map[string]any {
	env := envMapPreferences(cfg)

	switch clientID {
	case ClientVSCode:
		return map[string]any{
			"type":    "stdio",
			"command": cfg.BinaryPath,
			"env":     env,
			"envFile": envFileRef(),
		}
	case ClientCopilotCLI:
		return map[string]any{
			"type":    "stdio",
			"command": cfg.BinaryPath,
			"args":    []string{},
			"env":     env,
			"tools":   []string{"*"},
		}
	case ClientOpenCode:
		return map[string]any{
			"type":        "local",
			"command":     []string{cfg.BinaryPath},
			"environment": env,
			"enabled":     true,
		}
	case ClientCrush:
		return map[string]any{
			"type":    "stdio",
			"command": cfg.BinaryPath,
			"env":     env,
		}
	case ClientJetBrains:
		// JetBrains is display-only and cannot reference env files,
		// so we include the full environment map with secrets.
		return map[string]any{
			"command": cfg.BinaryPath,
			"env":     envMap(cfg),
		}
	default:
		// Claude Desktop, Claude Code, Cursor, Windsurf, Zed
		return map[string]any{
			"command": cfg.BinaryPath,
			"env":     env,
		}
	}
}

// RootKey returns the JSON root key under which the server entry is placed.
func RootKey(clientID ClientID) string {
	switch clientID {
	case ClientVSCode:
		return "servers"
	case ClientOpenCode, ClientCrush:
		return "mcp"
	case ClientZed:
		return "context_servers"
	default:
		return "mcpServers"
	}
}

// ServerEntryName is the name used for the server entry in all clients.
const ServerEntryName = "gitlab"

// restartHints maps client IDs to their restart instructions.
var restartHints = map[ClientID]string{
	ClientVSCode:        "restart VS Code or reload window",
	ClientClaudeDesktop: "restart Claude Desktop",
	ClientClaudeCode:    "run 'claude' in any terminal",
	ClientCursor:        "restart Cursor",
	ClientWindsurf:      "restart Windsurf",
	ClientCopilotCLI:    "run 'copilot' in any terminal",
	ClientOpenCode:      "run 'opencode' in any terminal",
	ClientCrush:         "run 'crush' in any terminal",
	ClientZed:           "restart Zed",
	ClientJetBrains:     "paste the JSON in Settings > Tools > AI Assistant > MCP Servers",
}

// RestartHint returns a user-friendly hint for how to activate the new config.
func RestartHint(clientID ClientID) string {
	if hint, ok := restartHints[clientID]; ok {
		return hint
	}
	return fmt.Sprintf("restart %s", clientID)
}
