// action_ids_test.go holds this package against the catalog: every canonical
// action ID it publishes to a model, in an ActionSpec's RelatedActions and in
// the hints its Markdown writes, must name an action the catalog really builds.
//
// It is an external test package (projects_test) on purpose. The oracle is the
// catalog, which lives in internal/tools and imports this package, so only a
// test binary outside the package under test can hold one against the other; an
// in-package test file would be an import cycle. Asserting the corrected
// literal beside the constant would prove nothing, since both halves would be
// the same mistake written twice.
package projects_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/hints"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

const (
	// ownerPackage is the OwnerPackage every spec of this domain declares, and
	// so the key its actions are found under in the catalog.
	ownerPackage = "projects"
	// outputPkgPath is where this domain's output types are declared, which is
	// how the registered Markdown formatters of this package are told from the
	// several hundred registered by the others.
	outputPkgPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
)

// ultimateCatalog builds the canonical catalog at the highest tier, since the
// push-rule and mirror actions this package publishes are Premium and would be
// absent from a Free build, where a reference to one would read as an ID that
// resolves to nothing.
func ultimateCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// assertResolves fails when id names no action of the catalog, naming where the
// ID was published so the failure points at the line to fix.
func assertResolves(t *testing.T, catalog *actioncatalog.Catalog, site, id string) {
	t.Helper()
	if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
		t.Errorf("%s publishes action ID %q, which the catalog does not hold", site, id)
	}
}

// TestProjectRelatedActions_EveryPublishedID_ResolvesInTheCatalog walks the
// RelatedActions of every project action as the catalog itself materialized
// them, and checks each names an action a model could go on to call.
func TestProjectRelatedActions_EveryPublishedID_ResolvesInTheCatalog(t *testing.T) {
	catalog := ultimateCatalog(t)

	var owned int
	for _, action := range catalog.Actions() {
		if action.OwnerPackage != ownerPackage {
			continue
		}
		owned++
		for _, related := range action.RelatedActions {
			assertResolves(t, catalog, string(action.ID)+" RelatedActions", related)
		}
	}
	if owned == 0 {
		t.Fatalf("the catalog holds no action owned by %q; the join this test rests on is broken", ownerPackage)
	}
}

// TestProjectHints_EveryPublishedID_ResolvesInTheCatalog renders every Markdown
// formatter this package registers with a populated fixture and checks each
// action ID the guidance invites a model to call.
//
// The formatters are driven rather than read, because a hint spelled inline at
// the call site is published just like one behind a constant, and most of this
// package's are spelled inline.
func TestProjectHints_EveryPublishedID_ResolvesInTheCatalog(t *testing.T) {
	catalog := ultimateCatalog(t)

	hinted := hints.HintedActionIDs(t, outputPkgPath)
	if len(hinted) == 0 {
		t.Fatalf("no formatter of %s wrote a hint; the fixture no longer reaches the guidance block", outputPkgPath)
	}
	for outputType, ids := range hinted {
		t.Run(outputType, func(t *testing.T) {
			for _, id := range ids {
				assertResolves(t, catalog, outputType, id)
			}
		})
	}
}
