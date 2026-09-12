package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
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
	// Package is the import path the test ran in, as the stream spells it.
	Package string
}

// resultKey names one test of one package.
//
// The package is part of the key because the stream carries every package of
// the run and nothing stops two of them from declaring a test of the same
// name: keyed by name alone, the later verdict would overwrite the earlier
// and be applied to the other package's calls.
type resultKey struct {
	// pkg is the package as [packageName] spells it.
	pkg string
	// test is the full test name, subtests included.
	test string
}

// testResults is every test's final verdict, keyed by package and test.
type testResults map[resultKey]testResult

// packageName reduces a stream's import path to the name a run line carries.
//
// The harness names its package after the directory the test binary runs in,
// which go test makes the package directory, so the last element of the
// import path is the same name.
func packageName(importPath string) string {
	if importPath == "" {
		return ""
	}
	return path.Base(importPath)
}

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
func readResults(name string) (testResults, error) {
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.Open(filepath.Clean(name))
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
		key := resultKey{pkg: packageName(event.Package), test: event.Test}
		results[key] = testResult{Status: status, Elapsed: event.Elapsed, Package: event.Package}
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

// packages names every package the stream holds a verdict for.
func (results testResults) packages() map[string]bool {
	known := map[string]bool{}
	for key := range results {
		known[key.pkg] = true
	}
	return known
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
	Package string  `json:"package,omitempty"`
	Test    string  `json:"test"`
	Elapsed float64 `json:"elapsed_seconds"`
}

// joinResults settles every call's test status from the stream and reports
// what the join found.
//
// A call is matched in the package its shard names first, on its full test
// name and then on its top-level test: a subtest that reported no verdict of
// its own still ended with its parent. A call whose package is unknown, or
// whose package the stream never reported under that name, is matched by
// test name alone, and only when one package reports it.
func joinResults(rt *runtimeRecords, results testResults) resultsJoin {
	join := resultsJoin{Tests: len(results)}
	known := results.packages()
	called := map[resultKey]bool{}
	for _, call := range rt.calls {
		key, result, found := lookupResult(results, known, rt.packages[call], call.Test)
		// Every ancestor of the call's test made this call too, as far as a
		// verdict is concerned: a passed parent whose child called is not a
		// test that called nothing.
		for _, name := range ancestorTests(call.Test) {
			called[resultKey{pkg: key.pkg, test: name}] = true
		}
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
	for key, result := range results {
		if result.Status == e2ecalls.StatusPassed && !called[key] {
			join.TestsWithoutCalls = append(join.TestsWithoutCalls, idleTest{Package: key.pkg, Test: key.test, Elapsed: result.Elapsed})
		}
	}
	sort.Slice(join.TestsWithoutCalls, func(i, j int) bool {
		if join.TestsWithoutCalls[i].Package != join.TestsWithoutCalls[j].Package {
			return join.TestsWithoutCalls[i].Package < join.TestsWithoutCalls[j].Package
		}
		return join.TestsWithoutCalls[i].Test < join.TestsWithoutCalls[j].Test
	})
	return join
}

// lookupResult finds the verdict for a test of a package, falling back to its
// top-level test, and to a match by name alone when the package is unknown
// or the stream holds nothing under it.
//
// The key returned is the one the verdict was found under, so a caller can
// mark it; when nothing was found it names the package the call was placed
// in, which the stream does not hold.
func lookupResult(results testResults, known map[string]bool, pkg, test string) (resultKey, testResult, bool) {
	names := []string{test}
	if top := topLevelTest(test); top != test {
		names = append(names, top)
	}
	if known[pkg] {
		for _, name := range names {
			key := resultKey{pkg: pkg, test: name}
			if result, found := results[key]; found {
				return key, result, true
			}
		}
		return resultKey{pkg: pkg, test: test}, testResult{}, false
	}
	for _, name := range names {
		if key, result, found := results.byName(name); found {
			return key, result, true
		}
	}
	return resultKey{pkg: pkg, test: test}, testResult{}, false
}

// byName finds the one verdict a test name has across every package, and
// reports false when no package or more than one reports it: two answers
// are no answer, since either could be the wrong package's.
func (results testResults) byName(test string) (resultKey, testResult, bool) {
	var (
		matched resultKey
		verdict testResult
		matches int
	)
	for key, result := range results {
		if key.test == test {
			matched, verdict = key, result
			matches++
		}
	}
	return matched, verdict, matches == 1
}

// ancestorTests names a test and every test it sits under, from the full
// name up to the Test function.
func ancestorTests(name string) []string {
	names := []string{name}
	for {
		slash := strings.LastIndex(name, "/")
		if slash < 0 {
			return names
		}
		name = name[:slash]
		names = append(names, name)
	}
}

// topLevelTest returns the Test function a subtest name belongs to.
func topLevelTest(name string) string {
	top, _, _ := strings.Cut(name, "/")
	return top
}
