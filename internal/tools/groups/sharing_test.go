// sharing_test.go contains unit tests for the group sharing and shared-project
// listing MCP tool handlers. Tests use httptest to mock GitLab API responses
// and verify request method/path/body and output parsing.
package groups

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	pathGroupShare         = "/api/v4/groups/99/share"
	pathGroupSharedProj    = "/api/v4/groups/99/projects/shared"
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

// TestShareGroupWithGroup_StatusHints verifies each status POST
// /groups/:id/share answers with gets the hint for what GitLab means by it,
// and nothing else: 400 is a parameter Grape refused, 409 every link the model
// would not save (an existing share among them, which the 400 hint used to
// claim), a plain 401 or 403 a credential refused a permission, and 404 a
// caller who may not link either group or a hierarchy that keeps shares
// inside it. A 403 naming an RFC 6750 error code refuses the token's scope,
// and a status the route never uses (422, 500) carries no hint at all.
func TestShareGroupWithGroup_StatusHints(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    []string
		notWant []string
	}{
		{
			name: "400", status: http.StatusBadRequest, body: `{"error":"group_access does not have a valid value"}`,
			want:    []string{"group_access must be 10/15/20/25/30/40/50", "5 (Minimal access) on a Premium or Ultimate top-level group", "does not have a valid value"},
			notWant: []string{"already shared"},
		},
		{
			name: "409", status: http.StatusConflict, body: `{"message":"Shared group has already been taken"}`,
			want: []string{
				"already be shared with this group", "group.shared_with", "group.unshare_from_group",
				"must belong to this group's top-level group", "base access level must equal group_access",
				"5 (Minimal access) needs a Premium or Ultimate license", "allowed domains must be a subset", "has already been taken",
			},
		},
		{
			name: "plain 403", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`,
			want: []string{"needs the share_group permission", "answered 404 rather than 403"},
		},
		{
			name: "plain 401", status: http.StatusUnauthorized, body: `{"message":"401 Unauthorized"}`,
			want: []string{"needs the share_group permission"},
		},
		{
			name: "403 insufficient scope", status: http.StatusForbidden, body: `{"error":"insufficient_scope"}`,
			notWant: []string{"share_group permission", "Suggestion"},
		},
		{
			name: "404", status: http.StatusNotFound, body: `{"message":"404 Not Found"}`,
			want: []string{"verify group_id and shared_group_id with group.get", "the Owner role grants that", "prevents sharing outside its hierarchy"},
		},
		{
			name: "422", status: http.StatusUnprocessableEntity, body: `{"message":"unprocessable"}`,
			want: []string{"groupShareWithGroup"}, notWant: []string{"Suggestion"},
		},
		{
			name: "500", status: http.StatusInternalServerError, body: `{"message":"boom"}`,
			want: []string{"groupShareWithGroup"}, notWant: []string{"Suggestion"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, tt.body)
			}))
			_, err := ShareGroupWithGroup(t.Context(), client, ShareGroupInput{GroupID: "99", SharedGroupID: 1, GroupAccess: 30})
			if err == nil {
				t.Fatalf("ShareGroupWithGroup() = nil error on %d", tt.status)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not carry %q", err, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("error %q carries %q", err, notWant)
				}
			}
		})
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
	if err == nil || !strings.Contains(err.Error(), "group.shared_with") {
		t.Fatalf("expected group.shared_with hint, got: %v", err)
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

// TestFormatShareGroupMarkdown verifies the share confirmation card byte for
// byte: the confirmation is the heading, the share's own fields are list rows
// rather than a two-column table, and the hints close the card.
func TestFormatShareGroupMarkdown(t *testing.T) {
	md := FormatShareGroupMarkdown(ShareGroupOutput{
		Message: "Group 99 shared with group 123 as Developer", SharedGroupID: 123, AccessRole: "Developer", GroupAccess: 30,
	})

	want := "## ✅ Group 99 shared with group 123 as Developer\n\n" +
		"- **Shared Group ID**: 123\n" +
		"- **Access**: Developer\n" +
		"- **Access Level**: 30\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.shared_with' to confirm the share\n" +
		"- Use action 'group.unshare_from_group' to revoke it\n"
	if md != want {
		t.Errorf("share card:\n got %q\nwant %q", md, want)
	}
}

// TestFormatSharedProjectsListMarkdown verifies the whole shared-projects
// response: the heading counts what GitLab reported, the table carries its own
// header, and the guidance closes the response rather than opening it.
func TestFormatSharedProjectsListMarkdown(t *testing.T) {
	md := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{
		Projects:   []ProjectItem{{ID: 42, Name: "shared-proj", PathWithNamespace: "other/shared-proj", Visibility: "private", WebURL: "https://x/y", Archived: new(false)}},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1, TotalItems: 1, PerPage: 20},
	})

	want := "## Shared Projects (1)\n\n" +
		"| ID | Name | Path | Visibility | Archived |\n| --- | --- | --- | --- | --- |\n" +
		"| 42 | [shared-proj](https://x/y) | other/shared-proj | private | ❌ |\n" +
		"\nPage 1 of 1 | 1 items total | 20 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'project.get' to view a shared project's details\n"
	if md != want {
		t.Errorf("shared projects list:\n got %q\nwant %q", md, want)
	}

	if empty := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{}); empty != "No shared projects found.\n" {
		t.Errorf("empty list = %q, want the one-sentence empty message", empty)
	}
}

// TestListSharedProjects_AllFilters sends every option at once and holds the
// query to eight of them by name and value; the access level and the search
// are held by [TestListSharedProjects_Success], and the archived flag by
// [TestApplyListSharedProjectsOptions_CarriesTheFiltersOnlyWhenGiven].
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

// TestShareGroupWithGroup_NamesTheRoleFromTheSharedTable verifies the granted
// role is read from toolutil's access-level table rather than a copy of it:
// the package-local copy knew six levels, so a Planner share reported "Level
// 15" and a level outside both tables still reports its number.
func TestShareGroupWithGroup_NamesTheRoleFromTheSharedTable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id": 42}`)
	}))
	for _, tc := range []struct {
		name  string
		level int
		want  string
	}{
		{name: "planner", level: 15, want: "Planner"},
		{name: "minimal access", level: 5, want: "Minimal access"},
		{name: "unknown level", level: 99, want: "Level 99"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ShareGroupWithGroup(t.Context(), client, ShareGroupInput{GroupID: "99", SharedGroupID: 7, GroupAccess: tc.level})
			if err != nil {
				t.Fatalf("ShareGroupWithGroup() error: %v", err)
			}
			if out.AccessRole != tc.want {
				t.Errorf("access role = %q, want %q", out.AccessRole, tc.want)
			}
		})
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
}

// TestShareGroupWithGroup_MemberRoleID_SentOnlyWhenGiven verifies the
// member_role_id option reaches the body with its value when set, and that a
// share without one carries no member_role_id key at all: the zero value is
// "no custom role", as it is on group.group_member_share, and sending 0 would
// ask GitLab for a role that does not exist.
func TestShareGroupWithGroup_MemberRoleID_SentOnlyWhenGiven(t *testing.T) {
	tests := []struct {
		name     string
		roleID   int64
		wantBody string
		wantKey  bool
	}{
		{name: "given", roleID: 9, wantBody: `"member_role_id":9`, wantKey: true},
		{name: "zero", roleID: 0, wantKey: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				testutil.RespondJSON(w, http.StatusOK, `{"id": 42}`)
			}))
			if _, err := ShareGroupWithGroup(t.Context(), client, ShareGroupInput{GroupID: "42", SharedGroupID: 7, GroupAccess: 30, MemberRoleID: tt.roleID}); err != nil {
				t.Fatalf("ShareGroupWithGroup() error = %v", err)
			}
			if got := strings.Contains(gotBody, `"member_role_id"`); got != tt.wantKey {
				t.Errorf("body %s carries member_role_id = %v, want %v", gotBody, got, tt.wantKey)
			}
			if tt.wantBody != "" && !strings.Contains(gotBody, tt.wantBody) {
				t.Errorf("body %s does not carry %s", gotBody, tt.wantBody)
			}
		})
	}
}

// TestFormatSharedProjectsListMarkdown_ArchivedRow verifies the shared
// projects table marks an archived project with the flag glyph every other
// boolean in this tree renders with.
func TestFormatSharedProjectsListMarkdown_ArchivedRow(t *testing.T) {
	md := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{Projects: []ProjectItem{
		{ID: 1, Name: "arch", Archived: new(true)},
	}})
	if !strings.Contains(md, "| 1 | arch |  |  | ✅ |\n") {
		t.Errorf("markdown missing the archived row:\n%s", md)
	}
}

// TestFormatSharedProjectsListMarkdown_SimpleRowLeavesArchivedUnanswered
// verifies a row GitLab rendered as BasicProjectDetails, which carries no
// archived flag, is not shown as a project that is not archived.
func TestFormatSharedProjectsListMarkdown_SimpleRowLeavesArchivedUnanswered(t *testing.T) {
	md := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{Projects: []ProjectItem{
		{ID: 1, Name: "basic"},
	}})
	if !strings.Contains(md, "| 1 | basic |  |  |  |\n") {
		t.Errorf("a row with no archived flag was given an answer:\n%s", md)
	}
}

// TestApplyListSharedProjectsOptions_CarriesTheFiltersOnlyWhenGiven verifies
// the two filters whose guards read a pointer and a level reach the SDK
// options when the input names them and are left unset when it does not.
//
// GitLab answers a list the same whether a filter was applied or dropped, so
// the options are the only place the difference shows.
func TestApplyListSharedProjectsOptions_CarriesTheFiltersOnlyWhenGiven(t *testing.T) {
	full := &gl.ListGroupSharedProjectsOptions{}
	applyListSharedProjectsOptions(ListSharedProjectsInput{Archived: new(true), MinAccessLevel: 30}, full)
	if full.Archived == nil || !*full.Archived {
		t.Errorf("archived = %v, want true", full.Archived)
	}
	if full.MinAccessLevel == nil || *full.MinAccessLevel != gl.AccessLevelValue(30) {
		t.Errorf("min_access_level = %v, want 30", full.MinAccessLevel)
	}

	bare := &gl.ListGroupSharedProjectsOptions{}
	applyListSharedProjectsOptions(ListSharedProjectsInput{GroupID: "99"}, bare)
	if bare.Archived != nil || bare.MinAccessLevel != nil {
		t.Errorf("options = %+v, want neither filter", bare)
	}
}

// TestSharingMarkdown_OptionalPartsAppearOnlyWhenPresent verifies the share
// result names the granted role only when there is one, and the shared-project
// table links a project only when GitLab sent its URL.
func TestSharingMarkdown_OptionalPartsAppearOnlyWhenPresent(t *testing.T) {
	withRole := FormatShareGroupMarkdown(ShareGroupOutput{Message: "shared", AccessRole: "Developer", GroupAccess: 30})
	if !strings.Contains(withRole, "- **Access**: Developer\n- **Access Level**: 30\n") {
		t.Errorf("markdown missing the granted role:\n%s", withRole)
	}
	withoutRole := FormatShareGroupMarkdown(ShareGroupOutput{Message: "shared"})
	if strings.Contains(withoutRole, "**Access**") {
		t.Errorf("markdown names a role the share result does not carry:\n%s", withoutRole)
	}

	linked := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{
		Projects: []ProjectItem{{ID: 1, Name: "proj", WebURL: "https://gl/proj"}},
	})
	if !strings.Contains(linked, "[proj](https://gl/proj)") {
		t.Errorf("markdown did not link a project that has a URL:\n%s", linked)
	}
	plain := FormatSharedProjectsListMarkdown(SharedProjectsListOutput{
		Projects: []ProjectItem{{ID: 1, Name: "proj"}},
	})
	if strings.Contains(plain, "[proj](") {
		t.Errorf("markdown linked a project with no URL:\n%s", plain)
	}
	if !strings.Contains(plain, "| proj |") {
		t.Errorf("markdown lost the name of a project with no URL:\n%s", plain)
	}
}

// TestListSharedProjects_AnswersArchivedUnlessTheRowsAreSimple verifies the
// archived flag is published from a full row and left unanswered from a
// simple one, on all three spellings of the input: simple omitted, simple
// false and simple true. GitLab renders BasicProjectDetails for a simple
// listing, which carries no archived key, so a flag copied off such a row
// would answer "not archived" where GitLab said nothing.
func TestListSharedProjects_AnswersArchivedUnlessTheRowsAreSimple(t *testing.T) {
	for _, tc := range []struct {
		name   string
		simple *bool
		want   *bool
	}{
		{name: "omitted", simple: nil, want: new(true)},
		{name: "false", simple: new(false), want: new(true)},
		{name: "true", simple: new(true), want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != pathGroupSharedProj {
					http.NotFound(w, r)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, `[{"id":42,"name":"shared-proj","archived":true}]`)
			}))
			out, err := ListSharedProjects(context.Background(), client, ListSharedProjectsInput{GroupID: "99", Simple: tc.simple})
			if err != nil {
				t.Fatalf("ListSharedProjects() error: %v", err)
			}
			if len(out.Projects) != 1 {
				t.Fatalf("len(out.Projects) = %d, want 1", len(out.Projects))
			}
			got := out.Projects[0].Archived
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("archived = %v, want %v", got, tc.want)
			}
		})
	}
}
