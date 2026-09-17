package main

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// fixtureSinks indexes the fixture's sinks by the formatter they were written
// in, so a case can name a shape rather than a line.
func fixtureSinks(t *testing.T) map[string][]sink {
	t.Helper()
	prog := loadFixture(t, caseFixture)
	byFunc := map[string][]sink{}
	for _, s := range collectSinks(prog) {
		if !strings.HasPrefix(s.pkg.PkgPath, modulePath+"/"+fixtureDir) {
			continue
		}
		byFunc[enclosingFunc(s.pkg, s.call.Pos())] = append(byFunc[enclosingFunc(s.pkg, s.call.Pos())], s)
	}
	return byFunc
}

// TestCollectSinks_Fixture_FindsTheCallsThatWriteMarkdown checks which calls
// are sinks and which are not, which is the first thing a wrong answer here
// would silently change.
func TestCollectSinks_Fixture_FindsTheCallsThatWriteMarkdown(t *testing.T) {
	byFunc := fixtureSinks(t)

	cases := []struct {
		name  string
		fn    string
		holes int
	}{
		{name: "every formatting call in the formatter", fn: "FormatOutputMarkdown", holes: 8},
		{name: "the cell builder has one hole per argument", fn: "FormatSpread", holes: 1},
		{name: "a runtime template is not a sink", fn: "FormatDynamic", holes: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			holes := 0
			for _, s := range byFunc[tc.fn] {
				holes += len(s.holes)
			}
			if holes != tc.holes {
				t.Errorf("%s has %d hole(s), want %d", tc.fn, holes, tc.holes)
			}
		})
	}
}

// TestCollectSinks_Fixture_ReadsANamedConstantTemplate checks the case a
// regular expression over the source would miss: a formatter that passes a
// shared template constant instead of writing the string at the call.
func TestCollectSinks_Fixture_ReadsANamedConstantTemplate(t *testing.T) {
	byFunc := fixtureSinks(t)

	for _, s := range byFunc["FormatOutputMarkdown"] {
		for _, h := range s.holes {
			if h.ctx == ctxHeading {
				return
			}
		}
	}
	t.Error("the heading written through a named template constant was not found")
}

// TestSinkOf_Fixture_RefusesWhatCarriesNoTemplate checks the calls sinkOf has
// to decline: everything that is not fmt or a cell builder, an fmt function
// with no template, and a template that is not a constant.
func TestSinkOf_Fixture_RefusesWhatCarriesNoTemplate(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	fences := collectFences(prog)
	refused := map[string]bool{}
	for _, pkg := range prog.order {
		if !strings.HasPrefix(pkg.PkgPath, modulePath+"/"+fixtureDir) {
			continue
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if _, isSink := (sinkFile{pkg: pkg, fences: fences, cards: true}).sinkOf(call); !isSink {
					refused[calleeName(pkg, call)] = true
				}
				return true
			})
		}
	}

	for _, name := range []string{"Sprintf", "Itoa", "WriteString", "String", "repeat"} {
		t.Run(name, func(t *testing.T) {
			if !refused[name] {
				t.Errorf("no call of %s was refused, so the refusal path is untested", name)
			}
		})
	}
}

// edgeSinks indexes the edge fixture's sinks by the formatter they were
// written in.
func edgeSinks(t *testing.T) map[string][]sink {
	t.Helper()
	prog := loadFixture(t, edgeFixture)
	byFunc := map[string][]sink{}
	for _, s := range collectSinks(prog) {
		if !strings.HasPrefix(s.pkg.PkgPath, modulePath+"/"+fixtureDir) {
			continue
		}
		byFunc[enclosingFunc(s.pkg, s.call.Pos())] = append(byFunc[enclosingFunc(s.pkg, s.call.Pos())], s)
	}
	return byFunc
}

// holeExpressions renders the values one sink interpolates.
func holeExpressions(s sink) []string {
	rendered := make([]string, 0, len(s.holes))
	for _, h := range s.holes {
		rendered = append(rendered, types.ExprString(h.expr))
	}
	return rendered
}

// TestSinkOf_MoreVerbsThanOperands_SplitsOnlyTheOnesPassed checks that a
// template declaring a hole the call never fills is split into the holes the
// call does fill.
//
// fmt renders the unfilled one as %!s(MISSING), which carries no value at all,
// so pairing it with an argument would mean pairing it with whatever sits past
// the end of the list: the wrong value, or none.
func TestSinkOf_MoreVerbsThanOperands_SplitsOnlyTheOnesPassed(t *testing.T) {
	sinks := edgeSinks(t)["TooFewOperands"]
	if len(sinks) == 0 {
		t.Fatal("the fixture's short template is not a sink")
	}

	for _, s := range sinks {
		if len(s.holes) != 1 {
			t.Errorf("%s split into %v, want only the hole the call fills", s.callee, holeExpressions(s))
		}
	}
}

// TestSinkOf_CellBuilderWithNoCells_IsNotASink checks the guard on a row
// builder called with nothing to put in the row.
//
// The builders are variadic, so a call with no cells compiles, and without the
// guard it would be collected as a sink holding no value at all: a row in the
// report that names nothing and can be neither judged nor fixed.
func TestSinkOf_CellBuilderWithNoCells_IsNotASink(t *testing.T) {
	sinks := edgeSinks(t)["Cells"]

	if len(sinks) != 1 {
		t.Fatalf("Cells produced %d sinks, want only the call that has cells in it", len(sinks))
	}
	if got := holeExpressions(sinks[0]); len(got) != 2 {
		t.Errorf("the row with cells split into %v, want its two cells", got)
	}
}

// TestCardCells_RowOfConstants_IsNotACardRow checks what tells a labeled
// field from a row of fixed text.
//
// A card row is a label beside a value, so the second cell has to carry a
// value; a row whose every cell is constant is layout the formatter wrote and
// there is nothing for Card to render.
func TestCardCells_RowOfConstants_IsNotACardRow(t *testing.T) {
	for _, s := range edgeSinks(t)["Cells"] {
		for _, h := range s.holes {
			if h.ctx == ctxCard {
				t.Errorf("a row of constants was reported as the card row %q", h.text)
			}
		}
	}
}

// TestCardCells_OutOfScope_ReadsNoRow checks that the file scope decides
// before the shape does. card.go's own writes are the rows every other
// formatter is asked to use, so reading them as hand-written rows would report
// the fix as the defect.
func TestCardCells_OutOfScope_ReadsNoRow(t *testing.T) {
	prog := loadFixture(t, cardFixture)
	pkg := fixturePackage(t, prog, "mdcard")
	headers := callsNamed(pkg, "MarkdownTableHeader")
	if len(headers) == 0 {
		t.Fatal("the fixture calls no MarkdownTableHeader")
	}

	inScope := 0
	for _, call := range headers {
		if _, ok := (sinkFile{pkg: pkg, cards: true}).cardCells(call, "MarkdownTableHeader"); ok {
			inScope++
		}
		if row, ok := (sinkFile{pkg: pkg, cards: false}).cardCells(call, "MarkdownTableHeader"); ok {
			t.Errorf("a file the rule does not read still reported the card row %q", row.text)
		}
	}
	if inScope == 0 {
		t.Error("no field-table header was read as a card row in a file the rule does read")
	}
}

// TestCardCells_BuilderWithNoCardShape_ReadsNoRow checks that the card shapes
// are matched by name and that the match is closed.
//
// Each cell builder has a shape of its own: a header is a pair of constants
// naming the columns, a row is a constant label beside a value. A builder
// neither case names has neither shape, so it must read as no card rather than
// be measured against the shape of whichever case happens to be last.
func TestCardCells_BuilderWithNoCardShape_ReadsNoRow(t *testing.T) {
	prog := loadFixture(t, cardFixture)
	pkg := fixturePackage(t, prog, "mdcard")
	rows := callsNamed(pkg, "MarkdownTableRow")
	if len(rows) == 0 {
		t.Fatal("the fixture calls no MarkdownTableRow")
	}
	in := sinkFile{pkg: pkg, cards: true}

	for _, call := range rows {
		if len(call.Args) != 2 {
			continue
		}
		if row, ok := in.cardCells(call, "MarkdownTableFooter"); ok {
			t.Errorf("a builder with no card shape of its own reported the row %q", row.text)
		}
	}
}

// callsNamed collects every call of the named function in a loaded package.
func callsNamed(pkg *packages.Package, name string) []*ast.CallExpr {
	var found []*ast.CallExpr
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callee := calleeOf(pkg, call); callee != nil && callee.Name() == name {
				found = append(found, call)
			}
			return true
		})
	}
	return found
}

// TestFormatSink_CallsItCannotRead_AreNotSinks checks the two ways a
// formatting call arrives without a template the audit can read: too few
// arguments to hold one, and a template the type checker recorded no value
// for. Either way the holes are unknown, and a sink whose holes are unknown
// would be judged against a template that is not there.
func TestFormatSink_CallsItCannotRead_AreNotSinks(t *testing.T) {
	untyped := untypedPackage()
	in := sinkFile{pkg: untyped}

	cases := []struct {
		name string
		call *ast.CallExpr
		fn   string
	}{
		{
			name: "fewer arguments than the template position",
			call: &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("w")}},
			fn:   "Fprintf",
		},
		{
			name: "a template the type checker recorded nothing for",
			call: &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("tmpl"), ast.NewIdent("v")}},
			fn:   "Sprintf",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if s, ok := in.formatSink(tc.call, tc.fn); ok {
				t.Errorf("formatSink accepted the call, splitting it into %v", holeExpressions(s))
			}
		})
	}
}

// calleeName renders the function a call names, for the test above to group by.
func calleeName(pkg *packages.Package, call *ast.CallExpr) string {
	callee := calleeOf(pkg, call)
	if callee == nil {
		return "(a function value)"
	}
	return callee.Name()
}

// TestSinkOf_CellBuilders_TreatEveryArgumentAsACell checks that a call with no
// template at all is still split into cells, spread included.
func TestSinkOf_CellBuilders_TreatEveryArgumentAsACell(t *testing.T) {
	byFunc := fixtureSinks(t)

	cases := []struct {
		name string
		fn   string
		verb string
	}{
		{name: "one argument per cell", fn: "FormatOutputMarkdown", verb: "cell"},
		{name: "a slice spread into the builder", fn: "FormatSpread", verb: "cells..."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, s := range byFunc[tc.fn] {
				if !cellArgFuncs[s.callee] {
					continue
				}
				for _, h := range s.holes {
					if h.ctx != ctxCell {
						t.Errorf("%s writes a %s, want a table cell", s.callee, h.ctx)
					}
					if h.verb != tc.verb {
						t.Errorf("%s names its hole %q, want %q", s.callee, h.verb, tc.verb)
					}
				}
				return
			}
			t.Errorf("%s calls no cell builder", tc.fn)
		})
	}
}

// TestSinkOf_CardWrites_JudgesTheTwoTheCallerRendersFor checks that the two
// Card writes whose value the caller renders are sinks at the call site, in
// the construct each writes, and that every other Card method, which escapes
// what it is given, is not.
func TestSinkOf_CardWrites_JudgesTheTwoTheCallerRendersFor(t *testing.T) {
	holes, refused := cardMethodCalls(t)

	cases := []struct {
		name  string
		verbs string
		ctx   mdContext
	}{
		{name: "Markdown", verbs: "item", ctx: ctxListItem},
		// Two cells written one by one, then a slice spread into the row.
		{name: "Row", verbs: "cell cell cells...", ctx: ctxCell},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var verbs []string
			for _, h := range holes[tc.name] {
				verbs = append(verbs, h.verb)
				if h.ctx != tc.ctx {
					t.Errorf("%s hole is in %s, want %s", tc.name, h.ctx, tc.ctx)
				}
			}
			if got := strings.Join(verbs, " "); got != tc.verbs {
				t.Errorf("%s holes are %q, want %q", tc.name, got, tc.verbs)
			}
		})
	}
	for _, name := range []string{"Int", "Field", "Bool", "Time", "URL", "Table", "End", "Row"} {
		t.Run(name+" refused", func(t *testing.T) {
			if !refused[name] {
				t.Errorf("no call of Card.%s was refused: every other method escapes for itself, and a Row with no cells writes nothing", name)
			}
		})
	}
}

// cardMethodCalls runs sink recognition over every Card method call of the
// card-safe fixture, returning the holes of the calls read as sinks by method
// name, and the methods at least one call of which was refused.
func cardMethodCalls(t *testing.T) (holes map[string][]sinkHole, refused map[string]bool) {
	t.Helper()
	prog := loadFixture(t, cardFixture)
	fences := collectFences(prog)
	pkg := fixturePackage(t, prog, "mdcardsafe")
	holes = map[string][]sinkHole{}
	refused = map[string]bool{}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := calleeOf(pkg, call)
			if callee == nil || callee.Pkg() == nil || callee.Pkg().Path() != toolutilPath || callee.Signature().Recv() == nil {
				return true
			}
			s, isSink := (sinkFile{pkg: pkg, fences: fences, cards: true}).sinkOf(call)
			if !isSink {
				refused[callee.Name()] = true
				return true
			}
			holes[callee.Name()] = append(holes[callee.Name()], s.holes...)
			return true
		})
	}
	return holes, refused
}

// TestSinkHole_Verbs_SayWhichVerdictsApply checks the two predicates the
// audit splits a hole by: the boolean verb and the card shape are outside the
// escaping verdict, and the card shape, a fenced value and a spread slice are
// outside the raw one.
func TestSinkHole_Verbs_SayWhichVerdictsApply(t *testing.T) {
	cases := []struct {
		name      string
		hole      sinkHole
		escapable bool
		raw       bool
	}{
		{name: "a textual verb in a cell", hole: sinkHole{ctx: ctxCell, verb: "%s"}, escapable: true, raw: true},
		{name: "the boolean verb", hole: sinkHole{ctx: ctxCell, verb: "%t"}, escapable: false, raw: true},
		{name: "a card row", hole: sinkHole{ctx: ctxCard, verb: "row"}, escapable: false, raw: false},
		{name: "a value inside a fence", hole: sinkHole{ctx: ctxFence, verb: "%v"}, escapable: true, raw: false},
		{name: "a slice spread into a row", hole: sinkHole{ctx: ctxCell, verb: "cells..."}, escapable: true, raw: false},
		{name: "a prose verb", hole: sinkHole{ctx: ctxProse, verb: "%t"}, escapable: false, raw: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.hole.escapable(); got != tc.escapable {
				t.Errorf("escapable() = %v, want %v", got, tc.escapable)
			}
			if got := tc.hole.rawJudged(); got != tc.raw {
				t.Errorf("rawJudged() = %v, want %v", got, tc.raw)
			}
		})
	}
}

// TestEnclosingFunc_Fixture_NamesTheFormatterOrNothing checks both answers: a
// sink inside a function is named by it, and one written at package level is
// named by nothing rather than by whichever function happens to be first.
func TestEnclosingFunc_Fixture_NamesTheFormatterOrNothing(t *testing.T) {
	byFunc := fixtureSinks(t)

	if len(byFunc["FormatLinked"]) == 0 {
		t.Error("a sink inside FormatLinked was not attributed to it")
	}
	if len(byFunc[""]) == 0 {
		t.Error("the package-level sink was attributed to some function")
	}
}
