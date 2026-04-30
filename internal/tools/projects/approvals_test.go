// approvals_test.go contains unit tests for project approval configuration
// and approval rule operations.
package projects

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// Test paths for approval operations.
const (
	pathProject42Approvals      = "/api/v4/projects/42/approvals"
	pathProject42ApprovalRules  = "/api/v4/projects/42/approval_rules"
	pathProject42ApprovalRule10 = "/api/v4/projects/42/approval_rules/10"

	approvalConfigJSON = `{
		"approvals_before_merge":2,
		"reset_approvals_on_push":true,
		"disable_overriding_approvers_per_merge_request":false,
		"merge_requests_author_approval":false,
		"merge_requests_disable_committers_approval":true,
		"require_password_to_approve":false,
		"selective_code_owner_removals":true
	}`

	approvalRuleJSON = `{
		"id":10,
		"name":"Security Review",
		"rule_type":"regular",
		"approvals_required":2,
		"contains_hidden_groups":false,
		"applies_to_all_protected_branches":true,
		"eligible_approvers":[{"username":"alice"}],
		"users":[{"username":"bob"}],
		"groups":[{"name":"security-team"}]
	}`
)

// TestGetApprovalConfig_Success verifies GetApprovalConfig returns the project's approval configuration when the GitLab API returns HTTP 200, asserting key fields like ApprovalsBeforeMerge and SelectiveCodeOwnerRemovals.
func TestGetApprovalConfig_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Approvals {
			testutil.RespondJSON(w, http.StatusOK, approvalConfigJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetApprovalConfig(context.Background(), client, GetApprovalConfigInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ApprovalsBeforeMerge != 2 {
		t.Errorf("ApprovalsBeforeMerge = %d, want 2", out.ApprovalsBeforeMerge)
	}
	if !out.ResetApprovalsOnPush {
		t.Error("ResetApprovalsOnPush = false, want true")
	}
	if !out.MergeRequestsDisableCommittersApproval {
		t.Error("MergeRequestsDisableCommittersApproval = false, want true")
	}
	if !out.SelectiveCodeOwnerRemovals {
		t.Error("SelectiveCodeOwnerRemovals = false, want true")
	}
}

// TestGetApprovalConfig_EmptyProjectID verifies GetApprovalConfig returns an error when ProjectID is empty.
func TestGetApprovalConfig_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetApprovalConfig(context.Background(), client, GetApprovalConfigInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestGetApprovalConfig_APIError verifies GetApprovalConfig returns an error when the GitLab API responds with HTTP 400.
func TestGetApprovalConfig_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := GetApprovalConfig(context.Background(), client, GetApprovalConfigInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetApprovalConfig_ContextCancelled verifies GetApprovalConfig returns an error when the request context is already cancelled.
func TestGetApprovalConfig_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetApprovalConfig(ctx, client, GetApprovalConfigInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestChangeApprovalConfig_Success verifies ChangeApprovalConfig updates approval settings via POST and returns the resulting configuration on HTTP 200.
func TestChangeApprovalConfig_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Approvals {
			testutil.RespondJSON(w, http.StatusOK, approvalConfigJSON)
			return
		}
		http.NotFound(w, r)
	}))
	approvals := int64(2)
	out, err := ChangeApprovalConfig(context.Background(), client, ChangeApprovalConfigInput{
		ProjectID: "42", ApprovalsBeforeMerge: &approvals,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ApprovalsBeforeMerge != 2 {
		t.Errorf("ApprovalsBeforeMerge = %d, want 2", out.ApprovalsBeforeMerge)
	}
}

// TestChangeApprovalConfig_EmptyProjectID verifies ChangeApprovalConfig returns an error when ProjectID is empty.
func TestChangeApprovalConfig_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ChangeApprovalConfig(context.Background(), client, ChangeApprovalConfigInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestChangeApprovalConfig_APIError verifies ChangeApprovalConfig returns an error when the GitLab API responds with HTTP 400.
func TestChangeApprovalConfig_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ChangeApprovalConfig(context.Background(), client, ChangeApprovalConfigInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListApprovalRules_Success verifies ListApprovalRules returns the project's approval rules from HTTP 200, asserting rule ID, name, and required approvals.
func TestListApprovalRules_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42ApprovalRules {
			testutil.RespondJSON(w, http.StatusOK, `[`+approvalRuleJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListApprovalRules(context.Background(), client, ListApprovalRulesInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(out.Rules))
	}
	if out.Rules[0].ID != 10 {
		t.Errorf("Rules[0].ID = %d, want 10", out.Rules[0].ID)
	}
	if out.Rules[0].Name != "Security Review" {
		t.Errorf("Rules[0].Name = %q, want %q", out.Rules[0].Name, "Security Review")
	}
	if out.Rules[0].ApprovalsRequired != 2 {
		t.Errorf("Rules[0].ApprovalsRequired = %d, want 2", out.Rules[0].ApprovalsRequired)
	}
}

// TestListApprovalRules_EmptyProjectID verifies ListApprovalRules returns an error when ProjectID is empty.
func TestListApprovalRules_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListApprovalRules(context.Background(), client, ListApprovalRulesInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestListApprovalRules_APIError verifies ListApprovalRules returns an error when the GitLab API responds with HTTP 400.
func TestListApprovalRules_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ListApprovalRules(context.Background(), client, ListApprovalRulesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetApprovalRule_Success verifies GetApprovalRule fetches a single approval rule by ID and asserts its name and AppliesToAllProtectedBranches flag.
func TestGetApprovalRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42ApprovalRule10 {
			testutil.RespondJSON(w, http.StatusOK, approvalRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetApprovalRule(context.Background(), client, GetApprovalRuleInput{
		ProjectID: "42", RuleID: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
	if out.Name != "Security Review" {
		t.Errorf("Name = %q, want %q", out.Name, "Security Review")
	}
	if !out.AppliesToAllProtectedBranches {
		t.Error("AppliesToAllProtectedBranches = false, want true")
	}
}

// TestGetApprovalRule_EmptyProjectID verifies GetApprovalRule returns an error when ProjectID is empty.
func TestGetApprovalRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetApprovalRule(context.Background(), client, GetApprovalRuleInput{RuleID: 10})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestGetApprovalRule_EmptyRuleID verifies GetApprovalRule returns an error when RuleID is zero.
func TestGetApprovalRule_EmptyRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetApprovalRule(context.Background(), client, GetApprovalRuleInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty rule_id, got nil")
	}
}

// TestGetApprovalRule_APIError verifies GetApprovalRule returns an error when the GitLab API responds with HTTP 400.
func TestGetApprovalRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := GetApprovalRule(context.Background(), client, GetApprovalRuleInput{
		ProjectID: "42", RuleID: 10,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateApprovalRule_Success verifies CreateApprovalRule issues POST to the approval rules endpoint and returns the created rule on HTTP 201.
func TestCreateApprovalRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42ApprovalRules {
			testutil.RespondJSON(w, http.StatusCreated, approvalRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateApprovalRule(context.Background(), client, CreateApprovalRuleInput{
		ProjectID: "42", Name: "Security Review", ApprovalsRequired: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
	if out.Name != "Security Review" {
		t.Errorf("Name = %q, want %q", out.Name, "Security Review")
	}
}

// TestCreateApprovalRule_EmptyProjectID verifies CreateApprovalRule returns an error when ProjectID is empty.
func TestCreateApprovalRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateApprovalRule(context.Background(), client, CreateApprovalRuleInput{Name: "rule"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestCreateApprovalRule_EmptyName verifies CreateApprovalRule returns an error when Name is empty.
func TestCreateApprovalRule_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateApprovalRule(context.Background(), client, CreateApprovalRuleInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

// TestCreateApprovalRule_APIError verifies CreateApprovalRule returns an error when the GitLab API responds with HTTP 400.
func TestCreateApprovalRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := CreateApprovalRule(context.Background(), client, CreateApprovalRuleInput{
		ProjectID: "42", Name: "rule", ApprovalsRequired: 1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateApprovalRule_ContextCancelled verifies CreateApprovalRule returns an error when the request context is already cancelled.
func TestCreateApprovalRule_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateApprovalRule(ctx, client, CreateApprovalRuleInput{
		ProjectID: "42", Name: "rule", ApprovalsRequired: 1,
	})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestUpdateApprovalRule_Success verifies UpdateApprovalRule issues PUT to the rule endpoint and returns the updated rule on HTTP 200.
func TestUpdateApprovalRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42ApprovalRule10 {
			testutil.RespondJSON(w, http.StatusOK, approvalRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := UpdateApprovalRule(context.Background(), client, UpdateApprovalRuleInput{
		ProjectID: "42", RuleID: 10, Name: "Security Review",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
}

// TestUpdateApprovalRule_EmptyProjectID verifies UpdateApprovalRule returns an error when ProjectID is empty.
func TestUpdateApprovalRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateApprovalRule(context.Background(), client, UpdateApprovalRuleInput{RuleID: 10})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestUpdateApprovalRule_EmptyRuleID verifies UpdateApprovalRule returns an error when RuleID is zero.
func TestUpdateApprovalRule_EmptyRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateApprovalRule(context.Background(), client, UpdateApprovalRuleInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty rule_id, got nil")
	}
}

// TestUpdateApprovalRule_APIError verifies UpdateApprovalRule returns an error when the GitLab API responds with HTTP 400.
func TestUpdateApprovalRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := UpdateApprovalRule(context.Background(), client, UpdateApprovalRuleInput{
		ProjectID: "42", RuleID: 10,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteApprovalRule_Success verifies DeleteApprovalRule issues DELETE to the rule endpoint and succeeds on HTTP 204.
func TestDeleteApprovalRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42ApprovalRule10 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := DeleteApprovalRule(context.Background(), client, DeleteApprovalRuleInput{
		ProjectID: "42", RuleID: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteApprovalRule_EmptyProjectID verifies DeleteApprovalRule returns an error when ProjectID is empty.
func TestDeleteApprovalRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteApprovalRule(context.Background(), client, DeleteApprovalRuleInput{RuleID: 10})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestDeleteApprovalRule_EmptyRuleID verifies DeleteApprovalRule returns an error when RuleID is zero.
func TestDeleteApprovalRule_EmptyRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteApprovalRule(context.Background(), client, DeleteApprovalRuleInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty rule_id, got nil")
	}
}

// TestDeleteApprovalRule_APIError verifies DeleteApprovalRule returns an error when the GitLab API responds with HTTP 400.
func TestDeleteApprovalRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := DeleteApprovalRule(context.Background(), client, DeleteApprovalRuleInput{
		ProjectID: "42", RuleID: 10,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFormatApprovalConfigMarkdown_NonEmpty verifies FormatApprovalConfigMarkdown produces non-empty markdown containing the approvals count.
func TestFormatApprovalConfigMarkdown_NonEmpty(t *testing.T) {
	md := FormatApprovalConfigMarkdown(ApprovalConfigOutput{
		ApprovalsBeforeMerge: 2, ResetApprovalsOnPush: true,
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "2") {
		t.Error("markdown missing approvals count")
	}
}

// TestFormatApprovalRuleMarkdown_NonEmpty verifies FormatApprovalRuleMarkdown produces non-empty markdown containing the rule name.
func TestFormatApprovalRuleMarkdown_NonEmpty(t *testing.T) {
	md := FormatApprovalRuleMarkdown(ApprovalRuleOutput{
		ID: 10, Name: "Security Review", ApprovalsRequired: 2,
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "Security Review") {
		t.Error("markdown missing rule name")
	}
}

// TestFormatListApprovalRulesMarkdown_NonEmpty verifies FormatListApprovalRulesMarkdown produces non-empty markdown containing each rule's name.
func TestFormatListApprovalRulesMarkdown_NonEmpty(t *testing.T) {
	md := FormatListApprovalRulesMarkdown(ListApprovalRulesOutput{
		Rules: []ApprovalRuleOutput{
			{ID: 10, Name: "Rule A", ApprovalsRequired: 1},
		},
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "Rule A") {
		t.Error("markdown missing rule name")
	}
}
