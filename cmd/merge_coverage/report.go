// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
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
	b.WriteString("\nA half reads absent when none of its profiles carries a block of the package.\n")
	return b.String()
}

// profileNames lists the file names of one half's profiles, in the order
// given.
func profileNames(sources []source, h half) string {
	var names []string
	for _, src := range sources {
		if src.half == h {
			names = append(names, "`"+filepath.Base(src.path)+"`")
		}
	}
	return strings.Join(names, ", ")
}

// cell is one half's figure for the row, or absent when none of that half's
// profiles carried a block of the package.
func (r *row) cell(h half) string {
	if !r.present[h] {
		return "absent"
	}
	return r.halves[h].String()
}

// appendSummary appends the report to path, after a blank line's worth of
// separation from whatever follows. It appends rather than replaces because
// the path a workflow passes is $GITHUB_STEP_SUMMARY, which every step of a
// job writes its own section to.
func appendSummary(path, report string) error {
	// #nosec G304 -- the path is the one the caller named on the command line.
	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.WriteString(report + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
