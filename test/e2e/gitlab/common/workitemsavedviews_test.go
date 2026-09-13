//go:build e2e

// workitemsavedviews_test.go covers the work item saved views of a group
// namespace: the listing, which answers on every instance that has the
// feature, and the life of one view, which the instance under test may
// refuse to begin. GitLab introduced saved views in 18.7 as an experiment
// and its create mutation answered 500 on the Community image this suite
// runs against, so the lifecycle probes the create and skips, naming the
// answer, when the experiment is broken there; the listing is held to its
// answer either way.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/workitemsavedviews"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The sort a view is created with and the one it is changed to.
const (
	savedViewSort        = "CREATED_DESC"
	savedViewUpdatedSort = "UPDATED_DESC"
)

// savedViewIDs lists the ids of a saved view listing.
func savedViewIDs(listed []workitemsavedviews.Item) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, view := range listed {
		ids = append(ids, view.ID)
	}
	return ids
}

// TestWorkItemSavedViews_List_AnswersTheNamespace lists the saved views of
// a group of each surface's own, which holds none.
//
// Replaces: TestMeta_WorkItemSavedViews
func TestWorkItemSavedViews_List_AnswersTheNamespace(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("savedviews"))

		listed := harness.Do[workitemsavedviews.ListOutput](s, actionSavedViewList, map[string]any{"namespace_path": group.Path})
		if listed.NamespacePath != group.Path || len(listed.SavedViews) != 0 {
			e.T.Errorf("the listing answered %+v, want the namespace %q with no view", listed, group.Path)
		}
	})
}

// TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete creates a
// view in a group of each surface's own, reads it with its filters,
// changes its sort and description, subscribes to it and unsubscribes,
// finds it in the listing and deletes it. An instance whose create answers
// the experiment's 500 ends the scenario there, with that answer as the
// reason.
//
// Replaces: TestMeta_WorkItemSavedViews
func TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("savedviewlife"))
		namespace := map[string]any{"namespace_path": group.Path}
		name := e.Name("view")

		created, err := harness.Try[workitemsavedviews.MutateOutput](s, actionSavedViewCreate, withParams(namespace, map[string]any{
			"name": name, "description": "created by the e2e suite", "sort": savedViewSort, "filters": map[string]any{"state": "opened"},
		}))
		if err != nil && strings.Contains(err.Error(), "500") && strings.Contains(err.Error(), "Internal server error") {
			e.Skipf("the saved view create answered the experiment's 500 on this GitLab version: %s", firstLine(err.Error()))
		}
		if err != nil || created.SavedView.ID == 0 || created.SavedView.Name != name {
			e.T.Fatalf("work_item_saved_view_create answered %+v, %v; want the view %q with an ID", created.SavedView, err, name)
		}
		view := map[string]any{"saved_view_id": created.SavedView.ID}

		got := harness.Do[workitemsavedviews.GetOutput](s, actionSavedViewGet, withParams(namespace, view))
		if got.SavedView.ID != created.SavedView.ID || got.SavedView.Name != name || got.SavedView.Filters == nil {
			e.T.Errorf("work_item_saved_view_get answered %+v, want view %d %q with its filters, which only the get resolves", got.SavedView, created.SavedView.ID, name)
		}
		updated := harness.Do[workitemsavedviews.MutateOutput](s, actionSavedViewUpdate, withParams(view, map[string]any{
			"description": "updated by the e2e suite", "sort": savedViewUpdatedSort,
		}))
		if updated.SavedView.Sort != savedViewUpdatedSort || updated.SavedView.Description != "updated by the e2e suite" {
			e.T.Errorf("work_item_saved_view_update answered %+v, want the sort %s and the new description", updated.SavedView, savedViewUpdatedSort)
		}

		subscribed := harness.Do[workitemsavedviews.MutateOutput](s, actionSavedViewSubscribe, view)
		if !subscribed.SavedView.Subscribed {
			e.T.Errorf("work_item_saved_view_subscribe answered %+v, want the view subscribed", subscribed.SavedView)
		}
		unsubscribed := harness.Do[workitemsavedviews.MutateOutput](s, actionSavedViewUnsubscribe, view)
		if unsubscribed.SavedView.Subscribed {
			e.T.Errorf("work_item_saved_view_unsubscribe answered %+v, want the view no longer subscribed", unsubscribed.SavedView)
		}
		listed := harness.Do[workitemsavedviews.ListOutput](s, actionSavedViewList, namespace)
		if !containsID(savedViewIDs(listed.SavedViews), created.SavedView.ID) {
			e.T.Errorf("the namespace lists the views %v, want %d among them", savedViewIDs(listed.SavedViews), created.SavedView.ID)
		}

		harness.DoVoid(s, actionSavedViewDelete, view)
		remaining := harness.Do[workitemsavedviews.ListOutput](s, actionSavedViewList, namespace)
		if containsID(savedViewIDs(remaining.SavedViews), created.SavedView.ID) {
			e.T.Errorf("the namespace still lists view %d after its delete", created.SavedView.ID)
		}
	})
}
