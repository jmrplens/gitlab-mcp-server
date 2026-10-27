// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// catalogFixtureDir is where the catalog fixtures are written: real packages
// on disk under testdata, which every gate of the tree passes over and which
// this package's tests name one by one, since a pattern ending in /... does
// not descend into testdata.
const catalogFixtureDir = "cmd/audit_md_escaping/testdata"

// catalogFixturePatterns load the two catalog fixtures beside the catalog
// package itself, whose own calls of New must never read as renders.
var catalogFixturePatterns = []string{
	"./" + catalogFixtureDir + "/catalogsinks",
	"./" + catalogFixtureDir + "/catalogcards",
	"./internal/i18n",
}

// catalogFixtureTemplates are the English templates of the fixture keys, which
// the tests install in place of the shipped catalog's so a fixture reads the
// same whatever the catalog holds.
var catalogFixtureTemplates = map[string]string{
	"fixture.cell":      "| {name} | {count} |",
	"fixture.heading":   "## {title} ({kind})",
	"fixture.list":      "- {flag}: {items}",
	"fixture.link":      "See [{label}]({url})",
	"fixture.fence":     "```\n{body}\n```",
	"fixture.prose":     "Created {title}.",
	"fixture.pair":      "| {first} | {second} |",
	"fixture.kind":      "kind",
	"fixture.inner":     "{text}",
	"fixture.summary":   "Create issue in {project}?\n\n**Title**: {title}",
	"fixture.labels":    "\n**Labels**: {labels}",
	"fixture.pair_row":  "| Name | {name} |",
	"fixture.header":    "| Field | Value |\n| --- | --- |",
	"fixture.emoji_row": "- {icon} **State**: {state}",
	"fixture.author":    "- **{author}**",
	"fixture.id_cell":   "| {value} |",
	"fixture.question":  "Delete {name}?",
}

// useFixtureTemplates installs the fixture templates for one test.
func useFixtureTemplates(t *testing.T) {
	t.Helper()
	original := englishTemplate
	t.Cleanup(func() { englishTemplate = original })
	englishTemplate = func(key string) (string, bool) {
		template, ok := catalogFixtureTemplates[key]
		return template, ok
	}
}

// loadCatalogFixture loads the catalog fixtures from disk, with the fixture
// templates installed.
func loadCatalogFixture(t *testing.T) *program {
	t.Helper()
	useFixtureTemplates(t)
	prog, err := cachedLoad(repoRoot(t), catalogFixturePatterns, nil)
	if err != nil {
		t.Fatalf("load the catalog fixtures: %v", err)
	}
	return prog
}

// catalogPackage returns one loaded catalog fixture package by name.
func catalogPackage(t *testing.T, prog *program, name string) *packages.Package {
	t.Helper()
	for _, pkg := range prog.order {
		if pkg.PkgPath == modulePath+"/"+catalogFixtureDir+"/"+name {
			return pkg
		}
	}
	t.Fatalf("catalog fixture %s was not loaded", name)
	return nil
}

// catalogCalls returns every call in a package of a function or method of the
// catalog package, in source order.
func catalogCalls(pkg *packages.Package) []*ast.CallExpr {
	var calls []*ast.CallExpr
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callee := calleeOf(pkg, call); callee != nil && callee.Pkg() != nil && callee.Pkg().Path() == i18nPath {
				calls = append(calls, call)
			}
			return true
		})
	}
	return calls
}

// spellHole renders one hole the way the cases below name it: the construct,
// how the call names the hole, the expression and, for a placeholder the call
// does not show the value of, that it is unbound.
func spellHole(h sinkHole) string {
	spelled := h.ctx.String() + " " + h.verb + " " + types.ExprString(h.expr)
	if h.text != "" {
		spelled = h.ctx.String() + " " + h.verb + " " + h.text
	}
	if h.unbound != "" {
		spelled += " (unbound)"
	}
	return spelled
}

// catalogHoles collects the holes of every sink one fixture package holds,
// by the function the sink is written in.
func catalogHoles(t *testing.T, prog *program, name string) map[string][]string {
	t.Helper()
	pkg := catalogPackage(t, prog, name)
	byFunc := map[string][]string{}
	for _, s := range collectSinks(prog) {
		if s.pkg != pkg {
			continue
		}
		fn := enclosingFunc(s.pkg, s.call.Pos())
		for _, h := range s.holes {
			byFunc[fn] = append(byFunc[fn], spellHole(h))
		}
	}
	return byFunc
}

// TestCatalogSink_Fixture_PlacesEveryPlaceholderTheCallFills holds what a
// render of the catalog is read as, function by function: a placeholder
// filled by S is a hole in the construct its line opens; one filled by M or L
// is a hole for every value an S of the messages they nest carries, through
// any depth of M, L and Concat, and for every message or argument among them
// the call does not build inline, which is unbound; one filled by N, or by
// messages that carry no value, is no hole at all; one no argument names is
// filled by the first argument the call does not name it by; a key that is
// not a constant, or that arrives in the results of one call, is a hole of
// its own; and a key with no template, or a template the call fills nothing
// of, is no sink. A nested message is a render of its own as well, read from
// its own template.
func TestCatalogSink_Fixture_PlacesEveryPlaceholderTheCallFills(t *testing.T) {
	prog := loadCatalogFixture(t)
	holes := catalogHoles(t, prog, "catalogsinks")

	cases := []struct {
		fn   string
		want []string
	}{
		{fn: "Cell", want: []string{"table-cell {name} issue.Title"}},
		{fn: "EscapedCell", want: []string{"table-cell {name} toolutil.EscapeMdTableCell(issue.Title)"}},
		{fn: "Heading", want: []string{"heading {title} issue.Title"}},
		{fn: "List", want: []string{"list-item {flag} strconv.FormatBool(done)"}},
		{fn: "Link", want: []string{"link-label {label} toolutil.EscapeMdLinkLabel(issue.Title)", "link-destination {url} issue.URL"}},
		{fn: "Fence", want: []string{"fence {body} issue.Body"}},
		{fn: "Prose", want: []string{"prose {title} issue.Title"}},
		{fn: "Repeated", want: []string{"table-cell {first} toolutil.EscapeMdTableCell(issue.Title)", "table-cell {second} toolutil.EscapeMdTableCell(issue.URL)"}},
		{fn: "Missing", want: []string{"table-cell {first} toolutil.EscapeMdTableCell(issue.Title)"}},
		{fn: "Held", want: []string{"table-cell {first} held (unbound)", "table-cell {second} issue.URL"}},
		{fn: "Spread", want: []string{"table-cell {first} args (unbound)", "table-cell {second} args (unbound)"}},
		{fn: "Named", want: []string{"table-cell {first} i18n.S(name, issue.Title) (unbound)", "table-cell {second} i18n.S(name, issue.Title) (unbound)"}},
		{fn: "Dynamic", want: []string{"catalog-key key key"}},
		{fn: "Unknown"},
		{fn: "Joined", want: []string{"table-cell {name} issue.Title"}},
		{fn: "Nested", want: []string{"table-cell {name} issue.Title", "prose {text} issue.Title"}},
		{fn: "Listed", want: []string{
			"table-cell {name} toolutil.EscapeMdTableCell(issue.Title)",
			"table-cell {name} issue.Body",
			"prose {text} toolutil.EscapeMdTableCell(issue.Title)",
			"prose {text} issue.Body",
		}},
		{fn: "HeldMessage", want: []string{"table-cell {first} held (unbound)", "table-cell {second} items (unbound)"}},
		{fn: "HeldInside", want: []string{
			"table-cell {first} held (unbound)",
			"table-cell {second} parts (unbound)",
			"prose {text} held (unbound)",
		}},
		{fn: "Returned", want: []string{"table-cell {first} titleMessage(issue) (unbound)", "table-cell {second} build() (unbound)"}},
		{fn: "titleMessage", want: []string{"prose {text} issue.Title"}},
		{fn: "Results", want: []string{"catalog-key key renderPair(ctx)"}},
		{fn: "ResultsInside", want: []string{
			"table-cell {first} i18n.New(keyArg(issue)) (unbound)",
			`table-cell {second} "x"`,
			"catalog-key key keyArg(issue)",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			if got := holes[tc.fn]; !slices.Equal(got, tc.want) {
				t.Errorf("%s holes = %q, want %q", tc.fn, got, tc.want)
			}
		})
	}
}

// TestCatalogSink_Fixture_ReadsCardRowsAsAnFmtTemplateWouldBe holds the card
// rule over templates of the catalog: a placeholder is read as the %s verb an
// fmt template would carry, so a bare row, a two-cell row with a constant
// label, a field header and a row with an emoji before its label are rows,
// and a placeholder in emphasis is a value rather than a label. The row is
// named as the catalog spells it.
func TestCatalogSink_Fixture_ReadsCardRowsAsAnFmtTemplateWouldBe(t *testing.T) {
	prog := loadCatalogFixture(t)
	holes := catalogHoles(t, prog, "catalogcards")

	cases := []struct {
		fn   string
		want []string
	}{
		{fn: "Declared", want: []string{
			"prose {project} toolutil.EscapeConsentValue(project)",
			"prose {title} toolutil.EscapeConsentValue(title)",
			"card row **Title**: {title}",
			"prose {labels} toolutil.EscapeConsentValue(labels)",
			"card row **Labels**: {labels}",
		}},
		{fn: "Undeclared", want: []string{
			"card header | Field | Value |",
			"table-cell {name} toolutil.EscapeMdTableCell(result.Name)",
			"card row | Name | {name} |",
			"list-item {icon} \"x\"",
			"list-item {state} toolutil.EscapeMdTableCell(result.State)",
			"card row - {icon} **State**: {state}",
		}},
		{fn: "Author", want: []string{"list-item {author} toolutil.EscapeMdTableCell(result.Name)"}},
		{fn: "Identifier", want: []string{"table-cell {value} result.ID"}},
		{fn: "Gone", want: []string{"prose {name} result.Name"}},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			if got := holes[tc.fn]; !slices.Equal(got, tc.want) {
				t.Errorf("%s holes = %q, want %q", tc.fn, got, tc.want)
			}
		})
	}
}

// TestCatalogSink_TheCatalogsOwnCalls_AreNotRenders holds the catalog package
// apart: T renders by calling New with the key it was handed, which is no
// constant, and reading that as a render would report the machinery every
// render goes through.
func TestCatalogSink_TheCatalogsOwnCalls_AreNotRenders(t *testing.T) {
	prog := loadCatalogFixture(t)
	for _, s := range collectSinks(prog) {
		if s.pkg.PkgPath == i18nPath {
			t.Errorf("a call in the catalog package was read as a sink: %s at %s", s.callee, prog.position(s.call.Pos()))
		}
	}
}

// TestCatalogSink_NotARender_IsNotASink holds the catalog functions that
// render nothing apart from T and New: an argument constructor called on its
// own, a join of messages and a method of a message are no sinks.
func TestCatalogSink_NotARender_IsNotASink(t *testing.T) {
	prog := loadCatalogFixture(t)
	pkg := catalogPackage(t, prog, "catalogsinks")
	file := sinkFile{pkg: pkg, fences: collectFences(prog), cards: true}
	seen := map[string]bool{}
	for _, call := range catalogCalls(pkg) {
		callee := calleeOf(pkg, call)
		if _, isSink := file.sinkOf(call); isSink {
			seen[callee.Name()+" sink"] = true
			continue
		}
		seen[callee.Name()] = true
	}
	for _, name := range []string{"S", "N", "M", "L", "Concat", "Text"} {
		t.Run(name, func(t *testing.T) {
			if !seen[name] || seen[name+" sink"] {
				t.Errorf("calls of %s: read %v, want each refused as a sink", name, seen)
			}
		})
	}
}

// TestAudit_CatalogFixture_JudgesEveryHoleOfARender holds the whole audit
// over the catalog fixtures: what fails, what is unresolved, what a directive
// excuses, and the directive that answers nothing.
func TestAudit_CatalogFixture_JudgesEveryHoleOfARender(t *testing.T) {
	prog := loadCatalogFixture(t)
	sel, err := parseContexts("all,card,bool-time")
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	report := audit(prog, sel, repoRoot(t))

	// Findings that share a line and an expression have no order between
	// them, so the lists are compared sorted.
	spell := func(findings []Finding) []string {
		var spelled []string
		for _, finding := range findings {
			spelled = append(spelled, strings.TrimPrefix(finding.Package, catalogFixtureDir+"/")+" "+finding.Func+" "+finding.Context+" "+finding.Verb+" "+finding.Expression)
		}
		slices.Sort(spelled)
		return spelled
	}
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "findings", got: spell(report.Findings), want: []string{
			"catalogcards Undeclared card header | Field | Value |",
			"catalogcards Undeclared card row - {icon} **State**: {state}",
			"catalogcards Undeclared card row | Name | {name} |",
			"catalogsinks Cell table-cell {name} issue.Title",
			"catalogsinks Dynamic catalog-key key key",
			"catalogsinks Fence fence {body} issue.Body",
			"catalogsinks Heading heading {title} issue.Title",
			"catalogsinks Held table-cell {second} issue.URL",
			"catalogsinks Joined table-cell {name} issue.Title",
			"catalogsinks Link link-destination {url} issue.URL",
			"catalogsinks List bool-time {flag} strconv.FormatBool(done)",
			"catalogsinks Listed table-cell {name} issue.Body",
			"catalogsinks Nested table-cell {name} issue.Title",
			"catalogsinks Results catalog-key key renderPair(ctx)",
			"catalogsinks ResultsInside catalog-key key keyArg(issue)",
		}},
		{name: "unresolved", got: spell(report.Unresolved), want: []string{
			"catalogsinks Held table-cell {first} held",
			"catalogsinks HeldInside table-cell {first} held",
			"catalogsinks HeldInside table-cell {second} parts",
			"catalogsinks HeldMessage table-cell {first} held",
			"catalogsinks HeldMessage table-cell {second} items",
			"catalogsinks Named table-cell {first} i18n.S(name, issue.Title)",
			"catalogsinks Named table-cell {second} i18n.S(name, issue.Title)",
			"catalogsinks ResultsInside table-cell {first} i18n.New(keyArg(issue))",
			"catalogsinks Returned table-cell {first} titleMessage(issue)",
			"catalogsinks Returned table-cell {second} build()",
			"catalogsinks Spread table-cell {first} args",
			"catalogsinks Spread table-cell {second} args",
		}},
		{name: "excused", got: spell(report.Excused), want: []string{
			"catalogcards Declared card row **Labels**: {labels}",
			"catalogcards Declared card row **Title**: {title}",
			"catalogcards Identifier table-cell {value} result.ID",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.Equal(tc.got, tc.want) {
				t.Errorf("%s =\n%s\nwant\n%s", tc.name, strings.Join(tc.got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
	var stale []string
	for _, directive := range report.StaleDirectives {
		stale = append(stale, string(directive.Kind)+" "+directive.Expression)
	}
	if want := []string{"card Gone"}; !slices.Equal(stale, want) {
		t.Errorf("stale directives = %q, want %q", stale, want)
	}
}

// TestAudit_CatalogFixture_ANonConstantKey_IsAFindingWhateverIsJudged holds
// the one finding a render carries whichever contexts a run judges: with no
// key there is no template, so a run that judges headings alone cannot say
// none of the render's placeholders sits in one. A key that arrives in the
// results of one call, with the context before it or with an argument after
// it, is no constant either.
func TestAudit_CatalogFixture_ANonConstantKey_IsAFindingWhateverIsJudged(t *testing.T) {
	prog := loadCatalogFixture(t)
	sel, err := parseContexts("heading")
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	report := audit(prog, sel, repoRoot(t))

	var keys []string
	for _, finding := range report.Findings {
		if finding.Context != "catalog-key" {
			continue
		}
		keys = append(keys, finding.Func+" "+finding.Expression)
		if finding.Wants != "a key constant declared in internal/i18n" ||
			finding.Reason != "the key is not a constant, so the template the values land in cannot be read" {
			t.Errorf("catalog-key finding = %+v", finding)
		}
		if finding.File != catalogFixtureDir+"/catalogsinks/catalogsinks.go" || finding.Line == 0 {
			t.Errorf("catalog-key finding sits at %s:%d, want the key argument in the fixture", finding.File, finding.Line)
		}
	}
	slices.Sort(keys)
	if want := []string{"Dynamic key", "Results renderPair(ctx)", "ResultsInside keyArg(issue)"}; !slices.Equal(keys, want) {
		t.Errorf("catalog-key findings = %q, want %q", keys, want)
	}
	// The run judged the one heading of the fixtures and the three keys, and
	// a key is counted among the holes it judged like any other.
	if report.Summary.Holes != 4 {
		t.Errorf("holes judged = %d, want the heading and the three keys", report.Summary.Holes)
	}
}

// TestCatalogSink_ARenderWithNothingToJudge_IsNotASink holds a render whose
// template carries no placeholder and no row apart from one that does: it
// writes nothing this server did not, so it is no sink, as an fmt template
// with nothing to judge is not.
func TestCatalogSink_ARenderWithNothingToJudge_IsNotASink(t *testing.T) {
	prog := loadCatalogFixture(t)
	pkg := catalogPackage(t, prog, "catalogsinks")
	file := sinkFile{pkg: pkg, fences: collectFences(prog), cards: true}
	var plain, judged int
	for _, call := range catalogCalls(pkg) {
		callee := calleeOf(pkg, call)
		if callee.Name() != "New" {
			continue
		}
		_, isSink := file.sinkOf(call)
		if key := types.ExprString(call.Args[0]); key == "keyKind" {
			plain++
			if isSink {
				t.Errorf("New(keyKind), whose template is plain text, was read as a sink")
			}
			continue
		}
		judged++
	}
	if plain == 0 || judged == 0 {
		t.Errorf("read %d plain renders and %d others, want both kinds from the fixture", plain, judged)
	}
}

// TestAudit_CatalogFixture_AnUnboundPlaceholder_SaysWhyItCannotBeFollowed
// holds the reason an unbound placeholder is unresolved for, which is a
// different blind spot from a value the walk lost: the call does not name the
// argument that fills it, at the top of the render or inside a message nested
// in it, or it fills the placeholder with a message it does not build inline
// out of arguments it names.
func TestAudit_CatalogFixture_AnUnboundPlaceholder_SaysWhyItCannotBeFollowed(t *testing.T) {
	prog := loadCatalogFixture(t)
	sel, err := parseContexts(allContexts)
	if err != nil {
		t.Fatalf("parseContexts: %v", err)
	}
	report := audit(prog, sel, repoRoot(t))
	const (
		byArgument = "filled by an argument that is not an inline i18n.S, N, M or L call with a constant name, so which value lands here cannot be read from the call"
		byMessage  = "filled by a catalog message the call does not build inline, with i18n.New or i18n.Concat, out of arguments it names, so which values it carries cannot be read from the call"
	)
	want := map[string]string{
		"Spread {first} args":                           byArgument,
		"Spread {second} args":                          byArgument,
		"HeldInside {first} held":                       byArgument,
		"HeldInside {second} parts":                     byMessage,
		"HeldMessage {first} held":                      byMessage,
		"HeldMessage {second} items":                    byMessage,
		"ResultsInside {first} i18n.New(keyArg(issue))": byMessage,
		"Returned {first} titleMessage(issue)":          byMessage,
		"Returned {second} build()":                     byMessage,
	}
	got := map[string]string{}
	for _, finding := range report.Unresolved {
		if _, asked := want[finding.Func+" "+finding.Verb+" "+finding.Expression]; asked {
			got[finding.Func+" "+finding.Verb+" "+finding.Expression] = finding.Reason
		}
	}
	for hole, reason := range want {
		t.Run(hole, func(t *testing.T) {
			if got[hole] != reason {
				t.Errorf("unbound placeholder reason = %q, want %q", got[hole], reason)
			}
		})
	}
}

// TestExecute_CatalogFixture_FailsTheGate holds the gate over a tree that
// renders a raw value through the catalog.
func TestExecute_CatalogFixture_FailsTheGate(t *testing.T) {
	useFixtureTemplates(t)
	var out, errOut strings.Builder

	code := execute(auditRun{dir: repoRoot(t), patterns: catalogFixturePatterns, contexts: allContexts, check: true}, &out, &errOut)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1\n%s%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "catalogsinks.go") {
		t.Errorf("the failing run does not name the fixture:\n%s", out.String())
	}
}

// TestCatalogEnglishTemplate_ShippedCatalog_AnswersByKey holds the default
// template source to the catalog compiled into the binary, the one a real run
// reads.
func TestCatalogEnglishTemplate_ShippedCatalog_AnswersByKey(t *testing.T) {
	if template, ok := catalogEnglishTemplate("common.list.and"); !ok || template != " and " {
		t.Errorf("common.list.and = %q, %v, want the shipped conjunction", template, ok)
	}
	if template, ok := catalogEnglishTemplate("fixture.cell"); ok {
		t.Errorf("a key the catalog does not hold answered %q", template)
	}
}

// TestI18nPath_UnderTheModulePath_MovesWithIt holds the written-out path to
// the module, so a module move still has one place to land.
func TestI18nPath_UnderTheModulePath_MovesWithIt(t *testing.T) {
	if want := modulePath + "/internal/i18n"; i18nPath != want {
		t.Errorf("i18nPath = %q, want %q", i18nPath, want)
	}
}

// TestAsFormat_Lines_WritesEveryPlaceholderAsAVerb holds the rendering the
// card rule reads a catalog line by.
func TestAsFormat_Lines_WritesEveryPlaceholderAsAVerb(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{line: "**Title**: {title}", want: "**Title**: %s"},
		{line: "| {a} | {b_2} |", want: "| %s | %s |"},
		{line: "{Title} and {2x} and {}", want: "{Title} and {2x} and {}"},
		{line: "no placeholder", want: "no placeholder"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			if got := asFormat(tc.line); got != tc.want {
				t.Errorf("asFormat(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}
