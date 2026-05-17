package toolutil

import (
	"strings"
	"testing"
)

// TestNewActionSpec_DeepClonesMetadata verifies NewActionSpec when deep clones metadata.
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

// TestActionSpecStringAndNoteNormalization verifies internal slice helpers
// trim, normalize, deduplicate, and preserve note casing as intended.
func TestActionSpecStringAndNoteNormalization(t *testing.T) {
	if got := normalizeActionSpecStrings(nil); got != nil {
		t.Fatalf("normalizeActionSpecStrings(nil) = %#v, want nil", got)
	}
	stringsOut := normalizeActionSpecStrings([]string{" Project.List ", "", "project.list", "Group.Get"})
	if len(stringsOut) != 2 || stringsOut[0] != "project.list" || stringsOut[1] != "group.get" {
		t.Fatalf("normalizeActionSpecStrings() = %#v", stringsOut)
	}

	if got := mergeActionSpecNotes(nil, nil); got != nil {
		t.Fatalf("mergeActionSpecNotes(nil, nil) = %#v, want nil", got)
	}
	notes := mergeActionSpecNotes([]string{" Preserve casing ", ""}, []string{"Preserve casing", "Second note"})
	if len(notes) != 2 || notes[0] != "Preserve casing" || notes[1] != "Second note" {
		t.Fatalf("mergeActionSpecNotes() = %#v", notes)
	}
}

// TestCloneActionSpecs_DefensiveCopiesMetadata verifies CloneActionSpec and
// CloneActionSpecs preserve normalized metadata without sharing mutable state.
func TestCloneActionSpecs_DefensiveCopiesMetadata(t *testing.T) {
	if got := CloneActionSpecs(nil); got != nil {
		t.Fatalf("CloneActionSpecs(nil) = %#v, want nil", got)
	}

	spec := NewActionSpec(" get ", ActionRoute{
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
		},
	}, ActionSpecOptions{
		Aliases:        []string{" Show "},
		Tags:           []string{" Projects "},
		RelatedActions: []string{"Project.List"},
		Compatibility: CompatibilityPolicy{
			ActionAliases:    []ActionAliasSpec{{Alias: "Show", Target: "get", Source: "dynamic", Reason: "legacy wording"}},
			ParameterAliases: []ParameterAliasSpec{{Alias: "project", Target: "project_id", Source: "dynamic", Reason: "legacy parameter"}},
		},
		ParameterGuidance: map[string]ParameterGuidance{"project_id": {CommonConfusions: []string{"namespace_id"}}},
		ReadOnly:          true,
		Idempotent:        true,
		OwnerPackage:      "projects",
		IndividualTool:    IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a GitLab project."},
	})

	clone := CloneActionSpec(spec)
	clones := CloneActionSpecs([]ActionSpec{spec})
	if len(clones) != 1 {
		t.Fatalf("CloneActionSpecs() length = %d, want 1", len(clones))
	}

	spec.Route.InputSchema["properties"].(map[string]any)["project_id"].(map[string]any)["type"] = "integer"
	spec.Aliases[0] = "changed"
	spec.Tags[0] = "changed"
	spec.RelatedActions[0] = "changed"
	spec.Compatibility.ActionAliases[0].Alias = "changed"
	spec.Compatibility.ParameterAliases[0].Alias = "changed"
	spec.ParameterGuidance["project_id"] = ParameterGuidance{CommonConfusions: []string{"changed"}}

	assertClonedSpec := func(t *testing.T, got ActionSpec) {
		t.Helper()
		if got.Name != "get" || !got.ReadOnly || !got.Idempotent {
			t.Fatalf("clone metadata = %+v, want normalized read-only get action", got)
		}
		if got.Route.InputSchema["properties"].(map[string]any)["project_id"].(map[string]any)["type"] != "string" {
			t.Fatalf("clone shares input schema with source: %#v", got.Route.InputSchema)
		}
		if got.Aliases[0] != "show" || got.Tags[0] != "projects" || got.RelatedActions[0] != "project.list" {
			t.Fatalf("clone normalized slices = aliases:%v tags:%v related:%v", got.Aliases, got.Tags, got.RelatedActions)
		}
		if got.Compatibility.ActionAliases[0].Alias != "show" || got.Compatibility.ParameterAliases[0].Alias != "project" {
			t.Fatalf("clone compatibility = %+v", got.Compatibility)
		}
		if got.ParameterGuidance["project_id"].CommonConfusions[0] != "namespace_id" {
			t.Fatalf("clone parameter guidance = %+v", got.ParameterGuidance)
		}
	}

	assertClonedSpec(t, clone)
	assertClonedSpec(t, clones[0])
}

// TestActionSpecValidate_CompatibilityPolicy verifies ActionSpecValidate when compatibility policy.
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

// TestActionSpecValidate_CompatibilityPolicyAcceptsNestedParameterAlias verifies nested schema paths can be used in parameter alias metadata.
func TestActionSpecValidate_CompatibilityPolicyAcceptsNestedParameterAlias(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"files": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	spec := NewActionSpec("project_create", ActionRoute{InputSchema: schema}, ActionSpecOptions{
		Compatibility: CompatibilityPolicy{
			ParameterAliases: []ParameterAliasSpec{{Alias: "files.file_name", Target: "files.file_path", Source: "dynamic", Reason: "Map legacy file names to file paths."}},
		},
	})

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

// TestNewActionSpec_AppliesInputSchemaOverrides verifies ActionSpec schema
// overrides patch root, property, and nested array-item schemas defensively.
func TestNewActionSpec_AppliesInputSchemaOverrides(t *testing.T) {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{"type": "string"},
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"color": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	overrides := []InputSchemaOverride{
		SchemaAnyOfRequired("mode", "items"),
		SchemaPropertyOverride("mode", map[string]any{"enum": []string{"ADD", "REMOVE"}}),
		SchemaPropertyOverride("items", map[string]any{"minItems": 1}),
		SchemaPropertyOverride("items.color", map[string]any{"pattern": "^#[0-9A-Fa-f]{6}$"}),
	}

	spec := NewActionSpec("bulk_update", ActionRoute{InputSchema: inputSchema}, ActionSpecOptions{InputSchemaOverrides: overrides})
	overrides[1].Values["enum"] = []string{"changed"}

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got := spec.Route.InputSchema["anyOf"].([]any); len(got) != 2 {
		t.Fatalf("anyOf = %#v, want two branches", got)
	}
	mode := spec.Route.InputSchema["properties"].(map[string]any)["mode"].(map[string]any)
	modeEnum := mode["enum"].([]string)
	if modeEnum[0] != "ADD" {
		t.Fatalf("mode enum = %#v, want cloned ADD/REMOVE", modeEnum)
	}
	items := spec.Route.InputSchema["properties"].(map[string]any)["items"].(map[string]any)
	if got := items["minItems"]; got != 1 {
		t.Fatalf("items minItems = %v, want 1", got)
	}
	color := items["items"].(map[string]any)["properties"].(map[string]any)["color"].(map[string]any)
	if got := color["pattern"]; got != "^#[0-9A-Fa-f]{6}$" {
		t.Fatalf("color pattern = %v, want hex pattern", got)
	}

	clone := CloneActionSpec(spec)
	spec.InputSchemaOverrides[1].Values["enum"] = []string{"mutated"}
	if got := clone.InputSchemaOverrides[1].Values["enum"].([]string)[0]; got != "ADD" {
		t.Fatalf("clone schema overrides share metadata, got %q", got)
	}
}

// TestActionSpecValidate_RejectsInvalidInputSchemaOverride verifies schema
// overrides must target existing input properties.
func TestActionSpecValidate_RejectsInvalidInputSchemaOverride(t *testing.T) {
	spec := NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id")}, ActionSpecOptions{
		InputSchemaOverrides: []InputSchemaOverride{SchemaPropertyOverride("missing", map[string]any{"type": "string"})},
	})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "unknown property path") {
		t.Fatalf("Validate() error = %v, want unknown property path", err)
	}
}

func TestSchemaAnyOfRequired_RejectsEmptyPropertyNames(t *testing.T) {
	spec := NewActionSpec("update", ActionRoute{InputSchema: testActionSpecSchema("name")}, ActionSpecOptions{
		InputSchemaOverrides: []InputSchemaOverride{SchemaAnyOfRequired(" ", "")},
	})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "empty input schema override") {
		t.Fatalf("Validate() error = %v, want empty override", err)
	}
}

// TestActionSpecValidate_RejectsInvalidCompatibilityPolicy covers ActionSpecValidate with table-driven subtests for rejects invalid compatibility policy.
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

// TestActionSpecValidate_RejectsMalformedCompatibilityAliases covers malformed
// action and parameter alias metadata that would otherwise produce ambiguous
// dynamic-tool compatibility mappings.
func TestActionSpecValidate_RejectsMalformedCompatibilityAliases(t *testing.T) {
	testCases := []struct {
		name string
		opts ActionSpecOptions
		want string
	}{
		{
			name: "action alias missing alias",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ActionAliases: []ActionAliasSpec{{Target: "get", Source: "dynamic", Reason: "missing alias"}}}},
			want: "without alias",
		},
		{
			name: "action alias missing target",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ActionAliases: []ActionAliasSpec{{Alias: "show", Source: "dynamic", Reason: "missing target"}}}},
			want: "has no target",
		},
		{
			name: "action alias missing reason",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ActionAliases: []ActionAliasSpec{{Alias: "show", Target: "get", Source: "dynamic"}}}},
			want: "has no reason",
		},
		{
			name: "parameter alias missing alias",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ParameterAliases: []ParameterAliasSpec{{Target: "project_id", Source: "dynamic", Reason: "missing alias"}}}},
			want: "without alias",
		},
		{
			name: "parameter alias missing target",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ParameterAliases: []ParameterAliasSpec{{Alias: "project", Source: "dynamic", Reason: "missing target"}}}},
			want: "has no target",
		},
		{
			name: "parameter alias missing reason",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ParameterAliases: []ParameterAliasSpec{{Alias: "project", Target: "project_id", Source: "dynamic"}}}},
			want: "has no reason",
		},
		{
			name: "deprecated parameter alias without removal version",
			opts: ActionSpecOptions{Compatibility: CompatibilityPolicy{ParameterAliases: []ParameterAliasSpec{{Alias: "project", Target: "project_id", Source: "dynamic", Deprecated: true, Reason: "missing version"}}}},
			want: "has no removal version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id")}, tc.opts)
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestActionSpecValidate_RejectsUnsupportedIndividualPolicies covers ActionSpecValidate with table-driven subtests for rejects unsupported individual policies.
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

// TestActionSpecValidate_AcceptsKnownIndividualPolicies verifies ActionSpecValidate accepts known individual policies.
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

// TestNewActionSpec_SyncsOptionDestructiveToRoute verifies NewActionSpec syncs option destructive to route.
func TestNewActionSpec_SyncsOptionDestructiveToRoute(t *testing.T) {
	spec := NewActionSpec("delete", ActionRoute{}, ActionSpecOptions{Destructive: true})

	if !spec.Destructive || !spec.Route.Destructive {
		t.Fatalf("destructive flags = spec:%t route:%t, want both true", spec.Destructive, spec.Route.Destructive)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// TestActionSpecsToMapWithError_RejectsDuplicateNames verifies ActionSpecsToMapWithError rejects duplicate names.
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

// TestActionSpecsToMapWithError_CollectsInvalidSpecError verifies map
// projection reports validation failures from otherwise named specs.
func TestActionSpecsToMapWithError_CollectsInvalidSpecError(t *testing.T) {
	spec := NewActionSpec("get", ActionRoute{}, ActionSpecOptions{ContentKind: "invalid"})
	_, err := ActionSpecsToMapWithError([]ActionSpec{spec})
	if err == nil || !strings.Contains(err.Error(), "unsupported content kind") {
		t.Fatalf("ActionSpecsToMapWithError() error = %v, want validation error", err)
	}
}

// TestActionSpecsToMap_ProjectsValidSpecsAndPanicsOnInvalid verifies ActionSpecsToMap projects valid specs and panics on invalid.
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

// TestActionSpecsToMapWithError_RejectsAliasMatchingCanonicalActionName verifies ActionSpecsToMapWithError rejects alias matching canonical action name.
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

// TestActionRouteFluentMetadata_FlowsToActionSpec verifies ActionRouteFluentMetadata flows to action spec.
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

// TestActionSpecsToMapWithError_MergesGuidanceWithoutOverwritingRouteFields verifies ActionSpecsToMapWithError when merges guidance without overwriting route fields.
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

// TestActionSpecsToMapWithError_DeduplicatesCommonConfusions verifies ActionSpecsToMapWithError deduplicates common confusions.
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

// TestActionSpecsToMapWithError_AllowsNilRouteSchemasWithoutGuidance verifies ActionSpecsToMapWithError allows nil route schemas without guidance.
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

// TestActionSpecValidate_RejectsUnknownGuidanceParameter verifies ActionSpecValidate rejects unknown guidance parameter.
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

// TestActionSpecValidate_RejectsGuidanceWithoutInputSchema verifies ActionSpecValidate rejects guidance without input schema.
func TestActionSpecValidate_RejectsGuidanceWithoutInputSchema(t *testing.T) {
	spec := NewActionSpec("get", ActionRoute{}, ActionSpecOptions{
		ParameterGuidance: map[string]ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
	})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "without an input schema") {
		t.Fatalf("Validate() error = %v, want missing schema rejection", err)
	}
}

// TestActionSpecValidate_RejectsEmptyName verifies ActionSpecValidate rejects empty name.
func TestActionSpecValidate_RejectsEmptyName(t *testing.T) {
	if err := (ActionSpec{}).Validate(); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("Validate() error = %v, want missing name rejection", err)
	}
}

// TestActionSpecValidate_RejectsReadOnlyDestructive verifies ActionSpecValidate rejects read only destructive.
func TestActionSpecValidate_RejectsReadOnlyDestructive(t *testing.T) {
	spec := NewActionSpec("delete", ActionRoute{Destructive: true}, ActionSpecOptions{ReadOnly: true, Destructive: true})

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "read-only and destructive") {
		t.Fatalf("Validate() error = %v, want read-only destructive rejection", err)
	}
}

// TestActionSpecValidate_RejectsConflictingAliases covers ActionSpecValidate with table-driven subtests for rejects conflicting aliases.
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

// TestActionSpecValidate_RejectsDestructiveMismatch verifies ActionSpecValidate rejects destructive mismatch.
func TestActionSpecValidate_RejectsDestructiveMismatch(t *testing.T) {
	spec := ActionSpec{Name: "delete", Route: ActionRoute{Destructive: true}}

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "destructive flag") {
		t.Fatalf("Validate() error = %v, want destructive mismatch rejection", err)
	}
}

// TestActionSpecValidate_RejectsNonNormalizedTags verifies ActionSpecValidate rejects non normalized tags.
func TestActionSpecValidate_RejectsNonNormalizedTags(t *testing.T) {
	spec := ActionSpec{Name: "list", Tags: []string{"Needs Cleanup"}}

	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "non-normalized tag") {
		t.Fatalf("Validate() error = %v, want non-normalized tag rejection", err)
	}
}
