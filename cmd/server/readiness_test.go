// readiness_test.go verifies the gate that lets a stdio server answer the
// handshake while its tool catalog is still being built.
//
// The property under test is the one the reported defect turned on: the
// handshake must not wait, and everything that would describe a catalog that
// does not exist yet must. The end-to-end half of this lives in
// test/e2e/stdio/readiness_test.go, where a real binary is driven over real
// pipes; these cases pin the decision itself, including the two ways a wait can
// end without the catalog ever arriving.
package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// gateReach is how long a case waits for a handler that should already have
// been reached. Generous on purpose: it bounds a hang, and every assertion that
// could fail spuriously is expressed as an ordering fact instead of a duration.
const gateReach = 10 * time.Second

// gateProbe drives one method through the gate's middleware on its own
// goroutine and records what the handler saw.
type gateProbe struct {
	// reached is signaled once the wrapped handler runs.
	reached chan struct{}
	// done is signaled once the middleware returns, whether or not the handler
	// ran.
	done chan struct{}
	// readyAtCall is what the gate reported at the moment the handler ran. It
	// is the assertion that matters: a method that had to wait can only have
	// been called after markReady, and one that did not can only have been
	// called before it.
	readyAtCall atomic.Bool
	// err is what the middleware returned.
	err atomic.Pointer[error]
}

// runThroughGate starts one request through the gate and returns its probe.
//
// The context is marked as a client's connection, which is what subjects a
// request to the gate at all; a case that wants the unmarked default passes its
// own context to runThroughUngatedConnection instead.
//
// Nothing on the spawned goroutine touches *testing.T: it records into atomics
// and channels, and every assertion is made on the test goroutine afterwards.
func runThroughGate(ctx context.Context, gate *readinessGate, method string) *gateProbe {
	return runThroughConnection(withReadinessGate(ctx), gate, method)
}

// runThroughConnection is runThroughGate without deciding for the caller
// whether the connection is a client's.
func runThroughConnection(ctx context.Context, gate *readinessGate, method string) *gateProbe {
	probe := &gateProbe{reached: make(chan struct{}), done: make(chan struct{})}
	handler := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		probe.readyAtCall.Store(gate.isReady())
		close(probe.reached)
		return &mcp.ListToolsResult{}, nil
	}
	wrapped := gate.middleware()(handler)
	go func() {
		defer close(probe.done)
		_, err := wrapped(ctx, method, nil)
		probe.err.Store(&err)
	}()
	return probe
}

// awaitChan waits for a channel to close, failing the test if it does not.
func awaitChan(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(gateReach):
		t.Fatalf("%s did not happen within %s", what, gateReach)
	}
}

// probeError returns the error the middleware returned, once it has returned.
func (p *gateProbe) probeError() error {
	if held := p.err.Load(); held != nil {
		return *held
	}
	return nil
}

// TestReadinessGate_MethodBeforeCatalog_HandshakePassesAndCatalogMethodsWait
// verifies which methods a server that is not yet registered will serve.
//
// Correctness rests on two different reasons and both are exercised here. The
// handshake calls must pass because a stdio client writes initialize before the
// process could possibly be ready, retries when nothing comes back, and has its
// connection killed by the SDK's correct refusal of the duplicate. Notifications
// must pass because the SDK's connection runs its handler queue sequentially
// and only releases the next entry when a handler calls jsonrpc2.Async, which
// notifications never do: one blocked here would stall every message behind it.
//
// The assertion is an ordering fact rather than a duration. The handler records
// whether the gate was open at the moment it ran, so a method that was supposed
// to wait fails if it ran before markReady, and one that was supposed to pass
// fails if it ran after.
func TestReadinessGate_MethodBeforeCatalog_HandshakePassesAndCatalogMethodsWait(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		wantWait bool
	}{
		{name: "initialize", method: "initialize", wantWait: false},
		{name: "discover", method: "server/discover", wantWait: false},
		{name: "ping", method: "ping", wantWait: false},
		{name: "initialized notification", method: "notifications/initialized", wantWait: false},
		{name: "cancelled notification", method: "notifications/cancelled", wantWait: false},
		{name: "progress notification", method: "notifications/progress", wantWait: false},
		{name: "tools list", method: "tools/list", wantWait: true},
		{name: "tools call", method: "tools/call", wantWait: true},
		{name: "resources list", method: "resources/list", wantWait: true},
		{name: "resources read", method: "resources/read", wantWait: true},
		{name: "prompts list", method: "prompts/list", wantWait: true},
		{name: "completion", method: "completion/complete", wantWait: true},
		{name: "subscriptions listen", method: "subscriptions/listen", wantWait: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := newReadinessGate(t.Context())
			probe := runThroughGate(t.Context(), gate, tt.method)

			if !tt.wantWait {
				awaitChan(t, probe.reached, tt.method+" reaching its handler")
				if probe.readyAtCall.Load() {
					t.Errorf("%s ran with the gate already open, so nothing was proved about waiting", tt.method)
				}
				awaitChan(t, probe.done, tt.method+" returning")
				return
			}

			gate.markReady()
			awaitChan(t, probe.reached, tt.method+" reaching its handler after the catalog was ready")
			if !probe.readyAtCall.Load() {
				t.Errorf("%s was served before the catalog existed; it would have answered about a catalog that is not there", tt.method)
			}
			awaitChan(t, probe.done, tt.method+" returning")
			if err := probe.probeError(); err != nil {
				t.Errorf("%s error = %v, want nil once the catalog is ready", tt.method, err)
			}
		})
	}
}

// TestReadinessGate_MarkReady_ReleasesEveryWaiter verifies that opening the gate
// once releases every request parked behind it, not merely the first.
//
// Registration finishes exactly once, so every method a client sent during
// startup is waiting on the same channel. A gate that woke one waiter per open
// would leave the rest hanging until their client gave up, which is the failure
// this whole change exists to remove.
func TestReadinessGate_MarkReady_ReleasesEveryWaiter(t *testing.T) {
	gate := newReadinessGate(t.Context())
	methods := []string{"tools/list", "resources/list", "prompts/list", "tools/call"}

	probes := make(map[string]*gateProbe, len(methods))
	for _, method := range methods {
		probes[method] = runThroughGate(t.Context(), gate, method)
	}

	gate.markReady()
	// A second call must be harmless: createServer and the stdio startup path
	// both end by opening the gate, and neither knows whether the other did.
	gate.markReady()

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			probe := probes[method]
			awaitChan(t, probe.done, method+" returning")
			if !probe.readyAtCall.Load() {
				t.Errorf("%s ran before the gate opened", method)
			}
			if err := probe.probeError(); err != nil {
				t.Errorf("%s error = %v, want nil", method, err)
			}
		})
	}
}

// TestReadinessGate_WaitEndsWithoutCatalog_AnswersRatherThanHangs verifies both
// ways a wait can end without the catalog arriving.
//
// A call must produce a response, and the SDK writes one even for a request the
// client has already cancelled, so the only real choice is what that response
// says. It answers with the server-busy code this server already uses for
// refusals that are about server state rather than the request: the request was
// well formed and the same one a moment later succeeds. Returning ctx.Err()
// would be marshaled as -32001, "unknown error", which is indistinguishable
// from a bug.
//
// The lifetime case is not redundant with the request case. On stdio the SDK
// wraps the connection context in jsonrpc2.notDone, so a request context there
// is never cancelled by shutdown, and mcp.Server.Run waits for in-flight
// requests after closing the session: a gate watching only the request would
// hold the process open through its own shutdown.
func TestReadinessGate_WaitEndsWithoutCatalog_AnswersRatherThanHangs(t *testing.T) {
	tests := []struct {
		name         string
		cancelServer bool
	}{
		{name: "the client gives up on the request", cancelServer: false},
		{name: "the server stops before the catalog is ready", cancelServer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifetime, stopServer := context.WithCancel(t.Context())
			defer stopServer()
			request, abandonRequest := context.WithCancel(t.Context())
			defer abandonRequest()

			gate := newReadinessGate(lifetime)
			probe := runThroughGate(request, gate, "tools/list")

			if tt.cancelServer {
				stopServer()
			} else {
				abandonRequest()
			}

			awaitChan(t, probe.done, "the refusal being returned")

			var rpcErr *jsonrpc.Error
			if err := probe.probeError(); !errors.As(err, &rpcErr) {
				t.Fatalf("error = %v (%T), want a *jsonrpc.Error a client can read", err, err)
			}
			if rpcErr.Code != codeServerBusy {
				t.Errorf("code = %d, want %d so a client knows a retry can succeed", rpcErr.Code, codeServerBusy)
			}
			if rpcErr.Message == "" {
				t.Error("the refusal carries no message, so nobody reading it learns the server was still starting")
			}
			select {
			case <-probe.reached:
				t.Error("the handler ran anyway, so the request was served from a catalog that does not exist")
			default:
			}
		})
	}
}

// TestReadinessGate_UnmarkedConnection_PassesWhileTheGateIsShut verifies the
// default registration itself depends on.
//
// Registration speaks MCP to the server it is building: the exclusion pass, the
// read-only and safe-mode filters, the tool count, the meta-route filter and
// the gitlab://tools manifest each connect an in-memory client and call
// tools/list, some on a context.Background() with no deadline. Gating those
// would make registration wait for a step that cannot finish until they return,
// which is a startup deadlock rather than the race it replaced, and it is what
// happened while the gate was opt-out, because one such call had no way to know
// it needed exempting. Only the connection serveStdio hands to a client is
// marked.
func TestReadinessGate_UnmarkedConnection_PassesWhileTheGateIsShut(t *testing.T) {
	gate := newReadinessGate(t.Context())
	probe := runThroughConnection(t.Context(), gate, "tools/list")

	awaitChan(t, probe.reached, "the registration phase's own tools/list reaching its handler")
	if probe.readyAtCall.Load() {
		t.Error("the gate was already open, so being unmarked was not what let the call through")
	}
	awaitChan(t, probe.done, "the inspection call returning")
	if err := probe.probeError(); err != nil {
		t.Errorf("error = %v, want nil", err)
	}
}

// TestDeferredCallIdentifier_Identify_AnswersNothingUntilRegistrationSetsIt
// verifies the placeholder telemetry is installed with.
//
// The middleware chain has to be complete before the first request arrives,
// because the SDK snapshots it per request, so mcpotel.Middleware is installed
// alongside the rest and the catalog it resolves calls against arrives later.
// Answering false rather than a placeholder identity is what keeps an
// unresolved call off the attribute entirely, which is what
// mcpotel.CallIdentifier asks of every implementation.
func TestDeferredCallIdentifier_Identify_AnswersNothingUntilRegistrationSetsIt(t *testing.T) {
	tests := []struct {
		name       string
		set        bool
		setToNil   bool
		wantAction string
		wantOK     bool
	}{
		{name: "before registration", set: false, wantAction: "", wantOK: false},
		{name: "after registration", set: true, wantAction: "issue.list", wantOK: true},
		{name: "registration with no catalog", set: true, setToNil: true, wantAction: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifier := &deferredCallIdentifier{}
			switch {
			case tt.set && tt.setToNil:
				identifier.set(nil)
			case tt.set:
				identifier.set(mcpotel.IdentifierFunc(func(string, any) (mcpotel.Identity, bool) {
					return mcpotel.Identity{ActionID: "issue.list", Domain: "issue"}, true
				}))
			}

			identity, ok := identifier.Identify("gitlab_execute_action", nil)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if identity.ActionID != tt.wantAction {
				t.Errorf("ActionID = %q, want %q", identity.ActionID, tt.wantAction)
			}
		})
	}
}

// TestDeferredIdentity_Middleware_OverlaysTheCallerOnceStartupResolvesIt
// verifies the replacement for a context value that used to be set before the
// transport was connected.
//
// stdio resolves one identity for the whole process by asking GitLab who the
// token belongs to, and that answer now arrives while the server is already
// serving. Tool-call logging and the telemetry identity policy both read the
// caller from the request context, so an overlay is what puts it back there;
// without it, stdio would log and trace every call as anonymous.
func TestDeferredIdentity_Middleware_OverlaysTheCallerOnceStartupResolvesIt(t *testing.T) {
	tests := []struct {
		name         string
		resolve      bool
		wantUserID   string
		wantUsername string
	}{
		{name: "before startup resolves the caller", resolve: false},
		{name: "after startup resolves the caller", resolve: true, wantUserID: "7", wantUsername: "someone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holder := &deferredIdentity{}
			if tt.resolve {
				holder.set(toolutil.UserIdentity{UserID: "7", Username: "someone"})
			}

			var seen toolutil.UserIdentity
			handler := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
				seen = toolutil.IdentityFromContext(ctx)
				return &mcp.ListToolsResult{}, nil
			}
			if _, err := holder.middleware()(handler)(t.Context(), "tools/list", nil); err != nil {
				t.Fatalf("middleware returned %v, want nil", err)
			}

			if seen.UserID != tt.wantUserID {
				t.Errorf("UserID = %q, want %q", seen.UserID, tt.wantUserID)
			}
			if seen.Username != tt.wantUsername {
				t.Errorf("Username = %q, want %q", seen.Username, tt.wantUsername)
			}
		})
	}
}

// TestPoolEntryCost_SplitsShellFromRegistration measures the two halves of
// building one pooled server, because which half dominates decides what an
// HTTP-mode readiness gate could actually buy.
//
// The shell is everything a server needs before it has spoken to GitLab:
// options, capabilities, middleware. Registration is the tool catalog, which
// is the part a client currently waits through on the first request of every
// new credential.
//
// It is a test rather than a Benchmark so it runs in the ordinary suite and
// reports its numbers, and it asserts only the ordering it exists to
// establish. Absolute timings vary per machine, so nothing here fails on a
// duration.
func TestPoolEntryCost_SplitsShellFromRegistration(t *testing.T) {
	mock := newMockGitLabServer(t)
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:   mock.URL,
		GitLabToken: testToken,
	})
	if err != nil {
		t.Fatalf("creating the client: %v", err)
	}

	surfaces := []struct {
		name    string
		surface string
	}{
		{name: "dynamic", surface: config.ToolSurfaceDynamic},
		{name: "individual", surface: config.ToolSurfaceIndividual},
	}

	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			cfg := &config.ServerConfig{ToolSurface: s.surface, Tier: edition.Free}

			shellStart := time.Now()
			shell, shellErr := newServerShell(t.Context(), client, cfg)
			shellCost := time.Since(shellStart)
			if shellErr != nil {
				t.Fatalf("newServerShell() error = %v", shellErr)
			}

			registerStart := time.Now()
			if registerErr := shell.register(t.Context()); registerErr != nil {
				t.Fatalf("register() error = %v", registerErr)
			}
			registerCost := time.Since(registerStart)

			t.Logf("%s surface: shell %v, registration %v (%.0f%% of the build is registration)",
				s.name, shellCost.Round(time.Millisecond), registerCost.Round(time.Millisecond),
				100*float64(registerCost)/float64(shellCost+registerCost))

			if registerCost <= shellCost {
				t.Errorf("registration (%v) did not dominate the shell (%v); "+
					"if this ever holds, deferring registration buys nothing and the gate should go",
					registerCost, shellCost)
			}
		})
	}
}
