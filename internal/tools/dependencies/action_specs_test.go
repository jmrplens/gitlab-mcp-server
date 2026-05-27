// action_specs_test.go contains integration tests for the dependency tool closures
// in ActionSpecs routes with a mock GitLab API.
package dependencies

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	registerDepListJSON = `[{"name":"rails","version":"7.0.0","package_manager":"bundler","dependency_file_path":"Gemfile.lock"}]`
	registerExportJSON  = `{"id":1,"has_finished":false,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":""}`
)

// TestActionSpecs_Metadata verifies dependency action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "dependencies" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	if byTool["gitlab_list_project_dependencies"].ParameterGuidance["project_id"].SemanticRole == "" {
		t.Fatal("gitlab_list_project_dependencies should define project_id parameter guidance")
	}
	if byTool["gitlab_download_dependency_list_export"].ParameterGuidance["export_id"].SemanticRole == "" {
		t.Fatal("gitlab_download_dependency_list_export should define export_id parameter guidance")
	}
}

// TestActionSpecs_CallRoutes verifies all 4 dependency canonical routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/dependencies") && r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerDepListJSON)
		case strings.Contains(path, "/dependency_list_exports") && strings.HasSuffix(path, "/download"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"bomFormat":"CycloneDX"}`))
		case strings.Contains(path, "/dependency_list_exports") && r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerExportJSON)
		case strings.Contains(path, "/dependency_list_exports") && r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"has_finished":true,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":"https://gitlab.example.com/api/v4/dependency_list_exports/1/download"}`)
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
		{"gitlab_list_project_dependencies", map[string]any{"project_id": "42"}},
		{"gitlab_create_dependency_list_export", map[string]any{"pipeline_id": 100}},
		{"gitlab_get_dependency_list_export", map[string]any{"export_id": 1}},
		{"gitlab_download_dependency_list_export", map[string]any{"export_id": 1}},
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
