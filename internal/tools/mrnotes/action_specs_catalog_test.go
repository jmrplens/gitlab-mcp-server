// action_specs_catalog_test.go holds the action IDs this package publishes to
// a model against the catalog the server really builds.
//
// It is an external test package on purpose. The oracle is internal/tools,
// which imports this one, so nothing inside the package can see the ID a spec
// name becomes: merge request notes, discussions and draft notes are routes on
// gitlab_mr_review while the merge request itself is on
// gitlab_merge_request, and this package used to spell every one of them
// "merge_request.note_list" and the like. Each of those has a near-miss that
// does resolve and reads the wrong thing, so the two blocks that disagreed
// were each plausible on their own and only the catalog can say which is real.
package mrnotes_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrnotes"
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
		for _, id := range mrnotes.PublishedActionIDs() {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action; a model following it is answered unknown action", id)
			}
		}
	})

	t.Run("every RelatedActions entry resolves", func(t *testing.T) {
		for _, spec := range mrnotes.ActionSpecs(nil) {
			for _, related := range spec.RelatedActions {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("action %q lists RelatedActions %q, which resolves to no catalog action", spec.Name, related)
				}
			}
		}
	})
}
