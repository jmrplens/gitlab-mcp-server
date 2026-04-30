// user_ssh_keys_test.go contains unit tests for GitLab user SSH key
// management operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
