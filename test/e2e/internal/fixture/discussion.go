//go:build e2e

// discussion.go opens a discussion on a merge request and on a commit.
//
// The two are one file because they are the same object under two parents and
// differ in exactly one way that matters to a case: a merge request
// discussion is addressed by the discussion ID alone, while a commit
// discussion is addressed by the commit as well, so the commit builder
// returns the SHA beside the IDs rather than leaving a caller to find it.
//
// Neither registers an undo: a discussion goes with its parent, and GitLab
// offers no way to delete one apart from deleting each of its notes.

package fixture

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// discussionBody is what every fixture discussion opens with.
const discussionBody = "e2e discussion fixture: the first note of the thread."

// Discussion is a discussion a builder opened, with the note that opened it.
type Discussion struct {
	// ID is what every discussion action takes, and is a hash rather than a
	// number.
	ID string
	// NoteID is the note the discussion opened with, which a note action
	// addresses on its own.
	NoteID int64
	// CommitSHA is the commit a commit discussion hangs off, and is empty
	// for a merge request discussion.
	CommitSHA string
}

// NewMergeRequestDiscussion opens an unresolved discussion on the merge
// request. It goes with the merge request, so nothing is registered.
func NewMergeRequestDiscussion(e *harness.Env, project Project, mergeRequestIID int64) Discussion {
	e.T.Helper()

	discussion, err := retryTransient(e, "create merge request discussion", createRetries, func() (Discussion, error) {
		return createMergeRequestDiscussion(e.Ctx, e.Client(), project.ID, mergeRequestIID)
	})
	if err != nil {
		e.T.Fatalf("opening a discussion on merge request !%d of project %d: %v", mergeRequestIID, project.ID, err)
	}
	return discussion
}

// NewCommitDiscussion opens a discussion on the commit and returns it with the
// commit it hangs off. It goes with the project, so nothing is registered.
func NewCommitDiscussion(e *harness.Env, project Project, sha string) Discussion {
	e.T.Helper()

	discussion, err := retryTransient(e, "create commit discussion", createRetries, func() (Discussion, error) {
		return createCommitDiscussion(e.Ctx, e.Client(), project.ID, sha)
	})
	if err != nil {
		e.T.Fatalf("opening a discussion on commit %s of project %d: %v", ShortSHA(sha), project.ID, err)
	}
	return discussion
}

// createMergeRequestDiscussion opens the thread and reads back the note it
// opened with.
func createMergeRequestDiscussion(ctx context.Context, client *gitlabclient.Client, projectID, mergeRequestIID int64) (Discussion, error) {
	created, _, err := client.GL().Discussions.CreateMergeRequestDiscussion(projectID, mergeRequestIID,
		&gl.CreateMergeRequestDiscussionOptions{Body: new(discussionBody)}, gl.WithContext(ctx))
	if err != nil {
		return Discussion{}, err
	}
	return discussionOf(created, "")
}

// createCommitDiscussion is its commit half.
func createCommitDiscussion(ctx context.Context, client *gitlabclient.Client, projectID int64, sha string) (Discussion, error) {
	created, _, err := client.GL().Discussions.CreateCommitDiscussion(projectID, sha,
		&gl.CreateCommitDiscussionOptions{Body: new(discussionBody)}, gl.WithContext(ctx))
	if err != nil {
		return Discussion{}, err
	}
	return discussionOf(created, sha)
}

// discussionOf reads what a test needs out of what GitLab returned.
//
// A discussion with no notes is refused rather than handed back: the note is
// half of what the fixture promises, and a zero note ID would send a case to
// an endpoint that answers 404 about an object nobody can find.
func discussionOf(discussion *gl.Discussion, sha string) (Discussion, error) {
	if len(discussion.Notes) == 0 || discussion.Notes[0] == nil {
		return Discussion{}, fmt.Errorf("discussion %s was created carrying no note", discussion.ID)
	}
	return Discussion{ID: discussion.ID, NoteID: discussion.Notes[0].ID, CommitSHA: sha}, nil
}
