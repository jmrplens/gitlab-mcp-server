package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// checkClock is the instant every check test judges a date against, so the
// window is exercised rather than the day the suite happens to run.
var checkClock = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

// checkFixture writes a record holding both runtimes and renders its page,
// and returns the options and paths the check reads them through. It is the
// state a green CI run is in, so every test below starts from a passing
// record and breaks exactly one thing.
func checkFixture(t *testing.T) (opts options, doc *coverageRecord, pagePath string) {
	t.Helper()
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "e2e-coverage.json")
	pagePath = filepath.Join(dir, "e2e-coverage.md")
	ce := shardReport(t, "ce")
	// The fixture's run ID is stamped two days before the clock, which keeps
	// the record inside the window without pinning the test to a real date.
	if err := writeRecord(recordPath, pagePath, []*report{ce, eeReport()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	doc, err := readRecord(recordPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return options{
		dir: dir, recordPath: recordPath, recordPage: pagePath,
		catalogs: func(edition.Tier) (*servedCatalog, error) { return fixtureCatalog(), nil },
		// No probe by default: whether a revision is reachable from HEAD is
		// about the repository the test runs in, which is not what any of
		// these assertions is about.
		now: func() time.Time { return checkClock },
	}, doc, pagePath
}

// silentProbe answers every revision question with "git could not be run",
// which is the branch a shallow CI checkout is in and the one that must stay
// silent.
func silentProbe(t *testing.T) {
	t.Helper()
	previous := gitProbe
	gitProbe = func(string, ...string) (bool, error) { return false, errors.New("no git here") }
	t.Cleanup(func() { gitProbe = previous })
}

// TestCheckRecord_Current_Passes verifies that a record written from this
// tree, against this tree's catalog, produces no findings and no notes.
func TestCheckRecord_Current_Passes(t *testing.T) {
	silentProbe(t)
	opts, doc, pagePath := checkFixture(t)
	verdict, err := checkRecord(opts, doc, pagePath, checkClock)
	if err != nil {
		t.Fatalf("checkRecord() = %v, want a verdict", err)
	}
	if len(verdict.Findings) > 0 || len(verdict.Notes) > 0 {
		t.Errorf("checkRecord() = findings %q, notes %q; want neither", verdict.Findings, verdict.Notes)
	}
}

// TestCheckRecord_Failures verifies every way the committed record can
// contradict the tree it sits in. Each case starts from a passing record and
// breaks one thing, so a finding here is about that one thing.
func TestCheckRecord_Failures(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(doc *coverageRecord)
		want    string
	}{
		{
			name:    "a runtime is missing",
			corrupt: func(doc *coverageRecord) { delete(doc.Runtimes, "ee") },
			want:    "the record holds no ee runtime",
		},
		{
			name:    "a runtime no target produces",
			corrupt: func(doc *coverageRecord) { doc.Runtimes["self-hosted"] = doc.Runtimes["ce"] },
			want:    `an entry keyed "self-hosted"`,
		},
		{
			name:    "an empty entry",
			corrupt: func(doc *coverageRecord) { doc.Runtimes["ee"] = nil },
			want:    "the ee entry is empty",
		},
		{
			name:    "an entry under the wrong key",
			corrupt: func(doc *coverageRecord) { doc.Runtimes["ee"].Runtime = "community/free" },
			want:    "the ee entry was measured on community/free",
		},
		{
			name:    "a tier this server does not know",
			corrupt: func(doc *coverageRecord) { doc.Runtimes["ee"].Tier = "diamond" },
			want:    `ee names the tier "diamond"`,
		},
		{
			name:    "the summary was edited without the list",
			corrupt: func(doc *coverageRecord) { doc.Runtimes["ce"].Summary.L1++ },
			want:    "the summary was edited without the list",
		},
		{
			name: "more asserted than the catalog holds",
			corrupt: func(doc *coverageRecord) {
				doc.Runtimes["ee"].Summary.CatalogActions = 0
			},
			want: "which is more actions than the catalog has",
		},
		{
			name: "a package refused",
			corrupt: func(doc *coverageRecord) {
				doc.Runtimes["ce"].Runs[0].Status = "refused"
				doc.Runtimes["ce"].Runs[0].Reason = "needs a runner"
			},
			want: "refused to run",
		},
		{
			name:    "the record was never measured",
			corrupt: func(doc *coverageRecord) { doc.Runtimes["ce"].Summary.TestCalls = 0 },
			want:    "no test call was recorded",
		},
		{
			name:    "the schema is not this one",
			corrupt: func(doc *coverageRecord) { doc.SchemaVersion = recordSchemaVersion + 1 },
			want:    "regenerate it with " + recordRegenerate,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			silentProbe(t)
			opts, doc, pagePath := checkFixture(t)
			tc.corrupt(doc)
			verdict, err := checkRecord(opts, doc, pagePath, checkClock)
			if err != nil {
				t.Fatalf("checkRecord() = %v, want a verdict", err)
			}
			if !strings.Contains(strings.Join(verdict.Findings, "\n"), tc.want) {
				t.Errorf("findings = %q, want one naming %q", verdict.Findings, tc.want)
			}
		})
	}
}

// TestCheckRecord_BelowAssertedFloor_Fails verifies that the ratchet the live
// -check applies is re-applied to the committed evidence, read through the
// same package-level table.
func TestCheckRecord_BelowAssertedFloor_Fails(t *testing.T) {
	silentProbe(t)
	opts, doc, pagePath := checkFixture(t)
	// Seeded after the record is written: the same floor applies at write
	// time, so a record under it could not have been committed in the first
	// place, and what is being tested here is that the committed evidence is
	// judged again on every run.
	t.Cleanup(func() { assertedFloors = map[string]int{} })
	assertedFloors = map[string]int{"ee": 500}
	verdict, err := checkRecord(opts, doc, pagePath, checkClock)
	if err != nil {
		t.Fatalf("checkRecord() = %v, want a verdict", err)
	}
	if !strings.Contains(strings.Join(verdict.Findings, "\n"), "below the floor of 500") {
		t.Errorf("findings = %q, want the floor named", verdict.Findings)
	}
}

// TestCheckRecord_StalePage_Fails verifies that the page cannot drift from
// the record: it is compared byte for byte with a fresh rendering.
func TestCheckRecord_StalePage_Fails(t *testing.T) {
	silentProbe(t)
	opts, doc, pagePath := checkFixture(t)
	writeFile(t, pagePath, "# E2E Coverage\n\nhand-edited\n")
	verdict, err := checkRecord(opts, doc, pagePath, checkClock)
	if err != nil {
		t.Fatalf("checkRecord() = %v, want a verdict", err)
	}
	if !strings.Contains(strings.Join(verdict.Findings, "\n"), "is stale; run "+pageRegenerate) {
		t.Errorf("findings = %q, want the page reported stale", verdict.Findings)
	}
}

// TestCheckRecord_DateProblems verifies the three ways a date stops the
// record being one a gate can rest on, and that the window is this record's
// own: 179 days is inside provenance.MaxAge and outside this one, which is
// the regression test for anyone who later folds the two together.
func TestCheckRecord_DateProblems(t *testing.T) {
	cases := []struct {
		name string
		date string
		want string
	}{
		{name: "not a date", date: "yesterday", want: "which is not a date"},
		{name: "has not happened", date: "2026-09-15", want: "which has not happened yet"},
		{name: "past this record's window", date: "2026-06-14", want: "is 92 days old and the window is 90"},
		{
			name: "past this window and inside the shared one",
			date: "2026-03-19",
			want: "is 179 days old and the window is 90",
		},
		{name: "inside the window", date: "2026-08-14", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := recordDateProblems("ce", tc.date, checkClock)
			joined := strings.Join(problems, "\n")
			switch {
			case tc.want == "" && len(problems) > 0:
				t.Errorf("recordDateProblems() = %q, want none", problems)
			case tc.want != "" && len(problems) != 1:
				t.Errorf("recordDateProblems() = %q, want exactly one naming %q", problems, tc.want)
			case tc.want != "" && !strings.Contains(joined, tc.want):
				t.Errorf("recordDateProblems() = %q, want it to name %q", problems, tc.want)
			}
		})
	}
}

// TestCheckRecord_CatalogDrift_Notes verifies that a catalog that has moved
// under the record is reported and does not fail: check-e2e-static already
// fails on a rename from the scenario's side, and failing here too would mean
// a rename could not be committed without a booted GitLab.
func TestCheckRecord_CatalogDrift_Notes(t *testing.T) {
	silentProbe(t)
	opts, doc, pagePath := checkFixture(t)
	opts.catalogs = func(edition.Tier) (*servedCatalog, error) {
		catalog := fixtureCatalog()
		delete(catalog.actions, "issue.list")
		for i, id := range catalog.ids {
			if id == "issue.list" {
				catalog.ids = append(catalog.ids[:i], catalog.ids[i+1:]...)
				break
			}
		}
		return catalog, nil
	}
	verdict, err := checkRecord(opts, doc, pagePath, checkClock)
	if err != nil {
		t.Fatalf("checkRecord() = %v, want a verdict", err)
	}
	if len(verdict.Findings) > 0 {
		t.Errorf("findings = %q, want catalog drift to be a note and nothing more", verdict.Findings)
	}
	notes := strings.Join(verdict.Notes, "\n")
	if !strings.Contains(notes, "no longer has (issue.list)") {
		t.Errorf("notes = %q, want the dropped action named", verdict.Notes)
	}
	if !strings.Contains(notes, "was measured against 12 catalog actions and this tree builds 11") {
		t.Errorf("notes = %q, want the catalog size difference named", verdict.Notes)
	}
}

// TestCheckRecord_CatalogUnbuildable_IsAnError verifies that a catalog this
// tree cannot build stops the run rather than being reported as a finding
// about the record: the record is not what is wrong.
func TestCheckRecord_CatalogUnbuildable_IsAnError(t *testing.T) {
	silentProbe(t)
	opts, doc, pagePath := checkFixture(t)
	opts.catalogs = func(edition.Tier) (*servedCatalog, error) { return nil, errors.New("no catalog") }
	if _, err := checkRecord(opts, doc, pagePath, checkClock); err == nil {
		t.Error("checkRecord() = nil error, want the catalog failure returned")
	}
}

// TestRevisionNote_Branches verifies each answer the revision probe can give,
// and in particular that a revision git cannot resolve says nothing: CI
// checks this repository out at depth one, where every revision but HEAD is
// unknown, and a note that fired there would fire on every run and inform
// nobody.
func TestRevisionNote_Branches(t *testing.T) {
	const sha = "6bd82ea61e0ee0751e28b0a75954b9e4b86a8648"
	cases := []struct {
		name   string
		commit string
		probe  func(dir string, args ...string) (bool, error)
		want   string
	}{
		{
			name:   "not a full revision",
			commit: "deadbeef",
			probe:  func(string, ...string) (bool, error) { return true, nil },
		},
		{
			name:   "no commit recorded",
			commit: "",
			probe:  func(string, ...string) (bool, error) { return true, nil },
		},
		{
			name:   "git could not be run",
			commit: sha,
			probe:  func(string, ...string) (bool, error) { return false, errors.New("no git here") },
		},
		{
			name:   "the revision is unknown here",
			commit: sha,
			probe:  func(string, ...string) (bool, error) { return false, nil },
		},
		{
			name:   "the revision is an ancestor",
			commit: sha,
			probe:  func(string, ...string) (bool, error) { return true, nil },
		},
		{
			name:   "the revision is known and not an ancestor",
			commit: sha,
			probe: func(_ string, args ...string) (bool, error) {
				return args[0] == "cat-file", nil
			},
			want: "which is not an ancestor of HEAD",
		},
		{
			name:   "the ancestry question could not be asked",
			commit: sha,
			probe: func(_ string, args ...string) (bool, error) {
				if args[0] == "cat-file" {
					return true, nil
				}
				return false, errors.New("no git here")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := gitProbe
			gitProbe = tc.probe
			t.Cleanup(func() { gitProbe = previous })
			got := revisionNote("/repo", "ee", &recordEntry{Runs: []runRow{{Commit: tc.commit}}})
			if tc.want == "" && got != "" {
				t.Errorf("revisionNote() = %q, want silence", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("revisionNote() = %q, want it to say %q", got, tc.want)
			}
		})
	}
}

// TestAskGit_ExitStatusIsAnAnswer verifies the one distinction the probe
// rests on: a non-zero exit is the answer "no", and a git that could not be
// run at all is an error, which the caller turns into silence.
func TestAskGit_ExitStatusIsAnAnswer(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	if ok, err := askGit(dir, "rev-parse", "--git-dir"); ok || err != nil {
		t.Errorf("askGit() in a directory that is no repository = %t, %v; want false and no error", ok, err)
	}
	if ok, err := askGit(dir, "--version"); !ok || err != nil {
		t.Errorf("askGit() = %t, %v; want true and no error", ok, err)
	}
	if _, err := askGit(filepath.Join(dir, "missing"), "--version"); err == nil {
		t.Error("askGit() in a directory that does not exist = nil error, want the failure to start reported")
	}
}

// TestRunCheckRecord_Fixture_PassesAndPrints verifies the whole gate: it
// prints the headline per runtime and exits zero on a record that holds.
func TestRunCheckRecord_Fixture_PassesAndPrints(t *testing.T) {
	silentProbe(t)
	opts, _, _ := checkFixture(t)
	var stdout, stderr strings.Builder
	if code := runCheckRecord(opts, &stdout, &stderr); code != exitOK {
		t.Fatalf("runCheckRecord() = %d, stderr %q; want 0", code, stderr.String())
	}
	for _, want := range []string{"record: ce (community/free, measured ", "record: ee (enterprise/ultimate, measured "} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want it to carry %q", stdout.String(), want)
			}
		})
	}
}

// TestRunCheckRecord_Note_IsPrintedAndDoesNotFail verifies where a note
// lands: on stdout, beside a zero exit, which is what lets CI tell a reader
// that the licensed half was measured on a tree that is not this one without
// refusing the push.
func TestRunCheckRecord_Note_IsPrintedAndDoesNotFail(t *testing.T) {
	previous := gitProbe
	gitProbe = func(_ string, args ...string) (bool, error) { return args[0] == "cat-file", nil }
	t.Cleanup(func() { gitProbe = previous })

	opts, _, _ := checkFixture(t)
	var stdout, stderr strings.Builder
	if code := runCheckRecord(opts, &stdout, &stderr); code != exitOK {
		t.Fatalf("runCheckRecord() = %d, stderr %q; want 0", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "record: note: ee was measured on 6bd82ea6") {
		t.Errorf("stdout = %q, want the revision note", stdout.String())
	}
}

// TestReadRecord_NoRuntimesKey_ReadsAsEmpty verifies that a document written
// before any half was recorded reads as a record with no runtimes rather than
// as one whose map is nil, so every caller below can write into it.
func TestReadRecord_NoRuntimesKey_ReadsAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e-coverage.json")
	writeFile(t, path, `{"schema_version":1}`)
	doc, err := readRecord(path)
	if err != nil {
		t.Fatalf("readRecord() = %v, want a document", err)
	}
	if doc.Runtimes == nil {
		t.Error("readRecord() left the runtimes map nil")
	}
}

// TestRunCheckRecord_MissingRecord_IsAFinding verifies that an absent record
// is a finding about the tree rather than an inability to run: the gate's job
// is to say the committed artifact is not there.
func TestRunCheckRecord_MissingRecord_IsAFinding(t *testing.T) {
	dir := t.TempDir()
	opts := options{dir: dir, recordPath: filepath.Join(dir, "absent.json"), recordPage: filepath.Join(dir, "absent.md")}
	var stdout, stderr strings.Builder
	if code := runCheckRecord(opts, &stdout, &stderr); code != exitFindings {
		t.Errorf("runCheckRecord() = %d, want %d", code, exitFindings)
	}
	if !strings.Contains(stderr.String(), recordRegenerate) {
		t.Errorf("stderr = %q, want it to name %s", stderr.String(), recordRegenerate)
	}
}

// TestRunCheckRecord_Findings_ExitNonZero verifies that a record that does not
// hold fails the gate and says what it found on stderr, while a note stays on
// stdout where it informs without failing.
func TestRunCheckRecord_Findings_ExitNonZero(t *testing.T) {
	silentProbe(t)
	opts, doc, _ := checkFixture(t)
	delete(doc.Runtimes, "ee")
	writeRecordJSON(t, opts.recordPath, doc)
	var stdout, stderr strings.Builder
	if code := runCheckRecord(opts, &stdout, &stderr); code != exitFindings {
		t.Errorf("runCheckRecord() = %d, want %d", code, exitFindings)
	}
	if !strings.Contains(stderr.String(), "the record holds no ee runtime") {
		t.Errorf("stderr = %q, want the missing runtime named", stderr.String())
	}
}

// writeRecordJSON writes a document without rendering its page, so a test can
// commit a record the page no longer describes.
func writeRecordJSON(t *testing.T, path string, doc *coverageRecord) {
	t.Helper()
	encoded, err := marshalRecord(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err = os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
