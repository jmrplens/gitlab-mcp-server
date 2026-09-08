package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/goprogram"
)

// queryTypeName names the request type whose Do method sends a document. A
// call is a send when its first argument has a named type of this name,
// whichever package declares it: client-go's is gitlab.GraphQLQuery, and
// matching the name rather than the import path lets a fixture module stand in
// for client-go in the tests without fetching anything.
const queryTypeName = "GraphQLQuery"

// queryField is the field of that type that holds the document.
const queryField = "Query"

// sendMethod is the method that sends it.
const sendMethod = "Do"

// pairing is one document and the type it is decoded into: the two halves of
// one exchange with GitLab, which every other gate judges one at a time.
type pairing struct {
	// Package is the import path of the package that makes the call.
	Package string
	// Position is the Do call.
	Position token.Position
	// Name is the constant the document is declared as, or "" when the call
	// assembles it in place.
	Name string
	// Text is the folded document, with any shared fragment spliced in.
	Text string
	// Response is what the pointer handed to Do points at.
	Response types.Type
	// TypeArgs binds the type parameters of a generic wrapper to the types
	// the caller instantiated it with, so a field typed by a parameter is
	// judged as the type it will hold.
	TypeArgs map[*types.TypeParam]types.Type
	// Origin is where the document was handed to the wrapper, when the call
	// received it through a parameter rather than naming it itself. It is
	// the zero position for a call that names its own document.
	Origin token.Position
}

// Label names the document for a report line.
func (p pairing) Label() string {
	if p.Name == "" {
		return "an inline document"
	}
	return p.Name
}

// problem is a call this audit could not pair, or a document it could not
// find a call for. Either is a failure: a document nobody judges is exactly
// the shape the gate exists to refuse.
type problem struct {
	Package  string
	Position token.Position
	Message  string
}

// wrapper is a Do call whose document arrives through a parameter of the
// enclosing function. The pairing is completed at every call of that function,
// where the document is named.
type wrapper struct {
	fn *types.Func
	// param indexes the flattened parameter list.
	param int
	// field is the field of the struct parameter that holds the document, or
	// "" when the parameter is the document itself.
	field    string
	response types.Type
	pkg      *packages.Package
	position token.Position
	// typeArgs binds the type parameters met on the way out from the Do call:
	// a generic wrapper called by a generic wrapper binds the inner one's
	// parameters to the outer one's, and the outer one's are bound by its own
	// callers. Every binding is carried so the decoder can be read through
	// all of them.
	typeArgs map[*types.TypeParam]types.Type
}

// program is the loaded source, ready to be walked.
type program struct {
	fset *token.FileSet
	pkgs []*packages.Package
}

// loadProgram type-checks the packages patterns name under dir.
//
// The load itself belongs to [goprogram.Load], including the refusal of a
// package that did not type-check completely: such a package folds no
// constants and types no expression, so every call in it would go unseen,
// which is the failure this audit must not have. This audit passes no overlay
// because its fixtures are written to a module on disk rather than into the
// loader.
func loadProgram(dir string, patterns []string) (*program, error) {
	loaded, err := goprogram.Load(dir, patterns, nil)
	if err != nil {
		return nil, err
	}
	return &program{fset: loaded[0].Fset, pkgs: loaded}, nil
}

// pairings finds every send in the program and pairs each with the document it
// sends, following a document that arrives through a wrapper's parameter to
// the wrapper's callers.
func (p *program) pairings() ([]pairing, []problem) {
	var (
		pairings []pairing
		problems []problem
		wrappers []wrapper
	)
	p.eachCall(func(pkg *packages.Package, fn *ast.FuncDecl, call *ast.CallExpr) {
		if !isSend(pkg, call) {
			return
		}
		position := p.fset.Position(call.Pos())
		response, why := responseType(pkg, call.Args[1])
		if why != "" {
			problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: why})
			return
		}
		expr, why := documentExpr(pkg, fn, call.Args[0])
		if why != "" {
			problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: why})
			return
		}
		for _, found := range documentSources(pkg, fn, expr) {
			switch {
			case found.viaParam:
				declared, _ := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
				wrappers = append(wrappers, wrapper{
					fn:       declared,
					param:    found.param,
					field:    found.field,
					response: response,
					pkg:      pkg,
					position: position,
				})
			case found.text == "":
				problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: found.why})
			default:
				pairings = append(pairings, pairing{
					Package:  pkg.PkgPath,
					Position: position,
					Name:     found.name,
					Text:     found.text,
					Response: response,
				})
			}
		}
	})
	for _, w := range wrappers {
		completed, unresolved := p.callers(w, 0)
		pairings = append(pairings, completed...)
		problems = append(problems, unresolved...)
	}
	sort.Slice(pairings, func(i, j int) bool { return pairingLess(pairings[i], pairings[j]) })
	sort.Slice(problems, func(i, j int) bool { return positionLess(problems[i].Position, problems[j].Position) })
	return pairings, problems
}

// eachCall calls visit for every call in the program, with the function
// declaration it sits in.
//
// Only calls inside a function body are visited. A send in a package-level
// initializer would have no locals and no parameters to resolve, and nothing
// in this repository sends one there. Every declaration has a body: a
// function without one does not type-check, and a package that does not
// type-check was refused by the loader.
func (p *program) eachCall(visit func(pkg *packages.Package, fn *ast.FuncDecl, call *ast.CallExpr)) {
	for _, pkg := range p.pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc {
					continue
				}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					if call, isCall := node.(*ast.CallExpr); isCall {
						visit(pkg, fn, call)
					}
					return true
				})
			}
		}
	}
}

// isSend reports whether call is Do with a GraphQLQuery and a decode target.
func isSend(pkg *packages.Package, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != sendMethod || len(call.Args) < 2 {
		return false
	}
	named, isNamed := pkg.TypesInfo.TypeOf(call.Args[0]).(*types.Named)
	return isNamed && named.Obj().Name() == queryTypeName
}

// responseType returns what the decode target points at, or says why the
// argument is not something a JSON decoder can fill.
func responseType(pkg *packages.Package, arg ast.Expr) (response types.Type, why string) {
	pointer, ok := pkg.TypesInfo.TypeOf(arg).(*types.Pointer)
	if !ok {
		return nil, "the decode target is not a pointer, so nothing GitLab answers can be written into it"
	}
	return pointer.Elem(), ""
}

// documentExpr finds the expression a call's request literal gives the Query
// field, reading through a local variable the literal was assigned to.
func documentExpr(pkg *packages.Package, fn *ast.FuncDecl, arg ast.Expr) (expr ast.Expr, why string) {
	literal := requestLiteral(pkg, fn, arg)
	if literal == nil {
		return nil, "the request is not a " + queryTypeName + " literal written at the call or assigned to the variable the call names"
	}
	expr = fieldValue(literal, queryField)
	if expr == nil {
		return nil, "the request literal sets no " + queryField + " field by name"
	}
	return expr, ""
}

// requestLiteral finds the GraphQLQuery composite literal a call sends: written
// at the call, or the first one assigned to the local variable the call names.
func requestLiteral(pkg *packages.Package, fn *ast.FuncDecl, arg ast.Expr) *ast.CompositeLit {
	switch expr := arg.(type) {
	case *ast.CompositeLit:
		return expr
	case *ast.Ident:
		for _, value := range localValues(pkg, fn, pkg.TypesInfo.ObjectOf(expr)) {
			if literal, isLiteral := value.(*ast.CompositeLit); isLiteral {
				return literal
			}
		}
		return nil
	default:
		return nil
	}
}

// localValues lists every value a function assigns to a variable, in source
// order: `query := a`, `query = b`, `var query = c`. A statement that assigns
// the variable from a call returning several values contributes that call,
// which folds to nothing and is reported as such.
func localValues(pkg *packages.Package, fn *ast.FuncDecl, variable types.Object) []ast.Expr {
	var values []ast.Expr
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		names, assigned := assignment(node)
		for i, name := range names {
			id, isIdent := name.(*ast.Ident)
			if !isIdent || pkg.TypesInfo.ObjectOf(id) != variable {
				continue
			}
			switch {
			case len(names) == len(assigned):
				values = append(values, assigned[i])
			case len(assigned) > 0:
				values = append(values, assigned[0])
			}
		}
		return true
	})
	return values
}

// assignment returns the names and values a statement assigns, for the two
// statements that give a local a value: an assignment and a var spec.
func assignment(node ast.Node) (names, values []ast.Expr) {
	switch stmt := node.(type) {
	case *ast.AssignStmt:
		return stmt.Lhs, stmt.Rhs
	case *ast.ValueSpec:
		for _, name := range stmt.Names {
			names = append(names, name)
		}
		return names, stmt.Values
	default:
		return nil, nil
	}
}

// fieldValue returns the value a composite literal gives the named field, or
// nil when the literal does not set it by name.
func fieldValue(literal *ast.CompositeLit, name string) ast.Expr {
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		if key, isIdent := pair.Key.(*ast.Ident); isIdent && key.Name == name {
			return pair.Value
		}
	}
	return nil
}

// source is where a document came from: its folded text when the expression
// is constant, the parameter it arrives through when the enclosing function is
// a wrapper, or the reason it is neither.
type source struct {
	text     string
	name     string
	viaParam bool
	param    int
	field    string
	why      string
}

// documentSources folds the expression holding the document, or traces it to
// where it comes from: a local variable the function gives one or more
// constants, or a parameter the caller fills.
//
// Constants are folded by the type checker, so a document assembled from a
// shared fragment constant is read as the one string GitLab receives. A local
// variable assigned in several branches yields one source per assignment, so
// a call that picks between two documents is paired with both. A parameter,
// or a field of a struct parameter, is the wrapper shape: the document is
// named by the caller, and the pairing is completed there.
func documentSources(pkg *packages.Package, fn *ast.FuncDecl, expr ast.Expr) []source {
	if folded, ok := foldDocument(pkg, expr); ok {
		return []source{folded}
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if index, isParam := parameterIndex(pkg, fn, e); isParam {
			return []source{{viaParam: true, param: index}}
		}
		if values := localValues(pkg, fn, pkg.TypesInfo.ObjectOf(e)); len(values) > 0 {
			sources := make([]source, 0, len(values))
			for _, value := range values {
				if folded, ok := foldDocument(pkg, value); ok {
					sources = append(sources, folded)
					continue
				}
				sources = append(sources, unfolded(value))
			}
			return sources
		}
	case *ast.SelectorExpr:
		if base, isIdent := e.X.(*ast.Ident); isIdent {
			if index, isParam := parameterIndex(pkg, fn, base); isParam {
				return []source{{viaParam: true, param: index, field: e.Sel.Name}}
			}
		}
	}
	return []source{unfolded(expr)}
}

// foldDocument reads the constant value of an expression, when it has one.
func foldDocument(pkg *packages.Package, expr ast.Expr) (source, bool) {
	value, typed := pkg.TypesInfo.Types[expr]
	if !typed || value.Value == nil || value.Value.Kind() != constant.String {
		return source{}, false
	}
	folded := source{text: constant.StringVal(value.Value)}
	if id, isIdent := expr.(*ast.Ident); isIdent {
		folded.name = id.Name
	}
	return folded, true
}

// unfolded says why an expression yields no document before a request is
// made.
func unfolded(expr ast.Expr) source {
	return source{why: "the document is built at run time, from " + types.ExprString(expr) + ", so nothing can be judged before a request is made"}
}

// parameterIndex returns the position of id in fn's flattened parameter list,
// when id names one of its parameters. A parameter list is named throughout
// or not at all, so a list an identifier can name has a name in every field.
func parameterIndex(pkg *packages.Package, fn *ast.FuncDecl, id *ast.Ident) (int, bool) {
	target := pkg.TypesInfo.ObjectOf(id)
	index := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if pkg.TypesInfo.ObjectOf(name) == target {
				return index, true
			}
			index++
		}
	}
	return 0, false
}

// maxHandovers bounds how many functions a document may pass through on its
// way to Do before the audit stops following it. Two is the deepest chain in
// this repository; the bound exists so a recursive function cannot send the
// walk around in a circle.
const maxHandovers = 8

// callers completes a wrapper's pairing at every call of it, reading the
// document from the argument the wrapper's parameter receives. A caller that
// hands over a parameter of its own is a wrapper too, followed to its callers
// in turn; depth counts the hand-overs so far.
func (p *program) callers(w wrapper, depth int) ([]pairing, []problem) {
	var (
		pairings []pairing
		problems []problem
	)
	p.eachCall(func(pkg *packages.Package, fn *ast.FuncDecl, call *ast.CallExpr) {
		callee := calleeIdent(call.Fun)
		if callee == nil || !sameFunc(pkg.TypesInfo.ObjectOf(callee), w.fn) {
			return
		}
		position := p.fset.Position(call.Pos())
		expr, why := wrapperArgument(call, w)
		if why != "" {
			problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: why})
			return
		}
		for _, handed := range documentSources(pkg, fn, expr) {
			switch {
			case handed.viaParam && depth >= maxHandovers:
				problems = append(problems, problem{
					Package:  pkg.PkgPath,
					Position: position,
					Message:  fmt.Sprintf("hands %s a document it received through a parameter of its own, %d hand-overs deep, which is further than this audit follows", w.fn.Name(), depth+1),
				})
			case handed.viaParam:
				declared, _ := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
				completed, unresolved := p.callers(wrapper{
					fn:       declared,
					param:    handed.param,
					field:    handed.field,
					response: w.response,
					pkg:      w.pkg,
					position: w.position,
					typeArgs: bindings(w.typeArgs, instantiation(pkg, callee, w.fn)),
				}, depth+1)
				pairings = append(pairings, completed...)
				problems = append(problems, unresolved...)
			case handed.text == "":
				problems = append(problems, problem{
					Package:  pkg.PkgPath,
					Position: position,
					Message:  "hands " + w.fn.Name() + " its document here, and " + handed.why,
				})
			default:
				pairings = append(pairings, pairing{
					Package:  w.pkg.PkgPath,
					Position: w.position,
					Name:     handed.name,
					Text:     handed.text,
					Response: w.response,
					TypeArgs: bindings(w.typeArgs, instantiation(pkg, callee, w.fn)),
					Origin:   position,
				})
			}
		}
	})
	if len(pairings) == 0 && len(problems) == 0 {
		problems = append(problems, problem{
			Package:  w.pkg.PkgPath,
			Position: w.position,
			Message:  w.fn.Name() + " receives its document through a parameter and nothing calls it, so no document reaches this decoder",
		})
	}
	return pairings, problems
}

// wrapperArgument returns the expression a call hands the wrapper's document
// parameter, or says why the call does not name one.
func wrapperArgument(call *ast.CallExpr, w wrapper) (expr ast.Expr, why string) {
	if len(call.Args) <= w.param {
		return nil, "calls " + w.fn.Name() + " with fewer arguments than the parameter its document arrives through"
	}
	arg := call.Args[w.param]
	if w.field == "" {
		return arg, ""
	}
	literal, isLiteral := arg.(*ast.CompositeLit)
	if !isLiteral {
		return nil, fmt.Sprintf("hands %s a %s that is not a literal written at the call, so its %s field cannot be read", w.fn.Name(), types.ExprString(arg), w.field)
	}
	expr = fieldValue(literal, w.field)
	if expr == nil {
		return nil, "hands " + w.fn.Name() + " a literal that sets no " + w.field + " field by name"
	}
	return expr, ""
}

// calleeIdent returns the identifier a call resolves through, looking past a
// package selector, an explicit instantiation and parentheses.
func calleeIdent(fun ast.Expr) *ast.Ident {
	for {
		switch f := fun.(type) {
		case *ast.Ident:
			return f
		case *ast.SelectorExpr:
			return f.Sel
		case *ast.IndexExpr:
			fun = f.X
		case *ast.IndexListExpr:
			fun = f.X
		case *ast.ParenExpr:
			fun = f.X
		default:
			return nil
		}
	}
}

// sameFunc reports whether obj is the wrapper's function.
//
// Compared by package path and name rather than by identity: a package loaded
// from export data and the same package type-checked from source are two
// objects for one function, and which one a use resolves to depends on load
// order this audit should not have to know.
func sameFunc(obj types.Object, fn *types.Func) bool {
	callee, isFunc := obj.(*types.Func)
	return isFunc && callee.Pkg() != nil && callee.Pkg().Path() == fn.Pkg().Path() && callee.Name() == fn.Name()
}

// instantiation binds the wrapper's type parameters to the arguments a call
// instantiated it with, or returns nil for a wrapper that has none.
func instantiation(pkg *packages.Package, callee *ast.Ident, fn *types.Func) map[*types.TypeParam]types.Type {
	instance, instantiated := pkg.TypesInfo.Instances[callee]
	if !instantiated {
		return nil
	}
	params := fn.Signature().TypeParams()
	bound := make(map[*types.TypeParam]types.Type, params.Len())
	for i := 0; i < params.Len() && i < instance.TypeArgs.Len(); i++ {
		bound[params.At(i)] = instance.TypeArgs.At(i)
	}
	return bound
}

// bindings joins the type-parameter bindings carried so far with the ones a
// call adds, returning nil when there are none at all so a pairing with no
// generics in its path carries nothing.
func bindings(carried, added map[*types.TypeParam]types.Type) map[*types.TypeParam]types.Type {
	if len(carried) == 0 && len(added) == 0 {
		return nil
	}
	joined := make(map[*types.TypeParam]types.Type, len(carried)+len(added))
	maps.Copy(joined, carried)
	maps.Copy(joined, added)
	return joined
}

// pairingLess orders pairings the way a reader walks a repository: by the call
// they judge, then by where the document was handed over.
func pairingLess(a, b pairing) bool {
	if a.Position != b.Position {
		return positionLess(a.Position, b.Position)
	}
	return positionLess(a.Origin, b.Origin)
}

// positionLess orders positions by file, then line, then column.
func positionLess(a, b token.Position) bool {
	if a.Filename != b.Filename {
		return a.Filename < b.Filename
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
}
