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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/completions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	mcpresources "github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func newMockGitLabClient() (*gitlabclient.Client, func(), error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	cfg := &config.Config{GitLabURL: srv.URL, GitLabToken: "eval-token", Enterprise: true}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		srv.Close()
		return nil, nil, fmt.Errorf("client: %w", err)
	}
	return client, srv.Close, nil
}

// loadCatalog loads catalog from evaluator inputs.
func loadCatalog(opts options) (catalog []modelTool, routes map[string]toolutil.ActionMap, enterprise bool, err error) {
	if opts.ToolsFile != "" {
		snapshotTools, snapshotRoutes, snapshotErr := loadToolsSnapshot(opts.ToolsFile)
		return snapshotTools, snapshotRoutes, catalogHasEnterpriseRoutes(snapshotRoutes), snapshotErr
	}
	toolSurface, err := normalizeEvalToolSurface(opts.ToolSurface)
	if err != nil {
		return nil, nil, false, err
	}
	client, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, nil, false, err
	}
	defer cleanup()
	mcpTools, routes, err := buildCatalog(client, toolSurface)
	if err != nil {
		return nil, nil, false, err
	}
	return convertTools(mcpTools), routes, client.IsEnterprise(), nil
}

// newCatalogGitLabClient derives new catalog GitLab client from catalog metadata.
func newCatalogGitLabClient(opts options) (*gitlabclient.Client, func(), error) {
	switch normalizedBackend(opts.Backend) {
	case backendMock:
		return newMockGitLabClient()
	case backendGitLab:
		cfg, err := config.Load()
		if err != nil {
			return nil, nil, fmt.Errorf("load GitLab config: %w", err)
		}
		client, err := gitlabclient.NewClient(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("client: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, pingErr := client.Ping(ctx); pingErr != nil {
			return nil, nil, fmt.Errorf("ping GitLab backend %s: %w", cfg.GitLabURL, pingErr)
		}
		client.DetectEnterprise(ctx, cfg.Enterprise)
		return client, func() {
			// GitLab catalog clients do not own an httptest server or other local resource.
		}, nil
	default:
		return nil, nil, fmt.Errorf("unknown backend %q (valid: %s, %s)", opts.Backend, backendMock, backendGitLab)
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
	client, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return err
	}
	defer cleanup()
	session, closeSession, err := newCatalogSession(client, opts.ToolSurface)
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
	client, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, nil, nil, err
	}
	session, closeSession, err := newCatalogSession(client, opts.ToolSurface)
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
	client, cleanup, err := newCatalogGitLabClient(opts)
	if err != nil {
		return nil, nil, err
	}
	session, closeSession, err := newCatalogSession(client, opts.ToolSurface)
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
		CreateMessageHandler: evalCreateMessageHandler,
		ElicitationHandler:   evalElicitationHandler,
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
func buildCatalog(client *gitlabclient.Client, toolSurface string) ([]*mcp.Tool, map[string]toolutil.ActionMap, error) {
	session, closeSession, toolsResult, routes, err := buildCatalogSession(client, toolSurface)
	if closeSession != nil {
		defer closeSession()
	}
	if err != nil {
		return nil, nil, err
	}
	_ = session
	return toolsResult, routes, nil
}

// newCatalogSession constructs catalog session.
func newCatalogSession(client *gitlabclient.Client, toolSurface string) (*mcp.ClientSession, func(), error) {
	session, closeSession, _, _, err := buildCatalogSession(client, toolSurface)
	return session, closeSession, err
}

// buildCatalogSession constructs the request parameters from the input.
func buildCatalogSession(client *gitlabclient.Client, toolSurface string) (session *mcp.ClientSession, closeSession func(), mcpTools []*mcp.Tool, routes map[string]toolutil.ActionMap, err error) {
	completionHandler := completions.NewHandler(client)
	server := mcp.NewServer(&mcp.Implementation{Name: "eval-mcp-surfaces", Version: "0.0.1"}, &mcp.ServerOptions{
		PageSize: 2000,
		Capabilities: &mcp.ServerCapabilities{
			Logging:   &mcp.LoggingCapabilities{},
			Tools:     &mcp.ToolCapabilities{ListChanged: true},
			Resources: &mcp.ResourceCapabilities{ListChanged: true},
			Prompts:   &mcp.PromptCapabilities{ListChanged: true},
		},
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return completionHandler.Complete(ctx, req)
		},
	})
	var surfaceCatalog *actioncatalog.Catalog
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		actionCatalog, catalogErr := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: client.IsEnterprise(), IncludeMCP: true})
		if catalogErr != nil {
			return nil, nil, nil, nil, fmt.Errorf(errBuildActionCatalog, catalogErr)
		}
		actionCatalog, catalogErr = dynamictools.AddStandaloneCatalog(actionCatalog, client, dynamictools.StandaloneOptions{})
		if catalogErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("add standalone dynamic catalog: %w", catalogErr)
		}
		surfaceCatalog = actionCatalog
		dynamictools.RegisterCatalogFindExecuteTools(server, actionCatalog)
		routes = dynamicValidationRoutes(actionCatalog.ActionMaps())
	case config.ToolSurfaceMeta:
		actionCatalog, catalogErr := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: client.IsEnterprise(), IncludeMCP: true})
		if catalogErr != nil {
			return nil, nil, nil, nil, fmt.Errorf(errBuildActionCatalog, catalogErr)
		}
		surfaceCatalog = actionCatalog
		tools.RegisterMetaCatalog(server, actionCatalog)
		tools.RegisterMetaStandaloneTools(server, client)
		routes = actionCatalog.ActionMaps()
	default:
		return nil, nil, nil, nil, fmt.Errorf("unsupported tool surface %q", toolSurface)
	}
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
		CreateMessageHandler: evalCreateMessageHandler,
		ElicitationHandler:   evalElicitationHandler,
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
	mcpresources.RegisterWorkspaceRoots(server, roots.NewManager())
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

// evalCreateMessageHandler handles eval create message handler and returns [*mcp.CreateMessageResult].
