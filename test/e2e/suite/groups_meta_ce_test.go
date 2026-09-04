//go:build e2e && !enterprise

// groups_meta_ce_test.go tests advanced CE/common gitlab_group meta-tool actions
// against a live GitLab instance.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
)

// TestMeta_GroupDeep exercises gitlab_group meta-tool actions not covered
// by groups_ce_test.go, group_labels_ce_test.go, or group_milestones_ce_test.go.
//
// The test creates a parent group fixture and a child subgroup, then
// drives the deep group actions (transfer, search, restore, and other
// catalog actions) through the catalog-backed gitlab_group meta-tool.
// Each subtest asserts the expected ID or path round-trips through the
// GitLab API.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	grpName := uniqueName("grp-deep")
	var groupID int64
	var groupIDStr string
	t.Run("CreateGroup", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name": grpName,
				"path": grpName,
			},
		})
		requireNoError(t, err, "group create")
		groupID = out.ID
		groupIDStr = strconv.FormatInt(groupID, 10)
		t.Logf("Created group %d: %s", groupID, grpName)
	})
	defer func() {
		if groupID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete",
				"params": map[string]any{"group_id": groupIDStr},
			})
		}
	}()

	runMetaGroupCoreOperations(t, ctx, grpName, groupID, groupIDStr)
	runMetaGroupBadgeOperations(t, ctx, groupID, groupIDStr)
	runMetaGroupMemberChecks(t, ctx, groupID, groupIDStr)
	runMetaGroupLabelOperations(t, ctx, groupID, groupIDStr)
	runMetaGroupMilestoneOperations(t, ctx, groupID, groupIDStr)
	runMetaGroupBoardOperations(t, ctx, groupID, groupIDStr)
	runMetaGroupEnterpriseOperations(t, ctx, grpName, groupID, groupIDStr)
}
