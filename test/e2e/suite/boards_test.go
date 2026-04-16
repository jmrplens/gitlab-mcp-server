//go:build e2e

package suite

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/boards"
)

// TestMeta_Boards exercises issue board CRUD via the gitlab_project meta-tool.
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
		requireTrue(t, out.ID > 0, "expected positive board ID")
		boardID = out.ID
		t.Logf("Created board %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/Board/List", func(t *testing.T) {
		requireTrue(t, boardID > 0, "boardID not set")
		out, err := callToolOn[boards.ListBoardsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "board list")
		requireTrue(t, len(out.Boards) >= 1, "expected at least 1 board")
		t.Logf("Listed %d board(s)", len(out.Boards))
	})

	t.Run("Meta/Board/Get", func(t *testing.T) {
		requireTrue(t, boardID > 0, "boardID not set")
		out, err := callToolOn[boards.BoardOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   fmt.Sprintf("%d", boardID),
			},
		})
		requireNoError(t, err, "board get")
		requireTrue(t, out.ID == boardID, "board ID mismatch")
		t.Logf("Got board %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/Board/Delete", func(t *testing.T) {
		requireTrue(t, boardID > 0, "boardID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   fmt.Sprintf("%d", boardID),
			},
		})
		requireNoError(t, err, "board delete")
		t.Logf("Deleted board %d", boardID)
	})
}
