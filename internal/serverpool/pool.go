package serverpool

import (
	"container/list"
	"context"
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

// poolEntry holds a server instance and its associated GitLab client,
// along with LRU tracking and session validation metadata.
type poolEntry struct {
	server        *mcp.Server
	client        *gitlabclient.Client
	serverConfig  *config.ServerConfig
	identity      UserIdentity
	element       *list.Element
	createdAt     time.Time
	lastValidated time.Time
	lastUsed      time.Time
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
}

// Snapshot is a point-in-time copy of pool [Metrics] plus current state.
// Safe for JSON serialization and cross-goroutine use.
type Snapshot struct {
	Hits                   int64     `json:"hits"`
	Misses                 int64     `json:"misses"`
	Evictions              int64     `json:"evictions"`
	IdleEvictions          int64     `json:"idle_evictions"`
	RevalidationsFailed    int64     `json:"revalidations_failed"`
	RevalidationsSucceeded int64     `json:"revalidations_succeeded"`
	CurrentSize            int       `json:"current_size"`
	MaxSize                int       `json:"max_size"`
	CreatedAt              time.Time `json:"created_at"`
}

// ServerPool maintains a bounded set of [*mcp.Server] instances keyed by
// token plus GitLab URL hash (SHA-256). When the pool reaches maxSize, the
// least recently used entry is evicted. Entries are periodically re-validated
// against the GitLab API; entries with revoked tokens are evicted automatically.
type ServerPool struct {
	mu                 sync.RWMutex
	entries            map[string]*poolEntry
	lru                *list.List
	maxSize            int
	cfg                *config.Config
	factory            ServerFactory
	revalidateInterval time.Duration
	idleTimeout        time.Duration
	metrics            Metrics
	createdAt          time.Time
	// building collapses concurrent first-requests for one key into a single
	// build, so a client opening several connections at once costs one set of
	// upstream lookups rather than one per connection.
	building singleflight.Group
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

// New creates a [ServerPool]. The cfg provides shared server-wide settings
// (GitLabURL, SkipTLSVerify, etc.). The factory function creates a fully
// registered [*mcp.Server] for each new GitLab client.
func New(cfg *config.Config, factory ServerFactory, opts ...Option) *ServerPool {
	p := &ServerPool{
		entries:            make(map[string]*poolEntry),
		lru:                list.New(),
		maxSize:            defaultMaxSize,
		cfg:                cfg,
		factory:            factory,
		revalidateInterval: DefaultRevalidateInterval,
		idleTimeout:        DefaultIdleTimeout,
		createdAt:          time.Now(),
		baseContext:        context.Background,
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
	if token == "" {
		return nil, errors.New("empty token: authentication required")
	}
	if gitlabURL == "" {
		return nil, errors.New("empty GitLab URL: set --gitlab-url or send GITLAB-URL header")
	}

	key := sessionKey(token, gitlabURL)

	// Fast path: read lock to check existing entry.
	p.mu.RLock()
	if entry, ok := p.entries[key]; ok {
		p.mu.RUnlock()
		p.mu.Lock()
		p.lru.MoveToFront(entry.element)
		// This is the hot path for every request on an established entry, so
		// it is what keeps an active entry out of reach of idle eviction.
		entry.lastUsed = time.Now()
		p.mu.Unlock()
		p.metrics.Hits.Add(1)
		return entry.server, nil
	}
	p.mu.RUnlock()

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
	server, ok := built.(*mcp.Server)
	if !ok || server == nil {
		return nil, errors.New("creating MCP server for pool: builder returned no server")
	}
	return server, nil
}

// buildEntry creates the GitLab client, resolves the per-entry configuration,
// and builds the MCP server for a new pool key. It performs network I/O and must
// be called without holding p.mu. The returned entry has no LRU element or
// timestamps yet; insertEntry finalizes those under the lock.
func (p *ServerPool) buildEntry(token, gitlabURL string, knownScopes []string) (*poolEntry, error) {
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
	newClient := gitlabclient.NewClientWithToken
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

	if verifyErr := verifyCredential(p.lifetime(), client); verifyErr != nil {
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

	return &poolEntry{
		server:       server,
		client:       client,
		serverConfig: entryCfg,
		identity:     resolveIdentity(p.lifetime(), client),
	}, nil
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
// already-stored server is returned and the freshly built one is discarded — its
// server holds no live sessions, mirroring [ServerPool.Close], which lets
// servers expire naturally rather than terminating them.
func (p *ServerPool) insertEntry(key, token string, entry *poolEntry) *mcp.Server {
	p.mu.Lock()
	defer p.mu.Unlock()

	if server, ok := p.existingServerLocked(key); ok {
		return server
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

	return entry.server
}

// existingServerLocked returns an existing server for key while p.mu is held.
// Every hit refreshes lastUsed, which is what idle eviction reads; the LRU
// position alone cannot serve that purpose because it only orders entries
// relative to each other and carries no wall-clock age.
//
// It counts nothing. Its only caller is the double check in [ServerPool.insertEntry],
// which is reached from the slow path where the miss has already been charged;
// counting a hit here too would make one call show up as both, and Hits plus
// Misses would stop being the number of calls.
func (p *ServerPool) existingServerLocked(key string) (*mcp.Server, bool) {
	entry, ok := p.entries[key]
	if !ok {
		return nil, false
	}
	p.lru.MoveToFront(entry.element)
	entry.lastUsed = time.Now()
	return entry.server, true
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
	// pin it explicitly via --tier/GITLAB_TIER.
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
	if entryCfg.ReadOnly || gitlabclient.WriteCapable(entryCfg.TokenScopes) {
		return
	}
	entryCfg.ReadOnly = true
	slog.Info("token cannot write; serving a read-only tool surface for it",
		"scopes", entryCfg.TokenScopes)
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
		Hits:                   p.metrics.Hits.Load(),
		Misses:                 p.metrics.Misses.Load(),
		Evictions:              p.metrics.Evictions.Load(),
		IdleEvictions:          p.metrics.IdleEvictions.Load(),
		RevalidationsFailed:    p.metrics.RevalidationsFailed.Load(),
		RevalidationsSucceeded: p.metrics.RevalidationsSucceeded.Load(),
		CurrentSize:            size,
		MaxSize:                p.maxSize,
		CreatedAt:              p.createdAt,
	}
}

// Close removes all entries from the pool. Active MCP sessions for evicted
// servers are not forcefully terminated — they will expire naturally via
// [StreamableHTTPOptions.SessionTimeout].
func (p *ServerPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.entries {
		delete(p.entries, key)
	}
	p.lru.Init()
	slog.Info("server pool: closed all entries")
}

// evictLRU removes the least recently used entry. Must be called with
// write lock held.
func (p *ServerPool) evictLRU() {
	back := p.lru.Back()
	if back == nil {
		return
	}
	key, _ := back.Value.(string)
	if entry, ok := p.entries[key]; ok {
		gitlabURL, enterprise := poolEntryConfigLogValues(entry)
		delete(p.entries, key)
		p.metrics.Evictions.Add(1)
		slog.Info(
			"server pool: evicted LRU entry",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
	p.lru.Remove(back)
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
		slog.Info("server pool: idle eviction disabled")
		return
	}

	interval := max(p.idleTimeout/idleSweepDivisor, idleSweepMinInterval)
	slog.Info("server pool: starting idle eviction",
		"idle_timeout", p.idleTimeout, "sweep_interval", interval)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("server pool: idle eviction goroutine panicked", "panic", r)
			}
		}()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("server pool: idle eviction stopped")
				return
			case <-ticker.C:
				p.evictIdle()
			}
		}
	}()
}

// evictIdle removes every entry whose last use is older than the idle timeout.
// Eviction only drops the pool's reference: live sessions already holding the
// server keep working, mirroring [ServerPool.Close].
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
		gitlabURL, enterprise := poolEntryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		delete(p.entries, key)
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
		slog.Info("server pool: token revalidation disabled")
		return
	}

	slog.Info("server pool: starting token revalidation", "interval", p.revalidateInterval)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("server pool: revalidation goroutine panicked", "panic", r)
			}
		}()

		ticker := time.NewTicker(p.revalidateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("server pool: revalidation stopped")
				return
			case <-ticker.C:
				p.revalidateAll(ctx)
			}
		}
	}()
}

// revalidateAll checks each pool entry's token by calling the GitLab version
// endpoint. Entries that fail are evicted.
func (p *ServerPool) revalidateAll(ctx context.Context) {
	p.mu.RLock()
	snapshot := make(map[string]*poolEntry, len(p.entries))
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
			slog.Warn(
				"server pool: token revalidation failed, evicting entry",
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

// evictByKey removes the entry with the given key from the pool.
func (p *ServerPool) evictByKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		gitlabURL, enterprise := poolEntryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		delete(p.entries, key)
		p.metrics.Evictions.Add(1)
		slog.Info(
			"server pool: evicted invalid entry",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
}

// poolEntryConfigLogValues extracts safe configuration values for eviction
// logs without requiring callers to nil-check partially initialized entries.
func poolEntryConfigLogValues(entry *poolEntry) (string, bool) {
	if entry == nil || entry.serverConfig == nil {
		return "", false
	}
	return entry.serverConfig.GitLabURL, entry.serverConfig.Enterprise()
}
