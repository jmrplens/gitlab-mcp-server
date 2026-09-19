// action_specs_test.go holds the published-ID assertions for the project
// import/export actions.
//
// The test package is external on purpose. The oracle for "does this ID
// exist" is the canonical catalog, which only [tools.BuildActionCatalog]
// builds, and internal/tools imports this package: an in-package test could
// not reach the catalog without an import cycle, so it could only compare the
// constants against a domain prefix written out beside them. That is the shape
// the defect hid in, since the prefix was itself what was wrong.
package projectimportexport_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectimportexport"
)

// hintActionPattern matches the canonical ID inside the sentence
// [toolutil.HintAction] renders, which is "Use action 'id' to purpose".
var hintActionPattern = regexp.MustCompile(`Use action '([^']+)' to `)

// TestPublishedActionIDs_NameActionsTheCatalogHolds holds every canonical ID
// this package offers a model to the catalog that has to resolve it.
//
// Both halves of that surface are read from what the package really publishes:
// the RelatedActions of each ActionSpec, and the hint rendered into the export
// status card. Until this existed nothing compared either against the catalog,
// and all five IDs named the owner package as the domain
// ("projectimportexport.export_status") where the catalog aggregates these
// actions into the project group ("project.export_status"). Every one of them
// answered a model "unknown action" the first time it followed the hint.
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
	for _, spec := range projectimportexport.ActionSpecs(client) {
		for _, related := range spec.RelatedActions {
			published = append(published, publishedID{where: "related action of " + spec.Name, id: related})
		}
	}
	for _, id := range hintActionPattern.FindAllStringSubmatch(exportStatusMarkdown(t), -1) {
		published = append(published, publishedID{where: "hint on the export status card", id: id[1]})
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

// exportStatusMarkdown renders the export status card and returns its text, so
// the hint is read as a model receives it rather than from the constant behind
// it.
func exportStatusMarkdown(t *testing.T) string {
	t.Helper()
	result := projectimportexport.FormatExportStatusMarkdown(projectimportexport.ExportStatusOutput{
		ID:                1,
		Name:              "example",
		PathWithNamespace: "group/example",
		ExportStatus:      "finished",
	})
	if len(result.Content) == 0 {
		t.Fatal("FormatExportStatusMarkdown() returned no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("FormatExportStatusMarkdown() content = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}
