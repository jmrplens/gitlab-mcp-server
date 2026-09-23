package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// hintSite is one folded hint at a fixed position, so a test states the prose
// it is about and nothing else.
func hintSite(line int, value string) site {
	return site{Package: "p", File: "p/a.go", Line: line, Kind: kindErrorHint, Value: value, Resolved: true}
}

// TestClassify_HintSpellings_AreKeptApartByRule holds the three things a hint
// can name that a session cannot look up, and the one it can.
//
// The canonical ID is the whole point of the rule, so it has to be silent: a
// rule that reported every capability a hint names would say nothing about
// which spelling to use.
func TestClassify_HintSpellings_AreKeptApartByRule(t *testing.T) {
	report := classify([]site{
		hintSite(1, "read it back with demo.get"),
		hintSite(2, "list them with gitlab_demo_list first"),
		hintSite(3, "the demo.fetch action accepts the same arguments"),
		hintSite(4, "verify it with demo.gone"),
	}, stubCatalog(), false)

	want := []HintFinding{
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindErrorHint, Rule: ruleToolName, Name: "gitlab_demo_list"},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindErrorHint, Rule: ruleAlias, Name: "demo.fetch", Canonical: "demo.get"},
		{Package: "p", File: "p/a.go", Line: 4, Kind: kindErrorHint, Rule: ruleUnknownID, Name: "demo.gone", Closest: "demo.get"},
	}
	if !slices.Equal(report.Hints.Rows, want) {
		t.Errorf("hint findings = %+v, want %+v", report.Hints.Rows, want)
	}
	if report.Hints.Read != 4 || report.Hints.Packages != 1 {
		t.Errorf("read = %d in %d package(s), want 4 hints in 1 package", report.Hints.Read, report.Hints.Packages)
	}
	if report.Hints.ByRule[ruleToolName] != 1 || report.Hints.ByRule[ruleAlias] != 1 || report.Hints.ByRule[ruleUnknownID] != 1 {
		t.Errorf("findings by rule = %v, want one of each", report.Hints.ByRule)
	}
}

// TestClassify_Hints_FailTheGateAndTheirUnfoldableSitesDoNot holds the two
// halves of the flip apart, because they moved in opposite directions and one
// test asserting only the first would leave the second free.
//
// A hint that names a tool fails -check now. The rule was staged for exactly
// as long as the tree needed: it opened at 785 findings and a gate refusing
// them would have refused every push until the tree was clean, which is the
// opposite order to the one work happens in.
//
// A hint the type checker could not fold still fails nothing, which is a
// deliberate departure from how the gate treats its own unfoldable sites. A
// published ID that cannot be read is an ID nobody can check; an unfoldable
// hint is still a sentence a reader reads, and the three in the tree carry no
// tool name between them.
func TestClassify_Hints_FailTheGateAndTheirUnfoldableSitesDoNot(t *testing.T) {
	unfoldable := []site{
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindErrorHint, Expr: "buildHint(x)"},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHintField, Expr: "other.Hint"},
	}

	t.Run("a tool name fails it", func(t *testing.T) {
		report := classify(append([]site{hintSite(1, "list them with gitlab_demo_list first")}, unfoldable...), stubCatalog(), false)
		if report.Clean() {
			t.Errorf("Clean() = true over a hint naming a tool; hints %+v", report.Hints)
		}
		if report.Hints.Findings != 1 {
			t.Errorf("hint findings = %d, want the one tool name", report.Hints.Findings)
		}
	})

	t.Run("an unfoldable hint alone does not", func(t *testing.T) {
		report := classify(unfoldable, stubCatalog(), false)
		if !report.Clean() {
			t.Errorf("Clean() = false over unfoldable hints alone; summary %+v hints %+v", report.Summary, report.Hints)
		}
		if report.Summary.Unresolved != 0 || len(report.Unresolved) != 0 {
			t.Errorf("unresolved = %+v, want the hint sites kept out of the gate's bucket", report.Unresolved)
		}
		if report.Hints.Unfolded != 2 {
			t.Errorf("hints not folded = %d, want both", report.Hints.Unfolded)
		}
		if report.Hints.NotFolded[0].Expression != "buildHint(x)" || report.Hints.NotFolded[1].Kind != kindHintField {
			t.Errorf("not folded = %+v, want both sites with their own kind", report.Hints.NotFolded)
		}
	})
}

// TestClassify_HintsReadByKind_CountTheTwoSitesApart holds the split the
// report publishes: the argument of an error helper and the struct field a
// hint is written into are both judged, and a reader can tell how much of the
// figure comes from each.
func TestClassify_HintsReadByKind_CountTheTwoSitesApart(t *testing.T) {
	report := classify([]site{
		hintSite(1, "use gitlab_demo_list"),
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindHintField, Value: "use gitlab_demo_get", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindHintField, Value: "nothing to name here", Resolved: true},
	}, stubCatalog(), false)

	if report.Hints.ReadByKind[kindErrorHint] != 1 || report.Hints.ReadByKind[kindHintField] != 2 {
		t.Errorf("hints read by kind = %v, want one argument and two fields", report.Hints.ReadByKind)
	}
	if report.Hints.Findings != 2 {
		t.Errorf("findings = %+v, want one per tool name", report.Hints.Rows)
	}
}

// TestClassify_HintNamingOneToolTwice_IsOneFinding holds that a sentence is
// judged for what it names rather than for how often: the fix is one edit, and
// two rows would make the backlog read bigger than it is.
func TestClassify_HintNamingOneToolTwice_IsOneFinding(t *testing.T) {
	report := classify([]site{
		hintSite(1, "call gitlab_demo_list, then gitlab_demo_list again"),
	}, stubCatalog(), false)

	if len(report.Hints.Rows) != 1 || report.Hints.Rows[0].Name != "gitlab_demo_list" {
		t.Errorf("hint findings = %+v, want the name once", report.Hints.Rows)
	}
}

// TestClassify_HintProseToken_IsFilteredLikeAUsageLine holds that the dotted
// tokens of a hint are picked out by the shared rule rather than by their
// shape, so a hint quoting a file name or an example binding is silent.
func TestClassify_HintProseToken_IsFilteredLikeAUsageLine(t *testing.T) {
	report := classify([]site{
		hintSite(1, "see github.com for the details and go.mod for the version"),
		hintSite(2, "params.get is the binding, not an action"),
	}, stubCatalog(), false)

	if len(report.Hints.Rows) != 0 {
		t.Errorf("hint findings = %+v, want none: neither token is offered as an ID", report.Hints.Rows)
	}
}

// TestClassify_DeclaredHintToolToken_IsExcusedAndTracked holds the one
// declaration this rule has: a gitlab_-shaped token that names a GitLab
// template family rather than a tool is excused, and the entry it used is
// recorded so the stale check can tell a live declaration from a dead one.
func TestClassify_DeclaredHintToolToken_IsExcusedAndTracked(t *testing.T) {
	report := classify([]site{
		hintSite(1, "verify template_type (dockerfiles, gitlab_ci_ymls, licenses)"),
	}, stubCatalog(), true)

	if len(report.Hints.Rows) != 0 {
		t.Errorf("hint findings = %+v, want the declared token excused", report.Hints.Rows)
	}
	if len(report.Hints.StaleDeclarations) != 0 {
		t.Errorf("stale declarations = %v, want none: the entry excused a token", report.Hints.StaleDeclarations)
	}
}

// TestClassify_UnusedHintDeclaration_IsReportedStale holds the other half: a
// declaration that no hint spells any more is reported, on the terms every
// declaration table here is held to.
func TestClassify_UnusedHintDeclaration_IsReportedStale(t *testing.T) {
	report := classify([]site{hintSite(1, "nothing declared here")}, stubCatalog(), true)

	if len(report.Hints.StaleDeclarations) != 1 ||
		!strings.Contains(report.Hints.StaleDeclarations[0], "gitlab_ci_ymls") {
		t.Errorf("stale declarations = %v, want the unused entry named", report.Hints.StaleDeclarations)
	}
	for _, entry := range report.StaleExemptions {
		if strings.Contains(entry, "hintToolExemptions") {
			t.Errorf("the gate's stale list carries %q, which belongs to the rule that reports", entry)
		}
	}
	if report.Summary.Stale != len(report.StaleExemptions) {
		t.Errorf("stale count = %d over %d gate declaration(s); the hint table must not be counted there",
			report.Summary.Stale, len(report.StaleExemptions))
	}
}

// TestClassify_NarrowedRun_LeavesTheHintDeclarationUnjudged holds why the
// stale list is scoped to a whole-tree run: over one package every entry
// excuses nothing, and reporting it would be an answer about the patterns
// rather than about the declaration.
func TestClassify_NarrowedRun_LeavesTheHintDeclarationUnjudged(t *testing.T) {
	report := classify([]site{hintSite(1, "nothing declared here")}, stubCatalog(), false)

	if len(report.Hints.StaleDeclarations) != 0 {
		t.Errorf("stale declarations = %v, want none from a narrowed run", report.Hints.StaleDeclarations)
	}
}

// TestClassify_HintFindings_AreOrderedByPositionThenName holds that two runs
// over one tree produce the same work list, and that a row is placed by where
// it was written rather than by the order the walk happened to reach it.
func TestClassify_HintFindings_AreOrderedByPositionThenName(t *testing.T) {
	report := classify([]site{
		{Package: "q", File: "q/a.go", Line: 1, Kind: kindErrorHint, Value: "gitlab_zzz", Resolved: true},
		{Package: "p", File: "p/b.go", Line: 9, Kind: kindErrorHint, Value: "gitlab_mmm", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindErrorHint, Value: "gitlab_bbb and gitlab_aaa", Resolved: true},
	}, stubCatalog(), false)

	var got []string
	for _, row := range report.Hints.Rows {
		got = append(got, row.Package+" "+row.File+" "+row.Name)
	}
	want := []string{"p p/a.go gitlab_aaa", "p p/a.go gitlab_bbb", "p p/b.go gitlab_mmm", "q q/a.go gitlab_zzz"}
	if !slices.Equal(got, want) {
		t.Errorf("hint findings order = %v, want %v", got, want)
	}
}

// TestWriteHintReport_Rows_AreAskedForAndTheCountIsNot holds the split between
// what every run says and what -v adds. check-action-ids runs on every push
// and this rule fails nothing, so a log that carried hundreds of rows would
// bury the refusals a reader came for; the count still has to be there, since
// it is the figure the rule was built to produce.
func TestWriteHintReport_Rows_AreAskedForAndTheCountIsNot(t *testing.T) {
	report := classify([]site{
		hintSite(1, "list them with gitlab_demo_list first"),
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindErrorHint, Expr: "buildHint(x)"},
	}, stubCatalog(), false)

	var quiet, loud bytes.Buffer
	writeHintReport(&quiet, report.Hints, false)
	writeHintReport(&loud, report.Hints, true)

	const count = "error hints: 1 finding(s) in 1 package(s) over 1 hint(s) read; 1 not folded (reported, not gated)"
	for _, want := range []string{count, "hint findings by rule: tool_name 1"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(quiet.String(), want) {
				t.Errorf("the quiet report left out %q; got %q", want, quiet.String())
			}
		})
	}
	if strings.Contains(quiet.String(), "gitlab_demo_list") || strings.Contains(quiet.String(), "buildHint(x)") {
		t.Errorf("the quiet report printed rows: %q", quiet.String())
	}
	for _, want := range []string{
		`  p/a.go:1 error_hint "gitlab_demo_list" is a tool name; the dynamic surface registers no such tool`,
		"  p/a.go:2 error_hint buildHint(x)",
		"hints read by kind: error_hint 1",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(loud.String(), want) {
				t.Errorf("the verbose report left out %q; got %q", want, loud.String())
			}
		})
	}
}

// TestWriteHintReport_EachRule_ReadsWithItsOwnVerb holds that a row says what
// is wrong with the spelling it names. One verb for all three would report an
// alias as resolving to nothing while printing, on the same line, what it
// resolves to.
func TestWriteHintReport_EachRule_ReadsWithItsOwnVerb(t *testing.T) {
	cases := map[string]struct {
		row  HintFinding
		want string
	}{
		"tool name": {
			row:  HintFinding{Rule: ruleToolName, Name: "gitlab_demo_list"},
			want: "is a tool name; the dynamic surface registers no such tool",
		},
		"alias": {
			row:  HintFinding{Rule: ruleAlias, Name: "demo.fetch", Canonical: "demo.get"},
			want: "is an alias, not the catalog ID: demo.get",
		},
		"unknown id with a lead": {
			row:  HintFinding{Rule: ruleUnknownID, Name: "demo.gone", Closest: "demo.get"},
			want: "resolves to no action; closest: demo.get",
		},
		"unknown id with none": {
			row:  HintFinding{Rule: ruleUnknownID, Name: "nothing.likethis"},
			want: "resolves to no action",
		},
	}
	for name, one := range cases {
		t.Run(name, func(t *testing.T) {
			if got := hintVerb(one.row); got != one.want {
				t.Errorf("hintVerb(%+v) = %q, want %q", one.row, got, one.want)
			}
		})
	}
}

// TestWriteHintReport_StaleDeclaration_IsPrintedWhateverVerbosityAsks holds
// that a declaration which excuses nothing is named by every run. It is the
// one line here a reader has to act on, and unlike the rows it is a single
// line whatever the backlog looks like.
func TestWriteHintReport_StaleDeclaration_IsPrintedWhateverVerbosityAsks(t *testing.T) {
	report := classify([]site{hintSite(1, "nothing declared here")}, stubCatalog(), true)

	var quiet bytes.Buffer
	writeHintReport(&quiet, report.Hints, false)
	if !strings.Contains(quiet.String(), "gitlab_ci_ymls is no longer spelled in any hint (hintToolExemptions). Remove the entry.") {
		t.Errorf("the quiet report left out the stale declaration: %q", quiet.String())
	}
}

// TestWriteHintReport_NothingRead_PrintsTheCountAndNoBreakdown holds a run
// with no hints at all: the count is still printed, so a reader can tell the
// rule ran and found nothing from the rule not having run.
//
// The two headings are asserted absent as well, and that is what makes the
// emptiness of each list load-bearing rather than incidental: a heading is
// printed by a verbose run, so asking only for verbosity would announce a
// section and then print nothing under it, which reads as a section whose rows
// were lost.
func TestWriteHintReport_NothingRead_PrintsTheCountAndNoBreakdown(t *testing.T) {
	report := classify(nil, stubCatalog(), false)

	var out bytes.Buffer
	writeHintReport(&out, report.Hints, true)
	if !strings.Contains(out.String(), "error hints: 0 finding(s) in 0 package(s) over 0 hint(s) read; 0 not folded") {
		t.Errorf("report = %q, want the empty count", out.String())
	}
	if strings.Contains(out.String(), "by rule") || strings.Contains(out.String(), "by kind") {
		t.Errorf("report = %q, want no breakdown of nothing", out.String())
	}
	for _, heading := range []string{hintRowsHeading, hintNotFoldedHeading} {
		t.Run(heading, func(t *testing.T) {
			if strings.Contains(out.String(), heading) {
				t.Errorf("report = %q, want no heading over an empty list", out.String())
			}
		})
	}
}

// TestWriteAssertionReport_OneSection_PrintsItsOwnLabelsAndRows holds the
// suite's section in its own words. It is printed by the printer the hint
// section uses, so every label is asserted: a section that said "error hints"
// about the suite would send a reader to fix the server when the test is what
// quotes the wrong name.
func TestWriteAssertionReport_OneSection_PrintsItsOwnLabelsAndRows(t *testing.T) {
	report := classify([]site{
		{Package: "s", File: "s/a_test.go", Line: 1, Kind: kindAssertion, Value: "list them with gitlab_demo_list", Resolved: true},
		{Package: "s", File: "s/a_test.go", Line: 2, Kind: kindAssertion, Expr: "e.Name(x)"},
	}, stubCatalog(), false)

	var quiet, loud, empty bytes.Buffer
	writeAssertionReport(&quiet, report.Assertions, false)
	writeAssertionReport(&loud, report.Assertions, true)
	writeAssertionReport(&empty, classify(nil, stubCatalog(), false).Assertions, true)

	const count = "  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read; 1 not folded (reported, not gated)\n    assertion findings by rule: tool_name 1\n"
	if quiet.String() != count {
		t.Errorf("the quiet section = %q, want %q", quiet.String(), count)
	}
	wantLoud := strings.Join([]string{
		assertionRowsHeading,
		"=== s ===",
		`  s/a_test.go:1 assertion "gitlab_demo_list" is a tool name; the dynamic surface registers no such tool`,
		assertionNotFoldedHeading,
		"  s/a_test.go:2 assertion e.Name(x)",
		"  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read; 1 not folded (reported, not gated)",
		"    assertion findings by rule: tool_name 1",
		"    assertions read by kind: assertion 1",
		"",
	}, "\n")
	if loud.String() != wantLoud {
		t.Errorf("the verbose section:\n%s\nwant:\n%s", loud.String(), wantLoud)
	}
	const wantEmpty = "  e2e assertions: 0 finding(s) in 0 package(s) over 0 assertion(s) read; 0 not folded (reported, not gated)\n"
	if empty.String() != wantEmpty {
		t.Errorf("an empty section = %q, want the count and no heading: %q", empty.String(), wantEmpty)
	}
}

// TestWriteHintGroups_RowsOfSeveralPackages_CarryOneHeadingEach holds the
// grouping the verbose report reads by: a heading opens each package and the
// rows of one package sit under one heading, however many there are.
//
// Both halves are the same comparison read in opposite directions, and a
// report that got it backwards would be readable either way: every row under
// its own heading says the same thing the rows say, and no heading at all says
// nothing about which package a file belongs to.
func TestWriteHintGroups_RowsOfSeveralPackages_CarryOneHeadingEach(t *testing.T) {
	var out bytes.Buffer
	writeHintGroups(&out, []HintFinding{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindErrorHint, Name: "gitlab_demo_list", Rule: ruleToolName},
		{Package: "p", File: "p/b.go", Line: 2, Kind: kindErrorHint, Name: "gitlab_demo_get", Rule: ruleToolName},
		{Package: "q", File: "q/a.go", Line: 3, Kind: kindErrorHint, Name: "gitlab_other_list", Rule: ruleToolName},
	})

	var headings []string
	for line := range strings.SplitSeq(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if strings.HasPrefix(line, "===") {
			headings = append(headings, line)
		}
	}
	if want := []string{"=== p ===", "=== q ==="}; !slices.Equal(headings, want) {
		t.Errorf("headings = %v, want %v", headings, want)
	}
}
