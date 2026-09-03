package evaluator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestValidateExecutionOptions_RejectsUnsafeCombinations verifies live execution
// cannot accidentally run against a non-Docker GitLab target.
func TestValidateExecutionOptions_RejectsUnsafeCombinations(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	if err := validateExecutionOptions(options{Execute: true, Backend: backendMock}); err == nil || !strings.Contains(err.Error(), "backend=gitlab") {
		t.Fatalf("validateExecutionOptions(mock) error = %v, want backend=gitlab", err)
	}
	if err := validateExecutionOptions(options{Execute: true, Backend: backendGitLab}); err == nil || !strings.Contains(err.Error(), "E2E_MODE=docker") {
		t.Fatalf("validateExecutionOptions(non-docker) error = %v, want Docker guard", err)
	}
	if err := validateExecutionOptions(options{Execute: true, Backend: backendGitLab, AllowLive: true}); err != nil {
		t.Fatalf("validateExecutionOptions(allow live) error = %v", err)
	}
	if err := validateExecutionOptions(options{Execute: true, MCPCommand: "server"}); err == nil || !strings.Contains(err.Error(), "tools-file") {
		t.Fatalf("validateExecutionOptions(external without tools) error = %v, want tools-file", err)
	}
}

// TestExternalMCPEnv_LoadsOverrides verifies external MCP env files replace
// existing variables while preserving the rest of the process environment.
func TestExternalMCPEnv_LoadsOverrides(t *testing.T) {
	t.Setenv("EVAL_MCP_TEST_ENV", "old")
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("EVAL_MCP_TEST_ENV=new\nEVAL_MCP_ADDED=1\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	env, err := externalMCPEnv(options{MCPEnv: envFile})
	if err != nil {
		t.Fatalf("externalMCPEnv() error = %v", err)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	if !strings.Contains(joined, "\nEVAL_MCP_TEST_ENV=new\n") || !strings.Contains(joined, "\nEVAL_MCP_ADDED=1\n") {
		t.Fatalf("env = %s, want overridden and added values", joined)
	}
}

// TestDockerModeEnabled_ReadsEnvironmentAndEnvFile verifies Docker safety checks
// accept either the process environment or an explicit env file.
func TestDockerModeEnabled_ReadsEnvironmentAndEnvFile(t *testing.T) {
	t.Setenv("E2E_MODE", "docker")
	if !dockerModeEnabled("") {
		t.Fatal("dockerModeEnabled(env) = false, want true")
	}
	t.Setenv("E2E_MODE", "")
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("E2E_MODE=docker\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if !dockerModeEnabled(envFile) || dockerModeEnabled(filepath.Join(t.TempDir(), "missing.env")) {
		t.Fatal("dockerModeEnabled(env file) did not respect present and missing files")
	}
}

// TestToolResultContent_HandlesStructuredTextAndEmptyResults verifies MCP result
// rendering prefers structured content and has stable fallbacks.
func TestToolResultContent_HandlesStructuredTextAndEmptyResults(t *testing.T) {
	if got := callToolResultText(nil); got != "empty error result" {
		t.Fatalf("callToolResultText(nil) = %q", got)
	}
	structured := &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	if got := toolResultContent(structured); got != `{"ok":true}` {
		t.Fatalf("toolResultContent(structured) = %q", got)
	}
	if got := toolResultContentForTool("gitlab_project", structured); got != `{"ok":true}` {
		t.Fatalf("toolResultContentForTool(non-find) = %q", got)
	}
	text := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: " one "}, &mcp.TextContent{Text: "two"}}}
	if got := toolResultContent(text); got != " one \ntwo" {
		t.Fatalf("toolResultContent(text) = %q", got)
	}
	find := &mcp.CallToolResult{
		StructuredContent: map[string]any{"results": []any{map[string]any{"id": "project.get", "schema": strings.Repeat("x", maxToolResultLen)}}},
		Content:           []mcp.Content{&mcp.TextContent{Text: "compact result for `project.get`"}},
	}
	if got := toolResultContentForTool(dynamicFindTool, find); got != "compact result for `project.get`" {
		t.Fatalf("toolResultContentForTool(dynamic find) = %q", got)
	}
	if got := truncateToolResult(strings.Repeat("x", maxToolResultLen+1)); !strings.HasSuffix(got, "\n...[truncated]") {
		t.Fatalf("truncateToolResult() suffix = %q, want truncated marker", got[len(got)-20:])
	}
}

// TestParseToolsSnapshot_AcceptsRawAndWrappedShapes verifies snapshots can be
// loaded from both tools/list-compatible and plain array fixtures.
func TestParseToolsSnapshot_AcceptsRawAndWrappedShapes(t *testing.T) {
	shapes := []struct {
		name string
		data []byte
	}{
		{"plain_array", []byte(`[{"name":"gitlab_project","inputSchema":{"type":"object"}}]`)},
		{"tools_list_wrapper", []byte(`{"tools":[{"name":"gitlab_project","inputSchema":{"type":"object"}}]}`)},
	}
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := parseToolsSnapshot(tc.data)
			if err != nil {
				t.Fatalf("parseToolsSnapshot(%s) error = %v", tc.data, err)
			}
			if len(snapshot) != 1 || snapshot[0].Name != "gitlab_project" {
				t.Fatalf("snapshot = %#v, want gitlab_project", snapshot)
			}
		})
	}
	if _, err := parseToolsSnapshot([]byte(`{"tools":`)); err == nil {
		t.Fatal("parseToolsSnapshot(invalid) error = nil, want error")
	}
}

// TestDynamicValidationRoutes_RewritesCatalogRoutes verifies dynamic mode routes
// are represented as gitlab_execute_action domain.action IDs.
func TestDynamicValidationRoutes_RewritesCatalogRoutes(t *testing.T) {
	routes := dynamicValidationRoutes(map[string]toolutil.ActionMap{
		"gitlab_project": {"get": toolutil.ActionRoute{}},
		"gitlab_issue":   {"create": toolutil.ActionRoute{Destructive: false}},
	})
	if _, ok := routes[dynamicExecuteActionTool]["project.get"]; !ok {
		t.Fatalf("dynamicValidationRoutes() = %#v, want project.get", routes)
	}
	if got := dynamicActionID("gitlab_issue", "create"); got != "issue.create" {
		t.Fatalf("dynamicActionID() = %q, want issue.create", got)
	}
}

// TestBuildCatalogSession_UsesClientEnterpriseMode verifies BuildCatalogSession uses client enterprise mode.
func TestBuildCatalogSession_UsesClientEnterpriseMode(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, _, routes, err := buildCatalogSession(client, config.ToolSurfaceMeta, ServerModeDefault)
	if err != nil {
		t.Fatalf("buildCatalogSession(enterprise=false) error = %v", err)
	}
	closeSession()
	if _, ok := routes["gitlab"]["merge_train.list_project"]; ok {
		t.Fatal("CE catalog registered enterprise-only merge_train.list_project route")
	}

	client = newEvalTestClient(t, true)
	_, closeSession, _, routes, err = buildCatalogSession(client, config.ToolSurfaceMeta, ServerModeDefault)
	if err != nil {
		t.Fatalf("buildCatalogSession(enterprise=true) error = %v", err)
	}
	defer closeSession()
	if _, routeOK := routes["gitlab"]["merge_train.list_project"]; !routeOK {
		if _, fallbackOK := routes["gitlab_merge_train"]["list_project"]; !fallbackOK {
			t.Skip("main catalog does not expose enterprise merge train routes")
		}
	}
}

// TestBuildCatalogSession_MetaSurfaceAppliesSchemaLockdown verifies the
// evaluator sees the same no-input object schema shape as runtime tools/list.
func TestBuildCatalogSession_MetaSurfaceAppliesSchemaLockdown(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, toolList, _, err := buildCatalogSession(client, config.ToolSurfaceMeta, ServerModeDefault)
	if err != nil {
		t.Fatalf("buildCatalogSession(meta) error = %v", err)
	}
	defer closeSession()

	var schema map[string]any
	for _, tool := range toolList {
		if tool.Name != "gitlab_interactive_project_create" {
			continue
		}
		var ok bool
		schema, ok = tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("input schema = %T, want map[string]any", tool.InputSchema)
		}
		break
	}
	if schema == nil {
		t.Fatal("gitlab_interactive_project_create was not registered")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || properties == nil {
		t.Fatalf("properties = %T, want map[string]any in %#v", schema["properties"], schema)
	}
	if len(properties) != 0 {
		t.Fatalf("properties = %#v, want empty map", properties)
	}
	if v, boolOK := schema["additionalProperties"].(bool); !boolOK || v {
		t.Fatalf("additionalProperties = %v, want false", schema["additionalProperties"])
	}
}

// TestBuildCatalogSession_DynamicSurfaceExposesExecuteRoutes verifies dynamic
// mode advertises the default low-token public tools while retaining catalog
// routes for validation and execution.
func TestBuildCatalogSession_DynamicSurfaceExposesExecuteRoutes(t *testing.T) {
	client := newEvalTestClient(t, false)
	_, closeSession, toolList, routes, err := buildCatalogSession(client, config.ToolSurfaceDynamic, ServerModeDefault)
	if err != nil {
		t.Fatalf("buildCatalogSession(dynamic) error = %v", err)
	}
	defer closeSession()

	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_action,gitlab_find_action" {
		t.Fatalf("dynamic catalog tools = %q, want find/execute", got)
	}
	if _, ok := routes[dynamicExecuteActionTool]["project.get"]; !ok {
		t.Fatal("dynamic validation routes missing project.get")
	}
	if _, ok := routes[dynamicExecuteActionTool]["discover_project.resolve"]; !ok {
		t.Fatal("dynamic validation routes missing discover_project.resolve")
	}
	if _, ok := routes["gitlab"]; ok {
		t.Fatal("dynamic validation routes unexpectedly exposed gitlab dispatcher")
	}
}

// newEvalTestClient constructs eval test client test fixtures.
func newEvalTestClient(t *testing.T, enterprise bool) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	}))
	t.Cleanup(srv.Close)
	tier := edition.Free
	if enterprise {
		tier = edition.Ultimate
	}
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       srv.URL,
		GitLabToken:     "eval-token",
		Tier:            tier,
		TierExplicit:    true,
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

// TestLoadToolsSnapshot_DerivesRoutes verifies LoadToolsSnapshot derives routes.
func TestLoadToolsSnapshot_DerivesRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.json")
	snapshot := `[
  {
    "name": "gitlab_project",
    "description": "Manage projects.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "action": {"type": "string", "enum": ["get", "list"]},
        "params": {"type": "object"}
      }
    }
  }
]`
	if err := os.WriteFile(path, []byte(snapshot), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	tools, routes, err := loadToolsSnapshot(path)
	if err != nil {
		t.Fatalf("loadToolsSnapshot() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gitlab_project" {
		t.Fatalf("tools = %+v, want gitlab_project", tools)
	}
	if _, ok := routes["gitlab_project"]["get"]; !ok {
		t.Fatalf("routes = %+v, want gitlab_project/get", routes)
	}
	if _, ok := routes["gitlab_project"]["list"]; !ok {
		t.Fatalf("routes = %+v, want gitlab_project/list", routes)
	}
}

// TestLoadCatalog_RejectsUnknownBackend verifies LoadCatalog rejects unknown backend.
func TestLoadCatalog_RejectsUnknownBackend(t *testing.T) {
	_, _, _, err := loadCatalog(options{Backend: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("error = %v, want unknown backend", err)
	}
}

// TestValidateExecutionOptions_AllowsExternalCommandWithDockerEnvFile verifies ValidateExecutionOptions allows external command with docker env file.
func TestValidateExecutionOptions_AllowsExternalCommandWithDockerEnvFile(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	envFile := filepath.Join(t.TempDir(), "docker.env")
	if err := os.WriteFile(envFile, []byte("E2E_MODE=docker\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	err := validateExecutionOptions(options{ToolsFile: "snapshot.json", MCPCommand: "gitlab-mcp-server", MCPEnv: envFile})
	if err != nil {
		t.Fatalf("validateExecutionOptions(external command) error = %v", err)
	}
}

// TestValidateExecutionOptions_ExternalCommandRequiresDockerGuard verifies ValidateExecutionOptions when external command requires docker guard.
func TestValidateExecutionOptions_ExternalCommandRequiresDockerGuard(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	err := validateExecutionOptions(options{ToolsFile: "snapshot.json", MCPCommand: "gitlab-mcp-server"})
	if err == nil || !strings.Contains(err.Error(), "E2E_MODE=docker") {
		t.Fatalf("error = %v, want external docker guard", err)
	}
}

// TestValidateExecutionOptions_ExternalCommandRequiresToolsFile verifies ValidateExecutionOptions when external command requires tools file.
func TestValidateExecutionOptions_ExternalCommandRequiresToolsFile(t *testing.T) {
	t.Setenv("E2E_MODE", "docker")
	err := validateExecutionOptions(options{MCPCommand: "gitlab-mcp-server"})
	if err == nil || !strings.Contains(err.Error(), "requires --tools-file") {
		t.Fatalf("error = %v, want tools-file guard", err)
	}
}

// TestToolResultContentPrefersStructuredContent verifies ToolResultContentPrefersStructuredContent.
func TestToolResultContentPrefersStructuredContent(t *testing.T) {
	result := &mcp.CallToolResult{
		StructuredContent: map[string]any{"username": "e2e-tester"},
		Content:           []mcp.Content{&mcp.TextContent{Text: "markdown fallback"}},
	}
	content := toolResultContent(result)
	if !strings.Contains(content, "e2e-tester") || strings.Contains(content, "markdown fallback") {
		t.Fatalf("content = %q, want structured content", content)
	}
}

// TestBuildCatalogSession_ExposesFullCapabilitySurface verifies eval sessions expose normal MCP capabilities.
func TestBuildCatalogSession_ExposesFullCapabilitySurface(t *testing.T) {
	client, cleanup, clientErr := newMockGitLabClient()
	if clientErr != nil {
		t.Fatalf("newMockGitLabClient() error = %v", clientErr)
	}
	defer cleanup()
	session, closeSession, _, _, sessionErr := buildCatalogSession(client, config.ToolSurfaceDynamic, ServerModeDefault)
	if sessionErr != nil {
		t.Fatalf("buildCatalogSession() error = %v", sessionErr)
	}
	defer closeSession()

	support := probeCapabilityBridgeSupport(session)
	if !support.Capabilities || !support.Resources || !support.Prompts || !support.Completion {
		t.Fatalf("capability support = %+v, want capabilities, resources, prompts, and completion", support)
	}

	resourcesResult, resourcesErr := session.ListResources(t.Context(), nil)
	if resourcesErr != nil {
		t.Fatalf("ListResources() error = %v", resourcesErr)
	}
	if !hasEvalResource(resourcesResult.Resources, "gitlab://tools") || !hasEvalResource(resourcesResult.Resources, "gitlab://user/current") {
		t.Fatalf("resources = %+v, want tools and normal GitLab resources", resourcesResult.Resources)
	}
	templatesResult, templatesErr := session.ListResourceTemplates(t.Context(), nil)
	if templatesErr != nil {
		t.Fatalf("ListResourceTemplates() error = %v", templatesErr)
	}
	if !hasEvalResourceTemplate(templatesResult.ResourceTemplates, "gitlab://tools/{id}") || !hasEvalResourceTemplate(templatesResult.ResourceTemplates, "gitlab://project/{project_id}") {
		t.Fatalf("resource templates = %+v, want tools detail and normal GitLab templates", templatesResult.ResourceTemplates)
	}
	requireReadResource(t, session, "gitlab://tools/project.get")

	promptsResult, promptsErr := session.ListPrompts(t.Context(), nil)
	if promptsErr != nil {
		t.Fatalf("ListPrompts() error = %v", promptsErr)
	}
	if len(promptsResult.Prompts) == 0 {
		t.Fatal("ListPrompts() returned no prompts, want normal prompt surface")
	}
	promptName, argumentName, ok := promptCompletionTarget(promptsResult.Prompts)
	if !ok {
		t.Fatalf("ListPrompts() = %+v, want at least one prompt argument supported by completion", promptsResult.Prompts)
	}
	completeResult, completeErr := session.Complete(t.Context(), &mcp.CompleteParams{
		Ref: &mcp.CompleteReference{Type: "ref/prompt", Name: promptName},
		Argument: mcp.CompleteParamsArgument{
			Name:  argumentName,
			Value: "",
		},
	})
	if completeErr != nil {
		t.Fatalf("Complete() error = %v", completeErr)
	}
	if completeResult == nil || completeResult.Completion.Values == nil {
		t.Fatalf("Complete() = %+v, want completion result", completeResult)
	}
}

// TestProbeCapabilityBridgeSupport_RequiresAdvertisedResources verifies that
// probeCapabilityBridgeSupport returns Resources=true only when the MCP session
// advertises the resources capability.
//
// The test calls the helper with two sessions: one without the resources
// capability and one with it, and asserts the bool flag flips correctly.
// This protects the runtime from enabling the capability bridge tools
// against servers that cannot serve them, which would otherwise produce
// empty responses and noisy trace events.
func TestProbeCapabilityBridgeSupport_RequiresAdvertisedResources(t *testing.T) {
	if support := probeCapabilityBridgeSupport(newProjectGetSession(t)); support.Resources {
		t.Fatalf("probeCapabilityBridgeSupport().Resources = true for session without resources capability, want false; support = %+v", support)
	}
	if support := probeCapabilityBridgeSupport(newResourceLookupSessionForTest(t)); !support.Resources {
		t.Fatalf("probeCapabilityBridgeSupport().Resources = false for session with resources, want true; support = %+v", support)
	}
}

func hasEvalResource(resources []*mcp.Resource, uri string) bool {
	for _, resource := range resources {
		if resource != nil && resource.URI == uri {
			return true
		}
	}
	return false
}

func hasEvalResourceTemplate(templates []*mcp.ResourceTemplate, uriTemplate string) bool {
	for _, template := range templates {
		if template != nil && template.URITemplate == uriTemplate {
			return true
		}
	}
	return false
}

func promptCompletionTarget(prompts []*mcp.Prompt) (string, string, bool) {
	for _, prompt := range prompts {
		if prompt == nil {
			continue
		}
		for _, argument := range prompt.Arguments {
			if argument != nil && promptArgumentSupportsCompletion(argument.Name) {
				return prompt.Name, argument.Name, true
			}
		}
	}
	return "", "", false
}

func promptArgumentSupportsCompletion(name string) bool {
	switch name {
	case "project_id", "group_id", "merge_request_iid", "issue_iid", "username", "from", "to", "ref", "tag", "pipeline_id", "sha", "branch", "source_branch", "target_branch", "label", "milestone_id", "milestone", "job_id":
		return true
	default:
		return false
	}
}

func requireReadResource(t *testing.T, session *mcp.ClientSession, uri string) {
	t.Helper()
	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource(%s) error = %v", uri, err)
	}
	if result == nil {
		t.Fatalf("ReadResource(%s) returned nil result", uri)
	}
}

// TestBuildCatalogSession_ServerModeShapesEvaluatedCatalog verifies that
// --server-mode reaches the catalog the evaluated model sees. Read-only and
// safe mode are per-action catalog policies, so evaluating them means the model
// is offered a different set of actions (read-only) or actions whose handlers
// preview instead of execute (safe mode). Without this the evaluator would keep
// scoring the unrestricted catalog and never exercise either mode.
func TestBuildCatalogSession_ServerModeShapesEvaluatedCatalog(t *testing.T) {
	client := newEvalTestClient(t, false)

	_, closeDefault, _, defaultRoutes, err := buildCatalogSession(client, config.ToolSurfaceMeta, ServerModeDefault)
	if err != nil {
		t.Fatalf("buildCatalogSession(default) error = %v", err)
	}
	closeDefault()

	_, closeReadOnly, _, readOnlyRoutes, err := buildCatalogSession(client, config.ToolSurfaceMeta, ServerModeReadOnly)
	if err != nil {
		t.Fatalf("buildCatalogSession(read-only) error = %v", err)
	}
	closeReadOnly()

	if _, mutatingPresent := readOnlyRoutes["gitlab_issue"]["create"]; mutatingPresent {
		t.Error("read-only evaluation catalog still offers issue create")
	}
	if _, readPresent := readOnlyRoutes["gitlab_issue"]["list"]; !readPresent {
		t.Error("read-only evaluation catalog dropped issue list: reads must stay evaluable")
	}
	if _, defaultMutating := defaultRoutes["gitlab_issue"]["create"]; !defaultMutating {
		t.Fatal("default evaluation catalog lacks issue create; the comparison above is meaningless")
	}

	_, closeSafe, _, safeRoutes, err := buildCatalogSession(client, config.ToolSurfaceMeta, ServerModeSafe)
	if err != nil {
		t.Fatalf("buildCatalogSession(safe-mode) error = %v", err)
	}
	closeSafe()

	safeCreate, safePresent := safeRoutes["gitlab_issue"]["create"]
	if !safePresent {
		t.Fatal("safe mode must keep mutating actions visible so the model can attempt them")
	}
	if safeCreate.Destructive {
		t.Error("safe-mode action stayed destructive; nothing executes, so nothing needs confirmation")
	}
	result, handlerErr := safeCreate.Handler(context.Background(), map[string]any{"project_id": "1", "title": "x"})
	if handlerErr != nil {
		t.Fatalf("safe-mode handler error = %v", handlerErr)
	}
	preview, isPreview := result.(toolutil.SafeModePreview)
	if !isPreview {
		t.Fatalf("safe-mode handler returned %T, want a preview", result)
	}
	if preview.Status != "blocked" || preview.Tool != "issue.create" {
		t.Errorf("preview = %+v, want blocked issue.create", preview)
	}
}

// TestBuildCatalogSession_ServerModeShapesDynamicSurface verifies the dynamic
// surface honors the evaluated server mode too: read-only leaves
// gitlab_execute_action routable but strips mutating actions from the routes it
// can reach, and safe mode keeps them routable as previews.
func TestBuildCatalogSession_ServerModeShapesDynamicSurface(t *testing.T) {
	client := newEvalTestClient(t, false)

	_, closeReadOnly, readOnlyTools, readOnlyRoutes, err := buildCatalogSession(client, config.ToolSurfaceDynamic, ServerModeReadOnly)
	if err != nil {
		t.Fatalf("buildCatalogSession(dynamic, read-only) error = %v", err)
	}
	closeReadOnly()

	var executeTool *mcp.Tool
	for _, tool := range readOnlyTools {
		if tool.Name == "gitlab_execute_action" {
			executeTool = tool
		}
	}
	if executeTool == nil {
		t.Fatal("read-only dynamic evaluation lost gitlab_execute_action: reads become unevaluable")
	}
	if executeTool.Annotations == nil || !executeTool.Annotations.ReadOnlyHint {
		t.Error("read-only dynamic execute tool must advertise ReadOnlyHint")
	}
	if hasEvalRoute(readOnlyRoutes, "issue.create") {
		t.Error("read-only dynamic evaluation still routes issue.create")
	}
	if !hasEvalRoute(readOnlyRoutes, "issue.list") {
		t.Error("read-only dynamic evaluation dropped issue.list")
	}

	_, closeSafe, _, safeRoutes, err := buildCatalogSession(client, config.ToolSurfaceDynamic, ServerModeSafe)
	if err != nil {
		t.Fatalf("buildCatalogSession(dynamic, safe-mode) error = %v", err)
	}
	closeSafe()
	if !hasEvalRoute(safeRoutes, "issue.create") {
		t.Error("safe mode must keep mutating actions routable so the model can attempt them")
	}
}

// hasEvalRoute reports whether any tool in routes exposes the action id, which
// dynamic routes key by canonical ID and meta routes key by action name.
func hasEvalRoute(routes map[string]toolutil.ActionMap, actionID string) bool {
	for _, actions := range routes {
		if _, ok := actions[actionID]; ok {
			return true
		}
	}
	return false
}
