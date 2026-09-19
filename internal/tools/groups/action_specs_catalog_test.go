// action_specs_catalog_test.go holds every canonical action ID this package
// publishes against the catalog the server really builds.
//
// It is an external test package on purpose: internal/tools imports this one,
// so only a test outside package groups can import the catalog back and ask it
// whether an ID resolves. Asserting the corrected literal beside the constant
// would prove only that the constant equals itself.
package groups_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
)

// hintedActionID matches the canonical ID inside a next-step hint as
// toolutil.HintAction renders it, which is how the ID reaches a model.
var hintedActionID = regexp.MustCompile(`Use action '([^']+)'`)

// TestPublishedActionIDs_NameActionsTheCatalogHolds asserts that every ID this
// package publishes, as a related action and as a rendered next-step hint,
// resolves to an action the catalog holds.
//
// The two sides had drifted apart. The cards named "group.group_member_add"
// and "group.push_rule_edit", which the catalog holds, while the related
// actions beside them spelled the verb-first individual tool names
// "group.member_add", "group.get_push_rules", "group.add_push_rule",
// "group.edit_push_rule" and "group.delete_push_rule", which no surface
// resolves. The closest surviving string for the first was "group.members",
// which lists members instead of adding one, so following it would have sent
// a model to the wrong call rather than to none.
func TestPublishedActionIDs_NameActionsTheCatalogHolds(t *testing.T) {
	catalog := ultimateCatalog(t)
	client := testutil.NewTestClient(t, http.NotFoundHandler())

	specs := groups.ActionSpecs(client)
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
	for _, rendered := range renderedGroupMarkdown() {
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

// TestGroupHints_ReachEveryFormatter guards the collection above: a formatter
// whose fixture is too empty to reach its hints would silently contribute
// nothing, leaving its action IDs unchecked.
func TestGroupHints_ReachEveryFormatter(t *testing.T) {
	for _, rendered := range renderedGroupMarkdown() {
		t.Run(rendered.name, func(t *testing.T) {
			if !hintedActionID.MatchString(rendered.markdown) {
				t.Errorf("%s rendered no next-step hint, so its action IDs go unchecked", rendered.name)
			}
		})
	}
}

// renderedGroupMarkdown renders every markdown formatter of this package that
// writes next-step hints, with a fixture populated far enough to reach them.
func renderedGroupMarkdown() []struct{ name, markdown string } {
	group := groups.Output{ID: 3, Name: "Platform", FullPath: "acme/platform"}
	project := groups.ProjectItem{ID: 9, Name: "api", PathWithNamespace: "acme/platform/api"}
	hook := groups.HookOutput{ID: 4, URL: "https://example.test/hook"}
	return []struct{ name, markdown string }{
		{"FormatOutputMarkdown", groups.FormatOutputMarkdown(group)},
		{"FormatListMarkdown", groups.FormatListMarkdown(groups.ListOutput{Groups: []groups.Output{group}})},
		{"FormatMemberListMarkdown", groups.FormatMemberListMarkdown(groups.MemberListOutput{
			Members: []groups.MemberOutput{{ID: 11, Username: "ada"}},
		})},
		{"FormatListProjectsMarkdown", groups.FormatListProjectsMarkdown(groups.ListProjectsOutput{
			Projects: []groups.ProjectItem{project},
		})},
		{"FormatHookMarkdown", groups.FormatHookMarkdown(hook)},
		{"FormatHookListMarkdown", groups.FormatHookListMarkdown(groups.HookListOutput{
			Hooks: []groups.HookOutput{hook},
		})},
		{"FormatTransferLocationsListMarkdown", groups.FormatTransferLocationsListMarkdown(
			groups.TransferLocationsListOutput{
				Locations: []groups.TransferLocationOutput{{ID: 2, FullPath: "acme"}},
			},
		)},
		{"FormatProvisionedUsersListMarkdown", groups.FormatProvisionedUsersListMarkdown(
			groups.ProvisionedUsersListOutput{
				Users: []groups.ProvisionedUserOutput{{ID: 12, Username: "grace"}},
			},
		)},
		{"FormatPushRuleMarkdown", groups.FormatPushRuleMarkdown(groups.PushRuleOutput{ID: 5})},
		{"FormatShareGroupMarkdown", groups.FormatShareGroupMarkdown(groups.ShareGroupOutput{
			Message: "Group shared", SharedGroupID: 3,
		})},
		{"FormatSharedProjectsListMarkdown", groups.FormatSharedProjectsListMarkdown(
			groups.SharedProjectsListOutput{Projects: []groups.ProjectItem{project}},
		)},
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
	if _, ok := catalog.Action("group.not_an_action"); ok {
		t.Fatal("catalog resolves an invented ID, the lookups below prove nothing")
	}
	return catalog
}
