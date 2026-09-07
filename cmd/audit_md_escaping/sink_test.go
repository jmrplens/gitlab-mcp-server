package main

import (
	"go/ast"
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
				if _, isSink := sinkOf(pkg, call); !isSink {
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
