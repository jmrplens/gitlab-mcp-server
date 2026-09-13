//go:build e2e

// b7_group_search_test.go covers the two group reads that answer about a
// group without being given one: the search that finds it by name, and the
// project listing that says what it holds.
//
// groupreads_test.go asks the group listing for a group by a search filter,
// which is a different route: this one is the dedicated search action, whose
// whole input is a query string. Both actions were reached here only by a
// read sweep, which asserts nothing about the answer, so neither said
// anything about whether the group asked for is the group found.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupSearchFixture is a group with one project in it and one sibling group
// beside it, which is what the two reads are held to.
type groupSearchFixture struct {
	group   fixture.Group
	project fixture.Project
	sibling fixture.Group
}

// TestGroupSearch_FixtureGroup_FindsItByNameAndListsWhatItHolds searches for
// a group by its name and lists the projects it holds.
//
// It runs on the dynamic, meta and individual surfaces against one group
// built for all three, since all three only read it. It asserts that the
// search finds the group whose name it was given and does not answer with the
// sibling group beside it, and that the project listing holds the one project
// created in the group.
func TestGroupSearch_FixtureGroup_FindsItByNameAndListsWhatItHolds(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) groupSearchFixture {
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("search"))
		return groupSearchFixture{
			group:   group,
			project: fixture.NewProject(e, fixture.InGroup(group), fixture.WithNamePrefix("held")),
			sibling: fixture.NewGroup(e, fixture.WithGroupNamePrefix("sibling")),
		}
	}, func(e *harness.Env, surface harness.Surface, f groupSearchFixture) {
		s := e.On(surface)

		found := harness.Do[groups.ListOutput](s, actionGroupSearch, map[string]any{"query": f.group.Name})
		ids := groupIDsOf(found.Groups)
		if !containsID(ids, f.group.ID) {
			e.T.Errorf("the search for %q answered the groups %v, want group %d among them", f.group.Name, ids, f.group.ID)
		}
		// The two fixture groups carry different names, so a search for one
		// that answers with the other is matching something other than what
		// it was asked for.
		if containsID(ids, f.sibling.ID) {
			e.T.Errorf("the search for %q answered the groups %v, and group %d is named %q", f.group.Name, ids, f.sibling.ID, f.sibling.Name)
		}

		held := harness.Do[groups.ListProjectsOutput](s, actionGroupProjects, map[string]any{"group_id": f.group.IDParam()})
		if projects := groupProjectIDs(held.Projects); !containsID(projects, f.project.ID) {
			e.T.Errorf("the group lists the projects %v, want project %d among them", projects, f.project.ID)
		}
	})
}
