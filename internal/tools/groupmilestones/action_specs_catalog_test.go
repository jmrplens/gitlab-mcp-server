// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package because the catalog is assembled by
// internal/tools, which imports this one: only a _test package may import back
// across that edge. The rule it enforces is the one 562 published IDs across
// the tree broke, this package's 33 among them, by naming a domain the catalog
// has no group for ("group_milestone.get" rather than
// "group.group_milestone_get"). Both halves are judged from what the package
// emits rather than from the constants that produced it: the RelatedActions of
// every spec, and the IDs the rendered Markdown hints name, so an ID added
// later is covered without this file being touched.
package groupmilestones_test

import (
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_RelatedActionsResolveInTheCatalog verifies every
// RelatedActions entry of every group milestone spec names an action the
// catalog holds.
func TestActionSpecs_RelatedActionsResolveInTheCatalog(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog := ultimateCatalog(t, client)

	for _, spec := range groupmilestones.ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				assertResolves(t, catalog, related, "RelatedActions")
			}
		})
	}
}

// TestMarkdownHints_NameActionsTheCatalogHolds verifies every action ID the
// rendered Markdown hints name resolves. Each list fixture carries one element
// because a formatter writes no footer for an empty collection, and so would
// render no hint to judge.
func TestMarkdownHints_NameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog := ultimateCatalog(t, client)

	rendered := map[string]string{
		"card": groupmilestones.FormatMarkdown(groupmilestones.Output{IID: 1, Title: "v1"}),
		"list": groupmilestones.FormatListMarkdownString(groupmilestones.ListOutput{
			Milestones: []groupmilestones.Output{{IID: 1, Title: "v1"}},
		}),
		"issues": groupmilestones.FormatIssuesMarkdownString(groupmilestones.IssuesOutput{
			Issues: []groupmilestones.IssueItem{{IID: 1, Title: "an issue"}},
		}),
		"merge_requests": groupmilestones.FormatMergeRequestsMarkdownString(groupmilestones.MergeRequestsOutput{
			MergeRequests: []groupmilestones.MergeRequestItem{{IID: 1, Title: "an MR"}},
		}),
		"burndown": groupmilestones.FormatBurndownChartEventsMarkdownString(groupmilestones.BurndownChartEventsOutput{
			Events: []groupmilestones.BurndownChartEventItem{{Action: "created", Weight: 1}},
		}),
	}

	for name, markdown := range rendered {
		t.Run(name, func(t *testing.T) {
			ids := hintedActionIDs(markdown)
			if len(ids) == 0 {
				t.Fatalf("%s rendered no action hint; the fixture no longer reaches the footer", name)
			}
			for _, id := range ids {
				assertResolves(t, catalog, id, "hint")
			}
		})
	}
}

// ultimateCatalog builds the catalog every published ID is judged against: the
// Ultimate tier so no licensed action is missing, and the MCP maintenance group
// so the server.* actions a hint may name are present.
func ultimateCatalog(t *testing.T, client *gitlabclient.Client) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// assertResolves fails when id names no action in the catalog, saying which
// surface published it so the failure points at the declaration to fix.
func assertResolves(t *testing.T, catalog *actioncatalog.Catalog, id, published string) {
	t.Helper()
	if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
		t.Errorf("%s %q resolves to no catalog action; a model following it is answered unknown action", published, id)
	}
}

// hintPattern matches the one sentence toolutil.HintAction writes. It is built
// from that function's own output around a sentinel, so the parser cannot drift
// from the writer: a reworded hint changes this pattern with it instead of
// leaving the test quietly finding nothing.
var hintPattern = func() *regexp.Regexp {
	const sentinel = "\x00"
	written := toolutil.HintAction(sentinel, sentinel)
	parts := regexp.MustCompile(sentinel).Split(written, -1)
	return regexp.MustCompile(regexp.QuoteMeta(parts[0]) + `([^']+)` + regexp.QuoteMeta(parts[1]))
}()

// hintedActionIDs returns the action IDs a rendered Markdown block invites a
// model to call next.
func hintedActionIDs(markdown string) []string {
	var ids []string
	for _, match := range hintPattern.FindAllStringSubmatch(markdown, -1) {
		ids = append(ids, match[1])
	}
	return ids
}
