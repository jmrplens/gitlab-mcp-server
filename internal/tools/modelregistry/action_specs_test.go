// action_specs_test.go contains integration tests for the model registry tool
// closures in ActionSpecs routes with a mock GitLab API.
package modelregistry

import (
	"encoding/base64"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_Metadata verifies model registry action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "modelregistry" || !specs[0].ReadOnly || !specs[0].Idempotent {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatalf("Usage for %s is empty", specs[0].Name)
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatalf("Aliases for %s are empty", specs[0].Name)
	}
	if !hasNaturalLanguageAlias(specs[0]) {
		t.Fatalf("Aliases for %s carry only the tool name; want distinctive natural-language aliases: %v", specs[0].Name, specs[0].Aliases)
	}
	if len(specs[0].RelatedActions) == 0 {
		t.Fatalf("RelatedActions for %s are empty", specs[0].Name)
	}
	desc := specs[0].IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Fatalf("IndividualTool.Description for %s missing Returns:/See also: form: %q", specs[0].Name, desc)
	}
}

// hasNaturalLanguageAlias reports whether the spec carries at least one alias
// that is neither the canonical action name nor the individual tool name,
// mirroring the R-META audit's aliases_only_toolname check.
func hasNaturalLanguageAlias(spec toolutil.ActionSpec) bool {
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	tool := strings.ToLower(strings.TrimSpace(spec.IndividualTool.Name))
	for _, alias := range spec.Aliases {
		normalized := strings.ToLower(strings.TrimSpace(alias))
		if normalized == "" || normalized == canonical || normalized == tool {
			continue
		}
		return true
	}
	return false
}

// TestActionSpecs_CallRoute verifies the model registry download route carries
// the caller's four arguments to the endpoint and hands back the typed output.
//
// The route is what a model reaches through every surface, and it decodes the
// argument map itself, so a name that does not match the input struct's json
// tag arrives empty here and nowhere else. The path is asserted rather than
// pattern-matched, and the result is type-asserted rather than checked for
// non-nil: a route that reached some endpoint and returned some value was all
// this test used to demand.
func TestActionSpecs_CallRoute(t *testing.T) {
	const wantPath = "/api/v4/projects/42/packages/ml_models/7/files/models/model.bin"
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, wantPath)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("model-binary-data"))
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)

	result, err := specs[0].Route.Handler(t.Context(), map[string]any{
		"project_id":       "42",
		"model_version_id": "7",
		"path":             "models",
		"filename":         "model.bin",
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(DownloadOutput)
	if !ok {
		t.Fatalf("Route.Handler returned %T, want DownloadOutput", result)
	}
	want := DownloadOutput{
		ProjectID:      "42",
		ModelVersionID: "7",
		Path:           "models",
		Filename:       "model.bin",
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("model-binary-data")),
		SizeBytes:      len("model-binary-data"),
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Route.Handler output = %+v, want %+v", out, want)
	}
}
