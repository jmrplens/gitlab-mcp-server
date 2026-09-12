//go:build e2e

// pipelines_test.go covers the creation of a pipeline through the server,
// which the old suite made only to build the pipeline its vulnerability
// scenario stood on. The pipeline is waited for, so it never contends with
// the next test's for the one runner the Docker stack registers.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestPipeline_Create_RunsOnTheDefaultBranch creates a pipeline on every
// surface in a shared project carrying the e2e CI configuration, reads the
// ref and the commit off the answer, and waits for it to finish.
//
// Replaces: TestMeta_VulnerabilityLifecycle
func TestPipeline_Create_RunsOnTheDefaultBranch(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner), harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("pipecreate"))
		fixture.CIFile(e, project)
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		created := harness.Do[pipelines.DetailOutput](s, actionPipelineCreate, map[string]any{"project_id": project.IDParam(), "ref": project.DefaultBranch})
		if created.ID == 0 || created.Ref != project.DefaultBranch || created.SHA == "" {
			e.T.Fatalf("pipeline create answered %+v, want a pipeline on %q with an ID and a commit", created, project.DefaultBranch)
		}
		e.T.Logf("pipeline %d ended in %q", created.ID, fixture.WaitForPipeline(e, project, created.ID, 0))
	})
}
