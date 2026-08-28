package oauth

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// RejectedTokens remembers the tokens GitLab has already refused, so a client
// replaying one does not cost an upstream verification call every time.
//
// Without it, each request carrying an invalid Bearer token is relayed 1:1 to
// the GitLab instance. That turns a public deployment into an amplifier:
// unauthenticated traffic anyone can generate becomes load on someone else's
// API, and on gitlab.com it becomes rate-limit pressure charged to the
// server's own address, where it lands on the legitimate users sharing it.
//
// Entries are keyed by the same SHA-256 digest [TokenCache] uses, never the
// raw credential — a mistyped valid token must not be left lying in memory in
// the clear. The map is bounded because its keys are supplied by whoever is
// calling: an unbounded one would trade an amplification vector for a memory
// exhaustion vector.
type RejectedTokens struct {
	mu      sync.Mutex
	entries map[string]time.Time
	max     int
	ttl     time.Duration
}

// NewRejectedTokens returns a cache holding at most capacity rejections, each for
// ttl. A non-positive capacity or ttl disables caching entirely: every method
// still works, and Contains simply never reports a hit, so a deployment that
// wants no negative cache loses the amplification defense rather than
// crashing.
func NewRejectedTokens(capacity int, ttl time.Duration) *RejectedTokens {
	return &RejectedTokens{
		entries: make(map[string]time.Time),
		max:     capacity,
		ttl:     ttl,
	}
}

// Contains reports whether the token was rejected recently enough to answer
// from memory. An expired entry is dropped on the way out, so a caller never
// sees a stale rejection.
func (r *RejectedTokens) Contains(token string) bool {
	if r.max <= 0 || r.ttl <= 0 {
		return false
	}
	key := rejectedKey(token)

	r.mu.Lock()
	defer r.mu.Unlock()

	expiresAt, ok := r.entries[key]
	if !ok {
		return false
	}
	if !time.Now().Before(expiresAt) {
		delete(r.entries, key)
		return false
	}
	return true
}

// Record notes that GitLab rejected this token.
//
// Only a definitive rejection belongs here. An upstream failure — a timeout,
// a 5xx, a 429 — says nothing about the credential, and caching one would
// lock out a valid token for the whole TTL over a transient outage.
func (r *RejectedTokens) Record(token string) {
	if r.max <= 0 || r.ttl <= 0 {
		return
	}
	key := rejectedKey(token)

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) >= r.max {
		r.evictLocked()
	}
	// Still full after evicting means every entry is live. Skipping the
	// insert is the safe direction: the rate limiter is the layer that
	// actually bounds a flood, and this cache only saves it upstream calls.
	if len(r.entries) >= r.max {
		return
	}
	r.entries[key] = time.Now().Add(r.ttl)
}

// evictLocked frees space by dropping expired entries, falling back to the
// entry closest to expiry when none have expired yet. The caller holds r.mu.
func (r *RejectedTokens) evictLocked() {
	now := time.Now()
	for key, expiresAt := range r.entries {
		if !now.Before(expiresAt) {
			delete(r.entries, key)
		}
	}
	if len(r.entries) < r.max {
		return
	}
	var oldestKey string
	var oldestAt time.Time
	for key, expiresAt := range r.entries {
		if oldestKey == "" || expiresAt.Before(oldestAt) {
			oldestKey, oldestAt = key, expiresAt
		}
	}
	if oldestKey != "" {
		delete(r.entries, oldestKey)
	}
}

// Len returns the number of entries held, expired ones included.
func (r *RejectedTokens) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// Cleanup drops every expired entry. Intended for periodic maintenance, so
// an idle server does not hold rejections until the next request evicts them.
func (r *RejectedTokens) Cleanup() {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()
	for key, expiresAt := range r.entries {
		if !now.Before(expiresAt) {
			delete(r.entries, key)
		}
	}
}

// rejectedKey returns the SHA-256 hex digest of a raw token.
//
// A rejection is not keyed by instance, unlike [TokenCache]: this cache only
// ever suppresses work — a token GitLab already refused is refused again
// without a round trip — so the worst a cross-instance collision could do is
// re-check a token that would have been re-checked anyway. Keying an
// admission decision that way would be a different matter entirely.
func rejectedKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
