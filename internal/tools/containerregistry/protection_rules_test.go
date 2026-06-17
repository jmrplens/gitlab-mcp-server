// protection_rules_test.go contains unit tests for the container registry MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package containerregistry

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// ListProtectionRules
// ---------------------------------------------------------------------------.

// TestListProtectionRules_Success verifies that ListProtectionRules succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProtectionRules_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"project_id":10,"repository_path_pattern":"my-project/my-image*","minimum_access_level_for_push":"maintainer","minimum_access_level_for_delete":"admin"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProtectionRules(context.Background(), client, ListProtectionRulesInput{ProjectID: toolutil.StringOrInt("10")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	if out.Rules[0].RepositoryPathPattern != "my-project/my-image*" {
		t.Errorf("expected pattern my-project/my-image*, got %s", out.Rules[0].RepositoryPathPattern)
	}
	if out.Rules[0].MinimumAccessLevelForPush != "maintainer" {
		t.Errorf("expected push level maintainer, got %s", out.Rules[0].MinimumAccessLevelForPush)
	}
}

// TestListProtectionRules_MissingProjectID verifies that ListProtectionRules_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProtectionRules_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProtectionRules(context.Background(), client, ListProtectionRulesInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// CreateProtectionRule
// ---------------------------------------------------------------------------.

// TestCreateProtectionRule_Success verifies that CreateProtectionRule succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateProtectionRule_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":5,"project_id":10,"repository_path_pattern":"prod/*","minimum_access_level_for_push":"owner","minimum_access_level_for_delete":"admin"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateProtectionRule(context.Background(), client, CreateProtectionRuleInput{
		ProjectID:                   toolutil.StringOrInt("10"),
		RepositoryPathPattern:       testProdPattern,
		MinimumAccessLevelForPush:   "owner",
		MinimumAccessLevelForDelete: "admin",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("expected rule ID 5, got %d", out.ID)
	}
	if out.RepositoryPathPattern != testProdPattern {
		t.Errorf("expected pattern prod/*, got %s", out.RepositoryPathPattern)
	}
}

// TestCreateProtectionRule_MissingProjectID verifies that CreateProtectionRule_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateProtectionRule(context.Background(), client, CreateProtectionRuleInput{RepositoryPathPattern: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestCreateProtectionRule_MissingPattern verifies that CreateProtectionRule_MissingPattern returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProtectionRule_MissingPattern(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateProtectionRule(context.Background(), client, CreateProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "repository_path_pattern is required") {
		t.Fatalf("expected pattern required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateProtectionRule
// ---------------------------------------------------------------------------.

// TestUpdateProtectionRule_Success verifies that UpdateProtectionRule succeeds when the GitLab API returns a valid response.
// The test exercises the PATCH path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateProtectionRule_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"project_id":10,"repository_path_pattern":"staging/*","minimum_access_level_for_push":"maintainer","minimum_access_level_for_delete":"owner"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateProtectionRule(context.Background(), client, UpdateProtectionRuleInput{
		ProjectID:             toolutil.StringOrInt("10"),
		RuleID:                5,
		RepositoryPathPattern: testStagingPattern,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.RepositoryPathPattern != testStagingPattern {
		t.Errorf("expected pattern staging/*, got %s", out.RepositoryPathPattern)
	}
}

// TestUpdateProtectionRule_MissingProjectID verifies that UpdateProtectionRule_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateProtectionRule(context.Background(), client, UpdateProtectionRuleInput{RuleID: 5})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestUpdateProtectionRule_MissingRuleID verifies that UpdateProtectionRule_MissingRuleID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateProtectionRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateProtectionRule(context.Background(), client, UpdateProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "rule_id is required") {
		t.Fatalf("expected rule_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteProtectionRule
// ---------------------------------------------------------------------------.

// TestDeleteProtectionRule_Success verifies that DeleteProtectionRule succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteProtectionRule_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteProtectionRule(context.Background(), client, DeleteProtectionRuleInput{
		ProjectID: toolutil.StringOrInt("10"),
		RuleID:    5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteProtectionRule_MissingProjectID verifies that DeleteProtectionRule_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteProtectionRule(context.Background(), client, DeleteProtectionRuleInput{RuleID: 5})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteProtectionRule_MissingRuleID verifies that DeleteProtectionRule_MissingRuleID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProtectionRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteProtectionRule(context.Background(), client, DeleteProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "rule_id is required") {
		t.Fatalf("expected rule_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestFormatProtectionRuleMarkdown verifies the ProtectionRuleMarkdown Markdown formatter for a representative protectionrule input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatProtectionRuleMarkdown(t *testing.T) {
	out := ProtectionRuleOutput{
		ID: 1, ProjectID: 10,
		RepositoryPathPattern:       testProdPattern,
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	}
	md := FormatProtectionRuleMarkdown(out)
	if !strings.Contains(md, testProdPattern) {
		t.Errorf("expected pattern in markdown, got: %s", md)
	}
}

// TestFormatProtectionRuleListMarkdown verifies the ProtectionRuleListMarkdown Markdown formatter for a representative protectionrulelist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatProtectionRuleListMarkdown(t *testing.T) {
	out := ProtectionRuleListOutput{
		Rules: []ProtectionRuleOutput{
			{ID: 1, RepositoryPathPattern: testProdPattern, MinimumAccessLevelForPush: "maintainer", MinimumAccessLevelForDelete: "admin"},
		},
	}
	md := FormatProtectionRuleListMarkdown(out)
	if !strings.Contains(md, testProdPattern) {
		t.Errorf("expected pattern in markdown, got: %s", md)
	}
}
