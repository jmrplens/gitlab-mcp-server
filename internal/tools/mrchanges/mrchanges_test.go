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

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

const diffVersionsListResponse = `[
  {"id":1,"head_commit_sha":"abc123","base_commit_sha":"def456","start_commit_sha":"ghi789","created_at":"2026-01-15T10:00:00Z","merge_request_id":1,"state":"collected","real_size":"3"},
  {"id":2,"head_commit_sha":"jkl012","base_commit_sha":"mno345","start_commit_sha":"pqr678","created_at":"2026-01-16T10:00:00Z","merge_request_id":1,"state":"collected","real_size":"5"}
]`

const diffVersionGetResponse = `{
  "id":2,
  "head_commit_sha":"jkl012",
  "base_commit_sha":"mno345",
  "start_commit_sha":"pqr678",
  "created_at":"2026-01-16T10:00:00Z",
  "merge_request_id":1,
  "state":"collected",
  "real_size":"5",
  "commits":[
    {"id":"jkl012abc","short_id":"jkl012a","title":"Fix bug","author_name":"Dev","created_at":"2026-01-16T09:00:00Z"}
  ],
  "diffs":[
    {"diff":"@@ -1 +1 @@\n-old\n+new","new_path":"main.go","old_path":"main.go","a_mode":"100644","b_mode":"100644","new_file":false,"renamed_file":false,"deleted_file":false}
  ]
}`

// TestListDiffVersions_Success verifies the behavior of list diff versions success.
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

// TestListDiffVersions_MissingProject verifies the behavior of list diff versions missing project.
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

// TestListDiffVersions_Error verifies the behavior of list diff versions error.
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

// TestGetDiffVersion_Success verifies the behavior of get diff version success.
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

// TestGetDiffVersion_MissingProject verifies the behavior of get diff version missing project.
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

// TestGetDiffVersion_Error verifies the behavior of get diff version error.
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

// assertContains is an internal helper for the mrchanges package.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

// TestMRIIDRequired_Validation validates m r i i d required validation across multiple scenarios using table-driven subtests.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when mr_iid is missing")
	}))

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
			assertContains(t, tt.fn(), "mr_iid")
		})
	}
}

// TestVersionIDRequired_Validation verifies the behavior of version i d required validation.
func TestVersionIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when version_id is missing")
	}))

	t.Run("GetDiffVersion", func(t *testing.T) {
		_, err := GetDiffVersion(context.Background(), client, DiffVersionGetInput{ProjectID: "42", MRIID: 1})
		assertContains(t, err, "version_id")
	})
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_WithChanges verifies the behavior of format output markdown with changes.
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
	md := FormatOutputMarkdown(out)

	for _, want := range []string{
		"## MR !42 Changes (4 files)",
		"| File | Status |",
		"| main.go | modified |",
		"| new_file.go | added |",
		"| removed.go | deleted |",
		"| new_name.go | renamed from old_name.go |",
		"diff_versions_list",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	for _, absent := range []string{"raw_diffs", "truncation"} {
		if strings.Contains(md, absent) {
			t.Errorf("markdown should not contain %q when no files are truncated:\n%s", absent, md)
		}
	}
}

// TestFormatOutputMarkdown_TruncatedFiles verifies hints when GitLab truncates large diffs.
func TestFormatOutputMarkdown_TruncatedFiles(t *testing.T) {
	out := Output{
		MRIID: 99,
		Changes: []FileDiffOutput{
			{NewPath: "small.go", OldPath: "small.go", Diff: "some diff"},
			{NewPath: "big_test.c", OldPath: "big_test.c", Diff: ""},
			{NewPath: "huge_test.c", OldPath: "huge_test.c", Diff: ""},
			{NewPath: "removed.go", OldPath: "removed.go", Diff: "", DeletedFile: true},
		},
	}
	md := FormatOutputMarkdown(out)

	for _, want := range []string{
		"## MR !99 Changes (4 files)",
		"diff_versions_list",
		"diff_version_get",
		"big_test.c",
		"huge_test.c",
		"truncation",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "removed.go") && strings.Contains(md, "truncation") {
		// Deleted files with empty diff should NOT be in truncation warning
		if strings.Contains(md, "removed.go, ") || strings.Contains(md, ", removed.go") {
			t.Errorf("deleted file should not appear in truncation warning:\n%s", md)
		}
	}
}

// TestFormatOutputMarkdown_Empty verifies the behavior of format output markdown empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	out := Output{MRIID: 7, Changes: nil}
	md := FormatOutputMarkdown(out)

	if !strings.Contains(md, "No file changes found.") {
		t.Errorf("expected 'No file changes found.' in markdown:\n%s", md)
	}
	if strings.Contains(md, "| File |") {
		t.Error("should not contain table header when no changes")
	}
}

// ---------------------------------------------------------------------------
// FormatDiffVersionsListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatDiffVersionsListMarkdown_WithVersions verifies the behavior of format diff versions list markdown with versions.
func TestFormatDiffVersionsListMarkdown_WithVersions(t *testing.T) {
	out := DiffVersionsListOutput{
		DiffVersions: []DiffVersionOutput{
			{ID: 1, State: "collected", HeadCommitSHA: "abcdef1234567890", BaseCommitSHA: "1234567890abcdef", CreatedAt: "2026-01-15T10:00:00Z"},
			{ID: 2, State: "overflow", HeadCommitSHA: "short", BaseCommitSHA: "short2", CreatedAt: "2026-01-16T10:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatDiffVersionsListMarkdown(out)

	for _, want := range []string{
		"## MR Diff Versions (2)",
		"| ID | State | Head SHA | Base SHA | Created |",
		"| 1 | collected |",
		"| 2 | overflow |",
		"abcdef12", // truncated to 8
		"12345678", // truncated to 8
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	// Full SHA should not appear (truncated to 8)
	if strings.Contains(md, "abcdef1234567890") {
		t.Errorf("head SHA should be truncated to 8 chars:\n%s", md)
	}
}

// TestFormatDiffVersionsListMarkdown_Empty verifies the behavior of format diff versions list markdown empty.
func TestFormatDiffVersionsListMarkdown_Empty(t *testing.T) {
	out := DiffVersionsListOutput{DiffVersions: nil}
	md := FormatDiffVersionsListMarkdown(out)

	if !strings.Contains(md, "No diff versions found.") {
		t.Errorf("expected 'No diff versions found.' in markdown:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatDiffVersionGetMarkdown
// ---------------------------------------------------------------------------.

// TestFormatDiffVersionGetMarkdown_Full verifies the behavior of format diff version get markdown full.
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
	md := FormatDiffVersionGetMarkdown(out)

	for _, want := range []string{
		"## Diff Version 5",
		"**State**: collected",
		"**Head SHA**: abc123",
		"**Base SHA**: def456",
		"**Start SHA**: ghi789",
		"**Created**: 16 Jan 2026 10:00 UTC",
		"**Real Size**: 3",
		"### Commits (2)",
		"| shrt123 |",
		"| anotherh |", // fallback: ID[:8] when ShortID empty
		"### File Changes (4)",
		"| main.go | modified |",
		"| added.go | added |",
		"| deleted.go | deleted |",
		"| renamed.go | renamed from original.go |",
		"diff_versions_list",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "raw_diffs") {
		t.Errorf("should not reference non-existent raw_diffs action:\n%s", md)
	}
}

// TestFormatDiffVersionGetMarkdown_Minimal verifies the behavior of format diff version get markdown minimal.
func TestFormatDiffVersionGetMarkdown_Minimal(t *testing.T) {
	out := DiffVersionOutput{
		ID:            1,
		State:         "empty",
		HeadCommitSHA: "abc",
		BaseCommitSHA: "def",
	}
	md := FormatDiffVersionGetMarkdown(out)

	if !strings.Contains(md, "## Diff Version 1") {
		t.Errorf("missing header:\n%s", md)
	}
	// No CreatedAt, RealSize, Commits, Diffs → those sections absent
	if strings.Contains(md, "**Created**") {
		t.Errorf("should not contain Created when empty:\n%s", md)
	}
	if strings.Contains(md, "**Real Size**") {
		t.Errorf("should not contain Real Size when empty:\n%s", md)
	}
	if strings.Contains(md, "### Commits") {
		t.Errorf("should not contain Commits section when empty:\n%s", md)
	}
	if strings.Contains(md, "### File Changes") {
		t.Errorf("should not contain File Changes section when empty:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// RawDiffs handler
// ---------------------------------------------------------------------------.

// TestRawDiffs_Success verifies the behavior of raw diffs success.
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

// TestRawDiffs_MissingProjectID verifies the behavior of raw diffs missing project i d.
func TestRawDiffs_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := RawDiffs(context.Background(), client, RawDiffsInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestRawDiffs_APIError verifies the behavior of raw diffs a p i error.
func TestRawDiffs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := RawDiffs(context.Background(), client, RawDiffsInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// TestRawDiffs_CancelledContext verifies the behavior of raw diffs cancelled context.
func TestRawDiffs_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

// TestFormatRawDiffsMarkdown_WithDiff verifies the behavior of format raw diffs markdown with diff.
func TestFormatRawDiffsMarkdown_WithDiff(t *testing.T) {
	out := RawDiffsOutput{MRIID: 3, RawDiff: "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1 +1 @@\n-old\n+new\n"}
	md := FormatRawDiffsMarkdown(out)

	for _, want := range []string{
		"## MR !3 Raw Diffs",
		"```diff",
		"diff --git",
		"```",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatRawDiffsMarkdown_Empty verifies the behavior of format raw diffs markdown empty.
func TestFormatRawDiffsMarkdown_Empty(t *testing.T) {
	out := RawDiffsOutput{MRIID: 4, RawDiff: ""}
	md := FormatRawDiffsMarkdown(out)

	if !strings.Contains(md, "No diffs found.") {
		t.Errorf("expected 'No diffs found.' in markdown:\n%s", md)
	}
	if strings.Contains(md, "```diff") {
		t.Error("should not contain code fence when no diffs")
	}
}

// TestFormatRawDiffsMarkdown_NoTrailingNewline verifies the behavior of format raw diffs markdown no trailing newline.
func TestFormatRawDiffsMarkdown_NoTrailingNewline(t *testing.T) {
	out := RawDiffsOutput{MRIID: 5, RawDiff: "some diff without trailing newline"}
	md := FormatRawDiffsMarkdown(out)

	// The formatter should add a newline before the closing fence
	if !strings.Contains(md, "some diff without trailing newline\n```") {
		t.Errorf("expected newline insertion before closing fence:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// Canceled-context paths for existing handlers
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(ctx, client, GetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGet_MissingProjectID verifies the behavior of get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{MRIID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestListDiffVersions_CancelledContext verifies the behavior of list diff versions cancelled context.
func TestListDiffVersions_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := ListDiffVersions(ctx, client, DiffVersionsListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGetDiffVersion_CancelledContext verifies the behavior of get diff version cancelled context.
func TestGetDiffVersion_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := GetDiffVersion(ctx, client, DiffVersionGetInput{ProjectID: "42", MRIID: 1, VersionID: 2})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestGetDiffVersion_Unidiff verifies the behavior of get diff version unidiff.
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
// TestRegisterTools_CallAllThroughMCP — full MCP roundtrip for all 4 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newMRChangesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_changes_get", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_diff_versions_list", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_diff_version_get", map[string]any{"project_id": "42", "mr_iid": 1, "version_id": 2}},
		{"gitlab_mr_raw_diffs", map[string]any{"project_id": "42", "mr_iid": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newMRChangesMCPSession is an internal helper for the mrchanges package.
func newMRChangesMCPSession(t *testing.T) *mcp.ClientSession {
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
