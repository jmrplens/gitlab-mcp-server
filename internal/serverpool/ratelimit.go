// ratelimit.go implements per-IP rate limiting for the HTTP MCP server,
// protecting against abuse of the server pool endpoint.

package serverpool

import (
	"sync"
	"time"
)

// AuthRateLimiter tracks authentication failures per client IP and blocks
// clients that exceed the maximum failure count within the configured window.
type AuthRateLimiter struct {
	mu       sync.Mutex
	failures map[string]*failureRecord
	maxFails int
	window   time.Duration
}

type failureRecord struct {
	count   int
	firstAt time.Time
}

// NewAuthRateLimiter creates a rate limiter that blocks a client IP after
// maxFails authentication failures within the given time window.
func NewAuthRateLimiter(maxFails int, window time.Duration) *AuthRateLimiter {
	return &AuthRateLimiter{
		failures: make(map[string]*failureRecord),
		maxFails: maxFails,
		window:   window,
	}
}

// RecordFailure records an authentication failure for the given IP.
func (l *AuthRateLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	rec, ok := l.failures[ip]
	if !ok || time.Since(rec.firstAt) > l.window {
		l.failures[ip] = &failureRecord{count: 1, firstAt: time.Now()}
		return
	}
	rec.count++
}

// IsBlocked returns true if the IP has exceeded the failure limit within the window.
func (l *AuthRateLimiter) IsBlocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	rec, ok := l.failures[ip]
	if !ok {
		return false
	}
	if time.Since(rec.firstAt) > l.window {
		delete(l.failures, ip)
		return false
	}
	return rec.count >= l.maxFails
}

// Cleanup removes expired entries. Call periodically to prevent memory growth.
func (l *AuthRateLimiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for ip, rec := range l.failures {
		if now.Sub(rec.firstAt) > l.window {
			delete(l.failures, ip)
		}
	}
}
