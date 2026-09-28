//go:build e2e

// workitemsavedviews_test.go covers the work item saved views of a group
// namespace: the listing, and the life of one view.
//
// GitLab introduced saved views in 18.7 as an experiment, and on 19.4 two of
// its mutations answer 500 to a caller authenticated with a token nearly
// every time: subscribing locks the user row while the sign-in tracking has
// left unsaved changes on it (docs/development/upstream-bugs.md, entry 72).
// The create meets it after it has saved the view, and the subscribe before
// it records anything. The lifecycle therefore runs on every GitLab and
// holds both answers: a create that failed is held to the hint that the view
// may exist and to the view being in the listing, and the scenario goes on
// with that view; a subscribe that failed is held to its hint and to the view
// still not being followed. It used to skip on the create's 500, which left
// the other five actions unreached on every instance that has the defect.

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
// finds it in the listing and deletes it. Where GitLab answers the create or
// the subscribe with the 500 of entry 72, the answer is held to what it left
// behind and the scenario goes on (see the file comment).
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
		viewID := created.SavedView.ID
		switch {
		case err != nil:
			viewID = savedViewLeftByAFailedCreate(e, s, namespace, name, err)
		case viewID == 0 || created.SavedView.Name != name:
			e.T.Fatalf("work_item_saved_view_create answered %+v; want the view %q with an ID", created.SavedView, name)
		}
		view := map[string]any{"saved_view_id": viewID}

		got := harness.Do[workitemsavedviews.GetOutput](s, actionSavedViewGet, withParams(namespace, view))
		if got.SavedView.ID != viewID || got.SavedView.Name != name || got.SavedView.Filters == nil {
			e.T.Errorf("work_item_saved_view_get answered %+v, want view %d %q with its filters, which only the get resolves", got.SavedView, viewID, name)
		}
		updated := harness.Do[workitemsavedviews.MutateOutput](s, actionSavedViewUpdate, withParams(view, map[string]any{
			"description": "updated by the e2e suite", "sort": savedViewUpdatedSort,
		}))
		if updated.SavedView.Sort != savedViewUpdatedSort || updated.SavedView.Description != "updated by the e2e suite" {
			e.T.Errorf("work_item_saved_view_update answered %+v, want the sort %s and the new description", updated.SavedView, savedViewUpdatedSort)
		}

		subscribed, err := harness.Try[workitemsavedviews.MutateOutput](s, actionSavedViewSubscribe, view)
		switch {
		case err != nil:
			assertSubscribeRecordedNothing(e, s, withParams(namespace, view), got.SavedView.Subscribed, err)
		case !subscribed.SavedView.Subscribed:
			e.T.Errorf("work_item_saved_view_subscribe answered %+v, want the view subscribed", subscribed.SavedView)
		}
		unsubscribed := harness.Do[workitemsavedviews.MutateOutput](s, actionSavedViewUnsubscribe, view)
		if unsubscribed.SavedView.Subscribed {
			e.T.Errorf("work_item_saved_view_unsubscribe answered %+v, want the view no longer subscribed", unsubscribed.SavedView)
		}
		listed := harness.Do[workitemsavedviews.ListOutput](s, actionSavedViewList, namespace)
		if !containsID(savedViewIDs(listed.SavedViews), viewID) {
			e.T.Errorf("the namespace lists the views %v, want %d among them", savedViewIDs(listed.SavedViews), viewID)
		}

		harness.DoVoid(s, actionSavedViewDelete, view)
		remaining := harness.Do[workitemsavedviews.ListOutput](s, actionSavedViewList, namespace)
		if containsID(savedViewIDs(remaining.SavedViews), viewID) {
			e.T.Errorf("the namespace still lists view %d after its delete", viewID)
		}
	})
}

// savedViewLeftByAFailedCreate holds a create GitLab answered with the 500 of
// upstream-bugs entry 72 to what that answer leaves behind, and returns the
// view it left so the lifecycle goes on with it.
//
// Two things are held. The refusal has to say the view may exist and name the
// listing to find it with, because a caller told only that the create failed
// creates it again and holds two. And the view has to be in the listing,
// because that promise is the whole of the hint: GitLab saves the view before
// the step that fails, and a release that stopped doing so would make the
// hint send a caller looking for something that is not there. Any other
// failure is a finding.
func savedViewLeftByAFailedCreate(e *harness.Env, s *harness.Session, namespace map[string]any, name string, err error) int64 {
	e.T.Helper()
	text := err.Error()
	if !strings.Contains(text, "500") {
		e.T.Fatalf("work_item_saved_view_create failed with something other than the server error of upstream-bugs entry 72: %v", err)
	}
	assertMentions(e, "the create's server error", text,
		"look for this name before creating it again", string(actionSavedViewList))

	listed := harness.Do[workitemsavedviews.ListOutput](s, actionSavedViewList, namespace)
	for _, view := range listed.SavedViews {
		if view.Name == name {
			e.T.Logf("the create answered GitLab's 500 after saving view %d, which the listing found: %s", view.ID, firstLine(text))
			return view.ID
		}
	}
	e.T.Fatalf("the create answered a server error and the namespace lists no view named %q (%v), so the hint that it may exist sent the caller after nothing: %v",
		name, savedViewIDs(listed.SavedViews), err)
	return 0
}

// assertSubscribeRecordedNothing holds a subscribe GitLab answered with the
// 500 of upstream-bugs entry 72 to its hint and to the view's subscription
// being what it was before the call, which is what the hint tells the caller.
// The state before is passed in rather than assumed unfollowed: a create that
// succeeded subscribed its creator, and a subscribe failing after it leaves
// the view followed without having recorded anything.
func assertSubscribeRecordedNothing(e *harness.Env, s *harness.Session, viewInNamespace map[string]any, subscribedBefore bool, err error) {
	e.T.Helper()
	text := err.Error()
	if !strings.Contains(text, "500") {
		e.T.Fatalf("work_item_saved_view_subscribe failed with something other than the server error of upstream-bugs entry 72: %v", err)
	}
	assertMentions(e, "the subscribe's server error", text, "before recording the subscription")

	after := harness.Do[workitemsavedviews.GetOutput](s, actionSavedViewGet, viewInNamespace)
	if after.SavedView.Subscribed != subscribedBefore {
		e.T.Errorf("the view's subscription went from %t to %t across a subscribe that answered a server error, so the hint that nothing changed is false: %+v",
			subscribedBefore, after.SavedView.Subscribed, after.SavedView)
	}
	e.T.Logf("the subscribe answered GitLab's 500 and left the view's subscription at %t: %s", subscribedBefore, firstLine(text))
}
