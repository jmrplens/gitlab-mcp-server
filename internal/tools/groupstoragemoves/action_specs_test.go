// action_specs_test.go contains integration tests for the group storage move tool
// closures in ActionSpecs routes with a mock GitLab API.
package groupstoragemoves

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerStorageMoveJSON = `[{"id":1,"state":"finished","group":{"id":42,"web_url":"https://gitlab.example.com/groups/test","full_path":"test"},"source_storage_name":"default","destination_storage_name":"storage2"}]`
	registerSingleMoveJSON  = `{"id":1,"state":"finished","group":{"id":42,"web_url":"https://gitlab.example.com/groups/test","full_path":"test"},"source_storage_name":"default","destination_storage_name":"storage2"}`
)

// TestActionSpecs_Metadata verifies group storage move action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupstoragemoves" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all 6 group storage move routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(path, "repository_storage_moves") && !strings.HasSuffix(path, "/1") {
				testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerSingleMoveJSON)
			}
		case http.MethodPost:
			if strings.Contains(path, "all") {
				testutil.RespondJSON(w, http.StatusAccepted, `{"message":"202 Accepted"}`)
			} else {
				testutil.RespondJSON(w, http.StatusCreated, registerSingleMoveJSON)
			}
		default:
			http.NotFound(w, r)
		}
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
		{"gitlab_retrieve_all_group_storage_moves", map[string]any{}},
		{"gitlab_retrieve_group_storage_moves", map[string]any{"group_id": 42}},
		{"gitlab_get_group_storage_move", map[string]any{"id": 1}},
		{"gitlab_get_group_storage_move_for_group", map[string]any{"group_id": 42, "id": 1}},
		{"gitlab_schedule_group_storage_move", map[string]any{"group_id": 42, "destination_storage_name": "storage2"}},
		{"gitlab_schedule_all_group_storage_moves", map[string]any{"source_storage_name": "default", "destination_storage_name": "storage2"}},
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
