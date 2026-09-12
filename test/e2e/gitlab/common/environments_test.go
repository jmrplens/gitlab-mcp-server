//go:build e2e

// environments_test.go covers the creation of an environment through the
// server, which the old suite made only to build the environment its
// deployment approval scenario stood on.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestEnvironment_Create_AnswersTheEnvironment creates an environment on
// every surface in a shared project and reads its name and slug off the
// answer.
//
// Replaces: TestMeta_DeploymentApproveOrReject
func TestEnvironment_Create_AnswersTheEnvironment(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("envcreate"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		name := e.Name("env")

		created := harness.Do[environments.Output](s, actionEnvironmentCreate, map[string]any{
			"project_id": project.IDParam(), "name": name, "description": "created by the e2e suite",
		})
		if created.ID == 0 || created.Name != name || created.Slug == "" {
			e.T.Errorf("environment create answered %+v, want the environment %q with an ID and a slug", created, name)
		}
	})
}
