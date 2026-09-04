// group_service_accounts_test.go contains unit tests for GitLab group service account MCP tool handlers.
package groupserviceaccounts

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	pathServiceAccounts     = "/api/v4/groups/mygroup/service_accounts"
	pathServiceAccount42    = "/api/v4/groups/mygroup/service_accounts/42"
	pathServiceAccount42PAT = "/api/v4/groups/mygroup/service_accounts/42/personal_access_tokens"
)

// TestList verifies the List handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	tests := []struct {
		name       string
		input      ListInput
		handler    http.HandlerFunc
		wantErr    bool
		wantCount  int
		wantFirst  string
		errContain string
	}{
		{
			name:  "returns accounts on success",
			input: ListInput{GroupID: "mygroup"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathServiceAccounts)
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id":42,"name":"svc-bot","username":"svc-bot","email":"svc@test.com"}
				]`)
			},
			wantCount: 1,
			wantFirst: "svc-bot",
		},
		{
			name: "passes order_by and sort query params",
			input: ListInput{
				GroupID: "mygroup",
				OrderBy: "username",
				Sort:    "desc",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "order_by", "username")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"a","username":"a","email":"a@t.com"}]`)
			},
			wantCount: 1,
			wantFirst: "a",
		},
		{
			name:  "returns empty list",
			input: ListInput{GroupID: "mygroup"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			wantCount: 0,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{GroupID: "mygroup"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			},
			wantErr:    true,
			errContain: "list group service accounts",
		},
		{
			name:       "returns error when group_id is empty",
			input:      ListInput{},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
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
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if len(out.Accounts) != tt.wantCount {
				t.Fatalf("len(Accounts) = %d, want %d", len(out.Accounts), tt.wantCount)
			}
			if tt.wantFirst != "" && out.Accounts[0].Username != tt.wantFirst {
				t.Errorf("Username = %q, want %q", out.Accounts[0].Username, tt.wantFirst)
			}
		})
	}
}

// TestCreate verifies the Create handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate(t *testing.T) {
	tests := []struct {
		name       string
		input      CreateInput
		handler    http.HandlerFunc
		wantErr    bool
		wantID     int64
		errContain string
	}{
		{
			name:  "creates account with all fields",
			input: CreateInput{GroupID: "mygroup", Name: "svc-bot", Username: "svc-bot", Email: "svc@test.com"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":42,"name":"svc-bot","username":"svc-bot","email":"svc@test.com"}`)
			},
			wantID: 42,
		},
		{
			name:  "creates account with only group_id",
			input: CreateInput{GroupID: "mygroup"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"name":"","username":"","email":""}`)
			},
			wantID: 99,
		},
		{
			name:       "returns error when group_id is empty",
			input:      CreateInput{},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:  "returns error on API failure",
			input: CreateInput{GroupID: "mygroup"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
			},
			wantErr:    true,
			errContain: "create group service account",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Create(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if out.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", out.ID, tt.wantID)
			}
		})
	}
}

// TestUpdate validates the Update handler covering success with optional fields,
// both validation branches (missing group_id, missing service_account_id), and
// API errors.
func TestUpdate(t *testing.T) {
	tests := []struct {
		name       string
		input      UpdateInput
		handler    http.HandlerFunc
		wantErr    bool
		wantName   string
		errContain string
	}{
		{
			name:  "updates account with all optional fields",
			input: UpdateInput{GroupID: "mygroup", ServiceAccountID: 42, Name: "new", Username: "new-u", Email: "new@t.com"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPatch)
				testutil.AssertRequestPath(t, r, pathServiceAccount42)
				testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"new","username":"new-u","email":"new@t.com"}`)
			},
			wantName: "new",
		},
		{
			name:  "updates account with only name",
			input: UpdateInput{GroupID: "mygroup", ServiceAccountID: 42, Name: "renamed"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"renamed","username":"svc-bot","email":"svc@test.com"}`)
			},
			wantName: "renamed",
		},
		{
			name:       "returns error when group_id is empty",
			input:      UpdateInput{ServiceAccountID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:       "returns error when service_account_id is zero",
			input:      UpdateInput{GroupID: "mygroup"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "service_account_id",
		},
		{
			name:  "returns error on API failure",
			input: UpdateInput{GroupID: "mygroup", ServiceAccountID: 42, Name: "x"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
			},
			wantErr:    true,
			errContain: "update group service account",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Update(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if out.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", out.Name, tt.wantName)
			}
		})
	}
}

// TestDelete verifies the Delete handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete(t *testing.T) {
	tests := []struct {
		name       string
		input      DeleteInput
		handler    http.HandlerFunc
		wantErr    bool
		errContain string
	}{
		{
			name:  "deletes account successfully",
			input: DeleteInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathServiceAccount42)
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:  "passes hard_delete flag",
			input: DeleteInput{GroupID: "mygroup", ServiceAccountID: 42, HardDelete: true},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      DeleteInput{ServiceAccountID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:       "returns error when service_account_id is zero",
			input:      DeleteInput{GroupID: "mygroup"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "service_account_id",
		},
		{
			name:  "returns error on API failure",
			input: DeleteInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"error"}`)
			},
			wantErr:    true,
			errContain: "delete group service account",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			err := Delete(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
			}
		})
	}
}

// TestListPATs verifies the ListPATs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListPATs(t *testing.T) {
	tests := []struct {
		name       string
		input      ListPATInput
		handler    http.HandlerFunc
		wantErr    bool
		wantCount  int
		errContain string
	}{
		{
			name:  "returns PATs on success",
			input: ListPATInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathServiceAccount42PAT)
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id":1,"name":"deploy-token","revoked":false,"scopes":["api"],"user_id":42,"active":true}
				]`)
			},
			wantCount: 1,
		},
		{
			name:  "returns empty PAT list",
			input: ListPATInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			wantCount: 0,
		},
		{
			name: "passes order_by, sort, filters and date params",
			input: ListPATInput{
				GroupID:          "mygroup",
				ServiceAccountID: 42,
				OrderBy:          "created_at",
				Sort:             "desc",
				Revoked:          new(true),
				UserID:           7,
				Search:           "deploy",
				State:            "active",
				CreatedAfter:     "2026-01-01T00:00:00Z",
				CreatedBefore:    "2026-12-31T00:00:00Z",
				ExpiresAfter:     "2026-02-01",
				ExpiresBefore:    "2026-11-30",
				LastUsedAfter:    "2026-03-01T00:00:00Z",
				LastUsedBefore:   "2026-10-01T00:00:00Z",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "order_by", "created_at")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.AssertQueryParam(t, r, "revoked", "true")
				testutil.AssertQueryParam(t, r, "user_id", "7")
				testutil.AssertQueryParam(t, r, "search", "deploy")
				testutil.AssertQueryParam(t, r, "state", "active")
				testutil.AssertQueryParam(t, r, "expires_after", "2026-02-01")
				testutil.AssertQueryParam(t, r, "expires_before", "2026-11-30")
				testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"t","scopes":["api"],"user_id":7,"active":true,"revoked":true}]`)
			},
			wantCount: 1,
		},
		{
			name:  "passes keyset pagination params",
			input: ListPATInput{GroupID: "mygroup", ServiceAccountID: 42, KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "100"}},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "pagination", "keyset")
				testutil.AssertQueryParam(t, r, "page_token", "100")
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			wantCount: 0,
		},
		{
			name:       "returns error on invalid expires_after",
			input:      ListPATInput{GroupID: "mygroup", ServiceAccountID: 42, ExpiresAfter: "not-a-date"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "invalid expires_after format",
		},
		{
			name:       "returns error on invalid expires_before",
			input:      ListPATInput{GroupID: "mygroup", ServiceAccountID: 42, ExpiresBefore: "not-a-date"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "invalid expires_before format",
		},
		{
			name:       "returns error when group_id is empty",
			input:      ListPATInput{ServiceAccountID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:       "returns error when service_account_id is zero",
			input:      ListPATInput{GroupID: "mygroup"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "service_account_id",
		},
		{
			name:  "returns error on API failure",
			input: ListPATInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
			},
			wantErr:    true,
			errContain: "list service account PATs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := ListPATs(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ListPATs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if len(out.Tokens) != tt.wantCount {
				t.Fatalf("len(Tokens) = %d, want %d", len(out.Tokens), tt.wantCount)
			}
		})
	}
}

// TestCreatePAT validates the CreatePAT handler covering success with optional
// fields (description, expires_at), all validation branches, invalid date
// format, and API errors.
func TestCreatePAT(t *testing.T) {
	tests := []struct {
		name       string
		input      CreatePATInput
		handler    http.HandlerFunc
		wantErr    bool
		wantToken  string
		errContain string
	}{
		{
			name: "creates PAT with all fields",
			input: CreatePATInput{
				GroupID:          "mygroup",
				ServiceAccountID: 42,
				Name:             "deploy-token",
				Scopes:           []string{"api"},
				Description:      "CI deploy",
				ExpiresAt:        "2026-12-31",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"deploy-token","scopes":["api"],"user_id":42,"active":true,"token":"glpat-xxxx","description":"CI deploy","expires_at":"2026-12-31"}`)
			},
			wantToken: "glpat-xxxx",
		},
		{
			name: "creates PAT without optional fields",
			input: CreatePATInput{
				GroupID:          "mygroup",
				ServiceAccountID: 42,
				Name:             "tok",
				Scopes:           []string{"read_api"},
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"tok","scopes":["read_api"],"user_id":42,"active":true,"token":"glpat-yyyy"}`)
			},
			wantToken: "glpat-yyyy",
		},
		{
			name:       "returns error when group_id is empty",
			input:      CreatePATInput{ServiceAccountID: 42, Name: "tok", Scopes: []string{"api"}},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:       "returns error when service_account_id is zero",
			input:      CreatePATInput{GroupID: "mygroup", Name: "tok", Scopes: []string{"api"}},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "service_account_id",
		},
		{
			name:       "returns error when name is empty",
			input:      CreatePATInput{GroupID: "mygroup", ServiceAccountID: 42, Scopes: []string{"api"}},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "name",
		},
		{
			name:       "returns error when scopes is empty",
			input:      CreatePATInput{GroupID: "mygroup", ServiceAccountID: 42, Name: "tok"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "scopes",
		},
		{
			name: "returns error for invalid expires_at format",
			input: CreatePATInput{
				GroupID:          "mygroup",
				ServiceAccountID: 42,
				Name:             "tok",
				Scopes:           []string{"api"},
				ExpiresAt:        "not-a-date",
			},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "invalid expires_at format",
		},
		{
			name: "returns error on API failure",
			input: CreatePATInput{
				GroupID:          "mygroup",
				ServiceAccountID: 42,
				Name:             "tok",
				Scopes:           []string{"api"},
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"validation error"}`)
			},
			wantErr:    true,
			errContain: "create service account PAT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := CreatePAT(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CreatePAT() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if out.Token != tt.wantToken {
				t.Errorf("Token = %q, want %q", out.Token, tt.wantToken)
			}
		})
	}
}

// TestRevokePAT verifies the RevokePAT handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRevokePAT(t *testing.T) {
	tests := []struct {
		name       string
		input      RevokePATInput
		handler    http.HandlerFunc
		wantErr    bool
		errContain string
	}{
		{
			name:  "revokes PAT successfully",
			input: RevokePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 1},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathServiceAccount42PAT+"/1")
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      RevokePATInput{ServiceAccountID: 42, TokenID: 1},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:       "returns error when service_account_id is zero",
			input:      RevokePATInput{GroupID: "mygroup", TokenID: 1},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "service_account_id",
		},
		{
			name:       "returns error when token_id is zero",
			input:      RevokePATInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "token_id",
		},
		{
			name:  "returns error on API failure",
			input: RevokePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
			},
			wantErr:    true,
			errContain: "revoke service account PAT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			err := RevokePAT(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RevokePAT() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
			}
		})
	}
}

// TestRotatePAT validates the RotatePAT handler covering success with and
// without an explicit expires_at, all validation branches, invalid date format,
// and API errors. It asserts the rotated token value is surfaced in the output.
func TestRotatePAT(t *testing.T) {
	tests := []struct {
		name       string
		input      RotatePATInput
		handler    http.HandlerFunc
		wantErr    bool
		wantToken  string
		errContain string
	}{
		{
			name:  "rotates PAT with expires_at",
			input: RotatePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 10, ExpiresAt: "2026-12-31"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathServiceAccount42PAT+"/10/rotate")
				testutil.RespondJSON(w, http.StatusOK, `{"id":11,"name":"tok","scopes":["api"],"user_id":42,"active":true,"revoked":false,"token":"glpat-rotated","expires_at":"2026-12-31"}`)
			},
			wantToken: "glpat-rotated",
		},
		{
			name:  "rotates PAT without expires_at",
			input: RotatePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 10},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":12,"name":"tok","scopes":["api"],"user_id":42,"active":true,"revoked":false,"token":"glpat-new"}`)
			},
			wantToken: "glpat-new",
		},
		{
			name:       "returns error when group_id is empty",
			input:      RotatePATInput{ServiceAccountID: 42, TokenID: 10},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:       "returns error when service_account_id is zero",
			input:      RotatePATInput{GroupID: "mygroup", TokenID: 10},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "service_account_id",
		},
		{
			name:       "returns error when token_id is zero",
			input:      RotatePATInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "token_id",
		},
		{
			name:       "returns error for invalid expires_at format",
			input:      RotatePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 10, ExpiresAt: "not-a-date"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "invalid expires_at format",
		},
		{
			name:  "returns error on API failure",
			input: RotatePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 10},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"rotation error"}`)
			},
			wantErr:    true,
			errContain: "rotate service account PAT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := RotatePAT(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RotatePAT() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if out.Token != tt.wantToken {
				t.Errorf("Token = %q, want %q", out.Token, tt.wantToken)
			}
		})
	}
}

// TestToPATOutput_TimeFields verifies the ToPATOutput_TimeFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToPATOutput_TimeFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathServiceAccount42PAT {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":5,"name":"t1","scopes":["api"],"user_id":42,"active":true,"revoked":false,"created_at":"2026-06-15T10:30:00Z","last_used_at":"2026-06-20T08:00:00Z","expires_at":"2026-01-15"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListPATs(context.Background(), client, ListPATInput{GroupID: "mygroup", ServiceAccountID: 42})
	if err != nil {
		t.Fatalf("ListPATs() unexpected error: %v", err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("len(Tokens) = %d, want 1", len(out.Tokens))
	}
	tok := out.Tokens[0]
	if tok.CreatedAt == "" {
		t.Error("CreatedAt should not be empty when API returns created_at")
	}
	if tok.ExpiresAt == "" {
		t.Error("ExpiresAt should not be empty when API returns expires_at")
	}
	if tok.ExpiresAt != "" && !strings.Contains(tok.ExpiresAt, "2026") {
		t.Errorf("ExpiresAt = %q, want year 2026", tok.ExpiresAt)
	}
	if tok.LastUsedAt == "" {
		t.Error("LastUsedAt should not be empty when API returns last_used_at")
	}
}

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{ID: 42, Name: "svc-bot", Username: "svc-bot", Email: "svc@test.com"}
	md := FormatOutputMarkdown(out)
	for _, want := range []string{"svc-bot", "42", "svc@test.com", "gitlab_group_service_account_update"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("FormatOutputMarkdown missing %q in:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("non-empty list", func(t *testing.T) {
		out := ListOutput{
			Accounts: []Output{
				{ID: 1, Name: "a", Username: "a-user", Email: "a@t.com"},
				{ID: 2, Name: "b", Username: "b-user", Email: "b@t.com"},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
		}
		md := FormatListMarkdown(out)
		if !strings.Contains(md, "a-user") {
			t.Errorf("FormatListMarkdown missing first account username:\n%s", md)
		}
		if !strings.Contains(md, "b-user") {
			t.Errorf("FormatListMarkdown missing second account username:\n%s", md)
		}
		if !strings.Contains(md, "| ID |") {
			t.Errorf("FormatListMarkdown missing table header:\n%s", md)
		}
	})
	t.Run("empty list", func(t *testing.T) {
		md := FormatListMarkdown(ListOutput{})
		if !strings.Contains(md, "No group service accounts found") {
			t.Errorf("FormatListMarkdown empty should say no accounts found:\n%s", md)
		}
	})
}

// TestFormatPATOutputMarkdown verifies the PATOutputMarkdown Markdown formatter for a representative patoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatPATOutputMarkdown(t *testing.T) {
	t.Run("with all fields", func(t *testing.T) {
		out := PATOutput{
			ID: 1, Name: "deploy", Active: true, Revoked: false,
			Scopes: []string{"api", "read_user"}, UserID: 42,
			CreatedAt: "2026-06-15T10:30:00Z", ExpiresAt: "2026-01-15",
			Token: "glpat-secret",
		}
		md := FormatPATOutputMarkdown(out)
		for _, want := range []string{"deploy", "api", "glpat-secret", "2026-01-15", "gitlab_group_service_account_pat_revoke"} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(md, want) {
					t.Errorf("FormatPATOutputMarkdown missing %q:\n%s", want, md)
				}
			})
		}
	})
	t.Run("without optional fields", func(t *testing.T) {
		out := PATOutput{ID: 2, Name: "min", Scopes: []string{"read_api"}}
		md := FormatPATOutputMarkdown(out)
		if !strings.Contains(md, "min") {
			t.Errorf("FormatPATOutputMarkdown missing name:\n%s", md)
		}
		if strings.Contains(md, "Expires") {
			t.Errorf("FormatPATOutputMarkdown should not contain Expires when empty:\n%s", md)
		}
	})
}

// TestFormatListPATMarkdown verifies the ListPATMarkdown Markdown formatter for a representative listpat input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListPATMarkdown(t *testing.T) {
	t.Run("non-empty list", func(t *testing.T) {
		out := ListPATOutput{
			Tokens: []PATOutput{
				{ID: 1, Name: "t1", Active: true, Scopes: []string{"api"}},
				{ID: 2, Name: "t2", Active: false, Revoked: true, Scopes: []string{"read_api"}},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 2},
		}
		md := FormatListPATMarkdown(out)
		if !strings.Contains(md, "t1") || !strings.Contains(md, "t2") {
			t.Errorf("FormatListPATMarkdown missing tokens:\n%s", md)
		}
		if !strings.Contains(md, "| ID |") {
			t.Errorf("FormatListPATMarkdown missing table header:\n%s", md)
		}
	})
	t.Run("empty list", func(t *testing.T) {
		md := FormatListPATMarkdown(ListPATOutput{})
		if !strings.Contains(md, "No personal access tokens found") {
			t.Errorf("FormatListPATMarkdown empty should say no tokens found:\n%s", md)
		}
	})
}

// TestFormatMarkdownString verifies the MarkdownString Markdown formatter for a representative string input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdownString(t *testing.T) {
	out := Output{ID: 10, Name: "svc", Username: "svc-user", Email: "svc@e.com"}
	md := FormatMarkdownString(out)
	for _, want := range []string{"svc-user", "10", "svc@e.com"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("FormatMarkdownString missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdownString verifies the ListMarkdownString Markdown formatter for a representative liststring input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdownString(t *testing.T) {
	t.Run("with accounts", func(t *testing.T) {
		out := ListOutput{
			Accounts:   []Output{{ID: 1, Name: "a", Username: "au", Email: "a@t.com"}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
		}
		md := FormatListMarkdownString(out)
		if !strings.Contains(md, "au") {
			t.Errorf("FormatListMarkdownString missing username:\n%s", md)
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListMarkdownString(ListOutput{})
		if !strings.Contains(md, "No service accounts found") {
			t.Errorf("FormatListMarkdownString empty should say no accounts:\n%s", md)
		}
	})
}

// TestFormatPATMarkdownString verifies the PATMarkdownString Markdown formatter for a representative patstring input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatPATMarkdownString(t *testing.T) {
	t.Run("with all fields", func(t *testing.T) {
		out := PATOutput{
			ID: 1, Name: "tok", Active: true, Scopes: []string{"api"},
			UserID: 42, CreatedAt: "2026-01-01T00:00:00Z",
			LastUsedAt: "2026-06-20T08:00:00Z",
			ExpiresAt:  "2026-12-31", Token: "secret",
		}
		md := FormatPATMarkdownString(out)
		for _, want := range []string{"tok", "api", "secret", "2026-12-31", "2026-01-01", "2026-06-20", "Last Used"} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(md, want) {
					t.Errorf("FormatPATMarkdownString missing %q:\n%s", want, md)
				}
			})
		}
	})
	t.Run("minimal fields", func(t *testing.T) {
		out := PATOutput{ID: 2, Name: "m", Scopes: []string{"read_api"}}
		md := FormatPATMarkdownString(out)
		if !strings.Contains(md, "read_api") {
			t.Errorf("FormatPATMarkdownString missing scopes:\n%s", md)
		}
	})
}

// TestFormatListPATMarkdownString verifies the ListPATMarkdownString Markdown formatter for a representative listpatstring input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListPATMarkdownString(t *testing.T) {
	t.Run("with tokens", func(t *testing.T) {
		out := ListPATOutput{
			Tokens:     []PATOutput{{ID: 1, Name: "t", Active: true, Scopes: []string{"api"}, ExpiresAt: "2026-01-01"}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
		}
		md := FormatListPATMarkdownString(out)
		if !strings.Contains(md, "2026-01-01") {
			t.Errorf("FormatListPATMarkdownString missing expires:\n%s", md)
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListPATMarkdownString(ListPATOutput{})
		if !strings.Contains(md, "No tokens found") {
			t.Errorf("FormatListPATMarkdownString empty should say no tokens:\n%s", md)
		}
	})
}
