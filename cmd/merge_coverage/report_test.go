// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// report_test.go covers what a run prints: the profiles each half was read
// from, one row per package with each half's figure and the merged one, a
// half that never measured a package, and percentages that never round a
// package up to whole.

package main

import (
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
