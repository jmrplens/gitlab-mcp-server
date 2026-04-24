// ratelimit_test.go contains unit tests for the HTTP server per-IP rate
// limiter, verifying token-bucket behavior and cleanup of idle limiters.

package serverpool

import (
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
