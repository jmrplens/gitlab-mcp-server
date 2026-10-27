// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"slices"
)

// outcome is one binary compared: the SHA-256 of each copy, and when they
// differ, what says why.
type outcome struct {
	asset, target string
	before, after string
	notes         []string
}

// same reports whether the two copies are the same bytes.
func (o outcome) same() bool {
	return o.before == o.after
}

// judge compares two copies of the binary asset, built for target.
func judge(asset, target string, before, after []byte, s sides) outcome {
	o := outcome{asset: asset, target: target, before: digest(before), after: digest(after)}
	if !o.same() {
		o.notes = explain(before, after, s)
	}
	return o
}

// report is what a run found: a line for each binary, then notes on what
// the run did not build, then the verdict.
type report struct {
	sides    sides
	header   string
	outcomes []outcome
	notes    []string
}

// ok reports whether every binary's two copies are the same bytes, which is
// the only verdict that exits 0.
func (r report) ok() bool {
	return !slices.ContainsFunc(r.outcomes, func(o outcome) bool { return !o.same() })
}

// write prints the report.
func (r report) write(w io.Writer) {
	fmt.Fprintf(w, "%s: %s\n", toolName, r.header)
	differ := 0
	for _, o := range r.outcomes {
		if o.same() {
			fmt.Fprintf(w, "%s %s (%s) %s\n", r.sides.same, o.asset, o.target, o.after)
			continue
		}
		differ++
		fmt.Fprintf(w, "DIFFERS %s (%s): %s %s, %s %s\n", o.asset, o.target, r.sides.before, o.before, r.sides.after, o.after)
		for _, note := range o.notes {
			fmt.Fprintf(w, "    %s\n", note)
		}
	}
	for _, note := range r.notes {
		fmt.Fprintln(w, note)
	}
	if differ == 0 {
		fmt.Fprintf(w, "%d of %d binaries %s\n", len(r.outcomes), len(r.outcomes), r.sides.agree)
		return
	}
	fmt.Fprintf(w, "%d of %d binaries %s\n", differ, len(r.outcomes), r.sides.differ)
}
