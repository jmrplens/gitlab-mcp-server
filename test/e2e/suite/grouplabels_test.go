//go:build e2e

// grouplabels_test.go tests the group label MCP tools against a live GitLab instance.
// Exercises create, list, and delete via the gitlab_group meta-tool.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestMeta_GroupLabels exercises group label CRUD via the gitlab_group meta-tool.
func TestMeta_GroupLabels(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a dedicated group for this test.
	groupPath := fmt.Sprintf("e2e-grplbl-%d", time.Now().UnixMilli())
	grp, grpErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       groupPath,
			"path":       groupPath,
			"visibility": "public",
		},
	})
	requireNoError(t, grpErr, "create group for labels")
	requireTruef(t, grp.ID > 0, "group ID should be positive")
	groupID := grp.ID
	t.Logf("Created group %d (%s) for label tests", groupID, grp.FullPath)

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
		})
	})

	gid := strconv.FormatInt(groupID, 10)
	var labelID int64

	t.Run("Meta/GroupLabel/Create", func(t *testing.T) {
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_create",
			"params": map[string]any{
				"group_id": gid,
				"name":     "e2e-group-label",
				"color":    "#FF0000",
			},
		})
		requireNoError(t, err, "group label create")
		requireTruef(t, out.ID > 0, "expected positive group label ID")
		labelID = out.ID
		t.Logf("Created group label %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/GroupLabel/List", func(t *testing.T) {
		requireTruef(t, labelID > 0, "labelID not set")
		out, err := callToolOn[grouplabels.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_list",
			"params": map[string]any{
				"group_id": gid,
			},
		})
		requireNoError(t, err, "group label list")
		requireTruef(t, len(out.Labels) >= 1, "expected at least 1 group label")
		t.Logf("Listed %d group label(s)", len(out.Labels))
	})

	t.Run("Meta/GroupLabel/Delete", func(t *testing.T) {
		requireTruef(t, labelID > 0, "labelID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_delete",
			"params": map[string]any{
				"group_id": gid,
				"label_id": fmt.Sprintf("%d", labelID),
			},
		})
		requireNoError(t, err, "group label delete")
		t.Logf("Deleted group label %d", labelID)
	})
}
