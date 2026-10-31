// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
)

// report is the verdict of one run.
type report struct {
	document   string
	statements int
	// scanned says whether the document was held to a scan, which it is
	// only once it reads as OpenVEX.
	scanned  bool
	scan     scanResult
	patterns []string
	tags     string

	stated       []string
	invalid      []string
	stale        []string
	contradicted []string
	unaccounted  []string
}

// judge holds every statement of a document that validated to the scan, and
// every advisory the scan reports that does not affect the code to the
// statements.
func judge(rep *report, doc document, module string, result scanResult) {
	rep.scanned, rep.scan = true, result
	for i, st := range doc.Statements {
		subject := fmt.Sprintf("statement %d (%s)", i+1, st.Vulnerability.Name)
		for _, dependency := range statedModules(st) {
			key := findingKey{osv: st.Vulnerability.Name, module: dependency}
			reached, found := result.levels[key]
			switch {
			case !found:
				rep.stale = append(rep.stale, fmt.Sprintf("%s: the dependency set does not contain %s against %s, so the statement describes one that is gone", subject, key.osv, dependency))
			case st.Status == statusNotAffected && reached > justifications[st.Justification]:
				rep.contradicted = append(rep.contradicted, fmt.Sprintf("%s: not_affected because %s, and the scan reports %s against %s %s", subject, st.Justification, key.osv, dependency, reached))
			default:
				rep.stated = append(rep.stated, fmt.Sprintf("%s %s@%s: %s, and the scan reports it %s", key.osv, dependency, result.versions[key], describeStatus(st), reached))
			}
		}
	}
	keys := make([]findingKey, 0, len(result.levels))
	for key, reached := range result.levels {
		if reached < levelSymbol {
			keys = append(keys, key)
		}
	}
	slices.SortFunc(keys, func(a, b findingKey) int { return cmp.Or(cmp.Compare(a.osv, b.osv), cmp.Compare(a.module, b.module)) })
	for _, key := range keys {
		accounted := slices.ContainsFunc(doc.Statements, func(st statement) bool {
			return st.Vulnerability.Name == key.osv && statesForModule(st, module, key.module)
		})
		if !accounted {
			rep.unaccounted = append(rep.unaccounted, fmt.Sprintf("%s %s@%s, reported %s, and no statement about %s names it: %s",
				key.osv, key.module, result.versions[key], result.levels[key], module, result.summaries[key.osv]))
		}
	}
}

// describeStatus names a statement's status, with its justification when it
// has one.
func describeStatus(st statement) string {
	if st.Justification == "" {
		return st.Status
	}
	return st.Status + " (" + st.Justification + ")"
}

// ok reports whether the run passes.
func (r *report) ok() bool {
	return len(r.invalid)+len(r.stale)+len(r.contradicted)+len(r.unaccounted) == 0
}

// write prints what was checked, every finding, and the verdict.
func (r *report) write(w io.Writer) {
	if r.scanned {
		tags := r.tags
		if tags == "" {
			tags = "none"
		}
		fmt.Fprintf(w, "%s: %s, %d statements, held to a source scan of %s (build tags: %s) against %s (modified %s)\n",
			toolName, r.document, r.statements, strings.Join(r.patterns, " "), tags, r.scan.db, r.scan.dbModified)
	} else {
		fmt.Fprintf(w, "%s: %s, %d statements, not held to the dependency set until it reads as OpenVEX\n", toolName, r.document, r.statements)
	}
	for _, line := range r.stated {
		fmt.Fprintf(w, "stated %s\n", line)
	}
	for _, sections := range []struct {
		label string
		lines []string
	}{
		{"INVALID", r.invalid}, {"STALE", r.stale}, {"CONTRADICTED", r.contradicted}, {"UNACCOUNTED", r.unaccounted},
	} {
		for _, line := range sections.lines {
			fmt.Fprintf(w, "%s %s\n", sections.label, line)
		}
	}
	if r.ok() {
		fmt.Fprintf(w, "%s: every statement holds, and every advisory the scan reports against code it does not affect is stated\n", toolName)
		return
	}
	fmt.Fprintf(w, "%s: FAILED: %d problems with the document, %d stale and %d contradicted statements, %d advisories unaccounted for\n",
		toolName, len(r.invalid), len(r.stale), len(r.contradicted), len(r.unaccounted))
}
