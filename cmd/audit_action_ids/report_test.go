package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
)

// stubCatalog is a small ID set, so the classification rules are exercised on
// values written here rather than on whatever the catalog happens to hold.
func stubCatalog(extraIDs ...string) *actionids.IDs {
	ids := append([]string{"demo.get", "demo.list", "demo.create", "other.get"}, extraIDs...)
	return actionids.New(ids, map[string]string{
		"demo.fetch":  "demo.get",
		"issue.close": "issue.update",
	})
}

// TestClassify_FourOutcomes_AreKeptApart holds the split the whole report
// rests on: an ID the catalog has is silent, anything it has never heard of is
// a finding, and an alias is one or the other depending on where it is
// written. In a structured field it is a finding carrying the canonical ID as
// the fix; in prose it is reported without being counted, because a sentence
// may be about the alias.
func TestClassify_FourOutcomes_AreKeptApart(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindRelated, Value: "demo.get", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindRelated, Value: "demo.fetch", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 4, Kind: kindUsage, Value: "Dynamic execute also accepts demo.fetch.", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 5, Kind: kindRelated, Expr: "helper(x)"},
	}, stubCatalog(), true)

	if report.Summary.Findings != 2 {
		t.Fatalf("findings = %+v, want the structured alias and the dead ID", report.Findings)
	}
	if report.Findings[0].ID != "demo.fetch" || report.Findings[0].Canonical != "demo.get" {
		t.Errorf("first finding = %+v, want demo.fetch naming demo.get as the fix", report.Findings[0])
	}
	if report.Findings[1].ID != "demo.gone" || report.Findings[1].Canonical != "" {
		t.Errorf("second finding = %+v, want the dead ID with no canonical target", report.Findings[1])
	}
	if report.Summary.AliasHits != 1 || report.AliasRefs[0].Kind != kindUsage {
		t.Errorf("alias references = %+v, want only the alias named in prose", report.AliasRefs)
	}
	if report.Summary.Unresolved != 1 || report.Unresolved[0].Expression != "helper(x)" {
		t.Errorf("unresolved = %+v, want the expression named", report.Unresolved)
	}
	if report.Summary.Judged != 4 {
		t.Errorf("judged = %d, want the three folded IDs and the prose token", report.Summary.Judged)
	}
	if report.Summary.Packages != 1 {
		t.Errorf("packages with findings = %d, want 1", report.Summary.Packages)
	}
	if report.Summary.ByKind[kindHint] != 1 || report.Summary.ByKind[kindRelated] != 1 {
		t.Errorf("findings by kind = %v, want the hint and the related entry counted", report.Summary.ByKind)
	}
}

// TestClassify_EveryRecord_CarriesItsWholePosition holds a finding, an alias
// finding and an unresolved site as whole records rather than by the one
// field each earlier test reads. The package, file, line and kind are four
// assignments of the same shape written side by side, and a swap between two
// of them is invisible to a test that asserts only the ID or the expression.
func TestClassify_EveryRecord_CarriesItsWholePosition(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "q/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
		{Package: "p", File: "q/a.go", Line: 2, Kind: kindRelated, Value: "demo.fetch", Resolved: true},
		{Package: "p", File: "q/a.go", Line: 5, Kind: kindRelated, Expr: "helper(x)"},
	}, stubCatalog(), true)

	wantFindings := []Finding{
		{Package: "p", File: "q/a.go", Line: 2, Kind: kindRelated, ID: "demo.fetch", Canonical: "demo.get"},
		{Package: "p", File: "q/a.go", Line: 3, Kind: kindHint, ID: "demo.gone", Closest: "demo.get"},
	}
	if !slices.Equal(report.Findings, wantFindings) {
		t.Errorf("findings = %+v, want %+v", report.Findings, wantFindings)
	}
	wantUnresolved := []Unresolved{{Package: "p", File: "q/a.go", Line: 5, Kind: kindRelated, Expression: "helper(x)"}}
	if !slices.Equal(report.Unresolved, wantUnresolved) {
		t.Errorf("unresolved = %+v, want %+v", report.Unresolved, wantUnresolved)
	}
}

// TestClassify_Summary_EveryCounterIsItsOwn drives a run in which no two
// counts of the summary agree, and holds the whole summary and both printed
// summary lines against it. The counts are filled from lengths written side
// by side at the end of one run and printed in one line each, so a report that
// swapped two of them would read as correct to every test that checks one
// count at a time, or that checks it on a run where the two happen to agree.
func TestClassify_Summary_EveryCounterIsItsOwn(t *testing.T) {
	sites := []site{
		{Package: "p1", File: "p1/a.go", Line: 1, Kind: kindRelated, Value: "demo.gone", Resolved: true},
		{Package: "p1", File: "p1/a.go", Line: 2, Kind: kindHint, Value: "demo.gone2", Resolved: true},
		{Package: "p2", File: "p2/b.go", Line: 3, Kind: kindRelated, Value: "demo.fetch", Resolved: true},
		{Package: "p2", File: "p2/b.go", Line: 4, Kind: kindHint, Value: "nope.nothing", Resolved: true},
		{Package: "p3", File: "p3/c.go", Line: 5, Kind: kindRelated, Value: "demo.missing", Resolved: true},
		{Package: "p3", File: "p3/c.go", Line: 6, Kind: kindHint, Value: "demo.absent", Resolved: true},
		{Package: "p1", File: "p1/a.go", Line: 7, Kind: kindUsage, Value: "Execute also accepts demo.fetch.", Resolved: true},
		{Package: "p1", File: "p1/a.go", Line: 8, Kind: kindDescription, Value: "See demo.fetch.", Resolved: true},
		{Package: "p2", File: "p2/b.go", Line: 9, Kind: kindUsage, Value: "Or demo.fetch.", Resolved: true},
		{Package: "p3", File: "p3/c.go", Line: 10, Kind: kindDescription, Value: "And demo.fetch.", Resolved: true},
		{Package: "p1", File: "p1/a.go", Line: 16, Kind: kindUsage, Value: "The remote ends in project.git; execute accepts issue.close.", Resolved: true},
	}
	for line := 11; line <= 15; line++ {
		sites = append(sites, site{Package: "p1", File: "p1/a.go", Line: line, Kind: kindRelated, Expr: fmt.Sprintf("h%d(x)", line)})
	}
	report := classify(sites, stubCatalog("project.get", "issue.update", "extra.one", "extra.two"), true)

	want := Summary{
		DeclarationsJudged: true,
		CatalogIDs:         8,
		Aliases:            2,
		Judged:             11,
		Findings:           6,
		Packages:           3,
		AliasHits:          4,
		Unresolved:         5,
		Stale:              1,
		ByKind:             map[string]int{kindRelated: 3, kindHint: 3},
		JudgedByKind:       map[string]int{kindRelated: 3, kindHint: 3, kindUsage: 3, kindDescription: 2},
	}
	if !reflect.DeepEqual(report.Summary, want) {
		t.Errorf("summary = %+v\nwant      %+v", report.Summary, want)
	}
	if report.Clean() {
		t.Error("a run with findings in every bucket reported itself clean")
	}

	var out bytes.Buffer
	writeReport(&out, report, true)
	for _, line := range []string{
		"audit_action_ids: 6 published ID(s) to fix in 3 package(s); 4 alias(es) named in prose; 5 site(s) not folded; 1 stale declaration(s)\n",
		"  judged 11 published ID(s) against 8 catalog ID(s) and 2 alias(es)\n",
		"  findings by kind: hint 3, related 3\n",
		"  judged by kind: hint 3, related 3, usage 3, description 2\n",
	} {
		t.Run(strings.TrimSpace(line), func(t *testing.T) {
			if !strings.Contains(out.String(), line) {
				t.Errorf("report = %q, want the line %q", out.String(), line)
			}
		})
	}
}

// TestClassify_ProseTokens_NeedACatalogDomain holds the test that keeps the
// prose rule usable. Every dotted token in a Usage line looks like an action
// ID; only the ones whose left half names a catalog domain are treated as one,
// which is what turns away github.com and every params.note_id an example
// binding writes.
func TestClassify_ProseTokens_NeedACatalogDomain(t *testing.T) {
	usage := "Clone from github.com, read params.note_id, then call demo.get and demo.gone."
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindUsage, Value: usage, Resolved: true},
	}, stubCatalog(), true)

	if report.Summary.Judged != 2 {
		t.Errorf("judged = %d, want only the two demo tokens", report.Summary.Judged)
	}
	if len(report.Findings) != 1 || report.Findings[0].ID != "demo.gone" {
		t.Errorf("findings = %+v, want only demo.gone", report.Findings)
	}
}

// TestClassify_ProseTokenRepeated_IsJudgedOnce holds that a line naming one
// token twice produces one row rather than two identical ones.
func TestClassify_ProseTokenRepeated_IsJudgedOnce(t *testing.T) {
	report := classify([]site{
		{
			Package: "p", File: "p/a.go", Line: 1, Kind: kindDescription,
			Value: "See demo.gone. Then see demo.gone again.", Resolved: true,
		},
	}, stubCatalog(), true)

	if len(report.Findings) != 1 {
		t.Errorf("findings = %+v, want one row for the repeated token", report.Findings)
	}
}

// TestClassify_DeclaredProseToken_IsExcusedAndNotJudged holds the exemption
// table and its one rule: a declared token is excused in prose only. The same
// spelling written as a cross-link is still a finding, because a
// RelatedActions entry is an ID and nothing else.
func TestClassify_DeclaredProseToken_IsExcusedAndNotJudged(t *testing.T) {
	const declared = "project.git"
	if !exemptProse(declared) {
		t.Fatalf("%s is expected to be the declared prose exemption", declared)
	}
	ids := stubCatalog("project.get")

	report := classify([]site{
		{
			Package: "p", File: "p/a.go", Line: 1, Kind: kindUsage,
			Value: "The remote ends in project.git.", Resolved: true,
		},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindRelated, Value: declared, Resolved: true},
	}, ids, true)

	if len(report.Findings) != 1 || report.Findings[0].Kind != kindRelated {
		t.Errorf("findings = %+v, want the cross-link only", report.Findings)
	}
	if report.Summary.Judged != 1 {
		t.Errorf("judged = %d, want the excused prose token left uncounted", report.Summary.Judged)
	}
	if slices.ContainsFunc(report.StaleExemptions, func(entry string) bool {
		return strings.HasPrefix(entry, declared+" ")
	}) {
		t.Errorf("stale declarations = %v, want the used entry left out", report.StaleExemptions)
	}
}

// TestClassify_AliasNamedInProse_IsExcusedOnlyThere holds the one shape the
// canonical-ID demand would be wrong for, and the structural limit on it. A
// Usage line whose subject is the alias may name it; the same spelling written
// as a cross-link is still refused, because that field publishes IDs a model
// calls rather than sentences it reads.
func TestClassify_AliasNamedInProse_IsExcusedOnlyThere(t *testing.T) {
	const alias = "issue.close"
	if !exemptAliasMention(alias) {
		t.Fatalf("%s is expected to be a declared alias mention", alias)
	}

	report := classify([]site{
		{
			Package: "p", File: "p/a.go", Line: 1, Kind: kindUsage,
			Value: "Dynamic execute also accepts the issue.close alias.", Resolved: true,
		},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindRelated, Value: alias, Resolved: true},
	}, stubCatalog(), true)

	if len(report.AliasRefs) != 0 {
		t.Errorf("alias references = %+v, want the declared prose mention excused", report.AliasRefs)
	}
	if len(report.Findings) != 1 || report.Findings[0].Kind != kindRelated {
		t.Fatalf("findings = %+v, want the cross-link only", report.Findings)
	}
	if report.Findings[0].Canonical != "issue.update" {
		t.Errorf("finding = %+v, want issue.update named as the fix", report.Findings[0])
	}
	if report.Clean() {
		t.Error("a run holding an alias cross-link reported itself clean")
	}
}

// TestClassify_UnusedDeclaration_IsReportedStale holds every declaration table
// here to the same rule: one that has stopped describing the tree is itself a
// finding, or a reader goes on trusting it. Both tables are named in the same
// list, each row saying which map to open.
func TestClassify_UnusedDeclaration_IsReportedStale(t *testing.T) {
	report := classify(nil, stubCatalog(), true)

	for _, want := range []string{"project.git", "proseExemptions", "issue.close", "declaredAliasMentions"} {
		t.Run(want, func(t *testing.T) {
			if !slices.ContainsFunc(report.StaleExemptions, func(entry string) bool {
				return strings.Contains(entry, want)
			}) {
				t.Errorf("stale declarations = %v, want %q named", report.StaleExemptions, want)
			}
		})
	}
	if report.Summary.Stale != len(report.StaleExemptions) {
		t.Errorf("stale count = %d, want %d", report.Summary.Stale, len(report.StaleExemptions))
	}
	if report.Clean() {
		t.Error("a run holding a stale declaration reported itself clean")
	}
}

// TestClassify_NarrowedRun_LeavesTheDeclarationsUnjudged holds the one thing a
// run over part of the tree may not answer. Every declaration excuses nothing
// there, so reporting them stale would be a statement about the patterns; the
// report says they were not judged instead, and the run stays clean.
func TestClassify_NarrowedRun_LeavesTheDeclarationsUnjudged(t *testing.T) {
	report := classify(nil, stubCatalog(), false)

	if len(report.StaleExemptions) != 0 || report.Summary.Stale != 0 {
		t.Errorf("stale declarations = %v, want none from a narrowed run", report.StaleExemptions)
	}
	if !report.Clean() {
		t.Error("a narrowed run over nothing reported itself unclean")
	}

	var out bytes.Buffer
	writeReport(&out, report, false)
	if !strings.Contains(out.String(), "declaration tables were not judged") {
		t.Errorf("summary = %q, want the narrowed run to say what it did not judge", out.String())
	}
}

// TestClean_OneBucketAtATime_FailsTheRun holds the gate's four terms one at a
// time. Clean is four equalities joined by three ands, and a report with two
// buckets filled answers the same under an and as under an or, so the tests
// that assert !Clean over a realistic run could not tell the four apart: what
// separates them is a report in which exactly one is non-zero.
func TestClean_OneBucketAtATime_FailsTheRun(t *testing.T) {
	if clean := (&Report{}).Clean(); !clean {
		t.Error("a report with nothing in any bucket reported itself unclean")
	}
	for name, summary := range map[string]Summary{
		"a published ID that resolves to nothing": {Findings: 1},
		"an alias named in prose":                 {AliasHits: 1},
		"a site that could not be folded":         {Unresolved: 1},
		"a declaration that excuses nothing":      {Stale: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if (&Report{Summary: summary}).Clean() {
				t.Errorf("a run holding %s reported itself clean", name)
			}
		})
	}
}

// TestSortFindings_OneFieldAtATime_DecidesTheOrder holds each of the three
// position fields deciding the order on its own, with the ID order pointing
// the other way so a comparator that fell through to the ID is visible. The
// existing order test varies the package and the file together, where a
// comparator reading either one alone sorts the same list identically.
//
// Each case is sorted twice, the second time from the order the first
// produced, because a comparator is asked both which of two rows comes first
// and whether a row already in place belongs after the one before it, and a
// list that is already in order is the only input that asks the second.
func TestSortFindings_OneFieldAtATime_DecidesTheOrder(t *testing.T) {
	for name, findings := range map[string][]Finding{
		"the package decides": {
			{Package: "z", File: "a.go", Line: 1, ID: "demo.aaa"},
			{Package: "a", File: "a.go", Line: 1, ID: "demo.zzz"},
		},
		"the file decides": {
			{Package: "p", File: "z.go", Line: 1, ID: "demo.aaa"},
			{Package: "p", File: "a.go", Line: 1, ID: "demo.zzz"},
		},
		"the line decides": {
			{Package: "p", File: "a.go", Line: 9, ID: "demo.aaa"},
			{Package: "p", File: "a.go", Line: 1, ID: "demo.zzz"},
		},
		"one position, the ID decides": {
			{Package: "p", File: "a.go", Line: 1, ID: "demo.zzz"},
			{Package: "p", File: "a.go", Line: 1, ID: "demo.aaa"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			sortFindings(findings)
			first := findings[0]
			sortFindings(findings)
			if findings[0] != first {
				t.Errorf("order = %+v, want a sorted list left in its order", findings)
			}
			if name == "one position, the ID decides" {
				if findings[0].ID != "demo.aaa" {
					t.Errorf("order = %+v, want the IDs in order where the position is one", findings)
				}
				return
			}
			if findings[0].ID != "demo.zzz" {
				t.Errorf("order = %+v, want the earlier position first although its ID sorts last", findings)
			}
		})
	}
}

// TestClassify_UnresolvedSitesOnOneLine_KeepTheOrderTheyWereWrittenIn holds
// the last comparison of lessPosition, which orders two rows that agree on
// everything above the line. Findings never reach it, since they are compared
// only once their positions differ; the unresolved list does, and a comparator
// that called two equal lines "less" would let the sort swap them, so the work
// list would differ between runs over one tree.
func TestClassify_UnresolvedSitesOnOneLine_KeepTheOrderTheyWereWrittenIn(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 7, Kind: kindRelated, Expr: "first(x)"},
		{Package: "p", File: "p/a.go", Line: 7, Kind: kindRelated, Expr: "second(x)"},
	}, stubCatalog(), true)

	want := []Unresolved{
		{Package: "p", File: "p/a.go", Line: 7, Kind: kindRelated, Expression: "first(x)"},
		{Package: "p", File: "p/a.go", Line: 7, Kind: kindRelated, Expression: "second(x)"},
	}
	if !slices.Equal(report.Unresolved, want) {
		t.Errorf("unresolved = %+v, want %+v", report.Unresolved, want)
	}
}

// TestWriteSummary_EmptyBreakdown_PrintsNoLine holds the two breakdown lines
// against the run they are for. A clean run has no findings by kind, and a
// judged-nothing run has no judged-by-kind, so a guard that admitted an empty
// map would print "findings by kind: " with nothing after it.
func TestWriteSummary_EmptyBreakdown_PrintsNoLine(t *testing.T) {
	var out bytes.Buffer
	writeSummary(&out, Summary{DeclarationsJudged: true, ByKind: map[string]int{}, JudgedByKind: map[string]int{}}, true)

	for _, unwanted := range []string{"findings by kind", "judged by kind"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(out.String(), unwanted) {
				t.Errorf("summary = %q, want no %q line over an empty breakdown", out.String(), unwanted)
			}
		})
	}
}

// TestClassify_Findings_AreOrderedByPosition holds that two runs over one tree
// produce the same work list, which is what lets a later layer diff it.
func TestClassify_Findings_AreOrderedByPosition(t *testing.T) {
	report := classify([]site{
		{Package: "z", File: "z/a.go", Line: 1, Kind: kindRelated, Value: "demo.gone", Resolved: true},
		{Package: "a", File: "a/b.go", Line: 9, Kind: kindRelated, Value: "demo.gone", Resolved: true},
		{Package: "a", File: "a/b.go", Line: 2, Kind: kindRelated, Value: "demo.zzz", Resolved: true},
		{Package: "a", File: "a/b.go", Line: 2, Kind: kindRelated, Value: "demo.aaa", Resolved: true},
	}, stubCatalog(), true)

	var order []string
	for _, finding := range report.Findings {
		order = append(order, finding.Package+":"+finding.File+":"+finding.ID)
	}
	want := []string{"a:a/b.go:demo.aaa", "a:a/b.go:demo.zzz", "a:a/b.go:demo.gone", "z:z/a.go:demo.gone"}
	if !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// TestClassify_EmptyValue_IsNotJudged holds that a blank cross-link entry is
// not reported as an action named "".
func TestClassify_EmptyValue_IsNotJudged(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindRelated, Value: "  ", Resolved: true},
	}, stubCatalog(), true)

	if report.Summary.Judged != 0 || len(report.Findings) != 0 {
		t.Errorf("judged %d, findings %+v, want a blank entry passed over", report.Summary.Judged, report.Findings)
	}
}

// TestWriteReport_EverythingTheGateRefuses_IsPrintedWithoutVerbose holds that
// the quiet report is the whole failure. Three of the four refusals used to be
// hidden behind -v, which would have made a red gate say nothing about why.
func TestWriteReport_EverythingTheGateRefuses_IsPrintedWithoutVerbose(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindRelated, Value: "demo.fetch", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 5, Kind: kindUsage, Value: "Execute also accepts demo.fetch.", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 4, Kind: kindRelated, Expr: "helper(x)"},
	}, stubCatalog(), true)

	var quiet bytes.Buffer
	writeReport(&quiet, report, false)
	for _, want := range []string{"demo.gone", "demo.fetch", "helper(x)", "declarations that excuse nothing"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(quiet.String(), want) {
				t.Errorf("the quiet report left out %q", want)
			}
		})
	}
	if strings.Contains(quiet.String(), "judged by kind") {
		t.Error("the quiet report printed the breakdown -v is for")
	}
	// The prose alias is refused too, so the quiet report has to name it: it
	// is the one refusal whose row only ever appeared under -v, which is what
	// would have let a red gate print nothing about it.
	if !strings.Contains(quiet.String(), "alias of") {
		t.Error("the quiet report left out the prose alias bucket, which fails the gate")
	}

	var loud bytes.Buffer
	writeReport(&loud, report, true)
	if !strings.Contains(loud.String(), "judged by kind") {
		t.Error("the verbose report left out the breakdown by kind")
	}
}

// TestWriteReport_Rows_ReadFileLineKindAndIDInThatOrder holds the whole quiet
// report of one small run, row for row. Each row is one Fprintf over four
// fields of the same record plus a verb and a trailer, and every earlier test
// looks for an ID somewhere in the text, which a row that printed the kind
// where the file belongs would still satisfy.
func TestWriteReport_Rows_ReadFileLineKindAndIDInThatOrder(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "q/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
		{Package: "p", File: "q/a.go", Line: 2, Kind: kindRelated, Value: "demo.fetch", Resolved: true},
		{Package: "p", File: "q/a.go", Line: 5, Kind: kindUsage, Value: "Execute also accepts demo.fetch.", Resolved: true},
		{Package: "p", File: "q/a.go", Line: 4, Kind: kindRelated, Expr: "helper(x)"},
	}, stubCatalog(), true)

	var out bytes.Buffer
	writeReport(&out, report, false)
	want := strings.Join([]string{
		"=== p ===",
		`  q/a.go:2 related "demo.fetch" is an alias, not the catalog ID demo.get`,
		`  q/a.go:3 hint "demo.gone" resolves to no action; closest: demo.get`,
		"=== registered aliases, not catalog IDs ===",
		"=== p ===",
		`  q/a.go:5 usage "demo.fetch" alias of demo.get`,
		"=== not folded ===",
		"  q/a.go:4 related helper(x)",
		"=== declarations that excuse nothing ===",
		"  issue.close is no longer a registered alias named in prose (declaredAliasMentions). Remove the entry.",
		"  issue.reopen is no longer a registered alias named in prose (declaredAliasMentions). Remove the entry.",
		"  project.git is no longer spelled in any Usage line or description (proseExemptions). Remove the entry.",
		"audit_action_ids: 2 published ID(s) to fix in 1 package(s); 1 alias(es) named in prose; 1 site(s) not folded; 3 stale declaration(s)",
		"  judged 3 published ID(s) against 4 catalog ID(s) and 2 alias(es)",
		"  findings by kind: hint 1, related 1",
		"  gitlab_ci_ymls is no longer spelled in any hint (hintToolExemptions). Remove the entry.",
		"  error hints: 0 finding(s) in 0 package(s) over 0 hint(s) read; 0 not folded (reported, not gated)",
		"",
	}, "\n")
	if out.String() != want {
		t.Errorf("report:\n%s\nwant:\n%s", out.String(), want)
	}
}

// TestWriteReport_CleanRunVerbose_NamesTheAliasHeadingAnyway holds the one
// thing -v still decides: on a clean run it prints the alias heading, so a
// reader can see the bucket was looked at and was empty.
func TestWriteReport_CleanRunVerbose_NamesTheAliasHeadingAnyway(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindRelated, Value: "demo.get", Resolved: true},
	}, stubCatalog(), true)

	var quiet, loud bytes.Buffer
	writeReport(&quiet, report, false)
	writeReport(&loud, report, true)

	const heading = "registered aliases, not catalog IDs"
	if strings.Contains(quiet.String(), heading) {
		t.Error("the quiet report printed an empty alias heading")
	}
	if !strings.Contains(loud.String(), heading) {
		t.Error("the verbose report left out the alias heading")
	}
}

// TestWriteReport_CleanRun_SaysWhatItWasCleanOver holds the summary line's
// job: a report with no findings has to name what it judged, or a clean run
// cannot be told from a run that looked at nothing.
func TestWriteReport_CleanRun_SaysWhatItWasCleanOver(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindRelated, Value: "demo.get", Resolved: true},
	}, stubCatalog(), true)

	var out bytes.Buffer
	writeReport(&out, report, false)
	if !strings.Contains(out.String(), "judged 1 published ID(s) against") {
		t.Errorf("summary = %q, want the judged count", out.String())
	}
}

// TestWriteJSON_Roundtrip_CarriesTheWholeReport holds that the work list a
// later layer reads is the report, and that it is written even when it is
// empty so a stale file cannot pass for today's answer.
//
// The suite's section is in it under its own keys, and the two prose sections
// share the neutral ones, which is what schema version 5 is.
func TestWriteJSON_Roundtrip_CarriesTheWholeReport(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
		{Package: "s", File: "s/a_test.go", Line: 4, Kind: kindAssertion, Value: "use gitlab_demo_list", Resolved: true},
	}, stubCatalog(), true)
	report.judgeHelpers(suiteRead{calls: map[string]int{"assertMentions": 1}}, false)

	path := filepath.Join(t.TempDir(), "nested", "action-ids.json")
	if err := writeJSON(path, report); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var decoded Report
	if unmarshalErr := json.Unmarshal(data, &decoded); unmarshalErr != nil {
		t.Fatalf("decode: %v", unmarshalErr)
	}
	if decoded.SchemaVersion != 5 || schemaVersion != 5 {
		t.Errorf("schema version = %d (constant %d), want 5", decoded.SchemaVersion, schemaVersion)
	}
	if len(decoded.Findings) != 1 || decoded.Findings[0].ID != "demo.gone" {
		t.Errorf("findings = %+v, want the one finding", decoded.Findings)
	}
	wantRow := HintFinding{Package: "s", File: "s/a_test.go", Line: 4, Kind: kindAssertion, Rule: ruleToolName, Name: "gitlab_demo_list"}
	if !decoded.SuiteJudged || !slices.Equal(decoded.Assertions.Rows, []HintFinding{wantRow}) {
		t.Errorf("suite judged %t, assertion rows %+v, want the one row", decoded.SuiteJudged, decoded.Assertions.Rows)
	}
	if decoded.CallsByHelper["assertMentions"] != 1 {
		t.Errorf("calls by helper = %v, want the one call carried", decoded.CallsByHelper)
	}
	for _, key := range []string{`"e2e_assertions": {`, `"suite_judged": true`, `"assertion_calls_by_helper"`, `"rows": [`, `"read": 1`, `"read_by_kind"`, `"not_folded": 0`} {
		t.Run(key, func(t *testing.T) {
			if !strings.Contains(string(data), key) {
				t.Errorf("work list = %s, want %s", data, key)
			}
		})
	}
	if strings.Contains(string(data), `"hints_read"`) || strings.Contains(string(data), `"hint_findings"`) {
		t.Errorf("work list = %s, want the section keys to say nothing about hints", data)
	}
}

// TestClassify_AssertionSite_IsJudgedInItsOwnSection holds where a quotation
// lands and what it is held to: the section of its own, by the three rules a
// hint is judged by, with the canonical ID silent and nothing reaching the
// hint section or the published-ID gate.
func TestClassify_AssertionSite_IsJudgedInItsOwnSection(t *testing.T) {
	assertion := func(line int, value string) site {
		return site{Package: "s", File: "s/a_test.go", Line: line, Kind: kindAssertion, Value: value, Resolved: true}
	}
	report := classify([]site{
		assertion(1, "list them with gitlab_demo_list"),
		assertion(2, "the demo.fetch action"),
		assertion(3, "demo.gone"),
		assertion(4, "demo.get"),
		{Package: "s", File: "s/a_test.go", Line: 5, Kind: kindAssertion, Expr: "e.Name(x)"},
	}, stubCatalog(), true)

	want := []HintFinding{
		{Package: "s", File: "s/a_test.go", Line: 1, Kind: kindAssertion, Rule: ruleToolName, Name: "gitlab_demo_list"},
		{Package: "s", File: "s/a_test.go", Line: 2, Kind: kindAssertion, Rule: ruleAlias, Name: "demo.fetch", Canonical: "demo.get"},
		{Package: "s", File: "s/a_test.go", Line: 3, Kind: kindAssertion, Rule: ruleUnknownID, Name: "demo.gone", Closest: "demo.get"},
	}
	if !slices.Equal(report.Assertions.Rows, want) {
		t.Errorf("assertion rows = %+v\nwant %+v", report.Assertions.Rows, want)
	}
	if report.Assertions.Read != 4 || report.Assertions.Findings != 3 || report.Assertions.Unfolded != 1 {
		t.Errorf("assertions read %d, findings %d, not folded %d; want 4, 3, 1",
			report.Assertions.Read, report.Assertions.Findings, report.Assertions.Unfolded)
	}
	if report.Hints.Read != 0 || len(report.Hints.Rows) != 0 || report.Summary.Judged != 0 || report.Summary.Unresolved != 0 {
		t.Errorf("hints %+v, summary %+v, want nothing from a quotation outside its own section", report.Hints, report.Summary)
	}
}

// TestClassify_AssertionQuotingADeclaredAliasMention_IsExcused holds the one
// place the two prose sections differ. A Usage line may name one of the two
// declared aliases on purpose, and a test quoting that line quotes it
// faithfully; a hint naming the same alias is the server writing a spelling
// it should not, and stays a finding.
func TestClassify_AssertionQuotingADeclaredAliasMention_IsExcused(t *testing.T) {
	const quoted = "dynamic execute also accepts issue.close"
	report := classify([]site{
		{Package: "s", File: "s/a_test.go", Line: 1, Kind: kindAssertion, Value: quoted, Resolved: true},
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindErrorHint, Value: quoted, Resolved: true},
		{Package: "s", File: "s/a_test.go", Line: 2, Kind: kindAssertion, Value: "the demo.fetch action", Resolved: true},
	}, stubCatalog("issue.update"), true)

	if want := []HintFinding{{Package: "s", File: "s/a_test.go", Line: 2, Kind: kindAssertion, Rule: ruleAlias, Name: "demo.fetch", Canonical: "demo.get"}}; !slices.Equal(report.Assertions.Rows, want) {
		t.Errorf("assertion rows = %+v, want only the undeclared alias", report.Assertions.Rows)
	}
	if len(report.Hints.Rows) != 1 || report.Hints.Rows[0].Name != "issue.close" {
		t.Errorf("hint rows = %+v, want the declared alias still refused in a hint", report.Hints.Rows)
	}
	if !slices.ContainsFunc(report.StaleExemptions, func(entry string) bool { return strings.HasPrefix(entry, "issue.close ") }) {
		t.Errorf("stale declarations = %v, want the entry left stale: a quotation keeps no declaration alive", report.StaleExemptions)
	}
}

// TestClassify_AssertionSpellingADeclaredToken_DoesNotKeepTheHintDeclarationAlive
// holds the tool-name declaration to the served source. A quotation may spell
// the declared token and is excused for it, but the declaration is about what
// the server writes, so a run whose only spelling of it is in the suite
// reports it stale.
func TestClassify_AssertionSpellingADeclaredToken_DoesNotKeepTheHintDeclarationAlive(t *testing.T) {
	report := classify([]site{
		{Package: "s", File: "s/a_test.go", Line: 1, Kind: kindAssertion, Value: "template_type gitlab_ci_ymls", Resolved: true},
	}, stubCatalog(), true)

	if len(report.Assertions.Rows) != 0 {
		t.Errorf("assertion rows = %+v, want the declared token excused", report.Assertions.Rows)
	}
	if !slices.ContainsFunc(report.Hints.StaleDeclarations, func(entry string) bool { return strings.Contains(entry, "gitlab_ci_ymls") }) {
		t.Errorf("hint stale declarations = %v, want the entry the suite alone spells reported", report.Hints.StaleDeclarations)
	}
	if len(report.Assertions.StaleDeclarations) != 0 {
		t.Errorf("assertion stale declarations = %v, want the suite's section never to judge the table", report.Assertions.StaleDeclarations)
	}
}

// TestJudgeHelpers_EachRun_MarksTheSuiteJudgedAndNamesWhatItCan holds what
// the suite walk hands the report: the mark that the suite was loaded, the
// counts, and the stale entries the run is allowed to name.
func TestJudgeHelpers_EachRun_MarksTheSuiteJudgedAndNamesWhatItCan(t *testing.T) {
	read := suiteRead{calls: map[string]int{"assertMentions": 2, "ExpectToolError": 1, "mentionsAny": 1}}

	t.Run("the whole suite", func(t *testing.T) {
		report := classify(nil, stubCatalog(), true)
		report.judgeHelpers(read, true)
		if !report.SuiteJudged || !maps.Equal(report.CallsByHelper, read.calls) {
			t.Errorf("suite judged %t, calls %v, want true and %v", report.SuiteJudged, report.CallsByHelper, read.calls)
		}
		if want := []string{"containsAny is called nowhere in the suite (servedTextAssertions)"}; !slices.Equal(report.StaleHelpers, want) {
			t.Errorf("stale helpers = %q, want %q", report.StaleHelpers, want)
		}
	})
	t.Run("part of it", func(t *testing.T) {
		report := classify(nil, stubCatalog(), true)
		report.judgeHelpers(read, false)
		if !report.SuiteJudged || len(report.StaleHelpers) != 0 {
			t.Errorf("suite judged %t, stale helpers %q, want true and none", report.SuiteJudged, report.StaleHelpers)
		}
	})
}

// TestReport_Clean_AnAssertionFindingOrAStaleHelper_Fails holds the three
// terms the prose sections and the helper table add to the gate, one at a
// time, since a report with two of them filled answers the same under an and
// as under an or.
func TestReport_Clean_AnAssertionFindingOrAStaleHelper_Fails(t *testing.T) {
	cases := []struct {
		name   string
		report Report
		clean  bool
	}{
		{name: "a hint naming a tool", report: Report{Hints: HintReport{Findings: 1}}},
		{name: "a quotation naming a tool", report: Report{Assertions: HintReport{Findings: 1}}},
		{name: "a helper entry describing no call", report: Report{StaleHelpers: []string{"containsAny is called nowhere"}}},
		{name: "a quotation nothing folds, alone", report: Report{Assertions: HintReport{Unfolded: 1}}, clean: true},
		{name: "a suite that was judged, and clean", report: Report{SuiteJudged: true}, clean: true},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := one.report.Clean(); got != one.clean {
				t.Errorf("Clean() = %t, want %t", got, one.clean)
			}
		})
	}
}

// TestWriteReport_SuiteJudged_PrintsTheSectionAndItsStaleHelpers holds the
// suite's half of the quiet report, line for line, after the hint count: the
// assertion rows, the count and its breakdown, then the helper entries that
// describe no call. The rows and the entries fail the gate and so are printed
// without -v; the breakdown by kind and the calls by helper fail nothing and
// are what -v adds.
func TestWriteReport_SuiteJudged_PrintsTheSectionAndItsStaleHelpers(t *testing.T) {
	report := classify([]site{
		{Package: "s", File: "s/a_test.go", Line: 7, Kind: kindAssertion, Value: "use gitlab_demo_list", Resolved: true},
	}, stubCatalog(), false)
	report.judgeHelpers(suiteRead{
		calls:      map[string]int{"assertMentions": 1},
		mismatches: map[string]string{"mentionsAny": "substrings"},
	}, false)

	var quiet, loud bytes.Buffer
	writeReport(&quiet, report, false)
	writeReport(&loud, report, true)

	const hintCount = "  error hints: 0 finding(s) in 0 package(s) over 0 hint(s) read; 0 not folded (reported, not gated)\n"
	wantTail := hintCount + strings.Join([]string{
		assertionRowsHeading,
		"=== s ===",
		`  s/a_test.go:7 assertion "gitlab_demo_list" is a tool name; the dynamic surface registers no such tool`,
		"  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read; 0 not folded (reported, not gated)",
		"    assertion findings by rule: tool_name 1",
		"=== assertion helpers that describe no call ===",
		"  mentionsAny takes no parameter named substrings (servedTextAssertions). Fix the entry.",
		"",
	}, "\n")
	if !strings.HasSuffix(quiet.String(), wantTail) {
		t.Errorf("report:\n%s\nwant it to end with:\n%s", quiet.String(), wantTail)
	}
	for _, want := range []string{
		"    assertions read by kind: assertion 1\n",
		"    assertion calls by helper: assertMentions 1\n",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(loud.String(), want) {
				t.Errorf("the verbose report left out %q; got %q", want, loud.String())
			}
			if strings.Contains(quiet.String(), want) {
				t.Errorf("the quiet report printed %q, which -v is for", want)
			}
		})
	}
}

// TestWriteReport_SuiteNotJudged_PrintsNoAssertionSection holds the section
// to the runs that loaded a suite. Its count over nothing would read "0
// finding(s) over 0 assertion(s) read", which a reader takes for a clean suite
// when nobody looked at one.
func TestWriteReport_SuiteNotJudged_PrintsNoAssertionSection(t *testing.T) {
	report := classify(nil, stubCatalog(), false)
	report.StaleHelpers = []string{"a line only a judged suite prints"}
	report.CallsByHelper = map[string]int{"assertMentions": 1}

	var out bytes.Buffer
	writeReport(&out, report, true)
	for _, unwanted := range []string{"e2e assertions", "a line only a judged suite prints", "assertion calls by helper"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(out.String(), unwanted) {
				t.Errorf("report = %q, want nothing about a suite the run did not load", out.String())
			}
		})
	}
}

// TestWriteSuiteReport_NothingStaleOrCounted_PrintsTheCountAlone holds the
// two guards of the suite section against the empty lists they guard: no
// heading over no stale entry, and no breakdown of no calls.
func TestWriteSuiteReport_NothingStaleOrCounted_PrintsTheCountAlone(t *testing.T) {
	report := classify(nil, stubCatalog(), false)
	report.judgeHelpers(suiteRead{calls: map[string]int{}}, false)

	var out bytes.Buffer
	writeSuiteReport(&out, report, true)
	const want = "  e2e assertions: 0 finding(s) in 0 package(s) over 0 assertion(s) read; 0 not folded (reported, not gated)\n"
	if out.String() != want {
		t.Errorf("suite report = %q, want %q", out.String(), want)
	}
}

// TestWriteJSON_BareFileName_WritesInTheWorkingDirectory holds the one path
// shape that names no directory to create. `-json action-ids.json` is a
// spelling a caller may reasonably use, and a run that tried to create the
// directory "." before writing would fail on a read-only checkout for no
// reason.
func TestWriteJSON_BareFileName_WritesInTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := writeJSON("action-ids.json", Report{SchemaVersion: schemaVersion}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat("action-ids.json"); err != nil {
		t.Errorf("work list: %v", err)
	}
}

// TestWriteJSON_UnwritablePath_IsAnError holds that a work list that could not
// be written is reported rather than swallowed.
func TestWriteJSON_UnwritablePath_IsAnError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := writeJSON(filepath.Join(blocker, "action-ids.json"), Report{}); err == nil {
		t.Fatal("writeJSON under a file returned no error")
	}
	if err := writeJSON(filepath.Join(t.TempDir(), "action-ids.json"), func() {}); err == nil {
		t.Fatal("writeJSON of an unencodable report returned no error")
	}
	// A directory already sitting where the work list goes: its own parent
	// exists, so the failure is the write rather than the directory, which is
	// the other half of what this function can be refused by.
	if err := writeJSON(t.TempDir(), Report{}); err == nil {
		t.Fatal("writeJSON onto a directory returned no error")
	}
}

// TestByCount_Breakdown_LeadsWithTheBiggest holds that a report opens with
// where the work is, and that equal counts are ordered by name so the line
// does not move between runs.
func TestByCount_Breakdown_LeadsWithTheBiggest(t *testing.T) {
	got := byCount(map[string]int{"related": 5, "hint": 5, "usage": 9})
	if got != "usage 9, hint 5, related 5" {
		t.Errorf("byCount = %q", got)
	}
}
