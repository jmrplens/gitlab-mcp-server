package main

import (
	"go/ast"
	"go/types"
	"maps"
	"strings"

	"golang.org/x/tools/go/packages"
)

// minFenceLength is the shortest backtick run that opens a fenced code block.
// A run of one or two backticks opens an inline code span instead, which ends
// at the end of its own line and so cannot swallow the rest of the document;
// those runs are deliberately outside this rule, since the value between them
// is already judged by the cell, list-item or heading it sits on.
const minFenceLength = 3

// maxFenceIndent is how far a fence may be indented and still open a block.
// CommonMark allows three spaces; a fourth makes the line indented code.
const maxFenceIndent = 3

// fenceCursor is what the audit knows about the Markdown one destination is
// being assembled into, at one point in a function body: whether the document
// is inside a fenced code block, how long the fence that opened it is, and
// whether the next byte written begins a line, which is the only place a fence
// marker counts.
//
// It is a value rather than a pointer so that saving and restoring it across a
// nested block is a copy.
type fenceCursor struct {
	open        bool
	length      int
	atLineStart bool
}

// newFenceCursor is the state a destination starts in: an empty document, at
// the start of its first line, with no fence open.
func newFenceCursor() fenceCursor {
	return fenceCursor{atLineStart: true}
}

// inside reports whether a value written now lands inside a fenced code block.
//
// The info string of the fence that opens one is inside it by this measure,
// because the block is open from the moment the marker is read, and that is the
// answer the rule wants: a newline in an info string ends the fence line, and
// everything the value carries after it is a line of the document rather than
// of the block.
func (c fenceCursor) inside() bool {
	return c.open
}

// writeText advances the cursor over literal text a formatter writes.
//
// Only the start of a line can open or close a fence, so the scan is by line:
// a run of backticks at the start of a line opens a block when none is open,
// and closes the open one when it is at least as long as the fence that opened
// it. The closing rule is deliberately more permissive than CommonMark, which
// also demands the line hold nothing else: a cursor wrongly left open would
// report every later value in the function, and a cursor wrongly closed reports
// nothing, so the error is taken in the direction that invents no finding.
func (c fenceCursor) writeText(text string) fenceCursor {
	for text != "" {
		line, rest, hasNewline := strings.Cut(text, "\n")
		if c.atLineStart {
			c = c.mark(line)
		}
		if !hasNewline {
			// The text ran out mid-line, and a line with nothing on it cannot
			// be reached here: the loop only runs on text that is not empty.
			c.atLineStart = false
			return c
		}
		c.atLineStart = true
		text = rest
	}
	return c
}

// writeValue advances the cursor over a value the audit cannot read.
//
// The document's shape is taken to be the one the server wrote, so a value is
// text that opens and closes nothing: assuming otherwise would mean assuming
// the very breakout this rule exists to prevent, and every hole after the first
// would be judged against a document that never renders.
func (c fenceCursor) writeValue() fenceCursor {
	c.atLineStart = false
	return c
}

// closed is the cursor for a destination the audit has stopped being able to
// follow, because something it does not read wrote into it. Closed rather than
// unchanged: a stale open fence would condemn every value written after it.
func (c fenceCursor) closed() fenceCursor {
	return fenceCursor{atLineStart: c.atLineStart}
}

// mark applies the fence marker a line opens with, if it opens with one.
func (c fenceCursor) mark(line string) fenceCursor {
	run, ok := fenceMarker(line)
	if !ok {
		return c
	}
	if c.open {
		if run >= c.length {
			return fenceCursor{atLineStart: c.atLineStart}
		}
		return c
	}
	c.open = true
	c.length = run
	return c
}

// fenceMarker reads the backtick run a line opens with, after the indentation
// CommonMark allows in front of one.
func fenceMarker(line string) (run int, ok bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > maxFenceIndent {
		return 0, false
	}
	for indent+run < len(line) && line[indent+run] == '`' {
		run++
	}
	if run < minFenceLength {
		return 0, false
	}
	return run, true
}

// fenceIndex is what one pass over the source knows about fenced code blocks:
// the cursor every Markdown-writing call starts from, and the values written
// into a destination while a fence was open.
type fenceIndex struct {
	// at is the cursor a call inherits from the writes before it. A call absent
	// from the map is one no pass reached, which starts from a fresh cursor.
	at map[*ast.CallExpr]fenceCursor
	// writes holds the sinks for values written into an open fence by a call
	// with no template of its own, which nothing else in this audit collects.
	writes []sink
}

// cursorFor returns the cursor a call starts from.
func (f *fenceIndex) cursorFor(call *ast.CallExpr) fenceCursor {
	if f == nil {
		return newFenceCursor()
	}
	if cursor, ok := f.at[call]; ok {
		return cursor
	}
	return newFenceCursor()
}

// collectFences walks every function body in the program and records where the
// Markdown being assembled is inside a fenced code block.
//
// The walk is over statements in source order, and a nested block inherits the
// cursor it is entered with and hands nothing back: a chart whose fence is
// written at the top of a function and whose rows are written in a loop is
// judged, while a fence opened in one branch of an if and closed in another
// leaves the outer cursor closed and reports nothing. That is the direction a
// gate has to err in, and it is what "adjacent to a fence" means here.
func collectFences(prog *program) *fenceIndex {
	index := &fenceIndex{at: map[*ast.CallExpr]fenceCursor{}}
	for _, pkg := range prog.order {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				walk := &fenceWalk{pkg: pkg, index: index, cursors: map[types.Object]fenceCursor{}}
				walk.body(fn.Body)
			}
		}
	}
	return index
}

// fenceWalk is one pass over one function body, carrying the cursor of every
// destination written to in it.
type fenceWalk struct {
	pkg     *packages.Package
	index   *fenceIndex
	cursors map[types.Object]fenceCursor
}

// body walks one function body in source order.
//
// The saved stack holds one entry per visited node so that the restore on the
// way out pairs with the save on the way in: a non-nil entry is the cursors a
// block was entered with, and a nil entry is a node that opened no scope.
func (w *fenceWalk) body(block *ast.BlockStmt) {
	var saved []map[types.Object]fenceCursor
	ast.Inspect(block, func(node ast.Node) bool {
		if node == nil {
			last := len(saved) - 1
			if outer := saved[last]; outer != nil {
				w.cursors = outer
			}
			saved = saved[:last]
			return true
		}
		var outer map[types.Object]fenceCursor
		switch typed := node.(type) {
		case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
			outer = w.snapshot()
		case *ast.CallExpr:
			w.call(typed)
		}
		saved = append(saved, outer)
		return true
	})
}

// snapshot copies the cursors, so a nested block can be entered with them and
// left without its own writes escaping.
func (w *fenceWalk) snapshot() map[types.Object]fenceCursor {
	copied := make(map[types.Object]fenceCursor, len(w.cursors))
	maps.Copy(copied, w.cursors)
	return copied
}

// call records the cursor one call starts from and advances it by what the call
// writes.
func (w *fenceWalk) call(call *ast.CallExpr) {
	callee := calleeOf(w.pkg, call)
	if callee == nil || callee.Pkg() == nil {
		w.stopFollowing(call)
		return
	}
	if callee.Pkg().Path() == "fmt" {
		w.fmtCall(call, callee.Name())
		return
	}
	if builderWriters[callee.Name()] && builderPackages[callee.Pkg().Path()] {
		w.builderCall(call, callee.Name())
		return
	}
	w.stopFollowing(call)
}

// builderPackages are the packages whose writer types this repository's
// formatters accumulate Markdown in.
var builderPackages = map[string]bool{"strings": true, "bytes": true}

// builderWriters are the methods of those types that put text on the page.
var builderWriters = map[string]bool{
	"WriteString": true,
	"Write":       true,
	"WriteByte":   true,
	"WriteRune":   true,
}

// fmtCall advances the destination of an fmt call that writes to one.
func (w *fenceWalk) fmtCall(call *ast.CallExpr, name string) {
	index, writes := formatArgIndex[name]
	if !writes {
		w.printCall(call, name)
		return
	}
	if index == 0 {
		// Sprintf and Printf write to no destination this pass follows: the
		// first has no writer at all and the second writes to standard output.
		return
	}
	// Fprintf and Appendf take the destination and the template before their
	// operands, so a call that type-checked has both arguments.
	dest := destinationOf(w.pkg, call.Args[0])
	if dest == nil {
		return
	}
	cursor := w.cursorOf(dest)
	w.index.at[call] = cursor
	template, constant := w.constantArg(call.Args[index])
	if !constant {
		w.cursors[dest] = cursor.closed()
		return
	}
	w.cursors[dest] = cursor.writeText(template)
}

// printCall advances the destination of an fmt call with no template: every
// operand is written as it is.
func (w *fenceWalk) printCall(call *ast.CallExpr, name string) {
	if name != "Fprint" && name != "Fprintln" {
		return
	}
	// Both take the destination first, so a call that type-checked has one.
	dest := destinationOf(w.pkg, call.Args[0])
	if dest == nil {
		return
	}
	cursor := w.cursorOf(dest)
	w.index.at[call] = cursor
	for _, arg := range call.Args[1:] {
		text, constant := w.constantArg(arg)
		if constant {
			cursor = cursor.writeText(text)
			continue
		}
		if cursor.inside() {
			w.record(call, name, arg)
		}
		cursor = cursor.writeValue()
	}
	if name == "Fprintln" {
		cursor = cursor.writeText("\n")
	}
	w.cursors[dest] = cursor
}

// builderCall advances a builder's own write, and records the value it writes
// when a fence is open around it.
func (w *fenceWalk) builderCall(call *ast.CallExpr, name string) {
	// Every method named here takes exactly one argument, which is the text.
	dest := destinationOf(w.pkg, receiverOf(call))
	if dest == nil {
		return
	}
	cursor := w.cursorOf(dest)
	w.index.at[call] = cursor
	if text, constant := w.constantArg(call.Args[0]); constant {
		w.cursors[dest] = cursor.writeText(text)
		return
	}
	if cursor.inside() {
		w.record(call, name, call.Args[0])
	}
	w.cursors[dest] = cursor.writeValue()
}

// record notes one value written into an open fenced block by a call that has
// no template for the rest of this audit to read.
func (w *fenceWalk) record(call *ast.CallExpr, callee string, arg ast.Expr) {
	w.index.writes = append(w.index.writes, sink{
		pkg:    w.pkg,
		call:   call,
		callee: callee,
		holes:  []sinkHole{{expr: arg, ctx: ctxFence, verb: "write"}},
	})
}

// stopFollowing gives up on every destination this call was handed.
//
// A helper that takes the builder writes text this pass cannot read, and it may
// well be the text that closes the fence. Keeping the cursor as it was would
// report every value written after the call, so the cursor is reset instead and
// the values that helper writes are simply not judged here.
func (w *fenceWalk) stopFollowing(call *ast.CallExpr) {
	for _, arg := range call.Args {
		dest := destinationOf(w.pkg, arg)
		if dest == nil {
			continue
		}
		// Only a destination this pass is already following can be given up on.
		// Any other variable handed to any other call is not a Markdown
		// document, and resetting it would be bookkeeping about nothing.
		if cursor, followed := w.cursors[dest]; followed {
			w.cursors[dest] = cursor.closed()
		}
	}
}

// cursorOf returns what is known about a destination so far.
func (w *fenceWalk) cursorOf(dest types.Object) fenceCursor {
	if cursor, ok := w.cursors[dest]; ok {
		return cursor
	}
	return newFenceCursor()
}

// constantArg reads an argument the type checker resolved to constant text,
// which is the only text this pass can read.
func (w *fenceWalk) constantArg(arg ast.Expr) (text string, constant bool) {
	// An expression the type checker recorded nothing for has no value either,
	// and the zero TypeAndValue answers that on its own.
	tv := w.pkg.TypesInfo.Types[arg]
	if tv.Value == nil {
		return "", false
	}
	return constantText(tv), true
}

// receiverOf returns the value a method is called on, or nil for a call that is
// not through a selector at all.
func receiverOf(call *ast.CallExpr) ast.Expr {
	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		return selector.X
	}
	return nil
}

// destinationOf names the variable a write goes to, through the address-of and
// the parentheses a call site puts around it. A nil expression names nothing,
// which is how a call with no receiver arrives here.
//
// Naming it by the object the type checker resolved rather than by the text of
// the expression is what keeps two builders in one function apart.
func destinationOf(pkg *packages.Package, expr ast.Expr) types.Object {
	for {
		switch typed := ast.Unparen(expr).(type) {
		case *ast.UnaryExpr:
			expr = typed.X
		case *ast.StarExpr:
			expr = typed.X
		case *ast.Ident:
			// A destination arrives as a use of the variable, never as its
			// declaration, and anything that is not a variable (a package name
			// in front of a selector, say) names no document.
			obj, isVar := pkg.TypesInfo.Uses[typed].(*types.Var)
			if !isVar {
				return nil
			}
			return obj
		default:
			return nil
		}
	}
}

// fenceHoles reports, for each hole of a template, whether it lands inside a
// fenced code block, given the cursor the write starts from.
//
// The text fed to the cursor between two holes begins at the previous verb
// rather than after it, because the verb's own characters are neither a newline
// nor a backtick and so move the cursor exactly as far as the value they stand
// for does.
func fenceHoles(template string, start fenceCursor, holes []hole) []bool {
	inside := make([]bool, len(holes))
	cursor := start
	pos := 0
	for i, h := range holes {
		cursor = cursor.writeText(template[pos:h.offset])
		inside[i] = cursor.inside()
		pos = h.offset
	}
	return inside
}
