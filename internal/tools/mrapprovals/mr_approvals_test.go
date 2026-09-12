// mr_approvals_test.go contains unit tests for the merge request approval MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrapprovals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	if len(r.ApprovedBy) != 1 || r.ApprovedBy[0] == nil || r.ApprovedBy[0].Name != "Alice" {
		t.Errorf("ApprovedBy = %v, want one entry [Alice]", r.ApprovedBy)
	}
	if len(r.EligibleApprovers) != 2 {
		t.Errorf("EligibleApprovers count = %d, want 2", len(r.EligibleApprovers))
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
	if len(r.EligibleApprovers) != exp.eligibleCount {
		t.Errorf("EligibleApprovers count = %d, want %d", len(r.EligibleApprovers), exp.eligibleCount)
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
		{"CodeOwners", 0, approvalRuleExpected{1, "Code Owners", "code_owner", 1, 2}},
		{"SecurityReview", 1, approvalRuleExpected{2, "Security Review", "regular", 2, 1}},
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
	if rule.EligibleApprovers != nil {
		t.Errorf("expected nil EligibleApprovers, got %v", rule.EligibleApprovers)
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
	if len(rule.EligibleApprovers) != 3 {
		t.Errorf("EligibleApprovers count = %d, want 3", len(rule.EligibleApprovers))
	}
	if rule.ID != 5 || rule.Name != "Team Lead" || rule.ApprovalsRequired != 3 {
		t.Errorf("unexpected output: %+v", rule)
	}
}

// TestApprovalRuleToOutputSkips_NilEntries verifies ApprovalRuleToOutputSkips when nil entries.
func TestApprovalRuleToOutputSkips_NilEntries(t *testing.T) {
	rule := RuleToOutput(&gl.MergeRequestApprovalRule{
		ApprovedBy:        []*gl.BasicUser{nil, {Name: "Valid"}},
		EligibleApprovers: []*gl.BasicUser{{Name: "E1"}, nil},
	})
	if len(rule.EligibleApprovers) != 1 || rule.EligibleApprovers[0] == nil || rule.EligibleApprovers[0].Name != "E1" {
		t.Errorf("EligibleApprovers = %v, want [E1]", rule.EligibleApprovers)
	}
}

// ---------------------------------------------------------------------------
// Config (GetConfiguration) tests
// ---------------------------------------------------------------------------.

// configResponse is what GitLab answers at
// GET /projects/:id/merge_requests/:merge_request_iid/approvals: four fields, on
// every tier.
//
// It used to carry ten more, invented here, and that is how the phantom fields
// survived a green test for as long as they did: the fixture was written from
// the SDK struct rather than from a response, so the assertions proved our
// converter agreed with our own invention. The extra keys are kept below on
// purpose, to prove they are dropped rather than merely absent.
const configResponse = `{
	"approved": true,
	"user_has_approved": true, "user_can_approve": false,
	"approved_by": [{"user": {"name": "Alice"}, "approved_at": "2026-01-15T10:30:00Z"}]
}`

// configResponseWithPOSTFields is the same answer with the deprecated POST's
// twenty extra fields bolted on, which is what an old GitLab or a proxy might
// send. Nothing in ConfigOutput may pick them up.
const configResponseWithPOSTFields = `{
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
	if !out.UserHasApproved {
		t.Error("UserHasApproved = false, want true")
	}
	if out.UserCanApprove {
		t.Error("UserCanApprove = true, want false")
	}
	if len(out.ApprovedBy) != 1 || out.ApprovedBy[0] == nil || out.ApprovedBy[0].User == nil || out.ApprovedBy[0].User.Name != "Alice" {
		t.Errorf("ApprovedBy = %v, want one entry with user Alice", out.ApprovedBy)
	}
	if out.ApprovedBy[0].ApprovedAt != "2026-01-15T10:30:00Z" {
		t.Errorf("ApprovedAt = %q, want %q", out.ApprovedBy[0].ApprovedAt, "2026-01-15T10:30:00Z")
	}
}

// TestMRApprovalConfig_TheDeprecatedPOSTsFields_AreNotPublished pins the repair.
// The twenty extra fields this output used to carry are the response of the POST
// at the same path, deprecated in GitLab 16.0 and never called here, so an
// answer that carries them anyway must still publish four keys and no more. A
// field test asserts the shape rather than one value, because the defect was
// that the shape was somebody else's.
func TestMRApprovalConfig_TheDeprecatedPOSTsFields_AreNotPublished(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/10/approvals" {
			testutil.RespondJSON(w, http.StatusOK, configResponseWithPOSTFields)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Config(context.Background(), client, ConfigInput{ProjectID: "42", MRIID: 10})
	if err != nil {
		t.Fatalf("Config() unexpected error: %v", err)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal the output: %v", err)
	}
	var keys map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(encoded, &keys); unmarshalErr != nil {
		t.Fatalf("read the output back: %v", unmarshalErr)
	}
	published := slices.Sorted(maps.Keys(keys))
	want := []string{"approved", "approved_by", "user_can_approve", "user_has_approved"}
	if !slices.Equal(published, want) {
		t.Errorf("published %v, want exactly %v: every other key of the SDK type belongs to the deprecated POST", published, want)
	}
	if !out.Approved || !out.UserHasApproved || len(out.ApprovedBy) != 1 {
		t.Errorf("output = %+v, want the four real fields still read", out)
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
	if len(out.Users) != 1 || out.Users[0] == nil || out.Users[0].Name != "Alice" {
		t.Errorf("Users = %v, want [Alice]", out.Users)
	}
	if len(out.Groups) != 1 || out.Groups[0] == nil || out.Groups[0].Name != "Security" {
		t.Errorf("Groups = %v, want [Security]", out.Groups)
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

// TestMRApproval_Config404_DoesNotBlameTheLicence verifies that a 404 here is
// reported as a wrong identifier and not as a missing tier.
//
// It used to assert the opposite, and the message it pinned was false: GitLab
// answers this endpoint on Community Edition, which the CE end-to-end suite
// calls successfully. A model reading "requires GitLab Premium" off a mistyped
// merge_request_iid stops trying instead of correcting the number.
func TestMRApproval_Config404_DoesNotBlameTheLicence(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Config(context.Background(), client, ConfigInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("Config() expected error for a 404, got nil")
	}
	if strings.Contains(err.Error(), "Premium") || strings.Contains(err.Error(), "Community Edition") {
		t.Errorf("Config() error blames the license for an endpoint every tier serves: %v", err)
	}
	if !strings.Contains(err.Error(), "merge_request_iid") {
		t.Errorf("Config() error should point at the identifiers, got: %v", err)
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error missing %q: %v", want, err)
			}
		})
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
	if len(out.Users) != 1 {
		t.Errorf("Users count = %d, want 1", len(out.Users))
	}
	if len(out.Groups) != 1 {
		t.Errorf("Groups count = %d, want 1", len(out.Groups))
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
	if len(out.Users) != 2 {
		t.Errorf("Users count = %d, want 2", len(out.Users))
	}
	if len(out.Groups) != 1 {
		t.Errorf("Groups count = %d, want 1", len(out.Groups))
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
	if len(out.Groups) != 1 || out.Groups[0] == nil || out.Groups[0].Name != "Good" {
		t.Errorf("Groups = %v, want [Good]", out.Groups)
	}
}

// ---------------------------------------------------------------------------
// configToOutput — nil approved_by entry
// ---------------------------------------------------------------------------.

// TestConfig_ToOutputNilEntries verifies Config when to output nil entries.
func TestConfig_ToOutputNilEntries(t *testing.T) {
	c := fakeConfigNilEntries(t)
	out := configToOutput(&c)
	// The nil *MergeRequestApproverUser element is skipped; the {User: nil}
	// element is preserved (1:1 SDK fidelity) with a nil user object, leaving
	// two output entries.
	if len(out.ApprovedBy) != 2 {
		t.Fatalf("ApprovedBy count = %d, want 2", len(out.ApprovedBy))
	}
	if out.ApprovedBy[0] == nil || out.ApprovedBy[0].User != nil {
		t.Errorf("ApprovedBy[0] = %v, want preserved entry with nil user", out.ApprovedBy[0])
	}
	if out.ApprovedBy[1] == nil || out.ApprovedBy[1].User == nil || out.ApprovedBy[1].User.Name != "Alice" {
		t.Errorf("ApprovedBy[1] = %v, want user Alice", out.ApprovedBy[1])
	}
}

// ---------------------------------------------------------------------------
// FormatStateMarkdown tests
// ---------------------------------------------------------------------------.

// stateHints is the guidance section an approval state card with rules closes
// with, and stateEmptyHints the one it closes with when there are none.
const (
	stateHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'merge_request.approve' to approve this merge request\n" +
		"- Use action 'merge_request.unapprove' to withdraw an approval\n"
	stateEmptyHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'merge_request.approval_rules' to list the rules configured on this merge request\n" +
		"- Use action 'merge_request.approval_rule_create' to add an approval rule\n"
)

// TestFormatStateMarkdown_WithRules verifies the whole rendering of an approval
// state: the card's own rows, then the rules as a nested collection under an H3
// of their own. The rows precede the table, which is what keeps every row a row
// rather than a line of literal pipes below the table's last one.
func TestFormatStateMarkdown_WithRules(t *testing.T) {
	s := StateOutput{
		ApprovalRulesOverwritten: true,
		Rules: []StateRuleOutput{
			{ID: 1, Name: "Security", RuleType: "regular", ApprovalsRequired: 2, Approved: true, ApprovedBy: []*BasicUserOutput{{Name: "Alice", Username: "alice"}}},
			{ID: 2, Name: "QA", RuleType: "code_owner", ApprovalsRequired: 1},
		},
	}
	want := "## MR Approval State\n\n" +
		"- **Rules overwritten**: ✅\n" +
		"- **Rules**: 2\n\n" +
		"### Rules\n\n" +
		"| ID | Name | Type | Required | Approved | Approved By |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | Security | regular | 2 | ✅ | @alice |\n" +
		"| 2 | QA | code_owner | 1 | ❌ |  |\n" + stateHints
	if got := FormatStateMarkdown(s); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatStateMarkdown_Empty verifies the whole rendering when no rule is
// configured: the card says so in one sentence and opens no empty table.
func TestFormatStateMarkdown_Empty(t *testing.T) {
	want := "## MR Approval State\n\n" +
		"- **Rules overwritten**: ❌\n" +
		"- **Rules**: 0\n\n" +
		"No approval rules are configured for this merge request.\n" + stateEmptyHints
	if got := FormatStateMarkdown(StateOutput{}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatRulesMarkdown tests
// ---------------------------------------------------------------------------.

// rulesHints is the guidance section an approval rules listing closes with,
// and rulesLinkedHints the same section for a listing whose approvers carry a
// profile URL, which is the only link the table can hold.
const (
	rulesHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'merge_request.approval_rule_create' to add a rule\n" +
		"- Use action 'merge_request.approval_rule_update' to change an existing rule\n" +
		"- Use action 'merge_request.approval_rule_delete' to remove a rule\n"
	rulesLinkedHints = "\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'merge_request.approval_rule_create' to add a rule\n" +
		"- Use action 'merge_request.approval_rule_update' to change an existing rule\n" +
		"- Use action 'merge_request.approval_rule_delete' to remove a rule\n"
	rulesTableHead = "| ID | Name | Type | Required | Eligible |\n| --- | --- | --- | --- | --- |\n"
)

// TestFormatRulesMarkdown_WithRules verifies the whole rendering of an approval
// rules listing. There is no Approved column: the three approval_rules routes
// present an entity that does not expose it, and a column claiming one would
// print false on every row. The instruction to keep the table's links is
// absent because this table has none: nobody here carries a profile URL.
func TestFormatRulesMarkdown_WithRules(t *testing.T) {
	out := RulesOutput{
		Rules: []RuleOutput{
			{ID: 10, Name: "Team", RuleType: "regular", ApprovalsRequired: 1, EligibleApprovers: []*BasicUserOutput{{Name: "Eve", Username: "eve"}, {Name: "Frank"}}},
		},
	}
	want := "## MR Approval Rules (1)\n\n" + rulesTableHead +
		"| 10 | Team | regular | 1 | @eve, Frank |\n" + rulesHints
	if got := FormatRulesMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRulesMarkdown_LinkedApprovers verifies that a listing whose
// approvers carry a profile URL links them and asks the model to keep the
// links, which the one above must not do.
func TestFormatRulesMarkdown_LinkedApprovers(t *testing.T) {
	out := RulesOutput{
		Rules: []RuleOutput{
			{ID: 11, Name: "Leads", RuleType: "regular", ApprovalsRequired: 2, EligibleApprovers: []*BasicUserOutput{
				{Name: "Eve", Username: "eve", WebURL: "https://gitlab.example.com/eve"},
			}},
		},
	}
	want := "## MR Approval Rules (1)\n\n" + rulesTableHead +
		"| 11 | Leads | regular | 2 | [@eve](https://gitlab.example.com/eve) |\n" + rulesLinkedHints
	if got := FormatRulesMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRulesMarkdown_Empty verifies that a merge request with no approval
// rule renders the one-sentence empty message and no heading counting zero.
func TestFormatRulesMarkdown_Empty(t *testing.T) {
	want := "No approval rules found.\n"
	if got := FormatRulesMarkdown(RulesOutput{}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatConfigMarkdown tests
// ---------------------------------------------------------------------------.

// configHints is the guidance section the approvals card closes with.
const configHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'merge_request.approve' to approve this merge request\n" +
	"- Use action 'merge_request.unapprove' to withdraw your approval\n" +
	"- Use action 'merge_request.approval_state' to see how many approvals are required and left\n" +
	"- Use action 'merge_request.approval_rules' to see every configured rule\n"

// TestFormatConfigMarkdown_Full verifies the whole rendering of the approvals
// card: four rows and the guidance. The rows that used to print here read
// fields GitLab does not answer with at this endpoint, so each printed a zero;
// the count and the rules live on approval_state, which the hints point at.
func TestFormatConfigMarkdown_Full(t *testing.T) {
	c := ConfigOutput{
		Approved:        true,
		UserHasApproved: true,
		UserCanApprove:  false,
		ApprovedBy:      []*MergeRequestApproverUserOutput{{User: &BasicUserOutput{Name: "Alice", Username: "alice"}}},
	}
	want := "## MR Approvals\n\n" +
		"- **Approved**: ✅\n" +
		"- **You have approved**: ✅\n" +
		"- **You can approve**: ❌\n" +
		"- **Approved By**: @alice\n" + configHints
	if got := FormatConfigMarkdown(c); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatConfigMarkdown_Minimal verifies that a merge request nobody has
// approved renders the three flags and no approver row at all.
func TestFormatConfigMarkdown_Minimal(t *testing.T) {
	want := "## MR Approvals\n\n" +
		"- **Approved**: ❌\n" +
		"- **You have approved**: ❌\n" +
		"- **You can approve**: ❌\n" + configHints
	if got := FormatConfigMarkdown(ConfigOutput{}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatConfigMarkdown_ApprovedByWithDate verifies that the approver row
// carries the moment each approval happened, in the display form every other
// timestamp takes, and nothing at all for an approval GitLab dated nowhere.
func TestFormatConfigMarkdown_ApprovedByWithDate(t *testing.T) {
	c := ConfigOutput{
		ApprovedBy: []*MergeRequestApproverUserOutput{
			{User: &BasicUserOutput{Name: "Alice", Username: "alice"}, ApprovedAt: "2026-03-15T14:00:00Z"},
			{User: &BasicUserOutput{Name: "Bob", Username: "bob"}, ApprovedAt: ""},
		},
	}
	want := "## MR Approvals\n\n" +
		"- **Approved**: ❌\n" +
		"- **You have approved**: ❌\n" +
		"- **You can approve**: ❌\n" +
		"- **Approved By**: @alice (15 Mar 2026 14:00 UTC), @bob\n" + configHints
	if got := FormatConfigMarkdown(c); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatConfigMarkdown_SkipsNilApprovers verifies the config Markdown
// formatter skips approver entries with a nil user object while rendering the
// remaining named approvers, and falls back to the display name for a user
// GitLab sent without a username.
func TestFormatConfigMarkdown_SkipsNilApprovers(t *testing.T) {
	c := ConfigOutput{
		ApprovedBy: []*MergeRequestApproverUserOutput{
			nil,
			{User: nil},
			{User: &BasicUserOutput{Name: "Carol"}},
		},
	}
	want := "## MR Approvals\n\n" +
		"- **Approved**: ❌\n" +
		"- **You have approved**: ❌\n" +
		"- **You can approve**: ❌\n" +
		"- **Approved By**: Carol\n" + configHints
	if got := FormatConfigMarkdown(c); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatRuleMarkdown tests
// ---------------------------------------------------------------------------.

// ruleHints is the guidance section an approval rule card closes with.
const ruleHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'merge_request.approval_rule_update' to modify this rule\n" +
	"- Use action 'merge_request.approval_rule_delete' to remove this rule\n" +
	"- Use action 'merge_request.merge' to merge once the rule is satisfied\n"

// TestFormatRuleMarkdown_Full verifies the whole rendering of one approval
// rule, including the two conditions the card never used to show: whether the
// rule overrides the project's, and whether it names groups the caller cannot
// see, which is why the eligible list can look shorter than it is.
func TestFormatRuleMarkdown_Full(t *testing.T) {
	r := RuleOutput{
		ID:                   1,
		Name:                 "Team Leads",
		RuleType:             "regular",
		ReportType:           "code_coverage",
		Section:              "backend",
		ApprovalsRequired:    2,
		Overridden:           true,
		ContainsHiddenGroups: true,
		EligibleApprovers:    []*BasicUserOutput{{Name: "Alice", Username: "alice"}, {Name: "Bob", Username: "bob"}},
		Users:                []*BasicUserOutput{{Name: "Alice", Username: "alice"}},
		Groups:               []*GroupOutput{{Name: "Leads", FullPath: "acme/leads"}},
	}
	want := "## Approval Rule: Team Leads\n\n" +
		"- **ID**: 1\n" +
		"- **Type**: regular\n" +
		"- **Report Type**: code_coverage\n" +
		"- **Section**: backend\n" +
		"- **Approvals Required**: 2\n" +
		"- **Overridden**: ✅\n" +
		"- ⚠️ **Contains groups you cannot see**\n" +
		"- **Eligible**: @alice, @bob\n" +
		"- **Users**: @alice\n" +
		"- **Groups**: acme/leads\n" + ruleHints
	if got := FormatRuleMarkdown(r); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRuleMarkdown_Minimal verifies that a rule GitLab sent nothing
// optional for shows no label with nothing after it.
func TestFormatRuleMarkdown_Minimal(t *testing.T) {
	r := RuleOutput{
		ID:                3,
		Name:              "Basic",
		RuleType:          "any_approver",
		ApprovalsRequired: 0,
	}
	want := "## Approval Rule: Basic\n\n" +
		"- **ID**: 3\n" +
		"- **Type**: any_approver\n" +
		"- **Approvals Required**: 0\n" +
		"- **Overridden**: ❌\n" + ruleHints
	if got := FormatRuleMarkdown(r); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
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

// ---------------------------------------------------------------------------
// overridden raw-fetch + version-tolerance tests
// ---------------------------------------------------------------------------.

// TestState_OverriddenSurfaced verifies the raw-fetched, SDK-missing "overridden"
// boolean on an approval_state rule is surfaced on RuleOutput.Overridden.
func TestState_OverriddenSurfaced(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_state" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"approval_rules_overwritten": true,
				"rules": [
					{"id": 1, "name": "Ruby", "rule_type": "regular", "approvals_required": 2, "approved": true, "overridden": true}
				]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := State(context.Background(), client, StateInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	if !out.Rules[0].Overridden {
		t.Errorf("expected Overridden=true, got %+v", out.Rules[0])
	}
}

// TestRules_OverriddenSurfaced verifies the raw-fetched "overridden" boolean is
// surfaced on each rule of the approval_rules list response.
func TestRules_OverriddenSurfaced(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testApprovalRulesPath && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 1, "name": "security", "rule_type": "regular", "approvals_required": 3, "overridden": true}
			]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := Rules(context.Background(), client, RulesInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 1 || !out.Rules[0].Overridden {
		t.Fatalf("expected one rule with Overridden=true, got %+v", out.Rules)
	}
}

// TestCreateRule_OverriddenSurfaced verifies the raw create response surfaces
// the documented "overridden" boolean.
func TestCreateRule_OverriddenSurfaced(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == testApprovalRulesPath {
			testutil.RespondJSON(w, http.StatusCreated, `{"id": 7, "name": "new", "rule_type": "regular", "approvals_required": 1, "overridden": true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))

	out, err := CreateRule(context.Background(), client, CreateRuleInput{
		ProjectID: "42", MRIID: 1, Name: "new", ApprovalsRequired: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Overridden || out.ID != 7 {
		t.Fatalf("expected created rule with Overridden=true, got %+v", out)
	}
}

// TestUpdateRule_OverriddenSurfaced verifies the raw update response surfaces
// the documented "overridden" boolean.
func TestUpdateRule_OverriddenSurfaced(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/1/approval_rules/5" {
			testutil.RespondJSON(w, http.StatusOK, `{"id": 5, "name": "upd", "rule_type": "regular", "approvals_required": 2, "overridden": true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"nf"}`)
	}))

	out, err := UpdateRule(context.Background(), client, UpdateRuleInput{
		ProjectID: "42", MRIID: 1, ApprovalRuleID: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Overridden {
		t.Fatalf("expected Overridden=true, got %+v", out)
	}
}

// TestRules_OverriddenVersionTolerant verifies that older GitLab instances which
// omit the "overridden" field decode successfully (no error) and that the absent
// field marshals out of the rendered JSON via the omitempty tag. This pins the
// version-tolerance guarantee: a single Do(&superset) unmarshal treats an absent
// field as its zero value rather than failing the tool.
func TestRules_OverriddenVersionTolerant(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testApprovalRulesPath && r.Method == http.MethodGet {
			// Response without the "overridden" field (older instance).
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id": 1, "name": "security", "rule_type": "regular", "approvals_required": 3}
			]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := Rules(context.Background(), client, RulesInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf("expected success when overridden omitted, got error: %v", err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	if out.Rules[0].Overridden {
		t.Errorf("expected Overridden=false (zero) when omitted, got true")
	}
	b, err := json.Marshal(out.Rules[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "overridden") {
		t.Errorf("expected omitempty to drop overridden from JSON, got %s", b)
	}
}

// TestRawRuleToOutput_NilNested verifies rawRuleToOutput is nil-safe for absent
// nested approver/user/group/source-rule objects and maps the scalar fields.
func TestRawStateRuleToOutput_NilNested(t *testing.T) {
	out := rawStateRuleToOutput(&mergeRequestApprovalRuleAPI{
		ID: 3, Name: "n", RuleType: "regular", ReportType: "rt", Section: "s",
		ApprovalsRequired: 4, Approved: true, ContainsHiddenGroups: true, Overridden: true,
	})
	if out.ID != 3 || out.Name != "n" || out.RuleType != "regular" || out.ReportType != "rt" ||
		out.Section != "s" || out.ApprovalsRequired != 4 || !out.Approved ||
		!out.ContainsHiddenGroups || !out.Overridden {
		t.Fatalf("rawStateRuleToOutput scalar mapping = %+v", out)
	}
	if out.ApprovedBy != nil || out.EligibleApprovers != nil || out.Users != nil ||
		out.Groups != nil || out.SourceRule != nil {
		t.Errorf("expected nil nested objects, got %+v", out)
	}
}

// TestRawStateRuleToOutput_NestedSubsets verifies the state converter projects nested
// objects through the documented-reference-subset converters (BasicUserOutput,
// GroupOutput, ProjectApprovalRuleOutput).
func TestRawStateRuleToOutput_NestedSubsets(t *testing.T) {
	out := rawStateRuleToOutput(&mergeRequestApprovalRuleAPI{
		ID:                1,
		ApprovedBy:        []*gl.BasicUser{{Name: "Ann"}},
		EligibleApprovers: []*gl.BasicUser{{Name: "Eli"}},
		Users:             []*gl.BasicUser{{Name: "Usr"}},
		Groups:            []*gl.Group{{Name: "Grp"}},
		SourceRule:        &gl.ProjectApprovalRule{ID: 9, Name: "src"},
	})
	if len(out.ApprovedBy) != 1 || out.ApprovedBy[0].Name != "Ann" {
		t.Errorf("ApprovedBy = %+v", out.ApprovedBy)
	}
	if len(out.EligibleApprovers) != 1 || out.EligibleApprovers[0].Name != "Eli" {
		t.Errorf("EligibleApprovers = %+v", out.EligibleApprovers)
	}
	if len(out.Users) != 1 || out.Users[0].Name != "Usr" {
		t.Errorf("Users = %+v", out.Users)
	}
	if len(out.Groups) != 1 || out.Groups[0].Name != "Grp" {
		t.Errorf("Groups = %+v", out.Groups)
	}
	if out.SourceRule == nil || out.SourceRule.ID != 9 || out.SourceRule.Name != "src" {
		t.Errorf("SourceRule = %+v", out.SourceRule)
	}
}

// TestRawHelpers_NewRequestError verifies the raw-fetch helpers propagate the
// error from client.GL().NewRequest when the path cannot be parsed (an invalid
// percent-escape makes url.PathUnescape fail), covering the early-return branch.
func TestRawHelpers_NewRequestError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := context.Background()
	const badPath = "projects/%zz/merge_requests/1/approval_rules"

	if _, err := rawListApprovalRules(ctx, client, badPath); err == nil {
		t.Error("rawListApprovalRules: expected error for malformed path")
	}
	if _, err := rawApprovalState(ctx, client, badPath); err == nil {
		t.Error("rawApprovalState: expected error for malformed path")
	}
	if _, err := rawMutateApprovalRule(ctx, client, http.MethodPost, badPath, nil); err == nil {
		t.Error("rawMutateApprovalRule: expected error for malformed path")
	}
}
