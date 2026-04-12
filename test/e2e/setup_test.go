//go:build e2e

// Package e2e contains end-to-end tests that exercise the MCP server tools
// against a real GitLab instance via the in-process MCP client-server loop.
// Run with: go test -v -tags e2e -timeout 120s ./test/e2e/.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Format strings and test file constants used across E2E test helpers.
const (
	fmtCallErr       = "call %s: %w"
	testFileMainGo   = "main.go"
	msgCommitIDEmpty = "commit ID should not be empty"
)

// testState holds shared state across sequential test steps.
type testState struct {
	glClient      *gitlabclient.Client
	session       *mcp.ClientSession
	metaSession   *mcp.ClientSession // meta-tools session
	projectID     int64
	projectPath   string
	mrIID         int64
	noteID        int64
	discussionID  string
	releaseLinkID int64
	lastCommitSHA string // SHA from most recent commit (for commit get/diff tests)
	issueIID      int64  // issue IID for issue lifecycle tests
	issueNoteID   int64  // issue note ID for issue note tests
	groupID       int64  // group ID discovered via group_list (0 if none)
	groupPath     string // group full path discovered via group_list
	packageID     int64  // package ID for package lifecycle tests
	packageFileID int64  // package file ID for package file tests
}

// state is the shared [testState] instance populated by [TestMain] and
// used by all sequential test steps across both workflow files.
var state testState

// TestMain initializes the E2E test environment by loading configuration,
// creating a GitLab client, verifying connectivity, and starting two
// in-process MCP server/client pairs: one for individual tools and one
// for meta-tools. It populates the global [state] and tears down servers
// after all tests complete.
func TestMain(m *testing.M) {
	// Load .env from project root (go test CWD is the package directory).
	_ = godotenv.Load("../../.env")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("e2e: load config: %v", err)
	}

	glClient, err := gitlabclient.NewClient(cfg)
	if err != nil {
		log.Fatalf("e2e: create GitLab client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = glClient.Ping(ctx); err != nil {
		log.Fatalf("e2e: gitlab ping failed: %v", err)
	}

	// Create MCP server with all individual tools registered.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e",
		Version: "test",
	}, nil)
	tools.RegisterAll(server, glClient)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := server.Run(serverCtx, serverTransport); srvErr != nil && serverCtx.Err() == nil {
			log.Printf("e2e: server stopped unexpectedly: %v", srvErr)
		}
	}()

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "test",
	}, nil)
	session, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		serverCancel()
		log.Fatalf("e2e: connect MCP client: %v", err)
	}

	// Create a second MCP server with meta-tools for meta-tool E2E tests.
	metaServer := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-meta",
		Version: "test",
	}, nil)
	tools.RegisterAllMeta(metaServer, glClient, true)

	metaServerTransport, metaClientTransport := mcp.NewInMemoryTransports()

	metaServerCtx, metaServerCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := metaServer.Run(metaServerCtx, metaServerTransport); srvErr != nil && metaServerCtx.Err() == nil {
			log.Printf("e2e: meta server stopped unexpectedly: %v", srvErr)
		}
	}()

	metaClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-meta-client",
		Version: "test",
	}, nil)
	metaSession, err := metaClient.Connect(context.Background(), metaClientTransport, nil)
	if err != nil {
		serverCancel()
		metaServerCancel()
		log.Fatalf("e2e: connect meta MCP client: %v", err)
	}

	state = testState{
		glClient:    glClient,
		session:     session,
		metaSession: metaSession,
	}

	code := m.Run()

	_ = session.Close()
	serverCancel()
	_ = metaSession.Close()
	metaServerCancel()
	os.Exit(code)
}

// uniqueName generates a timestamped name to avoid collisions.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
}

// ---------------------------------------------------------------------------
// MCP call helpers
// ---------------------------------------------------------------------------.

// callTool invokes an MCP tool via the client session and unmarshals the
// structured result into the output type O.
func callTool[O any](ctx context.Context, name string, input any) (O, error) {
	var zero O
	result, err := state.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return zero, fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return zero, extractToolError(name, result)
	}

	// Prefer structured content (typed ToolHandlerFor output).
	if result.StructuredContent != nil {
		var data []byte
		data, err = json.Marshal(result.StructuredContent)
		if err != nil {
			return zero, fmt.Errorf("marshal structured content: %w", err)
		}
		var out O
		err = json.Unmarshal(data, &out)
		if err != nil {
			return zero, fmt.Errorf("unmarshal %s result to %T: %w", name, out, err)
		}
		return out, nil
	}

	// Fallback: extract JSON from the first text content block.
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			var out O
			err = json.Unmarshal([]byte(tc.Text), &out)
			if err != nil {
				return zero, fmt.Errorf("unmarshal %s text to %T: %w", name, out, err)
			}
			return out, nil
		}
	}

	return zero, fmt.Errorf("tool %s: no extractable output", name)
}

// callToolVoid invokes an MCP tool that returns no structured output (e.g. delete, unapprove).
func callToolVoid(ctx context.Context, name string, input any) error {
	result, err := state.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return extractToolError(name, result)
	}
	return nil
}

// extractToolError reads the first text content block from a failed
// [mcp.CallToolResult] and returns it as a formatted error.
func extractToolError(name string, result *mcp.CallToolResult) error {
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			return fmt.Errorf("tool %s: %s", name, tc.Text)
		}
	}
	return fmt.Errorf("tool %s returned error", name)
}

// ---------------------------------------------------------------------------
// Meta-tool call helpers (use metaSession)
// ---------------------------------------------------------------------------.

// callMeta invokes a meta-tool via the meta session and unmarshals the output.
func callMeta[O any](ctx context.Context, metaTool, action string, params map[string]any) (O, error) {
	return callToolOn[O](ctx, state.metaSession, metaTool, map[string]any{
		"action": action,
		"params": params,
	})
}

// callMetaVoid invokes a meta-tool action that returns no structured output.
func callMetaVoid(ctx context.Context, metaTool, action string, params map[string]any) error {
	return callToolVoidOn(ctx, state.metaSession, metaTool, map[string]any{
		"action": action,
		"params": params,
	})
}

// callToolOn is a session-parameterized version of callTool.
func callToolOn[O any](ctx context.Context, session *mcp.ClientSession, name string, input any) (O, error) {
	var zero O
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return zero, fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return zero, extractToolError(name, result)
	}
	if result.StructuredContent != nil {
		var data []byte
		data, err = json.Marshal(result.StructuredContent)
		if err != nil {
			return zero, fmt.Errorf("marshal structured content: %w", err)
		}
		var out O
		err = json.Unmarshal(data, &out)
		if err != nil {
			return zero, fmt.Errorf("unmarshal %s result to %T: %w", name, out, err)
		}
		return out, nil
	}
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			var out O
			err = json.Unmarshal([]byte(tc.Text), &out)
			if err != nil {
				return zero, fmt.Errorf("unmarshal %s text to %T: %w", name, out, err)
			}
			return out, nil
		}
	}
	return zero, fmt.Errorf("tool %s: no extractable output", name)
}

// callToolVoidOn is a session-parameterized version of callToolVoid.
func callToolVoidOn(ctx context.Context, session *mcp.ClientSession, name string, input any) error {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return extractToolError(name, result)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Wait helpers
// ---------------------------------------------------------------------------.

// waitForBranchProtection polls GitLab until the given branch appears as
// protected. GitLab applies default branch protection asynchronously after
// project creation, so this helper prevents a race condition where
// UnprotectBranch returns 404 because protection hasn't been applied yet.
func waitForBranchProtection(ctx context.Context, t *testing.T, pid int, branch string) {
	t.Helper()
	for range 15 {
		_, resp, err := state.glClient.GL().ProtectedBranches.GetProtectedBranch(pid, branch)
		if err == nil {
			t.Logf("Branch %q is protected — ready to unprotect", branch)
			return
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			select {
			case <-ctx.Done():
				t.Fatalf("context canceled waiting for branch protection %q: %v", branch, ctx.Err())
			case <-time.After(1 * time.Second):
			}
			continue
		}
		requireNoError(t, err, "get protected branch "+branch)
	}
	t.Logf("Branch %q not protected after 15s — proceeding anyway", branch)
}

// ---------------------------------------------------------------------------
// Test assertion helpers
// ---------------------------------------------------------------------------.

// requireNoError calls t.Fatalf if err is non-nil, including the action
// label in the failure message.
func requireNoError(t *testing.T, err error, action string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed: %v", action, err)
	}
}

// requireTrue calls t.Fatalf with the given format string if condition
// is false.
func requireTrue(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Fatalf(format, args...)
	}
}
