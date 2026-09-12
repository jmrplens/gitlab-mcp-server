// plan_limits_test.go contains unit tests for the plan limit MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package planlimits

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestFormatGetMarkdown_EveryLimit_RendersTheWholeCard verifies that the plan
// limits render as one card whose every row carries the unit GitLab counts the
// limit in, with the binary-prefix size in front of the exact byte count.
func TestFormatGetMarkdown_EveryLimit_RendersTheWholeCard(t *testing.T) {
	out := GetOutput{
		ConanMaxFileSize:           3221225472,
		GenericPackagesMaxFileSize: 5368709120,
		HelmMaxFileSize:            5242880,
		MavenMaxFileSize:           3221225472,
		NPMMaxFileSize:             524288000,
		NugetMaxFileSize:           524288000,
		PyPiMaxFileSize:            3221225472,
		TerraformModuleMaxFileSize: 1073741824,
	}

	want := "## Plan Limits\n\n" +
		"- **Conan Max File Size**: 3 GiB (3221225472 bytes)\n" +
		"- **Generic Packages Max File Size**: 5 GiB (5368709120 bytes)\n" +
		"- **Helm Max File Size**: 5 MiB (5242880 bytes)\n" +
		"- **Maven Max File Size**: 3 GiB (3221225472 bytes)\n" +
		"- **NPM Max File Size**: 500 MiB (524288000 bytes)\n" +
		"- **NuGet Max File Size**: 500 MiB (524288000 bytes)\n" +
		"- **PyPI Max File Size**: 3 GiB (3221225472 bytes)\n" +
		"- **Terraform Module Max File Size**: 1 GiB (1073741824 bytes)\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.plan_limits_change' to raise or lower one of these limits\n"

	if got := FormatGetMarkdown(out); got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatChangeMarkdown_SmallAndZeroLimits_RendersTheWholeCard verifies that
// the card a change returns is the card a read returns, that a limit below a
// kibibyte renders as a plain byte count, and that a zero limit is written
// rather than dropped: zero is an answer GitLab gave.
func TestFormatChangeMarkdown_SmallAndZeroLimits_RendersTheWholeCard(t *testing.T) {
	out := ChangeOutput{
		ConanMaxFileSize: 1023,
		HelmMaxFileSize:  1024,
	}

	want := "## Updated Plan Limits\n\n" +
		"- **Conan Max File Size**: 1023 bytes\n" +
		"- **Generic Packages Max File Size**: 0 bytes\n" +
		"- **Helm Max File Size**: 1 KiB (1024 bytes)\n" +
		"- **Maven Max File Size**: 0 bytes\n" +
		"- **NPM Max File Size**: 0 bytes\n" +
		"- **NuGet Max File Size**: 0 bytes\n" +
		"- **PyPI Max File Size**: 0 bytes\n" +
		"- **Terraform Module Max File Size**: 0 bytes\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.plan_limits_get' to read the plan's limits back\n"

	if got := FormatChangeMarkdown(out); got != want {
		t.Errorf("FormatChangeMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFileSize_EveryMagnitude_CarriesTheUnit verifies that a byte count renders
// with the prefix a reader judges it by and the exact count the change action
// takes back, at every magnitude the helper distinguishes.
func TestFileSize_EveryMagnitude_CarriesTheUnit(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 bytes"},
		{name: "below a kibibyte", bytes: 1023, want: "1023 bytes"},
		{name: "exactly a kibibyte", bytes: 1024, want: "1 KiB (1024 bytes)"},
		{name: "fractional kibibytes", bytes: 1536, want: "1.5 KiB (1536 bytes)"},
		{name: "mebibytes", bytes: 5242880, want: "5 MiB (5242880 bytes)"},
		{name: "gibibytes", bytes: 3221225472, want: "3 GiB (3221225472 bytes)"},
		{name: "tebibytes", bytes: 1099511627776, want: "1 TiB (1099511627776 bytes)"},
		{name: "pebibytes", bytes: 1125899906842624, want: "1 PiB (1125899906842624 bytes)"},
		{name: "exbibytes", bytes: 1152921504606846976, want: "1 EiB (1152921504606846976 bytes)"},
		{name: "the largest prefix the table has", bytes: 1 << 62, want: "4 EiB (4611686018427387904 bytes)"},
		{name: "negative", bytes: -1, want: "-1 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileSize(tt.bytes); got != tt.want {
				t.Errorf("fileSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

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
