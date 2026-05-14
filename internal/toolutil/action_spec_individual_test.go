package toolutil

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

func testActionSpecSchema(properties ...string) map[string]any {
	props := make(map[string]any, len(properties))
	for _, name := range properties {
		props[name] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": props}
}
