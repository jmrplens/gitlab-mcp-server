// optional_params_test.go contains unit tests that exercise optional parameter
// branches in tool handlers. Each test provides all optional fields to ensure
// full code path coverage for parameter handling logic.
package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	testNewName     = "new-name"
	testCustomEmail = "custom@example.com"
)

// TestMRCreate_AllOptionalParams exercises every optional branch in mrCreate.
func TestMRCreate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"dev","target_branch":"main","web_url":"https://example.com/mr/1","author":{"username":"test"},"description":"d","detailed_merge_status":"mergeable","has_conflicts":false,"changes_count":"1"}`)
	}))

	out, err := mergerequests.Create(context.Background(), client, mergerequests.CreateInput{
		ProjectID:          "42",
		SourceBranch:       "dev",
		TargetBranch:       "main",
		Title:              "feat: test",
		Description:        "A description",
		AssigneeIDs:        []int64{1, 2},
		ReviewerIDs:        []int64{3, 4},
		RemoveSourceBranch: new(true),
		Squash:             new(true),
	})
	if err != nil {
		t.Fatalf("mergerequests.Create() unexpected error: %v", err)
	}
	if out.IID != 1 {
		t.Errorf("IID = %d, want 1", out.IID)
	}
}

// TestMRUpdate_AllOptionalParams exercises every optional branch in mrUpdate.
func TestMRUpdate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"updated","state":"opened","source_branch":"dev","target_branch":"release","web_url":"https://example.com/mr/1","author":{"username":"test"},"description":"new desc","detailed_merge_status":"mergeable","has_conflicts":false,"changes_count":"1"}`)
	}))

	out, err := mergerequests.Update(context.Background(), client, mergerequests.UpdateInput{
		ProjectID:    "42",
		MRIID:        1,
		Title:        "updated",
		Description:  "new desc",
		TargetBranch: "release",
		StateEvent:   "close",
		AssigneeIDs:  []int64{5},
		ReviewerIDs:  []int64{6, 7},
	})
	if err != nil {
		t.Fatalf("mergerequests.Update() unexpected error: %v", err)
	}
	if out.Title != "updated" {
		t.Errorf("Title = %q, want %q", out.Title, "updated")
	}
}

// TestMRMerge_AllOptionalParams exercises every optional branch in mrMerge.
func TestMRMerge_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","state":"merged","source_branch":"dev","target_branch":"main","web_url":"https://example.com/mr/1","author":{"username":"test"},"detailed_merge_status":"merged","has_conflicts":false,"changes_count":"1"}`)
	}))

	out, err := mergerequests.Merge(context.Background(), client, mergerequests.MergeInput{
		ProjectID:                "42",
		MRIID:                    1,
		MergeCommitMessage:       "custom msg",
		Squash:                   new(true),
		ShouldRemoveSourceBranch: new(true),
	})
	if err != nil {
		t.Fatalf("mergerequests.Merge() unexpected error: %v", err)
	}
	if out.State != "merged" {
		t.Errorf("State = %q, want %q", out.State, "merged")
	}
}

// TestMRList_AllOptionalFilters exercises every optional branch in mrList.
func TestMRList_AllOptionalFilters(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") == "" || q.Get("search") == "" || q.Get("order_by") == "" || q.Get("sort") == "" {
			t.Error("expected all optional query params to be set")
		}
		respondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"dev","target_branch":"main","web_url":"https://example.com","author":{"username":"test"}}]`)
	}))

	out, err := mergerequests.List(context.Background(), client, mergerequests.ListInput{
		ProjectID: "42",
		State:     "opened",
		Search:    "feat",
		OrderBy:   "created_at",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf("mergerequests.List() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 1 {
		t.Errorf("len(MergeRequests) = %d, want 1", len(out.MergeRequests))
	}
}

// TestProjectCreate_AllOptionalParams exercises every optional branch in projectCreate.
func TestProjectCreate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":42,"name":"proj","path_with_namespace":"ns/proj","visibility":"internal","web_url":"https://example.com/ns/proj","description":"desc","default_branch":"develop","namespace":{"id":10,"name":"ns","path":"ns","full_path":"ns"}}`)
	}))

	out, err := projects.Create(context.Background(), client, projects.CreateInput{
		Name:                 "proj",
		NamespaceID:          10,
		Description:          "desc",
		Visibility:           "internal",
		InitializeWithReadme: true,
		DefaultBranch:        "develop",
	})
	if err != nil {
		t.Fatalf("projectCreate() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// TestProjectUpdate_AllOptionalParams exercises every optional branch in projectUpdate.
func TestProjectUpdate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"id":42,"name":"new-name","path_with_namespace":"ns/proj","visibility":"public","web_url":"https://example.com/ns/proj","description":"new desc","default_branch":"develop","namespace":{"id":1,"name":"ns","path":"ns","full_path":"ns"}}`)
	}))

	out, err := projects.Update(context.Background(), client, projects.UpdateInput{
		ProjectID:     "42",
		Name:          testNewName,
		Description:   "new desc",
		Visibility:    "public",
		DefaultBranch: "develop",
		MergeMethod:   "rebase_merge",
	})
	if err != nil {
		t.Fatalf("projectUpdate() unexpected error: %v", err)
	}
	if out.Name != testNewName {
		t.Errorf("Name = %q, want %q", out.Name, testNewName)
	}
}

// TestProjectList_AllOptionalFilters exercises every optional branch in projectList.
func TestProjectList_AllOptionalFilters(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":42,"name":"test","path_with_namespace":"ns/test","visibility":"private","web_url":"https://example.com","description":"","default_branch":"main","namespace":{"id":1,"name":"ns","path":"ns","full_path":"ns"}}]`)
	}))

	out, err := projects.List(context.Background(), client, projects.ListInput{
		Owned:      true,
		Search:     "test",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("projectList() unexpected error: %v", err)
	}
	if len(out.Projects) != 1 {
		t.Errorf("len(Projects) = %d, want 1", len(out.Projects))
	}
}

// TestBranchList_WithSearchParam exercises the search and pagination branches in branchList.
func TestBranchList_WithSearchParam(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") == "" {
			t.Error("expected search query param")
		}
		respondJSON(w, http.StatusOK, `[{"name":"feature/auth","merged":false,"protected":false,"default":false,"web_url":"https://example.com","commit":{"id":"abc123"}}]`)
	}))

	out, err := branches.List(context.Background(), client, branches.ListInput{
		ProjectID:       "42",
		Search:          "feature",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("branchList() unexpected error: %v", err)
	}
	if len(out.Branches) != 1 {
		t.Errorf("len(Branches) = %d, want 1", len(out.Branches))
	}
}

// TestTagList_AllOptionalParams exercises every optional branch in tagList.
func TestTagList_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"name":"v1.0","message":"","target":"abc","commit":{"id":"abc","short_id":"ab","title":"init","message":"init","author_name":"test"}}]`)
	}))

	out, err := tags.List(context.Background(), client, tags.ListInput{
		ProjectID: "42",
		Search:    "v1",
		OrderBy:   "name",
		Sort:      "asc",
	})
	if err != nil {
		t.Fatalf("tags.List() unexpected error: %v", err)
	}
	if len(out.Tags) != 1 {
		t.Errorf("len(Tags) = %d, want 1", len(out.Tags))
	}
}

// TestReleaseList_AllOptionalParams exercises every optional branch in releaseList.
func TestReleaseList_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"tag_name":"v1.0","name":"v1.0","description":"notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"test"},"commit":{"id":"abc123"},"assets":{"links":[]}}]`)
	}))

	out, err := releases.List(context.Background(), client, releases.ListInput{
		ProjectID: "42",
		OrderBy:   "released_at",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf("releaseList() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Errorf("len(Releases) = %d, want 1", len(out.Releases))
	}
}

// TestReleaseUpdate_AllOptionalParams exercises every optional branch in releaseUpdate.
func TestReleaseUpdate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"tag_name":"v1.0","name":"Updated","description":"new notes","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"test"},"commit":{"id":"abc123"},"assets":{"links":[]}}`)
	}))

	out, err := releases.Update(context.Background(), client, releases.UpdateInput{
		ProjectID:   "42",
		TagName:     "v1.0",
		Name:        "Updated",
		Description: "new notes",
	})
	if err != nil {
		t.Fatalf("releaseUpdate() unexpected error: %v", err)
	}
	if out.Name != "Updated" {
		t.Errorf("Name = %q, want %q", out.Name, "Updated")
	}
}

// TestMRNotesList_AllOptionalParams exercises optional branches in mrNotesList.
func TestMRNotesList_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[{"id":1,"body":"note","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","system":false}]`)
	}))

	out, err := mrnotes.List(context.Background(), client, mrnotes.ListInput{
		ProjectID: "42",
		MRIID:     1,
		OrderBy:   "updated_at",
		Sort:      "asc",
	})
	if err != nil {
		t.Fatalf("mrnotes.List() unexpected error: %v", err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

// TestMRDiscussionCreate_InlineWithOldPath exercises the OldPath and OldLine branches.
func TestMRDiscussionCreateInline_WithOldPath(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":"disc1","individual_note":false,"notes":[{"id":1,"body":"inline","author":{"username":"test"},"created_at":"2026-01-01T00:00:00Z","resolved":false}]}`)
	}))

	out, err := mrdiscussions.Create(context.Background(), client, mrdiscussions.CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Body:      "inline note on old line",
		Position: &mrdiscussions.DiffPosition{
			BaseSHA:  "base",
			StartSHA: "start",
			HeadSHA:  "head",
			OldPath:  "old_file.go",
			NewPath:  "new_file.go",
			OldLine:  10,
			NewLine:  15,
		},
	})
	if err != nil {
		t.Fatalf("mrdiscussions.Create() unexpected error: %v", err)
	}
	if out.ID != "disc1" {
		t.Errorf("ID = %q, want %q", out.ID, "disc1")
	}
}

// TestCommitCreate_AllOptionalParams exercises optional branches in commitCreate.
func TestCommitCreate_AllOptionalParams(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":"abc123","short_id":"abc1","title":"custom commit","message":"custom commit","author_name":"Custom Author","author_email":"custom@example.com","created_at":"2026-01-01T00:00:00Z","web_url":"https://example.com/c/abc123","stats":{"additions":1,"deletions":0,"total":1}}`)
	}))

	out, err := commits.Create(context.Background(), client, commits.CreateInput{
		ProjectID:     "42",
		Branch:        "main",
		CommitMessage: "custom commit",
		StartBranch:   "develop",
		AuthorEmail:   testCustomEmail,
		AuthorName:    "Custom Author",
		Actions: []commits.Action{
			{Action: "create", FilePath: "new.txt", Content: "hello"},
			{Action: "move", FilePath: "moved.txt", PreviousPath: "old.txt"},
		},
	})
	if err != nil {
		t.Fatalf("commits.Create() unexpected error: %v", err)
	}
	if out.AuthorEmail != testCustomEmail {
		t.Errorf("AuthorEmail = %q, want %q", out.AuthorEmail, testCustomEmail)
	}
}

// TestFileGet_WithRef exercises the Ref branch in fileGet.
func TestFileGet_WithRef(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"file_name":"README.md","file_path":"README.md","size":5,"encoding":"base64","content":"SGVsbG8=","ref":"develop","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"}`)
	}))

	out, err := files.Get(context.Background(), client, files.GetInput{
		ProjectID: "42",
		FilePath:  "README.md",
		Ref:       "develop",
	})
	if err != nil {
		t.Fatalf("files.Get() unexpected error: %v", err)
	}
	if out.Content != "Hello" {
		t.Errorf("Content = %q, want %q", out.Content, "Hello")
	}
}

// TestFileGet_NonBase64 exercises the non-base64 encoding branch in fileGet.
func TestFileGet_NonBase64(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"file_name":"README.md","file_path":"README.md","size":5,"encoding":"text","content":"Hello","ref":"main","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"}`)
	}))

	out, err := files.Get(context.Background(), client, files.GetInput{
		ProjectID: "42",
		FilePath:  "README.md",
	})
	if err != nil {
		t.Fatalf("files.Get() unexpected error: %v", err)
	}
	if out.Content != "Hello" {
		t.Errorf("Content = %q, want %q", out.Content, "Hello")
	}
}

// TestFileGet_InvalidBase64 exercises the base64 decode error branch in fileGet.
func TestFileGet_InvalidBase64(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"file_name":"f.go","file_path":"f.go","size":5,"encoding":"base64","content":"!!!invalid!!!","ref":"main","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"}`)
	}))

	_, err := files.Get(context.Background(), client, files.GetInput{
		ProjectID: "42",
		FilePath:  "f.go",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64 content, got nil")
	}
}

// TestUnmarshalParams_MarshalError exercises the json.Marshal error branch in unmarshalParams.
func TestUnmarshalParamsMarshal_Error(t *testing.T) {
	// json.Marshal fails on channels
	params := map[string]any{"ch": make(chan int)}
	_, err := unmarshalParams[mergerequests.GetInput](params)
	if err == nil {
		t.Fatal("expected error for un-marshalable params")
	}
}

// TestMakeMetaHandler_SuccessfulDispatch exercises the successful dispatch path.
func TestMakeMetaHandler_SuccessfulDispatch(t *testing.T) {
	called := false
	handler := makeMetaHandler("test_tool", map[string]actionFunc{
		"get": func(ctx context.Context, params map[string]any) (any, error) {
			called = true
			return "result", nil
		},
	})

	_, result, err := handler(context.Background(), nil, MetaToolInput{Action: "get", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result != "result" {
		t.Errorf("result = %v, want %q", result, "result")
	}
}

// TestCommitToOutput_NilDate exercises the nil CommittedDate branch.
func TestCommitToOutput_NilDate(t *testing.T) {
	// json.Unmarshal will produce a nil CommittedDate if field is missing
	raw := `{"id":"abc","short_id":"a","title":"t","author_name":"n","author_email":"e@e.com","web_url":"http://x"}`
	var input struct {
		ID          string `json:"id"`
		ShortID     string `json:"short_id"`
		Title       string `json:"title"`
		AuthorName  string `json:"author_name"`
		AuthorEmail string `json:"author_email"`
		WebURL      string `json:"web_url"`
	}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	// The commitToOutput test with nil date is already covered via mocks
	// that don't include committed_date; this verifies the CommittedDate is empty.
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":"abc","short_id":"a","title":"t","message":"m","author_name":"n","author_email":"e@e.com","web_url":"http://x"}`)
	}))

	out, err := commits.Create(context.Background(), client, commits.CreateInput{
		ProjectID:     "42",
		Branch:        "main",
		CommitMessage: "t",
		Actions:       []commits.Action{{Action: "create", FilePath: "f.txt", Content: "x"}},
	})
	if err != nil {
		t.Fatalf("commits.Create() unexpected error: %v", err)
	}
	if out.CommittedDate != "" {
		t.Errorf("CommittedDate = %q, want empty string", out.CommittedDate)
	}
}
