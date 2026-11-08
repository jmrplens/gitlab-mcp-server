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
// not render as a heading. The file is opened only to write, since a
// summary that can be written and not read still takes the report, and the
// guard reads it where it can.
func appendSummary(path, report string) error {
	path = filepath.Clean(path)
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if endsMidLine(path) {
		report = "\n" + report
	}
	if _, err = file.WriteString(report + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// endsMidLine reports whether the last byte of the file at path is something
// other than a newline. It reads on a descriptor of its own, opened only to
// read, because the summary need only be writable: a file whose mode grants
// no read, or /dev/stdout on macOS, where a descriptor reopened through
// /dev/fd may ask for no more than the original grants, refuses the read and
// still takes the report. A file it cannot open, or seek back from the end
// of (an empty one, a pipe), ends no line as far as it can tell, so the
// report goes in with nothing before it. It is called once the file is open
// to write, so a named pipe, which opens to read only when it has a writer,
// already has one.
func endsMidLine(path string) bool {
	last := []byte{'\n'}
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.Open(path)
	if err == nil {
		if _, err = file.Seek(-1, io.SeekEnd); err == nil {
			_, _ = file.Read(last)
		}
		_ = file.Close()
	}
	return last[0] != '\n'
}
