package derive

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// frame is where an expression is evaluated: the package that types it, the
// body whose local assignments it reads, and the parameters of that body bound
// to what the caller passed.
type frame struct {
	pkg  *packages.Package
	body ast.Node
	env  map[*types.Var]binding
}

// binding is one parameter bound to the caller's argument, with the frame the
// argument is evaluated in.
type binding struct {
	expr ast.Expr
	at   *frame
}

// unknownPiece stands in a folded string for a piece nothing static names,
// which makes the whole path unreadable rather than wrong.
const unknownPiece = "\x00"

// placeholder stands for an identifier: a value a path carries that is not a
// literal segment of the route.
const placeholder = ":"

// maxFoldDepth bounds how far a fold follows names; real paths fold in two or
// three steps, and the bound only keeps a cycle from hanging a run.
const maxFoldDepth = 16

// maxSpellings bounds how many spellings one expression folds to, so a path
// built from several branched pieces cannot multiply without end.
const maxSpellings = 16

// fold evaluates a string expression to every spelling it can take.
//
// Constants fold to themselves; a parameter to what the caller passed, through
// as many callers as the expansion bound; a local to every value assigned to
// it; a concatenation and a fmt.Sprintf to the combinations of their pieces;
// a string helper of the loaded source to what it returns. A field, an index,
// a number and a call that formats a value (url.PathEscape, strconv.Itoa) fold
// to the placeholder, since what they carry is an identifier. A string nothing
// static names folds to [unknownPiece].
func (d *deriver) fold(expr ast.Expr, at *frame, depth int) []string {
	if depth > maxFoldDepth || expr == nil {
		return []string{unknownPiece}
	}
	expr = ast.Unparen(expr)
	if value := at.pkg.TypesInfo.Types[expr].Value; value != nil {
		if value.Kind() == constant.String {
			return []string{constant.StringVal(value)}
		}
		return []string{placeholder}
	}
	switch typed := expr.(type) {
	case *ast.Ident:
		return d.foldIdent(typed, at, depth)
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return []string{placeholder}
		}
		return combine(d.fold(typed.X, at, depth+1), d.fold(typed.Y, at, depth+1))
	case *ast.CallExpr:
		return d.foldCall(typed, at, depth)
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
		return []string{placeholder}
	}
	return []string{unknownPiece}
}

// foldIdent folds a name: a bound parameter, or a local through every
// assignment to it.
func (d *deriver) foldIdent(ident *ast.Ident, at *frame, depth int) []string {
	variable, ok := at.pkg.TypesInfo.Uses[ident].(*types.Var)
	if !ok {
		return []string{unknownPiece}
	}
	if bound, isBound := at.env[variable]; isBound {
		return d.fold(bound.expr, bound.at, depth+1)
	}
	var found []string
	for _, assigned := range assignmentsTo(at.pkg, at.body, variable) {
		found = append(found, d.fold(assigned, at, depth+1)...)
	}
	if len(found) > 0 {
		return dedupe(found)
	}
	if isString(variable.Type()) {
		return []string{unknownPiece}
	}
	return []string{placeholder}
}

// foldCall folds a call: a conversion to its operand, fmt.Sprintf to its
// format filled in, and a string helper of the loaded source to what it
// returns with its parameters bound.
func (d *deriver) foldCall(call *ast.CallExpr, at *frame, depth int) []string {
	if typeAndValue, ok := at.pkg.TypesInfo.Types[call.Fun]; ok && typeAndValue.IsType() && len(call.Args) == 1 {
		return d.fold(call.Args[0], at, depth+1)
	}
	callee := calleeOf(at.pkg, call)
	if callee == nil {
		return []string{unknownPiece}
	}
	if callee.Pkg() != nil && callee.Pkg().Path() == "fmt" && callee.Name() == "Sprintf" && len(call.Args) > 0 {
		var found []string
		for _, format := range d.fold(call.Args[0], at, depth+1) {
			found = append(found, d.sprintf(format, call.Args[1:], at, depth)...)
		}
		return dedupe(found)
	}
	fn, ours := d.prog.Function(callee.Origin())
	if !ours || fn.Decl() == nil || !returnsString(callee) {
		return []string{placeholder}
	}
	inner := &frame{pkg: fn.Package(), body: fn.Decl().Body, env: bind(fn.Package(), fn.Decl().Type.Params, call, at)}
	var found []string
	ast.Inspect(fn.Decl().Body, func(n ast.Node) bool {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		if ret, isReturn := n.(*ast.ReturnStmt); isReturn && len(ret.Results) > 0 {
			found = append(found, d.fold(ret.Results[0], inner, depth+1)...)
		}
		return true
	})
	if len(found) == 0 {
		return []string{unknownPiece}
	}
	return dedupe(found)
}

// sprintf fills a format's verbs with what each argument folds to.
func (d *deriver) sprintf(format string, args []ast.Expr, at *frame, depth int) []string {
	spellings := []string{""}
	next := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			spellings = combine(spellings, []string{format[i : i+1]})
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			spellings = combine(spellings, []string{"%"})
			i++
			continue
		}
		for i+1 < len(format) && strings.IndexByte("+-# 0123456789.", format[i+1]) >= 0 {
			i++
		}
		i++
		piece := []string{unknownPiece}
		if next < len(args) {
			piece = d.fold(args[next], at, depth+1)
		}
		next++
		spellings = combine(spellings, piece)
	}
	return spellings
}

// combine concatenates every spelling of a with every spelling of b, up to
// [maxSpellings].
func combine(a, b []string) []string {
	var out []string
	for _, left := range a {
		for _, right := range b {
			if len(out) == maxSpellings {
				return out
			}
			out = append(out, left+right)
		}
	}
	return out
}

// dedupe drops repeated spellings, keeping the first of each.
func dedupe(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

// bind binds a parameter list to a call's arguments, each evaluated in the
// caller's frame. A call with fewer arguments binds what it passes.
func bind(pkg *packages.Package, params *ast.FieldList, call *ast.CallExpr, at *frame) map[*types.Var]binding {
	env := map[*types.Var]binding{}
	if call == nil || params == nil {
		return env
	}
	index := 0
	for _, field := range params.List {
		for _, name := range field.Names {
			if index < len(call.Args) {
				if variable, ok := pkg.TypesInfo.Defs[name].(*types.Var); ok {
					env[variable] = binding{expr: call.Args[index], at: at}
				}
			}
			index++
		}
	}
	return env
}

// assignmentsTo lists every expression a body assigns to a variable, through
// := and =, and a var declaration with values.
func assignmentsTo(pkg *packages.Package, body ast.Node, variable *types.Var) []ast.Expr {
	if body == nil {
		return nil
	}
	var found []ast.Expr
	assigns := func(ident *ast.Ident) bool {
		return pkg.TypesInfo.Defs[ident] == variable || pkg.TypesInfo.Uses[ident] == variable
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.AssignStmt:
			if len(typed.Lhs) != len(typed.Rhs) {
				return true
			}
			for i, lhs := range typed.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && assigns(ident) {
					found = append(found, typed.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			if len(typed.Names) != len(typed.Values) {
				return true
			}
			for i, name := range typed.Names {
				if assigns(name) {
					found = append(found, typed.Values[i])
				}
			}
		}
		return true
	})
	return found
}

// calleeOf resolves the function a call calls, when it names one.
func calleeOf(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	var ident *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return nil
	}
	fn, _ := pkg.TypesInfo.Uses[ident].(*types.Func)
	return fn
}

// isString reports whether a type is a string.
func isString(typ types.Type) bool {
	basic, ok := typ.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

// returnsString reports whether a function's first result is a string.
func returnsString(fn *types.Func) bool {
	results := fn.Signature().Results()
	return results.Len() > 0 && isString(results.At(0).Type())
}

// normalizePath turns a folded request path into the route spelling client-go
// routes are compared in: no query, no API prefix, a leading slash, and every
// segment holding anything but a literal collapsed to the placeholder. It
// reports false for a path holding a piece nothing static names.
func normalizePath(raw string) (string, bool) {
	if strings.Contains(raw, unknownPiece) {
		return "", false
	}
	if cut := strings.IndexAny(raw, "?#"); cut >= 0 {
		raw = raw[:cut]
	}
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "/"), "api/v4/")
	segments := strings.Split(strings.Trim(raw, "/"), "/")
	for i, segment := range segments {
		if strings.Contains(segment, placeholder) {
			segments[i] = placeholder
		}
	}
	return "/" + strings.Join(segments, "/"), true
}
