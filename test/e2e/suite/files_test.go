//go:build e2e

// files_test.go tests the file MCP tools against a live GitLab instance.
// Covers text, image, and binary file download via gitlab_file_get (base64
// decoding), gitlab_file_raw, gitlab_file_metadata, gitlab_file_blame,
// and the full CRUD lifecycle (create, update, delete).
// Uses both individual tools and the gitlab_repository meta-tool.
package suite

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Minimal 1x1 red pixel PNG (67 bytes).
var pngPixel = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x00, 0x03, 0x00, 0x01, 0x36, 0x28, 0x19,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

// TestIndividual_Files exercises file download and CRUD operations using
// individual MCP tools. Verifies base64 decoding for text, image, and binary
// files, raw content retrieval, metadata, blame, and the create/update/delete
// lifecycle.
func TestIndividual_Files(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	// Commit test files: text, image (PNG), and binary (PDF header).
	const (
		textPath   = "hello.go"
		textBody   = "package main\n\nfunc hello() string { return \"world\" }\n"
		imgPath    = "logo.png"
		binPath    = "report.pdf"
		binContent = "%PDF-1.4 fake binary content"
	)
	pngBase64 := base64.StdEncoding.EncodeToString(pngPixel)
	binBase64 := base64.StdEncoding.EncodeToString([]byte(binContent))

	commitFile(ctx, t, sess.individual, proj, defaultBranch, textPath, textBody, "add text file")
	commitFileBase64(ctx, t, sess.individual, proj, defaultBranch, imgPath, pngBase64, "add PNG image")
	commitFileBase64(ctx, t, sess.individual, proj, defaultBranch, binPath, binBase64, "add binary PDF")

	// --- Base64 decode: text file ---
	t.Run("FileGet_Text", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.individual, "gitlab_file_get", files.GetInput{
			ProjectID: proj.pidOf(),
			FilePath:  textPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get text file")
		requireTrue(t, out.FileName == textPath, "expected %s, got %s", textPath, out.FileName)
		requireTrue(t, out.ContentCategory == "text", "expected text category, got %s", out.ContentCategory)
		requireTrue(t, strings.Contains(out.Content, "hello"), "text content should contain 'hello', got: %s", out.Content)
		requireTrue(t, out.Size > 0, "expected size > 0")
		requireTrue(t, out.CommitID != "", msgCommitIDEmpty)
		t.Logf("FileGet text: %s (size=%d, encoding=%s, category=%s)", out.FileName, out.Size, out.Encoding, out.ContentCategory)
	})

	// --- Base64 decode: image file ---
	t.Run("FileGet_Image", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.individual, "gitlab_file_get", files.GetInput{
			ProjectID: proj.pidOf(),
			FilePath:  imgPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get image file")
		requireTrue(t, out.FileName == imgPath, "expected %s, got %s", imgPath, out.FileName)
		requireTrue(t, out.ContentCategory == "image", "expected image category, got %s", out.ContentCategory)
		requireTrue(t, out.Content == "", "image content field should be empty")
		requireTrue(t, out.Size > 0, "expected size > 0")
		t.Logf("FileGet image: %s (size=%d, category=%s)", out.FileName, out.Size, out.ContentCategory)
	})

	// --- Base64 decode: binary file ---
	t.Run("FileGet_Binary", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.individual, "gitlab_file_get", files.GetInput{
			ProjectID: proj.pidOf(),
			FilePath:  binPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get binary file")
		requireTrue(t, out.FileName == binPath, "expected %s, got %s", binPath, out.FileName)
		requireTrue(t, out.ContentCategory == "binary", "expected binary category, got %s", out.ContentCategory)
		requireTrue(t, out.Content == "", "binary content field should be empty")
		t.Logf("FileGet binary: %s (size=%d, category=%s)", out.FileName, out.Size, out.ContentCategory)
	})

	// --- Raw content: text file ---
	t.Run("FileRaw_Text", func(t *testing.T) {
		out, err := callToolOn[files.RawOutput](ctx, sess.individual, "gitlab_file_raw", files.RawInput{
			ProjectID: proj.pidOf(),
			FilePath:  textPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get raw text file")
		requireTrue(t, out.ContentCategory == "text", "expected text category, got %s", out.ContentCategory)
		requireTrue(t, strings.Contains(out.Content, "hello"), "raw content should contain 'hello'")
		requireTrue(t, out.Size > 0, "expected size > 0")
		t.Logf("FileRaw text: %s (size=%d)", out.FilePath, out.Size)
	})

	// --- Raw content: image file ---
	t.Run("FileRaw_Image", func(t *testing.T) {
		out, err := callToolOn[files.RawOutput](ctx, sess.individual, "gitlab_file_raw", files.RawInput{
			ProjectID: proj.pidOf(),
			FilePath:  imgPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get raw image file")
		requireTrue(t, out.ContentCategory == "image", "expected image category, got %s", out.ContentCategory)
		requireTrue(t, out.Content == "", "raw image content field should be empty")
		requireTrue(t, out.Size > 0, "expected size > 0")
		t.Logf("FileRaw image: %s (size=%d, category=%s)", out.FilePath, out.Size, out.ContentCategory)
	})

	// --- Metadata (no content) ---
	t.Run("FileMetadata", func(t *testing.T) {
		out, err := callToolOn[files.MetaDataOutput](ctx, sess.individual, "gitlab_file_metadata", files.MetaDataInput{
			ProjectID: proj.pidOf(),
			FilePath:  textPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get file metadata")
		requireTrue(t, out.FileName == textPath, "expected %s, got %s", textPath, out.FileName)
		requireTrue(t, out.Size > 0, "expected size > 0")
		requireTrue(t, out.BlobID != "", "expected non-empty blob_id")
		requireTrue(t, out.SHA256 != "", "expected non-empty content_sha256")
		t.Logf("FileMetadata: %s (size=%d, sha256=%s)", out.FileName, out.Size, out.SHA256)
	})

	// --- Raw metadata (HEAD request) ---
	t.Run("FileRawMetadata", func(t *testing.T) {
		out, err := callToolOn[files.MetaDataOutput](ctx, sess.individual, "gitlab_file_raw_metadata", files.RawMetaDataInput{
			ProjectID: proj.pidOf(),
			FilePath:  textPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get raw file metadata")
		requireTrue(t, out.FileName == textPath, "expected %s, got %s", textPath, out.FileName)
		requireTrue(t, out.Size > 0, "expected size > 0")
		requireTrue(t, out.SHA256 != "", "expected non-empty content_sha256")
		t.Logf("FileRawMetadata: %s (size=%d, sha256=%s)", out.FileName, out.Size, out.SHA256)
	})

	// --- Blame ---
	t.Run("FileBlame", func(t *testing.T) {
		out, err := callToolOn[files.BlameOutput](ctx, sess.individual, "gitlab_file_blame", files.BlameInput{
			ProjectID: proj.pidOf(),
			FilePath:  textPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get file blame")
		requireTrue(t, out.FilePath == textPath, "expected path %s, got %s", textPath, out.FilePath)
		requireTrue(t, len(out.Ranges) >= 1, "expected at least 1 blame range, got %d", len(out.Ranges))
		requireTrue(t, out.Ranges[0].Commit.ID != "", "expected non-empty commit ID in blame")
		t.Logf("FileBlame: %s has %d blame ranges", out.FilePath, len(out.Ranges))
	})

	// --- CRUD lifecycle: create, update, delete via file tools ---
	const crudPath = "crud-test.txt"

	t.Run("FileCreate", func(t *testing.T) {
		out, err := callToolOn[files.FileInfoOutput](ctx, sess.individual, "gitlab_file_create", files.CreateInput{
			ProjectID:     proj.pidOf(),
			FilePath:      crudPath,
			Branch:        defaultBranch,
			Content:       "initial content",
			CommitMessage: "create crud test file",
		})
		requireNoError(t, err, "create file")
		requireTrue(t, out.FilePath == crudPath, "expected path %s, got %s", crudPath, out.FilePath)
		requireTrue(t, out.Branch == defaultBranch, "expected branch %s, got %s", defaultBranch, out.Branch)
		t.Logf("FileCreate: %s on %s", out.FilePath, out.Branch)
	})

	t.Run("FileUpdate", func(t *testing.T) {
		out, err := callToolOn[files.FileInfoOutput](ctx, sess.individual, "gitlab_file_update", files.UpdateInput{
			ProjectID:     proj.pidOf(),
			FilePath:      crudPath,
			Branch:        defaultBranch,
			Content:       "updated content",
			CommitMessage: "update crud test file",
		})
		requireNoError(t, err, "update file")
		requireTrue(t, out.FilePath == crudPath, "expected path %s, got %s", crudPath, out.FilePath)
		t.Logf("FileUpdate: %s", out.FilePath)
	})

	t.Run("FileGet_Updated", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.individual, "gitlab_file_get", files.GetInput{
			ProjectID: proj.pidOf(),
			FilePath:  crudPath,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get updated file")
		requireTrue(t, strings.Contains(out.Content, "updated"), "content should contain 'updated', got: %s", out.Content)
		t.Logf("FileGet updated: content=%q", out.Content)
	})

	t.Run("FileDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_file_delete", files.DeleteInput{
			ProjectID:     proj.pidOf(),
			FilePath:      crudPath,
			Branch:        defaultBranch,
			CommitMessage: "delete crud test file",
		})
		requireNoError(t, err, "delete file")
		t.Logf("FileDelete: %s", crudPath)
	})

	// --- Base64-encoded create via encoding field ---
	t.Run("FileCreate_Base64Encoding", func(t *testing.T) {
		b64Content := base64.StdEncoding.EncodeToString([]byte("base64 encoded create"))
		out, err := callToolOn[files.FileInfoOutput](ctx, sess.individual, "gitlab_file_create", files.CreateInput{
			ProjectID:     proj.pidOf(),
			FilePath:      "b64-created.txt",
			Branch:        defaultBranch,
			Content:       b64Content,
			Encoding:      "base64",
			CommitMessage: "create file with base64 encoding",
		})
		requireNoError(t, err, "create file with base64 encoding")
		requireTrue(t, out.FilePath == "b64-created.txt", "expected b64-created.txt, got %s", out.FilePath)
		t.Logf("FileCreate base64: %s", out.FilePath)

		// Verify the decoded content.
		got, err := callToolOn[files.Output](ctx, sess.individual, "gitlab_file_get", files.GetInput{
			ProjectID: proj.pidOf(),
			FilePath:  "b64-created.txt",
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "verify base64-created file")
		requireTrue(t, strings.Contains(got.Content, "base64 encoded create"), "content mismatch: %s", got.Content)
		t.Logf("Verified base64-created content: %q", got.Content)
	})
}

// TestMeta_Files exercises file download and CRUD operations using the
// gitlab_repository meta-tool. Verifies base64 decoding for text, image,
// and binary files, raw content, and the create/update/delete lifecycle.
func TestMeta_Files(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)

	const (
		textPath = "hello.go"
		textBody = "package main\n\nfunc hello() string { return \"world\" }\n"
		imgPath  = "logo.png"
		binPath  = "report.pdf"
	)
	pngBase64 := base64.StdEncoding.EncodeToString(pngPixel)
	binBase64 := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake binary"))

	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch, textPath, textBody, "add text file")
	commitFileBase64Meta(ctx, t, sess.meta, proj, defaultBranch, imgPath, pngBase64, "add PNG image")
	commitFileBase64Meta(ctx, t, sess.meta, proj, defaultBranch, binPath, binBase64, "add binary PDF")

	// --- FileGet text via meta-tool ---
	t.Run("FileGet_Text", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  textPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file get text")
		requireTrue(t, out.ContentCategory == "text", "expected text, got %s", out.ContentCategory)
		requireTrue(t, strings.Contains(out.Content, "hello"), "text should contain 'hello'")
		t.Logf("Meta FileGet text: %s (category=%s)", out.FileName, out.ContentCategory)
	})

	// --- FileGet image via meta-tool ---
	t.Run("FileGet_Image", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  imgPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file get image")
		requireTrue(t, out.ContentCategory == "image", "expected image, got %s", out.ContentCategory)
		requireTrue(t, out.Content == "", "image content should be empty")
		t.Logf("Meta FileGet image: %s (category=%s)", out.FileName, out.ContentCategory)
	})

	// --- FileGet binary via meta-tool ---
	t.Run("FileGet_Binary", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  binPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file get binary")
		requireTrue(t, out.ContentCategory == "binary", "expected binary, got %s", out.ContentCategory)
		requireTrue(t, out.Content == "", "binary content should be empty")
		t.Logf("Meta FileGet binary: %s (category=%s)", out.FileName, out.ContentCategory)
	})

	// --- FileRaw text via meta-tool ---
	t.Run("FileRaw_Text", func(t *testing.T) {
		out, err := callToolOn[files.RawOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_raw",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  textPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file raw text")
		requireTrue(t, out.ContentCategory == "text", "expected text, got %s", out.ContentCategory)
		requireTrue(t, strings.Contains(out.Content, "hello"), "raw content should contain 'hello'")
		t.Logf("Meta FileRaw text: %s (size=%d)", out.FilePath, out.Size)
	})

	// --- CRUD lifecycle via meta-tool ---
	const crudPath = "meta-crud.txt"

	t.Run("FileCreate", func(t *testing.T) {
		out, err := callToolOn[files.FileInfoOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_create",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      crudPath,
				"branch":         defaultBranch,
				"content":        "meta initial content",
				"commit_message": "create meta crud file",
			},
		})
		requireNoError(t, err, "meta file create")
		requireTrue(t, out.FilePath == crudPath, "expected %s, got %s", crudPath, out.FilePath)
		t.Logf("Meta FileCreate: %s", out.FilePath)
	})

	t.Run("FileUpdate", func(t *testing.T) {
		out, err := callToolOn[files.FileInfoOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_update",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      crudPath,
				"branch":         defaultBranch,
				"content":        "meta updated content",
				"commit_message": "update meta crud file",
			},
		})
		requireNoError(t, err, "meta file update")
		requireTrue(t, out.FilePath == crudPath, "expected %s, got %s", crudPath, out.FilePath)
		t.Logf("Meta FileUpdate: %s", out.FilePath)
	})

	t.Run("FileGet_Updated", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  crudPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta get updated file")
		requireTrue(t, strings.Contains(out.Content, "meta updated"), "expected updated content, got: %s", out.Content)
		t.Logf("Meta FileGet updated: %q", out.Content)
	})

	t.Run("FileDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_delete",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      crudPath,
				"branch":         defaultBranch,
				"commit_message": "delete meta crud file",
			},
		})
		requireNoError(t, err, "meta file delete")
		t.Logf("Meta FileDelete: %s", crudPath)
	})

	// --- FileBlame via meta-tool ---
	t.Run("FileBlame", func(t *testing.T) {
		out, err := callToolOn[files.BlameOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_blame",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  textPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file blame")
		requireTrue(t, len(out.Ranges) >= 1, "expected at least 1 blame range")
		t.Logf("Meta FileBlame: %s has %d ranges", out.FilePath, len(out.Ranges))
	})

	// --- FileMetadata via meta-tool ---
	t.Run("FileMetadata", func(t *testing.T) {
		out, err := callToolOn[files.MetaDataOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_metadata",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  textPath,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file metadata")
		requireTrue(t, out.FileName == textPath, "expected %s, got %s", textPath, out.FileName)
		requireTrue(t, out.SHA256 != "", "expected non-empty sha256")
		t.Logf("Meta FileMetadata: %s (sha256=%s)", out.FileName, out.SHA256)
	})
}

// commitFileBase64 creates a file with base64-encoded content via the
// gitlab_file_create tool (which supports encoding: "base64").
func commitFileBase64(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branch, path, b64Content, message string) {
	t.Helper()
	const maxRetries = 5
	for attempt := range maxRetries {
		_, err := callToolOn[files.FileInfoOutput](ctx, session, "gitlab_file_create", files.CreateInput{
			ProjectID:     proj.pidOf(),
			FilePath:      path,
			Branch:        branch,
			Content:       b64Content,
			Encoding:      "base64",
			CommitMessage: message,
		})
		if err == nil {
			return
		}
		if attempt < maxRetries-1 && strings.Contains(err.Error(), "only create or edit files when you are on a branch") {
			t.Logf("commitFileBase64 %s: retry %d/%d (branch not ready)", path, attempt+1, maxRetries)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		requireNoError(t, err, "create base64 file "+path)
	}
	t.Fatalf("commitFileBase64 %s: exhausted %d retries", path, maxRetries)
}

// commitFileBase64Meta creates a file with base64-encoded content via the
// gitlab_repository meta-tool (file_create action with encoding: "base64").
func commitFileBase64Meta(ctx context.Context, t *testing.T, session *mcp.ClientSession, proj ProjectFixture, branch, path, b64Content, message string) {
	t.Helper()
	const maxRetries = 8
	for attempt := range maxRetries {
		_, err := callToolOn[files.FileInfoOutput](ctx, session, "gitlab_repository", map[string]any{
			"action": "file_create",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"file_path":      path,
				"branch":         branch,
				"content":        b64Content,
				"encoding":       "base64",
				"commit_message": message,
			},
		})
		if err == nil {
			return
		}
		if attempt < maxRetries-1 && strings.Contains(err.Error(), "only create or edit files when you are on a branch") {
			t.Logf("commitFileBase64Meta %s: retry %d/%d (branch not ready)", path, attempt+1, maxRetries)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		requireNoError(t, err, "create base64 file meta "+path)
	}
	t.Fatalf("commitFileBase64Meta %s: exhausted %d retries", path, maxRetries)
}
