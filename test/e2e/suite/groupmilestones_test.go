//go:build e2e

package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestMeta_GroupMilestones exercises group milestone CRUD via the gitlab_group meta-tool.
func TestMeta_GroupMilestones(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a dedicated group for this test.
	groupPath := fmt.Sprintf("e2e-grpms-%d", time.Now().UnixMilli())
	grp, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       groupPath,
			"path":       groupPath,
			"visibility": "public",
		},
	})
	requireNoError(t, err, "create group for milestones")
	requireTrue(t, grp.ID > 0, "group ID should be positive")
	groupID := grp.ID
	t.Logf("Created group %d (%s) for milestone tests", groupID, grp.FullPath)

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
		})
	})

	gid := strconv.FormatInt(groupID, 10)
	var milestoneIID int64

	t.Run("Meta/GroupMilestone/Create", func(t *testing.T) {
		out, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_create",
			"params": map[string]any{
				"group_id": gid,
				"title":    "e2e-group-milestone",
			},
		})
		requireNoError(t, err, "group milestone create")
		requireTrue(t, out.IID > 0, "expected positive group milestone IID")
		milestoneIID = out.IID
		t.Logf("Created group milestone %s (IID=%d)", out.Title, out.IID)
	})

	t.Run("Meta/GroupMilestone/List", func(t *testing.T) {
		requireTrue(t, milestoneIID > 0, "milestoneIID not set")
		out, err := callToolOn[groupmilestones.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_list",
			"params": map[string]any{
				"group_id": gid,
			},
		})
		requireNoError(t, err, "group milestone list")
		requireTrue(t, len(out.Milestones) >= 1, "expected at least 1 group milestone")
		t.Logf("Listed %d group milestone(s)", len(out.Milestones))
	})

	t.Run("Meta/GroupMilestone/Get", func(t *testing.T) {
		requireTrue(t, milestoneIID > 0, "milestoneIID not set")
		out, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_get",
			"params": map[string]any{
				"group_id":      gid,
				"milestone_iid": milestoneIID,
			},
		})
		requireNoError(t, err, "group milestone get")
		requireTrue(t, out.IID == milestoneIID, "group milestone IID mismatch")
		t.Logf("Got group milestone %s (IID=%d)", out.Title, out.IID)
	})

	t.Run("Meta/GroupMilestone/Delete", func(t *testing.T) {
		requireTrue(t, milestoneIID > 0, "milestoneIID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_delete",
			"params": map[string]any{
				"group_id":      gid,
				"milestone_iid": milestoneIID,
			},
		})
		requireNoError(t, err, "group milestone delete")
		t.Logf("Deleted group milestone IID=%d", milestoneIID)
	})
}
