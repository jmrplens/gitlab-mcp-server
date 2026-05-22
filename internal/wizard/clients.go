package wizard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// ClientID identifies a supported MCP client.
type ClientID string

const (
	// ClientVSCode identifies Visual Studio Code with GitHub Copilot MCP configuration.
	ClientVSCode ClientID = "vscode"
	// ClientClaudeDesktop identifies Claude Desktop MCP configuration.
	ClientClaudeDesktop ClientID = "claude-desktop"
	// ClientClaudeCode identifies Claude Code CLI MCP configuration.
	ClientClaudeCode ClientID = "claude-code"
	// ClientCursor identifies Cursor MCP configuration.
	ClientCursor ClientID = "cursor"
	// ClientWindsurf identifies Windsurf MCP configuration.
	ClientWindsurf ClientID = "windsurf"
	// ClientJetBrains identifies JetBrains IDE MCP configuration guidance.
	ClientJetBrains ClientID = "jetbrains"
	// ClientCopilotCLI identifies GitHub Copilot CLI MCP configuration.
	ClientCopilotCLI ClientID = "copilot-cli"
	// ClientOpenCode identifies OpenCode MCP configuration.
	ClientOpenCode ClientID = "opencode"
	// ClientCrush identifies Crush MCP configuration.
	ClientCrush ClientID = "crush"
	// ClientZed identifies Zed editor MCP configuration.
	ClientZed ClientID = "zed"
)

// DefaultGitLabURL is pre-filled in UI modes as a convenience default.
const DefaultGitLabURL = config.DefaultGitLabURL

func effectiveGitLabURL(gitLabURL string) string {
	gitLabURL = strings.TrimSpace(gitLabURL)
	if gitLabURL == "" {
		return DefaultGitLabURL
	}
	return gitLabURL
}

// TokenCreationURL returns the GitLab URL for creating a personal access token
// with the "api" scope pre-selected, which is required for full MCP functionality.
func TokenCreationURL(gitlabURL string) string {
	return strings.TrimRight(effectiveGitLabURL(gitlabURL), "/") + "/-/user_settings/personal_access_tokens?name=gitlab-mcp-server&scopes=api"
}

// ServerConfig holds the user's configuration values for the MCP server.
type ServerConfig struct {
	BinaryPath        string
	GitLabURL         string
	GitLabToken       string
	SkipTLSVerify     bool
	MetaTools         bool
	ToolSurface       string
	CapabilitySurface string
	MetaParamSchema   string
	Enterprise        bool
	ReadOnly          bool
	SafeMode          bool
	EmbeddedResources bool
	ExcludeTools      string
	IgnoreScopes      bool
	UploadMaxFileSize string
	AutoUpdate        bool
	AutoUpdateMode    string
	AutoUpdateRepo    string
	AutoUpdateTimeout string
	RateLimitRPS      string
	RateLimitBurst    string
	LogLevel          string
	YoloMode          bool
}

// ToolSurfaceOptions lists the supported MCP tool catalog selectors.
var ToolSurfaceOptions = []string{
	config.ToolSurfaceDynamic,
	config.ToolSurfaceMeta,
	config.ToolSurfaceIndividual,
}

// CapabilitySurfaceOptions lists the supported resource and prompt catalog selectors.
var CapabilitySurfaceOptions = []string{
	config.CapabilitySurfaceFull,
	config.CapabilitySurfaceMinimal,
}

// MetaParamSchemaOptions lists the supported meta-tool input-schema modes.
var MetaParamSchemaOptions = []string{
	config.MetaParamSchemaOpaque,
	config.MetaParamSchemaCompact,
	config.MetaParamSchemaFull,
}

// AutoUpdateModeOptions lists the supported auto-update modes.
var AutoUpdateModeOptions = []string{"true", "check", "false"}

const (
	defaultUploadMaxFileSize = "2GB"
	defaultAutoUpdateTimeout = "60s"
	defaultRateLimitRPS      = "0"
	defaultRateLimitBurst    = "40"
)

// DefaultServerConfig returns the wizard defaults for a new setup.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		GitLabURL:         DefaultGitLabURL,
		MetaTools:         true,
		ToolSurface:       config.DefaultToolSurface,
		CapabilitySurface: config.DefaultCapabilitySurface,
		MetaParamSchema:   config.DefaultMetaParamSchema,
		EmbeddedResources: true,
		UploadMaxFileSize: defaultUploadMaxFileSize,
		AutoUpdate:        true,
		AutoUpdateMode:    "true",
		AutoUpdateRepo:    config.DefaultAutoUpdateRepo,
		AutoUpdateTimeout: defaultAutoUpdateTimeout,
		RateLimitRPS:      defaultRateLimitRPS,
		RateLimitBurst:    defaultRateLimitBurst,
		LogLevel:          "info",
	}
}

func (cfg ServerConfig) withDefaults() ServerConfig {
	defaults := DefaultServerConfig()
	defaults.BinaryPath = cfg.BinaryPath
	defaults.GitLabURL = firstNonEmpty(cfg.GitLabURL, defaults.GitLabURL)
	defaults.GitLabToken = cfg.GitLabToken
	defaults.SkipTLSVerify = cfg.SkipTLSVerify
	if strings.TrimSpace(cfg.ToolSurface) != "" {
		defaults.ToolSurface = strings.TrimSpace(cfg.ToolSurface)
	} else if cfg.MetaTools {
		defaults.ToolSurface = config.ToolSurfaceMeta
	}
	defaults.MetaTools = defaults.ToolSurface != config.ToolSurfaceIndividual
	defaults.CapabilitySurface = firstNonEmpty(cfg.CapabilitySurface, defaults.CapabilitySurface)
	defaults.MetaParamSchema = firstNonEmpty(cfg.MetaParamSchema, defaults.MetaParamSchema)
	defaults.Enterprise = cfg.Enterprise
	defaults.ReadOnly = cfg.ReadOnly
	defaults.SafeMode = cfg.SafeMode
	defaults.EmbeddedResources = cfg.EmbeddedResources
	defaults.ExcludeTools = cfg.ExcludeTools
	defaults.IgnoreScopes = cfg.IgnoreScopes
	defaults.UploadMaxFileSize = firstNonEmpty(cfg.UploadMaxFileSize, defaults.UploadMaxFileSize)
	defaults.AutoUpdateMode = autoUpdateMode(cfg)
	defaults.AutoUpdate = defaults.AutoUpdateMode != "false"
	defaults.AutoUpdateRepo = firstNonEmpty(cfg.AutoUpdateRepo, defaults.AutoUpdateRepo)
	defaults.AutoUpdateTimeout = firstNonEmpty(cfg.AutoUpdateTimeout, defaults.AutoUpdateTimeout)
	defaults.RateLimitRPS = firstNonEmpty(cfg.RateLimitRPS, defaults.RateLimitRPS)
	defaults.RateLimitBurst = firstNonEmpty(cfg.RateLimitBurst, defaults.RateLimitBurst)
	defaults.LogLevel = firstNonEmpty(cfg.LogLevel, defaults.LogLevel)
	defaults.YoloMode = cfg.YoloMode
	return defaults
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func toolSurfaceFromMetaTools(metaTools bool) string {
	if metaTools {
		return config.ToolSurfaceMeta
	}
	return config.ToolSurfaceIndividual
}

func autoUpdateMode(cfg ServerConfig) string {
	if strings.TrimSpace(cfg.AutoUpdateMode) != "" {
		return strings.TrimSpace(cfg.AutoUpdateMode)
	}
	if cfg.AutoUpdate {
		return "true"
	}
	return "false"
}

func boolString(value bool) string {
	return strconv.FormatBool(value)
}

func choiceIndex(options []string, value string, defaultIndex int) int {
	for i, option := range options {
		if strings.EqualFold(option, strings.TrimSpace(value)) {
			return i
		}
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		return 0
	}
	return defaultIndex
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
	if cfg.ToolSurface != "" {
		env["TOOL_SURFACE"] = cfg.ToolSurface
	} else if cfg.MetaTools {
		env["TOOL_SURFACE"] = config.ToolSurfaceMeta
	}
	if cfg.CapabilitySurface != "" && cfg.CapabilitySurface != config.DefaultCapabilitySurface {
		env["CAPABILITY_SURFACE"] = cfg.CapabilitySurface
	}
	if cfg.MetaParamSchema != "" && cfg.MetaParamSchema != config.DefaultMetaParamSchema {
		env["META_PARAM_SCHEMA"] = cfg.MetaParamSchema
	}
	if cfg.Enterprise {
		env["GITLAB_ENTERPRISE"] = "true"
	}
	if cfg.ReadOnly {
		env["GITLAB_READ_ONLY"] = "true"
	}
	if cfg.SafeMode {
		env["GITLAB_SAFE_MODE"] = "true"
	}
	if !cfg.EmbeddedResources && cfg.EmbeddedResources != DefaultServerConfig().EmbeddedResources {
		env["EMBEDDED_RESOURCES"] = "false"
	}
	if cfg.ExcludeTools != "" {
		env["EXCLUDE_TOOLS"] = cfg.ExcludeTools
	}
	if cfg.IgnoreScopes {
		env["GITLAB_IGNORE_SCOPES"] = "true"
	}
	if cfg.UploadMaxFileSize != "" && cfg.UploadMaxFileSize != defaultUploadMaxFileSize {
		env["UPLOAD_MAX_FILE_SIZE"] = cfg.UploadMaxFileSize
	}
	addAutoUpdateEnv(env, cfg)
	addRuntimeLimitEnv(env, cfg)
	return env
}

func addAutoUpdateEnv(env map[string]string, cfg ServerConfig) {
	if mode := strings.TrimSpace(cfg.AutoUpdateMode); mode != "" && mode != "true" {
		env["AUTO_UPDATE"] = mode
	} else if cfg.AutoUpdate && cfg.AutoUpdateMode == "" {
		env["AUTO_UPDATE"] = "true"
	}
	if cfg.AutoUpdateRepo != "" && cfg.AutoUpdateRepo != config.DefaultAutoUpdateRepo {
		env["AUTO_UPDATE_REPO"] = cfg.AutoUpdateRepo
	}
	if cfg.AutoUpdateTimeout != "" && cfg.AutoUpdateTimeout != defaultAutoUpdateTimeout {
		env["AUTO_UPDATE_TIMEOUT"] = cfg.AutoUpdateTimeout
	}
}

func addRuntimeLimitEnv(env map[string]string, cfg ServerConfig) {
	if cfg.RateLimitRPS != "" && cfg.RateLimitRPS != defaultRateLimitRPS {
		env["RATE_LIMIT_RPS"] = cfg.RateLimitRPS
	}
	if cfg.RateLimitBurst != "" && cfg.RateLimitBurst != defaultRateLimitBurst {
		env["RATE_LIMIT_BURST"] = cfg.RateLimitBurst
	}
	if cfg.YoloMode {
		env["YOLO_MODE"] = "true"
	}
	if cfg.LogLevel != "" && cfg.LogLevel != "info" {
		env["LOG_LEVEL"] = cfg.LogLevel
	}
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
