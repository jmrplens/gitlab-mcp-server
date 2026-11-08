// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// merge_test.go covers the merge itself: a block two profiles carry counted
// once, a statement only the runs reached, a package one half never measured,
// the modes and counts of the merged profile, and the disagreements between
// profiles that are refused rather than merged.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two packages the fixture profiles below measure, under the import
// paths a profile names their files by.
const (
	fixturePkg   = "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	modelevalPkg = "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval"
)

// offlineProfile is the offline half of the fixture: the first block of a.go
// reached, its second block and b.go not, and nothing of modeleval at all.
const offlineProfile = "mode: set\n" +
	fixturePkg + "/a.go:3.10,5.2 2 1\n" +
	fixturePkg + "/a.go:5.2,7.3 1 0\n" +
	fixturePkg + "/b.go:1.1,2.1 3 0\n"

// onlineProfile is the run half: the same three blocks of the fixture
// package, the first not reached this time and the second reached only here,
// and a package the offline half never measured.
const onlineProfile = "mode: set\n" +
	fixturePkg + "/a.go:3.10,5.2 2 0\n" +
	fixturePkg + "/a.go:5.2,7.3 1 1\n" +
	fixturePkg + "/b.go:1.1,2.1 3 0\n" +
	modelevalPkg + "/c.go:1.1,1.9 4 1\n"

// writeProfile writes text to a file of that name under dir and returns its
// path.
func writeProfile(t *testing.T, dir, name, text string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// readHalves writes one profile per half and reads them back merged, failing
// the test when the merge is refused.
func readHalves(t *testing.T, offline, online string) *coverage {
	t.Helper()

	dir := t.TempDir()
	cov, err := read([]source{
		{path: writeProfile(t, dir, "offline.out", offline), half: offlineHalf},
		{path: writeProfile(t, dir, "online.out", online), half: onlineHalf},
	})
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	return cov
}

// rowOf returns the row of one package, failing the test when the package
// cannot be measured.
func rowOf(t *testing.T, cov *coverage, pkg string) row {
	t.Helper()

	rows, err := cov.rows([]string{pkg})
	if err != nil {
		t.Fatalf("rows(%s) error = %v", pkg, err)
	}
	return rows[0]
}

// TestRows_OverlappingBlocks_CountEachStatementOnce holds the merge to
// counting a block both halves carry once: the fixture package has six
// statements in either half and six merged, the offline half reaches the two
// of the first block, the runs the one of the second, and together they
// reach three, one of which only the runs reached.
func TestRows_OverlappingBlocks_CountEachStatementOnce(t *testing.T) {
	got := rowOf(t, readHalves(t, offlineProfile, onlineProfile), fixturePkg)

	want := row{
		pkg:        fixturePkg,
		blocks:     3,
		present:    [halves]bool{true, true},
		halves:     [halves]figure{{statements: 6, covered: 2}, {statements: 6, covered: 1}},
		merged:     figure{statements: 6, covered: 3},
		onlyOnline: 1,
	}
	if got != want {
		t.Errorf("row = %+v, want %+v", got, want)
	}
}

// TestRows_PackageAbsentFromAHalf_IsMeasuredByTheOtherAlone holds a package
// one half never measured to the other half's figure, with the absent half
// marked rather than read as zero statements reached: modeleval is only in
// the runs' profile, and once the halves are swapped, only in the offline
// one, where none of its statements is reached by the runs alone.
func TestRows_PackageAbsentFromAHalf_IsMeasuredByTheOtherAlone(t *testing.T) {
	tests := []struct {
		name            string
		offline, online string
		want            row
	}{
		{
			name: "absent offline", offline: offlineProfile, online: onlineProfile,
			want: row{
				pkg: modelevalPkg, blocks: 1, present: [halves]bool{false, true},
				halves:     [halves]figure{{}, {statements: 4, covered: 4}},
				merged:     figure{statements: 4, covered: 4},
				onlyOnline: 4,
			},
		},
		{
			name: "absent online", offline: onlineProfile, online: offlineProfile,
			want: row{
				pkg: modelevalPkg, blocks: 1, present: [halves]bool{true, false},
				halves: [halves]figure{{statements: 4, covered: 4}, {}},
				merged: figure{statements: 4, covered: 4},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rowOf(t, readHalves(t, tc.offline, tc.online), modelevalPkg)
			if got != tc.want {
				t.Errorf("row = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestRows_PackageNoProfileCarries_IsRefused holds the report to refusing a
// package it has nothing to measure by, rather than reporting no statements
// reached out of none, and to naming the package and the reason. A package
// whose path only begins like a measured one is another package.
func TestRows_PackageNoProfileCarries_IsRefused(t *testing.T) {
	cov := readHalves(t, offlineProfile, onlineProfile)
	tests := []struct{ name, pkg string }{
		{name: "another package", pkg: "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"},
		{name: "a parent of a measured one", pkg: "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal"},
		{name: "a measured one's name continued", pkg: fixturePkg + "s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := cov.rows([]string{fixturePkg, tc.pkg})
			if err == nil {
				t.Fatalf("rows() = %+v, want a refusal", rows)
			}
			for _, want := range []string{"no profile carries " + tc.pkg, "-coverpkg"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("rows() error = %q, want it to hold %q", err, want)
				}
			}
		})
	}
}

// TestRows_EmptyFunctions_AreAPackageOfNoStatements holds a package whose
// blocks count no statements, which is what empty function bodies give, to
// being measured: present, with nothing to reach.
func TestRows_EmptyFunctions_AreAPackageOfNoStatements(t *testing.T) {
	empty := "mode: set\n" + modelevalPkg + "/c.go:3.14,3.15 0 0\n"

	got := rowOf(t, readHalves(t, empty, empty), modelevalPkg)

	want := row{pkg: modelevalPkg, blocks: 1, present: [halves]bool{true, true}}
	if got != want {
		t.Errorf("row = %+v, want %+v", got, want)
	}
}

// TestProfile_Modes_CombineCountsTheWayEachModeMeans holds the merged
// profile to its inputs' mode and counts: a counting mode the inputs share
// adds them, set mode records whether any input reached the block, and
// inputs of two modes merge in set mode. A block one profile repeats is
// folded first, by the same rule.
func TestProfile_Modes_CombineCountsTheWayEachModeMeans(t *testing.T) {
	block := fixturePkg + "/a.go:1.1,2.1 1 "
	other := fixturePkg + "/a.go:3.1,4.1 1 "
	tests := []struct {
		name            string
		offline, online string
		want            string
	}{
		{
			name:    "count adds",
			offline: "mode: count\n" + block + "3\n" + other + "0\n",
			online:  "mode: count\n" + block + "2\n" + other + "0\n",
			want:    "mode: count\n" + block + "5\n" + other + "0\n",
		},
		{
			name:    "atomic adds",
			offline: "mode: atomic\n" + block + "3\n" + other + "1\n",
			online:  "mode: atomic\n" + block + "4\n" + other + "0\n",
			want:    "mode: atomic\n" + block + "7\n" + other + "1\n",
		},
		{
			name:    "set records a reach",
			offline: "mode: set\n" + block + "1\n" + other + "0\n",
			online:  "mode: set\n" + block + "1\n" + other + "0\n",
			want:    "mode: set\n" + block + "1\n" + other + "0\n",
		},
		{
			name:    "two modes merge in set mode",
			offline: "mode: count\n" + block + "3\n" + other + "0\n",
			online:  "mode: set\n" + block + "0\n" + other + "0\n",
			want:    "mode: set\n" + block + "1\n" + other + "0\n",
		},
		{
			name:    "set mode after a counting one stays set",
			offline: "mode: set\n" + block + "0\n" + other + "1\n",
			online:  "mode: count\n" + block + "0\n" + other + "6\n",
			want:    "mode: set\n" + block + "0\n" + other + "1\n",
		},
		{
			name:    "two counting modes merge in set mode",
			offline: "mode: count\n" + block + "2\n" + other + "0\n",
			online:  "mode: atomic\n" + block + "2\n" + other + "0\n",
			want:    "mode: set\n" + block + "1\n" + other + "0\n",
		},
		{
			name:    "a repeated block folds first",
			offline: "mode: count\n" + block + "1\n" + block + "2\n" + other + "0\n",
			online:  "mode: count\n" + other + "0\n",
			want:    "mode: count\n" + block + "3\n" + other + "0\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(readHalves(t, tc.offline, tc.online).profile())
			if got != tc.want {
				t.Errorf("profile() =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// TestProfile_Fixture_WritesEveryBlockOnceInFileOrder pins the merged
// profile of the fixture: every block once, sorted by file and then by
// position, each reached when either half reached it.
func TestProfile_Fixture_WritesEveryBlockOnceInFileOrder(t *testing.T) {
	got := string(readHalves(t, onlineProfile, offlineProfile).profile())

	want := "mode: set\n" +
		fixturePkg + "/a.go:3.10,5.2 2 1\n" +
		fixturePkg + "/a.go:5.2,7.3 1 1\n" +
		fixturePkg + "/b.go:1.1,2.1 3 0\n" +
		modelevalPkg + "/c.go:1.1,1.9 4 1\n"
	if got != want {
		t.Errorf("profile() =\n%s\nwant\n%s", got, want)
	}
}

// TestRead_BlocksThatOnlyMeet_AreNotAnOverlap holds the overlap check to
// what an overlap is: a block that starts where the previous one ends shares
// no statement with it, a block that starts on the line the previous one ends
// on but past its end is beside it, and blocks of two files share nothing
// whatever their positions.
func TestRead_BlocksThatOnlyMeet_AreNotAnOverlap(t *testing.T) {
	tests := []struct{ name, offline, online string }{
		{
			name:    "end meets start",
			offline: "mode: set\n" + fixturePkg + "/a.go:3.10,5.2 2 1\n",
			online:  "mode: set\n" + fixturePkg + "/a.go:5.2,7.3 1 1\n",
		},
		{
			name:    "start past the end on its line",
			offline: "mode: set\n" + fixturePkg + "/a.go:3.10,5.2 2 1\n",
			online:  "mode: set\n" + fixturePkg + "/a.go:5.9,5.20 1 1\n",
		},
		{
			name:    "two files",
			offline: "mode: set\n" + fixturePkg + "/a.go:3.10,9.2 2 1\n",
			online:  "mode: set\n" + fixturePkg + "/b.go:4.1,5.1 1 1\n",
		},
		{
			name:    "a later file starting before the earlier file ends",
			offline: "mode: set\n" + fixturePkg + "/a.go:3.10,9.2 2 1\n",
			online:  "mode: set\n" + fixturePkg + "/b.go:1.1,2.1 1 1\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			readHalves(t, tc.offline, tc.online)
		})
	}
}

// TestRead_ProfilesThatDisagree_AreRefused holds the merge to refusing what
// two profiles of the same source can never say, and to naming the file, the
// blocks and the profiles in the refusal: the same extent counting another
// number of statements, two blocks of one file that overlap, whether across
// two profiles or within one, and a profile that cannot be read at all.
func TestRead_ProfilesThatDisagree_AreRefused(t *testing.T) {
	a := fixturePkg + "/a.go:"
	tests := []struct {
		name            string
		offline, online string
		want            []string
	}{
		{
			name:    "the same extent, another count of statements",
			offline: "mode: set\n" + a + "3.10,5.2 2 1\n",
			online:  "mode: set\n" + a + "3.10,5.2 3 0\n",
			want:    []string{fixturePkg + "/a.go: the block at 3.10,5.2 counts 2 statements in ", "offline.out and 3 in ", "online.out", "not written from the same source"},
		},
		{
			name:    "blocks that overlap across profiles",
			offline: "mode: set\n" + a + "3.10,5.2 2 1\n",
			online:  "mode: set\n" + a + "5.1,7.3 1 1\n",
			want:    []string{fixturePkg + "/a.go: the block at 3.10,5.2 in ", "offline.out and the block at 5.1,7.3 in ", "online.out overlap without being one block"},
		},
		{
			name:    "a block starting inside another on the same line",
			offline: "mode: set\n" + a + "5.2,5.30 1 1\n",
			online:  "mode: set\n" + a + "5.4,5.9 1 1\n",
			want:    []string{"the block at 5.2,5.30 in ", "the block at 5.4,5.9 in ", "overlap without being one block"},
		},
		{
			name:    "two blocks starting together",
			offline: "mode: set\n" + a + "3.10,5.2 2 1\n",
			online:  "mode: set\n" + a + "3.10,4.2 1 1\n",
			want:    []string{"the block at 3.10,4.2 in ", "the block at 3.10,5.2 in ", "overlap without being one block"},
		},
		{
			name:    "blocks that overlap within one profile",
			offline: "mode: set\n" + a + "3.10,5.2 2 1\n" + a + "4.1,6.1 1 0\n",
			online:  "mode: set\n" + a + "8.1,9.1 1 1\n",
			want:    []string{"offline.out and the block at 4.1,6.1 in ", "offline.out overlap"},
		},
		{
			name:    "a profile with no mode line",
			offline: a + "3.10,5.2 2 1\n",
			online:  onlineProfile,
			want:    []string{"profile ", "offline.out: ", "bad mode line"},
		},
		{
			name:    "a repeated block counting another number of statements",
			offline: offlineProfile,
			online:  "mode: set\n" + a + "3.10,5.2 2 0\n" + a + "3.10,5.2 3 0\n",
			want:    []string{"online.out: ", "inconsistent NumStmt"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cov, err := read([]source{
				{path: writeProfile(t, dir, "offline.out", tc.offline), half: offlineHalf},
				{path: writeProfile(t, dir, "online.out", tc.online), half: onlineHalf},
			})
			if err == nil {
				t.Fatalf("read() = %v, want a refusal", cov)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("read() error = %q, want it to hold %q", err, want)
				}
			}
		})
	}
}

// TestRead_ProfileThatIsNotThere_IsRefusedWithItsPath holds a missing file
// to a refusal naming it, rather than to a half measured by nothing.
func TestRead_ProfileThatIsNotThere_IsRefusedWithItsPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-written.out")

	cov, err := read([]source{{path: missing, half: onlineHalf}})

	if err == nil || !strings.Contains(err.Error(), "profile "+missing+": ") {
		t.Errorf("read() = %v, %v; want a refusal naming %s", cov, err, missing)
	}
}

// TestPosition_Before holds the order of two positions to line first and
// then column, with a position never before itself.
func TestPosition_Before(t *testing.T) {
	tests := []struct {
		name string
		p, q position
		want bool
	}{
		{name: "an earlier column on the same line", p: position{3, 5}, q: position{3, 6}, want: true},
		{name: "a later column on the same line", p: position{3, 6}, q: position{3, 5}},
		{name: "the same position", p: position{3, 5}, q: position{3, 5}},
		{name: "an earlier line, a later column", p: position{2, 9}, q: position{3, 1}, want: true},
		{name: "a later line, an earlier column", p: position{4, 1}, q: position{3, 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.before(tc.q); got != tc.want {
				t.Errorf("%v.before(%v) = %v, want %v", tc.p, tc.q, got, tc.want)
			}
		})
	}
}
