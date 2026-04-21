// mr_draft_notes_test.go contains unit tests for the merge request draft note MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrdraftnotes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	msgNotFound        = "not found"
	errExpCancelledCtx = "expected error for canceled context"
	fmtUnexpErr        = "unexpected error: %v"
	pathDraftNotes     = "/api/v4/projects/42/merge_requests/1/draft_notes"
	pathDraftNoteByID  = "/api/v4/projects/42/merge_requests/1/draft_notes/10"
	errExpZeroNoteID   = "expected error for zero note_id"
)

// ---------------------------------------------------------------------------
// draftNoteList tests
// ---------------------------------------------------------------------------.

// TestDraftNoteList_Success verifies the behavior of draft note list success.
func TestDraftNoteList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNotes && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":10,"author_id":1,"merge_request_id":1,"note":"Draft comment","commit_id":"abc123","discussion_id":"","resolve_discussion":false},
				{"id":11,"author_id":2,"merge_request_id":1,"note":"Another draft","commit_id":"","discussion_id":"disc1","resolve_discussion":true}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DraftNotes) != 2 {
		t.Fatalf("expected 2 draft notes, got %d", len(out.DraftNotes))
	}
	if out.DraftNotes[0].ID != 10 || out.DraftNotes[0].Note != "Draft comment" {
		t.Errorf("first note mismatch: %+v", out.DraftNotes[0])
	}
	if out.DraftNotes[1].ResolveDiscussion != true {
		t.Error("expected second note to have ResolveDiscussion=true")
	}
}

// TestDraftNoteList_WithSorting verifies the behavior of draft note list with sorting.
func TestDraftNoteList_WithSorting(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNotes {
			if r.URL.Query().Get("order_by") != "id" || r.URL.Query().Get("sort") != "asc" {
				t.Errorf("expected order_by=id&sort=asc, got %s", r.URL.RawQuery)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		MRIID:     1,
		OrderBy:   "id",
		Sort:      "asc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDraftNoteList_MissingProjectID verifies the behavior of draft note list missing project i d.
func TestDraftNoteList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDraftNoteList_CancelledContext verifies the behavior of draft note list cancelled context.
func TestDraftNoteList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// draftNoteGet tests
// ---------------------------------------------------------------------------.

// TestDraftNoteGet_Success verifies the behavior of draft note get success.
func TestDraftNoteGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNoteByID && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"author_id":1,"merge_request_id":1,"note":"My draft","commit_id":"sha1","discussion_id":"","resolve_discussion":false}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 || out.Note != "My draft" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestDraftNote_GetZeroNoteID verifies the behavior of draft note get zero note i d.
func TestDraftNote_GetZeroNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    0,
	})
	if err == nil {
		t.Fatal(errExpZeroNoteID)
	}
}

// TestDraftNoteGet_CancelledContext verifies the behavior of draft note get cancelled context.
func TestDraftNoteGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// draftNoteCreate tests
// ---------------------------------------------------------------------------.

// TestDraftNoteCreate_Success verifies the behavior of draft note create success.
func TestDraftNoteCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNotes && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"author_id":1,"merge_request_id":1,"note":"New draft","commit_id":"","discussion_id":"","resolve_discussion":false}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Note:      "New draft",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 20 || out.Note != "New draft" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestDraftNoteCreate_WithOptions verifies the behavior of draft note create with options.
func TestDraftNoteCreate_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNotes && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":21,"author_id":1,"merge_request_id":1,"note":"Reply","commit_id":"abc","discussion_id":"disc1","resolve_discussion":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	resolve := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:             "42",
		MRIID:                 1,
		Note:                  "Reply",
		CommitID:              "abc",
		InReplyToDiscussionID: "disc1",
		ResolveDiscussion:     &resolve,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 21 {
		t.Errorf("expected ID=21, got %d", out.ID)
	}
}

// TestDraftNoteCreate_WithPosition verifies the behavior of draft note create with position.
func TestDraftNoteCreate_WithPosition(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNotes && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":22,"author_id":1,"merge_request_id":1,"note":"Inline comment",
				"position":{"base_sha":"aaa","start_sha":"bbb","head_sha":"ccc","new_path":"main.go","old_path":"main.go","new_line":42,"old_line":0}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Note:      "Inline comment",
		Position: &DiffPosition{
			BaseSHA:  "aaa",
			StartSHA: "bbb",
			HeadSHA:  "ccc",
			NewPath:  "main.go",
			OldPath:  "main.go",
			NewLine:  42,
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 22 {
		t.Errorf("expected ID=22, got %d", out.ID)
	}
	if out.Position == nil {
		t.Fatal("expected position in output")
	}
	if out.Position.NewPath != "main.go" {
		t.Errorf("expected new_path=main.go, got %q", out.Position.NewPath)
	}
	if out.Position.NewLine != 42 {
		t.Errorf("expected new_line=42, got %d", out.Position.NewLine)
	}
}

// TestDraftNoteCreate_MissingNote verifies the behavior of draft note create missing note.
func TestDraftNoteCreate_MissingNote(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		MRIID:     1,
		Note:      "",
	})
	if err == nil {
		t.Fatal("expected error for missing note")
	}
}

// TestDraftNoteCreate_CancelledContext verifies the behavior of draft note create cancelled context.
func TestDraftNoteCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: "42", MRIID: 1, Note: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// draftNoteUpdate tests
// ---------------------------------------------------------------------------.

// TestDraftNoteUpdate_Success verifies the behavior of draft note update success.
func TestDraftNoteUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNoteByID && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"author_id":1,"merge_request_id":1,"note":"Updated text","commit_id":"","discussion_id":"","resolve_discussion":false}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    10,
		Note:      "Updated text",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Note != "Updated text" {
		t.Errorf("expected note 'Updated text', got %q", out.Note)
	}
}

// TestDraftNote_UpdateZeroNoteID verifies the behavior of draft note update zero note i d.
func TestDraftNote_UpdateZeroNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    0,
		Note:      "text",
	})
	if err == nil {
		t.Fatal(errExpZeroNoteID)
	}
}

// TestDraftNoteUpdate_CancelledContext verifies the behavior of draft note update cancelled context.
func TestDraftNoteUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 10, Note: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// draftNoteDelete tests
// ---------------------------------------------------------------------------.

// TestDraftNoteDelete_Success verifies the behavior of draft note delete success.
func TestDraftNoteDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNoteByID && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDraftNote_DeleteZeroNoteID verifies the behavior of draft note delete zero note i d.
func TestDraftNote_DeleteZeroNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    0,
	})
	if err == nil {
		t.Fatal(errExpZeroNoteID)
	}
}

// TestDraftNoteDelete_CancelledContext verifies the behavior of draft note delete cancelled context.
func TestDraftNoteDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// draftNotePublish tests
// ---------------------------------------------------------------------------.

// TestDraftNotePublish_Success verifies the behavior of draft note publish success.
func TestDraftNotePublish_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNoteByID+"/publish" && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Publish(context.Background(), client, PublishInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDraftNote_PublishZeroNoteID verifies the behavior of draft note publish zero note i d.
func TestDraftNote_PublishZeroNoteID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Publish(context.Background(), client, PublishInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    0,
	})
	if err == nil {
		t.Fatal(errExpZeroNoteID)
	}
}

// TestDraftNotePublishServer_Error verifies the behavior of draft note publish server error.
func TestDraftNotePublishServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := Publish(context.Background(), client, PublishInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    10,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestDraftNotePublish_CancelledContext verifies the behavior of draft note publish cancelled context.
func TestDraftNotePublish_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Publish(ctx, client, PublishInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// draftNotePublishAll tests
// ---------------------------------------------------------------------------.

// TestDraftNotePublishAll_Success verifies the behavior of draft note publish all success.
func TestDraftNotePublishAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathDraftNotes+"/bulk_publish" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := PublishAll(context.Background(), client, PublishAllInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDraftNotePublishAll_MissingProjectID verifies the behavior of draft note publish all missing project i d.
func TestDraftNotePublishAll_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := PublishAll(context.Background(), client, PublishAllInput{
		ProjectID: "",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDraftNotePublishAllServer_Error verifies the behavior of draft note publish all server error.
func TestDraftNotePublishAllServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	err := PublishAll(context.Background(), client, PublishAllInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

// TestDraftNotePublishAll_CancelledContext verifies the behavior of draft note publish all cancelled context.
func TestDraftNotePublishAll_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := PublishAll(ctx, client, PublishAllInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// int64 validation tests
// ---------------------------------------------------------------------------.

// assertContains is an internal helper for the mrdraftnotes package.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies the behavior of m r i i d required validation.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when MRIID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt("42")
	const wantSubstr = "mr_iid"

	t.Run("List", func(t *testing.T) {
		_, err := List(ctx, client, ListInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Get", func(t *testing.T) {
		_, err := Get(ctx, client, GetInput{ProjectID: pid, MRIID: 0, NoteID: 1})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Create", func(t *testing.T) {
		_, err := Create(ctx, client, CreateInput{ProjectID: pid, MRIID: 0, Note: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Update", func(t *testing.T) {
		_, err := Update(ctx, client, UpdateInput{ProjectID: pid, MRIID: 0, NoteID: 1, Note: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Delete", func(t *testing.T) {
		err := Delete(ctx, client, DeleteInput{ProjectID: pid, MRIID: 0, NoteID: 1})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Publish", func(t *testing.T) {
		err := Publish(ctx, client, PublishInput{ProjectID: pid, MRIID: 0, NoteID: 1})
		assertContains(t, err, wantSubstr)
	})
	t.Run("PublishAll", func(t *testing.T) {
		err := PublishAll(ctx, client, PublishAllInput{ProjectID: pid, MRIID: 0})
		assertContains(t, err, wantSubstr)
	})
}

// TestNoteIDRequired_Validation verifies the behavior of note i d required validation.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when NoteID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt("42")
	const wantSubstr = "note_id"

	t.Run("Get", func(t *testing.T) {
		_, err := Get(ctx, client, GetInput{ProjectID: pid, MRIID: 1, NoteID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Update", func(t *testing.T) {
		_, err := Update(ctx, client, UpdateInput{ProjectID: pid, MRIID: 1, NoteID: 0, Note: "test"})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Delete", func(t *testing.T) {
		err := Delete(ctx, client, DeleteInput{ProjectID: pid, MRIID: 1, NoteID: 0})
		assertContains(t, err, wantSubstr)
	})
	t.Run("Publish", func(t *testing.T) {
		err := Publish(ctx, client, PublishInput{ProjectID: pid, MRIID: 1, NoteID: 0})
		assertContains(t, err, wantSubstr)
	})
}

// ---------------------------------------------------------------------------
// toDiffPositionOptions tests
// ---------------------------------------------------------------------------.

// TestToDiffPositionOptions verifies the behavior of to diff position options.
func TestToDiffPositionOptions(t *testing.T) {
	pos := &DiffPosition{
		BaseSHA:  "base",
		StartSHA: "start",
		HeadSHA:  "head",
		NewPath:  "new.go",
		OldPath:  "old.go",
		NewLine:  10,
		OldLine:  5,
	}

	opts := toDiffPositionOptions(pos)

	if *opts.BaseSHA != "base" {
		t.Errorf("expected BaseSHA=base, got %q", *opts.BaseSHA)
	}
	if *opts.StartSHA != "start" {
		t.Errorf("expected StartSHA=start, got %q", *opts.StartSHA)
	}
	if *opts.HeadSHA != "head" {
		t.Errorf("expected HeadSHA=head, got %q", *opts.HeadSHA)
	}
	if *opts.NewPath != "new.go" {
		t.Errorf("expected NewPath=new.go, got %q", *opts.NewPath)
	}
	if *opts.OldPath != "old.go" {
		t.Errorf("expected OldPath=old.go, got %q", *opts.OldPath)
	}
	if *opts.PositionType != "text" {
		t.Errorf("expected PositionType=text, got %q", *opts.PositionType)
	}
	if *opts.NewLine != 10 {
		t.Errorf("expected NewLine=10, got %d", *opts.NewLine)
	}
	if *opts.OldLine != 5 {
		t.Errorf("expected OldLine=5, got %d", *opts.OldLine)
	}
}

// TestToDiffPositionOptions_OmitsZeroLines verifies the behavior of to diff position options omits zero lines.
func TestToDiffPositionOptions_OmitsZeroLines(t *testing.T) {
	pos := &DiffPosition{
		BaseSHA:  "b",
		StartSHA: "s",
		HeadSHA:  "h",
		NewPath:  "file.go",
	}

	opts := toDiffPositionOptions(pos)

	if opts.OldPath != nil {
		t.Error("expected OldPath=nil for empty string")
	}
	if opts.NewLine != nil {
		t.Error("expected NewLine=nil for zero value")
	}
	if opts.OldLine != nil {
		t.Error("expected OldLine=nil for zero value")
	}
}

// ---------------------------------------------------------------------------
// ToOutput with Position tests
// ---------------------------------------------------------------------------.

// TestToOutput_WithPosition verifies the behavior of to output with position.
func TestToOutput_WithPosition(t *testing.T) {
	dn := &gl.DraftNote{
		ID:             30,
		AuthorID:       1,
		MergeRequestID: 99,
		Note:           "inline",
		Position: &gl.NotePosition{
			BaseSHA:  "aaa",
			StartSHA: "bbb",
			HeadSHA:  "ccc",
			NewPath:  "main.go",
			OldPath:  "main.go",
			NewLine:  42,
			OldLine:  0,
		},
	}

	out := ToOutput(dn)
	if out.Position == nil {
		t.Fatal("expected position in output")
	}
	if out.Position.BaseSHA != "aaa" {
		t.Errorf("expected BaseSHA=aaa, got %q", out.Position.BaseSHA)
	}
	if out.Position.NewLine != 42 {
		t.Errorf("expected NewLine=42, got %d", out.Position.NewLine)
	}
}

// TestToOutput_WithoutPosition verifies the behavior of to output without position.
func TestToOutput_WithoutPosition(t *testing.T) {
	dn := &gl.DraftNote{
		ID:             31,
		AuthorID:       1,
		MergeRequestID: 99,
		Note:           "general",
	}

	out := ToOutput(dn)
	if out.Position != nil {
		t.Error("expected nil position")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_Full verifies the behavior of format output markdown full.
func TestFormatOutputMarkdown_Full(t *testing.T) {
	out := Output{
		ID:                10,
		AuthorID:          1,
		MergeRequestID:    99,
		Note:              "Draft review comment",
		CommitID:          "abc123def456",
		DiscussionID:      "disc-001",
		ResolveDiscussion: true,
	}
	md := FormatOutputMarkdown(out)

	for _, want := range []string{
		"## Draft Note #10",
		"**Author ID**: 1",
		"**MR ID**: 99",
		"**Commit**: `abc123def456`",
		"**Discussion**: disc-001",
		"**Resolve Discussion**: true",
		"### Body",
		"Draft review comment",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_Minimal verifies the behavior of format output markdown minimal.
func TestFormatOutputMarkdown_Minimal(t *testing.T) {
	out := Output{
		ID:                5,
		AuthorID:          2,
		MergeRequestID:    50,
		Note:              "simple note",
		ResolveDiscussion: false,
	}
	md := FormatOutputMarkdown(out)

	if !strings.Contains(md, "## Draft Note #5") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "**Resolve Discussion**: false") {
		t.Errorf("should show resolve false:\n%s", md)
	}
	// CommitID and DiscussionID are empty, lines should not appear
	if strings.Contains(md, "**Commit**") {
		t.Errorf("should not contain Commit when empty:\n%s", md)
	}
	if strings.Contains(md, "**Discussion**") {
		t.Errorf("should not contain Discussion when empty:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithDraftNotes verifies the behavior of format list markdown with draft notes.
func TestFormatListMarkdown_WithDraftNotes(t *testing.T) {
	out := ListOutput{
		DraftNotes: []Output{
			{ID: 1, AuthorID: 10, CommitID: "abcdef1234567890", Note: "Short note"},
			{ID: 2, AuthorID: 20, CommitID: "abc", Note: "Another note that is quite long and exceeds sixty characters so it should be truncated properly here"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Draft Notes (2)",
		"| ID |",
		"| -- |",
		"| 1 |",
		"| 2 |",
		"abcdef12", // commit truncated to 8 chars
		"Short note",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	// Full 16-char commit should not appear (truncated to 8)
	if strings.Contains(md, "abcdef1234567890") {
		t.Errorf("commit should be truncated to 8 chars:\n%s", md)
	}
	// Long note should be truncated to 60 chars with "..."
	if strings.Contains(md, "properly here") {
		t.Errorf("long note should be truncated:\n%s", md)
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		DraftNotes: []Output{},
		Pagination: toolutil.PaginationOutput{},
	}
	md := FormatListMarkdown(out)

	if !strings.Contains(md, "No draft notes found.") {
		t.Errorf("expected 'No draft notes found.' in markdown:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// Handler error branches not covered by existing tests
// ---------------------------------------------------------------------------.

// TestGet_MissingProjectID verifies the behavior of get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_MissingProjectID verifies the behavior of create missing project i d.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{MRIID: 1, Note: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", MRIID: 1, Note: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_MissingProjectID verifies the behavior of update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{MRIID: 1, NoteID: 10, Note: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_EmptyNote verifies the behavior of update empty note.
func TestUpdate_EmptyNote(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"author_id":1,"merge_request_id":1,"note":"old text","commit_id":"","discussion_id":"","resolve_discussion":false}`)
			return
		}
		http.NotFound(w, r)
	}))
	// Note empty → the optional Note field is skipped, API still called
	out, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID=10, got %d", out.ID)
	}
}

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 10, Note: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingProjectID verifies the behavior of delete missing project i d.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPublish_MissingProjectID verifies the behavior of publish missing project i d.
func TestPublish_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Publish(context.Background(), client, PublishInput{MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPublishAll_CancelledContext verifies the behavior of publish all cancelled context.
func TestPublishAll_CancelledContext(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := PublishAll(ctx, client, PublishAllInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected context.Canceled error, got nil")
	}
}

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestValidatePosition_FileNotInDiff verifies that validatePosition returns an
// error when the target file path is not found in the merge request diff.
func TestValidatePosition_FileNotInDiff(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"other.go","new_path":"other.go","diff":"@@ -1,3 +1,4 @@\n line1\n+line2\n line3\n line4\n"}]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		NewPath: "missing_file.go",
		NewLine: 1,
	})
	if err == nil {
		t.Fatal("expected error for file not in diff, got nil")
	}
	if !strings.Contains(err.Error(), "not in the merge request diff") {
		t.Errorf("error = %q, want 'not in the merge request diff'", err.Error())
	}
}

// TestValidatePosition_FileFoundInDiff verifies that validatePosition succeeds
// when the target file and line are present in the merge request diff.
func TestValidatePosition_FileFoundInDiff(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"main.go","new_path":"main.go","diff":"@@ -1,3 +1,4 @@\n line1\n+added line\n line3\n line4\n"}]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		NewPath: "main.go",
		NewLine: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidatePosition_DiffFetchError verifies that validatePosition silently
// skips validation when the diff API returns an error (best-effort behavior).
func TestValidatePosition_DiffFetchError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		NewPath: "any.go",
		NewLine: 1,
	})
	if err != nil {
		t.Fatalf("expected nil error for best-effort skip, got: %v", err)
	}
}

// TestValidatePosition_FallbackToOldPath verifies that validatePosition uses
// OldPath when NewPath is empty to find the file in the diff.
func TestValidatePosition_FallbackToOldPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"old_name.go","new_path":"new_name.go","diff":"@@ -1,3 +1,4 @@\n line1\n+added\n line3\n line4\n"}]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	// NewPath is empty, should fallback to OldPath
	err := validatePosition(context.Background(), client, "42", 1, &DiffPosition{
		OldPath: "old_name.go",
		NewLine: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdate_WithPositionValidation verifies that Update calls validatePosition
// when a position is provided and the position is valid.
func TestUpdate_WithPositionValidation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"main.go","new_path":"main.go","diff":"@@ -1,3 +1,4 @@\n line1\n+added\n line3\n line4\n"}]`)
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/draft_notes/") {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"author_id":1,"merge_request_id":1,"note":"updated","commit_id":"","discussion_id":"","resolve_discussion":false}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		MRIID:     1,
		NoteID:    10,
		Note:      "updated",
		Position: &DiffPosition{
			BaseSHA:  "aaa",
			HeadSHA:  "bbb",
			StartSHA: "ccc",
			NewPath:  "main.go",
			OldPath:  "main.go",
			NewLine:  2,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID=10, got %d", out.ID)
	}
}

// TestRegisterTools_GetNotFound verifies that the get tool returns a
// non-error NotFound result when the API responds with 404.
func TestRegisterTools_GetNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_mr_draft_note_get",
		Arguments: map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 999},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for 404 response")
	}
}

// ---------------------------------------------------------------------------
// TestRegisterTools_CallAllThroughMCP — full MCP roundtrip for all 7 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newDraftNotesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_draft_note_list", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_draft_note_get", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 10}},
		{"gitlab_mr_draft_note_create", map[string]any{"project_id": "42", "mr_iid": 1, "note": "new draft"}},
		{"gitlab_mr_draft_note_update", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 10, "note": "updated"}},
		{"gitlab_mr_draft_note_delete", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 10}},
		{"gitlab_mr_draft_note_publish", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 10}},
		{"gitlab_mr_draft_note_publish_all", map[string]any{"project_id": "42", "mr_iid": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newDraftNotesMCPSession is an internal helper for the mrdraftnotes package.
func newDraftNotesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	draftNoteJSON := `{"id":10,"author_id":1,"merge_request_id":1,"note":"Draft comment","commit_id":"abc123","discussion_id":"disc1","resolve_discussion":false}`
	draftNoteListJSON := `[` + draftNoteJSON + `]`

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		// GET .../draft_notes → list
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/draft_notes"):
			testutil.RespondJSON(w, http.StatusOK, draftNoteListJSON)

		// GET .../draft_notes/{id} → get single
		case r.Method == http.MethodGet && strings.Contains(path, "/draft_notes/"):
			testutil.RespondJSON(w, http.StatusOK, draftNoteJSON)

		// POST .../draft_notes → create
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/draft_notes"):
			testutil.RespondJSON(w, http.StatusCreated, draftNoteJSON)

		// PUT .../draft_notes/{id}/publish → publish single
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/publish"):
			w.WriteHeader(http.StatusNoContent)

		// PUT .../draft_notes/{id} → update
		case r.Method == http.MethodPut && strings.Contains(path, "/draft_notes/"):
			testutil.RespondJSON(w, http.StatusOK, draftNoteJSON)

		// DELETE .../draft_notes/{id} → delete
		case r.Method == http.MethodDelete && strings.Contains(path, "/draft_notes/"):
			w.WriteHeader(http.StatusNoContent)

		// POST .../draft_notes/bulk_publish → publish all
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/bulk_publish"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
