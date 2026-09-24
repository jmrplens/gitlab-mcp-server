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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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
	// rejected is set the moment GitLab has refused this entry's credential,
	// by a 401 on a call that named the credential or by the probe that
	// confirmed a 401 naming nothing, before the eviction that follows has
	// taken the lock, so a request that finds the entry in between rebuilds
	// instead of reusing a credential GitLab has already refused.
	rejected atomic.Bool
	// lastConfirmProbe is when this entry last claimed the right to confirm a
	// 401 that named no cause, as the [time.Now] reading the claim was made
	// at, monotonic reading included, and nil until the first.
	// It is apart from lastValidated on purpose: that one is set when the entry
	// is built, on every successful revalidation and by a confirmation GitLab
	// accepted ([ServerPool.keepConfirmedEntry]), so measuring the window
	// from it would skip the confirmation of every refusal in the first
	// window after either, which is where a token deleted in the meantime is
	// most likely to be refused first. See [Entry.claimConfirmation].
	lastConfirmProbe atomic.Pointer[time.Time]
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
	Hits   atomic.Int64
	Misses atomic.Int64
	// Evictions is the legacy total, and it overlaps SizeEvictions,
	// BusyEvictions, InvalidEvictions and RebuildEvictions rather than
	// complementing them: it counts all four together and always has. Keep it
	// for the callers and assertions that already read it, and never export it
	// as a series beside the four, which would double every eviction it covers.
	Evictions atomic.Int64
	// SizeEvictions and BusyEvictions split size pressure by what it took.
	// SizeEvictions counts the ordinary case, where the scan found an entry
	// doing no work of its own; BusyEvictions counts the fallback, where every
	// pooled entry was busy and the least recently used of them went anyway.
	// The second is the one an operator wants to see, because it is the only
	// path that ends a subscription somebody is waiting on.
	SizeEvictions atomic.Int64
	BusyEvictions atomic.Int64
	// InvalidEvictions counts entries dropped by [ServerPool.evictByKey], which
	// is the periodic revalidation finding that GitLab now refuses the
	// credential.
	InvalidEvictions atomic.Int64
	// RebuildEvictions counts entries dropped because a configuration shape's
	// catalog registration failed, taking every credential pointing at it:
	// [ServerPool.EvictServer] for the entries already pooled when the failure
	// landed, and [ServerPool.dropRefusedEntry] for one inserted after it,
	// which the insert callback refuses.
	RebuildEvictions       atomic.Int64
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
	// refused their credential on a call: a 401 that named the credential, or
	// a 401 that named nothing and that the credential probe then confirmed.
	// The token was revoked, expired or deleted while the entry was live, or
	// the GraphQL endpoint refused it for its scope (it answers 401 to a token
	// carrying neither api nor read_api, which legacy admission lets through
	// on read_user alone), and the first refused data call is the signal
	// rather than the next periodic check. A permission refusal answered with
	// 401 is not counted here, since the probe finds the credential accepted;
	// see UnauthorizedKept.
	RejectedCredentialEvictions atomic.Int64
	// UnauthorizedKept counts 401s that named no cause and after which the
	// credential probe found GitLab still accepting the credential, so the
	// entry was kept. Each is a permission refusal GitLab answered with 401
	// rather than 403 (entry 55 of docs/development/upstream-bugs.md), and
	// before the probe each one ended a valid credential's entry.
	//
	// It is read in the snapshot and exported nowhere else, like
	// RevalidationsTransient: it is not an eviction, so it belongs to none of
	// the series the eviction reason attribute splits.
	UnauthorizedKept atomic.Int64
}

// Snapshot is a point-in-time copy of pool [Metrics] plus current state.
// Safe for JSON serialization and cross-goroutine use.
type Snapshot struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
	// Evictions is the legacy total described on [Metrics.Evictions]: it
	// overlaps SizeEvictions, BusyEvictions, InvalidEvictions and
	// RebuildEvictions, so a reader graphing the four must leave this one out.
	Evictions int64 `json:"evictions"`
	// SizeEvictions and BusyEvictions split size pressure by whether the entry
	// it took was doing work of its own. See [Metrics.SizeEvictions].
	SizeEvictions int64 `json:"size_evictions"`
	BusyEvictions int64 `json:"busy_evictions"`
	// InvalidEvictions counts entries dropped when revalidation found GitLab
	// refusing the credential; RebuildEvictions counts entries dropped with a
	// configuration shape whose registration failed.
	InvalidEvictions         int64 `json:"invalid_evictions"`
	RebuildEvictions         int64 `json:"rebuild_evictions"`
	IdleEvictions            int64 `json:"idle_evictions"`
	RevalidationsFailed      int64 `json:"revalidations_failed"`
	RevalidationsSucceeded   int64 `json:"revalidations_succeeded"`
	RevalidationsTransient   int64 `json:"revalidations_transient"`
	StaleCredentialEvictions int64 `json:"stale_credential_evictions"`
	// RejectedCredentialEvictions counts entries dropped because GitLab
	// refused their credential on a call, by a 401 naming it or by a 401 the
	// credential probe then confirmed.
	RejectedCredentialEvictions int64 `json:"rejected_credential_evictions"`
	// UnauthorizedKept counts 401s that named no cause after which the
	// credential probe found the credential still accepted, and the entry was
	// kept. See [Metrics.UnauthorizedKept].
	UnauthorizedKept int64     `json:"unauthorized_kept"`
	CurrentSize      int       `json:"current_size"`
	MaxSize          int       `json:"max_size"`
	CreatedAt        time.Time `json:"created_at"`
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
	// onInsert is called with each entry the pool has just cached, and answers
	// whether the entry may stay. See [WithOnInsert].
	onInsert func(*Entry) bool
	// onEvict is called with a server the pool has just stopped owning, for
	// every removal path: LRU pressure, idle reclamation, revalidation of a
	// revoked token, and Close. It exists because a caller that keeps its own
	// per-server state — cmd/server maps each pooled server to the tag its
	// session IDs carry — otherwise has no way to learn that an entry is gone,
	// and its map grows past the pool's own size bound.
	//
	// It runs while the pool's write lock is held, so it must be cheap and
	// must not call back into the pool.
	onEvict func(*Entry, EvictionCause)
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
	// confirmCooldown is how long after one confirmation of an unexplained 401
	// an entry may start the next, set by [New] to
	// [unauthorizedConfirmCooldown]. A field rather than the constant read in
	// place, so a test can put both sides of the window within reach.
	confirmCooldown time.Duration
	// idleSweepInterval overrides the cadence derived from idleTimeout.
	//
	// Zero, which is what [New] leaves, means derive it: a quarter of the
	// idle timeout, floored at [idleSweepMinInterval] so an operator's short
	// timeout cannot turn the sweep into a busy loop. That floor is also why
	// this field exists: it puts the first tick a minute away, which is
	// longer than any test may wait, so the sweep goroutine's own body is
	// only reachable by shortening the cadence here.
	idleSweepInterval time.Duration
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

// EvictionCause says which removal path dropped an entry.
//
// It exists because "the entry is gone" is not enough for the caller to tell
// its client anything useful: a credential taken for size pressure is still
// valid and should reconnect at once, one GitLab has refused must
// re-authenticate first, and one dropped at shutdown should look for another
// instance. Without a cause, cmd/server could only say the same thing to all
// three, and it said the first.
//
// The values are the strings the eviction metric already labels its series
// with, so the log line, the counter and the callback name one path one way.
// [CausePoolClosed] is the exception, having no metric: an eviction at
// shutdown is counted by nobody, since nothing observes a metric after the
// process ends.
type EvictionCause string

// The causes, one per call site of [ServerPool.dropEntry]. A new removal path
// picks one of these or adds its own; what it must not do is leave the zero
// value, which names no path and would reach the caller as an ending it cannot
// explain.
//
//nolint:gosec // G101 fires on the identifiers containing "Credential"; these name why an entry was dropped and no credential is anywhere near them.
const (
	// CauseSizePressure is a full pool taking a new credential. The entry it
	// took may or may not have been busy, and that difference is deliberately
	// not a second cause: it is the pool's own state, not this credential's,
	// and it changes nothing about what the client should do next.
	CauseSizePressure EvictionCause = "size_pressure"
	// CauseIdle is the idle sweep reclaiming an entry nobody has used.
	CauseIdle EvictionCause = "idle"
	// CauseStaleCredential is an entry whose credential has not been checked
	// against GitLab inside the ceiling, so it is rebuilt rather than trusted.
	CauseStaleCredential EvictionCause = "stale_credential"
	// CauseRejectedCredential is GitLab refusing the entry's credential on a
	// call: a 401 that named the credential, or a 401 that named nothing and
	// that the credential probe then confirmed. A permission refusal answered
	// with 401 never produces it, which is what keeps the "re-authenticate"
	// ending it maps to true.
	CauseRejectedCredential EvictionCause = "rejected_credential"
	// CauseInvalidCredential is the periodic revalidation finding that GitLab
	// now refuses the credential.
	CauseInvalidCredential EvictionCause = "invalid_credential"
	// CauseRebuild is a configuration shape whose catalog registration failed,
	// taking every credential pointing at it.
	CauseRebuild EvictionCause = "rebuild"
	// CausePoolClosed is the pool shutting down.
	CausePoolClosed EvictionCause = "pool_closed"
)

// WithOnEvict registers a callback invoked with each entry the pool removes,
// and the cause that removed it.
//
// It takes the entry rather than its server because a server is shared by every
// entry of one configuration shape: told only "this server is gone" a caller
// would drop state belonging to credentials that are still pooled.
//
// The cause is what the caller turns into a reason for the client, so a removal
// path added later has to pick one of the [EvictionCause] values rather than
// leave the zero value: an unnamed cause reaches a subscriber as an ending
// nobody can explain, and the wrong named one tells it to do the wrong thing.
//
// The callback runs under the pool's write lock: it must not block and must not
// re-enter the pool.
func WithOnEvict(fn func(*Entry, EvictionCause)) Option {
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
// cached, once it is reachable by key, and reporting whether the entry may
// stay.
//
// It exists for work that must not start before the entry can be found again.
// A server whose catalog is registered in the background is the case: if that
// registration fails, the failure has to remove the entry, and a factory that
// started it would be racing its own insertion. The callback runs under the
// pool's write lock, so like [WithOnEvict] it must not block and must not
// re-enter the pool; starting a goroutine is what it is for.
//
// Returning false is how a callback that has just learned the entry is unusable
// gets it out of the pool without re-entering it: the insertion is undone under
// the lock it is already holding, before [ServerPool.GetOrCreateEntry] returns,
// so the request that built the entry cannot answer its client while a poisoned
// entry is still cached for the client's retry to find. Evicting from a
// goroutine instead left exactly that window open. The entry is still handed
// back to the caller that built it, since it is the only thing to answer that
// one request with; it is simply not pooled, so the next request rebuilds.
func WithOnInsert(fn func(*Entry) bool) Option {
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
		confirmCooldown:    unauthorizedConfirmCooldown,
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
	//
	// Released explicitly rather than with defer, unlike every other lock site
	// in this file, and it has to be: both arms of the switch below take the
	// write lock — dropRejectedEntry directly, the stale arm through the slow
	// path — and a read lock still held there deadlocks the process. The block
	// under it is three field reads that cannot panic, so defer buys nothing
	// here and costs the function.
	p.mu.RLock()
	cached, ok := p.entries[key]
	// Read under the same lock that guards the field: lastValidated is
	// written by the revalidation and confirmation goroutines.
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
	return entryFromBuild(built)
}

// entryFromBuild converts what the shared build returned into an entry.
//
// [singleflight.Group.Do] hands back an any, so this is the boundary where a
// build result stops being typed. Everything above it returns an entry or an
// error, which is why the failure it describes cannot be produced from here;
// it exists so that a future build path returning nothing is a refused
// request that names itself rather than a nil dereference in the caller,
// which would be an HTTP 500 with a stack trace and no cause.
func entryFromBuild(built any) (*Entry, error) {
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
	// A deployment that publishes no instance is --allow-any-gitlab-url, so
	// this URL came out of a caller's GITLAB-URL header rather than out of the
	// operator's configuration. The client is the only place that can tell the
	// difference, and it cannot tell it from the string. See ADR-0022.
	if len(p.cfg.InstanceURLs()) == 0 {
		client.MarkInstanceCallerNamed()
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
	// relayed and refused one by one. See [ServerPool.handleUnauthorized] for
	// why not every 401 is one.
	key := sessionKey(token, gitlabURL)
	client.SetOnUnauthorized(func(answer gitlabclient.UnauthorizedAnswer) {
		p.handleUnauthorized(key, entry, answer)
	})
	return entry, nil
}

// unauthorizedConfirmCooldown is how long after one confirmation of a 401 that
// named no cause an entry may start another.
//
// Without it, a client retrying a call GitLab refuses a permission for turns
// every refusal into a second request, a GET /user, and roughly doubles what
// it costs the instance. Thirty seconds bounds that to one probe per
// credential per window, while a token deleted just after a confirmation is
// still caught by the next refused call once the window has passed, or by the
// periodic revalidation, whichever comes first.
//
// It also has to outlast the probe itself, which [credentialCheckTimeout]
// bounds: the claim that opens the window is what keeps two refusals arriving
// together from confirming twice, so a window shorter than the probe would let
// the second refusal start its own while the first was still waiting.
const unauthorizedConfirmCooldown = 30 * time.Second

// handleUnauthorized decides what a 401 on a call made with entry's credential
// means for the entry.
//
// GitLab answers 401 for two different things. A 401 naming the credential
// (the RFC 6750 invalid_token code, or any 401 from the GraphQL endpoint) is
// GitLab's verdict on the token, and the entry goes at once. A 401 naming
// nothing is ambiguous: GitLab answers a permission refusal that way at the
// sites entry 55 of docs/development/upstream-bugs.md lists, approving a merge
// request one opened among them, and answers a token it no longer finds with
// the same bytes. Evicting on it ended a valid user's subscriptions,
// terminated their sessions and told them to re-authenticate a token that
// works; ignoring it would serve a deleted token until the next revalidation.
// So it is confirmed with the probe admission already trusts, and the entry
// goes only if that probe is refused.
//
// It runs inside the refused call's RoundTrip, which must not wait on a network
// probe or on the pool's write lock, so it takes no lock and hands the work to
// a goroutine of its own. A 401 naming the credential reaches it once per
// client, so that is one goroutine per revoked credential; one naming nothing
// reaches it on every refusal, so only the one that claims the confirmation
// window starts a goroutine.
//
// The mark on a verdict is synchronous and the drop is not: between the two, a
// request racing for the same key sees the mark on the fast path and rebuilds
// rather than reusing the refused credential.
func (p *ServerPool) handleUnauthorized(key string, entry *Entry, answer gitlabclient.UnauthorizedAnswer) {
	if answer == gitlabclient.UnauthorizedCredential {
		entry.rejected.Store(true)
		go p.evictRejectedCredential(key, entry)
		return
	}
	// An entry already marked is on its way out, and asking GitLab about it
	// again would only spend a probe on a verdict that has been given.
	if entry.rejected.Load() || !entry.claimConfirmation(time.Now(), p.confirmCooldown) {
		return
	}
	go p.confirmUnexplainedRefusal(key, entry)
}

// claimConfirmation reports whether the entry may confirm an unexplained 401
// at now, and if so claims the window that starts there.
//
// The check and the claim are one compare-and-swap, which is what makes the
// confirmation a single flight: of any number of refusals arriving together,
// exactly one replaces the claim and the rest find the window taken. An entry
// starts with no claim, so the first unexplained 401 it meets always claims
// the window, and is confirmed unless no probe slot is free or the entry was
// replaced first; an entry already marked rejected claims nothing.
//
// The window is the difference of two [time.Time] values rather than of two
// Unix timestamps, because between two readings of [time.Now] that difference
// is taken on the monotonic clock, the one every other interval in the pool is
// measured on. On the wall clock, a step of the system clock back by D after a
// claim would hold the window shut for D more, and a deleted token's refusals
// would go unconfirmed until revalidation.
//
// The window is spent by the claim, not by the probe: a claim whose probe
// could not be sent (no probe slot free, or the entry replaced meanwhile)
// still waits out the cooldown, which is what keeps a burst of refusals from
// retrying the slot on every one of them.
func (e *Entry) claimConfirmation(now time.Time, cooldown time.Duration) bool {
	last := e.lastConfirmProbe.Load()
	if last != nil && now.Sub(*last) < cooldown {
		return false
	}
	return e.lastConfirmProbe.CompareAndSwap(last, &now)
}

// confirmUnexplainedRefusal asks GitLab whether it still accepts entry's
// credential after a 401 that named no cause, and acts on the answer.
//
// Refused: the 401 was a token GitLab no longer finds, and the entry goes the
// way a 401 naming the credential sends it, counted and ended as a rejected
// credential. Accepted: the 401 was a permission refusal, the entry stays, and
// the probe counts as the credential check it is. No verdict: nothing is
// learned and nothing changes, the rule [verifyCredential] applies at
// admission; the question is left to the next refusal once the window has
// passed, or to revalidation.
//
// The probe slot is taken without waiting. A confirmation only refines an
// entry already being served, while an admission is a new credential waiting
// to be served, so when GitLab is slow enough to keep every slot busy, with
// admissions or with other entries' confirmations, it is the confirmation that
// gives way.
func (p *ServerPool) confirmUnexplainedRefusal(key string, entry *Entry) {
	defer recoverSweep(context.Background(), "unauthorized confirmation")
	if !p.isCurrent(key, entry) {
		return
	}
	release, ok := p.tryProbeSlot()
	if !ok {
		slog.Debug("server pool: no credential probe slot free to confirm a 401, keeping the entry")
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(p.lifetime(), credentialCheckTimeout)
	defer cancel()
	switch entry.client.CheckCredential(ctx) {
	case gitlabclient.CredentialRefused:
		entry.rejected.Store(true)
		p.evictRejectedCredential(key, entry)
	case gitlabclient.CredentialAccepted:
		p.keepConfirmedEntry(key, entry)
	default:
		slog.Debug("server pool: could not confirm a 401 that named no cause, keeping the entry")
	}
}

// isCurrent reports whether entry is still the one pooled under key.
func (p *ServerPool) isCurrent(key string, entry *Entry) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.entries[key] == entry
}

// tryProbeSlot takes one of the [maxConcurrentCredentialProbes] slots if one
// is free right now, returning the function that gives it back.
func (p *ServerPool) tryProbeSlot() (func(), bool) {
	if p.probes == nil {
		return func() {
			// No limiter is configured, so no slot was taken and there is
			// nothing to give back.
		}, true
	}
	select {
	case p.probes <- struct{}{}:
		return func() { <-p.probes }, true
	default:
		return nil, false
	}
}

// keepConfirmedEntry records that GitLab still accepts entry's credential
// after a 401 that named no cause, if entry is still the one under key.
//
// The probe is a credential check like any other, so it moves lastValidated
// the way a successful revalidation does, and the entry's credential-age
// ceiling runs from it.
func (p *ServerPool) keepConfirmedEntry(key string, entry *Entry) {
	p.mu.Lock()
	kept := p.entries[key] == entry
	if kept {
		entry.lastValidated = time.Now()
	}
	p.mu.Unlock()
	if !kept {
		return
	}
	p.metrics.UnauthorizedKept.Add(1)
	gitlabURL, _ := entryConfigLogValues(entry)
	slog.Debug("server pool: gitlab refused a call with 401 and still accepts the credential, keeping the entry",
		"gitlab_url", gitlabURL)
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
	defer recoverSweep(context.Background(), "rejected-credential eviction")
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
	p.dropEntry(key, CauseRejectedCredential)
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

// Admitted reports whether the pool already holds a usable entry for this
// credential: one GitLab accepted when it was built, that has not since been
// refused on a call, and whose credential is still inside the revalidation
// ceiling.
//
// It answers the question [ServerPool.GetOrCreateEntry] answers on its fast
// path, and answers only that: a false here means the next call would take the
// slow path and reach GitLab, never that the credential is bad. That is what
// makes it usable as an exemption from the per-address authentication budget —
// a credential this pool is already serving costs a hash and a map read to
// recognize, and serving it again spends nothing upstream, whoever else shares
// its address. Anything this returns false for still has to earn its entry the
// ordinary way, which is what keeps a blocked address unable to spend the
// deployment's standing with GitLab.
//
// The three conditions mirror that fast path deliberately. An entry marked
// rejected or past [WithMaxCredentialAge] is dropped and rebuilt on the next
// request, and a rebuild is a round trip, so neither may be reported as
// admitted.
func (p *ServerPool) Admitted(token, gitlabURL string) bool {
	if token == "" || gitlabURL == "" {
		return false
	}
	key := sessionKey(token, gitlabURL)

	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.entries[key]
	if !ok || entry.rejected.Load() {
		return false
	}
	// Read under the same lock that guards it: lastValidated is written by the
	// revalidation and confirmation goroutines.
	return p.maxCredentialAge <= 0 || time.Since(entry.lastValidated) <= p.maxCredentialAge
}

// insertEntry commits a freshly built entry under the write lock. If another
// goroutine created an entry for the same key while this one was building, the
// already-stored entry is returned and the freshly built one is discarded: its
// server holds no live sessions, mirroring [ServerPool.Close], which lets
// servers expire naturally rather than terminating them.
//
// An entry the insert callback refuses ([WithOnInsert]) is taken back out
// before this returns, so nothing is ever cached that the caller has already
// been told is unusable. It is still returned, because the request that built
// it has to be answered with something and that answer is the refusal its
// server produces.
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
	// which may later have to evict this server can find it. A callback that
	// already knows this server is unusable says so instead, and the insertion
	// is undone here rather than by an eviction racing this caller's answer.
	if p.onInsert != nil && !p.onInsert(entry) {
		p.dropRefusedEntry(key, entry)
		return entry
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

// dropRefusedEntry undoes an insertion the insert callback refused. Callers
// hold p.mu, which is the whole point: the entry never becomes visible to a
// lookup that could hand it out.
//
// It is counted as a rebuild eviction, the same cause
// [ServerPool.EvictServer] uses, because it is the same event seen a moment
// earlier: a configuration shape whose catalog registration failed, taking a
// credential with it. Which of the two finds a given entry is decided by
// whether the failure arrived before or after this insertion, and an operator
// reading the counter should not have to know that.
func (p *ServerPool) dropRefusedEntry(key string, entry *Entry) {
	p.lru.Remove(entry.element)
	p.dropEntry(key, CauseRebuild)
	p.metrics.Evictions.Add(1)
	p.metrics.RebuildEvictions.Add(1)
	gitlabURL, enterprise := entryConfigLogValues(entry)
	slog.Warn(
		"server pool: dropped a new entry whose server was already unusable",
		"pool_size", len(p.entries),
		"gitlab_url", gitlabURL,
		"enterprise", enterprise,
	)
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

// maxConcurrentCredentialProbes is how many credential probes the pool runs at
// once: admission's, through [verifyCredential], and the confirmations of 401s
// that named no cause, through [ServerPool.confirmUnexplainedRefusal].
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
// A bounded wait rather than an immediate refusal: a build is a new credential
// waiting to be served, so a legitimate burst should be served late rather
// than refused, and the wait is short enough that a caller sees a retryable
// answer well inside any sane client timeout. Only builds wait. A confirmation
// takes a slot without waiting and does without one when none is free
// ([ServerPool.tryProbeSlot]): no confirmation ever queues ahead of a build,
// and a slot one holds is back within that one probe's [credentialCheckTimeout].
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
// probeWait is how long a build waits for a probe slot before giving up.
//
// A pool assembled outside [New] carries no timeout, and a zero one would
// make the wait expire the instant it started: the queue is not full most of
// the time, so the caller would win the race about half the time and be told
// the queue is saturated the rest, which is the least useful of the three
// possible behaviors.
func (p *ServerPool) probeWait() time.Duration {
	if p.probeQueueTimeout <= 0 {
		return credentialProbeQueueTimeout
	}
	return p.probeQueueTimeout
}

func (p *ServerPool) acquireProbeSlot() (func(), error) {
	if p.probes == nil {
		return func() {
			// No limiter is configured, so no slot was taken and there is
			// nothing to give back. Returning a no-op rather than nil keeps
			// every caller's deferred release unconditional.
		}, nil
	}
	wait := p.probeWait()
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

// credentialCheckTimeout bounds the GET /user probe, in verifyCredential and in
// the confirmation of a 401 that named no cause. The first runs once per new
// pool entry and the second at most once per [unauthorizedConfirmCooldown] per
// entry, neither per request, and the probe does not retry, so this is a
// ceiling on a single round trip rather than on a retry budget.
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
		SizeEvictions:               p.metrics.SizeEvictions.Load(),
		BusyEvictions:               p.metrics.BusyEvictions.Load(),
		InvalidEvictions:            p.metrics.InvalidEvictions.Load(),
		RebuildEvictions:            p.metrics.RebuildEvictions.Load(),
		IdleEvictions:               p.metrics.IdleEvictions.Load(),
		RevalidationsFailed:         p.metrics.RevalidationsFailed.Load(),
		RevalidationsSucceeded:      p.metrics.RevalidationsSucceeded.Load(),
		RevalidationsTransient:      p.metrics.RevalidationsTransient.Load(),
		StaleCredentialEvictions:    p.metrics.StaleCredentialEvictions.Load(),
		RejectedCredentialEvictions: p.metrics.RejectedCredentialEvictions.Load(),
		UnauthorizedKept:            p.metrics.UnauthorizedKept.Load(),
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
		p.dropEntry(key, CausePoolClosed)
	}
	p.lru.Init()
	slog.Info("server pool: closed all entries")
}

// dropEntry removes one entry and notifies the eviction callback with the cause
// that removed it. It is the only place an entry leaves p.entries, so a new
// removal path cannot silently skip the notification, and taking the cause as
// an argument is what stops one being added without saying what it is. Must be
// called with the write lock held; the caller remains responsible for the LRU
// list, which differs per path.
func (p *ServerPool) dropEntry(key string, cause EvictionCause) *Entry {
	entry, ok := p.entries[key]
	if !ok {
		return nil
	}
	delete(p.entries, key)
	if p.onEvict != nil {
		p.onEvict(entry, cause)
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
// favor of another one, never in favor of growing past --max-http-clients, so
// a pool in which everything is busy still evicts its oldest, which is the case
// TestSharedServer_AnEvictedCredentialsListenIsEnded drives with a maximum of
// one. What a credential is told when that happens is [WithOnEvict]'s job.
//
// The scan costs one map lookup and one callback per entry it passes, under the
// write lock, and only when a full pool takes a new credential. It is short in
// practice because a kept entry is moved to the front by [ServerPool.evictIdle],
// so busy entries drift away from the tail rather than accumulating at it.
func (p *ServerPool) evictLRU() {
	victim, busy := p.lruVictimLocked()
	if victim == nil {
		return
	}
	key, _ := victim.Value.(string)
	if entry := p.dropEntry(key, CauseSizePressure); entry != nil {
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.metrics.Evictions.Add(1)
		// Two messages rather than one message at two levels, so an operator
		// can grep for exactly this event. The busy one is the fallback firing:
		// it means the pool held nothing quiet to take, which is the condition
		// --max-http-clients exists to keep out of reach, and the number to
		// raise is on the line beside it.
		if busy {
			p.metrics.BusyEvictions.Add(1)
			slog.Warn(
				"server pool: evicted an entry that was serving a subscription",
				"pool_size", len(p.entries),
				"max_size", p.maxSize,
				"gitlab_url", gitlabURL,
				"enterprise", enterprise,
				"in_use", true,
			)
		} else {
			p.metrics.SizeEvictions.Add(1)
			slog.Info(
				"server pool: evicted LRU entry",
				"pool_size", len(p.entries),
				"max_size", p.maxSize,
				"gitlab_url", gitlabURL,
				"enterprise", enterprise,
				"in_use", false,
			)
		}
	}
	p.lru.Remove(victim)
}

// lruVictimLocked picks the element size pressure should drop: the least
// recently used entry the caller does not report as busy, or the tail when
// every entry is busy. Callers hold p.mu.
//
// busy reports that the scan found nothing unbusy and fell back to the tail,
// which is the event worth a warning: it is the only path on which a credential
// doing work of its own is taken. It is false when no [WithInUse] was
// registered, when the list is empty, and when the element returned names no
// entry, because in none of those cases did the pool decide against a
// subscription it could see.
func (p *ServerPool) lruVictimLocked() (victim *list.Element, busy bool) {
	back := p.lru.Back()
	if back == nil || p.inUse == nil {
		return back, false
	}
	for element := back; element != nil; element = element.Prev() {
		key, _ := element.Value.(string)
		// An element naming no entry is already stale, so dropping it costs
		// nobody anything and tidies the list.
		if entry, ok := p.entries[key]; !ok || !p.inUse(entry) {
			return element, false
		}
	}
	return back, true
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
// recoverSweep keeps a panic raised inside a background sweep from taking the
// process down, and names the sweep that raised it.
//
// Every sweep calls back into the caller: the idle one asks whether an entry
// is still in use, and both evictions tell the caller to stop that
// credential's watchers. A panic in one of those is a bug in this process, but
// losing the goroutine to it is worse than the bug is: the sweep stops
// silently, and the pool then grows to its cap holding credentials nothing has
// re-checked since startup. Deferred by all three goroutines, so they recover
// the same way and the rule is stated once.
func recoverSweep(ctx context.Context, sweep string) {
	if r := recover(); r != nil {
		slog.ErrorContext(ctx, "server pool: "+sweep+" goroutine panicked", "panic", r)
	}
}

// sweepTick runs one sweep with the recovery around that tick alone, so a
// panic costs the tick and not the sweep.
//
// Recovering in the goroutine's own defer, which is where this started, only
// looks like it does the same thing: the panic unwinds the whole goroutine
// before the recover sees it, so the ticker is gone and no sweep ever runs
// again. That is precisely the outcome the comment above says is worse than
// the bug. The goroutine keeps its own defer as the last resort, for a panic
// raised outside the tick.
func sweepTick(ctx context.Context, sweep string, run func()) {
	defer recoverSweep(ctx, sweep)
	run()
}

// idleSweepCadence is how often the idle sweep runs.
//
// A quarter of the idle timeout, so an entry is noticed within a quarter of
// the deadline of passing it, floored at [idleSweepMinInterval] so a short
// timeout cannot turn the sweep into a busy loop, and overridden by
// idleSweepInterval where a test has to reach a tick.
func (p *ServerPool) idleSweepCadence() time.Duration {
	if p.idleSweepInterval > 0 {
		return p.idleSweepInterval
	}
	return max(p.idleTimeout/idleSweepDivisor, idleSweepMinInterval)
}

func (p *ServerPool) StartIdleEviction(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background() //nolint:contextcheck // defensive: nil-ctx guard for callers that pass uninitialized context
	}
	if p.idleTimeout <= 0 {
		slog.InfoContext(ctx, "server pool: idle eviction disabled")
		return
	}

	interval := p.idleSweepCadence()
	slog.InfoContext(ctx, "server pool: starting idle eviction",
		"idle_timeout", p.idleTimeout, "sweep_interval", interval)

	go func() {
		defer recoverSweep(ctx, "idle eviction")

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "server pool: idle eviction stopped")
				return
			case <-ticker.C:
				sweepTick(ctx, "idle eviction", p.evictIdle)
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
			// the entry where it was, at the tail, since a subscription
			// refreshes nothing here, so the sweep protected it and size
			// pressure took it first. [ServerPool.evictLRU] is what enforces
			// the decision; this is what keeps the ordering honest, and what
			// keeps that scan short.
			entry.lastUsed = time.Now()
			p.lru.MoveToFront(entry.element)
			continue
		}
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		p.dropEntry(key, CauseIdle)
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
		defer recoverSweep(ctx, "revalidation")

		ticker := time.NewTicker(p.revalidateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "server pool: revalidation stopped")
				return
			case <-ticker.C:
				sweepTick(ctx, "revalidation", func() { p.revalidateAll(ctx) })
			}
		}
	}()
}

// revalidateAll checks each pool entry's token with the credential probe,
// [gitlabclient.Client.CheckCredentialDetail]. Entries GitLab refuses are
// evicted; entries whose check could not reach a verdict are left alone. Both
// warnings carry the status GitLab answered with, and the one that keeps an
// entry carries why no verdict was reached as well, since that warning can
// repeat on every round and is what an operator diagnoses the instance from.
//
// The probe and not an SDK call, because the probe's answer is read by status
// alone and never reported to the unauthorized hook. An SDK call's 401 is, and
// the one a deleted token gets names no cause, so the hook would claim the
// confirmation window and send a second probe about an entry this sweep is
// already evicting.
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
		check := entry.client.CheckCredentialDetail(checkCtx)
		cancel()

		switch check.Verdict {
		case gitlabclient.CredentialRefused:
			slog.WarnContext(ctx,
				"server pool: gitlab rejected a pooled credential, evicting entry",
				"status", check.Status,
				"age", time.Since(entry.createdAt).Round(time.Second),
			)
			p.metrics.RevalidationsFailed.Add(1)
			p.evictByKey(key)
		case gitlabclient.CredentialAccepted:
			p.metrics.RevalidationsSucceeded.Add(1)
			p.mu.Lock()
			if e, ok := p.entries[key]; ok {
				e.lastValidated = time.Now()
			}
			p.mu.Unlock()
		default:
			slog.WarnContext(ctx,
				"server pool: token revalidation could not reach a verdict, keeping entry",
				"status", check.Status,
				"error", check.Err,
				"age", time.Since(entry.createdAt).Round(time.Second),
			)
			p.metrics.RevalidationsTransient.Add(1)
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
	p.dropEntry(key, CauseStaleCredential)
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
		p.dropEntry(key, CauseInvalidCredential)
		p.metrics.Evictions.Add(1)
		p.metrics.InvalidEvictions.Add(1)
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
		// Through the nil-safe accessor rather than the field: the map is
		// never given a nil entry, so a separate guard here would be a
		// branch nothing can take, and the accessor keeps the same answer
		// if one ever were.
		if entry.Server() != srv {
			continue
		}
		gitlabURL, enterprise := entryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		p.dropEntry(key, CauseRebuild)
		p.metrics.Evictions.Add(1)
		p.metrics.RebuildEvictions.Add(1)
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
