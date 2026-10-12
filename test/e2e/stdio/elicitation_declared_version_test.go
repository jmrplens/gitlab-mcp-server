//go:build stdioe2e

// elicitation_declared_version_test.go pins which protocol revision decides how
// a guided flow asks its questions on stdio, which is row 11 of
// docs/development/upstream-bugs.md and issue 1319.
//
// A client that sends initialize asking for 2026-07-28 is negotiated down to
// 2025-11-25, since 2026-07-28 deprecates initialize, and the go-sdk this
// module is built against still reads the version the client asked for
// (go-sdk#1258). Both the SDK's multi round-trip check and this server's
// internal/elicitation.FlowFromRequest read that declared value, so the flow
// answers the 2026-07-28 shape, resultType input_required with its
// inputRequests, on a session whose protocol carries elicitation as a
// server-initiated elicitation/create request. The server agrees with the SDK
// on purpose (row 11's workaround: none taken), and go-sdk#1266 is the fix.
//
// The defect shows only against the real SDK and the real binary, so the pin
// lives here. It fails on the go-sdk release that carries go-sdk#1266, with a
// message naming what to update.
package stdioe2e

import (
	"testing"
	"time"
)

// declaredVersionFix is what a failure of the pin asks the pull request that
// bumps go-sdk to do.
const declaredVersionFix = "go-sdk now selects the multi round-trip flow by the negotiated version (go-sdk#1266). " +
	"Update row 11 of docs/development/upstream-bugs.md and its section, the comment on the version check in " +
	"internal/elicitation/flow.go, the elicitation page's \"Which protocol version decides?\" section " +
	"(site/src/content/docs/capabilities/elicitation.mdx and its Spanish twin), and turn this " +
	"test into the assertion of the legacy flow, in this same pull request."

// TestElicitation_InitializeAskingForNewRevision_IsServedTheMultiRoundTripShape
// pins the current answer: initialize asking for 2026-07-28 is answered
// 2025-11-25, and a guided flow called on that session without any
// per-request protocol _meta answers resultType input_required with a request
// for its first field, and sends no elicitation/create.
func TestElicitation_InitializeAskingForNewRevision_IsServedTheMultiRoundTripShape(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "meta"
	s := startSession(t, env)

	initialized := s.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28",`+
		`"capabilities":{"elicitation":{"form":{}}},"clientInfo":{"name":"stdio-e2e","version":"1"}}}`)
	result, _ := initialized["result"].(map[string]any)
	if negotiated := result["protocolVersion"]; negotiated != "2025-11-25" {
		// Not declaredVersionFix: that names the upstream fix, and this
		// failure says nothing about whether it landed.
		t.Fatalf("initialize asking for 2026-07-28 negotiated %v, want 2025-11-25: the SDK's negotiation changed, and with it the premise of row 11 of docs/development/upstream-bugs.md, "+
			"that such a client is negotiated down to 2025-11-25 while the SDK reads the version it asked for. Reread row 11 and its section against the new negotiation before changing this test.",
			negotiated)
	}
	s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	const callID = 2
	s.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_interactive_issue_create","arguments":{"project_id":"42"}}}`)
	var answer map[string]any
	deadline := time.Now().Add(callTimeout)
	for answer == nil && time.Now().Before(deadline) {
		got := s.readMessage(t, callTimeout)
		switch method, _ := got["method"].(string); {
		case method == "elicitation/create":
			t.Fatalf("the flow asked through a server-initiated elicitation/create on the 2025-11-25 session (%v), which is the legacy flow the negotiated version calls for. %s",
				got, declaredVersionFix)
		case sameID(got["id"], callID):
			answer = got
		}
	}
	if answer == nil {
		t.Fatalf("the guided flow was not answered within %s:\n%s", callTimeout, s.stderrText())
	}
	if answer["error"] != nil {
		t.Fatalf("the guided flow answered an error, want its first question: %v", answer["error"])
	}
	called, _ := answer["result"].(map[string]any)
	if called["resultType"] != "input_required" {
		t.Fatalf("the guided flow answered resultType %v on the 2025-11-25 session, want input_required while go-sdk reads the declared version: %v. %s",
			called["resultType"], called, declaredVersionFix)
	}
	inputs, _ := called["inputRequests"].(map[string]any)
	if _, asked := inputs["title"]; !asked {
		t.Errorf("the input_required result asks for %v, want the issue title first: %v", inputs, called)
	}
}
