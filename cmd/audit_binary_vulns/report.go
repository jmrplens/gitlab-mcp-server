package main

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

// scanned is one built binary and what govulncheck said about it.
type scanned struct {
	binary binary
	result scanResult
}

// judged is one advisory against one module, merged across every binary that
// carries it, and the declaration that accepts it, if one does.
type judged struct {
	osv, module, version, fixed, summary string
	targets                              []string
	declaration                          *declaration
}

// report is the verdict of one run.
type report struct {
	scanned  []scanned
	findings []judged
	stale    []string
	invalid  []string
}

// judge merges the findings of every binary and holds them to the table.
//
// Every declaration is judged for staleness against the whole run, because a
// run always scans every target the release builds: a declaration no binary
// needed is one that has outlived what it excused.
func judge(scans []scanned, declared map[string]declaration) report {
	merged := map[string]*judged{}
	var order []string
	for _, s := range scans {
		for _, f := range s.result.findings {
			key := declarationKey(f.osv, f.module)
			j := merged[key]
			if j == nil {
				j = &judged{osv: f.osv, module: f.module, version: f.version, fixed: f.fixed, summary: s.result.summaries[f.osv]}
				merged[key] = j
				order = append(order, key)
			}
			if name := s.binary.target.String(); !slices.Contains(j.targets, name) {
				j.targets = append(j.targets, name)
			}
		}
	}

	rep := report{scanned: scans, invalid: invalidDeclarations(declared)}
	sort.Strings(order)
	for _, key := range order {
		j := merged[key]
		if entry, ok := declared[key]; ok {
			j.declaration = &entry
		}
		rep.findings = append(rep.findings, *j)
	}
	for key := range declared {
		if merged[key] == nil {
			rep.stale = append(rep.stale, key)
		}
	}
	sort.Strings(rep.stale)
	return rep
}

// undeclared counts the findings no declaration accepts.
func (r report) undeclared() int {
	n := 0
	for _, f := range r.findings {
		if f.declaration == nil {
			n++
		}
	}
	return n
}

// ok reports whether the run passes: nothing undeclared, nothing stale, and
// every declaration readable.
func (r report) ok() bool {
	return r.undeclared() == 0 && len(r.stale) == 0 && len(r.invalid) == 0
}

// write prints what was scanned, every finding, and the verdict.
func (r report) write(w io.Writer) {
	for _, s := range r.scanned {
		fmt.Fprintf(w, "%s: %s %s built with %s, %d modules, checked against %s (modified %s)\n",
			toolName, s.binary.build, s.binary.target, s.result.goVersion, s.result.modules,
			s.result.db, s.result.dbModified)
	}
	for _, f := range r.findings {
		fixed := "no fixed version"
		if f.fixed != "" {
			fixed = "fixed in " + f.fixed
		}
		subject := fmt.Sprintf("%s %s@%s (%s) in %s", f.osv, f.module, f.version, fixed, strings.Join(f.targets, ", "))
		if f.declaration != nil {
			fmt.Fprintf(w, "accepted %s [%s]: %s\n", subject, f.declaration.category, f.declaration.reason)
			continue
		}
		fmt.Fprintf(w, "FINDING %s: %s\n", subject, f.summary)
	}
	for _, key := range r.stale {
		fmt.Fprintf(w, "STALE declaration %q: no binary of this run carries the finding it accepts\n", key)
	}
	for _, problem := range r.invalid {
		fmt.Fprintf(w, "INVALID declaration %s\n", problem)
	}
	if r.ok() {
		if len(r.findings) == 0 {
			fmt.Fprintf(w, "%s: no advisory affects a module the %d binaries link\n", toolName, len(r.scanned))
			return
		}
		fmt.Fprintf(w, "%s: %d findings in the %d binaries, each accepted by a declaration\n", toolName, len(r.findings), len(r.scanned))
		return
	}
	fmt.Fprintf(w, "%s: FAILED: %d undeclared findings, %d stale and %d invalid declarations\n",
		toolName, r.undeclared(), len(r.stale), len(r.invalid))
}
