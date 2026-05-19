// terraformstates_test.go contains unit tests for the Terraform state MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package terraformstates

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{States: []StateItem{{Name: "state1", LatestSerial: 3}}})
	if md == "" {
		t.Error("expected non-empty markdown")
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

// TestFormatStateMarkdown_Coverage verifies FormatStateMarkdown when coverage.
func TestFormatStateMarkdown_Coverage(t *testing.T) {
	md := FormatStateMarkdown(StateItem{Name: "prod-state", LatestSerial: 42, DownloadPath: "/dl/path"})
	for _, want := range []string{"prod-state", "42", "/dl/path"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in markdown", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatLockMarkdown
// ---------------------------------------------------------------------------.

// TestFormatLockMarkdown_Coverage verifies FormatLockMarkdown when coverage.
func TestFormatLockMarkdown_Coverage(t *testing.T) {
	md := FormatLockMarkdown(LockOutput{Success: true, Message: "State 'x' locked"})
	if !strings.Contains(md, "true") {
		t.Error("missing success in markdown")
	}
	if !strings.Contains(md, "locked") {
		t.Error("missing lock message in markdown")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{States: nil})
	if !strings.Contains(md, "No Terraform states found") {
		t.Error("missing empty message")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution — all 6 individual tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes covers ActionSpecs with table-driven subtests for call routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, terraformHandler())
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_terraform_states", map[string]any{"project_path": "group/project"}},
		{"get", "gitlab_get_terraform_state", map[string]any{"project_path": "group/project", "name": "state1"}},
		{"delete", "gitlab_delete_terraform_state", map[string]any{"project_id": "1", "name": "state1"}},
		{"delete_version", "gitlab_delete_terraform_state_version", map[string]any{"project_id": "1", "name": "state1", "serial": float64(5)}},
		{"lock", "gitlab_lock_terraform_state", map[string]any{"project_id": "1", "name": "state1"}},
		{"unlock", "gitlab_unlock_terraform_state", map[string]any{"project_id": "1", "name": "state1"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
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
