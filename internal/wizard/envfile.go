// envfile.go reads and writes .env files for storing GitLab credentials
// and server configuration used by the MCP server.

package wizard

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
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

	cfg := ServerConfig{
		GitLabURL:     vars["GITLAB_URL"],
		GitLabToken:   vars["GITLAB_TOKEN"],
		SkipTLSVerify: strings.EqualFold(vars["GITLAB_SKIP_TLS_VERIFY"], "true"),
		MetaTools:     true,
		AutoUpdate:    true,
		LogLevel:      "info",
	}

	return cfg, cfg.GitLabURL != "" || cfg.GitLabToken != ""
}

// WriteEnvFile writes the GitLab secrets to the env file at EnvFilePath().
// The file is created with restricted permissions (0600 on Unix, 0644 on Windows).
func WriteEnvFile(cfg ServerConfig) (string, error) {
	path := EnvFilePath()
	return writeEnvFileToPath(path, cfg)
}

// writeEnvFileToPath writes the env file to a specific path.
func writeEnvFileToPath(path string, cfg ServerConfig) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# gitlab-mcp-server environment — managed by setup wizard\n")
	fmt.Fprintf(&b, "GITLAB_URL=%s\n", cfg.GitLabURL)
	fmt.Fprintf(&b, "GITLAB_TOKEN=%s\n", cfg.GitLabToken)
	if cfg.SkipTLSVerify {
		fmt.Fprintf(&b, "GITLAB_SKIP_TLS_VERIFY=true\n")
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
