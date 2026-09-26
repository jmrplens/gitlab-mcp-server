package tenancy

import (
	"strings"
	"time"
)

// BudgetOn reports whether the per-address failure budget is on: its limit and
// its window are both above zero (register row AUB-001).
//
// Zero means off (INV-015), and so does a negative figure. The server then
// builds no limiter at all, because the limiter blocks once a count reaches
// its limit, so one built with a limit of zero would block an address on its
// first failure: the harshest setting there is, reached by typing the figure
// every other budget reads as none. The limiter, its table and its sweep stay
// where the failures are counted; what lives here is which settings mean off,
// so that changing that is an edit to the register and to the oracle test
// beside it rather than to a condition in a layer.
func BudgetOn(limit int, window time.Duration) bool {
	return limit > 0 && window > 0
}

// EscalationOn reports whether the distinct-credential budget is on: its
// limit, its window and its escalation step are all above zero (register row
// AUB-003).
//
// The step is not a setting of its own. The server passes AUB-001's window, so
// that the ladder is one fast window, then ten, then sixty, and a window of
// zero, which switches AUB-001 off, switches this budget off with it. That
// coupling departs from INV-015's clause that switching one limit off does not
// switch off another, and is recorded as a finding (issue 958); it is stated
// here rather than changed, since this function decides exactly what the
// condition it replaced decided.
func EscalationOn(limit int, window, step time.Duration) bool {
	return BudgetOn(limit, window) && step > 0
}

// TransportSourceBudgetOn reports whether the transport-source budget exists:
// only when the deployment names a trusted-proxy header, one that is not blank
// once the whitespace around it is trimmed (register row AUB-002).
//
// A trusted header is the only configuration in which the per-address budget's
// key is chosen by the caller, so it is the only one in which this budget has
// anything to catch: it counts, per address a connection actually came from,
// the distinct keys that failed through it. Without the header the per-address
// budget is already keyed on that address, and a second limiter over the same
// string would only halve it. The budget's window falls back to the default
// when AUB-001's is zero, in three places the register declares rather than
// moves (issue 958).
func TransportSourceBudgetOn(trustedProxyHeader string) bool {
	return strings.TrimSpace(trustedProxyHeader) != ""
}
