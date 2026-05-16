package actioncatalog

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestGroupFromSpecs_ProjectsSpecMetadata verifies GroupFromSpecs projects spec metadata.
func TestGroupFromSpecs_ProjectsSpecMetadata(t *testing.T) {
	route := toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
	}}
	spec := toolutil.NewActionSpec("get", route, toolutil.ActionSpecOptions{
		Aliases:        []string{"project.show"},
		Tags:           []string{"Project"},
		Usage:          "Use for reading one project.",
		RelatedActions: []string{"project.list"},
		Compatibility: toolutil.CompatibilityPolicy{
			ActionAliases:    []toolutil.ActionAliasSpec{{Alias: "project.view", Target: "get", Source: "dynamic", Reason: "Preserve old prompt phrasing."}},
			ParameterAliases: []toolutil.ParameterAliasSpec{{Alias: "project", Target: "project_id", Source: "dynamic", Reason: "Map shorthand prompts to project_id."}},
		},
		ParameterGuidance:      map[string]toolutil.ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
		ReadOnly:               true,
		Idempotent:             true,
		OpenWorld:              true,
		Edition:                "core",
		OwnerPackage:           "projects",
		IndividualTool:         toolutil.IndividualToolSpec{Name: "gitlab_get_project", Title: "Get project", Description: "Get one GitLab project."},
		ContentKind:            toolutil.ActionSpecContentDetail,
		NotFoundPolicy:         toolutil.ActionSpecNotFoundResult,
		EmbeddedResourcePolicy: toolutil.ActionSpecEmbeddedNone,
		RichResultPolicy:       toolutil.ActionSpecRichStandard,
		SchemaValidationNotes:  []string{"project_id accepts numeric ID or URL-encoded path"},
		RuntimeValidationNotes: []string{"handler converts GitLab API 404 into NotFoundResult"},
	})

	group, err := GroupFromSpecs(GroupOptions{ToolName: "gitlab_project"}, []toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	action := group.Actions["get"]
	if action.ID != "project.get" || action.SchemaURI != "gitlab://schema/meta/gitlab_project/get" {
		t.Fatalf("action identity = %q %q, want normalized project.get schema URI", action.ID, action.SchemaURI)
	}
	if !action.SpecBacked || !action.ReadOnly || action.Edition != "core" || action.OwnerPackage != "projects" {
		t.Fatalf("action metadata = %+v, want projected read-only core project metadata", action)
	}
	if len(action.Aliases) != 1 || action.Aliases[0] != "project.show" {
		t.Fatalf("aliases = %+v, want projected aliases", action.Aliases)
	}
	if len(action.RelatedActions) != 1 || action.RelatedActions[0] != "project.list" {
		t.Fatalf("related actions = %+v, want projected related action", action.RelatedActions)
	}
	if len(action.Compatibility.ActionAliases) != 1 || action.Compatibility.ActionAliases[0].Alias != "project.view" {
		t.Fatalf("compatibility action aliases = %+v, want projected action alias", action.Compatibility.ActionAliases)
	}
	if len(action.Compatibility.ParameterAliases) != 1 || action.Compatibility.ParameterAliases[0].Target != "project_id" {
		t.Fatalf("compatibility parameter aliases = %+v, want projected parameter alias", action.Compatibility.ParameterAliases)
	}
	if action.Route.ParameterGuidance["project_id"].SemanticRole != "scope_project" {
		t.Fatalf("route guidance = %+v, want projected spec guidance", action.Route.ParameterGuidance)
	}
	if len(action.SchemaValidationNotes) != 1 || action.SchemaValidationNotes[0] != "project_id accepts numeric ID or URL-encoded path" {
		t.Fatalf("schema validation notes = %+v, want projected schema note", action.SchemaValidationNotes)
	}
	if len(action.RuntimeValidationNotes) != 1 || action.RuntimeValidationNotes[0] != "handler converts GitLab API 404 into NotFoundResult" {
		t.Fatalf("runtime validation notes = %+v, want projected runtime note", action.RuntimeValidationNotes)
	}
}

// TestActionsFromSpecs_RejectsInvalidSpecs verifies ActionsFromSpecs rejects invalid specs.
func TestActionsFromSpecs_RejectsInvalidSpecs(t *testing.T) {
	if _, err := ActionsFromSpecs([]toolutil.ActionSpec{{Name: ""}}); err == nil {
		t.Fatal("ActionsFromSpecs() error = nil, want invalid spec rejection")
	}
}

// TestGroupFromSpecs_PropagatesSpecProjectionErrors verifies GroupFromSpecs when propagates spec projection errors.
func TestGroupFromSpecs_PropagatesSpecProjectionErrors(t *testing.T) {
	if _, err := GroupFromSpecs(GroupOptions{ToolName: "gitlab_project"}, []toolutil.ActionSpec{{Name: ""}}); err == nil {
		t.Fatal("GroupFromSpecs() error = nil, want invalid spec rejection")
	}
}
