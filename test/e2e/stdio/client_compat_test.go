//go:build stdioe2e

package stdioe2e

import (
	"regexp"
	"strings"
	"testing"
)

// codexClientInfo is how Codex has named itself since v0.20, in initialize and
// in a 2026-07-28 request's _meta alike.
const codexClientInfo = `{"name":"codex-mcp-client","title":"Codex","version":"0.148.0"}`

// priorityOnTheWire captures every annotation priority exactly as the server
// wrote it, so 1 and 1.0 stay two different answers.
var priorityOnTheWire = regexp.MustCompile(`"priority":([^,}\]]+)`)

// compatCase is one session the Codex profile is judged on.
type compatCase struct {
	name string
	// client is the clientInfo the session reports, a JSON object.
	client string
	// legacy speaks 2025-11-25, with initialize, rather than 2026-07-28.
	legacy bool
	// off sets GITLAB_MCP_CLIENT_COMPAT=off.
	off bool
	// rounded is whether the profile applies, so every priority is written
	// as an integer.
	rounded bool
}

// TestClientCompat_CodexProfile_OnTheWire drives the Codex profile through the
// binary over stdio, in both protocol eras, and its switch. It is the one
// place this server chooses what it sends from the self-reported clientInfo,
// which MCP 2026-07-28 says an implementation SHOULD NOT do; issue 959 keeps it
// as a deliberate deviation, and this pins that it does what the deviation is
// kept for and nothing else a caller did not ask for.
//
// What is asserted is the line on the pipe rather than its decoding, because
// what the profile works around is a client that refuses a priority written
// with a decimal point: it accepts 1 and refuses 1.0 as firmly as 0.6, and a
// decoder reads 1 and 1.0 as one number. So a Codex session must see every
// priority written as a bare integer, and every other session, or a Codex one
// with GITLAB_MCP_CLIENT_COMPAT=off, must see the fraction the server set,
// which is also what shows the call carried a priority for the profile to
// round at all.
func TestClientCompat_CodexProfile_OnTheWire(t *testing.T) {
	for _, tc := range []compatCase{
		{name: "codex at 2026-07-28 gets integers", client: codexClientInfo, rounded: true},
		{name: "codex at 2025-11-25 gets integers", client: codexClientInfo, legacy: true, rounded: true},
		{name: "another client keeps the fraction", client: `{"name":"claude-code","title":"Claude Code","version":"2.0.0"}`},
		{name: "an openai-mcp label carrying the word Codex keeps the fraction", client: `{"name":"openai-mcp (Codex)","version":"1.0.0"}`},
		{name: "codex with the profile switched off keeps the fraction", client: codexClientInfo, off: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := servedToolCallLine(t, tc)
			priorities := priorityOnTheWire.FindAllStringSubmatch(line, -1)
			if len(priorities) == 0 {
				t.Fatalf("the result carries no priority, so it cannot show the profile either way: %s", line)
			}
			for _, p := range priorities {
				fractional := strings.ContainsAny(p[1], ".eE")
				if tc.rounded && fractional {
					t.Errorf("a Codex session was sent priority %s, which Codex refuses; want an integer: %s", p[1], line)
				}
				if !tc.rounded && !fractional {
					t.Errorf("priority %s was rounded for a session the profile does not apply to: %s", p[1], line)
				}
			}
		})
	}
}

// servedToolCallLine starts a session for the case, identifies it as the case's
// client in the case's era, and returns the line that answers a tool call
// reaching GitLab, as it crossed the pipe. It fails the test unless the call
// was served, since a refused or unknown call proves nothing about a result.
func servedToolCallLine(t *testing.T, tc compatCase) string {
	t.Helper()
	env := baseEnv(startFakeGitLab(t).URL)
	if tc.off {
		env["GITLAB_MCP_CLIENT_COMPAT"] = "off"
	}
	s := startSession(t, env)

	var line string
	var got map[string]any
	if tc.legacy {
		s.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":`+tc.client+`}}`)
		s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		line, got = s.callRaw(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":`+currentUserArguments+`}`)
	} else {
		line, got = s.callRaw(t, requestFrom(tc.client, 1, "tools/call", currentUserArguments))
	}
	if !served(got) {
		t.Fatalf("the tool call was not served: %s", line)
	}
	return line
}
