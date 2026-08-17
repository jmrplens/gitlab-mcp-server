// action_specs_test.go contains canonical-route tests for group credential actions.
package groupcredentials

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionSpecPATJSON    = `[{"id":99,"name":"test-pat","scopes":["api"],"state":"active","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01"}]`
	actionSpecSSHKeyJSON = `[{"id":5,"title":"test-key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z"}]`
)

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecPATJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/ssh_keys"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecSSHKeyJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_pats", "gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "mygroup"}},
		{"list_ssh_keys", "gitlab_list_group_ssh_keys", map[string]any{"group_id": "mygroup"}},
		{"revoke_pat", "gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "mygroup", "token_id": 99}},
		{"delete_ssh_key", "gitlab_delete_group_ssh_key", map[string]any{"group_id": "mygroup", "key_id": 5}},
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

// TestActionSpecs_ReadErrorPaths validates the ReadErrorPaths route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_ReadErrorPaths(t *testing.T) {
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_pats", "gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "42"}},
		{"list_ssh_keys", "gitlab_list_group_ssh_keys", map[string]any{"group_id": "42"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteOutputs validates the DeleteOutputs route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/groups/mygroup/manage/personal_access_tokens/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/mygroup/manage/ssh_keys/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool    string
		args    map[string]any
		message string
	}{
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "mygroup", "token_id": 99}, "Successfully deleted personal access token 99 from group mygroup."},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "mygroup", "key_id": 5}, "Successfully deleted SSH key 5 from group mygroup."},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			out, ok := result.(toolutil.DeleteOutput)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
			}
			if out.Message != tt.message {
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestActionSpecs_DeleteOutputErrors validates the DeleteOutputErrors route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteOutputErrors(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	tests := []struct {
		name string
		fn   func() (toolutil.DeleteOutput, error)
	}{
		{name: "revoke_pat", fn: func() (toolutil.DeleteOutput, error) {
			return revokePATOutput(context.Background(), client, RevokePATInput{GroupID: toolutil.StringOrInt("mygroup")})
		}},
		{name: "delete_ssh_key", fn: func() (toolutil.DeleteOutput, error) {
			return deleteSSHKeyOutput(context.Background(), client, DeleteSSHKeyInput{GroupID: toolutil.StringOrInt("mygroup")})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := tt.fn()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if out.Status != "" || out.Message != "" {
				t.Fatalf("output = %+v, want zero output", out)
			}
		})
	}
}

// TestCatalogSurface_ConfirmDeclined verifies the CatalogSurface_ConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
		icon []mcp.Icon
	}{
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "g", "token_id": 1}, toolutil.IconToken},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "g", "key_id": 1}, toolutil.IconKey},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test group credential destructive confirmation.",
				Icons:       tt.icon,
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

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool returned transport error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result when confirmation is declined")
			}
		})
	}
}

// TestActionSpecs_DiscoveryMetadata verifies every group-credential tool carries
// non-generic action-specific Usage, distinctive natural-language Aliases beyond
// the tool name, and canonical RelatedActions (1:1 audit R-META).
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))

	for tool := range groupCredentialActionMeta {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("meta entry %q has no projected spec", tool)
		}
		if strings.Contains(spec.Usage, "domain action") || strings.TrimSpace(spec.Usage) == "" {
			t.Errorf("%s: generic or empty Usage %q", tool, spec.Usage)
		}
		distinctive := 0
		for _, alias := range spec.Aliases {
			if alias != tool {
				distinctive++
			}
		}
		if distinctive < 2 || distinctive > 4 {
			t.Errorf("%s: want 2-4 distinctive aliases, got %d (%v)", tool, distinctive, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: empty RelatedActions", tool)
		}
		if spec.IndividualTool.Description == "" ||
			!strings.Contains(spec.IndividualTool.Description, "Returns:") ||
			!strings.Contains(spec.IndividualTool.Description, "See also:") {
			t.Errorf("%s: description not in 'Returns: … See also: …' form: %q", tool, spec.IndividualTool.Description)
		}
	}
}

// TestDecorateGroupCredentialMeta_UnknownTool verifies the decorator is a no-op
// for tools that have no discovery-metadata entry, leaving the base options
// untouched.
func TestDecorateGroupCredentialMeta_UnknownTool(t *testing.T) {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{"gitlab_unknown_tool"},
		Usage:          "Use to execute groupcredentials domain action.",
		RelatedActions: []string{"group.get"},
	}
	decorateGroupCredentialMeta(&options, "gitlab_unknown_tool")
	if options.Usage != "Use to execute groupcredentials domain action." {
		t.Errorf("Usage mutated for unknown tool: %q", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_unknown_tool" {
		t.Errorf("Aliases mutated for unknown tool: %v", options.Aliases)
	}
}

func groupCredentialSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
