package externalstatuschecks

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const mergeCheckJSON = `[{"id":1,"name":"CI Check","external_url":"https://ci.example.com","status":"passed"}]`
const projectCheckJSON = `[{"id":1,"name":"CI Check","external_url":"https://ci.example.com","hmac":true,"protected_branches":[{"id":1,"name":"main"}]}]`
const createdCheckJSON = `{"id":2,"name":"New Check","external_url":"https://new.example.com","hmac":false,"protected_branches":[]}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all external
// status check tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 14 registered external status check tools
// can be called through MCP in-memory transport, covering every handler closure in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(r.URL.Path, "merge_requests") {
				testutil.RespondJSON(w, http.StatusOK, mergeCheckJSON)
			} else {
				testutil.RespondJSON(w, http.StatusOK, projectCheckJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, createdCheckJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, createdCheckJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
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
		{"gitlab_list_merge_status_checks", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_set_external_status_check_status", map[string]any{"project_id": "42", "mr_iid": 1, "sha": "abc123", "external_status_check_id": 1, "status": "passed"}},
		{"gitlab_list_project_status_checks", map[string]any{"project_id": "42"}},
		{"gitlab_create_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_status_check_for_mr", map[string]any{"project_id": "42", "mr_iid": 1, "check_id": 1}},
		{"gitlab_list_project_mr_external_status_checks", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_list_project_external_status_checks", map[string]any{"project_id": "42"}},
		{"gitlab_create_project_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_external_status_check_for_project_mr", map[string]any{"project_id": "42", "mr_iid": 1, "check_id": 1}},
		{"gitlab_set_project_mr_external_status_check_status", map[string]any{"project_id": "42", "mr_iid": 1, "sha": "abc123", "external_status_check_id": 1, "status": "passed"}},
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

// TestRegisterTools_MutationErrors verifies that mutating tool closures in register.go
// return error results when the GitLab API responds with 500, covering the if-err
// branches after Set/Create/Update/Delete/Retry calls.
func TestRegisterTools_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
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
		{"gitlab_set_external_status_check_status", map[string]any{"project_id": "42", "mr_iid": 1, "sha": "abc", "external_status_check_id": 1, "status": "passed"}},
		{"gitlab_create_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_status_check_for_mr", map[string]any{"project_id": "42", "mr_iid": 1, "check_id": 1}},
		{"gitlab_create_project_external_status_check", map[string]any{"project_id": "42", "name": "check", "external_url": "https://ci.example.com"}},
		{"gitlab_delete_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_update_project_external_status_check", map[string]any{"project_id": "42", "check_id": 1}},
		{"gitlab_retry_failed_external_status_check_for_project_mr", map[string]any{"project_id": "42", "mr_iid": 1, "check_id": 1}},
		{"gitlab_set_project_mr_external_status_check_status", map[string]any{"project_id": "42", "mr_iid": 1, "sha": "abc", "external_status_check_id": 1, "status": "passed"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.name, err)
			}
			if result == nil || !result.IsError {
				t.Errorf("expected error result from %s with failing backend", tt.name)
			}
		})
	}
}
