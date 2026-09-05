//go:build stdioe2e

// token_scope_test.go drives the narrowing a token's scope imposes over the
// real binary on stdio: a read_api token is served the read-only surface, the
// way the HTTP pool already served it per entry, and the dynamic surface names
// the token as the reason a write action is withheld. Scope detection is
// stdio startup configuration, which is exactly what the in-process suites
// cannot see.
package stdioe2e

import (
	"maps"
	"strings"
	"testing"
)

// resultText joins the text blocks of a tools/call result.
func resultText(t *testing.T, got map[string]any) string {
	t.Helper()
	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call result missing: %v", got)
	}
	content, _ := result["content"].([]any)
	var b strings.Builder
	for _, raw := range content {
		if block, isMap := raw.(map[string]any); isMap {
			if text, _ := block["text"].(string); text != "" {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

// TestTokenScope_ReadAPITokenIsServedTheReadOnlySurface verifies that stdio
// narrows the catalog to what the token can call, as HTTP mode does per pool
// entry: with read_api the individual surface lists the reads and none of the
// writes, and the log says why. The api token beside it is the control that
// proves the writes are removed by the scope and not by something else.
func TestTokenScope_ReadAPITokenIsServedTheReadOnlySurface(t *testing.T) {
	tests := []struct {
		name       string
		scopes     []string
		env        map[string]string
		wantCreate bool
		wantLog    bool
	}{
		{name: "read_api", scopes: []string{"read_api"}, wantCreate: false, wantLog: true},
		{name: "api", scopes: []string{"api"}, wantCreate: true},
		{name: "read_api with scope detection ignored", scopes: []string{"read_api"}, env: map[string]string{"GITLAB_IGNORE_SCOPES": "true"}, wantCreate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = tt.scopes
			env := baseEnv(fake.URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
			maps.Copy(env, tt.env)
			s := startSession(t, env)

			names := toolNames(t, s.call(t, request(1, "tools/list", "")))
			if !contains(names, "gitlab_issue_get") {
				t.Errorf("gitlab_issue_get is not listed: the reads must stay whatever the scope")
			}
			if listed := contains(names, "gitlab_issue_create"); listed != tt.wantCreate {
				t.Errorf("gitlab_issue_create listed = %v, want %v with scopes %v", listed, tt.wantCreate, tt.scopes)
			}
			if logged := strings.Contains(s.stderrText(), "token cannot write"); logged != tt.wantLog {
				t.Errorf("startup log says the token cannot write = %v, want %v\nstderr: %s", logged, tt.wantLog, s.stderrText())
			}
		})
	}
}

// TestTokenScope_DynamicSurfaceNamesTheTokenAsTheReason verifies the other
// half of the contract: on the default surface a write action asked for under
// a read_api token is reported as withheld by the credential, not as an action
// the server does not have, so a model reports the scope instead of a missing
// capability.
func TestTokenScope_DynamicSurfaceNamesTheTokenAsTheReason(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"read_api"}
	s := startSession(t, baseEnv(fake.URL))

	got := s.call(t, request(1, "tools/call", `{"name":"gitlab_execute_action","arguments":{"action":"issue.create","params":{"project_id":"42","title":"never created"}}}`))
	text := resultText(t, got)
	if !strings.Contains(text, "does not carry a GitLab scope that covers it") {
		t.Fatalf("issue.create under read_api answered %q, want the scope named as the reason", text)
	}
	if strings.Contains(text, "unknown action") {
		t.Fatalf("issue.create under read_api was reported as unknown: %q", text)
	}
}
