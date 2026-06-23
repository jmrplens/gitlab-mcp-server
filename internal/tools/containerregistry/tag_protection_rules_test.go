// tag_protection_rules_test.go contains unit tests for the container registry
// tag protection rule MCP tool handlers. Tests use httptest to mock GitLab API
// responses and verify success, error, and edge-case paths.
package containerregistry

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const tagRulesPath = "/api/v4/projects/10/registry/protection/tag/rules"

// ---------------------------------------------------------------------------
// ListTagProtectionRules
// ---------------------------------------------------------------------------.

// TestListTagProtectionRules_Success verifies that ListTagProtectionRules succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListTagProtectionRules_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"project_id":10,"tag_name_pattern":"v.+","minimum_access_level_for_push":"maintainer","minimum_access_level_for_delete":"admin"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListTagProtectionRules(context.Background(), client, ListTagProtectionRulesInput{ProjectID: toolutil.StringOrInt("10")})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out.Rules))
	}
	if out.Rules[0].TagNamePattern != "v.+" {
		t.Errorf("expected pattern v.+, got %s", out.Rules[0].TagNamePattern)
	}
	if out.Rules[0].MinimumAccessLevelForPush != "maintainer" {
		t.Errorf("expected push level maintainer, got %s", out.Rules[0].MinimumAccessLevelForPush)
	}
}

// TestListTagProtectionRules_MissingProjectID verifies that ListTagProtectionRules returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestListTagProtectionRules_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListTagProtectionRules(context.Background(), client, ListTagProtectionRulesInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// CreateTagProtectionRule
// ---------------------------------------------------------------------------.

// TestCreateTagProtectionRule_Success verifies that CreateTagProtectionRule succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateTagProtectionRule_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":5,"project_id":10,"tag_name_pattern":"release-.+","minimum_access_level_for_push":"owner","minimum_access_level_for_delete":"admin"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateTagProtectionRule(context.Background(), client, CreateTagProtectionRuleInput{
		ProjectID:                   toolutil.StringOrInt("10"),
		TagNamePattern:              "release-.+",
		MinimumAccessLevelForPush:   "owner",
		MinimumAccessLevelForDelete: "admin",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("expected rule ID 5, got %d", out.ID)
	}
	if out.TagNamePattern != "release-.+" {
		t.Errorf("expected pattern release-.+, got %s", out.TagNamePattern)
	}
}

// TestCreateTagProtectionRule_Immutable verifies that omitting both access levels creates an immutable rule.
// The test exercises the POST path with no minimum access levels set.
// It asserts the empty access levels round-trip without error.
func TestCreateTagProtectionRule_Immutable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":6,"project_id":10,"tag_name_pattern":"prod-.+","minimum_access_level_for_push":"","minimum_access_level_for_delete":""}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateTagProtectionRule(context.Background(), client, CreateTagProtectionRuleInput{
		ProjectID:      toolutil.StringOrInt("10"),
		TagNamePattern: "prod-.+",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MinimumAccessLevelForPush != "" {
		t.Errorf("expected empty push level for immutable rule, got %s", out.MinimumAccessLevelForPush)
	}
}

// TestCreateTagProtectionRule_MissingProjectID verifies that CreateTagProtectionRule returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestCreateTagProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateTagProtectionRule(context.Background(), client, CreateTagProtectionRuleInput{TagNamePattern: "v.+"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestCreateTagProtectionRule_MissingPattern verifies that CreateTagProtectionRule returns a validation error when tag_name_pattern is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing tag_name_pattern field.
func TestCreateTagProtectionRule_MissingPattern(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateTagProtectionRule(context.Background(), client, CreateTagProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "tag_name_pattern is required") {
		t.Fatalf("expected tag_name_pattern required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateTagProtectionRule
// ---------------------------------------------------------------------------.

// TestUpdateTagProtectionRule_Success verifies that UpdateTagProtectionRule succeeds when the GitLab API returns a valid response.
// The test exercises the PATCH path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateTagProtectionRule_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath+"/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"project_id":10,"tag_name_pattern":"stable-.+","minimum_access_level_for_push":"maintainer","minimum_access_level_for_delete":"owner"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateTagProtectionRule(context.Background(), client, UpdateTagProtectionRuleInput{
		ProjectID:      toolutil.StringOrInt("10"),
		RuleID:         5,
		TagNamePattern: "stable-.+",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TagNamePattern != "stable-.+" {
		t.Errorf("expected pattern stable-.+, got %s", out.TagNamePattern)
	}
}

// TestUpdateTagProtectionRule_MissingProjectID verifies that UpdateTagProtectionRule returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestUpdateTagProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateTagProtectionRule(context.Background(), client, UpdateTagProtectionRuleInput{RuleID: 5})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestUpdateTagProtectionRule_MissingRuleID verifies that UpdateTagProtectionRule returns a validation error when rule_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing rule_id field.
func TestUpdateTagProtectionRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateTagProtectionRule(context.Background(), client, UpdateTagProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "rule_id is required") {
		t.Fatalf("expected rule_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteTagProtectionRule
// ---------------------------------------------------------------------------.

// TestDeleteTagProtectionRule_Success verifies that DeleteTagProtectionRule succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that no error is returned for a 204 response.
func TestDeleteTagProtectionRule_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath+"/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteTagProtectionRule(context.Background(), client, DeleteTagProtectionRuleInput{
		ProjectID: toolutil.StringOrInt("10"),
		RuleID:    5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteTagProtectionRule_MissingProjectID verifies that DeleteTagProtectionRule returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestDeleteTagProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTagProtectionRule(context.Background(), client, DeleteTagProtectionRuleInput{RuleID: 5})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteTagProtectionRule_MissingRuleID verifies that DeleteTagProtectionRule returns a validation error when rule_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing rule_id field.
func TestDeleteTagProtectionRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTagProtectionRule(context.Background(), client, DeleteTagProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "rule_id is required") {
		t.Fatalf("expected rule_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestFormatTagProtectionRuleMarkdown verifies the TagProtectionRuleMarkdown formatter for a representative input.
// The test exercises rendering of a single tag protection rule.
// It asserts the rendered Markdown contains the tag pattern.
func TestFormatTagProtectionRuleMarkdown(t *testing.T) {
	out := TagProtectionRuleOutput{
		ID: 1, ProjectID: 10,
		TagNamePattern:              "v.+",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	}
	md := FormatTagProtectionRuleMarkdown(out)
	if !strings.Contains(md, "v.+") {
		t.Errorf("expected pattern in markdown, got: %s", md)
	}
}

// TestFormatTagProtectionRuleMarkdown_Immutable verifies that an empty access level renders as "immutable".
// The test exercises the protectionAccessLabel helper through the formatter.
// It asserts the rendered Markdown surfaces the immutable label.
func TestFormatTagProtectionRuleMarkdown_Immutable(t *testing.T) {
	out := TagProtectionRuleOutput{ID: 1, ProjectID: 10, TagNamePattern: "prod-.+"}
	md := FormatTagProtectionRuleMarkdown(out)
	if !strings.Contains(md, "immutable") {
		t.Errorf("expected immutable label in markdown, got: %s", md)
	}
}

// TestFormatTagProtectionRuleListMarkdown verifies the TagProtectionRuleListMarkdown formatter for a representative input.
// The test exercises rendering of a populated rule list.
// It asserts the rendered Markdown contains the tag pattern.
func TestFormatTagProtectionRuleListMarkdown(t *testing.T) {
	out := TagProtectionRuleListOutput{
		Rules: []TagProtectionRuleOutput{
			{ID: 1, TagNamePattern: "v.+", MinimumAccessLevelForPush: "maintainer", MinimumAccessLevelForDelete: "admin"},
		},
	}
	md := FormatTagProtectionRuleListMarkdown(out)
	if !strings.Contains(md, "v.+") {
		t.Errorf("expected pattern in markdown, got: %s", md)
	}
}

// TestFormatTagProtectionRuleListMarkdown_Empty verifies that the list formatter handles the empty state.
// The test exercises rendering of an empty rule list.
// It asserts the rendered Markdown shows the empty-state message.
func TestFormatTagProtectionRuleListMarkdown_Empty(t *testing.T) {
	md := FormatTagProtectionRuleListMarkdown(TagProtectionRuleListOutput{})
	if !strings.Contains(md, "No tag protection rules found") {
		t.Errorf("expected empty-state message, got: %s", md)
	}
}

// ---------------------------------------------------------------------------
// API error paths + access-level branches + Output wrapper
// ---------------------------------------------------------------------------.

// TestListTagProtectionRules_APIError verifies the error path wraps the upstream failure.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestListTagProtectionRules_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListTagProtectionRules(context.Background(), client, ListTagProtectionRulesInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestCreateTagProtectionRule_APIError verifies the error path wraps the upstream failure.
// The test makes the mock return 400.
// It asserts a non-nil error is returned.
func TestCreateTagProtectionRule_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"tag_name_pattern is invalid"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := CreateTagProtectionRule(context.Background(), client, CreateTagProtectionRuleInput{
		ProjectID: toolutil.StringOrInt("10"), TagNamePattern: "[",
	})
	if err == nil {
		t.Fatal("expected error from 400 response")
	}
}

// TestUpdateTagProtectionRule_AccessLevels verifies the push/delete access-level branches are applied.
// The test sets both minimum access levels and inspects the request body.
// It asserts both fields reach the API and the call succeeds.
func TestUpdateTagProtectionRule_AccessLevels(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath+"/5", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"project_id":10,"tag_name_pattern":"v.+","minimum_access_level_for_push":"owner","minimum_access_level_for_delete":"admin"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateTagProtectionRule(context.Background(), client, UpdateTagProtectionRuleInput{
		ProjectID:                   toolutil.StringOrInt("10"),
		RuleID:                      5,
		MinimumAccessLevelForPush:   "owner",
		MinimumAccessLevelForDelete: "admin",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, field := range []string{"minimum_access_level_for_push", "minimum_access_level_for_delete"} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in request body, got: %s", field, body)
		}
	}
	if out.MinimumAccessLevelForPush != "owner" {
		t.Errorf("expected push level owner, got %s", out.MinimumAccessLevelForPush)
	}
}

// TestUpdateTagProtectionRule_APIError verifies the error path wraps the upstream failure.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestUpdateTagProtectionRule_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath+"/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Rule Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := UpdateTagProtectionRule(context.Background(), client, UpdateTagProtectionRuleInput{
		ProjectID: toolutil.StringOrInt("10"), RuleID: 5, TagNamePattern: "v.+",
	})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestDeleteTagProtectionRule_APIError verifies the error path wraps the upstream failure.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestDeleteTagProtectionRule_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath+"/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Rule Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := DeleteTagProtectionRule(context.Background(), client, DeleteTagProtectionRuleInput{
		ProjectID: toolutil.StringOrInt("10"), RuleID: 5,
	})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestDeleteTagProtectionRuleOutput verifies the catalog wrapper returns the canonical success shape.
// The test deletes a rule via the *Output helper used by the destructive action route.
// It asserts a success status on the happy path and a propagated error otherwise.
func TestDeleteTagProtectionRuleOutput(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(tagRulesPath+"/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, testMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := DeleteTagProtectionRuleOutput(context.Background(), client, DeleteTagProtectionRuleInput{
		ProjectID: toolutil.StringOrInt("10"), RuleID: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("expected success status, got %q", out.Status)
	}

	// Error propagation: missing rule_id short-circuits before any request.
	if _, errMissing := DeleteTagProtectionRuleOutput(context.Background(), client, DeleteTagProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")}); errMissing == nil {
		t.Fatal("expected error when rule_id is missing")
	}
}
