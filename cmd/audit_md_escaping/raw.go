package main

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// rawKind is what the second verdict found a value to be: a flag or an
// instant printed without the helper that renders it the way the rest of the
// tree does.
type rawKind int

const (
	// rawNone means the value is neither, or is rendered through its helper.
	rawNone rawKind = iota
	// rawBool is a flag that reaches the page as "true", "false" or a word a
	// package-local helper chose, where BoolEmoji and Card.Bool write the
	// house glyph.
	rawBool
	// rawTime is an instant that reaches the page as Go's default rendering
	// of a time.Time, as a layout a formatter chose, or as the RFC 3339 text
	// GitLab sent, where FormatTime and Card.Time write the display form.
	rawTime
)

// wants names the helper a raw value should go through.
func (k rawKind) wants() string {
	switch k {
	case rawBool:
		return "toolutil.BoolEmoji or Card.Bool"
	case rawTime:
		return "toolutil.FormatTime or Card.Time"
	default:
		return ""
	}
}

// boolWords are the spellings a hand-rolled flag helper returns, so that a
// helper whose every return is one of them is read as a flag renderer.
var boolWords = map[string]bool{
	"true": true, "false": true, "True": true, "False": true,
	"yes": true, "no": true, "Yes": true, "No": true, "YES": true, "NO": true,
}

// rawVerdict answers what a hole's value is, when it is a flag or an instant
// rendered without its helper.
//
// The question is about the value and not about where it lands, so the walk
// is short: the verb, the static type, the call that produced it, and the
// field it was read from, through the escapers and transforms the escaping
// verdict already sees through. Anything deeper is the escaping verdict's
// concern, and a value this cannot name is simply not a finding here.
func (c *classifier) rawVerdict(pkg *packages.Package, h sinkHole) (kind rawKind, explanation string) {
	if h.verb == "%t" {
		return rawBool, "a flag printed as true or false by %t"
	}
	expr := unwrapRendering(pkg, h.expr)
	tv := pkg.TypesInfo.Types[expr]
	if tv.Value != nil {
		return rawNone, ""
	}
	if byType, why := rawByType(tv.Type); byType != rawNone {
		return byType, why
	}
	switch typed := expr.(type) {
	case *ast.CallExpr:
		return c.rawCall(pkg, typed)
	case *ast.SelectorExpr:
		return rawSelector(pkg, typed)
	default:
		return rawNone, ""
	}
}

// unwrapRendering steps through the calls that change a value's text without
// changing what it is: an escaper, a strings transform, a conversion, and the
// dereference of a pointer field. A timestamp escaped for a cell is still a
// timestamp printed as it arrived. Each step moves to an operand of the
// expression it holds, so the walk ends with the source and needs no bound.
func unwrapRendering(pkg *packages.Package, expr ast.Expr) ast.Expr {
	for {
		expr = ast.Unparen(expr)
		if star, ok := expr.(*ast.StarExpr); ok {
			expr = star.X
			continue
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return expr
		}
		if tv, isType := pkg.TypesInfo.Types[call.Fun]; isType && tv.IsType() {
			expr = call.Args[0]
			continue
		}
		callee := calleeOf(pkg, call)
		if callee == nil {
			return expr
		}
		if isEscaper(callee) {
			expr = call.Args[0]
			continue
		}
		if carried, carries := passThroughArgs[qualifiedName(callee)]; carries && len(carried) == 1 && carried[0] < len(call.Args) {
			expr = call.Args[carried[0]]
			continue
		}
		return expr
	}
}

// rawByType answers from the static type alone: a boolean under a textual
// verb renders "true" or "false", and a time.Time renders in Go's default
// form, whatever the formatter meant.
func rawByType(t types.Type) (kind rawKind, explanation string) {
	if t == nil {
		return rawNone, ""
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	if named, ok := t.(*types.Named); ok {
		if obj := named.Obj(); obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "time" && obj.Name() == "Time" {
			return rawTime, "a time.Time printed in Go's default form"
		}
		t = named.Underlying()
	}
	if basic, ok := t.(*types.Basic); ok && basic.Info()&types.IsBoolean != 0 {
		return rawBool, "a boolean printed as true or false"
	}
	return rawNone, ""
}

// rawCall answers for a call: the standard-library spellings of a flag and of
// an instant, and a declared helper whose every return is a flag word.
func (c *classifier) rawCall(pkg *packages.Package, call *ast.CallExpr) (kind rawKind, explanation string) {
	callee := calleeOf(pkg, call)
	if callee == nil {
		return rawNone, ""
	}
	switch qualifiedName(callee) {
	case "strconv.FormatBool":
		return rawBool, "a flag spelled by strconv.FormatBool"
	case "time.Time.Format":
		return rawTime, "a time formatted by hand rather than through the display helper"
	}
	if decl, ok := c.prog.decls[callee]; ok && returnsBoolWords(decl) {
		return rawBool, "a helper that spells a flag as a word, " + callee.Name()
	}
	return rawNone, ""
}

// returnsBoolWords reports whether a declared function returns nothing but
// the words a flag is spelled with, which is the shape of a package-local
// yes-or-no helper. The function reached a hole as one value, so it has one
// result; a bare return through a named result is not read as a word, since
// what it returns is whatever was assigned.
func returnsBoolWords(decl *funcDecl) bool {
	found := false
	words := true
	ast.Inspect(decl.decl.Body, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		found = true
		tv := decl.pkg.TypesInfo.Types[ret.Results[0]]
		if tv.Value == nil || tv.Value.Kind() != constant.String || !boolWords[constant.StringVal(tv.Value)] {
			words = false
		}
		return true
	})
	return found && words
}

// rawSelector answers for a field read: a string field named like an instant
// (CreatedAt, ExpiresAt, DueDate) holds the RFC 3339 text GitLab sent, and
// printing it as it arrived is the one shape the tree excuses most often for
// escaping while it is still wrong for display.
func rawSelector(pkg *packages.Package, sel *ast.SelectorExpr) (kind rawKind, explanation string) {
	name := sel.Sel.Name
	if !strings.HasSuffix(name, "At") && !strings.HasSuffix(name, "Date") {
		return rawNone, ""
	}
	tv := pkg.TypesInfo.Types[sel]
	if tv.Type == nil {
		return rawNone, ""
	}
	t := types.Unalias(tv.Type)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	if basic, ok := t.Underlying().(*types.Basic); ok && basic.Info()&types.IsString != 0 {
		return rawTime, "a timestamp GitLab sent, printed as it arrived"
	}
	return rawNone, ""
}
