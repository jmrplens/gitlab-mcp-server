// uploads_test.go contains unit tests for GitLab project file upload
// operations. Tests use httptest to mock the GitLab API and verify successful
// uploads, base64 decoding validation, file content integrity, API errors,
// context cancellation, file_path mode, and list/delete operations.
package uploads

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const fmtUnexpErr = "unexpected error: %v"

// pathProjectUploads is the GitLab API endpoint path used across upload tests.
const pathProjectUploads = "/api/v4/projects/42/uploads"

// TestProjectUpload_Success verifies that Upload decodes base64 content,
// sends it to the GitLab upload endpoint, and correctly maps the response
// fields (alt, URL, full path, Markdown embed) to the output struct.
func TestProjectUpload_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathProjectUploads || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			t.Error("expected Content-Type header with multipart form data")
		}

		testutil.RespondJSON(w, http.StatusCreated, `{
			"alt": "screenshot",
			"url": "/uploads/abc123/screenshot.png",
			"full_path": "/my-group/my-project/uploads/abc123/screenshot.png",
			"markdown": "![screenshot](/uploads/abc123/screenshot.png)"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	content := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))

	out, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID:     "42",
		Filename:      "screenshot.png",
		ContentBase64: content,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	if out.Alt != "screenshot" {
		t.Errorf("expected alt 'screenshot', got %q", out.Alt)
	}
	if out.URL != "/uploads/abc123/screenshot.png" {
		t.Errorf("expected url '/uploads/abc123/screenshot.png', got %q", out.URL)
	}
	if out.Markdown != "![screenshot](/uploads/abc123/screenshot.png)" {
		t.Errorf("expected markdown embed, got %q", out.Markdown)
	}
	if out.FullPath != "/my-group/my-project/uploads/abc123/screenshot.png" {
		t.Errorf("expected full_path, got %q", out.FullPath)
	}
	if out.FullURL == "" {
		t.Error("FullURL should not be empty")
	}
	if !strings.Contains(out.FullURL, "/uploads/abc123/screenshot.png") {
		t.Errorf("FullURL should contain upload path, got %q", out.FullURL)
	}
}

// TestProjectUpload_InvalidBase64 verifies that Upload returns an error
// when the content_base64 field contains invalid base64 data. The mock should
// never be called because validation occurs before the API request.
func TestProjectUpload_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called with invalid base64 input")
	}))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID:     "42",
		Filename:      "test.png",
		ContentBase64: "not-valid-base64!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

// TestProjectUpload_APIError verifies that Upload propagates a 403
// Forbidden error returned by the GitLab API.
func TestProjectUpload_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message": "403 Forbidden"}`)
	})

	client := testutil.NewTestClient(t, handler)
	content := base64.StdEncoding.EncodeToString([]byte("data"))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID:     "42",
		Filename:      "test.txt",
		ContentBase64: content,
	})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

// TestProjectUpload_CancelledContext verifies that Upload returns an
// error immediately when called with an already-canceled context.
func TestProjectUpload_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called with canceled context")
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Upload(ctx, nil, client, UploadInput{
		ProjectID:     "42",
		Filename:      "test.png",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("data")),
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestProjectUpload_SendsFileContent verifies end-to-end file content integrity
// by encoding test content as base64, uploading it via Upload, and
// asserting the multipart form data received by the mock server matches the
// original content and filename.
func TestProjectUpload_SendsFileContent(t *testing.T) {
	originalContent := []byte("Hello, this is test content for upload verification.")
	encoded := base64.StdEncoding.EncodeToString(originalContent)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathProjectUploads {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()

		if header.Filename != "document.txt" {
			t.Errorf("expected filename 'document.txt', got %q", header.Filename)
		}

		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed to read file body: %v", err)
		}

		if string(body) != string(originalContent) {
			t.Errorf("file content mismatch: got %q", string(body))
		}

		testutil.RespondJSON(w, http.StatusCreated, `{
			"alt": "document",
			"url": "/uploads/def456/document.txt",
			"full_path": "/my-group/my-project/uploads/def456/document.txt",
			"markdown": "![document](/uploads/def456/document.txt)"
		}`)
	})

	client := testutil.NewTestClient(t, handler)

	out, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID:     "42",
		Filename:      "document.txt",
		ContentBase64: encoded,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	if out.Markdown != "![document](/uploads/def456/document.txt)" {
		t.Errorf("unexpected markdown: %q", out.Markdown)
	}
}

// Phase 2: file_path, both-params, neither-params, list, delete tests.

// TestProjectUpload_FilePath_Success verifies uploading via file_path.
func TestProjectUpload_FilePath_Success(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "upload.txt")
	content := []byte("file content for upload")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathProjectUploads || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		_ = r.ParseMultipartForm(10 << 20)
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()

		body, _ := io.ReadAll(file)
		if string(body) != string(content) {
			t.Errorf("file content mismatch: got %q", string(body))
		}

		testutil.RespondJSON(w, http.StatusCreated, `{
			"alt": "upload",
			"url": "/uploads/aaa/upload.txt",
			"full_path": "/g/p/uploads/aaa/upload.txt",
			"markdown": "![upload](/uploads/aaa/upload.txt)"
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID: "42",
		Filename:  "upload.txt",
		FilePath:  path,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.URL != "/uploads/aaa/upload.txt" {
		t.Errorf("URL = %q, want %q", out.URL, "/uploads/aaa/upload.txt")
	}
	if out.Alt != "upload" {
		t.Errorf("Alt = %q, want %q", out.Alt, "upload")
	}
	if out.Markdown != "![upload](/uploads/aaa/upload.txt)" {
		t.Errorf("Markdown = %q, want %q", out.Markdown, "![upload](/uploads/aaa/upload.txt)")
	}
	if out.FullPath != "/g/p/uploads/aaa/upload.txt" {
		t.Errorf("FullPath = %q, want %q", out.FullPath, "/g/p/uploads/aaa/upload.txt")
	}
}

// TestProjectUpload_BothParams_Error verifies error when both file_path and content_base64 are set.
func TestProjectUpload_BothParams_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID:     "42",
		Filename:      "test.txt",
		FilePath:      "/some/path",
		ContentBase64: "aGVsbG8=",
	})
	if err == nil {
		t.Fatal("expected error for both params")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("expected 'not both' error, got: %v", err)
	}
}

// TestProjectUpload_NeitherParams_Error verifies error when neither file_path nor content_base64 is set.
func TestProjectUpload_NeitherParams_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID: "42",
		Filename:  "test.txt",
	})
	if err == nil {
		t.Fatal("expected error for neither params")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' error, got: %v", err)
	}
}

// TestProjectUpload_FilePath_NotFound verifies error when file_path doesn't exist.
func TestProjectUpload_FilePath_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID: "42",
		Filename:  "test.txt",
		FilePath:  "/nonexistent/file.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestProjectUpload_FilePath_IsDirectory verifies error when file_path is a directory.
func TestProjectUpload_FilePath_IsDirectory(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID: "42",
		Filename:  "test.txt",
		FilePath:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("expected 'not a regular file' error, got: %v", err)
	}
}

// TestProjectUpload_FilePath_TooLarge verifies error when file exceeds the configured max file size.
func TestProjectUpload_FilePath_TooLarge(t *testing.T) {
	// Set a small max to avoid allocating real 2 GB in tests
	original := toolutil.GetUploadConfig()
	toolutil.SetUploadConfig(5 * 1024 * 1024)
	defer toolutil.SetUploadConfig(original.MaxFileSize)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "huge.bin")
	// Create a file just over the configured max (5 MB)
	if err := os.WriteFile(path, make([]byte, 6*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))

	_, err := Upload(context.Background(), nil, client, UploadInput{
		ProjectID: "42",
		Filename:  "huge.bin",
		FilePath:  path,
	})
	if err == nil {
		t.Fatal("expected error for too-large file")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' error, got: %v", err)
	}
}

// TestProjectUploadList_Success verifies listing markdown uploads.
func TestProjectUploadList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathProjectUploads {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": 1, "size": 1024, "filename": "test.png", "uploaded_by": {"username": "admin"}},
			{"id": 2, "size": 2048, "filename": "doc.pdf"}
		]`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(out.Uploads))
	}
	if out.Uploads[0].Filename != "test.png" {
		t.Errorf("expected filename 'test.png', got %q", out.Uploads[0].Filename)
	}
	if out.Uploads[0].ID != 1 {
		t.Errorf("Uploads[0].ID = %d, want 1", out.Uploads[0].ID)
	}
	if out.Uploads[0].Size != 1024 {
		t.Errorf("Uploads[0].Size = %d, want 1024", out.Uploads[0].Size)
	}
	if out.Uploads[0].UploadedBy != "admin" {
		t.Errorf("expected uploaded_by 'admin', got %q", out.Uploads[0].UploadedBy)
	}
	if out.Uploads[1].ID != 2 {
		t.Errorf("Uploads[1].ID = %d, want 2", out.Uploads[1].ID)
	}
	if out.Uploads[1].Filename != "doc.pdf" {
		t.Errorf("Uploads[1].Filename = %q, want %q", out.Uploads[1].Filename, "doc.pdf")
	}
	if out.Uploads[1].Size != 2048 {
		t.Errorf("Uploads[1].Size = %d, want 2048", out.Uploads[1].Size)
	}
	if out.Uploads[1].UploadedBy != "" {
		t.Errorf("expected empty uploaded_by for second upload, got %q", out.Uploads[1].UploadedBy)
	}
}

// TestProjectUploadDelete_Success verifies deleting a markdown upload.
func TestProjectUploadDelete_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		UploadID:  1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestFormatUploadMarkdown verifies the Markdown output includes alt, URL,
// full_url, and markdown fields.
func TestFormatUploadMarkdown(t *testing.T) {
	out := UploadOutput{
		Alt:      "screenshot.png",
		URL:      "/uploads/a1b2/screenshot.png",
		FullURL:  "https://gitlab.example.com/uploads/a1b2/screenshot.png",
		Markdown: "![screenshot.png](/uploads/a1b2/screenshot.png)",
	}
	md := FormatUploadMarkdown(out)
	if !strings.Contains(md, "## File Uploaded") {
		t.Error("expected header in markdown")
	}
	if !strings.Contains(md, "screenshot.png") {
		t.Error("expected alt in markdown")
	}
	if !strings.Contains(md, "- **URL**: [") {
		t.Error("expected full URL in markdown")
	}
	if !strings.Contains(md, "Markdown") {
		t.Error("expected markdown field")
	}
}

// TestFormatUploadMarkdown_NoFullURL verifies the output omits Full URL when empty.
func TestFormatUploadMarkdown_NoFullURL(t *testing.T) {
	out := UploadOutput{
		Alt:      "file.txt",
		URL:      "/uploads/a1b2/file.txt",
		Markdown: "![file.txt](/uploads/a1b2/file.txt)",
	}
	md := FormatUploadMarkdown(out)
	if strings.Contains(md, "Full URL") {
		t.Error("should not contain Full URL when empty")
	}
}

// TestUploadToolResult_Image verifies that image files get an inline embed.
func TestUploadToolResult_Image(t *testing.T) {
	out := UploadOutput{
		Alt:      "screenshot.png",
		URL:      "/uploads/a1b2/screenshot.png",
		FullURL:  "https://gitlab.example.com/uploads/a1b2/screenshot.png",
		Markdown: "![screenshot.png](/uploads/a1b2/screenshot.png)",
	}
	result := UploadToolResult(out)
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected non-empty result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "![screenshot.png]") {
		t.Error("expected inline image embed for image file")
	}
}

// TestUploadToolResult_NonImage verifies that non-image files don't get an inline embed.
func TestUploadToolResult_NonImage(t *testing.T) {
	out := UploadOutput{
		Alt:      "report.pdf",
		URL:      "/uploads/a1b2/report.pdf",
		FullURL:  "https://gitlab.example.com/uploads/a1b2/report.pdf",
		Markdown: "![report.pdf](/uploads/a1b2/report.pdf)",
	}
	result := UploadToolResult(out)
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected non-empty result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	// The embed pattern adds ![alt](full_url) with the full URL, not the relative URL
	if strings.Contains(tc.Text, "![report.pdf](https://") {
		t.Error("non-image should not have inline image embed with full URL")
	}
}

// TestList_MissingProjectID verifies List returns error for empty project_id.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestList_CancelledContext verifies List returns error for cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestList_APIError verifies List returns error on API failure.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestList_WithTimestampAndUploader verifies List maps CreatedAt and UploadedBy.
func TestList_WithTimestampAndUploader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`[{"id":1,"size":1024,"filename":"file.txt","created_at":"2026-01-01T00:00:00Z","uploaded_by":{"username":"admin"}}]`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 1 {
		t.Fatalf("got %d uploads, want 1", len(out.Uploads))
	}
	if out.Uploads[0].UploadedBy != "admin" {
		t.Errorf("UploadedBy = %q, want %q", out.Uploads[0].UploadedBy, "admin")
	}
	if out.Uploads[0].CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
}

// TestDelete_MissingProjectID verifies Delete returns error for empty project_id.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Delete(context.Background(), client, DeleteInput{UploadID: 1})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDelete_MissingUploadID verifies Delete returns error for zero upload_id.
func TestDelete_MissingUploadID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing upload_id")
	}
}

// TestDelete_APIError verifies Delete returns error on API failure.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", UploadID: 5})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestDelete_CancelledContext verifies Delete returns error for cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", UploadID: 5})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools does not panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered tools can be called
// through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/uploads"):
			testutil.RespondJSON(w, http.StatusCreated,
				`{"alt":"file.txt","url":"/uploads/a1/file.txt","full_path":"/uploads/a1/file.txt","markdown":"![file.txt](/uploads/a1/file.txt)"}`)
		case r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"size":100,"filename":"file.txt"}]`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_project_upload", map[string]any{"project_id": "42", "filename": "file.txt", "content_base64": base64.StdEncoding.EncodeToString([]byte("hello"))}},
		{"gitlab_project_upload_list", map[string]any{"project_id": "42"}},
		{"gitlab_project_upload_delete", map[string]any{"project_id": "42", "upload_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tt.name)
			}
		})
	}
}
