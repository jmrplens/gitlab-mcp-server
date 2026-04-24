// user_admin_test.go contains unit tests for GitLab user administration
// operations. Tests use httptest to mock the GitLab Users API.

package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestBlockUser_Success verifies BlockUser returns Success=true and Action="blocked"
// when POST /users/:id/block responds 201 Created.
func TestBlockUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/block" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := BlockUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("BlockUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
	if out.Action != "blocked" {
		t.Errorf("out.Action = %q, want %q", out.Action, "blocked")
	}
}

// TestBlockUser_InvalidUserID verifies BlockUser returns a validation error
// when user_id=0, without hitting the API.
func TestBlockUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := BlockUser(context.Background(), client, AdminActionInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

// TestUnblockUser_Success verifies UnblockUser returns Success=true when
// POST /users/:id/unblock responds 201 Created.
func TestUnblockUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/unblock" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UnblockUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("UnblockUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
}

// TestBanUser_Success verifies BanUser returns Success=true when
// POST /users/:id/ban responds 201 Created.
func TestBanUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/ban" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := BanUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("BanUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
}

// TestActivateUser_Success verifies ActivateUser returns Success=true when
// POST /users/:id/activate responds 201 Created.
func TestActivateUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/activate" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ActivateUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("ActivateUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
}

// TestDeactivateUser_Success verifies DeactivateUser returns Success=true when
// POST /users/:id/deactivate responds 201 Created.
func TestDeactivateUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/deactivate" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeactivateUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("DeactivateUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
}

// TestApproveUser_Success verifies ApproveUser returns Success=true when
// POST /users/:id/approve responds 201 Created.
func TestApproveUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/approve" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ApproveUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("ApproveUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
}

// TestRejectUser_Success verifies RejectUser returns Success=true when
// POST /users/:id/reject responds 200 OK.
func TestRejectUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/reject" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RejectUser(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("RejectUser() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("out.Success = false, want true")
	}
}

// TestFormatAdminActionMarkdownString verifies FormatAdminActionMarkdownString
// produces non-empty markdown for a successful admin action result.
func TestFormatAdminActionMarkdownString(t *testing.T) {
	md := FormatAdminActionMarkdownString(AdminActionOutput{UserID: 42, Action: "block", Success: true})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}
