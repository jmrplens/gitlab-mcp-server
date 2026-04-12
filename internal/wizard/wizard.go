package wizard

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Result holds all data collected from the user by any UI mode.
type Result struct {
	InstallDir      string
	BinaryPath      string
	Config          ServerConfig
	SelectedClients []int // indices into AllClients()
}

// LogLevelOptions lists the configurable log levels.
var LogLevelOptions = []string{"debug", "info", "warn", "error"}

// Apply writes the env file with secrets, then writes MCP configurations
// for selected clients, and prints a summary.
// It is called after any UI mode collects user input.
func Apply(w io.Writer, result *Result) error {
	// Write secrets to env file before configuring clients
	envPath, err := writeEnvFileFn(result.Config)
	if err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}
	fmt.Fprintf(w, "  * %-28s -> %s\n", "Secrets (env file)", envPath)

	clients := allClientsFn()
	var configured []ClientInfo

	for _, idx := range result.SelectedClients {
		if idx < 0 || idx >= len(clients) {
			continue
		}
		client := clients[idx]

		if client.DisplayOnly {
			if err = printJetBrainsConfig(w, result.Config); err != nil {
				return err
			}
			configured = append(configured, client)
			continue
		}

		entry := GenerateEntry(client.ID, result.Config)
		rootKey := RootKey(client.ID)

		if err = MergeServerEntry(client.ConfigPath, rootKey, ServerEntryName, entry); err != nil {
			fmt.Fprintf(w, "  ! %-28s FAILED: %v\n", client.Name, err)
			continue
		}
		fmt.Fprintf(w, "  * %-28s -> %s\n", client.Name, client.ConfigPath)
		configured = append(configured, client)
	}

	printSummary(w, configured)
	return nil
}

func printJetBrainsConfig(w io.Writer, cfg ServerConfig) error {
	entry := GenerateEntry(ClientJetBrains, cfg)
	snippet := map[string]any{
		RootKey(ClientJetBrains): map[string]any{
			ServerEntryName: entry,
		},
	}
	data, err := json.MarshalIndent(snippet, "    ", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  * %-28s (copy this JSON into your IDE):\n\n", "JetBrains IDEs")
	fmt.Fprintf(w, "    %s\n\n", string(data))
	return nil
}

func printSummary(w io.Writer, configured []ClientInfo) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "===================================================================")
	fmt.Fprintln(w, "  Setup Complete!")
	fmt.Fprintln(w, "===================================================================")
	fmt.Fprintln(w)

	if len(configured) == 0 {
		fmt.Fprintln(w, "No clients were configured.")
	} else {
		fmt.Fprintln(w, "The following clients are now configured:")
		for _, c := range configured {
			fmt.Fprintf(w, "  - %s -- %s\n", c.Name, RestartHint(c.ID))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "To reconfigure later, run: gitlab-mcp-server --setup")
	fmt.Fprintln(w, "For help: gitlab-mcp-server -h")
	fmt.Fprintln(w)
}

// MaskToken returns the first 8 characters of a token followed by asterisks.
func MaskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:8] + strings.Repeat("*", len(token)-8)
}
