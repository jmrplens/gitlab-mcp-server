// award_emoji_test.go contains unit tests for the award emoji MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package awardemoji

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testProjectID identifies the test project ID constant used by this package.
const testProjectID = "my-project"

const (
	// testEmojiThumbsup identifies the test emoji thumbsup constant used by this package.
	testEmojiThumbsup = "thumbsup"
	// testEmojiStar identifies the test emoji star constant used by this package.
	testEmojiStar = "star"
	// fmtExpected1Emoji identifies the fmt expected 1 emoji constant used by this package.
	fmtExpected1Emoji = "expected 1 emoji, got %d"
	// testFieldIssueIID identifies the test field issue IID constant used by this package.
	testFieldIssueIID = "issue_iid"
	// testFieldMRIID identifies the test field mriid constant used by this package.
	testFieldMRIID = "merge_request_iid"
	// testFieldSnippetID identifies the test field snippet ID constant used by this package.
	testFieldSnippetID = "snippet_id"
	// testFieldAwardID identifies the test field award ID constant used by this package.
	testFieldAwardID = "award_id"
	// testFieldNoteID identifies the test field note ID constant used by this package.
	testFieldNoteID = "note_id"
	// testPathAPIProjects identifies the test path API projects constant used by this package.
	testPathAPIProjects = "/api/v4/projects/"
	// fmtNameWantThumbsup identifies the fmt name want thumbsup constant used by this package.
	fmtNameWantThumbsup = "name = %q, want thumbsup"
	// testErrEmptyProjectID identifies the test err empty project ID constant used by this package.
	testErrEmptyProjectID = "expected error for empty project_id"
)

// Issue award emoji tests.

// TestListIssueAwardEmoji_Success verifies that ListIssueAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":10,"name":"thumbsup","user":{"id":1,"username":"admin"},"created_at":"2026-01-01T00:00:00Z","awardable_id":1,"awardable_type":"Issue"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{
		ProjectID: testProjectID,
		IID:       1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Fatalf(fmtExpected1Emoji, len(out.AwardEmoji))
	}
	if out.AwardEmoji[0].Name != testEmojiThumbsup {
		t.Errorf(fmtNameWantThumbsup, out.AwardEmoji[0].Name)
	}
	if out.AwardEmoji[0].User == nil || out.AwardEmoji[0].User.ID != 1 {
		t.Errorf("user.id = %v, want 1", out.AwardEmoji[0].User)
	}
}

// TestListIssueAwardEmoji_ValidationError verifies that ListIssueAwardEmoji_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListIssueAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{
		ProjectID: "",
		IID:       1,
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestGetIssueAwardEmoji_Success verifies that GetIssueAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetIssueAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/award_emoji/10" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"thumbsup","user":{"id":1,"username":"admin"},"created_at":"2026-01-01T00:00:00Z","awardable_id":1,"awardable_type":"Issue"}`)
	}))

	out, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{
		ProjectID: testProjectID,
		IID:       1,
		AwardID:   10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testEmojiThumbsup {
		t.Errorf(fmtNameWantThumbsup, out.Name)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
}

// TestCreateIssueAwardEmoji_Success verifies that CreateIssueAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateIssueAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body["name"] != testEmojiThumbsup {
				t.Errorf(fmtNameWantThumbsup, body["name"])
			}
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"thumbsup","user":{"id":1,"username":"admin"},"created_at":"2026-01-01T00:00:00Z","awardable_id":1,"awardable_type":"Issue"}`)
	}))

	out, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{
		ProjectID: testProjectID,
		IID:       1,
		Name:      testEmojiThumbsup,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testEmojiThumbsup {
		t.Errorf(fmtNameWantThumbsup, out.Name)
	}
}

// TestCreateIssueAwardEmoji_ValidationError verifies that CreateIssueAwardEmoji_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateIssueAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{
		ProjectID: "",
		IID:       1,
		Name:      testEmojiThumbsup,
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestDeleteIssueAwardEmoji_Success verifies that DeleteIssueAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/award_emoji/10" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{
		ProjectID: testProjectID,
		IID:       1,
		AwardID:   10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteIssueAwardEmoji_APIError verifies that DeleteIssueAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{
		ProjectID: testProjectID,
		IID:       1,
		AwardID:   10,
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// Issue note award emoji tests.

// TestListIssueNoteAwardEmoji_Success verifies that ListIssueNoteAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueNoteAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/notes/5/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":20,"name":"heart","user":{"id":2,"username":"dev"},"created_at":"2026-02-01T00:00:00Z","awardable_id":5,"awardable_type":"Note"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{
		ProjectID: testProjectID,
		IID:       1,
		NoteID:    5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Fatalf(fmtExpected1Emoji, len(out.AwardEmoji))
	}
	if out.AwardEmoji[0].Name != "heart" {
		t.Errorf("name = %q, want heart", out.AwardEmoji[0].Name)
	}
}

// TestDeleteIssueNoteAwardEmoji_Success verifies that DeleteIssueNoteAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueNoteAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/notes/5/award_emoji/20" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{
		ProjectID: testProjectID,
		IID:       1,
		NoteID:    5,
		AwardID:   20,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// MR award emoji tests.

// TestListMRAwardEmoji_Success verifies that ListMRAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListMRAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/merge_requests/3/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":30,"name":"rocket","user":{"id":3,"username":"user3"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListMRAwardEmoji(t.Context(), client, MRListInput{
		ProjectID: testProjectID,
		IID:       3,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Fatalf(fmtExpected1Emoji, len(out.AwardEmoji))
	}
	if out.AwardEmoji[0].Name != "rocket" {
		t.Errorf("name = %q, want rocket", out.AwardEmoji[0].Name)
	}
}

// TestCreateMRAwardEmoji_ValidationError verifies that CreateMRAwardEmoji_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateMRAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{
		ProjectID: "",
		IID:       3,
		Name:      "rocket",
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestCreateMRAwardEmoji_DuplicateReturnsExisting verifies CreateMRAwardEmoji returns the current user's existing award on GitLab's duplicate-name 404.
func TestCreateMRAwardEmoji_DuplicateReturnsExisting(t *testing.T) {
	requests := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/v4/user":
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			testutil.RespondJSON(w, http.StatusOK, `{"id":9,"username":"current"}`)
			return
		case testPathAPIProjects + testProjectID + "/merge_requests/3/award_emoji":
		default:
			t.Errorf(fmtUnexpPath, r.URL.Path)
			return
		}
		switch r.Method {
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Award Emoji Name has already been taken Not Found"}`)
		case http.MethodGet:
			switch r.URL.Query().Get("page") {
			case "1":
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":30,"name":"eyes","user":{"id":3,"username":"user3"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "2", NextPage: "2", PerPage: "100", Total: "2"})
			case "2":
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":31,"name":"eyes","user":{"id":9,"username":"current"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`, testutil.PaginationHeaders{Page: "2", TotalPages: "2", PerPage: "100", Total: "2"})
			default:
				t.Errorf("page = %q, want 1 or 2", r.URL.Query().Get("page"))
			}
		default:
			t.Errorf("method = %s, want POST or GET", r.Method)
		}
	}))

	out, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{
		ProjectID: testProjectID,
		IID:       3,
		Name:      "eyes",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want 4", requests)
	}
	if out.ID != 31 || out.Name != "eyes" || out.User == nil || out.User.ID != 9 {
		t.Fatalf("award = {ID:%d Name:%q User:%v}, want {ID:31 Name:eyes User.ID:9}", out.ID, out.Name, out.User)
	}
}

// TestCreateMRAwardEmoji_DuplicateByMessage_ReturnsTheExistingAward holds the
// second half of the condition that decides whether a refused create is worth
// searching for. GitLab reports "Name has already been taken" under more than
// one status, so the handler asks both whether the status was 404 and whether
// the message names a duplicate; every other test sends a 404 whose message
// also names one, which settles the first question and leaves the second
// answered by nothing. A duplicate reported as a 400 is the case only this
// test reaches, and the award already on the merge request is what the caller
// must get back rather than an error about a name it cannot use.
func TestCreateMRAwardEmoji_DuplicateByMessage_ReturnsTheExistingAward(t *testing.T) {
	client := testutil.NewTestClient(t, mrAwardEmojiCreateRefusalHandler(t, http.StatusBadRequest,
		`{"message":"Name has already been taken"}`, 9))

	out, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: testProjectID, IID: 3, Name: "eyes"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 31 {
		t.Fatalf("award = %+v, want the existing award with ID 31", out)
	}
}

// TestCreateMRAwardEmoji_NotFoundWithNoExistingAward_SurfacesTheRefusal holds
// the other side of the same branch. A 404 is searched because it is how a
// duplicate arrives, but it is also how a merge request that is not there
// arrives, and those are told apart by the search finding nothing. When it
// finds nothing the original refusal has to reach the caller with the hint that
// names the merge request, because a handler that swallowed it would answer a
// missing merge request with a silence the model reads as success.
func TestCreateMRAwardEmoji_NotFoundWithNoExistingAward_SurfacesTheRefusal(t *testing.T) {
	client := testutil.NewTestClient(t, mrAwardEmojiCreateRefusalHandler(t, http.StatusNotFound,
		`{"message":"404 Merge Request Not Found"}`, 3))

	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: testProjectID, IID: 3, Name: "eyes"})
	if err == nil {
		t.Fatal("expected the 404 to reach the caller when no award of the current user matches")
	}
	assertErrContains(t, err, "verify the merge request exists with merge_request.get")
}

// mrAwardEmojiCreateRefusalHandler answers a create with the given refusal and
// then serves the one page of award emoji the handler searches, attributed to
// awardUserID. Passing the current user's own ID makes the search succeed and
// any other ID makes it come up empty, which is the only difference between the
// two cases above. The list is bounded at one request for the reason
// [boundedMRAwardEmojiListHandler] carries.
func mrAwardEmojiCreateRefusalHandler(t *testing.T, status int, body string, awardUserID int) http.HandlerFunc {
	t.Helper()
	const emojiPath = testPathAPIProjects + testProjectID + "/merge_requests/3/award_emoji"
	var lists atomic.Int64
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/user":
			testutil.RespondJSON(w, http.StatusOK, `{"id":9,"username":"current"}`)
		case r.URL.Path != emojiPath:
			t.Errorf(fmtUnexpPath, r.URL.Path)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, status, body)
		case r.Method == http.MethodGet:
			if n := lists.Add(1); n > 1 {
				t.Errorf("list requests = %d, want 1", n)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
				return
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":31,"name":"eyes","user":{"id":`+strconv.Itoa(awardUserID)+`,"username":"someone"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`,
				testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "100", Total: "1"})
		default:
			t.Errorf("method = %s, want POST or GET", r.Method)
		}
	}
}

// Snippet award emoji tests.

// TestListSnippetAwardEmoji_Success verifies that ListSnippetAwardEmoji succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListSnippetAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/snippets/7/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":40,"name":"100","user":{"id":4,"username":"user4"},"created_at":"2026-04-01T00:00:00Z","awardable_id":7,"awardable_type":"Snippet"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{
		ProjectID: testProjectID,
		IID:       7,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Fatalf(fmtExpected1Emoji, len(out.AwardEmoji))
	}
	if out.AwardEmoji[0].Name != "100" {
		t.Errorf("name = %q, want 100", out.AwardEmoji[0].Name)
	}
}

// Formatter tests.

// The guidance sections an award emoji result ends with, so each expectation
// below can pin the whole rendered document. Delete award-emoji operations are
// destructive, so the hint that names the confirmation is part of every one.
const (
	cardHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use the selected tool surface's matching award emoji delete action with award_id, the same resource identifiers, and explicit confirm=true\n"
	linkedListHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use the selected tool surface's matching award emoji delete action with award_id, the same resource identifiers, and explicit confirm=true\n"
	plainListHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use the selected tool surface's matching award emoji delete action with award_id, the same resource identifiers, and explicit confirm=true\n"
)

// TestFormatAwardEmojiNotFound_HostileHints_ReachThePageAsText pins the whole
// not-found card for a result whose hints carry a raw anchor. The two hints are
// sentences this package wrote, but they reach the formatter as fields of the
// result rather than as literals, so each is escaped like any other value read
// off an output and the guidance opens no link.
func TestFormatAwardEmojiNotFound_HostileHints_ReachThePageAsText(t *testing.T) {
	result := formatAwardEmojiNotFound(awardEmojiNotFoundOutput{
		Identifier: "award 5 on issue IID 1 in project my-project",
		ListHint:   "<a href=\"http://attacker.invalid\">List</a> the emoji",
		VerifyHint: "Check the award ID",
	})
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("formatAwardEmojiNotFound() = %+v, want one content block", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
	}

	want := "## " + toolutil.EmojiQuestion + " Award Emoji Not Found\n\n" +
		"The award emoji **award 5 on issue IID 1 in project my-project** does not exist or is not accessible with your current permissions.\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- &lt;a href=\"http://attacker.invalid\">List&lt;/a> the emoji\n" +
		"- Check the award ID\n"
	if text.Text != want {
		t.Errorf("not-found card mismatch:\ngot:\n%s\nwant:\n%s", text.Text, want)
	}
}

// TestFormatListMarkdownString_WithEmoji pins the whole list document: the
// collection is a table, the awarding user is a link where GitLab gave a profile
// URL and the bare handle where it did not, and the footer carries the
// instruction to keep the links because the table has some.
func TestFormatListMarkdownString_WithEmoji(t *testing.T) {
	out := ListOutput{
		AwardEmoji: []Output{
			{ID: 10, Name: testEmojiThumbsup, User: &UserOutput{ID: 1, Username: "admin", WebURL: "https://gitlab.example.com/admin"}, CreatedAt: "2026-01-01T00:00:00Z", AwardableID: 1, AwardableType: "Issue"},
			{ID: 11, Name: "heart", User: &UserOutput{ID: 2, Username: "dev"}, CreatedAt: "2026-02-01T00:00:00Z", AwardableID: 1, AwardableType: "Issue"},
		},
	}

	got := FormatListMarkdownString(out)

	want := "## Award Emoji (2)\n\n" +
		"| ID | Emoji | User | Awarded |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 10 | :thumbsup: | [@admin](https://gitlab.example.com/admin) | 1 Jan 2026 00:00 UTC |\n" +
		"| 11 | :heart: | @dev | 1 Feb 2026 00:00 UTC |\n" +
		linkedListHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdownString_Paginated pins the heading and the footer of a
// page of a larger list: the heading counts the total GitLab reported, and the
// pagination line opens a block of its own after the last row rather than
// continuing the table.
func TestFormatListMarkdownString_Paginated(t *testing.T) {
	out := ListOutput{
		AwardEmoji: []Output{{ID: 10, Name: testEmojiThumbsup, User: &UserOutput{ID: 1, Username: "admin"}, CreatedAt: "2026-01-01T00:00:00Z"}},
		Pagination: toolutil.PaginationOutput{Page: 2, PerPage: 1, TotalItems: 3, TotalPages: 3, NextPage: 3, HasMore: true},
	}

	got := FormatListMarkdownString(out)

	want := "## Award Emoji (3)\n\n" +
		"Showing 1 of 3 results (page 2 of 3)\n\n" +
		"| ID | Emoji | User | Awarded |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 10 | :thumbsup: | @admin | 1 Jan 2026 00:00 UTC |\n\n" +
		"Page 2 of 3 | 3 items total | 1 per page\n" +
		plainListHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdownString_Empty verifies the ListMarkdownString_Empty Markdown formatter for a representative liststring_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{}}
	md := FormatListMarkdownString(out)
	if md != "No award emoji found.\n" {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatMarkdownString pins the whole card of one award emoji, the
// awardable included: an award that does not say what it sits on cannot be
// found again, and the object is both in the heading and in a row of its own.
func TestFormatMarkdownString(t *testing.T) {
	out := Output{
		ID:            10,
		Name:          testEmojiThumbsup,
		User:          &UserOutput{ID: 1, Username: "admin", Name: "Admin User", WebURL: "https://gitlab.example.com/admin"},
		CreatedAt:     "2026-01-01T00:00:00Z",
		AwardableID:   42,
		AwardableType: "Issue",
	}

	got := FormatMarkdownString(out)

	want := "## Award Emoji :thumbsup: on Issue 42\n\n" +
		"- **ID**: 10\n" +
		"- **Name**: :thumbsup:\n" +
		"- **Awarded On**: Issue (ID 42)\n" +
		"- **User**:\n" +
		"  - **ID**: 1\n" +
		"  - **Name**: Admin User\n" +
		"  - **Username**: [@admin](https://gitlab.example.com/admin)\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		cardHintsBlock
	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{}}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// contains reports whether contains.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && containsHelper(s, substr))
}

// containsHelper reports whether contains helper.
func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Int64 validation tests.

// assertErrContains checks err contains invariants for tests.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestListIssueAwardEmoji_InvalidIID verifies the ListIssueAwardEmoji_InvalidIID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: -1})
	assertErrContains(t, err, testFieldIssueIID)
}

// TestGetIssueAwardEmoji_InvalidIDs verifies the GetIssueAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetIssueAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateIssueAwardEmoji_InvalidIID verifies the CreateIssueAwardEmoji_InvalidIID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateIssueAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIssueIID)
}

// TestDeleteIssueAwardEmoji_InvalidIDs verifies the DeleteIssueAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	err = DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListIssueNoteAwardEmoji_InvalidIDs verifies the ListIssueNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetIssueNoteAwardEmoji_InvalidIDs verifies the GetIssueNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateIssueNoteAwardEmoji_InvalidIDs verifies the CreateIssueNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteIssueNoteAwardEmoji_InvalidIDs verifies the DeleteIssueNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	err = DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListMRAwardEmoji_InvalidIID verifies the ListMRAwardEmoji_InvalidIID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListMRAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListMRAwardEmoji(t.Context(), client, MRListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldMRIID)
}

// TestGetMRAwardEmoji_InvalidIDs verifies the GetMRAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetMRAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetMRAwardEmoji(t.Context(), client, MRGetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	_, err = GetMRAwardEmoji(t.Context(), client, MRGetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateMRAwardEmoji_InvalidIID verifies the CreateMRAwardEmoji_InvalidIID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateMRAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: "p", IID: -5, Name: testEmojiStar})
	assertErrContains(t, err, testFieldMRIID)
}

// TestDeleteMRAwardEmoji_InvalidIDs verifies the DeleteMRAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteMRAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	err = DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: -1})
	assertErrContains(t, err, testFieldAwardID)
}

// TestMRAwardEmoji_OmittedIdentifiers_AreRefusedBeforeTheRequest states which
// side of the guard the MR handlers have to refuse on. A model that leaves an
// identifier out of its arguments does not send a negative number: the field
// decodes to 0, so a guard written `< 0` admits every omitted identifier and
// sends GitLab `merge_request_iid=0` or `award_id=0`, turning a missing
// argument into an instance-side 404 the model reads as "the award is gone".
// The other MR cases pass -1 and -5, which both guards refuse either way, so
// zero is the only value that pins the boundary. [testutil.ForbiddenHandler] is
// the second half of the assertion: it fails the test if any request was made
// at all, so this says the refusal happened here rather than at GitLab.
func TestMRAwardEmoji_OmittedIdentifiers_AreRefusedBeforeTheRequest(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldMRIID)

	err = DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListMRNoteAwardEmoji_InvalidIDs verifies the ListMRNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldMRIID)
	_, err = ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetMRNoteAwardEmoji_InvalidIDs verifies the GetMRNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	_, err = GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateMRNoteAwardEmoji_InvalidIDs verifies the CreateMRNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldMRIID)
	_, err = CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteMRNoteAwardEmoji_InvalidIDs verifies the DeleteMRNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	err = DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListSnippetAwardEmoji_InvalidIID verifies the ListSnippetAwardEmoji_InvalidIID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListSnippetAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldSnippetID)
}

// TestGetSnippetAwardEmoji_InvalidIDs verifies the GetSnippetAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetSnippetAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateSnippetAwardEmoji_InvalidIID verifies the CreateSnippetAwardEmoji_InvalidIID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateSnippetAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateSnippetAwardEmoji(t.Context(), client, SnippetCreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldSnippetID)
}

// TestDeleteSnippetAwardEmoji_InvalidIDs verifies the DeleteSnippetAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteSnippetAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	err = DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListSnippetNoteAwardEmoji_InvalidIDs verifies the ListSnippetNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetSnippetNoteAwardEmoji_InvalidIDs verifies the GetSnippetNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateSnippetNoteAwardEmoji_InvalidIDs verifies the CreateSnippetNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteSnippetNoteAwardEmoji_InvalidIDs verifies the DeleteSnippetNoteAwardEmoji_InvalidIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	err = DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedValidation identifies the err expected validation constant used by this package.
const errExpectedValidation = "expected validation error"

// covEmojiJSON identifies the cov emoji JSON constant used by this package.
const covEmojiJSON = `[{"id":1,"name":"thumbsup","user":{"id":5,"username":"alice"},"created_at":"2026-06-01T10:00:00Z","awardable_id":10,"awardable_type":"Issue"}]`

// covEmojiSingle identifies the cov emoji single constant used by this package.
const covEmojiSingle = `{"id":1,"name":"thumbsup","user":{"id":5,"username":"alice"},"created_at":"2026-06-01T10:00:00Z","awardable_id":10,"awardable_type":"Issue"}`

// covBadHandler supports cov bad handler assertions in awardemoji tests.
func covBadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	})
}

// covOKList supports cov ok list assertions in awardemoji tests.
func covOKList() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covEmojiJSON)
	})
}

// covOKSingle supports cov ok single assertions in awardemoji tests.
func covOKSingle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covEmojiSingle)
	})
}

// covOKDelete supports cov ok delete assertions in awardemoji tests.
func covOKDelete() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// ======================== Issue Emoji ========================.

// TestListIssueAwardEmoji_Validation verifies the ListIssueAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueAwardEmoji_APIError verifies that ListIssueAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueAwardEmoji_Success_Cov verifies the ListIssueAwardEmoji_Success_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKList())
	out, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.AwardEmoji) != 1 || out.AwardEmoji[0].Name != "thumbsup" {
		t.Errorf("unexpected: %+v", out)
	}
}

// TestGetIssueAwardEmoji_Validation verifies the GetIssueAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueAwardEmoji_APIError verifies that GetIssueAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueAwardEmoji_Success_Cov verifies the GetIssueAwardEmoji_Success_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKSingle())
	out, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Name != "thumbsup" {
		t.Error("unexpected name")
	}
}

// TestCreateIssueAwardEmoji_Validation verifies the CreateIssueAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateIssueAwardEmoji_APIError verifies that CreateIssueAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{ProjectID: "p", IID: 1, Name: "thumbsup"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateIssueAwardEmoji_Success_Cov verifies the CreateIssueAwardEmoji_Success_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKSingle())
	out, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{ProjectID: "p", IID: 1, Name: "thumbsup"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != 1 {
		t.Error("unexpected ID")
	}
}

// TestDeleteIssueAwardEmoji_Validation verifies the DeleteIssueAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteIssueAwardEmoji_APIError_Cov verifies that DeleteIssueAwardEmoji_Cov returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteIssueAwardEmoji_APIError_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteIssueAwardEmoji_Success_Cov verifies the DeleteIssueAwardEmoji_Success_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKDelete())
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

// ======================== Issue Note Emoji ========================.

// TestListIssueNoteAwardEmoji_Validation verifies the ListIssueNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueNoteAwardEmoji_APIError verifies that ListIssueNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueNoteAwardEmoji_Success_Cov verifies the ListIssueNoteAwardEmoji_Success_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListIssueNoteAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKList())
	out, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Error("expected 1 emoji")
	}
}

// TestGetIssueNoteAwardEmoji_Validation verifies the GetIssueNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueNoteAwardEmoji_APIError verifies that GetIssueNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateIssueNoteAwardEmoji_Validation verifies the CreateIssueNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateIssueNoteAwardEmoji_APIError verifies that CreateIssueNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteIssueNoteAwardEmoji_Validation verifies the DeleteIssueNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteIssueNoteAwardEmoji_APIError verifies that DeleteIssueNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== MR Emoji ========================.

// TestListMRAwardEmoji_Validation verifies the ListMRAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRAwardEmoji(t.Context(), client, MRListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRAwardEmoji_APIError verifies that ListMRAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRAwardEmoji(t.Context(), client, MRListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRAwardEmoji_Success_Cov verifies the ListMRAwardEmoji_Success_Cov handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListMRAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKList())
	out, err := ListMRAwardEmoji(t.Context(), client, MRListInput{ProjectID: "p", IID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Error("expected 1 emoji")
	}
}

// TestGetMRAwardEmoji_Validation verifies the GetMRAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRAwardEmoji(t.Context(), client, MRGetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRAwardEmoji_APIError verifies that GetMRAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRAwardEmoji(t.Context(), client, MRGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateMRAwardEmoji_Validation verifies the CreateMRAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateMRAwardEmoji_APIError verifies that CreateMRAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: "p", IID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteMRAwardEmoji_Validation verifies the DeleteMRAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteMRAwardEmoji_APIError verifies that DeleteMRAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== MR Note Emoji ========================.

// TestListMRNoteAwardEmoji_Validation verifies the ListMRNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRNoteAwardEmoji_APIError verifies that ListMRNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRNoteAwardEmoji_Validation verifies the GetMRNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRNoteAwardEmoji_APIError verifies that GetMRNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateMRNoteAwardEmoji_Validation verifies the CreateMRNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateMRNoteAwardEmoji_APIError verifies that CreateMRNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteMRNoteAwardEmoji_Validation verifies the DeleteMRNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteMRNoteAwardEmoji_APIError verifies that DeleteMRNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Snippet Emoji ========================.

// TestListSnippetAwardEmoji_Validation verifies the ListSnippetAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListSnippetAwardEmoji_APIError verifies that ListSnippetAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetSnippetAwardEmoji_Validation verifies the GetSnippetAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetSnippetAwardEmoji_APIError verifies that GetSnippetAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateSnippetAwardEmoji_Validation verifies the CreateSnippetAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetAwardEmoji(t.Context(), client, SnippetCreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateSnippetAwardEmoji_APIError verifies that CreateSnippetAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetAwardEmoji(t.Context(), client, SnippetCreateInput{ProjectID: "p", IID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteSnippetAwardEmoji_Validation verifies the DeleteSnippetAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteSnippetAwardEmoji_APIError verifies that DeleteSnippetAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Snippet Note Emoji ========================.

// TestListSnippetNoteAwardEmoji_Validation verifies the ListSnippetNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListSnippetNoteAwardEmoji_APIError verifies that ListSnippetNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetSnippetNoteAwardEmoji_Validation verifies the GetSnippetNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetSnippetNoteAwardEmoji_APIError verifies that GetSnippetNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateSnippetNoteAwardEmoji_Validation verifies the CreateSnippetNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateSnippetNoteAwardEmoji_APIError verifies that CreateSnippetNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteSnippetNoteAwardEmoji_Validation verifies the DeleteSnippetNoteAwardEmoji_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteSnippetNoteAwardEmoji_APIError verifies that DeleteSnippetNoteAwardEmoji returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Formatters ========================.

// TestFormatListMarkdown_Empty_Cov verifies the ListMarkdown_Empty_Cov Markdown formatter for a representative list_empty_cov input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty_Cov(t *testing.T) {
	res := FormatListMarkdown(ListOutput{})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdownString_Empty_Cov pins the whole response of a list with
// nothing in it: one sentence, with no heading counting zero above it.
func TestFormatListMarkdownString_Empty_Cov(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{})

	if want := "No award emoji found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdownString_WithEmoji_Cov pins the whole list document of an
// award GitLab sent no time for: the date cell is empty rather than a zero
// instant, and the footer carries no instruction to keep links the table has
// none of.
func TestFormatListMarkdownString_WithEmoji_Cov(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{{ID: 1, Name: "thumbsup", User: &UserOutput{Username: "alice"}}}}

	got := FormatListMarkdownString(out)

	want := "## Award Emoji (1)\n\n" +
		"| ID | Emoji | User | Awarded |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | :thumbsup: | @alice |  |\n" +
		plainListHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdown_Wrapper verifies the Markdown_Wrapper Markdown formatter for a representative _wrapper input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	res := FormatMarkdown(Output{Name: "thumbsup"})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatMarkdownString_NoCreatedAt pins the whole card of an award GitLab
// sent neither a time nor an awardable for: no row is written for either, where
// a label with nothing after it used to read as a value GitLab sent.
func TestFormatMarkdownString_NoCreatedAt(t *testing.T) {
	got := FormatMarkdownString(Output{Name: "thumbsup", User: &UserOutput{Username: "alice"}})

	want := "## Award Emoji :thumbsup:\n\n" +
		"- **ID**: 0\n" +
		"- **Name**: :thumbsup:\n" +
		"- **User**:\n" +
		"  - **ID**: 0\n" +
		"  - **Username**: @alice\n" +
		cardHintsBlock
	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_WithCreatedAt pins the whole card of an award with a
// time on it, in the display form rather than as GitLab sent it.
func TestFormatMarkdownString_WithCreatedAt(t *testing.T) {
	got := FormatMarkdownString(Output{Name: "thumbsup", User: &UserOutput{Username: "alice"}, CreatedAt: "2026-06-01T10:00:00Z"})

	want := "## Award Emoji :thumbsup:\n\n" +
		"- **ID**: 0\n" +
		"- **Name**: :thumbsup:\n" +
		"- **User**:\n" +
		"  - **ID**: 0\n" +
		"  - **Username**: @alice\n" +
		"- **Created**: 1 Jun 2026 10:00 UTC\n" +
		cardHintsBlock
	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_HalfAnAwardable_NamesThePartGitLabSent pins the two
// cards the other formatter tests cannot reach: an award whose awardable GitLab
// described by type alone, and one it described by ID alone.
//
// Both matter because the awardable is the only identifier that leads back to
// what was reacted to, so a card that drops the half GitLab did send is worse
// than one that says nothing. The two halves are also what tells the guard
// apart from a looser one: with both absent the row is omitted and with both
// present it reads "Issue (ID 42)", and those two agree under either reading of
// the condition, so only a card carrying exactly one of them can say whether
// "type and ID are both missing" was asked as "and" or as "or".
func TestFormatMarkdownString_HalfAnAwardable_NamesThePartGitLabSent(t *testing.T) {
	tests := []struct {
		name string
		out  Output
		want string
	}{
		{
			name: "type without an ID",
			out:  Output{Name: testEmojiThumbsup, AwardableType: "Issue"},
			want: "## Award Emoji :thumbsup: on Issue\n\n" +
				"- **ID**: 0\n" +
				"- **Name**: :thumbsup:\n" +
				"- **Awarded On**: Issue\n" +
				cardHintsBlock,
		},
		{
			name: "ID without a type",
			out:  Output{Name: testEmojiThumbsup, AwardableID: 7},
			want: "## Award Emoji :thumbsup: on 7\n\n" +
				"- **ID**: 0\n" +
				"- **Name**: :thumbsup:\n" +
				"- **Awarded On**: ID 7\n" +
				cardHintsBlock,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatMarkdownString(tc.out); got != tc.want {
				t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// TestFormatMarkdownString_NoName_WritesNeitherHeadingNorRow pins the card of
// an award GitLab sent no name for. Both places the name is written guard
// against that, and each guard exists for a different reason: the cell would
// otherwise read "::", which is not an emoji anybody can look up, and the
// heading would otherwise read "Award Emoji :: on Issue 42". Nothing until now
// sent an unnamed award, so both guards were written and neither was asked.
func TestFormatMarkdownString_NoName_WritesNeitherHeadingNorRow(t *testing.T) {
	got := FormatMarkdownString(Output{ID: 10, AwardableType: "Issue", AwardableID: 42})

	want := "## Award Emoji on Issue 42\n\n" +
		"- **ID**: 10\n" +
		"- **Awarded On**: Issue (ID 42)\n" +
		cardHintsBlock
	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdownString_UserGitLabDidNotSend_KeepsTheCellAndTheFooterBare
// pins the row of an award whose user GitLab left out, which is what a list
// carries once the awarding account is gone. Two things have to follow from it
// and neither was exercised: the user cell is empty rather than a link to
// nowhere, and the footer drops the instruction to preserve the table's links,
// because a table with no links in it has none to preserve and the instruction
// would send the model looking for them.
func TestFormatListMarkdownString_UserGitLabDidNotSend_KeepsTheCellAndTheFooterBare(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{
		{ID: 12, Name: testEmojiThumbsup, CreatedAt: "2026-01-01T00:00:00Z", AwardableID: 1, AwardableType: "Issue"},
	}}

	got := FormatListMarkdownString(out)

	want := "## Award Emoji (1)\n\n" +
		"| ID | Emoji | User | Awarded |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 12 | :thumbsup: |  | 1 Jan 2026 00:00 UTC |\n" +
		plainListHintsBlock
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestListIssueAwardEmoji_ForwardsListQuery verifies that order_by, sort, and
// keyset pagination parameters (mirrored from gl.ListAwardEmojiOptions /
// gl.ListOptions) are forwarded to the underlying list request.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the request query string carries every supplied list option.
func TestListIssueAwardEmoji_ForwardsListQuery(t *testing.T) {
	var gotQuery url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "5", Total: "0"})
	}))

	in := IssueListInput{
		ProjectID: testProjectID, IID: 1, OrderBy: "created_at", Sort: "desc",
		Page:       2,
		PerPage:    5,
		Pagination: "keyset",
		PageToken:  "42",
	}
	if _, err := ListIssueAwardEmoji(t.Context(), client, in); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for key, want := range map[string]string{
		"order_by":   "created_at",
		"sort":       "desc",
		"page":       "2",
		"per_page":   "5",
		"pagination": "keyset",
		"page_token": "42",
	} {
		t.Run(key, func(t *testing.T) {
			if got := gotQuery.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		})
	}
}

// TestToOutput_FullUserAndTimestamps verifies that toOutput migrates the award
// emoji onto a full user object (mirroring gl.BasicUser) and surfaces both
// created_at and updated_at timestamps.
// The test exercises the conversion path only.
// It asserts every mirrored field is populated from the SDK struct.
func TestToOutput_FullUserAndTimestamps(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	userCreated := time.Date(2020, 5, 5, 0, 0, 0, 0, time.UTC)
	out := toOutput(&gl.AwardEmoji{
		ID:   7,
		Name: testEmojiThumbsup,
		User: gl.BasicUser{
			ID: 9, Username: "alice", Name: "Alice", State: "active",
			CreatedAt: &userCreated, AvatarURL: "https://gitlab.example.com/a.png", WebURL: "https://gitlab.example.com/alice",
		},
		CreatedAt:     &created,
		UpdatedAt:     &updated,
		AwardableID:   3,
		AwardableType: "Issue",
	})
	if out.User == nil {
		t.Fatal("expected user object")
	}
	if out.User.ID != 9 || out.User.Username != "alice" || out.User.Name != "Alice" || out.User.State != "active" ||
		out.User.AvatarURL == "" || out.User.WebURL == "" || out.User.CreatedAt == "" {
		t.Fatalf("user object missing mirrored fields: %+v", out.User)
	}
	if out.CreatedAt == "" || out.UpdatedAt == "" {
		t.Fatalf("expected created_at and updated_at, got created=%q updated=%q", out.CreatedAt, out.UpdatedAt)
	}
}

// TestAwardEmojiUserMarkdown_NilUser verifies the list formatter renders an
// empty awarding-user cell when the award has no user object.
// The test exercises the Markdown rendering path only.
// It asserts the rendered string omits any user link.
func TestAwardEmojiUserMarkdown_NilUser(t *testing.T) {
	if got := awardEmojiUserMarkdown(Output{Name: "thumbsup"}); got != "" {
		t.Errorf("awardEmojiUserMarkdown(nil user) = %q, want empty", got)
	}
}

// ======================== Action Specs ========================.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	specs := allAwardEmojiActionSpecs(client)
	byTool := awardEmojiSpecsByTool(t, specs)

	if len(specs) != 24 {
		t.Fatalf("len(ActionSpecs) = %d, want 24", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "awardemoji" {
			t.Fatalf("OwnerPackage for %s = %q, want awardemoji", spec.Name, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Fatalf("IndividualTool.Description for %s should follow the Returns:/See also: form, got %q", spec.Name, desc)
		}
	}
	if byTool["gitlab_issue_emoji_list"].ParameterGuidance["issue_iid"].SemanticRole == "" {
		t.Fatal("gitlab_issue_emoji_list should expose issue_iid parameter guidance")
	}
}

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, covEmojiSingle)
			return
		}
		path := r.URL.Path
		// Single resource if path has specific award ID pattern
		if strings.Contains(path, "/award_emoji/") {
			testutil.RespondJSON(w, http.StatusOK, covEmojiSingle)
		} else {
			testutil.RespondJSON(w, http.StatusOK, covEmojiJSON)
		}
	})

	client := testutil.NewTestClient(t, mux)
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_emoji_list", map[string]any{"project_id": "p", "issue_iid": 1}},
		{"gitlab_issue_emoji_get", map[string]any{"project_id": "p", "issue_iid": 1, "award_id": 1}},
		{"gitlab_issue_emoji_create", map[string]any{"project_id": "p", "issue_iid": 1, "name": "thumbsup"}},
		{"gitlab_issue_emoji_delete", map[string]any{"project_id": "p", "issue_iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_list", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1}},
		{"gitlab_issue_note_emoji_get", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_create", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_issue_note_emoji_delete", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_list", map[string]any{"project_id": "p", "merge_request_iid": 1}},
		{"gitlab_mr_emoji_get", map[string]any{"project_id": "p", "merge_request_iid": 1, "award_id": 1}},
		{"gitlab_mr_emoji_create", map[string]any{"project_id": "p", "merge_request_iid": 1, "name": "thumbsup"}},
		{"gitlab_mr_emoji_delete", map[string]any{"project_id": "p", "merge_request_iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_list", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1}},
		{"gitlab_mr_note_emoji_get", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_create", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_mr_note_emoji_delete", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_list", map[string]any{"project_id": "p", "snippet_id": 1}},
		{"gitlab_snippet_emoji_get", map[string]any{"project_id": "p", "snippet_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_create", map[string]any{"project_id": "p", "snippet_id": 1, "name": "thumbsup"}},
		{"gitlab_snippet_emoji_delete", map[string]any{"project_id": "p", "snippet_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_list", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1}},
		{"gitlab_snippet_note_emoji_get", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_create", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_snippet_note_emoji_delete", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "award_id": 1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// TestActionSpecs_GetNotFound validates the GetNotFound route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	client := testutil.NewTestClient(t, mux)
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))

	for _, tc := range awardEmojiGetToolCases() {
		t.Run(tc.name+"_404", func(t *testing.T) {
			res, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
			}
			if _, ok := res.(awardEmojiNotFoundOutput); !ok {
				t.Fatalf("result type = %T, want awardEmojiNotFoundOutput", res)
			}
			toolResult := toolutil.MarkdownForResult(res)
			if toolResult == nil || !toolResult.IsError {
				t.Fatalf("expected MarkdownForResult to return an error CallToolResult for %s", tc.name)
			}
		})
	}
}

// TestActionSpecs_GetNotFound_NamesTheArgumentsAsWritten verifies each get
// route's not-found card names the award, its parent and its project as the
// caller wrote them. A JSON number reaches a route as a float64, and %v
// printed every id of a million or more in exponent form, so a caller asking
// for award 31234567 was told award 3.1234567e+07 was not there.
func TestActionSpecs_GetNotFound_NamesTheArgumentsAsWritten(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))
	const award, parent, note, project = float64(31234567), float64(2345678), float64(3456789), float64(12345678)
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "gitlab_issue_emoji_get",
			args: map[string]any{"project_id": project, "issue_iid": parent, "award_id": award},
			want: "award 31234567 on issue IID 2345678 in project 12345678",
		},
		{
			name: "gitlab_issue_note_emoji_get",
			args: map[string]any{"project_id": project, "issue_iid": parent, "note_id": note, "award_id": award},
			want: "award 31234567 on note 3456789 (issue IID 2345678) in project 12345678",
		},
		{
			name: "gitlab_mr_emoji_get",
			args: map[string]any{"project_id": project, "merge_request_iid": parent, "award_id": award},
			want: "award 31234567 on MR IID 2345678 in project 12345678",
		},
		{
			name: "gitlab_mr_note_emoji_get",
			args: map[string]any{"project_id": project, "merge_request_iid": parent, "note_id": note, "award_id": award},
			want: "award 31234567 on note 3456789 (MR IID 2345678) in project 12345678",
		},
		{
			name: "gitlab_snippet_emoji_get",
			args: map[string]any{"project_id": project, "snippet_id": parent, "award_id": award},
			want: "award 31234567 on snippet IID 2345678 in project 12345678",
		},
		{
			name: "gitlab_snippet_note_emoji_get",
			args: map[string]any{"project_id": project, "snippet_id": parent, "note_id": note, "award_id": award},
			want: "award 31234567 on note 3456789 (snippet IID 2345678) in project 12345678",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
			}
			notFound, ok := res.(awardEmojiNotFoundOutput)
			if !ok || notFound.Identifier != tc.want {
				t.Errorf("Route.Handler(%s) = %#v, want the not-found card naming %q", tc.name, res, tc.want)
			}
		})
	}
}

// TestActionSpecs_GetForbidden_StaysAnError holds the limit of what the
// not-found wrapper is allowed to absorb. A get route answers a 404 with a card
// saying the award is not there, which is the right answer to that one status
// and to no other: answering a 403 the same way tells a model the award was
// removed when what the token actually lacks is permission, and the model's
// next step is to award it again. The status is therefore part of the
// condition, not a detail of it, and this test is what says so for every get
// route at once.
func TestActionSpecs_GetForbidden_StaysAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	})

	client := testutil.NewTestClient(t, mux)
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))

	for _, tc := range awardEmojiGetToolCases() {
		t.Run(tc.name+"_403", func(t *testing.T) {
			res, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) returned %#v and no error, want the 403 to reach the caller", tc.name, res)
			}
			if _, ok := res.(awardEmojiNotFoundOutput); ok {
				t.Fatalf("Route.Handler(%s) answered a 403 with the not-found card", tc.name)
			}
		})
	}
}

// TestCreateAPIErrors verifies that CreateAPIErrors returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateAPIErrors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	ctx := t.Context()

	t.Run("CreateIssueNoteAwardEmoji", func(t *testing.T) {
		_, err := CreateIssueNoteAwardEmoji(ctx, client, IssueCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateMRAwardEmoji", func(t *testing.T) {
		_, err := CreateMRAwardEmoji(ctx, client, MRCreateInput{ProjectID: "p", IID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateMRNoteAwardEmoji", func(t *testing.T) {
		_, err := CreateMRNoteAwardEmoji(ctx, client, MRCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateSnippetAwardEmoji", func(t *testing.T) {
		_, err := CreateSnippetAwardEmoji(ctx, client, SnippetCreateInput{ProjectID: "p", IID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateSnippetNoteAwardEmoji", func(t *testing.T) {
		_, err := CreateSnippetNoteAwardEmoji(ctx, client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestActionSpecs_CreateErrors validates the CreateErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_CreateErrors(t *testing.T) {
	assertActionSpecMutationErrors(t, http.MethodPost, []awardEmojiActionSpecCase{
		{"gitlab_issue_emoji_create", map[string]any{"project_id": "p", "issue_iid": 1, "name": "thumbsup"}},
		{"gitlab_issue_note_emoji_create", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_mr_emoji_create", map[string]any{"project_id": "p", "merge_request_iid": 1, "name": "thumbsup"}},
		{"gitlab_mr_note_emoji_create", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_snippet_emoji_create", map[string]any{"project_id": "p", "snippet_id": 1, "name": "thumbsup"}},
		{"gitlab_snippet_note_emoji_create", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "name": "thumbsup"}},
	})
}

type awardEmojiActionSpecCase struct {
	name string
	args map[string]any
}

// awardEmojiGetToolCases is the one list of get routes and the arguments that
// reach them, shared by the tests that ask what a get route does with a 404 and
// with every other refusal. One list, because the two answers are only
// comparable if both were asked of the same six routes.
func awardEmojiGetToolCases() []awardEmojiActionSpecCase {
	return []awardEmojiActionSpecCase{
		{"gitlab_issue_emoji_get", map[string]any{"project_id": "p", "issue_iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_get", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_get", map[string]any{"project_id": "p", "merge_request_iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_get", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_get", map[string]any{"project_id": "p", "snippet_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_get", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "award_id": 1}},
	}
}

func assertActionSpecMutationErrors(t *testing.T, method string, cases []awardEmojiActionSpecCase) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == method {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covEmojiJSON)
	})

	client := testutil.NewTestClient(t, mux)
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tc.name)
			}
		})
	}
}

// TestActionSpecs_DeleteErrors validates the DeleteErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	assertActionSpecMutationErrors(t, http.MethodDelete, []awardEmojiActionSpecCase{
		{"gitlab_issue_emoji_delete", map[string]any{"project_id": "p", "issue_iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_delete", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_delete", map[string]any{"project_id": "p", "merge_request_iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_delete", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_delete", map[string]any{"project_id": "p", "snippet_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_delete", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "award_id": 1}},
	})
}

// TestDeleteAwardEmoji_NotFoundHints verifies that DeleteAwardEmoji_NotFoundHints returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteAwardEmoji_NotFoundHints(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))

	tests := []struct {
		name string
		call func(context.Context) error
		// list is the canonical action the hint sends a model to, which is
		// what every surface resolves: a tool name there is right on one
		// surface of three.
		list string
	}{
		{"DeleteIssueAwardEmoji", func(ctx context.Context) error {
			return DeleteIssueAwardEmoji(ctx, client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		}, "issue.emoji_issue_list"},
		{"DeleteIssueNoteAwardEmoji", func(ctx context.Context) error {
			return DeleteIssueNoteAwardEmoji(ctx, client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		}, "issue.emoji_issue_note_list"},
		{"DeleteMRAwardEmoji", func(ctx context.Context) error {
			return DeleteMRAwardEmoji(ctx, client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		}, "merge_request.emoji_mr_list"},
		{"DeleteMRNoteAwardEmoji", func(ctx context.Context) error {
			return DeleteMRNoteAwardEmoji(ctx, client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		}, "merge_request.emoji_mr_note_list"},
		{"DeleteSnippetAwardEmoji", func(ctx context.Context) error {
			return DeleteSnippetAwardEmoji(ctx, client, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		}, "snippet.emoji_snippet_list"},
		{"DeleteSnippetNoteAwardEmoji", func(ctx context.Context) error {
			return DeleteSnippetNoteAwardEmoji(ctx, client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		}, "snippet.emoji_snippet_note_list"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t.Context())
			if err == nil {
				t.Fatal("expected not-found error")
			}
			if !strings.Contains(err.Error(), "award already removed") {
				t.Fatalf("error = %q, want not-found hint", err.Error())
			}
			if !strings.Contains(err.Error(), " awards with "+tc.list+" to verify award_id") {
				t.Errorf("error = %q, want the hint to name %s", err.Error(), tc.list)
			}
			if strings.Contains(err.Error(), "gitlab_") {
				t.Errorf("error = %q, want no tool name, which the default surface does not register", err.Error())
			}
		})
	}
}

// TestDeleteAwardEmoji_PermissionRefusedWith401_NamesTheAwarder verifies
// that each of the six delete actions hints who may remove an award on the
// 401 GitLab refuses anyone else with (lib/api/award_emoji.rb:124), which the
// handlers scoped to 403 and so never hinted; a plain 403 keeps the same hint,
// and a 401 GitLab said was about the token itself gets none.
func TestDeleteAwardEmoji_PermissionRefusedWith401_NamesTheAwarder(t *testing.T) {
	deletes := map[string]func(context.Context, *gitlabclient.Client) error{
		"issue": func(ctx context.Context, c *gitlabclient.Client) error {
			return DeleteIssueAwardEmoji(ctx, c, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		},
		"issue note": func(ctx context.Context, c *gitlabclient.Client) error {
			return DeleteIssueNoteAwardEmoji(ctx, c, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		},
		"merge request": func(ctx context.Context, c *gitlabclient.Client) error {
			return DeleteMRAwardEmoji(ctx, c, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		},
		"merge request note": func(ctx context.Context, c *gitlabclient.Client) error {
			return DeleteMRNoteAwardEmoji(ctx, c, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		},
		"snippet": func(ctx context.Context, c *gitlabclient.Client) error {
			return DeleteSnippetAwardEmoji(ctx, c, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		},
		"snippet note": func(ctx context.Context, c *gitlabclient.Client) error {
			return DeleteSnippetNoteAwardEmoji(ctx, c, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		},
	}
	answers := []struct {
		name     string
		status   int
		body     string
		wantHint bool
	}{
		{"401 from unauthorized!", http.StatusUnauthorized, `{"message":"401 Unauthorized"}`, true},
		{"a plain 403", http.StatusForbidden, `{"message":"403 Forbidden"}`, true},
		{"401 for a revoked token", http.StatusUnauthorized, `{"error":"invalid_token","error_description":"Token was revoked. You have to re-authorize from the user."}`, false},
	}
	for _, answer := range answers {
		for name, call := range deletes {
			t.Run(answer.name+" on "+name, func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, answer.status, answer.body)
				}))
				err := call(t.Context(), client)
				if err == nil {
					t.Fatalf("delete on %s error = nil, want the %d", name, answer.status)
				}
				if got := strings.Contains(err.Error(), hintEmojiOwnerOnly); got != answer.wantHint {
					t.Errorf("delete on %s error = %q carries the awarder hint: %v, want %v", name, err, got, answer.wantHint)
				}
			})
		}
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_emoji_delete", map[string]any{"project_id": "p", "issue_iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_delete", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_delete", map[string]any{"project_id": "p", "merge_request_iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_delete", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_delete", map[string]any{"project_id": "p", "snippet_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_delete", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "award_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test award emoji destructive confirmation.",
				Icons:       toolutil.IconLabel,
			})

			st, ct := mcp.NewInMemoryTransports()
			ctx := context.Background()
			serverSession, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatalf("server connect: %v", err)
			}
			mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
				ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					return &mcp.ElicitResult{Action: "decline"}, nil
				},
			})
			session, connectErr := mcpClient.Connect(ctx, ct, nil)
			if connectErr != nil {
				t.Fatalf("client connect: %v", connectErr)
			}
			t.Cleanup(func() {
				session.Close()
				_ = serverSession.Wait()
			})

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
		})
	}
}

// ----- branch coverage -----

// TestFindExistingMRAwardEmoji_Branches exercises the remaining branches of
// the findExistingMRAwardEmoji helper: failure to load the current user,
// failure to list the merge request's award emoji, and pagination
// termination when the response indicates no next page (resp == nil or
// NextPage == 0). Each branch must return an empty Output with the found
// flag set to false so that the caller can fall back to creating a new
// emoji. Without these tests the "err != nil" and "resp == nil || resp.NextPage == 0"
// branches were never reached, keeping the function below full coverage.
//
// The last case counts requests as well as reading the answer, because the
// answer alone cannot tell the two readings of the exit condition apart: a
// search that never stops returns nothing either, having asked GitLab for page
// 0 for as long as anyone waits.
func TestFindExistingMRAwardEmoji_Branches(t *testing.T) {
	const mrEmojiPath = testPathAPIProjects + testProjectID + "/merge_requests/3/award_emoji"

	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "current user request fails",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v4/user" {
					testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
					return
				}
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			},
		},
		{
			name: "list award emoji request fails",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v4/user":
					testutil.RespondJSON(w, http.StatusOK, `{"id":9,"username":"current"}`)
				case mrEmojiPath:
					testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			},
		},
		{
			name:    "a page naming no next page ends the search in one request",
			handler: boundedMRAwardEmojiListHandler(t, mrEmojiPath),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, found := findExistingMRAwardEmoji(t.Context(), client, MRCreateInput{
				ProjectID: testProjectID,
				IID:       3,
				Name:      "eyes",
			})
			if found {
				t.Fatalf("expected found = false, got true (out=%+v)", out)
			}
			if out.ID != 0 || out.Name != "" {
				t.Fatalf("expected zero-value Output, got %+v", out)
			}
		})
	}
}

// boundedMRAwardEmojiListHandler answers one page of award emoji that names no
// next page, and refuses a second list request rather than answering it.
//
// The bound is the assertion. A search whose exit condition is read the wrong
// way sets the page back to 0 and asks again, for ever; an unbounded fixture
// would keep answering that loop, so the defect would arrive as a suite that
// never finishes instead of as a test that fails, and a run that never
// finishes takes every assertion after it down with it. Refusing the second
// request names the failure in milliseconds and leaves the rest of the binary
// to run.
func boundedMRAwardEmojiListHandler(t *testing.T, emojiPath string) http.HandlerFunc {
	t.Helper()
	var lists atomic.Int64
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			testutil.RespondJSON(w, http.StatusOK, `{"id":9,"username":"current"}`)
		case emojiPath:
			if n := lists.Add(1); n > 1 {
				t.Errorf("list requests = %d, want 1: a page naming no next page did not end the search", n)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
				return
			}
			// Single-page response with NextPage=0.
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"other","user":{"id":1,"username":"u"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "100", Total: "1"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}
}

// allAwardEmojiActionSpecs supports all award emoji action specs assertions in awardemoji tests.
func allAwardEmojiActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	specs := append(IssueActionSpecs(client), MergeRequestActionSpecs(client)...)
	return append(specs, SnippetActionSpecs(client)...)
}

// awardEmojiSpecsByTool supports award emoji specs by tool assertions in awardemoji tests.
func awardEmojiSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
