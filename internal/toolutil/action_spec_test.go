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
	compatibility := CompatibilityPolicy{
		ActionAliases:    []ActionAliasSpec{{Alias: " Project.Remove ", Target: " Delete ", Source: "dynamic", Searchable: true, Reason: "Historical dynamic alias."}},
		ParameterAliases: []ParameterAliasSpec{{Alias: " Project ", Target: "project_id", Source: "dynamic", Reason: "Historical dynamic parameter alias."}},
	}
	schemaNotes := []string{"Schema cannot express file source exclusivity."}
	runtimeNotes := []string{"Validate project ownership."}
	individualIdempotent := false
	spec := NewActionSpec(" delete ", route, ActionSpecOptions{
		Aliases:                aliases,
		Tags:                   tags,
		RelatedActions:         relatedActions,
		Compatibility:          compatibility,
		ParameterGuidance:      map[string]ParameterGuidance{"project_id": specGuidance},
		ReadOnly:               false,
		OwnerPackage:           "projects",
		IndividualTool:         IndividualToolSpec{Name: "gitlab_delete_project", Title: "Delete project", Description: "Delete a GitLab project.", AnnotationOverrides: IndividualToolAnnotationOverrides{Idempotent: &individualIdempotent}},
		ContentKind:            ActionSpecContentMutate,
		NotFoundPolicy:         ActionSpecNotFoundResult,
		EmbeddedResourcePolicy: ActionSpecEmbeddedNone,
		RichResultPolicy:       ActionSpecRichStandard,
		SchemaValidationNotes:  schemaNotes,
		RuntimeValidationNotes: runtimeNotes,
	})

	inputSchema["properties"].(map[string]any)["project_id"] = map[string]any{"type": "integer"}
	routeGuidance.CommonConfusions[0] = "changed route"
	specGuidance.CommonConfusions[0] = "changed spec"
	aliases[0] = "changed"
	tags[0] = "changed"
	relatedActions[0] = "changed"
	compatibility.ActionAliases[0].Alias = "changed"
	compatibility.ParameterAliases[0].Alias = "changed"
	schemaNotes[0] = "changed"
	runtimeNotes[0] = "changed"
	individualIdempotent = true

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
	if spec.RelatedActions[0] != "project.get" || spec.SchemaValidationNotes[0] != "Schema cannot express file source exclusivity." || spec.RuntimeValidationNotes[0] != "Validate project ownership." {
		t.Fatalf("related/actions notes = %+v / %+v / %+v, want cloned normalized values", spec.RelatedActions, spec.SchemaValidationNotes, spec.RuntimeValidationNotes)
	}
	if spec.Compatibility.ActionAliases[0].Alias != "project.remove" || spec.Compatibility.ActionAliases[0].Target != "delete" {
		t.Fatalf("action compatibility aliases = %+v, want cloned normalized action alias", spec.Compatibility.ActionAliases)
	}
	if spec.Compatibility.ParameterAliases[0].Alias != "project" || spec.Compatibility.ParameterAliases[0].Target != "project_id" {
		t.Fatalf("parameter compatibility aliases = %+v, want cloned normalized parameter alias", spec.Compatibility.ParameterAliases)
	}
	if spec.IndividualTool.AnnotationOverrides.Idempotent == nil || *spec.IndividualTool.AnnotationOverrides.Idempotent {
		t.Fatalf("individual idempotent override = %v, want cloned false", spec.IndividualTool.AnnotationOverrides.Idempotent)
	}
}

func TestActionSpecValidate_CompatibilityPolicy(t *testing.T) {
	spec := NewActionSpec("delete", ActionRoute{InputSchema: testActionSpecSchema("project_id")}, ActionSpecOptions{
		Compatibility: CompatibilityPolicy{
			ActionAliases:    []ActionAliasSpec{{Alias: "remove", Target: "delete", Source: "dynamic", Searchable: true, Deprecated: true, RemovalVersion: "v3.0.0", Reason: "Preserve old Dynamic phrasing."}},
			ParameterAliases: []ParameterAliasSpec{{Alias: "project", Target: "project_id", Source: "dynamic", Reason: "Map shorthand prompts to the canonical parameter."}},
		},
	})

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestActionSpecValidate_RejectsInvalidCompatibilityPolicy(t *testing.T) {
	testCases := []struct {
		name string
		opts ActionSpecOptions
		want string
	}{
		{
			name: "action alias target mismatch",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ActionAliases: []ActionAliasSpec{{Alias: "remove", Target: "archive", Source: "dynamic", Reason: "wrong action"}}}},
			want: "targets \"archive\"",
		},
		{
			name: "missing source",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ActionAliases: []ActionAliasSpec{{Alias: "remove", Target: "delete", Reason: "missing source"}}}},
			want: "has no source",
		},
		{
			name: "deprecated alias without removal version",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ActionAliases: []ActionAliasSpec{{Alias: "remove", Target: "delete", Source: "dynamic", Deprecated: true, Reason: "missing version"}}}},
			want: "has no removal version",
		},
		{
			name: "unknown parameter target",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ParameterAliases: []ParameterAliasSpec{{Alias: "project", Target: "project", Source: "dynamic", Reason: "wrong parameter"}}}},
			want: "targets unknown parameter \"project\"",
		},
		{
			name: "parameter alias conflicting target",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ParameterAliases: []ParameterAliasSpec{
				{Alias: "project", Target: "project_id", Source: "dynamic", Reason: "first target"},
				{Alias: "project", Target: "namespace_id", Source: "dynamic", Reason: "second target"},
			}}},
			want: "targets both \"project_id\" and \"namespace_id\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := NewActionSpec("delete", ActionRoute{InputSchema: testActionSpecSchema("project_id", "namespace_id")}, tc.opts)
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestActionSpecValidate_RejectsUnsupportedIndividualPolicies(t *testing.T) {
	testCases := []struct {
		name string
		opts ActionSpecOptions
		want string
	}{
		{
			name: "content kind",
			opts: ActionSpecOptions{ContentKind: "summary"},
			want: "unsupported content kind",
		},
		{
			name: "not found policy",
			opts: ActionSpecOptions{NotFoundPolicy: "custom_404"},
			want: "unsupported not-found policy",
		},
		{
			name: "embedded resource policy",
			opts: ActionSpecOptions{EmbeddedResourcePolicy: "sometimes"},
			want: "unsupported embedded resource policy",
		},
		{
			name: "rich result policy",
			opts: ActionSpecOptions{RichResultPolicy: "binary"},
			want: "unsupported rich result policy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := NewActionSpec("get", ActionRoute{}, tc.opts)
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestActionSpecValidate_AcceptsKnownIndividualPolicies(t *testing.T) {
	spec := NewActionSpec("get", ActionRoute{}, ActionSpecOptions{
		ContentKind:            ActionSpecContentDetail,
		NotFoundPolicy:         ActionSpecNotFoundPropagate,
		EmbeddedResourcePolicy: ActionSpecEmbeddedOptional,
		RichResultPolicy:       ActionSpecRichImage,
	})

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestNewActionSpec_SyncsOptionDestructiveToRoute(t *testing.T) {
	spec := NewActionSpec("delete", ActionRoute{}, ActionSpecOptions{Destructive: true})

	if !spec.Destructive || !spec.Route.Destructive {
		t.Fatalf("destructive flags = spec:%t route:%t, want both true", spec.Destructive, spec.Route.Destructive)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
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

func TestActionSpecsToMap_ProjectsValidSpecsAndPanicsOnInvalid(t *testing.T) {
	valid := NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id")}, ActionSpecOptions{ReadOnly: true})
	routes := ActionSpecsToMap([]ActionSpec{valid})
	if _, ok := routes["get"]; !ok {
		t.Fatal("ActionSpecsToMap() missing get route")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("ActionSpecsToMap() did not panic for invalid spec")
		}
	}()
	_ = ActionSpecsToMap([]ActionSpec{{Name: ""}})
}

func TestActionSpecsToMapWithError_RejectsAliasMatchingCanonicalActionName(t *testing.T) {
	specs := []ActionSpec{
		NewActionSpec("list", ActionRoute{}, ActionSpecOptions{Aliases: []string{"show"}}),
		NewActionSpec("show", ActionRoute{}, ActionSpecOptions{}),
	}

	_, err := ActionSpecsToMapWithError(specs)
	if err == nil || !strings.Contains(err.Error(), "duplicates canonical action name") {
		t.Fatalf("ActionSpecsToMapWithError() error = %v, want alias/canonical action collision", err)
	}
}

func TestActionRouteFluentMetadata_FlowsToActionSpec(t *testing.T) {
	guidance := map[string]ParameterGuidance{
		"project_id": {SemanticRole: "scope_project", CommonConfusions: []string{"route confusion"}},
	}
	route := ActionRoute{InputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
	}}.
		WithParameterGuidance(guidance).
		WithAliases(" Project Search ", "project search").
		WithTags(" Project ", "project").
		WithUsage(" Use when searching projects. ").
		WithRelatedActions(" Project.Get ")

	guidance["project_id"] = ParameterGuidance{SemanticRole: "changed"}
	spec := NewActionSpec("list", route, ActionSpecOptions{})
	route.Aliases[0] = "changed"
	route.Tags[0] = "changed"
	route.RelatedActions[0] = "changed"
	route.ParameterGuidance["project_id"] = ParameterGuidance{SemanticRole: "changed"}

	if len(spec.Aliases) != 1 || spec.Aliases[0] != "project search" {
		t.Fatalf("aliases = %+v, want route aliases", spec.Aliases)
	}
	if len(spec.Tags) != 1 || spec.Tags[0] != "project" {
		t.Fatalf("tags = %+v, want route tags", spec.Tags)
	}
	if spec.Usage != "Use when searching projects." {
		t.Fatalf("Usage = %q, want trimmed route usage", spec.Usage)
	}
	if len(spec.RelatedActions) != 1 || spec.RelatedActions[0] != "project.get" {
		t.Fatalf("RelatedActions = %+v, want route related action", spec.RelatedActions)
	}
	if got := spec.Route.ParameterGuidance["project_id"].SemanticRole; got != "scope_project" {
		t.Fatalf("route guidance semantic role = %q, want cloned route guidance", got)
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

func TestActionSpecsToMapWithError_DeduplicatesCommonConfusions(t *testing.T) {
	route := ActionRoute{
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
		},
		ParameterGuidance: map[string]ParameterGuidance{
			"project_id": {CommonConfusions: []string{"do not use target_project_id", "do not use target_project_id"}},
		},
	}
	spec := NewActionSpec("remove", route, ActionSpecOptions{
		ParameterGuidance: map[string]ParameterGuidance{
			"project_id": {CommonConfusions: []string{"do not use target_project_id", "do not use source_project_id"}},
		},
	})

	routes, err := ActionSpecsToMapWithError([]ActionSpec{spec})
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	confusions := routes["remove"].ParameterGuidance["project_id"].CommonConfusions
	if len(confusions) != 2 || confusions[0] != "do not use target_project_id" || confusions[1] != "do not use source_project_id" {
		t.Fatalf("CommonConfusions = %+v, want deduplicated route then spec values", confusions)
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

func TestActionSpecValidate_RejectsGuidanceWithoutInputSchema(t *testing.T) {
	spec := NewActionSpec("get", ActionRoute{}, ActionSpecOptions{
		ParameterGuidance: map[string]ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
	})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "without an input schema") {
		t.Fatalf("Validate() error = %v, want missing schema rejection", err)
	}
}

func TestActionSpecValidate_RejectsEmptyName(t *testing.T) {
	if err := (ActionSpec{}).Validate(); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("Validate() error = %v, want missing name rejection", err)
	}
}

func TestActionSpecValidate_RejectsReadOnlyDestructive(t *testing.T) {
	spec := NewActionSpec("delete", ActionRoute{Destructive: true}, ActionSpecOptions{ReadOnly: true, Destructive: true})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "read-only and destructive") {
		t.Fatalf("Validate() error = %v, want read-only destructive rejection", err)
	}
}

func TestActionSpecValidate_RejectsConflictingAliases(t *testing.T) {
	testCases := []struct {
		name string
		spec ActionSpec
		want string
	}{
		{
			name: "alias duplicates action name",
			spec: NewActionSpec("list", ActionRoute{}, ActionSpecOptions{Aliases: []string{"list"}}),
			want: "duplicates its action name",
		},
		{
			name: "alias also related action",
			spec: NewActionSpec("list", ActionRoute{}, ActionSpecOptions{Aliases: []string{"project.get"}, RelatedActions: []string{"project.get"}}),
			want: "also appears in related actions",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.spec.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestActionSpecValidate_RejectsDestructiveMismatch(t *testing.T) {
	spec := ActionSpec{Name: "delete", Route: ActionRoute{Destructive: true}}

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "destructive flag") {
		t.Fatalf("Validate() error = %v, want destructive mismatch rejection", err)
	}
}

func TestActionSpecValidate_RejectsNonNormalizedTags(t *testing.T) {
	spec := ActionSpec{Name: "list", Tags: []string{"Needs Cleanup"}}

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "non-normalized tag") {
		t.Fatalf("Validate() error = %v, want non-normalized tag rejection", err)
	}
}
