package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// schemaVersion is the shape of the work list this writes. A reader outside
// this command takes the findings from that file, so a change to the shape has
// to be visible to it.
//
// Version 2 moved an alias written into a related entry or a hint argument out
// of alias_references and into findings, carrying its canonical target in the
// same `canonical` field. Version 3 keeps it there and changes what the other
// bucket means: once -check gates, an alias left in alias_references fails the
// run too, so that list stopped being the one place a spelling was tolerated
// and became the prose half of the same refusal, with declaredAliasMentions
// the only thing that excuses one. It also adds declarations_judged, which is
// about the run rather than the tree. Version 4 adds the `hints` section, the
// rule over the corrective prose an error helper hands a model, which was
// staged at 785 findings and gates now that the tree is clean; its unfolded
// sites are still only reported. Version 5 adds the `e2e_assertions` section,
// the same rule put to the substrings the e2e suite asserts a served text
// carries, with `suite_judged` saying whether the run loaded the suite at all
// and the helper table's own staleness beside it, `helpers_judged` saying
// whether that staleness was judged whole, and `served_judged` saying
// the same of the served tree, which a run naming only suite packages does not
// load; and it renames the keys the
// two prose sections share so they say nothing about hints (`read`,
// `read_by_kind`, `not_folded`, `rows`, `not_folded_sites`). Version 6 widens
// the `hints` section from the error helpers to the served prose: the kinds
// `message`, `next_step`, `param_guidance`, `schema_description` and `usage`
// in `read_by_kind`, the `values_passed_over` count both prose sections
// carry, and the dynamic surface's declarations in `stale_declarations`,
// which now fail the run. The counts move across every one of these lines, so
// a reader comparing two runs across any of them is comparing two rules.
const schemaVersion = 6

// Finding is one published action ID that is not a canonical catalog ID.
type Finding struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	// Closest is the nearest canonical ID, empty when nothing is near enough
	// to be worth naming. It is a lead for whoever fixes the link and never an
	// instruction: the right ID is often a neighbor of a different domain.
	Closest string `json:"closest,omitempty"`
	// Canonical is what an alias resolves to, set on an alias row only.
	Canonical string `json:"canonical,omitempty"`
}

// Unresolved is one site whose value the type checker could not fold.
type Unresolved struct {
	Package    string `json:"package"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Kind       string `json:"kind"`
	Expression string `json:"expression"`
}

// Summary is what the run saw, so a clean report says what it was clean over.
//
// DeclarationsJudged is the one field about the run rather than about the
// tree, and it is here because without it a clean report would be read for
// more than it says. The two declaration tables can only be held to the tree
// by a run that covered the tree: over one package every entry excuses
// nothing, and reporting them all stale would be an answer about the patterns
// rather than about the declarations. A narrowed run therefore leaves them
// unjudged and says so.
type Summary struct {
	DeclarationsJudged bool           `json:"declarations_judged"`
	CatalogIDs         int            `json:"catalog_ids"`
	Aliases            int            `json:"catalog_aliases"`
	Judged             int            `json:"ids_judged"`
	Findings           int            `json:"findings"`
	Packages           int            `json:"packages_with_findings"`
	AliasHits          int            `json:"alias_references"`
	Unresolved         int            `json:"unresolved_sites"`
	Stale              int            `json:"stale_exemptions"`
	ByKind             map[string]int `json:"findings_by_kind"`
	JudgedByKind       map[string]int `json:"judged_by_kind"`
}

// Report is the whole answer, written to stdout and to the work list.
type Report struct {
	SchemaVersion int          `json:"schema_version"`
	Summary       Summary      `json:"summary"`
	Findings      []Finding    `json:"findings"`
	AliasRefs     []Finding    `json:"alias_references"`
	Unresolved    []Unresolved `json:"unresolved"`
	// ServedJudged says whether this run loaded the served tree, for the
	// reason SuiteJudged says it of the suite: a run naming only
	// ./test/e2e/gitlab/ee reads no served source, and a summary printing
	// "judged 0 published ID(s)" and a hint section over 0 hints would read
	// as a clean tree rather than as one nobody looked at. [classify] sets
	// it, since what it is handed is the served tree's sites, and the run
	// clears it when its patterns named no served package.
	ServedJudged bool `json:"served_judged"`
	// Hints is the rule over served prose, under the key it had when it read
	// only the error helpers' hints. Its findings and its stale declarations
	// fail [Report.Clean]; the sites it could not fold and the values it passed
	// over do not.
	Hints HintReport `json:"hints"`
	// SuiteJudged says whether this run loaded the e2e suite, and it is here
	// for the reason DeclarationsJudged is in the summary: a run over
	// ./internal/tools/issues reads no suite, and an assertion section
	// printing "0 finding(s) over 0 assertion(s) read" would read as a clean
	// suite rather than as one nobody looked at. Without it the section is
	// neither printed nor meaningful.
	SuiteJudged bool `json:"suite_judged"`
	// Assertions is the same rule put to the e2e suite's quotations of served
	// text, on the same terms: its findings fail [Report.Clean] and what it
	// could not fold does not.
	Assertions HintReport `json:"e2e_assertions"`
	// StaleHelpers are the entries of the helper table that describe no call,
	// each with what would make it describe one, and CallsByHelper how many
	// calls of each the suite walk met.
	StaleHelpers  []string       `json:"stale_assertion_helpers,omitempty"`
	CallsByHelper map[string]int `json:"assertion_calls_by_helper,omitempty"`
	// HelpersJudged says whether the helper table was judged whole, which
	// only a run over the whole suite does: over part of it every entry that
	// part does not call is called nowhere, so a narrowed run names only the
	// entries whose helper takes no parameter of the declared name. It is here
	// for the reason DeclarationsJudged is, since an empty stale list from a
	// narrowed run would otherwise read as a table found clean.
	HelpersJudged bool `json:"helpers_judged"`
	// StaleExemptions are the declarations that excused nothing, of either
	// table, each named with the table it is in.
	StaleExemptions []string `json:"stale_exemptions,omitempty"`
	// usedExemptions and usedAliasMentions are what the run actually excused,
	// which is what the stale list is computed against.
	usedExemptions    map[string]struct{}
	usedAliasMentions map[string]struct{}
}

// classify holds every site against the catalog and builds the report.
//
// The three outcomes are deliberately kept apart, and two of the three fail
// the gate. A canonical ID is silent. An alias is counted apart because the
// fix differs: gitlab_execute_action resolves it, so the link works when it is
// followed, and gitlab_find_action publishes canonical IDs, so a model that
// looks that name up in a listing does not find it. Anything else resolves
// nowhere at all. Both are held to the same demand, which is the whole point
// of the gate: a field that publishes canonical IDs publishes canonical IDs.
//
// The site still decides one thing, and only one: whether a prose sentence may
// name an alias on purpose. A related entry and a hint argument are structured
// fields the discovery tools hand a model as the ID to call next, so an alias
// there is a cross-link a model can follow once and can never look up, and no
// declaration excuses it. A Usage line is prose, where naming an alias can be
// the whole point of the sentence: issue.update's usage says that dynamic
// execute also accepts issue.close and issue.reopen, which is true and useful.
// Those two are declared in declaredAliasMentions, which only a prose site
// consults.
//
// A site of served prose is judged by neither of those rules and lands in
// [HintReport] instead, since it is a sentence rather than a published ID and
// the question is what it names rather than whether it is one. A Usage line is
// the one site judged in both: its IDs here, and its tool names there
// ([HintReport.judgeUsage]), since it is served on every surface. An assertion site
// lands in a second [HintReport], because it is that prose quoted back by the
// e2e suite: it is held to the same spellings, and kept apart so a reader can
// tell a defect of the server from a defect of its tests.
//
// declarationsJudged says whether the sites cover the whole tree, which is the
// only run that can hold the declaration tables to it.
func classify(sites []site, ids *actionids.IDs, declarationsJudged bool) Report {
	report := Report{
		SchemaVersion: schemaVersion,
		Summary: Summary{
			DeclarationsJudged: declarationsJudged,
			CatalogIDs:         ids.Count(),
			Aliases:            ids.AliasCount(),
			ByKind:             map[string]int{},
			JudgedByKind:       map[string]int{},
		},
		ServedJudged:      true,
		Hints:             newHintReport(),
		Assertions:        newHintReport(),
		usedExemptions:    map[string]struct{}{},
		usedAliasMentions: map[string]struct{}{},
	}
	for _, at := range sites {
		if at.Kind == kindAssertion {
			report.Assertions.judge(at, ids)
			continue
		}
		if isHintKind(at.Kind) {
			report.Hints.judge(at, ids)
			continue
		}
		if !at.Resolved {
			if at.Kind == kindUsage {
				// A Usage line is folded as a sentence, so what it leaves
				// unfolded, and a value a format of it reports, is the
				// served-prose section's to count: a sentence a reader still
				// reads rather than a published ID nobody can.
				report.Hints.judge(at, ids)
				continue
			}
			report.Unresolved = append(report.Unresolved, Unresolved{
				Package: at.Package, File: at.File, Line: at.Line, Kind: at.Kind, Expression: at.Expr,
			})
			continue
		}
		if at.Kind == kindUsage {
			report.Hints.judgeUsage(at)
		}
		if at.Kind == kindDescription {
			report.Hints.judgeDescription(at)
		}
		for _, candidate := range candidateIDs(at, ids) {
			report.judge(at, candidate, ids)
		}
	}
	report.finish()
	return report
}

// judge records one candidate ID under the outcome it deserves.
func (r *Report) judge(at site, candidate string, ids *actionids.IDs) {
	if isProseKind(at.Kind) && exemptProse(candidate) {
		r.usedExemptions[candidate] = struct{}{}
		return
	}
	r.Summary.Judged++
	r.Summary.JudgedByKind[at.Kind]++
	if ids.IsID(candidate) {
		return
	}
	finding := Finding{Package: at.Package, File: at.File, Line: at.Line, Kind: at.Kind, ID: candidate}
	if canonical, isAlias := ids.Alias(candidate); isAlias {
		if isProseKind(at.Kind) && exemptAliasMention(candidate) {
			r.usedAliasMentions[candidate] = struct{}{}
			return
		}
		finding.Canonical = canonical
		if isProseKind(at.Kind) {
			r.AliasRefs = append(r.AliasRefs, finding)
			return
		}
		r.Findings = append(r.Findings, finding)
		r.Summary.ByKind[at.Kind]++
		return
	}
	finding.Closest = ids.Closest(candidate)
	r.Findings = append(r.Findings, finding)
	r.Summary.ByKind[at.Kind]++
}

// finish sorts every list and fills the counts that depend on the whole run.
func (r *Report) finish() {
	sortFindings(r.Findings)
	sortFindings(r.AliasRefs)
	slices.SortFunc(r.Unresolved, func(left, right Unresolved) int {
		return comparePosition(left.Package, left.File, left.Line, right.Package, right.File, right.Line)
	})
	packages := map[string]struct{}{}
	for _, finding := range r.Findings {
		packages[finding.Package] = struct{}{}
	}
	r.Summary.Findings = len(r.Findings)
	r.Summary.Packages = len(packages)
	r.Summary.AliasHits = len(r.AliasRefs)
	r.Summary.Unresolved = len(r.Unresolved)
	if r.Summary.DeclarationsJudged {
		// An alias entry stays alive while dynamic's schema description names
		// it, as well as while a Usage line does ([HintReport.excusesAlias]).
		maps.Copy(r.usedAliasMentions, r.Hints.usedAliasMentions)
		r.StaleExemptions = staleDeclarations(r.usedExemptions, r.usedAliasMentions)
	}
	r.Summary.Stale = len(r.StaleExemptions)
	r.Hints.finish(r.Summary.DeclarationsJudged)
	// The suite's section never judges the tool-name declarations: the table
	// describes the served source, and a quotation is not what it is about.
	r.Assertions.finish(false)
}

// judgeHelpers holds the helper table to what the suite walk met, and marks
// the run as one that loaded the suite.
//
// wholeSuite says whether the suite patterns name the whole suite and nothing
// else ([namesWhole]): the bare run's, one naming it itself, and one naming a
// wildcard that encloses it, ./... or ./test/.... That is the only run that
// can tell a helper nothing calls from a narrowed run.
func (r *Report) judgeHelpers(read suiteRead, wholeSuite bool) {
	r.SuiteJudged = true
	r.HelpersJudged = wholeSuite
	r.CallsByHelper = read.calls
	r.StaleHelpers = staleHelpers(read.calls, read.mismatches, wholeSuite)
}

// Clean reports whether this run found nothing the gate refuses.
//
// Eight things fail it, and the reason each is here rather than reported is
// the same one: a published ID that is not a canonical catalog ID, an ID that
// resolves only as an alias, a declaration that excuses nothing, a site the
// type checker could not fold, served prose that names a tool, a declaration
// of the served-prose rule that excuses nothing, an e2e assertion that quotes
// a tool, and a helper table entry that describes no call.
//
// The unfoldable site is the one that needs saying out loud. It is the audit's
// own blind spot rather than a defect of the tree, and it fails anyway,
// because a gate whose blind spot is silent is one any future site can step
// into: an ID assembled at run time would be reported as unreadable and pass,
// which is the shape every wrong ID would then take.
//
// The hint rule joined them when its count reached zero, which is the order
// this had to happen in: it opened at 785 findings across 137 packages, and a
// gate cannot land before the code it judges is clean. It was widened from
// the error helpers to the rest of the served prose the same way, with the
// tree rewritten in the change below the one that widened it. Its own
// unfoldable sites are counted apart and do not fail, which is the one place
// this departs from the paragraph above, because a sentence the type checker
// cannot fold is text a reader can still read, and neither do the values it
// passes over, which are GitLab's data rather than the server's prose. Its
// declarations fail when they excuse nothing, as every other table's do.
//
// The last two are the suite's, and joined with issue 902. A quotation naming
// a tool is a test that passes against a defective server text and breaks the
// day that text is fixed, which is what issue 901 was, found a month late by
// the licensed run. A helper entry that describes no call is how a renamed
// helper or parameter would stop every one of its calls being read, and a
// gate whose reading can stop in silence is not one. The suite's unfoldable
// needles fail nothing, on the hint rule's terms: a needle built from a
// fixture's name at run time carries no literal to judge.
func (r *Report) Clean() bool {
	return r.Summary.Findings == 0 && r.Summary.AliasHits == 0 &&
		r.Summary.Unresolved == 0 && r.Summary.Stale == 0 &&
		r.Hints.Findings == 0 && len(r.Hints.StaleDeclarations) == 0 &&
		r.Assertions.Findings == 0 && len(r.StaleHelpers) == 0
}

// sortFindings orders findings by position, then by the ID, so two runs over
// one tree produce the same file.
func sortFindings(findings []Finding) {
	slices.SortFunc(findings, func(left, right Finding) int {
		return cmp.Or(
			comparePosition(left.Package, left.File, left.Line, right.Package, right.File, right.Line),
			cmp.Compare(left.ID, right.ID),
		)
	})
}

// comparePosition orders two source positions: package, then file, then line.
//
// A comparison rather than a "less" predicate, and the difference is not
// style. The predicate form reads "if the packages differ, return leftPkg <
// rightPkg", and that < carries a boundary no input reaches: inside "they
// differ", < and <= cannot be told apart. Mutation testing reported one
// survivor per field that way, in this function and at every call site that
// repeated its shape, and none of them was a test anybody could have written.
func comparePosition(leftPkg, leftFile string, leftLine int, rightPkg, rightFile string, rightLine int) int {
	return cmp.Or(
		cmp.Compare(leftPkg, rightPkg),
		cmp.Compare(leftFile, rightFile),
		cmp.Compare(leftLine, rightLine),
	)
}

// candidateIDs picks the strings one site offers a model as an action ID.
//
// A related entry and a hint argument are the ID whole. A Usage line or a
// description is prose, and which of its dotted tokens is offered as an ID is
// the shared rule in cmd/internal/actionids, the same one the documentation
// gate applies to a page.
func candidateIDs(at site, ids *actionids.IDs) []string {
	switch at.Kind {
	case kindRelated, kindHint:
		if strings.TrimSpace(at.Value) == "" {
			return nil
		}
		return []string{at.Value}
	default:
		// A Usage line assembled by a format keeps its verbs in the value, for
		// the fixer, and they are masked here for the reason [maskVerbs]
		// gives: "%s.get" spells a dotted token whose right half is an action
		// name.
		return ids.Candidates(maskVerbs(at.Value))
	}
}

// isProseKind reports whether a site's IDs were picked out of model-facing
// prose rather than written as an ID.
func isProseKind(kind string) bool {
	return kind == kindUsage || kind == kindDescription
}

// writeReport prints the human report: the findings under the package that has
// to act on them, then the alias references and the sites nothing could fold,
// then what the run saw.
//
// The alias references, the unfolded sites and the rows of the two prose
// sections are printed whatever -v says, because all of them fail the gate
// and a gate's log has to name what it refused. -v decides only how much of
// what fails nothing is shown: the breakdowns by kind, and the prose sites
// the type checker could not fold.
//
// Each tree's sections are printed only by a run that loaded it, since a
// count over nothing reads as a clean tree. A run over ./internal/tools/...
// alone prints no suite section; a run naming only suite packages prints one
// line in place of the published-ID summary and the hint section, saying
// those rules were not run.
func writeReport(out io.Writer, report Report, verbose bool) {
	if report.ServedJudged {
		writeServedReport(out, report, verbose)
	} else {
		fmt.Fprintf(out, "%s: no served source loaded: the published-ID and served-prose rules were not run\n", toolName)
	}
	if report.SuiteJudged {
		writeSuiteReport(out, report, verbose)
	}
}

// writeServedReport prints what the run found in the served tree: the
// findings, the alias references, the sites nothing folded, the declarations
// that excuse nothing, the summary, and the hint section.
func writeServedReport(out io.Writer, report Report, verbose bool) {
	writeGroups(out, report.Findings, findingVerb)
	if len(report.AliasRefs) > 0 || verbose {
		fmt.Fprintln(out, "=== registered aliases, not catalog IDs ===")
		writeGroups(out, report.AliasRefs, aliasVerb)
	}
	writeUnresolved(out, report.Unresolved)
	writeStale(out, report.StaleExemptions)
	writeSummary(out, report.Summary, verbose)
	writeHintReport(out, report.Hints, verbose)
}

// helpersNotJudgedLine is what a run over part of the suite says in place of
// the helper table's whole verdict.
const helpersNotJudgedLine = "  the assertion helper table was not judged whole: only a run over the whole suite can tell an entry nothing calls from a narrowed run"

// writeSuiteReport prints what the run found in the e2e suite: its
// quotations, then the helper table entries that describe no call, which fail
// the gate and so are printed whatever -v says, then under -v how many calls
// of each helper were read.
//
// Each stale entry carries its own remedy, unlike a stale declaration, because
// the two kinds have different ones: an entry nothing calls is the table's to
// fix, and a copy of a helper that takes no parameter of the entry's name is
// the helper's (see [staleHelpers]).
//
// A run over part of the suite says it did not judge the table whole, as the
// served summary says it of the declaration tables, since its stale list can
// hold no entry nothing calls and would otherwise read as a table found clean.
func writeSuiteReport(out io.Writer, report Report, verbose bool) {
	writeAssertionReport(out, report.Assertions, verbose)
	if len(report.StaleHelpers) > 0 {
		fmt.Fprintln(out, "=== assertion helpers that describe no call ===")
		for _, entry := range report.StaleHelpers {
			fmt.Fprintf(out, "  %s.\n", entry)
		}
	}
	if !report.HelpersJudged {
		fmt.Fprintln(out, helpersNotJudgedLine)
	}
	if verbose && len(report.CallsByHelper) > 0 {
		fmt.Fprintf(out, "    assertion calls by helper: %s\n", byCount(report.CallsByHelper))
	}
}

// writeStale prints the declarations that excused nothing, which is a finding
// of its own: a declaration that has stopped describing the tree is one a
// reader would otherwise trust.
func writeStale(out io.Writer, stale []string) {
	if len(stale) == 0 {
		return
	}
	fmt.Fprintln(out, "=== declarations that excuse nothing ===")
	for _, entry := range stale {
		fmt.Fprintf(out, "  %s. Remove the entry.\n", entry)
	}
}

// writeGroups prints findings under one `=== package ===` heading each, each
// row read with the verb its own outcome deserves.
//
// The verb is per row rather than per list because the findings list holds two
// outcomes: an ID nothing resolves, and an alias written where a catalog ID
// belongs. Printing the second under the first's verb would say a string
// resolves to nothing while naming, in the same row, what it resolves to.
func writeGroups(out io.Writer, findings []Finding, verb func(Finding) string) {
	current := ""
	for _, finding := range findings {
		if finding.Package != current {
			current = finding.Package
			fmt.Fprintf(out, "=== %s ===\n", current)
		}
		fmt.Fprintf(out, "  %s:%d %s %q %s%s\n",
			finding.File, finding.Line, finding.Kind, finding.ID, verb(finding), trailer(finding))
	}
}

// findingVerb reads one finding: an alias standing in for the canonical ID, or
// a string the catalog has never heard of.
func findingVerb(finding Finding) string {
	if finding.Canonical != "" {
		return "is an alias, not the catalog ID"
	}
	return "resolves to no action"
}

// aliasVerb reads a row of the prose bucket, where the spelling is refused for
// being an alias rather than for resolving nowhere.
func aliasVerb(Finding) string { return "alias of" }

// trailer renders whatever a finding knows beyond the ID itself.
func trailer(finding Finding) string {
	if finding.Canonical != "" {
		return " " + finding.Canonical
	}
	if finding.Closest != "" {
		return "; closest: " + finding.Closest
	}
	return ""
}

// writeUnresolved prints the sites the type checker could not fold, which are
// the audit's own blind spot rather than a clean answer, and which the gate
// refuses for exactly that reason: spell the ID as a constant.
func writeUnresolved(out io.Writer, unresolved []Unresolved) {
	if len(unresolved) == 0 {
		return
	}
	fmt.Fprintln(out, "=== not folded ===")
	for _, at := range unresolved {
		fmt.Fprintf(out, "  %s:%d %s %s\n", at.File, at.Line, at.Kind, at.Expression)
	}
}

// writeSummary prints what the run saw.
func writeSummary(out io.Writer, summary Summary, verbose bool) {
	fmt.Fprintf(out, "%s: %d published ID(s) to fix in %d package(s); %d alias(es) named in prose; %d site(s) not folded; %d stale declaration(s)\n",
		toolName, summary.Findings, summary.Packages, summary.AliasHits, summary.Unresolved, summary.Stale)
	fmt.Fprintf(out, "  judged %d published ID(s) against %d catalog ID(s) and %d alias(es)\n",
		summary.Judged, summary.CatalogIDs, summary.Aliases)
	if !summary.DeclarationsJudged {
		fmt.Fprintln(out, "  the declaration tables were not judged: only a run over the whole tree can tell a stale entry from a narrowed run")
	}
	if len(summary.ByKind) > 0 {
		fmt.Fprintf(out, "  findings by kind: %s\n", byCount(summary.ByKind))
	}
	if verbose && len(summary.JudgedByKind) > 0 {
		fmt.Fprintf(out, "  judged by kind: %s\n", byCount(summary.JudgedByKind))
	}
}

// byCount renders a breakdown, biggest first, so a report opens with where the
// work is. Two kinds with the same count are ordered by name, so two runs over
// one tree print the same line.
//
// The order is expressed as a comparison rather than as a "less" predicate
// guarded by an inequality. The guarded form reads the same and carries a
// boundary no input reaches: inside "the counts differ", > and >= cannot be
// told apart, so mutation testing reports a survivor that no test could ever
// kill. A comparison has no boundary to get wrong.
func byCount(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(left, right string) int {
		return cmp.Or(cmp.Compare(counts[right], counts[left]), cmp.Compare(left, right))
	})
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

// writeJSON writes the machine-readable work list, creating its directory when
// it is not there.
//
// The list is written even when it is empty, so a later run cannot read a file
// a previous run left behind and take it for today's answer.
func writeJSON(path string, report any) error {
	// filepath.Dir answers "." for a bare file name and never the empty
	// string, so the only directory worth not creating is the one that means
	// "here".
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if writeErr := os.WriteFile(path, append(data, '\n'), 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	return nil
}
