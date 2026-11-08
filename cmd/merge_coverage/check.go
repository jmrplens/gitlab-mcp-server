// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// checkHeading opens the gate's report, as a section of the coverage job's
// summary.
const checkHeading = "### Statement coverage of the e2e support packages, held to the recorded runs\n\n"

// noRecordLine is what the gate says before the first complete run is
// recorded, when there is nothing to hold the packages to. It names the
// licensed run only as the instance allows it: without an EE license the CE
// run alone is recorded, and what only the licensed run reaches stays inside
// the ceiling.
const noRecordLine = "No complete run is recorded, so no package is held to a ceiling yet: after the complete runs the instance allows (make test-e2e-ce, and make test-e2e-ee with an EE license), make e2e-support-coverage-record writes the record."

// failRemedy closes the line of a package that fails, with the two ways out.
const failRemedy = " Reach them with a test, or record a complete run of this tree: the complete runs the instance allows (make test-e2e-ce, and make test-e2e-ee with an EE license), then make e2e-support-coverage-record."

// check is the gate: the offline half on this tree merged with the record of
// the complete runs, and each package held to the number of statements the
// record says neither half reached. It prints the report, appends it to the
// summary when asked, and returns the verdict of every package that fails,
// joined, after the report, so the reason is in the job's log and summary
// alike. A run it cannot judge, because a profile, the record or a package
// cannot be read or measured, is refused before any report.
func check(opts options, stdout io.Writer) error {
	cov, err := read(opts.sources)
	if err != nil {
		return err
	}
	rec, gone, err := creditRecord(cov, opts)
	if err != nil {
		return err
	}
	rows, err := cov.rows(opts.packages)
	if err != nil {
		return err
	}
	lines, failures := judge(rows, rec, gone, opts.deferred)
	if publishErr := publish(renderCheck(opts.sources, opts.checkRecord, rec, rows, lines), opts.summary, stdout); publishErr != nil {
		return publishErr
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// creditRecord reads the record the gate was given and credits its reach to
// the offline half, returning the record, or nil when there is none yet, and
// the declarations it dropped. A record of other packages is refused.
func creditRecord(cov *coverage, opts options) (*record, []dropped, error) {
	rec, found, err := loadRecord(opts.checkRecord)
	if err != nil || !found {
		return nil, nil, err
	}
	if mismatch := rec.matches(opts.packages); mismatch != nil {
		return nil, nil, mismatch
	}
	gone, err := cov.credit(rec)
	if err != nil {
		return nil, nil, err
	}
	return rec, gone, nil
}

// judge returns the lines the report closes with, every dropped declaration
// first and then each package's verdict, and the verdicts that fail. With
// no record there is one line and nothing to fail.
func judge(rows []row, rec *record, gone []dropped, deferred bool) (lines, failures []string) {
	if rec == nil {
		return []string{noRecordLine}, nil
	}
	for _, d := range gone {
		lines = append(lines, fmt.Sprintf("`%s` `%s`: %s, so what the runs reached in it no longer counts.", d.file, d.declaration, d.why))
	}
	for i, r := range rows {
		line, failed := verdict(r, rec.Packages[i], droppedIn(gone, rec.Packages[i].Package), deferred)
		if failed {
			lines = append(lines, "**Fails**: "+line)
			failures = append(failures, line)
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, failures
}

// verdict judges one package against its recorded ceiling. More statements
// reached by neither half than the record allows fails, unless freshness is
// deferred and the record predates the package, because a declaration of it
// was dropped or it counts other statements than the record did: below a
// stack's top that is a note, since the top records a complete run of the
// tree it changed. Fewer is a note that the next record lowers the ceiling,
// and as many says nothing.
func verdict(r row, p recordPackage, droppedHere int, deferred bool) (string, bool) {
	missed := r.merged.missed()
	if missed < p.Unreached {
		return fmt.Sprintf("`%s`: %d statements are reached by neither half, fewer than the %d the record allows; the next record lowers it.",
			p.Package, missed, p.Unreached), false
	}
	if missed == p.Unreached {
		return "", false
	}
	line := fmt.Sprintf("`%s`: %d statements are reached by neither half, and the record allows %d.", p.Package, missed, p.Unreached)
	if deferred && (droppedHere > 0 || r.merged.statements != p.Statements) {
		return line + fmt.Sprintf(staleNote, droppedHere, r.merged.statements, p.Statements), false
	}
	return line + failRemedy, true
}

// staleNote closes the line of a package whose excess a record older than
// the tree explains, below a stack's top.
const staleNote = " The record predates the package (recorded declarations that no longer match: %d; statements now and in the record: %d and %d), so below a stack's top this is a note: the top records a complete run of its tree."

// droppedIn counts the dropped declarations of one package, named as the
// record names it, the module's root package by the empty name the record
// gives it.
func droppedIn(gone []dropped, pkg string) int {
	n := 0
	for _, d := range gone {
		dir := path.Dir(d.file)
		if dir == "." {
			dir = ""
		}
		if dir == pkg {
			n++
		}
	}
	return n
}

// renderCheck writes the gate's report: where each half came from, one row
// per package with each half's figure, the merge, the statements neither
// half reached and what the record allows, then the lines judge returned.
func renderCheck(sources []source, recordPath string, rec *record, rows []row, lines []string) string {
	var b strings.Builder
	b.WriteString(checkHeading)
	fmt.Fprintf(&b, "Offline: %s. Runs: %s.\n\n", profileNames(sources, offlineHalf), recordSummary(recordPath, rec))
	b.WriteString("| Package | Offline tests | Recorded runs | Merged | Reached by neither | The record allows |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: |\n")
	for i, r := range rows {
		allows := "none"
		if rec != nil {
			allows = strconv.Itoa(rec.Packages[i].Unreached)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %d | %s |\n",
			goprogram.TrimModulePath(r.pkg), r.cell(offlineHalf), r.cell(onlineHalf), r.merged, r.merged.missed(), allows)
	}
	b.WriteString(absentNote)
	if len(lines) > 0 {
		b.WriteString("\n")
	}
	for _, line := range lines {
		b.WriteString("- " + line + "\n")
	}
	return b.String()
}

// recordSummary says where the runs' half came from: the record by its file
// name, the day it was taken and the run profiles it was folded from, or
// that there is none yet.
func recordSummary(recordPath string, rec *record) string {
	name := "`" + filepath.Base(recordPath) + "`"
	if rec == nil {
		return "none recorded, " + name + " does not exist yet"
	}
	return fmt.Sprintf("%s, recorded on %s from %s", name, rec.Source.Recorded, codeList(rec.Source.Runs))
}
