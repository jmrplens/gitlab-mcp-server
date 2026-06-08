package toolutil

import (
	"reflect"
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
	modeEnum, ok := mode["enum"].([]string)
	if !ok {
		t.Fatalf("mode enum = %#v, want []string", mode["enum"])
	}
	if wantModeEnum := []string{"ADD", "REMOVE"}; !reflect.DeepEqual(modeEnum, wantModeEnum) {
		t.Fatalf("mode enum = %#v, want %#v", modeEnum, wantModeEnum)
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
	cloneEnum, ok := clone.InputSchemaOverrides[1].Values["enum"].([]string)
	if !ok {
		t.Fatalf("CloneActionSpec enum type = %T, want []string", clone.InputSchemaOverrides[1].Values["enum"])
	}
	if cloneEnum[0] != "ADD" {
		t.Fatalf("clone schema overrides share metadata, got %q", cloneEnum[0])
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

// TestSchemaAnyOfRequired_RejectsEmptyPropertyNames verifies anyOf schema
// requirements cannot be created from blank property names.
//
// The test builds an update action with an empty SchemaAnyOfRequired override and
// expects validation to reject it. This prevents generated input schemas from
// containing impossible required-property alternatives.
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

// TestStandardActionSpecConstructors verifies the convenience constructors set
// canonical read/create/update/delete behavior flags without dropping options.
func TestStandardActionSpecConstructors(t *testing.T) {
	tests := []struct {
		name            string
		spec            ActionSpec
		wantReadOnly    bool
		wantDestructive bool
		wantIdempotent  bool
	}{
		{name: "read", spec: NewReadActionSpec("list", ActionRoute{}, ActionSpecOptions{OwnerPackage: "example"}), wantReadOnly: true, wantIdempotent: true},
		{name: "create", spec: NewCreateActionSpec("create", ActionRoute{}, ActionSpecOptions{OwnerPackage: "example"})},
		{name: "update", spec: NewUpdateActionSpec("update", ActionRoute{}, ActionSpecOptions{OwnerPackage: "example"}), wantIdempotent: true},
		{name: "delete", spec: NewDeleteActionSpec("delete", ActionRoute{}, ActionSpecOptions{OwnerPackage: "example"}), wantDestructive: true, wantIdempotent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.spec.ReadOnly != tt.wantReadOnly || tt.spec.Destructive != tt.wantDestructive || tt.spec.Route.Destructive != tt.wantDestructive || tt.spec.Idempotent != tt.wantIdempotent {
				t.Fatalf("flags = read:%t destructive:%t routeDestructive:%t idempotent:%t, want read:%t destructive:%t idempotent:%t", tt.spec.ReadOnly, tt.spec.Destructive, tt.spec.Route.Destructive, tt.spec.Idempotent, tt.wantReadOnly, tt.wantDestructive, tt.wantIdempotent)
			}
			if tt.spec.OwnerPackage != "example" {
				t.Fatalf("OwnerPackage = %q, want example", tt.spec.OwnerPackage)
			}
		})
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

// TestActionSpecsToMapWithError_ProjectsActionMetadata verifies non-guidance
// catalog metadata remains available to meta-tool routes.
func TestActionSpecsToMapWithError_ProjectsActionMetadata(t *testing.T) {
	route := ActionRoute{
		Aliases:        []string{"route alias"},
		Tags:           []string{"route"},
		Usage:          "route usage",
		RelatedActions: []string{"route.get"},
	}
	spec := NewActionSpec("settings_get", route, ActionSpecOptions{
		Aliases:        []string{"instance settings"},
		Tags:           []string{"settings"},
		Usage:          "Read current GitLab application settings.",
		RelatedActions: []string{"metadata.get"},
	})

	routes, err := ActionSpecsToMapWithError([]ActionSpec{spec})
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	got := routes["settings_get"]
	if got.Usage != "Read current GitLab application settings." {
		t.Fatalf("Usage = %q, want spec usage", got.Usage)
	}
	if len(got.Aliases) != 2 || got.Aliases[0] != "route alias" || got.Aliases[1] != "instance settings" {
		t.Fatalf("Aliases = %+v, want merged route and spec aliases", got.Aliases)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "route" || got.Tags[1] != "settings" {
		t.Fatalf("Tags = %+v, want merged route and spec tags", got.Tags)
	}
	if len(got.RelatedActions) != 2 || got.RelatedActions[0] != "route.get" || got.RelatedActions[1] != "metadata.get" {
		t.Fatalf("RelatedActions = %+v, want merged route and spec related actions", got.RelatedActions)
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

// TestValidateActionSpecAliasesAgainstNames_AliasEqualsCanonical verifies the
// early-continue branch when an alias matches the spec's own canonical name
// (after normalization). The function must not flag self-referential aliases,
// but must still surface collisions with canonical names of other specs.
func TestValidateActionSpecAliasesAgainstNames_AliasEqualsCanonical(t *testing.T) {
	spec := ActionSpec{Name: "LIST", Aliases: []string{"list", "show"}}
	canonicalNames := map[string]struct{}{"show": {}}

	// "list" alias matches canonicalName → skipped; "show" alias is in
	// canonicalNames → error returned for the second alias.
	if err := validateActionSpecAliasesAgainstNames(spec, canonicalNames); err == nil ||
		!strings.Contains(err.Error(), "duplicates canonical action name") {
		t.Fatalf("validateActionSpecAliasesAgainstNames() = %v, want canonical-name collision for 'show'", err)
	}
}

// TestValidateActionSpecAliasesAgainstNames_NoCollision verifies the function
// returns nil when none of the aliases collide with known canonical names.
func TestValidateActionSpecAliasesAgainstNames_NoCollision(t *testing.T) {
	spec := ActionSpec{Name: "list", Aliases: []string{"show", "fetch"}}
	canonicalNames := map[string]struct{}{"get": {}}

	if err := validateActionSpecAliasesAgainstNames(spec, canonicalNames); err != nil {
		t.Fatalf("validateActionSpecAliasesAgainstNames() = %v, want nil", err)
	}
}

// TestValidateActionAliasSpecs_DuplicateAliasSameTarget verifies the
// duplicate-alias-with-same-target branch of validateActionAliasSpecs is
// silently accepted (the seen map is keyed on alias name, target is canonical).
func TestValidateActionAliasSpecs_DuplicateAliasSameTarget(t *testing.T) {
	aliases := []ActionAliasSpec{
		{Alias: "old", Target: "delete", Source: "dynamic", Reason: "first target"},
		{Alias: "old", Target: "delete", Source: "dynamic", Reason: "second same target"},
	}
	if err := validateActionAliasSpecs("delete", aliases); err != nil {
		t.Fatalf("validateActionAliasSpecs(same targets) error = %v, want nil", err)
	}
}

// TestValidateActionAliasSpecs_NoSourceAndNoReason covers the empty Source
// and empty Reason validation branches for action aliases.
func TestValidateActionAliasSpecs_NoSourceAndNoReason(t *testing.T) {
	tests := []struct {
		name    string
		aliases []ActionAliasSpec
		want    string
	}{
		{
			name:    "no source",
			aliases: []ActionAliasSpec{{Alias: "old", Target: "delete", Reason: "r"}},
			want:    "has no source",
		},
		{
			name:    "no reason",
			aliases: []ActionAliasSpec{{Alias: "old", Target: "delete", Source: "dynamic"}},
			want:    "has no reason",
		},
		{
			name:    "deprecated without removal version",
			aliases: []ActionAliasSpec{{Alias: "old", Target: "delete", Source: "dynamic", Deprecated: true, Reason: "r"}},
			want:    "has no removal version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateActionAliasSpecs("delete", tt.aliases); err == nil ||
				!strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateActionAliasSpecs() = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestValidateParameterAliasSpecs_DuplicateAliasSameTarget verifies the
// duplicate-alias-with-same-target branch is accepted, and that the conflicting
// target branch returns the expected error.
func TestValidateParameterAliasSpecs_DuplicateAliasSameTarget(t *testing.T) {
	schema := testActionSpecSchema("project_id", "namespace_id")
	sameTarget := []ParameterAliasSpec{
		{Alias: "p", Target: "project_id", Source: "dynamic", Reason: "first"},
		{Alias: "p", Target: "project_id", Source: "dynamic", Reason: "second"},
	}
	if err := validateParameterAliasSpecs("get", schema, sameTarget); err != nil {
		t.Fatalf("validateParameterAliasSpecs(same targets) error = %v, want nil", err)
	}
	conflicting := []ParameterAliasSpec{
		{Alias: "p", Target: "project_id", Source: "dynamic", Reason: "first"},
		{Alias: "p", Target: "namespace_id", Source: "dynamic", Reason: "second"},
	}
	if err := validateParameterAliasSpecs("get", schema, conflicting); err == nil ||
		!strings.Contains(err.Error(), "targets both") {
		t.Fatalf("validateParameterAliasSpecs(conflicting) = %v, want both-targets error", err)
	}
}

// TestValidateParameterAliasSpecs_NoSource covers the "no source" branch of
// validateParameterAliasSpecs that the public API tests do not exercise.
func TestValidateParameterAliasSpecs_NoSource(t *testing.T) {
	schema := testActionSpecSchema("project_id")
	aliases := []ParameterAliasSpec{{Alias: "p", Target: "project_id", Reason: "r"}}
	if err := validateParameterAliasSpecs("get", schema, aliases); err == nil ||
		!strings.Contains(err.Error(), "has no source") {
		t.Fatalf("validateParameterAliasSpecs() = %v, want 'has no source' error", err)
	}
}

// TestSchemaHasPropertyPath covers the direct helper: empty path, top-level
// match, missing top-level, and nested match.
func TestSchemaHasPropertyPath(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"items": map[string]any{
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

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty path", "", false},
		{"whitespace path", "  ", false},
		{"top level match", "project_id", true},
		{"nested match", "items.file_path", true},
		{"missing top level", "missing", false},
		{"missing nested", "items.missing", false},
		{"no properties", "x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schemaHasPropertyPath(schema, tt.path); got != tt.want {
				t.Errorf("schemaHasPropertyPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestSchemaHasPropertyPathFrom_NoProperties exercises the "no properties key"
// defensive branch in the recursive resolver.
func TestSchemaHasPropertyPathFrom_NoProperties(t *testing.T) {
	schema := map[string]any{"type": "object"} // no "properties"
	if schemaHasPropertyPathFrom(schema, schema, []string{"x"}) {
		t.Error("schemaHasPropertyPathFrom(no properties) = true, want false")
	}
}

// TestResolveSchemaRef covers all branches of the $ref resolver: missing ref,
// unsupported prefix, missing $defs, missing definition, and successful
// resolution.
func TestResolveSchemaRef(t *testing.T) {
	tests := []struct {
		name string
		root map[string]any
		sch  map[string]any
		want map[string]any
	}{
		{
			name: "no ref returns schema unchanged",
			root: map[string]any{},
			sch:  map[string]any{"type": "string"},
			want: map[string]any{"type": "string"},
		},
		{
			name: "unsupported ref prefix returns schema unchanged",
			root: map[string]any{},
			sch:  map[string]any{"$ref": "#/components/schemas/Foo"},
			want: map[string]any{"$ref": "#/components/schemas/Foo"},
		},
		{
			name: "missing $defs returns schema unchanged",
			root: map[string]any{},
			sch:  map[string]any{"$ref": "#/$defs/Foo"},
			want: map[string]any{"$ref": "#/$defs/Foo"},
		},
		{
			name: "missing definition returns schema unchanged",
			root: map[string]any{"$defs": map[string]any{}},
			sch:  map[string]any{"$ref": "#/$defs/Foo"},
			want: map[string]any{"$ref": "#/$defs/Foo"},
		},
		{
			name: "successful $ref resolution",
			root: map[string]any{
				"$defs": map[string]any{
					"Foo": map[string]any{"type": "integer"},
				},
			},
			sch:  map[string]any{"$ref": "#/$defs/Foo"},
			want: map[string]any{"type": "integer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSchemaRef(tt.root, tt.sch)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveSchemaRef() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateInputSchemaOverrides_NoInputSchema covers the branch in
// validateInputSchemaOverrides that rejects overrides when the spec has no
// input schema at all.
func TestValidateInputSchemaOverrides_NoInputSchema(t *testing.T) {
	spec := ActionSpec{
		Name: "no-schema",
		InputSchemaOverrides: []InputSchemaOverride{
			SchemaPropertyOverride("foo", map[string]any{"type": "string"}),
		},
	}
	if err := validateInputSchemaOverrides(spec); err == nil ||
		!strings.Contains(err.Error(), "without an input schema") {
		t.Fatalf("validateInputSchemaOverrides() = %v, want missing-schema error", err)
	}
}

// TestSchemaOverrideTargetFrom_NoProperties covers the "no properties" branch
// of the recursive schema resolver used by input schema overrides.
func TestSchemaOverrideTargetFrom_NoProperties(t *testing.T) {
	root := map[string]any{"type": "object"} // no "properties"
	if got := schemaOverrideTargetFrom(root, root, []string{"foo"}); got != nil {
		t.Errorf("schemaOverrideTargetFrom(no properties) = %v, want nil", got)
	}
}

// TestSchemaOverrideTargetFrom_EmptyParts covers the defensive early-return
// when the recursive resolver is invoked with an empty parts slice.
func TestSchemaOverrideTargetFrom_EmptyParts(t *testing.T) {
	root := map[string]any{"type": "object"}
	if got := schemaOverrideTargetFrom(root, root, nil); got != nil {
		t.Errorf("schemaOverrideTargetFrom(nil parts) = %v, want nil", got)
	}
	if got := schemaOverrideTargetFrom(root, root, []string{}); got != nil {
		t.Errorf("schemaOverrideTargetFrom(empty parts) = %v, want nil", got)
	}
}

// TestSchemaOverrideTargetFrom_NilChildProperty covers the defensive guard
// when the resolved child map is nil but the parts slice still has more
// entries. This is exercised by storing a typed-nil map under the property
// name so the type assertion succeeds but the resulting value is nil.
func TestSchemaOverrideTargetFrom_NilChildProperty(t *testing.T) {
	var nilMap map[string]any
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"child": nilMap, // typed-nil map
		},
	}
	if got := schemaOverrideTargetFrom(root, root, []string{"child", "nested"}); got != nil {
		t.Errorf("schemaOverrideTargetFrom(nil child) = %v, want nil", got)
	}
}

// TestSchemaHasPropertyPathFrom_EmptyParts covers the defensive early-return
// when the recursive resolver is invoked with an empty parts slice.
func TestSchemaHasPropertyPathFrom_EmptyParts(t *testing.T) {
	root := map[string]any{"type": "object"}
	if schemaHasPropertyPathFrom(root, root, nil) {
		t.Error("schemaHasPropertyPathFrom(nil parts) = true, want false")
	}
	if schemaHasPropertyPathFrom(root, root, []string{}) {
		t.Error("schemaHasPropertyPathFrom(empty parts) = true, want false")
	}
}

// TestSchemaHasPropertyPathFrom_NilChildProperty covers the defensive guard
// when the resolved child map is nil but the parts slice still has more
// entries.
func TestSchemaHasPropertyPathFrom_NilChildProperty(t *testing.T) {
	var nilMap map[string]any
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"child": nilMap, // typed-nil map
		},
	}
	if schemaHasPropertyPathFrom(root, root, []string{"child", "nested"}) {
		t.Error("schemaHasPropertyPathFrom(nil child) = true, want false")
	}
}
