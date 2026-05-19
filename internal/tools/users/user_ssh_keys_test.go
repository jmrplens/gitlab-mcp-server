// user_ssh_keys_test.go contains unit tests for GitLab user SSH key
// management operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestListSSHKeysForUser_Success verifies ListSSHKeysForUser returns the key
// list when GET /users/:id/keys responds 200 with one key.
func TestListSSHKeysForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/keys" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"my-key","key":"ssh-rsa AAA","created_at":"2026-01-15T10:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListSSHKeysForUser(context.Background(), client, ListSSHKeysForUserInput{UserID: 42})
	if err != nil {
		t.Fatalf("ListSSHKeysForUser() unexpected error: %v", err)
	}
	if len(out.Keys) != 1 {
		t.Fatalf("len(out.Keys) = %d, want 1", len(out.Keys))
	}
}

// TestGetSSHKey_Success verifies GetSSHKey returns the key payload when
// GET /user/keys/:id responds 200 OK.
func TestGetSSHKey_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/keys/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"my-key","key":"ssh-rsa AAA","created_at":"2026-01-15T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetSSHKey(context.Background(), client, GetSSHKeyInput{KeyID: 1})
	if err != nil {
		t.Fatalf("GetSSHKey() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestAddSSHKey_Success verifies AddSSHKey returns the created key when
// POST /user/keys responds 201 Created.
func TestAddSSHKey_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/user/keys" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"title":"my-key","key":"ssh-rsa AAA","created_at":"2026-01-15T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddSSHKey(context.Background(), client, AddSSHKeyInput{Title: "my-key", Key: "ssh-rsa AAA"})
	if err != nil {
		t.Fatalf("AddSSHKey() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// TestAddSSHKey_MissingTitle verifies AddSSHKey returns a validation error when
// the title field is empty, without hitting the API.
func TestAddSSHKey_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := AddSSHKey(context.Background(), client, AddSSHKeyInput{Key: "ssh-rsa AAA"})
	if err == nil {
		t.Fatal("expected error for missing title, got nil")
	}
}

// TestDeleteSSHKey_Success verifies DeleteSSHKey reports Deleted=true when
// DELETE /user/keys/:id responds 204 No Content.
func TestDeleteSSHKey_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/user/keys/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{KeyID: 1})
	if err != nil {
		t.Fatalf("DeleteSSHKey() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}

const sshKeyJSON = `{"id":1,"title":"my-key","key":"ssh-rsa AAAA...BBBB","usage_type":"auth","created_at":"2026-01-15T10:00:00Z","expires_at":"2026-01-15T00:00:00Z"}`

// TestGetSSHKeyForUser_Success verifies retrieving an SSH key for a specific user.
func TestGetSSHKeyForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/keys/1" {
			testutil.RespondJSON(w, http.StatusOK, sshKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetSSHKeyForUser(context.Background(), client, GetSSHKeyForUserInput{UserID: 42, KeyID: 1})
	if err != nil {
		t.Fatalf("GetSSHKeyForUser() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.Title != "my-key" {
		t.Errorf("Title = %q, want %q", out.Title, "my-key")
	}
	if out.UsageType != "auth" {
		t.Errorf("UsageType = %q, want %q", out.UsageType, "auth")
	}
}

// TestGetSSHKeyForUser_MissingUserID verifies validation error for zero user_id.
func TestGetSSHKeyForUser_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetSSHKeyForUser(context.Background(), client, GetSSHKeyForUserInput{KeyID: 1})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestGetSSHKeyForUser_MissingKeyID verifies validation error for zero key_id.
func TestGetSSHKeyForUser_MissingKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetSSHKeyForUser(context.Background(), client, GetSSHKeyForUserInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for missing key_id, got nil")
	}
}

// TestGetSSHKeyForUser_APIError verifies error handling on API failure.
func TestGetSSHKeyForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetSSHKeyForUser(context.Background(), client, GetSSHKeyForUserInput{UserID: 42, KeyID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetSSHKeyForUser_CancelledContext verifies context cancellation.
func TestGetSSHKeyForUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetSSHKeyForUser(ctx, client, GetSSHKeyForUserInput{UserID: 42, KeyID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestAddSSHKeyForUser_Success verifies adding an SSH key to a specific user.
func TestAddSSHKeyForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/keys" {
			testutil.RespondJSON(w, http.StatusCreated, sshKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddSSHKeyForUser(context.Background(), client, AddSSHKeyForUserInput{
		UserID:    42,
		Title:     "my-key",
		Key:       "ssh-rsa AAAA...BBBB",
		ExpiresAt: "2026-01-15",
		UsageType: "auth",
	})
	if err != nil {
		t.Fatalf("AddSSHKeyForUser() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// TestAddSSHKeyForUser_MissingUserID verifies validation for zero user_id.
func TestAddSSHKeyForUser_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := AddSSHKeyForUser(context.Background(), client, AddSSHKeyForUserInput{Title: "k", Key: "ssh-rsa X"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestAddSSHKeyForUser_MissingTitle verifies validation for empty title.
func TestAddSSHKeyForUser_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := AddSSHKeyForUser(context.Background(), client, AddSSHKeyForUserInput{UserID: 42, Key: "ssh-rsa X"})
	if err == nil {
		t.Fatal("expected error for missing title, got nil")
	}
}

// TestAddSSHKeyForUser_MissingKey verifies validation for empty key.
func TestAddSSHKeyForUser_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := AddSSHKeyForUser(context.Background(), client, AddSSHKeyForUserInput{UserID: 42, Title: "k"})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

// TestAddSSHKeyForUser_APIError verifies error handling on API failure.
func TestAddSSHKeyForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"key already exists"}`)
	}))

	_, err := AddSSHKeyForUser(context.Background(), client, AddSSHKeyForUserInput{
		UserID: 42, Title: "dup", Key: "ssh-rsa AAAA",
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestAddSSHKeyForUser_CancelledContext verifies context cancellation.
func TestAddSSHKeyForUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := AddSSHKeyForUser(ctx, client, AddSSHKeyForUserInput{
		UserID: 42, Title: "k", Key: "ssh-rsa X",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteSSHKeyForUser_Success verifies deleting an SSH key for a specific user.
func TestDeleteSSHKeyForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/users/42/keys/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeleteSSHKeyForUser(context.Background(), client, DeleteSSHKeyForUserInput{UserID: 42, KeyID: 1})
	if err != nil {
		t.Fatalf("DeleteSSHKeyForUser() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("Deleted = false, want true")
	}
	if out.KeyID != 1 {
		t.Errorf("KeyID = %d, want 1", out.KeyID)
	}
}

// TestDeleteSSHKeyForUser_MissingUserID verifies validation for zero user_id.
func TestDeleteSSHKeyForUser_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := DeleteSSHKeyForUser(context.Background(), client, DeleteSSHKeyForUserInput{KeyID: 1})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestDeleteSSHKeyForUser_MissingKeyID verifies validation for zero key_id.
func TestDeleteSSHKeyForUser_MissingKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := DeleteSSHKeyForUser(context.Background(), client, DeleteSSHKeyForUserInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for missing key_id, got nil")
	}
}

// TestDeleteSSHKeyForUser_APIError verifies error handling on API failure.
func TestDeleteSSHKeyForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := DeleteSSHKeyForUser(context.Background(), client, DeleteSSHKeyForUserInput{UserID: 42, KeyID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestDeleteSSHKeyForUser_CancelledContext verifies context cancellation.
func TestDeleteSSHKeyForUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := DeleteSSHKeyForUser(ctx, client, DeleteSSHKeyForUserInput{UserID: 42, KeyID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListSSHKeysForUser_MissingUserID verifies validation for zero user_id.
func TestListSSHKeysForUser_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListSSHKeysForUser(context.Background(), client, ListSSHKeysForUserInput{})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestListSSHKeysForUser_APIError verifies error handling on API failure.
func TestListSSHKeysForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := ListSSHKeysForUser(context.Background(), client, ListSSHKeysForUserInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestListSSHKeysForUser_CancelledContext verifies context cancellation.
func TestListSSHKeysForUser_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListSSHKeysForUser(ctx, client, ListSSHKeysForUserInput{UserID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetSSHKey_MissingKeyID verifies validation for zero key_id.
func TestGetSSHKey_MissingKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetSSHKey(context.Background(), client, GetSSHKeyInput{})
	if err == nil {
		t.Fatal("expected error for missing key_id, got nil")
	}
}

// TestGetSSHKey_APIError verifies error handling on API failure.
func TestGetSSHKey_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetSSHKey(context.Background(), client, GetSSHKeyInput{KeyID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestGetSSHKey_CancelledContext verifies context cancellation.
func TestGetSSHKey_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetSSHKey(ctx, client, GetSSHKeyInput{KeyID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestAddSSHKey_WithExpiresAtAndUsageType verifies AddSSHKey with all optional fields.
func TestAddSSHKey_WithExpiresAtAndUsageType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/user/keys" {
			testutil.RespondJSON(w, http.StatusCreated, sshKeyJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddSSHKey(context.Background(), client, AddSSHKeyInput{
		Title:     "my-key",
		Key:       "ssh-rsa AAAA...BBBB",
		ExpiresAt: "2026-01-15",
		UsageType: "auth",
	})
	if err != nil {
		t.Fatalf("AddSSHKey() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
}

// TestAddSSHKey_MissingKey verifies validation for empty key.
func TestAddSSHKey_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := AddSSHKey(context.Background(), client, AddSSHKeyInput{Title: "test"})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

// TestAddSSHKey_APIError verifies error handling on API failure.
func TestAddSSHKey_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"key is invalid"}`)
	}))

	_, err := AddSSHKey(context.Background(), client, AddSSHKeyInput{Title: "k", Key: "invalid"})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestAddSSHKey_CancelledContext verifies context cancellation.
func TestAddSSHKey_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := AddSSHKey(ctx, client, AddSSHKeyInput{Title: "k", Key: "ssh-rsa X"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteSSHKey_MissingKeyID verifies validation for zero key_id.
func TestDeleteSSHKey_MissingKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{})
	if err == nil {
		t.Fatal("expected error for missing key_id, got nil")
	}
}

// TestDeleteSSHKey_APIError verifies error handling on API failure.
func TestDeleteSSHKey_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := DeleteSSHKey(context.Background(), client, DeleteSSHKeyInput{KeyID: 999})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// TestDeleteSSHKey_CancelledContext verifies context cancellation.
func TestDeleteSSHKey_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := DeleteSSHKey(ctx, client, DeleteSSHKeyInput{KeyID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestFormatSSHKeyMarkdownString_WithData verifies single SSH key markdown formatting.
func TestFormatSSHKeyMarkdownString_WithData(t *testing.T) {
	out := SSHKeyOutput{
		ID:        1,
		Title:     "Work Laptop",
		Key:       "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBig",
		UsageType: "auth",
		CreatedAt: "2026-01-01T00:00:00Z",
		ExpiresAt: "2026-01-01T00:00:00Z",
	}
	md := FormatSSHKeyMarkdownString(out)

	for _, want := range []string{
		"## SSH Key: Work Laptop",
		"**Title**: Work Laptop",
		"**Usage Type**: auth",
		"**Expires At**",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatSSHKeyMarkdownString_MinimalFields verifies markdown with no optional fields.
func TestFormatSSHKeyMarkdownString_MinimalFields(t *testing.T) {
	md := FormatSSHKeyMarkdownString(SSHKeyOutput{
		ID: 1, Title: "k", Key: "ssh-rsa AAAA...........BBBBCCCC",
	})
	if !strings.Contains(md, "## SSH Key: k") {
		t.Errorf("missing header:\n%s", md)
	}
	if strings.Contains(md, "**Usage Type**") {
		t.Error("should not contain Usage Type when empty")
	}
	if strings.Contains(md, "**Expires At**") {
		t.Error("should not contain Expires At when empty")
	}
}

// TestFormatSSHKeyMarkdown_ReturnsMCPResult verifies the MCP result wrapper.
func TestFormatSSHKeyMarkdown_ReturnsMCPResult(t *testing.T) {
	result := FormatSSHKeyMarkdown(SSHKeyOutput{ID: 1, Title: "k", Key: "ssh-rsa X"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}
