// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go covers the command end to end over profiles on disk: what it
// prints, the merged profile, the record and the summary it writes, the
// status it exits with in each mode, the command lines it refuses, and the
// package spellings it reads; and the places that hand it its profiles: the
// complete-run targets, the report, record and gate targets of the Makefile,
// test/e2e/scripts/run-docker-e2e.sh, the suite's job in
// .github/workflows/e2e.yml and the coverage job of .github/workflows/ci.yml.

package main

import (
	"bytes"
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	yaml "go.yaml.in/yaml/v3"

	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// fixtureFiles writes the two fixture profiles under a fresh directory and
// returns it with the arguments that read them for both packages.
func fixtureFiles(t *testing.T) (dir string, args []string) {
	t.Helper()

	dir = t.TempDir()
	args = []string{
		"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval",
		"-offline", writeProfile(t, dir, "offline.out", offlineProfile),
		"-online", writeProfile(t, dir, "online.out", onlineProfile),
	}
	return dir, args
}

// TestRunMain_FixtureProfiles_ReportAndWriteTheMerge runs the command over
// the fixture with every output asked for: the report on stdout, the merged
// profile at -o, and the report appended to a summary that already holds an
// earlier step's section, which stays.
func TestRunMain_FixtureProfiles_ReportAndWriteTheMerge(t *testing.T) {
	dir, args := fixtureFiles(t)
	merged := filepath.Join(dir, "merged.out")
	summary := writeProfile(t, dir, "summary.md", "### An earlier step\n\n")
	args = append(args, "-o", merged, "-summary", summary)
	var stdout, stderr bytes.Buffer

	if code := runMain(args, &stdout, &stderr); code != 0 {
		t.Fatalf("runMain() = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	if stdout.String() != fixtureReport {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), fixtureReport)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
	wantProfile := "mode: set\n" +
		fixturePkg + "/a.go:3.10,5.2 2 1\n" +
		fixturePkg + "/a.go:5.2,7.3 1 1\n" +
		fixturePkg + "/b.go:1.1,2.1 3 0\n" +
		modelevalPkg + "/c.go:1.1,1.9 4 1\n"
	if got := readFile(t, merged); got != wantProfile {
		t.Errorf("merged profile =\n%s\nwant\n%s", got, wantProfile)
	}
	if got, want := readFile(t, summary), "### An earlier step\n\n"+fixtureReport+"\n"; got != want {
		t.Errorf("summary =\n%s\nwant\n%s", got, want)
	}
}

// TestRunMain_NoOutputsAsked_PrintsTheReportAlone holds -o and -summary to
// being optional: without them the report is printed and nothing is written
// beside the profiles.
func TestRunMain_NoOutputsAsked_PrintsTheReportAlone(t *testing.T) {
	dir, args := fixtureFiles(t)
	var stdout, stderr bytes.Buffer

	if code := runMain(args, &stdout, &stderr); code != 0 {
		t.Fatalf("runMain() = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	if stdout.String() != fixtureReport {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), fixtureReport)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("the directory holds %d entries, want the two profiles alone", len(entries))
	}
}

// readFile returns the contents of a file the test expects to be there.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRunMain_Arguments_RefuseWhatTheyCannotUse holds every command line the
// command cannot use to the usage status and a reason on stderr, before any
// profile is read.
func TestRunMain_Arguments_RefuseWhatTheyCannotUse(t *testing.T) {
	both := []string{"-offline", "a.out", "-online", "b.out"}
	checking := []string{"-packages", "./x", "-offline", "a.out", "-check-record", "r.json"}
	merging := append([]string{"-packages", "./x"}, both...)
	stamping := []string{"-packages", "./x", "-stamp", "s"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "an unknown flag", args: []string{"-no-such-flag"}, want: "flag provided but not defined"},
		{name: "an argument", args: append([]string{"-packages", "./x"}, append(both, "c.out")...), want: `takes no arguments, and was given ["c.out"]`},
		{name: "no offline profile", args: []string{"-packages", "./x", "-online", "b.out"}, want: "needs at least one -offline profile"},
		{name: "no run profile", args: []string{"-packages", "./x", "-offline", "a.out"}, want: "needs at least one -online profile, which a run against GitLab writes, unless -check-record"},
		{name: "no package", args: both, want: "-packages names no package"},
		{name: "an empty package", args: append([]string{"-packages", "./x,,./y"}, both...), want: `-packages "./x,,./y" names an empty package`},
		{name: "a run profile beside a record", args: append(checking, "-online", "b.out"), want: "-check-record reads the runs' half from the record, so it takes no -online profile"},
		{name: "a record checked and written", args: append(checking, "-write-record", "w.json"), want: "-check-record judges a record and -write-record writes one; give one of them"},
		{name: "a merged profile beside a record", args: append(checking, "-o", "m.out"), want: "-o writes the merge of profiles, which -check-record does not make"},
		{name: "a freshness of another value", args: append(checking, "-freshness", "later"), want: `-freshness "later" is neither checked nor deferred`},
		{name: "a freshness without a record", args: append(merging, "-freshness", "deferred"), want: "-freshness says how -check-record judges a record that predates the tree, and -check-record is not given"},
		{name: "a stamp beside an offline profile", args: append(stamping, "-offline", "a.out"), want: "-stamp writes the stamp of the packages' sources and reads no profile"},
		{name: "a stamp beside a run profile", args: append(stamping, "-online", "b.out"), want: "-stamp writes the stamp of the packages' sources and reads no profile"},
		{name: "a stamp beside a merged profile", args: append(stamping, "-o", "m.out"), want: "-stamp writes the stamp alone, so it takes no -o"},
		{name: "a stamp beside a record written", args: append(stamping, "-write-record", "w.json"), want: "-stamp writes the stamp alone, so it takes no -write-record"},
		{name: "a stamp beside a record checked", args: append(stamping, "-check-record", "r.json"), want: "-stamp writes the stamp alone, so it takes no -check-record"},
		{name: "a stamp beside a freshness", args: append(stamping, "-freshness", "checked"), want: "-stamp writes the stamp alone, so it takes no -freshness"},
		{name: "a stamp beside a summary", args: append(stamping, "-summary", "s.md"), want: "-stamp writes the stamp alone, so it takes no -summary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runMain(tc.args, &stdout, &stderr); code != 2 {
				t.Errorf("runMain(%q) = %d, want 2", tc.args, code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr = %q, want it to hold %q", stderr.String(), tc.want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing for a command line it cannot use", stdout.String())
			}
		})
	}
}

// TestRunMain_Refusals_ExitOneWithTheReason holds every run that cannot
// report to status 1 and a reason on stderr naming the command: a profile it
// cannot read or merge, a package no profile carries, and an output it
// cannot write. A refusal before the report prints no report.
func TestRunMain_Refusals_ExitOneWithTheReason(t *testing.T) {
	tests := []struct {
		name       string
		extra      func(t *testing.T, dir string) []string
		want       string
		wantReport bool
	}{
		{
			name:  "a profile that is not there",
			extra: func(_ *testing.T, dir string) []string { return []string{"-online", filepath.Join(dir, "missing.out")} },
			want:  "missing.out: open ",
		},
		{
			name: "a profile that disagrees",
			extra: func(t *testing.T, dir string) []string {
				t.Helper()
				return []string{"-online", writeProfile(t, dir, "stale.out", "mode: set\n"+fixturePkg+"/a.go:3.10,5.2 5 1\n")}
			},
			want: "counts 2 statements in ",
		},
		{
			name: "a run profile of a version with a function this one deleted",
			extra: func(t *testing.T, dir string) []string {
				t.Helper()
				return []string{"-online", writeProfile(t, dir, "stale.out", onlineProfile+fixturePkg+"/a.go:9.1,11.2 1 1\n")}
			},
			want: "/a.go: the block at 9.1,11.2 is in ",
		},
		{
			name:  "a package no profile carries",
			extra: func(*testing.T, string) []string { return []string{"-packages", "./test/e2e/internal/harness"} },
			want:  "no profile carries github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness",
		},
		{
			name: "a merged profile it cannot write",
			extra: func(_ *testing.T, dir string) []string {
				return []string{"-o", filepath.Join(dir, "no-such-dir", "merged.out")}
			},
			want: "writing the merged profile: ",
		},
		{
			name: "a summary it cannot open",
			extra: func(_ *testing.T, dir string) []string {
				return []string{"-summary", filepath.Join(dir, "no-such-dir", "summary.md")}
			},
			want:       "appending the report to ",
			wantReport: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, args := fixtureFiles(t)
			args = append(args, tc.extra(t, dir)...)
			var stdout, stderr bytes.Buffer

			if code := runMain(args, &stdout, &stderr); code != 1 {
				t.Errorf("runMain() = %d, want 1; stderr:\n%s", code, stderr.String())
			}

			if !strings.HasPrefix(stderr.String(), "merge_coverage: ") || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr = %q, want the command's name and %q", stderr.String(), tc.want)
			}
			if got := stdout.String() == fixtureReport; got != tc.wantReport {
				t.Errorf("report printed = %v, want %v; stdout:\n%s", got, tc.wantReport, stdout.String())
			}
		})
	}
}

// TestRunMain_SummaryThatRefusesTheWrite_ExitsOne drives the failure after the
// summary opened: a write the device refuses is the error the run returns.
// /dev/full is the one file every Linux gives a test that opens and then
// refuses the write, which is a disk that filled between the two; the other
// platforms have no such file and skip.
func TestRunMain_SummaryThatRefusesTheWrite_ExitsOne(t *testing.T) {
	const devFull = "/dev/full"
	if _, err := os.Stat(devFull); err != nil {
		t.Skipf("%s is not here to refuse a write: %v", devFull, err)
	}
	_, args := fixtureFiles(t)
	var stdout, stderr bytes.Buffer

	code := runMain(append(args, "-summary", devFull), &stdout, &stderr)

	if code != 1 || !strings.Contains(stderr.String(), "appending the report to /dev/full: ") {
		t.Errorf("runMain() = %d, stderr %q; want 1 and the refused write", code, stderr.String())
	}
}

// errRefusedOutput is what refusingOutput answers every write with.
var errRefusedOutput = errors.New("the output refused the write")

// refusingOutput is a stdout that refuses every write, as a closed
// descriptor or a full disk behind a redirect does.
type refusingOutput struct{}

// Write refuses the write.
func (refusingOutput) Write([]byte) (int, error) { return 0, errRefusedOutput }

// TestRunMain_StdoutThatRefusesTheReport_ExitsOne holds both reports, the
// merge's and the gate's, to status 1 and the reason when stdout refuses
// them, rather than to a run that exits 0 having delivered nothing. The gate
// is run on the tree its record describes, which it passes otherwise.
func TestRunMain_StdoutThatRefusesTheReport_ExitsOne(t *testing.T) {
	tests := []struct {
		name string
		args func(t *testing.T) []string
	}{
		{
			name: "the merge's report",
			args: func(t *testing.T) []string {
				t.Helper()
				_, args := fixtureFiles(t)
				return args
			},
		},
		{
			name: "the gate's report",
			args: func(t *testing.T) []string {
				t.Helper()
				writeTree(t, nil)
				dir := t.TempDir()
				return []string{
					"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval",
					"-offline", writeProfile(t, dir, "offline.out", recordOffline),
					"-check-record", writeProfile(t, dir, "e2e-support-coverage.json", fixtureRecord),
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer

			code := runMain(tc.args(t), refusingOutput{}, &stderr)

			if want := "merge_coverage: printing the report: " + errRefusedOutput.Error() + "\n"; code != 1 || stderr.String() != want {
				t.Errorf("runMain() = %d, stderr %q; want 1 and %q", code, stderr.String(), want)
			}
		})
	}
}

// stampLine is one source file's line in a stamp, as sha256sum writes it:
// the SHA-256 of its text, two spaces and its path below the module root.
func stampLine(file, text string) string {
	return sum(text) + "  " + file + "\n"
}

// treeStamp is the stamp of the fixture tree with the fixture package's a.go
// holding source: that file, then modeleval's c.go, the packages in the
// order -packages names them.
func treeStamp(source string) string {
	return stampLine("test/e2e/internal/fixture/a.go", source) + stampLine("test/e2e/modeleval/c.go", modelevalSource)
}

// TestRunMain_Stamp_WritesTheStampOfTheSources runs the command the way
// run-docker-e2e.sh stamps a pass of a complete run: with -packages and
// -stamp alone it reads no profile, prints nothing, and writes the SHA-256 of
// every source file of the packages, in sha256sum's format.
func TestRunMain_Stamp_WritesTheStampOfTheSources(t *testing.T) {
	writeTree(t, nil)
	stamp := filepath.Join(t.TempDir(), "e2e-ce-coverprofile.complete.pending")
	var stdout, stderr bytes.Buffer

	code := runMain([]string{"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval", "-stamp", stamp}, &stdout, &stderr)

	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("runMain() = %d, stdout %q, stderr %q; want 0 and nothing printed", code, stdout.String(), stderr.String())
	}
	if got, want := readFile(t, stamp), treeStamp(fixtureSource); got != want {
		t.Errorf("stamp =\n%s\nwant\n%s", got, want)
	}
}

// TestRunMain_Stamp_Refusals_ExitOne holds a stamp the command cannot take,
// of a package directory that is not there, or cannot write, to status 1
// and the reason.
func TestRunMain_Stamp_Refusals_ExitOne(t *testing.T) {
	tests := []struct {
		name, packages, stamp, want string
	}{
		{
			name: "a package that is not there", packages: "./test/e2e/internal/fixture,./test/e2e/gone", stamp: "stamp",
			want: "stamping the sources of test/e2e/gone: ",
		},
		{name: "a stamp it cannot write", packages: "./test/e2e/internal/fixture", stamp: filepath.Join("no-such-dir", "stamp"), want: "writing the stamp: "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writeTree(t, nil)
			var stdout, stderr bytes.Buffer

			code := runMain([]string{"-packages", tc.packages, "-stamp", filepath.Join(t.TempDir(), tc.stamp)}, &stdout, &stderr)

			if code != 1 || !strings.HasPrefix(stderr.String(), "merge_coverage: ") || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("runMain() = %d, stderr %q; want 1 and %q", code, stderr.String(), tc.want)
			}
		})
	}
}

// TestRunMain_WriteRecord_RunProfilesNotStampedAsThisTree_AreRefused holds
// the record to the complete runs of the tree it is written on: a run
// profile with no stamp beside it, which run-docker-e2e.sh keeps only after a
// complete run, one whose stamp is of another version of the sources, here A
// edited after the run without moving a block, so the record would credit
// the old reach to the new text's hash, and one whose stamp cannot be read,
// are each refused with status 1, naming the profile, and no record is
// written.
func TestRunMain_WriteRecord_RunProfilesNotStampedAsThisTree_AreRefused(t *testing.T) {
	tests := []struct {
		name  string
		stamp func(t *testing.T, dir string)
		want  string
	}{
		{
			name:  "no stamp",
			stamp: func(*testing.T, string) {},
			want:  "online.out has no stamp beside it (online.complete), which run-docker-e2e.sh leaves only after a complete run",
		},
		{
			name: "a stamp of other sources",
			stamp: func(t *testing.T, dir string) {
				t.Helper()
				writeProfile(t, dir, "online.complete", treeStamp(fixtureSource))
			},
			want: "online.out was written from other sources than the tree holds",
		},
		{
			name: "a stamp it cannot read",
			stamp: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(dir, "online.complete"), 0o750); err != nil {
					t.Fatal(err)
				}
			},
			want: "reading the stamp of ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writeTree(t, map[string]string{"test/e2e/internal/fixture/a.go": strings.Replace(fixtureSource, "_ = 2", "_ = 3", 1)})
			dir := t.TempDir()
			tc.stamp(t, dir)
			recordPath := filepath.Join(dir, "record.json")
			args := []string{
				"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval",
				"-offline", writeProfile(t, dir, "offline.out", recordOffline),
				"-online", writeProfile(t, dir, "online.out", recordOnline),
				"-write-record", recordPath,
			}
			var stdout, stderr bytes.Buffer

			if code := runMain(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("runMain() = %d, stderr %q; want 1 and %q", code, stderr.String(), tc.want)
			}
			if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a refused record left %s behind (stat error %v)", recordPath, err)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want no report for a record refused before the merge", stdout.String())
			}
		})
	}
}

// TestRunMain_WriteRecord_WritesTheRecordBesideTheReport runs the command the
// way make e2e-support-coverage-record does, over a run profile a complete
// run of this tree stamped: the merge is reported as always, and the record
// of what the runs reached is written where -write-record says.
func TestRunMain_WriteRecord_WritesTheRecordBesideTheReport(t *testing.T) {
	writeTree(t, nil)
	setClock(t, time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC))
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "e2e-support-coverage.json")
	writeProfile(t, dir, "online.complete", treeStamp(fixtureSource))
	args := []string{
		"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval",
		"-offline", writeProfile(t, dir, "offline.out", recordOffline),
		"-online", writeProfile(t, dir, "online.out", recordOnline),
		"-write-record", recordPath,
	}
	var stdout, stderr bytes.Buffer

	if code := runMain(args, &stdout, &stderr); code != 0 {
		t.Fatalf("runMain() = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	if got := readFile(t, recordPath); got != fixtureRecord {
		t.Errorf("record =\n%s\nwant\n%s", got, fixtureRecord)
	}
	if !strings.HasPrefix(stdout.String(), reportHeading) {
		t.Errorf("stdout =\n%s\nwant the merge's report", stdout.String())
	}
}

// TestRunMain_WriteRecord_Refusals_ExitOne holds a record the command cannot
// build, because the source is not the one the profiles measured or a
// package is one the offline half does not carry, which the gate could then
// never check, or cannot write, to status 1 and the reason. The run profile
// carries the stamp of the tree as each case writes it, so what is refused
// is the case's own reason. -o is given as make e2e-support-coverage-record
// gives it: a record refused for what it would hold leaves neither the
// record nor the merged profile, and one refused only because it cannot be
// written fails after the merged profile was.
func TestRunMain_WriteRecord_Refusals_ExitOne(t *testing.T) {
	modelevalBlock := modelevalPkg + "/c.go:3.11,4.7 2 1\n"
	tests := []struct {
		name            string
		recordPath      func(dir string) string
		source          string
		offline, online string
		want            string
		mergedWritten   bool
	}{
		{
			name:       "a source the profiles did not measure",
			recordPath: func(dir string) string { return filepath.Join(dir, "record.json") },
			source:     "package fixture\n",
			want:       "no declaration of the file holds the block at 4.17,5.7",
		},
		{
			name:       "a package only the runs carry",
			recordPath: func(dir string) string { return filepath.Join(dir, "record.json") },
			offline:    strings.Replace(recordOffline, modelevalBlock, "", 1),
			online:     recordOnline + modelevalBlock,
			want:       "no offline profile carries " + modelevalPkg + ", and the gate holds the offline half alone to the record",
		},
		{
			name:          "a record it cannot write",
			recordPath:    func(dir string) string { return filepath.Join(dir, "no-such-dir", "record.json") },
			want:          "writing the record: ",
			mergedWritten: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var files map[string]string
			if tc.source != "" {
				files = map[string]string{"test/e2e/internal/fixture/a.go": tc.source}
			}
			writeTree(t, files)
			dir := t.TempDir()
			writeProfile(t, dir, "online.complete", treeStamp(cmp.Or(tc.source, fixtureSource)))
			offline, online := cmp.Or(tc.offline, recordOffline), cmp.Or(tc.online, recordOnline)
			recordPath, mergedPath := tc.recordPath(dir), filepath.Join(dir, "merged.out")
			args := []string{
				"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval",
				"-offline", writeProfile(t, dir, "offline.out", offline),
				"-online", writeProfile(t, dir, "online.out", online),
				"-o", mergedPath,
				"-write-record", recordPath,
			}
			var stdout, stderr bytes.Buffer

			if code := runMain(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("runMain() = %d, stderr %q; want 1 and %q", code, stderr.String(), tc.want)
			}
			if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a refused record left %s behind (stat error %v)", recordPath, err)
			}
			if _, err := os.Stat(mergedPath); (err == nil) != tc.mergedWritten {
				t.Errorf("merged profile written = %t, want %t (stat error %v)", err == nil, tc.mergedWritten, err)
			}
		})
	}
}

// TestRunMain_CheckRecord_ExitsByTheVerdict runs the gate the way CI's
// coverage job does: status 0 on the tree the record describes, status 1
// with the verdict on stderr and the report still printed when a package has
// more statements reached by neither half than the record allows, and
// -freshness deferred turning a record that predates the tree into a note.
func TestRunMain_CheckRecord_ExitsByTheVerdict(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		freshness []string
		want      int
	}{
		{name: "the tree the record describes", want: 0},
		{name: "a declaration changed, checked", files: aChanged, freshness: []string{"-freshness", "checked"}, want: 1},
		{name: "a declaration changed, checked by default", files: aChanged, want: 1},
		{name: "a declaration changed, deferred", files: aChanged, freshness: []string{"-freshness", "deferred"}, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writeTree(t, tc.files)
			dir := t.TempDir()
			args := append([]string{
				"-packages", "./test/e2e/internal/fixture,./test/e2e/modeleval",
				"-offline", writeProfile(t, dir, "offline.out", recordOffline),
				"-check-record", writeProfile(t, dir, "e2e-support-coverage.json", fixtureRecord),
			}, tc.freshness...)
			var stdout, stderr bytes.Buffer

			code := runMain(args, &stdout, &stderr)

			if code != tc.want {
				t.Errorf("runMain() = %d, want %d; stderr:\n%s", code, tc.want, stderr.String())
			}
			if !strings.HasPrefix(stdout.String(), checkHeading) {
				t.Errorf("stdout =\n%s\nwant the gate's report", stdout.String())
			}
			if wantStderr := map[int]string{0: "", 1: "merge_coverage: " + fixtureOver + failTail + "\n"}[tc.want]; stderr.String() != wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
			}
		})
	}
}

// TestParsePackages_Spellings holds -packages to the spellings go test's
// -coverpkg takes for a package of this module: a directory below the module
// root, with or without a trailing slash, the root itself, and an import
// path, each read as the import path a profile names its files by.
func TestParsePackages_Spellings(t *testing.T) {
	tests := []struct {
		name, list string
		want       []string
	}{
		{name: "a directory", list: "./test/e2e/modeleval", want: []string{modelevalPkg}},
		{name: "a directory with a trailing slash", list: "./test/e2e/modeleval/", want: []string{modelevalPkg}},
		{name: "the module root", list: ".", want: []string{"github.com/jmrplens/gitlab-mcp-server/v3"}},
		{name: "an import path", list: fixturePkg, want: []string{fixturePkg}},
		{name: "an import path with a trailing slash", list: fixturePkg + "/", want: []string{fixturePkg}},
		{name: "a list, in its order", list: "./test/e2e/modeleval," + fixturePkg, want: []string{modelevalPkg, fixturePkg}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePackages(tc.list)
			if err != nil {
				t.Fatalf("parsePackages(%q) error = %v", tc.list, err)
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("parsePackages(%q) = %q, want %q", tc.list, got, tc.want)
			}
		})
	}
}

// TestMain_UnknownFlag_ExitsWithTheUsageStatus holds main to handing the
// process the status runMain returns.
func TestMain_UnknownFlag_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = previousArgs, previousExit })
	got := -1
	os.Args = []string{toolName, "-no-such-flag"}
	osExit = func(code int) { got = code }

	main()

	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// repoFile returns a file of the repository, read from this package's
// directory two levels below the root.
func repoFile(t *testing.T, rel string) string {
	t.Helper()

	return readFile(t, filepath.Join("..", "..", filepath.FromSlash(rel)))
}

// recipe returns one target of the Makefile, from its rule line to the blank
// line that ends it, failing the test when the Makefile has no such target.
func recipe(t *testing.T, makefile, target string) string {
	t.Helper()

	start := strings.Index(makefile, "\n"+target+":")
	if start < 0 {
		t.Fatalf("the Makefile has no %s target", target)
	}
	rest := makefile[start+1:]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// supportCoverPkg returns the list of measured packages the Makefile spells,
// failing the test when it spells none.
func supportCoverPkg(t *testing.T, makefile string) string {
	t.Helper()

	match := regexp.MustCompile(`(?m)^E2E_SUPPORT_COVERPKG = (\S+)$`).FindStringSubmatch(makefile)
	if match == nil {
		t.Fatal("the Makefile does not spell E2E_SUPPORT_COVERPKG")
	}
	return match[1]
}

// TestMakefile_SupportPackages_AreTheTwoThisCommandMeasures holds the
// Makefile's list to the two support packages of the e2e suite, spelled the
// way -coverpkg and -packages both read it.
func TestMakefile_SupportPackages_AreTheTwoThisCommandMeasures(t *testing.T) {
	got := supportCoverPkg(t, repoFile(t, "Makefile"))

	if want := "./test/e2e/internal/fixture,./test/e2e/modeleval"; got != want {
		t.Errorf("E2E_SUPPORT_COVERPKG = %q, want %q", got, want)
	}
	pkgs, err := parsePackages(got)
	if err != nil || strings.Join(pkgs, " ") != fixturePkg+" "+modelevalPkg {
		t.Errorf("parsePackages(%q) = %q, %v; want the two support packages", got, pkgs, err)
	}
}

// TestMakefile_CompleteRunTargets_HandTheRunTheSupportPackages holds the two
// complete runs to handing run-docker-e2e.sh the list, which is what makes
// every test binary of the run measure the support packages.
func TestMakefile_CompleteRunTargets_HandTheRunTheSupportPackages(t *testing.T) {
	makefile := repoFile(t, "Makefile")
	for _, target := range []string{"test-e2e-ce", "test-e2e-ee"} {
		t.Run(target, func(t *testing.T) {
			got := recipe(t, makefile, target)
			if !strings.Contains(got, "\n\tE2E_COVERPKG=$(E2E_SUPPORT_COVERPKG) \\\n") {
				t.Errorf("the %s recipe does not hand the run E2E_COVERPKG=$(E2E_SUPPORT_COVERPKG):\n%s", target, got)
			}
			if !strings.Contains(got, "./test/e2e/scripts/run-docker-e2e.sh ") {
				t.Errorf("the %s recipe no longer runs run-docker-e2e.sh:\n%s", target, got)
			}
		})
	}
}

// supportOffline is the offline half as the Makefile spells it once, for the
// report target and the gate alike: the two packages' own tests on this tree,
// with the model evaluation's switches emptied so a .env cannot make it a
// paid run, and the on-demand sweep's emptied so one exported in the shell
// cannot make it delete anything.
const supportOffline = "\ne2e_support_offline = MODELEVAL_MODELS= MODELEVAL_PROBE= E2E_SWEEP_MIN_AGE= E2E_SWEEP_PREFIX= " +
	"go test -tags e2e -count=1 -coverpkg=$(E2E_SUPPORT_COVERPKG) -coverprofile=$(E2E_SUPPORT_OFFLINE_PROFILE) $(E2E_SUPPORT_PACKAGES)\n"

// TestMakefile_SupportCoverageTarget_MergesTheOfflineHalfWithTheRuns holds
// the report target to its three steps: it refuses a report directory no run
// left a profile in, runs the offline half on this tree, and hands this
// command both halves under the names the runner writes, with the record
// written when E2E_SUPPORT_WRITE_RECORD names one.
func TestMakefile_SupportCoverageTarget_MergesTheOfflineHalfWithTheRuns(t *testing.T) {
	makefile := repoFile(t, "Makefile")
	got := recipe(t, makefile, "e2e-support-coverage")
	tests := []struct{ name, want string }{
		{name: "the run profiles", want: "runs=($(E2E_REPORT_DIR)/*-coverprofile.out)"},
		{name: "no run profile refused", want: `if [ "$${#runs[@]}" -eq 0 ]; then`},
		{name: "the offline half", want: "\n\t  $(e2e_support_offline); \\\n"},
		{
			name: "the merge",
			want: `go run ./cmd/merge_coverage/ -packages $(E2E_SUPPORT_COVERPKG) -offline $(E2E_SUPPORT_OFFLINE_PROFILE) "$${online[@]}" ` +
				`-o $(E2E_SUPPORT_PROFILE) $(if $(E2E_SUPPORT_SUMMARY),-summary "$(E2E_SUPPORT_SUMMARY)") ` +
				`$(if $(E2E_SUPPORT_WRITE_RECORD),-write-record "$(E2E_SUPPORT_WRITE_RECORD)")'`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(got, tc.want) {
				t.Errorf("the e2e-support-coverage recipe does not hold %q:\n%s", tc.want, got)
			}
		})
	}
	if !strings.Contains(makefile, supportOffline) {
		t.Errorf("the Makefile does not spell the offline half as %q", supportOffline)
	}
	if !strings.Contains(makefile, "\nE2E_SUPPORT_PACKAGES = $(subst $(e2e_comma), ,$(E2E_SUPPORT_COVERPKG))\n") {
		t.Error("E2E_SUPPORT_PACKAGES is not the list E2E_SUPPORT_COVERPKG spells, one package per word")
	}
	if strings.HasSuffix(assignedFileName(t, makefile, "E2E_SUPPORT_OFFLINE_PROFILE"), "-coverprofile.out") ||
		strings.HasSuffix(assignedFileName(t, makefile, "E2E_SUPPORT_PROFILE"), "-coverprofile.out") {
		t.Error("an output of e2e-support-coverage is named like a run's profile, so the next merge would read it as one")
	}
}

// TestMakefile_SupportCoverageRecord_IsTheReportTargetWritingTheRecord holds
// the record target to the report target run with the committed record as
// what it writes, so the record is folded from exactly the halves the report
// shows, and the record to its one place in the tree.
func TestMakefile_SupportCoverageRecord_IsTheReportTargetWritingTheRecord(t *testing.T) {
	makefile := repoFile(t, "Makefile")

	if got, want := assignedFileName(t, makefile, "E2E_SUPPORT_RECORD"), "e2e-support-coverage.json"; got != want ||
		!strings.Contains(makefile, "\nE2E_SUPPORT_RECORD = docs/development/e2e-support-coverage.json\n") {
		t.Errorf("E2E_SUPPORT_RECORD names %q, want docs/development/e2e-support-coverage.json", got)
	}
	got := recipe(t, makefile, "e2e-support-coverage-record")
	if want := "\n\t@$(MAKE) --no-print-directory e2e-support-coverage E2E_SUPPORT_WRITE_RECORD=$(E2E_SUPPORT_RECORD)"; !strings.HasSuffix(got, want) {
		t.Errorf("the e2e-support-coverage-record recipe is\n%s\nwant it to end with %q", got, want)
	}
}

// TestMakefile_SupportCoverageGate_HoldsTheOfflineHalfToTheRecord holds the
// gate target to running the offline half and this command over it and the
// committed record, judging freshness as E2E_SUPPORT_FRESHNESS says, which is
// checked unless the caller says otherwise.
func TestMakefile_SupportCoverageGate_HoldsTheOfflineHalfToTheRecord(t *testing.T) {
	makefile := repoFile(t, "Makefile")
	got := recipe(t, makefile, "check-e2e-support-coverage")
	tests := []struct{ name, want string }{
		{name: "the offline half", want: "\n\t  $(e2e_support_offline); \\\n"},
		{
			name: "the gate",
			want: `go run ./cmd/merge_coverage/ -packages $(E2E_SUPPORT_COVERPKG) -offline $(E2E_SUPPORT_OFFLINE_PROFILE) ` +
				`-check-record $(E2E_SUPPORT_RECORD) -freshness $(E2E_SUPPORT_FRESHNESS) ` +
				`$(if $(E2E_SUPPORT_SUMMARY),-summary "$(E2E_SUPPORT_SUMMARY)")'`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(got, tc.want) {
				t.Errorf("the check-e2e-support-coverage recipe does not hold %q:\n%s", tc.want, got)
			}
		})
	}
	if !strings.Contains(makefile, "\nE2E_SUPPORT_FRESHNESS ?= checked\n") {
		t.Error("E2E_SUPPORT_FRESHNESS does not default to checked")
	}
}

// TestCIWorkflow_CoverageJob_HoldsTheSupportPackagesToTheRecord holds the
// coverage job of ci.yml, the gate that reads coverage, to running the gate
// target with the summary of its run and the freshness the workflow decides,
// so a layer below a stack's top is judged as every other committed artifact
// is there.
func TestCIWorkflow_CoverageJob_HoldsTheSupportPackagesToTheRecord(t *testing.T) {
	var workflow struct {
		Env  map[string]string `yaml:"env"`
		Jobs struct {
			Coverage e2eJob `yaml:"coverage"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(repoFile(t, ".github/workflows/ci.yml")), &workflow); err != nil {
		t.Fatal(err)
	}
	const want = `make -s check-e2e-support-coverage E2E_SUPPORT_FRESHNESS="$FRESHNESS" E2E_SUPPORT_SUMMARY="$GITHUB_STEP_SUMMARY"`
	found := false
	for _, step := range workflow.Jobs.Coverage.Steps {
		found = found || step.Run == want
	}
	if !found {
		t.Errorf("no step of the coverage job runs %s", want)
	}
	if got := workflow.Env["FRESHNESS"]; !strings.Contains(got, "'deferred' || 'checked'") {
		t.Errorf("FRESHNESS = %q, want the workflow's deferred-or-checked decision", got)
	}
}

// assignedFileName returns the file name a Makefile variable assigns, failing the
// test when the Makefile does not assign it.
func assignedFileName(t *testing.T, makefile, variable string) string {
	t.Helper()

	match := regexp.MustCompile(`(?m)^` + variable + ` = (\S+)$`).FindStringSubmatch(makefile)
	if match == nil {
		t.Fatalf("the Makefile does not assign %s", variable)
	}
	return filepath.Base(match[1])
}

// TestRunner_CoverPkg_ReachesBothPassesUnderTheNamesTheMergeReads holds
// run-docker-e2e.sh to handing go test -coverpkg and -coverprofile on the
// main pass and on the final pass, each into a profile the report target's
// pattern matches, and to removing both before anything starts, so a profile
// an earlier run left under this run's names is never merged as this run's.
func TestRunner_CoverPkg_ReachesBothPassesUnderTheNamesTheMergeReads(t *testing.T) {
	runner := repoFile(t, "test/e2e/scripts/run-docker-e2e.sh")
	mainPass := `-- -tags e2e -p 1 -count=1 ${E2E_COVERPKG:+"-coverpkg=${E2E_COVERPKG}" "-coverprofile=${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-coverprofile.out"} "$@"`
	finalPass := `-- -tags e2e -p 1 -count=1 ${E2E_COVERPKG:+"-coverpkg=${E2E_COVERPKG}" "-coverprofile=${E2E_REPORT_DIR}/${FINAL_REPORT_NAME}-coverprofile.out"} -timeout 1800s -run`
	cleared := `rm -f "${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-coverprofile.out" \` + "\n" +
		`    "${E2E_REPORT_DIR}/${FINAL_REPORT_NAME}-coverprofile.out"`
	lines := []struct{ name, text string }{
		{name: "main pass", text: mainPass},
		{name: "final pass", text: finalPass},
		{name: "cleared", text: cleared},
		{name: "trap", text: "trap teardown EXIT"},
	}
	positions := map[string]int{}
	for _, line := range lines {
		positions[line.name] = strings.Index(runner, line.text)
	}
	for _, line := range lines {
		t.Run(line.name, func(t *testing.T) {
			if positions[line.name] < 0 {
				t.Errorf("run-docker-e2e.sh does not hold the %s line %q", line.name, line.text)
			}
		})
	}
	if positions["cleared"] > positions["trap"] {
		t.Error("run-docker-e2e.sh removes the run's profiles after the stack starts, so a run that fails provisioning leaves an earlier run's standing")
	}
	if positions["main pass"] > positions["final pass"] {
		t.Error("run-docker-e2e.sh writes the final pass's profile before the main pass's")
	}
}

// TestRunner_CompleteRun_StampsEachPassUnderTheNamesTheRecordReads holds
// run-docker-e2e.sh to the stamps -write-record asks of every run profile:
// every stamp, kept or pending, removed before anything starts, so one an
// earlier run left never vouches for this run's profile; each pass of a
// complete run stamped right before it builds, under the name stampPath gives
// the pass's profile with .pending after it; and the stamps kept once the
// gate has decided the run's status, in that order. The stamp itself is
// skip-gate.sh's stamp_complete_run, which runs this command's -stamp.
func TestRunner_CompleteRun_StampsEachPassUnderTheNamesTheRecordReads(t *testing.T) {
	runner := repoFile(t, "test/e2e/scripts/run-docker-e2e.sh")
	mainStamp := stampPath("${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-coverprofile.out")
	finalStamp := stampPath("${E2E_REPORT_DIR}/${FINAL_REPORT_NAME}-coverprofile.out")
	lines := []struct{ name, text string }{
		{
			name: "cleared",
			text: `rm -f "` + mainStamp + `" \` + "\n" + `    "` + mainStamp + `.pending" \` + "\n" +
				`    "` + finalStamp + `" \` + "\n" + `    "` + finalStamp + `.pending"`,
		},
		{name: "trap", text: "trap teardown EXIT"},
		{
			name: "main pass stamped",
			text: `if [ "${E2E_GATE_SKIPS:-false}" = "true" ] && [ -n "${E2E_COVERPKG:-}" ]; then` + "\n" +
				`    stamp_complete_run "${REPO_ROOT}" "${E2E_COVERPKG}" "` + mainStamp + `"`,
		},
		{name: "main pass", text: `"-coverprofile=${E2E_REPORT_DIR}/${E2E_REPORT_NAME}-coverprofile.out"} "$@"`},
		{name: "final pass stamped", text: `stamp_complete_run "${REPO_ROOT}" "${E2E_COVERPKG}" "` + finalStamp + `"`},
		{name: "final pass", text: `"-coverprofile=${E2E_REPORT_DIR}/${FINAL_REPORT_NAME}-coverprofile.out"} -timeout 1800s -run`},
		{name: "gate", text: `RUN_STATUS="$(run_status_after_gate "${RUN_STATUS}" "${GATE_STATUS}")"`},
		{name: "stamps kept", text: `keep_complete_run_stamps "${RUN_STATUS}" "` + mainStamp + `" \` + "\n" + `    "` + finalStamp + `"`},
	}
	positions := make([]int, len(lines))
	for i, line := range lines {
		positions[i] = strings.Index(runner, line.text)
		t.Run(line.name, func(t *testing.T) {
			if positions[i] < 0 {
				t.Errorf("run-docker-e2e.sh does not hold the %s line %q", line.name, line.text)
			}
		})
	}
	for i := 1; i < len(lines); i++ {
		if positions[i-1] > positions[i] {
			t.Errorf("run-docker-e2e.sh has the %s line after the %s line", lines[i-1].name, lines[i].name)
		}
	}
	if want := `go run ./cmd/merge_coverage/ -packages "$2" -stamp "$3.pending"`; !strings.Contains(repoFile(t, "test/e2e/scripts/skip-gate.sh"), want) {
		t.Errorf("skip-gate.sh does not stamp a pass with %s", want)
	}
}

// e2eJob is the suite's job of .github/workflows/e2e.yml as the contract
// below reads it.
type e2eJob struct {
	Steps []struct {
		Name string            `yaml:"name"`
		Uses string            `yaml:"uses"`
		With map[string]string `yaml:"with"`
		Run  string            `yaml:"run"`
		Env  map[string]string `yaml:"env"`
	} `yaml:"steps"`
}

// TestE2EWorkflow_SuiteJob_MeasuresTheSupportPackagesAndPublishesTheMerge
// holds the job that runs the suite on every push to the same measurement the
// complete-run targets make: the run handed the Makefile's list, the report
// target run on its reports with the table appended to the run's summary,
// and the profiles uploaded under a name of their own by an action pinned to
// a commit.
func TestE2EWorkflow_SuiteJob_MeasuresTheSupportPackagesAndPublishesTheMerge(t *testing.T) {
	var workflow struct {
		Jobs map[string]e2eJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(repoFile(t, ".github/workflows/e2e.yml")), &workflow); err != nil {
		t.Fatal(err)
	}
	job, ok := workflow.Jobs["e2e-gitlab"]
	if !ok {
		t.Fatal("e2e.yml has no e2e-gitlab job")
	}
	want := supportCoverPkg(t, repoFile(t, "Makefile"))
	pinned := regexp.MustCompile(`^actions/upload-artifact@[0-9a-f]{40}$`)
	handed, merged, uploaded := false, false, false
	for _, step := range job.Steps {
		handed = handed || strings.Contains(step.Run, "run-docker-e2e.sh ce") && step.Env["E2E_COVERPKG"] == want
		merged = merged || step.Run == `make -s e2e-support-coverage E2E_REPORT_DIR=test-results E2E_SUPPORT_SUMMARY="$GITHUB_STEP_SUMMARY"`
		uploaded = uploaded || pinned.MatchString(step.Uses) && step.With["name"] == "e2e-support-coverage" &&
			strings.Contains(step.With["path"], "test-results/*-coverprofile.out") &&
			strings.Contains(step.With["path"], "test-results/e2e-support-offline.out") &&
			strings.Contains(step.With["path"], "test-results/e2e-support-coverage.out")
	}
	if !handed {
		t.Errorf("the step that runs the suite does not hand it E2E_COVERPKG=%s", want)
	}
	if !merged {
		t.Error("no step runs make -s e2e-support-coverage on test-results with the run's summary")
	}
	if !uploaded {
		t.Error("no step uploads the run, offline and merged profiles as e2e-support-coverage with upload-artifact pinned to a commit")
	}
}
