// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// coverageScript is the recipe every measurement runs, from the module root:
// the one behind `make coverage-conditions`, which ci.yml's legs run too.
const coverageScript = "scripts/coverage-conditions.sh"

// scriptRunner runs coverageScript over one package in the module root dir,
// with env added to the process environment and both of its streams written
// to output, and returns its exit status. An error means it could not be run
// at all. It is a parameter so a test can answer for the script.
type scriptRunner func(ctx context.Context, dir, pkg string, env []string, output io.Writer) (int, error)

// runScript is the scriptRunner production uses: the script through bash, as
// ci.yml's legs and `make coverage-conditions` run it. Both streams go to one
// writer, so the output reads in the order the script printed it, as `2>&1`
// would have it.
//
// bash is resolved to an absolute path first, which is how every other
// command here runs a tool it did not build (cmd/gen_orbit_record): the
// lookup is the one exec would do, decided here rather than by whatever PATH
// holds when the command starts. On a Windows runner it finds the Git Bash a
// bash step runs in.
func runScript(ctx context.Context, dir, pkg string, env []string, output io.Writer) (int, error) {
	bin, err := exec.LookPath("bash")
	if err != nil {
		return 0, fmt.Errorf("find bash: %w", err)
	}
	// #nosec G204 -- the program is the absolute path just resolved, with the repository's own script and a package parsePackages or the go command named
	cmd := exec.CommandContext(ctx, bin, coverageScript, pkg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	if exit, exited := errors.AsType[*exec.ExitError](err); exited {
		return exit.ExitCode(), nil
	}
	return 0, err
}

// measured reports whether a status of the script is a measurement taken: 0
// when the gate held nothing and 3 when it held a condition.
func measured(status int) bool {
	return status == statusMeasured || status == statusGateHeld
}

// verdict is how a progress line names what became of one package: the
// script's status, or that there was nothing to measure and the script was
// not run.
func verdict(status int, nothing bool) string {
	if nothing {
		return "measured, nothing to measure (no condition, the script not run)"
	}
	if measured(status) {
		return fmt.Sprintf("measured (exit %d)", status)
	}
	return fmt.Sprintf("not measured (exit %d)", status)
}

// scriptMode is the GOBCO_MODE the script will read from value, the
// environment's: gobco's default when it is empty, and empty when it is not a
// mode the script reads, which the script refuses itself.
func scriptMode(value string) string {
	if value == "" {
		return defaultMode
	}
	if _, known := coverageModes[value]; known {
		return value
	}
	return ""
}

// measurer is what every package of one run is measured with: the runner of
// the script, the go command that lists a package's files, the module root
// both run in, the system the records name, the directory of records they
// are left below, what is added to the script's environment, and the mode
// the script counts in, empty when it is one the script will refuse.
type measurer struct {
	run     scriptRunner
	list    lister
	dir     string
	system  string
	records string
	env     []string
	mode    string
}

// measurePackage runs the script over one package and leaves, in the
// directory of records named after the package, everything it printed and
// the record of what ran where and how it ended. echo receives a copy of the
// output as it is printed. It returns the script's status, or an error when
// the script could not be run or a file could not be written, in which case
// the package has no record.
//
// A package whose files hold no condition gobco counts in the mode is not
// handed to the script, whose gobco would report 0/0 for it, the figure the
// script refuses because a package gobco did not instrument reports it too
// (issue 1667). It is recorded as measured, with nothing to measure: the
// status of a measurement the gate held nothing in, and in place of the
// script's output the line that says so (nothingLine), and nothing comes
// back true. A package whose conditions could not be counted is handed to
// the script, which says why it cannot measure it if it cannot.
func measurePackage(ctx context.Context, m measurer, pkg string, echo io.Writer) (status int, nothing bool, err error) {
	recordDir := filepath.Join(m.records, packageSlug(pkg))
	if m.mode != "" {
		if counted, countErr := countPackage(ctx, m, pkg); countErr == nil && counted.count == 0 {
			said := nothingLine(pkg, m.mode, counted)
			fmt.Fprint(echo, said)
			return statusMeasured, true, writeRecord(recordDir, []byte(said), recordOf(pkg, m.system, statusMeasured))
		}
	}
	var output bytes.Buffer
	status, err = m.run(ctx, m.dir, pkg, m.env, io.MultiWriter(echo, &output))
	if err != nil {
		return 0, false, fmt.Errorf("%s could not be run over %s: %w", coverageScript, pkg, err)
	}
	return status, false, writeRecord(recordDir, output.Bytes(), recordOf(pkg, m.system, status))
}

// recordOf is the record of one package's run: the package, the system, the
// platform it ran on, and the status.
func recordOf(pkg, system string, status int) string {
	return fmt.Sprintf("package=%s\nsystem=%s\nplatform=%s/%s\nstatus=%d\n", pkg, system, runtime.GOOS, runtime.GOARCH, status)
}

// writeRecord leaves one run in recordDir: the raw output first and the
// record after it, so a directory holding a record always holds the output
// the record describes.
func writeRecord(recordDir string, output []byte, text string) error {
	if err := os.MkdirAll(recordDir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(recordDir, outputFile), output, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(recordDir, recordFile), []byte(text), 0o600)
}

// runMeasure measures the packages -packages names one after another, as a
// job of the workflow does, streaming the script's output to stdout and
// leaving each package's record below -records, where the upload step takes
// them from. The mode a package's conditions are counted in is the one the
// script will read, GOBCO_MODE as the job's environment gives it. It returns
// 2 for a usage error, 1 when a package could not be measured or a record
// could not be written, and 0 when every package was measured, whatever the
// figures say.
func runMeasure(args []string, stdout, stderr io.Writer, list lister, run scriptRunner) int {
	flags := flag.NewFlagSet("measure", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagesFlag := flags.String("packages", "", "packages to measure one after another, ./cmd/server style, separated by spaces or commas")
	system := flags.String("system", "", "the runner the records name")
	records := flags.String("records", "", "directory each package's record and raw output are written below, one directory per package")
	dir := flags.String("dir", ".", "module root the script runs in")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	packages, err := parsePackages(*packagesFlag)
	if err != nil {
		fmt.Fprintf(stderr, "%s: -packages: %v\n", toolName, err)
		return 2
	}
	if flags.NArg() > 0 || len(packages) == 0 || *system == "" || *records == "" {
		fmt.Fprintf(stderr, "%s: measure takes -packages, -system and -records, all three non-empty, and no arguments\n", toolName)
		return 2
	}
	var unmeasured []string
	m := measurer{run: run, list: list, dir: *dir, system: *system, records: *records, mode: scriptMode(os.Getenv("GOBCO_MODE"))}
	for _, pkg := range packages {
		status, nothing, measureErr := measurePackage(context.Background(), m, pkg, stdout)
		if measureErr != nil {
			fmt.Fprintf(stderr, "%s: %v\n", toolName, measureErr)
			return 1
		}
		fmt.Fprintf(stderr, "%s: %s on %s: %s\n", toolName, pkg, *system, verdict(status, nothing))
		if !measured(status) {
			unmeasured = append(unmeasured, pkg)
		}
	}
	if len(unmeasured) > 0 {
		fmt.Fprintf(stderr, "%s: %d of %d package(s) could not be measured on %s: %s; the gobco: lines above say why\n",
			toolName, len(unmeasured), len(packages), *system, strings.Join(unmeasured, " "))
		return 1
	}
	return 0
}
