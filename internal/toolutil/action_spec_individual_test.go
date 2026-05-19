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

// TestIndividualToolFromActionSpec_AppliesAnnotationOverrides verifies IndividualToolFromActionSpec applies annotation overrides.
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
	if !tool.Annotations.ReadOnlyHint {
		t.Fatal("read-only annotation = false, want override true")
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Fatalf("destructive annotation = %v, want override false", tool.Annotations.DestructiveHint)
	}
	if tool.Annotations.IdempotentHint {
		t.Fatal("idempotent annotation = true, want override false")
	}
	if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
		t.Fatalf("open-world annotation = %v, want override false", tool.Annotations.OpenWorldHint)
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
		if !slices.ContainsFunc(required, func(value any) bool { return value == field }) {
			t.Fatalf("required fields = %#v, want %q", required, field)
		}
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
