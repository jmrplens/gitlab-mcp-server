package main

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
)

// TestAuthBlockCounters_Record_CountsEachReasonApart checks that the three
// budgets are counted separately, which is the whole point of the reason
// dimension: an operator reading one series must be able to tell which rule is
// doing the refusing.
func TestAuthBlockCounters_Record_CountsEachReasonApart(t *testing.T) {
	t.Parallel()
	var c authBlockCounters

	c.record(mcpotel.AuthBlockFailureLockout)
	c.record(mcpotel.AuthBlockFailureLockout)
	c.record(mcpotel.AuthBlockTransportSource)
	c.record(mcpotel.AuthBlockDistinctTokens)
	c.record(mcpotel.AuthBlockDistinctTokens)
	c.record(mcpotel.AuthBlockDistinctTokens)

	got := c.counts()
	want := mcpotel.AuthBlockCounts{FailureLockout: 2, TransportSource: 1, DistinctTokens: 3}
	if got != want {
		t.Errorf("counts() = %+v, want %+v", got, want)
	}
}

// TestAuthBlockCounters_Record_UnknownReasonIsNotCounted pins the direction the
// switch must fail in. A budget added later that forgets its counter here
// should be silent rather than attributed to whichever reason happens to be
// first, because a wrong attribution is worse than a missing one: it reads as
// evidence about a rule that did nothing.
func TestAuthBlockCounters_Record_UnknownReasonIsNotCounted(t *testing.T) {
	t.Parallel()
	var c authBlockCounters

	c.record("a-budget-nobody-wired-up")
	c.record("")

	if got := c.counts(); got != (mcpotel.AuthBlockCounts{}) {
		t.Errorf("counts() = %+v, want every counter at zero", got)
	}
}

// TestAuthBlockCounters_Nil_IsUsable checks the zero-wiring case: a guard built
// without counters must not panic on the request path.
func TestAuthBlockCounters_Nil_IsUsable(t *testing.T) {
	t.Parallel()
	var c *authBlockCounters

	c.record(mcpotel.AuthBlockDistinctTokens)
	if got := c.counts(); got != (mcpotel.AuthBlockCounts{}) {
		t.Errorf("counts() on a nil receiver = %+v, want the zero value", got)
	}
}

// TestRetryAfterSeconds_RoundsUpAndNeverAnswersZero covers the arithmetic a
// client acts on. Truncating would answer "come back in no time at all" for
// any block under a second, which invites an immediate retry into the same
// refusal.
func TestRetryAfterSeconds_RoundsUpAndNeverAnswersZero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   time.Duration
		want int
	}{
		{name: "negative", in: -time.Hour, want: 1},
		{name: "zero", in: 0, want: 1},
		{name: "a nanosecond", in: time.Nanosecond, want: 1},
		{name: "just under a second", in: 999 * time.Millisecond, want: 1},
		{name: "exactly a second", in: time.Second, want: 1},
		{name: "just over a second", in: time.Second + time.Nanosecond, want: 2},
		{name: "a minute", in: time.Minute, want: 60},
		{name: "ten minutes", in: 10 * time.Minute, want: 600},
		{name: "an hour", in: time.Hour, want: 3600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := retryAfterSeconds(tc.in); got != tc.want {
				t.Errorf("retryAfterSeconds(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestAuthFailureLimiter_ZeroSettings_BuildNoLimiter is the regression for the
// trap that making the limit configurable opened.
//
// AuthRateLimiter blocks once a record reaches its limit, so a limit of zero
// blocks an address after a single failure: the harshest setting there is,
// reached by typing the figure every other budget here reads as "none". Nil is
// what off has to be, and every consulting site tolerates it.
func TestAuthFailureLimiter_ZeroSettings_BuildNoLimiter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{name: "zero limit", limit: 0, window: time.Minute},
		{name: "negative limit", limit: -1, window: time.Minute},
		{name: "zero window", limit: 10, window: 0},
		{name: "negative window", limit: 10, window: -time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{AuthFailureLimit: tc.limit, AuthFailureWindow: tc.window}
			if got := authFailureLimiter(cfg); got != nil {
				t.Fatalf("authFailureLimiter(%d, %v) built a limiter; zero must mean no budget, not a budget of zero",
					tc.limit, tc.window)
			}
		})
	}
}

// TestAuthFailureLimiter_RealSettings_BuildOne is the other half: a configured
// budget is built, and it blocks where it was told to rather than earlier.
func TestAuthFailureLimiter_RealSettings_BuildOne(t *testing.T) {
	t.Parallel()
	limiter := authFailureLimiter(&config.Config{AuthFailureLimit: 3, AuthFailureWindow: time.Minute})
	if limiter == nil {
		t.Fatal("authFailureLimiter(3, 1m) = nil, want a limiter")
	}

	limiter.RecordFailure("10.0.0.1")
	limiter.RecordFailure("10.0.0.1")
	if limiter.IsBlocked("10.0.0.1") {
		t.Error("blocked after two failures at a limit of three")
	}
	limiter.RecordFailure("10.0.0.1")
	if !limiter.IsBlocked("10.0.0.1") {
		t.Error("not blocked after three failures at a limit of three")
	}
}

// TestAuthSprayBudget_ZeroSettings_BuildNothing checks that the distinct-token
// budget is off under the same spellings, and that the escalation step it is
// given is the failure window.
func TestAuthSprayBudget_ZeroSettings_BuildNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  config.Config
	}{
		{name: "zero limit", cfg: config.Config{AuthDistinctTokenLimit: 0, AuthDistinctWindow: time.Minute, AuthFailureWindow: time.Minute}},
		{name: "zero distinct window", cfg: config.Config{AuthDistinctTokenLimit: 5, AuthDistinctWindow: 0, AuthFailureWindow: time.Minute}},
		{name: "zero step", cfg: config.Config{AuthDistinctTokenLimit: 5, AuthDistinctWindow: time.Minute, AuthFailureWindow: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := authSprayBudget(&tc.cfg); got != nil {
				t.Errorf("authSprayBudget(%+v) = %v, want nil", tc.cfg, got)
			}
		})
	}
}

// TestAuthSprayBudget_LadderIsBuiltFromTheFailureWindow pins the derivation
// that the whole design rests on: the first block is one failure window, so a
// deployment that shortens the window to observe the behavior shortens the
// ladder with it rather than waiting an hour for the third rung.
func TestAuthSprayBudget_LadderIsBuiltFromTheFailureWindow(t *testing.T) {
	t.Parallel()
	budget := authSprayBudget(&config.Config{
		AuthDistinctTokenLimit: 2,
		AuthDistinctWindow:     time.Minute,
		AuthFailureWindow:      2 * time.Second,
	})
	if budget == nil {
		t.Fatal("authSprayBudget with real settings = nil, want a budget")
	}

	budget.Charge("10.0.0.1", "glpat-a")
	budget.Charge("10.0.0.1", "glpat-b")

	blocked, remaining := budget.Blocked("10.0.0.1")
	if !blocked {
		t.Fatal("two distinct credentials at a limit of two did not block")
	}
	if remaining > 2*time.Second || remaining <= 0 {
		t.Errorf("first block = %v, want at most the 2s failure window", remaining)
	}
}

// TestValidateAuthBudgetBounds_RefusesWhatIsOutOfRange covers both directions
// of each of the four settings.
//
// A negative value is refused rather than read as "off", because zero already
// says that and a minus sign is a typo. The maxima matter for the same reason
// the pool's does: HTTP mode never runs Config.validate, so a bound enforced
// only there is a bound the flags escape.
func TestValidateAuthBudgetBounds_RefusesWhatIsOutOfRange(t *testing.T) {
	t.Parallel()
	valid := config.Config{
		AuthFailureLimit:       config.DefaultAuthFailureLimit,
		AuthFailureWindow:      config.DefaultAuthFailureWindow,
		AuthDistinctTokenLimit: config.DefaultAuthDistinctTokenLimit,
		AuthDistinctWindow:     config.DefaultAuthDistinctWindow,
	}

	t.Run("the defaults are accepted", func(t *testing.T) {
		t.Parallel()
		if err := validateAuthBudgetBounds(&valid); err != nil {
			t.Errorf("validateAuthBudgetBounds(defaults) = %v, want nil", err)
		}
	})

	t.Run("zero is accepted as off", func(t *testing.T) {
		t.Parallel()
		off := valid
		off.AuthFailureLimit = 0
		off.AuthFailureWindow = 0
		off.AuthDistinctTokenLimit = 0
		off.AuthDistinctWindow = 0
		if err := validateAuthBudgetBounds(&off); err != nil {
			t.Errorf("validateAuthBudgetBounds(all zero) = %v, want nil: zero turns a budget off", err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{name: "negative failure limit", mutate: func(c *config.Config) { c.AuthFailureLimit = -1 }},
		{name: "failure limit over the maximum", mutate: func(c *config.Config) { c.AuthFailureLimit = config.MaxAuthFailureLimit + 1 }},
		{name: "negative distinct limit", mutate: func(c *config.Config) { c.AuthDistinctTokenLimit = -1 }},
		{name: "distinct limit over the maximum", mutate: func(c *config.Config) { c.AuthDistinctTokenLimit = config.MaxAuthDistinctTokenLimit + 1 }},
		{name: "negative failure window", mutate: func(c *config.Config) { c.AuthFailureWindow = -time.Second }},
		{name: "failure window over the maximum", mutate: func(c *config.Config) { c.AuthFailureWindow = config.MaxAuthFailureWindow + time.Second }},
		{name: "negative distinct window", mutate: func(c *config.Config) { c.AuthDistinctWindow = -time.Second }},
		{name: "distinct window over the maximum", mutate: func(c *config.Config) { c.AuthDistinctWindow = config.MaxAuthDistinctWindow + time.Second }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tc.mutate(&cfg)
			if err := validateAuthBudgetBounds(&cfg); err == nil {
				t.Errorf("validateAuthBudgetBounds(%s) = nil, want a refusal", tc.name)
			}
		})
	}
}

// TestObserveAuthBlocks_RegistersWithoutTelemetry checks that the registration
// is safe to make unconditionally, which is how both handlers call it: with
// telemetry off the global meter is a no-op and nothing should fail or panic.
func TestObserveAuthBlocks_RegistersWithoutTelemetry(t *testing.T) {
	var c authBlockCounters
	observeAuthBlocks(&c)
}
