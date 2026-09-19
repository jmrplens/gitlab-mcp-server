// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package on purpose: internal/tools imports this one,
// so only a test outside package grouplabels can import the catalog back and
// ask it whether an ID resolves. Asserting the corrected literal beside the
// constant would prove only that the constant equals itself.
package grouplabels_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/grouplabels"
)

// TestRelatedActions_NameActionsTheCatalogHolds asserts that every ID the
// group-label specs publish as a related action resolves to an action the
// catalog holds.
//
// Group labels are routes on the gitlab_group catalog group, so the canonical
// ID is "group.group_label_get" and never the bare "group_label.get" the
// domain reads like. Every related action here used to spell the bare form,
// which no surface resolves: a model following one was answered "unknown
// action". The closest surviving strings were worse than useless, since
// "group_label.create" reads one edit away from "group.create", which creates
// a whole group.
func TestRelatedActions_NameActionsTheCatalogHolds(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	specs := grouplabels.ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() is empty, the test asserts nothing")
	}

	published := 0
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			published++
			t.Run(spec.Name+" -> "+related, func(t *testing.T) {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("related action %q resolves to no catalog action", related)
				}
			})
		}
	}
	if published == 0 {
		t.Fatal("no related actions collected, the test asserts nothing")
	}
}

// TestGroupLabelSpecs_OwnIDsAreNamespacedUnderTheGroupDomain asserts that each
// spec this package declares is itself reachable under the group domain, which
// is what makes the related actions above resolvable in the first place.
func TestGroupLabelSpecs_OwnIDsAreNamespacedUnderTheGroupDomain(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	for _, spec := range grouplabels.ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			id := actioncatalog.ActionID("group." + spec.Name)
			if _, ok := catalog.Action(id); !ok {
				t.Errorf("spec %q is not registered as %q", spec.Name, id)
			}
		})
	}
}

// ultimateCatalog builds the canonical catalog at the Ultimate tier, so no
// action is missing for want of a license rather than for want of an ID.
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
	if _, ok := catalog.Action("group.group_label_not_an_action"); ok {
		t.Fatal("catalog resolves an invented ID, the lookups below prove nothing")
	}
	return catalog
}
