// action_specs_test.go contains integration tests for the project storage move
// tool closures in ActionSpecs routes with a mock GitLab API.
package projectstoragemoves

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerStorageMoveJSON = `{
	"id": 1,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"project": {"id": 42, "name": "my-project", "path_with_namespace": "group/my-project"}
}`

// TestActionSpecs_Metadata verifies project storage move action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "projectstoragemoves" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all registered project storage move routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	// List all storage moves
	mux.HandleFunc("GET /api/v4/project_repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerStorageMoveJSON+`]`)
	})
	// List moves for a project
	mux.HandleFunc("GET /api/v4/projects/{pid}/repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerStorageMoveJSON+`]`)
	})
	// Get single move by global ID
	mux.HandleFunc("GET /api/v4/project_repository_storage_moves/{id}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
	})
	// Get single move for a project
	mux.HandleFunc("GET /api/v4/projects/{pid}/repository_storage_moves/{id}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
	})
	// Schedule move for a project
	mux.HandleFunc("POST /api/v4/projects/{pid}/repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, registerStorageMoveJSON)
	})
	// Schedule all moves
	mux.HandleFunc("POST /api/v4/project_repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
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
		{"gitlab_retrieve_all_project_storage_moves", map[string]any{}},
		{"gitlab_retrieve_project_storage_moves", map[string]any{"project_id": 42}},
		{"gitlab_get_project_storage_move", map[string]any{"id": 1}},
		{"gitlab_get_project_storage_move_for_project", map[string]any{"project_id": 42, "id": 1}},
		{"gitlab_schedule_project_storage_move", map[string]any{"project_id": 42, "destination_storage_name": "storage2"}},
		{"gitlab_schedule_all_project_storage_moves", map[string]any{"source_storage_name": "default"}},
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
