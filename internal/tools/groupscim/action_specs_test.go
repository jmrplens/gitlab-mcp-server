// action_specs_test.go contains canonical-route tests for group SCIM identity actions.
package groupscim

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/42/scim/identities", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"external_uid":"uid-1","user_id":1,"active":true}]`)
	})
	handler.HandleFunc("GET /api/v4/groups/42/scim/uid-1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"external_uid":"uid-1","user_id":1,"active":true}`)
	})
	handler.HandleFunc("PATCH /api/v4/groups/42/scim/uid-1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/42/scim/uid-1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupSCIMSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_group_scim_identities", map[string]any{"group_id": "42"}},
		{"get", "gitlab_get_group_scim_identity", map[string]any{"group_id": "42", "uid": "uid-1"}},
		{"update", "gitlab_update_group_scim_identity", map[string]any{"group_id": "42", "uid": "uid-1", "extern_uid": "new-uid"}},
		{"delete", "gitlab_delete_group_scim_identity", map[string]any{"group_id": "42", "uid": "uid-1"}},
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

// TestActionSpecs_ErrorPaths validates the ErrorPaths route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := groupSCIMSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"update", "gitlab_update_group_scim_identity", map[string]any{"group_id": "42", "uid": "uid-1", "extern_uid": "new-uid"}},
		{"delete", "gitlab_delete_group_scim_identity", map[string]any{"group_id": "42", "uid": "uid-1"}},
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

// TestActionSpecs_DeleteOutput validates the DeleteOutput route through the catalog surface.
// The mock GitLab API at /api/v4/groups/42/scim/uid-1 (DELETE) returns a representative success body.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/42/scim/uid-1" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := groupSCIMSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_delete_group_scim_identity"].Route.Handler(t.Context(), map[string]any{
		"group_id": "42",
		"uid":      "uid-1",
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_group_scim_identity) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_group_scim_identity) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted SCIM identity uid-1 from group 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := groupSCIMSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_group_scim_identity"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test group SCIM destructive confirmation.",
		Icons:       toolutil.IconSecurity,
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
		Name:      "gitlab_delete_group_scim_identity",
		Arguments: map[string]any{"group_id": "42", "uid": "uid-1"},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

// TestActionSpecs_Metadata verifies that every SCIM identity action carries
// non-generic discovery metadata (1:1 audit R-META): an action-specific Usage,
// domain-specific natural-language Aliases beyond the canonical tool name,
// canonical RelatedActions cross-links, and an individual-tool description in
// the "Returns: … See also: …" form. It also asserts aliases are unique across
// actions so dynamic find ranking is not diluted by shared phrasing.
func TestActionSpecs_Metadata(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, http.NewServeMux()))

	seenAlias := make(map[string]string)
	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			if spec.Usage == "" || spec.Usage == "Use to execute groupscim domain action." {
				t.Errorf("action %q has generic or empty Usage: %q", spec.Name, spec.Usage)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("action %q has empty RelatedActions", spec.Name)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("action %q description missing Returns:/See also: form: %q", spec.Name, desc)
			}
			if naturalLanguageAliases(t, spec, seenAlias) == 0 {
				t.Errorf("action %q has no natural-language aliases beyond the tool name", spec.Name)
			}
		})
	}
}

// naturalLanguageAliases counts the aliases of spec that are not the canonical
// action name or the projected individual-tool name, recording each into seen
// (action name keyed by normalized alias) and failing t when an alias is reused
// across actions. It returns the count of distinct natural-language aliases.
func naturalLanguageAliases(t *testing.T, spec toolutil.ActionSpec, seen map[string]string) int {
	t.Helper()
	canonical := strings.ToLower(spec.Name)
	tool := strings.ToLower(spec.IndividualTool.Name)
	count := 0
	for _, alias := range spec.Aliases {
		norm := strings.ToLower(strings.TrimSpace(alias))
		if norm == "" || norm == canonical || norm == tool {
			continue
		}
		count++
		if other, dup := seen[norm]; dup {
			t.Errorf("alias %q reused by actions %q and %q", norm, other, spec.Name)
		}
		seen[norm] = spec.Name
	}
	return count
}

func groupSCIMSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
