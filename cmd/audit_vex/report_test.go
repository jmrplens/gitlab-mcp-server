// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// report_test.go covers how the statements are held to a scan and to go.mod,
// and how the verdict is printed: a statement that holds, a stale one, a
// contradicted one, one about an image, and an advisory no statement
// accounts for.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// fixtureGoMod is the go.mod of the fixture module as [judge] reads it,
// requiring one module that no package of the fixture imports.
var fixtureGoMod = goModule{path: fixtureModule, requires: map[string]string{"example.com/testonly": "v0.3.0"}}

// scanOf is a scan result that loaded the standard library and the fixture
// module, and reports each advisory against the standard library at the
// level given.
func scanOf(levels map[string]level) scanResult {
	result := scanResult{
		db: "file:///db", dbModified: "2026-09-30T00:00:00Z",
		levels: map[findingKey]level{}, versions: map[findingKey]string{}, summaries: map[string]string{},
		modules: map[string]string{"stdlib": "v1.27.3", fixtureModule: ""},
	}
	for osv, reached := range levels {
		key := findingKey{osv: osv, module: "stdlib"}
		result.levels[key], result.versions[key], result.summaries[osv] = reached, "v1.27.3", "summary of "+osv
	}
	return result
}

// moduleOnlyDocument is a fixture document whose statements are about the
// module's own product alone, so every line [judge] writes is about the
// current tree.
func moduleOnlyDocument(osvs ...string) document {
	doc := fixtureDocument(osvs...)
	for i := range doc.Statements {
		doc.Statements[i].Products = doc.Statements[i].Products[:1]
	}
	return doc
}

// TestJudge_HoldsEachStatementToTheLevelTheScanReached covers each verdict a
// statement can get, by status and justification, against the level the scan
// reports for its advisory. A called function contradicts every statement,
// whatever its status or justification.
func TestJudge_HoldsEachStatementToTheLevelTheScanReached(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		status        string
		justification string
		reached       level
		label         string
		want          string
	}{
		{name: "not present, module", status: statusNotAffected, justification: "vulnerable_code_not_present", reached: levelModule, label: "stated", want: "not_affected (vulnerable_code_not_present), and the scan reports it against the module only"},
		{name: "not present, package", status: statusNotAffected, justification: "vulnerable_code_not_present", reached: levelPackage, label: "contradicted", want: "with a vulnerable package imported"},
		{name: "component not present, package", status: statusNotAffected, justification: "component_not_present", reached: levelPackage, label: "contradicted", want: "not_affected because component_not_present"},
		{name: "not in execute path, package", status: statusNotAffected, justification: "vulnerable_code_not_in_execute_path", reached: levelPackage, label: "stated", want: "not_affected (vulnerable_code_not_in_execute_path)"},
		{name: "mitigated, package", status: statusNotAffected, justification: "inline_mitigations_already_exist", reached: levelPackage, label: "stated", want: "inline_mitigations_already_exist"},
		{name: "under investigation, package", status: statusUnderInvestigation, reached: levelPackage, label: "stated", want: "GO-2099-0001 stdlib@v1.27.3: under_investigation, and the scan"},
		{name: "not in execute path, symbol", status: statusNotAffected, justification: "vulnerable_code_not_in_execute_path", reached: levelSymbol, label: "contradicted", want: "with a vulnerable function called, which make govulncheck fails on"},
		{name: "mitigated, symbol", status: statusNotAffected, justification: "inline_mitigations_already_exist", reached: levelSymbol, label: "contradicted", want: "so the document does not state it"},
		{name: "affected, symbol", status: statusAffected, reached: levelSymbol, label: "contradicted", want: "statement 1 (GO-2099-0001): the scan reports GO-2099-0001 against stdlib with a vulnerable function called"},
		{name: "under investigation, symbol", status: statusUnderInvestigation, reached: levelSymbol, label: "contradicted", want: "with a vulnerable function called"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := moduleOnlyDocument("GO-2099-0001")
			doc.Statements[0].Status, doc.Statements[0].Justification = tc.status, tc.justification
			rep := &report{}
			judge(rep, doc, fixtureGoMod, scanOf(map[string]level{"GO-2099-0001": tc.reached}))
			got := map[string][]string{"stated": rep.stated, "contradicted": rep.contradicted}
			if len(got[tc.label]) != 1 || !strings.Contains(got[tc.label][0], tc.want) || len(rep.stated)+len(rep.contradicted) != 1 {
				t.Errorf("stated %q, contradicted %q; want one %s line naming %q", rep.stated, rep.contradicted, tc.label, tc.want)
			}
			if len(rep.stale)+len(rep.unaccounted) != 0 {
				t.Errorf("stale %q, unaccounted %q; want none", rep.stale, rep.unaccounted)
			}
		})
	}
}

// holdsOneLineOrNone reports whether lines is empty when want is, and is
// otherwise one line containing want.
func holdsOneLineOrNone(lines []string, want string) bool {
	if want == "" {
		return len(lines) == 0
	}
	return len(lines) == 1 && strings.Contains(lines[0], want)
}

// TestJudge_AStatementTheScanDoesNotReport_IsJudgedByWhatIsPresent covers a
// statement about the module's own product whose advisory the scan does not
// report, which is stale when the scan loads the module or go.mod no longer
// requires it, and holds on go.mod's word when go.mod requires a module no
// loaded package comes from.
func TestJudge_AStatementTheScanDoesNotReport_IsJudgedByWhatIsPresent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, module, stale, stated string
	}{
		{name: "loaded and not reported", module: "stdlib", stale: "statement 1 (GO-2099-0001): the scan loads stdlib@v1.27.3 and does not report GO-2099-0001 against it"},
		{name: "required by go.mod alone", module: "example.com/testonly", stated: "GO-2099-0001 example.com/testonly@v0.3.0: not_affected (vulnerable_code_not_present), required by go.mod and outside the packages the scan loads"},
		{name: "gone from go.mod", module: "example.com/gone", stale: "statement 1 (GO-2099-0001): go.mod does not require example.com/gone and the scan does not load it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := moduleOnlyDocument("GO-2099-0001")
			doc.Statements[0].Products[0].Subcomponents = []component{{ID: "pkg:golang/" + tc.module}}
			rep := &report{}
			judge(rep, doc, fixtureGoMod, scanOf(nil))
			if !holdsOneLineOrNone(rep.stale, tc.stale) {
				t.Errorf("stale = %q, want %q", rep.stale, tc.stale)
			}
			if !holdsOneLineOrNone(rep.stated, tc.stated) {
				t.Errorf("stated = %q, want %q", rep.stated, tc.stated)
			}
			if len(rep.contradicted)+len(rep.unaccounted) != 0 {
				t.Errorf("contradicted %q, unaccounted %q; want none", rep.contradicted, rep.unaccounted)
			}
		})
	}
}

// TestJudge_AStatementAboutAnImage_IsNotHeldToTheCurrentTree covers the case
// the module's own claim leaves behind: once the advisory and its module are
// gone from the tree, a statement about a published image that carries them
// stays, about the version its subcomponent names, and nothing about it is
// stale or contradicted.
func TestJudge_AStatementAboutAnImage_IsNotHeldToTheCurrentTree(t *testing.T) {
	t.Parallel()

	doc := fixtureDocument("GO-2099-0001")
	doc.Statements[0].Products = doc.Statements[0].Products[1:]
	for _, tc := range []struct {
		name string
		scan scanResult
	}{
		{name: "the advisory gone", scan: scanOf(nil)},
		{name: "the function called", scan: scanOf(map[string]level{"GO-2099-0001": levelSymbol})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rep := &report{}
			judge(rep, doc, fixtureGoMod, tc.scan)
			want := "GO-2099-0001 stdlib@v1.27.0 in " + fixtureImage + ": not_affected (vulnerable_code_not_present), a published image, about the version it carries and not held to the current dependency set"
			if len(rep.stated) != 1 || !strings.HasPrefix(rep.stated[0], want) {
				t.Errorf("stated = %q, want one line starting %q", rep.stated, want)
			}
			if len(rep.stale)+len(rep.contradicted)+len(rep.unaccounted) != 0 {
				t.Errorf("stale %q, contradicted %q, unaccounted %q; want none", rep.stale, rep.contradicted, rep.unaccounted)
			}
		})
	}
}

// TestJudge_AnAdvisoryNoStatementAccountsFor_IsUnaccounted covers what
// OSPS-VM-04.02 asks for: every advisory the scan reports without a call is
// in the document, through a statement about this module, while one whose
// function is called is left to make govulncheck.
func TestJudge_AnAdvisoryNoStatementAccountsFor_IsUnaccounted(t *testing.T) {
	t.Parallel()

	imageOnly := fixtureDocument("GO-2099-0001")
	imageOnly.Statements[0].Products = imageOnly.Statements[0].Products[1:]
	for _, tc := range []struct {
		name string
		doc  document
		want []string
	}{
		{name: "no statement", doc: fixtureDocument(), want: []string{"GO-2099-0001 stdlib@v1.27.3, reported against the module only", "GO-2099-0002 stdlib@v1.27.3, reported with a vulnerable package imported"}},
		{name: "an image statement alone", doc: imageOnly, want: []string{"GO-2099-0001", "GO-2099-0002"}},
		{name: "both stated", doc: fixtureDocument("GO-2099-0001", "GO-2099-0002"), want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := tc.doc
			for i := range doc.Statements {
				doc.Statements[i].Justification = "vulnerable_code_not_in_execute_path"
			}
			rep := &report{}
			judge(rep, doc, fixtureGoMod, scanOf(map[string]level{"GO-2099-0001": levelModule, "GO-2099-0002": levelPackage, "GO-2099-0003": levelSymbol}))
			if len(rep.unaccounted) != len(tc.want) {
				t.Fatalf("unaccounted = %q, want %d lines", rep.unaccounted, len(tc.want))
			}
			for i, want := range tc.want {
				if !strings.HasPrefix(rep.unaccounted[i], want) || !strings.Contains(rep.unaccounted[i], "no statement about "+fixtureModule+" names it") {
					t.Errorf("unaccounted[%d] = %q, want it to start with %q", i, rep.unaccounted[i], want)
				}
			}
		})
	}
}

// TestReportWrite_PrintsWhatWasCheckedAndTheVerdict covers the three shapes
// of a report: a document that did not validate, one that held, and one with
// a finding of every kind.
func TestReportWrite_PrintsWhatWasCheckedAndTheVerdict(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		rep  report
		ok   bool
		want []string
	}{
		{
			name: "invalid",
			rep:  report{document: "vex.json", statements: 2, invalid: []string{"the document names no author"}},
			want: []string{
				"audit_vex: vex.json, 2 statements, not held to the dependency set until it reads as OpenVEX\n",
				"INVALID the document names no author\n",
				"audit_vex: FAILED: 1 problem with the document, 0 stale statements, 0 contradicted statements, 0 advisories unaccounted for\n",
			},
		},
		{
			name: "held",
			rep:  report{document: "vex.json", statements: 1, scanned: true, scan: scanOf(nil), patterns: []string{"./..."}, stated: []string{"GO-2099-0001 stdlib@v1.27.3: under_investigation"}},
			ok:   true,
			want: []string{
				"audit_vex: vex.json, 1 statement, held to a source scan of ./... (build tags: none) against file:///db (modified 2026-09-30T00:00:00Z), and to go.mod\n",
				"stated GO-2099-0001 stdlib@v1.27.3: under_investigation\n",
				"audit_vex: every statement holds, and every advisory the scan reports against code it does not affect is stated\n",
			},
		},
		{
			name: "every finding",
			rep: report{
				document: "vex.json", statements: 2, scanned: true, scan: scanOf(nil), patterns: []string{"./a", "./b"}, tags: "e2e",
				invalid: []string{"i1", "i2"}, stale: []string{"s"}, contradicted: []string{"c"}, unaccounted: []string{"u"},
			},
			want: []string{
				"audit_vex: vex.json, 2 statements, held to a source scan of ./a ./b (build tags: e2e)",
				"INVALID i1\nINVALID i2\nSTALE s\nCONTRADICTED c\nUNACCOUNTED u\n",
				"FAILED: 2 problems with the document, 1 stale statement, 1 contradicted statement, 1 advisory unaccounted for\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			tc.rep.write(&out)
			if tc.rep.ok() != tc.ok {
				t.Errorf("ok() = %v, want %v", tc.rep.ok(), tc.ok)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("the report lacks %q:\n%s", want, out.String())
				}
			}
		})
	}
}
