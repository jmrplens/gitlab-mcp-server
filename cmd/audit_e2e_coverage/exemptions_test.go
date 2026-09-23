package main

import (
	"testing"
)

// TestExemptions_AfterTheSwitch_RatchetIsOn pins the state the tables ship in
// now that the new suite is the gate: the ratchet on, so a catalog action with
// neither a scenario nor a declaration fails the push, and no floor, because a
// floor is read off a run's calls artifact by -check and is not a property of
// this table.
//
// It replaces the test that pinned the opposite, which is the point: switching
// the gate on is a change a reader sees in the tests rather than one buried in
// a constant.
func TestExemptions_AfterTheSwitch_RatchetIsOn(t *testing.T) {
	if !ratchetEnabled {
		t.Error("ratchetEnabled is off, so a catalog action with no scenario passes the gate")
	}
	if len(assertedFloors) != 0 {
		t.Errorf("assertedFloors holds %d entries, and floors belong to -check rather than to this table",
			len(assertedFloors))
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
