//go:build stdioe2e

package stdioe2e

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// TestStdout_CarriesNothingButJSONRPC pins the property every stdio client
// depends on and no unit test can observe.
//
// On this transport stdout is the protocol. One stray fmt.Println, one library
// that logs to the wrong stream, one banner behind a flag, and every client
// fails to parse the stream — usually with an error that names the client
// rather than this server. There is nothing in the type system stopping it, and
// nothing in the test suite noticed until now: the e2e suite drives an
// in-memory transport with no streams at all, so this could only have been
// caught by hand.
//
// The npm release pipeline checks the same property, which is the tell that it
// matters and that it was being checked in the wrong place: a defect found
// while publishing is found after the code is already tagged.
func TestStdout_CarriesNothingButJSONRPC(t *testing.T) {
	gitlab := startFakeGitLab(t)
	s := startSession(t, baseEnv(gitlab.URL))

	// A handshake and a call that reaches GitLab, which is where a stray write
	// is most likely: startup probes, the client wrapper, and tool execution
	// all run on this path.
	for i, msg := range []string{
		request(1, "initialize", `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"}}`),
		request(2, "tools/list", ""),
		request(3, "tools/call", `{"name":"gitlab_execute_action","arguments":{"action":"user.get_current"}}`),
	} {
		got := s.call(t, msg)
		// readMessage already fails on anything that is not JSON. What is left
		// to check is that it is JSON-RPC, and that it answers what was asked.
		if got["jsonrpc"] != "2.0" {
			t.Errorf("message %d is not JSON-RPC 2.0: %v", i+1, got)
		}
		if got["id"] == nil {
			t.Errorf("message %d carries no id, so a client cannot match it to a request: %v", i+1, got)
		}
	}

	if logs := s.stderrText(); logs == "" {
		t.Error("nothing was logged to stderr at LOG_LEVEL=info; either logging is off or it went somewhere else")
	}
}

// TestStderr_TakesTheLogsAndStdoutDoesNot checks the other half of the same
// rule, at a log level noisy enough to catch a misrouted writer.
func TestStderr_TakesTheLogsAndStdoutDoesNot(t *testing.T) {
	gitlab := startFakeGitLab(t)
	env := baseEnv(gitlab.URL)
	env["LOG_LEVEL"] = "debug"
	s := startSession(t, env)

	got := s.call(t, request(1, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("tools/list failed: %v", got["error"])
	}

	// Debug logging produces a lot; none of it may appear on stdout, and
	// readMessage would have failed above if it had.
	logs := s.stderrText()
	if !strings.Contains(logs, "level=DEBUG") && !strings.Contains(logs, "DEBUG") {
		t.Errorf("LOG_LEVEL=debug produced no debug lines on stderr:\n%s", logs)
	}
}

// TestHandshake_AnswersTheLegacyInitialize pins the older negotiation, which is
// what every shipping client uses today.
func TestHandshake_AnswersTheLegacyInitialize(t *testing.T) {
	gitlab := startFakeGitLab(t)
	s := startSession(t, baseEnv(gitlab.URL))

	got := s.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
	if got["error"] != nil {
		t.Fatalf("initialize failed: %v", got["error"])
	}

	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize returned no result: %v", got)
	}
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("the handshake declared no capabilities: %v", result)
	}
	// What is declared here is what a client will go on to use, so a
	// capability that is advertised and not served is worse than one that is
	// missing. These three are registered on the default surface.
	for _, want := range []string{"tools", "resources", "prompts"} {
		if _, present := caps[want]; !present {
			t.Errorf("the handshake does not declare %q, which this surface registers", want)
		}
	}
	if _, present := caps["logging"]; present {
		t.Error("the handshake declares logging, which this server does not implement")
	}
}

// TestIdleSession_IsNotClosedByTheServer pins the defect that made a
// 2026-07-28 stdio session die on its own.
//
// This server used to ask the SDK for a 30-second keepalive, which closes the
// session on the first unanswered ping. ping is removed in 2026-07-28, so a
// conformant client of that revision cannot answer, and the process exited 45
// seconds into an idle session — while a unit test asserted the ping ought to
// be there. The keepalive is opt-in and defaults to off, so this was a setting
// of ours rather than anything the SDK did unbidden.
//
// Sixty seconds is deliberate: it has to outlast the ping interval and the
// failure threshold that followed it, or the test passes by finishing early.
func TestIdleSession_IsNotClosedByTheServer(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out a keepalive interval")
	}

	gitlab := startFakeGitLab(t)
	s := startSession(t, baseEnv(gitlab.URL))

	if got := s.call(t, request(1, "tools/list", "")); got["error"] != nil {
		t.Fatalf("tools/list failed: %v", got["error"])
	}

	time.Sleep(60 * time.Second)

	if !s.alive() {
		t.Fatalf("the server exited while idle:\n%s", s.stderrText())
	}
	// Still serving, and nothing unsolicited arrived in between: the next read
	// must be this call's own answer.
	got := s.call(t, request(2, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("the session stopped working while idle: %v", got["error"])
	}
	if answeredID(got) != 2 {
		t.Errorf("the next message on stdout was not the answer to request 2 but %v; the server sent something unprompted", got)
	}
}

// TestElicitingCall_DoesNotKillTheProcess is the stdio half of the crash that
// took the server down on any tool call that could elicit.
//
// The HTTP module pins its own transport's version of this. Both are worth
// having: the same code was reachable from either, and stdio is where a user
// running this locally would have met it.
func TestElicitingCall_DoesNotKillTheProcess(t *testing.T) {
	gitlab := startFakeGitLab(t)
	s := startSession(t, baseEnv(gitlab.URL))

	// No initialize first: the SDK then synthesizes InitializeParams with a nil
	// Capabilities pointer, which is the shape that panicked.
	got := s.call(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_execute_action","arguments":{"action":"interactive.issue_create","params":{"project_id":"42"}}}}`)
	if got["jsonrpc"] != "2.0" {
		t.Fatalf("the eliciting call was not answered with JSON-RPC: %v", got)
	}

	// Whether it prompts or refuses is settled elsewhere; that the process is
	// still serving is the assertion.
	after := s.call(t, request(2, "tools/list", ""))
	if after["error"] != nil {
		t.Fatalf("the server stopped serving after an eliciting call: %v", after["error"])
	}
	if strings.Contains(s.stderrText(), "panic:") {
		t.Errorf("a panic escaped to the log:\n%s", s.stderrText())
	}
}

// TestMalformedInput_IsAnsweredAndTheSessionSurvives pins how the server
// answers a client that sends something it cannot read.
//
// The SDK's read loop ends the session on any error from its reader, so a
// message that fails to parse is handled exactly like a closed pipe: the
// process exits, having written nothing, and the client sees EOF on a stream it
// could still write to. One stray byte cost a client its whole session and its
// accumulated context.
//
// JSON-RPC 2.0 has codes for both shapes of this, and the framing is one
// message per line, so the next line is an independent message. serveStdio
// therefore filters stdin ahead of the SDK: an unreadable line is answered and
// dropped, and the session goes on. The underlying SDK behavior is unchanged
// and recorded in docs/development/upstream-bugs.md.
//
// Every case checks the same two things — the answer carries the right code,
// and the session still works afterwards — because either alone would miss the
// point: a correct refusal on a dead session is no better than silence.
func TestMalformedInput_IsAnsweredAndTheSessionSurvives(t *testing.T) {
	gitlab := startFakeGitLab(t)

	tests := []struct {
		name string
		body string
		// wantCode is the JSON-RPC error code the sender must receive, or 0
		// when the input is dropped without an answer.
		wantCode int
	}{
		{name: "an unknown method", body: `{"jsonrpc":"2.0","id":1,"method":"no/such/method","params":{}}`, wantCode: -32601},
		{name: "a tool that does not exist", body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"not_a_tool","arguments":{}}}`, wantCode: -32602},
		{name: "not JSON at all", body: `{not json`, wantCode: -32700},
		{name: "a truncated message", body: `{"jsonrpc":"2.0","id":1,"meth`, wantCode: -32700},
		{name: "JSON carrying no jsonrpc version", body: `{"hello":"world"}`, wantCode: -32600},
		// Valid JSON of the wrong shape: the server parsed it, so -32700 would
		// send the client looking for a syntax error it does not have.
		{name: "a JSON array, which is not a message", body: `["jsonrpc","2.0"]`, wantCode: -32600},
		{name: "a blank line is framing, not a message", body: ``, wantCode: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := startSession(t, baseEnv(gitlab.URL))
			// A completed call first, so the session is known good and the
			// outcome below is attributable to this input alone.
			if got := s.call(t, request(1, "tools/list", "")); got["error"] != nil {
				t.Fatalf("the session was not healthy to begin with: %v", got["error"])
			}

			s.send(t, tt.body)

			if tt.wantCode != 0 {
				got := s.readMessage(t, 30*time.Second)
				code, ok := errorCode(got)
				if !ok {
					t.Fatalf("the input was not refused: %v", got)
				}
				if code != tt.wantCode {
					t.Errorf("error code = %d, want %d: %v", code, tt.wantCode, got)
				}
			}

			// The session has to still be usable, which is the half the SDK
			// gets wrong on its own.
			s.send(t, request(2, "tools/list", ""))
			for {
				got := s.readMessage(t, 30*time.Second)
				if answeredID(got) == 2 {
					if got["error"] != nil {
						t.Fatalf("the session survived but stopped working: %v", got["error"])
					}
					return
				}
			}
		})
	}
}

// errorCode returns the JSON-RPC error code of a message, if it carries one.
func errorCode(message map[string]any) (int, bool) {
	raw, ok := message["error"].(map[string]any)
	if !ok {
		return 0, false
	}
	code, ok := raw["code"].(float64)
	if !ok {
		return 0, false
	}
	return int(code), true
}

// TestToolSurface_IsReadFromTheEnvironment pins the configuration path stdio
// actually uses.
//
// HTTP mode takes flags and has tests for them; stdio takes environment
// variables and had none. The surfaces differ by three orders of magnitude in
// tool count, so reading the wrong one is not a subtle failure — but nothing
// checked that the variable was read at all.
func TestToolSurface_IsReadFromTheEnvironment(t *testing.T) {
	gitlab := startFakeGitLab(t)

	tests := []struct {
		name    string
		surface string
		// The dynamic surface registers exactly two tools; the others register
		// hundreds. Bounds rather than counts, so the catalog can grow.
		wantAtMost  int
		wantAtLeast int
		wantTool    string
	}{
		{name: "dynamic is the default", surface: "", wantAtMost: 2, wantAtLeast: 2, wantTool: "gitlab_find_action"},
		{name: "dynamic named explicitly", surface: "dynamic", wantAtMost: 2, wantAtLeast: 2, wantTool: "gitlab_find_action"},
		{name: "meta", surface: "meta", wantAtLeast: 20, wantAtMost: 100, wantTool: "gitlab_issue"},
		{name: "individual", surface: "individual", wantAtLeast: 500, wantAtMost: 5000, wantTool: "gitlab_issue_list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := baseEnv(gitlab.URL)
			if tt.surface != "" {
				env["TOOL_SURFACE"] = tt.surface
			}
			s := startSession(t, env)

			got := s.call(t, request(1, "tools/list", ""))
			if got["error"] != nil {
				t.Fatalf("tools/list failed: %v", got["error"])
			}
			names := toolNames(t, got)

			if len(names) < tt.wantAtLeast || len(names) > tt.wantAtMost {
				t.Errorf("surface %q registered %d tools, want between %d and %d",
					tt.surface, len(names), tt.wantAtLeast, tt.wantAtMost)
			}
			if !contains(names, tt.wantTool) {
				t.Errorf("surface %q does not register %q", tt.surface, tt.wantTool)
			}
		})
	}
}

// TestReadOnly_RemovesMutationFromTheEnvironment pins the other
// environment-driven switch a deployment relies on.
func TestReadOnly_RemovesMutationFromTheEnvironment(t *testing.T) {
	gitlab := startFakeGitLab(t)

	env := baseEnv(gitlab.URL)
	env["TOOL_SURFACE"] = "individual"
	env["GITLAB_READ_ONLY"] = "true"
	s := startSession(t, env)

	got := s.call(t, request(1, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("tools/list failed: %v", got["error"])
	}
	names := toolNames(t, got)

	if contains(names, "gitlab_issue_delete") {
		t.Error("read-only mode still registers gitlab_issue_delete")
	}
	if !contains(names, "gitlab_issue_list") {
		t.Error("read-only mode removed gitlab_issue_list; reads must keep working")
	}
}

// toolNames extracts the tool names from a tools/list result.
func toolNames(t *testing.T, message map[string]any) []string {
	t.Helper()

	result, ok := message["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", message)
	}
	raw, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("no tools in %v", result)
	}
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		tool, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		if name, isString := tool["name"].(string); isString {
			names = append(names, name)
		}
	}
	return names
}

// answeredID returns the request id a message answers, or -1 when it carries
// none.
func answeredID(message map[string]any) int {
	id, ok := message["id"].(float64)
	if !ok {
		return -1
	}
	return int(id)
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// TestStdio_AliveReportsADeadProcess covers the predicate another test trusts,
// and it exists because that predicate could not fail.
//
// alive() read cmd.ProcessState, which only Cmd.Wait populates, while this
// harness reaps through Process.Wait deliberately: Cmd.Wait closes the output
// pipes the moment it sees the exit, and a read still in flight would fail
// rather than return what it had. So ProcessState stayed nil and alive()
// returned true for a process that had been dead for a minute.
//
// Its caller asserts the server survived sixty idle seconds, which is the
// regression for a keepalive ping that closed idle clients' sessions in a
// shipped build. It was passing vacuously, and what actually protected it was
// the request that follows.
func TestStdio_AliveReportsADeadProcess(t *testing.T) {
	gitlab := startFakeGitLab(t)
	s := startSession(t, baseEnv(gitlab.URL))

	if got := s.call(t, request(1, "tools/list", "")); got["error"] != nil {
		t.Fatalf("tools/list failed: %v", got["error"])
	}
	if !s.alive() {
		t.Fatal("the server is reported dead while it is answering requests")
	}

	if !s.terminate(t, 20*time.Second) {
		t.Fatal("the server did not exit when asked")
	}

	if s.alive() {
		t.Error("alive() still reports the process as running after it exited, so every assertion resting on it is vacuous")
	}
}
