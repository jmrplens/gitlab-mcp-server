// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// report_test.go covers what a run prints: the profiles each half was read
// from, one row per package with each half's figure and the merged one, a
// half that never measured a package, percentages that never round a
// package up to whole, and the report appended to a summary on a line of its
// own, or to one that can be written and not read.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFigure_String holds a figure to the statements reached out of the
// statements counted, with the percentage truncated to one decimal, so a
// package one statement short of whole never reads 100.0%, and to saying
// so when there was nothing to count.
func TestFigure_String(t *testing.T) {
	tests := []struct {
		name string
		f    figure
		want string
	}{
		{name: "a third", f: figure{statements: 6, covered: 2}, want: "2/6 (33.3%)"},
		{name: "a sixth, truncated", f: figure{statements: 6, covered: 1}, want: "1/6 (16.6%)"},
		{name: "a half", f: figure{statements: 6, covered: 3}, want: "3/6 (50.0%)"},
		{name: "whole", f: figure{statements: 4, covered: 4}, want: "4/4 (100.0%)"},
		{name: "one short of whole", f: figure{statements: 10000, covered: 9999}, want: "9999/10000 (99.9%)"},
		{name: "nothing reached", f: figure{statements: 7}, want: "0/7 (0.0%)"},
		{name: "nothing to count", f: figure{}, want: "0/0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// fixtureReport is the report of the fixture profiles, offline.out and
// online.out, for the two packages they measure.
const fixtureReport = "### Statement coverage, offline tests and runs against GitLab merged\n" +
	"\n" +
	"Offline: `offline.out`. Runs: `online.out`.\n" +
	"\n" +
	"| Package | Offline tests | Runs | Merged | Reached only by the runs |\n" +
	"| --- | ---: | ---: | ---: | ---: |\n" +
	"| `test/e2e/internal/fixture` | 2/6 (33.3%) | 1/6 (16.6%) | 3/6 (50.0%) | 1 |\n" +
	"| `test/e2e/modeleval` | absent | 4/4 (100.0%) | 4/4 (100.0%) | 4 |\n" +
	"\n" +
	"A half reads absent when none of its profiles carries a block of the package.\n"

// TestRender_Fixture_OneRowPerPackageWithEachHalf pins the whole report of
// the fixture: the profiles each half was read from by name, and one row per
// package in the order asked, the package the offline half never measured
// reading absent there.
func TestRender_Fixture_OneRowPerPackageWithEachHalf(t *testing.T) {
	cov := readHalves(t, offlineProfile, onlineProfile)
	rows, err := cov.rows([]string{fixturePkg, modelevalPkg})
	if err != nil {
		t.Fatal(err)
	}
	sources := []source{{path: "/tmp/a/offline.out", half: offlineHalf}, {path: "/tmp/b/online.out", half: onlineHalf}}

	if got := render(sources, rows); got != fixtureReport {
		t.Errorf("render() =\n%s\nwant\n%s", got, fixtureReport)
	}
}

// TestRender_SeveralProfilesAHalf_NamesEveryOne holds the line above the
// table to every profile of each half, in the order given, so a reader can
// tell whether the licensed run was folded in.
func TestRender_SeveralProfilesAHalf_NamesEveryOne(t *testing.T) {
	sources := []source{
		{path: "dist/e2e-reports/e2e-support-offline.out", half: offlineHalf},
		{path: "dist/e2e-reports/e2e-ce-coverprofile.out", half: onlineHalf},
		{path: "dist/e2e-reports/e2e-ce-final-coverprofile.out", half: onlineHalf},
		{path: "dist/e2e-reports/e2e-ee-coverprofile.out", half: onlineHalf},
	}

	got := render(sources, nil)

	want := "Offline: `e2e-support-offline.out`. Runs: `e2e-ce-coverprofile.out`, `e2e-ce-final-coverprofile.out`, `e2e-ee-coverprofile.out`.\n"
	if !strings.Contains(got, want) {
		t.Errorf("render() =\n%s\nwant it to hold\n%s", got, want)
	}
}

// TestAppendSummary_EarlierText_StartsTheReportOnALineOfItsOwn holds the
// report's heading to the start of a line whatever the summary already
// holds: a file not there yet, or empty, gets the report alone, text a
// newline closes is followed by it directly, and text whose last line no
// newline closes, which an earlier writer may leave, gets one first, since a
// heading continuing that line would not render as a heading.
func TestAppendSummary_EarlierText_StartsTheReportOnALineOfItsOwn(t *testing.T) {
	const report = "### A section\n\nA line.\n"
	tests := []struct {
		name    string
		exists  bool
		earlier string
		want    string
	}{
		{name: "no file yet", want: report + "\n"},
		{name: "an empty file", exists: true, want: report + "\n"},
		{name: "text a newline closes", exists: true, earlier: "An earlier step.\n", want: "An earlier step.\n" + report + "\n"},
		{name: "text a blank line closes", exists: true, earlier: "### An earlier step\n\n", want: "### An earlier step\n\n" + report + "\n"},
		{name: "text no newline closes", exists: true, earlier: "An earlier step.", want: "An earlier step.\n" + report + "\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.md")
			if tc.exists {
				writeProfile(t, filepath.Dir(path), filepath.Base(path), tc.earlier)
			}

			if err := appendSummary(path, report); err != nil {
				t.Fatalf("appendSummary() error = %v", err)
			}

			if got := readFile(t, path); got != tc.want {
				t.Errorf("summary =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// TestAppendSummary_WriteOnlyFile_ReceivesTheReport holds a summary the
// caller may write but not read to receiving the report, as it always did:
// appending asks for nothing but a write, and the newline guard reads the
// last byte on a descriptor of its own, so a file that refuses the read goes
// without the guard rather than without the report. A mode with no read bit
// is such a file on a Unix. Windows has no mode that refuses a read and
// grants a write, and root reads what no mode grants.
func TestAppendSummary_WriteOnlyFile_ReceivesTheReport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no file mode that grants a write and refuses a read")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced, so the file would be read")
	}
	const report = "### A section\n\nA line.\n"
	const earlier = "An earlier step.\n"
	path := writeProfile(t, t.TempDir(), "summary.md", earlier)
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}

	if err := appendSummary(path, report); err != nil {
		t.Fatalf("appendSummary() error = %v", err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, path), earlier+report+"\n"; got != want {
		t.Errorf("summary =\n%q\nwant\n%q", got, want)
	}
}
