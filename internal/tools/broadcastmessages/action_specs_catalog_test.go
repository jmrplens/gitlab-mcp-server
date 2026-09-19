// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package because internal/tools imports
// internal/tools/broadcastmessages, so the catalog is only reachable from
// outside the package under test.
package broadcastmessages_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/broadcastmessages"
)

// ultimateCatalog builds the canonical action catalog at the highest tier, so
// no action a published ID could name is filtered out before the comparison.
func ultimateCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// TestBroadcastMessageActionSpecs_PublishedActionIDs_NameCatalogActions
// asserts that every ID this package invites a model to call next resolves to
// a real catalog action.
//
// What it guards: the appearance sibling was published as
// "appearance.appearance_get", a plausible-looking pair that names nothing.
// Appearance is served from the gitlab_admin catalog group rather than a group
// of its own, so the real ID is "admin.appearance_get", and a model following
// the old spelling was answered "unknown action".
//
// Why it asserts against the catalog rather than the literals: repeating the
// corrected string beside the constant would pass no matter what the catalog
// holds. Catalog.Action resolves canonical IDs only, never aliases, so an ID
// that is merely some action's alias fails here too.
func TestBroadcastMessageActionSpecs_PublishedActionIDs_NameCatalogActions(t *testing.T) {
	catalog := ultimateCatalog(t)
	for _, id := range broadcastmessages.PublishedActionIDs {
		t.Run(id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action", id)
			}
		})
	}
}

// TestBroadcastMessageActionSpecs_RelatedActions_NameCatalogActions asserts
// the same for the RelatedActions actually attached to each spec, which is
// what reaches the served surface. It covers IDs written as literals at the
// call site as well as the shared constants, so a future spec that names a
// sibling inline is held to the catalog too.
func TestBroadcastMessageActionSpecs_RelatedActions_NameCatalogActions(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := broadcastmessages.ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() returned no specs")
	}
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			t.Run(spec.Name+"/"+related, func(t *testing.T) {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("%s: RelatedActions names %q, which resolves to no catalog action", spec.Name, related)
				}
			})
		}
	}
}
