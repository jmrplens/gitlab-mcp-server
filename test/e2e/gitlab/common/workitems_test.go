//go:build e2e

// workitems_test.go covers the Free half of the issue tool's work items:
// the type listing that says what a project may hold, and the lifecycle of
// one work item of the Issue type. The old suite kept the lifecycle in its
// Enterprise half, and on a Free runtime it still cannot run, for a reason
// that is client-go's rather than GitLab's: the SDK's get, create and
// update documents select five licensed widgets that Community Edition
// refuses, so the lifecycle skips there and names the register entry.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// issueTypeName is the name of the work item type every project holds.
const issueTypeName = "Issue"

// workItemTypeID finds the global ID of a type by name in a type listing.
func workItemTypeID(types []workitems.WorkItemTypeOutput, name string) (string, bool) {
	for _, workItemType := range types {
		if workItemType.Name == name {
			return workItemType.ID, true
		}
	}
	return "", false
}

// lookUpIssueType lists the work item types of a project through the server
// and returns the Issue type's global ID, failing the test when the
// listing does not hold it.
func lookUpIssueType(e *harness.Env, s *harness.Session, project fixture.Project) string {
	e.T.Helper()
	types := harness.Do[workitems.WorkItemTypeListOutput](s, actionWorkItemTypeList, map[string]any{"full_path": project.Path})
	typeID, found := workItemTypeID(types.Types, issueTypeName)
	if !found {
		e.T.Fatalf("the project's work item types do not hold %s: %+v", issueTypeName, types.Types)
	}
	return typeID
}

// TestWorkItemTypes_List_HoldsTheIssueType lists the work item types of a
// shared project on every surface and finds the Issue type among them,
// which is the one read of the family that answers on every edition.
//
// Replaces: TestMeta_IssueWorkItems
func TestWorkItemTypes_List_HoldsTheIssueType(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("witypes"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		e.T.Logf("the Issue type is %s", lookUpIssueType(e, e.On(surface), project))
	})
}

// TestWorkItems_Lifecycle_IssueTypeCreateListGetUpdateDelete looks the
// Issue type up in a shared project on every surface, creates a work item
// of it, finds it in the listing by its title, reads it back, retitles it,
// deletes it and checks the read is then refused.
//
// Replaces: TestMeta_IssueWorkItems
func TestWorkItems_Lifecycle_IssueTypeCreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)
	// The edition decides, not the tier: an Enterprise image carries the
	// widgets in its schema whether or not a license activates them, and a
	// Community image does not carry them at all.
	if !e.Runtime().Enterprise {
		e.Skipf("client-go v3.0.0's work item get, create and update documents select five licensed widgets " +
			"that Community Edition's schema does not have (docs/development/upstream-bugs.md, entry 45)," +
			"so the lifecycle cannot run on a Community image")
	}

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("workitems"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"full_path": project.Path}
		typeID := lookUpIssueType(e, s, project)

		title := e.Name("item")
		created := harness.Do[workitems.GetOutput](s, actionWorkItemCreate, withParams(params, map[string]any{"work_item_type_id": typeID, "title": title}))
		if created.WorkItem.IID == 0 || created.WorkItem.Title != title {
			e.T.Fatalf("work_item_create answered %+v, want the item %q with an iid", created.WorkItem, title)
		}
		item := withParams(params, map[string]any{"work_item_iid": created.WorkItem.IID})

		listed := harness.Do[workitems.ListOutput](s, actionWorkItemList, withParams(params, map[string]any{"search": title, "first": int64(5)}))
		found := false
		for _, listedItem := range listed.WorkItems {
			if listedItem.IID == created.WorkItem.IID {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the listing by %q does not hold the created item #%d: %+v", title, created.WorkItem.IID, listed.WorkItems)
		}
		got := harness.Do[workitems.GetOutput](s, actionWorkItemGet, item)
		if got.WorkItem.IID != created.WorkItem.IID || got.WorkItem.Title != title {
			e.T.Errorf("work_item_get answered #%d %q, want #%d %q", got.WorkItem.IID, got.WorkItem.Title, created.WorkItem.IID, title)
		}

		updated := harness.Do[workitems.GetOutput](s, actionWorkItemUpdate, withParams(item, map[string]any{"title": "Updated " + title}))
		if updated.WorkItem.Title != "Updated "+title {
			e.T.Errorf("work_item_update answered the title %q, want the one just written", updated.WorkItem.Title)
		}

		harness.DoVoid(s, actionWorkItemDelete, item)
		refused := harness.Refused(s, actionWorkItemGet, item, harness.FailureNotFound)
		e.T.Logf("the read of the deleted work item was refused: %s", firstLine(refused))
	})
}
