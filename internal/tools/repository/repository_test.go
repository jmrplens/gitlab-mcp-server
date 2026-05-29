// repository_test.go contains unit tests for GitLab repository operations
// (tree listing and branch/tag comparison). Tests use httptest to mock the
// GitLab API and verify both success and error paths.
package repository

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errExpAPIFailure identifies the err exp API failure constant used by this package.
	errExpAPIFailure = "expected error for API failure, got nil"
	// errExpEmptyProjectID identifies the err exp empty project ID constant used by this package.
	errExpEmptyProjectID = "expected error for empty project_id, got nil"
	// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
	errExpCancelledNil = "expected error for canceled context, got nil"
	// pathRepoTree identifies the path repo tree constant used by this package.
	pathRepoTree = "/api/v4/projects/42/repository/tree"
	// pathRepoCompare identifies the path repo compare constant used by this package.
	pathRepoCompare = "/api/v4/projects/42/repository/compare"
	// testReadmeName identifies the test readme name constant used by this package.
	testReadmeName = "README.md"
)

// TestRepositoryTree_Success verifies RepositoryTree when success.
func TestRepositoryTree_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTree {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":"abc1","name":"README.md","type":"blob","path":"README.md","mode":"100644"},
				{"id":"abc2","name":"src","type":"tree","path":"src","mode":"040000"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Tree(context.Background(), client, TreeInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf("Tree() unexpected error: %v", err)
	}
	if len(out.Tree) != 2 {
		t.Fatalf("len(Tree) = %d, want 2", len(out.Tree))
	}
	if out.Tree[0].Name != testReadmeName {
		t.Errorf("Tree[0].Name = %q, want %q", out.Tree[0].Name, testReadmeName)
	}
	if out.Tree[0].Type != "blob" {
		t.Errorf("Tree[0].Type = %q, want %q", out.Tree[0].Type, "blob")
	}
	if out.Tree[1].Type != "tree" {
		t.Errorf("Tree[1].Type = %q, want %q", out.Tree[1].Type, "tree")
	}
}

// TestRepositoryTree_WithOptions verifies RepositoryTree when with options.
func TestRepositoryTree_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTree {
			q := r.URL.Query()
			if q.Get("path") != "src" {
				t.Errorf("expected path=src, got %q", q.Get("path"))
			}
			if q.Get("ref") != "develop" {
				t.Errorf("expected ref=develop, got %q", q.Get("ref"))
			}
			if q.Get("recursive") != "true" {
				t.Errorf("expected recursive=true, got %q", q.Get("recursive"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":"abc3","name":"main.go","type":"blob","path":"src/main.go","mode":"100644"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Tree(context.Background(), client, TreeInput{
		ProjectID: "42",
		Path:      "src",
		Ref:       "develop",
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("Tree() unexpected error: %v", err)
	}
	if len(out.Tree) != 1 {
		t.Fatalf("len(Tree) = %d, want 1", len(out.Tree))
	}
	if out.Tree[0].Path != "src/main.go" {
		t.Errorf("Tree[0].Path = %q, want %q", out.Tree[0].Path, "src/main.go")
	}
}

// TestRepositoryTree_EmptyProjectID verifies RepositoryTree when empty project ID.
func TestRepositoryTree_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := Tree(context.Background(), client, TreeInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestRepositoryTree_APIError verifies RepositoryTree when API error.
func TestRepositoryTree_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := Tree(context.Background(), client, TreeInput{
		ProjectID: "999",
	})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryCompare_Success verifies RepositoryCompare when success.
func TestRepositoryCompare_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoCompare {
			testutil.RespondJSON(w, http.StatusOK, `{
				"commits":[
					{"id":"abc123","short_id":"abc123d","title":"feat: add file","author_name":"Test","committed_date":"2026-03-01T10:00:00Z","web_url":"https://gitlab.example.com/-/commit/abc123"}
				],
				"diffs":[
					{"old_path":"README.md","new_path":"README.md","diff":"@@ -1 +1,2 @@\n+hello","new_file":false,"renamed_file":false,"deleted_file":false}
				],
				"compare_timeout":false,
				"compare_same_ref":false,
				"web_url":"https://gitlab.example.com/-/compare/main...develop"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Compare(context.Background(), client, CompareInput{
		ProjectID: "42",
		From:      "main",
		To:        "develop",
	})
	if err != nil {
		t.Fatalf("Compare() unexpected error: %v", err)
	}
	if len(out.Commits) != 1 {
		t.Fatalf("len(Commits) = %d, want 1", len(out.Commits))
	}
	if out.Commits[0].Title != "feat: add file" {
		t.Errorf("Commits[0].Title = %q, want %q", out.Commits[0].Title, "feat: add file")
	}
	if len(out.Diffs) != 1 {
		t.Fatalf("len(Diffs) = %d, want 1", len(out.Diffs))
	}
	if out.Diffs[0].NewPath != testReadmeName {
		t.Errorf("Diffs[0].NewPath = %q, want %q", out.Diffs[0].NewPath, testReadmeName)
	}
	if out.WebURL == "" {
		t.Error("WebURL is empty")
	}
}

// TestRepositoryCompare_FromProjectID verifies Compare sets from_project_id
// when a non-zero FromProjectID is supplied and the API returns a valid result.
func TestRepositoryCompare_FromProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoCompare {
			q := r.URL.Query()
			if q.Get("from_project_id") != "99" {
				t.Errorf("expected from_project_id=99, got %q", q.Get("from_project_id"))
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"commits":[],
				"diffs":[],
				"compare_timeout":false,
				"compare_same_ref":false,
				"web_url":"https://gitlab.example.com/-/compare/main...develop"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Compare(context.Background(), client, CompareInput{
		ProjectID:     "42",
		From:          "main",
		To:            "develop",
		FromProjectID: 99,
	})
	if err != nil {
		t.Fatalf("Compare(FromProjectID=99) unexpected error: %v", err)
	}
	if out.WebURL == "" {
		t.Error("WebURL is empty")
	}
}

// TestRepositoryCompare_FromProjectIDZero verifies Compare omits from_project_id
// when FromProjectID is zero (default/unset).
func TestRepositoryCompare_FromProjectIDZero(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoCompare {
			if got := r.URL.Query().Get("from_project_id"); got != "" {
				t.Errorf("from_project_id should be absent, got %q", got)
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"commits":[],"diffs":[],"compare_timeout":false,"compare_same_ref":false,
				"web_url":"https://gitlab.example.com/-/compare/main...develop"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Compare(context.Background(), client, CompareInput{
		ProjectID: "42",
		From:      "main",
		To:        "develop",
	})
	if err != nil {
		t.Fatalf("Compare(FromProjectID=0) unexpected error: %v", err)
	}
}

// TestRepositoryCompare_EmptyProjectID verifies RepositoryCompare when empty project ID.
func TestRepositoryCompare_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Compare(context.Background(), client, CompareInput{
		From: "main",
		To:   "develop",
	})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestRepositoryCompare_APIError verifies RepositoryCompare when API error.
func TestRepositoryCompare_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := Compare(context.Background(), client, CompareInput{
		ProjectID: "999",
		From:      "main",
		To:        "develop",
	})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryTree_CancelledContext verifies RepositoryTree when cancelled context.
func TestRepositoryTree_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Tree(ctx, client, TreeInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryCompare_CancelledContext verifies RepositoryCompare when cancelled context.
func TestRepositoryCompare_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Compare(ctx, client, CompareInput{
		ProjectID: "42",
		From:      "main",
		To:        "develop",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Contributors
// ---------------------------------------------------------------------------.

// TestRepositoryContributors_Success verifies RepositoryContributors when success.
func TestRepositoryContributors_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/contributors" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"name":"Alice","email":"alice@example.com","commits":10,"additions":500,"deletions":100},
				{"name":"Bob","email":"bob@example.com","commits":5,"additions":200,"deletions":50}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Contributors(context.Background(), client, ContributorsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Contributors() unexpected error: %v", err)
	}
	if len(out.Contributors) != 2 {
		t.Fatalf("len(Contributors) = %d, want 2", len(out.Contributors))
	}
	if out.Contributors[0].Name != "Alice" {
		t.Errorf("Contributors[0].Name = %q, want %q", out.Contributors[0].Name, "Alice")
	}
	if out.Contributors[0].Commits != 10 {
		t.Errorf("Contributors[0].Commits = %d, want 10", out.Contributors[0].Commits)
	}
}

// TestRepositoryContributors_EmptyProjectID verifies RepositoryContributors when empty project ID.
func TestRepositoryContributors_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := Contributors(context.Background(), client, ContributorsInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// MergeBase
// ---------------------------------------------------------------------------.

// TestRepositoryMergeBase_Success verifies RepositoryMergeBase when success.
func TestRepositoryMergeBase_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/merge_base" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":"abc123def456","short_id":"abc123d","title":"Initial commit",
				"author_name":"Test","committed_date":"2026-01-01T00:00:00Z",
				"web_url":"https://gitlab.example.com/-/commit/abc123def456"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := MergeBase(context.Background(), client, MergeBaseInput{
		ProjectID: "42",
		Refs:      []string{"main", "develop"},
	})
	if err != nil {
		t.Fatalf("MergeBase() unexpected error: %v", err)
	}
	if out.ID != "abc123def456" {
		t.Errorf("MergeBase ID = %q, want %q", out.ID, "abc123def456")
	}
	if out.Title != "Initial commit" {
		t.Errorf("MergeBase Title = %q, want %q", out.Title, "Initial commit")
	}
}

// TestRepositoryMergeBase_EmptyProjectID verifies RepositoryMergeBase when empty project ID.
func TestRepositoryMergeBase_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := MergeBase(context.Background(), client, MergeBaseInput{Refs: []string{"main", "develop"}})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestRepositoryMerge_BaseTooFewRefs verifies RepositoryMerge when base too few refs.
func TestRepositoryMerge_BaseTooFewRefs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := MergeBase(context.Background(), client, MergeBaseInput{ProjectID: "42", Refs: []string{"main"}})
	if err == nil {
		t.Fatal("expected error for < 2 refs, got nil")
	}
}

// ---------------------------------------------------------------------------
// Blob
// ---------------------------------------------------------------------------.

// TestRepositoryBlob_Success verifies RepositoryBlob when success.
func TestRepositoryBlob_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/blobs/abc123" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello world"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Blob(context.Background(), client, BlobInput{ProjectID: "42", SHA: "abc123"})
	if err != nil {
		t.Fatalf("Blob() unexpected error: %v", err)
	}
	if out.SHA != "abc123" {
		t.Errorf("Blob SHA = %q, want %q", out.SHA, "abc123")
	}
	if out.Size != 11 {
		t.Errorf("Blob Size = %d, want 11", out.Size)
	}
	if out.Content == "" {
		t.Error("Blob Content is empty")
	}
}

// TestRepositoryBlob_EmptyProjectID verifies RepositoryBlob when empty project ID.
func TestRepositoryBlob_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Blob(context.Background(), client, BlobInput{SHA: "abc123"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// RawBlobContent
// ---------------------------------------------------------------------------.

// TestRepositoryRawBlobContent_Success verifies RepositoryRawBlobContent when success.
func TestRepositoryRawBlobContent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/blobs/abc123/raw" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("raw content here"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := RawBlobContent(context.Background(), client, BlobInput{ProjectID: "42", SHA: "abc123"})
	if err != nil {
		t.Fatalf("RawBlobContent() unexpected error: %v", err)
	}
	if out.SHA != "abc123" {
		t.Errorf("RawBlob SHA = %q, want %q", out.SHA, "abc123")
	}
	if out.Content != "raw content here" {
		t.Errorf("RawBlob Content = %q, want %q", out.Content, "raw content here")
	}
	if out.Size != 16 {
		t.Errorf("RawBlob Size = %d, want 16", out.Size)
	}
}

// TestRepositoryRawBlobContent_EmptyProjectID verifies RepositoryRawBlobContent when empty project ID.
func TestRepositoryRawBlobContent_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := RawBlobContent(context.Background(), client, BlobInput{SHA: "abc123"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Archive
// ---------------------------------------------------------------------------.

// TestRepositoryArchive_Success verifies RepositoryArchive when success.
func TestRepositoryArchive_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	out, err := Archive(context.Background(), client, ArchiveInput{ProjectID: "42", SHA: "main", Format: "zip"})
	if err != nil {
		t.Fatalf("Archive() unexpected error: %v", err)
	}
	if out.Format != "zip" {
		t.Errorf("Archive Format = %q, want %q", out.Format, "zip")
	}
	if out.URL == "" {
		t.Error("Archive URL is empty")
	}
	if out.SHA != "main" {
		t.Errorf("Archive SHA = %q, want %q", out.SHA, "main")
	}
}

// TestRepositoryArchive_DefaultFormat verifies RepositoryArchive when default format.
func TestRepositoryArchive_DefaultFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	out, err := Archive(context.Background(), client, ArchiveInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Archive() unexpected error: %v", err)
	}
	if out.Format != "tar.gz" {
		t.Errorf("Archive Format = %q, want %q", out.Format, "tar.gz")
	}
}

// TestRepositoryArchive_EmptyProjectID verifies RepositoryArchive when empty project ID.
func TestRepositoryArchive_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Archive(context.Background(), client, ArchiveInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// AddChangelog
// ---------------------------------------------------------------------------.

// TestRepositoryAddChangelog_Success verifies RepositoryAddChangelog when success.
func TestRepositoryAddChangelog_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/changelog" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddChangelog(context.Background(), client, AddChangelogInput{
		ProjectID: "42",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("AddChangelog() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("AddChangelog Success = false, want true")
	}
	if out.Version != "1.0.0" {
		t.Errorf("AddChangelog Version = %q, want %q", out.Version, "1.0.0")
	}
}

// TestRepositoryAddChangelog_EmptyProjectID verifies RepositoryAddChangelog when empty project ID.
func TestRepositoryAddChangelog_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := AddChangelog(context.Background(), client, AddChangelogInput{Version: "1.0.0"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestRepositoryAddChangelog_EmptyVersion verifies RepositoryAddChangelog when empty version.
func TestRepositoryAddChangelog_EmptyVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := AddChangelog(context.Background(), client, AddChangelogInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty version, got nil")
	}
}

// ---------------------------------------------------------------------------
// GenerateChangelogData
// ---------------------------------------------------------------------------.

// TestRepositoryGenerateChangelogData_Success verifies RepositoryGenerateChangelogData when success.
func TestRepositoryGenerateChangelogData_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/changelog" {
			testutil.RespondJSON(w, http.StatusOK, `{"notes":"## 1.0.0\n\n- feat: initial release\n"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GenerateChangelogData(context.Background(), client, GenerateChangelogInput{
		ProjectID: "42",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("GenerateChangelogData() unexpected error: %v", err)
	}
	if out.Notes == "" {
		t.Error("GenerateChangelogData Notes is empty")
	}
}

// TestRepositoryGenerateChangelogData_EmptyProjectID verifies RepositoryGenerateChangelogData when empty project ID.
func TestRepositoryGenerateChangelogData_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := GenerateChangelogData(context.Background(), client, GenerateChangelogInput{Version: "1.0.0"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestRepositoryGenerateChangelogData_EmptyVersion verifies RepositoryGenerateChangelogData when empty version.
func TestRepositoryGenerateChangelogData_EmptyVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := GenerateChangelogData(context.Background(), client, GenerateChangelogInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty version, got nil")
	}
}

// ---------------------------------------------------------------------------
// Canceled Context Tests
// ---------------------------------------------------------------------------.

// TestRepositoryContributors_CancelledContext verifies RepositoryContributors when cancelled context.
func TestRepositoryContributors_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Contributors(ctx, client, ContributorsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryMergeBase_CancelledContext verifies RepositoryMergeBase when cancelled context.
func TestRepositoryMergeBase_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := MergeBase(ctx, client, MergeBaseInput{ProjectID: "42", Refs: []string{"main", "dev"}})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryBlob_CancelledContext verifies RepositoryBlob when cancelled context.
func TestRepositoryBlob_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Blob(ctx, client, BlobInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryRawBlobContent_CancelledContext verifies RepositoryRawBlobContent when cancelled context.
func TestRepositoryRawBlobContent_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := RawBlobContent(ctx, client, BlobInput{ProjectID: "42", SHA: "abc"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryArchive_CancelledContext verifies RepositoryArchive when cancelled context.
func TestRepositoryArchive_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Archive(ctx, client, ArchiveInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryAddChangelog_CancelledContext verifies RepositoryAddChangelog when cancelled context.
func TestRepositoryAddChangelog_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddChangelog(ctx, client, AddChangelogInput{ProjectID: "42", Version: "1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRepositoryGenerateChangelogData_CancelledContext verifies RepositoryGenerateChangelogData when cancelled context.
func TestRepositoryGenerateChangelogData_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"notes":""}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GenerateChangelogData(ctx, client, GenerateChangelogInput{ProjectID: "42", Version: "1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// API Error Tests
// ---------------------------------------------------------------------------.

// TestRepositoryContributors_APIError verifies RepositoryContributors when API error.
func TestRepositoryContributors_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Contributors(context.Background(), client, ContributorsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryMergeBase_APIError verifies RepositoryMergeBase when API error.
func TestRepositoryMergeBase_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := MergeBase(context.Background(), client, MergeBaseInput{ProjectID: "42", Refs: []string{"main", "dev"}})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryBlob_APIError verifies RepositoryBlob when API error.
func TestRepositoryBlob_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Blob Not Found"}`)
	}))
	_, err := Blob(context.Background(), client, BlobInput{ProjectID: "42", SHA: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryRawBlobContent_APIError verifies RepositoryRawBlobContent when API error.
func TestRepositoryRawBlobContent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Blob Not Found"}`)
	}))
	_, err := RawBlobContent(context.Background(), client, BlobInput{ProjectID: "42", SHA: "bad"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryAddChangelog_APIError verifies RepositoryAddChangelog when API error.
func TestRepositoryAddChangelog_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))
	_, err := AddChangelog(context.Background(), client, AddChangelogInput{ProjectID: "42", Version: "1.0.0"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestRepositoryGenerateChangelogData_APIError verifies RepositoryGenerateChangelogData when API error.
func TestRepositoryGenerateChangelogData_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))
	_, err := GenerateChangelogData(context.Background(), client, GenerateChangelogInput{ProjectID: "42", Version: "1.0.0"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------------------------------------------------------------------------
// Handler Edge Cases (optional fields, query parameters)
// ---------------------------------------------------------------------------.

// TestRepositoryCompare_WithOptions verifies RepositoryCompare when with options.
func TestRepositoryCompare_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoCompare {
			q := r.URL.Query()
			if q.Get("straight") != "true" {
				t.Errorf("expected straight=true, got %q", q.Get("straight"))
			}
			if q.Get("unidiff") != "true" {
				t.Errorf("expected unidiff=true, got %q", q.Get("unidiff"))
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"commits":[],"diffs":[],
				"compare_timeout":false,"compare_same_ref":false,
				"web_url":"https://example.com"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Compare(context.Background(), client, CompareInput{
		ProjectID: "42",
		From:      "main",
		To:        "develop",
		Straight:  true,
		Unidiff:   true,
	})
	if err != nil {
		t.Fatalf("Compare() unexpected error: %v", err)
	}
	if out.WebURL != "https://example.com" {
		t.Errorf("WebURL = %q, want %q", out.WebURL, "https://example.com")
	}
}

// TestRepositoryContributors_WithOptions verifies RepositoryContributors when with options.
func TestRepositoryContributors_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/contributors" {
			q := r.URL.Query()
			if q.Get("order_by") != "name" {
				t.Errorf("expected order_by=name, got %q", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("expected sort=desc, got %q", q.Get("sort"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"name":"Z","email":"z@test.com","commits":1,"additions":0,"deletions":0}
			]`, testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "11", TotalPages: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	input := ContributorsInput{
		ProjectID: "42",
		OrderBy:   "name",
		Sort:      "desc",
	}
	input.Page = 2
	input.PerPage = 10

	out, err := Contributors(context.Background(), client, input)
	if err != nil {
		t.Fatalf("Contributors() unexpected error: %v", err)
	}
	if len(out.Contributors) != 1 {
		t.Fatalf("len(Contributors) = %d, want 1", len(out.Contributors))
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
}

// TestRepositoryAddChangelog_WithOptions verifies RepositoryAddChangelog when with options.
func TestRepositoryAddChangelog_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/changelog" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddChangelog(context.Background(), client, AddChangelogInput{
		ProjectID:  "42",
		Version:    "2.0.0",
		Branch:     "develop",
		ConfigFile: ".changelog.yml",
		File:       "CHANGELOG.md",
		From:       "v1.0.0",
		To:         "v2.0.0",
		Message:    "Update changelog",
		Trailer:    "Changelog",
	})
	if err != nil {
		t.Fatalf("AddChangelog() unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("AddChangelog Success = false, want true")
	}
	if out.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", out.Version, "2.0.0")
	}
}

// TestRepositoryGenerateChangelogData_WithOptions verifies RepositoryGenerateChangelogData when with options.
func TestRepositoryGenerateChangelogData_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/changelog" {
			testutil.RespondJSON(w, http.StatusOK, `{"notes":"## 2.0.0\n\n- feat: stuff\n"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GenerateChangelogData(context.Background(), client, GenerateChangelogInput{
		ProjectID:  "42",
		Version:    "2.0.0",
		ConfigFile: ".changelog.yml",
		From:       "v1.0.0",
		To:         "v2.0.0",
		Trailer:    "Changelog",
	})
	if err != nil {
		t.Fatalf("GenerateChangelogData() unexpected error: %v", err)
	}
	if out.Notes == "" {
		t.Error("Notes is empty")
	}
}

// TestRepositoryArchive_WithPath verifies RepositoryArchive when with path.
func TestRepositoryArchive_WithPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	out, err := Archive(context.Background(), client, ArchiveInput{
		ProjectID: "42",
		SHA:       "main",
		Format:    "zip",
		Path:      "src/",
	})
	if err != nil {
		t.Fatalf("Archive() unexpected error: %v", err)
	}
	if !strings.Contains(out.URL, "sha=main") {
		t.Errorf("URL should contain sha=main, got %q", out.URL)
	}
	if out.Format != "zip" {
		t.Errorf("Format = %q, want %q", out.Format, "zip")
	}
}

// TestRepositoryTree_WithPagination verifies RepositoryTree when with pagination.
func TestRepositoryTree_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTree {
			q := r.URL.Query()
			if q.Get("page") != "3" {
				t.Errorf("expected page=3, got %q", q.Get("page"))
			}
			if q.Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %q", q.Get("per_page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":"x","name":"a.go","type":"blob","path":"a.go","mode":"100644"}
			]`, testutil.PaginationHeaders{Page: "3", PerPage: "5", Total: "15", TotalPages: "3"})
			return
		}
		http.NotFound(w, r)
	}))

	input := TreeInput{ProjectID: "42"}
	input.Page = 3
	input.PerPage = 5

	out, err := Tree(context.Background(), client, input)
	if err != nil {
		t.Fatalf("Tree() unexpected error: %v", err)
	}
	if len(out.Tree) != 1 {
		t.Fatalf("len(Tree) = %d, want 1", len(out.Tree))
	}
	if out.Pagination.Page != 3 {
		t.Errorf("Pagination.Page = %d, want 3", out.Pagination.Page)
	}
}

// ---------------------------------------------------------------------------
// Format*Markdown Tests
// ---------------------------------------------------------------------------.

// TestFormatTreeMarkdown verifies FormatTreeMarkdown.
func TestFormatTreeMarkdown(t *testing.T) {
	out := TreeOutput{
		Tree: []TreeNodeOutput{
			{ID: "a", Name: "README.md", Type: "blob", Path: "README.md", Mode: "100644"},
			{ID: "b", Name: "src", Type: "tree", Path: "src", Mode: "040000"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatTreeMarkdown(out)
	if !strings.Contains(md, "Repository Tree (2 entries)") {
		t.Errorf("expected header with count, got:\n%s", md)
	}
	if !strings.Contains(md, "README.md") {
		t.Error("expected README.md in output")
	}
	if !strings.Contains(md, "src") {
		t.Error("expected src in output")
	}
}

// TestFormatTreeMarkdown_Empty verifies FormatTreeMarkdown when empty.
func TestFormatTreeMarkdown_Empty(t *testing.T) {
	out := TreeOutput{
		Tree:       nil,
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	md := FormatTreeMarkdown(out)
	if !strings.Contains(md, "No files or directories found") {
		t.Errorf("expected 'No files or directories found', got:\n%s", md)
	}
}

// TestFormatCompareMarkdown verifies FormatCompareMarkdown.
func TestFormatCompareMarkdown(t *testing.T) {
	out := CompareOutput{
		Commits: []commits.Output{
			{ShortID: "abc1", Title: "feat: init", AuthorName: "Alice"},
		},
		Diffs: []DiffOutput{
			{OldPath: "a.go", NewPath: "a.go", NewFile: false, DeletedFile: false, RenamedFile: false},
		},
		WebURL: "https://gitlab.example.com/-/compare/main...develop",
	}
	md := FormatCompareMarkdown(out)
	if !strings.Contains(md, "Repository Compare") {
		t.Error("expected header")
	}
	if !strings.Contains(md, "abc1") {
		t.Error("expected short_id in output")
	}
	if !strings.Contains(md, "a.go") {
		t.Error("expected file path in output")
	}
	if !strings.Contains(md, "modified") {
		t.Error("expected 'modified' status")
	}
	if !strings.Contains(md, "https://gitlab.example.com") {
		t.Error("expected web URL")
	}
}

// TestFormatCompareMarkdown_NewDeletedRenamed verifies FormatCompareMarkdown when new deleted renamed.
func TestFormatCompareMarkdown_NewDeletedRenamed(t *testing.T) {
	out := CompareOutput{
		Diffs: []DiffOutput{
			{NewPath: "new.go", NewFile: true},
			{NewPath: "old.go", DeletedFile: true},
			{NewPath: "moved.go", RenamedFile: true},
		},
	}
	md := FormatCompareMarkdown(out)
	if !strings.Contains(md, "added") {
		t.Error("expected 'added' status for new file")
	}
	if !strings.Contains(md, "deleted") {
		t.Error("expected 'deleted' status")
	}
	if !strings.Contains(md, "renamed") {
		t.Error("expected 'renamed' status")
	}
}

// TestFormatCompareMarkdown_SameRef verifies FormatCompareMarkdown when same ref.
func TestFormatCompareMarkdown_SameRef(t *testing.T) {
	out := CompareOutput{CompareSameRef: true}
	md := FormatCompareMarkdown(out)
	if !strings.Contains(md, "same ref") {
		t.Errorf("expected 'same ref' message, got:\n%s", md)
	}
}

// TestFormatCompareMarkdown_Timeout verifies FormatCompareMarkdown when timeout.
func TestFormatCompareMarkdown_Timeout(t *testing.T) {
	out := CompareOutput{CompareTimeout: true}
	md := FormatCompareMarkdown(out)
	if !strings.Contains(md, "timeout") {
		t.Errorf("expected 'timeout' message, got:\n%s", md)
	}
}

// TestFormatContributorsMarkdown verifies FormatContributorsMarkdown.
func TestFormatContributorsMarkdown(t *testing.T) {
	out := ContributorsOutput{
		Contributors: []ContributorOutput{
			{Name: "Alice", Email: "alice@test.com", Commits: 10, Additions: 500, Deletions: 100},
			{Name: "Bob", Email: "bob@test.com", Commits: 5, Additions: 200, Deletions: 50},
		},
	}
	md := FormatContributorsMarkdown(out)
	if !strings.Contains(md, "Repository Contributors (2)") {
		t.Errorf("expected header with count, got:\n%s", md)
	}
	if !strings.Contains(md, "Alice") {
		t.Error("expected Alice in output")
	}
	if !strings.Contains(md, "Bob") {
		t.Error("expected Bob in output")
	}
}

// TestFormatContributorsMarkdown_Empty verifies FormatContributorsMarkdown when empty.
func TestFormatContributorsMarkdown_Empty(t *testing.T) {
	out := ContributorsOutput{Contributors: nil}
	md := FormatContributorsMarkdown(out)
	if !strings.Contains(md, "No contributors found") {
		t.Errorf("expected 'No contributors found', got:\n%s", md)
	}
}

// TestFormatBlobMarkdown verifies FormatBlobMarkdown.
func TestFormatBlobMarkdown(t *testing.T) {
	out := BlobOutput{SHA: "abc123", Size: 1024, Content: "base64data"}
	md := FormatBlobMarkdown(out)
	if !strings.Contains(md, "Repository Blob") {
		t.Error("expected header")
	}
	if !strings.Contains(md, "abc123") {
		t.Error("expected SHA in output")
	}
	if !strings.Contains(md, "1024 bytes") {
		t.Error("expected size in output")
	}
}

// TestFormatRawBlobContentMarkdown verifies FormatRawBlobContentMarkdown.
func TestFormatRawBlobContentMarkdown(t *testing.T) {
	out := RawBlobContentOutput{SHA: "def456", Size: 42, Content: "hello world"}
	md := FormatRawBlobContentMarkdown(out)
	if !strings.Contains(md, "Raw Blob Content") {
		t.Error("expected header")
	}
	if !strings.Contains(md, "def456") {
		t.Error("expected SHA in output")
	}
	if !strings.Contains(md, "hello world") {
		t.Error("expected content in output")
	}
}

// TestMarkdownForResult_BlobContentCategories verifies blob result formatters
// preserve text metadata and attach image content for multimodal clients.
func TestMarkdownForResult_BlobContentCategories(t *testing.T) {
	tests := []struct {
		name          string
		result        any
		wantText      string
		wantImageMIME string
	}{
		{
			name:     "blob text",
			result:   BlobOutput{SHA: "textsha", Size: 4, Content: "text", ContentCategory: "text"},
			wantText: "Content**: text",
		},
		{
			name:     "blob binary",
			result:   BlobOutput{SHA: "binsha", Size: 4, ContentCategory: "binary"},
			wantText: "content omitted",
		},
		{
			name:          "blob image",
			result:        BlobOutput{SHA: "imgsha", Size: 4, ContentCategory: "image", ImageData: []byte{0x89, 0x50}, ImageMIMEType: "image/png"},
			wantText:      "image (image/png)",
			wantImageMIME: "image/png",
		},
		{
			name:     "raw text",
			result:   RawBlobContentOutput{SHA: "rawtext", Size: 4, Content: "text", ContentCategory: "text"},
			wantText: "text",
		},
		{
			name:     "raw binary",
			result:   RawBlobContentOutput{SHA: "rawbin", Size: 4, ContentCategory: "binary"},
			wantText: "content omitted",
		},
		{
			name:          "raw image",
			result:        RawBlobContentOutput{SHA: "rawimg", Size: 4, ContentCategory: "image", ImageData: []byte{0x89, 0x50}, ImageMIMEType: "image/png"},
			wantText:      "image (image/png)",
			wantImageMIME: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertBlobMarkdownResult(t, tt.result, tt.wantText, tt.wantImageMIME)
		})
	}
}

func assertBlobMarkdownResult(t *testing.T, result any, wantText, wantImageMIME string) {
	t.Helper()
	callResult := toolutil.MarkdownForResult(result)
	if callResult == nil {
		t.Fatal("MarkdownForResult returned nil")
	}
	content, ok := callResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want *mcp.TextContent", callResult.Content[0])
	}
	if !strings.Contains(content.Text, wantText) {
		t.Fatalf("markdown missing %q:\n%s", wantText, content.Text)
	}
	assertBlobImageContent(t, callResult, wantImageMIME)
}

func assertBlobImageContent(t *testing.T, callResult *mcp.CallToolResult, wantImageMIME string) {
	t.Helper()
	if wantImageMIME == "" {
		if len(callResult.Content) != 1 {
			t.Fatalf("content items = %d, want 1", len(callResult.Content))
		}
		return
	}
	if len(callResult.Content) != 2 {
		t.Fatalf("content items = %d, want 2", len(callResult.Content))
	}
	image, ok := callResult.Content[1].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content[1] = %T, want *mcp.ImageContent", callResult.Content[1])
	}
	if image.MIMEType != wantImageMIME {
		t.Fatalf("image MIMEType = %q, want %q", image.MIMEType, wantImageMIME)
	}
}

// TestClassifyBlobContent verifies MIME sniffing maps blobs to the content
// categories consumed by Markdown and multimodal result formatters.
func TestClassifyBlobContent(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		mime          string
		wantCategory  string
		wantContent   string
		wantImageMIME string
	}{
		{name: "image", data: []byte{0x89, 0x50}, mime: "image/png", wantCategory: "image", wantImageMIME: "image/png"},
		{name: "text", data: []byte("hello"), mime: "text/plain; charset=utf-8", wantCategory: "text", wantContent: "hello"},
		{name: "binary", data: []byte{0x00, 0x01}, mime: "application/octet-stream", wantCategory: "binary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, content, imageData, imageMIME := classifyBlobContent(tt.data, tt.mime)
			if category != tt.wantCategory {
				t.Fatalf("category = %q, want %q", category, tt.wantCategory)
			}
			if content != tt.wantContent {
				t.Fatalf("content = %q, want %q", content, tt.wantContent)
			}
			if imageMIME != tt.wantImageMIME {
				t.Fatalf("image MIME = %q, want %q", imageMIME, tt.wantImageMIME)
			}
			if tt.wantCategory == "image" && string(imageData) != string(tt.data) {
				t.Fatalf("image data = %v, want %v", imageData, tt.data)
			}
		})
	}
}

// TestFormatArchiveMarkdown verifies FormatArchiveMarkdown.
func TestFormatArchiveMarkdown(t *testing.T) {
	out := ArchiveOutput{ProjectID: "42", SHA: "main", Format: "zip", URL: "https://example.com/archive.zip"}
	md := FormatArchiveMarkdown(out)
	if !strings.Contains(md, "Repository Archive") {
		t.Error("expected header")
	}
	if !strings.Contains(md, "zip") {
		t.Error("expected format in output")
	}
	if !strings.Contains(md, "main") {
		t.Error("expected SHA/Ref in output")
	}
	if !strings.Contains(md, "https://example.com/archive.zip") {
		t.Error("expected URL in output")
	}
}

// TestFormatArchiveMarkdown_NoSHA verifies FormatArchiveMarkdown when no SHA.
func TestFormatArchiveMarkdown_NoSHA(t *testing.T) {
	out := ArchiveOutput{ProjectID: "42", Format: "tar.gz", URL: "https://example.com/archive.tar.gz"}
	md := FormatArchiveMarkdown(out)
	if strings.Contains(md, "SHA/Ref") {
		t.Error("expected no SHA/Ref line when SHA is empty")
	}
}

// TestFormatAddChangelogMarkdown_Success verifies FormatAddChangelogMarkdown when success.
func TestFormatAddChangelogMarkdown_Success(t *testing.T) {
	out := AddChangelogOutput{Success: true, Version: "1.0.0"}
	md := FormatAddChangelogMarkdown(out)
	if !strings.Contains(md, "Changelog Updated") {
		t.Error("expected success header")
	}
	if !strings.Contains(md, "1.0.0") {
		t.Error("expected version in output")
	}
}

// TestFormatAddChangelogMarkdown_Failure verifies FormatAddChangelogMarkdown when failure.
func TestFormatAddChangelogMarkdown_Failure(t *testing.T) {
	out := AddChangelogOutput{Success: false}
	md := FormatAddChangelogMarkdown(out)
	if !strings.Contains(md, "Failed") {
		t.Error("expected failure header")
	}
}

// TestFormatChangelogDataMarkdown verifies FormatChangelogDataMarkdown.
func TestFormatChangelogDataMarkdown(t *testing.T) {
	out := ChangelogDataOutput{Notes: "## 1.0.0\n\n- feat: initial release\n"}
	md := FormatChangelogDataMarkdown(out)
	if !strings.Contains(md, "Generated Changelog Data") {
		t.Error("expected header")
	}
	if !strings.Contains(md, "1.0.0") {
		t.Error("expected notes content")
	}
}

// TestFormatChangelogDataMarkdown_Empty verifies FormatChangelogDataMarkdown when empty.
func TestFormatChangelogDataMarkdown_Empty(t *testing.T) {
	out := ChangelogDataOutput{Notes: ""}
	md := FormatChangelogDataMarkdown(out)
	if !strings.Contains(md, "No changelog entries found") {
		t.Errorf("expected 'No changelog entries found', got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs Tests
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for repository actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := repositorySpecsByTool(t, specs)

	if len(specs) != 9 {
		t.Fatalf("len(ActionSpecs) = %d, want 9", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "repository" {
			t.Errorf("OwnerPackage for %s = %q, want repository", spec.Name, spec.OwnerPackage)
		}
	}

	tree := byTool["gitlab_repository_tree"]
	if tree.Usage == "" || len(tree.Aliases) == 0 || len(tree.ParameterGuidance) == 0 {
		t.Fatalf("gitlab_repository_tree metadata incomplete: usage=%q aliases=%d guidance=%d", tree.Usage, len(tree.Aliases), len(tree.ParameterGuidance))
	}
	if tree.IndividualTool.Description == "" {
		t.Fatal("gitlab_repository_tree description is empty")
	}

	blob := byTool["gitlab_repository_blob"]
	if blob.Usage == "" || len(blob.Aliases) == 0 {
		t.Fatalf("gitlab_repository_blob metadata incomplete: usage=%q aliases=%d", blob.Usage, len(blob.Aliases))
	}

	changelogAdd := byTool["gitlab_repository_changelog_add"]
	if changelogAdd.Usage == "" || len(changelogAdd.Aliases) == 0 {
		t.Fatalf("gitlab_repository_changelog_add metadata incomplete: usage=%q aliases=%d", changelogAdd.Usage, len(changelogAdd.Aliases))
	}
}

// newRepositorySpecsByTool constructs repository specs by tool test fixtures.
func newRepositorySpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/tree"):
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":"a","name":"f.go","type":"blob","path":"f.go","mode":"100644"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/compare"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"commits":[{"id":"c1","short_id":"c1s","title":"t","author_name":"A","committed_date":"2026-01-01T00:00:00Z","web_url":"u"}],
				"diffs":[],"compare_timeout":false,"compare_same_ref":false,"web_url":"u"
			}`)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/contributors"):
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"name":"A","email":"a@t.com","commits":1,"additions":0,"deletions":0}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/merge_base"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":"mb1","short_id":"mb","title":"base","author_name":"A",
				"committed_date":"2026-01-01T00:00:00Z","web_url":"u"
			}`)

		case r.Method == http.MethodGet && strings.Contains(path, "/repository/blobs/") && strings.HasSuffix(path, "/raw"):
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("raw"))

		case r.Method == http.MethodGet && strings.Contains(path, "/repository/blobs/"):
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("blob"))

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/repository/changelog"):
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/changelog"):
			testutil.RespondJSON(w, http.StatusOK, `{"notes":"## 1.0.0\n\n- feat: init\n"}`)

		default:
			http.NotFound(w, r)
		}
	}))
	return repositorySpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_CallAllRoutes validates all repository routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	specs := newRepositorySpecsByTool(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_repository_tree", map[string]any{"project_id": "42"}},
		{"gitlab_repository_compare", map[string]any{"project_id": "42", "from": "main", "to": "dev"}},
		{"gitlab_repository_contributors", map[string]any{"project_id": "42"}},
		{"gitlab_repository_merge_base", map[string]any{"project_id": "42", "refs": []any{"main", "dev"}}},
		{"gitlab_repository_blob", map[string]any{"project_id": "42", "sha": "abc"}},
		{"gitlab_repository_raw_blob", map[string]any{"project_id": "42", "sha": "abc"}},
		{"gitlab_repository_archive", map[string]any{"project_id": "42"}},
		{"gitlab_repository_changelog_add", map[string]any{"project_id": "42", "version": "1.0.0"}},
		{"gitlab_repository_changelog_generate", map[string]any{"project_id": "42", "version": "1.0.0"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := specs[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// repositorySpecsByTool supports repository specs by tool assertions in repository tests.
func repositorySpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
