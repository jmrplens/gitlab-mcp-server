//go:build e2e && enterprise

// groupepicboards_ee_test.go exercises the group epic boards read
// actions via the gitlab_group meta-tool against a live GitLab EE
// Ultimate instance. Group epic boards are Premium/Ultimate-only.
// The catalog exposes only read actions (epic_board_list, epic_board_get)
// — there is no create/delete in the action spec registry.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupepicboards"
)

// TestMeta_GroupEpicBoards exercises the group epic boards read
// actions on a fresh group (boards empty is valid).
func TestMeta_GroupEpicBoards(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group epic boards require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-epic-board")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	t.Run("Meta/GroupEpicBoard/List", func(t *testing.T) {
		out, err := callToolOn[groupepicboards.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_board_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "epic_board_list")
		t.Logf("Group %s has %d epic board(s) (empty is valid for fresh group)", groupName, len(out.Boards))
	})

	t.Run("Meta/GroupEpicBoard/Get_Graceful404", func(t *testing.T) {
		_, err := callToolOn[groupepicboards.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_board_get",
			"params": map[string]any{"group_id": grp.gidStr(), "board_id": 999999},
		})
		if err == nil {
			t.Fatal("epic_board_get for non-existent board_id returned no error; expected 404")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("epic_board_get error was not 404: %v", err)
		}
		t.Logf("epic_board_get 404 error path validated: %v", err)
	})
}
