package toolutil

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

type optionalIndividualInput struct {
	Name string `json:"name,omitempty"`
}

// TestIndividualToolFromActionSpec_ProjectsMetadata verifies IndividualToolFromActionSpec projects metadata.
func TestIndividualToolFromActionSpec_ProjectsMetadata(t *testing.T) {
	route := ActionRoute{
		InputSchema:  testActionSpecSchema("project_id"),
		OutputSchema: testActionSpecSchema("id"),
	}
	spec := NewActionSpec("get", route, ActionSpecOptions{
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "projects",
		IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Title: "Get project", Description: "Get a GitLab project."},
	})
	icons := []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}}

	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{Icons: icons})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}

	if tool.Name != "gitlab_project_get" {
		t.Fatalf("tool name = %q, want gitlab_project_get", tool.Name)
	}
	if tool.Title != "Get project" {
		t.Fatalf("tool title = %q, want Get project", tool.Title)
	}
	if tool.Description != "Get a GitLab project." {
		t.Fatalf("tool description = %q, want spec description", tool.Description)
	}
	if tool.InputSchema == nil || tool.OutputSchema == nil {
		t.Fatal("tool schemas must be projected")
	}
	if tool.Annotations == nil {
		t.Fatal("tool annotations must be projected")
	}
	if !tool.Annotations.ReadOnlyHint {
		t.Fatal("read-only annotation = false, want true")
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Fatalf("destructive annotation = %v, want false", tool.Annotations.DestructiveHint)
	}
	if !tool.Annotations.IdempotentHint {
		t.Fatal("idempotent annotation = false, want true")
	}
	if tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
		t.Fatalf("open-world annotation = %v, want true", tool.Annotations.OpenWorldHint)
	}
	if len(tool.Icons) != 1 || tool.Icons[0].Source != icons[0].Source {
		t.Fatalf("tool icons = %+v, want copied icon", tool.Icons)
	}
	icons[0].Source = "changed"
	if tool.Icons[0].Source == "changed" {
		t.Fatal("tool icons share backing storage with projection options")
	}
}

// TestIndividualToolFromActionSpec_FallsBackToOptionDescriptionAndGeneratedTitle verifies IndividualToolFromActionSpec falls back to option description and generated title.
func TestIndividualToolFromActionSpec_FallsBackToOptionDescriptionAndGeneratedTitle(t *testing.T) {
	spec := NewActionSpec("delete", ActionRoute{
		Destructive:  true,
		InputSchema:  testActionSpecSchema("project_id"),
		OutputSchema: testActionSpecSchema("deleted"),
	}, ActionSpecOptions{
		Destructive:    true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "projects",
		IndividualTool: IndividualToolSpec{Name: "gitlab_project_delete"},
	})

	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{Description: "Delete a GitLab project."})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	if tool.Title != "Project Delete" {
		t.Fatalf("tool title = %q, want Project Delete", tool.Title)
	}
	if tool.Description != "Delete a GitLab project." {
		t.Fatalf("tool description = %q, want options description", tool.Description)
	}
	if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
		t.Fatalf("destructive annotation = %v, want true", tool.Annotations.DestructiveHint)
	}
}

// TestIndividualToolFromActionSpec_AppliesAnnotationOverrides verifies that
// IndividualToolFromActionSpec applies the annotation overrides that narrow and
// refuses the one that widens.
//
// The spec is a mutating archive that overrides all four hints. Three of them
// make the tool look less capable or less safe and are applied verbatim. The
// fourth claims readOnlyHint on an action the spec itself calls mutating, and
// is dropped: --read-only, safe mode and a gateway's auto-allow all act on that
// bit, so honoring it would widen the operator's controls rather than describe
// the tool.
func TestIndividualToolFromActionSpec_AppliesAnnotationOverrides(t *testing.T) {
	overrideReadOnly := true
	overrideDestructive := false
	overrideIdempotent := false
	overrideOpenWorld := false
	spec := NewActionSpec("archive", ActionRoute{
		InputSchema:  testActionSpecSchema("project_id"),
		OutputSchema: testActionSpecSchema("id"),
	}, ActionSpecOptions{
		Idempotent:   true,
		OpenWorld:    true,
		OwnerPackage: "projects",
		IndividualTool: IndividualToolSpec{
			Name:        "gitlab_project_archive",
			Description: "Archive a GitLab project.",
			AnnotationOverrides: IndividualToolAnnotationOverrides{
				ReadOnly:    &overrideReadOnly,
				Destructive: &overrideDestructive,
				Idempotent:  &overrideIdempotent,
				OpenWorld:   &overrideOpenWorld,
			},
		},
	})

	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	checks := []struct {
		name string
		got  bool
		want bool
	}{
		{"the widening read-only override is dropped", tool.Annotations.ReadOnlyHint, false},
		{"the narrowing destructive override is applied", tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint, false},
		{"the narrowing idempotent override is applied", tool.Annotations.IdempotentHint, false},
		{"the narrowing open-world override is applied", tool.Annotations.OpenWorldHint != nil && *tool.Annotations.OpenWorldHint, false},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if check.got != check.want {
				t.Errorf("annotation = %t, want %t", check.got, check.want)
			}
		})
	}
}

// TestIndividualToolFromActionSpec_LockdownsInputSchema verifies IndividualToolFromActionSpec when lockdowns input schema.
func TestIndividualToolFromActionSpec_LockdownsInputSchema(t *testing.T) {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID,required"},
		},
	}
	spec := NewActionSpec("current", ActionRoute{
		InputSchema:  inputSchema,
		OutputSchema: testActionSpecSchema("id"),
	}, ActionSpecOptions{
		ReadOnly:       true,
		Idempotent:     true,
		OwnerPackage:   "users",
		IndividualTool: IndividualToolSpec{Name: "gitlab_user_current", Description: "Get the current user."},
	})

	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	schema, schemaOK := tool.InputSchema.(map[string]any)
	if !schemaOK {
		t.Fatalf("tool input schema = %T, want map[string]any", tool.InputSchema)
	}
	properties, propertiesOK := schema["properties"].(map[string]any)
	if !propertiesOK {
		t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
	}
	projectID, projectOK := properties["project_id"].(map[string]any)
	if !projectOK {
		t.Fatalf("project_id property = %T, want map[string]any", properties["project_id"])
	}
	if projectID["description"] != "Project ID" {
		t.Fatalf("project_id description = %q, want Project ID", projectID["description"])
	}
	if got, boolOK := schema["additionalProperties"].(bool); !boolOK || got {
		t.Fatalf("schema additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	if _, mutated := spec.Route.InputSchema["additionalProperties"]; mutated {
		t.Fatal("projection mutated the spec input schema")
	}
}

// TestIndividualToolFromActionSpec_PreservesIndividualRequiredFields verifies IndividualToolFromActionSpec preserves individual required fields.
func TestIndividualToolFromActionSpec_PreservesIndividualRequiredFields(t *testing.T) {
	type input struct {
		ProjectID        string `json:"project_id" jsonschema:"Project ID,required"`
		EnvironmentScope string `json:"environment_scope" jsonschema:"Filter by environment scope"`
	}
	route := RouteAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, input) (VoidOutput, error) {
		return VoidOutput{}, nil
	})
	spec := NewActionSpec("get", route, ActionSpecOptions{
		ReadOnly:       true,
		Idempotent:     true,
		OwnerPackage:   "ci_variables",
		IndividualTool: IndividualToolSpec{Name: "gitlab_ci_variable_get", Description: "Get a CI/CD variable."},
	})

	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	schema, schemaOK := tool.InputSchema.(map[string]any)
	if !schemaOK {
		t.Fatalf("tool input schema = %T, want map[string]any", tool.InputSchema)
	}
	required, requiredOK := schema["required"].([]any)
	if !requiredOK {
		t.Fatalf("schema required = %T, want []any", schema["required"])
	}
	for _, field := range []string{"project_id", "environment_scope"} {
		t.Run(field, func(t *testing.T) {
			if !slices.ContainsFunc(required, func(value any) bool { return value == field }) {
				t.Fatalf("required fields = %#v, want %q", required, field)
			}
		})
	}
}

// TestIndividualToolFromSpecs_ProjectsMatchingSpec verifies IndividualToolFromSpecs projects matching spec.
func TestIndividualToolFromSpecs_ProjectsMatchingSpec(t *testing.T) {
	specs := []ActionSpec{
		NewActionSpec("list", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{
			ReadOnly:       true,
			IndividualTool: IndividualToolSpec{Name: "gitlab_project_list", Description: "List projects."},
		}),
		NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{
			ReadOnly:       true,
			IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a project."},
		}),
	}

	tool, err := IndividualToolFromSpecs(specs, "gitlab_project_get", IndividualToolProjectionOptions{})
	if err != nil {
		t.Fatalf("IndividualToolFromSpecs() error = %v", err)
	}
	if tool.Name != "gitlab_project_get" {
		t.Fatalf("tool name = %q, want gitlab_project_get", tool.Name)
	}
}

// TestIndividualToolFromSpecs_RejectsEmptyName verifies individual projection
// rejects empty tool names before scanning specs.
func TestIndividualToolFromSpecs_RejectsEmptyName(t *testing.T) {
	if _, err := IndividualToolFromSpecs(nil, "  ", IndividualToolProjectionOptions{}); err == nil {
		t.Fatal("IndividualToolFromSpecs() empty name error = nil, want error")
	}
}

// TestMustIndividualToolFromSpecs_ProjectsOrPanics verifies the must helper
// returns projected tools and panics on invalid metadata.
func TestMustIndividualToolFromSpecs_ProjectsOrPanics(t *testing.T) {
	specs := []ActionSpec{
		NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{
			ReadOnly:       true,
			IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a project."},
		}),
	}
	tool := MustIndividualToolFromSpecs(specs, "gitlab_project_get", IndividualToolProjectionOptions{})
	if tool.Name != "gitlab_project_get" {
		t.Fatalf("tool name = %q, want gitlab_project_get", tool.Name)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustIndividualToolFromSpecs() did not panic for missing spec")
		}
	}()
	MustIndividualToolFromSpecs(specs, "gitlab_project_missing", IndividualToolProjectionOptions{})
}

// TestIndividualToolFromSpecs_RejectsMissingOrDuplicateSpec verifies IndividualToolFromSpecs rejects missing or duplicate spec.
func TestIndividualToolFromSpecs_RejectsMissingOrDuplicateSpec(t *testing.T) {
	specs := []ActionSpec{
		NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{
			ReadOnly:       true,
			IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a project."},
		}),
		NewActionSpec("show", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{
			ReadOnly:       true,
			IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Show a project."},
		}),
	}

	if _, err := IndividualToolFromSpecs(specs[:1], "gitlab_project_missing", IndividualToolProjectionOptions{}); err == nil {
		t.Fatal("IndividualToolFromSpecs() missing error = nil, want error")
	}
	if _, err := IndividualToolFromSpecs(specs, "gitlab_project_get", IndividualToolProjectionOptions{}); err == nil {
		t.Fatal("IndividualToolFromSpecs() duplicate error = nil, want error")
	}
}

// TestIndividualToolFromActionSpec_RemovesStaleRequired verifies required
// fields are recalculated from the reflected input type.
func TestIndividualToolFromActionSpec_RemovesStaleRequired(t *testing.T) {
	route := RouteFunc(func(_ context.Context, _ optionalIndividualInput) (testOutput, error) {
		return testOutput{}, nil
	})
	route.InputSchema["required"] = []any{"name"}
	spec := NewActionSpec("get", route, ActionSpecOptions{
		ReadOnly:       true,
		IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a project."},
	})

	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("tool input schema = %T, want map[string]any", tool.InputSchema)
	}
	if _, hasRequired := schema["required"]; hasRequired {
		t.Fatalf("schema required = %#v, want removed", schema["required"])
	}
}

// TestIndividualToolFromActionSpec_RejectsIncompleteMetadata covers IndividualToolFromActionSpec with table-driven subtests for rejects incomplete metadata.
func TestIndividualToolFromActionSpec_RejectsIncompleteMetadata(t *testing.T) {
	testCases := []struct {
		name string
		spec ActionSpec
	}{
		{
			name: "invalid action spec",
			spec: ActionSpec{},
		},
		{
			name: "missing individual tool name",
			spec: NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{ReadOnly: true}),
		},
		{
			name: "missing input schema",
			spec: NewActionSpec("get", ActionRoute{OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{ReadOnly: true, IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a GitLab project."}}),
		},
		{
			name: "missing output schema",
			spec: NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id")}, ActionSpecOptions{ReadOnly: true, IndividualTool: IndividualToolSpec{Name: "gitlab_project_get", Description: "Get a GitLab project."}}),
		},
		{
			name: "missing description",
			spec: NewActionSpec("get", ActionRoute{InputSchema: testActionSpecSchema("project_id"), OutputSchema: testActionSpecSchema("id")}, ActionSpecOptions{ReadOnly: true, IndividualTool: IndividualToolSpec{Name: "gitlab_project_get"}}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := IndividualToolFromActionSpec(tc.spec, IndividualToolProjectionOptions{}); err == nil {
				t.Fatal("IndividualToolFromActionSpec() error = nil, want error")
			}
		})
	}
}

// testActionSpecSchema supports test action spec schema assertions in toolutil tests.
func testActionSpecSchema(properties ...string) map[string]any {
	props := make(map[string]any, len(properties))
	for _, name := range properties {
		props[name] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": props}
}

// TestIndividualToolAnnotationOverrides_NarrowingOnly verifies that an
// individual-tool annotation override may make an action look less safe than
// the action is, never safer.
//
// readOnlyHint and idempotentHint are acted on, not merely displayed:
// --read-only removes what is not read-only, safe mode previews what is not,
// and a gateway may auto-allow readOnlyHint:true with no human in the loop. An
// override that raises either therefore widens the operator's controls rather
// than describing the tool, which is how one mutating action came to be
// read-only on the individual surface and mutating on the other two.
func TestIndividualToolAnnotationOverrides_NarrowingOnly(t *testing.T) {
	truth, falsehood := true, false
	tests := []struct {
		name           string
		overrides      IndividualToolAnnotationOverrides
		readOnly       bool
		idempotent     bool
		wantReadOnly   *bool
		wantIdempotent *bool
	}{
		{name: "no overrides"},
		{
			name:         "read-only claim on a mutating action is dropped",
			overrides:    IndividualToolAnnotationOverrides{ReadOnly: &truth},
			wantReadOnly: nil,
		},
		{
			name:         "read-only claim on a read-only action is kept",
			overrides:    IndividualToolAnnotationOverrides{ReadOnly: &truth},
			readOnly:     true,
			wantReadOnly: &truth,
		},
		{
			name:         "a narrowing read-only override is kept",
			overrides:    IndividualToolAnnotationOverrides{ReadOnly: &falsehood},
			readOnly:     true,
			wantReadOnly: &falsehood,
		},
		{
			name:           "idempotent claim on a non-repeatable action is dropped",
			overrides:      IndividualToolAnnotationOverrides{Idempotent: &truth},
			wantIdempotent: nil,
		},
		{
			name:           "idempotent claim on an idempotent action is kept",
			overrides:      IndividualToolAnnotationOverrides{Idempotent: &truth},
			idempotent:     true,
			wantIdempotent: &truth,
		},
		{
			name:           "a narrowing idempotent override is kept",
			overrides:      IndividualToolAnnotationOverrides{Idempotent: &falsehood},
			idempotent:     true,
			wantIdempotent: &falsehood,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.overrides.NarrowingOnly(tt.readOnly, tt.idempotent)
			if !sameOptionalBool(got.ReadOnly, tt.wantReadOnly) {
				t.Errorf("ReadOnly = %s, want %s", optionalBoolString(got.ReadOnly), optionalBoolString(tt.wantReadOnly))
			}
			if !sameOptionalBool(got.Idempotent, tt.wantIdempotent) {
				t.Errorf("Idempotent = %s, want %s", optionalBoolString(got.Idempotent), optionalBoolString(tt.wantIdempotent))
			}
		})
	}

	t.Run("destructive and open-world overrides are untouched", func(t *testing.T) {
		overrides := IndividualToolAnnotationOverrides{Destructive: &falsehood, OpenWorld: &falsehood}
		got := overrides.NarrowingOnly(false, false)
		if !sameOptionalBool(got.Destructive, &falsehood) || !sameOptionalBool(got.OpenWorld, &falsehood) {
			t.Errorf("Destructive/OpenWorld = %s/%s, want false/false", optionalBoolString(got.Destructive), optionalBoolString(got.OpenWorld))
		}
	})
}

// TestAnnotationsFromActionSpec_WideningOverridesDoNotReachTheClient verifies
// the served annotations apply the same rule, which is the only place a client
// or a gateway can see it.
func TestAnnotationsFromActionSpec_WideningOverridesDoNotReachTheClient(t *testing.T) {
	truth := true
	spec := NewActionSpec("hook_test", ActionRoute{
		Handler:     func(context.Context, map[string]any) (any, error) { return struct{}{}, nil },
		InputSchema: map[string]any{"type": "object"},
	}, ActionSpecOptions{
		OwnerPackage: "toolutil",
		IndividualTool: IndividualToolSpec{
			Name: "gitlab_test_hook_test",
			AnnotationOverrides: IndividualToolAnnotationOverrides{
				ReadOnly:   &truth,
				Idempotent: &truth,
			},
		},
	})

	annotations := annotationsFromActionSpec(spec)
	checks := []struct {
		name string
		got  bool
	}{
		{"readOnlyHint", annotations.ReadOnlyHint},
		{"idempotentHint", annotations.IdempotentHint},
	}
	for _, check := range checks {
		t.Run(check.name+" stays false for a mutating action", func(t *testing.T) {
			if check.got {
				t.Errorf("%s = true, want false", check.name)
			}
		})
	}
}

// sameOptionalBool reports whether two optional booleans carry the same value.
func sameOptionalBool(got, want *bool) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

// optionalBoolString renders an optional boolean for a failure message.
func optionalBoolString(value *bool) string {
	if value == nil {
		return "<nil>"
	}
	if *value {
		return "true"
	}
	return "false"
}
