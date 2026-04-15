//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestIndividual_UserProjects exercises user contributed/starred project listing via individual tools.
func TestIndividual_UserProjects(t *testing.T) {
	ctx := context.Background()
	user := os.Getenv("GITLAB_USER")
	if user == "" {
		t.Skip("GITLAB_USER not set")
	}

	t.Run("Individual/User/ContributedProjects", func(t *testing.T) {
		out, err := callToolOn[projects.ListOutput](ctx, sess.individual, "gitlab_project_list_user_contributed", projects.ListUserContributedProjectsInput{
			UserID: toolutil.StringOrInt(user),
		})
		requireNoError(t, err, "list user contributed projects")
		t.Logf("User %s contributed to %d projects", user, len(out.Projects))
	})

	t.Run("Individual/User/StarredProjects", func(t *testing.T) {
		out, err := callToolOn[projects.ListOutput](ctx, sess.individual, "gitlab_project_list_user_starred", projects.ListUserStarredProjectsInput{
			UserID: toolutil.StringOrInt(user),
		})
		requireNoError(t, err, "list user starred projects")
		t.Logf("User %s starred %d projects", user, len(out.Projects))
	})
}

// TestMeta_UserProjects exercises user contributed/starred project listing via the gitlab_project meta-tool.
func TestMeta_UserProjects(t *testing.T) {
	ctx := context.Background()
	user := os.Getenv("GITLAB_USER")
	if user == "" {
		t.Skip("GITLAB_USER not set")
	}

	t.Run("Meta/User/ContributedProjects", func(t *testing.T) {
		out, err := callToolOn[projects.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_user_contributed",
			"params": map[string]any{
				"user_id": user,
			},
		})
		requireNoError(t, err, "meta list user contributed")
		t.Logf("User %s contributed to %d projects (meta)", user, len(out.Projects))
	})

	t.Run("Meta/User/StarredProjects", func(t *testing.T) {
		out, err := callToolOn[projects.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_user_starred",
			"params": map[string]any{
				"user_id": user,
			},
		})
		requireNoError(t, err, "meta list user starred")
		t.Logf("User %s starred %d projects (meta)", user, len(out.Projects))
	})
}
