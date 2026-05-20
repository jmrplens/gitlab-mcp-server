// Command gen_readme auto-generates the managed README.md sections.
// It creates an in-memory MCP server, measures the README token-footprint
// configurations, collects filesystem-level codebase metrics, and replaces
// content between the token footprint and statistics marker pairs.
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
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/docgen"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/roots"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// README generation markers define managed sections and measurement constants.
const (
	footprintStartMarker = "<!-- START TOKEN FOOTPRINT -->"
	footprintEndMarker   = "<!-- END TOKEN FOOTPRINT -->"
	readmePath           = "README.md"
	repoRoot             = "."
	readmeServerName     = "gen-readme"
	readmeClientName     = "gen-readme-client"
	readmeVersion        = "0.0.1"
	readmeBytesPerToken  = 4
)

// main regenerates the README managed sections and exits non-zero on failure.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run introspects the base catalog and replaces README sections with fresh
// token footprint and repository statistics content.
func run() error {
	client, closeClient, err := newReadmeClient()
	if err != nil {
		return err
	}
	defer closeClient()

	schemaMode, err := readMetaParamSchemaMode()
	if err != nil {
		return err
	}

	tokenFootprint, err := buildTokenFootprint(client, schemaMode)
	if err != nil {
		return fmt.Errorf("building token footprint: %w", err)
	}
	if replaceErr := replaceSection(readmePath, footprintStartMarker, footprintEndMarker, tokenFootprint); replaceErr != nil {
		return replaceErr
	}

	stats, statsErr := collectStats(repoRoot)
	if statsErr != nil {
		return fmt.Errorf("collecting stats: %w", statsErr)
	}
	if replaceErr := replaceSection(readmePath, statsStartMarker, statsEndMarker, renderStats(stats)); replaceErr != nil {
		return replaceErr
	}

	fmt.Printf("Updated %s (token footprint using META_PARAM_SCHEMA=%s, stats regenerated)\n", readmePath, schemaMode)
	return nil
}

func newReadmeClient() (*gitlabclient.Client, func(), error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))

	cfg := &config.Config{ //#nosec G101 -- not a real credential, test-only dummy token
		GitLabURL:   srv.URL,
		GitLabToken: "gen-readme-token",
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		srv.Close()
		return nil, nil, fmt.Errorf("create client: %w", err)
	}
	return client, srv.Close, nil
}

func readMetaParamSchemaMode() (string, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("META_PARAM_SCHEMA")))
	if mode == "" {
		return config.DefaultMetaParamSchema, nil
	}
	switch mode {
	case config.MetaParamSchemaOpaque, config.MetaParamSchemaCompact, config.MetaParamSchemaFull:
		return mode, nil
	default:
		return "", fmt.Errorf("META_PARAM_SCHEMA must be one of %q, %q, or %q (got %q)",
			config.MetaParamSchemaOpaque, config.MetaParamSchemaCompact, config.MetaParamSchemaFull, mode)
	}
}

// tokenFootprintRow is a README-facing token measurement for one runtime
// configuration.
type tokenFootprintRow struct {
	Configuration    string
	MetaParamSchema  string
	VisibleTools     int
	ReachableActions int
	ToolSchemaTokens int
	SharedTokens     int
}

// resourceRegistrationOptions selects which MCP resource groups are advertised
// for README token-footprint measurements.
type resourceRegistrationOptions struct {
	Core           bool
	ToolManifest   bool
	ToolSurface    string
	ToolList       []*mcp.Tool
	ToolCatalog    *actioncatalog.Catalog
	WorkflowGuides bool
	WorkspaceRoots bool
}

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

func buildTokenFootprint(client *gitlabclient.Client, schemaMode string) (string, error) {
	rows, err := measureTokenFootprintRows(client, schemaMode)
	if err != nil {
		return "", err
	}
	return renderTokenFootprint(schemaMode, rows), nil
}

func measureTokenFootprintRows(client *gitlabclient.Client, schemaMode string) ([]tokenFootprintRow, error) {
	restoreSchemaMode := gitlabtools.SetMetaParamSchemaScoped(schemaMode)
	defer restoreSchemaMode()

	metaCatalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{
		Enterprise: false,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build base action catalog: %w", err)
	}

	dynamicCatalog, err := dynamictools.AddStandaloneCatalog(metaCatalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add standalone dynamic catalog: %w", err)
	}
	metaRoutes := metaCatalog.ActionMaps()
	dynamicRoutes := dynamicCatalog.ActionMaps()
	reachableActions := countActions(dynamicRoutes)

	dynamicTools, err := listDynamicTools(dynamicCatalog)
	if err != nil {
		return nil, err
	}
	metaTools, err := listMetaToolsFromCatalog(client, metaCatalog)
	if err != nil {
		return nil, err
	}
	individualTools, err := listIndividualTools(client)
	if err != nil {
		return nil, err
	}

	dynamicToolTokens, err := measureToolSchemaTokens(dynamicTools)
	if err != nil {
		return nil, err
	}
	metaToolTokens, err := measureToolSchemaTokens(metaTools)
	if err != nil {
		return nil, err
	}
	individualToolTokens, err := measureToolSchemaTokens(individualTools)
	if err != nil {
		return nil, err
	}
	promptTokens, err := measurePrompts(client)
	if err != nil {
		return nil, err
	}

	dynamicFullShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes:            dynamicRoutes,
		ToolCatalog:       dynamicCatalog,
		ToolList:          dynamicTools,
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
		PromptTokens:      promptTokens,
	})
	if err != nil {
		return nil, err
	}
	dynamicMinimalShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes:            dynamicRoutes,
		ToolCatalog:       dynamicCatalog,
		ToolList:          dynamicTools,
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceMinimal,
		PromptTokens:      promptTokens,
	})
	if err != nil {
		return nil, err
	}
	metaFullShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes:            metaRoutes,
		ToolCatalog:       metaCatalog,
		ToolList:          metaTools,
		ToolSurface:       config.ToolSurfaceMeta,
		CapabilitySurface: config.CapabilitySurfaceFull,
		PromptTokens:      promptTokens,
	})
	if err != nil {
		return nil, err
	}
	metaMinimalShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		Routes:            metaRoutes,
		ToolCatalog:       metaCatalog,
		ToolList:          metaTools,
		ToolSurface:       config.ToolSurfaceMeta,
		CapabilitySurface: config.CapabilitySurfaceMinimal,
		PromptTokens:      promptTokens,
	})
	if err != nil {
		return nil, err
	}
	individualFullShared, err := measureSharedTokens(client, sharedTokenMeasureOptions{
		ToolList:          individualTools,
		ToolSurface:       config.ToolSurfaceIndividual,
		CapabilitySurface: config.CapabilitySurfaceFull,
		PromptTokens:      promptTokens,
	})
	if err != nil {
		return nil, err
	}

	return []tokenFootprintRow{
		{
			Configuration:    "`dynamic` / `full` (default)",
			VisibleTools:     len(dynamicTools),
			ReachableActions: reachableActions,
			ToolSchemaTokens: dynamicToolTokens,
			SharedTokens:     dynamicFullShared,
		},
		{
			Configuration:    "`dynamic` / `minimal`",
			VisibleTools:     len(dynamicTools),
			ReachableActions: reachableActions,
			ToolSchemaTokens: dynamicToolTokens,
			SharedTokens:     dynamicMinimalShared,
		},
		{
			Configuration:    "`meta` / `full`",
			MetaParamSchema:  schemaMode,
			VisibleTools:     len(metaTools),
			ReachableActions: reachableActions,
			ToolSchemaTokens: metaToolTokens,
			SharedTokens:     metaFullShared,
		},
		{
			Configuration:    "`meta` / `minimal`",
			MetaParamSchema:  schemaMode,
			VisibleTools:     len(metaTools),
			ReachableActions: reachableActions,
			ToolSchemaTokens: metaToolTokens,
			SharedTokens:     metaMinimalShared,
		},
		{
			Configuration:    "`individual` / `full`",
			VisibleTools:     len(individualTools),
			ReachableActions: len(individualTools),
			ToolSchemaTokens: individualToolTokens,
			SharedTokens:     individualFullShared,
		},
	}, nil
}

func renderTokenFootprint(schemaMode string, rows []tokenFootprintRow) string {
	var b strings.Builder
	b.WriteString("Measured with `go run ./cmd/gen_readme/` against the current base catalog. Totals estimate startup context visible to an MCP client: visible tool schemas plus shared resources and prompts, using the same byte/4 token heuristic as `cmd/audit_tokens`.\n\n")
	b.WriteString("**Default configuration**: with `TOOL_SURFACE` unset or `TOOL_SURFACE=dynamic`, `CAPABILITY_SURFACE=full`, `META_TOOLS` unset, `META_PARAM_SCHEMA=opaque`, and `GITLAB_ENTERPRISE` unset or `false`, the server uses the **dynamic find/execute surface**. Use `TOOL_SURFACE=meta` only when you explicitly want domain meta-tools; use `TOOL_SURFACE=individual` only when your client can handle the full tool catalog.\n\n")

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		schemaCell := "n/a"
		if row.MetaParamSchema != "" {
			schemaCell = fmt.Sprintf("`%s`", row.MetaParamSchema)
		}
		tableRows = append(tableRows, []string{
			row.Configuration,
			fmtNum(row.VisibleTools),
			fmtNum(row.ReachableActions),
			schemaCell,
			fmtNum(row.ToolSchemaTokens),
			fmtNum(row.SharedTokens),
			fmtNum(row.totalTokens()),
		})
	}
	b.WriteString(docgen.RenderMarkdownTable(
		[]string{"Configuration (`TOOL_SURFACE` / `CAPABILITY_SURFACE`)", "Visible tools", "Reachable actions", "`META_PARAM_SCHEMA`", "Tool schema tokens", "Shared tokens", "Total tokens"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight},
		tableRows,
	))

	fmt.Fprintf(&b, "\nRows use the base Community Edition catalog (`GITLAB_ENTERPRISE=false`). `META_PARAM_SCHEMA=%s` affects only visible meta-tool input schemas; dynamic mode gets exact action schemas from `gitlab_find_action`, and every surface advertises `gitlab://tools` plus `gitlab://tools/{id}` for on-demand action browsing and input schemas. Individual mode already exposes one schema per tool.\n", schemaMode)
	return b.String()
}

func listMetaToolsFromCatalog(client *gitlabclient.Client, catalog *actioncatalog.Catalog) ([]*mcp.Tool, error) {
	server := newReadmeMCPServer()
	gitlabtools.RegisterMetaCatalog(server, catalog)
	gitlabtools.RegisterMetaStandaloneTools(server, client)
	return listToolsFromServer(server)
}

func listIndividualTools(client *gitlabclient.Client) ([]*mcp.Tool, error) {
	server := newReadmeMCPServer()
	gitlabtools.RegisterAll(server, client, false)
	return listToolsFromServer(server)
}

func listDynamicTools(catalog *actioncatalog.Catalog) ([]*mcp.Tool, error) {
	server := newReadmeMCPServer()
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return listToolsFromServer(server)
}

func newReadmeMCPServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: readmeServerName, Version: readmeVersion}, &mcp.ServerOptions{PageSize: 2000})
}

func listToolsFromServer(server *mcp.Server) ([]*mcp.Tool, error) {
	return withReadmeSession(server, "tools", func(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
		result, err := session.ListTools(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		return result.Tools, nil
	})
}

func measureToolSchemaTokens(toolList []*mcp.Tool) (int, error) {
	totalBytes := 0
	for _, t := range toolList {
		b, err := json.Marshal(t)
		if err != nil {
			return 0, fmt.Errorf("marshal tool %s: %w", t.Name, err)
		}
		totalBytes += len(b)
	}
	return totalBytes / readmeBytesPerToken, nil
}

func measureSharedTokens(client *gitlabclient.Client, opts sharedTokenMeasureOptions) (int, error) {
	resourceTokens, err := measureResourcesWithOptions(client, opts.Routes, resourceRegistrationOptions{
		Core:           opts.CapabilitySurface == config.CapabilitySurfaceFull,
		ToolManifest:   true,
		ToolSurface:    opts.ToolSurface,
		ToolList:       opts.ToolList,
		ToolCatalog:    opts.ToolCatalog,
		WorkflowGuides: opts.CapabilitySurface == config.CapabilitySurfaceFull,
		WorkspaceRoots: true,
	})
	if err != nil {
		return 0, err
	}
	if opts.CapabilitySurface == config.CapabilitySurfaceFull {
		return resourceTokens + opts.PromptTokens, nil
	}
	return resourceTokens, nil
}

func measureResourcesWithOptions(client *gitlabclient.Client, routes map[string]toolutil.ActionMap, opts resourceRegistrationOptions) (int, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: readmeServerName, Version: readmeVersion}, nil)
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
	if opts.WorkspaceRoots {
		resources.RegisterWorkspaceRoots(server, roots.NewManager())
	}
	if opts.WorkflowGuides {
		resources.RegisterWorkflowGuides(server)
	}

	return withReadmeSession(server, "resources", func(ctx context.Context, session *mcp.ClientSession) (int, error) {
		totalBytes := 0
		res, err := session.ListResources(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("list resources: %w", err)
		}
		for _, r := range res.Resources {
			b, mErr := json.Marshal(r)
			if mErr != nil {
				return 0, fmt.Errorf("marshal resource %s: %w", r.Name, mErr)
			}
			totalBytes += len(b)
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
			totalBytes += len(b)
		}
		return totalBytes / readmeBytesPerToken, nil
	})
}

func measurePrompts(client *gitlabclient.Client) (int, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: readmeServerName, Version: readmeVersion}, nil)
	prompts.Register(server, client)

	return withReadmeSession(server, "prompts", func(ctx context.Context, session *mcp.ClientSession) (int, error) {
		totalBytes := 0
		promptList, err := session.ListPrompts(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("list prompts: %w", err)
		}
		for _, pr := range promptList.Prompts {
			b, mErr := json.Marshal(pr)
			if mErr != nil {
				return 0, fmt.Errorf("marshal prompt %s: %w", pr.Name, mErr)
			}
			totalBytes += len(b)
		}
		return totalBytes / readmeBytesPerToken, nil
	})
}

func withReadmeSession[T any](server *mcp.Server, label string, fn func(context.Context, *mcp.ClientSession) (T, error)) (T, error) {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	var zero T

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return zero, fmt.Errorf("server connect %s: %w", label, err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: readmeClientName, Version: readmeVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return zero, fmt.Errorf("client connect %s: %w", label, err)
	}
	defer session.Close()

	return fn(ctx, session)
}

// countActions returns the number of actions in a route catalog.
func countActions(routes map[string]toolutil.ActionMap) int {
	total := 0
	for _, actions := range routes {
		total += len(actions)
	}
	return total
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
