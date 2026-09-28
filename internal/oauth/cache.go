package oauth

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// identityCacheCapacity is how many verified identities one [TokenCache]
// holds. When a verification arrives for a key the cache does not hold and
// the cache is full, the least recently used entry makes room for it.
//
// A bound is needed because the key is chosen by the caller. Every value of
// it costs a valid credential, and on GitLab that is cheap: a personal access
// token is a form in the user settings with no administrator and no cap, and
// a second OAuth token is one more authorization. Without it the cache held
// one entry for every credential verified inside one TTL, however many that
// was, so a caller holding many credentials grew the process's memory for as
// long as the TTL lasted.
//
// Eviction rather than refusal, as for the rejected-token memo beside it,
// because refusing a newcomer would refuse a credential GitLab has just
// accepted. What eviction costs the credential it takes is one more
// verification against GitLab the next time it is presented, which the
// verification ceiling ([verificationSlots]) bounds in turn, and, while its
// address is blocked by an authentication budget, the exemption a cached
// identity gives it. The victim is the entry used least recently, so a
// credential in steady use is the last to go: a caller would have to push more
// distinct valid credentials through verification than this bound between two
// of its requests, and verification admits at most [verificationSlots] at a
// time.
//
// Ten thousand is the largest pool an operator may configure (--max-http-clients
// tops out at the same figure), so no pool the server can be given serves more
// credentials than this cache remembers. Measured through the verifier, an
// entry retains about seven hundred bytes, so the bound holds the cache to about
// seven megabytes where a hundred thousand credentials used to hold sixty.
const identityCacheCapacity = tenancy.OAuthCacheCapacity // register row ADM-005

// cacheEntry is one verified identity and when it stops being one. The key is
// kept beside it so the entry taken off the back of the recency list can be
// removed from the map too.
type cacheEntry struct {
	key       string
	info      *auth.TokenInfo
	expiresAt time.Time
}

// TokenCache is a thread-safe, TTL-based cache for verified token identities,
// bounded in size. Keys are SHA-256 hashes of the instance URL and the raw
// token, so no sensitive material is stored.
//
// The instance is part of the key, not an afterthought. A token is only ever
// valid for the GitLab that issued it, so a cache keyed by the token alone
// would let a deployment publishing more than one instance accept a
// credential verified against the first as proof of identity on the second.
//
// Every read that finds a live entry marks it used, and a full cache makes
// room by dropping the entry used least recently (see [identityCacheCapacity]).
// That is why a read takes the same lock a write does: finding an entry
// changes the order the cache would evict in.
type TokenCache struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	recency  *list.List
	capacity int
}

// NewTokenCache creates an empty [TokenCache] holding at most
// [identityCacheCapacity] identities.
func NewTokenCache() *TokenCache {
	return newTokenCache(identityCacheCapacity)
}

// newTokenCache creates an empty cache holding at most capacity identities,
// which must be at least one. It is what [NewTokenCache] calls with the
// register's bound, and what a test calls with one small enough to fill.
func newTokenCache(capacity int) *TokenCache {
	return &TokenCache{
		entries:  make(map[string]*list.Element),
		recency:  list.New(),
		capacity: capacity,
	}
}

// Get returns the cached [auth.TokenInfo] for the given raw token if present
// and not expired, and marks the entry as used. Expired entries are lazily
// evicted on read.
//
// The expiry is judged under the same lock that removes the entry, so an
// entry another request stored a moment ago is never removed because an older
// one under its key had expired.
func (c *TokenCache) Get(gitlabURL, token string) (*auth.TokenInfo, bool) {
	key := tokenKey(gitlabURL, token)

	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	entry := entryOf(element)
	if expired(entry.expiresAt) {
		c.removeLocked(element)
		return nil, false
	}
	c.recency.MoveToFront(element)
	return entry.info, true
}

// Put stores a [auth.TokenInfo] for the given raw token with the specified TTL.
//
// A key already held is refreshed in place. A new key in a full cache first
// drops the entry used least recently, so the cache never holds more than its
// capacity.
func (c *TokenCache) Put(gitlabURL, token string, info *auth.TokenInfo, ttl time.Duration) {
	key := tokenKey(gitlabURL, token)
	expiresAt := time.Now().Add(ttl)

	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[key]; ok {
		entry := entryOf(element)
		entry.info, entry.expiresAt = info, expiresAt
		c.recency.MoveToFront(element)
		return
	}
	if len(c.entries) >= c.capacity {
		c.removeLocked(c.recency.Back())
	}
	c.entries[key] = c.recency.PushFront(&cacheEntry{key: key, info: info, expiresAt: expiresAt})
}

// removeLocked drops one entry from both the map and the recency list. The
// caller holds c.mu and passes an element of this cache's list.
func (c *TokenCache) removeLocked(element *list.Element) {
	c.recency.Remove(element)
	delete(c.entries, entryOf(element).key)
}

// entryOf is the entry a recency list element carries. The list holds nothing
// else, since [TokenCache.Put] is the only place anything is pushed onto it.
func entryOf(element *list.Element) *cacheEntry {
	entry, _ := element.Value.(*cacheEntry)
	return entry
}

// Evict removes the cache entry for the given raw token.
func (c *TokenCache) Evict(gitlabURL, token string) {
	key := tokenKey(gitlabURL, token)

	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		c.removeLocked(element)
	}
}

// Delete is an alias for [Evict] for API ergonomics.
func (c *TokenCache) Delete(gitlabURL, token string) {
	c.Evict(gitlabURL, token)
}

// Len returns the total number of entries (including potentially expired ones).
func (c *TokenCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Cleanup removes all expired entries. Intended for periodic maintenance.
func (c *TokenCache) Cleanup() {
	// One cutoff for the whole pass, read before the lock. Calling time.Now
	// per entry would make the sweep's result depend on the order it walks
	// the entries in, and [RejectedTokens.Cleanup] already snapshots for the
	// same reason.
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	for element := c.recency.Front(); element != nil; {
		next := element.Next()
		if expiredAt(now, entryOf(element).expiresAt) {
			c.removeLocked(element)
		}
		element = next
	}
}

// expired reports whether a deadline has been reached as of now, counting the
// instant of the deadline itself as reached.
//
// The obvious spelling, time.Now().After(deadline), keeps an entry alive for
// one clock tick past its deadline, and a tick is not the same length
// everywhere: Linux reads the clock with nanosecond resolution, while on
// Windows two consecutive calls to time.Now often return the same instant, so
// a deadline of "now" never passes there at all. The configured TTL range
// (1m to 2h) makes that harmless in a running server and wrong in what the
// code says, which is enough reason to say the other thing.
func expired(deadline time.Time) bool {
	return expiredAt(time.Now(), deadline)
}

// expiredAt is [expired] against a caller-supplied instant, for a sweep that
// must judge every entry by the same clock reading.
func expiredAt(now, deadline time.Time) bool {
	return !now.Before(deadline)
}

// RunCleanup sweeps expired entries every interval until ctx is done. It blocks,
// so callers run it in their own goroutine.
//
// Reads already evict lazily, which is enough for a token that comes back: its
// own entry is dropped the next time it is looked up. It is not enough for one
// that does not. A scanner walking an endpoint with fresh credentials, or a
// fleet whose tokens rotate, leaves entries nothing will ever read again, and
// the capacity only drops them once the cache is full. The sweep is what gives
// the memory back when the traffic that grew the cache has stopped.
func (c *TokenCache) RunCleanup(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Cleanup()
		}
	}
}

// tokenKey returns the SHA-256 hex digest of an instance URL and a raw token.
//
// The NUL separator is what keeps the pair unambiguous: without it the
// instance "https://a.example/b" with token "c" and "https://a.example" with
// token "/bc" would hash identically, and a NUL cannot occur in either.
func tokenKey(gitlabURL, token string) string {
	h := sha256.Sum256([]byte(gitlabURL + "\x00" + token))
	return hex.EncodeToString(h[:])
}
