//go:build httpe2e

// result_type_test.go holds the resultType of a tools/call result a receiving
// middleware made rather than the tool handler, on the real binary. It is the
// first item of issue 961 and row 66 of docs/development/upstream-bugs.md.
//
// 2026-07-28 says a server implementing it MUST include resultType on every
// result, and go-sdk v1.8.0 sets it on a tools/call result only inside its own
// tool dispatcher. The rate limiter answers a refused tools/call with a
// tool-error result before that dispatcher runs, so until the middleware
// labeled its refusal (toolutil.LabelForRevision) the refusal went out
// without the field, and this server broke that MUST on every rate-limited
// tools/call at 2026-07-28. The schema's rule that a client reads an absent
// resultType as complete covers only a result from a server implementing an
// earlier revision, so a client holding a 2026-07-28 server to the schema was
// entitled to reject it.
//
// This asserts the server's answer and not the SDK's behavior, so the go-sdk
// bump that carries upstream's fix (e40f35d) passes here: the SDK then labels
// the refusal itself, with the same value. What fails on that bump is the pin
// on a bare SDK server in internal/toolutil, which says what to delete.
package httpe2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// legacyToolCallBody is the call rateLimitedCall makes, as a client of an
// earlier revision sends it, with nothing in _meta.
const legacyToolCallBody = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` +
	`"name":"gitlab_find_action","arguments":{"query":"list projects"}}}`

// toolCallResult is the part of a tools/call response this test reads. The
// result is decoded as a map so an absent resultType can be told from an
// empty one.
type toolCallResult struct {
	Result map[string]any `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeToolCall decodes the JSON-RPC response to one tools/call, SSE-framed or
// not, and fails the test unless it carries a result.
func decodeToolCall(t *testing.T, got response) toolCallResult {
	t.Helper()
	if got.status != http.StatusOK {
		t.Fatalf("tools/call answered %d, want 200 with the result in the body: %s", got.status, truncate(got.body))
	}
	var decoded toolCallResult
	if err := json.Unmarshal([]byte(jsonRPCPayload(t, got.body)), &decoded); err != nil {
		t.Fatalf("the tools/call response is not JSON-RPC: %v (%s)", err, truncate(got.body))
	}
	if decoded.Error != nil || decoded.Result == nil {
		t.Fatalf("tools/call answered with no result (error %+v): %s", decoded.Error, truncate(got.body))
	}
	return decoded
}

// callToolAtModernRevision makes one tools/call at 2026-07-28 through
// rateLimitedCall, on a tool that reaches no GitLab so that the only thing
// that can refuse it is the rate limiter.
func callToolAtModernRevision(t *testing.T, srv *server) toolCallResult {
	t.Helper()
	return decodeToolCall(t, rateLimitedCall(t, srv))
}

// callToolAtLegacyRevision makes the same call as a client of 2025-11-25 does,
// naming its revision in the header alone and sending none of the headers
// 2026-07-28 added.
func callToolAtLegacyRevision(t *testing.T, srv *server) toolCallResult {
	t.Helper()
	return decodeToolCall(t, srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: legacyToolCallBody,
		headers: map[string]string{
			"PRIVATE-TOKEN":        "glpat-x",
			"MCP-Protocol-Version": "2025-11-25",
			"Mcp-Method":           "",
			"Mcp-Name":             "",
		},
	}))
}

// TestRateLimitedToolCall_EachRevision_CarriesTheResultTypeTheServedCallDoes
// holds the rate limiter's refusal of a tools/call to the resultType the call
// served just before it carries: "complete" at 2026-07-28, which that revision
// requires on every result, and none at 2025-11-25, which is what the SDK
// sends a client of that revision from its own dispatcher.
//
// The served call is the control: it is what makes the refusal's label about
// who built the result rather than about the revision, the transport or the
// tool. The bucket holds one call and refills once every thousand seconds, so
// the first call is served and the second refused whatever the machine's speed.
func TestRateLimitedToolCall_EachRevision_CarriesTheResultTypeTheServedCallDoes(t *testing.T) {
	cases := []struct {
		name string
		call func(*testing.T, *server) toolCallResult
		want any
	}{
		{name: "2026-07-28", call: callToolAtModernRevision, want: "complete"},
		{name: "2025-11-25", call: callToolAtLegacyRevision, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gitlab := acceptingGitLab(t)
			srv := startServer(t, nil,
				"--gitlab-url="+gitlab.url,
				"--rate-limit-rps=0.001",
				"--rate-limit-burst=1",
			)

			served := tc.call(t, srv)
			if isError, _ := served.Result["isError"].(bool); isError {
				t.Fatalf("the first call, which the bucket had room for, was refused: %v", served.Result)
			}
			if got := served.Result["resultType"]; got != tc.want {
				t.Fatalf("a served tools/call at %s carried resultType %v, want %v: the control is broken, so this case cannot say anything about the refusal", tc.name, got, tc.want)
			}

			refused := tc.call(t, srv)
			if isError, _ := refused.Result["isError"].(bool); !isError || !strings.Contains(strings.ToLower(refusalText(refused.Result)), "rate limit") {
				t.Fatalf("the second call was not the rate limiter's refusal, so this case is not about a middleware-made result: %v", refused.Result)
			}
			if got := refused.Result["resultType"]; got != tc.want {
				t.Errorf("the rate limiter's refusal at %s carried resultType %v, want %v, the same as the call the dispatcher served before it",
					tc.name, got, tc.want)
			}
		})
	}
}

// refusalText returns the text of a tool result's first content block, or ""
// when it has none.
func refusalText(result map[string]any) string {
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	return text
}
