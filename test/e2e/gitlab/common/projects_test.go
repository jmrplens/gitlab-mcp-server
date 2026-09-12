//go:build e2e

// projects_test.go covers a project's own lifecycle through the server:
// create one with a README, read it, change its description, delete it
// and check what the read answers afterwards. The old suite made these
// calls before nearly every scenario, to build the project that scenario
// stood on, and never as a scenario of their own.

package common

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestProject_Lifecycle_CreateGetUpdateDelete creates a private project on
// every surface, reads it back, changes its description, deletes it and
// checks the read is refused or answers a project marked for deletion. The
// fixture library's deletion is registered behind the server's, for a
// delete that only marked the project.
//
// Replaces: TestMeta_EpicIssues, TestIndividual_PushRules, TestMeta_IssueWorkItems
func TestProject_Lifecycle_CreateGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("proj")

		created := harness.Do[projects.Output](s, actionProjectCreate, map[string]any{
			"name": name, "visibility": "private", "initialize_with_readme": true, "default_branch": fixture.DefaultBranch,
		})
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("project create answered %+v, want a project named %q with an ID", created, name)
		}
		e.Defer("project "+created.PathWithNamespace, func(ctx context.Context) error {
			return fixture.DeleteProject(ctx, e.Client(), created.ID, created.PathWithNamespace)
		})
		project := fixture.Project{ID: created.ID, Path: created.PathWithNamespace, Name: created.Name, DefaultBranch: fixture.DefaultBranch}
		params := map[string]any{"project_id": project.IDParam()}

		got := harness.Do[projects.Output](s, actionProjectGet, params)
		if got.ID != created.ID || got.PathWithNamespace != created.PathWithNamespace {
			e.T.Errorf("project get answered %d at %q, want %d at %q", got.ID, got.PathWithNamespace, created.ID, created.PathWithNamespace)
		}

		updated := harness.Do[projects.Output](s, actionProjectUpdate, withParams(params, map[string]any{"description": "updated by the e2e suite"}))
		if updated.ID != created.ID || updated.Description != "updated by the e2e suite" {
			e.T.Errorf("project update answered %+v, want project %d with the new description", updated, created.ID)
		}

		// The delete answers what the instance did: removed it, or marked it
		// for a delayed deletion, which the read afterwards agrees with.
		deleted := harness.Do[projects.DeleteOutput](s, actionProjectDelete, params)
		if deleted.Status != "success" && deleted.Status != "scheduled" {
			e.T.Errorf("project delete answered %+v, want a success or a scheduled deletion", deleted)
		}
		remaining, err := harness.Try[projects.Output](s, actionProjectGet, params)
		switch {
		case err != nil && deleted.Status == "scheduled":
			e.T.Errorf("the delete reported a deletion scheduled on %s, and the read answers: %v", deleted.MarkedForDeletionOn, err)
		case err == nil && remaining.MarkedForDeletionOn == "":
			e.T.Errorf("the project still reads as %+v after its delete, and is not marked for deletion", remaining)
		case err == nil:
			e.T.Logf("the delete marked the project for deletion on %s", remaining.MarkedForDeletionOn)
		}
	})
}
