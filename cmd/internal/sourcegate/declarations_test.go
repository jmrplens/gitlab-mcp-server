// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sourcegate

import (
	"slices"
	"testing"
)

// categoryExample is the one category the tables under test define.
const categoryExample = "on-purpose"

// exampleCategories are the categories the tables under test may give.
var exampleCategories = map[string]string{categoryExample: "done on purpose, for a reason a reviewer read"}

// set is a set of the given strings, as a run's excused keys and scanned
// packages are kept.
func set(members ...string) map[string]struct{} {
	s := map[string]struct{}{}
	for _, member := range members {
		s[member] = struct{}{}
	}
	return s
}

// TestTable_UnknownCategories_NamesEachUndefinedOneInOrder verifies every
// entry giving a category the table does not define is named, an empty one
// included, in key order, and that an entry giving a defined one is not.
func TestTable_UnknownCategories_NamesEachUndefinedOneInOrder(t *testing.T) {
	table := Table{Categories: exampleCategories, Declared: map[string]Declaration{
		"p:C":       {Category: "also-made-up"},
		"p:A":       {Category: "made-up"},
		"p:B":       {Category: ""},
		"p:Defined": {Category: categoryExample},
	}}
	if got, want := table.UnknownCategories(), []string{"p:A", "p:B", "p:C"}; !slices.Equal(got, want) {
		t.Errorf("UnknownCategories = %v, want %v", got, want)
	}
	if got := (Table{Categories: exampleCategories}).UnknownCategories(); got != nil {
		t.Errorf("UnknownCategories of an empty table = %v, want none", got)
	}
}

// TestTable_StaleDeclarations_AreScopedToThePackagesScanned verifies an entry
// that excused nothing in a package the run scanned is stale, in key order;
// one that excused something is not, and neither is one for a package the run
// did not load, which is out of view rather than stale.
func TestTable_StaleDeclarations_AreScopedToThePackagesScanned(t *testing.T) {
	table := Table{Categories: exampleCategories, Declared: map[string]Declaration{
		"p:Used":      {Category: categoryExample},
		"p:Gone":      {Category: categoryExample},
		"p:AlsoGone":  {Category: categoryExample},
		"q:Elsewhere": {Category: categoryExample},
	}}
	got := table.staleDeclarations(set("p:Used"), set("p"))
	if want := []string{"p:AlsoGone", "p:Gone"}; !slices.Equal(got, want) {
		t.Errorf("staleDeclarations = %v, want %v", got, want)
	}
}

// exampleFinding is a finding as a gate's scanner would make one: where it
// sits, which is what a declaration is keyed on, and a mark telling two
// findings in one place apart.
type exampleFinding struct{ place, mark string }

// placeOf is the declaration key of an example finding.
func placeOf(f exampleFinding) string { return f.place }

// TestJudge_SplitsTheFindingsAndHoldsTheTableToTheRun verifies the whole
// verdict: the findings a declaration names are excused and the rest are
// reported, each in the order given; an entry that excused nothing is stale
// and one giving an undefined category is unknown, whatever it excused; and
// the unjudged files are sorted.
func TestJudge_SplitsTheFindingsAndHoldsTheTableToTheRun(t *testing.T) {
	table := Table{Categories: exampleCategories, Declared: map[string]Declaration{
		"p:Excused":  {Category: categoryExample, Reason: "on purpose"},
		"p:Invented": {Category: "invented", Reason: "nobody defined it"},
		"p:Gone":     {Category: categoryExample, Reason: "no longer there"},
	}}
	findings := []exampleFinding{
		{"p:Second", "1"}, {"p:Excused", "1"}, {"p:First", "1"}, {"p:Invented", "1"}, {"p:Excused", "2"},
	}
	verdict := Judge(findings, placeOf, table, set("p"), []string{"z.go", "a.go"})
	if want := []exampleFinding{{"p:Second", "1"}, {"p:First", "1"}}; !slices.Equal(verdict.Findings, want) {
		t.Errorf("Findings = %v, want %v", verdict.Findings, want)
	}
	if want := []exampleFinding{{"p:Excused", "1"}, {"p:Invented", "1"}, {"p:Excused", "2"}}; !slices.Equal(verdict.Excused, want) {
		t.Errorf("Excused = %v, want %v", verdict.Excused, want)
	}
	if want := []string{"p:Gone"}; !slices.Equal(verdict.Stale, want) {
		t.Errorf("Stale = %v, want %v", verdict.Stale, want)
	}
	if want := []string{"p:Invented"}; !slices.Equal(verdict.Unknown, want) {
		t.Errorf("Unknown = %v, want %v", verdict.Unknown, want)
	}
	if want := []string{"a.go", "z.go"}; !slices.Equal(verdict.Unjudged, want) {
		t.Errorf("Unjudged = %v, want %v", verdict.Unjudged, want)
	}
	if want := (Counts{Findings: 2, Excused: 3, Stale: 1, Unknown: 1, Unjudged: 2}); verdict.Counts() != want {
		t.Errorf("Counts = %+v, want %+v", verdict.Counts(), want)
	}
}

// TestJudge_ACleanRun_IsEmptyEverywhere verifies a run with nothing to report
// against an empty table gives an empty verdict, and that the files passed
// for sorting are not reordered under the caller.
func TestJudge_ACleanRun_IsEmptyEverywhere(t *testing.T) {
	verdict := Judge(nil, placeOf, Table{Categories: exampleCategories}, set("p"), nil)
	if verdict.Findings != nil || verdict.Excused != nil || verdict.Stale != nil || verdict.Unknown != nil || verdict.Unjudged != nil {
		t.Errorf("verdict = %+v, want every part empty", verdict)
	}
	unjudged := []string{"z.go", "a.go"}
	Judge(nil, placeOf, Table{}, nil, unjudged)
	if !slices.Equal(unjudged, []string{"z.go", "a.go"}) {
		t.Errorf("unjudged = %v after the verdict, want the caller's order kept", unjudged)
	}
}

// TestVerdict_OK_FailsOnEachKindOfProblem verifies each of the four ways a run
// has something to answer for fails it on its own, and that excused findings
// alone do not.
func TestVerdict_OK_FailsOnEachKindOfProblem(t *testing.T) {
	excused := []exampleFinding{{"p:F", "1"}}
	for _, tt := range []struct {
		name    string
		verdict Verdict[exampleFinding]
		want    bool
	}{
		{name: "nothing", want: true},
		{name: "only excused findings", verdict: Verdict[exampleFinding]{Excused: excused}, want: true},
		{name: "a finding", verdict: Verdict[exampleFinding]{Findings: excused}},
		{name: "a stale declaration", verdict: Verdict[exampleFinding]{Stale: []string{"p:F"}}},
		{name: "an unknown category", verdict: Verdict[exampleFinding]{Unknown: []string{"p:F"}}},
		{name: "an unjudged file", verdict: Verdict[exampleFinding]{Unjudged: []string{"a.go"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.verdict.OK(); got != tt.want {
				t.Errorf("OK() = %v, want %v", got, tt.want)
			}
		})
	}
}
