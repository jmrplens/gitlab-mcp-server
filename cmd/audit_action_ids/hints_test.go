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
// hint is still a sentence a reader reads, and the six in the tree carry no
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

// dynamicSentence is one sentence of the dynamic surface's own prose, which
// spells both of that surface's tools, so a whole-tree fixture can use every
// declaration of the served-prose rule.
func dynamicSentence(line int) site {
	return site{
		Package: "internal/tools/dynamic", File: "internal/tools/dynamic/register.go", Line: line,
		Kind: kindMessage, Value: "search with gitlab_find_action, then call gitlab_execute_action", Resolved: true,
	}
}

// TestClassify_DeclaredHintToolToken_IsExcusedAndTracked holds the two
// declarations this rule has: a gitlab_-shaped token that names a GitLab
// template family rather than a tool is excused anywhere, a tool the dynamic
// surface registers is excused in that surface's own package, and each entry
// used is recorded so the stale check can tell a live declaration from a dead
// one.
func TestClassify_DeclaredHintToolToken_IsExcusedAndTracked(t *testing.T) {
	report := classify([]site{
		hintSite(1, "verify template_type (dockerfiles, gitlab_ci_ymls, licenses)"),
		dynamicSentence(2),
	}, stubCatalog(), true)

	if len(report.Hints.Rows) != 0 {
		t.Errorf("hint findings = %+v, want every declared token excused", report.Hints.Rows)
	}
	if len(report.Hints.StaleDeclarations) != 0 {
		t.Errorf("stale declarations = %v, want none: every entry excused a token", report.Hints.StaleDeclarations)
	}
}

// TestClassify_DeclaredSurfaceToolMention_IsExcusedOnlyInItsPackage holds why
// the dynamic surface's tools are declared per package and not in the table
// that excuses a token anywhere: the same name in a sentence of any other
// package is served on a surface that does not register it.
func TestClassify_DeclaredSurfaceToolMention_IsExcusedOnlyInItsPackage(t *testing.T) {
	report := classify([]site{
		dynamicSentence(1),
		hintSite(2, "search with gitlab_find_action first"),
	}, stubCatalog(), false)

	want := []HintFinding{{Package: "p", File: "p/a.go", Line: 2, Kind: kindErrorHint, Rule: ruleToolName, Name: "gitlab_find_action"}}
	if !slices.Equal(report.Hints.Rows, want) {
		t.Errorf("hint findings = %+v, want only the other package's mention: %+v", report.Hints.Rows, want)
	}
}

// TestClassify_UnusedHintDeclaration_IsReportedStale holds the other half: a
// declaration that no served prose spells any more is reported, and it fails
// the gate on the terms every declaration table here is held to, in the
// section of the rule it belongs to rather than in the published-ID one.
func TestClassify_UnusedHintDeclaration_IsReportedStale(t *testing.T) {
	report := classify([]site{hintSite(1, "nothing declared here")}, stubCatalog(), true)

	want := []string{
		"gitlab_ci_ymls is no longer spelled in any served prose (hintToolExemptions)",
		"gitlab_execute_action is no longer spelled in the served prose of internal/tools/dynamic (declaredSurfaceToolMentions)",
		"gitlab_find_action is no longer spelled in the served prose of internal/tools/dynamic (declaredSurfaceToolMentions)",
	}
	if !slices.Equal(report.Hints.StaleDeclarations, want) {
		t.Errorf("stale declarations = %v, want every unused entry named: %v", report.Hints.StaleDeclarations, want)
	}
	for _, entry := range report.StaleExemptions {
		if strings.Contains(entry, "hintToolExemptions") || strings.Contains(entry, "declaredSurfaceToolMentions") {
			t.Errorf("the published-ID stale list carries %q, which belongs to the served-prose rule", entry)
		}
	}
	if report.Summary.Stale != len(report.StaleExemptions) {
		t.Errorf("stale count = %d over %d gate declaration(s); the served-prose tables must not be counted there",
			report.Summary.Stale, len(report.StaleExemptions))
	}
	if report.Clean() {
		t.Error("Clean() = true over declarations that excuse nothing")
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

// TestClassify_UsageLine_IsJudgedForToolNamesAndItsIDsOnce holds the one site
// judged in both sections, and what each section asks of it.
//
// A Usage line is served on every surface, so a tool name in it is the
// served-prose rule's finding. Its dotted IDs stay the published-ID section's:
// judged there once, with the declared alias mentions excused, where judging
// them in both sections would count a bad ID twice and refuse the issue.update
// line that names issue.close on purpose. A Description with the same text is
// not judged for tool names, since an individual tool's Description is served
// by that tool alone.
func TestClassify_UsageLine_IsJudgedForToolNamesAndItsIDsOnce(t *testing.T) {
	usage := func(line int, value string) site {
		return site{Package: "p", File: "p/specs.go", Line: line, Kind: kindUsage, Value: value, Resolved: true}
	}
	report := classify([]site{
		usage(1, "Use after gitlab_demo_list."),
		usage(2, "Execute also accepts issue.close."),
		usage(3, "Use after demo.gone."),
		{Package: "p", File: "p/specs.go", Line: 4, Kind: kindDescription, Value: "See also: gitlab_demo_list.", Resolved: true},
	}, stubCatalog("issue.update"), false)

	wantRows := []HintFinding{{Package: "p", File: "p/specs.go", Line: 1, Kind: kindUsage, Rule: ruleToolName, Name: "gitlab_demo_list"}}
	if !slices.Equal(report.Hints.Rows, wantRows) {
		t.Errorf("served prose findings = %+v, want only the Usage line's tool name: %+v", report.Hints.Rows, wantRows)
	}
	if report.Summary.Findings != 1 || report.Findings[0].ID != "demo.gone" {
		t.Errorf("published-ID findings = %+v, want the one unknown ID, counted once", report.Findings)
	}
	if report.Summary.AliasHits != 0 {
		t.Errorf("alias references = %+v, want the declared mentions excused", report.AliasRefs)
	}
	if report.Hints.ReadByKind[kindUsage] != 3 || report.Hints.Read != 3 {
		t.Errorf("read by kind = %v over %d, want the three Usage lines and no Description", report.Hints.ReadByKind, report.Hints.Read)
	}
}

// TestClassify_UsageLineNothingFolds_IsServedProseNotFolded holds where a
// Usage line folded as a sentence leaves what it cannot read: the half of it
// nothing folds is listed with the served prose nothing folds, and a value a
// format of it reports is counted as passed over. Neither is a published ID
// nobody can read, so neither fails the gate, and neither is counted as read.
func TestClassify_UsageLineNothingFolds_IsServedProseNotFolded(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/specs.go", Line: 1, Kind: kindUsage, Expr: "leads[action]"},
		{Package: "p", File: "p/specs.go", Line: 2, Kind: kindUsage, Expr: "name", PassedOver: true},
	}, stubCatalog(), false)

	if len(report.Unresolved) != 0 || !report.Clean() {
		t.Errorf("published-ID unresolved = %+v, clean %t; want neither site in that section", report.Unresolved, report.Clean())
	}
	if report.Hints.Unfolded != 1 || report.Hints.NotFolded[0].Expression != "leads[action]" {
		t.Errorf("served prose not folded = %+v, want the one half nothing folds", report.Hints.NotFolded)
	}
	if report.Hints.PassedOver != 1 || report.Hints.Read != 0 {
		t.Errorf("passed over %d, read %d; want the format's value counted and nothing read", report.Hints.PassedOver, report.Hints.Read)
	}
}

// TestClassify_UsageLineFromAFormat_IsJudgedWithItsVerbsMasked holds both
// rules a Usage line is judged by to the masking every other sentence gets.
// The line keeps its verbs for the fixer; unmasked, "%s.get" is the dotted
// token s.get, which the published-ID rule would refuse, and
// "%sgitlab_demo_list" hides the tool name the served-prose rule refuses.
func TestClassify_UsageLineFromAFormat_IsJudgedWithItsVerbsMasked(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/specs.go", Line: 1, Kind: kindUsage, Value: "Read %s.get after %sgitlab_demo_list.", Resolved: true},
	}, stubCatalog(), false)

	want := []HintFinding{{Package: "p", File: "p/specs.go", Line: 1, Kind: kindUsage, Rule: ruleToolName, Name: "gitlab_demo_list"}}
	if !slices.Equal(report.Hints.Rows, want) {
		t.Errorf("served prose findings = %+v, want the tool name the verb hid: %+v", report.Hints.Rows, want)
	}
	if report.Summary.Findings != 0 {
		t.Errorf("published-ID findings = %+v, want none from a verb", report.Findings)
	}
}

// TestClassify_FormatVerbs_AreMaskedBeforeJudging holds what a format's verbs
// are to the two rules: a word the sentence does not spell.
//
// Unmasked, "%s.get" reads as the dotted token s.get, whose right half is an
// action name the catalog uses, and "%sgitlab_demo_list" hides the tool name
// behind a word character. A verb with flags and a width is a verb too.
func TestClassify_FormatVerbs_AreMaskedBeforeJudging(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindMessage, Value: "lookup %s.get failed: %w", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindMessage, Value: "use %sgitlab_demo_list", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindMessage, Value: "%-10qgitlab_demo_get and 100%% of %[1]d.list", Resolved: true},
	}, stubCatalog(), false)

	want := []HintFinding{
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindMessage, Rule: ruleToolName, Name: "gitlab_demo_list"},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindMessage, Rule: ruleToolName, Name: "gitlab_demo_get"},
	}
	if !slices.Equal(report.Hints.Rows, want) {
		t.Errorf("findings = %+v, want the tool names the verbs hid and no ID they spelled: %+v", report.Hints.Rows, want)
	}
}

// TestMaskVerbs_OnlyVerbsAreMasked holds the masking on its own: every verb is
// a space, and the text around it is kept as written.
func TestMaskVerbs_OnlyVerbsAreMasked(t *testing.T) {
	for text, want := range map[string]string{
		"demo %d: use %q":           "demo  : use  ",
		"100%% sure":                "100  sure",
		"%-10.2f and %[2]*d":        "  and  ",
		"no verbs, 50% of them":     "no verbs, 50% of them",
		"50% gitlab_demo_list here": "50% gitlab_demo_list here",
		"issue.get stays as is.":    "issue.get stays as is.",
	} {
		t.Run(text, func(t *testing.T) {
			if got := maskVerbs(text); got != want {
				t.Errorf("maskVerbs(%q) = %q, want %q", text, got, want)
			}
		})
	}
}

// TestClassify_SchemaDescription_MayNameADeclaredAliasInDynamicAlone holds
// the one served sentence that consults the declared alias mentions: the
// description of dynamic execute's action parameter, whose subject is that
// execute accepts an alias, names issue.close as its example.
//
// Two things are held. The allowance is dynamic's: a schema description in
// any other package naming the alias is refused like any other sentence. And
// the use keeps the declaration alive, because the declaration now gives that
// description as its reason: with no Usage line naming issue.close, the entry
// is still in use, while issue.reopen, which no sentence here names, is stale.
func TestClassify_SchemaDescription_MayNameADeclaredAliasInDynamicAlone(t *testing.T) {
	report := classify([]site{
		{Package: dynamicPackage, File: "internal/tools/dynamic/register.go", Line: 1, Kind: kindSchemaDescription, Value: "An ID, or an alias such as issue.close.", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindSchemaDescription, Value: "An alias such as issue.close.", Resolved: true},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindMessage, Value: "then call issue.close", Resolved: true},
	}, stubCatalog("issue.update"), true)

	want := []HintFinding{
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindSchemaDescription, Rule: ruleAlias, Name: "issue.close", Canonical: "issue.update"},
		{Package: "p", File: "p/a.go", Line: 3, Kind: kindMessage, Rule: ruleAlias, Name: "issue.close", Canonical: "issue.update"},
	}
	if !slices.Equal(report.Hints.Rows, want) {
		t.Errorf("findings = %+v, want every alias outside dynamic: %+v", report.Hints.Rows, want)
	}
	stale := strings.Join(report.StaleExemptions, "\n")
	if strings.Contains(stale, "issue.close") {
		t.Errorf("stale = %v, want issue.close kept alive by dynamic's description", report.StaleExemptions)
	}
	if !strings.Contains(stale, "issue.reopen") {
		t.Errorf("stale = %v, want issue.reopen named: the judgement ran and nothing used it", report.StaleExemptions)
	}
}

// TestClassify_AQuotedAlias_KeepsNoDeclarationAlive holds the other site that
// consults the declared alias mentions. A quotation of issue.close is excused,
// since the Usage line it quotes names it on purpose, and it keeps no entry
// alive, since the entry is about the served source and not about its test.
func TestClassify_AQuotedAlias_KeepsNoDeclarationAlive(t *testing.T) {
	report := classify([]site{
		{Package: "test/e2e/gitlab/common", File: "test/e2e/gitlab/common/issues_test.go", Line: 1, Kind: kindAssertion, Value: "issue.close", Resolved: true},
	}, stubCatalog("issue.update"), true)

	if len(report.Assertions.Rows) != 0 {
		t.Errorf("assertion findings = %+v, want the quoted alias excused", report.Assertions.Rows)
	}
	if !strings.Contains(strings.Join(report.StaleExemptions, "\n"), "issue.close") {
		t.Errorf("stale = %v, want issue.close stale: a quotation keeps nothing alive", report.StaleExemptions)
	}
}

// TestClassify_PassedOverValues_AreCountedAndNotJudged holds what a value a
// sentence reports is to the report: a count, apart from both the sentences
// read and the sites nothing folds.
func TestClassify_PassedOverValues_AreCountedAndNotJudged(t *testing.T) {
	report := classify([]site{
		{Package: "p", File: "p/a.go", Line: 1, Kind: kindMessage, Expr: "c.Message", PassedOver: true},
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindMessage, Expr: "err", PassedOver: true},
	}, stubCatalog(), false)

	if report.Hints.PassedOver != 2 || report.Hints.Read != 0 || report.Hints.Unfolded != 0 || !report.Clean() {
		t.Errorf("passed over %d, read %d, not folded %d, clean %t; want two counted and nothing else", report.Hints.PassedOver, report.Hints.Read, report.Hints.Unfolded, report.Clean())
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

// TestWriteHintReport_QuietRun_PrintsTheRowsAndNotWhatFailsNothing holds the
// split between what every run says and what -v adds. check-action-ids passes
// no -v and a finding fails it, so its log has to carry the row a reader acts
// on: a count and a rule name alone would send them to run the audit again to
// learn which line to fix. What fails nothing, the site nothing folded and the
// breakdown by kind, is what -v is for.
func TestWriteHintReport_QuietRun_PrintsTheRowsAndNotWhatFailsNothing(t *testing.T) {
	report := classify([]site{
		hintSite(1, "list them with gitlab_demo_list first"),
		{Package: "p", File: "p/a.go", Line: 2, Kind: kindErrorHint, Expr: "buildHint(x)"},
	}, stubCatalog(), false)

	var quiet, loud bytes.Buffer
	writeHintReport(&quiet, report.Hints, false)
	writeHintReport(&loud, report.Hints, true)

	const row = `  p/a.go:1 error_hint "gitlab_demo_list" is a tool name; the dynamic surface registers no such tool`
	const count = "served prose: 1 finding(s) in 1 package(s) over 1 sentence(s) read; 1 not folded (reported, not gated); 0 value(s) passed over"
	for _, want := range []string{hintRowsHeading, row, count, "served prose findings by rule: tool_name 1"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(quiet.String(), want) {
				t.Errorf("the quiet report left out %q; got %q", want, quiet.String())
			}
		})
	}
	for _, verboseOnly := range []string{hintNotFoldedHeading, "  p/a.go:2 error_hint buildHint(x)", "served prose read by kind: error_hint 1"} {
		t.Run(verboseOnly, func(t *testing.T) {
			if strings.Contains(quiet.String(), verboseOnly) {
				t.Errorf("the quiet report printed %q, which -v is for; got %q", verboseOnly, quiet.String())
			}
			if !strings.Contains(loud.String(), verboseOnly) {
				t.Errorf("the verbose report left out %q; got %q", verboseOnly, loud.String())
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
	if !strings.Contains(quiet.String(), "gitlab_ci_ymls is no longer spelled in any served prose (hintToolExemptions). Remove the entry.") {
		t.Errorf("the quiet report left out the stale declaration: %q", quiet.String())
	}
}

// TestWriteHintReport_NothingRead_PrintsTheCountAndNoBreakdown holds a run
// with no hints at all: the count is still printed, so a reader can tell the
// rule ran and found nothing from the rule not having run.
//
// The two headings are asserted absent as well, and that is what makes the
// emptiness of each list load-bearing rather than incidental: the run is a
// verbose one, which prints both lists, so a guard that asked for verbosity or
// for nothing at all would announce a section and then print nothing under
// it, which reads as a section whose rows were lost.
func TestWriteHintReport_NothingRead_PrintsTheCountAndNoBreakdown(t *testing.T) {
	report := classify(nil, stubCatalog(), false)

	var out bytes.Buffer
	writeHintReport(&out, report.Hints, true)
	if !strings.Contains(out.String(), "served prose: 0 finding(s) in 0 package(s) over 0 sentence(s) read; 0 not folded (reported, not gated); 0 value(s) passed over") {
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
// section uses, so every label is asserted: a section that said "served prose"
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

	wantQuiet := strings.Join([]string{
		assertionRowsHeading,
		"=== s ===",
		`  s/a_test.go:1 assertion "gitlab_demo_list" is a tool name; the dynamic surface registers no such tool`,
		"  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read; 1 not folded (reported, not gated); 0 value(s) passed over",
		"    assertion findings by rule: tool_name 1",
		"",
	}, "\n")
	if quiet.String() != wantQuiet {
		t.Errorf("the quiet section:\n%s\nwant:\n%s", quiet.String(), wantQuiet)
	}
	wantLoud := strings.Join([]string{
		assertionRowsHeading,
		"=== s ===",
		`  s/a_test.go:1 assertion "gitlab_demo_list" is a tool name; the dynamic surface registers no such tool`,
		assertionNotFoldedHeading,
		"  s/a_test.go:2 assertion e.Name(x)",
		"  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read; 1 not folded (reported, not gated); 0 value(s) passed over",
		"    assertion findings by rule: tool_name 1",
		"    assertions read by kind: assertion 1",
		"",
	}, "\n")
	if loud.String() != wantLoud {
		t.Errorf("the verbose section:\n%s\nwant:\n%s", loud.String(), wantLoud)
	}
	const wantEmpty = "  e2e assertions: 0 finding(s) in 0 package(s) over 0 assertion(s) read; 0 not folded (reported, not gated); 0 value(s) passed over\n"
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
