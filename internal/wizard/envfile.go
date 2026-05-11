package wizard

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
)

// writeEnvFileFn is the function used to write the env file.
// Tests can swap this to write to a temp directory instead.
var writeEnvFileFn = WriteEnvFile

// loadExistingConfigFn is the function used to load existing env file values.
// Tests can swap this to return a controlled config instead.
var loadExistingConfigFn = LoadExistingConfig

// LoadExistingConfig reads the existing .gitlab-mcp-server.env file and returns
// a ServerConfig populated with the stored values. If the file does not exist
// or cannot be parsed, it returns an empty config and false.
func LoadExistingConfig() (ServerConfig, bool) {
	return loadExistingConfigFromPath(EnvFilePath())
}

// loadExistingConfigFromPath reads an env file and parses KEY=VALUE pairs.
func loadExistingConfigFromPath(path string) (ServerConfig, bool) {
	f, err := os.Open(path) // #nosec G304 -- path is the well-known env file location
	if err != nil {
		return ServerConfig{}, false
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vars[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	if len(vars) == 0 {
		return ServerConfig{}, false
	}

	cfg := DefaultServerConfig()
	cfg.GitLabURL = firstNonEmpty(vars["GITLAB_URL"], cfg.GitLabURL)
	cfg.GitLabToken = vars["GITLAB_TOKEN"]
	cfg.SkipTLSVerify = envBool(vars, "GITLAB_SKIP_TLS_VERIFY", false)
	cfg.MetaTools = envBool(vars, "META_TOOLS", true)
	cfg.ToolSurface = firstNonEmpty(vars["TOOL_SURFACE"], toolSurfaceFromMetaTools(cfg.MetaTools))
	cfg.CapabilitySurface = firstNonEmpty(vars["CAPABILITY_SURFACE"], cfg.CapabilitySurface)
	cfg.MetaParamSchema = firstNonEmpty(vars["META_PARAM_SCHEMA"], cfg.MetaParamSchema)
	cfg.Enterprise = envBool(vars, "GITLAB_ENTERPRISE", false)
	cfg.ReadOnly = envBool(vars, "GITLAB_READ_ONLY", false)
	cfg.SafeMode = envBool(vars, "GITLAB_SAFE_MODE", false)
	cfg.EmbeddedResources = envBool(vars, "EMBEDDED_RESOURCES", true)
	cfg.ExcludeTools = vars["EXCLUDE_TOOLS"]
	cfg.IgnoreScopes = envBool(vars, "GITLAB_IGNORE_SCOPES", false)
	cfg.UploadMaxFileSize = firstNonEmpty(vars["UPLOAD_MAX_FILE_SIZE"], cfg.UploadMaxFileSize)
	cfg.AutoUpdateMode = firstNonEmpty(vars["AUTO_UPDATE"], cfg.AutoUpdateMode)
	cfg.AutoUpdate = cfg.AutoUpdateMode != "false"
	cfg.AutoUpdateRepo = firstNonEmpty(vars["AUTO_UPDATE_REPO"], cfg.AutoUpdateRepo)
	cfg.AutoUpdateTimeout = firstNonEmpty(vars["AUTO_UPDATE_TIMEOUT"], cfg.AutoUpdateTimeout)
	cfg.RateLimitRPS = firstNonEmpty(vars["RATE_LIMIT_RPS"], cfg.RateLimitRPS)
	cfg.RateLimitBurst = firstNonEmpty(vars["RATE_LIMIT_BURST"], cfg.RateLimitBurst)
	cfg.LogLevel = firstNonEmpty(vars["LOG_LEVEL"], cfg.LogLevel)
	cfg.YoloMode = envBool(vars, "YOLO_MODE", false)

	return cfg, cfg.GitLabURL != "" || cfg.GitLabToken != ""
}

func envBool(vars map[string]string, key string, defaultValue bool) bool {
	value, ok := vars[key]
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}

// WriteEnvFile writes the GitLab secrets to the env file at EnvFilePath().
// The file is created with restricted permissions (0600 on Unix, 0644 on Windows).
func WriteEnvFile(cfg ServerConfig) (string, error) {
	path := EnvFilePath()
	return writeEnvFileToPath(path, cfg)
}

// writeEnvFileToPath writes the env file to a specific path.
func writeEnvFileToPath(path string, cfg ServerConfig) (string, error) {
	cfg = cfg.withDefaults()
	var b strings.Builder
	fmt.Fprintf(&b, "# gitlab-mcp-server environment — managed by setup wizard\n")
	for _, entry := range envFileEntries(cfg) {
		fmt.Fprintf(&b, "%s=%s\n", entry.key, entry.value)
	}

	perm := os.FileMode(0o644)
	if runtime.GOOS != "windows" {
		perm = 0o600
	}

	if err := os.WriteFile(path, []byte(b.String()), perm); err != nil {
		return "", fmt.Errorf("writing env file %s: %w", path, err)
	}

	return path, nil
}

type envFileEntry struct {
	key   string
	value string
}

func envFileEntries(cfg ServerConfig) []envFileEntry {
	entries := []envFileEntry{
		{"GITLAB_URL", cfg.GitLabURL},
		{"GITLAB_TOKEN", cfg.GitLabToken},
		{"GITLAB_SKIP_TLS_VERIFY", boolString(cfg.SkipTLSVerify)},
		{"META_TOOLS", boolString(cfg.MetaTools)},
		{"TOOL_SURFACE", cfg.ToolSurface},
		{"CAPABILITY_SURFACE", cfg.CapabilitySurface},
		{"META_PARAM_SCHEMA", cfg.MetaParamSchema},
		{"GITLAB_ENTERPRISE", boolString(cfg.Enterprise)},
		{"GITLAB_READ_ONLY", boolString(cfg.ReadOnly)},
		{"GITLAB_SAFE_MODE", boolString(cfg.SafeMode)},
		{"EMBEDDED_RESOURCES", boolString(cfg.EmbeddedResources)},
		{"EXCLUDE_TOOLS", cfg.ExcludeTools},
		{"GITLAB_IGNORE_SCOPES", boolString(cfg.IgnoreScopes)},
		{"UPLOAD_MAX_FILE_SIZE", cfg.UploadMaxFileSize},
		{"AUTO_UPDATE", cfg.AutoUpdateMode},
		{"AUTO_UPDATE_REPO", cfg.AutoUpdateRepo},
		{"AUTO_UPDATE_TIMEOUT", cfg.AutoUpdateTimeout},
		{"RATE_LIMIT_RPS", cfg.RateLimitRPS},
		{"RATE_LIMIT_BURST", cfg.RateLimitBurst},
		{"LOG_LEVEL", cfg.LogLevel},
		{"YOLO_MODE", boolString(cfg.YoloMode)},
	}
	return slices.DeleteFunc(entries, func(entry envFileEntry) bool {
		return strings.TrimSpace(entry.value) == ""
	})
}
