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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// pathProjectWikis identifies the path project wikis constant used by this package.
	pathProjectWikis = "/api/v4/projects/42/wikis"
	// pathProjectWikiSlug identifies the path project wiki slug constant used by this package.
	pathProjectWikiSlug = "/api/v4/projects/42/wikis/my-page"
)

// TestWikiList_Success verifies WikiList when success.
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

// TestWikiList_WithContent verifies WikiList when with content.
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

// TestWikiList_EmptyProjectID verifies WikiList when empty project ID.
func TestWikiList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiListServer_Error verifies WikiListServer when error.
func TestWikiListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
}

// TestWikiList_CancelledContext verifies WikiList when cancelled context.
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

// TestFormatWikiNotFound verifies not-found result formatting for wiki pages.
func TestFormatWikiNotFound(t *testing.T) {
	result := formatWikiNotFound(wikiNotFoundOutput{Identifier: "home in project 42"})
	if result == nil || !result.IsError {
		t.Fatalf("formatWikiNotFound() = %+v, want error result", result)
	}
}

// Get.

// TestWikiGet_Success verifies WikiGet when success.
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

// TestWikiGet_WithRenderHTML verifies WikiGet when with render HTML.
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

// TestWikiGet_EmptyProjectID verifies WikiGet when empty project ID.
func TestWikiGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{Slug: "my-page"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiGet_EmptySlug verifies WikiGet when empty slug.
func TestWikiGet_EmptySlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Get() expected error for empty slug, got nil")
	}
}

// TestWikiGet_NotFound verifies WikiGet when not found.
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

// TestWikiGet_CancelledContext verifies WikiGet when cancelled context.
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

// TestWikiCreate_Success verifies WikiCreate when success.
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

// TestWikiCreate_WithFormat verifies WikiCreate when with format.
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

// TestWikiCreate_EmptyProjectID verifies WikiCreate when empty project ID.
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

// TestWikiCreate_EmptyTitle verifies WikiCreate when empty title.
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

// TestWikiCreate_EmptyContent verifies WikiCreate when empty content.
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

// TestWikiCreate_CancelledContext verifies WikiCreate when cancelled context.
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

// TestWikiUpdate_Success verifies WikiUpdate when success.
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

// TestWikiUpdate_EmptyProjectID verifies WikiUpdate when empty project ID.
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

// TestWikiUpdate_EmptySlug verifies WikiUpdate when empty slug.
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

// TestWikiUpdate_CancelledContext verifies WikiUpdate when cancelled context.
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

// TestWikiDelete_Success verifies WikiDelete when success.
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

// TestWikiDelete_EmptyProjectID verifies WikiDelete when empty project ID.
func TestWikiDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{Slug: "my-page"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestWikiDelete_EmptySlug verifies WikiDelete when empty slug.
func TestWikiDelete_EmptySlug(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("Delete() expected error for empty slug, got nil")
	}
}

// TestWikiDelete_NotFound verifies WikiDelete when not found.
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

// TestWikiDelete_CancelledContext verifies WikiDelete when cancelled context.
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

// pathProjectWikiAttachments identifies the path project wiki attachments constant used by this package.
const pathProjectWikiAttachments = "/api/v4/projects/42/wikis/attachments"

// TestUploadAttachment_Base64Success verifies UploadAttachment when base 64 success.
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

// TestUploadAttachment_MissingProjectID verifies UploadAttachment when missing project ID.
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

// TestUploadAttachment_MissingFilename verifies UploadAttachment when missing filename.
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

// TestUploadAttachment_BothContentAndFilePath verifies UploadAttachment when both content and file path.
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

// TestUploadAttachment_NeitherContentNorFilePath verifies UploadAttachment when neither content nor file path.
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

// TestUploadAttachment_InvalidBase64 verifies UploadAttachment when invalid base 64.
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

// TestUploadAttachment_APIError verifies UploadAttachment when API error.
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

// TestFormatAttachmentMarkdownString verifies FormatAttachmentMarkdownString.
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
	// errExpected identifies the err expected constant used by this package.
	errExpected = "expected error"
	// fmtUnexpectedErr identifies the fmt unexpected err constant used by this package.
	fmtUnexpectedErr = "unexpected error: %v"
	// testFileName identifies the test file name constant used by this package.
	testFileName = "test.txt"
	// errExpectedNonNil identifies the err expected non nil constant used by this package.
	errExpectedNonNil = "expected non-nil"
)

// ---------------------------------------------------------------------------
// Get with Version parameter
// ---------------------------------------------------------------------------.

// TestGet_WithVersion verifies Get when with version.
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

// TestUpdate_WithFormat verifies Update when with format.
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

// TestUpdate_ServerError verifies Update when server error.
func TestUpdate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", Slug: "p", Title: "t"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestCreate_ServerError verifies Create when server error.
func TestCreate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "t", Content: "c"})
	if err == nil {
		t.Fatal(errExpected)
	}
}

// TestGet_ServerError verifies Get when server error.
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

// TestUploadAttachment_FilePath verifies UploadAttachment when file path.
func TestUploadAttachment_FilePath(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, testFileName)
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o600); err != nil {
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

// TestUploadAttachment_NoBranch verifies UploadAttachment when no branch.
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

// TestFormatOutputMarkdownString_WithEncodingAndContent verifies FormatOutputMarkdownString when with encoding and content.
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

// TestFormatOutputMarkdownString_Minimal verifies FormatOutputMarkdownString when minimal.
func TestFormatOutputMarkdownString_Minimal(t *testing.T) {
	s := FormatOutputMarkdownString(Output{Title: "T", Slug: "t", Format: "markdown"})
	if strings.Contains(s, "Encoding") {
		t.Error("should not include Encoding")
	}
	if strings.Contains(s, "Content") {
		t.Error("should not include Content section")
	}
}

// TestFormatOutputMarkdown_NonNil verifies FormatOutputMarkdown when non nil.
func TestFormatOutputMarkdown_NonNil(t *testing.T) {
	r := FormatOutputMarkdown(Output{Title: "T"})
	if r == nil {
		t.Error(errExpectedNonNil)
	}
}

// TestFormatListMarkdownString_WithPages verifies FormatListMarkdownString when with pages.
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

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(s, "No wiki pages found") {
		t.Error("expected empty message")
	}
}

// TestFormatListMarkdown_NonNil verifies FormatListMarkdown when non nil.
func TestFormatListMarkdown_NonNil(t *testing.T) {
	r := FormatListMarkdown(ListOutput{})
	if r == nil {
		t.Error(errExpectedNonNil)
	}
}

// TestFormatAttachmentMarkdownString_NoBranch verifies FormatAttachmentMarkdownString when no branch.
func TestFormatAttachmentMarkdownString_NoBranch(t *testing.T) {
	s := FormatAttachmentMarkdownString(AttachmentOutput{FileName: "f", FilePath: "p", URL: "u", Markdown: "m"})
	if strings.Contains(s, "Branch") {
		t.Error("should not include Branch")
	}
}

// TestFormatAttachmentMarkdown_NonNil verifies FormatAttachmentMarkdown when non nil.
func TestFormatAttachmentMarkdown_NonNil(t *testing.T) {
	r := FormatAttachmentMarkdown(AttachmentOutput{FileName: "f"})
	if r == nil {
		t.Error(errExpectedNonNil)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for wiki actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := wikiSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "wikis" {
			t.Fatalf("OwnerPackage for %s = %q, want wikis", spec.Name, spec.OwnerPackage)
		}
	}
}

// TestActionSpecs_CallAllRoutes validates wiki routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
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
	byTool := wikiSpecsByTool(t, ActionSpecs(client))

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
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// TestActionSpecs_WikiGetRoute verifies the canonical wiki get route output.
func TestActionSpecs_WikiGetRoute(t *testing.T) {
	const respJSON = `{"title":"Home","slug":"Home","format":"markdown","content":"hello","encoding":"UTF-8"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/wikis/Home") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := wikiSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_wiki_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "slug": "Home"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.Slug != "Home" || out.Title != "Home" {
		t.Fatalf("wiki output = %#v, want title and slug Home", out)
	}
}

// wikiSpecsByTool supports wiki specs by tool assertions in wikis tests.
func wikiSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
