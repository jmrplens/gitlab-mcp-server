package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sync"
)

// localSystem is what a tree measured on the machine at hand names its
// system in its records and its summary, since no runner label describes it.
const localSystem = "local"

// runTree measures every package below cmd/ and internal/ on the machine at
// hand and writes the summary the workflow's summary job would, the total
// included, to stdout: the one command that reproduces the whole-tree figure
// without a dispatch. The records are left below -records, one directory per
// package, and a record left there by an earlier run is replaced. -parallel
// packages are measured at once; each run's output goes to its record alone,
// and stderr says what became of each package as it finishes.
//
// It returns 2 for a usage error, 1 when the packages could not be listed, a
// record could not be written, or the summary found a record it could not
// use, and 0 otherwise, whatever the figures say and however many packages
// gobco could not measure, which the summary names with the reason.
func runTree(args []string, stdout, stderr io.Writer, list lister, run scriptRunner) int {
	flags := flag.NewFlagSet("tree", flag.ContinueOnError)
	flags.SetOutput(stderr)
	records := flags.String("records", "", "directory each package's record and raw output are written below, one directory per package")
	commit := flags.String("commit", "", "the commit the tree is at, which the summary names")
	gate := flags.String("gate", "all", "the GOBCO_GATE the script runs with: all or beyond:GOOS/GOARCH")
	parallel := flags.Int("parallel", 1, "how many packages are measured at once")
	dir := flags.String("dir", ".", "module root the packages are looked for and measured in")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 || *records == "" || *commit == "" || *parallel < 1 {
		fmt.Fprintf(stderr, "%s: tree takes -records and -commit, both non-empty, a -parallel of at least 1, and no arguments\n", toolName)
		return 2
	}
	if !gatePattern.MatchString(*gate) {
		fmt.Fprintf(stderr, "%s: -gate %q is neither all nor beyond:GOOS/GOARCH, which is what scripts/coverage-conditions.sh reads\n", toolName, *gate)
		return 2
	}
	ctx := context.Background()
	packages, err := treePackages(ctx, *dir, list)
	if err != nil {
		fmt.Fprintf(stderr, "%s: the packages below cmd/ and internal/ could not be listed: %v\n", toolName, err)
		return 1
	}
	if len(packages) == 0 {
		fmt.Fprintf(stderr, "%s: there is no package below cmd/ and internal/, so there is nothing to measure\n", toolName)
		return 1
	}
	fmt.Fprintf(stderr, "%s: measuring the %d packages below cmd/ and internal/, %d at a time, with GOBCO_GATE=%s\n", toolName, len(packages), *parallel, *gate)
	m := measurer{run: run, dir: *dir, system: localSystem, records: *records, env: []string{"GOBCO_GATE=" + *gate}}
	if err = measureAll(ctx, m, packages, *parallel, stderr); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	runs := make([]packageRun, 0, len(packages))
	for _, pkg := range packages {
		runs = append(runs, packageRun{Package: pkg, System: localSystem})
	}
	recs, problems := readRecords(*records)
	outcomes, more := join(runs, recs)
	problems = append(problems, more...)
	render(stdout, *gate, *commit, outcomes, problems)
	for _, problem := range problems {
		fmt.Fprintf(stderr, "%s: %s\n", toolName, problem)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

// measureAll measures the packages with m, parallel at a time, and says on
// progress what became of each as it finishes. Every package is measured, and
// every one that could not be run or recorded is returned as an error at the
// end, in the order they finished.
func measureAll(ctx context.Context, m measurer, packages []string, parallel int, progress io.Writer) error {
	var (
		mu       sync.Mutex
		failures []error
		wg       sync.WaitGroup
	)
	work := make(chan string)
	for range parallel {
		wg.Go(func() {
			for pkg := range work {
				status, err := measurePackage(ctx, m, pkg, io.Discard)
				mu.Lock()
				if err != nil {
					failures = append(failures, err)
				} else {
					fmt.Fprintf(progress, "%s: %s: %s\n", toolName, pkg, verdict(status))
				}
				mu.Unlock()
			}
		})
	}
	for _, pkg := range packages {
		work <- pkg
	}
	close(work)
	wg.Wait()
	return errors.Join(failures...)
}
