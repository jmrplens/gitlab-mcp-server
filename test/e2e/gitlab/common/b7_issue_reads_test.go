//go:build e2e

// b7_issue_reads_test.go covers the issue reads that reach an issue from
// outside the project it lives in (by its global id, across every project
// the caller can see, and across a group) and the three scopes of the issue
// counts. The old CE suite asserted all six; the rebuilt suite reached them
// only from the read sweep, which binds parameters from the shared World and
// asserts nothing about the answer.
//
// One group with one project and one issue in it is built for the whole
// file, because every read here is a read: the counts are asserted exactly
// because the project the group holds has one open issue and nothing else.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupIssueFixture is a group holding one project that holds one open
// issue, which is what the group-scoped reads and the counts are held to.
type groupIssueFixture struct {
	group   fixture.Group
	project fixture.Project
	issue   fixture.Issue
	title   string
}

// newGroupIssueFixture builds the group, the project inside it and the one
// issue, titled distinctly enough for a search across every project to find
// that issue and no other.
func newGroupIssueFixture(e *harness.Env, prefix string) groupIssueFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix(prefix))
	project := fixture.NewProject(e, fixture.WithNamePrefix(prefix), fixture.InGroup(group))
	title := e.Name(prefix + "-issue")
	return groupIssueFixture{group: group, project: project, issue: fixture.NewIssue(e, project, title), title: title}
}

// issueIDsOf lists the instance-wide ids of an issue listing, which is what
// a listing that crosses projects is matched on: an iid is only unique
// within its project.
func issueIDsOf(listed []issues.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, issue := range listed {
		ids = append(ids, issue.ID)
	}
	return ids
}

// TestIssueReads_ByIDAndAcrossProjects reads the fixture issue on every
// surface by its global id, and finds it again in the listing of every
// issue the caller opened.
//
// It declares NeedAdmin because GET /issues/:id is guarded by
// authenticated_as_admin! in lib/api/issues.rb and the API documentation says
// "Only for administrators". The Docker run's token has admin_mode, so this
// changes nothing there; against a self-hosted instance whose token is an
// ordinary user it skips with a reason rather than failing on a 403.
func TestIssueReads_ByIDAndAcrossProjects(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) groupIssueFixture {
		return newGroupIssueFixture(e, "issuereads")
	}, func(e *harness.Env, surface harness.Surface, f groupIssueFixture) {
		s := e.On(surface)

		got := harness.Do[issues.Output](s, actionIssueGetByID, map[string]any{"issue_id": f.issue.ID})
		if got.ID != f.issue.ID || got.IID != f.issue.IID || got.ProjectID != f.project.ID || got.Title != f.title {
			e.T.Errorf("get_by_id answered %+v, want issue %d (#%d of project %d) titled %q",
				got, f.issue.ID, f.issue.IID, f.project.ID, f.title)
		}

		// Scoped to what this user opened and searched for the one title,
		// so the listing is this issue rather than whatever else the run
		// left on the instance.
		listed := harness.Do[issues.ListAllOutput](s, actionIssueListAll,
			map[string]any{"scope": "created_by_me", "search": f.title})
		if !containsID(issueIDsOf(listed.Issues), f.issue.ID) {
			e.T.Errorf("the issues created by this user and matching %q are %v, want issue %d among them",
				f.title, issueIDsOf(listed.Issues), f.issue.ID)
		}
	})
}

// TestIssueListGroup_ListsTheIssueOfAProjectInTheGroup lists the issues of
// the fixture group and finds the one issue its project holds.
//
// It runs on the two dispatcher surfaces only. The individual surface binds
// the tool name gitlab_issue_list_group to the first action that declares
// it, and both issue.list_group and group.issues declare it
// (individual_catalog.go names the pair), so issue.list_group is served
// under no tool of its own there.
func TestIssueListGroup_ListsTheIssueOfAProjectInTheGroup(t *testing.T) {
	e := harness.New(t)
	f := newGroupIssueFixture(e, "issuegrouplist")

	harness.OnSurfaces(e, "the individual surface projects gitlab_issue_list_group from group.issues, so issue.list_group is registered under no tool name there",
		[]harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta}, func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)

			listed := harness.Do[issues.ListGroupOutput](s, actionIssueListGroup,
				map[string]any{"group_id": f.group.IDParam()})
			if !containsID(issueIDsOf(listed.Issues), f.issue.ID) {
				e.T.Errorf("the issues of group %d are %v, want issue %d among them",
					f.group.ID, issueIDsOf(listed.Issues), f.issue.ID)
			}
		})
}

// TestIssueStatistics_GlobalGroupAndProject reads the issue counts at all
// three scopes on every surface: the project and the group are held to the
// one open issue they hold, and the global counts to holding at least it.
func TestIssueStatistics_GlobalGroupAndProject(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) groupIssueFixture {
		return newGroupIssueFixture(e, "issuestats")
	}, func(e *harness.Env, surface harness.Surface, f groupIssueFixture) {
		s := e.On(surface)

		project := harness.Do[issuestatistics.StatisticsOutput](s, actionIssueStatisticsGetProject,
			map[string]any{"project_id": f.project.IDParam()})
		assertOneOpenIssueCounted(e, "the project", project)

		group := harness.Do[issuestatistics.StatisticsOutput](s, actionIssueStatisticsGetGroup,
			map[string]any{"group_id": f.group.IDParam()})
		assertOneOpenIssueCounted(e, "the group", group)

		// Global, scoped to what this user opened: the run has opened at
		// least the fixture issue, and how much else is the run's business
		// rather than this assertion's.
		all := harness.Do[issuestatistics.StatisticsOutput](s, actionIssueStatisticsGet,
			map[string]any{"scope": "created_by_me"})
		counts := all.Statistics.Counts
		if counts.Opened < 1 || counts.All < counts.Opened {
			e.T.Errorf("the global issue counts are %+v, want at least one open issue and a total no smaller than it", counts)
		}
	})
}

// assertOneOpenIssueCounted holds a statistics answer to the single open
// issue the fixture group and its project hold.
func assertOneOpenIssueCounted(e *harness.Env, scope string, stats issuestatistics.StatisticsOutput) {
	e.T.Helper()

	counts := stats.Statistics.Counts
	if counts.All != 1 || counts.Opened != 1 || counts.Closed != 0 {
		e.T.Errorf("the issue counts of %s are %+v, want one issue, open", scope, counts)
	}
}
