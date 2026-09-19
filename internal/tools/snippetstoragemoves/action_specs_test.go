// action_specs_test.go contains integration tests for the snippet storage move
// tool closures in ActionSpecs routes with a mock GitLab API.
package snippetstoragemoves

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const registerStorageMoveJSON = `{
	"id": 1,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"snippet": {"id": 99, "title": "test-snippet"}
}`

// TestActionSpecs_Metadata verifies snippet storage move action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	const genericUsage = "Use to execute snippetstoragemoves domain action."
	for _, spec := range specs {
		tool := spec.IndividualTool.Name
		if spec.OwnerPackage != "snippetstoragemoves" || tool == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" || spec.Usage == genericUsage {
			t.Fatalf("%s: Usage must be non-generic, got %q", tool, spec.Usage)
		}
		if len(spec.RelatedActions) == 0 {
			t.Fatalf("%s: RelatedActions must be non-empty", tool)
		}
		if !strings.Contains(spec.IndividualTool.Description, "Returns:") ||
			!strings.Contains(spec.IndividualTool.Description, "See also:") {
			t.Fatalf("%s: Description must use Returns/See also form, got %q", tool, spec.IndividualTool.Description)
		}
		// Aliases must carry distinctive natural-language phrasing beyond the
		// tool name (at least two extra entries) and include the tool name.
		if len(spec.Aliases) < 3 {
			t.Fatalf("%s: want >=2 distinctive aliases plus tool name, got %v", tool, spec.Aliases)
		}
		var hasTool bool
		for _, a := range spec.Aliases {
			if a == tool {
				hasTool = true
			}
		}
		if !hasTool {
			t.Fatalf("%s: Aliases must include the tool name, got %v", tool, spec.Aliases)
		}
	}
}

// TestActionSpecs_CrossLinks_NameActionsThisPackageDefines verifies that every
// canonical ID constant, every RelatedActions entry, and both IDs the markdown
// hints name, are the catalog ID of one of this package's own six actions.
//
// Nothing else checks these strings: audit_discovery_completeness only counts
// an empty related list, so a misspelled entry, or the individual tool name in
// place of the action ID, passes every gate and answers a model "unknown
// action" the moment it follows the hint. The domain is the one
// buildStorageMoveActionSpecs registers this package under, so a rename of
// either half is caught here rather than at a caller.
func TestActionSpecs_CrossLinks_NameActionsThisPackageDefines(t *testing.T) {
	const catalogDomain = "storage_move."

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	ids := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		ids[catalogDomain+spec.Name] = struct{}{}
	}

	declared := []string{
		actionRetrieveAllSnippet, actionRetrieveSnippet, actionGetSnippet,
		actionGetSnippetForSnippet, actionScheduleSnippet, actionScheduleAllSnippet,
	}
	for _, id := range declared {
		t.Run("constant/"+id, func(t *testing.T) {
			if _, ok := ids[id]; !ok {
				t.Errorf("constant %q names no action this package defines", id)
			}
		})
	}
	if len(ids) != len(declared) {
		t.Errorf("the package defines %d actions but declares %d canonical IDs", len(ids), len(declared))
	}

	for _, spec := range specs {
		t.Run("related/"+spec.Name, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				if _, ok := ids[related]; !ok {
					t.Errorf("%s: related action %q is not a canonical ID of this package", spec.IndividualTool.Name, related)
				}
			}
		})
	}
}

// TestActionSpecs_CallRoutes verifies all registered snippet storage move routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/snippet_repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerStorageMoveJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/snippets/{sid}/repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerStorageMoveJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/snippet_repository_storage_moves/{id}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
	})
	mux.HandleFunc("GET /api/v4/snippets/{sid}/repository_storage_moves/{id}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
	})
	mux.HandleFunc("POST /api/v4/snippets/{sid}/repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, registerStorageMoveJSON)
	})
	mux.HandleFunc("POST /api/v4/snippet_repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"message":"202 Accepted"}`)
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_retrieve_all_snippet_storage_moves", map[string]any{}},
		{"gitlab_retrieve_snippet_storage_moves", map[string]any{"snippet_id": 99}},
		{"gitlab_get_snippet_storage_move", map[string]any{"id": 1}},
		{"gitlab_get_snippet_storage_move_for_snippet", map[string]any{"snippet_id": 99, "id": 1}},
		{"gitlab_schedule_snippet_storage_move", map[string]any{"snippet_id": 99, "destination_storage_name": "storage2"}},
		{"gitlab_schedule_all_snippet_storage_moves", map[string]any{"source_storage_name": "default"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}
