// action_specs_test.go contains canonical-route tests for runner controller actions.
package runnercontrollers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// runnerGroup is the catalog group these specs are aggregated into by
// internal/tools/runners/action_specs.go, so a spec named controller_get is
// published as runner.controller_get.
const runnerGroup = "runner."

// TestActionSpecs_CallAllRoutes exercises every runner controller tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_runner_controller_list", map[string]any{}},
		{"gitlab_runner_controller_get", map[string]any{"controller_id": 1}},
		{"gitlab_runner_controller_create", map[string]any{"description": "new"}},
		{"gitlab_runner_controller_update", map[string]any{"controller_id": 1, "description": "updated"}},
		{"gitlab_runner_controller_delete", map[string]any{"controller_id": 1}},
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

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerActionHandler())))

	result, err := byTool["gitlab_runner_controller_delete"].Route.Handler(t.Context(), map[string]any{"controller_id": 1})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_runner_controller_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_runner_controller_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted runner controller." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteAPIError verifies the delete route propagates backend failures.
func TestActionSpecs_DeleteAPIError(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	_, err := byTool["gitlab_runner_controller_delete"].Route.Handler(t.Context(), map[string]any{"controller_id": 1})
	if err == nil {
		t.Fatal("expected route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_runner_controller_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test runner controller destructive confirmation.",
		Icons:       toolutil.IconRunner,
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
		Name:      "gitlab_runner_controller_delete",
		Arguments: map[string]any{"controller_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestActionSpecs_EveryToolCarriesItsOwnDiscoveryMetadata reads the published
// surface rather than the table behind it: a spec whose usage, aliases, related
// actions or individual description went missing is one a model discovers by a
// generic line, and the discovery audit counts such a line as present.
func TestActionSpecs_EveryToolCarriesItsOwnDiscoveryMetadata(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerActionHandler())))

	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			if spec.Usage == "" || !strings.Contains(spec.Usage, "runner controller") {
				t.Errorf("%s usage is not about runner controllers: %q", tool, spec.Usage)
			}
			if len(spec.Aliases) < 2 || spec.Aliases[0] != tool {
				t.Errorf("%s aliases = %v, want the tool name plus its own phrasings", tool, spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s names no related actions", tool)
			}
			if spec.IndividualTool.Description == "" {
				t.Errorf("%s has no individual-tool description", tool)
			}
		})
	}
}

// TestActionSpecs_RelatedActionsNameRegisteredActions holds every cross-link to
// the canonical action IDs this package registers. A misspelled one is a dead
// end a model follows once and abandons.
//
// The canonical IDs rather than the individual tool names: the discovery tools
// that publish this field hand out canonical IDs, so a tool name written here
// would be a cross-link a model cannot look up in any listing it is shown.
func TestActionSpecs_RelatedActionsNameRegisteredActions(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerActionHandler())))
	actions := make(map[string]string, len(byTool))
	for tool, spec := range byTool {
		actions["runner."+spec.Name] = tool
	}

	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				named, ok := actions[related]
				if !ok {
					t.Errorf("%s names related action %q, which this package does not register", tool, related)
					continue
				}
				if named == tool {
					t.Errorf("%s names itself as a related action", tool)
				}
			}
		})
	}
}

// TestMarkdownHints_NameActionsTheRunnerGroupRegisters resolves every canonical
// ID the three cards point a model at against the specs the runner group really
// publishes, this package's and the two sibling packages' aggregated beside it.
// Nothing else in the repository checks these strings, and a misspelled one
// answers "unknown action" the moment a model follows the hint.
func TestMarkdownHints_NameActionsTheRunnerGroupRegisters(t *testing.T) {
	client := testutil.NewTestClient(t, nopHandler())
	specs := ActionSpecs(client)
	specs = append(specs, runnercontrollertokens.ActionSpecs(client)...)
	specs = append(specs, runnercontrollerscopes.ActionSpecs(client)...)

	published := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		published[runnerGroup+spec.Name] = struct{}{}
	}

	hints := []string{
		actionControllerGet,
		actionControllerList,
		actionControllerUpdate,
		actionControllerTokenList,
		actionControllerScopeList,
	}
	for _, id := range hints {
		t.Run(id, func(t *testing.T) {
			if _, ok := published[id]; !ok {
				t.Errorf("%s is not an action the runner group registers", id)
			}
		})
	}
}

func runnerControllerActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/runner_controllers":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+sampleControllerJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/runner_controllers/1":
			testutil.RespondJSON(w, http.StatusOK, sampleDetailsJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/runner_controllers":
			testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/runner_controllers/1":
			testutil.RespondJSON(w, http.StatusOK, sampleControllerJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/runner_controllers/1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func runnerControllerSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
