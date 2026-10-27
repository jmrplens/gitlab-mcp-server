// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// elicitation_declared_version_test.go pins which protocol revision decides how
// a guided flow asks its questions on stdio, which is row 11 of
// docs/development/upstream-bugs.md and issue 1319, and tells its failure
// apart from the one row 66 of the same register causes (issue 1403).
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
// go-sdk#1226, row 66, merged four commits past the v1.8.0 tag and so in no
// release yet, sets resultType only when a request names the new protocol in
// its _meta, while the dispatcher still chooses the multi round-trip shape by
// the declared version. A release that carries it without go-sdk#1266
// answers this session inputRequests with no resultType at all, which a
// 2025-11-25 client reads as a complete result whose content is null, and the
// flow's question is lost. That fails the pin too, and is a reason to hold the
// bump rather than a sign that row 11's fix landed, so declaredVersionVerdict
// tells the two apart.
//
// The defect shows only against the real SDK and the real binary, so the pin
// lives here. It fails on the go-sdk release that carries go-sdk#1266 with
// declaredVersionFix, naming what to update, and on one that carries
// go-sdk#1226 without it with a message naming rows 11 and 66 and saying the
// bump is to be held or the case answered, not the test rewritten.
package stdioe2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// declaredVersionFix is what the pin asks the pull request that bumps go-sdk
// to do when the flow sends elicitation/create, which only go-sdk#1266
// produces.
const declaredVersionFix = "go-sdk now selects the multi round-trip flow by the negotiated version (go-sdk#1266). " +
	"Update row 11 of docs/development/upstream-bugs.md and its section, the comment on the version check in " +
	"internal/elicitation/flow.go, the elicitation page's \"Which protocol version decides?\" section " +
	"(site/src/content/docs/capabilities/elicitation.mdx and its Spanish twin), and turn this " +
	"test into the assertion of the legacy flow, in this same pull request."

// TestElicitation_InitializeAskingForNewRevision_IsServedTheMultiRoundTripShape
// pins the current answer: initialize asking for 2026-07-28 is answered
// 2025-11-25, and a guided flow called on that session without any
// per-request protocol _meta answers resultType input_required with a request
// for its first field, and sends no elicitation/create. An elicitation/create
// fails it with declaredVersionFix, and any other result with what
// declaredVersionVerdict makes of its shape.
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
	if failure := declaredVersionVerdict(answer["result"]); failure != "" {
		t.Fatal(failure)
	}
	called, _ := answer["result"].(map[string]any)
	inputs, _ := called["inputRequests"].(map[string]any)
	if _, asked := inputs["title"]; !asked {
		t.Errorf("the input_required result asks for %v, want the issue title first: %v", inputs, called)
	}
}

// declaredVersionVerdict reads the result member of the answer the guided flow
// gave on the session negotiated down to 2025-11-25, once the flow has sent
// no elicitation/create, and returns what the pin fails with, or "" for the
// answer row 11 pins today.
//
// The shapes it tells apart are the ones the go-sdk releases around rows 11
// and 66 produce: resultType input_required is v1.8.0's answer, and
// inputRequests with no resultType is go-sdk#1226 without go-sdk#1266. A
// resultType that is present but null is not an absent one, and inputRequests
// that ask for nothing lose no question, so both fall to the shape neither row
// predicts. No failure here names declaredVersionFix, because what row 11's
// fix produces is the elicitation/create the caller fails on before it reads a
// result. A failure prints result as it arrived rather than the object read
// from it, so an absent or null result reads <nil> and not as an empty object.
func declaredVersionVerdict(result any) string {
	called, _ := result.(map[string]any)
	resultType, labeled := called["resultType"]
	if resultType == "input_required" {
		return ""
	}
	if inputs, _ := called["inputRequests"].(map[string]any); !labeled && len(inputs) > 0 {
		return fmt.Sprintf("the guided flow answered the 2025-11-25 session with inputRequests and no resultType: %v. "+
			"That is go-sdk#1226 (row 66 of docs/development/upstream-bugs.md) without go-sdk#1266 (row 11): the SDK still chooses the multi round-trip shape "+
			"by the version the client declared in initialize, and sets resultType only when a request names the new protocol in its _meta, "+
			"so a 2025-11-25 client reads a complete result whose content is null and the flow's question is lost. "+
			"The SDK is not serving the legacy flow, so do not turn this test into that flow's assertion: hold the go-sdk bump until the release carries go-sdk#1266, "+
			"or answer the case in internal/elicitation (for example by labeling the result for that session) and record the decision on rows 11 and 66.",
			result)
	}
	return fmt.Sprintf("the guided flow answered the 2025-11-25 session with %v, which neither row 11 nor row 66 of docs/development/upstream-bugs.md predicts: "+
		"go-sdk v1.8.0 answers resultType input_required, go-sdk#1226 without go-sdk#1266 answers inputRequests with no resultType, "+
		"and go-sdk#1266 sends an elicitation/create instead. Reread both rows and their sections against the go-sdk this module is built against "+
		"before changing this test.",
		result)
}

// TestDeclaredVersionVerdict_EachResultShape_NamesWhatProducedIt holds the
// message the pin fails with to the go-sdk change each result shape comes
// from. The pin meets only the shape the go-sdk this module is built against
// produces, so the other shapes are read here, as values decoded from the
// wire.
func TestDeclaredVersionVerdict_EachResultShape_NamesWhatProducedIt(t *testing.T) {
	asksTitle := map[string]any{"title": map[string]any{"method": "elicitation/create"}}
	row66 := []string{
		"inputRequests and no resultType",
		"row 66", "row 11", "go-sdk#1226", "go-sdk#1266",
		"a 2025-11-25 client", "question is lost",
		"do not turn this test into", "hold the go-sdk bump", "internal/elicitation",
	}
	unpredicted := []string{"neither row 11 nor row 66", "Reread both rows"}
	for _, tc := range []struct {
		name string
		// result is the answer's result member as encoding/json decodes it
		// into an any, nil when the member is absent or null.
		result any
		// want is what the failure says, and nil for the pinned answer.
		want []string
		// refuse is what the failure must not say.
		refuse []string
	}{
		{
			name:   "input_required asking for the title is the pinned answer",
			result: map[string]any{"resultType": "input_required", "inputRequests": asksTitle},
		},
		{
			name:   "input_required is the pinned answer whatever it asks for",
			result: map[string]any{"resultType": "input_required"},
		},
		{
			name:   "inputRequests with no resultType is row 66 without row 11",
			result: map[string]any{"content": nil, "requestState": "state", "inputRequests": asksTitle},
			want:   row66,
			refuse: []string{declaredVersionFix},
		},
		{
			name:   "no resultType and no inputRequests is a shape neither row predicts",
			result: map[string]any{"content": []any{}},
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into"},
		},
		{
			name:   "no resultType and an empty inputRequests asks nothing, so neither row predicts it",
			result: map[string]any{"inputRequests": map[string]any{}},
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into"},
		},
		{
			name:   "a null resultType is not an absent one",
			result: map[string]any{"resultType": nil, "inputRequests": asksTitle},
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into"},
		},
		{
			name:   "complete with inputRequests is a shape neither row predicts",
			result: map[string]any{"resultType": "complete", "inputRequests": asksTitle},
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into"},
		},
		{
			name:   "complete is a shape neither row predicts",
			result: map[string]any{"resultType": "complete", "content": []any{}},
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into"},
		},
		{
			name:   "no result at all, or a null one, is printed as nil rather than as an empty object",
			result: nil,
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into", "map[]"},
		},
		{
			name:   "a result that is not an object is printed as it arrived",
			result: []any{map[string]any{"resultType": "input_required", "inputRequests": asksTitle}},
			want:   unpredicted,
			refuse: []string{declaredVersionFix, "turn this test into", "with map[]"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := declaredVersionVerdict(tc.result)
			if tc.want == nil {
				if got != "" {
					t.Errorf("declaredVersionVerdict(%v) failed the pin, want it to pass: %s", tc.result, got)
				}
				return
			}
			if got == "" {
				t.Fatalf("declaredVersionVerdict(%v) passed the pin, want a failure saying %q", tc.result, tc.want)
			}
			if printed := fmt.Sprint(tc.result); !strings.Contains(got, printed) {
				t.Errorf("the failure does not print the result %s: %s", printed, got)
			}
			assertSays(t, got, tc.want, tc.refuse)
		})
	}
}

// assertSays fails the test for each phrase of want the failure does not
// contain and for each phrase of refuse it does.
func assertSays(t *testing.T, failure string, want, refuse []string) {
	t.Helper()
	for _, said := range want {
		if !strings.Contains(failure, said) {
			t.Errorf("the failure does not say %q: %s", said, failure)
		}
	}
	for _, unsaid := range refuse {
		if strings.Contains(failure, unsaid) {
			t.Errorf("the failure says %q, which this shape does not call for: %s", unsaid, failure)
		}
	}
}
