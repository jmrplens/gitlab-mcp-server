// action_specs_test.go contains canonical-route tests for project mirror actions.
package projectmirrors

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every project mirror tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectMirrorActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_project_mirrors", map[string]any{"project_id": testProjectID}},
		{"gitlab_get_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_get_project_mirror_public_key", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_add_project_mirror", map[string]any{"project_id": testProjectID, "url": "https://example.com/repo.git"}},
		{"gitlab_edit_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_delete_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_force_push_mirror_update", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
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

// TestActionSpecs_DeleteAndForcePushOutputs verifies legacy success messages remain stable.
func TestActionSpecs_DeleteAndForcePushOutputs(t *testing.T) {
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectMirrorActionHandler())))

	deleteResult, err := byTool["gitlab_delete_project_mirror"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "mirror_id": 42})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_project_mirror) error: %v", err)
	}
	deleteOut, ok := deleteResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_project_mirror) returned %T, want toolutil.DeleteOutput", deleteResult)
	}
	if deleteOut.Message != "Successfully deleted mirror 42 from project myproject." {
		t.Fatalf("delete message = %q", deleteOut.Message)
	}

	forcePushResult, err := byTool["gitlab_force_push_mirror_update"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "mirror_id": 42})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_force_push_mirror_update) error: %v", err)
	}
	forcePushOut, ok := forcePushResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_force_push_mirror_update) returned %T, want toolutil.DeleteOutput", forcePushResult)
	}
	if forcePushOut.Message != "Force push update triggered for mirror 42 in project myproject" {
		t.Fatalf("force-push message = %q", forcePushOut.Message)
	}
}

// TestActionSpecs_ErrorPaths verifies destructive routes propagate backend failures.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_delete_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_force_push_mirror_update", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
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

// TestActionSpecs_MirrorAddRequiresConfirmation pins the classification that
// turns the confirmation guard on for push-mirror creation.
//
// Creating a push mirror hands the host named in url a continuous copy of the
// whole repository, and the url is a tool parameter, so a model that has read
// an issue body asking for a "backup mirror" could exfiltrate the repository
// with one unconfirmed call. Every surface runs the guard off Route.Destructive
// and nothing else, so this classification is the whole fix; that it reaches a
// caller is [TestCatalogSurface_MirrorAddWithoutConfirm_IsRefused].
func TestActionSpecs_MirrorAddRequiresConfirmation(t *testing.T) {
	spec := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectMirrorActionHandler())))["gitlab_add_project_mirror"]

	t.Run("the spec and its route agree the action is destructive", func(t *testing.T) {
		if !spec.Destructive || !spec.Route.Destructive {
			t.Errorf("spec.Destructive = %t, Route.Destructive = %t, want both true", spec.Destructive, spec.Route.Destructive)
		}
		if err := spec.Validate(); err != nil {
			t.Errorf("spec.Validate() error: %v", err)
		}
	})

	t.Run("repeating the call is not advertised as safe", func(t *testing.T) {
		// Two calls with one url leave two mirror rows, so idempotentHint must
		// stay false: it is the bit that tells a model a retry costs nothing.
		if spec.Idempotent {
			t.Error("spec.Idempotent = true, want false: creating a mirror twice creates two mirrors")
		}
	})

	t.Run("the served schema offers confirm", func(t *testing.T) {
		schema := toolutil.MetaActionSchema(spec.Route)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["confirm"]; !ok {
			t.Errorf("served input schema has no confirm property, properties = %v", properties)
		}
		if destructive, _ := schema["x_destructive"].(bool); !destructive {
			t.Error("served input schema does not carry x_destructive")
		}
	})
}

// TestCatalogSurface_MirrorAddWithoutConfirm_IsRefused drives push-mirror
// creation over a real MCP session and asserts the guard both refuses an
// unconfirmed call and lets a confirmed one through.
//
// The client registers no elicitation handler, which is the shape that matters:
// it cannot be asked, so the guard has to refuse rather than proceed silently.
// The request counter is what makes the refusal meaningful, since a refusal
// that still issued the POST would have exfiltrated the repository already.
//
// The env var is pinned because the guard's first branch is YOLO mode: an
// inherited GITLAB_MCP_YOLO_MODE or AUTOPILOT would skip confirmation and leave
// the refusal row failing on a machine that has it set.
func TestCatalogSurface_MirrorAddWithoutConfirm_IsRefused(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "false")

	var created atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathMirrors {
			created.Add(1)
			testutil.RespondJSON(w, http.StatusCreated, mirrorJSON)
			return
		}
		http.NotFound(w, r)
	})
	spec := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))["gitlab_add_project_mirror"]

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{
		Description: "Test push mirror creation confirmation.",
		Icons:       toolutil.IconInfra,
	})
	session := connectProjectMirrorSession(t, server)

	callCases := []struct {
		name        string
		args        map[string]any
		wantRefused bool
	}{
		{
			name:        "without confirm the call never reaches GitLab",
			args:        map[string]any{"project_id": testProjectID, "url": "https://attacker.example/repo.git"},
			wantRefused: true,
		},
		{
			name:        "with confirm true the mirror is created",
			args:        map[string]any{"project_id": testProjectID, "url": "https://example.com/repo.git", "confirm": true},
			wantRefused: false,
		},
	}
	for _, tc := range callCases {
		t.Run(tc.name, func(t *testing.T) {
			before := created.Load()

			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "gitlab_add_project_mirror", Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}

			assertMirrorAddOutcome(t, result, created.Load()-before, tc.wantRefused)
		})
	}
}

// assertMirrorAddOutcome checks one push-mirror creation call: a refusal must
// name confirm=true and must have sent no request, and an accepted call must
// have sent exactly one.
func assertMirrorAddOutcome(t *testing.T, result *mcp.CallToolResult, sent int64, wantRefused bool) {
	t.Helper()
	if wantRefused {
		if !result.IsError {
			t.Errorf("IsError = false, want a refusal for a call carrying no confirm")
		}
		if text := resultText(result); !strings.Contains(text, "confirm=true") {
			t.Errorf("refusal text = %q, want it to name confirm=true", text)
		}
		if sent != 0 {
			t.Errorf("POST requests = %d, want 0: an unconfirmed call must not reach GitLab", sent)
		}
		return
	}
	if result.IsError {
		t.Errorf("IsError = true for a confirmed call, text = %q", resultText(result))
	}
	if sent != 1 {
		t.Errorf("POST requests = %d, want 1", sent)
	}
}

// connectProjectMirrorSession connects an in-memory client with no elicitation
// handler to server and returns the client session, closed on cleanup.
func connectProjectMirrorSession(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})
	return session
}

// resultText joins the text content of a tool result, so an assertion can read
// what the caller was told.
func resultText(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_project_mirror"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test project mirror destructive confirmation.",
		Icons:       toolutil.IconInfra,
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
		Name:      "gitlab_delete_project_mirror",
		Arguments: map[string]any{"project_id": testProjectID, "mirror_id": 42},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func projectMirrorActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathMirrors:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+mirrorJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathMirrorKey42:
			testutil.RespondJSON(w, http.StatusOK, publicKeyJSON)
		case r.Method == http.MethodGet && path == pathMirror42:
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
		case r.Method == http.MethodPost && path == pathMirrors:
			testutil.RespondJSON(w, http.StatusCreated, mirrorJSON)
		case r.Method == http.MethodPut && path == pathMirror42:
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
		case r.Method == http.MethodDelete && path == pathMirror42:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == pathMirrorSync42:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func projectMirrorSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
