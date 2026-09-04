package serverpool

import (
	"log/slog"
	"sync"
	"time"
)

// maxTrackedAuthSources bounds the failure table.
//
// The key is chosen by the caller wherever a trusted-proxy header is
// configured, so without a ceiling the table grows with the number of distinct
// header values an attacker cares to send and is only ever pruned by [Cleanup]
// on its own schedule. The secondary budget in the HTTP front door bounds a
// real source to a few hundred distinct primary keys per window, so this admits
// several such sources at once and still costs a few hundred kilobytes at the
// ceiling.
const maxTrackedAuthSources = 4096

// AuthRateLimiter tracks authentication failures per client IP and blocks
// clients that exceed the maximum failure count within the configured window.
//
// The table is capped at [maxTrackedAuthSources] entries. At the cap a new key
// is not tracked rather than an existing record being evicted to make room:
// the records already there are the ones carrying evidence, and dropping one
// for a key the caller has just invented is precisely how a block would be
// cleared on demand. Saturating the table therefore costs the attacker their
// own accumulated count and buys them nothing, while the front door's second
// budget keeps bounding the source it actually came from: that one is keyed
// on the accepted socket, which no header can change.
type AuthRateLimiter struct {
	mu       sync.Mutex
	failures map[string]*failureRecord
	maxFails int
	window   time.Duration
	// lastSweepAt is when the table was last pruned, from either path. The
	// insert path consults it so a flood cannot make the cap its own
	// amplifier: see [AuthRateLimiter.roomForNewKeyLocked].
	lastSweepAt time.Time
	// warnedAtCap keeps the saturation warning to one line per episode rather
	// than one per refused insert, which under the flood that causes it would
	// be the loudest thing in the log. It clears as soon as the table has room
	// again, so a second episode is reported.
	warnedAtCap bool
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
	if ok && time.Since(rec.firstAt) <= l.window {
		rec.count++
		return
	}
	// A lapsed record is replaced in place, which is not growth. Only a key the
	// table does not hold has to fit under the cap.
	if !ok && !l.roomForNewKeyLocked() {
		return
	}
	l.failures[ip] = &failureRecord{count: 1, firstAt: time.Now()}
}

// roomForNewKeyLocked reports whether a key the table does not yet hold may be
// added, sweeping lapsed records first so the cap is a ceiling on live keys
// rather than on everything ever seen. The caller holds l.mu.
func (l *AuthRateLimiter) roomForNewKeyLocked() bool {
	if len(l.failures) < maxTrackedAuthSources {
		return true
	}
	// Sweeping is linear in the table size and the only thing that reaches this
	// branch is a flood, so sweeping on every refused insert would make the cap
	// its own amplifier: each spoofed key would cost a full pass over the
	// ceiling. Nothing can lapse faster than the window, so a bounded number of
	// sweeps per window recovers the same space for a fixed cost.
	now := time.Now()
	if now.Sub(l.lastSweepAt) >= l.window/8 {
		l.lastSweepAt = now
		l.sweepLocked(now)
	}
	if len(l.failures) < maxTrackedAuthSources {
		l.warnedAtCap = false
		return true
	}
	if !l.warnedAtCap {
		l.warnedAtCap = true
		slog.Warn("authentication failure table is full; new sources are not being counted",
			"tracked", len(l.failures),
			"limit", maxTrackedAuthSources,
			"window", l.window,
		)
	}
	return false
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

	l.lastSweepAt = time.Now()
	l.sweepLocked(l.lastSweepAt)
	if len(l.failures) < maxTrackedAuthSources {
		l.warnedAtCap = false
	}
}

// sweepLocked drops every record whose window has lapsed. The caller holds
// l.mu.
func (l *AuthRateLimiter) sweepLocked(now time.Time) {
	for ip, rec := range l.failures {
		if now.Sub(rec.firstAt) > l.window {
			delete(l.failures, ip)
		}
	}
}
