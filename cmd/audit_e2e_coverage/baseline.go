package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
)

// baselineResultsSuffix is appended to a baseline runtime's directory to
// name the gotestsum stream recorded beside it: the old suite's shards carry
// no verdicts of their own, and S09 wrote its results to
// dist/e2e-calls/baseline-<runtime>.results.json next to
// dist/e2e-calls/baseline-<runtime>/.
const baselineResultsSuffix = ".results.json"

// errBaselineUnjudged is a baseline whose calls carry no verdict and whose
// directory has no results stream beside it.
var errBaselineUnjudged = errors.New("the baseline carries no test verdicts and no results stream sits beside it")

// errBaselineStreamMismatch is a results stream that sits beside the baseline
// and judges none, or not all, of the tests its calls name: a stream from
// another run, which would leave the calls unjudged and the comparison
// vacuous.
var errBaselineStreamMismatch = errors.New("the results stream beside the baseline does not judge the tests its calls name")

// joinBaselineResults gives every baseline runtime its verdicts, so that the
// comparison has something to compare against.
//
// A credit counts only in a passing test, and the old suite's recorder wrote
// no verdict onto its calls: its shards were joined with the gotestsum
// stream when the baseline report was produced, and the same join has to
// happen here. Without it every baseline call classifies as failed, the
// baseline reaches nothing, and the superset check passes against nothing,
// which is exactly the silence it exists to refuse. A baseline whose calls
// already carry verdicts, the way the harness writes them, needs no stream
// and is left alone; one that carries none and has no stream is refused
// rather than compared vacuously. So is one whose stream judges none of the
// tests the calls name, or leaves some of them without a verdict: that is a
// stream from another run, and a stream that matches nothing would leave
// the reached set empty and the superset check passing against nothing, the
// same silence by another door. A call that names no test at all is a
// cleanup the old suite could not attribute, and stays unjudged on purpose.
func joinBaselineResults(runtimes []*runtimeRecords) error {
	for _, rt := range runtimes {
		if baselineJudged(rt) {
			continue
		}
		path := rt.dir + baselineResultsSuffix
		results, err := readResults(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s: %w (looked for %s)", rt.dir, errBaselineUnjudged, path)
			}
			return fmt.Errorf("%s: %w", rt.dir, err)
		}
		join := joinResults(rt, results)
		if unjudged := baselineUnjudgedCalls(rt); join.Filled == 0 || len(unjudged) > 0 {
			return fmt.Errorf("%s: %w: %s judged %d call(s) and left %d naming a test without a verdict (first: %s)",
				rt.dir, errBaselineStreamMismatch, path, join.Filled, len(unjudged), firstOrNone(unjudged))
		}
	}
	return nil
}

// baselineUnjudgedCalls lists the tests named by calls that carry no verdict
// after the join, each once.
func baselineUnjudgedCalls(rt *runtimeRecords) []string {
	seen := map[string]bool{}
	var names []string
	for _, call := range rt.calls {
		if call.Test == "" || call.TestStatus != "" || seen[call.Test] {
			continue
		}
		seen[call.Test] = true
		names = append(names, call.Test)
	}
	return names
}

// firstOrNone renders the first of a list, or "none" for an empty one.
func firstOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return names[0]
}

// baselineJudged reports whether any call of the runtime carries a verdict.
// One is enough to say the shards were written with verdicts: a recorder
// that writes them writes them on every call.
func baselineJudged(rt *runtimeRecords) bool {
	for _, call := range rt.calls {
		if call.TestStatus != "" {
			return true
		}
	}
	return false
}

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
		if !satisfied(now, key) {
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

// satisfied reports whether the run reached what one baseline key says the
// old suite reached.
//
// A credit is its own key, and most are not interchangeable: a refusal, an
// error path and a preview each show a behavior a successful call shows
// nothing about. The two that say only "the action ran and answered" are
// ordered, though. A cleanup credit is that answer made after the test
// body, a sweep credit that answer made with no assertion on it, and an
// asserted credit that answer with the test's assertion behind it, so each
// is met by itself or by the stronger ones. Without this, the old suite's
// habit of deleting in a cleanup what its body had already deleted, and
// counting GitLab's answer as an ok, would be a credit the port could only
// keep by repeating the habit.
func satisfied(now map[reachedKey]bool, key reachedKey) bool {
	if now[key] {
		return true
	}
	switch key.credit {
	case creditCleanup:
		return now[key.with(creditSweep)] || now[key.with(creditAsserted)]
	case creditSweep:
		return now[key.with(creditAsserted)]
	default:
		return false
	}
}

// with returns the key with another credit, for the lookup of a stronger one.
func (k reachedKey) with(credit credit) reachedKey {
	k.credit = credit
	return k
}
