// snippets_audit_test.go contains unit tests for the 1:1 audit additions to the
// snippet handlers: full nested author/repository_storage output, keyset and
// order_by/sort list inputs, and the per-tool discovery metadata decorator.
package snippets

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// snippetWithStorageJSON is a snippet payload that exercises the full author
// object (including created_at) and the repository_storage field added by the
// 1:1 audit.
const snippetWithStorageJSON = `{"id":7,"title":"Audit","file_name":"a.go","description":"","visibility":"public",` +
	`"author":{"id":3,"username":"dev","email":"dev@example.com","name":"Dev","state":"active","created_at":"2024-01-02T03:04:05Z"},` +
	`"project_id":0,"web_url":"https://gitlab.example.com/snippets/7","raw_url":"https://gitlab.example.com/snippets/7/raw",` +
	`"repository_storage":"default","files":[{"path":"a.go","raw_url":"https://gitlab.example.com/snippets/7/raw/main/a.go"}]}`

// TestConvertSnippet_FullAuthorAndStorage verifies that convertSnippet surfaces
// the full nested author object (with created_at) and repository_storage, per
// the 1:1 audit policy.
func TestConvertSnippet_FullAuthorAndStorage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/7", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, snippetWithStorageJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Get(context.Background(), client, GetInput{SnippetID: 7})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.RepositoryStorage != "default" {
		t.Errorf("RepositoryStorage = %q, want default", out.RepositoryStorage)
	}
	if out.Author == nil {
		t.Fatal("Author is nil, want full author object")
	}
	if out.Author.Email != "dev@example.com" || out.Author.State != "active" {
		t.Errorf("Author = %+v, want email/state populated", out.Author)
	}
	if out.Author.CreatedAt == nil || !out.Author.CreatedAt.Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("Author.CreatedAt = %v, want 2024-01-02T03:04:05Z", out.Author.CreatedAt)
	}
}

// TestList_KeysetAndOrdering verifies List forwards order_by, sort, pagination,
// and page_token onto the GitLab query, covering the keyset wiring.
func TestList_KeysetAndOrdering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := List(context.Background(), client, ListInput{
		OrderBy:               "created_at",
		Sort:                  "desc",
		PaginationInput:       toolutil.PaginationInput{PerPage: 50},
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok"},
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	for key, want := range map[string]string{
		"order_by": "created_at", "sort": "desc", "pagination": "keyset", "page_token": "tok", "per_page": "50",
	} {
		if got.Get(key) != want {
			t.Errorf("query %s = %q, want %q", key, got.Get(key), want)
		}
	}
}

// TestExplore_Ordering verifies Explore forwards order_by and sort.
func TestExplore_Ordering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/public", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	if _, err := Explore(context.Background(), client, ExploreInput{OrderBy: "updated_at", Sort: "asc"}); err != nil {
		t.Fatalf("Explore returned error: %v", err)
	}
	if got.Get("order_by") != "updated_at" || got.Get("sort") != "asc" {
		t.Errorf("explore order_by/sort = %q/%q, want updated_at/asc", got.Get("order_by"), got.Get("sort"))
	}
}

// TestListAll_RepositoryStorageAndOrdering verifies ListAll forwards the
// repository_storage filter alongside order_by and sort.
func TestListAll_RepositoryStorageAndOrdering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/snippets/all", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ListAll(context.Background(), client, ListAllInput{
		RepositoryStorage: "nfs-01",
		OrderBy:           "id",
		Sort:              "asc",
	})
	if err != nil {
		t.Fatalf("ListAll returned error: %v", err)
	}
	if got.Get("repository_storage") != "nfs-01" {
		t.Errorf("repository_storage = %q, want nfs-01", got.Get("repository_storage"))
	}
	if got.Get("order_by") != "id" || got.Get("sort") != "asc" {
		t.Errorf("order_by/sort = %q/%q, want id/asc", got.Get("order_by"), got.Get("sort"))
	}
}

// TestProjectList_Ordering verifies ProjectList forwards order_by and sort.
func TestProjectList_Ordering(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/p/snippets", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		testutil.RespondJSONWithPagination(w, http.StatusOK, snippetListJSON,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := ProjectList(context.Background(), client, ProjectListInput{
		ProjectID: "p", OrderBy: "created_at", Sort: "desc",
	})
	if err != nil {
		t.Fatalf("ProjectList returned error: %v", err)
	}
	if got.Get("order_by") != "created_at" || got.Get("sort") != "desc" {
		t.Errorf("order_by/sort = %q/%q, want created_at/desc", got.Get("order_by"), got.Get("sort"))
	}
}

// TestApplyOrderSort_NilOpts verifies applyOrderSort is a safe no-op when the
// options pointer is nil.
func TestApplyOrderSort_NilOpts(t *testing.T) {
	applyOrderSort(nil, "created_at", "desc") // must not panic
}

// TestAuthorUsername_Nil verifies authorUsername returns an empty string for a
// nil author object (minimal payloads).
func TestAuthorUsername_Nil(t *testing.T) {
	if got := authorUsername(nil); got != "" {
		t.Errorf("authorUsername(nil) = %q, want empty string", got)
	}
}

// TestDecorateSnippetMeta_UnknownToolIsNoop verifies decorateSnippetMeta leaves
// options untouched for a tool with no metadata entry.
func TestDecorateSnippetMeta_UnknownToolIsNoop(t *testing.T) {
	options := snippetOptions("gitlab_snippet_unknown")
	before := options.Usage
	decorateSnippetMeta(&options, "gitlab_snippet_unknown")
	if options.Usage != before {
		t.Errorf("Usage mutated for unknown tool: %q", options.Usage)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions set for unknown tool: %v", options.RelatedActions)
	}
}

// TestActionSpecs_MetadataPopulated verifies every snippet action spec carries
// non-generic discovery metadata (R-META): a custom usage, natural-language
// aliases, related actions, and a "Returns: … See also: …" description.
func TestActionSpecs_MetadataPopulated(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	for _, spec := range ActionSpecs(client) {
		tool := spec.IndividualTool.Name
		t.Run(tool, func(t *testing.T) {
			meta, ok := snippetActionMeta[tool]
			if !ok {
				t.Fatalf("no metadata entry for %s", tool)
			}
			if meta.usage == "" || spec.Usage == "Use to execute snippets domain action." {
				t.Errorf("%s has generic usage: %q", tool, spec.Usage)
			}
			if len(spec.Aliases) == 0 || spec.Aliases[0] == tool {
				t.Errorf("%s missing natural-language aliases: %v", tool, spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s missing related actions", tool)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description missing Returns:/See also: form: %q", tool, desc)
			}
		})
	}
}

// TestActionSpecs_Count guards that the snippet catalog still projects all 15
// canonical snippet actions after the audit.
func TestActionSpecs_Count(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if n := len(ActionSpecs(client)); n != 15 {
		t.Fatalf("ActionSpecs count = %d, want 15", n)
	}
}
