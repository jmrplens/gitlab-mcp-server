// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// mr_draft_notes_test.go contains unit tests for the merge request draft note MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package mrdraftnotes

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// pathDraftNotes identifies the path draft notes constant used by this package.
	pathDraftNotes = "/api/v4/projects/42/merge_requests/1/draft_notes"
	// pathDraftNoteByID identifies the path draft note by ID constant used by this package.
	pathDraftNoteByID = "/api/v4/projects/42/merge_requests/1/draft_notes/10"
	// errExpZeroNoteID identifies the err exp zero note ID constant used by this package.
	errExpZeroNoteID = "expected error for zero note_id"
)

// ---------------------------------------------------------------------------
// draftNoteList tests
// ---------------------------------------------------------------------------.

// TestDraftNoteList_Success verifies DraftNoteList when success.
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

// TestDraftNoteList_WithSorting verifies DraftNoteList when with sorting.
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

// TestDraftNoteList_MissingProjectID verifies DraftNoteList when missing project ID.
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

// TestDraftNoteList_CancelledContext verifies DraftNoteList when cancelled context.
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

// TestDraftNoteGet_Success verifies DraftNoteGet when success.
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

// TestDraftNote_GetZeroNoteID verifies DraftNote when get zero note ID.
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

// TestDraftNoteGet_CancelledContext verifies DraftNoteGet when cancelled context.
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

// TestDraftNoteCreate_Success verifies DraftNoteCreate when success.
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

// TestDraftNoteCreate_WithOptions verifies DraftNoteCreate when with options.
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

// TestDraftNoteCreate_WithPosition verifies DraftNoteCreate when with position.
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

// TestDraftNoteCreate_MissingNote verifies DraftNoteCreate when missing note.
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

// TestDraftNoteCreate_CancelledContext verifies DraftNoteCreate when cancelled context.
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

// TestDraftNoteUpdate_Success verifies DraftNoteUpdate when success.
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

// TestDraftNote_UpdateZeroNoteID verifies DraftNote when update zero note ID.
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

// TestDraftNoteUpdate_CancelledContext verifies DraftNoteUpdate when cancelled context.
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

// TestDraftNoteDelete_Success verifies DraftNoteDelete when success.
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

// TestDraftNote_DeleteZeroNoteID verifies DraftNote when delete zero note ID.
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

// TestDraftNoteDelete_CancelledContext verifies DraftNoteDelete when cancelled context.
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

// TestDraftNotePublish_Success verifies DraftNotePublish when success.
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

// TestDraftNote_PublishZeroNoteID verifies DraftNote when publish zero note ID.
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

// TestDraftNotePublishServer_Error verifies DraftNotePublishServer when error.
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

// TestDraftNotePublish_CancelledContext verifies DraftNotePublish when cancelled context.
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

// TestDraftNotePublishAll_Success verifies DraftNotePublishAll when success.
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

// TestDraftNotePublishAll_Body_CarriesExactlyTheReviewOptionsGiven pins the
// body of the bulk-publish POST for each combination of the three optional
// parameters: each one the caller named reaches the body with its value, and
// none the caller left out is sent.
//
// The empty case is the one with a history. client-go v3.12.0 made
// PublishAllDraftNotes delegate to PublishAllDraftNotesWithOptions with a nil
// options pointer, which NewRequestToURL marshals as the four bytes "null"
// (recorded in docs/development/upstream-bugs.md). PublishAll now calls the
// options variant itself with a non-nil value, so a call naming nothing sends
// the empty object: a body that parses to a Hash with no keys, which is what
// Grape reads as no parameters. An internal of false is asserted as sent, so a
// handler that forwards the flag only when it is true fails here.
func TestDraftNotePublishAll_Body_CarriesExactlyTheReviewOptionsGiven(t *testing.T) {
	internalOff, internalOn := false, true
	tests := []struct {
		name  string
		input PublishAllInput
		want  map[string]any
	}{
		{name: "nothing set", input: PublishAllInput{}, want: map[string]any{}},
		{name: "note only", input: PublishAllInput{Note: "Looks good overall"}, want: map[string]any{"note": "Looks good overall"}},
		{name: "internal false", input: PublishAllInput{Note: "Visible", Internal: &internalOff}, want: map[string]any{"note": "Visible", "internal": false}},
		{name: "reviewer state only", input: PublishAllInput{ReviewerState: "reviewed"}, want: map[string]any{"reviewer_state": "reviewed"}},
		{
			name:  "every option",
			input: PublishAllInput{Note: "Two blockers inline", Internal: &internalOn, ReviewerState: "requested_changes"},
			want:  map[string]any{"note": "Two blockers inline", "internal": true, "reviewer_state": "requested_changes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != pathDraftNotes+"/bulk_publish" || r.Method != http.MethodPost {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
					return
				}
				read, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
					http.Error(w, "read body", http.StatusInternalServerError)
					return
				}
				body = read
				w.WriteHeader(http.StatusNoContent)
			}))

			input := tt.input
			input.ProjectID, input.MRIID = "42", 1
			if err := PublishAll(context.Background(), client, input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			var sent map[string]any
			if err := json.Unmarshal(body, &sent); err != nil {
				t.Fatalf("bulk_publish body %q is not a JSON object: %v", body, err)
			}
			if sent == nil {
				t.Fatalf("bulk_publish body = %q, want a JSON object rather than null", body)
			}
			if !reflect.DeepEqual(sent, tt.want) {
				t.Errorf("bulk_publish body = %#v, want %#v", sent, tt.want)
			}
		})
	}
}

// TestDraftNotePublishAll_MissingProjectID verifies DraftNotePublishAll when missing project ID.
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

// TestDraftNotePublishAllServer_Error verifies DraftNotePublishAllServer when error.
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

// TestDraftNotePublishAll_CancelledContext verifies DraftNotePublishAll when cancelled context.
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

// assertContains checks contains invariants for tests.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q should contain %q", err.Error(), substr)
	}
}

// TestMRIIDRequired_Validation verifies MRIIDRequired when validation.
func TestMRIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when MRIID is 0")
		http.NotFound(w, nil)
	}))

	ctx := context.Background()
	pid := toolutil.StringOrInt("42")
	const wantSubstr = "merge_request_iid"

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

// TestNoteIDRequired_Validation verifies NoteIDRequired when validation.
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

// TestToDiffPositionOptions verifies ToDiffPositionOptions.
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

// TestToDiffPositionOptions_OmitsZeroLines verifies ToDiffPositionOptions omits zero lines.
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

// TestToOutput_WithPosition verifies ToOutput when with position.
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

// TestToOutput_WithoutPosition verifies ToOutput when without position.
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

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// draftCardHints is the guidance section a draft note card closes with, and
// draftListHints the one a listing closes with.
const (
	draftCardHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.draft_note_publish' to publish this draft note\n" +
		"- Use action 'mr_review.draft_note_update' to change it before publishing\n" +
		"- Use action 'mr_review.draft_note_delete' to discard it\n"
	draftListHints = "\n---\n💡 **Next steps:**\n" +
		"- Use action 'mr_review.draft_note_get' to read one draft note in full\n" +
		"- Use action 'mr_review.draft_note_publish_all' to publish every draft at once\n"
)

// TestFormatOutputMarkdown_Full verifies the whole rendering of a draft note,
// including the row this card used to label "MR ID": GitLab answers with the
// merge request's global database id there, not the project-scoped IID every
// draft note action takes, and a reader who passed it back got a 404.
func TestFormatOutputMarkdown_Full(t *testing.T) {
	out := Output{
		ID:                10,
		AuthorID:          1,
		MergeRequestID:    99,
		Note:              "Draft review comment",
		CommitID:          "abc123def456",
		LineCode:          "a1b2c3_10_11",
		DiscussionID:      "disc-001",
		ResolveDiscussion: true,
		Position:          &PositionOutput{NewPath: "main.go", NewLine: 42, PositionType: "text"},
	}
	want := "## Draft Note #10\n\n" +
		"- **ID**: 10\n" +
		"- **Author ID**: 1\n" +
		"- **MR global ID (not the IID)**: 99\n" +
		"- **Commit**: `abc123def456`\n" +
		"- **Line Code**: `a1b2c3_10_11`\n" +
		"- **Discussion**: `disc-001`\n" +
		"- **Resolves the discussion**: ✅\n" +
		"- **Position**: `main.go` line 42\n" +
		"- **Note**: Draft review comment\n" + draftCardHints
	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatOutputMarkdown_Minimal verifies that a draft note GitLab sent
// nothing optional for shows no label with nothing after it.
func TestFormatOutputMarkdown_Minimal(t *testing.T) {
	out := Output{
		ID:             5,
		AuthorID:       2,
		MergeRequestID: 50,
		Note:           "simple note",
	}
	want := "## Draft Note #5\n\n" +
		"- **ID**: 5\n" +
		"- **Author ID**: 2\n" +
		"- **MR global ID (not the IID)**: 50\n" +
		"- **Resolves the discussion**: ❌\n" +
		"- **Note**: simple note\n" + draftCardHints
	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatOutputMarkdown_PositionWithoutALine verifies that a note anchored
// to a whole file names no line: GitLab sends none for one, and "line 0" read
// as the top of the file.
func TestFormatOutputMarkdown_PositionWithoutALine(t *testing.T) {
	out := Output{ID: 6, AuthorID: 2, MergeRequestID: 50, Note: "on the file", Position: &PositionOutput{OldPath: "docs/img.png", PositionType: "image"}}
	want := "## Draft Note #6\n\n" +
		"- **ID**: 6\n" +
		"- **Author ID**: 2\n" +
		"- **MR global ID (not the IID)**: 50\n" +
		"- **Resolves the discussion**: ❌\n" +
		"- **Position**: `docs/img.png` (image)\n" +
		"- **Note**: on the file\n" + draftCardHints
	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithDraftNotes verifies FormatListMarkdown when with draft notes.
func TestFormatListMarkdown_WithDraftNotes(t *testing.T) {
	out := ListOutput{
		DraftNotes: []Output{
			{ID: 1, AuthorID: 10, CommitID: "abcdef1234567890", Note: "Short note"},
			{ID: 2, AuthorID: 20, CommitID: "abc", Note: "Another note that is quite long and exceeds sixty characters so it should be truncated properly here"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	want := "## Draft Notes (2)\n\n" +
		"| ID | Author ID | Commit | Note |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | 10 | `abcdef12` | Short note |\n" +
		"| 2 | 20 | `abc` | Another note that is quite long and exceeds sixty characters… |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" + draftListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatListMarkdown_TruncatesOnRuneBoundaries verifies that a note whose
// sixtieth rune is multi-byte is cut between characters rather than through
// one: the byte slice this replaced put half a character on the page.
func TestFormatListMarkdown_TruncatesOnRuneBoundaries(t *testing.T) {
	out := ListOutput{DraftNotes: []Output{{ID: 1, AuthorID: 10, Note: strings.Repeat("é", 70)}}}
	want := "## Draft Notes (1)\n\n" +
		"| ID | Author ID | Commit | Note |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | 10 |  | " + strings.Repeat("é", 60) + "… |\n" + draftListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatListMarkdown_Empty verifies that a merge request with no draft note
// renders the one-sentence empty message and no table header.
func TestFormatListMarkdown_Empty(t *testing.T) {
	want := "No draft notes found.\n"
	if got := FormatListMarkdown(ListOutput{DraftNotes: []Output{}}); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Handler error branches not covered by existing tests
// ---------------------------------------------------------------------------.

// TestGet_MissingProjectID verifies Get when missing project ID.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_MissingProjectID verifies Create when missing project ID.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{MRIID: 1, Note: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", MRIID: 1, Note: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_MissingProjectID verifies Update when missing project ID.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{MRIID: 1, NoteID: 10, Note: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_EmptyNote verifies Update when empty note.
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

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 10, Note: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingProjectID verifies Delete when missing project ID.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPublish_MissingProjectID verifies Publish when missing project ID.
func TestPublish_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Publish(context.Background(), client, PublishInput{MRIID: 1, NoteID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPublishAll_CancelledContext verifies PublishAll when cancelled context.
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

// TestList_APIError verifies List when API error.
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
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"aaa","start_sha":"ccc","head_sha":"bbb"}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"other.go","new_path":"other.go","diff":"@@ -1,3 +1,4 @@\n line1\n+line2\n line3\n line4\n"}]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	err := validatePosition(t.Context(), client, "42", 1, &DiffPosition{
		BaseSHA: "aaa", StartSHA: "ccc", HeadSHA: "bbb",
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
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"aaa","start_sha":"ccc","head_sha":"bbb"}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"main.go","new_path":"main.go","diff":"@@ -1,3 +1,4 @@\n line1\n+added line\n line3\n line4\n"}]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	err := validatePosition(t.Context(), client, "42", 1, &DiffPosition{
		BaseSHA: "aaa", StartSHA: "ccc", HeadSHA: "bbb",
		NewPath: "main.go",
		NewLine: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidatePosition_FileWithEmptyDiff_SkipsTheLineCheck verifies that a
// file GitLab lists with an empty diff, as it does for a binary file or one
// whose diff is too large to show, passes whatever line the position names:
// there are no lines to hold it to, so GitLab judges the write.
func TestValidatePosition_FileWithEmptyDiff_SkipsTheLineCheck(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"aaa","start_sha":"ccc","head_sha":"bbb"}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"logo.png","new_path":"logo.png","diff":""}]`)
	}))
	err := validatePosition(t.Context(), client, "42", 1, &DiffPosition{
		BaseSHA: "aaa", StartSHA: "ccc", HeadSHA: "bbb",
		NewPath: "logo.png",
		NewLine: 99,
	})
	if err != nil {
		t.Errorf("validatePosition() error = %v, want nil for a file with no diff lines", err)
	}
}

// TestValidatePosition_DiffFetchError verifies that validatePosition silently
// skips validation when the diff API returns an error (best-effort behavior).
func TestValidatePosition_DiffFetchError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"aaa","start_sha":"ccc","head_sha":"bbb"}}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))

	err := validatePosition(t.Context(), client, "42", 1, &DiffPosition{
		BaseSHA: "aaa", StartSHA: "ccc", HeadSHA: "bbb",
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
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"aaa","start_sha":"ccc","head_sha":"bbb"}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/diffs") {
			testutil.RespondJSON(w, http.StatusOK, `[{"old_path":"old_name.go","new_path":"new_name.go","diff":"@@ -1,3 +1,4 @@\n line1\n+added\n line3\n line4\n"}]`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	// NewPath is empty, should fallback to OldPath
	err := validatePosition(t.Context(), client, "42", 1, &DiffPosition{
		BaseSHA: "aaa", StartSHA: "ccc", HeadSHA: "bbb",
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
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"aaa","start_sha":"ccc","head_sha":"bbb"}}`)
			return
		}
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

	out, err := Update(t.Context(), client, UpdateInput{
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Full position object (1:1 audit) tests
// ---------------------------------------------------------------------------.

// TestToDiffPositionOptions_FullImageAndRange verifies that the expanded
// DiffPosition mirrors every PositionOptions field, including image coordinates
// and a multi-line line_range.
func TestToDiffPositionOptions_FullImageAndRange(t *testing.T) {
	pos := &DiffPosition{
		BaseSHA:      "base",
		StartSHA:     "start",
		HeadSHA:      "head",
		NewPath:      "img.png",
		OldPath:      "img.png",
		PositionType: "image",
		Width:        new(int64(800)),
		Height:       new(int64(600)),
		X:            new(12.5),
		Y:            new(34.5),
		LineRange: &DiffLineRange{
			Start: &DiffLinePosition{LineCode: "lc1", Type: "new", OldLine: 0, NewLine: 10},
			End:   &DiffLinePosition{LineCode: "lc2", Type: "new", OldLine: 0, NewLine: 12},
		},
	}

	opts := toDiffPositionOptions(pos)

	if *opts.PositionType != "image" {
		t.Errorf("expected PositionType=image, got %q", *opts.PositionType)
	}
	if opts.Width == nil || *opts.Width != 800 {
		t.Errorf("expected Width=800, got %v", opts.Width)
	}
	if opts.Height == nil || *opts.Height != 600 {
		t.Errorf("expected Height=600, got %v", opts.Height)
	}
	if opts.X == nil || *opts.X != 12.5 {
		t.Errorf("expected X=12.5, got %v", opts.X)
	}
	if opts.Y == nil || *opts.Y != 34.5 {
		t.Errorf("expected Y=34.5, got %v", opts.Y)
	}
	if opts.LineRange == nil || opts.LineRange.Start == nil || opts.LineRange.End == nil {
		t.Fatal("expected line_range start and end")
	}
	if *opts.LineRange.Start.LineCode != "lc1" || *opts.LineRange.Start.NewLine != 10 {
		t.Errorf("unexpected line_range start: %+v", opts.LineRange.Start)
	}
	if *opts.LineRange.End.Type != "new" || *opts.LineRange.End.NewLine != 12 {
		t.Errorf("unexpected line_range end: %+v", opts.LineRange.End)
	}
}

// TestToLinePositionOptions_Nil verifies the line-position converter handles nil.
func TestToLinePositionOptions_Nil(t *testing.T) {
	if toLinePositionOptions(nil) != nil {
		t.Error("expected nil for nil input")
	}
	// A line position with only OldLine set should omit empty fields.
	opt := toLinePositionOptions(&DiffLinePosition{OldLine: 7})
	if opt.LineCode != nil || opt.Type != nil || opt.NewLine != nil {
		t.Error("expected empty fields to be omitted")
	}
	if opt.OldLine == nil || *opt.OldLine != 7 {
		t.Errorf("expected OldLine=7, got %v", opt.OldLine)
	}
}

// TestToOutput_LineCodeAndLineRange verifies ToOutput surfaces line_code and the
// full NotePosition line_range.
func TestToOutput_LineCodeAndLineRange(t *testing.T) {
	dn := &gl.DraftNote{
		ID:             40,
		AuthorID:       1,
		MergeRequestID: 99,
		Note:           "inline",
		LineCode:       "abc_1_2",
		Position: &gl.NotePosition{
			BaseSHA:      "aaa",
			StartSHA:     "bbb",
			HeadSHA:      "ccc",
			PositionType: "text",
			NewPath:      "main.go",
			NewLine:      42,
			LineRange: &gl.LineRange{
				StartRange: &gl.LinePosition{LineCode: "s", Type: "new", NewLine: 40},
				EndRange:   &gl.LinePosition{LineCode: "e", Type: "new", NewLine: 42},
			},
		},
	}

	out := ToOutput(dn)
	if out.LineCode != "abc_1_2" {
		t.Errorf("expected LineCode=abc_1_2, got %q", out.LineCode)
	}
	if out.Position == nil || out.Position.PositionType != "text" {
		t.Fatalf("expected position with type text, got %+v", out.Position)
	}
	if out.Position.LineRange == nil || out.Position.LineRange.Start == nil || out.Position.LineRange.End == nil {
		t.Fatal("expected line_range start and end in output")
	}
	if out.Position.LineRange.Start.NewLine != 40 || out.Position.LineRange.End.NewLine != 42 {
		t.Errorf("unexpected output line_range: %+v", out.Position.LineRange)
	}
}

// TestFormatOutputMarkdown_PositionAndLineCode verifies the single-note
// Markdown renders the line code and inline diff position.
func TestFormatOutputMarkdown_PositionAndLineCode(t *testing.T) {
	out := Output{
		ID:             11,
		AuthorID:       1,
		MergeRequestID: 99,
		Note:           "inline note",
		LineCode:       "code_3_4",
		Position: &PositionOutput{
			BaseSHA: "a", StartSHA: "b", HeadSHA: "c",
			NewPath: "main.go", NewLine: 7,
		},
	}
	md := FormatOutputMarkdown(out)
	for _, want := range []string{
		"**Line Code**: `code_3_4`",
		"**Position**: `main.go` line 7",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatOutputMarkdown_PositionOldPathFallback verifies the position
// renderer falls back to old_path/old_line when new values are absent.
func TestFormatOutputMarkdown_PositionOldPathFallback(t *testing.T) {
	out := Output{
		ID: 12, AuthorID: 1, MergeRequestID: 99, Note: "removed line",
		Position: &PositionOutput{
			BaseSHA: "a", StartSHA: "b", HeadSHA: "c",
			OldPath: "old.go", OldLine: 3,
		},
	}
	md := FormatOutputMarkdown(out)
	if !strings.Contains(md, "**Position**: `old.go` line 3") {
		t.Errorf("expected old_path fallback in markdown:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// What the handlers put in the request body
// ---------------------------------------------------------------------------.

// TestDraftNoteCreate_OptionalFieldsReachGitLab verifies that each optional
// field of a create request is sent when the caller gave one and left out when
// they did not. A test that reads only the response cannot see either half:
// GitLab answers the same whatever the body held, so an inverted guard would
// drop the caller's commit_id and send an empty one for the caller who set
// nothing, and the draft note would be anchored to the wrong commit.
func TestDraftNoteCreate_OptionalFieldsReachGitLab(t *testing.T) {
	resolve := true
	for _, tc := range []struct {
		name    string
		input   CreateInput
		present []string
		absent  []string
	}{
		{
			name: "every optional field given",
			input: CreateInput{
				ProjectID: "42", MRIID: 1, Note: "Reply",
				CommitID:              "abc123",
				InReplyToDiscussionID: "disc1",
				ResolveDiscussion:     &resolve,
			},
			present: []string{`"commit_id":"abc123"`, `"in_reply_to_discussion_id":"disc1"`, `"resolve_discussion":true`},
		},
		{
			name:   "no optional field given",
			input:  CreateInput{ProjectID: "42", MRIID: 1, Note: "Reply"},
			absent: []string{"commit_id", "in_reply_to_discussion_id", "resolve_discussion"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":21,"author_id":1,"merge_request_id":1,"note":"Reply"}`)
			}))

			if _, err := Create(context.Background(), client, tc.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			for _, want := range tc.present {
				if !strings.Contains(body, want) {
					t.Errorf("request body %q is missing %s", body, want)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(body, unwanted) {
					t.Errorf("request body %q carries %s the caller never set", body, unwanted)
				}
			}
		})
	}
}

// TestDraftNoteUpdate_NoteBodyReachesGitLab verifies that the revised body a
// caller passes is the one sent, and that an omitted body is left out rather
// than sent as an empty string. Both halves matter for an edit: GitLab's
// answer is identical either way, so an inverted guard would blank the note it
// was asked to change while silently discarding the replacement text.
func TestDraftNoteUpdate_NoteBodyReachesGitLab(t *testing.T) {
	for _, tc := range []struct {
		name    string
		note    string
		present string
		absent  string
	}{
		{name: "a new body is sent", note: "Updated text", present: `"note":"Updated text"`},
		{name: "an omitted body is not sent", note: "", absent: "note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				testutil.RespondJSON(w, http.StatusOK, `{"id":10,"author_id":1,"merge_request_id":1,"note":"Updated text"}`)
			}))

			_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 10, Note: tc.note})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if tc.present != "" && !strings.Contains(body, tc.present) {
				t.Errorf("request body %q is missing %s", body, tc.present)
			}
			if tc.absent != "" && strings.Contains(body, tc.absent) {
				t.Errorf("request body %q carries %s the caller never set", body, tc.absent)
			}
		})
	}
}

// TestFormatListMarkdown_NoteOfExactlyTheCellWidth verifies that a note as long
// as the cell is shown whole. The ellipsis is a promise that something was cut,
// and a note that fits entirely had nothing cut from it.
func TestFormatListMarkdown_NoteOfExactlyTheCellWidth(t *testing.T) {
	note := strings.Repeat("a", noteCellRunes)
	out := ListOutput{DraftNotes: []Output{{ID: 1, AuthorID: 10, Note: note}}}
	want := "## Draft Notes (1)\n\n" +
		"| ID | Author ID | Commit | Note |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | 10 |  | " + note + " |\n" + draftListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("rendered =\n%q\nwant\n%q", got, want)
	}
}

// draftNoteWrittenBody is the draft note GitLab answers both writes with, for
// the tests whose assertions are all about the request.
const draftNoteWrittenBody = `{"id":10,"author_id":1,"merge_request_id":1,"note":"see here"}`

// draftNoteWrite is one of the two draft note writes that carry a position,
// as a caller reaches it: the individual tool every surface projects it
// from, the request GitLab receives and the arguments beside the position.
type draftNoteWrite struct {
	tool   string
	method string
	path   string
	args   map[string]any
}

// draftNoteWrites lists the writes a draft note's position reaches GitLab
// through, mr_review.draft_note_create and mr_review.draft_note_update.
func draftNoteWrites() []draftNoteWrite {
	return []draftNoteWrite{
		{tool: "gitlab_mr_draft_note_create", method: http.MethodPost, path: pathDraftNotes, args: map[string]any{"note": "see here"}},
		{tool: "gitlab_mr_draft_note_update", method: http.MethodPut, path: pathDraftNoteByID, args: map[string]any{"note_id": 10, "note": "see here"}},
	}
}

// call runs the write through its action route, with the arguments a caller
// spells, and position beside its own.
func (w draftNoteWrite) call(t *testing.T, client *gitlabclient.Client, position map[string]any) error {
	t.Helper()
	spec, ok := mrDraftNoteSpecsByTool(t, ActionSpecs(client))[w.tool]
	if !ok {
		t.Fatalf("no ActionSpec projects %s", w.tool)
	}
	args := map[string]any{"project_id": "42", "merge_request_iid": 1, "position": position}
	maps.Copy(args, w.args)
	_, err := spec.Route.Handler(t.Context(), args)
	return err
}

// sentPosition decodes the position of a mocked write's body. It reports
// rather than aborting, because it runs on the httptest server's goroutine,
// and answers nil for a body that carries none.
func sentPosition(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decoding the request body: %v", err)
	}
	position, _ := body["position"].(map[string]any)
	return position
}

// assertSentField asserts one key of the position a write carried: a nil want
// means the key must be absent.
func assertSentField(t *testing.T, position map[string]any, key string, want any) {
	t.Helper()
	got, present := position[key]
	switch {
	case want == nil && present:
		t.Errorf("%q = %v, want it absent from %v", key, got, position)
	case want == nil:
	case !present:
		t.Errorf("%q missing from %v, want %v", key, position, want)
	case got != want:
		t.Errorf("%q = %v, want %v", key, got, want)
	}
}

// TestWrites_ImageCoordinateOfZero_IsSent asserts that a draft note's image
// position on the image's edge reaches GitLab with its coordinate, on the
// create and on the update alike. GitLab takes 0 as a value of width, height,
// x and y, and keeps a draft note's position only when all four are there, so
// the x and the y of 0 a plain number dropped published the note as a
// general comment with its position gone. The merge request answers 404 here,
// so the position pre-check steps aside and the position reaching GitLab is
// the one the caller spelled; on the parent neither request carried x or y.
func TestWrites_ImageCoordinateOfZero_IsSent(t *testing.T) {
	for _, write := range draftNoteWrites() {
		t.Run(write.tool, func(t *testing.T) {
			var mu sync.Mutex
			var sent map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != write.method || r.URL.Path != write.path {
					http.NotFound(w, r)
					return
				}
				position := sentPosition(t, r)
				mu.Lock()
				sent = position
				mu.Unlock()
				testutil.RespondJSON(w, http.StatusOK, draftNoteWrittenBody)
			}))

			err := write.call(t, client, map[string]any{
				"base_sha": "aaa", "start_sha": "ccc", "head_sha": "bbb", "new_path": "assets/logo.png",
				"position_type": "image", "width": 64, "height": 32, "x": 0, "y": 0,
			})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if sent == nil {
				t.Fatal("no position reached GitLab")
			}
			assertSentField(t, sent, "width", float64(64))
			assertSentField(t, sent, "height", float64(32))
			assertSentField(t, sent, "x", float64(0))
			assertSentField(t, sent, "y", float64(0))
		})
	}
}

// positionTypePath is the file of the merge request diff the position_type
// tests name, and positionTypeDiff is that diff: positionTypePath with a
// patch of lines, so a text position is judged against them and a pre-check
// that judged every position by line would refuse any position naming none,
// and positionTypeBinaryPath with no patch, as GitLab lists a binary file.
const (
	positionTypePath       = "assets/logo.svg"
	positionTypeBinaryPath = "assets/logo.png"
	positionTypeDiff       = `[{"old_path":"assets/logo.svg","new_path":"assets/logo.svg","diff":"@@ -1,2 +1,2 @@\n-<svg width=\"10\"/>\n+<svg width=\"20\"/>\n <!-- end -->\n"},` +
		`{"old_path":"assets/logo.png","new_path":"assets/logo.png","diff":""}]`
)

// positionTypeGitLab is the GitLab the position_type tests write to. It
// answers the merge request with the position's diff refs and its diff with
// positionTypeDiff, counts those reads, and keeps the position the write
// carried. A write whose position_type is none of the three GitLab declares
// is answered with the 400 Grape refuses it with. It records rather than
// asserts, because it runs on the httptest server's goroutine.
type positionTypeGitLab struct {
	mu    sync.Mutex
	reads int
	sent  map[string]any
}

// handler serves the merge request, its diff and the write.
func (g *positionTypeGitLab) handler(t *testing.T, write draftNoteWrite) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1":
			g.read()
			testutil.RespondJSON(w, http.StatusOK, `{"diff_refs":{"base_sha":"base","start_sha":"start","head_sha":"head"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/1/diffs":
			g.read()
			testutil.RespondJSON(w, http.StatusOK, positionTypeDiff)
		case r.Method == write.method && r.URL.Path == write.path:
			position := sentPosition(t, r)
			g.mu.Lock()
			g.sent = position
			g.mu.Unlock()
			switch position["position_type"] {
			case "text", "image", "file":
				testutil.RespondJSON(w, http.StatusOK, draftNoteWrittenBody)
			default:
				testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"position[position_type] does not have a valid value"}`)
			}
		default:
			http.NotFound(w, r)
		}
	})
}

// read counts one read of the merge request or of its diff.
func (g *positionTypeGitLab) read() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reads++
}

// positionTypeCase is one position of the position_type tests, as a caller
// spells it, and what the pre-check does with it: refuse it in words naming
// wantErr, or send it carrying the fields of wantSent (a nil value meaning
// absent), having read the merge request and its diff only when wantRead. A
// case with both is a write GitLab receives and refuses.
type positionTypeCase struct {
	name     string
	position map[string]any
	wantErr  string
	wantSent map[string]any
	wantRead bool
}

// positionTypeCases lists a position of each position_type GitLab declares,
// one it does not, and for each the fields GitLab requires of it present and
// missing. Every position names the SHAs the merge request answers with and
// positionTypePath, which the diff carries.
func positionTypeCases() []positionTypeCase {
	at := func(fields map[string]any) map[string]any {
		position := map[string]any{"base_sha": "base", "start_sha": "start", "head_sha": "head", "new_path": positionTypePath}
		maps.Copy(position, fields)
		return position
	}
	return []positionTypeCase{
		{
			name: "text on an added line", position: at(map[string]any{"position_type": "text", "new_line": 1}),
			wantSent: map[string]any{"position_type": "text", "new_line": float64(1)}, wantRead: true,
		},
		{
			name: "text by default on an added line", position: at(map[string]any{"new_line": 1}),
			wantSent: map[string]any{"position_type": "text", "new_line": float64(1)}, wantRead: true,
		},
		{
			name: "text on a line outside the diff", position: at(map[string]any{"position_type": "text", "new_line": 40}),
			wantErr: "outside the diff range", wantRead: true,
		},
		{
			name: "text by default on a line outside the diff", position: at(map[string]any{"new_line": 40}),
			wantErr: "outside the diff range", wantRead: true,
		},
		{
			name:     "text on a file the diff shows no patch for",
			position: at(map[string]any{"position_type": "text", "new_path": positionTypeBinaryPath, "new_line": 40}),
			wantSent: map[string]any{"position_type": "text", "new_line": float64(40)}, wantRead: true,
		},
		{
			name:     "image with width, height, x and y",
			position: at(map[string]any{"position_type": "image", "width": 64, "height": 32, "x": 12.5, "y": 8}),
			wantSent: map[string]any{
				"position_type": "image", "width": float64(64), "height": float64(32), "x": 12.5, "y": float64(8),
				"new_line": nil, "old_line": nil,
			},
		},
		{
			name:     "image at its top left corner",
			position: at(map[string]any{"position_type": "image", "width": 64, "height": 32, "x": 0, "y": 0}),
			wantSent: map[string]any{"position_type": "image", "x": float64(0), "y": float64(0)},
		},
		{
			name: "image without x and y", position: at(map[string]any{"position_type": "image", "width": 64, "height": 32}),
			wantErr: "missing: x, y",
		},
		{
			name: "image without width and height", position: at(map[string]any{"position_type": "image", "x": 12, "y": 8}),
			wantErr: "missing: width, height",
		},
		{
			name: "file with both paths", position: at(map[string]any{"position_type": "file", "old_path": positionTypePath}),
			wantSent: map[string]any{
				"position_type": "file", "old_path": positionTypePath, "new_path": positionTypePath,
				"new_line": nil, "old_line": nil,
			},
		},
		{
			name: "file without its old path", position: at(map[string]any{"position_type": "file"}),
			wantErr: "missing: old_path",
		},
		{
			name:     "file with a blank new path",
			position: at(map[string]any{"position_type": "file", "old_path": positionTypePath, "new_path": " "}),
			wantErr:  "missing: new_path",
		},
		{
			name:     "a type GitLab does not declare",
			position: at(map[string]any{"position_type": "Image", "width": 64, "height": 32, "x": 12, "y": 8}),
			wantErr:  "does not have a valid value", wantSent: map[string]any{"position_type": "Image"},
		},
	}
}

// check asserts what the write did with the case's position, against what
// the GitLab it was sent to recorded.
func (c positionTypeCase) check(t *testing.T, g *positionTypeGitLab, err error) {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case c.wantErr == "" && err != nil:
		t.Errorf("error = %v, want the write sent", err)
	case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
		t.Errorf("error = %v, want one naming %q", err, c.wantErr)
	}
	if read := g.reads > 0; read != c.wantRead {
		t.Errorf("the merge request and its diff were read %d times, want them read: %v", g.reads, c.wantRead)
	}
	if c.wantSent == nil {
		if g.sent != nil {
			t.Errorf("position %v reached GitLab, want it refused before the write", g.sent)
		}
		return
	}
	if g.sent == nil {
		t.Fatalf("no position reached GitLab, want one carrying %v", c.wantSent)
	}
	for key, want := range c.wantSent {
		assertSentField(t, g.sent, key, want)
	}
}

// TestWrites_EachPositionType_IsJudgedByTheFieldsGitLabRequiresForIt holds
// the position pre-check of mr_review.draft_note_create and
// mr_review.draft_note_update to what GitLab reads for each position_type. A
// text position, named or by default, is judged against the lines of the
// merge request's diff. An image position is judged by its width, height, x
// and y, and a file position by its old and new paths, before any request
// and without reading the diff, since GitLab places neither by a line. For a
// draft note that check is the only one: GitLab saves a draft whose image or
// file position is incomplete and publishes it as a general comment, its
// position dropped. A position_type GitLab does not declare is left to
// GitLab, which refuses it by name.
//
// On the parent every image and file case fails on both writes, and so does
// the undeclared type: the pre-check read the diff for each and refused it
// as a text position naming no line.
func TestWrites_EachPositionType_IsJudgedByTheFieldsGitLabRequiresForIt(t *testing.T) {
	for _, write := range draftNoteWrites() {
		t.Run(write.tool, func(t *testing.T) {
			for _, tc := range positionTypeCases() {
				t.Run(tc.name, func(t *testing.T) {
					gitlab := &positionTypeGitLab{}
					client := testutil.NewTestClient(t, gitlab.handler(t, write))

					tc.check(t, gitlab, write.call(t, client, tc.position))
				})
			}
		})
	}
}
