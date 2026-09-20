package actioncatalog

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// projectionTestRoute is the route every projection test starts from: an
// input schema naming project_id, which the parameter guidance and the
// embedded-resource template of the spec both refer to.
func projectionTestRoute() toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
	}}
}

// TestGroupFromSpecs_ProjectsSpecMetadata verifies that every field of an
// ActionSpec reaches the catalog action GroupFromSpecs builds from it, with
// the route carrying the content kind and the embedded-resource template the
// dispatchers read from it.
//
// Each field of the fixture holds a value no other field holds, so a
// projection that reads a neighbor, or drops a field, is seen. The
// projection is a block of plain assignments, which is exactly the shape
// neither the mutation nor the condition gate can report on.
func TestGroupFromSpecs_ProjectsSpecMetadata(t *testing.T) {
	spec := toolutil.NewActionSpec("get", projectionTestRoute(), toolutil.ActionSpecOptions{
		Aliases:        []string{"project.show"},
		Tags:           []string{"project"},
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
		EmbeddedResourcePolicy: toolutil.ActionSpecEmbeddedAlways,
		EmbeddedResource:       "gitlab://project/{project_id}",
		RichResultPolicy:       toolutil.ActionSpecRichResourceLink,
		SchemaValidationNotes:  []string{"project_id accepts numeric ID or URL-encoded path"},
		RuntimeValidationNotes: []string{"handler converts GitLab API 404 into NotFoundResult"},
	})

	group, err := GroupFromSpecs(GroupOptions{ToolName: "gitlab_project"}, []toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	action := group.Actions["get"]
	assertProjectedActionMetadata(t, action)
}

// assertProjectedActionMetadata holds the action GroupFromSpecs projected
// from the fixture in TestGroupFromSpecs_ProjectsSpecMetadata to that
// fixture, one field at a time: its identity and lists here, its policies
// and flags in assertProjectedActionPolicies.
func assertProjectedActionMetadata(t *testing.T, action Action) {
	t.Helper()
	if action.ID != "project.get" || action.SchemaURI != "gitlab://schema/meta/gitlab_project/get" {
		t.Fatalf("action identity = %q %q, want normalized project.get schema URI", action.ID, action.SchemaURI)
	}
	if action.Name != "get" || action.ToolName != "gitlab_project" || action.Domain != "project" {
		t.Errorf("action name/tool/domain = %q/%q/%q, want get/gitlab_project/project", action.Name, action.ToolName, action.Domain)
	}
	if !action.SpecBacked || !action.ReadOnly || action.Edition != "core" || action.OwnerPackage != "projects" {
		t.Errorf("action metadata = %+v, want projected read-only core project metadata", action)
	}
	if got, want := action.Usage, "Use for reading one project."; got != want {
		t.Errorf("usage = %q, want %q", got, want)
	}
	if !slices.Equal(action.Aliases, []string{"project.show"}) {
		t.Errorf("aliases = %+v, want [project.show]", action.Aliases)
	}
	if !slices.Equal(action.Tags, []string{"project"}) {
		t.Errorf("tags = %+v, want [project]", action.Tags)
	}
	if !slices.Equal(action.RelatedActions, []string{"project.list"}) {
		t.Errorf("related actions = %+v, want [project.list]", action.RelatedActions)
	}
	if len(action.Compatibility.ActionAliases) != 1 || action.Compatibility.ActionAliases[0].Alias != "project.view" {
		t.Errorf("compatibility action aliases = %+v, want projected action alias", action.Compatibility.ActionAliases)
	}
	if len(action.Compatibility.ParameterAliases) != 1 || action.Compatibility.ParameterAliases[0].Target != "project_id" {
		t.Errorf("compatibility parameter aliases = %+v, want projected parameter alias", action.Compatibility.ParameterAliases)
	}
	if action.Route.ParameterGuidance["project_id"].SemanticRole != "scope_project" {
		t.Errorf("route guidance = %+v, want projected spec guidance", action.Route.ParameterGuidance)
	}
	wantTool := toolutil.IndividualToolSpec{Name: "gitlab_get_project", Title: "Get project", Description: "Get one GitLab project."}
	if action.IndividualTool != wantTool {
		t.Errorf("individual tool = %+v, want %+v", action.IndividualTool, wantTool)
	}
	assertProjectedActionPolicies(t, action)
}

// assertProjectedActionPolicies holds the policies, the notes and the flags
// of the action projected from the fixture in
// TestGroupFromSpecs_ProjectsSpecMetadata, including the two values the
// route carries for the dispatchers.
func assertProjectedActionPolicies(t *testing.T, action Action) {
	t.Helper()
	assertProjectedString(t, "content kind", action.ContentKind, toolutil.ActionSpecContentDetail)
	assertProjectedString(t, "not-found policy", action.NotFoundPolicy, toolutil.ActionSpecNotFoundResult)
	assertProjectedString(t, "embedded resource policy", action.EmbeddedResourcePolicy, toolutil.ActionSpecEmbeddedAlways)
	assertProjectedString(t, "embedded resource", action.EmbeddedResource, "gitlab://project/{project_id}")
	assertProjectedString(t, "rich result policy", action.RichResultPolicy, toolutil.ActionSpecRichResourceLink)
	assertProjectedString(t, "route content kind", action.Route.ContentKind, toolutil.ActionSpecContentDetail)
	assertProjectedString(t, "route embedded resource", action.Route.EmbeddedResource, "gitlab://project/{project_id}")
	if !slices.Equal(action.SchemaValidationNotes, []string{"project_id accepts numeric ID or URL-encoded path"}) {
		t.Errorf("schema validation notes = %+v, want projected schema note", action.SchemaValidationNotes)
	}
	if !slices.Equal(action.RuntimeValidationNotes, []string{"handler converts GitLab API 404 into NotFoundResult"}) {
		t.Errorf("runtime validation notes = %+v, want projected runtime note", action.RuntimeValidationNotes)
	}
	if action.Destructive || !action.Idempotent || !action.OpenWorld || action.GitLabDotComOnly {
		t.Errorf("flags destructive/idempotent/open-world/gitlab.com-only = %t/%t/%t/%t, want false/true/true/false",
			action.Destructive, action.Idempotent, action.OpenWorld, action.GitLabDotComOnly)
	}
}

// assertProjectedString reports a projected string field that does not carry
// the value the spec gave it.
func assertProjectedString(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

// actionFlags is the set of boolean hints an action carries, compared as one
// value so a test can say which flag it expects set and hold every other
// one to false.
type actionFlags struct {
	ReadOnly, Destructive, Idempotent, OpenWorld, GitLabDotComOnly bool
}

// TestActionsFromSpecs_ProjectsEachFlagOnItsOwn verifies that each boolean
// hint of a spec lands on the same hint of the action, by setting one flag
// at a time and holding the other four to false.
//
// A fixture that sets several flags to true cannot tell a swap of two of
// them apart, which is why the read-only fixture above cannot see whether
// Idempotent and OpenWorld changed places.
func TestActionsFromSpecs_ProjectsEachFlagOnItsOwn(t *testing.T) {
	tests := []struct {
		name string
		opts toolutil.ActionSpecOptions
		want actionFlags
	}{
		{name: "read only", opts: toolutil.ActionSpecOptions{ReadOnly: true}, want: actionFlags{ReadOnly: true}},
		{name: "destructive", opts: toolutil.ActionSpecOptions{Destructive: true}, want: actionFlags{Destructive: true}},
		{name: "idempotent", opts: toolutil.ActionSpecOptions{Idempotent: true}, want: actionFlags{Idempotent: true}},
		{name: "open world", opts: toolutil.ActionSpecOptions{OpenWorld: true}, want: actionFlags{OpenWorld: true}},
		{name: "gitlab.com only", opts: toolutil.ActionSpecOptions{GitLabDotComOnly: true}, want: actionFlags{GitLabDotComOnly: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := toolutil.NewActionSpec("get", projectionTestRoute(), tt.opts)
			actions, err := ActionsFromSpecs([]toolutil.ActionSpec{spec})
			if err != nil {
				t.Fatalf("ActionsFromSpecs() error = %v", err)
			}
			if len(actions) != 1 {
				t.Fatalf("ActionsFromSpecs() returned %d actions, want 1", len(actions))
			}
			action := actions[0]
			got := actionFlags{
				ReadOnly:         action.ReadOnly,
				Destructive:      action.Destructive,
				Idempotent:       action.Idempotent,
				OpenWorld:        action.OpenWorld,
				GitLabDotComOnly: action.GitLabDotComOnly,
			}
			if got != tt.want {
				t.Errorf("projected flags = %+v, want %+v", got, tt.want)
			}
			if action.Route.Destructive != tt.want.Destructive {
				t.Errorf("route destructive = %t, want %t", action.Route.Destructive, tt.want.Destructive)
			}
		})
	}
}

// TestActionsFromSpecs_EmbeddedResourceTravelsOnlyUnderAnEmbeddingPolicy
// verifies that the route the dispatchers read carries the spec's
// embedded-resource template exactly when the spec's policy embeds: under
// "optional" and "always" the route has the template, under "none" and an
// unset policy it has none.
//
// The dispatcher never sees the spec, so a template left off the route is
// a get-style result with no embedded resource, and a template put on the
// route under a policy that never embeds is the opposite defect.
func TestActionsFromSpecs_EmbeddedResourceTravelsOnlyUnderAnEmbeddingPolicy(t *testing.T) {
	const template = "gitlab://project/{project_id}"
	tests := []struct {
		name     string
		policy   string
		template string
		want     string
	}{
		{name: "unset policy", policy: "", template: "", want: ""},
		{name: "none", policy: toolutil.ActionSpecEmbeddedNone, template: "", want: ""},
		{name: "optional", policy: toolutil.ActionSpecEmbeddedOptional, template: template, want: template},
		{name: "always", policy: toolutil.ActionSpecEmbeddedAlways, template: template, want: template},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := toolutil.NewActionSpec("get", projectionTestRoute(), toolutil.ActionSpecOptions{
				ReadOnly:               true,
				EmbeddedResourcePolicy: tt.policy,
				EmbeddedResource:       tt.template,
			})
			actions, err := ActionsFromSpecs([]toolutil.ActionSpec{spec})
			if err != nil {
				t.Fatalf("ActionsFromSpecs() error = %v", err)
			}
			if len(actions) != 1 {
				t.Fatalf("ActionsFromSpecs() returned %d actions, want 1", len(actions))
			}
			if got := actions[0].Route.EmbeddedResource; got != tt.want {
				t.Errorf("route embedded resource under policy %q = %q, want %q", tt.policy, got, tt.want)
			}
			if got := actions[0].EmbeddedResourcePolicy; got != tt.policy {
				t.Errorf("action embedded resource policy = %q, want %q", got, tt.policy)
			}
		})
	}
}

// TestActionsFromSpecs_RejectsInvalidSpecs verifies that a spec without a
// name is refused rather than projected to an action with no identity.
func TestActionsFromSpecs_RejectsInvalidSpecs(t *testing.T) {
	if _, err := ActionsFromSpecs([]toolutil.ActionSpec{{Name: ""}}); err == nil {
		t.Fatal("ActionsFromSpecs() error = nil, want invalid spec rejection")
	}
}

// TestGroupFromSpecs_PropagatesSpecProjectionErrors verifies that
// GroupFromSpecs returns the projection error instead of a group missing
// the spec that failed.
func TestGroupFromSpecs_PropagatesSpecProjectionErrors(t *testing.T) {
	if _, err := GroupFromSpecs(GroupOptions{ToolName: "gitlab_project"}, []toolutil.ActionSpec{{Name: ""}}); err == nil {
		t.Fatal("GroupFromSpecs() error = nil, want invalid spec rejection")
	}
}

// TestActionsFromSpecs_WhitespaceNameNotInRoutes covers the
// "if !ok { errs = append(errs, ...) }" branch in ActionsFromSpecs.
// ActionSpecsToMapWithError normalizes spec.Name with strings.TrimSpace
// before storing it in the routes map, but the ActionsFromSpecs loop
// looks up the raw spec.Name. A trailing space therefore produces a
// successful lower-level projection yet leaves routes[spec.Name] unset,
// so the function must collect an error explaining that the spec was
// not projected to a route. The test asserts the spec is still
// considered invalid rather than silently dropped.
func TestActionsFromSpecs_WhitespaceNameNotInRoutes(t *testing.T) {
	spec := toolutil.NewActionSpec("get", toolutil.ActionRoute{
		InputSchema: map[string]any{"type": "object"},
	}, toolutil.ActionSpecOptions{ReadOnly: true, Idempotent: true})
	// Inject a trailing space so the raw name differs from the trimmed
	// key that ActionSpecsToMapWithError uses to index the routes map.
	spec.Name = "get "

	actions, err := ActionsFromSpecs([]toolutil.ActionSpec{spec})
	if err == nil {
		t.Fatal("ActionsFromSpecs() error = nil, want route-projection error for whitespace name")
	}
	if !strings.Contains(err.Error(), "get") || !strings.Contains(err.Error(), "not projected") {
		t.Fatalf("err = %q, want it to mention spec name and 'not projected'", err.Error())
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %+v, want empty slice on route-projection failure", actions)
	}
}

// TestActionsFromSpecs_DuplicateNames_RefusedAtProjection pins the property
// that lets the ActionsFromSpecs loop carry no duplicate guard of its own:
// two specs sharing a name are refused by ActionSpecsToMapWithError, by
// their trimmed names, and that refusal returns before the loop runs.
//
// The loop used to keep a "seen" set over the raw names anyway. Nothing
// could reach it, since two raw names that are equal trim equal, and a guard
// no input can observe is code a reader has to reason about for nothing, so
// it is gone and this test is what holds the reason it could go.
func TestActionsFromSpecs_DuplicateNames_RefusedAtProjection(t *testing.T) {
	makeSpec := func(name string) toolutil.ActionSpec {
		return toolutil.NewActionSpec(name, toolutil.ActionRoute{
			InputSchema: map[string]any{"type": "object"},
		}, toolutil.ActionSpecOptions{ReadOnly: true, Idempotent: true})
	}
	tests := []struct {
		name  string
		specs []toolutil.ActionSpec
	}{
		{name: "same raw name", specs: []toolutil.ActionSpec{makeSpec("dup"), makeSpec("dup")}},
		{name: "same trimmed name", specs: func() []toolutil.ActionSpec {
			padded := makeSpec("dup")
			padded.Name = "dup "
			return []toolutil.ActionSpec{makeSpec("dup"), padded}
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := ActionsFromSpecs(tt.specs)
			if err == nil {
				t.Fatal("ActionsFromSpecs() error = nil, want the duplicate refused at the projection step")
			}
			if !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("err = %q, want it to mention 'duplicate'", err.Error())
			}
			if len(actions) != 0 {
				t.Fatalf("actions = %+v, want none when the projection refused", actions)
			}
		})
	}
}
