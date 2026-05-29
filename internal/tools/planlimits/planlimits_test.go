// planlimits_test.go contains unit tests for the plan limit MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package planlimits

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// planLimitJSON identifies the plan limit JSON constant used by this package.
const planLimitJSON = `{
	"conan_max_file_size": 3221225472,
	"generic_packages_max_file_size": 5368709120,
	"helm_max_file_size": 5242880,
	"maven_max_file_size": 3221225472,
	"npm_max_file_size": 524288000,
	"nuget_max_file_size": 524288000,
	"pypi_max_file_size": 3221225472,
	"terraform_module_max_file_size": 1073741824
}`

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ConanMaxFileSize != 3221225472 {
		t.Fatalf("expected conan_max_file_size 3221225472, got %d", out.ConanMaxFileSize)
	}
	if out.GenericPackagesMaxFileSize != 5368709120 {
		t.Fatalf("expected generic_packages_max_file_size 5368709120, got %d", out.GenericPackagesMaxFileSize)
	}
}

// TestGet_WithPlanName verifies Get when with plan name.
func TestGet_WithPlanName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodGet {
			if r.URL.Query().Get("plan_name") != "default" {
				t.Errorf("expected plan_name=default, got %s", r.URL.Query().Get("plan_name"))
			}
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{PlanName: "default"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.NPMMaxFileSize != 524288000 {
		t.Fatalf("expected npm_max_file_size 524288000, got %d", out.NPMMaxFileSize)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestChange_Success verifies Change when success.
func TestChange_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	size := int64(1073741824)
	out, err := Change(t.Context(), client, ChangeInput{
		PlanName:        "default",
		HelmMaxFileSize: &size,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.HelmMaxFileSize != 5242880 {
		t.Fatalf("expected helm_max_file_size 5242880, got %d", out.HelmMaxFileSize)
	}
}

// TestChange_Error verifies Change when error.
func TestChange_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Change(t.Context(), client, ChangeInput{PlanName: "default"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		PlanLimitItem: PlanLimitItem{
			ConanMaxFileSize:           3221225472,
			GenericPackagesMaxFileSize: 5368709120,
			HelmMaxFileSize:            5242880,
			MavenMaxFileSize:           3221225472,
			NPMMaxFileSize:             524288000,
			NugetMaxFileSize:           524288000,
			PyPiMaxFileSize:            3221225472,
			TerraformModuleMaxFileSize: 1073741824,
		},
	}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "Plan Limits") {
		t.Fatal("expected 'Plan Limits' in markdown")
	}
	if !strings.Contains(md, "3221225472") {
		t.Fatal("expected '3221225472' in markdown")
	}
}

// TestFormatChangeMarkdown verifies FormatChangeMarkdown.
func TestFormatChangeMarkdown(t *testing.T) {
	out := ChangeOutput{
		PlanLimitItem: PlanLimitItem{
			ConanMaxFileSize: 3221225472,
			HelmMaxFileSize:  5242880,
		},
	}
	md := FormatChangeMarkdown(out)
	if !strings.Contains(md, "Updated Plan Limits") {
		t.Fatal("expected 'Updated Plan Limits' in markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatGetMarkdown — all fields
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies FormatGetMarkdown when all fields.
func TestFormatGetMarkdown_AllFields(t *testing.T) {
	out := GetOutput{
		PlanLimitItem: PlanLimitItem{
			ConanMaxFileSize:           100,
			GenericPackagesMaxFileSize: 200,
			HelmMaxFileSize:            300,
			MavenMaxFileSize:           400,
			NPMMaxFileSize:             500,
			NugetMaxFileSize:           600,
			PyPiMaxFileSize:            700,
			TerraformModuleMaxFileSize: 800,
		},
	}
	md := FormatGetMarkdown(out)
	for _, want := range []string{"100", "200", "300", "400", "500", "600", "700", "800", "Plan Limits"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatChangeMarkdown — all fields
// ---------------------------------------------------------------------------.

// TestFormatChangeMarkdown_AllFields verifies FormatChangeMarkdown when all fields.
func TestFormatChangeMarkdown_AllFields(t *testing.T) {
	out := ChangeOutput{
		PlanLimitItem: PlanLimitItem{
			ConanMaxFileSize:           1,
			GenericPackagesMaxFileSize: 2,
			HelmMaxFileSize:            3,
			MavenMaxFileSize:           4,
			NPMMaxFileSize:             5,
			NugetMaxFileSize:           6,
			PyPiMaxFileSize:            7,
			TerraformModuleMaxFileSize: 8,
		},
	}
	md := FormatChangeMarkdown(out)
	if !strings.Contains(md, "Updated Plan Limits") {
		t.Error("missing title")
	}
	for _, want := range []string{"Conan", "Generic", "Helm", "Maven", "NPM", "NuGet", "PyPI", "Terraform"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Change — all optional fields
// ---------------------------------------------------------------------------.

// TestChange_AllOptionalFields verifies Change when all optional fields.
func TestChange_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/application/plan_limits" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
			return
		}
		http.NotFound(w, r)
	}))

	v1, v2, v3, v4, v5, v6, v7 := int64(100), int64(200), int64(300), int64(400), int64(500), int64(600), int64(700)
	out, err := Change(t.Context(), client, ChangeInput{
		PlanName:                   "default",
		ConanMaxFileSize:           &v1,
		GenericPackagesMaxFileSize: &v2,
		HelmMaxFileSize:            &v3,
		MavenMaxFileSize:           &v4,
		NPMMaxFileSize:             &v5,
		NugetMaxFileSize:           &v6,
		PyPiMaxFileSize:            &v7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ConanMaxFileSize == 0 {
		t.Error("expected non-zero conan field from response")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies plan limit action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	specs := ActionSpecs(client)
	specByTool := planLimitSpecsByTool(specs)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "planlimits" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if specByTool["gitlab_get_plan_limits"].ParameterGuidance["plan_name"].SemanticRole == "" {
		t.Fatal("gitlab_get_plan_limits should define plan_name parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates plan limit canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newPlanLimitsRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_get_plan_limits", map[string]any{}},
		{"get_with_plan", "gitlab_get_plan_limits", map[string]any{"plan_name": "default"}},
		{"change", "gitlab_change_plan_limits", map[string]any{"plan_name": "default", "helm_max_file_size": float64(5242880)}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// newPlanLimitsRouteSpecs constructs plan limits route specs test fixtures.
func newPlanLimitsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/plan_limits", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/plan_limits", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	})

	client := testutil.NewTestClient(t, handler)
	return planLimitSpecsByTool(ActionSpecs(client))
}

// planLimitSpecsByTool supports plan limit specs by tool assertions in planlimits tests.
func planLimitSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
