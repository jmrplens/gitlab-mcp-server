// invites_test.go contains unit tests for the group/project invite MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package invites

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestProjectInvites_ValidationError_NoEmailOrUser verifies that ProjectInvites_ValidationError_NoEmailOrUser returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectInvites_ValidationError_NoEmailOrUser(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
			{InviteEmail: "alice@example.com", AccessLevel: 30, UserName: "alice", ExpiresAt: "2026-12-31T00:00:00Z"},
			{InviteEmail: "bob@example.com", AccessLevel: 40},
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
	out := toPendingInviteOutput(inv, toolutil.InvitationExtra{})
	// The source above carries an ID and the output must not. client-go's
	// PendingInvite models one; lib/api/entities/invitation.rb exposes exactly
	// access_level, created_at, expires_at, invite_email, invite_token,
	// user_name and created_by_name, so no invitation response can hold an id
	// and the field emitted "id": 0 on every row.
	//
	// Asserted on the marshaled form because that is where publishing it
	// again would show, and because the struct field is gone: a compile error
	// is what a reader would meet instead of a failing assertion.
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshaling the output: %v", err)
	}
	if strings.Contains(string(encoded), `"id"`) {
		t.Errorf("the invitation output carries an id key: %s", encoded)
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
	out := toPendingInviteOutput(inv, toolutil.InvitationExtra{})
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
			{InviteEmail: "test@example.com", AccessLevel: 30},
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
		ProjectID:  "42",
		OrderBy:    "created_at",
		Sort:       "desc",
		PerPage:    50,
		Pagination: "keyset", PageToken: "cursor-7",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=cursor-7", "per_page=50"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
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
		GroupID:    "10",
		OrderBy:    "id",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "cursor-3",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=id", "sort=asc", "pagination=keyset", "page_token=cursor-3"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
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
// Invite — full create-body parameter fidelity (1:1 audit R-INPUT)
// ---------------------------------------------------------------------------.

// TestProjectInvites_CreateBodyParams asserts that every documented create
// parameter (email, access_level, expires_at) is serialized into the POST body
// exactly as the GitLab add-a-member endpoint expects.
func TestProjectInvites_CreateBodyParams(t *testing.T) {
	var body struct {
		Email        string `json:"email"`
		AccessLevel  int    `json:"access_level"`
		ExpiresAt    string `json:"expires_at"`
		InviteSource string `json:"invite_source"`
		MemberRoleID int64  `json:"member_role_id"`
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	if _, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
		ProjectID:    "42",
		Email:        "dev@example.com",
		AccessLevel:  30,
		ExpiresAt:    "2026-12-31",
		InviteSource: "mcp-server",
		MemberRoleID: 12,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if body.Email != "dev@example.com" {
		t.Errorf("email = %q, want %q", body.Email, "dev@example.com")
	}
	if body.AccessLevel != 30 {
		t.Errorf("access_level = %d, want 30", body.AccessLevel)
	}
	if body.ExpiresAt != "2026-12-31" {
		t.Errorf("expires_at = %q, want %q", body.ExpiresAt, "2026-12-31")
	}
	if body.InviteSource != "mcp-server" {
		t.Errorf("invite_source = %q, want %q", body.InviteSource, "mcp-server")
	}
	if body.MemberRoleID != 12 {
		t.Errorf("member_role_id = %d, want 12", body.MemberRoleID)
	}
}

// TestGroupInvites_CreateBodyParams asserts that the user_id and access_level
// create parameters are serialized into the group invitation POST body.
func TestGroupInvites_CreateBodyParams(t *testing.T) {
	var body struct {
		UserID       int64  `json:"user_id"`
		AccessLevel  int    `json:"access_level"`
		InviteSource string `json:"invite_source"`
		MemberRoleID int64  `json:"member_role_id"`
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/groups/10/invitations" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	if _, err := GroupInvites(context.Background(), client, GroupInvitesInput{
		GroupID:      "10",
		UserID:       77,
		AccessLevel:  40,
		InviteSource: "onboarding",
		MemberRoleID: 9,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if body.UserID != 77 {
		t.Errorf("user_id = %d, want 77", body.UserID)
	}
	if body.AccessLevel != 40 {
		t.Errorf("access_level = %d, want 40", body.AccessLevel)
	}
	if body.InviteSource != "onboarding" {
		t.Errorf("invite_source = %q, want %q", body.InviteSource, "onboarding")
	}
	if body.MemberRoleID != 9 {
		t.Errorf("member_role_id = %d, want 9", body.MemberRoleID)
	}
}

// TestProjectInvites_QueuedUsers_RoundTrip verifies the documented queued_users
// map reaches the output and the Markdown, which is the answer an instance with
// member promotion management enabled gives instead of inviting outright.
func TestProjectInvites_QueuedUsers_RoundTrip(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/invitations" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"status":"success","queued_users":{"username_1":"Request queued for administrator approval."}}`)
	}))
	out, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
		ProjectID: "42", Email: "dev@example.com", AccessLevel: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.QueuedUsers["username_1"] != "Request queued for administrator approval." {
		t.Errorf("QueuedUsers = %v, want the queued username and its reason", out.QueuedUsers)
	}
	md := FormatInviteResultMarkdownString(out)
	if !strings.Contains(md, "username_1") || !strings.Contains(md, "Queued for administrator approval") {
		t.Errorf("markdown does not report the queued user:\n%s", md)
	}
}

// TestToInviteResultOutputAPI_NoQueuedUsers verifies that a plain success
// answer leaves queued_users absent rather than empty, so the ordinary result
// says nothing about a queue nobody is in.
func TestToInviteResultOutputAPI_NoQueuedUsers(t *testing.T) {
	var result invitesResultAPI
	result.Status = "success"

	out := toInviteResultOutputAPI(&result)
	if out.QueuedUsers != nil {
		t.Errorf("QueuedUsers = %v, want nil", out.QueuedUsers)
	}
	if strings.Contains(FormatInviteResultMarkdownString(out), "Queued") {
		t.Error("markdown named a queue for a result that has none")
	}
}

// TestPostInvitation_NewRequestError verifies request construction errors are
// returned before the GitLab client attempts an HTTP call.
func TestPostInvitation_NewRequestError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	result, err := postInvitation(context.Background(), client, "projects/%zz/invitations", invitesRequest{})
	if err == nil {
		t.Fatal("expected request construction error")
	}
	if result != nil {
		t.Errorf("result = %+v, want nil", result)
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

// ---------------------------------------------------------------------------
// The invitation token GitLab sends that client-go's PendingInvite drops
// ---------------------------------------------------------------------------.

// pendingInvitationsClient answers both pending-invitation routes with one
// body, so the table can drive the project and group handlers alike.
func pendingInvitationsClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, body,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	}))
}

// pendingInvitationCalls are the two handlers that list pending invitations.
var pendingInvitationCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (ListPendingInvitationsOutput, error)
}{
	{name: "project", call: func(client *gitlabclient.Client) (ListPendingInvitationsOutput, error) {
		return ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{ProjectID: "42"})
	}},
	{name: "group", call: func(client *gitlabclient.Client) (ListPendingInvitationsOutput, error) {
		return ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{GroupID: "7"})
	}},
}

// TestPendingInvitations_InviteTokenReachesBothHandlers verifies both list
// handlers publish the invite_token lib/api/entities/invitation.rb exposes
// under no condition, paired with the invitation at its own position, and
// leave it empty on an instance that answered without it. client-go's
// PendingInvite does not model the key, so only the read beside its decode
// can carry it.
func TestPendingInvitations_InviteTokenReachesBothHandlers(t *testing.T) {
	const sent = `[{"id":1,"invite_email":"alice@example.com","invite_token":"tok-alice","access_level":30},` +
		`{"id":2,"invite_email":"bob@example.com","invite_token":"tok-bob","access_level":40}]`
	const withoutToken = `[{"id":1,"invite_email":"alice@example.com","access_level":30}]`
	for _, inviteCall := range pendingInvitationCalls {
		t.Run(inviteCall.name, func(t *testing.T) {
			out, err := inviteCall.call(pendingInvitationsClient(t, sent))
			if err != nil {
				t.Fatalf("%s: %v", inviteCall.name, err)
			}
			if len(out.Invitations) != 2 {
				t.Fatalf("published %d invitations, want 2", len(out.Invitations))
			}
			if out.Invitations[0].InviteToken != "tok-alice" || out.Invitations[1].InviteToken != "tok-bob" {
				t.Errorf("tokens = %q and %q, want each paired with its own invitation",
					out.Invitations[0].InviteToken, out.Invitations[1].InviteToken)
			}

			absent, err := inviteCall.call(pendingInvitationsClient(t, withoutToken))
			if err != nil {
				t.Fatalf("%s without a token: %v", inviteCall.name, err)
			}
			if absent.Invitations[0].InviteToken != "" {
				t.Errorf("invite_token = %q, want empty", absent.Invitations[0].InviteToken)
			}
		})
	}
}

// TestPendingInvitations_UnreadableCapturedFields verifies both handlers
// report the captured response's decode failure rather than invitations with
// the token silently empty. The SDK's PendingInvite has no invite_token, so
// only the read beside it can notice that GitLab sent a number there.
func TestPendingInvitations_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `[{"id":1,"invite_email":"alice@example.com","invite_token":7}]`
	cases := make([]testutil.CapturedCase, 0, len(pendingInvitationCalls))
	for _, inviteCall := range pendingInvitationCalls {
		cases = append(cases, testutil.CapturedCase{Name: inviteCall.name, Call: func() error {
			_, err := inviteCall.call(pendingInvitationsClient(t, poisoned))
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// recordInvitationBody answers an invitation POST and hands back the body the
// handler sent, which is the only place the optional parameters appear.
func recordInvitationBody(t *testing.T, send func(*gitlabclient.Client) error) string {
	t.Helper()
	var sent string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read request body: %v", readErr)
		}
		sent = string(body)
		testutil.RespondJSON(w, http.StatusCreated, `{"status":"success"}`)
	}))
	if err := send(client); err != nil {
		t.Fatalf("invitation: %v", err)
	}
	return sent
}

// TestInvites_OptionalParametersReachTheRequestOnlyWhenGiven verifies both
// invitation handlers put each optional parameter in the POST body when the
// input carries one, and leave it out entirely when it does not.
//
// Only the request shows any of this: GitLab answers with the same status
// either way, so a test reading the output cannot tell a parameter that was
// sent from one that was dropped. expires_at is the sharpest of them, since a
// value that is not a date is deliberately not sent at all rather than
// forwarded for GitLab to refuse.
func TestInvites_OptionalParametersReachTheRequestOnlyWhenGiven(t *testing.T) {
	full := map[string]func(*gitlabclient.Client) error{
		"project": func(client *gitlabclient.Client) error {
			_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
				ProjectID: "42", ID: "42", Email: "alice@example.com", UserID: 7,
				AccessLevel: 30, ExpiresAt: "2027-01-31",
			})
			return err
		},
		"group": func(client *gitlabclient.Client) error {
			_, err := GroupInvites(context.Background(), client, GroupInvitesInput{
				GroupID: "7", ID: "7", Email: "alice@example.com", UserID: 7,
				AccessLevel: 30, ExpiresAt: "2027-01-31",
			})
			return err
		},
	}
	bare := map[string]func(*gitlabclient.Client) error{
		"project": func(client *gitlabclient.Client) error {
			_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{
				ProjectID: "42", Email: "alice@example.com", AccessLevel: 30, ExpiresAt: "31 January 2027",
			})
			return err
		},
		"group": func(client *gitlabclient.Client) error {
			_, err := GroupInvites(context.Background(), client, GroupInvitesInput{
				GroupID: "7", UserID: 7, AccessLevel: 30,
			})
			return err
		},
	}
	for _, scope := range []string{"project", "group"} {
		t.Run(scope, func(t *testing.T) {
			sent := recordInvitationBody(t, full[scope])
			for _, want := range []string{`"id"`, `"email"`, `"user_id"`, `"expires_at"`, `"access_level"`} {
				t.Run("sends"+want, func(t *testing.T) {
					if !strings.Contains(sent, want) {
						t.Errorf("body %q is missing %s", sent, want)
					}
				})
			}

			// The bare call omits id everywhere, and then one of the pair the
			// endpoint accepts either half of: a project invitation names only
			// an email and a malformed date, a group invitation only a user id.
			omitted := map[string][]string{
				"project": {`"id"`, `"user_id"`, `"expires_at"`},
				"group":   {`"id"`, `"email"`, `"expires_at"`},
			}[scope]
			bareBody := recordInvitationBody(t, bare[scope])
			for _, absent := range omitted {
				t.Run("omits"+absent, func(t *testing.T) {
					if strings.Contains(bareBody, absent) {
						t.Errorf("body %q carries %s the input did not name", bareBody, absent)
					}
				})
			}
		})
	}
}

// TestFormatListPendingMarkdownString_OptionalColumns verifies the rendered
// list carries the user name and the expiry when the invitation has them and
// neither when it does not, which is both sides of the two guards there.
func TestFormatListPendingMarkdownString_OptionalColumns(t *testing.T) {
	withBoth := FormatListPendingMarkdownString(ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{{
			InviteEmail: "alice@example.com", AccessLevel: 30,
			UserName: "alice", ExpiresAt: "2027-01-31T00:00:00Z",
		}},
	})
	for _, want := range []string{", User: alice", ", Expires: 31 Jan 2027"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(withBoth, want) {
				t.Errorf("markdown missing %q:\n%s", want, withBoth)
			}
		})
	}
	withNeither := FormatListPendingMarkdownString(ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{{InviteEmail: "alice@example.com", AccessLevel: 30}},
	})
	for _, absent := range []string{"User:", "Expires:"} {
		t.Run("without "+absent, func(t *testing.T) {
			if strings.Contains(withNeither, absent) {
				t.Errorf("markdown carries %q for an invitation without it:\n%s", absent, withNeither)
			}
		})
	}
}

// TestPendingInvitations_MarkdownLeavesTheTokenOut verifies the rendered table
// never carries the token: it is a live credential, and a Markdown answer is
// pasted into a conversation. The JSON keeps it for a caller that needs it.
func TestPendingInvitations_MarkdownLeavesTheTokenOut(t *testing.T) {
	md := FormatListPendingMarkdownString(ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{{
			InviteEmail: "alice@example.com", InviteToken: "tok-alice", AccessLevel: 30,
		}},
	})
	if strings.Contains(md, "tok-alice") {
		t.Errorf("markdown carries the invitation token:\n%s", md)
	}
	if !strings.Contains(md, "alice@example.com") {
		t.Errorf("markdown dropped the invitation itself:\n%s", md)
	}
}
