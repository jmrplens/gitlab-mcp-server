package deploymentmergerequests_test

import (
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deploymentmergerequests"
)

// ultimateCatalog builds the canonical action catalog once per test binary, at
// the Ultimate tier so no action is missing for being licensed, with a nil
// client the way the catalog's own tests build it. It is the oracle these tests
// judge against: a published ID is right when the catalog holds an action of
// that name, and asserting anything else (a literal repeated beside the
// constant, say) would only prove the constant equals itself.
var ultimateCatalog = sync.OnceValues(func() (*actioncatalog.Catalog, error) {
	return gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
})

// catalogForTest returns the shared catalog, failing the test if it could not
// be built.
func catalogForTest(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := ultimateCatalog()
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// TestPublishedActionIDs_NameActionsTheCatalogHolds verifies that every
// canonical action ID this package hands a model as a cross-link resolves to an
// action the catalog really holds.
//
// Why: these IDs are an invitation to call something next, and one that
// resolves to nothing is answered "unknown action". What a model concludes from
// that is that the capability is missing, not that the cross-link is wrong. The
// spec here published "mergerequest.get" and "mergerequest.list" while the
// formatter beside it named merge_request.get correctly, which is the drift the
// shared constant block now prevents.
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	t.Parallel()

	catalog := catalogForTest(t)
	ids := deploymentmergerequests.PublishedActionIDs()
	if len(ids) == 0 {
		t.Fatal("publishedActionIDs() is empty, so this test would assert nothing")
	}
	for _, id := range ids {
		if _, found := catalog.Action(actioncatalog.ActionID(id)); !found {
			t.Errorf("published action ID %q names no action the catalog holds", id)
		}
	}
}

// TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds verifies the same
// for the RelatedActions every spec actually carries, which is what the dynamic
// find and execute results publish.
//
// This is deliberately not the same assertion as the one above. That one reads
// the shared constant block; this one reads the specs as they are built, so a
// raw string literal written straight into a RelatedActions list, bypassing the
// block, is caught rather than silently exempted.
func TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds(t *testing.T) {
	t.Parallel()

	catalog := catalogForTest(t)
	specs := deploymentmergerequests.ActionSpecs(nil)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() is empty, so this test would assert nothing")
	}
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			if _, found := catalog.Action(actioncatalog.ActionID(related)); !found {
				t.Errorf("action %q relates to %q, which names no action the catalog holds", spec.Name, related)
			}
		}
	}
}
