// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// check_test.go covers the gate CI's coverage job runs: the offline half on
// this tree merged with the committed record of the complete runs, each
// package held to the number of statements the record says neither half
// reached, a record that predates the tree failing at a stack's top and
// noted below it, a package that has room under the record, the report that
// says all of it, and the state before any complete run is recorded.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// checkCase is one run of the gate over the fixture tree.
type checkCase struct {
	files    map[string]string
	offline  string
	noRecord bool
	deferred bool
	packages []string
}

// runCheck runs the gate over the fixture tree as the case edits it,
// returning the report it printed and the error it returned.
func runCheck(t *testing.T, tc checkCase) (string, error) {
	t.Helper()

	writeTree(t, tc.files)
	dir := t.TempDir()
	offline := tc.offline
	if offline == "" {
		offline = recordOffline
	}
	recordPath := filepath.Join(dir, "e2e-support-coverage.json")
	if !tc.noRecord {
		writeProfile(t, dir, "e2e-support-coverage.json", fixtureRecord)
	}
	packages := tc.packages
	if packages == nil {
		packages = recordPackages
	}
	opts := options{
		packages:    packages,
		sources:     []source{{path: writeProfile(t, dir, "offline.out", offline), half: offlineHalf}},
		checkRecord: recordPath,
		deferred:    tc.deferred,
	}
	var stdout bytes.Buffer
	err := check(opts, &stdout)
	return stdout.String(), err
}

// checkTable is the part of every report above its notes.
const checkTable = "### Statement coverage of the e2e support packages, held to the recorded runs\n" +
	"\n" +
	"Offline: `offline.out`. Runs: `e2e-support-coverage.json`, recorded on 2026-11-19 from `online.out`.\n" +
	"\n" +
	"| Package | Offline tests | Recorded runs | Merged | Reached by neither | The record allows |\n" +
	"| --- | ---: | ---: | ---: | ---: | ---: |\n"

// TestCheck_TreeTheRecordWasTakenFrom_PassesWithNothingToSay pins the whole
// report of the tree the record describes: each package with the offline
// figure, the runs' figure read from the record, the merge, and the
// statements neither half reached, which is what the record allows, so
// there is no note and nothing fails, whichever way freshness is asked.
func TestCheck_TreeTheRecordWasTakenFrom_PassesWithNothingToSay(t *testing.T) {
	want := checkTable +
		"| `test/e2e/internal/fixture` | 2/5 (40.0%) | 3/5 (60.0%) | 4/5 (80.0%) | 1 | 1 |\n" +
		"| `test/e2e/modeleval` | 2/2 (100.0%) | absent | 2/2 (100.0%) | 0 | 0 |\n" +
		"\n" +
		"A half reads absent when none of its profiles carries a block of the package.\n"
	for _, deferred := range []bool{false, true} {
		t.Run(map[bool]string{false: "checked", true: "deferred"}[deferred], func(t *testing.T) {
			got, err := runCheck(t, checkCase{deferred: deferred})
			if err != nil {
				t.Errorf("check() error = %v, want nil", err)
			}
			if got != want {
				t.Errorf("report =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestCheck_NoRecordYet_ReportsTheOfflineHalfAndHoldsNothing pins the report
// before the first complete run is recorded: the runs read absent, no
// package has a ceiling, the line says how the record gets written, and the
// gate passes, since there is nothing yet to hold the packages to.
func TestCheck_NoRecordYet_ReportsTheOfflineHalfAndHoldsNothing(t *testing.T) {
	got, err := runCheck(t, checkCase{noRecord: true})
	if err != nil {
		t.Errorf("check() error = %v, want nil", err)
	}
	want := "### Statement coverage of the e2e support packages, held to the recorded runs\n" +
		"\n" +
		"Offline: `offline.out`. Runs: none recorded, `e2e-support-coverage.json` does not exist yet.\n" +
		"\n" +
		"| Package | Offline tests | Recorded runs | Merged | Reached by neither | The record allows |\n" +
		"| --- | ---: | ---: | ---: | ---: | ---: |\n" +
		"| `test/e2e/internal/fixture` | 2/5 (40.0%) | absent | 2/5 (40.0%) | 3 | none |\n" +
		"| `test/e2e/modeleval` | 2/2 (100.0%) | absent | 2/2 (100.0%) | 0 | none |\n" +
		"\n" +
		"A half reads absent when none of its profiles carries a block of the package.\n" +
		"\n" +
		"- No complete run is recorded, so no package is held to a ceiling yet: after the complete runs the instance allows " +
		"(make test-e2e-ce, and make test-e2e-ee with an EE license), make e2e-support-coverage-record writes the record.\n"
	if got != want {
		t.Errorf("report =\n%s\nwant\n%s", got, want)
	}
}

// The edits of the tree and of the offline half the verdicts below run on.
var (
	// aChanged edits A's body in place, so its recorded reach is dropped.
	aChanged = map[string]string{"test/e2e/internal/fixture/a.go": strings.Replace(fixtureSource, "_ = 2", "_ = 3", 1)}
	// modelevalLost is the offline half no longer reaching modeleval.
	modelevalLost = strings.Replace(recordOffline, "c.go:3.11,4.7 2 1", "c.go:3.11,4.7 2 0", 1)
	// newStatement is the offline half of a tree with one more fixture
	// statement, in a file the record never saw, that it does not reach.
	newStatement = recordOffline + fixturePkg + "/d.go:3.11,4.7 1 0\n"
	// aSecondOffline is the offline half reaching A's second block too.
	aSecondOffline = strings.Replace(recordOffline, "a.go:5.7,7.3 1 0", "a.go:5.7,7.3 1 1", 1)
	// cOffline is the offline half reaching C, which no half reached.
	cOffline = strings.Replace(recordOffline, "a.go:13.11,14.7 1 0", "a.go:13.11,14.7 1 1", 1)
)

// The lines the verdicts below are held to.
const (
	droppedA = "- `test/e2e/internal/fixture/a.go` `func A`: " + whyChanged +
		", so what the runs reached in it no longer counts.\n"
	fixtureOver = "`test/e2e/internal/fixture`: 2 statements are reached by neither half, and the record allows 1."
	failTail    = " Reach them with a test, or record a complete run of this tree: the complete runs the instance allows " +
		"(make test-e2e-ce, and make test-e2e-ee with an EE license), then make e2e-support-coverage-record."
)

// TestCheck_Verdicts holds each package to the record's ceiling: a package
// with more statements reached by neither half fails, with the way out; at a
// stack's top (checked) it fails however it came about, and below it
// (deferred) it is a note only when the record predates the package, because
// a declaration changed or the package counts other statements, since then
// the top records a complete run of its tree; an offline test lost fails
// either way; a package below its ceiling passes with a note; and a dropped
// declaration whose statements the offline tests reach passes.
func TestCheck_Verdicts(t *testing.T) {
	tests := []struct {
		name     string
		tc       checkCase
		failure  string
		holds    []string
		notHolds []string
	}{
		{
			name:    "an offline test lost, checked",
			tc:      checkCase{offline: modelevalLost},
			failure: "`test/e2e/modeleval`: 2 statements are reached by neither half, and the record allows 0." + failTail,
			holds:   []string{"| `test/e2e/modeleval` | 0/2 (0.0%) | absent | 0/2 (0.0%) | 2 | 0 |\n"},
		},
		{
			name:    "an offline test lost, deferred",
			tc:      checkCase{offline: modelevalLost, deferred: true},
			failure: "`test/e2e/modeleval`: 2 statements are reached by neither half, and the record allows 0." + failTail,
		},
		{
			name:    "a declaration changed, checked",
			tc:      checkCase{files: aChanged},
			failure: fixtureOver + failTail,
			holds: []string{
				"| `test/e2e/internal/fixture` | 2/5 (40.0%) | 1/5 (20.0%) | 3/5 (60.0%) | 2 | 1 |\n",
				"\n" + droppedA,
				"- **Fails**: " + fixtureOver + failTail + "\n",
			},
		},
		{
			name: "a declaration changed, deferred",
			tc:   checkCase{files: aChanged, deferred: true},
			holds: []string{
				droppedA,
				"- " + fixtureOver + " The record predates the package (recorded declarations that no longer match: 1; statements " +
					"now and in the record: 5 and 5), so below a stack's top this is a note: the top records a complete run of its tree.\n",
			},
			notHolds: []string{"**Fails**"},
		},
		{
			name:    "a statement the record never saw, checked",
			tc:      checkCase{offline: newStatement},
			failure: fixtureOver + failTail,
		},
		{
			name: "a statement the record never saw, deferred",
			tc:   checkCase{offline: newStatement, deferred: true},
			holds: []string{
				"| `test/e2e/internal/fixture` | 2/6 (33.3%) | 3/6 (50.0%) | 4/6 (66.6%) | 2 | 1 |\n",
				"(recorded declarations that no longer match: 0; statements now and in the record: 6 and 5)",
			},
		},
		{
			name: "a changed declaration the offline tests reach",
			tc:   checkCase{files: aChanged, offline: aSecondOffline},
			holds: []string{
				"| `test/e2e/internal/fixture` | 3/5 (60.0%) | 1/5 (20.0%) | 4/5 (80.0%) | 1 | 1 |\n",
				droppedA,
			},
			notHolds: []string{"statements are reached by neither half"},
		},
		{
			name: "room under the record",
			tc:   checkCase{offline: cOffline},
			holds: []string{
				"- `test/e2e/internal/fixture`: 0 statements are reached by neither half, fewer than the 1 the record allows; " +
					"the next record lowers it.\n",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runCheck(t, tc.tc)

			if tc.failure == "" && err != nil || tc.failure != "" && (err == nil || err.Error() != tc.failure) {
				t.Errorf("check() error = %v, want %q", err, tc.failure)
			}
			for _, want := range tc.holds {
				if !strings.Contains(got, want) {
					t.Errorf("report =\n%s\nwant it to hold\n%s", got, want)
				}
			}
			for _, unwanted := range tc.notHolds {
				if strings.Contains(got, unwanted) {
					t.Errorf("report =\n%s\nwant it not to hold %q", got, unwanted)
				}
			}
		})
	}
}

// TestDroppedIn_Packages_CountTheirOwnDeclarations holds the count each
// verdict is given to the dropped declarations of its own package, named as
// the record names it: the module's root package by the empty name, whose
// files the record names with no directory, a package below it by its
// directory, and a package with no dropped declaration of its own at none.
func TestDroppedIn_Packages_CountTheirOwnDeclarations(t *testing.T) {
	gone := []dropped{
		{file: "root.go", declaration: "func Root", why: whyChanged},
		{file: "test/e2e/modeleval/c.go", declaration: "func C", why: whyGone},
	}
	tests := []struct {
		name string
		pkg  string
		want int
	}{
		{name: "the module's root package", pkg: "", want: 1},
		{name: "a package below the root", pkg: "test/e2e/modeleval", want: 1},
		{name: "a package with none dropped", pkg: "test/e2e/internal/fixture", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := droppedIn(gone, tc.pkg); got != tc.want {
				t.Errorf("droppedIn(%q) = %d, want %d", tc.pkg, got, tc.want)
			}
		})
	}
}

// TestCheck_Failures_TwoPackagesJoined holds a gate that fails for two
// packages to one error naming both, in the order asked.
func TestCheck_Failures_TwoPackagesJoined(t *testing.T) {
	_, err := runCheck(t, checkCase{files: aChanged, offline: modelevalLost})

	want := fixtureOver + failTail + "; `test/e2e/modeleval`: 2 statements are reached by neither half, and the record allows 0." + failTail
	if err == nil || err.Error() != want {
		t.Errorf("check() error = %v, want %q", err, want)
	}
}

// TestCheck_RunsItCannotJudge_AreRefusedBeforeAnyReport holds every input the
// gate cannot rest on to an error and no report: an offline profile it
// cannot read, a record it cannot read, a record of other packages, a record
// whose reach it cannot apply, and a package no profile carries.
func TestCheck_RunsItCannotJudge_AreRefusedBeforeAnyReport(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, opts *options)
		want  string
	}{
		{
			name: "an offline profile it cannot read",
			setup: func(_ *testing.T, opts *options) {
				opts.sources[0].path = filepath.Join(filepath.Dir(opts.checkRecord), "missing.out")
			},
			want: "missing.out: open ",
		},
		{
			name: "a record it cannot read",
			setup: func(t *testing.T, opts *options) {
				t.Helper()
				writeProfile(t, filepath.Dir(opts.checkRecord), filepath.Base(opts.checkRecord), "{")
			},
			want: "unexpected EOF",
		},
		{
			name:  "a record of other packages",
			setup: func(_ *testing.T, opts *options) { opts.packages = []string{fixturePkg} },
			want:  "the record measures [test/e2e/internal/fixture test/e2e/modeleval]",
		},
		{
			name: "a record whose reach it cannot apply",
			setup: func(t *testing.T, opts *options) {
				t.Helper()
				writeProfile(t, filepath.Dir(opts.checkRecord), filepath.Base(opts.checkRecord), strings.Replace(fixtureRecord, `"1.7,3.3 1"`, `"x"`, 1))
			},
			want: `the reached block "x" is not spelled`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writeTree(t, nil)
			dir := t.TempDir()
			opts := options{
				packages:    recordPackages,
				sources:     []source{{path: writeProfile(t, dir, "offline.out", recordOffline), half: offlineHalf}},
				checkRecord: writeProfile(t, dir, "e2e-support-coverage.json", fixtureRecord),
			}
			tc.setup(t, &opts)
			var stdout bytes.Buffer

			err := check(opts, &stdout)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("check() error = %v, want it to hold %q", err, tc.want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want no report", stdout.String())
			}
		})
	}
	t.Run("a package no profile carries", func(t *testing.T) {
		_, err := runCheck(t, checkCase{noRecord: true, packages: []string{fixturePkg, modelevalPkg + "/internal/provider"}})

		if err == nil || !strings.Contains(err.Error(), "no profile carries "+modelevalPkg+"/internal/provider") {
			t.Errorf("check() error = %v, want the package refused", err)
		}
	})
}

// TestCheck_Summary_AppendsTheReportOrRefusesTheWrite holds the gate to
// appending its report to the summary it is given, as the workflow's step
// summary, and to failing when that file cannot be opened.
func TestCheck_Summary_AppendsTheReportOrRefusesTheWrite(t *testing.T) {
	writeTree(t, nil)
	dir := t.TempDir()
	opts := options{
		packages:    recordPackages,
		sources:     []source{{path: writeProfile(t, dir, "offline.out", recordOffline), half: offlineHalf}},
		checkRecord: writeProfile(t, dir, "e2e-support-coverage.json", fixtureRecord),
		summary:     writeProfile(t, dir, "summary.md", "### An earlier step\n\n"),
	}
	var stdout bytes.Buffer

	if err := check(opts, &stdout); err != nil {
		t.Fatalf("check() error = %v", err)
	}

	if got, want := readFile(t, opts.summary), "### An earlier step\n\n"+stdout.String()+"\n"; got != want {
		t.Errorf("summary =\n%s\nwant\n%s", got, want)
	}
	opts.summary = filepath.Join(dir, "no-such-dir", "summary.md")
	if err := check(opts, &stdout); err == nil || !strings.Contains(err.Error(), "appending the report to ") {
		t.Errorf("check() with an unwritable summary error = %v, want the refused write", err)
	}
}
