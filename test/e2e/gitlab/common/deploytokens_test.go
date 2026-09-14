//go:build e2e

// deploytokens_test.go covers deploy tokens at the three scopes the access
// tool serves them: a project's and a group's, each from creation to
// deletion with the read by ID and the listing between, and the instance's,
// whose listing an administrator reads and finds a project token in.

package common

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// deployTokenScope is the scope every deploy token minted here carries.
const deployTokenScope = "read_repository"

// TestDeployTokens_Project_CreateGetListDelete mints one deploy token per
// surface on a project of the test's own, reads it by ID, finds it in the
// project's listing, deletes it and checks the listing no longer holds it.
//
// Replaces: TestMeta_DeployTokens, TestMeta_AccessDeployTokens
func TestDeployTokens_Project_CreateGetListDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("dtok"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		scope := map[string]any{"project_id": project.IDParam()}
		created := createProjectDeployToken(e, s, project)
		token := withParams(scope, map[string]any{"deploy_token_id": created.ID})

		got := harness.Do[deploytokens.Output](s, actionAccessDeployTokenGetProject, token)
		if got.ID != created.ID || got.Name != created.Name {
			e.T.Errorf("deploy_token_get_project answered %d %q, want %d %q", got.ID, got.Name, created.ID, created.Name)
		}
		listed := harness.Do[deploytokens.ListOutput](s, actionAccessDeployTokenListProject, scope)
		if !containsID(deployTokenIDs(listed.DeployTokens), created.ID) {
			e.T.Errorf("the deploy tokens of project %d do not hold %d: %+v", project.ID, created.ID, listed.DeployTokens)
		}

		harness.DoVoid(s, actionAccessDeployTokenDeleteProject, token)
		after := harness.Do[deploytokens.ListOutput](s, actionAccessDeployTokenListProject, scope)
		if containsID(deployTokenIDs(after.DeployTokens), created.ID) {
			e.T.Errorf("deploy token %d is still listed after its delete", created.ID)
		}
	})
}

// TestDeployTokens_Instance_ListsAProjectsToken mints one deploy token per
// surface on a project of the test's own and finds it in the instance-wide
// listing, which answers administrators only.
//
// Replaces: TestMeta_AccessDeployTokens
func TestDeployTokens_Instance_ListsAProjectsToken(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("dtokall"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		created := createProjectDeployToken(e, s, project)

		// The instance listing takes no parameter, so it is the first page of
		// every live token; the ones earlier tests minted are deleted with
		// their projects, which keeps that page short.
		everywhere := harness.Do[deploytokens.ListOutput](s, actionAccessDeployTokenListAll, nil)
		if !containsID(deployTokenIDs(everywhere.DeployTokens), created.ID) {
			e.T.Errorf("the instance's deploy tokens do not hold %d of project %d: %d listed", created.ID, project.ID, len(everywhere.DeployTokens))
		}
	})
}

// createProjectDeployToken mints a deploy token on a project through the
// session, checks the answer carries the secret that is shown only once,
// and registers its deletion through client-go behind whatever the scenario
// does with it.
func createProjectDeployToken(e *harness.Env, s *harness.Session, project fixture.Project) deploytokens.Output {
	e.T.Helper()

	name := e.Name("dtok")
	created := harness.Do[deploytokens.Output](s, actionAccessDeployTokenCreateProject, map[string]any{
		"project_id": project.IDParam(), "name": name, "scopes": []string{deployTokenScope},
	})
	if created.ID == 0 || created.Token == "" || created.Name != name {
		e.T.Fatalf("deploy_token_create_project answered %+v, want a token named %q with an ID and its secret", created, name)
	}
	e.Defer("deploy token "+name, func(ctx context.Context) error {
		_, err := e.Client().GL().DeployTokens.DeleteProjectDeployToken(project.ID, created.ID, gl.WithContext(ctx))
		if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
			return err
		}
		return nil
	})
	return created
}

// TestDeployTokens_Group_CreateGetListDelete mints one deploy token per
// surface on a group of the test's own, reads it by ID, finds it in the
// group's listing, deletes it and checks the listing no longer holds it.
//
// Replaces: TestIndividual_GroupDeployTokens
func TestDeployTokens_Group_CreateGetListDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("dtok"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		scope := map[string]any{"group_id": group.IDParam()}
		name := e.Name("gdtok")

		created := harness.Do[deploytokens.Output](s, actionAccessDeployTokenCreateGroup, withParams(scope, map[string]any{"name": name, "scopes": []string{deployTokenScope}}))
		if created.ID == 0 || created.Token == "" || created.Name != name {
			e.T.Fatalf("deploy_token_create_group answered %+v, want a token named %q with an ID and its secret", created, name)
		}
		e.Defer("group deploy token "+name, func(ctx context.Context) error {
			_, err := e.Client().GL().DeployTokens.DeleteGroupDeployToken(group.ID, created.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})
		token := withParams(scope, map[string]any{"deploy_token_id": created.ID})

		got := harness.Do[deploytokens.Output](s, actionAccessDeployTokenGetGroup, token)
		if got.ID != created.ID || got.Name != name {
			e.T.Errorf("deploy_token_get_group answered %d %q, want %d %q", got.ID, got.Name, created.ID, name)
		}
		listed := harness.Do[deploytokens.ListOutput](s, actionAccessDeployTokenListGroup, scope)
		if !containsID(deployTokenIDs(listed.DeployTokens), created.ID) {
			e.T.Errorf("the deploy tokens of group %d do not hold %d: %+v", group.ID, created.ID, listed.DeployTokens)
		}

		harness.DoVoid(s, actionAccessDeployTokenDeleteGroup, token)
		after := harness.Do[deploytokens.ListOutput](s, actionAccessDeployTokenListGroup, scope)
		if containsID(deployTokenIDs(after.DeployTokens), created.ID) {
			e.T.Errorf("group deploy token %d is still listed after its delete", created.ID)
		}
	})
}

// deployTokenIDs collects the IDs of listed deploy tokens.
func deployTokenIDs(listed []deploytokens.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, token := range listed {
		ids = append(ids, token.ID)
	}
	return ids
}
