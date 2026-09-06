// shape_test.go covers what a server is built for, and how many of them a
// process ends up with.
//
// The key is the whole decision: two credentials whose configurations produce
// the same string are served by the same mcp.Server, and everything that server
// holds is decided by those values. A field wrongly left out of it would serve
// one tenant another tenant's catalog; a field wrongly put in would build one
// full server per credential, which is exactly the cost sharing exists to
// remove. Both directions are asserted here.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// shapeTestConfig is the configuration every shape-key case starts from, so a
// difference between two keys is always the one field the case changed.
func shapeTestConfig() *config.ServerConfig {
	return &config.ServerConfig{
		GitLabURL:         "https://gitlab.example.com",
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
		MetaParamSchema:   "opaque",
		Tier:              edition.Free,
		TierExplicit:      true,
	}
}

// TestServerShapeKey_TheInstanceIsNotPartOfTheShape pins the one field that is
// deliberately left out.
//
// Two instances of the same tier are served the same catalog, and what differs
// between them is the client, which is per credential regardless. Putting the
// URL in would build one full server per instance for no gain, and a deployment
// publishing several instances is exactly the one that most wants the sharing.
func TestServerShapeKey_TheInstanceIsNotPartOfTheShape(t *testing.T) {
	first := shapeTestConfig()
	second := shapeTestConfig()
	second.GitLabURL = "https://gitlab.other.example"

	if serverShapeKey(first, false) != serverShapeKey(second, false) {
		t.Errorf("two instances of one tier produced different shapes:\n %s\n %s",
			serverShapeKey(first, false), serverShapeKey(second, false))
	}
}

// TestServerShapeKey_EveryCatalogDecidingFieldChangesTheShape is the other
// direction: each field that decides which tools exist, what shape they are
// listed in, or how the catalog is narrowed must produce a different server.
//
// A field missing here is a tenant served somebody else's surface. Read-only
// derived from the token's scope is in the list on purpose: it is set per pool
// entry by NarrowToTokenScope rather than by configuration, so a read_api token
// and a full one must not share a catalog even though the operator configured
// neither.
func TestServerShapeKey_EveryCatalogDecidingFieldChangesTheShape(t *testing.T) {
	base := shapeTestConfig()
	baseKey := serverShapeKey(base, false)

	tests := []struct {
		name   string
		dotcom bool
		change func(*config.ServerConfig)
	}{
		{name: "tool surface", change: func(c *config.ServerConfig) { c.ToolSurface = config.ToolSurfaceMeta }},
		{name: "the legacy meta-tools switch", change: func(c *config.ServerConfig) {
			c.ToolSurface = ""
			c.MetaTools = true
		}},
		{name: "capability surface", change: func(c *config.ServerConfig) {
			c.CapabilitySurface = config.CapabilitySurfaceMinimal
		}},
		{name: "meta parameter schema", change: func(c *config.ServerConfig) { c.MetaParamSchema = "full" }},
		{name: "tier", change: func(c *config.ServerConfig) { c.Tier = edition.Ultimate }},
		{name: "whether the tier was pinned", change: func(c *config.ServerConfig) { c.TierExplicit = false }},
		{name: "gitlab.com rather than self-managed", dotcom: true},
		{name: "read only", change: func(c *config.ServerConfig) { c.ReadOnly = true }},
		{name: "read only derived from the token scope", change: func(c *config.ServerConfig) {
			c.ReadOnlyFromTokenScope = true
		}},
		{name: "safe mode", change: func(c *config.ServerConfig) { c.SafeMode = true }},
		{name: "excluded tools", change: func(c *config.ServerConfig) { c.ExcludeTools = []string{"gitlab_project"} }},
		{name: "the token scopes", change: func(c *config.ServerConfig) { c.TokenScopes = []string{"read_api"} }},
		{name: "statelessness", change: func(c *config.ServerConfig) { c.Stateless = true }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := shapeTestConfig()
			if tt.change != nil {
				tt.change(cfg)
			}

			if got := serverShapeKey(cfg, tt.dotcom); got == baseKey {
				t.Errorf("changing %s left the shape key unchanged (%s); "+
					"two credentials differing in it would share one catalog", tt.name, got)
			}
		})
	}
}

// TestServerShapeKey_IsStableForOneConfiguration pins that the key is a
// function of the configuration and nothing else: two equal configurations must
// hash together whatever order their slices were built in, or a process would
// accumulate a shape per request.
func TestServerShapeKey_IsStableForOneConfiguration(t *testing.T) {
	first := shapeTestConfig()
	first.ExcludeTools = []string{"gitlab_project"}
	first.TokenScopes = []string{"api", "read_api"}

	second := shapeTestConfig()
	second.ExcludeTools = []string{"gitlab_project"}
	// The same scopes in the other order: the key sorts them, since the pool
	// takes them from whatever GitLab or the bearer guard reported.
	second.TokenScopes = []string{"read_api", "api"}

	if serverShapeKey(first, false) != serverShapeKey(second, false) {
		t.Errorf("one configuration produced two shapes:\n %s\n %s",
			serverShapeKey(first, false), serverShapeKey(second, false))
	}
}

// countingShapes returns a registry whose builder records how often it ran, so
// a test can assert that a shape is built once rather than per credential.
//
// The registration hook records the shapes it was handed, and asserts nothing
// on its own goroutine: it is called by get, under the registry's lock, so a
// failure here would be reported from whichever goroutine happened to be
// building.
func countingShapes(builds *int64, mu *sync.Mutex, fail error) (*shapeServers, *[]*serverShape) {
	var started []*serverShape
	shapes := newShapeServers(
		func(*config.ServerConfig, bool) (*serverShape, error) {
			mu.Lock()
			*builds++
			mu.Unlock()
			if fail != nil {
				return nil, fail
			}
			return &serverShape{shell: &serverShell{
				server: mcp.NewServer(&mcp.Implementation{Name: "shape", Version: "0"}, nil),
			}}, nil
		},
		func(shape *serverShape) {
			mu.Lock()
			started = append(started, shape)
			mu.Unlock()
		},
	)
	return shapes, &started
}

// TestShapeServers_Get_BuildsOncePerShape covers the whole point of the
// registry: the first credential of a configuration pays for the catalog and
// every later one finds it built.
//
// Building the catalog was the entire cost of a pool entry, measured at 1.8s on
// the dynamic surface, and it used to be paid on the first request of every
// credential. A second build for one shape would put that cost straight back.
func TestShapeServers_Get_BuildsOncePerShape(t *testing.T) {
	var builds int64
	var mu sync.Mutex
	shapes, started := countingShapes(&builds, &mu, nil)

	first, err := shapes.get(shapeTestConfig(), false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// A second credential of the same shape, differing only in the instance,
	// which the key deliberately leaves out.
	sameShape := shapeTestConfig()
	sameShape.GitLabURL = "https://gitlab.other.example"
	second, err := shapes.get(sameShape, false)
	if err != nil {
		t.Fatalf("get(same shape): %v", err)
	}

	if second != first {
		t.Error("a second credential of one shape got a server of its own; the catalog was built twice")
	}
	if builds != 1 {
		t.Errorf("the builder ran %d time(s) for one shape, want 1", builds)
	}
	if first.key == "" {
		t.Error("the shape was filed without its key, so forget could never find it")
	}

	// A different shape is a different server, or one tenant's narrowing would
	// decide another's catalog.
	otherShape := shapeTestConfig()
	otherShape.ReadOnly = true
	third, err := shapes.get(otherShape, false)
	if err != nil {
		t.Fatalf("get(other shape): %v", err)
	}
	if third == first {
		t.Error("a read-only configuration was served the writable shape's server")
	}
	if builds != 2 {
		t.Errorf("the builder ran %d time(s) for two shapes, want 2", builds)
	}
	if got := shapes.count(); got != 2 {
		t.Errorf("count = %d, want 2", got)
	}

	// Registration is started once per built shape, and started by the registry
	// rather than by the builder. The order is what makes a fast failure able to
	// remove the shape it belongs to: started from inside build, the failure
	// would look for a shape not yet filed under its key, find nothing, and
	// leave a server with no tools cached for the life of the process.
	//
	// Copied out from under the recorder's lock before asserting: the registry
	// takes its own lock around the hook, so reaching into the registry while
	// holding this one would invert the two.
	mu.Lock()
	registered := slices.Clone(*started)
	mu.Unlock()

	if len(registered) != 2 {
		t.Fatalf("registration was started %d time(s) for two shapes, want 2", len(registered))
	}
	for i, shape := range registered {
		if shape.key == "" {
			t.Errorf("shape %d was handed to registration before it was filed under its key", i)
		}
		if shapes.forServer(shape.shell.server) != shape {
			t.Errorf("shape %d was handed to registration before it was reachable by server", i)
		}
	}
}

// TestShapeServers_Get_ABuildFailure_IsNotCached covers the error path: a
// configuration whose shell cannot be built (a malformed description
// substitution, say) must not leave anything behind for the next request to
// find.
func TestShapeServers_Get_ABuildFailure_IsNotCached(t *testing.T) {
	var builds int64
	var mu sync.Mutex
	forced := errors.New("the shell could not be built")
	shapes, started := countingShapes(&builds, &mu, forced)

	shape, err := shapes.get(shapeTestConfig(), false)

	if !errors.Is(err, forced) {
		t.Fatalf("get error = %v, want the builder's own", err)
	}
	if shape != nil {
		t.Errorf("get returned %v alongside an error", shape)
	}
	if got := shapes.count(); got != 0 {
		t.Errorf("count = %d after a failed build, want 0; the next request would find the failure cached", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*started) != 0 {
		t.Errorf("registration was started for %d shape(s) that were never built", len(*started))
	}
}

// TestShapeServers_Get_ARegistrationThatFailsAtOnce_LeavesNothingBehind is the
// ordering the start hook exists for.
//
// Registration is started by get, after the shape is filed under its key and
// under the same lock, rather than from inside the builder. Started from inside
// the builder, a registration that failed immediately would call forget before
// the shape had been inserted, find nothing to remove, and leave a server with
// no tools cached for the life of the process: every later request for that
// configuration would be answered from the failed gate and nothing would ever
// rebuild it.
//
// The hook forgets the shape on a goroutine, as the real one does: it is called
// with the registry's lock held, so forgetting on the caller's goroutine would
// deadlock rather than test anything.
func TestShapeServers_Get_ARegistrationThatFailsAtOnce_LeavesNothingBehind(t *testing.T) {
	var builds int64
	var shapes *shapeServers
	forgotten := make(chan struct{}, 2)
	shapes = newShapeServers(
		func(*config.ServerConfig, bool) (*serverShape, error) {
			builds++
			return &serverShape{shell: &serverShell{
				server: mcp.NewServer(&mcp.Implementation{Name: "shape", Version: "0"}, nil),
			}}, nil
		},
		func(shape *serverShape) {
			go func() {
				shapes.forget(shape.shell.server)
				forgotten <- struct{}{}
			}()
		},
	)

	if _, err := shapes.get(shapeTestConfig(), false); err != nil {
		t.Fatalf("get: %v", err)
	}
	<-forgotten

	if got := shapes.count(); got != 0 {
		t.Fatalf("count = %d after a registration that failed at once, want 0; "+
			"a server with no tools would answer for that configuration forever", got)
	}

	// The next credential of that configuration builds again rather than
	// finding the broken one.
	if _, err := shapes.get(shapeTestConfig(), false); err != nil {
		t.Fatalf("get after the failure: %v", err)
	}
	<-forgotten
	if builds != 2 {
		t.Errorf("the builder ran %d time(s), want 2: the failed shape was cached rather than rebuilt", builds)
	}
}

// TestShapeServers_ForServerAndForget_FindAShapeByItsServer covers the two
// lookups keyed on the built server rather than on the configuration.
//
// The pool hands its callbacks an entry, and what the entry carries is the
// server; forServer is how the shell that built it, and so the way to mint the
// entry's own state, is found again. forget is the failure path: a registration
// that fails has failed for every credential of the shape, so the shape itself
// has to go or the next credential finds the broken one.
func TestShapeServers_ForServerAndForget_FindAShapeByItsServer(t *testing.T) {
	var builds int64
	var mu sync.Mutex
	shapes, _ := countingShapes(&builds, &mu, nil)

	shape, err := shapes.get(shapeTestConfig(), false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	t.Run("the shape that owns a server", func(t *testing.T) {
		if got := shapes.forServer(shape.shell.server); got != shape {
			t.Errorf("forServer = %v, want the shape that built it", got)
		}
	})

	t.Run("a server no shape owns", func(t *testing.T) {
		stranger := mcp.NewServer(&mcp.Implementation{Name: "stranger", Version: "0"}, nil)
		if got := shapes.forServer(stranger); got != nil {
			t.Errorf("forServer(stranger) = %v, want nil", got)
		}
	})

	t.Run("no server at all", func(t *testing.T) {
		if got := shapes.forServer(nil); got != nil {
			t.Errorf("forServer(nil) = %v, want nil", got)
		}
		shapes.forget(nil)
		if got := shapes.count(); got != 1 {
			t.Errorf("forget(nil) dropped %d shape(s)", 1-got)
		}
	})

	t.Run("forgetting makes the next request rebuild", func(t *testing.T) {
		shapes.forget(shape.shell.server)

		if got := shapes.count(); got != 0 {
			t.Fatalf("count = %d after forget, want 0", got)
		}
		rebuilt, rebuildErr := shapes.get(shapeTestConfig(), false)
		if rebuildErr != nil {
			t.Fatalf("get after forget: %v", rebuildErr)
		}
		if rebuilt == shape {
			t.Error("the forgotten shape was handed back; a failed registration would stay poisoned")
		}
		if builds != 2 {
			t.Errorf("the builder ran %d time(s), want 2: once before forget and once after", builds)
		}
	})
}

// TestShapeServers_Get_ConcurrentCallsForOneShape_BuildOnce pins the lock held
// across the build.
//
// Releasing it around the build instead would let two credentials of one shape
// each build a server, and the loser's would be discarded after its
// registration had already started: a full catalog built and thrown away, with
// the pool entries that were already pointing at it left behind.
//
// Run under -race, where an unsynchronized map would be reported outright.
func TestShapeServers_Get_ConcurrentCallsForOneShape_BuildOnce(t *testing.T) {
	const callers = 32

	var builds int64
	var mu sync.Mutex
	var started []*serverShape
	// arrived counts the callers that have reached get. The build holds until
	// every one of them has, which is what makes the race deterministic: with
	// the lock held across the build they are all queued behind it, so a second
	// builder can only appear if the lock was released around it, and then all
	// thirty-one of them appear. Without the wait the window is a few
	// microseconds wide and the regression showed up in none of the first twenty
	// runs.
	var arrived atomic.Int64
	shapes := newShapeServers(
		func(*config.ServerConfig, bool) (*serverShape, error) {
			mu.Lock()
			builds++
			mu.Unlock()
			// Bounded, because a correct implementation is the case where
			// nobody else can arrive: the other callers are blocked on the
			// registry's lock, which this build holds.
			deadline := time.Now().Add(2 * time.Second)
			for arrived.Load() < callers && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			return &serverShape{shell: &serverShell{
				server: mcp.NewServer(&mcp.Implementation{Name: "shape", Version: "0"}, nil),
			}}, nil
		},
		func(shape *serverShape) {
			mu.Lock()
			started = append(started, shape)
			mu.Unlock()
		},
	)

	results := make([]*serverShape, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Go(func() {
			<-start
			arrived.Add(1)
			results[i], errs[i] = shapes.get(shapeTestConfig(), false)
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: get: %v", i, err)
		}
	}
	for i, got := range results {
		if got != results[0] {
			t.Fatalf("caller %d got a different server than caller 0; one shape was built twice", i)
		}
	}
	if builds != 1 {
		t.Errorf("the builder ran %d time(s) for %d concurrent callers of one shape, want 1", builds, callers)
	}
	if got := shapes.count(); got != 1 {
		t.Errorf("count = %d, want 1", got)
	}
	mu.Lock()
	registrations := len(started)
	mu.Unlock()
	if registrations != 1 {
		t.Errorf("registration was started %d time(s) for one shape, want 1; "+
			"a second run would build the whole catalog again and throw it away", registrations)
	}
}

// TestShapeConfig_StripsTheInstanceTheShapeDoesNotName covers the configuration
// the shell captures.
//
// The instance is what the shape key leaves out, so leaving it in the config
// would let the first credential of a shape decide which GitLab URL every later
// one's instructions and logs named. Everything else has to survive, since the
// key says those values are the same for every credential of the shape.
func TestShapeConfig_StripsTheInstanceTheShapeDoesNotName(t *testing.T) {
	cfg := shapeTestConfig()
	cfg.ReadOnly = true
	cfg.ExcludeTools = []string{"gitlab_project"}

	shaped := shapeConfig(cfg)

	if shaped.GitLabURL != "" {
		t.Errorf("GitLabURL = %q, want it stripped; it is answered per request by the client", shaped.GitLabURL)
	}
	if cfg.GitLabURL == "" {
		t.Error("the caller's own configuration was modified; the pool entry still needs its instance")
	}
	if !shaped.ReadOnly || shaped.Tier != cfg.Tier || len(shaped.ExcludeTools) != 1 {
		t.Errorf("shapeConfig dropped more than the instance: %+v", shaped)
	}
}

// TestShapeClient_CarriesNoCredential covers what a shape's server is
// registered with.
//
// Registration reads a client only for the instance class, and every handler it
// registers resolves the caller's client from the request context. Building the
// shape with a real credential would leave that credential reachable from a
// server other tenants are served by, which is what the unbound client exists
// to prevent: it refuses every request rather than answering as whoever built
// the shape.
func TestShapeClient_CarriesNoCredential(t *testing.T) {
	tests := map[string]bool{"self-managed": false, "gitlab.com": true}

	for name, dotcom := range tests {
		t.Run(name, func(t *testing.T) {
			client := shapeClient(dotcom)

			if client == nil {
				t.Fatal("shapeClient returned no client; registration needs one for the instance class")
			}
			if client.IsGitLabDotCom() != dotcom {
				t.Errorf("IsGitLabDotCom = %v, want %v; the Orbit group depends on it", client.IsGitLabDotCom(), dotcom)
			}
			// Every request through it fails closed rather than reaching the
			// instance under whichever credential built the shape.
			if _, err := client.Initialize(t.Context()); !errors.Is(err, gitlabclient.ErrUnboundClient) {
				t.Errorf("a request through the shape's client = %v, want %v", err, gitlabclient.ErrUnboundClient)
			}
		})
	}
}

// TestStartShapeRegistration_ASuccessfulBuild_OpensTheGate covers the ordinary
// path: the catalog is built on a goroutine behind the readiness gate, so the
// first caller's handshake does not wait for it, and the gate opens once it is
// there.
func TestStartShapeRegistration_ASuccessfulBuild_OpensTheGate(t *testing.T) {
	client := newMockGitLabClient(t)
	shell, err := newServerShell(t.Context(), client, &config.ServerConfig{
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceMinimal,
	})
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}
	shape := &serverShape{shell: shell}

	var failures int64
	var mu sync.Mutex
	startShapeRegistration(t.Context(), shape, func(*mcp.Server) {
		mu.Lock()
		failures++
		mu.Unlock()
	})

	waitForCatalog(t, shell)
	if !shell.gate.isReady() {
		t.Error("the catalog was built and the readiness gate is still shut")
	}
	mu.Lock()
	defer mu.Unlock()
	if failures != 0 {
		t.Errorf("the failure callback ran %d time(s) for a build that succeeded", failures)
	}
}

// TestStartShapeRegistration_AFailedBuild_FailsTheGateAndDropsTheShape covers
// the failure the callback exists for.
//
// This is [TestStartPooledRegistration_AFailedBuild_FailsTheGateAndEvicts] with
// the blast radius the sharing gives it: a catalog that cannot be built has
// failed for every credential of the shape, not for one, so the requests parked
// on the gate are failed, the shape is forgotten, and every pool entry pointing
// at that server is evicted. The next request of that configuration rebuilds
// instead of finding either the broken shape or an entry with no tools.
func TestStartShapeRegistration_AFailedBuild_FailsTheGateAndDropsTheShape(t *testing.T) {
	forced := failDynamicCatalog(t, nil)
	client := newMockGitLabClient(t)
	shell, err := newServerShell(t.Context(), client, &config.ServerConfig{ToolSurface: config.ToolSurfaceDynamic})
	if err != nil {
		t.Fatalf("newServerShell: %v", err)
	}
	shape := &serverShape{shell: shell}
	failed := make(chan *mcp.Server, 1)

	startShapeRegistration(t.Context(), shape, func(srv *mcp.Server) { failed <- srv })

	select {
	case srv := <-failed:
		if srv != shell.server {
			t.Error("the failure callback was handed a server other than the shape's own")
		}
	case <-t.Context().Done():
		t.Fatal("the shape was never reported as failed")
	}
	if cause := shell.gate.failed(); !errors.Is(cause, forced) {
		t.Errorf("gate failure = %v, want the build's own error", cause)
	}
	if shell.gate.isReady() {
		t.Error("the gate opened for a catalog that could not be built")
	}
}

// waitForCatalog blocks until a shell's readiness gate settles, either way.
//
// The budget is the liveness one rather than waitFor's two seconds: this waits
// on a real dynamic catalog build, which is about 1.5 seconds warm and several
// times that under -race on a cold shared catalog.
func waitForCatalog(t *testing.T, shell *serverShell) {
	t.Helper()
	deadline := time.Now().Add(testHTTPLivenessTimeout)
	for time.Now().Before(deadline) {
		if shell.gate.isReady() || shell.gate.failed() != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the catalog build never finished, and the readiness gate never settled either way")
}

// TestStartShapeRegistration_ForgetsThroughTheRegistry drives the two halves
// wired to each other, the way serveHTTPOn wires them, rather than each in
// isolation: the registry starts the registration, the registration fails, and
// the failure has to leave the registry with nothing for the next credential to
// find.
//
// Assembled the same way round as production for the reason the ordering exists
// at all: the hook is called with the registry's lock held, and the failure it
// reports arrives on a goroutine that takes that lock again.
func TestStartShapeRegistration_ForgetsThroughTheRegistry(t *testing.T) {
	failDynamicCatalog(t, nil)
	client := newMockGitLabClient(t)
	var shapes *shapeServers
	shapes = newShapeServers(
		func(cfg *config.ServerConfig, _ bool) (*serverShape, error) {
			shell, err := newServerShell(t.Context(), client, cfg)
			if err != nil {
				return nil, err
			}
			return &serverShape{shell: shell}, nil
		},
		func(shape *serverShape) {
			startShapeRegistration(t.Context(), shape, shapes.forget)
		},
	)

	cfg := shapeTestConfig()
	cfg.ToolSurface = config.ToolSurfaceDynamic
	if _, err := shapes.get(cfg, false); err != nil {
		t.Fatalf("get: %v", err)
	}

	waitFor(t, func() bool { return shapes.count() == 0 })
	if got := shapes.count(); got != 0 {
		t.Errorf("count = %d after the registration failed, want 0; "+
			"the next credential of that configuration would find the broken shape", got)
	}
}

// TestServerShapeKey_NamesTheFieldsItHashes is a readability guard rather than
// a behavioral one: the key ends up in a log line an operator reads when a
// deployment builds more servers than expected, so it has to say which value
// each part is.
func TestServerShapeKey_NamesTheFieldsItHashes(t *testing.T) {
	key := serverShapeKey(shapeTestConfig(), false)

	for _, field := range []string{"surface=", "capability=", "schema=", "tier=", "dotcom=", "stateless="} {
		t.Run(strings.TrimSuffix(field, "="), func(t *testing.T) {
			if !strings.Contains(key, field) {
				t.Errorf("the shape key %q does not name %q", key, field)
			}
		})
	}
}

// shapedPoolGitLab is a GitLab that answers everything a shaped pool asks of it:
// the credential probe and identity lookup that build an entry, the version
// endpoint, and one project a subscription can watch.
func shapedPoolGitLab(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"username":"testuser"}`))
	})
	mux.HandleFunc("GET /api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"test"}`))
	})
	mux.HandleFunc("GET /api/v4/projects/42", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"name":"proj","path_with_namespace":"g/p","web_url":"https://example.invalid/g/p"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestNewShapedServerPool_EvictingAnEntry_EndsWhatThatCredentialOwned drives the
// pool and the shape registry wired to each other, which is the only place the
// teardown of a pooled credential exists.
//
// Each half of that teardown looks covered from a distance and is not: the tests
// that appear to exercise it call sessions.forgetOwner and credentials.remove
// themselves, so deleting either call from the wiring left the whole cmd/server
// suite green. What an evicted credential must leave behind is nothing: no
// session the gate would still accept, no watchers polling GitLab for a token
// the pool no longer holds, and no listen stream held open with nobody to fill
// it.
func TestNewShapedServerPool_EvictingAnEntry_EndsWhatThatCredentialOwned(t *testing.T) {
	gitlab := shapedPoolGitLab(t)
	binding, pool := newShapedServerPool(t.Context(), &config.Config{
		GitLabURL:         gitlab,
		Tier:              edition.Free,
		TierExplicit:      true,
		IgnoreScopes:      true,
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceFull,
	})
	t.Cleanup(pool.Close)

	entry, err := pool.GetOrCreateEntry("glpat-evicted", gitlab, nil)
	if err != nil {
		t.Fatalf("GetOrCreateEntry: %v", err)
	}
	state := binding.credentials.get(entry.Owner())
	if state == nil {
		t.Fatal("the pool's insert callback filed no state for the entry it built")
	}

	// A session, a watcher and an open listen stream: the three things the
	// entry owns that outlive the request that created them.
	session, _ := connectedSessions(t)
	binding.sessions.record(session, entry.Owner())
	if got := binding.sessions.ownerOf(session); got != entry.Owner() {
		t.Fatalf("ownerOf = %q before the eviction, want the entry's owner", got)
	}
	const uri = "gitlab://project/42"
	if subErr := state.subs.manager.Subscribe(t.Context(), session, uri); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}
	streamCtx, cancelStream := context.WithCancel(t.Context())
	t.Cleanup(cancelStream)
	_, release := state.streams.arm([]string{uri}, entry.Owner(), nil, cancelStream)
	t.Cleanup(release)

	// The entry is in use while it holds those, which is what keeps the idle
	// sweep off a client that subscribed and then waited.
	if !binding.credentials.inUse(entry) {
		t.Error("an entry with a watcher and an open stream was reported idle; the sweep would evict it")
	}

	pool.EvictServer(entry.Server())

	if got := binding.sessions.ownerOf(session); got != "" {
		t.Errorf("ownerOf = %q after the eviction, want none: the gate would go on accepting that session", got)
	}
	if binding.credentials.get(entry.Owner()) != nil {
		t.Error("the evicted entry's state is still filed; it keeps a GitLab client and a watcher set reachable")
	}
	waitFor(t, func() bool { return state.subs.manager.Len() == 0 && streamCtx.Err() != nil })
	if got := state.subs.manager.Len(); got != 0 {
		t.Errorf("watchers = %d after the eviction, want 0; they poll GitLab for a credential the pool dropped", got)
	}
	if streamCtx.Err() == nil {
		t.Error("the evicted credential's listen stream was left open, so its client is neither served nor told")
	}
	if binding.credentials.inUse(entry) {
		t.Error("an entry the pool no longer holds is still reported in use")
	}
}
