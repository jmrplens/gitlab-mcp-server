package groupprotectedbranches

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerBranchJSON = `{
	"id": 1,
	"name": "main",
	"push_access_levels": [{"access_level": 40, "access_level_description": "Maintainers"}],
	"merge_access_levels": [{"access_level": 40, "access_level_description": "Maintainers"}],
	"allow_force_push": false,
	"code_owner_approval_required": false
}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all group
// protected branch tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered group protected branch
// tools can be called through MCP in-memory transport, covering handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	// List
	mux.HandleFunc("GET /api/v4/groups/{gid}/protected_branches", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerBranchJSON+`]`)
	})
	// Get single (path with name segment)
	mux.HandleFunc("GET /api/v4/groups/{gid}/protected_branches/{name}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerBranchJSON)
	})
	// Protect (POST)
	mux.HandleFunc("POST /api/v4/groups/{gid}/protected_branches", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, registerBranchJSON)
	})
	// Update (PATCH)
	mux.HandleFunc("PATCH /api/v4/groups/{gid}/protected_branches/{name}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerBranchJSON)
	})
	// Unprotect (DELETE)
	mux.HandleFunc("DELETE /api/v4/groups/{gid}/protected_branches/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
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

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_protected_branch_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_protected_branch_get", map[string]any{"group_id": "42", "branch": "main"}},
		{"gitlab_group_protected_branch_protect", map[string]any{"group_id": "42", "name": "main"}},
		{"gitlab_group_protected_branch_update", map[string]any{"group_id": "42", "branch": "main"}},
		{"gitlab_group_protected_branch_unprotect", map[string]any{"group_id": "42", "branch": "main"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tt.name)
			}
		})
	}
}

// TestRegisterTools_UnprotectError verifies that the unprotect handler returns
// an error result when the GitLab API fails, covering the if-err-not-nil branch.
func TestRegisterTools_UnprotectError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
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
		Name:      "gitlab_group_protected_branch_unprotect",
		Arguments: map[string]any{"group_id": "42", "branch": "main"},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result from unprotect with failing backend")
	}
}
