package main

import "strings"

// exemptionDirective is how a value that needs no escaping is declared: in the
// source, in the package that owns the formatter, never in a list this audit
// carries.
//
//	//gitlab:allow-unescaped <expression>: <reason>
//
// The expression is the one a finding prints, so an exemption is written by
// copying the value the report named. It excuses that expression wherever the
// package interpolates it, which is what a value repeated across a domain's
// eight formatters needs, and a directive that excuses nothing is itself
// reported, so one left behind by a later change cannot quietly widen the
// gate.
//
// Escaping a value that needs no escaping is not free, which is why this
// exists at all rather than a blanket instruction to wrap everything: a
// canonical catalog ID compiled in from an ActionSpec is not GitLab-authored
// text, and wrapping it teaches the next reader a rule that is not the rule.
const exemptionDirective = "//gitlab:allow-unescaped"

// rawDirective is the same declaration for the second verdict:
//
//	//gitlab:allow-raw <expression>: <reason>
//
// It is a directive of its own rather than a second meaning of the first, so
// that a value excused for escaping (a timestamp cannot carry a pipe) is not
// thereby excused for display (it is still printed as GitLab sent it). The
// two verdicts ask different questions and each is answered on its own.
const rawDirective = "//gitlab:allow-raw"

// directiveKind names which verdict a directive answers.
type directiveKind string

const (
	// kindUnescaped excuses the escaping verdict.
	kindUnescaped directiveKind = "unescaped"
	// kindRaw excuses the bool-or-time verdict.
	kindRaw directiveKind = "raw"
)

// directivePrefixes maps each directive's spelling to the verdict it answers.
var directivePrefixes = map[string]directiveKind{
	exemptionDirective: kindUnescaped,
	rawDirective:       kindRaw,
}

// Directive is one declared exemption, kept with where it was declared so a
// stale one can be pointed at.
type Directive struct {
	Package    string        `json:"package"`
	File       string        `json:"file"`
	Line       int           `json:"line"`
	Kind       directiveKind `json:"kind"`
	Expression string        `json:"expression"`
	Reason     string        `json:"reason"`
}

// directiveKey identifies the findings one directive excuses: an expression,
// in the package that writes it, for one verdict.
type directiveKey struct {
	pkg        string
	kind       directiveKind
	expression string
}

// collectDirectives reads every exemption declared in the loaded packages.
func collectDirectives(prog *program, root string) map[directiveKey]Directive {
	found := map[directiveKey]Directive{}
	for _, pkg := range prog.order {
		for _, file := range pkg.Syntax {
			for _, group := range file.Comments {
				for _, comment := range group.List {
					kind, expression, reason, ok := parseDirective(comment.Text)
					if !ok {
						continue
					}
					pos := prog.position(comment.Pos())
					key := directiveKey{pkg: shortPackage(pkg.PkgPath), kind: kind, expression: expression}
					found[key] = Directive{
						Package:    key.pkg,
						File:       relativePath(pos.Filename, root),
						Line:       pos.Line,
						Kind:       kind,
						Expression: expression,
						Reason:     reason,
					}
				}
			}
		}
	}
	return found
}

// parseDirective splits one comment into the verdict it answers, the
// expression it excuses and the reason given for it.
//
// Both halves are required. A directive with no reason is not read as an
// exemption at all, because the reason is the whole value of the mechanism:
// the next reader has to be able to tell a value that needs no escaping from
// one somebody decided not to escape.
func parseDirective(text string) (kind directiveKind, expression, reason string, ok bool) {
	text = strings.TrimSpace(text)
	for prefix, candidate := range directivePrefixes {
		rest, isDirective := strings.CutPrefix(text, prefix)
		if !isDirective {
			continue
		}
		excused, why, hasReason := strings.Cut(strings.TrimSpace(rest), ":")
		excused = strings.TrimSpace(excused)
		why = strings.TrimSpace(why)
		if excused == "" || !hasReason || why == "" {
			return "", "", "", false
		}
		return candidate, excused, why, true
	}
	return "", "", "", false
}

// staleDirectives lists the exemptions that excused nothing this run.
//
// A raw directive is judged stale only when the raw verdict ran: the rule it
// answers is staged, and a run that never asked the question has no grounds
// to say the answer was not needed.
func staleDirectives(declared map[directiveKey]Directive, used map[directiveKey]bool, judgedRaw bool) []Directive {
	var stale []Directive
	for key, directive := range declared {
		if used[key] {
			continue
		}
		if key.kind == kindRaw && !judgedRaw {
			continue
		}
		stale = append(stale, directive)
	}
	sortDirectives(stale)
	return stale
}

// shortPackage renders an import path as the repository-relative path a report
// prints.
func shortPackage(path string) string {
	return strings.TrimPrefix(path, modulePath+"/")
}
