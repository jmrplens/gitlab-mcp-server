package toolutil

import (
	"strings"
	"testing"
)

func TestNewActionSpec_DeepClonesMetadata(t *testing.T) {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
	}
	routeGuidance := ParameterGuidance{SemanticRole: "scope_project", CommonConfusions: []string{"route confusion"}}
	specGuidance := ParameterGuidance{ValueSource: "prompt", CommonConfusions: []string{"spec confusion"}}
	route := ActionRoute{
		Destructive:       true,
		InputSchema:       inputSchema,
		ParameterGuidance: map[string]ParameterGuidance{"project_id": routeGuidance},
	}
	aliases := []string{" Project.Delete ", "project.delete"}
	tags := []string{" Admin ", "ADMIN"}
	relatedActions := []string{"Project.Get"}
	runtimeNotes := []string{"Validate project ownership."}
	spec := NewActionSpec(" delete ", route, ActionSpecOptions{
		Aliases:                aliases,
		Tags:                   tags,
		RelatedActions:         relatedActions,
		ParameterGuidance:      map[string]ParameterGuidance{"project_id": specGuidance},
		ReadOnly:               false,
		OwnerPackage:           "projects",
		IndividualTool:         IndividualToolSpec{Name: "gitlab_delete_project", Title: "Delete project", Description: "Delete a GitLab project."},
		ContentKind:            "mutate",
		NotFoundPolicy:         "not_found_result",
		EmbeddedResourcePolicy: "none",
		RichResultPolicy:       "standard",
		RuntimeValidationNotes: runtimeNotes,
	})

	inputSchema["properties"].(map[string]any)["project_id"] = map[string]any{"type": "integer"}
	routeGuidance.CommonConfusions[0] = "changed route"
	specGuidance.CommonConfusions[0] = "changed spec"
	aliases[0] = "changed"
	tags[0] = "changed"
	relatedActions[0] = "changed"
	runtimeNotes[0] = "changed"

	if spec.Name != "delete" || !spec.Destructive {
		t.Fatalf("spec = %+v, want trimmed destructive action", spec)
	}
	if got := spec.Route.InputSchema["properties"].(map[string]any)["project_id"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("spec input schema type = %v, want string", got)
	}
	if got := spec.Route.ParameterGuidance["project_id"].CommonConfusions[0]; got != "route confusion" {
		t.Fatalf("route guidance confusion = %q, want original value", got)
	}
	if got := spec.ParameterGuidance["project_id"].CommonConfusions[0]; got != "spec confusion" {
		t.Fatalf("spec guidance confusion = %q, want original value", got)
	}
	if len(spec.Aliases) != 1 || spec.Aliases[0] != "project.delete" {
		t.Fatalf("aliases = %+v, want normalized unique alias", spec.Aliases)
	}
	if len(spec.Tags) != 1 || spec.Tags[0] != "admin" {
		t.Fatalf("tags = %+v, want normalized unique tag", spec.Tags)
	}
	if spec.RelatedActions[0] != "project.get" || spec.RuntimeValidationNotes[0] != "validate project ownership." {
		t.Fatalf("related/actions notes = %+v / %+v, want cloned normalized values", spec.RelatedActions, spec.RuntimeValidationNotes)
	}
}

func TestActionSpecsToMapWithError_RejectsDuplicateNames(t *testing.T) {
	route := ActionRoute{}
	specs := []ActionSpec{
		NewActionSpec("get", route, ActionSpecOptions{}),
		NewActionSpec("get", route, ActionSpecOptions{}),
	}

	_, err := ActionSpecsToMapWithError(specs)
	if err == nil || !strings.Contains(err.Error(), "duplicate action spec") {
		t.Fatalf("ActionSpecsToMapWithError() error = %v, want duplicate rejection", err)
	}
}

func TestActionSpecsToMapWithError_MergesGuidanceWithoutOverwritingRouteFields(t *testing.T) {
	route := ActionRoute{
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string"},
			},
		},
		ParameterGuidance: map[string]ParameterGuidance{
			"project_id": {SemanticRole: "route_scope", CommonConfusions: []string{"route confusion"}},
		},
	}
	spec := NewActionSpec("remove", route, ActionSpecOptions{
		ParameterGuidance: map[string]ParameterGuidance{
			"project_id": {SemanticRole: "spec_scope", ValueSource: "prompt", ExampleBinding: "project `my/project`", CommonConfusions: []string{"spec confusion"}},
		},
	})

	routes, err := ActionSpecsToMapWithError([]ActionSpec{spec})
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	guidance := routes["remove"].ParameterGuidance["project_id"]
	if guidance.SemanticRole != "route_scope" || guidance.ValueSource != "prompt" || guidance.ExampleBinding != "project `my/project`" {
		t.Fatalf("guidance = %+v, want route precedence plus spec fill-ins", guidance)
	}
	if len(guidance.CommonConfusions) != 2 || guidance.CommonConfusions[0] != "route confusion" || guidance.CommonConfusions[1] != "spec confusion" {
		t.Fatalf("CommonConfusions = %+v, want route then spec", guidance.CommonConfusions)
	}
}

func TestActionSpecsToMapWithError_AllowsNilRouteSchemasWithoutGuidance(t *testing.T) {
	spec := NewActionSpec("current", ActionRoute{}, ActionSpecOptions{Tags: []string{"Read"}})

	routes, err := ActionSpecsToMapWithError([]ActionSpec{spec})
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	if routes["current"].InputSchema != nil || routes["current"].OutputSchema != nil {
		t.Fatalf("route schemas = %+v / %+v, want nil schemas", routes["current"].InputSchema, routes["current"].OutputSchema)
	}
}

func TestActionSpecValidate_RejectsUnknownGuidanceParameter(t *testing.T) {
	route := ActionRoute{InputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
	}}
	spec := NewActionSpec("get", route, ActionSpecOptions{
		ParameterGuidance: map[string]ParameterGuidance{"missing": {SemanticRole: "missing_param"}},
	})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "unknown parameter") {
		t.Fatalf("Validate() error = %v, want unknown parameter rejection", err)
	}
}
