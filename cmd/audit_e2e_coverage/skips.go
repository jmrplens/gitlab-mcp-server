package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The skip gate, -check-skips: every test a complete Docker run ended with a
// skip is held to [declaredSkips], and so is every declaration.
//
// It exists because nothing read the skips. The harness records each one with
// its reason and the coverage record counts them, but a run is green whatever
// they add up to, so a run of the CE image that skipped eleven scenarios read
// as complete (issue 1014). A skip is the one outcome that says nothing about
// the server, which makes it the one a green run can hide the most behind.
//
// It reads the go test -json stream rather than the shards the harness
// writes, because the stream holds every skip whatever called it, t.Skip on
// the testing.T included, while the harness records only the skips it was
// told about.

// skipEvent is one line of the go test -json stream as the skip gate reads
// it: the verdict [testEvent] reads, and the output a test printed, which is
// the only place its skip reason is written.
type skipEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

// recordedSkip is one test a run ended with a skip, and why.
type recordedSkip struct {
	// Package is the package as [packageName] spells it.
	Package string
	// Test is the full test name, subtests included.
	Test string
	// Reason is what the test printed when it skipped, empty when it printed
	// nothing.
	Reason string
}

// reasonText is the reason as a report prints it, which says so when there
// is none rather than printing nothing after the colon.
func (s recordedSkip) reasonText() string {
	if s.Reason == "" {
		return "(no reason printed)"
	}
	return s.Reason
}

// testRun is what the stream holds for the last run of one test: the lines it
// printed and the verdict it ended with, empty until it ends.
type testRun struct {
	output []string
	status string
}

// skipFrame opens the line go test writes when a test ends skipped.
const skipFrame = "--- SKIP: "

// logLine matches the first line of a message a test logged, which go test
// writes indented and prefixed with the file and line that logged it, as in
// "    workitems_test.go:74: the reason".
var logLine = regexp.MustCompile(`^\s+\S+\.go:\d+: `)

// readSkips reads a go test -json stream and returns every test whose last
// verdict is a skip, with the reason it printed.
func readSkips(name string) ([]recordedSkip, error) {
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.Open(filepath.Clean(name))
	if err != nil {
		return nil, fmt.Errorf("open results: %w", err)
	}
	defer func() { _ = file.Close() }()
	return parseSkips(file)
}

// parseSkips is [readSkips] over any reader, sorted by package and test.
//
// A test's output is kept per run: a test run again, by gotestsum's
// --rerun-fails into the same file, starts afresh, so the verdict and the
// reason are both the last run's. A line that is not JSON is refused, as
// [parseResults] refuses it, and so is a stream holding no verdict at all,
// which is not the file -results was meant to name.
func parseSkips(reader io.Reader) ([]recordedSkip, error) {
	runs := map[resultKey]*testRun{}
	sawVerdict := false
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxEventLine)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var event skipEvent
		if err := json.Unmarshal([]byte(text), &event); err != nil {
			return nil, fmt.Errorf("results line %d: %w", line, err)
		}
		if event.Test == "" {
			continue
		}
		key := resultKey{pkg: packageName(event.Package), test: event.Test}
		current := runs[key]
		if current == nil || event.Action == "run" {
			current = &testRun{}
			runs[key] = current
		}
		if event.Action == "output" {
			current.output = append(current.output, event.Output)
			continue
		}
		if status, verdict := verdictStatus(event.Action); verdict {
			current.status = status
			sawVerdict = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read results: %w", err)
	}
	if !sawVerdict {
		return nil, errNoVerdict
	}
	skips := []recordedSkip{}
	for key, current := range runs {
		if current.status == e2ecalls.StatusSkipped {
			skips = append(skips, recordedSkip{Package: key.pkg, Test: key.test, Reason: skipReason(current.output)})
		}
	}
	// A three-way comparison, for the reason [joinResults] gives: the list is
	// built by ranging a map.
	slices.SortFunc(skips, func(a, b recordedSkip) int {
		return cmp.Or(cmp.Compare(a.Package, b.Package), cmp.Compare(a.Test, b.Test))
	})
	return skips, nil
}

// skipReason reads the reason a skipped test printed out of its output: the
// last message it logged before the frame that reports the skip, since t.Skip
// logs its arguments the way t.Log does and then ends the test. A message
// that ran over several lines is joined with spaces, and a frame go test
// wrote between them is left out.
//
// A test that skipped with no message leaves nothing to read, and so does one
// whose output holds no logged line; its reason is empty. One that logged a
// line and then skipped with no message of its own is reported with that
// line, which is the limit of reading a reason back out of the output.
func skipReason(output []string) string {
	end := len(output)
	for index, line := range output {
		if strings.HasPrefix(strings.TrimSpace(line), skipFrame) {
			end = index
			break
		}
	}
	start := -1
	for index := end - 1; index >= 0; index-- {
		if logLine.MatchString(output[index]) {
			start = index
			break
		}
	}
	if start < 0 {
		return ""
	}
	parts := []string{strings.TrimSpace(logLine.ReplaceAllString(output[start], ""))}
	for _, line := range output[start+1 : end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "=== ") {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

// excuses reports whether the declaration covers a skip on the runtime: the
// same runtime and package, the declared test or a subtest of it, and a
// reason carrying the declared fragment.
func (d skipDeclaration) excuses(runtime string, skip recordedSkip) bool {
	if d.Runtime != runtime || d.Package != skip.Package {
		return false
	}
	if skip.Test != d.Test && !strings.HasPrefix(skip.Test, d.Test+"/") {
		return false
	}
	return strings.Contains(skip.Reason, d.Because)
}

// excusedSkip is a skip a declaration covered, with the declaration.
type excusedSkip struct {
	skip recordedSkip
	by   skipDeclaration
}

// skipVerdict is what the skip gate found in one run.
type skipVerdict struct {
	// excused are the skips a declaration covered.
	excused []excusedSkip
	// undeclared are the skips none did.
	undeclared []recordedSkip
	// stale are the declarations for the run's runtime that covered no skip
	// of it.
	stale []skipDeclaration
}

// failed reports whether the verdict fails the run.
func (v skipVerdict) failed() bool {
	return len(v.undeclared) > 0 || len(v.stale) > 0
}

// judgeSkips holds every skip of a run to the declarations for its runtime.
//
// A skip is covered by the first declaration that excuses it, so a second
// declaration of the same skip covers nothing and is reported stale, which is
// how a duplicate entry is found. A declaration for the other runtime is not
// judged here: that run judges it.
func judgeSkips(runtime string, skips []recordedSkip, declarations []skipDeclaration) skipVerdict {
	var verdict skipVerdict
	used := make([]bool, len(declarations))
	for _, skip := range skips {
		index := slices.IndexFunc(declarations, func(d skipDeclaration) bool { return d.excuses(runtime, skip) })
		if index < 0 {
			verdict.undeclared = append(verdict.undeclared, skip)
			continue
		}
		used[index] = true
		verdict.excused = append(verdict.excused, excusedSkip{skip: skip, by: declarations[index]})
	}
	for index, declaration := range declarations {
		if declaration.Runtime == runtime && !used[index] {
			verdict.stale = append(verdict.stale, declaration)
		}
	}
	return verdict
}

// runCheckSkips runs -check-skips over the -results stream of the -runtime
// run and prints what it found: each declared skip with its category, each
// undeclared one and each stale declaration, and a summary line.
func runCheckSkips(opts options, stdout, stderr io.Writer) int {
	if !skipRuntimes[opts.runtime] {
		fmt.Fprintf(stderr, "audit_e2e_coverage: -check-skips needs -runtime ce or ee, the run whose skips it judges, and was given %q\n", opts.runtime)
		return exitUsage
	}
	if opts.results == "" {
		fmt.Fprintln(stderr, "audit_e2e_coverage: -check-skips needs -results, the go test -json stream of that run")
		return exitUsage
	}
	skips, err := readSkips(opts.results)
	if err != nil {
		fmt.Fprintln(stderr, "audit_e2e_coverage: skips:", err)
		return exitUsage
	}
	verdict := judgeSkips(opts.runtime, skips, opts.skipDeclarations)
	for _, excused := range verdict.excused {
		fmt.Fprintf(stdout, "skips: declared (%s): %s %s: %s\n",
			excused.by.Category, excused.skip.Package, excused.skip.Test, excused.skip.reasonText())
	}
	for _, skip := range verdict.undeclared {
		fmt.Fprintf(stderr, "skips: undeclared: %s %s: %s\n", skip.Package, skip.Test, skip.reasonText())
	}
	for _, declaration := range verdict.stale {
		fmt.Fprintf(stderr, "skips: stale declaration: %s %s %s (because %q): no skip of this run matches it\n",
			declaration.Runtime, declaration.Package, declaration.Test, declaration.Because)
	}
	fmt.Fprintf(stdout, "skips: %d skipped on %s, %d declared, %d undeclared, %d stale declarations\n",
		len(skips), opts.runtime, len(verdict.excused), len(verdict.undeclared), len(verdict.stale))
	if verdict.failed() {
		return exitFindings
	}
	return exitOK
}
