// files_test.go contains unit tests for GitLab repository file operations
// (get, create, update, delete, blame, metadata, raw). Tests use httptest to
// mock the GitLab Repository Files API and verify success and error paths.

package files

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// Test fixture values used across file operation tests.
const (
	errExpEmptyProjectID = "expected error for empty project_id, got nil"
	errExpectedAPI       = "expected API error, got nil"
	testFileMainGo       = "main.go"
)

// TestFileGet_Success verifies that fileGet retrieves a file and automatically
// decodes its base64-encoded content. The mock returns a valid file JSON
// response with base64 content, and the test asserts the decoded content
// matches the original.
func TestFileGet_Success(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(content))

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/main.go" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"main.go",
				"file_path":"main.go",
				"size":30,
				"encoding":"base64",
				"content":"`+b64+`",
				"ref":"main",
				"blob_id":"blob123",
				"commit_id":"abc123",
				"last_commit_id":"abc123"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  testFileMainGo,
		Ref:       "main",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.FileName != testFileMainGo {
		t.Errorf("out.FileName = %q, want %q", out.FileName, testFileMainGo)
	}
	if out.Content != content {
		t.Errorf("out.Content = %q, want %q", out.Content, content)
	}
	if out.Ref != "main" {
		t.Errorf("out.Ref = %q, want %q", out.Ref, "main")
	}
}

// TestFileGet_NotFound verifies that fileGet returns an error when the
// requested file does not exist in the repository. The mock returns HTTP 404.
func TestFileGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 File Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "nonexistent.go",
		Ref:       "main",
	})
	if err == nil {
		t.Fatal("Get() expected error for missing file, got nil")
	}
}

// TestFileGet_NestedPath verifies that fileGet correctly handles URL-encoded
// nested file paths (e.g., "src%2Futils%2Fhelpers.go"). The mock returns a
// file at a nested path, and the test asserts the decoded path and content.
func TestFileGet_NestedPath(t *testing.T) {
	content := "package utils\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(content))

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/src%2Futils%2Fhelpers.go" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"helpers.go",
				"file_path":"src/utils/helpers.go",
				"size":15,
				"encoding":"base64",
				"content":"`+b64+`",
				"ref":"develop",
				"blob_id":"blob456",
				"commit_id":"def456",
				"last_commit_id":"def456"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "src%2Futils%2Fhelpers.go",
		Ref:       "develop",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.FilePath != "src/utils/helpers.go" {
		t.Errorf("out.FilePath = %q, want %q", out.FilePath, "src/utils/helpers.go")
	}
	if out.Content != content {
		t.Errorf("out.Content = %q, want %q", out.Content, content)
	}
}

// TestFileGet_ImageFile verifies that fileGet detects image files by extension,
// stores raw bytes in ImageData, empties Content, and sets ContentCategory="image".
func TestFileGet_ImageFile(t *testing.T) {
	rawPNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic bytes
	b64 := base64.StdEncoding.EncodeToString(rawPNG)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "logo.png") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"logo.png",
				"file_path":"assets/logo.png",
				"size":8,
				"encoding":"base64",
				"content":"`+b64+`",
				"ref":"main",
				"blob_id":"blobimg",
				"commit_id":"abc123",
				"last_commit_id":"abc123"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "assets%2Flogo.png",
		Ref:       "main",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ContentCategory != "image" {
		t.Errorf("ContentCategory = %q, want %q", out.ContentCategory, "image")
	}
	if out.ImageMIMEType != "image/png" {
		t.Errorf("ImageMIMEType = %q, want %q", out.ImageMIMEType, "image/png")
	}
	if len(out.ImageData) != len(rawPNG) {
		t.Errorf("ImageData length = %d, want %d", len(out.ImageData), len(rawPNG))
	}
	if out.Content != "" {
		t.Errorf("Content should be empty for images, got %q", out.Content)
	}
}

// TestFileGet_BinaryFile verifies that fileGet detects binary files by extension,
// empties Content, and sets ContentCategory="binary".
func TestFileGet_BinaryFile(t *testing.T) {
	rawPDF := []byte("%PDF-1.4 fake content")
	b64 := base64.StdEncoding.EncodeToString(rawPDF)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "doc.pdf") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"doc.pdf",
				"file_path":"docs/doc.pdf",
				"size":21,
				"encoding":"base64",
				"content":"`+b64+`",
				"ref":"main",
				"blob_id":"blobpdf",
				"commit_id":"abc123",
				"last_commit_id":"abc123"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "docs%2Fdoc.pdf",
		Ref:       "main",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ContentCategory != "binary" {
		t.Errorf("ContentCategory = %q, want %q", out.ContentCategory, "binary")
	}
	if out.Content != "" {
		t.Errorf("Content should be empty for binary files, got %q", out.Content)
	}
	if len(out.ImageData) != 0 {
		t.Errorf("ImageData should be empty for non-image binary, got %d bytes", len(out.ImageData))
	}
}

// TestFileGet_TextContentCategory verifies that regular text files get
// ContentCategory="text" and ImageData remains nil.
func TestFileGet_TextContentCategory(t *testing.T) {
	content := "package main\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(content))

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"file_name":"main.go",
			"file_path":"main.go",
			"size":14,
			"encoding":"base64",
			"content":"`+b64+`",
			"ref":"main",
			"blob_id":"blobtxt",
			"commit_id":"abc123",
			"last_commit_id":"abc123"
		}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", FilePath: "main.go"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ContentCategory != "text" {
		t.Errorf("ContentCategory = %q, want %q", out.ContentCategory, "text")
	}
	if out.Content != content {
		t.Errorf("Content = %q, want %q", out.Content, content)
	}
	if out.ImageData != nil {
		t.Error("ImageData should be nil for text files")
	}
}

// ---------------------------------------------------------------------------
// CreateFile
// ---------------------------------------------------------------------------.

// TestFileCreate_Success verifies the behavior of file create success.
func TestFileCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/files/new_file.txt" {
			testutil.RespondJSON(w, http.StatusCreated, `{"file_path":"new_file.txt","branch":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "42",
		FilePath:      "new_file.txt",
		Branch:        "main",
		Content:       "hello",
		CommitMessage: "add file",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.FilePath != "new_file.txt" {
		t.Errorf("FilePath = %q, want %q", out.FilePath, "new_file.txt")
	}
	if out.Branch != "main" {
		t.Errorf("Branch = %q, want %q", out.Branch, "main")
	}
}

// TestFileCreate_EmptyProjectID verifies the behavior of file create empty project i d.
func TestFileCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{Branch: "main", CommitMessage: "x"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestFileCreate_MissingBranch verifies the behavior of file create missing branch.
func TestFileCreate_MissingBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", CommitMessage: "x"})
	if err == nil {
		t.Fatal("expected error for empty branch, got nil")
	}
}

// ---------------------------------------------------------------------------
// UpdateFile
// ---------------------------------------------------------------------------.

// TestFileUpdate_Success verifies the behavior of file update success.
func TestFileUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/repository/files/main.go" {
			testutil.RespondJSON(w, http.StatusOK, `{"file_path":"main.go","branch":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "42",
		FilePath:      testFileMainGo,
		Branch:        "main",
		Content:       "updated",
		CommitMessage: "update file",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.FilePath != testFileMainGo {
		t.Errorf("FilePath = %q, want %q", out.FilePath, testFileMainGo)
	}
}

// TestFileUpdate_EmptyProjectID verifies the behavior of file update empty project i d.
func TestFileUpdate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{Branch: "main", CommitMessage: "x"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// DeleteFile
// ---------------------------------------------------------------------------.

// TestFileDelete_Success verifies the behavior of file delete success.
func TestFileDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/repository/files/old_file.txt" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "42",
		FilePath:      "old_file.txt",
		Branch:        "main",
		CommitMessage: "delete file",
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestFileDelete_EmptyProjectID verifies the behavior of file delete empty project i d.
func TestFileDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{Branch: "main", CommitMessage: "x"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetFileBlame
// ---------------------------------------------------------------------------.

// TestFileBlame_Success verifies the behavior of file blame success.
func TestFileBlame_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/main.go/blame" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{
					"commit":{"id":"abc123","message":"initial","author_name":"Alice","author_email":"alice@test.com","authored_date":"2026-01-01T00:00:00Z","committed_date":"2026-01-01T00:00:00Z"},
					"lines":["package main","","func main() {}"]
				}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Blame(context.Background(), client, BlameInput{
		ProjectID: "42",
		FilePath:  testFileMainGo,
	})
	if err != nil {
		t.Fatalf("Blame() unexpected error: %v", err)
	}
	if len(out.Ranges) != 1 {
		t.Fatalf("len(Ranges) = %d, want 1", len(out.Ranges))
	}
	if out.Ranges[0].Commit.AuthorName != "Alice" {
		t.Errorf("AuthorName = %q, want %q", out.Ranges[0].Commit.AuthorName, "Alice")
	}
	if len(out.Ranges[0].Lines) != 3 {
		t.Errorf("len(Lines) = %d, want 3", len(out.Ranges[0].Lines))
	}
}

// TestFileBlame_EmptyProjectID verifies the behavior of file blame empty project i d.
func TestFileBlame_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := Blame(context.Background(), client, BlameInput{FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetFileMetaData
// ---------------------------------------------------------------------------.

// TestFileMetaData_Success verifies the behavior of file meta data success.
func TestFileMetaData_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/files/main.go" && (r.Method == http.MethodHead || r.Method == http.MethodGet) {
			w.Header().Set("X-Gitlab-File-Name", "main.go")
			w.Header().Set("X-Gitlab-File-Path", "main.go")
			w.Header().Set("X-Gitlab-Size", "30")
			w.Header().Set("X-Gitlab-Blob-Id", "blob123")
			w.Header().Set("X-Gitlab-Commit-Id", "abc123")
			w.Header().Set("X-Gitlab-Last-Commit-Id", "abc123")
			w.Header().Set("X-Gitlab-Content-Sha256", "sha256hash")
			w.Header().Set("X-Gitlab-Encoding", "base64")
			w.Header().Set("X-Gitlab-Ref", "main")
			w.Header().Set("X-Gitlab-Execute-Filemode", "false")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetMetaData(context.Background(), client, MetaDataInput{
		ProjectID: "42",
		FilePath:  testFileMainGo,
	})
	if err != nil {
		t.Fatalf("GetMetaData() unexpected error: %v", err)
	}
	if out.FileName != testFileMainGo {
		t.Errorf("FileName = %q, want %q", out.FileName, testFileMainGo)
	}
	if out.Size != 30 {
		t.Errorf("Size = %d, want 30", out.Size)
	}
	if out.SHA256 != "sha256hash" {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, "sha256hash")
	}
}

// TestFileMetaData_EmptyProjectID verifies the behavior of file meta data empty project i d.
func TestFileMetaData_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := GetMetaData(context.Background(), client, MetaDataInput{FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// GetRawFile
// ---------------------------------------------------------------------------.

// TestFileGetRaw_Success verifies the behavior of file get raw success.
func TestFileGetRaw_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/main.go/raw" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("package main\n\nfunc main() {}\n"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRaw(context.Background(), client, RawInput{
		ProjectID: "42",
		FilePath:  testFileMainGo,
	})
	if err != nil {
		t.Fatalf("GetRaw() unexpected error: %v", err)
	}
	if out.Content != "package main\n\nfunc main() {}\n" {
		t.Errorf("Content = %q, want %q", out.Content, "package main\n\nfunc main() {}\n")
	}
	if out.Size != 29 {
		t.Errorf("Size = %d, want 29", out.Size)
	}
}

// TestFileGetRaw_EmptyProjectID verifies the behavior of file get raw empty project i d.
func TestFileGetRaw_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := GetRaw(context.Background(), client, RawInput{FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestFileGetRaw_ImageFile verifies that GetRaw detects image files,
// stores raw bytes in ImageData, and sets ContentCategory="image".
func TestFileGetRaw_ImageFile(t *testing.T) {
	rawPNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "icon.png") {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.Write(rawPNG)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRaw(context.Background(), client, RawInput{
		ProjectID: "42",
		FilePath:  "icon.png",
	})
	if err != nil {
		t.Fatalf("GetRaw() unexpected error: %v", err)
	}
	if out.ContentCategory != "image" {
		t.Errorf("ContentCategory = %q, want %q", out.ContentCategory, "image")
	}
	if out.ImageMIMEType != "image/png" {
		t.Errorf("ImageMIMEType = %q, want %q", out.ImageMIMEType, "image/png")
	}
	if len(out.ImageData) != len(rawPNG) {
		t.Errorf("ImageData length = %d, want %d", len(out.ImageData), len(rawPNG))
	}
	if out.Content != "" {
		t.Errorf("Content should be empty for images, got %q", out.Content)
	}
}

// TestFileGetRaw_BinaryFile verifies that GetRaw detects binary files,
// empties Content, and sets ContentCategory="binary".
func TestFileGetRaw_BinaryFile(t *testing.T) {
	rawPDF := []byte("%PDF-1.4 fake content")

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "doc.pdf") {
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			w.Write(rawPDF)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRaw(context.Background(), client, RawInput{
		ProjectID: "42",
		FilePath:  "doc.pdf",
	})
	if err != nil {
		t.Fatalf("GetRaw() unexpected error: %v", err)
	}
	if out.ContentCategory != "binary" {
		t.Errorf("ContentCategory = %q, want %q", out.ContentCategory, "binary")
	}
	if out.Content != "" {
		t.Errorf("Content should be empty for binary files, got %q", out.Content)
	}
}

// ---------------------------------------------------------------------------
// Canceled-context tests for ALL handlers
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: "42", FilePath: "f.txt", Branch: "main", CommitMessage: "m"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", FilePath: "f.txt", Branch: "main", CommitMessage: "m"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", FilePath: "f.txt", Branch: "main", CommitMessage: "m"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestBlame_CancelledContext verifies the behavior of blame cancelled context.
func TestBlame_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := Blame(ctx, client, BlameInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestGetMetaData_CancelledContext verifies the behavior of get meta data cancelled context.
func TestGetMetaData_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := GetMetaData(ctx, client, MetaDataInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestGetRaw_CancelledContext verifies the behavior of get raw cancelled context.
func TestGetRaw_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := GetRaw(ctx, client, RawInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestGetRawFileMetaData_CancelledContext verifies the behavior of get raw file meta data cancelled context.
func TestGetRawFileMetaData_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)

	_, err := GetRawFileMetaData(ctx, client, RawMetaDataInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// API error tests
// ---------------------------------------------------------------------------.

// TestGet_EmptyProjectID verifies the behavior of get empty project i d.
func TestGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %q, want it to contain 'project_id is required'", err.Error())
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", FilePath: "f.txt", Branch: "main", Content: "x", CommitMessage: "m",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_MissingCommitMessage verifies the behavior of create missing commit message.
func TestCreate_MissingCommitMessage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", FilePath: "f.txt", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for empty commit_message, got nil")
	}
	if !strings.Contains(err.Error(), "commit_message is required") {
		t.Errorf("error = %q, want it to contain 'commit_message is required'", err.Error())
	}
}

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", FilePath: "f.txt", Branch: "main", Content: "x", CommitMessage: "m",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_MissingBranch verifies the behavior of update missing branch.
func TestUpdate_MissingBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", CommitMessage: "m"})
	if err == nil {
		t.Fatal("expected error for empty branch, got nil")
	}
	if !strings.Contains(err.Error(), "branch is required") {
		t.Errorf("error = %q, want it to contain 'branch is required'", err.Error())
	}
}

// TestUpdate_MissingCommitMessage verifies the behavior of update missing commit message.
func TestUpdate_MissingCommitMessage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for empty commit_message, got nil")
	}
	if !strings.Contains(err.Error(), "commit_message is required") {
		t.Errorf("error = %q, want it to contain 'commit_message is required'", err.Error())
	}
}

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42", FilePath: "f.txt", Branch: "main", CommitMessage: "m",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingBranch verifies the behavior of delete missing branch.
func TestDelete_MissingBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", CommitMessage: "m"})
	if err == nil {
		t.Fatal("expected error for empty branch, got nil")
	}
	if !strings.Contains(err.Error(), "branch is required") {
		t.Errorf("error = %q, want it to contain 'branch is required'", err.Error())
	}
}

// TestDelete_MissingCommitMessage verifies the behavior of delete missing commit message.
func TestDelete_MissingCommitMessage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for empty commit_message, got nil")
	}
	if !strings.Contains(err.Error(), "commit_message is required") {
		t.Errorf("error = %q, want it to contain 'commit_message is required'", err.Error())
	}
}

// TestBlame_APIError verifies the behavior of blame a p i error.
func TestBlame_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Blame(context.Background(), client, BlameInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetMetaData_APIError verifies the behavior of get meta data a p i error.
func TestGetMetaData_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetMetaData(context.Background(), client, MetaDataInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetRaw_APIError verifies the behavior of get raw a p i error.
func TestGetRaw_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetRaw(context.Background(), client, RawInput{ProjectID: "42", FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// GetRawFileMetaData
// ---------------------------------------------------------------------------.

// TestGetRawFileMetaData_Success verifies the behavior of get raw file meta data success.
func TestGetRawFileMetaData_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/files/main.go/raw" && r.Method == http.MethodHead {
			w.Header().Set("X-Gitlab-File-Name", "main.go")
			w.Header().Set("X-Gitlab-File-Path", "main.go")
			w.Header().Set("X-Gitlab-Size", "30")
			w.Header().Set("X-Gitlab-Blob-Id", "blob789")
			w.Header().Set("X-Gitlab-Commit-Id", "commit789")
			w.Header().Set("X-Gitlab-Last-Commit-Id", "commit789")
			w.Header().Set("X-Gitlab-Content-Sha256", "sha256raw")
			w.Header().Set("X-Gitlab-Encoding", "base64")
			w.Header().Set("X-Gitlab-Ref", "develop")
			w.Header().Set("X-Gitlab-Execute-Filemode", "false")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRawFileMetaData(context.Background(), client, RawMetaDataInput{
		ProjectID: "42",
		FilePath:  testFileMainGo,
		Ref:       "develop",
	})
	if err != nil {
		t.Fatalf("GetRawFileMetaData() unexpected error: %v", err)
	}
	if out.FileName != testFileMainGo {
		t.Errorf("FileName = %q, want %q", out.FileName, testFileMainGo)
	}
	if out.BlobID != "blob789" {
		t.Errorf("BlobID = %q, want %q", out.BlobID, "blob789")
	}
	if out.SHA256 != "sha256raw" {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, "sha256raw")
	}
}

// TestGetRawFileMetaData_EmptyProjectID verifies the behavior of get raw file meta data empty project i d.
func TestGetRawFileMetaData_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	_, err := GetRawFileMetaData(context.Background(), client, RawMetaDataInput{FilePath: testFileMainGo})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %q, want it to contain 'project_id is required'", err.Error())
	}
}

// TestGetRawFileMetaData_APIError verifies the behavior of get raw file meta data a p i error.
func TestGetRawFileMetaData_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetRawFileMetaData(context.Background(), client, RawMetaDataInput{
		ProjectID: "42", FilePath: testFileMainGo,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Optional fields — Create with all optional fields
// ---------------------------------------------------------------------------.

// TestCreate_WithAllOptionalFields verifies the behavior of create with all optional fields.
func TestCreate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/repository/files/script.sh" {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{"file_path":"script.sh","branch":"feature"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:       "42",
		FilePath:        "script.sh",
		Branch:          "feature",
		Content:         "#!/bin/bash\necho hello",
		CommitMessage:   "add script",
		StartBranch:     "main",
		Encoding:        "text",
		AuthorEmail:     "dev@test.com",
		AuthorName:      "Dev",
		ExecuteFilemode: new(true),
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.FilePath != "script.sh" {
		t.Errorf("FilePath = %q, want %q", out.FilePath, "script.sh")
	}
	if out.Branch != "feature" {
		t.Errorf("Branch = %q, want %q", out.Branch, "feature")
	}
	// Verify optional fields were sent in the request body
	for _, want := range []string{"start_branch", "encoding", "author_email", "author_name", "execute_filemode"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Optional fields — Update with all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_WithAllOptionalFields verifies the behavior of update with all optional fields.
func TestUpdate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/repository/files/script.sh" {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, `{"file_path":"script.sh","branch":"feature"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:       "42",
		FilePath:        "script.sh",
		Branch:          "feature",
		Content:         "#!/bin/bash\necho updated",
		CommitMessage:   "update script",
		StartBranch:     "main",
		Encoding:        "text",
		AuthorEmail:     "dev@test.com",
		AuthorName:      "Dev",
		LastCommitID:    "abc123",
		ExecuteFilemode: new(true),
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.FilePath != "script.sh" {
		t.Errorf("FilePath = %q, want %q", out.FilePath, "script.sh")
	}
	for _, want := range []string{"start_branch", "encoding", "author_email", "author_name", "last_commit_id", "execute_filemode"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Optional fields — Delete with all optional fields
// ---------------------------------------------------------------------------.

// TestDelete_WithAllOptionalFields verifies the behavior of delete with all optional fields.
func TestDelete_WithAllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/repository/files/old.txt" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "42",
		FilePath:      "old.txt",
		Branch:        "main",
		CommitMessage: "delete old",
		StartBranch:   "develop",
		AuthorEmail:   "dev@test.com",
		AuthorName:    "Dev",
		LastCommitID:  "abc123",
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Blame with ref and range options
// ---------------------------------------------------------------------------.

// TestBlame_WithRefAndRange verifies the behavior of blame with ref and range.
func TestBlame_WithRefAndRange(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/main.go/blame" {
			q := r.URL.Query()
			if q.Get("ref") != "develop" {
				t.Errorf("ref = %q, want %q", q.Get("ref"), "develop")
			}
			if q.Get("range[start]") != "5" {
				t.Errorf("range[start] = %q, want %q", q.Get("range[start]"), "5")
			}
			if q.Get("range[end]") != "10" {
				t.Errorf("range[end] = %q, want %q", q.Get("range[end]"), "10")
			}
			testutil.RespondJSON(w, http.StatusOK, `[
				{
					"commit":{"id":"def456","message":"refactor","author_name":"Bob","author_email":"bob@test.com"},
					"lines":["line5","line6"]
				}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Blame(context.Background(), client, BlameInput{
		ProjectID:  "42",
		FilePath:   testFileMainGo,
		Ref:        "develop",
		RangeStart: 5,
		RangeEnd:   10,
	})
	if err != nil {
		t.Fatalf("Blame() unexpected error: %v", err)
	}
	if len(out.Ranges) != 1 {
		t.Fatalf("len(Ranges) = %d, want 1", len(out.Ranges))
	}
	if out.Ranges[0].Commit.ID != "def456" {
		t.Errorf("Commit.ID = %q, want %q", out.Ranges[0].Commit.ID, "def456")
	}
}

// ---------------------------------------------------------------------------
// MetaData with ref option
// ---------------------------------------------------------------------------.

// TestGetMetaData_WithRef verifies the behavior of get meta data with ref.
func TestGetMetaData_WithRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/files/main.go" {
			q := r.URL.Query()
			if q.Get("ref") != "v1.0.0" {
				t.Errorf("ref = %q, want %q", q.Get("ref"), "v1.0.0")
			}
			w.Header().Set("X-Gitlab-File-Name", "main.go")
			w.Header().Set("X-Gitlab-File-Path", "main.go")
			w.Header().Set("X-Gitlab-Size", "50")
			w.Header().Set("X-Gitlab-Blob-Id", "blobref")
			w.Header().Set("X-Gitlab-Commit-Id", "commitref")
			w.Header().Set("X-Gitlab-Last-Commit-Id", "commitref")
			w.Header().Set("X-Gitlab-Content-Sha256", "sha256ref")
			w.Header().Set("X-Gitlab-Encoding", "base64")
			w.Header().Set("X-Gitlab-Ref", "v1.0.0")
			w.Header().Set("X-Gitlab-Execute-Filemode", "false")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetMetaData(context.Background(), client, MetaDataInput{
		ProjectID: "42", FilePath: testFileMainGo, Ref: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("GetMetaData() unexpected error: %v", err)
	}
	if out.Ref != "v1.0.0" {
		t.Errorf("Ref = %q, want %q", out.Ref, "v1.0.0")
	}
}

// ---------------------------------------------------------------------------
// Raw with ref option
// ---------------------------------------------------------------------------.

// TestGetRaw_WithRef verifies the behavior of get raw with ref.
func TestGetRaw_WithRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/main.go/raw" {
			q := r.URL.Query()
			if q.Get("ref") != "v2.0.0" {
				t.Errorf("ref = %q, want %q", q.Get("ref"), "v2.0.0")
			}
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("tagged content"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetRaw(context.Background(), client, RawInput{
		ProjectID: "42", FilePath: testFileMainGo, Ref: "v2.0.0",
	})
	if err != nil {
		t.Fatalf("GetRaw() unexpected error: %v", err)
	}
	if out.Content != "tagged content" {
		t.Errorf("Content = %q, want %q", out.Content, "tagged content")
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown verifies the behavior of format output markdown.
func TestFormatOutputMarkdown(t *testing.T) {
	t.Run("empty file path returns empty string", func(t *testing.T) {
		got := FormatOutputMarkdown(Output{})
		if got != "" {
			t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", got)
		}
	})

	t.Run("non-empty file renders markdown", func(t *testing.T) {
		got := FormatOutputMarkdown(Output{
			FilePath: "src/main.go",
			Size:     1024,
			Ref:      "main",
			Encoding: "base64",
			BlobID:   "blob123",
		})
		for _, want := range []string{
			"## File: src/main.go",
			"**Size**: 1024 bytes",
			"**Ref**: main",
			"**Encoding**: base64",
			"**Blob ID**: blob123",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("FormatOutputMarkdown missing %q in:\n%s", want, got)
			}
		}
	})
}

// TestFormatFileInfoMarkdown verifies the behavior of format file info markdown.
func TestFormatFileInfoMarkdown(t *testing.T) {
	got := FormatFileInfoMarkdown(FileInfoOutput{
		FilePath: "new_file.txt",
		Branch:   "feature",
	})
	for _, want := range []string{
		"## File Operation Result",
		"**File**: new_file.txt",
		"**Branch**: feature",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatFileInfoMarkdown missing %q in:\n%s", want, got)
		}
	}
}

// TestFormatBlameMarkdown verifies the behavior of format blame markdown.
func TestFormatBlameMarkdown(t *testing.T) {
	t.Run("empty ranges", func(t *testing.T) {
		got := FormatBlameMarkdown(BlameOutput{
			FilePath: "empty.go",
			Ranges:   nil,
		})
		if !strings.Contains(got, "No blame data found") {
			t.Errorf("expected 'No blame data found' in:\n%s", got)
		}
	})

	t.Run("with ranges", func(t *testing.T) {
		got := FormatBlameMarkdown(BlameOutput{
			FilePath: "main.go",
			Ranges: []BlameRangeOutput{
				{
					Commit: BlameRangeCommitOutput{
						ID:         "abc12345deadbeef",
						Message:    "initial commit",
						AuthorName: "Alice",
					},
					Lines: []string{"package main", "", "func main() {}"},
				},
			},
		})
		for _, want := range []string{
			"## File Blame: main.go",
			"Range 1",
			"Alice",
			"abc12345",
			"initial commit",
			"package main",
			"func main() {}",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("FormatBlameMarkdown missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("short commit ID does not panic", func(t *testing.T) {
		got := FormatBlameMarkdown(BlameOutput{
			FilePath: "short.go",
			Ranges: []BlameRangeOutput{
				{
					Commit: BlameRangeCommitOutput{
						ID:         "abc",
						Message:    "short",
						AuthorName: "Bob",
					},
					Lines: []string{"line1"},
				},
			},
		})
		if !strings.Contains(got, "abc") {
			t.Errorf("expected short ID 'abc' in:\n%s", got)
		}
	})
}

// TestFormatMetaDataMarkdown verifies the behavior of format meta data markdown.
func TestFormatMetaDataMarkdown(t *testing.T) {
	t.Run("without execute filemode", func(t *testing.T) {
		got := FormatMetaDataMarkdown(MetaDataOutput{
			FileName:     "data.json",
			FilePath:     "data.json",
			Size:         512,
			Ref:          "main",
			Encoding:     "base64",
			BlobID:       "b1",
			CommitID:     "c1",
			LastCommitID: "c1",
			SHA256:       "sha256val",
		})
		for _, want := range []string{
			"## File Metadata: data.json",
			"**Name**: data.json",
			"**Size**: 512 bytes",
			"**SHA-256**: sha256val",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("FormatMetaDataMarkdown missing %q in:\n%s", want, got)
			}
		}
		if strings.Contains(got, "Executable") {
			t.Error("should not contain 'Executable' when ExecuteFilemode is false")
		}
	})

	t.Run("with execute filemode", func(t *testing.T) {
		got := FormatMetaDataMarkdown(MetaDataOutput{
			FilePath:        "script.sh",
			FileName:        "script.sh",
			ExecuteFilemode: true,
		})
		if !strings.Contains(got, "**Executable**: yes") {
			t.Errorf("expected '**Executable**: yes' in:\n%s", got)
		}
	})
}

// TestFormatRawMarkdown verifies the behavior of format raw markdown.
func TestFormatRawMarkdown(t *testing.T) {
	got := FormatRawMarkdown(RawOutput{
		FilePath: "readme.md",
		Size:     42,
		Content:  "# Hello World",
	})
	for _, want := range []string{
		"## Raw File: readme.md",
		"**Size**: 42 bytes",
		"# Hello World",
		"```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatRawMarkdown missing %q in:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// minLen helper
// ---------------------------------------------------------------------------.

// TestMinLen validates min len across multiple scenarios using table-driven subtests.
func TestMinLen(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{3, 8, 3},
		{8, 3, 3},
		{5, 5, 5},
		{0, 1, 0},
	}
	for _, tt := range tests {
		got := minLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("minLen(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Get: invalid base64 content triggers decode error
// ---------------------------------------------------------------------------.

// TestGet_InvalidBase64Content verifies the behavior of get invalid base64 content.
func TestGet_InvalidBase64Content(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/bad.txt" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"bad.txt",
				"file_path":"bad.txt",
				"size":10,
				"encoding":"base64",
				"content":"%%%NOT-BASE64%%%",
				"ref":"main",
				"blob_id":"b1",
				"commit_id":"c1",
				"last_commit_id":"c1"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "bad.txt",
	})
	if err == nil {
		t.Fatal("expected base64 decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode base64") {
		t.Errorf("error = %q, want it to contain 'decode base64'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Get: non-base64 encoding (content returned as-is)
// ---------------------------------------------------------------------------.

// TestGet_NonBase64Encoding verifies the behavior of get non base64 encoding.
func TestGet_NonBase64Encoding(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/files/plain.txt" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"plain.txt",
				"file_path":"plain.txt",
				"size":5,
				"encoding":"text",
				"content":"hello",
				"ref":"main",
				"blob_id":"b2",
				"commit_id":"c2",
				"last_commit_id":"c2"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "plain.txt",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Content != "hello" {
		t.Errorf("Content = %q, want %q", out.Content, "hello")
	}
	if out.Encoding != "text" {
		t.Errorf("Encoding = %q, want %q", out.Encoding, "text")
	}
}

// ---------------------------------------------------------------------------
// Get: API error (not found)
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		FilePath:  "missing.go",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — covers register.go handler closures via in-memory MCP
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// newFilesMCPSession is an internal helper for the files package.
func newFilesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	content := base64.StdEncoding.EncodeToString([]byte("package main\n"))
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// GetFile (GET /files/{path} with no /raw or /blame suffix)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/files/main.go") && !strings.Contains(path, "/raw") && !strings.Contains(path, "/blame"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"main.go","file_path":"main.go","size":13,
				"encoding":"base64","content":"`+content+`",
				"ref":"main","blob_id":"b1","commit_id":"c1","last_commit_id":"c1"
			}`)

		// CreateFile (POST)
		case r.Method == http.MethodPost && strings.Contains(path, "/repository/files/"):
			testutil.RespondJSON(w, http.StatusCreated, `{"file_path":"new.txt","branch":"main"}`)

		// UpdateFile (PUT)
		case r.Method == http.MethodPut && strings.Contains(path, "/repository/files/"):
			testutil.RespondJSON(w, http.StatusOK, `{"file_path":"main.go","branch":"main"}`)

		// DeleteFile (DELETE)
		case r.Method == http.MethodDelete && strings.Contains(path, "/repository/files/"):
			w.WriteHeader(http.StatusNoContent)

		// Blame (GET .../blame)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/blame"):
			testutil.RespondJSON(w, http.StatusOK, `[{"commit":{"id":"abc12345","message":"init","author_name":"A","author_email":"a@t.com"},"lines":["line1"]}]`)

		// Raw file (GET .../raw)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/raw"):
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("raw content"))

		// GetFileMetaData (HEAD on /files/{path}, no /raw)
		case r.Method == http.MethodHead && strings.Contains(path, "/repository/files/") && !strings.HasSuffix(path, "/raw"):
			w.Header().Set("X-Gitlab-File-Name", "main.go")
			w.Header().Set("X-Gitlab-File-Path", "main.go")
			w.Header().Set("X-Gitlab-Size", "13")
			w.Header().Set("X-Gitlab-Blob-Id", "b1")
			w.Header().Set("X-Gitlab-Commit-Id", "c1")
			w.Header().Set("X-Gitlab-Last-Commit-Id", "c1")
			w.Header().Set("X-Gitlab-Content-Sha256", "sha")
			w.Header().Set("X-Gitlab-Encoding", "base64")
			w.Header().Set("X-Gitlab-Ref", "main")
			w.Header().Set("X-Gitlab-Execute-Filemode", "false")
			w.WriteHeader(http.StatusOK)

		// GetRawFileMetaData (HEAD on .../raw)
		case r.Method == http.MethodHead && strings.HasSuffix(path, "/raw"):
			w.Header().Set("X-Gitlab-File-Name", "raw.go")
			w.Header().Set("X-Gitlab-File-Path", "raw.go")
			w.Header().Set("X-Gitlab-Size", "42")
			w.Header().Set("X-Gitlab-Blob-Id", "b2")
			w.Header().Set("X-Gitlab-Commit-Id", "c2")
			w.Header().Set("X-Gitlab-Last-Commit-Id", "c2")
			w.Header().Set("X-Gitlab-Content-Sha256", "sha-raw")
			w.Header().Set("X-Gitlab-Encoding", "text")
			w.Header().Set("X-Gitlab-Ref", "main")
			w.Header().Set("X-Gitlab-Execute-Filemode", "true")
			w.WriteHeader(http.StatusOK)

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

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newFilesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_file_get", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_create", map[string]any{"project_id": "42", "file_path": "new.txt", "branch": "main", "content": "x", "commit_message": "add"}},
		{"gitlab_file_update", map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "content": "y", "commit_message": "up"}},
		{"gitlab_file_delete", map[string]any{"project_id": "42", "file_path": "main.go", "branch": "main", "commit_message": "del"}},
		{"gitlab_file_blame", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_metadata", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_raw", map[string]any{"project_id": "42", "file_path": "main.go"}},
		{"gitlab_file_raw_metadata", map[string]any{"project_id": "42", "file_path": "main.go"}},
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
// langFromPath — language detection by file extension
// ---------------------------------------------------------------------------.

// TestLangFromPath validates that langFromPath correctly maps file extensions
// to language identifiers. Covers every branch of the switch statement plus
// edge cases like uppercase extensions, multi-dot filenames, Dockerfiles, and
// unknown extensions.
func TestLangFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		// Core languages
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"index.ts", "typescript"},
		{"app.rb", "ruby"},
		{"main.rs", "rust"},
		{"App.java", "java"},
		{"Main.kt", "kotlin"},
		{"build.gradle.kts", "kotlin"},
		{"Program.cs", "csharp"},
		{"lib.cpp", "cpp"},
		{"lib.cc", "cpp"},
		{"lib.cxx", "cpp"},
		{"lib.hpp", "cpp"},
		{"lib.c", "c"},
		{"lib.h", "c"},
		{"app.swift", "swift"},

		// Scripting & shells
		{"run.sh", "bash"},
		{"run.bash", "bash"},
		{"script.ps1", "powershell"},
		{"mod.psm1", "powershell"},

		// Data & config formats
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"data.json", "json"},
		{"pom.xml", "xml"},
		{"index.html", "html"},
		{"index.htm", "html"},
		{"style.css", "css"},
		{"style.scss", "scss"},
		{"query.sql", "sql"},
		{"README.md", "markdown"},
		{"README.markdown", "markdown"},
		{"Dockerfile.dockerfile", "dockerfile"},
		{"config.toml", "toml"},
		{"settings.ini", "ini"},
		{"settings.cfg", "ini"},

		// Other languages
		{"analysis.r", "r"},
		{"script.lua", "lua"},
		{"test.pl", "perl"},
		{"Test.pm", "perl"},
		{"index.php", "php"},
		{"schema.proto", "protobuf"},
		{"main.tf", "hcl"},
		{"schema.graphql", "graphql"},
		{"schema.gql", "graphql"},

		// Case insensitivity
		{"Main.GO", "go"},
		{"App.PY", "python"},
		{"style.CSS", "css"},

		// Multi-dot filenames — extension is the last segment
		{"archive.tar.gz", ""},
		{"docker-compose.override.yml", "yaml"},
		{"some.file.json", "json"},

		// No extension / unknown
		{"Makefile", ""},
		{"noext", ""},
		{"unknown.xyz", ""},
		{".hidden", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := langFromPath(tt.path)
			if got != tt.want {
				t.Errorf("langFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestFileGet_ServerError covers the generic (non-404) error path in Get.
func TestFileGet_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", FilePath: "f.go"})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// TestFileCreate_BadRequest covers the 400 error hint in Create.
func TestFileCreate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"file already exists"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", FilePath: "f.go", Branch: "main", CommitMessage: "add", Content: "x",
	})
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
}

// TestFileCreate_ServerError covers the generic (non-400) error path in Create.
func TestFileCreate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", FilePath: "f.go", Branch: "main", CommitMessage: "add", Content: "x",
	})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// TestFileUpdate_BadRequest covers the 400 error hint in Update.
func TestFileUpdate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad encoding"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", FilePath: "f.go", Branch: "main", CommitMessage: "upd", Content: "x",
	})
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
}

// TestFileUpdate_Conflict covers the 409 error hint in Update.
func TestFileUpdate_Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"file modified"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", FilePath: "f.go", Branch: "main", CommitMessage: "upd", Content: "x",
	})
	if err == nil {
		t.Fatal("expected error for 409, got nil")
	}
}

// TestFileUpdate_ServerError covers the generic error branch in Update.
func TestFileUpdate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", FilePath: "f.go", Branch: "main", CommitMessage: "upd", Content: "x",
	})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// TestFileDelete_ServerError covers the generic (non-404) error path in Delete.
func TestFileDelete_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42", FilePath: "f.go", Branch: "main", CommitMessage: "del",
	})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}
