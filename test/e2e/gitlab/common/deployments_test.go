//go:build e2e

// deployments_test.go covers a deployment through the server: creating one
// into an environment with the two facts GitLab 19 insists on, the reads of
// it, the status update that completes it, the environment that carries it
// as its last deployment once it is successful, and the delete GitLab
// refuses for the last deployment of an environment.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// deploymentIDs returns the identifiers of a deployment listing.
func deploymentIDs(listed []deployments.Output) []int {
	ids := make([]int, 0, len(listed))
	for _, deployment := range listed {
		ids = append(ids, deployment.ID)
	}
	return ids
}

// assertDeploymentReads lists and reads a deployment just created.
func assertDeploymentReads(e *harness.Env, s *harness.Session, project fixture.Project, environment fixture.Environment, commit fixture.Commit, created deployments.Output) {
	e.T.Helper()
	params := map[string]any{"project_id": project.IDParam()}

	listed := harness.Do[deployments.ListOutput](s, actionEnvironmentDeploymentList, withParams(params, map[string]any{"environment": environment.Name}))
	if !slices.Contains(deploymentIDs(listed.Deployments), created.ID) {
		e.T.Errorf("the deployments of %s do not hold %d: %v", environment.Name, created.ID, deploymentIDs(listed.Deployments))
	}

	got := harness.Do[deployments.Output](s, actionEnvironmentDeploymentGet, withParams(params, map[string]any{"deployment_id": created.ID}))
	if got.ID != created.ID || got.SHA != commit.SHA || got.Ref != project.DefaultBranch || got.Status != string(fixture.DeploymentStatus) {
		e.T.Errorf("deployment get answered %+v, want deployment %d of %s in the %s status", got, created.ID, commit.ShortID, fixture.DeploymentStatus)
	}
	if got.User == nil || got.User.Username != e.Runtime().Username {
		e.T.Errorf("deployment %d carries the user %+v, want the run's own %s", created.ID, got.User, e.Runtime().Username)
	}
	if got.Environment == nil || got.Environment.Name != environment.Name {
		e.T.Errorf("deployment %d carries the environment %+v, want %s", created.ID, got.Environment, environment.Name)
	}
}

// assertEnvironmentCarriesDeployment reads the environment and asserts the
// deployment is the last one it carries.
//
// It runs after the deployment was marked successful, and not before: an
// environment's last deployment is its last *successful* one, so while the
// deployment is running the environment carries none at all.
func assertEnvironmentCarriesDeployment(e *harness.Env, s *harness.Session, project fixture.Project, environment fixture.Environment, commit fixture.Commit, created deployments.Output) {
	e.T.Helper()

	env := harness.Do[environments.Output](s, actionEnvironmentGet, map[string]any{
		"project_id": project.IDParam(), "environment_id": environment.ID,
	})
	if env.ID != environment.ID || env.LastDeployment == nil {
		e.T.Fatalf("environment get answered %+v, want environment %d with a last deployment", env, environment.ID)
	}
	if env.LastDeployment.ID != int64(created.ID) || env.LastDeployment.SHA != commit.SHA || env.LastDeployment.Ref != project.DefaultBranch {
		e.T.Errorf("the last deployment of %s is %+v, want deployment %d of %s", environment.Name, env.LastDeployment, created.ID, commit.ShortID)
	}
}

// TestDeployment_Lifecycle_CreateListGetUpdateAndRefusedDelete deploys a
// commit of a shared project into an environment of the surface's own,
// lists and reads the deployment, marks it successful, finds it as the
// environment's last deployment, and asserts GitLab refuses to delete it,
// being the environment's last deployment.
//
// Replaces: TestMeta_DeploymentsExtended, TestMeta_DeploymentsGetUpdateDelete
func TestDeployment_Lifecycle_CreateListGetUpdateAndRefusedDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("deploy"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		environment := fixture.NewEnvironment(e, project, "production")
		commit := fixture.CommitFile(e, project, project.DefaultBranch, "deploy-"+string(surface)+".txt", "deployed content", "feat: the commit to deploy")

		created := harness.Do[deployments.Output](s, actionEnvironmentDeploymentCreate, withParams(params, map[string]any{
			"environment": environment.Name, "ref": project.DefaultBranch, "sha": commit.SHA,
			"tag": fixture.DeploymentRefIsBranch, "status": string(fixture.DeploymentStatus),
		}))
		if created.ID == 0 || created.SHA != commit.SHA || created.Ref != project.DefaultBranch || created.Status != string(fixture.DeploymentStatus) {
			e.T.Fatalf("deployment create answered %+v, want a %s deployment of %s from %s with an ID", created, fixture.DeploymentStatus, commit.ShortID, project.DefaultBranch)
		}
		byID := withParams(params, map[string]any{"deployment_id": created.ID})

		assertDeploymentReads(e, s, project, environment, commit, created)

		updated := harness.Do[deployments.Output](s, actionEnvironmentDeploymentUpdate, withParams(byID, map[string]any{"status": "success"}))
		if updated.ID != created.ID || updated.Status != "success" {
			e.T.Errorf("deployment update answered %+v, want deployment %d successful", updated, created.ID)
		}
		assertEnvironmentCarriesDeployment(e, s, project, environment, commit, created)

		refused := harness.ExpectToolError(s, actionEnvironmentDeploymentDelete, byID, "deployment")
		e.T.Logf("the delete of the environment's last deployment is refused: %s", firstLine(refused))
	})
}
