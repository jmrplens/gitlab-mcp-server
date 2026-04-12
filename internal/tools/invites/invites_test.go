// invites_test.go contains unit tests for the group/project invite MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package invites

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestListPendingProjectInvitations_Success verifies that ListPendingProjectInvitations handles the success scenario correctly.
func TestListPendingProjectInvitations_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"invite_email":"alice@example.com","access_level":30,"user_name":"","created_by_name":"Admin"},
			{"id":2,"invite_email":"bob@example.com","access_level":40,"user_name":"bob","created_by_name":"Admin"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	}))

	out, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{ProjectID: "42", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Invitations) != 2 {
		t.Fatalf("got %d invitations, want 2", len(out.Invitations))
	}
	if out.Invitations[0].InviteEmail != "alice@example.com" {
		t.Errorf("got email %q, want %q", out.Invitations[0].InviteEmail, "alice@example.com")
	}
	if out.Invitations[1].AccessLevel != 40 {
		t.Errorf("got access_level %d, want 40", out.Invitations[1].AccessLevel)
	}
}

// TestListPendingProjectInvitations_WithQuery verifies that ListPendingProjectInvitations handles the with query scenario correctly.
func TestListPendingProjectInvitations_WithQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "alice" {
			t.Errorf("expected query=alice, got %q", r.URL.Query().Get("query"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"invite_email":"alice@example.com","access_level":30}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{ProjectID: "42", Query: "alice"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Invitations) != 1 {
		t.Fatalf("got %d invitations, want 1", len(out.Invitations))
	}
}

// TestListPendingProjectInvitations_ValidationError verifies that ListPendingProjectInvitations handles the validation error scenario correctly.
func TestListPendingProjectInvitations_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestListPendingGroupInvitations_Success verifies that ListPendingGroupInvitations handles the success scenario correctly.
func TestListPendingGroupInvitations_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/10/invitations" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":5,"invite_email":"team@example.com","access_level":20,"created_by_name":"Manager"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Invitations) != 1 {
		t.Fatalf("got %d invitations, want 1", len(out.Invitations))
	}
	if out.Invitations[0].CreatedByName != "Manager" {
		t.Errorf("got created_by %q, want %q", out.Invitations[0].CreatedByName, "Manager")
	}
}

// TestListPendingGroupInvitations_ValidationError verifies that ListPendingGroupInvitations handles the validation error scenario correctly.
func TestListPendingGroupInvitations_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestProjectInvites_Success verifies that ProjectInvites handles the success scenario correctly.
func TestProjectInvites_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/invitations" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))

	out, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", Email: "new@example.com", AccessLevel: 30})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
}

// TestProjectInvites_ValidationError_NoProject verifies that ProjectInvites handles the validation error_ no project scenario correctly.
func TestProjectInvites_ValidationError_NoProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestProjectInvites_ValidationError_NoEmailOrUser verifies that ProjectInvites handles the validation error_ no email or user scenario correctly.
func TestProjectInvites_ValidationError_NoEmailOrUser(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing email and user_id, got nil")
	}
}

// TestGroupInvites_Success verifies that GroupInvites handles the success scenario correctly.
func TestGroupInvites_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/10/invitations" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))

	out, err := GroupInvites(context.Background(), client, GroupInvitesInput{GroupID: "10", UserID: 99, AccessLevel: 40})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
}

// TestGroupInvites_ValidationError_NoGroup verifies that GroupInvites handles the validation error_ no group scenario correctly.
func TestGroupInvites_ValidationError_NoGroup(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGroupInvites_APIError verifies that GroupInvites handles the a p i error scenario correctly.
func TestGroupInvites_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{GroupID: "10", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatListPendingMarkdownString_WithInvitations verifies that FormatListPendingMarkdownString handles the with invitations scenario correctly.
func TestFormatListPendingMarkdownString_WithInvitations(t *testing.T) {
	out := ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{
			{ID: 1, InviteEmail: "alice@example.com", AccessLevel: 30, UserName: "alice", ExpiresAt: "2025-12-31T00:00:00Z"},
			{ID: 2, InviteEmail: "bob@example.com", AccessLevel: 40},
		},
	}
	md := FormatListPendingMarkdownString(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !containsStr(md, "alice@example.com") {
		t.Errorf("markdown missing email: %s", md)
	}
	if !containsStr(md, "Expires:") {
		t.Errorf("markdown missing expiry: %s", md)
	}
}

// TestFormatListPendingMarkdownString_Empty verifies that FormatListPendingMarkdownString handles the empty scenario correctly.
func TestFormatListPendingMarkdownString_Empty(t *testing.T) {
	out := ListPendingInvitationsOutput{Invitations: []PendingInviteOutput{}}
	md := FormatListPendingMarkdownString(out)
	if md != "No pending invitations found.\n" {
		t.Errorf("got %q, want %q", md, "No pending invitations found.\n")
	}
}

// TestFormatInviteResultMarkdownString verifies the behavior of format invite result markdown string.
func TestFormatInviteResultMarkdownString(t *testing.T) {
	out := InviteResultOutput{Status: "success", Message: map[string]string{"alice@example.com": "Invite sent"}}
	md := FormatInviteResultMarkdownString(out)
	if !containsStr(md, "success") {
		t.Errorf("markdown missing status: %s", md)
	}
	if !containsStr(md, "alice@example.com") {
		t.Errorf("markdown missing message key: %s", md)
	}
}

// containsStr is an internal helper for the invites package.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListPendingProjectInvitations — API error
// ---------------------------------------------------------------------------.

// TestListPendingProjectInvitations_APIError verifies the behavior of list pending project invitations a p i error.
func TestListPendingProjectInvitations_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListPendingGroupInvitations — API error
// ---------------------------------------------------------------------------.

// TestListPendingGroupInvitations_APIError verifies the behavior of list pending group invitations a p i error.
func TestListPendingGroupInvitations_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListPendingGroupInvitations — with query filter
// ---------------------------------------------------------------------------.

// TestListPendingGroupInvitations_WithQuery verifies the behavior of list pending group invitations with query.
func TestListPendingGroupInvitations_WithQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "team" {
			t.Errorf("expected query=team, got %q", r.URL.Query().Get("query"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":5,"invite_email":"team@example.com","access_level":20}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))
	out, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{GroupID: "10", Query: "team"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Invitations) != 1 {
		t.Fatalf("got %d invitations, want 1", len(out.Invitations))
	}
}

// ---------------------------------------------------------------------------
// ProjectInvites — API error (403)
// ---------------------------------------------------------------------------.

// TestProjectInvites_APIError verifies the behavior of project invites a p i error.
func TestProjectInvites_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GroupInvites — validation: missing email AND user_id
// ---------------------------------------------------------------------------.

// TestGroupInvites_ValidationErrorNoEmailOrUser verifies the behavior of group invites validation error no email or user.
func TestGroupInvites_ValidationErrorNoEmailOrUser(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{GroupID: "10", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing email and user_id, got nil")
	}
	if !strings.Contains(err.Error(), "either email or user_id is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ProjectInvites — with user_id (exercises opts.UserID path)
// ---------------------------------------------------------------------------.

// TestProjectInvites_WithUserID verifies the behavior of project invites with user i d.
func TestProjectInvites_WithUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	out, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
		ProjectID:   "42",
		UserID:      55,
		AccessLevel: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
}

// ---------------------------------------------------------------------------
// ProjectInvites — with expires_at (exercises date parsing path)
// ---------------------------------------------------------------------------.

// TestProjectInvites_WithExpiresAt verifies the behavior of project invites with expires at.
func TestProjectInvites_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	out, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
		ProjectID:   "42",
		Email:       "dev@example.com",
		AccessLevel: 30,
		ExpiresAt:   "2026-12-31",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
}

// ---------------------------------------------------------------------------
// GroupInvites — with email AND expires_at
// ---------------------------------------------------------------------------.

// TestGroupInvites_WithEmailAndExpiresAt verifies the behavior of group invites with email and expires at.
func TestGroupInvites_WithEmailAndExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/groups/10/invitations" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	out, err := GroupInvites(context.Background(), client, GroupInvitesInput{
		GroupID:     "10",
		Email:       "team@example.com",
		AccessLevel: 30,
		ExpiresAt:   "2026-06-15",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
}

// ---------------------------------------------------------------------------
// toPendingInviteOutput — with dates populated
// ---------------------------------------------------------------------------.

// TestToPendingInviteOutput_WithDates verifies the behavior of to pending invite output with dates.
func TestToPendingInviteOutput_WithDates(t *testing.T) {
	created := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	expires := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	inv := &gl.PendingInvite{
		ID:            10,
		InviteEmail:   "alice@example.com",
		AccessLevel:   gl.DeveloperPermissions,
		UserName:      "alice",
		CreatedByName: "admin",
		CreatedAt:     &created,
		ExpiresAt:     &expires,
	}
	out := toPendingInviteOutput(inv)
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
	if out.UserName != "alice" {
		t.Errorf("UserName = %q, want %q", out.UserName, "alice")
	}
	if out.CreatedByName != "admin" {
		t.Errorf("CreatedByName = %q, want %q", out.CreatedByName, "admin")
	}
}

// ---------------------------------------------------------------------------
// toPendingInviteOutput — with nil dates
// ---------------------------------------------------------------------------.

// TestToPendingInviteOutput_NilDates verifies the behavior of to pending invite output nil dates.
func TestToPendingInviteOutput_NilDates(t *testing.T) {
	inv := &gl.PendingInvite{
		ID:          20,
		InviteEmail: "bob@example.com",
		AccessLevel: gl.ReporterPermissions,
	}
	out := toPendingInviteOutput(inv)
	if out.ID != 20 {
		t.Errorf("ID = %d, want 20", out.ID)
	}
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
	if out.ExpiresAt != "" {
		t.Errorf("expected empty ExpiresAt, got %q", out.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// toInviteResultOutput — direct coverage with message map
// ---------------------------------------------------------------------------.

// TestToInviteResultOutput_WithMessages verifies the behavior of to invite result output with messages.
func TestToInviteResultOutput_WithMessages(t *testing.T) {
	r := &gl.InvitesResult{
		Status: "error",
		Message: map[string]string{
			"alice@example.com": "already a member",
			"bob@example.com":   "invite sent",
		},
	}
	out := toInviteResultOutput(r)
	if out.Status != "error" {
		t.Errorf("Status = %q, want %q", out.Status, "error")
	}
	if len(out.Message) != 2 {
		t.Fatalf("len(Message) = %d, want 2", len(out.Message))
	}
	if out.Message["alice@example.com"] != "already a member" {
		t.Errorf("unexpected message for alice: %q", out.Message["alice@example.com"])
	}
}

// ---------------------------------------------------------------------------
// FormatInviteResultMarkdownString — empty message map
// ---------------------------------------------------------------------------.

// TestFormatInviteResultMarkdownString_EmptyMessages verifies the behavior of format invite result markdown string empty messages.
func TestFormatInviteResultMarkdownString_EmptyMessages(t *testing.T) {
	out := InviteResultOutput{Status: "success", Message: map[string]string{}}
	md := FormatInviteResultMarkdownString(out)
	if !strings.Contains(md, "success") {
		t.Errorf("markdown missing status: %s", md)
	}
	if strings.Contains(md, "Messages") {
		t.Errorf("markdown should not contain Messages section for empty map: %s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListPendingMarkdown — returns *mcp.CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatListPendingMarkdown_ReturnsCallToolResult verifies the behavior of format list pending markdown returns call tool result.
func TestFormatListPendingMarkdown_ReturnsCallToolResult(t *testing.T) {
	out := ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{
			{ID: 1, InviteEmail: "test@example.com", AccessLevel: 30},
		},
	}
	result := FormatListPendingMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty Content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "test@example.com") {
		t.Errorf("expected text to contain email, got: %s", tc.Text)
	}
}

// ---------------------------------------------------------------------------
// FormatInviteResultMarkdown — returns *mcp.CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatInviteResultMarkdown_ReturnsCallToolResult verifies the behavior of format invite result markdown returns call tool result.
func TestFormatInviteResultMarkdown_ReturnsCallToolResult(t *testing.T) {
	out := InviteResultOutput{Status: "success"}
	result := FormatInviteResultMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty Content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "success") {
		t.Errorf("expected text to contain status, got: %s", tc.Text)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 4 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newInvitesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_project_pending", "gitlab_project_invite_list_pending", map[string]any{"project_id": "42"}},
		{"list_group_pending", "gitlab_group_invite_list_pending", map[string]any{"group_id": "10"}},
		{"project_invite", "gitlab_project_invite", map[string]any{"project_id": "42", "email": "test@example.com", "access_level": 30}},
		{"group_invite", "gitlab_group_invite", map[string]any{"group_id": "10", "email": "test@example.com", "access_level": 30}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newInvitesMCPSession is an internal helper for the invites package.
func newInvitesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	invitationJSON := `{"id":1,"invite_email":"test@example.com","access_level":30,"created_by_name":"Admin"}`
	resultJSON := `{"status":"success"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/42/invitations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+invitationJSON+`]`)
	})

	handler.HandleFunc("GET /api/v4/groups/10/invitations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+invitationJSON+`]`)
	})

	handler.HandleFunc("POST /api/v4/projects/42/invitations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, resultJSON)
	})

	handler.HandleFunc("POST /api/v4/groups/10/invitations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, resultJSON)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
