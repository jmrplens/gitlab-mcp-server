// register_mcp_meta_test.go tests RegisterMCPMeta registration and the
// wrapUpdaterAction generic helper. Tests verify that gitlab_mcp is registered
// with both nil and non-nil updater paths, and that wrapUpdaterAction correctly
// delegates to the wrapped function.

package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegisterMCPMeta_NilUpdater verifies that RegisterMCPMeta registers the
// gitlab_mcp tool with only status and health_check actions when updater is nil.
func TestRegisterMCPMeta_NilUpdater(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMCPMeta(server, client, nil)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "gitlab_mcp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("gitlab_mcp tool not found after RegisterMCPMeta with nil updater")
	}

	// Call status action to verify it works
	callResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_mcp",
		Arguments: map[string]any{"action": "status"},
	})
	if err != nil {
		t.Fatalf("CallTool(status) error: %v", err)
	}
	if callResult.IsError {
		t.Fatal("CallTool(status) returned IsError=true")
	}
}

// TestRegisterMCPMeta_WithUpdater verifies that RegisterMCPMeta registers the
// gitlab_mcp tool with check_update and apply_update actions when updater is
// provided. The test uses a mock source for the updater.
func TestRegisterMCPMeta_WithUpdater(t *testing.T) {
	updater := autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Mode:           autoupdate.ModeCheck,
		Repository:     "group/project",
		CurrentVersion: "1.0.0",
	}, autoupdate.EmptySource{})

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMCPMeta(server, client, updater)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, connectErr := server.Connect(ctx, st, nil); connectErr != nil {
		t.Fatalf("server connect: %v", connectErr)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "gitlab_mcp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("gitlab_mcp tool not found after RegisterMCPMeta with updater")
	}
}

// TestWrapUpdaterAction_Dispatch verifies that wrapUpdaterAction correctly
// unmarshals params and delegates to the wrapped function.
func TestWrapUpdaterAction_Dispatch(t *testing.T) {
	type testInput struct {
		Value string `json:"value"`
	}
	type testOutput struct {
		Result string `json:"result"`
	}

	called := false
	fn := func(_ context.Context, _ *autoupdate.Updater, input testInput) (testOutput, error) {
		called = true
		return testOutput{Result: "got:" + input.Value}, nil
	}

	action := wrapUpdaterAction(nil, fn)
	result, err := action(context.Background(), map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("wrapUpdaterAction() error: %v", err)
	}
	if !called {
		t.Fatal("wrapped function was not called")
	}
	out, ok := result.(testOutput)
	if !ok {
		t.Fatalf("result type = %T, want testOutput", result)
	}
	if out.Result != "got:hello" {
		t.Errorf("result = %q, want %q", out.Result, "got:hello")
	}
}
