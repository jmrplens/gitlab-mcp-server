package main

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
)

// authBlockCounters counts the requests each authentication budget refused,
// which is what telemetry publishes through [mcpotel.ObserveAuthBlocks].
//
// It is shared by the two guards rather than kept per guard, because they share
// the budgets themselves: a caller refused by the bearer guard and one refused
// by the gate behind it were refused by the same rule, and two series would
// have to be added back together to answer any question anybody asks.
//
// What is counted is the refusal, not the block. A block is raised once and
// then refuses every request that arrives while it lasts, so counting the
// raising would measure how often a sprayer starts and counting the refusals
// measures what the deployment is actually turning away. The second is the one
// an operator acts on.
//
// A nil *authBlockCounters counts nothing and is usable, which is what a guard
// built without telemetry wiring gets.
type authBlockCounters struct {
	failureLockout  atomic.Int64
	transportSource atomic.Int64
	distinctTokens  atomic.Int64
}

// record counts one refusal under the given reason, which is one of the
// mcpotel.AuthBlock* values. A reason this does not know is not counted, so a
// new budget that forgets to add its counter here is silent rather than
// attributed to the wrong one.
func (c *authBlockCounters) record(reason string) {
	if c == nil {
		return
	}
	switch reason {
	case mcpotel.AuthBlockFailureLockout:
		c.failureLockout.Add(1)
	case mcpotel.AuthBlockTransportSource:
		c.transportSource.Add(1)
	case mcpotel.AuthBlockDistinctTokens:
		c.distinctTokens.Add(1)
	}
}

// validateAuthBudgetBounds holds the four authentication-budget flags to the
// same bounds their environment spellings are held to.
//
// HTTP mode never runs (*config.Config).validate, so a bound enforced only
// there is a bound the flags escape, which is the class of defect
// [validateHTTPPoolAndRateBounds] exists to have fixed once. A negative value
// is refused rather than read as "off", because zero already says that and a
// minus sign is a typo.
func validateAuthBudgetBounds(cfg *config.Config) error {
	for _, b := range []struct {
		flag  string
		value int
		max   int
	}{
		{"--auth-failure-limit", cfg.AuthFailureLimit, config.MaxAuthFailureLimit},
		{"--auth-distinct-token-limit", cfg.AuthDistinctTokenLimit, config.MaxAuthDistinctTokenLimit},
	} {
		if b.value < 0 {
			return fmt.Errorf("%s must not be negative, got %d (0 disables the budget)", b.flag, b.value)
		}
		if b.value > b.max {
			return fmt.Errorf("%s %d exceeds maximum of %d", b.flag, b.value, b.max)
		}
	}
	for _, w := range []struct {
		flag  string
		value time.Duration
		max   time.Duration
	}{
		{"--auth-failure-window", cfg.AuthFailureWindow, config.MaxAuthFailureWindow},
		{"--auth-distinct-token-window", cfg.AuthDistinctWindow, config.MaxAuthDistinctWindow},
	} {
		if w.value < 0 {
			return fmt.Errorf("%s must not be negative, got %s (0 disables the budget)", w.flag, w.value)
		}
		if w.value > w.max {
			return fmt.Errorf("%s %s exceeds maximum of %s", w.flag, w.value, w.max)
		}
	}
	return nil
}

// authSprayBudget builds the distinct-token budget from the configuration, or
// returns nil when the deployment turned it off.
//
// The escalation step is the fast window rather than a setting of its own, so
// the ladder is one window, then ten, then sixty, and a deployment that
// shortens the window to watch the behaviour shortens the whole ladder with
// it. At the defaults that is a minute, ten minutes and an hour.
func authSprayBudget(cfg *config.Config) *serverpool.DistinctTokenBudget {
	return serverpool.NewDistinctTokenBudget(cfg.AuthDistinctTokenLimit, cfg.AuthDistinctWindow, cfg.AuthFailureWindow)
}

// observeAuthBlocks registers the refusal counters with OpenTelemetry.
//
// A failure is logged and not returned: telemetry that cannot be registered is
// a reason to say so, never a reason to refuse to serve. With telemetry off the
// global meter is a no-op and this costs one registration at startup.
func observeAuthBlocks(counts *authBlockCounters) {
	if _, err := mcpotel.ObserveAuthBlocks(counts.counts); err != nil {
		slog.Warn("authentication block metrics are not being exported", "error", err)
	}
}

// retryAfterSeconds renders a block's remaining time for the Retry-After
// header, which RFC 9110 defines in whole seconds.
//
// It rounds up and never answers zero: a block with 400 milliseconds left
// truncates to 0, and "come back in no time at all" invites the client to
// retry immediately into the same refusal. One second is the smallest honest
// answer.
func retryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	// Rounding up cannot produce less than one here: the smallest d this
	// reaches is a nanosecond, and a nanosecond rounded up is a second. A
	// floor check would be a second guard on that arithmetic, reachable by no
	// input.
	return int((d + time.Second - 1) / time.Second)
}

// counts reads the three counters, in the shape mcpotel publishes them.
func (c *authBlockCounters) counts() mcpotel.AuthBlockCounts {
	if c == nil {
		return mcpotel.AuthBlockCounts{}
	}
	return mcpotel.AuthBlockCounts{
		FailureLockout:  c.failureLockout.Load(),
		TransportSource: c.transportSource.Load(),
		DistinctTokens:  c.distinctTokens.Load(),
	}
}
