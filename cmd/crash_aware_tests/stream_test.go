// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// stream_test.go holds the reading of a go test -json stream to the classes
// the script told apart: a crashed package, a dead one and a failing one, each
// over a recorded stream and over the single lines that decide it.
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The packages the recorded streams under testdata/ name.
const (
	alpha    = "example.com/m/alpha"
	gamma    = "example.com/m/gamma"
	server   = "example.com/m/server"
	toolutil = "example.com/m/toolutil"
)

// fixture reads one recorded stream.
func fixture(t *testing.T, name string) *stream {
	t.Helper()
	s, err := readStream(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return s
}

// streamOf writes events as test2json would, one JSON object per line.
func streamOf(t *testing.T, events ...event) []byte {
	t.Helper()
	var b strings.Builder
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("encode %+v: %v", e, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// TestReadStream_EachRecordedStream_IsClassifiedAsTheScriptClassifiedIt
// verifies every recorded stream against the classes it holds: which
// packages are rerun, which fail the run, and the line each rerun is
// reported with.
func TestReadStream_EachRecordedStream_IsClassifiedAsTheScriptClassifiedIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		file       string
		candidates []string
		failing    []string
		lines      map[string]string
	}{
		{name: "a passing run", file: "pass.json"},
		{
			name:       "a crash and a dead package",
			file:       "crash.json",
			candidates: []string{server, toolutil},
			lines: map[string]string{
				server:   "fatal error: unexpected signal during runtime execution",
				toolutil: deadLine,
			},
		},
		{
			name:       "a failing package beside a crash",
			file:       "failing.json",
			candidates: []string{server},
			failing:    []string{gamma},
			lines:      map[string]string{server: "fatal error: acquireSudog: found s.elem != nil in cache"},
		},
		{
			name:       "a rerun that crashed again",
			file:       "rerun-crash.json",
			candidates: []string{server},
			lines:      map[string]string{server: "fatal error: found pointer to free object"},
		},
		{name: "a rerun that failed", file: "rerun-fail.json", failing: []string{toolutil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := fixture(t, tc.file)
			if got := s.rerunCandidates(); !slices.Equal(got, tc.candidates) {
				t.Errorf("rerun candidates %q, want %q", got, tc.candidates)
			}
			if got := s.failing(); !slices.Equal(got, tc.failing) {
				t.Errorf("failing packages %q, want %q", got, tc.failing)
			}
			for pkg, want := range tc.lines {
				if got := s.rerunLine(pkg); got != want {
					t.Errorf("line for %s is %q, want %q", pkg, got, want)
				}
			}
		})
	}
}

// TestStream_FailedTests_AreListedOncePerTestInOrder verifies that a
// failing package names each of its failing tests, subtests included, once
// and sorted, the way sort -u listed them.
func TestStream_FailedTests_AreListedOncePerTestInOrder(t *testing.T) {
	t.Parallel()

	s := parseStream(streamOf(t,
		event{Action: "fail", Package: gamma, Test: "TestGamma/empty"},
		event{Action: "fail", Package: gamma, Test: "TestGamma"},
		event{Action: "fail", Package: gamma, Test: "TestDelta"},
		event{Action: "fail", Package: gamma, Test: "TestGamma"},
		event{Action: "fail", Package: gamma},
	))
	want := []string{"TestDelta", "TestGamma", "TestGamma/empty"}
	if got := s.testsFailedIn(gamma); !slices.Equal(got, want) {
		t.Errorf("failed tests %q, want %q", got, want)
	}
	if got := s.testsFailedIn(alpha); len(got) != 0 {
		t.Errorf("a package with no failed test lists %q", got)
	}
}

// TestParseStream_ABugInTheCodeUnderTest_IsAFailingPackage verifies that
// every fatal error the runtime reports for a bug in user code, and a panic,
// a build failure and a setup failure, make a failing package rather than a
// crashed or a dead one.
func TestParseStream_ABugInTheCodeUnderTest_IsAFailingPackage(t *testing.T) {
	t.Parallel()

	for _, output := range []string{
		"fatal error: all goroutines are asleep - deadlock!\n",
		"fatal error: concurrent map writes\n",
		"fatal error: concurrent map read and map write\n",
		"fatal error: sync: unlock of unlocked mutex\n",
		"fatal error: runtime: out of memory\n",
		"fatal error: out of memory\n",
		"fatal error: stack overflow\n",
		"fatal error: runtime: program exceeds 10000-thread limit\n",
		"fatal error: checkptr: pointer arithmetic result points to invalid allocation\n",
		"fatal error: unexpected call to os.Exit(0) during test\n",
		"fatal error: too many concurrent timer firings\n",
		"panic: runtime error: invalid memory address or nil pointer dereference\n",
		"panic: test timed out after 30m0s\n",
		"FAIL\texample.com/m/gamma [build failed]\n",
		"FAIL\texample.com/m/gamma [setup failed]\n",
	} {
		t.Run(strings.TrimSuffix(output, "\n"), func(t *testing.T) {
			t.Parallel()

			s := parseStream(streamOf(t,
				event{Action: "output", Package: gamma, Output: output},
				event{Action: "fail", Package: gamma},
			))
			if got := s.rerunCandidates(); len(got) != 0 {
				t.Errorf("rerun candidates %q, want none", got)
			}
			if got := s.failing(); !slices.Equal(got, []string{gamma}) {
				t.Errorf("failing packages %q, want %q", got, []string{gamma})
			}
		})
	}
}

// TestParseStream_ACrashLine_IsReadOnlyFromAnOutputEvent verifies that the
// crash signature counts where the runtime writes it, in the output, and
// not in a field of another event, and that it has to open the line.
func TestParseStream_ACrashLine_IsReadOnlyFromAnOutputEvent(t *testing.T) {
	t.Parallel()

	s := parseStream(streamOf(t,
		event{Action: "run", Package: alpha, Output: "fatal error: unexpected signal during runtime execution\n"},
		event{Action: "output", Package: gamma, Output: "    log: saw fatal error: unexpected signal\n"},
		event{Action: "fail", Package: gamma, Test: "TestGamma"},
		event{Action: "fail", Package: gamma},
	))
	if got := s.rerunCandidates(); len(got) != 0 {
		t.Errorf("rerun candidates %q, want none", got)
	}
}

// TestParseStream_ACrashLaterInAMultiLineOutput_IsACrash verifies that the
// signature is matched at the start of any line of an output event, as jq's
// anchor matched it, and that the line kept is the whole output less its
// final newline.
func TestParseStream_ACrashLaterInAMultiLineOutput_IsACrash(t *testing.T) {
	t.Parallel()

	output := "goroutine 1 [running]:\nfatal error: unexpected signal during runtime execution\n"
	s := parseStream(streamOf(t, event{Action: "output", Package: server, Output: output}))
	if got := s.rerunCandidates(); !slices.Equal(got, []string{server}) {
		t.Fatalf("rerun candidates %q, want %q", got, []string{server})
	}
	if got, want := s.rerunLine(server), strings.TrimSuffix(output, "\n"); got != want {
		t.Errorf("line %q, want %q", got, want)
	}
}

// TestParseStream_ALineThatIsNotAnEvent_IsSkipped verifies that a line no
// JSON decoder reads, and a blank one, leave the events around them judged.
func TestParseStream_ALineThatIsNotAnEvent_IsSkipped(t *testing.T) {
	t.Parallel()

	data := append([]byte("not json\n\n"), streamOf(t, event{Action: "fail", Package: toolutil})...)
	s := parseStream(append(data, []byte(`{"Action":`)...))
	if got := s.rerunCandidates(); !slices.Equal(got, []string{toolutil}) {
		t.Errorf("rerun candidates %q, want %q", got, []string{toolutil})
	}
}

// TestReadStream_NoStream_IsReportedAsSuch verifies that a stream that is
// missing or empty is errNoStream, which run refuses to read as a pass, and
// that a path that cannot be read as a file is an error of its own.
func TestReadStream_NoStream_IsReportedAsSuch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"missing": filepath.Join(dir, "absent.json"), "empty": empty} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := readStream(path); !errors.Is(err, errNoStream) {
				t.Errorf("read %s: %v, want errNoStream", path, err)
			}
		})
	}
	t.Run("a directory", func(t *testing.T) {
		t.Parallel()

		_, err := readStream(dir)
		if err == nil || errors.Is(err, errNoStream) {
			t.Errorf("read a directory: %v, want an error that is not errNoStream", err)
		}
	})
}
