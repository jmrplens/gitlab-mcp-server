//go:build e2e

// b7_mr_emoji_test.go covers the award emoji on a merge request and on one of
// its notes, which is the half of the award emoji family the issue and snippet
// scenarios beside it do not reach.
//
// A user awards a given emoji once per object, so each surface reacts to a
// merge request of its own inside one shared project.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestAwardEmoji_MergeRequestAndNote_AwardListGetDelete awards a thumbs-up to
// a merge request of its own on every surface and a heart to a note on that
// request, walking the four emoji actions over each object: the create answers
// the award named with an ID, the listing holds that ID, the singular read
// answers the same award, and the listing no longer holds it after the delete.
//
// The note is written through the server rather than through client-go,
// because the note emoji actions hang off a note and the request carries none
// until one is made.
func TestAwardEmoji_MergeRequestAndNote_AwardListGetDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mremoji"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "mremoji")
		params := f.params()

		awardLifecycle(e, s, params, emojiThumbsUp,
			actionMergeRequestEmojiCreate, actionMergeRequestEmojiList,
			actionMergeRequestEmojiGet, actionMergeRequestEmojiDelete)

		note := harness.Do[mrnotes.Output](s, actionMRReviewNoteCreate,
			withParams(params, map[string]any{"body": "the note the emoji lands on"}))
		if note.ID == 0 {
			e.T.Fatalf("merge request note_create answered %+v, want a note with an ID", note)
		}

		noteParams := withParams(params, map[string]any{"note_id": note.ID})
		awardLifecycle(e, s, noteParams, emojiHeart,
			actionMergeRequestNoteEmojiCreate, actionMergeRequestNoteEmojiList,
			actionMergeRequestNoteEmojiGet, actionMergeRequestNoteEmojiDelete)
	})
}
