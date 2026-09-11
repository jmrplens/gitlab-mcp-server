// mr_changes_test.go contains unit tests for merge request diff/changes
// retrieval and diff version operations. Tests use httptest to mock the
// GitLab Merge Request Diffs API and verify success, not-found, and
// empty-diff scenarios.
package mrchanges

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestMRChangesGet_Success verifies that mrChangesGet returns the correct file
// diffs for a merge request. The mock returns two diffs (one modified file and
// one new file) and the test asserts paths and the new-file flag.
func TestMRChangesGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/diffs" {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"internal/tools/repositories.go","new_path":"internal/tools/repositories.go","diff":"@@ -1,5 +1,10 @@\n package mrchanges\n","new_file":false,"renamed_file":false,"deleted_file":false,"a_mode":"100644","b_mode":"100644"},{"old_path":"/dev/null","new_path":"internal/tools/branches.go","diff":"@@ -0,0 +1,20 @@\n+package mrchanges\n","new_file":true,"renamed_file":false,"deleted_file":false,"a_mode":"0","b_mode":"100644"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf("mrChangesGet() unexpected error: %v", err)
	}
	if len(out.Changes) != 2 {
		t.Errorf("len(out.Changes) = %d, want 2", len(out.Changes))
	}
	if out.Changes[0].NewPath != "internal/tools/repositories.go" {
		t.Errorf("out.Changes[0].NewPath = %q, want %q", out.Changes[0].NewPath, "internal/tools/repositories.go")
	}
	if !out.Changes[1].NewFile {
		t.Error("out.Changes[1].NewFile = false, want true")
	}
}

// TestMRChangesGet_NotFound verifies that mrChangesGet returns an error when
// the GitLab API responds with 404 for a non-existent merge request.
func TestMRChangesGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 9999})
	if err == nil {
		t.Fatal("mrChangesGet() expected error for non-existent MR, got nil")
	}
}

// TestMRChangesGet_EmptyDiff verifies that mrChangesGet handles an empty diff
// response gracefully, returning zero changes without error.
func TestMRChangesGet_EmptyDiff(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/diffs" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf("mrChangesGet() unexpected error for empty diff: %v", err)
	}
	if len(out.Changes) != 0 {
		t.Errorf("len(out.Changes) = %d, want 0", len(out.Changes))
	}
}

// TestMRChangesGet_TruncatedFiles verifies that Get populates TruncatedFiles
// for non-deleted files with empty diffs (GitLab truncation behavior).
func TestMRChangesGet_TruncatedFiles(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/diffs" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"old_path":"small.go","new_path":"small.go","diff":"@@ -1 +1 @@\n-a\n+b","new_file":false,"renamed_file":false,"deleted_file":false},
				{"old_path":"big_test.c","new_path":"big_test.c","diff":"","new_file":false,"renamed_file":false,"deleted_file":false},
				{"old_path":"huge_test.c","new_path":"huge_test.c","diff":"","new_file":false,"renamed_file":false,"deleted_file":false},
				{"old_path":"removed.go","new_path":"removed.go","diff":"","new_file":false,"renamed_file":false,"deleted_file":true}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Changes) != 4 {
		t.Fatalf("len(out.Changes) = %d, want 4", len(out.Changes))
	}
	if len(out.TruncatedFiles) != 2 {
		t.Fatalf("len(out.TruncatedFiles) = %d, want 2", len(out.TruncatedFiles))
	}
	if out.TruncatedFiles[0] != "big_test.c" || out.TruncatedFiles[1] != "huge_test.c" {
		t.Errorf("TruncatedFiles = %v, want [big_test.c huge_test.c]", out.TruncatedFiles)
	}
}

// ---------------------------------------------------------------------------
// Diff Versions — Tests
// ---------------------------------------------------------------------------.

// diffVersionsListResponse identifies the diff versions list response constant used by this package.
const diffVersionsListResponse = `[
  {"id":1,"head_commit_sha":"abc123","base_commit_sha":"def456","start_commit_sha":"ghi789","created_at":"2026-01-15T10:00:00Z","merge_request_id":1,"state":"collected","real_size":"3"},
  {"id":2,"head_commit_sha":"jkl012","base_commit_sha":"mno345","start_commit_sha":"pqr678","created_at":"2026-01-16T10:00:00Z","merge_request_id":1,"state":"collected","real_size":"5"}
]`

// diffVersionGetResponse identifies the diff version get response constant used by this package.
const diffVersionGetResponse = `{
  "id":2,
  "head_commit_sha":"jkl012",
  "base_commit_sha":"mno345",
  "start_commit_sha":"pqr678",
  "created_at":"2026-01-16T10:00:00Z",
  "merge_request_id":1,
  "state":"collected",
  "real_size":"5",
  "patch_id_sha":"9f2c1d0",
  "commits":[
    {"id":"jkl012abc","short_id":"jkl012a","title":"Fix bug","author_name":"Dev","created_at":"2026-01-16T09:00:00Z"}
  ],
  "diffs":[
    {"diff":"@@ -1 +1 @@\n-old\n+new","new_path":"main.go","old_path":"main.go","a_mode":"100644","b_mode":"100644","new_file":false,"renamed_file":false,"deleted_file":false}
  ]
}`

// TestListDiffVersions_Success verifies ListDiffVersions when success.
func TestListDiffVersions_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/versions" {
			testutil.RespondJSON(w, http.StatusOK, diffVersionsListResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListDiffVersions(context.Background(), client, DiffVersionsListInput{
		ProjectID: "42", MRIID: 1,
	})
	if err != nil {
		t.Fatalf("ListDiffVersions() unexpected error: %v", err)
	}
	if len(out.DiffVersions) != 2 {
		t.Fatalf("len(DiffVersions) = %d, want 2", len(out.DiffVersions))
	}
	if out.DiffVersions[0].ID != 1 {
		t.Errorf("DiffVersions[0].ID = %d, want 1", out.DiffVersions[0].ID)
	}
	if out.DiffVersions[1].HeadCommitSHA != "jkl012" {
		t.Errorf("DiffVersions[1].HeadCommitSHA = %q, want %q", out.DiffVersions[1].HeadCommitSHA, "jkl012")
	}
	if out.DiffVersions[0].State != "collected" {
		t.Errorf("DiffVersions[0].State = %q, want %q", out.DiffVersions[0].State, "collected")
	}
}

// TestListDiffVersions_MissingProject verifies ListDiffVersions when missing project.
func TestListDiffVersions_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := ListDiffVersions(context.Background(), client, DiffVersionsListInput{
		MRIID: 1,
	})
	if err == nil {
		t.Fatal("ListDiffVersions() expected error for missing project_id, got nil")
	}
}

// TestListDiffVersions_Error verifies ListDiffVersions when error.
func TestListDiffVersions_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	_, err := ListDiffVersions(context.Background(), client, DiffVersionsListInput{
		ProjectID: "42", MRIID: 9999,
	})
	if err == nil {
		t.Fatal("ListDiffVersions() expected error for 404, got nil")
	}
}

// TestGetDiffVersion_Success verifies GetDiffVersion when success.
func TestGetDiffVersion_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/versions/2" {
			testutil.RespondJSON(w, http.StatusOK, diffVersionGetResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{
		ProjectID: "42", MRIID: 1, VersionID: 2,
	})
	if err != nil {
		t.Fatalf("GetDiffVersion() unexpected error: %v", err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.HeadCommitSHA != "jkl012" {
		t.Errorf("HeadCommitSHA = %q, want %q", out.HeadCommitSHA, "jkl012")
	}
	if out.PatchIDSHA != "9f2c1d0" {
		t.Errorf("PatchIDSHA = %q, want %q", out.PatchIDSHA, "9f2c1d0")
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
	if out.Commits[0].Title != "Fix bug" {
		t.Errorf("Commits[0].Title = %q, want %q", out.Commits[0].Title, "Fix bug")
	}
	if len(out.Diffs) != 1 {
		t.Fatalf("len(Diffs) = %d, want 1", len(out.Diffs))
	}
	if out.Diffs[0].NewPath != "main.go" {
		t.Errorf("Diffs[0].NewPath = %q, want %q", out.Diffs[0].NewPath, "main.go")
	}
}

// TestGetDiffVersion_MissingProject verifies GetDiffVersion when missing project.
func TestGetDiffVersion_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{
		MRIID: 1, VersionID: 2,
	})
	if err == nil {
		t.Fatal("GetDiffVersion() expected error for missing project_id, got nil")
	}
}

// TestGetDiffVersion_Error verifies GetDiffVersion when error.
func TestGetDiffVersion_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	_, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{
		ProjectID: "42", MRIID: 1, VersionID: 999,
	})
	if err == nil {
		t.Fatal("GetDiffVersion() expected error for 404, got nil")
	}
}

// ---------------------------------------------------------------------------
// MRIID & VersionID required-field validation
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
		{"Get", func() error {
			_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
			return err
		}},
		{"ListDiffVersions", func() error {
			_, err := ListDiffVersions(context.Background(), client, DiffVersionsListInput{ProjectID: "42"})
			return err
		}},
		{"GetDiffVersion", func() error {
			_, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{ProjectID: "42"})
			return err
		}},
		{"RawDiffs", func() error {
			_, err := RawDiffs(context.Background(), client, RawDiffsInput{ProjectID: "42"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "merge_request_iid")
		})
	}
}

// TestVersionIDRequired_Validation verifies VersionIDRequired when validation.
func TestVersionIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	t.Run("GetDiffVersion", func(t *testing.T) {
		_, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{ProjectID: "42", MRIID: 1})
		assertContains(t, err, "version_id")
	})
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// fileTableHead is the header and delimiter of the file table the changes
// result and the diff version card share.
const fileTableHead = "| File | Status |\n| --- | --- |\n"

// TestFormatOutputMarkdown_WithChanges verifies the whole rendering of a merge
// request's changes: the counts, the file table, then each patch as a fenced
// block under the file it belongs to. The patches used to be dropped entirely,
// so the result named the files and never showed a line of what changed.
func TestFormatOutputMarkdown_WithChanges(t *testing.T) {
	out := Output{
		MRIID: 42,
		Changes: []FileDiffOutput{
			{NewPath: "main.go", OldPath: "main.go", Diff: "some diff", NewFile: false, DeletedFile: false, RenamedFile: false},
			{NewPath: "new_file.go", OldPath: "/dev/null", Diff: "+new", NewFile: true},
			{NewPath: "removed.go", OldPath: "removed.go", DeletedFile: true},
			{NewPath: "new_name.go", OldPath: "old_name.go", Diff: "rename diff", RenamedFile: true},
		},
	}
	want := "## MR !42 Changes\n\n" +
		"- **Files**: 4\n\n" +
		"### Files\n\n" + fileTableHead +
		"| main.go | modified |\n" +
		"| new_file.go | added |\n" +
		"| removed.go | deleted |\n" +
		"| new_name.go | renamed from old_name.go |\n\n" +
		"### main.go\n\n```diff\nsome diff\n```\n\n" +
		"### new_file.go\n\n```diff\n+new\n```\n\n" +
		"### new_name.go\n\n```diff\nrename diff\n```\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.diff_versions_list' to list every diff version of this merge request\n"
	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatOutputMarkdown_TruncatedFiles verifies that GitLab's own truncation
// is counted and pointed at truncated_files, and that a deleted file — which
// carries no patch because there is nothing left of it — is not counted as
// truncated.
func TestFormatOutputMarkdown_TruncatedFiles(t *testing.T) {
	out := Output{
		MRIID: 99,
		Changes: []FileDiffOutput{
			{NewPath: "small.go", OldPath: "small.go", Diff: "some diff"},
			{NewPath: "big_test.c", OldPath: "big_test.c", Diff: ""},
			{NewPath: "huge_test.c", OldPath: "huge_test.c", Diff: ""},
			{NewPath: "removed.go", OldPath: "removed.go", Diff: "", DeletedFile: true},
		},
		TruncatedFiles: []string{"big_test.c", "huge_test.c"},
	}
	want := "## MR !99 Changes\n\n" +
		"- **Files**: 4\n" +
		"- **Truncated by GitLab**: 2\n\n" +
		"### Files\n\n" + fileTableHead +
		"| small.go | modified |\n" +
		"| big_test.c | modified |\n" +
		"| huge_test.c | modified |\n" +
		"| removed.go | deleted |\n\n" +
		"### small.go\n\n```diff\nsome diff\n```\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.diff_versions_list' to list every diff version of this merge request\n" +
		"- GitLab truncated 2 file diff(s); truncated_files names them. Use action 'mr_review.diff_version_get' with a version_id for the full patch\n"
	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatOutputMarkdown_Empty verifies that a merge request with no file
// change renders the one-sentence empty message and no table header.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	want := "No file changes found.\n"
	if got := FormatOutputMarkdown(Output{MRIID: 7}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatDiffVersionsListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatDiffVersionsListMarkdown_WithVersions verifies FormatDiffVersionsListMarkdown when with versions.
func TestFormatDiffVersionsListMarkdown_WithVersions(t *testing.T) {
	out := DiffVersionsListOutput{
		DiffVersions: []DiffVersionOutput{
			{ID: 1, State: "collected", HeadCommitSHA: "abcdef1234567890", BaseCommitSHA: "1234567890abcdef", CreatedAt: "2026-01-15T10:00:00Z"},
			{ID: 2, State: "overflow", HeadCommitSHA: "short", BaseCommitSHA: "short2", CreatedAt: "2026-01-16T10:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	want := "## MR Diff Versions (2)\n\n" +
		"| ID | State | Head SHA | Base SHA | Created |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | collected | `abcdef12` | `12345678` | 15 Jan 2026 10:00 UTC |\n" +
		"| 2 | overflow | `short` | `short2` | 16 Jan 2026 10:00 UTC |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.diff_version_get' to read one version's commits and file diffs\n"
	if got := FormatDiffVersionsListMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatDiffVersionsListMarkdown_Empty verifies that a merge request with
// no diff version renders the one-sentence empty message.
func TestFormatDiffVersionsListMarkdown_Empty(t *testing.T) {
	want := "No diff versions found.\n"
	if got := FormatDiffVersionsListMarkdown(DiffVersionsListOutput{}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatDiffVersionGetMarkdown
// ---------------------------------------------------------------------------.

// TestFormatDiffVersionGetMarkdown_Full verifies FormatDiffVersionGetMarkdown when full.
func TestFormatDiffVersionGetMarkdown_Full(t *testing.T) {
	out := DiffVersionOutput{
		ID:             5,
		State:          "collected",
		HeadCommitSHA:  "abc123",
		BaseCommitSHA:  "def456",
		StartCommitSHA: "ghi789",
		CreatedAt:      "2026-01-16T10:00:00Z",
		RealSize:       "3",
		Commits: []DiffVersionCommitOutput{
			{ID: "fullhashvalue", ShortID: "shrt123", Title: "Fix bug", AuthorName: "Dev"},
			{ID: "anotherhash1234", ShortID: "", Title: "Second commit", AuthorName: "Dev2"},
		},
		Diffs: []FileDiffOutput{
			{NewPath: "main.go", OldPath: "main.go"},
			{NewPath: "added.go", OldPath: "/dev/null", NewFile: true},
			{NewPath: "deleted.go", OldPath: "deleted.go", DeletedFile: true},
			{NewPath: "renamed.go", OldPath: "original.go", RenamedFile: true},
		},
	}
	want := "## Diff Version 5\n\n" +
		"- **ID**: 5\n" +
		"- **State**: collected\n" +
		"- **Head SHA**: `abc123`\n" +
		"- **Base SHA**: `def456`\n" +
		"- **Start SHA**: `ghi789`\n" +
		"- **Created**: 16 Jan 2026 10:00 UTC\n" +
		"- **Real Size**: 3\n\n" +
		"### Commits (2)\n\n" +
		"| SHA | Author | Title |\n| --- | --- | --- |\n" +
		"| `shrt123` | Dev | Fix bug |\n" +
		"| `anotherh` | Dev2 | Second commit |\n\n" +
		"### File Changes (4)\n\n" + fileTableHead +
		"| main.go | modified |\n" +
		"| added.go | added |\n" +
		"| deleted.go | deleted |\n" +
		"| renamed.go | renamed from original.go |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.diff_versions_list' to list every diff version of this merge request\n"
	if got := FormatDiffVersionGetMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatDiffVersionGetMarkdown_Minimal verifies that a diff version GitLab
// sent nothing optional for shows no label with nothing after it and opens no
// empty section.
func TestFormatDiffVersionGetMarkdown_Minimal(t *testing.T) {
	out := DiffVersionOutput{
		ID:            1,
		State:         "empty",
		HeadCommitSHA: "abc",
		BaseCommitSHA: "def",
	}
	want := "## Diff Version 1\n\n" +
		"- **ID**: 1\n" +
		"- **State**: empty\n" +
		"- **Head SHA**: `abc`\n" +
		"- **Base SHA**: `def`\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.diff_versions_list' to list every diff version of this merge request\n"
	if got := FormatDiffVersionGetMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// RawDiffs handler
// ---------------------------------------------------------------------------.

// TestRawDiffs_Success verifies RawDiffs when success.
func TestRawDiffs_Success(t *testing.T) {
	const rawDiff = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/raw_diffs" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(rawDiff))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RawDiffs(context.Background(), client, RawDiffsInput{ProjectID: "42", MRIID: 1})
	if err != nil {
		t.Fatalf("RawDiffs() unexpected error: %v", err)
	}
	if out.MRIID != 1 {
		t.Errorf("MRIID = %d, want 1", out.MRIID)
	}
	if out.RawDiff != rawDiff {
		t.Errorf("RawDiff = %q, want %q", out.RawDiff, rawDiff)
	}
}

// TestRawDiffs_MissingProjectID verifies RawDiffs when missing project ID.
func TestRawDiffs_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := RawDiffs(context.Background(), client, RawDiffsInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestRawDiffs_APIError verifies RawDiffs when API error.
func TestRawDiffs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := RawDiffs(context.Background(), client, RawDiffsInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// TestRawDiffs_CancelledContext verifies RawDiffs when cancelled context.
func TestRawDiffs_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := RawDiffs(ctx, client, RawDiffsInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatRawDiffsMarkdown
// ---------------------------------------------------------------------------.

// rawDiffsHints is the guidance section the raw diff card closes with.
const rawDiffsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'mr_review.changes_get' to see the file-by-file change summary\n"

// TestFormatRawDiffsMarkdown_WithDiff verifies the whole rendering of a raw
// patch: the heading, then the patch inside one fenced block.
func TestFormatRawDiffsMarkdown_WithDiff(t *testing.T) {
	const diff = "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1 +1 @@\n-old\n+new\n"
	want := "## MR !3 Raw Diffs\n\n```diff\n" + diff + "```\n" + rawDiffsHints
	if got := FormatRawDiffsMarkdown(RawDiffsOutput{MRIID: 3, RawDiff: diff}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRawDiffsMarkdown_Empty verifies that a merge request with no patch
// renders the one-sentence empty message and opens no fence.
func TestFormatRawDiffsMarkdown_Empty(t *testing.T) {
	want := "No diffs found.\n"
	if got := FormatRawDiffsMarkdown(RawDiffsOutput{MRIID: 4}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRawDiffsMarkdown_NoTrailingNewline verifies that a patch GitLab
// sent without a trailing newline still gets one before the closing fence,
// which is what keeps the fence on a line of its own.
func TestFormatRawDiffsMarkdown_NoTrailingNewline(t *testing.T) {
	want := "## MR !5 Raw Diffs\n\n```diff\nsome diff without trailing newline\n```\n" + rawDiffsHints
	if got := FormatRawDiffsMarkdown(RawDiffsOutput{MRIID: 5, RawDiff: "some diff without trailing newline"}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Canceled-context paths for existing handlers
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies Get when cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(ctx, client, GetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGet_MissingProjectID verifies Get when missing project ID.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestListDiffVersions_CancelledContext verifies ListDiffVersions when cancelled context.
func TestListDiffVersions_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := ListDiffVersions(ctx, client, DiffVersionsListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGetDiffVersion_CancelledContext verifies GetDiffVersion when cancelled context.
func TestGetDiffVersion_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := GetDiffVersion(ctx, client, DiffVersionGetInput{ProjectID: "42", MRIID: 1, VersionID: 2})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGetDiffVersion_Unidiff verifies GetDiffVersion when unidiff.
func TestGetDiffVersion_Unidiff(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1/versions/2" {
			if r.URL.Query().Get("unidiff") != "true" {
				t.Error("expected unidiff=true query parameter")
			}
			testutil.RespondJSON(w, http.StatusOK, diffVersionGetResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{
		ProjectID: "42", MRIID: 1, VersionID: 2, Unidiff: true,
	})
	if err != nil {
		t.Fatalf("GetDiffVersion(unidiff) unexpected error: %v", err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
}

// ---------------------------------------------------------------------------
// TestActionSpecs_CallAllRoutes — canonical route execution for all 4 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates all MR change actions through their canonical ActionSpecs routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	specs := newMRChangesActionSpecs(t)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_changes_get", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_diff_versions_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_diff_version_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "version_id": 2}},
		{"gitlab_mr_raw_diffs", map[string]any{"project_id": "42", "merge_request_iid": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			if !spec.ReadOnly || !spec.Idempotent || spec.OwnerPackage != "mrchanges" {
				t.Fatalf("unexpected ActionSpec semantics for %s: %+v", tt.name, spec)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestMRChangesGet_PaginationAndOptions verifies that Get forwards offset
// pagination, keyset pagination, order_by, sort, and unidiff as query
// parameters to the GitLab Merge Request Diffs API, and that the new
// collapsed/too_large diff flags are surfaced in the output.
func TestMRChangesGet_PaginationAndOptions(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"big.go","new_path":"big.go","diff":"","new_file":false,"deleted_file":false,"collapsed":true,"too_large":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		MRIID:     1,
		Unidiff:   true,
		OrderBy:   "id",
		Sort:      "desc",
		Page:      2, PerPage: 50,
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	for _, want := range []string{"unidiff=true", "order_by=id", "sort=desc", "page=2", "per_page=50", "pagination=keyset", "page_token=tok123"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
	if len(out.Changes) != 1 {
		t.Fatalf("len(Changes) = %d, want 1", len(out.Changes))
	}
	if !out.Changes[0].Collapsed || !out.Changes[0].TooLarge {
		t.Errorf("Collapsed=%v TooLarge=%v, want both true", out.Changes[0].Collapsed, out.Changes[0].TooLarge)
	}
}

// TestListDiffVersions_PaginationAndOptions verifies ListDiffVersions forwards
// offset, keyset, order_by, and sort parameters to the diff versions endpoint.
func TestListDiffVersions_PaginationAndOptions(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListDiffVersions(context.Background(), client, DiffVersionsListInput{
		ProjectID: "42",
		MRIID:     1,
		OrderBy:   "id",
		Sort:      "asc",
		Page:      3, PerPage: 25,
		Pagination: "keyset", PageToken: "cur9",
	})
	if err != nil {
		t.Fatalf("ListDiffVersions() unexpected error: %v", err)
	}
	for _, want := range []string{"order_by=id", "sort=asc", "page=3", "per_page=25", "pagination=keyset", "page_token=cur9"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestGetDiffVersion_FullCommitFields verifies that diffVersionToOutput surfaces
// every gl.Commit field (the full C-IMPORTS commit mirror), including authored/
// committed dates, status, and stats.
func TestGetDiffVersion_FullCommitFields(t *testing.T) {
	const body = `{
      "id":2,"head_commit_sha":"jkl012","base_commit_sha":"mno345","start_commit_sha":"pqr678",
      "created_at":"2026-01-16T10:00:00Z","merge_request_id":1,"state":"collected","real_size":"5",
      "commits":[
        {"id":"jkl012abc","short_id":"jkl012a","title":"Fix bug","author_name":"Dev","author_email":"dev@example.com",
         "authored_date":"2026-01-16T08:00:00Z","committer_name":"Comm","committer_email":"comm@example.com",
         "committed_date":"2026-01-16T08:30:00Z","created_at":"2026-01-16T09:00:00Z","message":"Fix bug\n",
         "parent_ids":["p1","p2"],"status":"success","project_id":42,"web_url":"https://gitlab.example.com/c/jkl012abc",
         "trailers":{"Signed-off-by":"Dev"},"extended_trailers":{"Signed-off-by":"Dev"},
         "stats":{"additions":10,"deletions":2,"total":12}}
      ],
      "diffs":[]
    }`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/versions/2") {
			testutil.RespondJSON(w, http.StatusOK, body)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{ProjectID: "42", MRIID: 1, VersionID: 2})
	if err != nil {
		t.Fatalf("GetDiffVersion() unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
	c := out.Commits[0]
	if c.AuthorEmail != "dev@example.com" {
		t.Errorf("AuthorEmail = %q, want dev@example.com", c.AuthorEmail)
	}
	if c.CommitterName != "Comm" || c.CommitterEmail != "comm@example.com" {
		t.Errorf("Committer = %q/%q", c.CommitterName, c.CommitterEmail)
	}
	if c.AuthoredDate == "" || c.CommittedDate == "" {
		t.Errorf("dates empty: authored=%q committed=%q", c.AuthoredDate, c.CommittedDate)
	}
	if c.Status != "success" {
		t.Errorf("Status = %q, want success", c.Status)
	}
	if c.ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", c.ProjectID)
	}
	if len(c.ParentIDs) != 2 {
		t.Errorf("ParentIDs = %v, want 2 entries", c.ParentIDs)
	}
	if c.WebURL == "" {
		t.Error("WebURL empty")
	}
	if c.Trailers["Signed-off-by"] != "Dev" {
		t.Errorf("Trailers = %v", c.Trailers)
	}
	if c.Stats == nil || c.Stats.Additions != 10 || c.Stats.Deletions != 2 || c.Stats.Total != 12 {
		t.Errorf("Stats = %+v", c.Stats)
	}
}

// TestMRChangeActionSpecs_Metadata verifies the R-META discovery metadata:
// each MR-changes action has non-generic Usage, natural-language aliases beyond
// the tool name, canonical RelatedActions, and a "Returns: … See also: …"
// individual-tool description.
func TestMRChangeActionSpecs_Metadata(t *testing.T) {
	specs := ActionSpecs(nil)
	if len(specs) != 4 {
		t.Fatalf("len(specs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		name := spec.IndividualTool.Name
		if strings.HasPrefix(strings.ToLower(spec.Usage), "use to execute") {
			t.Errorf("%s: generic Usage %q", name, spec.Usage)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: empty RelatedActions", name)
		}
		nlAliases := 0
		for _, a := range spec.Aliases {
			if a != name && !strings.HasPrefix(a, "gitlab_") {
				nlAliases++
			}
		}
		if nlAliases == 0 {
			t.Errorf("%s: no natural-language aliases (aliases=%v)", name, spec.Aliases)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: weak description %q", name, desc)
		}
		if len(spec.ParameterGuidance) == 0 {
			t.Errorf("%s: empty ParameterGuidance", name)
		}
	}
}

// TestDecorateMRChangeMeta_UnknownTool verifies decorateMRChangeMeta leaves the
// base options untouched for an unrecognized individual tool name.
func TestDecorateMRChangeMeta_UnknownTool(t *testing.T) {
	opts := mrChangeOptions("gitlab_unknown_tool")
	decorateMRChangeMeta(&opts, "gitlab_unknown_tool")
	if !strings.HasPrefix(opts.Usage, "Use to execute") {
		t.Errorf("Usage changed for unknown tool: %q", opts.Usage)
	}
	if opts.IndividualTool.Description != "" {
		t.Errorf("Description set for unknown tool: %q", opts.IndividualTool.Description)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// TestDiffVersions_UnreadableCapturedPatchIDSHA verifies that both diff version
// handlers return an error rather than a half-filled version when GitLab sends
// patch_id_sha as something that is not a string. The SDK ignores the key its
// own MergeRequestDiffVersion does not model, so the read of the captured
// response is the only thing that can notice.
func TestDiffVersions_UnreadableCapturedPatchIDSHA(t *testing.T) {
	// A list answers with an array and a get with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"head_commit_sha":"abc","patch_id_sha":42}]`)
			_, err := ListDiffVersions(context.Background(), client, DiffVersionsListInput{ProjectID: "42", MRIID: 7})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"head_commit_sha":"abc","patch_id_sha":42}`)
			_, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{ProjectID: "42", MRIID: 7, VersionID: 1})
			return err
		}},
	})
}

// newMRChangesActionSpecs builds canonical action specs backed by a mock GitLab API.
func newMRChangesActionSpecs(t *testing.T) []toolutil.ActionSpec {
	t.Helper()

	const diffsJSON = `[{"old_path":"main.go","new_path":"main.go","diff":"@@ -1 +1 @@\n-old\n+new","new_file":false,"renamed_file":false,"deleted_file":false,"a_mode":"100644","b_mode":"100644"}]`
	const versionsJSON = `[{"id":1,"head_commit_sha":"abc123","base_commit_sha":"def456","start_commit_sha":"ghi789","created_at":"2026-01-15T10:00:00Z","merge_request_id":1,"state":"collected","real_size":"3"}]`
	const rawDiffBody = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// GET .../diffs → list changes
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/diffs"):
			testutil.RespondJSON(w, http.StatusOK, diffsJSON)

		// GET .../raw_diffs → raw diff
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/raw_diffs"):
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(rawDiffBody))

		// GET .../versions/{id} → single version
		case r.Method == http.MethodGet && strings.Contains(path, "/versions/"):
			testutil.RespondJSON(w, http.StatusOK, diffVersionGetResponse)

		// GET .../versions → list versions
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/versions"):
			testutil.RespondJSON(w, http.StatusOK, versionsJSON)

		default:
			http.NotFound(w, r)
		}
	}))

	return ActionSpecs(client)
}
