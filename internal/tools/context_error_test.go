// context_error_test.go contains tests that verify every tool handler
// correctly propagates context cancellation errors and GitLab API error
// responses. Each domain section tests both canceled-context and HTTP
// error code scenarios.
package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
)

const (
	// msgCancelledCtxErr is the assertion message for tests expecting a canceled context error.
	msgCancelledCtxErr = "expected error for canceled context"
	// msgForbiddenErr is the assertion message for tests expecting a 403 Forbidden error.
	msgForbiddenErr = "expected error for 403 response"
	// msgNotFoundErr is the assertion message for tests expecting a 404 Not Found error.
	msgNotFoundErr = "expected error for 404 response"

	jsonNotFound  = `{"message":"404 Not Found"}`
	jsonForbidden = `{"message":"403 Forbidden"}`
)

// ----------- Branch context/error tests -----------.

// TestBranchProtect_ContextCancelled verifies the behavior of branch protect context cancelled.
func TestBranchProtect_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := branches.Protect(ctx, client, branches.ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchProtect_APIError verifies the behavior of branch protect a p i error.
func TestBranchProtect_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := branches.Protect(context.Background(), client, branches.ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// TestBranchProtect_AllowForcePush verifies the behavior of branch protect allow force push.
func TestBranchProtect_AllowForcePush(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusCreated, `{"id":1,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":true}`)
	}))

	out, err := branches.Protect(context.Background(), client, branches.ProtectInput{
		ProjectID:      "42",
		BranchName:     "main",
		AllowForcePush: new(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.AllowForcePush {
		t.Error("expected AllowForcePush=true")
	}
}

// TestBranchUnprotect_ContextCancelled verifies the behavior of branch unprotect context cancelled.
func TestBranchUnprotect_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := branches.Unprotect(ctx, client, branches.UnprotectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchCreate_ContextCancelled verifies the behavior of branch create context cancelled.
func TestBranchCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := branches.Create(ctx, client, branches.CreateInput{ProjectID: "42", BranchName: "dev", Ref: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchList_ContextCancelled verifies the behavior of branch list context cancelled.
func TestBranchList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := branches.List(ctx, client, branches.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProtectedBranchesList_ContextCancelled verifies the behavior of protected branches list context cancelled.
func TestProtectedBranchesList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := branches.ProtectedList(ctx, client, branches.ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestBranchList_APIError verifies the behavior of branch list a p i error.
func TestBranchList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"500 Server Error"}`)
	}))

	_, err := branches.List(context.Background(), client, branches.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestProtectedBranchesList_APIError verifies the behavior of protected branches list a p i error.
func TestProtectedBranchesList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := branches.ProtectedList(context.Background(), client, branches.ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// ----------- Tag context/error tests -----------.

// TestTagCreate_ContextCancelled verifies the behavior of tag create context cancelled.
func TestTagCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tags.Create(ctx, client, tags.CreateInput{ProjectID: "42", TagName: "v1.0", Ref: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestTagDelete_ContextCancelled verifies the behavior of tag delete context cancelled.
func TestTagDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := tags.Delete(ctx, client, tags.DeleteInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestTagList_ContextCancelled verifies the behavior of tag list context cancelled.
func TestTagList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tags.List(ctx, client, tags.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestTagList_APIError verifies the behavior of tag list a p i error.
func TestTagList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := tags.List(context.Background(), client, tags.ListInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- Release context/error tests -----------.

// TestReleaseCreate_ContextCancelled verifies the behavior of release create context cancelled.
func TestReleaseCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releases.Create(ctx, client, releases.CreateInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseGet_ContextCancelled verifies the behavior of release get context cancelled.
func TestReleaseGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releases.Get(ctx, client, releases.GetInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseUpdate_ContextCancelled verifies the behavior of release update context cancelled.
func TestReleaseUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releases.Update(ctx, client, releases.UpdateInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseDelete_ContextCancelled verifies the behavior of release delete context cancelled.
func TestReleaseDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releases.Delete(ctx, client, releases.DeleteInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseList_ContextCancelled verifies the behavior of release list context cancelled.
func TestReleaseList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releases.List(ctx, client, releases.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseGet_APIError verifies the behavior of release get a p i error.
func TestReleaseGet_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releases.Get(context.Background(), client, releases.GetInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseDelete_APIError verifies the behavior of release delete a p i error.
func TestReleaseDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releases.Delete(context.Background(), client, releases.DeleteInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseUpdate_APIError verifies the behavior of release update a p i error.
func TestReleaseUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releases.Update(context.Background(), client, releases.UpdateInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseList_APIError verifies the behavior of release list a p i error.
func TestReleaseList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := releases.List(context.Background(), client, releases.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// ----------- Release Link context/error tests -----------.

// TestReleaseLinkCreate_ContextCancelled verifies the behavior of release link create context cancelled.
func TestReleaseLinkCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releaselinks.Create(ctx, client, releaselinks.CreateInput{ProjectID: "42", TagName: "v1.0", Name: "bin", URL: "https://example.com"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseLinkDelete_ContextCancelled verifies the behavior of release link delete context cancelled.
func TestReleaseLinkDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releaselinks.Delete(ctx, client, releaselinks.DeleteInput{ProjectID: "42", TagName: "v1.0", LinkID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseLinkList_ContextCancelled verifies the behavior of release link list context cancelled.
func TestReleaseLinkList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := releaselinks.List(ctx, client, releaselinks.ListInput{ProjectID: "42", TagName: "v1.0"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestReleaseLinkDelete_APIError verifies the behavior of release link delete a p i error.
func TestReleaseLinkDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releaselinks.Delete(context.Background(), client, releaselinks.DeleteInput{ProjectID: "42", TagName: "v1.0", LinkID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestReleaseLinkList_APIError verifies the behavior of release link list a p i error.
func TestReleaseLinkList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := releaselinks.List(context.Background(), client, releaselinks.ListInput{ProjectID: "42", TagName: "v999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- MR context/error tests -----------.

// TestMRCreate_ContextCancelled verifies the behavior of m r create context cancelled.
func TestMRCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mergerequests.Create(ctx, client, mergerequests.CreateInput{ProjectID: "42", SourceBranch: "dev", TargetBranch: "main", Title: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRGet_ContextCancelled verifies the behavior of m r get context cancelled.
func TestMRGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mergerequests.Get(ctx, client, mergerequests.GetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRList_ContextCancelled verifies the behavior of m r list context cancelled.
func TestMRList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mergerequests.List(ctx, client, mergerequests.ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRUpdate_ContextCancelled verifies the behavior of m r update context cancelled.
func TestMRUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mergerequests.Update(ctx, client, mergerequests.UpdateInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRMerge_ContextCancelled verifies the behavior of m r merge context cancelled.
func TestMRMerge_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mergerequests.Merge(ctx, client, mergerequests.MergeInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRApprove_ContextCancelled verifies the behavior of m r approve context cancelled.
func TestMRApprove_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mergerequests.Approve(ctx, client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRUnapprove_ContextCancelled verifies the behavior of m r unapprove context cancelled.
func TestMRUnapprove_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mergerequests.Unapprove(ctx, client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRApprove_APIError verifies the behavior of m r approve a p i error.
func TestMRApprove_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := mergerequests.Approve(context.Background(), client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// TestMRUnapprove_APIError verifies the behavior of m r unapprove a p i error.
func TestMRUnapprove_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	err := mergerequests.Unapprove(context.Background(), client, mergerequests.ApproveInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// TestMRUpdate_APIError verifies the behavior of m r update a p i error.
func TestMRUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mergerequests.Update(context.Background(), client, mergerequests.UpdateInput{ProjectID: "42", MRIID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRMerge_APIError verifies the behavior of m r merge a p i error.
func TestMRMerge_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusMethodNotAllowed, `{"message":"405 Method Not Allowed - cannot merge"}`)
	}))

	_, err := mergerequests.Merge(context.Background(), client, mergerequests.MergeInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal("expected error for 405 response")
	}
}

// TestMRList_APIError verifies the behavior of m r list a p i error.
func TestMRList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mergerequests.List(context.Background(), client, mergerequests.ListInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- MR Notes context/error tests -----------.

// TestMRNoteCreate_ContextCancelled verifies the behavior of m r note create context cancelled.
func TestMRNoteCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrnotes.Create(ctx, client, mrnotes.CreateInput{ProjectID: "42", MRIID: 1, Body: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNotesList_ContextCancelled verifies the behavior of m r notes list context cancelled.
func TestMRNotesList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrnotes.List(ctx, client, mrnotes.ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNoteUpdate_ContextCancelled verifies the behavior of m r note update context cancelled.
func TestMRNoteUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrnotes.Update(ctx, client, mrnotes.UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 1, Body: "new"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNoteDelete_ContextCancelled verifies the behavior of m r note delete context cancelled.
func TestMRNoteDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mrnotes.Delete(ctx, client, mrnotes.DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRNoteUpdate_APIError verifies the behavior of m r note update a p i error.
func TestMRNoteUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrnotes.Update(context.Background(), client, mrnotes.UpdateInput{ProjectID: "42", MRIID: 1, NoteID: 999, Body: "x"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRNoteDelete_APIError verifies the behavior of m r note delete a p i error.
func TestMRNoteDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	err := mrnotes.Delete(context.Background(), client, mrnotes.DeleteInput{ProjectID: "42", MRIID: 1, NoteID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRNotesList_APIError verifies the behavior of m r notes list a p i error.
func TestMRNotesList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrnotes.List(context.Background(), client, mrnotes.ListInput{ProjectID: "42", MRIID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// ----------- MR Discussion context/error tests -----------.

// TestMRDiscussionCreate_ContextCancelled verifies the behavior of m r discussion create context cancelled.
func TestMRDiscussionCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrdiscussions.Create(ctx, client, mrdiscussions.CreateInput{ProjectID: "42", MRIID: 1, Body: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionResolve_ContextCancelled verifies the behavior of m r discussion resolve context cancelled.
func TestMRDiscussionResolve_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrdiscussions.Resolve(ctx, client, mrdiscussions.ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc", Resolved: true})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionReply_ContextCancelled verifies the behavior of m r discussion reply context cancelled.
func TestMRDiscussionReply_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrdiscussions.Reply(ctx, client, mrdiscussions.ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "abc", Body: "test"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionList_ContextCancelled verifies the behavior of m r discussion list context cancelled.
func TestMRDiscussionList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrdiscussions.List(ctx, client, mrdiscussions.ListInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestMRDiscussionResolve_APIError verifies the behavior of m r discussion resolve a p i error.
func TestMRDiscussionResolve_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrdiscussions.Resolve(context.Background(), client, mrdiscussions.ResolveInput{ProjectID: "42", MRIID: 1, DiscussionID: "xyz", Resolved: true})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRDiscussionReply_APIError verifies the behavior of m r discussion reply a p i error.
func TestMRDiscussionReply_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrdiscussions.Reply(context.Background(), client, mrdiscussions.ReplyInput{ProjectID: "42", MRIID: 1, DiscussionID: "xyz", Body: "x"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRDiscussionList_APIError verifies the behavior of m r discussion list a p i error.
func TestMRDiscussionList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := mrdiscussions.List(context.Background(), client, mrdiscussions.ListInput{ProjectID: "42", MRIID: 999})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestMRDiscussionCreate_APIError verifies the behavior of m r discussion create a p i error.
func TestMRDiscussionCreate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))

	_, err := mrdiscussions.Create(context.Background(), client, mrdiscussions.CreateInput{ProjectID: "42", MRIID: 1, Body: "test"})
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
}

// ----------- MR Changes context/error tests -----------.

// TestMRChangesGet_ContextCancelled verifies the behavior of m r changes get context cancelled.
func TestMRChangesGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mrchanges.Get(ctx, client, mrchanges.GetInput{ProjectID: "42", MRIID: 1})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// ----------- Commit context/error tests -----------.

// TestCommitCreate_ContextCancelled verifies the behavior of commit create context cancelled.
func TestCommitCreate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := commits.Create(ctx, client, commits.CreateInput{ProjectID: "42", Branch: "main", CommitMessage: "test", Actions: []commits.Action{{Action: "create", FilePath: "f.txt", Content: "x"}}})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// ----------- File context/error tests -----------.

// TestFileGet_ContextCancelled verifies the behavior of file get context cancelled.
func TestFileGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := files.Get(ctx, client, files.GetInput{ProjectID: "42", FilePath: "README.md", Ref: "main"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// ----------- Repository context/error tests -----------.

// TestProjectGet_ContextCancelled verifies the behavior of project get context cancelled.
func TestProjectGet_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := projects.Get(ctx, client, projects.GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectList_ContextCancelled verifies the behavior of project list context cancelled.
func TestProjectList_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := projects.List(ctx, client, projects.ListInput{})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectDelete_ContextCancelled verifies the behavior of project delete context cancelled.
func TestProjectDelete_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := projects.Delete(ctx, client, projects.DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectUpdate_ContextCancelled verifies the behavior of project update context cancelled.
func TestProjectUpdate_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := projects.Update(ctx, client, projects.UpdateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgCancelledCtxErr)
	}
}

// TestProjectUpdate_APIError verifies the behavior of project update a p i error.
func TestProjectUpdate_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := projects.Update(context.Background(), client, projects.UpdateInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestProjectList_APIError verifies the behavior of project list a p i error.
func TestProjectList_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"500 Error"}`)
	}))

	_, err := projects.List(context.Background(), client, projects.ListInput{})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestProjectGet_APIError verifies the behavior of project get a p i error.
func TestProjectGet_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := projects.Get(context.Background(), client, projects.GetInput{ProjectID: "999"})
	if err == nil {
		t.Fatal(msgNotFoundErr)
	}
}

// TestProjectDelete_APIError verifies the behavior of project delete a p i error.
func TestProjectDelete_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, jsonForbidden)
	}))

	_, err := projects.Delete(context.Background(), client, projects.DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(msgForbiddenErr)
	}
}

// ----------- Metatool additional tests -----------.

// TestUnmarshalParams_InvalidJSON verifies the behavior of unmarshal params invalid j s o n.
func TestUnmarshalParams_InvalidJSON(t *testing.T) {
	params := map[string]any{
		"project_id": make(chan int),
	}
	_, err := unmarshalParams[projects.GetInput](params)
	if err == nil {
		t.Fatal("expected error for un-marshalable params")
	}
}

// TestWrapActionUnmarshal_Error verifies the behavior of wrap action unmarshal error.
func TestWrapActionUnmarshal_Error(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))

	action := wrapAction(client, projects.Get)
	_, err := action(context.Background(), map[string]any{"project_id": make(chan int)})
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
}

// TestWrapVoidActionUnmarshal_Error verifies the behavior of wrap void action unmarshal error.
func TestWrapVoidActionUnmarshal_Error(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	action := wrapVoidAction(client, uploads.Delete)
	_, err := action(context.Background(), map[string]any{"project_id": make(chan int)})
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
}
