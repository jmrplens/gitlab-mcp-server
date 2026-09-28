//go:build stdioe2e

package stdioe2e

import (
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The startup line the tool-call limiter writes when it is attached, and the
// one serveStdio writes once the server is built. The second comes after the
// first whenever the first is written at all, so it is what an absence of the
// first is read against.
const (
	rateLimitEnabledLine = "rate limit enabled"
	stdioStartedLine     = "starting MCP server"
)

// refusedAsToolError reports whether a tools/call answer is the limiter's
// refusal: a result flagged as an error whose text begins with the prefix
// every refusal of this bucket carries, naming the tool it refused.
func refusedAsToolError(got map[string]any) bool {
	result, ok := got["result"].(map[string]any)
	if !ok || result["isError"] != true {
		return false
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		return false
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		return false
	}
	text, _ := first["text"].(string)
	return strings.HasPrefix(text, toolutil.RateLimitRefusalPrefix+"gitlab_execute_action")
}

// TestRateLimit_VariableSet_RefusesAStdioToolCall is the proof issue 959 asked
// for: MCP's one mandatory limit ("Rate limit tool invocations") is switched
// on for stdio by GITLAB_MCP_RATE_LIMIT_RPS, and a tool call beyond the bucket
// is refused in the shape register row RTC-001 declares. The flag
// --rate-limit-rps belongs to HTTP mode alone, so on stdio the variable is the
// whole switch, and nothing drove it against a binary before this.
//
// A bucket of one that refills once every thousand seconds is spent by the
// first call and stays empty for the rest of the test, so the second tool call
// and the resource read after it both meet an empty bucket. The read is there
// because the bucket is not the tool calls' alone: every method that reaches
// GitLab with the caller's credential draws on it, and those that have no
// error flag in their result are refused in-band with the code that mirrors
// HTTP 429.
func TestRateLimit_VariableSet_RefusesAStdioToolCall(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["GITLAB_MCP_RATE_LIMIT_RPS"] = "0.001"
	env["GITLAB_MCP_RATE_LIMIT_BURST"] = "1"
	s := startSession(t, env)

	if first := s.call(t, currentUserCall(1)); !served(first) {
		t.Fatalf("the first tool call was not served; the bucket holds one token: %v", first)
	}

	t.Run("a tool call past the bucket is a tool error", func(t *testing.T) {
		got := s.call(t, currentUserCall(2))
		if !refusedAsToolError(got) {
			t.Errorf("the second tool call against an empty bucket was not refused with %q: %v",
				toolutil.RateLimitRefusalPrefix+"gitlab_execute_action", got)
		}
	})

	t.Run("a resource read on the same bucket is refused in-band", func(t *testing.T) {
		got := s.call(t, request(3, "resources/read", `{"uri":"gitlab://tools"}`))
		rpcErr, ok := got["error"].(map[string]any)
		if !ok {
			t.Fatalf("a resource read against the empty bucket was answered: %v", got)
		}
		if code, _ := rpcErr["code"].(float64); int(code) != tenancy.CodeTooManyRequests {
			t.Errorf("refused with code %v, want %d", rpcErr["code"], tenancy.CodeTooManyRequests)
		}
		if msg, _ := rpcErr["message"].(string); !strings.HasPrefix(msg, toolutil.RateLimitRefusalPrefix+"resources/read") {
			t.Errorf("refused with %q, want the limiter's refusal naming resources/read", msg)
		}
	})

	s.waitForStderr(t, rateLimitEnabledLine, 5*time.Second)
}

// TestRateLimit_VariableUnset_IsOffOnStdio pins the other half of issue 959's
// position: with nothing set, stdio attaches no limiter. A process serving one
// person with their own token has no co-tenant to protect, so a limiter there
// only costs latency, and GitLab's own limits still apply to every call.
//
// Calls served are not enough to show it, since a limiter that refills fast
// enough serves them too. So the calls are made past the burst of 40 either
// transport would give a bucket, none of them may be refused, and the startup
// line the limiter writes when it is attached must be absent: read once the
// line the server always writes after it is there, since stderr reaches this
// harness on a goroutine of its own.
func TestRateLimit_VariableUnset_IsOffOnStdio(t *testing.T) {
	s := startSession(t, baseEnv(startFakeGitLab(t).URL))

	const calls = 50
	for i := 1; i <= calls; i++ {
		if got := s.call(t, currentUserCall(i)); !served(got) {
			t.Fatalf("call %d of %d was not served with no rate configured: %v", i, calls, got)
		}
	}

	logs := s.waitForStderr(t, stdioStartedLine, 5*time.Second)
	if strings.Contains(logs, rateLimitEnabledLine) {
		t.Errorf("stdio attached the limiter with GITLAB_MCP_RATE_LIMIT_RPS unset; the default is off:\n%s", logs)
	}
}
