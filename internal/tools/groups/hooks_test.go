// hooks_test.go contains unit tests for the group MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groups

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	pathGroupHooks         = "/api/v4/groups/99/hooks"
	pathGroupHook10        = "/api/v4/groups/99/hooks/10"
	testHookURL            = "https://example.com/hook"
	errZeroHookID          = "expected error for zero HookID"
	fmtExpectedHookIDError = "expected error to mention 'hook_id', got: %v"
)

var groupHookJSON = `{"id":10,"url":"https://example.com/hook","name":"CI Hook","description":"Triggers CI","group_id":99,"push_events":true,"merge_requests_events":true,"issues_events":false,"tag_push_events":false,"note_events":false,"job_events":false,"pipeline_events":true,"wiki_page_events":false,"deployment_events":false,"releases_events":false,"subgroup_events":false,"member_events":false,"confidential_issues_events":false,"confidential_note_events":false,"enable_ssl_verification":true,"alert_status":"executable","created_at":"2026-01-15T10:00:00Z"}`

var groupHookListJSON = `[` + groupHookJSON + `]`

// ---------------------------------------------------------------------------
// ListHooks tests
// ---------------------------------------------------------------------------.

// TestListHooks_Success verifies the behavior of list hooks success.
func TestListHooks_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupHooks {
			testutil.RespondJSONWithPagination(w, http.StatusOK, groupHookListJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListHooks(context.Background(), client, ListHooksInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("ListHooks() unexpected error: %v", err)
	}
	if len(out.Hooks) != 1 {
		t.Fatalf("len(out.Hooks) = %d, want 1", len(out.Hooks))
	}
	if out.Hooks[0].URL != testHookURL {
		t.Errorf("out.Hooks[0].URL = %q, want %q", out.Hooks[0].URL, testHookURL)
	}
	if !out.Hooks[0].PushEvents {
		t.Error("out.Hooks[0].PushEvents = false, want true")
	}
}

// TestListHooks_APIError verifies the behavior of list hooks a p i error.
func TestListHooks_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListHooks(context.Background(), client, ListHooksInput{GroupID: "99"})
	if err == nil {
		t.Fatal("ListHooks() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetHook tests
// ---------------------------------------------------------------------------.

// TestGetHook_Success verifies the behavior of get hook success.
func TestGetHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupHook10 {
			testutil.RespondJSON(w, http.StatusOK, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 10})
	if err != nil {
		t.Fatalf("GetHook() unexpected error: %v", err)
	}
	if out.Name != "CI Hook" {
		t.Errorf("out.Name = %q, want %q", out.Name, "CI Hook")
	}
	if !out.EnableSSLVerification {
		t.Error("out.EnableSSLVerification = false, want true")
	}
}

// TestGetHook_APIError verifies the behavior of get hook a p i error.
func TestGetHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 999})
	if err == nil {
		t.Fatal("GetHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// AddHook tests
// ---------------------------------------------------------------------------.

// TestAddHook_Success verifies the behavior of add hook success.
func TestAddHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupHooks {
			testutil.RespondJSON(w, http.StatusCreated, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	push := true
	out, err := AddHook(context.Background(), client, AddHookInput{
		GroupID: "99",
		HookInput: HookInput{
			URL:        testHookURL,
			PushEvents: &push,
		},
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
}

// TestAddHook_APIError verifies the behavior of add hook a p i error.
func TestAddHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	_, err := AddHook(context.Background(), client, AddHookInput{
		GroupID:   "99",
		HookInput: HookInput{URL: "https://bad.example.com"},
	})
	if err == nil {
		t.Fatal("AddHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// EditHook tests
// ---------------------------------------------------------------------------.

// TestEditHook_Success verifies the behavior of edit hook success.
func TestEditHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupHook10 {
			testutil.RespondJSON(w, http.StatusOK, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := EditHook(context.Background(), client, EditHookInput{
		GroupID: "99",
		HookID:  10,
		HookInput: HookInput{
			URL: testHookURL,
		},
	})
	if err != nil {
		t.Fatalf("EditHook() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
}

// TestEditHook_APIError verifies the behavior of edit hook a p i error.
func TestEditHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := EditHook(context.Background(), client, EditHookInput{
		GroupID:   "99",
		HookID:    999,
		HookInput: HookInput{URL: "https://example.com"},
	})
	if err == nil {
		t.Fatal("EditHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteHook tests
// ---------------------------------------------------------------------------.

// TestDeleteHook_Success verifies the behavior of delete hook success.
func TestDeleteHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGroupHook10 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{GroupID: "99", HookID: 10})
	if err != nil {
		t.Fatalf("DeleteHook() unexpected error: %v", err)
	}
}

// TestDeleteHook_APIError verifies the behavior of delete hook a p i error.
func TestDeleteHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{GroupID: "99", HookID: 10})
	if err == nil {
		t.Fatal("DeleteHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// HookID validation tests
// ---------------------------------------------------------------------------.

// TestGetHook_InvalidHookID verifies the behavior of get hook invalid hook i d.
func TestGetHook_InvalidHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 0})
	if err == nil {
		t.Fatal(errZeroHookID)
	}
	if !strings.Contains(err.Error(), "hook_id") {
		t.Errorf(fmtExpectedHookIDError, err)
	}
}

// TestEditHook_InvalidHookID verifies the behavior of edit hook invalid hook i d.
func TestEditHook_InvalidHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := EditHook(context.Background(), client, EditHookInput{GroupID: "99", HookID: 0})
	if err == nil {
		t.Fatal(errZeroHookID)
	}
	if !strings.Contains(err.Error(), "hook_id") {
		t.Errorf(fmtExpectedHookIDError, err)
	}
}

// TestDeleteHook_InvalidHookID verifies the behavior of delete hook invalid hook i d.
func TestDeleteHook_InvalidHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{GroupID: "99", HookID: 0})
	if err == nil {
		t.Fatal(errZeroHookID)
	}
	if !strings.Contains(err.Error(), "hook_id") {
		t.Errorf(fmtExpectedHookIDError, err)
	}
}
