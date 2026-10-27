// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// user_admin_test.go contains unit tests for GitLab user administration
// operations. Tests use httptest to mock the GitLab Users API.
package users

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestBlockUser_Success verifies BlockUser returns Success=true and Action="blocked"
// when POST /users/:id/block responds 201 Created.
func TestBlockUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/users/42/block" {
			testutil.RespondJSON(w, http.StatusCreated, "true")
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
			testutil.RespondJSON(w, http.StatusCreated, "true")
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
			testutil.RespondJSON(w, http.StatusCreated, "true")
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

// TestFormatAdminActionMarkdownString verifies the whole admin-action card for
// a successful action.
func TestFormatAdminActionMarkdownString(t *testing.T) {
	assertMarkdown(t, FormatAdminActionMarkdownString(AdminActionOutput{UserID: 42, Action: "block", Success: true}),
		"## User Admin Action\n\n"+
			"- **ID**: 42\n"+
			"- **Action**: block\n"+
			"- **Success**: ✅\n")
}

// TestStateChanges_ReportWhatGitLabAnswered verifies block, unblock and
// deactivate report what GitLab's answer says. GitLab answers all three with
// 201 whether or not it changed anything, and says which in the body: true
// when the state changed, false or null when it did not (an unblock of an
// active user answers false, a block of a blocked user and a second
// deactivate answer null). client-go reads the status alone (upstream-bugs
// row 117), so each of those used to read as done.
func TestStateChanges_ReportWhatGitLabAnswered(t *testing.T) {
	actions := []stateChangeCase{
		{"BlockUser", BlockUser, "/api/v4/users/42/block", "already blocked"},
		{"UnblockUser", UnblockUser, "/api/v4/users/42/unblock", "not blocked"},
		{"DeactivateUser", DeactivateUser, "/api/v4/users/42/deactivate", "already deactivated"},
	}
	for _, action := range actions {
		for _, answer := range []string{"true", "false", "null"} {
			t.Run(action.name+"_"+answer, func(t *testing.T) {
				assertStateChangeAnswer(t, action, answer)
			})
		}
	}
}

// stateChangeCase is one state change whose answer GitLab writes as a bool,
// and what a user it changed nothing for usually is.
type stateChangeCase struct {
	name      string
	fn        func(context.Context, *gitlabclient.Client, AdminActionInput) (AdminActionOutput, error)
	path      string
	unchanged string
}

// assertStateChangeAnswer answers the action's route with answer and holds
// the output and its card to what that answer means.
func assertStateChangeAnswer(t *testing.T, action stateChangeCase, answer string) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == action.path {
			testutil.RespondJSON(w, http.StatusCreated, answer)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := action.fn(context.Background(), client, AdminActionInput{UserID: 42})
	if err != nil {
		t.Fatalf("%s() unexpected error: %v", action.name, err)
	}
	if answer == "true" {
		if !out.Success || out.Note != "" {
			t.Errorf("%s() on true = %+v, want success and no note", action.name, out)
		}
		return
	}
	if out.Success || !strings.Contains(out.Note, "GitLab answered "+answer) || !strings.Contains(out.Note, action.unchanged) {
		t.Errorf("%s() on %s = %+v, want no success and a note saying GitLab answered %s for a user who is %s",
			action.name, answer, out, answer, action.unchanged)
	}
	if md := FormatAdminActionMarkdownString(out); !strings.Contains(md, out.Note) {
		t.Errorf("the card of an unchanged %s does not say so:\n%s", action.name, md)
	}
}

// TestStateChangeOutput_UnreadableAnswer_IsAnError verifies the reader of
// GitLab's answer reports a body it cannot read as a bool, and a request that
// recorded no answer, rather than calling either a success. The transport
// hands it a body client-go decoded without complaint, so only a direct call
// reaches these.
func TestStateChangeOutput_UnreadableAnswer_IsAnError(t *testing.T) {
	if _, err := stateChangeOutput(gitlabclient.CapturedBody([]byte(`{"message":"201 Created"}`)), "block_user", 42, "blocked", "already blocked"); err == nil ||
		!strings.Contains(err.Error(), "block_user") {
		t.Errorf("stateChangeOutput(object) error = %v, want the operation's refusal to read it", err)
	}
	var untouched gitlabclient.ResponseCapture
	if _, err := stateChangeOutput(&untouched, "block_user", 42, "blocked", "already blocked"); !errors.Is(err, gitlabclient.ErrNoResponseCaptured) {
		t.Errorf("stateChangeOutput(nothing captured) error = %v, want %v", err, gitlabclient.ErrNoResponseCaptured)
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
		t.Run(action.name, func(t *testing.T) {
			runAdminActionCases(t, action)
		})
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

// adminActionSuccessHandler answers the action's route with status, and with
// the true GitLab sends for a state change it applied, which block, unblock
// and deactivate read; a 204 carries no body.
func adminActionSuccessHandler(method, path string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == method && r.URL.Path == path {
			if status == http.StatusNoContent {
				w.WriteHeader(status)
				return
			}
			testutil.RespondJSON(w, status, "true")
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

// TestFormatAdminActionMarkdownString_Fields verifies the whole card for both
// outcomes. A refused action used to print the success glyph beside the word
// "false", which reads as a success whatever the word says.
func TestFormatAdminActionMarkdownString_Fields(t *testing.T) {
	tests := []struct {
		name  string
		input AdminActionOutput
		want  string
	}{
		{
			name:  "the action succeeded",
			input: AdminActionOutput{UserID: 99, Action: "banned", Success: true},
			want:  "## User Admin Action\n\n- **ID**: 99\n- **Action**: banned\n- **Success**: ✅\n",
		},
		{
			name:  "the action did not succeed",
			input: AdminActionOutput{UserID: 99, Action: "banned", Success: false},
			want:  "## User Admin Action\n\n- **ID**: 99\n- **Action**: banned\n- **Success**: ❌\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMarkdown(t, FormatAdminActionMarkdownString(tt.input), tt.want)
		})
	}
}
