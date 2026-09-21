// invites_test.go contains unit tests for the group/project invite MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package invites

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
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

// TestListPendingProjectInvitations_WithQuery verifies that the query filter
// reaches GitLab as the query parameter of that name, and that the page it
// answers with is published.
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

// TestListPendingProjectInvitations_ValidationError verifies that a call with
// no project_id is refused by the handler itself: the mock is
// [testutil.ForbiddenHandler], which fails the test if any request reaches it,
// so the error can only be the handler's own.
// What the refusal says is asserted by
// [TestInvites_MissingScope_NamesTheOperationAndTheFieldOfItsOwnHandler].
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

// TestListPendingGroupInvitations_ValidationError verifies that a call with no
// group_id is refused by the handler itself, on the terms
// [TestListPendingProjectInvitations_ValidationError] records.
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

// TestProjectInvites_ValidationError_NoProject verifies that an invitation with
// no project_id is refused before anything is sent: the mock fails the test if
// a request arrives, so no POST was built.
func TestProjectInvites_ValidationError_NoProject(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestProjectInvites_ValidationError_NoEmailOrUser verifies that an invitation
// naming neither an email nor a user_id is refused before anything is sent.
// The message is asserted by the group half,
// [TestGroupInvites_ValidationErrorNoEmailOrUser], which shares this branch.
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

// TestGroupInvites_ValidationError_NoGroup verifies that an invitation with no
// group_id is refused before anything is sent, on the terms
// [TestProjectInvites_ValidationError_NoProject] records.
func TestGroupInvites_ValidationError_NoGroup(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGroupInvites_APIError verifies that GroupInvites returns an error when
// the instance refuses the invitation POST with 403. What the refusal tells the
// caller is asserted by [TestInvites_Refusal_NamesTheScopeItWasCalledOn].
func TestGroupInvites_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GroupInvites(context.Background(), client, GroupInvitesInput{GroupID: "10", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGroupInvites_BadRequest verifies that a 400 from the invitation POST is
// answered with the access-level hint rather than the forbidden one, which is
// the other branch of the same refusal.
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

// pendingListHeader is the header and delimiter of the pending-invitations
// table.
const pendingListHeader = "| Email | Access Level | User | Invited By | Created | Expires |\n" +
	"| --- | --- | --- | --- | --- | --- |\n"

// pendingListHints is the guidance section a page of pending invitations ends
// with.
const pendingListHints = "\n---\n💡 **Next steps:**\n" +
	"- Manage pending invitations by approving, revoking, or resending them\n"

// inviteResultHints is the guidance section an invitation result ends with.
const inviteResultHints = "\n---\n💡 **Next steps:**\n" +
	"- Check invitation status or resend if the invite was not received\n"

// TestFormatListPendingMarkdownString_WithInvitations verifies the whole table
// a page of pending invitations renders: one row per invitation, the access
// level named as well as numbered, and an empty cell where GitLab sent
// nothing.
func TestFormatListPendingMarkdownString_WithInvitations(t *testing.T) {
	out := ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{
			{InviteEmail: "alice@example.com", AccessLevel: 30, UserName: "alice", ExpiresAt: "2026-12-31T00:00:00Z"},
			{InviteEmail: "bob@example.com", AccessLevel: 40},
		},
	}
	want := "## Pending Invitations (2)\n\n" + pendingListHeader +
		"| alice@example.com | Developer (30) | alice |  |  | 31 Dec 2026 00:00 UTC |\n" +
		"| bob@example.com | Maintainer (40) |  |  |  |  |\n" +
		pendingListHints
	if got := FormatListPendingMarkdownString(out); got != want {
		t.Errorf("FormatListPendingMarkdownString =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatListPendingMarkdownString_Empty verifies that a page with no
// invitations renders the empty message alone, with no table header under it.
func TestFormatListPendingMarkdownString_Empty(t *testing.T) {
	out := ListPendingInvitationsOutput{Invitations: []PendingInviteOutput{}}
	md := FormatListPendingMarkdownString(out)
	if md != "No pending invitations found.\n" {
		t.Errorf("got %q, want %q", md, "No pending invitations found.\n")
	}
}

// TestFormatInviteResultMarkdownString verifies the whole card an invitation
// result renders: the status as a list item, GitLab's per-address messages as
// a collection under a heading of their own, and the guidance last.
func TestFormatInviteResultMarkdownString(t *testing.T) {
	out := InviteResultOutput{Status: "success", Message: map[string]string{"alice@example.com": "Invite sent"}}
	want := "## Invitation Result\n\n" +
		"- **Status**: success\n" +
		"\n### Messages\n\n" +
		"| Invitee | Message |\n| --- | --- |\n" +
		"| alice@example.com | Invite sent |\n" +
		inviteResultHints
	if got := FormatInviteResultMarkdownString(out); got != want {
		t.Errorf("FormatInviteResultMarkdownString =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatInviteResultMarkdownString_MessagesAreOrdered verifies that the
// keyed messages render in key order. Go randomizes map iteration, so the
// unordered rendering this replaced answered two identical calls with two
// different documents.
func TestFormatInviteResultMarkdownString_MessagesAreOrdered(t *testing.T) {
	out := InviteResultOutput{
		Status:  "success",
		Message: map[string]string{"carol@example.com": "third", "alice@example.com": "first", "bob@example.com": "second"},
	}
	want := "## Invitation Result\n\n" +
		"- **Status**: success\n" +
		"\n### Messages\n\n" +
		"| Invitee | Message |\n| --- | --- |\n" +
		"| alice@example.com | first |\n" +
		"| bob@example.com | second |\n" +
		"| carol@example.com | third |\n" +
		inviteResultHints
	for range 8 {
		if got := FormatInviteResultMarkdownString(out); got != want {
			t.Fatalf("FormatInviteResultMarkdownString =\n%q\nwant\n%q", got, want)
		}
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListPendingProjectInvitations — API error
// ---------------------------------------------------------------------------.

// TestListPendingProjectInvitations_APIError verifies that a 403 from the
// project invitations endpoint is returned as an error rather than an empty
// page. The status is not the one the list hint is written for, so the message
// is the wrapper's; [TestInvites_Refusal_NamesTheScopeItWasCalledOn] asserts
// the 404 that carries the hint.
func TestListPendingProjectInvitations_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := ListPendingProjectInvitations(context.Background(), client, ListPendingProjectInvitationsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListPendingGroupInvitations — API error
// ---------------------------------------------------------------------------.

// TestListPendingGroupInvitations_APIError verifies that a 403 from the group
// invitations endpoint is returned as an error, on the terms
// [TestListPendingProjectInvitations_APIError] records.
func TestListPendingGroupInvitations_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := ListPendingGroupInvitations(context.Background(), client, ListPendingGroupInvitationsInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListPendingGroupInvitations — with query filter
// ---------------------------------------------------------------------------.

// TestListPendingGroupInvitations_WithQuery verifies that the query filter
// reaches the group invitations endpoint as the query parameter of that name.
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

// TestProjectInvites_APIError verifies that ProjectInvites returns an error
// when the instance refuses the invitation POST with 403. What the refusal
// tells the caller is asserted by
// [TestInvites_Refusal_NamesTheScopeItWasCalledOn].
func TestProjectInvites_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := ProjectInvites(context.Background(), client, ProjectInvitesInput{ProjectID: "42", Email: "a@b.com", AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectInvites_BadRequest verifies that a 400 from the project
// invitation POST is answered with the access-level hint, the same branch
// [TestGroupInvites_BadRequest] reaches from the group side.
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

// TestGroupInvites_ValidationErrorNoEmailOrUser verifies that an invitation
// naming neither an email nor a user_id is refused by the handler, with the
// message that says either will do. The mock fails the test if a request
// arrives, so the refusal is this server's and not GitLab's.
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

// TestProjectInvites_WithUserID verifies that an invitation naming a user_id
// rather than an email reaches POST /api/v4/projects/42/invitations and is
// reported as accepted.
//
// It asserts the status alone and reads nothing of the request, so it cannot
// tell a handler that forwarded user_id from one that dropped it. The
// parameter itself is held by TestProjectInvites_CreateBodyParams and
// TestInvites_OptionalParametersReachTheRequestOnlyWhenGiven, which decode the
// body the handler sent.
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

// TestProjectInvites_WithExpiresAt verifies that an invitation carrying an
// expiry date reaches POST /api/v4/projects/42/invitations and is reported as
// accepted, which is the path that parses the date.
//
// It asserts the status alone and reads nothing of the request, so a date that
// was parsed and then dropped looks the same from here. Whether expires_at
// reaches the body, and whether an unparseable one is withheld, is held by
// TestProjectInvites_CreateBodyParams and
// TestInvites_OptionalParametersReachTheRequestOnlyWhenGiven.
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

// TestGroupInvites_WithEmailAndExpiresAt verifies that a group invitation
// carrying both an email and an expiry date reaches POST
// /api/v4/groups/10/invitations and is reported as accepted.
//
// It asserts the status alone and reads nothing of the request, so neither
// parameter is held here. Both are held by TestGroupInvites_CreateBodyParams
// and TestInvites_OptionalParametersReachTheRequestOnlyWhenGiven, which decode
// the body the handler sent.
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

// TestToPendingInviteOutput_WithDates verifies that every published field of an
// invitation is filled from the source of its own name, and that the whole
// output is what the invitation and the captured token say and nothing else.
//
// It is asserted as one value against one literal, with no two sources sharing
// a value, because the conversion is straight-line assignment and a converter
// that reads the neighboring field has no branch for either gate to flip. The
// two timestamps were the pair that proved it: the test this replaced checked
// only that each was non-empty, so created_at and expires_at could be
// exchanged with the suite still green, and an invitation would have reported
// itself as expiring on the day it was created.
func TestToPendingInviteOutput_WithDates(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	inv := &gl.PendingInvite{
		ID:            10,
		InviteEmail:   "alice@example.com",
		AccessLevel:   gl.DeveloperPermissions,
		UserName:      "alice-invitee",
		CreatedByName: "admin-inviter",
		CreatedAt:     &created,
		ExpiresAt:     &expires,
	}
	out := toPendingInviteOutput(inv, toolutil.InvitationExtra{InviteToken: "tok-alice"})
	want := PendingInviteOutput{
		InviteEmail:   "alice@example.com",
		InviteToken:   "tok-alice",
		CreatedAt:     "2026-06-01T12:00:00Z",
		AccessLevel:   30,
		ExpiresAt:     "2026-12-31T00:00:00Z",
		UserName:      "alice-invitee",
		CreatedByName: "admin-inviter",
	}
	if out != want {
		t.Errorf("toPendingInviteOutput = %+v, want %+v", out, want)
	}
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
}

// ---------------------------------------------------------------------------
// toPendingInviteOutput — with nil dates
// ---------------------------------------------------------------------------.

// TestToPendingInviteOutput_NilDates verifies that an invitation GitLab sent no
// timestamps for publishes neither of them, and carries the rest unchanged.
// Both dates are pointers on the SDK struct, so each guard has to leave its own
// field alone rather than write a zero time through it.
func TestToPendingInviteOutput_NilDates(t *testing.T) {
	inv := &gl.PendingInvite{
		ID:          20,
		InviteEmail: "bob@example.com",
		AccessLevel: gl.ReporterPermissions,
	}
	out := toPendingInviteOutput(inv, toolutil.InvitationExtra{})
	want := PendingInviteOutput{InviteEmail: "bob@example.com", AccessLevel: 20}
	if out != want {
		t.Errorf("toPendingInviteOutput = %+v, want %+v", out, want)
	}
}

// ---------------------------------------------------------------------------
// toInviteResultOutput — direct coverage with message map
// ---------------------------------------------------------------------------.

// TestToInviteResultOutput_WithMessages verifies that the converter carries
// GitLab's status and its per-address message map through unchanged. No
// request is made: the conversion is the whole of what is under test.
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

// TestFormatInviteResultMarkdownString_EmptyMessages verifies that a result
// whose message map is empty renders no Messages heading at all, rather than a
// heading over a table with no rows.
func TestFormatInviteResultMarkdownString_EmptyMessages(t *testing.T) {
	out := InviteResultOutput{Status: "success", Message: map[string]string{}}
	want := "## Invitation Result\n\n- **Status**: success\n" + inviteResultHints
	if got := FormatInviteResultMarkdownString(out); got != want {
		t.Errorf("FormatInviteResultMarkdownString =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListPendingMarkdown — returns *mcp.CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatListPendingMarkdown_ReturnsCallToolResult verifies that the
// CallToolResult wrapper carries the same document the string formatter
// renders, as a single text content.
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
	want := "## Pending Invitations (1)\n\n" + pendingListHeader +
		"| test@example.com | Developer (30) |  |  |  |  |\n" +
		pendingListHints
	if tc.Text != want {
		t.Errorf("CallToolResult text =\n%q\nwant\n%q", tc.Text, want)
	}
}

// ---------------------------------------------------------------------------
// FormatInviteResultMarkdown — returns *mcp.CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatInviteResultMarkdown_ReturnsCallToolResult verifies that the
// CallToolResult wrapper carries the rendered card as a single text content,
// the status included. The whole document is asserted by
// [TestFormatInviteResultMarkdownString].
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

// TestActionSpecs_Metadata verifies that the package publishes four actions,
// each owned by this package and each projecting an individual tool under some
// name. No request is made: the specs are metadata.
//
// Which name each one projects, and that the four differ, is held by
// TestActionSpecs_EachActionNameReachesTheEndpointItNames instead: a name is
// non-empty here whichever action it was written for.
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

// TestActionSpecs_CallRoutes verifies that each of the four actions is
// reachable through the route the catalog registers, with the arguments a
// caller sends as JSON, and answers rather than failing to decode them.
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

// recordingInvitesRouteHandler answers the four invite endpoints and writes the
// method and path of the last request into got.
//
// The request is the only place a route's identity shows: all four handlers
// answer with a body of the same shape, so a spec wired to the wrong one of
// them returns an indistinguishable result.
func recordingInvitesRouteHandler(got *string) http.Handler {
	next := invitesRouteHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.Method + " " + r.URL.Path
		next.ServeHTTP(w, r)
	})
}

// TestActionSpecs_EachActionNameReachesTheEndpointItNames asserts, for each of
// the four canonical action names, both the individual tool it projects and the
// request its route issues to GitLab.
//
// The catalog joins a spec's Name to its Route by position in one literal, and
// nothing read the two together: TestActionSpecs_CallRoutes indexes by
// IndividualTool.Name and never looks at Name, TestActionSpecs_Metadata reads
// neither, and make check-action-ids answers only whether an ID resolves.
// Crossing the four names over their routes therefore left the suite green
// while invite_list_project resolved to the group handler, which would send a
// project id to /groups. Verified by hand, by exchanging the first arguments of
// the four constructor calls in ActionSpecs, which the suite passed.
func TestActionSpecs_EachActionNameReachesTheEndpointItNames(t *testing.T) {
	cases := []struct {
		action  string
		tool    string
		args    map[string]any
		request string
	}{
		{
			action:  "invite_list_project",
			tool:    "gitlab_project_invite_list_pending",
			args:    map[string]any{"project_id": "42"},
			request: "GET /api/v4/projects/42/invitations",
		},
		{
			action:  "invite_list_group",
			tool:    "gitlab_group_invite_list_pending",
			args:    map[string]any{"group_id": "10"},
			request: "GET /api/v4/groups/10/invitations",
		},
		{
			action:  "invite_project",
			tool:    "gitlab_project_invite",
			args:    map[string]any{"project_id": "42", "email": "test@example.com", "access_level": 30},
			request: "POST /api/v4/projects/42/invitations",
		},
		{
			action:  "invite_group",
			tool:    "gitlab_group_invite",
			args:    map[string]any{"group_id": "10", "email": "test@example.com", "access_level": 30},
			request: "POST /api/v4/groups/10/invitations",
		},
	}

	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			var got string
			client := testutil.NewTestClient(t, recordingInvitesRouteHandler(&got))
			specByName := make(map[string]toolutil.ActionSpec)
			for _, spec := range ActionSpecs(client) {
				specByName[spec.Name] = spec
			}
			spec, ok := specByName[tc.action]
			if !ok {
				t.Fatalf("no ActionSpec is published under the canonical name %q", tc.action)
			}
			if spec.IndividualTool.Name != tc.tool {
				t.Errorf("%s projects the individual tool %q, want %q", tc.action, spec.IndividualTool.Name, tc.tool)
			}
			if _, err := spec.Route.Handler(t.Context(), tc.args); err != nil {
				t.Fatalf("%s: %v", tc.action, err)
			}
			if got != tc.request {
				t.Errorf("%s asked GitLab for %q, want %q", tc.action, got, tc.request)
			}
		})
	}
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
// project invitation request, and that it is the caller's own id rather than
// the project the route was built from.
//
// The two are given different values on purpose. GitLab's add-a-member body
// takes an id of its own beside the path, "usually equal to project_id" and
// not always, and while the fixture gave both the same value a handler
// forwarding the path scope in its place was indistinguishable from one
// forwarding what the caller asked for.
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
		ID:          "77",
		Email:       "dev@example.com",
		AccessLevel: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
	if gotID != "77" {
		t.Errorf("id body param = %q, want %q", gotID, "77")
	}
}

// TestGroupInvites_WithID verifies that the id body parameter is sent on the
// group invitation request, and that it is the caller's own id rather than the
// group the route was built from, for the reason [TestProjectInvites_WithID]
// records.
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
		ID:          "88",
		UserID:      77,
		AccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("got status %q, want %q", out.Status, "success")
	}
	if gotID != "88" {
		t.Errorf("id body param = %q, want %q", gotID, "88")
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
	want := "## Invitation Result\n\n" +
		"- **Status**: success\n" +
		"\n### Queued for Administrator Approval\n\n" +
		"| User | Message |\n| --- | --- |\n" +
		"| username_1 | Request queued for administrator approval. |\n" +
		inviteResultHints
	if got := FormatInviteResultMarkdownString(out); got != want {
		t.Errorf("FormatInviteResultMarkdownString =\n%q\nwant\n%q", got, want)
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

// inviteScopeExpectation is everything one invite tool's discovery metadata has
// to say about the scope it operates on, spelled out so a value borrowed from
// the sibling entry fails rather than satisfying a shape check.
type inviteScopeExpectation struct {
	tool       string
	scopeWord  string
	otherWord  string
	scopeParam string
	otherParam string
	aliases    []string
	related    []string
	scope      toolutil.ParameterGuidance
}

// inviteScopeExpectations pairs each of the four entries of inviteActionMeta
// with the project-or-group scope it belongs to.
var inviteScopeExpectations = []inviteScopeExpectation{
	{
		tool:       "gitlab_project_invite",
		scopeWord:  "project",
		otherWord:  "group",
		scopeParam: "project_id",
		otherParam: "group_id",
		aliases:    []string{"invite user to project", "add user to project by email", "send project invitation"},
		related:    []string{actionInviteListProject, "project.member_add", "access.request_project", "project.members"},
		scope: toolutil.ParameterGuidance{
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path the user is being invited to.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the target project here. Use group_id only with group.invite_group."},
		},
	},
	{
		tool:       "gitlab_group_invite",
		scopeWord:  "group",
		otherWord:  "project",
		scopeParam: "group_id",
		otherParam: "project_id",
		aliases:    []string{"invite user to group", "add user to group by email", "send group invitation"},
		related:    []string{actionInviteListGroup, "group.group_member_add", "access.request_group", "group.members"},
		scope: toolutil.ParameterGuidance{
			SemanticRole:     "scope_group",
			ValueSource:      "Group ID or full group path the user is being invited to.",
			ExampleBinding:   `params.group_id:"platform/backend"`,
			CommonConfusions: []string{"Use group_id for the group scope. Use project_id only with project.invite_project."},
		},
	},
	{
		tool:       "gitlab_project_invite_list_pending",
		scopeWord:  "project",
		otherWord:  "group",
		scopeParam: "project_id",
		otherParam: "group_id",
		aliases:    []string{"list pending project invitations", "show outstanding project invites", "pending project invitations"},
		related:    []string{actionInviteProject, "project.members", "access.request_list_project"},
		scope: toolutil.ParameterGuidance{
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path whose pending invitations should be listed.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Lists pending invitations, not accepted members. Use project.members for current members."},
		},
	},
	{
		tool:       "gitlab_group_invite_list_pending",
		scopeWord:  "group",
		otherWord:  "project",
		scopeParam: "group_id",
		otherParam: "project_id",
		aliases:    []string{"list pending group invitations", "show outstanding group invites", "pending group invitations"},
		related:    []string{actionInviteGroup, "group.members", "access.request_list_group"},
		scope: toolutil.ParameterGuidance{
			SemanticRole:     "scope_group",
			ValueSource:      "Group ID or full group path whose pending invitations should be listed.",
			ExampleBinding:   `params.group_id:"platform/backend"`,
			CommonConfusions: []string{"Lists pending invitations, not accepted members. Use group.members for current members."},
		},
	},
}

// TestInviteActionSpecs_EachToolPublishesItsOwnScope asserts, per individual
// invite tool, the aliases, related actions, scope guidance and scope word its
// entry of inviteActionMeta carries.
//
// TestInviteActionSpecs_DiscoveryMetadata above cannot state any of this: every
// assertion there is shape-only (Usage non-generic, two or more aliases, a
// non-empty related list, "Returns:" and "See also:" in the description,
// non-empty guidance), and the project and group entries satisfy all five
// alike. Verified by hand, by exchanging the two invite entries of
// inviteActionMeta, which the suite passed while gitlab_project_invite
// published the group's usage, the group's aliases, group.group_member_add as a
// related action, and guidance keyed on a group_id its schema does not have.
func TestInviteActionSpecs_EachToolPublishesItsOwnScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	specByTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		specByTool[spec.IndividualTool.Name] = spec
	}

	for _, want := range inviteScopeExpectations {
		t.Run(want.tool, func(t *testing.T) {
			spec, ok := specByTool[want.tool]
			if !ok {
				t.Fatalf("no ActionSpec projects the individual tool %q", want.tool)
			}
			assertNamesOneScope(t, "Usage", spec.Usage, want)
			assertNamesOneScope(t, "description", spec.IndividualTool.Description, want)
			if !slices.Equal(spec.Aliases, want.aliases) {
				t.Errorf("%s aliases = %q, want %q", want.tool, spec.Aliases, want.aliases)
			}
			if !slices.Equal(spec.RelatedActions, want.related) {
				t.Errorf("%s related actions = %q, want %q", want.tool, spec.RelatedActions, want.related)
			}
			if _, present := spec.ParameterGuidance[want.otherParam]; present {
				t.Errorf("%s guides %q, which is the sibling scope and not a parameter of this tool", want.tool, want.otherParam)
			}
			got, present := spec.ParameterGuidance[want.scopeParam]
			if !present {
				t.Fatalf("%s guides no %q", want.tool, want.scopeParam)
			}
			if !reflect.DeepEqual(got, want.scope) {
				t.Errorf("%s guidance for %q =\n%+v\nwant:\n%+v", want.tool, want.scopeParam, got, want.scope)
			}
		})
	}
}

// assertNamesOneScope reports prose that talks about the sibling scope, which
// is what a borrowed metadata entry reads as.
func assertNamesOneScope(t *testing.T, field, text string, want inviteScopeExpectation) {
	t.Helper()
	if !strings.Contains(text, want.scopeWord) {
		t.Errorf("%s %s never says %q: %q", want.tool, field, want.scopeWord, text)
	}
	if strings.Contains(text, want.otherWord) {
		t.Errorf("%s %s says %q, which belongs to the sibling tool: %q", want.tool, field, want.otherWord, text)
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
	t.Run("with both", func(t *testing.T) {
		got := FormatListPendingMarkdownString(ListPendingInvitationsOutput{
			Invitations: []PendingInviteOutput{{
				InviteEmail: "alice@example.com", AccessLevel: 30,
				UserName: "alice", ExpiresAt: "2027-01-31T00:00:00Z",
			}},
		})
		want := "## Pending Invitations (1)\n\n" + pendingListHeader +
			"| alice@example.com | Developer (30) | alice |  |  | 31 Jan 2027 00:00 UTC |\n" +
			pendingListHints
		if got != want {
			t.Errorf("FormatListPendingMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("with neither", func(t *testing.T) {
		got := FormatListPendingMarkdownString(ListPendingInvitationsOutput{
			Invitations: []PendingInviteOutput{{InviteEmail: "alice@example.com", AccessLevel: 30}},
		})
		want := "## Pending Invitations (1)\n\n" + pendingListHeader +
			"| alice@example.com | Developer (30) |  |  |  |  |\n" +
			pendingListHints
		if got != want {
			t.Errorf("FormatListPendingMarkdownString =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestPendingInvitations_MarkdownLeavesTheTokenOut verifies the rendered table
// never carries the token: it is a live credential, and a Markdown answer is
// pasted into a conversation. The JSON keeps it for a caller that needs it.
func TestPendingInvitations_MarkdownLeavesTheTokenOut(t *testing.T) {
	got := FormatListPendingMarkdownString(ListPendingInvitationsOutput{
		Invitations: []PendingInviteOutput{{
			InviteEmail: "alice@example.com", InviteToken: "tok-alice", AccessLevel: 30,
		}},
	})
	want := "## Pending Invitations (1)\n\n" + pendingListHeader +
		"| alice@example.com | Developer (30) |  |  |  |  |\n" +
		pendingListHints
	if got != want {
		t.Errorf("FormatListPendingMarkdownString =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "tok-alice") {
		t.Errorf("markdown carries the invitation token:\n%s", got)
	}
}

// TestPendingInvitations_PaginationComesFromTheResponseHeaders verifies both
// list handlers publish GitLab's paging headers, each field from the header of
// its own name, so a caller holding one page can ask for the next.
//
// Nothing read the block back until now: every list fixture sent the headers
// and every assertion looked only at the invitations, so removing the
// pagination line altogether left the suite green while a model was handed a
// page with no way to tell it was one. The figures are deliberately all
// different, since a header read into the wrong field is a straight-line
// assignment neither gate can see.
func TestPendingInvitations_PaginationComesFromTheResponseHeaders(t *testing.T) {
	headers := testutil.PaginationHeaders{
		Page: "2", PerPage: "20", Total: "57", TotalPages: "3", NextPage: "3", PrevPage: "1",
	}
	for _, inviteCall := range pendingInvitationCalls {
		t.Run(inviteCall.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSONWithPagination(w, http.StatusOK,
					`[{"id":1,"invite_email":"alice@example.com","access_level":30}]`, headers)
			}))
			out, err := inviteCall.call(client)
			if err != nil {
				t.Fatalf("%s: %v", inviteCall.name, err)
			}
			want := toolutil.PaginationOutput{
				Page: 2, PerPage: 20, TotalItems: 57, TotalPages: 3,
				NextPage: 3, PrevPage: 1, HasMore: true,
			}
			if out.Pagination != want {
				t.Errorf("pagination = %+v, want %+v", out.Pagination, want)
			}
		})
	}
}

// TestFormatInviteResultMarkdownString_BothKeyedTables verifies that a result
// carrying GitLab's per-address messages and its queued usernames renders each
// map under its own heading and its own key column.
//
// Each was only ever rendered alone, and the two are the same Go type written
// through the same helper, so nothing held either call to its own map: a
// result would have named the invitees as users waiting for an administrator
// and the queued users as GitLab's replies to the addresses.
func TestFormatInviteResultMarkdownString_BothKeyedTables(t *testing.T) {
	out := InviteResultOutput{
		Status:      "success",
		Message:     map[string]string{"alice@example.com": "Invite sent"},
		QueuedUsers: map[string]string{"bob_user": "Request queued for administrator approval."},
	}
	want := "## Invitation Result\n\n" +
		"- **Status**: success\n" +
		"\n### Messages\n\n" +
		"| Invitee | Message |\n| --- | --- |\n" +
		"| alice@example.com | Invite sent |\n" +
		"\n### Queued for Administrator Approval\n\n" +
		"| User | Message |\n| --- | --- |\n" +
		"| bob_user | Request queued for administrator approval. |\n" +
		inviteResultHints
	if got := FormatInviteResultMarkdownString(out); got != want {
		t.Errorf("FormatInviteResultMarkdownString =\n%q\nwant\n%q", got, want)
	}
}

// TestInvites_MissingScope_NamesTheOperationAndTheFieldOfItsOwnHandler
// verifies that the refusal for a missing scope identifier is attributed to
// the operation the caller asked for and names that handler's own required
// parameter, not the other scope's.
//
// Both are strings passed into a shared helper and neither was read back: the
// four refusals were checked only for being non-nil, so the labels and the
// field names could be exchanged between the project and group handlers with
// the suite still green. A model told "group_id is required" by a tool whose
// schema has no group_id is being sent to a parameter that does not exist, and
// the operation is what the error prefix and the log attribute the failure to.
func TestInvites_MissingScope_NamesTheOperationAndTheFieldOfItsOwnHandler(t *testing.T) {
	cases := []struct {
		name string
		// operation is the label the wrapped error is prefixed with, and field
		// the parameter it has to name; otherField is the sibling scope's,
		// which the same message must not carry.
		operation  string
		field      string
		otherField string
		call       func(context.Context, *gitlabclient.Client) error
	}{
		{
			name: "project list", operation: "project_invite_list_pending",
			field: "project_id", otherField: "group_id",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListPendingProjectInvitations(ctx, c, ListPendingProjectInvitationsInput{})
				return err
			},
		},
		{
			name: "group list", operation: "group_invite_list_pending",
			field: "group_id", otherField: "project_id",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListPendingGroupInvitations(ctx, c, ListPendingGroupInvitationsInput{})
				return err
			},
		},
		{
			name: "project invitation", operation: "project_invite",
			field: "project_id", otherField: "group_id",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ProjectInvites(ctx, c, ProjectInvitesInput{Email: "a@b.com", AccessLevel: 30})
				return err
			},
		},
		{
			name: "group invitation", operation: "group_invite",
			field: "group_id", otherField: "project_id",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := GroupInvites(ctx, c, GroupInvitesInput{Email: "a@b.com", AccessLevel: 30})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The refusal is the handler's own, so the mock must never be
			// reached: an error that came back from GitLab would satisfy every
			// assertion below without the validation existing at all.
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			err := tc.call(context.Background(), client)
			if err == nil {
				t.Fatalf("expected a refusal for the missing %s, got nil", tc.field)
			}
			// Prefix rather than substring, because project_invite is itself a
			// prefix of project_invite_list_pending and a contains test could
			// not tell the list handler's label from the invitation's.
			if !strings.HasPrefix(err.Error(), tc.operation+":") {
				t.Errorf("refusal %q is not attributed to %q", err, tc.operation)
			}
			if !strings.Contains(err.Error(), tc.field+" is required") {
				t.Errorf("refusal %q does not name %q", err, tc.field)
			}
			if strings.Contains(err.Error(), tc.otherField) {
				t.Errorf("refusal %q names %q, which belongs to the other scope", err, tc.otherField)
			}
		})
	}
}

// TestInvites_Refusal_NamesTheScopeItWasCalledOn verifies that each of the four
// refusal hints names the identifier its own handler takes and the role GitLab
// requires at that scope.
//
// Both halves are what a caller acts on. The project handlers take no group_id
// and the group handlers take no project_id, so a hint naming the other one
// sends a model to a parameter that does not exist on the tool it just called.
// And the roles genuinely differ: Maintainer is enough to invite into a
// project, while a group needs Owner, so lending the project wording to the
// group refusal tells a caller their Maintainer membership should have worked
// and leaves them retrying a call GitLab will refuse every time.
//
// Nothing asserted any of this until now. The suite reached both refusal paths
// and only checked that some error came back, which is why the two scopes'
// hints could be swapped, or the whole StatusForbidden branch in runInvitation
// deleted, with every test still green.
func TestInvites_Refusal_NamesTheScopeItWasCalledOn(t *testing.T) {
	cases := []struct {
		name string
		// status is the refusal the mock answers with: the one whose branch
		// carries the hint under test. A list hint is reached through 404 and
		// an invitation hint through 403.
		status int
		call   func(context.Context, *gitlabclient.Client) error
		// wants and rejects are the scope and role words the hint has to carry
		// and has to leave out, rather than the sentence it happens to spell.
		wants   []string
		rejects []string
	}{
		{
			name:   "project list refusal names project_id and the project role",
			status: http.StatusNotFound,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListPendingProjectInvitations(ctx, c, ListPendingProjectInvitationsInput{ProjectID: "42"})
				return err
			},
			wants:   []string{"project_id", "Maintainer"},
			rejects: []string{"group_id"},
		},
		{
			name:   "group list refusal names group_id and demands Owner",
			status: http.StatusNotFound,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListPendingGroupInvitations(ctx, c, ListPendingGroupInvitationsInput{GroupID: "10"})
				return err
			},
			wants:   []string{"group_id", "Owner"},
			rejects: []string{"project_id", "Maintainer"},
		},
		{
			name:   "project invitation refusal names the project and its role",
			status: http.StatusForbidden,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ProjectInvites(ctx, c, ProjectInvitesInput{ProjectID: "42", Email: "a@b.com", AccessLevel: 30})
				return err
			},
			wants:   []string{"project", "Maintainer"},
			rejects: []string{"group"},
		},
		{
			name:   "group invitation refusal names the group and demands Owner",
			status: http.StatusForbidden,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := GroupInvites(ctx, c, GroupInvitesInput{GroupID: "10", Email: "a@b.com", AccessLevel: 30})
				return err
			},
			wants:   []string{"group", "Owner"},
			rejects: []string{"project", "Maintainer"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, `{"message":"refused"}`)
			}))
			err := tc.call(context.Background(), client)
			if err == nil {
				t.Fatalf("expected a refusal from status %d, got nil", tc.status)
			}
			// Only the suggestion is this handler's own: the classification
			// preamble toolutil writes for a 403 mentions both roles whatever
			// scope was called, so asserting over the whole message would be
			// asserting about the wrapper instead of about the hint chosen
			// here. Its absence is itself a failure, since that is what
			// deleting the branch looks like.
			const marker = "Suggestion: "
			_, hint, found := strings.Cut(err.Error(), marker)
			if !found {
				t.Fatalf("refusal %q carries no hint", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(hint, want) {
					t.Errorf("hint %q does not name %q", hint, want)
				}
			}
			for _, reject := range tc.rejects {
				if strings.Contains(hint, reject) {
					t.Errorf("hint %q names %q, which belongs to the other scope", hint, reject)
				}
			}
		})
	}
}
