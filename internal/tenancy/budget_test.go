package tenancy

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// The three oracles below are the conditions the register's budget switches
// replaced, as they stood at cb6379f53, before the register decided them. Each
// is the condition of an if that returned nil, so each answers whether the
// budget is off, and the promoted function answers the negation. The
// configuration fields each condition read are spelled as parameters. They are
// kept apart from the functions on purpose: a later edit of the register that
// changes what zero means fails the tests below until the copy is edited too,
// which is what makes such an edit a visible change of policy (issue 565).

// legacyFailureLimiterOff is the condition of authFailureLimiter in cmd/server
// (cmd/server/auth_blocks.go:106), with cfg.AuthFailureLimit and
// cfg.AuthFailureWindow spelled as limit and window.
func legacyFailureLimiterOff(limit int, window time.Duration) bool {
	return limit <= 0 || window <= 0
}

// legacyDistinctTokenBudgetOff is the condition of
// serverpool.NewDistinctTokenBudget (internal/serverpool/distinct_token_budget.go:111),
// whose parameters already carried these names.
func legacyDistinctTokenBudgetOff(limit int, window, step time.Duration) bool {
	return limit <= 0 || window <= 0 || step <= 0
}

// legacyTransportFailureBudgetOff is the condition of transportFailureBudget in
// cmd/server (cmd/server/main.go:3614), with cfg.TrustedProxyHeader spelled as
// header.
func legacyTransportFailureBudgetOff(header string) bool {
	return strings.TrimSpace(header) == ""
}

// budgetLimits are the limits the grids try: below zero, zero, the smallest
// limit that is on, and one past it.
func budgetLimits() []int {
	return []int{-1, 0, 1, 10}
}

// budgetDurations are the windows and steps the grids try: below zero, zero,
// the smallest duration that is on, and the two scales the defaults use.
func budgetDurations() []time.Duration {
	return []time.Duration{-time.Second, 0, time.Nanosecond, time.Second, time.Minute}
}

// trustedProxyHeaders are the header settings the grid tries: empty, blank in
// two ways, and two real headers, one written with whitespace around it.
func trustedProxyHeaders() []string {
	return []string{"", " ", "\t", "X-Forwarded-For", " CF-Connecting-IP "}
}

// budgetAgrees fails the test unless BudgetOn answers the negation of the
// replaced condition.
func budgetAgrees(t *testing.T, limit int, window time.Duration) {
	t.Helper()
	if got, off := BudgetOn(limit, window), legacyFailureLimiterOff(limit, window); got == off {
		t.Errorf("BudgetOn(%d, %s) = %v, and the replaced condition said off = %v", limit, window, got, off)
	}
}

// escalationAgrees fails the test unless EscalationOn answers the negation of
// the replaced condition.
func escalationAgrees(t *testing.T, limit int, window, step time.Duration) {
	t.Helper()
	if got, off := EscalationOn(limit, window, step), legacyDistinctTokenBudgetOff(limit, window, step); got == off {
		t.Errorf("EscalationOn(%d, %s, %s) = %v, and the replaced condition said off = %v", limit, window, step, got, off)
	}
}

// transportAgrees fails the test unless TransportSourceBudgetOn answers the
// negation of the replaced condition.
func transportAgrees(t *testing.T, header string) {
	t.Helper()
	if got, off := TransportSourceBudgetOn(header), legacyTransportFailureBudgetOff(header); got == off {
		t.Errorf("TransportSourceBudgetOn(%q) = %v, and the replaced condition said off = %v", header, got, off)
	}
}

// TestBudgetOn_AgreesWithTheReplacedCondition holds BudgetOn to the condition
// of authFailureLimiter over every limit of the grid crossed with every
// window: the per-address failure budget is on exactly when that condition
// did not switch it off.
func TestBudgetOn_AgreesWithTheReplacedCondition(t *testing.T) {
	for _, limit := range budgetLimits() {
		for _, window := range budgetDurations() {
			t.Run(fmt.Sprintf("limit=%d/window=%s", limit, window), func(t *testing.T) {
				budgetAgrees(t, limit, window)
			})
		}
	}
}

// TestEscalationOn_AgreesWithTheReplacedCondition holds EscalationOn to the
// condition of NewDistinctTokenBudget over every limit of the grid crossed
// with every window and every step.
func TestEscalationOn_AgreesWithTheReplacedCondition(t *testing.T) {
	for _, limit := range budgetLimits() {
		for _, window := range budgetDurations() {
			for _, step := range budgetDurations() {
				t.Run(fmt.Sprintf("limit=%d/window=%s/step=%s", limit, window, step), func(t *testing.T) {
					escalationAgrees(t, limit, window, step)
				})
			}
		}
	}
}

// TestTransportSourceBudgetOn_AgreesWithTheReplacedCondition holds
// TransportSourceBudgetOn to the condition of transportFailureBudget on every
// header of the grid.
func TestTransportSourceBudgetOn_AgreesWithTheReplacedCondition(t *testing.T) {
	for _, header := range trustedProxyHeaders() {
		t.Run(fmt.Sprintf("%q", header), func(t *testing.T) {
			transportAgrees(t, header)
		})
	}
}

// TestBudgetOn_ZeroInEitherSettingSwitchesTheBudgetOff states INV-015 for
// AUB-001 as cases rather than as agreement with a copy: a zero limit or a zero
// window is off, whatever the other setting says, and both above zero is on.
func TestBudgetOn_ZeroInEitherSettingSwitchesTheBudgetOff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		window time.Duration
		want   bool
	}{
		{"the defaults", 10, time.Minute, true},
		{"a zero limit", 0, time.Minute, false},
		{"a zero window", 10, 0, false},
		{"both zero", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := BudgetOn(tc.limit, tc.window); got != tc.want {
				t.Errorf("BudgetOn(%d, %s) = %v, want %v", tc.limit, tc.window, got, tc.want)
			}
		})
	}
}

// TestEscalationOn_AZeroStepSwitchesTheBudgetOff pins the coupling the
// register records rather than fixes (issue 958): the step is AUB-001's window,
// so a zero there switches AUB-003 off even with its own limit and window set.
func TestEscalationOn_AZeroStepSwitchesTheBudgetOff(t *testing.T) {
	if !EscalationOn(50, 10*time.Minute, time.Minute) {
		t.Error("EscalationOn at the defaults is off, want on")
	}
	if EscalationOn(50, 10*time.Minute, 0) {
		t.Error("EscalationOn with a zero step is on, want off: the step is AUB-001's window")
	}
}

// TestBudgetSwitches_AllocateNothing pins that the three switches allocate
// nothing. They run once at startup, so this is about the promoted form
// costing what the conditions it replaced cost, not about a hot path.
func TestBudgetSwitches_AllocateNothing(t *testing.T) {
	header := " X-Forwarded-For "
	allocs := testing.AllocsPerRun(1000, func() {
		_ = BudgetOn(10, time.Minute)
		_ = EscalationOn(50, 10*time.Minute, time.Minute)
		_ = TransportSourceBudgetOn(header)
	})
	if allocs != 0 {
		t.Errorf("the budget switches allocate %v times per call, want 0", allocs)
	}
}

// TestBudgetSwitches_AreTheRulesTheirRowsName ties each switch to the row that
// names it: AUB-001 names BudgetOn and AUB-003 names EscalationOn, both
// meaning off by zero, AUB-003 switched off with AUB-001, and AUB-002, whose
// value is a constant with no zero to mean anything, names
// TransportSourceBudgetOn.
func TestBudgetSwitches_AreTheRulesTheirRowsName(t *testing.T) {
	for _, tc := range []struct {
		row      string
		function string
		zero     Zero
		offWith  string
	}{
		{"AUB-001", "BudgetOn", ZeroOff, ""},
		{"AUB-002", "TransportSourceBudgetOn", ZeroNotApplicable, ""},
		{"AUB-003", "EscalationOn", ZeroOff, "AUB-001"},
	} {
		t.Run(tc.row, func(t *testing.T) {
			d, ok := Lookup(tc.row)
			if !ok {
				t.Fatalf("no row %s", tc.row)
			}
			if !has(d.Functions, tc.function) || d.Zero != tc.zero || d.OffWith != tc.offWith {
				t.Errorf("%s = %+v, want a row naming %s with zero meaning %v, off with %q",
					tc.row, d, tc.function, tc.zero, tc.offWith)
			}
		})
	}
}

// budgetFuzzLimits are the grid's limits and the extremes of the type, which
// no parser admits but which a comparison with zero still has to answer the
// way the replaced condition did.
func budgetFuzzLimits() []int {
	return append(budgetLimits(), math.MinInt, math.MaxInt)
}

// budgetFuzzDurations are the grid's durations and the extremes of the type,
// as the int64 the fuzzing engine supplies.
func budgetFuzzDurations() []int64 {
	var out []int64
	for _, d := range budgetDurations() {
		out = append(out, int64(d))
	}
	return append(out, math.MinInt64, math.MaxInt64)
}

// headerNearMisses are the header settings the grid leaves out that trimming
// could be thought to disagree about: the other whitespace strings.TrimSpace
// removes (a newline, a vertical tab, U+0085, U+00A0 and U+3000), what it
// keeps (U+200B, which is not a space to package unicode), a NUL and invalid
// UTF-8. The non-ASCII ones are spelled as their UTF-8 bytes.
func headerNearMisses() []string {
	return []string{
		"\n", "\v", "\xc2\x85", "\xc2\xa0", "\xe3\x80\x80", "\xe2\x80\x8b",
		"\x00", "\xff", " \xff ",
	}
}

// FuzzBudgetOn holds BudgetOn to the replaced condition on any limit and
// window.
func FuzzBudgetOn(f *testing.F) {
	for _, limit := range budgetFuzzLimits() {
		for _, window := range budgetFuzzDurations() {
			f.Add(limit, window)
		}
	}
	f.Fuzz(func(t *testing.T, limit int, window int64) {
		budgetAgrees(t, limit, time.Duration(window))
	})
}

// FuzzEscalationOn holds EscalationOn to the replaced condition on any limit,
// window and step.
func FuzzEscalationOn(f *testing.F) {
	for _, limit := range budgetFuzzLimits() {
		for _, window := range budgetFuzzDurations() {
			for _, step := range budgetFuzzDurations() {
				f.Add(limit, window, step)
			}
		}
	}
	f.Fuzz(func(t *testing.T, limit int, window, step int64) {
		escalationAgrees(t, limit, time.Duration(window), time.Duration(step))
	})
}

// FuzzTransportSourceBudgetOn holds TransportSourceBudgetOn to the replaced
// condition on any header setting, seeded with the grid's headers and the near
// misses.
func FuzzTransportSourceBudgetOn(f *testing.F) {
	for _, header := range append(trustedProxyHeaders(), headerNearMisses()...) {
		f.Add(header)
	}
	f.Fuzz(transportAgrees)
}
