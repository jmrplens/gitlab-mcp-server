package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

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

func TestBlockUser_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := BlockUser(context.Background(), client, AdminActionInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

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

func TestFormatAdminActionMarkdownString(t *testing.T) {
	md := FormatAdminActionMarkdownString(AdminActionOutput{UserID: 42, Action: "block", Success: true})
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}
