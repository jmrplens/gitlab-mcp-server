package toolutil

import (
	"errors"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestCapturedNote_ReadsWhatTheSDKDoesNotModel verifies the single-note
// reader: the fields GitLab sends beside the ones client-go decodes, the
// author's and resolver's own, and a capture nothing ran under reported as
// such.
func TestCapturedNote_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	capture := gitlabclient.CapturedBody([]byte(`{"id":1,"confidential":true,"imported":true,"imported_from":"github",` +
		`"commands_changes":{"label":"bug"},"suggestions":[{"id":5,"from_line":1,"to_line":2,"appliable":true,"applied":false,"from_content":"a","to_content":"b"}],` +
		`"author":{"id":1,"public_email":"alice@public.example","locked":true},"resolved_by":{"id":2,"locked":false}}`))

	got, err := CapturedNote(capture)
	if err != nil {
		t.Fatalf("CapturedNote() error = %v", err)
	}
	if !got.Confidential || !got.Imported || got.ImportedFrom != "github" || got.CommandsChanges["label"] != "bug" ||
		len(got.Suggestions) != 1 || got.Suggestions[0].ToContent != "b" || !got.Author.Locked ||
		got.Author.PublicEmail != "alice@public.example" || got.ResolvedBy.Locked {
		t.Errorf("CapturedNote() = %+v, want every captured field", got)
	}
	_, untouched := gitlabclient.WithResponseCapture(t.Context())
	_, err = CapturedNote(untouched)
	if !errors.Is(err, gitlabclient.ErrNoResponseCaptured) {
		t.Errorf("CapturedNote() on a capture nothing ran under = %v, want ErrNoResponseCaptured", err)
	}
}

// TestCapturedNotes_HoldsTheCountToTheSDKs verifies the list reader: one
// extra per note in order, a count other than the SDK's refused with both
// numbers, and a body that is not a list refused as a decode.
func TestCapturedNotes_HoldsTheCountToTheSDKs(t *testing.T) {
	capture := gitlabclient.CapturedBody([]byte(`[{"id":1,"imported":true},{"id":2,"imported_from":"gitlab"}]`))

	got, err := CapturedNotes(capture, 2)
	if err != nil {
		t.Fatalf("CapturedNotes() error = %v", err)
	}
	if len(got) != 2 || !got[0].Imported || got[1].ImportedFrom != "gitlab" {
		t.Errorf("CapturedNotes() = %+v, want two extras in order", got)
	}

	_, err = CapturedNotes(capture, 3)
	if err == nil || !strings.Contains(err.Error(), "holds 2 notes and the SDK decoded 3") {
		t.Errorf("CapturedNotes() with another count = %v, want the two numbers", err)
	}
	_, err = CapturedNotes(gitlabclient.CapturedBody([]byte(`{"id":1}`)), 1)
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("CapturedNotes() on an object = %v, want a decode error", err)
	}
}

// TestCapturedThreadCombinators_ConvertOrReportUnderTheOperation verifies the
// three combinators a discussion handler ends with: each converts the SDK's
// value with what the capture read beside it, and each reports a body the
// type cannot hold under the operation's own name.
func TestCapturedThreadCombinators_ConvertOrReportUnderTheOperation(t *testing.T) {
	discussion := &gl.Discussion{ID: "d1", Notes: []*gl.Note{{ID: 1}}}
	note := &gl.Note{ID: 1, Body: "hello"}
	good := func() *gitlabclient.ResponseCapture {
		return gitlabclient.CapturedBody([]byte(`{"id":"d1","resolvable":true,"notes":[{"id":1,"imported":true}]}`))
	}

	thread, err := CapturedThread("op", discussion, good())
	if err != nil || thread.ID != "d1" || !thread.Resolvable || len(thread.Notes) != 1 || !thread.Notes[0].Imported {
		t.Errorf("CapturedThread() = %+v, %v; want the thread with its captured half", thread, err)
	}

	threads, err := CapturedThreads("op", []*gl.Discussion{discussion},
		gitlabclient.CapturedBody([]byte(`[{"id":"d1","resolvable":true,"notes":[{"id":1}]}]`)))
	if err != nil || len(threads) != 1 || !threads[0].Resolvable {
		t.Errorf("CapturedThreads() = %+v, %v; want one thread with its captured half", threads, err)
	}

	threadNote, err := CapturedThreadNote("op", note, gitlabclient.CapturedBody([]byte(`{"id":1,"imported":true}`)))
	if err != nil || threadNote.Body != "hello" || !threadNote.Imported {
		t.Errorf("CapturedThreadNote() = %+v, %v; want both halves of the note", threadNote, err)
	}

	failures := []struct {
		name string
		call func() error
	}{
		{name: "thread", call: func() error {
			_, readErr := CapturedThread("op", discussion, gitlabclient.CapturedBody([]byte(`{"resolvable":7}`)))
			return readErr
		}},
		{name: "threads", call: func() error {
			_, readErr := CapturedThreads("op", []*gl.Discussion{discussion}, gitlabclient.CapturedBody([]byte(`{}`)))
			return readErr
		}},
		{name: "thread note", call: func() error {
			_, readErr := CapturedThreadNote("op", note, gitlabclient.CapturedBody([]byte(`{"imported":7}`)))
			return readErr
		}},
	}
	for _, failure := range failures {
		t.Run(failure.name+" reports under the operation", func(t *testing.T) {
			reported := failure.call()
			if reported == nil || !strings.Contains(reported.Error(), "op") {
				t.Errorf("error = %v, want one naming the operation", reported)
			}
		})
	}
}

// TestCapturedDiscussions_ReadTheThreadAndItsNotes verifies the discussion
// readers: the thread's own resolution state, the notes' extras nested under
// it, the single form, and the count held on the list form.
func TestCapturedDiscussions_ReadTheThreadAndItsNotes(t *testing.T) {
	body := []byte(`[{"id":"d1","resolvable":true,"resolved":true,"notes":[{"id":1,"confidential":true},{"id":2}]},{"id":"d2","notes":[]}]`)

	got, err := CapturedDiscussions(gitlabclient.CapturedBody(body), 2)
	if err != nil {
		t.Fatalf("CapturedDiscussions() error = %v", err)
	}
	if len(got) != 2 || !got[0].Resolvable || !got[0].Resolved || len(got[0].Notes) != 2 || !got[0].Notes[0].Confidential || got[1].Resolvable || len(got[1].Notes) != 0 {
		t.Errorf("CapturedDiscussions() = %+v, want the two threads with their notes", got)
	}
	_, err = CapturedDiscussions(gitlabclient.CapturedBody(body), 1)
	if err == nil || !strings.Contains(err.Error(), "holds 2 discussions and the SDK decoded 1") {
		t.Errorf("CapturedDiscussions() with another count = %v, want the two numbers", err)
	}

	one, err := CapturedDiscussion(gitlabclient.CapturedBody([]byte(`{"id":"d1","resolvable":true,"notes":[{"id":1,"imported":true}]}`)))
	if err != nil || !one.Resolvable || one.Resolved || len(one.Notes) != 1 || !one.Notes[0].Imported {
		t.Errorf("CapturedDiscussion() = %+v, %v; want the thread with its note", one, err)
	}
	_, err = CapturedDiscussion(gitlabclient.CapturedBody([]byte(`[]`)))
	if err == nil {
		t.Error("CapturedDiscussion() on a list = nil error, want a decode error")
	}
}
