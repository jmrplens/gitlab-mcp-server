//go:build e2e

// groups_test.go tests the group MCP tools against a live GitLab instance.
// Covers create, list, get, members, subgroups, and delete for both individual and meta-tool modes.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestIndividual_Groups exercises group CRUD using individual MCP tools.
func TestIndividual_Groups(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	groupPath := fmt.Sprintf("e2e-grp-%d", time.Now().UnixMilli())
	var groupID int64

	t.Cleanup(func() {
		if groupID > 0 {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanCancel()
			_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
				GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
			})
		}
	})

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_create", groups.CreateInput{
			Name:       groupPath,
			Path:       groupPath,
			Visibility: "public",
		})
		requireNoError(t, err, "group create")
		requireTrue(t, out.ID > 0, "group ID should be positive")
		groupID = out.ID
		t.Logf("Created group %d (%s)", out.ID, out.FullPath)
	})

	t.Run("List", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListOutput](ctx, sess.individual, "gitlab_group_list", groups.ListInput{
			Search: groupPath,
		})
		requireNoError(t, err, "group list")
		requireTrue(t, len(out.Groups) > 0, "expected at least 1 group")
		t.Logf("Found %d groups matching %q", len(out.Groups), groupPath)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.Output](ctx, sess.individual, "gitlab_group_get", groups.GetInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group get")
		requireTrue(t, out.ID == groupID, "expected group ID %d, got %d", groupID, out.ID)
		t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
	})

	t.Run("MembersList", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.MemberListOutput](ctx, sess.individual, "gitlab_group_members_list", groups.MembersListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group members list")
		t.Logf("Group %d has %d members", groupID, len(out.Members))
	})

	t.Run("SubgroupsList", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		out, err := callToolOn[groups.ListOutput](ctx, sess.individual, "gitlab_subgroups_list", groups.SubgroupsListInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "subgroups list")
		t.Logf("Group %d has %d subgroups", groupID, len(out.Groups))
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		gid := strconv.FormatInt(groupID, 10)
		err := callToolVoidOn(ctx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(gid),
		})
		requireNoError(t, err, "group delete")
		t.Logf("Deleted group %d", groupID)
		groupID = 0
	})
}

// TestMeta_Groups exercises group operations using the gitlab_group meta-tool.
func TestMeta_Groups(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	groupPath := fmt.Sprintf("e2e-meta-grp-%d", time.Now().UnixMilli())
	var groupID int64

	t.Cleanup(func() {
		if groupID > 0 {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanCancel()
			_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete",
				"params": map[string]any{
					"group_id": strconv.FormatInt(groupID, 10),
				},
			})
		}
	})

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name":       groupPath,
				"path":       groupPath,
				"visibility": "public",
			},
		})
		requireNoError(t, err, "meta group create")
		requireTrue(t, out.ID > 0, "group ID should be positive")
		groupID = out.ID
		t.Logf("Created group %d via meta-tool (%s)", out.ID, out.FullPath)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "list",
			"params": map[string]any{
				"search": groupPath,
			},
		})
		requireNoError(t, err, "meta group list")
		requireTrue(t, len(out.Groups) > 0, "expected at least 1 group")
		t.Logf("Found %d groups via meta-tool", len(out.Groups))
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "get",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta group get")
		requireTrue(t, out.ID == groupID, "expected group ID %d, got %d", groupID, out.ID)
		t.Logf("Group %d: %s (visibility=%s)", out.ID, out.FullPath, out.Visibility)
	})

	t.Run("MembersList", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.MemberListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "members",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta group members list")
		t.Logf("Group %d has %d members via meta-tool", groupID, len(out.Members))
	})

	t.Run("SubgroupsList", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "subgroups",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta subgroups list")
		t.Logf("Group %d has %d subgroups via meta-tool", groupID, len(out.Groups))
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"group_id": strconv.FormatInt(groupID, 10),
			},
		})
		requireNoError(t, err, "meta group delete")
		t.Logf("Deleted group %d via meta-tool", groupID)
		groupID = 0
	})
}
