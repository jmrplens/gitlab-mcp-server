// credential_test.go covers the per-credential half of a server shared by every
// credential of one configuration shape: the registry that holds it, the
// context values that carry it, and the middleware that puts it back in front
// of a handler.
//
// Everything here used to be a capture. The client a handler closed over, the
// bucket its middleware held, the watchers its manager owned and the counter
// its ceiling drew on were all decided when the server was built, which was
// correct by construction while a server served one credential and cost a full
// registered catalog per credential. Making them per request is what replaced
// that, so the binding is now the only thing keeping two tenants apart on one
// server, and a request nothing bound must fail closed rather than fall back to
// whichever credential happened to build the shape.
package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// credentialTestState builds a state the way a pool entry's would look, without
// a pool: an owner, a client, a bucket and a counter.
func credentialTestState(t *testing.T, owner string) *credentialState {
	t.Helper()
	return &credentialState{
		owner:   owner,
		client:  newMockGitLabClient(t),
		limiter: toolutil.NewRateLimiter(10, 40),
		listen:  &listenCounter{},
	}
}

// TestCredentialStates_AddGetRemove_TracksThePool covers the registry the pool's
// insert and evict callbacks keep in step with itself.
//
// It is keyed by the entry's opaque owner token rather than by the entry
// pointer, so that the notification filter, which only ever sees the token, and
// the request path, which sees the entry, agree without a second lookup table.
// The guards matter as much as the lookups: the callbacks fire under the pool's
// write lock, so a state that cannot be filed has to be dropped rather than
// panic there.
func TestCredentialStates_AddGetRemove_TracksThePool(t *testing.T) {
	states := &credentialStates{}
	mine := credentialTestState(t, "owner-mine")
	theirs := credentialTestState(t, "owner-theirs")
	states.add(mine)
	states.add(theirs)

	t.Run("a recorded entry", func(t *testing.T) {
		if got := states.get("owner-mine"); got != mine {
			t.Errorf("get = %v, want the state that was added", got)
		}
	})

	t.Run("an owner the pool never held", func(t *testing.T) {
		if got := states.get("owner-nobody"); got != nil {
			t.Errorf("get = %v, want nil for an owner nothing recorded", got)
		}
	})

	t.Run("no owner at all", func(t *testing.T) {
		if got := states.get(""); got != nil {
			t.Errorf("get(\"\") = %v, want nil; an unbound request names no entry", got)
		}
	})

	t.Run("no registry at all", func(t *testing.T) {
		var absent *credentialStates
		if got := absent.get("owner-mine"); got != nil {
			t.Errorf("get on a nil registry = %v, want nil; stdio wires none", got)
		}
	})

	t.Run("a state that cannot be filed is dropped", func(t *testing.T) {
		states.add(nil)
		states.add(&credentialState{})
		if got := states.get(""); got != nil {
			t.Errorf("a state with no owner was filed anyway: %v", got)
		}
	})

	t.Run("eviction drops only its own entry", func(t *testing.T) {
		states.remove("owner-mine")

		if got := states.get("owner-mine"); got != nil {
			t.Errorf("get after remove = %v, want nil; the state must not outlive its pool entry", got)
		}
		if got := states.get("owner-theirs"); got != theirs {
			t.Errorf("get(other) = %v, want it untouched by another entry's eviction", got)
		}
	})

	t.Run("removing what is not there", func(t *testing.T) {
		// Both reachable: the pool evicts an entry whose insert callback found
		// no shape, and an empty owner is what a state that was never filed
		// reports.
		states.remove("owner-mine")
		states.remove("")
	})
}

// TestCredentialStates_Remove_StopsTheCredentialsWatchers covers what eviction
// releases.
//
// While each credential had a server of its own, eviction dropped only the
// pool's reference and a live session kept working. On a shared server it
// cannot: the same eviction forgets which credential that session belongs to,
// so every later notification for it would be filtered away in silence.
// Stopping the watchers turns that into the ending the specification asks for,
// and it has to happen off the caller's goroutine because Manager.Close waits
// for every watcher to unwind while eviction holds the pool's write lock.
func TestCredentialStates_Remove_StopsTheCredentialsWatchers(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)
	runtime := newTestRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())

	const uri = "gitlab://project/42/pipeline/99"
	if err := runtime.manager.Subscribe(t.Context(), testSession, uri); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if runtime.manager.Len() != 1 {
		t.Fatalf("watchers = %d before the eviction, want 1", runtime.manager.Len())
	}

	states := &credentialStates{}
	state := credentialTestState(t, "owner-evicted")
	state.subs = runtime
	states.add(state)

	states.remove("owner-evicted")

	waitFor(t, func() bool { return runtime.manager.Len() == 0 })
	if got := runtime.manager.Len(); got != 0 {
		t.Errorf("watchers = %d after the credential was evicted, want 0; "+
			"they would poll GitLab for a credential the pool no longer holds", got)
	}
}

// TestCredentialState_Close_WithNothingToRelease_IsANoOp covers the two shapes
// close is called on that own no watchers: the nil state a registry hands back
// for an owner it never held, and the state of a capability surface that offers
// no subscriptions.
func TestCredentialState_Close_WithNothingToRelease_IsANoOp(t *testing.T) {
	var absent *credentialState
	absent.close()
	credentialTestState(t, "owner-mine").close()
}

// TestWithCredentialState_InstallsTheStateAndItsClientTogether covers the
// context values every downstream reader resolves through.
//
// The client is installed in the same call rather than left to each reader
// because those two facts must never disagree: a handler resolving its client
// through Client.For and a middleware resolving the rate-limit bucket through
// credentialStateFrom have to be talking about the same tenant.
func TestWithCredentialState_InstallsTheStateAndItsClientTogether(t *testing.T) {
	state := credentialTestState(t, "owner-mine")

	t.Run("a bound request", func(t *testing.T) {
		ctx := withCredentialState(t.Context(), state)

		if got := credentialStateFrom(ctx); got != state {
			t.Errorf("credentialStateFrom = %v, want the state that was installed", got)
		}
		if got := ownerOfRequest(ctx); got != "owner-mine" {
			t.Errorf("ownerOfRequest = %q, want %q", got, "owner-mine")
		}
		bound, ok := gitlabclient.ClientFrom(ctx)
		if !ok {
			t.Fatal("no client was bound; every handler would fall back to the unbound one and refuse")
		}
		if bound != state.client {
			t.Error("the bound client is not the credential's own; a request would run as another tenant")
		}
	})

	t.Run("an unbound request", func(t *testing.T) {
		ctx := withCredentialState(t.Context(), nil)

		if got := credentialStateFrom(ctx); got != nil {
			t.Errorf("credentialStateFrom = %v, want nil on a context nothing bound", got)
		}
		if got := ownerOfRequest(ctx); got != "" {
			t.Errorf("ownerOfRequest = %q, want %q on a context nothing bound", got, "")
		}
		if _, ok := gitlabclient.ClientFrom(ctx); ok {
			t.Error("a client was bound by a nil state; stdio and the in-memory transport must stay unbound")
		}
	})
}

// TestWithRequestCredential_CarriesTheGatesAnswerToTheBinding covers the hop
// between the two layers.
//
// The gate resolves the pool entry on the HTTP request context; the binding
// middleware runs on the MCP handler's context, which does not descend from it.
// The carrier registry is the bridge, and this is the value it carries.
func TestWithRequestCredential_CarriesTheGatesAnswerToTheBinding(t *testing.T) {
	state := credentialTestState(t, "owner-mine")

	t.Run("a resolved credential", func(t *testing.T) {
		ctx := withRequestCredential(t.Context(), state)
		if got := credentialFromRequestContext(ctx); got != state {
			t.Errorf("credentialFromRequestContext = %v, want the state the gate resolved", got)
		}
	})

	t.Run("nothing resolved", func(t *testing.T) {
		ctx := withRequestCredential(t.Context(), nil)
		if got := credentialFromRequestContext(ctx); got != nil {
			t.Errorf("credentialFromRequestContext = %v, want nil", got)
		}
	})
}

// registerCarrier registers ctx under a fresh carrier token and returns the
// header an MCP request would arrive with, the way the HTTP middleware stamps
// it. The registry is process-wide, so the entry is removed again with the test.
func registerCarrier(t *testing.T, ctx context.Context) http.Header {
	t.Helper()
	token := "carrier-" + t.Name()
	mcpCarriers.contexts.Store(token, ctx)
	t.Cleanup(func() { mcpCarriers.contexts.Delete(token) })
	return carrierHeaderWith(token)
}

// TestCredentialStates_BindCredential_RunsEachRequestUnderItsOwnCredential
// covers the middleware that makes a shared server correct.
//
// The channel is the carrier header for the reason requestCarriers already
// records: it is the only per-request value the SDK exposes on both transports,
// and a context value would be right in stateless mode and wrong in stateful
// mode, where the session is connected with the initialize POST's context.
//
// Every way of failing to resolve one leaves the request unbound rather than
// guessing. On stdio and the in-memory transport that is the normal case and
// the server's own client answers; on a shape server the captured client is the
// unbound one, so the same absence makes the request fail closed.
func TestCredentialStates_BindCredential_RunsEachRequestUnderItsOwnCredential(t *testing.T) {
	states := &credentialStates{}
	state := credentialTestState(t, "owner-mine")
	states.add(state)

	t.Run("a request carrying a live carrier", func(t *testing.T) {
		header := registerCarrier(t, withRequestCredential(t.Context(), state))

		var boundOwner string
		var boundClient *gitlabclient.Client
		handler := states.bindCredential(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			boundOwner = ownerOfRequest(ctx)
			boundClient, _ = gitlabclient.ClientFrom(ctx)
			return &mcp.CallToolResult{}, nil
		})

		if _, err := handler(t.Context(), "tools/call", carrierRequest(header)); err != nil {
			t.Fatalf("bindCredential returned %v", err)
		}
		if boundOwner != "owner-mine" {
			t.Errorf("owner = %q, want %q installed on the handler's context", boundOwner, "owner-mine")
		}
		if boundClient != state.client {
			t.Error("the handler did not receive the credential's own client")
		}
	})

	unbound := []struct {
		name string
		req  mcp.Request
		why  string
	}{
		{
			name: "a request from a transport that stamps no token",
			req:  stdioRequest(),
			why:  "stdio and the in-memory transport bind nothing; the server's own client is the answer there",
		},
		{
			name: "a request whose carrier has already ended",
			req:  carrierRequest(carrierHeaderWith("a-token-nothing-registered")),
			why:  "the POST is over, so there is no credential left to resolve",
		},
		{
			name: "a carrier the gate resolved nothing on",
			req:  nil, // filled in below, since it needs a registered carrier
			why:  "the gate stamped no credential, so there is nothing to install",
		},
	}
	unboundCarrier := registerCarrier(t, context.Background())
	unbound[2].req = carrierRequest(unboundCarrier)

	for _, tt := range unbound {
		t.Run(tt.name, func(t *testing.T) {
			var bound bool
			handler := states.bindCredential(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
				bound = credentialStateFrom(ctx) != nil
				return &mcp.CallToolResult{}, nil
			})

			if _, err := handler(t.Context(), "tools/call", tt.req); err != nil {
				t.Fatalf("bindCredential returned %v", err)
			}
			if bound {
				t.Errorf("a credential was bound where none could be resolved: %s", tt.why)
			}
		})
	}
}

// TestServerShell_NewCredentialState_TakesWhatTheEntryDecides covers the state
// the pool's insert callback mints for a freshly built entry.
//
// The shell supplies what the shape decides (the subscription machinery, the
// configured rate) and the entry supplies what the credential decides: its
// client, its resolved configuration, its owner token. The watchers are the
// half that must come from the entry, since a watcher polls with a token and
// ADR-0015 makes its first read the subscription's authorization check.
func TestServerShell_NewCredentialState_TakesWhatTheEntryDecides(t *testing.T) {
	gitlab := gateStubGitLab(t, false)
	// The rate is read from the entry's own resolved configuration rather than
	// from the shell's, so the pool is what has to carry it here. The shell is
	// deliberately built with the opposite setting, which is what makes the
	// assertion below prove where the bucket came from.
	pool := serverpool.New(&config.Config{
		GitLabURL:      gitlab,
		Tier:           edition.Free,
		TierExplicit:   true,
		IgnoreScopes:   true,
		RateLimitRPS:   config.DefaultHTTPRateLimitRPS,
		RateLimitBurst: config.DefaultRateLimitBurst,
	}, okFactory)
	entry := gateTestEntry(t, pool, gateTestToken, gitlab)

	shell, err := newServerShell(t.Context(), newMockGitLabClient(t), &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
	}, withSubscriptionOptions(fastOptions()))
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}

	state := shell.newCredentialState(entry)
	t.Cleanup(state.close)

	if state.owner != entry.Owner() {
		t.Errorf("owner = %q, want the entry's own token %q", state.owner, entry.Owner())
	}
	if state.owner == "" {
		t.Error("the entry was minted with no owner; nothing could tell its notifications from another tenant's")
	}
	if state.client != entry.Client() {
		t.Error("the state carries a client other than the entry's; requests would run as another tenant")
	}
	if state.limiter == nil {
		t.Error("no rate-limit bucket was built; the entry's own configured rate was ignored")
	}
	if state.listen == nil {
		t.Error("no listen counter was built; the per-credential ceiling could not be charged")
	}
	if state.subs == nil {
		t.Fatal("no watchers were built on a full capability surface")
	}
	if state.subs.notifier.owner != entry.Owner() {
		t.Errorf("the notifier stamps %q, want the entry's owner %q; its notifications would be filtered away",
			state.subs.notifier.owner, entry.Owner())
	}
	if state.subs.reader.client != entry.Client() {
		t.Error("the watcher polls with a client other than the entry's; the authorization check would be somebody else's")
	}
}

// TestServerShell_NewCredentialState_AnEntryWithNoConfiguration_FallsBackToTheShell
// covers the guard in front of the entry's configuration.
//
// A pool entry always carries one, so this is unreachable from serveHTTPOn; it
// is answered rather than dereferenced because the alternative is a nil
// dereference inside the pool's insert callback, which runs under the pool's
// write lock and would take the process with it.
func TestServerShell_NewCredentialState_AnEntryWithNoConfiguration_FallsBackToTheShell(t *testing.T) {
	shell, err := newServerShell(t.Context(), newMockGitLabClient(t), &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceMinimal,
		RateLimitRPS:      config.DefaultHTTPRateLimitRPS,
		RateLimitBurst:    config.DefaultRateLimitBurst,
	})
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}

	// The zero entry: no configuration, no client, no owner.
	state := shell.newCredentialState(&serverpool.Entry{})
	t.Cleanup(state.close)

	if state.limiter == nil {
		t.Error("the shell's own rate was not used for an entry that resolved none")
	}
	if state.subs != nil {
		t.Error("watchers were built on the minimal capability surface, which registers no subscribable resource")
	}
}

// TestServerShell_DefaultCredentialState_FailsClosedOnlyWhenShared covers the
// state a request that bound none runs under.
//
// On stdio that is the process's one credential, and it owns the watchers, the
// bucket and the ceiling exactly as the server used to, so nothing about the
// single-credential case changed. On a server shared by a configuration shape
// it is the fail-closed default instead: no watchers, and the unbound client,
// so a subscription that could not be attributed is refused rather than
// accepted and polled with a client that refuses every read.
func TestServerShell_DefaultCredentialState_FailsClosedOnlyWhenShared(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")

	tests := []struct {
		name         string
		opts         []serverOption
		wantWatchers bool
	}{
		{
			name:         "one credential, as stdio has",
			opts:         []serverOption{withSubscriptionOptions(fastOptions())},
			wantWatchers: true,
		},
		{
			name: "a server shared by a configuration shape",
			opts: []serverOption{
				withSubscriptionOptions(fastOptions()),
				withSharedCredentials(&credentialStates{}, newSessionOwners()),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, err := newServerShell(t.Context(), subscriptionGitLabClient(t, gitlab.URL), &config.ServerConfig{
				ToolSurface:       config.ToolSurfaceDynamic,
				CapabilitySurface: config.CapabilitySurfaceFull,
			}, tt.opts...)
			if err != nil {
				t.Fatalf("newServerShell: %v", err)
			}
			t.Cleanup(shell.state.close)

			if shell.state == nil {
				t.Fatal("the shell has no default state; every unbound request would resolve to nothing at all")
			}
			if got := shell.state.subs != nil; got != tt.wantWatchers {
				t.Errorf("default watchers = %v, want %v", got, tt.wantWatchers)
			}
			if shell.state.owner != "" {
				t.Errorf("the default state claims owner %q; it belongs to no pool entry", shell.state.owner)
			}
			// Resolved for a request nothing bound, which is the whole point of
			// its existing.
			if got := shell.stateFor(t.Context()); got != shell.state {
				t.Errorf("stateFor(unbound) = %v, want the shell's default", got)
			}
			if got := shell.subscriptionRuntimeFor(t.Context()); (got != nil) != tt.wantWatchers {
				t.Errorf("subscriptionRuntimeFor(unbound) = %v, want watchers=%v", got, tt.wantWatchers)
			}
			if got := shell.listenCounterFor(t.Context()); got != shell.state.listen {
				t.Errorf("listenCounterFor(unbound) = %v, want the default state's counter", got)
			}
			if got := shell.rateLimiterFor(t.Context()); got != shell.state.limiter {
				t.Errorf("rateLimiterFor(unbound) = %v, want the default state's bucket", got)
			}
		})
	}
}

// TestServerShell_StateFor_ABoundRequest_OverridesTheDefault covers the other
// branch of the same resolution: whatever the binding middleware installed
// wins, which is what makes one server answer for many credentials.
func TestServerShell_StateFor_ABoundRequest_OverridesTheDefault(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	shell, err := newServerShell(t.Context(), subscriptionGitLabClient(t, gitlab.URL), &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
	}, withSharedCredentials(&credentialStates{}, newSessionOwners()))
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}

	bound := credentialTestState(t, "owner-mine")
	ctx := withCredentialState(t.Context(), bound)

	if got := shell.stateFor(ctx); got != bound {
		t.Errorf("stateFor(bound) = %v, want the request's own credential", got)
	}
	if got := shell.rateLimiterFor(ctx); got != bound.limiter {
		t.Error("the rate limiter came from the shell rather than the request's credential; one tenant would throttle another")
	}
	if got := shell.listenCounterFor(ctx); got != bound.listen {
		t.Error("the listen counter came from the shell; one budget would be shared by every tenant")
	}
	if got := shell.subscriptionRuntimeFor(ctx); got != bound.subs {
		t.Errorf("subscriptionRuntimeFor(bound) = %v, want the request's own watchers", got)
	}
}

// TestServerShell_ResolversWithNoStateAtAll_AnswerNothing covers the nil-state
// branch of each resolver.
//
// It is unreachable from newServerShell, which always builds a default, and
// answered rather than dereferenced because each of these runs inside a
// middleware: a nil here would be a panic on the request path rather than a
// refusal.
func TestServerShell_ResolversWithNoStateAtAll_AnswerNothing(t *testing.T) {
	shell := &serverShell{}
	ctx := t.Context()

	if got := shell.stateFor(ctx); got != nil {
		t.Errorf("stateFor = %v, want nil", got)
	}
	if got := shell.subscriptionRuntimeFor(ctx); got != nil {
		t.Errorf("subscriptionRuntimeFor = %v, want nil so an unattributed subscribe is refused", got)
	}
	if got := shell.rateLimiterFor(ctx); got != nil {
		t.Errorf("rateLimiterFor = %v, want nil, which the limiter reads as unlimited", got)
	}
	if got := shell.listenCounterFor(ctx); got != nil {
		t.Errorf("listenCounterFor = %v, want nil, which the process-wide ceiling still bounds", got)
	}
}
