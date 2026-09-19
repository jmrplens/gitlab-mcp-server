package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
func TestWriteJSON_Roundtrip_CarriesTheWholeReport(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
	}, stubCatalog(), true)

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
	if decoded.SchemaVersion != schemaVersion {
		t.Errorf("schema version = %d, want %d", decoded.SchemaVersion, schemaVersion)
	}
	if len(decoded.Findings) != 1 || decoded.Findings[0].ID != "demo.gone" {
		t.Errorf("findings = %+v, want the one finding", decoded.Findings)
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
