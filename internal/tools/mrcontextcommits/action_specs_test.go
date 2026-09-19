// action_specs_test.go holds the action IDs this package publishes to a model
// against the catalog the server really builds.
//
// It is an external test package on purpose. The oracle is internal/tools,
// which imports this one, so nothing inside the package can see the ID a spec
// name becomes, and the IDs that went wrong here are the ones that reach
// beyond this package's own group: a context commit is a repository commit
// pinned to a review, and the cross-links to the commit history were spelled
// "commit.get" and "commit.list", which read as obvious pairs and are no
// actions at all (commits are routes on gitlab_repository).
package mrcontextcommits_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrcontextcommits"
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
		for _, id := range mrcontextcommits.PublishedActionIDs() {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action; a model following it is answered unknown action", id)
			}
		}
	})

	t.Run("every RelatedActions entry resolves", func(t *testing.T) {
		for _, spec := range mrcontextcommits.ActionSpecs(nil) {
			for _, related := range spec.RelatedActions {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("action %q lists RelatedActions %q, which resolves to no catalog action", spec.Name, related)
				}
			}
		}
	})
}
