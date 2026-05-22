package projectserviceaccounts

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	pathProjectServiceAccounts     = "/api/v4/projects/42/service_accounts"
	pathProjectServiceAccount7     = "/api/v4/projects/42/service_accounts/7"
	pathProjectServiceAccount7PATs = "/api/v4/projects/42/service_accounts/7/personal_access_tokens"

	projectServiceAccountJSON     = `{"id":7,"name":"svc","username":"svc-user","email":"svc@example.com","unconfirmed_email":"pending@example.com"}`
	projectServiceAccountsJSON    = `[{"id":7,"name":"svc","username":"svc-user","email":"svc@example.com"}]`
	projectServiceAccountPATJSON  = `{"id":11,"name":"tok","scopes":["api"],"active":true,"revoked":false,"user_id":7,"token":"glpat-test","expires_at":"2026-12-31"}`
	projectServiceAccountPATsJSON = `[{"id":11,"name":"tok","scopes":["api"],"active":true,"revoked":false,"user_id":7,"expires_at":"2026-12-31"}]`
)

// TestList validates project service account listing, optional filters,
// validation, and API error handling.
func TestList(t *testing.T) {
	tests := []struct {
		name       string
		input      ListInput
		handler    http.HandlerFunc
		wantErr    bool
		wantCount  int
		errContain string
	}{
		{
			name:  "returns accounts on success",
			input: ListInput{ProjectID: "42", OrderBy: "username", Sort: "desc"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathProjectServiceAccounts)
				testutil.AssertQueryParam(t, r, "order_by", "username")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.RespondJSON(w, http.StatusOK, projectServiceAccountsJSON)
			},
			wantCount: 1,
		},
		{
			name:  "returns empty list",
			input: ListInput{ProjectID: "42"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			wantCount: 0,
		},
		{
			name:       "returns error when project_id is empty",
			input:      ListInput{},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "project_id",
		},
		{
			name:  "returns API error",
			input: ListInput{ProjectID: "42"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
			},
			wantErr:    true,
			errContain: "list project service accounts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := List(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				assertErrorContains(t, err, tt.errContain)
				return
			}
			if len(out.Accounts) != tt.wantCount {
				t.Fatalf("len(Accounts) = %d, want %d", len(out.Accounts), tt.wantCount)
			}
		})
	}
}

// TestCreateUpdateDelete validates account mutation handlers and required
// field checks.
func TestCreateUpdateDelete(t *testing.T) {
	t.Run("create account", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccounts)
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountJSON)
		}))
		out, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "svc", Username: "svc-user", Email: "svc@example.com"})
		if err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
		if out.ID != 7 || out.UnconfirmedEmail == "" {
			t.Fatalf("Create() output = %#v", out)
		}
	})

	t.Run("update account", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodPatch)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7)
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountJSON)
		}))
		out, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", ServiceAccountID: 7, Name: "svc"})
		if err != nil {
			t.Fatalf("Update() unexpected error: %v", err)
		}
		if out.Username != "svc-user" {
			t.Fatalf("Update() username = %q, want svc-user", out.Username)
		}
	})

	t.Run("delete account with hard_delete", func(t *testing.T) {
		hardDelete := true
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodDelete)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7)
			testutil.AssertQueryParam(t, r, "hard_delete", "true")
			w.WriteHeader(http.StatusNoContent)
		}))
		if err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", ServiceAccountID: 7, HardDelete: &hardDelete}); err != nil {
			t.Fatalf("Delete() unexpected error: %v", err)
		}
	})

	for _, tt := range []struct {
		name       string
		errContain string
		call       func(*gitlabclient.Client) error
	}{
		{name: "create requires project_id", errContain: "project_id", call: func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{})
			return err
		}},
		{name: "update requires service_account_id", errContain: "service_account_id", call: func(client *gitlabclient.Client) error {
			_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42"})
			return err
		}},
		{name: "delete requires service_account_id", errContain: "service_account_id", call: func(client *gitlabclient.Client) error {
			return Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }))
			assertErrorContains(t, tt.call(client), tt.errContain)
		})
	}
}

// TestPATHandlers validates PAT list/create/rotate/revoke handlers, including
// supported query filters and date validation.
func TestPATHandlers(t *testing.T) {
	t.Run("list PATs with filters", func(t *testing.T) {
		revoked := true
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7PATs)
			testutil.AssertQueryParam(t, r, "revoked", "true")
			testutil.AssertQueryParam(t, r, "search", "deploy")
			testutil.AssertQueryParam(t, r, "state", "active")
			testutil.AssertQueryParam(t, r, "expires_after", "2026-01-01")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATsJSON)
		}))
		out, err := ListPATs(context.Background(), client, ListPATInput{
			ProjectID:        "42",
			ServiceAccountID: 7,
			ExpiresAfter:     "2026-01-01",
			Revoked:          &revoked,
			Search:           "deploy",
			State:            "active",
		})
		if err != nil {
			t.Fatalf("ListPATs() unexpected error: %v", err)
		}
		if len(out.Tokens) != 1 || out.Tokens[0].ID != 11 {
			t.Fatalf("ListPATs() output = %#v", out)
		}
	})

	t.Run("create PAT", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7PATs)
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountPATJSON)
		}))
		out, err := CreatePAT(context.Background(), client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}, ExpiresAt: "2026-12-31"})
		if err != nil {
			t.Fatalf("CreatePAT() unexpected error: %v", err)
		}
		if out.Token != "glpat-test" {
			t.Fatalf("CreatePAT() token = %q, want glpat-test", out.Token)
		}
	})

	t.Run("rotate PAT", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7PATs+"/11/rotate")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATJSON)
		}))
		out, err := RotatePAT(context.Background(), client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11, ExpiresAt: "2026-12-31"})
		if err != nil {
			t.Fatalf("RotatePAT() unexpected error: %v", err)
		}
		if out.ID != 11 {
			t.Fatalf("RotatePAT() ID = %d, want 11", out.ID)
		}
	})

	t.Run("revoke PAT", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodDelete)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7PATs+"/11")
			w.WriteHeader(http.StatusNoContent)
		}))
		if err := RevokePAT(context.Background(), client, RevokePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11}); err != nil {
			t.Fatalf("RevokePAT() unexpected error: %v", err)
		}
	})

	for _, tt := range []struct {
		name       string
		errContain string
		call       func() error
	}{
		{name: "create PAT requires name", errContain: "name", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Scopes: []string{"api"}})
			return err
		}},
		{name: "create PAT validates expires_at", errContain: "invalid expires_at format", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}, ExpiresAt: "bad"})
			return err
		}},
		{name: "rotate PAT requires token_id", errContain: "token_id", call: func() error {
			_, err := RotatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RotatePATInput{ProjectID: "42", ServiceAccountID: 7})
			return err
		}},
		{name: "list PAT validates created_after", errContain: "invalid created_after format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, CreatedAfter: "bad"})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.call(), tt.errContain)
		})
	}
}

// TestActionSpecs verifies project service account catalog metadata and route
// execution through ActionSpecs.
func TestActionSpecs(t *testing.T) {
	client := newProjectServiceAccountCatalogClient(t)
	specs := ActionSpecs(client)
	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	byTool := specsByTool(t, specs)
	if !byTool["gitlab_project_service_account_list"].ReadOnly {
		t.Fatal("gitlab_project_service_account_list should be read-only")
	}
	for _, toolName := range []string{"gitlab_project_service_account_delete", "gitlab_project_service_account_pat_revoke"} {
		if !byTool[toolName].Destructive || !byTool[toolName].Route.Destructive {
			t.Fatalf("%s should be destructive", toolName)
		}
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_project_service_account_list", map[string]any{"project_id": "42"}},
		{"gitlab_project_service_account_create", map[string]any{"project_id": "42", "name": "svc"}},
		{"gitlab_project_service_account_update", map[string]any{"project_id": "42", "service_account_id": 7, "name": "svc"}},
		{"gitlab_project_service_account_delete", map[string]any{"project_id": "42", "service_account_id": 7}},
		{"gitlab_project_service_account_pat_list", map[string]any{"project_id": "42", "service_account_id": 7}},
		{"gitlab_project_service_account_pat_create", map[string]any{"project_id": "42", "service_account_id": 7, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_project_service_account_pat_rotate", map[string]any{"project_id": "42", "service_account_id": 7, "token_id": 11}},
		{"gitlab_project_service_account_pat_revoke", map[string]any{"project_id": "42", "service_account_id": 7, "token_id": 11}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestMarkdownFormatters verifies the registered Markdown formatters include
// identifying fields and empty-state text.
func TestMarkdownFormatters(t *testing.T) {
	account := Output{ID: 7, Name: "svc", Username: "svc-user", Email: "svc@example.com"}
	if md := FormatMarkdownString(account); !strings.Contains(md, "svc-user") || !strings.Contains(md, "gitlab_project_service_account_update") {
		t.Fatalf("FormatMarkdownString missing expected content:\n%s", md)
	}
	if md := FormatListMarkdownString(ListOutput{}); !strings.Contains(md, "No project service accounts found") {
		t.Fatalf("FormatListMarkdownString missing empty state:\n%s", md)
	}
	token := PATOutput{ID: 11, Name: "tok", Active: true, Scopes: []string{"api"}, UserID: 7, Token: "glpat-test"}
	if md := FormatPATMarkdownString(token); !strings.Contains(md, "glpat-test") || !strings.Contains(md, "gitlab_project_service_account_pat_rotate") {
		t.Fatalf("FormatPATMarkdownString missing expected content:\n%s", md)
	}
	if md := FormatListPATMarkdownString(ListPATOutput{}); !strings.Contains(md, "No project service account tokens found") {
		t.Fatalf("FormatListPATMarkdownString missing empty state:\n%s", md)
	}
}

func specsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		if spec.OwnerPackage != "projectserviceaccounts" {
			t.Fatalf("OwnerPackage for %s = %q, want projectserviceaccounts", spec.Name, spec.OwnerPackage)
		}
		if spec.Edition != "premium" {
			t.Fatalf("Edition for %s = %q, want premium", spec.Name, spec.Edition)
		}
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

func newProjectServiceAccountCatalogClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountsJSON)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATsJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rotate"):
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATJSON)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountPATJSON)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/service_accounts"):
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountJSON)
		case r.Method == http.MethodPatch:
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q should contain %q", err.Error(), want)
	}
}
