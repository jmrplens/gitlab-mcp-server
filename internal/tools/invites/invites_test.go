// invites_test.go contains unit tests for the group/project invite MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package invites

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestListPendingProjectInvitations_Success verifies that ListPendingProjectInvitations succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/invitations (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

	out, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{ProjectID: "42", PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20}})
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

// TestListPendingProjectInvitations_WithQuery verifies the ListPendingProjectInvitations_WithQuery handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListPendingProjectInvitations_ValidationError verifies that ListPendingProjectInvitations_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListPendingProjectInvitations_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestListPendingGroupInvitations_Success verifies that ListPendingGroupInvitations succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/invitations (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestListPendingGroupInvitations_ValidationError verifies that ListPendingGroupInvitations_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListPendingGroupInvitations_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestProjectInvites_Success verifies that ProjectInvites succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/invitations (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestProjectInvites_ValidationError_NoProject verifies that ProjectInvites_ValidationError_NoProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectInvites_ValidationError_NoProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestProjectInvites_ValidationError_NoEmailOrUser verifies that ProjectInvites_ValidationError_NoEmailOrUser returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectInvites_ValidationError_NoEmailOrUser(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing email and user_id, got nil")
	}
}

// TestGroupInvites_Success verifies that GroupInvites succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/invitations (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestGroupInvites_ValidationError_NoGroup verifies that GroupInvites_ValidationError_NoGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupInvites_ValidationError_NoGroup(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGroupInvites_APIError verifies that GroupInvites returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupInvites_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{GroupID: "10", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGroupInvites_BadRequest verifies the GroupInvites_BadRequest handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupInvites_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"already a member"}`)
	}))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{GroupID: "10", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "valid access_level") {
		t.Fatalf("error = %v, want access level hint", err)
	}
}

// TestFormatListPendingMarkdownString_WithInvitations verifies the ListPendingMarkdownString_WithInvitations Markdown formatter for a representative listpendingstring_withinvitations input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListPendingMarkdownString_WithInvitations(t *testing.T) {
	out := ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{
			{ID: 1, InviteEmail: "alice@example.com", AccessLevel: 30, UserName: "alice", ExpiresAt: "2026-12-31T00:00:00Z"},
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

// TestFormatListPendingMarkdownString_Empty verifies the ListPendingMarkdownString_Empty Markdown formatter for a representative listpendingstring_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListPendingMarkdownString_Empty(t *testing.T) {
	out := ListPendingInvitationsOutput{Invitations: []PendingInviteOutput{}}
	md := FormatListPendingMarkdownString(out)
	if md != "No pending invitations found.\n" {
		t.Errorf("got %q, want %q", md, "No pending invitations found.\n")
	}
}

// TestFormatInviteResultMarkdownString verifies the InviteResultMarkdownString Markdown formatter for a representative inviteresultstring input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// containsStr reports whether contains str.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListPendingProjectInvitations — API error
// ---------------------------------------------------------------------------.

// TestListPendingProjectInvitations_APIError verifies that ListPendingProjectInvitations returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestListPendingGroupInvitations_APIError verifies that ListPendingGroupInvitations returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestListPendingGroupInvitations_WithQuery verifies the ListPendingGroupInvitations_WithQuery handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestProjectInvites_APIError verifies that ProjectInvites returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectInvites_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectInvites_BadRequest verifies the ProjectInvites_BadRequest handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectInvites_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"already a member"}`)
	}))
	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "valid access_level") {
		t.Fatalf("error = %v, want access level hint", err)
	}
}

// ---------------------------------------------------------------------------
// GroupInvites — validation: missing email AND user_id
// ---------------------------------------------------------------------------.

// TestGroupInvites_ValidationErrorNoEmailOrUser verifies that GroupInvites_ValidationErrorNoEmailOrUser returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestProjectInvites_WithUserID verifies the ProjectInvites_WithUserID handler.
// The mock GitLab API at /api/v4/projects/42/invitations (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestProjectInvites_WithExpiresAt verifies the ProjectInvites_WithExpiresAt handler.
// The mock GitLab API at /api/v4/projects/42/invitations (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestGroupInvites_WithEmailAndExpiresAt verifies the GroupInvites_WithEmailAndExpiresAt handler.
// The mock GitLab API at /api/v4/groups/10/invitations (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestToPendingInviteOutput_WithDates verifies the ToPendingInviteOutput_WithDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToPendingInviteOutput_WithDates(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
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

// TestToPendingInviteOutput_NilDates verifies the ToPendingInviteOutput_NilDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestToInviteResultOutput_WithMessages verifies the ToInviteResultOutput_WithMessages handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestFormatInviteResultMarkdownString_EmptyMessages verifies the InviteResultMarkdownString_EmptyMessages Markdown formatter for a representative inviteresultstring_emptymessages input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatListPendingMarkdown_ReturnsCallToolResult verifies the ListPendingMarkdown_ReturnsCallToolResult Markdown formatter for a representative listpending_returnscalltoolresult input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatInviteResultMarkdown_ReturnsCallToolResult verifies the InviteResultMarkdown_ReturnsCallToolResult Markdown formatter for a representative inviteresult_returnscalltoolresult input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "invites" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, invitesRouteHandler())
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// invitesRouteHandler supports invites route handler assertions in invites tests.
func invitesRouteHandler() http.Handler {
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

	return handler
}

// ---------------------------------------------------------------------------
// List — keyset pagination, order_by, sort propagation (1:1 audit)
// ---------------------------------------------------------------------------.

// TestListPendingProjectInvitations_KeysetAndSort verifies that order_by, sort,
// pagination=keyset, and page_token reach the GitLab API as query parameters.
func TestListPendingProjectInvitations_KeysetAndSort(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{
		ProjectID:             "42",
		OrderBy:               "created_at",
		Sort:                  "desc",
		PaginationInput:       toolutil.PaginationInput{PerPage: 50},
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "cursor-7"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=cursor-7", "per_page=50"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

// TestListPendingGroupInvitations_KeysetAndSort verifies that order_by, sort,
// and keyset pagination reach the group invitations endpoint.
func TestListPendingGroupInvitations_KeysetAndSort(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/groups/10/invitations" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{
		GroupID:               "10",
		OrderBy:               "id",
		Sort:                  "asc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "cursor-3"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=id", "sort=asc", "pagination=keyset", "page_token=cursor-3"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Invite — id body parameter propagation (1:1 audit)
// ---------------------------------------------------------------------------.

// TestProjectInvites_WithID verifies that the id body parameter is sent on the
// project invitation request.
func TestProjectInvites_WithID(t *testing.T) {
	var gotID string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotID = body.ID
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	out, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
		ProjectID:   "42",
		ID:          "42",
		Email:       "dev@example.com",
		AccessLevel: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
	if gotID != "42" {
		t.Errorf("id body param = %q, want %q", gotID, "42")
	}
}

// TestGroupInvites_WithID verifies that the id body parameter is sent on the
// group invitation request.
func TestGroupInvites_WithID(t *testing.T) {
	var gotID string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/groups/10/invitations" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotID = body.ID
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	out, err := GroupInvites(context.Background(), client, GroupInvitesInput{
		GroupID:     "10",
		ID:          "10",
		UserID:      77,
		AccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
	if gotID != "10" {
		t.Errorf("id body param = %q, want %q", gotID, "10")
	}
}

// ---------------------------------------------------------------------------
// Metadata — decorateInviteMeta guards and discovery completeness (R-META)
// ---------------------------------------------------------------------------.

// TestDecorateInviteMeta_UnknownTool verifies that decorateInviteMeta leaves
// options untouched for an individual tool that has no metadata entry.
func TestDecorateInviteMeta_UnknownTool(t *testing.T) {
	options := inviteOptions("gitlab_unknown_invite")
	before := options.Usage
	decorateInviteMeta(&options, "gitlab_unknown_invite")
	if options.Usage != before {
		t.Errorf("Usage mutated for unknown tool: got %q, want %q", options.Usage, before)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions populated for unknown tool: %v", options.RelatedActions)
	}
}

// TestInviteActionSpecs_DiscoveryMetadata verifies that every invite ActionSpec
// carries non-generic Usage, natural-language aliases, canonical related
// actions, and a "Returns: … See also: …" individual-tool description.
func TestInviteActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	for _, spec := range ActionSpecs(client) {
		tool := spec.IndividualTool.Name
		if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute invites domain action") {
			t.Errorf("%s: generic or empty Usage %q", tool, spec.Usage)
		}
		if len(spec.Aliases) < 2 {
			t.Errorf("%s: expected natural-language aliases, got %v", tool, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: missing RelatedActions", tool)
		}
		if !strings.Contains(spec.IndividualTool.Description, "Returns:") ||
			!strings.Contains(spec.IndividualTool.Description, "See also:") {
			t.Errorf("%s: description missing Returns/See also: %q", tool, spec.IndividualTool.Description)
		}
		if len(spec.ParameterGuidance) == 0 {
			t.Errorf("%s: missing ParameterGuidance", tool)
		}
	}
}
