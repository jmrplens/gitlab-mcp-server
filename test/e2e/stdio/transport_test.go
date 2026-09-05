//go:build stdioe2e

package stdioe2e

import (
	"encoding/json"
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
		t.Run(msg, func(t *testing.T) {
			got := s.call(t, msg)
			// readMessage already fails on anything that is not JSON. What is left
			// to check is that it is JSON-RPC, and that it answers what was asked.
			if got["jsonrpc"] != "2.0" {
				t.Errorf("message %d is not JSON-RPC 2.0: %v", i+1, got)
			}
			if got["id"] == nil {
				t.Errorf("message %d carries no id, so a client cannot match it to a request: %v", i+1, got)
			}
		})
	}

	// The startup line is written before the server answers anything, so its
	// absence here would mean logging is off or went somewhere else; waited
	// for, because the harness copies stderr on a goroutine of its own.
	s.waitForStderr(t, "starting MCP server", 5*time.Second)
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
	// readMessage would have failed above if it had. The debug lines are
	// waited for rather than read, since the harness copies stderr on a
	// goroutine of its own.
	s.waitForStderr(t, "DEBUG", 5*time.Second)
}

// TestStderr_DefaultLevelIsNotAllSessionChatter pins what an operator actually
// sees on the default log stream.
//
// The SDK logs nothing unless it is handed a Logger, and this server hands it
// one, so the volume is this repository's. What it produced was one connect,
// one log-level and one disconnect line per session, plus a run-start line, and
// nothing else: measured on HTTP, where a stateless POST is a session, twenty-
// four calls gave ninety-six records and all ninety-six were those four
// messages. Not most. All. The stream looked busy and said nothing, and
// LOG_LEVEL was no way out, since raising it silenced this server's own signal
// along with the noise.
//
// The check is on the wire rather than on the handler because the handler had
// no way of being reached wrongly: a Logger left unwrapped at the wiring point
// would pass every unit test in cmd/server and change nothing about them.
func TestStderr_DefaultLevelIsNotAllSessionChatter(t *testing.T) {
	s := startSession(t, baseEnv(startFakeGitLab(t).URL))

	// Enough calls that per-session chatter would dominate if it were emitted.
	for id := 1; id <= 6; id++ {
		if got := s.call(t, request(id, "tools/list", "")); got["error"] != nil {
			t.Fatalf("tools/list failed: %v", got["error"])
		}
	}

	// Anchored on a line the server writes after the session is up, so the
	// absences asserted below are absences in what was logged rather than in
	// what the harness had copied so far.
	logs := s.waitForStderr(t, "starting MCP server", 5*time.Second)
	for _, chatter := range []string{
		"server session connected",
		"server session disconnected",
		"client log level set",
		"server connecting",
	} {
		t.Run(chatter, func(t *testing.T) {
			if strings.Contains(logs, chatter) {
				t.Errorf("%q is on the default log stream; it is per-session and carries nothing:\n%s", chatter, logs)
			}
		})
	}

	// The other half of the claim: this is a demotion, not a silencing. The
	// server's own startup lines must still be there.
	if !strings.Contains(logs, "gitlab connection verified") {
		t.Errorf("the server's own startup signal is missing from the default stream:\n%s", logs)
	}
}

// TestStderr_LogLinesKeepTheirSeverity pins that a record's severity survives
// the attributes traveling with it.
//
// "client log level set" carries an attribute the SDK calls level, and level is
// also what slog's JSON handler names the record's own severity. Neither side
// deduplicates, so the line went out with two of them and any parser keeping
// the last read the severity as the empty string. On the default HTTP surface
// that was one steady-state line in four arriving at an aggregator with no
// severity at all.
//
// Asserted by parsing, because the raw text looks correct: both members are
// present, and only a parser has to choose between them.
func TestStderr_LogLinesKeepTheirSeverity(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["LOG_LEVEL"] = "debug"
	s := startSession(t, env)

	if got := s.call(t, request(1, "tools/list", "")); got["error"] != nil {
		t.Fatalf("tools/list failed: %v", got["error"])
	}

	for line := range strings.SplitSeq(strings.TrimSpace(s.stderrText()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("a log line is not JSON: %q (%v)", line, err)
		}
		if level, _ := record["level"].(string); level == "" {
			t.Errorf("a log line reached a parser with no severity: %s", line)
		}
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
		t.Run(want, func(t *testing.T) {
			if _, present := caps[want]; !present {
				t.Errorf("the handshake does not declare %q, which this surface registers", want)
			}
		})
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
				assertRefusedWith(t, s, tt.wantCode)
			}

			// The session has to still be usable, which is the half the SDK
			// gets wrong on its own.
			assertStillServing(t, s)
		})
	}
}

// assertRefusedWith checks that the server's next message is a refusal carrying
// the given JSON-RPC error code.
func assertRefusedWith(t *testing.T, s *session, want int) {
	t.Helper()

	got := s.readMessage(t, 30*time.Second)
	code, ok := errorCode(got)
	if !ok {
		t.Fatalf("the input was not refused: %v", got)
	}
	if code != want {
		t.Errorf("error code = %d, want %d: %v", code, want, got)
	}
}

// assertStillServing checks that the session answers a fresh call.
//
// It reads past anything the server is still saying about the previous input,
// matching on the id rather than on arrival order, so a refusal that turns up
// late is not mistaken for the answer.
func assertStillServing(t *testing.T, s *session) {
	t.Helper()

	s.send(t, request(2, "tools/list", ""))
	for {
		got := s.readMessage(t, 30*time.Second)
		if answeredID(got) != 2 {
			continue
		}
		if got["error"] != nil {
			t.Fatalf("the session survived but stopped working: %v", got["error"])
		}
		return
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
	env["GITLAB_MCP_READ_ONLY"] = "true"
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

// TestTransportAuto_WithAPipeOnStdin_SpeaksStdio pins the half of the
// transport inference a container image depends on, against a real process.
//
// The image's CMD is --transport auto, so what a client gets when it runs
// `docker run -i` is decided by what auto reads off file descriptor 0. An MCP
// client connects a pipe there, which is exactly what exec.Cmd's StdinPipe
// gives this session, so the shape under test is the shape that ships. The
// unit tests in cmd/server/transport_test.go cover the decision against every
// kind of stdin, including the /dev/null that means HTTP; what they cannot
// cover is the decision actually reaching the transport the process then
// serves, since they never start one.
//
// Both halves are asserted, because either alone would pass while the feature
// was broken: the log line says what was inferred, and the JSON-RPC answers
// coming back down stdout say the server went on to serve it. An HTTP listener
// answers nothing here whatever it logged.
func TestTransportAuto_WithAPipeOnStdin_SpeaksStdio(t *testing.T) {
	gitlab := startFakeGitLab(t)
	s := startSessionWithArgs(t, baseEnv(gitlab.URL), "--transport", "auto")

	// The same path TestStdout_CarriesNothingButJSONRPC walks, for the same
	// reason: the handshake, the catalog and a call that reaches GitLab are
	// where a stray write to stdout would land, and readMessage fails on
	// anything that is not JSON.
	for _, tc := range []struct {
		name    string
		request string
	}{
		// The legacy handshake, spelled raw: initialize is removed in
		// 2026-07-28, so the per-request _meta that request() attaches would
		// make the server refuse it.
		{name: "initialize", request: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`},
		{name: "tools/list", request: request(2, "tools/list", "")},
		{name: "tools/call", request: request(3, "tools/call", `{"name":"gitlab_execute_action","arguments":{"action":"user.get_current"}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := s.call(t, tc.request)
			if got["jsonrpc"] != "2.0" {
				t.Errorf("%s was not answered with JSON-RPC 2.0: %v", tc.name, got)
			}
			if got["error"] != nil {
				t.Errorf("%s failed: %v", tc.name, got["error"])
			}
		})
	}

	inferred := awaitLogRecord(t, s, "transport inferred from stdin", 10*time.Second)
	if got, _ := inferred["transport"].(string); got != "stdio" {
		t.Errorf("the transport was inferred as %q, want stdio: %v", got, inferred)
	}
	// The reason is asserted too: inferring stdio from a stdin it failed to
	// examine would satisfy the line above while meaning the inference never
	// looked at anything.
	if reason, _ := inferred["reason"].(string); !strings.Contains(reason, "pipe") {
		t.Errorf("stdio was inferred for reason %q, want the pipe the client connected: %v", reason, inferred)
	}

	if !s.alive() {
		t.Error("the server exited during the session")
	}
}

// awaitLogRecord polls stderr until a JSON log record with the given msg
// appears, and fails the test when none does within the window.
//
// Polling rather than reading once, because stderr is copied into the session
// by a goroutine of its own: a record the server has already written is not
// necessarily in that buffer at the moment a later request's answer arrives on
// stdout, so a single read raced the drain and could fail while the feature
// worked. An intermittent failure in a test that pins a startup property is
// worse than no test at all, since what it teaches is to re-run.
func awaitLogRecord(t *testing.T, s *session, msg string, within time.Duration) map[string]any {
	t.Helper()

	deadline := time.Now().Add(within)
	for {
		logs := s.stderrText()
		if record, ok := findLogRecord(logs, msg); ok {
			return record
		}
		if time.Now().After(deadline) {
			t.Fatalf("no log record with msg %q was written within %s:\n%s", msg, within, logs)
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// findLogRecord returns the first JSON log record in the given stderr text
// whose msg matches, and reports whether there was one.
func findLogRecord(logs, msg string) (map[string]any, bool) {
	for line := range strings.SplitSeq(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if got, _ := record["msg"].(string); got == msg {
			return record, true
		}
	}
	return nil, false
}
