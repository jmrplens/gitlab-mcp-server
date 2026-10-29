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

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestOrbitHandlers_CancelledOnArrival_ReturnCanceled verifies that every
// Orbit handler, on each source it reads, hands client-go the caller's
// context: the context ends when the request reaches the mock, so each
// returns context.Canceled promptly only if its request carried it.
func TestOrbitHandlers_CancelledOnArrival_ReturnCanceled(t *testing.T) {
	respond := func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}
	cases := []struct {
		name string
		call func(context.Context, *gitlabclient.Client) error
	}{
		{name: "status", call: callStatus},
		{name: "schema", call: callSchema},
		{name: "schema in the llm format", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Schema(ctx, client, SchemaInput{Format: "llm"})
			return err
		}},
		{name: "tools", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Tools(ctx, client, ToolsInput{})
			return err
		}},
		{name: "dsl", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := DSL(ctx, client, DSLInput{})
			return err
		}},
		{name: "query", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
			return err
		}},
		{name: "query in the llm format", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL, ResponseFormat: "llm"})
			return err
		}},
		{name: "query in the gql format", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL, ResponseFormat: "gql"})
			return err
		}},
		{name: "graph status", call: callGraphStatus},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, client := testutil.CancelOnArrival(t, respond)
			if err := tc.call(ctx, client); !errors.Is(err, context.Canceled) {
				t.Fatalf("%s error = %v after the context was cancelled mid-flight, want context.Canceled", tc.name, err)
			}
		})
	}
}

// TestQuery_GQLText_IsSentAsTheBodysQueryString verifies the body [Query]
// posts for a query written in GQL, the language GitLab.com reads every query
// in since 2026-10-15: {"query": "<GQL text>", "response_format": "raw"} and
// nothing else, the text a JSON string GitLab compiles as it was written. The
// answer is decoded as the structured envelope.
func TestQuery_GQLText_IsSentAsTheBodysQueryString(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "decode request", http.StatusInternalServerError)
			return
		}
		if want := map[string]any{"query": projectByIDGQL, "response_format": "raw"}; !reflect.DeepEqual(got, want) {
			t.Errorf("request body = %v, want %v", got, want)
			http.Error(w, "request body differs", http.StatusInternalServerError)
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
		Query:          projectByIDGQL,
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
// "raw" rather than left to the server's own default, and the answer is
// decoded as the structured envelope.
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

	out, err := Query(context.Background(), client, QueryInput{Query: projectByIDGQL})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	want := QueryOutput{Result: []any{map[string]any{"_id": "1"}}, QueryType: "traversal", RowCount: 1}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Query() = %+v, want the decoded envelope %+v", out, want)
	}
}

// projectByIDGQL is the smallest query GitLab.com runs in GQL: one Project
// node, labeled where it appears and bounded by its id.
const projectByIDGQL = "MATCH (p:Project {id: 1}) RETURN p.id, p.full_path"

// projectByIDObjectText is the same query written in version 12 of the JSON
// DSL, which GitLab.com read until 2026-10-15 and removed on 2026-10-27, as the
// JSON text a client that serializes a nested object sends, and which
// GitLab.com refused on 2026-10-17 and 2026-10-29 with "Orbit query syntax at
// line 1, column 1".
const projectByIDObjectText = `{"query_type":"traversal","nodes":[{"id":"p","entity":"Project","node_ids":[1]}]}`

// TestQuery_EveryQueryShape_InGQL_ReachesTheWire verifies that a query of each
// shape GQL writes reaches GitLab as the text the caller wrote: a filtered
// traversal, an aggregation grouped by a node and sorted by its alias, a page
// by SKIP and LIMIT, the neighbors of one node and a shortest path over named
// relationship types between two bounded ones, each answered 200 by GitLab.com
// on 2026-10-29. Nothing here parses the text, so a quote, an arrow or a brace
// inside it travels as it is.
func TestQuery_EveryQueryShape_InGQL_ReachesTheWire(t *testing.T) {
	tests := []struct {
		name      string
		queryType string
		query     string
	}{
		{name: "traversal", queryType: "traversal", query: "MATCH (p:Project) WHERE p.full_path STARTS WITH 'jmrp-oss/' RETURN p.id, p.full_path LIMIT 3"},
		{name: "page", queryType: "traversal", query: "MATCH (p:Project) WHERE p.full_path STARTS WITH 'jmrp-oss/' RETURN p.id, p.name ORDER BY p.name SKIP 1 LIMIT 1"},
		{name: "aggregation", queryType: "aggregation", query: "MATCH (j:Job)-[:IN_PROJECT]->(p:Project) WHERE p.full_path STARTS WITH 'jmrp-oss/' RETURN p, count(j) AS job_count ORDER BY job_count DESC"},
		{name: "neighbors", queryType: "neighbors", query: "MATCH (p:Project {full_path: 'jmrp-oss/orbit-fixtures/kg-fixtures'})--(n) RETURN n"},
		{name: "path_finding", queryType: "path_finding", query: "MATCH path = ANY SHORTEST (b:Branch {name: 'main'})-[:IN_PROJECT*1..2]->(p:Project {full_path: 'jmrp-oss/orbit-fixtures/kg-fixtures'}) RETURN path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var got struct {
					Query any `json:"query"`
				}
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusInternalServerError)
					return
				}
				if got.Query != tt.query {
					t.Errorf("query = %#v, want the caller's text %q", got.Query, tt.query)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"`+tt.queryType+`","row_count":0}`)
			}))

			out, err := Query(context.Background(), client, QueryInput{Query: tt.query})
			if err != nil {
				t.Fatalf("Query() error: %v", err)
			}
			if out.QueryType != tt.queryType {
				t.Fatalf("Query() query type = %q, want %q as GitLab inferred it", out.QueryType, tt.queryType)
			}
		})
	}
}

// TestQuery_LLMResponseFormat_UsesRawResponse verifies that [Query] uses the
// raw Orbit API path for LLM/text responses instead of JSON decoding, sending
// the query as it was written. The llm answer carries no query type of its
// own, and GitLab infers the type of a GQL query from the statement, so the
// output names none.
func TestQuery_LLMResponseFormat_UsesRawResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "decode request", http.StatusInternalServerError)
			return
		}
		if want := map[string]any{"query": projectByIDGQL, "response_format": "llm"}; !reflect.DeepEqual(got, want) {
			t.Errorf("request body = %v, want %v", got, want)
			http.Error(w, "request body differs", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("@header\nProject(name: gitlab)\n"))
	}))

	out, err := Query(context.Background(), client, QueryInput{
		Query:          projectByIDGQL,
		ResponseFormat: "llm",
	})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	want := QueryOutput{FormattedText: "@header\nProject(name: gitlab)\n"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Query() = %+v, want %+v", out, want)
	}
}

// TestQuery_LLMResponseFormat_RawError verifies that raw Orbit API failures are
// wrapped as errors instead of falling back to JSON decoding.
func TestQuery_LLMResponseFormat_RawError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := Query(context.Background(), client, QueryInput{
		Query:          projectByIDGQL,
		ResponseFormat: "llm",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// projectTable is GitLab.com's answer to a GQL query in the gql response
// format, as it answered MATCH (p:Project {full_path: 'plens1/kg-fixtures'})
// RETURN p.id, p.full_path LIMIT 1 on 2026-10-17: a graph pattern table as
// text/plain, written by writeLLMResultResponse in
// workhorse/internal/orbit/sendquery.go like the llm answer.
const projectTable = "+------------------------------------------------------------+\n" +
	"| p                                                          |\n" +
	"+------------------------------------------------------------+\n" +
	"| (:Project {id: 83234203, full_path: \"plens1/kg-fixtures\"}) |\n" +
	"+------------------------------------------------------------+\n\n1 row\n"

// TestQuery_GQLResponseFormat_ReturnsTheGraphPatternTable verifies the third
// response format POST /orbit/query takes, gql, which GitLab declares on that
// route alone (`values: %w[raw llm gql]` in ee/lib/api/orbit/data.rb) and which
// the query command orbit.dsl publishes in its input schema: it reaches GitLab
// as written, in any case, and the table GitLab answers with is published
// whole as formatted_text, the way the llm text is.
func TestQuery_GQLResponseFormat_ReturnsTheGraphPatternTable(t *testing.T) {
	for _, format := range []string{"gql", " GQL "} {
		t.Run(format, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
				var got map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusInternalServerError)
					return
				}
				if want := map[string]any{"query": projectByIDGQL, "response_format": "gql"}; !reflect.DeepEqual(got, want) {
					t.Errorf("request body = %v, want %v", got, want)
					http.Error(w, "request body differs", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(projectTable))
			}))

			out, err := Query(context.Background(), client, QueryInput{Query: projectByIDGQL, ResponseFormat: format})
			if err != nil {
				t.Fatalf("Query() error: %v", err)
			}
			if want := (QueryOutput{FormattedText: projectTable}); !reflect.DeepEqual(out, want) {
				t.Fatalf("Query() = %+v, want %+v", out, want)
			}
		})
	}
}

// TestOrbitReads_GQLResponseFormat_IsRefusedWhereGitLabDeclaresNone verifies
// that gql stays a format of the query route alone: the status, the query
// language, the schema and the indexing status take raw and llm, so gql is
// refused there before anything is sent.
func TestOrbitReads_GQLResponseFormat_IsRefusedWhereGitLabDeclaresNone(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for gql outside the query route: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{name: "status", want: "invalid response_format: use raw or llm", call: func() error {
			_, err := Status(context.Background(), client, StatusInput{StructuredFormatInput{ResponseFormat: "gql"}})
			return err
		}},
		{name: "dsl", want: "invalid response_format: use raw or llm", call: func() error {
			_, err := DSL(context.Background(), client, DSLInput{ResponseFormatInput{ResponseFormat: "gql"}})
			return err
		}},
		{name: "schema", want: "invalid format: use raw or llm", call: func() error {
			_, err := Schema(context.Background(), client, SchemaInput{Format: "gql"})
			return err
		}},
		{name: "graph_status", want: "invalid response_format: use raw or llm", call: func() error {
			_, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "jmrp-oss", ResponseFormat: "gql"})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil || err.Error() != tc.want {
				t.Fatalf("%s error = %v, want %q", tc.name, err, tc.want)
			}
		})
	}
}

// TestQuery_AnyShape_IsLeftForGitLabToJudge verifies that the handler holds a
// query to nothing but being one: GQL text GitLab refuses (an unbounded node,
// OR, PAGE, a syntax error) and a query written in the JSON DSL GitLab.com
// removed, sent as text, all reach GitLab exactly as the caller wrote them
// (issue 1031).
func TestQuery_AnyShape_IsLeftForGitLabToJudge(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "an unbounded GQL node", query: "MATCH (p:Project) RETURN p LIMIT 5"},
		{name: "GQL with OR", query: "MATCH (p:Project) WHERE p.id = 1 OR p.id = 2 RETURN p.id"},
		{name: "GQL with PAGE", query: "MATCH (p:Project {id: 1}) RETURN p.id PAGE 10"},
		{name: "GQL that does not parse", query: "MATCH (p:Project {id: 1}) RETURN p.id GROUP BY p.id"},
		{name: "the JSON DSL written as text", query: projectByIDObjectText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantQuery, marshalErr := json.Marshal(tt.query)
			if marshalErr != nil {
				t.Fatalf("marshal query: %v", marshalErr)
			}
			var sent atomic.Int32
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent.Add(1)
				var got map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusInternalServerError)
					return
				}
				if string(got["query"]) != string(wantQuery) {
					t.Errorf("query body = %s, want the caller's %s", got["query"], wantQuery)
				}
				testutil.RespondJSON(w, http.StatusBadRequest, `{"code":"compile_error","message":"schema violation"}`)
			}))

			_, err := Query(context.Background(), client, QueryInput{Query: tt.query})
			if err == nil {
				t.Fatal("Query() error = nil, want GitLab's refusal")
			}
			if sent.Load() != 1 {
				t.Fatalf("requests sent = %d, want the one GitLab judges the query by", sent.Load())
			}
		})
	}
}

// TestQuery_GitLabRefusal_ReachesTheCallerWithTheQueryHint verifies what a
// caller is told when GitLab refuses a GQL query: the operation, GitLab's
// code and message as Workhorse wrote them, the hint naming orbit.dsl and
// orbit.schema, and the request line, on both the structured and the llm
// path (issue 1031).
func TestQuery_GitLabRefusal_ReachesTheCallerWithTheQueryHint(t *testing.T) {
	const refusal = `schema violation: add filters or node_ids to at least one node`
	formats := []struct {
		name   string
		format string
	}{
		{name: "the default format", format: ""},
		{name: "raw", format: "raw"},
		{name: "llm", format: "llm"},
	}
	for _, tc := range formats {
		format := tc.format
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
				body, err := json.Marshal(map[string]string{"code": "compile_error", "message": refusal})
				if err != nil {
					t.Errorf("marshal refusal: %v", err)
				}
				testutil.RespondJSON(w, http.StatusBadRequest, string(body))
			}))

			_, err := Query(context.Background(), client, QueryInput{
				Query:          "MATCH (p:Project) RETURN p LIMIT 5",
				ResponseFormat: format,
			})
			if err == nil {
				t.Fatal("Query() error = nil, want GitLab's refusal")
			}
			want := "orbit_query: bad request: check your input parameters ({code: compile_error}, {message: " + refusal + "}). " +
				"Suggestion: " + queryRefusedHint + ": POST "
			if !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("Query() error = %q, want it to begin %q", err, want)
			}
			if !strings.HasSuffix(err.Error(), "/api/v4/orbit/query: 400 {code: compile_error}, {message: "+refusal+"}") {
				t.Fatalf("Query() error = %q, want the request line and GitLab's refusal at its end", err)
			}
		})
	}
}

// TestQuery_JSONObjectRefusedForItsLanguage_SaysGitLabReadsGQL verifies what
// a caller who wrote the query as a JSON object, sent as text, is told:
// GitLab.com's refusal of it for its language gets the hint that it reads GQL
// text, with an example, while the same text refused for anything else, and
// GQL text refused for its syntax, keep the hint that points at the fault
// (issue 1414).
func TestQuery_JSONObjectRefusedForItsLanguage_SaysGitLabReadsGQL(t *testing.T) {
	const (
		gqlSyntax  = `{"code":"compile_error","message":"schema violation: Orbit query syntax at line 1, column 1: expected Statement"}`
		jsonSchema = `{"code":"compile_error","message":"schema violation: \"nodes\" is a required property"}`
	)
	cases := []struct {
		name   string
		query  string
		format string
		body   string
		hint   string
	}{
		{name: "GQL text refused for its syntax", query: "MATCH (p:Project {id: 1}) RETURN", body: gqlSyntax, hint: queryRefusedHint},
		{name: "an object written as JSON text, parsed as GQL", query: projectByIDObjectText, body: gqlSyntax, hint: queryObjectRefusedHint},
		{name: "an object written as JSON text after white space, llm", query: "\n  " + projectByIDObjectText, format: "llm", body: gqlSyntax, hint: queryObjectRefusedHint},
		{name: "an object written as JSON text refused for something else", query: projectByIDObjectText, body: jsonSchema, hint: queryRefusedHint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
				testutil.RespondJSON(w, http.StatusBadRequest, tc.body)
			}))

			_, err := Query(context.Background(), client, QueryInput{Query: tc.query, ResponseFormat: tc.format})
			if err == nil || !strings.Contains(err.Error(), "Suggestion: "+tc.hint+":") {
				t.Fatalf("Query() error = %v, want the hint %q", err, tc.hint)
			}
		})
	}
}

// TestQuery_BlankQuery_IsRefusedBeforeSending verifies the one thing [Query]
// judges about a query: that there is one, as text that is not blank.
func TestQuery_BlankQuery_IsRefusedBeforeSending(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for a query that is not one: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	cases := []struct {
		name  string
		query string
	}{
		{name: "an empty string", query: ""},
		{name: "blank text", query: " \n\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Query(context.Background(), client, QueryInput{Query: tc.query})
			if err == nil || !strings.Contains(err.Error(), "query is required") {
				t.Fatalf("Query() error = %v, want %q", err, "query is required")
			}
		})
	}
}

// TestResponseFormatName_NilReturnsEmpty verifies that responseFormatName
// returns an empty string for a nil or empty format and the value otherwise.
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

// TestOrbit_ValidationErrors_ReturnActionableErrors verifies that input
// validation for every Orbit handler refuses an invalid format or a malformed
// query before anything is sent, with an actionable message.
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
		{name: "invalid status response format", want: "use raw or llm", call: func() error {
			_, err := Status(context.Background(), client, StatusInput{StructuredFormatInput{ResponseFormat: "xml"}})
			return err
		}},
		{name: "empty query", want: "query is required", call: func() error {
			_, err := Query(context.Background(), client, QueryInput{})
			return err
		}},
		{name: "invalid schema format", want: "use raw or llm", call: func() error {
			_, err := Schema(context.Background(), client, SchemaInput{Format: "xml"})
			return err
		}},
		{name: "conflicting schema formats", want: "must match", call: func() error {
			_, err := Schema(context.Background(), client, SchemaInput{Format: "raw", ResponseFormat: "llm"})
			return err
		}},
		{name: "json synonym conflicting with llm", want: "must match", call: func() error {
			_, err := Schema(context.Background(), client, SchemaInput{Format: "json", ResponseFormat: "llm"})
			return err
		}},
		{name: "invalid schema response_format beside a valid format", want: "invalid response_format: use raw or llm", call: func() error {
			_, err := Schema(context.Background(), client, SchemaInput{Format: "raw", ResponseFormat: "xml"})
			return err
		}},
		{name: "invalid schema format beside a valid response_format", want: "invalid format: use raw or llm", call: func() error {
			_, err := Schema(context.Background(), client, SchemaInput{Format: "xml", ResponseFormat: "raw"})
			return err
		}},
		{name: "invalid query response format", want: "use raw, llm or gql", call: func() error {
			_, err := Query(context.Background(), client, QueryInput{Query: projectByIDGQL, ResponseFormat: "xml"})
			return err
		}},
		{name: "invalid dsl response format", want: "use raw or llm", call: func() error {
			_, err := DSL(context.Background(), client, DSLInput{ResponseFormatInput{ResponseFormat: "xml"}})
			return err
		}},
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

// TestSchema_LLMFormatUnderEitherName_ConflictsOnlyWhenTheyDiffer verifies
// that the schema's two format inputs agree when either is empty or both name
// the same format.
func TestSchema_LLMFormatUnderEitherName_ConflictsOnlyWhenTheyDiffer(t *testing.T) {
	cases := []struct {
		name  string
		input SchemaInput
		want  string
	}{
		{name: "alias alone", input: SchemaInput{ResponseFormat: "llm"}, want: "llm"},
		{name: "both, json and raw", input: SchemaInput{Format: "json", ResponseFormat: "raw"}, want: "raw"},
		{name: "neither", input: SchemaInput{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format, err := schemaResponseFormat(tc.input)
			if err != nil {
				t.Fatalf("schemaResponseFormat() error: %v", err)
			}
			if got := responseFormatName(format); got != tc.want {
				t.Fatalf("schemaResponseFormat() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOrbit_HTTPErrorHints_ReturnExpectedGuidance verifies that HTTP failures
// from each Orbit source carry the guidance their status calls for: the
// availability hint for 404 and 403, the query hint for a refused query, the
// parameter hint for any other 400, and backoff for 429 and 503.
func TestOrbit_HTTPErrorHints_ReturnExpectedGuidance(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		call   func(context.Context, *gitlabclient.Client) error
		want   string
	}{
		{name: "status not found", path: graphQLPath, status: http.StatusNotFound, call: callStatus, want: orbitUnavailableHint},
		{name: "query forbidden", path: "/api/v4/orbit/query", status: http.StatusForbidden, call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
			return err
		}, want: orbitUnavailableHint},
		{name: "query refused", path: "/api/v4/orbit/query", status: http.StatusBadRequest, call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
			return err
		}, want: "Suggestion: " + queryRefusedHint},
		{name: "graph status bad request", path: graphQLPath, status: http.StatusBadRequest, call: callGraphStatus, want: "check the Orbit query"},
		{name: "query rate limited", path: "/api/v4/orbit/query", status: http.StatusTooManyRequests, call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
			return err
		}, want: "rate-limited"},
		{name: "graph status unavailable", path: graphQLPath, status: http.StatusServiceUnavailable, call: callGraphStatus, want: "temporarily unavailable"},
		{name: "tools teapot", path: mcpEndpoint, status: http.StatusTeapot, call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Tools(ctx, client, ToolsInput{})
			return err
		}, want: "orbit_tools"},
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

// TestOrbit_AccountRefused_GetsNoOrbitHint verifies that the availability
// hint follows GitLab's refusal of Orbit, the 403 it answered every query with
// once the fixture namespace lost Orbit on 2026-10-29, and not a 403 that
// refuses the account itself, which no Orbit access would change.
func TestOrbit_AccountRefused_GetsNoOrbitHint(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantHint bool
	}{
		{name: "Orbit not served", body: `{"message":"403 Forbidden - No Orbit enabled namespaces available"}`, wantHint: true},
		{name: "a blocked account", body: `{"message":"403 Forbidden - Your account has been blocked."}`, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
				testutil.RespondJSON(w, http.StatusForbidden, tc.body)
			}))
			_, err := Query(context.Background(), client, QueryInput{Query: projectByIDGQL})
			if err == nil {
				t.Fatal("Query() error = nil, want GitLab's refusal")
			}
			if got := strings.Contains(err.Error(), orbitUnavailableHint); got != tc.wantHint {
				t.Fatalf("Query() error = %v, carries the Orbit hint = %t, want %t", err, got, tc.wantHint)
			}
		})
	}
}

// TestWrapOrbitErr_GenericError_UsesWrappedMessage verifies that non-HTTP
// Orbit errors retain both operation context and the original error message.
func TestWrapOrbitErr_GenericError_UsesWrappedMessage(t *testing.T) {
	err := wrapOrbitErr("orbit_status", errors.New("network unavailable"))
	if err == nil || !strings.Contains(err.Error(), "orbit_status") || !strings.Contains(err.Error(), "network unavailable") {
		t.Fatalf("wrapOrbitErr() = %v, want operation context and source error", err)
	}
}

// TestOrbitHandlers_ContextCancellation_ReturnsError verifies that every
// Orbit handler returns the context's error without sending anything when the
// context ended before the call.
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
		{name: "status", call: func() error { return callStatus(ctx, client) }},
		{name: "schema", call: func() error { return callSchema(ctx, client) }},
		{name: "tools", call: func() error { _, err := Tools(ctx, client, ToolsInput{}); return err }},
		{name: "dsl", call: func() error { _, err := DSL(ctx, client, DSLInput{}); return err }},
		{name: "query", call: func() error {
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
			return err
		}},
		{name: "graph status", call: func() error { return callGraphStatus(ctx, client) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("handler error = %v, want context.Canceled", err)
			}
		})
	}
}

// TestOrbitConverters_EmptyAndUndecodable_FallBack verifies the converters'
// fallbacks: a nil query result is the zero output, and a payload that is not
// JSON is passed on as its text, or as nothing when it is empty.
func TestOrbitConverters_EmptyAndUndecodable_FallBack(t *testing.T) {
	if out := convertQuery(nil); !reflect.DeepEqual(out, QueryOutput{}) {
		t.Fatalf("convertQuery(nil) = %+v, want zero value", out)
	}
	if got := decodeRaw(json.RawMessage(`{`)); got != "{" {
		t.Fatalf("decodeRaw() = %v, want raw fallback", got)
	}
	if decodeRaw(nil) != nil {
		t.Fatal("decodeRaw(nil) = non-nil, want nil")
	}
}
