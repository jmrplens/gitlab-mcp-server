// group_wikis_test.go contains unit tests for GitLab group wiki operations.
// Tests use httptest to mock the GitLab Group Wikis API.
package groupwikis

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
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

// TestList_MissingGroupID verifies that List refuses a call naming no group
// before it reaches GitLab. ForbiddenHandler is what makes the second half an
// assertion rather than a claim: it fails the test if any request arrives.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
}

// TestList_CancelledContext verifies that a canceled context aborts List
// before it reaches GitLab, with ForbiddenHandler asserting the second half.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestGet_MissingFields verifies that Get refuses a call missing either
// required field before it reaches GitLab, one field at a time so each guard
// is the one refusing. ForbiddenHandler asserts that no request is made.
func TestGet_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestCreate_MissingFields verifies that Create refuses a call missing any of
// its three required fields before it reaches GitLab, one field at a time so
// each guard is the one refusing. ForbiddenHandler asserts no request is made.
func TestCreate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestEdit_MissingFields verifies that Edit refuses a call missing either
// identifying field before it reaches GitLab, one field at a time so each
// guard is the one refusing. ForbiddenHandler asserts no request is made.
func TestEdit_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestDelete_MissingFields verifies that Delete refuses a call missing either
// identifying field before it reaches GitLab, one field at a time so each
// guard is the one refusing. ForbiddenHandler asserts no request is made,
// which matters most here: the call it would make is destructive.
func TestDelete_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
		t.Fatal("List() expected error for 403 response, got nil")
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

// TestGet_CancelledContext verifies that a canceled context aborts Get before
// it reaches GitLab, with ForbiddenHandler asserting the second half.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestCreate_CancelledContext verifies that a canceled context aborts Create
// before it reaches GitLab, with ForbiddenHandler asserting the second half:
// a page created after the caller went away would be one nobody asked for.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
		t.Fatal("Edit() expected error for 403 response, got nil")
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

// TestEdit_CancelledContext verifies that a canceled context aborts Edit
// before it reaches GitLab, with ForbiddenHandler asserting the second half:
// an edit landing after the caller went away would overwrite a live page.
func TestEdit_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestGroupWikis_UnreadableCapturedMetaID verifies that every group wiki
// handler returns an error rather than a half-filled page when GitLab sends
// wiki_page_meta_id as something that is not a number. The SDK ignores the key
// its own Wiki does not model, so the read of the captured response is the only
// thing that can notice.
func TestGroupWikis_UnreadableCapturedMetaID(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"slug":"home","title":"Home","wiki_page_meta_id":"not-a-number"}]`)
			_, err := List(context.Background(), client, ListInput{GroupID: "42"})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"slug":"home","title":"Home","wiki_page_meta_id":"not-a-number"}`)
			_, err := Get(context.Background(), client, GetInput{GroupID: "42", Slug: "home"})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"slug":"home","title":"Home","wiki_page_meta_id":"not-a-number"}`)
			_, err := Create(context.Background(), client, CreateInput{GroupID: "42", Title: "Home", Content: "hello"})
			return err
		}},
		{Name: "edit", Call: func() error {
			client := poisoned(`{"slug":"home","title":"Home","wiki_page_meta_id":"not-a-number"}`)
			_, err := Edit(context.Background(), client, EditInput{GroupID: "42", Slug: "home", Content: "hello again"})
			return err
		}},
	})
}

// writeBody records the JSON object one write handler sent GitLab. The record
// is taken inside the httptest handler and read back on the test goroutine,
// which is the contract for an assertion that cannot abort where it is made.
type writeBody struct {
	mu     sync.Mutex
	fields map[string]any
	err    error
	seen   bool
}

// record decodes the request body and keeps it for the test goroutine. A body
// that does not decode is kept as the error rather than reported here, so the
// mock still answers and the caller's own assertions run.
func (b *writeBody) record(r *http.Request) {
	var fields map[string]any
	err := json.NewDecoder(r.Body).Decode(&fields)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fields, b.err, b.seen = fields, err, true
}

// assertSent compares the whole body with want rather than the keys want names.
// A field GitLab was never sent is as much a defect as one sent wrongly, and
// only the whole object says both at once: an optional field whose guard is
// inverted disappears from the request without changing any field that is
// there.
func (b *writeBody) assertSent(t *testing.T, want map[string]any) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.seen {
		t.Fatal("no write request reached the mock GitLab")
	}
	if b.err != nil {
		t.Fatalf("decode request body: %v", b.err)
	}
	if !maps.Equal(b.fields, want) {
		t.Errorf("request body = %#v, want %#v", b.fields, want)
	}
}

// TestCreate_OptionalFormat_IsSentOnlyWhenNamed pins the body Create builds.
// The format a caller names must reach GitLab, and a call naming none must send
// no format key at all, leaving the instance's own markdown default in place.
// Nothing held the request before, only the response: with the guard inverted,
// every page would be created with an empty format and the one call that named
// a format would lose it, and each test still passed.
func TestCreate_OptionalFormat_IsSentOnlyWhenNamed(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
		want  map[string]any
	}{
		{
			name:  "the named format is sent",
			input: CreateInput{GroupID: "mygroup", Title: "Setup", Content: "= Setup", Format: "asciidoc"},
			want:  map[string]any{"title": "Setup", "content": "= Setup", "format": "asciidoc"},
		},
		{
			name:  "no format named sends no format key",
			input: CreateInput{GroupID: "mygroup", Title: "Home", Content: "# Welcome"},
			want:  map[string]any{"title": "Home", "content": "# Welcome"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body writeBody
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != pathGroupWikis {
					http.NotFound(w, r)
					return
				}
				body.record(r)
				testutil.RespondJSON(w, http.StatusCreated, `{"title":"Setup","slug":"setup","format":"asciidoc"}`)
			}))
			if _, err := Create(context.Background(), client, c.input); err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}
			body.assertSent(t, c.want)
		})
	}
}

// TestEdit_OnlyTheNamedFieldsAreSent pins the body Edit builds. Every field of
// an edit is optional and GitLab overwrites whatever the request carries, so a
// key sent for a field the caller left alone would blank that field on the
// page: a rename that also emptied the body would be indistinguishable, in the
// response, from a rename. Only the request says which is which.
func TestEdit_OnlyTheNamedFieldsAreSent(t *testing.T) {
	cases := []struct {
		name  string
		input EditInput
		want  map[string]any
	}{
		{
			name:  "a rename sends the title alone",
			input: EditInput{GroupID: "mygroup", Slug: "home", Title: "Renamed"},
			want:  map[string]any{"title": "Renamed"},
		},
		{
			name:  "new content sends the content alone",
			input: EditInput{GroupID: "mygroup", Slug: "home", Content: "# Rewritten"},
			want:  map[string]any{"content": "# Rewritten"},
		},
		{
			name:  "a format change travels with its content",
			input: EditInput{GroupID: "mygroup", Slug: "home", Content: "= Rewritten", Format: "asciidoc"},
			want:  map[string]any{"content": "= Rewritten", "format": "asciidoc"},
		},
		{
			name:  "all three named are all three sent",
			input: EditInput{GroupID: "mygroup", Slug: "home", Title: "Renamed", Content: "= Rewritten", Format: "asciidoc"},
			want:  map[string]any{"title": "Renamed", "content": "= Rewritten", "format": "asciidoc"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body writeBody
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != pathGroupWikiSlug {
					http.NotFound(w, r)
					return
				}
				body.record(r)
				testutil.RespondJSON(w, http.StatusOK, `{"title":"Renamed","slug":"home","format":"asciidoc"}`)
			}))
			if _, err := Edit(context.Background(), client, c.input); err != nil {
				t.Fatalf("Edit() unexpected error: %v", err)
			}
			body.assertSent(t, c.want)
		})
	}
}

// TestDelete_CancelledContext verifies that a canceled context aborts Delete
// before it reaches GitLab. ForbiddenHandler is what makes the second half of
// that an assertion: it fails the test if any request arrives at all.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "mygroup", Slug: "home"})
	if err == nil {
		t.Fatal("Delete() expected error for cancelled context, got nil")
	}
}

// TestGet_EveryFieldIsReadFromItsOwnSource pins the whole page one Get
// produces, from a fixture in which no two values agree. Nothing compared the
// whole object before, only one or two fields per test, so five of the seven
// assignments in toOutput could have traded places unnoticed; encoding, the
// metadata id and the front matter were read back by nothing at all, and
// dropping the last two left the suite green.
func TestGet_EveryFieldIsReadFromItsOwnSource(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != pathGroupWikiSlug {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"title":"Release Notes",
			"slug":"release-notes",
			"format":"asciidoc",
			"content":"= Release Notes",
			"encoding":"UTF-16",
			"wiki_page_meta_id":4711,
			"front_matter":{"layout":"post"}
		}`)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "home"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	want := Output{
		Title:          "Release Notes",
		Slug:           "release-notes",
		Format:         "asciidoc",
		Content:        "= Release Notes",
		Encoding:       "UTF-16",
		WikiPageMetaID: 4711,
		FrontMatter:    map[string]any{"layout": "post"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Get() = %#v, want %#v", out, want)
	}
}

// TestList_EachPageKeepsItsOwnCapturedFields pins the pairing between a
// decoded page and the metadata read beside it out of the captured response.
// The two arrive by different routes and are joined by index, so handing every
// page the first one's extras is a defect with no branch to flip; with one
// page per fixture, or with the same values on both, it is also one no
// assertion can see.
func TestList_EachPageKeepsItsOwnCapturedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != pathGroupWikis {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"title":"Home","slug":"home","format":"markdown","wiki_page_meta_id":11,"front_matter":{"pin":"top"}},
			{"title":"Setup","slug":"setup","format":"asciidoc","wiki_page_meta_id":22,"front_matter":{"pin":"bottom"}}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	want := []Output{
		{Title: "Home", Slug: "home", Format: "markdown", WikiPageMetaID: 11, FrontMatter: map[string]any{"pin": "top"}},
		{Title: "Setup", Slug: "setup", Format: "asciidoc", WikiPageMetaID: 22, FrontMatter: map[string]any{"pin": "bottom"}},
	}
	if !reflect.DeepEqual(out.WikiPages, want) {
		t.Errorf("List() pages = %#v, want %#v", out.WikiPages, want)
	}
}

// queryRecord keeps the query one read handler was called with, so the whole
// of it can be compared on the test goroutine. A parameter this server sends
// on every call reads, in the response, exactly like one the caller asked for;
// only the request tells the two apart.
type queryRecord struct {
	mu     sync.Mutex
	values url.Values
	seen   bool
}

// record keeps the query for the test goroutine and asserts nothing here,
// where a failure could not abort the test it belongs to.
func (q *queryRecord) record(r *http.Request) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.values, q.seen = r.URL.Query(), true
}

// assertSent compares the whole query with want rather than the keys want
// names, so a parameter sent that nobody asked for fails as loudly as one the
// caller named and this server dropped.
func (q *queryRecord) assertSent(t *testing.T, want url.Values) {
	t.Helper()
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.seen {
		t.Fatal("no read request reached the mock GitLab")
	}
	equal := func(a, b []string) bool { return slices.Equal(a, b) }
	if !maps.EqualFunc(q.values, want, equal) {
		t.Errorf("query = %v, want %v", q.values, want)
	}
}

// TestGet_OptionalQueryParams_AreSentOnlyWhenNamed pins the query Get builds.
// Both options are optional and GitLab reads each one it is given, so a
// render_html sent on every call returns HTML to a caller who asked for the
// source, and a version sent always pins every read to the empty SHA. The
// single-parameter assertions here before said a named option arrives and
// nothing about one nobody named: sending both unconditionally kept the suite
// green.
func TestGet_OptionalQueryParams_AreSentOnlyWhenNamed(t *testing.T) {
	cases := []struct {
		name  string
		input GetInput
		want  url.Values
	}{
		{
			name:  "a plain read sends neither",
			input: GetInput{GroupID: "mygroup", Slug: "home"},
			want:  url.Values{},
		},
		{
			name:  "render_html travels alone",
			input: GetInput{GroupID: "mygroup", Slug: "home", RenderHTML: true},
			want:  url.Values{"render_html": {"true"}},
		},
		{
			name:  "a version travels alone",
			input: GetInput{GroupID: "mygroup", Slug: "home", Version: "abc123"},
			want:  url.Values{"version": {"abc123"}},
		},
		{
			name:  "both named are both sent",
			input: GetInput{GroupID: "mygroup", Slug: "home", RenderHTML: true, Version: "abc123"},
			want:  url.Values{"render_html": {"true"}, "version": {"abc123"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var query queryRecord
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != pathGroupWikiSlug {
					http.NotFound(w, r)
					return
				}
				query.record(r)
				testutil.RespondJSON(w, http.StatusOK, `{"title":"Home","slug":"home","format":"markdown"}`)
			}))
			if _, err := Get(context.Background(), client, c.input); err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			query.assertSent(t, c.want)
		})
	}
}

// TestList_WithContent_IsSentOnlyWhenAsked pins the query List builds. The
// option's own usage line warns that it returns the body of every page, so a
// listing that sends it unasked costs a model the whole wiki in tokens; the
// response is the same shape either way, which is why only the request can say
// so.
func TestList_WithContent_IsSentOnlyWhenAsked(t *testing.T) {
	cases := []struct {
		name  string
		input ListInput
		want  url.Values
	}{
		{
			name:  "a plain listing sends no option",
			input: ListInput{GroupID: "mygroup"},
			want:  url.Values{},
		},
		{
			name:  "the option is sent when asked for",
			input: ListInput{GroupID: "mygroup", WithContent: true},
			want:  url.Values{"with_content": {"true"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var query queryRecord
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != pathGroupWikis {
					http.NotFound(w, r)
					return
				}
				query.record(r)
				testutil.RespondJSON(w, http.StatusOK, `[{"title":"Home","slug":"home","format":"markdown"}]`)
			}))
			if _, err := List(context.Background(), client, c.input); err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}
			query.assertSent(t, c.want)
		})
	}
}

// groupWikiRefusal names one handler's refusal path: the status its hint is
// written for, another status that must not draw that hint, the operation the
// error is labeled with, and the hint itself.
type groupWikiRefusal struct {
	name      string
	hinted    int
	other     int
	operation string
	hint      string
	call      func(client *gitlabclient.Client) error
}

// TestGroupWikis_RefusalsCarryTheirOwnOperationAndHint pins what each handler
// says when GitLab refuses. The operation label, the status the hint is
// written for and the hint text are three arguments no test read back, so any
// two handlers could have traded any of them: the list hint could name the
// delete role requirement, on the status delete answers with, under the get
// handler's label, and every assertion here would still have passed on
// err != nil alone. The two subtests are what separate them, since a hint is
// published only on its own status and the fallback carries none.
func TestGroupWikis_RefusalsCarryTheirOwnOperationAndHint(t *testing.T) {
	refusals := []groupWikiRefusal{
		{
			name: "list", hinted: http.StatusNotFound, other: http.StatusForbidden,
			operation: "listGroupWikis",
			hint:      "verify group_id with gitlab_group_get; group wikis require GitLab Premium or higher",
			call: func(client *gitlabclient.Client) error {
				_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
				return err
			},
		},
		{
			name: "get", hinted: http.StatusNotFound, other: http.StatusForbidden,
			operation: "getGroupWikiPage",
			hint:      "verify slug with gitlab_group_wiki_list; slugs are case-sensitive and use hyphens for spaces",
			call: func(client *gitlabclient.Client) error {
				_, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Slug: "home"})
				return err
			},
		},
		{
			name: "create", hinted: http.StatusBadRequest, other: http.StatusUnprocessableEntity,
			operation: "createGroupWikiPage",
			hint: "title and content are required; format must be 'markdown', 'rdoc', 'asciidoc', or 'org'; " +
				"group wikis require GitLab Premium or higher",
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{GroupID: "mygroup", Title: "Home", Content: "# Welcome"})
				return err
			},
		},
		{
			name: "edit", hinted: http.StatusNotFound, other: http.StatusForbidden,
			operation: "editGroupWikiPage",
			hint:      "verify slug with gitlab_group_wiki_list; slugs are case-sensitive",
			call: func(client *gitlabclient.Client) error {
				_, err := Edit(context.Background(), client, EditInput{GroupID: "mygroup", Slug: "home", Title: "Renamed"})
				return err
			},
		},
		{
			name: "delete", hinted: http.StatusForbidden, other: http.StatusNotFound,
			operation: "deleteGroupWikiPage",
			hint:      "deleting group wiki pages requires Maintainer or Owner role",
			call: func(client *gitlabclient.Client) error {
				return Delete(context.Background(), client, DeleteInput{GroupID: "mygroup", Slug: "home"})
			},
		},
	}
	for _, r := range refusals {
		t.Run(r.name, func(t *testing.T) {
			t.Run("the status its hint is written for", func(t *testing.T) {
				got := refusalMessage(t, r.hinted, r.call)
				assertRefusalOperation(t, got, r.operation)
				// The trailing colon is the boundary the wrapper writes before
				// the cause, so a hint that is a prefix of a sibling's — edit's
				// is exactly get's, shorter — cannot pass for it.
				if want := "Suggestion: " + r.hint + ":"; !strings.Contains(got, want) {
					t.Errorf("error %q does not carry %q", got, want)
				}
			})
			t.Run("another status carries no hint", func(t *testing.T) {
				got := refusalMessage(t, r.other, r.call)
				assertRefusalOperation(t, got, r.operation)
				if strings.Contains(got, "Suggestion: ") {
					t.Errorf("error %q suggests something on a status the hint was not written for", got)
				}
			})
		})
	}
}

// refusalMessage drives one handler against a GitLab that refuses with status
// and returns the message the caller is left holding.
func refusalMessage(t *testing.T, status int, call func(client *gitlabclient.Client) error) string {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, `{"message":"refused"}`)
	}))
	err := call(client)
	if err == nil {
		t.Fatalf("expected an error for status %d, got nil", status)
	}
	return err.Error()
}

// assertRefusalOperation holds the message to the operation its own handler
// names. The label opens the message, so a prefix check is exact.
func assertRefusalOperation(t *testing.T, got, operation string) {
	t.Helper()
	if !strings.HasPrefix(got, operation+": ") {
		t.Errorf("error %q is not labeled %q", got, operation)
	}
}
