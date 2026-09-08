package toolutil

import (
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// CapturedNote reads, from the captured answer of a call that returned one
// note, what GitLab sends that client-go's Note does not model.
func CapturedNote(capture *gitlabclient.ResponseCapture) (NoteExtra, error) {
	var extra NoteExtra
	if err := capture.Decode(&extra); err != nil {
		return NoteExtra{}, err
	}
	return extra, nil
}

// CapturedNotes reads the same from the captured answer of a call that
// returned a list of notes, one extra per note in the list's order. The
// count is held to what the SDK decoded: the two read the same bytes, so a
// difference is a fault in this reader and not in GitLab's answer.
func CapturedNotes(capture *gitlabclient.ResponseCapture, decoded int) ([]NoteExtra, error) {
	return capturedList[NoteExtra](capture, decoded, "notes")
}

// CapturedDiscussion reads, from the captured answer of a call that returned
// one discussion, what GitLab sends that client-go's Discussion and its notes
// do not model.
func CapturedDiscussion(capture *gitlabclient.ResponseCapture) (DiscussionExtra, error) {
	var extra DiscussionExtra
	if err := capture.Decode(&extra); err != nil {
		return DiscussionExtra{}, err
	}
	return extra, nil
}

// CapturedDiscussions reads the same from the captured answer of a call that
// returned a list of discussions, one extra per discussion in the list's
// order, the count held to what the SDK decoded as [CapturedNotes] holds it.
func CapturedDiscussions(capture *gitlabclient.ResponseCapture, decoded int) ([]DiscussionExtra, error) {
	return capturedList[DiscussionExtra](capture, decoded, "discussions")
}

// CapturedThread converts one discussion with what its captured answer
// carries beside the SDK's decode, or reports under op the answer the type
// cannot hold. It is the whole tail of a handler that returns one thread.
func CapturedThread(op string, d *gl.Discussion, capture *gitlabclient.ResponseCapture) (DiscussionThreadOutput, error) {
	extra, err := CapturedDiscussion(capture)
	if err != nil {
		return DiscussionThreadOutput{}, WrapErr(op, err)
	}
	return DiscussionThreadOutputFromGitLab(d, extra), nil
}

// CapturedThreads does the same for a list of discussions, pairing each with
// its own extra by position.
func CapturedThreads(op string, ds []*gl.Discussion, capture *gitlabclient.ResponseCapture) ([]DiscussionThreadOutput, error) {
	extras, err := CapturedDiscussions(capture, len(ds))
	if err != nil {
		return nil, WrapErr(op, err)
	}
	return DiscussionThreadOutputsFromGitLab(ds, extras), nil
}

// CapturedThreadNote converts one note of a thread the same way.
func CapturedThreadNote(op string, n *gl.Note, capture *gitlabclient.ResponseCapture) (DiscussionThreadNoteOutput, error) {
	extra, err := CapturedNote(capture)
	if err != nil {
		return DiscussionThreadNoteOutput{}, WrapErr(op, err)
	}
	return DiscussionThreadNoteOutputFromGitLab(n, extra), nil
}

// capturedList decodes a captured list answer and holds its length to the
// SDK's.
func capturedList[T any](capture *gitlabclient.ResponseCapture, decoded int, what string) ([]T, error) {
	var extras []T
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d %s and the SDK decoded %d", len(extras), what, decoded)
	}
	return extras, nil
}
