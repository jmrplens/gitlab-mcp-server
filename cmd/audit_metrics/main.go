// Command audit_metrics generates a comprehensive metrics summary for the
// gitlab-mcp-server MCP server. It creates in-memory MCP servers to count tools,
// meta-tools, resources, and prompts at runtime — the only reliable counting
// method. It also scans the filesystem for Go packages, source files, and test
// files.
//
// Usage:
//
//	go run ./cmd/audit_metrics/
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

// Audit server identity values used for in-memory MCP introspection sessions.
const (
	auditServerName  = "audit-metrics"
	auditClientName  = "audit-metrics-client"
	auditVersion     = "0.0.1"
	toolListFormat   = "  - %s\n"
	metricLabelWidth = 48
)

var topDomains = 20

// main builds the audit client, gathers runtime counts from the registered MCP
// surface, and prints the metrics report to stdout.
func main() {
	flag.IntVar(&topDomains, "top-domains", 20, "number of domains to list by tool count")
	jsonOut := flag.Bool("json", false, "emit JSON summary instead of markdown report")
	siteStatsPath := flag.String("site-stats", "", "write the single-sourced site stats JSON to the given path (use with -check to verify instead of write)")
	checkOnly := flag.Bool("check", false, "with -site-stats, verify the committed file is up to date instead of writing it")
	flag.Parse()
	if topDomains < 0 {
		cmdutil.Fatalf("-top-domains must be >= 0")
	}

	cmdutil.Progressf("audit_metrics: building catalog and counting tools/resources/prompts across surfaces...")
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()
	gitLabComClient, err := gitlabclient.NewClient(&config.Config{ //#nosec G101 -- not a real credential, audit-only dummy token
		GitLabURL:   config.DefaultGitLabURL,
		GitLabToken: "audit-token",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create gitlab.com client: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit.
	}

	if *siteStatsPath != "" {
		if statsErr := writeOrCheckSiteStats(*siteStatsPath, generateSiteStats(client, gitLabComClient), *checkOnly); statsErr != nil {
			cmdutil.Fatalf("%v", statsErr)
		}
		return
	}

	metrics := collectMetrics(client, gitLabComClient)
	if *jsonOut {
		if encErr := writeJSONSummary(os.Stdout, metrics); encErr != nil {
			cmdutil.Fatalf("encode json: %v", encErr)
		}
		return
	}
	printReport(metrics, client)
}

// exitProcess terminates the process after an unrecoverable failure has been
// reported on stderr. It is a variable so tests can observe the exit path
// instead of dying with it.
var exitProcess = os.Exit

// auditMetrics holds every number the text report and the JSON summary
// print, gathered once by [collectMetrics] so both renderers read the same
// measurement.
//
// The tool lists are the advertised MCP tool definitions per surface and
// deployment (self-managed vs GitLab.com, base vs enterprise). The dynamic
// action counts are catalog route totals, the registry metrics describe the
// dynamic search index, and gitLabComEnterpriseDomains is the per-domain
// action count of the widest catalog. The remaining fields are the resource,
// prompt and codebase counts.
type auditMetrics struct {
	individualTools            []*mcp.Tool
	gitLabComIndividualTools   []*mcp.Tool
	metaBase                   []*mcp.Tool
	metaEnterprise             []*mcp.Tool
	metaGitLabComEnterprise    []*mcp.Tool
	dynamicBase                []*mcp.Tool
	dynamicEnterprise          []*mcp.Tool
	dynamicGitLabComEnterprise []*mcp.Tool

	dynamicBaseActions                int
	dynamicEnterpriseActions          int
	dynamicGitLabComEnterpriseActions int

	dynamicBaseMetrics                dynamictools.RegistryMetrics
	dynamicEnterpriseMetrics          dynamictools.RegistryMetrics
	dynamicGitLabComEnterpriseMetrics dynamictools.RegistryMetrics

	enterpriseActionAudit      enterpriseActionSpecAudit
	gitLabComEnterpriseDomains map[string]int

	staticResources   int
	templateResources int
	promptCount       int
	toolPackages      int
	srcFiles          int
	testFiles         int
	elicitationCount  int
}

// collectMetrics gathers every count the report prints from the registered
// MCP surfaces, the canonical catalog and the repository tree. client is a
// self-managed instance client; gitLabComClient targets GitLab.com so the
// GitLab.com-only tools are counted where relevant.
func collectMetrics(client, gitLabComClient *gitlabclient.Client) auditMetrics {
	dynamicBaseCatalog := dynamicActionCatalog(client, false)
	dynamicEnterpriseCatalog := dynamicActionCatalog(client, true)
	dynamicGitLabComEnterpriseCatalog := dynamicActionCatalog(gitLabComClient, true)

	metrics := auditMetrics{
		individualTools:                   listServerTools(client, false, false),
		gitLabComIndividualTools:          listServerTools(gitLabComClient, false, false),
		metaBase:                          listServerTools(client, true, false),
		metaEnterprise:                    listServerTools(client, true, true),
		metaGitLabComEnterprise:           listServerTools(gitLabComClient, true, true),
		dynamicBase:                       listDynamicTools(dynamicBaseCatalog),
		dynamicEnterprise:                 listDynamicTools(dynamicEnterpriseCatalog),
		dynamicGitLabComEnterprise:        listDynamicTools(dynamicGitLabComEnterpriseCatalog),
		dynamicBaseActions:                countActionRoutes(dynamicBaseCatalog.ActionMaps()),
		dynamicEnterpriseActions:          countActionRoutes(dynamicEnterpriseCatalog.ActionMaps()),
		dynamicGitLabComEnterpriseActions: countActionRoutes(dynamicGitLabComEnterpriseCatalog.ActionMaps()),
		dynamicBaseMetrics:                dynamicSearchMetrics(dynamicBaseCatalog),
		dynamicEnterpriseMetrics:          dynamicSearchMetrics(dynamicEnterpriseCatalog),
		dynamicGitLabComEnterpriseMetrics: dynamicSearchMetrics(dynamicGitLabComEnterpriseCatalog),
		enterpriseActionAudit:             auditEnterpriseActionSpecs(dynamicBaseCatalog, dynamicEnterpriseCatalog, dynamicGitLabComEnterpriseCatalog),
		gitLabComEnterpriseDomains:        countCatalogDomains(dynamicGitLabComEnterpriseCatalog),
		promptCount:                       countPrompts(client),
		toolPackages:                      countToolPackages(),
	}
	metrics.staticResources, metrics.templateResources = countResources(client)
	metrics.srcFiles, metrics.testFiles = countSourceFiles()
	for _, tool := range metrics.individualTools {
		if strings.HasPrefix(tool.Name, "gitlab_interactive_") {
			metrics.elicitationCount++
		}
	}
	return metrics
}

// writeJSONSummary encodes the headline counts as an indented JSON object,
// the -json form of the report.
func writeJSONSummary(w io.Writer, metrics auditMetrics) error {
	summary := struct {
		IndividualTools   int `json:"individual_tools"`
		MetaBase          int `json:"meta_base"`
		MetaEnterprise    int `json:"meta_enterprise"`
		DynamicBase       int `json:"dynamic_base"`
		DynamicEnterprise int `json:"dynamic_enterprise"`
		Resources         int `json:"resources"`
		Prompts           int `json:"prompts"`
		ToolPackages      int `json:"tool_packages"`
		SourceFiles       int `json:"source_files"`
		TestFiles         int `json:"test_files"`
	}{
		len(metrics.individualTools), len(metrics.metaBase), len(metrics.metaEnterprise),
		len(metrics.dynamicBase), len(metrics.dynamicEnterprise),
		metrics.staticResources + metrics.templateResources, metrics.promptCount, metrics.toolPackages, metrics.srcFiles, metrics.testFiles,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

// printReport writes the Markdown metrics report to stdout. client is needed
// only by the schema-mode section, which re-registers the meta surface under
// each META_PARAM_SCHEMA mode to size its input schemas.
func printReport(metrics auditMetrics, client *gitlabclient.Client) {
	resourceCount := metrics.staticResources + metrics.templateResources

	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("  gitlab-mcp-server. MCP Server Metrics Audit")
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println()

	fmt.Println("## Core Metrics")
	fmt.Println()
	printRow("Individual MCP tools (self-managed enterprise)", len(metrics.individualTools))
	printRow("Individual MCP tools (GitLab.com enterprise)", len(metrics.gitLabComIndividualTools))
	printRow("Meta-tools (base)", len(metrics.metaBase))
	printRow("Meta-tools (self-managed enterprise)", len(metrics.metaEnterprise))
	printRow("Meta-tools (GitLab.com enterprise)", len(metrics.metaGitLabComEnterprise))
	printRow("Dynamic tools (base)", len(metrics.dynamicBase))
	printRow("Dynamic tools (self-managed enterprise)", len(metrics.dynamicEnterprise))
	printRow("Dynamic tools (GitLab.com enterprise)", len(metrics.dynamicGitLabComEnterprise))
	printRow("Dynamic catalog actions (base)", metrics.dynamicBaseActions)
	printRow("Dynamic catalog actions (self-managed enterprise)", metrics.dynamicEnterpriseActions)
	printRow("Dynamic catalog actions (GitLab.com enterprise)", metrics.dynamicGitLabComEnterpriseActions)
	printDynamicSearchMetrics(metrics.dynamicBaseMetrics, metrics.dynamicEnterpriseMetrics, metrics.dynamicGitLabComEnterpriseMetrics)
	printRow("Spec-backed enterprise catalog actions", len(metrics.enterpriseActionAudit.SpecBacked))
	printRow("Enterprise catalog actions missing ActionSpec", len(metrics.enterpriseActionAudit.MissingSpec))
	printRow("Enterprise-only meta-tools", diffByName(metrics.metaEnterprise, metrics.metaBase))
	printRow("GitLab.com-only meta-tools", diffByName(metrics.metaGitLabComEnterprise, metrics.metaEnterprise))
	printRow("GitLab.com-only individual tools", diffByName(metrics.gitLabComIndividualTools, metrics.individualTools))
	printRow("MCP Resources (total)", resourceCount)
	printRow("  Static resources", metrics.staticResources)
	printRow("  Resource templates", metrics.templateResources)
	printRow("  Workspace roots", 1)
	printRow("MCP Prompts", metrics.promptCount)
	fmt.Println()

	fmt.Println("## Tool Categories")
	fmt.Println()
	printRow("Elicitation tools", metrics.elicitationCount)
	printRow("Standard tools", len(metrics.individualTools)-metrics.elicitationCount)
	fmt.Println()

	fmt.Println("## Meta-Tool Schema Modes")
	fmt.Println()
	printMetaSchemaModes(client)
	fmt.Println()

	fmt.Println("## Codebase Metrics")
	fmt.Println()
	printRow("internal/tools Go packages", metrics.toolPackages)
	printRow("Source files (.go)", metrics.srcFiles)
	printRow("Test files (_test.go)", metrics.testFiles)
	fmt.Println()

	fmt.Printf("## Catalog Domain Breakdown (GitLab.com enterprise, top %d)\n", topDomains)
	fmt.Println()
	printDomainTable(metrics.gitLabComEnterpriseDomains)
	fmt.Println()

	printEnterpriseActionSpecAudit(metrics.enterpriseActionAudit)
	fmt.Println()

	fmt.Println("## Meta-tools List")
	fmt.Println()
	fmt.Println("### Base (" + strconv.Itoa(len(metrics.metaBase)) + ")")
	for _, tool := range metrics.metaBase {
		fmt.Printf(toolListFormat, tool.Name)
	}
	fmt.Println()
	fmt.Println("### Enterprise-only (" + strconv.Itoa(diffByName(metrics.metaEnterprise, metrics.metaBase)) + ")")
	baseNames := map[string]bool{}
	for _, tool := range metrics.metaBase {
		baseNames[tool.Name] = true
	}
	for _, tool := range metrics.metaEnterprise {
		if !baseNames[tool.Name] {
			fmt.Printf(toolListFormat, tool.Name)
		}
	}
	fmt.Println()
	fmt.Println("### GitLab.com-only enterprise (" + strconv.Itoa(diffByName(metrics.metaGitLabComEnterprise, metrics.metaEnterprise)) + ")")
	enterpriseNames := map[string]bool{}
	for _, tool := range metrics.metaEnterprise {
		enterpriseNames[tool.Name] = true
	}
	for _, tool := range metrics.metaGitLabComEnterprise {
		if !enterpriseNames[tool.Name] {
			fmt.Printf(toolListFormat, tool.Name)
		}
	}
}

// countToolPackages counts Go package directories under internal/tools,
// matching the catalog-first architecture where ordinary tool packages no
// longer carry package-local register.go files.
func countToolPackages() int {
	return countToolPackageDirsAt(filepath.Join(repositoryRoot(), "internal", "tools"))
}

func repositoryRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func countToolPackageDirsAt(toolsDir string) int {
	count := 0
	err := filepath.WalkDir(toolsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "WalkDir %s: %v\n", path, walkErr)
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if directoryHasGoFile(path) {
			count++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "WalkDir %s: %v\n", toolsDir, err)
	}
	return count
}

func directoryHasGoFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true
		}
	}
	return false
}

func countCatalogDomains(catalog *actioncatalog.Catalog) map[string]int {
	domains := map[string]int{}
	if catalog == nil {
		return domains
	}
	for _, action := range catalog.Actions() {
		domain := action.Domain
		if domain == "" {
			domain, _, _ = strings.Cut(string(action.ID), ".")
		}
		if domain == "" {
			domain = "unknown"
		}
		domains[domain]++
	}
	return domains
}

// enterpriseActionSpecAudit classifies the catalog actions that only appear
// in self-managed or GitLab.com Enterprise builds but not in the base
// catalog.
//
// SpecBacked lists action IDs that are covered by an ActionSpec in the
// owner package. MissingSpec lists action IDs that lack an ActionSpec and
// therefore violate the catalog-first contract. Both slices are sorted and
// deduplicated before being returned.
type enterpriseActionSpecAudit struct {
	SpecBacked  []string
	MissingSpec []string
}

func auditEnterpriseActionSpecs(base, selfManagedEnterprise, gitLabComEnterprise *actioncatalog.Catalog) enterpriseActionSpecAudit {
	baseIDs := catalogActionIDSet(base)
	seen := map[actioncatalog.ActionID]bool{}
	audit := enterpriseActionSpecAudit{}
	for _, catalog := range []*actioncatalog.Catalog{selfManagedEnterprise, gitLabComEnterprise} {
		for _, action := range catalog.Actions() {
			if baseIDs[action.ID] || seen[action.ID] {
				continue
			}
			seen[action.ID] = true
			if action.SpecBacked {
				audit.SpecBacked = append(audit.SpecBacked, string(action.ID))
				continue
			}
			audit.MissingSpec = append(audit.MissingSpec, string(action.ID))
		}
	}
	sort.Strings(audit.SpecBacked)
	sort.Strings(audit.MissingSpec)
	return audit
}

func catalogActionIDSet(catalog *actioncatalog.Catalog) map[actioncatalog.ActionID]bool {
	ids := map[actioncatalog.ActionID]bool{}
	for _, action := range catalog.Actions() {
		ids[action.ID] = true
	}
	return ids
}

func printEnterpriseActionSpecAudit(audit enterpriseActionSpecAudit) {
	fmt.Println("## Enterprise ActionSpec Audit")
	fmt.Println()
	fmt.Println("### Spec-backed enterprise actions (" + strconv.Itoa(len(audit.SpecBacked)) + ")")
	printActionIDList(audit.SpecBacked)
	fmt.Println()
	fmt.Println("### Enterprise actions missing ActionSpec (" + strconv.Itoa(len(audit.MissingSpec)) + ")")
	printActionIDList(audit.MissingSpec)
}

func printActionIDList(actionIDs []string) {
	if len(actionIDs) == 0 {
		fmt.Println("  - none")
		return
	}
	for _, actionID := range actionIDs {
		fmt.Printf(toolListFormat, actionID)
	}
}

// dynamicSearchMetrics builds the dynamic registry and returns static search
// index and alias metrics without changing the advertised MCP tool count.
func dynamicSearchMetrics(catalog *actioncatalog.Catalog) dynamictools.RegistryMetrics {
	return dynamictools.NewRegistryFromCatalog(catalog).Metrics()
}

func printDynamicSearchMetrics(base, enterprise, gitLabCom dynamictools.RegistryMetrics) {
	printRow("Dynamic search index tokens (base)", base.IndexTokenCount)
	printRow("Dynamic search index tokens (self-managed enterprise)", enterprise.IndexTokenCount)
	printRow("Dynamic search index tokens (GitLab.com enterprise)", gitLabCom.IndexTokenCount)
	printRow("Dynamic search index postings (base)", base.IndexPostingCount)
	printRow("Dynamic search index postings (self-managed enterprise)", enterprise.IndexPostingCount)
	printRow("Dynamic search index postings (GitLab.com enterprise)", gitLabCom.IndexPostingCount)
	printRow("Dynamic aliases (base)", base.AliasCount)
	printRow("Dynamic aliases (self-managed enterprise)", enterprise.AliasCount)
	printRow("Dynamic aliases (GitLab.com enterprise)", gitLabCom.AliasCount)
	printRow("Dynamic aliases searchable (base)", base.SearchableAliasCount)
	printRow("Dynamic aliases searchable (self-managed enterprise)", enterprise.SearchableAliasCount)
	printRow("Dynamic aliases searchable (GitLab.com enterprise)", gitLabCom.SearchableAliasCount)
	printRow("Dynamic aliases unsearchable (base)", base.UnsearchableAliasCount)
	printRow("Dynamic aliases unsearchable (self-managed enterprise)", enterprise.UnsearchableAliasCount)
	printRow("Dynamic aliases unsearchable (GitLab.com enterprise)", gitLabCom.UnsearchableAliasCount)
	printRow("Dynamic aliases ambiguous (base)", base.AmbiguousAliasCount)
	printRow("Dynamic aliases ambiguous (self-managed enterprise)", enterprise.AmbiguousAliasCount)
	printRow("Dynamic aliases ambiguous (GitLab.com enterprise)", gitLabCom.AmbiguousAliasCount)
}

func dynamicActionCatalog(client *gitlabclient.Client, enterprise bool) *actioncatalog.Catalog {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build dynamic action catalog: %v\n", err)
		exitProcess(1)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add standalone dynamic actions: %v\n", err)
		exitProcess(1)
	}
	return catalog
}

// listDynamicTools registers the low-token dynamic public toolset backed by
// catalog action routes and returns the advertised tool definitions.
func listDynamicTools(catalog *actioncatalog.Catalog) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, &mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}})
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return listToolsFromServer(server)
}

// countActionRoutes counts catalog action routes in a dynamic/meta route map.
func countActionRoutes(routes map[string]toolutil.ActionMap) int {
	count := 0
	for _, actions := range routes {
		count += len(actions)
	}
	return count
}

// listToolsFromServer connects to server in-memory and returns advertised tools.
func listToolsFromServer(server *mcp.Server) []*mcp.Tool {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		exitProcess(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect: %v\n", err)
		exitProcess(1)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTools: %v\n", err)
		exitProcess(1)
	}
	return result.Tools
}

// listServerTools registers tools on an in-memory MCP server and returns
// the full tool list. When meta is true, meta-tools are registered.
// Enterprise controls whether Enterprise/Premium meta-tools are included.
// listedToolsCache memoizes listServerTools per (client, surface, tier,
// meta schema mode): registering a full surface costs seconds, the result
// depends only on the compiled-in catalog and those inputs, and callers only
// read it. The schema mode is part of the key because printMetaSchemaModes
// re-registers the meta surface under each mode to size its input schemas;
// a key without it would hand every mode the first listing and report three
// identical sizes. The audit repeats the opaque meta enterprise listing, and
// the test binary repeats several combinations.
var listedToolsCache sync.Map // listKey -> []*mcp.Tool

type listKey struct {
	client     *gitlabclient.Client
	meta       bool
	enterprise bool
	schemaMode string
}

func listServerTools(client *gitlabclient.Client, meta, enterprise bool) []*mcp.Tool {
	key := listKey{client: client, meta: meta, enterprise: enterprise, schemaMode: tools.MetaParamSchema()}
	if cached, ok := listedToolsCache.Load(key); ok {
		listed, _ := cached.([]*mcp.Tool)
		return listed
	}
	listed := listServerToolsUncached(client, meta, enterprise)
	listedToolsCache.Store(key, listed)
	return listed
}

func listServerToolsUncached(client *gitlabclient.Client, meta, enterprise bool) []*mcp.Tool {
	opts := &mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}}
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, opts)

	if meta {
		if err := tools.RegisterAllMeta(server, client, edition.TierForEnterprise(enterprise)); err != nil {
			fmt.Fprintf(os.Stderr, "register meta tools: %v\n", err)
			exitProcess(1)
		}
	} else {
		tools.RegisterAll(server, client, edition.Ultimate)
	}

	return listToolsFromServer(server)
}

// countResources registers all MCP resources and returns static and template counts.
// This includes resources from Register(), schema resources, and RegisterWorkflowGuides().
// Workspace roots (+1) are counted separately because they need a roots.Manager.
func countResources(client *gitlabclient.Client) (static, templates int) {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	metaCatalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build meta action catalog: %v\n", err)
		exitProcess(1)
	}
	resources.Register(server, client)
	resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
		Surface: config.ToolSurfaceDynamic,
		Catalog: metaCatalog,
	})
	resources.RegisterWorkflowGuides(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, connectErr := server.Connect(ctx, st, nil); connectErr != nil {
		fmt.Fprintf(os.Stderr, "server connect (resources): %v\n", connectErr)
		exitProcess(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (resources): %v\n", err)
		exitProcess(1)
	}
	defer session.Close()

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListResources: %v\n", err)
		exitProcess(1)
	}
	static = len(res.Resources)

	tpl, tplErr := session.ListResourceTemplates(ctx, nil)
	if tplErr != nil {
		fmt.Fprintf(os.Stderr, "ListResourceTemplates: %v\n", tplErr)
		exitProcess(1)
	}
	templates = len(tpl.ResourceTemplates)
	return static, templates
}

// countPrompts registers all MCP prompts and returns the count.
func countPrompts(client *gitlabclient.Client) int {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	prompts.Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (prompts): %v\n", err)
		exitProcess(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (prompts): %v\n", err)
		exitProcess(1)
	}
	defer session.Close()

	result, err := session.ListPrompts(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListPrompts: %v\n", err)
		exitProcess(1)
	}
	return len(result.Prompts)
}

// countSourceFiles walks the internal/ directory and counts .go source files
// and _test.go test files separately.
func countSourceFiles() (src, test int) {
	return countSourceFilesAt(filepath.Join(repositoryRoot(), "internal"))
}

// countSourceFilesAt walks dir and counts .go source files and _test.go test
// files separately, reporting a walk failure on stderr.
func countSourceFilesAt(dir string) (src, test int) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			test++
		} else {
			src++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Walk: %v\n", err)
	}
	return src, test
}

// printRow prints a metric row with aligned formatting.
func printRow(label string, value int) {
	fmt.Printf("  %-*s %d\n", metricLabelWidth, label, value)
}

func diffByName(a, b []*mcp.Tool) int {
	seen := make(map[string]struct{}, len(b))
	for _, tool := range b {
		seen[tool.Name] = struct{}{}
	}
	count := 0
	for _, tool := range a {
		if _, ok := seen[tool.Name]; !ok {
			count++
		}
	}
	return count
}

// printMetaSchemaModes reports the active META_PARAM_SCHEMA mode and the
// total meta-tool InputSchema byte size each mode would produce. Useful
// for ops to size the impact of META_PARAM_SCHEMA before flipping it.
func printMetaSchemaModes(client *gitlabclient.Client) {
	// Read META_PARAM_SCHEMA directly: config.Load() requires GITLAB_URL +
	// GITLAB_TOKEN and would silently fall back to "opaque" if they are
	// missing, misreporting the active mode in environments where this
	// tool is invoked without full GitLab credentials (e.g., audits).
	active := strings.ToLower(strings.TrimSpace(os.Getenv("META_PARAM_SCHEMA")))
	switch active {
	case config.MetaParamSchemaCompact, config.MetaParamSchemaFull, config.MetaParamSchemaOpaque:
		// recognized — keep as-is
	default:
		active = config.MetaParamSchemaOpaque
	}
	fmt.Printf("  Active mode (env): %s\n\n", active)
	fmt.Printf("  %-12s %12s\n", "mode", "total bytes")
	fmt.Printf("  %-12s %12s\n", strings.Repeat("-", 12), strings.Repeat("-", 12))
	for _, mode := range []string{"opaque", "compact", "full"} {
		tools.SetMetaParamSchema(mode)
		fmt.Printf("  %-12s %12d\n", mode, totalInputSchemaBytes(listServerTools(client, true, true)))
	}
	tools.SetMetaParamSchema("opaque")
}

// totalInputSchemaBytes sums the serialized InputSchema size of the listed
// tools. A tool without a schema, or whose schema does not serialize, adds
// nothing rather than aborting the sizing table.
func totalInputSchemaBytes(listed []*mcp.Tool) int {
	total := 0
	for _, t := range listed {
		if t.InputSchema == nil {
			continue
		}
		raw, err := json.Marshal(t.InputSchema)
		if err != nil {
			continue
		}
		total += len(raw)
	}
	return total
}

// printDomainTable prints the top 20 tool domains sorted by count.
func printDomainTable(domains map[string]int) {
	type kv struct {
		key string
		val int
	}
	var sorted []kv
	for k, v := range domains {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].val != sorted[j].val {
			return sorted[i].val > sorted[j].val
		}
		return sorted[i].key < sorted[j].key
	})
	limit := min(topDomains, len(sorted))
	fmt.Printf("  %-25s %s\n", "Domain", "Tools")
	fmt.Printf("  %-25s %s\n", strings.Repeat("-", 25), strings.Repeat("-", 5))
	for _, kv := range sorted[:limit] {
		fmt.Printf("  %-25s %d\n", kv.key, kv.val)
	}
	if len(sorted) > limit {
		fmt.Printf("  ... and %d more domains\n", len(sorted)-limit)
	}
}
