// cache_test.go contains unit tests for the OAuth token identity cache,
// verifying TTL expiration, concurrent access, and eviction behavior.
package oauth

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// testInstance is the GitLab instance these cache entries belong to. The
// cache keys on instance and token together, so every call names one.
const testInstance = "https://gitlab.example.com"

// TestTokenCache_PutAndGet verifies that a token stored via Put is returned
// by Get with the same UserID and Extra fields intact.
func TestTokenCache_PutAndGet(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	info := &auth.TokenInfo{UserID: "42", Extra: map[string]any{"username": "test"}}
	cache.Put(testInstance, "token-abc", info, 5*time.Minute)

	got, ok := cache.Get(testInstance, "token-abc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.UserID != "42" {
		t.Errorf("UserID = %q, want %q", got.UserID, "42")
	}
	if got.Extra["username"] != "test" {
		t.Errorf("username = %v, want %q", got.Extra["username"], "test")
	}
}

// TestTokenCache_GetMiss verifies that Get returns ok=false for a token
// that was never stored in the cache.
func TestTokenCache_GetMiss(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()

	_, ok := cache.Get(testInstance, "nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

// TestTokenCache_GetExpired verifies that Get returns a miss for an entry
// whose TTL has elapsed and that the expired entry is lazily evicted.
func TestTokenCache_GetExpired(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	info := &auth.TokenInfo{UserID: "42"}

	// Use a TTL of zero so the entry is immediately expired.
	cache.Put(testInstance, "expired-token", info, 0)

	_, ok := cache.Get(testInstance, "expired-token")
	if ok {
		t.Fatal("expected cache miss for expired entry")
	}

	if cache.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after lazy eviction", cache.Len())
	}
}

// TestTokenCache_Get_JudgesExpiryAtTheDeadline pins where an entry stops
// answering: an entry whose deadline has passed, or is the instant it was
// stored, is a miss and is removed, while one still inside its lifetime hits
// and stays.
//
// The expiry is judged under the same lock that removes the entry, so there is
// no gap in which another request's fresh verification under the same key
// could be thrown away for its predecessor's expiry, which is what a separate
// re-check used to guard.
func TestTokenCache_Get_JudgesExpiryAtTheDeadline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ttl     time.Duration
		wantHit bool
	}{
		{name: "a deadline already past", ttl: -time.Minute},
		{name: "a deadline that is the instant it was stored", ttl: 0},
		{name: "a deadline still ahead", ttl: time.Minute, wantHit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := NewTokenCache()
			cache.Put(testInstance, "token", &auth.TokenInfo{UserID: "7"}, tt.ttl)

			_, hit := cache.Get(testInstance, "token")
			if hit != tt.wantHit {
				t.Errorf("hit = %v, want %v", hit, tt.wantHit)
			}
			if kept := cache.Len() == 1; kept != tt.wantHit {
				t.Errorf("entry kept = %v, want %v: a miss removes the entry and a hit keeps it", kept, tt.wantHit)
			}
		})
	}
}

// TestNewTokenCache_HoldsTheRegisterCapacity pins the bound the server's cache
// is built with to the register's value (ADM-005), so the figure the
// documentation states is the one that applies.
func TestNewTokenCache_HoldsTheRegisterCapacity(t *testing.T) {
	t.Parallel()

	if got := NewTokenCache().capacity; got != tenancy.OAuthCacheCapacity {
		t.Errorf("capacity = %d, want the register's %d", got, tenancy.OAuthCacheCapacity)
	}
}

// TestTokenCache_Full_EvictsTheLeastRecentlyUsed pins the victim a full cache
// chooses: the identity used least recently, where a read counts as a use.
//
// A read is what an identity in service does. A credential a client presents
// every few seconds is read on every request, so under an eviction by
// insertion order it would still go first once enough newer credentials
// arrived; the LRU order is what keeps it.
func TestTokenCache_Full_EvictsTheLeastRecentlyUsed(t *testing.T) {
	t.Parallel()

	cache := newTokenCache(3)
	for _, token := range []string{"a", "b", "c"} {
		cache.Put(testInstance, token, &auth.TokenInfo{UserID: token}, time.Hour)
	}
	// a was stored first and is read now, so b is the least recently used.
	if _, ok := cache.Get(testInstance, "a"); !ok {
		t.Fatal("a missing before the cache was full")
	}

	cache.Put(testInstance, "d", &auth.TokenInfo{UserID: "d"}, time.Hour)

	if got := cache.Len(); got != 3 {
		t.Errorf("Len() = %d after a fourth identity, want the capacity, 3", got)
	}
	for _, tc := range []struct {
		token string
		want  bool
	}{{"a", true}, {"b", false}, {"c", true}, {"d", true}} {
		t.Run(tc.token, func(t *testing.T) {
			t.Parallel()
			if _, ok := cache.Get(testInstance, tc.token); ok != tc.want {
				t.Errorf("%s cached = %v, want %v: b is the one used least recently", tc.token, ok, tc.want)
			}
		})
	}
}

// TestTokenCache_Full_DropsAnExpiredIdentityBeforeALiveOne pins the other half
// of the victim a full cache chooses: an expired identity goes first, whatever
// its place in the recency order, and the identity used least recently goes
// only when none has expired. An entry can be used recently and expired
// already, since a TTL is shortened to the token's own expiry, and dropping a
// live identity to keep it would cost a credential in service its entry for
// nothing.
func TestTokenCache_Full_DropsAnExpiredIdentityBeforeALiveOne(t *testing.T) {
	t.Parallel()

	cache := newTokenCache(3)
	cache.Put(testInstance, "live-oldest", &auth.TokenInfo{UserID: "1"}, time.Hour)
	cache.Put(testInstance, "live-newer", &auth.TokenInfo{UserID: "2"}, time.Hour)
	// Stored last, so it is the most recently used, and expired already.
	cache.Put(testInstance, "expired-recent", &auth.TokenInfo{UserID: "3"}, -time.Minute)

	cache.Put(testInstance, "arriving", &auth.TokenInfo{UserID: "4"}, time.Hour)

	if got := cache.Len(); got != 3 {
		t.Errorf("Len() = %d, want the capacity, 3", got)
	}
	for _, tc := range []struct {
		token string
		want  bool
	}{{"live-oldest", true}, {"live-newer", true}, {"expired-recent", false}, {"arriving", true}} {
		t.Run(tc.token, func(t *testing.T) {
			t.Parallel()
			if _, ok := cache.Get(testInstance, tc.token); ok != tc.want {
				t.Errorf("%s cached = %v, want %v: the expired identity makes room, not the live one used least recently",
					tc.token, ok, tc.want)
			}
		})
	}
}

// TestTokenCache_Full_WalksOnlyOnceSomethingCanHaveExpired pins what keeps a
// full cache cheap: it looks for an expired identity only once the earliest
// expiry it holds has passed. A cache whose bound says nothing can have
// expired yet drops the identity used least recently without walking the
// others, which a cache of ten thousand live identities under a stream of new
// ones would otherwise do on every verification. The bound is set by hand here
// to a state the cache never reaches on its own, because that is the only way
// to see whether the walk happened.
func TestTokenCache_Full_WalksOnlyOnceSomethingCanHaveExpired(t *testing.T) {
	t.Parallel()

	cache := newTokenCache(2)
	cache.Put(testInstance, "live", &auth.TokenInfo{UserID: "1"}, time.Hour)
	cache.Put(testInstance, "expired", &auth.TokenInfo{UserID: "2"}, -time.Minute)
	cache.mu.Lock()
	cache.soonest = time.Now().Add(time.Hour)
	cache.mu.Unlock()

	cache.Put(testInstance, "arriving", &auth.TokenInfo{UserID: "3"}, time.Hour)

	cache.mu.Lock()
	_, expiredKept := cache.entries[tokenKey(testInstance, "expired")]
	_, liveKept := cache.entries[tokenKey(testInstance, "live")]
	cache.mu.Unlock()
	if !expiredKept || liveKept {
		t.Errorf("expired kept %v, live kept %v; want no walk while the bound is ahead, so the least recently used goes",
			expiredKept, liveKept)
	}
}

// TestTokenCache_Soonest_TracksTheEarliestExpiryHeld pins the bound the walk
// is gated on: the first identity stored sets it, a later expiry leaves it, an
// earlier one lowers it whether it arrives with a new key or a refreshed one,
// and a sweep sets it to the earliest expiry among what is left, or clears it
// when nothing is.
func TestTokenCache_Soonest_TracksTheEarliestExpiryHeld(t *testing.T) {
	t.Parallel()

	cache := newTokenCache(10)
	expiryOf := func(token string) time.Time {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return entryOf(cache.entries[tokenKey(testInstance, token)]).expiresAt
	}
	soonest := func() time.Time {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.soonest
	}

	// sequential: each step reads the bound the step before it left.
	for _, step := range []struct {
		name  string
		do    func()
		token string
	}{
		{"the first identity sets it", func() { cache.Put(testInstance, "b", &auth.TokenInfo{}, time.Hour) }, "b"},
		{"a later expiry leaves it", func() { cache.Put(testInstance, "c", &auth.TokenInfo{}, 2*time.Hour) }, "b"},
		{"an earlier new key lowers it", func() { cache.Put(testInstance, "a", &auth.TokenInfo{}, 30*time.Minute) }, "a"},
		{"an earlier refresh lowers it", func() { cache.Put(testInstance, "c", &auth.TokenInfo{}, time.Minute) }, "c"},
		{"an expired key lowers it into the past", func() { cache.Put(testInstance, "gone", &auth.TokenInfo{}, -time.Minute) }, "gone"},
		{"a sweep sets it to the earliest left", cache.Cleanup, "c"},
	} {
		step.do()
		if got, want := soonest(), expiryOf(step.token); !got.Equal(want) {
			t.Errorf("%s: soonest = %v, want %s's expiry, %v", step.name, got, step.token, want)
		}
	}

	for _, token := range []string{"a", "b", "c"} {
		cache.Evict(testInstance, token)
	}
	cache.Put(testInstance, "gone-too", &auth.TokenInfo{}, -time.Minute)
	cache.Cleanup()
	if got := soonest(); !got.IsZero() {
		t.Errorf("soonest = %v after a sweep that left nothing, want it cleared", got)
	}
}

// TestTokenCache_Put_KnownKeyRefreshesInPlace pins that storing a key the
// cache already holds replaces its identity and deadline, makes it the most
// recently used, and evicts nothing: re-verifying a credential in service must
// not cost another credential its entry.
func TestTokenCache_Put_KnownKeyRefreshesInPlace(t *testing.T) {
	t.Parallel()

	cache := newTokenCache(2)
	cache.Put(testInstance, "a", &auth.TokenInfo{UserID: "old"}, time.Hour)
	cache.Put(testInstance, "b", &auth.TokenInfo{UserID: "b"}, time.Hour)

	cache.Put(testInstance, "a", &auth.TokenInfo{UserID: "new"}, time.Hour)

	if got := cache.Len(); got != 2 {
		t.Fatalf("Len() = %d after refreshing a held key, want 2: nothing is evicted for a key already held", got)
	}
	if info, ok := cache.Get(testInstance, "a"); !ok || info.UserID != "new" {
		t.Errorf("a = %+v, %v; want the refreshed identity", info, ok)
	}

	// a is now the most recent, so a third key takes b.
	cache.Put(testInstance, "c", &auth.TokenInfo{UserID: "c"}, time.Hour)
	if _, ok := cache.Get(testInstance, "b"); ok {
		t.Error("b survived a third key: the refresh did not make a the most recently used")
	}
	if _, ok := cache.Get(testInstance, "a"); !ok {
		t.Error("a was evicted although it was refreshed after b was stored")
	}
}

// TestTokenCache_Put_RefreshedDeadlineReplacesTheOldOne pins that refreshing a
// key replaces its deadline as well as its identity: an entry stored expired and
// then refreshed with a lifetime answers, and one refreshed to an expired
// deadline stops answering.
func TestTokenCache_Put_RefreshedDeadlineReplacesTheOldOne(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "token", &auth.TokenInfo{UserID: "1"}, -time.Minute)
	cache.Put(testInstance, "token", &auth.TokenInfo{UserID: "1"}, time.Hour)
	if _, ok := cache.Get(testInstance, "token"); !ok {
		t.Error("a key refreshed with a lifetime still reads as expired")
	}

	cache.Put(testInstance, "token", &auth.TokenInfo{UserID: "1"}, -time.Minute)
	if _, ok := cache.Get(testInstance, "token"); ok {
		t.Error("a key refreshed to an expired deadline still answers")
	}
}

// TestTokenCache_ManyDistinctCredentials_StayAtTheBound is the measurement the
// issue asked for, at a scale a unit test can afford: ten thousand distinct
// credentials pushed through a cache of a hundred leave it at a hundred, and a
// credential read between every two of them keeps its entry throughout.
//
// Before the bound the cache held one entry per credential verified inside a
// TTL, however many that was; the credential in steady use is what the LRU
// order exists for, since it is the one an address block's exemption reads.
func TestTokenCache_ManyDistinctCredentials_StayAtTheBound(t *testing.T) {
	t.Parallel()

	const capacity, distinct = 100, 10000
	cache := newTokenCache(capacity)
	cache.Put(testInstance, "in-service", &auth.TokenInfo{UserID: "0"}, time.Hour)

	for i := range distinct {
		cache.Put(testInstance, "flood-"+strconv.Itoa(i), &auth.TokenInfo{UserID: strconv.Itoa(i + 1)}, time.Hour)
		if _, ok := cache.Get(testInstance, "in-service"); !ok {
			t.Fatalf("the credential in service was evicted after %d others arrived", i+1)
		}
	}

	if got := cache.Len(); got != capacity {
		t.Errorf("Len() = %d after %d distinct credentials, want the bound, %d", got, distinct, capacity)
	}
	// The oldest of the flood are gone and the newest stay.
	if _, ok := cache.Get(testInstance, "flood-0"); ok {
		t.Error("the first credential of the flood survived ten thousand newer ones")
	}
	if _, ok := cache.Get(testInstance, "flood-"+strconv.Itoa(distinct-1)); !ok {
		t.Error("the newest credential of the flood is missing")
	}
}

// TestTokenCache_Evict_AbsentKeyChangesNothing pins that evicting a key the
// cache does not hold leaves every other entry where it was.
func TestTokenCache_Evict_AbsentKeyChangesNothing(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "kept", &auth.TokenInfo{UserID: "1"}, time.Hour)

	cache.Evict(testInstance, "never-stored")

	if got := cache.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1", got)
	}
}

// TestTokenCache_Evict verifies that Evict removes a specific token entry
// and subsequent Get calls for that token return a miss.
func TestTokenCache_Evict(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "to-evict", &auth.TokenInfo{UserID: "1"}, 5*time.Minute)

	cache.Evict(testInstance, "to-evict")

	_, ok := cache.Get(testInstance, "to-evict")
	if ok {
		t.Fatal("expected cache miss after eviction")
	}
}

// TestTokenCache_IsScopedToTheInstance is the reason the key carries the
// instance URL at all.
//
// A token is only ever valid for the GitLab that issued it. A cache keyed by
// the token alone would let a deployment publishing more than one instance
// accept a credential verified against the first as a verified identity on
// the second — the same string, a different account, no upstream call to
// notice.
func TestTokenCache_IsScopedToTheInstance(t *testing.T) {
	t.Parallel()

	const (
		instanceA = "https://gitlab.com"
		instanceB = "https://gitlab.internal.example.com"
		token     = "glpat-same-string-on-both"
	)

	cache := NewTokenCache()
	cache.Put(instanceA, token, &auth.TokenInfo{UserID: "1"}, 5*time.Minute)

	if _, ok := cache.Get(instanceB, token); ok {
		t.Error("a token verified against one instance must not answer for another")
	}
	if _, ok := cache.Get(instanceA, token); !ok {
		t.Error("the instance it was verified against must still hit")
	}

	// Eviction is scoped the same way, or one instance's revocation would
	// silently drop another's still-valid entry.
	cache.Put(instanceB, token, &auth.TokenInfo{UserID: "2"}, 5*time.Minute)
	cache.Evict(instanceA, token)
	if _, ok := cache.Get(instanceB, token); !ok {
		t.Error("evicting one instance's entry must leave the other's alone")
	}
}

// TestTokenCache_Cleanup verifies that Cleanup removes all expired entries
// in a single pass while leaving still-valid entries untouched.
func TestTokenCache_Cleanup(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "expired-1", &auth.TokenInfo{UserID: "1"}, 0)
	cache.Put(testInstance, "expired-2", &auth.TokenInfo{UserID: "2"}, 0)
	cache.Put(testInstance, "valid", &auth.TokenInfo{UserID: "3"}, 5*time.Minute)

	cache.Cleanup()

	if cache.Len() != 1 {
		t.Errorf("Len() = %d after cleanup, want 1", cache.Len())
	}

	_, ok := cache.Get(testInstance, "valid")
	if !ok {
		t.Fatal("expected valid entry to survive cleanup")
	}
}

// TestTokenCache_SHA256Isolation verifies that distinct token strings are
// stored under distinct cache keys so their identities do not collide.
func TestTokenCache_SHA256Isolation(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "token-A", &auth.TokenInfo{UserID: "100"}, 5*time.Minute)
	cache.Put(testInstance, "token-B", &auth.TokenInfo{UserID: "200"}, 5*time.Minute)

	gotA, ok := cache.Get(testInstance, "token-A")
	if !ok {
		t.Fatal("expected hit for token-A")
	}
	gotB, ok := cache.Get(testInstance, "token-B")
	if !ok {
		t.Fatal("expected hit for token-B")
	}

	if gotA.UserID == gotB.UserID {
		t.Error("different tokens should map to different cache entries")
	}
}

// TestTokenCache_Delete verifies that the Delete alias delegates to Evict
// and removes the cache entry for the given token.
func TestTokenCache_Delete(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "del-token", &auth.TokenInfo{UserID: "99"}, 5*time.Minute)

	cache.Delete(testInstance, "del-token")

	_, ok := cache.Get(testInstance, "del-token")
	if ok {
		t.Fatal("expected cache miss after Delete")
	}
}

// TestTokenCache_Len_NonEmpty verifies that Len returns the correct count
// when the cache contains entries (including potentially expired ones).
func TestTokenCache_Len_NonEmpty(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put(testInstance, "a", &auth.TokenInfo{UserID: "1"}, 5*time.Minute)
	cache.Put(testInstance, "b", &auth.TokenInfo{UserID: "2"}, 5*time.Minute)
	cache.Put(testInstance, "c", &auth.TokenInfo{UserID: "3"}, 0) // expired

	if got := cache.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3 (includes expired)", got)
	}
}

// TestTokenCache_Len_Empty verifies that Len returns 0 for a fresh cache.
func TestTokenCache_Len_Empty(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	if got := cache.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
}

// TestTokenCache_ConcurrentAccess exercises Put, Get, Evict and Cleanup
// concurrently across many goroutines to surface data races under -race.
func TestTokenCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			token := "concurrent-token"
			info := &auth.TokenInfo{UserID: "42"}
			cache.Put(testInstance, token, info, 5*time.Minute)
			cache.Get(testInstance, token)
			if n%3 == 0 {
				cache.Evict(testInstance, token)
			}
			if n%5 == 0 {
				cache.Cleanup()
			}
		}(i)
	}
	wg.Wait()
}

// TestRunCleanup_EvictsEntriesNobodyComesBackFor pins that an expired entry
// nobody reads again is dropped by the sweep, without waiting for the cache to
// fill.
//
// Get evicts lazily, which handles a token that returns: its entry is dropped
// the next time it is looked up. It does nothing for one that does not. Every
// bearer a deployment has ever verified held a map entry for the process's
// lifetime, so a scanner walking a public endpoint with fresh credentials, or a
// fleet whose tokens rotate, grew the map without bound. Two documentation
// pages described a background sweep, at 30 seconds on one and 5 minutes on
// the other, and neither existed. The capacity now bounds how many entries
// there can be; the sweep is what gives their memory back once the traffic
// that stored them has stopped.
func TestRunCleanup_EvictsEntriesNobodyComesBackFor(t *testing.T) {
	t.Parallel()

	cache := NewTokenCache()
	cache.Put("https://gitlab.example.com", "expired-token", &auth.TokenInfo{}, time.Millisecond)
	cache.Put("https://gitlab.example.com", "live-token", &auth.TokenInfo{}, time.Hour)
	if got := cache.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		cache.RunCleanup(ctx, time.Millisecond)
	}()

	deadline := time.After(5 * time.Second)
	for cache.Len() > 1 {
		select {
		case <-deadline:
			t.Fatal("the expired entry was never swept; nothing but a repeat lookup would ever remove it")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	if _, ok := cache.Get("https://gitlab.example.com", "live-token"); !ok {
		t.Error("the sweep removed an entry that had not expired")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("RunCleanup ignored context cancellation")
	}
}

// TestRunCleanup_NonPositiveIntervalReturns verifies the guard against a ticker
// built from a zero or negative period, which panics in the standard library.
//
// Both shapes are covered because they arrive differently: zero is what a
// caller passes to mean "disabled", while a negative value is what arithmetic
// on a misconfigured TTL produces.
func TestRunCleanup_NonPositiveIntervalReturns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "zero", interval: 0},
		{name: "negative", interval: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				NewTokenCache().RunCleanup(t.Context(), tt.interval)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatalf("RunCleanup did not return for interval %v", tt.interval)
			}
		})
	}
}
