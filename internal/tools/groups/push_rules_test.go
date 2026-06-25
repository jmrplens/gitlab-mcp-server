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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
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
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %q: %s", want, gotBody)
		}
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
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %q: %s", want, gotBody)
		}
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
