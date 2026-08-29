// rejected_test.go contains unit tests for the bounded negative cache that
// stops a replayed invalid token from costing an upstream verification call.
package oauth

import (
	"strconv"
	"testing"
	"time"
)

// TestRejectedTokens_RecordedToken_IsRecognized verifies the basic contract:
// a token that was recorded is reported as rejected, and one that never was
// is not. This is the whole amplification defense — a miss means the caller
// goes upstream.
func TestRejectedTokens_RecordedToken_IsRecognized(t *testing.T) {
	t.Parallel()

	r := NewRejectedTokens(8, time.Minute)
	r.Record(testInstance, "gloas-bad")

	if !r.Contains(testInstance, "gloas-bad") {
		t.Error("recorded token should be recognized as rejected")
	}
	if r.Contains(testInstance, "gloas-other") {
		t.Error("unrecorded token must not be reported as rejected")
	}
}

// TestRejectedTokens_ExpiredEntry_IsForgotten verifies that a rejection stops
// applying once its TTL passes, and that reading it evicts it. A rejection
// held forever would keep refusing a token long after the instance that
// refused it was fixed.
func TestRejectedTokens_ExpiredEntry_IsForgotten(t *testing.T) {
	t.Parallel()

	r := NewRejectedTokens(8, time.Nanosecond)
	r.Record(testInstance, "gloas-bad")
	time.Sleep(time.Millisecond)

	if r.Contains(testInstance, "gloas-bad") {
		t.Error("expired rejection should no longer apply")
	}
	if got := r.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0 — reading an expired entry should evict it", got)
	}
}

// TestRejectedTokens_AtCapacity_StaysBounded verifies the memory bound. The
// keys are supplied by whoever is calling, so an unbounded map would trade
// an amplification vector for a memory exhaustion one.
func TestRejectedTokens_AtCapacity_StaysBounded(t *testing.T) {
	t.Parallel()

	const capacity = 4
	r := NewRejectedTokens(capacity, time.Minute)
	for i := range 50 {
		r.Record(testInstance, string(rune('a'+i%26))+string(rune('0'+i/26)))
	}

	if got := r.Len(); got > capacity {
		t.Errorf("Len() = %d, want at most %d", got, capacity)
	}
}

// TestRejectedTokens_ExpiredEntriesMakeRoom verifies that a full cache admits
// a new rejection by sweeping the expired entries first, instead of
// sacrificing one that is still doing useful work.
//
// Eviction has two stages and the order matters: the fallback that drops the
// entry nearest expiry is for a cache full of live entries only. If it ran
// first on a cache of dead ones it would discard a live rejection while three
// useless entries stayed, and the token behind that live entry would go back
// to reaching GitLab on every attempt — the amplification this cache exists
// to prevent. The bounded case is covered separately by
// TestRejectedTokens_AtCapacity_StaysBounded, which never lets an entry
// expire and so only ever reaches the fallback.
func TestRejectedTokens_ExpiredEntriesMakeRoom(t *testing.T) {
	t.Parallel()

	const capacity = 3
	const ttl = 100 * time.Millisecond

	r := NewRejectedTokens(capacity, ttl)
	for i := range capacity {
		r.Record(testInstance, "glpat-stale-"+strconv.Itoa(i))
	}
	if got := r.Len(); got != capacity {
		t.Fatalf("Len() = %d, want %d before anything expires", got, capacity)
	}

	time.Sleep(ttl + 50*time.Millisecond)
	r.Record(testInstance, "glpat-fresh")

	// Only the new entry survives: all three expired ones were swept, which
	// is what freed the slot.
	if got := r.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1 (three expired entries swept, one admitted)", got)
	}
	if !r.Contains(testInstance, "glpat-fresh") {
		t.Error("the newly recorded rejection was not admitted")
	}
}

// TestRejectedTokens_Cleanup_DropsOnlyExpired verifies that periodic
// maintenance frees expired entries without discarding live ones, so an idle
// server does not accumulate rejections and an active one does not lose its
// defense.
func TestRejectedTokens_Cleanup_DropsOnlyExpired(t *testing.T) {
	t.Parallel()

	r := NewRejectedTokens(8, time.Nanosecond)
	r.Record(testInstance, "stale")
	time.Sleep(time.Millisecond)

	live := NewRejectedTokens(8, time.Hour)
	live.Record(testInstance, "fresh")

	r.Cleanup()
	live.Cleanup()

	if got := r.Len(); got != 0 {
		t.Errorf("expired entries remaining after Cleanup: %d", got)
	}
	if !live.Contains(testInstance, "fresh") {
		t.Error("Cleanup dropped an entry that had not expired")
	}
}

// TestRejectedTokens_Disabled_NeverReportsAHit verifies that a non-positive
// bound turns the cache off without breaking: every method still works and
// Contains never hits, so a deployment configured that way loses the
// optimization rather than failing.
func TestRejectedTokens_Disabled_NeverReportsAHit(t *testing.T) {
	t.Parallel()

	for name, r := range map[string]*RejectedTokens{
		"zero size": NewRejectedTokens(0, time.Minute),
		"zero ttl":  NewRejectedTokens(8, 0),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r.Record(testInstance, "gloas-bad")
			if r.Contains(testInstance, "gloas-bad") {
				t.Error("a disabled cache must never report a hit")
			}
			if got := r.Len(); got != 0 {
				t.Errorf("Len() = %d, want 0 for a disabled cache", got)
			}
		})
	}
}

// TestRejectedTokens_DoesNotStoreRawTokens verifies that the raw credential
// never reaches the map. A mistyped valid token can land here, so what is
// held must be a digest, exactly as [TokenCache] does.
func TestRejectedTokens_DoesNotStoreRawTokens(t *testing.T) {
	t.Parallel()

	// Not shaped like a real PAT on purpose: a fixture that looks like one
	// trips secret scanners on every commit for no benefit — the property
	// under test is that whatever arrives is hashed, not what it looks like.
	const secret = "a-credential-typed-into-the-wrong-field"
	r := NewRejectedTokens(8, time.Minute)
	r.Record(testInstance, secret)

	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.entries {
		if key == secret {
			t.Fatal("raw token stored as a cache key")
		}
	}
	if _, ok := r.entries[rejectedKey(testInstance, secret)]; !ok {
		t.Error("entry not keyed by the SHA-256 digest")
	}
}

// TestRejectedTokens_IsScopedToTheInstance pins why a rejection carries the
// instance that issued it.
//
// Contains makes the guard answer 401 WITHOUT asking GitLab, so a rejection is
// an admission decision rather than a cached lookup. On a deployment
// publishing more than one instance, keying by the token alone would let a
// refusal from one instance refuse a perfectly valid credential on another for
// the whole TTL, with no upstream call able to correct it.
func TestRejectedTokens_IsScopedToTheInstance(t *testing.T) {
	t.Parallel()

	const (
		instanceA = "https://gitlab.com"
		instanceB = "https://gitlab.internal.example.com"
		token     = "gloas-same-string-on-both"
	)

	r := NewRejectedTokens(8, time.Minute)
	r.Record(instanceA, token)

	tests := []struct {
		name     string
		instance string
		want     bool
		why      string
	}{
		{
			name:     "the instance that rejected it remembers",
			instance: instanceA,
			want:     true,
			why:      "the rejection must still suppress the upstream call it was recorded for",
		},
		{
			name:     "another instance is unaffected",
			instance: instanceB,
			want:     false,
			why:      "one instance's rejection must not refuse a valid token on another",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := r.Contains(tt.instance, token); got != tt.want {
				t.Errorf("Contains(%q) = %v, want %v — %s", tt.instance, got, tt.want, tt.why)
			}
		})
	}
}
