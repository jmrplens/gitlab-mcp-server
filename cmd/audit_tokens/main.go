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
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	ToolManifest   bool
	ToolSurface    string
	ToolList       []*mcp.Tool
	ToolCatalog    *actioncatalog.Catalog
	WorkflowGuides bool
	WorkspaceRoots bool
}

// main creates the mock GitLab-backed client, measures all MCP catalog modes,
// and prints token overhead comparisons for tools, resources, and prompts.
func main() {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	metaBaseRoutes := buildMetaActionMaps(client, false)
	metaEnterpriseRoutes := buildMetaActionMaps(client, true)
	dynamicBaseCatalog, err := dynamictools.AddStandaloneCatalog(actioncatalog.FromActionMaps(metaBaseRoutes), client, dynamictools.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add standalone base dynamic catalog: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit.
	}
	dynamicEnterpriseCatalog, err := dynamictools.AddStandaloneCatalog(actioncatalog.FromActionMaps(metaEnterpriseRoutes), client, dynamictools.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add standalone enterprise dynamic catalog: %v\n", err)
		os.Exit(1)
	}
	dynamicBaseRoutes := dynamicBaseCatalog.ActionMaps()
	dynamicEnterpriseRoutes := dynamicEnterpriseCatalog.ActionMaps()

	individualTools := listTools(client, config.ToolSurfaceIndividual, true)
	metaBaseTools := listTools(client, config.ToolSurfaceMeta, false)
	metaEnterpriseTools := listTools(client, config.ToolSurfaceMeta, true)
	dynamicBaseTools := listDynamicTools(dynamicBaseCatalog)
	dynamicEnterpriseTools := listDynamicTools(dynamicEnterpriseCatalog)

	individualInfo := measureTools(individualTools)
	metaBaseInfo := measureTools(metaBaseTools)
	metaEnterpriseInfo := measureTools(metaEnterpriseTools)
	dynamicBaseInfo := measureTools(dynamicBaseTools)
	dynamicEnterpriseInfo := measureTools(dynamicEnterpriseTools)

	individualResourceTokens := measureResources(client, nil, nil, individualTools, config.ToolSurfaceIndividual)
	metaBaseResourceTokens := measureResources(client, metaBaseRoutes, actioncatalog.FromActionMaps(metaBaseRoutes), metaBaseTools, config.ToolSurfaceMeta)
	dynamicBaseResourceTokens := measureResources(client, dynamicBaseRoutes, dynamicBaseCatalog, dynamicBaseTools, config.ToolSurfaceDynamic)
	dynamicMinimalResourceTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{
		ToolManifest:   true,
		ToolSurface:    config.ToolSurfaceDynamic,
		ToolList:       dynamicBaseTools,
		ToolCatalog:    dynamicBaseCatalog,
		WorkspaceRoots: true,
	})
	promptTokens := measurePrompts(client)

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("  gitlab-mcp-server — Token Overhead Audit")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()

	// Mode comparison
	indTotal := totalTokens(individualInfo)
	metaTotal := totalTokens(metaBaseInfo)
	metaEntTotal := totalTokens(metaEnterpriseInfo)
	dynamicTotal := totalTokens(dynamicBaseInfo)
	dynamicEntTotal := totalTokens(dynamicEnterpriseInfo)
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
	fmt.Fprintf(tw, "  Dynamic (base)\t%d\t%d\t%s\t%s\n", len(dynamicBaseInfo), baseReachableActions, fmtNum(dynamicTotal), fmtNum(dynamicTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Dynamic (enterprise)\t%d\t%d\t%s\t%s\n", len(dynamicEnterpriseInfo), enterpriseReachableActions, fmtNum(dynamicEntTotal), fmtNum(dynamicEntTotal*bytesPerTok))
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
		savings := float64(indTotal-dynamicTotal) / float64(indTotal) * 100
		fmt.Printf("  Dynamic mode reduces visible tool token overhead by %.1f%% vs individual mode\n", savings)
		fmt.Println()
	}

	// Shared overhead (resources + prompts)
	fmt.Println("## Shared Overhead (Resources + Prompts)")
	fmt.Println()
	fmt.Printf("  Resources (individual): ~%s tokens (%s bytes)\n", fmtNum(individualResourceTokens), fmtNum(individualResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (meta-tools): ~%s tokens (%s bytes)\n", fmtNum(metaBaseResourceTokens), fmtNum(metaBaseResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (dynamic): ~%s tokens (%s bytes)\n", fmtNum(dynamicBaseResourceTokens), fmtNum(dynamicBaseResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (dynamic-minimal): ~%s tokens (%s bytes)\n", fmtNum(dynamicMinimalResourceTokens), fmtNum(dynamicMinimalResourceTokens*bytesPerTok))
	fmt.Printf("  Prompts (full): ~%s tokens (%s bytes)\n", fmtNum(promptTokens), fmtNum(promptTokens*bytesPerTok))
	fmt.Println("  Prompts (dynamic-minimal): ~0 tokens (0 bytes)")
	fmt.Printf("  Individual total: ~%s tokens\n", fmtNum(individualResourceTokens+promptTokens))
	fmt.Printf("  Meta-tool total:  ~%s tokens\n", fmtNum(metaBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic total:    ~%s tokens\n", fmtNum(dynamicBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic-minimal total: ~%s tokens\n", fmtNum(dynamicMinimalResourceTokens))
	fmt.Println()

	fmt.Println("## Minimal Capability Candidate")
	fmt.Println()
	fmt.Println("  Required for dynamic action use: `gitlab_find_action` returns exact schemas inline, and `gitlab_execute_action` performs execution.")
	fmt.Println("  Retained minimal resources: `gitlab://workspace/roots` for local project discovery and `gitlab://tools` for action call shapes.")
	fmt.Println("  Optional in minimal mode: static GitLab data resources, workflow guide resources, and prompt templates.")
	if dynamicBaseResourceTokens+promptTokens > 0 {
		savings := float64(dynamicBaseResourceTokens+promptTokens-dynamicMinimalResourceTokens) / float64(dynamicBaseResourceTokens+promptTokens) * 100
		fmt.Printf("  Shared-overhead reduction: %.1f%% vs full dynamic resources+prompts\n", savings)
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
	printTopTools(dynamicBaseInfo, len(dynamicBaseInfo))

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
	fmt.Printf("  Dynamic mode:    ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamicTotal), fmtNum(dynamicBaseResourceTokens+promptTokens), fmtNum(dynamicTotal+dynamicBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic minimal: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamicTotal), fmtNum(dynamicMinimalResourceTokens), fmtNum(dynamicTotal+dynamicMinimalResourceTokens))
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
func listDynamicTools(catalog *actioncatalog.Catalog) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{PageSize: 2000})
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return listToolsFromServer(server)
}

// buildMetaActionMaps builds the action route catalog that backs both
// meta-tools and the dynamic toolset.
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

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		_ = serverSession.Close()
		fmt.Fprintf(os.Stderr, "client connect: %v\n", err)
		os.Exit(1)
	}

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()
		_ = serverSession.Close()
		fmt.Fprintf(os.Stderr, "ListTools: %v\n", err)
		os.Exit(1)
	}
	_ = session.Close()
	_ = serverSession.Close()
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

// measureResources registers static, template, workflow, and tool manifest MCP
// resources, then estimates the token cost of their advertised definitions.
func measureResources(client *gitlabclient.Client, metaRoutes map[string]toolutil.ActionMap, catalog *actioncatalog.Catalog, toolList []*mcp.Tool, toolSurface string) int {
	return measureResourcesWithOptions(client, metaRoutes, resourceRegistrationOptions{
		Core:           true,
		ToolManifest:   true,
		ToolSurface:    toolSurface,
		ToolList:       toolList,
		ToolCatalog:    catalog,
		WorkflowGuides: true,
		WorkspaceRoots: true,
	})
}

func measureResourcesWithOptions(client *gitlabclient.Client, metaRoutes map[string]toolutil.ActionMap, opts resourceRegistrationOptions) int {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	if opts.Core {
		resources.Register(server, client)
	}
	if opts.ToolManifest {
		resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
			Surface:    opts.ToolSurface,
			Tools:      opts.ToolList,
			Catalog:    opts.ToolCatalog,
			MetaRoutes: metaRoutes,
		})
	}
	if opts.WorkspaceRoots {
		resources.RegisterWorkspaceRoots(server, roots.NewManager())
	}
	if opts.WorkflowGuides {
		resources.RegisterWorkflowGuides(server)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server connect (resources): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		_ = serverSession.Close()
		fmt.Fprintf(os.Stderr, "client connect (resources): %v\n", err)
		os.Exit(1)
	}
	fatalWithSession := func(format string, args ...any) {
		_ = session.Close()
		_ = serverSession.Close()
		fmt.Fprintf(os.Stderr, format, args...)
		os.Exit(1)
	}

	totalBytes := 0

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		fatalWithSession("list resources: %v\n", err)
	}
	for _, r := range res.Resources {
		b, mErr := json.Marshal(r)
		if mErr != nil {
			fatalWithSession("marshal resource %s: %v\n", r.Name, mErr)
		}
		totalBytes += len(b)
	}

	tpl, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		fatalWithSession("list resource templates: %v\n", err)
	}
	for _, t := range tpl.ResourceTemplates {
		b, mErr := json.Marshal(t)
		if mErr != nil {
			fatalWithSession("marshal template %s: %v\n", t.Name, mErr)
		}
		totalBytes += len(b)
	}

	_ = session.Close()
	_ = serverSession.Close()
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

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server connect (prompts): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		_ = serverSession.Close()
		fmt.Fprintf(os.Stderr, "client connect (prompts): %v\n", err)
		os.Exit(1)
	}

	totalBytes := 0
	p, err := session.ListPrompts(ctx, nil)
	if err == nil {
		for _, pr := range p.Prompts {
			b, mErr := json.Marshal(pr)
			if mErr != nil {
				_ = session.Close()
				_ = serverSession.Close()
				fmt.Fprintf(os.Stderr, "marshal prompt %s: %v\n", pr.Name, mErr)
				os.Exit(1)
			}
			totalBytes += len(b)
		}
	}
	_ = session.Close()
	_ = serverSession.Close()
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
