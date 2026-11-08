// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
)

// crashMark matches an output line the runtime writes as it stops the
// process, a crash and a bug in the code under test alike. The anchor is a
// line's start, as jq's was, so a crash later in a multi-line output counts.
var crashMark = regexp.MustCompile(`(?m)^fatal error: `)

// bugMark matches the fatal errors the runtime reports for a bug in the code
// under test: a deadlock, a concurrent map access, a sync misuse, out of
// memory, a goroutine stack past its limit, a thread-limit overrun, a checkptr
// violation, a test calling os.Exit. These fail the run and are not rerun.
var bugMark = regexp.MustCompile(`(?m)^fatal error: (?:all goroutines are asleep|concurrent map |sync: |runtime: out of memory|out of memory|stack overflow|runtime: program exceeds|checkptr|unexpected call to os\.Exit|too many )`)

// explainedMark matches an output line that says why a package failed
// without a test of its own failing: a runtime message of either kind, a
// panic (a test timeout is one, "panic: test timed out"), or a package that
// never ran. A package carrying one of these is a failing package and is not
// rerun.
var explainedMark = regexp.MustCompile(`(?m)^(?:fatal error: |panic: )|\[(?:build|setup) failed\]`)

// deadLine is what a dead package is reported with, in the place a crashed
// one shows its crash line, so the two classes share every report.
const deadLine = "stopped mid-run with no failing test and no message"

// errNoStream is why a run's stream is not judged: the file is missing or
// empty, so nothing in it can say whether the run passed.
var errNoStream = errors.New("no go test -json stream")

// event is the part of one test2json event this command reads. A package
// event carries no Test, which reads as the empty string.
type event struct {
	Action  string
	Package string
	Test    string `json:",omitempty"`
	Output  string `json:",omitempty"`
}

// stream is what one run's go test -json stream says about its packages.
type stream struct {
	// failed holds the packages with a package-level fail event.
	failed map[string]bool
	// failedTests holds, per package, the tests with a fail event, in the
	// order the stream carried them and possibly repeated.
	failedTests map[string][]string
	// crashes holds, per crashed package, the first crash line its stream
	// carried.
	crashes map[string]string
	// explained holds the packages with a line saying why they failed.
	explained map[string]bool
}

// readStream reads the stream at path. A missing or empty file is
// errNoStream; a file that cannot be read is an error of its own.
func readStream(path string) (*stream, error) {
	data, err := os.ReadFile(path) //#nosec G304,G703 -- the stream gotestsum was told to write, below the directory the workflow step names
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(data) == 0) {
		return nil, errNoStream
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseStream(data), nil
}

// parseStream reads a go test -json stream, one event per line. A line that
// is not an event is skipped, so a stray line costs only itself rather than
// every event after it.
func parseStream(data []byte) *stream {
	s := &stream{
		failed:      map[string]bool{},
		failedTests: map[string][]string{},
		crashes:     map[string]string{},
		explained:   map[string]bool{},
	}
	for line := range bytes.Lines(data) {
		var e event
		if json.Unmarshal(line, &e) == nil {
			s.add(e)
		}
	}
	return s
}

// add records what one event says.
func (s *stream) add(e event) {
	switch e.Action {
	case "fail":
		if e.Test == "" {
			s.failed[e.Package] = true
		} else {
			s.failedTests[e.Package] = append(s.failedTests[e.Package], e.Test)
		}
	case "output":
		if explainedMark.MatchString(e.Output) {
			s.explained[e.Package] = true
		}
		if _, seen := s.crashes[e.Package]; !seen && crashMark.MatchString(e.Output) && !bugMark.MatchString(e.Output) {
			s.crashes[e.Package] = strings.TrimSuffix(e.Output, "\n")
		}
	}
}

// rerunCandidates lists the crashed and the dead packages, sorted: a crashed
// one carries a crash line, and a dead one failed with no failing test and no
// line explaining it, a binary that stopped mid-run and said nothing.
func (s *stream) rerunCandidates() []string {
	candidates := slices.Collect(maps.Keys(s.crashes))
	for pkg := range s.failed {
		if len(s.failedTests[pkg]) == 0 && !s.explained[pkg] {
			candidates = append(candidates, pkg)
		}
	}
	slices.Sort(candidates)
	return slices.Compact(candidates)
}

// rerunLine is what a rerun candidate is reported with: its crash line, or
// for a dead package [deadLine].
func (s *stream) rerunLine(pkg string) string {
	if line, crashed := s.crashes[pkg]; crashed {
		return line
	}
	return deadLine
}

// failing lists the packages with a package-level fail event that are not
// rerun candidates, sorted: the packages that fail the run.
func (s *stream) failing() []string {
	candidates := s.rerunCandidates()
	var failing []string
	for pkg := range s.failed {
		if !slices.Contains(candidates, pkg) {
			failing = append(failing, pkg)
		}
	}
	slices.Sort(failing)
	return failing
}

// testsFailedIn lists the tests of pkg with a fail event, subtests included,
// each once and sorted.
func (s *stream) testsFailedIn(pkg string) []string {
	tests := slices.Clone(s.failedTests[pkg])
	slices.Sort(tests)
	return slices.Compact(tests)
}
