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

// The kinds of site a model-facing capability name is written at. The first
// four are the published action IDs the gate refuses; the last two are the
// corrective prose the staged hint rule reports.
const (
	kindRelated     = "related"
	kindHint        = "hint"
	kindUsage       = "usage"
	kindDescription = "description"
	// kindErrorHint is a hint argument of one of the error helpers, and
	// kindHintField the struct field such a hint is written into on its way to
	// one. They are counted apart because the second is a wider net than the
	// argument rule alone: a field named for a hint is judged wherever it is
	// written, and telling a field that reaches an error helper from one that
	// reaches a Markdown formatter would need dataflow this walk does not do.
	kindErrorHint = "error_hint"
	kindHintField = "hint_field"
)

// hintActionFunc is the toolutil helper every cross-link hint is written
// through: toolutil.HintAction(id, purpose) renders "Use action 'id' to
// purpose" into the Markdown a model reads.
const hintActionFunc = "HintAction"

// listHintsFunc is the toolutil helper a list formatter builds its next-step
// hints with: toolutil.ListHints(hint, hint) prepends the preserve-links hint
// to the ones it is given, so each of its arguments is a hint.
const listHintsFunc = "ListHints"

// errorHintArgs are the toolutil helpers that hand a model corrective prose
// when a call fails, each mapped to the argument that prose starts at.
//
// The hint is the last parameter of all three, so every argument from that
// index on is hint text: WrapErrWithHint(op, err, hint),
// WrapErrWithStatusHint(op, err, code, hint) and NotFoundResult(resource,
// identifier, hints...).
var errorHintArgs = map[string]int{
	"WrapErrWithHint":       2,
	"WrapErrWithStatusHint": 3,
	"NotFoundResult":        2,
}

// hintNameSuffixes are the endings that make a field or a parameter a carrier
// of hint prose, matched without case.
//
// A suffix rather than a substring, because the substring rule admits
// hintAction and every other name that merely mentions hints. What the tree
// writes is hint, hints, notFoundHint, forbiddenHint, validationHint and
// badRequestHint, and each of those is corrective prose a model reads.
var hintNameSuffixes = []string{"hint", "hints"}

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
	// variadic says this is the last parameter of a variadic signature, whose
	// callers spell its elements one by one rather than passing the list.
	variadic bool
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
// absolutePath resolves the walk root, swapped in tests. filepath.Abs fails
// only when the process has no working directory, which a test cannot arrange
// and which would otherwise leave the one branch that reports it unexercised.
var absolutePath = filepath.Abs

func collectSites(dir string, patterns []string, overlay map[string][]byte) ([]site, error) {
	loaded, err := goprogram.Load(dir, patterns, overlay)
	if err != nil {
		return nil, err
	}
	root, err := absolutePath(dir)
	if err != nil {
		return nil, err
	}
	prog := indexProgram(root, loaded)
	collect := &collector{
		prog:        prog,
		visited:     map[*types.Func]struct{}{},
		visitedVars: map[*types.Var]struct{}{},
		recorded:    map[ast.Expr]struct{}{},
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
		// A function's type is a signature, so the assertion reads the value
		// and asks nothing.
		signature := obj.Type().(*types.Signature) //nolint:errcheck,forcetypeassert // a *types.Func is always a signature
		for index := range signature.Params().Len() {
			p.params[signature.Params().At(index)] = paramRef{
				fn:       obj,
				index:    index,
				variadic: signature.Variadic() && index == signature.Params().Len()-1,
			}
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
	// recorded is every expression already turned into a site, so one
	// expression is one site however many routes reach it. Two do: a call is
	// visited where it is written, and it is visited again when a parameter of
	// the function it calls is followed back out to its callers. A helper that
	// forwards its own hint to another helper puts every one of its callers'
	// hints on that second path, which is what toolutil.WrapErrWithStatusHint
	// does to WrapErrWithHint.
	recorded map[ast.Expr]struct{}
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

// visitCall records what one call of a toolutil helper publishes: the first
// argument of HintAction, which is a canonical ID, and the hint arguments of
// the three error helpers, which are prose.
func (w *walker) visitCall(call *ast.CallExpr) {
	callee, ok := w.callee(call)
	if !ok || callee.Pkg() == nil || callee.Pkg().Path() != goprogram.ToolutilPath {
		return
	}
	if callee.Name() == hintActionFunc {
		// The ID is read without asking whether it is there: HintAction
		// declares it as a required first parameter, so a call this walk
		// reaches has one. A length guard would be a branch no source that
		// type-checks can take, and the run stops on a package that did not.
		w.recordID(kindHint, call.Args[0])
		return
	}
	if first, isErrorHint := errorHintArgs[callee.Name()]; isErrorHint {
		w.recordErrorHintArgs(kindErrorHint, call, first)
	}
}

// recordErrorHintArgs records the corrective prose one call hands a model,
// from the argument the hint starts at to the end.
//
// A spread passes the hints as one slice rather than as elements, so the last
// argument is then a list: badges assembles its hints in a local and hands
// them to NotFoundResult that way.
func (w *walker) recordErrorHintArgs(kind string, call *ast.CallExpr, first int) {
	for index := first; index < len(call.Args); index++ {
		// The spread is asked about and the position is not: Go lets only the
		// last argument carry the ellipsis, and a spread call passes exactly
		// one expression for the whole variadic part, which starts where the
		// hints start. So every index this loop visits on a spread call is the
		// last one, and testing for it is a condition no call can make false.
		if call.Ellipsis.IsValid() {
			w.recordHintList(kind, call.Args[index])
			continue
		}
		w.recordErrorHint(kind, call.Args[index])
	}
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
		// The key is read without asking whether it is an identifier or whether
		// the struct has such a field: a keyed literal of a struct type names
		// its fields by identifier, and a key naming no field of that struct
		// does not compile. What is asked, once per field, is which field the
		// key names, and that comparison is false for every other field of the
		// struct.
		key := pair.Key.(*ast.Ident) //nolint:errcheck,forcetypeassert // a struct literal's keys are field names
		for field := range structType.Fields() {
			if field.Name() == key.Name {
				w.recordField(key.Name, field.Type(), pair.Value)
				break
			}
		}
	}
}

// recordPositionalField records one element of a struct literal written
// without field names, which names its fields by order.
//
// The index is used without being bounded, for the same reason the ID of a
// HintAction call is read without being counted: Go demands one element per
// field of a positional literal, so a literal that type-checks has no element
// past the last field and a guard here would answer a question no source can
// ask.
func (w *walker) recordPositionalField(structType *types.Struct, index int, value ast.Expr) {
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
		// A named type always has an object; what is asked is whether that
		// object belongs to a package, since the universe's named types
		// (error and comparable) belong to none.
		if resolved.Obj().Pkg() == nil {
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
		return
	}
	if !isHintName(fieldName) {
		return
	}
	switch {
	case isString(fieldType):
		w.recordErrorHint(kindHintField, value)
	case isStringSlice(fieldType):
		w.recordHintList(kindHintField, value)
	}
}

// isHintName reports whether a field or parameter name says it carries hint
// prose, matched without case so one rule covers the exported spelling and the
// unexported one.
func isHintName(name string) bool {
	lowered := strings.ToLower(name)
	for _, suffix := range hintNameSuffixes {
		if strings.HasSuffix(lowered, suffix) {
			return true
		}
	}
	return false
}

// isHintKind reports whether a site's value is hint prose rather than a
// published action ID, which is what decides the rule it is judged by and
// whether it can fail the gate.
func isHintKind(kind string) bool {
	return kind == kindErrorHint || kind == kindHintField
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
// Only a parameter whose own name says it carries the kind of value the site
// publishes is followed, and the restriction is not fussiness. A parameter is
// followed out to every call of its function, so following one that is merely
// a list of strings judges whatever any caller ever passes:
// actioncatalog.cloneStrings takes one and is called on a group's aliases, its
// tags and its validation notes, each of which then reads as a cross-link that
// resolves to nothing. The naming is the same convention the field rule leans
// on, and a parameter outside it is reported unfolded rather than guessed at.
//
// A variadic parameter is read the way its callers spell it, which is two
// different things: a call that spreads a slice passes the whole list, and a
// call that spells its elements passes one value each. Reading the second as
// the first is what the ID rule got away with, since every variadic list of
// IDs in the tree is spread, and it is not what a list of hints is written as.
func (w *walker) followParameter(kind string, variable *types.Var, record recordFunc) bool {
	param, isParam := w.prog.params[variable]
	if !isParam || !followableParamName(kind, variable.Name()) {
		return false
	}
	w.visitedVars[variable] = struct{}{}
	for _, caller := range w.prog.callers[param.fn] {
		w.recordArguments(kind, param, caller, record)
	}
	return true
}

// recordArguments records what one caller passes for a followed parameter.
func (w *walker) recordArguments(kind string, param paramRef, caller callSite, record recordFunc) {
	inner := w.inPackage(caller.pkg)
	args := caller.call.Args
	if param.index >= len(args) {
		return
	}
	if !param.variadic {
		record(inner, kind, args[param.index])
		return
	}
	for index := param.index; index < len(args); index++ {
		// As in [walker.recordErrorHintArgs]: a spread call carries one
		// expression for the whole variadic part, which begins at this
		// parameter, so the position needs no test of its own.
		if caller.call.Ellipsis.IsValid() {
			record(inner, kind, args[index])
			continue
		}
		elementRecorder(kind)(inner, kind, args[index])
	}
}

// elementRecorder is how one element of a list of this kind is recorded: an
// action ID is folded whole, a hint is prose.
func elementRecorder(kind string) recordFunc {
	if isHintKind(kind) {
		return recordHintValue
	}
	return recordSingleValue
}

// followableParamName reports whether a parameter names itself a carrier of
// the value the site publishes: hint prose for a hint site, related actions
// for the rest.
func followableParamName(kind, name string) bool {
	if isHintKind(kind) {
		return isHintName(name)
	}
	return isRelatedParamName(name)
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

// recordHintListValue records an expression as a list of hints.
func recordHintListValue(w *walker, kind string, expr ast.Expr) { w.recordHintList(kind, expr) }

// recordHintValue records an expression as one hint.
func recordHintValue(w *walker, kind string, expr ast.Expr) { w.recordErrorHint(kind, expr) }

// recordListCall records a list produced by a call: an append, a conversion, a
// copy of a list recorded elsewhere, or a function whose returns are followed.
func (w *walker) recordListCall(kind string, call *ast.CallExpr) {
	if w.isAppend(call) {
		w.recordAppend(kind, call)
		return
	}
	// A conversion is recognized by what its callee denotes, and by nothing
	// else: the zero TypeAndValue a missing entry yields is not a type, and a
	// conversion that compiles takes exactly one operand, so neither the
	// lookup nor the argument count is a question this can answer twice.
	if w.pkg.TypesInfo.Types[call.Fun].IsType() {
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
// recorded where it was written. Following such a call into its body finds a
// loop over a parameter and no literal, so recognizing the copy is the
// difference between a quiet pass-through and a site reported as unfoldable.
//
// Two shapes qualify, and the second is a narrowing rather than a copy:
// cloneStrings(spec.RelatedActions) is handed the list itself, and
// Registry.publishedRelatedActions(entry) is handed the value it hangs off and
// returns the subset one session may be shown. Both are judged where the IDs
// are written, which is the whole reason this is safe: a pass-through can drop
// an ID or respell it, and the declaration it came from is still read. What
// neither shape can prove is that the body adds no ID of its own, so a literal
// written inside one is a hole in this audit rather than a finding. That hole
// was accepted for the copy and is the same size here.
func (w *walker) isListCopy(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	for _, arg := range call.Args {
		if !w.carriesRecordedIDList(arg) {
			return false
		}
	}
	return true
}

// carriesRecordedIDList reports whether one argument of a call is an ID list
// this walk records where it is written, or a value carrying one.
//
// The second half is deliberately narrow: a bare name whose type is a struct
// of this module with an ID-list field of its own. Anything looser would
// silence a call that was handed nothing to do with action IDs and returned a
// list of them.
func (w *walker) carriesRecordedIDList(arg ast.Expr) bool {
	switch expr := ast.Unparen(arg).(type) {
	case *ast.SelectorExpr:
		return w.isIDListRead(expr)
	case *ast.Ident:
		structType, ok := w.structType(w.pkg.TypesInfo.TypeOf(expr))
		return ok && hasIDListField(structType)
	default:
		return false
	}
}

// hasIDListField reports whether a struct declares a field this walk reads as
// a list of canonical action IDs.
func hasIDListField(structType *types.Struct) bool {
	for field := range structType.Fields() {
		if _, isIDList := idListFieldNames[strings.ToLower(field.Name())]; isIDList && isStringSlice(field.Type()) {
			return true
		}
	}
	return false
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
	return w.isBuiltin(call, "append")
}

// isBuiltin reports whether a call is the named builtin, resolved through the
// type checker so a local function of the same name cannot fake one.
func (w *walker) isBuiltin(call *ast.CallExpr, name string) bool {
	ident, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := w.pkg.TypesInfo.Uses[ident].(*types.Builtin)
	return ok && builtin.Name() == name
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

// recordErrorHint records one hint string, folding what it can of a sentence
// assembled at run time.
//
// A hint is prose rather than an ID, so the interesting half is the sentence
// nothing folds whole: dorametrics writes "... omit environment_tiers unless
// the " + scope + " has ...", and a capability name would be spelled in one of
// those literal halves or nowhere. [walker.foldProse] keeps the halves. A name
// that is read out of a field or followed to the values a local carries is
// recorded where it was written instead, and anything left is reported rather
// than passed over, like every other site here.
func (w *walker) recordErrorHint(kind string, expr ast.Expr) {
	// A hint constructor is asked about before the fold, not after it.
	// toolutil.HintAction is a one-line helper, so binding its parameters
	// renders the whole sentence, and recording that would publish the same ID
	// twice: once here as prose and once at the same call site as the ID the
	// gate refuses.
	if call, isCall := ast.Unparen(expr).(*ast.CallExpr); isCall && w.recordHintCall(kind, call) {
		return
	}
	if value, ok := w.foldProse(expr); ok {
		w.addSite(site{Kind: kind, Value: value, Resolved: true}, expr)
		return
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		if w.followValues(kind, typed, recordHintValue) {
			return
		}
	case *ast.SelectorExpr:
		if w.isHintRead(typed) {
			return
		}
	}
	w.recordUnresolved(kind, expr)
}

// recordHintCall records the hints a call produces, reporting whether it did.
//
// Three shapes are recognized, and each is a hint accounted for somewhere
// else. toolutil.HintAction composes the one form this whole command exists to
// ask for, and its ID is judged as an ID at this same call site, so there is
// nothing the prose rule can add. toolutil.ListHints is a list of hints
// spelled as its arguments. And a call handed nothing but hints read off
// fields hands back prose recorded where it was written, which is the bargain
// [walker.isListCopy] already makes for a list of IDs, with the same hole:
// what such a body adds of its own is invisible here.
func (w *walker) recordHintCall(kind string, call *ast.CallExpr) bool {
	callee, ok := w.callee(call)
	if ok && callee.Pkg() != nil && callee.Pkg().Path() == goprogram.ToolutilPath {
		switch callee.Name() {
		case hintActionFunc:
			return true
		case listHintsFunc:
			w.recordErrorHintArgs(kind, call, 0)
			return true
		}
	}
	return w.carriesRecordedHints(call)
}

// carriesRecordedHints reports whether a call was handed nothing but hints
// read off fields this walk records where they are written.
func (w *walker) carriesRecordedHints(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	for _, arg := range call.Args {
		selector, isSelector := ast.Unparen(arg).(*ast.SelectorExpr)
		if !isSelector || !w.isHintRead(selector) {
			return false
		}
	}
	return true
}

// foldProse folds an expression to the prose it renders, keeping the literal
// halves of a sentence assembled at run time.
//
// The type checker answers whole for a literal, a constant and a concatenation
// of constants, which is most of them. Past that, only a concatenation is
// folded, and it is folded to its literal halves with a space where the value
// goes: a space rather than nothing, so two halves cannot be joined into a
// token neither of them spells. A call is folded the way an ID is, by binding
// a one-line helper's parameters.
func (w *walker) foldProse(expr ast.Expr) (string, bool) {
	if value, ok := w.constantString(expr); ok {
		return value, true
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.BinaryExpr:
		// The operator is not asked about, unlike in [walker.foldExpr], which
		// folds arguments of any type and so meets arithmetic. A hint is a
		// string, and the only binary operator a string expression can carry
		// is a concatenation: every other one yields a bool, which no hint
		// parameter accepts.
		left, leftFolded := w.foldProse(typed.X)
		right, rightFolded := w.foldProse(typed.Y)
		if !leftFolded && !rightFolded {
			return "", false
		}
		return left + " " + right, true
	case *ast.CallExpr:
		return w.foldCall(typed, nil, 0)
	default:
		return "", false
	}
}

// recordHintList records every hint of a list of them: the []string a
// not-found output carries, and the slice a spread hands NotFoundResult.
//
// A make is the empty list it allocates and publishes no prose, so it is
// passed over rather than reported; an append is followed through both halves;
// a name is followed to the values it is given; a read of another hint field
// is a copy of a list recorded where it was written. Anything else is
// reported.
func (w *walker) recordHintList(kind string, value ast.Expr) {
	switch expr := ast.Unparen(value).(type) {
	case *ast.CompositeLit:
		for _, element := range expr.Elts {
			w.recordErrorHint(kind, element)
		}
	case *ast.CallExpr:
		w.recordHintListCall(kind, expr)
	case *ast.SelectorExpr:
		if !w.isHintRead(expr) {
			w.recordUnresolved(kind, expr)
		}
	case *ast.Ident:
		if expr.Name == "nil" {
			return
		}
		if !w.followValues(kind, expr, recordHintListValue) {
			w.recordUnresolved(kind, expr)
		}
	default:
		w.recordUnresolved(kind, value)
	}
}

// recordHintListCall records a list of hints produced by a call: a make, which
// allocates and carries no prose; an append, whose first argument is the list
// being grown and whose rest are elements; or one of the shapes
// [walker.recordHintCall] recognizes, which is where toolutil.ListHints lands.
func (w *walker) recordHintListCall(kind string, call *ast.CallExpr) {
	switch {
	case w.isBuiltin(call, "make"):
	case w.isAppend(call):
		w.recordHintAppend(kind, call)
	case w.recordHintCall(kind, call):
	default:
		w.recordUnresolved(kind, call)
	}
}

// recordHintAppend records the arguments of an append: the first is the list
// being grown and is followed back through the same rule, the rest are hints,
// unless the call spreads a slice, in which case that slice is a list too.
func (w *walker) recordHintAppend(kind string, call *ast.CallExpr) {
	for index, arg := range call.Args {
		spread := call.Ellipsis.IsValid() && index == len(call.Args)-1
		if index == 0 || spread {
			w.recordHintList(kind, arg)
			continue
		}
		w.recordErrorHint(kind, arg)
	}
}

// isHintRead reports whether an expression reads a field that is itself hint
// prose, which makes it a copy of a hint recorded where it was written rather
// than a new one.
func (w *walker) isHintRead(selector *ast.SelectorExpr) bool {
	field, ok := w.selectedField(selector)
	if !ok {
		return false
	}
	if !isString(field.Type()) && !isStringSlice(field.Type()) {
		return false
	}
	return isHintName(field.Name())
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
// came from and keeps it, once per expression.
func (w *walker) addSite(recorded site, expr ast.Expr) {
	if _, seen := w.recorded[expr]; seen {
		return
	}
	w.recorded[expr] = struct{}{}
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
