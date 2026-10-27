// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// plantedDir is the planted module named by tree, below this package's
// testdata.
func plantedDir(t *testing.T, tree string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", tree))
	if err != nil {
		t.Fatalf("resolve the planted tree %s: %v", tree, err)
	}
	return dir
}

// plantedConfig is a run over the default patterns of a planted module, with
// its stand-in as the catalog and the given declarations.
func plantedConfig(t *testing.T, tree string, declared map[string]sourcegate.Declaration) auditConfig {
	t.Helper()
	return auditConfig{
		dir:      plantedDir(t, tree),
		patterns: defaultPatterns,
		catalog:  "example.com/" + tree + "/internal/i18n",
		declared: declared,
	}
}

// auditPlanted audits a planted module and returns the report.
func auditPlanted(t *testing.T, cfg auditConfig) Report {
	t.Helper()
	report, err := audit(cfg)
	if err != nil {
		t.Fatalf("audit %s: %v", cfg.dir, err)
	}
	return report
}

// spell renders findings the way the expectations below are written.
func spell(findings []Finding) []string {
	spelled := make([]string, 0, len(findings))
	for _, f := range findings {
		spelled = append(spelled, fmt.Sprintf("%s:%d %s %s: %s", f.File, f.Line, f.Rule, f.Func, f.Reason))
	}
	return spelled
}

// findingsDeclarations is the table the findings tree is held to: an entry
// that excuses a conversion, one that excuses a conversion under a category
// nobody defined, one whose function converts nothing, and one naming a
// package the run never loads, which is out of view rather than stale.
var findingsDeclarations = map[string]sourcegate.Declaration{
	"example.com/findings/internal/reader:Excused": {Category: categoryCatalogReader, Reason: "reads a key from source"},
	"example.com/findings/internal/reader:Odd":     {Category: "nobody-defined-this", Reason: "an excuse nobody reviewed"},
	"example.com/findings/internal/reader:Stale":   {Category: categoryCatalogReader, Reason: "used to read a key"},
	"example.com/elsewhere:Gone":                   {Category: categoryCatalogReader, Reason: "a package this run never loads"},
}

// plantedFindings is every finding the findings tree holds, in the report's
// order, each written where its fixture says it is.
var plantedFindings = []string{
	`cmd/tool/main.go:14 I2 key: converts "flow.pair" to i18n.Key, which only the catalog package does`,
	`cmd/tool/main.go:17 I2 main: names its key with key, which is not a key constant declared in example.com/findings/internal/i18n`,
	`internal/flows/flows.go:20 I1 Unfilled: leaves {second} of flow.pair unfilled`,
	`internal/flows/flows.go:25 I1 Extra: fills {third}, which the English template of flow.pair does not carry`,
	`internal/flows/flows.go:30 I1 Twice: fills {first} twice`,
	`internal/flows/flows.go:35 I1 Held: passes held, which is not an inline i18n.S, N, M or L call, so the placeholder it fills cannot be read from the call`,
	`internal/flows/flows.go:41 I1 Helped: passes firstArg(a), which is not an inline i18n.S, N, M or L call, so the placeholder it fills cannot be read from the call`,
	`internal/flows/flows.go:41 I1 Helped: passes makeArg("second", b), which is not an inline i18n.S, N, M or L call, so the placeholder it fills cannot be read from the call`,
	`internal/flows/flows.go:46 I1 Named: names a placeholder with firstName, which is not a string literal`,
	`internal/flows/flows.go:46 I1 Named: names a placeholder with name, which is not a string literal`,
	`internal/flows/flows.go:51 I1 Spread: spreads args into the call, so the placeholders its arguments fill cannot be read from it`,
	`internal/flows/flows.go:56 I2 ByLiteral: names its key with "flow.literal", which is not a key constant declared in example.com/findings/internal/i18n`,
	`internal/flows/flows.go:61 I2 ByLocal: names its key with localKey, which is not a key constant declared in example.com/findings/internal/i18n`,
	`internal/flows/flows.go:66 I2 ByVariable: names its key with key, which is not a key constant declared in example.com/findings/internal/i18n`,
	`internal/flows/flows.go:71 I2 Converted: converts s to i18n.Key, which only the catalog package does`,
	`internal/flows/flows.go:71 I2 Converted: names its key with i18n.Key(s), which is not a key constant declared in example.com/findings/internal/i18n`,
	`internal/flows/flows.go:80 I2 render: uses i18n.T as a value, so the key of a call made through it cannot be read`,
	`internal/flows/flows.go:84 I4 Verbatim: uses i18n.Verbatim outside a test file, which serves one text in every language`,
	`internal/flows/flows.go:84 I4 Verbatim: uses i18n.Verbatim outside a test file, which serves one text in every language`,
	`internal/flows/flows.go:89 I5 Literal: writes an i18n.Text literal, which only the catalog package makes`,
	`internal/flows/flows.go:89 I5 Literal: writes an i18n.Text literal, which only the catalog package makes`,
	`internal/flows/flows.go:106 I2 Results: takes its key from the results of pair(ctx), so which key it renders cannot be read from the call`,
	`internal/flows/flows.go:116 I5 Empty: calls i18n.Concat with nothing to join, which renders the empty text in every language`,
	`internal/flows/flows.go:116 I5 Empty: writes an i18n.Message literal, which only the catalog package makes`,
	`internal/i18n/catalog_en.go:11 I3 english: flow.dead has an English template and nothing outside the catalog files references its key`,
	`internal/i18n/catalog_en.go:12 I3 english: flow.test_only has an English template and nothing outside the catalog files references its key`,
	`internal/i18n/catalog_en.go:13 I3 english: flow.literal has an English template and nothing outside the catalog files references its key`,
	`internal/i18n/keys.go:13 I3 Missing: names flow.missing, which the English catalog has no template for`,
}

// TestAudit_FindingsTree_ReportsEveryPlantedShape is the gate held whole over
// the tree that breaks every rule: each shape is reported where its fixture
// says, the conversion a declaration answers is excused, the declaration that
// answers nothing and the one with an undefined category are reported, the
// file a constraint hides is named, and the figures say what the run was
// over.
func TestAudit_FindingsTree_ReportsEveryPlantedShape(t *testing.T) {
	report := auditPlanted(t, plantedConfig(t, "findings", findingsDeclarations))

	if got := spell(report.Findings); !slices.Equal(got, plantedFindings) {
		t.Errorf("findings =\n%s\nwant\n%s", lines(got), lines(plantedFindings))
	}
	wantExcused := []string{
		`internal/reader/reader.go:10 I2 Excused: converts s to i18n.Key, which only the catalog package does`,
		`internal/reader/reader.go:15 I2 Odd: converts s to i18n.Key, which only the catalog package does`,
	}
	if got := spell(report.Excused); !slices.Equal(got, wantExcused) {
		t.Errorf("excused =\n%s\nwant\n%s", lines(got), lines(wantExcused))
	}
	if want := []string{"example.com/findings/internal/reader:Stale"}; !slices.Equal(report.Stale, want) {
		t.Errorf("stale = %v, want %v", report.Stale, want)
	}
	if want := []string{"example.com/findings/internal/reader:Odd"}; !slices.Equal(report.Unknown, want) {
		t.Errorf("unknown = %v, want %v", report.Unknown, want)
	}
	if want := []string{"internal/flows/hidden.go"}; !slices.Equal(report.Unjudged, want) {
		t.Errorf("unjudged = %v, want %v", report.Unjudged, want)
	}
	want := Summary{
		Packages: 4, Renders: 15, Keys: 5, WholeTree: true,
		Findings: len(plantedFindings), Excused: 2, Stale: 1, Unknown: 1, Unjudged: 1,
	}
	if report.Summary != want {
		t.Errorf("summary = %+v, want %+v", report.Summary, want)
	}
	if report.OK() {
		t.Error("a report holding findings reads as clean")
	}
}

// TestAudit_PackageLevelConversion_IsDeclaredByItsVariable holds how a
// conversion outside every function is declared: by the variable its
// initializer belongs to, which is the name its finding gives it, so the one
// declaration answers it and the finding moves to the excused ones.
func TestAudit_PackageLevelConversion_IsDeclaredByItsVariable(t *testing.T) {
	declared := maps.Clone(findingsDeclarations)
	declared["example.com/findings/cmd/tool:key"] = sourcegate.Declaration{Category: categoryCatalogReader, Reason: "reads a key at package level"}
	report := auditPlanted(t, plantedConfig(t, "findings", declared))

	conversion := plantedFindings[0]
	if slices.Contains(spell(report.Findings), conversion) {
		t.Errorf("the declared conversion is still a finding: %s", conversion)
	}
	if !slices.Contains(spell(report.Excused), conversion) {
		t.Errorf("excused =\n%s\nwant it to hold %s", lines(spell(report.Excused)), conversion)
	}
	if len(report.Findings) != len(plantedFindings)-1 || len(report.Stale) != 1 {
		t.Errorf("findings = %d and stale = %v, want one finding fewer and the one stale declaration", len(report.Findings), report.Stale)
	}
}

// TestAudit_CleanTree_PassesTheGate holds the other side: a tree that renders
// every text through the catalog the way the rules ask, whose test file does
// what only a test may and whose catalog package renders with a key that is
// no constant, is clean, and says what it was clean over.
func TestAudit_CleanTree_PassesTheGate(t *testing.T) {
	report := auditPlanted(t, plantedConfig(t, "clean", map[string]sourcegate.Declaration{}))

	if !report.OK() {
		t.Errorf("the clean tree is not clean: findings %s, stale %v, unknown %v, unjudged %v",
			lines(spell(report.Findings)), report.Stale, report.Unknown, report.Unjudged)
	}
	want := Summary{Packages: 3, Renders: 7, Keys: 5, WholeTree: true}
	if report.Summary != want {
		t.Errorf("summary = %+v, want %+v", report.Summary, want)
	}
}

// TestAudit_NarrowedRun_DoesNotJudgeDeadKeys holds the one rule a run over
// part of the tree cannot answer: a key the rest of the tree renders would
// read as one nothing renders, so dead keys are judged only over the default
// patterns, while a key with no template is still a finding.
func TestAudit_NarrowedRun_DoesNotJudgeDeadKeys(t *testing.T) {
	cfg := plantedConfig(t, "findings", findingsDeclarations)
	cfg.patterns = []string{"./internal/reader"}
	report := auditPlanted(t, cfg)

	want := []string{`internal/i18n/keys.go:13 I3 Missing: names flow.missing, which the English catalog has no template for`}
	if got := spell(report.Findings); !slices.Equal(got, want) {
		t.Errorf("findings =\n%s\nwant\n%s", lines(got), lines(want))
	}
	if report.Summary.WholeTree || report.Summary.Packages != 2 {
		t.Errorf("summary = %+v, want a narrowed run over the reader and the catalog", report.Summary)
	}
}

// lines joins a list one entry per line, for a failure message.
func lines(entries []string) string {
	var joined strings.Builder
	for _, entry := range entries {
		joined.WriteString("  " + entry + "\n")
	}
	return joined.String()
}
