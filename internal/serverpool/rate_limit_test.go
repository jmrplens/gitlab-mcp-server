// rate_limit_test.go contains unit tests for the HTTP server per-IP rate
// limiter, verifying token-bucket behavior and cleanup of idle limiters.
package serverpool

import (
	"strconv"
	"testing"
	"time"
)

// TestAuthRateLimiter_BlocksAfterMaxFailures verifies that an IP is blocked
// after exceeding the maximum number of authentication failures.
func TestAuthRateLimiter_BlocksAfterMaxFailures(t *testing.T) {
	limiter := NewAuthRateLimiter(3, 1*time.Minute)

	if limiter.IsBlocked("1.2.3.4") {
		t.Fatal("expected IP to not be blocked initially")
	}

	limiter.RecordFailure("1.2.3.4")
	limiter.RecordFailure("1.2.3.4")
	if limiter.IsBlocked("1.2.3.4") {
		t.Fatal("expected IP to not be blocked after 2 failures (max=3)")
	}

	limiter.RecordFailure("1.2.3.4")
	if !limiter.IsBlocked("1.2.3.4") {
		t.Fatal("expected IP to be blocked after 3 failures")
	}
}

// TestAuthRateLimiter_WindowExpiry verifies that the rate limiter resets
// after the time window expires.
func TestAuthRateLimiter_WindowExpiry(t *testing.T) {
	limiter := NewAuthRateLimiter(2, 50*time.Millisecond)

	limiter.RecordFailure("10.0.0.1")
	limiter.RecordFailure("10.0.0.1")
	if !limiter.IsBlocked("10.0.0.1") {
		t.Fatal("expected IP to be blocked")
	}

	time.Sleep(60 * time.Millisecond)

	if limiter.IsBlocked("10.0.0.1") {
		t.Fatal("expected IP to be unblocked after window expiry")
	}
}

// TestAuthRateLimiter_IndependentIPs verifies that rate limiting
// is tracked independently per IP address.
func TestAuthRateLimiter_IndependentIPs(t *testing.T) {
	limiter := NewAuthRateLimiter(2, 1*time.Minute)

	limiter.RecordFailure("1.1.1.1")
	limiter.RecordFailure("1.1.1.1")

	if !limiter.IsBlocked("1.1.1.1") {
		t.Fatal("expected 1.1.1.1 to be blocked")
	}
	if limiter.IsBlocked("2.2.2.2") {
		t.Fatal("expected 2.2.2.2 to not be blocked")
	}
}

// TestAuthRateLimiter_Cleanup verifies that expired entries are removed.
func TestAuthRateLimiter_Cleanup(t *testing.T) {
	limiter := NewAuthRateLimiter(1, 50*time.Millisecond)

	limiter.RecordFailure("10.0.0.1")
	limiter.RecordFailure("10.0.0.2")

	time.Sleep(60 * time.Millisecond)

	limiter.Cleanup()

	limiter.mu.Lock()
	count := len(limiter.failures)
	limiter.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 entries after cleanup, got %d", count)
	}
}

// tableSize returns how many keys the limiter is currently tracking.
func tableSize(t *testing.T, limiter *AuthRateLimiter) int {
	t.Helper()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	return len(limiter.failures)
}

// TestAuthRateLimiter_StopsGrowingAtTheCap verifies the failure table has a
// ceiling.
//
// The key is caller-chosen wherever a trusted-proxy header is configured, so
// before the cap existed the table grew with the number of distinct header
// values an attacker cared to send and shrank only when Cleanup happened to
// run. Recording twice the cap in distinct keys inside one window is the shape
// of that flood.
func TestAuthRateLimiter_StopsGrowingAtTheCap(t *testing.T) {
	limiter := NewAuthRateLimiter(10, 1*time.Minute)

	for i := range maxTrackedAuthSources * 2 {
		limiter.RecordFailure("spoofed-" + strconv.Itoa(i))
	}

	if got := tableSize(t, limiter); got != maxTrackedAuthSources {
		t.Errorf("tracked keys = %d, want %d", got, maxTrackedAuthSources)
	}
}

// TestAuthRateLimiter_CapDoesNotClearAnExistingBlock verifies a saturating
// flood cannot evict the record that is already blocking someone.
//
// This is the reason the cap refuses new keys instead of evicting old ones. An
// eviction policy would hand an attacker a way to clear their own block on
// demand: exceed the limit, then mint enough fresh keys to push the record that
// holds the count out of the table.
func TestAuthRateLimiter_CapDoesNotClearAnExistingBlock(t *testing.T) {
	limiter := NewAuthRateLimiter(3, 1*time.Minute)

	for range 3 {
		limiter.RecordFailure("10.0.0.1")
	}
	if !limiter.IsBlocked("10.0.0.1") {
		t.Fatal("10.0.0.1 should be blocked before the flood")
	}

	for i := range maxTrackedAuthSources * 2 {
		limiter.RecordFailure("spoofed-" + strconv.Itoa(i))
	}

	if !limiter.IsBlocked("10.0.0.1") {
		t.Error("10.0.0.1 stopped being blocked after the table saturated")
	}
}

// TestAuthRateLimiter_CapAdmitsNewKeysOnceRecordsLapse verifies the ceiling is
// on live keys rather than on everything ever seen: once the records in the
// table have lapsed, a key it would have refused is admitted again.
//
// Without the sweep on the insert path the table would stay permanently full
// after one flood and the limiter would stop counting anybody for the life of
// the process, blocking nothing while looking exactly like it was working.
//
// The records are aged by hand rather than by sleeping. A window short enough
// to wait out is also short enough that records lapse while the table is still
// being filled, so the test would be racing its own setup.
func TestAuthRateLimiter_CapAdmitsNewKeysOnceRecordsLapse(t *testing.T) {
	limiter := NewAuthRateLimiter(10, 1*time.Minute)

	for i := range maxTrackedAuthSources + 1 {
		limiter.RecordFailure("spoofed-" + strconv.Itoa(i))
	}
	if got := tableSize(t, limiter); got != maxTrackedAuthSources {
		t.Fatalf("tracked keys = %d, want the table full at %d", got, maxTrackedAuthSources)
	}

	limiter.mu.Lock()
	lapsed := time.Now().Add(-2 * limiter.window)
	for _, rec := range limiter.failures {
		rec.firstAt = lapsed
	}
	limiter.lastSweepAt = lapsed
	limiter.mu.Unlock()

	limiter.RecordFailure("10.0.0.9")

	if limiter.IsBlocked("10.0.0.9") {
		t.Fatal("one failure should not block 10.0.0.9")
	}
	if got := tableSize(t, limiter); got != 1 {
		t.Errorf("tracked keys = %d, want only the one live record", got)
	}
}
