package serverpool

import (
	"container/list"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/singleflight"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// ServerFactory creates a fully configured [*mcp.Server] with all tools,
// resources, and prompts registered for the given GitLab client and per-entry
// configuration.
// This is provided by the caller to decouple pool management from
// registration logic.
type ServerFactory func(client *gitlabclient.Client, cfg *config.ServerConfig) (*mcp.Server, error)

// Entry is one pooled credential: a GitLab client, the configuration resolved
// for it, the user it belongs to, and the MCP server that serves it.
//
// The server is deliberately not the entry's own. Since one server is built per
// configuration shape and shared by every credential that hashes to it, the same
// [*mcp.Server] answers for many entries, and a caller that holds only that
// pointer can no longer say which credential a request belongs to. Everything
// that used to be keyed on the server — the tag its sessions carry, the
// subscription watchers, the rate-limit bucket, the caller identity — is keyed
// on the entry instead, and [Entry.Owner] is the opaque name it goes by.
type Entry struct {
	server        *mcp.Server
	client        *gitlabclient.Client
	serverConfig  *config.ServerConfig
	identity      UserIdentity
	owner         string
	element       *list.Element
	createdAt     time.Time
	lastValidated time.Time
	lastUsed      time.Time
	// rejected is set the moment GitLab answers 401 to a call on this entry,
	// before the eviction that follows has taken the lock, so a request that
	// finds the entry in between rebuilds instead of reusing a credential
	// GitLab has already refused.
	rejected atomic.Bool
}

// Server returns the MCP server serving this entry, which may be shared with
// every other entry of the same configuration shape.
func (e *Entry) Server() *mcp.Server {
	if e == nil {
		return nil
	}
	return e.server
}

// Client returns the GitLab client carrying this entry's credential.
func (e *Entry) Client() *gitlabclient.Client {
	if e == nil {
		return nil
	}
	return e.client
}

// Config returns the configuration resolved for this entry: the process
// settings, plus the instance, tier and token-scope narrowing discovered when
// it was built.
func (e *Entry) Config() *config.ServerConfig {
	if e == nil {
		return nil
	}
	return e.serverConfig
}

// Identity returns the GitLab user behind this entry's credential, whose zero
// value means the lookup did not succeed.
func (e *Entry) Identity() UserIdentity {
	if e == nil {
		return UserIdentity{}
	}
	return e.identity
}

// Owner returns the opaque token naming this entry.
//
// It is minted here, from [crypto/rand.Text], and is never derived from the
// credential, the user or the instance: it travels in the `_meta` of a
// resource-updated notification so a shared server can tell whose watcher
// produced it, and anything derived from the credential would be a credential
// on the wire. It is unique per entry and per process, so a rebuilt entry for
// the same token is a different owner, which is what makes eviction forget the
// sessions that belonged to the entry that is gone.
func (e *Entry) Owner() string {
	if e == nil {
		return ""
	}
	return e.owner
}

// UserIdentity is the GitLab user a pooled credential belongs to.
//
// It is resolved once when the entry is built, alongside tier and scope
// discovery, and then answers for every request that reuses the entry. The
// zero value means the lookup did not succeed — an instance that refuses
// /user to this token, say — which callers must treat as "unknown", never as
// "anonymous".
type UserIdentity struct {
	UserID   string
	Username string
}

// Resolved reports whether the identity was actually determined.
func (u UserIdentity) Resolved() bool { return u.UserID != "" }

// defaultMaxSize is the fallback number of HTTP client sessions retained when
// the operator does not configure a pool size.
const defaultMaxSize = 100

// DefaultRevalidateInterval is the default period between token re-validation
// checks via a lightweight GitLab API call.
const DefaultRevalidateInterval = 15 * time.Minute

// DefaultIdleTimeout is how long an entry may go unused before the pool
// reclaims it. Without it an abandoned entry survives until enough distinct
// token+URL pairs push it out of the LRU, holding a fully registered server
// and drawing a revalidation ping against GitLab every interval, forever.
const DefaultIdleTimeout = 1 * time.Hour

// DefaultMaxCredentialAge is the longest an entry keeps serving on the
// strength of a credential check made that long ago.
//
// It exists because nothing else bounds the window between an operator
// revoking a token and this server ceasing to answer for it. An entry is
// verified once, when it is built; the fast path then returns it and refreshes
// lastUsed with no re-verification, so an entry in continuous use never idles
// out. Periodic revalidation normally keeps the window at the revalidation
// interval, but --revalidate-interval 0 is a documented, supported setting,
// and with it off an actively used entry survived for the life of the process.
// This is the floor under that: whatever the operator turns off, a credential
// is re-checked at least this often, because the entry is rebuilt from scratch
// and the rebuild runs [verifyCredential].
//
// What survives inside the window is the *surface* — initialize, tools/list,
// the catalog, the resource and prompt listings — not the tenant's data,
// since every tool call forwards the token and GitLab answers 401 the moment
// it dies. An hour bounds that disclosure while costing at most one rebuild
// per hour per active credential.
const DefaultMaxCredentialAge = 1 * time.Hour

// maxCredentialAgeCeiling is the largest value [WithMaxCredentialAge] honors.
// A ceiling that can be set arbitrarily high is not a ceiling; this is the
// same upper bound the --revalidate-interval flag already documents.
const maxCredentialAgeCeiling = 24 * time.Hour

// idleSweepDivisor sets the sweep cadence as a fraction of the idle timeout,
// bounded below by idleSweepMinInterval so a small timeout cannot turn the
// sweep into a hot loop.
//
// The floor is the part worth stating plainly: an entry outlives its timeout by
// at most a quarter of it only while that quarter is longer than the floor,
// which means from a four-minute timeout upwards. Below that the floor
// dominates, and an entry configured to expire after a second can still be
// held for up to a minute. That is a deliberate trade — the sweep costs a lock
// and a walk of every entry — but it is not what "a quarter of the timeout"
// suggests on its own.
const (
	idleSweepDivisor     = 4
	idleSweepMinInterval = 1 * time.Minute
)

// Metrics holds operational counters for the [ServerPool]. All counters are
// monotonically increasing and use lock-free atomic increments.
type Metrics struct {
	Hits                   atomic.Int64
	Misses                 atomic.Int64
	Evictions              atomic.Int64
	IdleEvictions          atomic.Int64
	RevalidationsFailed    atomic.Int64
	RevalidationsSucceeded atomic.Int64
	// RevalidationsTransient counts revalidation rounds that could not reach
	// a verdict — the instance was unreachable, or answered 5xx — and left
	// the entry in place. Separated from RevalidationsFailed so an operator
	// can tell "tokens are being revoked" from "GitLab was down for a
	// minute", which used to look identical and evict the same way.
	RevalidationsTransient atomic.Int64
	// StaleCredentialEvictions counts entries dropped because their
	// credential had not been checked within [DefaultMaxCredentialAge].
	StaleCredentialEvictions atomic.Int64
	// RejectedCredentialEvictions counts entries dropped because GitLab
	// answered a call made with their credential with 401: the token was
	// revoked or expired while the entry was live, and the first refused
	// data call is the signal rather than the next periodic check.
	RejectedCredentialEvictions atomic.Int64
}

// Snapshot is a point-in-time copy of pool [Metrics] plus current state.
// Safe for JSON serialization and cross-goroutine use.
type Snapshot struct {
	Hits                     int64 `json:"hits"`
	Misses                   int64 `json:"misses"`
	Evictions                int64 `json:"evictions"`
	IdleEvictions            int64 `json:"idle_evictions"`
	RevalidationsFailed      int64 `json:"revalidations_failed"`
	RevalidationsSucceeded   int64 `json:"revalidations_succeeded"`
	RevalidationsTransient   int64 `json:"revalidations_transient"`
	StaleCredentialEvictions int64 `json:"stale_credential_evictions"`
	// RejectedCredentialEvictions counts entries dropped on a 401 from a
	// call made with their credential.
	RejectedCredentialEvictions int64     `json:"rejected_credential_evictions"`
	CurrentSize                 int       `json:"current_size"`
	MaxSize                     int       `json:"max_size"`
	CreatedAt                   time.Time `json:"created_at"`
}

// ServerPool maintains a bounded set of [*mcp.Server] instances keyed by
// token plus GitLab URL hash (SHA-256). When the pool reaches maxSize, the
// least recently used entry is evicted. Entries are periodically re-validated
// against the GitLab API; entries with revoked tokens are evicted automatically.
type ServerPool struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	lru     *list.List
	maxSize int
	cfg     *config.Config
	factory ServerFactory
	// onEvict is called with a server the pool has just stopped owning, for
	// every removal path: LRU pressure, idle reclamation, revalidation of a
	// revoked token, and Close. It exists because a caller that keeps its own
	// per-server state — cmd/server maps each pooled server to the tag its
	// session IDs carry — otherwise has no way to learn that an entry is gone,
	// and its map grows past the pool's own size bound.
	//
	// It runs while the pool's write lock is held, so it must be cheap and
	// must not call back into the pool.
	onInsert func(*Entry)
	onEvict  func(*Entry)
	// inUse answers whether an entry is doing work the pool cannot see, for
	// idle eviction alone. See [WithInUse].
	inUse              func(*Entry) bool
	revalidateInterval time.Duration
	idleTimeout        time.Duration
	maxCredentialAge   time.Duration
	metrics            Metrics
	createdAt          time.Time
	// building collapses concurrent first-requests for one key into a single
	// build, so a client opening several connections at once costs one set of
	// upstream lookups rather than one per connection.
	building singleflight.Group
	// probes bounds how many credential probes may be in flight at once. See
	// [maxConcurrentCredentialProbes]; nil means unbounded, which only a pool
	// built outside [New] can be.
	probes chan struct{}
	// probeQueueTimeout is how long a build waits for one of those slots,
	// defaulting to [credentialProbeQueueTimeout].
	probeQueueTimeout time.Duration
	// baseContext supplies the lifetime that bounds the GitLab lookups which
	// build an entry — the credential probe, tier and scope discovery,
	// identity resolution.
	//
	// It is the server's lifetime, not the request's: an entry is shared by
	// every request carrying the same credential, so deriving from whichever
	// one happened to trigger construction would let a single client
	// disconnecting abort work that others are already waiting on, and leave
	// the next request to start it over. Shutdown, however, must stop it.
	//
	// A function rather than a stored context, mirroring
	// [net/http.Server.BaseContext], which exists for exactly this shape: a
	// lifetime that belongs to the long-lived object rather than to any
	// caller. Storing the context itself would hide an effective deadline
	// from callers that cannot see it, which is what the guidance against
	// context fields is about.
	baseContext func() context.Context
}

// Option configures pool behavior.
type Option func(*ServerPool)

// WithOnEvict registers a callback invoked with each entry the pool removes.
//
// It takes the entry rather than its server because a server is shared by every
// entry of one configuration shape: told only "this server is gone" a caller
// would drop state belonging to credentials that are still pooled.
//
// The callback runs under the pool's write lock: it must not block and must not
// re-enter the pool.
func WithOnEvict(fn func(*Entry)) Option {
	return func(p *ServerPool) { p.onEvict = fn }
}

// WithInUse registers a callback that reports whether an entry is still doing
// work of its own, which exempts it from idle eviction.
//
// The pool measures idleness by when an entry was last handed out, and that is
// the whole truth only while every piece of work a credential has running also
// passes through the pool. It does not: an open subscriptions/listen is a
// watcher polling GitLab directly, so a client that subscribed and then went
// quiet refreshes nothing here, and after --pool-idle-timeout it was evicted
// with its subscriptions ended under it while it was being served correctly.
//
// Idle eviction skips such an entry outright. Size pressure prefers an entry
// that is not busy and takes a busy one only when every entry is
// ([ServerPool.evictLRU]), because otherwise the protection was defeasible by
// any caller willing to present --max-http-clients credentials of its own: the
// busy entries are the ones sitting at the LRU tail, precisely because their
// work does not pass through the pool. A credential GitLab has refused is
// evicted whatever this says, since there is nothing left to protect, and
// [WithOnEvict] is what tells the client in every case.
//
// Like the other callbacks it runs under the pool's write lock: it must be a
// cheap read, must not block, and must not re-enter the pool. Size pressure
// calls it once per entry it passes over, so "cheap" is meant literally.
func WithInUse(fn func(*Entry) bool) Option {
	return func(p *ServerPool) { p.inUse = fn }
}

// WithOnInsert registers a callback invoked with each server the pool has just
// cached, once it is reachable by key.
//
// It exists for work that must not start before the entry can be found again.
// A server whose catalog is registered in the background is the case: if that
// registration fails, the failure has to remove the entry, and a factory that
// started it would be racing its own insertion. The callback runs under the
// pool's write lock, so like [WithOnEvict] it must not block and must not
// re-enter the pool; starting a goroutine is what it is for.
func WithOnInsert(fn func(*Entry)) Option {
	return func(p *ServerPool) { p.onInsert = fn }
}

// WithMaxSize sets the maximum number of unique token entries in the pool.
// Values ≤ 0 are ignored; the default is 100.
func WithMaxSize(n int) Option {
	return func(p *ServerPool) {
		if n > 0 {
			p.maxSize = n
		}
	}
}

// WithRevalidateInterval sets the interval between periodic token
// re-validation checks. Values ≤ 0 disable revalidation.
func WithRevalidateInterval(d time.Duration) Option {
	return func(p *ServerPool) {
		p.revalidateInterval = d
	}
}

// WithBaseContext ties entry construction to a lifetime the caller controls,
// normally the server's root context.
//
// Without it the GitLab lookups that build an entry run under
// context.Background() and survive shutdown until their own timeout expires.
// They are deliberately not derived from the request that triggered them —
// see [ServerPool.baseContext] — but "not this request" is not the same as
// "no lifetime at all".
//
// The signature mirrors [net/http.Server.BaseContext]: a function, so the
// pool never holds a context of its own. A nil function is ignored, and one
// that returns nil falls back to [context.Background].
func WithBaseContext(fn func() context.Context) Option {
	return func(p *ServerPool) {
		if fn != nil {
			p.baseContext = fn
		}
	}
}

// WithIdleTimeout sets how long an entry may go unused before the pool
// reclaims it. Values <= 0 disable idle eviction, leaving the LRU bound as the
// only reclamation path.
func WithIdleTimeout(d time.Duration) Option {
	return func(p *ServerPool) {
		p.idleTimeout = d
	}
}

// WithMaxCredentialAge sets the ceiling on how long an entry serves without
// its credential having been re-checked against GitLab.
//
// Unlike the other options here it cannot be turned off, which is the point of
// it: see [DefaultMaxCredentialAge]. A value of zero or less keeps the
// default, and a value above [maxCredentialAgeCeiling] is clamped down to it.
func WithMaxCredentialAge(d time.Duration) Option {
	return func(p *ServerPool) {
		switch {
		case d <= 0:
			p.maxCredentialAge = DefaultMaxCredentialAge
		case d > maxCredentialAgeCeiling:
			p.maxCredentialAge = maxCredentialAgeCeiling
		default:
			p.maxCredentialAge = d
		}
	}
}

// New creates a [ServerPool]. The cfg provides shared server-wide settings
// (GitLabURL, SkipTLSVerify, etc.). The factory function creates a fully
// registered [*mcp.Server] for each new GitLab client.
func New(cfg *config.Config, factory ServerFactory, opts ...Option) *ServerPool {
	p := &ServerPool{
		entries:            make(map[string]*Entry),
		lru:                list.New(),
		maxSize:            defaultMaxSize,
		cfg:                cfg,
		factory:            factory,
		revalidateInterval: DefaultRevalidateInterval,
		idleTimeout:        DefaultIdleTimeout,
		maxCredentialAge:   DefaultMaxCredentialAge,
		createdAt:          time.Now(),
		baseContext:        context.Background,
		probes:             make(chan struct{}, maxConcurrentCredentialProbes),
		probeQueueTimeout:  credentialProbeQueueTimeout,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// lifetime returns the context that bounds entry construction, falling back
// to [context.Background] when the configured function yields nothing.
func (p *ServerPool) lifetime() context.Context {
	if p.baseContext == nil {
		return context.Background()
	}
	if ctx := p.baseContext(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// GetOrCreate returns the [*mcp.Server] for the given token and GitLab URL,
// creating one if it doesn't exist. The pool key is derived from both the
// token and gitlabURL, so the same token against different GitLab instances
// gets separate server entries. It is safe for concurrent use.
// Returns an error if the GitLab client or MCP server cannot be created.
func (p *ServerPool) GetOrCreate(token, gitlabURL string) (*mcp.Server, error) {
	return p.GetOrCreateWithScopes(token, gitlabURL, nil)
}

// GetOrCreateWithScopes is [ServerPool.GetOrCreate] for a caller that has
// already resolved the token's scopes.
//
// OAuth mode has: verifying the bearer token required reading them. Passing
// them in spares a second introspection, and more importantly it is the only
// way the entry learns them at all — the PAT self endpoint the pool would
// otherwise ask does not answer for an OAuth access token, so a read_api
// OAuth token would look like "scopes unknown" and be served a catalog it
// cannot use. A nil slice means "not resolved"; the pool then detects them
// itself, exactly as before.
func (p *ServerPool) GetOrCreateWithScopes(token, gitlabURL string, scopes []string) (*mcp.Server, error) {
	entry, err := p.GetOrCreateEntry(token, gitlabURL, scopes)
	if err != nil {
		return nil, err
	}
	return entry.server, nil
}

// GetOrCreateEntry is [ServerPool.GetOrCreateWithScopes] returning the whole
// pool entry rather than its server.
//
// It is the form every caller that has to act per credential needs, and it
// became the primary one when servers started being shared between credentials
// of the same configuration shape: the server no longer identifies the caller,
// and the entry does.
func (p *ServerPool) GetOrCreateEntry(token, gitlabURL string, scopes []string) (*Entry, error) {
	if token == "" {
		return nil, errors.New("empty token: authentication required")
	}
	if gitlabURL == "" {
		return nil, errors.New("empty GitLab URL: set --gitlab-url or send GITLAB-URL header")
	}

	key := sessionKey(token, gitlabURL)

	// Fast path: read lock to check existing entry.
	p.mu.RLock()
	cached, ok := p.entries[key]
	// Read under the same lock that guards the field: lastValidated is
	// written by the revalidation goroutine.
	stale := ok && p.maxCredentialAge > 0 && time.Since(cached.lastValidated) > p.maxCredentialAge
	rejected := ok && cached.rejected.Load()
	p.mu.RUnlock()

	switch {
	case ok && rejected:
		// GitLab refused this entry's credential on a call and the eviction
		// that follows has not taken the lock yet. Dropping it here, rather
		// than serving it once more, sends this request down the slow path,
		// which rebuilds and re-verifies; the pending eviction then finds
		// nothing under the key and does nothing.
		p.dropRejectedEntry(key, cached)
	case ok && stale:
		// The credential behind this entry has not been checked with GitLab
		// inside the ceiling, so the entry stops being an answer. Dropping it
		// sends this very request down the slow path, which rebuilds and
		// re-runs verifyCredential — a revoked token is refused there rather
		// than served from a cache nothing re-examines.
		p.evictStaleCredential(key)
	case ok:
		p.mu.Lock()
		p.lru.MoveToFront(cached.element)
		// This is the hot path for every request on an established entry, so
		// it is what keeps an active entry out of reach of idle eviction.
		cached.lastUsed = time.Now()
		p.mu.Unlock()
		p.metrics.Hits.Add(1)
		return cached, nil
	}

	// Slow path: build WITHOUT holding p.mu. Client creation, tier and scope
	// detection and the factory all perform GitLab network I/O, and doing that
	// under the write lock would serialize every caller behind one slow
	// round-trip, stalling the whole pool whenever an instance is slow.
	//
	// Callers racing for the *same* key are collapsed into one build instead of
	// each doing their own. A client that opens several connections at once —
	// which is the normal startup burst — used to cost one credential probe,
	// one tier lookup, one scope lookup and one identity lookup per connection,
	// all for a single credential, with every result but one thrown away. The
	// waiters block exactly as long as they would have blocked building it
	// themselves, so nothing is slower and the upstream cost is one.
	// Counted here rather than inside the build, so that Hits plus Misses is
	// still the number of calls: a caller that joins someone else's in-flight
	// build found no entry, which is a miss however the work was shared.
	p.metrics.Misses.Add(1)

	built, err, _ := p.building.Do(key, func() (any, error) {
		entry, buildErr := p.buildEntry(token, gitlabURL, scopes)
		if buildErr != nil {
			return nil, buildErr
		}
		return p.insertEntry(key, token, entry), nil
	})
	if err != nil {
		return nil, err
	}
	entry, ok := built.(*Entry)
	if !ok || entry == nil {
		return nil, errors.New("creating MCP server for pool: builder returned no server")
	}
	return entry, nil
}

// buildEntry creates the GitLab client, resolves the per-entry configuration,
// and builds the MCP server for a new pool key. It performs network I/O and must
// be called without holding p.mu. The returned entry has no LRU element or
// timestamps yet; insertEntry finalizes those under the lock.
func (p *ServerPool) buildEntry(token, gitlabURL string, knownScopes []string) (*Entry, error) {
	if p.factory == nil {
		return nil, errors.New("creating MCP server for pool: server factory is nil")
	}

	// Bail before doing any work if the pool's lifetime has already ended.
	// The lookups below each bound themselves with a timeout derived from
	// this context, so a cancelled one makes them fail fast — but the
	// credential probe reports "not rejected" when it cannot reach GitLab,
	// which on a cancelled context would wave a build through and register a
	// full tool catalog after shutdown had begun. Checking here stops that
	// at the door.
	if err := p.lifetime().Err(); err != nil {
		return nil, fmt.Errorf("pool shutting down, not building entry: %w", err)
	}

	// In oauth mode every credential arrives as Authorization: Bearer, and
	// the pool must forward it the same way: an OAuth access token is only
	// valid as Bearer (GitLab rejects gloas- tokens in PRIVATE-TOKEN, which
	// is what NewClientWithToken sends), while PATs work in both schemes.
	newClient := func(baseURL, token string, skipTLSVerify bool) (*gitlabclient.Client, error) {
		return gitlabclient.NewClientWithTokenRetries(baseURL, token, skipTLSVerify, p.cfg.DisableRetries)
	}
	if p.cfg.AuthMode == "oauth" {
		newClient = gitlabclient.NewOAuthClientWithToken
	}
	client, err := newClient(
		gitlabURL, token, p.cfg.SkipTLSVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client for pool: %w", err)
	}
	client.SetTier(p.cfg.Tier)

	if verifyErr := p.verifyUnderProbeBound(client); verifyErr != nil {
		return nil, verifyErr
	}

	entryCfg := p.entryConfig(client, gitlabURL, knownScopes)
	server, err := p.factory(client, entryCfg)
	if err != nil {
		return nil, fmt.Errorf("creating MCP server for pool: %w", err)
	}
	// A factory reporting success while handing back nothing is rejected here
	// rather than downstream, because the entry is what gets cached. Letting
	// it through poisons the key: the caller that triggered the build is told
	// about it, but every later caller for the same credential takes the fast
	// path, finds the entry, and receives a nil server with a nil error — the
	// dereference this check exists to prevent, minus the diagnosis.
	if server == nil {
		return nil, errors.New("creating MCP server for pool: factory returned no server")
	}

	entry := &Entry{
		server:       server,
		client:       client,
		serverConfig: entryCfg,
		identity:     resolveIdentity(p.lifetime(), client),
		// Minted before anything can observe the entry, and never again: see
		// [Entry.Owner] for why it is random rather than derived.
		owner: rand.Text(),
	}
	// The first data call GitLab refuses is the revocation signal. Without
	// this, a token revoked while its entry was live kept being served until
	// the periodic re-check, up to an hour, and every call in between was
	// relayed and refused one by one.
	//
	// Off the calling goroutine, because that goroutine may hold the pool's
	// lock: the revalidation sweep verifies credentials under it, and a 401
	// there would otherwise wait on the sweep for a lock the sweep holds.
	// The call fires once per client, so this is one goroutine per revoked
	// credential, never per request.
	// The mark is synchronous and the drop is not: between the two, a
	// request racing for the same key sees the mark on the fast path and
	// rebuilds rather than reusing the refused credential.
	key := sessionKey(token, gitlabURL)
	client.SetOnUnauthorized(func() {
		entry.rejected.Store(true)
		go p.evictRejectedCredential(key, entry)
	})
	return entry, nil
}

// evictRejectedCredential drops entry, if it is still the one under key: a
// concurrent request may already have rebuilt the key, and that entry's
// credential has just been verified.
func (p *ServerPool) evictRejectedCredential(key string, entry *Entry) {
	// On a goroutine of its own, behind the same recover the sweeps run
	// behind: dropping the entry calls back into code the pool does not own,
	// and a panic there must not take the process with it. The lock is
	// released by a defer for the same reason, so the panic cannot leave it
	// held.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("server pool: eviction of a rejected credential panicked", "panic", r)
		}
	}()
	gitlabURL, size, dropped := p.dropRejectedEntry(key, entry)
	if !dropped {
		return
	}
	slog.Info("server pool: gitlab rejected the credential on a call, dropping the entry",
		"gitlab_url", gitlabURL,
		"pool_size", size)
}

// dropRejectedEntry removes entry under the lock, if it is still the one
// under key, and reports what to log about it.
func (p *ServerPool) dropRejectedEntry(key string, entry *Entry) (gitlabURL string, size int, dropped bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	current, ok := p.entries[key]
	if !ok || current != entry {
		return "", 0, false
	}
	gitlabURL, _ = entryConfigLogValues(entry)
	if entry.element != nil {
		p.lru.Remove(entry.element)
	}
	p.metrics.RejectedCredentialEvictions.Add(1)
	p.dropEntry(key)
	return gitlabURL, len(p.entries), true
}

// resolveIdentity looks up the GitLab user behind a pooled credential.
//
// Cost is one call per pool entry, not per request, alongside the tier and
// scope lookups the entry already performs. A failure is not fatal: the
// credential has already been verified by this point, so an instance that
// will not answer /user costs the caller a username in its log lines and
// nothing else.
//
// The context is background-scoped with its own bound, matching
// [verifyCredential] and [ServerPool.entryConfig], and that is deliberate
// rather than an oversight: an entry is shared by every request carrying the
// same credential. Deriving from the request that happened to trigger
// construction would let one client disconnecting abort a build that other
// requests are already waiting on, and leave the next one to start it over.
func resolveIdentity(base context.Context, client *gitlabclient.Client) UserIdentity {
	ctx, cancel := context.WithTimeout(base, credentialCheckTimeout)
	defer cancel()

	info, err := client.CurrentUser(ctx)
	if err != nil {
		slog.Debug("could not resolve the user behind a pooled token", "error", err)
		return UserIdentity{}
	}
	// A zero id is not user zero: no such user can exist. GitLab's users.id
	// is a bigint fed by users_id_seq, whose range starts at 1 — Postgres
	// rejects setval(..., 0) as out of bounds — and the first account on a
	// fresh instance is root with id 1. So a zero here only ever means the
	// response carried no id, and formatting it would put the string "0" in
	// the logs as though it were a real user.
	if info.UserID == 0 {
		slog.Debug("gitlab returned no user id for a pooled token")
		return UserIdentity{}
	}
	return UserIdentity{UserID: strconv.Itoa(info.UserID), Username: info.Username}
}

// IdentityFor returns the GitLab user behind a pooled credential, and whether
// the pool holds an entry for it at all.
//
// Reading rather than resolving is the point: the answer was determined when
// the entry was built, so a request costs a map lookup. A caller that gets
// ok=false has asked before [ServerPool.GetOrCreate] ran for this credential.
func (p *ServerPool) IdentityFor(token, gitlabURL string) (UserIdentity, bool) {
	key := sessionKey(token, gitlabURL)

	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.entries[key]
	if !ok {
		return UserIdentity{}, false
	}
	return entry.identity, true
}

// insertEntry commits a freshly built entry under the write lock. If another
// goroutine created an entry for the same key while this one was building, the
// already-stored entry is returned and the freshly built one is discarded: its
// server holds no live sessions, mirroring [ServerPool.Close], which lets
// servers expire naturally rather than terminating them.
func (p *ServerPool) insertEntry(key, token string, entry *Entry) *Entry {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.existingEntryLocked(key); ok {
		return existing
	}

	if p.lru.Len() >= p.maxSize {
		p.evictLRU()
	}

	now := time.Now()
	entry.element = p.lru.PushFront(key)
	entry.createdAt = now
	entry.lastValidated = now
	entry.lastUsed = now
	p.entries[key] = entry

	// After the entry is reachable by key, so a callback that starts work
	// which may later have to evict this server can find it.
	if p.onInsert != nil {
		p.onInsert(entry)
	}

	slog.Info(
		"server pool: created new entry",
		"pool_size", len(p.entries),
		"gitlab_url", entry.serverConfig.GitLabURL,
		"tier", entry.serverConfig.Tier.String(),
		"enterprise", entry.serverConfig.Enterprise(),
		"tier_source", p.tierSource(),
		"scopes_detected", entry.serverConfig.TokenScopes != nil,
		"token_suffix", tokenSuffix(token),
	)

	return entry
}

// existingEntryLocked returns an existing entry for key while p.mu is held.
// Every hit refreshes lastUsed, which is what idle eviction reads; the LRU
// position alone cannot serve that purpose because it only orders entries
// relative to each other and carries no wall-clock age.
//
// It counts nothing. Its only caller is the double check in [ServerPool.insertEntry],
// which is reached from the slow path where the miss has already been charged;
// counting a hit here too would make one call show up as both, and Hits plus
// Misses would stop being the number of calls.
func (p *ServerPool) existingEntryLocked(key string) (*Entry, bool) {
	entry, ok := p.entries[key]
	if !ok {
		return nil, false
	}
	p.lru.MoveToFront(entry.element)
	entry.lastUsed = time.Now()
	return entry, true
}

// ErrInvalidCredential reports that GitLab itself rejected the credential.
//
// It is distinct from every other pool error: those mean the instance could
// not be reached or the server could not be built, whereas this one is a
// verdict from GitLab about the token. Callers map it to 401 rather than 503.
var ErrInvalidCredential = errors.New("gitlab rejected the credential")

// verifyCredential asks GitLab whether the token is usable before the pool
// admits an entry for it.
//
// Without this, the pool builds an entry for any non-empty string, so an
// unauthenticated caller can obtain a full MCP session with PRIVATE-TOKEN: x
// and a stream of distinct invented tokens churns the LRU. Checking the token
// format instead would be wrong: GitLab lets self-managed administrators
// change the glpat- prefix, so a prefix rule would reject legitimate
// self-hosted tokens while still admitting any well-shaped fake.
//
// GET /user is the probe rather than the calls entryConfig already makes,
// because neither of those is a verdict about the credential: /license
// answers 403 to a valid non-admin token, and /personal_access_tokens/self
// answers 401 to a valid credential that is not a PAT.
//
// Only an explicit 401 or 403 rejects. Any other outcome — a network error, a
// 5xx, a 404 from a stubbed instance — means no verdict was obtained, and the
// entry is admitted: failing closed whenever GitLab is unreachable would turn
// an instance outage into a total denial of service, which is worse than the
// churn this prevents.
func verifyCredential(base context.Context, client *gitlabclient.Client) error {
	ctx, cancel := context.WithTimeout(base, credentialCheckTimeout)
	defer cancel()

	if client.CredentialRejected(ctx) {
		return fmt.Errorf("%w", ErrInvalidCredential)
	}
	return nil
}

// maxConcurrentCredentialProbes is how many [verifyCredential] probes the pool
// runs at once.
//
// The singleflight group collapses concurrent requests for the same credential,
// so it bounds nothing here: every distinct token is a distinct key, and a
// stream of invented ones is the whole shape of the attack. Each admitted key
// costs one GET /user against the configured instance, so without a ceiling the
// server relays an unauthenticated flood to GitLab at whatever rate it arrives,
// amplified by nothing more than the cost of inventing a string.
//
// It is the measure that covers the distributed variant. The front door's
// failure budgets bound one source at a time and need no header spoofing to
// evade: enough sources each staying under the limit produce no blocked
// request and any number of probes. This is on the other side of that, counting
// work rather than callers.
//
// Sixteen: each probe is a single round trip with a five-second ceiling, so
// sixteen in flight is a few requests per second of steady load against the
// instance for credentials it has never seen, while a legitimate burst of new
// clients drains through in well under a second at typical latencies.
const maxConcurrentCredentialProbes = 16

// credentialProbeQueueTimeout is how long a build waits for a probe slot.
//
// A bounded wait rather than an immediate refusal: the queue is only ever
// contended by new credentials, so a legitimate burst should be served late
// rather than refused, and the wait is short enough that a caller sees a
// retryable answer well inside any sane client timeout.
const credentialProbeQueueTimeout = 5 * time.Second

// ErrCredentialProbeBusy reports that no credential probe slot came free in
// time.
//
// It is deliberately not [ErrInvalidCredential]: nothing was learned about the
// token, so the caller must map it to 503 and not to 401, and it must not be
// charged to any authentication budget. Telling a client with a perfectly good
// credential to reauthorize because the server was busy would be the same
// conflation of causes the front door already avoids for pool failures.
var ErrCredentialProbeBusy = errors.New("credential verification is saturated, retry shortly")

// acquireProbeSlot takes one of the [maxConcurrentCredentialProbes] slots,
// returning the function that gives it back.
//
// The wait is bounded by [credentialProbeQueueTimeout] and by the pool's
// lifetime, so a shutdown does not leave builds parked on a queue that will
// never move.
func (p *ServerPool) acquireProbeSlot() (func(), error) {
	if p.probes == nil {
		return func() {
			// No limiter is configured, so no slot was taken and there is
			// nothing to give back. Returning a no-op rather than nil keeps
			// every caller's deferred release unconditional.
		}, nil
	}
	wait := p.probeQueueTimeout
	if wait <= 0 {
		wait = credentialProbeQueueTimeout
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case p.probes <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-p.probes }) }, nil
	case <-p.lifetime().Done():
		return nil, fmt.Errorf("pool shutting down, not verifying credential: %w", p.lifetime().Err())
	case <-timer.C:
		slog.Warn("credential verification queue is saturated",
			"in_flight", maxConcurrentCredentialProbes,
			"waited", wait,
		)
		return nil, ErrCredentialProbeBusy
	}
}

// verifyUnderProbeBound runs [verifyCredential] holding one of the pool's probe
// slots.
//
// The release is deferred rather than called after the probe returns: a panic
// escaping the client would otherwise retire a slot permanently, and a ceiling
// that only ever shrinks ends up refusing every new credential on a server that
// is otherwise healthy.
func (p *ServerPool) verifyUnderProbeBound(client *gitlabclient.Client) error {
	release, err := p.acquireProbeSlot()
	if err != nil {
		return err
	}
	defer release()
	return verifyCredential(p.lifetime(), client)
}

// credentialCheckTimeout bounds the GET /user probe in verifyCredential. It
// runs once per new pool entry, not per request, and the probe does not retry,
// so this is a ceiling on a single round trip rather than on a retry budget.
const credentialCheckTimeout = 5 * time.Second

// entryConfig builds the per-pool-entry server configuration, applying the
// resolved GitLab URL plus optional edition and token-scope discovery.
func (p *ServerPool) entryConfig(client *gitlabclient.Client, gitlabURL string, knownScopes []string) *config.ServerConfig {
	entryCfg := p.cfg.ServerConfig()
	entryCfg.GitLabURL = gitlabURL

	// Detect the tier from the instance license only when the operator did not
	// pin it explicitly via --tier/GITLAB_MCP_TIER.
	autoDetectTier := !p.cfg.TierExplicit
	needScopes := !p.cfg.IgnoreScopes && knownScopes == nil
	if autoDetectTier || needScopes {
		ctx, cancel := context.WithTimeout(p.lifetime(), 10*time.Second)
		defer cancel()

		if autoDetectTier {
			entryCfg.Tier = client.DetectTier(ctx)
		}

		if needScopes {
			// The PAT self endpoint does not answer for an OAuth access
			// token, which is why the caller may hand the scopes in: in
			// oauth mode they were already resolved to verify the token,
			// and asking GitLab a second question it cannot answer would
			// only lose the answer.
			knownScopes = gitlabclient.DetectScopes(ctx, client.GL())
		}
	}
	if p.cfg.IgnoreScopes {
		return entryCfg
	}
	entryCfg.TokenScopes = knownScopes
	applyScopeReadOnly(entryCfg)
	return entryCfg
}

// applyScopeReadOnly narrows an entry to read-only when its token cannot
// write, which is what makes the write check a property of the action rather
// than of the deployment.
//
// A deployment that serves writes had to demand a write-capable token from
// everyone, because the only check ran at the door: a read_api token was
// refused at initialize, before it could so much as list the tools it was
// perfectly entitled to call. The tools themselves already carry the
// distinction — every action declares whether it mutates, and --read-only
// already projects a catalog from it — so the entry a read-only token gets
// is simply that catalog. Nothing new decides what may write; the existing
// decision is moved to where the authority is actually known.
//
// The narrowing is per pool entry, and an entry is per token, so one client's
// read_api token cannot narrow another client's api token.
func applyScopeReadOnly(entryCfg *config.ServerConfig) {
	gitlabclient.NarrowToTokenScope(entryCfg)
}

// tierSource returns the label used in logs for how the licensing tier was
// selected for new pool entries.
func (p *ServerPool) tierSource() string {
	if p.cfg.TierExplicit {
		return "configured"
	}
	return "detected"
}

// Size returns the current number of entries in the pool.
func (p *ServerPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}

// Stats returns a point-in-time [Snapshot] of pool metrics and state.
func (p *ServerPool) Stats() Snapshot {
	p.mu.RLock()
	size := len(p.entries)
	p.mu.RUnlock()

	return Snapshot{
		Hits:                        p.metrics.Hits.Load(),
		Misses:                      p.metrics.Misses.Load(),
		Evictions:                   p.metrics.Evictions.Load(),
		IdleEvictions:               p.metrics.IdleEvictions.Load(),
		RevalidationsFailed:         p.metrics.RevalidationsFailed.Load(),
		RevalidationsSucceeded:      p.metrics.RevalidationsSucceeded.Load(),
		RevalidationsTransient:      p.metrics.RevalidationsTransient.Load(),
		StaleCredentialEvictions:    p.metrics.StaleCredentialEvictions.Load(),
		RejectedCredentialEvictions: p.metrics.RejectedCredentialEvictions.Load(),
		CurrentSize:                 size,
		MaxSize:                     p.maxSize,
		CreatedAt:                   p.createdAt,
	}
}

// Close removes all entries from the pool. Active MCP sessions for evicted
// servers are not forcefully terminated — they will expire naturally via
// [StreamableHTTPOptions.SessionTimeout].
func (p *ServerPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.entries {
		p.dropEntry(key)
	}
	p.lru.Init()
	slog.Info("server pool: closed all entries")
}

// dropEntry removes one entry and notifies the eviction callback. It is the
// only place an entry leaves p.entries, so a new removal path cannot silently
// skip the notification. Must be called with the write lock held; the caller
// remains responsible for the LRU list, which differs per path.
func (p *ServerPool) dropEntry(key string) *Entry {
	entry, ok := p.entries[key]
	if !ok {
		return nil
	}
	delete(p.entries, key)
	if p.onEvict != nil {
		p.onEvict(entry)
	}
	return entry
}

// evictLRU removes the least recently used entry that is not doing work of its
// own, falling back to the least recently used of all when every entry is.
// Must be called with write lock held.
//
// Skipping the busy ones is what makes [WithInUse] a protection rather than a
// delay. It used to take the tail unconditionally, which handed size pressure
// exactly the entries the idle sweep had just decided to keep: a credential
// whose only activity is an open subscriptions/listen refreshes nothing here,
// so it sits at the tail, and any caller could evict every quiet subscriber in
// the pool by presenting --max-http-clients credentials of its own, repeatably.
// The idle sweep declining to evict those entries an hour in was not much of a
// protection when a stranger could evict them in a second.
//
// The fallback is what keeps the pool bounded. An entry is only skipped in
// favour of another one, never in favour of growing past --max-http-clients, so
// a pool in which everything is busy still evicts its oldest — the case
// TestSharedServer_AnEvictedCredentialsListenIsEnded drives with a maximum of
// one. What a credential is told when that happens is [WithOnEvict]'s job.
//
// The scan costs one map lookup and one callback per entry it passes, under the
// write lock, and only when a full pool takes a new credential. It is short in
// practice because a kept entry is moved to the front by [ServerPool.evictIdle],
// so busy entries drift away from the tail rather than accumulating at it.
func (p *ServerPool) evictLRU() {
	victim := p.lruVictimLocked()
	if victim == nil {
		return
	}
	key, _ := victim.Value.(string)
	if entry := p.dropEntry(key); entry != nil {
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.metrics.Evictions.Add(1)
		slog.Info(
			"server pool: evicted LRU entry",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
	p.lru.Remove(victim)
}

// lruVictimLocked picks the element size pressure should drop: the least
// recently used entry the caller does not report as busy, or the tail when
// every entry is busy. Callers hold p.mu.
func (p *ServerPool) lruVictimLocked() *list.Element {
	back := p.lru.Back()
	if back == nil || p.inUse == nil {
		return back
	}
	for element := back; element != nil; element = element.Prev() {
		key, _ := element.Value.(string)
		// An element naming no entry is already stale, so dropping it costs
		// nobody anything and tidies the list.
		if entry, ok := p.entries[key]; !ok || !p.inUse(entry) {
			return element
		}
	}
	return back
}

// tokenHash returns a hex-encoded SHA-256 hash of the token.
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// sessionKey returns a hex-encoded SHA-256 hash of the token combined with
// the GitLab URL. This ensures the same token against different GitLab
// instances results in separate pool entries.
func sessionKey(token, gitlabURL string) string {
	h := sha256.Sum256([]byte(token + "\x00" + gitlabURL))
	return hex.EncodeToString(h[:])
}

// tokenSuffix returns the last 4 characters of the token for safe logging.
func tokenSuffix(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return "..." + token[len(token)-4:]
}

// StartIdleEviction launches a background goroutine that reclaims entries
// unused for longer than the configured idle timeout. Cancel the context to
// stop it. It is a no-op when idle eviction is disabled.
//
// Idle eviction runs independently of revalidation so that disabling one does
// not silently disable the other. It also needs no network I/O: an idle entry
// is dropped on its timestamp alone, which is the point — the entries it
// reclaims are exactly the ones revalidation would otherwise keep pinging
// GitLab about on behalf of a client that is gone.
func (p *ServerPool) StartIdleEviction(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background() //nolint:contextcheck // defensive: nil-ctx guard for callers that pass uninitialized context
	}
	if p.idleTimeout <= 0 {
		slog.InfoContext(ctx, "server pool: idle eviction disabled")
		return
	}

	interval := max(p.idleTimeout/idleSweepDivisor, idleSweepMinInterval)
	slog.InfoContext(ctx, "server pool: starting idle eviction",
		"idle_timeout", p.idleTimeout, "sweep_interval", interval)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "server pool: idle eviction goroutine panicked", "panic", r)
			}
		}()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "server pool: idle eviction stopped")
				return
			case <-ticker.C:
				p.evictIdle()
			}
		}
	}()
}

// evictIdle removes every entry whose last use is older than the idle timeout
// and that the caller does not report as still in use.
//
// Eviction ends what the entry owns: [WithOnEvict] is where the caller stops
// the credential's watchers and closes the streams it holds open, because the
// server that answered for it is shared and the entry is what said which
// credential a session belonged to. It is not the reference-drop it used to be
// when each credential had a server to itself.
//
// Which is why "idle" cannot be read off lastUsed alone. That timestamp is
// refreshed by pool hits, and a credential whose only activity is an open
// subscriptions/listen never produces one: its watcher polls GitLab directly.
// After --pool-idle-timeout such a client looked exactly like an abandoned one.
// [WithInUse] is how the caller answers that. It is consulted by
// [ServerPool.evictLRU] as well, so size pressure cannot take back what this
// sweep grants; only a credential GitLab has refused is evicted regardless.
func (p *ServerPool) evictIdle() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.idleTimeout <= 0 {
		return
	}
	cutoff := time.Now().Add(-p.idleTimeout)
	for key, entry := range p.entries {
		if entry.lastUsed.After(cutoff) {
			continue
		}
		if p.inUse != nil && p.inUse(entry) {
			// Kept, and its clock restarted: an entry doing work the pool
			// cannot see is not idle, and rechecking it every sweep would
			// otherwise cost a callback per sweep for as long as the work runs.
			//
			// Moved in the LRU as well, because the pool keeps two clocks and
			// this decision has to reach both. Restarting lastUsed alone left
			// the entry where it was — at the tail, since a subscription
			// refreshes nothing here — so the sweep protected it and size
			// pressure took it first. [ServerPool.evictLRU] is what enforces
			// the decision; this is what keeps the ordering honest, and what
			// keeps that scan short.
			entry.lastUsed = time.Now()
			p.lru.MoveToFront(entry.element)
			continue
		}
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		p.dropEntry(key)
		p.metrics.IdleEvictions.Add(1)
		slog.Info(
			"server pool: evicted idle entry",
			"pool_size", len(p.entries),
			"idle_for", time.Since(entry.lastUsed).Round(time.Second),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
}

// StartRevalidation launches a background goroutine that periodically
// checks all pool entries for token validity using a lightweight GitLab API
// call. Entries that fail validation are evicted. Cancel the context to stop.
func (p *ServerPool) StartRevalidation(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background() //nolint:contextcheck // defensive: nil-ctx guard for callers that pass uninitialized context
	}

	if p.revalidateInterval <= 0 {
		slog.InfoContext(ctx, "server pool: token revalidation disabled")
		return
	}

	slog.InfoContext(ctx, "server pool: starting token revalidation", "interval", p.revalidateInterval)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "server pool: revalidation goroutine panicked", "panic", r)
			}
		}()

		ticker := time.NewTicker(p.revalidateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "server pool: revalidation stopped")
				return
			case <-ticker.C:
				p.revalidateAll(ctx)
			}
		}
	}()
}

// revalidateAll checks each pool entry's token by calling the GitLab version
// endpoint. Entries GitLab refuses are evicted; entries whose check could not
// reach a verdict are left alone.
//
// The classification matters as much as the check. Evicting on any error at
// all makes a GitLab that is briefly unreachable, or answers 500 for ten
// seconds, drop every tenant's entry at once — and each is then rebuilt on its
// next request with a fresh credential probe, tier lookup, scope lookup and
// identity lookup, which is a thundering herd against an instance that has
// only just come back. [verifyCredential] is careful about exactly this
// distinction at admission time, and this path now inherits it.
func (p *ServerPool) revalidateAll(ctx context.Context) {
	p.mu.RLock()
	snapshot := make(map[string]*Entry, len(p.entries))
	maps.Copy(snapshot, p.entries)
	p.mu.RUnlock()

	for key, entry := range snapshot {
		if ctx.Err() != nil {
			return
		}

		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := entry.client.Ping(checkCtx)
		cancel()

		if err != nil {
			if !gitlabclient.IsCredentialRejection(err) {
				slog.WarnContext(ctx,
					"server pool: token revalidation could not reach a verdict, keeping entry",
					"error", err,
					"age", time.Since(entry.createdAt).Round(time.Second),
				)
				p.metrics.RevalidationsTransient.Add(1)
				continue
			}
			slog.WarnContext(ctx,
				"server pool: gitlab rejected a pooled credential, evicting entry",
				"error", err,
				"age", time.Since(entry.createdAt).Round(time.Second),
			)
			p.metrics.RevalidationsFailed.Add(1)
			p.evictByKey(key)
		} else {
			p.metrics.RevalidationsSucceeded.Add(1)
			p.mu.Lock()
			if e, ok := p.entries[key]; ok {
				e.lastValidated = time.Now()
			}
			p.mu.Unlock()
		}
	}
}

// evictStaleCredential removes an entry whose credential has not been checked
// against GitLab inside [ServerPool.maxCredentialAge], counting it apart from
// the evictions that mean GitLab said no.
func (p *ServerPool) evictStaleCredential(key string) {
	p.mu.Lock()
	entry, ok := p.entries[key]
	// Re-checked under the write lock, not just under the read lock that
	// spotted it: a concurrent request may already have rebuilt this key
	// between the two, and dropping that entry would throw away a check made
	// a moment ago and send the next caller round again.
	if !ok || p.maxCredentialAge <= 0 || time.Since(entry.lastValidated) <= p.maxCredentialAge {
		p.mu.Unlock()
		return
	}
	gitlabURL, _ := entryConfigLogValues(entry)
	age := time.Since(entry.lastValidated).Round(time.Second)
	p.lru.Remove(entry.element)
	p.dropEntry(key)
	p.metrics.StaleCredentialEvictions.Add(1)
	p.mu.Unlock()

	slog.Info(
		"server pool: credential not re-checked within the ceiling, rebuilding entry",
		"unverified_for", age,
		"ceiling", p.maxCredentialAge,
		"gitlab_url", gitlabURL,
	)
}

// evictByKey removes the entry with the given key from the pool.
func (p *ServerPool) evictByKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		p.dropEntry(key)
		p.metrics.Evictions.Add(1)
		slog.Info(
			"server pool: evicted invalid entry",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
}

// EvictServer removes every entry served by srv, and reports whether it found
// any.
//
// It exists for a build that fails after the entry is already cached. A server
// whose catalog registration ran in the background and failed is not usable and
// must not be handed to the next request for that credential: the pool would
// otherwise serve the poisoned entry until an idle timeout or a revalidation
// happened to replace it, which is an hour by default. Dropping it makes the
// next request rebuild, which is what a synchronous failure already does.
//
// Every entry rather than the first: one server now answers for every
// credential of a configuration shape, and a registration that failed failed
// for all of them. Stopping at the first match would leave the others holding a
// server with no tools, which is the exact condition this exists to clear.
//
// The scan is linear over the pool, which is bounded by --max-http-clients and
// only walked when a registration has failed, so it is not on any hot path.
func (p *ServerPool) EvictServer(srv *mcp.Server) bool {
	if srv == nil {
		return false
	}
	// Held across the search AND the removal. Releasing between the two would
	// let a replacement entry be built for the same key and then delete that
	// replacement instead: its sessions would go with it, and its own stateful
	// requests would start being refused as if they belonged to somebody else.
	p.mu.Lock()
	defer p.mu.Unlock()
	evicted := false
	for key, entry := range p.entries {
		if entry == nil || entry.server != srv {
			continue
		}
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		p.dropEntry(key)
		p.metrics.Evictions.Add(1)
		evicted = true
		slog.Info(
			"server pool: evicted entry whose build did not finish",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
	return evicted
}

// entryConfigLogValues extracts safe configuration values for eviction
// logs without requiring callers to nil-check partially initialized entries.
func entryConfigLogValues(entry *Entry) (string, bool) {
	if entry == nil || entry.serverConfig == nil {
		return "", false
	}
	return entry.serverConfig.GitLabURL, entry.serverConfig.Enterprise()
}
