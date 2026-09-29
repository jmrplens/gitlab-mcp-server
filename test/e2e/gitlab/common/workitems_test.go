//go:build e2e

// workitems_test.go covers the Free half of the issue tool's work items:
// the type listing that says what a project may hold, and the lifecycle of
// one work item of the Issue type. The old suite kept the lifecycle in its
// Enterprise half, and on a Community image it still cannot run whole, for a
// reason that is client-go's rather than GitLab's: the SDK's get, create and
// update documents select five licensed widgets that Community Edition's
// schema does not define (docs/development/upstream-bugs.md, entry 45). The
// lifecycle used to skip there, which held nothing at all; it now runs what
// Community Edition does today and pins the refusal of the other three, so the
// day a client-go release carrying the fix is taken here, that assertion fails
// and says to run the whole lifecycle on Community Edition again.

package common

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/workitems"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// issueTypeName is the name of the work item type every project holds.
const issueTypeName = "Issue"

// communitySchemaRefusal is the stable part of what a Community Edition
// instance answers a work item document selecting a licensed widget with:
// graphql-ruby's wording for a field the schema does not define, on the type
// the widgets hang off. What surrounds it (the operation client-go names, the
// class this server gives the error, the elapsed time) is left out on purpose.
const communitySchemaRefusal = "doesn't exist on type 'WorkItemFeatures'"

// communityMissingWidgets are the five widgets client-go's get, create and
// update documents select that Community Edition's schema does not define,
// each named in the refusal as GitLab names it.
var communityMissingWidgets = []string{"color", "healthStatus", "iteration", "status", "weight"}

// gitLabMaxValidationErrors is how many validation errors GitLab reports for
// one document before it stops (`validate_max_errors 5` in
// app/graphql/gitlab_schema.rb, read at 19.4.1). The five widgets fill it
// exactly, so a sixth licensed field client-go started selecting would push
// one of them out of the refusal while client-go still selects it, and the
// assertion has to say so rather than report the widget gone.
const gitLabMaxValidationErrors = 5

// communityRefusedField reads each field a Community Edition refusal names,
// in graphql-ruby's wording.
var communityRefusedField = regexp.MustCompile(`Field '([^']+)' ` + regexp.QuoteMeta(communitySchemaRefusal))

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
// On a Community image it holds what that edition does today instead:
// [workItemLifecycleOnCommunity].
//
// Replaces: TestMeta_IssueWorkItems
func TestWorkItems_Lifecycle_IssueTypeCreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)
	// The edition decides, not the tier: an Enterprise image carries the
	// widgets in its schema whether or not a license activates them, and a
	// Community image does not carry them at all.
	lifecycle := workItemLifecycle
	if !e.Runtime().Enterprise {
		lifecycle = workItemLifecycleOnCommunity
	}

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("workitems"))
	}, lifecycle)
}

// workItemLifecycle is the whole lifecycle, on an instance whose schema
// defines every widget client-go's documents select.
func workItemLifecycle(e *harness.Env, surface harness.Surface, project fixture.Project) {
	s := e.On(surface)
	params := map[string]any{"full_path": project.Path}
	typeID := lookUpIssueType(e, s, project)

	title := e.Name("item")
	created := harness.Do[workitems.GetOutput](s, actionWorkItemCreate, withParams(params, map[string]any{"work_item_type_id": typeID, "title": title}))
	if created.WorkItem.IID == 0 || created.WorkItem.Title != title {
		e.T.Fatalf("work_item_create answered %+v, want the item %q with an iid", created.WorkItem, title)
	}
	item := withParams(params, map[string]any{"work_item_iid": created.WorkItem.IID})

	assertWorkItemListed(e, s, params, title, created.WorkItem.IID)
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
}

// workItemLifecycleOnCommunity is the lifecycle as Community Edition runs it
// while client-go selects licensed widgets in its get, create and update
// documents (docs/development/upstream-bugs.md, entry 45).
//
// The create is refused, so the item the rest works on is an issue made
// through the REST API, which is a work item of the Issue type under the same
// iid. The listing and the delete select nothing licensed and are held to
// working; the read and the retitle are held to GitLab's refusal. The deletion
// is confirmed through the REST API, since the tool's own read is refused
// whether the item exists or not.
func workItemLifecycleOnCommunity(e *harness.Env, surface harness.Surface, project fixture.Project) {
	s := e.On(surface)
	params := map[string]any{"full_path": project.Path}
	typeID := lookUpIssueType(e, s, project)

	title := e.Name("item")
	assertCommunityWidgetRefusal(e, s, actionWorkItemCreate, withParams(params, map[string]any{"work_item_type_id": typeID, "title": title}))

	issue := fixture.NewIssue(e, project, title)
	item := withParams(params, map[string]any{"work_item_iid": issue.IID})
	assertWorkItemListed(e, s, params, title, issue.IID)
	assertCommunityWidgetRefusal(e, s, actionWorkItemGet, item)
	assertCommunityWidgetRefusal(e, s, actionWorkItemUpdate, withParams(item, map[string]any{"title": "Updated " + title}))

	harness.DoVoid(s, actionWorkItemDelete, item)
	_, _, err := e.Client().GL().Issues.GetIssue(project.ID, issue.IID, gl.WithContext(e.Ctx))
	if !errors.Is(err, gl.ErrNotFound) {
		e.T.Errorf("the issue behind work item #%d is still readable after work_item_delete answered: %v", issue.IID, err)
	}
}

// assertWorkItemListed lists the project's work items by title and fails the
// test when the one with the given iid is not among them.
func assertWorkItemListed(e *harness.Env, s *harness.Session, params map[string]any, title string, iid int64) {
	e.T.Helper()
	listed := harness.Do[workitems.ListOutput](s, actionWorkItemList, withParams(params, map[string]any{"search": title, "first": int64(5)}))
	for _, listedItem := range listed.WorkItems {
		if listedItem.IID == iid {
			return
		}
	}
	e.T.Errorf("the listing by %q does not hold item #%d: %+v", title, iid, listed.WorkItems)
}

// assertCommunityWidgetRefusal calls a work item action whose client-go
// document selects the licensed widgets, and holds the answer to GitLab's
// refusal of the fields its Community schema lacks.
//
// An answer is the news this is waiting for: the action now sends a document
// that selects only what Community Edition defines, so the whole lifecycle
// can run on Community Edition, and the failure says so. Taking the client-go
// release that carries the fix is not enough on its own, since it leaves the
// default selecting every field, and the maintainers decided that default
// stays Enterprise: the actions have to pass WorkItemDefaultListFields() on a
// Free instance.
func assertCommunityWidgetRefusal(e *harness.Env, s *harness.Session, id harness.ActionID, params map[string]any) {
	e.T.Helper()
	_, err := harness.Try[workitems.GetOutput](s, id, params)
	if err == nil {
		e.T.Fatalf("%s answered on Community Edition, where client-go's document selected five licensed widgets "+
			"its schema does not define (docs/development/upstream-bugs.md, entry 45). The action now sends a "+
			"selection Community Edition defines, WorkItemDefaultListFields() from a client-go release that "+
			"carries the fix, since the default stays Enterprise: run workItemLifecycle on Community Edition too, "+
			"delete workItemLifecycleOnCommunity, and record the entry as merged", id)
	}
	refusal := err.Error()
	if !strings.Contains(refusal, communitySchemaRefusal) {
		e.T.Fatalf("%s was refused, but not with GitLab's refusal of a field Community Edition's schema lacks (%q), "+
			"which is the refusal entry 45 of docs/development/upstream-bugs.md describes: %s", id, communitySchemaRefusal, firstLine(refusal))
	}
	named := map[string]bool{}
	for _, match := range communityRefusedField.FindAllStringSubmatch(refusal, -1) {
		named[match[1]] = true
	}
	var unlisted []string
	for field := range named {
		if !slices.Contains(communityMissingWidgets, field) {
			unlisted = append(unlisted, field)
		}
	}
	slices.Sort(unlisted)
	if len(unlisted) > 0 {
		e.T.Errorf("%s was refused naming %s, which client-go's document now selects and entry 45 of "+
			"docs/development/upstream-bugs.md does not list: add it there and to communityMissingWidgets. GitLab "+
			"reports at most %d such errors, so a field past the five also pushes a listed one out of the refusal: %s",
			id, strings.Join(unlisted, ", "), gitLabMaxValidationErrors, firstLine(refusal))
	}
	for _, widget := range communityMissingWidgets {
		if named[widget] {
			continue
		}
		if len(named) >= gitLabMaxValidationErrors {
			e.T.Errorf("%s was refused without naming the %s widget, but the refusal already names %d fields, "+
				"which is all GitLab reports for one document, so client-go may still select it; the field that "+
				"took its place is the one to add to entry 45 of docs/development/upstream-bugs.md: %s",
				id, widget, len(named), firstLine(refusal))
			continue
		}
		e.T.Errorf("%s was refused without naming the %s widget, one of the five entry 45 of "+
			"docs/development/upstream-bugs.md lists, while GitLab had room to report it; if client-go "+
			"stopped selecting it, the entry needs its list corrected: %s", id, widget, firstLine(refusal))
	}
}
