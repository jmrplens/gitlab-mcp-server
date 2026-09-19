// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package on purpose: internal/tools imports this one,
// so only a test outside package epics can import the catalog back and ask it
// whether an ID resolves. Asserting the corrected literal beside the constant
// would prove only that the constant equals itself.
package epics_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epics"
)

// hintedActionID matches the canonical ID inside a next-step hint as
// toolutil.HintAction renders it, which is how the ID reaches a model.
var hintedActionID = regexp.MustCompile(`Use action '([^']+)'`)

// TestPublishedActionIDs_NameActionsTheCatalogHolds asserts that every ID this
// package publishes, as a related action and as a rendered next-step hint,
// resolves to an action the catalog holds.
//
// Epics are routes on the gitlab_group catalog group, so the canonical ID is
// "group.epic_list" and never the bare "epic.list" the domain reads like.
// Every related action here used to spell the bare form, which no surface
// resolves: a model following one was answered "unknown action", while the
// hints rendered beside them already named the routes the catalog holds.
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	specs := epics.ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() is empty, the test asserts nothing")
	}

	type use struct{ where, id string }
	var uses []use
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			uses = append(uses, use{where: "related action of " + spec.Name, id: related})
		}
	}
	for _, rendered := range renderedEpicMarkdown() {
		for _, match := range hintedActionID.FindAllStringSubmatch(rendered.markdown, -1) {
			uses = append(uses, use{where: "hint in " + rendered.name, id: match[1]})
		}
	}
	if len(uses) == 0 {
		t.Fatal("no action IDs collected, the test asserts nothing")
	}

	for _, u := range uses {
		t.Run(u.where+" -> "+u.id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(u.id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action", u.id)
			}
		})
	}
}

// TestEpicSpecs_OwnIDsAreNamespacedUnderTheGroupDomain asserts that each spec
// this package declares is itself reachable under the group domain, which is
// what makes the IDs above resolvable in the first place.
func TestEpicSpecs_OwnIDsAreNamespacedUnderTheGroupDomain(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	for _, spec := range epics.ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			id := actioncatalog.ActionID("group." + spec.Name)
			if _, ok := catalog.Action(id); !ok {
				t.Errorf("spec %q is not registered as %q", spec.Name, id)
			}
		})
	}
}

// TestEpicHints_ReachEveryFormatter guards the collection above: a formatter
// whose fixture is too empty to reach its hints would silently contribute
// nothing, leaving its IDs unchecked.
func TestEpicHints_ReachEveryFormatter(t *testing.T) {
	for _, rendered := range renderedEpicMarkdown() {
		t.Run(rendered.name, func(t *testing.T) {
			if !hintedActionID.MatchString(rendered.markdown) {
				t.Errorf("%s rendered no next-step hint, so its action IDs go unchecked", rendered.name)
			}
		})
	}
}

// renderedEpicMarkdown renders every markdown formatter this package registers
// with a fixture populated far enough to reach the hints it writes at the end.
func renderedEpicMarkdown() []struct{ name, markdown string } {
	one := epics.Output{IID: 7, Title: "Fixture epic", State: "opened"}
	return []struct{ name, markdown string }{
		{"FormatOutputMarkdown", epics.FormatOutputMarkdown(one)},
		{"FormatListMarkdown", epics.FormatListMarkdown(epics.ListOutput{Epics: []epics.Output{one}})},
		{"FormatLinksMarkdown", epics.FormatLinksMarkdown(epics.LinksOutput{
			ChildEpics: []epics.LinksItem{{IID: 8, Title: "Fixture child"}},
		})},
	}
}

// ultimateCatalog builds the canonical catalog at the Ultimate tier, which is
// the only tier that carries the Premium epic actions this package declares.
func ultimateCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if len(catalog.Actions()) == 0 {
		t.Fatal("BuildActionCatalog() returned an empty catalog, every lookup would fail")
	}
	// A catalog that answers every lookup would pass the assertions above
	// without judging anything.
	if _, ok := catalog.Action("group.epic_not_an_action"); ok {
		t.Fatal("catalog resolves an invented ID, the lookups below prove nothing")
	}
	return catalog
}
