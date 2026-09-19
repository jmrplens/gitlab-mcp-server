// action_specs_test.go holds the dependency proxy action IDs this package
// publishes against the catalog the server really builds. It is an external
// test package because internal/tools imports this one, so only a package
// outside it may import the catalog back.
package dependencyproxy_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dependencyproxy"
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

// TestDependencyProxyActionIDs_EveryPublishedID_NamesACatalogAction asserts
// that every ID this package cross-links resolves to an action the catalog
// registers.
//
// It compares against the catalog rather than against the literal beside the
// constant on purpose: a test that repeats the string proves only that the
// string was copied twice. The list carried "project.package_registry_list"
// before, which resolves to nothing at all.
func TestDependencyProxyActionIDs_EveryPublishedID_NamesACatalogAction(t *testing.T) {
	ids := catalogActionIDs(t)
	for _, id := range dependencyproxy.PublishedActionIDs() {
		t.Run(id, func(t *testing.T) {
			if _, ok := ids[id]; !ok {
				t.Errorf("published action ID %q names no action in the catalog", id)
			}
		})
	}
}

// TestDependencyProxySpecs_EveryRelatedAction_NamesACatalogAction asserts the
// same of the RelatedActions the spec actually carries. The block above is
// where they come from today; this catches a literal written straight into the
// metadata tomorrow.
func TestDependencyProxySpecs_EveryRelatedAction_NamesACatalogAction(t *testing.T) {
	ids := catalogActionIDs(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	for _, spec := range dependencyproxy.ActionSpecs(client) {
		for _, related := range spec.RelatedActions {
			t.Run(spec.Name+"/"+related, func(t *testing.T) {
				if _, ok := ids[related]; !ok {
					t.Errorf("action %q cross-links %q, which names no action in the catalog", spec.Name, related)
				}
			})
		}
	}
}
