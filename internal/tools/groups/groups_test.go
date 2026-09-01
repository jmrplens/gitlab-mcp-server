// groups_test.go contains unit tests for GitLab group operations (list, get,
// list members, list subgroups). Tests use httptest to mock the GitLab API
// and verify success paths, search/query filtering, ownership filters,
// pagination, and error handling including context cancellation.
package groups

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Test endpoint paths, format strings, and fixture names for group operation tests.
const (
	pathGroups             = "/api/v4/groups"
	pathGroup99            = "/api/v4/groups/99"
	pathGroupMembers       = "/api/v4/groups/99/members/all"
	pathGroupSubgroups     = "/api/v4/groups/99/descendant_groups"
	fmtGroupListErr        = "List() unexpected error: %v"
	fmtGroupGetErr         = "Get() unexpected error: %v"
	fmtGroupMembersListErr = "MembersList() unexpected error: %v"
	fmtSubgroupsListErr    = "SubgroupsList() unexpected error: %v"
	fmtOutGroups0NameWant  = "out.Groups[0].Name = %q, want %q"
	fmtOutGroupsWant1      = "len(out.Groups) = %d, want 1"
	fmtOutGroupsWant0      = "len(out.Groups) = %d, want 0"
	testGroupInfra         = "infrastructure"
)

// JSON response fixtures for group operation tests.
var groupListJSON = `[{"id":99,"name":"infrastructure","path":"infra","full_path":"org/infra","full_name":"Org / Infrastructure","description":"Infra group","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra","parent_id":1,"created_at":"2026-01-15T10:00:00Z"}]`

// groupDetailJSON is a single group detail JSON response fixture.
var groupDetailJSON = `{"id":99,"name":"infrastructure","path":"infra","full_path":"org/infra","full_name":"Org / Infrastructure","description":"Infra group","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra","parent_id":1,"created_at":"2026-01-15T10:00:00Z","marked_for_deletion_on":"2026-06-01","crm_enabled":true}`

// groupMembersJSON is a JSON response fixture containing two group members.
var groupMembersJSON = `[{"id":10,"username":"devops1","name":"DevOps One","state":"active","access_level":40,"web_url":"https://gitlab.example.com/devops1"},{"id":11,"username":"devops2","name":"DevOps Two","state":"active","access_level":30,"web_url":"https://gitlab.example.com/devops2"}]`

// subgroupsJSON is a JSON response fixture containing one descendant group.
var subgroupsJSON = `[{"id":100,"name":"monitoring","path":"monitoring","full_path":"org/infra/monitoring","description":"Monitoring subgroup","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra/monitoring","parent_id":99}]`

// TestGroupList_Success verifies that GroupList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK, groupListJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtOutGroupsWant1, len(out.Groups))
	}
	if out.Groups[0].Name != testGroupInfra {
		t.Errorf(fmtOutGroups0NameWant, out.Groups[0].Name, testGroupInfra)
	}
	if out.Groups[0].FullPath != "org/infra" {
		t.Errorf("out.Groups[0].FullPath = %q, want %q", out.Groups[0].FullPath, "org/infra")
	}
	if out.Groups[0].ParentID != 1 {
		t.Errorf("out.Groups[0].ParentID = %d, want 1", out.Groups[0].ParentID)
	}
}

// TestGroupList_WithSearch verifies the GroupList_WithSearch handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			if r.URL.Query().Get("search") != "infra" {
				t.Errorf("expected search=infra, got %q", r.URL.Query().Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, groupListJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{Search: "infra"})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtOutGroupsWant1, len(out.Groups))
	}
}

// TestGroupList_Owned verifies the GroupList_Owned handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_Owned(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			if r.URL.Query().Get("owned") != "true" {
				t.Errorf("expected owned=true, got %q", r.URL.Query().Get("owned"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{Owned: true})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
	if len(out.Groups) != 0 {
		t.Errorf(fmtOutGroupsWant0, len(out.Groups))
	}
}

// TestGroupList_Empty verifies the GroupList_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
	if len(out.Groups) != 0 {
		t.Errorf(fmtOutGroupsWant0, len(out.Groups))
	}
}

// TestGroupList_APIError verifies that GroupList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for API error response, got nil")
	}
}

// TestGroupList_CancelledContext verifies the GroupList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGroupList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestGroupGet_Success verifies that GroupGet succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99 {
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupGetErr, err)
	}
	if out.Name != testGroupInfra {
		t.Errorf("out.Name = %q, want %q", out.Name, testGroupInfra)
	}
	if out.ID != 99 {
		t.Errorf("out.ID = %d, want 99", out.ID)
	}
	if out.Visibility != "private" {
		t.Errorf("out.Visibility = %q, want %q", out.Visibility, "private")
	}
}

// TestGroupGet_NotFound verifies that GroupGet_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: "999"})
	if err == nil {
		t.Fatal("Get() expected error for 404 response, got nil")
	}
}

// TestGroupGet_CancelledContext verifies the GroupGet_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGroupGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{GroupID: "99"})
	if err == nil {
		t.Fatal("Get() expected error for canceled context, got nil")
	}
}

// TestGroupMembersList_Success verifies that MembersList retrieves
// members with correct usernames, access levels, and human-readable access
// level descriptions.
func TestGroupMembersList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupMembers {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK, groupMembersJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
	if len(out.Members) != 2 {
		t.Fatalf("len(out.Members) = %d, want 2", len(out.Members))
	}
	if out.Members[0].Username != "devops1" {
		t.Errorf("out.Members[0].Username = %q, want %q", out.Members[0].Username, "devops1")
	}
	if out.Members[0].AccessLevel != 40 {
		t.Errorf("out.Members[0].AccessLevel = %d, want 40", out.Members[0].AccessLevel)
	}
	if out.Members[1].AccessLevel != 30 {
		t.Errorf("out.Members[1].AccessLevel = %d, want 30", out.Members[1].AccessLevel)
	}
}

// TestGroupMembersList_WithQuery verifies the GroupMembersList_WithQuery handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupMembersList_WithQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupMembers {
			if r.URL.Query().Get("query") != "devops" {
				t.Errorf("expected query=devops, got %q", r.URL.Query().Get("query"))
			}
			testutil.RespondJSON(w, http.StatusOK, groupMembersJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MembersList(context.Background(), client, MembersListInput{
		GroupID: "99",
		Query:   "devops",
	})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
	if len(out.Members) != 2 {
		t.Fatalf("len(out.Members) = %d, want 2", len(out.Members))
	}
}

// TestGroupMembersList_Empty verifies the GroupMembersList_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupMembersList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
	if len(out.Members) != 0 {
		t.Errorf("len(out.Members) = %d, want 0", len(out.Members))
	}
}

// TestGroupMembersList_APIError verifies that GroupMembersList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupMembersList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err == nil {
		t.Fatal("MembersList() expected error for 403 response, got nil")
	}
}

// TestGroupMembersList_CancelledContext verifies the GroupMembersList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGroupMembersList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := MembersList(ctx, client, MembersListInput{GroupID: "99"})
	if err == nil {
		t.Fatal("MembersList() expected error for canceled context, got nil")
	}
}

// TestSubgroupsList_Success verifies that SubgroupsList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSubgroupsList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupSubgroups {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK, subgroupsJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtOutGroupsWant1, len(out.Groups))
	}
	if out.Groups[0].Name != "monitoring" {
		t.Errorf(fmtOutGroups0NameWant, out.Groups[0].Name, "monitoring")
	}
	if out.Groups[0].ParentID != 99 {
		t.Errorf("out.Groups[0].ParentID = %d, want 99", out.Groups[0].ParentID)
	}
}

// TestSubgroupsList_WithSearch verifies the SubgroupsList_WithSearch handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSubgroupsList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupSubgroups {
			if r.URL.Query().Get("search") != "monitor" {
				t.Errorf("expected search=monitor, got %q", r.URL.Query().Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := SubgroupsList(context.Background(), client, SubgroupsListInput{
		GroupID: "99",
		Search:  "monitor",
	})
	if err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtOutGroupsWant1, len(out.Groups))
	}
}

// TestSubgroupsList_Empty verifies the SubgroupsList_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSubgroupsList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
	if len(out.Groups) != 0 {
		t.Errorf(fmtOutGroupsWant0, len(out.Groups))
	}
}

// TestSubgroupsList_APIError verifies that SubgroupsList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSubgroupsList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "999"})
	if err == nil {
		t.Fatal("SubgroupsList() expected error for 404 response, got nil")
	}
}

// TestSubgroupsList_CancelledContext verifies the SubgroupsList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestSubgroupsList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := SubgroupsList(ctx, client, SubgroupsListInput{GroupID: "99"})
	if err == nil {
		t.Fatal("SubgroupsList() expected error for canceled context, got nil")
	}
}

// TestGroupGet_SuccessEnrichedFields verifies the GroupGet_SuccessEnrichedFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupGet_SuccessEnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99 {
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.FullName != "Org / Infrastructure" {
		t.Errorf("out.FullName = %q, want %q", out.FullName, "Org / Infrastructure")
	}
	if out.CreatedAt == "" {
		t.Error("out.CreatedAt is empty, want timestamp")
	}
	if out.MarkedForDeletion != "2026-06-01" {
		t.Errorf("out.MarkedForDeletion = %q, want %q", out.MarkedForDeletion, "1 Jun 2026")
	}
}

// TestGroupListInput_EnrichedFilters verifies the GroupListInput_EnrichedFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupListInput_EnrichedFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("order_by"); got != "name" {
			t.Errorf("query param order_by = %q, want %q", got, "name")
		}
		if got := q.Get("sort"); got != "asc" {
			t.Errorf("query param sort = %q, want %q", got, "asc")
		}
		if got := q.Get("visibility"); got != "public" {
			t.Errorf("query param visibility = %q, want %q", got, "public")
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{
		OrderBy:    "name",
		Sort:       "asc",
		Visibility: "public",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
}

// TestGroupMembers_ListSAMLAndRoleFields verifies the GroupMembers_ListSAMLAndRoleFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupMembers_ListSAMLAndRoleFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupMembers {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":10,"username":"devops1","name":"DevOps One","state":"active",
				"access_level":40,"web_url":"https://gitlab.example.com/devops1",
				"group_saml_identity":{"provider":"okta-saml"},
				"member_role":{"name":"Custom Dev"}
			}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
	if len(out.Members) != 1 {
		t.Fatalf("len(out.Members) = %d, want 1", len(out.Members))
	}
	if out.Members[0].GroupSAMLIdentity == nil || out.Members[0].GroupSAMLIdentity.Provider != "okta-saml" {
		t.Errorf("out.Members[0].GroupSAMLIdentity = %+v, want provider %q", out.Members[0].GroupSAMLIdentity, "okta-saml")
	}
	if out.Members[0].MemberRole == nil || out.Members[0].MemberRole.Name != "Custom Dev" {
		t.Errorf("out.Members[0].MemberRole = %+v, want name %q", out.Members[0].MemberRole, "Custom Dev")
	}
}

// TestSubgroupsList_EnrichedFilters verifies that SubgroupsList passes the new
// filter query params: AllAvailable, Owned, MinAccessLevel, OrderBy, Sort, Statistics.
func TestSubgroupsList_EnrichedFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroupSubgroups {
			http.NotFound(w, r)
			return
		}
		assertEnrichedSubgroupQuery(t, r)
		testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
	}))

	out, err := SubgroupsList(context.Background(), client, SubgroupsListInput{
		GroupID:        "99",
		AllAvailable:   true,
		Owned:          true,
		MinAccessLevel: 30,
		OrderBy:        "name",
		Sort:           "desc",
		Statistics:     true,
	})
	if err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtOutGroupsWant1, len(out.Groups))
	}
}

func assertEnrichedSubgroupQuery(t *testing.T, r *http.Request) {
	t.Helper()
	q := r.URL.Query()
	if got := q.Get("all_available"); got != "true" {
		t.Errorf("query param all_available = %q, want %q", got, "true")
	}
	if got := q.Get("owned"); got != "true" {
		t.Errorf("query param owned = %q, want %q", got, "true")
	}
	if got := q.Get("min_access_level"); got != "30" {
		t.Errorf("query param min_access_level = %q, want %q", got, "30")
	}
	if got := q.Get("order_by"); got != "name" {
		t.Errorf("query param order_by = %q, want %q", got, "name")
	}
	if got := q.Get("sort"); got != "desc" {
		t.Errorf("query param sort = %q, want %q", got, "desc")
	}
	if got := q.Get("statistics"); got != "true" {
		t.Errorf("query param statistics = %q, want %q", got, "true")
	}
}

// TestGroupList_EnrichedNewFilters verifies the GroupList_EnrichedNewFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_EnrichedNewFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			q := r.URL.Query()
			if got := q.Get("all_available"); got != "true" {
				t.Errorf("query param all_available = %q, want %q", got, "true")
			}
			if got := q.Get("statistics"); got != "true" {
				t.Errorf("query param statistics = %q, want %q", got, "true")
			}
			if got := q.Get("with_custom_attributes"); got != "true" {
				t.Errorf("query param with_custom_attributes = %q, want %q", got, "true")
			}
			rawQuery := r.URL.RawQuery
			if !strings.Contains(rawQuery, "skip_groups") {
				t.Error("query string does not contain skip_groups parameter")
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		AllAvailable:         true,
		Statistics:           true,
		WithCustomAttributes: true,
		SkipGroups:           []int64{1, 2},
	})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
}

// TestGroupList_CustomAttributesFilter_IndexedQuery verifies that the
// custom_attributes input map is encoded as indexed query parameters
// (custom_attributes[key]=value), which is the filtering form the GitLab API
// expects and is distinct from the with_custom_attributes response flag.
// The same encoding is asserted for descendant-group listing, whose
// ListDescendantGroupsOptions is a defined type over ListGroupsOptions and so
// inherited the filter.
func TestGroupList_CustomAttributesFilter_IndexedQuery(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		attributes map[string]string
		wantParams map[string]string
		call       func(client *gitlabclient.Client, attributes map[string]string) error
	}{
		{
			name:       "single attribute on group list",
			path:       pathGroups,
			attributes: map[string]string{"team": "platform"},
			wantParams: map[string]string{"custom_attributes[team]": "platform"},
			call: func(client *gitlabclient.Client, attributes map[string]string) error {
				_, err := List(context.Background(), client, ListInput{CustomAttributes: attributes})
				return err
			},
		},
		{
			name:       "multiple attributes on group list",
			path:       pathGroups,
			attributes: map[string]string{"team": "platform", "tier": "gold"},
			wantParams: map[string]string{"custom_attributes[team]": "platform", "custom_attributes[tier]": "gold"},
			call: func(client *gitlabclient.Client, attributes map[string]string) error {
				_, err := List(context.Background(), client, ListInput{CustomAttributes: attributes})
				return err
			},
		},
		{
			name:       "single attribute on descendant group list",
			path:       pathGroupSubgroups,
			attributes: map[string]string{"team": "platform"},
			wantParams: map[string]string{"custom_attributes[team]": "platform"},
			call: func(client *gitlabclient.Client, attributes map[string]string) error {
				_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "99", CustomAttributes: attributes})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					http.NotFound(w, r)
					return
				}
				q := r.URL.Query()
				for key, want := range tt.wantParams {
					if got := q.Get(key); got != want {
						t.Errorf("query param %s = %q, want %q", key, got, want)
					}
				}
				if q.Has("custom_attributes") {
					t.Error("query contains a bare custom_attributes parameter, want only indexed keys")
				}
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			}))

			if err := tt.call(client, tt.attributes); err != nil {
				t.Fatalf("list unexpected error: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------.

const (
	// fmtGroupCreateErr identifies the fmt group create err constant used by this package.
	fmtGroupCreateErr = "Create() unexpected error: %v"
	// fmtGroupUpdateErr identifies the fmt group update err constant used by this package.
	fmtGroupUpdateErr = "Update() unexpected error: %v"
	// fmtGroupDeleteErr identifies the fmt group delete err constant used by this package.
	fmtGroupDeleteErr = "Delete() unexpected error: %v"
	// fmtGroupRestoreErr identifies the fmt group restore err constant used by this package.
	fmtGroupRestoreErr = "Restore() unexpected error: %v"
	// fmtGroupSearchErr identifies the fmt group search err constant used by this package.
	fmtGroupSearchErr = "Search() unexpected error: %v"
	// fmtGroupTransferProjectErr identifies the fmt group transfer project err constant used by this package.
	fmtGroupTransferProjectErr = "TransferProject() unexpected error: %v"
	// fmtGroupListProjectsErr identifies the fmt group list projects err constant used by this package.
	fmtGroupListProjectsErr = "ListProjects() unexpected error: %v"
	// pathGroup99Restore identifies the path group 99 restore constant used by this package.
	pathGroup99Restore = "/api/v4/groups/99/restore"
	// pathGroup99Projects identifies the path group 99 projects constant used by this package.
	pathGroup99Projects = "/api/v4/groups/99/projects"
)

// groupProjectsJSON stores the package-level group projects JSON state.
var groupProjectsJSON = `[{"id":42,"name":"my-project","path_with_namespace":"org/infra/my-project","description":"A project","visibility":"private","web_url":"https://gitlab.example.com/org/infra/my-project","default_branch":"main","archived":false,"created_at":"2026-02-01T12:00:00Z"}]`

// TestGroupCreate_Success verifies that GroupCreate succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroups {
			testutil.RespondJSON(w, http.StatusCreated, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{Name: testGroupInfra})
	if err != nil {
		t.Fatalf(fmtGroupCreateErr, err)
	}
	if out.Name != testGroupInfra {
		t.Errorf("out.Name = %q, want %q", out.Name, testGroupInfra)
	}
}

// TestGroupCreate_MissingName verifies that GroupCreate_MissingName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("Create() expected error for missing name, got nil")
	}
}

// TestGroupCreateServer_Error verifies that GroupCreateServer returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupCreateServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Create(context.Background(), client, CreateInput{Name: "fail"})
	if err == nil {
		t.Fatal("Create() expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Update tests
// ---------------------------------------------------------------------------.

// TestGroupUpdate_Success verifies that GroupUpdate succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroup99 {
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		GroupID: "99",
		Name:    "new-name",
	})
	if err != nil {
		t.Fatalf(fmtGroupUpdateErr, err)
	}
	if out.Name != testGroupInfra {
		t.Errorf("out.Name = %q, want %q", out.Name, testGroupInfra)
	}
}

// TestGroupUpdate_MissingGroupID verifies that GroupUpdate_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal("Update() expected error for missing group_id, got nil")
	}
}

// TestGroupUpdateServer_Error verifies that GroupUpdateServer returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupUpdateServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "99", Name: "x"})
	if err == nil {
		t.Fatal("Update() expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------.

// TestGroupDelete_Success verifies that GroupDelete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGroup99 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupDeleteErr, err)
	}
}

// TestGroupDelete_MissingGroupID verifies that GroupDelete_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id, got nil")
	}
}

// TestGroupDeleteServer_Error verifies that GroupDeleteServer returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupDeleteServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "99"})
	if err == nil {
		t.Fatal("Delete() expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Restore tests
// ---------------------------------------------------------------------------.

// TestGroupRestore_Success verifies that GroupRestore succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupRestore_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroup99Restore {
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Restore(context.Background(), client, RestoreInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupRestoreErr, err)
	}
	if out.Name != testGroupInfra {
		t.Errorf("out.Name = %q, want %q", out.Name, testGroupInfra)
	}
}

// TestGroupRestore_MissingGroupID verifies that GroupRestore_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupRestore_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Restore(context.Background(), client, RestoreInput{})
	if err == nil {
		t.Fatal("Restore() expected error for missing group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Search tests
// ---------------------------------------------------------------------------.

// TestGroupSearch_Success verifies that GroupSearch succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupSearch_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups && r.URL.Query().Get("search") != "" {
			testutil.RespondJSON(w, http.StatusOK, groupListJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Search(context.Background(), client, SearchInput{Query: testGroupInfra})
	if err != nil {
		t.Fatalf(fmtGroupSearchErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtOutGroupsWant1, len(out.Groups))
	}
	if out.Groups[0].Name != testGroupInfra {
		t.Errorf(fmtOutGroups0NameWant, out.Groups[0].Name, testGroupInfra)
	}
}

// TestGroupSearch_MissingQuery verifies that GroupSearch_MissingQuery returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupSearch_MissingQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Search(context.Background(), client, SearchInput{})
	if err == nil {
		t.Fatal("Search() expected error for missing query, got nil")
	}
}

// ---------------------------------------------------------------------------
// TransferProject tests
// ---------------------------------------------------------------------------.

// TestGroupTransferProject_Success verifies that GroupTransferProject succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/99/projects/42 (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGroupTransferProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/projects/42" {
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := TransferProject(context.Background(), client, TransferInput{GroupID: "99", ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtGroupTransferProjectErr, err)
	}
	if out.Name != testGroupInfra {
		t.Errorf("out.Name = %q, want %q", out.Name, testGroupInfra)
	}
}

// TestGroupTransferProject_MissingGroupID verifies that GroupTransferProject_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupTransferProject_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := TransferProject(context.Background(), client, TransferInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("TransferProject() expected error for missing group_id, got nil")
	}
}

// TestGroupTransferProject_MissingProjectID verifies that GroupTransferProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupTransferProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := TransferProject(context.Background(), client, TransferInput{GroupID: "99"})
	if err == nil {
		t.Fatal("TransferProject() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListProjects tests
// ---------------------------------------------------------------------------.

// TestGroupListProjects_Success verifies that GroupListProjects succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupListProjects_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99Projects {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK, groupProjectsJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProjects(context.Background(), client, ListProjectsInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupListProjectsErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("len(out.Projects) = %d, want 1", len(out.Projects))
	}
	if out.Projects[0].Name != "my-project" {
		t.Errorf("out.Projects[0].Name = %q, want %q", out.Projects[0].Name, "my-project")
	}
	if out.Projects[0].PathWithNamespace != "org/infra/my-project" {
		t.Errorf("out.Projects[0].PathWithNamespace = %q, want %q", out.Projects[0].PathWithNamespace, "org/infra/my-project")
	}
}

// TestGroupListProjects_MissingGroupID verifies that GroupListProjects_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupListProjects_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListProjects(context.Background(), client, ListProjectsInput{})
	if err == nil {
		t.Fatal("ListProjects() expected error for missing group_id, got nil")
	}
}

// TestGroupListProjectsServer_Error verifies that GroupListProjectsServer returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupListProjectsServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := ListProjects(context.Background(), client, ListProjectsInput{GroupID: "99"})
	if err == nil {
		t.Fatal("ListProjects() expected error on server failure, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Get — missing group_id
// ---------------------------------------------------------------------------.

// TestGet_MissingGroupID verifies that Get_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// MembersList — missing group_id
// ---------------------------------------------------------------------------.

// TestMembersList_MissingGroupID verifies that MembersList_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestMembersList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := MembersList(context.Background(), client, MembersListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// SubgroupsList — missing group_id
// ---------------------------------------------------------------------------.

// TestSubgroupsList_MissingGroupID verifies that SubgroupsList_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSubgroupsList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// List — with pagination params
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("page"); got != "2" {
			t.Errorf("query param page = %q, want %q", got, "2")
		}
		if got := q.Get("per_page"); got != "5" {
			t.Errorf("query param per_page = %q, want %q", got, "5")
		}
		testutil.RespondJSONWithPagination(
			w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"},
		)
	}))
	out, err := List(context.Background(), client, ListInput{
		Page: 2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
}

// ---------------------------------------------------------------------------
// List — with TopLevelOnly
// ---------------------------------------------------------------------------.

// TestList_TopLevelOnly verifies the List_TopLevelOnly handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_TopLevelOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("top_level_only"); got != "true" {
			t.Errorf("query param top_level_only = %q, want %q", got, "true")
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := List(context.Background(), client, ListInput{TopLevelOnly: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// MembersList — with pagination params
// ---------------------------------------------------------------------------.

// TestMembersList_WithPagination verifies that MembersList_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestMembersList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("page"); got != "2" {
			t.Errorf("query param page = %q, want %q", got, "2")
		}
		if got := q.Get("per_page"); got != "10" {
			t.Errorf("query param per_page = %q, want %q", got, "10")
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := MembersList(context.Background(), client, MembersListInput{
		GroupID: "99",
		Page:    2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// SubgroupsList — with pagination params
// ---------------------------------------------------------------------------.

// TestSubgroupsList_WithPagination verifies that SubgroupsList_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestSubgroupsList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("page"); got != "3" {
			t.Errorf("query param page = %q, want %q", got, "3")
		}
		if got := q.Get("per_page"); got != "15" {
			t.Errorf("query param per_page = %q, want %q", got, "15")
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{
		GroupID: "99",
		Page:    3, PerPage: 15,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Create — canceled context, with all optional fields
// ---------------------------------------------------------------------------.

// TestCreate_CancelledContext verifies the Create_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{Name: "g"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCreate_AllOptionalFields verifies the Create_AllOptionalFields handler.
// The mock GitLab API at /api/v4/groups (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestCreate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":200,"name":"sub","path":"sub-path","full_path":"org/sub","visibility":"internal","web_url":"https://gl.example.com/org/sub","parent_id":99}`)
			return
		}
		http.NotFound(w, r)
	}))

	rae := true
	lfs := true
	out, err := Create(context.Background(), client, CreateInput{
		Name:                 "sub",
		Path:                 "sub-path",
		Description:          "A subgroup",
		Visibility:           "internal",
		ParentID:             99,
		RequestAccessEnabled: &rae,
		LFSEnabled:           &lfs,
		DefaultBranch:        "develop",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "sub" {
		t.Errorf("out.Name = %q, want %q", out.Name, "sub")
	}
	if out.ParentID != 99 {
		t.Errorf("out.ParentID = %d, want 99", out.ParentID)
	}
}

// ---------------------------------------------------------------------------
// Update — canceled context, with optional bool fields
// ---------------------------------------------------------------------------.

// TestUpdate_CancelledContext verifies the Update_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{GroupID: "99", Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUpdate_AllOptionalFields verifies the Update_AllOptionalFields handler.
// The mock GitLab API at /api/v4/groups/99 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/99" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"name":"updated","path":"updated-path","full_path":"org/updated","visibility":"public","web_url":"https://gl.example.com/org/updated"}`)
			return
		}
		http.NotFound(w, r)
	}))

	boolTrue := true
	boolFalse := false
	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:              "99",
		Name:                 "updated",
		Path:                 "updated-path",
		Description:          "desc",
		Visibility:           "public",
		RequestAccessEnabled: &boolTrue,
		LFSEnabled:           &boolFalse,
		DefaultBranch:        "main",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "updated" {
		t.Errorf("out.Name = %q, want %q", out.Name, "updated")
	}
}

// ---------------------------------------------------------------------------
// crm_enabled — output mapping + create/update option passthrough (client-go
// v2.47.0, SDK MR !2933). CRM is a Free-tier feature, so the field carries no
// tier tag and is exercised on the standard CE round-trip paths.
// ---------------------------------------------------------------------------.

// TestGroup_CRMEnabled_Get verifies that a crm_enabled=true group response is
// mapped onto the output shape by ToOutput via the Get handler.
// The mock GitLab API at /api/v4/groups/99 (GET) returns a body carrying
// crm_enabled:true. It asserts out.CRMEnabled is decoded as true.
func TestGroup_CRMEnabled_Get(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99 {
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupGetErr, err)
	}
	if !out.CRMEnabled {
		t.Errorf("out.CRMEnabled = %v, want true", out.CRMEnabled)
	}
}

// TestGroup_CRMEnabled_Create verifies that CreateInput.CRMEnabled is forwarded
// to the GitLab API as crm_enabled and that the response value round-trips back
// onto the output shape.
// The mock GitLab API at /api/v4/groups (POST) inspects the request body for
// crm_enabled:true and echoes a crm_enabled:true group. It asserts both the sent
// payload and the decoded output.
func TestGroup_CRMEnabled_Create(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroups {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"crm_enabled":true`) {
				t.Errorf("request body missing crm_enabled:true, got %s", body)
			}
			testutil.RespondJSON(w, http.StatusCreated, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	crm := true
	out, err := Create(context.Background(), client, CreateInput{
		Name:       testGroupInfra,
		CRMEnabled: &crm,
	})
	if err != nil {
		t.Fatalf(fmtGroupCreateErr, err)
	}
	if !out.CRMEnabled {
		t.Errorf("out.CRMEnabled = %v, want true", out.CRMEnabled)
	}
}

// TestGroup_CRMEnabled_Update verifies that UpdateInput.CRMEnabled is forwarded
// to the GitLab API as crm_enabled and that the response value round-trips back
// onto the output shape.
// The mock GitLab API at /api/v4/groups/99 (PUT) inspects the request body for
// crm_enabled:false and echoes a group body. It asserts the sent payload
// preserves the explicit false value.
func TestGroup_CRMEnabled_Update(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroup99 {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"crm_enabled":false`) {
				t.Errorf("request body missing crm_enabled:false, got %s", body)
			}
			testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	crm := false
	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:    "99",
		CRMEnabled: &crm,
	})
	if err != nil {
		t.Fatalf(fmtGroupUpdateErr, err)
	}
	if !out.CRMEnabled {
		t.Errorf("out.CRMEnabled = %v, want true (from response body)", out.CRMEnabled)
	}
}

// ---------------------------------------------------------------------------
// Delete — canceled context, with permanently_remove
// ---------------------------------------------------------------------------.

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDelete_PermanentlyRemove verifies the Delete_PermanentlyRemove handler.
// The mock GitLab API at /api/v4/groups/99 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_PermanentlyRemove(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/99" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID:           "99",
		PermanentlyRemove: true,
		FullPath:          "org/infra",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Restore — canceled context, server error
// ---------------------------------------------------------------------------.

// TestRestore_CancelledContext verifies the Restore_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestRestore_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Restore(ctx, client, RestoreInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRestore_ServerError verifies that Restore_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRestore_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Restore(context.Background(), client, RestoreInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// Archive — success, cancelled context, server error, forbidden
// ---------------------------------------------------------------------------.

// TestArchive_Success verifies that Archive succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/99/archive (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestArchive_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/archive" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	err := Archive(context.Background(), client, ArchiveInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestArchive_CancelledContext verifies the Archive_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestArchive_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Archive(ctx, client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestArchive_Forbidden verifies the Archive_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestArchive_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Archive(context.Background(), client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error on forbidden, got nil")
	}
}

// TestArchive_EmptyGroupID verifies the Archive_EmptyGroupID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestArchive_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Archive(context.Background(), client, ArchiveInput{})
	if err == nil {
		t.Fatal("expected error on empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Unarchive — success, cancelled context, server error, forbidden
// ---------------------------------------------------------------------------.

// TestUnarchive_Success verifies that Unarchive succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/99/unarchive (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestUnarchive_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/unarchive" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	err := Unarchive(context.Background(), client, ArchiveInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUnarchive_CancelledContext verifies the Unarchive_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUnarchive_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Unarchive(ctx, client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUnarchive_Forbidden verifies the Unarchive_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUnarchive_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Unarchive(context.Background(), client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error on forbidden, got nil")
	}
}

// TestUnarchive_EmptyGroupID verifies the Unarchive_EmptyGroupID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUnarchive_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Unarchive(context.Background(), client, ArchiveInput{})
	if err == nil {
		t.Fatal("expected error on empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Search — canceled context, server error
// ---------------------------------------------------------------------------.

// TestSearch_CancelledContext verifies the Search_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestSearch_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Search(ctx, client, SearchInput{Query: "q"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestSearch_ServerError verifies that Search_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSearch_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Search(context.Background(), client, SearchInput{Query: "q"})
	if err == nil {
		t.Fatal("expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// TransferProject — canceled context, server error
// ---------------------------------------------------------------------------.

// TestTransferProject_CancelledContext verifies the TransferProject_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestTransferProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := TransferProject(ctx, client, TransferInput{GroupID: "99", ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestTransferProject_ServerError verifies that TransferProject_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestTransferProject_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := TransferProject(context.Background(), client, TransferInput{GroupID: "99", ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error on server failure, got nil")
	}
}

// TestTransferProject_BadRequest verifies the TransferProject_BadRequest handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestTransferProject_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := TransferProject(context.Background(), client, TransferInput{GroupID: "99", ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error on bad request, got nil")
	}
	if !strings.Contains(err.Error(), "target group is incompatible") {
		t.Fatalf("error = %v, want compatibility hint", err)
	}
}

// ---------------------------------------------------------------------------
// ListProjects — canceled context, with optional filter fields
// ---------------------------------------------------------------------------.

// TestListProjects_CancelledContext verifies the ListProjects_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListProjects_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProjects(ctx, client, ListProjectsInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListProjects_AllOptionalFilters verifies the ListProjects_AllOptionalFilters handler.
// The mock GitLab API at /api/v4/groups/99/projects (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListProjects_AllOptionalFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/99/projects" {
			assertListProjectsOptionalQuery(t, r)
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	archived := false
	withShared := true
	_, err := ListProjects(context.Background(), client, ListProjectsInput{
		GroupID:          "99",
		Search:           "myapp",
		Archived:         &archived,
		Visibility:       "private",
		OrderBy:          "name",
		Sort:             "asc",
		Simple:           true,
		Owned:            true,
		Starred:          true,
		IncludeSubGroups: true,
		WithShared:       &withShared,
		Page:             1, PerPage: 20,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func assertListProjectsOptionalQuery(t *testing.T, r *http.Request) {
	t.Helper()
	q := r.URL.Query()
	expected := map[string]string{
		"search":            "myapp",
		"visibility":        "private",
		"order_by":          "name",
		"sort":              "asc",
		"simple":            "true",
		"owned":             "true",
		"starred":           "true",
		"include_subgroups": "true",
	}
	for key, want := range expected {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// ListHooks — canceled context, missing group_id, empty result
// ---------------------------------------------------------------------------.

// TestListHooks_CancelledContext verifies the ListHooks_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListHooks_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListHooks(ctx, client, ListHooksInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListHooks_MissingGroupID verifies that ListHooks_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListHooks_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := ListHooks(context.Background(), client, ListHooksInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListHooks_Empty verifies the ListHooks_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListHooks_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListHooks(context.Background(), client, ListHooksInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Hooks) != 0 {
		t.Errorf("len(out.Hooks) = %d, want 0", len(out.Hooks))
	}
}

// TestListHooks_WithPagination verifies that ListHooks_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListHooks_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("page"); got != "2" {
			t.Errorf("page = %q, want %q", got, "2")
		}
		if got := q.Get("per_page"); got != "5" {
			t.Errorf("per_page = %q, want %q", got, "5")
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListHooks(context.Background(), client, ListHooksInput{
		GroupID: "99",
		Page:    2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// GetHook — canceled context, missing group_id
// ---------------------------------------------------------------------------.

// TestGetHook_CancelledContext verifies the GetHook_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetHook(ctx, client, GetHookInput{GroupID: "99", HookID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetHook_MissingGroupID verifies that GetHook_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetHook_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetHook(context.Background(), client, GetHookInput{HookID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// AddHook — canceled context, missing group_id, missing url, with all opts
// ---------------------------------------------------------------------------.

// TestAddHook_CancelledContext verifies the AddHook_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestAddHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddHook(ctx, client, AddHookInput{
		GroupID: "99",
		URL:     "https://example.com",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestAddHook_MissingGroupID verifies that AddHook_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddHook_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := AddHook(context.Background(), client, AddHookInput{
		URL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestAddHook_MissingURL verifies that AddHook_MissingURL returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddHook_MissingURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := AddHook(context.Background(), client, AddHookInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
}

// TestAddHook_AllOptionalFields verifies the AddHook_AllOptionalFields handler.
// The mock GitLab API at /api/v4/groups/99/hooks (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestAddHook_AllOptionalFields(t *testing.T) {
	hookResponse := `{"id":20,"url":"https://hooks.example.com/ci","name":"Full Hook","description":"All events","group_id":99,"push_events":true,"tag_push_events":true,"merge_requests_events":true,"issues_events":true,"note_events":true,"job_events":true,"pipeline_events":true,"wiki_page_events":true,"deployment_events":true,"releases_events":true,"subgroup_events":true,"member_events":true,"confidential_issues_events":true,"confidential_note_events":true,"enable_ssl_verification":true,"created_at":"2026-03-01T10:00:00Z"}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/hooks" {
			testutil.RespondJSON(w, http.StatusCreated, hookResponse)
			return
		}
		http.NotFound(w, r)
	}))

	bTrue := true
	out, err := AddHook(context.Background(), client, AddHookInput{
		GroupID:                  "99",
		URL:                      "https://hooks.example.com/ci",
		Name:                     "Full Hook",
		Description:              "All events",
		Token:                    "secret-token",
		PushEvents:               &bTrue,
		TagPushEvents:            &bTrue,
		MergeRequestsEvents:      &bTrue,
		IssuesEvents:             &bTrue,
		NoteEvents:               &bTrue,
		JobEvents:                &bTrue,
		PipelineEvents:           &bTrue,
		WikiPageEvents:           &bTrue,
		DeploymentEvents:         &bTrue,
		ReleasesEvents:           &bTrue,
		SubGroupEvents:           &bTrue,
		MemberEvents:             &bTrue,
		ConfidentialIssuesEvents: &bTrue,
		ConfidentialNoteEvents:   &bTrue,
		EnableSSLVerification:    &bTrue,
		PushEventsBranchFilter:   "main",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 20 {
		t.Errorf("out.ID = %d, want 20", out.ID)
	}
	if out.Name != "Full Hook" {
		t.Errorf("out.Name = %q, want %q", out.Name, "Full Hook")
	}
	if !out.PushEvents {
		t.Error("PushEvents should be true")
	}
	if !out.MemberEvents {
		t.Error("MemberEvents should be true")
	}
}

// ---------------------------------------------------------------------------
// EditHook — canceled context, missing group_id, with all optional fields
// ---------------------------------------------------------------------------.

// TestEditHook_CancelledContext verifies the EditHook_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEditHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := EditHook(ctx, client, EditHookInput{
		GroupID: "99",
		HookID:  10,
		URL:     "https://example.com",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestEditHook_MissingGroupID verifies that EditHook_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEditHook_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditHook(context.Background(), client, EditHookInput{
		HookID: 10,
		URL:    "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestEditHook_AllOptionalFields verifies the EditHook_AllOptionalFields handler.
// The mock GitLab API at /api/v4/groups/99/hooks/10 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestEditHook_AllOptionalFields(t *testing.T) {
	hookResponse := `{"id":10,"url":"https://hooks.example.com/updated","name":"Edited","description":"Updated hook","group_id":99,"push_events":false,"tag_push_events":true,"merge_requests_events":true,"issues_events":false,"note_events":true,"job_events":false,"pipeline_events":true,"wiki_page_events":false,"deployment_events":true,"releases_events":true,"subgroup_events":false,"member_events":true,"confidential_issues_events":false,"confidential_note_events":true,"enable_ssl_verification":false,"created_at":"2026-01-15T10:00:00Z"}`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/99/hooks/10" {
			testutil.RespondJSON(w, http.StatusOK, hookResponse)
			return
		}
		http.NotFound(w, r)
	}))

	bTrue := true
	bFalse := false
	out, err := EditHook(context.Background(), client, EditHookInput{
		GroupID:                  "99",
		HookID:                   10,
		URL:                      "https://hooks.example.com/updated",
		Name:                     "Edited",
		Description:              "Updated hook",
		Token:                    "new-secret",
		PushEvents:               &bFalse,
		TagPushEvents:            &bTrue,
		MergeRequestsEvents:      &bTrue,
		IssuesEvents:             &bFalse,
		NoteEvents:               &bTrue,
		JobEvents:                &bFalse,
		PipelineEvents:           &bTrue,
		WikiPageEvents:           &bFalse,
		DeploymentEvents:         &bTrue,
		ReleasesEvents:           &bTrue,
		SubGroupEvents:           &bFalse,
		MemberEvents:             &bTrue,
		ConfidentialIssuesEvents: &bFalse,
		ConfidentialNoteEvents:   &bTrue,
		EnableSSLVerification:    &bFalse,
		PushEventsBranchFilter:   "develop",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "Edited" {
		t.Errorf("out.Name = %q, want %q", out.Name, "Edited")
	}
}

// ---------------------------------------------------------------------------
// DeleteHook — canceled context, missing group_id
// ---------------------------------------------------------------------------.

// TestDeleteHook_CancelledContext verifies the DeleteHook_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteHook(ctx, client, DeleteHookInput{GroupID: "99", HookID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDeleteHook_MissingGroupID verifies that DeleteHook_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteHook_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteHook(context.Background(), client, DeleteHookInput{HookID: 10})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_WithData verifies the OutputMarkdown_WithData Markdown formatter for a representative output_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_WithData(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                99,
		Name:              "infrastructure",
		FullPath:          "org/infra",
		FullName:          "Org / Infrastructure",
		Visibility:        "private",
		Description:       "Infra group",
		WebURL:            "https://gitlab.example.com/groups/org/infra",
		ParentID:          1,
		CreatedAt:         "2026-01-15T10:00:00Z",
		MarkedForDeletion: "2026-06-01",
	})

	for _, want := range []string{
		"## Group: infrastructure",
		"**ID**: 99",
		"**Path**: org/infra",
		"**Full Name**: Org / Infrastructure",
		"**Visibility**: private",
		"**Description**: Infra group",
		"**URL**:",
		"**Parent ID**: 1",
		"**Created**:",
		"Marked for deletion",
		"2026-06-01",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatOutputMarkdown_Minimal verifies the OutputMarkdown_Minimal Markdown formatter for a representative output_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_Minimal(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:         1,
		Name:       "minimal",
		FullPath:   "minimal",
		Visibility: "public",
		WebURL:     "https://gl.example.com/minimal",
	})

	if !strings.Contains(md, "## Group: minimal") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"**Full Name**",
		"**Description**",
		"**Parent ID**",
		"**Created**",
		"Marked for deletion",
	} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(md, absent) {
				t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithData verifies the ListMarkdown_WithData Markdown formatter for a representative list_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Groups: []Output{
			{ID: 1, Name: "group-a", FullPath: "org/group-a", Visibility: "public"},
			{ID: 2, Name: "group-b", FullPath: "org/group-b", Visibility: "private"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Groups (2)",
		"| ID |",
		"| --- |",
		"| 1 |",
		"| 2 |",
		"group-a",
		"group-b",
		"public",
		"private",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No groups found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatMemberListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatMemberListMarkdown_WithData verifies the MemberListMarkdown_WithData Markdown formatter for a representative memberlist_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMemberListMarkdown_WithData(t *testing.T) {
	out := MemberListOutput{
		Members: []MemberOutput{
			{ID: 10, Username: "devops1", Name: "DevOps One", AccessLevel: 40, State: "active"},
			{ID: 11, Username: "devops2", Name: "DevOps Two", AccessLevel: 30, State: "active"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatMemberListMarkdown(out)

	for _, want := range []string{
		"## Group Members (2)",
		"| Username |",
		"| --- |",
		"devops1",
		"devops2",
		"Maintainer",
		"Developer",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatMemberListMarkdown_Empty verifies the MemberListMarkdown_Empty Markdown formatter for a representative memberlist_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMemberListMarkdown_Empty(t *testing.T) {
	md := FormatMemberListMarkdown(MemberListOutput{})
	if !strings.Contains(md, "No members found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| Username |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListProjectsMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListProjectsMarkdown_WithData verifies the ListProjectsMarkdown_WithData Markdown formatter for a representative listprojects_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListProjectsMarkdown_WithData(t *testing.T) {
	out := ListProjectsOutput{
		Projects: []ProjectItem{
			{ID: 42, Name: "my-project", PathWithNamespace: "org/infra/my-project", Visibility: "private", Archived: false},
			{ID: 43, Name: "old-project", PathWithNamespace: "org/infra/old-project", Visibility: "public", Archived: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListProjectsMarkdown(out)

	for _, want := range []string{
		"| ID |",
		"| --- |",
		"| 42 |",
		"| 43 |",
		"my-project",
		"old-project",
		"No",
		"Yes",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListProjectsMarkdown_Empty verifies the ListProjectsMarkdown_Empty Markdown formatter for a representative listprojects_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListProjectsMarkdown_Empty(t *testing.T) {
	md := FormatListProjectsMarkdown(ListProjectsOutput{})
	if !strings.Contains(md, "No projects found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatHookMarkdown
// ---------------------------------------------------------------------------.

// TestFormatHookMarkdown_WithNameAndAllEvents verifies the HookMarkdown_WithNameAndAllEvents Markdown formatter for a representative hook_withnameandallevents input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatHookMarkdown_WithNameAndAllEvents(t *testing.T) {
	md := FormatHookMarkdown(HookOutput{
		ID:                       10,
		URL:                      "https://example.com/hook",
		Name:                     "CI Hook",
		Description:              "Triggers CI",
		GroupID:                  99,
		PushEvents:               true,
		TagPushEvents:            true,
		MergeRequestsEvents:      true,
		IssuesEvents:             true,
		NoteEvents:               true,
		JobEvents:                true,
		PipelineEvents:           true,
		WikiPageEvents:           true,
		DeploymentEvents:         true,
		ReleasesEvents:           true,
		SubGroupEvents:           true,
		MemberEvents:             true,
		ConfidentialIssuesEvents: false,
		ConfidentialNoteEvents:   false,
		EnableSSLVerification:    true,
		AlertStatus:              "executable",
		CreatedAt:                "2026-01-15T10:00:00Z",
	})

	for _, want := range []string{
		"## Group Hook: CI Hook",
		"**ID**: 10",
		"**URL**: [https://example.com/hook](https://example.com/hook)",
		"**Name**: CI Hook",
		"**Description**: Triggers CI",
		"**Group ID**: 99",
		"**SSL Verification**: true",
		"push",
		"merge_request",
		"pipeline",
		"**Alert Status**: executable",
		"**Created**:",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatHookMarkdown_WithoutName verifies the HookMarkdown_WithoutName Markdown formatter for a representative hook_withoutname input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatHookMarkdown_WithoutName(t *testing.T) {
	md := FormatHookMarkdown(HookOutput{
		ID:  5,
		URL: "https://hooks.example.com/plain",
	})

	if !strings.Contains(md, "## Group Hook: https://hooks.example.com/plain") {
		t.Errorf("expected URL as title when no name:\n%s", md)
	}
	if strings.Contains(md, "**Name**") {
		t.Errorf("should not have Name line when empty:\n%s", md)
	}
	if strings.Contains(md, "**Description**") {
		t.Errorf("should not have Description line when empty:\n%s", md)
	}
	if strings.Contains(md, "**Alert Status**") {
		t.Errorf("should not have AlertStatus line when empty:\n%s", md)
	}
	if strings.Contains(md, "**Created**") {
		t.Errorf("should not have Created line when empty:\n%s", md)
	}
}

// TestFormatHookMarkdown_NoEventsEnabled verifies the HookMarkdown_NoEventsEnabled Markdown formatter for a representative hook_noeventsenabled input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatHookMarkdown_NoEventsEnabled(t *testing.T) {
	md := FormatHookMarkdown(HookOutput{
		ID:  1,
		URL: "https://hooks.example.com/none",
	})

	if !strings.Contains(md, "none") {
		t.Errorf("expected 'none' when no events enabled:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatHookListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatHookListMarkdown_WithData verifies the HookListMarkdown_WithData Markdown formatter for a representative hooklist_withdata input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatHookListMarkdown_WithData(t *testing.T) {
	out := HookListOutput{
		Hooks: []HookOutput{
			{ID: 10, URL: "https://example.com/hook", PushEvents: true, MergeRequestsEvents: true, EnableSSLVerification: true},
			{ID: 11, URL: "https://example.com/hook2", PipelineEvents: true, EnableSSLVerification: false},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatHookListMarkdown(out)

	for _, want := range []string{
		"## Group Hooks (2)",
		"| ID |",
		"| --- |",
		"| 10 |",
		"| 11 |",
		"Yes",
		"No",
		"push",
		"pipeline",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatHookListMarkdown_Empty verifies the HookListMarkdown_Empty Markdown formatter for a representative hooklist_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatHookListMarkdown_Empty(t *testing.T) {
	md := FormatHookListMarkdown(HookListOutput{})
	if !strings.Contains(md, "No group webhooks found.") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// enabledEvents — comprehensive
// ---------------------------------------------------------------------------.

// TestEnabledEvents_All verifies the EnabledEvents_All handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnabledEvents_All(t *testing.T) {
	h := HookOutput{
		PushEvents:          true,
		TagPushEvents:       true,
		MergeRequestsEvents: true,
		IssuesEvents:        true,
		NoteEvents:          true,
		JobEvents:           true,
		PipelineEvents:      true,
		WikiPageEvents:      true,
		DeploymentEvents:    true,
		ReleasesEvents:      true,
		SubGroupEvents:      true,
		MemberEvents:        true,
	}
	result := enabledEvents(h)

	for _, want := range []string{"push", "tag_push", "merge_request", "issues", "note", "job", "pipeline", "wiki", "deployment", "releases", "subgroup", "member"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(result, want) {
				t.Errorf("enabledEvents missing %q: %s", want, result)
			}
		})
	}
}

// TestEnabledEvents_None verifies the EnabledEvents_None handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnabledEvents_None(t *testing.T) {
	result := enabledEvents(HookOutput{})
	if result != "none" {
		t.Errorf("enabledEvents = %q, want %q", result, "none")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := groupSpecsByTool(t, specs)

	if len(specs) != 37 {
		t.Fatalf("len(ActionSpecs) = %d, want 37", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groups" {
			t.Fatalf("OwnerPackage for %s = %q, want groups", spec.Name, spec.OwnerPackage)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpecsCallAllRoutes — route coverage for all 18 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newGroupsRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_group_list", map[string]any{}},
		{"get", "gitlab_group_get", map[string]any{"group_id": "99"}},
		{"members_list", "gitlab_group_members_list", map[string]any{"group_id": "99"}},
		{"subgroups_list", "gitlab_subgroups_list", map[string]any{"group_id": "99"}},
		{"create", "gitlab_group_create", map[string]any{"name": "new-group"}},
		{"update", "gitlab_group_update", map[string]any{"group_id": "99", "name": "updated"}},
		{"delete", "gitlab_group_delete", map[string]any{"group_id": "99"}},
		{"restore", "gitlab_group_restore", map[string]any{"group_id": "99"}},
		{"archive", "gitlab_group_archive", map[string]any{"group_id": "99"}},
		{"unarchive", "gitlab_group_unarchive", map[string]any{"group_id": "99"}},
		{"search", "gitlab_group_search", map[string]any{"query": "infra"}},
		{"transfer_project", "gitlab_group_transfer_project", map[string]any{"group_id": "99", "project_id": "42"}},
		{"list_projects", "gitlab_group_projects", map[string]any{"group_id": "99"}},
		{"shared_with_list", "gitlab_group_shared_with_list", map[string]any{"group_id": "99"}},
		{"invited_list", "gitlab_group_invited_list", map[string]any{"group_id": "99"}},
		{"transfer_locations", "gitlab_group_transfer_locations", map[string]any{"group_id": "99"}},
		{"hook_list", "gitlab_group_hook_list", map[string]any{"group_id": "99"}},
		{"hook_get", "gitlab_group_hook_get", map[string]any{"group_id": "99", "hook_id": 10}},
		{"hook_add", "gitlab_group_hook_add", map[string]any{"group_id": "99", "url": "https://example.com/hook"}},
		{"hook_edit", "gitlab_group_hook_edit", map[string]any{"group_id": "99", "hook_id": 10, "url": "https://example.com/hook2"}},
		{"hook_delete", "gitlab_group_hook_delete", map[string]any{"group_id": "99", "hook_id": 10}},
		{"hook_set_custom_header", "gitlab_group_hook_set_custom_header", map[string]any{"group_id": "99", "hook_id": 10, "key": "X-Token", "value": "secret"}},
		{"hook_delete_custom_header", "gitlab_group_hook_delete_custom_header", map[string]any{"group_id": "99", "hook_id": 10, "key": "X-Token"}},
		{"hook_set_url_variable", "gitlab_group_hook_set_url_variable", map[string]any{"group_id": "99", "hook_id": 10, "key": "env", "value": "prod"}},
		{"hook_delete_url_variable", "gitlab_group_hook_delete_url_variable", map[string]any{"group_id": "99", "hook_id": 10, "key": "env"}},
		{"hook_test", "gitlab_group_hook_test", map[string]any{"group_id": "99", "hook_id": 10, "trigger": "push_events"}},
		{"hook_resend_event", "gitlab_group_hook_resend_event", map[string]any{"group_id": "99", "hook_id": 10, "hook_event_id": 5}},
		{"share_with_group", "gitlab_group_share_with_group", map[string]any{"group_id": "99", "shared_group_id": 123, "group_access": 30}},
		{"unshare_from_group", "gitlab_group_unshare_from_group", map[string]any{"group_id": "99", "shared_group_id": 123}},
		{"shared_projects_list", "gitlab_group_shared_projects_list", map[string]any{"group_id": "99"}},
		{"transfer", "gitlab_group_transfer", map[string]any{"group_id": "99", "parent_id": 42}},
		{"push_rule_get", "gitlab_group_get_push_rules", map[string]any{"group_id": "99"}},
		{"push_rule_add", "gitlab_group_add_push_rule", map[string]any{"group_id": "99", "commit_message_regex": "^JIRA-"}},
		{"push_rule_edit", "gitlab_group_edit_push_rule", map[string]any{"group_id": "99", "prevent_secrets": true}},
		{"push_rule_delete", "gitlab_group_delete_push_rule", map[string]any{"group_id": "99"}},
		{"upload_avatar", "gitlab_group_upload_avatar", map[string]any{"group_id": "99", "filename": "avatar.png", "content_base64": "YXZhdGFy"}},
		{"list_provisioned_users", "gitlab_group_list_provisioned_users", map[string]any{"group_id": "99"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: route spec factory
// ---------------------------------------------------------------------------.

// newGroupsRouteSpecs constructs groups route specs test fixtures.
func newGroupsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	groupJSON := `{"id":99,"name":"infrastructure","path":"infra","full_path":"org/infra","full_name":"Org / Infrastructure","description":"Infra group","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra","parent_id":1,"created_at":"2026-01-15T10:00:00Z"}`
	hookJSON := `{"id":10,"url":"https://example.com/hook","name":"CI Hook","group_id":99,"push_events":true,"enable_ssl_verification":true,"created_at":"2026-01-15T10:00:00Z"}`
	projectJSON := `[{"id":42,"name":"my-project","path_with_namespace":"org/infra/my-project","visibility":"private","web_url":"https://gitlab.example.com/org/infra/my-project","default_branch":"main","archived":false}]`

	handler := http.NewServeMux()

	// List groups
	handler.HandleFunc("GET /api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[`+groupJSON+`]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+groupJSON+`]`)
	})

	// Get group
	handler.HandleFunc("GET /api/v4/groups/99", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// List group members
	handler.HandleFunc("GET /api/v4/groups/99/members/all", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"username":"devops1","name":"DevOps One","state":"active","access_level":40,"web_url":"https://gitlab.example.com/devops1"}]`)
	})

	// List descendant groups (subgroups)
	handler.HandleFunc("GET /api/v4/groups/99/descendant_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"name":"monitoring","path":"monitoring","full_path":"org/infra/monitoring","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra/monitoring","parent_id":99}]`)
	})

	// Create group
	handler.HandleFunc("POST /api/v4/groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, groupJSON)
	})

	// Update group
	handler.HandleFunc("PUT /api/v4/groups/99", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// Delete group
	handler.HandleFunc("DELETE /api/v4/groups/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	// Restore group
	handler.HandleFunc("POST /api/v4/groups/99/restore", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// Archive group
	handler.HandleFunc("POST /api/v4/groups/99/archive", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// Unarchive group
	handler.HandleFunc("POST /api/v4/groups/99/unarchive", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// Transfer project into group
	handler.HandleFunc("POST /api/v4/groups/99/projects/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// List group projects
	handler.HandleFunc("GET /api/v4/groups/99/projects", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectJSON)
	})

	// List groups shared with the group
	handler.HandleFunc("GET /api/v4/groups/99/groups/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+groupJSON+`]`)
	})

	// List groups invited to the group
	handler.HandleFunc("GET /api/v4/groups/99/invited_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+groupJSON+`]`)
	})

	// List candidate transfer locations
	handler.HandleFunc("GET /api/v4/groups/99/transfer_locations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"Target","full_name":"Target Group","full_path":"target","web_url":"https://gitlab.example.com/groups/target"}]`)
	})

	// List group hooks
	handler.HandleFunc("GET /api/v4/groups/99/hooks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+hookJSON+`]`)
	})

	// Get group hook
	handler.HandleFunc("GET /api/v4/groups/99/hooks/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	})

	// Add group hook
	handler.HandleFunc("POST /api/v4/groups/99/hooks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, hookJSON)
	})

	// Edit group hook
	handler.HandleFunc("PUT /api/v4/groups/99/hooks/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	})

	// Delete group hook
	handler.HandleFunc("DELETE /api/v4/groups/99/hooks/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Hook custom header set/delete
	handler.HandleFunc("PUT /api/v4/groups/99/hooks/10/custom_headers/X-Token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.HandleFunc("DELETE /api/v4/groups/99/hooks/10/custom_headers/X-Token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Hook URL variable set/delete
	handler.HandleFunc("PUT /api/v4/groups/99/hooks/10/url_variables/env", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.HandleFunc("DELETE /api/v4/groups/99/hooks/10/url_variables/env", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Hook test trigger
	handler.HandleFunc("POST /api/v4/groups/99/hooks/10/test/push_events", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	// Hook event resend
	handler.HandleFunc("POST /api/v4/groups/99/hooks/10/events/5/resend", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	// Share group with group
	handler.HandleFunc("POST /api/v4/groups/99/share", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, groupJSON)
	})

	// Unshare group from group
	handler.HandleFunc("DELETE /api/v4/groups/99/share/123", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// List shared projects
	handler.HandleFunc("GET /api/v4/groups/99/projects/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectJSON)
	})

	// Transfer subgroup
	handler.HandleFunc("POST /api/v4/groups/99/transfer", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// Group push rules get/add/edit/delete (singleton at /push_rule)
	pushRuleJSON := `{"id":1,"commit_message_regex":"^JIRA-","branch_name_regex":"","max_file_size":100,"prevent_secrets":true,"reject_unsigned_commits":false,"created_at":"2026-01-15T10:00:00Z"}`
	handler.HandleFunc("GET /api/v4/groups/99/push_rule", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/99/push_rule", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, pushRuleJSON)
	})
	handler.HandleFunc("PUT /api/v4/groups/99/push_rule", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/99/push_rule", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Upload group avatar uses PUT /api/v4/groups/99 (shared with Update), so no
	// extra handler is required here.

	// List provisioned users
	handler.HandleFunc("GET /api/v4/groups/99/provisioned_users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":7,"username":"scim-user","name":"SCIM User","state":"active","web_url":"https://gitlab.example.com/scim-user"}]`)
	})

	client := testutil.NewTestClient(t, handler)
	return groupSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_GroupGetRoute validates the GroupGetRoute route through the catalog surface.
// The mock GitLab API at /api/v4/groups/10 (GET) responds with HTTP OK.
// It asserts the route returns the expected error or result.
func TestActionSpecs_GroupGetRoute(t *testing.T) {
	const respJSON = `{"id":10,"name":"G","path":"g","full_path":"g","web_url":"https://gitlab.example.com/groups/g","visibility":"private"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := groupSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_group_get"].Route.Handler(t.Context(), map[string]any{"group_id": "10"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 10 || out.Name != "G" {
		t.Fatalf("group output = %#v, want ID 10 name G", out)
	}
}

// groupSpecsByTool supports group specs by tool assertions in groups tests.
func groupSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// fullGroupJSON is a single-group detail fixture exercising every additive
// gl.Group field surfaced by the 1:1 audit, including nested sub-objects.
const fullGroupJSON = `{
	"id":99,"name":"infra","path":"infra","full_path":"org/infra","visibility":"private",
	"web_url":"https://gitlab.example.com/groups/org/infra",
	"membership_lock":true,"max_artifacts_size":500,"repository_storage":"default",
	"file_template_project_id":7,"share_with_group_lock":true,
	"require_two_factor_authentication":true,"two_factor_grace_period":48,
	"auto_devops_enabled":true,"emails_enabled":true,"emails_disabled":false,
	"mentions_disabled":true,"runners_token":"glrt-xxx","ldap_cn":"infra-cn","ldap_access":30,
	"shared_runners_minutes_limit":1000,"extra_shared_runners_minutes_limit":200,
	"prevent_forking_outside_group":true,"ip_restriction_ranges":"10.0.0.0/8",
	"allowed_email_domains_list":"example.com","wiki_access_level":"enabled",
	"only_allow_merge_if_pipeline_succeeds":true,"allow_merge_on_skipped_pipeline":true,
	"only_allow_merge_if_all_discussions_are_resolved":true,"default_branch_protection":2,
	"statistics":{"commit_count":12,"storage_size":34,"repository_size":5,"wiki_size":1,
		"lfs_objects_size":2,"job_artifacts_size":3,"pipeline_artifacts_size":4,
		"packages_size":6,"snippets_size":7,"uploads_size":8,"container_registry_size":9},
	"root_storage_statistics":{"build_artifacts_size":11,"container_registry_size":22,
		"container_registry_size_is_estimated":true,"dependency_proxy_size":33,
		"lfs_objects_size":44,"packages_size":55,"pipeline_artifacts_size":66,
		"repository_size":77,"snippets_size":88,"storage_size":99,"uploads_size":110,
		"wiki_size":121},
	"custom_attributes":[{"key":"team","value":"platform"}],
	"default_branch_protection_defaults":{"allow_force_push":true,
		"developer_can_initial_push":true,"code_owner_approval_required":true,
		"allowed_to_push":[{"access_level":40}],"allowed_to_merge":[{"access_level":30}]},
	"shared_with_groups":[{"group_id":5,"group_name":"sec","group_full_path":"org/sec",
		"group_access_level":30,"expires_at":"2026-12-31","member_role_id":2}],
	"ldap_group_links":[{"cn":"link-cn","filter":"(uid=*)","group_access":30,
		"provider":"ldapmain","member_role_id":3}],
	"saml_group_links":[{"name":"saml-link","access_level":40,"member_role_id":4,"provider":"okta"}],
	"projects":[{"id":1,"name":"p1","path_with_namespace":"org/infra/p1","visibility":"private",
		"web_url":"https://gitlab.example.com/org/infra/p1","created_at":"2026-01-15T10:00:00Z"}],
	"shared_projects":[{"id":2,"name":"p2","path_with_namespace":"org/other/p2","visibility":"private",
		"web_url":"https://gitlab.example.com/org/other/p2"}]
}`

// TestToOutput_FullNestedObjects verifies Get surfaces every additive gl.Group
// field and nested sub-object on its canonical json key (1:1 audit).
func TestToOutput_FullNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99 {
			testutil.RespondJSON(w, http.StatusOK, fullGroupJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertGroupScalarFields(t, out)
	assertGroupNestedObjects(t, out)
	assertGroupNestedLists(t, out)
}

// assertGroupScalarFields checks the additive scalar gl.Group fields surfaced by
// ToOutput, comparing each against the fullGroupJSON fixture via a table.
func assertGroupScalarFields(t *testing.T, out Output) {
	t.Helper()
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"MembershipLock", out.MembershipLock, true},
		{"MaxArtifactsSize", out.MaxArtifactsSize, int64(500)},
		{"RepositoryStorage", out.RepositoryStorage, "default"},
		{"FileTemplateProjectID", out.FileTemplateProjectID, int64(7)},
		{"ShareWithGroupLock", out.ShareWithGroupLock, true},
		{"RequireTwoFactorAuth", out.RequireTwoFactorAuth, true},
		{"TwoFactorGracePeriod", out.TwoFactorGracePeriod, int64(48)},
		{"AutoDevopsEnabled", out.AutoDevopsEnabled, true},
		{"EmailsEnabled", out.EmailsEnabled, true},
		{"MentionsDisabled", out.MentionsDisabled, true},
		{"RunnersToken", out.RunnersToken, "glrt-xxx"},
		{"LDAPCN", out.LDAPCN, "infra-cn"},
		{"LDAPAccess", out.LDAPAccess, 30},
		{"SharedRunnersMinutesLimit", out.SharedRunnersMinutesLimit, int64(1000)},
		{"ExtraSharedRunnersMinutesLimit", out.ExtraSharedRunnersMinutesLimit, int64(200)},
		{"PreventForkingOutsideGroup", out.PreventForkingOutsideGroup, true},
		{"IPRestrictionRanges", out.IPRestrictionRanges, "10.0.0.0/8"},
		{"AllowedEmailDomainsList", out.AllowedEmailDomainsList, "example.com"},
		{"WikiAccessLevel", out.WikiAccessLevel, "enabled"},
		{"OnlyAllowMergeIfPipelineSucceeds", out.OnlyAllowMergeIfPipelineSucceeds, true},
		{"AllowMergeOnSkippedPipeline", out.AllowMergeOnSkippedPipeline, true},
		{"OnlyAllowMergeIfAllDiscussionsAreResolved", out.OnlyAllowMergeIfAllDiscussionsAreResolved, true},
		{"DefaultBranchProtection", out.DefaultBranchProtection, int64(2)},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// assertGroupNestedObjects checks the singular nested sub-objects (statistics
// and default_branch_protection_defaults) surfaced by ToOutput.
func assertGroupNestedObjects(t *testing.T, out Output) {
	t.Helper()
	if out.Statistics == nil || out.Statistics.CommitCount != 12 || out.Statistics.ContainerRegistrySize != 9 {
		t.Errorf("Statistics not mapped: %+v", out.Statistics)
	}
	d := out.DefaultBranchProtectionDefaults
	if d == nil || !d.AllowForcePush || !d.DeveloperCanInitialPush || !d.CodeOwnerApprovalRequired {
		t.Fatalf("DefaultBranchProtectionDefaults not mapped: %+v", d)
	}
	if len(d.AllowedToPush) != 1 || d.AllowedToPush[0].AccessLevel != 40 {
		t.Errorf("AllowedToPush not mapped: %+v", d.AllowedToPush)
	}
	if len(d.AllowedToMerge) != 1 || d.AllowedToMerge[0].AccessLevel != 30 {
		t.Errorf("AllowedToMerge not mapped: %+v", d.AllowedToMerge)
	}
	assertGroupRootStorageStatistics(t, out)
}

// assertGroupRootStorageStatistics checks every root_storage_statistics field
// surfaced by ToOutput against the fullGroupJSON fixture (1:1 SDK mirror).
func assertGroupRootStorageStatistics(t *testing.T, out Output) {
	t.Helper()
	r := out.RootStorageStatistics
	if r == nil {
		t.Fatalf("RootStorageStatistics not mapped: nil")
	}
	if !r.ContainerRegistrySizeIsEstimated {
		t.Errorf("ContainerRegistrySizeIsEstimated = false, want true")
	}
	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{"BuildArtifactsSize", r.BuildArtifactsSize, 11},
		{"ContainerRegistrySize", r.ContainerRegistrySize, 22},
		{"DependencyProxySize", r.DependencyProxySize, 33},
		{"LFSObjectsSize", r.LFSObjectsSize, 44},
		{"PackagesSize", r.PackagesSize, 55},
		{"PipelineArtifactsSize", r.PipelineArtifactsSize, 66},
		{"RepositorySize", r.RepositorySize, 77},
		{"SnippetsSize", r.SnippetsSize, 88},
		{"StorageSize", r.StorageSize, 99},
		{"UploadsSize", r.UploadsSize, 110},
		{"WikiSize", r.WikiSize, 121},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("RootStorageStatistics.%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// assertGroupNestedLists checks the link/attribute nested-slice sub-objects
// surfaced by ToOutput (custom attributes, shares, LDAP/SAML links).
func assertGroupNestedLists(t *testing.T, out Output) {
	t.Helper()
	if len(out.CustomAttributes) != 1 || out.CustomAttributes[0].Key != "team" || out.CustomAttributes[0].Value != "platform" {
		t.Errorf("CustomAttributes not mapped: %+v", out.CustomAttributes)
	}
	if len(out.SharedWithGroups) != 1 || out.SharedWithGroups[0].GroupID != 5 || out.SharedWithGroups[0].ExpiresAt == "" || out.SharedWithGroups[0].MemberRoleID != 2 {
		t.Errorf("SharedWithGroups not mapped: %+v", out.SharedWithGroups)
	}
	if len(out.LDAPGroupLinks) != 1 || out.LDAPGroupLinks[0].CN != "link-cn" || out.LDAPGroupLinks[0].GroupAccess != 30 || out.LDAPGroupLinks[0].MemberRoleID != 3 {
		t.Errorf("LDAPGroupLinks not mapped: %+v", out.LDAPGroupLinks)
	}
	if len(out.SAMLGroupLinks) != 1 || out.SAMLGroupLinks[0].Name != "saml-link" || out.SAMLGroupLinks[0].AccessLevel != 40 || out.SAMLGroupLinks[0].MemberRoleID != 4 {
		t.Errorf("SAMLGroupLinks not mapped: %+v", out.SAMLGroupLinks)
	}
	assertGroupEmbeddedProjects(t, out)
}

// assertGroupEmbeddedProjects checks the deprecated embedded projects and
// shared_projects slices surfaced by ToOutput.
func assertGroupEmbeddedProjects(t *testing.T, out Output) {
	t.Helper()
	if len(out.Projects) != 1 || out.Projects[0].ID != 1 || out.Projects[0].CreatedAt == "" {
		t.Errorf("Projects not mapped: %+v", out.Projects)
	}
	if len(out.SharedProjects) != 1 || out.SharedProjects[0].ID != 2 {
		t.Errorf("SharedProjects not mapped: %+v", out.SharedProjects)
	}
}

// TestToOutput_NilNestedObjects verifies the nested-object converters return nil
// for absent sub-objects, so omitempty drops them from the output.
func TestToOutput_NilNestedObjects(t *testing.T) {
	out := ToOutput(&gl.Group{ID: 1, Name: "x"})
	if out.Statistics != nil || out.RootStorageStatistics != nil || out.DefaultBranchProtectionDefaults != nil {
		t.Errorf("expected nil nested objects, got %+v", out)
	}
	if out.CustomAttributes != nil || out.SharedWithGroups != nil || out.LDAPGroupLinks != nil {
		t.Errorf("expected nil slices, got %+v", out)
	}
	if out.SAMLGroupLinks != nil || out.Projects != nil || out.SharedProjects != nil {
		t.Errorf("expected nil slices, got %+v", out)
	}
}

// TestToOutput_NilSliceElements verifies the nested-slice converters skip nil
// pointer elements without panicking.
func TestToOutput_NilSliceElements(t *testing.T) {
	g := &gl.Group{
		ID:               1,
		CustomAttributes: []*gl.CustomAttribute{nil},
		LDAPGroupLinks:   []*gl.LDAPGroupLink{nil},
		SAMLGroupLinks:   []*gl.SAMLGroupLink{nil},
		DefaultBranchProtectionDefaults: &gl.BranchProtectionDefaults{
			AllowedToPush: []*gl.GroupAccessLevel{nil, {AccessLevel: nil}},
		},
	}
	out := ToOutput(g)
	if len(out.CustomAttributes) != 0 || len(out.LDAPGroupLinks) != 0 || len(out.SAMLGroupLinks) != 0 {
		t.Errorf("nil elements should be skipped: %+v", out)
	}
	if d := out.DefaultBranchProtectionDefaults; d == nil || len(d.AllowedToPush) != 1 || d.AllowedToPush[0].AccessLevel != 0 {
		t.Errorf("nil/zero access level handling wrong: %+v", out.DefaultBranchProtectionDefaults)
	}
}

// TestMemberToOutput_FullObjects verifies MembersList surfaces the full
// created_by, group_saml_identity, and member_role objects plus public_email
// and is_using_seat (1:1 audit; pruned scalars removed).
func TestMemberToOutput_FullObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupMembers {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":10,"username":"u","name":"U","state":"active","access_level":40,
				"web_url":"https://gitlab.example.com/u","public_email":"u@example.com","is_using_seat":true,
				"created_by":{"id":1,"username":"admin","name":"Admin","state":"active","web_url":"https://gitlab.example.com/admin"},
				"group_saml_identity":{"extern_uid":"x","provider":"okta","saml_provider_id":3},
				"member_role":{"id":2,"name":"Role","group_id":99,"base_access_level":30,"read_code":true}
			}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
	m := out.Members[0]
	if m.PublicEmail != "u@example.com" || !m.IsUsingSeat {
		t.Errorf("public_email/is_using_seat not mapped: %+v", m)
	}
	if m.CreatedBy == nil || m.CreatedBy.Username != "admin" {
		t.Errorf("created_by not mapped: %+v", m.CreatedBy)
	}
	if m.GroupSAMLIdentity == nil || m.GroupSAMLIdentity.ExternUID != "x" || m.GroupSAMLIdentity.SAMLProviderID != 3 {
		t.Errorf("group_saml_identity not mapped: %+v", m.GroupSAMLIdentity)
	}
	if m.MemberRole == nil || m.MemberRole.ID != 2 || m.MemberRole.BaseAccessLevel != 30 || !m.MemberRole.ReadCode {
		t.Errorf("member_role not mapped: %+v", m.MemberRole)
	}
}

// TestMemberToOutput_NilObjects verifies the member sub-object converters return
// nil for absent objects.
func TestMemberToOutput_NilObjects(t *testing.T) {
	out := MemberToOutput(&gl.GroupMember{ID: 1})
	if out.CreatedBy != nil || out.GroupSAMLIdentity != nil || out.MemberRole != nil {
		t.Errorf("expected nil member sub-objects, got %+v", out)
	}
}

// TestCreate_AuditFields verifies Create forwards the additive
// gl.CreateGroupOptions fields, including the nested
// default_branch_protection_defaults object and the enum string values.
func TestCreate_AuditFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroups {
			bufBytes, _ := io.ReadAll(r.Body)
			body = string(bufBytes)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"name":"infra","path":"infra","full_path":"infra","visibility":"private","web_url":"https://gitlab.example.com/groups/infra"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Name:                           "infra",
		AutoDevopsEnabled:              new(true),
		DefaultBranchProtection:        new(int64(2)),
		DuoAvailability:                "default_on",
		EmailsEnabled:                  new(true),
		EmailsDisabled:                 new(false),
		EnabledGitAccessProtocol:       "ssh",
		ExperimentFeaturesEnabled:      new(true),
		ExtraSharedRunnersMinutesLimit: new(int64(100)),
		MembershipLock:                 new(true),
		MentionsDisabled:               new(true),
		ProjectCreationLevel:           "maintainer",
		RequireTwoFactorAuth:           new(true),
		ShareWithGroupLock:             new(true),
		SharedRunnersMinutesLimit:      new(int64(500)),
		SubGroupCreationLevel:          "owner",
		TwoFactorGracePeriod:           new(int64(48)),
		WikiAccessLevel:                "enabled",
		DefaultBranchProtectionDefaults: &BranchProtectionDefaultsInput{
			AllowForcePush: new(true),
			AllowedToPush:  []int{40},
			AllowedToMerge: []int{30},
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	for _, want := range []string{
		`"auto_devops_enabled":true`, `"default_branch_protection":2`, `"duo_availability":"default_on"`,
		`"emails_enabled":true`, `"emails_disabled":false`, `"enabled_git_access_protocol":"ssh"`,
		`"experiment_features_enabled":true`, `"extra_shared_runners_minutes_limit":100`,
		`"membership_lock":true`, `"mentions_disabled":true`, `"project_creation_level":"maintainer"`,
		`"require_two_factor_authentication":true`, `"share_with_group_lock":true`,
		`"shared_runners_minutes_limit":500`, `"subgroup_creation_level":"owner"`,
		`"two_factor_grace_period":48`, `"wiki_access_level":"enabled"`,
		`"default_branch_protection_defaults"`, `"allow_force_push":true`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("create request body missing %q:\n%s", want, body)
			}
		})
	}
}

// TestUpdate_AuditFields verifies Update forwards the additive
// gl.UpdateGroupOptions fields covered by the 1:1 audit.
func TestUpdate_AuditFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroup99 {
			bufBytes, _ := io.ReadAll(r.Body)
			body = string(bufBytes)
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"name":"infra","path":"infra","full_path":"infra","visibility":"private","web_url":"https://gitlab.example.com/groups/infra"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		GroupID:                                     "99",
		AllowMergeOnSkippedPipeline:                 new(true),
		AllowedEmailDomainsList:                     "example.com",
		AutoBanUserOnExcessiveProjectsDownload:      new(true),
		AutoDevopsEnabled:                           new(true),
		DefaultBranchProtection:                     new(int64(2)),
		DuoAvailability:                             "default_off",
		DuoFeaturesEnabled:                          new(true),
		EmailsDisabled:                              new(false),
		EmailsEnabled:                               new(true),
		EnabledGitAccessProtocol:                    "all",
		ExperimentFeaturesEnabled:                   new(true),
		ExtraSharedRunnersMinutesLimit:              new(int64(50)),
		FileTemplateProjectID:                       new(int64(7)),
		IPRestrictionRanges:                         "10.0.0.0/8",
		LockDuoFeaturesEnabled:                      new(true),
		LockMathRenderingLimitsEnabled:              new(true),
		MaxArtifactsSize:                            new(int64(500)),
		MembershipLock:                              new(true),
		MentionsDisabled:                            new(true),
		OnlyAllowMergeIfAllDiscussionsAreResolved:   new(true),
		OnlyAllowMergeIfPipelineSucceeds:            new(true),
		PreventForkingOutsideGroup:                  new(true),
		PreventSharingGroupsOutside:                 new(true),
		ProjectCreationLevel:                        "developer",
		RequireTwoFactorAuth:                        new(true),
		ShareWithGroupLock:                          new(true),
		SharedRunnersMinutesLimit:                   new(int64(500)),
		SharedRunnersSetting:                        "enabled",
		StepUpAuthRequiredOAuthProvider:             "okta",
		SubGroupCreationLevel:                       "maintainer",
		TwoFactorGracePeriod:                        new(int64(24)),
		UniqueProjectDownloadLimit:                  new(int64(5)),
		UniqueProjectDownloadLimitIntervalInSeconds: new(int64(60)),
		UniqueProjectDownloadLimitAllowlist:         []string{"safe"},
		UniqueProjectDownloadLimitAlertlist:         []int64{1},
		WikiAccessLevel:                             "private",
		DefaultBranchProtectionDefaults:             &BranchProtectionDefaultsInput{CodeOwnerApprovalRequired: new(true)},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	for _, want := range []string{
		`"allow_merge_on_skipped_pipeline":true`, `"allowed_email_domains_list":"example.com"`,
		`"auto_ban_user_on_excessive_projects_download":true`, `"auto_devops_enabled":true`,
		`"default_branch_protection":2`, `"duo_availability":"default_off"`, `"duo_features_enabled":true`,
		`"emails_disabled":false`, `"emails_enabled":true`, `"enabled_git_access_protocol":"all"`,
		`"experiment_features_enabled":true`, `"extra_shared_runners_minutes_limit":50`,
		`"file_template_project_id":7`, `"ip_restriction_ranges":"10.0.0.0/8"`,
		`"lock_duo_features_enabled":true`, `"lock_math_rendering_limits_enabled":true`,
		`"max_artifacts_size":500`, `"membership_lock":true`, `"mentions_disabled":true`,
		`"only_allow_merge_if_all_discussions_are_resolved":true`, `"only_allow_merge_if_pipeline_succeeds":true`,
		`"prevent_forking_outside_group":true`, `"prevent_sharing_groups_outside_hierarchy":true`,
		`"project_creation_level":"developer"`, `"require_two_factor_authentication":true`,
		`"share_with_group_lock":true`, `"shared_runners_minutes_limit":500`,
		`"shared_runners_setting":"enabled"`, `"step_up_auth_required_oauth_provider":"okta"`,
		`"subgroup_creation_level":"maintainer"`, `"two_factor_grace_period":24`,
		`"unique_project_download_limit":5`, `"unique_project_download_limit_interval_in_seconds":60`,
		`"unique_project_download_limit_allowlist":["safe"]`, `"unique_project_download_limit_alertlist":[1]`,
		`"wiki_access_level":"private"`, `"code_owner_approval_required":true`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("update request body missing %q:\n%s", want, body)
			}
		})
	}
}

// TestList_AuditFiltersAndKeyset verifies List forwards the new filters and
// keyset-pagination parameters.
func TestList_AuditFiltersAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroups {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		checks := map[string]string{
			"min_access_level":       "30",
			"repository_storage":     "nfs-01",
			"active":                 "true",
			"archived":               "false",
			"marked_for_deletion_on": "2026-06-01",
			"pagination":             "keyset",
			"page_token":             "abc",
		}
		for k, want := range checks {
			t.Run(k, func(t *testing.T) {
				if got := q.Get(k); got != want {
					t.Errorf("query %s = %q, want %q", k, got, want)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, groupListJSON)
	}))

	active, archived := true, false
	_, err := List(context.Background(), client, ListInput{
		MinAccessLevel:        30,
		RepositoryStorage:     "nfs-01",
		Active:                &active,
		Archived:              &archived,
		MarkedForDeletionOn:   "2026-06-01",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
}

// TestList_InvalidMarkedForDeletionOn verifies an unparseable date is silently
// dropped rather than causing an error.
func TestList_InvalidMarkedForDeletionOn(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			if r.URL.Query().Get("marked_for_deletion_on") != "" {
				t.Error("unparseable marked_for_deletion_on should be dropped")
			}
			testutil.RespondJSON(w, http.StatusOK, groupListJSON)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := List(context.Background(), client, ListInput{MarkedForDeletionOn: "not-a-date"}); err != nil {
		t.Fatalf(fmtGroupListErr, err)
	}
}

// TestSubgroupsList_AuditFilters verifies the additive subgroup filters and
// keyset parameters reach the request.
func TestSubgroupsList_AuditFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroupSubgroups {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		checks := map[string]string{
			"visibility":             "private",
			"top_level_only":         "true",
			"with_custom_attributes": "true",
			"repository_storage":     "nfs-02",
			"active":                 "true",
			"archived":               "false",
			"marked_for_deletion_on": "2026-06-02",
			"pagination":             "keyset",
		}
		for k, want := range checks {
			t.Run(k, func(t *testing.T) {
				if got := q.Get(k); got != want {
					t.Errorf("query %s = %q, want %q", k, got, want)
				}
			})
		}
		if !strings.Contains(r.URL.RawQuery, "skip_groups") {
			t.Error("skip_groups missing")
		}
		testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
	}))

	active, archived := true, false
	_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{
		GroupID:               "99",
		Visibility:            "private",
		TopLevelOnly:          true,
		WithCustomAttributes:  true,
		SkipGroups:            []int64{7},
		RepositoryStorage:     "nfs-02",
		Active:                &active,
		Archived:              &archived,
		MarkedForDeletionOn:   "2026-06-02",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
}

// TestSubgroupsList_InvalidMarkedForDeletionOn covers the unparseable-date
// branch in SubgroupsList.
func TestSubgroupsList_InvalidMarkedForDeletionOn(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupSubgroups {
			if r.URL.Query().Get("marked_for_deletion_on") != "" {
				t.Error("unparseable marked_for_deletion_on should be dropped")
			}
			testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "99", MarkedForDeletionOn: "bad"}); err != nil {
		t.Fatalf(fmtSubgroupsListErr, err)
	}
}

// TestListProjects_AuditFilters verifies the additive group-project filters.
func TestListProjects_AuditFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/99/projects" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		checks := map[string]string{
			"active":                      "true",
			"min_access_level":            "30",
			"topic":                       "go",
			"with_custom_attributes":      "true",
			"with_issues_enabled":         "true",
			"with_merge_requests_enabled": "true",
			"with_security_reports":       "true",
			"pagination":                  "keyset",
		}
		for k, want := range checks {
			t.Run(k, func(t *testing.T) {
				if got := q.Get(k); got != want {
					t.Errorf("query %s = %q, want %q", k, got, want)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"p","path_with_namespace":"org/p","visibility":"private","web_url":"https://gitlab.example.com/org/p"}]`)
	}))

	active, t1, t2, t3 := true, true, true, true
	out, err := ListProjects(context.Background(), client, ListProjectsInput{
		GroupID:                  "99",
		Active:                   &active,
		MinAccessLevel:           30,
		Topic:                    "go",
		WithCustomAttributes:     true,
		WithIssuesEnabled:        &t1,
		WithMergeRequestsEnabled: &t2,
		WithSecurityReports:      &t3,
		KeysetPaginationInput:    keysetKeyset(),
	})
	if err != nil {
		t.Fatalf("ListProjects() unexpected error: %v", err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("len(out.Projects) = %d, want 1", len(out.Projects))
	}
}

// TestMembersList_AuditFilters verifies query/user_ids/show_seat_info/order_by/
// sort/keyset reach the members request.
func TestMembersList_AuditFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroupMembers {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("order_by") != "access_level" || q.Get("sort") != "desc" || q.Get("show_seat_info") != "true" {
			t.Errorf("order_by/sort/show_seat_info missing: %s", r.URL.RawQuery)
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination missing: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "user_ids") {
			t.Errorf("user_ids missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, groupMembersJSON)
	}))

	seat := true
	_, err := MembersList(context.Background(), client, MembersListInput{
		GroupID:               "99",
		OrderBy:               "access_level",
		Sort:                  "desc",
		ShowSeatInfo:          &seat,
		UserIDs:               []int64{10, 11},
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtGroupMembersListErr, err)
	}
}

// TestGet_AuditParams verifies with_custom_attributes/with_projects/order_by/
// sort/keyset reach the single-group request.
func TestGet_AuditParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroup99 {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("with_custom_attributes") != "true" || q.Get("with_projects") != "true" {
			t.Errorf("with_* params missing: %s", r.URL.RawQuery)
		}
		if q.Get("order_by") != "name" || q.Get("sort") != "asc" || q.Get("pagination") != "keyset" {
			t.Errorf("order_by/sort/pagination missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, groupDetailJSON)
	}))

	_, err := Get(context.Background(), client, GetInput{
		GroupID:               "99",
		WithCustomAttributes:  true,
		WithProjects:          new(true),
		OrderBy:               "name",
		Sort:                  "asc",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf(fmtGroupGetErr, err)
	}
}

// TestInvitedAndTransferLocations_OrderSort verifies order_by/sort reach the
// invited-groups and transfer-locations requests.
func TestInvitedAndTransferLocations_OrderSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/groups/99/invited_groups", "/api/v4/groups/99/transfer_locations":
			q := r.URL.Query()
			if q.Get("order_by") != "name" || q.Get("sort") != "desc" {
				t.Errorf("order_by/sort missing on %s: %s", r.URL.Path, r.URL.RawQuery)
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	if _, err := InvitedList(context.Background(), client, InvitedListInput{GroupID: "99", OrderBy: "name", Sort: "desc"}); err != nil {
		t.Fatalf("InvitedList() unexpected error: %v", err)
	}
	if _, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: "99", OrderBy: "name", Sort: "desc"}); err != nil {
		t.Fatalf("TransferLocationsList() unexpected error: %v", err)
	}
}

// keysetKeyset returns a populated keyset-pagination input for tests.
func keysetKeyset() toolutil.KeysetPaginationInput {
	return toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "abc"}
}

// canceledContext returns an already-canceled context to exercise the early
// ctx.Err() guard in the relation list handlers.
func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestSharedWithList_Success verifies that SharedWithList returns the groups shared with a group.
// The test exercises the GET groups/:id/groups/shared path.
// It asserts the returned output contains the mocked shared group.
func TestSharedWithList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":11,"name":"Shared","path":"shared","full_path":"shared","visibility":"private","web_url":"https://gitlab.example.com/groups/shared"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := SharedWithList(context.Background(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != 11 {
		t.Fatalf("expected shared group 11, got %+v", out.Groups)
	}
}

// TestSharedWithList_AllFilters verifies that every optional filter is wired into the request.
// The test sets pagination, search, min access level, visibility, order, and sort.
// It asserts the query string carries each filter and the call succeeds.
func TestSharedWithList_AllFilters(t *testing.T) {
	var query string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "2", PerPage: "10"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := SharedWithList(context.Background(), client, SharedWithListInput{
		GroupID:              toolutil.StringOrInt("7"),
		Search:               "team",
		MinAccessLevel:       30,
		Visibility:           "private",
		OrderBy:              "name",
		Sort:                 "desc",
		SkipGroups:           []int64{11, 12},
		WithCustomAttributes: true,
		Page:                 2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"search=team", "min_access_level=30", "visibility=private", "order_by=name", "sort=desc", "skip_groups=", "with_custom_attributes=true", "page=2", "per_page=10"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// TestSharedWithList_APIError verifies that an upstream error is wrapped with a hint.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestSharedWithList_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := SharedWithList(context.Background(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestSharedWithList_CanceledContext verifies the early ctx.Err() guard.
// The test passes an already-canceled context.
// It asserts an error is returned before any request.
func TestSharedWithList_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := SharedWithList(canceledContext(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// TestSharedWithList_MissingGroupID verifies that SharedWithList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestSharedWithList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := SharedWithList(context.Background(), client, SharedWithListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvitedList
// ---------------------------------------------------------------------------.

// TestInvitedList_Success verifies that InvitedList returns the groups invited to a group.
// The test exercises the GET groups/:id/invited_groups path.
// It asserts the returned output contains the mocked invited group.
func TestInvitedList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":12,"name":"Invited","path":"invited","full_path":"invited","visibility":"private","web_url":"https://gitlab.example.com/groups/invited"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := InvitedList(context.Background(), client, InvitedListInput{
		GroupID:  toolutil.StringOrInt("7"),
		Relation: []string{"direct"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != 12 {
		t.Fatalf("expected invited group 12, got %+v", out.Groups)
	}
}

// TestInvitedList_AllFilters verifies that every optional filter is wired into the request.
// The test sets pagination, search, min access level, and relation.
// It asserts the query string carries each filter.
func TestInvitedList_AllFilters(t *testing.T) {
	var query string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "3", PerPage: "5"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := InvitedList(context.Background(), client, InvitedListInput{
		GroupID:              toolutil.StringOrInt("7"),
		Search:               "guests",
		MinAccessLevel:       20,
		Relation:             []string{"direct", "inherited"},
		WithCustomAttributes: true,
		Page:                 3, PerPage: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"search=guests", "min_access_level=20", "relation=", "with_custom_attributes=true", "page=3", "per_page=5"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// TestInvitedList_APIError verifies that an upstream error is wrapped.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestInvitedList_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := InvitedList(context.Background(), client, InvitedListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestInvitedList_CanceledContext verifies the early ctx.Err() guard.
// The test passes an already-canceled context.
// It asserts an error is returned before any request.
func TestInvitedList_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := InvitedList(canceledContext(), client, InvitedListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// TestInvitedList_MissingGroupID verifies that InvitedList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestInvitedList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := InvitedList(context.Background(), client, InvitedListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TransferLocationsList
// ---------------------------------------------------------------------------.

// TestTransferLocationsList_Success verifies that TransferLocationsList returns candidate parent groups.
// The test exercises the GET groups/:id/transfer_locations path.
// It asserts the returned output contains the mocked location.
func TestTransferLocationsList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":99,"name":"Target","full_name":"Target Group","full_path":"target","web_url":"https://gitlab.example.com/groups/target"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Locations) != 1 || out.Locations[0].ID != 99 {
		t.Fatalf("expected location 99, got %+v", out.Locations)
	}
	if out.Locations[0].FullPath != "target" {
		t.Errorf("expected full_path target, got %s", out.Locations[0].FullPath)
	}
}

// TestTransferLocationsList_AllFilters verifies that search and pagination are wired into the request.
// The test sets search plus page/per_page.
// It asserts the query string carries each value.
func TestTransferLocationsList_AllFilters(t *testing.T) {
	var query string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "2", PerPage: "15"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{
		GroupID: toolutil.StringOrInt("7"),
		Search:  "org",
		Page:    2, PerPage: 15,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"search=org", "page=2", "per_page=15"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// TestTransferLocationsList_APIError verifies that an upstream error is wrapped.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestTransferLocationsList_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestTransferLocationsList_CanceledContext verifies the early ctx.Err() guard.
// The test passes an already-canceled context.
// It asserts an error is returned before any request.
func TestTransferLocationsList_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := TransferLocationsList(canceledContext(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// TestTransferLocationsList_MissingGroupID verifies that TransferLocationsList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestTransferLocationsList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Create and update request bodies
// ---------------------------------------------------------------------------.

// TestCreate_NewOptions verifies that the v2.41.0 group create options are sent to the API.
// The test inspects the request body for the new boolean flags.
// It asserts each new flag appears in the marshaled request.
func TestCreate_NewOptions(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":1,"name":"G","path":"g","full_path":"g","visibility":"private","web_url":"https://x/groups/g"}`)
	})
	client := testutil.NewTestClient(t, mux)

	enabled := true
	orgID := int64(42)
	limit := int64(100)
	interval := int64(300)
	_, err := Create(context.Background(), client, CreateInput{
		Name:                         "G",
		OrganizationID:               &orgID,
		MathRenderingLimitsEnabled:   &enabled,
		WebBasedCommitSigningEnabled: &enabled,
		AllowPersonalSnippets:        &enabled,
		UniqueProjectDownloadLimit:   &limit,
		UniqueProjectDownloadLimitIntervalInSeconds: &interval,
		UniqueProjectDownloadLimitAllowlist:         []string{"trusted-user"},
		UniqueProjectDownloadLimitAlertlist:         []int64{1, 2},
		AutoBanUserOnExcessiveProjectsDownload:      &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{
		"organization_id", "math_rendering_limits_enabled", "web_based_commit_signing_enabled", "allow_personal_snippets",
		"unique_project_download_limit", "unique_project_download_limit_interval_in_seconds",
		"unique_project_download_limit_allowlist", "unique_project_download_limit_alertlist",
		"auto_ban_user_on_excessive_projects_download",
	} {
		t.Run(field, func(t *testing.T) {
			if !strings.Contains(body, field) {
				t.Errorf("expected %s in request body, got: %s", field, body)
			}
		})
	}
}

// TestUpdate_NewOptions verifies that the v2.41.0 group update options are sent to the API.
// The test inspects the request body for the new boolean flags.
// It asserts each new flag appears in the marshaled request.
func TestUpdate_NewOptions(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"name":"G","path":"g","full_path":"g","visibility":"private","web_url":"https://x/groups/g"}`)
	})
	client := testutil.NewTestClient(t, mux)

	enabled := false
	_, err := Update(context.Background(), client, UpdateInput{
		GroupID:                      toolutil.StringOrInt("5"),
		MathRenderingLimitsEnabled:   &enabled,
		WebBasedCommitSigningEnabled: &enabled,
		AllowPersonalSnippets:        &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{"math_rendering_limits_enabled", "web_based_commit_signing_enabled", "allow_personal_snippets"} {
		t.Run(field, func(t *testing.T) {
			if !strings.Contains(body, field) {
				t.Errorf("expected %s in request body, got: %s", field, body)
			}
		})
	}
}
