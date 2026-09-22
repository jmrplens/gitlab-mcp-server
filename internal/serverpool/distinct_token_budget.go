package serverpool

import (
	"crypto/sha256"
	"log/slog"
	"sync"
	"time"
)

// distinctDigestLen is how much of the SHA-256 digest of a credential is kept
// to tell one refused token from another.
//
// The question this budget asks of a token is only whether it is the same one
// as the last, so the digest needs to make a collision unlikely and nothing
// else: it is never compared against a stored secret and never leaves the
// process. Sixteen bytes is 128 bits, which no caller reaches by chance, and
// it is a quarter of what the hex spelling the rest of the tree stores would
// cost per entry. At the ceilings below the whole table is a few megabytes
// rather than a few tens.
const distinctDigestLen = 16

// escalationLadder is how much longer each block lasts than the one before it,
// as multiples of the budget's step.
//
// With the default step of one minute this is the minute, the ten minutes and
// the hour the escalation was specified as. The last entry repeats: an
// attacker who keeps going after the third block stays on the hour rather than
// being blocked for a day, because the block is a defence and not a
// punishment, and a permanent one lands on whoever inherits the address.
var escalationLadder = [...]int{1, 10, 60}

// DistinctTokenBudget counts the distinct credentials one address has had
// refused inside a window, and blocks that address for longer each time the
// count is reached.
//
// It is the slow half of the pair whose fast half is [AuthRateLimiter], and
// the two answer different questions on purpose. Ten failures in a minute is
// a stuck client retrying one bad token as much as it is an attack, so the
// answer to it is a block that lasts a minute and forgives. Fifty *distinct*
// invalid tokens from one address inside ten minutes is only an attack: a
// person has one token, and a fleet behind a NAT has one each, so the distinct
// count is the one thing a legitimate neighbour never produces and a sprayer
// cannot avoid producing.
//
// What it protects is not really this server, which refuses those requests
// cheaply either way. Every distinct token that reaches verification is a
// request to GitLab from the deployment's own address, and GitLab rate-limits
// failed authentication per source address; a sprayer is therefore spending
// the deployment's standing with GitLab, and the throttle GitLab eventually
// applies lands on the server for everyone using it. The distinct count is the
// earliest signal of that, earlier than the failure count.
//
// Both tables are bounded, and both ceilings are reached by the same reasoning
// as [AuthRateLimiter]'s: the keys come from whoever is calling. The address
// table shares that limiter's cap, and the digest set per address needs no cap
// of its own because reaching the limit both blocks the address and clears the
// set, so it cannot hold more than limit entries.
//
// A nil *DistinctTokenBudget is usable and answers "not blocked" to
// everything, which is what a deployment that has turned the budget off gets.
type DistinctTokenBudget struct {
	mu        sync.Mutex
	addresses map[string]*distinctRecord
	limit     int
	window    time.Duration
	// step is the first block's length; the ladder above multiplies it.
	step time.Duration
	// lastSweepAt and warnedAtCap carry the same meaning as their namesakes
	// in [AuthRateLimiter]: the insert path sweeps at most once every
	// window/8 so that the cap cannot become an amplifier, and the
	// saturation warning is one line per episode.
	lastSweepAt time.Time
	warnedAtCap bool
}

// distinctRecord is one address's state.
type distinctRecord struct {
	// digests holds the distinct credentials refused inside the current
	// window. It is cleared when the window lapses and when a block is
	// raised, so its size is bounded by the limit.
	digests map[[distinctDigestLen]byte]struct{}
	// windowStartedAt is when the current counting window opened.
	windowStartedAt time.Time
	// step is how many blocks this address has been given; it indexes
	// [escalationLadder], saturating at its last entry.
	step int
	// blockedUntil is when the current block lifts. Zero when none is on.
	blockedUntil time.Time
	// lastChargeAt is when this address last had a credential refused, which
	// is what the silence that resets the ladder is measured from.
	lastChargeAt time.Time
}

// NewDistinctTokenBudget returns a budget that blocks an address once limit
// distinct credentials of its have been refused inside window, for step, then
// ten times step, then sixty times step, resetting after sixty times step of
// silence.
//
// A limit of zero or less turns it off, and so does a window or step of zero
// or less: the constructor returns nil rather than a live object with limits
// nothing can reach, so "off" is one state rather than several.
func NewDistinctTokenBudget(limit int, window, step time.Duration) *DistinctTokenBudget {
	if limit <= 0 || window <= 0 || step <= 0 {
		return nil
	}
	return &DistinctTokenBudget{
		addresses: make(map[string]*distinctRecord),
		limit:     limit,
		window:    window,
		step:      step,
	}
}

// resetAfter is how long an address must stay silent before its ladder is
// forgotten: the longest block the ladder can impose.
func (b *DistinctTokenBudget) resetAfter() time.Duration {
	return time.Duration(escalationLadder[len(escalationLadder)-1]) * b.step
}

// Charge records that the given address has had this credential refused, and
// reports whether that charge raised a new block.
//
// The return value exists for the caller's telemetry: a block is an event
// worth counting once, and counting it on every later refusal from the same
// address would report the block's length rather than its occurrence.
func (b *DistinctTokenBudget) Charge(address, token string) bool {
	if b == nil || address == "" || token == "" {
		return false
	}

	sum := sha256.Sum256([]byte(token))
	var digest [distinctDigestLen]byte
	copy(digest[:], sum[:])

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	rec, ok := b.addresses[address]
	switch {
	case !ok:
		if !b.roomForNewKeyLocked(now) {
			return false
		}
		rec = &distinctRecord{
			digests:         make(map[[distinctDigestLen]byte]struct{}, 1),
			windowStartedAt: now,
		}
		b.addresses[address] = rec
	case now.Sub(rec.lastChargeAt) >= b.resetAfter():
		// Silence long enough to forget the ladder. The record is reused
		// rather than replaced so that this path costs the same whether the
		// table is at its cap or not: nothing new is being inserted.
		*rec = distinctRecord{
			digests:         make(map[[distinctDigestLen]byte]struct{}, 1),
			windowStartedAt: now,
		}
	case now.Sub(rec.windowStartedAt) > b.window:
		clear(rec.digests)
		rec.windowStartedAt = now
	}
	rec.lastChargeAt = now

	rec.digests[digest] = struct{}{}
	if len(rec.digests) < b.limit {
		return false
	}

	// The limit is reached: raise the next block and start a fresh window, so
	// a sprayer who keeps going is judged on what it sends next rather than on
	// what it has already been blocked for.
	clear(rec.digests)
	rec.windowStartedAt = now
	if rec.step < len(escalationLadder) {
		rec.step++
	}
	rec.blockedUntil = now.Add(b.blockFor(rec.step))
	return true
}

// blockFor is how long the step-th block lasts.
//
// step is 1-based and never leaves the ladder: [DistinctTokenBudget.Charge] is
// the only writer and saturates it at the ladder's length, which is what makes
// the last rung repeat. A bounds check here would be a second guard on that
// same invariant, reachable by no program.
func (b *DistinctTokenBudget) blockFor(step int) time.Duration {
	return time.Duration(escalationLadder[step-1]) * b.step
}

// Blocked reports whether the address is currently blocked, and for how much
// longer, which is what the caller answers Retry-After with.
func (b *DistinctTokenBudget) Blocked(address string) (bool, time.Duration) {
	if b == nil {
		return false, 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	rec, ok := b.addresses[address]
	if !ok || rec.blockedUntil.IsZero() {
		return false, 0
	}
	remaining := time.Until(rec.blockedUntil)
	if remaining <= 0 {
		// The block has lifted. The record stays, because the ladder it
		// carries is the whole point: the next block is longer than this one
		// was, and only silence clears that.
		rec.blockedUntil = time.Time{}
		return false, 0
	}
	return true, remaining
}

// roomForNewKeyLocked reports whether an address the table does not hold may
// be added, sweeping first if it has not swept recently. The caller holds
// b.mu.
//
// It is [AuthRateLimiter.roomForNewKeyLocked] applied to this table, for the
// same reasons: at the cap a new key is refused rather than an existing record
// evicted, because the records already there are the ones carrying evidence
// and dropping one on demand is how a block would be cleared.
func (b *DistinctTokenBudget) roomForNewKeyLocked(now time.Time) bool {
	if len(b.addresses) < maxTrackedAuthSources {
		return true
	}
	if now.Sub(b.lastSweepAt) >= b.window/8 {
		b.lastSweepAt = now
		b.sweepLocked(now)
	}
	if len(b.addresses) < maxTrackedAuthSources {
		b.warnedAtCap = false
		return true
	}
	if !b.warnedAtCap {
		b.warnedAtCap = true
		slog.Warn("distinct-token budget table is full; new addresses are not being counted",
			"tracked", len(b.addresses),
			"limit", maxTrackedAuthSources,
			"window", b.window,
		)
	}
	return false
}

// Cleanup removes every record that has gone quiet long enough to be
// forgotten. Call it on the same schedule as [AuthRateLimiter.Cleanup].
func (b *DistinctTokenBudget) Cleanup() {
	if b == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.lastSweepAt = time.Now()
	b.sweepLocked(b.lastSweepAt)
	if len(b.addresses) < maxTrackedAuthSources {
		b.warnedAtCap = false
	}
}

// sweepLocked drops every record whose silence has outlasted the ladder and
// whose block has lifted. The caller holds b.mu.
//
// A blocked address is kept whatever its silence says: the block is the reason
// it is silent, and dropping it would end the block early.
func (b *DistinctTokenBudget) sweepLocked(now time.Time) {
	for address, rec := range b.addresses {
		if now.Before(rec.blockedUntil) {
			continue
		}
		if now.Sub(rec.lastChargeAt) >= b.resetAfter() {
			delete(b.addresses, address)
		}
	}
}

// Len reports how many addresses are tracked, for tests and for the
// saturation warning's sake.
func (b *DistinctTokenBudget) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.addresses)
}
