package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// skipsFixture returns the path of one of the two recorded runs of issue
// 1014, ce or ee (testdata/skips/README.txt).
func skipsFixture(runtime string) string {
	return filepath.Join("testdata", "skips", runtime+"-de1ab3b49.json")
}

// The reasons the recorded runs printed, as the test that skipped wrote them.
const (
	reasonNoExternalNetwork = "external network unavailable: set E2E_EXTERNAL_NETWORK=true to allow tests that call public URLs"
	reasonNoBitbucket       = "bitbucket fixture unavailable: BITBUCKET_SERVER_URL is not set, so the bitbucket fixture is not provisioned for this run"
	reasonTierFallback      = "the run's token read no license, so the probe's tier (free) is the fallback rather than a reading"
	reasonWorkItemWidgets   = "client-go v3.0.0's work item get, create and update documents select five licensed widgets " +
		"that Community Edition's schema does not have (docs/development/upstream-bugs.md, entry 45),so the lifecycle " +
		"cannot run on a Community image"
	savedViewAnswer = "create_work_item_saved_view: GitLab internal server error: the server encountered an unexpected " +
		"condition. Suggestion: verify namespace_path is a full group or project path you can write to, and that sort " +
		"is a WorkItemSort enum value such as CREATED_DESC: POST http://localhost:8929/api/graphql: 500 (GraphQL " +
		"errors: Internal server error)"
	savedViewLifecycle = "TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete"
	// savedViewCreateRefusal is the start of the reason the saved view
	// lifecycle printed when it skipped on the create's 500, which it did in
	// both recorded runs. It lived beside the live declarations while they
	// excused that skip; the scenario no longer skips, so it is a fact about
	// these two runs and nothing else.
	savedViewCreateRefusal = "the saved view create answered the experiment's 500"
)

// needsRelease is the reason a scenario gated on GitLab 19.4 printed on the
// release the run tested.
func needsRelease(tested string) string {
	return "GitLab 19.4 or later unavailable: this instance is GitLab " + tested + ", and the test needs GitLab 19.4 or later"
}

// savedViewRefusal is the reason the saved view lifecycle printed on one
// surface, the call's duration being the only part that differs.
func savedViewRefusal(elapsed string) string {
	return savedViewCreateRefusal + " on this GitLab version: tool_error after " + elapsed + ": " + savedViewAnswer
}

// recordedRunDeclarations declares, the way [declaredSkips] does, the skips
// issue 1014 left standing in the two recorded runs. The tests judge the gate
// with it rather than with the live table, which is free to change as runs
// change, while these runs never will.
var recordedRunDeclarations = []skipDeclaration{
	{Runtime: "ce", Package: "common", Test: savedViewLifecycle, Because: savedViewCreateRefusal, Category: skipCategoryGitLabDefect, Reason: "500 on 19.3.0"},
	{Runtime: "ee", Package: "common", Test: savedViewLifecycle, Because: savedViewCreateRefusal, Category: skipCategoryGitLabDefect, Reason: "500 on 19.3.1-ee"},
}

// TestParseSkips_RecordedRuns_EverySkipWithTheReasonItPrinted verifies that
// the two recorded runs read back as the skips issue 1014 lists, each with the
// reason its test printed and in package and test order: the subtests that
// skipped under a parent that passed, the importers and the saved view
// lifecycle's three surfaces, and none of the tests that passed beside them.
func TestParseSkips_RecordedRuns_EverySkipWithTheReasonItPrinted(t *testing.T) {
	cases := []struct {
		runtime string
		want    []recordedSkip
	}{
		{runtime: "ce", want: []recordedSkip{
			{Package: "common", Test: "TestAdmin_ExternalImporters/BitbucketCloudImport", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestAdmin_ExternalImporters/GistsImport", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestAdmin_ExternalImporters/GitHubImportAndCancel", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestBitbucketCloudImport_Individual", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestGroupServiceAccounts_Get_ReadsTheAccountBackByID", Reason: needsRelease("19.3.0")},
			{Package: "common", Test: "TestProjectServiceAccounts_Get_ReadsTheAccountBackByID", Reason: needsRelease("19.3.0")},
			{Package: "common", Test: "TestTierPin_DetectionAgreesWithTheRuntimeProbe", Reason: reasonTierFallback},
			{Package: "common", Test: savedViewLifecycle + "/dynamic", Reason: savedViewRefusal("155ms")},
			{Package: "common", Test: savedViewLifecycle + "/individual", Reason: savedViewRefusal("134ms")},
			{Package: "common", Test: savedViewLifecycle + "/meta", Reason: savedViewRefusal("122ms")},
			{Package: "common", Test: "TestWorkItems_Lifecycle_IssueTypeCreateListGetUpdateDelete", Reason: reasonWorkItemWidgets},
		}},
		{runtime: "ee", want: []recordedSkip{
			{Package: "common", Test: "TestAdmin_ExternalImporters/BitbucketCloudImport", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestAdmin_ExternalImporters/BitbucketServerImport", Reason: reasonNoBitbucket},
			{Package: "common", Test: "TestAdmin_ExternalImporters/GistsImport", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestAdmin_ExternalImporters/GitHubImportAndCancel", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestBitbucketCloudImport_Individual", Reason: reasonNoExternalNetwork},
			{Package: "common", Test: "TestGroupServiceAccounts_Get_ReadsTheAccountBackByID", Reason: needsRelease("19.3.1-ee")},
			{Package: "common", Test: "TestProjectServiceAccounts_Get_ReadsTheAccountBackByID", Reason: needsRelease("19.3.1-ee")},
			{Package: "common", Test: savedViewLifecycle + "/dynamic", Reason: savedViewRefusal("139ms")},
			{Package: "common", Test: savedViewLifecycle + "/individual", Reason: savedViewRefusal("160ms")},
			{Package: "common", Test: savedViewLifecycle + "/meta", Reason: savedViewRefusal("150ms")},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.runtime, func(t *testing.T) {
			got, err := readSkips(skipsFixture(tc.runtime))
			if err != nil {
				t.Fatalf("readSkips() error = %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("readSkips() =\n%v\nwant\n%v", got, tc.want)
			}
		})
	}
}

// TestReadSkips_MissingFile_SaysWhatItCouldNotOpen verifies that a stream that
// is not there is an error naming the open, not an empty list of skips.
func TestReadSkips_MissingFile_SaysWhatItCouldNotOpen(t *testing.T) {
	got, err := readSkips(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil || !strings.Contains(err.Error(), "open results") || got != nil {
		t.Errorf("readSkips() = %v, %v; want no skips and an error opening the results", got, err)
	}
}

// TestParseSkips_Streams verifies what the parser keeps of a stream: only a
// test whose last run ended skipped, that run's reason and not an earlier
// one's, and the refusals of a stream that is not one.
func TestParseSkips_Streams(t *testing.T) {
	cases := []struct {
		name    string
		stream  string
		want    []recordedSkip
		wantErr string
	}{
		{
			name: "a skip that a rerun passed is no skip",
			stream: `{"Action":"run","Package":"m/test/e2e/gitlab/common","Test":"TestA"}
{"Action":"output","Package":"m/test/e2e/gitlab/common","Test":"TestA","Output":"    a_test.go:3: flaked\n"}
{"Action":"skip","Package":"m/test/e2e/gitlab/common","Test":"TestA"}
{"Action":"run","Package":"m/test/e2e/gitlab/common","Test":"TestA"}
{"Action":"pass","Package":"m/test/e2e/gitlab/common","Test":"TestA"}`,
			want: []recordedSkip{},
		},
		{
			name: "a rerun that skipped carries its own reason and not the first run's",
			stream: `{"Action":"run","Package":"m/test/e2e/gitlab/common","Test":"TestA"}
{"Action":"output","Package":"m/test/e2e/gitlab/common","Test":"TestA","Output":"    a_test.go:3: the first run's log\n"}
{"Action":"fail","Package":"m/test/e2e/gitlab/common","Test":"TestA"}
{"Action":"run","Package":"m/test/e2e/gitlab/common","Test":"TestA"}
{"Action":"output","Package":"m/test/e2e/gitlab/common","Test":"TestA","Output":"--- SKIP: TestA (0.00s)\n"}
{"Action":"skip","Package":"m/test/e2e/gitlab/common","Test":"TestA"}`,
			want: []recordedSkip{{Package: "common", Test: "TestA"}},
		},
		{
			name: "a test's events before its run line are kept",
			stream: `{"Action":"output","Package":"m/test/e2e/gitlab/ce","Test":"TestB","Output":"    b_test.go:9: why\n"}

{"Action":"pause","Package":"m/test/e2e/gitlab/ce","Test":"TestB"}
{"Action":"skip","Package":"m/test/e2e/gitlab/ce","Test":"TestB"}`,
			want: []recordedSkip{{Package: "ce", Test: "TestB", Reason: "why"}},
		},
		{
			name: "the same test name in two packages is two tests",
			stream: `{"Action":"output","Package":"m/test/e2e/gitlab/ee","Test":"TestC","Output":"    c_test.go:1: in ee\n"}
{"Action":"skip","Package":"m/test/e2e/gitlab/ee","Test":"TestC"}
{"Action":"output","Package":"m/test/e2e/gitlab/common","Test":"TestC","Output":"    c_test.go:1: in common\n"}
{"Action":"skip","Package":"m/test/e2e/gitlab/common","Test":"TestC"}
{"Action":"skip","Package":"m/test/e2e/gitlab/common"}`,
			want: []recordedSkip{
				{Package: "common", Test: "TestC", Reason: "in common"},
				{Package: "ee", Test: "TestC", Reason: "in ee"},
			},
		},
		{
			name:    "a line that is not JSON is refused with its number",
			stream:  "{\"Action\":\"pass\",\"Test\":\"TestA\"}\nPASS\n",
			wantErr: "results line 2",
		},
		{
			name:    "a stream of package verdicts alone holds no test verdict",
			stream:  `{"Action":"start","Package":"m/a"}` + "\n" + `{"Action":"skip","Package":"m/a"}`,
			wantErr: errNoVerdict.Error(),
		},
		{
			name:    "a line longer than an event can be is refused",
			stream:  `{"Action":"pass","Test":"TestA","Output":"` + strings.Repeat("x", maxEventLine) + `"}`,
			wantErr: "read results",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSkips(strings.NewReader(tc.stream))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || got != nil {
					t.Fatalf("parseSkips() = %v, %v; want no skips and an error carrying %q", got, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSkips() error = %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("parseSkips() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseSkips_NoVerdict_IsTheSentinel verifies that the error a stream with
// no verdict gives is the one [parseResults] gives, so a caller can tell it
// apart from a stream it could not read.
func TestParseSkips_NoVerdict_IsTheSentinel(t *testing.T) {
	_, err := parseSkips(strings.NewReader(""))
	if !errors.Is(err, errNoVerdict) {
		t.Errorf("parseSkips(empty) error = %v, want %v", err, errNoVerdict)
	}
}

// TestSkipReason_ReadsTheLastMessageBeforeTheSkip verifies how a reason is
// read back out of what a test printed.
func TestSkipReason_ReadsTheLastMessageBeforeTheSkip(t *testing.T) {
	cases := []struct {
		name   string
		output []string
		want   string
	}{
		{name: "nothing printed", output: nil, want: ""},
		{
			name:   "a skip with no message prints the frames alone",
			output: []string{"=== RUN   TestA\n", "--- SKIP: TestA (0.00s)\n"},
			want:   "",
		},
		{
			name:   "the message before the frame",
			output: []string{"=== RUN   TestA\n", "    a_test.go:12: the reason\n", "--- SKIP: TestA (0.00s)\n"},
			want:   "the reason",
		},
		{
			name:   "the last of two messages is the skip's",
			output: []string{"    a_test.go:10: a log line\n", "    a_test.go:12: the reason\n", "--- SKIP: TestA (0.00s)\n"},
			want:   "the reason",
		},
		{
			name: "a message over several lines is joined, and a frame and a blank line between them are left out",
			output: []string{
				"    a_test.go:12: the first line\n", "        the second line\n", "=== CONT  TestA\n", "\n",
				"        the third line\n", "--- SKIP: TestA (0.00s)\n",
			},
			want: "the first line the second line the third line",
		},
		{
			name:   "what a test printed after the frame is not its reason",
			output: []string{"    a_test.go:12: the reason\n", "    --- SKIP: TestA/sub (0.00s)\n", "    a_test.go:40: printed later\n"},
			want:   "the reason",
		},
		{
			name:   "a stream without the frame reads up to its end",
			output: []string{"    a_test.go:12: the reason\n", "    a_test.go:13: the last line\n"},
			want:   "the last line",
		},
		{
			name:   "a line naming a file without the indentation of a log line is not a message",
			output: []string{"a_test.go:12: printed by the program\n", "--- SKIP: TestA (0.00s)\n"},
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipReason(tc.output); got != tc.want {
				t.Errorf("skipReason() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSkipDeclaration_Excuses verifies what one declaration covers: its
// runtime, its package, its test or a subtest of it, and a reason carrying
// its fragment, each of which alone is not enough.
func TestSkipDeclaration_Excuses(t *testing.T) {
	declaration := skipDeclaration{Runtime: "ce", Package: "common", Test: "TestA", Because: "fixture"}
	cases := []struct {
		name    string
		runtime string
		skip    recordedSkip
		want    bool
	}{
		{name: "the test itself", runtime: "ce", skip: recordedSkip{Package: "common", Test: "TestA", Reason: "no fixture here"}, want: true},
		{name: "a subtest", runtime: "ce", skip: recordedSkip{Package: "common", Test: "TestA/meta", Reason: "the fixture"}, want: true},
		{name: "another runtime", runtime: "ee", skip: recordedSkip{Package: "common", Test: "TestA", Reason: "fixture"}},
		{name: "another package", runtime: "ce", skip: recordedSkip{Package: "ce", Test: "TestA", Reason: "fixture"}},
		{name: "a test whose name only begins with it", runtime: "ce", skip: recordedSkip{Package: "common", Test: "TestAB", Reason: "fixture"}},
		{name: "the parent of a declared subtest", runtime: "ce", skip: recordedSkip{Package: "common", Test: "Test", Reason: "fixture"}},
		{name: "another reason", runtime: "ce", skip: recordedSkip{Package: "common", Test: "TestA", Reason: "no runner"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaration.excuses(tc.runtime, tc.skip); got != tc.want {
				t.Errorf("excuses(%s, %+v) = %t, want %t", tc.runtime, tc.skip, got, tc.want)
			}
		})
	}
}

// skipTests names the tests of a list of skips, which is what a verdict's
// assertions compare.
func skipTests(skips []recordedSkip) []string {
	names := make([]string, 0, len(skips))
	for _, skip := range skips {
		names = append(names, skip.Test)
	}
	return names
}

// excusedTests names the tests a verdict excused, each with the category of
// the declaration that did.
func excusedTests(excused []excusedSkip) []string {
	names := make([]string, 0, len(excused))
	for _, entry := range excused {
		names = append(names, entry.by.Category+" "+entry.skip.Test)
	}
	return names
}

// staleTests names the declarations a verdict found stale, by runtime and
// test.
func staleTests(stale []skipDeclaration) []string {
	names := make([]string, 0, len(stale))
	for _, declaration := range stale {
		names = append(names, declaration.Runtime+" "+declaration.Test)
	}
	return names
}

// TestJudgeSkips_RecordedRuns_DeclaredAndUndeclared verifies the gate on the
// two recorded runs: the CE run's eight skips issue 1014 fixed are undeclared
// and its saved view skips are declared, and the EE run's seven likewise, the
// Bitbucket Server import among them, since both runtimes start its fixture
// now. No declaration is stale, since a declaration for the other runtime is
// not judged in this one.
func TestJudgeSkips_RecordedRuns_DeclaredAndUndeclared(t *testing.T) {
	cases := []struct {
		runtime        string
		wantExcused    []string
		wantUndeclared []string
	}{
		{
			runtime: "ce",
			wantExcused: []string{
				"gitlab-defect " + savedViewLifecycle + "/dynamic",
				"gitlab-defect " + savedViewLifecycle + "/individual",
				"gitlab-defect " + savedViewLifecycle + "/meta",
			},
			wantUndeclared: []string{
				"TestAdmin_ExternalImporters/BitbucketCloudImport", "TestAdmin_ExternalImporters/GistsImport",
				"TestAdmin_ExternalImporters/GitHubImportAndCancel", "TestBitbucketCloudImport_Individual",
				"TestGroupServiceAccounts_Get_ReadsTheAccountBackByID", "TestProjectServiceAccounts_Get_ReadsTheAccountBackByID",
				"TestTierPin_DetectionAgreesWithTheRuntimeProbe", "TestWorkItems_Lifecycle_IssueTypeCreateListGetUpdateDelete",
			},
		},
		{
			runtime: "ee",
			wantExcused: []string{
				"gitlab-defect " + savedViewLifecycle + "/dynamic",
				"gitlab-defect " + savedViewLifecycle + "/individual",
				"gitlab-defect " + savedViewLifecycle + "/meta",
			},
			wantUndeclared: []string{
				"TestAdmin_ExternalImporters/BitbucketCloudImport", "TestAdmin_ExternalImporters/BitbucketServerImport",
				"TestAdmin_ExternalImporters/GistsImport", "TestAdmin_ExternalImporters/GitHubImportAndCancel",
				"TestBitbucketCloudImport_Individual", "TestGroupServiceAccounts_Get_ReadsTheAccountBackByID",
				"TestProjectServiceAccounts_Get_ReadsTheAccountBackByID",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.runtime, func(t *testing.T) {
			skips, err := readSkips(skipsFixture(tc.runtime))
			if err != nil {
				t.Fatalf("readSkips() error = %v", err)
			}
			verdict := judgeSkips(tc.runtime, skips, recordedRunDeclarations)
			if got := excusedTests(verdict.excused); !slices.Equal(got, tc.wantExcused) {
				t.Errorf("excused = %v, want %v", got, tc.wantExcused)
			}
			if got := skipTests(verdict.undeclared); !slices.Equal(got, tc.wantUndeclared) {
				t.Errorf("undeclared = %v, want %v", got, tc.wantUndeclared)
			}
			if len(verdict.stale) != 0 || !verdict.failed() {
				t.Errorf("stale = %v, failed = %t; want no stale declaration and a failed run", verdict.stale, verdict.failed())
			}
		})
	}
}

// TestJudgeSkips_Declarations verifies the two directions a declaration table
// is held to: a skip it does not cover fails, and so does a declaration no
// skip of its runtime needs, a duplicate included.
func TestJudgeSkips_Declarations(t *testing.T) {
	declared := recordedSkip{Package: "common", Test: savedViewLifecycle + "/meta", Reason: savedViewRefusal("1ms")}
	cases := []struct {
		name           string
		runtime        string
		skips          []recordedSkip
		declarations   []skipDeclaration
		wantExcused    []string
		wantUndeclared []string
		wantStale      []string
	}{
		{
			name:         "a run whose every skip is declared passes",
			runtime:      "ce",
			skips:        []recordedSkip{declared},
			declarations: recordedRunDeclarations,
			wantExcused:  []string{"gitlab-defect " + savedViewLifecycle + "/meta"},
		},
		{
			name:         "a run with no skip leaves its runtime's declarations stale and not the other's",
			runtime:      "ee",
			declarations: recordedRunDeclarations,
			wantStale:    []string{"ee " + savedViewLifecycle},
		},
		{
			name:    "a declared test skipping for another reason is undeclared, and its declaration stale",
			runtime: "ce",
			skips: []recordedSkip{{
				Package: "common", Test: savedViewLifecycle + "/dynamic", Reason: "no runner is registered",
			}},
			declarations:   recordedRunDeclarations,
			wantUndeclared: []string{savedViewLifecycle + "/dynamic"},
			wantStale:      []string{"ce " + savedViewLifecycle},
		},
		{
			name:         "a second declaration of one skip is stale",
			runtime:      "ce",
			skips:        []recordedSkip{declared},
			declarations: append(slices.Clone(recordedRunDeclarations), recordedRunDeclarations[0]),
			wantExcused:  []string{"gitlab-defect " + savedViewLifecycle + "/meta"},
			wantStale:    []string{"ce " + savedViewLifecycle},
		},
		{
			name:           "no table declares everything undeclared",
			runtime:        "ce",
			skips:          []recordedSkip{declared},
			wantUndeclared: []string{savedViewLifecycle + "/meta"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := judgeSkips(tc.runtime, tc.skips, tc.declarations)
			if got := excusedTests(verdict.excused); !slices.Equal(got, nonNil(tc.wantExcused)) {
				t.Errorf("excused = %v, want %v", got, tc.wantExcused)
			}
			if got := skipTests(verdict.undeclared); !slices.Equal(got, nonNil(tc.wantUndeclared)) {
				t.Errorf("undeclared = %v, want %v", got, tc.wantUndeclared)
			}
			if got := staleTests(verdict.stale); !slices.Equal(got, nonNil(tc.wantStale)) {
				t.Errorf("stale = %v, want %v", got, tc.wantStale)
			}
			wantFailed := len(tc.wantUndeclared) > 0 || len(tc.wantStale) > 0
			if verdict.failed() != wantFailed {
				t.Errorf("failed() = %t, want %t", verdict.failed(), wantFailed)
			}
		})
	}
}

// nonNil turns a nil list into an empty one, which is what the name helpers
// return for a verdict with nothing in a list.
func nonNil(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// TestSkipVerdict_Failed_EitherListFailsTheRun verifies that an undeclared skip
// alone fails a run, and a stale declaration alone does too.
func TestSkipVerdict_Failed_EitherListFailsTheRun(t *testing.T) {
	cases := []struct {
		name    string
		verdict skipVerdict
		want    bool
	}{
		{name: "nothing", verdict: skipVerdict{}, want: false},
		{name: "only declared skips", verdict: skipVerdict{excused: []excusedSkip{{}}}, want: false},
		{name: "an undeclared skip", verdict: skipVerdict{undeclared: []recordedSkip{{}}}, want: true},
		{name: "a stale declaration", verdict: skipVerdict{stale: []skipDeclaration{{}}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.verdict.failed(); got != tc.want {
				t.Errorf("failed() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestRecordedSkip_ReasonText_SaysWhenThereIsNone verifies that a skip with no
// reason prints that it has none rather than an empty field.
func TestRecordedSkip_ReasonText_SaysWhenThereIsNone(t *testing.T) {
	if got := (recordedSkip{}).reasonText(); got != "(no reason printed)" {
		t.Errorf("reasonText() = %q for no reason", got)
	}
	if got := (recordedSkip{Reason: "why"}).reasonText(); got != "why" {
		t.Errorf("reasonText() = %q, want the reason", got)
	}
}

// runCheckSkipsFor runs -check-skips alone over a stream and a table.
func runCheckSkipsFor(results, runtime string, declarations []skipDeclaration) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = runCheckSkips(options{results: results, runtime: runtime, skipDeclarations: declarations}, &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestRunCheckSkips_RecordedCERun_NamesEveryUndeclaredSkip verifies what a
// complete run that ended as the recorded CE run did prints: each declared
// skip on stdout with its category, each undeclared one on stderr with its
// reason, and a summary, and that it fails.
func TestRunCheckSkips_RecordedCERun_NamesEveryUndeclaredSkip(t *testing.T) {
	code, stdout, stderr := runCheckSkipsFor(skipsFixture("ce"), "ce", recordedRunDeclarations)

	if code != exitFindings {
		t.Errorf("runCheckSkips() = %d, want %d", code, exitFindings)
	}
	wantStdout := "skips: declared (gitlab-defect): common " + savedViewLifecycle + "/dynamic: " + savedViewRefusal("155ms") + "\n" +
		"skips: declared (gitlab-defect): common " + savedViewLifecycle + "/individual: " + savedViewRefusal("134ms") + "\n" +
		"skips: declared (gitlab-defect): common " + savedViewLifecycle + "/meta: " + savedViewRefusal("122ms") + "\n" +
		"skips: 11 skipped on ce, 3 declared, 8 undeclared, 0 stale declarations\n"
	if stdout != wantStdout {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout, wantStdout)
	}
	wantStderr := "skips: undeclared: common TestAdmin_ExternalImporters/BitbucketCloudImport: " + reasonNoExternalNetwork + "\n" +
		"skips: undeclared: common TestAdmin_ExternalImporters/GistsImport: " + reasonNoExternalNetwork + "\n" +
		"skips: undeclared: common TestAdmin_ExternalImporters/GitHubImportAndCancel: " + reasonNoExternalNetwork + "\n" +
		"skips: undeclared: common TestBitbucketCloudImport_Individual: " + reasonNoExternalNetwork + "\n" +
		"skips: undeclared: common TestGroupServiceAccounts_Get_ReadsTheAccountBackByID: " + needsRelease("19.3.0") + "\n" +
		"skips: undeclared: common TestProjectServiceAccounts_Get_ReadsTheAccountBackByID: " + needsRelease("19.3.0") + "\n" +
		"skips: undeclared: common TestTierPin_DetectionAgreesWithTheRuntimeProbe: " + reasonTierFallback + "\n" +
		"skips: undeclared: common TestWorkItems_Lifecycle_IssueTypeCreateListGetUpdateDelete: " + reasonWorkItemWidgets + "\n"
	if stderr != wantStderr {
		t.Errorf("stderr =\n%s\nwant\n%s", stderr, wantStderr)
	}
}

// TestRunCheckSkips_Outcomes verifies the rest of what the mode can end with:
// a run the table covers passes, a stale declaration fails and is named, a
// skip with no reason says so, and each thing the mode cannot run without is
// a usage error that names it.
func TestRunCheckSkips_Outcomes(t *testing.T) {
	dir := t.TempDir()
	covered := filepath.Join(dir, "covered.json")
	writeFile(t, covered, `{"Action":"output","Package":"m/test/e2e/gitlab/common","Test":"`+savedViewLifecycle+`/meta","Output":"    workitemsavedviews_test.go:77: `+savedViewRefusal("9ms")+`\n"}
{"Action":"skip","Package":"m/test/e2e/gitlab/common","Test":"`+savedViewLifecycle+`/meta"}
{"Action":"pass","Package":"m/test/e2e/gitlab/common","Test":"TestOther"}
`)
	bare := filepath.Join(dir, "bare.json")
	writeFile(t, bare, `{"Action":"skip","Package":"m/test/e2e/gitlab/ee","Test":"TestBare"}`+"\n")
	notJSON := filepath.Join(dir, "not.json")
	writeFile(t, notJSON, "PASS\n")

	cases := []struct {
		name         string
		results      string
		runtime      string
		declarations []skipDeclaration
		want         int
		wantStdout   string
		wantStderr   string
	}{
		{
			name: "a run whose every skip is declared passes", results: covered, runtime: "ce",
			declarations: recordedRunDeclarations, want: exitOK,
			wantStdout: "skips: declared (gitlab-defect): common " + savedViewLifecycle + "/meta: " + savedViewRefusal("9ms") + "\n" +
				"skips: 1 skipped on ce, 1 declared, 0 undeclared, 0 stale declarations\n",
		},
		{
			name: "a declaration no skip matches fails and is named", results: covered, runtime: "ee",
			declarations: append(slices.Clone(recordedRunDeclarations), skipDeclaration{
				Runtime: "ee", Package: "common", Test: "TestAdmin_ExternalImporters/BitbucketServerImport",
				Because: "BITBUCKET_SERVER_URL is not set", Category: skipCategoryGitLabDefect, Reason: "gone",
			}),
			want: exitFindings,
			wantStdout: "skips: declared (gitlab-defect): common " + savedViewLifecycle + "/meta: " + savedViewRefusal("9ms") + "\n" +
				"skips: 1 skipped on ee, 1 declared, 0 undeclared, 1 stale declarations\n",
			wantStderr: "skips: stale declaration: ee common TestAdmin_ExternalImporters/BitbucketServerImport (because \"BITBUCKET_SERVER_URL is not set\"): no skip of this run matches it\n",
		},
		{
			name: "a skip with no reason says it printed none", results: bare, runtime: "ee", want: exitFindings,
			wantStdout: "skips: 1 skipped on ee, 0 declared, 1 undeclared, 0 stale declarations\n",
			wantStderr: "skips: undeclared: ee TestBare: (no reason printed)\n",
		},
		{
			name: "no runtime is a usage error", results: covered, want: exitUsage,
			wantStderr: "audit_e2e_coverage: -check-skips needs -runtime ce or ee, the run whose skips it judges, and was given \"\"\n",
		},
		{
			name: "a runtime selector of the coverage report is not a run", results: covered, runtime: "community/free", want: exitUsage,
			wantStderr: "audit_e2e_coverage: -check-skips needs -runtime ce or ee, the run whose skips it judges, and was given \"community/free\"\n",
		},
		{
			name: "no results is a usage error", runtime: "ce", want: exitUsage,
			wantStderr: "audit_e2e_coverage: -check-skips needs -results, the go test -json stream of that run\n",
		},
		{
			name: "a stream that cannot be read is a usage error", results: notJSON, runtime: "ce", want: exitUsage,
			wantStderr: "audit_e2e_coverage: skips: results line 1: invalid character 'P' looking for beginning of value\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCheckSkipsFor(tc.results, tc.runtime, tc.declarations)
			if code != tc.want {
				t.Errorf("runCheckSkips() = %d, want %d", code, tc.want)
			}
			if tc.wantStdout != "" && stdout != tc.wantStdout {
				t.Errorf("stdout =\n%q\nwant\n%q", stdout, tc.wantStdout)
			}
			if stderr != tc.wantStderr {
				t.Errorf("stderr =\n%q\nwant\n%q", stderr, tc.wantStderr)
			}
		})
	}
}

// TestRun_CheckSkips_IsAModeOfItsOwn verifies that -check-skips passes the
// "nothing to do" guard with no -calls, since the stream is all it reads, and
// that its status is the run's.
func TestRun_CheckSkips_IsAModeOfItsOwn(t *testing.T) {
	code, stdout, stderr := runFixture(t, options{
		dir: t.TempDir(), checkSkips: true, results: skipsFixture("ce"), runtime: "ce",
		skipDeclarations: recordedRunDeclarations,
	})
	if code != exitFindings || strings.Contains(stderr, "nothing to do") {
		t.Errorf("run(-check-skips) = %d, stderr %q; want %d from the gate itself", code, stderr, exitFindings)
	}
	if !strings.Contains(stdout, "skips: 11 skipped on ce, 3 declared, 8 undeclared, 0 stale declarations\n") {
		t.Errorf("stdout = %q, want the gate's summary", stdout)
	}
}
