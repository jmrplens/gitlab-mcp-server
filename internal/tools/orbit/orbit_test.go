package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestStatus_FlatShape_MirrorsEveryField verifies that [Status] carries the
// flat status response into [StatusOutput] field for field: the cluster
// labels, each subsystem's name and status, its replica counts in the right
// order, and its metrics decoded from raw JSON, with a stateless subsystem
// left without replicas.
//
// No two values in the fixture agree, because a fixture answering 3/3 could
// not tell ready from desired and a swap of the two used to pass.
func TestStatus_FlatShape_MirrorsEveryField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.RespondJSON(w, http.StatusOK, `{
			"status": "healthy",
			"timestamp": "2026-04-28T12:00:00Z",
			"version": "0.5.0",
			"components": [
				{"name": "clickhouse", "status": "degraded", "replicas": {"ready": 2, "desired": 3}, "metrics": {"kind": "Deployment"}},
				{"name": "api", "status": "healthy"}
			]
		}`)
	}))

	out, err := Status(context.Background(), client, StatusInput{})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	want := StatusOutput{
		Status:    "healthy",
		Timestamp: "2026-04-28T12:00:00Z",
		Version:   "0.5.0",
		Components: []StatusComponent{
			{Name: "clickhouse", Status: "degraded", Replicas: &StatusReplicas{Ready: 2, Desired: 3}, Metrics: map[string]any{"kind": "Deployment"}},
			{Name: "api", Status: "healthy"},
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Status() = %+v, want %+v", out, want)
	}
}

// TestStatus_NestedShape_PromotesSystemAndRendersItsError verifies the shape
// GitLab.com sends today, end to end: the SDK promotes the nested system object
// into the flat fields, so [StatusOutput] carries the cluster health twice, and
// the card reads it once, taking the flat copy and adding the one row only the
// nested object has, the backend error.
//
// The formatter tests build a nested output whose flat fields are empty, which
// the SDK never produces, so this is the only place the "flat wins, System fills
// the rest" merge runs against a real response.
func TestStatus_NestedShape_PromotesSystemAndRendersItsError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.RespondJSON(w, http.StatusOK, `{
			"user": {"available": true},
			"system": {
				"status": "unknown",
				"timestamp": "2026-03-20T15:45:00Z",
				"version": "0.5.0",
				"error": "cannot reach the gRPC cluster",
				"components": [{"name": "clickhouse", "status": "unknown"}]
			}
		}`)
	}))

	out, err := Status(context.Background(), client, StatusInput{})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	wantSystem := &StatusSystem{
		Status:     "unknown",
		Timestamp:  "2026-03-20T15:45:00Z",
		Version:    "0.5.0",
		Error:      "cannot reach the gRPC cluster",
		Components: []StatusComponent{{Name: "clickhouse", Status: "unknown"}},
	}
	want := StatusOutput{
		User:       &StatusUser{Available: true},
		System:     wantSystem,
		Status:     wantSystem.Status,
		Timestamp:  wantSystem.Timestamp,
		Version:    wantSystem.Version,
		Components: wantSystem.Components,
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Status() = %+v, want promoted nested shape %+v", out, want)
	}

	wantCard := "## Orbit Status\n\n" +
		"- **Available to you**: ✅\n" +
		"- **Status**: unknown\n" +
		"- **Version**: 0.5.0\n" +
		"- **Timestamp**: 20 Mar 2026 15:45 UTC\n" +
		"- **Error**: cannot reach the gRPC cluster\n\n" +
		"### Components\n\n" +
		"| Component | Status | Replicas |\n| --- | --- | --- |\n" +
		"| clickhouse | unknown |  |\n" +
		statusHints
	if got := FormatStatusMarkdown(out); got != wantCard {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, wantCard)
	}
}

// TestStatus_NestedLLMShape_RendersThePromotedTextOnce verifies the nested
// shape under response_format=llm: GitLab puts the pre-formatted body inside
// the system object, the SDK promotes it to the flat field, and the card fences
// that text once rather than twice or not at all.
func TestStatus_NestedLLMShape_RendersThePromotedTextOnce(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.AssertQueryParam(t, r, "response_format", "llm")
		testutil.RespondJSON(w, http.StatusOK, `{
			"user": {"available": true},
			"system": {"formatted_text": "status: healthy\nversion: 0.5.0"}
		}`)
	}))

	out, err := Status(context.Background(), client, StatusInput{ResponseFormat: "llm"})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if out.FormattedText != "status: healthy\nversion: 0.5.0" || out.System == nil || out.System.FormattedText != out.FormattedText {
		t.Fatalf("Status() = %+v, want the formatted text promoted from the system object", out)
	}

	want := "## Orbit Status\n\n```text\nstatus: healthy\nversion: 0.5.0\n```\n" + statusHints
	if got := FormatStatusMarkdown(out); got != want {
		t.Errorf("FormatStatusMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestStatus_ResponseFormats_ForwardTheNormalizedValue verifies that the
// explicit JSON alias reaches GitLab as "json", and that a value with case or
// whitespace around it is forwarded trimmed and lowercased rather than refused
// or sent verbatim.
func TestStatus_ResponseFormats_ForwardTheNormalizedValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "json alias", input: "json", want: "json"},
		{name: "mixed case with whitespace", input: " Raw ", want: "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
				testutil.AssertQueryParam(t, r, "response_format", tt.want)
				testutil.RespondJSON(w, http.StatusOK, `{"status":"healthy"}`)
			}))

			out, err := Status(context.Background(), client, StatusInput{ResponseFormat: tt.input})
			if err != nil {
				t.Fatalf("Status() error: %v", err)
			}
			if out.Status != "healthy" {
				t.Fatalf("Status() = %+v, want healthy", out)
			}
		})
	}
}

// TestSchema_WithExpandAndFormat_ForwardsQuery verifies that [Schema] forwards
// expand as one comma-joined parameter and format as given, and mirrors the
// ontology into [SchemaOutput] field for field: each domain's name, description
// and node names, each edge's name, description and source/target variants,
// and each node decoded from raw JSON into a value rather than dropped.
//
// The decoded node is asserted as a value, not as a count: skipping the decoded
// entries and keeping the nil one used to produce the same length.
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
	want := SchemaOutput{
		SchemaVersion: "1.0",
		Domains:       []SchemaDomain{{Name: "core", Description: "Core entities", NodeNames: []string{"User", "Project"}}},
		Nodes:         []any{map[string]any{"name": "User"}},
		Edges: []SchemaEdge{{
			Name:        "AUTHORED",
			Description: "Authorship",
			Variants:    []SchemaEdgeVariant{{SourceType: "User", TargetType: "Issue"}},
		}},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Schema() = %+v, want %+v", out, want)
	}
}

// TestSchema_FormatAndAliasAgreeing_ForwardsOnce verifies that setting both
// format and its response_format alias to the same value, whatever the case,
// is accepted and forwarded as one lowercase format parameter, since the
// mismatch refusal is for values that disagree and not for the alias being
// present at all.
func TestSchema_FormatAndAliasAgreeing_ForwardsOnce(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		testutil.AssertQueryParam(t, r, "format", "raw")
		if got := r.URL.Query().Get("response_format"); got != "" {
			t.Errorf("response_format query parameter = %q, want empty", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0"}`)
	}))

	out, err := Schema(context.Background(), client, SchemaInput{Format: "raw", ResponseFormat: "RAW"})
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	if out.SchemaVersion != "1.0" {
		t.Fatalf("Schema() = %+v, want schema version 1.0", out)
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
			t.Errorf("response_format query parameter = %q, want empty", got)
			http.Error(w, "response_format query parameter, want empty", http.StatusInternalServerError)
			return
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

// TestTools_Success_ExpectedOutput verifies that [Tools] mirrors the Orbit
// tool manifest into [ToolsOutput]: each tool's name and description, and its
// parameter schema decoded from raw JSON into a value the caller can read,
// which is the part of the manifest a model builds its query from.
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
	want := ToolsOutput{Tools: []ToolDefinition{{
		Name:        "query_graph",
		Description: "Execute graph queries",
		Parameters:  map[string]any{"type": "object"},
	}}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Tools() = %+v, want %+v", out, want)
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

	out, err := DSL(context.Background(), client, DSLInput{ResponseFormat: "llm"})
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
			t.Errorf("decode request: %v", err)
			http.Error(w, "decode request", http.StatusInternalServerError)
			return
		}
		if string(got["query"]) != `{"node":{"entity":"Project","id":"p","node_ids":[1]},"query_type":"traversal"}` {
			t.Errorf("query body = %s, want traversal query with node_ids", got["query"])
			http.Error(w, "query body, want traversal query with node_ids", http.StatusInternalServerError)
			return
		}
		if string(got["response_format"]) != `"raw"` {
			t.Errorf("response_format = %s, want raw", got["response_format"])
			http.Error(w, "response_format, want raw", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"result": [{"_id": "1", "_type": "Project"}],
			"query_type": "traversal",
			"raw_query_strings": ["SELECT ..."],
			"row_count": 1
		}`)
	}))

	out, err := Query(context.Background(), client, QueryInput{
		Query: map[string]any{
			"query_type": "traversal",
			"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		},
		ResponseFormat: "raw",
	})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	want := QueryOutput{
		Result:          []any{map[string]any{"_id": "1", "_type": "Project"}},
		QueryType:       "traversal",
		RawQueryStrings: []string{"SELECT ..."},
		RowCount:        1,
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Query() = %+v, want %+v", out, want)
	}
}

// TestQuery_NoResponseFormat_DefaultsToRaw verifies the choice the [Query]
// godoc makes: a caller who names no response format is sent to GitLab as
// "raw" rather than left to the server's own "llm" default, and the answer is
// decoded as the structured envelope. Nothing asserted this before, and the
// default answering "llm" left the suite green.
func TestQuery_NoResponseFormat_DefaultsToRaw(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "decode request", http.StatusInternalServerError)
			return
		}
		if string(got["response_format"]) != `"raw"` {
			t.Errorf("response_format = %s, want raw when the caller set none", got["response_format"])
			http.Error(w, "response_format, want raw", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"result":[{"_id":"1"}],"query_type":"traversal","row_count":1}`)
	}))

	out, err := Query(context.Background(), client, QueryInput{Query: map[string]any{
		"query_type": "traversal",
		"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
	}})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	want := QueryOutput{Result: []any{map[string]any{"_id": "1"}}, QueryType: "traversal", RowCount: 1}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Query() = %+v, want the decoded envelope %+v", out, want)
	}
}

// TestQuery_EveryQueryType_ReachesTheWire verifies that a well-formed query of
// each of the other three types passes the client-side validator and is
// forwarded to GitLab as the caller wrote it: an aggregation with a scoped
// node, a neighbors expansion referencing its node by id, and a path between
// two nodes. The rejection tests prove the validator refuses; this proves it
// does not refuse what GitLab accepts.
func TestQuery_EveryQueryType_ReachesTheWire(t *testing.T) {
	tests := []struct {
		name  string
		query map[string]any
	}{
		{
			name: "aggregation",
			query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{"id": "p", "entity": "Project", "filters": map[string]any{"full_path": map[string]any{"op": "starts_with", "value": "plens1/"}}},
					map[string]any{"id": "mr", "entity": "MergeRequest", "columns": []any{"id"}},
				},
				"relationships": []any{map[string]any{"type": "IN_PROJECT", "from": "mr", "to": "p"}},
				"aggregations":  []any{map[string]any{"function": "count", "target": "mr", "alias": "mr_count"}},
			},
		},
		{
			name: "neighbors",
			query: map[string]any{
				"query_type": "neighbors",
				"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}},
				"neighbors":  map[string]any{"node": "p", "direction": "both"},
			},
		},
		{
			name: "path_finding",
			query: map[string]any{
				"query_type": "path_finding",
				"nodes": []any{
					map[string]any{"id": "u", "entity": "User", "node_ids": []any{7}},
					map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}},
				},
				"path": map[string]any{"type": "shortest", "from": "u", "to": "p", "max_depth": 3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantQuery, marshalErr := json.Marshal(tt.query)
			if marshalErr != nil {
				t.Fatalf("marshal query: %v", marshalErr)
			}
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
				var got map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusInternalServerError)
					return
				}
				if string(got["query"]) != string(wantQuery) {
					t.Errorf("query body = %s, want %s", got["query"], wantQuery)
					http.Error(w, "query body differs from the caller's", http.StatusInternalServerError)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"`+tt.name+`","row_count":0}`)
			}))

			out, err := Query(context.Background(), client, QueryInput{Query: tt.query})
			if err != nil {
				t.Fatalf("Query() error: %v", err)
			}
			if out.QueryType != tt.name {
				t.Fatalf("Query() query type = %q, want %q", out.QueryType, tt.name)
			}
		})
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
			t.Errorf("decode request: %v", err)
			http.Error(w, "decode request", http.StatusInternalServerError)
			return
		}
		if string(got["response_format"]) != `"llm"` {
			t.Errorf("response_format = %s, want llm", got["response_format"])
			http.Error(w, "response_format, want llm", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("@header\nProject(name: gitlab)\n"))
	}))

	out, err := Query(context.Background(), client, QueryInput{
		Query: map[string]any{
			"query_type": "traversal",
			"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		},
		ResponseFormat: "llm",
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
		Query: map[string]any{
			"query_type": "traversal",
			"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		},
		ResponseFormat: "llm",
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

// TestResponseFormatName_NilReturnsEmpty verifies that responseFormatName returns
// an empty string for a nil format pointer, signaling "use the API server-side
// default" (which differs per endpoint: "json" for status/schema/tools,
// "raw" for dsl/query). The corresponding [responseFormat] helper also
// returns (nil, nil) for empty input so the SDK URL builder omits the
// response_format parameter.
func TestResponseFormatName_NilReturnsEmpty(t *testing.T) {
	if got := responseFormatName(nil); got != "" {
		t.Fatalf("responseFormatName(nil) = %q, want empty", got)
	}
	empty := gl.OrbitResponseFormatValue("")
	if got := responseFormatName(&empty); got != "" {
		t.Fatalf("responseFormatName(&\"\") = %q, want empty", got)
	}
	llm := gl.OrbitResponseFormatLLM
	if got := responseFormatName(&llm); got != "llm" {
		t.Fatalf("responseFormatName(&llm) = %q, want llm", got)
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
// project path and mirrors the answer into [GraphStatusOutput] field for field:
// the project counts, each domain's per-type counts, and the indexing state
// with the duration GitLab sent.
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
	want := GraphStatusOutput{
		Projects: &GraphStatusProjects{Indexed: 3, TotalKnown: 4},
		Domains:  []GraphStatusDomain{{Name: "SDLC", Items: []GraphStatusDomainItem{{Name: "MergeRequest", Count: 42}}}},
		Indexing: &GraphStatusIndexing{State: "indexed", LastDurationMs: 99},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("GraphStatus() = %+v, want %+v", out, want)
	}
}

// TestGraphStatus_EachScope_SendsOnlyThatParameter verifies that the one scope
// the caller chose is the only scope parameter on the wire. GitLab refuses a
// request naming more than one, and the SDK encodes a pointer to zero as
// `namespace_id=0`, so a guard that let an unset id through would turn every
// full-path lookup into a refused request.
func TestGraphStatus_EachScope_SendsOnlyThatParameter(t *testing.T) {
	tests := []struct {
		name  string
		input GraphStatusInput
		want  url.Values
	}{
		{name: "namespace", input: GraphStatusInput{NamespaceID: 123}, want: url.Values{"namespace_id": {"123"}}},
		{name: "project", input: GraphStatusInput{ProjectID: 456}, want: url.Values{"project_id": {"456"}}},
		{name: "full path", input: GraphStatusInput{FullPath: "gitlab-org/gitlab"}, want: url.Values{"full_path": {"gitlab-org/gitlab"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/graph_status")
				if got := r.URL.Query(); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("query parameters = %v, want exactly %v", got, tt.want)
					http.Error(w, "query parameters, want exactly the chosen scope", http.StatusInternalServerError)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, `{"projects":{"indexed":1,"total_known":1}}`)
			}))

			out, err := GraphStatus(context.Background(), client, tt.input)
			if err != nil {
				t.Fatalf("GraphStatus() error: %v", err)
			}
			if out.Projects == nil || out.Projects.Indexed != 1 {
				t.Fatalf("GraphStatus() = %+v, want one indexed project", out)
			}
		})
	}
}

// TestGraphStatus_WithoutIndexing_LeavesIndexingNil verifies that a status
// answer carrying no indexing object and no domains is mirrored as such: the
// Indexing pointer stays nil rather than pointing at an empty state, and the
// domains are an empty list, so the card writes neither section.
func TestGraphStatus_WithoutIndexing_LeavesIndexingNil(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/graph_status")
		testutil.RespondJSON(w, http.StatusOK, `{"projects":{"indexed":1,"total_known":2}}`)
	}))

	out, err := GraphStatus(context.Background(), client, GraphStatusInput{ProjectID: 9})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	want := GraphStatusOutput{Projects: &GraphStatusProjects{Indexed: 1, TotalKnown: 2}, Domains: []GraphStatusDomain{}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("GraphStatus() = %+v, want %+v", out, want)
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
				_, err := Status(context.Background(), client, StatusInput{ResponseFormat: "xml"})
				return err
			},
			want: "use raw, llm, or json",
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
				// A func value cannot be JSON-encoded. The query passes
				// structural validation (it has query_type and node_ids) but
				// fails the final json.Marshal step. This protects callers
				// from passing unserializable values in the query map.
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{
					"query_type": "traversal",
					"node": map[string]any{
						"id":       "p",
						"entity":   "Project",
						"node_ids": []int{1},
						"bad":      func() {},
					},
				}})
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
			want: "use raw, llm, or json",
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
					Query: map[string]any{
						"query_type": "traversal",
						"node": map[string]any{
							"id":       "p",
							"entity":   "Project",
							"node_ids": []int{1},
						},
					},
					ResponseFormat: "xml",
				})
				return err
			},
			want: "use raw, llm, or json",
		},
		{
			name: "invalid dsl response format",
			call: func() error {
				_, err := DSL(context.Background(), client, DSLInput{ResponseFormat: "xml"})
				return err
			},
			want: "use raw, llm, or json",
		},
		{
			name: "missing query_type",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{}})
				return err
			},
			want: "query_type is required",
		},
		{
			name: "unknown query_type",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{"query_type": "search"}})
				return err
			},
			want: "must be one of: traversal, aggregation, neighbors, path_finding",
		},
		{
			name: "traversal without node_ids or filters",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{
					"query_type": "traversal",
					"node":       map[string]any{"id": "p", "entity": "Project", "columns": []string{"id"}},
				}})
				return err
			},
			want: "require at least one node",
		},
		{
			name: "aggregation without node_ids or filters",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{
					"query_type":   "aggregation",
					"nodes":        []any{map[string]any{"id": "mr", "entity": "MergeRequest", "columns": []any{"id"}}},
					"aggregations": []any{map[string]any{"function": "count", "target": "mr", "alias": "mr_count"}},
				}})
				return err
			},
			want: "aggregation queries require at least one node",
		},
		{
			name: "traversal with id_range is also scoped",
			call: func() error {
				// id_range counts as a valid scope per the Orbit query
				// language reference. This subtest uses its own client that
				// returns 500 on every call so the test can confirm the
				// request reached the wire (and thus passed client-side
				// validation) without triggering the shared fixture's
				// "handler should not be called" assertion.
				allowClient := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				_, err := Query(context.Background(), allowClient, QueryInput{Query: map[string]any{
					"query_type": "traversal",
					"node": map[string]any{
						"id":       "p",
						"entity":   "Project",
						"id_range": map[string]any{"start": 1, "end": 100},
					},
				}})
				return err
			},
			want: "internal server error", // httptest handler returns 500
		},
		{
			name: "path_finding with fewer than two nodes",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{
					"query_type": "path_finding",
					"nodes":      []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}}},
					"path":       map[string]any{"type": "shortest", "from": "p", "to": "p", "max_depth": 1},
				}})
				return err
			},
			want: "at least two top-level node",
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
				_, err := Query(ctx, client, QueryInput{Query: map[string]any{
					"query_type": "traversal",
					"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
				}})
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
				_, err := Query(ctx, client, QueryInput{Query: map[string]any{
					"query_type": "traversal",
					"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
				}})
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
			_, err := Query(ctx, client, QueryInput{Query: map[string]any{
				"query_type": "traversal",
				"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
			}})
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

// TestConvertStatus_NestedShape_MirrorsUserAndSystem verifies that the
// nested Orbit status response shape (user/system keys) is mirrored
// field-for-field into [StatusOutput].
//
// The test asserts that User.Available is carried through and that the
// System object mirrors status, version, timestamp, error, and the
// per-subsystem components (with nil component entries dropped).
func TestConvertStatus_NestedShape_MirrorsUserAndSystem(t *testing.T) {
	nested := convertStatus(&gl.OrbitStatus{
		User: &gl.OrbitStatusUser{Available: true},
		System: &gl.OrbitStatusSystem{
			FormattedText: "system text",
			Status:        "ok",
			Version:       "1.2.3",
			Timestamp:     "2026-01-01T00:00:00Z",
			Error:         "grpc unreachable",
			Components:    []*gl.OrbitStatusComponent{nil, {Name: "api", Status: "healthy"}},
		},
	})
	if nested.User == nil || !nested.User.Available {
		t.Fatalf("convertStatus() user = %+v, want available", nested.User)
	}
	if nested.System == nil {
		t.Fatal("convertStatus() system = nil, want mirrored object")
	}
	if nested.System.FormattedText != "system text" || nested.System.Status != "ok" ||
		nested.System.Version != "1.2.3" || nested.System.Timestamp != "2026-01-01T00:00:00Z" ||
		nested.System.Error != "grpc unreachable" {
		t.Fatalf("convertStatus() system = %+v, want mirrored scalar fields", nested.System)
	}
	if len(nested.System.Components) != 1 || nested.System.Components[0].Name != "api" {
		t.Fatalf("convertStatus() system components = %+v, want single api entry", nested.System.Components)
	}
}

// TestOrbitConverters_SkipNilNestedEntriesAndPreserveOptionalFields verifies that
// Orbit response converters skip nil entries in every nested slice while
// preserving the optional indexing fields, each timestamp normalized to UTC
// RFC3339 whatever zone the SDK decoded it in, and the start and completion
// kept apart.
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

	started := time.Date(2026, 5, 6, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
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
	wantDomains := []GraphStatusDomain{{Name: "SDLC", Items: []GraphStatusDomainItem{{Name: "Issue", Count: 7}}}}
	if !reflect.DeepEqual(graphStatus.Domains, wantDomains) {
		t.Fatalf("convertGraphStatus() domains = %+v, want nil entries skipped: %+v", graphStatus.Domains, wantDomains)
	}
	wantIndexing := &GraphStatusIndexing{
		State:           "error",
		LastStartedAt:   "2026-05-06T10:00:00Z",
		LastCompletedAt: "2026-05-06T10:02:00Z",
		LastDurationMs:  120000,
		LastError:       "index timeout",
	}
	if !reflect.DeepEqual(graphStatus.Indexing, wantIndexing) {
		t.Fatalf("convertGraphStatus() indexing = %+v, want %+v", graphStatus.Indexing, wantIndexing)
	}
}

// TestStatus_WithResponseFormat_ForwardsValue verifies that [Status]
// forwards a non-empty `response_format` as the GitLab client option
// when the caller passes an explicit value. The existing Status tests
// only exercise the empty-format path; this test covers the
// `if hasFormat` branch in [Status] that sets
// `opts.ResponseFormat = format`.
func TestStatus_WithResponseFormat_ForwardsValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.AssertQueryParam(t, r, "response_format", "llm")
		testutil.RespondJSON(w, http.StatusOK, `{"status":"healthy","version":"0.5.0"}`)
	}))

	out, err := Status(context.Background(), client, StatusInput{
		ResponseFormat: "llm",
	})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if out.Status != "healthy" || out.Version != "0.5.0" {
		t.Fatalf("Status() = %+v, want healthy 0.5.0", out)
	}
}

// TestSchema_StatusError_WrapsOrbitHint verifies that [Schema] routes
// HTTP errors through [wrapOrbitErr] so callers see the same actionable
// hints as the other Orbit handlers. This test covers the
// `if err != nil` branch in [Schema] that returns the wrapped error.
func TestSchema_StatusError_WrapsOrbitHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"Not Found"}`)
	}))
	_, err := Schema(context.Background(), client, SchemaInput{})
	if err == nil {
		t.Fatal("Schema() error = nil, want Not Found error")
	}
	if !strings.Contains(err.Error(), "knowledge_graph") {
		t.Fatalf("Schema() error = %q, want knowledge_graph hint", err)
	}
}

// TestSchema_ConflictingFormatAndResponseFormat_DetectsMismatch
// verifies that [Schema] surfaces the "must match" error when the
// caller sets both `format` and `response_format` to different values
// (case-insensitive). This covers the `!strings.EqualFold(format,
// responseFormatAlias)` branch in [schemaResponseFormat].
func TestSchema_ConflictingFormatAndResponseFormat_DetectsMismatch(t *testing.T) {
	_, err := Schema(context.Background(), nil, SchemaInput{Format: "raw", ResponseFormat: "LLM"})
	if err == nil {
		t.Fatal("Schema() error = nil, want format/response_format mismatch error")
	}
	if !strings.Contains(err.Error(), "must match") {
		t.Fatalf("Schema() error = %q, want must-match message", err)
	}
}

// TestSchema_ResponseFormatAliasEmptyString verifies that an empty
// `response_format` alias is treated the same as an empty `format`:
// the SDK options carry no format so the API applies its server-side
// default.
func TestSchema_ResponseFormatAliasEmptyString(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		if got := r.URL.Query().Get("format"); got != "" {
			t.Errorf("format query parameter = %q, want empty (server default)", got)
			http.Error(w, "format query parameter, want empty (server default)", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0"}`)
	}))

	if _, err := Schema(context.Background(), client, SchemaInput{ResponseFormat: "   "}); err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
}

// TestGraphStatusOptions_EmptyFullPathIsTreatedAsNoScope verifies that
// a whitespace-only `full_path` is normalized to empty and therefore
// does not count as a scope. With no other scope set,
// [graphStatusOptions] must surface the "set exactly one" error
// rather than silently sending an empty full_path query parameter.
func TestGraphStatusOptions_EmptyFullPathIsTreatedAsNoScope(t *testing.T) {
	if _, err := graphStatusOptions(GraphStatusInput{FullPath: "   "}); err == nil {
		t.Fatal("graphStatusOptions() error = nil, want set-exactly-one error")
	}
}
