// cli.go implements the non-interactive CLI wizard mode, collecting
// configuration from command-line flags and stdin without a TUI or web UI.

package wizard

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// RunCLI executes the interactive setup wizard using plain terminal I/O.
func RunCLI(version string, r io.Reader, w io.Writer) error {
	p := NewPrompter(r, w)

	printBanner(w, version)

	// Load existing configuration as defaults
	existing, hasExisting := loadExistingConfigFn()
	if hasExisting {
		fmt.Fprintln(w, "  Existing configuration detected — values will be used as defaults.")
		fmt.Fprintln(w)
	}

	// Step 1: Binary installation
	binaryPath, err := stepInstall(p, w)
	if err != nil {
		return err
	}

	// Step 2: GitLab configuration
	cfg, err := stepGitLabConfig(p, w, existing, hasExisting)
	if err != nil {
		return err
	}
	cfg.BinaryPath = binaryPath

	// Optional: Advanced options (defaults are pre-configured)
	advanced, err := p.AskYesNo("Configure advanced options?", false)
	if err != nil {
		return err
	}
	if advanced {
		if err = stepOptions(p, w, cfg); err != nil {
			return err
		}
	}

	// Step 3: Client selection & configuration
	return stepClients(p, w, *cfg)
}

func printBanner(w io.Writer, version string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "===================================================================")
	fmt.Fprintf(w, "  gitlab-mcp-server Setup Wizard  (v%s)\n", version)
	fmt.Fprintln(w, "  GitLab MCP Server for AI Assistants")
	fmt.Fprintln(w, "===================================================================")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "This wizard will help you:")
	fmt.Fprintln(w, "  1. Install the binary to a standard location")
	fmt.Fprintln(w, "  2. Configure your GitLab connection")
	fmt.Fprintln(w, "  3. Set up your MCP client(s)")
	fmt.Fprintln(w)
}

func printSection(w io.Writer, title string) {
	fmt.Fprintln(w, "-------------------------------------------------------------------")
	fmt.Fprintf(w, "  %s\n", title)
	fmt.Fprintln(w, "-------------------------------------------------------------------")
	fmt.Fprintln(w)
}

func stepInstall(p *Prompter, w io.Writer) (string, error) {
	printSection(w, "Step 1: Binary Installation")

	currentPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting executable path: %w", err)
	}
	fmt.Fprintf(w, "Current location: %s\n", currentPath)

	defaultPath := filepath.Join(DefaultInstallDir(), DefaultBinaryName())

	installDir, err := p.AskStringDefault("Install to", defaultPath)
	if err != nil {
		return "", err
	}

	if strings.HasSuffix(installDir, DefaultBinaryName()) {
		installDir = strings.TrimSuffix(installDir, string(os.PathSeparator)+DefaultBinaryName())
		installDir = strings.TrimSuffix(installDir, DefaultBinaryName())
		if installDir == "" {
			installDir = "."
		}
	}

	expandedDir, err := ExpandPath(installDir)
	if err != nil {
		return "", fmt.Errorf("expanding path: %w", err)
	}

	installed, err := InstallBinary(expandedDir)
	if err != nil {
		fmt.Fprintf(w, "\n  ! Could not install binary: %v\n", err)
		fmt.Fprintln(w, "    Continuing with current location...")
		return currentPath, nil
	}

	fmt.Fprintf(w, "\n  * Binary installed to %s\n\n", installed)
	return installed, nil
}

func stepGitLabConfig(p *Prompter, w io.Writer, existing ServerConfig, hasExisting bool) (*ServerConfig, error) {
	printSection(w, "Step 2: GitLab Configuration")

	defaultURL := DefaultGitLabURL
	if hasExisting && existing.GitLabURL != "" {
		defaultURL = existing.GitLabURL
	}

	gitlabURL, err := p.AskStringDefault("GitLab URL", defaultURL)
	if err != nil {
		return nil, err
	}

	if _, parseErr := url.ParseRequestURI(gitlabURL); parseErr != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", gitlabURL, parseErr)
	}

	tokenURL := TokenCreationURL(gitlabURL)
	fmt.Fprintf(w, "\n  Need a token? Create one at:\n  %s\n", tokenURL)
	fmt.Fprintln(w, "  Required scope: api (full access to the GitLab API)")
	fmt.Fprintln(w)

	var token string
	if hasExisting && existing.GitLabToken != "" {
		token, err = p.AskPasswordDefault("Personal Access Token (glpat-...)", existing.GitLabToken)
	} else {
		token, err = p.AskPassword("Personal Access Token (glpat-...)")
	}
	if err != nil {
		return nil, err
	}

	masked := MaskToken(token)
	fmt.Fprintf(w, "\n  Token: %s\n", masked)
	fmt.Fprintf(w, "  * Token will be stored securely in %s\n", EnvFilePath())
	fmt.Fprintln(w, "    Client config files will NOT contain your token.")
	fmt.Fprintln(w)

	skipTLS := false
	if hasExisting {
		skipTLS = existing.SkipTLSVerify
	}

	return &ServerConfig{
		GitLabURL:     gitlabURL,
		GitLabToken:   token,
		SkipTLSVerify: skipTLS,
		MetaTools:     true,
		AutoUpdate:    true,
		LogLevel:      "info",
	}, nil
}

func stepOptions(p *Prompter, w io.Writer, cfg *ServerConfig) error {
	printSection(w, "Advanced Options")

	skipTLS, err := p.AskYesNo("Skip TLS verification?", false)
	if err != nil {
		return err
	}
	cfg.SkipTLSVerify = skipTLS

	metaTools, err := p.AskYesNo("Enable meta-tools?", true)
	if err != nil {
		return err
	}
	cfg.MetaTools = metaTools

	autoUpdate, err := p.AskYesNo("Enable auto-update?", true)
	if err != nil {
		return err
	}
	cfg.AutoUpdate = autoUpdate

	yolo, err := p.AskYesNo("Enable YOLO mode?", false)
	if err != nil {
		return err
	}
	cfg.YoloMode = yolo

	logIdx, err := p.AskChoice("Log level", LogLevelOptions)
	if err != nil {
		return err
	}
	cfg.LogLevel = LogLevelOptions[logIdx]
	fmt.Fprintln(w)

	return nil
}

func stepClients(p *Prompter, w io.Writer, cfg ServerConfig) error {
	printSection(w, "Step 3: MCP Client Configuration")

	clients := allClientsFn()
	options := make([]string, len(clients))
	defaults := make([]bool, len(clients))
	for i, c := range clients {
		if c.DisplayOnly {
			options[i] = fmt.Sprintf("%-28s (prints JSON to paste in IDE)", c.Name)
		} else {
			options[i] = fmt.Sprintf("%-28s %s", c.Name, c.ConfigPath)
		}
		defaults[i] = c.DefaultSelected
	}

	selected, err := p.AskMultiChoice("Select clients", options, defaults)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	printSection(w, "Writing Configurations")

	result := &Result{
		Config: cfg,
	}
	for i, sel := range selected {
		if sel {
			result.SelectedClients = append(result.SelectedClients, i)
		}
	}

	return Apply(w, result)
}
