// protected_envs_test.go contains unit tests for the protected environment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package protectedenvs

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// pathProtectedEnvs identifies the path protected envs constant used by this package.
	pathProtectedEnvs = "/api/v4/projects/42/protected_environments"
	// pathProtectedEnv1 identifies the path protected env 1 constant used by this package.
	pathProtectedEnv1 = "/api/v4/projects/42/protected_environments/production"
	// envJSON identifies the env JSON constant used by this package.
	envJSON = `{
		"name": "production",
		"deploy_access_levels": [
			{"id": 1, "access_level": 40, "access_level_description": "Maintainers", "user_id": 0, "group_id": 0}
		],
		"required_approval_count": 2,
		"approval_rules": [
			{"id": 10, "user_id": 5, "group_id": 0, "access_level": 40, "access_level_description": "Maintainers", "required_approvals": 1}
		]
	}`
)

// ---------- List ----------.

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedEnvs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+envJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Environments) != 1 {
		t.Fatalf("len(Environments) = %d, want 1", len(out.Environments))
	}
	if out.Environments[0].Name != "production" {
		t.Errorf("Name = %q, want %q", out.Environments[0].Name, "production")
	}
	if out.Environments[0].RequiredApprovalCount != 2 {
		t.Errorf("RequiredApprovalCount = %d, want 2", out.Environments[0].RequiredApprovalCount)
	}
}

// TestList_OrderBySortKeyset verifies List forwards order_by, sort, and keyset
// pagination parameters (pagination, page_token) to the GitLab API query.
func TestList_OrderBySortKeyset(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedEnvs {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+envJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	for _, want := range []string{"order_by=name", "sort=desc", "pagination=keyset", "page_token=tok123"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestList_MissingProjectID verifies List when missing project ID.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing project_id")
	}
}

// ---------- Get ----------.

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedEnv1 {
			testutil.RespondJSON(w, http.StatusOK, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", Environment: "production"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
	if len(out.DeployAccessLevels) != 1 {
		t.Fatalf("len(DeployAccessLevels) = %d, want 1", len(out.DeployAccessLevels))
	}
	if out.DeployAccessLevels[0].AccessLevel != 40 {
		t.Errorf("AccessLevel = %d, want 40", out.DeployAccessLevels[0].AccessLevel)
	}
	if len(out.ApprovalRules) != 1 {
		t.Fatalf("len(ApprovalRules) = %d, want 1", len(out.ApprovalRules))
	}
	if out.ApprovalRules[0].RequiredApprovalCount != 1 {
		t.Errorf("RequiredApprovalCount = %d, want 1", out.ApprovalRules[0].RequiredApprovalCount)
	}
}

// TestGet_MissingProjectID verifies Get when missing project ID.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{Environment: "production"})
	if err == nil {
		t.Fatal("Get() expected error for missing project_id")
	}
}

// TestGet_MissingEnvironment verifies Get when missing environment.
func TestGet_MissingEnvironment(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Get() expected error for missing environment")
	}
}

// ---------- Protect ----------.

// TestProtect_Success verifies Protect when success.
func TestProtect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID: "42",
		Name:      "production",
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
}

// TestProtect_WithAccessLevels verifies Protect when with access levels.
func TestProtect_WithAccessLevels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	al := 40
	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID: "42",
		Name:      "production",
		DeployAccessLevels: []DeployAccessLevelInput{
			{AccessLevel: &al},
		},
		ApprovalRules: []ApprovalRuleInput{
			{AccessLevel: &al, RequiredApprovalCount: new(int64(1))},
		},
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
}

// TestProtect_WithAllOptionalFields validates the Protect function covers all
// optional field branches: UserID, GroupID, GroupInheritanceType in both
// DeployAccessLevels and ApprovalRules.
func TestProtect_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	al := 30
	uid := int64(5)
	gid := int64(10)
	git := int64(0)

	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID: "42",
		Name:      "staging",
		DeployAccessLevels: []DeployAccessLevelInput{
			{UserID: &uid, GroupID: &gid, AccessLevel: &al, GroupInheritanceType: &git},
		},
		ApprovalRules: []ApprovalRuleInput{
			{UserID: &uid, GroupID: &gid, AccessLevel: &al, RequiredApprovalCount: new(int64(2)), GroupInheritanceType: &git},
		},
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	for _, want := range []string{"deploy_access_levels", "user_id", "group_id", "group_inheritance_type", "approval_rules", "required_approvals"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q", want)
			}
		})
	}
}

// TestGet_CancelledContext validates that Get returns an error when the
// context is cancelled before the API call.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", Environment: "prod"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestProtect_CancelledContext validates that Protect returns an error when the
// context is cancelled before the API call.
func TestProtect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, envJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Protect(ctx, client, ProtectInput{ProjectID: "42", Name: "prod"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestUnprotect_CancelledContext validates that Unprotect returns an error when
// the context is cancelled before the API call.
func TestUnprotect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Unprotect(ctx, client, UnprotectInput{ProjectID: "42", Environment: "prod"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestProtect_MissingName verifies Protect when missing name.
func TestProtect_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Protect() expected error for missing name")
	}
}

// ---------- Update ----------.

// TestUpdate_Success verifies Update when success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProtectedEnv1 {
			testutil.RespondJSON(w, http.StatusOK, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "42",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
}

// TestUpdate_MissingProjectID verifies Update when missing project ID.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{Environment: "production"})
	if err == nil {
		t.Fatal("Update() expected error for missing project_id")
	}
}

// TestUpdate_MissingEnvironment verifies Update when missing environment.
func TestUpdate_MissingEnvironment(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Update() expected error for missing environment")
	}
}

// ---------- Unprotect ----------.

// TestUnprotect_Success verifies Unprotect when success.
func TestUnprotect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProtectedEnv1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42", Environment: "production"})
	if err != nil {
		t.Fatalf("Unprotect() unexpected error: %v", err)
	}
}

// TestUnprotect_MissingProjectID verifies Unprotect when missing project ID.
func TestUnprotect_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{Environment: "production"})
	if err == nil {
		t.Fatal("Unprotect() expected error for missing project_id")
	}
}

// TestUnprotect_MissingEnvironment verifies Unprotect when missing environment.
func TestUnprotect_MissingEnvironment(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Unprotect() expected error for missing environment")
	}
}

// ---------- Formatters ----------.

// cardHints is the guidance section every protected-environment card closes
// with.
const cardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'environment.protected_update' to change the deploy access levels or approval rules\n" +
	"- Use action 'environment.protected_unprotect' to remove this environment's protection\n"

// listHints is the guidance section the listing closes with. The table carries
// no link, so the preserve-links instruction is not written.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'environment.protected_get' to see one environment's rules in full\n" +
	"- Use action 'environment.protected_protect' to protect another environment or wildcard\n" +
	"- Use action 'environment.protected_list' to page through the rest of the project's protected environments\n"

// TestFormatOutputMarkdown_WithRules pins the whole card of a protected
// environment: the headline defers to the per-rule counts below it, and every
// rule says which role it grants, to whom, and whether a group rule reaches
// inherited members.
func TestFormatOutputMarkdown_WithRules(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		Name:                  "production",
		RequiredApprovalCount: 2,
		DeployAccessLevels: []AccessLevelOutput{
			{ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
			{ID: 2, AccessLevelDescription: "Sam Bauch", UserID: 123},
		},
		ApprovalRules: []ApprovalRuleOutput{
			{ID: 10, AccessLevel: 40, AccessLevelDescription: "Maintainers", RequiredApprovalCount: 1},
			{ID: 11, AccessLevelDescription: "Release managers", GroupID: 55, GroupInheritanceType: 1, RequiredApprovalCount: 2},
		},
	})

	want := "## Protected Environment: production\n\n" +
		"- **Required Approvals**: per approval rule (see below)\n" +
		"\n### Deploy Access Levels\n\n" +
		"| ID | Level | Grantee | Description | Inheritance |\n| --- | --- | --- | --- | --- |\n" +
		"| 1 | Maintainer |  | Maintainers |  |\n" +
		"| 2 | - | user #123 | Sam Bauch |  |\n" +
		"\n### Approval Rules\n\n" +
		"| ID | Level | Grantee | Description | Required | Inheritance |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 10 | Maintainer |  | Maintainers | 1 |  |\n" +
		"| 11 | - | group #55 | Release managers | 2 | inherited |\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_NoRules pins the card of an environment on the
// unified approval setting: the headline count GitLab sent, and no table.
func TestFormatOutputMarkdown_NoRules(t *testing.T) {
	got := FormatOutputMarkdown(Output{Name: "staging", RequiredApprovalCount: 1})

	want := "## Protected Environment: staging\n\n" +
		"- **Required Approvals**: 1\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_Empty verifies FormatOutputMarkdown renders nothing
// at all for a value carrying no environment name.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	if md := FormatOutputMarkdown(Output{}); md != "" {
		t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", md)
	}
}

// TestFormatListMarkdown pins the whole listing: the heading counting what
// GitLab reported, the table, the pagination line and one guidance section.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Environments: []Output{
			{Name: "production", RequiredApprovalCount: 2, DeployAccessLevels: []AccessLevelOutput{{ID: 1}}},
			{Name: "staging", RequiredApprovalCount: 0},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1, Page: 1, PerPage: 20},
	})

	want := "## Protected Environments (2)\n\n" +
		"| Name | Required Approvals | Deploy Access Levels | Approval Rules |\n| --- | --- | --- | --- |\n" +
		"| production | 2 | 1 | 0 |\n" +
		"| staging | 0 | 0 | 0 |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_KeysetPage pins the heading of a page GitLab sent no
// total for: it counts the rows shown and says more are available, rather than
// claiming a total of zero above two rows.
func TestFormatListMarkdown_KeysetPage(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Environments: []Output{{Name: "production"}, {Name: "staging"}},
		Pagination:   toolutil.PaginationOutput{HasMore: true},
	})

	want := "## Protected Environments (2 shown, more available)\n\n" +
		"| Name | Required Approvals | Deploy Access Levels | Approval Rules |\n| --- | --- | --- | --- |\n" +
		"| production | 0 | 0 | 0 |\n" +
		"| staging | 0 | 0 | 0 |\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins that a project with no protected
// environments renders the one sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No protected environments found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestUpdate_WithAllFields verifies Update maps all optional fields including
// DeployAccessLevels and ApprovalRules with all sub-fields populated.
func TestUpdate_WithAllFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProtectedEnv1 {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, envJSON)
			return
		}
		http.NotFound(w, r)
	}))

	reqApproval := int64(2)
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:             "42",
		Environment:           "production",
		Name:                  "staging",
		RequiredApprovalCount: &reqApproval,
		DeployAccessLevels: []UpdateDeployAccessLevelInput{
			{
				ID:                   new(int64(1)),
				AccessLevel:          new(30),
				UserID:               new(int64(10)),
				GroupID:              new(int64(20)),
				GroupInheritanceType: new(int64(1)),
				Destroy:              new(false),
			},
		},
		ApprovalRules: []UpdateApprovalRuleInput{
			{
				ID:                    new(int64(2)),
				AccessLevel:           new(40),
				UserID:                new(int64(11)),
				GroupID:               new(int64(21)),
				RequiredApprovalCount: new(int64(1)),
				GroupInheritanceType:  new(int64(0)),
				Destroy:               new(true),
			},
		},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "production" {
		t.Errorf("Name = %q, want %q", out.Name, "production")
	}
	for _, want := range []string{"deploy_access_levels", "user_id", "group_id", "group_inheritance_type", "approval_rules", "required_approvals", "_destroy"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q", want)
			}
		})
	}
}

// TestUpdate_APIError verifies Update returns error on API failure.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "42",
		Environment: "production",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestUpdate_CancelledContext verifies Update respects context cancellation.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{
		ProjectID:   "42",
		Environment: "production",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestList_APIError verifies List returns error on API failure.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestList_CancelledContext verifies List respects context cancellation.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestGet_APIError verifies Get returns error on API failure.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", Environment: "production"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestProtect_MissingProjectID verifies Protect returns error for empty project_id.
func TestProtect_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{Name: "staging"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestProtect_APIError verifies Protect returns error on API failure.
func TestProtect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", Name: "staging"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestProtect_Forbidden verifies Protect returns permission guidance for 403 responses.
func TestProtect_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", Name: "staging"})
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}
	if !strings.Contains(err.Error(), "Premium/Ultimate") {
		t.Fatalf("error = %v, want permission hint", err)
	}
}

// TestUpdate_NotFound verifies Update returns protection guidance for 404 responses.
func TestUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", Environment: "production"})
	if err == nil {
		t.Fatal("expected error for not found response")
	}
	if !strings.Contains(err.Error(), "gitlab_protected_environment_protect") {
		t.Fatalf("error = %v, want protect hint", err)
	}
}

// TestUnprotect_APIError verifies Unprotect returns error on API failure.
func TestUnprotect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42", Environment: "production"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestProtect_WithRequiredApprovalCount verifies that Protect forwards the
// required_approval_count value to the GitLab API when input.RequiredApprovalCount
// is non-nil. This targets the optional-field branch at the top of Protect.
func TestProtect_WithRequiredApprovalCount(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
			return
		}
		http.NotFound(w, r)
	}))
	count := int64(3)
	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:             "42",
		Name:                  "production",
		RequiredApprovalCount: &count,
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	if !strings.Contains(capturedBody, "required_approval_count") {
		t.Errorf("request body missing required_approval_count; body=%q", capturedBody)
	}
}
