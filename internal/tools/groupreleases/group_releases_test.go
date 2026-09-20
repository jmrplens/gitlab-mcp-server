// group_releases_test.go contains unit tests for GitLab group release operations.
// Tests use httptest to mock the GitLab Group Releases API.
package groupreleases

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const pathGroupReleases = "/api/v4/groups/mygroup/releases"

func inputPagination(page, perPage int) toolutil.PaginationInput {
	return toolutil.PaginationInput{Page: page, PerPage: perPage}
}

// TestList_Success validates that List returns a fully populated release
// when the GitLab API responds with all fields (tag, name, description,
// dates, author, upcoming flag, and self-link).
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"tag_name":"v1.0.0","name":"Release 1.0","description":"First release","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"admin"},"upcoming_release":false,"_links":{"self":"https://git.example.com/group/proj/-/releases/v1.0.0"}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	r := out.Releases[0]
	if r.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want %q", r.TagName, "v1.0.0")
	}
	if r.Name != "Release 1.0" {
		t.Errorf("Name = %q, want %q", r.Name, "Release 1.0")
	}
	if r.Description != "First release" {
		t.Errorf("Description = %q, want %q", r.Description, "First release")
	}
	if r.Author == nil || r.Author.Username != "admin" {
		t.Errorf("Author = %+v, want username %q", r.Author, "admin")
	}
	if r.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if r.ReleasedAt == "" {
		t.Error("ReleasedAt should not be empty")
	}
	if r.Links == nil || r.Links.Self != "https://git.example.com/group/proj/-/releases/v1.0.0" {
		t.Errorf("Links.Self = %+v, want self link URL", r.Links)
	}
}

// TestList_Simple verifies the List_Simple handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_Simple(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.AssertQueryParam(t, r, "simple", "true")
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v1.0.0","name":"Release 1.0"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup", Simple: true})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
}

// TestList_MissingGroupID asserts that an empty group_id is refused by the
// handler itself, naming the field, and that no request reaches GitLab: the
// mock fails the test if one arrives, so the refusal cannot be the mock's.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
	if !strings.Contains(err.Error(), "group_id") {
		t.Errorf("error %q does not name the missing field", err)
	}
}

// TestList_CancelledContext asserts that a canceled context aborts the call
// without contacting GitLab, which is what the forbidding mock holds: the
// context is checked before the request is built.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "mygroup"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}

// TestList_EmptyResults verifies the List_EmptyResults handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResults(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 0 {
		t.Errorf("len(Releases) = %d, want 0", len(out.Releases))
	}
}

// TestList_NotFound verifies that a 404 is the status the group_id hint is
// attached to: the wrapped error names the operation and carries the suggestion
// naming what to check. GitLab's own message is not asserted here because
// client-go answers a 404 with its ErrNotFound sentinel and keeps no body.
func TestList_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "listGroupReleases") {
		t.Errorf("error %q does not name the operation", err)
	}
	if !strings.Contains(err.Error(), "Suggestion: verify group_id with gitlab_group_get") {
		t.Errorf("error %q does not carry the group_id hint", err)
	}
}

// TestList_Forbidden verifies that a status other than 404 is wrapped without
// the group_id suggestion: a token that may not read the group is not a group
// that does not exist, and offering the lookup hint there would send a model
// after the wrong cause.
func TestList_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"insufficient scope for this group"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for 403 response, got nil")
	}
	// The fixture message is not a status line, so finding it proves GitLab's
	// own account of the refusal reaches the caller.
	if !strings.Contains(err.Error(), "insufficient scope for this group") {
		t.Errorf("error %q does not carry GitLab's message", err)
	}
	if strings.Contains(err.Error(), "Suggestion:") {
		t.Errorf("error %q carries the 404-only hint", err)
	}
}

// TestList_Pagination verifies that List forwards pagination parameters to the
// GitLab API and parses the response metadata into the
// [toolutil.PaginationOutput] it publishes.
//
// The middle-page case gives each of the six headers a number no other header
// carries, so a block filled from the wrong header cannot pass the whole-struct
// comparison; that is what a page-2-of-2 fixture could not hold, since Page and
// TotalPages read the same there and swapping them changed nothing. The
// last-page case keeps the other half: with no X-Next-Page, NextPage is zero
// and HasMore is derived false.
func TestList_Pagination(t *testing.T) {
	cases := []struct {
		name    string
		page    int
		perPage int
		headers testutil.PaginationHeaders
		want    toolutil.PaginationOutput
	}{
		{
			name:    "middle page",
			page:    2,
			perPage: 10,
			headers: testutil.PaginationHeaders{
				Page:       "2",
				PerPage:    "10",
				Total:      "45",
				TotalPages: "5",
				NextPage:   "3",
				PrevPage:   "1",
			},
			want: toolutil.PaginationOutput{
				Page: 2, PerPage: 10, TotalItems: 45, TotalPages: 5,
				NextPage: 3, PrevPage: 1, HasMore: true,
			},
		},
		{
			name:    "last page",
			page:    5,
			perPage: 10,
			headers: testutil.PaginationHeaders{
				Page:       "5",
				PerPage:    "10",
				Total:      "45",
				TotalPages: "5",
				PrevPage:   "4",
			},
			want: toolutil.PaginationOutput{
				Page: 5, PerPage: 10, TotalItems: 45, TotalPages: 5,
				PrevPage: 4, HasMore: false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
					testutil.AssertQueryParam(t, r, "page", strconv.Itoa(tc.page))
					testutil.AssertQueryParam(t, r, "per_page", strconv.Itoa(tc.perPage))
					testutil.RespondJSONWithPagination(
						w, http.StatusOK,
						`[{"tag_name":"v0.9.0","name":"Old Release"}]`,
						tc.headers,
					)
					return
				}
				http.NotFound(w, r)
			}))

			out, err := List(context.Background(), client, ListInput{
				GroupID:         "mygroup",
				PaginationInput: inputPagination(tc.page, tc.perPage),
			})
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}
			if len(out.Releases) != 1 {
				t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
			}
			if out.Pagination != tc.want {
				t.Errorf("Pagination = %+v, want %+v", out.Pagination, tc.want)
			}
		})
	}
}

// TestList_MinimalRelease verifies the List_MinimalRelease handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_MinimalRelease(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v0.0.1","name":"Minimal"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	r := out.Releases[0]
	if r.TagName != "v0.0.1" {
		t.Errorf("TagName = %q, want %q", r.TagName, "v0.0.1")
	}
	if r.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty for nil date", r.CreatedAt)
	}
	if r.ReleasedAt != "" {
		t.Errorf("ReleasedAt = %q, want empty for nil date", r.ReleasedAt)
	}
	if r.Author == nil || r.Author.Username != "" {
		t.Errorf("Author = %+v, want object with empty username for no author", r.Author)
	}
	if r.Links != nil {
		t.Errorf("Links = %+v, want nil for no _links", r.Links)
	}
}

// TestList_UpcomingRelease verifies the List_UpcomingRelease handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_UpcomingRelease(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v2.0.0-rc1","name":"RC1","upcoming_release":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	if !out.Releases[0].UpcomingRelease {
		t.Error("UpcomingRelease = false, want true")
	}
}

// TestList_MultipleReleases verifies the List_MultipleReleases handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_MultipleReleases(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"tag_name":"v2.0.0","name":"Major","created_at":"2026-07-01T00:00:00Z","author":{"username":"lead"}},
				{"tag_name":"v1.1.0","name":"Minor","created_at":"2026-06-01T00:00:00Z","author":{"username":"dev"}},
				{"tag_name":"v1.0.0","name":"Initial","created_at":"2026-05-01T00:00:00Z","author":{"username":"admin"}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 3 {
		t.Fatalf("len(Releases) = %d, want 3", len(out.Releases))
	}
	wantTags := []string{"v2.0.0", "v1.1.0", "v1.0.0"}
	for i, wt := range wantTags {
		t.Run(wt, func(t *testing.T) {
			if out.Releases[i].TagName != wt {
				t.Errorf("Releases[%d].TagName = %q, want %q", i, out.Releases[i].TagName, wt)
			}
		})
	}
}

// fullReleaseJSON is a single group release with every nested sub-object
// populated so the shapes.go converters (author, commit with stats/status,
// assets with sources and links, milestones with issue stats and dates,
// evidences, and _links) are all exercised.
const fullReleaseJSON = `[{
	"tag_name":"v3.0.0",
	"name":"Full Release",
	"description":"notes",
	"description_html":"<p>notes</p>",
	"created_at":"2026-01-01T00:00:00Z",
	"released_at":"2026-01-02T00:00:00Z",
	"upcoming_release":false,
	"commit_path":"/group/proj/-/commit/abc",
	"tag_path":"/group/proj/-/tags/v3.0.0",
	"author":{"id":7,"username":"rel","name":"Releaser","state":"active","avatar_url":"https://a","web_url":"https://u","created_at":"2025-12-01T00:00:00Z"},
	"commit":{"id":"abcdef","short_id":"abc","title":"t","author_name":"an","author_email":"ae","authored_date":"2026-01-01T01:00:00Z","committer_name":"cn","committer_email":"ce","committed_date":"2026-01-01T02:00:00Z","created_at":"2026-01-01T03:00:00Z","message":"m","parent_ids":["p1"],"project_id":9,"web_url":"https://c","status":"success","stats":{"additions":1,"deletions":2,"total":3}},
	"milestones":[{"id":1,"iid":2,"project_id":3,"title":"M1","description":"d","state":"active","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","due_date":"2026-02-01","start_date":"2026-01-15","web_url":"https://m","issue_stats":{"total":5,"closed":3}},null],
	"evidences":[{"sha":"sha1","filepath":"/ev","collected_at":"2026-01-01T00:00:00Z"},null],
	"assets":{"count":2,"evidence_file_path":"/evfile","sources":[{"format":"zip","url":"https://z"}],"links":[{"id":11,"name":"bin","url":"https://l","direct_asset_url":"https://d","external":true,"link_type":"package"},null]},
	"_links":{"self":"https://git.example.com/g/p/-/releases/v3.0.0","edit_url":"https://e/edit","closed_issues_url":"https://ci","closed_merge_requests_url":"https://cmr","merged_merge_requests_url":"https://mmr","opened_issues_url":"https://oi","opened_merge_requests_url":"https://omr"}
}]`

// TestList_FullNestedObjects verifies that every nested sub-object of a group
// release is mapped field-for-field by the shapes.go converters.
func TestList_FullNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, fullReleaseJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	r := out.Releases[0]

	if r.DescriptionHTML != "<p>notes</p>" {
		t.Errorf("DescriptionHTML = %q", r.DescriptionHTML)
	}
	// The two timestamps are read through guards of their own, so each is held
	// to the value GitLab sent for it rather than to being non-empty.
	if r.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want the created_at GitLab sent", r.CreatedAt)
	}
	if r.ReleasedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("ReleasedAt = %q, want the released_at GitLab sent", r.ReleasedAt)
	}
	// Each path is held to its own value rather than to being populated: the two
	// are strings of the same shape, so "both non-empty" is satisfied by a
	// converter that reads the commit path into tag_path and the other way.
	if r.CommitPath != "/group/proj/-/commit/abc" {
		t.Errorf("CommitPath = %q, want the commit path", r.CommitPath)
	}
	if r.TagPath != "/group/proj/-/tags/v3.0.0" {
		t.Errorf("TagPath = %q, want the tag path", r.TagPath)
	}
	assertFullAuthor(t, r.Author)
	assertFullCommit(t, r.Commit)
	assertFullMilestones(t, r.Milestones)
	assertFullEvidences(t, r.Evidences)
	assertFullAssets(t, r.Assets)
	assertFullLinks(t, r.Links)
}

func assertFullAuthor(t *testing.T, a *toolutil.AuthorOutput) {
	t.Helper()
	// The fixture server sends an extra author.created_at field that is not part
	// of the documented author subset; the trimmed toolutil.AuthorOutput must ignore it
	// (version tolerance) while mapping the documented fields.
	if a == nil || a.ID != 7 || a.Username != "rel" || a.Name != "Releaser" || a.State != "active" ||
		a.AvatarURL != "https://a" || a.WebURL != "https://u" {
		t.Errorf("Author not fully mapped: %+v", a)
	}
}

func assertFullCommit(t *testing.T, c *CommitOutput) {
	t.Helper()
	if c == nil {
		t.Fatal("Commit should be populated")
	}
	// The fixture server sends extra commit fields (project_id, status, stats)
	// that are not part of the documented group-release commit subset; the
	// trimmed CommitOutput must ignore them (version tolerance) while mapping the
	// documented fields including web_url.
	if c.WebURL != "https://c" {
		t.Errorf("Commit.WebURL = %q, want the commit page URL", c.WebURL)
	}
	// shapes.go says every base field is promoted from the shared commit shape,
	// which nothing here held. The fixture gives each its own value, the three
	// timestamps an hour apart, so a field read from the wrong one is visible.
	want := toolutil.CommitOutput{
		ID: "abcdef", ShortID: "abc", Title: "t",
		AuthorName: "an", AuthorEmail: "ae", AuthoredDate: "2026-01-01T01:00:00Z",
		CommitterName: "cn", CommitterEmail: "ce", CommittedDate: "2026-01-01T02:00:00Z",
		CreatedAt: "2026-01-01T03:00:00Z", Message: "m", ParentIDs: []string{"p1"},
	}
	if !reflect.DeepEqual(c.CommitOutput, want) {
		t.Errorf("Commit base fields = %+v, want %+v", c.CommitOutput, want)
	}
}

func assertFullMilestones(t *testing.T, ms []*toolutil.MilestoneOutput) {
	t.Helper()
	if len(ms) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1 (nil entry skipped)", len(ms))
	}
	m := ms[0]
	if m.ID != 1 || m.IID != 2 || m.ProjectID != 3 || m.Title != "M1" || m.State != "active" ||
		m.DueDate != "2026-02-01" || m.StartDate != "2026-01-15" || m.WebURL != "https://m" {
		t.Errorf("Milestone not fully mapped: %+v", m)
	}
	// The three fields the check above leaves out, each held to the value the
	// fixture sent for it rather than to being populated: title and description
	// are both free text, and the two timestamps are a day apart, so a converter
	// reading one into the other's field is visible here and nowhere else.
	if m.Description != "d" {
		t.Errorf("Milestone.Description = %q, want the description GitLab sent", m.Description)
	}
	if m.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("Milestone.CreatedAt = %q, want the created_at GitLab sent", m.CreatedAt)
	}
	if m.UpdatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("Milestone.UpdatedAt = %q, want the updated_at GitLab sent", m.UpdatedAt)
	}
	if m.IssueStats == nil || m.IssueStats.Total != 5 || m.IssueStats.Closed != 3 {
		t.Errorf("Milestone.IssueStats not mapped: %+v", m.IssueStats)
	}
}

func assertFullEvidences(t *testing.T, evs []*toolutil.EvidenceOutput) {
	t.Helper()
	if len(evs) != 1 {
		t.Fatalf("len(Evidences) = %d, want 1 (nil entry skipped)", len(evs))
	}
	if evs[0].SHA != "sha1" || evs[0].Filepath != "/ev" || evs[0].CollectedAt == "" {
		t.Errorf("Evidence not mapped: %+v", evs[0])
	}
}

func assertFullAssets(t *testing.T, a *toolutil.AssetsOutput) {
	t.Helper()
	if a == nil || a.Count != 2 || a.EvidenceFilePath != "/evfile" {
		t.Fatalf("Assets header not mapped: %+v", a)
	}
	if len(a.Sources) != 1 || a.Sources[0].Format != "zip" || a.Sources[0].URL != "https://z" {
		t.Errorf("Asset sources not mapped: %+v", a.Sources)
	}
	if len(a.Links) != 1 {
		t.Fatalf("len(Assets.Links) = %d, want 1 (nil entry skipped)", len(a.Links))
	}
	if a.Links[0].ID != 11 || a.Links[0].Name != "bin" || !a.Links[0].External ||
		a.Links[0].LinkType != "package" || a.Links[0].DirectAssetURL != "https://d" {
		t.Errorf("Asset link not mapped: %+v", a.Links[0])
	}
}

func assertFullLinks(t *testing.T, l *toolutil.LinksOutput) {
	t.Helper()
	if l == nil || l.Self != "https://git.example.com/g/p/-/releases/v3.0.0" ||
		l.EditURL != "https://e/edit" || l.ClosedIssuesURL != "https://ci" ||
		l.ClosedMergeRequestsURL != "https://cmr" || l.MergedMergeRequestsURL != "https://mmr" ||
		l.OpenedIssuesURL != "https://oi" || l.OpenedMergeRequestsURL != "https://omr" {
		t.Errorf("Links not fully mapped: %+v", l)
	}
}

// TestList_EmptyNestedObjects verifies that empty/zero nested payloads collapse
// to nil pointers and empty slices (commit with no id, no _links, no assets
// links/sources).
func TestList_EmptyNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v0.0.2","name":"Bare","assets":{"count":0}}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	r := out.Releases[0]
	if r.Commit != nil {
		t.Errorf("Commit = %+v, want nil for empty commit", r.Commit)
	}
	if r.Links != nil {
		t.Errorf("Links = %+v, want nil for empty _links", r.Links)
	}
	if r.Milestones != nil || r.Evidences != nil {
		t.Errorf("Milestones/Evidences should be nil: %+v %+v", r.Milestones, r.Evidences)
	}
	if r.Assets != nil {
		if len(r.Assets.Sources) != 0 || len(r.Assets.Links) != 0 {
			t.Errorf("Assets sources/links should be empty: %+v", r.Assets)
		}
	}
}

// TestList_MilestoneWithoutDates verifies that a milestone with nil due/start
// dates renders empty date strings (covers the nil branch of formatISOTimePtr).
func TestList_MilestoneWithoutDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v0.0.3","name":"MS","milestones":[{"id":1,"title":"NoDates","state":"active"}]}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases[0].Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Releases[0].Milestones))
	}
	m := out.Releases[0].Milestones[0]
	if m.DueDate != "" || m.StartDate != "" {
		t.Errorf("DueDate/StartDate = %q/%q, want empty for nil dates", m.DueDate, m.StartDate)
	}
	if m.IssueStats != nil {
		t.Errorf("IssueStats = %+v, want nil when absent", m.IssueStats)
	}
}

// TestList_OrderingAndKeyset verifies that order_by, sort, and keyset
// pagination parameters are forwarded to the GitLab API.
func TestList_OrderingAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.AssertQueryParam(t, r, "order_by", "released_at")
			testutil.AssertQueryParam(t, r, "sort", "desc")
			testutil.AssertQueryParam(t, r, "pagination", "keyset")
			testutil.AssertQueryParam(t, r, "page_token", "tok123")
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v1.0.0","name":"R"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:    "mygroup",
		OrderBy:    "released_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
}
