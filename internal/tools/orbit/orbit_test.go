package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestStatus_Success_ExpectedOutput verifies that [Status] decodes the Orbit
// status endpoint, including component replica metadata, from a successful API response.
//
// The test mocks a healthy response from /api/v4/orbit/status with a clickhouse component.
// It asserts that the output status, version, and component replica counts match the mock.
// This ensures the handler correctly parses Orbit status and component details.
func TestStatus_Success_ExpectedOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.RespondJSON(w, http.StatusOK, `{
			"status": "healthy",
			"timestamp": "2026-04-28T12:00:00Z",
			"version": "0.5.0",
			"components": [
				{"name": "clickhouse", "status": "healthy", "replicas": {"ready": 3, "desired": 3}, "metrics": {"kind": "Deployment"}}
			]
		}`)
	}))

	out, err := Status(context.Background(), client, StatusInput{})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if out.Status != "healthy" || out.Version != "0.5.0" {
		t.Fatalf("Status() = status %q version %q, want healthy 0.5.0", out.Status, out.Version)
	}
	if len(out.Components) != 1 || out.Components[0].Replicas.Ready != 3 {
		t.Fatalf("Status() components = %+v, want clickhouse replicas", out.Components)
	}
}

// TestSchema_WithExpandAndFormat_ForwardsQuery verifies that [Schema] forwards
// expand and format query parameters and decodes schema domains, nodes, and edges.
//
// The test mocks a response from /api/v4/orbit/schema with expand and format parameters.
// It asserts that the output schema version, domains, and edges are decoded as expected.
// This ensures query parameter forwarding and schema decoding are correct.
func TestSchema_WithExpandAndFormat_ForwardsQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		testutil.AssertQueryParam(t, r, "expand", "User,Project")
		testutil.AssertQueryParam(t, r, "format", "llm")
		testutil.RespondJSON(w, http.StatusOK, `{
			"schema_version": "1.0",
			"domains": [{"name": "core", "description": "Core entities", "node_names": ["User", "Project"]}],
			"nodes": [{"name": "User"}],
			"edges": [{"name": "AUTHORED", "description": "Authorship", "variants": [{"source_type": "User", "target_type": "Issue"}]}]
		}`)
	}))

	out, err := Schema(context.Background(), client, SchemaInput{Expand: []string{"User", "Project"}, Format: "llm"})
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	if out.SchemaVersion != "1.0" || len(out.Domains) != 1 || len(out.Edges) != 1 {
		t.Fatalf("Schema() = %+v, want decoded schema", out)
	}
}

// TestSchema_ResponseFormatAlias_ForwardsFormat verifies that [Schema] accepts
// response_format as a compatibility alias while forwarding GitLab's format query parameter.
//
// The test provides ResponseFormat in the input and asserts that only the format query param is sent.
// It ensures backward compatibility and correct parameter mapping.
func TestSchema_ResponseFormatAlias_ForwardsFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		testutil.AssertQueryParam(t, r, "format", "llm")
		if got := r.URL.Query().Get("response_format"); got != "" {
			t.Fatalf("response_format query parameter = %q, want empty", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0"}`)
	}))

	out, err := Schema(context.Background(), client, SchemaInput{ResponseFormat: "llm"})
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	if out.SchemaVersion != "1.0" {
		t.Fatalf("Schema() = %+v, want schema version 1.0", out)
	}
}

// TestTools_Success_ExpectedOutput verifies that [Tools] decodes the Orbit tools
// catalog returned by the GitLab API.
//
// The test mocks a response from /api/v4/orbit/tools and asserts the tool name and count.
// This ensures the handler parses the tools catalog correctly.
func TestTools_Success_ExpectedOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/tools")
		testutil.RespondJSON(w, http.StatusOK, `[
			{"name": "query_graph", "description": "Execute graph queries", "parameters": {"type": "object"}}
		]`)
	}))

	out, err := Tools(context.Background(), client, ToolsInput{})
	if err != nil {
		t.Fatalf("Tools() error: %v", err)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "query_graph" {
		t.Fatalf("Tools() = %+v, want query_graph", out.Tools)
	}
}

// TestDSL_WithResponseFormat_ReturnsRawBody verifies that [DSL] forwards the
// requested response format and returns the Orbit DSL response verbatim.
//
// The test mocks a text/plain response from /api/v4/orbit/schema/dsl and checks
// that the output contains the expected DSL content and response format.
func TestDSL_WithResponseFormat_ReturnsRawBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema/dsl")
		testutil.AssertQueryParam(t, r, "response_format", "llm")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("@dsl\nquery_type: traversal\n"))
	}))

	out, err := DSL(context.Background(), client, DSLInput{ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"}})
	if err != nil {
		t.Fatalf("DSL() error: %v", err)
	}
	if out.ResponseFormat != "llm" || !strings.Contains(out.Content, "@dsl") {
		t.Fatalf("DSL() = %+v, want llm raw DSL content", out)
	}
}

// TestQuery_Success_ForwardsRawQuery verifies that [Query] posts the raw graph
// query object and response format before decoding the query result.
//
// The test posts a traversal query and asserts the request body and decoded result.
// This ensures the handler sends the correct payload and parses the response.
func TestQuery_Success_ForwardsRawQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if string(got["query"]) != `{"query_type":"traversal"}` {
			t.Fatalf("query body = %s, want traversal query", got["query"])
		}
		if string(got["response_format"]) != `"raw"` {
			t.Fatalf("response_format = %s, want raw", got["response_format"])
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"result": [{"_id": "1", "_type": "Project"}],
			"query_type": "traversal",
			"raw_query_strings": ["SELECT ..."],
			"row_count": 1
		}`)
	}))

	out, err := Query(context.Background(), client, QueryInput{
		Query:               map[string]any{"query_type": "traversal"},
		ResponseFormatInput: ResponseFormatInput{ResponseFormat: "raw"},
	})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	resultJSON, err := json.Marshal(out.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if out.QueryType != "traversal" || out.RowCount != 1 || !strings.Contains(string(resultJSON), "Project") {
		t.Fatalf("Query() = %+v, want traversal result", out)
	}
}

// TestQuery_LLMResponseFormat_UsesRawResponse verifies that [Query] uses the
// raw Orbit API path for LLM/text responses instead of JSON decoding.
//
// The test mocks a text/plain response and asserts that FormattedText is set and Result is nil.
func TestQuery_LLMResponseFormat_UsesRawResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if string(got["response_format"]) != `"llm"` {
			t.Fatalf("response_format = %s, want llm", got["response_format"])
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("@header\nProject(name: gitlab)\n"))
	}))

	out, err := Query(context.Background(), client, QueryInput{
		Query:               map[string]any{"query_type": "traversal"},
		ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"},
	})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	if !strings.Contains(out.FormattedText, "@header") || out.Result != nil || out.QueryType != "traversal" {
		t.Fatalf("Query() = %+v, want raw formatted text with query type", out)
	}
}

// TestQuery_LLMResponseFormat_RawError verifies that raw Orbit API failures are
// wrapped as errors instead of falling back to JSON decoding.
//
// The test mocks a 500 error and asserts that an error is returned.
func TestQuery_LLMResponseFormat_RawError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := Query(context.Background(), client, QueryInput{
		Query:               map[string]any{"query_type": "traversal"},
		ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestQueryType_NonString_ReturnsEmpty verifies that queryType ignores malformed query_type values.
//
// The test passes a non-string query_type and expects an empty result. This keeps raw LLM response
// metadata safe when user-provided Orbit query JSON contains an unexpected value type.
func TestQueryType_NonString_ReturnsEmpty(t *testing.T) {
	if got := queryType(map[string]any{"query_type": 42}); got != "" {
		t.Fatalf("queryType() = %q, want empty", got)
	}
}

// TestResponseFormatName_NilDefaultsToRaw verifies that responseFormatName treats a nil format as raw.
//
// The test covers the default branch used by Orbit handlers when the GitLab client option omits an
// explicit response format. This preserves the public raw-format default expected by the tool schema.
func TestResponseFormatName_NilDefaultsToRaw(t *testing.T) {
	if got := responseFormatName(nil); got != string(gl.OrbitResponseFormatRaw) {
		t.Fatalf("responseFormatName(nil) = %q, want raw", got)
	}
}

// TestGraphStatus_RequiresExactlyOneScope verifies that [GraphStatus] rejects
// missing, conflicting, or invalid scope inputs before making an HTTP request.
//
// The test runs subtests for missing, multiple, and negative namespace inputs and asserts validation errors.
func TestGraphStatus_RequiresExactlyOneScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for invalid input: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))

	tests := []struct {
		name  string
		input GraphStatusInput
	}{
		{name: "none", input: GraphStatusInput{}},
		{name: "multiple", input: GraphStatusInput{NamespaceID: 1, ProjectID: 2}},
		{name: "negative namespace", input: GraphStatusInput{NamespaceID: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GraphStatus(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("GraphStatus() error = nil, want validation error")
			}
		})
	}
}

// TestGraphStatus_Success_ByFullPath verifies that [GraphStatus] forwards a full
// project path and decodes project, domain, and indexing status data.
//
// The test mocks a response with indexed projects and domains and asserts the output fields.
func TestGraphStatus_Success_ByFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/graph_status")
		testutil.AssertQueryParam(t, r, "full_path", "gitlab-org/gitlab")
		testutil.RespondJSON(w, http.StatusOK, `{
			"projects": {"indexed": 3, "total_known": 4},
			"domains": [{"name": "SDLC", "items": [{"name": "MergeRequest", "count": 42}]}],
			"indexing": {"state": "indexed", "last_duration_ms": 99}
		}`)
	}))

	out, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	if out.Projects.Indexed != 3 || out.Indexing.State != "indexed" || out.Domains[0].Items[0].Count != 42 {
		t.Fatalf("GraphStatus() = %+v, want indexed graph status", out)
	}
}

// TestOrbit_ValidationErrors_ReturnActionableErrors verifies that client-side input
// validation for all Orbit handlers returns actionable error messages for invalid formats and malformed queries.
//
// The test runs table-driven subtests for various invalid inputs and asserts that the error contains the expected substring.
func TestOrbit_ValidationErrors_ReturnActionableErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for invalid input: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "invalid response format",
			call: func() error {
				_, err := Status(context.Background(), client, StatusInput{ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"}})
				return err
			},
			want: "use raw or llm",
		},
		{
			name: "empty query",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{})
				return err
			},
			want: "query",
		},
		{
			name: "unmarshalable query",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{"bad": func() {}}})
				return err
			},
			want: "JSON object",
		},
		{
			name: "invalid schema format",
			call: func() error {
				_, err := Schema(context.Background(), client, SchemaInput{Format: "xml"})
				return err
			},
			want: "use raw or llm",
		},
		{
			name: "conflicting schema formats",
			call: func() error {
				_, err := Schema(context.Background(), client, SchemaInput{Format: "raw", ResponseFormat: "llm"})
				return err
			},
			want: "must match",
		},
		{
			name: "invalid query response format",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{
					Query:               map[string]any{"query_type": "traversal"},
					ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"},
				})
				return err
			},
			want: "use raw or llm",
		},
		{
			name: "invalid dsl response format",
			call: func() error {
				_, err := DSL(context.Background(), client, DSLInput{ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"}})
				return err
			},
			want: "use raw or llm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

// TestOrbit_HTTPErrorHints_ReturnExpectedGuidance verifies that HTTP failures
// from Orbit endpoints include actionable enablement or retry guidance.
//
// The test runs table-driven subtests for various HTTP error codes and asserts that the error message contains the expected guidance.
func TestOrbit_HTTPErrorHints_ReturnExpectedGuidance(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		call   func(context.Context, *gitlabclient.Client) error
		want   string
	}{
		{
			name:   "not found",
			path:   "/api/v4/orbit/status",
			status: http.StatusNotFound,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Status(ctx, client, StatusInput{})
				return err
			},
			want: "knowledge_graph",
		},
		{
			name:   "schema forbidden",
			path:   "/api/v4/orbit/schema",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Schema(ctx, client, SchemaInput{})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "tools forbidden",
			path:   "/api/v4/orbit/tools",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Tools(ctx, client, ToolsInput{})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "dsl forbidden",
			path:   "/api/v4/orbit/schema/dsl",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := DSL(ctx, client, DSLInput{})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "forbidden",
			path:   "/api/v4/orbit/query",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Query(ctx, client, QueryInput{Query: map[string]any{"query_type": "traversal"}})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "bad request",
			path:   "/api/v4/orbit/graph_status",
			status: http.StatusBadRequest,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
				return err
			},
			want: "check the Orbit query",
		},
		{
			name:   "rate limited",
			path:   "/api/v4/orbit/query",
			status: http.StatusTooManyRequests,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Query(ctx, client, QueryInput{Query: map[string]any{"query_type": "traversal"}})
				return err
			},
			want: "rate-limited",
		},
		{
			name:   "service unavailable",
			path:   "/api/v4/orbit/graph_status",
			status: http.StatusServiceUnavailable,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
				return err
			},
			want: "temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, tt.path)
				testutil.RespondJSON(w, tt.status, `{"message":"error"}`)
			}))
			err := tt.call(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

// TestWrapOrbitErr_GenericError_UsesWrappedMessage verifies that non-HTTP Orbit
// errors retain both operation context and the original error message.
//
// The test calls wrapOrbitErr with a generic error and asserts that the returned error includes both the operation and the original message.
func TestWrapOrbitErr_GenericError_UsesWrappedMessage(t *testing.T) {
	err := wrapOrbitErr("orbit_status", errors.New("network unavailable"))
	if err == nil || !strings.Contains(err.Error(), "orbit_status") || !strings.Contains(err.Error(), "network unavailable") {
		t.Fatalf("wrapOrbitErr() = %v, want operation context and source error", err)
	}
}

// TestOrbitHandlers_ContextCancellation_ReturnsError verifies that all Orbit
// handlers respect context cancellation and do not issue requests after cancellation.
//
// The test uses a cancelled context and asserts that each handler returns a context error without making an HTTP request.
func TestOrbitHandlers_ContextCancellation_ReturnsError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called after context cancellation: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	ctx := testutil.CancelledCtx(t)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "status", call: func() error { _, err := Status(ctx, client, StatusInput{}); return err }},
		{name: "schema", call: func() error { _, err := Schema(ctx, client, SchemaInput{}); return err }},
		{name: "tools", call: func() error { _, err := Tools(ctx, client, ToolsInput{}); return err }},
		{name: "dsl", call: func() error { _, err := DSL(ctx, client, DSLInput{}); return err }},
		{name: "query", call: func() error {
			_, err := Query(ctx, client, QueryInput{Query: map[string]any{"query_type": "traversal"}})
			return err
		}},
		{name: "graph status", call: func() error {
			_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("handler error = nil, want context cancellation error")
			}
		})
	}
}

// TestOrbit_ActionSpecs_Metadata verifies that [ActionSpecs] returns canonical
// individual Orbit metadata with correct owner, gating, and tool names.
//
// The test asserts that all expected tool names are present and that the gating fields are set for GitLab.com premium.
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
		if !containsTool(names, want) {
			t.Fatalf("ActionSpecs() missing %s in %v", want, names)
		}
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
		t.Fatalf("gitlab_orbit meta registration missing gitlab_orbit in %v", names)
	}
}

// TestOrbit_RegisterMeta_UsesActionSpecs verifies that gitlab_orbit meta routes
// are projected from the canonical GitLab.com-only ActionSpec definitions.
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
			t.Fatalf("unexpected path: %s", r.URL.Path)
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
		{name: "gitlab_orbit_query", args: map[string]any{"query": map[string]any{"query_type": "traversal"}}},
		{name: "gitlab_orbit_graph_status", args: map[string]any{"full_path": "gitlab-org/gitlab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := routes[tt.name].Handler(t.Context(), tt.args)
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
		{name: "gitlab_orbit_query", args: map[string]any{"query": map[string]any{"query_type": "traversal"}}},
		{name: "gitlab_orbit_graph_status", args: map[string]any{"full_path": "gitlab-org/gitlab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := routes[tt.name].Handler(t.Context(), tt.args)
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

// TestFormatQueryMarkdown_IncludesPrettyJSON verifies that [FormatQueryMarkdown]
// renders structured result data inside a JSON code fence.
//
// The test asserts that the markdown output includes a JSON code block and the expected data.
func TestFormatQueryMarkdown_IncludesPrettyJSON(t *testing.T) {
	md := FormatQueryMarkdown(QueryOutput{
		QueryType: "traversal",
		RowCount:  1,
		Result:    []any{map[string]any{"name": "alpha"}},
	})
	if !strings.Contains(md, "```json") || !strings.Contains(md, "alpha") {
		t.Fatalf("FormatQueryMarkdown() = %q, want JSON result", md)
	}
}

// TestOrbitMarkdownFormatters_IncludeExpectedSections verifies that each Orbit
// markdown formatter emits the expected headings and key result sections.
//
// The test runs table-driven subtests for all formatters and asserts that the output contains the expected headings and content.
func TestOrbitMarkdownFormatters_IncludeExpectedSections(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []string
	}{
		{
			name: "status structured",
			md: FormatStatusMarkdown(StatusOutput{
				Status:  "healthy",
				Version: "0.5.0",
				Components: []StatusComponent{
					{Name: "clickhouse", Status: "healthy", Replicas: &StatusReplicas{Ready: 3, Desired: 3}},
				},
			}),
			want: []string{"Orbit Status", "clickhouse", "3/3"},
		},
		{
			name: "status formatted",
			md:   FormatStatusMarkdown(StatusOutput{FormattedText: "status: healthy"}),
			want: []string{"```text", "status: healthy"},
		},
		{
			name: "schema",
			md: FormatSchemaMarkdown(SchemaOutput{
				SchemaVersion: "1.0",
				Domains:       []SchemaDomain{{Name: "core", Description: "Core entities", NodeNames: []string{"User"}}},
				Nodes:         []any{map[string]any{"name": "User"}},
				Edges:         []SchemaEdge{{Name: "AUTHORED"}},
			}),
			want: []string{"Orbit Schema", "Schema version", "core"},
		},
		{
			name: "tools",
			md:   FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query_graph", Description: "Execute graph queries"}}}),
			want: []string{"Orbit Tools", "query_graph"},
		},
		{
			name: "dsl",
			md:   FormatDSLMarkdown(DSLOutput{ResponseFormat: "llm", Content: "@dsl\nquery_type: traversal"}),
			want: []string{"Orbit DSL", "```text", "query_type: traversal", "Use gitlab_orbit query"},
		},
		{
			name: "query formatted",
			md:   FormatQueryMarkdown(QueryOutput{FormattedText: "@header\nProject(name: gitlab)"}),
			want: []string{"Orbit Query Result", "```text", "@header", "Use gitlab_orbit graph_status"},
		},
		{
			name: "graph status structured",
			md: FormatGraphStatusMarkdown(GraphStatusOutput{
				Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3},
				Domains:  []GraphStatusDomain{{Name: "SDLC", Items: []GraphStatusDomainItem{{Name: "Issue", Count: 4}}}},
				Indexing: &GraphStatusIndexing{State: "indexed", LastDurationMs: 5},
			}),
			want: []string{"Orbit Graph Status", "Indexed projects", "SDLC"},
		},
		{
			name: "graph status formatted",
			md:   FormatGraphStatusMarkdown(GraphStatusOutput{FormattedText: "indexing: indexed"}),
			want: []string{"```text", "indexing: indexed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.want {
				if !strings.Contains(tt.md, want) {
					t.Fatalf("markdown = %q, want substring %q", tt.md, want)
				}
			}
			if tt.name == "query formatted" && strings.Contains(tt.md, "Query type") {
				t.Fatalf("markdown = %q, want formatted text without structured query fields", tt.md)
			}
		})
	}
}

// TestOrbitMarkdownFormatters_UseSafeFences verifies that formatter output uses
// longer fences when embedded query text contains triple backticks.
//
// The test asserts that the output uses four-backtick fences for embedded triple-backtick content.
func TestOrbitMarkdownFormatters_UseSafeFences(t *testing.T) {
	md := FormatQueryMarkdown(QueryOutput{
		QueryType:       "traversal",
		RawQueryStrings: []string{"MATCH (n) RETURN ```"},
		Result:          map[string]any{"text": "contains ``` fenced text"},
	})

	if !strings.Contains(md, "````text\nMATCH (n) RETURN ```\n````") {
		t.Fatalf("FormatQueryMarkdown() = %q, want four-backtick text fence", md)
	}
	if !strings.Contains(md, "````json\n") || !strings.Contains(md, "contains ``` fenced text") {
		t.Fatalf("FormatQueryMarkdown() = %q, want four-backtick JSON fence", md)
	}
}

// TestOrbitMarkdownFormatters_AnonymousFence verifies that [fencedBlock] produces
// fenced code blocks without a language marker when requested.
func TestOrbitMarkdownFormatters_AnonymousFence(t *testing.T) {
	got := fencedBlock("", "plain text")
	if got != "```\nplain text\n```\n" {
		t.Fatalf("fencedBlock() = %q, want anonymous fence", got)
	}
}

// TestOrbitMarkdownFormatters_EscapeTableCells verifies that markdown table
// cells escape pipes and normalize newlines from Orbit data.
//
// The test runs table-driven subtests for schema, tools, and graph status tables and asserts that pipes are escaped and newlines normalized.
func TestOrbitMarkdownFormatters_EscapeTableCells(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "schema",
			md: FormatSchemaMarkdown(SchemaOutput{Domains: []SchemaDomain{{
				Name:        "core|domain",
				Description: "Core\nentities",
				NodeNames:   []string{"User|Account"},
			}}}),
			want: "core&#124;domain | Core entities | User&#124;Account",
		},
		{
			name: "tools",
			md:   FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query`|graph", Description: "Run\nqueries"}}}),
			want: "`query&#124;graph` | Run queries",
		},
		{
			name: "graph status",
			md:   FormatGraphStatusMarkdown(GraphStatusOutput{Domains: []GraphStatusDomain{{Name: "SDLC|core", Items: []GraphStatusDomainItem{{Name: "Issue|Bug", Count: 4}}}}}),
			want: "SDLC&#124;core | Issue&#124;Bug: 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.md, tt.want) {
				t.Fatalf("markdown = %q, want escaped substring %q", tt.md, tt.want)
			}
		})
	}
}

// TestOrbitMarkdownAndDynamicJSON_FallbackBranches verifies empty-state
// markdown and converter fallback behavior for nil or invalid Orbit payloads.
//
// The test asserts that all formatters and converters handle nil and invalid input gracefully, returning empty-state text or zero values.
func TestOrbitMarkdownAndDynamicJSON_FallbackBranches(t *testing.T) {
	if md := FormatStatusMarkdown(StatusOutput{}); !strings.Contains(md, "No Orbit status data") {
		t.Fatalf("FormatStatusMarkdown() = %q, want empty-state text", md)
	}
	if md := FormatToolsMarkdown(ToolsOutput{}); !strings.Contains(md, "No Orbit tools") {
		t.Fatalf("FormatToolsMarkdown() = %q, want empty-state text", md)
	}
	if md := FormatDSLMarkdown(DSLOutput{}); !strings.Contains(md, "No Orbit DSL") {
		t.Fatalf("FormatDSLMarkdown() = %q, want empty-state text", md)
	}
	if md := FormatQueryMarkdown(QueryOutput{QueryType: "traversal", RawQueryStrings: []string{"MATCH (n) RETURN n"}}); !strings.Contains(md, "Raw Query Strings") || !strings.Contains(md, "MATCH (n)") {
		t.Fatalf("FormatQueryMarkdown() = %q, want raw query strings", md)
	}
	if md := FormatQueryMarkdown(QueryOutput{QueryType: "traversal"}); strings.Contains(md, "### Result") {
		t.Fatalf("FormatQueryMarkdown() = %q, did not expect result section", md)
	}
	if prettyAny(func() {}) == "" {
		t.Fatal("prettyAny() returned empty fallback")
	}
	if got := decodeRaw(json.RawMessage(`{`)); got != "{" {
		t.Fatalf("decodeRaw() = %v, want raw fallback", got)
	}
	if decodeRaw(nil) != nil {
		t.Fatal("decodeRaw(nil) = non-nil, want nil")
	}
	if out := convertStatus(nil); out.Status != "" {
		t.Fatalf("convertStatus(nil) = %+v, want zero value", out)
	}
	if out := convertSchema(nil); out.SchemaVersion != "" {
		t.Fatalf("convertSchema(nil) = %+v, want zero value", out)
	}
	if out := convertTools(nil); len(out.Tools) != 0 {
		t.Fatalf("convertTools(nil) = %+v, want zero tools", out)
	}
	if out := convertQuery(nil); out.QueryType != "" {
		t.Fatalf("convertQuery(nil) = %+v, want zero value", out)
	}
	if out := convertGraphStatus(nil); out.Projects != nil {
		t.Fatalf("convertGraphStatus(nil) = %+v, want zero value", out)
	}
}

// TestGraphStatusOptions_SetsEachSupportedScope verifies that [graphStatusOptions]
// maps namespace, project, and full-path scopes into GitLab client options.
//
// The test runs subtests for each supported scope and asserts that the options struct is set as expected.
func TestGraphStatusOptions_SetsEachSupportedScope(t *testing.T) {
	tests := []struct {
		name  string
		input GraphStatusInput
		check func(*testing.T, *gl.GetGraphStatusOptions)
	}{
		{
			name:  "namespace",
			input: GraphStatusInput{NamespaceID: 123},
			check: func(t *testing.T, opts *gl.GetGraphStatusOptions) {
				t.Helper()
				if opts.NamespaceID == nil || *opts.NamespaceID != 123 {
					t.Fatalf("NamespaceID = %v, want 123", opts.NamespaceID)
				}
			},
		},
		{
			name:  "project",
			input: GraphStatusInput{ProjectID: 456},
			check: func(t *testing.T, opts *gl.GetGraphStatusOptions) {
				t.Helper()
				if opts.ProjectID == nil || *opts.ProjectID != 456 {
					t.Fatalf("ProjectID = %v, want 456", opts.ProjectID)
				}
			},
		},
		{
			name:  "full path with llm format",
			input: GraphStatusInput{FullPath: " gitlab-org/gitlab ", ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"}},
			check: func(t *testing.T, opts *gl.GetGraphStatusOptions) {
				t.Helper()
				if opts.FullPath == nil || *opts.FullPath != "gitlab-org/gitlab" {
					t.Fatalf("FullPath = %v, want trimmed path", opts.FullPath)
				}
				if opts.ResponseFormat == nil || string(*opts.ResponseFormat) != "llm" {
					t.Fatalf("ResponseFormat = %v, want llm", opts.ResponseFormat)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := graphStatusOptions(tt.input)
			if err != nil {
				t.Fatalf("graphStatusOptions() error: %v", err)
			}
			tt.check(t, opts)
		})
	}
}

// TestGraphStatusOptions_InvalidProjectAndFormat verifies that [graphStatusOptions]
// rejects invalid project IDs and unsupported response formats.
//
// The test runs subtests for negative project IDs and invalid response formats and asserts that an error is returned.
func TestGraphStatusOptions_InvalidProjectAndFormat(t *testing.T) {
	tests := []struct {
		name  string
		input GraphStatusInput
	}{
		{name: "negative project ID", input: GraphStatusInput{ProjectID: -1}},
		{name: "invalid response format", input: GraphStatusInput{FullPath: "gitlab-org/gitlab", ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := graphStatusOptions(tt.input); err == nil {
				t.Fatalf("graphStatusOptions(%+v) error = nil, want validation error", tt.input)
			}
		})
	}
}

// TestOrbitConverters_SkipNilNestedEntriesAndPreserveOptionalFields verifies that
// Orbit response converters skip nil slices while preserving optional metadata.
//
// The test asserts that nil entries are skipped in all nested slices and that optional fields are preserved in the output.
func TestOrbitConverters_SkipNilNestedEntriesAndPreserveOptionalFields(t *testing.T) {
	status := convertStatus(&gl.OrbitStatus{Components: []*gl.OrbitStatusComponent{nil, {Name: "api", Status: "healthy"}}})
	if len(status.Components) != 1 || status.Components[0].Name != "api" || status.Components[0].Replicas != nil {
		t.Fatalf("convertStatus() = %+v, want one component without replicas", status.Components)
	}

	schema := convertSchema(&gl.OrbitSchema{
		Domains: []*gl.OrbitSchemaDomain{nil, {Name: "core", NodeNames: []string{"Project"}}},
		Nodes:   []json.RawMessage{nil, json.RawMessage(`{"name":"Project"}`)},
		Edges: []*gl.OrbitSchemaEdge{nil, {
			Name:     "AUTHORED",
			Variants: []*gl.OrbitSchemaEdgeVariant{nil, {SourceType: "User", TargetType: "Issue"}},
		}},
	})
	if len(schema.Domains) != 1 || len(schema.Nodes) != 1 || len(schema.Edges) != 1 || len(schema.Edges[0].Variants) != 1 {
		t.Fatalf("convertSchema() = %+v, want nil entries skipped", schema)
	}

	tools := convertTools(&gl.OrbitTools{Tools: []*gl.OrbitTool{nil, {Name: "query_graph", Parameters: json.RawMessage(`{"type":"object"}`)}}})
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "query_graph" {
		t.Fatalf("convertTools() = %+v, want one tool", tools.Tools)
	}

	started := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	completed := started.Add(2 * time.Minute)
	duration := int64(120000)
	lastErr := "index timeout"
	graphStatus := convertGraphStatus(&gl.OrbitGraphStatus{
		Domains: []*gl.OrbitGraphStatusDomain{nil, {Name: "SDLC", Items: []*gl.OrbitGraphStatusDomainItem{nil, {Name: "Issue", Count: 7}}}},
		Indexing: &gl.OrbitGraphStatusIndexing{
			State:           "error",
			LastStartedAt:   &started,
			LastCompletedAt: &completed,
			LastDurationMs:  &duration,
			LastError:       &lastErr,
		},
	})
	if len(graphStatus.Domains) != 1 || len(graphStatus.Domains[0].Items) != 1 {
		t.Fatalf("convertGraphStatus() domains = %+v, want nil entries skipped", graphStatus.Domains)
	}
	if graphStatus.Indexing.LastStartedAt == "" || graphStatus.Indexing.LastCompletedAt == "" || graphStatus.Indexing.LastError != lastErr {
		t.Fatalf("convertGraphStatus() indexing = %+v, want optional fields", graphStatus.Indexing)
	}
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
