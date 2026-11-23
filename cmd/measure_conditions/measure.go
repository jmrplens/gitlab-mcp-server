// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/conditions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// coverageRecipe is the recipe every measurement runs, as a message names it:
// the one behind `make coverage-conditions`, which ci.yml's legs run too.
const coverageRecipe = "cmd/internal/conditions"

// recipeRunner runs the recipe over one package in the module root dir, with
// env added to the process environment and both of its streams written to
// output, and returns its status. It is a parameter so a test can answer for
// the recipe.
type recipeRunner func(ctx context.Context, dir, pkg string, env []string, output io.Writer) int

// runRecipe is the recipeRunner production uses: the recipe in this process,
// where its status reaches the record as it is, which it would not through
// `go run`. Both streams go to one writer, so the output holds everything the
// recipe printed.
func runRecipe(ctx context.Context, dir, pkg string, env []string, output io.Writer) int {
	return conditions.Measure(ctx, conditions.DefaultTools(), conditions.Request{
		Dir: dir, Package: pkg, Environ: append(os.Environ(), env...), Stdout: output, Stderr: output,
	})
}

// measured reports whether a status of the recipe is a measurement taken: 0
// when the gate held nothing and 3 when it held a condition.
func measured(status int) bool {
	return status == statusMeasured || status == statusGateHeld
}

// verdict is how a progress line names what became of one package: the
// recipe's status, or that there was nothing to measure and the recipe was
// not run.
func verdict(status int, nothing bool) string {
	if nothing {
		return "measured, nothing to measure (no condition, the recipe not run)"
	}
	if measured(status) {
		return fmt.Sprintf("measured (exit %d)", status)
	}
	return fmt.Sprintf("not measured (exit %d)", status)
}

// recipeMode is the GOBCO_MODE the recipe will read from value, the
// environment's: gobco's default when it is empty, and empty when it is not a
// mode the recipe reads, which the recipe refuses itself.
func recipeMode(value string) string {
	if value == "" {
		return defaultMode
	}
	if _, known := coverageModes[value]; known {
		return value
	}
	return ""
}

// countingMode is the mode measure counts a package's conditions in before
// handing it to the recipe, from the environment's GOBCO_MODE and
// GOBCO_GATE: the mode the recipe will read, and none when the recipe will
// refuse either value, so that every package is handed to the recipe, which
// says why it will not measure it, rather than a package with nothing to
// measure being recorded as measured under a setting the recipe refuses. An
// empty GOBCO_GATE is the recipe's default, which it reads.
func countingMode(mode, gate string) string {
	if gate != "" && !gatePattern.MatchString(gate) {
		return ""
	}
	return recipeMode(mode)
}

// measurer is what every package of one run is measured with: the runner of
// the recipe, the go command that lists a package's files, the module root
// both run in, the system the records name, the directory of records they
// are left below, what is added to the recipe's environment, and the mode
// the recipe counts in, empty when the recipe will refuse it or the gate it
// is run with (countingMode).
type measurer struct {
	run     recipeRunner
	list    lister
	dir     string
	system  string
	records string
	env     []string
	mode    string
}

// measurePackage runs the recipe over one package and leaves, in the
// directory of records named after the package, everything it printed and
// the record of what ran where and how it ended. echo receives a copy of the
// output as it is printed. It returns the recipe's status, or an error when a
// file could not be written, in which case the package has no record. A run a
// signal ended has none either, since it measured nothing and says nothing of
// why.
//
// A package whose files hold no condition gobco counts in the mode is not
// handed to the recipe, whose gobco would report 0/0 for it, the figure the
// recipe refuses because a package gobco did not instrument reports it too
// (issue 1667). It is recorded as measured, with nothing to measure: the
// status of a measurement the gate held nothing in, and in place of the
// recipe's output the line that says so (nothingLine), and nothing comes
// back true. A package whose conditions could not be counted is handed to
// the recipe, which says why it cannot measure it if it cannot.
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
	status = m.run(ctx, m.dir, pkg, m.env, io.MultiWriter(echo, &output))
	if _, interrupted := recipe.Status(ctx); interrupted {
		return status, false, nil
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
// job of the workflow does, streaming the recipe's output to stdout and
// leaving each package's record below -records, where the upload step takes
// them from. The mode a package's conditions are counted in is the one the
// recipe will read, GOBCO_MODE as the job's environment gives it, and none
// when the recipe will refuse that or the environment's GOBCO_GATE. It returns
// 2 for a usage error, 1 when a package could not be measured or a record
// could not be written, a signal's status when one ended the run, after which
// no package is measured, and 0 when every package was measured, whatever the
// figures say.
func runMeasure(ctx context.Context, args []string, stdout, stderr io.Writer, list lister, run recipeRunner) int {
	flags := flag.NewFlagSet("measure", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagesFlag := flags.String("packages", "", "packages to measure one after another, ./cmd/server style, separated by spaces or commas")
	system := flags.String("system", "", "the runner the records name")
	records := flags.String("records", "", "directory each package's record and raw output are written below, one directory per package")
	dir := flags.String("dir", ".", "module root the recipe runs in")
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
	m := measurer{run: run, list: list, dir: *dir, system: *system, records: *records, mode: countingMode(os.Getenv("GOBCO_MODE"), os.Getenv("GOBCO_GATE"))}
	for _, pkg := range packages {
		status, nothing, measureErr := measurePackage(ctx, m, pkg, stdout)
		if ended, interrupted := recipe.Status(ctx); interrupted {
			return ended
		}
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
