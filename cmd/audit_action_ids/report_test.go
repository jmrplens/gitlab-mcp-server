package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// stubOracle is a small ID set, so the classification rules are exercised on
// values written here rather than on whatever the catalog happens to hold.
func stubOracle() *oracle {
	ids := &oracle{
		ids:     map[string]struct{}{},
		aliases: map[string]string{},
		domains: map[string]struct{}{},
	}
	for _, id := range []string{"demo.get", "demo.list", "demo.create", "other.get"} {
		ids.ids[id] = struct{}{}
		domain, _, _ := strings.Cut(id, ".")
		ids.domains[domain] = struct{}{}
	}
	ids.addAlias("demo.fetch", "demo.get")
	ids.finish()
	return ids
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
	}, stubOracle())

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
	}, stubOracle())

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
	}, stubOracle())

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
	ids := stubOracle()
	ids.ids["project.get"] = struct{}{}
	ids.domains["project"] = struct{}{}
	ids.finish()

	report := classify([]site{
		{
			Package: "p", File: "p/a.go", Line: 1, Kind: kindUsage,
			Value: "The remote ends in project.git.", Resolved: true,
		},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindRelated, Value: declared, Resolved: true},
	}, ids)

	if len(report.Findings) != 1 || report.Findings[0].Kind != kindRelated {
		t.Errorf("findings = %+v, want the cross-link only", report.Findings)
	}
	if report.Summary.Judged != 1 {
		t.Errorf("judged = %d, want the excused prose token left uncounted", report.Summary.Judged)
	}
	if report.Summary.Stale != 0 {
		t.Errorf("stale exemptions = %v, want none: the entry excused a token", report.StaleExemptions)
	}
}

// TestClassify_UnusedExemption_IsReportedStale holds every declaration table
// here to the same rule: one that has stopped describing the tree is itself a
// finding, or a reader goes on trusting it.
func TestClassify_UnusedExemption_IsReportedStale(t *testing.T) {
	report := classify(nil, stubOracle())

	if !slices.Contains(report.StaleExemptions, "project.git") {
		t.Errorf("stale exemptions = %v, want the unused entry named", report.StaleExemptions)
	}
	if report.Summary.Stale != len(report.StaleExemptions) {
		t.Errorf("stale count = %d, want %d", report.Summary.Stale, len(report.StaleExemptions))
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
	}, stubOracle())

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
	}, stubOracle())

	if report.Summary.Judged != 0 || len(report.Findings) != 0 {
		t.Errorf("judged %d, findings %+v, want a blank entry passed over", report.Summary.Judged, report.Findings)
	}
}

// TestClosestID_FarFromEverything_SuggestsNothing holds the bound on a
// suggestion. Past a third of the length the nearest ID is an accident of the
// alphabet, and naming it would send a reader after the wrong fix.
func TestClosestID_FarFromEverything_SuggestsNothing(t *testing.T) {
	sorted := []string{"demo.create", "demo.get", "demo.list"}
	if got := closestID("demo.gt", sorted); got != "demo.get" {
		t.Errorf("closestID for a near miss = %q, want demo.get", got)
	}
	if got := closestID("unrelated.something_entirely_else", sorted); got != "" {
		t.Errorf("closestID for a distant ID = %q, want nothing", got)
	}
}

// TestEditDistance_KnownPairs_AreTheLevenshteinDistance holds the measure the
// suggestion is ranked by.
func TestEditDistance_KnownPairs_AreTheLevenshteinDistance(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
	}
	for _, tc := range cases {
		t.Run(tc.left+"/"+tc.right, func(t *testing.T) {
			if got := editDistance(tc.left, tc.right); got != tc.want {
				t.Errorf("editDistance(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
			}
		})
	}
}

// TestWriteReport_Verbose_AddsTheBucketsThatAreNotFindings holds what the two
// report modes say. The quiet one is the work list; the verbose one adds the
// aliases named in prose and the sites that could not be folded, which are the
// audit's own blind spot rather than a clean answer.
//
// The two alias rows are both here on purpose, because each is printed with a
// verb of its own: the one written into a cross-link is a finding and is read
// as a spelling to correct, the one written into a sentence is not.
func TestWriteReport_Verbose_AddsTheBucketsThatAreNotFindings(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHint, Value: "demo.gone", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindRelated, Value: "demo.fetch", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 5, Kind: kindUsage, Value: "Execute also accepts demo.fetch.", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 4, Kind: kindRelated, Expr: "helper(x)"},
	}, stubOracle())

	var quiet bytes.Buffer
	writeReport(&quiet, report, false)
	if !strings.Contains(quiet.String(), "demo.gone") {
		t.Error("the quiet report left out the finding")
	}
	if strings.Contains(quiet.String(), "helper(x)") {
		t.Error("the quiet report printed the unresolved bucket")
	}
	if strings.Contains(quiet.String(), "alias of") {
		t.Error("the quiet report printed the prose alias bucket")
	}

	var loud bytes.Buffer
	writeReport(&loud, report, true)
	for _, want := range []string{
		"demo.gone",
		`"demo.fetch" is an alias, not the catalog ID demo.get`,
		"aliases named in prose",
		`"demo.fetch" alias of demo.get`,
		"helper(x)",
		"exemptions that excuse nothing",
		"judged by kind",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(loud.String(), want) {
				t.Errorf("the verbose report left out %q", want)
			}
		})
	}
}

// TestWriteReport_CleanRun_SaysWhatItWasCleanOver holds the summary line's
// job: a report with no findings has to name what it judged, or a clean run
// cannot be told from a run that looked at nothing.
func TestWriteReport_CleanRun_SaysWhatItWasCleanOver(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindRelated, Value: "demo.get", Resolved: true},
	}, stubOracle())

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
	}, stubOracle())

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
