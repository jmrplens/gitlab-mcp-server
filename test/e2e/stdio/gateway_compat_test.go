//go:build stdioe2e

// gateway_compat_test.go drives the description-substitution knob over the
// real binary: GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS is stdio configuration,
// and stdio configuration is exactly what the in-process suites cannot see.
// One test proves a configured substitution reaches the wire; the other
// proves a malformed value refuses to start instead of serving an unrewritten
// catalog to the gateway the operator configured it for.
package stdioe2e

import (
	"strings"
	"testing"
	"time"
)

// escapeSubstitutionHalf escapes one half of an old=new pair the way the
// documented format defines, so arbitrary served text can be the pattern.
func escapeSubstitutionHalf(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return strings.ReplaceAll(s, "=", `\=`)
}

// firstToolDescription returns the name and description of the first tool
// with a non-empty description in a tools/list response.
func firstToolDescription(t *testing.T, got map[string]any) (name, description string) {
	t.Helper()
	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result missing: %v", got)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools/list returned no tools: %v", got)
	}
	for _, raw := range tools {
		tool, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		desc, _ := tool["description"].(string)
		if desc == "" {
			continue
		}
		toolName, _ := tool["name"].(string)
		return toolName, desc
	}
	t.Fatal("no listed tool carries a description; the fixture surface changed")
	return "", ""
}

// TestDescriptionSubstitutions_RewriteReachesTheWire starts one server to
// learn a real served description, then a second with a substitution whose
// pattern is that description, and asserts the rewritten text is what
// crosses stdout. Reading the pattern from the binary itself keeps the test
// true whatever the catalog's wording becomes.
func TestDescriptionSubstitutions_RewriteReachesTheWire(t *testing.T) {
	gitlab := startFakeGitLab(t)

	control := startSession(t, baseEnv(gitlab.URL))
	got := control.call(t, request(1, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("control tools/list failed: %v", got["error"])
	}
	toolName, original := firstToolDescription(t, got)

	const rewritten = "REWRITTEN BY THE SUBSTITUTION KNOB"
	env := baseEnv(gitlab.URL)
	env["GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS"] = escapeSubstitutionHalf(original) + "=" + rewritten

	s := startSession(t, env)
	got = s.call(t, request(1, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("tools/list under substitutions failed: %v", got["error"])
	}
	result := got["result"].(map[string]any)
	for _, raw := range result["tools"].([]any) {
		tool, ok := raw.(map[string]any)
		if !ok || tool["name"] != toolName {
			continue
		}
		if desc, _ := tool["description"].(string); desc != rewritten {
			t.Errorf("tool %s description = %q, want %q", toolName, desc, rewritten)
		}
		return
	}
	t.Fatalf("tool %s missing from the substituted listing", toolName)
}

// TestDescriptionSubstitutions_MalformedValueRefusesToStart pins the failure
// mode: an operator who configures the knob is complying with a gateway rule,
// and a server that silently dropped a malformed value would sail through
// every local check and be rejected at the gateway's door. Refusing to start,
// with the variable named on stderr, is the only version of this error the
// operator meets before the gateway does.
func TestDescriptionSubstitutions_MalformedValueRefusesToStart(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS"] = "no-separator"

	s := startSession(t, env)
	code, exited := s.waitExit(t, 20*time.Second)
	if !exited {
		t.Fatalf("the server started despite a malformed substitution value\nstderr: %s", s.stderrText())
	}
	if code == 0 {
		t.Errorf("the server exited 0 on a configuration error\nstderr: %s", s.stderrText())
	}
	// Waited for rather than read: the process has exited, but the harness
	// copies stderr on its own goroutine and may still be draining the pipe.
	s.waitForStderr(t, "GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS", 5*time.Second)
}
