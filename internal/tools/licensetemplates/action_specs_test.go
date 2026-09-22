// action_specs_test.go holds the published-ID assertions for the license
// template actions.
//
// The test package is external on purpose. The oracle for "does this ID exist"
// is the canonical catalog, which only [tools.BuildActionCatalog] builds, and
// internal/tools imports this package: an in-package test could not reach the
// catalog without an import cycle, so it could only compare the constants
// against a domain prefix written out beside them. That is the shape the
// defect hid in, since the prefix was itself what was wrong.
package licensetemplates_test

import (
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestPublishedActionIDs_NameActionsTheCatalogHolds holds every canonical ID
// this package offers a model to the catalog that has to resolve it.
//
// The IDs are the RelatedActions of each ActionSpec, which is the whole of
// what this package publishes as an ID: its Markdown formatters do render
// hints, but each names an individual tool in prose rather than composing a
// canonical ID through toolutil.HintAction, so none reaches this set. The
// sibling cross-links named "licensetemplates" as the domain, where the
// catalog aggregates both actions into the shared template group
// ("template.license_get"), so a model following one was answered
// "unknown action".
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	type publishedID struct{ where, id string }
	var published []publishedID
	for _, spec := range licensetemplates.ActionSpecs(client) {
		for _, related := range spec.RelatedActions {
			published = append(published, publishedID{where: "related action of " + spec.Name, id: related})
		}
	}

	// A silent collection would make every assertion below vacuous, which is
	// the way this test could rot into proving nothing.
	if len(published) == 0 {
		t.Fatal("collected no published action IDs, so the test asserts nothing")
	}

	for _, p := range published {
		t.Run(p.where+" "+p.id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(p.id)); !ok {
				t.Errorf("published action ID %q resolves to no action in the catalog", p.id)
			}
		})
	}
}

// TestActionSpecs_EachActionPublishesItsOwnMetadata holds the canonical name,
// the usage sentence, the aliases, the description, the parameter guidance and
// the cross-links of each license action to that action and not to its sibling.
//
// One function builds both specs and branches on the action name, so the list's
// sentence landing on the get spec, or a cross-link naming the very action that
// publishes it, is a straight substitution: the ID still resolves, so the test
// above stays green; the field is still non-empty, so the metadata test stays
// green; and neither is a branch, so neither gate has anything to flip. What a
// model gets is a cross-link back to where it already is, and a usage line
// describing the other tool.
//
// Every one of those fields is compared whole rather than probed, because each
// of them is a place where one branch's value satisfies the other branch's
// shape. The two alias lists are four license phrases each, and a list moved
// wholesale to the other action is caught outside this package only by the
// catalog's duplicate-alias validator, and only when the move is one-sided.
// The two descriptions both carry "Returns:" and "See also:" and each already
// names the sibling tool, so an exchange leaves every reference valid. And
// within one action's guidance map the three entries are interchangeable
// structs of the same shape: exchange project and fullname and the guidance a
// model reads for project describes the copyright holder, so it fills the
// project placeholder with a person's name.
func TestActionSpecs_EachActionPublishesItsOwnMetadata(t *testing.T) {
	const (
		listID = "template.license_list"
		getID  = "template.license_get"
	)
	want := map[string]licenseTemplateExpectation{
		"gitlab_list_license_templates": {
			name:    "license_list",
			id:      listID,
			usage:   "List available license templates with optional popular filter, ordering, and keyset pagination.",
			related: []string{getID, "repository.file_create", "project.create"},
			aliases: []string{
				"gitlab_list_license_templates",
				"list license templates",
				"show available open-source licenses",
				"browse mit/apache/gpl templates",
			},
			description: "List available license templates with an optional popular filter, order_by/sort, and offset or keyset pagination. Returns: each template's key, name, nickname, featured flag, source/HTML URLs, description, conditions, permissions, limitations, and content, with pagination metadata. See also: gitlab_get_license_template, gitlab_file_create.",
			guidance: map[string]toolutil.ParameterGuidance{
				"popular":  {SemanticRole: "popular_filter", ValueSource: "Set true to restrict the listing to popular/featured license templates.", ExampleBinding: `params.popular:true`},
				"order_by": {SemanticRole: "order_by", ValueSource: "Column by which to order keyset-paginated results.", ExampleBinding: `params.order_by:"name"`},
				"sort":     {SemanticRole: "sort", ValueSource: "Sort direction for ordered results.", ExampleBinding: `params.sort:"asc"`},
			},
		},
		"gitlab_get_license_template": {
			name:    "license_get",
			id:      getID,
			usage:   "Get one license template by key for project README/LICENSE scaffolding.",
			related: []string{listID, "repository.file_create", "project.create"},
			aliases: []string{
				"gitlab_get_license_template",
				"get license template by key",
				"render license file for a project",
				"fetch mit/apache license text",
			},
			description: "Get a single license template by key, optionally substituting project and fullname placeholders. Returns: the license's key, name, nickname, featured flag, source/HTML URLs, description, conditions, permissions, limitations, and rendered content. See also: gitlab_list_license_templates, gitlab_file_create.",
			guidance: map[string]toolutil.ParameterGuidance{
				"key":      {SemanticRole: "template_key", ValueSource: "License key returned by license template list output.", ExampleBinding: `params.key:"mit"`},
				"project":  {SemanticRole: "placeholder_project", ValueSource: "Project name substituted into the rendered license content placeholder.", ExampleBinding: `params.project:"my-project"`},
				"fullname": {SemanticRole: "placeholder_fullname", ValueSource: "Full name substituted into the rendered license copyright placeholder.", ExampleBinding: `params.fullname:"Jane Doe"`},
			},
		},
	}

	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := licensetemplates.ActionSpecs(client)
	if len(specs) != len(want) {
		t.Fatalf("len(ActionSpecs) = %d, want %d", len(specs), len(want))
	}

	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			expected, ok := want[spec.IndividualTool.Name]
			if !ok {
				t.Fatalf("unexpected individual tool %q", spec.IndividualTool.Name)
			}
			assertLicenseTemplateSpec(t, spec, expected)
		})
	}
}

// licenseTemplateExpectation is everything one action publishes that a model
// reads, named so the table above and the comparison below are one shape.
type licenseTemplateExpectation struct {
	name        string
	id          string
	usage       string
	related     []string
	aliases     []string
	description string
	guidance    map[string]toolutil.ParameterGuidance
}

// assertLicenseTemplateSpec holds one spec to everything the table says it
// publishes. Each field is compared whole rather than probed, because each is a
// place where one action's value satisfies the other action's shape.
func assertLicenseTemplateSpec(t *testing.T, spec toolutil.ActionSpec, expected licenseTemplateExpectation) {
	t.Helper()
	if spec.Name != expected.name {
		t.Errorf("Name = %q, want %q", spec.Name, expected.name)
	}
	if spec.Usage != expected.usage {
		t.Errorf("Usage = %q, want %q", spec.Usage, expected.usage)
	}
	if !slices.Equal(spec.RelatedActions, expected.related) {
		t.Errorf("RelatedActions = %q, want %q", spec.RelatedActions, expected.related)
	}
	if slices.Contains(spec.RelatedActions, expected.id) {
		t.Errorf("RelatedActions = %q, which offers a model %q, the action it is already reading", spec.RelatedActions, expected.id)
	}
	if !slices.Equal(spec.Aliases, expected.aliases) {
		t.Errorf("Aliases = %q, want %q", spec.Aliases, expected.aliases)
	}
	if spec.IndividualTool.Description != expected.description {
		t.Errorf("IndividualTool.Description =\n got %q\nwant %q", spec.IndividualTool.Description, expected.description)
	}
	if !reflect.DeepEqual(spec.ParameterGuidance, expected.guidance) {
		t.Errorf("ParameterGuidance =\n got %#v\nwant %#v", spec.ParameterGuidance, expected.guidance)
	}
}
