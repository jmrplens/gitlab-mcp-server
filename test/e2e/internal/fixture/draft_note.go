//go:build e2e

// draft_note.go builds an unpublished draft note on a merge request.
//
// A draft note belongs to the person who wrote it and is visible to nobody
// else until it is published, which is what makes it a fixture rather than a
// side effect: the run's own token wrote it, so the run's own session is what
// can read, publish or delete it.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// draftNoteBody is what every fixture draft note holds.
const draftNoteBody = "e2e draft note fixture: unpublished."

// DraftNote is a draft note a builder wrote on a merge request.
type DraftNote struct {
	// ID is what every draft note action takes.
	ID int64
	// Note is the text it holds.
	Note string
}

// NewDraftNote writes an unpublished draft note on the merge request and
// registers its deletion.
func NewDraftNote(e *harness.Env, project Project, mergeRequestIID int64) DraftNote {
	e.T.Helper()

	note, err := retryTransient(e, "create draft note", createRetries, func() (DraftNote, error) {
		return createDraftNote(e.Ctx, e.Client(), project.ID, mergeRequestIID)
	})
	if err != nil {
		e.T.Fatalf("writing a draft note on merge request !%d of project %d: %v", mergeRequestIID, project.ID, err)
	}

	e.Defer(fmt.Sprintf("draft note %d", note.ID), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteDraftNote(ctx, e.Client(), project.ID, mergeRequestIID, note.ID)
	})
	return note
}

// createDraftNote asks GitLab for the note.
func createDraftNote(ctx context.Context, client *gitlabclient.Client, projectID, mergeRequestIID int64) (DraftNote, error) {
	created, _, err := client.GL().DraftNotes.CreateDraftNote(projectID, mergeRequestIID,
		&gl.CreateDraftNoteOptions{Note: new(draftNoteBody)}, gl.WithContext(ctx))
	if err != nil {
		return DraftNote{}, err
	}
	return DraftNote{ID: created.ID, Note: created.Note}, nil
}

// deleteDraftNote removes the note and tolerates one a case deleted or
// published.
func deleteDraftNote(ctx context.Context, client *gitlabclient.Client, projectID, mergeRequestIID, noteID int64) error {
	_, err := client.GL().DraftNotes.DeleteDraftNote(projectID, mergeRequestIID, noteID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting draft note %d of merge request !%d: %w", noteID, mergeRequestIID, err)
	}
	return nil
}

// DraftNoteExists reports whether the merge request still holds the draft
// note, which is what a case that publishes or deletes one is verified
// against.
func DraftNoteExists(ctx context.Context, client *gitlabclient.Client, projectID, mergeRequestIID, noteID int64) (bool, error) {
	_, _, err := client.GL().DraftNotes.GetDraftNote(projectID, mergeRequestIID, noteID, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading draft note %d of merge request !%d: %w", noteID, mergeRequestIID, err)
	}
	return true, nil
}
