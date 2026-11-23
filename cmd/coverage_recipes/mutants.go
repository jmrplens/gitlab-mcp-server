// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// gremlinsProgram is the release of gremlins the recipe runs, whose reading of
// its flags, of a package and of its own coverage run the recipe is written
// against.
const gremlinsProgram = "github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0"

// trimpathFlags are the two flags every go command here, and gremlins with
// every command it runs, is given, so what the recipe measures and warms is
// what gremlins will find.
//
// -trimpath because gremlins copies the module into a directory per worker and
// runs each mutant's tests there, and without it cmd/go puts the package's
// directory into its compile key (buildActionID in
// cmd/go/internal/work/exec.go, Go 1.27.1). No compile done here was then of
// any use to a worker, and the first mutant on each one recompiled every
// package of this module its test imports inside its own deadline. That is
// what the 300 s default budget was sized to absorb, and a fixed coefficient
// passed through GREMLINS_FLAGS does not absorb it: under `--workers 2
// --timeout-coefficient 20` six packages of issue 971 reported twenty-one
// mutants TIMED OUT, every one among the first a worker compiled. Under
// -trimpath the directory is left out of the key.
//
// -count=1 because gremlins multiplies the coefficient by the wall time of its
// own coverage run, which Go's test cache would otherwise answer instantly for
// a package whose files have not changed since it last ran. A go command
// ignores a flag in GOFLAGS that it does not know, so the same setting reaches
// go list and go mod download harmlessly.
const trimpathFlags = "-trimpath -count=1"

// errInterrupted is what a call made after a signal returns: nothing is
// started once a run is ending.
var errInterrupted = errors.New("interrupted")

// mutation is one run of the mutation recipe: the package as the caller named
// it, the bounds and flags it runs under, the environment every program is
// given, the module's root, what go says of the package, and its staged copy.
type mutation struct {
	*session

	name    string
	knobs   knobs
	flags   gremlinsFlags
	environ []string
	goflags string
	goroot  string
	root    string
	pkg     golist.PackageFiles
	staged  staged
}

// mutants is the mutants subcommand: gremlins over one package, its verdicts
// made to mean what they say. It returns the status of the run.
func (s *session) mutants(ctx context.Context, args []string) int {
	if len(args) == 0 || len(args) > 4 || args[0] == "" {
		fmt.Fprintf(s.stderr, "%s: mutants takes a package, and a budget, a floor and a ceiling in seconds after it\n", toolName)
		return 2
	}
	m := &mutation{session: s, name: args[0], environ: s.environ}
	status, err := m.run(ctx, args[1:])
	if interrupted, hit := recipe.Status(ctx); hit {
		return interrupted
	}
	if err != nil {
		fmt.Fprintf(s.stderr, "gremlins: %v\n", err)
		return 1
	}
	return status
}

// sayf prints one message of the recipe's own to stdout.
func (m *mutation) sayf(format string, args ...any) {
	fmt.Fprintf(m.stdout, "gremlins: "+format+"\n", args...)
}

// run takes the run from its settings to gremlins' own status. An error is the
// reason nothing was measured.
func (m *mutation) run(ctx context.Context, args []string) (int, error) {
	if err := m.readSettings(args); err != nil {
		return 0, err
	}
	if err := m.load(ctx); err != nil {
		return 0, err
	}
	cleanup, err := m.stage(ctx)
	defer cleanup()
	if err != nil {
		return 0, err
	}
	given, err := m.measureDeadline(ctx)
	if err != nil {
		return 0, err
	}
	for _, notice := range given.notices {
		m.sayf("%s", notice)
	}
	return m.unleash(ctx, given)
}

// readSettings reads the knobs and GREMLINS_FLAGS, and says what decided where
// that is not what the caller wrote.
func (m *mutation) readSettings(args []string) error {
	read, notices, err := readKnobs(args, m.session.environ)
	for _, notice := range notices {
		m.sayf("%s", notice)
	}
	if err != nil {
		return err
	}
	m.knobs = read
	if m.flags, err = readGremlinsFlags(m.session.environ); err != nil {
		return err
	}
	if notice := m.flags.integrationNotice(m.session.environ); notice != "" {
		m.sayf("%s", notice)
	}
	return nil
}

// goCall is the go command with args, run in dir with the recipe's
// environment.
func (m *mutation) goCall(dir string, args ...string) recipe.Call {
	return recipe.Call{Program: m.Go, Args: args, Dir: dir, Env: m.environ, Stderr: m.stderr}
}

// exec runs a call, unless a signal is ending the run.
func (m *mutation) exec(ctx context.Context, call recipe.Call) (int, error) {
	if ctx.Err() != nil {
		return 0, errInterrupted
	}
	return m.Run(ctx, call)
}

// goOutput runs the go command in dir and returns what it wrote to its
// standard output. Its standard error is the caller's.
func (m *mutation) goOutput(ctx context.Context, dir string, args ...string) (string, error) {
	return recipe.Output(ctx, m.exec, m.goCall(dir, args...))
}

// load sets the environment every later program runs under, and reads the
// package, refusing one no mutant of which could be killed.
//
// -trimpath also drops the root the toolchain was installed at from the
// binaries it builds, and that root is where runtime.GOROOT, and go/build's
// Default.GOROOT after it, fall back to when GOROOT is not set. A test binary
// is started with the environment the go command was given, so without this a
// test that runs the go command out of cmd/internal/golist would exec "bin/go"
// and fail: cmd/gen_testing_docs, cmd/godoc_tool and cmd/internal/golist did.
// Setting the root the go command itself uses restores exactly the value an
// untrimmed build records.
//
// The package is loaded under gremlins' own tags, and -e keeps one go cannot
// load (every file behind a tag nobody passed, a path that is not there) from
// ending the run with go's own message, which does not say what to do about
// it.
func (m *mutation) load(ctx context.Context) error {
	own, _ := recipe.Getenv(m.environ, "GOFLAGS")
	m.goflags = strings.TrimSpace(own + " " + trimpathFlags)
	m.environ = append(slices.Clone(m.environ), "GOFLAGS="+m.goflags)
	goroot, err := m.goOutput(ctx, m.dir, "env", "GOROOT")
	if err != nil {
		return fmt.Errorf("go env failed: %w; refusing to measure", err)
	}
	m.goroot = strings.TrimSpace(goroot)
	m.environ = append(m.environ, "GOROOT="+m.goroot)
	root, err := m.goOutput(ctx, m.dir, "list", "-m", "-f", golist.ModuleFormat)
	if err != nil {
		return fmt.Errorf("go list -m failed: %w; refusing to measure", err)
	}
	m.root = strings.ReplaceAll(strings.TrimSpace(root), `\`, "/")
	out, err := m.goOutput(ctx, m.dir, slices.Concat([]string{"list", "-e"}, m.flags.tagArgs(), []string{"-f", golist.FilesFormat, m.name})...)
	if err != nil {
		return fmt.Errorf("go list could not list %s: %w; refusing to measure", m.name, err)
	}
	rows, err := golist.ParseFileRows([]byte(out))
	if err != nil {
		return fmt.Errorf("go list could not list %s: %w; refusing to measure", m.name, err)
	}
	if len(rows) != 1 {
		return fmt.Errorf("%s names %d packages, and one is measured at a time; refusing to measure", m.name, len(rows))
	}
	m.pkg = rows[0]
	return m.requireTests(ctx)
}

// requireTests refuses a package go cannot load, and one with no test file
// under its tags unless the run is an integration run with a -coverpkg that
// names the package, and the package is one another package's test can link.
//
// Each mutant runs only this package's tests unless --integration is given, so
// without it no mutant of such a package can be killed, and unless a -coverpkg
// names it nothing covers its blocks and every mutant is reported NOT COVERED:
// a -coverpkg naming only other packages leaves it exactly as uncovered as
// none. Before this check its baseline passed having run nothing, printed no
// duration, and handed gremlins a coefficient of 3001. Under both, the
// module's other tests cover its blocks and gremlins runs them against each
// mutant, so it is measured, and the run says so.
//
// That holds only for an importable package measured where it is. No other
// package's test can link a package main, and a package staged as a copy named
// <dir>.mutants-<name> is one nothing imports, whose files gremlins matches
// against the profile by their module-relative path, the copy's and never the
// original's. So either is refused whatever -i and -coverpkg say, since every
// mutant of it would be reported NOT COVERED.
func (m *mutation) requireTests(ctx context.Context) error {
	const tagHint = "gremlins: a package behind a build tag is measured with GREMLINS_FLAGS='--tags <tag>', which reaches this baseline and gremlins alike; a tag set only in a .gremlins.yaml reaches gremlins and not the baseline."
	const elsewhereHint = " One tested only from elsewhere in the module is measured with GREMLINS_FLAGS='-i --coverpkg <pattern>', a pattern that names it"
	if m.pkg.Error != "" {
		return fmt.Errorf("go cannot load %s under build tags %s: %s\n%s%s", m.name, m.flags.tagsShown(), m.pkg.Error, tagHint, elsewhereHint)
	}
	if len(m.pkg.TestGoFiles)+len(m.pkg.XTestGoFiles) > 0 {
		return nil
	}
	untested := fmt.Sprintf("%s has no test files under build tags %s", m.name, m.flags.tagsShown())
	if m.pkg.Name == "main" || needsStaging(m.pkg) {
		return fmt.Errorf("%s, and it is a package main or one measured through a staged copy, which no other package's test can link, so no -coverpkg covers it and every mutant would be reported NOT COVERED; refusing to measure\n%s", untested, tagHint)
	}
	if err := m.requireCoverage(ctx, untested); err != nil {
		return fmt.Errorf("%w\n%s%s", err, tagHint, elsewhereHint)
	}
	m.sayf("%s; measuring it through the module's other tests, which --integration runs against each mutant and -coverpkg %s lets cover it", untested, m.flags.coverpkg)
	return nil
}

// requireCoverage refuses an untested package the module's other tests would
// not cover or not be run against. The -coverpkg patterns are resolved the way
// gremlins' coverage run resolves them: a comma-separated list, from the
// module root, under the same tags. A pattern that matches nothing lists
// nothing, which is a no, and one go list cannot resolve at all (an empty
// entry in the list, or one go reads as a flag) is not the same answer and is
// not reported as one.
func (m *mutation) requireCoverage(ctx context.Context, untested string) error {
	if !m.flags.integration || m.flags.coverpkg == "" {
		return fmt.Errorf("%s, so no mutant of it could be killed: each mutant runs only this package's tests without --integration, and without a -coverpkg every one is reported NOT COVERED; refusing to measure", untested)
	}
	args := slices.Concat([]string{"list", "-e"}, m.flags.tagArgs(), []string{"-f", "{{.ImportPath}}"}, strings.Split(m.flags.coverpkg, ","))
	listed, err := m.goOutput(ctx, filepath.FromSlash(m.root), args...)
	if err != nil {
		return fmt.Errorf("go list could not resolve -coverpkg %s (its own message is above); refusing to measure", m.flags.coverpkg)
	}
	if !slices.Contains(strings.Fields(listed), m.pkg.ImportPath) {
		return fmt.Errorf("%s, and -coverpkg %s does not name %s, so nothing would cover its blocks and every mutant would be reported NOT COVERED; refusing to measure", untested, m.flags.coverpkg, m.pkg.ImportPath)
	}
	return nil
}

// measureDeadline runs the three runs of the package's own suite that decide
// how long each mutant is given, and whether its verdict would mean anything.
//
// The first run is the gate, and it is not timed. A package whose suite fails
// is refused rather than measured, because gremlins reads a failing test run
// as a killed mutant, so every mutant of it would read as killed. It also
// warms the build cache: gremlins downloads modules outside its own timer and
// times a coverage run whose compile the run before it already paid for, so
// the run timed here has to find the cache the way gremlins' will. Timing a
// first build instead makes the base too long on a package just edited, and
// the per-mutant deadline too short to run a mutant in.
//
// The last run is timed by the clock, because `go test`'s own summary line
// says nothing reliable about it: it reports the test binary's run without the
// build gremlins' figure includes, a -cover run ends in a coverage figure
// rather than a duration, and a package with no tests under the tags given
// prints `[no test files]` and exits 0. Reading that line, and falling back to
// a guess of 0.010s when it carried no duration, is what turned 114 seconds of
// harness tests into a 95-hour deadline for every mutant.
func (m *mutation) measureDeadline(ctx context.Context) (deadline, error) {
	// The patterns naming the package where it is, which is what a reader
	// reproducing a failure runs: a staged copy is gone by the time they
	// read it.
	shownScan, shownEach := m.patterns(m.pkg.Dir)
	dir := m.pkg.Dir
	if m.staged.dir != "" {
		dir = m.staged.dir
	}
	scan, each := m.patterns(dir)
	profile, err := os.CreateTemp("", "coverage-mutants-")
	if err != nil {
		return deadline{}, err
	}
	_ = profile.Close()
	defer func() { _ = os.Remove(profile.Name()) }()
	download := m.goCall(filepath.FromSlash(m.root), "mod", "download")
	download.Stdout = m.stdout
	status, err := m.exec(ctx, download)
	if err != nil {
		return deadline{}, fmt.Errorf("go mod download could not be started: %w; refusing to measure", err)
	}
	if status != 0 {
		return deadline{}, fmt.Errorf("go mod download exited %d; refusing to measure", status)
	}
	baseline := slices.Concat([]string{"test", "-count=1"}, m.flags.tagArgs(), m.coverArgs(), []string{"-cover", "-coverprofile", profile.Name(), scan})
	if !m.suitePasses(ctx, baseline) {
		return deadline{}, m.baselineFailure(shownScan)
	}
	if !m.suitePasses(ctx, m.eachMutant(each)) {
		return deadline{}, fmt.Errorf("%s passes its tests under -cover and fails go test -failfast %s, the command gremlins runs against each mutant, so every mutant would read as killed; refusing to measure\n%s", m.name, each, m.trimpathHint(shownEach))
	}
	started := m.now()
	passed := m.suitePasses(ctx, baseline)
	base := strconv.FormatFloat(m.now().Sub(started).Seconds(), 'f', 3, 64)
	if !passed {
		return deadline{}, fmt.Errorf("%s passed its tests and then failed them on the timed run, so a mutant's verdict would depend on which way the suite fell; refusing to measure", m.name)
	}
	if number(base) <= 0 {
		return deadline{}, fmt.Errorf("the timed baseline reads '%s', which is not a positive number of seconds; refusing to derive a deadline from it", base)
	}
	return deriveDeadline(m.name, base, m.flags.fixed, m.knobs)
}

// coverArgs is the -coverpkg as go test takes it.
func (m *mutation) coverArgs() []string {
	if m.flags.coverpkg == "" {
		return nil
	}
	return []string{"-coverpkg", m.flags.coverpkg}
}

// eachMutant is the command gremlins runs against each mutant, which the
// recipe runs once against the unmutated package: `go test [-tags T] -failfast
// <package>`, or `./...` from the root under --integration (getTestArgs in
// internal/engine/executor.go, v0.6.0; gremlins adds a -timeout, which decides
// nothing about a compile). gremlins names the package by its import path and
// runs it from the worker's copy of the target directory; the relative path
// from the root names the same package, and under -trimpath neither the
// working directory nor the copy's own location is part of any compile key, so
// this run leaves in the build cache every package the first mutant on each
// worker would otherwise compile. A caller who states their own
// --exclude-files may leave packages below the target to be mutated, each run
// as its own `go test <package>`, so the run covers the target's whole subtree
// then (patterns).
//
// The coverage run does not: it compiles the packages its pattern or its
// -coverpkg names with coverage counters, which a mutant's plain `go test`
// does not share, so a package below the target that the target imports would
// still be compiled inside the first mutant's deadline. Measured on six
// packages of issue 971 under `--workers 2 --timeout-coefficient 20`,
// -trimpath and this run together turned all twenty-one first-compile timeouts
// into kills with no other verdict moving.
//
// It is also a gate. gremlins reads a failing run of this command as a killed
// mutant, and it is not the command the coverage run is: a test that behaves
// differently when coverage is on passes one and fails the other.
func (m *mutation) eachMutant(each string) []string {
	return slices.Concat([]string{"test", "-count=1"}, m.flags.tagArgs(), []string{"-failfast", each})
}

// suitePasses runs a go test from the module root and reports whether it
// passed. What it printed is shown when it did not.
func (m *mutation) suitePasses(ctx context.Context, args []string) bool {
	var out bytes.Buffer
	call := m.goCall(filepath.FromSlash(m.root), args...)
	call.Stdout, call.Stderr = &out, &out
	status, err := m.exec(ctx, call)
	if err == nil && status == 0 {
		return true
	}
	if err != nil {
		fmt.Fprintf(&out, "gremlins: go test could not be started: %v\n", err)
	}
	if ctx.Err() == nil {
		_, _ = m.stderr.Write(out.Bytes())
	}
	return false
}

// baselineFailure is the refusal of a suite that fails the first run. What it
// does NOT do is paper over a staged run that cannot work: a copy that does
// not pass its own tests where it was staged stops the run rather than falling
// back to the unstaged one, because that run is the one that reports a clean
// package without measuring it. See issue 872.
func (m *mutation) baselineFailure(shownScan string) error {
	if m.staged.dir == "" {
		return fmt.Errorf("%s does not pass its own tests, or a package below it does not (the output above says which), so every mutant would read as killed; refusing to measure\n%s", m.name, m.trimpathHint(shownScan))
	}
	said := []string{
		fmt.Sprintf("the staged copy of %s does not pass its own tests there, so the verdicts would be about the staging rather than the package; refusing to measure", m.name),
		"gremlins: a test that reads its own directory or import path is the usual cause. Run it unstaged to see, and read issue 872 before trusting a clean figure from one.",
	}
	if m.staged.below {
		said = append(said, fmt.Sprintf("gremlins: the copy is one level below %s, so a test that climbs out of its directory with a relative path such as ../.. lands one level short of where it meant to; a test that finds such a file from the module root reads it from either place", m.name))
	}
	return errors.New(strings.Join(append(said, m.trimpathHint(shownScan)), "\n"))
}

// trimpathHint is said after either refusal of a suite that fails here, since
// -trimpath is a cause the output above cannot name: a test that finds its
// files through its own compiled source path (runtime.Caller) is handed a
// module-relative path under it rather than a directory, and fails here while
// passing a plain run. The command it offers is the one the recipe ran, with
// the GOFLAGS, GOROOT and tags it ran under, over the package where it is
// rather than over a staged copy that is removed before anyone reads this.
// Without GOROOT a test that runs the go command fails for another reason (see
// load).
func (m *mutation) trimpathHint(pattern string) string {
	tags := ""
	if m.flags.tags != "" {
		tags = " -tags " + m.flags.tags
	}
	return fmt.Sprintf("gremlins: every go command here runs under -trimpath (issue 1029), which hands a test that locates files through runtime.Caller a module-relative path; GOROOT=%s GOFLAGS='%s' go test%s %s, run from the module root, shows whether that is the cause",
		m.goroot, m.goflags, tags, pattern)
}

// memoryCeiling returns the ceiling on the address space of gremlins and what
// it starts, in bytes, 0 for none, and what is said about it. It is set for
// gremlins and not for the baseline, which runs the package's own code and
// nothing a mutant changed. A soft limit the run inherited at or under the
// ceiling is left alone, because setting the ceiling would raise it as far as
// the hard limit allows and so loosen what the caller set. Outside Linux the
// limit may be accepted and never enforced, which would make the line a claim
// nobody checked, so it is not set there at all.
func (m *mutation) memoryCeiling() (ceiling uint64, said string) {
	if m.knobs.memoryMax == 0 {
		return 0, ""
	}
	const unbounded = "a mutant that allocates without end runs until the host stops it"
	if m.goos != "linux" {
		return 0, fmt.Sprintf("MUTANT_MEMORY_MAX=%d is applied with RLIMIT_AS, which this recipe relies on Linux to enforce, so on %s %s", m.knobs.memoryMax, m.goos, unbounded)
	}
	held, _, err := m.limits()
	if err != nil {
		return 0, fmt.Sprintf("MUTANT_MEMORY_MAX=%d cannot be set here (%v), so %s", m.knobs.memoryMax, err, unbounded)
	}
	wanted := m.knobs.memoryMax << 30
	if held <= wanted {
		return 0, fmt.Sprintf("every process gremlins starts is already held to %d MiB of address space, the limit this run inherited, which is no more than MUTANT_MEMORY_MAX=%d GiB and so is kept", held>>20, m.knobs.memoryMax)
	}
	return wanted, fmt.Sprintf("every process gremlins starts is held to %d GiB of address space (MUTANT_MEMORY_MAX), so a mutant that allocates without end fails its test instead of exhausting the host", m.knobs.memoryMax)
}

// unleash runs gremlins over the package, or over its staged copy, and returns
// its status.
//
// The recipe measures ONE package, which is what its usage says and what the
// sweep's per-package figures claim. gremlins does not read it that way: it
// walks the directory it is given, so a package with anything under it is
// measured together with its whole subtree. Measured on ./cmd/audit_1to1: 789
// runnable mutants, of which 27 are the package's own and 762 belong to its
// seven sub-packages, each of which the sweep also measures on its own. It
// reaches fixtures too, so the planted trees under a command's testdata were
// being mutated as though they were source.
//
// --exclude-files takes a regexp over the path RELATIVE TO THE TARGET, which
// is the one fact this rests on and it was measured rather than assumed: on
// that same package `^internal/` and `/` both leave 24, while the
// module-relative `^cmd/audit_1to1/internal/` leaves all 789 and so matches
// nothing. A path with a separator in it is therefore exactly a file below the
// package, and `/` excludes all of them and nothing else. A leaf package has
// none, so passing it there changes no figure.
//
// A caller who states their own exclusion, as a flag in any spelling or
// through GREMLINS_UNLEASH_EXCLUDE_FILES, is left alone: they are asking for a
// different measurement, and two rules for one flag is how one of them
// silently wins. A flag passed here would override their variable without a
// word.
func (m *mutation) unleash(ctx context.Context, given deadline) (int, error) {
	args := []string{"run", gremlinsProgram, "unleash", "--invert-logical", "--workers", "4"}
	if given.derived {
		args = append(args, "--timeout-coefficient", given.coefficient)
	}
	args = append(args, m.flags.words...)
	if !m.flags.excluded {
		args = append(args, "--exclude-files=/")
	}
	target := m.name
	if m.staged.dir != "" {
		target = "./" + m.fromRoot(m.staged.dir)
	}
	call := m.goCall(m.dir, append(args, target)...)
	call.Stdout = m.stdout
	ceiling, said := m.memoryCeiling()
	if said != "" {
		m.sayf("%s", said)
	}
	call.AddressSpace = ceiling
	status, err := m.exec(ctx, call)
	if err != nil {
		return 0, fmt.Errorf("gremlins could not be started: %w", err)
	}
	return status, nil
}
