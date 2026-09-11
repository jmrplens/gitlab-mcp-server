// protected_packages_test.go contains unit tests for GitLab protected package
// operations. Tests use httptest to mock the GitLab Protected Packages API.
package protectedpackages

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	testProjectID = "myproject"
	pathRules     = "/api/v4/projects/myproject/packages/protection/rules"
	pathRule1     = "/api/v4/projects/myproject/packages/protection/rules/1"

	ruleJSON = `{
		"id": 1,
		"project_id": 42,
		"package_name_pattern": "@scope/pkg*",
		"package_type": "npm",
		"minimum_access_level_for_push": "maintainer",
		"minimum_access_level_for_delete": "owner"
	}`
)

// List tests.

// TestList_Success verifies List returns one rule when
// GET /projects/:id/packages/protection/rules responds 200 with a single rule.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRules {
			testutil.RespondJSON(w, http.StatusOK, "["+ruleJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(out.Rules))
	}
	if out.Rules[0].ID != 1 {
		t.Errorf("ID = %d, want 1", out.Rules[0].ID)
	}
	if out.Rules[0].ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", out.Rules[0].ProjectID)
	}
	if out.Rules[0].PackageNamePattern != "@scope/pkg*" {
		t.Errorf("PackageNamePattern = %q", out.Rules[0].PackageNamePattern)
	}
	if out.Rules[0].PackageType != "npm" {
		t.Errorf("PackageType = %q", out.Rules[0].PackageType)
	}
	if out.Rules[0].MinimumAccessLevelForPush != "maintainer" {
		t.Errorf("MinPush = %q", out.Rules[0].MinimumAccessLevelForPush)
	}
	if out.Rules[0].MinimumAccessLevelForDelete != "owner" {
		t.Errorf("MinDelete = %q", out.Rules[0].MinimumAccessLevelForDelete)
	}
}

// TestList_MissingProjectID verifies List returns a validation error when
// project_id is empty, without hitting the API.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestList_CancelledContext verifies List returns a context error when invoked
// with an already-cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestList_APIError verifies List propagates an error when the protection
// rules endpoint responds 403 Forbidden.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// TestList_Pagination verifies List forwards the page query parameter to the
// GitLab API when pagination input is supplied.
func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("page = %q, want 2", r.URL.Query().Get("page"))
		}
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		Page:      2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// TestList_KeysetAndOrdering verifies List forwards keyset pagination
// (pagination, page_token), order_by, and sort query parameters to the
// GitLab API.
func TestList_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "id" {
			t.Errorf("order_by = %q, want id", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort = %q, want desc", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
		}
		if q.Get("page_token") != "tok42" {
			t.Errorf("page_token = %q, want tok42", q.Get("page_token"))
		}
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:  testProjectID,
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok42",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// Create tests.

// TestCreate_Success verifies Create returns the new rule when
// POST /projects/:id/packages/protection/rules responds 201 Created.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRules {
			testutil.RespondJSON(w, http.StatusCreated, ruleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:                   testProjectID,
		PackageNamePattern:          "@scope/pkg*",
		PackageType:                 "npm",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "owner",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.MinimumAccessLevelForPush != "maintainer" {
		t.Errorf("MinPush = %q, want maintainer", out.MinimumAccessLevelForPush)
	}
}

// TestCreate_MissingProjectID verifies Create returns a validation error when
// project_id is empty.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		PackageNamePattern: "@scope/pkg*",
		PackageType:        "npm",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCreate_MissingPattern verifies Create returns a validation error when
// package_name_pattern is empty.
func TestCreate_MissingPattern(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:   testProjectID,
		PackageType: "npm",
	})
	if err == nil {
		t.Fatal("expected error for missing package_name_pattern")
	}
}

// TestCreate_MissingPackageType verifies Create returns a validation error
// when package_type is empty.
func TestCreate_MissingPackageType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:          testProjectID,
		PackageNamePattern: "@scope/pkg*",
	})
	if err == nil {
		t.Fatal("expected error for missing package_type")
	}
}

// TestCreate_CancelledContext verifies Create returns a context error when
// invoked with an already-cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{
		ProjectID:          testProjectID,
		PackageNamePattern: "@scope/pkg*",
		PackageType:        "npm",
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestCreate_WithoutAccessLevels verifies Create succeeds when optional
// minimum_access_level_for_push/delete fields are omitted, leaving those
// fields empty in the output.
func TestCreate_WithoutAccessLevels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRules {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 2,
				"project_id": 42,
				"package_name_pattern": "mylib*",
				"package_type": "pypi"
			}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:          testProjectID,
		PackageNamePattern: "mylib*",
		PackageType:        "pypi",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.MinimumAccessLevelForPush != "" {
		t.Errorf("MinPush = %q, want empty", out.MinimumAccessLevelForPush)
	}
}

// Update tests.

// TestUpdate_Success verifies Update returns the updated rule when
// PATCH /projects/:id/packages/protection/rules/:rule_id responds 200 OK.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathRule1 {
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:                   testProjectID,
		RuleID:                      1,
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "admin",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// TestUpdate_MissingProjectID verifies Update returns a validation error when
// project_id is empty.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{RuleID: 1})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestUpdate_MissingRuleID verifies Update returns a validation error when
// rule_id is zero.
func TestUpdate_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing rule_id")
	}
}

// TestUpdate_CancelledContext verifies Update returns a context error when
// invoked with an already-cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: testProjectID, RuleID: 1})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestUpdate_PartialFields verifies Update succeeds when only a subset of
// optional fields (package_name_pattern, package_type) are provided.
func TestUpdate_PartialFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathRule1 {
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:          testProjectID,
		RuleID:             1,
		PackageNamePattern: "@scope/new-pkg*",
		PackageType:        "maven",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// Delete tests.

// TestDelete_Success verifies Delete returns no error when
// DELETE /projects/:id/packages/protection/rules/:rule_id responds 204.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathRule1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, RuleID: 1})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

// TestDelete_MissingProjectID verifies Delete returns a validation error when
// project_id is empty.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{RuleID: 1})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDelete_MissingRuleID verifies Delete returns a validation error when
// rule_id is zero.
func TestDelete_MissingRuleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("expected error for missing rule_id")
	}
}

// TestDelete_CancelledContext verifies Delete returns a context error when
// invoked with an already-cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, RuleID: 1})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// Markdown tests.

// ruleCardHints is the guidance section a protection rule card closes with.
const ruleCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use `gitlab_update_package_protection_rule` to modify this rule\n" +
	"- Use `gitlab_delete_package_protection_rule` to remove it\n"

// TestFormatOutputMarkdown_Basic verifies FormatOutputMarkdown renders a
// fully populated rule as the whole card: the heading, the pattern as a code
// span, the type and both access levels.
func TestFormatOutputMarkdown_Basic(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                          1,
		PackageNamePattern:          "@scope/pkg*",
		PackageType:                 "npm",
		MinimumAccessLevelForPush:   "maintainer",
		MinimumAccessLevelForDelete: "owner",
	})
	want := "## Package Protection Rule #1\n\n" +
		"- **Pattern**: `@scope/pkg*`\n" +
		"- **Package Type**: npm\n" +
		"- **Min Push Level**: maintainer\n" +
		"- **Min Delete Level**: owner\n" +
		ruleCardHints
	if got != want {
		t.Errorf("FormatOutputMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatOutputMarkdown_Empty verifies FormatOutputMarkdown returns an
// empty string for a zero-value Output (ID == 0).
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string, got %q", md)
	}
}

// TestFormatOutputMarkdown_NoAccessLevels verifies FormatOutputMarkdown omits
// the push/delete level rows when those fields are empty: the card shows what
// GitLab sent and never a label with nothing after it.
func TestFormatOutputMarkdown_NoAccessLevels(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                 2,
		PackageNamePattern: "mylib*",
		PackageType:        "pypi",
	})
	want := "## Package Protection Rule #2\n\n" +
		"- **Pattern**: `mylib*`\n" +
		"- **Package Type**: pypi\n" +
		ruleCardHints
	if got != want {
		t.Errorf("FormatOutputMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdown_Empty verifies an empty list is the one sentence and
// nothing else: no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})
	want := "No package protection rules found.\n"
	if got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatListMarkdown_WithRules verifies FormatListMarkdown produces the
// whole table: one row per rule, the pattern as a code span, and the guidance
// after the rows rather than before them.
func TestFormatListMarkdown_WithRules(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Rules: []Output{
			{ID: 1, PackageNamePattern: "@scope/pkg*", PackageType: "npm", MinimumAccessLevelForPush: "maintainer"},
			{ID: 2, PackageNamePattern: "mylib*", PackageType: "pypi"},
		},
	})
	want := "## Package Protection Rules (2)\n\n" +
		"| ID | Pattern | Type | Min Push | Min Delete |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | `@scope/pkg*` | npm | maintainer |  |\n" +
		"| 2 | `mylib*` | pypi |  |  |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use `gitlab_create_package_protection_rule` to add a new rule\n"
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdown_CountsTheTotalGitLabSent verifies the heading counts
// what the response reports rather than the page length, which is what a
// reader compares against the rows.
func TestFormatListMarkdown_CountsTheTotalGitLabSent(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Rules:      []Output{{ID: 1, PackageNamePattern: "a*", PackageType: "npm"}},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 1, TotalItems: 45, TotalPages: 45, HasMore: true, NextPage: 2},
	})
	want := "## Package Protection Rules (45)\n\n" +
		"Showing 1 of 45 results (page 1 of 45)\n\n" +
		"| ID | Pattern | Type | Min Push | Min Delete |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | `a*` | npm |  |  |\n" +
		"\nPage 1 of 45 | 45 items total | 1 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use `gitlab_create_package_protection_rule` to add a new rule\n"
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestCreate_APIError covers the API error path in Create.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", PackageNamePattern: "pkg-*", PackageType: "npm"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestUpdate_APIError covers the API error path in Update.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", RuleID: 1})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestDelete_APIError covers the API error path in Delete.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", RuleID: 1})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}
