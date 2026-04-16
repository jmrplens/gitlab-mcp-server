//go:build e2e

package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
)

// TestMeta_GroupDeep exercises gitlab_group meta-tool actions not covered by
// groups_test.go, grouplabels_test.go, or groupmilestones_test.go.
func TestMeta_GroupDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a group for testing
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

	// ── Core operations ──────────────────────────────────────────────────
	t.Run("Update", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "update",
			"params": map[string]any{
				"group_id":    groupIDStr,
				"description": "Deep test group",
			},
		})
		requireNoError(t, err, "group update")
		t.Logf("Updated group %d", out.ID)
	})

	t.Run("Search", func(t *testing.T) {
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "search",
			"params": map[string]any{"query": grpName},
		})
		requireNoError(t, err, "group search")
		requireTrue(t, len(out.Groups) >= 1, "expected at least 1 group in search")
		t.Logf("Search found %d groups", len(out.Groups))
	})

	t.Run("Projects", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListProjectsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "projects",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "group projects")
		t.Logf("Group has %d projects", len(out.Projects))
	})

	// ── Hooks ────────────────────────────────────────────────────────────
	var hookID int64
	t.Run("HookAdd", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_add",
			"params": map[string]any{
				"group_id":    groupIDStr,
				"url":         "https://example.com/hook",
				"push_events": true,
			},
		})
		requireNoError(t, err, "hook_add")
		hookID = out.ID
		t.Logf("Added hook %d", hookID)
	})

	t.Run("HookList", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.HookListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "hook_list")
		requireTrue(t, len(out.Hooks) >= 1, "expected at least 1 hook")
		t.Logf("Listed %d hooks", len(out.Hooks))
	})

	t.Run("HookGet", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTrue(t, hookID > 0, "hookID not set")
		out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_get",
			"params": map[string]any{"group_id": groupIDStr, "hook_id": hookID},
		})
		requireNoError(t, err, "hook_get")
		requireTrue(t, out.ID == hookID, "hook ID mismatch")
		t.Logf("Got hook %d", out.ID)
	})

	t.Run("HookEdit", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTrue(t, hookID > 0, "hookID not set")
		out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_edit",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"hook_id":       hookID,
				"url":           "https://example.com/hook-updated",
				"issues_events": true,
			},
		})
		requireNoError(t, err, "hook_edit")
		t.Logf("Edited hook %d", out.ID)
	})

	t.Run("HookDelete", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTrue(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_delete",
			"params": map[string]any{"group_id": groupIDStr, "hook_id": hookID},
		})
		requireNoError(t, err, "hook_delete")
		t.Logf("Deleted hook %d", hookID)
	})

	// ── Badges ───────────────────────────────────────────────────────────
	var badgeID int64
	t.Run("BadgeAdd", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[badges.AddGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_add",
			"params": map[string]any{
				"group_id":  groupIDStr,
				"link_url":  "https://example.com/badge",
				"image_url": "https://example.com/badge.svg",
			},
		})
		requireNoError(t, err, "badge_add")
		badgeID = out.Badge.ID
		t.Logf("Added group badge %d", badgeID)
	})

	t.Run("BadgeList", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[badges.ListGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "badge_list")
		requireTrue(t, len(out.Badges) >= 1, "expected at least 1 badge")
		t.Logf("Listed %d badges", len(out.Badges))
	})

	t.Run("BadgeGet", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.GetGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_get",
			"params": map[string]any{"group_id": groupIDStr, "badge_id": badgeID},
		})
		requireNoError(t, err, "badge_get")
		requireTrue(t, out.Badge.ID == badgeID, "badge ID mismatch")
		t.Logf("Got badge %d", out.Badge.ID)
	})

	t.Run("BadgeEdit", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.EditGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_edit",
			"params": map[string]any{
				"group_id":  groupIDStr,
				"badge_id":  badgeID,
				"link_url":  "https://example.com/badge-updated",
				"image_url": "https://example.com/badge-updated.svg",
			},
		})
		requireNoError(t, err, "badge_edit")
		t.Logf("Edited badge %d", out.Badge.ID)
	})

	t.Run("BadgePreview", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[badges.PreviewGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_preview",
			"params": map[string]any{
				"group_id":  groupIDStr,
				"link_url":  "https://example.com/badge",
				"image_url": "https://example.com/badge.svg",
			},
		})
		requireNoError(t, err, "badge_preview")
		t.Logf("Preview rendered: %s", out.Badge.RenderedLinkURL)
	})

	t.Run("BadgeDelete", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_delete",
			"params": map[string]any{"group_id": groupIDStr, "badge_id": badgeID},
		})
		requireNoError(t, err, "badge_delete")
		t.Logf("Deleted badge %d", badgeID)
	})

	// ── Members ──────────────────────────────────────────────────────────
	t.Run("GroupMemberGet", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		// User ID 1 (root) is NOT a member of a freshly created group by e2e-tester
		_, err := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_get",
			"params": map[string]any{
				"group_id": groupIDStr,
				"user_id":  "1",
			},
		})
		requireTrue(t, err != nil, "expected error: user 1 is not a member of the group")
		t.Logf("Expected error for non-member user: %v", err)
	})

	t.Run("GroupMemberGetInherited", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		// Standalone group has no inherited members
		_, err := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_get_inherited",
			"params": map[string]any{
				"group_id": groupIDStr,
				"user_id":  "1",
			},
		})
		requireTrue(t, err != nil, "expected error: user 1 is not an inherited member")
		t.Logf("Expected error for non-inherited member: %v", err)
	})

	// ── Labels deep ──────────────────────────────────────────────────────
	var labelName string
	t.Run("LabelCreate", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		labelName = uniqueName("lbl")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     labelName,
				"color":    "#FF0000",
			},
		})
		requireNoError(t, err, "label create")
		t.Logf("Created label: %s (ID=%d)", out.Name, out.ID)
	})

	t.Run("LabelList", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		out, err := callToolOn[grouplabels.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "label list")
		requireTrue(t, len(out.Labels) >= 1, "expected at least 1 label")
		t.Logf("Listed %d labels", len(out.Labels))
	})

	t.Run("LabelGet", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_get",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label get")
		t.Logf("Got label: %s", out.Name)
	})

	t.Run("LabelUpdate", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_update",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
				"new_name": labelName + "-upd",
				"color":    "#00FF00",
			},
		})
		requireNoError(t, err, "label update")
		labelName = out.Name
		t.Logf("Updated label: %s", labelName)
	})

	t.Run("LabelSubscribe", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_subscribe",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label subscribe")
		t.Logf("Subscribed to label: %s", out.Name)
	})

	t.Run("LabelUnsubscribe", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_unsubscribe",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label unsubscribe")
		t.Log("Unsubscribed from label")
	})

	t.Run("LabelDelete", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_delete",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label delete")
		t.Log("Deleted label")
	})

	// ── Milestones deep ──────────────────────────────────────────────────
	var milestoneID int64
	t.Run("MilestoneGet", func(t *testing.T) {
		requireTrue(t, groupID > 0, "groupID not set")
		// Create a milestone to get
		out, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"title":    uniqueName("ms-deep"),
			},
		})
		requireNoError(t, err, "milestone create")
		milestoneID = out.IID

		got, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_get",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "milestone get")
		requireTrue(t, got.IID == milestoneID, "milestone IID mismatch")
		t.Logf("Got milestone IID %d: %s", got.IID, got.Title)
	})

	t.Run("MilestoneUpdate", func(t *testing.T) {
		requireTrue(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_update",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"milestone_iid": milestoneID,
				"description":  "Updated milestone",
			},
		})
		requireNoError(t, err, "milestone update")
		t.Logf("Updated milestone %d", out.ID)
	})

	t.Run("MilestoneIssues", func(t *testing.T) {
		requireTrue(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[groupmilestones.IssuesOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_issues",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "milestone issues")
		t.Logf("Milestone has %d issues", len(out.Issues))
	})

	t.Run("MilestoneMergeRequests", func(t *testing.T) {
		requireTrue(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[groupmilestones.MergeRequestsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_merge_requests",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "milestone merge_requests")
		t.Logf("Milestone has %d MRs", len(out.MergeRequests))
	})

	t.Run("MilestoneBurndown", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTrue(t, milestoneID > 0, "milestoneID not set")
		_, err := callToolOn[groupmilestones.BurndownChartEventsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_burndown",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "group_milestone_burndown")
		t.Log("Got burndown chart events")
	})

	// ── Boards (Premium/Ultimate) ────────────────────────────────────────
	if sess.enterprise {
		var boardID int64

		t.Run("BoardCreate", func(t *testing.T) {
			requireTrue(t, groupID > 0, "groupID not set")
			out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_create",
				"params": map[string]any{
					"group_id": groupIDStr,
					"name":     "Test Board",
				},
			})
			requireNoError(t, err, "group_board_create")
			boardID = out.ID
			t.Logf("Created group board %d", boardID)
		})

		t.Run("BoardList", func(t *testing.T) {
			requireTrue(t, groupID > 0, "groupID not set")
			out, err := callToolOn[groupboards.ListGroupBoardsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_list",
				"params": map[string]any{"group_id": groupIDStr},
			})
			requireNoError(t, err, "group_board_list")
			requireTrue(t, len(out.Boards) > 0, "expected at least 1 group board")
			t.Logf("Listed %d group boards", len(out.Boards))
			if boardID == 0 && len(out.Boards) > 0 {
				boardID = out.Boards[0].ID
			}
		})

		t.Run("BoardGet", func(t *testing.T) {
			if boardID == 0 {
				return
			}
			out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_get",
				"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
			})
			requireNoError(t, err, "board_get")
			requireTrue(t, out.ID == boardID, "board ID mismatch")
			t.Logf("Got board %d: %s", out.ID, out.Name)
		})

		t.Run("BoardUpdate", func(t *testing.T) {
			if boardID == 0 {
				return
			}
			out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_update",
				"params": map[string]any{
					"group_id": groupIDStr,
					"board_id": boardID,
					"name":     "Updated Board",
				},
			})
			requireNoError(t, err, "group_board_update")
			t.Logf("Updated board %d: %s", out.ID, out.Name)
		})

		t.Run("BoardListLists", func(t *testing.T) {
			if boardID == 0 {
				return
			}
			out, err := callToolOn[groupboards.ListBoardListsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_list_lists",
				"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
			})
			requireNoError(t, err, "board_list_lists")
			t.Logf("Board has %d lists", len(out.Lists))
		})

		t.Run("BoardDelete", func(t *testing.T) {
			if boardID == 0 {
				return
			}
			err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_delete",
				"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
			})
			requireNoError(t, err, "group_board_delete")
			t.Logf("Deleted board %d", boardID)
		})
	}
}
