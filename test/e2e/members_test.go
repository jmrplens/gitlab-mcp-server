//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
)

func TestIndividual_Members(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[members.ListOutput](ctx, sess.individual, "gitlab_project_members_list", members.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list members")
		requireTrue(t, len(out.Members) >= 1, "expected at least 1 member (owner)")
		t.Logf("Listed %d members", len(out.Members))
	})

	t.Run("GetOwner", func(t *testing.T) {
		out, err := callToolOn[members.ListOutput](ctx, sess.individual, "gitlab_project_members_list", members.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list members for get")
		requireTrue(t, len(out.Members) >= 1, "expected members")

		ownerID := out.Members[0].ID

		member, err := callToolOn[members.Output](ctx, sess.individual, "gitlab_project_member_get", members.GetInput{
			ProjectID: proj.pidOf(),
			UserID:    ownerID,
		})
		requireNoError(t, err, "get member")
		requireTrue(t, member.ID == ownerID, "expected owner ID %d", ownerID)
		t.Logf("Got member %s (access level %d)", member.Username, member.AccessLevel)
	})
}

func TestMeta_Members(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[members.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "members",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list members")
		requireTrue(t, len(out.Members) >= 1, "expected at least 1 member")
		t.Logf("Listed %d members via meta-tool", len(out.Members))
	})

	t.Run("GetOwner", func(t *testing.T) {
		list, err := callToolOn[members.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "members",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list members for get")
		requireTrue(t, len(list.Members) >= 1, "expected members")

		ownerID := list.Members[0].ID

		member, err := callToolOn[members.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "member_get",
			"params": map[string]any{"project_id": proj.pidStr(), "user_id": ownerID},
		})
		requireNoError(t, err, "meta get member")
		requireTrue(t, member.ID == ownerID, "expected owner ID %d", ownerID)
		t.Logf("Got member %s via meta-tool", member.Username)
	})
}
