// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// group_service_accounts_test.go contains unit tests for GitLab group service account MCP tool handlers.
package groupserviceaccounts

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	pathServiceAccounts     = "/api/v4/groups/mygroup/service_accounts"
	pathServiceAccount42    = "/api/v4/groups/mygroup/service_accounts/42"
	pathServiceAccount42PAT = "/api/v4/groups/mygroup/service_accounts/42/personal_access_tokens"
)

// TestListPATs_ReadsWhatTheSDKDoesNotModel verifies a service account's
// tokens carry, beside what client-go decoded, the three fields
// lib/api/entities/personal_access_token.rb sends and gl.PersonalAccessToken
// does not, each paired with its token by position.
func TestListPATs_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"a","granular":true,"last_used_ips":["192.0.2.10"],`+
			`"granular_scopes":[{"access":"group","permissions":["read_job"],"group_id":5}]},{"id":2,"name":"b"}]`)
	}))

	out, err := ListPATs(context.Background(), client, ListPATInput{GroupID: "mygroup", ServiceAccountID: 42})
	if err != nil {
		t.Fatalf("ListPATs() unexpected error: %v", err)
	}
	if len(out.Tokens) != 2 || !out.Tokens[0].Granular || len(out.Tokens[0].GranularScopes) != 1 ||
		out.Tokens[0].GranularScopes[0].GroupID != 5 || len(out.Tokens[0].LastUsedIPs) != 1 ||
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
		body := `{"id":1,"name":"a","granular":"not-a-bool"}`
		if r.Method == http.MethodGet {
			body = "[" + body + "]"
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			_, err := ListPATs(context.Background(), client, ListPATInput{GroupID: "mygroup", ServiceAccountID: 42})
			return err
		}},
		{Name: "create", Call: func() error {
			_, err := CreatePAT(context.Background(), client, CreatePATInput{GroupID: "mygroup", ServiceAccountID: 42, Name: "a", Scopes: []string{"api"}})
			return err
		}},
		{Name: "rotate", Call: func() error {
			_, err := RotatePAT(context.Background(), client, RotatePATInput{GroupID: "mygroup", ServiceAccountID: 42, TokenID: 1})
			return err
		}},
	})
}

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

// TestGet verifies the Get handler: it asks GET
// /groups/:id/service_accounts/:user_id for the account the caller named and
// publishes every field GitLab's service account entity sends, refuses a call
// missing either identifier before reaching GitLab, and says what a 400 means
// on this route, where GitLab answers one for a user of the group who is not a
// service account. Any other refusal is reported with GitLab's own message.
func TestGet(t *testing.T) {
	tests := []struct {
		name       string
		input      GetInput
		handler    http.HandlerFunc
		want       Output
		errContain []string
		errAbsent  string
	}{
		{
			name:  "returns every field GitLab sent",
			input: GetInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathServiceAccount42)
				testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"svc-bot","username":"svc-bot-user",`+
					`"email":"svc@test.com","public_email":"pub@test.com","unconfirmed_email":"new@test.com"}`)
			},
			want: Output{
				ID: 42, Name: "svc-bot", Username: "svc-bot-user", Email: "svc@test.com",
				PublicEmail: "pub@test.com", UnconfirmedEmail: "new@test.com",
			},
		},
		{
			name:       "refuses an empty group_id",
			input:      GetInput{ServiceAccountID: 42},
			handler:    testutil.ForbiddenHandler(t).ServeHTTP,
			errContain: []string{"group_id"},
		},
		{
			name:       "refuses a zero service_account_id",
			input:      GetInput{GroupID: "mygroup"},
			handler:    testutil.ForbiddenHandler(t).ServeHTTP,
			errContain: []string{"service_account_id"},
		},
		{
			name:  "a 400 says the user is not a service account",
			input: GetInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad request - User is not of type Service Account"}`)
			},
			errContain: []string{"get group service account", "not a service account", "group.service_account_list"},
		},
		{
			name:  "another refusal carries GitLab's message and no such hint",
			input: GetInput{GroupID: "mygroup", ServiceAccountID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			errContain: []string{"get group service account", "403 Forbidden"},
			errAbsent:  "not a service account",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Get(context.Background(), client, tt.input)
			if len(tt.errContain) == 0 {
				if err != nil {
					t.Fatalf("Get() unexpected error: %v", err)
				}
				if !reflect.DeepEqual(out, tt.want) {
					t.Errorf("Get() = %+v, want %+v", out, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("Get() = %+v, want an error", out)
			}
			assertGetError(t, err, tt.errContain, tt.errAbsent)
		})
	}
}

// assertGetError checks a Get refusal names every fragment in contain and,
// when absent is set, does not carry that hint.
func assertGetError(t *testing.T, err error, contain []string, absent string) {
	t.Helper()
	for _, want := range contain {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Get() error %q does not contain %q", err, want)
		}
	}
	if absent != "" && strings.Contains(err.Error(), absent) {
		t.Errorf("Get() error %q carries %q, a hint for a status GitLab did not answer", err, absent)
	}
}

// TestFormatServiceAccountNotFound verifies the not-found answer the get
// route gives for a 404, read back through the Markdown registry so the
// registration is held too: an error result naming the account and the group,
// pointing at the listing, and saying that an instance older than GitLab 19.4
// answers 404 for every ID because it does not mount the route.
func TestFormatServiceAccountNotFound(t *testing.T) {
	result := toolutil.MarkdownForResult(serviceAccountNotFoundOutput{Identifier: "ID 42 in group mygroup"})
	if result == nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("MarkdownForResult(not found) = %#v, want one error content block", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T, want *mcp.TextContent", result.Content[0])
	}
	for _, want := range []string{
		"Group Service Account Not Found",
		"ID 42 in group mygroup",
		"Use group.service_account_list with group_id to find the account's ID",
		"GitLab mounts this route from 19.4",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text.Text, want) {
				t.Errorf("not-found card does not contain %q:\n%s", want, text.Text)
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
			name: "passes sort, filters and date params",
			input: ListPATInput{
				GroupID:          "mygroup",
				ServiceAccountID: 42,
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
			name:  "passes offset pagination params",
			input: ListPATInput{GroupID: "mygroup", ServiceAccountID: 42, PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 50}},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "50")
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			wantCount: 0,
		},
		{
			name:       "returns error on invalid expires_after",
			input:      ListPATInput{GroupID: "mygroup", ServiceAccountID: 42, ExpiresAfter: "not-a-date"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "expires_after must be a date",
		},
		{
			name:       "returns error on invalid expires_before",
			input:      ListPATInput{GroupID: "mygroup", ServiceAccountID: 42, ExpiresBefore: "not-a-date"},
			handler:    func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			wantErr:    true,
			errContain: "expires_before must be a date",
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
			errContain: "expires_at must be a date",
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
			errContain: "expires_at must be a date",
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

// TestFormatMarkdownString verifies the whole card a service account renders:
// the H2 the formatter composes, one list item per field GitLab sent, no item
// for a field it did not, and the guidance section last.
func TestFormatMarkdownString(t *testing.T) {
	t.Run("every field", func(t *testing.T) {
		out := Output{ID: 10, Name: "svc", Username: "svc-user", Email: "svc@e.com", PublicEmail: "pub@e.com", UnconfirmedEmail: "new@e.com"}
		want := "## Service Account: svc-user\n\n" +
			"- **ID**: 10\n" +
			"- **Name**: svc\n" +
			"- **Username**: svc-user\n" +
			"- **Email**: svc@e.com\n" +
			"- **Public Email**: pub@e.com\n" +
			"- **Unconfirmed Email**: new@e.com\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'group.service_account_update' to change this account's name or username\n" +
			"- Use action 'group.service_account_pat_create' to create a token for it\n"
		if got := FormatMarkdownString(out); got != want {
			t.Errorf("FormatMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("only what GitLab sent", func(t *testing.T) {
		out := Output{ID: 10, Username: "svc-user"}
		want := "## Service Account: svc-user\n\n" +
			"- **ID**: 10\n" +
			"- **Username**: svc-user\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'group.service_account_update' to change this account's name or username\n" +
			"- Use action 'group.service_account_pat_create' to create a token for it\n"
		if got := FormatMarkdownString(out); got != want {
			t.Errorf("FormatMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestFormatListMarkdownString verifies the whole list a page of service
// accounts renders: the heading counting what GitLab reported rather than the
// page length, the table, the pagination footer and the guidance last.
func TestFormatListMarkdownString(t *testing.T) {
	t.Run("with accounts", func(t *testing.T) {
		out := ListOutput{
			Accounts:   []Output{{ID: 1, Name: "a", Username: "au", Email: "a@t.com"}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
		}
		want := "## Group Service Accounts (1)\n\n" +
			"| ID | Username | Name | Email |\n| --- | --- | --- | --- |\n" +
			"| 1 | au | a | a@t.com |\n" +
			"\nPage 1 of 1 | 1 items total\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'group.service_account_pat_list' to list one account's tokens\n"
		if got := FormatListMarkdownString(out); got != want {
			t.Errorf("FormatListMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got, want := FormatListMarkdownString(ListOutput{}), "No service accounts found.\n"; got != want {
			t.Errorf("FormatListMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestFormatPATMarkdownString verifies the whole card a service account token
// renders, including the granular scopes table and the store-it-now hint the
// secret adds.
func TestFormatPATMarkdownString(t *testing.T) {
	t.Run("with all fields", func(t *testing.T) {
		out := PATOutput{
			ID: 1, Name: "tok", Active: true, Scopes: []string{"api"},
			UserID: 42, CreatedAt: "2026-01-01T00:00:00Z",
			LastUsedAt: "2026-06-20T08:00:00Z",
			ExpiresAt:  "2026-12-31", Token: "secret",
			Granular:       true,
			GranularScopes: []toolutil.TokenGranularScopeOutput{{Access: "read", Permissions: []string{"read_code"}, ProjectID: 7}},
		}
		want := "## Personal Access Token: tok\n\n" +
			"- **ID**: 1\n" +
			"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
			"- **Scopes**: api\n" +
			"- **Granular**: " + toolutil.BoolEmoji(true) + "\n" +
			"- **User ID**: 42\n" +
			"- **Created**: 1 Jan 2026 00:00 UTC\n" +
			"- **Last Used**: 20 Jun 2026 08:00 UTC\n" +
			"- **Expires**: 31 Dec 2026\n" +
			"- **Token**: `secret`\n" +
			"\n### Granular Scopes\n\n" +
			"| Access | Permissions | Project | Group |\n| --- | --- | --- | --- |\n" +
			"| read | read_code | 7 | - |\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Store the token securely. It cannot be retrieved later\n" +
			"- Use action 'group.service_account_pat_rotate' to rotate this token\n" +
			"- Use action 'group.service_account_pat_revoke' to revoke it\n"
		if got := FormatPATMarkdownString(out); got != want {
			t.Errorf("FormatPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("revoked and without optional fields", func(t *testing.T) {
		out := PATOutput{ID: 2, Name: "m", Revoked: true, Scopes: []string{"read_api"}}
		want := "## Personal Access Token: m\n\n" +
			"- **ID**: 2\n" +
			"- **Active**: " + toolutil.BoolEmoji(false) + "\n" +
			"- " + toolutil.EmojiWarning + " **Revoked**\n" +
			"- **Scopes**: read_api\n" +
			"- **Granular**: " + toolutil.BoolEmoji(false) + "\n" +
			"- **User ID**: 0\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'group.service_account_pat_rotate' to rotate this token\n" +
			"- Use action 'group.service_account_pat_revoke' to revoke it\n"
		if got := FormatPATMarkdownString(out); got != want {
			t.Errorf("FormatPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestGroupServiceAccounts_UnreadableCapturedPublicEmail verifies that every
// service account handler returns an error rather than a half-filled account
// when GitLab sends public_email as something that is not a string. client-go
// models public_email on its own GroupServiceAccount as of v3.12.0, so its
// decoder reaches the bad value before the read of the captured response does;
// this asserts the refusal rather than which of the two made it.
func TestGroupServiceAccounts_UnreadableCapturedPublicEmail(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		call func(context.Context, *gitlabclient.Client) error
	}{
		{"list", `[{"id":1,"username":"svc","name":"Service","public_email":42}]`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := List(ctx, c, ListInput{GroupID: "7"})
			return err
		}},
		{"create", `{"id":1,"username":"svc","name":"Service","public_email":42}`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Create(ctx, c, CreateInput{GroupID: "7", Name: "Service"})
			return err
		}},
		{"update", `{"id":1,"username":"svc","name":"Service","public_email":42}`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Update(ctx, c, UpdateInput{GroupID: "7", ServiceAccountID: 1, Name: "Renamed"})
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

// TestAccountHandlers_External_IsReadOffTheCapture verifies that each handler
// answering with an account publishes the external flag GitLab sends on every
// account from 19.5, which client-go's GroupServiceAccount does not model, so
// the value comes from the captured response; and that a value the flag cannot
// hold is an error rather than an account published as internal. The SDK does
// not read the key, so the captured decode is the one that refuses it.
func TestAccountHandlers_External_IsReadOffTheCapture(t *testing.T) {
	handlers := []struct {
		name string
		list bool
		call func(context.Context, *gitlabclient.Client) ([]Output, error)
	}{
		{"list", true, func(ctx context.Context, c *gitlabclient.Client) ([]Output, error) {
			out, err := List(ctx, c, ListInput{GroupID: "7"})
			return out.Accounts, err
		}},
		{"get", false, func(ctx context.Context, c *gitlabclient.Client) ([]Output, error) {
			out, err := Get(ctx, c, GetInput{GroupID: "7", ServiceAccountID: 1})
			return []Output{out}, err
		}},
		{"create", false, func(ctx context.Context, c *gitlabclient.Client) ([]Output, error) {
			out, err := Create(ctx, c, CreateInput{GroupID: "7", Name: "Service"})
			return []Output{out}, err
		}},
		{"update", false, func(ctx context.Context, c *gitlabclient.Client) ([]Output, error) {
			out, err := Update(ctx, c, UpdateInput{GroupID: "7", ServiceAccountID: 1, Name: "Renamed"})
			return []Output{out}, err
		}},
	}
	for _, h := range handlers {
		for _, tt := range []struct {
			name    string
			field   string
			want    *bool
			wantErr bool
		}{
			{name: "external", field: `,"external":true`, want: new(true)},
			{name: "internal", field: `,"external":false`, want: new(false)},
			{name: "not sent by an older instance", field: ""},
			{name: "unreadable", field: `,"external":"yes"`, wantErr: true},
		} {
			t.Run(h.name+"/"+tt.name, func(t *testing.T) {
				body := `{"id":1,"username":"svc","name":"Service"` + tt.field + `}`
				if h.list {
					body = "[" + body + "]"
				}
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusOK, body)
				}))
				accounts, err := h.call(context.Background(), client)
				if tt.wantErr {
					if err == nil {
						t.Fatal("error = nil, want the captured decode to refuse the value")
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(accounts) != 1 || !reflect.DeepEqual(accounts[0].External, tt.want) {
					t.Errorf("accounts = %+v, want one with External %v", accounts, tt.want)
				}
			})
		}
	}
}

// TestFormatListPATMarkdownString verifies the whole list a page of service
// account tokens renders: every flag through the emoji and the expiry through
// the display layout rather than as GitLab spelled it.
func TestFormatListPATMarkdownString(t *testing.T) {
	t.Run("with tokens", func(t *testing.T) {
		out := ListPATOutput{
			Tokens:     []PATOutput{{ID: 1, Name: "t", Active: true, Scopes: []string{"api"}, ExpiresAt: "2026-01-01"}},
			Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1},
		}
		want := "## Service Account Tokens (1)\n\n" +
			"| ID | Name | Active | Revoked | Scopes | Expires |\n| --- | --- | --- | --- | --- | --- |\n" +
			"| 1 | t | " + toolutil.BoolEmoji(true) + " | " + toolutil.BoolEmoji(false) + " | api | 1 Jan 2026 |\n" +
			"\nPage 1 of 1 | 1 items total\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- Use action 'group.service_account_pat_revoke' to revoke one of these tokens\n"
		if got := FormatListPATMarkdownString(out); got != want {
			t.Errorf("FormatListPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got, want := FormatListPATMarkdownString(ListPATOutput{}), "No tokens found.\n"; got != want {
			t.Errorf("FormatListPATMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
}

// recordRequestBody decodes the JSON object a handler was sent into body. It
// runs on the httptest server's goroutine, so a body that does not decode is
// reported and the handler still answers, rather than aborting that goroutine.
func recordRequestBody(t *testing.T, r *http.Request, body *map[string]any) {
	t.Helper()
	var decoded map[string]any
	if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
		t.Errorf("decoding the %s %s body: %v", r.Method, r.URL.Path, err)
		return
	}
	*body = decoded
}

// assertRequestBody compares what GitLab was sent with the exact object the
// input should have produced. The comparison is whole rather than per key
// because an inverted guard does two things at once — it drops the field the
// caller gave and adds the one they left out — and a check for the presence of
// one key sees only half of that.
func assertRequestBody(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %v, want %v", got, want)
	}
}

// TestCreate_OptionalFields_AreSentOnlyWhenTheCallerGaveThem holds the three
// guards that decide what Create puts in the POST body. Nothing in the response
// depends on them: GitLab answers with the account it made, and a test that
// only reads the returned account passes whether the request named the account
// the caller asked for or an empty one. Inverted, each guard sends an empty
// value for the field that was given and omits it when it was not, which is a
// service account created under the wrong name and nothing to see it.
func TestCreate_OptionalFields_AreSentOnlyWhenTheCallerGaveThem(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateInput
		wantBody map[string]any
	}{
		{
			name:     "every optional field given",
			input:    CreateInput{GroupID: "mygroup", Name: "svc-bot", Username: "svc-user", Email: "svc@test.com"},
			wantBody: map[string]any{"name": "svc-bot", "username": "svc-user", "email": "svc@test.com"},
		},
		{
			name:     "none given",
			input:    CreateInput{GroupID: "mygroup"},
			wantBody: map[string]any{},
		},
		{
			name:     "only the username given",
			input:    CreateInput{GroupID: "mygroup", Username: "svc-user"},
			wantBody: map[string]any{"username": "svc-user"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordRequestBody(t, r, &sent)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":42,"name":"svc-bot","username":"svc-user","email":"svc@test.com"}`)
			}))
			if _, err := Create(context.Background(), client, tt.input); err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}
			assertRequestBody(t, sent, tt.wantBody)
		})
	}
}

// TestUpdate_OptionalFields_AreSentOnlyWhenTheCallerGaveThem is the same
// property for Update, where the cost of an inverted guard is higher: a PATCH
// carrying a field the caller never named overwrites what the account already
// had, and one omitting the field they did name renames nothing while the
// handler reports the account GitLab echoed back.
func TestUpdate_OptionalFields_AreSentOnlyWhenTheCallerGaveThem(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateInput
		wantBody map[string]any
	}{
		{
			name:     "every optional field given",
			input:    UpdateInput{GroupID: "mygroup", ServiceAccountID: 42, Name: "new", Username: "new-u", Email: "new@t.com"},
			wantBody: map[string]any{"name": "new", "username": "new-u", "email": "new@t.com"},
		},
		{
			name:     "none given",
			input:    UpdateInput{GroupID: "mygroup", ServiceAccountID: 42},
			wantBody: map[string]any{},
		},
		{
			name:     "only the email given",
			input:    UpdateInput{GroupID: "mygroup", ServiceAccountID: 42, Email: "new@t.com"},
			wantBody: map[string]any{"email": "new@t.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordRequestBody(t, r, &sent)
				testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"new","username":"new-u","email":"new@t.com"}`)
			}))
			if _, err := Update(context.Background(), client, tt.input); err != nil {
				t.Fatalf("Update() unexpected error: %v", err)
			}
			assertRequestBody(t, sent, tt.wantBody)
		})
	}
}

// TestCreatePAT_Description_IsSentOnlyWhenTheCallerGaveIt holds the one
// optional-field guard of the token create call, alongside the two fields it
// always sends and the expiry it parses. A token's description is the only
// thing distinguishing two tokens of the same name in a listing, and the
// response echoes whatever GitLab stored, so an inverted guard shows a
// description in the result of a request that never carried one.
func TestCreatePAT_Description_IsSentOnlyWhenTheCallerGaveIt(t *testing.T) {
	tests := []struct {
		name     string
		input    CreatePATInput
		wantBody map[string]any
	}{
		{
			name: "description and expiry given",
			input: CreatePATInput{
				GroupID: "mygroup", ServiceAccountID: 42, Name: "deploy-token",
				Scopes: []string{"api"}, Description: "CI deploy", ExpiresAt: "2026-12-31",
			},
			wantBody: map[string]any{
				"name": "deploy-token", "scopes": []any{"api"},
				"description": "CI deploy", "expires_at": "2026-12-31",
			},
		},
		{
			name: "neither given",
			input: CreatePATInput{
				GroupID: "mygroup", ServiceAccountID: 42, Name: "tok", Scopes: []string{"read_api"},
			},
			wantBody: map[string]any{"name": "tok", "scopes": []any{"read_api"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recordRequestBody(t, r, &sent)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"tok","scopes":["api"],"user_id":42,"active":true,"token":"glpat-xxxx"}`)
			}))
			if _, err := CreatePAT(context.Background(), client, tt.input); err != nil {
				t.Fatalf("CreatePAT() unexpected error: %v", err)
			}
			assertRequestBody(t, sent, tt.wantBody)
		})
	}
}

// TestList_KeysetPage_FollowsTheNextPageToken verifies that a group's service
// accounts, a list of users GitLab pages by cursor under keyset pagination,
// can be followed past the first page through the next_page_token it
// publishes (issue 1165).
func TestList_KeysetPage_FollowsTheNextPageToken(t *testing.T) {
	listing := &testutil.KeysetListing{
		Path: "/api/v4/groups/5/service_accounts", Param: "cursor", Value: "eyJpZCI6IjIifQ",
		First:  `[{"id":1,"username":"svc-1"},{"id":2,"username":"svc-2"}]`,
		Second: `[{"id":3,"username":"svc-3"}]`,
	}
	client := testutil.NewTestClient(t, listing)
	testutil.FollowKeyset(t, listing, func(token string) (testutil.KeysetPage, error) {
		out, err := List(t.Context(), client, ListInput{
			GroupID:    "5",
			Pagination: "keyset",
			PageToken:  token,
		})
		return testutil.KeysetPage{Token: out.Pagination.NextPageToken, HasMore: out.Pagination.HasMore, Rows: len(out.Accounts)}, err
	})
}

// TestListPATs_TimestampFilterThatDoesNotParse_IsRefusedBeforeAnyRequest
// verifies the token list refuses a timestamp filter it cannot read, naming
// the field and quoting the value, before any request (issue 1308).
func TestListPATs_TimestampFilterThatDoesNotParse_IsRefusedBeforeAnyRequest(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListPATs(context.Background(), client, ListPATInput{GroupID: "10", ServiceAccountID: 7, LastUsedBefore: "not-a-date"})
	if err == nil || !strings.Contains(err.Error(), "last_used_before must be") || !strings.Contains(err.Error(), `"not-a-date"`) {
		t.Errorf("ListPATs() error = %v, want the last_used_before refusal quoting the value", err)
	}
}

// TestListPATs_KeysetArgument_IsRefusedBeforeAnyRequest verifies that the
// token list refuses pagination, page_token and order_by, the three keyset
// fields of client-go's ListOptions, as arguments it does not take, before any
// request. GitLab pages these tokens with paginate_with_strategies over a
// PersonalAccessToken relation, a class with no keyset ordering, so it answers
// pagination=keyset with 405 and otherwise pages by offset, which ignores
// page_token; the route does not declare order_by, so nothing reads it
// (lib/api/group_service_accounts.rb at v19.5.0-ee, issue 1165). Offered, none
// of the three did anything a caller wants.
func TestListPATs_KeysetArgument_IsRefusedBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name  string
		param string
		value string
	}{
		{name: "Pagination", param: "pagination", value: "keyset"},
		{name: "PageToken", param: "page_token", value: "100"},
		{name: "OrderBy", param: "order_by", value: "created_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := toolutil.RouteAction(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), ListPATs)
			_, err := route.Handler(t.Context(), map[string]any{"group_id": "mygroup", "service_account_id": 42, tc.param: tc.value})
			if err == nil || !strings.Contains(err.Error(), `unknown field "`+tc.param+`"`) {
				t.Errorf("Handler() error = %v, want %s refused as an unknown field", err, tc.param)
			}
		})
	}
}

// TestListPATInput_Schema_OffersOffsetPaginationOnly verifies that the
// token list publishes page and per_page, which GitLab pages these tokens by,
// and none of the keyset fields: GitLab answers pagination=keyset with 405 on
// this route, ignores page_token without it, and does not declare order_by
// (issue 1165).
func TestListPATInput_Schema_OffersOffsetPaginationOnly(t *testing.T) {
	route := toolutil.RouteAction(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), ListPATs)
	props, _ := route.InputSchema["properties"].(map[string]any)
	cases := []struct {
		param   string
		offered bool
	}{
		{param: "page", offered: true},
		{param: "per_page", offered: true},
		{param: "pagination", offered: false},
		{param: "page_token", offered: false},
		{param: "order_by", offered: false},
	}
	for _, tc := range cases {
		t.Run(tc.param, func(t *testing.T) {
			if _, ok := props[tc.param]; ok != tc.offered {
				t.Errorf("schema offers %s = %t, want %t", tc.param, ok, tc.offered)
			}
		})
	}
}
