// action_specs_catalog_test.go holds the action IDs this package publishes to
// a model against the catalog the server really builds.
//
// It is an external test package on purpose. The oracle is internal/tools,
// which imports this one, so nothing inside the package can see the ID its
// spec names become: the catalog group a package's actions are projected into
// is decided by the aggregation in internal/tools/action_specs.go, and it is
// the owner package's own name that reads as the plausible domain and is
// wrong. These three actions are routes on gitlab_group, and this package used
// to cross-link them as bare "ssh_cert_list", "ssh_cert_create" and
// "ssh_cert_delete", which resolve to nothing at all.
package groupsshcerts_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupsshcerts"
)

// TestPublishedActionIDs_ResolveInTheActionCatalog fails when this package
// invites a model to call an action the catalog does not hold.
//
// Two halves, because a published ID reaches a model by two routes and a test
// over either alone has been green while the other was wrong: the constant
// block covers the hints the Markdown formatters write as well as the
// cross-links, and the pass over ActionSpecs catches a literal written at a
// call site that never went through the block.
func TestPublishedActionIDs_ResolveInTheActionCatalog(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("every ID in the canonical block resolves", func(t *testing.T) {
		for _, id := range groupsshcerts.PublishedActionIDs() {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action; a model following it is answered unknown action", id)
			}
		}
	})

	t.Run("every RelatedActions entry resolves", func(t *testing.T) {
		for _, spec := range groupsshcerts.ActionSpecs(nil) {
			for _, related := range spec.RelatedActions {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("action %q lists RelatedActions %q, which resolves to no catalog action", spec.Name, related)
				}
			}
		}
	})
}
