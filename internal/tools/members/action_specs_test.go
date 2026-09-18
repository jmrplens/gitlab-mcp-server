// action_specs_test.go contains canonical-route tests for project member actions.
package members

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const actionSpecMemberJSON = `{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice"}`

// TestActionSpecs_CallAllRoutes exercises every project member tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := memberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, memberActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "list", tool: "gitlab_project_members_list", args: map[string]any{"project_id": "42"}},
		{name: "get", tool: "gitlab_project_member_get", args: map[string]any{"project_id": "42", "user_id": 10}},
		{name: "get_inherited", tool: "gitlab_project_member_get_inherited", args: map[string]any{"project_id": "42", "user_id": 10}},
		{name: "add", tool: "gitlab_project_member_add", args: map[string]any{"project_id": "42", "user_id": 10, "access_level": 30}},
		{name: "edit", tool: "gitlab_project_member_edit", args: map[string]any{"project_id": "42", "user_id": 10, "access_level": 30}},
		{name: "delete", tool: "gitlab_project_member_delete", args: map[string]any{"project_id": "42", "user_id": 10}},
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

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := memberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, memberActionHandler())))

	result, err := byTool["gitlab_project_member_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "user_id": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_project_member_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_project_member_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted project member." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_MutationErrors verifies mutating and lookup route failures propagate.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := memberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_project_member_add", map[string]any{"project_id": "42", "user_id": 1, "access_level": 30}},
		{"gitlab_project_member_edit", map[string]any{"project_id": "42", "user_id": 1, "access_level": 40}},
		{"gitlab_project_member_delete", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_project_member_get", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_project_member_get_inherited", map[string]any{"project_id": "42", "user_id": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := memberSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_project_member_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test project member destructive confirmation.",
		Icons:       toolutil.IconUser,
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
		Name:      "gitlab_project_member_delete",
		Arguments: map[string]any{"project_id": "42", "user_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMemberOptions_EveryRegisteredTool_CarriesMetadataOfItsOwn verifies that
// each of the six tool names ActionSpecs registers is answered by a branch of
// memberOptions with its own usage, its own aliases and an individual-tool
// description, rather than falling through to the package defaults.
//
// Why it matters: memberOptions decides that per tool name, and a name with no
// branch is not an error — it yields a spec whose usage is the placeholder and
// whose individual description is empty. The surface is still registered and
// still routes, so a tool added to ActionSpecs and forgotten here ships with no
// description at all, which is the one piece of metadata a model reads to
// decide whether the tool is the one it wants.
func TestMemberOptions_EveryRegisteredTool_CarriesMetadataOfItsOwn(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, memberActionHandler()))
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs()) = %d, want 6", len(specs))
	}

	defaults := memberOptions("gitlab_project_member_not_a_tool")
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			opts := memberOptions(spec.IndividualTool.Name)
			if opts.Usage == defaults.Usage {
				t.Errorf("usage is still the package placeholder %q", opts.Usage)
			}
			if opts.IndividualTool.Description == "" {
				t.Error("IndividualTool.Description is empty")
			}
			if len(opts.Aliases) < 2 {
				t.Errorf("Aliases = %v, want the tool name plus natural-language aliases", opts.Aliases)
			}
		})
	}
}

// TestMemberOptions_AnUnknownToolName_KeepsThePackageDefaults verifies what a
// name no branch matches is answered with: the shared tags, owner package and
// related actions, its own name as the only alias, and no individual-tool
// description.
//
// Why it matters: this is the fall-through the test above is measured against,
// and it is deliberately harmless rather than a panic or a half-filled spec —
// the defaults are a complete, if generic, spec. Stating it here is what makes
// "the description is empty" a fact about the branch that is missing rather
// than about whichever branch happened to run last.
func TestMemberOptions_AnUnknownToolName_KeepsThePackageDefaults(t *testing.T) {
	const name = "gitlab_project_member_not_a_tool"
	opts := memberOptions(name)

	if opts.Usage != "Use to execute members domain action." {
		t.Errorf("Usage = %q, want the package default", opts.Usage)
	}
	if opts.IndividualTool.Description != "" {
		t.Errorf("IndividualTool.Description = %q, want empty for a name no branch matches", opts.IndividualTool.Description)
	}
	if opts.IndividualTool.Name != name {
		t.Errorf("IndividualTool.Name = %q, want %q", opts.IndividualTool.Name, name)
	}
	if len(opts.Aliases) != 1 || opts.Aliases[0] != name {
		t.Errorf("Aliases = %v, want only %q", opts.Aliases, name)
	}
	if opts.ParameterGuidance != nil {
		t.Errorf("ParameterGuidance = %v, want none", opts.ParameterGuidance)
	}
	if opts.OwnerPackage != "members" {
		t.Errorf("OwnerPackage = %q, want %q", opts.OwnerPackage, "members")
	}
}

func memberActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/members/all/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecMemberJSON)
	})
	handler.HandleFunc("GET /api/v4/projects/42/members/all", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecMemberJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecMemberJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecMemberJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecMemberJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func memberSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
