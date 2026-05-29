// Command gen_llms generates llms.txt and llms-full.txt files. It creates an
// in-memory MCP server with all tools, resources, and prompts registered,
// introspects them via the SDK, and writes two files to the project root:
//
//   - llms.txt: concise llmstxt.org index for LLM discovery
//   - llms-full.txt: detailed companion reference with tool schemas
//
// Usage:
//
//	go run ./cmd/gen_llms/
//	go run ./cmd/gen_llms/ --check
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// maxFullDescRunes caps the length of tool descriptions in llms-full.txt to
	// keep the file scannable. When a description exceeds this limit, generation
	// falls back to its first sentence; if that is still too long, the text is
	// hard-truncated at the rune boundary.
	maxFullDescRunes             = 600
	llmsFileName                 = "llms.txt"
	llmsFullFileName             = "llms-full.txt"
	dynamicFindToolName          = "gitlab_find_action"
	dynamicExecuteActionToolName = "gitlab_execute_action"
	llmsSummaryItemFormat        = "- %s: %s\n"
	llmsBoldTitleFormat          = "**%s**\n\n"
)

type llmsCatalog struct {
	Individual              []*mcp.Tool
	IndividualSelfManaged   []*mcp.Tool
	MetaBase                []*mcp.Tool
	MetaEnterprise          []*mcp.Tool
	MetaGitLabComEnterprise []*mcp.Tool
	Dynamic                 []*mcp.Tool
	MetaRoutes              map[string]toolutil.ActionMap
	Resources               []*mcp.Resource
	ResourceTemplates       []*mcp.ResourceTemplate
	Prompts                 []*mcp.Prompt
}

func main() {
	checkOnly := flag.Bool("check", false, "validate generated llms files without writing them")
	flag.Parse()

	if err := run(*checkOnly); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate llms files: %v\n", err)
		os.Exit(1)
	}
}

// run introspects the live MCP catalog and regenerates llms.txt and
// llms-full.txt in the project root.
func run(checkOnly bool) error {
	rootDir, err := findProjectRoot()
	if err != nil {
		return err
	}

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
	version := readVersion(rootDir)
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
	dynamicTools, err := listDynamicTools(gitLabComClient)
	if err != nil {
		return err
	}
	metaCatalog, err := tools.BuildActionCatalog(gitLabComClient, tools.ActionCatalogOptions{Enterprise: true})
	if err != nil {
		return fmt.Errorf("build meta action catalog: %w", err)
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
		Dynamic:                 dynamicTools,
		MetaRoutes:              metaCatalog.ActionMaps(),
		Resources:               res,
		ResourceTemplates:       resTpl,
		Prompts:                 promptList,
	}

	if writeErr := writeLLMSTxt(version, catalog, checkOnly); writeErr != nil {
		return writeErr
	}
	if writeErr := writeLLMSFullTxt(version, catalog, checkOnly); writeErr != nil {
		return writeErr
	}

	if checkOnly {
		fmt.Printf("Validated llms.txt and llms-full.txt\n")
		return nil
	}
	fmt.Printf("Generated llms.txt (%d max tools, %d base meta, %d GitLab.com enterprise meta, %d dynamic tools, %d resources, %d prompts)\n",
		len(catalog.Individual), len(catalog.MetaBase), len(catalog.MetaGitLabComEnterprise), len(catalog.Dynamic), len(catalog.Resources)+len(catalog.ResourceTemplates)+1, len(catalog.Prompts))
	fmt.Printf("Generated llms-full.txt\n")
	return nil
}

// readVersion reads the VERSION file from the project root.
func readVersion(rootDir string) string {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return "unknown"
	}
	defer func() { _ = root.Close() }()

	data, err := root.ReadFile("VERSION")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// newSession creates an in-memory MCP server+client session with high page size.
func newSession(setupServer func(*mcp.Server) error) (session *mcp.ClientSession, cleanup func(), err error) {
	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: "gen-llms", Version: "0.0.1"}, opts)
	if setupErr := setupServer(server); setupErr != nil {
		return nil, nil, setupErr
	}
	toolutil.LockdownInputSchemas(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("server connect: %w", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "gen-llms-client", Version: "0.0.1"}, nil)
	session, err = mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		_ = serverSession.Close()
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
	session, cleanup, err := newSession(func(server *mcp.Server) error {
		if meta {
			return tools.RegisterAllMeta(server, client, false)
		}
		tools.RegisterAll(server, client, true)
		return nil
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
	session, cleanup, err := newSession(func(server *mcp.Server) error {
		return tools.RegisterAllMeta(server, client, true)
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

// listDynamicTools returns the visible two-tool dynamic catalog from a real MCP
// tools/list session.
func listDynamicTools(client *gitlabclient.Client) ([]*mcp.Tool, error) {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build dynamic action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add dynamic standalone catalog: %w", err)
	}
	session, cleanup, err := newSession(func(server *mcp.Server) error {
		dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("list dynamic tools: %w", err)
	}
	sortDynamicTools(result.Tools)
	if contractErr := validateDynamicToolContract(result.Tools); contractErr != nil {
		return nil, contractErr
	}
	return result.Tools, nil
}

func sortDynamicTools(dynamicTools []*mcp.Tool) {
	order := map[string]int{
		dynamicFindToolName:          0,
		dynamicExecuteActionToolName: 1,
	}
	sort.SliceStable(dynamicTools, func(i, j int) bool {
		left, leftOK := order[dynamicTools[i].Name]
		right, rightOK := order[dynamicTools[j].Name]
		if leftOK && rightOK {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return dynamicTools[i].Name < dynamicTools[j].Name
	})
}

func validateDynamicToolContract(dynamicTools []*mcp.Tool) error {
	expected := []string{dynamicFindToolName, dynamicExecuteActionToolName}
	if len(dynamicTools) != len(expected) {
		return fmt.Errorf("expected %d dynamic tools, got %d", len(expected), len(dynamicTools))
	}
	for i, name := range expected {
		if dynamicTools[i].Name != name {
			return fmt.Errorf("unexpected dynamic tool %q at position %d", dynamicTools[i].Name, i)
		}
	}
	return nil
}

// listResources returns the static resources and resource templates advertised
// by the MCP server, including the surface-aware tool manifest template.
func listResources(client *gitlabclient.Client) ([]*mcp.Resource, []*mcp.ResourceTemplate, error) {
	dynamicCatalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		return nil, nil, fmt.Errorf("build dynamic action catalog: %w", err)
	}
	dynamicCatalog, err = dynamictools.AddStandaloneCatalog(dynamicCatalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("add dynamic standalone catalog: %w", err)
	}
	dynamicTools, err := listDynamicTools(client)
	if err != nil {
		return nil, nil, err
	}
	session, cleanup, err := newSession(func(server *mcp.Server) error {
		resources.Register(server, client)
		resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
			Surface: config.ToolSurfaceDynamic,
			Tools:   dynamicTools,
			Catalog: dynamicCatalog,
		})
		resources.RegisterWorkflowGuides(server)
		return nil
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
	session, cleanup, err := newSession(func(server *mcp.Server) error {
		prompts.Register(server, client)
		return nil
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
func writeLLMSTxt(version string, catalog llmsCatalog, checkOnly bool) error {
	var b strings.Builder
	resourceCount := len(catalog.Resources) + len(catalog.ResourceTemplates) + 1 // +1 for workspace_roots
	domains := classifyMetaDomains(catalog.MetaBase)

	b.WriteString("# gitlab-mcp-server\n\n")
	b.WriteString("> A Model Context Protocol (MCP) server that exposes GitLab REST API v4 and GraphQL operations as tools for AI assistants.\n\n")
	fmt.Fprintf(&b, "gitlab-mcp-server v%s is a single static binary (Go) that runs locally via stdio or remotely via HTTP transport.\n", version)
	fmt.Fprintf(&b, "It provides up to %d individual MCP tools across %d GitLab API domains, %d base meta-tools, %d self-managed enterprise meta-tools, %d GitLab.com Enterprise meta-tools,\n",
		len(catalog.Individual), countDomains(catalog.Individual), len(catalog.MetaBase), len(catalog.MetaEnterprise), len(catalog.MetaGitLabComEnterprise))
	fmt.Fprintf(&b, "a default %d-tool dynamic find/execute surface, %d resources, %d prompts, and 6 MCP capabilities. Cross-platform: Windows, Linux, macOS (amd64 + arm64).\n\n",
		len(catalog.Dynamic), resourceCount, len(catalog.Prompts))

	b.WriteString("Quick start:\n\n")
	b.WriteString("1. Download the binary for your platform from the Releases page\n")
	b.WriteString("2. Run `gitlab-mcp-server --setup` to launch the interactive setup wizard\n")
	b.WriteString("3. The wizard configures your AI client (VS Code, Cursor, Claude Desktop, etc.)\n\n")

	b.WriteString("Configuration (environment variables, stdio mode):\n\n")
	fmt.Fprintf(&b, "- GITLAB_URL: GitLab instance URL (default: `%s`; set for self-managed instances)\n", config.DefaultGitLabURL)
	b.WriteString("- GITLAB_TOKEN: Personal Access Token (required)\n")
	b.WriteString("- GITLAB_SKIP_TLS_VERIFY: Skip TLS verification for self-signed certs (default: false)\n")
	b.WriteString("- TOOL_SURFACE: Canonical catalog selector: meta, individual, dynamic\n")
	b.WriteString("- META_TOOLS: Deprecated compatibility selector; prefer TOOL_SURFACE for new configs\n")
	b.WriteString("- CAPABILITY_SURFACE: Use minimal with dynamic mode when startup context must be tiny\n")
	b.WriteString("- GITLAB_ENTERPRISE: Enable enterprise/premium tools; GitLab.com Enterprise also exposes Orbit Knowledge Graph tools (default: false)\n\n")

	b.WriteString("Tool domains:\n\n")
	b.WriteString(strings.Join(domains, ", "))
	b.WriteString(".\n\n")

	b.WriteString("Dynamic toolset (default mode):\n\n")
	b.WriteString("When TOOL_SURFACE is unset or set to dynamic, the server exposes only gitlab_find_action and gitlab_execute_action while keeping the same canonical GitLab action catalog. Models should find an action with its exact schema, then execute the canonical domain.action ID returned by find. Set TOOL_SURFACE=meta to use consolidated domain meta-tools instead.\n\n")
	for _, t := range catalog.Dynamic {
		desc := firstSentence(t.Description)
		desc = truncateRunes(desc, 80)
		fmt.Fprintf(&b, llmsSummaryItemFormat, t.Name, desc)
	}
	b.WriteString("\n")

	b.WriteString("Meta-tool overview:\n\n")
	fmt.Fprintf(&b, "When TOOL_SURFACE=meta, %d domain meta-tools are registered instead of\n", len(catalog.MetaBase))
	fmt.Fprintf(&b, "up to %d individual tools. Enterprise/Premium entries register %d meta-tools on self-managed GitLab,\n", len(catalog.Individual), len(catalog.MetaEnterprise))
	fmt.Fprintf(&b, "or %d on GitLab.com when Orbit is available. Each meta-tool groups related operations under a single\n", len(catalog.MetaGitLabComEnterprise))
	b.WriteString("tool with an \"action\" parameter. Key meta-tools:\n\n")
	for _, t := range catalog.MetaBase {
		desc := firstSentence(toolutil.StripMetaToolDescriptionPrefix(t.Description))
		desc = truncateRunes(desc, 80)
		fmt.Fprintf(&b, llmsSummaryItemFormat, t.Name, desc)
	}
	b.WriteString("\n")

	b.WriteString("Resources:\n\n")
	fmt.Fprintf(&b, "%d read-only resources:\n\n", resourceCount)
	for _, r := range catalog.Resources {
		fmt.Fprintf(&b, llmsSummaryItemFormat, r.URI, r.Name)
	}
	for _, r := range catalog.ResourceTemplates {
		fmt.Fprintf(&b, llmsSummaryItemFormat, r.URITemplate, r.Name)
	}
	b.WriteString("- gitlab://workspace/roots: Workspace Roots\n")
	b.WriteString("\n")

	b.WriteString("Prompts:\n\n")
	fmt.Fprintf(&b, "%d prompts:\n\n", len(catalog.Prompts))
	for _, p := range catalog.Prompts {
		desc := firstSentence(p.Description)
		desc = truncateRunes(desc, 80)
		fmt.Fprintf(&b, llmsSummaryItemFormat, p.Name, desc)
	}
	b.WriteString("\n")

	b.WriteString("## Documentation\n\n")
	writeLLMSLink(&b, "Getting started", "docs/getting-started.md", "Installation and first-run guide")
	writeLLMSLink(&b, "Configuration", "docs/configuration.md", "Full configuration reference")
	writeLLMSLink(&b, "Environment variables", "docs/env-reference.md", "Environment variable reference")
	writeLLMSLink(&b, "HTTP server mode", "docs/http-server-mode.md", "Remote MCP transport setup")
	writeLLMSLink(&b, "Security model", "docs/security.md", "Authentication, read-only mode, safe mode, and security controls")

	b.WriteString("\n## Tool References\n\n")
	writeLLMSLink(&b, "Dynamic tools", "docs/dynamic-tools.md", "Low-token find/execute mode and usage pattern")
	writeLLMSLink(&b, "Meta-tools", "docs/meta-tools.md", "Consolidated domain meta-tool action reference")
	writeLLMSLink(&b, "All tools", "docs/tools/README.md", "Complete per-domain tool reference")
	writeLLMSLink(&b, "Resources", "docs/resources-reference.md", "Read-only MCP resource reference")
	writeLLMSLink(&b, "Prompts", "docs/prompts-reference.md", "Reusable MCP prompt templates")

	b.WriteString("\n## Optional\n\n")
	writeLLMSLink(&b, "Full LLM reference", llmsFullFileName, "Generated companion reference with tool schemas, resource listings, and prompts")
	writeLLMSLink(&b, "Architecture", "docs/architecture.md", "Internal architecture and catalog-first runtime overview")
	writeLLMSLink(&b, "Output format", "docs/output-format.md", "Markdown and structured output conventions")
	writeLLMSLink(&b, "Troubleshooting", "docs/troubleshooting.md", "Common setup and runtime issues")
	writeLLMSLink(&b, "Evaluation results", "docs/testing/model-results.md", "Surface evaluation summaries for model behavior")

	content := b.String()
	if err := validateLLMSTxt(content); err != nil {
		return fmt.Errorf("validate llms.txt: %w", err)
	}
	if err := writeGeneratedFile(llmsFileName, content, checkOnly); err != nil {
		return fmt.Errorf("write llms.txt: %w", err)
	}
	return nil
}

func writeLLMSLink(b *strings.Builder, label, target, description string) {
	fmt.Fprintf(b, "- [%s](%s): %s\n", label, target, description)
}

func validateLLMSTxt(content string) error {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return errors.New("missing H1 title")
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") || strings.HasPrefix(strings.TrimSpace(lines[0]), "##") {
		return fmt.Errorf("first line must be an H1 title, got %q", lines[0])
	}

	state := llmsTxtValidationState{}
	for index, rawLine := range lines[1:] {
		lineNumber := index + 2
		line := strings.TrimSpace(rawLine)
		if err := state.validateLine(lineNumber, line); err != nil {
			return err
		}
	}
	if !state.foundSummary {
		return errors.New("missing blockquote summary")
	}
	if state.inFileListSection && !state.sectionHasLink {
		return fmt.Errorf("section %q has no file links", state.currentSection)
	}
	return nil
}

type llmsTxtValidationState struct {
	foundSummary      bool
	inFileListSection bool
	currentSection    string
	sectionHasLink    bool
}

func (s *llmsTxtValidationState) validateLine(lineNumber int, line string) error {
	if line == "" {
		return nil
	}
	if strings.HasPrefix(line, "#") {
		return s.validateHeading(lineNumber, line)
	}
	if !s.inFileListSection {
		if strings.HasPrefix(line, ">") {
			s.foundSummary = true
		}
		return nil
	}
	if err := validateLLMSFileListItem(line); err != nil {
		return fmt.Errorf("line %d: %w", lineNumber, err)
	}
	s.sectionHasLink = true
	return nil
}

func (s *llmsTxtValidationState) validateHeading(lineNumber int, line string) error {
	if !strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "###") {
		return fmt.Errorf("line %d: llms.txt only allows H1 plus H2 file-list sections", lineNumber)
	}
	if s.inFileListSection && !s.sectionHasLink {
		return fmt.Errorf("section %q has no file links", s.currentSection)
	}
	s.currentSection = strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if s.currentSection == "" {
		return fmt.Errorf("line %d: H2 section title is empty", lineNumber)
	}
	s.inFileListSection = true
	s.sectionHasLink = false
	return nil
}

func validateLLMSFileListItem(line string) error {
	if !strings.HasPrefix(line, "- [") {
		return fmt.Errorf("file-list entries must start with a markdown link, got %q", line)
	}
	closeLabel := strings.Index(line, "](")
	if closeLabel <= len("- [") {
		return fmt.Errorf("file-list entry is missing markdown link label, got %q", line)
	}
	urlStart := closeLabel + len("](")
	urlEnd := strings.Index(line[urlStart:], ")")
	if urlEnd < 0 {
		return fmt.Errorf("file-list entry is missing markdown link target, got %q", line)
	}
	url := strings.TrimSpace(line[urlStart : urlStart+urlEnd])
	if url == "" {
		return fmt.Errorf("file-list entry has empty markdown link target, got %q", line)
	}
	remainder := strings.TrimSpace(line[urlStart+urlEnd+1:])
	if remainder != "" && !strings.HasPrefix(remainder, ":") {
		return fmt.Errorf("file-list entry notes must follow ':' after the markdown link, got %q", line)
	}
	return nil
}

func validateLLMSFullTxt(content string) error {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		return errors.New("missing H1 title")
	}
	for _, section := range []string{"## Dynamic Toolset", "## Meta-Tools", "## Individual Tools", "## Resources", "## Prompts"} {
		if !strings.Contains(content, section+"\n") {
			return fmt.Errorf("missing %q section", section)
		}
	}
	return nil
}

// writeLLMSFullTxt generates the detailed llms-full.txt with tool schemas.
func writeLLMSFullTxt(version string, catalog llmsCatalog, checkOnly bool) error {
	var b strings.Builder
	resourceCount := len(catalog.Resources) + len(catalog.ResourceTemplates) + 1

	b.WriteString("# gitlab-mcp-server — Full Reference\n\n")
	fmt.Fprintf(&b, "> Version %s | up to %d tools | %d base meta-tools; %d self-managed enterprise meta-tools; %d GitLab.com Enterprise meta-tools | %d dynamic tools | %d resources | %d prompts\n\n",
		version, len(catalog.Individual), len(catalog.MetaBase), len(catalog.MetaEnterprise), len(catalog.MetaGitLabComEnterprise), len(catalog.Dynamic), resourceCount, len(catalog.Prompts))

	writeLLMSFullDynamicTools(&b, catalog.Dynamic)
	writeLLMSFullMetaTools(&b, catalog)
	writeLLMSFullIndividualTools(&b, catalog)

	writeLLMSFullResources(&b, catalog, resourceCount)
	writeLLMSFullPrompts(&b, catalog.Prompts)

	content := b.String()
	if err := validateLLMSFullTxt(content); err != nil {
		return fmt.Errorf("validate llms-full.txt: %w", err)
	}
	if err := writeGeneratedFile(llmsFullFileName, content, checkOnly); err != nil {
		return fmt.Errorf("write llms-full.txt: %w", err)
	}
	return nil
}

func writeLLMSFullMetaTools(b *strings.Builder, catalog llmsCatalog) {
	b.WriteString("## Meta-Tools\n\n")
	b.WriteString("Meta-tools are enabled with `TOOL_SURFACE=meta`. Each groups related\n")
	b.WriteString("operations under a single tool with an `action` parameter.\n\n")
	for _, tool := range catalog.MetaBase {
		writeLLMSFullMetaTool(b, tool, catalog.MetaRoutes)
	}
	writeLLMSFullEnterpriseOnlyMetaTools(b, catalog)
}

func writeLLMSFullEnterpriseOnlyMetaTools(b *strings.Builder, catalog llmsCatalog) {
	enterpriseOnly := enterpriseOnlyMetaTools(catalog.MetaBase, catalog.MetaGitLabComEnterprise)
	if len(enterpriseOnly) == 0 {
		return
	}
	b.WriteString("## Enterprise-Only Meta-Tools\n\n")
	fmt.Fprintf(b, "These %d tools require GITLAB_ENTERPRISE=true. GitLab.com-only tools, including Orbit, also require GITLAB_URL=%s.\n\n", len(enterpriseOnly), config.DefaultGitLabURL)
	for _, tool := range enterpriseOnly {
		writeLLMSFullMetaTool(b, tool, catalog.MetaRoutes)
	}
}

func enterpriseOnlyMetaTools(baseTools, gitLabComTools []*mcp.Tool) []*mcp.Tool {
	baseNames := make(map[string]bool, len(baseTools))
	for _, tool := range baseTools {
		baseNames[tool.Name] = true
	}
	enterpriseOnly := make([]*mcp.Tool, 0)
	for _, tool := range gitLabComTools {
		if !baseNames[tool.Name] {
			enterpriseOnly = append(enterpriseOnly, tool)
		}
	}
	return enterpriseOnly
}

func writeLLMSFullMetaTool(b *strings.Builder, tool *mcp.Tool, routesByTool map[string]toolutil.ActionMap) {
	fmt.Fprintf(b, toolutil.FmtMdH3, tool.Name)
	if tool.Title != "" {
		fmt.Fprintf(b, llmsBoldTitleFormat, tool.Title)
	}
	b.WriteString(tool.Description)
	b.WriteString("\n\n")
	writeAnnotations(b, tool.Annotations)
	b.WriteString("\n")
	if routes, ok := routesByTool[tool.Name]; ok {
		writeActionOutputSchemas(b, tool.Name, routes)
	}
}

func writeLLMSFullIndividualTools(b *strings.Builder, catalog llmsCatalog) {
	b.WriteString("## Individual Tools\n\n")
	fmt.Fprintf(b, "When `TOOL_SURFACE=individual`, up to %d individual tools are registered on GitLab.com Enterprise/Premium; self-managed Enterprise/Premium registers %d.\n", len(catalog.Individual), len(catalog.IndividualSelfManaged))
	b.WriteString("Grouped by domain:\n\n")
	for _, domain := range sortedDomains(catalog.Individual) {
		domainTools := groupByDomain(catalog.Individual)[domain]
		fmt.Fprintf(b, "### %s (%d tools)\n\n", domain, len(domainTools))
		for _, tool := range domainTools {
			writeLLMSFullIndividualTool(b, tool)
		}
	}
}

func sortedDomains(mcpTools []*mcp.Tool) []string {
	domainTools := groupByDomain(mcpTools)
	domainNames := make([]string, 0, len(domainTools))
	for domain := range domainTools {
		domainNames = append(domainNames, domain)
	}
	sort.Strings(domainNames)
	return domainNames
}

func writeLLMSFullIndividualTool(b *strings.Builder, tool *mcp.Tool) {
	fmt.Fprintf(b, "#### %s\n\n", tool.Name)
	b.WriteString(compactToolDescription(tool.Description))
	b.WriteString("\n\n")
	writeInputSchema(b, tool.InputSchema)
	writeAnnotations(b, tool.Annotations)
	b.WriteString("\n")
}

func compactToolDescription(description string) string {
	desc := firstParagraph(description)
	if utf8.RuneCountInString(desc) <= maxFullDescRunes {
		return desc
	}
	if sentence := firstSentence(desc); sentence != "" && utf8.RuneCountInString(sentence) <= maxFullDescRunes {
		return sentence
	}
	return truncateRunes(desc, maxFullDescRunes)
}

func writeLLMSFullDynamicTools(b *strings.Builder, dynamicTools []*mcp.Tool) {
	b.WriteString("## Dynamic Toolset\n\n")
	b.WriteString("Dynamic mode is the default when `TOOL_SURFACE` is unset or set to `dynamic`. It exposes `gitlab_find_action` and `gitlab_execute_action` over the same canonical action catalog used by the meta-tool catalog. Models should find candidate actions with exact input schemas and safety metadata, then execute the canonical `domain.action` ID. Set `TOOL_SURFACE=meta` to use consolidated domain meta-tools instead.\n\n")
	for _, tool := range dynamicTools {
		fmt.Fprintf(b, toolutil.FmtMdH3, tool.Name)
		if tool.Title != "" {
			fmt.Fprintf(b, llmsBoldTitleFormat, tool.Title)
		}
		b.WriteString(tool.Description)
		b.WriteString("\n\n")
		writeInputSchema(b, tool.InputSchema)
		writeAnnotations(b, tool.Annotations)
		b.WriteString("\n")
	}
}

func writeLLMSFullResources(b *strings.Builder, catalog llmsCatalog, resourceCount int) {
	b.WriteString("## Resources\n\n")
	fmt.Fprintf(b, "%d resources providing read-only access to GitLab data.\n\n", resourceCount)
	for _, resource := range catalog.Resources {
		writeLLMSResource(b, resource.Name, resource.URI, "URI", resource.MIMEType, resource.Description)
	}
	for _, template := range catalog.ResourceTemplates {
		writeLLMSResource(b, template.Name, template.URITemplate, "URI Template", template.MIMEType, template.Description)
	}
	b.WriteString("### Workspace Roots\n\n")
	b.WriteString("- **URI**: `gitlab://workspace/roots`\n")
	b.WriteString("- **Description**: Lists workspace root directories reported by the MCP client\n\n")
}

func writeLLMSResource(b *strings.Builder, name, uri, uriLabel, mimeType, description string) {
	fmt.Fprintf(b, toolutil.FmtMdH3, name)
	fmt.Fprintf(b, "- **%s**: `%s`\n", uriLabel, uri)
	if mimeType != "" {
		fmt.Fprintf(b, "- **MIME**: %s\n", mimeType)
	}
	if description != "" {
		fmt.Fprintf(b, "- **Description**: %s\n", description)
	}
	b.WriteString("\n")
}

func writeLLMSFullPrompts(b *strings.Builder, promptDefs []*mcp.Prompt) {
	b.WriteString("## Prompts\n\n")
	fmt.Fprintf(b, "%d prompt templates for AI-assisted GitLab workflows.\n\n", len(promptDefs))
	for _, prompt := range promptDefs {
		fmt.Fprintf(b, toolutil.FmtMdH3, prompt.Name)
		if prompt.Description != "" {
			b.WriteString(prompt.Description)
			b.WriteString("\n\n")
		}
		writeLLMSPromptArguments(b, prompt.Arguments)
	}
}

func writeLLMSPromptArguments(b *strings.Builder, arguments []*mcp.PromptArgument) {
	if len(arguments) == 0 {
		return
	}
	b.WriteString("**Arguments:**\n\n")
	for _, argument := range arguments {
		req := ""
		if argument.Required {
			req = " (required)"
		}
		desc := argument.Description
		if desc == "" {
			desc = argument.Name
		}
		fmt.Fprintf(b, "- `%s`%s: %s\n", argument.Name, req, desc)
	}
	b.WriteString("\n")
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
		typ := schemaTypeLabel(prop)
		desc, _ := prop["description"].(string)
		desc = strings.TrimSuffix(desc, ",required")
		req := ""
		if required[name] {
			req = " (required)"
		}
		if desc != "" {
			fmt.Fprintf(b, "- `%s` (%s)%s: %s\n", name, typ, req, desc)
		} else {
			fmt.Fprintf(b, "- `%s` (%s)%s\n", name, typ, req)
		}
	}
	b.WriteString("\n")
}

func schemaTypeLabel(schema map[string]any) string {
	types := schemaTypeValues(schema["type"])
	types = removeSchemaType(types, "null")
	if len(types) == 0 {
		if _, ok := schema["items"]; ok {
			return "array"
		}
		if _, ok := schema["properties"]; ok {
			return "object"
		}
		return "any"
	}
	if slices.Contains(types, "array") {
		items, _ := schema["items"].(map[string]any)
		itemType := schemaTypeLabel(items)
		if itemType == "" || itemType == "any" {
			return "array"
		}
		return "array of " + pluralSchemaType(itemType)
	}
	if len(types) == 1 {
		return types[0]
	}
	return strings.Join(types, " or ")
}

func schemaTypeValues(raw any) []string {
	switch value := raw.(type) {
	case string:
		return []string{value}
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func removeSchemaType(types []string, remove string) []string {
	filtered := types[:0]
	for _, typ := range types {
		if typ != remove {
			filtered = append(filtered, typ)
		}
	}
	return filtered
}

func pluralSchemaType(typ string) string {
	if itemType, ok := strings.CutPrefix(typ, "array of "); ok {
		return "arrays of " + itemType
	}
	switch typ {
	case "integer":
		return "integers"
	case "number":
		return "numbers"
	case "string":
		return "strings"
	case "boolean":
		return "booleans"
	case "object":
		return "objects"
	default:
		if strings.Contains(typ, " or ") {
			return "values"
		}
		return typ + "s"
	}
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

// writeGeneratedFile writes or checks generated content in the project root.
func writeGeneratedFile(name, content string, checkOnly bool) error {
	if !isGeneratedLLMSFile(name) {
		return fmt.Errorf("unexpected generated file %q", name)
	}
	dir, err := findProjectRoot()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	if checkOnly {
		existing, readErr := root.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		if normalizeLineEndings(string(existing)) != normalizeLineEndings(content) {
			return fmt.Errorf("%s is out of date; run go run ./cmd/gen_llms/", name)
		}
		return nil
	}
	return root.WriteFile(name, []byte(content), 0o644)
}

func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func isGeneratedLLMSFile(name string) bool {
	switch name {
	case llmsFileName, llmsFullFileName:
		return true
	default:
		return false
	}
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
