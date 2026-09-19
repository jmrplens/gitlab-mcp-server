// action_specs_test.go holds the published-ID assertions for the .gitignore
// template actions.
//
// The test package is external on purpose. The oracle for "does this ID exist"
// is the canonical catalog, which only [tools.BuildActionCatalog] builds, and
// internal/tools imports this package: an in-package test could not reach the
// catalog without an import cycle, so it could only compare the constants
// against a domain prefix written out beside them. That is the shape the
// defect hid in, since the prefix was itself what was wrong.
package gitignoretemplates_test

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/gitignoretemplates"
)

// TestPublishedActionIDs_NameActionsTheCatalogHolds holds every canonical ID
// this package offers a model to the catalog that has to resolve it.
//
// The IDs are the RelatedActions of each ActionSpec, which is the whole of
// what this package publishes: its Markdown formatters render no hints. The
// sibling cross-links named "gitignoretemplates" as the domain, where the
// catalog aggregates both actions into the shared template group
// ("template.gitignore_get"), so a model following one was answered
// "unknown action".
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	type publishedID struct{ where, id string }
	var published []publishedID
	for _, spec := range gitignoretemplates.ActionSpecs(client) {
		for _, related := range spec.RelatedActions {
			published = append(published, publishedID{where: "related action of " + spec.Name, id: related})
		}
	}

	// A silent collection would make every assertion below vacuous, which is
	// the way this test could rot into proving nothing.
	if len(published) == 0 {
		t.Fatal("collected no published action IDs, so the test asserts nothing")
	}

	for _, p := range published {
		t.Run(p.where+" "+p.id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(p.id)); !ok {
				t.Errorf("published action ID %q resolves to no action in the catalog", p.id)
			}
		})
	}
}
