// push_rules_test.go contains unit tests for the group push-rule MCP tool
// handlers. Tests use httptest to mock GitLab API responses and verify request
// method/path/body, output parsing, and error paths.
package groups

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const pathGroupPushRule = "/api/v4/groups/99/push_rule"

var groupPushRuleJSON = `{"id":7,"commit_message_regex":"^JIRA-","commit_message_negative_regex":"WIP","branch_name_regex":"^(feature|bugfix)/","author_email_regex":"@example.com$","file_name_regex":"\\.exe$","max_file_size":100,"deny_delete_tag":true,"member_check":true,"prevent_secrets":true,"commit_committer_check":true,"commit_committer_name_check":false,"reject_unsigned_commits":true,"reject_non_dco_commits":false,"created_at":"2026-01-15T10:00:00Z"}`

// TestGetPushRules_Success verifies GetPushRules issues a GET to the push_rule
// endpoint and maps every gl.GroupPushRules field into the output.
func TestGetPushRules_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupPushRule {
			testutil.RespondJSON(w, http.StatusOK, groupPushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetPushRules(context.Background(), client, GetPushRulesInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("GetPushRules() unexpected error: %v", err)
	}
	if out.ID != 7 || out.CommitMessageRegex != "^JIRA-" || out.MaxFileSize != 100 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !out.DenyDeleteTag || !out.MemberCheck || !out.PreventSecrets || !out.CommitCommitterCheck || !out.RejectUnsignedCommits {
		t.Errorf("expected boolean flags set: %+v", out)
	}
	if out.CreatedAt == "" {
		t.Error("expected created_at to be populated")
	}
}

// TestGetPushRules_RequiresGroupID verifies the required-input guard.
func TestGetPushRules_RequiresGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if _, err := GetPushRules(context.Background(), client, GetPushRulesInput{}); err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestGetPushRules_NotFound verifies a 404 produces a feature/Premium hint.
func TestGetPushRules_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := GetPushRules(context.Background(), client, GetPushRulesInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "Premium") {
		t.Fatalf("expected Premium hint, got: %v", err)
	}
}

// TestGetPushRules_Forbidden verifies that a non-404 API failure (403) falls
// through to the generic Owner/Premium hint branch instead of the 404-specific
// "no push rules configured" hint. Regressing this branch (e.g. dropping the
// WrapErrWithStatusHint call) would still return a non-nil error, so this
// checks the wrapped message content rather than err != nil alone.
func TestGetPushRules_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := GetPushRules(context.Background(), client, GetPushRulesInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "groupGetPushRules") {
		t.Errorf("expected operation name in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Owner role") {
		t.Errorf("expected Owner-role hint, got: %v", err)
	}
}

// TestAddPushRule_Success verifies AddPushRule POSTs the configured settings.
func TestAddPushRule_Success(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupPushRule {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, groupPushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	deny := true
	maxSize := int64(50)
	out, err := AddPushRule(context.Background(), client, AddPushRuleInput{
		GroupID:            "99",
		CommitMessageRegex: "^JIRA-",
		DenyDeleteTag:      &deny,
		MaxFileSize:        &maxSize,
	})
	if err != nil {
		t.Fatalf("AddPushRule() unexpected error: %v", err)
	}
	if out.ID != 7 {
		t.Fatalf("unexpected output ID: %+v", out)
	}
	if !strings.Contains(gotBody, "commit_message_regex") || !strings.Contains(gotBody, "deny_delete_tag") {
		t.Fatalf("request body missing fields: %s", gotBody)
	}
}

// TestAddPushRule_RequiresSetting verifies add rejects an empty settings payload.
func TestAddPushRule_RequiresSetting(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if _, err := AddPushRule(context.Background(), client, AddPushRuleInput{GroupID: "99"}); err == nil {
		t.Fatal("expected error when no push rule setting supplied")
	}
}

// TestAddPushRule_RequiresGroupID verifies the group_id guard.
func TestAddPushRule_RequiresGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if _, err := AddPushRule(context.Background(), client, AddPushRuleInput{CommitMessageRegex: "x"}); err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestAddPushRule_Conflict verifies a 422 produces an already-exists hint.
func TestAddPushRule_Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	_, err := AddPushRule(context.Background(), client, AddPushRuleInput{GroupID: "99", CommitMessageRegex: "x"})
	if err == nil || !strings.Contains(err.Error(), "already exist") {
		t.Fatalf("expected already-exist hint, got: %v", err)
	}
}

// TestAddPushRule_Forbidden verifies that a non-422/400 API failure (403)
// falls through to the generic Owner/Premium hint branch instead of the
// already-exists/invalid-regex hint. Regressing this branch would still
// return a non-nil error, so this checks the wrapped message content.
func TestAddPushRule_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := AddPushRule(context.Background(), client, AddPushRuleInput{GroupID: "99", CommitMessageRegex: "x"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "groupAddPushRule") {
		t.Errorf("expected operation name in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Owner role") {
		t.Errorf("expected Owner-role hint, got: %v", err)
	}
}

// TestEditPushRule_Success verifies EditPushRule PUTs the changed settings.
func TestEditPushRule_Success(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupPushRule {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, groupPushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	prevent := true
	regex := "^updated-"
	out, err := EditPushRule(context.Background(), client, EditPushRuleInput{
		GroupID:            "99",
		PreventSecrets:     &prevent,
		CommitMessageRegex: &regex,
	})
	if err != nil {
		t.Fatalf("EditPushRule() unexpected error: %v", err)
	}
	if out.ID != 7 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !strings.Contains(gotBody, "prevent_secrets") || !strings.Contains(gotBody, "updated-") {
		t.Fatalf("request body missing fields: %s", gotBody)
	}
}

// TestEditPushRule_RequiresGroupID verifies the group_id guard.
func TestEditPushRule_RequiresGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if _, err := EditPushRule(context.Background(), client, EditPushRuleInput{}); err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestEditPushRule_InvalidRegex verifies a 422 response is reported as an
// invalid-regex/Premium hint rather than falling through to the generic
// add-first (404) hint. This is the branch a caller hits when they submit a
// malformed regex, so losing the WrapErrWithHint call here would silently
// downgrade a validation error into an unhelpful "no push rules exist" hint.
func TestEditPushRule_InvalidRegex(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	regex := "(unterminated"
	_, err := EditPushRule(context.Background(), client, EditPushRuleInput{GroupID: "99", CommitMessageRegex: &regex})
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
	if !strings.Contains(err.Error(), "groupEditPushRule") {
		t.Errorf("expected operation name in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "regex") {
		t.Errorf("expected invalid-regex hint, got: %v", err)
	}
	if strings.Contains(err.Error(), "add_push_rule") {
		t.Errorf("422 should not use the 404 add-first hint, got: %v", err)
	}
}

// TestEditPushRule_NotFound verifies a 404 produces an add-first hint.
func TestEditPushRule_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := EditPushRule(context.Background(), client, EditPushRuleInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "add_push_rule") {
		t.Fatalf("expected add-first hint, got: %v", err)
	}
}

// TestDeletePushRule_Success verifies DeletePushRule DELETEs the endpoint.
func TestDeletePushRule_Success(t *testing.T) {
	var hit bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGroupPushRule {
			hit = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	if err := DeletePushRule(context.Background(), client, DeletePushRuleInput{GroupID: "99"}); err != nil {
		t.Fatalf("DeletePushRule() unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("expected DELETE request to push_rule endpoint")
	}
}

// TestDeletePushRule_RequiresGroupID verifies the group_id guard.
func TestDeletePushRule_RequiresGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if err := DeletePushRule(context.Background(), client, DeletePushRuleInput{}); err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestDeletePushRuleOutput_Success verifies the void wrapper emits a confirmation.
func TestDeletePushRuleOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	out, err := DeletePushRuleOutput(context.Background(), client, DeletePushRuleInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("DeletePushRuleOutput() unexpected error: %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "99") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestFormatPushRuleMarkdown verifies the Markdown formatter renders regex,
// flags, and file-size fields.
func TestFormatPushRuleMarkdown(t *testing.T) {
	var r PushRuleOutput
	if err := json.Unmarshal([]byte(groupPushRuleJSON), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	md := FormatPushRuleMarkdown(r)
	for _, want := range []string{"Group Push Rules", "Commit Message Regex", "^JIRA-", "Prevent Secrets", "Max File Size"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestAddPushRule_AllFields exercises every optional setter in applyAddPushRuleOptions.
func TestAddPushRule_AllFields(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		testutil.RespondJSON(w, http.StatusCreated, groupPushRuleJSON)
	}))
	tt := true
	ff := false
	sz := int64(25)
	_, err := AddPushRule(context.Background(), client, AddPushRuleInput{
		GroupID:                    "99",
		AuthorEmailRegex:           "@x.com",
		BranchNameRegex:            "^feat/",
		CommitCommitterCheck:       &tt,
		CommitCommitterNameCheck:   &ff,
		CommitMessageNegativeRegex: "WIP",
		CommitMessageRegex:         "^A-",
		DenyDeleteTag:              &tt,
		FileNameRegex:              "\\.exe$",
		MaxFileSize:                &sz,
		MemberCheck:                &tt,
		PreventSecrets:             &tt,
		RejectUnsignedCommits:      &tt,
		RejectNonDCOCommits:        &ff,
	})
	if err != nil {
		t.Fatalf("AddPushRule() error: %v", err)
	}
	for _, want := range []string{"author_email_regex", "branch_name_regex", "commit_committer_check", "file_name_regex", "max_file_size", "member_check", "reject_unsigned_commits"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotBody, want) {
				t.Errorf("body missing %q: %s", want, gotBody)
			}
		})
	}
}

// TestEditPushRule_AllFields exercises every optional setter in applyEditPushRuleOptions.
func TestEditPushRule_AllFields(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, groupPushRuleJSON)
	}))
	tt := true
	ff := false
	sz := int64(25)
	s := func(v string) *string { return &v }
	_, err := EditPushRule(context.Background(), client, EditPushRuleInput{
		GroupID:                    "99",
		AuthorEmailRegex:           s("@x.com"),
		BranchNameRegex:            s("^feat/"),
		CommitCommitterCheck:       &tt,
		CommitCommitterNameCheck:   &ff,
		CommitMessageNegativeRegex: s("WIP"),
		CommitMessageRegex:         s("^A-"),
		DenyDeleteTag:              &tt,
		FileNameRegex:              s("\\.exe$"),
		MaxFileSize:                &sz,
		MemberCheck:                &tt,
		PreventSecrets:             &tt,
		RejectUnsignedCommits:      &tt,
		RejectNonDCOCommits:        &ff,
	})
	if err != nil {
		t.Fatalf("EditPushRule() error: %v", err)
	}
	for _, want := range []string{"author_email_regex", "branch_name_regex", "commit_committer_check", "file_name_regex", "max_file_size", "member_check", "reject_unsigned_commits"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotBody, want) {
				t.Errorf("body missing %q: %s", want, gotBody)
			}
		})
	}
}

// TestPushRules_CanceledContext verifies each handler honors a canceled context.
func TestPushRules_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetPushRules(ctx, client, GetPushRulesInput{GroupID: "99"}); err == nil {
		t.Error("GetPushRules: expected context error")
	}
	if _, err := AddPushRule(ctx, client, AddPushRuleInput{GroupID: "99", CommitMessageRegex: "x"}); err == nil {
		t.Error("AddPushRule: expected context error")
	}
	if _, err := EditPushRule(ctx, client, EditPushRuleInput{GroupID: "99"}); err == nil {
		t.Error("EditPushRule: expected context error")
	}
	if err := DeletePushRule(ctx, client, DeletePushRuleInput{GroupID: "99"}); err == nil {
		t.Error("DeletePushRule: expected context error")
	}
}

// TestPushRuleWrites_UnprocessableCarriesTheSameHintAsBadRequest verifies the
// add and edit handlers answer both statuses GitLab rejects a push rule with
// using the same hint, since only one of the two is reachable per instance and
// the caller needs the explanation either way.
func TestPushRuleWrites_UnprocessableCarriesTheSameHintAsBadRequest(t *testing.T) {
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusBadRequest} {
		for name, call := range map[string]func(*gitlabclient.Client) error{
			"add": func(client *gitlabclient.Client) error {
				_, err := AddPushRule(context.Background(), client, AddPushRuleInput{GroupID: "99", CommitMessageRegex: "^JIRA-"})
				return err
			},
			"edit": func(client *gitlabclient.Client) error {
				_, err := EditPushRule(context.Background(), client, EditPushRuleInput{GroupID: "99", CommitMessageRegex: new("^JIRA-")})
				return err
			},
		} {
			t.Run(name+"_"+http.StatusText(status), func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, status, `{"message":"rejected"}`)
				}))
				if err := call(client); err == nil || !strings.Contains(err.Error(), "regex") {
					t.Errorf("%s on a %d = %v, want the regex hint", name, status, err)
				}
			})
		}
	}
}

// TestPushRuleToOutput_CreatedAtOnlyWhenGitLabSentOne verifies a rule with a
// creation date publishes it and one without publishes nothing there, rather
// than the zero time formatted as a date.
func TestPushRuleToOutput_CreatedAtOnlyWhenGitLabSentOne(t *testing.T) {
	created := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	withDate := pushRuleOutputFromGL(&gl.GroupPushRules{ID: 1, CreatedAt: &created})
	if withDate.CreatedAt == "" {
		t.Error("a rule with a creation date published none")
	}
	withoutDate := pushRuleOutputFromGL(&gl.GroupPushRules{ID: 1})
	if withoutDate.CreatedAt != "" {
		t.Errorf("a rule with no creation date published %q", withoutDate.CreatedAt)
	}
}

// TestHasAddPushRuleSetting_EverySettingCountsOnItsOwn verifies each of the
// thirteen settings makes the input a push rule worth sending, and that an
// input naming none of them does not.
//
// The predicate is what stops the handler POSTing an empty rule, so a term
// dropped from the chain would silently discard the one setting a caller
// asked for; only naming each of them alone can tell.
func TestHasAddPushRuleSetting_EverySettingCountsOnItsOwn(t *testing.T) {
	for name, input := range map[string]AddPushRuleInput{
		"author_email_regex":            {AuthorEmailRegex: "@example.com$"},
		"branch_name_regex":             {BranchNameRegex: "^feature/"},
		"commit_committer_check":        {CommitCommitterCheck: new(true)},
		"commit_committer_name_check":   {CommitCommitterNameCheck: new(true)},
		"commit_message_negative_regex": {CommitMessageNegativeRegex: "WIP"},
		"commit_message_regex":          {CommitMessageRegex: "^JIRA-"},
		"deny_delete_tag":               {DenyDeleteTag: new(true)},
		"file_name_regex":               {FileNameRegex: `\.exe$`},
		"max_file_size":                 {MaxFileSize: new(int64(10))},
		"member_check":                  {MemberCheck: new(true)},
		"prevent_secrets":               {PreventSecrets: new(true)},
		"reject_unsigned_commits":       {RejectUnsignedCommits: new(true)},
		"reject_non_dco_commits":        {RejectNonDCOCommits: new(true)},
	} {
		t.Run(name, func(t *testing.T) {
			if !hasAddPushRuleSetting(input) {
				t.Errorf("%s alone did not count as a push-rule setting", name)
			}
		})
	}
	if hasAddPushRuleSetting(AddPushRuleInput{GroupID: "99"}) {
		t.Error("an input naming no setting counted as a push rule")
	}
}

// TestApplyAddPushRuleOptions_CarriesEachSettingOnlyWhenGiven verifies every
// setting the input names reaches the SDK options and every one it does not
// leaves the option unset, so a rule created with one setting does not carry
// twelve zero values GitLab would apply.
func TestApplyAddPushRuleOptions_CarriesEachSettingOnlyWhenGiven(t *testing.T) {
	full := &gl.AddGroupPushRuleOptions{}
	applyAddPushRuleOptions(AddPushRuleInput{
		AuthorEmailRegex: "@example.com$", BranchNameRegex: "^feature/",
		CommitCommitterCheck: new(true), CommitCommitterNameCheck: new(true),
		CommitMessageNegativeRegex: "WIP", CommitMessageRegex: "^JIRA-",
		DenyDeleteTag: new(true), FileNameRegex: `\.exe$`, MaxFileSize: new(int64(10)),
		MemberCheck: new(true), PreventSecrets: new(true),
		RejectUnsignedCommits: new(true), RejectNonDCOCommits: new(true),
	}, full)
	for name, set := range map[string]bool{
		"author_email_regex":            full.AuthorEmailRegex != nil,
		"branch_name_regex":             full.BranchNameRegex != nil,
		"commit_committer_check":        full.CommitCommitterCheck != nil,
		"commit_committer_name_check":   full.CommitCommitterNameCheck != nil,
		"commit_message_negative_regex": full.CommitMessageNegativeRegex != nil,
		"commit_message_regex":          full.CommitMessageRegex != nil,
		"deny_delete_tag":               full.DenyDeleteTag != nil,
		"file_name_regex":               full.FileNameRegex != nil,
		"max_file_size":                 full.MaxFileSize != nil,
		"member_check":                  full.MemberCheck != nil,
		"prevent_secrets":               full.PreventSecrets != nil,
		"reject_unsigned_commits":       full.RejectUnsignedCommits != nil,
		"reject_non_dco_commits":        full.RejectNonDCOCommits != nil,
	} {
		t.Run(name, func(t *testing.T) {
			if !set {
				t.Errorf("%s was dropped on the way to the options", name)
			}
		})
	}

	bare := &gl.AddGroupPushRuleOptions{}
	applyAddPushRuleOptions(AddPushRuleInput{GroupID: "99"}, bare)
	if *bare != (gl.AddGroupPushRuleOptions{}) {
		t.Errorf("options = %+v, want nothing set from an input naming no setting", bare)
	}
}

// TestApplyEditPushRuleOptions_CarriesEachSettingOnlyWhenGiven verifies the
// same for the edit half, where every input field is a pointer so that a
// setting can be turned off as well as on: an omitted field must stay omitted
// rather than being sent as false.
func TestApplyEditPushRuleOptions_CarriesEachSettingOnlyWhenGiven(t *testing.T) {
	full := &gl.EditGroupPushRuleOptions{}
	applyEditPushRuleOptions(EditPushRuleInput{
		AuthorEmailRegex: new("@example.com$"), BranchNameRegex: new("^feature/"),
		CommitCommitterCheck: new(true), CommitCommitterNameCheck: new(true),
		CommitMessageNegativeRegex: new("WIP"), CommitMessageRegex: new("^JIRA-"),
		DenyDeleteTag: new(false), FileNameRegex: new(`\.exe$`), MaxFileSize: new(int64(10)),
		MemberCheck: new(false), PreventSecrets: new(false),
		RejectUnsignedCommits: new(false), RejectNonDCOCommits: new(false),
	}, full)
	for name, set := range map[string]bool{
		"author_email_regex":            full.AuthorEmailRegex != nil,
		"branch_name_regex":             full.BranchNameRegex != nil,
		"commit_committer_check":        full.CommitCommitterCheck != nil,
		"commit_committer_name_check":   full.CommitCommitterNameCheck != nil,
		"commit_message_negative_regex": full.CommitMessageNegativeRegex != nil,
		"commit_message_regex":          full.CommitMessageRegex != nil,
		"deny_delete_tag":               full.DenyDeleteTag != nil,
		"file_name_regex":               full.FileNameRegex != nil,
		"max_file_size":                 full.MaxFileSize != nil,
		"member_check":                  full.MemberCheck != nil,
		"prevent_secrets":               full.PreventSecrets != nil,
		"reject_unsigned_commits":       full.RejectUnsignedCommits != nil,
		"reject_non_dco_commits":        full.RejectNonDCOCommits != nil,
	} {
		t.Run(name, func(t *testing.T) {
			if !set {
				t.Errorf("%s was dropped on the way to the options", name)
			}
		})
	}

	bare := &gl.EditGroupPushRuleOptions{}
	applyEditPushRuleOptions(EditPushRuleInput{GroupID: "99"}, bare)
	if *bare != (gl.EditGroupPushRuleOptions{}) {
		t.Errorf("options = %+v, want nothing set from an input naming no setting", bare)
	}
}

// TestFormatPushRuleMarkdown_OptionalLinesAppearOnlyWhenSet verifies the
// rendered rule carries every regex, the file-size cap and the creation date
// when the rule has them, and none of those lines when it does not, so an
// unset rule does not read as one that forbids everything.
func TestFormatPushRuleMarkdown_OptionalLinesAppearOnlyWhenSet(t *testing.T) {
	full := FormatPushRuleMarkdown(PushRuleOutput{
		ID: 1, CommitMessageRegex: "^JIRA-", CommitMessageNegativeRegex: "WIP",
		BranchNameRegex: "^feature/", AuthorEmailRegex: "@example.com$", FileNameRegex: `\.exe$`,
		MaxFileSize: 10, CreatedAt: "2026-06-15T10:30:00Z",
	})
	for _, want := range []string{
		"Commit Message Regex", "Commit Message Negative Regex", "Branch Name Regex",
		"Author Email Regex", "File Name Regex", "Max File Size", "15 Jun 2026",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(full, want) {
				t.Errorf("markdown missing %q:\n%s", want, full)
			}
		})
	}

	bare := FormatPushRuleMarkdown(PushRuleOutput{ID: 1})
	for _, absent := range []string{
		"Commit Message Regex", "Commit Message Negative Regex", "Branch Name Regex",
		"Author Email Regex", "File Name Regex", "Max File Size", "Created",
	} {
		t.Run("without "+absent, func(t *testing.T) {
			if strings.Contains(bare, absent) {
				t.Errorf("markdown carries %q for a rule that has none:\n%s", absent, bare)
			}
		})
	}
}
