//go:build e2e

// commitdiscussions_test.go covers the discussion threads a commit carries:
// opening one, listing and reading it, replying to it, editing the reply
// and deleting it.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commitdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// commitNoteIDs returns the identifiers of a thread's notes.
func commitNoteIDs(notes []*commitdiscussions.NoteOutput) []int64 {
	ids := make([]int64, 0, len(notes))
	for _, note := range notes {
		if note != nil {
			ids = append(ids, note.ID)
		}
	}
	return ids
}

// TestCommitDiscussion_Lifecycle_CreateListGetAddUpdateDeleteNote opens a
// discussion on a commit of a shared project on every surface, finds it in
// the listing and reads it back, replies to it, edits the reply, deletes it
// and checks the thread is left with its opening note alone.
//
// Replaces: TestMeta_CommitDiscussions
func TestCommitDiscussion_Lifecycle_CreateListGetAddUpdateDeleteNote(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("cdisc"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		commit := fixture.CommitFile(e, project, project.DefaultBranch, "disc-"+string(surface)+".txt", "discussion content", "chore: a commit to discuss")
		params := map[string]any{"project_id": project.IDParam(), "commit_sha": commit.SHA}
		body := "e2e discussion on " + string(surface)

		created := harness.Do[commitdiscussions.Output](s, actionRepositoryCommitDiscussionCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == "" || len(created.Notes) != 1 || created.Notes[0] == nil || created.Notes[0].Body != body {
			e.T.Fatalf("discussion create answered %+v, want a thread with an ID and the one note %q", created, body)
		}
		thread := withParams(params, map[string]any{"discussion_id": created.ID})

		listed := harness.Do[commitdiscussions.ListOutput](s, actionRepositoryCommitDiscussionList, params)
		if !slices.ContainsFunc(listed.Discussions, func(d commitdiscussions.Output) bool { return d.ID == created.ID }) {
			e.T.Errorf("the discussions of %s do not hold %s: %d listed", commit.ShortID, created.ID, len(listed.Discussions))
		}

		got := harness.Do[commitdiscussions.Output](s, actionRepositoryCommitDiscussionGet, thread)
		if got.ID != created.ID || len(got.Notes) != 1 {
			e.T.Errorf("discussion get answered %+v, want thread %s with its one note", got, created.ID)
		}

		reply := harness.Do[commitdiscussions.NoteOutput](s, actionRepositoryCommitDiscussionAddNote, withParams(thread, map[string]any{"body": "e2e reply"}))
		if reply.ID == 0 || reply.Body != "e2e reply" {
			e.T.Fatalf("discussion add note answered %+v, want a note with an ID and the reply body", reply)
		}
		note := withParams(thread, map[string]any{"note_id": reply.ID})

		edited := harness.Do[commitdiscussions.NoteOutput](s, actionRepositoryCommitDiscussionUpdateNote, withParams(note, map[string]any{"body": "e2e edited reply"}))
		if edited.ID != reply.ID || edited.Body != "e2e edited reply" {
			e.T.Errorf("discussion update note answered %+v, want note %d with the edited body", edited, reply.ID)
		}

		harness.DoVoid(s, actionRepositoryCommitDiscussionDeleteNote, note)
		remaining := harness.Do[commitdiscussions.Output](s, actionRepositoryCommitDiscussionGet, thread)
		if containsID(commitNoteIDs(remaining.Notes), reply.ID) {
			e.T.Errorf("the thread still holds note %d after its delete: %v", reply.ID, commitNoteIDs(remaining.Notes))
		}
	})
}
