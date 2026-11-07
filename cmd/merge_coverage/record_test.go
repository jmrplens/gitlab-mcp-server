// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// record_test.go covers the committed record of what the complete runs
// reached: the record written from a merge (the packages with their
// statements and the ones neither half reached, the declarations the runs
// reached into, keyed and hashed, with their blocks relative to the
// declaration), the source it refuses to describe, the record read back,
// the records it refuses to read, and the runs' reach credited to the
// declarations whose text is still what the runs measured.

package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The tree the record tests stand on: a.go of the fixture package, three
// declarations of which the runs reach two, and c.go of modeleval.
const (
	fixtureSource = "package fixture\n" +
		"\n" +
		"// A is reached in part.\n" +
		"func A(y bool) {\n" +
		"\tif y {\n" +
		"\t\t_ = 1\n" +
		"\t}\n" +
		"\t_ = 2\n" +
		"}\n" +
		"\n" +
		"var b = func() int { return 3 }\n" +
		"\n" +
		"func C() {\n" +
		"\t_ = 4\n" +
		"}\n"
	modelevalSource = "package modeleval\n\nfunc M() {\n\t_ = 5\n}\n"
)

// The texts the two declarations the runs reach are hashed over.
const (
	textOfA = "func A(y bool) {\n\tif y {\n\t\t_ = 1\n\t}\n\t_ = 2\n}"
	textOfB = "var b = func() int { return 3 }"
)

// recordOffline is the offline half over the tree: the first and third
// blocks of A, nothing else of the fixture package, and all of modeleval.
const recordOffline = "mode: set\n" +
	fixturePkg + "/a.go:4.17,5.7 1 1\n" +
	fixturePkg + "/a.go:5.7,7.3 1 0\n" +
	fixturePkg + "/a.go:7.3,8.8 1 1\n" +
	fixturePkg + "/a.go:11.21,11.31 1 0\n" +
	fixturePkg + "/a.go:13.11,14.7 1 0\n" +
	modelevalPkg + "/c.go:3.11,4.7 2 1\n"

// recordOnline is the run half: the first two blocks of A and the function
// literal of b, never C, nothing of modeleval, and a block of a package the
// record is not about.
const recordOnline = "mode: set\n" +
	fixturePkg + "/a.go:4.17,5.7 1 1\n" +
	fixturePkg + "/a.go:5.7,7.3 1 1\n" +
	fixturePkg + "/a.go:7.3,8.8 1 0\n" +
	fixturePkg + "/a.go:11.21,11.31 1 1\n" +
	fixturePkg + "/a.go:13.11,14.7 1 0\n" +
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness/h.go:1.1,2.1 1 1\n"

// fixtureRecord is the record of the fixture tree, written on 2026-11-19 UTC.
var fixtureRecord = fmt.Sprintf(`{
  "schema": 1,
  "source": {
    "recorded": "2026-11-19",
    "runs": [
      "online.out"
    ]
  },
  "packages": [
    {
      "package": "test/e2e/internal/fixture",
      "measured_by_runs": true,
      "statements": 5,
      "unreached": 1
    },
    {
      "package": "test/e2e/modeleval",
      "measured_by_runs": false,
      "statements": 2,
      "unreached": 0
    }
  ],
  "declarations": [
    {
      "file": "test/e2e/internal/fixture/a.go",
      "declaration": "func A",
      "sha256": "%s",
      "reached": [
        "0.17,1.7 1",
        "1.7,3.3 1"
      ]
    },
    {
      "file": "test/e2e/internal/fixture/a.go",
      "declaration": "var b",
      "sha256": "%s",
      "reached": [
        "0.21,0.31 1"
      ]
    }
  ]
}
`, sum(textOfA), sum(textOfB))

// recordPackages is the -packages list every record test measures.
var recordPackages = []string{fixturePkg, modelevalPkg}

// writeTree writes the fixture tree, with any file replaced or added by
// files, under a fresh directory, makes it the working directory the
// sources are read from, and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()

	tree := map[string]string{
		"test/e2e/internal/fixture/a.go": fixtureSource,
		"test/e2e/modeleval/c.go":        modelevalSource,
	}
	maps.Copy(tree, files)
	root := t.TempDir()
	for name, text := range tree {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		writeProfile(t, filepath.Dir(path), filepath.Base(path), text)
	}
	t.Chdir(root)
	return root
}

// setClock makes now return at for the rest of the test.
func setClock(t *testing.T, at time.Time) {
	t.Helper()

	previous := now
	t.Cleanup(func() { now = previous })
	now = func() time.Time { return at }
}

// TestNewRecord_FixtureTree_RecordsWhatTheRunsReached pins the whole record
// of the fixture tree: the day it was taken in UTC, the run profile it was
// folded from, each package with its statements, the ones
// neither half reached and whether the runs measured it at all, and the two
// declarations the runs reached into, sorted by key, each with its hash and
// the blocks the runs reached relative to its first line. C, which no half
// reached, and the block of a package the record is not about are not in it.
func TestNewRecord_FixtureTree_RecordsWhatTheRunsReached(t *testing.T) {
	writeTree(t, nil)
	setClock(t, time.Date(2026, 11, 18, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60)))
	cov := readHalves(t, recordOffline, recordOnline)
	rows, err := cov.rows(recordPackages)
	if err != nil {
		t.Fatal(err)
	}
	sources := []source{{path: "/tmp/offline.out", half: offlineHalf}, {path: "/tmp/b/online.out", half: onlineHalf}}

	rec, err := cov.newRecord(rows, sources)
	if err != nil {
		t.Fatalf("newRecord() error = %v", err)
	}

	if got := string(rec.encode()); got != fixtureRecord {
		t.Errorf("record =\n%s\nwant\n%s", got, fixtureRecord)
	}
}

// TestNewRecord_SourceItCannotDescribe_IsRefused holds the writer to the
// source on disk being the one the profiles measured: a block of a reached
// declaration that no declaration of the file holds, a file that does not
// parse, and a file that is not there are each refused, naming the file.
func TestNewRecord_SourceItCannotDescribe_IsRefused(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		gone  bool
		want  string
	}{
		{
			name:  "a block outside every declaration",
			files: map[string]string{"test/e2e/internal/fixture/a.go": strings.Replace(fixtureSource, "func A(y bool) {", "\n\n\nfunc A(y bool) {", 1)},
			want:  fixturePkg + "/a.go: no declaration of the file holds the block at 4.17,5.7",
		},
		{
			name:  "a file that does not parse",
			files: map[string]string{"test/e2e/internal/fixture/a.go": "package fixture\n\nfunc {\n"},
			want:  "test/e2e/internal/fixture/a.go:3:",
		},
		{name: "a file that is not there", gone: true, want: "reading the source the profiles measured: "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, tc.files)
			if tc.gone {
				if err := os.Remove(filepath.Join(root, "test", "e2e", "internal", "fixture", "a.go")); err != nil {
					t.Fatal(err)
				}
			}
			cov := readHalves(t, recordOffline, recordOnline)
			rows, err := cov.rows(recordPackages)
			if err != nil {
				t.Fatal(err)
			}

			_, err = cov.newRecord(rows, nil)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("newRecord() error = %v, want it to hold %q", err, tc.want)
			}
		})
	}
}

// TestLoadRecord_Fixture_ReadsBackWhatWasWritten holds the reader to the
// writer: the fixture record read from disk encodes back to the same bytes.
func TestLoadRecord_Fixture_ReadsBackWhatWasWritten(t *testing.T) {
	path := writeProfile(t, t.TempDir(), "record.json", fixtureRecord)

	rec, found, err := loadRecord(path)

	if err != nil || !found {
		t.Fatalf("loadRecord() = found %v, error %v; want the record", found, err)
	}
	if got := string(rec.encode()); got != fixtureRecord {
		t.Errorf("record read back =\n%s\nwant\n%s", got, fixtureRecord)
	}
}

// TestLoadRecord_NoFile_IsNotFoundRatherThanAnError holds a record that does
// not exist yet, which is the state before the first complete run is
// recorded, apart from one that cannot be read.
func TestLoadRecord_NoFile_IsNotFoundRatherThanAnError(t *testing.T) {
	rec, found, err := loadRecord(filepath.Join(t.TempDir(), "e2e-support-coverage.json"))

	if rec != nil || found || err != nil {
		t.Errorf("loadRecord() = %v, %v, %v; want nothing found and no error", rec, found, err)
	}
}

// TestLoadRecord_WhiteSpaceAfterTheRecord_IsReadAsTheRecord holds the reader
// to taking blank lines after the record, which an editor may leave, as the
// record alone.
func TestLoadRecord_WhiteSpaceAfterTheRecord_IsReadAsTheRecord(t *testing.T) {
	path := writeProfile(t, t.TempDir(), "record.json", fixtureRecord+"\n \t\r\n")

	rec, found, err := loadRecord(path)

	if err != nil || !found {
		t.Fatalf("loadRecord() = found %v, error %v; want the record", found, err)
	}
	if got := string(rec.encode()); got != fixtureRecord {
		t.Errorf("record read back =\n%s\nwant\n%s", got, fixtureRecord)
	}
}

// TestLoadRecord_RecordsItCannotUse_AreRefused holds every record the reader
// cannot trust to an error naming the record: a path it cannot read, text
// that is not JSON, a field the layout does not have, another schema, and
// text after the record, a second record or anything else, which the reader
// would otherwise leave unread while the gate held the packages to the first.
func TestLoadRecord_RecordsItCannotUse_AreRefused(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{name: "not JSON", text: "{", want: "unexpected EOF"},
		{name: "an unknown field", text: `{"schema": 1, "floors": {}}`, want: `unknown field "floors"`},
		{name: "another schema", text: `{"schema": 2}`, want: "is schema 2, and this command reads schema 1"},
		{name: "a second record after it", text: fixtureRecord + fixtureRecord, want: "text follows the record"},
		{name: "other text after it", text: fixtureRecord + "x\n", want: "text follows the record"},
		{name: "a closing brace after it", text: fixtureRecord + "}\n", want: "text follows the record"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProfile(t, t.TempDir(), "record.json", tc.text)

			_, _, err := loadRecord(path)

			if err == nil || !strings.Contains(err.Error(), "record "+path+": ") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("loadRecord() error = %v, want it to name the record and hold %q", err, tc.want)
			}
		})
	}
	t.Run("a path it cannot read", func(t *testing.T) {
		dir := t.TempDir()

		_, found, err := loadRecord(dir)

		if found || err == nil || !strings.Contains(err.Error(), "record "+dir+": ") {
			t.Errorf("loadRecord(a directory) = found %v, error %v; want a refusal naming it", found, err)
		}
	})
}

// TestWriteRecord_Fixture_WritesTheEncodedRecord holds the file written to the
// encoding pinned above, and a path it cannot write to an error.
func TestWriteRecord_Fixture_WritesTheEncodedRecord(t *testing.T) {
	var rec record
	if err := json.Unmarshal([]byte(fixtureRecord), &rec); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")

	if err := writeRecord(path, &rec); err != nil {
		t.Fatalf("writeRecord() error = %v", err)
	}

	if got := readFile(t, path); got != fixtureRecord {
		t.Errorf("record written =\n%s\nwant\n%s", got, fixtureRecord)
	}
	if err := writeRecord(filepath.Join(dir, "no-such-dir", "record.json"), &rec); err == nil ||
		!strings.Contains(err.Error(), "writing the record: ") {
		t.Errorf("writeRecord(an unwritable path) error = %v, want the refused write", err)
	}
}

// TestRecord_Matches_ThePackagesAskedFor holds a record to the packages the
// gate measures, in their order: the same list matches, and a list it does
// not record, or the same packages in another order, are refused naming both.
func TestRecord_Matches_ThePackagesAskedFor(t *testing.T) {
	rec := fixtureRecordValue(t)
	tests := []struct {
		name     string
		packages []string
		want     string
	}{
		{name: "the same list", packages: recordPackages},
		{
			name: "another list", packages: []string{fixturePkg},
			want: "the record measures [test/e2e/internal/fixture test/e2e/modeleval], and the gate was asked for [test/e2e/internal/fixture]",
		},
		{
			name: "another order", packages: []string{modelevalPkg, fixturePkg},
			want: "the gate was asked for [test/e2e/modeleval test/e2e/internal/fixture]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := rec.matches(tc.packages)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Errorf("matches() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// fixtureRecordValue returns the fixture record decoded.
func fixtureRecordValue(t *testing.T) *record {
	t.Helper()

	var rec record
	if err := json.Unmarshal([]byte(fixtureRecord), &rec); err != nil {
		t.Fatal(err)
	}
	return &rec
}

// creditFixture reads the offline half alone, credits the fixture record, as
// edited, to it on the tree, as edited, and returns the merge with what was
// dropped.
func creditFixture(t *testing.T, files map[string]string, offline string, edit func(*record)) (*coverage, []dropped) {
	t.Helper()

	writeTree(t, files)
	cov, err := read([]source{{path: writeProfile(t, t.TempDir(), "offline.out", offline), half: offlineHalf}})
	if err != nil {
		t.Fatal(err)
	}
	rec := fixtureRecordValue(t)
	if edit != nil {
		edit(rec)
	}
	gone, err := cov.credit(rec)
	if err != nil {
		t.Fatalf("credit() error = %v", err)
	}
	return cov, gone
}

// TestCredit_UnchangedTree_CountsWhatTheRunsReached holds the credit on the
// tree the record was taken from to the merge the record was written from:
// the runs reach three of the fixture package's five statements, two of them
// only the runs, nothing is dropped, and modeleval, which the runs never
// measured, reads absent on their side.
func TestCredit_UnchangedTree_CountsWhatTheRunsReached(t *testing.T) {
	cov, gone := creditFixture(t, nil, recordOffline, nil)

	if len(gone) != 0 {
		t.Errorf("dropped = %+v, want nothing", gone)
	}
	rows, err := cov.rows(recordPackages)
	if err != nil {
		t.Fatal(err)
	}
	want := []row{
		{
			pkg: fixturePkg, blocks: 5, present: [halves]bool{true, true},
			halves: [halves]figure{{statements: 5, covered: 2}, {statements: 5, covered: 3}},
			merged: figure{statements: 5, covered: 4}, onlyOnline: 2,
		},
		{
			pkg: modelevalPkg, blocks: 1, present: [halves]bool{true, false},
			halves: [halves]figure{{statements: 2, covered: 2}, {}},
			merged: figure{statements: 2, covered: 2},
		},
	}
	if !slices.Equal(rows, want) {
		t.Errorf("rows =\n%+v\nwant\n%+v", rows, want)
	}
}

// TestCredit_DeclarationsTheRecordNoLongerDescribes_AreDropped holds every
// way a recorded declaration stops describing the tree to dropping its reach
// alone, with the reason, while the other declaration keeps its own: its
// text changed, the file no longer declares it, the offline tests no longer
// measure its file, and its blocks are not the ones this build instruments.
func TestCredit_DeclarationsTheRecordNoLongerDescribes_AreDropped(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		offline string
		edit    func(*record)
		want    dropped
	}{
		{
			name:  "its text changed",
			files: map[string]string{"test/e2e/internal/fixture/a.go": strings.Replace(fixtureSource, "_ = 2", "_ = 3", 1)},
			want:  dropped{file: "test/e2e/internal/fixture/a.go", declaration: "func A", why: whyChanged},
		},
		{
			name:  "the file no longer declares it",
			files: map[string]string{"test/e2e/internal/fixture/a.go": strings.Replace(fixtureSource, "func A(", "func Z(", 1)},
			want:  dropped{file: "test/e2e/internal/fixture/a.go", declaration: "func A", why: whyGone},
		},
		{
			name: "the offline tests no longer measure its file",
			edit: func(r *record) { r.Declarations[0].File = "test/e2e/internal/fixture/gone.go" },
			want: dropped{file: "test/e2e/internal/fixture/gone.go", declaration: "func A", why: whyUnmeasured},
		},
		{
			name:    "its blocks are not this build's",
			offline: strings.Replace(recordOffline, "a.go:5.7,7.3 1 0", "a.go:5.7,7.3 2 0", 1),
			want:    dropped{file: "test/e2e/internal/fixture/a.go", declaration: "func A", why: whyInstrumented},
		},
		{
			name:    "a block this build does not instrument",
			offline: strings.Replace(recordOffline, "a.go:5.7,7.3 1 0", "a.go:5.8,7.3 1 0", 1),
			want:    dropped{file: "test/e2e/internal/fixture/a.go", declaration: "func A", why: whyInstrumented},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			offline := tc.offline
			if offline == "" {
				offline = recordOffline
			}
			cov, gone := creditFixture(t, tc.files, offline, tc.edit)

			if len(gone) != 1 || gone[0] != tc.want {
				t.Errorf("dropped = %+v, want [%+v]", gone, tc.want)
			}
			if !cov.blocks[span{file: fixturePkg + "/a.go", start: position{11, 21}, end: position{11, 31}}].hit[onlineHalf] {
				t.Error("b's block lost the runs' reach, which only A's should have")
			}
			for s, b := range cov.blocks {
				if s.start.line < 11 && b.hit[onlineHalf] {
					t.Errorf("the block at %s kept the runs' reach of a dropped declaration", s.extent())
				}
			}
		})
	}
}

// TestCredit_RecordItCannotApply_IsRefused holds a record whose reach cannot
// be read, or whose file cannot be read, to an error rather than to a merge
// that leaves part of it out: a reached block spelled any other way than the
// writer spells it, and a recorded file the offline tests measure that is
// not on disk.
func TestCredit_RecordItCannotApply_IsRefused(t *testing.T) {
	tests := []struct {
		name    string
		reached string
		gone    bool
		want    string
	}{
		{name: "not an extent", reached: "x", want: `test/e2e/internal/fixture/a.go func A: the reached block "x" is not spelled as the record writes one`},
		{name: "a signed line", reached: "+0.17,1.7 1", want: `the reached block "+0.17,1.7 1" is not spelled`},
		{name: "text after it", reached: "0.17,1.7 1 x", want: `the reached block "0.17,1.7 1 x" is not spelled`},
		{name: "a file that is not there", gone: true, want: "reading the source the profiles measured: "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, nil)
			if tc.gone {
				if err := os.Remove(filepath.Join(root, "test", "e2e", "internal", "fixture", "a.go")); err != nil {
					t.Fatal(err)
				}
			}
			cov, err := read([]source{{path: writeProfile(t, t.TempDir(), "offline.out", recordOffline), half: offlineHalf}})
			if err != nil {
				t.Fatal(err)
			}
			rec := fixtureRecordValue(t)
			if tc.reached != "" {
				rec.Declarations[0].Reached[1] = tc.reached
			}

			_, err = cov.credit(rec)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("credit() error = %v, want it to hold %q", err, tc.want)
			}
		})
	}
}
