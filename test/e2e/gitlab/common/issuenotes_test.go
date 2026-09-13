//go:build e2e

// issuenotes_test.go covers the notes of an issue through the server:
// create, list, read, update and delete, once per surface on one shared
// issue, since a note belongs to the surface that wrote it.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// issueFixture is a project with one issue in it.
type issueFixture struct {
	project fixture.Project
	issue   fixture.Issue
}

// newIssueFixture creates the project and the issue.
func newIssueFixture(e *harness.Env, prefix string) issueFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix(prefix))
	issue := fixture.NewIssue(e, project, prefix+" fixture")
	return issueFixture{project: project, issue: issue}
}

// params spells the two parameters every issue-scoped action takes.
func (f issueFixture) params() map[string]any {
	return map[string]any{"project_id": f.project.IDParam(), "issue_iid": f.issue.IID}
}

// noteIDs lists the IDs of a note listing. The issue, merge request and
// snippet note packages all alias the one shared note shape, so one helper
// reads the three listings.
func noteIDs(notes []toolutil.NoteOutput) []int64 {
	ids := make([]int64, 0, len(notes))
	for _, note := range notes {
		ids = append(ids, note.ID)
	}
	return ids
}

// noteBodies lists the bodies of a note listing.
func noteBodies(notes []toolutil.NoteOutput) []string {
	bodies := make([]string, 0, len(notes))
	for _, note := range notes {
		bodies = append(bodies, note.Body)
	}
	return bodies
}

// TestIssueNotes_Lifecycle_CreateListGetUpdateDelete writes a note on the
// fixture issue on every surface, finds it in the listing, reads it back,
// changes its body, deletes it and checks the listing no longer holds it.
//
// Replaces: TestIndividual_IssueNotes, TestMeta_IssueNotes
func TestIssueNotes_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issuenotes")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.On(surface)
		params := f.params()
		body := "note from the " + string(surface) + " surface"

		created := harness.Do[issuenotes.Output](s, actionIssueNoteCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == 0 || created.Body != body {
			e.T.Fatalf("note_create answered %+v, want the note %q with an ID", created, body)
		}
		noteParams := withParams(params, map[string]any{"note_id": created.ID})

		listed := harness.Do[issuenotes.ListOutput](s, actionIssueNoteList, params)
		if !containsID(noteIDs(listed.Notes), created.ID) {
			e.T.Errorf("the issue's notes do not hold %d: %v", created.ID, noteIDs(listed.Notes))
		}

		got := harness.Do[issuenotes.Output](s, actionIssueNoteGet, noteParams)
		if got.ID != created.ID || got.Body != body {
			e.T.Errorf("note_get answered %+v, want note %d with body %q", got, created.ID, body)
		}

		updated := harness.Do[issuenotes.Output](s, actionIssueNoteUpdate, withParams(noteParams, map[string]any{"body": body + " (edited)"}))
		if updated.ID != created.ID || updated.Body != body+" (edited)" {
			e.T.Errorf("note_update answered %+v, want note %d with the edited body", updated, created.ID)
		}

		harness.DoVoid(s, actionIssueNoteDelete, noteParams)
		remaining := harness.Do[issuenotes.ListOutput](s, actionIssueNoteList, params)
		if containsID(noteIDs(remaining.Notes), created.ID) {
			e.T.Errorf("note %d is still listed after its delete", created.ID)
		}
	})
}
