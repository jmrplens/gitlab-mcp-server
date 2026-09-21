package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// shardReport classifies one committed shard directory against the fixture
// catalog and publishes it, which is the report the record is built from. It
// reads the shards rather than [fixtureRuntime] because the record's whole
// provenance half -- the date, the commit, the fixture profile -- lives on a
// run line, and only the committed shards carry one.
func shardReport(t *testing.T, name string) *report {
	t.Helper()
	runtimes, err := readRuntimes(callsFixture(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if len(runtimes) != 1 {
		t.Fatalf("read %s: %d runtimes, want one", name, len(runtimes))
	}
	return buildReport(classify(runtimes[0], fixtureCatalog()))
}

// TestBuildRecordEntry_CEFixture_CarriesProvenanceAndLevels verifies that the
// entry is built off the run line rather than off the writing process: the
// date comes from the run ID's stamp, the commit, version, requirement and
// fixture profile come from the run line, and the levels are the sorted id
// lists the report settled.
func TestBuildRecordEntry_CEFixture_CarriesProvenanceAndLevels(t *testing.T) {
	entry, err := buildRecordEntry(shardReport(t, "ce"))
	if err != nil {
		t.Fatalf("buildRecordEntry() = %v, want an entry", err)
	}
	if entry.RetrievedAt != "2026-09-12" {
		t.Errorf("retrieved_at = %q, want the run ID's stamp 2026-09-12", entry.RetrievedAt)
	}
	if entry.Runtime != "community/free" || entry.Edition != "community" || entry.Tier != "free" {
		t.Errorf("runtime = %s (%s/%s), want community/free", entry.Runtime, entry.Edition, entry.Tier)
	}
	if len(entry.Runs) != 1 {
		t.Fatalf("runs = %d, want the one run line the fixture carries", len(entry.Runs))
	}
	run := entry.Runs[0]
	switch {
	case run.Commit != "deadbeef":
		t.Errorf("commit = %q, want deadbeef", run.Commit)
	case run.GitLabVersion != "18.4.0":
		t.Errorf("gitlab_version = %q, want 18.4.0", run.GitLabVersion)
	case run.RunID != "20260912t100000z-abc":
		t.Errorf("run_id = %q, want the fixture's", run.RunID)
	case !run.TierConfirmed:
		t.Error("tier_confirmed = false, want the fixture's true")
	case !run.Fixtures.FixtureService || run.Fixtures.Runner:
		t.Errorf("fixtures = %+v, want the fixture service up and no runner", run.Fixtures)
	}
	if entry.Summary.L1 != len(entry.Levels.L1) || entry.Summary.L1 == 0 {
		t.Errorf("l1 = %d and lists %d actions, want a non-zero pair that agrees", entry.Summary.L1, len(entry.Levels.L1))
	}
}

// TestBuildRecordEntry_Refusals verifies every reason a report is not
// evidence worth committing: a runtime no Makefile target produces, a package
// that refused, a package that ran under a filter, a runtime with no test
// call, run lines that name two revisions, and a run whose IDs carry no
// readable stamp.
func TestBuildRecordEntry_Refusals(t *testing.T) {
	started := func(pkg, runID, commit string) runRow {
		return runRow{Package: pkg, Status: e2ecalls.RunStarted, RunID: runID, Commit: commit}
	}
	cases := []struct {
		name string
		rep  *report
		want string
	}{
		{
			// An enterprise image with no license is neither selector: ce
			// means a community build and ee an enterprise one holding a
			// license, and no target produces this.
			name: "a runtime no target produces",
			rep: &report{Runtime: "enterprise/free", Summary: summary{TestCalls: 1}, Runs: []runRow{
				started("common", "20260912t100000z-abc", ""),
			}},
			want: "no Makefile target produces it",
		},
		{
			name: "a package refused",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1}, Runs: []runRow{
				{Package: "ce", Status: e2ecalls.RunRefused, Reason: "needs a runner", RunID: "20260912t100000z-abc"},
			}},
			want: "package ce refused to run",
		},
		{
			name: "a package ran under a filter",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1}, Runs: []runRow{
				{Package: "ce", Status: e2ecalls.RunStarted, Filter: "^TestIssue", RunID: "20260912t100000z-abc"},
			}},
			want: "a partial run is not a coverage claim",
		},
		{
			name: "no test call",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 0}, Runs: []runRow{
				started("ce", "20260912t100000z-abc", ""),
			}},
			want: "no test call was recorded",
		},
		{
			name: "two revisions",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1}, Runs: []runRow{
				started("ce", "20260912t100000z-abc", "1111111111111111111111111111111111111111"),
				started("common", "20260912t110000z-def", "2222222222222222222222222222222222222222"),
			}},
			want: "the run lines name 2 revisions",
		},
		{
			name: "no readable stamp",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1}, Runs: []runRow{
				started("ce", "r1", ""),
			}},
			want: "the entry would have no date",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := buildRecordEntry(tc.rep)
			if err == nil {
				t.Fatalf("buildRecordEntry() = %+v, want a refusal naming %q", entry, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("buildRecordEntry() = %q, want it to name %q", err, tc.want)
			}
		})
	}
}

// TestRecordDate_EarliestStamp_Wins verifies that a directory whose packages
// started minutes apart is dated by its oldest part, and that a run line with
// no ID at all is passed over rather than refusing the whole entry.
func TestRecordDate_EarliestStamp_Wins(t *testing.T) {
	got, err := recordDate(&report{Runs: []runRow{
		{RunID: "20260912t110000z-def"},
		{RunID: ""},
		{RunID: "20260911t235959z-abc"},
		{RunID: "not-a-stamp"},
		// A stamp later than the one already held, so the comparison is
		// asked both ways round and cannot be a "take the last one".
		{RunID: "20260913t000000z-ghi"},
	}})
	if err != nil || got != "2026-09-11" {
		t.Errorf("recordDate() = %q, %v; want 2026-09-11 and no error", got, err)
	}
}

// TestWriteRecord_SecondRuntime_MergesAndKeepsTheFirst verifies the property a
// one-half refresh depends on: writing ee into a document that already holds
// ce leaves the ce entry byte for byte as it was.
func TestWriteRecord_SecondRuntime_MergesAndKeepsTheFirst(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "e2e-coverage.json")
	pagePath := filepath.Join(dir, "e2e-coverage.md")

	if err := writeRecord(recordPath, pagePath, []*report{shardReport(t, "ce")}); err != nil {
		t.Fatalf("write ce: %v", err)
	}
	first := readRecordFile(t, recordPath)

	if err := writeRecord(recordPath, pagePath, []*report{eeReport()}); err != nil {
		t.Fatalf("write ee: %v", err)
	}
	second := readRecordFile(t, recordPath)

	if _, held := second.Runtimes["ee"]; !held {
		t.Fatal("the second write left no ee entry")
	}
	before, err := json.Marshal(first.Runtimes["ce"])
	if err != nil {
		t.Fatalf("encode the first ce entry: %v", err)
	}
	after, err := json.Marshal(second.Runtimes["ce"])
	if err != nil {
		t.Fatalf("encode the second ce entry: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the ce entry changed when ee was written:\n before %s\n after  %s", before, after)
	}
}

// TestWriteRecord_TwoRunsOneKey_Refused verifies that the committed figures
// cannot be replaced by a stranger's without a word.
//
// A key is an edition and a tier, so `make test-e2e-gitlab`'s self-hosted
// directory -- a developer's own instance, which the record must never hold --
// is community/free exactly as the Docker ce one is, and both answer to the ce
// selector. Pointed at the parent directory both sit under, the write used to
// take them in sorted order and leave self-hosted's figures under ce with a nil
// error. The refusal names both, which is the only way the operator can tell
// which directory to pass.
func TestWriteRecord_TwoRunsOneKey_Refused(t *testing.T) {
	docker := shardReport(t, "ce")
	developer := shardReport(t, "ce")
	developer.Directory = filepath.Join("dist", "e2e-calls", "self-hosted")

	cases := []struct {
		name  string
		reps  []*report
		wants []string
	}{
		{
			name:  "two directories of one edition",
			reps:  []*report{docker, developer},
			wants: []string{"settle to the ce entry", docker.Directory, developer.Directory},
		},
		{
			// A report built without shards has no directory to name, so the
			// refusal falls back to the runtime rather than printing an empty
			// pair of brackets.
			name:  "reports with no directory to name",
			reps:  []*report{eeReport(), eeReport()},
			wants: []string{"settle to the ee entry", "enterprise/ultimate and enterprise/ultimate"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			err := writeRecord(filepath.Join(dir, "e2e-coverage.json"), filepath.Join(dir, "page.md"), tc.reps)

			if err == nil {
				t.Fatal("writeRecord() = nil, want a refusal to file two runs under one key")
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("writeRecord() = %q, want it to name %q", err, want)
				}
			}
		})
	}
}

// TestWriteRecord_Deterministic_SameInputSameBytes verifies that the record is
// stable from one write to the next, which is what lets the page be compared
// byte for byte and a refresh produce a diff a reviewer can read.
func TestWriteRecord_Deterministic_SameInputSameBytes(t *testing.T) {
	write := func(t *testing.T) []byte {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "e2e-coverage.json")
		if err := writeRecord(path, filepath.Join(dir, "page.md"), []*report{shardReport(t, "ce"), eeReport()}); err != nil {
			t.Fatalf("write: %v", err)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- the path is this test's own temporary directory.
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		return data
	}
	if first, second := write(t), write(t); string(first) != string(second) {
		t.Error("two writes of one classification produced different bytes")
	}
}

// TestWriteRecord_NoLocalPaths verifies that nothing about the machine that
// ran the suite reaches the committed document: report.Directory is a path on
// that machine and means nothing on any other.
func TestWriteRecord_NoLocalPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e2e-coverage.json")
	rep := shardReport(t, "ce")
	if rep.Directory == "" {
		t.Fatal("the report carries no directory, so this test would prove nothing")
	}
	if err := writeRecord(path, filepath.Join(dir, "page.md"), []*report{rep}); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the path is this test's own temporary directory.
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(data), rep.Directory) {
		t.Errorf("the record names the shard directory %q", rep.Directory)
	}
}

// TestWriteRecord_UnreadableDocument_Refused verifies that a document nothing
// can parse is refused rather than replaced: replacing it would silently drop
// the half this run did not measure.
func TestWriteRecord_UnreadableDocument_Refused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e2e-coverage.json")
	writeFile(t, path, "{not json")
	err := writeRecord(path, filepath.Join(dir, "page.md"), []*report{shardReport(t, "ce")})
	if err == nil {
		t.Fatal("writeRecord() = nil, want a refusal to overwrite a document it could not read")
	}
}

// scannedStatic is a static result that ran and found the tree, which is what
// [staticApplied] admits and therefore what the record write demands.
func scannedStatic() *staticResult {
	return &staticResult{unassertedIDs: map[string]bool{}, testIDs: map[string]map[string]bool{}}
}

// TestRunRecordWrite_WithoutTheGates_Refused verifies that the record cannot
// be written from a run that classified more generously than the gates do: no
// results stream means a failed test still counts, and no static scan means a
// call site that discards its answer does too.
//
// The last two cases are the ones the flag alone could not see, and they are
// the realistic ones: -static passed, and the scan then produced nothing to
// apply because it could not load the packages (any type error under the e2e
// build tag) or because the tree has no test/e2e/gitlab in it. Both used to
// write the record from the unnarrowed classification and exit 0.
func TestRunRecordWrite_WithoutTheGates_Refused(t *testing.T) {
	cases := []struct {
		name   string
		opts   options
		static *staticResult
	}{
		{name: "no results", opts: options{static: true}, static: scannedStatic()},
		{name: "no static", opts: options{results: "log.json"}},
		{name: "the scan could not run", opts: options{results: "log.json", static: true}},
		{
			name:   "the scan found no suite to read",
			opts:   options{results: "log.json", static: true},
			static: &staticResult{Skipped: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := runRecordWrite(tc.opts, tc.static, []*report{shardReport(t, "ce")}, &stdout, &stderr); code != exitUsage {
				t.Errorf("runRecordWrite() = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr.String(), "-record needs -results and a -static scan that ran") {
				t.Errorf("stderr = %q, want it to name both preconditions", stderr.String())
			}
		})
	}
}

// TestRunRecordWrite_NoRuntimeSelected_Refused verifies that a run whose
// selectors matched nothing does not rewrite the document with nothing in it,
// which would read as a successful refresh.
func TestRunRecordWrite_NoRuntimeSelected_Refused(t *testing.T) {
	var stdout, stderr strings.Builder
	opts := options{dir: t.TempDir(), results: "log.json", static: true}
	if code := runRecordWrite(opts, scannedStatic(), nil, &stdout, &stderr); code != exitUsage {
		t.Errorf("runRecordWrite() = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "selected no runtime") {
		t.Errorf("stderr = %q, want it to say no runtime was selected", stderr.String())
	}
}

// TestRunRecordWrite_Fixture_WritesBothArtifacts verifies the whole write
// path: the JSON and the page land where the options put them, and the
// summary line names what was recorded.
func TestRunRecordWrite_Fixture_WritesBothArtifacts(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		dir: dir, results: "log.json", static: true,
		recordPath: filepath.Join(dir, "e2e-coverage.json"),
		recordPage: filepath.Join(dir, "e2e-coverage.md"),
	}
	var stdout, stderr strings.Builder
	if code := runRecordWrite(opts, scannedStatic(), []*report{shardReport(t, "ce")}, &stdout, &stderr); code != exitOK {
		t.Fatalf("runRecordWrite() = %d, stderr %q; want 0", code, stderr.String())
	}
	for _, path := range []string{opts.recordPath, opts.recordPage} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("stat %s: %v", path, err)
			}
		})
	}
	if !strings.Contains(stdout.String(), "record: ce: L1 ") {
		t.Errorf("stdout = %q, want the ce summary line", stdout.String())
	}
}

// TestRunRecordWrite_SummaryLine_NamesTheLevelsAndTheRecord verifies the
// line the write prints: the key, the three levels over the catalog, and the
// path of the record rather than of the page. The report is hand-built with
// three levels that all differ, since the ce fixture's L1 and L2 agree and
// could not tell the two figures apart.
func TestRunRecordWrite_SummaryLine_NamesTheLevelsAndTheRecord(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		dir: dir, results: "log.json", static: true,
		recordPath: filepath.Join(dir, "e2e-coverage.json"),
		recordPage: filepath.Join(dir, "e2e-coverage.md"),
	}
	rep := &report{
		Runtime: "community/free", Edition: "community", Tier: "free",
		Runs:    []runRow{{Package: "common", Status: e2ecalls.RunStarted, RunID: "20260912t100000z-abc"}},
		Summary: summary{CatalogActions: 12, TestCalls: 5, L1: 3, L2: 2, L3: 1},
		Levels:  levels{L1: []string{"issue.get", "issue.list", "project.get"}, L2: []string{"issue.list", "project.get"}, L3: []string{"issue.list"}},
	}
	var stdout, stderr strings.Builder
	if code := runRecordWrite(opts, scannedStatic(), []*report{rep}, &stdout, &stderr); code != exitOK {
		t.Fatalf("runRecordWrite() = %d, stderr %q; want 0", code, stderr.String())
	}
	if want := "record: ce: L1 3/12, L2 2, L3 1, written to " + opts.recordPath + "\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestRunRecordWrite_UnrecordableRuntime_Reported verifies that a refusal from
// the build reaches the operator instead of the process: the write is abandoned
// and its reason is printed, so a run that measured a runtime no Makefile target
// produces does not leave a half-written document behind.
func TestRunRecordWrite_UnrecordableRuntime_Reported(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		dir: dir, results: "log.json", static: true,
		recordPath: filepath.Join(dir, "e2e-coverage.json"),
		recordPage: filepath.Join(dir, "e2e-coverage.md"),
	}
	unlicensed := shardReport(t, "ce")
	unlicensed.Runtime = "enterprise/free"

	var stdout, stderr strings.Builder
	if code := runRecordWrite(opts, scannedStatic(), []*report{unlicensed}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("runRecordWrite() = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "no Makefile target produces it") {
		t.Errorf("stderr = %q, want the reason the runtime is not recordable", stderr.String())
	}
	if _, err := os.Stat(opts.recordPath); err == nil {
		t.Error("the refused write left a record behind")
	}
}

// TestRunRecordRender_MissingRecord_IsAUsageError verifies that the render
// mode refuses rather than drawing an empty page, that it redraws when the
// record is there and says which page it drew from which record, and that a
// page it cannot write is refused with the reason rather than reported drawn.
func TestRunRecordRender_MissingRecord_IsAUsageError(t *testing.T) {
	dir := t.TempDir()
	opts := options{
		dir: dir, recordPath: filepath.Join(dir, "e2e-coverage.json"),
		recordPage: filepath.Join(dir, "e2e-coverage.md"),
	}
	var stdout, stderr strings.Builder
	if code := runRecordRender(opts, &stdout, &stderr); code != exitUsage {
		t.Fatalf("runRecordRender() = %d, want %d with no record on disk", code, exitUsage)
	}

	if err := writeRecord(opts.recordPath, opts.recordPage, []*report{shardReport(t, "ce")}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Remove(opts.recordPage); err != nil {
		t.Fatalf("remove the page: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runRecordRender(opts, &stdout, &stderr); code != exitOK {
		t.Fatalf("runRecordRender() = %d, stderr %q; want 0", code, stderr.String())
	}
	if _, err := os.Stat(opts.recordPage); err != nil {
		t.Errorf("the page was not redrawn: %v", err)
	}
	if want := "record: rendered " + opts.recordPage + " from " + opts.recordPath + "\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}

	// The page under a regular file cannot be written on any filesystem.
	blocker := filepath.Join(dir, "blocker")
	writeFile(t, blocker, "not a directory\n")
	opts.recordPage = filepath.Join(blocker, "e2e-coverage.md")
	stdout.Reset()
	stderr.Reset()
	if code := runRecordRender(opts, &stdout, &stderr); code != exitUsage || !strings.HasPrefix(stderr.String(), recordErrPrefix) {
		t.Errorf("runRecordRender() = %d, stderr %q; want %d with the record prefix", code, stderr.String(), exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing said of a page that was not drawn", stdout.String())
	}
}

// TestRecordPaths_EmptyOptions_UseTheRepositoryArtifacts verifies the
// defaults, which are what every Makefile target relies on.
func TestRecordPaths_EmptyOptions_UseTheRepositoryArtifacts(t *testing.T) {
	recordPath, pagePath := recordPaths(options{dir: "/repo"})
	wantRecord := filepath.Join("/repo", filepath.FromSlash(recordRelPath))
	wantPage := filepath.Join("/repo", filepath.FromSlash(recordPageRelPath))
	if recordPath != wantRecord || pagePath != wantPage {
		t.Errorf("recordPaths() = %q, %q; want %q, %q", recordPath, pagePath, wantRecord, wantPage)
	}
	named, page := recordPaths(options{dir: "/repo", recordPath: "a.json", recordPage: "b.md"})
	if named != "a.json" || page != "b.md" {
		t.Errorf("recordPaths() = %q, %q; want the named paths", named, page)
	}
}

// readRecordFile reads a written document back.
func readRecordFile(t *testing.T, path string) *coverageRecord {
	t.Helper()
	doc, err := readRecord(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return doc
}

// eeReport is a licensed runtime's report, hand-built because the committed
// two-runtimes fixture's ee half carries a refused package and is therefore
// exactly what the record refuses to commit.
func eeReport() *report {
	return &report{
		Runtime: "enterprise/ultimate", Edition: "enterprise", Tier: "ultimate",
		Runs: []runRow{{
			Package: "ee", Requirement: "licensed", Status: e2ecalls.RunStarted,
			RunID: "20260912t110000z-def", GitLabVersion: "18.4.0-ee", TierConfirmed: true,
			Commit: "6bd82ea61e0ee0751e28b0a75954b9e4b86a8648",
		}},
		Sessions: []sessionRow{{Surface: "dynamic", Mode: modeDefault, Sessions: 1, Tools: 2}},
		Summary: summary{
			CatalogActions: 12, TestCalls: 3, L1: 1, L2: 1,
			States:       map[string]map[state]int{"dynamic": {stateAsserted: 1, stateAbsent: 11}},
			Capabilities: map[string]map[state]int{"prompts": {stateAsserted: 1}},
		},
		Levels: levels{L1: []string{"issue.list"}, L2: []string{"issue.list"}},
	}
}
