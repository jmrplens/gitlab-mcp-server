// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// report_test.go covers how the statements are held to a scan and how the
// verdict is printed: a statement that holds, a stale one, a contradicted
// one, and an advisory no statement accounts for.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// scanOf is a scan result reporting each advisory against the standard
// library at the level given.
func scanOf(levels map[string]level) scanResult {
	result := scanResult{db: "file:///db", dbModified: "2026-09-30T00:00:00Z", levels: map[findingKey]level{}, versions: map[findingKey]string{}, summaries: map[string]string{}}
	for osv, reached := range levels {
		key := findingKey{osv: osv, module: "stdlib"}
		result.levels[key], result.versions[key], result.summaries[osv] = reached, "v1.27.3", "summary of "+osv
	}
	return result
}

// TestJudge_HoldsEachStatementToTheLevelTheScanReached covers each verdict a
// statement can get, by status and justification, against the level the scan
// reports for its advisory.
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
		{name: "not in execute path, symbol", status: statusNotAffected, justification: "vulnerable_code_not_in_execute_path", reached: levelSymbol, label: "contradicted", want: "with a vulnerable function called"},
		{name: "mitigated, symbol", status: statusNotAffected, justification: "inline_mitigations_already_exist", reached: levelSymbol, label: "stated", want: "inline_mitigations_already_exist"},
		{name: "under investigation, symbol", status: statusUnderInvestigation, reached: levelSymbol, label: "stated", want: "GO-2099-0001 stdlib@v1.27.3: under_investigation, and the scan"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := fixtureDocument("GO-2099-0001")
			doc.Statements[0].Status, doc.Statements[0].Justification = tc.status, tc.justification
			rep := &report{}
			judge(rep, doc, fixtureModule, scanOf(map[string]level{"GO-2099-0001": tc.reached}))
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

// TestJudge_AStatementTheScanDoesNotReport_IsStale covers a statement whose
// advisory the dependency set no longer contains.
func TestJudge_AStatementTheScanDoesNotReport_IsStale(t *testing.T) {
	t.Parallel()

	rep := &report{}
	judge(rep, fixtureDocument("GO-2099-0001"), fixtureModule, scanOf(nil))
	if len(rep.stale) != 1 || !strings.Contains(rep.stale[0], "does not contain GO-2099-0001 against stdlib") {
		t.Errorf("stale = %q, want the statement named", rep.stale)
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
			judge(rep, doc, fixtureModule, scanOf(map[string]level{"GO-2099-0001": levelModule, "GO-2099-0002": levelPackage, "GO-2099-0003": levelSymbol}))
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
				"audit_vex: FAILED: 1 problems with the document, 0 stale and 0 contradicted statements, 0 advisories unaccounted for\n",
			},
		},
		{
			name: "held",
			rep:  report{document: "vex.json", statements: 1, scanned: true, scan: scanOf(nil), patterns: []string{"./..."}, stated: []string{"GO-2099-0001 stdlib@v1.27.3: under_investigation"}},
			ok:   true,
			want: []string{
				"audit_vex: vex.json, 1 statements, held to a source scan of ./... (build tags: none) against file:///db (modified 2026-09-30T00:00:00Z)\n",
				"stated GO-2099-0001 stdlib@v1.27.3: under_investigation\n",
				"audit_vex: every statement holds, and every advisory the scan reports against code it does not affect is stated\n",
			},
		},
		{
			name: "every finding",
			rep: report{
				document: "vex.json", statements: 2, scanned: true, scan: scanOf(nil), patterns: []string{"./a", "./b"}, tags: "e2e",
				stale: []string{"s"}, contradicted: []string{"c"}, unaccounted: []string{"u1", "u2"},
			},
			want: []string{
				"source scan of ./a ./b (build tags: e2e)",
				"STALE s\nCONTRADICTED c\nUNACCOUNTED u1\nUNACCOUNTED u2\n",
				"FAILED: 0 problems with the document, 1 stale and 1 contradicted statements, 2 advisories unaccounted for\n",
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
