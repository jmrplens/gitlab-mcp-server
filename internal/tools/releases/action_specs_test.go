// action_specs_test.go holds every canonical action ID this package publishes
// against the catalog the server really builds.
//
// It is an external test package on purpose: internal/tools imports this one,
// so only a test outside package releases can import the catalog back and ask
// it whether an ID resolves. Asserting the corrected literal beside the
// constant would prove only that the constant equals itself.
package releases_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
)

// hintedActionID matches the canonical ID inside a next-step hint as
// toolutil.HintAction renders it, which is how the ID reaches a model.
var hintedActionID = regexp.MustCompile(`Use action '([^']+)'`)

// TestPublishedActionIDs_NameActionsTheCatalogHolds asserts that every ID this
// package publishes, as a related action and as a rendered next-step hint,
// resolves to an action the catalog holds.
//
// The asset-link actions are projected under the release domain, so the ID
// every surface resolves is "release.link_list" and "release.link_create". The
// cards already named those, while the related actions beside them spelled
// "release_link.list" and "release_link.create" after the owning package, and
// the comment in markdown.go recorded the disagreement instead of ending it.
// Both readers now take the IDs from the one block in action_specs.go.
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	specs := releases.ActionSpecs(client)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() is empty, the test asserts nothing")
	}

	type use struct{ where, id string }
	var uses []use
	for _, spec := range specs {
		for _, related := range spec.RelatedActions {
			uses = append(uses, use{where: "related action of " + spec.Name, id: related})
		}
	}
	for _, rendered := range renderedReleaseMarkdown() {
		for _, match := range hintedActionID.FindAllStringSubmatch(rendered.markdown, -1) {
			uses = append(uses, use{where: "hint in " + rendered.name, id: match[1]})
		}
	}
	if len(uses) == 0 {
		t.Fatal("no action IDs collected, the test asserts nothing")
	}

	for _, u := range uses {
		t.Run(u.where+" -> "+u.id, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(u.id)); !ok {
				t.Errorf("published action ID %q resolves to no catalog action", u.id)
			}
		})
	}
}

// TestReleaseSpecs_OwnIDsAreNamespacedUnderTheReleaseDomain asserts that each
// spec this package declares is itself reachable under the release domain,
// which is what makes the IDs above resolvable in the first place.
func TestReleaseSpecs_OwnIDsAreNamespacedUnderTheReleaseDomain(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	for _, spec := range releases.ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			id := actioncatalog.ActionID("release." + spec.Name)
			if _, ok := catalog.Action(id); !ok {
				t.Errorf("spec %q is not registered as %q", spec.Name, id)
			}
		})
	}
}

// TestReleaseHints_ReachEveryFormatter guards the collection above: a
// formatter whose fixture is too empty to reach its hints would silently
// contribute nothing, leaving its action IDs unchecked.
func TestReleaseHints_ReachEveryFormatter(t *testing.T) {
	for _, rendered := range renderedReleaseMarkdown() {
		t.Run(rendered.name, func(t *testing.T) {
			if !hintedActionID.MatchString(rendered.markdown) {
				t.Errorf("%s rendered no next-step hint, so its action IDs go unchecked", rendered.name)
			}
		})
	}
}

// renderedReleaseMarkdown renders every markdown formatter this package
// registers with a fixture populated far enough to reach the hints it writes.
func renderedReleaseMarkdown() []struct{ name, markdown string } {
	one := releases.Output{TagName: "v1.2.0", Name: "Release 1.2.0"}
	return []struct{ name, markdown string }{
		{"FormatMarkdown", releases.FormatMarkdown(one)},
		{"FormatListMarkdown", releases.FormatListMarkdown(releases.ListOutput{
			Releases: []releases.Output{one},
		})},
	}
}

// ultimateCatalog builds the canonical catalog at the Ultimate tier, so no
// action is missing for want of a license rather than for want of an ID.
func ultimateCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if len(catalog.Actions()) == 0 {
		t.Fatal("BuildActionCatalog() returned an empty catalog, every lookup would fail")
	}
	// A catalog that answers every lookup would pass the assertions above
	// without judging anything.
	if _, ok := catalog.Action("release.not_an_action"); ok {
		t.Fatal("catalog resolves an invented ID, the lookups below prove nothing")
	}
	return catalog
}
