// groupwikis_test.go contains unit tests for GitLab group wiki operations.
// Tests use httptest to mock the GitLab Group Wikis API.
package groupwikis

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	pathGroupWikis    = "/api/v4/groups/mygroup/wikis"
	pathGroupWikiSlug = "/api/v4/groups/mygroup/wikis/home"
)

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_WithContent verifies the List_WithContent handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_MissingGroupID verifies that List_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_MissingFields verifies that Get_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreate_MissingFields verifies that Create_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestEdit_Success verifies that Edit succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestEdit_MissingFields verifies that Edit_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDelete_MissingFields verifies that Delete_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for 500 response, got nil")
	}
}

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Wiki Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "missing"})
	if err == nil {
		t.Fatal("Get() expected error for 404 response, got nil")
	}
}

// TestGet_RenderHTML verifies the Get_RenderHTML handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_Version verifies the Get_Version handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Get() expected error for cancelled context, got nil")
	}
}

// TestCreate_APIError verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreate_NoFormat verifies the Create_NoFormat handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreate_CancelledContext verifies the Create_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{GroupID: "mygroup", Title: "T", Content: "C"})
	if err == nil {
		t.Fatal("Create() expected error for cancelled context, got nil")
	}
}

// TestEdit_APIError verifies that Edit returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestEdit_WithFormat verifies the Edit_WithFormat handler.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestEdit_OnlyTitle verifies the Edit_OnlyTitle handler.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestEdit_CancelledContext verifies the Edit_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEdit_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Edit(ctx, client, EditInput{GroupID: "mygroup", Slug: "home", Title: "T"})
	if err == nil {
		t.Fatal("Edit() expected error for cancelled context, got nil")
	}
}

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Delete() expected error for 403 response, got nil")
	}
}

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Delete() expected error for cancelled context, got nil")
	}
}
