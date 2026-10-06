//go:build stdioe2e

package stdioe2e

import (
	"maps"
	"strings"
	"testing"
)

// confirmRefusal is the sentence every surface ends a destructive refusal
// with: the dynamic gate's own, and the one the meta and individual
// dispatchers write for a client that cannot be asked. Matching it rather than
// either whole message is what lets one case table cover the three surfaces.
const confirmRefusal = "Re-send with confirm=true only after the user explicitly approves this operation."

// issueDeleteCalls names the deletion of issue 1 of project 42 the way each
// surface takes it, none of them carrying a confirm: the call issue 1166 is
// about on the dynamic surface, and its twin on the other two.
var issueDeleteCalls = map[string]string{
	"dynamic":    `{"name":"gitlab_execute_action","arguments":{"action":"issue.delete","params":{"project_id":"42","issue_iid":1}}}`,
	"meta":       `{"name":"gitlab_issue","arguments":{"action":"delete","params":{"project_id":"42","issue_iid":1}}}`,
	"individual": `{"name":"gitlab_issue_delete","arguments":{"project_id":"42","issue_iid":1}}`,
}

// deleteOutcome is what one destructive call came to: the text a model reads,
// whether it was flagged an error, and how many deletions reached the
// instance, which is the one fact that says whether anything was dispatched.
type deleteOutcome struct {
	text    string
	isError bool
	reached int32
}

// deleteIssueWithout starts the binary on the given surface and environment,
// sends the surface's deletion of issue 1 without a confirm, and reports what
// came of it. The client declares no capabilities, so it cannot be asked
// through elicitation: the only things that can let the call through are the
// two switches under test.
func deleteIssueWithout(t *testing.T, surface string, extra map[string]string) deleteOutcome {
	t.Helper()

	fake := startFakeGitLab(t)
	env := baseEnv(fake.URL)
	env["GITLAB_MCP_TOOL_SURFACE"] = surface
	maps.Copy(env, extra)
	s := startSession(t, env)

	got := s.call(t, request(1, "tools/call", issueDeleteCalls[surface]))
	text, isError := toolResultText(t, got)
	return deleteOutcome{text: text, isError: isError, reached: fake.issuesDeleted.Load()}
}

// TestYOLOMode_DestructiveCall_DispatchedAlikeOnEverySurface is the scenario
// issue 1166 asked for, against the binary: GITLAB_MCP_YOLO_MODE=true lets a
// destructive call sent without confirm reach GitLab on the default dynamic
// surface, as it already did on the meta and individual ones, and with the
// switch unset all three refuse it in the same sentence and send nothing.
// Before the fix the dynamic row with the switch set was refused and the
// deletion never left the process, while the other two surfaces dispatched it.
func TestYOLOMode_DestructiveCall_DispatchedAlikeOnEverySurface(t *testing.T) {
	for _, surface := range []string{"dynamic", "meta", "individual"} {
		t.Run(surface, func(t *testing.T) {
			t.Run("GITLAB_MCP_YOLO_MODE=true dispatches", func(t *testing.T) {
				got := deleteIssueWithout(t, surface, map[string]string{"GITLAB_MCP_YOLO_MODE": "true"})
				if got.isError {
					t.Errorf("the deletion was refused: %q", got.text)
				}
				if got.reached != 1 {
					t.Errorf("%d deletions reached GitLab, want 1", got.reached)
				}
			})
			t.Run("unset refuses and sends nothing", func(t *testing.T) {
				got := deleteIssueWithout(t, surface, nil)
				if !got.isError || !strings.Contains(got.text, confirmRefusal) {
					t.Errorf("answered %q (isError=%v), want the refusal ending %q", got.text, got.isError, confirmRefusal)
				}
				if got.reached != 0 {
					t.Errorf("%d deletions reached GitLab, want none", got.reached)
				}
			})
		})
	}
}

// TestYOLOMode_DynamicGate_FollowsTheSwitchAndStaysBehindTheOtherGuards holds
// the rest of what the dynamic gate now reads to the binary: AUTOPILOT counts
// while GITLAB_MCP_YOLO_MODE is unset, a set GITLAB_MCP_YOLO_MODE decides
// alone, and read-only and safe mode, which narrow the catalog before any call
// arrives, still keep the deletion from GitLab with the switch on.
func TestYOLOMode_DynamicGate_FollowsTheSwitchAndStaysBehindTheOtherGuards(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		wantError bool
		wantText  string
		wantSent  int32
	}{
		{
			name:     "AUTOPILOT=1 with GITLAB_MCP_YOLO_MODE unset dispatches",
			env:      map[string]string{"AUTOPILOT": "1"},
			wantSent: 1,
		},
		{
			name:      "GITLAB_MCP_YOLO_MODE=false keeps the refusal over AUTOPILOT=true",
			env:       map[string]string{"GITLAB_MCP_YOLO_MODE": "false", "AUTOPILOT": "true"},
			wantError: true,
			wantText:  confirmRefusal,
		},
		{
			name:     "safe mode previews instead",
			env:      map[string]string{"GITLAB_MCP_YOLO_MODE": "true", "GITLAB_MCP_SAFE_MODE": "true"},
			wantText: "Safe mode blocked",
		},
		{
			name:      "read-only withholds the action",
			env:       map[string]string{"GITLAB_MCP_YOLO_MODE": "true", "GITLAB_MCP_READ_ONLY": "true"},
			wantError: true,
			wantText:  `action "issue.delete" exists but is not available`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deleteIssueWithout(t, "dynamic", tc.env)
			if got.isError != tc.wantError {
				t.Errorf("isError = %v, want %v: %q", got.isError, tc.wantError, got.text)
			}
			if tc.wantText != "" && !strings.Contains(got.text, tc.wantText) {
				t.Errorf("answered %q, want it to contain %q", got.text, tc.wantText)
			}
			if got.reached != tc.wantSent {
				t.Errorf("%d deletions reached GitLab, want %d", got.reached, tc.wantSent)
			}
		})
	}
}
