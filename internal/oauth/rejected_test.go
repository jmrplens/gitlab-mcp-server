// rejected_test.go contains unit tests for the bounded negative cache that
// stops a replayed invalid token from costing an upstream verification call.
package oauth

import (
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
	r.Record("gloas-bad")

	if !r.Contains("gloas-bad") {
		t.Error("recorded token should be recognized as rejected")
	}
	if r.Contains("gloas-other") {
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
	r.Record("gloas-bad")
	time.Sleep(time.Millisecond)

	if r.Contains("gloas-bad") {
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
		r.Record(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}

	if got := r.Len(); got > capacity {
		t.Errorf("Len() = %d, want at most %d", got, capacity)
	}
}

// TestRejectedTokens_Cleanup_DropsOnlyExpired verifies that periodic
// maintenance frees expired entries without discarding live ones, so an idle
// server does not accumulate rejections and an active one does not lose its
// defense.
func TestRejectedTokens_Cleanup_DropsOnlyExpired(t *testing.T) {
	t.Parallel()

	r := NewRejectedTokens(8, time.Nanosecond)
	r.Record("stale")
	time.Sleep(time.Millisecond)

	live := NewRejectedTokens(8, time.Hour)
	live.Record("fresh")

	r.Cleanup()
	live.Cleanup()

	if got := r.Len(); got != 0 {
		t.Errorf("expired entries remaining after Cleanup: %d", got)
	}
	if !live.Contains("fresh") {
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
			r.Record("gloas-bad")
			if r.Contains("gloas-bad") {
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

	const secret = "glpat-a-real-token-typed-into-the-wrong-field"
	r := NewRejectedTokens(8, time.Minute)
	r.Record(secret)

	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.entries {
		if key == secret {
			t.Fatal("raw token stored as a cache key")
		}
	}
	if _, ok := r.entries[tokenKey(secret)]; !ok {
		t.Error("entry not keyed by the SHA-256 digest")
	}
}
