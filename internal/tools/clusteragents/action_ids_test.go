package clusteragents_test

import (
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/clusteragents"
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
// IDs in this package used to name "deployment.list", which reads as a
// perfectly plausible pair and is not one: the deployment actions are
// aggregated into the environment group.
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	t.Parallel()

	catalog := catalogForTest(t)
	ids := clusteragents.PublishedActionIDs()
	if len(ids) == 0 {
		t.Fatal("publishedActionIDs() is empty, so this test would assert nothing")
	}
	for _, id := range ids {
		if _, found := catalog.Action(actioncatalog.ActionID(id)); !found {
			t.Errorf("published action ID %q names no action the catalog holds", id)
		}
	}
}

// The RelatedActions half of this file went with the specs: this package used
// to declare a full set of its own that nothing aggregated, and the specs the
// gitlab_admin group really serves are declared in internal/tools/adminspecs,
// where their related actions are held to the catalog beside them. What stays
// here is the constant block the Markdown formatters read, which is published
// to a model whatever surface is serving.
