//go:build e2e

// epic.go builds an epic, which GitLab 19 offers only as a work item: the
// REST epics API is gone there, so the builder goes through client-go's work
// item service, the same route the epic tools take. An epic goes with its
// group, so nothing is registered.
//
// The note and the issue assignment go through GraphQL rather than through
// the work item service, because client-go models neither: a note on a work
// item is the createNote mutation, and an epic's children are the hierarchy
// widget of a workItemUpdate. Both documents are written here rather than
// borrowed from the tools under test, so a fixture cannot go green or red
// with the thing it is a fixture for.

package fixture

import (
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Epic is an epic a builder created in a group.
type Epic struct {
	// IID is the group-scoped number every epic action takes.
	IID int64
	// ID is the instance-wide identifier.
	ID int64
	// Title is what it was created with.
	Title string
	// GroupPath is the full path of the group it belongs to, which is what
	// the epic actions name the group by.
	GroupPath string
}

// NewEpic creates an epic in the group. Epics are a licensed feature, and a
// package that runs on an unlicensed instance never reaches this builder:
// the harness refuses a Premium action before it is sent.
func NewEpic(e *harness.Env, group Group, title string) Epic {
	e.T.Helper()

	epic, err := retryTransient(e, "create epic "+title, createRetries, func() (Epic, error) {
		created, _, err := e.Client().GL().WorkItems.CreateWorkItem(group.Path, gl.WorkItemTypeEpic,
			&gl.CreateWorkItemOptions{Title: title}, gl.WithContext(e.Ctx))
		if err != nil {
			return Epic{}, err
		}
		return Epic{IID: created.IID, ID: created.ID, Title: created.Title, GroupPath: group.Path}, nil
	})
	if err != nil {
		e.T.Fatalf("creating epic %q in group %s: %v", title, group.Path, err)
	}
	return epic
}

// EpicNote is a note a builder wrote on an epic, with the discussion GitLab
// opened for it.
type EpicNote struct {
	// ID is the note's numeric identifier, which is what the epic note
	// actions take.
	ID int64
	// DiscussionID is the thread GitLab opened, as its hexadecimal
	// identifier rather than as a global id, which is what the epic
	// discussion actions take.
	DiscussionID string
	// Body is what the note holds.
	Body string
}

// epicNoteBody is what every fixture epic note holds.
const epicNoteBody = "e2e epic note fixture."

// The documents the epic builders send.
const (
	createEpicNoteMutation = `
mutation($noteableId: NoteableID!, $body: String!) {
  createNote(input: { noteableId: $noteableId, body: $body }) {
    note {
      id
      discussion { id }
    }
    errors
  }
}
`
	assignEpicChildMutation = `
mutation($id: WorkItemID!, $childrenIds: [WorkItemID!]!) {
  workItemUpdate(input: { id: $id, hierarchyWidget: { childrenIds: $childrenIds } }) {
    workItem { id }
    errors
  }
}
`
)

// NewEpicNote writes a note on the epic and returns it with the discussion
// GitLab opened for it. The note goes with the epic, so nothing is
// registered.
func NewEpicNote(e *harness.Env, epic Epic) EpicNote {
	e.T.Helper()

	var created struct {
		CreateNote struct {
			Note *struct {
				ID         string `json:"id"`
				Discussion *struct {
					ID string `json:"id"`
				} `json:"discussion"`
			} `json:"note"`
			Errors []string `json:"errors"`
		} `json:"createNote"`
	}
	err := mutate(e, createEpicNoteMutation, map[string]any{
		"noteableId": workItemGID(epic.ID),
		"body":       epicNoteBody,
	}, &created)
	if err != nil {
		e.T.Fatalf("writing a note on epic &%d of %s: %v", epic.IID, epic.GroupPath, err)
	}
	if refusals := created.CreateNote.Errors; len(refusals) > 0 {
		e.T.Fatalf("GitLab refused the note on epic &%d: %s", epic.IID, strings.Join(refusals, "; "))
	}
	note := created.CreateNote.Note
	if note == nil || note.Discussion == nil {
		e.T.Fatalf("the note mutation answered %+v for epic &%d, want a note and its discussion", note, epic.IID)
	}

	id, parseErr := gidNumber(note.ID)
	if parseErr != nil {
		e.T.Fatalf("reading the note id out of %q: %v", note.ID, parseErr)
	}
	return EpicNote{ID: id, DiscussionID: discussionHex(note.Discussion.ID), Body: epicNoteBody}
}

// AssignIssueToEpic makes the issue a child of the epic. The link goes with
// the two objects, so nothing is registered.
func AssignIssueToEpic(e *harness.Env, epic Epic, project Project, issue Issue) {
	e.T.Helper()

	child, _, err := e.Client().GL().WorkItems.GetWorkItem(project.Path, issue.IID, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading issue #%d of %s as a work item: %v", issue.IID, project.Path, err)
	}

	var updated struct {
		WorkItemUpdate struct {
			Errors []string `json:"errors"`
		} `json:"workItemUpdate"`
	}
	mutateErr := mutate(e, assignEpicChildMutation, map[string]any{
		"id":          workItemGID(epic.ID),
		"childrenIds": []string{workItemGID(child.ID)},
	}, &updated)
	if mutateErr != nil {
		e.T.Fatalf("assigning issue #%d of %s to epic &%d: %v", issue.IID, project.Path, epic.IID, mutateErr)
	}
	if refusals := updated.WorkItemUpdate.Errors; len(refusals) > 0 {
		e.T.Fatalf("GitLab refused to assign issue #%d to epic &%d: %s", issue.IID, epic.IID, strings.Join(refusals, "; "))
	}
}

// workItemGID spells a work item's numeric identifier as the global id every
// work item mutation takes.
func workItemGID(id int64) string {
	return "gid://gitlab/WorkItem/" + strconv.FormatInt(id, 10)
}

// gidNumber reads the number off the end of a global id.
func gidNumber(gid string) (int64, error) {
	slash := strings.LastIndex(gid, "/")
	if slash < 0 || slash+1 >= len(gid) {
		return 0, fmt.Errorf("%q is not a global id", gid)
	}
	return strconv.ParseInt(gid[slash+1:], 10, 64)
}

// discussionHex reads the hexadecimal identifier off a discussion's global
// id, which is what the epic discussion actions take: GitLab accepts either,
// and the hexadecimal one is what its own listings show.
func discussionHex(gid string) string {
	slash := strings.LastIndex(gid, "/")
	if slash < 0 || slash+1 >= len(gid) {
		return gid
	}
	return gid[slash+1:]
}
