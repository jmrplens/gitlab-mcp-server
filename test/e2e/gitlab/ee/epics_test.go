// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// epics_test.go covers the epic family of the group tool: an epic's own
// lifecycle, its notes, its discussions, the issues it collects, its child
// links, the epic boards of its group and the label events GitLab records
// on it. Every scenario runs on the three surfaces against an epic the
// fixture library built over the work items API, which is the only route
// GitLab 19 leaves to one.

package ee

import (
	"slices"
	"strings"
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

// TestEpics_Update_RefusesAnImpossibleDateAndRemovesDatesAndHealthStatus
// creates an epic with a start date, a due date and a health status on every
// surface, asks the update for a date that does not exist and holds it to a
// refusal naming the field, then removes all three in one update with empty
// strings and holds the answer to none of them (issue 1302).
//
// Before the fix the impossible date was dropped and the unchanged epic came
// back as a success, and "" sent nothing, so the values stayed.
func TestEpics_Update_RefusesAnImpossibleDateAndRemovesDatesAndHealthStatus(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("epicdates"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		created := harness.Do[epics.Output](s, actionEpicCreate, map[string]any{
			"full_path": group.Path, "title": e.Name("epic"),
			"start_date": "2026-11-01", "due_date": "2026-12-31", "health_status": "onTrack",
		})
		if created.StartDate == "" || created.DueDate == "" || created.HealthStatus == "" {
			e.T.Fatalf("epic_create answered start %q, due %q, health %q, want all three set",
				created.StartDate, created.DueDate, created.HealthStatus)
		}
		params := epicParams(fixture.Epic{IID: created.IID, GroupPath: group.Path})

		refused := harness.ExpectToolError(s, actionEpicUpdate, withParams(params, map[string]any{"due_date": "2026-02-30"}), "due_date")
		e.T.Logf("the impossible due date was refused: %s", firstLine(refused))

		cleared := harness.Do[epics.Output](s, actionEpicUpdate,
			withParams(params, map[string]any{"start_date": "", "due_date": "", "health_status": ""}))
		if cleared.StartDate != "" || cleared.DueDate != "" || cleared.HealthStatus != "" {
			e.T.Errorf("epic_update answered start %q, due %q, health %q, want all three removed",
				cleared.StartDate, cleared.DueDate, cleared.HealthStatus)
		}
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
		assertMentions(e, "the read of a missing epic board", refused, "board_id", "group.epic_board_list", "configure an epic board")
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
// it, deletes it and checks the listing no longer holds it. Neither the note
// nor its edit names a command, so neither answer carries a quick actions
// status.
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
		// GitLab answers a body without a command with an empty quick actions
		// status rather than none, which must not reach the output.
		if created.QuickActionsStatus != nil {
			e.T.Errorf("epic_note_create answered a body without a quick action with the status %+v, want none", *created.QuickActionsStatus)
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
		if updated.QuickActionsStatus != nil {
			e.T.Errorf("epic_note_update answered a body without a quick action with the status %+v, want none", *updated.QuickActionsStatus)
		}

		harness.DoVoid(s, actionEpicNoteDelete, note)
		after := harness.Do[epicnotes.ListOutput](s, actionEpicNoteList, params)
		if containsID(epicNoteIDs(after.Notes), created.ID) {
			e.T.Errorf("the epic's notes still hold note %d after its delete", created.ID)
		}
	})
}

// TestEpicNotes_QuickActions_TheStatusReachesTheOutput writes two notes on an
// epic of its own per surface, one mixing text with a /label quick action and
// one holding a quick action alone, and checks that what GitLab reports about
// each reaches the output: the note kept beside the command it ran and what
// the command did, then no note at all beside a status saying the body held
// only commands. The epic is read back between the two, so the label the first
// note put on it is GitLab's own evidence that the reported command ran.
//
// Before the note mutations selected the status, the first note came back
// with no word about its command and the second failed as a missing note
// although GitLab had run the command.
func TestEpicNotes_QuickActions_TheStatusReachesTheOutput(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicLabelFixture, func(e *harness.Env, surface harness.Surface, f epicLabelFixture) {
		s := e.On(surface)
		epic := fixture.NewEpic(e, f.group, e.Name("epic"))
		params := epicParams(epic)
		label := `~"` + f.label.Name + `"`

		mixed := harness.Do[epicnotes.Output](s, actionEpicNoteCreate, withParams(params, map[string]any{
			"body": "triaged from the " + string(surface) + " surface\n/label " + label,
		}))
		if mixed.ID == 0 {
			e.T.Fatalf("epic_note_create answered %+v, want the note GitLab kept beside the command", mixed)
		}
		status := mixed.QuickActionsStatus
		if status == nil {
			e.T.Fatalf("epic_note_create answered note %d with no quick actions status, want the status of the /label it carried", mixed.ID)
		}
		if !slices.Contains(status.CommandNames, "label") || status.CommandsOnly || len(status.Messages) == 0 || len(status.ErrorMessages) != 0 {
			e.T.Errorf("the quick actions status is %+v, want the label command applied, a message, no failure and the note kept", *status)
		}
		afterLabel := harness.Do[epics.Output](s, actionEpicGet, params)
		if !slices.Contains(afterLabel.Labels, f.label.Name) {
			e.T.Errorf("the epic carries the labels %v after the /label, want %q among them", afterLabel.Labels, f.label.Name)
		}

		only := harness.Do[epicnotes.Output](s, actionEpicNoteCreate, withParams(params, map[string]any{"body": "/unlabel " + label}))
		if only.ID != 0 {
			e.T.Errorf("epic_note_create kept note %d for a body of quick actions alone, want none", only.ID)
		}
		if only.QuickActionsStatus == nil || !only.QuickActionsStatus.CommandsOnly || !slices.Contains(only.QuickActionsStatus.CommandNames, "unlabel") {
			e.T.Errorf("the quick actions status of a body of commands alone is %+v, want the unlabel command reported as the whole body", only.QuickActionsStatus)
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

// TestEpicNotes_WritesNamingAnotherEpic_AreRefusedAndChangeNothing writes a
// note on the fixture epic and opens a second epic per surface, then asks
// each of the four note writes, through the second epic, to edit or delete
// the first epic's note (issue 1301). GitLab addresses a note by its ID
// alone, so each used to change the note whichever epic held it and report
// the epic the caller named. Each must be refused naming where the note is,
// and the note must still be on the first epic with its body.
func TestEpicNotes_WritesNamingAnotherEpic_AreRefusedAndChangeNothing(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicFixture("epicnoteown"), func(e *harness.Env, surface harness.Surface, f epicFixture) {
		s := e.On(surface)
		note := fixture.NewEpicNote(e, f.epic)
		other := fixture.NewEpic(e, f.group, e.Name("other"))
		elsewhere := withParams(epicParams(other), map[string]any{"note_id": note.ID})

		for _, write := range []struct {
			action harness.ActionID
			params map[string]any
		}{
			{actionEpicNoteUpdate, withParams(elsewhere, map[string]any{"body": "moved"})},
			{actionEpicNoteDelete, elsewhere},
			{actionEpicDiscussionUpdateNote, withParams(elsewhere, map[string]any{"body": "moved"})},
			{actionEpicDiscussionDeleteNote, elsewhere},
		} {
			harness.ExpectToolError(s, write.action, write.params, "is not a note of epic")
		}

		kept := harness.Do[epicnotes.Output](s, actionEpicNoteGet, withParams(epicParams(f.epic), map[string]any{"note_id": note.ID}))
		if kept.Body != note.Body {
			e.T.Errorf("the note on the first epic reads %q after writes that named another epic, want %q", kept.Body, note.Body)
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

		removed := harness.Do[epicissues.RemoveOutput](s, actionEpicIssueRemove, child(first))
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

// TestEpicIssues_RemoveAndReorderNamingAnotherEpic_AreRefusedAndChangeNothing
// puts one issue in epic A and another in epic B of their own per surface,
// then asks epic B to remove A's issue, to reorder its own issue beside A's
// and to reorder A's issue beside its own (issue 1301). Removal used to clear
// the issue's parent whichever epic that was, and reordering moved A's issue
// into B, both reported as done.
// Each must be refused naming epic A, and A must still hold its issue.
func TestEpicIssues_RemoveAndReorderNamingAnotherEpic_AreRefusedAndChangeNothing(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicIssueFixture, func(e *harness.Env, surface harness.Surface, f epicIssueFixture) {
		s := e.On(surface)
		epicA := fixture.NewEpic(e, f.group, e.Name("epic-a"))
		epicB := fixture.NewEpic(e, f.group, e.Name("epic-b"))
		inA := fixture.NewIssue(e, f.project, e.Name("in-a"))
		inB := fixture.NewIssue(e, f.project, e.Name("in-b"))
		child := func(epic fixture.Epic, issue fixture.Issue) map[string]any {
			return withParams(epicParams(epic), map[string]any{"child_project_path": f.project.Path, "child_iid": issue.IID})
		}
		harness.DoVoid(s, actionEpicIssueAssign, child(epicA, inA))
		harness.DoVoid(s, actionEpicIssueAssign, child(epicB, inB))
		inEpicA := harness.Do[epicissues.ListOutput](s, actionEpicIssueList, epicParams(epicA))
		childA, found := childByIID(inEpicA.Issues, inA.IID)
		if !found {
			e.T.Fatalf("epic A does not hold issue #%d: %+v", inA.IID, inEpicA.Issues)
		}
		inEpicB := harness.Do[epicissues.ListOutput](s, actionEpicIssueList, epicParams(epicB))
		childB, found := childByIID(inEpicB.Issues, inB.IID)
		if !found {
			e.T.Fatalf("epic B does not hold issue #%d: %+v", inB.IID, inEpicB.Issues)
		}

		harness.ExpectToolError(s, actionEpicIssueRemove, child(epicB, inA), "is a child of")
		// Both directions of a reorder: B's own issue beside A's, and A's issue
		// beside B's own, which is the call that used to move A's issue into B.
		harness.ExpectToolError(s, actionEpicIssueUpdate, withParams(epicParams(epicB), map[string]any{
			"child_id": childB.ID, "adjacent_id": childA.ID, "relative_position": "AFTER",
		}), "is a child of")
		harness.ExpectToolError(s, actionEpicIssueUpdate, withParams(epicParams(epicB), map[string]any{
			"child_id": childA.ID, "adjacent_id": childB.ID, "relative_position": "AFTER",
		}), "is a child of")

		after := harness.Do[epicissues.ListOutput](s, actionEpicIssueList, epicParams(epicA))
		if _, still := childByIID(after.Issues, inA.IID); !still {
			e.T.Errorf("epic A lost issue #%d to a call that named epic B: %+v", inA.IID, after.Issues)
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

// TestEpicNotes_LabelEvent_IsNamedByItsGlobalIDNotNoteZero puts the fixture
// label on an epic of its own per surface and reads the epic's notes and
// threads, where GitLab answers the label change as a note it synthesizes
// from the event (issue 1304). That note has a global ID ending in the
// hexadecimal discussion ID rather than a number, and both listings used to
// publish it as note 0, an ID GitLab never assigned and every event shared.
// Each listing must carry the event with its global ID and marked as one,
// and no note anywhere with id 0 and nothing to name it by.
func TestEpicNotes_LabelEvent_IsNamedByItsGlobalIDNotNoteZero(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildEpicLabelFixture, func(e *harness.Env, surface harness.Surface, f epicLabelFixture) {
		s := e.On(surface)
		epic := fixture.NewEpic(e, f.group, e.Name("epic"))
		params := epicParams(epic)
		harness.DoVoid(s, actionEpicUpdate, withParams(params, map[string]any{"add_label_ids": []int64{f.label.ID}}))

		notes := harness.Eventually(s, actionEpicNoteList, params, resourceEventInterval, resourceEventWait,
			func(out epicnotes.ListOutput) bool {
				return slices.ContainsFunc(out.Notes, func(n epicnotes.Output) bool { return n.ResourceEvent })
			})
		for _, n := range notes.Notes {
			assertNoteNamed(e, "epic_note_list", n.ID, n.GID, n.ResourceEvent)
		}

		threads := harness.Do[epicdiscussions.ListOutput](s, actionEpicDiscussionList, params)
		events := 0
		for _, thread := range threads.Discussions {
			for _, n := range thread.Notes {
				assertNoteNamed(e, "epic_discussion_list", n.ID, n.GID, n.ResourceEvent)
				if n.ResourceEvent {
					events++
				}
			}
		}
		if events == 0 {
			e.T.Errorf("epic_discussion_list answered no resource event for the label put on the epic: %+v", threads.Discussions)
		}
	})
}

// assertNoteNamed checks that a listed epic note can be named: a note with
// an ID is one, and a note without one is a resource event carrying the
// global ID GitLab gave it.
func assertNoteNamed(e *harness.Env, action string, id int64, gid string, resourceEvent bool) {
	e.T.Helper()
	if id == 0 && !resourceEvent {
		e.T.Errorf("%s answered note 0 that is not marked as a resource event (gid %q)", action, gid)
	}
	if resourceEvent && !strings.HasPrefix(gid, "gid://gitlab/") {
		e.T.Errorf("%s answered a resource event without the global ID GitLab gave it: %q", action, gid)
	}
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
