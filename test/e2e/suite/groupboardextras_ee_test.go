//go:build e2e && enterprise

// groupboardextras_ee_test.go covers the gitlab_group meta-tool group board
// list (column) CRUD actions. The coverage lives behind the enterprise gate
// because provisioning a group issue board via the API is a Premium
// capability — CE instances 404 on the create route.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouplabels"
)

// TestMeta_GroupBoardListColumns exercises group_board_create_list,
// group_board_get_list, group_board_update_list, and group_board_delete_list
// through the gitlab_group meta-tool.
//
// The test creates a group and a group label, provisions an issue board
// through the raw SDK (board creation is Premium-gated on the meta surface,
// and CE allows a single board per group), then creates a label-backed
// column, fetches it, reorders it, and deletes it. Creating a group issue
// board through the API is a Premium capability, so the coverage is
// enterprise-gated; on an EE runtime provisioning must succeed.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_GroupBoardListColumns(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-board-cols")

	boardID, ok := groupExtrasEnsureBoard(ctx, t, grp)
	requireTruef(t, ok, "group issue board provisioning must succeed on an EE runtime")
	labelID := groupExtrasCreateGroupLabel(ctx, t, grp)
	secondLabelID := groupExtrasCreateGroupLabel(ctx, t, grp)

	var listID, secondListID int64

	t.Run("CreateList", func(t *testing.T) {
		out, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_create_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"label_id": labelID,
			},
		})
		requireNoError(t, err, "group_board_create_list")
		requireTruef(t, out.ID > 0, "expected positive board list ID")
		listID = out.ID
		t.Logf("Created board list %d on board %d", listID, boardID)

		// A second list backs the reorder subtest: GitLab rejects moving a
		// board's only label list ("List could not be moved!") because there
		// is nothing to reorder against.
		second, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_create_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"label_id": secondLabelID,
			},
		})
		requireNoError(t, err, "group_board_create_list (second)")
		secondListID = second.ID
	})

	t.Run("GetList", func(t *testing.T) {
		requireTruef(t, listID > 0, "listID not set")
		out, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_get_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"list_id":  listID,
			},
		})
		requireNoError(t, err, "group_board_get_list")
		requireTruef(t, out.ID == listID, "got board list %d, want %d", out.ID, listID)
		t.Logf("Got board list %d at position %d", out.ID, out.Position)
	})

	t.Run("UpdateList", func(t *testing.T) {
		requireTruef(t, listID > 0 && secondListID > 0, "list IDs not set")
		out, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_update_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"list_id":  secondListID,
				"position": 0,
			},
		})
		requireNoError(t, err, "group_board_update_list")
		requireTruef(t, out.ID > 0, "expected a board list in the update response")
		t.Logf("Reordered board list %d to position %d", out.ID, out.Position)
	})

	t.Run("DeleteList", func(t *testing.T) {
		requireTruef(t, listID > 0, "listID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_delete_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"list_id":  listID,
			},
		})
		requireNoError(t, err, "group_board_delete_list")
		t.Logf("Deleted board list %d", listID)
	})
}

// groupExtrasEnsureBoard returns an issue board ID for the group, reusing an
// existing board when present or provisioning the group's single CE board
// through the raw SDK. Returns false when no board can be obtained.
func groupExtrasEnsureBoard(ctx context.Context, t *testing.T, grp GroupFixture) (int64, bool) {
	t.Helper()
	boards, _, err := sess.glClient.GL().GroupIssueBoards.ListGroupIssueBoards(grp.ID, &gl.ListGroupIssueBoardsOptions{}, gl.WithContext(ctx))
	if err == nil && len(boards) > 0 {
		return boards[0].ID, true
	}
	board, _, err := sess.glClient.GL().GroupIssueBoards.CreateGroupIssueBoard(grp.ID, &gl.CreateGroupIssueBoardOptions{
		Name: new("E2E Board"),
	}, gl.WithContext(ctx))
	if err != nil {
		t.Logf("cannot provision a group issue board on this instance: %v", err)
		return 0, false
	}
	return board.ID, true
}

// groupExtrasCreateGroupLabel creates a group label through the meta-tool
// and returns its numeric ID for label-backed board lists.
func groupExtrasCreateGroupLabel(ctx context.Context, t *testing.T, grp GroupFixture) int64 {
	t.Helper()
	out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "group_label_create",
		"params": map[string]any{
			"group_id": grp.gidStr(),
			"name":     uniqueName("board-lbl"),
			"color":    "#0000FF",
		},
	})
	requireNoError(t, err, "create group label fixture")
	requireTruef(t, out.ID > 0, "expected group label ID")
	return out.ID
}
