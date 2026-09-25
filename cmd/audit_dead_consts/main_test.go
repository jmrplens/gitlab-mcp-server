package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"os"
	"os/exec"
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
	code, stdout, stderr := runFixture(t, `package fixture

func Live() string { return undefinedSymbol }
`, false, false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, toolName+": ") || !strings.Contains(stderr, "undefinedSymbol") {
		t.Fatalf("stderr = %q, want the tool naming the type error it stopped on", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want nothing: a run that could not be made reports nothing", stdout)
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
	// The load would fail here too, in a go command that cannot find its own
	// working directory, but it says so in text; only the resolution of the
	// root hands back the operating system's error itself.
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("audit error = %v, want the working directory's own not-exist error from resolving the root", err)
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
// gate must not have: auditing no package is not a clean run. The pattern
// names a directory that is not there, which the go command refuses itself;
// a directory that is there and holds no package is the other way to match
// nothing, and [TestMain_PatternMatchingNothing_FailsOnStandardError] drives
// that one.
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
	if !strings.Contains(stderr.String(), "pattern ./cmd/audit_dead_consts/nothing-is-here/...: ") {
		t.Fatalf("stderr = %q, want the refusal of the pattern naming a directory that is not there", stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want no report over no package", stdout.String())
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

// mainArgsEnv carries, as JSON, the arguments a child copy of this test binary
// hands main; its presence is what makes the copy a child.
const mainArgsEnv = "AUDIT_DEAD_CONSTS_MAIN_ARGS"

// TestMain_InAChildProcess_RunsTheCommand is where [runMain] starts a copy of
// this binary: there main can end the process with os.Exit, as it does when
// CI runs the gate, without ending the suite. Run by the suite it does nothing.
func TestMain_InAChildProcess_RunsTheCommand(t *testing.T) {
	encoded, isChild := os.LookupEnv(mainArgsEnv)
	if !isChild {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(encoded), &args); err != nil {
		t.Fatalf("decode %s: %v", mainArgsEnv, err)
	}
	os.Args = append([]string{toolName}, args...)
	flag.CommandLine = flag.NewFlagSet(toolName, flag.ExitOnError)
	main()
}

// runMain runs main in a child copy of this test binary, since main ends in
// os.Exit, and returns the code the child exited with and what it wrote to
// each stream.
//
// The child is given a GOCOVERDIR of its own. A binary built for a coverage
// run writes its counters when it exits and warns on stderr when it has
// nowhere to write them, which would read as main having written there.
func runMain(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encode the arguments: %v", err)
	}
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMain_InAChildProcess_RunsTheCommand$")
	child.Env = append(os.Environ(), mainArgsEnv+"="+string(encoded), "GOCOVERDIR="+t.TempDir(), "GOWORK=off")
	var out, errOut strings.Builder
	child.Stdout, child.Stderr = &out, &errOut
	if runErr := child.Run(); runErr != nil {
		exitErr, exited := errors.AsType[*exec.ExitError](runErr)
		if !exited {
			t.Fatalf("run main in a child: %v", runErr)
		}
		code = exitErr.ExitCode()
	}
	return code, out.String(), errOut.String()
}

// mainModule writes a module for main to audit, since main takes no overlay
// and a test must not plant Go source in this repository. Its packages sit
// where the default patterns look and where they do not: one under internal
// declaring an unread constant in a group beside one read only by a file for
// another platform, one under cmd whose one constant is read, and one under
// neither with an unread constant of its own.
//
// The directory is resolved through its links first, since the go command
// reports the files it loads under their real path and a finding is named
// relative to the root as the command resolved it.
func mainModule(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve the module directory: %v", err)
	}
	platformFile := "fixture_" + otherPlatform + ".go"
	files := map[string]string{
		"go.mod": "module example.com/deadconsts\n\ngo 1.22\n",
		"internal/fixture/fixture.go": `package fixture

const (
	usedConst     = "used"
	deadConst     = "dead"
	platformConst = "read only where the constraint holds"
)

func Live() string { return usedConst }
`,
		"internal/fixture/" + platformFile: constrainedFixture(otherPlatform)[platformFile],
		"cmd/tool/main.go": `package main

const greeting = "read"

func main() { println(greeting) }
`,
		"other/other.go": `package other

const otherDead = "outside the default patterns"
`,
	}
	for name, source := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create the directory of %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// TestMain_VerboseWithoutCheck_ReportsEveryPlatformAndSucceeds is main as a
// reader runs it with -v. It holds what main hands the run, since no test of
// run can: the default patterns when none is named, the project's own build
// targets, the verbose flag and not the gate flag, and standard output for
// both the progress and the report. The module holds one package with a file
// for another platform, so the progress names one package under each target
// but the host, which is a different number from the targets it is read under;
// its one unread constant is reported and the run still exits 0; and neither
// the constant read only by the other platform's file nor the one outside the
// default patterns is reported.
func TestMain_VerboseWithoutCheck_ReportsEveryPlatformAndSucceeds(t *testing.T) {
	code, stdout, stderr := runMain(t, "-v", "-dir", mainModule(t))
	var want strings.Builder
	host := target{goos: runtime.GOOS, goarch: runtime.GOARCH}
	for _, tgt := range buildTargets {
		if tgt != host {
			want.WriteString(toolName + ": re-reading 1 package(s) as " + tgt.String() + "\n")
		}
	}
	want.WriteString("internal/fixture/fixture.go:5: deadConst is never read (1 of 3 in its const declaration)\n" +
		"\n" +
		toolName + ": 4 unexported constants in 2 packages, 1 never read (1 of those in a group the linter cannot see)\n")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 without -check; stdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stdout != want.String() {
		t.Fatalf("stdout = %q, want %q", stdout, want.String())
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: progress and findings are the report's", stderr)
	}
}

// TestMain_CheckOverANamedPackage_FailsOnItsFinding is the gate as CI runs
// it: -check, not verbose, over the patterns named on the command line
// rather than the defaults. The finding is on standard output and the exit
// code is 1; with no progress line, the report starts at the finding.
func TestMain_CheckOverANamedPackage_FailsOnItsFinding(t *testing.T) {
	code, stdout, stderr := runMain(t, "-check", "-dir", mainModule(t), "./other")
	want := "other/other.go:3: otherDead is never read (declared on its own)\n" +
		"\n" +
		toolName + ": 1 unexported constants in 1 packages, 1 never read (0 of those in a group the linter cannot see)\n"
	if code != 1 {
		t.Fatalf("exit = %d, want 1 under -check; stdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: a finding is not a broken run", stderr)
	}
}

// TestMain_PatternMatchingNothing_FailsOnStandardError holds the other
// stream: a run that could not be made says why on standard error, prints
// no report, and exits 1. The pattern names a directory that is there and
// holds no package, which the go command answers with an empty list rather
// than an error, so the refusal is the one that keeps a gate over nothing
// from passing.
func TestMain_PatternMatchingNothing_FailsOnStandardError(t *testing.T) {
	dir := mainModule(t)
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o750); err != nil {
		t.Fatalf("create the empty directory: %v", err)
	}
	code, stdout, stderr := runMain(t, "-check", "-dir", dir, "./empty/...")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr:\n%s", code, stderr)
	}
	if !strings.HasPrefix(stderr, toolName+": ") || !strings.Contains(stderr, "no packages matched ./empty/...") {
		t.Fatalf("stderr = %q, want the tool naming the pattern that matched nothing", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want no report over no package", stdout)
	}
}
