package main

import "testing"

// TestExemptions_BeforeTheSwitch_ShipEmpty pins the state the tables ship
// in until the switch: the ratchet off, no exemption, no floor. The step
// that turns the ratchet on edits this test with it, which is the point:
// switching the gate on is a change a reader sees in the tests.
func TestExemptions_BeforeTheSwitch_ShipEmpty(t *testing.T) {
	if ratchetEnabled {
		t.Error("ratchetEnabled is on before the switch step")
	}
	if len(exemptedActions) != 0 {
		t.Errorf("exemptedActions holds %d entries before the switch step", len(exemptedActions))
	}
	if len(assertedFloors) != 0 {
		t.Errorf("assertedFloors holds %d entries before the switch step", len(assertedFloors))
	}
}

// TestExemptions_EveryEntry_WellFormed verifies that every exemption and every
// drop declaration names a known category and gives a reason, so a stale or
// mistyped entry is caught here as well as by the gate.
func TestExemptions_EveryEntry_WellFormed(t *testing.T) {
	for id, exemption := range exemptedActions {
		if !declaredCategories[exemption.Category] || exemption.Reason == "" {
			t.Errorf("exemption %s: category %q, reason %q", id, exemption.Category, exemption.Reason)
		}
	}
	for name, drop := range declaredDrops {
		if !declaredDropCategories[drop.Category] || drop.Reason == "" {
			t.Errorf("drop %s: category %q, reason %q", name, drop.Category, drop.Reason)
		}
	}
	for category := range declaredCategories {
		if category == "" {
			t.Error("an empty category is declared")
		}
	}
}
