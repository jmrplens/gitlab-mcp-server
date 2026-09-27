//go:build httpe2e

// result_type_test.go holds what the go-sdk this binary is built against
// writes as the resultType of a tools/call result a receiving middleware made
// rather than the tool handler. It is the first item of issue 961 and row 61
// of docs/development/upstream-bugs.md.
//
// 2026-07-28 puts resultType on every result, and go-sdk v1.8.0 sets it on a
// tools/call result only inside its own tool dispatcher. The rate limiter
// answers a refused tools/call with a tool-error result before that dispatcher
// runs, so the refusal goes out without the field. A client MUST read an absent
// resultType as complete, so nothing breaks, but a strict client rejects it.
// Upstream fixed it in e40f35d, which no tag carried when this was written;
// the bump that brings it fails here, in its own pull request, with a message
// saying what to update.
package httpe2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// resultTypeFix is what a failure here asks the pull request to do.
const resultTypeFix = "Update row 61 of docs/development/upstream-bugs.md and its section, drop F-20 from row RTC-001 " +
	"in internal/tenancy/decisions_allow.go, and turn this test into the assertion that the refusal carries " +
	"resultType \"complete\", in this same pull request."

// toolCallAtModernRevision is a tools/call exactly as a 2026-07-28 client sends
// it, on a tool that reaches no GitLab, so the only thing that can refuse it is
// the rate limiter.
const toolCallAtModernRevision = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_find_action",` +
	`"arguments":{"query":"list projects"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
	`"io.modelcontextprotocol/clientCapabilities":{}}}}`

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

// callToolAtModernRevision makes one tools/call and decodes its JSON-RPC
// response, SSE-framed or not.
func callToolAtModernRevision(t *testing.T, srv *server) toolCallResult {
	t.Helper()
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: toolCallAtModernRevision,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-result-type"},
	})
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

// TestRateLimitedToolCall_ModernRevision_CarriesNoResultTypeWhereTheServedOneDoes
// pins that the rate limiter's refusal of a tools/call at 2026-07-28 carries no
// resultType, while the call it served just before carries "complete".
//
// The served call is the control: it is what makes the absence about who built
// the result rather than about the revision, the transport or the tool. The
// bucket holds one call and refills once every thousand seconds, so the first
// call is served and the second refused whatever the machine's speed.
func TestRateLimitedToolCall_ModernRevision_CarriesNoResultTypeWhereTheServedOneDoes(t *testing.T) {
	gitlab := acceptingGitLab(t)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--rate-limit-rps=0.001",
		"--rate-limit-burst=1",
	)

	served := callToolAtModernRevision(t, srv)
	if isError, _ := served.Result["isError"].(bool); isError {
		t.Fatalf("the first call, which the bucket had room for, was refused: %v", served.Result)
	}
	if got, ok := served.Result["resultType"]; !ok || got != "complete" {
		t.Fatalf("a served tools/call at 2026-07-28 carried resultType %v (present: %t), want \"complete\": the control is broken, so this test cannot say anything about the refusal", got, ok)
	}

	refused := callToolAtModernRevision(t, srv)
	if isError, _ := refused.Result["isError"].(bool); !isError || !strings.Contains(strings.ToLower(refusalText(refused.Result)), "rate limit") {
		t.Fatalf("the second call was not the rate limiter's refusal, so this test is not about a middleware-made result: %v", refused.Result)
	}
	if got, ok := refused.Result["resultType"]; ok {
		t.Errorf("the rate limiter's refusal now carries resultType %v: the go-sdk this binary is built against labels a result a middleware made, which is e40f35d (go-sdk#1226). %s",
			got, resultTypeFix)
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
