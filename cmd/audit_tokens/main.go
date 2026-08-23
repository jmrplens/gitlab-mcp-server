// Command audit_tokens measures the LLM context window overhead of all
// registered MCP tool definitions. It creates in-memory MCP servers in both
// individual and meta-tool modes, serializes tool definitions to JSON, and
// counts tokens with the cl100k_base tokenizer (see countTokens), falling back
// to a bytes/4 heuristic only if the tokenizer is unavailable.
//
// Usage:
//
//	go run ./cmd/audit_tokens/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Token audit constants define the in-memory MCP session identity and the
// byte-to-token conversion heuristic used by the report.
const (
	serverName = "audit-tokens"
	clientName = "audit-tokens-client"
	auditVer   = "0.0.1"
)

// toolTokenInfo stores the serialized size estimate for one MCP tool.
//
// Name is the MCP tool name. Domain is the GitLab API domain parsed from
// the tool name (e.g. "project" for "gitlab_project_get"). Tokens is the
// cl100k_base token count from [countTokens]. Bytes is the raw JSON length.
type toolTokenInfo struct {
	Name   string
	Domain string
	Tokens int
	Bytes  int
}

// resourceRegistrationOptions selects which MCP resource groups are
// advertised for token-audit measurements.
//
// Core includes the static resources from [resources.Register]. ToolManifest
// adds the surface-aware tool manifest template and tools/{id} template;
// ToolSurface, ToolList, and ToolCatalog drive its content. WorkflowGuides
// includes the static workflow guides.
type resourceRegistrationOptions struct {
	Core           bool
	ToolManifest   bool
	ToolSurface    string
	ToolList       []*mcp.Tool
	ToolCatalog    *actioncatalog.Catalog
	WorkflowGuides bool
}

// main creates the mock GitLab-backed client, measures all MCP catalog modes,
// and prints token overhead comparisons for tools, resources, and prompts.
//
// With --compare-schemas it instead runs the meta-tool InputSchema sizing spike
// (formerly the standalone audit_meta_schema binary), comparing the byte cost of
// each META_PARAM_SCHEMA mode (opaque/full/compact).
func main() {
	footprint := flag.Bool("footprint", false, "measure all tiers \u00d7 surfaces \u00d7 META_PARAM_SCHEMA modes and write the README token-footprint section + docs/development/token-footprint.md")
	check := flag.Bool("check", false, "with -footprint, verify the README token-footprint section and docs/development/token-footprint.md are current without writing (exits non-zero on drift)")
	compareSchemas := flag.Bool("compare-schemas", false, "compare META_PARAM_SCHEMA modes (opaque/full/compact) for meta-tool InputSchema sizing instead of the normal token audit")
	topTools := flag.Int("top-tools", 30, "number of individual tools to list by token cost")
	topDomains := flag.Int("top-domains", 20, "number of domains to list by token cost")
	jsonOut := flag.Bool("json", false, "emit JSON summary instead of markdown report")
	flag.Parse()

	if *footprint {
		if err := runFootprintMode(*check); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *compareSchemas {
		if err := runMetaSchemaSizing(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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

	cmdutil.Progressf("audit_tokens: enumerating tools across individual/meta/dynamic surfaces…")
	individualTools := listTools(client, config.ToolSurfaceIndividual, true)
	metaBaseTools := listTools(client, config.ToolSurfaceMeta, false)
	metaEnterpriseTools := listTools(client, config.ToolSurfaceMeta, true)
	dynamicBaseTools := listDynamicTools(dynamicBaseCatalog)
	dynamicEnterpriseTools := listDynamicTools(dynamicEnterpriseCatalog)

	cmdutil.Progressf("audit_tokens: measuring token cost (tools, resources, prompts)…")
	individualInfo := measureTools(individualTools)
	metaBaseInfo := measureTools(metaBaseTools)
	metaEnterpriseInfo := measureTools(metaEnterpriseTools)
	dynamicBaseInfo := measureTools(dynamicBaseTools)
	dynamicEnterpriseInfo := measureTools(dynamicEnterpriseTools)

	individualResourceTokens := measureResources(client, nil, nil, individualTools, config.ToolSurfaceIndividual)
	metaBaseResourceTokens := measureResources(client, metaBaseRoutes, actioncatalog.FromActionMaps(metaBaseRoutes), metaBaseTools, config.ToolSurfaceMeta)
	dynamicBaseResourceTokens := measureResources(client, dynamicBaseRoutes, dynamicBaseCatalog, dynamicBaseTools, config.ToolSurfaceDynamic)
	dynamicMinimalResourceTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{
		ToolManifest: true,
		ToolSurface:  config.ToolSurfaceDynamic,
		ToolList:     dynamicBaseTools,
		ToolCatalog:  dynamicBaseCatalog,
	})
	promptTokens := measurePrompts(client)

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

	if *jsonOut {
		summary := struct {
			IndividualTools         int `json:"individual_tools"`
			MetaBaseTools           int `json:"meta_base_tools"`
			DynamicBaseTools        int `json:"dynamic_base_tools"`
			IndividualTokens        int `json:"individual_tokens"`
			MetaBaseTokens          int `json:"meta_base_tokens"`
			MetaEnterpriseTokens    int `json:"meta_enterprise_tokens"`
			DynamicBaseTokens       int `json:"dynamic_base_tokens"`
			DynamicEnterpriseTokens int `json:"dynamic_enterprise_tokens"`
			BaseReachableActions    int `json:"base_reachable_actions"`
			ResourceTokens          int `json:"resource_tokens"`
			PromptTokens            int `json:"prompt_tokens"`
		}{
			len(individualInfo), len(metaBaseInfo), len(dynamicBaseInfo),
			indTotal, metaTotal, metaEntTotal, dynamicTotal, dynamicEntTotal,
			baseReachableActions, individualResourceTokens, promptTokens,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(summary); encErr != nil {
			fmt.Fprintf(os.Stderr, "encode json: %v\n", encErr)
		}
		return
	}

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("  gitlab-mcp-server — Token Overhead Audit")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()

	fmt.Println("## Mode Comparison")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Mode\tTools\tReachable actions\tTokens\tBytes\n")
	fmt.Fprintf(tw, "  ────\t─────\t──────────────\t──────\t─────\n")
	fmt.Fprintf(tw, "  Individual (all)\t%d\t%d\t%s\t%s\n", len(individualInfo), len(individualInfo), fmtNum(indTotal), fmtNum(totalBytes(individualInfo)))
	fmt.Fprintf(tw, "  Meta-tools (base)\t%d\t%d\t%s\t%s\n", len(metaBaseInfo), baseReachableActions, fmtNum(metaTotal), fmtNum(totalBytes(metaBaseInfo)))
	fmt.Fprintf(tw, "  Meta-tools (enterprise)\t%d\t%d\t%s\t%s\n", len(metaEnterpriseInfo), enterpriseReachableActions, fmtNum(metaEntTotal), fmtNum(totalBytes(metaEnterpriseInfo)))
	fmt.Fprintf(tw, "  Dynamic (base)\t%d\t%d\t%s\t%s\n", len(dynamicBaseInfo), baseReachableActions, fmtNum(dynamicTotal), fmtNum(totalBytes(dynamicBaseInfo)))
	fmt.Fprintf(tw, "  Dynamic (enterprise)\t%d\t%d\t%s\t%s\n", len(dynamicEnterpriseInfo), enterpriseReachableActions, fmtNum(dynamicEntTotal), fmtNum(totalBytes(dynamicEnterpriseInfo)))
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
	fmt.Printf("  Resources (individual): ~%s tokens\n", fmtNum(individualResourceTokens))
	fmt.Printf("  Resources (meta-tools): ~%s tokens\n", fmtNum(metaBaseResourceTokens))
	fmt.Printf("  Resources (dynamic): ~%s tokens\n", fmtNum(dynamicBaseResourceTokens))
	fmt.Printf("  Resources (dynamic-minimal): ~%s tokens\n", fmtNum(dynamicMinimalResourceTokens))
	fmt.Printf("  Prompts (full): ~%s tokens\n", fmtNum(promptTokens))
	fmt.Println("  Prompts (dynamic-minimal): ~0 tokens (0 bytes)")
	fmt.Printf("  Individual total: ~%s tokens\n", fmtNum(individualResourceTokens+promptTokens))
	fmt.Printf("  Meta-tool total:  ~%s tokens\n", fmtNum(metaBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic total:    ~%s tokens\n", fmtNum(dynamicBaseResourceTokens+promptTokens))
	fmt.Printf("  Dynamic-minimal total: ~%s tokens\n", fmtNum(dynamicMinimalResourceTokens))
	fmt.Println()

	fmt.Println("## Minimal Capability Candidate")
	fmt.Println()
	fmt.Println("  Required for dynamic action use: `gitlab_find_action` returns exact schemas inline, and `gitlab_execute_action` performs execution.")
	fmt.Println("  Retained minimal resource: `gitlab://tools` for action call shapes.")
	fmt.Println("  Optional in minimal mode: static GitLab data resources, workflow guide resources, and prompt templates.")
	if dynamicBaseResourceTokens+promptTokens > 0 {
		savings := float64(dynamicBaseResourceTokens+promptTokens-dynamicMinimalResourceTokens) / float64(dynamicBaseResourceTokens+promptTokens) * 100
		fmt.Printf("  Shared-overhead reduction: %.1f%% vs full dynamic resources+prompts\n", savings)
	}
	fmt.Println()

	// Top 30 individual tools by token cost
	fmt.Println("## Top 30 Individual Tools by Token Cost")
	fmt.Println()
	printTopTools(individualInfo, *topTools)

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
	printDomainTotals(individualInfo, *topDomains)

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
		if err := tools.RegisterAllMeta(server, client, edition.TierForEnterprise(enterprise)); err != nil {
			fmt.Fprintf(os.Stderr, "register meta tools: %v\n", err)
			os.Exit(1)
		}
		tools.RegisterMCPMeta(server, client, nil)
	case config.ToolSurfaceIndividual:
		tools.RegisterAll(server, client, edition.TierForEnterprise(enterprise))
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
		tokens := countTokens(b)
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

	totalTokens := 0

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		fatalWithSession("list resources: %v\n", err)
	}
	for _, r := range res.Resources {
		b, mErr := json.Marshal(r)
		if mErr != nil {
			fatalWithSession("marshal resource %s: %v\n", r.Name, mErr)
		}
		totalTokens += countTokens(b)
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
		totalTokens += countTokens(b)
	}

	_ = session.Close()
	_ = serverSession.Close()
	return totalTokens
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

	totalTokens := 0
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
			totalTokens += countTokens(b)
		}
	}
	_ = session.Close()
	_ = serverSession.Close()
	return totalTokens
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

// totalBytes sums actual byte sizes across a measured tool list.
func totalBytes(infos []toolTokenInfo) int {
	total := 0
	for _, i := range infos {
		total += i.Bytes
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

// runMetaSchemaSizing builds an in-memory MCP server with the full meta-tool
// catalog and compares the generated action parameter schemas across all
// supported META_PARAM_SCHEMA modes (opaque/full/compact), printing a sizing
// table to stdout. Formerly the standalone audit_meta_schema binary.
func runMetaSchemaSizing() error {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer cleanup()

	server := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "0"}, &mcp.ServerOptions{PageSize: 2000})
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true})
	if err != nil {
		return fmt.Errorf("build meta action catalog: %w", err)
	}
	routes := catalog.ActionMaps()
	tools.RegisterMetaCatalog(server, catalog)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, cerr := server.Connect(ctx, st, nil)
	if cerr != nil {
		return fmt.Errorf("server connect: %w", cerr)
	}
	defer serverSession.Close()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "spike-cli", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	currentByName := map[string]map[string]any{}
	for _, t := range listed.Tools {
		if t.InputSchema == nil {
			continue
		}
		raw, mErr := json.Marshal(t.InputSchema)
		if mErr != nil {
			return fmt.Errorf("marshal input schema for %s: %w", t.Name, mErr)
		}
		var m map[string]any
		if uErr := json.Unmarshal(raw, &m); uErr != nil {
			return fmt.Errorf("unmarshal input schema for %s: %w", t.Name, uErr)
		}
		currentByName[t.Name] = m
	}

	type row struct {
		name      string
		actions   int
		opaque    int
		full      int
		compact   int
		fullDelta int
	}
	rows := []row{}

	names := make([]string, 0, len(routes))
	for n := range routes {
		names = append(names, n)
	}
	sort.Strings(names)

	totalOpaque, totalFull, totalCompact := 0, 0, 0

	for _, name := range names {
		rmap := routes[name]
		opaque := currentByName[name]
		opaqueJSON, mErr := json.Marshal(opaque)
		if mErr != nil {
			return fmt.Errorf("marshal opaque schema for %s: %w", name, mErr)
		}

		full := toolutil.BuildMetaToolSchema(rmap, toolutil.MetaParamSchemaFull)
		compact := toolutil.BuildMetaToolSchema(rmap, toolutil.MetaParamSchemaCompact)

		fullJSON, mErr := json.Marshal(full)
		if mErr != nil {
			return fmt.Errorf("marshal full schema for %s: %w", name, mErr)
		}
		compactJSON, mErr := json.Marshal(compact)
		if mErr != nil {
			return fmt.Errorf("marshal compact schema for %s: %w", name, mErr)
		}

		r := row{
			name:      name,
			actions:   len(rmap),
			opaque:    len(opaqueJSON),
			full:      len(fullJSON),
			compact:   len(compactJSON),
			fullDelta: len(fullJSON) - len(opaqueJSON),
		}
		rows = append(rows, r)
		totalOpaque += r.opaque
		totalFull += r.full
		totalCompact += r.compact
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].full > rows[j].full })

	fmt.Println("============================================================")
	fmt.Println(" Meta-tool InputSchema sizing spike")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("%-46s %7s %10s %10s %10s %10s\n",
		"meta-tool", "actions", "opaque", "full", "compact", "Δ full")
	fmt.Println(strings.Repeat("-", 96))
	for _, r := range rows {
		fmt.Printf("%-46s %7d %10s %10s %10s %+10s\n",
			r.name, r.actions,
			humanBytes(r.opaque), humanBytes(r.full), humanBytes(r.compact),
			humanBytes(r.fullDelta))
	}
	fmt.Println(strings.Repeat("-", 96))
	fmt.Printf("%-46s %7s %10s %10s %10s\n",
		"TOTAL", "",
		humanBytes(totalOpaque), humanBytes(totalFull), humanBytes(totalCompact))
	fmt.Println()
	fmt.Printf("Full / opaque   ratio: %.1fx\n", float64(totalFull)/float64(totalOpaque))
	fmt.Printf("Compact / opaque ratio: %.1fx\n", float64(totalCompact)/float64(totalOpaque))
	return nil
}

// humanBytes formats a byte count using compact B, KB, or MB units for the
// schema sizing table.
func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ─── Token footprint mode (-footprint) ────────────────────────────────────────
//
// The footprint mode measures every tier \u00d7 surface \u00d7 META_PARAM_SCHEMA
// combination and writes the README managed section plus the standalone
// docs/development/token-footprint.md reference. It was previously the
// standalone cmd/gen_readme binary.

// Footprint section markers and output paths.
const (
	footprintStartMarker  = "<!-- START TOKEN FOOTPRINT -->"
	footprintEndMarker    = "<!-- END TOKEN FOOTPRINT -->"
	detailedFootprintPath = "docs/development/token-footprint.md"
	readmePath            = "README.md"
	// siteFootprintPath receives the headline figures the documentation site
	// quotes. The site used to carry the startup-context reduction only as text
	// baked into a social-card image, with no measured figure anywhere in the
	// prose. Emitting it from the same measurement run that produces the README
	// and the reference doc means the published claim cannot drift from what the
	// tokenizer actually measured.
	siteFootprintPath = "site/src/data/token-footprint.json"
)

// Row identifiers used to pick the headline configurations out of the full
// footprint matrix.
const (
	dynamicDefaultConfiguration = "`dynamic` / `full` (default)"
	dynamicMinimalConfiguration = "`dynamic` / `minimal`"
	individualConfiguration     = "`individual` / `full`"
	ultimateTierLabel           = "Ultimate"
)

// tokenFootprintRow is a README-facing token measurement for one runtime
// configuration.
//
// Configuration is the Markdown label rendered in the leftmost column.
// MetaParamSchema is set for meta-tool configurations and empty ("n/a") for
// dynamic and individual. VisibleTools is the number of MCP tools the server
// exposes under this configuration. ReachableActions is the count of distinct
// GitLab API actions a client can drive (matches VisibleTools for individual
// mode). ToolSchemaTokens estimates the byte cost of publishing the visible
// tool schemas. SharedTokens captures the resources-plus-prompts overhead for
// this configuration.
type tokenFootprintRow struct {
	Tier             string
	Configuration    string
	MetaParamSchema  string
	VisibleTools     int
	ReachableActions int
	ToolSchemaTokens int
	SharedTokens     int
}

// sharedTokenMeasureOptions bundles the catalog metadata and capability
// surface needed to estimate the shared (resources + prompts) token cost of
// one runtime configuration.
//
// Routes is the catalog route map (used by the meta surface). ToolCatalog and
// ToolList drive the tool-manifest templates. ToolSurface selects the MCP
// surface label; CapabilitySurface toggles full vs minimal prompts and
// workflow guides; PromptTokens is the pre-computed prompt catalog size.
type sharedTokenMeasureOptions struct {
	Routes            map[string]toolutil.ActionMap
	ToolCatalog       *actioncatalog.Catalog
	ToolList          []*mcp.Tool
	ToolSurface       string
	CapabilitySurface string
	PromptTokens      int
}

func (r tokenFootprintRow) totalTokens() int {
	return r.ToolSchemaTokens + r.SharedTokens
}

// runFootprintMode builds the mock-backed client and runs the full token
// footprint measurement, mirroring the self-contained client pattern of
// [runMetaSchemaSizing].
func runFootprintMode(check bool) error {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer cleanup()
	if check {
		return runFootprintCheck(client)
	}
	return runFootprint(client)
}

// runFootprintCheck measures the token footprint and verifies that the README
// managed section and the detailed reference doc already match the freshly
// rendered content, without writing anything. It returns an error naming the
// stale targets when either is out of date — the CI counterpart to runFootprint
// and the gen_stats -check gate. This restores the README-token-footprint half
// of the former gen_readme -check (the stats half lives in gen_stats -check).
func runFootprintCheck(client *gitlabclient.Client) error {
	rows, err := measureTokenFootprintRows(client)
	if err != nil {
		return fmt.Errorf("measuring token footprint: %w", err)
	}

	readmeData, err := os.ReadFile(readmePath) //#nosec G304 -- README path is a compile-time constant, not user input
	if err != nil {
		return fmt.Errorf("reading %s: %w", readmePath, err)
	}
	detailedData, err := os.ReadFile(filepath.Clean(detailedFootprintPath)) //#nosec G304 -- generated doc path is a compile-time constant
	if err != nil {
		return fmt.Errorf("reading %s: %w", detailedFootprintPath, err)
	}
	siteData, err := os.ReadFile(filepath.Clean(siteFootprintPath)) //#nosec G304 -- generated data path is a compile-time constant
	if err != nil {
		return fmt.Errorf("reading %s: %w", siteFootprintPath, err)
	}

	stale, err := footprintStaleTargets(string(readmeData), string(detailedData), string(siteData), rows)
	if err != nil {
		return err
	}
	if len(stale) > 0 {
		return fmt.Errorf("token footprint is stale (%s); run: go run ./cmd/audit_tokens/ -footprint", strings.Join(stale, "; "))
	}
	fmt.Printf("Token footprint is current (%d rows across all tiers/surfaces/modes)\n", len(rows))
	return nil
}

// footprintStaleTargets compares the live README text and detailed-doc text
// against the freshly rendered footprint content and returns the human-readable
// names of any targets that are out of date. An empty slice means both are
// current. Kept pure (no I/O) so the drift logic is unit-testable.
func footprintStaleTargets(readmeText, detailedText, siteText string, rows []tokenFootprintRow) ([]string, error) {
	var stale []string

	updated, err := docgen.ComputeReplacedSection(readmeText, footprintStartMarker, footprintEndMarker, renderReadmeFootprint(rows))
	if err != nil {
		return nil, err
	}
	if updated != readmeText {
		stale = append(stale, readmePath+" token-footprint section")
	}
	if detailedText != renderDetailedFootprint(rows) {
		stale = append(stale, detailedFootprintPath)
	}
	siteDoc, err := renderSiteFootprintJSON(rows)
	if err != nil {
		return nil, err
	}
	if siteText != string(siteDoc) {
		stale = append(stale, siteFootprintPath)
	}
	return stale, nil
}

// runFootprint measures the full tier \u00d7 surface \u00d7 mode matrix and writes the
// README managed section plus the detailed reference doc.
func runFootprint(client *gitlabclient.Client) error {
	rows, err := measureTokenFootprintRows(client)
	if err != nil {
		return fmt.Errorf("measuring token footprint: %w", err)
	}
	if replaceErr := docgen.ReplaceSection(readmePath, footprintStartMarker, footprintEndMarker, renderReadmeFootprint(rows)); replaceErr != nil {
		return replaceErr
	}
	detailedDoc := renderDetailedFootprint(rows)
	if writeErr := os.WriteFile(filepath.Clean(detailedFootprintPath), []byte(detailedDoc), 0o600); writeErr != nil { //#nosec G306,G703 -- generated doc path is a compile-time constant
		return fmt.Errorf("writing %s: %w", detailedFootprintPath, writeErr)
	}
	siteDoc, err := renderSiteFootprintJSON(rows)
	if err != nil {
		return err
	}
	if writeErr := os.WriteFile(filepath.Clean(siteFootprintPath), siteDoc, 0o600); writeErr != nil { //#nosec G306,G703 -- generated data path is a compile-time constant
		return fmt.Errorf("writing %s: %w", siteFootprintPath, writeErr)
	}
	fmt.Printf("Updated %s token-footprint section, %s and %s (%d rows across all tiers/surfaces/modes)\n", readmePath, detailedFootprintPath, siteFootprintPath, len(rows))
	return nil
}

// siteFootprint is the headline subset of the footprint matrix consumed by the
// documentation site (site/src/data/token-footprint.json).
type siteFootprint struct {
	Tokenizer  string                             `json:"tokenizer"`
	Shared     siteFootprintShared                `json:"shared"`
	Dynamic    siteFootprintSurface               `json:"dynamic"`
	Individual map[string]siteFootprintIndividual `json:"individual"`
}

// siteFootprintShared is the resource + prompt cost that every surface pays on
// top of its tool schemas, per capability surface. It is identical across
// tiers and tool surfaces, which is why the site quotes it as a single figure
// rather than a per-row column.
type siteFootprintShared struct {
	Full    int `json:"full"`
	Minimal int `json:"minimal"`
}

// siteFootprintSurface is one measured surface: how many tool definitions the
// client receives at startup and what they cost.
type siteFootprintSurface struct {
	VisibleTools     int `json:"visible_tools"`
	ToolSchemaTokens int `json:"tool_schema_tokens"`
}

// siteFootprintIndividual adds the reduction factor against the dynamic surface, so the
// site never has to compute (and risk mis-stating) the ratio itself.
type siteFootprintIndividual struct {
	VisibleTools     int `json:"visible_tools"`
	ToolSchemaTokens int `json:"tool_schema_tokens"`
	ReductionFactor  int `json:"reduction_factor_vs_dynamic"`
}

// renderSiteFootprintJSON extracts the dynamic and individual headline rows and
// marshals them to prettier-compatible JSON (2-space indent, trailing newline).
func renderSiteFootprintJSON(rows []tokenFootprintRow) ([]byte, error) {
	tierKeys := map[string]string{"Free/CE": "free", "Premium": "premium", ultimateTierLabel: "ultimate"}

	var dynamic siteFootprintSurface
	var shared siteFootprintShared
	for _, r := range rows {
		if r.Tier != ultimateTierLabel {
			continue
		}
		switch r.Configuration {
		case dynamicDefaultConfiguration:
			dynamic = siteFootprintSurface{VisibleTools: r.VisibleTools, ToolSchemaTokens: r.ToolSchemaTokens}
			shared.Full = r.SharedTokens
		case dynamicMinimalConfiguration:
			shared.Minimal = r.SharedTokens
		}
	}
	if dynamic.ToolSchemaTokens == 0 {
		return nil, fmt.Errorf("no %s row found for the %s tier", dynamicDefaultConfiguration, ultimateTierLabel)
	}
	if shared.Full == 0 || shared.Minimal == 0 {
		return nil, fmt.Errorf("missing shared token counts for the %s tier (full=%d, minimal=%d)",
			ultimateTierLabel, shared.Full, shared.Minimal)
	}

	individual := make(map[string]siteFootprintIndividual, len(tierKeys))
	for _, r := range rows {
		key, ok := tierKeys[r.Tier]
		if !ok || r.Configuration != individualConfiguration {
			continue
		}
		individual[key] = siteFootprintIndividual{
			VisibleTools:     r.VisibleTools,
			ToolSchemaTokens: r.ToolSchemaTokens,
			ReductionFactor:  int(math.Round(float64(r.ToolSchemaTokens) / float64(dynamic.ToolSchemaTokens))),
		}
	}
	if len(individual) != len(tierKeys) {
		return nil, fmt.Errorf("expected %d individual-surface rows, found %d", len(tierKeys), len(individual))
	}

	raw, err := json.MarshalIndent(siteFootprint{
		Tokenizer:  "cl100k_base",
		Shared:     shared,
		Dynamic:    dynamic,
		Individual: individual,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal site footprint: %w", err)
	}
	return append(raw, '\n'), nil
}

func measureTokenFootprintRows(client *gitlabclient.Client) ([]tokenFootprintRow, error) {
	var allRows []tokenFootprintRow
	tiers := []struct {
		label string
		tier  edition.Tier
	}{
		{"Free/CE", edition.Free},
		{"Premium", edition.Premium},
		{"Ultimate", edition.Ultimate},
	}

	for i, t := range tiers {
		cmdutil.Progressf("audit_tokens: measuring footprint [%d/%d] %s tier (all surfaces × schema modes)…", i+1, len(tiers), t.label)
		rows, err := measureTierFootprint(client, t.tier, t.label)
		if err != nil {
			return nil, err
		}
		allRows = append(allRows, rows...)
	}
	return allRows, nil
}

func measureTierFootprint(client *gitlabclient.Client, tier edition.Tier, tierLabel string) ([]tokenFootprintRow, error) {
	metaCatalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       tier,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build %s action catalog: %w", tierLabel, err)
	}

	dynamicCatalog, err := dynamictools.AddStandaloneCatalog(metaCatalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add standalone dynamic catalog: %w", err)
	}
	metaRoutes := metaCatalog.ActionMaps()
	dynamicRoutes := dynamicCatalog.ActionMaps()
	reachableActions := countActions(dynamicRoutes)

	dynamicTools, err := fpListDynamicTools(dynamicCatalog)
	if err != nil {
		return nil, err
	}
	individualTools, err := listIndividualToolsAtTier(client, tier)
	if err != nil {
		return nil, err
	}

	dynamicToolTokens, err := measureToolSchemaTokens(dynamicTools)
	if err != nil {
		return nil, err
	}
	individualToolTokens, err := measureToolSchemaTokens(individualTools)
	if err != nil {
		return nil, err
	}
	promptTokens, err := fpMeasurePrompts(client)
	if err != nil {
		return nil, err
	}

	dynamicFullShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: dynamicRoutes, ToolCatalog: dynamicCatalog, ToolList: dynamicTools,
		ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: config.CapabilitySurfaceFull, PromptTokens: promptTokens,
	})
	if err != nil {
		return nil, err
	}
	dynamicMinimalShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: dynamicRoutes, ToolCatalog: dynamicCatalog, ToolList: dynamicTools,
		ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: config.CapabilitySurfaceMinimal, PromptTokens: promptTokens,
	})
	if err != nil {
		return nil, err
	}
	metaFullShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: metaRoutes, ToolCatalog: metaCatalog, ToolList: nil,
		ToolSurface: config.ToolSurfaceMeta, CapabilitySurface: config.CapabilitySurfaceFull, PromptTokens: promptTokens,
	})
	if err != nil {
		return nil, err
	}
	metaMinimalShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: metaRoutes, ToolCatalog: metaCatalog, ToolList: nil,
		ToolSurface: config.ToolSurfaceMeta, CapabilitySurface: config.CapabilitySurfaceMinimal, PromptTokens: promptTokens,
	})
	if err != nil {
		return nil, err
	}
	individualFullShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		ToolList:    individualTools,
		ToolSurface: config.ToolSurfaceIndividual, CapabilitySurface: config.CapabilitySurfaceFull, PromptTokens: promptTokens,
	})
	if err != nil {
		return nil, err
	}

	rows := []tokenFootprintRow{
		{Tier: tierLabel, Configuration: dynamicDefaultConfiguration, VisibleTools: len(dynamicTools), ReachableActions: reachableActions, ToolSchemaTokens: dynamicToolTokens, SharedTokens: dynamicFullShared},
		{Tier: tierLabel, Configuration: dynamicMinimalConfiguration, VisibleTools: len(dynamicTools), ReachableActions: reachableActions, ToolSchemaTokens: dynamicToolTokens, SharedTokens: dynamicMinimalShared},
	}

	metaSchemaModes := []string{"opaque", "compact", "full"}
	for _, mode := range metaSchemaModes {
		restore := tools.SetMetaParamSchemaScoped(mode)
		metaTools, metaErr := listMetaToolsFromCatalog(client, metaCatalog)
		if metaErr != nil {
			restore()
			return nil, metaErr
		}
		metaTokens, metaMeasureErr := measureToolSchemaTokens(metaTools)
		restore()
		if metaMeasureErr != nil {
			return nil, metaMeasureErr
		}
		rows = append(
			rows,
			tokenFootprintRow{Tier: tierLabel, Configuration: fmt.Sprintf("`meta` / `full` (%s)", mode), MetaParamSchema: mode, VisibleTools: len(metaTools), ReachableActions: reachableActions, ToolSchemaTokens: metaTokens, SharedTokens: metaFullShared},
			tokenFootprintRow{Tier: tierLabel, Configuration: fmt.Sprintf("`meta` / `minimal` (%s)", mode), MetaParamSchema: mode, VisibleTools: len(metaTools), ReachableActions: reachableActions, ToolSchemaTokens: metaTokens, SharedTokens: metaMinimalShared},
		)
	}

	rows = append(rows, tokenFootprintRow{Tier: tierLabel, Configuration: "`individual` / `full`", VisibleTools: len(individualTools), ReachableActions: len(individualTools), ToolSchemaTokens: individualToolTokens, SharedTokens: individualFullShared})
	return rows, nil
}

// renderReadmeFootprint renders the README managed section: only the default
// surface (dynamic) across all tiers, plus a link to the detailed doc.
func renderReadmeFootprint(rows []tokenFootprintRow) string {
	var b strings.Builder
	b.WriteString("Measured with `go run ./cmd/audit_tokens/ -footprint` against the current catalog. Totals estimate startup context visible to an MCP client: visible tool schemas plus shared resources and prompts, using the cl100k_base tokenizer (GPT-4/GPT-3.5 encoding). For the full matrix (meta and individual surfaces, all `META_PARAM_SCHEMA` modes), see [Token Footprint Reference](docs/development/token-footprint.md).\n\n")
	b.WriteString("**Default configuration**: with `TOOL_SURFACE` unset or `TOOL_SURFACE=dynamic`, `CAPABILITY_SURFACE=full`, `META_TOOLS` unset, `META_PARAM_SCHEMA=opaque`, and `GITLAB_TIER` unset (detected, fallback `free`), the server uses the **dynamic find/execute surface**. Use `TOOL_SURFACE=meta` only when you explicitly want domain meta-tools; use `TOOL_SURFACE=individual` only when your client can handle the full tool catalog.\n\n")

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		if !strings.Contains(row.Configuration, "dynamic") {
			continue
		}
		tierCell := row.Tier
		schemaCell := "n/a"
		if row.MetaParamSchema != "" {
			schemaCell = fmt.Sprintf("`%s`", row.MetaParamSchema)
		}
		tableRows = append(tableRows, []string{
			row.Configuration, tierCell,
			fmtNum(row.VisibleTools), fmtNum(row.ReachableActions),
			schemaCell, fmtNum(row.ToolSchemaTokens),
			fmtNum(row.SharedTokens), fmtNum(row.totalTokens()),
		})
	}
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Configuration (`TOOL_SURFACE` / `CAPABILITY_SURFACE`)", "Tier", "Visible tools", "Reachable actions", "`META_PARAM_SCHEMA`", "Tool schema tokens", "Shared tokens", "Total tokens"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight},
		tableRows,
	))
	b.WriteString("\nRows use the base Community Edition catalog unless the Tier column says otherwise. `GITLAB_TIER` controls which actions are available; higher tiers expose more tools and thus more reachable actions.\n")
	return b.String()
}

// renderDetailedFootprint renders the full token matrix to a standalone doc
// with explanatory prose. All surfaces \u00d7 all META_PARAM_SCHEMA modes \u00d7 all tiers.
func renderDetailedFootprint(rows []tokenFootprintRow) string {
	var b strings.Builder
	b.WriteString("# Token Footprint Reference\n\n")
	b.WriteString("> **Di\u00e1taxis type**: Reference\n")
	b.WriteString("> **Audience**: Developers integrating with the MCP server\n")
	b.WriteString("> **Generated by**: `go run ./cmd/audit_tokens/ -footprint`\n\n")
	b.WriteString("---\n\n")

	b.WriteString("## How tokens are counted\n\n")
	b.WriteString("Token counts use the **cl100k_base** tokenizer (the GPT-4 / GPT-3.5 encoding) via [`github.com/tiktoken-go/tokenizer`](https://github.com/tiktoken-go/tokenizer) \u2014 a pure Go port of OpenAI's tiktoken. The vocabulary is embedded at compile time (~4 MB). This is significantly more accurate than the `bytes \u00f7 4` heuristic for JSON-dense content like MCP tool schemas, which contain many braces, identifiers, and nested objects.\n\n")

	b.WriteString("## What each column means\n\n")
	// Rendered through docgen (not hand-written) so the emitted table is already
	// format_md_tables-canonical; this keeps -footprint output idempotent under
	// `make update-all` and lets -footprint -check compare without false drift.
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Column", "Description"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft},
		[][]string{
			{"**Configuration**", "The `TOOL_SURFACE` / `CAPABILITY_SURFACE` combination. `dynamic` = find/execute (default); `meta` = domain-grouped dispatchers; `individual` = one tool per action."},
			{"**Tier**", "The GitLab licensing tier: Free/CE, Premium, or Ultimate. Higher tiers expose more actions."},
			{"**Visible tools**", "How many MCP tool definitions the client receives at startup."},
			{"**Reachable actions**", "How many distinct GitLab API actions the client can drive (may exceed visible tools in meta/dynamic mode via action routing)."},
			{"**META_PARAM_SCHEMA**", "How meta-tool input schemas are generated: `opaque` (action enum + params:any, default), `compact` (property names + types only), `full` (full per-action schema with descriptions)."},
			{"**Tool schema tokens**", "Token cost of the visible tool definitions (InputSchema, annotations, description)."},
			{"**Shared tokens**", "Token cost of MCP resources (`gitlab://tools`, templates, workflow guides) and prompt templates. `full` = all resources; `minimal` = only `gitlab://tools` for on-demand schema browsing."},
			{"**Total tokens**", "Tool schema tokens + shared tokens. This is the approximate startup context-window cost."},
		},
	))
	b.WriteString("\n")

	b.WriteString("## Full matrix\n\n")
	b.WriteString("All measurements are against the current source tree. The catalog is built in-memory with a mock GitLab client; no network calls are made.\n\n")

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		schemaCell := "n/a"
		if row.MetaParamSchema != "" {
			schemaCell = fmt.Sprintf("`%s`", row.MetaParamSchema)
		}
		tableRows = append(tableRows, []string{
			row.Configuration, row.Tier,
			fmtNum(row.VisibleTools), fmtNum(row.ReachableActions),
			schemaCell, fmtNum(row.ToolSchemaTokens),
			fmtNum(row.SharedTokens), fmtNum(row.totalTokens()),
		})
	}
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Configuration", "Tier", "Visible tools", "Reachable actions", "`META_PARAM_SCHEMA`", "Tool schema tokens", "Shared tokens", "Total tokens"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight},
		tableRows,
	))

	b.WriteString("\n## Interpretation guide\n\n")
	b.WriteString("- **Dynamic mode** (default) exposes only 2 tools (`gitlab_find_action` + `gitlab_execute_action`) but reaches all catalog actions via routing. This is the lowest-token surface.\n")
	b.WriteString("- **Meta mode** exposes one dispatcher per domain (e.g. `gitlab_branch`, `gitlab_issue`). The `META_PARAM_SCHEMA` controls whether the action parameter's schema is generic (`opaque`) or detailed (`compact`/`full`). `full` doubles the token cost vs `opaque` but gives the LLM exact per-action input shapes.\n")
	b.WriteString("- **Individual mode** exposes every action as its own tool. This is the highest-fidelity but most expensive surface \u2014 suitable only for clients with large context windows.\n")
	b.WriteString("- **Tier scaling**: Free/CE has the fewest actions. Premium adds enterprise features. Ultimate includes everything. The token cost scales with the number of available actions.\n")
	b.WriteString("- **Shared tokens** are dominated by MCP resources (`gitlab://tools` template, workflow guides) and prompts. The `minimal` capability surface strips these to just `gitlab://tools`, cutting shared overhead by ~90%%.\n")
	return b.String()
}

func listMetaToolsFromCatalog(client *gitlabclient.Client, catalog *actioncatalog.Catalog) ([]*mcp.Tool, error) {
	server := newFootprintServer()
	tools.RegisterMetaCatalog(server, catalog)
	tools.RegisterMetaStandaloneTools(server, client)
	return fpListToolsFromServer(server)
}

func listIndividualToolsAtTier(client *gitlabclient.Client, tier edition.Tier) ([]*mcp.Tool, error) {
	server := newFootprintServer()
	tools.RegisterAll(server, client, tier)
	return fpListToolsFromServer(server)
}

// fpListDynamicTools is the footprint variant of [listDynamicTools]: it returns
// an error instead of calling os.Exit so the footprint runner can surface
// measurement failures cleanly.
func fpListDynamicTools(catalog *actioncatalog.Catalog) ([]*mcp.Tool, error) {
	server := newFootprintServer()
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return fpListToolsFromServer(server)
}

func newFootprintServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{PageSize: 2000})
}

// fpListToolsFromServer is the footprint variant of [listToolsFromServer]: it
// returns an error instead of calling os.Exit.
func fpListToolsFromServer(server *mcp.Server) ([]*mcp.Tool, error) {
	return withFootprintSession(server, "tools", func(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
		result, err := session.ListTools(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		return result.Tools, nil
	})
}

func measureToolSchemaTokens(toolList []*mcp.Tool) (int, error) {
	totalTokens := 0
	for _, t := range toolList {
		b, err := json.Marshal(t)
		if err != nil {
			return 0, fmt.Errorf("marshal tool %s: %w", t.Name, err)
		}
		totalTokens += countTokens(b)
	}
	return totalTokens, nil
}

func measureSharedTokens(client *gitlabclient.Client, opts sharedTokenMeasureOptions) (int, error) {
	resourceTokens, err := fpMeasureResourcesWithOptions(client, opts.Routes, resourceRegistrationOptions{
		Core:           opts.CapabilitySurface == config.CapabilitySurfaceFull,
		ToolManifest:   true,
		ToolSurface:    opts.ToolSurface,
		ToolList:       opts.ToolList,
		ToolCatalog:    opts.ToolCatalog,
		WorkflowGuides: opts.CapabilitySurface == config.CapabilitySurfaceFull,
	})
	if err != nil {
		return 0, err
	}
	if opts.CapabilitySurface == config.CapabilitySurfaceFull {
		return resourceTokens + opts.PromptTokens, nil
	}
	return resourceTokens, nil
}

// fpMeasureResourcesWithOptions is the footprint variant of
// [measureResourcesWithOptions]: it returns an error instead of calling
// os.Exit so measurement failures propagate to the footprint runner.
func fpMeasureResourcesWithOptions(client *gitlabclient.Client, routes map[string]toolutil.ActionMap, opts resourceRegistrationOptions) (int, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	if opts.Core {
		resources.Register(server, client)
	}
	if opts.ToolManifest {
		resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
			Surface:    opts.ToolSurface,
			Tools:      opts.ToolList,
			Catalog:    opts.ToolCatalog,
			MetaRoutes: routes,
		})
	}
	if opts.WorkflowGuides {
		resources.RegisterWorkflowGuides(server)
	}

	return withFootprintSession(server, "resources", func(ctx context.Context, session *mcp.ClientSession) (int, error) {
		totalTokens := 0
		res, err := session.ListResources(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("list resources: %w", err)
		}
		for _, r := range res.Resources {
			b, mErr := json.Marshal(r)
			if mErr != nil {
				return 0, fmt.Errorf("marshal resource %s: %w", r.Name, mErr)
			}
			totalTokens += countTokens(b)
		}

		tpl, err := session.ListResourceTemplates(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("list resource templates: %w", err)
		}
		for _, t := range tpl.ResourceTemplates {
			b, mErr := json.Marshal(t)
			if mErr != nil {
				return 0, fmt.Errorf("marshal template %s: %w", t.Name, mErr)
			}
			totalTokens += countTokens(b)
		}
		return totalTokens, nil
	})
}

// fpMeasurePrompts is the footprint variant of [measurePrompts]: it returns an
// error instead of calling os.Exit.
func fpMeasurePrompts(client *gitlabclient.Client) (int, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	prompts.Register(server, client)

	return withFootprintSession(server, "prompts", func(ctx context.Context, session *mcp.ClientSession) (int, error) {
		totalTokens := 0
		promptList, err := session.ListPrompts(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("list prompts: %w", err)
		}
		for _, pr := range promptList.Prompts {
			b, mErr := json.Marshal(pr)
			if mErr != nil {
				return 0, fmt.Errorf("marshal prompt %s: %w", pr.Name, mErr)
			}
			totalTokens += countTokens(b)
		}
		return totalTokens, nil
	})
}

// withFootprintSession connects server in-memory, runs fn against a fresh
// client session, and returns fn's result. It is the error-returning analog
// of the inline connect-and-exit pattern used by the rest of the audit.
func withFootprintSession[T any](server *mcp.Server, label string, fn func(context.Context, *mcp.ClientSession) (T, error)) (T, error) {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	var zero T

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return zero, fmt.Errorf("server connect %s: %w", label, err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return zero, fmt.Errorf("client connect %s: %w", label, err)
	}
	defer session.Close()

	return fn(ctx, session)
}
