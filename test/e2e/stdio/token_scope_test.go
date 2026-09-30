//go:build stdioe2e

// token_scope_test.go drives the narrowing a token's scope imposes over the
// real binary on stdio: a read_api token is served the read-only surface, the
// way the HTTP pool already served it per entry, and the dynamic surface names
// the token as the reason a write action is withheld. Scope detection is
// stdio startup configuration, which is exactly what the in-process suites
// cannot see.
package stdioe2e

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
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
		{name: "read_api with scope detection ignored", scopes: []string{"read_api"}, env: map[string]string{"GITLAB_MCP_IGNORE_SCOPES": "true"}, wantCreate: true},
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

// TestTokenScope_FineGrainedTokenIsNotReadOnly verifies that a fine-grained
// personal access token, whose scope list is the single value granular, is
// served as unknown authority rather than as a token that cannot write: every
// write stays listed, the groups that need admin_mode stay listed, and the log
// never says the token cannot write. GitLab judges each call against the
// permissions the token was granted, and a write it lacks is GitLab's own 403.
//
// The api row is the control for the admin tool: a classic token without
// admin_mode loses it, so its presence under the fine-grained token is the scope
// filter reading that list as unknown and not a surface that never had it.
func TestTokenScope_FineGrainedTokenIsNotReadOnly(t *testing.T) {
	tests := []struct {
		name      string
		scopes    []string
		wantAdmin bool
	}{
		{name: "fine-grained", scopes: []string{"granular"}, wantAdmin: true},
		{name: "api without admin_mode", scopes: []string{"api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := startFakeGitLab(t)
			fake.scopes = tt.scopes
			env := baseEnv(fake.URL)
			env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
			s := startSession(t, env)

			names := toolNames(t, s.call(t, request(1, "tools/list", "")))
			if !contains(names, "gitlab_issue_create") {
				t.Errorf("gitlab_issue_create is not listed with scopes %v: the token was served a read-only surface", tt.scopes)
			}
			if listed := contains(names, "gitlab_get_settings"); listed != tt.wantAdmin {
				t.Errorf("gitlab_get_settings listed = %v, want %v with scopes %v", listed, tt.wantAdmin, tt.scopes)
			}
			if strings.Contains(s.stderrText(), "token cannot write") {
				t.Errorf("startup log says the token cannot write with scopes %v\nstderr: %s", tt.scopes, s.stderrText())
			}
		})
	}
}

// TestTokenScope_FineGrainedTokenIsListedWhatUnknownScopesAre verifies the
// reading itself rather than two tools of it: on the individual surface a
// fine-grained token is listed exactly what a token whose scopes could not be
// detected is listed, tool for tool and descriptor for descriptor. That listing
// is the catalog no scope narrowed, so a partial narrowing on the strength of
// the fine-grained list, one write or one admin_mode group removed, makes the
// two differ, and the difference is named.
//
// The unknown-scope session is the fake instance with its self endpoint left
// unanswered, which is what DetectScopes reads as unknown; the write and the
// admin tool checked on it hold that the reference is the unfiltered listing
// and not one that happens to be narrowed the same way.
func TestTokenScope_FineGrainedTokenIsListedWhatUnknownScopesAre(t *testing.T) {
	unknown := individualListing(t, nil)
	fineGrained := individualListing(t, []string{"granular"})

	for _, name := range []string{"gitlab_issue_create", "gitlab_get_settings"} {
		t.Run("reference lists "+name, func(t *testing.T) {
			if _, listed := unknown[name]; !listed {
				t.Errorf("%s is not listed with unknown scopes, so the reference listing is not the unfiltered one", name)
			}
		})
	}

	var missing, extra, differ []string
	for name, tool := range unknown {
		other, listed := fineGrained[name]
		switch {
		case !listed:
			missing = append(missing, name)
		case !reflect.DeepEqual(tool, other):
			differ = append(differ, name)
		}
	}
	for name := range fineGrained {
		if _, listed := unknown[name]; !listed {
			extra = append(extra, name)
		}
	}
	if len(missing)+len(extra)+len(differ) > 0 {
		t.Errorf("the fine-grained listing is not the unknown-scope listing of %d tools: missing %s, extra %s, described differently %s",
			len(unknown), firstNames(missing), firstNames(extra), firstNames(differ))
	}
}

// individualListing starts a server on the individual surface against a fake
// instance whose self endpoint reports scopes, or is left unanswered when
// scopes is nil, and returns every tool it lists keyed by name, following the
// cursor to the last page so that no comparison is made on a prefix.
func individualListing(t *testing.T, scopes []string) map[string]any {
	t.Helper()
	fake := startFakeGitLab(t)
	fake.scopes = scopes
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = "individual"
	s := startSession(t, env)

	const maxPages = 50
	tools := map[string]any{}
	params := ""
	for id := 1; id <= maxPages; id++ {
		got := s.call(t, request(id, "tools/list", params))
		result, ok := got["result"].(map[string]any)
		if !ok {
			t.Fatalf("tools/list with scopes %v answered no result: %v", scopes, got)
		}
		page, _ := result["tools"].([]any)
		for _, entry := range page {
			tool, _ := entry.(map[string]any)
			name, _ := tool["name"].(string)
			tools[name] = tool
		}
		next, _ := result["nextCursor"].(string)
		if next == "" {
			return tools
		}
		cursor, err := json.Marshal(next)
		if err != nil {
			t.Fatalf("encoding the cursor %q: %v", next, err)
		}
		params = `{"cursor":` + string(cursor) + `}`
	}
	t.Fatalf("tools/list with scopes %v still had a next page after %d pages", scopes, maxPages)
	return nil
}

// firstNames renders a list of tool names for a failure message: its length
// and, sorted, at most the first ten, so a wholesale difference stays readable.
func firstNames(names []string) string {
	slices.Sort(names)
	return fmt.Sprintf("%d %v", len(names), names[:min(len(names), 10)])
}

// TestTokenScope_FineGrainedWriteReachesGitLab verifies the same on the default
// surface: a write asked for under a fine-grained token is dispatched to GitLab
// rather than withheld by the credential, so the answer comes from the instance
// and not from a scope the token never had to carry.
func TestTokenScope_FineGrainedWriteReachesGitLab(t *testing.T) {
	fake := startFakeGitLab(t)
	fake.scopes = []string{"granular"}
	s := startSession(t, baseEnv(fake.URL))

	got := s.call(t, request(1, "tools/call", `{"name":"gitlab_execute_action","arguments":{"action":"issue.create","params":{"project_id":"42","title":"sent to GitLab"}}}`))
	text := resultText(t, got)
	if strings.Contains(text, "does not carry a GitLab scope that covers it") {
		t.Fatalf("issue.create under a fine-grained token was withheld by scope: %q", text)
	}
	if !served(got) {
		t.Fatalf("issue.create under a fine-grained token was not served: %q", text)
	}
	if created := fake.issuesCreated.Load(); created != 1 {
		t.Errorf("the instance was asked to create %d issues, want exactly 1", created)
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
