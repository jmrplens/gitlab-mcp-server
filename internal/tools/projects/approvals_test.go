// approvals_test.go contains unit tests for project approval configuration
// and approval rule operations.
package projects

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestFormatApprovalConfigMarkdown_NonEmpty verifies the whole approval
// configuration card, every setting a flag rendered as its glyph.
func TestFormatApprovalConfigMarkdown_NonEmpty(t *testing.T) {
	md := FormatApprovalConfigMarkdown(ApprovalConfigOutput{
		ApprovalsBeforeMerge: 2, ResetApprovalsOnPush: true,
	})
	want := "## Approval Configuration\n\n" +
		"- **Approvals before merge**: 2\n" +
		"- **Reset approvals on push**: ✅\n" +
		"- **Disable overriding approvers per MR**: ❌\n" +
		"- **Author self-approval**: ❌\n" +
		"- **Disable committers approval**: ❌\n" +
		"- **Require reauthentication to approve**: ❌\n" +
		"- **Selective code owner removals**: ❌\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project.approval_config_change' to modify these settings\n" +
		"- Use action 'project.approval_rule_list' to see the approval rules\n"
	if md != want {
		t.Errorf("FormatApprovalConfigMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatApprovalRuleMarkdown_NonEmpty verifies the whole approval rule
// card: the rule GitLab sent no type, users or groups for writes none of those
// rows.
func TestFormatApprovalRuleMarkdown_NonEmpty(t *testing.T) {
	md := FormatApprovalRuleMarkdown(ApprovalRuleOutput{
		ID: 10, Name: "Security Review", ApprovalsRequired: 2,
	})
	want := "## Approval Rule: Security Review\n\n" +
		"- **ID**: 10\n" +
		"- **Approvals Required**: 2\n" +
		"- **Applies to all protected branches**: ❌\n" +
		"- **Contains hidden groups**: ❌\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project.approval_rule_update' to modify this rule\n" +
		"- Use action 'project.approval_rule_delete' to remove it\n"
	if md != want {
		t.Errorf("FormatApprovalRuleMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestApprovalRuleHandlers_ReadTheCoverageThresholdOffTheCapture verifies the
// one key client-go's ProjectApprovalRule does not model reaches the output on
// every rule handler, and that an answer whose threshold is not a number is
// an error rather than a rule with the key quietly dropped.
func TestApprovalRuleHandlers_ReadTheCoverageThresholdOffTheCapture(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		body     string
		call     func(context.Context, *testing.T, string) (*int64, error)
		wantFail bool
	}{
		{name: "get", body: `{"id":10,"report_type":"code_coverage","coverage_minimum_threshold":80}`, call: getRuleThreshold},
		{name: "list", body: `[{"id":10,"report_type":"code_coverage","coverage_minimum_threshold":80}]`, call: listRuleThreshold},
		{name: "get with a threshold that is not a number", body: `{"id":10,"coverage_minimum_threshold":"high"}`, call: getRuleThreshold, wantFail: true},
		{name: "list with a threshold that is not a number", body: `[{"id":10,"coverage_minimum_threshold":"high"}]`, call: listRuleThreshold, wantFail: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.call(t.Context(), t, testCase.body)
			if testCase.wantFail {
				if err == nil {
					t.Error("handler succeeded on a threshold its extra cannot hold")
				}
				return
			}
			if err != nil || got == nil || *got != 80 {
				t.Errorf("threshold = %v, %v; want 80", got, err)
			}
		})
	}
}

// getRuleThreshold serves body as the answer to a single approval rule and
// returns the threshold the handler read.
func getRuleThreshold(ctx context.Context, t *testing.T, body string) (*int64, error) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	out, err := GetApprovalRule(ctx, client, GetApprovalRuleInput{ProjectID: "42", RuleID: 10})
	return out.CoverageMinimumThreshold, err
}

// listRuleThreshold serves body as a page of approval rules and returns the
// first row's threshold.
func listRuleThreshold(ctx context.Context, t *testing.T, body string) (*int64, error) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	out, err := ListApprovalRules(ctx, client, ListApprovalRulesInput{ProjectID: "42"})
	if err != nil || len(out.Rules) == 0 {
		return nil, err
	}
	return out.Rules[0].CoverageMinimumThreshold, nil
}

// TestListApprovalRules_OrderAndSort_ReachTheQuery holds the keyset ordering
// of a rule listing to the query GitLab receives, since each is copied under
// an emptiness guard and a guard that drops the copy still lists the rules,
// in whatever order the server chose.
func TestListApprovalRules_OrderAndSort_ReachTheQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, pathProject42ApprovalRules)
		testutil.AssertQueryParam(t, r, "order_by", "name")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	if _, err := ListApprovalRules(t.Context(), client, ListApprovalRulesInput{ProjectID: "42", OrderBy: "name", Sort: "desc"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreateApprovalRule_SendsExactlyWhatTheCallerSet holds the create body
// to the caller's own settings, both when only the required ones are given
// and when every optional one is: an optional list that is not set must not
// reach GitLab as null, and one that is must reach it with its members.
func TestCreateApprovalRule_SendsExactlyWhatTheCallerSet(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input CreateApprovalRuleInput
		want  string
	}{
		{
			name:  "required only",
			input: CreateApprovalRuleInput{ProjectID: "42", Name: "review", ApprovalsRequired: 2},
			want:  `{"name":"review","approvals_required":2}`,
		},
		{
			name: "every setting",
			input: CreateApprovalRuleInput{
				ProjectID: "42", Name: "review", ApprovalsRequired: 2,
				RuleType: "regular", ReportType: "code_coverage",
				UserIDs: []int64{11, 12}, GroupIDs: []int64{21}, ProtectedBranchIDs: []int64{31},
				Usernames: []string{"alice"}, AppliesToAllProtectedBranches: new(false),
			},
			want: `{"name":"review","approvals_required":2,"rule_type":"regular","report_type":"code_coverage",` +
				`"user_ids":[11,12],"group_ids":[21],"protected_branch_ids":[31],"usernames":["alice"],"applies_to_all_protected_branches":false}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := bodyAssertingClient(t, http.MethodPost, pathProject42ApprovalRules, testCase.want, http.StatusCreated, approvalRuleJSON)
			if _, err := CreateApprovalRule(t.Context(), client, testCase.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
		})
	}
}

// TestUpdateApprovalRule_SendsExactlyWhatTheCallerSet holds the update body
// to the caller's own settings: an update naming only the rule sends an empty
// object, and one naming every setting sends each under its own key.
func TestUpdateApprovalRule_SendsExactlyWhatTheCallerSet(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input UpdateApprovalRuleInput
		want  string
	}{
		{
			name:  "rule only",
			input: UpdateApprovalRuleInput{ProjectID: "42", RuleID: 10},
			want:  `{}`,
		},
		{
			name: "every setting",
			input: UpdateApprovalRuleInput{
				ProjectID: "42", RuleID: 10, Name: "renamed", ApprovalsRequired: new(int64(3)),
				UserIDs: []int64{11}, GroupIDs: []int64{21, 22}, ProtectedBranchIDs: []int64{31},
				Usernames: []string{"bob"}, AppliesToAllProtectedBranches: new(true),
			},
			want: `{"name":"renamed","approvals_required":3,"user_ids":[11],"group_ids":[21,22],` +
				`"protected_branch_ids":[31],"usernames":["bob"],"applies_to_all_protected_branches":true}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := bodyAssertingClient(t, http.MethodPut, pathProject42ApprovalRule10, testCase.want, http.StatusOK, approvalRuleJSON)
			if _, err := UpdateApprovalRule(t.Context(), client, testCase.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
		})
	}
}

// TestGetApprovalConfig_EachKeyIsPublishedUnderItsOwnName holds the approval
// configuration to what GitLab sent, its seven flags one at a time and its
// one count beside them.
func TestGetApprovalConfig_EachKeyIsPublishedUnderItsOwnName(t *testing.T) {
	get := func(client *gitlabclient.Client) (any, error) {
		return GetApprovalConfig(t.Context(), client, GetApprovalConfigInput{ProjectID: "42"})
	}
	assertFlagsPublishedOneAtATime(t, []reflect.Type{reflect.TypeFor[gl.ProjectApprovals]()}, nil, nil, oneKeyJSON, get)
	fixture := distinctScalarFixture(reflect.TypeFor[gl.ProjectApprovals]())
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, mustJSON(t, fixture))
	}))
	out, err := get(client)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertPublishedAsSent(t, fixture, out, nil)
}

// TestGetApprovalRule_EachKeyIsPublishedUnderItsOwnName holds one approval
// rule to what GitLab sent: its two flags one at a time, and its scalars
// together, the coverage threshold read off the capture among them.
func TestGetApprovalRule_EachKeyIsPublishedUnderItsOwnName(t *testing.T) {
	get := func(client *gitlabclient.Client) (any, error) {
		return GetApprovalRule(t.Context(), client, GetApprovalRuleInput{ProjectID: "42", RuleID: 10})
	}
	assertFlagsPublishedOneAtATime(t, []reflect.Type{reflect.TypeFor[gl.ProjectApprovalRule]()}, nil, nil, oneKeyJSON, get)
	fixture := distinctScalarFixture(reflect.TypeFor[gl.ProjectApprovalRule](), reflect.TypeFor[toolutil.ProjectApprovalRuleExtra]())
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, mustJSON(t, fixture))
	}))
	out, err := get(client)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertPublishedAsSent(t, fixture, out, nil)
}

// TestFormatListApprovalRulesMarkdown_NonEmpty verifies the whole rule table,
// with the dash a cell whose value GitLab did not send renders as, and a footer
// that no longer names links the table has none of.
func TestFormatListApprovalRulesMarkdown_NonEmpty(t *testing.T) {
	md := FormatListApprovalRulesMarkdown(ListApprovalRulesOutput{
		Rules: []ApprovalRuleOutput{
			{ID: 10, Name: "Rule A", ApprovalsRequired: 1},
		},
	})
	want := "## Approval Rules (1)\n\n" +
		"| ID | Name | Type | Approvals | All Protected | Users | Groups |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| 10 | Rule A | - | 1 | ❌ | - | - |\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use action 'project.approval_rule_get' to see one rule in full\n" +
		"- Use action 'project.approval_rule_create' to add a new rule\n"
	if md != want {
		t.Errorf("FormatListApprovalRulesMarkdown()\n got: %q\nwant: %q", md, want)
	}
}
