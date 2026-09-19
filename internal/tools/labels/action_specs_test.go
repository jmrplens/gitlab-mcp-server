// action_specs_test.go holds the action IDs this package publishes to a model
// against the catalog the server really builds.
//
// It is an external test package on purpose. The oracle is internal/tools,
// which imports this one, so nothing inside the package can see the ID a spec
// name becomes: project labels are routes on the gitlab_project catalog group,
// so label_get is project.label_get and there is no "label" domain. Every one
// of this package's eight cross-linked labels actions used to be spelled under
// that domain, which is what the owner package's own name suggests and what
// the catalog does not hold.
package labels_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/labels"
)

// TestPublishedActionIDs_ResolveInTheActionCatalog fails when this package
// invites a model to call an action the catalog does not hold.
//
// Two halves: the constant block is the set the package reads, and the pass
// over ActionSpecs catches a literal written at a call site that never went
// through the block.
func TestPublishedActionIDs_ResolveInTheActionCatalog(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("every ID in the canonical block resolves", func(t *testing.T) {
		for _, id := range labels.PublishedActionIDs() {
			if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action; a model following it is answered unknown action", id)
			}
		}
	})

	t.Run("every RelatedActions entry resolves", func(t *testing.T) {
		for _, spec := range labels.ActionSpecs(nil) {
			for _, related := range spec.RelatedActions {
				if _, ok := catalog.Action(actioncatalog.ActionID(related)); !ok {
					t.Errorf("action %q lists RelatedActions %q, which resolves to no catalog action", spec.Name, related)
				}
			}
		}
	})
}
