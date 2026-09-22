//go:build e2e

// mcp_discover_project_test.go covers the project discovery utility on all
// three surfaces: a git remote URL, as a client reads it from .git/config,
// resolved to the project it names.
//
// It is a standalone utility, a tool of its own on meta and individual and an
// action of the execute tool on dynamic, and until issue 903 the harness could
// reach it on the dynamic surface alone, so the other two were never exercised
// by anything the coverage record could credit.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestDiscoverProject_Resolve_FindsTheProjectOnEverySurface resolves a project's
// own clone URL, the one GitLab published for it, and holds the answer to that
// project.
func TestDiscoverProject_Resolve_FindsTheProjectOnEverySurface(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		project := fixture.NewProject(e, fixture.WithNamePrefix("discover"))
		if project.HTTPURLToRepo == "" {
			e.T.Fatal("GitLab published no clone URL for the project, so there is no remote to resolve")
		}
		return project
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		resolved := harness.Do[projectdiscovery.ResolveOutput](e.On(surface), actionDiscoverProjectResolve,
			map[string]any{"remote_url": project.HTTPURLToRepo})

		if resolved.ID != project.ID || resolved.PathWithNamespace != project.Path {
			e.T.Errorf("the remote %s resolved to %d (%s), want %d (%s)",
				project.HTTPURLToRepo, resolved.ID, resolved.PathWithNamespace, project.ID, project.Path)
		}
		if resolved.ExtractedPath != project.Path {
			e.T.Errorf("the path read off the remote is %q, want %q", resolved.ExtractedPath, project.Path)
		}
	})
}
