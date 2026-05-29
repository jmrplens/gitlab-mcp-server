// action_specs_test.go contains catalog and direct route regression tests for group actions.
package groups

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCatalogSurface_ConfirmDeclined covers the confirmation early-return
// branches in group delete and webhook delete handlers when the user declines.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	byTool := groupSpecsByTool(t, ActionSpecs(client))
	for _, toolName := range []string{"gitlab_group_delete", "gitlab_group_hook_delete"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test group destructive confirmation.",
			Icons:       toolutil.IconGroup,
		})
	}

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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_delete", map[string]any{"group_id": "42"}},
		{"gitlab_group_hook_delete", map[string]any{"group_id": "42", "hook_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool error: %v", callErr)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

// TestActionSpecs_PrimaryMetadata verifies richer metadata on the core group actions.
func TestActionSpecs_PrimaryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := groupSpecsByTool(t, ActionSpecs(client))

	getSpec := byTool["gitlab_group_get"]
	if !strings.Contains(getSpec.Usage, "group_id") {
		t.Fatalf("get Usage = %q, want group_id guidance", getSpec.Usage)
	}
	if guidance := getSpec.ParameterGuidance["group_id"]; guidance.SemanticRole != "scope_group" {
		t.Fatalf("group get guidance = %+v, want scope_group", guidance)
	}
	if !slices.Contains(getSpec.RelatedActions, "group.update") {
		t.Fatalf("group get RelatedActions = %v, want group.update", getSpec.RelatedActions)
	}

	listSpec := byTool["gitlab_group_list"]
	if !slices.Contains(listSpec.Aliases, "list groups") {
		t.Fatalf("group list Aliases = %v, want list groups", listSpec.Aliases)
	}
	if !strings.Contains(listSpec.Usage, "pagination") {
		t.Fatalf("group list Usage = %q, want pagination guidance", listSpec.Usage)
	}

	createSpec := byTool["gitlab_group_create"]
	if guidance := createSpec.ParameterGuidance["path"]; guidance.SemanticRole != "group_path_segment" {
		t.Fatalf("group create path guidance = %+v, want group_path_segment", guidance)
	}
	if !strings.Contains(createSpec.IndividualTool.Description, "Returns:") || !strings.Contains(createSpec.IndividualTool.Description, "See also:") {
		t.Fatalf("group create description = %q, want Returns/See also", createSpec.IndividualTool.Description)
	}
}

// TestActionSpecs_GetNotFound covers the not-found branch in the gitlab_group_get route.
func TestActionSpecs_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_group_get"].Route.Handler(t.Context(), map[string]any{"group_id": "999"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if _, ok := result.(groupNotFoundOutput); !ok {
		t.Fatalf("result type = %T, want groupNotFoundOutput", result)
	}
}

// TestFormatGroupNotFound verifies not-found result formatting for group lookups.
func TestFormatGroupNotFound(t *testing.T) {
	result := formatGroupNotFound(groupNotFoundOutput{Identifier: "my%2Fgroup"})
	if result == nil || !result.IsError {
		t.Fatalf("formatGroupNotFound() = %+v, want error result", result)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(content.Text, "Group") || !strings.Contains(content.Text, "my%2Fgroup") {
		t.Fatalf("content = %q, want group identifier", content.Text)
	}
}

// TestMemberToOutput_OptionalFields covers the optional field branches
// (CreatedAt, ExpiresAt, GroupSAMLIdentity, MemberRole) in MemberToOutput.
func TestMemberToOutput_OptionalFields(t *testing.T) {
	now := time.Now()
	expires := gl.ISOTime(now)
	m := &gl.GroupMember{
		ID:          1,
		Username:    "user",
		AccessLevel: gl.DeveloperPermissions,
		CreatedAt:   &now,
		ExpiresAt:   &expires,
		GroupSAMLIdentity: &gl.GroupMemberSAMLIdentity{
			Provider: "saml-provider",
		},
		MemberRole: &gl.MemberRole{
			Name: "custom-role",
		},
	}
	out := MemberToOutput(m)
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
	if out.GroupSAMLProvider != "saml-provider" {
		t.Errorf("GroupSAMLProvider = %q, want %q", out.GroupSAMLProvider, "saml-provider")
	}
	if out.MemberRoleName != "custom-role" {
		t.Errorf("MemberRoleName = %q, want %q", out.MemberRoleName, "custom-role")
	}
}

// TestActionSpecs_ErrorPaths covers route error propagation for non-404 API errors.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_list", map[string]any{}},
		{"gitlab_group_members_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_create", map[string]any{"name": "x", "path": "x"}},
		{"gitlab_group_update", map[string]any{"group_id": "42"}},
		{"gitlab_group_restore", map[string]any{"group_id": "42"}},
		{"gitlab_group_archive", map[string]any{"group_id": "42"}},
		{"gitlab_group_unarchive", map[string]any{"group_id": "42"}},
		{"gitlab_group_search", map[string]any{"search": "x"}},
		{"gitlab_group_transfer_project", map[string]any{"group_id": "42", "project_id": 1}},
		{"gitlab_group_projects", map[string]any{"group_id": "42"}},
		{"gitlab_group_hook_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_hook_get", map[string]any{"group_id": "42", "hook_id": 1}},
		{"gitlab_group_hook_add", map[string]any{"group_id": "42", "url": "http://x"}},
		{"gitlab_group_hook_edit", map[string]any{"group_id": "42", "hook_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := byTool[tt.name].Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatal("expected route error for server error response")
			}
		})
	}
}
