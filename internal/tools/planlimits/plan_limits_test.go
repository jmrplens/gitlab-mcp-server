// plan_limits_test.go contains unit tests for the plan limit MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package planlimits

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// planLimitJSON identifies the plan limit JSON constant used by this package.
//
// No two limits share a value on purpose. GitLab really does serve the same
// number for several package formats, and a fixture that copies that makes a
// converter reading maven out of pypi indistinguishable from a correct one.
const planLimitJSON = `{
	"conan_max_file_size": 3221225472,
	"generic_packages_max_file_size": 5368709120,
	"helm_max_file_size": 5242880,
	"maven_max_file_size": 2147483648,
	"npm_max_file_size": 524288000,
	"nuget_max_file_size": 419430400,
	"pypi_max_file_size": 1610612736,
	"terraform_module_max_file_size": 1073741824
}`

// wantPlanLimits is planLimitJSON read as the output it has to produce: the
// answer to "which GitLab key did this field come from", one row per field.
var wantPlanLimits = PlanLimitItem{
	ConanMaxFileSize:           3221225472,
	GenericPackagesMaxFileSize: 5368709120,
	HelmMaxFileSize:            5242880,
	MavenMaxFileSize:           2147483648,
	NPMMaxFileSize:             524288000,
	NugetMaxFileSize:           419430400,
	PyPiMaxFileSize:            1610612736,
	TerraformModuleMaxFileSize: 1073741824,
}

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

// TestGet_EveryLimit_CarriesTheValueOfItsOwnKey verifies that each limit in
// the output is the one GitLab sent under the matching key.
//
// The converter is eight straight-line assignments and no branch, so a field
// read out of a neighbour's key answers plausibly and wrongly; only a fixture
// where no two limits agree can tell the two apart.
func TestGet_EveryLimit_CarriesTheValueOfItsOwnKey(t *testing.T) {
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
	if out.PlanLimitItem != wantPlanLimits {
		t.Errorf("Get() limits = %+v, want %+v", out.PlanLimitItem, wantPlanLimits)
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

// TestChange_EverySetLimit_ReachesGitLabUnderItsOwnKey verifies that the plan
// name and every limit the caller set are sent under their own keys, and that
// what GitLab answers comes back as the action's output.
//
// Nothing else guards the request: the options are eight straight-line
// assignments, so a limit routed to a neighbour's field, or left out, changes
// what the instance is told without changing what the caller is shown.
func TestChange_EverySetLimit_ReachesGitLabUnderItsOwnKey(t *testing.T) {
	limits := map[string]int64{
		"conan_max_file_size":            11,
		"generic_packages_max_file_size": 22,
		"helm_max_file_size":             33,
		"maven_max_file_size":            44,
		"npm_max_file_size":              55,
		"nuget_max_file_size":            66,
		"pypi_max_file_size":             77,
		"terraform_module_max_file_size": 88,
	}
	wantBody := map[string]any{"plan_name": "silver"}
	for key, value := range limits {
		wantBody[key] = float64(value)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/plan_limits" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the request body: %v", err)
		} else if !reflect.DeepEqual(body, wantBody) {
			t.Errorf("PUT body = %v, want %v", body, wantBody)
		}
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	}))

	out, err := Change(t.Context(), client, ChangeInput{
		PlanName:                   "silver",
		ConanMaxFileSize:           new(limits["conan_max_file_size"]),
		GenericPackagesMaxFileSize: new(limits["generic_packages_max_file_size"]),
		HelmMaxFileSize:            new(limits["helm_max_file_size"]),
		MavenMaxFileSize:           new(limits["maven_max_file_size"]),
		NPMMaxFileSize:             new(limits["npm_max_file_size"]),
		NugetMaxFileSize:           new(limits["nuget_max_file_size"]),
		PyPiMaxFileSize:            new(limits["pypi_max_file_size"]),
		TerraformModuleMaxFileSize: new(limits["terraform_module_max_file_size"]),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PlanLimitItem != wantPlanLimits {
		t.Errorf("Change() limits = %+v, want %+v", out.PlanLimitItem, wantPlanLimits)
	}
}

// TestChange_LimitsLeftUnset_AreOmittedFromTheRequest verifies that a limit
// the caller did not name is absent from the PUT body rather than sent as
// zero.
//
// Zero is not "leave this alone" to GitLab but "allow nothing", so sending one
// for every unnamed format would forbid every upload of it while the caller
// asked to change one number.
func TestChange_LimitsLeftUnset_AreOmittedFromTheRequest(t *testing.T) {
	wantBody := map[string]any{"plan_name": "default", "helm_max_file_size": float64(5242880)}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/application/plan_limits" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the request body: %v", err)
		} else if !reflect.DeepEqual(body, wantBody) {
			t.Errorf("PUT body = %v, want %v", body, wantBody)
		}
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	}))

	size := int64(5242880)
	if _, err := Change(t.Context(), client, ChangeInput{PlanName: "default", HelmMaxFileSize: &size}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
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

// TestPlanLimits_EachHandlerReachesItsOwnEndpoint drives both handlers against
// the two endpoints GitLab serves the plan limits on, so a handler sending the
// wrong method or path fails rather than being answered by a catch-all.
func TestPlanLimits_EachHandlerReachesItsOwnEndpoint(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/application/plan_limits", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	})
	handler.HandleFunc("PUT /api/v4/application/plan_limits", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, planLimitJSON)
	})
	client := testutil.NewTestClient(t, handler)

	t.Run("get", func(t *testing.T) {
		if _, err := Get(t.Context(), client, GetInput{PlanName: "default"}); err != nil {
			t.Fatalf("Get() error = %v, want nil", err)
		}
	})
	t.Run("change", func(t *testing.T) {
		size := int64(5242880)
		if _, err := Change(t.Context(), client, ChangeInput{PlanName: "default", HelmMaxFileSize: &size}); err != nil {
			t.Fatalf("Change() error = %v, want nil", err)
		}
	})
}
