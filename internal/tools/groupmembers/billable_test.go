// billable_test.go contains unit tests for the billable group member MCP tool
// handlers (Enterprise Premium/Ultimate). Tests use httptest to mock GitLab API
// responses and verify success, error, validation, and markdown paths.
package groupmembers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ----------------------------------------------
// ListBillableMembers
// ----------------------------------------------.

// TestListBillableMembers_Success verifies that ListBillableMembers succeeds,
// mirrors the SDK fields 1:1, applies search/sort, and surfaces pagination.
// The test exercises the GET /groups/:id/billable_members path.
func TestListBillableMembers_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != "dev" {
			t.Errorf("search = %q, want dev", got)
		}
		if got := r.URL.Query().Get("sort"); got != "name_asc" {
			t.Errorf("sort = %q, want name_asc", got)
		}
		if got := r.URL.Query().Get("order_by"); got != "last_activity_on" {
			t.Errorf("order_by = %q, want last_activity_on", got)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":10,"username":"dev","name":"Developer","state":"active","avatar_url":"https://gl/a.png","web_url":"https://gl/dev","email":"dev@x.io","last_activity_on":"2026-06-01","membership_type":"group_member","removable":true,"created_at":"2026-01-01T00:00:00Z","is_last_owner":false,"last_login_at":"2026-06-20T08:00:00Z"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListBillableMembers(t.Context(), client, ListBillableMembersInput{GroupID: "5", Search: "dev", Sort: "name_asc", OrderBy: "last_activity_on"})
	if err != nil {
		t.Fatalf("ListBillableMembers error: %v", err)
	}
	if len(out.Members) != 1 {
		t.Fatalf("len(Members) = %d, want 1", len(out.Members))
	}
	m := out.Members[0]
	if m.ID != 10 || m.Username != "dev" || m.Name != "Developer" || m.State != "active" {
		t.Errorf("unexpected member identity: %+v", m)
	}
	if m.Email != "dev@x.io" || m.MembershipType != "group_member" || !m.Removable || m.IsLastOwner {
		t.Errorf("unexpected member fields: %+v", m)
	}
	if m.LastActivityOn != "2026-06-01" {
		t.Errorf("LastActivityOn = %q", m.LastActivityOn)
	}
	if m.CreatedAt == "" || m.LastLoginAt == "" {
		t.Errorf("expected created_at and last_login_at set: %+v", m)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("pagination total = %d, want 1", out.Pagination.TotalItems)
	}
}

// TestListBillableMembers_MissingGroupID verifies validation when group_id is
// empty.
func TestListBillableMembers_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListBillableMembers(t.Context(), client, ListBillableMembersInput{}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestListBillableMembers_APIError verifies the 404 error path is wrapped with
// a useful hint.
func TestListBillableMembers_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	if _, err := ListBillableMembers(t.Context(), client, ListBillableMembersInput{GroupID: "5"}); err == nil {
		t.Fatal("expected error for 404")
	}
}

// ----------------------------------------------
// ListBillableMemberMemberships
// ----------------------------------------------.

// TestListBillableMemberMemberships_Success verifies the memberships list
// mirrors the SDK 1:1 including the access_level sub-object and pagination.
func TestListBillableMemberMemberships_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members/10/memberships", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("order_by"); got != "id" {
			t.Errorf("order_by = %q, want id", got)
		}
		if got := r.URL.Query().Get("sort"); got != "desc" {
			t.Errorf("sort = %q, want desc", got)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":99,"source_id":7,"source_full_name":"Org / Team","source_members_url":"https://gl/groups/team/-/group_members","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-12-31","access_level":{"integer_value":30,"string_value":"Developer"}}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{GroupID: "5", UserID: 10, OrderBy: "id", Sort: "desc"})
	if err != nil {
		t.Fatalf("ListBillableMemberMemberships error: %v", err)
	}
	if len(out.Memberships) != 1 {
		t.Fatalf("len(Memberships) = %d, want 1", len(out.Memberships))
	}
	ms := out.Memberships[0]
	if ms.ID != 99 || ms.SourceID != 7 || ms.SourceFullName != "Org / Team" {
		t.Errorf("unexpected membership: %+v", ms)
	}
	if ms.AccessLevel == nil || ms.AccessLevel.IntegerValue != 30 || ms.AccessLevel.StringValue != "Developer" {
		t.Errorf("unexpected access_level: %+v", ms.AccessLevel)
	}
	if ms.CreatedAt == "" || ms.ExpiresAt == "" {
		t.Errorf("expected created_at and expires_at set: %+v", ms)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("pagination total = %d, want 1", out.Pagination.TotalItems)
	}
}

// TestListBillableMemberMemberships_MissingGroupID verifies validation when
// group_id is empty.
func TestListBillableMemberMemberships_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{UserID: 10}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestListBillableMemberMemberships_MissingUserID verifies validation when
// user_id is zero.
func TestListBillableMemberMemberships_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{GroupID: "5"}); err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

// TestListBillableMemberMemberships_APIError verifies the 404 error path.
func TestListBillableMemberMemberships_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members/10/memberships", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	if _, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{GroupID: "5", UserID: 10}); err == nil {
		t.Fatal("expected error for 404")
	}
}

// ----------------------------------------------
// RemoveBillableMember
// ----------------------------------------------.

// TestRemoveBillableMember_Success verifies the DELETE path succeeds.
func TestRemoveBillableMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10}); err != nil {
		t.Fatalf("RemoveBillableMember error: %v", err)
	}
}

// TestRemoveBillableMember_Output verifies the DeleteOutput wrapper message.
func TestRemoveBillableMember_Output(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := removeBillableMemberOutput(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf("removeBillableMemberOutput error: %v", err)
	}
	if out.Status != "success" || out.Message != "Successfully removed billable group member." {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestRemoveBillableMember_OutputError verifies the DeleteOutput wrapper
// propagates the underlying error (empty group_id) instead of a success.
func TestRemoveBillableMember_OutputError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := removeBillableMemberOutput(t.Context(), client, RemoveBillableMemberInput{UserID: 10}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestRemoveBillableMember_MissingGroupID verifies validation when group_id is
// empty.
func TestRemoveBillableMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{UserID: 10}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestRemoveBillableMember_MissingUserID verifies validation when user_id is
// zero.
func TestRemoveBillableMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5"}); err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

// TestRemoveBillableMember_Forbidden verifies the 403 hint path (last owner).
func TestRemoveBillableMember_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "Owner role") {
		t.Errorf("expected owner-role hint, got: %v", err)
	}
}

// TestRemoveBillableMember_BadRequest verifies the 400 hint path (not
// removable).
func TestRemoveBillableMember_BadRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "removable") {
		t.Errorf("expected removable hint, got: %v", err)
	}
}

// TestRemoveBillableMember_NotFound verifies the 404 error path.
func TestRemoveBillableMember_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10}); err == nil {
		t.Fatal("expected error for 404")
	}
}

// ----------------------------------------------
// Markdown formatters
// ----------------------------------------------.

// TestFormatBillableMembersMarkdown covers the populated and empty paths.
func TestFormatBillableMembersMarkdown(t *testing.T) {
	empty := FormatBillableMembersMarkdown(BillableMembersOutput{})
	if !strings.Contains(empty, "No billable members found.") {
		t.Errorf("empty markdown = %q", empty)
	}
	md := FormatBillableMembersMarkdown(BillableMembersOutput{
		Members: []BillableMemberOutput{{
			ID: 10, Username: "dev", Name: "Developer", State: "active",
			WebURL: "https://gl/dev", MembershipType: "group_member",
			Removable: true, LastActivityOn: "2026-06-01",
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"Billable Group Members", "dev", "group_member", "https://gl/dev"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatBillableMembershipsMarkdown covers the populated and empty paths.
func TestFormatBillableMembershipsMarkdown(t *testing.T) {
	empty := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{})
	if !strings.Contains(empty, "No memberships found") {
		t.Errorf("empty markdown = %q", empty)
	}
	md := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{
		Memberships: []BillableMembershipOutput{{
			ID: 99, SourceID: 7, SourceFullName: "Org / Team",
			SourceMembersURL: "https://gl/groups/team/-/group_members",
			ExpiresAt:        "2026-12-31",
			AccessLevel:      &AccessLevelDetailsOutput{IntegerValue: 30, StringValue: "Developer"},
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"Billable Member Memberships", "Org / Team", "Developer", "30"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}
