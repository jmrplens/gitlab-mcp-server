// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// push_rule_guard_test.go holds the rule that decides when an edit of push
// rules relaxes them, and the confirmation it asks for when one does.
package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// englishChanges renders what the guard found as the question names it, one
// fragment per setting, so a case reads as the sentence a person is asked.
func englishChanges(changes []i18n.Message) []string {
	out := make([]string, 0, len(changes))
	for _, change := range changes {
		out = append(out, change.English())
	}
	return out
}

// everyRuleSet is a rule with every setting restricting something, the state
// against which each way of relaxing one is visible.
func everyRuleSet() PushRuleState {
	return PushRuleState{
		DenyDeleteTag: true, MemberCheck: true, PreventSecrets: true,
		CommitCommitterCheck: true, CommitCommitterNameCheck: true,
		RejectUnsignedCommits: true, RejectNonDCOCommits: true,
		CommitMessageRegex: "^JIRA-", CommitMessageNegativeRegex: "WIP",
		BranchNameRegex: "^(feature|fix)/", AuthorEmailRegex: "@example\\.com$",
		FileNameRegex: "\\.exe$", MaxFileSize: 50,
	}
}

// TestPushRuleEdit_Relaxations_ByWhatTheRuleHolds walks every row of the
// rule: what an edit sends, against what GitLab holds, and whether that
// relaxes. A switch relaxes only when it turns off one that is on, a pattern
// when it empties or replaces one that is set, and max_file_size when it
// raises or removes a limit that is set. Everything else adds a restriction,
// tightens one or changes nothing, and asks nothing.
func TestPushRuleEdit_Relaxations_ByWhatTheRuleHolds(t *testing.T) {
	tests := []struct {
		name    string
		edit    PushRuleEdit
		current PushRuleState
		want    []string
	}{
		{name: "a switch turned on", edit: PushRuleEdit{PreventSecrets: new(true)}},
		{name: "a switch turned on that is on", edit: PushRuleEdit{PreventSecrets: new(true)}, current: PushRuleState{PreventSecrets: true}},
		{
			name: "a switch turned off that is on", edit: PushRuleEdit{PreventSecrets: new(false)},
			current: PushRuleState{PreventSecrets: true}, want: []string{"turns off prevent_secrets"},
		},
		{name: "a switch turned off that is off", edit: PushRuleEdit{PreventSecrets: new(false)}},
		{
			name: "a pattern emptied that is set", edit: PushRuleEdit{BranchNameRegex: new("")},
			current: PushRuleState{BranchNameRegex: "^feature/"}, want: []string{"removes branch_name_regex"},
		},
		{name: "a pattern emptied that is empty", edit: PushRuleEdit{BranchNameRegex: new("")}},
		{name: "a pattern set where there was none", edit: PushRuleEdit{BranchNameRegex: new("^feature/")}},
		{
			name: "a pattern resent byte for byte", edit: PushRuleEdit{BranchNameRegex: new("^feature/")},
			current: PushRuleState{BranchNameRegex: "^feature/"},
		},
		{
			name: "a pattern replaced by another", edit: PushRuleEdit{BranchNameRegex: new(".*")},
			current: PushRuleState{BranchNameRegex: "^feature/"},
			want:    []string{"replaces branch_name_regex with another pattern that may accept more"},
		},
		{
			name: "a pattern that must not match, emptied", edit: PushRuleEdit{FileNameRegex: new("")},
			current: PushRuleState{FileNameRegex: "\\.exe$"}, want: []string{"removes file_name_regex"},
		},
		{
			name: "the size limit removed", edit: PushRuleEdit{MaxFileSize: new(int64(0))},
			current: PushRuleState{MaxFileSize: 50}, want: []string{"removes the max_file_size limit of 50 MB"},
		},
		{name: "no size limit, resent", edit: PushRuleEdit{MaxFileSize: new(int64(0))}},
		{name: "a size limit set where there was none", edit: PushRuleEdit{MaxFileSize: new(int64(100))}},
		{
			name: "the size limit raised", edit: PushRuleEdit{MaxFileSize: new(int64(100))},
			current: PushRuleState{MaxFileSize: 50}, want: []string{"raises max_file_size from 50 MB to 100 MB"},
		},
		{
			name: "the size limit raised by one", edit: PushRuleEdit{MaxFileSize: new(int64(51))},
			current: PushRuleState{MaxFileSize: 50}, want: []string{"raises max_file_size from 50 MB to 51 MB"},
		},
		{name: "the size limit resent", edit: PushRuleEdit{MaxFileSize: new(int64(50))}, current: PushRuleState{MaxFileSize: 50}},
		{name: "the size limit lowered", edit: PushRuleEdit{MaxFileSize: new(int64(10))}, current: PushRuleState{MaxFileSize: 50}},
		{name: "a negative size, which GitLab refuses", edit: PushRuleEdit{MaxFileSize: new(int64(-1))}, current: PushRuleState{MaxFileSize: 50}},
		{name: "a negative size where there is no limit", edit: PushRuleEdit{MaxFileSize: new(int64(-1))}},
		{name: "a size the rule does not send", edit: PushRuleEdit{PreventSecrets: new(true)}, current: PushRuleState{MaxFileSize: 50}},
		{
			name:    "one setting relaxed beside one tightened",
			edit:    PushRuleEdit{PreventSecrets: new(true), DenyDeleteTag: new(false), MaxFileSize: new(int64(10))},
			current: PushRuleState{DenyDeleteTag: true, MaxFileSize: 50},
			want:    []string{"turns off deny_delete_tag"},
		},
		{name: "nothing sent", current: everyRuleSet()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := englishChanges(tt.edit.Relaxations(tt.current))
			if strings.Join(got, " | ") != strings.Join(tt.want, " | ") {
				t.Errorf("Relaxations() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPushRuleEdit_EverySetting_IsNamedInAFixedOrder relaxes all thirteen
// settings at once, each in the way its kind is relaxed. The answer names
// every one, by the name GitLab gives it, in the order the question always
// uses, so a setting the rule forgot, or read under another setting's value,
// shows here. The same edit against a rule that could not be read names the
// same thirteen as unread, and the arguments alone say it may relax.
func TestPushRuleEdit_EverySetting_IsNamedInAFixedOrder(t *testing.T) {
	edit := PushRuleEdit{
		DenyDeleteTag: new(false), MemberCheck: new(false), PreventSecrets: new(false),
		CommitCommitterCheck: new(false), CommitCommitterNameCheck: new(false),
		RejectUnsignedCommits: new(false), RejectNonDCOCommits: new(false),
		CommitMessageRegex: new(""), CommitMessageNegativeRegex: new(""),
		BranchNameRegex: new(""), AuthorEmailRegex: new(""), FileNameRegex: new(""),
		MaxFileSize: new(int64(0)),
	}
	settings := []string{
		"deny_delete_tag", "member_check", "prevent_secrets", "commit_committer_check",
		"commit_committer_name_check", "reject_unsigned_commits", "reject_non_dco_commits",
		"commit_message_regex", "commit_message_negative_regex", "branch_name_regex",
		"author_email_regex", "file_name_regex",
	}
	var wantRead, wantUnread []string
	for index, setting := range settings {
		verb := "turns off "
		if index >= 7 {
			verb = "removes "
		}
		wantRead = append(wantRead, verb+setting)
		wantUnread = append(wantUnread, "may relax "+setting+" (the current rules could not be read)")
	}
	wantRead = append(wantRead, "removes the max_file_size limit of 50 MB")
	wantUnread = append(wantUnread, "may relax max_file_size (the current rules could not be read)")

	if got := englishChanges(edit.Relaxations(everyRuleSet())); strings.Join(got, " | ") != strings.Join(wantRead, " | ") {
		t.Errorf("Relaxations() =\n%q\nwant\n%q", got, wantRead)
	}
	if got := englishChanges(edit.UnreadRelaxations()); strings.Join(got, " | ") != strings.Join(wantUnread, " | ") {
		t.Errorf("UnreadRelaxations() =\n%q\nwant\n%q", got, wantUnread)
	}
	if !edit.MayRelax() {
		t.Error("MayRelax() = false for an edit that relaxes every setting")
	}

	// Each pattern, replaced where it was set, is named as replaced.
	replacing := PushRuleEdit{
		CommitMessageRegex: new("a"), CommitMessageNegativeRegex: new("b"),
		BranchNameRegex: new("c"), AuthorEmailRegex: new("d"), FileNameRegex: new("e"),
	}
	var wantReplaced []string
	for _, setting := range settings[7:] {
		wantReplaced = append(wantReplaced, "replaces "+setting+" with another pattern that may accept more")
	}
	if got := englishChanges(replacing.Relaxations(everyRuleSet())); strings.Join(got, " | ") != strings.Join(wantReplaced, " | ") {
		t.Errorf("Relaxations() of five replaced patterns =\n%q\nwant\n%q", got, wantReplaced)
	}
}

// TestPushRuleEdit_MayRelax_ReadsTheArgumentsAlone pins what the arguments
// say without the rule: an edit that only turns switches on can relax
// nothing and is never read for, and a switch turned off, any pattern, empty
// or not, and any max_file_size can. Each setting is tried alone, so one the
// check skipped would be an edit that relaxes it unasked.
func TestPushRuleEdit_MayRelax_ReadsTheArgumentsAlone(t *testing.T) {
	tests := []struct {
		name string
		edit PushRuleEdit
		want bool
	}{
		{name: "nothing sent", edit: PushRuleEdit{}},
		{
			name: "every switch turned on",
			edit: PushRuleEdit{
				DenyDeleteTag: new(true), MemberCheck: new(true), PreventSecrets: new(true),
				CommitCommitterCheck: new(true), CommitCommitterNameCheck: new(true),
				RejectUnsignedCommits: new(true), RejectNonDCOCommits: new(true),
			},
		},
		{name: "deny_delete_tag off", edit: PushRuleEdit{DenyDeleteTag: new(false)}, want: true},
		{name: "member_check off", edit: PushRuleEdit{MemberCheck: new(false)}, want: true},
		{name: "prevent_secrets off", edit: PushRuleEdit{PreventSecrets: new(false)}, want: true},
		{name: "commit_committer_check off", edit: PushRuleEdit{CommitCommitterCheck: new(false)}, want: true},
		{name: "commit_committer_name_check off", edit: PushRuleEdit{CommitCommitterNameCheck: new(false)}, want: true},
		{name: "reject_unsigned_commits off", edit: PushRuleEdit{RejectUnsignedCommits: new(false)}, want: true},
		{name: "reject_non_dco_commits off", edit: PushRuleEdit{RejectNonDCOCommits: new(false)}, want: true},
		{name: "commit_message_regex set", edit: PushRuleEdit{CommitMessageRegex: new("^JIRA-")}, want: true},
		{name: "commit_message_negative_regex emptied", edit: PushRuleEdit{CommitMessageNegativeRegex: new("")}, want: true},
		{name: "branch_name_regex set", edit: PushRuleEdit{BranchNameRegex: new(".*")}, want: true},
		{name: "author_email_regex emptied", edit: PushRuleEdit{AuthorEmailRegex: new("")}, want: true},
		{name: "file_name_regex set", edit: PushRuleEdit{FileNameRegex: new("\\.exe$")}, want: true},
		{name: "max_file_size lowered or raised", edit: PushRuleEdit{MaxFileSize: new(int64(10))}, want: true},
		{name: "max_file_size removed", edit: PushRuleEdit{MaxFileSize: new(int64(0))}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.edit.MayRelax(); got != tt.want {
				t.Errorf("MayRelax() = %v, want %v", got, tt.want)
			}
			if unread := tt.edit.UnreadRelaxations(); (len(unread) > 0) != tt.want {
				t.Errorf("UnreadRelaxations() names %d setting(s), want some exactly when the edit may relax (%v)", len(unread), tt.want)
			}
		})
	}
}

// TestPushRuleEdit_RelaxationsAfterRead_ByWhatTheReadAnswered pins the four
// answers a read can give. A rule is compared with the edit. No rule, which
// GitLab says with a 200 carrying none or with a 404, has nothing to relax,
// so the edit goes on to the refusal GitLab gives the write. Any other
// failure names every setting the edit may relax as unread, since a guard
// that cannot see the rule must not assume the edit is harmless.
func TestPushRuleEdit_RelaxationsAfterRead_ByWhatTheReadAnswered(t *testing.T) {
	edit := PushRuleEdit{PreventSecrets: new(false), MaxFileSize: new(int64(100))}
	set := PushRuleState{PreventSecrets: true, MaxFileSize: 50}
	const unreadSwitch = "may relax prevent_secrets (the current rules could not be read)"
	const unreadSize = "may relax max_file_size (the current rules could not be read)"
	tests := []struct {
		name    string
		current *PushRuleState
		readErr error
		want    []string
	}{
		{name: "a rule", current: &set, want: []string{"turns off prevent_secrets", "raises max_file_size from 50 MB to 100 MB"}},
		{name: "a rule the edit does not relax", current: &PushRuleState{}},
		{name: "no rule", current: nil},
		{name: "404 as client-go names it", readErr: gl.ErrNotFound},
		{name: "404 as a response", readErr: &gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}}},
		{
			name:    "403",
			readErr: &gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusForbidden}},
			want:    []string{unreadSwitch, unreadSize},
		},
		{
			name:    "500",
			readErr: &gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusInternalServerError}},
			want:    []string{unreadSwitch, unreadSize},
		},
		{name: "a failure that is no response", readErr: errors.New("connection refused"), want: []string{unreadSwitch, unreadSize}},
		{
			name: "a failure beside a rule", current: &PushRuleState{}, readErr: errors.New("connection reset"),
			want: []string{unreadSwitch, unreadSize},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := englishChanges(edit.RelaxationsAfterRead(tt.current, tt.readErr))
			if strings.Join(got, " | ") != strings.Join(tt.want, " | ") {
				t.Errorf("RelaxationsAfterRead() = %q, want %q", got, tt.want)
			}
		})
	}
}

// pushRuleCall builds the tool call a guard reads the confirmation from, with
// the arguments as the surface sent them.
func pushRuleCall(arguments string) context.Context {
	return ContextWithRequest(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "gitlab_project_edit_push_rule", Arguments: json.RawMessage(arguments)},
	})
}

// TestPushRuleEditNeedsRead_AConfirmedCallIsNotReadFor pins when the rule
// is read at all: only for an edit that may relax one, and only while nothing
// has confirmed the call. The confirmation is honored in each shape a surface
// sends it in, flat on an individual tool, inside params on a dispatcher and
// beside params on the dynamic one, and the operator's switch under both of
// its names.
func TestPushRuleEditNeedsRead_AConfirmedCallIsNotReadFor(t *testing.T) {
	relaxing := PushRuleEdit{PreventSecrets: new(false)}
	tightening := PushRuleEdit{PreventSecrets: new(true)}
	tests := []struct {
		name string
		edit PushRuleEdit
		ctx  context.Context
		env  map[string]string
		want bool
	}{
		{name: "an edit that may relax, unconfirmed", edit: relaxing, ctx: context.Background(), want: true},
		{name: "an edit that may relax, on a call without confirm", edit: relaxing, ctx: pushRuleCall(`{"project_id":"42","prevent_secrets":false}`), want: true},
		{name: "an edit that may relax, confirm false", edit: relaxing, ctx: pushRuleCall(`{"project_id":"42","confirm":false}`), want: true},
		{name: "an edit that cannot relax", edit: tightening, ctx: context.Background()},
		{name: "confirm flat", edit: relaxing, ctx: pushRuleCall(`{"project_id":"42","confirm":true}`)},
		{name: "confirm inside params", edit: relaxing, ctx: pushRuleCall(`{"action":"push_rule_edit","params":{"project_id":"42","confirm":true}}`)},
		{name: "confirm beside params", edit: relaxing, ctx: pushRuleCall(`{"action":"project.push_rule_edit","params":{"project_id":"42"},"confirm":true}`)},
		{name: "YOLO mode", edit: relaxing, ctx: context.Background(), env: map[string]string{"GITLAB_MCP_YOLO_MODE": "true"}},
		{name: "AUTOPILOT", edit: relaxing, ctx: context.Background(), env: map[string]string{"AUTOPILOT": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", "")
			t.Setenv("AUTOPILOT", "")
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			if got := PushRuleEditNeedsRead(tt.ctx, tt.edit); got != tt.want {
				t.Errorf("PushRuleEditNeedsRead() = %v, want %v", got, tt.want)
			}
		})
	}
}

// pushRuleElicitingCall builds a tool call from a client that supports form
// elicitation over the multi round-trip mechanism, carrying the answers the
// user has already given.
func pushRuleElicitingCall(answers mcp.InputResponseMap) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{
		Session: &mcp.ServerSession{},
		Params: &mcp.CallToolParamsRaw{
			Name:      "gitlab_project_edit_push_rule",
			Arguments: json.RawMessage(`{"project_id":"42","prevent_secrets":false}`),
			Meta: mcp.Meta{
				"io.modelcontextprotocol/protocolVersion": "2026-07-28",
				"io.modelcontextprotocol/clientCapabilities": map[string]any{
					"elicitation": map[string]any{"form": map[string]any{}},
				},
			},
			InputResponses: answers,
		},
	}
}

// pushRuleQuestion is the question the confirmation tests ask about: project
// 42, one switch turned off and the size limit raised.
const pushRuleQuestion = "Relax the push rules of project 42? This call turns off prevent_secrets and raises max_file_size from 50 MB to 100 MB." +
	" Pushes those rules reject today are accepted from then on."

// pushRuleChanges is what the guard found for [pushRuleQuestion].
func pushRuleChanges() []i18n.Message {
	return PushRuleEdit{PreventSecrets: new(false), MaxFileSize: new(int64(100))}.
		Relaxations(PushRuleState{PreventSecrets: true, MaxFileSize: 50})
}

// TestConfirmPushRuleRelaxation_NothingRelaxed_AsksNothing pins that an edit
// the rule found nothing to say about proceeds without a question, on a
// client that could be asked and on one that could not.
func TestConfirmPushRuleRelaxation_NothingRelaxed_AsksNothing(t *testing.T) {
	target := i18n.New(i18n.ConfirmPushRuleProject, i18n.S("id", "42"))
	contexts := map[string]context.Context{
		"a client that cannot be asked": context.Background(),
		"a client that can be asked":    ContextWithRequest(context.Background(), pushRuleElicitingCall(nil)),
	}
	for name, ctx := range contexts {
		t.Run(name, func(t *testing.T) {
			if err := ConfirmPushRuleRelaxation(ctx, "projectEditPushRule", target, nil); err != nil {
				t.Errorf("error = %v, want the edit to proceed", err)
			}
		})
	}
}

// TestConfirmPushRuleRelaxation_WithoutAWayToAsk_RefusesWithTheQuestionInEnglish
// pins the refusal a model reads when nobody can be asked: the operation, the
// question a person would have been asked, word for word and in English, and
// the way to proceed. A confirmation state this server did not issue is
// refused the same way rather than taken as an answer, since the state
// travels through the client.
func TestConfirmPushRuleRelaxation_WithoutAWayToAsk_RefusesWithTheQuestionInEnglish(t *testing.T) {
	corrupt := pushRuleElicitingCall(nil)
	corrupt.Params.RequestState = "not a state this server issued"
	tests := []struct {
		name   string
		ctx    context.Context
		op     string
		target i18n.Message
		want   string
	}{
		{
			name: "no request at all", ctx: context.Background(), op: "projectEditPushRule",
			target: i18n.New(i18n.ConfirmPushRuleProject, i18n.S("id", "42")),
			want:   "projectEditPushRule: " + pushRuleQuestion + " Re-send the same call with confirm=true once the user has approved it",
		},
		{
			name: "a client without elicitation", ctx: pushRuleCall(`{"group_id":"acme"}`), op: "groupEditPushRule",
			target: i18n.New(i18n.ConfirmPushRuleGroup, i18n.S("id", "acme")),
			want: "groupEditPushRule: " + strings.Replace(pushRuleQuestion, "project 42", "group acme", 1) +
				" Re-send the same call with confirm=true once the user has approved it",
		},
		{
			name: "a confirmation state this server did not issue", ctx: ContextWithRequest(context.Background(), corrupt), op: "projectEditPushRule",
			target: i18n.New(i18n.ConfirmPushRuleProject, i18n.S("id", "42")),
			want:   "projectEditPushRule: " + pushRuleQuestion + " Re-send the same call with confirm=true once the user has approved it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConfirmPushRuleRelaxation(tt.ctx, tt.op, tt.target, pushRuleChanges())
			if err == nil || err.Error() != tt.want {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tt.want)
			}
		})
	}
}

// TestConfirmPushRuleRelaxation_ElicitingClient_TheAnswerDecides pins each
// answer a client that can ask may give. No answer yet suspends the call with
// the pending input request. A confirmed form proceeds. A form submitted with
// the box unticked is a decision to keep the rules, answered with the way to
// edit without relaxing. A declined or dismissed dialog, an answer to another
// question and an answer of the wrong shape are failed exchanges, and a
// failed confirmation never proceeds.
func TestConfirmPushRuleRelaxation_ElicitingClient_TheAnswerDecides(t *testing.T) {
	target := i18n.New(i18n.ConfirmPushRuleProject, i18n.S("id", "42"))
	tests := []struct {
		name    string
		answer  *mcp.ElicitResult
		pending bool
		want    string
	}{
		{name: "no answer yet", pending: true},
		{name: "confirmed", answer: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}},
		{
			name: "submitted unticked", answer: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": false}},
			want: "projectEditPushRule: the user declined relaxing the push rules; send only the settings that add or tighten a rule, or leave them as they are",
		},
		{name: "declined", answer: &mcp.ElicitResult{Action: "decline"}, want: "projectEditPushRule: confirmation failed: "},
		{name: "dismissed", answer: &mcp.ElicitResult{Action: "cancel"}, want: "projectEditPushRule: confirmation failed: "},
		{
			name: "an answer of another shape", answer: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"title": "unrelated"}},
			want: "projectEditPushRule: confirmation failed: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var answers mcp.InputResponseMap
			if tt.answer != nil {
				answers = mcp.InputResponseMap{PushRuleRelaxConfirmID: tt.answer}
			}
			ctx := ContextWithRequest(context.Background(), pushRuleElicitingCall(answers))
			err := ConfirmPushRuleRelaxation(ctx, "projectEditPushRule", target, pushRuleChanges())
			if _, pending := InputRequiredResultFromError(err); pending != tt.pending {
				t.Fatalf("error = %v, pending input request %v, want %v", err, pending, tt.pending)
			}
			if tt.pending {
				return
			}
			if tt.want == "" {
				if err != nil {
					t.Errorf("error = %v, want the confirmed edit to proceed", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("error = %v, want one that begins %q", err, tt.want)
			}
		})
	}
}

// TestConfirmPushRuleRelaxation_ThePersonIsAskedInTheirLanguage pins the
// other reader of the question. The pending input request carries it as the
// catalog renders it for the request's language, Spanish here, with each
// change a fragment that continues the sentence, while the refusal a model
// reads on a client that cannot ask stays English.
func TestConfirmPushRuleRelaxation_ThePersonIsAskedInTheirLanguage(t *testing.T) {
	target := i18n.New(i18n.ConfirmPushRuleGroup, i18n.S("id", "acme"))
	ctx := i18n.WithLanguage(ContextWithRequest(context.Background(), pushRuleElicitingCall(nil)), i18n.Spanish)
	err := ConfirmPushRuleRelaxation(ctx, "groupEditPushRule", target, pushRuleChanges())
	result, pending := InputRequiredResultFromError(err)
	if !pending {
		t.Fatalf("error = %v, want the pending input request", err)
	}
	encoded, encodeErr := json.Marshal(result)
	if encodeErr != nil {
		t.Fatalf("encode the input-required result: %v", encodeErr)
	}
	const spanish = "¿Relajar las push rules del grupo acme? Esta llamada desactiva prevent_secrets y sube max_file_size de 50 MB a 100 MB." +
		" Los push que esas reglas rechazan hoy se aceptan desde entonces."
	var decoded struct {
		InputRequests map[string]struct {
			Params struct {
				Message string `json:"message"`
			} `json:"params"`
		} `json:"inputRequests"`
	}
	if decodeErr := json.Unmarshal(encoded, &decoded); decodeErr != nil {
		t.Fatalf("decode the input-required result: %v", decodeErr)
	}
	request, asked := decoded.InputRequests[PushRuleRelaxConfirmID]
	if !asked {
		t.Fatalf("the result asks nothing under %q: %s", PushRuleRelaxConfirmID, encoded)
	}
	if request.Params.Message != spanish {
		t.Errorf("the person is asked\n%q\nwant\n%q", request.Params.Message, spanish)
	}
}
