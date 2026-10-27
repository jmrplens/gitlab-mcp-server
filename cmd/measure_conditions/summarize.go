// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// reasonTail is how many of the last lines of a run's output stand for its
// reason when no line of it says why (failureLine).
const reasonTail = 20

// packageRun is one package measured on one runner: what a plan asks of a
// job for each package it names, and where the job uploads that package's
// record. A tree measured on the machine at hand has no artifact, its records
// sitting directly below the directory it was given.
type packageRun struct {
	Package  string
	System   string
	Artifact string
}

// runsOf is every package run the cells of a plan ask for, cell by cell and,
// within a cell, in the order its packages are named.
func runsOf(cells []cell) []packageRun {
	var runs []packageRun
	for _, planned := range cells {
		for pkg := range strings.FieldsSeq(planned.Packages) {
			runs = append(runs, packageRun{Package: pkg, System: planned.System, Artifact: planned.Artifact})
		}
	}
	return runs
}

// recordPath is where a run's record lies below the directory the records
// were downloaded or written into: the run's artifact, if it has one, and
// the package's own directory in it.
func (r packageRun) recordPath() string {
	return path.Join(r.Artifact, packageSlug(r.Package))
}

// outcome is what became of one planned run.
type outcome struct {
	packageRun
	rec      *record
	measured *measurement
	problem  string
}

// runSummarize writes the job summary for a dispatch to stdout. It returns 2
// for a usage error, 1 when a record could not be read or disagreed with the
// plan or with itself, and 0 otherwise, whatever the figures say.
func runSummarize(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("summarize", flag.ContinueOnError)
	flags.SetOutput(stderr)
	planFlag := flags.String("plan", "", "the matrix plan printed, as JSON")
	records := flags.String("records", "", "directory the runs' artifacts were downloaded into")
	gate := flags.String("gate", "", "the GOBCO_GATE the runs used")
	mode := flags.String("mode", defaultMode, "the GOBCO_MODE the runs used: condition or branch")
	commit := flags.String("commit", "", "the commit the runs measured")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 || *planFlag == "" || *records == "" || *gate == "" || *commit == "" {
		fmt.Fprintf(stderr, "%s: summarize takes -plan, -records, -gate and -commit, all four non-empty, and no arguments\n", toolName)
		return 2
	}
	if refuseMode(stderr, *mode) {
		return 2
	}
	var planned matrix
	if err := json.Unmarshal([]byte(*planFlag), &planned); err != nil || len(runsOf(planned.Include)) == 0 {
		fmt.Fprintf(stderr, "%s: -plan is not a matrix plan with at least one run in it\n", toolName)
		return 2
	}
	recs, problems := readRecords(*records)
	outcomes, more := join(runsOf(planned.Include), recs, *mode)
	problems = append(problems, more...)
	render(stdout, *gate, *mode, *commit, outcomes, problems)
	for _, problem := range problems {
		fmt.Fprintf(stderr, "%s: %s\n", toolName, problem)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

// join pairs every planned run with its record, in the plan's order, and
// reads the measurement of each record whose status says one was taken, in
// the mode the runs were asked for. A record the plan did not ask for, a
// second record for one run, and a record whose output disagrees with its
// status or carries no figure of the mode are problems, since a table built
// on any of them would be a guess.
func join(runs []packageRun, recs []record, mode string) (outcomes []outcome, problems []string) {
	byRun := map[string]*record{}
	for i := range recs {
		rec := &recs[i]
		key := rec.pkg + " on " + rec.system
		if byRun[key] != nil {
			problems = append(problems, "two records name "+key)
			continue
		}
		byRun[key] = rec
	}
	outcomes = make([]outcome, 0, len(runs))
	for _, planned := range runs {
		key := planned.Package + " on " + planned.System
		result := outcome{packageRun: planned, rec: byRun[key]}
		delete(byRun, key)
		if result.rec != nil && measured(result.rec.status) {
			found, err := parseMeasurement(result.rec.output, result.rec.status, mode)
			if err != nil {
				result.problem = err.Error()
				problems = append(problems, fmt.Sprintf("the record of %s cannot be read: %v", key, err))
			} else {
				result.measured = &found
			}
		}
		outcomes = append(outcomes, result)
	}
	for _, unplanned := range slices.Sorted(maps.Keys(byRun)) {
		problems = append(problems, "a record names "+unplanned+", which the plan did not ask for")
	}
	return outcomes, problems
}

// render writes the summary of the mode the runs measured in: the figures,
// the total of each system, then every condition not evaluated both ways,
// then what could not be measured and why.
func render(w io.Writer, gate, mode, commit string, outcomes []outcome, problems []string) {
	measured := coverageModes[mode]
	fmt.Fprintf(w, "## %s, measured\n\n", measured.figure)
	fmt.Fprintf(w, "`GOBCO_GATE=%s` at `%s`. Every figure is the one gobco printed on the runner named, through `scripts/coverage-conditions.sh`, the recipe behind `make coverage-conditions`; none is inferred from another platform. %s No figure fails this workflow: it is a measurement, not a gate.\n\n", gate, commit, measured.counts)
	writeRow(w, "Package", "System", "Covered", "Not both ways", "Held by the gate", "Raw output")
	fmt.Fprintf(w, "| --- | --- | ---: | ---: | ---: | --- |\n")
	for _, result := range outcomes {
		writeRow(w, codeSpan(result.Package), systemCell(result), coveredCell(result),
			countCell(result, len(conditionsOf(result))), countCell(result, heldOf(result)), codeSpan(result.recordPath()))
	}
	writeTotals(w, outcomes)
	writeConditions(w, outcomes)
	writeUnmeasured(w, outcomes)
	if len(problems) > 0 {
		fmt.Fprintf(w, "\n### Records this summary could not use\n\n")
		for _, problem := range problems {
			fmt.Fprintf(w, "- %s\n", problem)
		}
	}
}

// total is what the measured runs of one system add up to.
type total struct {
	system   string
	planned  int
	measured int
	covered  int
	outcomes int
}

// writeTotals writes, for each system in the order the runs name them, how
// many of its planned packages were measured and the outcomes gobco counted
// as evaluated of those there are, summed over the measured ones, with that
// share rounded down to a tenth of a percent so it never reads higher than
// it is. A run that took no measurement is in no total: the table above says
// which ones and why, and the share is of the packages measured.
func writeTotals(w io.Writer, outcomes []outcome) {
	var totals []*total
	bySystem := map[string]*total{}
	for _, result := range outcomes {
		sum := bySystem[result.System]
		if sum == nil {
			sum = &total{system: result.System}
			bySystem[result.System] = sum
			totals = append(totals, sum)
		}
		sum.planned++
		if result.measured != nil {
			sum.measured++
			sum.covered += result.measured.covered
			sum.outcomes += result.measured.total
		}
	}
	fmt.Fprintf(w, "\n### Total\n\n")
	fmt.Fprintf(w, "A package that was not measured is in no total: the table above says which and why, and each share is of the packages its system measured.\n\n")
	writeRow(w, "System", "Packages measured", "Covered", "Share")
	fmt.Fprintf(w, "| --- | ---: | ---: | ---: |\n")
	for _, sum := range totals {
		writeRow(w, codeSpan(sum.system), fmt.Sprintf("%d of %d", sum.measured, sum.planned),
			fmt.Sprintf("%d of %d", sum.covered, sum.outcomes), share(sum.covered, sum.outcomes))
	}
}

// share is covered of outcomes as a percentage rounded down to a tenth, so
// 799 of 1000 reads 79.9% and never 80.0%, and a dash when nothing was
// counted.
func share(covered, outcomes int) string {
	if outcomes == 0 {
		return "-"
	}
	tenths := covered * 1000 / outcomes
	return fmt.Sprintf("%d.%d%%", tenths/10, tenths%10)
}

// writeConditions lists every condition gobco named as not evaluated both
// ways, run by run.
func writeConditions(w io.Writer, outcomes []outcome) {
	header := false
	for _, result := range outcomes {
		for _, cond := range conditionsOf(result) {
			if !header {
				fmt.Fprintf(w, "\n### Not evaluated both ways\n\n")
				writeRow(w, "Package", "System", "File", "Line", "Condition", "gobco says", "Held")
				fmt.Fprintf(w, "| --- | --- | --- | ---: | --- | --- | --- |\n")
				header = true
			}
			held := "no"
			if cond.held {
				held = "yes"
			}
			writeRow(w, codeSpan(result.Package), codeSpan(result.System), codeSpan(cond.file), cond.line,
				codeSpan(cond.code), cond.says, held)
		}
	}
}

// writeUnmeasured says, for every run that took no measurement, why: the
// lines of its output that say it (reasonLines), or that no record came back
// at all.
func writeUnmeasured(w io.Writer, outcomes []outcome) {
	header := false
	for _, result := range outcomes {
		if result.measured != nil || result.problem != "" {
			continue
		}
		if !header {
			fmt.Fprintf(w, "\n### Not measured\n")
			header = true
		}
		fmt.Fprintf(w, "\n#### %s on %s\n\n", codeSpan(result.Package), codeSpan(result.System))
		if result.rec == nil {
			fmt.Fprintf(w, "No record came back in %s: the run ended before its measuring step wrote one (a setup step failed, or the run was cancelled, timed out or lost its runner), or it did not upload the one it wrote, and its job log says which.\n", codeSpan(result.recordPath()))
			continue
		}
		fmt.Fprintf(w, "`scripts/coverage-conditions.sh` exited %d, which means it could not measure, and said:\n\n", result.rec.status)
		reason := reasonLines(result.rec.output)
		fence := strings.Repeat("`", max(3, longestRun(reason, '`')+1))
		fmt.Fprintf(w, "%stext\n%s\n%s\n", fence, reason, fence)
	}
}

// failureLine is a line of a run's output that says why it could not
// measure: one the script wrote, which it prefixes with `gobco: `, or one
// gobco or its go test printed when either failed, which is what the
// script's verdict that the run is not a measurement follows. Those are a
// panic, which is how gobco stops on a package it cannot type-check; a
// failing top-level test; a test package that did not build or set up; and
// the compiler's position for either, which is the only line naming the
// cause of a setup failure. A condition gobco names has a position too, and
// is told apart by conditionPattern, since a report printed beside a failure
// says nothing about the failure.
var failureLine = regexp.MustCompile(`^(gobco: |panic: |--- FAIL: |FAIL\t.*\[(build|setup) failed\]$|\S+\.go:\d+:\d+: )`)

// reasonLines returns the lines of a run's output that say why it could not
// measure, failureLine's, in the order they were printed, or the end of the
// output when there is none.
func reasonLines(output string) string {
	var said, all []string
	for line := range strings.SplitSeq(strings.TrimRight(output, "\r\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		all = append(all, line)
		if failureLine.MatchString(line) && !conditionPattern.MatchString(line) {
			said = append(said, line)
		}
	}
	if len(said) == 0 {
		said = all[max(0, len(all)-reasonTail):]
	}
	return strings.Join(said, "\n")
}

// systemCell names the runner, and the platform the go command reported on
// it when a record says.
func systemCell(result outcome) string {
	if result.rec == nil {
		return codeSpan(result.System)
	}
	return codeSpan(result.System) + " (" + result.rec.platform + ")"
}

// coveredCell is gobco's figure, or why there is none. It is a chain of ifs
// rather than a tagless switch because the coverage profile gremlins reads
// gives a case expression no block of its own, so every mutant of one is
// reported as not covered however many tests reach it.
func coveredCell(result outcome) string {
	if result.measured != nil {
		return fmt.Sprintf("%d of %d", result.measured.covered, result.measured.total)
	}
	if result.problem != "" {
		return "unreadable record"
	}
	if result.rec == nil {
		return "no record"
	}
	return fmt.Sprintf("not measured (exit %d)", result.rec.status)
}

// countCell is a count of a measured run, and a dash for any other.
func countCell(result outcome, count int) string {
	if result.measured == nil {
		return "-"
	}
	return strconv.Itoa(count)
}

// conditionsOf is the conditions a run named, none when it measured nothing.
func conditionsOf(result outcome) []condition {
	if result.measured == nil {
		return nil
	}
	return result.measured.conditions
}

// heldOf is how many conditions a run's gate held, none when it measured
// nothing.
func heldOf(result outcome) int {
	if result.measured == nil {
		return 0
	}
	return result.measured.held
}

// codeSpan writes text as a Markdown code span, its delimiter one backtick
// longer than the longest run of backticks inside, and padded with a space
// where the text starts or ends with one, which is the CommonMark rule for a
// code span that holds backticks.
func codeSpan(text string) string {
	delimiter := strings.Repeat("`", longestRun(text, '`')+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return delimiter + text + delimiter
}

// writeRow writes one table row, escaping in every cell the one character a
// cell cannot hold as written: a pipe, which GitHub reads as a column break
// even inside a code span, and reads as a pipe there once escaped.
func writeRow(w io.Writer, cells ...string) {
	for _, text := range cells {
		fmt.Fprintf(w, "| %s ", strings.ReplaceAll(text, "|", `\|`))
	}
	fmt.Fprintf(w, "|\n")
}

// longestRun is the length of the longest run of r in text.
func longestRun(text string, r rune) int {
	longest, run := 0, 0
	for _, c := range text {
		if c != r {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return longest
}
