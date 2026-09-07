//go:build e2e && !enterprise

// boards_ce_test.go tests the issue board MCP tools via the gitlab_project
// meta-tool against a live GitLab instance. Exercises the full board
// lifecycle: create → list → get → delete.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/boards"
)

// TestMeta_Boards exercises issue board CRUD via the gitlab_project
// meta-tool against a live GitLab instance.
//
// The test creates a project fixture, runs board_create, board_list,
// board_get, and board_delete as subtests against the catalog-backed
// gitlab_project tool, and asserts each step's expected ID round-trips.
// Cleanup relies on the explicit delete subtest; project removal is
// handled by the per-test resource ledger.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Boards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	var boardID int64

	t.Run("Meta/Board/Create", func(t *testing.T) {
		out, err := callToolOn[boards.BoardOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-board",
			},
		})
		requireNoError(t, err, "board create")
		requireTruef(t, out.ID > 0, "expected positive board ID")
		boardID = out.ID
		t.Logf("Created board %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/Board/List", func(t *testing.T) {
		requireTruef(t, boardID > 0, "boardID not set")
		out, err := callToolOn[boards.ListBoardsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "board list")
		requireTruef(t, len(out.Boards) >= 1, "expected at least 1 board")
		t.Logf("Listed %d board(s)", len(out.Boards))
	})

	t.Run("Meta/Board/Get", func(t *testing.T) {
		requireTruef(t, boardID > 0, "boardID not set")
		out, err := callToolOn[boards.BoardOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   strconv.FormatInt(boardID, 10),
			},
		})
		requireNoError(t, err, "board get")
		requireTruef(t, out.ID == boardID, "board ID mismatch")
		t.Logf("Got board %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/Board/Delete", func(t *testing.T) {
		requireTruef(t, boardID > 0, "boardID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   strconv.FormatInt(boardID, 10),
			},
		})
		requireNoError(t, err, "board delete")
		requireNotListedOn(ctx, t, sess.meta, "project boards after delete", "gitlab_project", map[string]any{
			"action": "board_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		}, func(out boards.ListBoardsOutput) []int64 {
			ids := make([]int64, 0, len(out.Boards))
			for _, b := range out.Boards {
				ids = append(ids, b.ID)
			}
			return ids
		}, boardID)
		t.Logf("Deleted board %d", boardID)
	})
}
