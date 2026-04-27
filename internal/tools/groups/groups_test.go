// groups_test.go contains unit tests for GitLab group operations (list, get,
// list members, list subgroups). Tests use httptest to mock the GitLab API
// and verify success paths, search/query filtering, ownership filters,
// pagination, and error handling including context cancellation.

package groups

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
var groupDetailJSON = `{"id":99,"name":"infrastructure","path":"infra","full_path":"org/infra","full_name":"Org / Infrastructure","description":"Infra group","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra","parent_id":1,"created_at":"2026-01-15T10:00:00Z","marked_for_deletion_on":"2026-06-01"}`

// groupMembersJSON is a JSON response fixture containing two group members.
var groupMembersJSON = `[{"id":10,"username":"devops1","name":"DevOps One","state":"active","access_level":40,"web_url":"https://gitlab.example.com/devops1"},{"id":11,"username":"devops2","name":"DevOps Two","state":"active","access_level":30,"web_url":"https://gitlab.example.com/devops2"}]`

// subgroupsJSON is a JSON response fixture containing one descendant group.
var subgroupsJSON = `[{"id":100,"name":"monitoring","path":"monitoring","full_path":"org/infra/monitoring","description":"Monitoring subgroup","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra/monitoring","parent_id":99}]`

// TestGroupList_Success verifies that List retrieves groups and correctly
// maps name, full path, parent ID, and pagination metadata.
func TestGroupList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroups {
			testutil.RespondJSONWithPagination(w, http.StatusOK, groupListJSON,
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

// TestGroupList_WithSearch verifies that List forwards the search query
// parameter to the GitLab API.
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

// TestGroupList_Owned verifies that List passes the owned=true filter
// to the GitLab API when the Owned field is set.
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

// TestGroupList_Empty verifies that List returns an empty slice when
// the GitLab API returns no groups.
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

// TestGroupList_APIError verifies that List propagates an API error
// Server Error returned by the GitLab API.
func TestGroupList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for API error response, got nil")
	}
}

// TestGroupList_CancelledContext verifies that List returns an error
// immediately when called with an already-canceled context.
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

// TestGroupGet_Success verifies that Get retrieves a single group by ID
// and correctly maps name, ID, and visibility to the output struct.
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

// TestGroupGet_NotFound verifies that Get returns an error when the
// GitLab API responds with 404 for a non-existent group.
func TestGroupGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: "999"})
	if err == nil {
		t.Fatal("Get() expected error for 404 response, got nil")
	}
}

// TestGroupGet_CancelledContext verifies that Get returns an error
// immediately when called with an already-canceled context.
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
			testutil.RespondJSONWithPagination(w, http.StatusOK, groupMembersJSON,
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
	if out.Members[0].AccessLevelDescription != "Maintainer" {
		t.Errorf("out.Members[0].AccessLevelDescription = %q, want %q", out.Members[0].AccessLevelDescription, "Maintainer")
	}
	if out.Members[1].AccessLevelDescription != "Developer" {
		t.Errorf("out.Members[1].AccessLevelDescription = %q, want %q", out.Members[1].AccessLevelDescription, "Developer")
	}
}

// TestGroupMembersList_WithQuery verifies that MembersList forwards
// the query filter parameter to the GitLab API.
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

// TestGroupMembersList_Empty verifies that MembersList returns an empty
// member slice when the GitLab API returns no members.
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

// TestGroupMembersList_APIError verifies that MembersList propagates a
// 403 Forbidden error returned by the GitLab API.
func TestGroupMembersList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := MembersList(context.Background(), client, MembersListInput{GroupID: "99"})
	if err == nil {
		t.Fatal("MembersList() expected error for 403 response, got nil")
	}
}

// TestGroupMembersList_CancelledContext verifies that MembersList returns
// an error immediately when called with an already-canceled context.
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

// TestSubgroupsList_Success verifies that SubgroupsList retrieves descendant
// groups with correct name, parent ID, and pagination metadata.
func TestSubgroupsList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupSubgroups {
			testutil.RespondJSONWithPagination(w, http.StatusOK, subgroupsJSON,
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

// TestSubgroupsList_WithSearch verifies that SubgroupsList forwards the
// search query parameter to the GitLab API.
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

// TestSubgroupsList_Empty verifies that SubgroupsList returns an empty slice
// when the GitLab API returns no descendant groups.
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

// TestSubgroupsList_APIError verifies that SubgroupsList propagates a 404
// error returned by the GitLab API for a non-existent parent group.
func TestSubgroupsList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := SubgroupsList(context.Background(), client, SubgroupsListInput{GroupID: "999"})
	if err == nil {
		t.Fatal("SubgroupsList() expected error for 404 response, got nil")
	}
}

// TestSubgroupsList_CancelledContext verifies that SubgroupsList returns an
// error immediately when called with an already-canceled context.
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

// TestGroupGetSuccess_EnrichedFields verifies that Get maps the enriched
// fields: FullName, CreatedAt, MarkedForDeletion.
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

// TestGroupListInput_EnrichedFilters verifies that List passes new filters
// (order_by, sort, visibility) as query parameters.
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

// TestGroupMembers_ListSAMLAndRoleFields verifies that MembersList maps
// GroupSAMLProvider and MemberRoleName from the API response.
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
	if out.Members[0].GroupSAMLProvider != "okta-saml" {
		t.Errorf("out.Members[0].GroupSAMLProvider = %q, want %q", out.Members[0].GroupSAMLProvider, "okta-saml")
	}
	if out.Members[0].MemberRoleName != "Custom Dev" {
		t.Errorf("out.Members[0].MemberRoleName = %q, want %q", out.Members[0].MemberRoleName, "Custom Dev")
	}
}

// TestSubgroupsList_EnrichedFilters verifies that SubgroupsList passes the new
// filter query params: AllAvailable, Owned, MinAccessLevel, OrderBy, Sort, Statistics.
func TestSubgroupsList_EnrichedFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupSubgroups {
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
			testutil.RespondJSON(w, http.StatusOK, subgroupsJSON)
			return
		}
		http.NotFound(w, r)
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

// TestGroupList_EnrichedNewFilters verifies that List passes the new
// params: AllAvailable, Statistics, WithCustomAttributes, SkipGroups.
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

// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------.

const (
	fmtGroupCreateErr          = "Create() unexpected error: %v"
	fmtGroupUpdateErr          = "Update() unexpected error: %v"
	fmtGroupDeleteErr          = "Delete() unexpected error: %v"
	fmtGroupRestoreErr         = "Restore() unexpected error: %v"
	fmtGroupSearchErr          = "Search() unexpected error: %v"
	fmtGroupTransferProjectErr = "TransferProject() unexpected error: %v"
	fmtGroupListProjectsErr    = "ListProjects() unexpected error: %v"
	pathGroup99Restore         = "/api/v4/groups/99/restore"
	pathGroup99Projects        = "/api/v4/groups/99/projects"
	pathGroup99Transfer        = "/api/v4/groups/99/transfer"
)

var groupProjectsJSON = `[{"id":42,"name":"my-project","path_with_namespace":"org/infra/my-project","description":"A project","visibility":"private","web_url":"https://gitlab.example.com/org/infra/my-project","default_branch":"main","archived":false,"created_at":"2026-02-01T12:00:00Z"}]`

// TestGroupCreate_Success verifies the behavior of group create success.
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

// TestGroupCreate_MissingName verifies the behavior of group create missing name.
func TestGroupCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("Create() expected error for missing name, got nil")
	}
}

// TestGroupCreateServer_Error verifies the behavior of group create server error.
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

// TestGroupUpdate_Success verifies the behavior of group update success.
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

// TestGroupUpdate_MissingGroupID verifies the behavior of group update missing group i d.
func TestGroupUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal("Update() expected error for missing group_id, got nil")
	}
}

// TestGroupUpdateServer_Error verifies the behavior of group update server error.
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

// TestGroupDelete_Success verifies the behavior of group delete success.
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

// TestGroupDelete_MissingGroupID verifies the behavior of group delete missing group i d.
func TestGroupDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id, got nil")
	}
}

// TestGroupDeleteServer_Error verifies the behavior of group delete server error.
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

// TestGroupRestore_Success verifies the behavior of group restore success.
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

// TestGroupRestore_MissingGroupID verifies the behavior of group restore missing group i d.
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

// TestGroupSearch_Success verifies the behavior of group search success.
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

// TestGroupSearch_MissingQuery verifies the behavior of group search missing query.
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

// TestGroupTransferProject_Success verifies the behavior of group transfer project success.
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

// TestGroupTransferProject_MissingGroupID verifies the behavior of group transfer project missing group i d.
func TestGroupTransferProject_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := TransferProject(context.Background(), client, TransferInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("TransferProject() expected error for missing group_id, got nil")
	}
}

// TestGroupTransferProject_MissingProjectID verifies the behavior of group transfer project missing project i d.
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

// TestGroupListProjects_Success verifies the behavior of group list projects success.
func TestGroupListProjects_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroup99Projects {
			testutil.RespondJSONWithPagination(w, http.StatusOK, groupProjectsJSON,
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

// TestGroupListProjects_MissingGroupID verifies the behavior of group list projects missing group i d.
func TestGroupListProjects_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListProjects(context.Background(), client, ListProjectsInput{})
	if err == nil {
		t.Fatal("ListProjects() expected error for missing group_id, got nil")
	}
}

// TestGroupListProjectsServer_Error verifies the behavior of group list projects server error.
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

const errExpCancelledNil = "expected error for canceled context, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Get — missing group_id
// ---------------------------------------------------------------------------.

// TestGet_MissingGroupID verifies the behavior of cov get missing group i d.
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

// TestMembersList_MissingGroupID verifies the behavior of cov members list missing group i d.
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

// TestSubgroupsList_MissingGroupID verifies the behavior of cov subgroups list missing group i d.
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

// TestList_WithPagination verifies the behavior of cov list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("page"); got != "2" {
			t.Errorf("query param page = %q, want %q", got, "2")
		}
		if got := q.Get("per_page"); got != "5" {
			t.Errorf("query param per_page = %q, want %q", got, "5")
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"},
		)
	}))
	out, err := List(context.Background(), client, ListInput{
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
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

// TestList_TopLevelOnly verifies the behavior of cov list top level only.
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

// TestMembersList_WithPagination verifies the behavior of cov members list with pagination.
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
		GroupID:         "99",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// SubgroupsList — with pagination params
// ---------------------------------------------------------------------------.

// TestSubgroupsList_WithPagination verifies the behavior of cov subgroups list with pagination.
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
		GroupID:         "99",
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 15},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Create — canceled context, with all optional fields
// ---------------------------------------------------------------------------.

// TestCreate_CancelledContext verifies the behavior of cov create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{Name: "g"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCreate_AllOptionalFields verifies the behavior of cov create all optional fields.
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

// TestUpdate_CancelledContext verifies the behavior of cov update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{GroupID: "99", Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUpdate_AllOptionalFields verifies the behavior of cov update all optional fields.
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
// Delete — canceled context, with permanently_remove
// ---------------------------------------------------------------------------.

// TestDelete_CancelledContext verifies the behavior of cov delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDelete_PermanentlyRemove verifies the behavior of cov delete permanently remove.
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

// TestRestore_CancelledContext verifies the behavior of cov restore cancelled context.
func TestRestore_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Restore(ctx, client, RestoreInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRestore_ServerError verifies the behavior of cov restore server error.
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

// TestArchive_Success verifies that archiving a group calls the correct endpoint.
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

// TestArchive_CancelledContext verifies archive respects context cancellation.
func TestArchive_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Archive(ctx, client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestArchive_Forbidden verifies archive returns a hint on 403.
func TestArchive_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Archive(context.Background(), client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error on forbidden, got nil")
	}
}

// TestArchive_EmptyGroupID verifies archive requires group_id.
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

// TestUnarchive_Success verifies that unarchiving a group calls the correct endpoint.
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

// TestUnarchive_CancelledContext verifies unarchive respects context cancellation.
func TestUnarchive_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Unarchive(ctx, client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUnarchive_Forbidden verifies unarchive returns a hint on 403.
func TestUnarchive_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Unarchive(context.Background(), client, ArchiveInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error on forbidden, got nil")
	}
}

// TestUnarchive_EmptyGroupID verifies unarchive requires group_id.
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

// TestSearch_CancelledContext verifies the behavior of cov search cancelled context.
func TestSearch_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Search(ctx, client, SearchInput{Query: "q"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestSearch_ServerError verifies the behavior of cov search server error.
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

// TestTransferProject_CancelledContext verifies the behavior of cov transfer project cancelled context.
func TestTransferProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := TransferProject(ctx, client, TransferInput{GroupID: "99", ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestTransferProject_ServerError verifies the behavior of cov transfer project server error.
func TestTransferProject_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := TransferProject(context.Background(), client, TransferInput{GroupID: "99", ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error on server failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListProjects — canceled context, with optional filter fields
// ---------------------------------------------------------------------------.

// TestListProjects_CancelledContext verifies the behavior of cov list projects cancelled context.
func TestListProjects_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProjects(ctx, client, ListProjectsInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListProjects_AllOptionalFilters verifies the behavior of cov list projects all optional filters.
func TestListProjects_AllOptionalFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/99/projects" {
			q := r.URL.Query()
			if got := q.Get("search"); got != "myapp" {
				t.Errorf("search = %q, want %q", got, "myapp")
			}
			if got := q.Get("visibility"); got != "private" {
				t.Errorf("visibility = %q, want %q", got, "private")
			}
			if got := q.Get("order_by"); got != "name" {
				t.Errorf("order_by = %q, want %q", got, "name")
			}
			if got := q.Get("sort"); got != "asc" {
				t.Errorf("sort = %q, want %q", got, "asc")
			}
			if got := q.Get("simple"); got != "true" {
				t.Errorf("simple = %q, want %q", got, "true")
			}
			if got := q.Get("owned"); got != "true" {
				t.Errorf("owned = %q, want %q", got, "true")
			}
			if got := q.Get("starred"); got != "true" {
				t.Errorf("starred = %q, want %q", got, "true")
			}
			if got := q.Get("include_subgroups"); got != "true" {
				t.Errorf("include_subgroups = %q, want %q", got, "true")
			}
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
		PaginationInput:  toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// ListHooks — canceled context, missing group_id, empty result
// ---------------------------------------------------------------------------.

// TestListHooks_CancelledContext verifies the behavior of cov list hooks cancelled context.
func TestListHooks_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListHooks(ctx, client, ListHooksInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListHooks_MissingGroupID verifies the behavior of cov list hooks missing group i d.
func TestListHooks_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := ListHooks(context.Background(), client, ListHooksInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListHooks_Empty verifies the behavior of cov list hooks empty.
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

// TestListHooks_WithPagination verifies the behavior of cov list hooks with pagination.
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
		GroupID:         "99",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// GetHook — canceled context, missing group_id
// ---------------------------------------------------------------------------.

// TestGetHook_CancelledContext verifies the behavior of cov get hook cancelled context.
func TestGetHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetHook(ctx, client, GetHookInput{GroupID: "99", HookID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetHook_MissingGroupID verifies the behavior of cov get hook missing group i d.
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

// TestAddHook_CancelledContext verifies the behavior of cov add hook cancelled context.
func TestAddHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddHook(ctx, client, AddHookInput{
		GroupID:   "99",
		HookInput: HookInput{URL: "https://example.com"},
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestAddHook_MissingGroupID verifies the behavior of cov add hook missing group i d.
func TestAddHook_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := AddHook(context.Background(), client, AddHookInput{
		HookInput: HookInput{URL: "https://example.com"},
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestAddHook_MissingURL verifies the behavior of cov add hook missing u r l.
func TestAddHook_MissingURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := AddHook(context.Background(), client, AddHookInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
}

// TestAddHook_AllOptionalFields verifies the behavior of cov add hook all optional fields.
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
		GroupID: "99",
		HookInput: HookInput{
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
		},
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

// TestEditHook_CancelledContext verifies the behavior of cov edit hook cancelled context.
func TestEditHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := EditHook(ctx, client, EditHookInput{
		GroupID:   "99",
		HookID:    10,
		HookInput: HookInput{URL: "https://example.com"},
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestEditHook_MissingGroupID verifies the behavior of cov edit hook missing group i d.
func TestEditHook_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditHook(context.Background(), client, EditHookInput{
		HookID:    10,
		HookInput: HookInput{URL: "https://example.com"},
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestEditHook_AllOptionalFields verifies the behavior of cov edit hook all optional fields.
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
		GroupID: "99",
		HookID:  10,
		HookInput: HookInput{
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
		},
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

// TestDeleteHook_CancelledContext verifies the behavior of cov delete hook cancelled context.
func TestDeleteHook_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteHook(ctx, client, DeleteHookInput{GroupID: "99", HookID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDeleteHook_MissingGroupID verifies the behavior of cov delete hook missing group i d.
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

// TestFormatOutputMarkdown_WithData verifies the behavior of format output markdown with data.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_Minimal verifies the behavior of format output markdown minimal.
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
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
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

// TestFormatMemberListMarkdown_WithData verifies the behavior of format member list markdown with data.
func TestFormatMemberListMarkdown_WithData(t *testing.T) {
	out := MemberListOutput{
		Members: []MemberOutput{
			{ID: 10, Username: "devops1", Name: "DevOps One", AccessLevelDescription: "Maintainer", State: "active"},
			{ID: 11, Username: "devops2", Name: "DevOps Two", AccessLevelDescription: "Developer", State: "active"},
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatMemberListMarkdown_Empty verifies the behavior of format member list markdown empty.
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

// TestFormatListProjectsMarkdown_WithData verifies the behavior of format list projects markdown with data.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListProjectsMarkdown_Empty verifies the behavior of format list projects markdown empty.
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

// TestFormatHookMarkdown_WithNameAndAllEvents verifies the behavior of format hook markdown with name and all events.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatHookMarkdown_WithoutName verifies the behavior of format hook markdown without name.
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

// TestFormatHookMarkdown_NoEventsEnabled verifies the behavior of format hook markdown no events enabled.
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

// TestFormatHookListMarkdown_WithData verifies the behavior of format hook list markdown with data.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatHookListMarkdown_Empty verifies the behavior of format hook list markdown empty.
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

// TestEnabledEvents_All verifies the behavior of enabled events all.
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
		if !strings.Contains(result, want) {
			t.Errorf("enabledEvents missing %q: %s", want, result)
		}
	}
}

// TestEnabledEvents_None verifies the behavior of enabled events none.
func TestEnabledEvents_None(t *testing.T) {
	result := enabledEvents(HookOutput{})
	if result != "none" {
		t.Errorf("enabledEvents = %q, want %q", result, "none")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 16 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newGroupsMCPSession(t)
	ctx := context.Background()

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
		{"search", "gitlab_group_search", map[string]any{"query": "infra"}},
		{"transfer_project", "gitlab_group_transfer_project", map[string]any{"group_id": "99", "project_id": "42"}},
		{"list_projects", "gitlab_group_projects", map[string]any{"group_id": "99"}},
		{"hook_list", "gitlab_group_hook_list", map[string]any{"group_id": "99"}},
		{"hook_get", "gitlab_group_hook_get", map[string]any{"group_id": "99", "hook_id": 10}},
		{"hook_add", "gitlab_group_hook_add", map[string]any{"group_id": "99", "url": "https://example.com/hook"}},
		{"hook_edit", "gitlab_group_hook_edit", map[string]any{"group_id": "99", "hook_id": 10, "url": "https://example.com/hook2"}},
		{"hook_delete", "gitlab_group_hook_delete", map[string]any{"group_id": "99", "hook_id": 10}},
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

// newGroupsMCPSession is an internal helper for the groups package.
func newGroupsMCPSession(t *testing.T) *mcp.ClientSession {
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

	// Transfer project into group
	handler.HandleFunc("POST /api/v4/groups/99/projects/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// List group projects
	handler.HandleFunc("GET /api/v4/groups/99/projects", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectJSON)
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

// TestGroupGet_EmbedsCanonicalResource asserts gitlab_group_get attaches
// an EmbeddedResource block with URI gitlab://group/{id}.
func TestGroupGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":10,"name":"G","path":"g","full_path":"g","web_url":"https://gitlab.example.com/groups/g","visibility":"private"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"group_id": "10"}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_group_get", args, "gitlab://group/10", toolutil.EnableEmbeddedResources)
}
