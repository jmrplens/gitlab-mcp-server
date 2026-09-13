package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// fixtureOptions is a run over the committed ce shards with the fixture
// catalog standing in for the compiled-in one, so the end-to-end paths are
// exercised without building the real catalog in every test.
func fixtureOptions(t *testing.T) options {
	t.Helper()
	return options{
		dir:            t.TempDir(),
		calls:          callsFixture("ce"),
		catalogs:       func(edition.Tier) (*servedCatalog, error) { return fixtureCatalog(), nil },
		harnessPath:    fakeHarnessPath,
		staticPatterns: staticPatterns,
		exemptions:     map[string]actionExemption{},
		drops:          map[string]dropDeclaration{},
	}
}

// runFixture runs the command and returns its streams.
func runFixture(t *testing.T, opts options) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(opts, &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestRun_NothingToDo_IsAUsageError verifies that a run asked for nothing
// says so and exits 2 rather than reporting an empty success.
func TestRun_NothingToDo_IsAUsageError(t *testing.T) {
	code, _, stderr := runFixture(t, options{})
	if code != exitUsage || !strings.Contains(stderr, "nothing to do") {
		t.Errorf("run() = %d, %q; want exit %d and the usage message", code, stderr, exitUsage)
	}
}

// TestRun_Calls_WritesTheJSONReport verifies the default output: one JSON
// document listing one report per runtime, and the fixture's asserted,
// cleanup and failed cells landing where the shards put them.
func TestRun_Calls_WritesTheJSONReport(t *testing.T) {
	code, stdout, stderr := runFixture(t, fixtureOptions(t))
	if code != exitOK || stderr != "" {
		t.Fatalf("run() = %d, stderr %q; want 0 and nothing on stderr", code, stderr)
	}
	var reports []*report
	if err := json.Unmarshal([]byte(stdout), &reports); err != nil {
		t.Fatalf("stdout is not the JSON report: %v\n%s", err, stdout)
	}
	if len(reports) != 1 || reports[0].Runtime != "community/free" {
		t.Fatalf("reports = %d, want one for community/free", len(reports))
	}
	rep := reports[0]
	if rep.Summary.L1 != 2 || rep.Summary.L2 != 2 || rep.Summary.L3 != 1 {
		t.Errorf("levels = L1 %d, L2 %d, L3 %d; want issue.list on three surfaces and issue.delete on dynamic (2, 2, 1)",
			rep.Summary.L1, rep.Summary.L2, rep.Summary.L3)
	}
	states := map[string]state{}
	for _, row := range rep.Cells {
		states[row.Surface+" "+row.Target] = row.State
	}
	cases := []struct {
		cell string
		want state
	}{
		{cell: "dynamic issue.list", want: stateAsserted},
		{cell: "meta issue.list", want: stateAsserted},
		{cell: "individual issue.list", want: stateAsserted},
		{cell: "dynamic issue.delete", want: stateAsserted},
		{cell: "dynamic project.get", want: stateFailed},
		{cell: "meta admin.list", want: stateUnservable},
	}
	for _, tc := range cases {
		t.Run(tc.cell, func(t *testing.T) {
			if got := states[tc.cell]; got != tc.want {
				t.Errorf("state = %s, want %s", got, tc.want)
			}
		})
	}
	if rep.Diagnostics.LateJoins != 1 {
		t.Errorf("late joins = %d, want the meta call joined from its dispatch line", rep.Diagnostics.LateJoins)
	}
	if rep.Check != nil || rep.Baseline != nil || rep.Results != nil {
		t.Error("check, baseline and results were reported without being asked for")
	}
}

// TestRun_Results_JoinsTheStream verifies that -results joins the go test
// stream and lists the idle tests.
func TestRun_Results_JoinsTheStream(t *testing.T) {
	opts := fixtureOptions(t)
	opts.results = resultsFixture()
	code, stdout, _ := runFixture(t, opts)
	if code != exitOK {
		t.Fatalf("run() = %d, want 0", code)
	}
	var reports []*report
	if err := json.Unmarshal([]byte(stdout), &reports); err != nil {
		t.Fatalf("stdout is not the JSON report: %v", err)
	}
	join := reports[0].Results
	if join == nil || join.Tests != 8 || len(join.TestsWithoutCalls) != 2 {
		t.Errorf("results = %+v, want 8 tests and the two idle approval tests", join)
	}
	opts.results = filepath.Join(t.TempDir(), "absent.json")
	if absentCode, _, stderr := runFixture(t, opts); absentCode != exitUsage || !strings.Contains(stderr, "open results") {
		t.Errorf("run() with a missing results file = %d, %q; want exit 2", absentCode, stderr)
	}
}

// TestRun_Check_FloorsAndExpectedRuntimes verifies -check on the fixture:
// it passes on the ce shards, fails when an expected runtime is missing, and
// fails when a package refused.
func TestRun_Check_FloorsAndExpectedRuntimes(t *testing.T) {
	cases := []struct {
		name     string
		calls    string
		runtime  string
		wantCode int
		wantErr  string
	}{
		{name: "ce passes", calls: callsFixture("ce"), runtime: "ce", wantCode: exitOK},
		{name: "an expected runtime is missing", calls: callsFixture("ce"), runtime: "ce,ee", wantCode: exitFindings, wantErr: "no runtime under"},
		{name: "a package refused", calls: callsFixture("two-runtimes"), runtime: "ee", wantCode: exitFindings, wantErr: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := fixtureOptions(t)
			opts.calls = tc.calls
			opts.runtime = tc.runtime
			opts.check = true
			opts.output = filepath.Join(t.TempDir(), "report.json")
			code, _, stderr := runFixture(t, opts)
			if code != tc.wantCode || !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("run() = %d, stderr %q; want %d mentioning %q", code, stderr, tc.wantCode, tc.wantErr)
			}
		})
	}
}

// TestRun_Runtime_SelectsAndWarns verifies -runtime without -check: a
// selector nothing matches is warned about, the report is an empty list
// rather than null, and the exit stays 0.
func TestRun_Runtime_SelectsAndWarns(t *testing.T) {
	opts := fixtureOptions(t)
	opts.runtime = "ee"
	code, stdout, stderr := runFixture(t, opts)
	if code != exitOK || !strings.Contains(stderr, "no runtime under") || strings.TrimSpace(stdout) != "[]" {
		t.Errorf("run() = %d, stdout %q, stderr %q; want 0, an empty list and the warning", code, stdout, stderr)
	}
}

// TestRun_Baseline_SupersetPassingAndFailing verifies -baseline both ways:
// the ce shards against themselves lose nothing, and against the old-suite
// fixture they lose the credit it reached and they did not.
func TestRun_Baseline_SupersetPassingAndFailing(t *testing.T) {
	passing := fixtureOptions(t)
	passing.baseline = callsFixture("ce")
	passing.output = filepath.Join(t.TempDir(), "report.json")
	if code, _, stderr := runFixture(t, passing); code != exitOK {
		t.Errorf("run() against itself = %d, %q; want 0", code, stderr)
	}

	failing := fixtureOptions(t)
	failing.baseline = callsFixture("baseline-ce")
	failing.summary = "-"
	failing.output = filepath.Join(t.TempDir(), "report.json")
	code, stdout, _ := runFixture(t, failing)
	if code != exitFindings {
		t.Errorf("run() against the old suite = %d, want %d", code, exitFindings)
	}
	if !strings.Contains(stdout, "- meta/default issue.create asserted") {
		t.Errorf("summary does not list the lost credit:\n%s", stdout)
	}

	missing := fixtureOptions(t)
	missing.baseline = callsFixture("two-runtimes")
	missing.runtime = "ce"
	missing.output = filepath.Join(t.TempDir(), "report.json")
	if missingCode, _, stderr := runFixture(t, missing); missingCode != exitOK {
		t.Errorf("run() with a baseline holding ce = %d, %q; want 0", missingCode, stderr)
	}
	absent := fixtureOptions(t)
	absent.calls = callsFixture("two-runtimes")
	absent.runtime = "ee"
	absent.baseline = callsFixture("ce")
	absent.output = filepath.Join(t.TempDir(), "report.json")
	if absentCode, _, stderr := runFixture(t, absent); absentCode != exitUsage || !strings.Contains(stderr, "no run of this runtime") {
		t.Errorf("run() with no baseline for ee = %d, %q; want exit 2 saying so", absentCode, stderr)
	}
}

// TestRun_Report_WritesTheWorkList verifies -report: the TSV work list
// replaces the JSON on stdout, and -o still writes the JSON beside it.
func TestRun_Report_WritesTheWorkList(t *testing.T) {
	opts := fixtureOptions(t)
	opts.report = true
	opts.output = filepath.Join(t.TempDir(), "report.json")
	code, stdout, _ := runFixture(t, opts)
	if code != exitOK {
		t.Fatalf("run() = %d, want 0", code)
	}
	if !strings.HasPrefix(stdout, "admin.list\tadmin\tfree\tdynamic=unservable\tmeta=unservable\tindividual=unservable\n") {
		t.Errorf("work list does not start with the admin gap:\n%s", stdout)
	}
	if !strings.Contains(stdout, "e2e coverage community/free: L1 2/12 (16.7%), L2 2, L3 1, 10 actions not asserted on any surface\n") {
		t.Errorf("work list summary line is wrong:\n%s", stdout)
	}
	if !strings.Contains(stdout, "written to "+opts.output) {
		t.Errorf("the JSON path was not announced:\n%s", stdout)
	}
	if _, err := os.Stat(opts.output); err != nil {
		t.Errorf("the JSON report was not written: %v", err)
	}
}

// TestRun_Summary_WrittenToFile verifies -summary to a path, and the
// refusal when the path cannot be written.
func TestRun_Summary_WrittenToFile(t *testing.T) {
	opts := fixtureOptions(t)
	opts.summary = filepath.Join(t.TempDir(), "summary.md")
	opts.output = filepath.Join(t.TempDir(), "report.json")
	if code, _, stderr := runFixture(t, opts); code != exitOK {
		t.Fatalf("run() = %d, %q; want 0", code, stderr)
	}
	content, err := os.ReadFile(opts.summary)
	if err != nil || !strings.HasPrefix(string(content), "## E2E coverage\n\n### community/free\n") {
		t.Errorf("summary = %q, %v; want the Markdown summary", content, err)
	}
	// A second run appends, since the path a CI step gives is the job's
	// step summary, which every step writes its own section to.
	if code, _, stderr := runFixture(t, opts); code != exitOK {
		t.Fatalf("second run() = %d, %q; want 0", code, stderr)
	}
	if appended, readErr := os.ReadFile(opts.summary); readErr != nil || strings.Count(string(appended), "## E2E coverage\n") != 2 {
		t.Errorf("summary after two runs holds %d sections, %v; want 2", strings.Count(string(appended), "## E2E coverage\n"), readErr)
	}
	opts.summary = filepath.Join(t.TempDir(), "missing", "summary.md")
	if code, _, stderr := runFixture(t, opts); code != exitUsage || !strings.Contains(stderr, "write summary") {
		t.Errorf("run() with an unwritable summary path = %d, %q; want exit 2", code, stderr)
	}
	opts.summary = ""
	opts.output = filepath.Join(t.TempDir(), "missing", "report.json")
	if code, _, stderr := runFixture(t, opts); code != exitUsage || !strings.Contains(stderr, "write report") {
		t.Errorf("run() with an unwritable report path = %d, %q; want exit 2", code, stderr)
	}
}

// TestRun_SummaryOnStdout_OwnsTheStream verifies that -summary - with no -o
// puts the Markdown on stdout in place of the JSON, rather than after it: a
// stream holding both is a document nothing parses and nobody reads. With -o
// the JSON goes to the file and stdout still carries only the summary and
// the line naming that file.
func TestRun_SummaryOnStdout_OwnsTheStream(t *testing.T) {
	opts := fixtureOptions(t)
	opts.summary = summaryToStdout
	code, stdout, stderr := runFixture(t, opts)
	if code != exitOK || stderr != "" {
		t.Fatalf("run() = %d, stderr %q; want 0 and nothing on stderr", code, stderr)
	}
	if !strings.HasPrefix(stdout, "## E2E coverage\n") {
		t.Errorf("stdout does not start with the summary:\n%s", stdout)
	}
	if strings.Contains(stdout, `"runtime"`) {
		t.Errorf("stdout carries the JSON report beside the summary:\n%s", stdout)
	}

	opts.output = filepath.Join(t.TempDir(), "report.json")
	code, stdout, _ = runFixture(t, opts)
	if code != exitOK {
		t.Fatalf("run() with -o = %d, want 0", code)
	}
	if !strings.Contains(stdout, "## E2E coverage\n") || !strings.Contains(stdout, "written to "+opts.output) {
		t.Errorf("stdout with -o lacks the summary or the file line:\n%s", stdout)
	}
	if _, err := os.Stat(opts.output); err != nil {
		t.Errorf("the JSON report was not written beside the summary: %v", err)
	}
}

// TestRun_Calls_Refusals verifies the refusals on the way in: a directory
// with no shard, a directory naming two runtimes, and a catalog that cannot
// be built.
func TestRun_Calls_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*options)
		wantErr string
	}{
		{name: "no shard", mutate: func(o *options) { o.calls = t.TempDir() }, wantErr: "no calls-*.jsonl shard"},
		{name: "two runtimes in one directory", mutate: func(o *options) { o.calls = callsFixture("mixed") }, wantErr: "one directory per runtime"},
		{name: "the catalog cannot be built", mutate: func(o *options) {
			o.catalogs = func(edition.Tier) (*servedCatalog, error) { return nil, os.ErrNotExist }
		}, wantErr: "file does not exist"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := fixtureOptions(t)
			tc.mutate(&opts)
			code, _, stderr := runFixture(t, opts)
			if code != exitUsage || !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("run() = %d, %q; want exit 2 mentioning %q", code, stderr, tc.wantErr)
			}
		})
	}
}

// TestRun_Static_OverTheFixtureModule verifies -static through run(): the
// planted defects fail it, the notes and the summary line are printed, and
// with -calls beside it the static result reaches the classification.
func TestRun_Static_OverTheFixtureModule(t *testing.T) {
	opts := fixtureOptions(t)
	opts.calls = ""
	opts.static = true
	opts.dir = fakeModuleDir(t)
	opts.staticCatalog = fakeCatalog
	code, stdout, stderr := runFixture(t, opts)
	if code != exitFindings {
		t.Errorf("run() = %d, want %d for the planted defects", code, exitFindings)
	}
	if !strings.Contains(stderr, "nope.action is not a catalog action") {
		t.Errorf("stderr lacks the unknown-id finding:\n%s", stderr)
	}
	for _, want := range []string{
		"static: note: harness export Unused is used by nothing yet",
		"non-constant id tc.id",
		"static: 22 id sites in 3 packages, 2 non-constant sites, 4 unused harness exports, 11 findings",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout lacks %q:\n%s", want, stdout)
			}
		})
	}
}

// TestRun_Static_NoSuite_Passes verifies the answer on the tree before the
// new suite exists: the gate passes and says why.
func TestRun_Static_NoSuite_Passes(t *testing.T) {
	opts := fixtureOptions(t)
	opts.calls = ""
	opts.static = true
	opts.staticCatalog = fakeCatalog
	code, stdout, _ := runFixture(t, opts)
	if code != exitOK || !strings.Contains(stdout, "does not exist yet") {
		t.Errorf("run() = %d, %q; want 0 and the skip message", code, stdout)
	}
}

// TestRun_Static_CatalogFromBuilder verifies that without an injected
// catalog the gate builds one at Ultimate, and that a builder that fails is
// reported.
func TestRun_Static_CatalogFromBuilder(t *testing.T) {
	opts := fixtureOptions(t)
	opts.calls = ""
	opts.static = true
	opts.dir = fakeModuleDir(t)
	opts.catalogs = func(tier edition.Tier) (*servedCatalog, error) {
		if tier != edition.Ultimate {
			t.Errorf("static catalog built at %s, want ultimate", tier)
		}
		return fixtureCatalog(), nil
	}
	if code, _, _ := runFixture(t, opts); code != exitFindings {
		t.Errorf("run() = %d, want %d: the fixture names actions the fixture catalog lacks", code, exitFindings)
	}
	opts.catalogs = func(edition.Tier) (*servedCatalog, error) { return nil, os.ErrClosed }
	if code, _, stderr := runFixture(t, opts); code != exitUsage || !strings.Contains(stderr, "static:") {
		t.Errorf("run() with a failing builder = %d, %q; want exit 2", code, stderr)
	}
}

// TestRun_Static_WithCalls_ReachesTheClassification verifies that a static
// result runs into the coverage report: the fixture's discarded issue.get
// would mark an asserted issue.get unasserted, and the skip line's test ids
// mark project.list skipped.
func TestRun_Static_WithCalls_ReachesTheClassification(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "calls-x.jsonl"), strings.Join([]string{
		`{"schema":1,"type":"run","run":{"package":"common","requirement":"any","edition":"community","tier":"free","run_id":"r","status":"started"}}`,
		`{"schema":1,"type":"session","session":{"label":"d","surface":"dynamic","mode":"default","capabilities":"full","transport":"stdio","tools":["gitlab_execute_action"],"dispatch_observed":true}}`,
		`{"schema":1,"type":"call","call":{"test":"TestPlanted_DiscardedResult_Reported","purpose":"test","expectation":"ok","session":"d","surface":"dynamic","mode":"default","capabilities":"full","requirement":"any","method":"tools/call","tool":"gitlab_execute_action","action":"issue.get","dispatched":"issue.get","outcome":"ok","test_status":"passed"}}`,
		`{"schema":1,"type":"skip","skip":{"test":"TestPlanted_HelperConstant_Resolved","reason":"no runner"}}`,
	}, "\n")+"\n")
	opts := fixtureOptions(t)
	opts.calls = dir
	opts.static = true
	opts.dir = fakeModuleDir(t)
	opts.staticCatalog = fakeCatalog
	opts.catalogs = func(edition.Tier) (*servedCatalog, error) {
		catalog := fixtureCatalog()
		catalog.actions["issue.get"] = catalogAction{id: "issue.get", domain: "issue", readOnly: true, metaTool: "gitlab_issue", individualTool: "gitlab_issue_get"}
		catalog.ids = append(catalog.ids, "issue.get")
		return catalog, nil
	}
	opts.output = filepath.Join(t.TempDir(), "report.json")
	if code, _, _ := runFixture(t, opts); code != exitFindings {
		t.Fatalf("run() = %d, want %d from the static findings", code, exitFindings)
	}
	content, err := os.ReadFile(opts.output)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var reports []*report
	if decodeErr := json.Unmarshal(content, &reports); decodeErr != nil {
		t.Fatalf("report is not JSON: %v", decodeErr)
	}
	states := map[string]state{}
	for _, row := range reports[0].Cells {
		states[row.Target] = row.State
	}
	if states["issue.get"] != stateUnasserted || states["project.get"] != stateSkipped {
		t.Errorf("states = issue.get %s, project.get %s; want unasserted and skipped", states["issue.get"], states["project.get"])
	}
}

// TestRun_PortMap_OverTheFixtures verifies -port-map through run(): the
// unresolved tests and the findings are printed, the retired list is
// counted in the summary and the exit code says the map is incomplete; an
// old suite that does not exist is a usage error.
func TestRun_PortMap_OverTheFixtures(t *testing.T) {
	opts := fixtureOptions(t)
	opts.calls = ""
	opts.portMap = true
	opts.dir = "testdata"
	opts.oldSuite = filepath.Join("portmap", "old")
	opts.newSuite = filepath.Join("portmap", "new")
	opts.drops = fixtureDrops
	opts.retired = fixtureRetired
	code, stdout, stderr := runFixture(t, opts)
	if code != exitFindings {
		t.Errorf("run() = %d, want %d", code, exitFindings)
	}
	if !strings.Contains(stdout, "port map: TestMeta_Unresolved has no Replaces: successor and no declared drop\n") ||
		!strings.Contains(stdout, "port map: 7 old tests (1 retired), 4 replaced, 1 dropped, 2 unresolved, 5 findings\n") {
		t.Errorf("stdout is not the port map report:\n%s", stdout)
	}
	if !strings.Contains(stderr, "port map: TestIssue_Ghost replaces TestMeta_Ghost") ||
		!strings.Contains(stderr, "port map: TestMeta_Issues is retired and still declared in the old suite") {
		t.Errorf("stderr lacks the findings:\n%s", stderr)
	}

	opts.oldSuite = "absent"
	if absentCode, _, absentErr := runFixture(t, opts); absentCode != exitUsage || !strings.Contains(absentErr, "port map") {
		t.Errorf("run() with no old suite = %d, %q; want exit 2", absentCode, absentErr)
	}
	opts.oldSuite = filepath.Join("portmap", "new")
	if flatCode, _, flatErr := runFixture(t, opts); flatCode != exitUsage || !strings.Contains(flatErr, "no Test function") {
		t.Errorf("run() with an old suite holding tests only one level down = %d, %q; want exit 2", flatCode, flatErr)
	}
	opts.oldSuite = filepath.Join("portmap", "old")
	opts.newSuite = "absent"
	if earlyCode, earlyOut, _ := runFixture(t, opts); earlyCode != exitFindings || !strings.Contains(earlyOut, "7 old tests (1 retired), 0 replaced") {
		t.Errorf("run() before the new suite exists = %d, %q; want exit 1 with nothing replaced", earlyCode, earlyOut)
	}
}

// TestRun_Static_RepositoryRoot_FoundFromTheWorkingDirectory verifies the
// plan's own verification line: with no -dir the root is found from the
// working directory, and on this tree, where test/e2e/gitlab does not exist
// yet, -static exits 0.
func TestRun_Static_RepositoryRoot_FoundFromTheWorkingDirectory(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", gitlabTestDir)); err == nil {
		t.Skip("test/e2e/gitlab exists now; the no-suite answer is pinned by TestRun_Static_NoSuite_Passes")
	}
	opts := options{static: true, staticCatalog: fakeCatalog, harnessPath: fakeHarnessPath, staticPatterns: staticPatterns}
	code, stdout, stderr := runFixture(t, opts)
	if code != exitOK || !strings.Contains(stdout, "does not exist yet") {
		t.Errorf("run() = %d, %q, %q; want 0 and the skip message", code, stdout, stderr)
	}
}

// TestRun_Static_RatchetOn_FailsOnDeadExports verifies the ratchet through
// run(): dead exports become findings and are no longer printed as notes.
func TestRun_Static_RatchetOn_FailsOnDeadExports(t *testing.T) {
	opts := fixtureOptions(t)
	opts.calls = ""
	opts.static = true
	opts.dir = fakeModuleDir(t)
	opts.staticCatalog = fakeCatalog
	opts.ratchet = true
	code, stdout, stderr := runFixture(t, opts)
	if code != exitFindings {
		t.Errorf("run() = %d, want %d", code, exitFindings)
	}
	if strings.Contains(stdout, "is used by nothing yet") || !strings.Contains(stderr, "dead-export: Unused is exported by the harness and used by nothing") {
		t.Errorf("dead exports were not promoted to findings:\nstdout %s\nstderr %s", stdout, stderr)
	}
}

// TestRun_Check_PassingVerdictInSummary verifies the passing spelling of the
// check verdict in the Markdown summary, beside a baseline that lost
// nothing.
func TestRun_Check_PassingVerdictInSummary(t *testing.T) {
	opts := fixtureOptions(t)
	opts.check = true
	opts.baseline = callsFixture("ce")
	opts.summary = "-"
	opts.output = filepath.Join(t.TempDir(), "report.json")
	code, stdout, _ := runFixture(t, opts)
	if code != exitOK {
		t.Fatalf("run() = %d, want 0", code)
	}
	for _, want := range []string{"- Check: passed\n", "- Baseline: passed (4 reached in the baseline, 0 lost)\n"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(stdout, want) {
				t.Errorf("summary lacks %q:\n%s", want, stdout)
			}
		})
	}
}

// TestRun_Baseline_Unreadable_IsAUsageError verifies that a baseline
// directory that cannot be read stops the run before any comparison.
func TestRun_Baseline_Unreadable_IsAUsageError(t *testing.T) {
	opts := fixtureOptions(t)
	opts.baseline = filepath.Join(t.TempDir(), "absent")
	if code, _, stderr := runFixture(t, opts); code != exitUsage || !strings.Contains(stderr, "baseline") {
		t.Errorf("run() = %d, %q; want exit 2 naming the baseline", code, stderr)
	}
}

// TestRun_PortMap_Complete verifies the passing exit when every old test is
// replaced.
func TestRun_PortMap_Complete(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "old"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "new"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(root, "old", "a_test.go"), "package old\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n")
	writeFile(t, filepath.Join(root, "new", "a_test.go"), "package new\n\nimport \"testing\"\n\n// Replaces: TestA\nfunc TestNewA(t *testing.T) {}\n")
	opts := fixtureOptions(t)
	opts.calls = ""
	opts.portMap = true
	opts.dir = root
	opts.oldSuite = "old"
	opts.newSuite = "new"
	if code, stdout, _ := runFixture(t, opts); code != exitOK || !strings.Contains(stdout, "1 old tests (0 retired), 1 replaced, 0 dropped, 0 unresolved, 0 findings") {
		t.Errorf("run() = %d, %q; want 0 and a complete map", code, stdout)
	}
}
