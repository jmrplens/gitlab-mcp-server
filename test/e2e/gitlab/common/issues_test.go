//go:build e2e

// issues_test.go covers an issue's create and update through the server,
// which the old suite made only to build the issue its Enterprise
// scenarios stood on: the one an epic collects, the one whose weight
// events are listed.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestIssue_CreateAndUpdate_TitleFollows creates an issue on every surface
// in a shared project, then retitles it and reads the new title off the
// answer.
//
// Replaces: TestMeta_EpicIssues, TestMeta_IssueWeightEvents
func TestIssue_CreateAndUpdate_TitleFollows(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuecreate"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("issue")

		created := harness.Do[issues.Output](s, actionIssueCreate, withParams(params, map[string]any{"title": title, "description": "created by the e2e suite"}))
		if created.IID == 0 || created.Title != title {
			e.T.Fatalf("issue create answered %+v, want the issue %q with an iid", created, title)
		}

		updated := harness.Do[issues.Output](s, actionIssueUpdate, withParams(params, map[string]any{"issue_iid": created.IID, "title": "Updated " + title}))
		if updated.IID != created.IID || updated.Title != "Updated "+title {
			e.T.Errorf("issue update answered %+v, want issue #%d retitled", updated, created.IID)
		}
	})
}
