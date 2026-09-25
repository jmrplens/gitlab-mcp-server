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
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/subscriptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
		states.remove("owner-mine", nil, endOfCredentialEviction)

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
		states.remove("owner-mine", nil, endOfCredentialEviction)
		states.remove("", nil, endOfCredentialEviction)
	})
}

// TestCredentialStates_Remove_StopsTheWatchersAndEndsTheStreams covers what
// eviction releases, and what it tells the client.
//
// While each credential had a server of its own, eviction dropped only the
// pool's reference and a live session kept working. On a shared server it
// cannot: the same eviction forgets which credential that session belongs to,
// so every later notification for it would be filtered away in silence.
//
// Stopping the watchers is only half of the ending. Manager.Close is the one
// stop path that fires no OnStop, so nothing reaches listenStreams.stoppedFor
// and the client's open subscriptions/listen was left neither closed nor
// completed: a stream that stayed open and silent for the rest of its life,
// which is precisely the outcome ADR-0020 rejects. Ending this credential's
// streams is what makes the eviction an ending it is told about.
//
// All of it happens off the caller's goroutine, because Manager.Close waits for
// every watcher to unwind while eviction holds the pool's write lock.
func TestCredentialStates_Remove_StopsTheWatchersAndEndsTheStreams(t *testing.T) {
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

	// Real contexts, because ending a stream means canceling the one the SDK's
	// listen handler is blocked on: that is what makes it write the completion
	// result the client is owed.
	streams := newListenStreams()
	mineCtx, cancelMine := context.WithCancel(t.Context())
	t.Cleanup(cancelMine)
	_, releaseMine := streams.arm([]string{uri}, "owner-evicted", nil, cancelMine)
	t.Cleanup(releaseMine)
	theirsCtx, cancelTheirs := context.WithCancel(t.Context())
	t.Cleanup(cancelTheirs)
	_, releaseTheirs := streams.arm([]string{uri}, "owner-still-pooled", nil, cancelTheirs)
	t.Cleanup(releaseTheirs)

	states := &credentialStates{}
	state := credentialTestState(t, "owner-evicted")
	state.subs = runtime
	state.streams = streams
	states.add(state)

	states.remove("owner-evicted", nil, endOfCredentialEviction)

	waitFor(t, func() bool { return runtime.manager.Len() == 0 && mineCtx.Err() != nil })
	if got := runtime.manager.Len(); got != 0 {
		t.Errorf("watchers = %d after the credential was evicted, want 0; "+
			"they would poll GitLab for a credential the pool no longer holds", got)
	}
	if mineCtx.Err() == nil {
		t.Error("the evicted credential's listen stream was left open; its client is served nothing and told nothing")
	}
	if theirsCtx.Err() != nil {
		t.Error("another credential's listen stream was ended by this one's eviction")
	}
}

// TestCredentialState_Busy_ReportsTheWorkThePoolCannotSee covers what exempts
// an entry from idle eviction.
//
// The pool measures idleness by when it last handed an entry out, and a
// credential whose only activity is a subscription produces no such hit: the
// watcher polls GitLab directly and the listen is one request the client holds
// open rather than repeats. Reading lastUsed alone therefore evicted a client
// that was being served correctly, and ended its subscriptions under it.
func TestCredentialState_Busy_ReportsTheWorkThePoolCannotSee(t *testing.T) {
	_, gitlab := newPipelineBackend(t, "running")
	client := subscriptionGitLabClient(t, gitlab.URL)

	watching := credentialTestState(t, "owner-watching")
	watching.subs = newTestRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(watching.subs.close)
	if err := watching.subs.manager.Subscribe(t.Context(), testSession, "gitlab://project/42/pipeline/99"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	listening := credentialTestState(t, "owner-listening")
	if !listening.listen.acquire(2) {
		t.Fatal("the listen counter refused the first stream")
	}

	// The same credential as above, with the per-credential ceiling removed.
	// GITLAB_MCP_MAX_LISTEN_STREAMS=0 is documented as removing a ceiling, and
	// the counter it switched off is also the only evidence this credential is
	// holding a stream open: without the count, a client whose only activity is
	// an open subscriptions/listen reads as quiet, gets idle-swept and has its
	// stream closed for having done nothing wrong. Counting and capping are two
	// jobs, and only the second is what the variable turns off.
	uncapped := credentialTestState(t, "owner-listening-uncapped")
	if !uncapped.listen.acquire(0) {
		t.Fatal("the listen counter refused a stream with no ceiling configured")
	}

	quiet := credentialTestState(t, "owner-quiet")
	quiet.subs = newTestRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), fastOptions())
	t.Cleanup(quiet.subs.close)

	tests := []struct {
		name  string
		state *credentialState
		want  bool
	}{
		{name: "a credential with a watcher", state: watching, want: true},
		{name: "a credential holding an open listen stream", state: listening, want: true},
		{name: "a credential holding one with no ceiling configured", state: uncapped, want: true},
		{name: "a credential with neither", state: quiet},
		{name: "a credential on a surface with no subscriptions", state: credentialTestState(t, "owner-minimal")},
		{name: "an owner the registry never held", state: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.busy(); got != tt.want {
				t.Errorf("busy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// busyStateCase is one credential state busy() is measured in, with the answer
// it gives there.
type busyStateCase struct {
	name  string
	state *credentialState
	want  bool
}

// busyStates builds a credential state for each way busy() reaches its answer:
// one holding an open listen stream and no watcher, where the stream count
// decides and the watcher count is never read; one holding a watcher and no
// stream, where both are read; and one holding neither, where both are read and
// both are zero. Each carries a watcher manager, so what they hold is the only
// difference between them.
//
// It takes a testing.TB, unlike the helpers the tests above build states with,
// because the allocation pin and the benchmark measure the same three states.
//
// The watcher polls at the production cadence a pipeline that has finished is
// polled at, so it stays quiet for the length of a measurement: a poll landing
// inside one would be counted against busy(), since the allocation count a
// measurement reads is the process's.
func busyStates(tb testing.TB) []busyStateCase {
	tb.Helper()
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(hdrContentType, mimeJSON)
		switch r.URL.Path {
		case "/api/v4/version":
			_, _ = w.Write([]byte(`{"version":"16.0.0","revision":"test"}`))
		case "/api/v4/projects/42/pipelines/99":
			_, _ = w.Write([]byte(`{"id":99,"iid":7,"status":"success","ref":"main","sha":"abc123",` +
				`"web_url":"https://gitlab.example.com/p/-/pipelines/99","source":"push"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	tb.Cleanup(gitlab.Close)
	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: gitlab.URL, GitLabToken: testToken})
	if err != nil {
		tb.Fatalf("gitlabclient.NewClient: %v", err)
	}
	holding := func(owner string) *credentialState {
		runtime := newTestRuntime(client, subscriptionCfg(config.CapabilitySurfaceFull), subscriptions.Options{})
		tb.Cleanup(runtime.close)
		return &credentialState{owner: owner, listen: &listenCounter{}, subs: runtime}
	}

	streaming := holding("owner-streaming")
	if !streaming.listen.acquire(0) {
		tb.Fatal("the listen counter refused a stream with no ceiling configured")
	}
	watching := holding("owner-watching")
	if err = watching.subs.manager.Subscribe(tb.Context(), testSession, "gitlab://project/42/pipeline/99"); err != nil {
		tb.Fatalf("Subscribe: %v", err)
	}
	quiet := holding("owner-quiet")

	return []busyStateCase{
		{name: "stream_open", state: streaming, want: true},
		{name: "watchers_only", state: watching, want: true},
		{name: "neither", state: quiet, want: false},
	}
}

// TestCredentialState_Busy_AllocatesNothing pins busy() at no allocation in
// each of the three states it can be asked about.
//
// The pool's idle sweep asks it under the pool's write lock, once for every
// entry the sweep passes, so whatever it costs is paid while every other
// request waits for that lock. It reads one counter and, only when no stream is
// open, the watcher count behind the manager's mutex, and neither allocates.
// This is the baseline the layer that moves the predicate into the register
// (POL-003) is held to: reading the state through the interface that layer
// declares must not allocate either.
//
// It does not run in parallel: the count a measurement reads is the whole
// process's, so a test allocating beside it would be counted against busy().
func TestCredentialState_Busy_AllocatesNothing(t *testing.T) {
	for _, tc := range busyStates(t) {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.state.busy(); got != tc.want {
				t.Fatalf("busy() = %v, want %v; the state is not the one this case measures", got, tc.want)
			}
			if allocs := testing.AllocsPerRun(1000, func() { _ = tc.state.busy() }); allocs != 0 {
				t.Errorf("busy() allocates %v times per call, want 0", allocs)
			}
		})
	}
}

// BenchmarkCredentialState_Busy measures busy() in the same three states as
// [TestCredentialState_Busy_AllocatesNothing], which is the baseline benchstat
// compares the layer that moves the predicate into the register against.
func BenchmarkCredentialState_Busy(b *testing.B) {
	for _, tc := range busyStates(b) {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = tc.state.busy()
			}
		})
	}
}

// TestCredentialState_Close_WithNothingToRelease_IsANoOp covers the two shapes
// close is called on that own no watchers: the nil state a registry hands back
// for an owner it never held, and the state of a capability surface that offers
// no subscriptions.
func TestCredentialState_Close_WithNothingToRelease_IsANoOp(t *testing.T) {
	var absent *credentialState
	absent.close(nil, endOfCredentialEviction)
	credentialTestState(t, "owner-mine").close(nil, endOfCredentialEviction)
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
	t.Cleanup(func() { state.close(nil, endOfCredentialEviction) })

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

// TestServerShell_TwoCredentialsOfOneUser_HoldTwoOfEverything pins the server
// half of what one GitLab user holding two tokens gets today: two credential
// states, each with a rate-limit bucket, a listen counter and a watcher manager
// of its own.
//
// The tenant policy specification defines a tenant as the pair (canonical
// instance URL, GitLab user id), and many credentials map to one tenant
// (TEN-001, TEN-003). What a credential holds on a shared server is keyed on
// its pool entry instead, and an entry is one per token, so one user's two
// tokens hold two of everything (F-01, issue 955). This is AC-004 of the
// specification's dated record (plan/issue-565/spec.md) on the server, beside
// the pool's own half in internal/serverpool, and it pins the behavior as it
// is, not a target: a change that keys an allowance on the tenant is the change
// that must break it, and say so.
func TestServerShell_TwoCredentialsOfOneUser_HoldTwoOfEverything(t *testing.T) {
	// The stub answers every token as user 42, so the two below are one user's.
	gitlab := gateStubGitLab(t, false)
	pool := serverpool.New(&config.Config{
		GitLabURL:      gitlab,
		Tier:           edition.Free,
		TierExplicit:   true,
		IgnoreScopes:   true,
		RateLimitRPS:   config.DefaultHTTPRateLimitRPS,
		RateLimitBurst: config.DefaultRateLimitBurst,
	}, okFactory)
	first := gateTestEntry(t, pool, "glpat-first-token-of-user-42", gitlab)
	second := gateTestEntry(t, pool, "glpat-second-token-of-user-42", gitlab)
	if first.Identity().UserID != "42" || second.Identity().UserID != "42" {
		t.Fatalf("identities = %+v and %+v, want one user, 42", first.Identity(), second.Identity())
	}
	if first.Config().GitLabURL != second.Config().GitLabURL {
		t.Fatalf("instances = %q and %q, want one instance", first.Config().GitLabURL, second.Config().GitLabURL)
	}

	shell, err := newServerShell(t.Context(), newMockGitLabClient(t), &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
	}, withSubscriptionOptions(fastOptions()))
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}
	mine := shell.newCredentialState(first)
	t.Cleanup(func() { mine.close(nil, endOfCredentialEviction) })
	theirs := shell.newCredentialState(second)
	t.Cleanup(func() { theirs.close(nil, endOfCredentialEviction) })

	if mine.owner == theirs.owner {
		t.Errorf("both states carry owner %q; one token's notifications would reach the other's sessions", mine.owner)
	}
	if mine.limiter == nil || theirs.limiter == nil {
		t.Fatal("a state was built with no bucket although the entries resolved a rate")
	}
	if mine.limiter == theirs.limiter {
		t.Error("the two tokens draw on one bucket; the limit would be the user's, which nothing here keys on")
	}
	if mine.listen == nil || theirs.listen == nil {
		t.Fatal("a state was built with no listen counter")
	}
	if mine.listen == theirs.listen {
		t.Error("the two tokens share one listen counter")
	}
	if mine.subs == nil || theirs.subs == nil {
		t.Fatal("a state was built with no watchers on a full capability surface")
	}
	if mine.subs.manager == theirs.subs.manager {
		t.Error("the two tokens share one watcher manager")
	}

	// Two counters in what they count, and not only in where they live: a
	// stream one token holds open is not one the other holds.
	if !mine.listen.acquire(0) {
		t.Fatal("the listen counter refused a stream with no ceiling configured")
	}
	if got := theirs.listen.count(); got != 0 {
		t.Errorf("the other token's counter reads %d open streams, want 0", got)
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
	t.Cleanup(func() { state.close(nil, endOfCredentialEviction) })

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
				withSharedCredentials(&credentialStates{}, newSessionOwners(false)),
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
			t.Cleanup(func() { shell.state.close(nil, endOfCredentialEviction) })

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
	}, withSharedCredentials(&credentialStates{}, newSessionOwners(false)))
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}

	bound := credentialTestState(t, "owner-mine")
	ctx := withCredentialState(t.Context(), bound)

	if got := shell.stateFor(ctx); got != bound {
		t.Errorf("stateFor(bound) = %v, want the request's own credential", got)
	}
	if shell.rateLimiterFor(ctx) != bound.limiter {
		t.Error("the rate limiter came from the shell rather than the request's credential; one tenant would throttle another")
	}
	if shell.listenCounterFor(ctx) != bound.listen {
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
