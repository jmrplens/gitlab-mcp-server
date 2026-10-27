// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// judge_test.go covers how the findings of every platform are merged, held to
// the severity rule and the declaration table, and printed.
package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"
)

// scanOf is one platform's report carrying the given findings.
func scanOf(platform string, osFindings []finding, binaries ...finding) scan {
	return scan{
		path: platform + ".json", platform: platform, artifact: "gitlab-mcp-server:smoke", image: "sha256:1f4c", trivyVersion: "0.76.1",
		distro: "alpine 3.24.2", packages: 18, os: osFindings, binaries: binaries,
	}
}

// fixtureDB is the database the judged fixtures name.
var fixtureDB = database{
	trivyVersion: "0.76.1", schema: 2,
	updatedAt:  time.Date(2026, 10, 25, 7, 10, 41, 0, time.UTC),
	nextUpdate: time.Date(2026, 10, 26, 7, 10, 41, 0, time.UTC),
}

var (
	fixableHigh     = finding{id: "CVE-2026-1001", pkg: "libssl3", installed: "3.5.8-r0", fixed: "3.5.8-r1", severity: "HIGH", source: "nvd", title: "a heap overflow"}
	fixableCritical = finding{id: "CVE-2026-1002", pkg: "busybox", installed: "1.37.0-r31", fixed: "1.37.0-r32", severity: "CRITICAL", source: "nvd"}
	fixableMedium   = finding{id: "CVE-2026-85091", pkg: "zlib", installed: "1.3.2-r0", fixed: "1.3.2-r1", severity: "MEDIUM", vendors: []string{"ubuntu"}}
	fixableLow      = finding{id: "CVE-2026-1003", pkg: "zlib", installed: "1.3.2-r0", fixed: "1.3.2-r1", severity: "LOW", source: "nvd"}
	fixableUnknown  = finding{id: "CVE-2026-107891", pkg: "musl", installed: "1.2.6-r2", fixed: "1.2.6-r3", severity: "UNKNOWN"}
	unfixedCritical = finding{id: "CVE-2026-1004", pkg: "musl", installed: "1.2.6-r2", severity: "CRITICAL", source: "nvd"}
	binaryHigh      = finding{id: "CVE-2026-111943", pkg: "stdlib", installed: "v1.27.2", fixed: "1.26.10, 1.27.3", severity: "HIGH", vendors: []string{"ghsa"}}
)

// TestJudge_AFixableHighOrCriticalFindingFailsTheRun is the gate doing its
// job: an OS package with a fixed version available and a HIGH or CRITICAL
// rating fails the run when nothing declares it.
func TestJudge_AFixableHighOrCriticalFindingFailsTheRun(t *testing.T) {
	t.Parallel()

	for _, f := range []finding{fixableHigh, fixableCritical} {
		t.Run(f.severity, func(t *testing.T) {
			t.Parallel()

			v := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{f})}, nil, fixtureNow)
			if v.ok() || v.undeclared() != 1 || len(v.findings) != 1 || !v.findings[0].gates() {
				t.Fatalf("verdict = %+v, want the one %s finding gating and undeclared", v, f.severity)
			}
		})
	}
}

// TestJudge_WhatIsNotFixableAndSevere_IsReportedOnly covers each finding the
// rule leaves to the log: a CRITICAL with no fix to take, and a fix rated
// MEDIUM, LOW or by no source at all.
func TestJudge_WhatIsNotFixableAndSevere_IsReportedOnly(t *testing.T) {
	t.Parallel()

	for _, f := range []finding{unfixedCritical, fixableMedium, fixableLow, fixableUnknown} {
		t.Run(f.id+" "+f.severity, func(t *testing.T) {
			t.Parallel()

			v := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{f})}, nil, fixtureNow)
			if !v.ok() || len(v.findings) != 1 || v.findings[0].gates() {
				t.Fatalf("verdict = %+v, want the finding reported and the run passed", v)
			}
		})
	}
}

// TestJudge_MergesOneFindingAcrossThePlatforms covers the merge: one advisory
// against one package at one version is one finding naming every platform
// that carries it, in the order they were scanned, and the findings come out
// sorted by advisory and package.
func TestJudge_MergesOneFindingAcrossThePlatforms(t *testing.T) {
	t.Parallel()

	v := judge(fixtureDB, []scan{
		scanOf("linux/amd64", []finding{fixableMedium, fixableHigh}, binaryHigh),
		scanOf("linux/arm64", []finding{fixableHigh, fixableMedium}, binaryHigh),
	}, nil, fixtureNow)
	if len(v.findings) != 2 || v.findings[0].id != fixableHigh.id || v.findings[1].id != fixableMedium.id {
		t.Fatalf("findings = %+v, want the HIGH and the MEDIUM once each, sorted", v.findings)
	}
	for _, j := range append(v.findings, v.binaries...) {
		if want := []string{"linux/amd64", "linux/arm64"}; !slices.Equal(j.platforms, want) {
			t.Errorf("%s platforms = %v, want %v", j.id, j.platforms, want)
		}
	}
	if len(v.binaries) != 1 || v.undeclared() != 1 {
		t.Errorf("binaries %+v, undeclared %d; want the binary advisory once and only the HIGH OS finding undeclared", v.binaries, v.undeclared())
	}
}

// TestJudge_TheSameAdvisoryAtAnotherVersion_IsAFindingOfItsOwn: a platform
// carrying the package at another version is a different finding, so neither
// version is reported with the other's numbers.
func TestJudge_TheSameAdvisoryAtAnotherVersion_IsAFindingOfItsOwn(t *testing.T) {
	t.Parallel()

	older := fixableHigh
	older.installed = "3.5.7-r0"
	v := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh}), scanOf("linux/arm64", []finding{older})}, nil, fixtureNow)
	if len(v.findings) != 2 || v.undeclared() != 2 {
		t.Fatalf("findings = %+v, want the two versions apart and both undeclared", v.findings)
	}
}

// TestJudge_TheBinaryIsReportedAndNotJudged covers the scope of the gate: an
// advisory against a module of the server binary is printed, as trivy image
// prints it, and never fails the run, since make check-binary-vulns holds the
// binary to the Go vulnerability database and each question keeps one oracle.
func TestJudge_TheBinaryIsReportedAndNotJudged(t *testing.T) {
	t.Parallel()

	v := judge(fixtureDB, []scan{scanOf("linux/amd64", nil, binaryHigh)}, nil, fixtureNow)
	if !v.ok() || len(v.binaries) != 1 || len(v.findings) != 0 {
		t.Fatalf("verdict = %+v, want the binary advisory reported and the run passed", v)
	}
}

// TestJudge_HoldsTheFindingsToTheTable covers the outcomes an entry can have
// in a run of every platform the release pushes: accepting a finding,
// matching nothing, matching only a finding the rule does not gate on (which
// excuses nothing and is stale), and not being readable, the last of which
// fails the run even when its key matches.
func TestJudge_HoldsTheFindingsToTheTable(t *testing.T) {
	t.Parallel()

	accepted := declaration{category: categoryNotAffected, reason: "the server never calls into libssl3 for this"}
	v := judge(fixtureDB, []scan{
		scanOf("linux/amd64", []finding{fixableHigh, fixableMedium}),
		scanOf("linux/arm64", []finding{fixableMedium}),
	}, map[string]declaration{
		declarationKey(fixableHigh.id, fixableHigh.pkg):     accepted,
		declarationKey(fixableMedium.id, fixableMedium.pkg): {category: categoryNotAffected, reason: "below the bar anyway"},
		"CVE-2099-0009 gone":                                {category: categoryNotAffected, reason: "the package left the image"},
	}, fixtureNow)
	if len(v.findings) != 2 || v.undeclared() != 0 || v.findings[0].declaration == nil || *v.findings[0].declaration != accepted {
		t.Fatalf("findings = %+v, want the HIGH accepted by its declaration", v.findings)
	}
	if v.findings[1].declaration != nil {
		t.Errorf("the MEDIUM finding carries a declaration %+v; a declaration accepts only what the rule gates on", v.findings[1].declaration)
	}
	if want := []string{"CVE-2026-85091 zlib", "CVE-2099-0009 gone"}; !slices.Equal(v.stale, want) {
		t.Errorf("stale = %v, want %v", v.stale, want)
	}
	if v.ok() {
		t.Error("a run with stale declarations passed")
	}

	unreviewed := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh})}, map[string]declaration{
		declarationKey(fixableHigh.id, fixableHigh.pkg): {category: "invented", reason: "x"},
	}, fixtureNow)
	if unreviewed.undeclared() != 0 || len(unreviewed.stale) != 0 || len(unreviewed.invalid) != 1 || unreviewed.ok() {
		t.Errorf("undeclared %d, stale %v, invalid %v, ok %v; want only the unknown category reported, and the run failed",
			unreviewed.undeclared(), unreviewed.stale, unreviewed.invalid, unreviewed.ok())
	}

	clean := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh})}, map[string]declaration{
		declarationKey(fixableHigh.id, fixableHigh.pkg): accepted,
	}, fixtureNow)
	if !clean.ok() {
		t.Errorf("verdict = %+v, want a run whose one finding is declared to pass", clean)
	}
}

// TestJudge_ARunOfFewerPlatforms_LeavesStalenessUnjudged covers a run told to
// scan only some of the platforms the release pushes, as -platforms
// linux/amd64 does: a declaration that accepts none of its findings may
// accept another platform's, so the run cannot call it stale and passes,
// listing it as unjudged. A run that scans every platform the release
// pushes, in any order and with others beside them, judges it stale.
func TestJudge_ARunOfFewerPlatforms_LeavesStalenessUnjudged(t *testing.T) {
	t.Parallel()

	key := declarationKey(fixableHigh.id, fixableHigh.pkg)
	declared := map[string]declaration{key: {category: categoryNotAffected, reason: "the server never calls into libssl3 for this"}}
	for _, tc := range []struct {
		name      string
		platforms []string
		stale     bool
	}{
		{name: "one platform", platforms: []string{"linux/amd64"}},
		{name: "the other platform", platforms: []string{"linux/arm64"}},
		{name: "a platform the release does not push", platforms: []string{"linux/riscv64"}},
		{name: "every platform the release pushes", platforms: []string{"linux/amd64", "linux/arm64"}, stale: true},
		{name: "in another order", platforms: []string{"linux/arm64", "linux/amd64"}, stale: true},
		{name: "and one more", platforms: []string{"linux/amd64", "linux/riscv64", "linux/arm64"}, stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scans := make([]scan, 0, len(tc.platforms))
			for _, platform := range tc.platforms {
				scans = append(scans, scanOf(platform, []finding{fixableMedium}))
			}
			v := judge(fixtureDB, scans, declared, fixtureNow)
			wantStale, wantUnjudged := []string(nil), []string{key}
			if tc.stale {
				wantStale, wantUnjudged = wantUnjudged, nil
			}
			if !slices.Equal(v.stale, wantStale) || !slices.Equal(v.unjudged, wantUnjudged) || v.ok() == tc.stale {
				t.Errorf("stale %v, unjudged %v, ok %v; want stale %v, unjudged %v and the run failed only when stale",
					v.stale, v.unjudged, v.ok(), wantStale, wantUnjudged)
			}
		})
	}
}

// TestJudge_AFixNotYetAdoptable_ExpiresAfterItsDay covers the date a
// fix-not-yet-adoptable entry carries: it holds through that day in UTC and
// fails the run from the next, when the bump that takes the fix should have
// landed and the entry should be gone.
func TestJudge_AFixNotYetAdoptable_ExpiresAfterItsDay(t *testing.T) {
	t.Parallel()

	pending := declaration{category: categoryFixNotYetAdoptable, reason: "alpine 3.24.3 carries 3.5.8-r1", until: "2026-10-25"}
	declared := map[string]declaration{declarationKey(fixableHigh.id, fixableHigh.pkg): pending}
	for _, tc := range []struct {
		now     time.Time
		name    string
		expired bool
	}{
		{name: "the day before", now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)},
		{name: "the last moment of the day", now: time.Date(2026, 10, 25, 23, 59, 59, 999999999, time.UTC)},
		{name: "the next day", now: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), expired: true},
		{name: "the next day in UTC, still the day where the run is", now: time.Date(2026, 10, 25, 23, 0, 0, 0, time.FixedZone("UTC-2", -2*3600)), expired: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh})}, declared, tc.now)
			if got := len(v.expired) == 1; got != tc.expired || v.ok() == tc.expired || v.undeclared() != 0 {
				t.Fatalf("expired %v, ok %v, undeclared %d; want expired %v", v.expired, v.ok(), v.undeclared(), tc.expired)
			}
		})
	}

	notAffected := map[string]declaration{declarationKey(fixableHigh.id, fixableHigh.pkg): {category: categoryNotAffected, reason: "x"}}
	if v := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh})}, notAffected, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)); !v.ok() {
		t.Errorf("verdict = %+v, want a not-affected entry, which names no day, never to expire", v)
	}
	for _, tc := range []struct {
		name  string
		entry declaration
	}{
		{name: "a day that does not parse", entry: declaration{category: categoryFixNotYetAdoptable, reason: "x", until: "soon"}},
		{name: "a day on a not-affected entry", entry: declaration{category: categoryNotAffected, reason: "x", until: "2026-10-24"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unreadable := map[string]declaration{declarationKey(fixableHigh.id, fixableHigh.pkg): tc.entry}
			if v := judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh})}, unreadable, fixtureNow); len(v.expired) != 0 || len(v.invalid) != 1 {
				t.Errorf("expired %v, invalid %v; want the entry reported as invalid and not as expired", v.expired, v.invalid)
			}
		})
	}
}

// TestVerdictWrite_SaysWhatWasScannedAndTheVerdict covers each shape of the
// report: a clean run, a run whose findings are all declared, and a failed
// one naming each kind of failure, with every line a finding can print and
// the three ways a rating names where it came from.
func TestVerdictWrite_SaysWhatWasScannedAndTheVerdict(t *testing.T) {
	t.Parallel()

	header := []string{
		"audit_image_vulns: linux/amd64: alpine 3.24.2, 18 OS packages, in gitlab-mcp-server:smoke, image sha256:1f4c, scanned by Trivy 0.76.1\n",
		"audit_image_vulns: judged against Trivy's vulnerability database, schema 2, updated 2026-10-25T07:10:41Z, next update due 2026-10-26T07:10:41Z\n",
		"audit_image_vulns: an OS package finding fails the run when it has a fixed version and is rated HIGH or CRITICAL, unless declarations.go accepts it; " +
			"the rating is the one Trivy selects by default, from the advisory's own source (Alpine's secdb rates nothing), then NVD, then other vendors, and each finding names where its rating came from\n",
	}
	pending := declaration{category: categoryFixNotYetAdoptable, reason: "alpine 3.24.3 carries 3.5.8-r1", until: "2026-11-02"}
	for _, tc := range []struct {
		name string
		v    verdict
		want []string
	}{
		{
			name: "clean",
			v:    judge(fixtureDB, []scan{scanOf("linux/amd64", nil), scanOf("linux/arm64", nil)}, nil, fixtureNow),
			want: append(slices.Clone(header), "audit_image_vulns: no fixable HIGH or CRITICAL advisory affects an OS package of the 2 images\n"),
		},
		{
			name: "every gating finding declared",
			v: judge(fixtureDB, []scan{scanOf("linux/amd64", []finding{fixableHigh, fixableCritical, fixableMedium, fixableUnknown, unfixedCritical}, binaryHigh)}, map[string]declaration{
				declarationKey(fixableHigh.id, fixableHigh.pkg):         pending,
				declarationKey(fixableCritical.id, fixableCritical.pkg): {category: categoryNotAffected, reason: "the image runs no busybox applet"},
			}, fixtureNow),
			want: []string{
				"accepted CVE-2026-1001 libssl3 3.5.8-r0, fixed in 3.5.8-r1, HIGH from nvd, in linux/amd64 [fix-not-yet-adoptable until 2026-11-02]: alpine 3.24.3 carries 3.5.8-r1\n",
				"accepted CVE-2026-1002 busybox 1.37.0-r31, fixed in 1.37.0-r32, CRITICAL from nvd, in linux/amd64 [not-affected]: the image runs no busybox applet\n",
				"reported CVE-2026-1004 musl 1.2.6-r2, no fixed version, CRITICAL from nvd, in linux/amd64\n",
				"reported CVE-2026-107891 musl 1.2.6-r2, fixed in 1.2.6-r3, UNKNOWN from no source, in linux/amd64\n",
				"reported CVE-2026-85091 zlib 1.3.2-r0, fixed in 1.3.2-r1, MEDIUM from other vendors (ubuntu), in linux/amd64\n",
				"binary CVE-2026-111943 stdlib v1.27.2, fixed in 1.26.10, 1.27.3, HIGH from other vendors (ghsa), in linux/amd64: make check-binary-vulns judges the binary\n",
				"audit_image_vulns: 2 fixable HIGH or CRITICAL findings in the 1 images, each accepted by a declaration\n",
			},
		},
		{
			name: "a run of fewer platforms",
			v: judge(fixtureDB, []scan{scanOf("linux/amd64", nil)}, map[string]declaration{
				"CVE-2099-0009 gone": {category: categoryNotAffected, reason: "x"},
			}, fixtureNow),
			want: []string{
				`unjudged declaration "CVE-2099-0009 gone": no image of this run carries a fixable HIGH or CRITICAL finding it accepts, ` +
					"and the run scans linux/amd64 rather than every platform the release pushes (linux/amd64, linux/arm64), " +
					"so a run that scans them all decides whether it is stale\n",
				"audit_image_vulns: no fixable HIGH or CRITICAL advisory affects an OS package of the 1 images\n",
			},
		},
		{
			name: "failed",
			v: judge(fixtureDB, []scan{
				scanOf("linux/amd64", []finding{fixableHigh, fixableCritical}),
				scanOf("linux/arm64", []finding{fixableHigh, fixableCritical}),
			}, map[string]declaration{
				declarationKey(fixableCritical.id, fixableCritical.pkg): {category: categoryFixNotYetAdoptable, reason: "fixed in 1.37.0-r32", until: "2026-10-24"},
				"CVE-2099-0009 gone": {category: categoryNotAffected, reason: "x"},
				"broken":             {category: categoryNotAffected, reason: "x"},
			}, fixtureNow),
			want: []string{
				"FINDING CVE-2026-1001 libssl3 3.5.8-r0, fixed in 3.5.8-r1, HIGH from nvd, in linux/amd64, linux/arm64: a heap overflow\n",
				`STALE declaration "CVE-2099-0009 gone": no image of this run carries a fixable HIGH or CRITICAL finding it accepts` + "\n",
				`STALE declaration "broken": no image of this run carries a fixable HIGH or CRITICAL finding it accepts` + "\n",
				`EXPIRED declaration "CVE-2026-1002 busybox": its fix was to be adoptable by 2026-10-24, and nothing has taken it` + "\n",
				`INVALID declaration "broken": the key is not an advisory id and a package separated by one space` + "\n",
				"audit_image_vulns: FAILED: 1 undeclared findings, 2 stale, 1 expired and 1 invalid declarations\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			tc.v.write(&out)
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("the report lacks %q:\n%s", want, out.String())
				}
			}
			for _, want := range header {
				if !strings.HasPrefix(out.String(), header[0]) || !strings.Contains(out.String(), want) {
					t.Errorf("the report does not open with what it scanned and judged against, %q:\n%s", want, out.String())
				}
			}
		})
	}
}
