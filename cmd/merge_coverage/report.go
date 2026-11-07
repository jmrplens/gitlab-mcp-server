// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// reportHeading opens the report, as a section of a workflow run's summary.
const reportHeading = "### Statement coverage, offline tests and runs against GitLab merged\n\n"

// render writes the report: the profiles each half was read from, then one
// row per package with what each half reached, what they reach together and
// how many statements only the runs reached.
func render(sources []source, rows []row) string {
	var b strings.Builder
	b.WriteString(reportHeading)
	fmt.Fprintf(&b, "Offline: %s. Runs: %s.\n\n", profileNames(sources, offlineHalf), profileNames(sources, onlineHalf))
	b.WriteString("| Package | Offline tests | Runs | Merged | Reached only by the runs |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %d |\n",
			goprogram.TrimModulePath(r.pkg), r.cell(offlineHalf), r.cell(onlineHalf), r.merged, r.onlyOnline)
	}
	b.WriteString(absentNote)
	return b.String()
}

// absentNote closes the table of either report, saying what an absent half
// is.
const absentNote = "\nA half reads absent when none of its profiles carries a block of the package.\n"

// profileNames lists the file names of one half's profiles, in the order
// given.
func profileNames(sources []source, h half) string {
	return codeList(sourceNames(sources, h))
}

// sourceNames returns the file names of one half's profiles, in the order
// given, which is also how a record names the runs it was folded from.
func sourceNames(sources []source, h half) []string {
	var names []string
	for _, src := range sources {
		if src.half == h {
			names = append(names, filepath.Base(src.path))
		}
	}
	return names
}

// codeList spells names as code, comma-separated.
func codeList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, "`"+name+"`")
	}
	return strings.Join(quoted, ", ")
}

// cell is one half's figure for the row, or absent when none of that half's
// profiles carried a block of the package.
func (r *row) cell(h half) string {
	if !r.present[h] {
		return "absent"
	}
	return r.halves[h].String()
}

// publish prints the report and, when summary names a file, appends it there.
// An output that refuses the report is an error like a summary that cannot
// be written, so a run that delivered no report does not exit 0.
func publish(report, summary string, stdout io.Writer) error {
	if _, err := io.WriteString(stdout, report); err != nil {
		return fmt.Errorf("printing the report: %w", err)
	}
	if summary == "" {
		return nil
	}
	if err := appendSummary(summary, report); err != nil {
		return fmt.Errorf("appending the report to %s: %w", summary, err)
	}
	return nil
}

// appendSummary appends the report to path, on a line of its own and after
// a blank line's worth of separation from whatever follows. It appends
// rather than replaces because the path a workflow passes is
// $GITHUB_STEP_SUMMARY, which every step of a job writes its own section
// to. Text the file already holds that ends without a newline gets one
// first, or the report's heading would continue that text's last line and
// not render as a heading.
func appendSummary(path, report string) error {
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if endsMidLine(file) {
		report = "\n" + report
	}
	if _, err = file.WriteString(report + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// endsMidLine reports whether the file's last byte is something other than
// a newline. An empty file, which a seek back from its end refuses, ends no
// line. The file is opened to append, so reading it here moves nothing the
// write goes to.
func endsMidLine(file *os.File) bool {
	if _, err := file.Seek(-1, io.SeekEnd); err != nil {
		return false
	}
	last := make([]byte, 1)
	n, _ := file.Read(last)
	return n == 1 && last[0] != '\n'
}
