// namespaces_test.go contains unit tests for the namespace MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package namespaces

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/namespaces" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"name":"root","path":"root","kind":"user","full_path":"root","web_url":"https://gitlab.example.com/root"},
			{"id":2,"name":"group1","path":"group1","kind":"group","full_path":"group1","web_url":"https://gitlab.example.com/groups/group1"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Namespaces) != 2 {
		t.Fatalf("got %d namespaces, want 2", len(out.Namespaces))
	}
	if out.Namespaces[0].Name != "root" {
		t.Errorf("got name %q, want %q", out.Namespaces[0].Name, "root")
	}
	if out.Namespaces[1].Kind != "group" {
		t.Errorf("got kind %q, want %q", out.Namespaces[1].Kind, "group")
	}
}

// TestList_WithSearch verifies List when with search.
func TestList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "test" {
			t.Errorf("expected search=test, got %q", r.URL.Query().Get("search"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":3,"name":"test-ns","path":"test-ns","kind":"group","full_path":"test-ns"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{Search: "test"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Namespaces) != 1 {
		t.Fatalf("got %d namespaces, want 1", len(out.Namespaces))
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/namespaces/42" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"myns","path":"myns","kind":"group","full_path":"mygroup/myns","web_url":"https://gitlab.example.com/groups/mygroup/myns","parent_id":10}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("got ID %d, want 42", out.ID)
	}
	if out.FullPath != "mygroup/myns" {
		t.Errorf("got full_path %q, want %q", out.FullPath, "mygroup/myns")
	}
	if out.ParentID != 10 {
		t.Errorf("got parent_id %d, want 10", out.ParentID)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := Get(context.Background(), client, GetInput{ID: "999"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestExists_Available verifies Exists when available.
func TestExists_Available(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/namespaces/new-path/exists" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"exists":false,"suggests":["new-path1","new-path2"]}`)
	}))

	out, err := Exists(context.Background(), client, ExistsInput{ID: "new-path"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Exists {
		t.Error("expected Exists=false, got true")
	}
	if len(out.Suggests) != 2 {
		t.Errorf("got %d suggestions, want 2", len(out.Suggests))
	}
}

// TestExists_Taken verifies Exists when taken.
func TestExists_Taken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/namespaces/taken-path/exists" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"exists":true,"suggests":[]}`)
	}))

	out, err := Exists(context.Background(), client, ExistsInput{ID: "taken-path"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Exists {
		t.Error("expected Exists=true, got false")
	}
}

// TestExists_WithParentID verifies Exists when with parent ID.
func TestExists_WithParentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("parent_id") != "5" {
			t.Errorf("expected parent_id=5, got %q", r.URL.Query().Get("parent_id"))
		}
		testutil.RespondJSON(w, http.StatusOK, `{"exists":false,"suggests":[]}`)
	}))

	out, err := Exists(context.Background(), client, ExistsInput{ID: "sub-path", ParentID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Exists {
		t.Error("expected Exists=false, got true")
	}
}

// TestExists_Error verifies Exists when error.
func TestExists_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Exists(context.Background(), client, ExistsInput{ID: "bad"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestSearch_Success verifies Search when success.
func TestSearch_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":10,"name":"myquery-ns","path":"myquery-ns","kind":"group","full_path":"myquery-ns"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := Search(context.Background(), client, SearchInput{Query: "myquery"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Namespaces) != 1 {
		t.Fatalf("got %d namespaces, want 1", len(out.Namespaces))
	}
	if out.Namespaces[0].Name != "myquery-ns" {
		t.Errorf("got name %q, want %q", out.Namespaces[0].Name, "myquery-ns")
	}
}

// TestSearch_Error verifies Search when error.
func TestSearch_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Search(context.Background(), client, SearchInput{Query: "fail"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{})
	if s != "No namespaces found.\n" {
		t.Errorf("unexpected output: %s", s)
	}
}

// TestFormatMarkdownString verifies FormatMarkdownString.
func TestFormatMarkdownString(t *testing.T) {
	s := FormatMarkdownString(Output{
		ID: 1, Name: "test", Path: "test", FullPath: "test", Kind: "user",
	})
	if s == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatExistsMarkdownString verifies FormatExistsMarkdownString.
func TestFormatExistsMarkdownString(t *testing.T) {
	s := FormatExistsMarkdownString(ExistsOutput{Exists: true, Suggests: []string{"a", "b"}})
	if s == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	// errExpNonNilResult identifies the err exp non nil result constant used by this package.
	errExpNonNilResult = "expected non-nil result"
	// errExpNonNil identifies the err exp non nil constant used by this package.
	errExpNonNil = "expected non-nil"
)

// Pre-built fixtures for toOutput tests.
var (
	avatarURL    = "https://avatar.url"
	nsWithAvatar = gl.Namespace{ID: 1, Name: "a", Path: "a", Kind: "group", FullPath: "a", AvatarURL: &avatarURL}
	nsNoAvatar   = gl.Namespace{ID: 2, Name: "b", Path: "b", Kind: "user", FullPath: "b", AvatarURL: nil}
)

// ---------------------------------------------------------------------------
// List with OwnedOnly and TopLevelOnly
// ---------------------------------------------------------------------------.

// TestList_OwnedAndTopLevel verifies List when owned and top level.
func TestList_OwnedAndTopLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("owned_only") != "true" {
			t.Errorf("expected owned_only=true, got %q", q.Get("owned_only"))
		}
		if q.Get("top_level_only") != "true" {
			t.Errorf("expected top_level_only=true, got %q", q.Get("top_level_only"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"mine","path":"mine","kind":"user","full_path":"mine"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{OwnedOnly: true, TopLevelOnly: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Namespaces) != 1 {
		t.Fatalf("got %d, want 1", len(out.Namespaces))
	}
}

// ---------------------------------------------------------------------------
// toOutput with AvatarURL
// ---------------------------------------------------------------------------.

// TestToOutput_WithAvatarURL verifies ToOutput when with avatar URL.
func TestToOutput_WithAvatarURL(t *testing.T) {
	ns := &nsWithAvatar
	o := toOutput(ns)
	if o.AvatarURL != "https://avatar.url" {
		t.Errorf("expected avatar URL, got %q", o.AvatarURL)
	}
}

// TestToOutput_NilAvatarURL verifies ToOutput when nil avatar URL.
func TestToOutput_NilAvatarURL(t *testing.T) {
	ns := &nsNoAvatar
	o := toOutput(ns)
	if o.AvatarURL != "" {
		t.Errorf("expected empty avatar URL, got %q", o.AvatarURL)
	}
}

// ---------------------------------------------------------------------------
// Formatter tests
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_WithItems verifies FormatListMarkdownString when with items.
func TestFormatListMarkdownString_WithItems(t *testing.T) {
	s := FormatListMarkdownString(ListOutput{
		Namespaces: []Output{
			{ID: 1, Name: "ns1", Kind: "group", FullPath: "ns1"},
			{ID: 2, Name: "ns2", Kind: "user", FullPath: "users/ns2"},
		},
	})
	if !strings.Contains(s, "ns1") {
		t.Error("expected ns1")
	}
	if !strings.Contains(s, "ns2") {
		t.Error("expected ns2")
	}
	if !strings.Contains(s, "Namespaces (2)") {
		t.Error("expected count header")
	}
}

// TestFormatListMarkdown_NonNil verifies FormatListMarkdown when non nil.
func TestFormatListMarkdown_NonNil(t *testing.T) {
	r := FormatListMarkdown(ListOutput{})
	if r == nil {
		t.Error(errExpNonNilResult)
	}
}

// TestFormatMarkdownString_AllFields verifies FormatMarkdownString when all fields.
func TestFormatMarkdownString_AllFields(t *testing.T) {
	s := FormatMarkdownString(Output{
		ID: 1, Name: "test", Path: "test", FullPath: "grp/test", Kind: "group",
		ParentID: 5, WebURL: "https://x", Plan: "gold",
	})
	if !strings.Contains(s, "Parent ID") {
		t.Error("expected Parent ID")
	}
	if !strings.Contains(s, "gold") {
		t.Error("expected plan")
	}
	if !strings.Contains(s, "https://x") {
		t.Error("expected web URL")
	}
}

// TestFormatMarkdownString_Minimal verifies FormatMarkdownString when minimal.
func TestFormatMarkdownString_Minimal(t *testing.T) {
	s := FormatMarkdownString(Output{ID: 1, Name: "n", Path: "n", Kind: "user"})
	if strings.Contains(s, "Parent ID") {
		t.Error("should skip Parent ID when 0")
	}
	if strings.Contains(s, "Plan") {
		t.Error("should skip Plan when empty")
	}
}

// TestFormatMarkdown_NonNil verifies FormatMarkdown when non nil.
func TestFormatMarkdown_NonNil(t *testing.T) {
	r := FormatMarkdown(Output{ID: 1, Name: "n"})
	if r == nil {
		t.Error(errExpNonNilResult)
	}
}

// TestFormatExistsMarkdownString_NotExists verifies FormatExistsMarkdownString when not exists.
func TestFormatExistsMarkdownString_NotExists(t *testing.T) {
	s := FormatExistsMarkdownString(ExistsOutput{Exists: false})
	if !strings.Contains(s, "does not exist") {
		t.Errorf("got %q", s)
	}
}

// TestFormatExistsMarkdownString_ExistsWithSuggestions verifies FormatExistsMarkdownString when exists with suggestions.
func TestFormatExistsMarkdownString_ExistsWithSuggestions(t *testing.T) {
	s := FormatExistsMarkdownString(ExistsOutput{Exists: true, Suggests: []string{"alt1", "alt2"}})
	if !strings.Contains(s, "exists") {
		t.Error("expected exists")
	}
	if !strings.Contains(s, "alt1") {
		t.Error("expected suggestions")
	}
}

// TestFormatExistsMarkdown_NonNil verifies FormatExistsMarkdown when non nil.
func TestFormatExistsMarkdown_NonNil(t *testing.T) {
	r := FormatExistsMarkdown(ExistsOutput{})
	if r == nil {
		t.Error(errExpNonNilResult)
	}
}

// TestActionSpecs_Metadata verifies namespace action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "namespaces" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_namespace_get"].ParameterGuidance["id"].SemanticRole == "" {
		t.Fatal("gitlab_namespace_get should define id parameter guidance")
	}
	if specByTool["gitlab_namespace_search"].ParameterGuidance["query"].SemanticRole == "" {
		t.Fatal("gitlab_namespace_search should define query parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// Markdown registry dispatch
// ---------------------------------------------------------------------------.

// TestMarkdownRegistry_ListOutput verifies namespace list output markdown registration.
func TestMarkdownRegistry_ListOutput(t *testing.T) {
	r := toolutil.MarkdownForResult(ListOutput{})
	if r == nil {
		t.Error(errExpNonNil)
	}
}

// TestMarkdownRegistry_Output verifies namespace detail output markdown registration.
func TestMarkdownRegistry_Output(t *testing.T) {
	r := toolutil.MarkdownForResult(Output{ID: 1, Name: "n"})
	if r == nil {
		t.Error(errExpNonNil)
	}
}

// TestMarkdownRegistry_ExistsOutput verifies namespace existence output markdown registration.
func TestMarkdownRegistry_ExistsOutput(t *testing.T) {
	r := toolutil.MarkdownForResult(ExistsOutput{})
	if r == nil {
		t.Error(errExpNonNil)
	}
}

// TestMarkdownRegistry_Unknown verifies unknown output types do not have namespace markdown.
func TestMarkdownRegistry_Unknown(t *testing.T) {
	r := toolutil.MarkdownForResult("unknown")
	if r != nil {
		t.Error("expected nil for unknown type")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates all namespace canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/namespaces" && r.Method == http.MethodGet:
			if r.URL.Query().Get("search") != "" {
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"s","path":"s","kind":"group","full_path":"s"}]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			} else {
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"ns","path":"ns","kind":"user","full_path":"ns"}]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			}
		case r.URL.Path == "/api/v4/namespaces/42":
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"myns","path":"myns","kind":"group","full_path":"myns"}`)
		case strings.HasSuffix(r.URL.Path, "/exists"):
			testutil.RespondJSON(w, http.StatusOK, `{"exists":false,"suggests":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_namespace_list", map[string]any{}},
		{"gitlab_namespace_get", map[string]any{"id": "42"}},
		{"gitlab_namespace_exists", map[string]any{"id": "test-path"}},
		{"gitlab_namespace_search", map[string]any{"query": "test"}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			result, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s): %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// TestGet_ArrayFallback verifies that Get handles the GitLab array response
// fallback when the standard endpoint returns "cannot unmarshal array".
func TestGet_ArrayFallback(t *testing.T) {
	calls := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// First call: return array (triggers unmarshal error in client-go).
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"root","path":"root","kind":"user","full_path":"root","web_url":"https://example.com/root"}]`)
			return
		}
		// Second call via raw Do(): return the same array.
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"root","path":"root","kind":"user","full_path":"root","web_url":"https://example.com/root"}]`)
	}))

	out, err := Get(context.Background(), client, GetInput{ID: "root"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "root" {
		t.Errorf("Name = %q, want %q", out.Name, "root")
	}
}

// TestGet_APIError verifies that Get wraps standard API errors.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Namespace Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// TestGet_ArrayFallback_EmptyArray verifies that Get returns an error when
// the array fallback returns an empty list.
func TestGet_ArrayFallback_EmptyArray(t *testing.T) {
	calls := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1},{"id":2}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := Get(context.Background(), client, GetInput{ID: "missing"})
	if err == nil {
		t.Fatal("expected error for empty fallback array")
	}
}

// TestGet_ArrayFallback_DoError verifies raw fallback request errors are wrapped.
func TestGet_ArrayFallback_DoError(t *testing.T) {
	calls := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			testutil.RespondJSON(w, http.StatusOK, `[{}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ID: "missing"})
	if err == nil {
		t.Fatal("expected error for fallback request")
	}
	if !strings.Contains(err.Error(), "namespace_get") {
		t.Fatalf("error = %v, want namespace_get context", err)
	}
}
