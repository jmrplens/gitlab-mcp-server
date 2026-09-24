package main

import (
	"cmp"
	"fmt"
	"io"
	"regexp"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// toolToken matches a tool-name-shaped token in prose, which is the
// documentation gate's own rule (cmd/audit_doc_tool_names) spelled again here.
//
// The two are deliberately not shared, because they answer different
// questions of the same token: the documentation gate asks whether some
// surface registers that name and refuses the ones none does, while this rule
// refuses every one of them, registered or not. Both gate now, so the token
// rule belongs in cmd/internal/actionids beside the ID rule the two gates
// already share, and moving it there is the consolidation still open.
var toolToken = regexp.MustCompile(`\bgitlab_[a-z0-9_]+\b`)

// printfVerb matches one verb of a fmt format, flags, width, precision and
// argument index included.
//
// The space flag is left out on purpose. It is valid Go ("% d") and nothing in
// this tree writes it, while prose writes "50% of" constantly, which with the
// flag reads as the verb "% o" and loses the word after it; before a tool name
// ("% gitlab_x") it would hide the name, which is the opposite of the point.
var printfVerb = regexp.MustCompile(`%[-+#0]*(?:\[\d+\])?(?:\d+|\*)?(?:\.(?:\d+|\*)?)?(?:\[\d+\])?[a-zA-Z%]`)

// maskVerbs replaces every printf verb of a text with a space, which is what
// a format is judged as.
//
// A format is kept verbatim in a site's value, for the fixer, and read as
// written it misleads both rules: "%s.get" spells the dotted token s.get,
// whose right half is an action name the catalog uses, and "%sgitlab_x" hides
// the tool name behind a word character. A space in the verb's place is what
// the verb is to a reader, a word the sentence does not spell, and it is only
// the judged text that is masked: the value the fixer matches literals
// against keeps its verbs.
func maskVerbs(text string) string {
	return printfVerb.ReplaceAllString(text, " ")
}

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
	// Kind is the site: the argument of an error helper, the struct field a
	// hint is written into on its way to one, or a substring the e2e suite
	// asserts a served text carries.
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

// HintReport is a rule over prose that names capabilities, and the report
// carries two of them: the corrective prose a handler hands a model, and the
// e2e suite's quotations of that prose. Both gate on their findings, and
// neither gates on what it could not fold.
//
// One type for both, because the two are one question put to two corpora: a
// quotation is held to the spellings the text it quotes is held to, and a
// second judge written for it is a judge that could drift from the first. The
// JSON keys say nothing about hints for the same reason, since the suite's
// section carries the same keys.
//
// What its findings cost is bounded and known: a hint that names a tool no
// surface registers sends a model after a call it cannot make, which it reads
// as the capability being absent rather than as the sentence being wrong; and
// a quotation that names one is a test that breaks the day the server is
// fixed, as twenty of them did in issue 901.
type HintReport struct {
	// Read is how many strings were folded and judged, and ReadByKind splits
	// that between the kinds of site, because the field site of the hint rule
	// is the wider net of its two and a reader should be able to tell which
	// figure is which.
	Read       int            `json:"read"`
	ReadByKind map[string]int `json:"read_by_kind"`
	Packages   int            `json:"packages_with_findings"`
	Findings   int            `json:"findings"`
	ByRule     map[string]int `json:"findings_by_rule"`
	// Unfolded is how many sites the type checker could not fold. They are
	// this rule's own blind spot, named rather than passed over, and unlike the
	// ID gate's they fail nothing.
	Unfolded int `json:"not_folded"`
	// PassedOver is how many values a sentence reports were deliberately not
	// read: the arguments of a format that are neither constants nor named as
	// prose, and the writes of a message field that read another struct's
	// field. It is a count and never a list, because the list would be
	// GitLab's data from end to end (see [walker.foldFormat]); a figure that
	// moves is what says the walk reads less than it did.
	PassedOver int           `json:"values_passed_over"`
	Rows       []HintFinding `json:"rows,omitempty"`
	NotFolded  []Unresolved  `json:"not_folded_sites,omitempty"`
	// StaleDeclarations are the tool-name declarations that excused nothing,
	// of both tables, filled only by a run over the whole tree, since over one
	// package every entry excuses nothing and reporting them all would be an
	// answer about the patterns rather than about the declarations. The
	// suite's section never fills it: the tables describe the served source.
	StaleDeclarations []string `json:"stale_declarations,omitempty"`
	// usedToolExemptions and usedSurfaceMentions are what the run excused,
	// which the stale list is computed against. Each section keeps its own, so
	// a quotation spelling a declared token cannot keep that declaration alive
	// for the served source. usedAliasMentions is the declared aliases the
	// served prose named where it may ([HintReport.excusesAlias]), which the
	// published-ID section's stale judgement of declaredAliasMentions counts
	// beside its own Usage lines.
	usedToolExemptions  map[string]struct{}
	usedSurfaceMentions map[surfaceMention]struct{}
	usedAliasMentions   map[string]struct{}
}

// judge records what one site names.
//
// A site that could not be folded is kept apart from the ID gate's own
// unfolded list, so that this rule's blind spot cannot fail a build the gate
// would have passed, and a value passed over is counted and not judged.
func (h *HintReport) judge(at site, ids *actionids.IDs) {
	if at.PassedOver {
		h.PassedOver++
		return
	}
	if !at.Resolved {
		h.NotFolded = append(h.NotFolded, Unresolved{
			Package: at.Package, File: at.File, Line: at.Line, Kind: at.Kind, Expression: at.Expr,
		})
		return
	}
	h.Read++
	h.ReadByKind[at.Kind]++
	text := maskVerbs(at.Value)
	h.judgeToolNames(at, text)
	h.judgeIDs(at, text, ids)
}

// judgeUsage holds a Usage line to the tool-name rule, and to nothing else.
//
// A Usage line is served on every surface (the dynamic find and describe
// results, the meta tool's description and gitlab://tools), so a tool name in
// it is wrong for two surfaces of three, exactly as in a hint. Its dotted IDs
// are the ID section's to judge, where declaredAliasMentions excuses the
// issue.close and issue.reopen the issue.update line names on purpose, so
// judging them here too would count every bad ID twice and refuse those two.
// The verbs of a line a format assembled are masked, as they are for every
// sentence this section judges. A Description is judged by
// [HintReport.judgeDescription].
func (h *HintReport) judgeUsage(at site) {
	h.Read++
	h.ReadByKind[at.Kind]++
	h.judgeToolNames(at, maskVerbs(at.Value))
}

// judgeDescription holds an individual tool's Description to the tool-name
// rule everywhere but its "See also" clause, the span
// [actioncatalog.SeeAlsoClause] matches: the one definition internal/resources
// rewrites per surface as well, so the part skipped here is always the part
// the manifests rewrite.
//
// A domain action's Description is not served by its individual tool alone:
// gitlab://tools serves it verbatim as the description of that action's entry
// on the dynamic and meta surfaces too, rewriting only the "See also" clause
// into each surface's names. So a tool name anywhere else in it reaches two
// surfaces that do not register the tool, as a name in a Usage line does, and
// the clause is the one part where the individual tool name is right, since
// it is the part the manifests project. A standalone surface tool's
// Description is served verbatim on every surface and is read the same way;
// its clause names canonical IDs, which the published-ID rule judges. Its
// dotted IDs are that rule's to judge here as well, for the reason
// [HintReport.judgeUsage] gives.
//
// Only a constant Description reaches this. One assembled at run time (a
// switch over the action name, a map read) is read by no rule of this
// command; TestToolManifest_ServedDescriptions_NameNoToolOutsideTheirSeeAlsoClause
// in internal/resources holds every Description the built catalog carries to
// the same, whatever produced it.
func (h *HintReport) judgeDescription(at site) {
	h.Read++
	h.ReadByKind[at.Kind]++
	h.judgeToolNames(at, actioncatalog.SeeAlsoClause.ReplaceAllString(at.Value, " "))
}

// judgeToolNames records every gitlab_* name one site's judged text spells,
// once each: a sentence naming the same tool twice is one thing to fix.
func (h *HintReport) judgeToolNames(at site, text string) {
	seen := map[string]struct{}{}
	for _, name := range toolToken.FindAllString(text, -1) {
		if _, repeated := seen[name]; repeated {
			continue
		}
		seen[name] = struct{}{}
		if exemptHintTool(name) {
			h.usedToolExemptions[name] = struct{}{}
			continue
		}
		if mention := (surfaceMention{pkg: at.Package, tool: name}); exemptSurfaceMention(mention) {
			h.usedSurfaceMentions[mention] = struct{}{}
			continue
		}
		h.addFinding(at, ruleToolName, name, HintFinding{})
	}
}

// judgeIDs records every dotted token one site offers as an action ID that is
// not the canonical one.
//
// Which tokens are offered as IDs is the shared rule in cmd/internal/actionids,
// and the prose exemptions are the gate's own table, consulted here without
// marking an entry used: the staleness judgement belongs to the rule that
// gates, and a hint keeping a declaration alive would make that judgement say
// something false about the Usage lines it is written about.
//
// A quotation consults declaredAliasMentions on the same terms and a hint
// does not. A hint is prose the server writes, and it writes canonical IDs; a
// quotation is whatever the server wrote, and a Usage line may name one of
// those aliases by design, so a test quoting that line would otherwise be
// refused for quoting it faithfully. The schema descriptions of
// internal/tools/dynamic consult it too, for the reason the Usage line does:
// the one that names an alias is the description of dynamic execute's action
// parameter, whose subject is that execute accepts one
// ([HintReport.excusesAlias]).
func (h *HintReport) judgeIDs(at site, text string, ids *actionids.IDs) {
	for _, token := range ids.Candidates(text) {
		if ids.IsID(token) || exemptProse(token) {
			continue
		}
		if canonical, isAlias := ids.Alias(token); isAlias {
			if h.excusesAlias(at, token) {
				continue
			}
			h.addFinding(at, ruleAlias, token, HintFinding{Canonical: canonical})
			continue
		}
		h.addFinding(at, ruleUnknownID, token, HintFinding{Closest: ids.Closest(token)})
	}
}

// excusesAlias reports whether a site may name a registered alias because
// declaredAliasMentions declares it, and records the use where the site is
// one the declaration is written about.
//
// Two sites may. A quotation consults the table without keeping an entry
// alive, since what it quotes is the served source and the entry is about
// that. A schema description of internal/tools/dynamic keeps its entry alive,
// because that description is one the entry now names as its reason: were the
// Usage line that first asked for the entry rewritten, the entry would read as
// stale while dynamic's description still needed it, and removing it would
// fail the description instead. The description is scoped to that package, as
// [declaredSurfaceToolMentions] is: anywhere else a description naming an
// alias is a sentence in a spelling no listing publishes.
func (h *HintReport) excusesAlias(at site, token string) bool {
	if !exemptAliasMention(token) {
		return false
	}
	if at.Kind == kindAssertion {
		return true
	}
	if at.Kind != kindSchemaDescription || at.Package != dynamicPackage {
		return false
	}
	h.usedAliasMentions[token] = struct{}{}
	return true
}

// addFinding records one finding, taking its position from the site and
// whatever else the rule knows from detail.
func (h *HintReport) addFinding(at site, rule, name string, detail HintFinding) {
	detail.Package, detail.File, detail.Line = at.Package, at.File, at.Line
	detail.Kind, detail.Rule, detail.Name = at.Kind, rule, name
	h.Rows = append(h.Rows, detail)
	h.ByRule[rule]++
}

// finish sorts the lists and fills the counts that depend on the whole run.
//
// declarationsJudged says whether the sites cover the whole tree, which is the
// only run that can hold the declaration table to it.
func (h *HintReport) finish(declarationsJudged bool) {
	if declarationsJudged {
		h.StaleDeclarations = append(staleHintDeclarations(h.usedToolExemptions),
			staleSurfaceMentions(h.usedSurfaceMentions)...)
	}
	slices.SortFunc(h.Rows, func(left, right HintFinding) int {
		return cmp.Or(
			comparePosition(left.Package, left.File, left.Line, right.Package, right.File, right.Line),
			cmp.Compare(left.Name, right.Name),
		)
	})
	slices.SortFunc(h.NotFolded, func(left, right Unresolved) int {
		return comparePosition(left.Package, left.File, left.Line, right.Package, right.File, right.Line)
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
		ReadByKind:          map[string]int{},
		ByRule:              map[string]int{},
		usedToolExemptions:  map[string]struct{}{},
		usedSurfaceMentions: map[surfaceMention]struct{}{},
		usedAliasMentions:   map[string]struct{}{},
	}
}

// The headings the report opens its lists with: the rows under every run, the
// sites nothing folded under -v alone. Each is written only over a list that
// has something in it, so a run that found nothing announces no section.
const (
	hintRowsHeading           = "=== capabilities named in served prose by a spelling no listing publishes ==="
	hintNotFoldedHeading      = "=== served prose not folded ==="
	assertionRowsHeading      = "=== e2e assertions quoting a spelling no listing publishes ==="
	assertionNotFoldedHeading = "=== e2e assertions not folded ==="
)

// proseLabels are the words one section of prose findings is printed with,
// so the hint section and the suite's section share one printer and differ
// only in what they call themselves.
type proseLabels struct {
	// count opens the count line, and unit names what was read.
	count string
	unit  string
	// byRule and byKind caption the two breakdowns.
	byRule string
	byKind string
	// rowsHeading and notFoldedHeading open the two lists.
	rowsHeading      string
	notFoldedHeading string
}

// hintLabels and assertionLabels are the two sections the report prints.
var (
	hintLabels = proseLabels{
		count: "served prose", unit: "sentence(s)",
		byRule: "served prose findings by rule", byKind: "served prose read by kind",
		rowsHeading: hintRowsHeading, notFoldedHeading: hintNotFoldedHeading,
	}
	assertionLabels = proseLabels{
		count: "e2e assertions", unit: "assertion(s)",
		byRule: "assertion findings by rule", byKind: "assertions read by kind",
		rowsHeading: assertionRowsHeading, notFoldedHeading: assertionNotFoldedHeading,
	}
)

// writeHintReport prints what the rule over served prose found.
func writeHintReport(out io.Writer, hints HintReport, verbose bool) {
	writeProseSection(out, hintLabels, hints, verbose)
}

// writeAssertionReport prints what the rule over the e2e suite's quotations
// found.
func writeAssertionReport(out io.Writer, assertions HintReport, verbose bool) {
	writeProseSection(out, assertionLabels, assertions, verbose)
}

// writeProseSection prints one section of prose findings under its labels.
//
// The rows are printed by every run, because every one of them fails the gate:
// check-action-ids passes no -v, and a red job whose log carried a count and a
// rule name and no file, line or needle would send its reader to run the audit
// again to learn what to fix. What -v adds is what fails nothing, the sites
// the type checker could not fold and the breakdown by kind, which is the
// split writeReport makes for the published IDs too. The rows are in the JSON
// either way.
func writeProseSection(out io.Writer, labels proseLabels, section HintReport, verbose bool) {
	if len(section.Rows) > 0 {
		fmt.Fprintln(out, labels.rowsHeading)
		writeHintGroups(out, section.Rows)
	}
	if verbose && len(section.NotFolded) > 0 {
		fmt.Fprintln(out, labels.notFoldedHeading)
		for _, at := range section.NotFolded {
			fmt.Fprintf(out, "  %s:%d %s %s\n", at.File, at.Line, at.Kind, at.Expression)
		}
	}
	for _, entry := range section.StaleDeclarations {
		fmt.Fprintf(out, "  %s. Remove the entry.\n", entry)
	}
	fmt.Fprintf(out, "  %s: %d finding(s) in %d package(s) over %d %s read; %d not folded (reported, not gated); %d value(s) passed over\n",
		labels.count, section.Findings, section.Packages, section.Read, labels.unit, section.Unfolded, section.PassedOver)
	if len(section.ByRule) > 0 {
		fmt.Fprintf(out, "    %s: %s\n", labels.byRule, byCount(section.ByRule))
	}
	if verbose && len(section.ReadByKind) > 0 {
		fmt.Fprintf(out, "    %s: %s\n", labels.byKind, byCount(section.ReadByKind))
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
