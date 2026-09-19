// action_specs_test.go holds the CI/CD Catalog action IDs this package
// publishes against the catalog the server really builds. It is an external
// test package because internal/tools imports this one, so only a package
// outside it may import the catalog back.
package cicatalog_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/cicatalog"
)

// catalogActionIDs builds the Ultimate action catalog and returns the set of
// canonical IDs it holds. Ultimate is the widest tier, so an ID gated above
// Free is present rather than reported missing; no request is made while the
// catalog is assembled, which is why any handler will do.
func catalogActionIDs(t *testing.T) map[string]struct{} {
	t.Helper()

	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog: %v", err)
	}
	ids := make(map[string]struct{})
	for _, action := range catalog.Actions() {
		ids[string(action.ID)] = struct{}{}
	}
	if len(ids) == 0 {
		t.Fatal("the catalog holds no actions, so nothing below would be judged")
	}
	return ids
}

// TestCICatalogActionIDs_EveryPublishedID_NamesACatalogAction asserts that
// every ID in the shared block resolves to an action the catalog registers.
//
// It compares against the catalog rather than against the literal beside the
// constant on purpose: a test that repeats the string proves only that the
// string was copied twice. These specs are aggregated into the
// gitlab_ci_catalog group, so their domain is "ci_catalog"; the "cicatalog."
// spelling the metadata carried before named the owner package instead.
func TestCICatalogActionIDs_EveryPublishedID_NamesACatalogAction(t *testing.T) {
	ids := catalogActionIDs(t)
	for _, id := range cicatalog.PublishedActionIDs() {
		t.Run(id, func(t *testing.T) {
			if _, ok := ids[id]; !ok {
				t.Errorf("published action ID %q names no action in the catalog", id)
			}
		})
	}
}

// TestCICatalogSpecs_EveryRelatedAction_NamesACatalogAction asserts the same of
// the RelatedActions each spec actually carries, which is what a model reads.
// The block above is where they come from today; this catches a literal
// written straight into the metadata tomorrow.
func TestCICatalogSpecs_EveryRelatedAction_NamesACatalogAction(t *testing.T) {
	ids := catalogActionIDs(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	for _, spec := range cicatalog.ActionSpecs(client) {
		for _, related := range spec.RelatedActions {
			t.Run(spec.Name+"/"+related, func(t *testing.T) {
				if _, ok := ids[related]; !ok {
					t.Errorf("action %q cross-links %q, which names no action in the catalog", spec.Name, related)
				}
			})
		}
	}
}
