// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
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

// TestStatus_ResponseFormats_ForwardTheNormalizedValue verifies that the json
// synonym reaches GitLab as "raw", since GitLab answers 400 to "json" on every
// Orbit route, and that a value with case or whitespace around it is forwarded
// trimmed and lowercased rather than refused or sent verbatim.
func TestStatus_ResponseFormats_ForwardTheNormalizedValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "json synonym", input: "json", want: "raw"},
		{name: "json synonym in capitals", input: " JSON ", want: "raw"},
		{name: "mixed case with whitespace", input: " Raw ", want: "raw"},
		{name: "llm", input: "llm", want: "llm"},
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
// expand as one comma-joined parameter and the format as response_format, the
// name GitLab's route declares, and never as format, which Grape reserves and
// answers 406 to; and that it mirrors the ontology into [SchemaOutput] field
// for field: each domain's name, description and node names, each edge's name,
// description and source/target variants, and each node decoded from raw JSON
// into a value rather than dropped.
//
// The decoded node is asserted as a value, not as a count: skipping the decoded
// entries and keeping the nil one used to produce the same length.
func TestSchema_WithExpandAndFormat_ForwardsQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		testutil.AssertQueryParam(t, r, "expand", "User,Project")
		testutil.AssertQueryParam(t, r, "response_format", "raw")
		if r.URL.Query().Has("format") {
			t.Errorf("format query parameter = %q, want it absent: GitLab answers 406 to it", r.URL.Query().Get("format"))
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"schema_version": "1.0",
			"domains": [{"name": "core", "description": "Core entities", "node_names": ["User", "Project"]}],
			"nodes": [{"name": "User"}],
			"edges": [{"name": "AUTHORED", "description": "Authorship", "variants": [{"source_type": "User", "target_type": "Issue"}]}]
		}`)
	}))

	out, err := Schema(context.Background(), client, SchemaInput{Expand: []string{"User", "Project"}, Format: "raw"})
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
// format and its response_format alias to the same format is accepted and
// forwarded as one lowercase response_format parameter, whether the two agree
// by spelling in another case or through the json synonym, since the mismatch
// refusal is for formats that disagree and not for the alias being present at
// all.
func TestSchema_FormatAndAliasAgreeing_ForwardsOnce(t *testing.T) {
	tests := []struct {
		name   string
		format string
		alias  string
	}{
		{name: "same value in another case", format: "raw", alias: "RAW"},
		{name: "json synonym beside raw", format: "json", alias: "raw"},
		{name: "raw beside the json synonym", format: "raw", alias: "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
				if got := r.URL.Query()["response_format"]; len(got) != 1 || got[0] != "raw" {
					t.Errorf("response_format query parameter = %q, want one raw", got)
				}
				if r.URL.Query().Has("format") {
					t.Errorf("format query parameter = %q, want it absent", r.URL.Query().Get("format"))
				}
				testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0"}`)
			}))

			out, err := Schema(context.Background(), client, SchemaInput{Format: tt.format, ResponseFormat: tt.alias})
			if err != nil {
				t.Fatalf("Schema() error: %v", err)
			}
			if out.SchemaVersion != "1.0" {
				t.Fatalf("Schema() = %+v, want schema version 1.0", out)
			}
		})
	}
}

// TestSchema_LLMFormat_PublishesTheFormattedText verifies the whole of the llm
// path, which could not work before: the format reaches GitLab as
// response_format under either input name, and the compact text GitLab then
// answers with, as the only key of its body, is published as formatted_text
// and rendered as the card's body. client-go's OrbitSchema models no such key,
// so a handler reading only the SDK struct returned an empty ontology for it.
func TestSchema_LLMFormat_PublishesTheFormattedText(t *testing.T) {
	tests := []struct {
		name  string
		input SchemaInput
	}{
		{name: "through format", input: SchemaInput{Format: "llm"}},
		{name: "through the response_format alias", input: SchemaInput{ResponseFormat: " LLM "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
				testutil.AssertQueryParam(t, r, "response_format", "llm")
				if r.URL.Query().Has("format") {
					t.Errorf("format query parameter = %q, want it absent", r.URL.Query().Get("format"))
				}
				testutil.RespondJSON(w, http.StatusOK, `{"formatted_text":"schema: 12.1\nnodes: User, Project"}`)
			}))

			out, err := Schema(context.Background(), client, tt.input)
			if err != nil {
				t.Fatalf("Schema() error: %v", err)
			}
			if !reflect.DeepEqual(out, SchemaOutput{FormattedText: "schema: 12.1\nnodes: User, Project", Domains: []SchemaDomain{}, Nodes: []any{}, Edges: []SchemaEdge{}}) {
				t.Fatalf("Schema() = %+v, want the formatted text and nothing else", out)
			}
			md := FormatSchemaMarkdown(out)
			if !strings.Contains(md, "```text\nschema: 12.1\nnodes: User, Project\n```") {
				t.Errorf("FormatSchemaMarkdown() = %q, want the formatted text fenced as the body", md)
			}
			if strings.Contains(md, "Schema version") {
				t.Errorf("FormatSchemaMarkdown() = %q, want no structured fields beside the text", md)
			}
		})
	}
}

// TestSchema_CapturedBodyThatDoesNotDecode_ReturnsAnError verifies that a
// body client-go accepted and the formatted_text reader cannot decode is an
// error the handler returns rather than an ontology with the text silently
// missing. The only way to reach it is a response whose formatted_text is not a
// string, which the SDK does not read at all.
func TestSchema_CapturedBodyThatDoesNotDecode_ReturnsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0","formatted_text":42}`)
	}))

	_, err := Schema(context.Background(), client, SchemaInput{})
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Fatalf("Schema() error = %v, want the captured body's decode failure", err)
	}
}

// TestResponseFormatQuery_ReplacesAnExistingValue verifies that the request
// option sets response_format rather than adding a second one beside a value
// already on the query, and leaves every other parameter alone, and that with
// no format it leaves the request untouched.
func TestResponseFormatQuery_ReplacesAnExistingValue(t *testing.T) {
	llm := gl.OrbitResponseFormatLLM
	tests := []struct {
		name   string
		format *gl.OrbitResponseFormatValue
		want   string
	}{
		{name: "a format replaces the value", format: &llm, want: "expand=User&response_format=llm"},
		{name: "no format leaves the query as it was", format: nil, want: "expand=User&response_format=raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := retryablehttp.NewRequest(http.MethodGet, "https://gitlab.example.com/api/v4/orbit/schema?expand=User&response_format=raw", nil)
			if err != nil {
				t.Fatalf("NewRequest() error: %v", err)
			}
			if err = responseFormatQuery(tt.format)(req); err != nil {
				t.Fatalf("responseFormatQuery() error: %v", err)
			}
			if req.URL.RawQuery != tt.want {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.want)
			}
		})
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

// queryGraphGrammar stands in for the description GitLab.com gives the
// query_graph command, which is the grammar of the GQL it compiles queries in.
const queryGraphGrammar = "Read-only openCypher 9-based syntax (MATCH ... RETURN). No OR, XOR, DISTINCT, count(*), OPTIONAL MATCH, WITH or UNION."

// agentCommandsRaw is the raw answer of GET /api/v4/orbit/agent/commands in
// the shape GitLab.com gave it on 2026-10-17, with a second command ahead of
// query_graph so the reader has to find the query command rather than take the
// first one it meets.
const agentCommandsRaw = `[{"name":"list_commands","description":"Lists the commands.","parameters":{"type":"object"}},` +
	`{"name":"query_graph","description":"` + queryGraphGrammar + `",` +
	`"parameters":{"type":"object","required":["query"],"properties":{"query":{"type":"string","description":"Read-only GQL query text."}},"additionalProperties":false}}]`

// queryGraphParameters is the JSON Schema of the query_graph input in
// [agentCommandsRaw], decoded.
func queryGraphParameters() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"query"},
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Read-only GQL query text."},
		},
		"additionalProperties": false,
	}
}

// TestDSL_EachResponseFormat_ReadsTheQueryCommand verifies that [DSL] reads
// the language GitLab.com compiles queries in from the query_graph command of
// GET /orbit/agent/commands, and never asks GET /orbit/schema/dsl, which
// answers 404 since GitLab.com turned GQL on for everyone (issue 1414). The raw
// answer, asked for or left to GitLab's default, publishes the command, its
// grammar and the JSON Schema of its input; the llm answer publishes the
// compact text. The command is named in command_names, and a requested
// format reaches GitLab as response_format and none is sent otherwise.
func TestDSL_EachResponseFormat_ReadsTheQueryCommand(t *testing.T) {
	const (
		llmText = "commands[1]{name,description}:\n  query_graph,Read-only openCypher 9-based syntax"
		llmBody = `{"formatted_text":"commands[1]{name,description}:\n  query_graph,Read-only openCypher 9-based syntax"}`
	)
	cases := []struct {
		name   string
		format string
		body   string
		want   DSLOutput
	}{
		{
			name:   "raw",
			format: "raw",
			body:   agentCommandsRaw,
			want:   DSLOutput{ResponseFormat: "raw", Name: "query_graph", Description: queryGraphGrammar, Parameters: queryGraphParameters()},
		},
		{
			name: "no format, which GitLab answers raw",
			body: agentCommandsRaw,
			want: DSLOutput{Name: "query_graph", Description: queryGraphGrammar, Parameters: queryGraphParameters()},
		},
		{
			name:   "llm",
			format: "llm",
			body:   llmBody,
			want:   DSLOutput{ResponseFormat: "llm", FormattedText: llmText},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var retiredRoute atomic.Int32
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v4/orbit/schema/dsl" {
					retiredRoute.Add(1)
					testutil.RespondJSON(w, http.StatusNotFound, `{"code":5,"message":"Orbit request failed"}`)
					return
				}
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/agent/commands")
				testutil.AssertQueryParam(t, r, "command_names", "query_graph")
				testutil.AssertQueryParam(t, r, "response_format", tc.format)
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))

			out, err := DSL(context.Background(), client, DSLInput{ResponseFormat: tc.format})
			if err != nil {
				t.Fatalf("DSL() error: %v", err)
			}
			if !reflect.DeepEqual(out, tc.want) {
				t.Errorf("DSL() = %+v, want %+v", out, tc.want)
			}
			if n := retiredRoute.Load(); n != 0 {
				t.Errorf("DSL() asked GET /orbit/schema/dsl %d time(s), want never", n)
			}
		})
	}
}

// TestDSL_NoQueryCommand_PublishesNothing verifies that an answer describing
// no query_graph command, an empty list or one holding other commands only,
// publishes no language at all rather than describing another command as the
// query language.
func TestDSL_NoQueryCommand_PublishesNothing(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "an empty list", body: `[]`},
		{name: "other commands only", body: `[{"name":"list_commands","description":"Lists the commands.","parameters":{"type":"object"}}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/agent/commands")
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))

			out, err := DSL(context.Background(), client, DSLInput{ResponseFormat: "raw"})
			if err != nil {
				t.Fatalf("DSL() error: %v", err)
			}
			if want := (DSLOutput{ResponseFormat: "raw"}); !reflect.DeepEqual(out, want) {
				t.Errorf("DSL() = %+v, want %+v", out, want)
			}
		})
	}
}

// TestDSL_BodyThatDoesNotDecode_ReturnsAnError verifies that an answer [DSL]
// cannot read, an object whose text is not a string or anything but an object
// or a list of commands, is an error naming the operation and what failed to
// decode, rather than an empty card that reads as GitLab describing nothing.
func TestDSL_BodyThatDoesNotDecode_ReturnsAnError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "an object whose text is a number", body: `{"formatted_text":5}`, want: "decode the query command as text"},
		{name: "a string", body: `"query_graph"`, want: "decode the query command: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))

			_, err := DSL(context.Background(), client, DSLInput{})
			if err == nil || !strings.Contains(err.Error(), "orbit_dsl") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("DSL() error = %v, want orbit_dsl and %q", err, tc.want)
			}
		})
	}
}

// TestGetAgentCommands_PathThatCannotBeBuilt_ReturnsTheError verifies that
// the request client-go cannot build is reported rather than sent. No
// constant path produces one, which is why the path is a parameter.
func TestGetAgentCommands_PathThatCannotBeBuilt_ReturnsTheError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for a request that cannot be built: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))

	if _, err := getAgentCommands(context.Background(), client, "orbit/%zz", agentCommandsOptions{CommandNames: queryCommand}); err == nil {
		t.Fatal("getAgentCommands() error = nil, want the error of the request that cannot be built")
	}
}

// TestOrbitHandlers_CancelledOnArrival_ReturnCanceled verifies that [DSL] and
// [Query], on both of its paths, hand client-go the caller's context: the
// context ends when the request reaches the mock, so each returns
// context.Canceled promptly only if its request carried it.
func TestOrbitHandlers_CancelledOnArrival_ReturnCanceled(t *testing.T) {
	respond := func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}
	cases := []struct {
		name string
		call func(context.Context, *gitlabclient.Client) error
	}{
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

// projectByIDObject is the same query written as an object in version 12 of
// the JSON DSL, which GitLab.com read until 2026-10-15 and which [Query] still
// forwards as written.
func projectByIDObject() map[string]any {
	return map[string]any{
		"query_type": "traversal",
		"nodes":      []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}}},
	}
}

// projectByIDObjectText is that object written as JSON text, which a client
// that serializes a nested object sends as a string, and which GitLab.com
// refused on 2026-10-17 with the same "Orbit query syntax at line 1, column 1"
// it answers the object with.
const projectByIDObjectText = `{"query_type":"traversal","nodes":[{"id":"p","entity":"Project","node_ids":[1]}]}`

// TestQuery_EveryQueryShape_InGQL_ReachesTheWire verifies that a query of each
// shape GQL writes, each answered 200 by GitLab.com on 2026-10-17 against the
// fixture namespace, reaches GitLab as the text the caller wrote: a filtered
// traversal, an aggregation grouped by a node and sorted by its alias, the
// neighbors of one node and a shortest path between two bounded ones. Nothing
// here parses the text, so a quote, an arrow or a brace inside it travels as
// it is.
func TestQuery_EveryQueryShape_InGQL_ReachesTheWire(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "traversal", query: "MATCH (p:Project) WHERE p.full_path STARTS WITH 'plens1/' RETURN p.id, p.full_path LIMIT 3"},
		{name: "aggregation", query: "MATCH (mr:MergeRequest)-[:IN_PROJECT]->(p:Project) WHERE p.full_path STARTS WITH 'plens1/' RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC"},
		{name: "neighbors", query: "MATCH (p:Project {full_path: 'plens1/kg-fixtures'})--(n) RETURN n"},
		{name: "path_finding", query: "MATCH path = ANY SHORTEST (mr:MergeRequest {source_branch: 'feature/restock-helper'})-[:IN_PROJECT*1..2]->(p:Project {full_path: 'plens1/kg-fixtures'}) RETURN path"},
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
				testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"`+tt.name+`","row_count":0}`)
			}))

			out, err := Query(context.Background(), client, QueryInput{Query: tt.query})
			if err != nil {
				t.Fatalf("Query() error: %v", err)
			}
			if out.QueryType != tt.name {
				t.Fatalf("Query() query type = %q, want %q as GitLab inferred it", out.QueryType, tt.name)
			}
		})
	}
}

// TestQuery_JSONObject_IsForwardedAsWritten verifies that a query written as
// an object, in the version 12 shape of the JSON DSL GitLab.com answered 200
// to on 2026-10-05, is still forwarded to GitLab exactly as the caller wrote
// it rather than refused here: a filtered traversal, an aggregation, a
// neighbors query and a path. GitLab.com reads GQL now and refuses it, but an
// object keeps working if GitLab.com turns GQL back off before
// gitlab-org/gitlab!261947 removes the JSON DSL (issue 1414). The neighbors
// case is the shape the validator this package once carried refused before
// sending anything (issue 1031).
func TestQuery_JSONObject_IsForwardedAsWritten(t *testing.T) {
	tests := []struct {
		name  string
		query map[string]any
	}{
		{
			name: "traversal",
			query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{
					map[string]any{"id": "p", "entity": "Project", "filters": map[string]any{"full_path": map[string]any{"starts_with": "plens1/"}}, "columns": []any{"id", "full_path"}},
				},
				"limit": 3,
			},
		},
		{
			name: "aggregation",
			query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{"id": "p", "entity": "Project", "filters": map[string]any{"full_path": map[string]any{"starts_with": "plens1/"}}},
					map[string]any{"id": "mr", "entity": "MergeRequest"},
				},
				"relationships":    []any{map[string]any{"type": "IN_PROJECT", "from": "mr", "to": "p"}},
				"group_by":         []any{"p"},
				"aggregations":     []any{map[string]any{"count": "mr", "as": "mr_count"}},
				"aggregation_sort": "-mr_count",
			},
		},
		{
			name: "neighbors",
			query: map[string]any{
				"query_type": "neighbors",
				"nodes":      []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}}},
				"neighbors":  map[string]any{"direction": "both"},
				"limit":      5,
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
				"path": map[string]any{"type": "shortest", "from": "u", "to": "p", "max_depth": 3, "rel_types": []any{"*"}},
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
// raw Orbit API path for LLM/text responses instead of JSON decoding, sending
// the query as it was written. The llm answer carries no query type of its
// own: a query written as an object is labeled with the query_type it names,
// and a GQL one, whose type GitLab infers from the statement, with none.
func TestQuery_LLMResponseFormat_UsesRawResponse(t *testing.T) {
	cases := []struct {
		name      string
		query     any
		wantQuery any
		wantType  string
	}{
		{name: "GQL text", query: projectByIDGQL, wantQuery: projectByIDGQL, wantType: ""},
		{name: "a JSON object", query: projectByIDObject(), wantQuery: map[string]any{
			"query_type": "traversal",
			"nodes":      []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []any{float64(1)}}},
		}, wantType: "traversal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
				var got map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusInternalServerError)
					return
				}
				if want := map[string]any{"query": tc.wantQuery, "response_format": "llm"}; !reflect.DeepEqual(got, want) {
					t.Errorf("request body = %v, want %v", got, want)
					http.Error(w, "request body differs", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte("@header\nProject(name: gitlab)\n"))
			}))

			out, err := Query(context.Background(), client, QueryInput{
				Query:          tc.query,
				ResponseFormat: "llm",
			})
			if err != nil {
				t.Fatalf("Query() error: %v", err)
			}
			want := QueryOutput{FormattedText: "@header\nProject(name: gitlab)\n", QueryType: tc.wantType}
			if !reflect.DeepEqual(out, want) {
				t.Fatalf("Query() = %+v, want %+v", out, want)
			}
		})
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
// whole as formatted_text, the way the llm text is. It was refused here as
// "use raw or llm" while orbit.dsl taught it.
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

// TestQuery_UnknownResponseFormat_NamesTheThreeItTakes verifies that a
// response format POST /orbit/query does not declare is refused before
// anything is sent, with the three values the route takes.
func TestQuery_UnknownResponseFormat_NamesTheThreeItTakes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for an unknown response format: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	_, err := Query(context.Background(), client, QueryInput{Query: projectByIDGQL, ResponseFormat: "xml"})
	if err == nil || err.Error() != "invalid response_format: use raw, llm or gql" {
		t.Fatalf("Query() error = %v, want the three formats the query route takes", err)
	}
}

// TestOrbitReads_GQLResponseFormat_IsRefusedWhereGitLabDeclaresNone verifies
// that gql stays a format of the query route alone: status, the query
// language, the schema and the indexing status declare raw and llm, so gql is
// refused there before anything is sent rather than answered 400 by GitLab.
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
			_, err := Status(context.Background(), client, StatusInput{ResponseFormatInput{ResponseFormat: "gql"}})
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
			_, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "plens1", ResponseFormat: "gql"})
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
// OR, a syntax error), objects in the older shape of the JSON DSL this
// package taught (a top-level node, an {op, value} filter, a node named
// inside neighbors), an unbounded object, one with an unknown or missing
// query_type, and an empty object all reach GitLab exactly as the caller
// wrote them.
//
// The shape rules this package used to check were written for an older shape
// of the JSON DSL, and they refused the one neighbors query GitLab runs while
// letting through queries GitLab refuses (issue 1031). GitLab compiles every
// query and says what is wrong with it, so a local copy of its rules could
// only ever fall behind it.
func TestQuery_AnyShape_IsLeftForGitLabToJudge(t *testing.T) {
	tests := []struct {
		name  string
		query any
	}{
		{name: "an unbounded GQL node", query: "MATCH (p:Project) RETURN p LIMIT 5"},
		{name: "GQL with OR", query: "MATCH (p:Project) WHERE p.id = 1 OR p.id = 2 RETURN p.id"},
		{name: "GQL that does not parse", query: "MATCH (p:Project {id: 1}) RETURN p.id GROUP BY p.id"},
		{name: "a top-level node", query: map[string]any{"query_type": "traversal", "node": map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}}}},
		{name: "an op and value filter", query: map[string]any{"query_type": "traversal", "nodes": []any{map[string]any{"id": "p", "entity": "Project", "filters": map[string]any{"full_path": map[string]any{"op": "starts_with", "value": "plens1/"}}}}}},
		{name: "a node named inside neighbors", query: map[string]any{"query_type": "neighbors", "nodes": []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}}}, "neighbors": map[string]any{"node": "p"}}},
		{name: "an unbounded node", query: map[string]any{"query_type": "traversal", "nodes": []any{map[string]any{"id": "p", "entity": "Project"}}}},
		{name: "an unknown query_type", query: map[string]any{"query_type": "search", "nodes": []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}}}}},
		{name: "no query_type", query: map[string]any{"nodes": []any{map[string]any{"id": "p", "entity": "Project", "node_ids": []any{1}}}}},
		{name: "an empty object", query: map[string]any{}},
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
// path. The message, GitLab.com's own answer to an unbounded node on
// 2026-10-17, is what makes the refusal correctable, and before issue 1031 it
// was withheld: the code key read as the mark of a body something other than
// GitLab composed, so a model was told only "bad request".
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
// a caller who sent an object is told. While GitLab.com reads queries as GQL
// it parses the object's text as GQL and answers compile_error with "Orbit
// query syntax at line 1, column 1" (measured 2026-10-17), and once
// gitlab-org/gitlab!261947 removes the JSON DSL Grape answers "query is
// invalid": both get the hint that GitLab.com reads a query as GQL text, with
// an example, on the structured and the llm path. The object written as JSON
// text, which a client that serializes nested objects sends as a string, is
// refused the same way and gets the same hint. An object GitLab compiled as
// the JSON DSL and refused for what it asked, and GQL text refused for its
// syntax, keep the hint that points at the fault, since telling either to
// send GQL would send it the wrong way.
func TestQuery_JSONObjectRefusedForItsLanguage_SaysGitLabReadsGQL(t *testing.T) {
	const (
		gqlSyntax  = `{"code":"compile_error","message":"schema violation: Orbit query syntax at line 1, column 1: expected Statement"}`
		grape      = `{"error":"query is invalid"}`
		jsonSchema = `{"code":"compile_error","message":"schema violation: \"nodes\" is a required property"}`
	)
	cases := []struct {
		name   string
		query  any
		format string
		body   string
		hint   string
	}{
		{name: "an object GitLab.com parsed as GQL", query: projectByIDObject(), body: gqlSyntax, hint: queryObjectRefusedHint},
		{name: "an object GitLab.com parsed as GQL, llm", query: projectByIDObject(), format: "llm", body: gqlSyntax, hint: queryObjectRefusedHint},
		{name: "an object Grape refused", query: projectByIDObject(), body: grape, hint: queryObjectRefusedHint},
		{name: "an object refused as the JSON DSL", query: projectByIDObject(), body: jsonSchema, hint: queryRefusedHint},
		{name: "GQL text refused for its syntax", query: "MATCH (p:Project {id: 1}) RETURN", body: gqlSyntax, hint: queryRefusedHint},
		{name: "an object written as JSON text, parsed as GQL", query: projectByIDObjectText, body: gqlSyntax, hint: queryObjectRefusedHint},
		{name: "an object written as JSON text after white space, llm", query: "\n  " + projectByIDObjectText, format: "llm", body: gqlSyntax, hint: queryObjectRefusedHint},
		{name: "an object written as JSON text Grape refused", query: projectByIDObjectText, body: grape, hint: queryObjectRefusedHint},
		{name: "an object written as JSON text refused as the JSON DSL", query: projectByIDObjectText, body: jsonSchema, hint: queryRefusedHint},
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

// TestQuery_NeitherTextNorObject_IsRefusedBeforeSending verifies the one
// thing [Query] judges about a query: that there is one, as text that is not
// blank or as an object. A number, a list or a boolean is no query in any
// language GitLab reads, and is refused with what a query has to be rather
// than sent; a blank string is a missing query.
func TestQuery_NeitherTextNorObject_IsRefusedBeforeSending(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for a query that is not one: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	cases := []struct {
		name  string
		query any
		want  string
	}{
		{name: "a number", query: float64(42), want: "query must be GQL text or a JSON object, not float64"},
		{name: "a list", query: []any{"MATCH (p:Project {id: 1}) RETURN p.id"}, want: "query must be GQL text or a JSON object, not []interface {}"},
		{name: "a boolean", query: true, want: "query must be GQL text or a JSON object, not bool"},
		{name: "an empty string", query: "", want: "query is required"},
		{name: "blank text", query: " \n\t", want: "query is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Query(context.Background(), client, QueryInput{Query: tc.query})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Query() error = %v, want %q", err, tc.want)
			}
			if tc.want != "query is required" && !errors.Is(err, errQueryShape) {
				t.Fatalf("Query() error = %v, want it to wrap errQueryShape", err)
			}
		})
	}
}

// TestQueryType_NonString_ReturnsEmpty verifies that queryType labels only a
// query written as an object naming its query_type as a string: GQL text,
// whose type GitLab infers from the statement, and a malformed query_type
// value both leave the llm answer unlabeled.
func TestQueryType_NonString_ReturnsEmpty(t *testing.T) {
	cases := []struct {
		name  string
		query any
	}{
		{name: "a number for query_type", query: map[string]any{"query_type": 42}},
		{name: "GQL text", query: projectByIDGQL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := queryType(tc.query); got != "" {
				t.Fatalf("queryType() = %q, want empty", got)
			}
		})
	}
}

// TestResponseFormatName_NilReturnsEmpty verifies that responseFormatName returns
// an empty string for a nil format pointer, signaling "use GitLab's default"
// ("raw" on every Orbit route). The corresponding [responseFormat] helper also
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
			"projects": {"indexed": 3, "total_known": 4, "gaps": 1},
			"domains": [{"name": "SDLC", "items": [{"name": "MergeRequest", "count": 42}]}],
			"indexing": {"state": "indexed", "last_duration_ms": 99}
		}`)
	}))

	out, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	want := GraphStatusOutput{
		Projects: &GraphStatusProjects{Indexed: 3, TotalKnown: 4, Gaps: 1},
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

// TestGraphStatus_LLMFormat_HasNoProjectsToCountGapsIn verifies that the llm
// answer, formatted_text and nothing else, publishes the text and no project
// counts: the captured body carries no projects object, so there is no gaps
// count to read and no counts object to write it into.
func TestGraphStatus_LLMFormat_HasNoProjectsToCountGapsIn(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "response_format", "llm")
		testutil.RespondJSON(w, http.StatusOK, `{"formatted_text":"indexed: 2 of 2"}`)
	}))

	out, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "plens1", ResponseFormat: "llm"})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	want := GraphStatusOutput{FormattedText: "indexed: 2 of 2", Domains: []GraphStatusDomain{}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("GraphStatus() = %+v, want %+v", out, want)
	}
}

// TestGraphStatus_CapturedBodyThatDoesNotDecode_ReturnsAnError verifies that a
// gaps count the captured body carries as something other than a number is an
// error the handler reports, rather than a zero published as if GitLab had
// counted none: client-go ignores the key, so its own decode succeeds and
// only the read of the capture can notice.
func TestGraphStatus_CapturedBodyThatDoesNotDecode_ReturnsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"projects":{"indexed":1,"total_known":2,"gaps":"many"}}`)
	}))

	_, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "plens1"})
	if err == nil || !strings.Contains(err.Error(), "orbit_graph_status") || !strings.Contains(err.Error(), "decode the captured response") {
		t.Fatalf("GraphStatus() error = %v, want the captured body's decode failure under the operation", err)
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
			want: "use raw or llm",
		},
		{
			name: "empty query",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{})
				return err
			},
			want: "query is required",
		},
		{
			name: "unmarshalable query",
			call: func() error {
				// A func value cannot be JSON-encoded, which is one of the two
				// things about a query GitLab cannot be asked to judge, since
				// nothing can be sent. A caller decoding JSON never builds one;
				// a caller building the map in Go can.
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{
					"query_type": "traversal",
					"nodes": []any{map[string]any{
						"id":       "p",
						"entity":   "Project",
						"node_ids": []int{1},
						"bad":      func() {},
					}},
				}})
				return err
			},
			want: "query must be GQL text or a JSON object: json: unsupported type: func()",
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
			name: "json synonym conflicting with llm",
			call: func() error {
				_, err := Schema(context.Background(), client, SchemaInput{Format: "json", ResponseFormat: "llm"})
				return err
			},
			want: "must match",
		},
		{
			name: "invalid schema response_format beside a valid format",
			call: func() error {
				_, err := Schema(context.Background(), client, SchemaInput{Format: "raw", ResponseFormat: "xml"})
				return err
			},
			want: "invalid response_format: use raw or llm",
		},
		{
			name: "invalid schema format beside a valid response_format",
			call: func() error {
				_, err := Schema(context.Background(), client, SchemaInput{Format: "xml", ResponseFormat: "raw"})
				return err
			},
			want: "invalid format: use raw or llm",
		},
		{
			name: "invalid query response format",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{
					Query:          projectByIDGQL,
					ResponseFormat: "xml",
				})
				return err
			},
			want: "use raw, llm or gql",
		},
		{
			name: "invalid dsl response format",
			call: func() error {
				_, err := DSL(context.Background(), client, DSLInput{ResponseFormat: "xml"})
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
			path:   "/api/v4/orbit/agent/commands",
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
				_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			// A query GitLab refuses gets the hint naming where a correction
			// comes from, and only the query route gets it: the graph_status
			// case below keeps the hint every other route gives a 400.
			name:   "query refused",
			path:   "/api/v4/orbit/query",
			status: http.StatusBadRequest,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
				return err
			},
			want: "Suggestion: " + queryRefusedHint,
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
				_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
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
			_, err := Query(ctx, client, QueryInput{Query: projectByIDGQL})
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
	if md := FormatDSLMarkdown(DSLOutput{}); !strings.Contains(md, "GitLab described no query command") {
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
// caller sets both `format` and `response_format` to different formats,
// compared after each is normalized, which is the disagreement branch in
// [schemaResponseFormat].
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
// neither response_format nor format is sent, so GitLab applies its
// default.
func TestSchema_ResponseFormatAliasEmptyString(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		if r.URL.Query().Has("format") {
			t.Errorf("format query parameter = %q, want it absent (GitLab default)", r.URL.Query().Get("format"))
		}
		if r.URL.Query().Has("response_format") {
			t.Errorf("response_format query parameter = %q, want it absent (GitLab default)", r.URL.Query().Get("response_format"))
		}
		if r.URL.Query().Has("expand") {
			t.Errorf("expand query parameter = %q, want it absent when no node was named", r.URL.Query().Get("expand"))
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
