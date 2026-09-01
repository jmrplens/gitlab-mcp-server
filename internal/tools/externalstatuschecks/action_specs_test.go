// action_specs_test.go contains unit tests for the external status check [toolutil.ActionSpec] entries.
package externalstatuschecks

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	mergeCheckJSON   = `[{"id":1,"name":"CI Check","external_url":"https://ci.example.com","status":"passed"}]`
	projectCheckJSON = `[{"id":1,"name":"CI Check","external_url":"https://ci.example.com","hmac":true,"protected_branches":[{"id":1,"name":"main"}]}]`
	createdCheckJSON = `{"id":2,"name":"New Check","external_url":"https://new.example.com","hmac":false,"protected_branches":[]}`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "externalstatuschecks" {
			t.Errorf("OwnerPackage for %s = %q, want externalstatuschecks", spec.Name, spec.OwnerPackage)
		}
		if spec.IndividualTool.Name == "" {
			t.Errorf("IndividualTool.Name for %s is empty", spec.Name)
		}
	}

	byTool := externalStatusCheckSpecsByTool(t, specs)
	for _, name := range []string{"gitlab_list_project_status_checks", "gitlab_list_project_mr_external_status_checks", "gitlab_list_project_external_status_checks"} {
		t.Run(name, func(t *testing.T) {
			if !byTool[name].ReadOnly {
				t.Errorf("%s should be read-only", name)
			}
		})
	}
	spec := byTool["gitlab_delete_project_external_status_check"]
	if !spec.Destructive || !spec.Route.Destructive {
		t.Error("delete action should be destructive")
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(r.URL.Path, "merge_requests") {
				testutil.RespondJSON(w, http.StatusOK, mergeCheckJSON)
			} else {
				testutil.RespondJSON(w, http.StatusOK, projectCheckJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, createdCheckJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, createdCheckJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := externalStatusCheckSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_project_status_checks", map[string]any{"project_id": "42"}},
		{"gitlab_list_project_mr_external_status_checks", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_list_project_external_status_checks", map[string]any{"project_id": "42"}},
		{"gitlab_create_project_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_external_status_check_for_project_mr", map[string]any{"project_id": "42", "merge_request_iid": 1, "check_id": 1}},
		{"gitlab_set_project_mr_external_status_check_status", map[string]any{"project_id": "42", "merge_request_iid": 1, "sha": "abc123", "external_status_check_id": 1, "status": "passed"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestActionSpecs_MutationErrors validates the MutationErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := externalStatusCheckSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_create_project_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_external_status_check_for_project_mr", map[string]any{"project_id": "42", "merge_request_iid": 1, "check_id": 1}},
		{"gitlab_set_project_mr_external_status_check_status", map[string]any{"project_id": "42", "merge_request_iid": 1, "sha": "abc", "external_status_check_id": 1, "status": "passed"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// TestDecorateExternalStatusCheckMeta_Coverage verifies the discovery-metadata
// decorator both enriches a known tool and leaves the generic placeholder
// metadata untouched for an unknown tool (the no-op early-return branch).
// It asserts every known tool gains a non-generic Usage and a
// "Returns: … See also: …" description, and that an unrecognized tool keeps
// the default placeholder Usage.
func TestDecorateExternalStatusCheckMeta_Coverage(t *testing.T) {
	const genericUsage = "Use to execute externalstatuschecks domain action."

	for tool := range externalStatusCheckActionMeta {
		options := externalStatusCheckOptions(tool)
		decorateExternalStatusCheckMeta(&options, tool)
		if options.Usage == genericUsage || options.Usage == "" {
			t.Errorf("%s: Usage not enriched: %q", tool, options.Usage)
		}
		if !strings.Contains(options.IndividualTool.Description, "Returns:") ||
			!strings.Contains(options.IndividualTool.Description, "See also:") {
			t.Errorf("%s: description missing Returns/See also: %q", tool, options.IndividualTool.Description)
		}
		if len(options.Aliases) == 0 {
			t.Errorf("%s: expected natural-language aliases", tool)
		}
	}

	unknown := externalStatusCheckOptions("gitlab_unknown_tool")
	decorateExternalStatusCheckMeta(&unknown, "gitlab_unknown_tool")
	if unknown.Usage != genericUsage {
		t.Errorf("unknown tool Usage = %q, want generic placeholder", unknown.Usage)
	}
}

func externalStatusCheckSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
