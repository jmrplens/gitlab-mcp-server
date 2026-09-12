//go:build e2e

// epics_test.go covers the epic family of the group tool: an epic's own
// lifecycle, its notes, its discussions, the issues it collects, its child
// links, the epic boards of its group and the label events GitLab records
// on it. Every scenario runs on the three surfaces against an epic the
// fixture library built over the work items API, which is the only route
// GitLab 19 leaves to one.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epics"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupepicboards"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingBoardID is a board number no fresh group has, for the refusal of a
// read that names one.
const missingBoardID = int64(999999999)

// epicParams names one epic the way every epic action does.
func epicParams(epic fixture.Epic) map[string]any {
	return map[string]any{"full_path": epic.GroupPath, "epic_iid": epic.IID}
}

// epicIIDs lists the iids of an epic listing.
func epicIIDs(listed []epics.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, epic := range listed {
		ids = append(ids, epic.IID)
	}
	return ids
}

// TestEpics_Lifecycle_CreateListGetUpdateDelete creates an epic on every
// surface in a shared group, finds it in the listing, reads it back,
// changes its description, deletes it and checks the read is then refused.
//
// Replaces: TestMeta_Epics
func TestEpics_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("epics"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		title := e.Name("epic")

		created := harness.Do[epics.Output](s, actionEpicCreate, map[string]any{
			"full_path": group.Path, "title": title, "description": "created by the e2e suite",
		})
		if created.IID == 0 || created.Title != title {
			e.T.Fatalf("epic_create answered %+v, want an epic titled %q with an iid", created, title)
		}
		epic := fixture.Epic{IID: created.IID, ID: created.ID, Title: title, GroupPath: group.Path}

		listed := harness.Do[epics.ListOutput](s, actionEpicList, map[string]any{"full_path": group.Path})
		if !containsID(epicIIDs(listed.Epics), epic.IID) {
			e.T.Errorf("the group's epics do not hold the created epic %d: %v", epic.IID, epicIIDs(listed.Epics))
		}

		got := harness.Do[epics.Output](s, actionEpicGet, epicParams(epic))
		if got.IID != epic.IID || got.Title != title {
			e.T.Errorf("epic_get answered epic %d %q, want %d %q", got.IID, got.Title, epic.IID, title)
		}

		updated := harness.Do[epics.Output](s, actionEpicUpdate, withParams(epicParams(epic), map[string]any{"description": "updated by the e2e suite"}))
		if updated.Description != "updated by the e2e suite" {
			e.T.Errorf("epic_update answered the description %q, want the one just written", updated.Description)
		}

		harness.DoVoid(s, actionEpicDelete, epicParams(epic))
		refused := harness.Refused(s, actionEpicGet, epicParams(epic), harness.FailureNotFound)
		e.T.Logf("the read of the deleted epic was refused: %s", firstLine(refused))
	})
}

// TestEpicBoards_FreshGroup_ListsAndRefusesAMissingBoard lists the epic
// boards of a fresh group on every surface, reads the first one back when
// the instance seeded one, and asks for a board number nothing has, which
// is refused with the hint naming the listing to consult.
//
// Replaces: TestMeta_EpicBoards, TestMeta_GroupEpicBoards, TestMeta_Epics
func TestEpicBoards_FreshGroup_ListsAndRefusesAMissingBoard(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("epicboards"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		params := map[string]any{"group_id": group.IDParam()}

		listed := harness.Do[groupepicboards.ListOutput](s, actionEpicBoardList, params)
		e.T.Logf("the fresh group lists %d epic board(s)", len(listed.Boards))
		if len(listed.Boards) > 0 {
			got := harness.Do[groupepicboards.Output](s, actionEpicBoardGet, withParams(params, map[string]any{"board_id": listed.Boards[0].ID}))
			if got.ID != listed.Boards[0].ID {
				e.T.Errorf("epic_board_get answered board %d, want the listed %d", got.ID, listed.Boards[0].ID)
			}
		}

		refused := harness.Refused(s, actionEpicBoardGet, withParams(params, map[string]any{"board_id": missingBoardID}), harness.FailureNotFound)
		assertMentions(e, "the read of a missing epic board", refused, "epic_board_list", "gitlab_group", "configure an epic board")
	})
}

// epicFixture is a group with one epic the fixture library built in it.
type epicFixture struct {
	group fixture.Group
	epic  fixture.Epic
}

// buildEpicFixture creates the group and the epic.
func buildEpicFixture(prefix string) func(*harness.Env) epicFixture {
	return func(e *harness.Env) epicFixture {
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix(prefix))
		return epicFixture{group: group, epic: fixture.NewEpic(e, group, e.Name("epic"))}
	}
}

// epicNoteIDs lists the ids of an epic note listing.
func epicNoteIDs(notes []epicnotes.Output) []int64 {
	ids := make([]int64, 0, len(notes))
	for _, note := range notes {
		ids = append(ids, note.ID)
	}
	return ids
}

// TestEpicNotes_Lifecycle_CreateListGetUpdateDelete writes one note on the
// fixture epic per surface, finds it in the listing, reads it back, edits
// it, deletes it and checks the listing no longer holds it.
//
// Replaces: TestMeta_EpicNotes
func TestEpicNotes_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicFixture("epicnotes"), func(e *harness.Env, surface harness.Surface, f epicFixture) {
		s := e.On(surface)
		params := epicParams(f.epic)
		body := "note from the " + string(surface) + " surface"

		created := harness.Do[epicnotes.Output](s, actionEpicNoteCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == 0 || created.Body != body {
			e.T.Fatalf("epic_note_create answered %+v, want a note with an ID carrying %q", created, body)
		}
		note := withParams(params, map[string]any{"note_id": created.ID})

		listed := harness.Do[epicnotes.ListOutput](s, actionEpicNoteList, params)
		if !containsID(epicNoteIDs(listed.Notes), created.ID) {
			e.T.Errorf("the epic's notes do not hold the created note %d: %v", created.ID, epicNoteIDs(listed.Notes))
		}
		got := harness.Do[epicnotes.Output](s, actionEpicNoteGet, note)
		if got.ID != created.ID {
			e.T.Errorf("epic_note_get answered note %d, want %d", got.ID, created.ID)
		}
		updated := harness.Do[epicnotes.Output](s, actionEpicNoteUpdate, withParams(note, map[string]any{"body": "updated " + body}))
		if updated.Body != "updated "+body {
			e.T.Errorf("epic_note_update answered the body %q, want the one just written", updated.Body)
		}

		harness.DoVoid(s, actionEpicNoteDelete, note)
		after := harness.Do[epicnotes.ListOutput](s, actionEpicNoteList, params)
		if containsID(epicNoteIDs(after.Notes), created.ID) {
			e.T.Errorf("the epic's notes still hold note %d after its delete", created.ID)
		}
	})
}

// discussionNoteIDs lists the ids of the notes of one discussion.
func discussionNoteIDs(notes []epicdiscussions.NoteOutput) []int64 {
	ids := make([]int64, 0, len(notes))
	for _, note := range notes {
		ids = append(ids, note.ID)
	}
	return ids
}

// TestEpicDiscussions_Lifecycle_ThreadAndReply opens a discussion on the
// fixture epic per surface, finds it in the listing, reads it back, replies
// in it, edits the reply, deletes the reply and checks the thread no longer
// holds it.
//
// Replaces: TestMeta_EpicDiscussions
func TestEpicDiscussions_Lifecycle_ThreadAndReply(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicFixture("epicdisc"), func(e *harness.Env, surface harness.Surface, f epicFixture) {
		s := e.On(surface)
		params := epicParams(f.epic)

		created := harness.Do[epicdiscussions.Output](s, actionEpicDiscussionCreate, withParams(params, map[string]any{"body": "thread from " + string(surface)}))
		if created.ID == "" || len(created.Notes) == 0 {
			e.T.Fatalf("epic_discussion_create answered %+v, want a discussion with an ID and its first note", created)
		}
		thread := withParams(params, map[string]any{"discussion_id": created.ID})

		listed := harness.Do[epicdiscussions.ListOutput](s, actionEpicDiscussionList, params)
		found := false
		for _, discussion := range listed.Discussions {
			if discussion.ID == created.ID {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the epic's discussions do not hold the created thread %s among %d", created.ID, len(listed.Discussions))
		}
		got := harness.Do[epicdiscussions.Output](s, actionEpicDiscussionGet, thread)
		if got.ID != created.ID {
			e.T.Errorf("epic_discussion_get answered thread %s, want %s", got.ID, created.ID)
		}

		reply := harness.Do[epicdiscussions.NoteOutput](s, actionEpicDiscussionAddNote, withParams(thread, map[string]any{"body": "reply"}))
		if reply.ID == 0 || reply.Body != "reply" {
			e.T.Fatalf("epic_discussion_add_note answered %+v, want a reply with an ID", reply)
		}
		note := withParams(params, map[string]any{"note_id": reply.ID})
		updated := harness.Do[epicdiscussions.NoteOutput](s, actionEpicDiscussionUpdateNote, withParams(note, map[string]any{"body": "updated reply"}))
		if updated.Body != "updated reply" {
			e.T.Errorf("epic_discussion_update_note answered the body %q, want the one just written", updated.Body)
		}

		harness.DoVoid(s, actionEpicDiscussionDeleteNote, note)
		after := harness.Do[epicdiscussions.Output](s, actionEpicDiscussionGet, thread)
		if containsID(discussionNoteIDs(after.Notes), reply.ID) {
			e.T.Errorf("the thread still holds the reply %d after its delete", reply.ID)
		}
	})
}

// epicIssueFixture is a group with a project in it, so the issues an epic
// collects have somewhere to live.
type epicIssueFixture struct {
	group   fixture.Group
	project fixture.Project
}

// buildEpicIssueFixture creates the group and the project.
func buildEpicIssueFixture(e *harness.Env) epicIssueFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("epiciss"))
	project := fixture.NewProject(e, fixture.WithNamePrefix("epiciss"), fixture.InGroup(group))
	return epicIssueFixture{group: group, project: project}
}

// childByIID finds one child of an epic by the issue's iid.
func childByIID(children []epicissues.ChildOutput, iid int64) (epicissues.ChildOutput, bool) {
	for _, child := range children {
		if child.IID == iid {
			return child, true
		}
	}
	return epicissues.ChildOutput{}, false
}

// TestEpicIssues_AssignRemoveAndReorder_ChildrenFollow puts an issue into
// an epic of its own per surface, lists it as a child, takes it out again,
// then puts two issues in and reorders them.
//
// Replaces: TestMeta_EpicIssues
func TestEpicIssues_AssignRemoveAndReorder_ChildrenFollow(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicIssueFixture, func(e *harness.Env, surface harness.Surface, f epicIssueFixture) {
		s := e.On(surface)
		epic := fixture.NewEpic(e, f.group, e.Name("epic"))
		first := fixture.NewIssue(e, f.project, e.Name("first"))
		second := fixture.NewIssue(e, f.project, e.Name("second"))
		params := epicParams(epic)
		child := func(issue fixture.Issue) map[string]any {
			return withParams(params, map[string]any{"child_project_path": f.project.Path, "child_iid": issue.IID})
		}

		assigned := harness.Do[epicissues.AssignOutput](s, actionEpicIssueAssign, child(first))
		if assigned.EpicGID == "" || assigned.ChildGID == "" {
			e.T.Fatalf("epic_issue_assign answered %+v, want both global IDs", assigned)
		}
		listed := harness.Do[epicissues.ListOutput](s, actionEpicIssueList, params)
		if _, found := childByIID(listed.Issues, first.IID); !found {
			e.T.Errorf("the epic's children do not hold issue #%d: %+v", first.IID, listed.Issues)
		}

		removed := harness.Do[epicissues.AssignOutput](s, actionEpicIssueRemove, child(first))
		if removed.EpicGID == "" {
			e.T.Errorf("epic_issue_remove answered %+v, want the epic's global ID", removed)
		}
		empty := harness.Do[epicissues.ListOutput](s, actionEpicIssueList, params)
		if len(empty.Issues) != 0 {
			e.T.Errorf("the epic still has %d child issue(s) after the remove: %+v", len(empty.Issues), empty.Issues)
		}

		harness.DoVoid(s, actionEpicIssueAssign, child(first))
		harness.DoVoid(s, actionEpicIssueAssign, child(second))
		both := harness.Do[epicissues.ListOutput](s, actionEpicIssueList, params)
		firstChild, foundFirst := childByIID(both.Issues, first.IID)
		secondChild, foundSecond := childByIID(both.Issues, second.IID)
		if !foundFirst || !foundSecond {
			e.T.Fatalf("the epic's children do not hold both issues #%d and #%d: %+v", first.IID, second.IID, both.Issues)
		}
		reordered := harness.Do[epicissues.ListOutput](s, actionEpicIssueUpdate, withParams(params, map[string]any{
			"child_id": firstChild.ID, "adjacent_id": secondChild.ID, "relative_position": "AFTER",
		}))
		if len(reordered.Issues) < 2 {
			e.T.Errorf("epic_issue_update answered %d child issue(s), want both", len(reordered.Issues))
		}
	})
}

// TestEpicLinks_FreshEpic_HasNoChildEpics reads the child epics of a fresh
// epic on every surface and checks there are none.
//
// Replaces: TestMeta_EpicLinks
func TestEpicLinks_FreshEpic_HasNoChildEpics(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicFixture("epiclinks"), func(e *harness.Env, surface harness.Surface, f epicFixture) {
		s := e.On(surface)
		links := harness.Do[epics.LinksOutput](s, actionEpicGetLinks, epicParams(f.epic))
		if len(links.ChildEpics) != 0 {
			e.T.Errorf("a fresh epic reports %d child epic(s), want none: %+v", len(links.ChildEpics), links.ChildEpics)
		}
	})
}

// epicLabelFixture is a group with a label to put on epics.
type epicLabelFixture struct {
	group fixture.Group
	label fixture.GroupLabel
}

// buildEpicLabelFixture creates the group and the label.
func buildEpicLabelFixture(e *harness.Env) epicLabelFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("epiclabel"))
	return epicLabelFixture{group: group, label: fixture.NewGroupLabel(e, group, "epic")}
}

// TestEpicLabelEvents_LabelAddedAndRemoved_EventsOrTheDocumentedRefusal
// puts the fixture label on an epic of its own per surface and takes it
// off, then asks for the label events. On GitLab 19 and later the epic
// REST endpoints are gone, so the listing and the read are refused with
// the hint that says where epics went; on an earlier release the two
// events are listed and the first is read back.
//
// Replaces: TestMeta_GroupEpicLabelEvents
func TestEpicLabelEvents_LabelAddedAndRemoved_EventsOrTheDocumentedRefusal(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicLabelFixture, func(e *harness.Env, surface harness.Surface, f epicLabelFixture) {
		s := e.On(surface)
		epic := fixture.NewEpic(e, f.group, e.Name("epic"))
		harness.DoVoid(s, actionEpicUpdate, withParams(epicParams(epic), map[string]any{"add_label_ids": []int64{f.label.ID}}))
		harness.DoVoid(s, actionEpicUpdate, withParams(epicParams(epic), map[string]any{"remove_label_ids": []int64{f.label.ID}}))

		params := map[string]any{"group_id": f.group.IDParam(), "epic_iid": epic.IID}
		if fixture.LegacyEpicRESTRemoved(e) {
			hint := []string{"GitLab 19", "work item"}
			refused := harness.Refused(s, actionEpicLabelEventList, params, harness.FailureNotFound)
			assertMentions(e, "the epic label event listing", refused, hint...)
			refused = harness.Refused(s, actionEpicLabelEventGet, withParams(params, map[string]any{"label_event_id": int64(1)}), harness.FailureNotFound)
			assertMentions(e, "the epic label event read", refused, hint...)
			return
		}

		events := harness.Eventually(s, actionEpicLabelEventList, params, resourceEventInterval, resourceEventWait,
			func(out resourceevents.ListLabelEventsOutput) bool { return len(out.Events) >= 2 })
		got := harness.Do[resourceevents.LabelEventOutput](s, actionEpicLabelEventGet, withParams(params, map[string]any{"label_event_id": events.Events[0].ID}))
		if got.ID != events.Events[0].ID {
			e.T.Errorf("event_epic_label_get answered event %d, want the listed %d", got.ID, events.Events[0].ID)
		}
	})
}
