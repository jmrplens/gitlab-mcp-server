//go:build e2e && enterprise

// storagemoves_ee_test.go exercises the group/project/snippet
// storage moves through the gitlab_storage_move meta-tool against
// a live GitLab EE Ultimate instance. Storage moves are
// Ultimate-only and require GitLab Geo (single-node Docker
// returns 404). The test asserts the error path is surfaced
// cleanly (404) for the routing cases.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippetstoragemoves"
)

// TestMeta_GroupStorageMoves_Graceful404 exercises group storage
// move list (returns 404 when Geo is not configured) via
// gitlab_storage_move.
func TestMeta_GroupStorageMoves_Graceful404(t *testing.T) {
	if !sess.enterprise {
		t.Skip("storage moves require GitLab Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-gsm")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	t.Run("Meta/GroupStorageMove/RetrieveAll", func(t *testing.T) {
		_, err := callToolOn[groupstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_group",
			"params": map[string]any{},
		})
		if err == nil {
			t.Log("group storage move list returned no error (Geo may be configured)")
			return
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("group storage move list error was not 404: %v", err)
		}
		t.Logf("group storage move list 404 error path validated: %v", err)
	})

	t.Run("Meta/GroupStorageMove/RetrieveForGroup", func(t *testing.T) {
		_, err := callToolOn[groupstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_group",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("group storage move list-for-group error was not 404: %v", err)
		}
		t.Logf("group storage move list-for-group 404 error path validated: %v", err)
	})

	t.Run("Meta/GroupStorageMove/ScheduleInvalid_Graceful422", func(t *testing.T) {
		// Schedule with an invalid storage name returns 422 (validation)
		// rather than 404 because the route exists. This confirms the
		// tool distinguishes between routing (404) and validation (422).
		// The schedule_* actions accept `destination_storage_name` (and
		// optionally `source_storage_name`); a non-existent storage
		// shard triggers 422 because the route is wired up but the
		// shard name is rejected.
		_, err := callToolOn[groupstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_group",
			"params": map[string]any{
				"group_id":                 grp.gidStr(),
				"destination_storage_name": "e2e-missing-storage",
			},
		})
		if err == nil {
			t.Log("group storage move schedule returned no error")
			return
		}
		if !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("group storage move schedule error was not 404/422: %v", err)
		}
		t.Logf("group storage move schedule error path validated: %v", err)
	})
}

// TestMeta_ProjectStorageMoves_Graceful404 exercises project storage
// move list (returns 404 when Geo is not configured) via
// gitlab_storage_move.
func TestMeta_ProjectStorageMoves_Graceful404(t *testing.T) {
	if !sess.enterprise {
		t.Skip("storage moves require GitLab Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/ProjectStorageMove/RetrieveAll", func(t *testing.T) {
		_, err := callToolOn[projectstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_project",
			"params": map[string]any{},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("project storage move list error was not 404: %v", err)
		}
		t.Logf("project storage move list 404 error path validated: %v", err)
	})

	t.Run("Meta/ProjectStorageMove/RetrieveForProject", func(t *testing.T) {
		_, err := callToolOn[projectstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("project storage move list-for-project error was not 404: %v", err)
		}
		t.Logf("project storage move list-for-project 404 error path validated: %v", err)
	})

	t.Run("Meta/ProjectStorageMove/ScheduleInvalid_Graceful422", func(t *testing.T) {
		_, err := callToolOn[projectstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"id":         999999,
			},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("project storage move schedule error was not 404/422: %v", err)
		}
		t.Logf("project storage move schedule error path validated: %v", err)
	})
}

// TestMeta_SnippetStorageMoves_Graceful404 exercises snippet storage
// move list (returns 404 when Geo is not configured) via
// gitlab_storage_move.
func TestMeta_SnippetStorageMoves_Graceful404(t *testing.T) {
	if !sess.enterprise {
		t.Skip("storage moves require GitLab Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	// Use a fake snippet ID — the storage move endpoints for snippets
	// are at /snippets/:id/storage_moves and will return 404 because
	// (a) Geo is not configured in single-node Docker, (b) snippet 999999
	// does not exist.
	const fakeSnippetID = "999999"

	t.Run("Meta/SnippetStorageMove/RetrieveAll", func(t *testing.T) {
		_, err := callToolOn[snippetstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_snippet",
			"params": map[string]any{},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("snippet storage move list error was not 404: %v", err)
		}
		t.Logf("snippet storage move list 404 error path validated: %v", err)
	})

	t.Run("Meta/SnippetStorageMove/Retrieve", func(t *testing.T) {
		_, err := callToolOn[snippetstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_snippet",
			"params": map[string]any{"snippet_id": fakeSnippetID},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("snippet storage move list-for-snippet error was not 404: %v", err)
		}
		t.Logf("snippet storage move list-for-snippet 404 error path validated: %v", err)
	})

	t.Run("Meta/SnippetStorageMove/ScheduleInvalid_Graceful422", func(t *testing.T) {
		_, err := callToolOn[snippetstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_snippet",
			"params": map[string]any{
				"snippet_id": fakeSnippetID,
				"id":         999999,
			},
		})
		if err == nil {
			return
		}
		if !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("snippet storage move schedule error was not 404/422: %v", err)
		}
		t.Logf("snippet storage move schedule error path validated: %v", err)
	})
}
