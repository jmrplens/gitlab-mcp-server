// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// push_rule_guard_test.go holds the edit of a project's push rules to the
// confirmation it owes: none for an edit that adds or tightens a rule, and
// one, before anything is written, for an edit that relaxes a rule in force.
package projects

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// pathProject42PushRule is the route both the guard's read and the edit use.
const pathProject42PushRule = "/api/v4/projects/42/push_rule"

// storedPushRule is a rule in force: one switch on, one pattern set and a
// file size limit, so each way of relaxing a rule has something to relax.
const storedPushRule = `{"id":1,"project_id":42,"prevent_secrets":true,"branch_name_regex":"^(feature|fix)/","max_file_size":50}`

// pushRuleClient answers the project's push rule route and counts what it
// was sent: GET with readStatus and readBody, the rule the guard compares the
// edit with, and PUT with the edited rule.
func pushRuleClient(t *testing.T, calls *guardCalls, readStatus int, readBody string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathProject42PushRule {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			calls.reads.Add(1)
			testutil.RespondJSON(w, readStatus, readBody)
			return
		}
		calls.writes.Add(1)
		testutil.RespondJSON(w, http.StatusOK, storedPushRule)
	}))
}

// pushRuleRefusal is the refusal an edit of project 42 that relaxes a rule is
// answered with when nobody can be asked: the question, naming what the call
// does, and the way to proceed.
func pushRuleRefusal(changes string) string {
	return "projectEditPushRule: Relax the push rules of project 42? This call " + changes + "." +
		" Pushes those rules reject today are accepted from then on." +
		" Re-send the same call with confirm=true once the user has approved it"
}

// TestEditPushRule_RelaxingARuleInForce_IsRefusedBeforeAnyWrite walks each
// way an edit relaxes the rule GitLab holds: a switch turned off, a pattern
// removed, a pattern replaced by another, the size limit raised and the size
// limit removed. Each is refused with the question, word for word, after one
// read and before any write.
func TestEditPushRule_RelaxingARuleInForce_IsRefusedBeforeAnyWrite(t *testing.T) {
	tests := []struct {
		name  string
		input EditPushRuleInput
		want  string
	}{
		{name: "a switch turned off", input: EditPushRuleInput{PreventSecrets: new(false)}, want: "turns off prevent_secrets"},
		{name: "a pattern removed", input: EditPushRuleInput{BranchNameRegex: new("")}, want: "removes branch_name_regex"},
		{
			name: "a pattern replaced", input: EditPushRuleInput{BranchNameRegex: new(".*")},
			want: "replaces branch_name_regex with another pattern that may accept more",
		},
		{name: "the size limit raised", input: EditPushRuleInput{MaxFileSize: new(int64(100000))}, want: "raises max_file_size from 50 MB to 100000 MB"},
		{name: "the size limit removed", input: EditPushRuleInput{MaxFileSize: new(int64(0))}, want: "removes the max_file_size limit of 50 MB"},
		{
			name:  "two rules relaxed beside one tightened",
			input: EditPushRuleInput{PreventSecrets: new(false), MaxFileSize: new(int64(0)), DenyDeleteTag: new(true)},
			want:  "turns off prevent_secrets and removes the max_file_size limit of 50 MB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := pushRuleClient(t, &calls, http.StatusOK, storedPushRule)
			tt.input.ProjectID = "42"

			_, err := EditPushRule(t.Context(), client, tt.input)
			if want := pushRuleRefusal(tt.want); err == nil || err.Error() != want {
				t.Errorf("refusal =\n%v\nwant\n%q", err, want)
			}
			if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != 1 || writes != 0 {
				t.Errorf("GitLab was sent %d read(s) and %d write(s), want 1 and 0", reads, writes)
			}
		})
	}
}

// TestEditPushRule_AddingOrTighteningARule_AsksNothing is the other half: an
// edit that turns rules on is written without a read, since no rule can make
// it a relaxation, and an edit whose answer depends on the rule is read for
// once and then written, when the rule says it adds a restriction, tightens
// one or resends what is there. A prompt raised by these would be one a
// caller learns to pass through.
func TestEditPushRule_AddingOrTighteningARule_AsksNothing(t *testing.T) {
	tests := []struct {
		name      string
		input     EditPushRuleInput
		wantReads int64
	}{
		{name: "switches turned on", input: EditPushRuleInput{PreventSecrets: new(true), DenyDeleteTag: new(true), MemberCheck: new(true)}},
		{name: "a switch turned off that is off", input: EditPushRuleInput{DenyDeleteTag: new(false)}, wantReads: 1},
		{name: "a pattern set where there was none", input: EditPushRuleInput{CommitMessageRegex: new("^JIRA-")}, wantReads: 1},
		{name: "a pattern emptied that is empty", input: EditPushRuleInput{FileNameRegex: new("")}, wantReads: 1},
		{name: "a pattern resent as it is", input: EditPushRuleInput{BranchNameRegex: new("^(feature|fix)/")}, wantReads: 1},
		{name: "the size limit lowered", input: EditPushRuleInput{MaxFileSize: new(int64(10))}, wantReads: 1},
		{name: "the size limit resent", input: EditPushRuleInput{MaxFileSize: new(int64(50))}, wantReads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := pushRuleClient(t, &calls, http.StatusOK, storedPushRule)
			tt.input.ProjectID = "42"

			if _, err := EditPushRule(t.Context(), client, tt.input); err != nil {
				t.Fatalf("EditPushRule() error = %v, want the edit applied without a question", err)
			}
			if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != tt.wantReads || writes != 1 {
				t.Errorf("GitLab was sent %d read(s) and %d write(s), want %d and 1", reads, writes, tt.wantReads)
			}
		})
	}
}

// TestEditPushRule_ByWhatTheReadAnswers pins the read's other answers for an
// edit that would relax a rule if one were in force. A project without push
// rules, which GitLab answers 200 with no rule, and a 404 have nothing to
// relax: the edit is sent and GitLab refuses it for the missing rule. A read
// GitLab refuses or fails is answered by asking about every setting the edit
// may relax, since a guard that cannot see the rule must not assume the edit
// is harmless.
func TestEditPushRule_ByWhatTheReadAnswers(t *testing.T) {
	const unread = "may relax prevent_secrets (the current rules could not be read)" +
		" and may relax max_file_size (the current rules could not be read)"
	tests := []struct {
		name       string
		readStatus int
		readBody   string
		wantWrites int64
		refusal    string
	}{
		{name: "a project without push rules", readStatus: http.StatusOK, readBody: "null", wantWrites: 1},
		{name: "404", readStatus: http.StatusNotFound, readBody: `{"message":"404 Project Not Found"}`, wantWrites: 1},
		{name: "403", readStatus: http.StatusForbidden, readBody: `{"message":"403 Forbidden"}`, refusal: pushRuleRefusal(unread)},
		{name: "500", readStatus: http.StatusInternalServerError, readBody: `{"message":"500 Internal Server Error"}`, refusal: pushRuleRefusal(unread)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := pushRuleClient(t, &calls, tt.readStatus, tt.readBody)

			_, err := EditPushRule(t.Context(), client, EditPushRuleInput{
				ProjectID: "42", PreventSecrets: new(false), MaxFileSize: new(int64(100)),
			})
			if tt.refusal == "" && err != nil {
				t.Fatalf("EditPushRule() error = %v, want the edit sent", err)
			}
			if tt.refusal != "" && (err == nil || err.Error() != tt.refusal) {
				t.Errorf("refusal =\n%v\nwant\n%q", err, tt.refusal)
			}
			if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != 1 || writes != tt.wantWrites {
				t.Errorf("GitLab was sent %d read(s) and %d write(s), want 1 and %d", reads, writes, tt.wantWrites)
			}
		})
	}
}

// TestEditPushRule_Confirmed_IsWrittenWithoutARead pins the two ways through: a
// call that carries confirm=true, in the shape each surface sends it, and a
// server whose operator switched confirmation off. Neither reads the rule,
// since nobody is going to be asked what the call relaxes, and both write.
func TestEditPushRule_Confirmed_IsWrittenWithoutARead(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		yolo      bool
	}{
		{name: "confirm flat", arguments: `{"project_id":"42","prevent_secrets":false,"confirm":true}`},
		{name: "confirm inside params", arguments: `{"action":"push_rule_edit","params":{"project_id":"42","prevent_secrets":false,"confirm":true}}`},
		{name: "YOLO mode", arguments: `{"project_id":"42","prevent_secrets":false}`, yolo: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.yolo {
				t.Setenv("GITLAB_MCP_YOLO_MODE", "true")
			}
			var calls guardCalls
			client := pushRuleClient(t, &calls, http.StatusOK, storedPushRule)
			ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: toolProjectEditPushRule, Arguments: json.RawMessage(tt.arguments)},
			})

			if _, err := EditPushRule(ctx, client, EditPushRuleInput{ProjectID: "42", PreventSecrets: new(false)}); err != nil {
				t.Fatalf("EditPushRule() error = %v, want the confirmed edit applied", err)
			}
			if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != 0 || writes != 1 {
				t.Errorf("GitLab was sent %d read(s) and %d write(s), want 0 and 1", reads, writes)
			}
		})
	}
}

// pushRuleElicitingRequest builds the edit as a client that supports form
// elicitation over the multi round-trip mechanism sends it, carrying the
// answers the user has already given.
func pushRuleElicitingRequest(answers mcp.InputResponseMap) *mcp.CallToolRequest {
	req := elicitingRequest(answers)
	req.Params.Name = toolProjectEditPushRule
	req.Params.Arguments = json.RawMessage(`{"project_id":"42","prevent_secrets":false}`)
	return req
}

// TestEditPushRule_ElicitingClient_TheAnswerDecidesTheCall pins the client
// that can ask. The first pass suspends the call with the pending question
// and writes nothing, an approval writes once, and a refusal writes nothing
// and says how to edit without relaxing.
func TestEditPushRule_ElicitingClient_TheAnswerDecidesTheCall(t *testing.T) {
	tests := []struct {
		name       string
		answers    mcp.InputResponseMap
		pending    bool
		wantWrites int64
		wantError  string
	}{
		{name: "not answered yet", pending: true},
		{
			name: "approved", wantWrites: 1,
			answers: mcp.InputResponseMap{toolutil.PushRuleRelaxConfirmID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}},
		},
		{
			name:      "refused",
			answers:   mcp.InputResponseMap{toolutil.PushRuleRelaxConfirmID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": false}}},
			wantError: "projectEditPushRule: the user declined relaxing the push rules; send only the settings that add or tighten a rule, or leave them as they are",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := pushRuleClient(t, &calls, http.StatusOK, storedPushRule)
			ctx := toolutil.ContextWithRequest(t.Context(), pushRuleElicitingRequest(tt.answers))

			_, err := EditPushRule(ctx, client, EditPushRuleInput{ProjectID: "42", PreventSecrets: new(false)})
			if _, pending := toolutil.InputRequiredResultFromError(err); pending != tt.pending {
				t.Fatalf("error = %v, pending input request %v, want %v", err, pending, tt.pending)
			}
			if !tt.pending && tt.wantError == "" && err != nil {
				t.Fatalf("EditPushRule() error = %v, want the approved edit applied", err)
			}
			if tt.wantError != "" && (err == nil || err.Error() != tt.wantError) {
				t.Errorf("error =\n%v\nwant\n%q", err, tt.wantError)
			}
			if got := calls.writes.Load(); got != tt.wantWrites {
				t.Errorf("writes = %d, want %d", got, tt.wantWrites)
			}
		})
	}
}

// TestEditPushRule_ThroughTheRoute_HonorsTheReservedConfirmKey drives the
// path a surface takes: the route's own handler, given the parameters as a
// map with the reserved key among them and the raw call in the context. The
// typed input is unmarshalled strictly, so a confirm key the struct did not
// declare would be refused as an unknown field before the handler ran, and
// the guard reads the confirmation from the raw call rather than from that
// map. The unconfirmed half is beside it, since a key that is accepted and
// never consulted would pass the first half alone. The action is not
// destructive: the gate that would ask before every edit is not what asks.
func TestEditPushRule_ThroughTheRoute_HonorsTheReservedConfirmKey(t *testing.T) {
	tests := []struct {
		name       string
		confirm    bool
		wantWrites int64
	}{
		{name: "confirmed", confirm: true, wantWrites: 1},
		{name: "unconfirmed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := pushRuleClient(t, &calls, http.StatusOK, storedPushRule)
			spec := projectSpecsByTool(t, ActionSpecs(client, true))[toolProjectEditPushRule]
			if spec.Destructive || spec.Route.Destructive {
				t.Fatal("project.push_rule_edit is destructive, so the destructive gate would ask before an edit that tightens a rule")
			}

			params := map[string]any{"project_id": "42", "max_file_size": 100}
			if tt.confirm {
				params["confirm"] = true
			}
			arguments, marshalErr := json.Marshal(params)
			if marshalErr != nil {
				t.Fatalf("marshal params: %v", marshalErr)
			}
			ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: toolProjectEditPushRule, Arguments: arguments},
			})

			_, err := spec.Route.Handler(ctx, params)
			if tt.confirm && err != nil {
				t.Fatalf("a confirmed edit was refused: %v", err)
			}
			if want := pushRuleRefusal("raises max_file_size from 50 MB to 100 MB"); !tt.confirm && (err == nil || err.Error() != want) {
				t.Errorf("refusal =\n%v\nwant\n%q", err, want)
			}
			if got := calls.writes.Load(); got != tt.wantWrites {
				t.Errorf("writes = %d, want %d", got, tt.wantWrites)
			}
		})
	}
}

// TestEditPushRule_TheRead_CarriesTheCallersContext verifies that the guard's
// read is sent with the caller's context, so an abandoned call cancels it,
// and that a call abandoned during the read is answered with the context's
// error rather than with a question nobody is there to answer.
func TestEditPushRule_TheRead_CarriesTheCallersContext(t *testing.T) {
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, storedPushRule)
	})

	_, err := EditPushRule(ctx, client, EditPushRuleInput{ProjectID: "42", PreventSecrets: new(false)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EditPushRule() error = %v, want context.Canceled", err)
	}
}

// TestPushRuleState_CarriesEverySettingOfTheRule holds the two restatements
// the guard reads through to the thirteen settings: a rule with every setting
// restricting something arrives in the shared rule with each one under its
// own name, so an edit relaxing all thirteen is asked about all thirteen, and
// a rule GitLab did not send is no rule rather than an empty one.
func TestPushRuleState_CarriesEverySettingOfTheRule(t *testing.T) {
	const everySetting = `{"id":1,"project_id":42,"deny_delete_tag":true,"member_check":true,"prevent_secrets":true,` +
		`"commit_committer_check":true,"commit_committer_name_check":true,"reject_unsigned_commits":true,"reject_non_dco_commits":true,` +
		`"commit_message_regex":"^JIRA-","commit_message_negative_regex":"WIP","branch_name_regex":"^feature/",` +
		`"author_email_regex":"@example\\.com$","file_name_regex":"\\.exe$","max_file_size":50}`
	var calls guardCalls
	client := pushRuleClient(t, &calls, http.StatusOK, everySetting)

	_, err := EditPushRule(t.Context(), client, EditPushRuleInput{
		ProjectID:     "42",
		DenyDeleteTag: new(false), MemberCheck: new(false), PreventSecrets: new(false),
		CommitCommitterCheck: new(false), CommitCommitterNameCheck: new(false),
		RejectUnsignedCommits: new(false), RejectNonDCOCommits: new(false),
		CommitMessageRegex: new(""), CommitMessageNegativeRegex: new(""),
		BranchNameRegex: new(""), AuthorEmailRegex: new(""), FileNameRegex: new(""),
		MaxFileSize: new(int64(0)),
	})
	want := pushRuleRefusal(strings.Join([]string{
		"turns off deny_delete_tag", "turns off member_check", "turns off prevent_secrets",
		"turns off commit_committer_check", "turns off commit_committer_name_check", "turns off reject_unsigned_commits",
		"turns off reject_non_dco_commits", "removes commit_message_regex", "removes commit_message_negative_regex",
		"removes branch_name_regex", "removes author_email_regex", "removes file_name_regex",
		"removes the max_file_size limit of 50 MB",
	}, " and "))
	if err == nil || err.Error() != want {
		t.Errorf("refusal =\n%v\nwant\n%q", err, want)
	}
	if got := pushRuleState(nil); got != nil {
		t.Errorf("pushRuleState(nil) = %+v, want no rule", got)
	}
}
