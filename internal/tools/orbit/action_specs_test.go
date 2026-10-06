package orbit

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
			if !specs[i].OpenWorld || !specs[i].ReadOnly || specs[i].Destructive {
				t.Fatalf("ActionSpecs()[%d] open_world:%t read_only:%t destructive:%t, want an open-world read-only action",
					i, specs[i].OpenWorld, specs[i].ReadOnly, specs[i].Destructive)
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

// TestOrbit_QuerySpec_TeachesTheVersion12DSL verifies that what orbit.query
// serves a model about its one parameter is version 12 of the DSL, the one
// GitLab.com compiles queries against: the usage line, the schema description
// of query, and its parameter guidance, whose example is a query of that
// version. Every confusion names a rule of that version a model gets wrong,
// three of them refusing the older shape this server taught until issue 1031.
// The other five actions take no query and carry no guidance.
func TestOrbit_QuerySpec_TeachesTheVersion12DSL(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "tok", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	specs := ActionSpecs(client)
	for i, spec := range specs {
		if i != 4 && spec.ParameterGuidance != nil {
			t.Errorf("ActionSpecs()[%d] (%s).ParameterGuidance = %v, want none", i, spec.Name, spec.ParameterGuidance)
		}
	}
	querySpec := specs[4]
	field, ok := reflect.TypeFor[QueryInput]().FieldByName("Query")
	if !ok {
		t.Fatal("QueryInput has no Query field")
	}
	guidance, ok := querySpec.ParameterGuidance["query"]
	if !ok {
		t.Fatalf("orbit.query ParameterGuidance = %v, want guidance for query", querySpec.ParameterGuidance)
	}

	// The first sentence is what a reader that shortens descriptions keeps
	// (llms-medium.txt keeps it, cut at 160 runes), so the two pointers are
	// held there and not merely somewhere in the line.
	firstSentence, _, _ := strings.Cut(querySpec.Usage, ". ")
	if n := utf8.RuneCountInString(firstSentence); n > 160 {
		t.Errorf("orbit.query usage's first sentence is %d runes, want at most 160 so a reader that cuts there keeps it whole: %q", n, firstSentence)
	}

	served := []struct {
		name string
		text string
		want string
	}{
		{name: "the first sentence points at the DSL", text: firstSentence, want: "orbit.dsl"},
		{name: "the first sentence points at the schema", text: firstSentence, want: "orbit.schema"},
		{name: "usage names the version", text: querySpec.Usage, want: "version 12"},
		{name: "usage names the nodes list", text: querySpec.Usage, want: "nodes array"},
		{name: "usage points at the DSL", text: querySpec.Usage, want: "orbit.dsl"},
		{name: "usage points at the schema", text: querySpec.Usage, want: "orbit.schema"},
		{name: "usage points at the refusal", text: querySpec.Usage, want: "message GitLab answers with"},
		{name: "description names the version", text: field.Tag.Get("jsonschema"), want: "version 12"},
		{name: "description points at the DSL", text: field.Tag.Get("jsonschema"), want: "orbit.dsl"},
		{name: "description points at the schema", text: field.Tag.Get("jsonschema"), want: "orbit.schema"},
		{name: "description lists nodes", text: field.Tag.Get("jsonschema"), want: "nodes, a list"},
		{name: "description keys a filter by its operator", text: field.Tag.Get("jsonschema"), want: `{"starts_with"`},
		{name: "description refuses op and value", text: field.Tag.Get("jsonschema"), want: "never {op, value}"},
		{name: "description gives the neighbors default", text: field.Tag.Get("jsonschema"), want: "defaults to outgoing"},
		{name: "description requires rel_types on a path", text: field.Tag.Get("jsonschema"), want: "rel_types required"},
		{name: "description keeps the parameter required", text: field.Tag.Get("jsonschema"), want: ",required"},
		{name: "value source points at the DSL", text: guidance.ValueSource, want: "orbit.dsl"},
		{name: "value source points at the schema", text: guidance.ValueSource, want: "orbit.schema"},
		{name: "confusions refuse a top-level node", text: strings.Join(guidance.CommonConfusions, "\n"), want: "top-level node is refused"},
		{name: "confusions key a filter by its operator", text: strings.Join(guidance.CommonConfusions, "\n"), want: `{"starts_with": "gitlab-org/"}`},
		{name: "confusions refuse op and value", text: strings.Join(guidance.CommonConfusions, "\n"), want: "older {op, value} form is refused"},
		{name: "confusions give the neighbors default", text: strings.Join(guidance.CommonConfusions, "\n"), want: "defaults to outgoing"},
		{name: "confusions name any relationship type", text: strings.Join(guidance.CommonConfusions, "\n"), want: `["*"]`},
		{name: "confusions key an aggregation by its function", text: strings.Join(guidance.CommonConfusions, "\n"), want: `{"count": "mr", "as": "mr_count"}`},
	}
	for _, tt := range served {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.text, tt.want) {
				t.Errorf("served text = %q, want it to say %q", tt.text, tt.want)
			}
		})
	}

	var example map[string]any
	if err = json.Unmarshal([]byte(guidance.ExampleBinding), &example); err != nil {
		t.Fatalf("query ExampleBinding %q is not JSON: %v", guidance.ExampleBinding, err)
	}
	nodes, isList := example["nodes"].([]any)
	if example["query_type"] != "traversal" || !isList || len(nodes) != 1 || example["node"] != nil {
		t.Errorf("query ExampleBinding = %v, want a traversal listing its one node in nodes", example)
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

// TestOrbit_RegisterMeta_UsesActionSpecs verifies that the route map the
// gitlab_orbit meta-tool is registered with holds, under each spec's name,
// the route that spec declared: same destructiveness, same input schema, same
// output schema.
//
// The map is compared with the specs it was built from. It used to be
// compared with a second copy of itself, which held nothing.
func TestOrbit_RegisterMeta_UsesActionSpecs(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	got := orbitActionSpecRoutes(t, client)
	want := ActionSpecs(client)

	if len(got) != len(want) {
		t.Fatalf("registered orbit route count = %d, want %d", len(got), len(want))
	}
	for _, spec := range want {
		t.Run(spec.Name, func(t *testing.T) {
			gotRoute, ok := got[spec.Name]
			if !ok {
				t.Fatalf("registered meta routes missing %q", spec.Name)
			}
			if gotRoute.Destructive != spec.Route.Destructive {
				t.Fatalf("destructive = %t, want %t", gotRoute.Destructive, spec.Route.Destructive)
			}
			if !reflect.DeepEqual(gotRoute.InputSchema, spec.Route.InputSchema) {
				t.Fatal("input schema differs from ActionSpec projection")
			}
			if !reflect.DeepEqual(gotRoute.OutputSchema, spec.Route.OutputSchema) {
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
		{name: "gitlab_orbit_query", args: map[string]any{"query": projectByIDQuery()}},
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
		{name: "gitlab_orbit_query", args: map[string]any{"query": projectByIDQuery()}},
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

// TestOrbit_ActionSpecs_ForbiddenStaysAnError verifies that the not-found
// wrapper every Orbit route carries converts a 404 and nothing else: a 403
// keeps its Knowledge Graph access hint and reaches the caller as the error it
// is, rather than as the "Orbit is not enabled" card, which would send a
// caller with the wrong token off to check feature flags.
func TestOrbit_ActionSpecs_ForbiddenStaysAnError(t *testing.T) {
	routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "gitlab_orbit_status", args: map[string]any{}},
		{name: "gitlab_orbit_schema", args: map[string]any{}},
		{name: "gitlab_orbit_tools", args: map[string]any{}},
		{name: "gitlab_orbit_dsl", args: map[string]any{}},
		{name: "gitlab_orbit_query", args: map[string]any{"query": projectByIDQuery()}},
		{name: "gitlab_orbit_graph_status", args: map[string]any{"full_path": "gitlab-org/gitlab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := routes[tt.name]
			if !ok {
				t.Fatalf("routes map missing %q; registration dropped an individual tool", tt.name)
			}
			result, err := route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler() = %#v with nil error, want the forbidden error", result)
			}
			if !strings.Contains(err.Error(), "Knowledge Graph enabled") {
				t.Fatalf("Route.Handler() error = %q, want the Knowledge Graph access hint", err)
			}
			if _, notFound := result.(orbitNotFoundOutput); notFound {
				t.Fatalf("Route.Handler() = %#v, want no not-found card for a 403", result)
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
