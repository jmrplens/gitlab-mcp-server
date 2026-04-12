// members_test.go contains unit tests for GitLab project member operations.
// Tests use httptest to mock the GitLab API and verify member listing with
// query filters, pagination, access level description mapping, and error
// handling including context cancellation.
package members

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Test endpoint paths and format strings for project member operation tests.
const (
	errNoReachAPI              = "should not reach API"
	pathProjectMembers         = "/api/v4/projects/42/members/all"
	fmtMembersListErr          = "List() unexpected error: %v"
	fmtOutMembers0UsernameWant = "out.Members[0].Username = %q, want %q"
	testProjectID              = "42"
	testFieldUserID            = "user_id"
	testUsername               = "alice"
	fmtErrShouldContain        = "error %q should contain %q"
	fmtOutUsernameWant         = "out.Username = %q, want %q"
)

// TestProjectMembersList_Success verifies that projectMembersList returns all
// members with correct usernames, access levels, and human-readable access
// level descriptions. The mock returns two members with Developer and
// Maintainer permissions.
func TestProjectMembersList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectMembers {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"username":"jdoe","name":"John Doe","state":"active","access_level":30,"web_url":"https://gitlab.example.com/jdoe"},{"id":2,"username":"asmith","name":"Alice Smith","state":"active","access_level":40,"web_url":"https://gitlab.example.com/asmith"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf(fmtMembersListErr, err)
	}
	if len(out.Members) != 2 {
		t.Fatalf("len(out.Members) = %d, want 2", len(out.Members))
	}
	if out.Members[0].Username != "jdoe" {
		t.Errorf(fmtOutMembers0UsernameWant, out.Members[0].Username, "jdoe")
	}
	if out.Members[0].AccessLevel != 30 {
		t.Errorf("out.Members[0].AccessLevel = %d, want 30", out.Members[0].AccessLevel)
	}
	if out.Members[0].AccessLevelDescription != "Developer" {
		t.Errorf("out.Members[0].AccessLevelDescription = %q, want %q", out.Members[0].AccessLevelDescription, "Developer")
	}
	if out.Members[1].AccessLevel != 40 {
		t.Errorf("out.Members[1].AccessLevel = %d, want 40", out.Members[1].AccessLevel)
	}
	if out.Members[1].AccessLevelDescription != "Maintainer" {
		t.Errorf("out.Members[1].AccessLevelDescription = %q, want %q", out.Members[1].AccessLevelDescription, "Maintainer")
	}
	if out.Pagination.TotalItems != 2 {
		t.Errorf("out.Pagination.TotalItems = %d, want 2", out.Pagination.TotalItems)
	}
}

// TestProjectMembersList_WithQuery verifies that projectMembersList forwards
// the query filter parameter to the GitLab API to search members by name
// or username.
func TestProjectMembersList_WithQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectMembers {
			if r.URL.Query().Get("query") != "alice" {
				t.Errorf("expected query param 'alice', got %q", r.URL.Query().Get("query"))
			}
			testutil.RespondJSON(w, http.StatusOK,
				`[{"id":2,"username":"asmith","name":"Alice Smith","state":"active","access_level":40,"web_url":"https://gitlab.example.com/asmith"}]`,
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		Query:     "alice",
	})
	if err != nil {
		t.Fatalf(fmtMembersListErr, err)
	}
	if len(out.Members) != 1 {
		t.Fatalf("len(out.Members) = %d, want 1", len(out.Members))
	}
	if out.Members[0].Username != "asmith" {
		t.Errorf(fmtOutMembers0UsernameWant, out.Members[0].Username, "asmith")
	}
}

// TestProjectMembersList_Empty verifies that projectMembersList returns an
// empty member slice when the GitLab API returns no members.
func TestProjectMembersList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf(fmtMembersListErr, err)
	}
	if len(out.Members) != 0 {
		t.Errorf("len(out.Members) = %d, want 0", len(out.Members))
	}
}

// TestProjectMembersList_APIError verifies that projectMembersList propagates
// a 403 Forbidden error returned by the GitLab API.
func TestProjectMembersList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("List() expected error for 403 response, got nil")
	}
}

// TestProjectMembersList_CancelledContext verifies that projectMembersList
// returns an error immediately when called with an already-canceled context.
func TestProjectMembersList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := List(ctx, client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestAccessLevelDescription_Mapping uses table-driven subtests to verify
// that accessLevelDescription correctly maps GitLab numeric access levels
// (10=Guest through 50=Owner) to their human-readable labels.
func TestAccessLevelDescription_Mapping(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"Guest", `[{"id":1,"username":"u","name":"n","state":"active","access_level":10,"web_url":"u"}]`, "Guest"},
		{"Reporter", `[{"id":1,"username":"u","name":"n","state":"active","access_level":20,"web_url":"u"}]`, "Reporter"},
		{"Developer", `[{"id":1,"username":"u","name":"n","state":"active","access_level":30,"web_url":"u"}]`, "Developer"},
		{"Maintainer", `[{"id":1,"username":"u","name":"n","state":"active","access_level":40,"web_url":"u"}]`, "Maintainer"},
		{"Owner", `[{"id":1,"username":"u","name":"n","state":"active","access_level":50,"web_url":"u"}]`, "Owner"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tc.json)
			}))

			out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
			if err != nil {
				t.Fatalf(fmtMembersListErr, err)
			}
			if out.Members[0].AccessLevelDescription != tc.want {
				t.Errorf("AccessLevelDescription = %q, want %q", out.Members[0].AccessLevelDescription, tc.want)
			}
		})
	}
}

// TestProjectMembersList_WithPagination verifies that projectMembersList
// correctly forwards page and per_page parameters to the API and parses
// the pagination response headers.
func TestProjectMembersList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectMembers {
			if r.URL.Query().Get("page") != "2" {
				t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
			}
			if r.URL.Query().Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %q", r.URL.Query().Get("per_page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":6,"username":"user6","name":"User Six","state":"active","access_level":30,"web_url":"https://gitlab.example.com/user6"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "6", TotalPages: "2", PrevPage: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtMembersListErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("out.Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.PerPage != 5 {
		t.Errorf("out.Pagination.PerPage = %d, want 5", out.Pagination.PerPage)
	}
}

// TestMembersList_RoleAndSeatFields verifies that projectMembersList maps
// MemberRoleName and IsUsingSeat from the API response.
func TestMembersList_RoleAndSeatFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectMembers {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":1,"username":"jdoe","name":"John Doe","state":"active",
				"access_level":40,"web_url":"https://gitlab.example.com/jdoe",
				"member_role":{"name":"Security Lead"},
				"is_using_seat":true
			}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err != nil {
		t.Fatalf(fmtMembersListErr, err)
	}
	if len(out.Members) != 1 {
		t.Fatalf("len(out.Members) = %d, want 1", len(out.Members))
	}
	if out.Members[0].MemberRoleName != "Security Lead" {
		t.Errorf("out.Members[0].MemberRoleName = %q, want %q", out.Members[0].MemberRoleName, "Security Lead")
	}
	if !out.Members[0].IsUsingSeat {
		t.Error("out.Members[0].IsUsingSeat = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------.

const (
	pathProjectMember10 = "/api/v4/projects/42/members/10"
	memberJSON          = `{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice"}`
)

// TestMemberGet_Success verifies the behavior of member get success.
func TestMemberGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectMember10 {
			testutil.RespondJSON(w, http.StatusOK, memberJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, UserID: 10})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Username != testUsername {
		t.Errorf(fmtOutUsernameWant, out.Username, testUsername)
	}
	if out.AccessLevel != 30 {
		t.Errorf("out.AccessLevel = %d, want 30", out.AccessLevel)
	}
}

// TestMemberGet_MissingProjectID verifies the behavior of member get missing project i d.
func TestMemberGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("Get() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetInherited tests
// ---------------------------------------------------------------------------.

// TestMemberGetInherited_Success verifies the behavior of member get inherited success.
func TestMemberGetInherited_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/members/all/10" {
			testutil.RespondJSON(w, http.StatusOK, memberJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetInherited(context.Background(), client, GetInput{ProjectID: testProjectID, UserID: 10})
	if err != nil {
		t.Fatalf("GetInherited() unexpected error: %v", err)
	}
	if out.Username != testUsername {
		t.Errorf(fmtOutUsernameWant, out.Username, testUsername)
	}
}

// TestMemberGetInherited_MissingProjectID verifies the behavior of member get inherited missing project i d.
func TestMemberGetInherited_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetInherited(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("GetInherited() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Add tests
// ---------------------------------------------------------------------------.

// TestMemberAdd_Success verifies the behavior of member add success.
func TestMemberAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/members" {
			testutil.RespondJSON(w, http.StatusCreated, memberJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{ProjectID: testProjectID, UserID: 10, AccessLevel: 30})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.Username != testUsername {
		t.Errorf(fmtOutUsernameWant, out.Username, testUsername)
	}
}

// TestMemberAdd_MissingProjectID verifies the behavior of member add missing project i d.
func TestMemberAdd_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{UserID: 10, AccessLevel: 30})
	if err == nil {
		t.Fatal("Add() expected error for missing project_id, got nil")
	}
}

// TestMemberAdd_MissingUserAndUsername verifies the behavior of member add missing user and username.
func TestMemberAdd_MissingUserAndUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{ProjectID: testProjectID, AccessLevel: 30})
	if err == nil {
		t.Fatal("Add() expected error for missing user_id/username, got nil")
	}
}

// TestMemberAdd_MissingAccessLevel verifies the behavior of member add missing access level.
func TestMemberAdd_MissingAccessLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Add(context.Background(), client, AddInput{ProjectID: testProjectID, UserID: 10})
	if err == nil {
		t.Fatal("Add() expected error for missing access_level, got nil")
	}
}

// ---------------------------------------------------------------------------
// Edit tests
// ---------------------------------------------------------------------------.

// TestMemberEdit_Success verifies the behavior of member edit success.
func TestMemberEdit_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProjectMember10 {
			testutil.RespondJSON(w, http.StatusOK, memberJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Edit(context.Background(), client, EditInput{ProjectID: testProjectID, UserID: 10, AccessLevel: 30})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if out.Username != testUsername {
		t.Errorf(fmtOutUsernameWant, out.Username, testUsername)
	}
}

// TestMemberEdit_MissingProjectID verifies the behavior of member edit missing project i d.
func TestMemberEdit_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Edit(context.Background(), client, EditInput{UserID: 10, AccessLevel: 30})
	if err == nil {
		t.Fatal("Edit() expected error for missing project_id, got nil")
	}
}

// TestMemberEdit_MissingAccessLevel verifies the behavior of member edit missing access level.
func TestMemberEdit_MissingAccessLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Edit(context.Background(), client, EditInput{ProjectID: testProjectID, UserID: 10})
	if err == nil {
		t.Fatal("Edit() expected error for missing access_level, got nil")
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------.

// TestMemberDelete_Success verifies the behavior of member delete success.
func TestMemberDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProjectMember10 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, UserID: 10})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestMemberDelete_MissingProjectID verifies the behavior of member delete missing project i d.
func TestMemberDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{UserID: 10})
	if err == nil {
		t.Fatal("Delete() expected error for missing project_id, got nil")
	}
}

// TestMemberDeleteServer_Error verifies the behavior of member delete server error.
func TestMemberDeleteServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, UserID: 10})
	if err == nil {
		t.Fatal("Delete() expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// UserID validation tests
// ---------------------------------------------------------------------------.

// TestMemberGet_MissingUserID verifies the behavior of member get missing user i d.
func TestMemberGet_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, UserID: 0})
	if err == nil {
		t.Fatal("Get() expected error for missing user_id, got nil")
	}
	if !strings.Contains(err.Error(), testFieldUserID) {
		t.Errorf(fmtErrShouldContain, err.Error(), testFieldUserID)
	}
}

// TestMemberGetInherited_MissingUserID verifies the behavior of member get inherited missing user i d.
func TestMemberGetInherited_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetInherited(context.Background(), client, GetInput{ProjectID: testProjectID, UserID: 0})
	if err == nil {
		t.Fatal("GetInherited() expected error for missing user_id, got nil")
	}
	if !strings.Contains(err.Error(), testFieldUserID) {
		t.Errorf(fmtErrShouldContain, err.Error(), testFieldUserID)
	}
}

// TestMemberAdd_MissingUserID verifies the behavior of member add missing user i d.
func TestMemberAdd_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Add(context.Background(), client, AddInput{ProjectID: testProjectID, UserID: 0, AccessLevel: 30})
	if err == nil {
		t.Fatal("Add() expected error for missing user_id, got nil")
	}
	if !strings.Contains(err.Error(), testFieldUserID) {
		t.Errorf(fmtErrShouldContain, err.Error(), testFieldUserID)
	}
}

// TestMemberEdit_MissingUserID verifies the behavior of member edit missing user i d.
func TestMemberEdit_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Edit(context.Background(), client, EditInput{ProjectID: testProjectID, UserID: 0, AccessLevel: 30})
	if err == nil {
		t.Fatal("Edit() expected error for missing user_id, got nil")
	}
	if !strings.Contains(err.Error(), testFieldUserID) {
		t.Errorf(fmtErrShouldContain, err.Error(), testFieldUserID)
	}
}

// TestMemberDelete_MissingUserID verifies the behavior of member delete missing user i d.
func TestMemberDelete_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, UserID: 0})
	if err == nil {
		t.Fatal("Delete() expected error for missing user_id, got nil")
	}
	if !strings.Contains(err.Error(), testFieldUserID) {
		t.Errorf(fmtErrShouldContain, err.Error(), testFieldUserID)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — missing project_id validation
// ---------------------------------------------------------------------------.

// TestList_MissingProjectID verifies the behavior of list missing project i d.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id, got nil")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %q, want substring %q", err.Error(), "project_id is required")
	}
}

// ---------------------------------------------------------------------------
// AccessLevelDescription — edge cases
// ---------------------------------------------------------------------------.

// TestAccessLevelDescription_NoPermissions verifies the behavior of access level description no permissions.
func TestAccessLevelDescription_NoPermissions(t *testing.T) {
	got := AccessLevelDescription(gl.NoPermissions)
	if got != "No access" {
		t.Errorf("AccessLevelDescription(0) = %q, want %q", got, "No access")
	}
}

// TestAccessLevelDescription_MinimalAccess verifies the behavior of access level description minimal access.
func TestAccessLevelDescription_MinimalAccess(t *testing.T) {
	got := AccessLevelDescription(gl.MinimalAccessPermissions)
	if got != "Minimal access" {
		t.Errorf("AccessLevelDescription(5) = %q, want %q", got, "Minimal access")
	}
}

// TestAccessLevelDescription_Unknown verifies the behavior of access level description unknown.
func TestAccessLevelDescription_Unknown(t *testing.T) {
	got := AccessLevelDescription(gl.AccessLevelValue(999))
	if got != "Unknown" {
		t.Errorf("AccessLevelDescription(999) = %q, want %q", got, "Unknown")
	}
}

// ---------------------------------------------------------------------------
// Get — API error
// ---------------------------------------------------------------------------.

// TestMemberGet_APIError verifies the behavior of member get a p i error.
func TestMemberGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", UserID: 999})
	if err == nil {
		t.Fatal("expected API error for 404, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetInherited — API error
// ---------------------------------------------------------------------------.

// TestMemberGetInherited_APIError verifies the behavior of member get inherited a p i error.
func TestMemberGetInherited_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetInherited(context.Background(), client, GetInput{ProjectID: "42", UserID: 999})
	if err == nil {
		t.Fatal("expected API error for API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Add — username path, ExpiresAt, MemberRoleID, API error
// ---------------------------------------------------------------------------.

// TestMemberAdd_WithUsername verifies the behavior of member add with username.
func TestMemberAdd_WithUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/members" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":20,"username":"bob","name":"Bob","state":"active","access_level":30,"web_url":"https://gitlab.example.com/bob"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{
		ProjectID:   "42",
		Username:    "bob",
		AccessLevel: 30,
	})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.Username != "bob" {
		t.Errorf("out.Username = %q, want %q", out.Username, "bob")
	}
}

// TestMemberAdd_WithExpiresAt verifies the behavior of member add with expires at.
func TestMemberAdd_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/members" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice","expires_at":"2025-06-30"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{
		ProjectID:   "42",
		UserID:      10,
		AccessLevel: 30,
		ExpiresAt:   "2025-06-30",
	})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.ExpiresAt == "" {
		t.Error("out.ExpiresAt is empty, want non-empty")
	}
}

// TestMemberAdd_WithMemberRoleID verifies the behavior of member add with member role i d.
func TestMemberAdd_WithMemberRoleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/members" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice","member_role":{"name":"Custom Role"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{
		ProjectID:    "42",
		UserID:       10,
		AccessLevel:  30,
		MemberRoleID: 5,
	})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.MemberRoleName != "Custom Role" {
		t.Errorf("out.MemberRoleName = %q, want %q", out.MemberRoleName, "Custom Role")
	}
}

// TestMemberAdd_APIError verifies the behavior of member add a p i error.
func TestMemberAdd_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Add(context.Background(), client, AddInput{ProjectID: "42", UserID: 10, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected API error for API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Edit — ExpiresAt, MemberRoleID, API error
// ---------------------------------------------------------------------------.

// TestMemberEdit_WithExpiresAt verifies the behavior of member edit with expires at.
func TestMemberEdit_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/members/10" {
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":10,"username":"alice","name":"Alice","state":"active","access_level":40,"web_url":"https://gitlab.example.com/alice","expires_at":"2025-12-31"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Edit(context.Background(), client, EditInput{
		ProjectID:   "42",
		UserID:      10,
		AccessLevel: 40,
		ExpiresAt:   "2025-12-31",
	})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if out.ExpiresAt == "" {
		t.Error("out.ExpiresAt is empty, want non-empty")
	}
}

// TestMemberEdit_WithMemberRoleID verifies the behavior of member edit with member role i d.
func TestMemberEdit_WithMemberRoleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/members/10" {
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":10,"username":"alice","name":"Alice","state":"active","access_level":40,"web_url":"https://gitlab.example.com/alice","member_role":{"name":"Lead Dev"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Edit(context.Background(), client, EditInput{
		ProjectID:    "42",
		UserID:       10,
		AccessLevel:  40,
		MemberRoleID: 7,
	})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if out.MemberRoleName != "Lead Dev" {
		t.Errorf("out.MemberRoleName = %q, want %q", out.MemberRoleName, "Lead Dev")
	}
}

// TestMemberEdit_APIError verifies the behavior of member edit a p i error.
func TestMemberEdit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Edit(context.Background(), client, EditInput{ProjectID: "42", UserID: 10, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected API error for API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ToOutput — CreatedAt, ExpiresAt paths
// ---------------------------------------------------------------------------.

// TestToOutput_WithCreatedAt verifies the behavior of to output with created at.
func TestToOutput_WithCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/members/10" {
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice","created_at":"2024-01-15T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", UserID: 10})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.CreatedAt == "" {
		t.Error("out.CreatedAt is empty, want non-empty")
	}
	if !strings.Contains(out.CreatedAt, "2024-01-15") {
		t.Errorf("out.CreatedAt = %q, want to contain %q", out.CreatedAt, "2024-01-15")
	}
}

// TestToOutput_WithExpiresAt verifies the behavior of to output with expires at.
func TestToOutput_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/members/10" {
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice","expires_at":"2025-06-30"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", UserID: 10})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ExpiresAt == "" {
		t.Error("out.ExpiresAt is empty, want non-empty")
	}
	if !strings.Contains(out.ExpiresAt, "2025-06-30") {
		t.Errorf("out.ExpiresAt = %q, want to contain %q", out.ExpiresAt, "2025-06-30")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — all optional fields populated
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllOptionalFields validates format markdown all optional fields across multiple scenarios using table-driven subtests.
func TestFormatMarkdown_AllOptionalFields(t *testing.T) {
	out := Output{
		ID:                     10,
		Username:               "alice",
		Name:                   "Alice Smith",
		State:                  "active",
		AccessLevel:            40,
		AccessLevelDescription: "Maintainer",
		WebURL:                 "https://gitlab.example.com/alice",
		Email:                  "alice@example.com",
		MemberRoleName:         "Security Lead",
		ExpiresAt:              "2025-06-30",
		CreatedAt:              "2024-01-15T10:00:00Z",
	}

	md := FormatMarkdown(out)

	checks := []struct {
		name string
		want string
	}{
		{"header", "## Member: alice"},
		{"id", "- **ID**: 10"},
		{"name", "- **Name**: Alice Smith"},
		{"username", "- **Username**: alice"},
		{"state", "- **State**: active"},
		{"access_level", "- **Access Level**: Maintainer (40)"},
		{"web_url", "- **URL**: [https://gitlab.example.com/alice](https://gitlab.example.com/alice)"},
		{"email", "- **Email**: alice@example.com"},
		{"member_role", "- **Member Role**: Security Lead"},
		{"expires_at", "- **Expires At**: 30 Jun 2025"},
		{"created_at", "- **Created**: 15 Jan 2024 10:00 UTC"},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(md, tc.want) {
				t.Errorf("FormatMarkdown missing %q in:\n%s", tc.want, md)
			}
		})
	}
}

// TestFormatMarkdown_NoOptionalFields verifies the behavior of format markdown no optional fields.
func TestFormatMarkdown_NoOptionalFields(t *testing.T) {
	out := Output{
		ID:                     10,
		Username:               "alice",
		Name:                   "Alice",
		State:                  "active",
		AccessLevel:            30,
		AccessLevelDescription: "Developer",
		WebURL:                 "https://gitlab.example.com/alice",
	}

	md := FormatMarkdown(out)

	if strings.Contains(md, "**Email**") {
		t.Error("FormatMarkdown should not contain Email when empty")
	}
	if strings.Contains(md, "**Member Role**") {
		t.Error("FormatMarkdown should not contain Member Role when empty")
	}
	if strings.Contains(md, "**Expires At**") {
		t.Error("FormatMarkdown should not contain Expires At when empty")
	}
	if strings.Contains(md, "**Created**") {
		t.Error("FormatMarkdown should not contain Created when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdownString — direct empty call
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{Members: []Output{}})
	if got != "No members found.\n" {
		t.Errorf("FormatListMarkdownString(empty) = %q, want %q", got, "No members found.\n")
	}
}

// TestFormatListMarkdownString_WithMembers verifies the behavior of format list markdown string with members.
func TestFormatListMarkdownString_WithMembers(t *testing.T) {
	lo := ListOutput{
		Members: []Output{
			{Username: "alice", Name: "Alice", AccessLevelDescription: "Developer", State: "active"},
			{Username: "bob", Name: "Bob", AccessLevelDescription: "Maintainer", State: "active"},
		},
	}
	got := FormatListMarkdownString(lo)
	if !strings.Contains(got, "| alice |") {
		t.Error("FormatListMarkdownString missing alice row")
	}
	if !strings.Contains(got, "| bob |") {
		t.Error("FormatListMarkdownString missing bob row")
	}
	if !strings.Contains(got, "| Username |") {
		t.Error("FormatListMarkdownString missing header row")
	}
}

// TestFormatListMarkdownString_ClickableUsernameLinks verifies that list table
// renders usernames as clickable Markdown links when WebURL is present.
func TestFormatListMarkdownString_ClickableUsernameLinks(t *testing.T) {
	lo := ListOutput{
		Members: []Output{
			{Username: "alice", Name: "Alice", AccessLevelDescription: "Developer",
				State: "active", WebURL: "https://gitlab.example.com/alice"},
		},
	}
	got := FormatListMarkdownString(lo)
	if !strings.Contains(got, "[alice](https://gitlab.example.com/alice)") {
		t.Errorf("expected clickable username link, got:\n%s", got)
	}
}

// TestFormatListMarkdownString_NoLinkWithoutWebURL verifies that usernames
// appear as plain text when WebURL is empty.
func TestFormatListMarkdownString_NoLinkWithoutWebURL(t *testing.T) {
	lo := ListOutput{
		Members: []Output{
			{Username: "bob", Name: "Bob", AccessLevelDescription: "Maintainer", State: "active"},
		},
	}
	got := FormatListMarkdownString(lo)
	if strings.Contains(got, "[bob](") {
		t.Errorf("should not contain link when WebURL is empty, got:\n%s", got)
	}
	if !strings.Contains(got, "bob") {
		t.Errorf("should contain username as plain text, got:\n%s", got)
	}
}

// TestFormatMarkdown_ClickableURL verifies that the detail Markdown renders
// the WebURL as a clickable link in the new format.
func TestFormatMarkdown_ClickableURL(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 10, Username: "alice", Name: "Alice", State: "active",
		AccessLevel: 40, AccessLevelDescription: "Maintainer",
		WebURL: "https://gitlab.example.com/alice",
	})
	if !strings.Contains(md, "[https://gitlab.example.com/alice](https://gitlab.example.com/alice)") {
		t.Errorf("expected clickable URL in detail, got:\n%s", md)
	}
}

// TestFormatMarkdown_NoURLWhenEmpty verifies that no URL line appears when
// WebURL is empty.
func TestFormatMarkdown_NoURLWhenEmpty(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 10, Username: "alice", Name: "Alice", State: "active",
		AccessLevel: 30, AccessLevelDescription: "Developer",
	})
	if strings.Contains(md, "**URL**") {
		t.Errorf("should not contain URL when empty, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — returns *mcp.CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_ReturnsCallToolResult verifies the behavior of format list markdown returns call tool result.
func TestFormatListMarkdown_ReturnsCallToolResult(t *testing.T) {
	lo := ListOutput{
		Members: []Output{
			{Username: "alice", Name: "Alice", AccessLevelDescription: "Developer", State: "active"},
		},
	}
	result := FormatListMarkdown(lo)
	if result == nil {
		t.Fatal("FormatListMarkdown returned nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("FormatListMarkdown returned empty content")
	}
}

// TestFormatListMarkdown_EmptyReturnsCallToolResult verifies the behavior of format list markdown empty returns call tool result.
func TestFormatListMarkdown_EmptyReturnsCallToolResult(t *testing.T) {
	result := FormatListMarkdown(ListOutput{Members: []Output{}})
	if result == nil {
		t.Fatal("FormatListMarkdown returned nil for empty list")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterTools — MCP round-trip for all 6 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_MCPRoundTrip validates register tools m c p round trip across multiple scenarios using table-driven subtests.
func TestRegisterTools_MCPRoundTrip(t *testing.T) {
	session := newMembersMCPSession(t)

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{
			name: "list",
			tool: "gitlab_project_members_list",
			args: map[string]any{"project_id": "42"},
		},
		{
			name: "get",
			tool: "gitlab_project_member_get",
			args: map[string]any{"project_id": "42", "user_id": float64(10)},
		},
		{
			name: "get_inherited",
			tool: "gitlab_project_member_get_inherited",
			args: map[string]any{"project_id": "42", "user_id": float64(10)},
		},
		{
			name: "add",
			tool: "gitlab_project_member_add",
			args: map[string]any{"project_id": "42", "user_id": float64(10), "access_level": float64(30)},
		},
		{
			name: "edit",
			tool: "gitlab_project_member_edit",
			args: map[string]any{"project_id": "42", "user_id": float64(10), "access_level": float64(30)},
		},
		{
			name: "delete",
			tool: "gitlab_project_member_delete",
			args: map[string]any{"project_id": "42", "user_id": float64(10)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.tool,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tc.tool, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tc.tool)
			}
			if result.IsError {
				for _, c := range result.Content {
					if txt, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tc.tool, txt.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tc.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newMembersMCPSession is an internal helper for the members package.
func newMembersMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	memberJSON := `{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice"}`

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/42/members/all/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("GET /api/v4/projects/42/members/all", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+memberJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, memberJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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
