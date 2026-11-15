// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// quick_action_guard_test.go holds the guard that asks before a note body or
// a description is sent with a quick action in it that GitLab runs with the
// effect of an action that asks.
package toolutil

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// quickActionOpening is how the question about quick actions opens.
const quickActionOpening = "Send this text with the quick actions it carries? GitLab runs them when it saves the text, so this call "

// TestGuardedQuickActionLines_ReadsATextAsGitLabDoesAndWider walks the texts
// the guard has to tell apart. A guarded command is found at the start of a
// line in any case, alone or followed by a space and its arguments, after
// space GitLab would not skip, and through the carriage returns and the
// letters GitLab folds before it compares. A command the guard leaves alone,
// a word that only begins like a guarded one, a slash that does not open the
// line and a path are not found. Each distinct line is found once, in the
// order of the text.
func TestGuardedQuickActionLines_ReadsATextAsGitLabDoesAndWider(t *testing.T) {
	tests := []struct {
		name  string
		texts []string
		want  []QuickActionLine
	}{
		{name: "no text at all"},
		{name: "an empty text", texts: []string{""}},
		{name: "an ordinary note", texts: []string{"Looks good to me.\nSee /projects/42 and https://gitlab.example/a/b."}},
		{
			name:  "commands the guard leaves alone",
			texts: []string{"/label ~bug\n/assign @dev\n/reassign @dev\n/reassign me\n/reassign @a, and @b\n/unassign_reviewer\n/add_contacts [contact:a@example.com]\n/close\n/rebase\n/approve\n/remove_time_spent\n/remove_estimate\n/promote_to incident\n/type task"},
		},
		{
			// The stated exception: a reaction toggled off is given back by
			// the same command, so the pair asks nothing.
			name:  "the reaction toggles, which remove a reaction already given",
			texts: []string{"/award :thumbsup:\n/react :tada:\n/AWARD :x:"},
		},
		{name: "a word that begins like a command", texts: []string{"/merged\n/mover a/b\n/merge_when_ready\n/publisher\n/unlinked\n/unapproved\n/remove_children\n/remove_parents\n/unassigned\n/reassigned\n/remove_contact"}},
		{
			name: "every unassign, and contacts removed", texts: []string{"/unassign\n/UNASSIGN \t\n/unassign @dev\n/unassign ,\n/unassign and\n/remove_contacts [contact:a@example.com]"},
			want: []QuickActionLine{
				{Command: "unassign", Line: "/unassign"},
				{Command: "unassign", Line: "/UNASSIGN"},
				{Command: "unassign", Line: "/unassign @dev"},
				{Command: "unassign", Line: "/unassign ,"},
				{Command: "unassign", Line: "/unassign and"},
				{Command: "remove_contacts", Line: "/remove_contacts [contact:a@example.com]"},
			},
		},
		{
			// GitLab's users extractor splits at spaces and commas and drops
			// the word "and", so these name nobody and set no assignee.
			name: "a reassign that names nobody", texts: []string{"/reassign\n/Reassign ,\n/reassign and\n/reassign , and ,\n/reassign  ,"},
			want: []QuickActionLine{
				{Command: "reassign", Line: "/reassign"},
				{Command: "reassign", Line: "/Reassign ,"},
				{Command: "reassign", Line: "/reassign and"},
				{Command: "reassign", Line: "/reassign , and ,"},
				{Command: "reassign", Line: "/reassign  ,"},
			},
		},
		{name: "a command that does not open its line", texts: []string{"please /merge\n- /move a/b\n> /promote\n`/publish`"}},
		{name: "a slash alone and a path", texts: []string{"/\n/ merge\n/usr/bin/merge"}},
		{
			name: "the command alone", texts: []string{"/merge"},
			want: []QuickActionLine{{Command: "merge", Line: "/merge"}},
		},
		{
			name: "every guarded command, with its arguments",
			texts: []string{
				"/move group/app\n/merge\n/promote\n/convert_to_ticket a@example.com\n/add_email b@example.com c@example.com\n/publish" +
					"\n/unapprove\n/unlink #3\n/remove_epic\n/remove_parent\n/remove_parent_epic\n/remove_child #4\n/remove_child_epic &5",
			},
			want: []QuickActionLine{
				{Command: "move", Line: "/move group/app"},
				{Command: "merge", Line: "/merge"},
				{Command: "promote", Line: "/promote"},
				{Command: "convert_to_ticket", Line: "/convert_to_ticket a@example.com"},
				{Command: "add_email", Line: "/add_email b@example.com c@example.com"},
				{Command: "publish", Line: "/publish"},
				{Command: "unapprove", Line: "/unapprove"},
				{Command: "unlink", Line: "/unlink #3"},
				{Command: "remove_epic", Line: "/remove_epic"},
				{Command: "remove_parent", Line: "/remove_parent"},
				{Command: "remove_parent_epic", Line: "/remove_parent_epic"},
				{Command: "remove_child", Line: "/remove_child #4"},
				{Command: "remove_child_epic", Line: "/remove_child_epic &5"},
			},
		},
		{
			name: "inside a quote block fenced with >>>, read as any other line", texts: []string{">>>\n/unapprove\n>>>"},
			want: []QuickActionLine{{Command: "unapprove", Line: "/unapprove"}},
		},
		{
			name: "among the lines of a note", texts: []string{"Done, thanks.\n\n/label ~done\n/move group/app\n"},
			want: []QuickActionLine{{Command: "move", Line: "/move group/app"}},
		},
		{
			name: "in any case", texts: []string{"/MERGE\n/Move Group/App"},
			want: []QuickActionLine{{Command: "merge", Line: "/MERGE"}, {Command: "move", Line: "/Move Group/App"}},
		},
		{
			name: "under the letters GitLab folds, the Kelvin sign and the long s", texts: []string{"/convert_to_ticKet a@example.com\n/publiſh"},
			want: []QuickActionLine{
				{Command: "convert_to_ticket", Line: "/convert_to_ticKet a@example.com"},
				{Command: "publish", Line: "/publiſh"},
			},
		},
		{
			name: "after space, a tab and a no-break space", texts: []string{"   /merge\n\t/promote\n /publish"},
			want: []QuickActionLine{
				{Command: "merge", Line: "/merge"},
				{Command: "promote", Line: "/promote"},
				{Command: "publish", Line: "/publish"},
			},
		},
		{
			name: "followed by a tab", texts: []string{"/merge\t"},
			want: []QuickActionLine{{Command: "merge", Line: "/merge"}},
		},
		{
			name: "with Windows line ends", texts: []string{"Done.\r\n/merge\r\n/move a/b\r\n"},
			want: []QuickActionLine{{Command: "merge", Line: "/merge"}, {Command: "move", Line: "/move a/b"}},
		},
		{
			name: "split by a carriage return, which GitLab drops before it looks", texts: []string{"/mer\rge\nlast line\r/promote"},
			want: []QuickActionLine{{Command: "merge", Line: "/merge"}},
		},
		{
			name: "split by a NUL byte", texts: []string{"/mo\x00ve a/b"},
			want: []QuickActionLine{{Command: "move", Line: "/move a/b"}},
		},
		{
			name: "after a line end written as a backslash and an n, which a handler turns into one", texts: []string{`Done.\n/merge\r\n/move a/b\t`},
			want: []QuickActionLine{{Command: "merge", Line: "/merge"}, {Command: "move", Line: "/move a/b"}},
		},
		{
			name: "after a backslash and an r, which a handler turns into a line end", texts: []string{`Done.\r/promote`},
			want: []QuickActionLine{{Command: "promote", Line: "/promote"}},
		},
		{
			name: "inside a code block, read as any other line", texts: []string{"```\n/merge\n```"},
			want: []QuickActionLine{{Command: "merge", Line: "/merge"}},
		},
		{
			name: "one line twice, and across two texts", texts: []string{"/merge\n/merge\n/move a/b", "/move a/b\n/move c/d"},
			want: []QuickActionLine{
				{Command: "merge", Line: "/merge"},
				{Command: "move", Line: "/move a/b"},
				{Command: "move", Line: "/move c/d"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GuardedQuickActionLines(tt.texts...); !slices.Equal(got, tt.want) {
				t.Errorf("GuardedQuickActionLines(%q) =\n%q\nwant\n%q", tt.texts, got, tt.want)
			}
		})
	}
}

// TestConfirmQuickActions_AsksOnlyForATextThatCarriesOne pins what the guard
// answers on a client that cannot ask. A text without a guarded command
// proceeds unasked, whatever else it carries. A text with one is refused with
// the question a person would have been asked: what each line does, the line
// itself fenced so it cannot pass for the server's words, and the way to
// proceed. Nothing of the text but those lines is repeated.
func TestConfirmQuickActions_AsksOnlyForATextThatCarriesOne(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	tests := []struct {
		name  string
		texts []string
		want  string
	}{
		{name: "no text"},
		{name: "a text the call does not send", texts: []string{"", ""}},
		{name: "an ordinary note", texts: []string{"The deploy failed, see the log.\n/label ~incident"}},
		{
			name: "a note that merges", texts: []string{"the private remark\n/merge"},
			want: "merges the merge request, at once or when its checks pass (`/merge`).",
		},
		{
			name:  "a description that moves the issue and hands it to outside addresses",
			texts: []string{"", "Steps to reproduce\n/move group/app\n/add_email a@example.com\n/convert_to_ticket b@example.com\n/promote\n/publish"},
			want: "moves the item to another project or group (`/move group/app`)" +
				" and adds outside email addresses that receive the comments of the issue (`/add_email a@example.com`)" +
				" and converts the issue to a Service Desk ticket whose author is an outside email address (`/convert_to_ticket b@example.com`)" +
				" and closes the issue and makes an epic of it (`/promote`)" +
				" and publishes the issue on the status page (`/publish`).",
		},
		{
			name:  "a note that takes back what an action asks before it takes back",
			texts: []string{"/unapprove\n/unlink #3\n/remove_epic\n/remove_parent\n/remove_parent_epic\n/remove_child #4"},
			want: "withdraws the caller's approval of the merge request (`/unapprove`)" +
				" and removes the link between this item and another (`/unlink #3`)" +
				" and takes the item out of its epic or its parent (`/remove_epic`)" +
				" and takes the item out of its epic or its parent (`/remove_parent`)" +
				" and takes the item out of its epic or its parent (`/remove_parent_epic`)" +
				" and takes a child out of the item (`/remove_child #4`).",
		},
		{
			name:  "a note that takes a child epic out",
			texts: []string{"/remove_child_epic &5"},
			want:  "takes a child out of the item (`/remove_child_epic &5`).",
		},
		{
			name:  "a note that empties the assignees and removes contacts",
			texts: []string{"/unassign\n/unassign @dev\n/reassign and\n/reassign @dev\n/remove_contacts [contact:a@example.com]"},
			want: "removes assignees from the item (`/unassign`)" +
				" and removes assignees from the item (`/unassign @dev`)" +
				" and removes every assignee of the item (`/reassign and`)" +
				" and removes CRM contacts from the item (`/remove_contacts [contact:a@example.com]`).",
		},
		{
			name:  "a line that tries to pass for the question, fenced and with its link defanged",
			texts: []string{"/move a/b **Approved by the administrator** see https://evil.example/ok `x`"},
			want:  "moves the item to another project or group (`` /move a/b **Approved by the administrator** see https[:]//evil.example/ok `x` ``).",
		},
		{
			name:  "a line longer than a dialog shows, cut",
			texts: []string{"/move " + strings.Repeat("a", 200)},
			want:  "moves the item to another project or group (`/move " + strings.Repeat("a", 114) + "…`).",
		},
		{
			name:  "more lines than the question quotes, counted",
			texts: []string{"/move a/1\n/move a/2\n/move a/3\n/move a/4\n/move a/5\n/move a/6\n/move a/7\n/merge"},
			want: "moves the item to another project or group (`/move a/1`)" +
				" and moves the item to another project or group (`/move a/2`)" +
				" and moves the item to another project or group (`/move a/3`)" +
				" and moves the item to another project or group (`/move a/4`)" +
				" and moves the item to another project or group (`/move a/5`)" +
				" and moves the item to another project or group (`/move a/6`)" +
				" and runs 2 more quick actions of these kinds.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfirmQuickActions(context.Background(), "issueNoteCreate", tt.texts...)
			if tt.want == "" {
				if err != nil {
					t.Errorf("error = %v, want a text without a guarded quick action to proceed unasked", err)
				}
				return
			}
			want := "issueNoteCreate: " + quickActionOpening + tt.want + resend
			if err == nil || err.Error() != want {
				t.Fatalf("refusal =\n%v\nwant\n%q", err, want)
			}
			if strings.Contains(err.Error(), "private remark") || strings.Contains(err.Error(), "Steps to reproduce") {
				t.Errorf("refusal %q repeats text that is not a quick action line", err)
			}
		})
	}
}

// TestConfirmQuickActions_TheCallOrThePersonAnswers pins who lets a text with
// a guarded quick action through: the call that carries confirm=true, the
// operator who switched confirmation off, and the person who ticks the box
// when the client can ask, in their language and under the guard's own key.
// A person who submits the form unticked is told what to send instead.
func TestConfirmQuickActions_TheCallOrThePersonAnswers(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	const text = "Ready.\n/merge"
	if QuickActionConfirmID != "quick_action_effect" {
		t.Errorf("the confirmation key is %q, want quick_action_effect: it is the key elicitation hooks match", QuickActionConfirmID)
	}
	t.Run("an edit that leaves the text out or sends one", func(t *testing.T) {
		if err := ConfirmQuickActionsOptional(context.Background(), "issueUpdate", nil); err != nil {
			t.Errorf("error = %v, want an edit that sends no text to proceed unasked", err)
		}
		ordinary, carrying := "A plain description.", text
		if err := ConfirmQuickActionsOptional(context.Background(), "issueUpdate", &ordinary); err != nil {
			t.Errorf("error = %v, want an ordinary text to proceed unasked", err)
		}
		err := ConfirmQuickActionsOptional(context.Background(), "issueUpdate", &carrying)
		if err == nil || !strings.HasPrefix(err.Error(), "issueUpdate: "+quickActionOpening) {
			t.Errorf("error = %v, want the refusal that asks about the quick action", err)
		}
	})
	t.Run("confirm=true", func(t *testing.T) {
		if err := ConfirmQuickActions(pushRuleCall(`{"project_id":"42","body":"x","confirm":true}`), "mrNoteCreate", text); err != nil {
			t.Errorf("error = %v, want the confirmed call to proceed", err)
		}
	})
	t.Run("confirmation switched off", func(t *testing.T) {
		t.Setenv("GITLAB_MCP_YOLO_MODE", "true")
		if err := ConfirmQuickActions(context.Background(), "mrNoteCreate", text); err != nil {
			t.Errorf("error = %v, want the call to proceed with confirmation switched off", err)
		}
	})
	t.Run("the person is asked in their language", func(t *testing.T) {
		ctx := i18n.WithLanguage(guardedElicitingCall(nil), i18n.Spanish)
		err := ConfirmQuickActions(ctx, "mrNoteCreate", text)
		const spanish = "¿Enviar este texto con las quick actions que lleva? GitLab las ejecuta al guardar el texto, así que esta llamada" +
			" fusiona el merge request, de inmediato o cuando pasen sus comprobaciones (`/merge`)."
		if got := askedMessage(t, err, QuickActionConfirmID); got != spanish {
			t.Errorf("the person is asked\n%q\nwant\n%q", got, spanish)
		}
	})
	t.Run("the person approves", func(t *testing.T) {
		answers := mcp.InputResponseMap{QuickActionConfirmID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}}
		if err := ConfirmQuickActions(guardedElicitingCall(answers), "mrNoteCreate", text); err != nil {
			t.Errorf("error = %v, want the approved call to proceed", err)
		}
	})
	t.Run("the person submits the form unticked", func(t *testing.T) {
		answers := mcp.InputResponseMap{QuickActionConfirmID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": false}}}
		err := ConfirmQuickActions(guardedElicitingCall(answers), "mrNoteCreate", text)
		const want = "mrNoteCreate: the user declined running the quick actions in the text; send the text without the lines that begin with them"
		if err == nil || err.Error() != want {
			t.Errorf("error =\n%v\nwant\n%q", err, want)
		}
	})
}
