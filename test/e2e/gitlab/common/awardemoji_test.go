//go:build e2e

// awardemoji_test.go covers the award emoji through the server on the two
// objects the old suite reacted to, an issue and a project snippet, and on
// a snippet's note: award one, find it in the listing, read it by its ID,
// take it back. A user awards a given emoji once per object, so each
// surface reacts to an object of its own.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippetnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The emoji the tests award, named as GitLab names them, without colons.
const (
	emojiThumbsUp = "thumbsup"
	emojiHeart    = "heart"
)

// awardIDs lists the IDs of an award emoji listing.
func awardIDs(awards []awardemoji.Output) []int64 {
	ids := make([]int64, 0, len(awards))
	for _, award := range awards {
		ids = append(ids, award.ID)
	}
	return ids
}

// awardLifecycle runs the four emoji actions on one object: award, list,
// get and delete, holding each answer to the emoji named.
func awardLifecycle(e *harness.Env, s *harness.Session, params map[string]any, name string, create, list, get, del harness.ActionID) {
	e.T.Helper()

	created := harness.Do[awardemoji.Output](s, create, withParams(params, map[string]any{"name": name}))
	if created.ID == 0 || created.Name != name {
		e.T.Fatalf("%s answered %+v, want the award %q with an ID", create, created, name)
	}
	awardParams := withParams(params, map[string]any{"award_id": created.ID})

	listed := harness.Do[awardemoji.ListOutput](s, list, params)
	if !containsID(awardIDs(listed.AwardEmoji), created.ID) {
		e.T.Errorf("%s does not hold award %d: %v", list, created.ID, awardIDs(listed.AwardEmoji))
	}

	got := harness.Do[awardemoji.Output](s, get, awardParams)
	if got.ID != created.ID || got.Name != name {
		e.T.Errorf("%s answered %+v, want award %d named %q", get, got, created.ID, name)
	}

	harness.DoVoid(s, del, awardParams)
	remaining := harness.Do[awardemoji.ListOutput](s, list, params)
	if containsID(awardIDs(remaining.AwardEmoji), created.ID) {
		e.T.Errorf("award %d is still listed after its delete", created.ID)
	}
}

// TestAwardEmoji_Issue_AwardListGetDelete awards a thumbs-up to an issue
// of its own on every surface and walks the four actions on it.
//
// Replaces: TestIndividual_AwardEmoji, TestMeta_AwardEmoji
func TestAwardEmoji_Issue_AwardListGetDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issueemoji"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		issue := fixture.NewIssue(e, project, "award emoji fixture for the "+string(surface)+" surface")
		params := map[string]any{"project_id": project.IDParam(), "issue_iid": issue.IID}

		awardLifecycle(e, s, params, emojiThumbsUp, actionIssueEmojiCreate, actionIssueEmojiList, actionIssueEmojiGet, actionIssueEmojiDelete)
	})
}

// TestAwardEmoji_SnippetAndNote_AwardListGetDelete awards a thumbs-up to a
// project snippet of its own on every surface and a heart to a note on
// that snippet, walking the four actions on each. The note is written
// through the server, since a note is what the snippet note actions
// answer and the awards on it are the second half of this scenario.
//
// Replaces: TestMeta_SnippetEmoji
func TestAwardEmoji_SnippetAndNote_AwardListGetDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("snippetemoji"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		snippet := fixture.NewProjectSnippet(e, project)
		params := map[string]any{"project_id": project.IDParam(), "snippet_id": snippet.ID}

		awardLifecycle(e, s, params, emojiThumbsUp, actionSnippetEmojiCreate, actionSnippetEmojiList, actionSnippetEmojiGet, actionSnippetEmojiDelete)

		note := harness.Do[snippetnotes.Output](s, actionSnippetNoteCreate, withParams(params, map[string]any{"body": "the note the emoji lands on"}))
		if note.ID == 0 {
			e.T.Fatalf("snippet note_create answered %+v, want a note with an ID", note)
		}
		noteParams := withParams(params, map[string]any{"note_id": note.ID})
		awardLifecycle(e, s, noteParams, emojiHeart, actionSnippetNoteEmojiCreate, actionSnippetNoteEmojiList, actionSnippetNoteEmojiGet, actionSnippetNoteEmojiDelete)
	})
}
