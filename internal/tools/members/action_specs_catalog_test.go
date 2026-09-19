// action_specs_catalog_test.go holds this package's published action IDs
// against the catalog that has to resolve them.
//
// It is an external test package because the catalog is assembled by
// internal/tools, which imports this one: only a _test package may import back
// the other way. The IDs are read off the published surface, the specs'
// RelatedActions and the hints the formatters write, rather than restated as
// literals beside the constants, since a test that copies a constant proves
// only that it was copied twice.
package members_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/members"
)

// hintedAction matches what toolutil.HintAction writes, which is the one form
// an action ID reaches a model in through a Markdown hint.
var hintedAction = regexp.MustCompile(`Use action '([^']+)' to `)

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
// What it guards: these actions are aggregated into the project group, so their
// IDs read project.*. The package published member.* and members.* instead,
// each of which reads as a plausible pair and resolves to nothing, and a model
// following one is answered "unknown action".
func TestActionSpecs_EveryRelatedActionID_ResolvesInTheCatalog(t *testing.T) {
	t.Parallel()

	ids := catalogIDs(t)
	specs := members.ActionSpecs(nil)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() returned no specs, so nothing below was asserted")
	}
	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			t.Parallel()

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
	for _, spec := range members.ActionSpecs(nil) {
		if _, ok := actions[spec.Name]; !ok {
			t.Errorf("spec %q reaches the catalog under no action ID", spec.Name)
		}
	}
}

// TestMarkdown_EveryHintedActionID_ResolvesInTheCatalog verifies the IDs the
// Markdown formatters hand a model resolve too.
//
// They are asserted apart from the specs because the two sets lived in
// different files and had drifted: markdown.go named the project.* IDs while
// action_specs.go named member.*, and nothing compared them. The count check
// at the end is what stops this passing by matching nothing, which is how it
// would read if the hint form ever changed.
func TestMarkdown_EveryHintedActionID_ResolvesInTheCatalog(t *testing.T) {
	t.Parallel()

	ids := catalogIDs(t)
	member := members.Output{ID: 1, Username: "octocat", WebURL: "https://gitlab.example.com/octocat"}
	rendered := map[string]string{
		"FormatListMarkdownString": members.FormatListMarkdownString(members.ListOutput{Members: []members.Output{member}}),
		"FormatMarkdown":           members.FormatMarkdown(member),
	}
	hinted, total := readHints(rendered)
	if want := 4; total != want {
		t.Fatalf("read %d action hints from the formatters, want %d: the assertions below would judge the wrong set", total, want)
	}
	for where, actions := range hinted {
		t.Run(where, func(t *testing.T) {
			t.Parallel()

			for _, action := range actions {
				if _, ok := ids[action]; !ok {
					t.Errorf("hints action %q, which the catalog does not hold", action)
				}
			}
		})
	}
}

// readHints returns the action IDs each rendered document hints, and how many
// there are in all. The count is taken here rather than inside the subtests so
// they can run in parallel without sharing a counter.
func readHints(rendered map[string]string) (hinted map[string][]string, total int) {
	hinted = make(map[string][]string, len(rendered))
	for where, markdown := range rendered {
		for _, match := range hintedAction.FindAllStringSubmatch(markdown, -1) {
			hinted[where] = append(hinted[where], match[1])
			total++
		}
	}
	return hinted, total
}
