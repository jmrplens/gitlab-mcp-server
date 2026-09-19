// action_specs_test.go holds the published-ID assertions for the namespace
// actions.
//
// The test package is external on purpose. The oracle for "does this ID exist"
// is the canonical catalog, which only [tools.BuildActionCatalog] builds, and
// internal/tools imports this package: an in-package test could not reach the
// catalog without an import cycle, so it could only compare the constants
// against a domain prefix written out beside them. That is the shape the
// defect hid in, since the prefix was itself what was wrong.
package namespaces_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/namespaces"
)

// hintActionPattern matches the canonical ID inside the sentence
// [toolutil.HintAction] renders, which is "Use action 'id' to purpose".
var hintActionPattern = regexp.MustCompile(`Use action '([^']+)' to `)

// TestPublishedActionIDs_NameActionsTheCatalogHolds holds every canonical ID
// this package offers a model to the catalog that has to resolve it.
//
// The IDs are read from what the package really publishes: the RelatedActions
// of each ActionSpec, and the hints rendered into the list footer and the
// namespace card. Nothing compared either against the catalog before, and the
// constants named "namespace" as the domain where the catalog aggregates these
// actions into the user group ("user.namespace_get"), so a model following one
// was answered "unknown action".
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
	for _, spec := range namespaces.ActionSpecs(client) {
		for _, related := range spec.RelatedActions {
			published = append(published, publishedID{where: "related action of " + spec.Name, id: related})
		}
	}
	rendered := map[string]string{
		"list footer":    namespaces.FormatListMarkdownString(namespaces.ListOutput{}),
		"namespace card": namespaces.FormatMarkdownString(namespaces.Output{ID: 1, Name: "example", Kind: "group"}),
	}
	for where, text := range rendered {
		for _, id := range hintActionPattern.FindAllStringSubmatch(text, -1) {
			published = append(published, publishedID{where: "hint in the " + where, id: id[1]})
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
