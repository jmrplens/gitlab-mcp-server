// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// mcpEndpoint is where the Orbit MCP endpoint is mounted.
const mcpEndpoint = "/api/v4/orbit/mcp"

// mcpCall is a JSON-RPC request as a test handler reads it.
type mcpCall struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

// readMCP decodes the JSON-RPC request a handler received.
func readMCP(t *testing.T, r *http.Request) mcpCall {
	t.Helper()
	testutil.AssertRequestMethod(t, r, http.MethodPost)
	testutil.AssertRequestPath(t, r, mcpEndpoint)
	var call mcpCall
	if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
		t.Errorf("decode MCP request: %v", err)
	}
	if call.JSONRPC != "2.0" || call.ID != 1 {
		t.Errorf("JSON-RPC envelope = %+v, want version 2.0 and id 1", call)
	}
	return call
}

// respondToolText answers a tools/call with one text block.
func respondToolText(w http.ResponseWriter, text string, isError bool) {
	encoded, err := json.Marshal(text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":`+
		string(encoded)+`}],"isError":`+strconv.FormatBool(isError)+`}}`)
}

// queryGraphParameters is the JSON Schema of the query_graph command's input
// as GitLab.com answered it on 2026-10-29.
func queryGraphParameters() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"query"},
		"properties": map[string]any{
			"query":  map[string]any{"type": "string", "description": "Read-only GQL query text."},
			"format": map[string]any{"type": "string", "enum": []any{"llm", "raw", "gql"}},
		},
		"additionalProperties": false,
	}
}

// queryGraphGrammar is the description of the query_graph command, cut from
// GitLab.com's answer of 2026-10-29.
const queryGraphGrammar = "Read-only openCypher 9-based syntax (MATCH ... RETURN). Page: SKIP 0-10000, LIMIT 1-1000. No OR, XOR, RETURN DISTINCT, count(*), OPTIONAL MATCH, WITH or UNION."

// agentCommandsRaw is list_commands' raw text, with a second command ahead of
// query_graph so the reader has to find the query command rather than take the
// first one it meets.
const agentCommandsRaw = `[{"name":"get_graph_schema","description":"Return the graph schema.","parameters":{"type":"object"}},` +
	`{"name":"query_graph","description":"` + queryGraphGrammar + `",` +
	`"parameters":{"type":"object","required":["query"],"properties":{"query":{"type":"string","description":"Read-only GQL query text."}},"additionalProperties":false}}]`

// orbitSourcesHandler answers every source the Orbit handlers read the way
// GitLab.com did on 2026-10-29: the GraphQL documents by the field they ask,
// the MCP methods and tools by name, and the query route. A request to
// anything else is reported, so a handler that went back to a removed route
// fails the test that reaches it.
func orbitSourcesHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case graphQLPath:
			req := readGraphQL(t, r)
			switch {
			case strings.Contains(req.Query, "serviceStatus"):
				testutil.RespondGraphQL(w, http.StatusOK, serviceStatusAnswer)
			case strings.Contains(req.Query, "OrbitSchema"):
				testutil.RespondGraphQL(w, http.StatusOK, schemaPageAnswer(false))
			default:
				testutil.RespondGraphQL(w, http.StatusOK, `{"group":{"orbit":`+graphStatusAnswer+`},"project":null}`)
			}
		case mcpEndpoint:
			call := readMCP(t, r)
			if call.Method == "tools/list" {
				testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"list_commands","description":"Lists commands","inputSchema":{"type":"object"}}]}}`)
				return
			}
			respondToolText(w, agentCommandsRaw, false)
		case "/api/v4/orbit/query":
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"traversal","row_count":0}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}
}

// commandsText is list_commands' raw answer: the command list as JSON text.
func commandsText(t *testing.T, commands ...map[string]any) string {
	t.Helper()
	text, err := json.Marshal(commands)
	if err != nil {
		t.Fatalf("marshal commands: %v", err)
	}
	return string(text)
}

// TestTools_ToolsList_MirrorsEveryTool verifies that [Tools] lists the tools
// of the Orbit MCP endpoint with tools/list and carries each one's name,
// description, input schema and trust flag, and nothing for a flag GitLab
// leaves out.
func TestTools_ToolsList_MirrorsEveryTool(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := readMCP(t, r)
		if call.Method != "tools/list" || len(call.Params) != 0 {
			t.Errorf("request = %+v, want tools/list with no parameter", call)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":[`+
			`{"name":"list_commands","description":"Lists commands","inputSchema":{"type":"object"},"trusted":true},`+
			`{"name":"invoke_command","description":"Runs a command","inputSchema":{"type":"object","required":["command_name"]}}]}}`)
	}))

	out, err := Tools(context.Background(), client, ToolsInput{})
	if err != nil {
		t.Fatalf("Tools() error: %v", err)
	}
	trusted := true
	want := ToolsOutput{Tools: []ToolDefinition{
		{Name: "list_commands", Description: "Lists commands", Parameters: map[string]any{"type": "object"}, Trusted: &trusted},
		{Name: "invoke_command", Description: "Runs a command", Parameters: map[string]any{"type": "object", "required": []any{"command_name"}}},
	}}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("Tools() = %+v, want %+v", out, want)
	}
}

// TestTools_NextCursor_IsFollowed verifies that a tool list answered in pages
// is read whole, each page asked with the cursor the one before it named.
func TestTools_NextCursor_IsFollowed(t *testing.T) {
	var calls atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := readMCP(t, r)
		if calls.Add(1) == 1 {
			testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a"}],"nextCursor":"c1"}}`)
			return
		}
		if call.Params["cursor"] != "c1" {
			t.Errorf("second page params = %v, want the cursor c1", call.Params)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"b"}]}}`)
	}))
	out, err := Tools(context.Background(), client, ToolsInput{})
	if err != nil {
		t.Fatalf("Tools() error: %v", err)
	}
	if len(out.Tools) != 2 || out.Tools[1].Name != "b" {
		t.Fatalf("Tools() = %+v, want both pages", out)
	}
}

// TestTools_ALaterPageRefused_FailsTheCall verifies that a refusal of a later
// page fails the call rather than listing the tools before it as the whole
// list.
func TestTools_ALaterPageRefused_FailsTheCall(t *testing.T) {
	var calls atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readMCP(t, r)
		if calls.Add(1) == 1 {
			testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a"}],"nextCursor":"c1"}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusTooManyRequests, `{"message":"429"}`)
	}))
	_, err := Tools(context.Background(), client, ToolsInput{})
	if err == nil || !strings.Contains(err.Error(), "rate-limited") {
		t.Fatalf("Tools() error = %v, want the refusal of the second page", err)
	}
}

// TestTools_CursorThatNeverEnds_StopsAtThePageBound verifies that a tool list
// whose cursor never ends stops after maxToolPages requests and says so.
func TestTools_CursorThatNeverEnds_StopsAtThePageBound(t *testing.T) {
	var calls atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readMCP(t, r)
		calls.Add(1)
		testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":[],"nextCursor":"again"}}`)
	}))
	_, err := Tools(context.Background(), client, ToolsInput{})
	if err == nil || !strings.Contains(err.Error(), "did not end within 20 pages") || calls.Load() != maxToolPages {
		t.Fatalf("Tools() error = %v after %d requests, want the page bound", err, calls.Load())
	}
}

// TestTools_ResultThatDoesNotDecode_IsAnError verifies that a tools/list
// result that is not a tool list fails the call rather than listing nothing.
func TestTools_ResultThatDoesNotDecode_IsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readMCP(t, r)
		testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"tools":"none"}}`)
	}))
	_, err := Tools(context.Background(), client, ToolsInput{})
	if err == nil || !strings.Contains(err.Error(), "decode the Orbit MCP tool list") {
		t.Fatalf("Tools() error = %v, want the decode failure", err)
	}
}

// TestDSL_EachResponseFormat_ReadsTheQueryCommand verifies [DSL] against
// list_commands: it asks for query_graph alone, sends raw when the caller
// chose no format, since the tool's own default is llm and the REST route's
// was raw, and publishes the command or its compact text, echoing the format
// the caller asked for.
func TestDSL_EachResponseFormat_ReadsTheQueryCommand(t *testing.T) {
	description := "Read-only openCypher 9-based syntax (MATCH ... RETURN)."
	queryGraph := map[string]any{"name": "query_graph", "description": description, "parameters": queryGraphParameters()}
	schemaCommandOnly := map[string]any{"name": "get_graph_schema"}
	cases := []struct {
		name   string
		input  string
		sent   string
		answer string
		want   DSLOutput
	}{
		{
			name: "default", input: "", sent: "raw", answer: commandsText(t, queryGraph),
			want: DSLOutput{Name: "query_graph", Description: description, Parameters: queryGraphParameters()},
		},
		{
			name: "json read as raw", input: "json", sent: "raw", answer: commandsText(t, schemaCommandOnly, queryGraph),
			want: DSLOutput{ResponseFormat: "raw", Name: "query_graph", Description: description, Parameters: queryGraphParameters()},
		},
		{
			name: "llm", input: " LLM ", sent: "llm", answer: "commands[1]:\n  - name: query_graph",
			want: DSLOutput{ResponseFormat: "llm", FormattedText: "commands[1]:\n  - name: query_graph"},
		},
		{
			name: "raw without the query command", input: "raw", sent: "raw", answer: commandsText(t, schemaCommandOnly),
			want: DSLOutput{ResponseFormat: "raw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := readMCP(t, r)
				wantParams := map[string]any{"name": "list_commands", "arguments": map[string]any{"command_names": []any{"query_graph"}, "format": tc.sent}}
				if call.Method != "tools/call" || !reflect.DeepEqual(call.Params, wantParams) {
					t.Errorf("request = %+v, want tools/call with %v", call, wantParams)
				}
				respondToolText(w, tc.answer, false)
			}))
			out, err := DSL(context.Background(), client, DSLInput{ResponseFormatInput{ResponseFormat: tc.input}})
			if err != nil {
				t.Fatalf("DSL() error: %v", err)
			}
			if !reflect.DeepEqual(out, tc.want) {
				t.Fatalf("DSL() = %+v, want %+v", out, tc.want)
			}
		})
	}
}

// TestDSL_RawTextThatIsNotACommandList_IsAnError verifies that a raw answer
// that is not the JSON list of commands fails the call.
func TestDSL_RawTextThatIsNotACommandList_IsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readMCP(t, r)
		respondToolText(w, "commands[1]: not JSON", false)
	}))
	_, err := DSL(context.Background(), client, DSLInput{})
	if err == nil || !strings.Contains(err.Error(), "decode the query command") {
		t.Fatalf("DSL() error = %v, want the decode failure", err)
	}
}

// TestMCPTools_Refusals_CarryGitLabsText verifies the three ways the Orbit MCP
// endpoint refuses: a tool result marked isError, whose text is GitLab's
// reason; a JSON-RPC error object; and an HTTP refusal, which GitLab sends a
// request for an unknown tool with (400) and an account Orbit does not serve
// with (403), classified the way every Orbit failure is.
func TestMCPTools_Refusals_CarryGitLabsText(t *testing.T) {
	cases := []struct {
		name    string
		respond func(http.ResponseWriter)
		want    string
	}{
		{
			name:    "tool result marked isError",
			respond: func(w http.ResponseWriter) { respondToolText(w, "Unknown command(s): query_graph", true) },
			want:    "refused: Unknown command(s): query_graph",
		},
		{
			name: "JSON-RPC error with 200",
			respond: func(w http.ResponseWriter) {
				testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"Invalid params"}}`)
			},
			want: "answered error -32602: Invalid params",
		},
		{
			name: "403",
			respond: func(w http.ResponseWriter) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			want: orbitUnavailableHint,
		},
		{
			name: "result that is not a tool result",
			respond: func(w http.ResponseWriter) {
				testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"content":"text"}}`)
			},
			want: "decode the result of Orbit MCP tool",
		},
	}
	calls := []struct {
		name string
		call func(context.Context, *testClient) error
	}{
		{name: "dsl", call: func(ctx context.Context, client *testClient) error {
			_, err := DSL(ctx, client, DSLInput{})
			return err
		}},
		{name: "schema llm", call: func(ctx context.Context, client *testClient) error {
			_, err := Schema(ctx, client, SchemaInput{Format: "llm"})
			return err
		}},
	}
	for _, call := range calls {
		for _, tc := range cases {
			t.Run(call.name+", "+tc.name, func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					readMCP(t, r)
					tc.respond(w)
				}))
				err := call.call(context.Background(), client)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
			})
		}
	}
}

// TestTools_Refusals_CarryTheOrbitHint verifies that tools/list refused over
// HTTP or with a JSON-RPC error fails the call with GitLab's reason.
func TestTools_Refusals_CarryTheOrbitHint(t *testing.T) {
	cases := []struct {
		name    string
		respond func(http.ResponseWriter)
		want    string
	}{
		{name: "404", respond: func(w http.ResponseWriter) {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		}, want: orbitUnavailableHint},
		{name: "JSON-RPC error", respond: func(w http.ResponseWriter) {
			testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`)
		}, want: "answered error -32601: Method not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readMCP(t, r)
				tc.respond(w)
			}))
			_, err := Tools(context.Background(), client, ToolsInput{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Tools() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestSchema_LLMFormat_AsksGetGraphSchema verifies the compact text of the
// schema, which GraphQL does not render: [Schema] invokes get_graph_schema
// with format llm and the nodes named in expand, and publishes the text whole,
// the text blocks joined and any other block left out.
func TestSchema_LLMFormat_AsksGetGraphSchema(t *testing.T) {
	cases := []struct {
		name       string
		input      SchemaInput
		parameters map[string]any
	}{
		{name: "no expand", input: SchemaInput{Format: "llm"}, parameters: map[string]any{"format": "llm"}},
		{name: "expand", input: SchemaInput{ResponseFormat: "llm", Expand: []string{"Project"}}, parameters: map[string]any{"format": "llm", "expand_nodes": []any{"Project"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := readMCP(t, r)
				want := map[string]any{"name": "invoke_command", "arguments": map[string]any{"command_name": "get_graph_schema", "parameters": tc.parameters}}
				if call.Method != "tools/call" || !reflect.DeepEqual(call.Params, want) {
					t.Errorf("request = %+v, want tools/call with %v", call, want)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"content":[`+
					`{"type":"text","text":"domains[9]:"},{"type":"image","data":"AA=="},{"type":"text","text":"edges[60]: APPROVED"}]}}`)
			}))
			out, err := Schema(context.Background(), client, tc.input)
			if err != nil {
				t.Fatalf("Schema() error: %v", err)
			}
			if want := (SchemaOutput{FormattedText: "domains[9]:\nedges[60]: APPROVED"}); !reflect.DeepEqual(out, want) {
				t.Fatalf("Schema() = %+v, want %+v", out, want)
			}
		})
	}
}

// TestCallMCP_PathThatCannotBeBuilt_ReturnsTheError verifies the one failure
// before anything is sent: a request client-go cannot build.
func TestCallMCP_PathThatCannotBeBuilt_ReturnsTheError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for a request that cannot be built: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	if _, err := callMCP(context.Background(), client, "orbit/%zz", "tools/list", map[string]any{}); err == nil {
		t.Fatal("callMCP() error = nil, want the error of the request that cannot be built")
	}
	if _, err := callMCPTool(context.Background(), client, "orbit/%zz", "list_commands", nil); err == nil {
		t.Fatal("callMCPTool() error = nil, want the error of the request that cannot be built")
	}
	if _, err := schemaText(context.Background(), client, "orbit/%zz", nil); err == nil {
		t.Fatal("schemaText() error = nil, want the error of the request that cannot be built")
	}
}

// TestCallMCPTool_IsError_WrapsErrToolRefused verifies that a refused tool
// result can be told apart from a transport failure.
func TestCallMCPTool_IsError_WrapsErrToolRefused(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readMCP(t, r)
		respondToolText(w, "schema violation", true)
	}))
	_, err := callMCPTool(context.Background(), client, mcpPath, "invoke_command", map[string]any{})
	if !errors.Is(err, errToolRefused) {
		t.Fatalf("callMCPTool() error = %v, want errToolRefused", err)
	}
}
