// The tests are an external package on purpose: export_test.go says why.
package serialtypecheck_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck"
)

// goPackagesPath is the loader whose semaphore this package sizes.
const goPackagesPath = "golang.org/x/tools/go/packages"

// packageName is the name every file of the package under test declares, as
// opposed to this external test package's.
const packageName = "serialtypecheck"

// analysisTags are the build tags the consumer guard lists the module under:
// GO_ANALYSIS_TAGS in the Makefile, the set golangci-lint reads, so that a
// test binary that exists only behind one of the end-to-end tags is held to
// the rule too. No file in the module is constrained on the negation of one
// of these tags, so a listing under all of them sees every file the untagged
// build sees.
const analysisTags = "e2e,collectore2e,httpe2e,orbitlive,stdioe2e"

// companyWait is how long the first file the probe sees being parsed waits for
// a second one to start alongside it. A loader that admits two parses at once
// lets the second in within microseconds, so the bound only decides how long a
// serial loader keeps the test waiting before the probe concludes it is
// serial, which it is once per run.
const companyWait = 2 * time.Second

// setProcessors sets GOMAXPROCS for the length of one test and puts back the
// value it found, since the setting belongs to the whole process.
func setProcessors(t *testing.T, n int) {
	t.Helper()
	previous := runtime.GOMAXPROCS(n)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
}

// TestSerializeUnderRace_RaceBuild_LeavesOneProcessor verifies the half a race
// build takes: whatever the process had, it is left with one processor, and
// the value returned is that one, which is what the loader will read.
func TestSerializeUnderRace_RaceBuild_LeavesOneProcessor(t *testing.T) {
	setProcessors(t, 3)

	if got := serialtypecheck.SerializeUnderRace(true); got != 1 {
		t.Errorf("serializeUnderRace(true) = %d, want 1: the loader sizes its semaphore from this value", got)
	}
	if got := runtime.GOMAXPROCS(0); got != 1 {
		t.Errorf("GOMAXPROCS after serializeUnderRace(true) = %d, want 1", got)
	}
}

// TestSerializeUnderRace_OrdinaryBuild_LeavesTheSettingAlone verifies the half
// every other build takes: the setting is read and not changed, so an ordinary
// test binary that links this package runs exactly as it did before.
func TestSerializeUnderRace_OrdinaryBuild_LeavesTheSettingAlone(t *testing.T) {
	setProcessors(t, 3)

	if got := serialtypecheck.SerializeUnderRace(false); got != 3 {
		t.Errorf("serializeUnderRace(false) = %d, want the 3 the process had", got)
	}
	if got := runtime.GOMAXPROCS(0); got != 3 {
		t.Errorf("GOMAXPROCS after serializeUnderRace(false) = %d, want it left at 3", got)
	}
}

// TestRaceEnabled_MatchesTheBuild holds the seam to the build it was compiled
// into, read from the binary's own build information, where the go command
// records -race=true for a race build and nothing for any other.
//
// The probe below cannot catch a seam that answers wrong: it takes the seam's
// word for which build it is in, so a race.go that said false would make the
// package do nothing under the detector and the probe expect exactly that.
func TestRaceEnabled_MatchesTheBuild(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("this test binary carries no build information to compare the seam with")
	}
	built := false
	for _, setting := range info.Settings {
		if setting.Key == "-race" {
			built = setting.Value == "true"
		}
	}
	if serialtypecheck.RaceEnabled != built {
		t.Errorf("raceEnabled = %t in a binary built with -race=%t: the seam files disagree with their "+
			"build constraints", serialtypecheck.RaceEnabled, built)
	}
}

// TestLoad_ParsingSemaphore_AdmitsWhatThisPackageSet holds go/packages to the
// value this package's initializer left, in the binary the initializer ran in.
//
// The semaphore is unexported, so the probe watches it from outside: the
// loader calls Config.ParseFile while holding one of its tokens, and the first
// file to arrive waits there for a second. A loader whose semaphore admits two
// lets one in and the peak reaches two; a loader sized at one makes every
// other file wait for the token, and the peak stays at one. In a race build
// that is the claim the package documentation rests on, and it fails if
// x/tools stops sizing the semaphore when it is initialized, or if this
// package ever stops being initialized first. It has done so once already:
// its first version sat in the package itself, whose import of go/packages
// made the package initialize after the loader in this binary, and the race
// build answered two. In an ordinary build it shows the probe can see
// parallelism at all, which is what keeps the race build's answer from being
// true of a probe that sees nothing.
//
// Parsing is what the probe can observe; type-checking takes a token from the
// same semaphore in the same function, and a change that gave it one of its
// own is a change the probe would not see.
func TestLoad_ParsingSemaphore_AdmitsWhatThisPackageSet(t *testing.T) {
	processors := serialtypecheck.Processors
	if serialtypecheck.RaceEnabled && processors != 1 {
		t.Fatalf("processors = %d in a race build, want 1", processors)
	}

	var (
		parsed, active, peak atomic.Int32
		waited               atomic.Bool
		joined               sync.Once
		company              = make(chan struct{})
	)
	cfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
		Context: t.Context(),
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			parsed.Add(1)
			now := active.Add(1)
			defer active.Add(-1)
			raiseTo(&peak, now)
			if now > 1 {
				joined.Do(func() { close(company) })
			}
			if waited.CompareAndSwap(false, true) {
				select {
				case <-company:
				case <-time.After(companyWait):
				}
			}
			return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
		},
	}
	loaded, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("load this package: %v", err)
	}
	if len(loaded) != 1 || len(loaded[0].Errors) > 0 {
		t.Fatalf("load this package: got %d package(s), errors %v", len(loaded), packageErrors(loaded))
	}
	if parsed.Load() < 2 {
		t.Fatalf("the loader parsed %d file(s); the probe needs two to tell one at a time from more", parsed.Load())
	}

	got := peak.Load()
	switch {
	case processors == 1 && got != 1:
		t.Errorf("go/packages parsed %d files at once with its semaphore meant to be sized at one: "+
			"it was not sized from the value this package set, so type-checking is parallel again "+
			"and golang/go#81122 can fail a race run (a file of this package importing the loader "+
			"is one way to get here)", got)
	case processors > 1 && got < 2:
		t.Errorf("go/packages parsed one file at a time with GOMAXPROCS at %d when this package was "+
			"initialized: either the loader no longer parses in parallel, and this probe can no longer "+
			"tell a serial loader from a parallel one, or its semaphore is sized from something else",
			processors)
	}
}

// raiseTo records n in peak if it is higher than what peak holds.
func raiseTo(peak *atomic.Int32, n int32) {
	for {
		seen := peak.Load()
		if n <= seen || peak.CompareAndSwap(seen, n) {
			return
		}
	}
}

// packageErrors lists every error the loaded packages carry, for a failure
// message.
func packageErrors(loaded []*packages.Package) []packages.Error {
	var errs []packages.Error
	for _, pkg := range loaded {
		errs = append(errs, pkg.Errors...)
	}
	return errs
}

// TestInitialization_PrecedesGoPackages_InEveryBinary holds the two facts that
// make this package's initializer run before go/packages' in any binary that
// links both, rather than only in this package's own test binary where the
// probe above watches it.
//
// The specification initializes, at each step, the first package in import
// path order whose imports are all initialized. When go/packages becomes
// ready, everything it imports has been initialized; if everything this
// package imports is among those, this package is ready at the same step or
// earlier, and if its path sorts first it is chosen first. An import outside
// that set, or a move to a path that sorts after golang.org, would leave the
// order to whatever else a binary links, and the race back in some of them.
// The imports read are those of every file compiled into the package,
// internal test files included, since in this package's own test binary they
// are part of it.
func TestInitialization_PrecedesGoPackages_InEveryBinary(t *testing.T) {
	self := strings.TrimSpace(goOutput(t, ".", "list", "-f", "{{.ImportPath}}", "."))
	if self >= goPackagesPath {
		t.Errorf("this package's import path %q does not sort before %q, so the specification no longer "+
			"initializes it first", self, goPackagesPath)
	}

	deps := map[string]bool{}
	for line := range strings.Lines(goOutput(t, ".", "list", "-deps", goPackagesPath)) {
		deps[strings.TrimSpace(line)] = true
	}
	if !deps["go/types"] || !deps[goPackagesPath] {
		t.Fatalf("go list -deps %s named %d packages and not go/types and the loader itself, so it is not "+
			"listing the loader's dependencies", goPackagesPath, len(deps))
	}

	var outside []string
	for _, imported := range importsOfThePackage(t, ".") {
		if !deps[imported] {
			outside = append(outside, imported)
		}
	}
	if len(outside) > 0 {
		t.Errorf("this package imports %s, which %s does not depend on: a binary could initialize the loader "+
			"before this package, whatever its path", strings.Join(outside, ", "), goPackagesPath)
	}
}

// importsOfThePackage returns the import paths of every Go file in dir that
// declares the package under test, its internal test files included and this
// external test package's files left out. They are read from the source
// rather than from a build, because the race and ordinary halves of the seam
// are never compiled together and either could import something the other
// does not.
func importsOfThePackage(t *testing.T, dir string) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	var imports []string
	for _, name := range names {
		file, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		if file.Name.Name != packageName {
			continue
		}
		for _, spec := range file.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("import in %s: %v", name, unquoteErr)
			}
			if !slices.Contains(imports, path) {
				imports = append(imports, path)
			}
		}
	}
	if len(imports) == 0 {
		t.Fatalf("no file in %s imports anything, which leaves nothing to set GOMAXPROCS with", dir)
	}
	return imports
}

// TestConsumers_EveryTestBinaryLinkingGoPackages_LinksThisPackage is the gate
// that keeps the workaround where the race is.
//
// The rule is the test binary rather than the test: whether a test reaches a
// load that type-checks two packages from source is not something a listing
// can decide, while whether its binary links go/packages is, and the semaphore
// is one per binary. When this was written, 20 of the 22 other test binaries
// that link go/packages were measured type-checking at least two packages
// from source in one load, and the remaining two link it without any of their
// tests loading; for them the rule costs a race build that runs on one
// processor, and it holds them the day one of their tests starts loading.
func TestConsumers_EveryTestBinaryLinkingGoPackages_LinksThisPackage(t *testing.T) {
	self := strings.TrimSpace(goOutput(t, ".", "list", "-f", "{{.ImportPath}}", "."))
	listing := goOutput(t, moduleRoot(t), "list", "-test", "-tags="+analysisTags,
		"-f", "{{.ImportPath}}\t{{join .Deps \",\"}}", "./...")

	var linking, missing []string
	for line := range strings.Lines(listing) {
		binary, depList, _ := strings.Cut(strings.TrimSpace(line), "\t")
		// The test main of package p is listed as p.test; p's own variants
		// carry the binary in brackets and are not binaries.
		if !strings.HasSuffix(binary, ".test") || strings.Contains(binary, " ") {
			continue
		}
		deps := map[string]bool{}
		for dep := range strings.SplitSeq(depList, ",") {
			// A dependency recompiled for this binary is listed with the
			// binary in brackets after its path, which is how this package
			// appears in its own test binary; the path is what counts.
			path, _, _ := strings.Cut(dep, " [")
			deps[path] = true
		}
		if !deps[goPackagesPath] {
			continue
		}
		linking = append(linking, strings.TrimSuffix(binary, ".test"))
		if !deps[self] {
			missing = append(missing, strings.TrimSuffix(binary, ".test"))
		}
	}

	if !slices.Contains(linking, self) {
		t.Fatalf("the listing found %d test binaries linking %s and not this package's own, so it is not "+
			"listing this module's tests", len(linking), goPackagesPath)
	}
	if len(missing) > 0 {
		t.Errorf("%d test binaries link %s and not %s, so a race build of them type-checks in parallel and "+
			"golang/go#81122 can fail it at random. Add the blank import to one _test.go file of each:\n  %s",
			len(missing), goPackagesPath, self, strings.Join(missing, "\n  "))
	}
}

// goOutput runs the go command in dir and returns what it wrote to standard
// output, failing the test with its standard error when it does not succeed.
func goOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out)
}

// moduleRoot walks up from the package directory to the go.mod above it, which
// is where the module-wide listing has to run from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}
