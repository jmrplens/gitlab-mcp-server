// wikis_test.go contains unit tests for GitLab wiki page operations.
// Tests use httptest to mock the GitLab Wikis API.
package wikis

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pathProjectWikis    = "/api/v4/projects/42/wikis"
	pathProjectWikiSlug = "/api/v4/projects/42/wikis/my-page"
)

// TestWikiList_Success verifies the behavior of wiki list success.
func TestWikiList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectWikis {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"title":"Home","slug":"home","format":"markdown"},
				{"title":"Getting Started","slug":"getting-started","format":"markdown"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.WikiPages) != 2 {
		t.Fatalf("len(WikiPages) = %d, want 2", len(out.WikiPages))
	}
	if out.WikiPages[0].Title != "Home" {
		t.Errorf("WikiPages[0].Title = %q, want %q", out.WikiPages[0].Title, "Home")
	}
	if out.WikiPages[1].Slug != "getting-started" {
		t.Errorf("WikiPages[1].Slug = %q, want %q", out.WikiPages[1].Slug, "getting-started")
	}
}

// TestWikiList_WithContent verifies the behavior of wiki list with content.
func TestWikiList_WithContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectWikis {
			q := r.URL.Query()
			if q.Get("with_content") != "true" {
				t.Errorf("expected with_content=true, got %q", q.Get("with_content"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[
				{"title":"Home","slug":"home","format":"markdown","content":"# Welcome","encoding":"UTF-8"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:   "42",
		WithContent: true,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.WikiPages) != 1 {
		t.Fatalf("len(WikiPages) = %d, want 1", len(out.WikiPages))
	}
	if out.WikiPages[0].Content != "# Welcome" {
		t.Errorf("WikiPages[0].Content = %q, want %q", out.WikiPages[0].Content, "# Welcome")
	}
}

// TestWikiList_EmptyProjectID verifies the behavior of wiki list empty project i d.
func TestWikiList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiListServer_Error verifies the behavior of wiki list server error.
func TestWikiListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
}

// TestWikiList_CancelledContext verifies the behavior of wiki list cancelled context.
func TestWikiList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// Get.

// TestWikiGet_Success verifies the behavior of wiki get success.
func TestWikiGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectWikiSlug {
			testutil.RespondJSON(w, http.StatusOK, `{
				"title":"My Page",
				"slug":"my-page",
				"format":"markdown",
				"content":"# Hello World",
				"encoding":"UTF-8"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		Slug:      "my-page",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Title != "My Page" {
		t.Errorf("Title = %q, want %q", out.Title, "My Page")
	}
	if out.Content != "# Hello World" {
		t.Errorf("Content = %q, want %q", out.Content, "# Hello World")
	}
}

// TestWikiGet_WithRenderHTML verifies the behavior of wiki get with render h t m l.
func TestWikiGet_WithRenderHTML(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectWikiSlug {
			q := r.URL.Query()
			if q.Get("render_html") != "true" {
				t.Errorf("expected render_html=true, got %q", q.Get("render_html"))
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"title":"My Page",
				"slug":"my-page",
				"format":"markdown",
				"content":"<h1>Hello World</h1>"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:  "42",
		Slug:       "my-page",
		RenderHTML: true,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Content != "<h1>Hello World</h1>" {
		t.Errorf("Content = %q, want rendered HTML", out.Content)
	}
}

// TestWikiGet_EmptyProjectID verifies the behavior of wiki get empty project i d.
func TestWikiGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{Slug: "my-page"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiGet_EmptySlug verifies the behavior of wiki get empty slug.
func TestWikiGet_EmptySlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Get() expected error for empty slug, got nil")
	}
}

// TestWikiGet_NotFound verifies the behavior of wiki get not found.
func TestWikiGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Wiki Page Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		Slug:      "nonexistent",
	})
	if err == nil {
		t.Fatal("Get() expected error for not found page, got nil")
	}
}

// TestWikiGet_CancelledContext verifies the behavior of wiki get cancelled context.
func TestWikiGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", Slug: "my-page"})
	if err == nil {
		t.Fatal("Get() expected error for canceled context, got nil")
	}
}

// Create.

// TestWikiCreate_Success verifies the behavior of wiki create success.
func TestWikiCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectWikis {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"title":"New Page",
				"slug":"new-page",
				"format":"markdown",
				"content":"Hello world"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Title:     "New Page",
		Content:   "Hello world",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Title != "New Page" {
		t.Errorf("Title = %q, want %q", out.Title, "New Page")
	}
	if out.Slug != "new-page" {
		t.Errorf("Slug = %q, want %q", out.Slug, "new-page")
	}
}

// TestWikiCreate_WithFormat verifies the behavior of wiki create with format.
func TestWikiCreate_WithFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectWikis {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"title":"AsciiDoc Page",
				"slug":"asciidoc-page",
				"format":"asciidoc",
				"content":"= Title"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Title:     "AsciiDoc Page",
		Content:   "= Title",
		Format:    "asciidoc",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Format != "asciidoc" {
		t.Errorf("Format = %q, want %q", out.Format, "asciidoc")
	}
}

// TestWikiCreate_EmptyProjectID verifies the behavior of wiki create empty project i d.
func TestWikiCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Title:   "New Page",
		Content: "Hello world",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiCreate_EmptyTitle verifies the behavior of wiki create empty title.
func TestWikiCreate_EmptyTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Content:   "Hello world",
	})
	if err == nil {
		t.Fatal("Create() expected error for empty title, got nil")
	}
}

// TestWikiCreate_EmptyContent verifies the behavior of wiki create empty content.
func TestWikiCreate_EmptyContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Title:     "New Page",
	})
	if err == nil {
		t.Fatal("Create() expected error for empty content, got nil")
	}
}

// TestWikiCreate_CancelledContext verifies the behavior of wiki create cancelled context.
func TestWikiCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{
		ProjectID: "42",
		Title:     "New Page",
		Content:   "Hello world",
	})
	if err == nil {
		t.Fatal("Create() expected error for canceled context, got nil")
	}
}

// Update.

// TestWikiUpdate_Success verifies the behavior of wiki update success.
func TestWikiUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProjectWikiSlug {
			testutil.RespondJSON(w, http.StatusOK, `{
				"title":"Updated Page",
				"slug":"my-page",
				"format":"markdown",
				"content":"Updated content"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		Slug:      "my-page",
		Title:     "Updated Page",
		Content:   "Updated content",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Title != "Updated Page" {
		t.Errorf("Title = %q, want %q", out.Title, "Updated Page")
	}
}

// TestWikiUpdate_EmptyProjectID verifies the behavior of wiki update empty project i d.
func TestWikiUpdate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		Slug:    "my-page",
		Content: "Updated content",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiUpdate_EmptySlug verifies the behavior of wiki update empty slug.
func TestWikiUpdate_EmptySlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		Content:   "Updated content",
	})
	if err == nil {
		t.Fatal("Update() expected error for empty slug, got nil")
	}
}

// TestWikiUpdate_CancelledContext verifies the behavior of wiki update cancelled context.
func TestWikiUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{
		ProjectID: "42",
		Slug:      "my-page",
		Content:   "Updated content",
	})
	if err == nil {
		t.Fatal("Update() expected error for canceled context, got nil")
	}
}

// Delete.

// TestWikiDelete_Success verifies the behavior of wiki delete success.
func TestWikiDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProjectWikiSlug {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		Slug:      "my-page",
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestWikiDelete_EmptyProjectID verifies the behavior of wiki delete empty project i d.
func TestWikiDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{Slug: "my-page"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiDelete_EmptySlug verifies the behavior of wiki delete empty slug.
func TestWikiDelete_EmptySlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Delete() expected error for empty slug, got nil")
	}
}

// TestWikiDelete_NotFound verifies the behavior of wiki delete not found.
func TestWikiDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Wiki Page Not Found"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		Slug:      "nonexistent",
	})
	if err == nil {
		t.Fatal("Delete() expected error for not found page, got nil")
	}
}

// TestWikiDelete_CancelledContext verifies the behavior of wiki delete cancelled context.
func TestWikiDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", Slug: "my-page"})
	if err == nil {
		t.Fatal("Delete() expected error for canceled context, got nil")
	}
}

// Upload Attachment Tests.

const pathProjectWikiAttachments = "/api/v4/projects/42/wikis/attachments"

// TestUploadAttachment_Base64Success verifies the behavior of upload attachment base64 success.
func TestUploadAttachment_Base64Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectWikiAttachments {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name":"diagram.png",
				"file_path":"uploads/abc123/diagram.png",
				"branch":"main",
				"link":{"url":"/uploads/abc123/diagram.png","markdown":"![diagram](uploads/abc123/diagram.png)"}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID:     "42",
		Filename:      "diagram.png",
		ContentBase64: "aGVsbG8=", // "hello"
		Branch:        "main",
	})
	if err != nil {
		t.Fatalf("UploadAttachment() unexpected error: %v", err)
	}
	if out.FileName != "diagram.png" {
		t.Errorf("FileName = %q, want %q", out.FileName, "diagram.png")
	}
	if out.FilePath != "uploads/abc123/diagram.png" {
		t.Errorf("FilePath = %q, want %q", out.FilePath, "uploads/abc123/diagram.png")
	}
	if out.Branch != "main" {
		t.Errorf("Branch = %q, want %q", out.Branch, "main")
	}
	if out.URL != "/uploads/abc123/diagram.png" {
		t.Errorf("URL = %q, want %q", out.URL, "/uploads/abc123/diagram.png")
	}
	if out.Markdown != "![diagram](uploads/abc123/diagram.png)" {
		t.Errorf("Markdown = %q, want %q", out.Markdown, "![diagram](uploads/abc123/diagram.png)")
	}
}

// TestUploadAttachment_MissingProjectID verifies the behavior of upload attachment missing project i d.
func TestUploadAttachment_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		Filename:      "file.png",
		ContentBase64: "aGVsbG8=",
	})
	if err == nil {
		t.Fatal("UploadAttachment() expected error for missing project_id, got nil")
	}
}

// TestUploadAttachment_MissingFilename verifies the behavior of upload attachment missing filename.
func TestUploadAttachment_MissingFilename(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID:     "42",
		ContentBase64: "aGVsbG8=",
	})
	if err == nil {
		t.Fatal("UploadAttachment() expected error for missing filename, got nil")
	}
}

// TestUploadAttachment_BothContentAndFilePath verifies the behavior of upload attachment both content and file path.
func TestUploadAttachment_BothContentAndFilePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID:     "42",
		Filename:      "file.png",
		ContentBase64: "aGVsbG8=",
		FilePath:      "/tmp/file.png",
	})
	if err == nil {
		t.Fatal("UploadAttachment() expected error when both content and file_path provided, got nil")
	}
}

// TestUploadAttachment_NeitherContentNorFilePath verifies the behavior of upload attachment neither content nor file path.
func TestUploadAttachment_NeitherContentNorFilePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID: "42",
		Filename:  "file.png",
	})
	if err == nil {
		t.Fatal("UploadAttachment() expected error when neither content nor file_path provided, got nil")
	}
}

// TestUploadAttachment_InvalidBase64 verifies the behavior of upload attachment invalid base64.
func TestUploadAttachment_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID:     "42",
		Filename:      "file.png",
		ContentBase64: "!!!invalid-base64!!!",
	})
	if err == nil {
		t.Fatal("UploadAttachment() expected error for invalid base64, got nil")
	}
}

// TestUploadAttachment_APIError verifies the behavior of upload attachment a p i error.
func TestUploadAttachment_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID:     "42",
		Filename:      "file.png",
		ContentBase64: "aGVsbG8=",
	})
	if err == nil {
		t.Fatal("UploadAttachment() expected error for API error, got nil")
	}
}

// TestFormatAttachmentMarkdownString verifies the behavior of format attachment markdown string.
func TestFormatAttachmentMarkdownString(t *testing.T) {
	out := AttachmentOutput{
		FileName: "diagram.png",
		FilePath: "uploads/abc/diagram.png",
		Branch:   "main",
		URL:      "/uploads/abc/diagram.png",
		Markdown: "![diagram](uploads/abc/diagram.png)",
	}
	md := FormatAttachmentMarkdownString(out)
	if md == "" {
		t.Fatal("FormatAttachmentMarkdownString() returned empty string")
	}
	if !strings.Contains(md, "diagram.png") {
		t.Errorf("markdown should contain filename")
	}
	if !strings.Contains(md, "main") {
		t.Errorf("markdown should contain branch")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	errExpected       = "expected error"
	fmtUnexpectedErr  = "unexpected error: %v"
	testFileName      = "test.txt"
	errExpectedNonNil = "expected non-nil"
)

// ---------------------------------------------------------------------------
// Get with Version parameter
// ---------------------------------------------------------------------------.

// TestGet_WithVersion verifies the behavior of get with version.
func TestGet_WithVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") != "abc123" {
			t.Errorf("expected version=abc123, got %q", r.URL.Query().Get("version"))
		}
		testutil.RespondJSON(w, http.StatusOK, `{"title":"Old","slug":"old","format":"markdown","content":"v1"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", Slug: "old", Version: "abc123"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.Content != "v1" {
		t.Errorf("Content = %q, want %q", out.Content, "v1")
	}
}

// ---------------------------------------------------------------------------
// Update with Format parameter
// ---------------------------------------------------------------------------.

// TestUpdate_WithFormat verifies the behavior of update with format.
func TestUpdate_WithFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"title":"P","slug":"p","format":"rdoc","content":"x"}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", Slug: "p", Format: "rdoc"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.Format != "rdoc" {
		t.Errorf("Format = %q, want rdoc", out.Format)
	}
}

// TestUpdate_ServerError verifies the behavior of update server error.
func TestUpdate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", Slug: "p", Title: "t"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestCreate_ServerError verifies the behavior of create server error.
func TestCreate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "t", Content: "c"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestGet_ServerError verifies the behavior of get server error.
func TestGet_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", Slug: "s"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// ---------------------------------------------------------------------------
// UploadAttachment with file path
// ---------------------------------------------------------------------------.

// TestUploadAttachment_FilePath verifies the behavior of upload attachment file path.
func TestUploadAttachment_FilePath(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, testFileName)
	if err := os.WriteFile(tmpFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"file_name":"test.txt","file_path":"uploads/x/test.txt","branch":"main","link":{"url":"/uploads/x/test.txt","markdown":"![test](uploads/x/test.txt)"}}`)
	}))

	out, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID: "42",
		Filename:  testFileName,
		FilePath:  tmpFile,
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if out.FileName != testFileName {
		t.Errorf("FileName = %q", out.FileName)
	}
}

// TestUploadAttachment_NoBranch verifies the behavior of upload attachment no branch.
func TestUploadAttachment_NoBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"file_name":"f.png","file_path":"uploads/x/f.png","branch":"","link":{"url":"/u","markdown":"![f](u)"}}`)
	}))

	_, err := UploadAttachment(context.Background(), client, UploadAttachmentInput{
		ProjectID:     "42",
		Filename:      "f.png",
		ContentBase64: "aGVsbG8=",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

// ---------------------------------------------------------------------------
// Formatter tests
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdownString_WithEncodingAndContent verifies the behavior of format output markdown string with encoding and content.
func TestFormatOutputMarkdownString_WithEncodingAndContent(t *testing.T) {
	s := FormatOutputMarkdownString(Output{
		Title: "Test", Slug: "test", Format: "markdown",
		Content: "# Hello", Encoding: "UTF-8",
	})
	if !strings.Contains(s, "Encoding") {
		t.Error("expected Encoding field")
	}
	if !strings.Contains(s, "# Hello") {
		t.Error("expected content")
	}
}

// TestFormatOutputMarkdownString_Minimal verifies the behavior of format output markdown string minimal.
func TestFormatOutputMarkdownString_Minimal(t *testing.T) {
	s := FormatOutputMarkdownString(Output{Title: "T", Slug: "t", Format: "markdown"})
	if strings.Contains(s, "Encoding") {
		t.Error("should not include Encoding")
	}
	if strings.Contains(s, "Content") {
		t.Error("should not include Content section")
	}
}

// TestFormatOutputMarkdown_NonNil verifies the behavior of format output markdown non nil.
func TestFormatOutputMarkdown_NonNil(t *testing.T) {
	r := FormatOutputMarkdown(Output{Title: "T"})
	if r == nil {
		t.Error(errExpectedNonNil)
	}
}

// TestFormatListMarkdownString_WithPages verifies the behavior of format list markdown string with pages.
func TestFormatListMarkdownString_WithPages(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{WikiPages: []Output{
		{Title: "Home", Slug: "home", Format: "markdown"},
		{Title: "FAQ", Slug: "faq", Format: "rdoc"},
	}})
	if !strings.Contains(s, "Home") {
		t.Error("expected Home")
	}
	if !strings.Contains(s, "FAQ") {
		t.Error("expected FAQ")
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(s, "No wiki pages found") {
		t.Error("expected empty message")
	}
}

// TestFormatListMarkdown_NonNil verifies the behavior of format list markdown non nil.
func TestFormatListMarkdown_NonNil(t *testing.T) {
	r := FormatListMarkdown(ListOutput{})
	if r == nil {
		t.Error(errExpectedNonNil)
	}
}

// TestFormatAttachmentMarkdownString_NoBranch verifies the behavior of format attachment markdown string no branch.
func TestFormatAttachmentMarkdownString_NoBranch(t *testing.T) {
	s := FormatAttachmentMarkdownString(AttachmentOutput{FileName: "f", FilePath: "p", URL: "u", Markdown: "m"})
	if strings.Contains(s, "Branch") {
		t.Error("should not include Branch")
	}
}

// TestFormatAttachmentMarkdown_NonNil verifies the behavior of format attachment markdown non nil.
func TestFormatAttachmentMarkdown_NonNil(t *testing.T) {
	r := FormatAttachmentMarkdown(AttachmentOutput{FileName: "f"})
	if r == nil {
		t.Error(errExpectedNonNil)
	}
}

// ---------------------------------------------------------------------------
// Registration and MCP round-trip
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	RegisterTools(server, client)
}

// TestMCPRoundTrip_AllWikiTools validates m c p round trip all wiki tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllWikiTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/wikis"):
			testutil.RespondJSON(w, http.StatusOK, `[{"title":"Home","slug":"home","format":"markdown"}]`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/wikis/"):
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Home","slug":"home","format":"markdown","content":"x"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/attachments"):
			testutil.RespondJSON(w, http.StatusOK, `{"file_name":"f","file_path":"p","branch":"main","link":{"url":"u","markdown":"m"}}`)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, `{"title":"New","slug":"new","format":"markdown","content":"c"}`)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Up","slug":"up","format":"markdown","content":"u"}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_wiki_list", map[string]any{"project_id": "42"}},
		{"gitlab_wiki_get", map[string]any{"project_id": "42", "slug": "home"}},
		{"gitlab_wiki_create", map[string]any{"project_id": "42", "title": "New", "content": "c"}},
		{"gitlab_wiki_update", map[string]any{"project_id": "42", "slug": "home", "title": "Up"}},
		{"gitlab_wiki_delete", map[string]any{"project_id": "42", "slug": "home"}},
		{"gitlab_wiki_upload_attachment", map[string]any{"project_id": "42", "filename": "f.png", "content_base64": "aGVsbG8="}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if result.IsError {
				t.Errorf("expected no error for %s", tc.name)
			}
		})
	}
}

// TestWikiGet_EmbedsCanonicalResource asserts gitlab_wiki_get attaches an
// EmbeddedResource block with URI gitlab://project/{id}/wiki/{slug}.
func TestWikiGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"title":"Home","slug":"Home","format":"markdown","content":"hello","encoding":"UTF-8"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/wikis/Home") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "slug": "Home"}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_wiki_get", args, "gitlab://project/42/wiki/Home", toolutil.EnableEmbeddedResources)
}
