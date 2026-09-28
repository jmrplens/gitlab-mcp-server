// mr_context_commits_test.go contains unit tests for the merge request context commit MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrcontextcommits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// fmtUnexpMethod identifies the fmt unexp method constant used by this package.
const fmtUnexpMethod = "unexpected method: %s"

// pathMRContextCommits identifies the path MR context commits constant used by this package.
const pathMRContextCommits = "/api/v4/projects/1/merge_requests/10/context_commits"

// testCommitSHA identifies the test commit SHA constant used by this package.
const testCommitSHA = "abc123"

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMRContextCommits {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"abc123","short_id":"abc1","title":"Initial commit","author_name":"Dev","author_email":"dev@test.com"},
			{"id":"def456","short_id":"def4","title":"Second commit","author_name":"Dev2","author_email":"dev2@test.com"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(out.Commits))
	}
	if out.Commits[0].ID != testCommitSHA {
		t.Errorf("expected ID abc123, got %s", out.Commits[0].ID)
	}
	if out.Commits[1].Title != "Second commit" {
		t.Errorf("expected title 'Second commit', got %s", out.Commits[1].Title)
	}
}

// TestList_PageAndPerPage_ReachTheRequest holds that the page a caller asks
// for is the page GitLab is asked for. GitLab pages the context commits
// although the route declares neither page nor per_page, and client-go's
// ListMergeRequestContextCommits takes no options struct, so until the input
// carried the two this action could only ever read the first twenty.
func TestList_PageAndPerPage_ReachTheRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, pathMRContextCommits)
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "1")
		testutil.RespondJSON(w, http.StatusOK, `[{"id":"def456","short_id":"def4","title":"Second commit"}]`)
	}))

	out, err := List(t.Context(), client, ListInput{
		ProjectID: "1", MergeRequest: 10,
		Page: 2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 || out.Commits[0].ID != "def456" {
		t.Errorf("commits = %+v, want the one of page 2", out.Commits)
	}
}

// TestList_NoPageAsked_SendsNeither holds that a caller who asks for no page
// leaves the choice to GitLab rather than sending a zero.
func TestList_NoPageAsked_SendsNeither(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("page") || r.URL.Query().Has("per_page") {
			t.Errorf("query = %q, want neither page nor per_page", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	if _, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// firstOfTwoPages is the pagination block of the first page of a two-commit
// list read one commit at a time.
var firstOfTwoPages = toolutil.PaginationOutput{Page: 1, PerPage: 1, TotalItems: 2, TotalPages: 2, NextPage: 2, HasMore: true}

// firstOfTwoPagesHeaders are the headers GitLab answers that page with.
var firstOfTwoPagesHeaders = testutil.PaginationHeaders{Page: "1", PerPage: "1", Total: "2", TotalPages: "2", NextPage: "2"}

// TestList_NextPageHeader_PublishesThePaginationBlock holds that the page
// GitLab answers is published as a page, whether or not client-go could read
// its commits. The commits are read from the captured body and the capture
// keeps no headers, so the block has to come from the response client-go
// returns; a commit carrying a trailer makes client-go's own decode fail, and
// the block must survive that too, since that is the failure List passes over.
func TestList_NextPageHeader_PublishesThePaginationBlock(t *testing.T) {
	cases := []struct {
		name   string
		commit string
		wantID string
	}{
		{"client-go reads the page", `{"id":"abc123","short_id":"abc1","title":"Initial commit"}`, testCommitSHA},
		{"client-go misreads a trailer", fullCommitJSON, fullCommitItem.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, pathMRContextCommits)
				testutil.RespondJSONWithPagination(w, http.StatusOK, "["+tc.commit+"]", firstOfTwoPagesHeaders)
			}))

			out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if len(out.Commits) != 1 || out.Commits[0].ID != tc.wantID {
				t.Errorf("commits = %+v, want the one commit %s", out.Commits, tc.wantID)
			}
			if out.Pagination != firstOfTwoPages {
				t.Errorf("pagination = %+v, want %+v", out.Pagination, firstOfTwoPages)
			}
		})
	}
}

// TestList_Empty verifies List when empty.
func TestList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(out.Commits))
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMRContextCommits {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"abc123","short_id":"abc1","title":"Initial commit","author_name":"Dev","author_email":"dev@test.com"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(out.Commits))
	}
	if out.Commits[0].ID != testCommitSHA {
		t.Errorf("expected ID abc123, got %s", out.Commits[0].ID)
	}
}

// TestCreate_Error verifies Create when error.
func TestCreate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathMRContextCommits {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{
		ProjectID:    "1",
		MergeRequest: 10,
		Commits:      []string{testCommitSHA},
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Commits: []CommitItem{
			{ID: "abc123", ShortID: "abc1", Title: "Fix bug", AuthorName: "Dev"},
			{ID: "def456", ShortID: "def4", Title: "Add feature", AuthorName: "Dev2"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// TestFormatListMarkdown_APageOfALongerList verifies that a page which is not
// the whole list says so: the total in the heading, the page between the
// heading and the table, and the pagination line before the next steps. The
// heading used to be handed an empty block, so a page read as the whole list.
func TestFormatListMarkdown_APageOfALongerList(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{
		Commits:    []CommitItem{{ID: testCommitSHA, ShortID: "abc1", Title: "Fix bug", AuthorName: "Dev"}},
		Pagination: firstOfTwoPages,
	})
	want := "## MR Context Commits (2)\n\n" +
		"Showing 1 of 2 results (page 1 of 2)\n\n" +
		"| SHA | Title | Author | Created |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `abc1` | Fix bug | Dev |  |\n" +
		"\nPage 1 of 2 | 2 items total | 1 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'repository.commit_get' to read one of these commits in full\n" +
		"- Use action 'merge_request.context_commits_create' to pin another commit to this review\n" +
		"- Use action 'merge_request.context_commits_delete' to unpin one of these commits\n"
	if got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// MRIID required-field validation
// ---------------------------------------------------------------------------.

// assertContains checks contains invariants for tests.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

// TestMRIIDRequired_Validation covers MRIIDRequired with table-driven subtests for validation.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error {
			_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
			return err
		}},
		{"Create", func() error {
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Commits: []string{"abc"}})
			return err
		}},
		{"Delete", func() error {
			return Delete(context.Background(), client, DeleteInput{ProjectID: "42", Commits: []string{"abc"}})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "merge_request_iid")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — CreatedAt branch + canceled context
// ---------------------------------------------------------------------------.

// TestList_WithCreatedAt verifies that a context commit's creation instant goes
// out in RFC 3339, which is what the table's time helper reads back: Go's
// default layout, which it used to carry, printed raw.
func TestList_WithCreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"aaa111","short_id":"aaa1","title":"Commit with date","author_name":"Dev","author_email":"dev@test.com","created_at":"2026-06-15T10:30:00+02:00"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(out.Commits))
	}
	if got := out.Commits[0].CreatedAt; got != "2026-06-15T08:30:00Z" {
		t.Errorf("CreatedAt = %q, want the instant in RFC 3339 UTC", got)
	}
}

// fullCommitJSON is a context commit carrying every key
// lib/api/entities/commit.rb exposes, each with a value no other key shares,
// in the shape GitLab sends them: trailers maps each trailer to its last
// value and extended_trailers to the list of all of them, which is what
// Gitlab::Git::Commit#parse_commit_trailers builds. A trailer given twice,
// as Reviewed-by is here, is the case the two keys exist to tell apart.
const fullCommitJSON = `{"id":"abc123def","short_id":"abc123d","title":"Fix the parser",` +
	`"message":"Fix the parser\n\nIt read one byte too many.","author_name":"Ann","author_email":"ann@example.com",` +
	`"authored_date":"2026-05-01T09:00:00Z","committer_name":"Cid","committer_email":"cid@example.com",` +
	`"committed_date":"2026-05-02T10:00:00Z","created_at":"2026-05-03T11:00:00Z","parent_ids":["p1","p2"],` +
	`"trailers":{"Reviewed-by":"Fay","Changelog":"fixed"},` +
	`"extended_trailers":{"Reviewed-by":["Eve","Fay"],"Changelog":["fixed"]},` +
	`"web_url":"https://gitlab.example.com/g/p/-/commit/abc123def"}`

// fullCommitItem is fullCommitJSON as the handlers publish it.
var fullCommitItem = CommitItem{
	ID: "abc123def", ShortID: "abc123d", Title: "Fix the parser",
	Message:    "Fix the parser\n\nIt read one byte too many.",
	AuthorName: "Ann", AuthorEmail: "ann@example.com", AuthoredDate: "2026-05-01T09:00:00Z",
	CommitterName: "Cid", CommitterEmail: "cid@example.com", CommittedDate: "2026-05-02T10:00:00Z",
	CreatedAt:        "2026-05-03T11:00:00Z",
	ParentIDs:        []string{"p1", "p2"},
	Trailers:         map[string]string{"Reviewed-by": "Fay", "Changelog": "fixed"},
	ExtendedTrailers: map[string][]string{"Reviewed-by": {"Eve", "Fay"}, "Changelog": {"fixed"}},
	WebURL:           "https://gitlab.example.com/g/p/-/commit/abc123def",
}

// TestHandlers_PublishEveryCommitKey verifies that both routes answering with
// commits publish every key of the commit entity, each from its own field: a
// value copied into the wrong key would make the item differ. The commit
// carries trailers, which client-go's Commit cannot decode (its
// extended_trailers is a map of strings), so the answer is published only
// because it is read from the captured response in the shape GitLab sends.
func TestHandlers_PublishEveryCommitKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "["+fullCommitJSON+"]")
	}))
	cases := []struct {
		name string
		call func() ([]CommitItem, error)
	}{
		{"list", func() ([]CommitItem, error) {
			out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
			return out.Commits, err
		}},
		{"create", func() ([]CommitItem, error) {
			out, err := Create(t.Context(), client, CreateInput{ProjectID: "1", MergeRequest: 10, Commits: []string{"abc123def"}})
			return out.Commits, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			commits, err := tc.call()
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if len(commits) != 1 || !reflect.DeepEqual(commits[0], fullCommitItem) {
				t.Errorf("commits = %+v, want [%+v]", commits, fullCommitItem)
			}
		})
	}
}

// TestCreateOutput_PublishesNoPagination holds the answer to
// create_context_commits to what the route sends: the commits it pinned and
// no page, since the route is not paged and sends no pagination headers. It
// used to share the list's output, which published a pagination block that
// was zero on every answer and told a model paging applied to a write.
func TestCreateOutput_PublishesNoPagination(t *testing.T) {
	encoded, err := json.Marshal(CreateOutput{Commits: []CommitItem{{ID: testCommitSHA}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var answer map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &answer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := answer["pagination"]; ok {
		t.Errorf("answer %s publishes a pagination block", encoded)
	}
	if _, ok := answer["commits"]; !ok {
		t.Errorf("answer %s carries no commits", encoded)
	}
}

// TestFormatCreateMarkdown_RendersThePinnedCommitsWithNoPage verifies that
// the answer to create_context_commits renders as the list's table, headed by
// the number of commits pinned and carrying no page line, since the answer is
// every commit pinned rather than a page of a longer list.
func TestFormatCreateMarkdown_RendersThePinnedCommitsWithNoPage(t *testing.T) {
	result := FormatCreateMarkdown(CreateOutput{
		Commits: []CommitItem{{ID: testCommitSHA, ShortID: "abc1", Title: "Fix bug", AuthorName: "Dev"}},
	})
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T, want text", result.Content[0])
	}
	want := "## MR Context Commits (1)\n\n" +
		"| SHA | Title | Author | Created |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `abc1` | Fix bug | Dev |  |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'repository.commit_get' to read one of these commits in full\n" +
		"- Use action 'merge_request.context_commits_create' to pin another commit to this review\n" +
		"- Use action 'merge_request.context_commits_delete' to unpin one of these commits\n"
	if text.Text != want {
		t.Errorf("rendered =\n%q\nwant\n%q", text.Text, want)
	}
}

// TestFormatCreateMarkdown_NothingPinned_SaysSo verifies that an answer with
// no commit renders the empty message rather than an empty table.
func TestFormatCreateMarkdown_NothingPinned_SaysSo(t *testing.T) {
	if got, want := FormatCreateMarkdownString(CreateOutput{}), toolutil.EmptyMessage("context commits"); got != want {
		t.Errorf("rendered = %q, want %q", got, want)
	}
}

// commitWithLinkKeys are the keys lib/api/entities/commit_with_link.rb adds
// to a commit under `type: :full`, in the shape the context commit list sends
// them: the author as a UserPath whose status association was not loaded
// (so show_status is false and the status keys are absent), the rendered
// title and description, and the six keys this server does not publish,
// signature_html and the three the route never fills arriving null.
const commitWithLinkKeys = `"author":{"id":7,"username":"ann","public_email":"ann@example.com","name":"Ann Example",` +
	`"state":"active","locked":true,"avatar_url":"https://gitlab.example.com/uploads/-/system/user/avatar/7/a.png",` +
	`"web_url":"https://gitlab.example.com/ann","show_status":false,"path":"/ann"},` +
	`"author_gravatar_url":"https://www.gravatar.com/avatar/0bc83cb5?s=80&d=identicon",` +
	`"commit_url":"https://gitlab.example.com/g/p/-/commit/abc123def","commit_path":"/g/p/-/commit/abc123def",` +
	`"description_html":"<p dir=\"auto\">It read one byte too many.</p>","title_html":"Fix the parser",` +
	`"signature_html":null,"prev_commit_id":null,"next_commit_id":null,"pipeline_status_path":null`

// commitWithLinkJSON is fullCommitJSON as the context commit list sends it,
// which presents CommitWithLink rather than the Commit its route annotates.
var commitWithLinkJSON = strings.TrimSuffix(fullCommitJSON, "}") + "," + commitWithLinkKeys + "}"

// listedCommitItem is commitWithLinkJSON as List publishes it: every key of
// fullCommitItem, and the four CommitWithLink keys this server surfaces.
var listedCommitItem = func() CommitItem {
	item := fullCommitItem
	item.Author = &CommitAuthor{
		ID: 7, Username: "ann", PublicEmail: "ann@example.com", Name: "Ann Example",
		State: "active", Locked: true,
		AvatarURL: "https://gitlab.example.com/uploads/-/system/user/avatar/7/a.png",
		WebURL:    "https://gitlab.example.com/ann",
		Path:      "/ann",
	}
	item.AuthorGravatarURL = "https://www.gravatar.com/avatar/0bc83cb5?s=80&d=identicon"
	item.DescriptionHTML = `<p dir="auto">It read one byte too many.</p>`
	item.TitleHTML = "Fix the parser"
	return item
}()

// TestList_CommitWithLink_PublishesTheAuthorAndTheRenderedText verifies that
// the list publishes what CommitWithLink adds to a commit and this server
// surfaces, each from its own key: the GitLab account the author email
// belongs to, the Gravatar image of that email, and the title and
// description rendered to HTML. The route's annotation names Commit, so
// nothing but the handler's own reading of the capture can surface them, and
// a value copied into the wrong key would make the item differ.
func TestList_CommitWithLink_PublishesTheAuthorAndTheRenderedText(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "["+commitWithLinkJSON+"]")
	}))

	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 || !reflect.DeepEqual(out.Commits[0], listedCommitItem) {
		t.Errorf("commits = %+v, want [%+v]", out.Commits, listedCommitItem)
	}
}

// TestList_CommitWithLink_PublishesExactlyTheDecidedKeys holds the published
// row to the keys decided for it, as a caller reads them on the wire.
// commit_url and commit_path say web_url again, and signature_html,
// prev_commit_id, next_commit_id and pipeline_status_path are null on every
// commit this route sends, so none of the six is published; the author
// carries the unconditional keys of a UserPath, show_status included although
// it is false, and none of the conditional ones, which the route never fills.
func TestList_CommitWithLink_PublishesExactlyTheDecidedKeys(t *testing.T) {
	encoded, err := json.Marshal(listedCommitItem)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var row map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &row); err != nil {
		t.Fatalf("unmarshal row: %v", err)
	}
	for _, key := range []string{"author", "author_gravatar_url", "description_html", "title_html"} {
		t.Run("publishes "+key, func(t *testing.T) {
			if _, ok := row[key]; !ok {
				t.Errorf("row %s carries no %s", encoded, key)
			}
		})
	}
	for _, key := range []string{"commit_url", "commit_path", "signature_html", "prev_commit_id", "next_commit_id", "pipeline_status_path"} {
		t.Run("leaves out "+key, func(t *testing.T) {
			if _, ok := row[key]; ok {
				t.Errorf("row %s carries %s", encoded, key)
			}
		})
	}

	var author map[string]json.RawMessage
	if err = json.Unmarshal(row["author"], &author); err != nil {
		t.Fatalf("unmarshal author: %v", err)
	}
	keys := make([]string, 0, len(author))
	for key := range author {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	want := []string{"avatar_url", "id", "locked", "name", "path", "public_email", "show_status", "state", "username", "web_url"}
	if !slices.Equal(keys, want) {
		t.Errorf("author keys = %v, want %v", keys, want)
	}
}

// TestList_AuthorNoAccountHolds_IsAbsent verifies that a commit whose author
// email no confirmed account holds, which GitLab answers with a null author,
// is published without one rather than with an empty account.
func TestList_AuthorNoAccountHolds_IsAbsent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":"abc123","short_id":"abc1","title":"Unknown author",`+
			`"author_name":"Ghost","author_email":"ghost@example.com","author":null,`+
			`"author_gravatar_url":"https://www.gravatar.com/avatar/1?s=80&d=identicon","title_html":"Unknown author"}]`)
	}))

	out, err := List(t.Context(), client, ListInput{ProjectID: "1", MergeRequest: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("commits = %+v, want one", out.Commits)
	}
	if out.Commits[0].Author != nil {
		t.Errorf("author = %+v, want none for an email no account holds", out.Commits[0].Author)
	}
	encoded, err := json.Marshal(out.Commits[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), `"author":`) {
		t.Errorf("row %s publishes an author GitLab did not send", encoded)
	}
}

// TestHandlers_AnswerNoTypeReads_IsAnError verifies that an answer neither
// client-go's Commit nor this package's own row can decode is reported under
// the operation's name rather than published empty: passing over client-go's
// failure to read a trailer must not pass over a body nothing can read. And a
// body that is not JSON at all is client-go's error to report, with the
// route's hint, since no type of this package's could read it either.
func TestHandlers_AnswerNoTypeReads_IsAnError(t *testing.T) {
	cases := []struct {
		name string
		body string
		call func(*gitlabclient.Client) ([]CommitItem, error)
		want string
	}{
		{"list, id a number", `[{"id":5}]`, func(c *gitlabclient.Client) ([]CommitItem, error) {
			out, err := List(t.Context(), c, ListInput{ProjectID: "1", MergeRequest: 10})
			return out.Commits, err
		}, "list_mr_context_commits: "},
		{"create, id a number", `[{"id":5}]`, func(c *gitlabclient.Client) ([]CommitItem, error) {
			out, err := Create(t.Context(), c, CreateInput{ProjectID: "1", MergeRequest: 10, Commits: []string{"abc"}})
			return out.Commits, err
		}, "create_mr_context_commits: "},
		{"list, not JSON", `[{`, func(c *gitlabclient.Client) ([]CommitItem, error) {
			out, err := List(t.Context(), c, ListInput{ProjectID: "1", MergeRequest: 10})
			return out.Commits, err
		}, "list_mr_context_commits: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))
			commits, err := tc.call(client)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one naming %q", err, tc.want)
			}
			if commits != nil {
				t.Errorf("commits = %+v, want none beside the error", commits)
			}
		})
	}
}

// TestMisreadByClientGo pins which errors the handlers pass over to read the
// capture instead: a type mismatch in client-go's decoder, however wrapped,
// and nothing else.
func TestMisreadByClientGo(t *testing.T) {
	var typeErr error = &json.UnmarshalTypeError{Value: "array", Field: "extended_trailers"}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"type mismatch", typeErr, true},
		{"wrapped type mismatch", fmt.Errorf("decode: %w", typeErr), true},
		{"syntax error", &json.SyntaxError{}, false},
		{"other error", errors.New("403 Forbidden"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := misreadByClientGo(tc.err); got != tc.want {
				t.Errorf("misreadByClientGo(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "1", MergeRequest: 5})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Create — CreatedAt branch + canceled context
// ---------------------------------------------------------------------------.

// TestCreate_WithCreatedAt verifies Create when with created at.
func TestCreate_WithCreatedAt(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":"bbb222","short_id":"bbb2","title":"Created with date","author_name":"Dev","author_email":"dev@test.com","created_at":"2026-07-01T08:00:00Z"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 5,
		Commits:      []string{"bbb222"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(out.Commits))
	}
	if out.Commits[0].CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Create(ctx, client, CreateInput{
		ProjectID:    "1",
		MergeRequest: 5,
		Commits:      []string{"abc123"},
	})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Delete — canceled context
// ---------------------------------------------------------------------------.

// TestDelete_CancelledContext verifies Delete when cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Delete(ctx, client, DeleteInput{
		ProjectID:    "1",
		MergeRequest: 5,
		Commits:      []string{"abc123"},
	})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — content validation
// ---------------------------------------------------------------------------.

// contextCommitHints is the guidance section a context commit listing closes
// with.
//
// The IDs come from the package's own constants rather than being written out
// again: what these tests assert is the shape of the rendered guidance, and
// whether each ID names an action that exists is a question only the catalog
// can answer, which action_specs_test.go asks. Spelling the literal here would
// have held the formatter to the old "commit.get", an ID the catalog does not
// have, and called that a pass.
const contextCommitHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action '" + actionCommitGet + "' to read one of these commits in full\n" +
	"- Use action '" + actionContextCommitsCreate + "' to pin another commit to this review\n" +
	"- Use action '" + actionContextCommitsDelete + "' to unpin one of these commits\n"

// TestFormatListMarkdown_ContentValidation verifies the whole rendering of a
// context commit listing: the SHA as a code span, the pipe in a commit title
// escaped so it cannot split the row, and the guidance naming canonical action
// IDs rather than a tool name.
func TestFormatListMarkdown_ContentValidation(t *testing.T) {
	out := ListOutput{
		Commits: []CommitItem{
			{ID: "abc123", ShortID: "abc1", Title: "First | commit", AuthorName: "Dev", CreatedAt: "2026-01-15T10:00:00Z"},
			{ID: "def456", ShortID: "def4", Title: "Second commit", AuthorName: "Dev2"},
		},
	}
	want := "## MR Context Commits (2)\n\n" +
		"| SHA | Title | Author | Created |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `abc1` | First &#124; commit | Dev | 15 Jan 2026 10:00 UTC |\n" +
		"| `def4` | Second commit | Dev2 |  |\n" + contextCommitHints
	if got := FormatListMarkdownString(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatListMarkdown_FallsBackToTheFullSHA verifies that a commit GitLab
// sent no abbreviated id for is still identified, by its full one.
func TestFormatListMarkdown_FallsBackToTheFullSHA(t *testing.T) {
	out := ListOutput{Commits: []CommitItem{{ID: "abc123def456", Title: "Only a long id", AuthorName: "Dev"}}}
	want := "## MR Context Commits (1)\n\n" +
		"| SHA | Title | Author | Created |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `abc123def456` | Only a long id | Dev |  |\n" + contextCommitHints
	if got := FormatListMarkdownString(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatListMarkdown_LinksTheSHAToTheCommit verifies that a commit GitLab
// sent a web URL for is linked by its short SHA, and that the guidance then
// opens with the instruction to keep the links, which a table without one
// leaves out.
func TestFormatListMarkdown_LinksTheSHAToTheCommit(t *testing.T) {
	out := ListOutput{Commits: []CommitItem{
		{ID: "abc123", ShortID: "abc1", Title: "Linked", AuthorName: "Dev", WebURL: "https://gitlab.example.com/g/p/-/commit/abc123"},
		{ID: "def456", ShortID: "def4", Title: "Unlinked", AuthorName: "Dev2"},
	}}
	want := "## MR Context Commits (2)\n\n" +
		"| SHA | Title | Author | Created |\n" +
		"| --- | --- | --- | --- |\n" +
		"| [abc1](https://gitlab.example.com/g/p/-/commit/abc123) | Linked | Dev |  |\n" +
		"| `def4` | Unlinked | Dev2 |  |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action '" + actionCommitGet + "' to read one of these commits in full\n" +
		"- Use action '" + actionContextCommitsCreate + "' to pin another commit to this review\n" +
		"- Use action '" + actionContextCommitsDelete + "' to unpin one of these commits\n"
	if got := FormatListMarkdownString(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatListMarkdown_LinksTheAuthorToTheAccount verifies the author cell
// of each case the listing can hold: an author GitLab matched to an account is
// linked to that account's profile under the name the commit was authored
// under, or under the account's name when the commit carries none, which sets
// the instruction to keep the links although no SHA is linked; an author with
// no account, or with an account GitLab sent no profile URL for, is the
// commit's author name as text.
func TestFormatListMarkdown_LinksTheAuthorToTheAccount(t *testing.T) {
	account := func(webURL string) *CommitAuthor {
		return &CommitAuthor{ID: 7, Username: "ann", Name: "Ann Example", WebURL: webURL}
	}
	cases := []struct {
		name   string
		commit CommitItem
		cell   string
		linked bool
	}{
		{
			"account with a profile",
			CommitItem{ID: "a1", Title: "T", AuthorName: "Ann", Author: account("https://gitlab.example.com/ann")},
			"[Ann](https://gitlab.example.com/ann)", true,
		},
		{
			"account, blank author name",
			CommitItem{ID: "a1", Title: "T", Author: account("https://gitlab.example.com/ann")},
			"[Ann Example](https://gitlab.example.com/ann)", true,
		},
		{
			"account without a profile URL",
			CommitItem{ID: "a1", Title: "T", AuthorName: "Ann", Author: account("")},
			"Ann", false,
		},
		{
			"no account",
			CommitItem{ID: "a1", Title: "T", AuthorName: "Ghost"},
			"Ghost", false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hints := contextCommitHints
			if tc.linked {
				hints = "\n---\n💡 **Next steps:**\n- " + toolutil.HintPreserveLinks + "\n" +
					strings.TrimPrefix(contextCommitHints, "\n---\n💡 **Next steps:**\n")
			}
			want := "## MR Context Commits (1)\n\n" +
				"| SHA | Title | Author | Created |\n" +
				"| --- | --- | --- | --- |\n" +
				"| `a1` | T | " + tc.cell + " |  |\n" + hints
			if got := FormatListMarkdownString(ListOutput{Commits: []CommitItem{tc.commit}}); got != want {
				t.Errorf("rendered =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route execution for all context commit tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for MR context commit actions.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := newMRContextCommitsSpecsByTool(t)

	if len(byTool) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(byTool))
	}
	if !byTool["gitlab_list_mr_context_commits"].ReadOnly || !byTool["gitlab_list_mr_context_commits"].Idempotent {
		t.Error("list action should be read-only and idempotent")
	}
	if !byTool["gitlab_delete_mr_context_commits"].Destructive || !byTool["gitlab_delete_mr_context_commits"].Idempotent {
		t.Error("delete action should be destructive and idempotent")
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "mrcontextcommits" {
			t.Errorf("OwnerPackage for %s = %q, want mrcontextcommits", spec.Name, spec.OwnerPackage)
		}
	}
}

// TestActionSpecs_DiscoveryMetadata_IsPerAction verifies that each context
// commit action publishes its own Usage, aliases, related actions and
// "Returns: … See also: …" description, and that a tool name
// contextCommitOptions does not know falls through carrying none of them
// rather than a sibling's. An action added without its own case would
// register and still be unreachable from a model's own words.
func TestActionSpecs_DiscoveryMetadata_IsPerAction(t *testing.T) {
	byTool := newMRContextCommitsSpecsByTool(t)

	usages := make(map[string]string, len(byTool))
	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			if spec.Usage == "" {
				t.Error("empty Usage: nothing tells a model when to reach for this action")
			}
			if len(spec.Aliases) < 2 {
				t.Errorf("Aliases = %v, want the tool name plus natural-language phrasings", spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("empty RelatedActions: the result can hint at no next step")
			}
			if !strings.Contains(spec.IndividualTool.Description, "Returns:") ||
				!strings.Contains(spec.IndividualTool.Description, "See also:") {
				t.Errorf("description not in 'Returns: … See also: …' form: %q", spec.IndividualTool.Description)
			}
			usages[spec.Usage] = tool
		})
	}
	if len(usages) != len(byTool) {
		t.Errorf("%d actions publish only %d distinct Usage strings", len(byTool), len(usages))
	}

	unknown := contextCommitOptions("gitlab_unknown_mr_context_commits")
	if unknown.Usage != "" || unknown.Aliases != nil || unknown.RelatedActions != nil ||
		unknown.IndividualTool.Description != "" {
		t.Errorf("an unknown tool inherited metadata: %+v", unknown)
	}
	if unknown.OwnerPackage != "mrcontextcommits" {
		t.Errorf("unknown tool OwnerPackage = %q, want mrcontextcommits", unknown.OwnerPackage)
	}
}

// TestActionSpecs_CallAllRoutes validates all MR context commit routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newMRContextCommitsSpecsByTool(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_mr_context_commits", map[string]any{"project_id": "1", "merge_request_iid": int64(10)}},
		{"gitlab_create_mr_context_commits", map[string]any{"project_id": "1", "merge_request_iid": int64(10), "commits": []any{"abc123"}}},
		{"gitlab_delete_mr_context_commits", map[string]any{"project_id": "1", "merge_request_iid": int64(10), "commits": []any{"abc123"}}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newMRContextCommitsSpecsByTool constructs MR context commits specs by tool test fixtures.
func newMRContextCommitsSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	const commitsJSON = `[
		{"id":"abc123","short_id":"abc1","title":"Initial commit","author_name":"Dev","author_email":"dev@test.com","created_at":"2026-06-15T10:30:00Z"}
	]`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/context_commits"):
			testutil.RespondJSON(w, http.StatusOK, commitsJSON)

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/context_commits"):
			testutil.RespondJSON(w, http.StatusOK, commitsJSON)

		case r.Method == http.MethodDelete && strings.HasSuffix(path, "/context_commits"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))

	specs := ActionSpecs(client)
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestList_EmptyProjectID verifies that List returns an error when project_id
// is empty, covering the missed validation branch.
func TestList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(t.Context(), client, ListInput{ProjectID: "", MergeRequest: 1})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestCreate_EmptyProjectID verifies that Create returns an error when
// project_id is empty.
func TestCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "", MergeRequest: 1, Commits: []string{"abc"}})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestDelete_EmptyProjectID verifies that Delete returns an error when
// project_id is empty.
func TestDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "", MergeRequest: 1, Commits: []string{"abc"}})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestActionSpecs_DeleteError validates the delete route error path against a 403 backend.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	byTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		byTool[spec.IndividualTool.Name] = spec
	}

	_, err := byTool["gitlab_delete_mr_context_commits"].Route.Handler(t.Context(), map[string]any{"project_id": "p", "merge_request_iid": int64(1), "commits": []any{"abc"}})
	if err == nil {
		t.Error("expected delete route error")
	}
}
