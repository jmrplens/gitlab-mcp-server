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

		harness.DoVoid(s, actionProjectAliasDelete, map[string]any{"name": name})
		refused := harness.Refused(s, actionProjectAliasGet, map[string]any{"name": name}, harness.FailureNotFound)
		assertMentions(e, "the read of a deleted alias", refused, "alias", "gitlab_list_project_aliases")
	})
}
