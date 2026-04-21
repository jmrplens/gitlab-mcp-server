// groupwikis_test.go contains unit tests for GitLab group wiki operations.
// Tests use httptest to mock the GitLab Group Wikis API.
package groupwikis

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	pathGroupWikis    = "/api/v4/groups/mygroup/wikis"
	pathGroupWikiSlug = "/api/v4/groups/mygroup/wikis/home"
)

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupWikis {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"title":"Home","slug":"home","format":"markdown"},
				{"title":"Getting Started","slug":"getting-started","format":"markdown"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.WikiPages) != 2 {
		t.Fatalf("len(WikiPages) = %d, want 2", len(out.WikiPages))
	}
	if out.WikiPages[0].Title != "Home" {
		t.Errorf("WikiPages[0].Title = %q, want %q", out.WikiPages[0].Title, "Home")
	}
}

func TestList_WithContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupWikis {
			testutil.AssertQueryParam(t, r, "with_content", "true")
			testutil.RespondJSON(w, http.StatusOK, `[
				{"title":"Home","slug":"home","format":"markdown","content":"# Welcome","encoding":"UTF-8"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup", WithContent: true})
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

func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupWikiSlug {
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Home","slug":"home","format":"markdown","content":"# Welcome","encoding":"utf-8"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "home"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Title != "Home" {
		t.Errorf("Title = %q, want %q", out.Title, "Home")
	}
	if out.Content != "# Welcome" {
		t.Errorf("Content = %q, want %q", out.Content, "# Welcome")
	}
}

func TestGet_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() expected error for missing group_id, got nil")
	}
	_, err = Get(context.Background(), client, GetInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("Get() expected error for missing slug, got nil")
	}
}

func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupWikis {
			testutil.RespondJSON(w, http.StatusCreated, `{"title":"Home","slug":"home","format":"markdown","content":"# Welcome"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID: "mygroup",
		Title:   "Home",
		Content: "# Welcome",
		Format:  "markdown",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Title != "Home" {
		t.Errorf("Title = %q, want %q", out.Title, "Home")
	}
}

func TestCreate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("Create() expected error for missing group_id, got nil")
	}
	_, err = Create(context.Background(), client, CreateInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("Create() expected error for missing title, got nil")
	}
	_, err = Create(context.Background(), client, CreateInput{GroupID: "mygroup", Title: "Home"})
	if err == nil {
		t.Fatal("Create() expected error for missing content, got nil")
	}
}

func TestEdit_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupWikiSlug {
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Updated","slug":"home","format":"markdown","content":"Updated content"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Edit(context.Background(), client, EditInput{
		GroupID: "mygroup",
		Slug:    "home",
		Title:   "Updated",
		Content: "Updated content",
	})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if out.Title != "Updated" {
		t.Errorf("Title = %q, want %q", out.Title, "Updated")
	}
}

func TestEdit_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Edit(context.Background(), client, EditInput{})
	if err == nil {
		t.Fatal("Edit() expected error for missing group_id, got nil")
	}
	_, err = Edit(context.Background(), client, EditInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("Edit() expected error for missing slug, got nil")
	}
}

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGroupWikiSlug {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: "mygroup", Slug: "home"})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

func TestDelete_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id, got nil")
	}
	err = Delete(context.Background(), client, DeleteInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("Delete() expected error for missing slug, got nil")
	}
}

// TestList_APIError verifies that List propagates GitLab API errors.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for 500 response, got nil")
	}
}

// TestList_EmptyResult verifies that List returns an empty slice when no pages exist.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.WikiPages) != 0 {
		t.Errorf("len(WikiPages) = %d, want 0", len(out.WikiPages))
	}
}

// TestGet_APIError verifies that Get propagates GitLab API errors.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Wiki Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "missing"})
	if err == nil {
		t.Fatal("Get() expected error for 404 response, got nil")
	}
}

// TestGet_RenderHTML verifies that Get passes the render_html query parameter.
func TestGet_RenderHTML(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupWikiSlug {
			testutil.AssertQueryParam(t, r, "render_html", "true")
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Home","slug":"home","format":"markdown","content":"<h1>Welcome</h1>"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "home", RenderHTML: true})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Content != "<h1>Welcome</h1>" {
		t.Errorf("Content = %q, want %q", out.Content, "<h1>Welcome</h1>")
	}
}

// TestGet_Version verifies that Get passes the version query parameter.
func TestGet_Version(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupWikiSlug {
			testutil.AssertQueryParam(t, r, "version", "abc123")
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Home","slug":"home","format":"markdown","content":"old content"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "home", Version: "abc123"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Content != "old content" {
		t.Errorf("Content = %q, want %q", out.Content, "old content")
	}
}

// TestGet_CancelledContext verifies that Get returns an error when the
// context is cancelled before the API call.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, client, GetInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Get() expected error for cancelled context, got nil")
	}
}

// TestCreate_APIError verifies that Create propagates GitLab API errors.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		GroupID: "mygroup",
		Title:   "Home",
		Content: "content",
	})
	if err == nil {
		t.Fatal("Create() expected error for 422 response, got nil")
	}
}

// TestCreate_NoFormat verifies that Create works without specifying a format,
// exercising the branch where Format is empty.
func TestCreate_NoFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupWikis {
			testutil.RespondJSON(w, http.StatusCreated, `{"title":"New Page","slug":"new-page","format":"markdown","content":"hello"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID: "mygroup",
		Title:   "New Page",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Title != "New Page" {
		t.Errorf("Title = %q, want %q", out.Title, "New Page")
	}
}

// TestCreate_CancelledContext verifies that Create returns an error when the
// context is cancelled before the API call.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, client, CreateInput{GroupID: "mygroup", Title: "T", Content: "C"})
	if err == nil {
		t.Fatal("Create() expected error for cancelled context, got nil")
	}
}

// TestEdit_APIError verifies that Edit propagates GitLab API errors.
func TestEdit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Edit(context.Background(), client, EditInput{
		GroupID: "mygroup",
		Slug:    "home",
		Title:   "Updated",
	})
	if err == nil {
		t.Fatal("Edit() expected error for 500 response, got nil")
	}
}

// TestEdit_WithFormat verifies that Edit passes the format option when set.
func TestEdit_WithFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupWikiSlug {
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Home","slug":"home","format":"asciidoc","content":"= Title"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Edit(context.Background(), client, EditInput{
		GroupID: "mygroup",
		Slug:    "home",
		Content: "= Title",
		Format:  "asciidoc",
	})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if out.Format != "asciidoc" {
		t.Errorf("Format = %q, want %q", out.Format, "asciidoc")
	}
}

// TestEdit_OnlyTitle verifies that Edit works when only the title is updated,
// exercising partial-update branches (no content, no format).
func TestEdit_OnlyTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupWikiSlug {
			testutil.RespondJSON(w, http.StatusOK, `{"title":"Renamed","slug":"home","format":"markdown"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Edit(context.Background(), client, EditInput{
		GroupID: "mygroup",
		Slug:    "home",
		Title:   "Renamed",
	})
	if err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	if out.Title != "Renamed" {
		t.Errorf("Title = %q, want %q", out.Title, "Renamed")
	}
}

// TestEdit_CancelledContext verifies that Edit returns an error when the
// context is cancelled before the API call.
func TestEdit_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Edit(ctx, client, EditInput{GroupID: "mygroup", Slug: "home", Title: "T"})
	if err == nil {
		t.Fatal("Edit() expected error for cancelled context, got nil")
	}
}

// TestDelete_APIError verifies that Delete propagates GitLab API errors.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Delete() expected error for 403 response, got nil")
	}
}

// TestDelete_CancelledContext verifies that Delete returns an error when the
// context is cancelled before the API call.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Delete(ctx, client, DeleteInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Delete() expected error for cancelled context, got nil")
	}
}
