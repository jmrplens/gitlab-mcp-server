package oauth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
	// RejectionPermissionMissing is GitLab's verdict that the token is
	// genuine and its fine-grained grant cannot read its own user, which
	// every door here needs (GET /api/v4/user answered 403 with
	// insufficient_granular_scope). It is cached, uncharged like the fresh
	// answer, because nothing at this release changes a grant after its token
	// is created, so the verdict holds for the token's life; the entry's TTL
	// bounds how long a feature flag an administrator turns on stays unseen.
	RejectionPermissionMissing
	// RejectionBelowMinimum is the verdict that the token is genuine and
	// carries neither read_api nor api, the minimum every door admits at
	// (issue 952): GET /api/v4/user answered 403 with insufficient_scope, or
	// the token's own description named only scopes below it. It is cached,
	// uncharged like the fresh answer, because a token's scopes cannot change
	// after it is created: GitLab has no route that edits a personal access
	// token's scopes, and an OAuth token granted more scopes is a new token.
	RejectionBelowMinimum
)

// rejection is one cached refusal: when it stops applying, what it was, and,
// for a missing permission, GitLab's sentence as the door quoted it.
type rejection struct {
	expiresAt   time.Time
	kind        RejectionKind
	description string
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
// from memory. It is [RejectedTokens.Lookup] without the reason, so an expired
// entry is dropped on the way out and a caller never sees a stale rejection.
func (r *RejectedTokens) Contains(gitlabURL, token string) bool {
	_, ok := r.Lookup(gitlabURL, token)
	return ok
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
	r.record(gitlabURL, token, rejection{kind: kind})
}

// RecordPermissionMissing notes that GitLab accepted this token and refused it
// the permission to read its own user, keeping the sentence GitLab gave as a
// door quotes it ([QuotedDescription]), so the refusal served from here is the
// one the round trip produced, word for word.
//
// The sentence is bounded before it is stored, because the cache holds up to
// its capacity of them and the text is the instance's, which under
// --allow-any-gitlab-url is the caller's own.
func (r *RejectedTokens) RecordPermissionMissing(gitlabURL, token, description string) {
	r.record(gitlabURL, token, rejection{kind: RejectionPermissionMissing, description: QuotedDescription(description)})
}

// RecordBelowMinimum notes that GitLab accepted this token and that it carries
// neither read_api nor api ([RejectionBelowMinimum]). Every door records it
// through this one method, the fresh refusal's and the pool's alike, so the
// refusal served from here is the one the round trip produced.
func (r *RejectedTokens) RecordBelowMinimum(gitlabURL, token string) {
	r.RecordKind(gitlabURL, token, RejectionBelowMinimum)
}

// record stores one refusal, its expiry set from the cache's TTL.
func (r *RejectedTokens) record(gitlabURL, token string, refusal rejection) {
	// The cache's one "disabled" check, and the only one it needs: this is the
	// only place an entry is stored, so a disabled cache stays empty and every
	// read misses because there is nothing to find.
	if r.max <= 0 || r.ttl <= 0 {
		return
	}
	key := rejectedKey(gitlabURL, token)

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) >= r.max {
		r.evictLocked()
	}
	// evictLocked frees a slot on every full map: it drops what has expired
	// and, when nothing has, the live entry nearest expiry, so the insert
	// never grows the map past max. A second "still full" check used to sit
	// here, unreachable for that reason, with a rationale ("every entry is
	// live, skip the insert") describing a case the eviction had already
	// handled. The bound it seemed to add is the one
	// TestRejectedTokens_AtCapacity_StaysBounded pins.
	refusal.expiresAt = time.Now().Add(r.ttl)
	r.entries[key] = refusal
}

// Lookup returns why a token was refused, and whether the refusal still
// applies. An expired entry is dropped on the way out.
func (r *RejectedTokens) Lookup(gitlabURL, token string) (RejectionKind, bool) {
	kind, _, ok := r.LookupRefusal(gitlabURL, token)
	return kind, ok
}

// LookupRefusal is [RejectedTokens.Lookup] with the sentence a
// [RejectionPermissionMissing] refusal was recorded with, already bounded and
// filtered as [QuotedDescription] leaves it, and empty for every other kind.
//
// There is no "disabled" check here. One used to sit at the top of this method
// and of Contains, repeating the one in [RejectedTokens.RecordKind]; since a
// disabled cache never stores anything, the lookup below misses on it anyway,
// and the copies could change no answer.
func (r *RejectedTokens) LookupRefusal(gitlabURL, token string) (RejectionKind, string, bool) {
	key := rejectedKey(gitlabURL, token)

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok {
		return RejectionInvalid, "", false
	}
	// expired, not time.Now().After: the other deadline checks in this file
	// already treat the instant of the deadline as reached, and this one
	// disagreeing meant a refusal outlived its TTL by a clock tick, which on
	// Windows is long enough to be observable.
	if expired(entry.expiresAt) {
		delete(r.entries, key)
		return RejectionInvalid, "", false
	}
	return entry.kind, entry.description, true
}

// maxQuotedDescriptionBytes is how much of GitLab's refusal sentence a door
// quotes, and the rejected-token cache keeps beside a refusal. GitLab's own
// sentence for a missing permission is a few hundred bytes at most.
const maxQuotedDescriptionBytes = 512

// QuotedDescription returns GitLab's sentence as a door may quote it to a
// caller: printable ASCII only, every other rune (a control character, a
// quote's lookalike, a byte of another script) written as one space, runs of
// spaces collapsed, and cut at 512 bytes.
//
// The sentence comes from whichever instance the request selected, which under
// --allow-any-gitlab-url is the caller's own, and a caller is told it in a
// JSON-RPC message body. The challenge a door sends never carries it: RFC 6749
// section 5.2 allows error_description a narrower set than this, and the
// challenge's text is the door's own constant.
func QuotedDescription(raw string) string {
	// Every field holds only the runes from '!' to '~', one byte each, so the
	// joined text is ASCII and cutting it at a byte never splits a rune. The
	// joined text neither starts nor ends with a space, so trimming the right
	// end changes only a text the cut left ending on one.
	quoted := strings.Join(strings.FieldsFunc(raw, func(r rune) bool { return r <= ' ' || r > '~' }), " ")
	return strings.TrimRight(quoted[:min(len(quoted), maxQuotedDescriptionBytes)], " ")
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
	// The map is non-empty here: the early return above fired unless at least
	// r.max entries survived the expiry sweep, and r.max is positive whenever
	// this cache is enabled. So the loop always names a key, and guarding the
	// delete against the empty string would only be a guard against a state
	// that cannot be reached — and a no-op even if it were, since deleting an
	// absent key does nothing.
	var oldestKey string
	var oldestAt time.Time
	for key, entry := range r.entries {
		if oldestKey == "" || entry.expiresAt.Before(oldestAt) {
			oldestKey, oldestAt = key, entry.expiresAt
		}
	}
	delete(r.entries, oldestKey)
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
