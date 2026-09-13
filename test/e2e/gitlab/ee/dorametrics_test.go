//go:build e2e

// dorametrics_test.go covers the two DORA metric reads, a project's and a
// group's, and the one refusal the project read makes on its own: an
// environment tier filter the endpoint does not take.

package ee

import (
	"maps"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dorametrics"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// doraFixture is a group and a project to ask metrics of.
type doraFixture struct {
	group   fixture.Group
	project fixture.Project
}

// buildDORAFixture creates both. Neither has deployed anything, so every
// metric is empty, and what the test asserts is that each scope answers.
func buildDORAFixture(e *harness.Env) doraFixture {
	return doraFixture{
		group:   fixture.NewGroup(e, fixture.WithGroupNamePrefix("dora")),
		project: fixture.NewProject(e, fixture.WithNamePrefix("dora")),
	}
}

// TestDORAMetrics_ProjectAndGroup_AnswerOverTheLastMonth reads the
// deployment frequency of a project and the lead time of a group over the
// last month on every surface, and shows that an environment_tiers filter is
// refused with the hint to omit it.
//
// Replaces: TestMeta_DORAMetrics
func TestDORAMetrics_ProjectAndGroup_AnswerOverTheLastMonth(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))
	end := time.Now().UTC()
	start := end.AddDate(0, -1, 0)
	window := map[string]any{"start_date": start.Format(time.DateOnly), "end_date": end.Format(time.DateOnly)}

	harness.SurfacesWith(e, buildDORAFixture, func(e *harness.Env, surface harness.Surface, f doraFixture) {
		s := e.On(surface)

		project := harness.Do[dorametrics.Output](s, actionDORAMetricsProject, withParams(window, map[string]any{
			"project_id": f.project.IDParam(), "metric": "deployment_frequency", "interval": "daily",
		}))
		e.T.Logf("project %s deployment frequency: %d metric rows", f.project.Path, len(project.Metrics))

		refused := harness.ExpectToolError(s, actionDORAMetricsProject, withParams(window, map[string]any{
			"project_id": f.project.IDParam(), "metric": "deployment_frequency", "interval": "daily",
			"environment_tiers": []string{"production"},
		}), "environment_tiers")
		assertMentions(e, "the environment_tiers refusal", refused, "omit environment_tiers", "deployment environment tiers")

		group := harness.Do[dorametrics.Output](s, actionDORAMetricsGroup, withParams(window, map[string]any{
			"group_id": f.group.IDParam(), "metric": "lead_time_for_changes", "interval": "all",
		}))
		e.T.Logf("group %s lead time: %d metric rows", f.group.Path, len(group.Metrics))
	})
}

// withParams returns one parameter map holding both, for a call that shares
// a window or a scope with its siblings and adds its own.
func withParams(shared, own map[string]any) map[string]any {
	merged := make(map[string]any, len(shared)+len(own))
	maps.Copy(merged, shared)
	maps.Copy(merged, own)
	return merged
}
