// tier_test.go contains unit tests for the edition tier type, its ordering
// helpers, and the parsing/mapping functions used by configuration loading and
// instance detection.
package edition

import "testing"

// TestParseTier verifies that ParseTier accepts the documented aliases
// case-insensitively and reports unrecognized values via ok=false.
func TestParseTier(t *testing.T) {
	tests := []struct {
		in       string
		wantTier Tier
		wantOK   bool
	}{
		{in: "free", wantTier: Free, wantOK: true},
		{in: "ce", wantTier: Free, wantOK: true},
		{in: "CE", wantTier: Free, wantOK: true},
		{in: "premium", wantTier: Premium, wantOK: true},
		{in: "  Premium ", wantTier: Premium, wantOK: true},
		{in: "ultimate", wantTier: Ultimate, wantOK: true},
		{in: "ULTIMATE", wantTier: Ultimate, wantOK: true},
		{in: "", wantTier: Free, wantOK: false},
		{in: "platinum", wantTier: Free, wantOK: false},
		{in: "starter", wantTier: Free, wantOK: false}, // legacy plan names are not config inputs
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			gotTier, gotOK := ParseTier(tc.in)
			if gotTier != tc.wantTier || gotOK != tc.wantOK {
				t.Errorf("ParseTier(%q) = (%v, %v), want (%v, %v)", tc.in, gotTier, gotOK, tc.wantTier, tc.wantOK)
			}
		})
	}
}

// TestTierFromPlan verifies the GitLab license plan to tier mapping, including
// legacy plan names (starter/bronze/silver → Premium, gold → Ultimate).
func TestTierFromPlan(t *testing.T) {
	tests := []struct {
		plan string
		want Tier
	}{
		{plan: "premium", want: Premium},
		{plan: "ultimate", want: Ultimate},
		{plan: "Ultimate", want: Ultimate},
		{plan: "starter", want: Premium},
		{plan: "bronze", want: Premium},
		{plan: "silver", want: Premium},
		{plan: "gold", want: Ultimate},
		{plan: "free", want: Free},
		{plan: "", want: Free},
		{plan: "unknown", want: Free},
	}
	for _, tc := range tests {
		t.Run(tc.plan, func(t *testing.T) {
			if got := TierFromPlan(tc.plan); got != tc.want {
				t.Errorf("TierFromPlan(%q) = %v, want %v", tc.plan, got, tc.want)
			}
		})
	}
}

// TestTierFromEdition verifies the per-action Edition metadata string maps to
// the correct minimum tier, including the legacy "core" marker and the Free
// default for empty/unknown values.
func TestTierFromEdition(t *testing.T) {
	tests := []struct {
		edition string
		want    Tier
	}{
		{edition: "premium", want: Premium},
		{edition: "Premium", want: Premium},
		{edition: " ultimate ", want: Ultimate},
		{edition: "core", want: Free},
		{edition: "free", want: Free},
		{edition: "ce", want: Free},
		{edition: "", want: Free},
		{edition: "unknown", want: Free},
	}
	for _, tc := range tests {
		t.Run(tc.edition, func(t *testing.T) {
			if got := TierFromEdition(tc.edition); got != tc.want {
				t.Errorf("TierFromEdition(%q) = %v, want %v", tc.edition, got, tc.want)
			}
		})
	}
}

// TestTierOrderingHelpers verifies AtLeast, IsEnterprise, and String for each
// tier value.
func TestTierOrderingHelpers(t *testing.T) {
	if !Ultimate.AtLeast(Premium) || !Premium.AtLeast(Free) || !Free.AtLeast(Free) {
		t.Error("AtLeast ordering Free < Premium < Ultimate not satisfied")
	}
	if Free.AtLeast(Premium) || Premium.AtLeast(Ultimate) {
		t.Error("AtLeast should reject higher required tiers")
	}
	if Free.IsEnterprise() {
		t.Error("Free should not be enterprise")
	}
	if !Premium.IsEnterprise() || !Ultimate.IsEnterprise() {
		t.Error("Premium and Ultimate should be enterprise")
	}
	if Free.String() != "free" || Premium.String() != "premium" || Ultimate.String() != "ultimate" {
		t.Errorf("String() mismatch: %q %q %q", Free, Premium, Ultimate)
	}
}

// TestTierString_FreeAndUnknown verifies String() maps Free to "free" and
// coerces unknown tier values to "free" as the safe default branch.
func TestTierString_FreeAndUnknown(t *testing.T) {
	if got := Free.String(); got != "free" {
		t.Errorf("Free.String() = %q, want free", got)
	}
	if got := Tier(99).String(); got != "free" {
		t.Errorf("Tier(99).String() = %q, want free (default branch)", got)
	}
}
