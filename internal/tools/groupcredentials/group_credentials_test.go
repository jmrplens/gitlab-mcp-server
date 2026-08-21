// group_credentials_test.go contains unit tests for GitLab group credential
// storage operations. Tests use httptest to mock the GitLab API.
package groupcredentials

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	patJSON    = `[{"id":1,"name":"my-token","revoked":false,"created_at":"2026-01-01T00:00:00Z","description":"desc","scopes":["api"],"user_id":10,"active":true,"expires_at":"2026-01-01"}]`
	sshKeyJSON = `[{"id":5,"title":"my-key","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-06-01T00:00:00Z","usage_type":"auth","user_id":10}]`
)

// TestListPATs_Success verifies that ListPATs succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListPATs_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.RespondJSON(w, http.StatusOK, patJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	if out.Tokens[0].Name != "my-token" {
		t.Errorf("expected name my-token, got %s", out.Tokens[0].Name)
	}
	if out.Tokens[0].UserID != 10 {
		t.Errorf("expected user_id 10, got %d", out.Tokens[0].UserID)
	}
	if !out.Tokens[0].Active {
		t.Errorf("expected token to be active, got Active=%t", out.Tokens[0].Active)
	}
}

// TestGroupCredential404Hints verifies that unavailable group credential
// inventory endpoints return model-actionable guidance instead of only asking
// the model to re-check a valid group_id.
func TestGroupCredential404Hints(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/mygroup/manage/personal_access_tokens",
			"/api/v4/groups/mygroup/manage/personal_access_tokens/99",
			"/api/v4/groups/mygroup/manage/ssh_keys",
			"/api/v4/groups/mygroup/manage/ssh_keys/5":
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	tests := []struct {
		name string
		run  func() error
		want []string
	}{
		{
			name: "list PATs",
			run: func() error {
				_, err := ListPATs(context.Background(), client, ListPATsInput{GroupID: toolutil.StringOrInt("mygroup")})
				return err
			},
			want: []string{"group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin"},
		},
		{
			name: "list SSH keys",
			run: func() error {
				_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{GroupID: toolutil.StringOrInt("mygroup")})
				return err
			},
			want: []string{"group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin"},
		},
		{
			name: "revoke PAT",
			run: func() error {
				return RevokePAT(context.Background(), client, RevokePATInput{GroupID: toolutil.StringOrInt("mygroup"), TokenID: 99})
			},
			want: []string{"credential_list_pats", "group credential inventory", "Ultimate", "Owner or admin"},
		},
		{
			name: "delete SSH key",
			run: func() error {
				return DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{GroupID: toolutil.StringOrInt("mygroup"), KeyID: 5})
			},
			want: []string{"credential_list_ssh_keys", "group credential inventory", "Ultimate", "Owner or admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestToPATOutput_Nil verifies the ToPATOutput_Nil handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToPATOutput_Nil(t *testing.T) {
	out := toPATOutput(nil)
	if out.ID != 0 || out.Name != "" || len(out.Scopes) != 0 || out.Active || out.Revoked {
		t.Fatalf("toPATOutput(nil) = %+v, want zero output", out)
	}
}

// TestListPATs_WithPagination verifies that ListPATs_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListPATs_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.AssertQueryParam(t, r, "page", "2")
			testutil.AssertQueryParam(t, r, "per_page", "10")
			testutil.RespondJSON(w, http.StatusOK, patJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Page:    2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
}

// TestListPATs_WithFilters verifies the ListPATs_WithFilters handler.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListPATs_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.AssertQueryParam(t, r, "search", "deploy")
			testutil.AssertQueryParam(t, r, "state", "active")
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Search:  "deploy",
		State:   "active",
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
}

// TestListPATs_MissingGroupID verifies that ListPATs_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListPATs_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListPATs_CancelledContext verifies the ListPATs_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListPATs_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListPATs(ctx, client, ListPATsInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListPATs_APIError verifies that ListPATs returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListPATs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestListSSHKeys_Success verifies that ListSSHKeys succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/manage/ssh_keys (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListSSHKeys_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys" {
			testutil.RespondJSON(w, http.StatusOK, sshKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("ListSSHKeys() error: %v", err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.Keys))
	}
	if out.Keys[0].Title != "my-key" {
		t.Errorf("expected title my-key, got %s", out.Keys[0].Title)
	}
	if out.Keys[0].UsageType != "auth" {
		t.Errorf("expected usage_type auth, got %s", out.Keys[0].UsageType)
	}
}

// TestToSSHKeyOutput_Nil verifies the ToSSHKeyOutput_Nil handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToSSHKeyOutput_Nil(t *testing.T) {
	out := toSSHKeyOutput(nil)
	if out != (SSHKeyOutput{}) {
		t.Fatalf("toSSHKeyOutput(nil) = %+v, want zero output", out)
	}
}

// TestListSSHKeys_MissingGroupID verifies that ListSSHKeys_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListSSHKeys_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListSSHKeys_CancelledContext verifies the ListSSHKeys_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListSSHKeys_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListSSHKeys(ctx, client, ListSSHKeysInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListSSHKeys_APIError verifies that ListSSHKeys returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/manage/ssh_keys (GET) responds with HTTP BadRequest.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListSSHKeys_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestRevokePAT_Success verifies that RevokePAT succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens/99 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestRevokePAT_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens/99" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := RevokePAT(context.Background(), client, RevokePATInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		TokenID: 99,
	})
	if err != nil {
		t.Fatalf("RevokePAT() error: %v", err)
	}
}

// TestRevokePAT_MissingGroupID verifies that RevokePAT_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRevokePAT_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := RevokePAT(context.Background(), client, RevokePATInput{TokenID: 99})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestRevokePAT_MissingTokenID verifies that RevokePAT_MissingTokenID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRevokePAT_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := RevokePAT(context.Background(), client, RevokePATInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for zero token_id, got nil")
	}
}

// TestRevokePAT_CancelledContext verifies the RevokePAT_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestRevokePAT_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := RevokePAT(ctx, client, RevokePATInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		TokenID: 99,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestRevokePAT_APIError verifies that RevokePAT returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens/99 (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRevokePAT_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens/99" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := RevokePAT(context.Background(), client, RevokePATInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		TokenID: 99,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestDeleteSSHKey_Success verifies that DeleteSSHKey succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/mygroup/manage/ssh_keys/5 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDeleteSSHKey_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys/5" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		KeyID:   5,
	})
	if err != nil {
		t.Fatalf("DeleteSSHKey() error: %v", err)
	}
}

// TestDeleteSSHKey_MissingGroupID verifies that DeleteSSHKey_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteSSHKey_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{KeyID: 5})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDeleteSSHKey_MissingKeyID verifies that DeleteSSHKey_MissingKeyID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteSSHKey_MissingKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for zero key_id, got nil")
	}
}

// TestDeleteSSHKey_CancelledContext verifies the DeleteSSHKey_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteSSHKey_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := DeleteSSHKey(ctx, client, DeleteSSHKeyInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		KeyID:   5,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteSSHKey_APIError verifies that DeleteSSHKey returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/groups/mygroup/manage/ssh_keys/5 (GET) responds with HTTP NotFound.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteSSHKey_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys/5" {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		KeyID:   5,
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestListSSHKeys_WithPagination verifies that ListSSHKeys_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/groups/mygroup/manage/ssh_keys (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListSSHKeys_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys" {
			testutil.AssertQueryParam(t, r, "page", "3")
			testutil.AssertQueryParam(t, r, "per_page", "5")
			testutil.RespondJSON(w, http.StatusOK, sshKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Page:    3, PerPage: 5,
	})
	if err != nil {
		t.Fatalf("ListSSHKeys() error: %v", err)
	}
}

// TestListPATs_RevokedTokenState verifies that toPATOutput surfaces the SDK
// Revoked flag when the token has revoked=true, and that the LastUsedAt date is populated.
func TestListPATs_RevokedTokenState(t *testing.T) {
	revokedJSON := `[{"id":2,"name":"revoked-token","revoked":true,"active":false,"created_at":"2026-01-01T00:00:00Z","scopes":["read_api"],"user_id":20,"last_used_at":"2026-03-01T12:00:00Z","expires_at":"2026-01-01"}]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.RespondJSON(w, http.StatusOK, revokedJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	tok := out.Tokens[0]
	if !tok.Revoked {
		t.Error("expected Revoked to be true")
	}
	if tok.Active {
		t.Error("expected Active to be false for a revoked token")
	}
	if tok.LastUsedAt == "" {
		t.Error("expected LastUsedAt to be set")
	}
}

// TestListPATs_InactiveTokenState verifies that toPATOutput reports a token that
// is neither revoked nor active, and omits unset optional dates.
func TestListPATs_InactiveTokenState(t *testing.T) {
	inactiveJSON := `[{"id":3,"name":"inactive-token","revoked":false,"active":false,"created_at":"2026-01-01T00:00:00Z","scopes":["read_user"],"user_id":30}]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.RespondJSON(w, http.StatusOK, inactiveJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	tok := out.Tokens[0]
	if tok.Revoked || tok.Active {
		t.Errorf("expected inactive token, got Revoked=%t Active=%t", tok.Revoked, tok.Active)
	}
	if tok.ExpiresAt != "" {
		t.Errorf("expected ExpiresAt to be empty, got %s", tok.ExpiresAt)
	}
	if tok.LastUsedAt != "" {
		t.Errorf("expected LastUsedAt to be empty, got %s", tok.LastUsedAt)
	}
}

// TestListSSHKeys_WithLastUsedAt verifies the ListSSHKeys_WithLastUsedAt handler.
// The mock GitLab API at /api/v4/groups/mygroup/manage/ssh_keys (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListSSHKeys_WithLastUsedAt(t *testing.T) {
	keyJSON := `[{"id":7,"title":"used-key","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-06-01T00:00:00Z","last_used_at":"2026-06-15T10:30:00Z","usage_type":"auth","user_id":15}]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys" {
			testutil.RespondJSON(w, http.StatusOK, keyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("ListSSHKeys() error: %v", err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.Keys))
	}
	if out.Keys[0].LastUsedAt == "" {
		t.Error("expected LastUsedAt to be set")
	}
}

// TestListPATs_WithRevokedFilter verifies the ListPATs_WithRevokedFilter handler.
// The mock GitLab API at /api/v4/groups/mygroup/manage/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListPATs_WithRevokedFilter(t *testing.T) {
	revoked := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.AssertQueryParam(t, r, "revoked", "true")
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Revoked: &revoked,
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
}

// TestListPATs_WithDateAndKeysetFilters verifies that ListPATs forwards the
// created/last-used date filters, order_by, sort, and keyset pagination
// (pagination + page_token) query parameters to the GitLab API.
func TestListPATs_WithDateAndKeysetFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			testutil.AssertQueryParam(t, r, "created_after", "2026-01-01")
			testutil.AssertQueryParam(t, r, "created_before", "2026-12-31")
			testutil.AssertQueryParam(t, r, "last_used_after", "2026-02-01")
			testutil.AssertQueryParam(t, r, "last_used_before", "2026-11-30")
			testutil.AssertQueryParam(t, r, "order_by", "created_at")
			testutil.AssertQueryParam(t, r, "sort", "desc")
			testutil.AssertQueryParam(t, r, "pagination", "keyset")
			testutil.AssertQueryParam(t, r, "page_token", "abc123")
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID:        toolutil.StringOrInt("mygroup"),
		CreatedAfter:   "2026-01-01",
		CreatedBefore:  "2026-12-31",
		LastUsedAfter:  "2026-02-01",
		LastUsedBefore: "2026-11-30",
		OrderBy:        "created_at",
		Sort:           "desc",
		Pagination:     "keyset",
		PageToken:      "abc123",
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
}

// TestListPATs_InvalidDateIgnored verifies that an unparseable date filter is
// dropped (parseISODate returns nil) rather than rejected, so the request still
// reaches the API without the offending query parameter.
func TestListPATs_InvalidDateIgnored(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/personal_access_tokens" {
			if r.URL.Query().Has("created_after") {
				t.Errorf("expected created_after to be omitted for invalid date, got %q", r.URL.Query().Get("created_after"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{
		GroupID:      toolutil.StringOrInt("mygroup"),
		CreatedAfter: "not-a-date",
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
}

// TestListSSHKeys_WithDateAndKeysetFilters verifies that ListSSHKeys forwards the
// created/expires date filters, order_by, sort, and keyset pagination query
// parameters to the GitLab API.
func TestListSSHKeys_WithDateAndKeysetFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/manage/ssh_keys" {
			testutil.AssertQueryParam(t, r, "created_after", "2026-01-01")
			testutil.AssertQueryParam(t, r, "created_before", "2026-12-31")
			testutil.AssertQueryParam(t, r, "expires_after", "2026-03-01")
			testutil.AssertQueryParam(t, r, "expires_before", "2026-10-31")
			testutil.AssertQueryParam(t, r, "order_by", "id")
			testutil.AssertQueryParam(t, r, "sort", "asc")
			testutil.AssertQueryParam(t, r, "pagination", "keyset")
			testutil.AssertQueryParam(t, r, "page_token", "xyz789")
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{
		GroupID:       toolutil.StringOrInt("mygroup"),
		CreatedAfter:  "2026-01-01",
		CreatedBefore: "2026-12-31",
		ExpiresAfter:  "2026-03-01",
		ExpiresBefore: "2026-10-31",
		OrderBy:       "id",
		Sort:          "asc",
		Pagination:    "keyset",
		PageToken:     "xyz789",
	})
	if err != nil {
		t.Fatalf("ListSSHKeys() error: %v", err)
	}
}

// TestParseISODate verifies parseISODate parses valid YYYY-MM-DD input into a
// non-nil *gl.ISOTime and returns nil for empty or malformed input.
func TestParseISODate(t *testing.T) {
	if got := parseISODate(""); got != nil {
		t.Errorf("parseISODate(\"\") = %v, want nil", got)
	}
	if got := parseISODate("nope"); got != nil {
		t.Errorf("parseISODate(\"nope\") = %v, want nil", got)
	}
	got := parseISODate("2026-01-02")
	if got == nil {
		t.Fatal("parseISODate(\"2026-01-02\") = nil, want non-nil")
	}
	if s := got.String(); s != "2026-01-02" {
		t.Errorf("parseISODate(\"2026-01-02\").String() = %q, want %q", s, "2026-01-02")
	}
}
