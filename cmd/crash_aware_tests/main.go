// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// usage is the command line, printed when it names no suite.
const usage = "Usage: crash_aware_tests <json-dir> [-flag=value ...] <packages ...>"

// exitProcess is [os.Exit] behind a seam, so the code [run] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

// gotestsumCommand builds the gotestsum invocation for args. It is a seam so
// the tests can answer it with a stand-in; the program itself is the one CI
// installs with go install tool, found on PATH as the script found it.
//
// The program is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_orbit_record): the
// lookup is the one exec would do, decided here rather than by whatever PATH
// holds when the command starts. A gotestsum the lookup does not find is the
// command's Err, which Start returns wrapping the lookup's own error, so the
// run reports it as a gotestsum that cannot start.
var gotestsumCommand = func(ctx context.Context, args []string) *exec.Cmd {
	bin, err := exec.LookPath("gotestsum")
	if err != nil {
		return &exec.Cmd{Err: fmt.Errorf("find gotestsum: %w", err)}
	}
	// #nosec G204 G702 -- the program is the absolute path just resolved, with this command's own arguments, passed on to gotestsum
	return exec.CommandContext(ctx, bin, args...)
}

func main() {
	exitProcess(run(context.Background(), os.Args[1:], os.Getenv("GITHUB_STEP_SUMMARY"), os.Stdout, os.Stderr))
}

// run runs the suite args name, reruns what crashed or died once, and returns
// the exit code. A non-empty summaryPath is the job summary file the reports
// are appended to as well as printed.
func run(ctx context.Context, args []string, summaryPath string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	jsonDir := args[0]
	var flags, packages []string
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			packages = append(packages, arg)
		}
	}
	if len(packages) == 0 {
		fmt.Fprintln(stderr, "ERROR: no packages given")
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err := os.MkdirAll(jsonDir, 0o750); err != nil { //#nosec G703 -- the stream directory the workflow step names on the command line
		fmt.Fprintf(stderr, "ERROR: create %s: %v\n", jsonDir, err)
		return 1
	}
	report := &reporter{stdout: stdout, stderr: stderr, path: summaryPath}

	firstPath := filepath.Join(jsonDir, "first.json")
	first, status, err := runSuite(ctx, firstPath, flags, packages, stdout, stderr)
	if err != nil {
		reportRunError(stderr, firstPath, status, err)
		// A run that wrote no stream judged nothing, so it has not passed,
		// whatever gotestsum exited with.
		return max(exitStatus(status), 1)
	}

	candidates := first.rerunCandidates()
	if failing := first.failing(); len(failing) > 0 {
		reportFailing(report, first, failing, candidates)
		return 1
	}
	if len(candidates) == 0 {
		return exitStatus(status)
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "The Go runtime crashed in the following package(s), or their binary stopped without a word; rerunning them alone once (issue 467, golang/go#81238):")
	for _, pkg := range candidates {
		fmt.Fprintf(stdout, "  %s: %s\n", pkg, first.rerunLine(pkg))
	}
	fmt.Fprintln(stdout)

	rerunPath := filepath.Join(jsonDir, "rerun.json")
	rerun, rerunStatus, rerunErr := runSuite(ctx, rerunPath, flags, candidates, stdout, stderr)
	if rerunErr != nil {
		reportRunError(stderr, rerunPath, rerunStatus, rerunErr)
	}
	if !reportRerun(report, first, rerun, candidates, rerunStatus) {
		report.summary("", "The rerun did not pass, so the run fails.")
		return 1
	}
	fmt.Fprintf(stdout, "::warning title=Go runtime crash::the Go runtime crashed in %d package(s) and every one passed when rerun alone; see the job summary (issue 467)\n", len(candidates))
	return 0
}

// runSuite runs gotestsum over packages with flags, writing its stream to
// path, and returns the stream and gotestsum's status. The status is -1 when
// gotestsum did not exit on its own, and carries nothing when gotestsum did
// not start, which the error says.
func runSuite(ctx context.Context, path string, flags, packages []string, stdout, stderr io.Writer) (*stream, int, error) {
	args := append([]string{"--jsonfile", path, "--"}, flags...)
	cmd := gotestsumCommand(ctx, append(args, packages...))
	cmd.Stdout, cmd.Stderr = stdout, stderr
	status := 0
	if err := cmd.Run(); err != nil {
		exitErr, exited := errors.AsType[*exec.ExitError](err)
		if !exited {
			return nil, 0, fmt.Errorf("run gotestsum: %w", err)
		}
		status = exitErr.ExitCode()
	}
	s, err := readStream(path)
	return s, status, err
}

// reportRunError says why a run's stream could not be judged.
func reportRunError(stderr io.Writer, path string, status int, err error) {
	if errors.Is(err, errNoStream) {
		fmt.Fprintf(stderr, "ERROR: gotestsum wrote no JSON stream to %s (exit %d)\n", path, status)
		return
	}
	fmt.Fprintf(stderr, "ERROR: %v\n", err)
}

// exitStatus is the exit code a gotestsum status hands on: the status
// itself, or 1 for a gotestsum that did not exit on its own, whose -1 the
// process would otherwise wrap around.
func exitStatus(status int) int {
	if status < 0 {
		return 1
	}
	return status
}

// reportFailing lists the failing packages with their failing tests, then the
// crashes the failures left unexamined.
func reportFailing(report *reporter, first *stream, failing, candidates []string) {
	report.summary("### Test suite: failing packages", "")
	for _, pkg := range failing {
		report.summary("- `" + pkg + "`")
		for _, test := range first.testsFailedIn(pkg) {
			report.summary("  - " + test)
		}
	}
	if len(candidates) == 0 {
		return
	}
	report.summary("", "Also crashed inside the Go runtime or stopped without a word, not rerun because the failures above decide the run:")
	for _, pkg := range candidates {
		report.summary("- `" + pkg + "`: " + first.rerunLine(pkg))
	}
}

// reportRerun writes the table of what each candidate did when rerun and
// reports whether the rerun passed: every package of it passed and gotestsum
// exited 0. A rerun with no stream (rerun nil) did not pass.
func reportRerun(report *reporter, first, rerun *stream, candidates []string, rerunStatus int) bool {
	report.summary("### Test suite: Go runtime crash", "",
		"The first run crashed inside the Go runtime, or a test binary stopped without a word (issue 467, golang/go#81238), and those packages were run once more on their own.", "",
		"| Package | First run | Rerun |", "| --- | --- | --- |")
	passed := rerun != nil && rerunStatus == 0
	for _, pkg := range candidates {
		outcome := rerunOutcome(rerun, pkg)
		passed = passed && outcome == "passed"
		report.summary(fmt.Sprintf("| `%s` | `%s` | %s |", pkg, strings.ReplaceAll(first.rerunLine(pkg), "|", `\|`), outcome))
	}
	return passed
}

// rerunOutcome is what pkg did in the rerun.
func rerunOutcome(rerun *stream, pkg string) string {
	if rerun == nil {
		return "no stream"
	}
	if slices.Contains(rerun.rerunCandidates(), pkg) {
		return "crashed again"
	}
	if rerun.failed[pkg] {
		return "failed"
	}
	return "passed"
}
