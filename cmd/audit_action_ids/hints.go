package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// toolToken matches a tool-name-shaped token in prose, which is the
// documentation gate's own rule (cmd/audit_doc_tool_names) spelled again here.
//
// The two are deliberately not shared yet, because they answer different
// questions of the same token: the documentation gate asks whether some
// surface registers that name and refuses the ones none does, while this rule
// refuses every one of them, registered or not. If this ever gates, the token
// rule belongs in cmd/internal/actionids beside the ID rule the two gates
// already share.
var toolToken = regexp.MustCompile(`\bgitlab_[a-z0-9_]+\b`)

// The three ways a hint can name a capability that the session reading it
// cannot look up.
const (
	// ruleToolName is a gitlab_* tool name. It is a finding whatever surface
	// happens to register it: the default dynamic surface registers no such
	// tool at all, and on meta only the bare domain tools exist, so the name
	// is at best right for one of three surfaces. The canonical action ID is
	// the portable form, for the same reason it is in a documentation example.
	ruleToolName = "tool_name"
	// ruleAlias is a registered alias rather than the canonical ID.
	// gitlab_execute_action resolves one, so a model that follows it gets
	// through; gitlab_find_action publishes canonical IDs, so a model that
	// looks it up in a listing does not find it.
	ruleAlias = "alias"
	// ruleUnknownID is a dotted ID the catalog resolves nowhere at all.
	ruleUnknownID = "unknown_id"
)

// HintFinding is one capability a hint names in a form the session reading it
// cannot call.
type HintFinding struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	// Kind is the site: the argument of an error helper, or the struct field a
	// hint is written into on its way to one.
	Kind string `json:"kind"`
	// Rule says which of the three spellings this is.
	Rule string `json:"rule"`
	// Name is what the hint spelled.
	Name string `json:"name"`
	// Closest is the nearest canonical ID to an unknown one, empty when
	// nothing is near enough to be worth naming. It is a lead and never an
	// instruction.
	Closest string `json:"closest,omitempty"`
	// Canonical is what an alias resolves to, set on an alias row only.
	Canonical string `json:"canonical,omitempty"`
}

// HintReport is the staged rule over the corrective prose a handler hands a
// model, and it reports rather than gates.
//
// It reports because the class it names is everywhere: a gate cannot land
// before the code it judges is clean, and this is the first run that says how
// much of it there is. What it costs to leave ungated is bounded and known: a
// hint that names a tool no surface registers sends a model after a call it
// cannot make, which it reads as the capability being absent rather than as
// the sentence being wrong.
type HintReport struct {
	// Read is how many hint strings were folded and judged, and ReadByKind
	// splits that between the two sites, because the field site is the wider
	// net of the two and a reader should be able to tell which figure is which.
	Read       int            `json:"hints_read"`
	ReadByKind map[string]int `json:"hints_read_by_kind"`
	Packages   int            `json:"packages_with_findings"`
	Findings   int            `json:"findings"`
	ByRule     map[string]int `json:"findings_by_rule"`
	// Unfolded is how many hint sites the type checker could not fold. They are
	// this rule's own blind spot, named rather than passed over, and unlike the
	// gate's they fail nothing.
	Unfolded  int           `json:"hints_not_folded"`
	Rows      []HintFinding `json:"hint_findings,omitempty"`
	NotFolded []Unresolved  `json:"hints_not_folded_sites,omitempty"`
	// StaleDeclarations are the tool-name declarations that excused nothing,
	// filled only by a run over the whole tree, since over one package every
	// entry excuses nothing and reporting them all would be an answer about
	// the patterns rather than about the declarations.
	StaleDeclarations []string `json:"stale_declarations,omitempty"`
	// usedToolExemptions is what the run excused, which the stale list is
	// computed against.
	usedToolExemptions map[string]struct{}
}

// judgeHint records what one hint site names.
//
// A hint that could not be folded is kept apart from the gate's own unfolded
// list, so that this rule's blind spot cannot fail a build the gate would have
// passed.
func (r *Report) judgeHint(at site, ids *actionids.IDs) {
	if !at.Resolved {
		r.Hints.NotFolded = append(r.Hints.NotFolded, Unresolved{
			Package: at.Package, File: at.File, Line: at.Line, Kind: at.Kind, Expression: at.Expr,
		})
		return
	}
	r.Hints.Read++
	r.Hints.ReadByKind[at.Kind]++
	r.judgeHintToolNames(at)
	r.judgeHintIDs(at, ids)
}

// judgeHintToolNames records every gitlab_* name one hint spells, once each:
// a sentence naming the same tool twice is one thing to fix.
func (r *Report) judgeHintToolNames(at site) {
	seen := map[string]struct{}{}
	for _, name := range toolToken.FindAllString(at.Value, -1) {
		if _, repeated := seen[name]; repeated {
			continue
		}
		seen[name] = struct{}{}
		if exemptHintTool(name) {
			r.Hints.usedToolExemptions[name] = struct{}{}
			continue
		}
		r.addHintFinding(at, ruleToolName, name, HintFinding{})
	}
}

// judgeHintIDs records every dotted token one hint offers as an action ID that
// is not the canonical one.
//
// Which tokens are offered as IDs is the shared rule in cmd/internal/actionids,
// and the prose exemptions are the gate's own table, consulted here without
// marking an entry used: the staleness judgement belongs to the rule that
// gates, and a hint keeping a declaration alive would make that judgement say
// something false about the Usage lines it is written about.
func (r *Report) judgeHintIDs(at site, ids *actionids.IDs) {
	for _, token := range ids.Candidates(at.Value) {
		if ids.IsID(token) || exemptProse(token) {
			continue
		}
		if canonical, isAlias := ids.Alias(token); isAlias {
			r.addHintFinding(at, ruleAlias, token, HintFinding{Canonical: canonical})
			continue
		}
		r.addHintFinding(at, ruleUnknownID, token, HintFinding{Closest: ids.Closest(token)})
	}
}

// addHintFinding records one finding, taking its position from the site and
// whatever else the rule knows from detail.
func (r *Report) addHintFinding(at site, rule, name string, detail HintFinding) {
	detail.Package, detail.File, detail.Line = at.Package, at.File, at.Line
	detail.Kind, detail.Rule, detail.Name = at.Kind, rule, name
	r.Hints.Rows = append(r.Hints.Rows, detail)
	r.Hints.ByRule[rule]++
}

// finish sorts the lists and fills the counts that depend on the whole run.
//
// declarationsJudged says whether the sites cover the whole tree, which is the
// only run that can hold the declaration table to it.
func (h *HintReport) finish(declarationsJudged bool) {
	if declarationsJudged {
		h.StaleDeclarations = staleHintDeclarations(h.usedToolExemptions)
	}
	sort.Slice(h.Rows, func(i, j int) bool {
		if h.Rows[i].Package != h.Rows[j].Package ||
			h.Rows[i].File != h.Rows[j].File ||
			h.Rows[i].Line != h.Rows[j].Line {
			return lessPosition(h.Rows[i].Package, h.Rows[i].File, h.Rows[i].Line,
				h.Rows[j].Package, h.Rows[j].File, h.Rows[j].Line)
		}
		return h.Rows[i].Name < h.Rows[j].Name
	})
	sort.Slice(h.NotFolded, func(i, j int) bool {
		return lessPosition(h.NotFolded[i].Package, h.NotFolded[i].File, h.NotFolded[i].Line,
			h.NotFolded[j].Package, h.NotFolded[j].File, h.NotFolded[j].Line)
	})
	packages := map[string]struct{}{}
	for _, row := range h.Rows {
		packages[row.Package] = struct{}{}
	}
	h.Findings = len(h.Rows)
	h.Packages = len(packages)
	h.Unfolded = len(h.NotFolded)
}

// newHintReport prepares the counters one run fills.
func newHintReport() HintReport {
	return HintReport{
		ReadByKind:         map[string]int{},
		ByRule:             map[string]int{},
		usedToolExemptions: map[string]struct{}{},
	}
}

// writeHintReport prints what the staged rule found.
//
// The count is printed by every run and the rows only by a verbose one, which
// is the split between what a gate's log should carry and what a report is
// read for: check-action-ids runs this on every push and the backlog is
// hundreds of rows, while audit-action-ids passes -v and is where the work
// list is read from. The rows are in the JSON either way.
func writeHintReport(out io.Writer, hints HintReport, verbose bool) {
	if verbose && len(hints.Rows) > 0 {
		fmt.Fprintln(out, "=== capabilities named in a hint by a spelling no listing publishes ===")
		writeHintGroups(out, hints.Rows)
	}
	if verbose && len(hints.NotFolded) > 0 {
		fmt.Fprintln(out, "=== hints not folded ===")
		for _, at := range hints.NotFolded {
			fmt.Fprintf(out, "  %s:%d %s %s\n", at.File, at.Line, at.Kind, at.Expression)
		}
	}
	for _, entry := range hints.StaleDeclarations {
		fmt.Fprintf(out, "  %s. Remove the entry.\n", entry)
	}
	fmt.Fprintf(out, "  error hints: %d finding(s) in %d package(s) over %d hint(s) read; %d not folded (reported, not gated)\n",
		hints.Findings, hints.Packages, hints.Read, hints.Unfolded)
	if len(hints.ByRule) > 0 {
		fmt.Fprintf(out, "    hint findings by rule: %s\n", byCount(hints.ByRule))
	}
	if verbose && len(hints.ReadByKind) > 0 {
		fmt.Fprintf(out, "    hints read by kind: %s\n", byCount(hints.ReadByKind))
	}
}

// writeHintGroups prints the hint findings under one `=== package ===` heading
// each, every row read with the verb its own rule deserves.
func writeHintGroups(out io.Writer, rows []HintFinding) {
	current := ""
	for _, row := range rows {
		if row.Package != current {
			current = row.Package
			fmt.Fprintf(out, "=== %s ===\n", current)
		}
		fmt.Fprintf(out, "  %s:%d %s %q %s\n", row.File, row.Line, row.Kind, row.Name, hintVerb(row))
	}
}

// hintVerb reads one hint finding: a tool name no surface is guaranteed to
// register, an alias standing in for the canonical ID, or a string the catalog
// has never heard of.
func hintVerb(row HintFinding) string {
	switch row.Rule {
	case ruleToolName:
		return "is a tool name; the dynamic surface registers no such tool"
	case ruleAlias:
		return "is an alias, not the catalog ID: " + row.Canonical
	default:
		if row.Closest != "" {
			return "resolves to no action; closest: " + row.Closest
		}
		return "resolves to no action"
	}
}
