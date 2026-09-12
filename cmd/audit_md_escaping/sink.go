package main

import (
	"go/ast"
	"go/token"
	"go/types"

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
//
// A card-shaped line carries no value of its own: its expr is the constant
// that holds the line, for the position, and text is the line itself, which
// is what the finding names.
type sinkHole struct {
	expr ast.Expr
	ctx  mdContext
	verb string
	text string
}

// escapable reports whether the escaping verdict applies to this hole. A
// boolean verb renders "true" or "false" and nothing else, so it can change
// no construct, and a card-shaped line is a shape rather than a value.
func (h sinkHole) escapable() bool {
	return h.ctx != ctxCard && h.verb != "%t"
}

// verbSpread is the verb a hole carries when the call spreads a whole slice
// into the sink rather than naming its cells one by one. The elements are then
// values the caller assembled elsewhere, so the hole names the slice and no
// expression this pass can read.
const verbSpread = "cells..."

// rawJudged reports whether the second verdict, on what the value is rather
// than where it lands, applies to this hole: a formatted verb or a cell, but
// not a card line, not a whole slice spread into a row, and not a value
// inside a fence, where "true" and a Go-formatted instant are exactly what a
// JSON body says.
func (h sinkHole) rawJudged() bool {
	return h.ctx != ctxCard && h.ctx != ctxFence && h.verb != verbSpread
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

// printFuncs are the fmt functions with no template, whose constant operands
// are text on the page as they are.
var printFuncs = map[string]bool{
	"Fprint":   true,
	"Fprintln": true,
	"Sprint":   true,
	"Sprintln": true,
	"Print":    true,
	"Println":  true,
}

// cellArgFuncs are the toolutil builders with no template at all, because
// every argument they take is a table cell by construction. A gate that only
// parsed printf templates would not see them.
var cellArgFuncs = map[string]bool{
	"MarkdownTableRow":    true,
	"MarkdownTableHeader": true,
}

// cardCallSinks are the Card methods whose argument the caller renders, which
// is why each is judged at the call site: Markdown takes a value the formatter
// composed from escaped parts and writes it into a list item as given, and a
// table Row takes cells the way MarkdownTableRow does. Every other Card method
// escapes what it is given, so a raw value passed to it reaches no construct.
var cardCallSinks = map[string]struct {
	ctx  mdContext
	from int
	verb string
}{
	"toolutil.Card.Markdown": {ctx: ctxListItem, from: 1, verb: "item"},
	"toolutil.CardTable.Row": {ctx: ctxCell, from: 0, verb: "cell"},
}

// collectSinks finds every call in the loaded packages that writes Markdown
// with a runtime value in it, or writes a card row by hand.
//
// A WriteString of a constant carries no runtime value, and one of a
// concatenation builds its pieces with Sprintf in this codebase, which is
// itself a sink. A WriteString of a value is a sink only inside a fenced code
// block, which is the one construct a whole line of prose can break out of;
// collectFences is what knows where those are. A WriteString of a constant is
// still read for the card shape, since a hand-written row is constant text.
func collectSinks(prog *program) []sink {
	fences := collectFences(prog)
	var sinks []sink
	for _, pkg := range prog.order {
		for _, file := range pkg.Syntax {
			in := sinkFile{pkg: pkg, fences: fences, cards: cardScoped(pkg, prog.position(file.Pos()).Filename)}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if s, isSink := in.sinkOf(call); isSink {
					sinks = append(sinks, s)
				}
				return true
			})
		}
	}
	return append(sinks, fences.writes...)
}

// sinkFile is what the recognition of a sink needs to know about the file the
// call is in: the package for its types, the fence index for the cursor a
// write inherits, and whether the card rule reads this file at all.
type sinkFile struct {
	pkg    *packages.Package
	fences *fenceIndex
	cards  bool
}

// sinkOf recognizes a Markdown-writing call and splits it into its holes.
func (f sinkFile) sinkOf(call *ast.CallExpr) (sink, bool) {
	callee := calleeOf(f.pkg, call)
	if callee == nil || callee.Pkg() == nil {
		return sink{}, false
	}
	switch callee.Pkg().Path() {
	case "fmt":
		if printFuncs[callee.Name()] {
			return f.constantSink(call, callee.Name(), call.Args)
		}
		return f.formatSink(call, callee.Name())
	case toolutilPath:
		if callee.Signature().Recv() != nil {
			return f.cardCallSink(call, callee)
		}
		return f.cellSink(call, callee.Name())
	default:
		if builderWriters[callee.Name()] && builderPackages[callee.Pkg().Path()] {
			return f.constantSink(call, callee.Name(), call.Args)
		}
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
func (f sinkFile) formatSink(call *ast.CallExpr, name string) (sink, bool) {
	index, known := formatArgIndex[name]
	if !known || index >= len(call.Args) {
		return sink{}, false
	}
	tv, ok := f.pkg.TypesInfo.Types[call.Args[index]]
	if !ok || tv.Value == nil {
		return sink{}, false
	}
	template := constantText(tv)
	if template == "" {
		return sink{}, false
	}
	args := call.Args[index+1:]
	holes := parseVerbs(template)
	fenced := fenceHoles(template, f.fences.cursorFor(call), holes)
	s := sink{pkg: f.pkg, call: call, callee: name}
	for i, h := range holes {
		if !judgedVerbs[h.verb] || h.arg >= len(args) {
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
	s.holes = append(s.holes, f.cardHoles(call.Args[index], template)...)
	return s, len(s.holes) > 0
}

// constantSink reads the card shape out of a call with no template whose
// operands are constants: a builder's own write, or an fmt print call. The
// values such a call writes are judged by the fence pass, which is the one
// construct they can change.
func (f sinkFile) constantSink(call *ast.CallExpr, name string, args []ast.Expr) (sink, bool) {
	s := sink{pkg: f.pkg, call: call, callee: name}
	for _, arg := range args {
		tv := f.pkg.TypesInfo.Types[arg]
		if tv.Value == nil {
			continue
		}
		s.holes = append(s.holes, f.cardHoles(arg, constantText(tv))...)
	}
	return s, len(s.holes) > 0
}

// cardHoles turns the card-shaped lines of one constant into holes, when the
// card rule reads this file.
func (f sinkFile) cardHoles(holder ast.Expr, text string) []sinkHole {
	if !f.cards {
		return nil
	}
	var holes []sinkHole
	for _, row := range cardRows(text) {
		holes = append(holes, sinkHole{expr: holder, ctx: ctxCard, verb: row.verb, text: row.text})
	}
	return holes
}

// cellSink splits a call whose every argument is a table cell.
//
// A slice spread with '...' is one hole holding the whole slice, since which
// element carries what is not knowable from the call. Classifying the slice
// answers the same question for every cell at once when the slice is built
// where the audit can see it, and is reported unresolved when it is not.
//
// The same call is read for the card shape: a header of "Field" and "Value"
// opens a field table, and a two-cell row whose first cell is a constant is a
// labeled field, each of which is a card row written by hand.
func (f sinkFile) cellSink(call *ast.CallExpr, name string) (sink, bool) {
	if !cellArgFuncs[name] || len(call.Args) == 0 {
		return sink{}, false
	}
	s := sink{pkg: f.pkg, call: call, callee: name}
	verb := "cell"
	if call.Ellipsis.IsValid() {
		verb = verbSpread
	}
	for _, arg := range call.Args {
		s.holes = append(s.holes, sinkHole{expr: arg, ctx: ctxCell, verb: verb})
	}
	if row, ok := f.cardCells(call, name); ok {
		s.holes = append(s.holes, row)
	}
	return s, true
}

// cardCells reads the card shape out of a cell builder's constant arguments.
func (f sinkFile) cardCells(call *ast.CallExpr, name string) (sinkHole, bool) {
	if !f.cards || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return sinkHole{}, false
	}
	first := constantText(f.pkg.TypesInfo.Types[call.Args[0]])
	if first == "" {
		return sinkHole{}, false
	}
	text := types.ExprString(call)
	switch name {
	case "MarkdownTableHeader":
		second := constantText(f.pkg.TypesInfo.Types[call.Args[1]])
		if cardHeaderCells([]string{first, second}) {
			return sinkHole{expr: call.Args[0], ctx: ctxCard, verb: "header", text: text}, true
		}
	case "MarkdownTableRow":
		if f.pkg.TypesInfo.Types[call.Args[1]].Value == nil {
			return sinkHole{expr: call.Args[0], ctx: ctxCard, verb: "row", text: text}, true
		}
	}
	return sinkHole{}, false
}

// cardCallSink splits a call of a Card method the caller renders the value
// for, each argument landing in the construct the method writes.
func (f sinkFile) cardCallSink(call *ast.CallExpr, callee *types.Func) (sink, bool) {
	spec, ok := cardCallSinks[qualifiedName(callee)]
	if !ok || spec.from >= len(call.Args) {
		return sink{}, false
	}
	s := sink{pkg: f.pkg, call: call, callee: callee.Name()}
	verb := spec.verb
	if call.Ellipsis.IsValid() {
		verb = verbSpread
	}
	for _, arg := range call.Args[spec.from:] {
		s.holes = append(s.holes, sinkHole{expr: arg, ctx: spec.ctx, verb: verb})
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
