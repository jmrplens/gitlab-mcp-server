// action_specs_test.go holds the tests over the canonical action IDs the
// search specs publish as cross-links.
package search

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/repository"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/wikis"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// crossLinkableIDs is the set of canonical IDs the search cross-links may name:
// every search action, plus the actions of the packages a search result points
// a model at next, each under the domain of the group that package joins.
//
// Every action half is built from the owning package's own ActionSpecs rather
// than listed here, because that half is what was wrong and a list written
// beside the constants would agree with them whatever they said. A domain is
// shared by several packages (project, issue and repository each aggregate
// more than one), which is why the packages are grouped under their prefix
// rather than paired one to one.
func crossLinkableIDs(t *testing.T) map[string]bool {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))

	byDomain := map[string][][]toolutil.ActionSpec{
		domainPrefix:     {ActionSpecs(client)},
		"project.":       {projects.ActionSpecs(client, true), milestones.ActionSpecs(client), members.ActionSpecs(client)},
		"issue.":         {issues.ActionSpecs(client), issuenotes.ActionSpecs(client)},
		"repository.":    {repository.ActionSpecs(client), commits.ActionSpecs(client), files.ActionSpecs(client)},
		"merge_request.": {mergerequests.ActionSpecs(client)},
		"user.":          {users.ActionSpecs(client, true)},
		"wiki.":          {wikis.ActionSpecs(client)},
	}

	ids := map[string]bool{}
	for prefix, groups := range byDomain {
		for _, specs := range groups {
			if len(specs) == 0 {
				t.Errorf("a package contributing to %q registered no specs", prefix)
			}
			for _, spec := range specs {
				ids[prefix+spec.Name] = true
			}
		}
	}
	return ids
}

// TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds asserts that every
// related action a search spec publishes is an ID the catalog really holds.
//
// Four of them named nothing, and each was wrong in a way that reads as
// plausible. "milestone.list" and "milestone.get" take the resource for the
// domain, but milestones are routes on the project group, so the IDs are
// "project.milestone_list" and "project.milestone_get". "issue.notes_list"
// pluralises a name the catalog spells "issue.note_list". And
// "project.members_list" is the tail of the individual tool name
// gitlab_project_members_list read backwards: the action is "project.members",
// and the nearest ID by string distance is "project.member_edit", which would
// send a model to change somebody's access level when it asked for the roster.
func TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	registered := crossLinkableIDs(t)

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatalf("%s publishes no related actions", spec.Name)
			}
			for _, related := range spec.RelatedActions {
				if !registered[related] {
					t.Errorf("%s relates to %q, which the catalog holds no action under", spec.Name, related)
				}
			}
		})
	}
}

// TestCrossDomainIDs_NameActionsTheOwningPackagesRegister asserts the four IDs
// this package spells for other domains, on their own.
//
// They are the entries a fix had to choose rather than derive, so each is held
// to the package that owns it by name: the action half comes from that
// package's specs, and a rename there fails here rather than on the tree-wide
// gate.
func TestCrossDomainIDs_NameActionsTheOwningPackagesRegister(t *testing.T) {
	registered := crossLinkableIDs(t)

	for _, id := range []string{
		actionProjectMembers,
		actionProjectMilestoneGet,
		actionProjectMilestoneList,
		actionIssueNoteList,
	} {
		t.Run(id, func(t *testing.T) {
			if !registered[id] {
				t.Errorf("%q names no action any of the packages this test reads registers", id)
			}
			if strings.HasPrefix(id, domainPrefix) {
				t.Errorf("%q carries the search domain, so it is not a cross-domain ID", id)
			}
		})
	}
}

// TestSearchActionIDs_AreTheSpecNamesUnderTheCatalogDomain asserts that the ID
// constants and the names the specs are registered under are one source.
//
// Ten ID constants sat beside ten spec names written as literals at the call
// sites, and six of the constants were referenced by nothing at all, which is
// the state a cross-link drifts out of without anything noticing.
func TestSearchActionIDs_AreTheSpecNamesUnderTheCatalogDomain(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	ids := map[string]string{
		specCode:          actionSearchCode,
		specProjects:      actionSearchProjects,
		specMergeRequests: actionSearchMergeRequests,
		specIssues:        actionSearchIssues,
		specCommits:       actionSearchCommits,
		specMilestones:    actionSearchMilestones,
		specNotes:         actionSearchNotes,
		specSnippets:      actionSearchSnippets,
		specUsers:         actionSearchUsers,
		specWiki:          actionSearchWiki,
	}

	registered := map[string]bool{}
	for _, spec := range ActionSpecs(client) {
		registered[spec.Name] = true
	}
	for name, id := range ids {
		t.Run(name, func(t *testing.T) {
			if !registered[name] {
				t.Errorf("no spec is registered under the name %q", name)
			}
			if want := domainPrefix + name; id != want {
				t.Errorf("the ID for %q is %q, want %q", name, id, want)
			}
		})
	}
	if len(registered) != len(ids) {
		t.Errorf("%d specs are registered and %d have an ID constant", len(registered), len(ids))
	}
}
