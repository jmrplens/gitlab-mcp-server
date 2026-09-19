// action_specs_catalog_test.go holds this package's published action IDs
// against the catalog that has to resolve them.
//
// It is an external test package because the catalog is assembled by
// internal/tools, which imports this one: only a _test package may import back
// the other way. The IDs are read off the published surface rather than
// restated as literals beside the constants, since a test that copies a
// constant proves only that it was copied twice.
package uploads_test

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/uploads"
)

// catalogIDs returns the canonical action IDs the Ultimate catalog holds.
// Ultimate is the widest tier, so an ID gated above Free is present rather
// than reported missing, and the client is nil because catalog construction
// registers handlers and issues no request.
func catalogIDs(t *testing.T) map[string]struct{} {
	t.Helper()

	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	ids := make(map[string]struct{}, len(catalog.Actions()))
	for _, action := range catalog.Actions() {
		ids[string(action.ID)] = struct{}{}
	}
	return ids
}

// TestActionSpecs_EveryRelatedActionID_ResolvesInTheCatalog verifies that every
// canonical ID this package offers a model as a next step names an action the
// catalog really holds.
//
// What it guards: one set of constants named both the spec names and the
// related actions, so the related lists published bare names with no domain at
// all (upload, upload_list, upload_delete) and a model following one was
// answered "unknown action". The spec names and the IDs are separate constants
// now, and this is what keeps the derivation honest.
func TestActionSpecs_EveryRelatedActionID_ResolvesInTheCatalog(t *testing.T) {
	t.Parallel()

	ids := catalogIDs(t)
	specs := uploads.ActionSpecs(nil)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() returned no specs, so nothing below was asserted")
	}
	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			t.Parallel()

			if len(spec.RelatedActions) == 0 {
				t.Fatal("spec publishes no related actions, so nothing below was asserted")
			}
			for _, related := range spec.RelatedActions {
				if _, ok := ids[related]; !ok {
					t.Errorf("RelatedActions names %q, which the catalog does not hold", related)
				}
			}
		})
	}
}

// TestActionSpecs_EverySpecName_ReachesTheCatalog verifies the other half of
// the same join: each spec this package declares is projected into the catalog
// under some ID, which is what makes the project.* prefix the related lists
// carry a read of the catalog rather than a guess at it.
func TestActionSpecs_EverySpecName_ReachesTheCatalog(t *testing.T) {
	t.Parallel()

	ids := catalogIDs(t)
	actions := make(map[string]struct{}, len(ids))
	for id := range ids {
		if _, action, found := strings.Cut(id, "."); found {
			actions[action] = struct{}{}
		}
	}
	for _, spec := range uploads.ActionSpecs(nil) {
		if _, ok := actions[spec.Name]; !ok {
			t.Errorf("spec %q reaches the catalog under no action ID", spec.Name)
		}
	}
}
