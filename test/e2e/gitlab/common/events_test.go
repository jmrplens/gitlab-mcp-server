//go:build e2e

// events_test.go covers the two event feeds the user tool serves: the run
// user's own contribution events and a project's activity. Both are read
// against a project of the test's own with one issue the run user opened
// in it, because that is the newest thing the run user did and so the one
// event certain to be on the first page of a feed a long run fills with
// hundreds of others.

package common

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/events"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The polling an event feed is given: GitLab writes some events from a
// background job, so the feed can lag the action by a moment.
const (
	eventFeedInterval = 2 * time.Second
	eventFeedWait     = 60 * time.Second
)

// eventFixture is what the feeds are read against: a project and the one
// issue the run user opened in it.
type eventFixture struct {
	project fixture.Project
	issue   fixture.Issue
}

// TestEvents_OwnIssue_ListsTheRunUsersAndTheProjects opens one issue in a
// project of its own, then on every surface reads the run user's
// contribution feed until it holds an event in that project, and the
// project's activity until it holds the issue's opening by the run user.
//
// Replaces: TestMeta_UserTodosEvents
func TestEvents_OwnIssue_ListsTheRunUsersAndTheProjects(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.SurfacesWith(e, func(e *harness.Env) eventFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("events"))
		return eventFixture{project: project, issue: fixture.NewIssue(e, project, "event fixture issue")}
	}, func(e *harness.Env, surface harness.Surface, f eventFixture) {
		s := e.On(surface)

		contributions := harness.Eventually(s, actionUserEventListContributions, map[string]any{"per_page": 100},
			eventFeedInterval, eventFeedWait, func(out events.ListContributionEventsOutput) bool {
				return hasContributionIn(out.Events, f.project.ID)
			})
		e.T.Logf("the run user's feed holds %d event(s) on its first page, one of them in project %d", len(contributions.Events), f.project.ID)

		activity := harness.Eventually(s, actionUserEventListProject, map[string]any{"project_id": f.project.IDParam(), "per_page": 100},
			eventFeedInterval, eventFeedWait, func(out events.ListProjectEventsOutput) bool {
				return hasIssueEventBy(out.Events, rt.UserID, f.issue.IID)
			})
		e.T.Logf("the activity of project %d holds %d event(s), one of them the run user opening issue #%d", f.project.ID, len(activity.Events), f.issue.IID)
	})
}

// hasContributionIn reports whether a contribution feed holds an event in
// the given project.
func hasContributionIn(listed []events.ContributionEventOutput, projectID int64) bool {
	for _, event := range listed {
		if event.ProjectID == projectID {
			return true
		}
	}
	return false
}

// hasIssueEventBy reports whether a project's activity holds an event by the
// given author on the issue with the given IID.
func hasIssueEventBy(listed []events.ProjectEventOutput, authorID, issueIID int64) bool {
	for _, event := range listed {
		if event.AuthorID == authorID && event.TargetIID == issueIID {
			return true
		}
	}
	return false
}
