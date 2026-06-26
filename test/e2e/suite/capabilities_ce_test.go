//go:build e2e && !enterprise

// capabilities_ce_test.go contains end-to-end tests for the MCP server
// capabilities not already covered by elicitation_test.go: progress and
// completions.
//
// Each test spins up its own dedicated in-memory MCP server-client pair
// configured exactly like the production server (see cmd/server/main.go
// createServer): tools, resources, prompts, completion handler, and the
// progress notification handler. This is the only way to exercise these
// capabilities end-to-end because the shared sessions in setup_test.go do
// not register the per-test handlers (ProgressNotificationHandler) these
// capabilities require on the client side.
package suite

import (
	"context"
	"encoding/base64"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/completions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// capabilitiesSession bundles a dedicated server-client pair built exactly
// like the production server, plus the channels and counters that capture
// the protocol notifications under test.
type capabilitiesSession struct {
	client    *mcp.ClientSession
	mcpClient *mcp.Client
	progress  chan mcp.ProgressNotificationParams
}

// newCapabilitiesSession builds an in-memory MCP server matching the
// production configuration in cmd/server/main.go createServer (tools,
// resources, prompts, completions, progress) and pairs it with a client
// wired with the supplied client-side capability handlers.
//
// When withProgress is set, a ProgressNotificationHandler pushes
// notifications onto the returned progress channel.
//
// The session is closed in t.Cleanup.
func newCapabilitiesSession(t *testing.T, client *gitlabclient.Client, enterprise, withProgress bool) *capabilitiesSession {
	t.Helper()

	completionHandler := completions.NewHandler(client)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-capabilities",
		Version: "test",
	}, &mcp.ServerOptions{
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return completionHandler.Complete(ctx, req)
		},
	})

	tools.RegisterAll(server, client, edition.TierForEnterprise(enterprise))
	resources.Register(server, client)
	resources.RegisterWorkflowGuides(server)
	prompts.Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	go func() {
		if err := server.Run(serverCtx, st); err != nil && serverCtx.Err() == nil {
			log.Printf("e2e capabilities server stopped: %v", err)
		}
	}()

	cs := &capabilitiesSession{}
	clientOpts := &mcp.ClientOptions{}
	if withProgress {
		cs.progress = make(chan mcp.ProgressNotificationParams, 64)
		clientOpts.ProgressNotificationHandler = func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			select {
			case cs.progress <- *req.Params:
			default:
			}
		}
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-capabilities-client",
		Version: "test",
	}, clientOpts)

	session, err := mcpClient.Connect(context.Background(), ct, nil)
	if err != nil {
		serverCancel()
		t.Fatalf("connect capabilities client: %v", err)
	}
	cs.client = session
	cs.mcpClient = mcpClient

	t.Cleanup(func() {
		_ = session.Close()
		serverCancel()
	})

	return cs
}

// drainProgress collects progress notifications until at least minNotifs
// have been seen or timeout elapses.
func drainProgress(ch <-chan mcp.ProgressNotificationParams, minNotifs int, timeout time.Duration) []mcp.ProgressNotificationParams {
	var notifs []mcp.ProgressNotificationParams
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case n := <-ch:
			notifs = append(notifs, n)
			if len(notifs) >= minNotifs {
				return notifs
			}
		case <-deadline.C:
			return notifs
		}
	}
}

// ---------------------------------------------------------------------------
// TestCapability_Progress
// ---------------------------------------------------------------------------.

// TestCapability_Progress verifies that the server emits MCP progress
// notifications when a tool handler uses progress.Tracker. The client
// supplies a progressToken in the call-tool request meta, then asserts
// at least one progress notification carrying that token arrives.
//
// gitlab_project_upload is exercised because uploads.Upload wraps the
// reader in a ProgressReader that fires at least one notification on EOF
// regardless of payload size. The upload payload is intentionally small
// (~1 KB) to keep the test fast.
func TestCapability_Progress(t *testing.T) {
	t.Parallel()
	if sess.glClient == nil || sess.individual == nil {
		t.Skip("gitlab client or individual session not configured")
	}

	cs := newCapabilitiesSession(t, sess.glClient, sess.enterprise, true)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Reuse the shared session to create a project — its t.Cleanup deletes it.
	proj := createProject(ctx, t, sess.individual)

	const token = "e2e-progress-token-1"
	payload := strings.Repeat("E2E progress payload bytes\n", 64) // ~1.7 KB
	content := base64.StdEncoding.EncodeToString([]byte(payload))

	args := map[string]any{
		"project_id":     proj.pidOf().String(),
		"filename":       "e2e-progress.txt",
		"content_base64": content,
	}

	if _, err := cs.client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_project_upload",
		Arguments: args,
		Meta:      mcp.Meta{"progressToken": token},
	}); err != nil {
		t.Fatalf("call gitlab_project_upload: %v", err)
	}

	notifs := drainProgress(cs.progress, 1, 5*time.Second)
	if len(notifs) == 0 {
		t.Fatal("expected at least one progress notification, got none")
	}
	for _, n := range notifs {
		if n.ProgressToken != token {
			t.Errorf("notification token = %v, want %q", n.ProgressToken, token)
		}
	}
	// At least one notification must carry a non-zero progress value to prove
	// the tracker actually reported byte counts (not just an empty signal).
	var sawNonZero bool
	for _, n := range notifs {
		if n.Progress > 0 {
			sawNonZero = true
			break
		}
	}
	if !sawNonZero {
		t.Errorf("expected at least one notification with progress>0; got %+v", notifs)
	}
}

// ---------------------------------------------------------------------------
// TestCapability_Completions
// ---------------------------------------------------------------------------.

// TestCapability_Completions verifies the MCP completions capability:
// the client sends completion/complete requests for prompt arguments and
// resource template parameters, and the server's CompletionHandler
// returns matching values from the GitLab API.
//
// We test two reference types:
//  1. ref/prompt with argument "project_id" — backed by GitLab project list.
//  2. ref/resource with argument "project_id" — same backing source via the
//     resource template router.
//
// Both must return at least one suggestion that looks like a numeric
// project ID (canonical form per MCP 2025-11-25 spec).
func TestCapability_Completions(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.glClient == nil {
		t.Skip("gitlab client not configured")
	}

	cs := newCapabilitiesSession(t, sess.glClient, sess.enterprise, false)

	// Ensure at least one project exists so completions have something to
	// return. Use the shared session to avoid duplicating cleanup logic.
	setupCtx, setupCancel := e2eTimeoutContext(30*time.Second, 180*time.Second)
	defer setupCancel()
	_ = createProject(setupCtx, t, sess.individual)

	// Each subtest uses its own context. Subtests run with t.Parallel() so
	// the parent function returns before they execute; sharing a parent
	// context with `defer cancel()` would cancel it prematurely.
	t.Run("PromptArg_ProjectID", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := e2eTimeoutContext(30*time.Second, 90*time.Second)
		defer cancel()
		out, err := cs.client.Complete(ctx, &mcp.CompleteParams{
			Ref: &mcp.CompleteReference{
				Type: "ref/prompt",
				Name: "summarize_mr_changes",
			},
			Argument: mcp.CompleteParamsArgument{
				Name:  "project_id",
				Value: "",
			},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
		if len(out.Completion.Values) == 0 {
			t.Fatal("expected at least one completion value, got none")
		}
		for i, v := range out.Completion.Values {
			if strings.TrimSpace(v) == "" {
				t.Errorf("value[%d] is empty", i)
			}
		}
	})

	t.Run("ResourceArg_ProjectID", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := e2eTimeoutContext(30*time.Second, 90*time.Second)
		defer cancel()
		out, err := cs.client.Complete(ctx, &mcp.CompleteParams{
			Ref: &mcp.CompleteReference{
				Type: "ref/resource",
				URI:  "gitlab://project/{project_id}",
			},
			Argument: mcp.CompleteParamsArgument{
				Name:  "project_id",
				Value: "",
			},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
		if len(out.Completion.Values) == 0 {
			t.Fatal("expected at least one completion value, got none")
		}
	})

	t.Run("UnknownArgReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := e2eTimeoutContext(30*time.Second, 90*time.Second)
		defer cancel()
		out, err := cs.client.Complete(ctx, &mcp.CompleteParams{
			Ref: &mcp.CompleteReference{
				Type: "ref/prompt",
				Name: "summarize_mr_changes",
			},
			Argument: mcp.CompleteParamsArgument{
				Name:  "totally_unknown_argument",
				Value: "",
			},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
		if len(out.Completion.Values) != 0 {
			t.Errorf("expected empty values for unknown argument, got %v", out.Completion.Values)
		}
	})
}
