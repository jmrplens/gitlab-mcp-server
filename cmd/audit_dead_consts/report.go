package main

import (
	"fmt"
	"io"
	"sort"
)

// Summary is what a run looked at, so a clean report says what it was clean
// over rather than only that it was clean.
type Summary struct {
	Packages int `json:"packages"`
	Declared int `json:"declared"`
	Findings int `json:"findings"`
	InGroup  int `json:"findings_in_a_group"`
	Stale    int `json:"stale_declarations"`
}

// Report is the whole answer.
type Report struct {
	Summary  Summary    `json:"summary"`
	Findings []Constant `json:"findings"`
	// Stale are the declarations that excused nothing this run, on the terms
	// every declaration table in this repository is held to: one that has
	// stopped describing the tree is itself a finding.
	Stale []string `json:"stale_declarations,omitempty"`
}

// buildReport holds what the scan found against the declaration table.
//
// scanned is what the run looked at, and the stale rule is scoped to it: a
// declaration naming a package this run never loaded is neither used nor
// stale, only out of view. Without that scope every run over one package
// would report the whole table as stale, which is the opposite of what the
// table is for.
func buildReport(found []Constant, declaredCount int, scanned map[string]struct{}) Report {
	report := Report{Summary: Summary{Packages: len(scanned), Declared: declaredCount}}
	excused := map[string]struct{}{}
	for _, constant := range found {
		key := declarationKey(constant)
		if _, allowed := unreadOnPurpose[key]; allowed {
			excused[key] = struct{}{}
			continue
		}
		report.Findings = append(report.Findings, constant)
		if constant.GroupSize > 1 {
			report.Summary.InGroup++
		}
	}
	report.Stale = staleDeclarations(excused, scanned)
	report.Summary.Findings = len(report.Findings)
	report.Summary.Stale = len(report.Stale)
	return report
}

// ok reports whether the run found nothing to answer for.
func (r Report) ok() bool { return len(r.Findings) == 0 && len(r.Stale) == 0 }

// write renders the report.
func (r Report) write(out io.Writer, verbose bool) {
	for _, constant := range r.Findings {
		fmt.Fprintf(out, "%s:%d: %s is never read (%s)\n",
			constant.File, constant.Line, constant.Name, groupNote(constant.GroupSize))
	}
	for _, key := range r.Stale {
		fmt.Fprintf(out, "%s: declared unread on purpose, and this run found it read or found it gone\n", key)
	}
	if verbose || !r.ok() {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s: %d unexported constants in %d packages, %d never read (%d of those in a group the linter cannot see)\n",
		toolName, r.Summary.Declared, r.Summary.Packages, r.Summary.Findings, r.Summary.InGroup)
}

// groupNote says whether the linter already had its chance at a finding. A
// constant on its own is one staticcheck's unused reports; one sharing a
// declaration with a constant that is read is the class this rule exists for.
func groupNote(groupSize int) string {
	if groupSize <= 1 {
		return "declared on its own"
	}
	return fmt.Sprintf("1 of %d in its const declaration", groupSize)
}

// sortConstants puts findings in a stable order: by file, then by line.
func sortConstants(found []Constant) {
	sort.Slice(found, func(i, j int) bool {
		if found[i].File != found[j].File {
			return found[i].File < found[j].File
		}
		return found[i].Line < found[j].Line
	})
}
