// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// graphQLPath is where client-go sends every GraphQL request.
const graphQLPath = "/api/graphql"

// graphQLRequest is a GraphQL request as a test handler reads it.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// readGraphQL decodes the GraphQL request a handler received, reporting a
// body that does not decode.
func readGraphQL(t *testing.T, r *http.Request) graphQLRequest {
	t.Helper()
	testutil.AssertRequestMethod(t, r, http.MethodPost)
	testutil.AssertRequestPath(t, r, graphQLPath)
	var req graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decode GraphQL request: %v", err)
	}
	return req
}

// serviceStatusAnswer is GitLab.com's answer to [queryServiceStatus] on
// 2026-10-29, with two components left out.
const serviceStatusAnswer = `{"orbit":{"serviceStatus":{"status":"HEALTHY","version":"0.163.0","components":[` +
	`{"name":"orbit-webserver","status":"HEALTHY"},{"name":"clickhouse-graph","status":"DEGRADED"}]}}}`

// TestStatus_ServiceStatus_MirrorsEveryField verifies that [Status] asks
// GitLab's GraphQL API for the service status under the names the output
// publishes, and carries the answer field for field.
func TestStatus_ServiceStatus_MirrorsEveryField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := readGraphQL(t, r)
		if req.Query != queryServiceStatus {
			t.Errorf("query = %q, want queryServiceStatus", req.Query)
		}
		testutil.RespondGraphQL(w, http.StatusOK, serviceStatusAnswer)
	}))

	out, err := Status(context.Background(), client, StatusInput{})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	want := StatusOutput{
		Status:  "HEALTHY",
		Version: "0.163.0",
		Components: []StatusComponent{
			{Name: "orbit-webserver", Status: "HEALTHY"},
			{Name: "clickhouse-graph", Status: "DEGRADED"},
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Status() = %+v, want %+v", out, want)
	}
}

// TestStatus_EachResponseFormat_AnswersTheSameStructuredStatus verifies that
// every response format the REST route took is still accepted and answered
// with the one shape GraphQL has, so a caller that sends llm is not refused.
func TestStatus_EachResponseFormat_AnswersTheSameStructuredStatus(t *testing.T) {
	for _, format := range []string{"", "raw", "llm", "json", " LLM "} {
		t.Run("format "+format, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQL(w, http.StatusOK, serviceStatusAnswer)
			}))
			out, err := Status(context.Background(), client, StatusInput{StructuredFormatInput{ResponseFormat: format}})
			if err != nil {
				t.Fatalf("Status() error: %v", err)
			}
			if out.Status != "HEALTHY" || len(out.Components) != 2 {
				t.Fatalf("Status() = %+v, want the structured status", out)
			}
		})
	}
}

// TestGraphQLHandlers_NullAnswers_ReportOrbitUnavailable verifies the answers
// GitLab gives a user Orbit is not available to: serviceStatus or schema null,
// or the whole orbit field null, and a subgroup or a project whose index GitLab
// answers null with no access beside it, as it answered jmrp-oss/orbit-fixtures
// and kg-fixtures under it at 16:58 UTC on 2026-10-29. Each is an error
// wrapping [ErrOrbitUnavailable] with the hint naming what Orbit needs, never
// an empty answer that would read as a service with nothing in it; the index's
// hint adds that the top-level group reports its access.
func TestGraphQLHandlers_NullAnswers_ReportOrbitUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		call   func(context.Context, *testClient) error
		hint   string
	}{
		{name: "status, serviceStatus null", answer: `{"orbit":{"serviceStatus":null}}`, call: callStatus, hint: orbitUnavailableHint},
		{name: "status, orbit null", answer: `{"orbit":null}`, call: callStatus, hint: orbitUnavailableHint},
		{name: "schema, schema null", answer: `{"orbit":{"schema":null}}`, call: callSchema, hint: orbitUnavailableHint},
		{name: "schema, orbit null", answer: `{"orbit":null}`, call: callSchema, hint: orbitUnavailableHint},
		{name: "graph status, orbit null", answer: `{"group":{"orbit":null},"project":null}`, call: callGraphStatus, hint: graphStatusUnavailableHint},
		{name: "graph status, subgroup index null", answer: `{"group":{"orbit":` + subgroupNoIndexAnswer + `},"project":null}`, call: callGraphStatus, hint: graphStatusUnavailableHint},
		{name: "graph status, project index null", answer: `{"projects":{"nodes":[{"orbit":` + projectNoIndexAnswer + `}]}}`, call: callGraphStatusByProjectID, hint: graphStatusUnavailableHint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQL(w, http.StatusOK, tc.answer)
			}))
			err := tc.call(context.Background(), client)
			if !errors.Is(err, ErrOrbitUnavailable) {
				t.Fatalf("error = %v, want it to wrap ErrOrbitUnavailable", err)
			}
			if !strings.Contains(err.Error(), "with null") || !strings.HasSuffix(err.Error(), "Suggestion: "+tc.hint) {
				t.Fatalf("error = %q, want the null answer named and the hint %q", err, tc.hint)
			}
		})
	}
}

// TestGraphQLHandlers_TopLevelErrors_AreReturned verifies that a GraphQL
// answer carrying errors[] is the action's error with GitLab's message, on
// every handler that reads GraphQL.
func TestGraphQLHandlers_TopLevelErrors_AreReturned(t *testing.T) {
	cases := []struct {
		name string
		call func(context.Context, *testClient) error
	}{
		{name: "status", call: callStatus},
		{name: "schema", call: callSchema},
		{name: "graph status", call: callGraphStatus},
		{name: "graph status by group id", call: callGraphStatusByGroupID},
		{name: "graph status by project id", call: callGraphStatusByProjectID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQLError(w, http.StatusOK, "rate limit exceeded")
			}))
			err := tc.call(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), "GraphQL errors: rate limit exceeded") {
				t.Fatalf("error = %v, want GitLab's GraphQL error", err)
			}
		})
	}
}

// TestGraphQLHandlers_HTTPFailure_CarriesTheOrbitHint verifies that an HTTP
// refusal of the GraphQL request is classified the way every Orbit failure
// is: a 404 with the availability hint, and a 403 without it, since GitLab
// answers an Orbit field it does not serve the user with null rather than a
// refusal of the request, so a 403 of the GraphQL endpoint is about the token
// or the account and says nothing about Orbit ([toolutil.IsPermissionRefusal]
// reads no GraphQL answer as a permission refusal).
func TestGraphQLHandlers_HTTPFailure_CarriesTheOrbitHint(t *testing.T) {
	handlers := []struct {
		name string
		call func(context.Context, *testClient) error
	}{
		{name: "status", call: callStatus},
		{name: "schema", call: callSchema},
		{name: "graph status", call: callGraphStatus},
		{name: "graph status by group id", call: callGraphStatusByGroupID},
		{name: "graph status by project id", call: callGraphStatusByProjectID},
	}
	statuses := []struct {
		status   int
		wantHint bool
	}{
		{status: http.StatusNotFound, wantHint: true},
		{status: http.StatusForbidden, wantHint: false},
	}
	for _, handler := range handlers {
		for _, answer := range statuses {
			t.Run(handler.name+", "+http.StatusText(answer.status), func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					readGraphQL(t, r)
					testutil.RespondJSON(w, answer.status, `{"message":"`+http.StatusText(answer.status)+`"}`)
				}))
				err := handler.call(context.Background(), client)
				if err == nil {
					t.Fatal("error = nil, want GitLab's refusal")
				}
				if got := strings.Contains(err.Error(), orbitUnavailableHint); got != answer.wantHint {
					t.Fatalf("error = %v, carries the availability hint = %t, want %t", err, got, answer.wantHint)
				}
			})
		}
	}
}

// schemaPageAnswer is one page of the schema, two node kinds and two edge
// kinds of GitLab.com's answer on 2026-10-29, with the properties of Project
// when they were asked for.
func schemaPageAnswer(withProperties bool) string {
	properties := ""
	if withProperties {
		properties = `,"properties":{"pageInfo":{"hasNextPage":false},"nodes":[` +
			`{"name":"id","data_type":"Int","nullable":false,"enum_values":[]},` +
			`{"name":"visibility_level","data_type":"Enum","nullable":true,"enum_values":["private","public"]}]}`
	}
	return `{"orbit":{"schema":{` +
		`"nodes":{"pageInfo":{"hasNextPage":false,"endCursor":"Mg"},"nodes":[` +
		`{"name":"Project","description":"A project","primary_key":"id","label_field":"full_path","domain":{"name":"core"},"display":{"color":"#22C55E","size":40}` + properties + `},` +
		`{"name":"Branch","description":"A Git branch","primary_key":"id","label_field":"name","domain":{"name":"source_code"},"display":null` + properties + `}]},` +
		`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":"Mg"},"nodes":[` +
		`{"name":"IN_PROJECT","description":"Entity in a project","variants":{"pageInfo":{"hasNextPage":false},"nodes":[` +
		`{"source":{"name":"Branch"},"target":{"name":"Project"}},{"source":{"name":"Job"},"target":{"name":"Project"}}]}},` +
		`{"name":"CONTAINS","description":"Containment","variants":{"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}}`
}

// TestSchema_Structured_ProjectsTheGraphQLSchema verifies the structured
// schema: the request asks for no property without expand, and the answer is
// projected into nodes with their domain, key, label field and display hints,
// domains grouped from the nodes in GitLab's order, and edges with the pair of
// node kinds each endpoint names.
func TestSchema_Structured_ProjectsTheGraphQLSchema(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := readGraphQL(t, r)
		if req.Query != querySchema {
			t.Errorf("query = %q, want querySchema", req.Query)
		}
		if want := map[string]any{"withProperties": false}; !reflect.DeepEqual(req.Variables, want) {
			t.Errorf("variables = %v, want %v", req.Variables, want)
		}
		testutil.RespondGraphQL(w, http.StatusOK, schemaPageAnswer(false))
	}))

	out, err := Schema(context.Background(), client, SchemaInput{})
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	want := SchemaOutput{
		Domains: []SchemaDomain{{Name: "core", NodeNames: []string{"Project"}}, {Name: "source_code", NodeNames: []string{"Branch"}}},
		Nodes: []SchemaNode{
			{Name: "Project", Description: "A project", Domain: "core", PrimaryKey: "id", LabelField: "full_path", Display: &SchemaNodeDisplay{Color: "#22C55E", Size: 40}},
			{Name: "Branch", Description: "A Git branch", Domain: "source_code", PrimaryKey: "id", LabelField: "name"},
		},
		Edges: []SchemaEdge{
			{Name: "IN_PROJECT", Description: "Entity in a project", Variants: []SchemaEdgeVariant{{SourceType: "Branch", TargetType: "Project"}, {SourceType: "Job", TargetType: "Project"}}},
			{Name: "CONTAINS", Description: "Containment"},
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Schema() = %+v, want %+v", out, want)
	}
}

// TestSchema_Expand_KeepsTheNamedNodesProperties verifies expand: it asks for
// the properties and keeps them on the named node kinds alone, matched without
// regard to case, under both input names of the format.
func TestSchema_Expand_KeepsTheNamedNodesProperties(t *testing.T) {
	inputs := []struct {
		name  string
		input SchemaInput
	}{
		{name: "no format", input: SchemaInput{Expand: []string{" project "}}},
		{name: "format raw", input: SchemaInput{Expand: []string{"Project"}, Format: "raw"}},
		{name: "response_format json", input: SchemaInput{Expand: []string{"PROJECT"}, ResponseFormat: "json"}},
	}
	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req := readGraphQL(t, r)
				if req.Variables["withProperties"] != true {
					t.Errorf("withProperties = %v, want true when expand names a node", req.Variables["withProperties"])
				}
				testutil.RespondGraphQL(w, http.StatusOK, schemaPageAnswer(true))
			}))
			out, err := Schema(context.Background(), client, tc.input)
			if err != nil {
				t.Fatalf("Schema() error: %v", err)
			}
			wantProject := []SchemaProperty{
				{Name: "id", DataType: "Int", EnumValues: []string{}},
				{Name: "visibility_level", DataType: "Enum", Nullable: true, EnumValues: []string{"private", "public"}},
			}
			if !reflect.DeepEqual(out.Nodes[0].Properties, wantProject) {
				t.Fatalf("Project properties = %+v, want %+v", out.Nodes[0].Properties, wantProject)
			}
			if out.Nodes[1].Properties != nil {
				t.Fatalf("Branch properties = %+v, want none for a node expand did not name", out.Nodes[1].Properties)
			}
		})
	}
}

// TestSchema_Paging_FollowsBothCursors verifies that a schema answered in more
// than one page is read whole: the next request carries each connection's last
// cursor, and the pages are joined in order.
func TestSchema_Paging_FollowsBothCursors(t *testing.T) {
	var calls atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := readGraphQL(t, r)
		switch calls.Add(1) {
		case 1:
			if _, has := req.Variables["nodesAfter"]; has {
				t.Errorf("first page variables = %v, want no cursor", req.Variables)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"orbit":{"schema":{`+
				`"nodes":{"pageInfo":{"hasNextPage":true,"endCursor":"N1"},"nodes":[{"name":"A","description":null,"primary_key":null,"label_field":null,"domain":{"name":"d"},"display":null}]},`+
				`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":"E1"},"nodes":[{"name":"X","description":null,"variants":{"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}}`)
		default:
			if want := map[string]any{"nodesAfter": "N1", "edgesAfter": "E1", "withProperties": false}; !reflect.DeepEqual(req.Variables, want) {
				t.Errorf("second page variables = %v, want %v", req.Variables, want)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"orbit":{"schema":{`+
				`"nodes":{"pageInfo":{"hasNextPage":false,"endCursor":"N2"},"nodes":[{"name":"B","description":null,"primary_key":null,"label_field":null,"domain":{"name":"d"},"display":null}]},`+
				`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}`)
		}
	}))

	out, err := Schema(context.Background(), client, SchemaInput{})
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	if calls.Load() != 2 || len(out.Nodes) != 2 || out.Nodes[1].Name != "B" || len(out.Edges) != 1 {
		t.Fatalf("Schema() = %+v after %d requests, want both pages joined", out, calls.Load())
	}
	if want := []SchemaDomain{{Name: "d", NodeNames: []string{"A", "B"}}}; !reflect.DeepEqual(out.Domains, want) {
		t.Fatalf("domains = %+v, want %+v", out.Domains, want)
	}
}

// TestSchema_CursorThatNeverEnds_StopsAtThePageBound verifies that a schema
// whose cursor never ends is not followed forever: the call stops after
// maxSchemaPages requests and says so.
func TestSchema_CursorThatNeverEnds_StopsAtThePageBound(t *testing.T) {
	var calls atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readGraphQL(t, r)
		calls.Add(1)
		testutil.RespondGraphQL(w, http.StatusOK, `{"orbit":{"schema":{`+
			`"nodes":{"pageInfo":{"hasNextPage":true,"endCursor":"N"},"nodes":[]},`+
			`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}`)
	}))
	_, err := Schema(context.Background(), client, SchemaInput{})
	if err == nil || !strings.Contains(err.Error(), "did not end within 20 pages") {
		t.Fatalf("Schema() error = %v, want the page bound named", err)
	}
	if calls.Load() != maxSchemaPages {
		t.Fatalf("requests = %d, want %d", calls.Load(), maxSchemaPages)
	}
}

// TestSchema_ALaterPageRefused_FailsTheCall verifies that a refusal of a
// later page fails the call rather than publishing the pages before it as the
// whole schema.
func TestSchema_ALaterPageRefused_FailsTheCall(t *testing.T) {
	var calls atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readGraphQL(t, r)
		if calls.Add(1) == 1 {
			testutil.RespondGraphQL(w, http.StatusOK, `{"orbit":{"schema":{`+
				`"nodes":{"pageInfo":{"hasNextPage":true,"endCursor":"N1"},"nodes":[]},`+
				`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusServiceUnavailable, `{"message":"503"}`)
	}))
	_, err := Schema(context.Background(), client, SchemaInput{})
	if err == nil || !strings.Contains(err.Error(), "temporarily unavailable") {
		t.Fatalf("Schema() error = %v, want the refusal of the second page", err)
	}
}

// TestSchema_NestedConnectionNotReadWhole_IsAnError verifies that a node's
// properties or an edge's endpoints that one page did not hold make the call
// fail naming the entry, rather than publishing part of it.
func TestSchema_NestedConnectionNotReadWhole_IsAnError(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{
			name: "properties",
			answer: `{"orbit":{"schema":{` +
				`"nodes":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"name":"Big","description":null,"primary_key":null,"label_field":null,"domain":{"name":"d"},"display":null,` +
				`"properties":{"pageInfo":{"hasNextPage":true},"nodes":[]}}]},` +
				`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}`,
			want: "the properties of node Big holds more entries",
		},
		{
			name: "endpoints",
			answer: `{"orbit":{"schema":{` +
				`"nodes":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]},` +
				`"edges":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"name":"WIDE","description":null,"variants":{"pageInfo":{"hasNextPage":true},"nodes":[]}}]}}}}`,
			want: "the endpoints of edge WIDE holds more entries",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQL(w, http.StatusOK, tc.answer)
			}))
			_, err := Schema(context.Background(), client, SchemaInput{Expand: []string{"Big"}})
			if !errors.Is(err, errSchemaNested) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Schema() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// graphStatusAnswer is GitLab.com's answer for jmrp-oss/orbit-fixtures on
// 2026-10-29 at 09:09 UTC, before Orbit stopped answering the fixture
// namespace, in the shape the graph status documents ask for and cut to two
// areas and two domains. access and trial are null, as GitLab answers them for
// a subgroup.
const graphStatusAnswer = `{"enabled":true,"enabled_at":"2026-06-11T23:19:22Z","access":null,"trial":null,` +
	`"indexing":{"state":"READY","last_completed_at":"2026-10-29T08:24:01Z",` +
	`"projects":{"indexed":2,"total_known":2,"gaps":0,"not_indexed":0,"total":2},` +
	`"areas":[{"name":"ci","state":"READY"},{"name":"core","state":"INDEXING"}]},` +
	`"itemCounts":{"domains":[{"name":"ci","items":[{"name":"Job","count":"18"},{"name":"Pipeline","count":"5"}]},` +
	`{"name":"core","items":[{"name":"Project","count":"2"}]}]}}`

// graphStatusWant is what [GraphStatus] publishes for [graphStatusAnswer].
func graphStatusWant() GraphStatusOutput {
	enabled := true
	return GraphStatusOutput{
		Enabled:   &enabled,
		EnabledAt: "2026-06-11T23:19:22Z",
		Projects:  &GraphStatusProjects{Indexed: 2, TotalKnown: 2, Total: 2},
		Domains: []GraphStatusDomain{
			{Name: "ci", Items: []GraphStatusDomainItem{{Name: "Job", Count: 18}, {Name: "Pipeline", Count: 5}}},
			{Name: "core", Items: []GraphStatusDomainItem{{Name: "Project", Count: 2}}},
		},
		Indexing: &GraphStatusIndexing{
			State:           "READY",
			LastCompletedAt: "2026-10-29T08:24:01Z",
			Areas:           []GraphStatusArea{{Name: "ci", State: "READY"}, {Name: "core", State: "INDEXING"}},
		},
	}
}

// TestGraphStatus_EachScope_AsksTheMatchingDocument verifies the three ways
// a group or project is named: a full path asked as both a group and a
// project, whichever GitLab answers, and an ID asked as the global ID of the
// group or project it names. Each answer is projected the same way, the
// project counts at the top and each entity count parsed from GitLab's BigInt
// string.
func TestGraphStatus_EachScope_AsksTheMatchingDocument(t *testing.T) {
	cases := []struct {
		name      string
		input     GraphStatusInput
		query     string
		variables map[string]any
		answer    string
	}{
		{
			name: "full path of a group", input: GraphStatusInput{FullPath: " jmrp-oss/orbit-fixtures "},
			query: queryGraphStatusByPath, variables: map[string]any{"fullPath": "jmrp-oss/orbit-fixtures"},
			answer: `{"group":{"orbit":` + graphStatusAnswer + `},"project":null}`,
		},
		{
			name: "full path of a project", input: GraphStatusInput{FullPath: "jmrp-oss/orbit-fixtures/kg-fixtures"},
			query: queryGraphStatusByPath, variables: map[string]any{"fullPath": "jmrp-oss/orbit-fixtures/kg-fixtures"},
			answer: `{"group":null,"project":{"orbit":` + graphStatusAnswer + `}}`,
		},
		{
			name: "namespace id", input: GraphStatusInput{NamespaceID: 145841973},
			query: queryGraphStatusByGroupID, variables: map[string]any{"ids": []any{"gid://gitlab/Group/145841973"}},
			answer: `{"groups":{"nodes":[{"orbit":` + graphStatusAnswer + `}]}}`,
		},
		{
			name: "project id", input: GraphStatusInput{ProjectID: 83234203},
			query: queryGraphStatusByProjectID, variables: map[string]any{"ids": []any{"gid://gitlab/Project/83234203"}},
			answer: `{"projects":{"nodes":[{"orbit":` + graphStatusAnswer + `}]}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req := readGraphQL(t, r)
				if req.Query != tc.query {
					t.Errorf("query = %q, want %q", req.Query, tc.query)
				}
				if !reflect.DeepEqual(req.Variables, tc.variables) {
					t.Errorf("variables = %v, want %v", req.Variables, tc.variables)
				}
				testutil.RespondGraphQL(w, http.StatusOK, tc.answer)
			}))
			out, err := GraphStatus(context.Background(), client, tc.input)
			if err != nil {
				t.Fatalf("GraphStatus() error: %v", err)
			}
			if want := graphStatusWant(); !reflect.DeepEqual(out, want) {
				t.Fatalf("GraphStatus() = %+v, want %+v", out, want)
			}
		})
	}
}

// TestGraphStatus_NothingVisible_NamesTheScope verifies the answer to a path
// or ID that names nothing the token can read: GitLab answers both roots null,
// or an empty list, and the error names what was asked.
func TestGraphStatus_NothingVisible_NamesTheScope(t *testing.T) {
	cases := []struct {
		name   string
		input  GraphStatusInput
		answer string
		want   string
	}{
		{name: "full path", input: GraphStatusInput{FullPath: "nobody/here"}, answer: `{"group":null,"project":null}`, want: "group or project nobody/here"},
		{name: "namespace id", input: GraphStatusInput{NamespaceID: 7}, answer: `{"groups":{"nodes":[]}}`, want: "group 7"},
		{name: "project id", input: GraphStatusInput{ProjectID: 9}, answer: `{"projects":null}`, want: "project 9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQL(w, http.StatusOK, tc.answer)
			}))
			_, err := GraphStatus(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), "no "+tc.want+" visible to this token") {
				t.Fatalf("GraphStatus() error = %v, want %q named", err, tc.want)
			}
		})
	}
}

// TestGraphStatus_ItemCountsWithoutIndexing_PublishesTheCounts verifies that a
// namespace GitLab answers item counts for and no indexing state publishes the
// counts and leaves the rest out, rather than reporting Orbit unavailable.
func TestGraphStatus_ItemCountsWithoutIndexing_PublishesTheCounts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readGraphQL(t, r)
		testutil.RespondGraphQL(w, http.StatusOK, `{"group":null,"project":{"orbit":{"enabled":null,"enabled_at":null,"indexing":null,`+
			`"itemCounts":{"domains":[{"name":"core","items":[{"name":"Project","count":"not a number"}]}]}}}}`)
	}))
	out, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "a/b"})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	want := GraphStatusOutput{Domains: []GraphStatusDomain{{Name: "core", Items: []GraphStatusDomainItem{{Name: "Project"}}}}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("GraphStatus() = %+v, want %+v", out, want)
	}
}

// The Orbit namespaces GitLab.com answered the graph status documents with, as
// each was asked, once access and trial were in them.
const (
	// licensedTopLevelAnswer is jmrp-oss at 09:09 UTC on 2026-10-29, before
	// Orbit's general availability reached it: LICENSED through the free beta,
	// no trial, and its index, cut to one area and one domain.
	licensedTopLevelAnswer = `{"enabled":true,"enabled_at":"2026-06-11T23:19:22Z",` +
		`"access":{"state":"LICENSED","reason":"BETA","turn_on_denial_reason":null},"trial":null,` +
		`"indexing":{"state":"READY","last_completed_at":"2026-10-29T08:24:01Z",` +
		`"projects":{"indexed":2,"total_known":2,"gaps":0,"not_indexed":0,"total":2},"areas":[{"name":"core","state":"READY"}]},` +
		`"itemCounts":{"domains":[{"name":"core","items":[{"name":"Group","count":"2"},{"name":"Project","count":"3"}]}]}}`
	// notEligibleAnswer is jmrp-oss at 09:22 UTC on 2026-10-29, the first
	// answer after the flip: TRIAL_NOT_ELIGIBLE, a trial it cannot start with
	// no credit left, and no index.
	notEligibleAnswer = `{"enabled":true,"enabled_at":"2026-06-11T23:19:22Z",` +
		`"access":{"state":"TRIAL_NOT_ELIGIBLE","reason":null,"turn_on_denial_reason":null},` +
		`"trial":{"state":"NOT_ELIGIBLE","expires_on":null,"credits":{"remaining":0.0,"total":null},"offer":null},` +
		`"indexing":null,"itemCounts":null}`
	// purchaseRequiredAnswer is jmrp-oss at 16:58 UTC the same day, once Orbit
	// had been turned off for it: the same access, and the reason the token's
	// user cannot turn Orbit back on.
	purchaseRequiredAnswer = `{"enabled":false,"enabled_at":null,` +
		`"access":{"state":"TRIAL_NOT_ELIGIBLE","reason":null,"turn_on_denial_reason":"PURCHASE_REQUIRED"},` +
		`"trial":{"state":"NOT_ELIGIBLE","expires_on":null,"credits":{"remaining":0.0,"total":null},"offer":null},` +
		`"indexing":null,"itemCounts":null}`
	// subgroupNoIndexAnswer is jmrp-oss/orbit-fixtures at 16:58 UTC on
	// 2026-10-29: a subgroup, whose access and trial GitLab answers null.
	subgroupNoIndexAnswer = `{"enabled":false,"enabled_at":null,"access":null,"trial":null,"indexing":null,"itemCounts":null}`
	// projectNoIndexAnswer is jmrp-oss/orbit-fixtures/kg-fixtures at the same
	// time: a project, for which GitLab sends no enabled flag either.
	projectNoIndexAnswer = `{"enabled":null,"enabled_at":null,"access":null,"trial":null,"indexing":null,"itemCounts":null}`
	// eligibleAnswer is the shape GitLab's schema gives a top-level group that
	// can start a trial, with every field of the offer and the credits set: no
	// namespace this repository can reach answered one, so the values are the
	// test's own.
	eligibleAnswer = `{"enabled":false,"enabled_at":null,` +
		`"access":{"state":"TRIAL_ELIGIBLE","reason":null,"turn_on_denial_reason":"TRIAL_REQUIRED"},` +
		`"trial":{"state":"ELIGIBLE","expires_on":null,"credits":{"remaining":null,"total":null},"offer":{"duration_days":30,"initial_credits":500.5}},` +
		`"indexing":null,"itemCounts":null}`
)

// TestGraphStatus_TopLevelGroup_PublishesAccessAndTrial verifies that the
// access and trial GitLab answers for a top-level group are published as it
// spelled them: beside the index for a licensed group, and alone for a group
// GitLab.com serves no index, whose access is the answer rather than an
// error. A remaining credit count of zero is kept and a null total is left
// out.
func TestGraphStatus_TopLevelGroup_PublishesAccessAndTrial(t *testing.T) {
	enabled, disabled := true, false
	zero, offerCredits := 0.0, 500.5
	thirty := int64(30)
	notEligibleTrial := &GraphStatusTrial{State: "NOT_ELIGIBLE", Credits: &GraphStatusTrialCredits{Remaining: &zero}}
	cases := []struct {
		name   string
		input  GraphStatusInput
		answer string
		want   GraphStatusOutput
	}{
		{
			name: "licensed, by full path", input: GraphStatusInput{FullPath: "jmrp-oss"},
			answer: `{"group":{"orbit":` + licensedTopLevelAnswer + `},"project":null}`,
			want: GraphStatusOutput{
				Enabled: &enabled, EnabledAt: "2026-06-11T23:19:22Z",
				Access:   &GraphStatusAccess{State: AccessLicensed, Reason: "BETA"},
				Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 2, Total: 2},
				Domains:  []GraphStatusDomain{{Name: "core", Items: []GraphStatusDomainItem{{Name: "Group", Count: 2}, {Name: "Project", Count: 3}}}},
				Indexing: &GraphStatusIndexing{State: "READY", LastCompletedAt: "2026-10-29T08:24:01Z", Areas: []GraphStatusArea{{Name: "core", State: "READY"}}},
			},
		},
		{
			name: "not eligible, by full path", input: GraphStatusInput{FullPath: "jmrp-oss"},
			answer: `{"group":{"orbit":` + notEligibleAnswer + `},"project":null}`,
			want: GraphStatusOutput{
				Enabled: &enabled, EnabledAt: "2026-06-11T23:19:22Z",
				Access: &GraphStatusAccess{State: "TRIAL_NOT_ELIGIBLE"}, Trial: notEligibleTrial,
			},
		},
		{
			name: "purchase required, by group id", input: GraphStatusInput{NamespaceID: 134059988},
			answer: `{"groups":{"nodes":[{"orbit":` + purchaseRequiredAnswer + `}]}}`,
			want: GraphStatusOutput{
				Enabled: &disabled,
				Access:  &GraphStatusAccess{State: "TRIAL_NOT_ELIGIBLE", TurnOnDenialReason: "PURCHASE_REQUIRED"}, Trial: notEligibleTrial,
			},
		},
		{
			name: "trial eligible with an offer", input: GraphStatusInput{NamespaceID: 1},
			answer: `{"groups":{"nodes":[{"orbit":` + eligibleAnswer + `}]}}`,
			want: GraphStatusOutput{
				Enabled: &disabled,
				Access:  &GraphStatusAccess{State: "TRIAL_ELIGIBLE", TurnOnDenialReason: "TRIAL_REQUIRED"},
				Trial: &GraphStatusTrial{
					State: "ELIGIBLE", Credits: &GraphStatusTrialCredits{},
					Offer: &GraphStatusTrialOffer{DurationDays: &thirty, InitialCredits: &offerCredits},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQL(w, http.StatusOK, tc.answer)
			}))
			out, err := GraphStatus(context.Background(), client, tc.input)
			if err != nil {
				t.Fatalf("GraphStatus() error: %v", err)
			}
			if !reflect.DeepEqual(out, tc.want) {
				t.Fatalf("GraphStatus() = %+v, want %+v", out, tc.want)
			}
		})
	}
}

// TestGraphStatus_AccessAndTrial_PublishedUnderGitLabsNames verifies the JSON
// a caller reads: GitLab's field names in snake case, its enums as it spells
// them, the zero remaining credits written and every null left out.
func TestGraphStatus_AccessAndTrial_PublishedUnderGitLabsNames(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readGraphQL(t, r)
		testutil.RespondGraphQL(w, http.StatusOK, `{"group":{"orbit":`+purchaseRequiredAnswer+`},"project":null}`)
	}))
	out, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "jmrp-oss"})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	want := `{"enabled":false,` +
		`"access":{"state":"TRIAL_NOT_ELIGIBLE","turn_on_denial_reason":"PURCHASE_REQUIRED"},` +
		`"trial":{"state":"NOT_ELIGIBLE","credits":{"remaining":0}}}`
	if string(encoded) != want {
		t.Fatalf("published = %s, want %s", encoded, want)
	}
}

// TestGraphStatusAccess_Serves_OnlyLicensedOrTrialActive verifies the two
// access states GitLab.com serves an index under, and that a group with no
// access state, a subgroup or a project, is not read as one.
func TestGraphStatusAccess_Serves_OnlyLicensedOrTrialActive(t *testing.T) {
	cases := []struct {
		name   string
		access *GraphStatusAccess
		want   bool
	}{
		{name: "licensed", access: &GraphStatusAccess{State: AccessLicensed}, want: true},
		{name: "trial active", access: &GraphStatusAccess{State: AccessTrialActive}, want: true},
		{name: "trial not eligible", access: &GraphStatusAccess{State: "TRIAL_NOT_ELIGIBLE"}},
		{name: "unlicensed", access: &GraphStatusAccess{State: "UNLICENSED", Reason: "NO_ACCESS"}},
		{name: "no access state", access: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.access.Serves(); got != tc.want {
				t.Fatalf("Serves() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestGraphStatus_InvalidInput_IsRefusedBeforeSending verifies the checks
// [GraphStatus] makes before asking GitLab: exactly one scope, no negative ID,
// and a response format the REST route took.
func TestGraphStatus_InvalidInput_IsRefusedBeforeSending(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for invalid input: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	cases := []struct {
		name  string
		input GraphStatusInput
		want  string
	}{
		{name: "no scope", input: GraphStatusInput{}, want: "set exactly one of namespace_id, project_id, or full_path"},
		{name: "blank path", input: GraphStatusInput{FullPath: "  "}, want: "set exactly one"},
		{name: "two scopes", input: GraphStatusInput{NamespaceID: 1, FullPath: "a"}, want: "set exactly one"},
		{name: "project and path", input: GraphStatusInput{ProjectID: 1, FullPath: "a"}, want: "set exactly one"},
		{name: "negative namespace", input: GraphStatusInput{NamespaceID: -1}, want: "namespace_id must not be negative"},
		{name: "negative project", input: GraphStatusInput{ProjectID: -1}, want: "project_id must not be negative"},
		{name: "unknown format", input: GraphStatusInput{FullPath: "a", StructuredFormatInput: StructuredFormatInput{ResponseFormat: "gql"}}, want: "invalid response_format: use raw or llm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GraphStatus(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("GraphStatus() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestGraphQLDocuments_CostWhatGitLabMeasured holds each GraphQL document's
// estimated complexity, which GitLab refuses above its limit, to the figure
// GitLab.com reported for it: each document sent with queryComplexity { score
// limit } beside its selection, which GitLab answered 3 more than the figures
// below (the selection costs 3 on its own, measured the same minute), against
// a limit of 250. The service status was measured at 14:41 UTC on 2026-10-29,
// and the three graph status documents at 16:58 UTC the same day, once they
// asked for access and trial (73, 40 and 39 with queryComplexity). A change to
// a selection changes the estimate and sends whoever made it to measure again.
// The schema document is not held here: its properties carry @include, which
// the estimate does not evaluate, and GitLab.com reported 39 for it with
// properties and 31 without, well under the limit.
func TestGraphQLDocuments_CostWhatGitLabMeasured(t *testing.T) {
	cases := []struct {
		name      string
		document  string
		variables map[string]any
		measured  int
	}{
		{name: "service status", document: queryServiceStatus, measured: 7},
		{name: "graph status by group id", document: queryGraphStatusByGroupID, variables: map[string]any{"ids": []any{"gid://gitlab/Group/1"}}, measured: 37},
		{name: "graph status by project id", document: queryGraphStatusByProjectID, variables: map[string]any{"ids": []any{"gid://gitlab/Project/1"}}, measured: 36},
		{name: "graph status by path", document: queryGraphStatusByPath, variables: map[string]any{"fullPath": "a"}, measured: 70},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cost, err := testutil.GitLabQueryComplexity(tc.document, tc.variables)
			if err != nil {
				t.Fatalf("GitLabQueryComplexity() error: %v", err)
			}
			if cost != tc.measured {
				t.Errorf("complexity = %d by the estimate, and GitLab.com measured %d: measure it again and record the figure", cost, tc.measured)
			}
			if cost > testutil.GitLabAuthenticatedMaxComplexity {
				t.Errorf("complexity = %d, over the %d GitLab refuses a query above", cost, testutil.GitLabAuthenticatedMaxComplexity)
			}
		})
	}
}

// testClient is the client every handler takes, named short for the case
// tables.
type testClient = gitlabclient.Client

// callStatus calls [Status] with an input that sends a request, for the case
// tables that hold every GraphQL handler to the same answer.
func callStatus(ctx context.Context, client *testClient) error {
	_, err := Status(ctx, client, StatusInput{})
	return err
}

// callSchema calls [Schema] for the structured schema.
func callSchema(ctx context.Context, client *testClient) error {
	_, err := Schema(ctx, client, SchemaInput{})
	return err
}

// callGraphStatus calls [GraphStatus] by the fixture namespace's path.
func callGraphStatus(ctx context.Context, client *testClient) error {
	_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "jmrp-oss/orbit-fixtures"})
	return err
}

// callGraphStatusByGroupID calls [GraphStatus] by the fixture group's ID.
func callGraphStatusByGroupID(ctx context.Context, client *testClient) error {
	_, err := GraphStatus(ctx, client, GraphStatusInput{NamespaceID: 145841973})
	return err
}

// callGraphStatusByProjectID calls [GraphStatus] by a fixture project's ID.
func callGraphStatusByProjectID(ctx context.Context, client *testClient) error {
	_, err := GraphStatus(ctx, client, GraphStatusInput{ProjectID: 83234203})
	return err
}
