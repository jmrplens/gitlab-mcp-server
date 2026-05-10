// Command audit_tokens measures the LLM context window overhead of all
// registered MCP tool definitions. It creates in-memory MCP servers in both
// individual and meta-tool modes, serializes tool definitions to JSON, and
// estimates token counts using a byte-based heuristic (bytes / 4).
//
// Usage:
//
//	go run ./cmd/audit_tokens/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Token audit constants define the in-memory MCP session identity and the
// byte-to-token conversion heuristic used by the report.
const (
	serverName  = "audit-tokens"
	clientName  = "audit-tokens-client"
	auditVer    = "0.0.1"
	bytesPerTok = 4 // Approximate: 1 token ≈ 4 bytes for English text (cl100k_base average)
)

// toolTokenInfo stores the serialized size estimate for one MCP tool.
type toolTokenInfo struct {
	Name   string
	Domain string
	Tokens int
	Bytes  int
}

// resourceRegistrationOptions selects which MCP resource groups are advertised
// for token-audit measurements.
type resourceRegistrationOptions struct {
	Core           bool
	MetaSchema     bool
	WorkflowGuides bool
	WorkspaceRoots bool
}

// main creates the mock GitLab-backed client, measures all MCP catalog modes,
// and prints token overhead comparisons for tools, resources, and prompts.
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{
		GitLabURL:   srv.URL,
		GitLabToken: "audit-token",
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}

	metaBaseRoutes := buildMetaActionMaps(client, false)
	metaEnterpriseRoutes := buildMetaActionMaps(client, true)
	dynamicBaseCatalog, err := dynamictools.AddStandaloneCatalog(actionregistry.FromActionMaps(metaBaseRoutes), client, dynamictools.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add standalone base dynamic catalog: %v\n", err)
		os.Exit(1)
	}
	dynamicEnterpriseCatalog, err := dynamictools.AddStandaloneCatalog(actionregistry.FromActionMaps(metaEnterpriseRoutes), client, dynamictools.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add standalone enterprise dynamic catalog: %v\n", err)
		os.Exit(1)
	}
	dynamicBaseRoutes := dynamicBaseCatalog.ActionMaps()
	dynamicEnterpriseRoutes := dynamicEnterpriseCatalog.ActionMaps()

	individualTools := listTools(client, config.ToolSurfaceIndividual, true)
	metaBaseTools := listTools(client, config.ToolSurfaceMeta, false)
	metaEnterpriseTools := listTools(client, config.ToolSurfaceMeta, true)
	dynamic3BaseTools := listDynamicTools(dynamicBaseCatalog)
	dynamic3EnterpriseTools := listDynamicTools(dynamicEnterpriseCatalog)
	dynamic2BaseTools := listDynamic2Tools(dynamicBaseCatalog)
	dynamic2EnterpriseTools := listDynamic2Tools(dynamicEnterpriseCatalog)

	individualInfo := measureTools(individualTools)
	metaBaseInfo := measureTools(metaBaseTools)
	metaEnterpriseInfo := measureTools(metaEnterpriseTools)
	dynamic3BaseInfo := measureTools(dynamic3BaseTools)
	dynamic3EnterpriseInfo := measureTools(dynamic3EnterpriseTools)
	dynamic2BaseInfo := measureTools(dynamic2BaseTools)
	dynamic2EnterpriseInfo := measureTools(dynamic2EnterpriseTools)

	individualResourceTokens := measureResources(client, nil)
	metaBaseResourceTokens := measureResources(client, metaBaseRoutes)
	dynamicBaseResourceTokens := measureResources(client, dynamicBaseRoutes)
	dynamicMinimalResourceTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{WorkspaceRoots: true})
	promptTokens := measurePrompts(client)

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("  gitlab-mcp-server — Token Overhead Audit")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()

	// Mode comparison
	indTotal := totalTokens(individualInfo)
	metaTotal := totalTokens(metaBaseInfo)
	metaEntTotal := totalTokens(metaEnterpriseInfo)
	dynamic3Total := totalTokens(dynamic3BaseInfo)
	dynamic3EntTotal := totalTokens(dynamic3EnterpriseInfo)
	dynamic2Total := totalTokens(dynamic2BaseInfo)
	dynamic2EntTotal := totalTokens(dynamic2EnterpriseInfo)
	metaBaseCatalogActions := countActions(metaBaseRoutes)
	metaEnterpriseCatalogActions := countActions(metaEnterpriseRoutes)
	baseReachableActions := countActions(dynamicBaseRoutes)
	enterpriseReachableActions := countActions(dynamicEnterpriseRoutes)

	fmt.Println("## Mode Comparison")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Mode\tTools\tReachable actions\tTokens\tBytes\n")
	fmt.Fprintf(tw, "  ────\t─────\t──────────────\t──────\t─────\n")
	fmt.Fprintf(tw, "  Individual (all)\t%d\t%d\t%s\t%s\n", len(individualInfo), len(individualInfo), fmtNum(indTotal), fmtNum(indTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Meta-tools (base)\t%d\t%d\t%s\t%s\n", len(metaBaseInfo), baseReachableActions, fmtNum(metaTotal), fmtNum(metaTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Meta-tools (enterprise)\t%d\t%d\t%s\t%s\n", len(metaEnterpriseInfo), enterpriseReachableActions, fmtNum(metaEntTotal), fmtNum(metaEntTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Dynamic-3 (base)\t%d\t%d\t%s\t%s\n", len(dynamic3BaseInfo), baseReachableActions, fmtNum(dynamic3Total), fmtNum(dynamic3Total*bytesPerTok))
	fmt.Fprintf(tw, "  Dynamic-3 (enterprise)\t%d\t%d\t%s\t%s\n", len(dynamic3EnterpriseInfo), enterpriseReachableActions, fmtNum(dynamic3EntTotal), fmtNum(dynamic3EntTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Dynamic-2 (base)\t%d\t%d\t%s\t%s\n", len(dynamic2BaseInfo), baseReachableActions, fmtNum(dynamic2Total), fmtNum(dynamic2Total*bytesPerTok))
	fmt.Fprintf(tw, "  Dynamic-2 (enterprise)\t%d\t%d\t%s\t%s\n", len(dynamic2EnterpriseInfo), enterpriseReachableActions, fmtNum(dynamic2EntTotal), fmtNum(dynamic2EntTotal*bytesPerTok))
	_ = tw.Flush()
	fmt.Println()
	if addedStandalone := baseReachableActions - metaBaseCatalogActions; addedStandalone > 0 {
		fmt.Printf("  Reachable action counts include %d standalone utility actions (project discovery + interactive flows) that are visible tools in meta mode and folded into the dynamic catalog.\n", addedStandalone)
		fmt.Printf("  Catalog-only meta route counts: base %s / enterprise %s.\n", fmtNum(metaBaseCatalogActions), fmtNum(metaEnterpriseCatalogActions))
		fmt.Println()
	}

	if indTotal > 0 {
		savings := float64(indTotal-metaTotal) / float64(indTotal) * 100
		fmt.Printf("  Meta-tools reduce token overhead by %.1f%% vs individual mode\n", savings)
		fmt.Println()
	}
	if indTotal > 0 {
		savings := float64(indTotal-dynamic3Total) / float64(indTotal) * 100
		fmt.Printf("  Dynamic-3 mode reduces visible tool token overhead by %.1f%% vs individual mode\n", savings)
		candidateSavings := float64(indTotal-dynamic2Total) / float64(indTotal) * 100
		fmt.Printf("  Dynamic-2 candidate reduces visible tool token overhead by %.1f%% vs individual mode\n", candidateSavings)
		fmt.Println()
	}

	// Shared overhead (resources + prompts)
	fmt.Println("## Shared Overhead (Resources + Prompts)")
	fmt.Println()
	fmt.Printf("  Resources (individual): ~%s tokens (%s bytes)\n", fmtNum(individualResourceTokens), fmtNum(individualResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (meta-tools): ~%s tokens (%s bytes)\n", fmtNum(metaBaseResourceTokens), fmtNum(metaBaseResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (dynamic): ~%s tokens (%s bytes)\n", fmtNum(dynamicBaseResourceTokens), fmtNum(dynamicBaseResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (dynamic-minimal candidate): ~%s tokens (%s bytes)\n", fmtNum(dynamicMinimalResourceTokens), fmtNum(dynamicMinimalResourceTokens*bytesPerTok))
	fmt.Printf("  Prompts (full): ~%s tokens (%s bytes)\n", fmtNum(promptTokens), fmtNum(promptTokens*bytesPerTok))
	fmt.Println("  Prompts (dynamic-minimal candidate): ~0 tokens (0 bytes)")
	fmt.Printf("  Individual total: ~%s tokens\n", fmtNum(individualResourceTokens+promptTokens))
	fmt.Printf("  Meta-tool total:  ~%s tokens\n", fmtNum(metaBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic total:    ~%s tokens\n", fmtNum(dynamicBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic-minimal candidate total: ~%s tokens\n", fmtNum(dynamicMinimalResourceTokens))
	fmt.Println()

	fmt.Println("## Minimal Capability Candidate")
	fmt.Println()
	fmt.Println("  Required for dynamic-3 action use: the three dynamic tools. `gitlab_describe_tools` returns exact schemas inline, so meta-schema resources are not required.")
	fmt.Println("  Experimental dynamic-2 action use combines discovery and schemas in `gitlab_find_action`, plus `gitlab_execute_tool` for final execution.")
	fmt.Println("  Retained candidate resource: `gitlab://workspace/roots` for local project discovery from .git/config remotes.")
	fmt.Println("  Optional in the candidate: static GitLab data resources, meta-schema resources, workflow guide resources, and prompt templates.")
	if dynamicBaseResourceTokens+promptTokens > 0 {
		savings := float64(dynamicBaseResourceTokens+promptTokens-dynamicMinimalResourceTokens) / float64(dynamicBaseResourceTokens+promptTokens) * 100
		fmt.Printf("  Shared-overhead reduction: %.1f%% vs current dynamic resources+prompts\n", savings)
	}
	fmt.Println()

	// Top 30 individual tools by token cost
	fmt.Println("## Top 30 Individual Tools by Token Cost")
	fmt.Println()
	printTopTools(individualInfo, 30)

	// Top 20 meta-tools by token cost
	fmt.Println("## Meta-Tools by Token Cost (base)")
	fmt.Println()
	printTopTools(metaBaseInfo, len(metaBaseInfo))

	// Dynamic tools by token cost
	fmt.Println("## Dynamic Tools by Token Cost (base)")
	fmt.Println()
	printTopTools(dynamic3BaseInfo, len(dynamic3BaseInfo))

	// Dynamic-2 tools by token cost
	fmt.Println("## Dynamic-2 Tools by Token Cost (base)")
	fmt.Println()
	printTopTools(dynamic2BaseInfo, len(dynamic2BaseInfo))

	// Domain aggregation for individual tools
	fmt.Println("## Domain Totals (Individual Mode, Top 20)")
	fmt.Println()
	printDomainTotals(individualInfo, 20)

	// Grand total
	fmt.Println("## Grand Total (what an LLM sees)")
	fmt.Println()
	fmt.Printf("  Individual mode: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(indTotal), fmtNum(individualResourceTokens+promptTokens), fmtNum(indTotal+individualResourceTokens+promptTokens))
	fmt.Printf("  Meta-tool mode:  ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(metaTotal), fmtNum(metaBaseResourceTokens+promptTokens), fmtNum(metaTotal+metaBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic-3 mode:  ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamic3Total), fmtNum(dynamicBaseResourceTokens+promptTokens), fmtNum(dynamic3Total+dynamicBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic-3 minimal candidate: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamic3Total), fmtNum(dynamicMinimalResourceTokens), fmtNum(dynamic3Total+dynamicMinimalResourceTokens))
	fmt.Printf("  Dynamic-2 mode:  ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamic2Total), fmtNum(dynamicBaseResourceTokens+promptTokens), fmtNum(dynamic2Total+dynamicBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic-2 minimal candidate: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamic2Total), fmtNum(dynamicMinimalResourceTokens), fmtNum(dynamic2Total+dynamicMinimalResourceTokens))
	fmt.Println()
}

// listTools registers either individual tools or meta-tools on an in-memory MCP
// server and returns the published tool definitions for measurement.
func listTools(client *gitlabclient.Client, toolSurface string, enterprise bool) []*mcp.Tool {
	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, opts)

	switch toolSurface {
	case config.ToolSurfaceMeta:
		if err := tools.RegisterAllMeta(server, client, enterprise); err != nil {
			fmt.Fprintf(os.Stderr, "register meta tools: %v\n", err)
			os.Exit(1)
		}
		tools.RegisterMCPMeta(server, client, nil)
	case config.ToolSurfaceIndividual:
		tools.RegisterAll(server, client, enterprise)
	default:
		fmt.Fprintf(os.Stderr, "unknown tool surface %q\n", toolSurface)
		os.Exit(1)
	}
	return listToolsFromServer(server)
}

// listDynamicTools registers the low-token dynamic public toolset backed by
// action routes and returns the advertised tool definitions.
func listDynamicTools(catalog *actionregistry.Catalog) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{PageSize: 2000})
	dynamictools.RegisterCatalogTools(server, catalog)
	return listToolsFromServer(server)
}

// listDynamic2Tools registers the experimental find/execute dynamic public
// toolset backed by action routes and returns the advertised tools.
func listDynamic2Tools(catalog *actionregistry.Catalog) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{PageSize: 2000})
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return listToolsFromServer(server)
}

// buildMetaActionMaps builds the action route catalog that backs both
// meta-schema resources and the dynamic toolset.
func buildMetaActionMaps(client *gitlabclient.Client, enterprise bool) map[string]toolutil.ActionMap {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build action catalog: %v\n", err)
		os.Exit(1)
	}
	return catalog.ActionMaps()
}

// listToolsFromServer connects to server in-memory and returns the advertised
// tool definitions.
func listToolsFromServer(server *mcp.Server) []*mcp.Tool {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
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

// measureTools serializes each tool definition to JSON and estimates its token
// cost using the audit's byte-based heuristic.
func measureTools(toolList []*mcp.Tool) []toolTokenInfo {
	infos := make([]toolTokenInfo, 0, len(toolList))
	for _, t := range toolList {
		b, err := json.Marshal(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal tool %s: %v\n", t.Name, err)
			os.Exit(1)
		}
		tokens := len(b) / bytesPerTok
		domain := extractDomain(t.Name)
		infos = append(infos, toolTokenInfo{
			Name:   t.Name,
			Domain: domain,
			Tokens: tokens,
			Bytes:  len(b),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Tokens > infos[j].Tokens
	})
	return infos
}

// measureResources registers static, template, workflow, and optionally
// meta-schema MCP resources, then estimates the token cost of their advertised
// definitions.
func measureResources(client *gitlabclient.Client, metaRoutes map[string]toolutil.ActionMap) int {
	return measureResourcesWithOptions(client, metaRoutes, resourceRegistrationOptions{
		Core:           true,
		MetaSchema:     len(metaRoutes) > 0,
		WorkflowGuides: true,
		WorkspaceRoots: true,
	})
}

func measureResourcesWithOptions(client *gitlabclient.Client, metaRoutes map[string]toolutil.ActionMap, opts resourceRegistrationOptions) int {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	if opts.Core {
		resources.Register(server, client)
	}
	if opts.MetaSchema && len(metaRoutes) > 0 {
		resources.RegisterMetaSchemaResources(server, metaRoutes)
	}
	if opts.WorkspaceRoots {
		resources.RegisterWorkspaceRoots(server, roots.NewManager())
	}
	if opts.WorkflowGuides {
		resources.RegisterWorkflowGuides(server)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (resources): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (resources): %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	totalBytes := 0

	res, err := session.ListResources(ctx, nil)
	if err == nil {
		for _, r := range res.Resources {
			b, mErr := json.Marshal(r)
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "marshal resource %s: %v\n", r.Name, mErr)
				os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
			}
			totalBytes += len(b)
		}
	}

	tpl, err := session.ListResourceTemplates(ctx, nil)
	if err == nil {
		for _, t := range tpl.ResourceTemplates {
			b, mErr := json.Marshal(t)
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "marshal template %s: %v\n", t.Name, mErr)
				os.Exit(1)
			}
			totalBytes += len(b)
		}
	}

	return totalBytes / bytesPerTok
}

// countActions returns the number of actions in a route catalog.
func countActions(routes map[string]toolutil.ActionMap) int {
	total := 0
	for _, actions := range routes {
		total += len(actions)
	}
	return total
}

// measurePrompts registers MCP prompts and estimates the token cost of their
// advertised definitions.
func measurePrompts(client *gitlabclient.Client) int {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	prompts.Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (prompts): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (prompts): %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	totalBytes := 0
	p, err := session.ListPrompts(ctx, nil)
	if err == nil {
		for _, pr := range p.Prompts {
			b, mErr := json.Marshal(pr)
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "marshal prompt %s: %v\n", pr.Name, mErr)
				os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
			}
			totalBytes += len(b)
		}
	}
	return totalBytes / bytesPerTok
}

// extractDomain returns the GitLab tool domain from names like
// gitlab_{domain}_{action}. It returns "unknown" for malformed names.
func extractDomain(name string) string {
	// gitlab_{domain}_{action} → domain
	parts := strings.SplitN(name, "_", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

// totalTokens sums token estimates across a measured tool list.
func totalTokens(infos []toolTokenInfo) int {
	total := 0
	for _, i := range infos {
		total += i.Tokens
	}
	return total
}

// printTopTools writes the n most expensive tool definitions to stdout in a
// stable tabular format.
func printTopTools(infos []toolTokenInfo, n int) {
	if n > len(infos) {
		n = len(infos)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  #\tTokens\tBytes\tTool Name\n")
	fmt.Fprintf(tw, "  ─\t──────\t─────\t─────────\n")
	for i := range n {
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\n", i+1, fmtNum(infos[i].Tokens), fmtNum(infos[i].Bytes), infos[i].Name)
	}
	_ = tw.Flush()
	fmt.Println()
}

// printDomainTotals aggregates token estimates by tool domain and prints the
// highest-cost domains first.
func printDomainTotals(infos []toolTokenInfo, n int) {
	domainTotals := map[string]int{}
	domainCounts := map[string]int{}
	for _, i := range infos {
		domainTotals[i.Domain] += i.Tokens
		domainCounts[i.Domain]++
	}

	type domainEntry struct {
		Domain string
		Tokens int
		Count  int
	}
	entries := make([]domainEntry, 0, len(domainTotals))
	for d, t := range domainTotals {
		entries = append(entries, domainEntry{Domain: d, Tokens: t, Count: domainCounts[d]})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Tokens > entries[j].Tokens
	})

	if n > len(entries) {
		n = len(entries)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  #\tDomain\tTools\tTokens\n")
	fmt.Fprintf(tw, "  ─\t──────\t─────\t──────\n")
	for i := range n {
		fmt.Fprintf(tw, "  %d\t%s\t%d\t%s\n", i+1, entries[i].Domain, entries[i].Count, fmtNum(entries[i].Tokens))
	}
	_ = tw.Flush()
	fmt.Println()
}

// fmtNum formats integers with comma thousands separators for report tables.
func fmtNum(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
}
