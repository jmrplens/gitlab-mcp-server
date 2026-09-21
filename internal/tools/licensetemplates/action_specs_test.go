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
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/licensetemplates"
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
// the usage sentence and the cross-links of each license action to that action
// and not to its sibling.
//
// One function builds both specs and branches on the action name, so the list's
// sentence landing on the get spec, or a cross-link naming the very action that
// publishes it, is a straight substitution: the ID still resolves, so the test
// above stays green; the field is still non-empty, so the metadata test stays
// green; and neither is a branch, so neither gate has anything to flip. What a
// model gets is a cross-link back to where it already is, and a usage line
// describing the other tool.
func TestActionSpecs_EachActionPublishesItsOwnMetadata(t *testing.T) {
	const (
		listID = "template.license_list"
		getID  = "template.license_get"
	)
	want := map[string]struct {
		name    string
		id      string
		usage   string
		related []string
	}{
		"gitlab_list_license_templates": {
			name:    "license_list",
			id:      listID,
			usage:   "List available license templates with optional popular filter, ordering, and keyset pagination.",
			related: []string{getID, "repository.file_create", "project.create"},
		},
		"gitlab_get_license_template": {
			name:    "license_get",
			id:      getID,
			usage:   "Get one license template by key for project README/LICENSE scaffolding.",
			related: []string{listID, "repository.file_create", "project.create"},
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
		})
	}
}
