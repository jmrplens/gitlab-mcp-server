package derive

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"unicode/utf8"

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

// maxFoldDepth bounds how many folds deep one path follows names; real paths
// fold in two or three steps, and the bound only keeps a cycle from hanging a
// run. Each fold counts once, on entry, whatever it was reached through.
const maxFoldDepth = 16

// maxSpellings bounds how many spellings one expression folds to, so a path
// built from several branched pieces cannot multiply without end. What the
// bound leaves out is folded to [unknownPiece] rather than dropped.
const maxSpellings = 16

// fold evaluates a string expression to every spelling it can take.
//
// Constants fold to themselves; a parameter to what the caller passed, through
// as many callers as the expansion bound; a local to every value assigned to
// it; a concatenation and a fmt.Sprintf to the combinations of their pieces;
// a string helper of the loaded source to what it returns. A field, an index,
// a number and a call that formats a value (url.PathEscape, strconv.Itoa) fold
// to the placeholder, since what they carry is an identifier. A string nothing
// static names folds to [unknownPiece], and so does a local any compound
// assignment (+=) writes, whose value depends on which statements ran before
// it. depth is how many folds enclose this one.
func (d *deriver) fold(expr ast.Expr, at *frame, depth int) []string {
	depth++
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
		return combine(d.fold(typed.X, at, depth), d.fold(typed.Y, at, depth))
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
		return d.fold(bound.expr, bound.at, depth)
	}
	assignments := assignmentsTo(at.pkg, at.body, variable)
	if slices.Contains(assignments, nil) {
		return []string{unknownPiece}
	}
	var found []string
	for _, assigned := range assignments {
		found = append(found, d.fold(assigned, at, depth)...)
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
// returns with its parameters bound. A conversion always has one operand and
// fmt.Sprintf always a format, which the type checker holds a call to.
func (d *deriver) foldCall(call *ast.CallExpr, at *frame, depth int) []string {
	if at.pkg.TypesInfo.Types[call.Fun].IsType() {
		return d.fold(call.Args[0], at, depth)
	}
	callee := calleeOf(at.pkg, call)
	if callee == nil {
		return []string{unknownPiece}
	}
	if callee.Pkg() != nil && callee.Pkg().Path() == "fmt" && callee.Name() == "Sprintf" {
		var found []string
		for _, format := range d.fold(call.Args[0], at, depth) {
			found = append(found, d.sprintf(format, call.Args[1:], at, depth)...)
		}
		return dedupe(found)
	}
	// The index holds a declared function with its body and a stand-in for a
	// literal or an initializer, which no call names, so a function of ours a
	// call names has a declaration.
	fn, ours := d.prog.Function(callee.Origin())
	if !ours || !returnsString(callee) {
		return []string{placeholder}
	}
	decl := fn.Decl()
	inner := &frame{pkg: fn.Package(), body: decl.Body, env: bind(fn.Package(), decl.Type.Params, call, at)}
	var found []string
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		if ret, isReturn := n.(*ast.ReturnStmt); isReturn && len(ret.Results) > 0 {
			found = append(found, d.fold(ret.Results[0], inner, depth)...)
		}
		return true
	})
	if len(found) == 0 {
		return []string{unknownPiece}
	}
	return dedupe(found)
}

// sprintf fills a format's verbs with what each argument folds to: the text
// up to a verb as written, a doubled percent sign as one, a verb's flags,
// width and precision dropped with it, and a verb with no argument left as a
// piece nothing static names.
func (d *deriver) sprintf(format string, args []ast.Expr, at *frame, depth int) []string {
	spellings := []string{""}
	next := 0
	for {
		literal, rest, found := strings.Cut(format, "%")
		spellings = combine(spellings, []string{literal})
		if !found {
			return spellings
		}
		if after, escaped := strings.CutPrefix(rest, "%"); escaped {
			spellings = combine(spellings, []string{"%"})
			format = after
			continue
		}
		rest = strings.TrimLeft(rest, "+-# 0123456789.")
		_, verb := utf8.DecodeRuneInString(rest)
		format = rest[verb:]
		piece := []string{unknownPiece}
		if next < len(args) {
			piece = d.fold(args[next], at, depth)
		}
		next++
		spellings = combine(spellings, piece)
	}
}

// combine concatenates every spelling of a with every spelling of b, up to
// [maxSpellings]. The spellings past the bound fold to one [unknownPiece]
// after the ones kept, so a path that multiplies further is reported as one
// nothing static names rather than read as the routes kept, which would leave
// the rest out of the derivation without a word.
func combine(a, b []string) []string {
	var out []string
	for _, left := range a {
		for _, right := range b {
			if len(out) == maxSpellings {
				return append(out, unknownPiece)
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
// caller's frame. A call with fewer arguments, a variadic one passing none,
// binds what it passes. A function's parameter list is never nil, and every
// name in it defines a variable.
func bind(pkg *packages.Package, params *ast.FieldList, call *ast.CallExpr, at *frame) map[*types.Var]binding {
	env := map[*types.Var]binding{}
	if call == nil {
		return env
	}
	index := 0
	for _, field := range params.List {
		for _, name := range field.Names {
			if index < len(call.Args) {
				variable, _ := pkg.TypesInfo.Defs[name].(*types.Var)
				env[variable] = binding{expr: call.Args[index], at: at}
			}
			index++
		}
	}
	return env
}

// assignmentsTo lists every expression a body assigns to a variable, through
// := and =, and a var declaration with values. A compound assignment (+=) is
// listed as a nil expression, which marks the variable as one no fold reads.
func assignmentsTo(pkg *packages.Package, body ast.Node, variable *types.Var) []ast.Expr {
	if body == nil {
		return nil
	}
	assigns := func(ident *ast.Ident) bool {
		return pkg.TypesInfo.ObjectOf(ident) == variable
	}
	var found []ast.Expr
	ast.Inspect(body, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.AssignStmt:
			found = append(found, assignedBy(typed, assigns)...)
		case *ast.ValueSpec:
			found = append(found, declaredBy(typed, assigns)...)
		}
		return true
	})
	return found
}

// assignedBy lists what one assignment statement writes to the variable
// assigns recognizes: the value := or = gives it, and a nil expression for a
// compound assignment. A statement assigning a call's several results names
// none.
func assignedBy(stmt *ast.AssignStmt, assigns func(*ast.Ident) bool) []ast.Expr {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return nil
	}
	var found []ast.Expr
	for i, lhs := range stmt.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || !assigns(ident) {
			continue
		}
		if stmt.Tok == token.ASSIGN || stmt.Tok == token.DEFINE {
			found = append(found, stmt.Rhs[i])
		} else {
			found = append(found, nil)
		}
	}
	return found
}

// declaredBy lists the values a var declaration gives the variable assigns
// recognizes. A declaration taking a call's several results names none.
func declaredBy(spec *ast.ValueSpec, assigns func(*ast.Ident) bool) []ast.Expr {
	if len(spec.Names) != len(spec.Values) {
		return nil
	}
	var found []ast.Expr
	for i, name := range spec.Names {
		if assigns(name) {
			found = append(found, spec.Values[i])
		}
	}
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

// returnsString reports whether a function's first result is a string. A
// call that is folded is a value, so the function it calls has a result.
func returnsString(fn *types.Func) bool {
	return isString(fn.Signature().Results().At(0).Type())
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
