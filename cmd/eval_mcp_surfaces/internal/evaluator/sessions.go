package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/completions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/prompts"
	mcpresources "github.com/jmrplens/gitlab-mcp-server/v3/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamiccatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/toolvisibility"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// deploymentFacts records what the deployment a catalog was built for actually
// resolved to, as opposed to what the command line asked for: the licensing
// tier the client settled on, the scopes the credential carries and the
// version the instance answered with. Two runs of the same cases against
// different deployments are not comparable, and nothing in a report says so
// unless the header does, so these travel from the client that resolved them
// to [writeReportHeader].
type deploymentFacts struct {
	// Resolved is false when no GitLab client was built at all, which is the
	// --tools-file case: the catalog came out of a snapshot and carries no
	// deployment with it. The header then reports every field as unknown
	// rather than as the Free tier the zero value of [edition.Tier] would
	// otherwise claim.
	Resolved bool
	// Enterprise is whether the catalog holds Enterprise routes. For a client
	// it follows the resolved tier; for a snapshot it is read from the routes
	// the snapshot carries, which is the only source there is for one.
	Enterprise bool
	Tier       edition.Tier
	// TokenScopes is nil when scope detection was disabled or unavailable,
	// which every reader of it treats as "every tool", as the server does.
	TokenScopes   []string
	GitLabVersion string
}

// deploymentFactsForClient reads off a client what can be known from it
// without asking GitLab anything: the tier it resolved and whether that tier
// is an enterprise one. The instance version and the credential's scopes come
// from the two calls [newCatalogGitLabClient] already makes and stay empty
// here, so a caller holding only a client gets a catalog filtered exactly as
// it was before scopes were detected at all.
func deploymentFactsForClient(client *gitlabclient.Client) deploymentFacts {
	if client == nil {
		return deploymentFacts{}
	}
	return deploymentFacts{Resolved: true, Enterprise: client.IsEnterprise(), Tier: client.Tier()}
}

// tierLabel names the tier the catalog was built for, and nothing at all for a
// deployment that was never resolved: the zero value of [edition.Tier] is
// Free, so an unresolved deployment reported through it would name a tier the
// run never asked any instance about.
func (d deploymentFacts) tierLabel() string {
	if !d.Resolved {
		return ""
	}
	return d.Tier.String()
}

// tokenScopesLabel lists the credential's scopes. Empty means detection was
// disabled or unavailable, which the catalog filter reads as "every tool" and
// the header reports as unknown, since that is what it is: nothing here can
// tell a token with no scopes from a token whose scopes nothing asked for.
func (d deploymentFacts) tokenScopesLabel() string {
	if len(d.TokenScopes) == 0 {
		return ""
	}
	return strings.Join(d.TokenScopes, ", ")
}

// mockGitLabVersion is the version the offline catalog backend answers with,
// and so the version a mock-backed report records.
const mockGitLabVersion = "17.0.0"

func newMockGitLabClient() (*gitlabclient.Client, deploymentFacts, func(), error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"version":%q}`, mockGitLabVersion)
	}))
	cfg := &config.Config{GitLabURL: srv.URL, GitLabToken: "eval-token", Tier: edition.Ultimate, TierExplicit: true}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		srv.Close()
		return nil, deploymentFacts{}, nil, fmt.Errorf("client: %w", err)
	}
	facts := deploymentFactsForClient(client)
	facts.GitLabVersion = mockGitLabVersion
	return client, facts, srv.Close, nil
}

// loadCatalog loads catalog from evaluator inputs, along with what the
// deployment it was built for resolved to.
func loadCatalog(opts options) (catalog []modelTool, routes map[string]toolutil.ActionMap, facts deploymentFacts, err error) {
	if opts.ToolsFile != "" {
		snapshotTools, snapshotRoutes, snapshotErr := loadToolsSnapshot(opts.ToolsFile)
		return snapshotTools, snapshotRoutes, deploymentFacts{Enterprise: catalogHasEnterpriseRoutes(snapshotRoutes)}, snapshotErr
	}
	toolSurface, err := normalizeEvalToolSurface(opts.ToolSurface)
	if err != nil {
		return nil, nil, deploymentFacts{}, err
	}
	client, facts, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, nil, deploymentFacts{}, err
	}
	defer cleanup()
	mcpTools, routes, err := buildCatalog(client, facts, toolSurface, opts.ServerMode)
	if err != nil {
		return nil, nil, deploymentFacts{}, err
	}
	return convertTools(mcpTools), routes, facts, nil
}

// newCatalogGitLabClient derives new catalog GitLab client from catalog
// metadata, and reports what the instance behind it resolved to.
//
// The version comes from the ping the client already has to make, and the
// credential's scopes from the same detection cmd/server runs at startup,
// which GITLAB_MCP_IGNORE_SCOPES turns off there and here alike. Both are
// facts about the catalog a run evaluated: the scopes decide which groups are
// in it at all, and a report that named neither could not be told apart from
// a report of a different deployment.
func newCatalogGitLabClient(opts options) (*gitlabclient.Client, deploymentFacts, func(), error) {
	switch normalizedBackend(opts.Backend) {
	case backendMock:
		return newMockGitLabClient()
	case backendGitLab:
		cfg, err := config.Load()
		if err != nil {
			return nil, deploymentFacts{}, nil, fmt.Errorf("load GitLab config: %w", err)
		}
		client, err := gitlabclient.NewClient(cfg)
		if err != nil {
			return nil, deploymentFacts{}, nil, fmt.Errorf("client: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		version, pingErr := client.Ping(ctx)
		if pingErr != nil {
			return nil, deploymentFacts{}, nil, fmt.Errorf("ping GitLab backend %s: %w", cfg.GitLabURL, pingErr)
		}
		if cfg.TierExplicit {
			client.SetTier(cfg.Tier)
		} else {
			client.DetectTier(ctx)
		}
		facts := deploymentFactsForClient(client)
		facts.GitLabVersion = version
		if !cfg.IgnoreScopes {
			facts.TokenScopes = gitlabclient.DetectScopes(ctx, client.GL())
		}
		return client, facts, func() {
			// GitLab catalog clients do not own an httptest server or other local resource.
		}, nil
	default:
		return nil, deploymentFacts{}, nil, fmt.Errorf("unknown backend %q (valid: %s, %s)", opts.Backend, backendMock, backendGitLab)
	}
}

// runMCPSmoke runs MCP smoke for the evaluator package.
func runMCPSmoke(opts options) error {
	if opts.ToolsFile != "" {
		return errors.New("--mcp-smoke requires a live catalog, not --tools-file")
	}
	if normalizedBackend(opts.Backend) != backendGitLab {
		return errors.New("--mcp-smoke requires --backend=gitlab")
	}
	client, facts, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return err
	}
	defer cleanup()
	session, closeSession, err := newCatalogSession(client, facts, opts.ToolSurface, opts.ServerMode)
	if err != nil {
		return err
	}
	defer closeSession()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	toolName := "gitlab"
	arguments := map[string]any{
		"action": "user.current",
		"params": map[string]any{},
	}
	if isDynamicEvalSurface(opts.ToolSurface) {
		toolName = dynamicExecuteActionTool
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		return fmt.Errorf("mcp smoke %s/user.current: %w", toolName, err)
	}
	if result != nil && result.IsError {
		return fmt.Errorf("mcp smoke %s/user.current: %s", toolName, callToolResultText(result))
	}
	terminalPrintf("mcp-smoke: %s/user.current succeeded against GitLab backend\n", toolName)
	return nil
}

// newExecutionSession constructs execution session.
func newExecutionSession(opts options) (*mcp.ClientSession, *gitlabclient.Client, func(), error) {
	if err := validateExecutionOptions(opts); err != nil {
		return nil, nil, nil, err
	}
	if strings.TrimSpace(opts.MCPCommand) != "" {
		session, cleanup, err := newExternalExecutionSession(opts)
		return session, nil, cleanup, err
	}
	client, facts, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, nil, nil, err
	}
	session, closeSession, err := newCatalogSession(client, facts, opts.ToolSurface, opts.ServerMode)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	return session, client, func() {
		closeSession()
		cleanup()
	}, nil
}

// newResourceLookupSession constructs a read-only MCP session for resource bridge tools.
func newResourceLookupSession(opts options) (*mcp.ClientSession, func(), error) {
	client, facts, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, nil, err
	}
	session, closeSession, err := newCatalogSession(client, facts, opts.ToolSurface, opts.ServerMode)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return session, func() {
		closeSession()
		cleanup()
	}, nil
}

func probeCapabilityBridgeSupport(session *mcp.ClientSession) mcpBridgeSupport {
	var support mcpBridgeSupport
	if session == nil {
		return support
	}
	initResult := session.InitializeResult()
	if initResult == nil || initResult.Capabilities == nil {
		return support
	}
	support.Capabilities = true
	support.Completion = initResult.Capabilities.Completions != nil
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if initResult.Capabilities.Resources != nil {
		if _, resourcesErr := session.ListResources(ctx, nil); resourcesErr == nil {
			if _, templatesErr := session.ListResourceTemplates(ctx, nil); templatesErr == nil {
				support.Resources = true
			}
		}
	}
	if initResult.Capabilities.Prompts != nil {
		if _, err := session.ListPrompts(ctx, nil); err == nil {
			support.Prompts = true
		}
	}
	return support
}

// validateExecutionOptions validates execution options for the evaluator package.
func validateExecutionOptions(opts options) error {
	if strings.TrimSpace(opts.MCPCommand) != "" {
		if opts.ToolsFile == "" {
			return errors.New("--execute-tools with --mcp-command requires --tools-file from the same target catalog")
		}
		if !opts.AllowLive && !dockerModeEnabled(opts.MCPEnv) {
			return errors.New("--execute-tools with --mcp-command requires E2E_MODE=docker in the environment or --mcp-env-file unless --allow-live-mutations is set")
		}
		return nil
	}
	if opts.ToolsFile != "" {
		return errors.New("--execute-tools requires a live catalog, not --tools-file")
	}
	if normalizedBackend(opts.Backend) != backendGitLab {
		return errors.New("--execute-tools requires --backend=gitlab")
	}
	if !opts.AllowLive && !strings.EqualFold(os.Getenv("E2E_MODE"), "docker") {
		return errors.New("--execute-tools requires E2E_MODE=docker unless --allow-live-mutations is set")
	}
	return nil
}

// newExternalExecutionSession constructs external execution session.
func newExternalExecutionSession(opts options) (*mcp.ClientSession, func(), error) {
	cmd := exec.CommandContext(context.Background(), opts.MCPCommand, []string(opts.MCPArgs)...) // #nosec G204 -- explicit developer-provided MCP server command for version comparison.
	env, err := externalMCPEnv(opts)
	if err != nil {
		return nil, nil, err
	}
	cmd.Env = env
	transport := &mcp.CommandTransport{Command: cmd, TerminateDuration: 5 * time.Second}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "eval-mcp-surfaces-external-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: evalElicitationHandler,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("connect external MCP server: %w", err)
	}
	return session, func() { _ = session.Close() }, nil
}

func externalMCPEnv(opts options) ([]string, error) {
	env := os.Environ()
	if strings.TrimSpace(opts.MCPEnv) == "" {
		return env, nil
	}
	values, err := godotenv.Read(opts.MCPEnv)
	if err != nil {
		return nil, fmt.Errorf("load mcp env file %s: %w", opts.MCPEnv, err)
	}
	for key, value := range values {
		replaced := false
		prefix := key + "="
		for i, entry := range env {
			if strings.HasPrefix(entry, prefix) {
				env[i] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}
	return env, nil
}

// dockerModeEnabled reports whether the evaluation environment targets Docker fixtures.
func dockerModeEnabled(envFile string) bool {
	if strings.EqualFold(os.Getenv("E2E_MODE"), "docker") {
		return true
	}
	if strings.TrimSpace(envFile) == "" {
		return false
	}
	values, err := godotenv.Read(envFile)
	if err != nil {
		return false
	}
	return strings.EqualFold(values["E2E_MODE"], "docker")
}

// callToolResultText resolves call tool result text for evaluator execution.
func callToolResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return "empty error result"
	}
	if text, ok := result.Content[0].(*mcp.TextContent); ok {
		return text.Text
	}
	return fmt.Sprintf("error result with first content type %T", result.Content[0])
}

// toolResultContent converts the GitLab API response to the tool output format.
func toolResultContent(result *mcp.CallToolResult) string {
	if result == nil {
		return "empty result"
	}
	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err == nil {
			return truncateToolResult(string(data))
		}
	}
	return toolResultTextContent(result)
}

func toolResultContentForTool(toolName string, result *mcp.CallToolResult) string {
	if toolName == dynamicFindTool {
		return toolResultTextContent(result)
	}
	return toolResultContent(result)
}

func toolResultTextContent(result *mcp.CallToolResult) string {
	if result == nil {
		return "empty result"
	}
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && strings.TrimSpace(text.Text) != "" {
			parts = append(parts, text.Text)
		}
	}
	if len(parts) == 0 {
		return "ok"
	}
	return truncateToolResult(strings.Join(parts, "\n"))
}

// truncateToolResult resolves truncate tool result for evaluator execution.
func truncateToolResult(content string) string {
	if len(content) <= maxToolResultLen {
		return content
	}
	return content[:maxToolResultLen] + "\n...[truncated]"
}

// loadToolsSnapshot loads tools snapshot from evaluator inputs.
func loadToolsSnapshot(path string) ([]modelTool, map[string]toolutil.ActionMap, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- tools snapshot path is an explicit evaluator input.
	if err != nil {
		return nil, nil, fmt.Errorf("read tools snapshot: %w", err)
	}
	snapshot, err := parseToolsSnapshot(data)
	if err != nil {
		return nil, nil, err
	}
	return convertSnapshotTools(snapshot), routesFromSnapshot(snapshot), nil
}

// parseToolsSnapshot handles parse tools snapshot and returns [[]snapshotTool].
func parseToolsSnapshot(data []byte) ([]snapshotTool, error) {
	var snapshot []snapshotTool
	if err := json.Unmarshal(data, &snapshot); err == nil {
		return snapshot, nil
	}
	var wrapped struct {
		Tools []snapshotTool `json:"tools"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("decode tools snapshot: %w", err)
	}
	return wrapped.Tools, nil
}

// buildCatalog constructs the request parameters from the input.
func buildCatalog(client *gitlabclient.Client, facts deploymentFacts, toolSurface, serverMode string) ([]*mcp.Tool, map[string]toolutil.ActionMap, error) {
	_, closeSession, toolsResult, routes, err := buildCatalogSession(client, facts, toolSurface, serverMode)
	if closeSession != nil {
		defer closeSession()
	}
	if err != nil {
		return nil, nil, err
	}
	return toolsResult, routes, nil
}

// evalSchemaCache caches resolved tool schemas across every catalog
// session; see the SchemaCache option below.
var evalSchemaCache = mcp.NewSchemaCache()

// newCatalogSession constructs catalog session.
func newCatalogSession(client *gitlabclient.Client, facts deploymentFacts, toolSurface, serverMode string) (*mcp.ClientSession, func(), error) {
	session, closeSession, _, _, err := buildCatalogSession(client, facts, toolSurface, serverMode)
	return session, closeSession, err
}

// evalServerConfig describes the deployment the evaluated catalog is assembled
// for, so the assemblers the server itself uses can be handed the same
// configuration a server would receive.
//
// Every field is passed through rather than restated. The tier used to be
// collapsed through [edition.TierForEnterprise], which reads a resolved tier
// back as the binary "is this an enterprise instance" question and answers
// Ultimate to it: a Premium instance was evaluated against the Ultimate
// catalog, so a report could name a tier the run never served. The scopes are
// what [tools.FilterActionCatalog] narrows the catalog by, and leaving them
// nil evaluated a wider surface than the credential would have been served.
// GITLAB_MCP_READ_ONLY and GITLAB_MCP_SAFE_MODE both act per action, so
// evaluating either means evaluating a different catalog, not a different
// client.
func evalServerConfig(facts deploymentFacts, serverMode string) *config.ServerConfig {
	return &config.ServerConfig{
		Tier:        facts.Tier,
		TokenScopes: facts.TokenScopes,
		ReadOnly:    serverMode == ServerModeReadOnly,
		SafeMode:    serverMode == ServerModeSafe,
	}
}

// Test seams, the pair cmd/server keeps for the same two assemblers. Neither
// can fail from any input the evaluator takes: both assemble the ActionSpecs
// compiled into this binary, and the filter they apply adds groups the base
// catalog already validated. The branches reporting their failure exist for
// the day one of those facts changes, and would otherwise never run.
var (
	buildDynamicCatalog = dynamiccatalog.Build
	sharedMetaCatalog   = tools.SharedMetaCatalog
)

// buildCatalogSession constructs the request parameters from the input.
func buildCatalogSession(client *gitlabclient.Client, facts deploymentFacts, toolSurface, serverMode string) (session *mcp.ClientSession, closeSession func(), mcpTools []*mcp.Tool, routes map[string]toolutil.ActionMap, err error) {
	completionHandler := completions.NewHandler(client)
	server := mcp.NewServer(&mcp.Implementation{Name: "eval-mcp-surfaces", Version: "0.0.1"}, &mcp.ServerOptions{
		PageSize: 2000,
		// Shared across every catalog session this process builds: resolved
		// tool schemas depend only on the compiled-in catalog, and reusing
		// them skips schema resolution on every registration after the
		// first (the same cache cmd/server shares across its pool).
		SchemaCache: evalSchemaCache,
		Capabilities: &mcp.ServerCapabilities{
			Tools:     &mcp.ToolCapabilities{ListChanged: true},
			Resources: &mcp.ResourceCapabilities{ListChanged: true},
			Prompts:   &mcp.PromptCapabilities{ListChanged: true},
		},
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return completionHandler.Complete(ctx, req)
		},
	})
	cfg := evalServerConfig(facts, serverMode)
	var surfaceCatalog *actioncatalog.Catalog
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		// dynamiccatalog.Build is the assembler cmd/server uses, in the order
		// it uses it: the filters run before the standalone actions join, so
		// no narrowed action is left behind them, and safe-mode previews come
		// last over the whole catalog, so a standalone write is previewed like
		// any other. Assembling an equivalent catalog here was what the e2e
		// suite did before this package existed, and a test that builds its
		// own copy of the thing under test is testing the copy.
		actionCatalog, withheld, catalogErr := buildDynamicCatalog(client, cfg)
		if catalogErr != nil {
			return nil, nil, nil, nil, fmt.Errorf(errBuildActionCatalog, catalogErr)
		}
		surfaceCatalog = actionCatalog
		// The bookkeeping is the point of building it this way: without it the
		// evaluated model reads a narrowed action as absent rather than as
		// withheld, and scores a capability the deployment has.
		dynamictools.RegisterCatalogFindExecuteTools(server, actionCatalog,
			dynamictools.WithWithheldActions(withheld.ByTokenScope, withheld.ByOperator))
		routes = dynamicValidationRoutes(actionCatalog.ActionMaps())
	case config.ToolSurfaceMeta:
		filteredCatalog, _, catalogErr := sharedMetaCatalog(client, cfg)
		if catalogErr != nil {
			return nil, nil, nil, nil, fmt.Errorf(errBuildActionCatalog, catalogErr)
		}
		surfaceCatalog = filteredCatalog
		tools.RegisterMetaCatalog(server, filteredCatalog)
		tools.RegisterMetaStandaloneTools(server, client)
		routes = filteredCatalog.ActionMaps()
	default:
		return nil, nil, nil, nil, fmt.Errorf("unsupported tool surface %q", toolSurface)
	}
	// The pass cmd/server runs after registration, over the tools registered
	// outside the catalog: on the meta surface those are the
	// gitlab_interactive_* flows, which read-only mode withdraws and safe
	// mode previews. Without it a protective meta evaluation scored a surface
	// on which the flows kept their real handlers, and a model could create
	// the issue the product would have refused.
	toolvisibility.Apply(context.Background(), server, cfg, toolSurface, surfaceCatalog)
	toolutil.LockdownInputSchemas(server)
	toolutil.EnrichPaginationConstraints(server)
	mcpTools, err = inspectEvalTools(server)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	registerEvalResources(server, client, toolSurface, surfaceCatalog, routes, mcpTools)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, serverErr := server.Connect(ctx, st, nil)
	if serverErr != nil {
		return nil, nil, nil, nil, fmt.Errorf("server connect: %w", serverErr)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "eval-mcp-surfaces-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: evalElicitationHandler,
	})
	session, err = mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		_ = serverSession.Close()
		return nil, nil, nil, nil, fmt.Errorf("client connect: %w", err)
	}
	return session, func() {
		_ = session.Close()
		_ = serverSession.Close()
	}, mcpTools, routes, nil
}

// inspectEvalTools returns the tool list before evaluator resources are attached.
func inspectEvalTools(server *mcp.Server) ([]*mcp.Tool, error) {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return nil, fmt.Errorf("server connect: %w", err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "eval-mcp-surfaces-inspector", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return result.Tools, nil
}

// registerEvalResources mirrors the default full resource and prompt capability surface.
func registerEvalResources(server *mcp.Server, client *gitlabclient.Client, toolSurface string, catalog *actioncatalog.Catalog, routes map[string]toolutil.ActionMap, toolList []*mcp.Tool) {
	mcpresources.Register(server, client)
	mcpresources.RegisterWorkflowGuides(server)
	prompts.Register(server, client)
	mcpresources.RegisterToolSurfaceResources(server, mcpresources.ToolSurfaceResourceOptions{
		Surface:    toolSurface,
		Tools:      toolList,
		Catalog:    catalog,
		MetaRoutes: routes,
	})
}

// dynamicValidationRoutes converts action routes into the single
// gitlab_execute_action action namespace used by dynamic mode.
func dynamicValidationRoutes(catalogRoutes map[string]toolutil.ActionMap) map[string]toolutil.ActionMap {
	executeRoutes := make(toolutil.ActionMap)
	for toolName, actions := range catalogRoutes {
		for action, route := range actions {
			executeRoutes[dynamicActionID(toolName, action)] = route
		}
	}
	return map[string]toolutil.ActionMap{dynamicExecuteActionTool: executeRoutes}
}

// dynamicActionID returns the canonical dynamic action ID for a catalog route.
func dynamicActionID(toolName, action string) string {
	return strings.TrimPrefix(toolName, "gitlab_") + "." + action
}
