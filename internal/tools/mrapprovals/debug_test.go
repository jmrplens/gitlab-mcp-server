// debug_test.go contains unit tests for the merge request approval MCP tool handlers.
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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestDebug_ErrorType verifies the behavior of debug error type.
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
		t.Logf("ErrorResponse.Response.StatusCode: %d", glErr.Response.StatusCode)
	} else {
		t.Log("error is NOT a *gl.ErrorResponse")
		t.Logf("unwrapped: %v", errors.Unwrap(err))
		t.Logf("fmt: %s", fmt.Sprintf("%+v", err))
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

// ---------------------------------------------------------------------------
// Config — canceled context & server error
// ---------------------------------------------------------------------------.

// TestConfig_CancelledContext verifies the behavior of config cancelled context.
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

// TestConfig_ServerError verifies the behavior of config server error.
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

// TestReset_CancelledContext verifies the behavior of reset cancelled context.
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

// TestReset_ServerError verifies the behavior of reset server error.
func TestReset_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"fail"}`)
	}))

	err := Reset(context.Background(), client, ResetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// ---------------------------------------------------------------------------
// CreateRule — canceled context, server error & ApprovalProjectRuleID path
// ---------------------------------------------------------------------------.

// TestCreateRule_CancelledContext verifies the behavior of create rule cancelled context.
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

// TestCreateRule_ServerError verifies the behavior of create rule server error.
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

// TestCreateRule_WithApprovalProjectRuleID verifies the behavior of create rule with approval project rule i d.
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

// TestUpdateRule_CancelledContext verifies the behavior of update rule cancelled context.
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

// TestUpdateRule_ServerError verifies the behavior of update rule server error.
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

// TestUpdateRule_AllOptionalFields verifies the behavior of update rule all optional fields.
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

// TestDeleteRule_CancelledContext verifies the behavior of delete rule cancelled context.
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

// TestDeleteRule_ServerError verifies the behavior of delete rule server error.
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

// TestRuleToOutput_WithUsersAndGroups verifies the behavior of rule to output with users and groups.
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

// TestRuleToOutput_NilGroupEntry verifies the behavior of rule to output nil group entry.
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

// TestConfig_ToOutputNilEntries verifies the behavior of config to output nil entries.
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

// TestFormatStateMarkdown_WithRules verifies the behavior of format state markdown with rules.
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

// TestFormatStateMarkdown_Empty verifies the behavior of format state markdown empty.
func TestFormatStateMarkdown_Empty(t *testing.T) {
	md := FormatStateMarkdown(StateOutput{})
	assertContains(t, md, "**Rules overwritten**: No")
	assertContains(t, md, "No approval rules configured.")
}

// ---------------------------------------------------------------------------
// FormatRulesMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatRulesMarkdown_WithRules verifies the behavior of format rules markdown with rules.
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

// TestFormatRulesMarkdown_Empty verifies the behavior of format rules markdown empty.
func TestFormatRulesMarkdown_Empty(t *testing.T) {
	md := FormatRulesMarkdown(RulesOutput{})
	assertContains(t, md, "## MR Approval Rules (0)")
	assertContains(t, md, "No approval rules configured.")
}

// ---------------------------------------------------------------------------
// FormatConfigMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatConfigMarkdown_Full verifies the behavior of format config markdown full.
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

// TestFormatConfigMarkdown_Minimal verifies the behavior of format config markdown minimal.
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

// TestFormatRuleMarkdown_Full verifies the behavior of format rule markdown full.
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

// TestFormatRuleMarkdown_Minimal verifies the behavior of format rule markdown minimal.
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

// ---------------------------------------------------------------------------
// TestRegisterTools_CallAllThroughMCP — full MCP roundtrip
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newApprovalsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_approval_state", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_rules", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_config", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_reset", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_rule_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "name": "R", "approvals_required": 1, "approval_project_rule_id": 0, "user_ids": []any{}, "group_ids": []any{}}},
		{"gitlab_mr_approval_rule_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5, "name": "U", "approvals_required": 1, "user_ids": []any{}, "group_ids": []any{}}},
		{"gitlab_mr_approval_rule_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newApprovalsMCPSession is an internal helper for the mrapprovals package.
func newApprovalsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		// approval_state
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approval_state"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"approval_rules_overwritten": false,
				"rules": [{"id":1,"name":"Default","rule_type":"any_approver","approvals_required":1,"approved":true}]
			}`)

		// approval_rules GET
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approval_rules"):
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"Default","rule_type":"any_approver","approvals_required":1,"approved":true}]`)

		// approval_rules POST (create)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/approval_rules"):
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":5,"name":"R","rule_type":"regular","approvals_required":1,"approved":false,
				"approved_by":[],"eligible_approvers":[],"users":[],"groups":[]
			}`)

		// approval_rules PUT (update)
		case r.Method == http.MethodPut && strings.Contains(path, "/approval_rules/"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":5,"name":"U","rule_type":"regular","approvals_required":1,"approved":false,
				"approved_by":[],"eligible_approvers":[],"users":[],"groups":[]
			}`)

		// approval_rules DELETE
		case r.Method == http.MethodDelete && strings.Contains(path, "/approval_rules/"):
			w.WriteHeader(http.StatusNoContent)

		// approvals (config)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approvals"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"project_id":42,"title":"MR","state":"opened",
				"approved":false,"approvals_required":1,"approvals_left":1,
				"approvals_before_merge":0,"has_approval_rules":true,
				"user_has_approved":false,"user_can_approve":true,
				"approved_by":[],"suggested_approvers":[]
			}`)

		// reset_approvals
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/reset_approvals"):
			w.WriteHeader(http.StatusAccepted)

		default:
			http.NotFound(w, r)
		}
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// assertContains is an internal helper for the mrapprovals package.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected string to contain %q, got:\n%s", substr, s)
	}
}

// assertNotContains is an internal helper for the mrapprovals package.
func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected string NOT to contain %q, got:\n%s", substr, s)
	}
}

// ---------------------------------------------------------------------------
// Fake data factories (avoid import cycle with gl types in helpers)
// ---------------------------------------------------------------------------.

// fakeApprovalRule is an internal helper for the mrapprovals package.
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

// fakeApprovalRuleNilGroup is an internal helper for the mrapprovals package.
func fakeApprovalRuleNilGroup(t *testing.T) gl.MergeRequestApprovalRule {
	t.Helper()
	return gl.MergeRequestApprovalRule{
		Groups: []*gl.Group{nil, {Name: "Good"}},
	}
}

// fakeConfigNilEntries is an internal helper for the mrapprovals package.
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
