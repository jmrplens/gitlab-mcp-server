// protected_envs_test.go contains unit tests for the protected environment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package protectedenvs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
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
	// envJSON is the protected environment every handler test is answered
	// with. No two numbers inside one rule agree and each rule names exactly
	// one kind of grantee, the way GitLab answers: with the id, the user and
	// the group all zero, a converter reading a neighbour's key would be
	// indistinguishable from one reading its own.
	envJSON = `{
		"name": "production",
		"deploy_access_levels": [
			{"id": 11, "access_level": 40, "access_level_description": "Maintainers", "user_id": 0, "group_id": 0, "group_inheritance_type": 0},
			{"id": 12, "access_level": 0, "access_level_description": "Ada Lovelace", "user_id": 71, "group_id": 0, "group_inheritance_type": 0},
			{"id": 13, "access_level": 0, "access_level_description": "Release Engineers", "user_id": 0, "group_id": 83, "group_inheritance_type": 0}
		],
		"required_approval_count": 2,
		"approval_rules": [
			{"id": 21, "user_id": 0, "group_id": 0, "access_level": 30, "access_level_description": "Developers", "required_approvals": 3, "group_inheritance_type": 0},
			{"id": 22, "user_id": 94, "group_id": 0, "access_level": 0, "access_level_description": "Grace Hopper", "required_approvals": 1, "group_inheritance_type": 0},
			{"id": 23, "user_id": 0, "group_id": 105, "access_level": 0, "access_level_description": "Security Team", "required_approvals": 4, "group_inheritance_type": 1}
		]
	}`
)

// wantEnvOutput is what envJSON has to convert to, field for field. It is a
// whole-struct expectation rather than a handful of spot checks because a
// converter that reads the wrong source field is a straight-line assignment
// no mutation or condition gate can see.
func wantEnvOutput() Output {
	return Output{
		Name:                  "production",
		RequiredApprovalCount: 2,
		DeployAccessLevels: []AccessLevelOutput{
			{ID: 11, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
			{ID: 12, AccessLevelDescription: "Ada Lovelace", UserID: 71},
			{ID: 13, AccessLevelDescription: "Release Engineers", GroupID: 83},
		},
		ApprovalRules: []ApprovalRuleOutput{
			{ID: 21, AccessLevel: 30, AccessLevelDescription: "Developers", RequiredApprovalCount: 3},
			{ID: 22, AccessLevelDescription: "Grace Hopper", UserID: 94, RequiredApprovalCount: 1},
			{ID: 23, AccessLevelDescription: "Security Team", GroupID: 105, RequiredApprovalCount: 4, GroupInheritanceType: 1},
		},
	}
}

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
	if want := wantEnvOutput(); !reflect.DeepEqual(out.Environments[0], want) {
		t.Errorf("Environments[0] = %+v, want %+v", out.Environments[0], want)
	}
	// The page block comes from the response headers, not from the body, so
	// a list filled from the wrong response would still carry the right rows.
	wantPage := toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1}
	if !reflect.DeepEqual(out.Pagination, wantPage) {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, wantPage)
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

// TestGet_Success pins every field GitLab sent onto the field it has to land
// in, rather than spot-checking two of them. Each access level and approval
// rule names one grantee, so a converter that read a neighbour's key would
// move a user id into the group column and nothing else would notice.
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
	if want := wantEnvOutput(); !reflect.DeepEqual(out, want) {
		t.Errorf("Get() = %+v, want %+v", out, want)
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

// ruleBody mirrors one entry of deploy_access_levels or approval_rules as
// GitLab receives it, and envBody the request around them. The body tests
// decode into these rather than searching the raw text, because every key
// name appears whichever caller field it was filled from: only the values
// tell a builder that read the caller's user id from one that read the group.
type ruleBody struct {
	ID                     *int64  `json:"id"`
	AccessLevel            *int64  `json:"access_level"`
	UserID                 *int64  `json:"user_id"`
	GroupID                *int64  `json:"group_id"`
	GroupInheritanceType   *int64  `json:"group_inheritance_type"`
	AccessLevelDescription *string `json:"access_level_description"`
	RequiredApprovals      *int64  `json:"required_approvals"`
	Destroy                *bool   `json:"_destroy"`
}

// envBody is the protect or update request as GitLab receives it. Every field
// is a pointer so that a key left out reads differently from one sent empty.
type envBody struct {
	Name                  *string     `json:"name"`
	RequiredApprovalCount *int64      `json:"required_approval_count"`
	DeployAccessLevels    *[]ruleBody `json:"deploy_access_levels"`
	ApprovalRules         *[]ruleBody `json:"approval_rules"`
}

// captureEnvBody answers a protect or update call with envJSON and decodes the
// request body it was sent into got.
func captureEnvBody(t *testing.T, got *envBody) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		if decodeErr := json.Unmarshal(body, got); decodeErr != nil {
			t.Errorf("decode request body %q: %v", body, decodeErr)
			http.Error(w, "decode request body", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	})
}

// bodyText renders a request body for a failure message, since a struct of
// pointers otherwise prints as a list of addresses.
func bodyText(t *testing.T, b envBody) string {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(raw)
}

// TestProtect_EveryOptionalField_ReachesItsOwnKeyInTheRequestBody sends one
// rule of each shape GitLab accepts (a role, a user, a group) and compares
// the decoded body field for field. A rule naming a user carries no
// access_level, which is the guard the builder wraps that assignment in, and
// a builder that read a neighbour's field would send the same set of keys.
func TestProtect_EveryOptionalField_ReachesItsOwnKeyInTheRequestBody(t *testing.T) {
	var got envBody
	client := testutil.NewTestClient(t, captureEnvBody(t, &got))

	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:             "42",
		Name:                  "production",
		RequiredApprovalCount: new(int64(7)),
		DeployAccessLevels: []DeployAccessLevelInput{
			{AccessLevel: new(40)},
			{UserID: new(int64(71))},
			{GroupID: new(int64(83)), GroupInheritanceType: new(int64(1))},
		},
		ApprovalRules: []ApprovalRuleInput{
			{AccessLevel: new(30), RequiredApprovalCount: new(int64(3))},
			{UserID: new(int64(94)), AccessLevelDescription: new("Grace Hopper"), RequiredApprovalCount: new(int64(1))},
			{GroupID: new(int64(105)), GroupInheritanceType: new(int64(1)), RequiredApprovalCount: new(int64(4))},
		},
	})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}

	want := envBody{
		Name:                  new("production"),
		RequiredApprovalCount: new(int64(7)),
		DeployAccessLevels: &[]ruleBody{
			{AccessLevel: new(int64(40))},
			{UserID: new(int64(71))},
			{GroupID: new(int64(83)), GroupInheritanceType: new(int64(1))},
		},
		ApprovalRules: &[]ruleBody{
			{AccessLevel: new(int64(30)), RequiredApprovals: new(int64(3))},
			{UserID: new(int64(94)), AccessLevelDescription: new("Grace Hopper"), RequiredApprovals: new(int64(1))},
			{GroupID: new(int64(105)), GroupInheritanceType: new(int64(1)), RequiredApprovals: new(int64(4))},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %s, want %s", bodyText(t, got), bodyText(t, want))
	}
}

// TestProtect_NoRulesGiven_SendsNeitherCollectionKey pins that a call naming
// only the environment leaves deploy_access_levels and approval_rules out of
// the request. An empty array is a different request: GitLab reads it as an
// environment whose rules are now none, rather than as a field left alone.
func TestProtect_NoRulesGiven_SendsNeitherCollectionKey(t *testing.T) {
	var got envBody
	client := testutil.NewTestClient(t, captureEnvBody(t, &got))

	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", Name: "production"})
	if err != nil {
		t.Fatalf("Protect() unexpected error: %v", err)
	}
	if want := (envBody{Name: new("production")}); !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %s, want %s", bodyText(t, got), bodyText(t, want))
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
// inherited members. Both group rules are here on purpose, one direct and one
// inherited, since GitLab's zero means "direct" for a group rule and "not
// applicable" for a rule that is not about a group at all.
func TestFormatOutputMarkdown_WithRules(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		Name:                  "production",
		RequiredApprovalCount: 2,
		DeployAccessLevels: []AccessLevelOutput{
			{ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
			{ID: 2, AccessLevelDescription: "Sam Bauch", UserID: 123},
			{ID: 3, AccessLevelDescription: "Platform team", GroupID: 77},
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
		"| 3 | - | group #77 | Platform team | direct |\n" +
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

// TestUpdate_EveryOptionalField_ReachesItsOwnKeyInTheRequestBody is the
// protect test's counterpart for the update shapes, which carry two fields
// protect has no equivalent of: the id of the entry being edited and the
// _destroy flag that removes it. A rule whose id went out under another key
// would edit nothing and silently add a rule instead.
func TestUpdate_EveryOptionalField_ReachesItsOwnKeyInTheRequestBody(t *testing.T) {
	var got envBody
	client := testutil.NewTestClient(t, captureEnvBody(t, &got))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:             "42",
		Environment:           "production",
		Name:                  "staging",
		RequiredApprovalCount: new(int64(7)),
		DeployAccessLevels: []UpdateDeployAccessLevelInput{
			{ID: new(int64(11)), AccessLevel: new(40)},
			{ID: new(int64(12)), UserID: new(int64(71)), Destroy: new(true)},
			{GroupID: new(int64(83)), GroupInheritanceType: new(int64(1))},
		},
		ApprovalRules: []UpdateApprovalRuleInput{
			{ID: new(int64(21)), AccessLevel: new(30), RequiredApprovalCount: new(int64(3))},
			{ID: new(int64(22)), UserID: new(int64(94)), AccessLevelDescription: new("Grace Hopper"), Destroy: new(true)},
			{GroupID: new(int64(105)), GroupInheritanceType: new(int64(1)), RequiredApprovalCount: new(int64(4))},
		},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}

	want := envBody{
		Name:                  new("staging"),
		RequiredApprovalCount: new(int64(7)),
		DeployAccessLevels: &[]ruleBody{
			{ID: new(int64(11)), AccessLevel: new(int64(40))},
			{ID: new(int64(12)), UserID: new(int64(71)), Destroy: new(true)},
			{GroupID: new(int64(83)), GroupInheritanceType: new(int64(1))},
		},
		ApprovalRules: &[]ruleBody{
			{ID: new(int64(21)), AccessLevel: new(int64(30)), RequiredApprovals: new(int64(3))},
			{ID: new(int64(22)), UserID: new(int64(94)), AccessLevelDescription: new("Grace Hopper"), Destroy: new(true)},
			{GroupID: new(int64(105)), GroupInheritanceType: new(int64(1)), RequiredApprovals: new(int64(4))},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %s, want %s", bodyText(t, got), bodyText(t, want))
	}
}

// TestUpdate_OnlyTheEnvironmentGiven_SendsNoOptionalKey pins that an update
// naming no change sends an empty body. Every key here replaces what the
// environment has, so a name sent empty would rename it to nothing and an
// empty rule array would delete every gate the environment carries.
func TestUpdate_OnlyTheEnvironmentGiven_SendsNoOptionalKey(t *testing.T) {
	var got envBody
	client := testutil.NewTestClient(t, captureEnvBody(t, &got))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", Environment: "production"})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if want := (envBody{}); !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %s, want %s", bodyText(t, got), bodyText(t, want))
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
	if !strings.Contains(err.Error(), "environment.protected_protect") {
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
