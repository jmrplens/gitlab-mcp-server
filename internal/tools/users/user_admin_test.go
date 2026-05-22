// user_admin_test.go contains unit tests for GitLab user administration
// operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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

// TestAdminActions_TableDriven validates all admin state actions (block, unblock,
// ban, unban, activate, deactivate, approve, reject, disable_two_factor) across
// success, validation error, API error, and cancelled context scenarios.
func TestAdminActions_TableDriven(t *testing.T) {
	actions := []struct {
		name       string
		fn         func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
		method     string
		path       string
		mockStatus int
		wantAction string
	}{
		{"UnbanUser", UnbanUser, http.MethodPost, "/api/v4/users/42/unban", http.StatusCreated, "unbanned"},
		{"DisableTwoFactor", DisableTwoFactor, http.MethodPatch, "/api/v4/users/42/disable_two_factor", http.StatusNoContent, "two_factor_disabled"},
		{"BlockUser", BlockUser, http.MethodPost, "/api/v4/users/42/block", http.StatusCreated, "blocked"},
		{"UnblockUser", UnblockUser, http.MethodPost, "/api/v4/users/42/unblock", http.StatusCreated, "unblocked"},
		{"BanUser", BanUser, http.MethodPost, "/api/v4/users/42/ban", http.StatusCreated, "banned"},
		{"ActivateUser", ActivateUser, http.MethodPost, "/api/v4/users/42/activate", http.StatusCreated, "activated"},
		{"DeactivateUser", DeactivateUser, http.MethodPost, "/api/v4/users/42/deactivate", http.StatusCreated, "deactivated"},
		{"ApproveUser", ApproveUser, http.MethodPost, "/api/v4/users/42/approve", http.StatusCreated, "approved"},
		{"RejectUser", RejectUser, http.MethodPost, "/api/v4/users/42/reject", http.StatusOK, "rejected"},
	}

	for _, action := range actions {
		runAdminActionCases(t, action)
	}
}

func runAdminActionCases(t *testing.T, action struct {
	name       string
	fn         func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	method     string
	path       string
	mockStatus int
	wantAction string
},
) {
	t.Helper()
	t.Run(action.name+"_Success", func(t *testing.T) { assertAdminActionSuccess(t, action) })
	t.Run(action.name+"_ValidationError", func(t *testing.T) { assertAdminActionValidationError(t, action) })
	t.Run(action.name+"_APIError", func(t *testing.T) { assertAdminActionAPIError(t, action) })
	t.Run(action.name+"_CancelledContext", func(t *testing.T) { assertAdminActionCancelledContext(t, action) })
}

func assertAdminActionSuccess(t *testing.T, action struct {
	name       string
	fn         func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	method     string
	path       string
	mockStatus int
	wantAction string
},
) {
	t.Helper()
	client := testutil.NewTestClient(t, adminActionSuccessHandler(action.method, action.path, action.mockStatus))
	out, err := action.fn(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("%s() unexpected error: %v", action.name, err)
	}
	if !out.Success {
		t.Errorf("%s(): Success = false, want true", action.name)
	}
	if out.Action != action.wantAction {
		t.Errorf("%s(): Action = %q, want %q", action.name, out.Action, action.wantAction)
	}
	if out.UserID != 42 {
		t.Errorf("%s(): UserID = %d, want 42", action.name, out.UserID)
	}
}

func adminActionSuccessHandler(method, path string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == method && r.URL.Path == path {
			w.WriteHeader(status)
			return
		}
		http.NotFound(w, r)
	}
}

func assertAdminActionValidationError(t *testing.T, action struct {
	name       string
	fn         func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	method     string
	path       string
	mockStatus int
	wantAction string
},
) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }))
	_, err := action.fn(context.Background(), client, AdminActionInput{UserID: 0})
	if err == nil {
		t.Fatalf("%s(): expected validation error for zero user_id, got nil", action.name)
	}
}

func assertAdminActionAPIError(t *testing.T, action struct {
	name       string
	fn         func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	method     string
	path       string
	mockStatus int
	wantAction string
},
) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := action.fn(context.Background(), client, AdminActionInput{UserID: 42})
	if err == nil {
		t.Fatalf("%s(): expected API error, got nil", action.name)
	}
}

func assertAdminActionCancelledContext(t *testing.T, action struct {
	name       string
	fn         func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	method     string
	path       string
	mockStatus int
	wantAction string
},
) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) }))
	_, err := action.fn(testutil.CancelledCtx(t), client, AdminActionInput{UserID: 42})
	if err == nil {
		t.Fatalf("%s(): expected error for cancelled context, got nil", action.name)
	}
}

// TestFormatAdminActionMarkdownString_Fields verifies that all output fields
// appear in the formatted Markdown string.
func TestFormatAdminActionMarkdownString_Fields(t *testing.T) {
	md := FormatAdminActionMarkdownString(AdminActionOutput{
		UserID: 99, Action: "banned", Success: true,
	})
	for _, want := range []string{"99", "banned", "true"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}
