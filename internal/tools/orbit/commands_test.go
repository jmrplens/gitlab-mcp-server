// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestCommandList_RawAnswer_NamesTheActionThatRunsEachCommand holds
// [CommandList] to the list_commands call it makes, raw when the caller
// chose no format, and to the three commands GitLab.com answered on
// 2026-10-29 (descriptions this project's): each published with its input
// schema and the action of this server that runs it.
func TestCommandList_RawAnswer_NamesTheActionThatRunsEachCommand(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := readMCP(t, r)
		want := map[string]any{"name": "list_commands", "arguments": map[string]any{"format": "raw"}}
		if call.Method != "tools/call" || !reflect.DeepEqual(call.Params, want) {
			t.Errorf("MCP call = %s %v, want tools/call %v", call.Method, call.Params, want)
		}
		respondToolText(w, readFixture(t, "list_commands_raw.json"), false)
	}))
	out, err := CommandList(context.Background(), client, CommandListInput{})
	if err != nil {
		t.Fatalf("CommandList() error: %v", err)
	}
	if out.ResponseFormat != "" || out.FormattedText != "" {
		t.Errorf("CommandList() = %+v, want no format named and no text", out)
	}
	got := map[string]string{}
	for _, command := range out.Commands {
		got[command.Name] = command.PerformedBy
		if command.Parameters == nil || command.Description == "" {
			t.Errorf("command %s = %+v, want its description and input schema", command.Name, command)
		}
	}
	want := map[string]string{"query_graph": "orbit.query", "get_graph_schema": "orbit.schema", "get_response_format": "orbit.response_format"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("commands and their actions = %v, want %v", got, want)
	}
}

// TestCommandList_NamesAndLLM_AreSentAsGitLabTakesThem verifies the
// arguments of a call naming commands in the llm format: the names trimmed
// with the empty ones dropped, the format as GitLab spells it, and GitLab's
// text published whole.
func TestCommandList_NamesAndLLM_AreSentAsGitLabTakesThem(t *testing.T) {
	const text = "commands[1]:\n  - name: query_graph\n    description: First command of this answer."
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := readMCP(t, r)
		want := map[string]any{"name": "list_commands", "arguments": map[string]any{"format": "llm", "command_names": []any{"query_graph"}}}
		if !reflect.DeepEqual(call.Params, want) {
			t.Errorf("MCP call params = %v, want %v", call.Params, want)
		}
		respondToolText(w, text, false)
	}))
	out, err := CommandList(context.Background(), client, CommandListInput{
		CommandNames:   []string{" query_graph ", ""},
		ResponseFormat: "llm",
	})
	if err != nil {
		t.Fatalf("CommandList() error: %v", err)
	}
	if out.FormattedText != text || out.ResponseFormat != "llm" || out.Commands != nil {
		t.Errorf("CommandList() = %+v, want the llm text alone", out)
	}
}

// TestCommandList_AsManyNamesAsTheBound_AreSent holds the bound on the names
// to a list longer than it: one of exactly that many is sent whole.
func TestCommandList_AsManyNamesAsTheBound_AreSent(t *testing.T) {
	names := strings.Fields(strings.Repeat("x ", maxCommandNames))
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arguments, _ := readMCP(t, r).Params["arguments"].(map[string]any)
		if sent, _ := arguments["command_names"].([]any); len(sent) != maxCommandNames {
			t.Errorf("command_names = %v, want the %d names", sent, maxCommandNames)
		}
		respondToolText(w, "[]", false)
	}))
	if _, err := CommandList(context.Background(), client, CommandListInput{CommandNames: names}); err != nil {
		t.Fatalf("CommandList() error: %v", err)
	}
}

// TestCommandList_RefusedOrMalformed_IsAnError covers what does not reach a
// list: a name GitLab does not serve, which list_commands answers as a tool
// error naming it (13e in the capture) and the action follows with a hint, a
// raw answer that does not decode, a format GitLab does not take and a list of
// names longer than the bound, the last two refused before a request.
func TestCommandList_RefusedOrMalformed_IsAnError(t *testing.T) {
	cases := []struct {
		name     string
		input    CommandListInput
		answer   string
		isError  bool
		want     string
		sendsNot bool
	}{
		{name: "an unknown command", input: CommandListInput{CommandNames: []string{"no_such_command"}}, answer: readFixture(t, "list_commands_unknown.txt"), isError: true, want: "Unknown command(s): no_such_command. Suggestion: " + commandRefusedHint},
		{name: "a raw answer that is not a list", answer: `{"not":"a list"}`, want: "decode the Orbit command list"},
		{name: "a format GitLab does not take", input: CommandListInput{ResponseFormat: "yaml"}, want: "invalid response_format", sendsNot: true},
		{name: "too many names", input: CommandListInput{CommandNames: strings.Fields(strings.Repeat("x ", maxCommandNames+1))}, want: "at most 20 are taken", sendsNot: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.sendsNot {
					t.Errorf("request sent for an input refused before one: %s %s", r.Method, r.URL.Path)
				}
				readMCP(t, r)
				respondToolText(w, tc.answer, tc.isError)
			}))
			_, err := CommandList(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CommandList() error = %v, want it to say %q", err, tc.want)
			}
			if tc.isError && !errors.Is(err, errToolRefused) {
				t.Errorf("CommandList() error = %v, want it to wrap errToolRefused", err)
			}
		})
	}
}

// TestQueryResponseFormat_RawAnswer_PublishesTheSchemaAndVersion holds
// [QueryResponseFormat] to the invoke_command call it makes, raw when the
// caller chose no format, and to GitLab.com's raw answer of 2026-10-29 (the
// schema cut to two properties): the version and the JSON Schema as they came.
func TestQueryResponseFormat_RawAnswer_PublishesTheSchemaAndVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := readMCP(t, r)
		want := map[string]any{"name": "invoke_command", "arguments": map[string]any{
			"command_name": "get_response_format", "parameters": map[string]any{"format": "raw"},
		}}
		if !reflect.DeepEqual(call.Params, want) {
			t.Errorf("MCP call params = %v, want %v", call.Params, want)
		}
		respondToolText(w, readFixture(t, "response_format_raw.json"), false)
	}))
	out, err := QueryResponseFormat(context.Background(), client, QueryResponseFormatInput{})
	if err != nil {
		t.Fatalf("QueryResponseFormat() error: %v", err)
	}
	if out.Version != "6.0.0" {
		t.Errorf("Version = %q, want 6.0.0", out.Version)
	}
	schema, _ := out.Schema.(map[string]any)
	if schema["$id"] != "https://gitlab.com/gitlab-org/orbit/knowledge-graph/schemas/query_response/v6" || schema["type"] != "object" {
		t.Errorf("Schema = %v, want the JSON Schema GitLab answered", out.Schema)
	}
}

// TestQueryResponseFormat_LLMAndRefusals verifies the llm text published
// whole, a raw answer that does not decode, a command GitLab refused and a
// format it does not take, refused before a request.
func TestQueryResponseFormat_LLMAndRefusals(t *testing.T) {
	llm := readFixture(t, "response_format_llm.txt")
	cases := []struct {
		name     string
		format   string
		answer   string
		isError  bool
		want     string
		sendsNot bool
	}{
		{name: "llm", format: "llm", answer: llm},
		{name: "a raw answer that does not decode", answer: "ResponseFormat v6.0.0", want: "decode the Orbit response format"},
		{name: "a refused command", answer: "Unknown command: get_response_format", isError: true, want: "Unknown command: get_response_format"},
		{name: "a format GitLab does not take", format: "yaml", want: "invalid response_format", sendsNot: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.sendsNot {
					t.Errorf("request sent for a format refused before one: %s %s", r.Method, r.URL.Path)
				}
				readMCP(t, r)
				respondToolText(w, tc.answer, tc.isError)
			}))
			out, err := QueryResponseFormat(context.Background(), client, QueryResponseFormatInput{ResponseFormat: tc.format})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("QueryResponseFormat() error: %v", err)
				}
				if out.FormattedText != llm || out.ResponseFormat != "llm" || out.Schema != nil {
					t.Errorf("QueryResponseFormat() = %+v, want the llm text alone", out)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("QueryResponseFormat() error = %v, want it to say %q", err, tc.want)
			}
		})
	}
}
