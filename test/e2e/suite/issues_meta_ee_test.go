//go:build e2e && enterprise

// issues_meta_ee_test.go tests Enterprise issue meta-tool actions.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/workitems"
)

// TestMeta_IssueWorkItems exercises the work_item_* actions on gitlab_issue
// meta-tool: create → list → get → update → delete. Requires Enterprise license.
func TestMeta_IssueWorkItems(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", testFileMainGo, "work items test", "init commit")

	var workItemIID int64
	workItemTitle := uniqueName("work-item")

	t.Run("WorkItemCreate", func(t *testing.T) {
		out, err := callToolOn[workitems.GetOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_create",
			"params": map[string]any{
				"full_path":         proj.Path,
				"work_item_type_id": "gid://gitlab/WorkItems::Type/1",
				"title":             workItemTitle,
			},
		})
		requireNoError(t, err, "work_item_create")
		workItemIID = out.WorkItem.IID
		t.Logf("Created work item IID=%d", workItemIID)
	})

	t.Run("WorkItemList", func(t *testing.T) {
		if workItemIID == 0 {
			return
		}
		first := int64(5)
		out, err := callToolOn[workitems.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_list",
			"params": map[string]any{
				"full_path": proj.Path,
				"search":    workItemTitle,
				"first":     first,
			},
		})
		requireNoError(t, err, "work_item_list")
		for _, item := range out.WorkItems {
			if item.IID == workItemIID || item.Title == workItemTitle {
				t.Logf("Listed work item IID=%d title=%q", item.IID, item.Title)
				return
			}
		}
		t.Fatalf("work_item_list did not include created work item IID=%d title=%q: %+v", workItemIID, workItemTitle, out.WorkItems)
	})

	t.Run("WorkItemGet", func(t *testing.T) {
		if workItemIID == 0 {
			return
		}
		out, err := callToolOn[workitems.GetOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_get",
			"params": map[string]any{
				"full_path":     proj.Path,
				"work_item_iid": workItemIID,
			},
		})
		requireNoError(t, err, "work_item_get")
		t.Logf("Got work item: %s", out.WorkItem.Title)
	})

	t.Run("WorkItemUpdate", func(t *testing.T) {
		if workItemIID == 0 {
			return
		}
		out, err := callToolOn[workitems.GetOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_update",
			"params": map[string]any{
				"full_path":     proj.Path,
				"work_item_iid": workItemIID,
				"title":         "Updated Work Item",
			},
		})
		requireNoError(t, err, "work_item_update")
		t.Logf("Updated work item: %s", out.WorkItem.Title)
	})

	t.Run("WorkItemDelete", func(t *testing.T) {
		if workItemIID == 0 {
			return
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_delete",
			"params": map[string]any{
				"full_path":     proj.Path,
				"work_item_iid": workItemIID,
			},
		})
		requireNoError(t, err, "work_item_delete")
		t.Log("Deleted work item")
	})
}
