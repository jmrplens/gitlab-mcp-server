// terraform_states_test.go contains unit tests for the Terraform state MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package terraformstates

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestList verifies List.
func TestList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/graphql" {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformStates":{"nodes":[{"name":"state1","latestVersion":{"serial":5,"downloadPath":"/dl"}}]}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.States) != 1 || out.States[0].Name != "state1" {
		t.Errorf("unexpected states: %+v", out.States)
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectPath: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGet verifies Get.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/graphql" {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformState":{"name":"state1","latestVersion":{"serial":3}}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.LatestSerial != 3 {
		t.Errorf("expected serial 3, got %d", out.LatestSerial)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectPath: "x", Name: "y"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDelete verifies Delete.
func TestDelete(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteVersion verifies DeleteVersion.
func TestDeleteVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestLock verifies Lock.
func TestLock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Success {
		t.Error("expected success")
	}
}

// TestLock_Error verifies Lock when error.
func TestLock_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"already locked"}`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestLock_BadRequest_HintsSDKLimitation verifies that the guaranteed 400 from
// GitLab (client-go sends no Terraform lock-info body) is wrapped with a hint
// pointing at the terraform CLI and gitlab_unlock_terraform_state.
func TestLock_BadRequest_HintsSDKLimitation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad request"}`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), "lock-info body") || !strings.Contains(err.Error(), "gitlab_unlock_terraform_state") {
		t.Errorf("error = %q, want SDK-limitation hint with unlock alternative", err.Error())
	}
}

// TestUnlock verifies Unlock.
func TestUnlock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Success {
		t.Error("expected success")
	}
}

// TestUnlock_Error verifies Unlock when error.
func TestUnlock_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"not locked"}`)
	}))
	_, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// listHints is the guidance a Terraform state list closes with.
const listHints = "\n---\n💡 **Next steps:**\n" +
	"- Use `gitlab_get_terraform_state` to view details of a specific state\n"

// TestFormatListMarkdown verifies FormatListMarkdown renders the whole table:
// a heading counting the states, one row each, and a state nothing has
// written to yet saying so rather than showing a serial of zero.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{States: []StateItem{
		{Name: "state1", LatestSerial: 3},
		{Name: "fresh"},
	}})
	want := "## Terraform States (2)\n\n" +
		"| Name | Latest Serial |\n" +
		"| --- | --- |\n" +
		"| state1 | 3 |\n" +
		"| fresh | no versions |\n" +
		listHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// DeleteVersion — error
// ---------------------------------------------------------------------------.

// TestDeleteVersion_Error verifies DeleteVersion when error.
func TestDeleteVersion_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 99})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatStateMarkdown
// ---------------------------------------------------------------------------.

// stateHints is the guidance a Terraform state card closes with: what
// locking really needs, how to clear a stale lock, and how to delete.
const stateHints = "\n---\n💡 **Next steps:**\n" +
	"- " + hintLockNeedsCLI + "\n" +
	"- " + hintUnlockStaleLock + "\n" +
	"- Use `gitlab_delete_terraform_state` to remove it\n"

// TestFormatStateMarkdown_Coverage verifies FormatStateMarkdown renders the
// whole card for a written state, and that the guidance no longer sends a
// reader to gitlab_lock_terraform_state, which GitLab refuses on every call.
func TestFormatStateMarkdown_Coverage(t *testing.T) {
	got := FormatStateMarkdown(StateItem{Name: "prod-state", LatestSerial: 42, DownloadPath: "/dl/path"})
	want := "## Terraform State: prod-state\n\n" +
		"- **Latest Serial**: 42\n" +
		"- **Download Path**: `/dl/path`\n" +
		stateHints
	if got != want {
		t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatStateMarkdown_UnwrittenState verifies a state with no serial and
// no download path renders neither row and says why, rather than printing two
// labels with nothing after them.
func TestFormatStateMarkdown_UnwrittenState(t *testing.T) {
	got := FormatStateMarkdown(StateItem{Name: "fresh"})
	want := "## Terraform State: fresh\n\n" +
		"GitLab has recorded no versions of this state.\n" +
		stateHints
	if got != want {
		t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatLockMarkdown
// ---------------------------------------------------------------------------.

// TestFormatLockMarkdown_Coverage verifies FormatLockMarkdown renders the
// whole card, with the outcome as the flag glyph rather than the word "true".
func TestFormatLockMarkdown_Coverage(t *testing.T) {
	got := FormatLockMarkdown(LockOutput{Success: true, Message: "State 'x' locked"})
	want := "## Terraform State Lock\n\n" +
		"- **Success**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Message**: State 'x' locked\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + hintLockNeedsCLI + "\n" +
		"- " + hintUnlockStaleLock + "\n"
	if got != want {
		t.Errorf("FormatLockMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies an empty list is the one sentence and
// nothing else: no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{States: nil})
	want := "No Terraform states found.\n"
	if got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// TestTerraformStates_EachHandlerReachesItsOwnEndpoint drives all six handlers
// against the endpoints GitLab serves them on, so one pointed at a sibling's
// path fails rather than being answered by a catch-all.
func TestTerraformStates_EachHandlerReachesItsOwnEndpoint(t *testing.T) {
	client := testutil.NewTestClient(t, terraformHandler())

	tests := []struct {
		name string
		call func() error
	}{
		{name: "list", call: func() error {
			_, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
			return err
		}},
		{name: "get", call: func() error {
			_, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "state1"})
			return err
		}},
		{name: "delete", call: func() error {
			return Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
		}},
		{name: "delete_version", call: func() error {
			return DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 5})
		}},
		{name: "lock", call: func() error {
			_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
			return err
		}},
		{name: "unlock", call: func() error {
			_, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s error = %v, want nil", tt.name, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Shared mock handler
// ---------------------------------------------------------------------------.

// terraformHandler supports terraform handler assertions in terraformstates tests.
func terraformHandler() http.Handler {
	mux := http.NewServeMux()

	graphQLListResp := `{"data":{"project":{"terraformStates":{"nodes":[{"name":"state1","latestVersion":{"serial":5,"downloadPath":"/dl"}}]}}}}`
	graphQLGetResp := `{"data":{"project":{"terraformState":{"name":"state1","latestVersion":{"serial":3,"downloadPath":"/dl/state1"}}}}}`

	// GraphQL endpoint for List and Get
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		bodyStr := string(body[:n])
		if strings.Contains(bodyStr, "terraformStates") {
			testutil.RespondJSON(w, http.StatusOK, graphQLListResp)
		} else {
			testutil.RespondJSON(w, http.StatusOK, graphQLGetResp)
		}
	})

	// Delete state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete version
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/versions/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Lock state
	mux.HandleFunc("POST /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Unlock state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
}
