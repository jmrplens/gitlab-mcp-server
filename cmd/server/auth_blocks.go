package main

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
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
	failureLockout   atomic.Int64
	transportSource  atomic.Int64
	distinctTokens   atomic.Int64
	networkAggregate atomic.Int64
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
	case mcpotel.AuthBlockNetworkAggregate:
		c.networkAggregate.Add(1)
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

// authFailureLimiter builds the fast per-address budget, or returns nil when
// the deployment turned it off.
//
// The nil is what "off" has to be. [serverpool.AuthRateLimiter] blocks once a
// record's count reaches its limit, so a limit of zero blocks an address after
// a single failure: the most aggressive setting there is, reached by typing the
// figure that every other budget here reads as "no budget". Every consulting
// site already tolerates a nil limiter, so refusing to build one is both the
// smallest change and the only one that cannot be read two ways.
func authFailureLimiter(cfg *config.Config) *serverpool.AuthRateLimiter {
	if !tenancy.BudgetOn(cfg.AuthFailureLimit, cfg.AuthFailureWindow) { // register row AUB-001
		return nil
	}
	return serverpool.NewAuthRateLimiter(cfg.AuthFailureLimit, cfg.AuthFailureWindow)
}

// authSprayBudget builds the distinct-token budget from the configuration, or
// returns nil when the deployment turned it off.
//
// The escalation step is the fast window rather than a setting of its own, so
// the ladder is one window, then ten, then sixty, and a deployment that
// shortens the window to watch the behavior shortens the whole ladder with
// it. At the defaults that is a minute, ten minutes and an hour.
func authSprayBudget(cfg *config.Config) *serverpool.DistinctTokenBudget {
	return serverpool.NewDistinctTokenBudget(cfg.AuthDistinctTokenLimit, cfg.AuthDistinctWindow, cfg.AuthFailureWindow)
}

// networkFailingPrefixes is how many distinct IPv6 /64s of one /48 may have a
// credential refused inside the distinct-credential window before the /48 is
// blocked.
//
// The aggregate exists because the address budgets are keyed on the /64
// ([budgetKey]), and a caller holding a /48 holds 65536 of them, each with a
// fast budget and a distinct-credential budget of its own: keying on the /64
// stops rotation inside a /64 and buys nothing against rotation across them.
// Ten is issue 954's figure, and it is reached by more than a spray. A
// rotating spray reaches it on its tenth /64; so does an organization whose
// network is one /48 when callers on ten of its subnets have a credential
// refused inside the window, an expired or revoked token as much as an
// invented one, and the /48 is then refused for every credential the
// deployment does not already hold. Nor is a /48 always one organization's:
// a provider that delegates each subscriber a /56 or a /60 puts 256 or 4096
// of them in one, and a mobile network gives each device a /64 of its own
// (RFC 6459), so ten unrelated subscribers presenting an expired token inside
// the window block the /48 for every other one whose credential is not
// already held. A /64 whose clients only forgot their header never counts.
// The figure is the register's and no operator's, which the issue decided.
const networkFailingPrefixes = tenancy.AuthNetworkFailingPrefixes // register row AUB-006

// authNetworkBudget builds the /48 aggregate (AUB-006).
//
// It is the distinct-token budget's machinery counting /64s instead of
// credentials: the /48 is the key, the /64 is the distinct value, held as a
// digest like a refused credential is, and the block climbs the same ladder
// in the same window. The figure is a register constant, so it is built for
// every deployment; the windows it reads are the operator's, and a window of
// zero, which switches the budget that owns it off, gives this one the default
// rather than switching it off too (INV-015), as [transportFailureBudget]
// reads the fast window.
func authNetworkBudget(cfg *config.Config) *serverpool.DistinctTokenBudget {
	window := cfg.AuthDistinctWindow
	if window <= 0 {
		window = config.DefaultAuthDistinctWindow
	}
	step := cfg.AuthFailureWindow
	if step <= 0 {
		step = config.DefaultAuthFailureWindow
	}
	return serverpool.NewDistinctTokenBudget(networkFailingPrefixes, window, step)
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

// counts reads the four counters, in the shape mcpotel publishes them.
func (c *authBlockCounters) counts() mcpotel.AuthBlockCounts {
	if c == nil {
		return mcpotel.AuthBlockCounts{}
	}
	return mcpotel.AuthBlockCounts{
		FailureLockout:   c.failureLockout.Load(),
		TransportSource:  c.transportSource.Load(),
		DistinctTokens:   c.distinctTokens.Load(),
		NetworkAggregate: c.networkAggregate.Load(),
	}
}
