// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package because internal/tools imports
// internal/tools/featureflags, so the catalog is only reachable from outside
// the package under test.
package featureflags_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/featureflags"
)

// ultimateCatalog builds the canonical action catalog at the highest tier.
// Feature flags are a Premium/Ultimate surface, so a Free catalog would not
// hold these actions at all and the comparison would be vacuous.
func ultimateCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// TestFeatureFlagActionSpecs_PublishedActionIDs_NameCatalogActions asserts
// that every ID this package invites a model to call next resolves to a real
// catalog action.
//
// What it guards: this package kept two blocks of these constants and they
// drifted. markdown.go had the right "feature_flags.feature_flag_get" while
// action_specs.go had "feature_flag.get", which names no action; a model
// following a RelatedActions entry was answered "unknown action". The one
// spelling that did resolve, "feature_flag.list", resolved only as an alias,
// which is why this test calls Catalog.Action: it resolves canonical IDs only,
// so an alias fails here as loudly as an invention does.
//
// Why it asserts against the catalog rather than the literals: repeating the
// corrected string beside the constant would pass no matter what the catalog
// holds.
func TestFeatureFlagActionSpecs_PublishedActionIDs_NameCatalogActions(t *testing.T) {
	catalog := ultimateCatalog(t)
	for _, id := range featureflags.PublishedActionIDs {
		t.Run(id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action", id)
			}
		})
	}
}

// TestFeatureFlagActionSpecs_RelatedActions_NameCatalogActions asserts the
// same for the RelatedActions actually attached to each spec, which is what
// reaches the served surface. It covers IDs written as literals at the call
// site, such as the "environment.list" and "ci_variable.list" cross-domain
// siblings, as well as the shared constants.
func TestFeatureFlagActionSpecs_RelatedActions_NameCatalogActions(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := featureflags.ActionSpecs(client)
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
