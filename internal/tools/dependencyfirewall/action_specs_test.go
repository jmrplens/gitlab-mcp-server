// action_specs_test.go contains canonical action spec and route tests for the
// Dependency Firewall domain: the metadata the catalog projects, the input
// schema enum, and the 404 path that names the feature flag.
package dependencyfirewall

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// newSpec returns the single Dependency Firewall spec built against handler.
func newSpec(t *testing.T, handler http.Handler) toolutil.ActionSpec {
	t.Helper()
	specs := ActionSpecs(testutil.NewTestClient(t, handler))
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	return specs[0]
}

// okHandler answers every request with an allowed verdict.
func okHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
	})
}

// TestActionSpecs_Metadata verifies the catalog metadata that decides where the
// action is projected and who may see it.
//
// The tier is the load-bearing assertion: the API page states "Tier: Premium,
// Ultimate", so premium is the documented minimum, and "Offering: GitLab.com,
// GitLab Self-Managed, GitLab Dedicated" is why GitLabDotComOnly stays false.
func TestActionSpecs_Metadata(t *testing.T) {
	spec := newSpec(t, okHandler(t))

	if spec.Name != "dependency_firewall_evaluate" {
		t.Errorf("Name = %q, want %q", spec.Name, "dependency_firewall_evaluate")
	}
	if spec.OwnerPackage != "dependencyfirewall" {
		t.Errorf("OwnerPackage = %q, want %q", spec.OwnerPackage, "dependencyfirewall")
	}
	if spec.Edition != "premium" {
		t.Errorf("Edition = %q, want %q (docs: Tier: Premium, Ultimate)", spec.Edition, "premium")
	}
	if spec.GitLabDotComOnly {
		t.Error("GitLabDotComOnly = true, want false: the API is offered on Self-Managed and Dedicated too")
	}
	if !spec.ReadOnly {
		t.Error("ReadOnly = false, want true: evaluating a coordinate changes nothing")
	}
	if spec.Destructive {
		t.Error("Destructive = true, want false")
	}
	if !spec.Idempotent {
		t.Error("Idempotent = false, want true")
	}
	if spec.IndividualTool.Name != individualToolName {
		t.Errorf("IndividualTool.Name = %q, want %q", spec.IndividualTool.Name, individualToolName)
	}
	if spec.IndividualTool.Description == "" {
		t.Error("IndividualTool.Description is empty")
	}
	if !slices.Contains(spec.Aliases, individualToolName) {
		t.Errorf("Aliases = %v, want it to contain %q", spec.Aliases, individualToolName)
	}
	if len(spec.RelatedActions) == 0 {
		t.Error("RelatedActions is empty")
	}
	for _, param := range []string{"project_id", "ecosystem", "name", "version"} {
		t.Run("guidance/"+param, func(t *testing.T) {
			if _, ok := spec.ParameterGuidance[param]; !ok {
				t.Errorf("ParameterGuidance is missing %q", param)
			}
		})
	}
}

// TestActionSpecs_CanonicalIDMatchesProjection verifies the exported canonical
// ID agrees with the domain the spec is projected under. The gitlab_project
// group makes the domain "project", and a mismatch would leave every
// cross-reference to this action pointing at nothing.
func TestActionSpecs_CanonicalIDMatchesProjection(t *testing.T) {
	spec := newSpec(t, okHandler(t))
	if want := "project." + spec.Name; ActionEvaluate != want {
		t.Errorf("ActionEvaluate = %q, want %q", ActionEvaluate, want)
	}
}

// TestActionSpecs_EcosystemEnumIsComplete verifies the input schema advertises
// every documented ecosystem, so a model picking a value from the schema can
// never pick one the handler then rejects.
func TestActionSpecs_EcosystemEnumIsComplete(t *testing.T) {
	spec := newSpec(t, okHandler(t))

	props, ok := spec.Route.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("InputSchema has no properties: %#v", spec.Route.InputSchema)
	}
	ecosystem, ok := props["ecosystem"].(map[string]any)
	if !ok {
		t.Fatalf("InputSchema has no ecosystem property: %#v", props)
	}
	enum, ok := ecosystem["enum"].([]any)
	if !ok {
		t.Fatalf("ecosystem has no enum: %#v", ecosystem)
	}
	if len(enum) != len(Ecosystems) {
		t.Fatalf("len(enum) = %d, want %d", len(enum), len(Ecosystems))
	}
	for i, value := range enum {
		if value != Ecosystems[i] {
			t.Errorf("enum[%d] = %v, want %q", i, value, Ecosystems[i])
		}
	}
}

// TestActionSpecs_CallRoute verifies the projected route reaches the handler
// and returns the typed output.
func TestActionSpecs_CallRoute(t *testing.T) {
	spec := newSpec(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"warned","reason":"license policy"}`)
	}))

	result, err := spec.Route.Handler(t.Context(), map[string]any{
		"project_id": "42",
		"ecosystem":  "maven",
		"name":       "com.example:trivial-lib",
		"version":    "1.2.3",
	})
	if err != nil {
		t.Fatalf("Route.Handler() error = %v", err)
	}
	out, ok := result.(EvaluatePackageOutput)
	if !ok {
		t.Fatalf("Route.Handler() = %T, want EvaluatePackageOutput", result)
	}
	if out.Outcome != outcomeWarned {
		t.Errorf("Outcome = %q, want %q", out.Outcome, outcomeWarned)
	}
}

// TestActionSpecs_CallRouteError verifies a non-404 failure still reaches the
// caller as an error rather than being swallowed as guidance.
func TestActionSpecs_CallRouteError(t *testing.T) {
	spec := newSpec(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
	}))

	if _, err := spec.Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "ecosystem": "npm", "name": "lodash", "version": "1",
	}); err == nil {
		t.Fatal("Route.Handler() error = nil, want the server error")
	}
}

// TestActionSpecs_NotFoundNamesTheFeatureFlag verifies that a 404 becomes an
// informational result naming dependency_firewall_phase1.
//
// While the flag is off every project on the instance answers 404, so a caller
// told only "not found" concludes the project reference is wrong and retries
// with another one forever. The flag has to be in the text.
func TestActionSpecs_NotFoundNamesTheFeatureFlag(t *testing.T) {
	spec := newSpec(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	result, err := spec.Route.Handler(t.Context(), map[string]any{
		"project_id": "group/project", "ecosystem": "npm", "name": "lodash", "version": "1",
	})
	if err != nil {
		t.Fatalf("Route.Handler() error = %v, want nil so the guidance is returned instead", err)
	}
	callResult := toolutil.MarkdownForResult(result)
	if callResult == nil || !callResult.IsError {
		t.Fatalf("MarkdownForResult() = %#v, want an informational error result", callResult)
	}
	text := resultText(t, callResult.Content)
	for _, want := range []string{FeatureFlag, "group/project", "Premium", "gitlab_project_get"} {
		t.Run("mentions/"+want, func(t *testing.T) {
			if !strings.Contains(text, want) {
				t.Errorf("not-found guidance is missing %q:\n%s", want, text)
			}
		})
	}
}

// TestProjectIdentifier verifies the not-found message degrades gracefully
// when the caller passed no readable project reference.
func TestProjectIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{name: "string id", input: map[string]any{"project_id": "group/project"}, want: "project group/project"},
		{name: "numeric id", input: map[string]any{"project_id": 42}, want: "project 42"},
		{name: "absent", input: map[string]any{}, want: "the requested project"},
		{name: "nil", input: map[string]any{"project_id": nil}, want: "the requested project"},
		{name: "empty", input: map[string]any{"project_id": ""}, want: "the requested project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectIdentifier(tt.input); got != tt.want {
				t.Errorf("projectIdentifier() = %q, want %q", got, tt.want)
			}
		})
	}
}
