package main

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/packages"
)

// sink is one call that writes Markdown with runtime values in it, already
// split into the places those values land.
type sink struct {
	pkg    *packages.Package
	call   *ast.CallExpr
	callee string
	holes  []sinkHole
}

// sinkHole is one place a runtime value lands: the expression that produces
// it, the Markdown construct it lands in, and how the call names it, which a
// report prints so a reader can find the hole in the template.
type sinkHole struct {
	expr ast.Expr
	ctx  mdContext
	verb string
}

// formatArgIndex names, per fmt function, which argument is the template.
// Errorf is absent on purpose: its result is an error message, which the error
// path renders with containment of its own.
var formatArgIndex = map[string]int{
	"Sprintf": 0,
	"Fprintf": 1,
	"Printf":  0,
	"Appendf": 1,
}

// cellArgFuncs are the toolutil builders with no template at all, because
// every argument they take is a table cell by construction. A gate that only
// parsed printf templates would not see them.
var cellArgFuncs = map[string]bool{
	"MarkdownTableRow":    true,
	"MarkdownTableHeader": true,
}

// fieldArgFuncs names, per toolutil card helper, which argument lands in a
// list item with nothing between it and the page.
//
// Only the rendered variant is here. Every other WriteMdField* helper escapes
// or renders its own value, so its call sites hold no hole, the same way
// WriteMdURL's do not. Declaring this one at the call site rather than letting
// the audit resolve the parameter inside toolutil is what keeps a finding — and
// the //gitlab:allow-unescaped directive that answers it — with the package
// that wrote the value rather than with the helper every package shares.
var fieldArgFuncs = map[string]int{
	"WriteMdFieldRendered": 2,
}

// fieldSink splits a card-field call whose value argument lands in a list item.
func fieldSink(pkg *packages.Package, call *ast.CallExpr, name string) (sink, bool) {
	index, known := fieldArgFuncs[name]
	if !known || index >= len(call.Args) {
		return sink{}, false
	}
	return sink{
		pkg:    pkg,
		call:   call,
		callee: name,
		holes:  []sinkHole{{expr: call.Args[index], ctx: ctxListItem, verb: "field"}},
	}, true
}

// collectSinks finds every call in the loaded packages that writes Markdown
// with a runtime value in it.
//
// A WriteString of a constant carries no runtime value, and one of a
// concatenation builds its pieces with Sprintf in this codebase, which is
// itself a sink. A WriteString of a value is a sink only inside a fenced code
// block, which is the one construct a whole line of prose can break out of;
// collectFences is what knows where those are.
func collectSinks(prog *program) []sink {
	fences := collectFences(prog)
	var sinks []sink
	for _, pkg := range prog.order {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if s, isSink := sinkOf(pkg, call, fences); isSink {
					sinks = append(sinks, s)
				}
				return true
			})
		}
	}
	return append(sinks, fences.writes...)
}

// sinkOf recognizes a Markdown-writing call and splits it into its holes.
func sinkOf(pkg *packages.Package, call *ast.CallExpr, fences *fenceIndex) (sink, bool) {
	callee := calleeOf(pkg, call)
	if callee == nil || callee.Pkg() == nil {
		return sink{}, false
	}
	switch callee.Pkg().Path() {
	case "fmt":
		return formatSink(pkg, call, callee.Name(), fences)
	case toolutilPath:
		if s, isCell := cellSink(pkg, call, callee.Name()); isCell {
			return s, true
		}
		return fieldSink(pkg, call, callee.Name())
	default:
		return sink{}, false
	}
}

// formatSink splits an fmt formatting call whose template is a constant.
//
// The template must be constant for the audit to know where a value lands, and
// the type checker resolves a named constant as readily as a literal, which
// matters because several formatters pass a shared template constant rather
// than writing the string at the call.
// A hole inside a fenced code block is judged as one, whatever the line it sits
// on would otherwise say: inside a block a pipe is text and a '#' is text, and
// the only thing the value can do is end the block, so the fence is the
// containment that has to hold.
func formatSink(pkg *packages.Package, call *ast.CallExpr, name string, fences *fenceIndex) (sink, bool) {
	index, known := formatArgIndex[name]
	if !known || index >= len(call.Args) {
		return sink{}, false
	}
	tv, ok := pkg.TypesInfo.Types[call.Args[index]]
	if !ok || tv.Value == nil {
		return sink{}, false
	}
	template := constantText(tv)
	if template == "" {
		return sink{}, false
	}
	args := call.Args[index+1:]
	holes := parseVerbs(template)
	fenced := fenceHoles(template, fences.cursorFor(call), holes)
	s := sink{pkg: pkg, call: call, callee: name}
	for i, h := range holes {
		if !stringishVerbs[h.verb] || h.arg >= len(args) {
			continue
		}
		ctx := contextAt(template, h.offset)
		if fenced[i] {
			ctx = ctxFence
		}
		s.holes = append(s.holes, sinkHole{
			expr: args[h.arg],
			ctx:  ctx,
			verb: "%" + string(h.verb),
		})
	}
	return s, len(s.holes) > 0
}

// cellSink splits a call whose every argument is a table cell.
//
// A slice spread with '...' is one hole holding the whole slice, since which
// element carries what is not knowable from the call. Classifying the slice
// answers the same question for every cell at once when the slice is built
// where the audit can see it, and is reported unresolved when it is not.
func cellSink(pkg *packages.Package, call *ast.CallExpr, name string) (sink, bool) {
	if !cellArgFuncs[name] || len(call.Args) == 0 {
		return sink{}, false
	}
	s := sink{pkg: pkg, call: call, callee: name}
	verb := "cell"
	if call.Ellipsis.IsValid() {
		verb = "cells..."
	}
	for _, arg := range call.Args {
		s.holes = append(s.holes, sinkHole{expr: arg, ctx: ctxCell, verb: verb})
	}
	return s, true
}

// enclosingFunc names the function a position sits in, so a finding points at
// the formatter rather than only at a line.
func enclosingFunc(pkg *packages.Package, pos token.Pos) string {
	for _, file := range pkg.Syntax {
		if pos < file.Pos() || pos > file.End() {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && pos >= fn.Pos() && pos <= fn.End() {
				return fn.Name.Name
			}
		}
	}
	return ""
}
