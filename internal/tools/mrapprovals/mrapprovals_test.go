// mrapprovals_test.go contains unit tests for the merge request approval MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrapprovals

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testApprovalRulesPath identifies the test approval rules path constant used by this package.
const testApprovalRulesPath = "/api/v4/projects/42/merge_requests/1/approval_rules"

// fmtNameWant identifies the fmt name want constant used by this package.
const fmtNameWant = "Name = %q, want %q"

// testSecurityTeam identifies the test security team constant used by this package.
const testSecurityTeam = "Security Team"

// testUpdatedRule identifies the test updated rule constant used by this package.
const testUpdatedRule = "Updated Rule"

// ---------------------------------------------------------------------------
// mrApprovalState tests
// ---------------------------------------------------------------------------.

// TestMRApprovalState_Success verifies MRApprovalState when success.
func TestMRApprovalState_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_state" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"approval_rules_overwritten": true,
				"rules": [
					{
						"id": 10,
						"name": "Security",
						"rule_type": "regular",
						"approvals_required": 2,
						"approved": false,
						"approved_by": [{"name": "Alice"}],
						"eligible_approvers": [{"name": "Alice"}, {"name": "Bob"}]
					}
				]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := State(context.Background(), client, StateInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.ApprovalRulesOverwritten {
		t.Error("expected ApprovalRulesOverwritten to be true")
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	r := out.Rules[0]
	if r.ID != 10 {
		t.Errorf("rule ID = %d, want 10", r.ID)
	}
	if r.Name != "Security" {
		t.Errorf("rule Name = %q, want %q", r.Name, "Security")
	}
	if r.ApprovalsRequired != 2 {
		t.Errorf("rule ApprovalsRequired = %d, want 2", r.ApprovalsRequired)
	}
	if r.Approved {
		t.Error("expected rule Approved to be false")
	}
	if len(r.ApprovedByNames) != 1 || r.ApprovedByNames[0] != "Alice" {
		t.Errorf("ApprovedByNames = %v, want [Alice]", r.ApprovedByNames)
	}
	if len(r.EligibleNames) != 2 {
		t.Errorf("EligibleNames count = %d, want 2", len(r.EligibleNames))
	}
}

// TestMRApprovalState_EmptyRules verifies MRApprovalState when empty rules.
func TestMRApprovalState_EmptyRules(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_state" {
			testutil.RespondJSON(w, http.StatusOK, `{"approval_rules_overwritten": false, "rules": []}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := State(context.Background(), client, StateInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ApprovalRulesOverwritten {
		t.Error("expected ApprovalRulesOverwritten to be false")
	}
	if len(out.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(out.Rules))
	}
}

// TestMRApprovalState_MissingProjectID verifies MRApprovalState when missing project ID.
func TestMRApprovalState_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := State(context.Background(), client, StateInput{
		ProjectID: "",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestMRApprovalStateServer_Error verifies MRApprovalStateServer when error.
func TestMRApprovalStateServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := State(context.Background(), client, StateInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestMRApprovalState_CancelledContext verifies MRApprovalState when cancelled context.
func TestMRApprovalState_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := State(ctx, client, StateInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// mrApprovalRules tests
// ---------------------------------------------------------------------------.

// approvalRuleExpected holds approval rule expected data for the mrapprovals package.
type approvalRuleExpected struct {
	id                int64
	name              string
	ruleType          string
	approvalsRequired int
	approved          bool
	approvedByCount   int
	eligibleCount     int
}

// assertApprovalRule checks approval rule invariants for tests.
func assertApprovalRule(t *testing.T, r RuleOutput, exp approvalRuleExpected) {
	t.Helper()
	if r.ID != exp.id {
		t.Errorf("ID = %d, want %d", r.ID, exp.id)
	}
	if r.Name != exp.name {
		t.Errorf(fmtNameWant, r.Name, exp.name)
	}
	if r.RuleType != exp.ruleType {
		t.Errorf("RuleType = %q, want %q", r.RuleType, exp.ruleType)
	}
	if r.ApprovalsRequired != exp.approvalsRequired {
		t.Errorf("ApprovalsRequired = %d, want %d", r.ApprovalsRequired, exp.approvalsRequired)
	}
	if r.Approved != exp.approved {
		t.Errorf("Approved = %v, want %v", r.Approved, exp.approved)
	}
	if len(r.ApprovedByNames) != exp.approvedByCount {
		t.Errorf("ApprovedByNames count = %d, want %d", len(r.ApprovedByNames), exp.approvedByCount)
	}
	if len(r.EligibleNames) != exp.eligibleCount {
		t.Errorf("EligibleNames count = %d, want %d", len(r.EligibleNames), exp.eligibleCount)
	}
}

// TestMRApprovalRules_Success covers MRApprovalRules with table-driven subtests for success.
func TestMRApprovalRules_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testApprovalRulesPath && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[
				{
					"id": 1,
					"name": "Code Owners",
					"rule_type": "code_owner",
					"approvals_required": 1,
					"approved": true,
					"approved_by": [{"name": "Charlie"}],
					"eligible_approvers": [{"name": "Charlie"}, {"name": "Dave"}]
				},
				{
					"id": 2,
					"name": "Security Review",
					"rule_type": "regular",
					"approvals_required": 2,
					"approved": false,
					"approved_by": [],
					"eligible_approvers": [{"name": "Eve"}]
				}
			]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Rules(context.Background(), client, RulesInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(out.Rules))
	}

	tests := []struct {
		name string
		idx  int
		exp  approvalRuleExpected
	}{
		{"CodeOwners", 0, approvalRuleExpected{1, "Code Owners", "code_owner", 1, true, 1, 2}},
		{"SecurityReview", 1, approvalRuleExpected{2, "Security Review", "regular", 2, false, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertApprovalRule(t, out.Rules[tt.idx], tt.exp)
		})
	}
}

// TestMRApprovalRules_Empty verifies MRApprovalRules when empty.
func TestMRApprovalRules_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testApprovalRulesPath {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Rules(context.Background(), client, RulesInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(out.Rules))
	}
}

// TestMRApprovalRules_MissingProjectID verifies MRApprovalRules when missing project ID.
func TestMRApprovalRules_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := Rules(context.Background(), client, RulesInput{
		ProjectID: "",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestMRApprovalRulesServer_Error verifies MRApprovalRulesServer when error.
func TestMRApprovalRulesServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := Rules(context.Background(), client, RulesInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestMRApprovalRules_CancelledContext verifies MRApprovalRules when cancelled context.
func TestMRApprovalRules_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Rules(ctx, client, RulesInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// approvalRuleToOutput converter tests
// ---------------------------------------------------------------------------.

// TestApprovalRuleToOutput_NilUsers verifies ApprovalRuleToOutput when nil users.
func TestApprovalRuleToOutput_NilUsers(t *testing.T) {
	rule := RuleToOutput(&gl.MergeRequestApprovalRule{
		ID:                1,
		Name:              "Test",
		RuleType:          "regular",
		ApprovalsRequired: 1,
		Approved:          false,
		ApprovedBy:        nil,
		EligibleApprovers: nil,
	})
	if rule.ApprovedByNames != nil {
		t.Errorf("expected nil ApprovedByNames, got %v", rule.ApprovedByNames)
	}
	if rule.EligibleNames != nil {
		t.Errorf("expected nil EligibleNames, got %v", rule.EligibleNames)
	}
}

// TestApprovalRuleToOutput_MultipleUsers verifies ApprovalRuleToOutput when multiple users.
func TestApprovalRuleToOutput_MultipleUsers(t *testing.T) {
	rule := RuleToOutput(&gl.MergeRequestApprovalRule{
		ID:                5,
		Name:              "Team Lead",
		RuleType:          "regular",
		ApprovalsRequired: 3,
		Approved:          true,
		ApprovedBy: []*gl.BasicUser{
			{Name: "Alice"},
			{Name: "Bob"},
		},
		EligibleApprovers: []*gl.BasicUser{
			{Name: "Alice"},
			{Name: "Bob"},
			{Name: "Charlie"},
		},
	})
	if len(rule.ApprovedByNames) != 2 {
		t.Errorf("ApprovedByNames count = %d, want 2", len(rule.ApprovedByNames))
	}
	if len(rule.EligibleNames) != 3 {
		t.Errorf("EligibleNames count = %d, want 3", len(rule.EligibleNames))
	}
	if rule.ID != 5 || rule.Name != "Team Lead" || rule.ApprovalsRequired != 3 || !rule.Approved {
		t.Errorf("unexpected output: %+v", rule)
	}
}

// TestApprovalRuleToOutputSkips_NilEntries verifies ApprovalRuleToOutputSkips when nil entries.
func TestApprovalRuleToOutputSkips_NilEntries(t *testing.T) {
	rule := RuleToOutput(&gl.MergeRequestApprovalRule{
		ApprovedBy:        []*gl.BasicUser{nil, {Name: "Valid"}},
		EligibleApprovers: []*gl.BasicUser{{Name: "E1"}, nil},
	})
	if len(rule.ApprovedByNames) != 1 || rule.ApprovedByNames[0] != "Valid" {
		t.Errorf("ApprovedByNames = %v, want [Valid]", rule.ApprovedByNames)
	}
	if len(rule.EligibleNames) != 1 || rule.EligibleNames[0] != "E1" {
		t.Errorf("EligibleNames = %v, want [E1]", rule.EligibleNames)
	}
}

// ---------------------------------------------------------------------------
// Config (GetConfiguration) tests
// ---------------------------------------------------------------------------.

// configResponse identifies the config response constant used by this package.
const configResponse = `{
	"id": 1, "iid": 10, "project_id": 42, "title": "Test MR", "state": "opened",
	"approved": true, "approvals_required": 2, "approvals_left": 0,
	"approvals_before_merge": 2, "has_approval_rules": true,
	"user_has_approved": true, "user_can_approve": false,
	"approved_by": [{"user": {"name": "Alice"}, "approved_at": "2026-01-15T10:30:00Z"}],
	"suggested_approvers": [{"name": "Bob"}]
}`

// TestMRApprovalConfig_Success verifies MRApprovalConfig when success.
func TestMRApprovalConfig_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/10/approvals" {
			testutil.RespondJSON(w, http.StatusOK, configResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Config(context.Background(), client, ConfigInput{ProjectID: "42", MRIID: 10})
	if err != nil {
		t.Fatalf("Config() unexpected error: %v", err)
	}
	if !out.Approved {
		t.Error("Approved = false, want true")
	}
	if out.ApprovalsRequired != 2 {
		t.Errorf("ApprovalsRequired = %d, want 2", out.ApprovalsRequired)
	}
	if out.ApprovalsLeft != 0 {
		t.Errorf("ApprovalsLeft = %d, want 0", out.ApprovalsLeft)
	}
	if !out.UserHasApproved {
		t.Error("UserHasApproved = false, want true")
	}
	if len(out.ApprovedBy) != 1 || out.ApprovedBy[0].Name != "Alice" {
		t.Errorf("ApprovedBy = %v, want [{Alice 2026-01-15T10:30:00Z}]", out.ApprovedBy)
	}
	if out.ApprovedBy[0].ApprovedAt != "2026-01-15T10:30:00Z" {
		t.Errorf("ApprovedAt = %q, want %q", out.ApprovedBy[0].ApprovedAt, "2026-01-15T10:30:00Z")
	}
	if len(out.SuggestedNames) != 1 || out.SuggestedNames[0] != "Bob" {
		t.Errorf("SuggestedNames = %v, want [Bob]", out.SuggestedNames)
	}
}

// TestMRApprovalConfig_MissingProject verifies MRApprovalConfig when missing project.
func TestMRApprovalConfig_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Config(context.Background(), client, ConfigInput{MRIID: 1})
	if err == nil {
		t.Fatal("Config() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// Reset (ResetApprovalsOfMergeRequest) tests
// ---------------------------------------------------------------------------.

// TestMRApprovalReset_Success verifies MRApprovalReset when success.
func TestMRApprovalReset_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/reset_approvals" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	err := Reset(context.Background(), client, ResetInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf("Reset() unexpected error: %v", err)
	}
}

// TestMRApprovalReset_MissingProject verifies MRApprovalReset when missing project.
func TestMRApprovalReset_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Reset(context.Background(), client, ResetInput{MRIID: 1})
	if err == nil {
		t.Fatal("Reset() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// CreateRule tests
// ---------------------------------------------------------------------------.

// ruleResponse identifies the rule response constant used by this package.
const ruleResponse = `{
	"id": 5, "name": "Security Team", "rule_type": "regular",
	"report_type": "", "section": "",
	"approvals_required": 2, "approved": false,
	"contains_hidden_groups": false,
	"approved_by": [], "eligible_approvers": [{"name": "Alice"}],
	"users": [{"name": "Alice"}], "groups": [{"name": "Security"}]
}`

// TestMRApprovalRuleCreate_Success verifies MRApprovalRuleCreate when success.
func TestMRApprovalRuleCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == testApprovalRulesPath {
			testutil.RespondJSON(w, http.StatusCreated, ruleResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateRule(context.Background(), client, CreateRuleInput{
		ProjectID:         "42",
		MRIID:             1,
		Name:              testSecurityTeam,
		ApprovalsRequired: 2,
		UserIDs:           []int64{100},
		GroupIDs:          []int64{200},
	})
	if err != nil {
		t.Fatalf("CreateRule() unexpected error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if out.Name != testSecurityTeam {
		t.Errorf(fmtNameWant, out.Name, testSecurityTeam)
	}
	if out.ApprovalsRequired != 2 {
		t.Errorf("ApprovalsRequired = %d, want 2", out.ApprovalsRequired)
	}
	if len(out.UserNames) != 1 || out.UserNames[0] != "Alice" {
		t.Errorf("UserNames = %v, want [Alice]", out.UserNames)
	}
	if len(out.GroupNames) != 1 || out.GroupNames[0] != "Security" {
		t.Errorf("GroupNames = %v, want [Security]", out.GroupNames)
	}
}

// TestMRApprovalRuleCreate_MissingName verifies MRApprovalRuleCreate when missing name.
func TestMRApprovalRuleCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateRule(context.Background(), client, CreateRuleInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("CreateRule() expected error for missing name")
	}
}

// TestMRApprovalRuleCreate_MissingProject verifies MRApprovalRuleCreate when missing project.
func TestMRApprovalRuleCreate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateRule(context.Background(), client, CreateRuleInput{MRIID: 1, Name: "Test"})
	if err == nil {
		t.Fatal("CreateRule() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// UpdateRule tests
// ---------------------------------------------------------------------------.

// TestMRApprovalRuleUpdate_Success verifies MRApprovalRuleUpdate when success.
func TestMRApprovalRuleUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_rules/5" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 5, "name": "Updated Rule", "rule_type": "regular",
				"approvals_required": 3, "approved": false,
				"approved_by": [], "eligible_approvers": [],
				"users": [], "groups": []
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateRule(context.Background(), client, UpdateRuleInput{
		ProjectID:      "42",
		MRIID:          1,
		ApprovalRuleID: 5,
		Name:           testUpdatedRule,
	})
	if err != nil {
		t.Fatalf("UpdateRule() unexpected error: %v", err)
	}
	if out.Name != testUpdatedRule {
		t.Errorf(fmtNameWant, out.Name, testUpdatedRule)
	}
}

// TestMRApprovalRuleUpdate_MissingRuleID verifies MRApprovalRuleUpdate when missing rule ID.
func TestMRApprovalRuleUpdate_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateRule(context.Background(), client, UpdateRuleInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("UpdateRule() expected error for missing approval_rule_id")
	}
}

// TestMRApprovalRuleUpdate_MissingProject verifies MRApprovalRuleUpdate when missing project.
func TestMRApprovalRuleUpdate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateRule(context.Background(), client, UpdateRuleInput{MRIID: 1, ApprovalRuleID: 5})
	if err == nil {
		t.Fatal("UpdateRule() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// DeleteRule tests
// ---------------------------------------------------------------------------.

// TestMRApprovalRuleDelete_Success verifies MRApprovalRuleDelete when success.
func TestMRApprovalRuleDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_rules/5" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteRule(context.Background(), client, DeleteRuleInput{ProjectID: "42", MRIID: 1, ApprovalRuleID: 5})
	if err != nil {
		t.Fatalf("DeleteRule() unexpected error: %v", err)
	}
}

// TestMRApprovalRuleDelete_MissingRuleID verifies MRApprovalRuleDelete when missing rule ID.
func TestMRApprovalRuleDelete_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteRule(context.Background(), client, DeleteRuleInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("DeleteRule() expected error for missing approval_rule_id")
	}
}

// TestMRApprovalRuleDelete_MissingProject verifies MRApprovalRuleDelete when missing project.
func TestMRApprovalRuleDelete_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteRule(context.Background(), client, DeleteRuleInput{MRIID: 1, ApprovalRuleID: 5})
	if err == nil {
		t.Fatal("DeleteRule() expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// int64 validation tests
// ---------------------------------------------------------------------------.

// assertErrContains checks err contains invariants for tests.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies MRIIDRequired when validation.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when MRIID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt("42")
	const wantSubstr = "merge_request_iid"

	t.Run("State", func(t *testing.T) {
		_, err := State(ctx, client, StateInput{ProjectID: pid, MRIID: 0})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("Rules", func(t *testing.T) {
		_, err := Rules(ctx, client, RulesInput{ProjectID: pid, MRIID: 0})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("Config", func(t *testing.T) {
		_, err := Config(ctx, client, ConfigInput{ProjectID: pid, MRIID: 0})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("Reset", func(t *testing.T) {
		err := Reset(ctx, client, ResetInput{ProjectID: pid, MRIID: 0})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("CreateRule", func(t *testing.T) {
		_, err := CreateRule(ctx, client, CreateRuleInput{ProjectID: pid, MRIID: 0, Name: "test", ApprovalsRequired: 1})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("UpdateRule", func(t *testing.T) {
		_, err := UpdateRule(ctx, client, UpdateRuleInput{ProjectID: pid, MRIID: 0, ApprovalRuleID: 1})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("DeleteRule", func(t *testing.T) {
		err := DeleteRule(ctx, client, DeleteRuleInput{ProjectID: pid, MRIID: 0, ApprovalRuleID: 1})
		assertErrContains(t, err, wantSubstr)
	})
}

// TestApprovalRuleIDRequired_Validation verifies ApprovalRuleIDRequired when validation.
func TestApprovalRuleIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when ApprovalRuleID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt("42")
	const wantSubstr = "approval_rule_id"

	t.Run("UpdateRule", func(t *testing.T) {
		_, err := UpdateRule(ctx, client, UpdateRuleInput{ProjectID: pid, MRIID: 1, ApprovalRuleID: 0})
		assertErrContains(t, err, wantSubstr)
	})
	t.Run("DeleteRule", func(t *testing.T) {
		err := DeleteRule(ctx, client, DeleteRuleInput{ProjectID: pid, MRIID: 1, ApprovalRuleID: 0})
		assertErrContains(t, err, wantSubstr)
	})
}

// TestMRApproval_State404CommunityEdition verifies that State returns a
// clear feature-tier message when GitLab CE returns 404 for approval endpoints.
func TestMRApproval_State404CommunityEdition(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := State(context.Background(), client, StateInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("State() expected error for CE 404, got nil")
	}
	if !strings.Contains(err.Error(), "GitLab Premium") {
		t.Errorf("State() error should mention GitLab Premium, got: %v", err)
	}
}

// TestMRApproval_Rules404CommunityEdition verifies that Rules returns a
// clear feature-tier message when GitLab CE returns 404.
func TestMRApproval_Rules404CommunityEdition(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Rules(context.Background(), client, RulesInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("Rules() expected error for CE 404, got nil")
	}
	if !strings.Contains(err.Error(), "GitLab Premium") {
		t.Errorf("Rules() error should mention GitLab Premium, got: %v", err)
	}
}

// TestMRApproval_Config404CommunityEdition verifies that Config returns a
// clear feature-tier message when GitLab CE returns 404.
func TestMRApproval_Config404CommunityEdition(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Config(context.Background(), client, ConfigInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("Config() expected error for CE 404, got nil")
	}
	if !strings.Contains(err.Error(), "GitLab Premium") {
		t.Errorf("Config() error should mention GitLab Premium, got: %v", err)
	}
}

// TestDebug_ErrorType verifies Debug when error type.
func TestDebug_ErrorType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, _, err := client.GL().MergeRequestApprovals.GetApprovalState("42", 1, gl.WithContext(context.Background()))
	if err == nil {
		t.Fatal("expected error")
	}

	var glErr *gl.ErrorResponse
	t.Logf("error type: %T", err)
	t.Logf("error value: %v", err)
	t.Logf("errors.As for ErrorResponse: %v", errors.As(err, &glErr))
	if errors.As(err, &glErr) {
		if glErr.Response != nil {
			t.Logf("ErrorResponse.Response.StatusCode: %d", glErr.Response.StatusCode)
		} else {
			t.Log("ErrorResponse.Response is nil")
		}
	} else {
		t.Log("error is NOT a *gl.ErrorResponse")
		t.Logf("unwrapped: %v", errors.Unwrap(err))
		t.Logf("fmt: %s", fmt.Sprintf("%+v", err))
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// ---------------------------------------------------------------------------
// Config — canceled context & server error
// ---------------------------------------------------------------------------.

// TestConfig_CancelledContext verifies Config when cancelled context.
func TestConfig_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := Config(ctx, client, ConfigInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestConfig_ServerError verifies Config when server error.
func TestConfig_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"fail"}`)
	}))

	_, err := Config(context.Background(), client, ConfigInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// ---------------------------------------------------------------------------
// Reset — canceled context & server error
// ---------------------------------------------------------------------------.

// TestReset_CancelledContext verifies Reset when cancelled context.
func TestReset_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	err := Reset(ctx, client, ResetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestReset_ServerError verifies Reset when server error.
func TestReset_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"fail"}`)
	}))

	err := Reset(context.Background(), client, ResetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestReset_NotFoundMentionsAccessTokenRequirement verifies 404 responses explain the bot-token requirement.
func TestReset_NotFoundMentionsAccessTokenRequirement(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	err := Reset(context.Background(), client, ResetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	for _, want := range []string{"bot user", "project/group access token", "PATs from human users"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// ---------------------------------------------------------------------------
// CreateRule — canceled context, server error & ApprovalProjectRuleID path
// ---------------------------------------------------------------------------.

// TestCreateRule_CancelledContext verifies CreateRule when cancelled context.
func TestCreateRule_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := CreateRule(ctx, client, CreateRuleInput{ProjectID: "42", MRIID: 1, Name: "R"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreateRule_ServerError verifies CreateRule when server error.
func TestCreateRule_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"fail"}`)
	}))

	_, err := CreateRule(context.Background(), client, CreateRuleInput{
		ProjectID: "42", MRIID: 1, Name: "R", ApprovalsRequired: 1,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestCreateRule_WithApprovalProjectRuleID verifies CreateRule when with approval project rule ID.
func TestCreateRule_WithApprovalProjectRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_rules" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 7, "name": "Inherited", "rule_type": "regular",
				"approvals_required": 1, "approved": false,
				"approved_by": [], "eligible_approvers": [],
				"users": [], "groups": []
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateRule(context.Background(), client, CreateRuleInput{
		ProjectID:             "42",
		MRIID:                 1,
		Name:                  "Inherited",
		ApprovalsRequired:     1,
		ApprovalProjectRuleID: 99,
	})
	if err != nil {
		t.Fatalf("CreateRule() unexpected error: %v", err)
	}
	if out.ID != 7 {
		t.Errorf("ID = %d, want 7", out.ID)
	}
}

// ---------------------------------------------------------------------------
// UpdateRule — canceled context, server error & optional fields
// ---------------------------------------------------------------------------.

// TestUpdateRule_CancelledContext verifies UpdateRule when cancelled context.
func TestUpdateRule_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := UpdateRule(ctx, client, UpdateRuleInput{ProjectID: "42", MRIID: 1, ApprovalRuleID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateRule_ServerError verifies UpdateRule when server error.
func TestUpdateRule_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"fail"}`)
	}))

	_, err := UpdateRule(context.Background(), client, UpdateRuleInput{
		ProjectID: "42", MRIID: 1, ApprovalRuleID: 5,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestUpdateRule_AllOptionalFields verifies UpdateRule when all optional fields.
func TestUpdateRule_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_rules/5" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 5, "name": "Full Update", "rule_type": "regular",
				"approvals_required": 4, "approved": false,
				"approved_by": [], "eligible_approvers": [],
				"users": [{"name":"X"}], "groups": [{"name":"G"}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	approvals := int64(4)
	out, err := UpdateRule(context.Background(), client, UpdateRuleInput{
		ProjectID:         "42",
		MRIID:             1,
		ApprovalRuleID:    5,
		Name:              "Full Update",
		ApprovalsRequired: &approvals,
		UserIDs:           []int64{10},
		GroupIDs:          []int64{20},
	})
	if err != nil {
		t.Fatalf("UpdateRule() unexpected error: %v", err)
	}
	if out.Name != "Full Update" {
		t.Errorf("Name = %q, want %q", out.Name, "Full Update")
	}
	if out.ApprovalsRequired != 4 {
		t.Errorf("ApprovalsRequired = %d, want 4", out.ApprovalsRequired)
	}
	if len(out.UserNames) != 1 {
		t.Errorf("UserNames count = %d, want 1", len(out.UserNames))
	}
	if len(out.GroupNames) != 1 {
		t.Errorf("GroupNames count = %d, want 1", len(out.GroupNames))
	}
}

// ---------------------------------------------------------------------------
// DeleteRule — canceled context & server error
// ---------------------------------------------------------------------------.

// TestDeleteRule_CancelledContext verifies DeleteRule when cancelled context.
func TestDeleteRule_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	err := DeleteRule(ctx, client, DeleteRuleInput{ProjectID: "42", MRIID: 1, ApprovalRuleID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteRule_ServerError verifies DeleteRule when server error.
func TestDeleteRule_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"fail"}`)
	}))

	err := DeleteRule(context.Background(), client, DeleteRuleInput{
		ProjectID: "42", MRIID: 1, ApprovalRuleID: 5,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// ---------------------------------------------------------------------------
// RuleToOutput — Users & Groups paths
// ---------------------------------------------------------------------------.

// TestRuleToOutput_WithUsersAndGroups verifies RuleToOutput when with users and groups.
func TestRuleToOutput_WithUsersAndGroups(t *testing.T) {
	r := fakeApprovalRule(t)
	out := RuleToOutput(&r)
	if len(out.UserNames) != 2 {
		t.Errorf("UserNames count = %d, want 2", len(out.UserNames))
	}
	if len(out.GroupNames) != 1 {
		t.Errorf("GroupNames count = %d, want 1", len(out.GroupNames))
	}
	if out.ReportType != "test_report" {
		t.Errorf("ReportType = %q, want %q", out.ReportType, "test_report")
	}
	if out.Section != "sec" {
		t.Errorf("Section = %q, want %q", out.Section, "sec")
	}
	if !out.ContainsHiddenGroups {
		t.Error("ContainsHiddenGroups = false, want true")
	}
}

// TestRuleToOutput_NilGroupEntry verifies RuleToOutput when nil group entry.
func TestRuleToOutput_NilGroupEntry(t *testing.T) {
	r := fakeApprovalRuleNilGroup(t)
	out := RuleToOutput(&r)
	if len(out.GroupNames) != 1 || out.GroupNames[0] != "Good" {
		t.Errorf("GroupNames = %v, want [Good]", out.GroupNames)
	}
}

// ---------------------------------------------------------------------------
// configToOutput — nil approved_by entry, nil suggested_approvers entry
// ---------------------------------------------------------------------------.

// TestConfig_ToOutputNilEntries verifies Config when to output nil entries.
func TestConfig_ToOutputNilEntries(t *testing.T) {
	c := fakeConfigNilEntries(t)
	out := configToOutput(&c)
	if len(out.ApprovedBy) != 1 || out.ApprovedBy[0].Name != "Alice" {
		t.Errorf("ApprovedBy = %v, want [{Alice}]", out.ApprovedBy)
	}
	if len(out.SuggestedNames) != 1 || out.SuggestedNames[0] != "Bob" {
		t.Errorf("SuggestedNames = %v, want [Bob]", out.SuggestedNames)
	}
}

// ---------------------------------------------------------------------------
// FormatStateMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatStateMarkdown_WithRules verifies FormatStateMarkdown when with rules.
func TestFormatStateMarkdown_WithRules(t *testing.T) {
	s := StateOutput{
		ApprovalRulesOverwritten: true,
		Rules: []RuleOutput{
			{ID: 1, Name: "Security", RuleType: "regular", ApprovalsRequired: 2, Approved: true, ApprovedByNames: []string{"Alice"}},
			{ID: 2, Name: "QA", RuleType: "code_owner", ApprovalsRequired: 1, Approved: false, ApprovedByNames: nil},
		},
	}
	md := FormatStateMarkdown(s)
	assertContains(t, md, "## MR Approval State")
	assertContains(t, md, "**Rules overwritten**: Yes")
	assertContains(t, md, "| 1 |")
	assertContains(t, md, "| 2 |")
	assertContains(t, md, "✅")
	assertContains(t, md, "❌")
	assertContains(t, md, "Alice")
}

// TestFormatStateMarkdown_Empty verifies FormatStateMarkdown when empty.
func TestFormatStateMarkdown_Empty(t *testing.T) {
	md := FormatStateMarkdown(StateOutput{})
	assertContains(t, md, "**Rules overwritten**: No")
	assertContains(t, md, "No approval rules configured.")
}

// ---------------------------------------------------------------------------
// FormatRulesMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatRulesMarkdown_WithRules verifies FormatRulesMarkdown when with rules.
func TestFormatRulesMarkdown_WithRules(t *testing.T) {
	out := RulesOutput{
		Rules: []RuleOutput{
			{ID: 10, Name: "Team", RuleType: "regular", ApprovalsRequired: 1, Approved: true, EligibleNames: []string{"Eve", "Frank"}},
		},
	}
	md := FormatRulesMarkdown(out)
	assertContains(t, md, "## MR Approval Rules (1)")
	assertContains(t, md, "| 10 |")
	assertContains(t, md, "✅")
	assertContains(t, md, "Eve, Frank")
}

// TestFormatRulesMarkdown_Empty verifies FormatRulesMarkdown when empty.
func TestFormatRulesMarkdown_Empty(t *testing.T) {
	md := FormatRulesMarkdown(RulesOutput{})
	assertContains(t, md, "## MR Approval Rules (0)")
	assertContains(t, md, "No approval rules configured.")
}

// ---------------------------------------------------------------------------
// FormatConfigMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatConfigMarkdown_Full verifies FormatConfigMarkdown when full.
func TestFormatConfigMarkdown_Full(t *testing.T) {
	c := ConfigOutput{
		IID:               10,
		State:             "opened",
		Approved:          true,
		ApprovalsRequired: 2,
		ApprovalsLeft:     0,
		HasApprovalRules:  true,
		UserHasApproved:   true,
		UserCanApprove:    false,
		ApprovedBy:        []Approver{{Name: "Alice"}},
		SuggestedNames:    []string{"Bob"},
	}
	md := FormatConfigMarkdown(c)
	assertContains(t, md, "## MR Approval Configuration")
	assertContains(t, md, "| MR | !10 |")
	assertContains(t, md, "| State | opened |")
	assertContains(t, md, "| Approved | true |")
	assertContains(t, md, "| Approvals Required | 2 |")
	assertContains(t, md, "| Approvals Left | 0 |")
	assertContains(t, md, "| Has Approval Rules | true |")
	assertContains(t, md, "| User Has Approved | true |")
	assertContains(t, md, "| User Can Approve | false |")
	assertContains(t, md, "**Approved by**: Alice")
	assertContains(t, md, "**Suggested approvers**: Bob")
}

// TestFormatConfigMarkdown_Minimal verifies FormatConfigMarkdown when minimal.
func TestFormatConfigMarkdown_Minimal(t *testing.T) {
	md := FormatConfigMarkdown(ConfigOutput{State: "merged"})
	assertContains(t, md, "| State | merged |")
	assertNotContains(t, md, "**Approved by**")
	assertNotContains(t, md, "**Suggested approvers**")
}

// TestFormatConfigMarkdown_ApprovedByWithDate verifies that FormatConfigMarkdown
// includes the approval date in parentheses when ApprovedAt is non-empty.
func TestFormatConfigMarkdown_ApprovedByWithDate(t *testing.T) {
	c := ConfigOutput{
		State: "opened",
		ApprovedBy: []Approver{
			{Name: "Alice", ApprovedAt: "2026-03-15T14:00:00Z"},
			{Name: "Bob", ApprovedAt: ""},
		},
	}
	md := FormatConfigMarkdown(c)
	assertContains(t, md, "Alice (2026-03-15T14:00:00Z)")
	assertContains(t, md, "Bob")
	if strings.Contains(md, "Bob (") {
		t.Error("Bob should not have date parentheses")
	}
}

// ---------------------------------------------------------------------------
// FormatRuleMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatRuleMarkdown_Full verifies FormatRuleMarkdown when full.
func TestFormatRuleMarkdown_Full(t *testing.T) {
	r := RuleOutput{
		ID:                1,
		Name:              "Team Leads",
		RuleType:          "regular",
		ApprovalsRequired: 2,
		Approved:          true,
		EligibleNames:     []string{"Alice", "Bob"},
		UserNames:         []string{"Alice"},
		GroupNames:        []string{"Leads"},
	}
	md := FormatRuleMarkdown(r)
	assertContains(t, md, "## Approval Rule: Team Leads")
	assertContains(t, md, "| ID | 1 |")
	assertContains(t, md, "| Type | regular |")
	assertContains(t, md, "| Approvals Required | 2 |")
	assertContains(t, md, "✅")
	assertContains(t, md, "| Eligible | Alice, Bob |")
	assertContains(t, md, "| Users | Alice |")
	assertContains(t, md, "| Groups | Leads |")
}

// TestFormatRuleMarkdown_Minimal verifies FormatRuleMarkdown when minimal.
func TestFormatRuleMarkdown_Minimal(t *testing.T) {
	r := RuleOutput{
		ID:                3,
		Name:              "Basic",
		RuleType:          "any_approver",
		ApprovalsRequired: 0,
		Approved:          false,
	}
	md := FormatRuleMarkdown(r)
	assertContains(t, md, "## Approval Rule: Basic")
	assertContains(t, md, "❌")
	assertNotContains(t, md, "| Eligible |")
	assertNotContains(t, md, "| Users |")
	assertNotContains(t, md, "| Groups |")
}

// assertContains checks contains invariants for tests.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected string to contain %q, got:\n%s", substr, s)
	}
}

// assertNotContains checks not contains invariants for tests.
func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected string NOT to contain %q, got:\n%s", substr, s)
	}
}

// ---------------------------------------------------------------------------
// Fake data factories (avoid import cycle with gl types in helpers)
// ---------------------------------------------------------------------------.

// fakeApprovalRule supports fake approval rule assertions in mrapprovals tests.
func fakeApprovalRule(t *testing.T) gl.MergeRequestApprovalRule {
	t.Helper()
	return gl.MergeRequestApprovalRule{
		ID:                   1,
		Name:                 "Test Rule",
		RuleType:             "regular",
		ReportType:           "test_report",
		Section:              "sec",
		ApprovalsRequired:    2,
		Approved:             true,
		ContainsHiddenGroups: true,
		ApprovedBy:           []*gl.BasicUser{{Name: "A1"}},
		EligibleApprovers:    []*gl.BasicUser{{Name: "E1"}},
		Users:                []*gl.BasicUser{{Name: "U1"}, {Name: "U2"}},
		Groups:               []*gl.Group{{Name: "G1"}},
	}
}

// fakeApprovalRuleNilGroup supports fake approval rule nil group assertions in mrapprovals tests.
func fakeApprovalRuleNilGroup(t *testing.T) gl.MergeRequestApprovalRule {
	t.Helper()
	return gl.MergeRequestApprovalRule{
		Groups: []*gl.Group{nil, {Name: "Good"}},
	}
}

// fakeConfigNilEntries supports fake config nil entries assertions in mrapprovals tests.
func fakeConfigNilEntries(t *testing.T) gl.MergeRequestApprovals {
	t.Helper()
	return gl.MergeRequestApprovals{
		ApprovedBy: []*gl.MergeRequestApproverUser{
			nil,
			{User: nil},
			{User: &gl.BasicUser{Name: "Alice"}},
		},
		SuggestedApprovers: []*gl.BasicUser{
			nil,
			{Name: "Bob"},
		},
	}
}
