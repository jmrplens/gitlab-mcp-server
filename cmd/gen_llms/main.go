// Command gen_llms generates llms.txt and llms-full.txt files following the
// llmstxt.org standard. It creates an in-memory MCP server with all tools,
// resources, and prompts registered, introspects them via the SDK, and writes
// two files to the project root:
//
//   - llms.txt: concise overview for LLM discovery
//   - llms-full.txt: detailed listing with tool schemas
//
// Usage:
//
//	go run ./cmd/gen_llms/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// maxFullDescRunes caps the length of tool descriptions in llms-full.txt to
// keep the file scannable. When a description exceeds this limit, generation
// falls back to its first sentence; if that is still too long, the text is
// hard-truncated at the rune boundary.
const maxFullDescRunes = 600

type llmsCatalog struct {
	Individual              []*mcp.Tool
	IndividualSelfManaged   []*mcp.Tool
	MetaBase                []*mcp.Tool
	MetaEnterprise          []*mcp.Tool
	MetaGitLabComEnterprise []*mcp.Tool
	Resources               []*mcp.Resource
	ResourceTemplates       []*mcp.ResourceTemplate
	Prompts                 []*mcp.Prompt
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate llms files: %v\n", err)
		os.Exit(1)
	}
}

// run introspects the live MCP catalog and regenerates llms.txt and
// llms-full.txt in the project root.
func run() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{ //#nosec G101 -- not a real credential, test-only dummy token
		GitLabURL:   srv.URL,
		GitLabToken: "gen-llms-token",
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	gitLabComClient, err := gitlabclient.NewClient(&config.Config{ //#nosec G101 -- not a real credential, test-only dummy token
		GitLabURL:   config.DefaultGitLabURL,
		GitLabToken: "gen-llms-token",
	})
	if err != nil {
		return fmt.Errorf("create gitlab.com client: %w", err)
	}

	version := readVersion()
	res, resTpl, err := listResources(client)
	if err != nil {
		return err
	}
	individualSelfManaged, err := listTools(client, false)
	if err != nil {
		return err
	}
	individualGitLabCom, err := listTools(gitLabComClient, false)
	if err != nil {
		return err
	}
	metaBase, err := listTools(client, true)
	if err != nil {
		return err
	}
	metaEnterprise, err := listToolsEnterprise(client)
	if err != nil {
		return err
	}
	metaGitLabComEnterprise, err := listToolsEnterprise(gitLabComClient)
	if err != nil {
		return err
	}
	promptList, err := listPrompts(client)
	if err != nil {
		return err
	}
	catalog := llmsCatalog{
		Individual:              individualGitLabCom,
		IndividualSelfManaged:   individualSelfManaged,
		MetaBase:                metaBase,
		MetaEnterprise:          metaEnterprise,
		MetaGitLabComEnterprise: metaGitLabComEnterprise,
		Resources:               res,
		ResourceTemplates:       resTpl,
		Prompts:                 promptList,
	}

	if writeErr := writeLLMSTxt(version, catalog); writeErr != nil {
		return writeErr
	}
	if writeErr := writeLLMSFullTxt(version, catalog); writeErr != nil {
		return writeErr
	}

	fmt.Printf("Generated llms.txt (%d max tools, %d base meta, %d GitLab.com enterprise meta, %d resources, %d prompts)\n",
		len(catalog.Individual), len(catalog.MetaBase), len(catalog.MetaGitLabComEnterprise), len(catalog.Resources)+len(catalog.ResourceTemplates)+1, len(catalog.Prompts))
	fmt.Printf("Generated llms-full.txt\n")
	return nil
}

// readVersion reads the VERSION file from the project root.
func readVersion() string {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// newSession creates an in-memory MCP server+client session with high page size.
func newSession(setupServer func(*mcp.Server)) (session *mcp.ClientSession, cleanup func(), err error) {
	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: "gen-llms", Version: "0.0.1"}, opts)
	setupServer(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("server connect: %w", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "gen-llms-client", Version: "0.0.1"}, nil)
	session, err = mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		serverSession.Close()
		_ = serverSession.Wait()
		return nil, nil, fmt.Errorf("client connect: %w", err)
	}

	return session, func() {
		_ = session.Close()
		_ = serverSession.Wait()
	}, nil
}

// listTools returns either the enterprise individual catalog or the base
// meta-tool catalog, depending on meta.
func listTools(client *gitlabclient.Client, meta bool) ([]*mcp.Tool, error) {
	session, cleanup, err := newSession(func(server *mcp.Server) {
		if meta {
			tools.RegisterAllMeta(server, client, false)
		} else {
			tools.RegisterAll(server, client, true)
		}
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return result.Tools, nil
}

// listToolsEnterprise returns the Enterprise/Premium meta-tool catalog.
func listToolsEnterprise(client *gitlabclient.Client) ([]*mcp.Tool, error) {
	session, cleanup, err := newSession(func(server *mcp.Server) {
		tools.RegisterAllMeta(server, client, true)
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("list enterprise tools: %w", err)
	}
	return result.Tools, nil
}

// listResources returns the static resources and resource templates advertised
// by the MCP server, including the per-action meta-schema template.
func listResources(client *gitlabclient.Client) ([]*mcp.Resource, []*mcp.ResourceTemplate, error) {
	session, cleanup, err := newSession(func(server *mcp.Server) {
		metaRoutes := toolutil.CaptureMetaRoutes(func() {
			tools.RegisterAllMeta(server, client, false)
		})
		resources.Register(server, client)
		resources.RegisterMetaSchemaResources(server, metaRoutes)
		resources.RegisterWorkflowGuides(server)
	})
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	ctx := context.Background()
	res, err := session.ListResources(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list resources: %w", err)
	}
	tpl, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list resource templates: %w", err)
	}
	return res.Resources, tpl.ResourceTemplates, nil
}

// listPrompts returns all registered MCP prompt definitions for llms output.
func listPrompts(client *gitlabclient.Client) ([]*mcp.Prompt, error) {
	session, cleanup, err := newSession(func(server *mcp.Server) {
		prompts.Register(server, client)
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("list prompts: %w", err)
	}
	return result.Prompts, nil
}

// writeLLMSTxt generates the concise llms.txt overview.
func writeLLMSTxt(version string, catalog llmsCatalog) error {
	var b strings.Builder
	resourceCount := len(catalog.Resources) + len(catalog.ResourceTemplates) + 1 // +1 for workspace_roots

	b.WriteString("# gitlab-mcp-server\n\n")
	b.WriteString("> A Model Context Protocol (MCP) server that exposes GitLab REST API v4 and GraphQL operations as tools for AI assistants.\n\n")
	fmt.Fprintf(&b, "gitlab-mcp-server v%s is a single static binary (Go) that runs locally via stdio or remotely via HTTP transport.\n", version)
	fmt.Fprintf(&b, "It provides up to %d individual MCP tools across %d GitLab API domains, %d meta-tools (%d self-managed enterprise, %d on GitLab.com Enterprise),\n",
		len(catalog.Individual), countDomains(catalog.Individual), len(catalog.MetaBase), len(catalog.MetaEnterprise), len(catalog.MetaGitLabComEnterprise))
	fmt.Fprintf(&b, "%d resources, %d prompts, and 6 MCP capabilities. Cross-platform: Windows, Linux, macOS (amd64 + arm64).\n\n",
		resourceCount, len(catalog.Prompts))

	b.WriteString("## Quick Start\n\n")
	b.WriteString("1. Download the binary for your platform from the Releases page\n")
	b.WriteString("2. Run `gitlab-mcp-server --setup` to launch the interactive setup wizard\n")
	b.WriteString("3. The wizard configures your AI client (VS Code, Cursor, Claude Desktop, etc.)\n\n")

	b.WriteString("## Configuration (environment variables — stdio mode)\n\n")
	fmt.Fprintf(&b, "- GITLAB_URL: GitLab instance URL (default: %s; set for self-managed instances)\n", config.DefaultGitLabURL)
	b.WriteString("- GITLAB_TOKEN: Personal Access Token (required)\n")
	b.WriteString("- GITLAB_SKIP_TLS_VERIFY: Skip TLS verification for self-signed certs (default: false)\n")
	b.WriteString("- META_TOOLS: Enable meta-tools for reduced tool count (default: true)\n")
	b.WriteString("- GITLAB_ENTERPRISE: Enable enterprise/premium tools; GitLab.com Enterprise also exposes Orbit Knowledge Graph tools (default: false)\n\n")

	b.WriteString("## Tool Domains\n\n")
	domains := classifyMetaDomains(catalog.MetaBase)
	b.WriteString(strings.Join(domains, ", "))
	b.WriteString(".\n\n")

	b.WriteString("## Meta-Tools (default mode)\n\n")
	fmt.Fprintf(&b, "When META_TOOLS=true (default), %d domain meta-tools are registered instead of\n", len(catalog.MetaBase))
	fmt.Fprintf(&b, "up to %d individual tools. Enterprise/Premium entries register %d meta-tools on self-managed GitLab,\n", len(catalog.Individual), len(catalog.MetaEnterprise))
	fmt.Fprintf(&b, "or %d on GitLab.com when Orbit is available. Each meta-tool groups related operations under a single\n", len(catalog.MetaGitLabComEnterprise))
	b.WriteString("tool with an \"action\" parameter. Key meta-tools:\n\n")
	for _, t := range catalog.MetaBase {
		desc := firstSentence(stripMetaPrefix(t.Description))
		desc = truncateRunes(desc, 80)
		fmt.Fprintf(&b, "- %s — %s\n", t.Name, desc)
	}
	b.WriteString("\n")

	b.WriteString("## Resources\n\n")
	fmt.Fprintf(&b, "%d read-only resources:\n\n", resourceCount)
	for _, r := range catalog.Resources {
		fmt.Fprintf(&b, "- %s: %s\n", r.URI, r.Name)
	}
	for _, r := range catalog.ResourceTemplates {
		fmt.Fprintf(&b, "- %s: %s\n", r.URITemplate, r.Name)
	}
	b.WriteString("- gitlab://workspace/roots: Workspace Roots\n")
	b.WriteString("\n")

	b.WriteString("## Prompts\n\n")
	fmt.Fprintf(&b, "%d prompts:\n\n", len(catalog.Prompts))
	for _, p := range catalog.Prompts {
		desc := firstSentence(p.Description)
		desc = truncateRunes(desc, 80)
		fmt.Fprintf(&b, "- %s — %s\n", p.Name, desc)
	}
	b.WriteString("\n")

	b.WriteString("## Documentation\n\n")
	b.WriteString("- docs/configuration.md — Full configuration reference\n")
	b.WriteString("- docs/meta-tools.md — Meta-tool action reference\n")
	b.WriteString("- docs/tools/README.md — All tools reference\n")
	b.WriteString("- docs/resources-reference.md — Resources reference\n")
	b.WriteString("- docs/prompts-reference.md — Prompts reference\n")
	b.WriteString("- docs/security.md — Security model\n")
	b.WriteString("- llms-full.txt — Full tool listing with schemas\n")

	if err := writeFile("llms.txt", b.String()); err != nil {
		return fmt.Errorf("write llms.txt: %w", err)
	}
	return nil
}

// writeLLMSFullTxt generates the detailed llms-full.txt with tool schemas.
func writeLLMSFullTxt(version string, catalog llmsCatalog) error {
	var b strings.Builder
	resourceCount := len(catalog.Resources) + len(catalog.ResourceTemplates) + 1

	b.WriteString("# gitlab-mcp-server — Full Reference\n\n")
	fmt.Fprintf(&b, "> Version %s | up to %d tools | %d meta-tools (%d self-managed enterprise, %d GitLab.com Enterprise) | %d resources | %d prompts\n\n",
		version, len(catalog.Individual), len(catalog.MetaBase), len(catalog.MetaEnterprise), len(catalog.MetaGitLabComEnterprise), resourceCount, len(catalog.Prompts))

	// --- Meta-tools (primary mode) ---
	b.WriteString("## Meta-Tools\n\n")
	b.WriteString("Meta-tools are the default mode (META_TOOLS=true). Each groups related\n")
	b.WriteString("operations under a single tool with an `action` parameter.\n\n")

	allRoutes := toolutil.MetaRoutes()

	for _, t := range catalog.MetaBase {
		fmt.Fprintf(&b, toolutil.FmtMdH3, t.Name)
		if t.Title != "" {
			fmt.Fprintf(&b, "**%s**\n\n", t.Title)
		}
		b.WriteString(t.Description)
		b.WriteString("\n\n")
		writeAnnotations(&b, t.Annotations)
		b.WriteString("\n")
		if routes, ok := allRoutes[t.Name]; ok {
			writeActionOutputSchemas(&b, t.Name, routes)
		}
	}

	// Enterprise-only meta-tools
	baseNames := make(map[string]bool, len(catalog.MetaBase))
	for _, t := range catalog.MetaBase {
		baseNames[t.Name] = true
	}
	var enterpriseOnly []*mcp.Tool
	for _, t := range catalog.MetaGitLabComEnterprise {
		if !baseNames[t.Name] {
			enterpriseOnly = append(enterpriseOnly, t)
		}
	}
	if len(enterpriseOnly) > 0 {
		b.WriteString("## Enterprise-Only Meta-Tools\n\n")
		fmt.Fprintf(&b, "These %d tools require GITLAB_ENTERPRISE=true. GitLab.com-only tools, including Orbit, also require GITLAB_URL=%s.\n\n", len(enterpriseOnly), config.DefaultGitLabURL)
		for _, t := range enterpriseOnly {
			fmt.Fprintf(&b, toolutil.FmtMdH3, t.Name)
			if t.Title != "" {
				fmt.Fprintf(&b, "**%s**\n\n", t.Title)
			}
			b.WriteString(t.Description)
			b.WriteString("\n\n")
			writeAnnotations(&b, t.Annotations)
			b.WriteString("\n")
			if routes, ok := allRoutes[t.Name]; ok {
				writeActionOutputSchemas(&b, t.Name, routes)
			}
		}
	}

	// --- Individual tools (by domain) ---
	b.WriteString("## Individual Tools\n\n")
	fmt.Fprintf(&b, "When META_TOOLS=false, up to %d individual tools are registered on GitLab.com Enterprise/Premium; self-managed Enterprise/Premium registers %d.\n", len(catalog.Individual), len(catalog.IndividualSelfManaged))
	b.WriteString("Grouped by domain:\n\n")

	domainTools := groupByDomain(catalog.Individual)
	domainNames := make([]string, 0, len(domainTools))
	for d := range domainTools {
		domainNames = append(domainNames, d)
	}
	sort.Strings(domainNames)

	for _, domain := range domainNames {
		tls := domainTools[domain]
		fmt.Fprintf(&b, "### %s (%d tools)\n\n", domain, len(tls))
		for _, t := range tls {
			fmt.Fprintf(&b, "#### %s\n\n", t.Name)
			desc := firstParagraph(t.Description)
			if utf8.RuneCountInString(desc) > maxFullDescRunes {
				if sent := firstSentence(desc); sent != "" && utf8.RuneCountInString(sent) <= maxFullDescRunes {
					desc = sent
				} else {
					desc = truncateRunes(desc, maxFullDescRunes)
				}
			}
			b.WriteString(desc)
			b.WriteString("\n\n")
			writeInputSchema(&b, t.InputSchema)
			writeAnnotations(&b, t.Annotations)
			b.WriteString("\n")
		}
	}

	// --- Resources ---
	b.WriteString("## Resources\n\n")
	fmt.Fprintf(&b, "%d resources providing read-only access to GitLab data.\n\n", resourceCount)
	for _, r := range catalog.Resources {
		fmt.Fprintf(&b, toolutil.FmtMdH3, r.Name)
		fmt.Fprintf(&b, "- **URI**: `%s`\n", r.URI)
		if r.MIMEType != "" {
			fmt.Fprintf(&b, "- **MIME**: %s\n", r.MIMEType)
		}
		if r.Description != "" {
			fmt.Fprintf(&b, "- **Description**: %s\n", r.Description)
		}
		b.WriteString("\n")
	}
	for _, r := range catalog.ResourceTemplates {
		fmt.Fprintf(&b, toolutil.FmtMdH3, r.Name)
		fmt.Fprintf(&b, "- **URI Template**: `%s`\n", r.URITemplate)
		if r.MIMEType != "" {
			fmt.Fprintf(&b, "- **MIME**: %s\n", r.MIMEType)
		}
		if r.Description != "" {
			fmt.Fprintf(&b, "- **Description**: %s\n", r.Description)
		}
		b.WriteString("\n")
	}
	b.WriteString("### Workspace Roots\n\n")
	b.WriteString("- **URI**: `gitlab://workspace/roots`\n")
	b.WriteString("- **Description**: Lists workspace root directories reported by the MCP client\n\n")

	// --- Prompts ---
	b.WriteString("## Prompts\n\n")
	fmt.Fprintf(&b, "%d prompt templates for AI-assisted GitLab workflows.\n\n", len(catalog.Prompts))
	for _, p := range catalog.Prompts {
		fmt.Fprintf(&b, toolutil.FmtMdH3, p.Name)
		if p.Description != "" {
			b.WriteString(p.Description)
			b.WriteString("\n\n")
		}
		if len(p.Arguments) > 0 {
			b.WriteString("**Arguments:**\n\n")
			for _, a := range p.Arguments {
				req := ""
				if a.Required {
					req = " (required)"
				}
				desc := a.Description
				if desc == "" {
					desc = a.Name
				}
				fmt.Fprintf(&b, "- `%s`%s: %s\n", a.Name, req, desc)
			}
			b.WriteString("\n")
		}
	}

	if err := writeFile("llms-full.txt", b.String()); err != nil {
		return fmt.Errorf("write llms-full.txt: %w", err)
	}
	return nil
}

// writeAnnotations writes tool annotation hints to the builder.
func writeAnnotations(b *strings.Builder, ann *mcp.ToolAnnotations) {
	if ann == nil {
		return
	}
	dest := false
	if ann.DestructiveHint != nil {
		dest = *ann.DestructiveHint
	}
	openWorld := true
	if ann.OpenWorldHint != nil {
		openWorld = *ann.OpenWorldHint
	}
	fmt.Fprintf(b, "Annotations: readOnly=%v, destructive=%v, idempotent=%v, openWorld=%v\n",
		ann.ReadOnlyHint, dest, ann.IdempotentHint, openWorld)
}

// writeActionOutputSchemas writes a per-action output schema summary for a meta-tool.
func writeActionOutputSchemas(b *strings.Builder, _ string, routes toolutil.ActionMap) {
	if len(routes) == 0 {
		return
	}
	names := make([]string, 0, len(routes))
	for name, route := range routes {
		if route.OutputSchema != nil {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)

	b.WriteString("**Action Output Schemas:**\n\n")
	for _, name := range names {
		schema := routes[name].OutputSchema
		data, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		fmt.Fprintf(b, "<details><summary>%s</summary>\n\n```json\n%s\n```\n\n</details>\n\n", name, data)
	}
}

// writeInputSchema writes a compact representation of the tool's input schema.
func writeInputSchema(b *strings.Builder, schema any) {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return
	}
	props, ok := schemaMap["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return
	}

	required := map[string]bool{}
	if reqList, isSlice := schemaMap["required"].([]any); isSlice {
		for _, r := range reqList {
			if s, isStr := r.(string); isStr {
				required[s] = true
			}
		}
	}

	b.WriteString("**Parameters:**\n\n")
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		prop, isMap := props[name].(map[string]any)
		if !isMap {
			continue
		}
		typ, _ := prop["type"].(string)
		desc, _ := prop["description"].(string)
		desc = strings.TrimSuffix(desc, ",required")
		req := ""
		if required[name] {
			req = " (required)"
		}
		if desc != "" {
			if name != "search_type" {
				desc = truncateRunes(desc, 120)
			}
			fmt.Fprintf(b, "- `%s` (%s)%s: %s\n", name, typ, req, desc)
		} else {
			fmt.Fprintf(b, "- `%s` (%s)%s\n", name, typ, req)
		}
	}
	b.WriteString("\n")
}

// countDomains counts unique domain prefixes from tool names (gitlab_{domain}_*).
func countDomains(tls []*mcp.Tool) int {
	domains := map[string]bool{}
	for _, t := range tls {
		parts := strings.SplitN(t.Name, "_", 3)
		if len(parts) >= 2 {
			domains[parts[1]] = true
		}
	}
	return len(domains)
}

// classifyMetaDomains extracts human-friendly domain names from meta-tool names.
func classifyMetaDomains(metaTools []*mcp.Tool) []string {
	domains := make([]string, 0, len(metaTools))
	for _, t := range metaTools {
		name := strings.TrimPrefix(t.Name, "gitlab_")
		domains = append(domains, capitalizeWords(name))
	}
	sort.Strings(domains)
	return domains
}

// groupByDomain groups tools by their domain prefix.
func groupByDomain(tls []*mcp.Tool) map[string][]*mcp.Tool {
	result := map[string][]*mcp.Tool{}
	for _, t := range tls {
		parts := strings.SplitN(t.Name, "_", 3)
		domain := "other"
		if len(parts) >= 2 {
			domain = parts[1]
		}
		result[domain] = append(result[domain], t)
	}
	return result
}

// capitalizeWords capitalizes domain names for display.
func capitalizeWords(s string) string {
	acronyms := map[string]string{
		"ci":   "CI",
		"mr":   "MR",
		"dora": "DORA",
		"scim": "SCIM",
		"ssh":  "SSH",
		"gpg":  "GPG",
		"api":  "API",
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if v, ok := acronyms[p]; ok {
			parts[i] = v
		} else if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// truncateRunes truncates s to at most maxRunes runes, appending "..." if truncated.
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	var size int
	for range maxRunes {
		_, w := utf8.DecodeRuneInString(s[size:])
		size += w
	}
	return s[:size] + "..."
}

// stripMetaPrefix removes the generated meta-tool usage header so llms.txt
// summary lines show the actual user-facing domain description.
func stripMetaPrefix(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	firstLine := strings.TrimSpace(lines[0])
	secondLine := strings.TrimSpace(lines[1])
	if !strings.Contains(firstLine, `Example: {"action":`) ||
		!strings.HasPrefix(secondLine, "For the params schema of any action") {
		return s
	}
	start := 2
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	return strings.Join(lines[start:], "\n")
}

// firstParagraph returns text up to the first blank-line paragraph break (\n\n).
// Used to cut tool descriptions at a natural boundary instead of mid-sentence.
func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if before, _, ok := strings.Cut(s, "\n\n"); ok {
		return strings.TrimSpace(before)
	}
	return s
}

// firstSentence returns text up to the first sentence-ending period or newline.
// It skips common abbreviations (e.g., i.e., etc., vs.) to avoid false splits.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if i := findSentenceEnd(s); i >= 0 {
		return s[:i+1]
	}
	return s
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

// writeFile writes content to a file in the project root.
func writeFile(name, content string) error {
	dir, err := findProjectRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	return os.WriteFile(path, []byte(content), 0o644) //#nosec G306 -- generated documentation files, not secrets
}

// findProjectRoot walks up from cwd looking for go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}
