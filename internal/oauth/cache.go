package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

type cacheEntry struct {
	info      *auth.TokenInfo
	expiresAt time.Time
}

// TokenCache is a thread-safe, TTL-based cache for verified token identities.
// Keys are SHA-256 hashes of the instance URL and the raw token, so no
// sensitive material is stored.
//
// The instance is part of the key, not an afterthought. A token is only ever
// valid for the GitLab that issued it, so a cache keyed by the token alone
// would let a deployment publishing more than one instance accept a
// credential verified against the first as proof of identity on the second.
type TokenCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

// NewTokenCache creates an empty [TokenCache].
func NewTokenCache() *TokenCache {
	return &TokenCache{
		entries: make(map[string]cacheEntry),
	}
}

// Get returns the cached [auth.TokenInfo] for the given raw token if present
// and not expired. Expired entries are lazily evicted on read.
func (c *TokenCache) Get(gitlabURL, token string) (*auth.TokenInfo, bool) {
	key := tokenKey(gitlabURL, token)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.info, true
}

// Put stores a [auth.TokenInfo] for the given raw token with the specified TTL.
func (c *TokenCache) Put(gitlabURL, token string, info *auth.TokenInfo, ttl time.Duration) {
	key := tokenKey(gitlabURL, token)

	c.mu.Lock()
	c.entries[key] = cacheEntry{
		info:      info,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// Evict removes the cache entry for the given raw token.
func (c *TokenCache) Evict(gitlabURL, token string) {
	key := tokenKey(gitlabURL, token)

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Delete is an alias for [Evict] for API ergonomics.
func (c *TokenCache) Delete(gitlabURL, token string) {
	c.Evict(gitlabURL, token)
}

// Len returns the total number of entries (including potentially expired ones).
func (c *TokenCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Cleanup removes all expired entries. Intended for periodic maintenance.
func (c *TokenCache) Cleanup() {
	now := time.Now()

	c.mu.Lock()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

// RunCleanup sweeps expired entries every interval until ctx is done. It blocks,
// so callers run it in their own goroutine.
//
// Reads already evict lazily, which is enough for a token that comes back: its
// own entry is dropped the next time it is looked up. It is not enough for one
// that does not. Every distinct bearer this deployment has ever verified holds a
// map entry for the lifetime of the process — a scanner walking an endpoint with
// fresh credentials, or a fleet whose tokens rotate, leaves entries nothing will
// ever read again. The sweep is what makes the TTL a bound on memory and not
// only on staleness.
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
