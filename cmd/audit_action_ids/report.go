package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// schemaVersion is the shape of the work list this writes. A later layer reads
// the file to know which cross-links to fix, so a change to the shape has to
// be visible to it.
//
// Version 2 moved an alias written into a related entry or a hint argument out
// of alias_references and into findings, carrying its canonical target in the
// same `canonical` field. The field set is unchanged and the counts are not:
// a reader that compares two runs across this line is comparing two rules.
const schemaVersion = 2

// dottedToken matches an action-ID-shaped token inside prose. The shape alone
// is far too generous, which is why every match is also held to a domain the
// catalog has.
var dottedToken = regexp.MustCompile(`\b[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\b`)

// Finding is one published action ID that resolves to nothing.
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
type Summary struct {
	CatalogIDs   int            `json:"catalog_ids"`
	Aliases      int            `json:"catalog_aliases"`
	Judged       int            `json:"ids_judged"`
	Findings     int            `json:"findings"`
	Packages     int            `json:"packages_with_findings"`
	AliasHits    int            `json:"alias_references"`
	Unresolved   int            `json:"unresolved_sites"`
	Stale        int            `json:"stale_exemptions"`
	ByKind       map[string]int `json:"findings_by_kind"`
	JudgedByKind map[string]int `json:"judged_by_kind"`
}

// Report is the whole answer, written to stdout and to the work list.
type Report struct {
	SchemaVersion int          `json:"schema_version"`
	Summary       Summary      `json:"summary"`
	Findings      []Finding    `json:"findings"`
	AliasRefs     []Finding    `json:"alias_references"`
	Unresolved    []Unresolved `json:"unresolved"`
	// StaleExemptions are the prose exemptions that excused nothing.
	StaleExemptions []string `json:"stale_exemptions,omitempty"`
	// usedExemptions is what the run actually excused, which is what the stale
	// list is computed against.
	usedExemptions map[string]struct{}
}

// classify holds every site against the oracle and builds the report.
//
// The three outcomes are deliberately kept apart. A canonical ID is silent,
// anything the catalog has never heard of is a finding, and an alias is judged
// by where it was written.
//
// That last rule is the one this command deferred while there was nothing to
// settle it with. Both halves of the old reasoning are true at once:
// gitlab_execute_action resolves an alias, and gitlab_find_action publishes
// canonical IDs and so lists it under no name. What decides between them is
// the site. A related entry and a hint argument are structured fields that the
// discovery tools hand a model as the ID to call next, and an alias there is a
// cross-link a model can follow once and can never look up, so it is a
// finding. A Usage line or a description is prose, where naming an alias can
// be the whole point of the sentence: issue.update's usage says that dynamic
// execute also accepts issue.close and issue.reopen, which is true, useful,
// and would be a defect under one rule for both. Those stay reported apart.
func classify(sites []site, ids *oracle) Report {
	report := Report{
		SchemaVersion: schemaVersion,
		Summary: Summary{
			CatalogIDs:   len(ids.ids),
			Aliases:      len(ids.aliases),
			ByKind:       map[string]int{},
			JudgedByKind: map[string]int{},
		},
		usedExemptions: map[string]struct{}{},
	}
	for _, at := range sites {
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
func (r *Report) judge(at site, candidate string, ids *oracle) {
	if isProseKind(at.Kind) && exemptProse(candidate) {
		r.usedExemptions[candidate] = struct{}{}
		return
	}
	r.Summary.Judged++
	r.Summary.JudgedByKind[at.Kind]++
	if ids.isID(candidate) {
		return
	}
	finding := Finding{Package: at.Package, File: at.File, Line: at.Line, Kind: at.Kind, ID: candidate}
	if canonical, isAlias := ids.alias(candidate); isAlias {
		finding.Canonical = canonical
		if isProseKind(at.Kind) {
			r.AliasRefs = append(r.AliasRefs, finding)
			return
		}
		r.Findings = append(r.Findings, finding)
		r.Summary.ByKind[at.Kind]++
		return
	}
	finding.Closest = closestID(candidate, ids.sorted)
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
	r.StaleExemptions = staleProseExemptions(r.usedExemptions)
	r.Summary.Stale = len(r.StaleExemptions)
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
// description is prose, and only a dotted token whose left half names a
// catalog domain is taken as an ID: without that test the rule reports
// github.com, gitlab.com, e.g and every params.note_id an example binding
// spells, which is a list a reader learns to skip.
func candidateIDs(at site, ids *oracle) []string {
	switch at.Kind {
	case kindRelated, kindHint:
		if strings.TrimSpace(at.Value) == "" {
			return nil
		}
		return []string{at.Value}
	default:
		return proseCandidates(at.Value, ids)
	}
}

// isProseKind reports whether a site's IDs were picked out of model-facing
// prose rather than written as an ID.
func isProseKind(kind string) bool {
	return kind == kindUsage || kind == kindDescription
}

// proseCandidates extracts the dotted tokens of a prose line that look like an
// action ID, deduplicated in the order they appear.
//
// A token qualifies when either half is one the catalog uses: a known domain,
// or a known action name. Requiring the domain alone was the first rule and it
// was blind to the commonest defect, since the usual wrong spelling keeps the
// action and invents the domain. "Resolve SHAs with commit.list" survived a
// clean run that way, because there is no gitlab_commit group and the token
// was dropped before it could be judged. Requiring neither half would admit
// every "example.com" and "toolutil.HintAction" in the tree, which is the
// noise the first rule was avoiding; requiring either half turns those away
// and keeps the invented domain.
func proseCandidates(prose string, ids *oracle) []string {
	var candidates []string
	seen := map[string]struct{}{}
	for _, token := range dottedToken.FindAllString(prose, -1) {
		domain, member, found := strings.Cut(token, ".")
		if !found || (!ids.hasDomain(domain) && !ids.hasMember(member)) {
			continue
		}
		if _, repeated := seen[token]; repeated {
			continue
		}
		seen[token] = struct{}{}
		candidates = append(candidates, token)
	}
	return candidates
}

// closestID names the nearest canonical ID, or nothing when the nearest is too
// far to be a lead. The bound is a third of the length: past that the nearest
// ID is an accident of the alphabet and naming it would send a reader off.
func closestID(id string, sorted []string) string {
	best, bestDistance := "", 0
	bound := max(len(id)/3, 2)
	for _, candidate := range sorted {
		distance := editDistance(normalizeID(id), candidate)
		if distance > bound {
			continue
		}
		if best == "" || distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// editDistance is the Levenshtein distance between two strings.
func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(left); i++ {
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 1
			if left[i-1] == right[j-1] {
				cost = 0
			}
			current[j] = min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}

// writeReport prints the human report: the findings under the package that has
// to act on them, then the two buckets that are not findings, then what the
// run saw.
func writeReport(out io.Writer, report Report, verbose bool) {
	writeGroups(out, report.Findings, findingVerb)
	if verbose {
		fmt.Fprintln(out, "=== aliases named in prose, not catalog IDs ===")
		writeGroups(out, report.AliasRefs, aliasVerb)
		writeUnresolved(out, report.Unresolved)
	}
	writeStale(out, report.StaleExemptions)
	writeSummary(out, report.Summary, verbose)
}

// writeStale prints the prose exemptions that excused nothing, which is a
// finding of its own: a declaration that has stopped describing the tree is
// one a reader would otherwise trust.
func writeStale(out io.Writer, stale []string) {
	if len(stale) == 0 {
		return
	}
	fmt.Fprintln(out, "=== exemptions that excuse nothing ===")
	for _, token := range stale {
		fmt.Fprintf(out, "  %s is no longer spelled in any Usage line or description. Remove the entry.\n", token)
	}
}

// writeGroups prints findings under one `=== package ===` heading each, each
// row read with the verb its own outcome deserves.
//
// The verb is per row rather than per list because the findings list now holds
// two outcomes: an ID nothing resolves, and an alias written where a catalog
// ID belongs. Printing the second under the first's verb would say a string
// resolves to nothing while naming what it resolves to.
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

// aliasVerb reads a row of the prose bucket, where naming an alias is allowed.
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
// the audit's own blind spot rather than a clean answer.
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
	fmt.Fprintf(out, "%s: %d published ID(s) to fix in %d package(s); %d alias(es) named in prose; %d site(s) not folded; %d stale exemption(s)\n",
		toolName, summary.Findings, summary.Packages, summary.AliasHits, summary.Unresolved, summary.Stale)
	fmt.Fprintf(out, "  judged %d published ID(s) against %d catalog ID(s) and %d alias(es)\n",
		summary.Judged, summary.CatalogIDs, summary.Aliases)
	if len(summary.ByKind) > 0 {
		fmt.Fprintf(out, "  findings by kind: %s\n", byCount(summary.ByKind))
	}
	if verbose && len(summary.JudgedByKind) > 0 {
		fmt.Fprintf(out, "  judged by kind: %s\n", byCount(summary.JudgedByKind))
	}
}

// byCount renders a breakdown, biggest first, so a report opens with where the
// work is.
func byCount(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
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
