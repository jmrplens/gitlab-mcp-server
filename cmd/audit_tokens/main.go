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
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
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

// auditOptions is what the command line selects: which mode to run and how
// long the ranking tables are. Parsing it is separate from acting on it so
// the modes are testable.
type auditOptions struct {
	footprint      bool
	check          bool
	compareSchemas bool
	topTools       int
	topDomains     int
	jsonOut        bool
}

// main parses flags and hands the work to run, whose exit code it returns.
func main() {
	opts := auditOptions{}
	flag.BoolVar(&opts.footprint, "footprint", false, "measure all tiers \u00d7 surfaces \u00d7 META_PARAM_SCHEMA modes and write the README token-claim block and token-footprint section, docs/development/token-footprint.md and site/src/data/token-footprint.json")
	flag.BoolVar(&opts.check, "check", false, "with -footprint, verify the README token-claim block and token-footprint section, docs/development/token-footprint.md and site/src/data/token-footprint.json are current without writing (exits non-zero on drift)")
	flag.BoolVar(&opts.compareSchemas, "compare-schemas", false, "compare META_PARAM_SCHEMA modes (opaque/full/compact) for meta-tool InputSchema sizing instead of the normal token audit")
	flag.IntVar(&opts.topTools, "top-tools", 30, "number of individual tools to list by token cost")
	flag.IntVar(&opts.topDomains, "top-domains", 20, "number of domains to list by token cost")
	flag.BoolVar(&opts.jsonOut, "json", false, "emit JSON summary instead of markdown report")
	flag.Parse()

	os.Exit(run(opts, os.Stdout, os.Stderr))
}

// run creates the mock GitLab-backed client, measures all MCP catalog modes,
// and prints token overhead comparisons for tools, resources, and prompts.
//
// With -compare-schemas it instead runs the meta-tool InputSchema sizing spike
// (formerly the standalone audit_meta_schema binary), comparing the byte cost of
// each META_PARAM_SCHEMA mode (opaque/full/compact); with -footprint it writes
// or verifies the tier x surface x mode matrix.
//
// It is the one place a failure is reported. The failures that reach it are
// the ones a caller can act on: the generated footprint targets are missing,
// unreadable or stale, or stdout refuses the JSON summary. The measurement
// itself cannot fail — it registers a catalog compiled into this binary onto
// an in-memory server — so it panics through [cmdutil.Must] rather than
// threading a return path through every frame between here and the failure.
func run(opts auditOptions, stdout, stderr io.Writer) int {
	if opts.footprint {
		if err := runFootprintMode(opts.check); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if opts.compareSchemas {
		runMetaSchemaSizing()
		return 0
	}

	client, cleanup := auditclient.NewMock()
	defer cleanup()

	audit := measureTokenAudit(client)
	if opts.jsonOut {
		// A failed encode keeps the status it has always had: the message is
		// reported and the exit code stays 0. Changing that is a behavior
		// change, which this refactor deliberately does not make.
		if encErr := writeTokenAuditJSON(stdout, audit); encErr != nil {
			fmt.Fprintf(stderr, "encode json: %v\n", encErr)
		}
		return 0
	}
	printTokenAuditReport(audit, opts.topTools, opts.topDomains)
	return 0
}

// tokenAudit holds every measurement the default report and its -json form
// print, gathered once by [measureTokenAudit] so both renderers read the same
// numbers.
//
// The info slices are the per-tool measurements of each surface, sorted by
// token cost. The resource token fields are the shared resource cost under
// each surface's registration (the dynamic-minimal one keeps only the tool
// manifest), promptTokens the prompt catalog cost. The action counts
// distinguish catalog-only meta routes from the reachable set the dynamic
// catalog folds standalone utilities into.
type tokenAudit struct {
	individualInfo        []toolTokenInfo
	metaBaseInfo          []toolTokenInfo
	metaEnterpriseInfo    []toolTokenInfo
	dynamicBaseInfo       []toolTokenInfo
	dynamicEnterpriseInfo []toolTokenInfo

	individualResourceTokens     int
	metaBaseResourceTokens       int
	dynamicBaseResourceTokens    int
	dynamicMinimalResourceTokens int
	promptTokens                 int

	metaBaseCatalogActions       int
	metaEnterpriseCatalogActions int
	baseReachableActions         int
	enterpriseReachableActions   int
}

// auditSurfaces is the registered material every measurement pass reads: the
// route catalogs behind meta and dynamic mode, and the tool definitions each
// surface publishes.
type auditSurfaces struct {
	metaBaseRoutes           map[string]toolutil.ActionMap
	metaEnterpriseRoutes     map[string]toolutil.ActionMap
	dynamicBaseCatalog       *actioncatalog.Catalog
	dynamicEnterpriseCatalog *actioncatalog.Catalog
	dynamicBaseRoutes        map[string]toolutil.ActionMap
	dynamicEnterpriseRoutes  map[string]toolutil.ActionMap

	individualTools        []*mcp.Tool
	metaBaseTools          []*mcp.Tool
	metaEnterpriseTools    []*mcp.Tool
	dynamicBaseTools       []*mcp.Tool
	dynamicEnterpriseTools []*mcp.Tool
}

// buildAuditSurfaces builds the base and enterprise action catalogs and
// enumerates the tools the individual, meta and dynamic surfaces advertise
// over them.
func buildAuditSurfaces(client *gitlabclient.Client) auditSurfaces {
	var s auditSurfaces

	s.metaBaseRoutes = buildMetaActionMaps(client, false)
	s.metaEnterpriseRoutes = buildMetaActionMaps(client, true)
	s.dynamicBaseCatalog = cmdutil.Must(dynamictools.AddStandaloneCatalog(actioncatalog.FromActionMaps(s.metaBaseRoutes), client, dynamictools.StandaloneOptions{}))
	s.dynamicEnterpriseCatalog = cmdutil.Must(dynamictools.AddStandaloneCatalog(actioncatalog.FromActionMaps(s.metaEnterpriseRoutes), client, dynamictools.StandaloneOptions{}))
	s.dynamicBaseRoutes = s.dynamicBaseCatalog.ActionMaps()
	s.dynamicEnterpriseRoutes = s.dynamicEnterpriseCatalog.ActionMaps()

	cmdutil.Progressf("audit_tokens: enumerating tools across individual/meta/dynamic surfaces...")
	s.individualTools = listTools(client, config.ToolSurfaceIndividual, true)
	s.metaBaseTools = listTools(client, config.ToolSurfaceMeta, false)
	s.metaEnterpriseTools = listTools(client, config.ToolSurfaceMeta, true)
	s.dynamicBaseTools = listDynamicTools(s.dynamicBaseCatalog)
	s.dynamicEnterpriseTools = listDynamicTools(s.dynamicEnterpriseCatalog)
	return s
}

// measureTokenAudit registers the individual, meta and dynamic surfaces with
// their resources and prompts and measures what each costs a model.
func measureTokenAudit(client *gitlabclient.Client) tokenAudit {
	s := buildAuditSurfaces(client)

	cmdutil.Progressf("audit_tokens: measuring token cost (tools, resources, prompts)...")
	audit := tokenAudit{
		metaBaseCatalogActions:       countActions(s.metaBaseRoutes),
		metaEnterpriseCatalogActions: countActions(s.metaEnterpriseRoutes),
		baseReachableActions:         countActions(s.dynamicBaseRoutes),
		enterpriseReachableActions:   countActions(s.dynamicEnterpriseRoutes),

		individualInfo:        measureTools(s.individualTools),
		metaBaseInfo:          measureTools(s.metaBaseTools),
		metaEnterpriseInfo:    measureTools(s.metaEnterpriseTools),
		dynamicBaseInfo:       measureTools(s.dynamicBaseTools),
		dynamicEnterpriseInfo: measureTools(s.dynamicEnterpriseTools),

		individualResourceTokens:  measureResources(client, nil, nil, s.individualTools, config.ToolSurfaceIndividual),
		metaBaseResourceTokens:    measureResources(client, s.metaBaseRoutes, actioncatalog.FromActionMaps(s.metaBaseRoutes), s.metaBaseTools, config.ToolSurfaceMeta),
		dynamicBaseResourceTokens: measureResources(client, s.dynamicBaseRoutes, s.dynamicBaseCatalog, s.dynamicBaseTools, config.ToolSurfaceDynamic),
		dynamicMinimalResourceTokens: measureResourcesWithOptions(client, nil, resourceRegistrationOptions{
			ToolManifest: true,
			ToolSurface:  config.ToolSurfaceDynamic,
			ToolList:     s.dynamicBaseTools,
			ToolCatalog:  s.dynamicBaseCatalog,
		}),
		promptTokens: measurePrompts(client),
	}
	return audit
}

// writeTokenAuditJSON encodes the headline measurements as an indented JSON
// object, the -json form of the report.
func writeTokenAuditJSON(w io.Writer, audit tokenAudit) error {
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
		len(audit.individualInfo), len(audit.metaBaseInfo), len(audit.dynamicBaseInfo),
		totalTokens(audit.individualInfo), totalTokens(audit.metaBaseInfo), totalTokens(audit.metaEnterpriseInfo),
		totalTokens(audit.dynamicBaseInfo), totalTokens(audit.dynamicEnterpriseInfo),
		audit.baseReachableActions, audit.individualResourceTokens, audit.promptTokens,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

// printTokenAuditReport writes the Markdown token report to stdout: the
// per-surface comparison, the shared overhead, the most expensive tools and
// domains, and the grand totals a model sees at startup. topTools and
// topDomains cap the individual-tool and domain rankings.
func printTokenAuditReport(audit tokenAudit, topTools, topDomains int) {
	indTotal := totalTokens(audit.individualInfo)
	metaTotal := totalTokens(audit.metaBaseInfo)
	metaEntTotal := totalTokens(audit.metaEnterpriseInfo)
	dynamicTotal := totalTokens(audit.dynamicBaseInfo)
	dynamicEntTotal := totalTokens(audit.dynamicEnterpriseInfo)

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("  gitlab-mcp-server. Token Overhead Audit")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()

	fmt.Println("## Mode Comparison")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Mode\tTools\tReachable actions\tTokens\tBytes\n")
	fmt.Fprintf(tw, "  ----\t-----\t--------------\t------\t-----\n")
	fmt.Fprintf(tw, "  Individual (all)\t%d\t%d\t%s\t%s\n", len(audit.individualInfo), len(audit.individualInfo), fmtNum(indTotal), fmtNum(totalBytes(audit.individualInfo)))
	fmt.Fprintf(tw, "  Meta-tools (base)\t%d\t%d\t%s\t%s\n", len(audit.metaBaseInfo), audit.baseReachableActions, fmtNum(metaTotal), fmtNum(totalBytes(audit.metaBaseInfo)))
	fmt.Fprintf(tw, "  Meta-tools (enterprise)\t%d\t%d\t%s\t%s\n", len(audit.metaEnterpriseInfo), audit.enterpriseReachableActions, fmtNum(metaEntTotal), fmtNum(totalBytes(audit.metaEnterpriseInfo)))
	fmt.Fprintf(tw, "  Dynamic (base)\t%d\t%d\t%s\t%s\n", len(audit.dynamicBaseInfo), audit.baseReachableActions, fmtNum(dynamicTotal), fmtNum(totalBytes(audit.dynamicBaseInfo)))
	fmt.Fprintf(tw, "  Dynamic (enterprise)\t%d\t%d\t%s\t%s\n", len(audit.dynamicEnterpriseInfo), audit.enterpriseReachableActions, fmtNum(dynamicEntTotal), fmtNum(totalBytes(audit.dynamicEnterpriseInfo)))
	_ = tw.Flush()
	fmt.Println()
	if addedStandalone := audit.baseReachableActions - audit.metaBaseCatalogActions; addedStandalone > 0 {
		fmt.Printf("  Reachable action counts include %d standalone utility actions (project discovery + interactive flows) that are visible tools in meta mode and folded into the dynamic catalog.\n", addedStandalone)
		fmt.Printf("  Catalog-only meta route counts: base %s / enterprise %s.\n", fmtNum(audit.metaBaseCatalogActions), fmtNum(audit.metaEnterpriseCatalogActions))
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
	fmt.Printf("  Resources (individual): ~%s tokens\n", fmtNum(audit.individualResourceTokens))
	fmt.Printf("  Resources (meta-tools): ~%s tokens\n", fmtNum(audit.metaBaseResourceTokens))
	fmt.Printf("  Resources (dynamic): ~%s tokens\n", fmtNum(audit.dynamicBaseResourceTokens))
	fmt.Printf("  Resources (dynamic-minimal): ~%s tokens\n", fmtNum(audit.dynamicMinimalResourceTokens))
	fmt.Printf("  Prompts (full): ~%s tokens\n", fmtNum(audit.promptTokens))
	fmt.Println("  Prompts (dynamic-minimal): ~0 tokens (0 bytes)")
	fmt.Printf("  Individual total: ~%s tokens\n", fmtNum(audit.individualResourceTokens+audit.promptTokens))
	fmt.Printf("  Meta-tool total:  ~%s tokens\n", fmtNum(audit.metaBaseResourceTokens+audit.promptTokens))
	fmt.Printf("  Dynamic total:    ~%s tokens\n", fmtNum(audit.dynamicBaseResourceTokens+audit.promptTokens))
	fmt.Printf("  Dynamic-minimal total: ~%s tokens\n", fmtNum(audit.dynamicMinimalResourceTokens))
	fmt.Println()

	fmt.Println("## Minimal Capability Candidate")
	fmt.Println()
	fmt.Println("  Required for dynamic action use: `gitlab_find_action` returns exact schemas inline, and `gitlab_execute_action` performs execution.")
	fmt.Println("  Retained minimal resource: `gitlab://tools` for action call shapes.")
	fmt.Println("  Optional in minimal mode: static GitLab data resources, workflow guide resources, and prompt templates.")
	if audit.dynamicBaseResourceTokens+audit.promptTokens > 0 {
		savings := float64(audit.dynamicBaseResourceTokens+audit.promptTokens-audit.dynamicMinimalResourceTokens) / float64(audit.dynamicBaseResourceTokens+audit.promptTokens) * 100
		fmt.Printf("  Shared-overhead reduction: %.1f%% vs full dynamic resources+prompts\n", savings)
	}
	fmt.Println()

	// Top 30 individual tools by token cost
	fmt.Println("## Top 30 Individual Tools by Token Cost")
	fmt.Println()
	printTopTools(audit.individualInfo, topTools)

	// Top 20 meta-tools by token cost
	fmt.Println("## Meta-Tools by Token Cost (base)")
	fmt.Println()
	printTopTools(audit.metaBaseInfo, len(audit.metaBaseInfo))

	// Dynamic tools by token cost
	fmt.Println("## Dynamic Tools by Token Cost (base)")
	fmt.Println()
	printTopTools(audit.dynamicBaseInfo, len(audit.dynamicBaseInfo))

	// Domain aggregation for individual tools
	fmt.Println("## Domain Totals (Individual Mode, Top 20)")
	fmt.Println()
	printDomainTotals(audit.individualInfo, topDomains)

	// Grand total
	fmt.Println("## Grand Total (what an LLM sees)")
	fmt.Println()
	fmt.Printf("  Individual mode: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(indTotal), fmtNum(audit.individualResourceTokens+audit.promptTokens), fmtNum(indTotal+audit.individualResourceTokens+audit.promptTokens))
	fmt.Printf("  Meta-tool mode:  ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(metaTotal), fmtNum(audit.metaBaseResourceTokens+audit.promptTokens), fmtNum(metaTotal+audit.metaBaseResourceTokens+audit.promptTokens))
	fmt.Printf("  Dynamic mode:    ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamicTotal), fmtNum(audit.dynamicBaseResourceTokens+audit.promptTokens), fmtNum(dynamicTotal+audit.dynamicBaseResourceTokens+audit.promptTokens))
	fmt.Printf("  Dynamic minimal: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(dynamicTotal), fmtNum(audit.dynamicMinimalResourceTokens), fmtNum(dynamicTotal+audit.dynamicMinimalResourceTokens))
	fmt.Println()
}

// listTools registers either individual tools or meta-tools on an in-memory MCP
// server and returns the published tool definitions for measurement.
//
// toolSurface comes from the [config] surface constants at every call site, so
// an unrecognized one is a programming error rather than a condition a caller
// could handle: it panics instead of measuring an empty server as if it were a
// surface.
func listTools(client *gitlabclient.Client, toolSurface string, enterprise bool) []*mcp.Tool {
	server := newAuditServer()

	switch toolSurface {
	case config.ToolSurfaceMeta:
		cmdutil.MustDo(tools.RegisterAllMeta(server, client, edition.TierForEnterprise(enterprise)))
		tools.RegisterMCPMeta(server, client)
	case config.ToolSurfaceIndividual:
		tools.RegisterAll(server, client, edition.TierForEnterprise(enterprise))
	default:
		panic(fmt.Sprintf("audit_tokens: unknown tool surface %q", toolSurface))
	}
	return listToolsFromServer(server)
}

// listDynamicTools registers the low-token dynamic public toolset backed by
// action routes and returns the advertised tool definitions.
func listDynamicTools(catalog *actioncatalog.Catalog) []*mcp.Tool {
	server := newAuditServer()
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return listToolsFromServer(server)
}

// buildMetaActionMaps builds the action route catalog that backs both
// meta-tools and the dynamic toolset.
func buildMetaActionMaps(client *gitlabclient.Client, enterprise bool) map[string]toolutil.ActionMap {
	return cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})).ActionMaps()
}

// newAuditServer builds the in-memory MCP server every tool-listing pass
// registers onto. The page size is large enough that a single tools/list page
// carries the whole individual surface.
func newAuditServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}})
}

// withSession pairs an in-memory client session with server, runs fn against
// it, and returns fn's result. Both sessions are torn down before it returns,
// the client's first.
//
// It replaces a pair of helpers, one handing the session back and one taking a
// callback, that had come to differ only in the wording of failures neither
// could produce. So did the four measurement passes layered on them.
//
// Every step of the handshake is a [cmdutil.Must]: an in-memory transport has
// no peer to be unreachable, no address to be taken and no wire to be cut, so
// there is nothing here for a caller to recover from and nothing for a report
// to say beyond what the panic already carries.
func withSession[T any](server *mcp.Server, fn func(context.Context, *mcp.ClientSession) T) T {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession := cmdutil.Must(server.Connect(ctx, st, nil))
	defer func() { cmdutil.MustDo(serverSession.Close()) }()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session := cmdutil.Must(mcpClient.Connect(ctx, ct, nil))
	defer func() { cmdutil.MustDo(session.Close()) }()

	return fn(ctx, session)
}

// listToolsFromServer connects to server in-memory and returns the advertised
// tool definitions.
func listToolsFromServer(server *mcp.Server) []*mcp.Tool {
	return withSession(server, func(ctx context.Context, session *mcp.ClientSession) []*mcp.Tool {
		return cmdutil.Must(session.ListTools(ctx, nil)).Tools
	})
}

// measureTools serializes each tool definition to JSON and estimates its token
// cost using the audit's byte-based heuristic.
func measureTools(toolList []*mcp.Tool) []toolTokenInfo {
	infos := make([]toolTokenInfo, 0, len(toolList))
	for _, t := range toolList {
		b := mustMarshalModelFacing(t)
		infos = append(infos, toolTokenInfo{
			Name:   t.Name,
			Domain: extractDomain(t.Name),
			Tokens: countTokens(b),
			Bytes:  len(b),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Tokens > infos[j].Tokens
	})
	return infos
}

// mustMarshalModelFacing serializes one catalog entry an in-memory session
// just handed back.
//
// Every entry measured here reached this process as JSON moments earlier and
// was decoded into the value being re-encoded, so the encode cannot fail. The
// error return of [marshalModelFacing] is kept for its own sake — it accepts
// any value, and its tests exercise inputs no session can produce.
func mustMarshalModelFacing(entry any) []byte {
	return cmdutil.Must(marshalModelFacing(entry))
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
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
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

	return withSession(server, func(ctx context.Context, session *mcp.ClientSession) int {
		totalTokens := 0
		for _, r := range cmdutil.Must(session.ListResources(ctx, nil)).Resources {
			totalTokens += countTokens(mustMarshalModelFacing(r))
		}
		for _, t := range cmdutil.Must(session.ListResourceTemplates(ctx, nil)).ResourceTemplates {
			totalTokens += countTokens(mustMarshalModelFacing(t))
		}
		return totalTokens
	})
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
//
// A failed listing used to be swallowed here and reported by the footprint
// path's own copy of this function, the one disagreement between the two.
// Neither can happen, and both now say so the same way.
func measurePrompts(client *gitlabclient.Client) int {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	prompts.Register(server, client)

	return withSession(server, func(ctx context.Context, session *mcp.ClientSession) int {
		totalTokens := 0
		for _, pr := range cmdutil.Must(session.ListPrompts(ctx, nil)).Prompts {
			totalTokens += countTokens(mustMarshalModelFacing(pr))
		}
		return totalTokens
	})
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
	fmt.Fprintf(tw, "  -\t------\t-----\t---------\n")
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
	fmt.Fprintf(tw, "  -\t------\t-----\t------\n")
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
func runMetaSchemaSizing() {
	client, cleanup := auditclient.NewMock()
	defer cleanup()

	server := newAuditServer()
	catalog := cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true}))
	routes := catalog.ActionMaps()
	tools.RegisterMetaCatalog(server, catalog)

	currentByName := map[string]map[string]any{}
	for _, t := range listToolsFromServer(server) {
		if t.InputSchema == nil {
			continue
		}
		raw := cmdutil.Must(json.Marshal(t.InputSchema))
		var m map[string]any
		cmdutil.MustDo(json.Unmarshal(raw, &m))
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
		opaqueJSON := cmdutil.Must(json.Marshal(currentByName[name]))

		full := toolutil.BuildMetaToolSchema(rmap, toolutil.MetaParamSchemaFull)
		compact := toolutil.BuildMetaToolSchema(rmap, toolutil.MetaParamSchemaCompact)

		fullJSON := cmdutil.Must(json.Marshal(full))
		compactJSON := cmdutil.Must(json.Marshal(compact))

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
		"meta-tool", "actions", "opaque", "full", "compact", "delta full")
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

// --- Token footprint mode (-footprint) ----------------------------------------
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
	// The token claim is the one-paragraph headline near the top of the README:
	// the startup cost of the default configuration, stated before the reader
	// has scrolled anywhere. It is rendered from the same measured rows as the
	// footprint table, so the two blocks cannot disagree, and it has its own
	// marker pair because it lives several sections away from the table.
	claimStartMarker = "<!-- START TOKEN CLAIM -->"
	claimEndMarker   = "<!-- END TOKEN CLAIM -->"
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

// measureFootprintRows is the measurement the footprint modes run. It is a
// variable so tests can drive the write and check paths against known rows
// without paying for the full tier x surface x mode measurement each time.
var measureFootprintRows = measureTokenFootprintRows

// runFootprintMode builds the mock-backed client and runs the full token
// footprint measurement, mirroring the self-contained client pattern of
// [runMetaSchemaSizing].
func runFootprintMode(check bool) error {
	client, cleanup := auditclient.NewMock()
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
	rows := measureFootprintRows(client)

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

// footprintStaleTargets compares the live README text, detailed-doc text and
// site data against the freshly rendered footprint content and returns the
// human-readable names of any targets that are out of date. An empty slice
// means everything is current. Kept pure (no I/O) so the drift logic is
// unit-testable.
//
// The README token-claim block is reported as stale, not as an error, when
// its markers are missing: a README without the block is precisely the
// "not current" state the check exists to catch, and the fix is the same
// generator run either way.
func footprintStaleTargets(readmeText, detailedText, siteText string, rows []tokenFootprintRow) ([]string, error) {
	var stale []string

	updated, err := docgen.ComputeReplacedSection(readmeText, footprintStartMarker, footprintEndMarker, renderReadmeFootprint(rows))
	if err != nil {
		return nil, err
	}
	if updated != readmeText {
		stale = append(stale, readmePath+" token-footprint section")
	}
	claim, err := renderReadmeTokenClaim(rows)
	if err != nil {
		return nil, err
	}
	if claimed, claimErr := docgen.ComputeReplacedSection(readmeText, claimStartMarker, claimEndMarker, claim); claimErr != nil {
		stale = append(stale, readmePath+" token-claim block (markers missing)")
	} else if claimed != readmeText {
		stale = append(stale, readmePath+" token-claim block")
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
// README managed sections (the headline claim and the footprint table) plus
// the detailed reference doc and the site data file.
func runFootprint(client *gitlabclient.Client) error {
	rows := measureFootprintRows(client)
	claim, err := renderReadmeTokenClaim(rows)
	if err != nil {
		return err
	}
	if replaceErr := docgen.ReplaceSection(readmePath, claimStartMarker, claimEndMarker, claim); replaceErr != nil {
		return replaceErr
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
	fmt.Printf("Updated %s token-claim block and token-footprint section, %s and %s (%d rows across all tiers/surfaces/modes)\n", readmePath, detailedFootprintPath, siteFootprintPath, len(rows))
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
	// The site quotes the dynamic figures once, for every tier, and its landing
	// says so in words. Refuse to publish that sentence the day it stops being
	// true rather than let one tier's numbers stand for all of them.
	if err := requireTierInvariantDynamic(rows, dynamic.ToolSchemaTokens, shared); err != nil {
		return nil, err
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

	raw := cmdutil.Must(json.MarshalIndent(siteFootprint{
		Tokenizer:  "cl100k_base",
		Shared:     shared,
		Dynamic:    dynamic,
		Individual: individual,
	}, "", "  "))
	return append(raw, '\n'), nil
}

// requireTierInvariantDynamic returns an error naming the first dynamic-surface
// row whose figures differ from the ones the site data quotes for every tier.
func requireTierInvariantDynamic(rows []tokenFootprintRow, toolSchemaTokens int, shared siteFootprintShared) error {
	expectedShared := map[string]int{
		dynamicDefaultConfiguration: shared.Full,
		dynamicMinimalConfiguration: shared.Minimal,
	}
	for _, r := range rows {
		wantShared, ok := expectedShared[r.Configuration]
		if !ok {
			continue
		}
		if r.ToolSchemaTokens != toolSchemaTokens || r.SharedTokens != wantShared {
			return fmt.Errorf("%s tier %s differs from %s (tool schema %d vs %d, shared %d vs %d); the site data quotes one dynamic figure for every tier",
				r.Tier, r.Configuration, ultimateTierLabel, r.ToolSchemaTokens, toolSchemaTokens, r.SharedTokens, wantShared)
		}
	}
	return nil
}

func measureTokenFootprintRows(client *gitlabclient.Client) []tokenFootprintRow {
	var allRows []tokenFootprintRow
	tiers := []struct {
		label string
		tier  edition.Tier
	}{
		{"Free/CE", edition.Free},
		{"Premium", edition.Premium},
		{"Ultimate", edition.Ultimate},
	}

	promptTokens := measurePrompts(client)
	for i, t := range tiers {
		cmdutil.Progressf("audit_tokens: measuring footprint [%d/%d] %s tier (all surfaces x schema modes)...", i+1, len(tiers), t.label)
		allRows = append(allRows, measureTierFootprintWithPrompts(client, t.tier, t.label, promptTokens)...)
	}
	return allRows
}

// measureTierFootprintWithPrompts measures one tier; promptTokens carries the
// tier-independent prompt measurement so the caller pays it once for all
// three tiers (a negative value measures it here, for a standalone caller).
func measureTierFootprintWithPrompts(client *gitlabclient.Client, tier edition.Tier, tierLabel string, promptTokens int) []tokenFootprintRow {
	metaCatalog := cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       tier,
		IncludeMCP: true,
	}))

	dynamicCatalog := cmdutil.Must(dynamictools.AddStandaloneCatalog(metaCatalog, client, dynamictools.StandaloneOptions{}))
	metaRoutes := metaCatalog.ActionMaps()
	dynamicRoutes := dynamicCatalog.ActionMaps()
	reachableActions := countActions(dynamicRoutes)

	dynamicTools := listDynamicTools(dynamicCatalog)
	individualTools := listIndividualToolsAtTier(client, tier)

	dynamicToolTokens := measureToolSchemaTokens(dynamicTools)
	individualToolTokens := measureToolSchemaTokens(individualTools)
	if promptTokens < 0 {
		promptTokens = measurePrompts(client)
	}

	dynamicFullShared := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: dynamicRoutes, ToolCatalog: dynamicCatalog, ToolList: dynamicTools,
		ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: config.CapabilitySurfaceFull, PromptTokens: promptTokens,
	})
	dynamicMinimalShared := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: dynamicRoutes, ToolCatalog: dynamicCatalog, ToolList: dynamicTools,
		ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: config.CapabilitySurfaceMinimal, PromptTokens: promptTokens,
	})
	metaFullShared := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: metaRoutes, ToolCatalog: metaCatalog, ToolList: nil,
		ToolSurface: config.ToolSurfaceMeta, CapabilitySurface: config.CapabilitySurfaceFull, PromptTokens: promptTokens,
	})
	metaMinimalShared := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes: metaRoutes, ToolCatalog: metaCatalog, ToolList: nil,
		ToolSurface: config.ToolSurfaceMeta, CapabilitySurface: config.CapabilitySurfaceMinimal, PromptTokens: promptTokens,
	})
	individualFullShared := measureSharedTokens(client, sharedTokenMeasureOptions{
		ToolList:    individualTools,
		ToolSurface: config.ToolSurfaceIndividual, CapabilitySurface: config.CapabilitySurfaceFull, PromptTokens: promptTokens,
	})

	rows := []tokenFootprintRow{
		{Tier: tierLabel, Configuration: dynamicDefaultConfiguration, VisibleTools: len(dynamicTools), ReachableActions: reachableActions, ToolSchemaTokens: dynamicToolTokens, SharedTokens: dynamicFullShared},
		{Tier: tierLabel, Configuration: dynamicMinimalConfiguration, VisibleTools: len(dynamicTools), ReachableActions: reachableActions, ToolSchemaTokens: dynamicToolTokens, SharedTokens: dynamicMinimalShared},
	}

	metaSchemaModes := []string{"opaque", "compact", "full"}
	for _, mode := range metaSchemaModes {
		restore := tools.SetMetaParamSchemaScoped(mode)
		metaTools := listMetaToolsFromCatalog(client, metaCatalog)
		metaTokens := measureToolSchemaTokens(metaTools)
		restore()
		rows = append(
			rows,
			tokenFootprintRow{Tier: tierLabel, Configuration: fmt.Sprintf("`meta` / `full` (%s)", mode), MetaParamSchema: mode, VisibleTools: len(metaTools), ReachableActions: reachableActions, ToolSchemaTokens: metaTokens, SharedTokens: metaFullShared},
			tokenFootprintRow{Tier: tierLabel, Configuration: fmt.Sprintf("`meta` / `minimal` (%s)", mode), MetaParamSchema: mode, VisibleTools: len(metaTools), ReachableActions: reachableActions, ToolSchemaTokens: metaTokens, SharedTokens: metaMinimalShared},
		)
	}

	rows = append(rows, tokenFootprintRow{Tier: tierLabel, Configuration: "`individual` / `full`", VisibleTools: len(individualTools), ReachableActions: len(individualTools), ToolSchemaTokens: individualToolTokens, SharedTokens: individualFullShared})
	return rows
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

// renderReadmeTokenClaim renders the README's headline paragraph: what the
// default configuration (dynamic surface, full capability surface) costs in
// startup context, and what the minimal capability surface costs instead.
// Both figures are the per-tier totals of the same measured rows the footprint
// table prints, so the claim can never say something the table does not.
//
// When every tier costs the same the claim states one figure and says so;
// when the tiers ever diverge it states the span ("from X to Y") instead of
// picking a tier, so a future tier-dependent cost cannot make the sentence
// quietly wrong.
func renderReadmeTokenClaim(rows []tokenFootprintRow) (string, error) {
	defaultLo, defaultHi, ok := dynamicTotalSpan(rows, dynamicDefaultConfiguration)
	if !ok {
		return "", fmt.Errorf("token claim: no %s row to quote", dynamicDefaultConfiguration)
	}
	minimalLo, minimalHi, ok := dynamicTotalSpan(rows, dynamicMinimalConfiguration)
	if !ok {
		return "", fmt.Errorf("token claim: no %s row to quote", dynamicMinimalConfiguration)
	}

	tierClause := "the same on every GitLab tier"
	if defaultLo != defaultHi {
		tierClause = "depending on the GitLab tier"
	}
	return fmt.Sprintf(
		"**%s tokens of startup context by default, %s (%s with `CAPABILITY_SURFACE=minimal`).** Two tools reach the whole catalog; measured with the cl100k_base tokenizer and verified in CI on every commit. [How it is measured](#token-footprint)\n",
		fmtTokenSpan(defaultLo, defaultHi, "From"),
		tierClause,
		fmtTokenSpan(minimalLo, minimalHi, "from"),
	), nil
}

// dynamicTotalSpan returns the smallest and largest startup total (tool schemas
// plus shared resources and prompts) among the rows of one configuration
// across all tiers. ok is false when no row carries that configuration.
func dynamicTotalSpan(rows []tokenFootprintRow, configuration string) (lo, hi int, ok bool) {
	var totals []int
	for _, r := range rows {
		if r.Configuration == configuration {
			totals = append(totals, r.totalTokens())
		}
	}
	if len(totals) == 0 {
		return 0, 0, false
	}
	return slices.Min(totals), slices.Max(totals), true
}

// fmtTokenSpan renders a single thousands-separated figure when lo and hi
// agree, and "<from> lo to hi" otherwise. from is the word that opens the
// span, capitalised or not depending on where the caller places it.
func fmtTokenSpan(lo, hi int, from string) string {
	if lo == hi {
		return fmtNum(lo)
	}
	return fmt.Sprintf("%s %s to %s", from, fmtNum(lo), fmtNum(hi))
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
	b.WriteString("Token counts use the **cl100k_base** tokenizer (the GPT-4 / GPT-3.5 encoding) via [`github.com/tiktoken-go/tokenizer`](https://github.com/tiktoken-go/tokenizer). A pure Go port of OpenAI's tiktoken. The vocabulary is embedded at compile time (~4 MB). This is significantly more accurate than the `bytes \u00f7 4` heuristic for JSON-dense content like MCP tool schemas, which contain many braces, identifiers, and nested objects.\n\n")

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
	b.WriteString("- **Individual mode** exposes every action as its own tool. This is the highest-fidelity but most expensive surface. Suitable only for clients with large context windows.\n")
	b.WriteString("- **Tier scaling**: Free/CE has the fewest actions. Premium adds enterprise features. Ultimate includes everything. The token cost scales with the number of available actions.\n")
	b.WriteString("- **Shared tokens** are dominated by MCP resources (`gitlab://tools` template, workflow guides) and prompts. The `minimal` capability surface strips these to just `gitlab://tools`, cutting shared overhead by ~90%%.\n")
	return b.String()
}

func listMetaToolsFromCatalog(client *gitlabclient.Client, catalog *actioncatalog.Catalog) []*mcp.Tool {
	server := newAuditServer()
	tools.RegisterMetaCatalog(server, catalog)
	tools.RegisterMetaStandaloneTools(server, client)
	return listToolsFromServer(server)
}

func listIndividualToolsAtTier(client *gitlabclient.Client, tier edition.Tier) []*mcp.Tool {
	server := newAuditServer()
	tools.RegisterAll(server, client, tier)
	return listToolsFromServer(server)
}

// measureToolSchemaTokens sums the token cost of a tool list without keeping
// the per-tool breakdown [measureTools] records.
func measureToolSchemaTokens(toolList []*mcp.Tool) int {
	totalTokens := 0
	for _, t := range toolList {
		totalTokens += countTokens(mustMarshalModelFacing(t))
	}
	return totalTokens
}

func measureSharedTokens(client *gitlabclient.Client, opts sharedTokenMeasureOptions) int {
	resourceTokens := measureResourcesWithOptions(client, opts.Routes, resourceRegistrationOptions{
		Core:           opts.CapabilitySurface == config.CapabilitySurfaceFull,
		ToolManifest:   true,
		ToolSurface:    opts.ToolSurface,
		ToolList:       opts.ToolList,
		ToolCatalog:    opts.ToolCatalog,
		WorkflowGuides: opts.CapabilitySurface == config.CapabilitySurfaceFull,
	})
	if opts.CapabilitySurface == config.CapabilitySurfaceFull {
		return resourceTokens + opts.PromptTokens
	}
	return resourceTokens
}
