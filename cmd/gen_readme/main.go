// Command gen_readme auto-generates the managed README.md sections.
// It creates an in-memory MCP server, lists meta-tools, counts actions
// from each InputSchema action enum, collects filesystem-level codebase
// metrics, and replaces content between the tools and statistics marker pairs.
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
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

// README generation markers define the managed tools table section.
const (
	startMarker = "<!-- START TOOLS -->"
	endMarker   = "<!-- END TOOLS -->"
	readmePath  = "README.md"
	repoRoot    = "."
)

// main regenerates the README meta-tool table and exits non-zero on failure.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run introspects base and Enterprise/Premium meta-tool catalogs and replaces
// the managed README section with a regenerated table.
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

	if replaceErr := replaceSection(readmePath, startMarker, endMarker, table); replaceErr != nil {
		return replaceErr
	}

	stats, statsErr := collectStats(repoRoot)
	if statsErr != nil {
		return fmt.Errorf("collecting stats: %w", statsErr)
	}
	if replaceErr := replaceSection(readmePath, statsStartMarker, statsEndMarker, renderStats(stats)); replaceErr != nil {
		return replaceErr
	}

	fmt.Printf("Updated %s (%d base / %d enterprise meta-tools, stats regenerated)\n",
		readmePath, len(baseTools), len(allTools))
	return nil
}

// listMetaTools registers meta-tools on an in-memory MCP server and returns the
// tool definitions exposed by tools/list.
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

// toolInfo is the normalized row model used to render the README meta-tool
// table.
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

// descriptionSummary returns the README-facing summary for a tool description.
// Meta-tools prepend a generated usage example and schema-resource hint for
// MCP clients; those lines are useful in tools/list but noisy in README tables.
func descriptionSummary(description string) string {
	return firstSentence(stripMetaToolDescriptionPrefix(description))
}

// stripMetaToolDescriptionPrefix removes the generated meta-tool usage header
// added by toolutil.MetaToolDescriptionPrefix while preserving standalone tool
// descriptions that happen to start with an example.
func stripMetaToolDescriptionPrefix(description string) string {
	lines := strings.Split(description, "\n")
	if len(lines) < 2 {
		return description
	}

	firstLine := strings.TrimSpace(lines[0])
	secondLine := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(firstLine, `Example: {"action":`) ||
		!strings.HasPrefix(secondLine, "For the params schema of any action") {
		return description
	}

	start := 2
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	return strings.Join(lines[start:], "\n")
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

// buildTable renders the managed README Markdown table, marking tools that are
// only available in the Enterprise/Premium catalog.
func buildTable(baseTools, allTools []*mcp.Tool) string {
	baseSet := make(map[string]bool, len(baseTools))
	for _, t := range baseTools {
		baseSet[t.Name] = true
	}

	var infos []toolInfo
	for _, t := range allTools {
		infos = append(infos, toolInfo{
			Name:        t.Name,
			Description: descriptionSummary(t.Description),
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
		actions := strconv.Itoa(info.Actions)
		if info.Actions == 0 {
			actions = "—"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", name, actions, info.Description)
	}

	fmt.Fprintf(&b, "\n**%d base** / **%d with enterprise** meta-tools. See [Meta-Tools Reference](docs/meta-tools.md) for the complete list with actions and examples.\n",
		len(baseTools), len(allTools))

	return b.String()
}

// replaceSection replaces content between startMark and endMark in the file at
// path, preserving both markers themselves.
func replaceSection(path, startMark, endMark, content string) error {
	data, err := os.ReadFile(path) //#nosec G304 -- path is a hardcoded constant
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	text := string(data)
	startIdx := strings.Index(text, startMark)
	if startIdx < 0 {
		return fmt.Errorf("start marker %s not found in %s", startMark, path)
	}

	// Search for endMark only after startMark to avoid matching an earlier
	// occurrence of the same marker string that belongs to a different section.
	searchFrom := startIdx + len(startMark)
	relEndIdx := strings.Index(text[searchFrom:], endMark)
	if relEndIdx < 0 {
		return fmt.Errorf("end marker %s not found after start marker in %s", endMark, path)
	}
	endIdx := searchFrom + relEndIdx

	before := text[:searchFrom]
	after := text[endIdx:]
	result := before + "\n\n" + content + "\n" + after

	return os.WriteFile(filepath.Clean(path), []byte(result), 0o644) //#nosec G306,G703 -- README path is a compile-time constant, not user input
}
