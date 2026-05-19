// action_specs_test.go contains canonical-route tests for group SSH certificate actions.
package groupsshcerts

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionSpecCertJSON = `{"id":1,"title":"test-cert","key":"ssh-rsa AAAA...","created_at":"2026-01-15T10:30:00Z"}`

// TestActionSpecs_CallAllRoutes exercises every group SSH certificate tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/42/ssh_certificates", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecCertJSON+`]`)
	})
	handler.HandleFunc("POST /api/v4/groups/42/ssh_certificates", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecCertJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/42/ssh_certificates/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupSSHCertSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_group_ssh_certificates", map[string]any{"group_id": "42"}},
		{"create", "gitlab_create_group_ssh_certificate", map[string]any{"group_id": "42", "key": "ssh-rsa AAAA...", "title": "test-cert"}},
		{"delete", "gitlab_delete_group_ssh_certificate", map[string]any{"group_id": "42", "certificate_id": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteError verifies the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := groupSSHCertSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_group_ssh_certificate"].Route.Handler(t.Context(), map[string]any{
		"group_id":       "42",
		"certificate_id": 1,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/42/ssh_certificates/1" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := groupSSHCertSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_delete_group_ssh_certificate"].Route.Handler(t.Context(), map[string]any{
		"group_id":       "42",
		"certificate_id": 1,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_group_ssh_certificate) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_group_ssh_certificate) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted SSH certificate 1 from group 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := groupSSHCertSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_group_ssh_certificate"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test group SSH certificate destructive confirmation.",
		Icons:       toolutil.IconKey,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_group_ssh_certificate",
		Arguments: map[string]any{"group_id": "42", "certificate_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func groupSSHCertSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
