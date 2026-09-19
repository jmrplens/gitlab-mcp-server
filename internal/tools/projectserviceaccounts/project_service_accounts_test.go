package projectserviceaccounts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	pathProjectServiceAccounts     = "/api/v4/projects/42/service_accounts"
	pathProjectServiceAccount7     = "/api/v4/projects/42/service_accounts/7"
	pathProjectServiceAccount7PATs = "/api/v4/projects/42/service_accounts/7/personal_access_tokens"

	projectServiceAccountJSON     = `{"id":7,"name":"svc","username":"svc-user","email":"svc@example.com","unconfirmed_email":"pending@example.com"}`
	projectServiceAccountsJSON    = `[{"id":7,"name":"svc","username":"svc-user","email":"svc@example.com"}]`
	projectServiceAccountPATJSON  = `{"id":11,"name":"tok","scopes":["api"],"active":true,"revoked":false,"user_id":7,"token":"glpat-test","expires_at":"2026-12-31","created_at":"2026-01-01T02:03:04Z","last_used_at":"2026-01-02T03:04:05Z","description":"deploy token"}`
	projectServiceAccountPATsJSON = `[{"id":11,"name":"tok","scopes":["api"],"active":true,"revoked":false,"user_id":7,"expires_at":"2026-12-31","created_at":"2026-01-01T02:03:04Z","last_used_at":"2026-01-02T03:04:05Z","description":"deploy token"}]`
)

// TestListPATs_ReadsWhatTheSDKDoesNotModel verifies a project service
// account's tokens carry, beside what client-go decoded, the three fields
// lib/api/entities/personal_access_token.rb sends and gl.PersonalAccessToken
// does not, each paired with its token by position.
func TestListPATs_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":11,"name":"tok","granular":true,"last_used_ips":["192.0.2.10"],`+
			`"granular_scopes":[{"access":"personal_projects","permissions":["read_job"],"project_id":42}]},{"id":12,"name":"other"}]`)
	}))

	out, err := ListPATs(context.Background(), client, ListPATInput{ProjectID: "42", ServiceAccountID: 7})
	if err != nil {
		t.Fatalf("ListPATs() unexpected error: %v", err)
	}
	if len(out.Tokens) != 2 || !out.Tokens[0].Granular || len(out.Tokens[0].GranularScopes) != 1 ||
		out.Tokens[0].GranularScopes[0].ProjectID != 42 || len(out.Tokens[0].LastUsedIPs) != 1 ||
		out.Tokens[1].Granular || out.Tokens[1].GranularScopes != nil {
		t.Errorf("ListPATs() tokens = %+v, want each paired with its captured fields", out.Tokens)
	}
}

// TestPATHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to every handler that presents a token:
// GitLab's answer decodes for the SDK and not for the fields read beside it,
// and the handler reports it rather than swallowing it.
func TestPATHandlers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"id":11,"name":"tok","granular":"not-a-bool"}`
		if r.Method == http.MethodGet {
			body = "[" + body + "]"
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			_, err := ListPATs(context.Background(), client, ListPATInput{ProjectID: "42", ServiceAccountID: 7})
			return err
		}},
		{Name: "create", Call: func() error {
			_, err := CreatePAT(context.Background(), client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}})
			return err
		}},
		{Name: "rotate", Call: func() error {
			_, err := RotatePAT(context.Background(), client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11})
			return err
		}},
	})
}

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
			input: ListInput{ProjectID: "42", PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 50}, KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "9"}, OrderBy: "username", Sort: "desc"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathProjectServiceAccounts)
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "50")
				testutil.AssertQueryParam(t, r, "pagination", "keyset")
				testutil.AssertQueryParam(t, r, "page_token", "9")
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
		out, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", ServiceAccountID: 7, Name: "svc", Username: "svc-user", Email: "svc@example.com"})
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
		{name: "create with name requires project_id", errContain: "project_id", call: func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{Name: "svc"})
			return err
		}},
		{name: "create requires project_id", errContain: "project_id", call: func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{})
			return err
		}},
		{name: "update requires project_id", errContain: "project_id", call: func(client *gitlabclient.Client) error {
			_, err := Update(context.Background(), client, UpdateInput{ServiceAccountID: 7})
			return err
		}},
		{name: "update requires service_account_id", errContain: "service_account_id", call: func(client *gitlabclient.Client) error {
			_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42"})
			return err
		}},
		{name: "delete requires project_id", errContain: "project_id", call: func(client *gitlabclient.Client) error {
			return Delete(context.Background(), client, DeleteInput{ServiceAccountID: 7})
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

	t.Run("returns API errors", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
		}))
		for _, tt := range []struct {
			name       string
			errContain string
			call       func() error
		}{
			{name: "create", errContain: "create project service account", call: func() error {
				_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "svc"})
				return err
			}},
			{name: "update", errContain: "update project service account", call: func() error {
				_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", ServiceAccountID: 7, Name: "svc"})
				return err
			}},
			{name: "delete", errContain: "delete project service account", call: func() error {
				return Delete(context.Background(), client, DeleteInput{ProjectID: "42", ServiceAccountID: 7})
			}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				assertErrorContains(t, tt.call(), tt.errContain)
			})
		}
	})
}

// TestAccountMutations_OptionalFieldsTravelUnderTheirOwnKeys pins both halves
// of what the account handlers put on the wire: a field the caller named
// reaches GitLab under its own key, and one the caller left unset does not
// reach it at all. The guards deciding that are invertible in silence, and
// each direction is its own defect: inverted, the caller's name never leaves
// this process, while an unset one arrives as "", which GitLab reads as an
// instruction to blank the field.
func TestAccountMutations_OptionalFieldsTravelUnderTheirOwnKeys(t *testing.T) {
	t.Run("create sends every named field", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodeRequestBody(t, r)
			assertSent(t, body, "name", "svc")
			assertSent(t, body, "username", "svc-user")
			assertSent(t, body, "email", "svc@example.com")
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountJSON)
		}))
		if _, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "svc", Username: "svc-user", Email: "svc@example.com"}); err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
	})

	t.Run("create omits every unnamed field", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodeRequestBody(t, r)
			assertSent(t, body, "username", "svc-user")
			assertNotSent(t, body, "name", "email")
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountJSON)
		}))
		if _, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Username: "svc-user"}); err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
	})

	t.Run("update sends every named field", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodeRequestBody(t, r)
			assertSent(t, body, "name", "renamed")
			assertSent(t, body, "username", "renamed-user")
			assertSent(t, body, "email", "renamed@example.com")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountJSON)
		}))
		if _, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", ServiceAccountID: 7, Name: "renamed", Username: "renamed-user", Email: "renamed@example.com"}); err != nil {
			t.Fatalf("Update() unexpected error: %v", err)
		}
	})

	t.Run("update omits every unnamed field", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodeRequestBody(t, r)
			assertSent(t, body, "email", "renamed@example.com")
			assertNotSent(t, body, "name", "username")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountJSON)
		}))
		if _, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", ServiceAccountID: 7, Email: "renamed@example.com"}); err != nil {
			t.Fatalf("Update() unexpected error: %v", err)
		}
	})
}

// TestPATMutations_RequestCarriesWhatTheCallerAskedFor pins the token requests
// the same way: the name, the scopes and the two optional fields each reach
// GitLab under their own key, and an expiry or description the caller withheld
// is absent rather than empty. A token created with the wrong scopes or with no
// expiry at all is a credential nobody asked for, and nothing above this reads
// the request back.
func TestPATMutations_RequestCarriesWhatTheCallerAskedFor(t *testing.T) {
	t.Run("create sends name, scopes, description and expiry", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodeRequestBody(t, r)
			assertSent(t, body, "name", "tok")
			assertSent(t, body, "description", "deploy token")
			assertSent(t, body, "expires_at", "2026-12-31")
			assertSentList(t, body, "scopes", "api", "read_repository")
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountPATJSON)
		}))
		if _, err := CreatePAT(context.Background(), client, CreatePATInput{
			ProjectID: "42", ServiceAccountID: 7, Name: "tok",
			Scopes: []string{"api", "read_repository"}, Description: "deploy token", ExpiresAt: "2026-12-31",
		}); err != nil {
			t.Fatalf("CreatePAT() unexpected error: %v", err)
		}
	})

	t.Run("create omits a description and expiry the caller withheld", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := decodeRequestBody(t, r)
			assertSent(t, body, "name", "tok")
			assertNotSent(t, body, "description", "expires_at")
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountPATJSON)
		}))
		if _, err := CreatePAT(context.Background(), client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}}); err != nil {
			t.Fatalf("CreatePAT() unexpected error: %v", err)
		}
	})

	t.Run("rotate sends the new expiry", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertSent(t, decodeRequestBody(t, r), "expires_at", "2026-12-31")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATJSON)
		}))
		if _, err := RotatePAT(context.Background(), client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11, ExpiresAt: "2026-12-31"}); err != nil {
			t.Fatalf("RotatePAT() unexpected error: %v", err)
		}
	})

	t.Run("rotate omits an expiry the caller withheld", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertNotSent(t, decodeRequestBody(t, r), "expires_at")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATJSON)
		}))
		if _, err := RotatePAT(context.Background(), client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11}); err != nil {
			t.Fatalf("RotatePAT() unexpected error: %v", err)
		}
	})
}

// TestPATList validates PAT listing, supported query filters, and output mapping.
func TestPATList(t *testing.T) {
	t.Run("list PATs with filters", func(t *testing.T) {
		revoked := true
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7PATs)
			testutil.AssertQueryParam(t, r, "page", "2")
			testutil.AssertQueryParam(t, r, "per_page", "50")
			testutil.AssertQueryParam(t, r, "revoked", "true")
			testutil.AssertQueryParam(t, r, "search", "deploy")
			testutil.AssertQueryParam(t, r, "state", "active")
			testutil.AssertQueryParam(t, r, "sort", "created_desc")
			testutil.AssertQueryParam(t, r, "order_by", "created_at")
			testutil.AssertQueryParam(t, r, "pagination", "keyset")
			testutil.AssertQueryParam(t, r, "page_token", "55")
			testutil.AssertQueryParam(t, r, "user_id", "7")
			testutil.AssertQueryParam(t, r, "created_after", "2026-01-01T02:03:04Z")
			testutil.AssertQueryParam(t, r, "created_before", "2026-01-02T00:00:00Z")
			testutil.AssertQueryParam(t, r, "expires_after", "2026-01-01")
			testutil.AssertQueryParam(t, r, "expires_before", "2026-12-31")
			testutil.AssertQueryParam(t, r, "last_used_after", "2026-01-03T00:00:00Z")
			testutil.AssertQueryParam(t, r, "last_used_before", "2026-01-04T02:03:04Z")
			testutil.RespondJSON(w, http.StatusOK, projectServiceAccountPATsJSON)
		}))
		out, err := ListPATs(context.Background(), client, ListPATInput{
			ProjectID:        "42",
			ServiceAccountID: 7,
			Page:             2, PerPage: 50,
			Pagination: "keyset", PageToken: "55",
			OrderBy:        "created_at",
			CreatedAfter:   "2026-01-01T02:03:04Z",
			CreatedBefore:  "2026-01-02",
			ExpiresAfter:   "2026-01-01",
			ExpiresBefore:  "2026-12-31",
			LastUsedAfter:  "2026-01-03",
			LastUsedBefore: "2026-01-04T02:03:04Z",
			Revoked:        &revoked,
			UserID:         7,
			Search:         "deploy",
			Sort:           "created_desc",
			State:          "active",
		})
		if err != nil {
			t.Fatalf("ListPATs() unexpected error: %v", err)
		}
		if len(out.Tokens) != 1 || out.Tokens[0].ID != 11 {
			t.Fatalf("ListPATs() output = %#v", out)
		}
		if out.Tokens[0].CreatedAt == "" || out.Tokens[0].LastUsedAt == "" || out.Tokens[0].Description == "" {
			t.Fatalf("ListPATs() token timestamps/description not mapped: %#v", out.Tokens[0])
		}
	})
}

// TestPATMutations validates PAT create, rotate, and revoke handlers.
func TestPATMutations(t *testing.T) {
	t.Run("create PAT", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.AssertRequestPath(t, r, pathProjectServiceAccount7PATs)
			testutil.RespondJSON(w, http.StatusCreated, projectServiceAccountPATJSON)
		}))
		out, err := CreatePAT(context.Background(), client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}, Description: "deploy token", ExpiresAt: "2026-12-31"})
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
}

// TestPATValidation validates PAT required fields and date parsing errors.
func TestPATValidation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		errContain string
		call       func() error
	}{
		{name: "list PATs requires project_id", errContain: "project_id", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ServiceAccountID: 7})
			return err
		}},
		{name: "list PATs requires service_account_id", errContain: "service_account_id", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42"})
			return err
		}},
		{name: "create PAT requires project_id", errContain: "project_id", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}})
			return err
		}},
		{name: "create PAT requires service_account_id", errContain: "service_account_id", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ProjectID: "42", Name: "tok", Scopes: []string{"api"}})
			return err
		}},
		{name: "create PAT requires name", errContain: "name", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Scopes: []string{"api"}})
			return err
		}},
		{name: "create PAT requires scopes", errContain: "scopes", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok"})
			return err
		}},
		{name: "create PAT validates expires_at", errContain: "invalid expires_at format", call: func() error {
			_, err := CreatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}, ExpiresAt: "bad"})
			return err
		}},
		{name: "revoke PAT requires project_id", errContain: "project_id", call: func() error {
			return RevokePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RevokePATInput{ServiceAccountID: 7, TokenID: 11})
		}},
		{name: "revoke PAT requires service_account_id", errContain: "service_account_id", call: func() error {
			return RevokePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RevokePATInput{ProjectID: "42", TokenID: 11})
		}},
		{name: "revoke PAT requires token_id", errContain: "token_id", call: func() error {
			return RevokePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RevokePATInput{ProjectID: "42", ServiceAccountID: 7})
		}},
		{name: "rotate PAT requires project_id", errContain: "project_id", call: func() error {
			_, err := RotatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RotatePATInput{ServiceAccountID: 7, TokenID: 11})
			return err
		}},
		{name: "rotate PAT requires service_account_id", errContain: "service_account_id", call: func() error {
			_, err := RotatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RotatePATInput{ProjectID: "42", TokenID: 11})
			return err
		}},
		{name: "rotate PAT requires token_id", errContain: "token_id", call: func() error {
			_, err := RotatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RotatePATInput{ProjectID: "42", ServiceAccountID: 7})
			return err
		}},
		{name: "rotate PAT validates expires_at", errContain: "invalid expires_at format", call: func() error {
			_, err := RotatePAT(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11, ExpiresAt: "bad"})
			return err
		}},
		{name: "list PAT validates created_after", errContain: "invalid created_after format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, CreatedAfter: "bad"})
			return err
		}},
		{name: "list PAT validates created_before", errContain: "invalid created_before format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, CreatedBefore: "bad"})
			return err
		}},
		{name: "list PAT validates last_used_after", errContain: "invalid last_used_after format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, LastUsedAfter: "bad"})
			return err
		}},
		{name: "list PAT validates last_used_before", errContain: "invalid last_used_before format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, LastUsedBefore: "bad"})
			return err
		}},
		{name: "list PAT validates expires_after", errContain: "invalid expires_after format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, ExpiresAfter: "bad"})
			return err
		}},
		{name: "list PAT validates expires_before", errContain: "invalid expires_before format", call: func() error {
			_, err := ListPATs(context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })), ListPATInput{ProjectID: "42", ServiceAccountID: 7, ExpiresBefore: "bad"})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.call(), tt.errContain)
		})
	}
}

// TestPATAPIErrors validates PAT handlers wrap GitLab API failures.
func TestPATAPIErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	for _, tt := range []struct {
		name       string
		errContain string
		call       func() error
	}{
		{name: "list", errContain: "list project service account PATs", call: func() error {
			_, err := ListPATs(context.Background(), client, ListPATInput{ProjectID: "42", ServiceAccountID: 7})
			return err
		}},
		{name: "create", errContain: "create project service account PAT", call: func() error {
			_, err := CreatePAT(context.Background(), client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}})
			return err
		}},
		{name: "rotate", errContain: "rotate project service account PAT", call: func() error {
			_, err := RotatePAT(context.Background(), client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11})
			return err
		}},
		{name: "revoke", errContain: "revoke project service account PAT", call: func() error {
			return RevokePAT(context.Background(), client, RevokePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.call(), tt.errContain)
		})
	}
}

// TestPATTokenIDHint verifies ambiguous token lookup failures guide callers to
// use the PAT ID rather than the service account user ID. GitLab answers a
// token id it cannot place with any of three codes, depending on where the
// lookup gave up, and the hint is attached to all three: under the two that
// only the 404 used to stand in for, a caller was told the request was refused
// and nothing about the identifier that refused it.
func TestPATTokenIDHint(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, status, `{"message":"refused"}`)
			}))
			for _, tt := range []struct {
				name string
				call func() error
			}{
				{name: "rotate", call: func() error {
					_, err := RotatePAT(context.Background(), client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 7})
					return err
				}},
				{name: "revoke", call: func() error {
					return RevokePAT(context.Background(), client, RevokePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 7})
				}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					err := tt.call()
					for _, want := range []string{"token_id", "service_account_id", "service_account_pat_list", "service_account_pat_create"} {
						assertErrorContains(t, err, want)
					}
				})
			}
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
		t.Run(toolName, func(t *testing.T) {
			if !byTool[toolName].Destructive || !byTool[toolName].Route.Destructive {
				t.Fatalf("%s should be destructive", toolName)
			}
		})
	}
	for _, spec := range specs {
		description := spec.IndividualTool.Description
		for _, want := range []string{"Returns:", "See also:"} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(description, want) {
					t.Fatalf("%s description = %q, want %q", spec.IndividualTool.Name, description, want)
				}
			})
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

// TestMarkdownFormatters verifies the whole document each registered
// formatter renders: the card's items and guidance for one account and one
// token, the table and footer for a page of each, and the one sentence an
// empty page answers with.
func TestMarkdownFormatters(t *testing.T) {
	account := Output{ID: 7, Name: "svc", Username: "svc-user", Email: "svc@example.com", PublicEmail: "public@example.com", UnconfirmedEmail: "pending@example.com"}
	token := PATOutput{ID: 11, Name: "tok", Active: true, Scopes: []string{"api"}, UserID: 7, Token: "glpat-test", CreatedAt: "2026-01-01T02:03:04Z", LastUsedAt: "2026-01-02T03:04:05Z", ExpiresAt: "2026-12-31"}

	t.Run("account card", func(t *testing.T) {
		want := "## Project Service Account: svc-user\n\n" +
			"- **ID**: 7\n" +
			"- **Name**: svc\n" +
			"- **Username**: svc-user\n" +
			"- **Email**: svc@example.com\n" +
			"- **Public Email**: public@example.com\n" +
			"- **Unconfirmed Email**: pending@example.com\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'project.service_account_update' to modify this account\n" +
			"- Use action 'project.service_account_pat_create' to create a token for it\n"
		if got := FormatMarkdownString(account); got != want {
			t.Errorf("FormatMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("account list", func(t *testing.T) {
		want := "## Project Service Accounts (1)\n\n" +
			"| ID | Username | Name | Email |\n| --- | --- | --- | --- |\n" +
			"| 7 | svc-user | svc | svc@example.com |\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'project.service_account_pat_list' to list one account's tokens\n"
		if got := FormatListMarkdownString(ListOutput{Accounts: []Output{account}}); got != want {
			t.Errorf("FormatListMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("empty account list", func(t *testing.T) {
		if got, want := FormatListMarkdownString(ListOutput{}), "No project service accounts found.\n"; got != want {
			t.Errorf("FormatListMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("token card", func(t *testing.T) {
		want := "## Project Service Account Token: tok\n\n" +
			"- **ID**: 11\n" +
			"- **Name**: tok\n" +
			"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
			"- **Scopes**: api\n" +
			"- **Granular**: " + toolutil.BoolEmoji(false) + "\n" +
			"- **User ID**: 7\n" +
			"- **Created**: 1 Jan 2026 02:03 UTC\n" +
			"- **Last used**: 2 Jan 2026 03:04 UTC\n" +
			"- **Expires**: 31 Dec 2026\n" +
			"- **Token**: `glpat-test`\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Store the token securely. It cannot be retrieved later\n" +
			"- Use action 'project.service_account_pat_rotate' to rotate this token\n" +
			"- Use action 'project.service_account_pat_revoke' to revoke it\n"
		if got := FormatPATMarkdownString(token); got != want {
			t.Errorf("FormatPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("token list", func(t *testing.T) {
		want := "## Project Service Account Tokens (1)\n\n" +
			"| ID | Name | Active | Revoked | Scopes | Expires |\n| --- | --- | --- | --- | --- | --- |\n" +
			"| 11 | tok | " + toolutil.BoolEmoji(true) + " | " + toolutil.BoolEmoji(false) + " | api | 31 Dec 2026 |\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'project.service_account_pat_revoke' to revoke one of these tokens\n"
		if got := FormatListPATMarkdownString(ListPATOutput{Tokens: []PATOutput{token}}); got != want {
			t.Errorf("FormatListPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("empty token list", func(t *testing.T) {
		if got, want := FormatListPATMarkdownString(ListPATOutput{}), "No project service account tokens found.\n"; got != want {
			t.Errorf("FormatListPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("granular token", func(t *testing.T) {
		granular := PATOutput{
			ID: 12, Name: "gran", Active: true, Granular: true, Scopes: []string{"api"},
			GranularScopes: []toolutil.TokenGranularScopeOutput{
				{Access: "read", Permissions: []string{"read_code", "read_runners"}, ProjectID: 3},
				{Access: "admin", Permissions: []string{"admin_runners"}, GroupID: 9},
			},
		}
		want := "## Project Service Account Token: gran\n\n" +
			"- **ID**: 12\n" +
			"- **Name**: gran\n" +
			"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
			"- **Scopes**: api\n" +
			"- **Granular**: " + toolutil.BoolEmoji(true) + "\n" +
			"- **User ID**: 0\n" +
			"\n### Granular Scopes\n\n" +
			"| Access | Permissions | Project | Group |\n| --- | --- | --- | --- |\n" +
			"| read | read_code, read_runners | 3 | - |\n" +
			"| admin | admin_runners | - | 9 |\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'project.service_account_pat_rotate' to rotate this token\n" +
			"- Use action 'project.service_account_pat_revoke' to revoke it\n"
		if got := FormatPATMarkdownString(granular); got != want {
			t.Errorf("FormatPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestContextCancellation verifies handlers return before making API calls when
// the caller's context is already canceled.
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }))
	for _, tt := range []struct {
		name string
		call func() error
	}{
		{name: "list", call: func() error { _, err := List(ctx, client, ListInput{ProjectID: "42"}); return err }},
		{name: "create", call: func() error { _, err := Create(ctx, client, CreateInput{ProjectID: "42"}); return err }},
		{name: "update", call: func() error {
			_, err := Update(ctx, client, UpdateInput{ProjectID: "42", ServiceAccountID: 7})
			return err
		}},
		{name: "delete", call: func() error { return Delete(ctx, client, DeleteInput{ProjectID: "42", ServiceAccountID: 7}) }},
		{name: "list PATs", call: func() error {
			_, err := ListPATs(ctx, client, ListPATInput{ProjectID: "42", ServiceAccountID: 7})
			return err
		}},
		{name: "create PAT", call: func() error {
			_, err := CreatePAT(ctx, client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}})
			return err
		}},
		{name: "revoke PAT", call: func() error {
			return RevokePAT(ctx, client, RevokePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11})
		}},
		{name: "rotate PAT", call: func() error {
			_, err := RotatePAT(ctx, client, RotatePATInput{ProjectID: "42", ServiceAccountID: 7, TokenID: 11})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.call(), toolutil.ErrMsgContextCanceled)
		})
	}
}

func specsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		if spec.OwnerPackage != "projectserviceaccounts" {
			t.Fatalf("OwnerPackage for %s = %q, want projectserviceaccounts", spec.Name, spec.OwnerPackage)
		}
		if spec.Edition != "" {
			t.Fatalf("Edition for %s = %q, want \"\" (Free)", spec.Name, spec.Edition)
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

// TestProjectServiceAccounts_UnreadableCapturedPublicEmail verifies that every
// service account handler returns an error rather than a half-filled account
// when GitLab sends public_email as something that is not a string. The SDK
// ignores the key its own ServiceAccount does not model, so the read of the
// captured response is the only thing that can notice.
func TestProjectServiceAccounts_UnreadableCapturedPublicEmail(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		call func(context.Context, *gitlabclient.Client) error
	}{
		{"list", `[{"id":1,"username":"svc","name":"Service","public_email":42}]`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := List(ctx, c, ListInput{ProjectID: "42"})
			return err
		}},
		{"create", `{"id":1,"username":"svc","name":"Service","public_email":42}`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Create(ctx, c, CreateInput{ProjectID: "42", Name: "Service"})
			return err
		}},
		{"update", `{"id":1,"username":"svc","name":"Service","public_email":42}`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Update(ctx, c, UpdateInput{ProjectID: "42", ServiceAccountID: 1, Name: "Renamed"})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tt.body)
			}))
			if err := tt.call(context.Background(), client); err == nil {
				t.Fatal("error = nil, want the captured decode to fail")
			}
		})
	}
}

// TestAccountOutput_EachFieldComesFromItsOwnSource drives one account through
// the handler with six values that are all different from one another, so that
// a field filled from a neighbour's source is a failure rather than a
// coincidence. Nothing else here would notice: a converter is straight-line
// assignment, which no branch coverage scores, and the fixtures that agree on
// two values make two fields indistinguishable.
func TestAccountOutput_EachFieldComesFromItsOwnSource(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":7,"name":"Service Account","username":"svc-user",`+
			`"email":"svc@example.com","unconfirmed_email":"pending@example.com","public_email":"public@example.com"}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "Service Account"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	for _, tt := range []struct {
		field     string
		got, want any
	}{
		{"id", out.ID, int64(7)},
		{"name", out.Name, "Service Account"},
		{"username", out.Username, "svc-user"},
		{"email", out.Email, "svc@example.com"},
		{"unconfirmed_email", out.UnconfirmedEmail, "pending@example.com"},
		{"public_email", out.PublicEmail, "public@example.com"},
	} {
		t.Run(tt.field, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %#v, want %#v", tt.field, tt.got, tt.want)
			}
		})
	}
}

// TestPATOutput_EachFieldComesFromItsOwnSource does the same for a token, whose
// converter has the more confusable pairs: two booleans that a revoked token
// sets opposite ways, two identifiers, two names and three timestamps. The
// fixture gives each one a value no other field carries.
func TestPATOutput_EachFieldComesFromItsOwnSource(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":11,"name":"tok","description":"deploy token",`+
			`"revoked":true,"active":false,"user_id":7,"scopes":["api"],"token":"glpat-test",`+
			`"created_at":"2026-01-01T02:03:04Z","last_used_at":"2026-01-02T03:04:05Z","expires_at":"2026-12-31",`+
			`"granular":true,"last_used_ips":["192.0.2.10"]}`)
	}))

	out, err := CreatePAT(context.Background(), client, CreatePATInput{ProjectID: "42", ServiceAccountID: 7, Name: "tok", Scopes: []string{"api"}})
	if err != nil {
		t.Fatalf("CreatePAT() unexpected error: %v", err)
	}
	for _, tt := range []struct {
		field     string
		got, want any
	}{
		{"id", out.ID, int64(11)},
		{"name", out.Name, "tok"},
		{"description", out.Description, "deploy token"},
		{"revoked", out.Revoked, true},
		{"active", out.Active, false},
		{"user_id", out.UserID, int64(7)},
		{"token", out.Token, "glpat-test"},
		{"created_at", out.CreatedAt, "2026-01-01T02:03:04Z"},
		{"last_used_at", out.LastUsedAt, "2026-01-02T03:04:05Z"},
		{"expires_at", out.ExpiresAt, "2026-12-31"},
		{"granular", out.Granular, true},
		{"scopes", strings.Join(out.Scopes, ","), "api"},
		{"last_used_ips", strings.Join(out.LastUsedIPs, ","), "192.0.2.10"},
	} {
		t.Run(tt.field, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %#v, want %#v", tt.field, tt.got, tt.want)
			}
		})
	}
}

// TestListHandlers_PublishTheServersOwnPageMarkers verifies both list handlers
// fill their pagination block from the response GitLab answered with. A caller
// reading a page whose markers stayed zero cannot tell one page from the first
// of twelve, and the block is the only thing that says which it is.
func TestListHandlers_PublishTheServersOwnPageMarkers(t *testing.T) {
	headers := testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "12", TotalPages: "3", NextPage: "3", PrevPage: "1"}
	want := toolutil.PaginationOutput{Page: 2, PerPage: 5, TotalItems: 12, TotalPages: 3, NextPage: 3, PrevPage: 1, HasMore: true}

	t.Run("accounts", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSONWithPagination(w, http.StatusOK, projectServiceAccountsJSON, headers)
		}))
		out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
		if err != nil {
			t.Fatalf("List() unexpected error: %v", err)
		}
		if out.Pagination != want {
			t.Errorf("List() pagination = %#v, want %#v", out.Pagination, want)
		}
	})

	t.Run("tokens", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSONWithPagination(w, http.StatusOK, projectServiceAccountPATsJSON, headers)
		}))
		out, err := ListPATs(context.Background(), client, ListPATInput{ProjectID: "42", ServiceAccountID: 7})
		if err != nil {
			t.Fatalf("ListPATs() unexpected error: %v", err)
		}
		if out.Pagination != want {
			t.Errorf("ListPATs() pagination = %#v, want %#v", out.Pagination, want)
		}
	})
}

// decodeRequestBody reads the JSON body a handler built for GitLab.
//
// It decodes into a map rather than a struct because these tests assert as much
// about a key being absent as about its value: the SDK's option structs are
// pointers with omitempty, so "the caller left this unset" and "the caller set
// it empty" differ only by whether the key reaches the wire. An empty body is
// an options struct with nothing set, which is a shape these tests assert too.
func decodeRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("decode request body: %v", err)
	}
	return body
}

// assertSent holds one key of a request body to the value the caller gave for
// it. It runs on the mock's own goroutine, so it reports and never aborts.
func assertSent(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	if got := body[key]; got != want {
		t.Errorf("%s sent = %#v, want %#v", key, got, want)
	}
}

// assertSentList holds one key of a request body to a list of values in order,
// which is what a token's scopes are.
func assertSentList(t *testing.T, body map[string]any, key string, want ...string) {
	t.Helper()
	got, _ := body[key].([]any)
	if len(got) != len(want) {
		t.Errorf("%s sent = %#v, want %v", key, body[key], want)
		return
	}
	for i, value := range want {
		if got[i] != value {
			t.Errorf("%s[%d] sent = %#v, want %q", key, i, got[i], value)
		}
	}
}

// assertNotSent holds a key off the wire entirely, which is the half of an
// optional field that a value assertion cannot state.
func assertNotSent(t *testing.T, body map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if got, ok := body[key]; ok {
			t.Errorf("%s sent = %#v, want the key absent", key, got)
		}
	}
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
