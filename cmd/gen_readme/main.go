// Command gen_readme auto-generates the meta-tool table in README.md.
// It creates an in-memory MCP server, lists meta-tools, counts actions
// from the inputSchema enum, and replaces content between
// <!-- START TOOLS --> and <!-- END TOOLS --> markers.
//
// Usage:
//
//	go run ./cmd/gen_readme/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

const (
	startMarker = "<!-- START TOOLS -->"
	endMarker   = "<!-- END TOOLS -->"
	readmePath  = "README.md"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{ //#nosec G101 -- not a real credential, test-only dummy token
		GitLabURL:   srv.URL,
		GitLabToken: "gen-readme-token",
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	baseTools := listMetaTools(client, false)
	allTools := listMetaTools(client, true)

	table := buildTable(baseTools, allTools)

	if replaceErr := replaceSection(readmePath, table); replaceErr != nil {
		return replaceErr
	}

	fmt.Printf("Updated %s (%d base / %d enterprise meta-tools)\n",
		readmePath, len(baseTools), len(allTools))
	return nil
}

func listMetaTools(client *gitlabclient.Client, enterprise bool) []*mcp.Tool {
	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: "gen-readme", Version: "0.0.1"}, opts)
	tools.RegisterAllMeta(server, client, enterprise)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "gen-readme-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTools: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}
	return result.Tools
}

// actionCount extracts the number of actions from the tool's inputSchema
// by looking for the "action" property's enum values.
func actionCount(tool *mcp.Tool) int {
	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return 0
	}

	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	err = json.Unmarshal(raw, &schema)
	if err != nil {
		return 0
	}
	if action, ok := schema.Properties["action"]; ok {
		return len(action.Enum)
	}
	return 0
}

type toolInfo struct {
	Name        string
	Description string
	Actions     int
	Enterprise  bool
}

// firstSentence returns the first sentence (up to the first sentence-ending
// period or newline), whichever is shorter. Skips common abbreviations.
func firstSentence(s string) string {
	// Take the first line.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	// Trim to first real sentence boundary, skipping abbreviations.
	if idx := findSentenceEnd(s); idx >= 0 {
		s = s[:idx+1]
	}
	// Escape pipe characters for Markdown tables.
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

// abbreviations that should not be treated as sentence boundaries.
var abbreviations = []string{"e.g.", "i.e.", "etc.", "vs.", "approx.", "dept.", "est.", "govt.", "incl."}

// findSentenceEnd returns the index of the first ". " that is NOT part of a
// common abbreviation, or -1 if none found.
func findSentenceEnd(s string) int {
	offset := 0
	for {
		i := strings.Index(s[offset:], ". ")
		if i < 0 {
			return -1
		}
		pos := offset + i
		isAbbrev := false
		for _, abbr := range abbreviations {
			if len(abbr) <= pos+1 && s[pos+1-len(abbr):pos+1] == abbr {
				isAbbrev = true
				break
			}
		}
		if !isAbbrev {
			return pos
		}
		offset = pos + 2
	}
}

func buildTable(baseTools, allTools []*mcp.Tool) string {
	baseSet := make(map[string]bool, len(baseTools))
	for _, t := range baseTools {
		baseSet[t.Name] = true
	}

	var infos []toolInfo
	for _, t := range allTools {
		infos = append(infos, toolInfo{
			Name:        t.Name,
			Description: firstSentence(t.Description),
			Actions:     actionCount(t),
			Enterprise:  !baseSet[t.Name],
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		// Base tools first, then enterprise, alphabetical within each group.
		if infos[i].Enterprise != infos[j].Enterprise {
			return !infos[i].Enterprise
		}
		return infos[i].Name < infos[j].Name
	})

	var b strings.Builder
	b.WriteString("| Meta-Tool | Actions | Description |\n")
	b.WriteString("|-----------|:-------:|-------------|\n")

	for _, info := range infos {
		name := fmt.Sprintf("`%s`", info.Name)
		if info.Enterprise {
			name += " 🏢"
		}
		fmt.Fprintf(&b, "| %s | %d | %s |\n", name, info.Actions, info.Description)
	}

	fmt.Fprintf(&b, "\n**%d base** / **%d with enterprise** meta-tools. See [Meta-Tools Reference](docs/meta-tools.md) for the complete list with actions and examples.\n",
		len(baseTools), len(allTools))

	return b.String()
}

func replaceSection(path, content string) error {
	data, err := os.ReadFile(path) //#nosec G304 -- path is a hardcoded constant
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	text := string(data)
	startIdx := strings.Index(text, startMarker)
	endIdx := strings.Index(text, endMarker)
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return fmt.Errorf("markers %s / %s not found in %s", startMarker, endMarker, path)
	}

	// Replace content between markers (preserve markers).
	before := text[:startIdx+len(startMarker)]
	after := text[endIdx:]
	result := before + "\n\n" + content + "\n" + after

	return os.WriteFile(filepath.Clean(path), []byte(result), 0o644) //#nosec G306,G703 -- README path is a compile-time constant, not user input
}
