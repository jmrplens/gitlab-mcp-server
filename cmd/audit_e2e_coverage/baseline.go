package main

import (
	"fmt"
	"sort"
)

// reachedKey is one runtime x surface x mode x action x credit a passing test
// reached, which is the unit the superset check compares.
type reachedKey struct {
	surface string
	mode    string
	action  string
	credit  credit
}

// String spells the key for a loss list.
func (k reachedKey) String() string {
	return fmt.Sprintf("%s/%s %s %s", k.surface, k.mode, k.action, k.credit)
}

// baselineResult is the superset comparison of one runtime against the same
// runtime in the baseline directory.
type baselineResult struct {
	// BaselineDirectory is where the baseline runtime was read from.
	BaselineDirectory string `json:"baseline_directory"`
	// BaselineReached is how many keys the baseline reached.
	BaselineReached int `json:"baseline_reached"`
	// Reached is how many keys this run reached.
	Reached int `json:"reached"`
	// Lost lists the baseline keys this run did not reach, sorted.
	Lost []string `json:"lost"`
	// Gained is how many keys this run reached that the baseline did not.
	Gained int `json:"gained"`
}

// reachedSet collects every key a passing test reached.
//
// Only credit earned in a passing test counts, and only the credits that
// say something ran: an unobserved assertion is not proof the action ran, a
// skipped or failed cell is not something reached, and a cell reached both
// as asserted and as cleanup yields two keys because both are things the old
// suite could lose.
func reachedSet(c *classification) map[reachedKey]bool {
	reached := map[reachedKey]bool{}
	for key, found := range c.cells {
		for earned := range found.counts {
			if earned <= creditFailed || earned == creditUnobserved {
				continue
			}
			reached[reachedKey{surface: key.shape.surface, mode: key.shape.mode, action: key.action, credit: earned}] = true
		}
	}
	return reached
}

// compareBaseline reports what the baseline reached that this run did not.
func compareBaseline(current, baseline *classification, baselineDir string) *baselineResult {
	before := reachedSet(baseline)
	now := reachedSet(current)
	result := &baselineResult{BaselineDirectory: baselineDir, BaselineReached: len(before), Reached: len(now)}
	for key := range before {
		if !now[key] {
			result.Lost = append(result.Lost, key.String())
		}
	}
	for key := range now {
		if !before[key] {
			result.Gained++
		}
	}
	sort.Strings(result.Lost)
	return result
}
