// action_specs_test.go holds every canonical action ID this package publishes
// against the catalog the server really builds.
//
// It is an external test package because the catalog is assembled by
// internal/tools, which imports this one: only a _test package may import back
// across that edge. The rule it enforces is the one this package's seven
// published IDs broke, by naming the owner package as the domain
// ("repositorysubmodules.list_submodules" rather than
// "repository.list_submodules") and by pointing at "commit.get", which is a
// route on the repository group too. Both halves are judged from what the
// package emits rather than from the constants that produced it: the
// RelatedActions of every spec, and the IDs the rendered Markdown hints name,
// so an ID added later is covered without this file being touched.
package repositorysubmodules_test

import (
	"regexp"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/repositorysubmodules"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_RelatedActionsResolveInTheCatalog verifies every
// RelatedActions entry of every submodule spec names an action the catalog
// holds.
func TestActionSpecs_RelatedActionsResolveInTheCatalog(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog := ultimateCatalog(t, client)

	for _, spec := range repositorysubmodules.ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				assertResolves(t, catalog, related, "RelatedActions")
			}
		})
	}
}

// TestMarkdownHints_NameActionsTheCatalogHolds verifies every action ID the
// rendered Markdown hints name resolves. The list fixture carries a non-zero
// Count because the formatter writes no footer for an empty repository, and so
// would render no hint to judge.
func TestMarkdownHints_NameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	catalog := ultimateCatalog(t, client)

	rendered := map[string]*mcp.CallToolResult{
		"list": repositorysubmodules.FormatListMarkdown(repositorysubmodules.ListOutput{
			Submodules: []repositorysubmodules.SubmoduleEntry{{Name: "vendor", Path: "vendor"}},
			Count:      1,
		}),
		"read":   repositorysubmodules.FormatReadMarkdown(repositorysubmodules.ReadOutput{FileName: "go.mod"}),
		"update": repositorysubmodules.FormatUpdateMarkdown(repositorysubmodules.UpdateOutput{ShortID: "abc12345"}),
	}

	for name, result := range rendered {
		t.Run(name, func(t *testing.T) {
			ids := hintedActionIDs(markdownText(t, result))
			if len(ids) == 0 {
				t.Fatalf("%s rendered no action hint; the fixture no longer reaches the footer", name)
			}
			for _, id := range ids {
				assertResolves(t, catalog, id, "hint")
			}
		})
	}
}

// markdownText returns the Markdown a formatter wrote into its tool result.
func markdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("tool result carries no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content is %T, want text", result.Content[0])
	}
	return text.Text
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
