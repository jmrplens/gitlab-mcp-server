// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestNamedQuery_RawAnswer_IsTheQueryEnvelope holds [NamedQuery] to the
// request GitLab.com answered on 2026-10-29: POST /orbit/query/:name with a
// JSON body of the parameters and the format, raw when the caller chose none,
// the name without the spaces around it, and the answer decoded as the envelope
// orbit.query decodes.
func TestNamedQuery_RawAnswer_IsTheQueryEnvelope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query/context_for_job")
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type = %q, want JSON: GitLab refuses a form body", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		want := map[string]any{"parameters": map[string]any{"node_ids": []any{float64(14831871432)}, "limit": float64(100)}, "response_format": "raw"}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("request body = %v, want %v", body, want)
		}
		testutil.RespondJSON(w, http.StatusOK, readFixture(t, "named_query_recent_merges_raw.json"))
	}))
	out, err := NamedQuery(context.Background(), client, NamedQueryInput{
		Name:       " context_for_job\n",
		Parameters: map[string]any{"node_ids": []any{14831871432}, "limit": 100},
	})
	if err != nil {
		t.Fatalf("NamedQuery() error: %v", err)
	}
	if out.QueryType != "traversal" || out.RowCount != 2 || out.FormattedText != "" {
		t.Fatalf("NamedQuery() = %+v, want the traversal envelope with two rows", out)
	}
	result, _ := out.Result.(map[string]any)
	if nodes, _ := result["nodes"].([]any); len(nodes) != 2 {
		t.Errorf("result nodes = %v, want the two nodes GitLab answered", result["nodes"])
	}
}

// TestNamedQuery_NoParameters_SendsAnEmptyObject verifies that a named query
// that takes no parameters is sent with an empty parameters object, which
// GitLab.com answered for top_mr_authors on 2026-10-29, and that the text
// formats are published as GitLab answers them.
func TestNamedQuery_NoParameters_SendsAnEmptyObject(t *testing.T) {
	llm := readFixture(t, "named_query_recent_merges_llm.txt")
	for _, format := range []string{"llm", "gql"} {
		t.Run(format, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if want := map[string]any{"parameters": map[string]any{}, "response_format": format}; !reflect.DeepEqual(body, want) {
					t.Errorf("request body = %v, want %v", body, want)
				}
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(llm))
			}))
			out, err := NamedQuery(context.Background(), client, NamedQueryInput{Name: "recent_merges", ResponseFormat: format})
			if err != nil {
				t.Fatalf("NamedQuery() error: %v", err)
			}
			if out.FormattedText != llm || out.Result != nil {
				t.Errorf("NamedQuery() = %+v, want the text GitLab answered", out)
			}
		})
	}
}

// callNamedQuery calls orbit.named_query through an MCP session with the
// arguments as JSON text, so each number reaches the action the way a client's
// does, and returns the result's text and whether it is an error.
func callNamedQuery(t *testing.T, handler http.HandlerFunc, name, parameters string) (text string, isError bool) {
	t.Helper()
	session := newOrbitMetaSession(t, handler)
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "gitlab_orbit",
		Arguments: json.RawMessage(`{"action":"named_query","params":{"name":"` + name + `","parameters":` + parameters + `}}`),
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("CallTool() = %+v, want content", result)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	return content.Text, result.IsError
}

// TestNamedQuery_GraphIDs_ReachGitLabExactly verifies that an id reaches GitLab
// digit by digit. The ids of definitions and files have up to 63 bits
// (662267755689139244 in the 2026-10-29 capture), which a JSON number decoded
// as a float64 does not carry, and GitLab takes node_ids as integers alone (it
// refused "14831871432" there): a decimal string in node_ids is sent as the
// integer it spells and a number below 2^53 as it came. What is no id stays
// the caller's and is GitLab's to refuse, and node_id, which GitLab takes as a
// string, stays one.
func TestNamedQuery_GraphIDs_ReachGitLabExactly(t *testing.T) {
	cases := []struct {
		name       string
		parameters string
		want       string
	}{
		{name: "decimal strings", parameters: `{"entity":"Definition","node_ids":["662267755689139244","7035406773376642791"],"limit":10}`, want: `{"entity":"Definition","limit":10,"node_ids":[662267755689139244,7035406773376642791]}`},
		{name: "the largest id", parameters: `{"node_ids":["9223372036854775807"]}`, want: `{"node_ids":[9223372036854775807]}`},
		{name: "numbers up to the last exact one", parameters: `{"node_ids":[14831871432,9007199254740991]}`, want: `{"node_ids":[14831871432,9007199254740991]}`},
		{name: "a negative number as far from zero", parameters: `{"offset":-9007199254740991}`, want: `{"offset":-9007199254740991}`},
		{name: "a string beside a number", parameters: `{"node_ids":["662267755689139244",14831871432]}`, want: `{"node_ids":[662267755689139244,14831871432]}`},
		{name: "strings that are no id", parameters: `{"node_ids":["main","9223372036854775808"]}`, want: `{"node_ids":["main","9223372036854775808"]}`},
		{name: "node_ids that is no list", parameters: `{"node_ids":"662267755689139244"}`, want: `{"node_ids":"662267755689139244"}`},
		{name: "node_id, a string", parameters: `{"node_id":"662267755689139244"}`, want: `{"node_id":"662267755689139244"}`},
		{name: "values that hold no number", parameters: `{"entity":"File","filter":{"paths":["a"],"deep":[{"b":true}]},"text":null}`, want: `{"entity":"File","filter":{"deep":[{"b":true}],"paths":["a"]},"text":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent atomic.Value
			text, isError := callNamedQuery(t, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				}
				sent.Store(strings.TrimSpace(string(body)))
				testutil.RespondJSON(w, http.StatusOK, readFixture(t, "named_query_recent_merges_raw.json"))
			}, "expand_neighbors", tc.parameters)
			if isError {
				t.Fatalf("named_query = %s, want GitLab's answer", text)
			}
			if want := `{"parameters":` + tc.want + `,"response_format":"raw"}`; sent.Load() != want {
				t.Errorf("request body = %v, want %s", sent.Load(), want)
			}
		})
	}
}

// TestNamedQuery_ANumberTooLargeToBeExact_IsRefusedBeforeSending verifies that
// a number from 2^53 on, which is decoded as another number by the time the
// action reads it, is refused by the parameter that holds it, wherever in it
// the number is, before GitLab runs the query on an id the caller never gave.
// 9007199254740993 is decoded as 2^53 itself, so 2^53 is refused too.
func TestNamedQuery_ANumberTooLargeToBeExact_IsRefusedBeforeSending(t *testing.T) {
	cases := []struct {
		name       string
		parameters string
		parameter  string
	}{
		{name: "an id in node_ids", parameters: `{"entity":"Definition","node_ids":[14831871432,662267755689139244],"limit":10}`, parameter: "node_ids"},
		{name: "a number as node_id", parameters: `{"node_id":662267755689139244}`, parameter: "node_id"},
		{name: "2^53", parameters: `{"node_ids":[9007199254740992]}`, parameter: "node_ids"},
		{name: "2^53 and one", parameters: `{"node_ids":[9007199254740993]}`, parameter: "node_ids"},
		{name: "2^53 below zero", parameters: `{"offset":-9007199254740992}`, parameter: "offset"},
		{name: "inside an object of a list", parameters: `{"filter":[{"ids":[9007199254740993]}]}`, parameter: "filter"},
		{name: "two parameters, the first by name", parameters: `{"node_ids":[9007199254740993],"limit":9007199254740993}`, parameter: "limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, isError := callNamedQuery(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("request sent for a number refused before one: %s %s", r.Method, r.URL.Path)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{}`)
			}, "expand_neighbors", tc.parameters)
			if !isError || !strings.Contains(text, "parameters."+tc.parameter+" holds") || !strings.Contains(text, "decimal string") {
				t.Fatalf("named_query (error %t) = %s, want parameters.%s refused with the decimal string to send", isError, text, tc.parameter)
			}
		})
	}
}

// TestNamedQuery_GitLabsRefusals_CarryTheirHint covers the 400 invalid_request
// GitLab.com answered on 2026-10-29: an unknown named query, which lists the
// available ones, an unknown parameter and a missing one, each passed on with
// GitLab's words and the hint pointing at the parameters' description.
func TestNamedQuery_GitLabsRefusals_CarryTheirHint(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		input   NamedQueryInput
		message string
	}{
		{name: "an unknown named query", answer: "named_query_unknown.json", input: NamedQueryInput{Name: "no_such_query"}, message: "unknown named query `no_such_query`"},
		{name: "an unknown parameter", answer: "named_query_unknown_parameter.json", input: NamedQueryInput{Name: "top_mr_authors", Parameters: map[string]any{"unexpected": 1}}, message: "Valid parameters: none"},
		{name: "a missing parameter", answer: "named_query_missing_parameter.json", input: NamedQueryInput{Name: "context_for_job"}, message: "missing required parameter `limit`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := readFixture(t, tc.answer)
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusBadRequest, body)
			}))
			_, err := NamedQuery(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.message) || !strings.Contains(err.Error(), namedQueryRefusedHint) {
				t.Fatalf("NamedQuery() error = %v, want GitLab's %q and the named query hint", err, tc.message)
			}
		})
	}
}

// TestNamedQueryRefusal_OnlyGitLabsShape_IsRead holds the reading of a named
// query's refusal to GitLab's exact shape: the three codes that name the
// request as the fault, each with its message on one line, and a body with a
// key more, a code that names the service, a code or a message that is no
// string or a body that is not JSON not reflected, so a gateway's answer on
// the same route never reaches the caller as GitLab's words.
func TestNamedQueryRefusal_OnlyGitLabsShape_IsRead(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "GitLab's refusal", body: `{"code":"invalid_request","message":"unknown named query"}`, want: "invalid_request: unknown named query"},
		{name: "a query that did not compile", body: `{"code":"compile_error","message":"unknown\nentity"}`, want: "compile_error: unknown entity"},
		{name: "a query that did not validate", body: `{"code":"validation_error","message":"limit"}`, want: "validation_error: limit"},
		{name: "a key more", body: `{"code":"invalid_request","message":"x","upstream":"web-03"}`},
		{name: "a key less", body: `{"code":"invalid_request"}`},
		{name: "a service code", body: `{"code":"internal_error","message":"x"}`},
		{name: "a code that is no string", body: `{"code":400,"message":"x"}`},
		{name: "a message that is no string", body: `{"code":"invalid_request","message":["x"]}`},
		{name: "a null message", body: `{"code":"invalid_request","message":null}`},
		{name: "not JSON", body: `<html>bad gateway</html>`},
		{name: "GitLab's message shape", body: `{"message":"400 Bad request"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusBadRequest, tc.body)
			}))
			_, err := NamedQuery(context.Background(), client, NamedQueryInput{Name: "recent_merges"})
			if got := namedQueryRefusal(err); got != tc.want {
				t.Errorf("namedQueryRefusal() = %q, want %q", got, tc.want)
			}
		})
	}
	if got := namedQueryRefusal(errors.New("dial tcp: refused")); got != "" {
		t.Errorf("namedQueryRefusal() of an error with no answer = %q, want none", got)
	}
}

// TestNamedQuerySchema_OffersATemplateAsItsExample holds the input schema of
// orbit.named_query to the one example its guidance binds, a template, which
// find fills the call it offers with: the name's own placeholder is no named
// query.
func TestNamedQuerySchema_OffersATemplateAsItsExample(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	schema := orbitActionSpecRoutes(t, client)["named_query"].InputSchema
	if got := propertySchema(t, schema, "name")["examples"]; !reflect.DeepEqual(got, []any{namedQueryExample}) {
		t.Errorf("name examples = %#v, want %q", got, namedQueryExample)
	}
	if got := namedQueryGuidance()["name"].ExampleBinding; got != namedQueryExample {
		t.Errorf("name ExampleBinding = %q, want %q", got, namedQueryExample)
	}
}

// TestNamedQuery_InvalidInput_IsRefusedBeforeSending covers what the handler
// refuses before a request: no name, a name that is not a named query's
// spelling (which would also leave its one path segment), and a format the
// query route does not take.
func TestNamedQuery_InvalidInput_IsRefusedBeforeSending(t *testing.T) {
	cases := []struct {
		name  string
		input NamedQueryInput
		want  string
	}{
		{name: "no name", input: NamedQueryInput{}, want: "name"},
		{name: "a path in the name", input: NamedQueryInput{Name: "../query"}, want: "lowercase letters, digits and underscores"},
		{name: "a capital letter", input: NamedQueryInput{Name: "Recent_merges"}, want: "lowercase letters, digits and underscores"},
		{name: "a format the route does not take", input: NamedQueryInput{Name: "recent_merges", ResponseFormat: "yaml"}, want: "invalid response_format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("request sent for an input refused before one: %s %s", r.Method, r.URL.Path)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{}`)
			}))
			_, err := NamedQuery(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NamedQuery() error = %v, want it to say %q", err, tc.want)
			}
		})
	}
}

// TestNamedQueryParameters_EveryServedText_ListsEveryParameterizedQuery holds
// both texts served about the parameters to the twelve named queries that take
// them, the ones nothing GitLab serves lists, each with the parameters
// GitLab.com named for it on 2026-10-29: the schema's description, the one the
// individual surface serves, and the guidance, which the other two serve.
func TestNamedQueryParameters_EveryServedText_ListsEveryParameterizedQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	description, _ := propertySchema(t, orbitActionSpecRoutes(t, client)["named_query"].InputSchema, "parameters")["description"].(string)
	texts := []struct {
		name, text string
	}{
		{name: "the schema's description", text: description},
		{name: "the guidance", text: namedQueryGuidance()["parameters"].ValueSource},
	}
	queries := []string{
		"context_for_issue, context_for_job, context_for_review, expand_merge_request and expand_work_item take node_ids (a list of ids) and limit, both required",
		"expand_neighbors takes entity, node_ids and limit",
		"definition_callees and definition_references take node_id, a string",
		"file_definitions and file_definition_callers take project_id and file_path",
		"list_nodes takes entity",
		"search_nodes takes entity, field and text",
	}
	for _, tc := range texts {
		t.Run(tc.name, func(t *testing.T) {
			if lacks := missingFrom(tc.text, queries...); len(lacks) > 0 {
				t.Errorf("%s lacks %q: %s", tc.name, lacks, tc.text)
			}
		})
	}
}

// TestNamedQuery_WhatPointsAtTheParameters_NamesATextEverySurfaceServes holds
// the two sentences that send a caller to the parameters of a named query, the
// action's usage and the hint of a refusal, to the description of the parameter
// rather than the guidance, which the individual surface does not serve.
func TestNamedQuery_WhatPointsAtTheParameters_NamesATextEverySurfaceServes(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	sentences := []struct {
		name, text string
	}{
		{name: "the usage", text: orbitActionSpecRoutes(t, client)["named_query"].Usage},
		{name: "the hint of a refusal", text: namedQueryRefusedHint},
	}
	for _, tc := range sentences {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.text, "the description of parameters lists") || strings.Contains(tc.text, "guidance") {
				t.Errorf("%s = %q, want it to name the description of parameters", tc.name, tc.text)
			}
		})
	}
}
