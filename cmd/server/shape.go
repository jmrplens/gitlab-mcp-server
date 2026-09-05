package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// serverShapeKey names the configuration a server is built for.
//
// Two credentials whose configurations produce the same key are served by the
// same [mcp.Server], because everything that server holds is decided by these
// values: the catalog and its schemas, the tool surface projected from it, the
// resource and prompt registrations, the instructions, the manifest snapshot,
// and the capabilities in the handshake.
//
// # What is in it, and why
//
//   - The tool surface, the capability surface and the meta parameter-schema
//     mode: each of the three decides which tools exist and what shape they are
//     listed in.
//   - The tier, and whether it was pinned or detected: the tier prunes input and
//     output schemas per field, so it changes the catalog itself.
//   - Whether the instance is GitLab.com: it decides whether the Orbit group
//     exists at all.
//   - Read-only, including read-only derived from the token's scope, safe mode,
//     the excluded tools and the token scopes: these are the narrowing
//     [gitlabtools.FilterActionCatalog] applies, and two credentials narrowed
//     differently are served different catalogs.
//
// # What is deliberately not in it
//
// The instance URL. Two instances of the same tier are served the same catalog,
// and what differs between them is the client, which is per credential
// regardless. Putting the URL in would build one full server per instance for
// no gain, and a deployment publishing several instances is exactly the one
// that most wants the sharing.
//
// Nor is anything about the credential itself. The token, the user, the
// instance the token belongs to: none of them changes what is registered, and a
// key that depended on any of them would be a key per credential, which is what
// this replaces.
func serverShapeKey(cfg *config.ServerConfig, dotcom bool) string {
	toolSurface := config.EffectiveToolSurface(cfg.MetaTools, cfg.ToolSurface)
	capabilitySurface := config.EffectiveCapabilitySurface(cfg.CapabilitySurface)
	return fmt.Sprintf("surface=%s|capability=%s|schema=%s|tier=%s|tierPinned=%t|dotcom=%t|%s|stateless=%t",
		toolSurface,
		capabilitySurface,
		cfg.MetaParamSchema,
		cfg.Tier.String(),
		cfg.TierExplicit,
		dotcom,
		gitlabtools.CatalogFilterKey(cfg),
		cfg.Stateless,
	)
}

// shapeServers holds one built server per configuration shape.
//
// A shape is built at most once, by whichever credential arrives first, and
// every credential after it finds the built one. Registration runs on a
// goroutine behind that server's readiness gate exactly as it did per
// credential, so the first caller does not wait for it either; the difference
// is that every later credential of the same shape now finds a server whose
// catalog is already built, which used to be the whole cost of a pool entry.
type shapeServers struct {
	mu     sync.Mutex
	shapes map[string]*serverShape
	// build makes a shape, and exists as a field so tests can substitute one.
	build func(cfg *config.ServerConfig, dotcom bool) (*serverShape, error)
	// start is what makes a built shape usable: it registers the catalog, on a
	// goroutine, behind the readiness gate.
	//
	// It is separate from build, and called only once the shape is reachable by
	// key, because registration can fail and a failure has to remove the shape.
	// Started from inside build, a fast failure would look for a shape that had
	// not been inserted yet, find nothing, and leave a server with no tools
	// cached for the life of the process.
	start func(*serverShape)
}

// serverShape is one built server and the shell that knows how to mint a
// credential's state on it.
type serverShape struct {
	key   string
	shell *serverShell
}

func newShapeServers(
	build func(cfg *config.ServerConfig, dotcom bool) (*serverShape, error),
	start func(*serverShape),
) *shapeServers {
	return &shapeServers{shapes: make(map[string]*serverShape), build: build, start: start}
}

// get returns the shape serving cfg, building it on first use.
//
// The lock is held across the build. That is deliberate and cheap: the build is
// [newServerShell], which touches no network and takes microseconds, while the
// registration it schedules runs on a goroutine outside the lock. Releasing the
// lock around the build instead would let two credentials of one shape each
// build a server, and the loser's would be discarded after its registration had
// already started.
func (s *shapeServers) get(cfg *config.ServerConfig, dotcom bool) (*serverShape, error) {
	key := serverShapeKey(cfg, dotcom)

	s.mu.Lock()
	defer s.mu.Unlock()
	if shape, ok := s.shapes[key]; ok {
		return shape, nil
	}
	shape, err := s.build(cfg, dotcom)
	if err != nil {
		return nil, err
	}
	shape.key = key
	s.shapes[key] = shape
	if s.start != nil {
		// After the insertion and under the same lock, so a registration that
		// fails immediately still finds the shape it has to remove. It only
		// launches a goroutine, so holding the lock across it costs nothing.
		s.start(shape)
	}
	slog.Info("built the MCP server for a configuration shape",
		"shapes", len(s.shapes),
		"tool_surface", config.EffectiveToolSurface(cfg.MetaTools, cfg.ToolSurface),
		"capability_surface", config.EffectiveCapabilitySurface(cfg.CapabilitySurface),
		"tier", cfg.Tier.String(),
		"read_only", cfg.ReadOnly,
		"safe_mode", cfg.SafeMode,
	)
	return shape, nil
}

// forServer returns the shape that owns a server, or nil when none does.
//
// The pool hands its callbacks an entry, and what the entry carries is the
// server; this is how the shell that built it, and so the way to mint the
// entry's own state, is found again.
func (s *shapeServers) forServer(server *mcp.Server) *serverShape {
	if server == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, shape := range s.shapes {
		if shape.shell.server == server {
			return shape
		}
	}
	return nil
}

// forget drops a shape whose registration failed, so the next credential of
// that configuration builds it again instead of finding the broken one.
func (s *shapeServers) forget(server *mcp.Server) {
	if server == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, shape := range s.shapes {
		if shape.shell.server == server {
			delete(s.shapes, key)
		}
	}
}

// count reports how many shapes have been built.
func (s *shapeServers) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.shapes)
}

// shapeConfig returns the configuration a shape's server is built from: the
// entry's own, with the instance stripped.
//
// The instance is what the shape key leaves out, so leaving it in the config
// the shell captures would let the first credential of a shape decide which
// GitLab URL every later one's instructions and logs named. It is answered per
// request by the client instead.
func shapeConfig(cfg *config.ServerConfig) *config.ServerConfig {
	shaped := *cfg
	shaped.GitLabURL = ""
	return &shaped
}

// shapeClient is the client a shape's server is registered with: the
// credential-less one for the instance class the shape names.
//
// Registration reads a client only for the instance class, and every handler it
// registers resolves the caller's client from the request context. Building the
// shape with a real credential would leave that credential reachable from a
// server other tenants are served by, which is exactly what
// [gitlabclient.NewUnboundClient] exists to prevent.
func shapeClient(dotcom bool) *gitlabclient.Client {
	return gitlabtools.UnboundClient(dotcom)
}

// newShapedServerPool builds the credential pool of an HTTP deployment and the
// shape registry that serves it, wired to each other.
//
// The two are declared together because each needs the other. The pool's
// factory asks the registry for the server of a credential's configuration
// shape, and the registry's failure path asks the pool to drop every entry
// pointing at a server whose catalog could not be built. The returned
// [poolBinding] is what the authentication gate needs to stamp a request with
// the credential it authenticated as.
//
// The caller closes the pool.
func newShapedServerPool(ctx context.Context, cfg *config.Config) (poolBinding, *serverpool.ServerPool) {
	// sessions records which pooled credential each MCP session belongs to, so
	// the gate can refuse a session presented with a different credential and
	// so a resource-updated notification reaches only its own subscriber. It
	// replaces the per-server session tag, which a shared server cannot mint.
	sessions := newSessionOwners()
	// credentials holds the per-credential half of every shared server: the
	// client, the rate-limit bucket, the watchers, the listen ceiling.
	credentials := &credentialStates{}
	// One server per configuration shape, shared by every credential that
	// hashes to it. Declared before the pool so the factory can reach it.
	var shapes *shapeServers
	// Declared before the factory so the factory's background registration can
	// evict the entries it belongs to. The factory only reads it from a
	// goroutine it starts while serving a request, which is necessarily after
	// New has returned and assigned it.
	var pool *serverpool.ServerPool
	shapes = newShapeServers(func(serverCfg *config.ServerConfig, dotcom bool) (*serverShape, error) {
		// No server-initiated keepalive on any HTTP entry, stateful included.
		// The SDK's keepalive is a JSON-RPC ping request, and it closes the
		// session the first time one goes unanswered, so a client that is
		// simply between requests, or one whose transport does not carry
		// server-initiated messages to it, loses its session at the 30-second
		// mark for being idle. Liveness on this transport is the SSE
		// keep-alive comment (see sseAwareWriter), which puts bytes on the
		// wire without asking the client for anything.
		//
		// The shell rather than the whole server, and registration on a
		// goroutine behind the readiness gate. Building the catalog was the
		// entire cost of a pool entry, measured at 1.8s on the dynamic surface
		// and 3.0s on individual against a shell too fast to time, and it used
		// to be paid on the first request of every credential rather than once
		// per process the way stdio pays it. It is now paid once per
		// configuration shape, and answering the handshake from the shell keeps
		// even that off the path the first client waits on.
		//
		// The client is the credential-less one for the instance class: the
		// registration reads a client only for that, and every handler it
		// registers resolves the caller's own from the request context.
		shell, err := newServerShell(ctx, shapeClient(dotcom), shapeConfig(serverCfg),
			withSharedCredentials(credentials, sessions),
			withKeepAlive(0), withTransport(mcpotel.TransportTCP))
		if err != nil {
			return nil, err
		}
		return &serverShape{shell: shell}, nil
	}, func(shape *serverShape) {
		// Registration is started by the registry, once the shape is reachable
		// by key, rather than from the pool's post-insert hook: the shape is not
		// a pool entry and nothing about its lifetime depends on the insertion
		// of one. A failure drops the shape and every entry already pointing at
		// it; entries built later find no shape and rebuild.
		startShapeRegistration(ctx, shape, func(srv *mcp.Server) {
			shapes.forget(srv)
			pool.EvictServer(srv)
		})
	})
	pool = serverpool.New(cfg, func(client *gitlabclient.Client, serverCfg *config.ServerConfig) (*mcp.Server, error) {
		shape, err := shapes.get(serverCfg, client.IsGitLabDotCom())
		if err != nil {
			return nil, err
		}
		return shape.shell.server, nil
	}, serverpool.WithMaxSize(cfg.MaxHTTPClients),
		serverpool.WithOnInsert(func(entry *serverpool.Entry) {
			// Under the pool's write lock, so it does no work beyond building
			// the entry's own state: the shape it belongs to is already built,
			// and its registration is already running.
			shape := shapes.forServer(entry.Server())
			if shape == nil {
				// The shape's registration failed and forgot it between this
				// entry's build and its insertion, so the eviction that
				// followed the failure ran while this entry was not yet in the
				// map and could not have seen it. That ordering is reachable
				// whenever registration fails quickly, because the factory
				// hands the server back before the pool files the entry, and
				// without this check the poisoned entry stays cached: every
				// later request for that credential is answered from a failed
				// readiness gate, and nothing rebuilds until an idle timeout or
				// a revalidation happens to drop it.
				//
				// Checking the registry rather than the gate is what makes it
				// exhaustive: the failure path forgets the shape before it
				// evicts, so an entry inserted before the forget is found by
				// that eviction and one inserted after it is found here.
				//
				// On a goroutine, because eviction re-enters the pool and this
				// callback runs under the pool's write lock.
				slog.WarnContext(ctx, "dropping a pooled credential whose configuration shape failed to register")
				go pool.EvictServer(entry.Server())
				return
			}
			credentials.add(shape.shell.newCredentialState(entry))
		}),
		// The per-credential state must not outlive the pool entry it belongs
		// to. Without this it grew past --max-http-clients on credential churn,
		// and every stale key kept a GitLab client and a set of watchers
		// reachable. Forgetting the sessions with it is what stops an evicted
		// credential's session ID from being accepted by the entry that
		// replaced it.
		serverpool.WithOnEvict(func(entry *serverpool.Entry) {
			credentials.remove(entry.Owner())
			sessions.forgetOwner(entry.Owner())
		}),
		serverpool.WithRevalidateInterval(cfg.RevalidateInterval),
		serverpool.WithIdleTimeout(cfg.PoolIdleTimeout),
		// Entry construction is bounded by the server's lifetime rather than
		// by the request that triggered it: an entry is shared, so one client
		// disconnecting must not abort a build others are waiting on, and
		// shutdown must stop it.
		serverpool.WithBaseContext(func() context.Context { return ctx }))

	return poolBinding{credentials: credentials, sessions: sessions}, pool
}

// startShapeRegistration builds the catalog for a freshly built shape, on a
// goroutine, behind that server's readiness gate.
//
// Registration used to be started from the pool's post-insert hook, once per
// credential. Sharing changes the failure path more than the success one: a
// registration that fails has failed for every credential of the shape, so the
// pool drops all of them and the shape itself is forgotten.
func startShapeRegistration(ctx context.Context, shape *serverShape, onFailure func(*mcp.Server)) {
	shell := shape.shell
	go func() {
		if registerErr := shell.register(ctx); registerErr != nil {
			shell.gate.markFailed(registerErr)
			slog.ErrorContext(ctx, "the tool catalog could not be built for a configuration shape",
				"error", registerErr)
			onFailure(shell.server)
			return
		}
		shell.gate.markReady()
	}()
}
