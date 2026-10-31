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

// judge holds every statement of a document that validated to the scan and
// to go.mod, and every advisory the scan reports that does not affect the
// code to the statements.
//
// What a statement says of this module's own product is held to the current
// tree by [judgeOwn]. What it says of a published image is about the version
// its subcomponent names, which validation requires, and is held to nothing
// the current tree says, since the image is pinned by its digest and does
// not change: see [imageModules].
func judge(rep *report, doc document, mod goModule, result scanResult) {
	rep.scanned, rep.scan = true, result
	for i, st := range doc.Statements {
		subject := fmt.Sprintf("statement %d (%s)", i+1, st.Vulnerability.Name)
		for _, dependency := range ownModules(st, mod.path) {
			judgeOwn(rep, subject, st, mod, result, dependency)
		}
		for _, image := range imageModules(st) {
			rep.stated = append(rep.stated, fmt.Sprintf("%s %s@%s in %s: %s, a published image, about the version it carries and not held to the current dependency set",
				st.Vulnerability.Name, image.path, image.version, image.product, describeStatus(st)))
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
			return st.Vulnerability.Name == key.osv && slices.Contains(ownModules(st, mod.path), key.module)
		})
		if !accounted {
			rep.unaccounted = append(rep.unaccounted, fmt.Sprintf("%s %s@%s, reported %s, and no statement about %s names it: %s",
				key.osv, key.module, result.versions[key], result.levels[key], mod.path, result.summaries[key.osv]))
		}
	}
}

// judgeOwn holds what statement st says of dependency, a module it names
// under this module's own product, to the current tree.
//
// When the scan reports the advisory against the module, the level it
// reached decides: a called function is never stated, since make govulncheck
// fails on it whatever a statement says and the document would only hide
// from other scanners what the build refuses, and a not_affected
// justification the level disproves is contradicted. When the scan does not
// report it, which modules are present decides. A module the scan loads
// without reporting the advisory is at a version, or against a database,
// that no longer carries it, and a module go.mod no longer requires is gone
// altogether: either makes the statement stale. A module go.mod requires
// that no loaded package comes from (one only a test imports, say) is outside
// what the scan can speak for, while a scanner reading go.mod alone still
// reports its advisories, so the statement stands on go.mod's word.
func judgeOwn(rep *report, subject string, st statement, mod goModule, result scanResult, dependency string) {
	key := findingKey{osv: st.Vulnerability.Name, module: dependency}
	reached, found := result.levels[key]
	loaded, inScan := result.modules[dependency]
	required, inGoMod := mod.requires[dependency]
	switch {
	case found && reached == levelSymbol:
		rep.contradicted = append(rep.contradicted, fmt.Sprintf("%s: the scan reports %s against %s %s, which make govulncheck fails on whatever a statement says, so the document does not state it", subject, key.osv, dependency, reached))
	case found && st.Status == statusNotAffected && reached > justifications[st.Justification]:
		rep.contradicted = append(rep.contradicted, fmt.Sprintf("%s: not_affected because %s, and the scan reports %s against %s %s", subject, st.Justification, key.osv, dependency, reached))
	case found:
		rep.stated = append(rep.stated, fmt.Sprintf("%s %s@%s: %s, and the scan reports it %s", key.osv, dependency, result.versions[key], describeStatus(st), reached))
	case inScan:
		rep.stale = append(rep.stale, fmt.Sprintf("%s: the scan loads %s@%s and does not report %s against it, so the statement describes a dependency set that is gone", subject, dependency, loaded, key.osv))
	case inGoMod:
		rep.stated = append(rep.stated, fmt.Sprintf("%s %s@%s: %s, required by go.mod and outside the packages the scan loads, where a scanner reading go.mod alone still reports it", key.osv, dependency, required, describeStatus(st)))
	default:
		rep.stale = append(rep.stale, fmt.Sprintf("%s: go.mod does not require %s and the scan does not load it, so the statement describes a dependency set that is gone", subject, dependency))
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
		fmt.Fprintf(w, "%s: %s, %s, held to a source scan of %s (build tags: %s) against %s (modified %s), and to go.mod\n",
			toolName, r.document, counted(r.statements, "statement", "statements"), strings.Join(r.patterns, " "), tags, r.scan.db, r.scan.dbModified)
	} else {
		fmt.Fprintf(w, "%s: %s, %s, not held to the dependency set until it reads as OpenVEX\n", toolName, r.document, counted(r.statements, "statement", "statements"))
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
	fmt.Fprintf(w, "%s: FAILED: %s with the document, %s, %s, %s unaccounted for\n", toolName,
		counted(len(r.invalid), "problem", "problems"), counted(len(r.stale), "stale statement", "stale statements"),
		counted(len(r.contradicted), "contradicted statement", "contradicted statements"), counted(len(r.unaccounted), "advisory", "advisories"))
}

// counted writes n with the noun that agrees with it.
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
