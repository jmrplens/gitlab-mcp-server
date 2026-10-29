// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

// TestOrbit_QuerySpec_TeachesGQL verifies that what orbit.query serves a model
// about its one parameter is GQL, the language GitLab.com compiles every query
// in since 2026-10-15 (issue 1414): the usage line, the schema description of
// query, and its parameter guidance, whose example is a GQL statement
// GitLab.com answered. Every confusion names a rule of GQL a model gets wrong,
// and one says what a query sent as a JSON object is answered. Nothing served
// teaches the JSON DSL any longer. The other five actions take no query and
// carry no guidance.
func TestOrbit_QuerySpec_TeachesGQL(t *testing.T) {
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

	description := field.Tag.Get("jsonschema")
	confusions := strings.Join(guidance.CommonConfusions, "\n")
	served := []struct {
		name string
		text string
		want string
	}{
		{name: "the first sentence points at the language", text: firstSentence, want: "orbit.dsl"},
		{name: "the first sentence points at the schema", text: firstSentence, want: "orbit.schema"},
		{name: "the first sentence names GQL", text: firstSentence, want: "GQL"},
		{name: "usage names the statement", text: querySpec.Usage, want: "MATCH ... RETURN"},
		{name: "usage says a query is text", text: querySpec.Usage, want: "sent as text"},
		{name: "usage names the anchoring rule", text: querySpec.Usage, want: "bounded by its id or a literal property"},
		{name: "usage points at the refusal", text: querySpec.Usage, want: "message GitLab answers with"},
		{name: "description names GQL", text: description, want: "GQL text"},
		{name: "description points at the language", text: description, want: "orbit.dsl"},
		{name: "description points at the schema", text: description, want: "orbit.schema"},
		{name: "description gives the statement", text: description, want: "MATCH pattern [WHERE predicates] RETURN projections"},
		{name: "description gives an example", text: description, want: "MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1"},
		{name: "description gives the relationship type syntax", text: description, want: "-[:IN_PROJECT]->"},
		{name: "description refuses OR", text: description, want: "never OR"},
		{name: "description gives an aggregation", text: description, want: "count(mr) AS mr_count"},
		{name: "description names where the vocabulary comes from", text: description, want: "orbit.schema lists the entities, properties and relationship types"},
		{name: "description says what an object gets", text: description, want: "GitLab.com refuses it while it reads queries as GQL"},
		{name: "description keeps the parameter required", text: description, want: ",required"},
		{name: "value source names GQL", text: guidance.ValueSource, want: "GQL text"},
		{name: "value source points at the language", text: guidance.ValueSource, want: "orbit.dsl"},
		{name: "value source points at the schema", text: guidance.ValueSource, want: "orbit.schema"},
		{name: "confusions bound a node", text: confusions, want: "LIMIT bounds nothing"},
		{name: "confusions give the colon", text: confusions, want: "Written without it, -[IN_PROJECT]-> declares a variable"},
		{name: "confusions refuse OR", text: confusions, want: "OR, XOR, RETURN DISTINCT, count(*), OPTIONAL MATCH, WITH and UNION are refused"},
		{name: "confusions page with SKIP and LIMIT", text: confusions, want: "A page is SKIP and LIMIT"},
		{name: "confusions name a shortest path's relationship types", text: confusions, want: "a wildcard -[*1..3]-> is refused"},
		{name: "description pages with SKIP and LIMIT", text: description, want: "SKIP takes 0 to 10000 and LIMIT 1 to 1000"},
		{name: "confusions negate a list", text: confusions, want: "NOT p.visibility_level IN [...]"},
		{name: "confusions keep -- for neighbors", text: confusions, want: "The undirected -- is for the neighbors of one node"},
		{name: "confusions give a shortest path", text: confusions, want: "ANY SHORTEST"},
		{name: "confusions say what an object gets", text: confusions, want: "answered compile_error while it reads GQL"},
	}
	for _, tt := range served {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.text, tt.want) {
				t.Errorf("served text = %q, want it to say %q", tt.text, tt.want)
			}
		})
	}

	// What the JSON DSL taught: a nodes list, an operator-keyed filter, the
	// version number. A served sentence teaching any of it again would send a
	// model to write the language GitLab.com refuses.
	retired := []string{"version 12", "nodes array", "query_type (", `{"starts_with"`, "rel_types required", `{"count": "mr"`}
	texts := []struct {
		name string
		text string
	}{
		{name: "usage", text: querySpec.Usage},
		{name: "description", text: description},
		{name: "value source", text: guidance.ValueSource},
		{name: "confusions", text: confusions},
		{name: "example", text: guidance.ExampleBinding},
	}
	for _, tt := range texts {
		t.Run("the "+tt.name+" teaches no JSON DSL", func(t *testing.T) {
			for _, gone := range retired {
				if strings.Contains(tt.text, gone) {
					t.Errorf("served %s %q still teaches the JSON DSL (%q)", tt.name, tt.text, gone)
				}
			}
		})
	}

	if guidance.ExampleBinding != queryExampleGQL {
		t.Errorf("query ExampleBinding = %q, want the GQL statement %q", guidance.ExampleBinding, queryExampleGQL)
	}
}

// TestOrbit_QueryResponseFormat_NamesTheThreeFormats verifies that the schema
// description of orbit.query's response_format names the three formats
// POST /orbit/query takes, gql among them, that orbit.dsl, whose MCP tool
// takes two, says raw or llm, and that the two actions GitLab now answers
// through GraphQL say every value returns the one structured shape.
func TestOrbit_QueryResponseFormat_NamesTheThreeFormats(t *testing.T) {
	cases := []struct {
		name  string
		input reflect.Type
		want  string
	}{
		{name: "query", input: reflect.TypeFor[QueryInput](), want: "raw (structured JSON, the default here), llm (compact text) or gql (the result as a graph pattern table)"},
		{name: "dsl", input: reflect.TypeFor[DSLInput](), want: "Response format: raw or llm"},
		{name: "status", input: reflect.TypeFor[StatusInput](), want: "every value returns the same output"},
		{name: "graph_status", input: reflect.TypeFor[GraphStatusInput](), want: "every value returns the same output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			field, ok := tc.input.FieldByName("ResponseFormat")
			if !ok {
				t.Fatalf("%s has no ResponseFormat field", tc.input)
			}
			if got := field.Tag.Get("jsonschema"); !strings.Contains(got, tc.want) {
				t.Errorf("%s response_format description = %q, want it to say %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestOrbit_DSLRoute_NotFound_NamesTheQueryLanguage verifies the card a 404
// from the query command's route produces: it names the query language and
// the query_graph command it is read from, never the JSON DSL GitLab.com
// stopped serving.
func TestOrbit_DSLRoute_NotFound_NamesTheQueryLanguage(t *testing.T) {
	routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	result, err := routes["gitlab_orbit_dsl"].Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler() error = %v, want nil", err)
	}
	callResult := toolutil.MarkdownForResult(result)
	if callResult == nil || len(callResult.Content) == 0 {
		t.Fatalf("MarkdownForResult() = %#v, want the not-found card", callResult)
	}
	text, ok := callResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", callResult.Content[0])
	}
	for _, want := range []string{"GitLab Orbit Query Language Not Found", "query_graph command"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text.Text, want) {
				t.Errorf("card = %q, want it to say %q", text.Text, want)
			}
		})
	}
	if strings.Contains(text.Text, "DSL") {
		t.Errorf("card = %q, still names the JSON DSL", text.Text)
	}
}

// TestOrbit_DSLSpec_DescribesGQL verifies that orbit.dsl's usage line says it
// serves GQL, the language GitLab.com compiles queries in, rather than the
// JSON DSL schema GitLab.com no longer serves.
func TestOrbit_DSLSpec_DescribesGQL(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "tok", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	dslSpec := ActionSpecs(client)[3]
	for _, want := range []string{"GQL", "query language", "JSON Schema of its input"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(dslSpec.Usage, want) {
				t.Errorf("orbit.dsl usage = %q, want it to say %q", dslSpec.Usage, want)
			}
		})
	}
	if strings.Contains(dslSpec.Usage, "DSL schema") {
		t.Errorf("orbit.dsl usage = %q, still promises the JSON DSL schema", dslSpec.Usage)
	}
}

// TestOrbit_QuerySchema_TakesTextOrAnObjectOnEverySurface verifies that the
// query parameter is served as a string or an object on every surface that
// carries a schema: the individual tool, and the meta tool's per-action schema
// in its full and compact modes. A Go field typed any reflects as no type at
// all, which the compact mode would serve as an empty schema, and the type
// override is what says both forms are taken; a schema typed object alone, as
// it was until issue 1414, refuses the GQL text GitLab.com reads before the
// handler sees it.
func TestOrbit_QuerySchema_TakesTextOrAnObjectOnEverySurface(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.com", "tok", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	want := []any{"string", "object"}

	tool, err := toolutil.IndividualToolFromSpecs(ActionSpecs(client), "gitlab_orbit_query", toolutil.IndividualToolProjectionOptions{Description: "Run an Orbit query."})
	if err != nil {
		t.Fatalf("IndividualToolFromSpecs() error: %v", err)
	}
	individual, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("individual input schema = %T, want a map", tool.InputSchema)
	}
	wantExamples := []any{queryExampleGQL}
	if got := schemaPropertyType(t, individual, "query"); !reflect.DeepEqual(got, want) {
		t.Errorf("individual query type = %#v, want %#v", got, want)
	}
	if got := propertySchema(t, individual, "query")["examples"]; !reflect.DeepEqual(got, wantExamples) {
		t.Errorf("individual query examples = %#v, want the GQL statement %#v", got, wantExamples)
	}

	routes := orbitActionSpecRoutes(t, client)
	modes := []struct {
		mode     string
		examples any
	}{
		// The compact mode keeps a property's type and enum and nothing else,
		// so it carries no example by its own design.
		{mode: toolutil.MetaParamSchemaFull, examples: wantExamples},
		{mode: toolutil.MetaParamSchemaCompact, examples: nil},
	}
	for _, tc := range modes {
		t.Run("meta "+tc.mode, func(t *testing.T) {
			params := metaActionParams(t, toolutil.BuildMetaToolSchema(routes, tc.mode), "query")
			if got := schemaPropertyType(t, params, "query"); !reflect.DeepEqual(got, want) {
				t.Errorf("meta %s query type = %#v, want %#v", tc.mode, got, want)
			}
			if got := propertySchema(t, params, "query")["examples"]; !reflect.DeepEqual(got, tc.examples) {
				t.Errorf("meta %s query examples = %#v, want %#v", tc.mode, got, tc.examples)
			}
		})
	}
}

// TestOrbit_MetaFullSchema_AcceptsGQLText verifies end to end that a GQL
// statement passed to the gitlab_orbit meta tool's query action, under the
// full parameter schema whose conditionals the SDK validates arguments
// against, reaches GitLab as the body's query string and comes back as a
// result rather than a validation error.
func TestOrbit_MetaFullSchema_AcceptsGQLText(t *testing.T) {
	restore := toolutil.SetMetaParamSchemaModeScoped(toolutil.MetaParamSchemaFull)
	t.Cleanup(restore)
	session := newOrbitMetaSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got struct {
			Query any `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got.Query != projectByIDGQL {
			t.Errorf("query = %#v, want the GQL text %q", got.Query, projectByIDGQL)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"traversal","row_count":0}`)
	}))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_orbit",
		Arguments: map[string]any{"action": "query", "params": map[string]any{"query": projectByIDGQL}},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("gitlab_orbit query with GQL text = %#v, want a result", result.Content)
	}
}

// TestOrbit_DSLRoute_ReadsTheQueryCommandFromTheMCPEndpoint verifies the
// orbit.dsl route as a caller meets it: GitLab.com answers 404 at the two
// REST routes this action read before (GET /orbit/schema/dsl since issue 1414,
// GET /orbit/agent/commands since gitlab-org/gitlab!266009), and the route
// serves the query command's grammar from list_commands of the Orbit MCP
// endpoint, its card ending on the action that lists the entities a query
// names.
func TestOrbit_DSLRoute_ReadsTheQueryCommandFromTheMCPEndpoint(t *testing.T) {
	routes := newOrbitSpecsByTool(t, orbitSourcesHandler(t))

	result, err := routes["gitlab_orbit_dsl"].Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler() error: %v", err)
	}
	callResult := toolutil.MarkdownForResult(result)
	if callResult == nil || callResult.IsError || len(callResult.Content) == 0 {
		t.Fatalf("MarkdownForResult() = %#v, want the query language card", callResult)
	}
	text, ok := callResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", callResult.Content[0])
	}
	for _, want := range []string{queryGraphGrammar, "Use action 'orbit.schema'"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text.Text, want) {
				t.Errorf("card = %q, want it to carry %q", text.Text, want)
			}
		})
	}
}

// TestOrbit_QueryRoute_SendsGQLTextAndExplainsARefusedObject verifies the
// orbit.query route as a caller meets it: GQL text passed as the query
// argument reaches GitLab as the body's query string, and a JSON object that
// GitLab.com refuses for its language is answered with the hint that it reads
// a query as GQL text.
func TestOrbit_QueryRoute_SendsGQLTextAndExplainsARefusedObject(t *testing.T) {
	routes := newOrbitSpecsByTool(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			Query any `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if _, isObject := got.Query.(map[string]any); isObject {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"code":"compile_error","message":"schema violation: Orbit query syntax at line 1, column 1: expected Statement"}`)
			return
		}
		if got.Query != projectByIDGQL {
			t.Errorf("query = %#v, want the GQL text %q", got.Query, projectByIDGQL)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"traversal","row_count":0}`)
	}))
	route := routes["gitlab_orbit_query"]

	if _, err := route.Handler(t.Context(), map[string]any{"query": projectByIDGQL}); err != nil {
		t.Fatalf("Route.Handler() with GQL text error: %v", err)
	}
	_, err := route.Handler(t.Context(), map[string]any{"query": projectByIDObject()})
	if err == nil || !strings.Contains(err.Error(), "GitLab.com reads a query as GQL text, not as a JSON object") {
		t.Fatalf("Route.Handler() with an object error = %v, want the hint that GitLab.com reads GQL", err)
	}
}

// schemaPropertyType returns the type a schema's properties give name,
// failing the test when the schema has no such property.
func schemaPropertyType(t *testing.T, schema map[string]any, name string) any {
	t.Helper()
	return propertySchema(t, schema, name)["type"]
}

// propertySchema returns the schema a schema's properties give name, failing
// the test when the schema has no such property.
func propertySchema(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %v, want %q", properties, name)
	}
	return property
}

// metaActionParams returns the params schema a meta tool schema defines for
// action under $defs, failing the test when it defines none.
func metaActionParams(t *testing.T, schema map[string]any, action string) map[string]any {
	t.Helper()
	definitions, _ := schema["$defs"].(map[string]any)
	params, ok := definitions[action].(map[string]any)
	if !ok {
		t.Fatalf("meta schema $defs = %v, want a params schema for %q", definitions, action)
	}
	return params
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
	session := newOrbitMetaSession(t, orbitSourcesHandler(t))

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
	routes := newOrbitSpecsByTool(t, orbitSourcesHandler(t))

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "gitlab_orbit_status", args: map[string]any{}},
		{name: "gitlab_orbit_schema", args: map[string]any{}},
		{name: "gitlab_orbit_tools", args: map[string]any{}},
		{name: "gitlab_orbit_dsl", args: map[string]any{}},
		{name: "gitlab_orbit_query", args: map[string]any{"query": projectByIDGQL}},
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
		{name: "gitlab_orbit_query", args: map[string]any{"query": projectByIDGQL}},
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
// wrapper every Orbit route carries converts a 404 and nothing else: a 403,
// which GitLab.com answers an account Orbit is not served to since its general
// availability, keeps the hint naming what Orbit needs and reaches the caller
// as the error it is, rather than as the not-found card.
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
		{name: "gitlab_orbit_query", args: map[string]any{"query": projectByIDGQL}},
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
			if !strings.Contains(err.Error(), orbitUnavailableHint) {
				t.Fatalf("Route.Handler() error = %q, want the Orbit availability hint", err)
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
