// awardemoji_test.go contains unit tests for the award emoji MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package awardemoji

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// errNoReachAPI identifies the err no reach API constant used by this package.
const errNoReachAPI = "should not reach API"

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

// TestListIssueAwardEmoji_Success verifies ListIssueAwardEmoji when success.
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
	if out.AwardEmoji[0].UserID != 1 {
		t.Errorf("user_id = %d, want 1", out.AwardEmoji[0].UserID)
	}
}

// TestListIssueAwardEmoji_ValidationError verifies ListIssueAwardEmoji when validation error.
func TestListIssueAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{
		ProjectID: "",
		IID:       1,
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestGetIssueAwardEmoji_Success verifies GetIssueAwardEmoji when success.
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

// TestCreateIssueAwardEmoji_Success verifies CreateIssueAwardEmoji when success.
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

// TestCreateIssueAwardEmoji_ValidationError verifies CreateIssueAwardEmoji when validation error.
func TestCreateIssueAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{
		ProjectID: "",
		IID:       1,
		Name:      testEmojiThumbsup,
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestDeleteIssueAwardEmoji_Success verifies DeleteIssueAwardEmoji when success.
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

// TestDeleteIssueAwardEmoji_APIError verifies DeleteIssueAwardEmoji when API error.
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

// TestListIssueNoteAwardEmoji_Success verifies ListIssueNoteAwardEmoji when success.
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

// TestDeleteIssueNoteAwardEmoji_Success verifies DeleteIssueNoteAwardEmoji when success.
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

// TestListMRAwardEmoji_Success verifies ListMRAwardEmoji when success.
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

// TestCreateMRAwardEmoji_ValidationError verifies CreateMRAwardEmoji when validation error.
func TestCreateMRAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

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
	if out.ID != 31 || out.Name != "eyes" || out.UserID != 9 {
		t.Fatalf("award = {ID:%d Name:%q UserID:%d}, want {ID:31 Name:eyes UserID:9}", out.ID, out.Name, out.UserID)
	}
}

// Snippet award emoji tests.

// TestListSnippetAwardEmoji_Success verifies ListSnippetAwardEmoji when success.
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

// TestFormatListMarkdownString_WithEmoji verifies FormatListMarkdownString when with emoji.
func TestFormatListMarkdownString_WithEmoji(t *testing.T) {
	out := ListOutput{
		AwardEmoji: []Output{
			{ID: 10, Name: testEmojiThumbsup, UserID: 1, Username: "admin", UserWebURL: "https://gitlab.example.com/admin", CreatedAt: "2026-01-01T00:00:00Z", AwardableID: 1, AwardableType: "Issue"},
			{ID: 11, Name: "heart", UserID: 2, Username: "dev", CreatedAt: "2026-02-01T00:00:00Z", AwardableID: 1, AwardableType: "Issue"},
		},
	}
	md := FormatListMarkdownString(out)
	if !contains(md, "Award Emoji (2)") {
		t.Error("expected header with count 2")
	}
	if !contains(md, ":thumbsup:") {
		t.Error("expected :thumbsup:")
	}
	if !contains(md, ":heart:") {
		t.Error("expected :heart:")
	}
	if !contains(md, "[admin](https://gitlab.example.com/admin)") {
		t.Error("expected linked username when user profile URL is available")
	}
}

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{}}
	md := FormatListMarkdownString(out)
	if md != "No award emoji found.\n" {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatMarkdownString verifies FormatMarkdownString.
func TestFormatMarkdownString(t *testing.T) {
	out := Output{
		ID:        10,
		Name:      testEmojiThumbsup,
		UserID:    1,
		Username:  "admin",
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	md := FormatMarkdownString(out)
	if !contains(md, ":thumbsup:") {
		t.Error("expected :thumbsup: in markdown")
	}
	if !contains(md, "admin") {
		t.Error("expected admin in markdown")
	}
	// Delete award-emoji operations are destructive and must require explicit confirmation guidance.
	if !contains(md, "explicit confirm=true") {
		t.Error("expected destructive confirmation hint in markdown")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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

// TestListIssueAwardEmoji_InvalidIID verifies ListIssueAwardEmoji when invalid IID.
func TestListIssueAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: -1})
	assertErrContains(t, err, testFieldIssueIID)
}

// TestGetIssueAwardEmoji_InvalidIDs verifies GetIssueAwardEmoji when invalid IDs.
func TestGetIssueAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateIssueAwardEmoji_InvalidIID verifies CreateIssueAwardEmoji when invalid IID.
func TestCreateIssueAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIssueIID)
}

// TestDeleteIssueAwardEmoji_InvalidIDs verifies DeleteIssueAwardEmoji when invalid IDs.
func TestDeleteIssueAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	err = DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListIssueNoteAwardEmoji_InvalidIDs verifies ListIssueNoteAwardEmoji when invalid IDs.
func TestListIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetIssueNoteAwardEmoji_InvalidIDs verifies GetIssueNoteAwardEmoji when invalid IDs.
func TestGetIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateIssueNoteAwardEmoji_InvalidIDs verifies CreateIssueNoteAwardEmoji when invalid IDs.
func TestCreateIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIssueIID)
	_, err = CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteIssueNoteAwardEmoji_InvalidIDs verifies DeleteIssueNoteAwardEmoji when invalid IDs.
func TestDeleteIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIssueIID)
	err = DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListMRAwardEmoji_InvalidIID verifies ListMRAwardEmoji when invalid IID.
func TestListMRAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRAwardEmoji(t.Context(), client, MRListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldMRIID)
}

// TestGetMRAwardEmoji_InvalidIDs verifies GetMRAwardEmoji when invalid IDs.
func TestGetMRAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRAwardEmoji(t.Context(), client, MRGetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	_, err = GetMRAwardEmoji(t.Context(), client, MRGetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateMRAwardEmoji_InvalidIID verifies CreateMRAwardEmoji when invalid IID.
func TestCreateMRAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: "p", IID: -5, Name: testEmojiStar})
	assertErrContains(t, err, testFieldMRIID)
}

// TestDeleteMRAwardEmoji_InvalidIDs verifies DeleteMRAwardEmoji when invalid IDs.
func TestDeleteMRAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	err = DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: -1})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListMRNoteAwardEmoji_InvalidIDs verifies ListMRNoteAwardEmoji when invalid IDs.
func TestListMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldMRIID)
	_, err = ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetMRNoteAwardEmoji_InvalidIDs verifies GetMRNoteAwardEmoji when invalid IDs.
func TestGetMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	_, err = GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateMRNoteAwardEmoji_InvalidIDs verifies CreateMRNoteAwardEmoji when invalid IDs.
func TestCreateMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldMRIID)
	_, err = CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteMRNoteAwardEmoji_InvalidIDs verifies DeleteMRNoteAwardEmoji when invalid IDs.
func TestDeleteMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldMRIID)
	err = DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListSnippetAwardEmoji_InvalidIID verifies ListSnippetAwardEmoji when invalid IID.
func TestListSnippetAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldSnippetID)
}

// TestGetSnippetAwardEmoji_InvalidIDs verifies GetSnippetAwardEmoji when invalid IDs.
func TestGetSnippetAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateSnippetAwardEmoji_InvalidIID verifies CreateSnippetAwardEmoji when invalid IID.
func TestCreateSnippetAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateSnippetAwardEmoji(t.Context(), client, SnippetCreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldSnippetID)
}

// TestDeleteSnippetAwardEmoji_InvalidIDs verifies DeleteSnippetAwardEmoji when invalid IDs.
func TestDeleteSnippetAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	err = DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListSnippetNoteAwardEmoji_InvalidIDs verifies ListSnippetNoteAwardEmoji when invalid IDs.
func TestListSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetSnippetNoteAwardEmoji_InvalidIDs verifies GetSnippetNoteAwardEmoji when invalid IDs.
func TestGetSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateSnippetNoteAwardEmoji_InvalidIDs verifies CreateSnippetNoteAwardEmoji when invalid IDs.
func TestCreateSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldSnippetID)
	_, err = CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteSnippetNoteAwardEmoji_InvalidIDs verifies DeleteSnippetNoteAwardEmoji when invalid IDs.
func TestDeleteSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
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

// TestListIssueAwardEmoji_Validation verifies ListIssueAwardEmoji when validation.
func TestListIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueAwardEmoji_APIError verifies ListIssueAwardEmoji when API error.
func TestListIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueAwardEmoji_Success_Cov verifies ListIssueAwardEmoji when success cov.
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

// TestGetIssueAwardEmoji_Validation verifies GetIssueAwardEmoji when validation.
func TestGetIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueAwardEmoji_APIError verifies GetIssueAwardEmoji when API error.
func TestGetIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueAwardEmoji_Success_Cov verifies GetIssueAwardEmoji when success cov.
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

// TestCreateIssueAwardEmoji_Validation verifies CreateIssueAwardEmoji when validation.
func TestCreateIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateIssueAwardEmoji_APIError verifies CreateIssueAwardEmoji when API error.
func TestCreateIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueAwardEmoji(t.Context(), client, IssueCreateInput{ProjectID: "p", IID: 1, Name: "thumbsup"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateIssueAwardEmoji_Success_Cov verifies CreateIssueAwardEmoji when success cov.
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

// TestDeleteIssueAwardEmoji_Validation verifies DeleteIssueAwardEmoji when validation.
func TestDeleteIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteIssueAwardEmoji_APIError_Cov verifies DeleteIssueAwardEmoji when API error cov.
func TestDeleteIssueAwardEmoji_APIError_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteIssueAwardEmoji_Success_Cov verifies DeleteIssueAwardEmoji when success cov.
func TestDeleteIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKDelete())
	err := DeleteIssueAwardEmoji(t.Context(), client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

// ======================== Issue Note Emoji ========================.

// TestListIssueNoteAwardEmoji_Validation verifies ListIssueNoteAwardEmoji when validation.
func TestListIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueNoteAwardEmoji_APIError verifies ListIssueNoteAwardEmoji when API error.
func TestListIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, IssueListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueNoteAwardEmoji_Success_Cov verifies ListIssueNoteAwardEmoji when success cov.
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

// TestGetIssueNoteAwardEmoji_Validation verifies GetIssueNoteAwardEmoji when validation.
func TestGetIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueNoteAwardEmoji_APIError verifies GetIssueNoteAwardEmoji when API error.
func TestGetIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, IssueGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateIssueNoteAwardEmoji_Validation verifies CreateIssueNoteAwardEmoji when validation.
func TestCreateIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateIssueNoteAwardEmoji_APIError verifies CreateIssueNoteAwardEmoji when API error.
func TestCreateIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, IssueCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteIssueNoteAwardEmoji_Validation verifies DeleteIssueNoteAwardEmoji when validation.
func TestDeleteIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteIssueNoteAwardEmoji_APIError verifies DeleteIssueNoteAwardEmoji when API error.
func TestDeleteIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== MR Emoji ========================.

// TestListMRAwardEmoji_Validation verifies ListMRAwardEmoji when validation.
func TestListMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRAwardEmoji(t.Context(), client, MRListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRAwardEmoji_APIError verifies ListMRAwardEmoji when API error.
func TestListMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRAwardEmoji(t.Context(), client, MRListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRAwardEmoji_Success_Cov verifies ListMRAwardEmoji when success cov.
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

// TestGetMRAwardEmoji_Validation verifies GetMRAwardEmoji when validation.
func TestGetMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRAwardEmoji(t.Context(), client, MRGetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRAwardEmoji_APIError verifies GetMRAwardEmoji when API error.
func TestGetMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRAwardEmoji(t.Context(), client, MRGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateMRAwardEmoji_Validation verifies CreateMRAwardEmoji when validation.
func TestCreateMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateMRAwardEmoji_APIError verifies CreateMRAwardEmoji when API error.
func TestCreateMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: "p", IID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteMRAwardEmoji_Validation verifies DeleteMRAwardEmoji when validation.
func TestDeleteMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteMRAwardEmoji_APIError verifies DeleteMRAwardEmoji when API error.
func TestDeleteMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRAwardEmoji(t.Context(), client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== MR Note Emoji ========================.

// TestListMRNoteAwardEmoji_Validation verifies ListMRNoteAwardEmoji when validation.
func TestListMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRNoteAwardEmoji_APIError verifies ListMRNoteAwardEmoji when API error.
func TestListMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRNoteAwardEmoji(t.Context(), client, MRListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRNoteAwardEmoji_Validation verifies GetMRNoteAwardEmoji when validation.
func TestGetMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRNoteAwardEmoji_APIError verifies GetMRNoteAwardEmoji when API error.
func TestGetMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRNoteAwardEmoji(t.Context(), client, MRGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateMRNoteAwardEmoji_Validation verifies CreateMRNoteAwardEmoji when validation.
func TestCreateMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateMRNoteAwardEmoji_APIError verifies CreateMRNoteAwardEmoji when API error.
func TestCreateMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, MRCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteMRNoteAwardEmoji_Validation verifies DeleteMRNoteAwardEmoji when validation.
func TestDeleteMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteMRNoteAwardEmoji_APIError verifies DeleteMRNoteAwardEmoji when API error.
func TestDeleteMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRNoteAwardEmoji(t.Context(), client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Snippet Emoji ========================.

// TestListSnippetAwardEmoji_Validation verifies ListSnippetAwardEmoji when validation.
func TestListSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListSnippetAwardEmoji_APIError verifies ListSnippetAwardEmoji when API error.
func TestListSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetAwardEmoji(t.Context(), client, SnippetListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetSnippetAwardEmoji_Validation verifies GetSnippetAwardEmoji when validation.
func TestGetSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetSnippetAwardEmoji_APIError verifies GetSnippetAwardEmoji when API error.
func TestGetSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetAwardEmoji(t.Context(), client, SnippetGetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateSnippetAwardEmoji_Validation verifies CreateSnippetAwardEmoji when validation.
func TestCreateSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetAwardEmoji(t.Context(), client, SnippetCreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateSnippetAwardEmoji_APIError verifies CreateSnippetAwardEmoji when API error.
func TestCreateSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetAwardEmoji(t.Context(), client, SnippetCreateInput{ProjectID: "p", IID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteSnippetAwardEmoji_Validation verifies DeleteSnippetAwardEmoji when validation.
func TestDeleteSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteSnippetAwardEmoji_APIError verifies DeleteSnippetAwardEmoji when API error.
func TestDeleteSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetAwardEmoji(t.Context(), client, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Snippet Note Emoji ========================.

// TestListSnippetNoteAwardEmoji_Validation verifies ListSnippetNoteAwardEmoji when validation.
func TestListSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListSnippetNoteAwardEmoji_APIError verifies ListSnippetNoteAwardEmoji when API error.
func TestListSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, SnippetListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetSnippetNoteAwardEmoji_Validation verifies GetSnippetNoteAwardEmoji when validation.
func TestGetSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetSnippetNoteAwardEmoji_APIError verifies GetSnippetNoteAwardEmoji when API error.
func TestGetSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, SnippetGetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateSnippetNoteAwardEmoji_Validation verifies CreateSnippetNoteAwardEmoji when validation.
func TestCreateSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateSnippetNoteAwardEmoji_APIError verifies CreateSnippetNoteAwardEmoji when API error.
func TestCreateSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, SnippetCreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteSnippetNoteAwardEmoji_Validation verifies DeleteSnippetNoteAwardEmoji when validation.
func TestDeleteSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteSnippetNoteAwardEmoji_APIError verifies DeleteSnippetNoteAwardEmoji when API error.
func TestDeleteSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Formatters ========================.

// TestFormatListMarkdown_Empty_Cov verifies FormatListMarkdown when empty cov.
func TestFormatListMarkdown_Empty_Cov(t *testing.T) {
	res := FormatListMarkdown(ListOutput{})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdownString_Empty_Cov verifies FormatListMarkdownString when empty cov.
func TestFormatListMarkdownString_Empty_Cov(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No award emoji found") {
		t.Error("expected empty message")
	}
}

// TestFormatListMarkdownString_WithEmoji_Cov verifies FormatListMarkdownString when with emoji cov.
func TestFormatListMarkdownString_WithEmoji_Cov(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{{ID: 1, Name: "thumbsup", Username: "alice"}}}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "thumbsup") || !strings.Contains(md, "alice") {
		t.Error("expected emoji details")
	}
}

// TestFormatMarkdown_Wrapper verifies FormatMarkdown when wrapper.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	res := FormatMarkdown(Output{Name: "thumbsup"})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatMarkdownString_NoCreatedAt verifies FormatMarkdownString when no created at.
func TestFormatMarkdownString_NoCreatedAt(t *testing.T) {
	md := FormatMarkdownString(Output{Name: "thumbsup", Username: "alice"})
	if strings.Contains(md, "Created") {
		t.Error("should not show Created for empty CreatedAt")
	}
}

// TestFormatMarkdownString_WithCreatedAt verifies FormatMarkdownString when with created at.
func TestFormatMarkdownString_WithCreatedAt(t *testing.T) {
	md := FormatMarkdownString(Output{Name: "thumbsup", Username: "alice", CreatedAt: "2026-06-01T10:00:00Z"})
	if !strings.Contains(md, "Created") || !strings.Contains(md, "1 Jun 2026") {
		t.Error("expected Created date")
	}
}

// ======================== Action Specs ========================.

// TestActionSpecs_Metadata verifies canonical metadata for all award emoji actions.
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
	}
	if byTool["gitlab_issue_emoji_list"].ParameterGuidance["issue_iid"].SemanticRole == "" {
		t.Fatal("gitlab_issue_emoji_list should expose issue_iid parameter guidance")
	}
}

// TestActionSpecs_CallAllRoutes validates all award emoji routes through canonical specs.
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

// TestActionSpecs_GetNotFound validates get routes preserve NotFoundResult details.
func TestActionSpecs_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	client := testutil.NewTestClient(t, mux)
	byTool := awardEmojiSpecsByTool(t, allAwardEmojiActionSpecs(client))

	getTools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_emoji_get", map[string]any{"project_id": "p", "issue_iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_get", map[string]any{"project_id": "p", "issue_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_get", map[string]any{"project_id": "p", "merge_request_iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_get", map[string]any{"project_id": "p", "merge_request_iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_get", map[string]any{"project_id": "p", "snippet_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_get", map[string]any{"project_id": "p", "snippet_id": 1, "note_id": 1, "award_id": 1}},
	}
	for _, tc := range getTools {
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

// TestCreateAPIErrors covers the API error return paths in all five create
// functions that lack API-error coverage.
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

// TestActionSpecs_CreateErrors validates create routes return API errors.
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

// TestActionSpecs_DeleteErrors validates delete routes return API errors.
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

// TestDeleteAwardEmoji_NotFoundHints covers delete not-found hint branches.
func TestDeleteAwardEmoji_NotFoundHints(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{"DeleteIssueAwardEmoji", func(ctx context.Context) error {
			return DeleteIssueAwardEmoji(ctx, client, IssueDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		}},
		{"DeleteIssueNoteAwardEmoji", func(ctx context.Context) error {
			return DeleteIssueNoteAwardEmoji(ctx, client, IssueDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		}},
		{"DeleteMRAwardEmoji", func(ctx context.Context) error {
			return DeleteMRAwardEmoji(ctx, client, MRDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		}},
		{"DeleteMRNoteAwardEmoji", func(ctx context.Context) error {
			return DeleteMRNoteAwardEmoji(ctx, client, MRDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		}},
		{"DeleteSnippetAwardEmoji", func(ctx context.Context) error {
			return DeleteSnippetAwardEmoji(ctx, client, SnippetDeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
		}},
		{"DeleteSnippetNoteAwardEmoji", func(ctx context.Context) error {
			return DeleteSnippetNoteAwardEmoji(ctx, client, SnippetDeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
		}},
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
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
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
			name: "pagination terminates with resp.NextPage == 0",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v4/user":
					testutil.RespondJSON(w, http.StatusOK, `{"id":9,"username":"current"}`)
				case mrEmojiPath:
					// Single-page response with NextPage=0.
					testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"other","user":{"id":1,"username":"u"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "100", Total: "1"})
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			},
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
