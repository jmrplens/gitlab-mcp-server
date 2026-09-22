package serverpool

import (
	"strconv"
	"testing"
	"time"
)

// TestNewDistinctTokenBudget_OffSettings_ReturnOneNilBudget checks that every
// way of asking for no budget produces the same thing.
//
// The constructor returns nil rather than a live object whose limits nothing
// can reach, so a caller has one "off" state to reason about and every method
// tolerates it.
func TestNewDistinctTokenBudget_OffSettings_ReturnOneNilBudget(t *testing.T) {
	cases := []struct {
		name   string
		limit  int
		window time.Duration
		step   time.Duration
	}{
		{name: "zero limit", limit: 0, window: time.Minute, step: time.Minute},
		{name: "negative limit", limit: -1, window: time.Minute, step: time.Minute},
		{name: "zero window", limit: 5, window: 0, step: time.Minute},
		{name: "negative window", limit: 5, window: -time.Minute, step: time.Minute},
		{name: "zero step", limit: 5, window: time.Minute, step: 0},
		{name: "negative step", limit: 5, window: time.Minute, step: -time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewDistinctTokenBudget(tc.limit, tc.window, tc.step); got != nil {
				t.Fatalf("NewDistinctTokenBudget(%d, %v, %v) = %v, want nil",
					tc.limit, tc.window, tc.step, got)
			}
		})
	}
}

// TestDistinctTokenBudget_NilIsUsable checks that the nil budget answers every
// method rather than panicking, which is what a deployment with the budget
// turned off relies on.
func TestDistinctTokenBudget_NilIsUsable(t *testing.T) {
	var b *DistinctTokenBudget

	if b.Charge("10.0.0.1", "glpat-x") {
		t.Error("a nil budget must never report a block")
	}
	if blocked, d := b.Blocked("10.0.0.1"); blocked || d != 0 {
		t.Errorf("Blocked = (%v, %v), want (false, 0)", blocked, d)
	}
	if got := b.Len(); got != 0 {
		t.Errorf("Len = %d, want 0", got)
	}
	b.Cleanup()
}

// TestDistinctTokenBudget_DistinctTokensBlock_SameTokenDoesNot is the whole
// point of the budget: the count is of distinct credentials, so a client
// retrying one bad token forever never reaches it while a sprayer reaches it
// immediately.
func TestDistinctTokenBudget_DistinctTokensBlock_SameTokenDoesNot(t *testing.T) {
	t.Run("one token repeated", func(t *testing.T) {
		b := NewDistinctTokenBudget(3, time.Minute, time.Minute)
		for range 20 {
			if b.Charge("10.0.0.1", "glpat-the-same-one") {
				t.Fatal("repeating one token must never raise a block")
			}
		}
		if blocked, _ := b.Blocked("10.0.0.1"); blocked {
			t.Error("address blocked after retrying a single token")
		}
	})

	t.Run("distinct tokens", func(t *testing.T) {
		b := NewDistinctTokenBudget(3, time.Minute, time.Minute)
		raised := 0
		for i := range 3 {
			if b.Charge("10.0.0.1", "glpat-"+strconv.Itoa(i)) {
				raised++
			}
		}
		if raised != 1 {
			t.Errorf("blocks raised = %d, want exactly 1", raised)
		}
		blocked, remaining := b.Blocked("10.0.0.1")
		if !blocked {
			t.Fatal("three distinct tokens did not block an address at a limit of three")
		}
		if remaining <= 0 || remaining > time.Minute {
			t.Errorf("Retry-After = %v, want (0, 1m]", remaining)
		}
	})
}

// TestDistinctTokenBudget_BudgetIsPerAddress checks that one address's spray
// does not spend another's allowance.
func TestDistinctTokenBudget_BudgetIsPerAddress(t *testing.T) {
	b := NewDistinctTokenBudget(3, time.Minute, time.Minute)

	for i := range 3 {
		b.Charge("10.0.0.1", "glpat-"+strconv.Itoa(i))
	}
	if blocked, _ := b.Blocked("10.0.0.1"); !blocked {
		t.Fatal("the spraying address was not blocked")
	}
	if blocked, _ := b.Blocked("10.0.0.2"); blocked {
		t.Error("a neighbor was blocked by another address's spray")
	}
}

// TestDistinctTokenBudget_EscalationLengthensThenSaturates walks the ladder:
// each block is longer than the last, and past its end the length stops
// growing rather than becoming a de facto permanent ban on an address whoever
// inherits it did nothing to earn.
func TestDistinctTokenBudget_EscalationLengthensThenSaturates(t *testing.T) {
	const step = time.Minute
	b := NewDistinctTokenBudget(2, time.Minute, step)

	want := []time.Duration{step, 10 * step, 60 * step, 60 * step}
	// sequential: each round climbs the ladder the previous round raised
	for round, wantLen := range want {
		// Two distinct tokens raise the next block. The record's own clock is
		// wound back first so the previous block has lifted, which is what a
		// real attacker waiting one out produces.
		b.mu.Lock()
		if rec, ok := b.addresses["10.0.0.1"]; ok {
			rec.blockedUntil = time.Now().Add(-time.Second)
		}
		b.mu.Unlock()

		b.Charge("10.0.0.1", "glpat-r"+strconv.Itoa(round)+"-a")
		b.Charge("10.0.0.1", "glpat-r"+strconv.Itoa(round)+"-b")

		blocked, remaining := b.Blocked("10.0.0.1")
		if !blocked {
			t.Fatalf("round %d: the address was not blocked", round)
		}
		// The remaining time is measured from a moment after the block was
		// set, so it is at most the intended length and within a whisker of it.
		if remaining > wantLen || remaining < wantLen-time.Second {
			t.Errorf("round %d: block = %v, want about %v", round, remaining, wantLen)
		}
	}
}

// TestDistinctTokenBudget_ChargesWhileBlocked_DoNotClimbTheLadder covers the
// race between the caller's own Blocked check and this charge, which are two
// critical sections with a request's authentication in between.
//
// A burst that passes the check together, before any of it has failed, arrives
// here afterwards. Counting it would let 3*limit concurrent refusals reach the
// second and third rungs while the first block is still on, so a single burst
// would earn the hour without the sender ever having been told it was blocked
// once. The ladder answers persistence after a block, and a charge that lands
// during one is not that.
func TestDistinctTokenBudget_ChargesWhileBlocked_DoNotClimbTheLadder(t *testing.T) {
	const step = time.Minute
	b := NewDistinctTokenBudget(2, time.Minute, step)
	const address = "10.0.0.9"

	// The first two distinct tokens raise the first block.
	b.Charge(address, "glpat-a")
	b.Charge(address, "glpat-b")
	blocked, first := b.Blocked(address)
	if !blocked {
		t.Fatal("the address was not blocked by the first pair")
	}
	if first > step || first < step-time.Second {
		t.Fatalf("first block = %v, want about %v", first, step)
	}

	// Four more distinct tokens arrive while that block is on, which is twice
	// what the limit asks for and would be two more rungs if they counted.
	// sequential: one burst accumulating against the block, not four cases
	for _, token := range []string{"glpat-c", "glpat-d", "glpat-e", "glpat-f"} {
		if raised := b.Charge(address, token); raised {
			t.Errorf("Charge(%q) reported a new block while one was already on", token)
		}
	}

	blocked, remaining := b.Blocked(address)
	if !blocked {
		t.Fatal("the address stopped being blocked while charges were arriving")
	}
	if remaining > step {
		t.Errorf("block = %v after charges made during it, want no longer than the first %v", remaining, step)
	}
}

// TestDistinctTokenBudget_HammeringThroughABlock_IsNotSilence covers the gap
// between the two calls a blocked address makes.
//
// A blocked caller is refused before its credential is read, so Charge never
// sees it and only Blocked does. Measuring the silence that forgives the
// ladder from the last charge therefore read an address that kept hammering
// throughout its block as having been silent for the whole of it, and handed
// back a clean ladder the moment the block lifted. Persistence through a block
// is the opposite of silence.
func TestDistinctTokenBudget_HammeringThroughABlock_IsNotSilence(t *testing.T) {
	const step = time.Minute
	b := NewDistinctTokenBudget(2, time.Minute, step)
	const address = "10.0.0.11"

	b.Charge(address, "glpat-a")
	b.Charge(address, "glpat-b")
	if blocked, _ := b.Blocked(address); !blocked {
		t.Fatal("the address was not blocked by the first pair")
	}

	// Wind the record back so the ladder would be forgiven on silence alone,
	// then have the address ask once, which is what a blocked caller does.
	b.mu.Lock()
	rec := b.addresses[address]
	rec.lastSeenAt = time.Now().Add(-2 * b.resetAfter())
	b.mu.Unlock()

	if blocked, _ := b.Blocked(address); !blocked {
		t.Fatal("the block lifted early")
	}

	b.mu.Lock()
	seen := rec.lastSeenAt
	b.mu.Unlock()
	if time.Since(seen) > time.Minute {
		t.Errorf("asking while blocked did not count as activity: last seen %v ago", time.Since(seen))
	}

	// The block is wound down so the next pair is judged, and the ladder must
	// have been kept: the second block is ten steps, not another first one.
	b.mu.Lock()
	rec.blockedUntil = time.Now().Add(-time.Second)
	b.mu.Unlock()

	b.Charge(address, "glpat-c")
	b.Charge(address, "glpat-d")
	blocked, remaining := b.Blocked(address)
	if !blocked {
		t.Fatal("the second pair did not block")
	}
	if want := 10 * step; remaining > want || remaining < want-time.Second {
		t.Errorf("second block = %v, want about %v: the ladder was forgiven despite the hammering", remaining, want)
	}
}

// TestDistinctTokenBudget_SilenceResetsTheLadder checks the other end of the
// escalation: an address that stops is forgiven, so today's block does not
// make tomorrow's longer for a client that had one bad afternoon.
func TestDistinctTokenBudget_SilenceResetsTheLadder(t *testing.T) {
	const step = time.Minute
	b := NewDistinctTokenBudget(2, time.Minute, step)

	b.Charge("10.0.0.1", "glpat-a")
	b.Charge("10.0.0.1", "glpat-b")
	if blocked, remaining := b.Blocked("10.0.0.1"); !blocked || remaining > step {
		t.Fatalf("first block = (%v, %v), want blocked for at most %v", blocked, remaining, step)
	}

	// Wind the record back past the reset horizon, which is the longest block
	// the ladder imposes.
	b.mu.Lock()
	rec := b.addresses["10.0.0.1"]
	past := time.Now().Add(-b.resetAfter() - time.Second)
	rec.lastSeenAt = past
	rec.windowStartedAt = past
	rec.blockedUntil = past
	b.mu.Unlock()

	b.Charge("10.0.0.1", "glpat-c")
	b.Charge("10.0.0.1", "glpat-d")

	blocked, remaining := b.Blocked("10.0.0.1")
	if !blocked {
		t.Fatal("the address was not blocked after spraying again")
	}
	if remaining > step {
		t.Errorf("block after silence = %v, want the first rung of at most %v", remaining, step)
	}
}

// TestDistinctTokenBudget_WindowLapses_ForgetsTheCount checks that distinct
// tokens spread thinly enough never add up: the count is per window, so a
// client failing a handful of credentials a day is not eventually blocked by
// accumulation.
func TestDistinctTokenBudget_WindowLapses_ForgetsTheCount(t *testing.T) {
	b := NewDistinctTokenBudget(3, time.Minute, time.Minute)

	b.Charge("10.0.0.1", "glpat-a")
	b.Charge("10.0.0.1", "glpat-b")

	b.mu.Lock()
	b.addresses["10.0.0.1"].windowStartedAt = time.Now().Add(-2 * time.Minute)
	b.mu.Unlock()

	if b.Charge("10.0.0.1", "glpat-c") {
		t.Error("a charge in a fresh window raised a block on a lapsed count")
	}
	if blocked, _ := b.Blocked("10.0.0.1"); blocked {
		t.Error("the address was blocked by a count the window had already dropped")
	}
}

// TestDistinctTokenBudget_BlockLifts checks that a block ends on its own, and
// that the record survives it so the ladder is still remembered.
func TestDistinctTokenBudget_BlockLifts(t *testing.T) {
	b := NewDistinctTokenBudget(2, time.Minute, time.Minute)

	b.Charge("10.0.0.1", "glpat-a")
	b.Charge("10.0.0.1", "glpat-b")

	b.mu.Lock()
	b.addresses["10.0.0.1"].blockedUntil = time.Now().Add(-time.Millisecond)
	b.mu.Unlock()

	if blocked, remaining := b.Blocked("10.0.0.1"); blocked || remaining != 0 {
		t.Errorf("Blocked after the block lapsed = (%v, %v), want (false, 0)", blocked, remaining)
	}
	if b.Len() != 1 {
		t.Errorf("Len = %d, want the record kept so the ladder survives", b.Len())
	}
}

// TestDistinctTokenBudget_EmptyArguments_AreNotCharged checks the two inputs
// that name nothing. An empty address cannot be blocked meaningfully and an
// empty token is not a credential, so neither opens a record.
func TestDistinctTokenBudget_EmptyArguments_AreNotCharged(t *testing.T) {
	cases := []struct {
		name    string
		address string
		token   string
	}{
		{name: "no address", address: "", token: "glpat-a"},
		{name: "no token", address: "10.0.0.1", token: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewDistinctTokenBudget(1, time.Minute, time.Minute)
			if b.Charge(tc.address, tc.token) {
				t.Error("an empty argument raised a block")
			}
			if b.Len() != 0 {
				t.Errorf("Len = %d, want 0", b.Len())
			}
		})
	}
}

// TestDistinctTokenBudget_StopsGrowingAtTheCap checks that the address table
// is bounded, since its keys come from whoever is calling.
func TestDistinctTokenBudget_StopsGrowingAtTheCap(t *testing.T) {
	b := NewDistinctTokenBudget(1000, time.Minute, time.Minute)

	for i := range maxTrackedAuthSources + 100 {
		b.Charge("10.0."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256), "glpat-"+strconv.Itoa(i))
	}
	if got := b.Len(); got > maxTrackedAuthSources {
		t.Errorf("tracked addresses = %d, want at most %d", got, maxTrackedAuthSources)
	}
}

// TestDistinctTokenBudget_CapDoesNotClearAnExistingBlock checks the direction
// the cap must not fail in: an attacker who saturates the table must not
// thereby drop the record that is blocking them.
func TestDistinctTokenBudget_CapDoesNotClearAnExistingBlock(t *testing.T) {
	b := NewDistinctTokenBudget(2, time.Minute, time.Minute)

	b.Charge("10.0.0.1", "glpat-a")
	b.Charge("10.0.0.1", "glpat-b")
	if blocked, _ := b.Blocked("10.0.0.1"); !blocked {
		t.Fatal("the address was not blocked to begin with")
	}

	for i := range maxTrackedAuthSources + 100 {
		b.Charge("172.16."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256), "glpat-flood-"+strconv.Itoa(i))
	}

	if blocked, _ := b.Blocked("10.0.0.1"); !blocked {
		t.Error("saturating the table cleared an existing block")
	}
}

// TestDistinctTokenBudget_Cleanup_DropsQuietRecordsAndKeepsBlockedOnes checks
// both halves of the sweep: a record that has gone quiet long enough is
// forgotten, and a blocked one is kept however quiet it is, because the block
// is the reason for the quiet.
func TestDistinctTokenBudget_Cleanup_DropsQuietRecordsAndKeepsBlockedOnes(t *testing.T) {
	b := NewDistinctTokenBudget(2, time.Minute, time.Minute)

	b.Charge("10.0.0.1", "glpat-a")
	b.Charge("10.0.0.2", "glpat-b")
	b.Charge("10.0.0.2", "glpat-c")

	b.mu.Lock()
	past := time.Now().Add(-b.resetAfter() - time.Second)
	b.addresses["10.0.0.1"].lastSeenAt = past
	b.addresses["10.0.0.2"].lastSeenAt = past
	b.mu.Unlock()

	b.Cleanup()

	b.mu.Lock()
	_, quietKept := b.addresses["10.0.0.1"]
	_, blockedKept := b.addresses["10.0.0.2"]
	b.mu.Unlock()

	if quietKept {
		t.Error("a quiet, unblocked record survived the sweep")
	}
	if !blockedKept {
		t.Error("a blocked record was swept away, which would end its block early")
	}
}

// TestDistinctTokenBudget_CapAdmitsNewAddressesOnceRecordsLapse covers the
// other half of the cap: it is a ceiling on live records, not on everything
// ever seen, so a saturated table recovers on its own rather than refusing to
// count anybody until the process restarts.
func TestDistinctTokenBudget_CapAdmitsNewAddressesOnceRecordsLapse(t *testing.T) {
	b := NewDistinctTokenBudget(1000, time.Minute, time.Minute)

	for i := range maxTrackedAuthSources {
		b.Charge("10.0."+strconv.Itoa(i/256)+"."+strconv.Itoa(i%256), "glpat-"+strconv.Itoa(i))
	}
	if b.Len() < maxTrackedAuthSources {
		t.Fatalf("Len = %d, want the table filled to %d before the cap is tested", b.Len(), maxTrackedAuthSources)
	}

	// A new address is refused while every record is live.
	b.Charge("172.16.0.1", "glpat-refused")
	b.mu.Lock()
	_, admittedWhileFull := b.addresses["172.16.0.1"]
	// Wind every record past the reset horizon, and the last sweep back far
	// enough that the insert path is allowed to sweep again.
	past := time.Now().Add(-b.resetAfter() - time.Second)
	for _, rec := range b.addresses {
		rec.lastSeenAt = past
		rec.blockedUntil = time.Time{}
	}
	b.lastSweepAt = time.Time{}
	b.mu.Unlock()

	if admittedWhileFull {
		t.Error("a new address was tracked while the table was full of live records")
	}

	b.Charge("172.16.0.2", "glpat-admitted")
	b.mu.Lock()
	_, admittedAfterSweep := b.addresses["172.16.0.2"]
	warned := b.warnedAtCap
	b.mu.Unlock()

	if !admittedAfterSweep {
		t.Error("a new address was still refused after every record had lapsed; the cap is counting dead records")
	}
	if warned {
		t.Error("the saturation warning is still armed after the table recovered, so a second episode would be silent")
	}
}
