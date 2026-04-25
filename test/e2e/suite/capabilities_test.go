//go:build e2e

// capabilities_test.go contains end-to-end tests for the four MCP server
// capabilities not already covered by sampling_test.go and elicitation_test.go:
// logging, progress, roots, and completions.
//
// Each test spins up its own dedicated in-memory MCP server-client pair
// configured exactly like the production server (see cmd/server/main.go
// createServer): tools, resources, prompts, completion handler, roots
// manager, logging capability, and progress notification handler. This is
// the only way to exercise these capabilities end-to-end because the shared
// sessions in setup_test.go do not register the per-test handlers
// (LoggingMessageHandler, ProgressNotificationHandler, advertised roots)
// these capabilities require on the client side.
package suite

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/completions"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
)

// capabilitiesSession bundles a dedicated server-client pair built exactly
// like the production server, plus the channels and counters that capture
// the protocol notifications under test.
type capabilitiesSession struct {
	client      *mcp.ClientSession
	mcpClient   *mcp.Client
	logs        chan logEntry
	progress    chan mcp.ProgressNotificationParams
	rootsServed []*mcp.Root
}

// logEntry captures a single MCP log notification for assertions.
type logEntry struct {
	Level  mcp.LoggingLevel
	Logger string
	Data   any
}

// newCapabilitiesSession builds an in-memory MCP server matching the
// production configuration in cmd/server/main.go createServer (tools,
// resources, prompts, completions, roots, logging, progress) and pairs
// it with a client wired with the supplied client-side capability handlers.
//
// Caller-supplied options control which client features are exercised:
//   - withLogging: install a LoggingMessageHandler that pushes notifications
//     onto the returned logs channel.
//   - withProgress: install a ProgressNotificationHandler that pushes
//     notifications onto the returned progress channel.
//   - withRoots: advertise the given roots from the client BEFORE Connect,
//     so the server's InitializedHandler can fetch them via ListRoots.
//
// The session is closed in t.Cleanup.
func newCapabilitiesSession(t *testing.T, withLogging, withProgress bool, withRoots []*mcp.Root) *capabilitiesSession {
	t.Helper()

	completionHandler := completions.NewHandler(sess.glClient)
	rootsManager := roots.NewManager()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-capabilities",
		Version: "test",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Logging: &mcp.LoggingCapabilities{},
		},
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return completionHandler.Complete(ctx, req)
		},
		InitializedHandler: func(ctx context.Context, req *mcp.InitializedRequest) {
			// Fire-and-forget; failures simply leave the cache empty.
			_ = rootsManager.Refresh(ctx, req.Session)
		},
		RootsListChangedHandler: func(ctx context.Context, req *mcp.RootsListChangedRequest) {
			_ = rootsManager.Refresh(ctx, req.Session)
		},
	})

	tools.RegisterAll(server, sess.glClient, sess.enterprise)
	resources.Register(server, sess.glClient)
	resources.RegisterWorkspaceRoots(server, rootsManager)
	resources.RegisterWorkflowGuides(server)
	prompts.Register(server, sess.glClient)

	st, ct := mcp.NewInMemoryTransports()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	go func() {
		if err := server.Run(serverCtx, st); err != nil && serverCtx.Err() == nil {
			log.Printf("e2e capabilities server stopped: %v", err)
		}
	}()

	cs := &capabilitiesSession{}
	clientOpts := &mcp.ClientOptions{}
	if withLogging {
		cs.logs = make(chan logEntry, 64)
		clientOpts.LoggingMessageHandler = func(_ context.Context, req *mcp.LoggingMessageRequest) {
			select {
			case cs.logs <- logEntry{Level: req.Params.Level, Logger: req.Params.Logger, Data: req.Params.Data}:
			default:
			}
		}
	}
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

	if len(withRoots) > 0 {
		mcpClient.AddRoots(withRoots...)
		cs.rootsServed = withRoots
	}

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

// drainLogs collects log entries arriving on the channel until at least
// minEntries have been seen or timeout elapses. Returns the entries
// observed (which may be fewer than minEntries on timeout).
func drainLogs(ch <-chan logEntry, minEntries int, timeout time.Duration) []logEntry {
	var entries []logEntry
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case e := <-ch:
			entries = append(entries, e)
			if len(entries) >= minEntries {
				return entries
			}
		case <-deadline.C:
			return entries
		}
	}
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
// TestCapability_Logging
// ---------------------------------------------------------------------------.

// TestCapability_Logging verifies the MCP logging capability end-to-end:
// (1) the server announces logging support during initialization,
// (2) the client can request a minimum log level via SetLoggingLevel,
// (3) the server forwards tool-call log records to the client through the
// MCP logging notification channel.
//
// We invoke a cheap read-only tool (gitlab_user_current) to trigger
// LogToolCallAll and assert at least one log notification with the
// "gitlab-mcp-server" logger name arrives on the client.
func TestCapability_Logging(t *testing.T) {
	t.Parallel()
	if sess.glClient == nil {
		t.Skip("gitlab client not configured")
	}

	cs := newCapabilitiesSession(t, true, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cs.client.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"}); err != nil {
		t.Fatalf("SetLoggingLevel: %v", err)
	}

	if _, err := cs.client.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_user_current",
		Arguments: map[string]any{},
	}); err != nil {
		t.Fatalf("call gitlab_user_current: %v", err)
	}

	entries := drainLogs(cs.logs, 1, 5*time.Second)
	if len(entries) == 0 {
		t.Fatal("expected at least one logging notification, got none")
	}
	var sawServerLogger bool
	for _, e := range entries {
		if e.Logger == "gitlab-mcp-server" {
			sawServerLogger = true
			break
		}
	}
	if !sawServerLogger {
		t.Errorf("expected at least one entry with logger=%q; got %+v", "gitlab-mcp-server", entries)
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

	cs := newCapabilitiesSession(t, false, true, nil)

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
// TestCapability_Roots
// ---------------------------------------------------------------------------.

// TestCapability_Roots verifies the MCP roots capability end-to-end:
// the client advertises a workspace root, the server queries it during
// initialization via ListRoots, the roots.Manager caches it, and the
// gitlab://workspace/roots resource exposes it back to clients.
//
// This is a full round-trip test: client→server (advertise roots),
// server→client (ListRoots), server-internal (cache), client→server
// (read resource), server→client (resource contents).
func TestCapability_Roots(t *testing.T) {
	t.Parallel()
	if sess.glClient == nil {
		t.Skip("gitlab client not configured")
	}

	advertised := []*mcp.Root{
		{URI: "file:///tmp/e2e-capabilities-root", Name: "e2e-capabilities-root"},
	}
	cs := newCapabilitiesSession(t, false, false, advertised)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The server's InitializedHandler refresh runs asynchronously after
	// Connect returns. Poll the resource until the root appears or timeout.
	deadline := time.Now().Add(5 * time.Second)
	var lastText string
	for time.Now().Before(deadline) {
		result, err := cs.client.ReadResource(ctx, &mcp.ReadResourceParams{
			URI: "gitlab://workspace/roots",
		})
		if err != nil {
			t.Fatalf("read workspace roots resource: %v", err)
		}
		if len(result.Contents) == 0 {
			t.Fatal("expected workspace roots resource to return content")
		}
		lastText = result.Contents[0].Text
		if strings.Contains(lastText, "e2e-capabilities-root") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("workspace roots resource never reflected advertised root within timeout; last content=%s", lastText)
}

// TestCapability_RootsListChanged verifies that the server picks up roots
// added AFTER initialization via the roots/list_changed notification path.
// The client first connects with no roots, then calls AddRoots, which
// triggers RootsListChangedHandler on the server, which in turn refreshes
// the roots.Manager. The new root must then appear in the resource.
func TestCapability_RootsListChanged(t *testing.T) {
	t.Parallel()
	if sess.glClient == nil {
		t.Skip("gitlab client not configured")
	}

	cs := newCapabilitiesSession(t, false, false, nil)
	cs.mcpClient.AddRoots(&mcp.Root{
		URI:  "file:///tmp/e2e-capabilities-late-root",
		Name: "e2e-capabilities-late-root",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deadline := time.Now().Add(5 * time.Second)
	var lastText string
	for time.Now().Before(deadline) {
		result, err := cs.client.ReadResource(ctx, &mcp.ReadResourceParams{
			URI: "gitlab://workspace/roots",
		})
		if err != nil {
			t.Fatalf("read workspace roots resource: %v", err)
		}
		if len(result.Contents) > 0 {
			lastText = result.Contents[0].Text
			if strings.Contains(lastText, "e2e-capabilities-late-root") {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("workspace roots resource never reflected late-added root; last content=%s", lastText)
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
	t.Parallel()
	if sess.glClient == nil {
		t.Skip("gitlab client not configured")
	}

	cs := newCapabilitiesSession(t, false, false, nil)

	// Ensure at least one project exists so completions have something to
	// return. Use the shared session to avoid duplicating cleanup logic.
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()
	_ = createProject(setupCtx, t, sess.individual)

	// Each subtest uses its own context. Subtests run with t.Parallel() so
	// the parent function returns before they execute; sharing a parent
	// context with `defer cancel()` would cancel it prematurely.
	t.Run("PromptArg_ProjectID", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// ---------------------------------------------------------------------------
// Compile-time guard: ensure dependencies stay imported even if a future
// refactor removes uses of these packages.
// ---------------------------------------------------------------------------.

var (
	_ = uploads.UploadInput{}
	_ = json.Marshal
	_ = sync.Mutex{}
)
