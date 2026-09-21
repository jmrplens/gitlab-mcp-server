package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
// sites are still only reported. The counts move across all three lines, so a
// reader comparing two runs across any of them is comparing two rules.
const schemaVersion = 4

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
	// Hints is the staged rule over corrective prose, which reports and never
	// gates, so nothing it holds is read by [Report.Clean].
	Hints HintReport `json:"hints"`
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
// A hint site is judged by neither of those rules and lands in [HintReport]
// instead. It is corrective prose rather than a published ID, the class it
// names is everywhere in the tree, and a gate cannot land before the code it
// judges is clean, so that half reports and nothing in it reaches
// [Report.Clean].
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
		Hints:             newHintReport(),
		usedExemptions:    map[string]struct{}{},
		usedAliasMentions: map[string]struct{}{},
	}
	for _, at := range sites {
		if isHintKind(at.Kind) {
			report.judgeHint(at, ids)
			continue
		}
		if !at.Resolved {
			report.Unresolved = append(report.Unresolved, Unresolved{
				Package: at.Package, File: at.File, Line: at.Line, Kind: at.Kind, Expression: at.Expr,
			})
			continue
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
	sort.Slice(r.Unresolved, func(i, j int) bool {
		return lessPosition(r.Unresolved[i].Package, r.Unresolved[i].File, r.Unresolved[i].Line,
			r.Unresolved[j].Package, r.Unresolved[j].File, r.Unresolved[j].Line)
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
		r.StaleExemptions = staleDeclarations(r.usedExemptions, r.usedAliasMentions)
	}
	r.Summary.Stale = len(r.StaleExemptions)
	r.Hints.finish(r.Summary.DeclarationsJudged)
}

// Clean reports whether this run found nothing the gate refuses.
//
// Five things fail it, and the reason each is here rather than reported is the
// same one: a published ID that is not a canonical catalog ID, an ID that
// resolves only as an alias, a declaration that excuses nothing, a site the
// type checker could not fold, and a hint that names a tool.
//
// The unfoldable site is the one that needs saying out loud. It is the audit's
// own blind spot rather than a defect of the tree, and it fails anyway,
// because a gate whose blind spot is silent is one any future site can step
// into: an ID assembled at run time would be reported as unreadable and pass,
// which is the shape every wrong ID would then take.
//
// The hint rule joined them when its count reached zero, which is the order
// this had to happen in: it opened at 785 findings across 137 packages, and a
// gate cannot land before the code it judges is clean. Its own unfoldable
// sites are counted apart and do not fail, which is the one place this departs
// from the paragraph above, because a hint the type checker cannot fold is
// text a reader can still read: three sites build one from a function call or
// a format string and carry no tool name between them.
func (r *Report) Clean() bool {
	return r.Summary.Findings == 0 && r.Summary.AliasHits == 0 &&
		r.Summary.Unresolved == 0 && r.Summary.Stale == 0 &&
		r.Hints.Findings == 0
}

// sortFindings orders findings by position, then by the ID, so two runs over
// one tree produce the same file.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Package != findings[j].Package ||
			findings[i].File != findings[j].File ||
			findings[i].Line != findings[j].Line {
			return lessPosition(findings[i].Package, findings[i].File, findings[i].Line,
				findings[j].Package, findings[j].File, findings[j].Line)
		}
		return findings[i].ID < findings[j].ID
	})
}

// lessPosition orders two source positions.
func lessPosition(leftPkg, leftFile string, leftLine int, rightPkg, rightFile string, rightLine int) bool {
	if leftPkg != rightPkg {
		return leftPkg < rightPkg
	}
	if leftFile != rightFile {
		return leftFile < rightFile
	}
	return leftLine < rightLine
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
		return ids.Candidates(at.Value)
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
// The alias references and the unfolded sites are printed whatever -v says,
// because both fail the gate. -v decides only how much of a clean run is
// shown, and how much of the staged hint rule: its count is always printed and
// its rows only when they are asked for, since nothing there fails a build and
// the backlog is hundreds of rows long.
func writeReport(out io.Writer, report Report, verbose bool) {
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
	if dir := filepath.Dir(path); dir != "" && dir != "." {
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
