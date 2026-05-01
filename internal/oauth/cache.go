package oauth

import (
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
// Keys are SHA-256 hashes of raw tokens to avoid storing sensitive material.
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
func (c *TokenCache) Get(token string) (*auth.TokenInfo, bool) {
	key := tokenKey(token)

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
func (c *TokenCache) Put(token string, info *auth.TokenInfo, ttl time.Duration) {
	key := tokenKey(token)

	c.mu.Lock()
	c.entries[key] = cacheEntry{
		info:      info,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// Evict removes the cache entry for the given raw token.
func (c *TokenCache) Evict(token string) {
	key := tokenKey(token)

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Delete is an alias for [Evict] for API ergonomics.
func (c *TokenCache) Delete(token string) {
	c.Evict(token)
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

// tokenKey returns the SHA-256 hex digest of a raw token.
func tokenKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
