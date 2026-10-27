// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// CapturedNote reads, from the captured answer of a call that returned one
// note, what GitLab sends that client-go's Note does not model.
func CapturedNote(capture *gitlabclient.ResponseCapture) (NoteExtra, error) {
	return capturedOne[NoteExtra](capture)
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
	return capturedOne[DiscussionExtra](capture)
}

// CapturedDiscussions reads the same from the captured answer of a call that
// returned a list of discussions, one extra per discussion in the list's
// order, the count held to what the SDK decoded as [CapturedNotes] holds it.
func CapturedDiscussions(capture *gitlabclient.ResponseCapture, decoded int) ([]DiscussionExtra, error) {
	return capturedList[DiscussionExtra](capture, decoded, "discussions")
}

// CapturedThread converts one discussion with what its captured answer
// carries beside the SDK's decode, or reports under op the answer the type
// cannot hold. It is the whole tail of a handler that returns one thread, and
// answers in T, the thread type the handler's own package declares over
// [DiscussionThreadOutput] so the Markdown registry finds that package's
// formatter (see convertShape for what T must be).
func CapturedThread[T any](op string, d *gl.Discussion, capture *gitlabclient.ResponseCapture) (T, error) {
	extra, err := CapturedDiscussion(capture)
	if err != nil {
		var zero T
		return zero, WrapErr(op, err)
	}
	return convertShape[T](DiscussionThreadOutputFromGitLab(d, extra)), nil
}

// CapturedThreads does the same for a list of discussions, pairing each with
// its own extra by position.
func CapturedThreads[T any](op string, ds []*gl.Discussion, capture *gitlabclient.ResponseCapture) ([]T, error) {
	extras, err := CapturedDiscussions(capture, len(ds))
	if err != nil {
		return nil, WrapErr(op, err)
	}
	return DiscussionThreadOutputsFromGitLab[T](ds, extras), nil
}

// CapturedThreadNote converts one note of a thread the same way, answering in
// T, the note type the handler's package declares over
// [DiscussionThreadNoteOutput].
func CapturedThreadNote[T any](op string, n *gl.Note, capture *gitlabclient.ResponseCapture) (T, error) {
	extra, err := CapturedNote(capture)
	if err != nil {
		var zero T
		return zero, WrapErr(op, err)
	}
	return convertShape[T](DiscussionThreadNoteOutputFromGitLab(n, extra)), nil
}

// capturedOne decodes a captured answer to a request for one object.
//
// It is the single-object half of [capturedList], and exists for the same
// reason: every reader in sent_shapes.go was the same five lines around a
// different type, and one more copy per gap is how a file of small readers
// stops being readable.
func capturedOne[T any](capture *gitlabclient.ResponseCapture) (T, error) {
	var extra T
	if err := capture.Decode(&extra); err != nil {
		var zero T
		return zero, err
	}
	return extra, nil
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
