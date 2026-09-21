package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fixtureWithADeadConstant is the smallest package this rule has anything to
// say about: one constant read, one not.
const fixtureWithADeadConstant = `package fixture

const (
	usedConst = "used"
	deadConst = "dead"
)

func Live() string { return usedConst }
`

// runFixture runs the command over an in-memory fixture package and returns
// the exit code with what each stream said.
func runFixture(t *testing.T, source string, check, verbose bool) (int, string, string) {
	t.Helper()
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(source),
	}
	var stdout, stderr strings.Builder
	code := run(auditConfig{
		dir:      root,
		patterns: []string{"./" + fixtureDir},
		overlay:  overlay,
		verbose:  verbose,
		out:      &stdout,
	}, check, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestRun_FindingWithoutCheck_ReportsAndSucceeds: the report is worth having
// without the gate, so a plain run prints the finding and exits 0. The
// summary is held whole, because every figure on it is read off the scan:
// both constants declared means every name in the group was recorded, and
// one package means the package was counted once.
func TestRun_FindingWithoutCheck_ReportsAndSucceeds(t *testing.T) {
	code, stdout, stderr := runFixture(t, fixtureWithADeadConstant, false, false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 without -check", code)
	}
	want := fixtureDir + "/fixture.go:5: deadConst is never read (1 of 2 in its const declaration)\n" +
		"\n" +
		"audit_dead_consts: 2 unexported constants in 1 packages, 1 never read (1 of those in a group the linter cannot see)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: a finding is not a broken run", stderr)
	}
}

// TestRun_FindingUnderCheck_Fails is the gate.
func TestRun_FindingUnderCheck_Fails(t *testing.T) {
	code, stdout, stderr := runFixture(t, fixtureWithADeadConstant, true, false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 under -check", code)
	}
	if !strings.Contains(stdout, "deadConst is never read") {
		t.Fatalf("stdout does not name the finding:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: the finding goes to stdout", stderr)
	}
}

// TestRun_NothingToReport_SucceedsUnderCheck. The output is held whole: a
// quiet clean run is the one summary line and nothing before it, which is
// what tells the gate flag apart from the verbose one.
func TestRun_NothingToReport_SucceedsUnderCheck(t *testing.T) {
	code, stdout, stderr := runFixture(t, `package fixture

const usedConst = "used"

func Live() string { return usedConst }
`, true, false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	want := "audit_dead_consts: 1 unexported constants in 1 packages, 0 never read (0 of those in a group the linter cannot see)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

// TestRun_SourceThatDoesNotTypeCheck_FailsOnStderr: a package that did not
// type-check folds nothing, so a report over it would be a clean answer about
// source nobody understood. [goprogram.Load] refuses it and this says so.
func TestRun_SourceThatDoesNotTypeCheck_FailsOnStderr(t *testing.T) {
	code, _, stderr := runFixture(t, `package fixture

func Live() string { return undefinedSymbol }
`, false, false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, toolName) {
		t.Fatalf("stderr does not name the tool:\n%s", stderr)
	}
}

// TestRun_VerboseCleanRun_SetsTheSummaryApart: in verbose mode the progress
// lines and the report share one stream, so the summary is set apart by a
// blank line even when there is no finding above it to do that. The run is
// verbose and not gated, the reverse of the quiet gated run beside it, so
// the two flags cannot stand in for each other.
func TestRun_VerboseCleanRun_SetsTheSummaryApart(t *testing.T) {
	code, stdout, stderr := runFixture(t, `package fixture

const usedConst = "used"

func Live() string { return usedConst }
`, false, true)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	want := "\naudit_dead_consts: 1 unexported constants in 1 packages, 0 never read (0 of those in a group the linter cannot see)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

// TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading: the root is made
// absolute before anything is loaded, so a run whose working directory is
// gone stops on that error instead of resolving the patterns against
// wherever the process happens to be.
//
// Only Linux can be made to fail that way. Windows refuses to remove a
// process's working directory, and macOS keeps answering getcwd from the
// path it remembers, so on both the premise cannot be set up and the case is
// skipped rather than reported as a defect in the operating system.
func TestAudit_RootThatCannotBeResolved_FailsRatherThanLoading(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Skipf("this platform will not remove the working directory, so filepath.Abs cannot be made to fail here: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed, so filepath.Abs cannot fail here")
	}

	report, err := audit(auditConfig{dir: ".", patterns: []string{"./..."}})
	if err == nil {
		t.Fatalf("audit error = nil, want the unresolved root; report = %+v", report)
	}
}

// TestReadOtherPlatforms_VerboseWithNoProgressStream_StillReads keeps a
// verbose configuration that names no writer from being a panic: the
// re-read is made and the progress line has nowhere to go.
func TestReadOtherPlatforms_VerboseWithNoProgressStream_StillReads(t *testing.T) {
	root := repoRoot(t)
	found := newScanner(root)
	found.platformPackages["github.com/jmrplens/gitlab-mcp-server/v3/cmd/server"] = struct{}{}
	err := readOtherPlatforms(auditConfig{
		dir:     root,
		targets: []target{{runtime.GOOS, otherArch}},
		verbose: true,
	}, found)
	if err != nil {
		t.Fatalf("readOtherPlatforms: %v", err)
	}
	if len(found.declared) == 0 {
		t.Fatal("declared is empty, want the constants of cmd/server read under the other architecture")
	}
}

// TestRun_PatternMatchingNothing_FailsRatherThanPassing guards the silence a
// gate must not have: auditing no package is not a clean run.
func TestRun_PatternMatchingNothing_FailsRatherThanPassing(t *testing.T) {
	var stdout, stderr strings.Builder
	code := run(auditConfig{
		dir:      repoRoot(t),
		patterns: []string{"./cmd/audit_dead_consts/nothing-is-here/..."},
		out:      &stdout,
	}, true, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Fatal("stderr is empty, want the refusal")
	}
}

// TestReadOtherPlatforms_NoConstrainedPackage_LoadsNothingAgain keeps the
// extra loads measured rather than assumed.
func TestReadOtherPlatforms_NoConstrainedPackage_LoadsNothingAgain(t *testing.T) {
	found := newScanner(repoRoot(t))
	var out strings.Builder
	err := readOtherPlatforms(auditConfig{
		dir:     repoRoot(t),
		targets: []target{{"plan9", "amd64"}},
		verbose: true,
		out:     &out,
	}, found)
	if err != nil {
		t.Fatalf("readOtherPlatforms: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("out = %q, want nothing: no package had a file this platform left out", out.String())
	}
}

// TestReadOtherPlatforms_UnknownPlatform_FailsRatherThanPassingQuietly: a
// platform load that cannot be made leaves the uses of that platform's files
// unrecorded, which would turn into findings about live code.
func TestReadOtherPlatforms_UnknownPlatform_FailsRatherThanPassingQuietly(t *testing.T) {
	root := repoRoot(t)
	found := newScanner(root)
	found.platformPackages["github.com/jmrplens/gitlab-mcp-server/v3/cmd/server"] = struct{}{}
	var out strings.Builder
	err := readOtherPlatforms(auditConfig{
		dir:     root,
		targets: []target{{"notanoperatingsystem", runtime.GOARCH}},
		verbose: true,
		out:     &out,
	}, found)
	if err == nil {
		t.Fatal("readOtherPlatforms error = nil, want the load failure")
	}
	if !strings.Contains(err.Error(), "notanoperatingsystem/"+runtime.GOARCH) {
		t.Fatalf("error = %v, want it to name the target", err)
	}
	if !strings.Contains(out.String(), "re-reading 1 package(s) as notanoperatingsystem/"+runtime.GOARCH) {
		t.Fatalf("verbose output = %q, want the target load named", out.String())
	}
}

// TestReadOtherPlatforms_TheHostTarget_IsNotReadTwice: the exact pair the run
// is on was read by the first load, and only that pair. The host's operating
// system under another architecture is a different target and is read.
func TestReadOtherPlatforms_TheHostTarget_IsNotReadTwice(t *testing.T) {
	root := repoRoot(t)
	found := newScanner(root)
	found.platformPackages["github.com/jmrplens/gitlab-mcp-server/v3/cmd/server"] = struct{}{}
	var out strings.Builder
	if err := readOtherPlatforms(auditConfig{
		dir:     root,
		targets: []target{{runtime.GOOS, runtime.GOARCH}},
		verbose: true,
		out:     &out,
	}, found); err != nil {
		t.Fatalf("readOtherPlatforms: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("out = %q, want nothing: the first load already read this target", out.String())
	}
	if err := readOtherPlatforms(auditConfig{
		dir:     root,
		targets: []target{{runtime.GOOS, otherArch}},
		verbose: true,
		out:     &out,
	}, found); err != nil {
		t.Fatalf("readOtherPlatforms under %s/%s: %v", runtime.GOOS, otherArch, err)
	}
	if want := "re-reading 1 package(s) as " + runtime.GOOS + "/" + otherArch; !strings.Contains(out.String(), want) {
		t.Fatalf("verbose output = %q, want %q: another architecture is not the host", out.String(), want)
	}
}

// TestPatternsOrDefault_NoArguments_AuditsTheWholeRepository.
func TestPatternsOrDefault_NoArguments_AuditsTheWholeRepository(t *testing.T) {
	if got := patternsOrDefault(nil); !slices.Equal(got, defaultPatterns) {
		t.Fatalf("patternsOrDefault(nil) = %v, want %v", got, defaultPatterns)
	}
	named := []string{"./internal/tools/issues"}
	if got := patternsOrDefault(named); !slices.Equal(got, named) {
		t.Fatalf("patternsOrDefault(%v) = %v, want it unchanged", named, got)
	}
}

// TestSortedKeys_ASet_IsRenderedInOneOrder keeps a verbose report and the
// order of the platform loads the same from one run to the next.
func TestSortedKeys_ASet_IsRenderedInOneOrder(t *testing.T) {
	ordered := sortedKeys(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	if !slices.Equal(ordered, []string{"a", "b", "c"}) {
		t.Fatalf("sortedKeys = %v, want a b c", ordered)
	}
	if empty := sortedKeys(nil); len(empty) != 0 {
		t.Fatalf("sortedKeys(nil) = %v, want empty", empty)
	}
}

// TestDefaultPatterns_CoverTheRepositorysOwnSource states what the gate is
// over, so narrowing it is a visible change rather than a quiet one.
func TestDefaultPatterns_CoverTheRepositorysOwnSource(t *testing.T) {
	for _, want := range []string{"./internal/...", "./cmd/..."} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(defaultPatterns, want) {
				t.Fatalf("defaultPatterns = %v, want it to include %q", defaultPatterns, want)
			}
		})
	}
}

// TestBuildTargets_AreTheOnesThisProjectShips: a pair missing here is a
// package half nothing re-reads, and its constants read as dead. Both halves
// are held, since an operating system alone would keep the host's
// architecture and leave every `_arm64.go` file out on an amd64 runner.
func TestBuildTargets_AreTheOnesThisProjectShips(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			want := target{goos, goarch}
			t.Run(want.String(), func(t *testing.T) {
				if !slices.Contains(buildTargets, want) {
					t.Fatalf("buildTargets = %v, want it to include %s", buildTargets, want)
				}
			})
		}
	}
}
