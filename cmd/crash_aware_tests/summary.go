// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// reporter writes a report to the log and, when GitHub names one, to the job
// summary, which is where a reader of a run looks for what it decided.
type reporter struct {
	stdout, stderr io.Writer
	// path is the job summary file, empty off CI.
	path string
	// warned records that the summary could not be written, so the warning
	// is printed once rather than once per line.
	warned bool
}

// summary writes lines, one per line, to stdout and appends them to the job
// summary. A summary that cannot be written costs the record and not the
// verdict: the run goes on, and the failure is warned about once.
func (r *reporter) summary(lines ...string) {
	text := strings.Join(lines, "\n") + "\n"
	_, _ = io.WriteString(r.stdout, text)
	if r.path == "" {
		return
	}
	if err := appendFile(r.path, text); err != nil && !r.warned {
		r.warned = true
		fmt.Fprintf(r.stderr, "WARNING: could not write the job summary %s: %v\n", r.path, err)
	}
}

// appendFile appends text to the file at path, creating it when it is not
// there, as the runner's own summary file is created.
func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //#nosec G304 -- the job summary file GitHub names
	if err != nil {
		return err
	}
	_, err = f.WriteString(text)
	return errors.Join(err, f.Close())
}
