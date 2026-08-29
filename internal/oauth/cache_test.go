// cache_test.go contains unit tests for the OAuth token identity cache,
// verifying TTL expiration, concurrent access, and eviction behavior.
package oauth

import (
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
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
