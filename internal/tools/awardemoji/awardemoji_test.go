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

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpPath = "unexpected path: %s"

const errNoReachAPI = "should not reach API"

const fmtUnexpErr = "unexpected error: %v"

const testProjectID = "my-project"

const (
	testEmojiThumbsup     = "thumbsup"
	testEmojiStar         = "star"
	fmtExpected1Emoji     = "expected 1 emoji, got %d"
	testFieldIID          = "iid"
	testFieldAwardID      = "award_id"
	testFieldNoteID       = "note_id"
	testPathAPIProjects   = "/api/v4/projects/"
	fmtNameWantThumbsup   = "name = %q, want thumbsup"
	testErrEmptyProjectID = "expected error for empty project_id"
)

// Issue award emoji tests.

// TestListIssueAwardEmoji_Success verifies the behavior of list issue award emoji success.
func TestListIssueAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":10,"name":"thumbsup","user":{"id":1,"username":"admin"},"created_at":"2026-01-01T00:00:00Z","awardable_id":1,"awardable_type":"Issue"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListIssueAwardEmoji(t.Context(), client, ListInput{
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

// TestListIssueAwardEmoji_ValidationError verifies the behavior of list issue award emoji validation error.
func TestListIssueAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := ListIssueAwardEmoji(t.Context(), client, ListInput{
		ProjectID: "",
		IID:       1,
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestGetIssueAwardEmoji_Success verifies the behavior of get issue award emoji success.
func TestGetIssueAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/award_emoji/10" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"thumbsup","user":{"id":1,"username":"admin"},"created_at":"2026-01-01T00:00:00Z","awardable_id":1,"awardable_type":"Issue"}`)
	}))

	out, err := GetIssueAwardEmoji(t.Context(), client, GetInput{
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

// TestCreateIssueAwardEmoji_Success verifies the behavior of create issue award emoji success.
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

	out, err := CreateIssueAwardEmoji(t.Context(), client, CreateInput{
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

// TestCreateIssueAwardEmoji_ValidationError verifies the behavior of create issue award emoji validation error.
func TestCreateIssueAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := CreateIssueAwardEmoji(t.Context(), client, CreateInput{
		ProjectID: "",
		IID:       1,
		Name:      testEmojiThumbsup,
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// TestDeleteIssueAwardEmoji_Success verifies the behavior of delete issue award emoji success.
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

	err := DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{
		ProjectID: testProjectID,
		IID:       1,
		AwardID:   10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteIssueAwardEmoji_APIError verifies the behavior of delete issue award emoji a p i error.
func TestDeleteIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{
		ProjectID: testProjectID,
		IID:       1,
		AwardID:   10,
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// Issue note award emoji tests.

// TestListIssueNoteAwardEmoji_Success verifies the behavior of list issue note award emoji success.
func TestListIssueNoteAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/notes/5/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":20,"name":"heart","user":{"id":2,"username":"dev"},"created_at":"2026-02-01T00:00:00Z","awardable_id":5,"awardable_type":"Note"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListIssueNoteAwardEmoji(t.Context(), client, ListOnNoteInput{
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

// TestDeleteIssueNoteAwardEmoji_Success verifies the behavior of delete issue note award emoji success.
func TestDeleteIssueNoteAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/issues/1/notes/5/award_emoji/20" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := DeleteIssueNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{
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

// TestListMRAwardEmoji_Success verifies the behavior of list m r award emoji success.
func TestListMRAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/merge_requests/3/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":30,"name":"rocket","user":{"id":3,"username":"user3"},"created_at":"2026-03-01T00:00:00Z","awardable_id":3,"awardable_type":"MergeRequest"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListMRAwardEmoji(t.Context(), client, ListInput{
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

// TestCreateMRAwardEmoji_ValidationError verifies the behavior of create m r award emoji validation error.
func TestCreateMRAwardEmoji_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := CreateMRAwardEmoji(t.Context(), client, CreateInput{
		ProjectID: "",
		IID:       3,
		Name:      "rocket",
	})
	if err == nil {
		t.Fatal(testErrEmptyProjectID)
	}
}

// Snippet award emoji tests.

// TestListSnippetAwardEmoji_Success verifies the behavior of list snippet award emoji success.
func TestListSnippetAwardEmoji_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPathAPIProjects+testProjectID+"/snippets/7/award_emoji" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":40,"name":"100","user":{"id":4,"username":"user4"},"created_at":"2026-04-01T00:00:00Z","awardable_id":7,"awardable_type":"Snippet"}]`, testutil.PaginationHeaders{Page: "1", TotalPages: "1", PerPage: "20", Total: "1"})
	}))

	out, err := ListSnippetAwardEmoji(t.Context(), client, ListInput{
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

// TestFormatListMarkdownString_WithEmoji verifies the behavior of format list markdown string with emoji.
func TestFormatListMarkdownString_WithEmoji(t *testing.T) {
	out := ListOutput{
		AwardEmoji: []Output{
			{ID: 10, Name: testEmojiThumbsup, UserID: 1, Username: "admin", CreatedAt: "2026-01-01T00:00:00Z", AwardableID: 1, AwardableType: "Issue"},
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
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{}}
	md := FormatListMarkdownString(out)
	if md != "No award emoji found.\n" {
		t.Errorf("expected empty message, got: %s", md)
	}
}

// TestFormatMarkdownString verifies the behavior of format markdown string.
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
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{}}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// contains is an internal helper for the awardemoji package.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && containsHelper(s, substr))
}

// containsHelper is an internal helper for the awardemoji package.
func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Int64 validation tests.

// assertErrContains is an internal helper for the awardemoji package.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestListIssueAwardEmoji_InvalidIID verifies the behavior of list issue award emoji invalid i i d.
func TestListIssueAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldIID)
	_, err = ListIssueAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: -1})
	assertErrContains(t, err, testFieldIID)
}

// TestGetIssueAwardEmoji_InvalidIDs verifies the behavior of get issue award emoji invalid i ds.
func TestGetIssueAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = GetIssueAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateIssueAwardEmoji_InvalidIID verifies the behavior of create issue award emoji invalid i i d.
func TestCreateIssueAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateIssueAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIID)
}

// TestDeleteIssueAwardEmoji_InvalidIDs verifies the behavior of delete issue award emoji invalid i ds.
func TestDeleteIssueAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	err = DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListIssueNoteAwardEmoji_InvalidIDs verifies the behavior of list issue note award emoji invalid i ds.
func TestListIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = ListIssueNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetIssueNoteAwardEmoji_InvalidIDs verifies the behavior of get issue note award emoji invalid i ds.
func TestGetIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = GetIssueNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetIssueNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateIssueNoteAwardEmoji_InvalidIDs verifies the behavior of create issue note award emoji invalid i ds.
func TestCreateIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIID)
	_, err = CreateIssueNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteIssueNoteAwardEmoji_InvalidIDs verifies the behavior of delete issue note award emoji invalid i ds.
func TestDeleteIssueNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	err = DeleteIssueNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteIssueNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListMRAwardEmoji_InvalidIID verifies the behavior of list m r award emoji invalid i i d.
func TestListMRAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldIID)
}

// TestGetMRAwardEmoji_InvalidIDs verifies the behavior of get m r award emoji invalid i ds.
func TestGetMRAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = GetMRAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateMRAwardEmoji_InvalidIID verifies the behavior of create m r award emoji invalid i i d.
func TestCreateMRAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateMRAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: -5, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIID)
}

// TestDeleteMRAwardEmoji_InvalidIDs verifies the behavior of delete m r award emoji invalid i ds.
func TestDeleteMRAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteMRAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	err = DeleteMRAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: -1})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListMRNoteAwardEmoji_InvalidIDs verifies the behavior of list m r note award emoji invalid i ds.
func TestListMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = ListMRNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetMRNoteAwardEmoji_InvalidIDs verifies the behavior of get m r note award emoji invalid i ds.
func TestGetMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = GetMRNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetMRNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateMRNoteAwardEmoji_InvalidIDs verifies the behavior of create m r note award emoji invalid i ds.
func TestCreateMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIID)
	_, err = CreateMRNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteMRNoteAwardEmoji_InvalidIDs verifies the behavior of delete m r note award emoji invalid i ds.
func TestDeleteMRNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteMRNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	err = DeleteMRNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteMRNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListSnippetAwardEmoji_InvalidIID verifies the behavior of list snippet award emoji invalid i i d.
func TestListSnippetAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListSnippetAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 0})
	assertErrContains(t, err, testFieldIID)
}

// TestGetSnippetAwardEmoji_InvalidIDs verifies the behavior of get snippet award emoji invalid i ds.
func TestGetSnippetAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetSnippetAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = GetSnippetAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateSnippetAwardEmoji_InvalidIID verifies the behavior of create snippet award emoji invalid i i d.
func TestCreateSnippetAwardEmoji_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateSnippetAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIID)
}

// TestDeleteSnippetAwardEmoji_InvalidIDs verifies the behavior of delete snippet award emoji invalid i ds.
func TestDeleteSnippetAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteSnippetAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	err = DeleteSnippetAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestListSnippetNoteAwardEmoji_InvalidIDs verifies the behavior of list snippet note award emoji invalid i ds.
func TestListSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = ListSnippetNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0})
	assertErrContains(t, err, testFieldNoteID)
}

// TestGetSnippetNoteAwardEmoji_InvalidIDs verifies the behavior of get snippet note award emoji invalid i ds.
func TestGetSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	_, err = GetSnippetNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	_, err = GetSnippetNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// TestCreateSnippetNoteAwardEmoji_InvalidIDs verifies the behavior of create snippet note award emoji invalid i ds.
func TestCreateSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, Name: testEmojiStar})
	assertErrContains(t, err, testFieldIID)
	_, err = CreateSnippetNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, Name: testEmojiStar})
	assertErrContains(t, err, testFieldNoteID)
}

// TestDeleteSnippetNoteAwardEmoji_InvalidIDs verifies the behavior of delete snippet note award emoji invalid i ds.
func TestDeleteSnippetNoteAwardEmoji_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 0, NoteID: 1, AwardID: 1})
	assertErrContains(t, err, testFieldIID)
	err = DeleteSnippetNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 0, AwardID: 1})
	assertErrContains(t, err, testFieldNoteID)
	err = DeleteSnippetNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 0})
	assertErrContains(t, err, testFieldAwardID)
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedValidation = "expected validation error"

const covEmojiJSON = `[{"id":1,"name":"thumbsup","user":{"id":5,"username":"alice"},"created_at":"2026-06-01T10:00:00Z","awardable_id":10,"awardable_type":"Issue"}]`
const covEmojiSingle = `{"id":1,"name":"thumbsup","user":{"id":5,"username":"alice"},"created_at":"2026-06-01T10:00:00Z","awardable_id":10,"awardable_type":"Issue"}`

// covBadHandler is an internal helper for the awardemoji package.
func covBadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	})
}

// covOKList is an internal helper for the awardemoji package.
func covOKList() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covEmojiJSON)
	})
}

// covOKSingle is an internal helper for the awardemoji package.
func covOKSingle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covEmojiSingle)
	})
}

// covOKDelete is an internal helper for the awardemoji package.
func covOKDelete() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// ======================== Issue Emoji ========================.

// TestListIssueAwardEmoji_Validation verifies the behavior of cov list issue award emoji validation.
func TestListIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueAwardEmoji(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueAwardEmoji_APIError verifies the behavior of cov list issue award emoji a p i error.
func TestListIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueAwardEmoji_Success_Cov verifies the behavior of cov list issue award emoji success.
func TestListIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKList())
	out, err := ListIssueAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.AwardEmoji) != 1 || out.AwardEmoji[0].Name != "thumbsup" {
		t.Errorf("unexpected: %+v", out)
	}
}

// TestGetIssueAwardEmoji_Validation verifies the behavior of cov get issue award emoji validation.
func TestGetIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueAwardEmoji(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueAwardEmoji_APIError verifies the behavior of cov get issue award emoji a p i error.
func TestGetIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueAwardEmoji_Success_Cov verifies the behavior of cov get issue award emoji success.
func TestGetIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKSingle())
	out, err := GetIssueAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Name != "thumbsup" {
		t.Error("unexpected name")
	}
}

// TestCreateIssueAwardEmoji_Validation verifies the behavior of cov create issue award emoji validation.
func TestCreateIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueAwardEmoji(t.Context(), client, CreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateIssueAwardEmoji_APIError verifies the behavior of cov create issue award emoji a p i error.
func TestCreateIssueAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: 1, Name: "thumbsup"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateIssueAwardEmoji_Success_Cov verifies the behavior of cov create issue award emoji success.
func TestCreateIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKSingle())
	out, err := CreateIssueAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: 1, Name: "thumbsup"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ID != 1 {
		t.Error("unexpected ID")
	}
}

// TestDeleteIssueAwardEmoji_Validation verifies the behavior of cov delete issue award emoji validation.
func TestDeleteIssueAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteIssueAwardEmoji_APIError_Cov verifies the behavior of cov delete issue award emoji API error.
func TestDeleteIssueAwardEmoji_APIError_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteIssueAwardEmoji_Success_Cov verifies the behavior of cov delete issue award emoji success.
func TestDeleteIssueAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKDelete())
	err := DeleteIssueAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

// ======================== Issue Note Emoji ========================.

// TestListIssueNoteAwardEmoji_Validation verifies the behavior of cov list issue note award emoji validation.
func TestListIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, ListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueNoteAwardEmoji_APIError verifies the behavior of cov list issue note award emoji a p i error.
func TestListIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueNoteAwardEmoji_Success_Cov verifies the behavior of cov list issue note award emoji success.
func TestListIssueNoteAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKList())
	out, err := ListIssueNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Error("expected 1 emoji")
	}
}

// TestGetIssueNoteAwardEmoji_Validation verifies the behavior of cov get issue note award emoji validation.
func TestGetIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, GetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueNoteAwardEmoji_APIError verifies the behavior of cov get issue note award emoji a p i error.
func TestGetIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateIssueNoteAwardEmoji_Validation verifies the behavior of cov create issue note award emoji validation.
func TestCreateIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateIssueNoteAwardEmoji_APIError verifies the behavior of cov create issue note award emoji a p i error.
func TestCreateIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateIssueNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteIssueNoteAwardEmoji_Validation verifies the behavior of cov delete issue note award emoji validation.
func TestDeleteIssueNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteIssueNoteAwardEmoji_APIError verifies the behavior of cov delete issue note award emoji a p i error.
func TestDeleteIssueNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteIssueNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== MR Emoji ========================.

// TestListMRAwardEmoji_Validation verifies the behavior of cov list m r award emoji validation.
func TestListMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRAwardEmoji(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRAwardEmoji_APIError verifies the behavior of cov list m r award emoji a p i error.
func TestListMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRAwardEmoji_Success_Cov verifies the behavior of cov list MR award emoji success.
func TestListMRAwardEmoji_Success_Cov(t *testing.T) {
	client := testutil.NewTestClient(t, covOKList())
	out, err := ListMRAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.AwardEmoji) != 1 {
		t.Error("expected 1 emoji")
	}
}

// TestGetMRAwardEmoji_Validation verifies the behavior of cov get m r award emoji validation.
func TestGetMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRAwardEmoji(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRAwardEmoji_APIError verifies the behavior of cov get m r award emoji a p i error.
func TestGetMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateMRAwardEmoji_Validation verifies the behavior of cov create m r award emoji validation.
func TestCreateMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRAwardEmoji(t.Context(), client, CreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateMRAwardEmoji_APIError verifies the behavior of cov create m r award emoji a p i error.
func TestCreateMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteMRAwardEmoji_Validation verifies the behavior of cov delete m r award emoji validation.
func TestDeleteMRAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRAwardEmoji(t.Context(), client, DeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteMRAwardEmoji_APIError verifies the behavior of cov delete m r award emoji a p i error.
func TestDeleteMRAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== MR Note Emoji ========================.

// TestListMRNoteAwardEmoji_Validation verifies the behavior of cov list m r note award emoji validation.
func TestListMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRNoteAwardEmoji(t.Context(), client, ListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRNoteAwardEmoji_APIError verifies the behavior of cov list m r note award emoji a p i error.
func TestListMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRNoteAwardEmoji_Validation verifies the behavior of cov get m r note award emoji validation.
func TestGetMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRNoteAwardEmoji(t.Context(), client, GetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRNoteAwardEmoji_APIError verifies the behavior of cov get m r note award emoji a p i error.
func TestGetMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateMRNoteAwardEmoji_Validation verifies the behavior of cov create m r note award emoji validation.
func TestCreateMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateMRNoteAwardEmoji_APIError verifies the behavior of cov create m r note award emoji a p i error.
func TestCreateMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateMRNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteMRNoteAwardEmoji_Validation verifies the behavior of cov delete m r note award emoji validation.
func TestDeleteMRNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteMRNoteAwardEmoji_APIError verifies the behavior of cov delete m r note award emoji a p i error.
func TestDeleteMRNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteMRNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Snippet Emoji ========================.

// TestListSnippetAwardEmoji_Validation verifies the behavior of cov list snippet award emoji validation.
func TestListSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetAwardEmoji(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListSnippetAwardEmoji_APIError verifies the behavior of cov list snippet award emoji a p i error.
func TestListSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetAwardEmoji(t.Context(), client, ListInput{ProjectID: "p", IID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetSnippetAwardEmoji_Validation verifies the behavior of cov get snippet award emoji validation.
func TestGetSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetAwardEmoji(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetSnippetAwardEmoji_APIError verifies the behavior of cov get snippet award emoji a p i error.
func TestGetSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetAwardEmoji(t.Context(), client, GetInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateSnippetAwardEmoji_Validation verifies the behavior of cov create snippet award emoji validation.
func TestCreateSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetAwardEmoji(t.Context(), client, CreateInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateSnippetAwardEmoji_APIError verifies the behavior of cov create snippet award emoji a p i error.
func TestCreateSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetAwardEmoji(t.Context(), client, CreateInput{ProjectID: "p", IID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteSnippetAwardEmoji_Validation verifies the behavior of cov delete snippet award emoji validation.
func TestDeleteSnippetAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetAwardEmoji(t.Context(), client, DeleteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteSnippetAwardEmoji_APIError verifies the behavior of cov delete snippet award emoji a p i error.
func TestDeleteSnippetAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetAwardEmoji(t.Context(), client, DeleteInput{ProjectID: "p", IID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Snippet Note Emoji ========================.

// TestListSnippetNoteAwardEmoji_Validation verifies the behavior of cov list snippet note award emoji validation.
func TestListSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, ListOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListSnippetNoteAwardEmoji_APIError verifies the behavior of cov list snippet note award emoji a p i error.
func TestListSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListSnippetNoteAwardEmoji(t.Context(), client, ListOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetSnippetNoteAwardEmoji_Validation verifies the behavior of cov get snippet note award emoji validation.
func TestGetSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, GetOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetSnippetNoteAwardEmoji_APIError verifies the behavior of cov get snippet note award emoji a p i error.
func TestGetSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetSnippetNoteAwardEmoji(t.Context(), client, GetOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestCreateSnippetNoteAwardEmoji_Validation verifies the behavior of cov create snippet note award emoji validation.
func TestCreateSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestCreateSnippetNoteAwardEmoji_APIError verifies the behavior of cov create snippet note award emoji a p i error.
func TestCreateSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := CreateSnippetNoteAwardEmoji(t.Context(), client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "x"})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestDeleteSnippetNoteAwardEmoji_Validation verifies the behavior of cov delete snippet note award emoji validation.
func TestDeleteSnippetNoteAwardEmoji_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteSnippetNoteAwardEmoji_APIError verifies the behavior of cov delete snippet note award emoji a p i error.
func TestDeleteSnippetNoteAwardEmoji_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	err := DeleteSnippetNoteAwardEmoji(t.Context(), client, DeleteOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, AwardID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// ======================== Formatters ========================.

// TestFormatListMarkdown_Empty_Cov verifies the behavior of cov format list markdown empty.
func TestFormatListMarkdown_Empty_Cov(t *testing.T) {
	res := FormatListMarkdown(ListOutput{})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdownString_Empty_Cov verifies the behavior of cov format list markdown string empty.
func TestFormatListMarkdownString_Empty_Cov(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No award emoji found") {
		t.Error("expected empty message")
	}
}

// TestFormatListMarkdownString_WithEmoji_Cov verifies the behavior of cov format list markdown string with emoji.
func TestFormatListMarkdownString_WithEmoji_Cov(t *testing.T) {
	out := ListOutput{AwardEmoji: []Output{{ID: 1, Name: "thumbsup", Username: "alice"}}}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "thumbsup") || !strings.Contains(md, "alice") {
		t.Error("expected emoji details")
	}
}

// TestFormatMarkdown_Wrapper verifies the behavior of cov format markdown wrapper.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	res := FormatMarkdown(Output{Name: "thumbsup"})
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatMarkdownString_NoCreatedAt verifies the behavior of cov format markdown string no created at.
func TestFormatMarkdownString_NoCreatedAt(t *testing.T) {
	md := FormatMarkdownString(Output{Name: "thumbsup", Username: "alice"})
	if strings.Contains(md, "Created") {
		t.Error("should not show Created for empty CreatedAt")
	}
}

// TestFormatMarkdownString_WithCreatedAt verifies the behavior of cov format markdown string with created at.
func TestFormatMarkdownString_WithCreatedAt(t *testing.T) {
	md := FormatMarkdownString(Output{Name: "thumbsup", Username: "alice", CreatedAt: "2026-06-01T10:00:00Z"})
	if !strings.Contains(md, "Created") || !strings.Contains(md, "1 Jun 2026") {
		t.Error("expected Created date")
	}
}

// ======================== Register ========================.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, covBadHandler())
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, covBadHandler())
	RegisterMeta(server, client)
}

// ======================== MCP Round-trip ========================.

// TestMCPRound_Trip validates cov m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
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

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_emoji_list", map[string]any{"project_id": "p", "iid": 1}},
		{"gitlab_issue_emoji_get", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_issue_emoji_create", map[string]any{"project_id": "p", "iid": 1, "name": "thumbsup"}},
		{"gitlab_issue_emoji_delete", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_list", map[string]any{"project_id": "p", "iid": 1, "note_id": 1}},
		{"gitlab_issue_note_emoji_get", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_create", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_issue_note_emoji_delete", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_list", map[string]any{"project_id": "p", "iid": 1}},
		{"gitlab_mr_emoji_get", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_mr_emoji_create", map[string]any{"project_id": "p", "iid": 1, "name": "thumbsup"}},
		{"gitlab_mr_emoji_delete", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_list", map[string]any{"project_id": "p", "iid": 1, "note_id": 1}},
		{"gitlab_mr_note_emoji_get", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_create", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_mr_note_emoji_delete", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_list", map[string]any{"project_id": "p", "iid": 1}},
		{"gitlab_snippet_emoji_get", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_create", map[string]any{"project_id": "p", "iid": 1, "name": "thumbsup"}},
		{"gitlab_snippet_emoji_delete", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_list", map[string]any{"project_id": "p", "iid": 1, "note_id": 1}},
		{"gitlab_snippet_note_emoji_get", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_create", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_snippet_note_emoji_delete", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var res *mcp.CallToolResult
			res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
		})
	}
}

// TestMCPRound_Trip_NotFound validates that get tools return NotFoundResult
// when the GitLab API responds with 404, covering the register.go 404 paths.
func TestMCPRound_Trip_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	getTools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_emoji_get", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_issue_note_emoji_get", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_mr_emoji_get", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_mr_note_emoji_get", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
		{"gitlab_snippet_emoji_get", map[string]any{"project_id": "p", "iid": 1, "award_id": 1}},
		{"gitlab_snippet_note_emoji_get", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "award_id": 1}},
	}
	for _, tc := range getTools {
		t.Run(tc.name+"_404", func(t *testing.T) {
			res, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if callErr != nil {
				t.Fatalf("CallTool %s: %v", tc.name, callErr)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
			if !res.IsError {
				t.Errorf("expected IsError=true for 404 on %s", tc.name)
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
		_, err := CreateIssueNoteAwardEmoji(ctx, client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateMRAwardEmoji", func(t *testing.T) {
		_, err := CreateMRAwardEmoji(ctx, client, CreateInput{ProjectID: "p", IID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateMRNoteAwardEmoji", func(t *testing.T) {
		_, err := CreateMRNoteAwardEmoji(ctx, client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateSnippetAwardEmoji", func(t *testing.T) {
		_, err := CreateSnippetAwardEmoji(ctx, client, CreateInput{ProjectID: "p", IID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("CreateSnippetNoteAwardEmoji", func(t *testing.T) {
		_, err := CreateSnippetNoteAwardEmoji(ctx, client, CreateOnNoteInput{ProjectID: "p", IID: 1, NoteID: 1, Name: "star"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestMCPRound_Trip_CreateErrors validates the register.go create error paths
// via MCP round-trip where the GitLab API returns 500.
func TestMCPRound_Trip_CreateErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covEmojiJSON)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	createTools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_emoji_create", map[string]any{"project_id": "p", "iid": 1, "name": "thumbsup"}},
		{"gitlab_issue_note_emoji_create", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_mr_emoji_create", map[string]any{"project_id": "p", "iid": 1, "name": "thumbsup"}},
		{"gitlab_mr_note_emoji_create", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "name": "thumbsup"}},
		{"gitlab_snippet_emoji_create", map[string]any{"project_id": "p", "iid": 1, "name": "thumbsup"}},
		{"gitlab_snippet_note_emoji_create", map[string]any{"project_id": "p", "iid": 1, "note_id": 1, "name": "thumbsup"}},
	}
	for _, tc := range createTools {
		t.Run(tc.name, func(t *testing.T) {
			res, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if callErr != nil {
				t.Fatalf("CallTool %s: %v", tc.name, callErr)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
			if !res.IsError {
				t.Errorf("expected IsError=true for create error on %s", tc.name)
			}
		})
	}
}
