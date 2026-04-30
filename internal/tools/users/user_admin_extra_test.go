// user_admin_extra_test.go covers admin action functions not exercised by the base
// test file: UnbanUser, DisableTwoFactor success paths, API error paths, and
// cancelled-context paths for every admin action.
package users

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

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
		t.Run(action.name+"_Success", func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == action.method && r.URL.Path == action.path {
					w.WriteHeader(action.mockStatus)
					return
				}
				http.NotFound(w, r)
			}))

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
		})

		t.Run(action.name+"_ValidationError", func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.NotFound(w, nil)
			}))

			_, err := action.fn(context.Background(), client, AdminActionInput{UserID: 0})
			if err == nil {
				t.Fatalf("%s(): expected validation error for zero user_id, got nil", action.name)
			}
		})

		t.Run(action.name+"_APIError", func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			}))

			_, err := action.fn(context.Background(), client, AdminActionInput{UserID: 42})
			if err == nil {
				t.Fatalf("%s(): expected API error, got nil", action.name)
			}
		})

		t.Run(action.name+"_CancelledContext", func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			ctx := testutil.CancelledCtx(t)

			_, err := action.fn(ctx, client, AdminActionInput{UserID: 42})
			if err == nil {
				t.Fatalf("%s(): expected error for cancelled context, got nil", action.name)
			}
		})
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
