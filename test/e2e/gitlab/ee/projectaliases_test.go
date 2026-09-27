//go:build e2e

// projectaliases_test.go covers the project alias lifecycle: create one for
// a project, read it, find it in the listing, delete it, and show the read
// of a deleted alias is refused with the hint naming the listing.

package ee

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// aliasListed reports whether a listing holds an alias by name and project.
func aliasListed(aliases []projectaliases.Output, name string, projectID int64) bool {
	for _, alias := range aliases {
		if alias.Name == name && alias.ProjectID == projectID {
			return true
		}
	}
	return false
}

// TestProjectAliases_Lifecycle_CreatesReadsListsAndDeletes walks one alias
// per surface through its life on a shared project. Aliases are an
// administrator's, so the run's token must be one.
//
// Replaces: TestMeta_ProjectAliases
func TestProjectAliases_Lifecycle_CreatesReadsListsAndDeletes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("alias"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		name := e.Name("alias")

		before := harness.Do[projectaliases.ListOutput](s, actionProjectAliasList, nil)
		if aliasListed(before.Aliases, name, project.ID) {
			e.T.Fatalf("alias %q exists before the create", name)
		}

		created := harness.Do[projectaliases.Output](s, actionProjectAliasCreate, map[string]any{"name": name, "project_id": project.ID})
		if created.Name != name || created.ProjectID != project.ID {
			e.T.Fatalf("create answered %+v, want alias %q for project %d", created, name, project.ID)
		}
		e.Defer("project alias "+name, func(ctx context.Context) error {
			_, err := e.Client().GL().ProjectAliases.DeleteProjectAlias(name, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		got := harness.Do[projectaliases.Output](s, actionProjectAliasGet, map[string]any{"name": name})
		if got.Name != name || got.ProjectID != project.ID {
			e.T.Errorf("get answered %+v, want alias %q for project %d", got, name, project.ID)
		}

		after := harness.Do[projectaliases.ListOutput](s, actionProjectAliasList, nil)
		if !aliasListed(after.Aliases, name, project.ID) {
			e.T.Errorf("the listing does not hold the created alias %q: %+v", name, after.Aliases)
		}

		// A second alias makes the instance's listing at least two pages long
		// at one alias per page. The listing used to claim it returned the
		// full set; GitLab answers twenty and stops. This scenario is the only
		// one that creates an alias and its surfaces run one after another,
		// so nothing moves an alias between the two reads.
		pageName := e.Name("alias-page")
		if _, _, err := e.Client().GL().ProjectAliases.CreateProjectAlias(&gl.CreateProjectAliasOptions{
			Name: new(pageName), ProjectID: project.ID,
		}, gl.WithContext(e.Ctx)); err != nil {
			e.T.Fatalf("creating the alias %q: %v", pageName, err)
		}
		e.Defer("project alias "+pageName, func(ctx context.Context) error {
			_, err := e.Client().GL().ProjectAliases.DeleteProjectAlias(pageName, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})
		assertPagesOneAtATime(e, s, actionProjectAliasList, nil, func(out projectaliases.ListOutput) ([]string, toolutil.PaginationOutput) {
			ids := make([]int64, 0, len(out.Aliases))
			for _, alias := range out.Aliases {
				ids = append(ids, alias.ID)
			}
			return idKeys(ids), out.Pagination
		})

		harness.DoVoid(s, actionProjectAliasDelete, map[string]any{"name": name})
		refused := harness.Refused(s, actionProjectAliasGet, map[string]any{"name": name}, harness.FailureNotFound)
		assertMentions(e, "the read of a deleted alias", refused, "alias", "project_alias.list")
	})
}
