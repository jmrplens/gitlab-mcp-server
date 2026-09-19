package main

import (
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// The four kinds of site a published action ID is written at.
const (
	kindRelated     = "related"
	kindHint        = "hint"
	kindUsage       = "usage"
	kindDescription = "description"
)

// hintActionFunc is the toolutil helper every cross-link hint is written
// through: toolutil.HintAction(id, purpose) renders "Use action 'id' to
// purpose" into the Markdown a model reads.
const hintActionFunc = "HintAction"

// idListFieldNames are the fields whose every element is a canonical action
// ID, matched without case so the exported spelling and the unexported one are
// one rule.
//
// Both spellings are needed and neither covers the tree. ActionSpec,
// ActionSpecOptions, ActionRoute and the catalog's own Action carry
// RelatedActions; about fifty packages instead keep a package-local metadata
// table whose entry struct has a lowercase related field, copied onto the
// options by a decorate helper. The IDs are written in that table, which is
// where a reader fixes them, and a rule that knew only the exported field
// would see the copy and never the list.
var idListFieldNames = map[string]struct{}{
	"related":        {},
	"relatedactions": {},
}

// proseFieldNames are the model-facing prose fields, mapped to the site kind
// they are reported under. A dotted ID inside one of these is an invitation to
// call it, the same as a cross-link.
var proseFieldNames = map[string]string{
	"usage":       kindUsage,
	"description": kindDescription,
}

// site is one place a published action ID was written.
//
// Value carries the folded constant: the ID itself for a related entry or a
// hint, and the whole prose for a Usage line or a description, whose candidate
// IDs are extracted later because picking them needs the catalog's domains.
// An unresolved site carries Expr instead, the expression as it was written,
// since naming what could not be folded is the only honest alternative to
// passing over it.
type site struct {
	Package  string
	File     string
	Line     int
	Kind     string
	Value    string
	Expr     string
	Resolved bool
}

// program is the loaded source, indexed by the two things the walk has to look
// up: the body of a function it follows into, and every value a variable is
// ever given.
type program struct {
	root  string
	pkgs  []*packages.Package
	decls map[*types.Func]funcDecl
	// values maps a variable to every expression assigned to it. A list built
	// up over a few conditional appends has no single value, so what is
	// recorded is all of them: the question here is which IDs a variable can
	// carry, and the answer is the union.
	values map[*types.Var][]ast.Expr
	// params maps a function parameter to the function and position it sits
	// at, and callers maps a function to every call of it, which together
	// answer what a parameter can hold.
	params  map[*types.Var]paramRef
	callers map[*types.Func][]callSite
}

// paramRef is one parameter's place in the signature of a declared function.
type paramRef struct {
	fn    *types.Func
	index int
}

// callSite is one call of a declared function, kept with the package it was
// written in so its arguments resolve in their own scope.
type callSite struct {
	call *ast.CallExpr
	pkg  *packages.Package
}

// funcDecl is a declared function kept with the package it was written in, so
// its return expressions resolve in their own scope.
type funcDecl struct {
	pkg  *packages.Package
	decl *ast.FuncDecl
}

// collectSites loads the packages named by patterns, rooted at dir, and
// returns every site a published action ID was written at.
//
// The load, including the refusal of a package that did not type-check,
// belongs to [goprogram.Load]. What is here is the walk: a package that did
// not type-check folds no constants, and folding constants is the point.
//
// The overlay is how a test supplies source that is not on disk, so the walk
// is exercised on the shapes it has to handle, type-checked against the real
// toolutil, rather than on a mock of them. Production passes nil.
func collectSites(dir string, patterns []string, overlay map[string][]byte) ([]site, error) {
	loaded, err := goprogram.Load(dir, patterns, overlay)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	prog := indexProgram(root, loaded)
	collect := &collector{
		prog:        prog,
		visited:     map[*types.Func]struct{}{},
		visitedVars: map[*types.Var]struct{}{},
	}
	for _, pkg := range prog.pkgs {
		walk := &walker{collector: collect, pkg: pkg}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, walk.visit)
		}
	}
	return collect.sites, nil
}

// indexProgram records every function declared in the loaded packages, so a
// value handed over by a helper can be followed to the literals inside it, and
// every value a variable is given, so a list built up in a local can be too.
func indexProgram(root string, loaded []*packages.Package) *program {
	prog := &program{
		root:    root,
		pkgs:    loaded,
		decls:   map[*types.Func]funcDecl{},
		values:  map[*types.Var][]ast.Expr{},
		params:  map[*types.Var]paramRef{},
		callers: map[*types.Func][]callSite{},
	}
	for _, pkg := range loaded {
		for _, file := range pkg.Syntax {
			prog.indexFuncs(pkg, file)
			ast.Inspect(file, func(node ast.Node) bool {
				prog.indexValues(pkg, node)
				prog.indexCall(pkg, node)
				return true
			})
		}
	}
	return prog
}

// indexFuncs records the functions one file declares, and where each of their
// parameters sits.
func (p *program) indexFuncs(pkg *packages.Package, file *ast.File) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		obj, isFunc := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
		if !isFunc {
			continue
		}
		p.decls[obj] = funcDecl{pkg: pkg, decl: fn}
		signature, isSignature := obj.Type().(*types.Signature)
		if !isSignature {
			continue
		}
		for index := range signature.Params().Len() {
			p.params[signature.Params().At(index)] = paramRef{fn: obj, index: index}
		}
	}
}

// indexCall records one call of a declared function, so a parameter can be
// followed to the arguments every caller passes.
func (p *program) indexCall(pkg *packages.Package, node ast.Node) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return
	}
	var ident *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return
	}
	if callee, isFunc := pkg.TypesInfo.Uses[ident].(*types.Func); isFunc {
		p.callers[callee] = append(p.callers[callee], callSite{call: call, pkg: pkg})
	}
}

// indexValues records the expressions assigned to a variable, by a short
// declaration, an assignment or a var declaration alike.
func (p *program) indexValues(pkg *packages.Package, node ast.Node) {
	switch typed := node.(type) {
	case *ast.AssignStmt:
		if len(typed.Lhs) != len(typed.Rhs) {
			return
		}
		for index, left := range typed.Lhs {
			p.recordValue(pkg, left, typed.Rhs[index])
		}
	case *ast.ValueSpec:
		if len(typed.Names) != len(typed.Values) {
			return
		}
		for index, name := range typed.Names {
			p.recordValue(pkg, name, typed.Values[index])
		}
	}
}

// recordValue records one expression as a value the named variable can hold.
func (p *program) recordValue(pkg *packages.Package, target, value ast.Expr) {
	ident, ok := target.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return
	}
	variable, ok := variableOf(pkg, ident)
	if !ok {
		return
	}
	p.values[variable] = append(p.values[variable], value)
}

// variableOf resolves an identifier to the variable it declares or names.
func variableOf(pkg *packages.Package, ident *ast.Ident) (*types.Var, bool) {
	if defined, ok := pkg.TypesInfo.Defs[ident].(*types.Var); ok {
		return defined, true
	}
	used, ok := pkg.TypesInfo.Uses[ident].(*types.Var)
	return used, ok
}

// collector is what every walker, including the ones that follow into another
// package, writes into.
type collector struct {
	prog  *program
	sites []site
	// visited is the set of functions already followed into, so a helper that
	// calls itself, or two helpers that call each other, cannot loop.
	visited map[*types.Func]struct{}
	// visitedVars is the same guard for variables, and it is not optional
	// here: a list grown with related = append(related, id) names itself in
	// its own value.
	visitedVars map[*types.Var]struct{}
}

// walker walks one package, writing into the shared collector.
type walker struct {
	*collector
	pkg *packages.Package
}

// inPackage returns a walker over another package, sharing this one's
// collector. It is how a value handed over by a helper in another package is
// followed to the literals that make it.
func (w *walker) inPackage(pkg *packages.Package) *walker {
	return &walker{collector: w.collector, pkg: pkg}
}

// visit dispatches the three shapes a published ID is written in: a call of
// the hint helper, a field of a composite literal, and an assignment to a
// field.
func (w *walker) visit(node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.CallExpr:
		w.visitCall(typed)
	case *ast.CompositeLit:
		w.visitCompositeLit(typed)
	case *ast.AssignStmt:
		w.visitAssign(typed)
	}
	return true
}

// visitCall records the first argument of every toolutil.HintAction call.
func (w *walker) visitCall(call *ast.CallExpr) {
	callee, ok := w.callee(call)
	if !ok || callee.Pkg() == nil {
		return
	}
	if callee.Pkg().Path() != goprogram.ToolutilPath || callee.Name() != hintActionFunc {
		return
	}
	if len(call.Args) == 0 {
		return
	}
	w.recordID(kindHint, call.Args[0])
}

// callee resolves the function a call names, through an import selector or a
// bare identifier alike. Resolution is by object rather than by the text of
// the selector, so an import alias cannot hide a call and a local helper of
// the same name cannot fake one.
func (w *walker) callee(call *ast.CallExpr) (*types.Func, bool) {
	var ident *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return nil, false
	}
	callee, ok := w.pkg.TypesInfo.Uses[ident].(*types.Func)
	return callee, ok
}

// visitCompositeLit records the published fields set by a struct literal,
// which is where the metadata tables write their lists and their prose.
func (w *walker) visitCompositeLit(lit *ast.CompositeLit) {
	structType, ok := w.structType(w.pkg.TypesInfo.Types[lit].Type)
	if !ok {
		return
	}
	for index, element := range lit.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			// A literal written positionally names its fields by order, and
			// Go allows no mixture, so an element that is not a pair means the
			// whole literal is positional.
			w.recordPositionalField(structType, index, element)
			continue
		}
		key, isIdent := pair.Key.(*ast.Ident)
		if !isIdent {
			continue
		}
		fieldType, found := structFieldType(structType, key.Name)
		if !found {
			continue
		}
		w.recordField(key.Name, fieldType, pair.Value)
	}
}

// recordPositionalField records one element of a struct literal written
// without field names, which names its fields by order.
func (w *walker) recordPositionalField(structType *types.Struct, index int, value ast.Expr) {
	if index >= structType.NumFields() {
		return
	}
	field := structType.Field(index)
	w.recordField(field.Name(), field.Type(), value)
}

// visitAssign records the published fields set by an assignment, which is how
// the options are filled: opts.RelatedActions = []string{...} on a value a
// helper built.
func (w *walker) visitAssign(assign *ast.AssignStmt) {
	if len(assign.Lhs) != len(assign.Rhs) {
		return
	}
	for index, left := range assign.Lhs {
		selector, ok := ast.Unparen(left).(*ast.SelectorExpr)
		if !ok {
			continue
		}
		field, ok := w.selectedField(selector)
		if !ok {
			continue
		}
		w.recordField(field.Name(), field.Type(), assign.Rhs[index])
	}
}

// selectedField resolves x.f to the struct field it names, when the struct is
// one this module writes.
func (w *walker) selectedField(selector *ast.SelectorExpr) (*types.Var, bool) {
	selection, ok := w.pkg.TypesInfo.Selections[selector]
	if !ok || selection.Kind() != types.FieldVal {
		return nil, false
	}
	if _, inModule := w.structType(selection.Recv()); !inModule {
		return nil, false
	}
	field, ok := selection.Obj().(*types.Var)
	return field, ok
}

// structType is the struct a value's type is, when that struct is one this
// module writes. Anything from another module is not a surface this repository
// publishes, so it is not judged here.
//
// An anonymous struct is accepted on the strength of where it was found rather
// than of a package on its type: it has no declaring object to ask, and the
// only files walked are this module's. Leaving it out was not neutral.
// internal/tools/integrations keeps its metadata table as a map to an
// anonymous struct, so the rule that asked for a named type read neither the
// table's lists nor its prose, and reported the copy taken from it as a value
// it could not follow.
func (w *walker) structType(typ types.Type) (*types.Struct, bool) {
	if typ == nil {
		return nil, false
	}
	if pointer, ok := types.Unalias(typ).(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	switch resolved := types.Unalias(typ).(type) {
	case *types.Struct:
		return resolved, true
	case *types.Named:
		if resolved.Obj() == nil || resolved.Obj().Pkg() == nil {
			return nil, false
		}
		if !strings.HasPrefix(resolved.Obj().Pkg().Path(), goprogram.ModulePath) {
			return nil, false
		}
		underlying, isStruct := resolved.Underlying().(*types.Struct)
		return underlying, isStruct
	default:
		return nil, false
	}
}

// structFieldType is the declared type of one field of a struct.
func structFieldType(structType *types.Struct, fieldName string) (types.Type, bool) {
	for field := range structType.Fields() {
		if field.Name() == fieldName {
			return field.Type(), true
		}
	}
	return nil, false
}

// recordField routes one field write to the rule that judges it, by the
// field's name and its declared type.
//
// The type is checked as well as the name because the names are common: a
// Description that is not a string, or a Related that is not a list of
// strings, is some other struct's field and not a surface this publishes.
func (w *walker) recordField(fieldName string, fieldType types.Type, value ast.Expr) {
	lowered := strings.ToLower(fieldName)
	if _, isIDList := idListFieldNames[lowered]; isIDList && isStringSlice(fieldType) {
		w.recordIDList(kindRelated, value)
		return
	}
	if kind, isProse := proseFieldNames[lowered]; isProse && isString(fieldType) {
		w.recordProse(kind, value)
	}
}

// isStringSlice reports whether a type is []string.
func isStringSlice(typ types.Type) bool {
	slice, ok := types.Unalias(typ).Underlying().(*types.Slice)
	return ok && isString(slice.Elem())
}

// isString reports whether a type is string.
func isString(typ types.Type) bool {
	basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

// recordIDList records every element of a list of action IDs.
//
// Six shapes reach it. A slice literal and an append are the lists as written.
// A conversion is unwrapped, which is what []string(nil) is. A read of another
// ID-list field is a copy of a list recorded where it was written, so it is
// passed over rather than counted twice or called unfoldable. A call is
// followed into the function's returns, which is what turns a helper such as
// accessTokenRelatedActions into the literals inside it. A variable is
// followed to every value it is ever given, which is what a list assembled
// over a few conditional appends is. Anything else is recorded unresolved
// rather than skipped.
func (w *walker) recordIDList(kind string, value ast.Expr) {
	switch expr := ast.Unparen(value).(type) {
	case *ast.CompositeLit:
		for _, element := range expr.Elts {
			w.recordID(kind, element)
		}
	case *ast.CallExpr:
		w.recordListCall(kind, expr)
	case *ast.SelectorExpr:
		if !w.isIDListRead(expr) {
			w.recordUnresolved(kind, expr)
		}
	case *ast.Ident:
		w.recordListIdent(kind, expr)
	default:
		w.recordUnresolved(kind, value)
	}
}

// recordListIdent records the lists a named variable can hold, and treats a
// nil as the empty list it is.
func (w *walker) recordListIdent(kind string, ident *ast.Ident) {
	if ident.Name == "nil" {
		return
	}
	if w.followValues(kind, ident, recordListValue) {
		return
	}
	w.recordUnresolved(kind, ident)
}

// followValues records every value a named variable can hold, through the rule
// its kind of site is judged by.
//
// A local is followed to what it was assigned; a parameter is followed out to
// the argument every caller passes, which is the shape a handful of packages
// write their specs in: an options builder takes related []string and the
// lists themselves are at its call sites. A parameter's callers are walked in
// their own package, since that is where their arguments resolve.
//
// It reports whether the variable was followed at all, so an identifier this
// index knows nothing about still lands in the unresolved bucket. A variable
// is followed once per run, which is what stops a list that names itself in
// its own append from looping.
func (w *walker) followValues(kind string, ident *ast.Ident, record recordFunc) bool {
	variable, ok := variableOf(w.pkg, ident)
	if !ok {
		return false
	}
	if _, seen := w.visitedVars[variable]; seen {
		return true
	}
	if values, found := w.prog.values[variable]; found {
		w.visitedVars[variable] = struct{}{}
		for _, value := range values {
			record(w, kind, value)
		}
		return true
	}
	return w.followParameter(kind, variable, record)
}

// followParameter records the argument every caller passes for one parameter.
//
// Only a parameter whose own name says it carries related actions is followed,
// and the restriction is not fussiness. A parameter is followed out to every
// call of its function, so following one that is merely a list of strings
// judges whatever any caller ever passes: actioncatalog.cloneStrings takes one
// and is called on a group's aliases, its tags and its validation notes, each
// of which then reads as a cross-link that resolves to nothing. The naming is
// the same convention the field rule leans on, and a parameter outside it is
// reported unfolded rather than guessed at.
func (w *walker) followParameter(kind string, variable *types.Var, record recordFunc) bool {
	param, isParam := w.prog.params[variable]
	if !isParam || !isRelatedParamName(variable.Name()) {
		return false
	}
	w.visitedVars[variable] = struct{}{}
	for _, caller := range w.prog.callers[param.fn] {
		if param.index >= len(caller.call.Args) {
			continue
		}
		record(w.inPackage(caller.pkg), kind, caller.call.Args[param.index])
	}
	return true
}

// isRelatedParamName reports whether a parameter names itself a carrier of
// related actions. The numbered spellings are real: one package takes
// related1 and related2 as two strings rather than a list.
func isRelatedParamName(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "related")
}

// recordFunc is one of the two recording rules, taken as a value so a value
// followed into another package is recorded by the walker that resolves it
// there rather than by the one that asked.
type recordFunc func(w *walker, kind string, expr ast.Expr)

// recordListValue records an expression as a list of action IDs.
func recordListValue(w *walker, kind string, expr ast.Expr) { w.recordIDList(kind, expr) }

// recordSingleValue records an expression as one action ID.
func recordSingleValue(w *walker, kind string, expr ast.Expr) { w.recordID(kind, expr) }

// recordListCall records a list produced by a call: an append, a conversion, a
// copy of a list recorded elsewhere, or a function whose returns are followed.
func (w *walker) recordListCall(kind string, call *ast.CallExpr) {
	if w.isAppend(call) {
		w.recordAppend(kind, call)
		return
	}
	if typed, ok := w.pkg.TypesInfo.Types[call.Fun]; ok && typed.IsType() && len(call.Args) == 1 {
		w.recordIDList(kind, call.Args[0])
		return
	}
	if w.isListCopy(call) {
		return
	}
	if w.followReturns(kind, call) {
		return
	}
	w.recordUnresolved(kind, call)
}

// isListCopy reports whether a call does nothing but hand back a list already
// recorded where it was written, which is what cloneStrings(spec.RelatedActions)
// is. Following such a call into its body finds a loop over a parameter and no
// literal, so recognizing the copy is the difference between a quiet
// pass-through and a site reported as unfoldable.
func (w *walker) isListCopy(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	for _, arg := range call.Args {
		selector, isSelector := ast.Unparen(arg).(*ast.SelectorExpr)
		if !isSelector || !w.isIDListRead(selector) {
			return false
		}
	}
	return true
}

// recordAppend records the arguments of an append: the first is the list being
// grown and is followed back through the same rule, the rest are elements,
// unless the call spreads a slice, in which case that slice is a list too.
func (w *walker) recordAppend(kind string, call *ast.CallExpr) {
	for index, arg := range call.Args {
		spread := call.Ellipsis.IsValid() && index == len(call.Args)-1
		if index == 0 || spread {
			w.recordIDList(kind, arg)
			continue
		}
		w.recordID(kind, arg)
	}
}

// isAppend reports whether a call is the builtin append.
func (w *walker) isAppend(call *ast.CallExpr) bool {
	ident, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := w.pkg.TypesInfo.Uses[ident].(*types.Builtin)
	return ok && builtin.Name() == "append"
}

// isIDListRead reports whether an expression reads another field that is
// itself a list of action IDs, which makes it a copy rather than a new
// publication.
func (w *walker) isIDListRead(selector *ast.SelectorExpr) bool {
	field, ok := w.selectedField(selector)
	if !ok || !isStringSlice(field.Type()) {
		return false
	}
	_, isIDList := idListFieldNames[strings.ToLower(field.Name())]
	return isIDList
}

// followReturns records the lists a called function returns, reading its body
// in the package it was written in.
//
// It reports whether the call was followed at all, so a call into another
// module, or one this loader did not get a body for, still lands in the
// unresolved bucket instead of passing for a clean answer. A function is
// followed once per run: it is the literals inside it that are being
// collected, and following it again would duplicate them and could not
// terminate on a helper that calls itself.
func (w *walker) followReturns(kind string, call *ast.CallExpr) bool {
	callee, ok := w.callee(call)
	if !ok {
		return false
	}
	declared, ok := w.prog.decls[callee]
	if !ok {
		return false
	}
	if _, seen := w.visited[callee]; seen {
		return true
	}
	w.visited[callee] = struct{}{}
	inner := w.inPackage(declared.pkg)
	ast.Inspect(declared.decl.Body, func(node ast.Node) bool {
		ret, isReturn := node.(*ast.ReturnStmt)
		if !isReturn {
			return true
		}
		for _, result := range ret.Results {
			inner.recordIDList(kind, result)
		}
		return true
	})
	return true
}

// recordID folds one expression to the string constant it denotes and records
// it, or records that it could not be folded.
//
// Folding is the type checker's wherever it reaches, which is the reason this
// audit loads a typed program at all: the IDs are written as package-local
// constants, and two packages build one by concatenating a domain prefix onto
// one. Past that, a call of a one-line helper is folded by binding its
// parameters, and a variable is followed to the values it is given.
func (w *walker) recordID(kind string, expr ast.Expr) {
	if value, ok := w.constantString(expr); ok {
		w.addSite(site{Kind: kind, Value: value, Resolved: true}, expr)
		return
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.CallExpr:
		if value, ok := w.foldCall(typed, nil, 0); ok {
			w.addSite(site{Kind: kind, Value: value, Resolved: true}, expr)
			return
		}
	case *ast.Ident:
		if w.followValues(kind, typed, recordSingleValue) {
			return
		}
	}
	w.recordUnresolved(kind, expr)
}

// recordProse folds a model-facing string and records it whole. Its candidate
// IDs are picked out later, because deciding which dotted token is meant as an
// action ID needs the catalog's domains.
func (w *walker) recordProse(kind string, expr ast.Expr) {
	value, ok := w.constantString(expr)
	if !ok {
		// A prose line assembled at run time is not judged and is not a blind
		// spot worth reporting either: it carries no literal ID a reader could
		// have got wrong, and what it renders is whatever it is handed.
		return
	}
	w.addSite(site{Kind: kind, Value: value, Resolved: true}, expr)
}

// recordUnresolved records a site the audit could not fold.
func (w *walker) recordUnresolved(kind string, expr ast.Expr) {
	w.addSite(site{Kind: kind, Expr: types.ExprString(expr)}, expr)
}

// constantString folds an expression to the string it denotes.
func (w *walker) constantString(expr ast.Expr) (string, bool) {
	return constantStringIn(w.pkg, expr)
}

// stringValue reads a constant as the string it denotes, and refuses a
// constant of any other kind rather than rendering one.
func stringValue(value constant.Value) (string, bool) {
	if value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

// addSite stamps a site with the package and position of the expression it
// came from and keeps it.
func (w *walker) addSite(recorded site, expr ast.Expr) {
	position := w.pkg.Fset.Position(expr.Pos())
	recorded.Package = trimModulePath(w.pkg.PkgPath)
	recorded.File = relativePath(position.Filename, w.prog.root)
	recorded.Line = position.Line
	w.sites = append(w.sites, recorded)
}

// trimModulePath names a package the way the repository does.
func trimModulePath(pkgPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(pkgPath, goprogram.ModulePath), "/")
}

// relativePath renders a file below the repository root, so a finding reads
// the same wherever the audit was run from.
func relativePath(path, root string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}
