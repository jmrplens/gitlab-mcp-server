package main

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// checkResult is the -check verdict for one runtime.
type checkResult struct {
	// Passed is whether every check held.
	Passed bool `json:"passed"`
	// Findings says what did not, one line each.
	Findings []string `json:"findings,omitempty"`
}

// checkRuntime applies the floors to one runtime.
//
// Three of them answer the critic's release gate that passes with zero tests:
// a runtime whose packages recorded no test call covered nothing, and a
// package that refused to run covered nothing either, whatever the others
// did. The fourth is the ratchet on the asserted count, read from
// [assertedFloors] under every selector the runtime matches.
func checkRuntime(rep *report, selectors []string) *checkResult {
	result := &checkResult{Passed: true}
	if rep.Summary.TestCalls == 0 {
		result.failf("no test call was recorded on %s", rep.Runtime)
	}
	for _, run := range rep.Runs {
		if run.Status == e2ecalls.RunRefused {
			result.failf("package %s refused to run: %s", run.Package, run.Reason)
		}
	}
	for _, selector := range floorSelectors(rep.Runtime, selectors) {
		floor := assertedFloors[selector]
		if rep.Summary.L1 < floor {
			result.failf("%d actions asserted on %s, below the floor of %d recorded for %s",
				rep.Summary.L1, rep.Runtime, floor, selector)
		}
	}
	return result
}

// floorSelectors names the selectors a runtime's floor is read under: every
// -runtime entry the runtime matches, and its own key.
func floorSelectors(key string, selectors []string) []string {
	matched := []string{key}
	for _, selector := range selectors {
		if selector != key && matchesRuntime(key, selector) {
			matched = append(matched, selector)
		}
	}
	return matched
}

// failf records one finding.
func (r *checkResult) failf(format string, args ...any) {
	r.Passed = false
	r.Findings = append(r.Findings, fmt.Sprintf(format, args...))
}

// missingRuntimes names the -runtime selectors no runtime under -calls
// matched, which is the check that an expected runtime left no run line.
func missingRuntimes(runtimes []*runtimeRecords, selectors []string) []string {
	var missing []string
	for _, selector := range selectors {
		found := false
		for _, rt := range runtimes {
			if matchesRuntime(rt.key, selector) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, selector)
		}
	}
	return missing
}
