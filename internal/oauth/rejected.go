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
	entries map[string]rejection
	max     int
	ttl     time.Duration
}

// RejectionKind records why a token was refused, so an answer served from this
// cache is the same answer the caller would have got from the round trip.
//
// Without it a cached refusal degrades to the harshest available response: an
// unadmitted recipient would be reported as a token GitLab rejected, and would
// be charged the authentication-failure budget the first refusal deliberately
// spared it.
type RejectionKind int

const (
	// RejectionInvalid is GitLab's own verdict on the credential.
	RejectionInvalid RejectionKind = iota
	// RejectionUnaccepted is this deployment's: the instance accepts the
	// token, but it was not issued to an admitted OAuth application.
	RejectionUnaccepted
)

// rejection is one cached refusal: when it stops applying, and what it was.
type rejection struct {
	expiresAt time.Time
	kind      RejectionKind
}

// NewRejectedTokens returns a cache holding at most capacity rejections, each for
// ttl. A non-positive capacity or ttl disables caching entirely: every method
// still works, and Contains simply never reports a hit, so a deployment that
// wants no negative cache loses the amplification defense rather than
// crashing.
func NewRejectedTokens(capacity int, ttl time.Duration) *RejectedTokens {
	return &RejectedTokens{
		entries: make(map[string]rejection),
		max:     capacity,
		ttl:     ttl,
	}
}

// Contains reports whether the token was rejected recently enough to answer
// from memory. An expired entry is dropped on the way out, so a caller never
// sees a stale rejection.
func (r *RejectedTokens) Contains(gitlabURL, token string) bool {
	if r.max <= 0 || r.ttl <= 0 {
		return false
	}
	key := rejectedKey(gitlabURL, token)

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok {
		return false
	}
	if !time.Now().Before(entry.expiresAt) {
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
func (r *RejectedTokens) Record(gitlabURL, token string) {
	r.RecordKind(gitlabURL, token, RejectionInvalid)
}

// RecordKind notes a refusal and why, so [RejectedTokens.Lookup] can reproduce
// it rather than collapsing every cached refusal into GitLab's verdict.
func (r *RejectedTokens) RecordKind(gitlabURL, token string, kind RejectionKind) {
	if r.max <= 0 || r.ttl <= 0 {
		return
	}
	key := rejectedKey(gitlabURL, token)

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
	r.entries[key] = rejection{expiresAt: time.Now().Add(r.ttl), kind: kind}
}

// Lookup returns why a token was refused, and whether the refusal still
// applies. An expired entry is dropped on the way out.
func (r *RejectedTokens) Lookup(gitlabURL, token string) (RejectionKind, bool) {
	if r.max <= 0 || r.ttl <= 0 {
		return RejectionInvalid, false
	}
	key := rejectedKey(gitlabURL, token)

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok {
		return RejectionInvalid, false
	}
	// expired, not time.Now().After: the three other deadline checks in this
	// file already treat the instant of the deadline as reached, and this one
	// disagreeing meant a refusal outlived its TTL by a clock tick, which on
	// Windows is long enough to be observable.
	if expired(entry.expiresAt) {
		delete(r.entries, key)
		return RejectionInvalid, false
	}
	return entry.kind, true
}

// evictLocked frees space by dropping expired entries, falling back to the
// entry closest to expiry when none have expired yet. The caller holds r.mu.
func (r *RejectedTokens) evictLocked() {
	now := time.Now()
	for key, entry := range r.entries {
		if !now.Before(entry.expiresAt) {
			delete(r.entries, key)
		}
	}
	if len(r.entries) < r.max {
		return
	}
	var oldestKey string
	var oldestAt time.Time
	for key, entry := range r.entries {
		if oldestKey == "" || entry.expiresAt.Before(oldestAt) {
			oldestKey, oldestAt = key, entry.expiresAt
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
	for key, entry := range r.entries {
		if !now.Before(entry.expiresAt) {
			delete(r.entries, key)
		}
	}
}

// rejectedKey returns the SHA-256 hex digest of an instance URL and a raw
// token, matching [tokenKey].
//
// A rejection is scoped to the instance that issued it because a rejection is
// an admission DECISION, not merely a cached lookup: [RejectedTokens.Contains]
// makes the guard answer 401 without asking GitLab at all. A token GitLab.com
// refused says nothing about the same string on a self-managed instance — and
// on a deployment publishing both, keying by the token alone would refuse a
// perfectly valid credential for the whole TTL, with no upstream call able to
// correct it.
func rejectedKey(gitlabURL, token string) string {
	h := sha256.Sum256([]byte(gitlabURL + "\x00" + token))
	return hex.EncodeToString(h[:])
}
