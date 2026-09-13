//go:build e2e

// snippetnotes_test.go covers the notes of a project snippet through the
// server: create, list, read, update and delete, once per surface on one
// shared snippet the fixture library built.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// projectSnippetFixture is a project with one snippet in it.
type projectSnippetFixture struct {
	project fixture.Project
	snippet fixture.ProjectSnippet
}

// newProjectSnippetFixture creates the project and the snippet.
func newProjectSnippetFixture(e *harness.Env, prefix string) projectSnippetFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix(prefix))
	return projectSnippetFixture{project: project, snippet: fixture.NewProjectSnippet(e, project)}
}

// params spells the two parameters every snippet-scoped action takes.
func (f projectSnippetFixture) params() map[string]any {
	return map[string]any{"project_id": f.project.IDParam(), "snippet_id": f.snippet.ID}
}

// TestSnippetNotes_Lifecycle_CreateListGetUpdateDelete writes a note on
// the fixture snippet on every surface, finds it in the listing, reads it
// back, changes its body, deletes it and checks the listing no longer holds
// it.
//
// Replaces: TestMeta_SnippetNotes
func TestSnippetNotes_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) projectSnippetFixture {
		return newProjectSnippetFixture(e, "snippetnotes")
	}, func(e *harness.Env, surface harness.Surface, f projectSnippetFixture) {
		s := e.On(surface)
		params := f.params()
		body := "note from the " + string(surface) + " surface"

		created := harness.Do[snippetnotes.Output](s, actionSnippetNoteCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == 0 || created.Body != body {
			e.T.Fatalf("note_create answered %+v, want the note %q with an ID", created, body)
		}
		noteParams := withParams(params, map[string]any{"note_id": created.ID})

		listed := harness.Do[snippetnotes.ListOutput](s, actionSnippetNoteList, params)
		if !containsID(noteIDs(listed.Notes), created.ID) {
			e.T.Errorf("the snippet's notes do not hold %d: %v", created.ID, noteIDs(listed.Notes))
		}

		got := harness.Do[snippetnotes.Output](s, actionSnippetNoteGet, noteParams)
		if got.ID != created.ID || got.Body != body {
			e.T.Errorf("note_get answered %+v, want note %d with body %q", got, created.ID, body)
		}

		updated := harness.Do[snippetnotes.Output](s, actionSnippetNoteUpdate, withParams(noteParams, map[string]any{"body": body + " (edited)"}))
		if updated.ID != created.ID || updated.Body != body+" (edited)" {
			e.T.Errorf("note_update answered %+v, want note %d with the edited body", updated, created.ID)
		}

		harness.DoVoid(s, actionSnippetNoteDelete, noteParams)
		remaining := harness.Do[snippetnotes.ListOutput](s, actionSnippetNoteList, params)
		if containsID(noteIDs(remaining.Notes), created.ID) {
			e.T.Errorf("note %d is still listed after its delete", created.ID)
		}
	})
}
