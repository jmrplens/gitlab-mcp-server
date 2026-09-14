//go:build e2e

// userprojects_test.go covers the two user-scoped project listings of the
// project tool. They are read apart because GitLab answers them from two
// different records: a star is a row written by the request that stars, and
// a contribution is a record GitLab maintains outside the request that made
// it, so only the first can be asserted the moment it is made.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUserProjects_Starred_ListsWhatTheRunUserStarred stars one project and
// finds it, on every surface, in the projects the run user has starred.
//
// Replaces: TestIndividual_UserProjects, TestMeta_UserProjects
func TestUserProjects_Starred_ListsWhatTheRunUserStarred(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("userproj"))
		if _, _, err := e.Client().GL().Projects.StarProject(project.ID, gl.WithContext(e.Ctx)); err != nil {
			e.T.Fatalf("starring project %d: %v", project.ID, err)
		}
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		// Newest first, so the project just starred is on the first page
		// whatever the account starred before it.
		starred := harness.Do[projects.ListOutput](s, actionProjectListUserStarred,
			map[string]any{"user_id": rt.Username, "order_by": "id", "sort": "desc", "per_page": 100})
		if !containsID(userProjectIDs(starred.Projects), project.ID) {
			e.T.Errorf("the projects %s starred do not hold %d: %v", rt.Username, project.ID, userProjectIDs(starred.Projects))
		}
	})
}

// TestUserProjects_Contributed_AnswersForTheUserItWasAskedAbout reads the
// contributed listing of an account created moments ago, which has
// contributed to nothing and must therefore answer with nothing, and the run
// user's own, whose every entry has to be a whole project.
//
// The listing is deliberately not asserted to hold a project the test
// contributed to, and that is the finding this scenario carries rather than
// a concession: GitLab answers it from a record it keeps beside the events,
// and that record is not written by the request that makes the contribution.
// Measured on GitLab CE 19.3.1, a project the run user committed to, opened
// an issue in and starred was still absent from its author's contributed
// listing three minutes later, while the same issue's event was on the
// contribution feed within seconds, which events_test.go asserts. Waiting
// longer would only measure that instance's job queue, so what is asserted
// here is the half that is the endpoint's own: that it answers per user.
//
// The fresh account is what makes the emptiness mean something. An endpoint
// that ignored the user it was asked about and answered for the caller would
// pass a "the answer is a well-formed listing" check and fail this one.
//
// Replaces: TestIndividual_UserProjects, TestMeta_UserProjects
func TestUserProjects_Contributed_AnswersForTheUserItWasAskedAbout(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	rt := e.Runtime()

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "contrib")
	}, func(e *harness.Env, surface harness.Surface, fresh fixture.User) {
		s := e.On(surface)

		none := harness.Do[projects.ListOutput](s, actionProjectListUserContributed,
			map[string]any{"user_id": fresh.Username, "per_page": 100})
		if len(none.Projects) != 0 {
			e.T.Errorf("the contributed projects of %s, an account created moments ago, are %v, want none",
				fresh.Username, userProjectIDs(none.Projects))
		}

		mine := harness.Do[projects.ListOutput](s, actionProjectListUserContributed,
			map[string]any{"user_id": rt.Username, "order_by": "id", "sort": "desc", "per_page": 100})
		for _, project := range mine.Projects {
			if project.ID == 0 || project.PathWithNamespace == "" {
				e.T.Errorf("a contributed project of %s is listed without an ID or a path: %+v", rt.Username, project)
			}
		}
		e.T.Logf("%d contributed project(s) listed for %s", len(mine.Projects), rt.Username)
	})
}

// userProjectIDs collects the IDs of listed projects.
func userProjectIDs(listed []projects.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, project := range listed {
		ids = append(ids, project.ID)
	}
	return ids
}
