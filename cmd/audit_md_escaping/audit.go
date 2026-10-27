// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"go/types"
	"slices"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
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
	run := auditPass{
		prog:       prog,
		classifier: newClassifier(prog),
		sel:        sel,
		root:       root,
		directives: collectDirectives(prog, root),
		report:     Report{Summary: Summary{Contexts: sel.label}},
	}
	run.used = make(map[directiveKey]bool, len(run.directives))
	for _, s := range collectSinks(prog) {
		run.report.Summary.Sinks++
		run.auditSink(s)
	}
	run.report.StaleDirectives = staleDirectives(run.directives, run.used, sel)
	finish(&run.report)
	return run.report
}

// auditPass is one pass over the program: everything the classification of a
// hole consults, and the report it accumulates into. It exists so the per-sink
// step reads as one step rather than as a signature.
type auditPass struct {
	prog       *program
	classifier *classifier
	sel        selection
	root       string
	directives map[directiveKey]Directive
	used       map[directiveKey]bool
	report     Report
}

// auditSink judges every hole of one sink: the card shape, which is a finding
// as it stands; the escaping verdict, on where the value lands; and the raw
// verdict, on what the value is. The last two are asked of the same hole
// independently, so a timestamp excused for escaping is still reported for
// display, and each is excused by its own directive.
//
// A catalog render whose key is not a constant is a finding whatever the run
// judges: with no template, none of its placeholders can be placed, so no
// selection of contexts could say the render reaches none of them.
func (r *auditPass) auditSink(s sink) {
	for _, h := range s.holes {
		if h.ctx == ctxCatalogKey {
			r.report.Summary.Holes++
			r.report.Findings = append(r.report.Findings, newFinding(r.prog, s, h,
				"the key is not a constant, so the template the values land in cannot be read", r.root))
			continue
		}
		judged := false
		if h.ctx == ctxCard && r.sel.judges(ctxCard) {
			judged = true
			r.auditCard(s, h)
		}
		if h.escapable() && r.sel.judges(h.ctx) {
			judged = true
			r.auditEscaping(s, h)
		}
		if h.rawJudged() && r.sel.judges(ctxRaw) {
			judged = true
			r.auditRaw(s, h)
		}
		if judged {
			r.report.Summary.Holes++
		}
	}
}

// auditEscaping applies the escaping verdict to one hole.
func (r *auditPass) auditEscaping(s sink, h sinkHole) {
	got, why := r.verdictOf(s, h)
	if got == safe {
		r.report.Summary.Safe++
		return
	}
	finding := newFinding(r.prog, s, h, why, r.root)
	if r.excused(kindUnescaped, finding) {
		return
	}
	if got == unescaped {
		r.report.Findings = append(r.report.Findings, finding)
		return
	}
	r.report.Unresolved = append(r.report.Unresolved, finding)
}

// verdictOf classifies the value one hole carries.
//
// A placeholder of a catalog render the call does not show the value of is
// the exception, unresolved for the reason the hole carries: the expression
// standing in it is an argument the call does not name a placeholder by, so
// following it would answer a question about some other placeholder, or a
// message whose arguments the call does not show, so following it would
// answer a question about the catalog's own machinery.
func (r *auditPass) verdictOf(s sink, h sinkHole) (got verdict, why string) {
	if h.unbound != "" {
		return unresolved, h.unbound
	}
	return r.classifier.classifyExpr(s.pkg, h.expr, nil, 0)
}

// auditCard applies the card verdict to one hole.
//
// The excuse is looked up by the enclosing function rather than by the
// finding's own expression, which is the row text: a row carries a colon of
// its own and so cannot be written as a directive expression, and the claim a
// declaration makes here is about the whole shape a function writes rather
// than about one of its lines. See [cardDirective].
func (r *auditPass) auditCard(s sink, h sinkHole) {
	finding := newFinding(r.prog, s, h,
		"a card row written by hand; Card writes the row, escapes the value and keeps the layout", r.root)
	if r.excusedBy(kindCard, finding.Func, finding) {
		return
	}
	r.report.Findings = append(r.report.Findings, finding)
}

// auditRaw applies the second verdict to one hole. A value that is neither a
// flag nor an instant is not counted safe here, because the question was not
// whether it is safe; it is simply not a finding.
func (r *auditPass) auditRaw(s sink, h sinkHole) {
	kind, why := r.classifier.rawVerdict(s.pkg, h)
	if kind == rawNone {
		return
	}
	finding := newFinding(r.prog, s, sinkHole{expr: h.expr, ctx: ctxRaw, verb: h.verb}, why, r.root)
	finding.Wants = kind.wants()
	if r.excused(kindRaw, finding) {
		return
	}
	r.report.Findings = append(r.report.Findings, finding)
}

// excused records a finding a directive of the given kind declares safe, and
// reports whether it did. The directive is the one naming the finding's own
// expression, which is what every verdict but the card shape is keyed by.
func (r *auditPass) excused(kind directiveKind, finding Finding) bool {
	return r.excusedBy(kind, finding.Expression, finding)
}

// excusedBy is the same lookup against a subject the caller names, for a
// verdict whose finding is not keyed by the expression it prints.
func (r *auditPass) excusedBy(kind directiveKind, subject string, finding Finding) bool {
	key := directiveKey{pkg: finding.Package, kind: kind, expression: subject}
	if _, declared := r.directives[key]; !declared {
		return false
	}
	r.used[key] = true
	r.report.Excused = append(r.report.Excused, finding)
	return true
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
// groups by, and what an exemption directive is written by copying. A
// card-shaped line names the line instead, since it has no value of its own.
func newFinding(prog *program, s sink, h sinkHole, why, root string) Finding {
	pos := prog.position(h.expr.Pos())
	expression := h.text
	if expression == "" {
		expression = types.ExprString(h.expr)
	}
	return Finding{
		Package:    shortPackage(s.pkg.PkgPath),
		File:       goprogram.RelativePath(pos.Filename, root),
		Line:       pos.Line,
		Func:       enclosingFunc(s.pkg, s.call.Pos()),
		Context:    h.ctx.String(),
		Verb:       h.verb,
		Expression: expression,
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
//
// The fields are compared through cmp.Or rather than a switch of inequalities,
// and the order is the same: the first field that differs decides. A switch
// left nothing a mutation run could hold. Its case conditions sit outside every
// block Go's coverage counts, and the < under a case that has already found the
// two fields different reads exactly like a <=. Here the only comparison is the
// final one, and two equal findings tell its two spellings apart.
func findingLess(a, b Finding) bool {
	return cmp.Or(
		cmp.Compare(a.Package, b.Package),
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Expression, b.Expression),
	) < 0
}

// sortDirectives orders declared exemptions the way findings are ordered, by
// file and then line.
//
// It sorts by a comparison rather than by a less function, whose < no test
// could tell from a <=: under the test that the files differ the two agree,
// and on the line only a tie tells them apart. Two directives tie only when a
// //line directive maps both onto one line of one file, since each is a line
// comment of its own, and they arrive in map order, so a tie has no order to
// keep: slices.SortFunc orders exactly as sort.Slice did, ties included.
func sortDirectives(directives []Directive) {
	slices.SortFunc(directives, func(a, b Directive) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
}
