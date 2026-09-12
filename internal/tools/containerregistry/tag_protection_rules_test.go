// tag_protection_rules_test.go contains unit tests for the container registry
// tag protection rule MCP tool handlers. Tests use httptest to mock GitLab API
// responses and verify success, error, and edge-case paths.
package containerregistry

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// tagRuleCardHints is the guidance a tag protection rule card closes with,
// and tagRuleListHints the guidance the list closes with.
const (
	tagRuleCardHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'registry_tag_rule_update' to modify access levels\n" +
		"- Use action 'registry_tag_rule_delete' to remove this rule\n"
	tagRuleListHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'registry_tag_rule_create' to add a new rule\n" +
		"- These rules protect image *tags*; use action 'registry_rule_list' for repository-path protection rules\n"
)

// TestFormatTagProtectionRuleMarkdown verifies the whole card of a tag
// protection rule. The pattern is an RE2 expression a person types, so it is
// a code span rather than an escaped cell: '|' is ordinary alternation there.
func TestFormatTagProtectionRuleMarkdown(t *testing.T) {
	out := TagProtectionRuleOutput{
		ID: 1, ProjectID: 10,
		TagNamePattern:              "v.+",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	}
	got := FormatTagProtectionRuleMarkdown(out)
	want := "## Tag Protection Rule: v.+\n\n" +
		"- **ID**: 1\n" +
		"- **Tag Name Pattern**: `v.+`\n" +
		"- **Min Access Level (Push)**: maintainer\n" +
		"- **Min Access Level (Delete)**: admin\n" +
		tagRuleCardHints
	if got != want {
		t.Errorf("FormatTagProtectionRuleMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatTagProtectionRuleMarkdown_Immutable verifies that an empty access
// level renders as "immutable", which is how the API expresses a rule that
// forbids push and delete for everyone.
func TestFormatTagProtectionRuleMarkdown_Immutable(t *testing.T) {
	got := FormatTagProtectionRuleMarkdown(TagProtectionRuleOutput{ID: 1, ProjectID: 10, TagNamePattern: "prod-.+"})
	want := "## Tag Protection Rule: prod-.+\n\n" +
		"- **ID**: 1\n" +
		"- **Tag Name Pattern**: `prod-.+`\n" +
		"- **Min Access Level (Push)**: immutable\n" +
		"- **Min Access Level (Delete)**: immutable\n" +
		tagRuleCardHints
	if got != want {
		t.Errorf("FormatTagProtectionRuleMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatTagProtectionRuleListMarkdown verifies the whole table for a
// populated rule list.
func TestFormatTagProtectionRuleListMarkdown(t *testing.T) {
	out := TagProtectionRuleListOutput{
		Rules: []TagProtectionRuleOutput{
			{ID: 1, TagNamePattern: "v.+", MinimumAccessLevelForPush: "maintainer", MinimumAccessLevelForDelete: "admin"},
		},
	}
	got := FormatTagProtectionRuleListMarkdown(out)
	want := "## Tag Protection Rules (1)\n\n" +
		"| ID | Tag Pattern | Min Push | Min Delete |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | `v.+` | maintainer | admin |\n" +
		tagRuleListHints
	if got != want {
		t.Errorf("FormatTagProtectionRuleListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatTagProtectionRuleListMarkdown_Empty verifies an empty list is the
// one sentence and nothing else.
func TestFormatTagProtectionRuleListMarkdown_Empty(t *testing.T) {
	got := FormatTagProtectionRuleListMarkdown(TagProtectionRuleListOutput{})
	want := "No tag protection rules found.\n"
	if got != want {
		t.Errorf("FormatTagProtectionRuleListMarkdown() = %q, want %q", got, want)
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
		t.Run(field, func(t *testing.T) {
			if !strings.Contains(body, field) {
				t.Errorf("expected %s in request body, got: %s", field, body)
			}
		})
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
