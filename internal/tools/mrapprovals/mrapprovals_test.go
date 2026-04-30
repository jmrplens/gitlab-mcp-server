// mrapprovals_test.go contains unit tests for the merge request approval MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrapprovals

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const fmtUnexpErr = "unexpected error: %v"

const testApprovalRulesPath = "/api/v4/projects/42/merge_requests/1/approval_rules"
const fmtNameWant = "Name = %q, want %q"
const testSecurityTeam = "Security Team"
const testUpdatedRule = "Updated Rule"

// ---------------------------------------------------------------------------
// mrApprovalState tests
// ---------------------------------------------------------------------------.

// TestMRApprovalState_Success verifies the behavior of m r approval state success.
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

// TestMRApprovalState_EmptyRules verifies the behavior of m r approval state empty rules.
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

// TestMRApprovalState_MissingProjectID verifies the behavior of m r approval state missing project i d.
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

// TestMRApprovalStateServer_Error verifies the behavior of m r approval state server error.
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

// TestMRApprovalState_CancelledContext verifies the behavior of m r approval state cancelled context.
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

// approvalRuleExpected holds data for mrapprovals operations.
type approvalRuleExpected struct {
	id                int64
	name              string
	ruleType          string
	approvalsRequired int
	approved          bool
	approvedByCount   int
	eligibleCount     int
}

// assertApprovalRule is an internal helper for the mrapprovals package.
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

// TestMRApprovalRules_Success validates m r approval rules success across multiple scenarios using table-driven subtests.
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

// TestMRApprovalRules_Empty verifies the behavior of m r approval rules empty.
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

// TestMRApprovalRules_MissingProjectID verifies the behavior of m r approval rules missing project i d.
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

// TestMRApprovalRulesServer_Error verifies the behavior of m r approval rules server error.
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

// TestMRApprovalRules_CancelledContext verifies the behavior of m r approval rules cancelled context.
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

// TestApprovalRuleToOutput_NilUsers verifies the behavior of approval rule to output nil users.
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

// TestApprovalRuleToOutput_MultipleUsers verifies the behavior of approval rule to output multiple users.
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

// TestApprovalRuleToOutputSkips_NilEntries verifies the behavior of approval rule to output skips nil entries.
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

const configResponse = `{
	"id": 1, "iid": 10, "project_id": 42, "title": "Test MR", "state": "opened",
	"approved": true, "approvals_required": 2, "approvals_left": 0,
	"approvals_before_merge": 2, "has_approval_rules": true,
	"user_has_approved": true, "user_can_approve": false,
	"approved_by": [{"user": {"name": "Alice"}, "approved_at": "2026-01-15T10:30:00Z"}],
	"suggested_approvers": [{"name": "Bob"}]
}`

// TestMRApprovalConfig_Success verifies the behavior of m r approval config success.
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

// TestMRApprovalConfig_MissingProject verifies the behavior of m r approval config missing project.
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

// TestMRApprovalReset_Success verifies the behavior of m r approval reset success.
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

// TestMRApprovalReset_MissingProject verifies the behavior of m r approval reset missing project.
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

const ruleResponse = `{
	"id": 5, "name": "Security Team", "rule_type": "regular",
	"report_type": "", "section": "",
	"approvals_required": 2, "approved": false,
	"contains_hidden_groups": false,
	"approved_by": [], "eligible_approvers": [{"name": "Alice"}],
	"users": [{"name": "Alice"}], "groups": [{"name": "Security"}]
}`

// TestMRApprovalRuleCreate_Success verifies the behavior of m r approval rule create success.
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

// TestMRApprovalRuleCreate_MissingName verifies the behavior of m r approval rule create missing name.
func TestMRApprovalRuleCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateRule(context.Background(), client, CreateRuleInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("CreateRule() expected error for missing name")
	}
}

// TestMRApprovalRuleCreate_MissingProject verifies the behavior of m r approval rule create missing project.
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

// TestMRApprovalRuleUpdate_Success verifies the behavior of m r approval rule update success.
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

// TestMRApprovalRuleUpdate_MissingRuleID verifies the behavior of m r approval rule update missing rule i d.
func TestMRApprovalRuleUpdate_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateRule(context.Background(), client, UpdateRuleInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("UpdateRule() expected error for missing approval_rule_id")
	}
}

// TestMRApprovalRuleUpdate_MissingProject verifies the behavior of m r approval rule update missing project.
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

// TestMRApprovalRuleDelete_Success verifies the behavior of m r approval rule delete success.
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

// TestMRApprovalRuleDelete_MissingRuleID verifies the behavior of m r approval rule delete missing rule i d.
func TestMRApprovalRuleDelete_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteRule(context.Background(), client, DeleteRuleInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("DeleteRule() expected error for missing approval_rule_id")
	}
}

// TestMRApprovalRuleDelete_MissingProject verifies the behavior of m r approval rule delete missing project.
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

// assertErrContains is an internal helper for the mrapprovals package.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies the behavior of m r i i d required validation.
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

// TestApprovalRuleIDRequired_Validation verifies the behavior of approval rule i d required validation.
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
