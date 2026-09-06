package main

import (
	"go/types"
	"sort"
)

// Finding is one value reaching a Markdown construct with nothing between it
// and the page.
type Finding struct {
	Package    string `json:"package"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Func       string `json:"func"`
	Context    string `json:"context"`
	Verb       string `json:"verb"`
	Expression string `json:"expression"`
	Wants      string `json:"wants"`
	Reason     string `json:"reason"`
}

// Report is the work list one run produces.
type Report struct {
	Findings []Finding `json:"findings"`
	// Unresolved holds the values the audit could not follow to either
	// answer. They are not failures by default and are not safe either: they
	// are the audit's own blind spots, listed so they can be looked at.
	Unresolved []Finding `json:"unresolved"`
	// Excused holds the findings a source directive declared safe, so an
	// exemption is reviewable rather than invisible.
	Excused         []Finding   `json:"excused"`
	StaleDirectives []Directive `json:"stale_directives"`
	Summary         Summary     `json:"summary"`
}

// Summary aggregates what the sweep saw, including the two breakdowns the walk
// through the findings is organized by.
type Summary struct {
	Contexts   string         `json:"contexts"`
	Sinks      int            `json:"sinks"`
	Holes      int            `json:"holes"`
	Safe       int            `json:"safe"`
	Findings   int            `json:"findings"`
	Unresolved int            `json:"unresolved"`
	Excused    int            `json:"excused"`
	Stale      int            `json:"stale_directives"`
	Packages   int            `json:"packages"`
	ByContext  map[string]int `json:"findings_by_context"`
	ByPackage  map[string]int `json:"findings_by_package"`
}

// audit classifies every value that lands in one of the selected Markdown
// constructs and returns the work list.
func audit(prog *program, sel selection, root string) Report {
	c := newClassifier(prog)
	directives := collectDirectives(prog, root)
	used := make(map[directiveKey]bool, len(directives))
	report := Report{Summary: Summary{Contexts: sel.label}}
	for _, s := range collectSinks(prog) {
		report.Summary.Sinks++
		auditSink(prog, c, s, sel, root, directives, used, &report)
	}
	report.StaleDirectives = staleDirectives(directives, used)
	finish(&report)
	return report
}

// auditSink classifies every hole of one sink.
func auditSink(prog *program, c *classifier, s sink, sel selection, root string,
	directives map[directiveKey]Directive, used map[directiveKey]bool, report *Report,
) {
	for _, h := range s.holes {
		if !sel.judges(h.ctx) {
			continue
		}
		report.Summary.Holes++
		got, why := c.classifyExpr(s.pkg, h.expr, nil, 0)
		if got == safe {
			report.Summary.Safe++
			continue
		}
		finding := newFinding(prog, s, h, why, root)
		key := directiveKey{pkg: finding.Package, expression: finding.Expression}
		if _, excused := directives[key]; excused {
			used[key] = true
			report.Excused = append(report.Excused, finding)
			continue
		}
		if got == unescaped {
			report.Findings = append(report.Findings, finding)
			continue
		}
		report.Unresolved = append(report.Unresolved, finding)
	}
}

// finish counts and orders what the sweep collected, so two runs over one tree
// produce the same work list.
func finish(report *Report) {
	sortFindings(report.Findings)
	sortFindings(report.Unresolved)
	sortFindings(report.Excused)
	report.Summary.ByContext = map[string]int{}
	report.Summary.ByPackage = map[string]int{}
	packages := map[string]bool{}
	for _, finding := range report.Findings {
		report.Summary.ByContext[finding.Context]++
		report.Summary.ByPackage[finding.Package]++
		packages[finding.Package] = true
	}
	report.Summary.Findings = len(report.Findings)
	report.Summary.Unresolved = len(report.Unresolved)
	report.Summary.Excused = len(report.Excused)
	report.Summary.Stale = len(report.StaleDirectives)
	report.Summary.Packages = len(packages)
}

// newFinding renders one classified hole as a reportable finding.
//
// The expression is printed as go/types renders it, on one line, because a
// finding has to name the value rather than a position: it is what the report
// groups by, and what an exemption directive is written by copying.
func newFinding(prog *program, s sink, h sinkHole, why, root string) Finding {
	pos := prog.position(h.expr.Pos())
	return Finding{
		Package:    shortPackage(s.pkg.PkgPath),
		File:       relativePath(pos.Filename, root),
		Line:       pos.Line,
		Func:       enclosingFunc(s.pkg, s.call.Pos()),
		Context:    h.ctx.String(),
		Verb:       h.verb,
		Expression: types.ExprString(h.expr),
		Wants:      h.ctx.wants(),
		Reason:     why,
	}
}

// sortFindings orders findings by package, file, line and expression.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		return findingLess(findings[i], findings[j])
	})
}

// findingLess is the order two runs of the audit have to agree on.
func findingLess(a, b Finding) bool {
	switch {
	case a.Package != b.Package:
		return a.Package < b.Package
	case a.File != b.File:
		return a.File < b.File
	case a.Line != b.Line:
		return a.Line < b.Line
	default:
		return a.Expression < b.Expression
	}
}

// sortDirectives orders declared exemptions the way findings are ordered.
func sortDirectives(directives []Directive) {
	sort.Slice(directives, func(i, j int) bool {
		if directives[i].File != directives[j].File {
			return directives[i].File < directives[j].File
		}
		return directives[i].Line < directives[j].Line
	})
}
