// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// argument_validation_test.go pins that, on the individual surface, the MCP
// SDK refuses a tool call whose arguments the tool's input schema does not
// admit, and that such a call never reaches GitLab.
//
// The dynamic and meta surfaces decode params strictly in this server's own
// code, and unit tests hold that. The individual surface leaves the check to
// the SDK: every tool is registered through mcp.AddTool with its projected
// input schema, and the SDK validates the arguments against it before the
// handler runs. Safe mode is the exception: it re-registers each write tool
// through server.AddTool with a preview handler, which the SDK hands the
// arguments unvalidated, and the preview sends nothing to GitLab. Nothing in
// this repository held the SDK's check, so a change of
// registration path, or an SDK release that stopped validating, would have
// let a value of the wrong type or an argument no schema names reach a
// handler, and through it GitLab, without any test noticing. The only way to
// see what the SDK the binary links actually does is to drive the binary.
package stdioe2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// validatedTool is the individual tool the cases call: a read whose one
// numeric argument, issue_iid, gives a value of the wrong type something to
// be wrong against.
const validatedTool = "gitlab_issue_get"

// validatedIssuePath is the request validatedTool sends for project 42 and
// issue 1, which the control call has to be seen sending.
const validatedIssuePath = "/api/v4/projects/42/issues/1"

// argumentValidationGitLab is a stand-in instance that records every request
// it receives.
//
// A refusal is only worth asserting beside evidence that nothing left the
// process, and the evidence is only worth something if the stand-in would
// have seen the request: the control call at the end of the test is what
// shows it does.
type argumentValidationGitLab struct {
	url string

	mu       sync.Mutex
	requests []string
}

// startArgumentValidationGitLab serves the startup probes and the one issue
// the control call reads, and records the method and path of every request.
func startArgumentValidationGitLab(t *testing.T) *argumentValidationGitLab {
	t.Helper()

	fake := &argumentValidationGitLab{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"version":"17.0.0","revision":"abcdef"}`)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":7,"username":"someone","name":"Some One"}`)
	})
	mux.HandleFunc("GET "+validatedIssuePath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"id":1,"iid":1,"project_id":42,"title":"stand-in issue","state":"opened","web_url":"http://example.invalid/g/proj/-/issues/1"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, r.Method+" "+r.URL.EscapedPath())
		fake.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	return fake
}

// projectRequests returns the recorded requests that name a project, which
// are the ones a call of validatedTool would send; the startup probes name
// none.
func (f *argumentValidationGitLab) projectRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var named []string
	for _, request := range f.requests {
		if _, path, _ := strings.Cut(request, " "); strings.HasPrefix(path, "/api/v4/projects/") {
			named = append(named, request)
		}
	}
	return named
}

// TestIndividualSurface_ArgumentsTheSchemaRefuses_NeverReachGitLab starts the
// real binary on the individual surface against a stand-in that records every
// request, and calls gitlab_issue_get with a value of the wrong type and with
// an argument its schema does not name. Each call is answered with a tool
// result flagged isError whose one text block is the SDK's validation error,
// naming the argument and why it was refused, and the stand-in receives no
// request naming the project for either. A call with valid arguments,
// made last, is served and reaches the stand-in, so the absence of a request
// above is the refusal and not a stand-in that sees nothing.
//
// The served schema is read first, because both cases rest on it: issue_iid
// has to be declared an integer, and the schema has to refuse properties it
// does not name, for the SDK to have anything to refuse.
func TestIndividualSurface_ArgumentsTheSchemaRefuses_NeverReachGitLab(t *testing.T) {
	fake := startArgumentValidationGitLab(t)
	env := baseEnv(fake.url)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	schema := servedInputSchema(t, s.call(t, request(1, "tools/list", "")), validatedTool)
	if got := schema["additionalProperties"]; got != false {
		t.Fatalf("%s is served with additionalProperties %v, want false: the unknown-argument case would have nothing to refuse", validatedTool, got)
	}
	properties, _ := schema["properties"].(map[string]any)
	issueIID, _ := properties["issue_iid"].(map[string]any)
	if got := issueIID["type"]; got != "integer" {
		t.Fatalf("%s serves issue_iid with type %v, want integer: the wrong-type case would have nothing to refuse", validatedTool, got)
	}

	tests := []struct {
		name      string
		arguments string
		// wantNamed is how the refusal names the argument, and wantReason
		// why it was refused, both in the words of the schema validator the
		// SDK links.
		wantNamed  string
		wantReason string
	}{
		{
			name:       "a value of the wrong type",
			arguments:  `{"project_id":"42","issue_iid":"one"}`,
			wantNamed:  "/properties/issue_iid",
			wantReason: `has type "string", want "integer"`,
		},
		{
			name:       "an argument the schema does not name",
			arguments:  `{"project_id":"42","issue_iid":1,"not_a_parameter":true}`,
			wantNamed:  `"not_a_parameter"`,
			wantReason: "unexpected additional properties",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.call(t, request(2+i, "tools/call",
				`{"name":"`+validatedTool+`","arguments":`+tt.arguments+`}`))

			text := refusedToolResultText(t, got)
			const sdkPrefix = `validating "arguments": `
			if !strings.HasPrefix(text, sdkPrefix) {
				t.Errorf("the refusal is not the SDK's argument validation (no %q prefix): %q", sdkPrefix, text)
			}
			if !strings.Contains(text, tt.wantNamed) {
				t.Errorf("the refusal does not name the argument as %s: %q", tt.wantNamed, text)
			}
			if !strings.Contains(text, tt.wantReason) {
				t.Errorf("the refusal does not say %q: %q", tt.wantReason, text)
			}
			if sent := fake.projectRequests(); len(sent) != 0 {
				t.Errorf("the refused call reached GitLab: %v", sent)
			}
		})
	}

	control := s.call(t, request(2+len(tests), "tools/call",
		`{"name":"`+validatedTool+`","arguments":{"project_id":"42","issue_iid":1}}`))
	if !served(control) {
		t.Fatalf("the control call with valid arguments was not served: %v\nstderr: %s", control, s.stderrText())
	}
	if sent := fake.projectRequests(); len(sent) != 1 || sent[0] != "GET "+validatedIssuePath {
		t.Errorf("the control call sent %v, want exactly [GET %s]: without it the absence of a request above proves nothing", sent, validatedIssuePath)
	}
}

// servedInputSchema returns the input schema a tools/list result serves for
// the named tool, failing when the listing does not carry it.
func servedInputSchema(t *testing.T, listing map[string]any, name string) map[string]any {
	t.Helper()

	result, ok := listing["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list answered no result: %v", listing["error"])
	}
	tools, _ := result["tools"].([]any)
	for _, entry := range tools {
		tool, _ := entry.(map[string]any)
		if tool["name"] != name {
			continue
		}
		schema, isMap := tool["inputSchema"].(map[string]any)
		if !isMap {
			t.Fatalf("%s is listed without an input schema: %v", name, tool)
		}
		return schema
	}
	t.Fatalf("tools/list on the individual surface does not list %s", name)
	return nil
}

// refusedToolResultText returns the one text block of a tools/call answered
// as a tool result flagged isError, and fails on any other shape: a JSON-RPC
// error, a result not flagged, a result carrying structured content, or more
// or less than one text block. The SDK answers a call its schema refuses in
// exactly that shape, and a change to any part of it is a change a client
// sees.
func refusedToolResultText(t *testing.T, msg map[string]any) string {
	t.Helper()

	if msg["error"] != nil {
		t.Fatalf("the call was answered with a JSON-RPC error, want a tool result flagged isError: %v", msg["error"])
	}
	encoded, err := json.Marshal(msg["result"])
	if err != nil {
		t.Fatalf("re-encoding the result: %v", err)
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError           bool `json:"isError"`
		StructuredContent any  `json:"structuredContent"`
	}
	if unmarshalErr := json.Unmarshal(encoded, &result); unmarshalErr != nil {
		t.Fatalf("the answer is not a tool result: %v: %s", unmarshalErr, encoded)
	}
	if !result.IsError {
		t.Fatalf("the call was answered without isError, so the schema did not refuse it: %s", encoded)
	}
	if result.StructuredContent != nil {
		t.Errorf("the refusal carries structured content: %s", encoded)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("the refusal carries %d content block(s), want one text block: %s", len(result.Content), encoded)
	}
	return result.Content[0].Text
}
