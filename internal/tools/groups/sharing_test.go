// sharing_test.go contains unit tests for the group sharing, shared-project
// listing, and subgroup transfer MCP tool handlers. Tests use httptest to mock
// GitLab API responses and verify request method/path/body and output parsing.
package groups

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	pathGroupShare         = "/api/v4/groups/99/share"
	pathGroupSharedProj    = "/api/v4/groups/99/projects/shared"
	pathGroupTransfer      = "/api/v4/groups/99/transfer"
	sharingGroupJSON       = `{"id":99,"name":"infra","path":"infra","full_path":"org/infra","visibility":"private","web_url":"https://gitlab.example.com/groups/org/infra"}`
	sharingProjectListJSON = `[{"id":42,"name":"shared-proj","path_with_namespace":"other/shared-proj","visibility":"private","web_url":"https://gitlab.example.com/other/shared-proj","archived":false}]`
)

// TestShareGroupWithGroup_Success verifies the POST /share request body and output.
func TestShareGroupWithGroup_Success(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupShare {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, sharingGroupJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ShareGroupWithGroup(context.Background(), client, ShareGroupInput{
		GroupID:       "99",
		SharedGroupID: 123,
		GroupAccess:   30,
		ExpiresAt:     "2026-12-31",
	})
	if err != nil {
		t.Fatalf("ShareGroupWithGroup() unexpected error: %v", err)
	}
	if out.SharedGroupID != 123 || out.AccessRole != "Developer" || out.GroupAccess != 30 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !strings.Contains(gotBody, "123") || !strings.Contains(gotBody, "2026-12-31") {
		t.Fatalf("request body missing fields: %s", gotBody)
	}
}

// TestShareGroupWithGroup_Guards verifies the required-input guards: each case
// omits one required field, in the order the handler checks them.
func TestShareGroupWithGroup_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []struct {
		name string
		in   ShareGroupInput
	}{
		{name: "missing_group_id", in: ShareGroupInput{}},
		{name: "missing_shared_group_id", in: ShareGroupInput{GroupID: "99"}},
		{name: "missing_group_access", in: ShareGroupInput{GroupID: "99", SharedGroupID: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ShareGroupWithGroup(context.Background(), client, tc.in); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestShareGroupWithGroup_BadExpiresAt verifies an invalid date is rejected.
func TestShareGroupWithGroup_BadExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := ShareGroupWithGroup(context.Background(), client, ShareGroupInput{
		GroupID: "99", SharedGroupID: 1, GroupAccess: 30, ExpiresAt: "not-a-date",
	})
	if err == nil || !strings.Contains(err.Error(), "expires_at") {
		t.Fatalf("expected expires_at error, got: %v", err)
	}
}

// TestShareGroupWithGroup_Conflict verifies a 409/422 produces a hint.
func TestShareGroupWithGroup_Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ShareGroupWithGroup(context.Background(), client, ShareGroupInput{
		GroupID: "99", SharedGroupID: 1, GroupAccess: 30,
	})
	if err == nil || !strings.Contains(err.Error(), "group_access") {
		t.Fatalf("expected access-level hint, got: %v", err)
	}
}

// TestUnshareGroupFromGroup_Success verifies the DELETE /share/{id} request.
func TestUnshareGroupFromGroup_Success(t *testing.T) {
	var hit bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/99/share/123" {
			hit = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	if err := UnshareGroupFromGroup(context.Background(), client, UnshareGroupInput{GroupID: "99", SharedGroupID: 123}); err != nil {
		t.Fatalf("UnshareGroupFromGroup() unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("expected DELETE request to share endpoint")
	}
}

// TestUnshareGroupFromGroup_Guards verifies required-input guards.
func TestUnshareGroupFromGroup_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if err := UnshareGroupFromGroup(context.Background(), client, UnshareGroupInput{}); err == nil {
		t.Error("expected error for empty group_id")
	}
	if err := UnshareGroupFromGroup(context.Background(), client, UnshareGroupInput{GroupID: "99"}); err == nil {
		t.Error("expected error for zero shared_group_id")
	}
}

// TestUnshareGroupFromGroup_NotFound verifies a 404 produces a hint.
func TestUnshareGroupFromGroup_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	err := UnshareGroupFromGroup(context.Background(), client, UnshareGroupInput{GroupID: "99", SharedGroupID: 1})
	if err == nil || !strings.Contains(err.Error(), "shared_with_list") {
		t.Fatalf("expected shared_with_list hint, got: %v", err)
	}
}

// TestListSharedProjects_Success verifies the GET /projects/shared request,
// query filters, and output parsing.
func TestListSharedProjects_Success(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupSharedProj {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSONWithPagination(w, http.StatusOK, sharingProjectListJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	archived := false
	out, err := ListSharedProjects(context.Background(), client, ListSharedProjectsInput{
		GroupID:        "99",
		Search:         "shared",
		Archived:       &archived,
		MinAccessLevel: 30,
	})
	if err != nil {
		t.Fatalf("ListSharedProjects() unexpected error: %v", err)
	}
	if len(out.Projects) != 1 || out.Projects[0].ID != 42 {
		t.Fatalf("unexpected projects: %+v", out.Projects)
	}
	if !strings.Contains(gotQuery, "search=shared") || !strings.Contains(gotQuery, "min_access_level=30") {
		t.Fatalf("query missing filters: %s", gotQuery)
	}
}

// TestListSharedProjects_RequiresGroupID verifies the group_id guard.
func TestListSharedProjects_RequiresGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if _, err := ListSharedProjects(context.Background(), client, ListSharedProjectsInput{}); err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestListSharedProjects_NotFound verifies a 404 produces a hint.
func TestListSharedProjects_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := ListSharedProjects(context.Background(), client, ListSharedProjectsInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "shared *into*") {
		t.Fatalf("expected shared-into hint, got: %v", err)
	}
}

// TestTransferSubGroup_Success verifies the POST /transfer request with parent_id.
func TestTransferSubGroup_Success(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupTransfer {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, sharingGroupJSON)
			return
		}
		http.NotFound(w, r)
	}))

	parent := int64(42)
	out, err := TransferSubGroup(context.Background(), client, TransferSubGroupInput{GroupID: "99", ParentID: &parent})
	if err != nil {
		t.Fatalf("TransferSubGroup() unexpected error: %v", err)
	}
	if out.ID != 99 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !strings.Contains(gotBody, "42") {
		t.Fatalf("request body missing parent group_id: %s", gotBody)
	}
}

// TestTransferSubGroup_TopLevel verifies promotion to top level (no parent_id).
func TestTransferSubGroup_TopLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTransfer {
			testutil.RespondJSON(w, http.StatusOK, sharingGroupJSON)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := TransferSubGroup(context.Background(), client, TransferSubGroupInput{GroupID: "99"}); err != nil {
		t.Fatalf("TransferSubGroup() unexpected error: %v", err)
	}
}

// TestTransferSubGroup_RequiresGroupID verifies the group_id guard.
func TestTransferSubGroup_RequiresGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	if _, err := TransferSubGroup(context.Background(), client, TransferSubGroupInput{}); err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestTransferSubGroup_Forbidden verifies a 403 produces an Owner-role hint.
func TestTransferSubGroup_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := TransferSubGroup(context.Background(), client, TransferSubGroupInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "Owner role") {
		t.Fatalf("expected Owner-role hint, got: %v", err)
	}
}

// TestTransferSubGroup_NotFound verifies that a status which is neither 403
// nor 400 (here, 404) falls through both dedicated hint branches to the
// final gitlab_group_get verification hint. Without this test the fallback
// WrapErrWithStatusHint call at the end of TransferSubGroup's error handling
// is never exercised, and a regression there (e.g. losing the hint) would go
// unnoticed since only err != nil would be implicitly checked elsewhere.
func TestTransferSubGroup_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := TransferSubGroup(context.Background(), client, TransferSubGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "groupTransferSubGroup") {
		t.Errorf("expected operation name in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gitlab_group_get") {
		t.Errorf("expected gitlab_group_get verification hint, got: %v", err)
	}
	if strings.Contains(err.Error(), "Owner role") || strings.Contains(err.Error(), "destination") {
		t.Errorf("404 should not use the 403/400 hints, got: %v", err)
	}
}

// TestFormatShareGroupMarkdown verifies the share confirmation Markdown.
func TestFormatShareGroupMarkdown(t *testing.T) {
	md := FormatShareGroupMarkdown(ShareGroupOutput{
		Message: "Group 99 shared with group 123 as Developer", AccessRole: "Developer", GroupAccess: 30,
	})
	if !strings.Contains(md, "Developer") || !strings.Contains(md, "shared with group 123") {
		t.Errorf("unexpected markdown:\n%s", md)
	}
}

// TestFormatSharedProjectsListMarkdown verifies the shared-projects table and
// the empty-list path.
func TestFormatSharedProjectsListMarkdown(t *testing.T) {
	md := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{
		Projects: []ProjectItem{{ID: 42, Name: "shared-proj", PathWithNamespace: "other/shared-proj", Visibility: "private", WebURL: "https://x/y"}},
	})
	if !strings.Contains(md, "Shared Projects") || !strings.Contains(md, "shared-proj") {
		t.Errorf("unexpected markdown:\n%s", md)
	}
	empty := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{})
	if !strings.Contains(empty, "No shared projects") {
		t.Errorf("expected empty-list message:\n%s", empty)
	}
}

// TestListSharedProjects_AllFilters exercises every option setter.
func TestListSharedProjects_AllFilters(t *testing.T) {
	var q string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, sharingProjectListJSON)
	}))
	b := true
	_, err := ListSharedProjects(context.Background(), client, ListSharedProjectsInput{
		GroupID:                  "99",
		Archived:                 &b,
		MinAccessLevel:           40,
		OrderBy:                  "name",
		Search:                   "x",
		Simple:                   &b,
		Sort:                     "asc",
		Starred:                  &b,
		Visibility:               "private",
		WithCustomAttributes:     &b,
		WithIssuesEnabled:        &b,
		WithMergeRequestsEnabled: &b,
	})
	if err != nil {
		t.Fatalf("ListSharedProjects() error: %v", err)
	}
	for _, want := range []string{"order_by=name", "simple=true", "sort=asc", "starred=true", "visibility=private", "with_custom_attributes=true", "with_issues_enabled=true", "with_merge_requests_enabled=true"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(q, want) {
				t.Errorf("query missing %q: %s", want, q)
			}
		})
	}
}

// TestTransferSubGroup_BadRequest verifies a 400 produces a destination hint.
func TestTransferSubGroup_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := TransferSubGroup(context.Background(), client, TransferSubGroupInput{GroupID: "99"})
	if err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("expected destination hint, got: %v", err)
	}
}

// TestAccessLevelNameFallback verifies the unknown-level fallback path.
func TestAccessLevelNameFallback(t *testing.T) {
	if got := accessLevelName(99); !strings.Contains(got, "99") {
		t.Errorf("accessLevelName(99) = %q, want fallback with level number", got)
	}
}

// TestSharing_CanceledContext verifies each handler honors a canceled context.
func TestSharing_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ShareGroupWithGroup(ctx, client, ShareGroupInput{GroupID: "99", SharedGroupID: 1, GroupAccess: 30}); err == nil {
		t.Error("ShareGroupWithGroup: expected context error")
	}
	if err := UnshareGroupFromGroup(ctx, client, UnshareGroupInput{GroupID: "99", SharedGroupID: 1}); err == nil {
		t.Error("UnshareGroupFromGroup: expected context error")
	}
	if _, err := ListSharedProjects(ctx, client, ListSharedProjectsInput{GroupID: "99"}); err == nil {
		t.Error("ListSharedProjects: expected context error")
	}
	if _, err := TransferSubGroup(ctx, client, TransferSubGroupInput{GroupID: "99"}); err == nil {
		t.Error("TransferSubGroup: expected context error")
	}
}

// TestShareGroupWithGroup_MemberRoleAndErrorFallthrough verifies the
// member_role_id option is forwarded when set and that non-422/400 API
// failures take the NotFound-hint fallthrough branch.
func TestShareGroupWithGroup_MemberRoleAndErrorFallthrough(t *testing.T) {
	roleID := int64(9)

	okClient := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "member_role_id") {
			t.Errorf("request body missing member_role_id: %s", body)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id": 42}`)
	}))
	if _, err := ShareGroupWithGroup(t.Context(), okClient, ShareGroupInput{GroupID: "42", SharedGroupID: 7, GroupAccess: 30, MemberRoleID: &roleID}); err != nil {
		t.Fatalf("ShareGroupWithGroup with member role error = %v", err)
	}

	failClient := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := ShareGroupWithGroup(t.Context(), failClient, ShareGroupInput{GroupID: "42", SharedGroupID: 7, GroupAccess: 30})
	if err == nil || !strings.Contains(err.Error(), "groupShareWithGroup") {
		t.Errorf("fallthrough err = %v, want groupShareWithGroup-wrapped error", err)
	}
}

// TestTransferSubGroup_BadRequestHint verifies an HTTP 400 transfer failure
// surfaces the invalid-destination hint branch instead of the generic
// not-found wrap.
func TestTransferSubGroup_BadRequestHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	parentID := int64(7)
	_, err := TransferSubGroup(t.Context(), client, TransferSubGroupInput{GroupID: "42", ParentID: &parentID})
	if err == nil || !strings.Contains(err.Error(), "gitlab_group_transfer_locations") {
		t.Errorf("TransferSubGroup 400 err = %v, want invalid-destination hint", err)
	}
}

// TestFormatSharedProjectsListMarkdown_ArchivedRow verifies the shared
// projects table marks archived projects with "Yes" in the Archived column.
func TestFormatSharedProjectsListMarkdown_ArchivedRow(t *testing.T) {
	md := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{Projects: []ProjectItem{
		{ID: 1, Name: "arch", Archived: true},
	}})
	if !strings.Contains(md, "| Yes |") {
		t.Errorf("markdown missing archived Yes cell:\n%s", md)
	}
}
