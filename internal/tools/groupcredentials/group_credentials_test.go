package groupcredentials

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const patJSON = `[{"id":1,"name":"my-token","revoked":false,"created_at":"2024-01-01T00:00:00Z","description":"desc","scopes":["api"],"user_id":10,"active":true,"expires_at":"2025-01-01"}]`
const sshKeyJSON = `[{"id":5,"title":"my-key","created_at":"2024-01-01T00:00:00Z","expires_at":"2025-06-01T00:00:00Z","usage_type":"auth","user_id":10}]`

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
	if out.Tokens[0].State != "active" {
		t.Errorf("expected state active, got %s", out.Tokens[0].State)
	}
}

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
		GroupID:         toolutil.StringOrInt("mygroup"),
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("ListPATs() error: %v", err)
	}
}

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

func TestListPATs_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListPATs(context.Background(), client, ListPATsInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestListPATs_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListPATs(ctx, client, ListPATsInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

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

func TestListSSHKeys_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListSSHKeys(context.Background(), client, ListSSHKeysInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestListSSHKeys_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListSSHKeys(ctx, client, ListSSHKeysInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

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

func TestRevokePAT_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := RevokePAT(context.Background(), client, RevokePATInput{TokenID: 99})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

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

func TestRevokePAT_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RevokePAT(ctx, client, RevokePATInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		TokenID: 99,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

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

func TestDeleteSSHKey_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{KeyID: 5})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDeleteSSHKey_MissingKeyID verifies that DeleteSSHKey returns a validation
// error when key_id is zero.
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

// TestDeleteSSHKey_CancelledContext verifies that DeleteSSHKey returns an error
// when the context is already cancelled before the API call.
func TestDeleteSSHKey_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DeleteSSHKey(ctx, client, DeleteSSHKeyInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		KeyID:   5,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteSSHKey_APIError verifies that DeleteSSHKey propagates a 404 API error.
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

// TestListSSHKeys_WithPagination verifies that page and per_page parameters
// are sent as query parameters to the GitLab API.
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
		GroupID:         toolutil.StringOrInt("mygroup"),
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("ListSSHKeys() error: %v", err)
	}
}

// TestListPATs_RevokedTokenState verifies that toPATOutput assigns state "revoked"
// when the token has revoked=true, and that the LastUsedAt date is populated.
func TestListPATs_RevokedTokenState(t *testing.T) {
	revokedJSON := `[{"id":2,"name":"revoked-token","revoked":true,"active":false,"created_at":"2024-01-01T00:00:00Z","scopes":["read_api"],"user_id":20,"last_used_at":"2024-03-01T12:00:00Z","expires_at":"2025-01-01"}]`
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
	if tok.State != "revoked" {
		t.Errorf("expected state revoked, got %s", tok.State)
	}
	if !tok.Revoked {
		t.Error("expected Revoked to be true")
	}
	if tok.LastUsedAt == "" {
		t.Error("expected LastUsedAt to be set")
	}
}

// TestListPATs_InactiveTokenState verifies that toPATOutput assigns state "inactive"
// when the token is neither revoked nor active, and omits unset optional dates.
func TestListPATs_InactiveTokenState(t *testing.T) {
	inactiveJSON := `[{"id":3,"name":"inactive-token","revoked":false,"active":false,"created_at":"2024-01-01T00:00:00Z","scopes":["read_user"],"user_id":30}]`
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
	if tok.State != "inactive" {
		t.Errorf("expected state inactive, got %s", tok.State)
	}
	if tok.ExpiresAt != "" {
		t.Errorf("expected ExpiresAt to be empty, got %s", tok.ExpiresAt)
	}
	if tok.LastUsedAt != "" {
		t.Errorf("expected LastUsedAt to be empty, got %s", tok.LastUsedAt)
	}
}

// TestListSSHKeys_WithLastUsedAt verifies that toSSHKeyOutput populates the
// LastUsedAt field when the API response includes last_used_at.
func TestListSSHKeys_WithLastUsedAt(t *testing.T) {
	keyJSON := `[{"id":7,"title":"used-key","created_at":"2024-01-01T00:00:00Z","expires_at":"2025-06-01T00:00:00Z","last_used_at":"2024-06-15T10:30:00Z","usage_type":"auth","user_id":15}]`
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

// TestListPATs_WithRevokedFilter verifies that the revoked filter parameter
// is passed through to the GitLab API as a query parameter.
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
