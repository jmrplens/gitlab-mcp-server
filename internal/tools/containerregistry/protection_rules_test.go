// protection_rules_test.go contains unit tests for the container registry MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package containerregistry

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestListProtectionRules_MissingProjectID verifies that ListProtectionRules returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestListProtectionRules_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestCreateProtectionRule_MissingProjectID verifies that CreateProtectionRule returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestCreateProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateProtectionRule(context.Background(), client, CreateProtectionRuleInput{RepositoryPathPattern: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestCreateProtectionRule_MissingPattern verifies that CreateProtectionRule returns a validation error when repository_path_pattern is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing repository_path_pattern field.
func TestCreateProtectionRule_MissingPattern(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestUpdateProtectionRule_MissingProjectID verifies that UpdateProtectionRule returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestUpdateProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := UpdateProtectionRule(context.Background(), client, UpdateProtectionRuleInput{RuleID: 5})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestUpdateProtectionRule_MissingRuleID verifies that UpdateProtectionRule returns a validation error when rule_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing rule_id field.
func TestUpdateProtectionRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestDeleteProtectionRule_MissingProjectID verifies that DeleteProtectionRule returns a validation error when project_id is empty.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing project_id field.
func TestDeleteProtectionRule_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteProtectionRule(context.Background(), client, DeleteProtectionRuleInput{RuleID: 5})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpectedProjectIDErr, err)
	}
}

// TestDeleteProtectionRule_MissingRuleID verifies that DeleteProtectionRule returns a validation error when rule_id is zero.
// The test exercises the input validation guard before any API call.
// It asserts that the returned error names the missing rule_id field.
func TestDeleteProtectionRule_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteProtectionRule(context.Background(), client, DeleteProtectionRuleInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "rule_id is required") {
		t.Fatalf("expected rule_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// protectionRuleCardHints is the guidance a repository-path protection rule
// card closes with.
const protectionRuleCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'registry_rule_update' to modify access levels\n" +
	"- Use action 'registry_rule_delete' to remove this rule\n"

// TestFormatProtectionRuleMarkdown verifies the whole card of a
// repository-path protection rule.
func TestFormatProtectionRuleMarkdown(t *testing.T) {
	out := ProtectionRuleOutput{
		ID: 1, ProjectID: 10,
		RepositoryPathPattern:       testProdPattern,
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	}
	got := FormatProtectionRuleMarkdown(out)
	want := "## Protection Rule: " + testProdPattern + "\n\n" +
		"- **ID**: 1\n" +
		"- **Repository Path Pattern**: `" + testProdPattern + "`\n" +
		"- **Min Access Level (Push)**: maintainer\n" +
		"- **Min Access Level (Delete)**: admin\n" +
		protectionRuleCardHints
	if got != want {
		t.Errorf("FormatProtectionRuleMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// What each request carries, one input at a time
// ---------------------------------------------------------------------------.

// TestCreateProtectionRule_OneAccessLevelAtATime_SendsThatLevelAndNothingElse
// drives one creation per optional access level and compares the whole request
// body. One at a time is what distinguishes them: the two guards can be
// inverted independently, which sends GitLab the level the caller left out and
// drops the one they set, and no test that sets both can tell. The pattern is
// required, so it rides along in every case.
func TestCreateProtectionRule_OneAccessLevelAtATime_SendsThatLevelAndNothingElse(t *testing.T) {
	tests := []struct {
		name  string
		input CreateProtectionRuleInput
		want  map[string]any
	}{
		{
			"pattern only",
			CreateProtectionRuleInput{ProjectID: "10", RepositoryPathPattern: testProdPattern},
			map[string]any{"repository_path_pattern": testProdPattern},
		},
		{
			"push",
			CreateProtectionRuleInput{ProjectID: "10", RepositoryPathPattern: testProdPattern, MinimumAccessLevelForPush: "owner"},
			map[string]any{"repository_path_pattern": testProdPattern, "minimum_access_level_for_push": "owner"},
		},
		{
			"delete",
			CreateProtectionRuleInput{ProjectID: "10", RepositoryPathPattern: testProdPattern, MinimumAccessLevelForDelete: "admin"},
			map[string]any{"repository_path_pattern": testProdPattern, "minimum_access_level_for_delete": "admin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules", func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				got = registryRequestBody(t, r)
				testutil.RespondJSON(w, http.StatusCreated, covRuleJSON)
			})
			client := testutil.NewTestClient(t, mux)

			if _, err := CreateProtectionRule(context.Background(), client, tt.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("request body = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestUpdateProtectionRule_OneFieldAtATime_SendsThatFieldAndNothingElse drives
// one update per optional field and compares the whole request body. The empty
// case is the other half: a rule whose pattern was never mentioned must not
// have it rewritten to the empty string.
func TestUpdateProtectionRule_OneFieldAtATime_SendsThatFieldAndNothingElse(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateProtectionRuleInput
		want  map[string]any
	}{
		{"nothing", UpdateProtectionRuleInput{ProjectID: "10", RuleID: 5}, map[string]any{}},
		{
			"repository_path_pattern",
			UpdateProtectionRuleInput{ProjectID: "10", RuleID: 5, RepositoryPathPattern: testStagingPattern},
			map[string]any{"repository_path_pattern": testStagingPattern},
		},
		{
			"minimum_access_level_for_push",
			UpdateProtectionRuleInput{ProjectID: "10", RuleID: 5, MinimumAccessLevelForPush: "owner"},
			map[string]any{"minimum_access_level_for_push": "owner"},
		},
		{
			"minimum_access_level_for_delete",
			UpdateProtectionRuleInput{ProjectID: "10", RuleID: 5, MinimumAccessLevelForDelete: "admin"},
			map[string]any{"minimum_access_level_for_delete": "admin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules/5", func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPatch)
				got = registryRequestBody(t, r)
				testutil.RespondJSON(w, http.StatusOK, covRuleJSON)
			})
			client := testutil.NewTestClient(t, mux)

			if _, err := UpdateProtectionRule(context.Background(), client, tt.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("request body = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestListProtectionRules_PublishesEveryFieldGitLabSent verifies the whole
// output a caller is handed, so a converter that filled one field from its
// neighbor is a mismatch: the two access levels carry different values here
// for exactly that reason.
func TestListProtectionRules_PublishesEveryFieldGitLabSent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/registry/protection/repository/rules", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covRuleJSON+`]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProtectionRules(context.Background(), client, ListProtectionRulesInput{ProjectID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := []ProtectionRuleOutput{{
		ID: 77, ProjectID: 42,
		RepositoryPathPattern:       testProdPattern,
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	}}
	if !reflect.DeepEqual(out.Rules, want) {
		t.Errorf("Rules =\n%#v\nwant:\n%#v", out.Rules, want)
	}
	if out.Pagination.TotalItems != 1 || out.Pagination.PerPage != 20 {
		t.Errorf("Pagination = %+v, want the page GitLab reported", out.Pagination)
	}
}

// TestFormatProtectionRuleMarkdown_NoPattern_HeadsWithTheResourceAlone
// verifies a rule GitLab sent no path pattern for is headed by the resource
// alone, rather than by a heading ending in a colon with nothing behind it.
func TestFormatProtectionRuleMarkdown_NoPattern_HeadsWithTheResourceAlone(t *testing.T) {
	got := FormatProtectionRuleMarkdown(ProtectionRuleOutput{ID: 3, ProjectID: 10, MinimumAccessLevelForPush: "owner"})
	want := "## Protection Rule\n\n" +
		"- **ID**: 3\n" +
		"- **Min Access Level (Push)**: owner\n" +
		protectionRuleCardHints
	if got != want {
		t.Errorf("FormatProtectionRuleMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatProtectionRuleListMarkdown verifies the whole table for a
// single-rule list.
func TestFormatProtectionRuleListMarkdown(t *testing.T) {
	out := ProtectionRuleListOutput{
		Rules: []ProtectionRuleOutput{
			{ID: 1, RepositoryPathPattern: testProdPattern, MinimumAccessLevelForPush: "maintainer", MinimumAccessLevelForDelete: "admin"},
		},
	}
	got := FormatProtectionRuleListMarkdown(out)
	want := "## Protection Rules (1)\n\n" +
		"| ID | Pattern | Min Push | Min Delete |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | `" + testProdPattern + "` | maintainer | admin |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'registry_rule_create' to add a new rule\n"
	if got != want {
		t.Errorf("FormatProtectionRuleListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}
