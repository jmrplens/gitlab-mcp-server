//go:build e2e

// discussion_test.go drives the discussion builders against the stub, and
// pins the refusal that matters: a discussion GitLab answered with no notes
// leaves a case addressing note zero, so the builder reports it rather than
// handing it back.

package fixture

import (
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestCreateMergeRequestDiscussion_Created_ReadsTheThreadAndItsFirstNote
// checks that both identifiers a case needs come out of one answer.
func TestCreateMergeRequestDiscussion_Created_ReadsTheThreadAndItsFirstNote(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/merge_requests/2/discussions", stubCreated(map[string]any{
		"id": "abc123", "individual_note": false,
		"notes": []any{map[string]any{"id": 77, "body": discussionBody}},
	}))

	got, err := createMergeRequestDiscussion(t.Context(), client, 1, 2)
	if err != nil {
		t.Fatalf("createMergeRequestDiscussion() error = %v, want nil", err)
	}
	want := Discussion{ID: "abc123", NoteID: 77}
	if got != want {
		t.Errorf("createMergeRequestDiscussion() = %+v, want %+v", got, want)
	}
}

// TestCreateCommitDiscussion_Created_CarriesTheCommitBeside checks the one
// way the commit half differs: a commit discussion is addressed by the commit
// as well, so the SHA travels with it.
func TestCreateCommitDiscussion_Created_CarriesTheCommitBeside(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/repository/commits/"+sha+"/discussions", stubCreated(map[string]any{
		"id": "def456", "notes": []any{map[string]any{"id": 88, "body": discussionBody}},
	}))

	got, err := createCommitDiscussion(t.Context(), client, 1, sha)
	if err != nil {
		t.Fatalf("createCommitDiscussion() error = %v, want nil", err)
	}
	want := Discussion{ID: "def456", NoteID: 88, CommitSHA: sha}
	if got != want {
		t.Errorf("createCommitDiscussion() = %+v, want %+v", got, want)
	}
}

// TestDiscussionOf_Notes_RefusesAThreadThatCarriesNone checks that an answer
// with no usable note is reported, naming the discussion, rather than handed
// back with a note ID of zero.
func TestDiscussionOf_Notes_RefusesAThreadThatCarriesNone(t *testing.T) {
	cases := []struct {
		name       string
		discussion *gl.Discussion
		wantErr    bool
	}{
		{name: "one note", discussion: &gl.Discussion{ID: "a", Notes: []*gl.Note{{ID: 1}}}},
		{name: "no notes", discussion: &gl.Discussion{ID: "b"}, wantErr: true},
		{name: "nil note", discussion: &gl.Discussion{ID: "c", Notes: []*gl.Note{nil}}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := discussionOf(tc.discussion, "")
			if (err != nil) != tc.wantErr {
				t.Fatalf("discussionOf() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got.NoteID == 0 {
				t.Error("discussionOf() returned note 0, which addresses nothing")
			}
		})
	}
}
