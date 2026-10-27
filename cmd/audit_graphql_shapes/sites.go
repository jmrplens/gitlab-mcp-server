// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
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
	// OriginPackage is the package that handed it over, which is the domain a
	// document sent through a shared wrapper belongs to.
	Origin        token.Position
	OriginPackage string
	// Function is the function that names the document: the one the Do call
	// is written in, or, when the document was handed to a wrapper, the
	// function that handed it over, which is the one a reader would say sends
	// it. The client-go run asks whether this server reaches it, and files
	// what the document leaves out under the service it belongs to, so a
	// helper two services send through neither makes one service's document
	// reached because the other's is, nor files both under the helper.
	Function *types.Func
	// Unit is what a field the document leaves out is filed under when it is
	// not the package of the struct decoding the object: the client-go run
	// sets it to the service the function belongs to, since every decoder of
	// client-go is declared in one package.
	Unit string
	// ReadThrough makes an object that decodes no leaf of its own count as
	// read when the object it hangs from is read, so the schema is asked what
	// it offers there too. The client-go run sets it: a work item's features
	// and each widget under them carry only the next object, and asking
	// nothing of them hides every widget and every widget field client-go
	// leaves out, which is where GitLab grows the type. Our own documents keep
	// the narrower rule, under which such an object is an envelope around a
	// domain R-PATH already asks this question of.
	ReadThrough bool
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
	// Function is the function the problem was found in, which the client-go
	// run reads to tell a send this server reaches from one it never calls.
	// It is nil for a document no send carries, which is in no function.
	Function *types.Func
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
	// documents is what the document inventory reads out of the same
	// packages, rendered wherever the inventory can say what the text is. It
	// is where a document assembled at run time gets its text from: the
	// format of a fmt.Sprintf call, or the template a set executes. A named
	// one is found by the object it is declared as and one written in place
	// by its position, which is how the inventory keys its own renderings.
	documents  []graphqldocs.Document
	byObject   map[types.Object]graphqldocs.Document
	byPosition map[token.Position]graphqldocs.Document
	// initializers holds the value every package variable is declared with,
	// which is how a template set a function executes is followed to the
	// text it was parsed from.
	initializers map[*types.Var]initializer
}

// initializer is the expression a package variable is declared with, in the
// package that declares it.
type initializer struct {
	pkg   *packages.Package
	value ast.Expr
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
	return newProgram(loaded), nil
}

// newProgram indexes loaded packages for the walk: the documents the inventory
// reads out of them, and the value each package variable is declared with.
func newProgram(loaded []*packages.Package) *program {
	p := &program{
		fset:         loaded[0].Fset,
		pkgs:         loaded,
		documents:    graphqldocs.FromPackages(loaded),
		byObject:     map[types.Object]graphqldocs.Document{},
		byPosition:   map[token.Position]graphqldocs.Document{},
		initializers: map[*types.Var]initializer{},
	}
	for _, document := range p.documents {
		if document.Object != nil {
			p.byObject[document.Object] = document
			continue
		}
		p.byPosition[document.Position] = document
	}
	for _, pkg := range loaded {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				p.recordInitializers(pkg, decl)
			}
		}
	}
	return p
}

// recordInitializers notes the value every package variable a declaration
// names is given, one value per name. A declaration giving several names the
// results of one call gives none of them a value this walk can follow, and is
// passed over like one that gives no value at all.
func (p *program) recordInitializers(pkg *packages.Package, decl ast.Decl) {
	general, isGeneral := decl.(*ast.GenDecl)
	if !isGeneral {
		return
	}
	for _, spec := range general.Specs {
		values, isValues := spec.(*ast.ValueSpec)
		if !isValues || len(values.Values) != len(values.Names) {
			continue
		}
		for i, name := range values.Names {
			if variable, isVar := pkg.TypesInfo.Defs[name].(*types.Var); isVar {
				p.initializers[variable] = initializer{pkg: pkg, value: values.Values[i]}
			}
		}
	}
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
		declared := declaredFunc(pkg, fn)
		response, why := responseType(pkg, call.Args[1])
		if why != "" {
			problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: why, Function: declared})
			return
		}
		expr, why := documentExpr(pkg, fn, call.Args[0])
		if why != "" {
			problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: why, Function: declared})
			return
		}
		for _, found := range p.documentSources(pkg, fn, expr) {
			// Ifs rather than a tagless switch here and in the two below: the
			// mutation tool cannot see a case expression, so every condition
			// of one reads as never covered.
			if found.viaParam {
				wrappers = append(wrappers, wrapper{
					fn:       declared,
					param:    found.param,
					field:    found.field,
					response: response,
					pkg:      pkg,
					position: position,
				})
				continue
			}
			if found.text == "" {
				problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: found.why, Function: declared})
				continue
			}
			pairings = append(pairings, pairing{
				Package:  pkg.PkgPath,
				Position: position,
				Name:     found.name,
				Text:     found.text,
				Response: response,
				Function: declared,
			})
		}
	})
	for _, w := range wrappers {
		completed, unresolved := p.callers(w, 0)
		pairings = append(pairings, completed...)
		problems = append(problems, unresolved...)
	}
	slices.SortFunc(pairings, comparePairings)
	slices.SortFunc(problems, func(a, b problem) int { return comparePositions(a.Position, b.Position) })
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
			if len(names) == len(assigned) {
				values = append(values, assigned[i])
				continue
			}
			if len(assigned) > 0 {
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
		// Both callers hand over a struct literal, whose keys are field names,
		// so a key is spelled exactly as the field it sets.
		if types.ExprString(pair.Key) == name {
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
// named by the caller, and the pairing is completed there. A call is a
// document assembled at run time, read through the rendering the document
// inventory makes of it (see [program.assembled]).
func (p *program) documentSources(pkg *packages.Package, fn *ast.FuncDecl, expr ast.Expr) []source {
	switch e := expr.(type) {
	case *ast.Ident:
		if index, isParam := parameterIndex(pkg, fn, e); isParam {
			return []source{{viaParam: true, param: index}}
		}
		if values := localValues(pkg, fn, pkg.TypesInfo.ObjectOf(e)); len(values) > 0 {
			sources := make([]source, 0, len(values))
			for _, value := range values {
				sources = append(sources, p.valueSource(pkg, fn, value))
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
	return []source{p.valueSource(pkg, fn, expr)}
}

// valueSource reads the document one expression holds: its constant value, the
// rendering of the call that assembles it, or the reason it is neither.
//
// A constant is tried first whatever the expression is, since a named
// constant is an identifier and a selector as much as a parameter or a local
// is, and a constant expression may be written as a call (a conversion).
func (p *program) valueSource(pkg *packages.Package, fn *ast.FuncDecl, value ast.Expr) source {
	if folded, ok := foldDocument(pkg, value); ok {
		return folded
	}
	if call, isCall := value.(*ast.CallExpr); isCall {
		return p.assembled(pkg, fn, call)
	}
	return unfolded(value)
}

// The calls an assembled document is read through, by their full names as the
// type checker spells them, so a package that imports one under another name
// is read the same way.
const (
	sprintfName       = "fmt.Sprintf"
	builderStringName = "(*strings.Builder).String"
	bufferStringName  = "(*bytes.Buffer).String"
	executeName       = "(*text/template.Template).Execute"
	mustName          = "text/template.Must"
	parseName         = "(*text/template.Template).Parse"
)

// assembled reads a document a call assembles at run time, which is how
// client-go writes six of its own: the format of a fmt.Sprintf call, and what a
// text/template set writes into a strings.Builder or a bytes.Buffer whose
// String the request is given.
//
// The text is the document inventory's rendering (see
// [graphqldocs.Assembly]), found the way the inventory keys it, so this walk
// judges the same string the schema gate does and never renders anything
// itself. A call of any other kind, a format that is not a document, and a
// builder that is not a local are what they were before: a document built at
// run time.
func (p *program) assembled(pkg *packages.Package, fn *ast.FuncDecl, call *ast.CallExpr) source {
	switch calleeName(pkg.TypesInfo, call) {
	case sprintfName:
		if document, found := p.documentAt(pkg, call.Args[0]); found {
			return renderedSource(document, document.Name)
		}
	case builderStringName, bufferStringName:
		if builder, isIdent := ast.Unparen(receiverOf(call)).(*ast.Ident); isIdent {
			return p.executed(pkg, fn, builder)
		}
	}
	return unfolded(call)
}

// executed reads the document a template set writes into a builder: the one
// text/template Execute call in the function that writes it, the package
// variable that call executes, and the text that variable's set was parsed
// from, which the document inventory has rendered.
//
// Each step that does not hold is a reason of its own, because each is a
// different change to client-go: a builder two templates write needs a
// different reading than one written by a set built when a function runs.
func (p *program) executed(pkg *packages.Package, fn *ast.FuncDecl, builder *ast.Ident) source {
	written := executions(pkg.TypesInfo, fn.Body, pkg.TypesInfo.ObjectOf(builder))
	if len(written) != 1 {
		return builtFrom(builder, fmt.Sprintf("%d text/template Execute calls in this function write it, where exactly one says which template it is", len(written)))
	}
	set := receiverOf(written[0])
	variable, isPackageVar := packageVariable(pkg.TypesInfo, set)
	if !isPackageVar {
		return builtFrom(builder, "the template it is written from, "+types.ExprString(set)+", is not a package variable, so its set is built when a function runs")
	}
	declared, found := p.initializers[variable]
	if !found {
		return builtFrom(builder, variable.Name()+" is declared with no value this audit reads")
	}
	text, isParsed := parsedText(declared.pkg.TypesInfo, declared.value)
	if !isParsed {
		return builtFrom(builder, variable.Name()+" is built with "+types.ExprString(text)+" rather than parsed from one text")
	}
	document, found := p.documentAt(declared.pkg, text)
	if !found {
		return builtFrom(builder, "the text "+variable.Name()+" parses is not a document the inventory reads")
	}
	return renderedSource(document, variable.Name())
}

// builtFrom says why the document a builder holds could not be read.
func builtFrom(builder *ast.Ident, reason string) source {
	return source{why: "the document is what " + builder.Name + " holds, and " + reason}
}

// renderedSource is the document an assembled send carries, under the name
// the call reaches it by, or the reason the inventory could not render it.
func renderedSource(document graphqldocs.Document, name string) source {
	if assembly := document.Assembly; assembly != nil && assembly.Unrendered != "" {
		return source{why: "the document is a shell " + assembly.By + " fills, and " + assembly.Unrendered}
	}
	return source{text: document.Text, name: name}
}

// documentAt finds the inventory's document for the expression a call takes
// it as: by the object a name resolves to, or by the position of one written in
// place.
func (p *program) documentAt(pkg *packages.Package, expr ast.Expr) (graphqldocs.Document, bool) {
	if object := namedObject(pkg.TypesInfo, expr); object != nil {
		document, found := p.byObject[object]
		return document, found
	}
	document, found := p.byPosition[p.fset.Position(ast.Unparen(expr).Pos())]
	return document, found
}

// executions lists the text/template Execute calls in a body that write into
// a builder, which they do when they are handed its address.
func executions(info *types.Info, body ast.Node, builder types.Object) []*ast.CallExpr {
	var found []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if isCall && calleeName(info, call) == executeName && addresses(info, call.Args[0], builder) {
			found = append(found, call)
		}
		return true
	})
	return found
}

// addresses reports whether an argument is the address of the object, written
// as &name.
func addresses(info *types.Info, argument ast.Expr, object types.Object) bool {
	address, isUnary := ast.Unparen(argument).(*ast.UnaryExpr)
	if !isUnary || address.Op != token.AND {
		return false
	}
	target, isIdent := ast.Unparen(address.X).(*ast.Ident)
	return isIdent && info.ObjectOf(target) == object
}

// parsedText reads through the calls a package variable's template set is
// declared with, template.Must and nothing else, to the text the last Parse
// gives it, which is the template the variable executes. It returns the
// expression it stopped at, and false, when the set is built some other way.
func parsedText(info *types.Info, value ast.Expr) (ast.Expr, bool) {
	for {
		call, isCall := ast.Unparen(value).(*ast.CallExpr)
		if !isCall {
			return value, false
		}
		switch calleeName(info, call) {
		case mustName:
			value = call.Args[0]
		case parseName:
			return call.Args[0], true
		default:
			return value, false
		}
	}
}

// packageVariable reports the variable an expression names and whether it is
// declared at package level, the one place a value is given once for every
// function that executes it.
func packageVariable(info *types.Info, expr ast.Expr) (*types.Var, bool) {
	variable, isVar := namedObject(info, expr).(*types.Var)
	return variable, isVar && variable.Parent() == variable.Pkg().Scope()
}

// namedObject is the object an expression names, a qualified identifier
// included, or nil for one that names none.
func namedObject(info *types.Info, expr ast.Expr) types.Object {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return info.Uses[e]
	case *ast.SelectorExpr:
		return info.Uses[e.Sel]
	default:
		return nil
	}
}

// calleeName is the full name of the function a call reaches, or "" when it
// reaches none statically: a conversion, a builtin or a call through a
// function value.
func calleeName(info *types.Info, call *ast.CallExpr) string {
	if callee, isFunc := typeutil.Callee(info, call).(*types.Func); isFunc {
		return callee.FullName()
	}
	return ""
}

// receiverOf is the value a method call is made on. Every call this is asked
// about reached a method, which is always written as a selector.
func receiverOf(call *ast.CallExpr) ast.Expr {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	return selector.X
}

// declaredFunc is the function a declaration defines. Every declaration this
// walk visits type-checked, so it defines one.
func declaredFunc(pkg *packages.Package, fn *ast.FuncDecl) *types.Func {
	declared, _ := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
	return declared
}

// foldDocument reads the constant value of an expression, when it has one.
//
// Every expression it is handed is one a GraphQLQuery's Query field is given,
// directly, through a local, or through a wrapper's parameter feeding it, so
// its type is a string and a constant of it is a string constant.
func foldDocument(pkg *packages.Package, expr ast.Expr) (source, bool) {
	value := pkg.TypesInfo.Types[expr]
	if value.Value == nil {
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
		declared := declaredFunc(pkg, fn)
		expr, why := wrapperArgument(call, w)
		if why != "" {
			problems = append(problems, problem{Package: pkg.PkgPath, Position: position, Message: why, Function: declared})
			return
		}
		for _, handed := range p.documentSources(pkg, fn, expr) {
			if handed.viaParam && depth >= maxHandovers {
				problems = append(problems, problem{
					Package:  pkg.PkgPath,
					Position: position,
					Message:  fmt.Sprintf("hands %s a document it received through a parameter of its own, %d hand-overs deep, which is further than this audit follows", w.fn.Name(), depth+1),
					Function: declared,
				})
				continue
			}
			if handed.viaParam {
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
				continue
			}
			if handed.text == "" {
				problems = append(problems, problem{
					Package:  pkg.PkgPath,
					Position: position,
					Message:  "hands " + w.fn.Name() + " its document here, and " + handed.why,
					Function: declared,
				})
				continue
			}
			pairings = append(pairings, pairing{
				Package:  w.pkg.PkgPath,
				Position: w.position,
				Name:     handed.name,
				Text:     handed.text,
				Response: w.response,
				TypeArgs: bindings(w.typeArgs, instantiation(pkg, callee, w.fn)),

				Origin:        position,
				OriginPackage: pkg.PkgPath,
				Function:      declared,
			})
		}
	})
	if len(pairings) == 0 && len(problems) == 0 {
		problems = append(problems, problem{
			Package:  w.pkg.PkgPath,
			Position: w.position,
			Message:  w.fn.Name() + " receives its document through a parameter and nothing calls it, so no document reaches this decoder",
			Function: w.fn,
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
// Compared by full name rather than by identity: a package loaded from export
// data and the same package type-checked from source are two objects for one
// function, and which one a use resolves to depends on load order this audit
// should not have to know. The full name carries the package path and, for a
// method, the receiver, so a method that shares the wrapper's name in the
// wrapper's package is not read as a call of the wrapper.
//
// A method of a generic type called on an instance resolves to the instance's
// own copy of the method, whose receiver spells the instance's type arguments
// where the declaration spells its type parameters, so the callee is compared
// by the method it was instantiated from.
func sameFunc(obj types.Object, fn *types.Func) bool {
	callee, isFunc := obj.(*types.Func)
	return isFunc && callee.Origin().FullName() == fn.FullName()
}

// instantiation binds the wrapper's type parameters to the arguments a call
// instantiated it with, or returns nil for a wrapper that has none.
//
// An instance carries one type argument per type parameter, inferred ones
// included: go/types promises that instantiating the callee with them
// reproduces the instance, and the callee is the wrapper itself, which is why
// the loop is bounded by the wrapper's parameters alone.
//
// A method has no type parameters of its own, and a method of a generic type
// is bound by its receiver instead, which the instances record says nothing
// about: see [receiverInstantiation].
func instantiation(pkg *packages.Package, callee *ast.Ident, fn *types.Func) map[*types.TypeParam]types.Type {
	if fn.Signature().RecvTypeParams().Len() > 0 {
		return receiverInstantiation(pkg, callee, fn)
	}
	instance, instantiated := pkg.TypesInfo.Instances[callee]
	if !instantiated {
		return nil
	}
	params := fn.Signature().TypeParams()
	bound := make(map[*types.TypeParam]types.Type, params.Len())
	for i := range params.Len() {
		bound[params.At(i)] = instance.TypeArgs.At(i)
	}
	return bound
}

// receiverInstantiation binds the receiver type parameters of a wrapper that
// is a method of a generic type to the type arguments of the receiver a call
// is made on.
//
// go/types records no instance for a method selector, so the binding is read
// off the method the call resolves to: called on an instance, that is the
// instance's own copy of the method, whose receiver spells the type arguments
// (client[project]) where the declaration spells its parameters (client[T]).
// The receiver declares one parameter per argument of its type, in the order
// the type declares them, so the two lists pair by position. A call from a
// sibling method binds the parameters to the sibling's own, which its callers
// would bind in turn, and a decoder typed by one of those is left unjudged the
// way a generic function's is when the document is named in between.
//
// The call has already been matched to the wrapper, so what it resolves to is
// a method, and the receiver of a method of a generic type is that type or a
// pointer to it: neither assertion can fail here.
func receiverInstantiation(pkg *packages.Package, callee *ast.Ident, fn *types.Func) map[*types.TypeParam]types.Type {
	method, _ := pkg.TypesInfo.ObjectOf(callee).(*types.Func)
	receiver, _ := pointee(method.Signature().Recv().Type()).(*types.Named)
	params, args := fn.Signature().RecvTypeParams(), receiver.TypeArgs()
	bound := make(map[*types.TypeParam]types.Type, params.Len())
	for i := range params.Len() {
		bound[params.At(i)] = args.At(i)
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
	joined := map[*types.TypeParam]types.Type{}
	maps.Copy(joined, carried)
	maps.Copy(joined, added)
	return joined
}

// comparePairings orders pairings the way a reader walks a repository: by the
// call they judge, then by where the document was handed over.
func comparePairings(a, b pairing) int {
	return cmp.Or(comparePositions(a.Position, b.Position), comparePositions(a.Origin, b.Origin))
}

// comparePositions orders positions by file, then line, then column. Each
// level is decided by the sign of one comparison, and a tie falls through to
// the next.
func comparePositions(a, b token.Position) int {
	return cmp.Or(strings.Compare(a.Filename, b.Filename), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}
