//go:build stdioe2e

// readiness_test.go drives the reported startup defect against the real binary.
//
// A stdio client has no way to observe that the process it just spawned is not
// reading its pipe yet, so it writes initialize immediately and writes it again
// when nothing comes back. The server used to spend about 1.8 seconds building
// its tool catalog before it read anything; a client giving up at 1.7 seconds
// put two handshakes in the pipe, and the SDK correctly refused the second with
// `duplicate "initialize" received`, which killed the connection. The retry
// meant to recover it is what broke it.
//
// This is the only place the defect is visible. The e2e suite drives an
// in-memory transport in the same process and builds the server directly, so it
// has no spawn, no pipe and no startup to be early to; a unit test that
// reassembled the handler chain would be testing its own copy of the thing that
// was wrong. What is under test here is a process and the order in which it
// does things.
package stdioe2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// copilotGiveUp is how long the reporting client waited before retrying its
// handshake. Measured, not guessed: the server answered at about 1.8 seconds
// and GitHub Copilot CLI had already given up at 1.7.
const copilotGiveUp = 1700 * time.Millisecond

// settleWindow is how long a case listens for anything further after the
// handshake has been answered. Long enough to catch a second response the
// server was about to write, short enough not to dominate the suite.
const settleWindow = time.Second

// promptHandshake is the ceiling this suite holds the handshake to.
//
// A backstop rather than the assertion: the real check is the ordering one
// beside it, since a duration passes on a fast machine for the wrong reason.
// It is chosen to sit between two measured numbers.
//
// The floor is Go package initialization, which happens before main runs at
// all: -version pays exactly the same and printed in 60 ms on an idle machine
// and 203 ms on a loaded one, and the handshake follows it within a couple of
// milliseconds. The failure it guards answered in 2.2 to 2.4 seconds on the
// default surface and 4.7 to 5.4 on the individual surface, measured on this
// same binary before the fix. A second sits above the floor by an order of
// magnitude and below the failure by more than a factor of two.
const promptHandshake = time.Second

// catalogReadyLine is what the server logs once registration has finished.
// Reading it off stderr is how a case establishes ORDER between the handshake
// and the slow half, rather than asserting on a stopwatch.
const catalogReadyLine = "tool catalog ready"

// legacyInitialize is a handshake exactly as a pre-2026-07-28 client sends it.
//
// Not the harness's request helper, which stamps every message with the
// per-request _meta a 2026-07-28 client carries; the SDK answers initialize
// with method-not-found when it sees that, because the revision removed the
// method. The client that reported this speaks the older revision, and the
// duplicate-handshake refusal only exists on that path.
func legacyInitialize(id int) string {
	return fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"initialize","params":`+
			`{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"stdio-e2e","version":"1"}}}`,
		id,
	)
}

// legacyRequest is any other call on that same pre-2026-07-28 session.
func legacyRequest(id int, method, params string) string {
	if params == "" {
		params = "{}"
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)
}

// lines starts the one goroutine that reads this session's stdout and returns
// the channel it feeds.
//
// The harness's own readMessage abandons its reader goroutine when it times
// out, which is right for a case that treats silence as a failure and wrong for
// one whose subject IS the silence: what happens next here depends on nothing
// having been said, and every message that eventually arrives has to be
// accounted for. A session uses one or the other, never both.
//
// Nothing on this goroutine touches *testing.T. A line that will not parse is
// passed on as data and judged on the test goroutine, where a failure can be
// reported properly.
func (s *session) lines(t *testing.T) <-chan map[string]any {
	t.Helper()

	ctx := t.Context()
	out := make(chan map[string]any, 64)
	go func() {
		defer close(out)
		for {
			line, err := s.stdout.ReadString('\n')
			if strings.TrimSpace(line) != "" {
				decoded := map[string]any{}
				if unmarshalErr := json.Unmarshal([]byte(line), &decoded); unmarshalErr != nil {
					decoded = map[string]any{unparsedKey: line}
				}
				select {
				case out <- decoded:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return out
}

// unparsedKey marks a stdout line that was not JSON, so the test goroutine can
// fail on it with the line in hand.
const unparsedKey = "__unparsed"

// nextMessage returns the next thing the server wrote, or false if it wrote
// nothing within the window.
func nextMessage(messages <-chan map[string]any, within time.Duration) (map[string]any, bool) {
	timer := time.NewTimer(within)
	defer timer.Stop()
	select {
	case msg, open := <-messages:
		return msg, open
	case <-timer.C:
		return nil, false
	}
}

// nextResponse returns the next message carrying an id, skipping notifications.
//
// Registration adds tools to a server that is already connected, and the SDK
// tells a connected session so: notifications/tools/list_changed can arrive
// before, after, or in between the responses a case is waiting for. That is the
// design working, not noise to be suppressed, so it is stepped over rather than
// treated as an answer.
func nextResponse(messages <-chan map[string]any, within time.Duration) (map[string]any, bool) {
	deadline := time.Now().Add(within)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, false
		}
		msg, ok := nextMessage(messages, remaining)
		if !ok {
			return nil, false
		}
		if _, isResponse := msg["id"]; isResponse {
			return msg, true
		}
	}
}

// TestInitialize_ClientRetriesAfterSilence_ServerNeverRefusesADuplicate
// reproduces the reported failure end to end.
//
// It behaves the way the client did: write the handshake, wait as long as
// GitHub Copilot CLI waits, and write it again if nothing has come back. On the
// broken server nothing comes back in time, the retry goes into the same pipe,
// and both handshakes are read at once: the first answered, the second refused
// with `duplicate "initialize" received`, which fails the connection. On a
// server that answers from a connected transport the retry never happens, so
// there is exactly one response and it is not an error.
//
// The retry is deliberately conditional rather than unconditional. A test that
// always wrote two handshakes would be asserting that a duplicate is tolerated,
// which is not the fix and not desirable: the SDK's refusal is correct. What
// must not happen is a client being driven to send one.
func TestInitialize_ClientRetriesAfterSilence_ServerNeverRefusesADuplicate(t *testing.T) {
	gitlab := startFakeGitLab(t)
	s := startSession(t, baseEnv(gitlab.URL))
	messages := s.lines(t)

	start := time.Now()
	s.send(t, legacyInitialize(1))

	first, answered := nextResponse(messages, copilotGiveUp)
	retried := !answered
	if retried {
		s.send(t, legacyInitialize(2))
		first, answered = nextResponse(messages, 30*time.Second)
		if !answered {
			t.Fatalf("the server answered no handshake at all\nstderr: %s", s.stderrText())
		}
	}
	t.Logf("handshake answered after %s (client had to retry: %v)", time.Since(start).Round(time.Millisecond), retried)

	responses := []map[string]any{first}
	for {
		msg, more := nextMessage(messages, settleWindow)
		if !more {
			break
		}
		if line, unparsed := msg[unparsedKey]; unparsed {
			t.Errorf("stdout carried a line that is not JSON: %v", line)
			continue
		}
		if _, isResponse := msg["id"]; isResponse {
			responses = append(responses, msg)
		}
	}

	for i, msg := range responses {
		failure, isError := msg["error"]
		if !isError {
			continue
		}
		rendered := fmt.Sprint(failure)
		if strings.Contains(rendered, `duplicate "initialize" received`) {
			t.Errorf("the server refused the client's retried handshake, which is the reported failure: %s", rendered)
			continue
		}
		t.Errorf("handshake response %d is an error: %s", i+1, rendered)
	}
	if len(responses) != 1 {
		t.Errorf("the client got %d handshake responses, want 1; a second one means it was made to ask twice", len(responses))
	}
	if retried {
		t.Errorf("the server said nothing for %s, so a client gave up and retried its handshake; that is the defect", copilotGiveUp)
	}
}

// TestInitialize_WhileTheCatalogIsStillBuilding_IsAnsweredFirst pins the
// ordering the fix is actually about.
//
// The claim is not "startup is fast", since making it faster only moves the
// threshold and a slower machine or a larger catalog puts the failure back.
// The claim is that the handshake no longer waits on the catalog at all, and
// the honest way to state that is an order: the answer arrives while the server
// has not yet logged that registration finished. The duration below is a
// backstop for the case where the ordering somehow holds by accident, chosen so
// a regression to the 1.8 seconds this replaced cannot pass it.
func TestInitialize_WhileTheCatalogIsStillBuilding_IsAnsweredFirst(t *testing.T) {
	gitlab := startFakeGitLab(t)
	// The individual surface registers the largest catalog this server has, so
	// the slow half is at its slowest and the ordering claim is at its
	// strongest.
	env := baseEnv(gitlab.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)
	messages := s.lines(t)

	start := time.Now()
	s.send(t, legacyInitialize(1))

	response, answered := nextResponse(messages, 30*time.Second)
	elapsed := time.Since(start)
	if !answered {
		t.Fatalf("the handshake was never answered\nstderr: %s", s.stderrText())
	}
	logsAtAnswer := s.stderrText()
	t.Logf("handshake answered after %s", elapsed.Round(time.Millisecond))

	if failure, isError := response["error"]; isError {
		t.Fatalf("the handshake failed: %v", failure)
	}
	if strings.Contains(logsAtAnswer, catalogReadyLine) {
		t.Errorf("the catalog was already registered when the handshake was answered, so nothing was proved about the order:\n%s", logsAtAnswer)
	}
	if elapsed > promptHandshake {
		t.Errorf("the handshake took %s, over the %s ceiling; the defect this replaced answered at about 1.8s",
			elapsed.Round(time.Millisecond), promptHandshake)
	}

	// The other half of the claim: the slow work still happens, and the server
	// says so. Without this a server that answered promptly by registering
	// nothing at all would pass.
	awaitStderr(t, s, catalogReadyLine, 60*time.Second)
}

// TestToolsList_IssuedDuringStartup_ReturnsTheWholeCatalog verifies that the
// wait a client may experience is a wait and never a wrong answer.
//
// Answering an early tools/list with an empty catalog would be the worse
// failure: a client that does not act on notifications/tools/list_changed
// concludes the server has no tools, and reports it as the server having no
// tools rather than as a race. So the request is written into the same pipe as
// the handshake, before the server has read either, and the answer must be the
// finished catalog.
func TestToolsList_IssuedDuringStartup_ReturnsTheWholeCatalog(t *testing.T) {
	tests := []struct {
		name      string
		surface   string
		minTools  int
		mustCarry []string
	}{
		{
			name:      "dynamic surface",
			surface:   "dynamic",
			minTools:  2,
			mustCarry: []string{"gitlab_find_action", "gitlab_execute_action"},
		},
		{
			name:      "individual surface",
			surface:   "individual",
			minTools:  500,
			mustCarry: []string{"gitlab_project_get", "gitlab_issue_list"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := listToolsDuringStartup(t, tt.surface)
			if len(names) < tt.minTools {
				t.Errorf("tools/list returned %d tools, want at least %d; an early caller was served a catalog that was not finished",
					len(names), tt.minTools)
			}
			for _, want := range tt.mustCarry {
				if !contains(names, want) {
					t.Errorf("tools/list did not carry %q, so the catalog it answered from was incomplete", want)
				}
			}
		})
	}
}

// listToolsDuringStartup starts a server on the given surface, writes the
// handshake and a tools/list into the pipe before either has been read, and
// returns the tool names that came back.
//
// Both messages go in before the server reads anything, so tools/list is
// unambiguously queued while the catalog is still being built. The SDK handles
// initialize synchronously and only then dequeues the next message, so the
// session is initialized by the time tools/list is dispatched.
func listToolsDuringStartup(t *testing.T, surface string) []string {
	t.Helper()

	gitlab := startFakeGitLab(t)
	env := baseEnv(gitlab.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = surface
	s := startSession(t, env)
	messages := s.lines(t)

	s.send(t, legacyInitialize(1))
	s.send(t, legacyRequest(2, "tools/list", ""))
	if logs := s.stderrText(); strings.Contains(logs, catalogReadyLine) {
		t.Fatalf("the catalog was registered before the requests were even written, so the case proves nothing:\n%s", logs)
	}

	handshake, answered := nextResponse(messages, 30*time.Second)
	if !answered {
		t.Fatalf("the handshake was never answered\nstderr: %s", s.stderrText())
	}
	if failure, isError := handshake["error"]; isError {
		t.Fatalf("the handshake failed: %v", failure)
	}

	listed, answered := nextResponse(messages, 60*time.Second)
	if !answered {
		t.Fatalf("tools/list was never answered\nstderr: %s", s.stderrText())
	}
	if failure, isError := listed["error"]; isError {
		t.Fatalf("tools/list failed: %v", failure)
	}
	return toolNames(t, listed)
}

// awaitStderr waits for a line to appear in the server's log.
//
// Polled rather than streamed because the harness collects stderr into a
// buffer: what is being asked is whether something has been logged yet, and
// that question only has an answer at the moment it is asked.
func awaitStderr(t *testing.T, s *session, want string, within time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), within)
	defer cancel()
	for {
		if strings.Contains(s.stderrText(), want) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%q never appeared in the server's log within %s:\n%s", want, within, s.stderrText())
			return
		case <-time.After(25 * time.Millisecond):
		}
	}
}
