package orbit

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_OrderAndCount verifies that [ActionSpecs] returns
// exactly six specs in the canonical order: status, schema, tools,
// dsl, query, graph_status. Meta-tool and individual-tool projection
// rely on this order to map aliases and routes deterministically.
//
// The first alias on every spec is the individual tool name
// (gitlab_orbit_<name>); the spec may carry additional natural-language
// aliases for the dynamic find tool. Every spec is read-only,
// GitLab.com-only, Premium/Ultimate-gated, and marked OpenWorld.
func TestActionSpecs_OrderAndCount(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "tok", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("ActionSpecs() len = %d, want 6", len(specs))
	}
	wantNames := []string{"status", "schema", "tools", "dsl", "query", "graph_status"}
	for i, want := range wantNames {
		t.Run(want, func(t *testing.T) {
			if specs[i].Name != want {
				t.Fatalf("ActionSpecs()[%d].Name = %q, want %q", i, specs[i].Name, want)
			}
			if len(specs[i].Aliases) == 0 {
				t.Fatalf("ActionSpecs()[%d].Aliases = %v, want first alias gitlab_orbit_%s", i, specs[i].Aliases, want)
			}
			if specs[i].Aliases[0] != "gitlab_orbit_"+want {
				t.Fatalf("ActionSpecs()[%d].Aliases[0] = %q, want gitlab_orbit_%s (full list: %v)",
					i, specs[i].Aliases[0], want, specs[i].Aliases)
			}
			if specs[i].OwnerPackage != "orbit" {
				t.Fatalf("ActionSpecs()[%d].OwnerPackage = %q, want orbit", i, specs[i].OwnerPackage)
			}
			if !specs[i].GitLabDotComOnly || specs[i].Edition != "premium" {
				t.Fatalf("ActionSpecs()[%d] gating = dotcom:%t edition:%q, want GitLab.com premium", i, specs[i].GitLabDotComOnly, specs[i].Edition)
			}
			if !specs[i].OpenWorld {
				t.Fatalf("ActionSpecs()[%d].OpenWorld = false, want true", i)
			}
		})
	}
}

// TestActionSpecs_AliasesAndRelatedActions verifies that every orbit
// action spec carries the dynamic-surface aliases (the `kg.*` and
// `knowledge_graph.*` family) and the cross-references that let the
// LLM chain calls without re-discovering the catalog. Together with
// the canonical `{individualTool}` alias, every action must be
// reachable via at least one of the natural-language shorthands so
// the dynamic find tool can resolve queries like "kg query" or
// "knowledge graph schema".
//
// The check is per-spec rather than strictly "kg.<name>": some specs
// expose semantically richer shorthands (e.g. kg.indexing for
// graph_status) that the LLM is more likely to query. The contract
// enforced here is "every spec has at least one `kg.*` and one
// `knowledge_graph.*` alias, plus at least one RelatedAction".
func TestActionSpecs_AliasesAndRelatedActions(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "tok", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("ActionSpecs() len = %d, want 6", len(specs))
	}
	for i, spec := range specs {
		hasKGAlias := false
		hasKnowledgeGraphAlias := false
		for _, alias := range spec.Aliases {
			if strings.HasPrefix(alias, "kg.") {
				hasKGAlias = true
			}
			if strings.HasPrefix(alias, "knowledge_graph.") {
				hasKnowledgeGraphAlias = true
			}
		}
		if !hasKGAlias {
			t.Fatalf("ActionSpecs()[%d] (%s) missing kg.* alias (full list: %v)", i, spec.Name, spec.Aliases)
		}
		if !hasKnowledgeGraphAlias {
			t.Fatalf("ActionSpecs()[%d] (%s) missing knowledge_graph.* alias (full list: %v)", i, spec.Name, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Fatalf("ActionSpecs()[%d] (%s) has no RelatedActions; LLM cannot chain calls", i, spec.Name)
		}
	}
	// Spot-check: query and graph_status expose the most useful related links.
	querySpec := specs[4]
	if !slices.Contains(querySpec.RelatedActions, "orbit.schema") {
		t.Fatalf("orbit.query RelatedActions = %v, want orbit.schema", querySpec.RelatedActions)
	}
	if !slices.Contains(querySpec.RelatedActions, "orbit.graph_status") {
		t.Fatalf("orbit.query RelatedActions = %v, want orbit.graph_status", querySpec.RelatedActions)
	}
	graphStatusSpec := specs[5]
	if !slices.Contains(graphStatusSpec.RelatedActions, "orbit.query") {
		t.Fatalf("orbit.graph_status RelatedActions = %v, want orbit.query", graphStatusSpec.RelatedActions)
	}
}

// TestOrbit_ActionSpecs_Metadata verifies that every canonical Orbit
// ActionSpec carries the metadata required for projection: an
// `orbit` owner package, GitLab.com-only Premium/Ultimate edition
// gating, and the expected individual tool alias.
func TestOrbit_ActionSpecs_Metadata(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	specs := ActionSpecs(client)
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.OwnerPackage != "orbit" {
			t.Fatalf("OwnerPackage for %s = %q, want orbit", spec.Name, spec.OwnerPackage)
		}
		if !spec.GitLabDotComOnly || spec.Edition != "premium" {
			t.Fatalf("spec %s gating = dotcom:%t edition:%q, want GitLab.com premium", spec.Name, spec.GitLabDotComOnly, spec.Edition)
		}
		names = append(names, spec.IndividualTool.Name)
	}
	for _, want := range []string{"gitlab_orbit_status", "gitlab_orbit_schema", "gitlab_orbit_tools", "gitlab_orbit_dsl", "gitlab_orbit_query", "gitlab_orbit_graph_status"} {
		t.Run(want, func(t *testing.T) {
			if !containsTool(names, want) {
				t.Fatalf("ActionSpecs() missing %s in %v", want, names)
			}
		})
	}
}

// TestOrbit_RegisterMeta_RegistersMetaTool verifies that the consolidated
// gitlab_orbit meta-tool is registered with the MCP server.
//
// The test registers the meta-tool and asserts that it appears in the tool list.
func TestOrbit_RegisterMeta_RegistersMetaTool(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerOrbitMetaForTest(t, server, client)
	if names := registeredToolNames(t, server); !containsTool(names, "gitlab_orbit") {
		t.Fatalf("registered tools = %v, want gitlab_orbit present", names)
	}
}

// TestOrbit_RegisterMeta_UsesActionSpecs verifies that the routes used by
// the consolidated gitlab_orbit meta-tool are projected from the canonical
// GitLab.com-only ActionSpec definitions.
//
// The test compares the registered meta routes to the ActionSpecs projection for schema and destructiveness.
func TestOrbit_RegisterMeta_UsesActionSpecs(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	got := orbitActionSpecRoutes(t, client)
	want, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(client))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("registered orbit route count = %d, want %d", len(got), len(want))
	}
	for actionName, wantRoute := range want {
		t.Run(actionName, func(t *testing.T) {
			gotRoute, ok := got[actionName]
			if !ok {
				t.Fatalf("registered meta routes missing %q", actionName)
			}
			if gotRoute.Destructive != wantRoute.Destructive {
				t.Fatalf("destructive = %t, want %t", gotRoute.Destructive, wantRoute.Destructive)
			}
			if !reflect.DeepEqual(gotRoute.InputSchema, wantRoute.InputSchema) {
				t.Fatal("input schema differs from ActionSpec projection")
			}
			if !reflect.DeepEqual(gotRoute.OutputSchema, wantRoute.OutputSchema) {
				t.Fatal("output schema differs from ActionSpec projection")
			}
		})
	}
}

// TestOrbit_RegisterMeta_CallThroughMCP verifies that the consolidated Orbit
// meta-tool dispatches an MCP status call through to the GitLab API.
//
// The test creates a meta-tool session and calls the status action, asserting a non-error result.
func TestOrbit_RegisterMeta_CallThroughMCP(t *testing.T) {
	session := newOrbitMetaSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.RespondJSON(w, http.StatusOK, `{"status":"healthy","version":"0.5.0"}`)
	}))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_orbit",
		Arguments: map[string]any{"action": "status", "params": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatal("gitlab_orbit status returned error result")
	}
}

// TestOrbit_ActionSpecs_CallAllRoutes verifies that each individual Orbit
// route can be invoked through the canonical action specs.
//
// The test iterates all canonical tool routes, invokes each handler, and asserts a non-nil result.
func TestOrbit_ActionSpecs_CallAllRoutes(t *testing.T) {
	routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/orbit/status":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"status":"healthy","version":"0.5.0"}`)
		case "/api/v4/orbit/schema":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0"}`)
		case "/api/v4/orbit/tools":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"query_graph","description":"Execute graph queries","parameters":{"type":"object"}}]`)
		case "/api/v4/orbit/schema/dsl":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"type":"object","properties":{"query_type":{"type":"string"}}}`)
		case "/api/v4/orbit/query":
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"traversal","row_count":0}`)
		case "/api/v4/orbit/graph_status":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"projects":{"indexed":1,"total_known":1},"indexing":{"state":"indexed"}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
	}))

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "gitlab_orbit_status", args: map[string]any{}},
		{name: "gitlab_orbit_schema", args: map[string]any{}},
		{name: "gitlab_orbit_tools", args: map[string]any{}},
		{name: "gitlab_orbit_dsl", args: map[string]any{}},
		{name: "gitlab_orbit_query", args: map[string]any{"query": map[string]any{
			"query_type": "traversal",
			"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		}}},
		{name: "gitlab_orbit_graph_status", args: map[string]any{"full_path": "gitlab-org/gitlab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := routes[tt.name]
			if !ok {
				t.Fatalf("routes map missing %q; registration dropped an individual tool", tt.name)
			}
			result, err := route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler() error: %v", err)
			}
			if result == nil {
				t.Fatalf("Route.Handler() returned nil for %s", tt.name)
			}
		})
	}
}

// TestOrbit_ActionSpecs_NotFoundReturnsInformationalResult verifies that
// Orbit 404 responses become informational MCP errors with setup guidance.
//
// The test mocks 404 responses for all routes and asserts that the markdown result is an informational error with guidance text.
func TestOrbit_ActionSpecs_NotFoundReturnsInformationalResult(t *testing.T) {
	routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "gitlab_orbit_status", args: map[string]any{}},
		{name: "gitlab_orbit_schema", args: map[string]any{}},
		{name: "gitlab_orbit_tools", args: map[string]any{}},
		{name: "gitlab_orbit_dsl", args: map[string]any{}},
		{name: "gitlab_orbit_query", args: map[string]any{"query": map[string]any{
			"query_type": "traversal",
			"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		}}},
		{name: "gitlab_orbit_graph_status", args: map[string]any{"full_path": "gitlab-org/gitlab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := routes[tt.name]
			if !ok {
				t.Fatalf("routes map missing %q; registration dropped an individual tool", tt.name)
			}
			result, err := route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler() error = %v, want nil", err)
			}
			callResult := toolutil.MarkdownForResult(result)
			if callResult == nil || !callResult.IsError {
				t.Fatalf("MarkdownForResult() = %#v, want informational error result", callResult)
			}
			if len(callResult.Content) == 0 {
				t.Fatal("MarkdownForResult() content is empty, want Orbit not-found guidance")
			}
			textContent, ok := callResult.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content type = %T, want *mcp.TextContent", callResult.Content[0])
			}
			if !strings.Contains(textContent.Text, "Not Found") || !strings.Contains(textContent.Text, "GitLab Orbit") {
				t.Fatalf("content = %q, want Orbit not-found guidance", textContent.Text)
			}
		})
	}
}

// orbitActionSpecRoutes returns the ActionMap for all canonical Orbit ActionSpecs for use in meta-tool registration tests.
// It fails the test if ActionSpecsToMapWithError returns an error.
func orbitActionSpecRoutes(t *testing.T, client *gitlabclient.Client) toolutil.ActionMap {
	t.Helper()
	routes, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(client))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	return routes
}

// registeredToolNames returns the list of tool names registered in the MCP server for test assertions.
// It connects a test client and server, lists tools, and returns their names.
func registeredToolNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// newOrbitMetaSession creates a new MCP client session with the Orbit meta-tool registered for testing.
// It uses the provided HTTP handler for all Orbit API calls.
func newOrbitMetaSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	return newOrbitMCPSession(t, handler, func(server *mcp.Server, client *gitlabclient.Client) {
		registerOrbitMetaForTest(t, server, client)
	})
}

// newOrbitSpecsByTool returns a map of tool name to ActionRoute for all canonical Orbit ActionSpecs.
// It is used to test route invocation and error handling for all tools.
func newOrbitSpecsByTool(t *testing.T, handler http.Handler) map[string]toolutil.ActionRoute {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	routes := make(map[string]toolutil.ActionRoute)
	for _, spec := range ActionSpecs(client) {
		routes[spec.IndividualTool.Name] = spec.Route
	}
	return routes
}

// registerOrbitMetaForTest registers the gitlab_orbit meta-tool with the MCP server for use in meta-tool tests.
// It uses the canonical ActionSpecs and the analytics icon.
func registerOrbitMetaForTest(t *testing.T, server *mcp.Server, client *gitlabclient.Client) {
	t.Helper()
	toolutil.AddReadOnlyMetaTool(server, "gitlab_orbit", "Query GitLab Orbit context.", orbitActionSpecRoutes(t, client), toolutil.IconAnalytics, toolutil.MarkdownForResult)
}

// newOrbitMCPSession creates a new MCP client session and registers the Orbit meta-tool or other tools as needed for integration tests.
// It sets up in-memory transports and cleans up all resources after the test.
func newOrbitMCPSession(t *testing.T, handler http.Handler, registerFn func(*mcp.Server, *gitlabclient.Client)) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerFn(server, client)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})
	return session
}

// containsTool reports whether the given tool name is present in the list of names.
func containsTool(names []string, want string) bool {
	return slices.Contains(names, want)
}
