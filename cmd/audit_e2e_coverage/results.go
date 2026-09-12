package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// errNoVerdict is a results stream holding no pass, fail or skip at all,
// which is a file that is not the one -results was meant to name.
var errNoVerdict = errors.New("the results stream holds no test verdict")

// testEvent is one line of the go test -json stream, which is what gotestsum
// writes to its --jsonfile. Only the fields the join reads are named.
type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
}

// testResult is the final verdict of one test in the stream.
type testResult struct {
	// Status is the record's own vocabulary for the verdict.
	Status string
	// Elapsed is the test's own duration in seconds, which is how a test that
	// passed without doing anything gives itself away.
	Elapsed float64
	// Package is the package the test ran in.
	Package string
}

// testResults is every test's final verdict, keyed by the full test name.
type testResults map[string]testResult

// maxEventLine bounds one event line. An output event carries one line of
// test output, which is short; the cap exists so that a stray multi-megabyte
// line is refused rather than assembled.
const maxEventLine = 1 << 20

// readResults parses a go test -json stream into final verdicts.
//
// The last run, pass, fail or skip event for a test wins, which is also the
// only one that exists for a test that ran once. Lines that are not JSON are
// refused rather than skipped: gotestsum writes nothing else to that file, so
// one means the file is not what -results was told it was.
func readResults(path string) (testResults, error) {
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("open results: %w", err)
	}
	defer func() { _ = file.Close() }()
	return parseResults(file)
}

// parseResults is [readResults] over any reader.
func parseResults(reader io.Reader) (testResults, error) {
	results := testResults{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxEventLine)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var event testEvent
		if err := json.Unmarshal([]byte(text), &event); err != nil {
			return nil, fmt.Errorf("results line %d: %w", line, err)
		}
		if event.Test == "" {
			continue
		}
		status, verdict := verdictStatus(event.Action)
		if !verdict {
			continue
		}
		results[event.Test] = testResult{Status: status, Elapsed: event.Elapsed, Package: event.Package}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read results: %w", err)
	}
	if len(results) == 0 {
		return nil, errNoVerdict
	}
	return results, nil
}

// verdictStatus maps a go test action to the record's status vocabulary, and
// reports false for the actions that are not verdicts.
func verdictStatus(action string) (string, bool) {
	switch action {
	case "pass":
		return e2ecalls.StatusPassed, true
	case "fail":
		return e2ecalls.StatusFailed, true
	case "skip":
		return e2ecalls.StatusSkipped, true
	default:
		return "", false
	}
}

// resultsJoin is what joining the results stream to the calls found.
type resultsJoin struct {
	// Tests is how many tests the stream holds a verdict for.
	Tests int `json:"tests"`
	// Filled is how many calls had no status of their own and took the
	// stream's.
	Filled int `json:"filled"`
	// Overridden is how many calls carried a status the stream contradicted.
	// The stream wins, since it is the test binary's own verdict, and the
	// count is published because a nonzero one means the recorder's idea of
	// when a test ended differs from the binary's.
	Overridden int `json:"overridden"`
	// Unmatched is how many calls named a test the stream never reported.
	Unmatched int `json:"unmatched"`
	// TestsWithoutCalls names the tests that passed while recording no call
	// at all, with their duration. A test that passes in no time and calls
	// nothing is the shape the baseline calibration looks for.
	TestsWithoutCalls []idleTest `json:"tests_without_calls,omitempty"`
}

// idleTest is a test that passed and made no recorded call.
type idleTest struct {
	Test    string  `json:"test"`
	Elapsed float64 `json:"elapsed_seconds"`
}

// joinResults settles every call's test status from the stream and reports
// what the join found.
//
// A call is matched on its full test name first, then on its top-level test:
// a subtest that reported no verdict of its own still ended with its parent.
func joinResults(rt *runtimeRecords, results testResults) resultsJoin {
	join := resultsJoin{Tests: len(results)}
	called := map[string]bool{}
	for _, call := range rt.calls {
		called[call.Test] = true
		called[topLevelTest(call.Test)] = true
		result, found := lookupResult(results, call.Test)
		if !found {
			join.Unmatched++
			continue
		}
		switch {
		case call.TestStatus == "":
			join.Filled++
		case call.TestStatus != result.Status:
			join.Overridden++
		default:
			continue
		}
		call.TestStatus = result.Status
	}
	for name, result := range results {
		if result.Status == e2ecalls.StatusPassed && !called[name] {
			join.TestsWithoutCalls = append(join.TestsWithoutCalls, idleTest{Test: name, Elapsed: result.Elapsed})
		}
	}
	sort.Slice(join.TestsWithoutCalls, func(i, j int) bool {
		return join.TestsWithoutCalls[i].Test < join.TestsWithoutCalls[j].Test
	})
	return join
}

// lookupResult finds the verdict for a test, falling back to its top-level
// test.
func lookupResult(results testResults, test string) (testResult, bool) {
	if result, found := results[test]; found {
		return result, true
	}
	result, found := results[topLevelTest(test)]
	return result, found
}

// topLevelTest returns the Test function a subtest name belongs to.
func topLevelTest(name string) string {
	top, _, _ := strings.Cut(name, "/")
	return top
}
