// action_specs_test.go contains integration tests for the dependency tool closures
// in ActionSpecs routes with a mock GitLab API.
package dependencies

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerDepListJSON = `[{"name":"rails","version":"7.0.0","package_manager":"bundler","dependency_file_path":"Gemfile.lock"}]`
	registerExportJSON  = `{"id":1,"has_finished":false,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":""}`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_ExportStatusAndDownload_KeepTheirOwnDiscoveryText holds each
// of the two export actions to the discovery text that describes it.
//
// Both read the same export id, so they share one case in dependencyOptions
// and are told apart by a single comparison inside it. What that comparison
// hands out is exactly what `gitlab_find_action` scores a query against: swap
// the two and a model asking to download an SBOM is routed to the action that
// only reports status, with nothing in the response to say it was misrouted.
// Each case therefore asserts the word that distinguishes its own action and
// the absence of the sibling's, so the test fails on the swap rather than on a
// rewording.
func TestActionSpecs_ExportStatusAndDownload_KeepTheirOwnDiscoveryText(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	specs := ActionSpecs(client)
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}

	tests := []struct {
		name    string
		tool    string
		want    string
		notWant string
	}{
		{
			name:    "the status action never advertises downloading",
			tool:    "gitlab_get_dependency_list_export",
			want:    "status",
			notWant: "download",
		},
		{
			name:    "the download action never advertises status",
			tool:    "gitlab_download_dependency_list_export",
			want:    "download",
			notWant: "status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := byTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			text := strings.ToLower(strings.Join(append([]string{spec.Usage}, spec.Aliases...), " "))
			if !strings.Contains(text, tt.want) {
				t.Errorf("discovery text for %s never mentions %q: %s", tt.tool, tt.want, text)
			}
			if strings.Contains(text, tt.notWant) {
				t.Errorf("discovery text for %s is the sibling action's, it mentions %q: %s", tt.tool, tt.notWant, text)
			}
		})
	}
}

// TestDependencyOptions_UnknownAction_CarriesNoDiscoveryText pins what the
// switch in dependencyOptions does with a name it does not list.
//
// It has no default branch, so an action added to ActionSpecs without a case
// beside it keeps its tags, edition, owner and individual tool name and loses
// every word a model could search it by: no aliases, no usage, no parameter
// guidance. Such an action is reachable only by its exact catalog id. That is
// what makes TestActionSpecs_Metadata's insistence that all four names come
// back furnished worth asserting, and this test states the silence on the
// other side of it.
func TestDependencyOptions_UnknownAction_CarriesNoDiscoveryText(t *testing.T) {
	opts := dependencyOptions("export_purge", "gitlab_purge_dependency_list_export")

	if opts.Usage != "" {
		t.Errorf("Usage = %q, want empty for an action the switch does not list", opts.Usage)
	}
	if len(opts.Aliases) != 0 {
		t.Errorf("Aliases = %v, want none for an action the switch does not list", opts.Aliases)
	}
	if len(opts.ParameterGuidance) != 0 {
		t.Errorf("ParameterGuidance = %v, want none for an action the switch does not list", opts.ParameterGuidance)
	}
	if len(opts.InputSchemaOverrides) != 0 {
		t.Errorf("InputSchemaOverrides = %v, want none for an action the switch does not list", opts.InputSchemaOverrides)
	}
	if opts.OwnerPackage != "dependencies" {
		t.Errorf("OwnerPackage = %q, want %q: the fields set outside the switch are unaffected", opts.OwnerPackage, "dependencies")
	}
	if opts.IndividualTool.Name != "gitlab_purge_dependency_list_export" {
		t.Errorf("IndividualTool.Name = %q, want the name it was given", opts.IndividualTool.Name)
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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
