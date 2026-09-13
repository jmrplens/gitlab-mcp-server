//go:build e2e

// protectedenvs_test.go covers a project's protected environments: protect,
// list, get, update an access level in place, unprotect, and the not-found
// a read gives afterwards; then the deployment approval flow a protected
// environment with an approval rule gates.
//
// The approval is asserted as the refusal GitLab makes of it: the fixture
// creates the deployment and then approves it as the same user, which
// GitLab does not allow, so the answer is deterministic without a second
// user and still goes through the whole approval path.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/protectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The access levels the protection is created with and updated to:
// Maintainer, then Developer.
const (
	accessLevelMaintainer = 40
	accessLevelDeveloper  = 30
)

// protectEnvironment protects an environment of the project with a
// Maintainer deploy access level, plus whatever extra the caller adds, and
// answers the protection.
func protectEnvironment(e *harness.Env, s *harness.Session, project fixture.Project, name string, extra map[string]any) protectedenvs.Output {
	e.T.Helper()

	params := withParams(map[string]any{
		"project_id":           project.IDParam(),
		"name":                 name,
		"deploy_access_levels": []map[string]any{{"access_level": accessLevelMaintainer}},
	}, extra)
	protected := harness.Do[protectedenvs.Output](s, actionProtectedEnvProtect, params)
	if protected.Name != name {
		e.T.Fatalf("protect answered %+v, want the environment %q", protected, name)
	}
	return protected
}

// TestProtectedEnvironments_Lifecycle_ProtectsUpdatesAndUnprotects walks
// one protected environment per surface on a shared project.
//
// Replaces: TestMeta_ProtectedEnvs, TestMeta_ProtectedEnvUpdate
func TestProtectedEnvironments_Lifecycle_ProtectsUpdatesAndUnprotects(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("penv"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		name := e.Name("staging")

		protected := protectEnvironment(e, s, project, name, nil)
		if len(protected.DeployAccessLevels) == 0 || protected.DeployAccessLevels[0].ID == 0 {
			e.T.Fatalf("protect answered no deploy access level with an ID: %+v", protected)
		}
		levelID := protected.DeployAccessLevels[0].ID

		listed := harness.Do[protectedenvs.ListOutput](s, actionProtectedEnvList, map[string]any{"project_id": project.IDParam()})
		found := false
		for _, environment := range listed.Environments {
			if environment.Name == name {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the listing does not hold the protected environment %q: %+v", name, listed.Environments)
		}

		got := harness.Do[protectedenvs.Output](s, actionProtectedEnvGet, map[string]any{"project_id": project.IDParam(), "environment": name})
		if got.Name != name {
			e.T.Errorf("get answered %+v, want the environment %q", got, name)
		}

		updated := harness.Do[protectedenvs.Output](s, actionProtectedEnvUpdate, map[string]any{
			"project_id": project.IDParam(), "environment": name,
			"deploy_access_levels": []map[string]any{{"id": levelID, "access_level": accessLevelDeveloper}},
		})
		lowered := false
		for _, level := range updated.DeployAccessLevels {
			if level.ID == levelID && level.AccessLevel == accessLevelDeveloper {
				lowered = true
			}
		}
		if updated.Name != name || !lowered {
			e.T.Errorf("update answered %+v, want access level %d of %q lowered to %d", updated, levelID, name, accessLevelDeveloper)
		}

		harness.DoVoid(s, actionProtectedEnvUnprotect, map[string]any{"project_id": project.IDParam(), "environment": name})
		refused := harness.Refused(s, actionProtectedEnvGet, map[string]any{"project_id": project.IDParam(), "environment": name}, harness.FailureNotFound)
		e.T.Logf("the read after the unprotect is refused: %s", firstLine(refused))
	})
}

// TestDeploymentApproval_ProtectedEnvironment_RefusesTheDeployer creates
// an environment, protects it with a one-approval rule naming the run's
// user, deploys to it, and asks that user to approve the deployment, which
// GitLab refuses. The refusal is the whole approval path exercised end to
// end, with the one answer a single credential can be sure of.
//
// Replaces: TestMeta_DeploymentApproveOrReject
func TestDeploymentApproval_ProtectedEnvironment_RefusesTheDeployer(t *testing.T) {
	e := harness.New(t)
	user := e.Runtime().UserID

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("deployapp"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		environment := fixture.NewEnvironment(e, project, "approval")
		protectEnvironment(e, s, project, environment.Name, map[string]any{
			"approval_rules": []map[string]any{{"user_id": user, "required_approvals": 1}},
		})
		commit := fixture.CommitFile(e, project, project.DefaultBranch, "deploy-"+string(surface)+".txt", "deployment approval fixture\n", "add the deployment approval fixture")

		deployed := harness.Do[deployments.Output](s, actionDeploymentCreate, map[string]any{
			"project_id": project.IDParam(), "environment": environment.Name, "ref": project.DefaultBranch,
			"sha": commit.SHA, "tag": fixture.DeploymentRefIsBranch, "status": string(fixture.DeploymentStatus),
		})
		if deployed.ID == 0 {
			e.T.Fatalf("deployment_create answered %+v, want a deployment with an ID", deployed)
		}

		refused := harness.ExpectToolError(s, actionDeploymentApproveOrReject, map[string]any{
			"project_id": project.IDParam(), "deployment_id": deployed.ID, "status": "approved", "comment": "e2e deployment approval",
		}, "deployment")
		e.T.Logf("approving deployment %d (%s) as its deployer is refused: %s", deployed.ID, deployed.Status, firstLine(refused))
	})
}
