package main

import (
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// The kinds of site a model-facing capability name is written at, and the one
// it is quoted back at. The first four are the published action IDs; the next
// six are the prose the server hands a model; the last is the e2e suite
// asserting that a served text carries a substring, which is the same prose
// read back rather than written.
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
	// kindMessage is the message of an error a handler returns, the text of a
	// refusal it answers with, and a field named for a message: the sentence a
	// model reads when a call fails or ends, written without a hint helper.
	kindMessage = "message"
	// kindNextStep is a hint written into a result's next-step section, which
	// every formatter reaches through toolutil.WriteHints, the list footer or
	// a card's End.
	kindNextStep = "next_step"
	// kindParamGuidance is the parameter guidance a spec publishes, the
	// ValueSource and CommonConfusions every surface serves beside an action's
	// schema.
	kindParamGuidance = "param_guidance"
	// kindSchemaDescription is the description a jsonschema struct tag gives
	// an input or output field, which every surface serves in the schema.
	kindSchemaDescription = "schema_description"
	// kindAssertion is a substring the e2e suite asserts a served text
	// carries. It is not served, so it is judged in a section of its own; it
	// quotes what is, so it is held to the spellings a hint is held to, and a
	// quotation naming a tool is a test that breaks the day the server is
	// fixed. See suite.go.
	kindAssertion = "assertion"
)

// hintActionFunc is the toolutil helper every cross-link hint is written
// through: toolutil.HintAction(id, purpose) renders "Use action 'id' to
// purpose" into the Markdown a model reads.
const hintActionFunc = "HintAction"

// listHintsFunc is the toolutil helper a list formatter builds its next-step
// hints with: toolutil.ListHints(hint, hint) prepends the preserve-links hint
// to the ones it is given, so each of its arguments is a hint.
const listHintsFunc = "ListHints"

// proseSink is a function that hands a model the prose it is given: the
// argument that prose starts at, and the kind of site it is recorded as.
//
// A format sink is read whole rather than from an argument, because its first
// argument is a format and the ones after it are values the sentence reports
// or prose it is assembled from, which [walker.foldFormat] tells apart.
type proseSink struct {
	first  int
	kind   string
	format bool
}

// proseSinks are the functions that hand a model prose, keyed by the full
// name the type checker gives the function, so a method is keyed by its
// receiver and a local function of the same name is none of them.
//
// Every argument from first on is prose: the hint is the last parameter of
// all three error helpers, WrapErrWithHint(op, err, hint),
// WrapErrWithStatusHint(op, err, code, hint) and NotFoundResult(resource,
// identifier, hints...); the next-step writers take their hints as a variadic
// tail, WriteHints(b, hints...), WriteListFooter(b, p, linked, hints...) and
// Card.End(hints...); and the message is the one argument of errors.New,
// toolutil.ErrorResult and toolutil.CancelledResult.
//
// WriteListFooter and Card.End are sinks of their own although both forward
// to WriteHints, because the forwarding is read only when toolutil is loaded
// from source: a run narrowed to one domain package loads it from export
// data, and would read none of that package's next steps. The route through
// WriteHints reaches the same expressions and records none of them twice.
//
// ErrorResultAnnotated is deliberately not one, because nothing reaches it
// that a sink here does not read already or that is a sentence of the
// server's. Most of its callers hand it rendered Markdown (a not-found card, a
// detailed error, a safe-mode preview), each written through a sink this
// table reads; CancelledResult hands it the refusal a declined confirmation
// answers with, which is read as that function's own argument; and toolutil's
// rate-limit refusal hands it a sentence built around the name of the tool
// the caller called, which is a value the refusal reports. As a sink it would
// add those sites to the ones nothing folds and read no sentence the others
// do not.
var proseSinks = map[string]proseSink{
	toolutilFullName("WrapErrWithHint"):       {first: 2, kind: kindErrorHint},
	toolutilFullName("WrapErrWithStatusHint"): {first: 3, kind: kindErrorHint},
	toolutilFullName("NotFoundResult"):        {first: 2, kind: kindErrorHint},
	toolutilFullName("WriteHints"):            {first: 1, kind: kindNextStep},
	toolutilFullName("WriteListFooter"):       {first: 3, kind: kindNextStep},
	toolutilMethodFullName("Card", "End"):     {first: 0, kind: kindNextStep},
	toolutilFullName("ErrorResult"):           {first: 0, kind: kindMessage},
	toolutilFullName("CancelledResult"):       {first: 0, kind: kindMessage},
	"errors.New":                              {first: 0, kind: kindMessage},
	"fmt.Errorf":                              {first: 0, kind: kindMessage, format: true},
}

// toolutilFullName is the full name the type checker gives a toolutil
// function.
//
// It is a function rather than a concatenation written into the table,
// because a package variable's initializer sits outside every block coverage
// counts, so mutation testing reports each operator in one as uncovered
// however many tests read the table.
func toolutilFullName(name string) string {
	return goprogram.ToolutilPath + "." + name
}

// toolutilMethodFullName is the full name the type checker gives a method on a
// pointer receiver of toolutil, written as a function for the reason
// [toolutilFullName] is.
func toolutilMethodFullName(receiver, name string) string {
	return "(*" + goprogram.ToolutilPath + "." + receiver + ")." + name
}

// formatFuncs are the format functions whose call [walker.foldFormat] folds:
// the one a sink reads whole, and the one a sentence handed to any sink is
// most often built with.
var formatFuncs = map[string]struct{}{
	"fmt.Errorf":  {},
	"fmt.Sprintf": {},
}

// hintNameSuffixes are the endings that make a field or a parameter a carrier
// of hint prose, matched without case.
//
// A suffix rather than a substring, because the substring rule admits
// hintAction and every other name that merely mentions hints. What the tree
// writes is hint, hints, notFoundHint, forbiddenHint, validationHint and
// badRequestHint, and each of those is corrective prose a model reads.
// nextSteps is the same prose under the name the JSON a meta tool returns
// gives it (toolutil.HintableOutput), which a handler fills itself where it
// has something to add, and nextStep the one sentence of it dynamic's search
// result carries.
var hintNameSuffixes = []string{"hint", "hints", "nextstep", "nextsteps"}

// messageNameSuffixes are the endings that make a field or a parameter a
// carrier of a message, matched without case: missingProjectMsg,
// missingUserMsg, emptyMessage and a result's Message.
//
// They are a weaker claim than a hint's name. A message field is as often
// GitLab's own text (a commit's message, a broadcast message) as it is the
// server's, so a write of one is judged where it folds, passed over and
// counted where it reads another struct's field, and listed with the sites
// nothing folds otherwise ([walker.recordMessageField]). A format argument
// named for a message is followed only where it is a parameter, a helper
// handing its caller's sentence on; a local or a field so named is read as a
// value (glMsg is GitLab's message, spelled into a sentence the server writes
// around it).
var messageNameSuffixes = []string{"msg", "message"}

// descriptionNameSuffixes and usageNameSuffixes are the endings that make a
// parameter a carrier of a schema description and of a Usage line, matched
// without case, so a helper handed one is followed out to the callers that
// write it (see [followableParamName]).
var (
	descriptionNameSuffixes = []string{"description"}
	usageNameSuffixes       = []string{"usage"}
)

// guidanceFieldNames are the fields of toolutil.ParameterGuidance that are
// prose a model reads beside an action's schema, matched without case, so the
// parameter a guidance constructor takes under the field's own name
// (DiscussionIDParamGuidance's valueSource) is followed out to its callers.
//
// SemanticRole and ExampleBinding are left out: the first is a token naming
// the parameter's role and the second a binding in params.x:value form, and
// neither is a sentence.
var guidanceFieldNames = map[string]struct{}{
	"valuesource":      {},
	"commonconfusions": {},
}

// jsonschemaTagKey is the struct tag the input and output schemas take their
// field descriptions from, and requiredTagSuffix the marker the schema
// builder strips off one before it serves it (toolutil/meta_tool.go).
// schemaDescriptionKey is the key a schema written as a map gives the same
// text under.
const (
	jsonschemaTagKey     = "jsonschema"
	requiredTagSuffix    = ",required"
	schemaDescriptionKey = "description"
)

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
//
// A site passed over is neither: it is a value a sentence reports rather than
// prose it writes (an ID, a path, GitLab's own message, the error a format
// wraps), which no rule could judge and no list of them could be read. It is
// kept as a site so the report can count what the walk declined to read,
// which is the difference between a decision and a blind spot.
type site struct {
	Package    string
	File       string
	Line       int
	Kind       string
	Value      string
	Expr       string
	Resolved   bool
	PassedOver bool
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
	// ranges maps the value variable of a range over a list of strings to the
	// list it walks, which is every value that variable is given. It is kept
	// apart from values because the expression is the list rather than one
	// value, and is read as a list.
	ranges map[*types.Var]ast.Expr
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

// absolutePath resolves the walk root, swapped in tests. filepath.Abs fails
// only when the process has no working directory, which a test cannot arrange
// and which would otherwise leave the one branch that reports it unexercised.
var absolutePath = filepath.Abs

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
	collect, err := newCollector(dir, loaded)
	if err != nil {
		return nil, err
	}
	collect.walk((*walker).visit)
	return collect.sites, nil
}

// newCollector indexes the loaded packages, rooted at dir, into the collector
// a walk over them writes into.
//
// It is shared by the two loads this command makes, the served tree and the
// e2e suite, which differ in what they load and in what a walk looks for and
// in nothing else: the index, the fold and the recording rules are one, so a
// needle the suite quotes is folded by exactly the machinery that folds the
// hint it quotes.
func newCollector(dir string, loaded []*packages.Package) (*collector, error) {
	root, err := absolutePath(dir)
	if err != nil {
		return nil, err
	}
	return &collector{
		prog:        indexProgram(root, loaded),
		visited:     map[*types.Func]struct{}{},
		visitedVars: map[*types.Var]struct{}{},
		recorded:    map[ast.Expr]struct{}{},
		calls:       map[string]int{},
		mismatches:  map[helperCopy]string{},
		returned:    map[*ast.UnaryExpr]struct{}{},
	}, nil
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
		ranges:  map[*types.Var]ast.Expr{},
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
// declaration, an assignment or a var declaration alike, and the list a range
// statement hands its value variable.
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
	case *ast.RangeStmt:
		p.recordRange(pkg, typed)
	}
}

// recordRange records the list a range over strings walks as what its value
// variable holds.
//
// A filter is written that way (toolutil's withoutPreserveLinks appends each
// element it keeps to the list it returns), and without it the element was a
// value nothing followed: the filter's own body read as a site nothing folds
// wherever a helper handed it hints to follow. Only a list of strings is
// recorded. Over a map the variable is a map's value, and over anything else
// it is not text, and neither is a list a rule here reads.
func (p *program) recordRange(pkg *packages.Package, loop *ast.RangeStmt) {
	ident, named := loop.Value.(*ast.Ident)
	if !named || !isStringSlice(pkg.TypesInfo.TypeOf(loop.X)) {
		return
	}
	variable, ok := variableOf(pkg, ident)
	if !ok {
		return
	}
	p.ranges[variable] = loop.X
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
	// calls counts the calls of each declared assertion helper the suite walk
	// met, and mismatches names the declared parameter a copy of a helper
	// turned out not to take. Both are the suite walk's alone: they are what
	// holds the helper table to the suite, and the served walk leaves them
	// empty.
	calls      map[string]int
	mismatches map[helperCopy]string
	// returned is every unary expression a return statement of the suite
	// hands back, so a predicate negated there is known not to be a claim
	// when the walk reaches it. The suite walk's alone, like the two above.
	returned map[*ast.UnaryExpr]struct{}
}

// walk visits every file of every indexed package with one walker per
// package, all writing into this collector.
func (c *collector) walk(visit func(*walker, ast.Node) bool) {
	for _, pkg := range c.prog.pkgs {
		current := &walker{collector: c, pkg: pkg}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool { return visit(current, node) })
		}
	}
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

// visit dispatches the four shapes a published ID or served prose is written
// in: a call of the hint helper or of a prose sink, a field of a composite
// literal, an assignment to a field, and the struct tag a schema field is
// described by.
func (w *walker) visit(node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.CallExpr:
		w.visitCall(typed)
	case *ast.CompositeLit:
		w.visitCompositeLit(typed)
	case *ast.AssignStmt:
		w.visitAssign(typed)
	case *ast.StructType:
		w.visitStructTags(typed)
	}
	return true
}

// visitCall records what one call publishes: the first argument of
// HintAction, which is a canonical ID, and the prose a sink of
// [proseSinks] hands a model.
func (w *walker) visitCall(call *ast.CallExpr) {
	callee, ok := w.callee(call)
	if !ok {
		return
	}
	if isToolutilFunc(callee, hintActionFunc) {
		// The ID is read without asking whether it is there: HintAction
		// declares it as a required first parameter, so a call this walk
		// reaches has one. A length guard would be a branch no source that
		// type-checks can take, and the run stops on a package that did not.
		w.recordID(kindHint, call.Args[0])
		return
	}
	sink, isSink := proseSinks[callee.FullName()]
	if !isSink {
		return
	}
	if sink.format {
		w.recordErrorHint(sink.kind, call)
		return
	}
	w.recordErrorHintArgs(sink.kind, call, sink.first)
}

// isToolutilFunc reports whether a function is the named one of toolutil, by
// object rather than by the text of a selector.
func isToolutilFunc(callee *types.Func, name string) bool {
	return callee.Pkg() != nil && callee.Pkg().Path() == goprogram.ToolutilPath && callee.Name() == name
}

// visitStructTags records the description the jsonschema tag of each field
// gives it, which is the text every surface serves beside the field in the
// input or output schema.
//
// A tag is a constant by construction, so the site is always resolved; the
// marker the schema builder strips before it serves the description is
// stripped here too, so what is judged is what is served. A field whose tag
// describes nothing is not a site.
func (w *walker) visitStructTags(structType *ast.StructType) {
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		// The unquote answers for a tag the parser accepted, so its error is a
		// branch no source reaches, and the empty text it would leave carries
		// no description, which the test below already turns away.
		text, _ := strconv.Unquote(field.Tag.Value)
		description := reflect.StructTag(text).Get(jsonschemaTagKey)
		if description == "" {
			continue
		}
		w.addSite(site{
			Kind:     kindSchemaDescription,
			Value:    strings.TrimSuffix(description, requiredTagSuffix),
			Resolved: true,
		}, field.Tag)
	}
}

// isStringKeyedMap reports whether a type is a map keyed by a string, the
// shape a JSON schema is written in by hand. A composite literal the type
// checker recorded no type for is no map.
func isStringKeyedMap(typ types.Type) bool {
	if typ == nil {
		return false
	}
	mapType, ok := types.Unalias(typ).Underlying().(*types.Map)
	return ok && isString(mapType.Key())
}

// visitSchemaMap records the description a string-keyed map literal gives,
// which is the text a schema serves beside the property it describes.
//
// A tag is not the only place a served schema is described. An input schema
// override is a map (toolutil.SchemaPropertyOverride("links.url",
// map[string]any{"description": ...})) applied to the one schema every surface
// serves, and toolutil and dynamic build whole schemas out of maps. Reading
// tags alone left all of those out, and one of them still told a model to use
// a meta tool the other two surfaces do not register.
//
// The entry is found by its constant key. Its value is a description only
// when it is text: a map whose key happens to spell description and holds a
// schema of its own, or a parameter's guidance, is a property named
// description rather than one, and is left alone.
func (w *walker) visitSchemaMap(lit *ast.CompositeLit) {
	for _, element := range lit.Elts {
		// Every element of a map literal is a key and a value, since Go
		// accepts no other form for one.
		pair := element.(*ast.KeyValueExpr) //nolint:errcheck,forcetypeassert // a map literal's elements are pairs
		key, isConstant := w.constantString(pair.Key)
		if !isConstant || key != schemaDescriptionKey || !isString(w.pkg.TypesInfo.TypeOf(pair.Value)) {
			continue
		}
		w.recordSchemaMapDescription(pair.Value)
	}
}

// recordSchemaMapDescription records one description a map literal gives, on
// the terms a message field's write is recorded on ([walker.recordMessageField]).
//
// A map keyed by description is as often a GraphQL input carrying GitLab's
// text (securityattributes hands an attribute's own description to a
// mutation that way) as it is a schema, so the two are told apart by what is
// written: a constant is judged, a name is followed to what it is given, and
// a read of another struct's field is a value, passed over and counted.
func (w *walker) recordSchemaMapDescription(value ast.Expr) {
	if selector, isSelector := ast.Unparen(value).(*ast.SelectorExpr); isSelector && w.readsAField(selector) && !w.readsARecordedHint(kindSchemaDescription, selector) {
		w.passOver(kindSchemaDescription, value)
		return
	}
	w.recordErrorHint(kindSchemaDescription, value)
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
// which is where the metadata tables write their lists and their prose, and
// the description a map literal gives a schema property.
func (w *walker) visitCompositeLit(lit *ast.CompositeLit) {
	litType := w.pkg.TypesInfo.Types[lit].Type
	if isStringKeyedMap(litType) {
		w.visitSchemaMap(lit)
		return
	}
	structType, ok := w.structType(litType)
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
	kind, carriesProse := proseFieldKind(fieldName)
	if !carriesProse {
		return
	}
	// Written as ifs rather than a switch, for the reason
	// [followableParamName] gives: a condition in a case clause sits outside
	// every block coverage counts.
	if isStringSlice(fieldType) {
		w.recordHintList(kind, value)
		return
	}
	if !isString(fieldType) {
		return
	}
	if kind == kindMessage {
		w.recordMessageField(value)
		return
	}
	w.recordErrorHint(kind, value)
}

// proseFieldKind is the kind a field's writes are recorded as when its name
// says it carries served prose: a hint, a parameter's guidance, or a message.
//
// The hint name is asked first, since it is the strongest claim of the three:
// a field named for a hint is server prose wherever it is written.
func proseFieldKind(name string) (string, bool) {
	if isHintName(name) {
		return kindHintField, true
	}
	if isGuidanceName(name) {
		return kindParamGuidance, true
	}
	if isMessageName(name) {
		return kindMessage, true
	}
	return "", false
}

// recordMessageField records one write of a field named for a message, where
// it is a sentence the server writes.
//
// A message field holds GitLab's text as often as the server's, and the two
// are told apart by what is written into it rather than by the name: a
// literal, a constant or a format folds and is judged, a name is followed to
// what it is given, and a read of a field this walk records is a copy of
// prose judged where it was written. A read of any other field is a value
// (Message: c.Message is a commit's own, and so is the *u.AwardMessage an
// optional one is dereferenced from), passed over and counted rather than
// listed with the sites nothing folds, since a list of every GitLab message
// the tree copies would be a list nobody reads.
func (w *walker) recordMessageField(value ast.Expr) {
	read := ast.Unparen(value)
	if star, isDeref := read.(*ast.StarExpr); isDeref {
		read = ast.Unparen(star.X)
	}
	if selector, isSelector := read.(*ast.SelectorExpr); isSelector && w.readsAField(selector) && !w.readsARecordedHint(kindMessage, selector) {
		w.passOver(kindMessage, value)
		return
	}
	w.recordErrorHint(kindMessage, value)
}

// readsAField reports whether selector reads a field of a value, which is
// what the two writes above pass over. A package-qualified name
// (Message: otherpkg.Msg) is a selector too, and the type checker records it
// among the uses rather than the selections; read as a field it would be
// passed over although it is a constant the writer chose, which folds and is
// judged like one written in place.
func (w *walker) readsAField(selector *ast.SelectorExpr) bool {
	selection, ok := w.pkg.TypesInfo.Selections[selector]
	return ok && selection.Kind() == types.FieldVal
}

// isHintName reports whether a field or parameter name says it carries hint
// prose, matched without case so one rule covers the exported spelling and the
// unexported one.
func isHintName(name string) bool {
	return hasSuffixFold(name, hintNameSuffixes)
}

// isMessageName reports whether a field or parameter name says it carries a
// message, matched the way [isHintName] matches a hint.
func isMessageName(name string) bool {
	return hasSuffixFold(name, messageNameSuffixes)
}

// isGuidanceName reports whether a field or parameter carries the name of a
// guidance field that is prose.
func isGuidanceName(name string) bool {
	_, isGuidance := guidanceFieldNames[strings.ToLower(name)]
	return isGuidance
}

// hasSuffixFold reports whether a name ends in one of the suffixes, without
// case.
func hasSuffixFold(name string, suffixes []string) bool {
	lowered := strings.ToLower(name)
	for _, suffix := range suffixes {
		if strings.HasSuffix(lowered, suffix) {
			return true
		}
	}
	return false
}

// isHintKind reports whether a site's value is served prose rather than a
// published action ID, which is what decides the rule it is judged by and
// whether it can fail the gate.
func isHintKind(kind string) bool {
	switch kind {
	case kindErrorHint, kindHintField, kindMessage, kindNextStep, kindParamGuidance, kindSchemaDescription:
		return true
	default:
		return false
	}
}

// readsAsHintProse reports whether a site's value is folded as a sentence
// rather than as one action ID: a hint, and the suite's quotation of one.
//
// It is apart from [isHintKind] on purpose. That one decides which section a
// site is judged in and what -fix-hints may rewrite, and a quotation belongs
// to neither: it has a section of its own, and a fixer that rewrote the suite
// would be moving the test to agree with the server rather than holding the
// server to it.
func readsAsHintProse(kind string) bool {
	return isHintKind(kind) || kind == kindAssertion
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
//
// The value variable of a range over a list of strings holds each element of
// that list in turn, so it is followed to the list, read as one: an element
// of a list of hints is a hint, and one of a list of IDs an ID.
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
	if list, walked := w.prog.ranges[variable]; walked {
		w.visitedVars[variable] = struct{}{}
		listRecorder(kind)(w, kind, list)
		return true
	}
	return w.followParameter(kind, variable, record)
}

// listRecorder is how the list a range walks is read, by the kind of site its
// element was reached from: a sentence's kinds read a list of sentences, and
// the published IDs a list of IDs.
//
// A Usage line is a sentence here although [readsAsHintProse] leaves it out:
// that test decides which kinds are the served-prose section's to judge, and
// a Usage line is judged in both sections, but it is folded as prose.
func listRecorder(kind string) recordFunc {
	if kind == kindUsage || readsAsHintProse(kind) {
		return recordHintListValue
	}
	return recordListValue
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
//
// The prose parameter of a sink is the one parameter not followed out,
// although it is followed, in the sense that the walk stops there having
// accounted for it: the sink's own visit reads every call of it
// ([walker.visitCall]), so its callers' arguments are recorded already, under
// the sink's own kind. Following it as well is what a sink body forwarding
// its prose to another sink would do: NotFoundResult hands its hints to
// Card.End and WrapErrWithHint hands its hint to a format, so every argument
// of theirs was reached a second time under the other sink's kind, and which
// kind it kept depended on which package the walk met first.
func (w *walker) followParameter(kind string, variable *types.Var, record recordFunc) bool {
	param, isParam := w.prog.params[variable]
	if !isParam || !followableParamName(kind, variable.Name()) {
		return false
	}
	w.visitedVars[variable] = struct{}{}
	// Which parameter it is is not asked. A carrier's name is a hint's, a
	// message's or a guidance field's, and every sink's parameters by those
	// names are the ones it reads as prose: the one before them is an
	// operation, a resource, a builder or a pagination block.
	if _, isSink := proseSinks[param.fn.FullName()]; isSink {
		return true
	}
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
// action ID is folded whole, a hint and a quotation of one are prose.
func elementRecorder(kind string) recordFunc {
	if readsAsHintProse(kind) {
		return recordHintValue
	}
	return recordSingleValue
}

// followableParamName reports whether a parameter names itself a carrier of
// the value the site publishes: hint prose for a hint site, related actions
// for the rest.
//
// An assertion accepts the hint names and the names the helper table declares
// besides, because what a suite wrapper forwards to one of those helpers is
// the helper's own argument under the helper's own name: a wrapper taking
// substrings and handing them to a helper in a position that asserts is
// followed out to its callers rather than reported as a needle nothing folds.
// A wrapper returning a predicate's answer reaches no follow at all, negated
// or not: an unnegated call is not read, and a negation inside what a return
// hands back is passed over ([walker.markReturnedNegations]), since in both
// the wrapper's callers decide whether the needles are claims (see doc.go).
//
// A message accepts the message names besides the hint names, because that is
// what a helper taking one calls it (missingProjectMsg, missingUserMsg,
// emptyMessage), and parameter guidance the names of its own prose fields,
// because that is what a guidance constructor calls the text it is handed
// (DiscussionIDParamGuidance's valueSource). A schema description accepts a
// name ending in description, which is what a helper building a property's
// schema calls the text its callers hand it (branches'
// branchProtectionAccessLevelSchema), and a Usage line only a name ending in
// usage, since an options builder taking the line calls it that and a Usage
// line is no hint.
//
// It is written as ifs rather than a switch on purpose: a condition in a case
// clause sits outside every block Go's coverage counts, so mutation testing
// reports it uncovered however many tests reach it.
func followableParamName(kind, name string) bool {
	if kind == kindAssertion {
		return isHintName(name) || isAssertionParamName(name)
	}
	if kind == kindMessage {
		return isHintName(name) || isMessageName(name)
	}
	if kind == kindParamGuidance {
		return isHintName(name) || isGuidanceName(name)
	}
	if kind == kindSchemaDescription {
		return isHintName(name) || hasSuffixFold(name, descriptionNameSuffixes)
	}
	if kind == kindUsage {
		return hasSuffixFold(name, usageNameSuffixes)
	}
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
	if w.isListCopy(kind, call) {
		return
	}
	if w.followReturns(kind, call, recordListValue) {
		return
	}
	w.recordUnresolved(kind, call)
}

// isListCopy reports whether a call does nothing but hand back a list already
// recorded where it was written. Following such a call into its body finds a
// loop over a parameter and no literal, so recognizing the copy is the
// difference between a quiet pass-through and a site reported as unfoldable.
//
// Three shapes qualify, and the second is a narrowing rather than a copy:
// cloneStrings(spec.RelatedActions) is handed the list itself, and
// Registry.publishedRelatedActions(entry) is handed the value it hangs off and
// returns the subset one session may be shown. Both are judged where the IDs
// are written, which is the whole reason this is safe: a pass-through can drop
// an ID or respell it, and the declaration it came from is still read. What
// neither shape can prove is that the body adds no ID of its own, so a literal
// written inside one is a hole in this audit rather than a finding. That hole
// was accepted for the copy and is the same size here.
//
// The third shape is a merge. An argument may also be a list parameter named
// as a carrier of this kind of site's values ([walker.carrierParam]), which is
// a list recorded where its callers write it, so beside a list recorded where
// it is written it is followed out to them, and the call is then a merge of
// lists recorded elsewhere. toolutil's ActionRoute.WithRelatedActions is that
// shape, merging the route's own list with the ones it is handed through a
// normalizing helper, and following that helper's body instead found two
// values nothing folds and no ID. A run over the served tree alone never met
// it, since toolutil was not loaded; a run loading toolutil reported it on
// every -check. The merge has the copy's hole too, and it is the one this
// shape adds: a literal the merging body adds of its own is not read.
//
// A carrier counts only beside a recorded list and only once every argument
// has qualified, and nothing is recorded on the way to deciding. A call handed
// carriers alone merges nothing recorded, so it is followed into as before,
// which is where an ID the callee appends to what it was handed is written;
// passing it over dropped that ID without a word. A call that also passes
// something else is followed into for the same reason.
func (w *walker) isListCopy(kind string, call *ast.CallExpr) bool {
	recorded := false
	var carriers []*ast.Ident
	for _, arg := range call.Args {
		if w.carriesRecordedIDList(arg) {
			recorded = true
			continue
		}
		param, followable := w.carrierParam(kind, arg)
		if !followable {
			return false
		}
		carriers = append(carriers, param)
	}
	if !recorded {
		return false
	}
	for _, param := range carriers {
		w.followValues(kind, param, recordListValue)
	}
	return true
}

// carrierParam reports whether an argument is a list parameter whose name says
// it carries this kind of site's values, the one shape [walker.followParameter]
// follows out to the callers that write them.
//
// The list is part of the shape. A scalar carrier holds one value, which
// [walker.followReturns] reaches through the callee's body and records one ID
// at a time; followed from here as a list, each caller's constant was
// reported as a list nothing folds. A variadic parameter is a list in its
// function's signature, so it qualifies.
func (w *walker) carrierParam(kind string, arg ast.Expr) (*ast.Ident, bool) {
	ident, isIdent := ast.Unparen(arg).(*ast.Ident)
	if !isIdent {
		return nil, false
	}
	variable, known := variableOf(w.pkg, ident)
	if !known {
		return nil, false
	}
	if _, isParam := w.prog.params[variable]; !isParam || !isStringSlice(variable.Type()) {
		return nil, false
	}
	return ident, followableParamName(kind, variable.Name())
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
// in the package it was written in, each through the recording rule given: a
// list of IDs for a related list, a list of hints for a next-step section,
// which is what reads a formatter's jobHints(j) or snippetHints(out) whatever
// branch it returns from.
//
// It reports whether the call was followed at all, so a call into another
// module, or one this loader did not get a body for, still lands in the
// unresolved bucket instead of passing for a clean answer. A function is
// followed once per run: it is the literals inside it that are being
// collected, and following it again would duplicate them and could not
// terminate on a helper that calls itself.
func (w *walker) followReturns(kind string, call *ast.CallExpr, record recordFunc) bool {
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
			record(inner, kind, result)
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
		if value, ok := w.foldCall(typed, nil, 0, false); ok {
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
//
// A Usage line is folded as a sentence ([walker.recordUsage]). An individual
// tool's Description is read only where it is a constant, by the published-ID
// rule for its dotted IDs and by the served-prose rule for its tool names
// outside the "See also" clause ([HintReport.judgeDescription]); one
// assembled at run time is neither judged nor listed, and doc.go names this
// as one of its limits.
func (w *walker) recordProse(kind string, expr ast.Expr) {
	if kind == kindUsage {
		w.recordUsage(expr)
		return
	}
	value, ok := w.constantString(expr)
	if !ok {
		return
	}
	w.addSite(site{Kind: kind, Value: value, Resolved: true}, expr)
}

// recordUsage records a Usage line, folded the way a hint is.
//
// A Usage line is judged twice, for its IDs by the published-ID rule and for
// its tool names by the served-prose rule, and both have to read what it
// renders. A constant is most of them. One assembled at run time used to be
// passed over in silence, on the grounds that it carried no literal ID a
// reader could get wrong, which stopped being true the day the tool-name rule
// read the same line: its literal halves are exactly where a tool name is
// spelled. So it is folded as a hint is: a concatenation keeps its literal
// halves, a local is followed to what it is given, a parameter named for a
// Usage line to its callers, a format to its format and constant arguments,
// and a helper that returns one to every branch it returns from
// (ffuserlists' userListUsage). A read of another Usage field is a copy of a
// line recorded where it was written (runners' options.Usage = meta.usage).
// What still folds nowhere, and a value a format reports, is the served-prose
// section's to count, never the published-ID gate's ([classify]): it is a
// sentence a reader can read, not an ID nobody can.
func (w *walker) recordUsage(expr ast.Expr) {
	w.recordErrorHint(kindUsage, expr)
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
//
// A call of a Usage line is followed into the helper it calls, to every
// branch it returns from, and a call of any other kind is not. A Usage line
// is picked by a helper keyed by the action's name (ffuserlists'
// userListUsage), whose branches are constants; the helpers a hint is handed
// through build their sentence from values (levelHint, searchNextStep) or
// escape it (toolutil.EscapeMdTableCell), and following those read the
// escaping as ten sites nothing folds where the call had been one.
func (w *walker) recordErrorHint(kind string, expr ast.Expr) {
	// A hint constructor is asked about before the fold, not after it.
	// toolutil.HintAction is a one-line helper, so binding its parameters
	// renders the whole sentence, and recording that would publish the same ID
	// twice: once here as prose and once at the same call site as the ID the
	// gate refuses.
	if call, isCall := ast.Unparen(expr).(*ast.CallExpr); isCall && w.recordHintCall(kind, call) {
		return
	}
	if value, ok := w.foldProse(kind, expr); ok {
		w.addSite(site{Kind: kind, Value: value, Resolved: true}, expr)
		return
	}
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		if w.followValues(kind, typed, recordHintValue) {
			return
		}
	case *ast.SelectorExpr:
		if w.readsARecordedHint(kind, typed) {
			return
		}
	case *ast.CallExpr:
		if kind == kindUsage && w.followReturns(kind, typed, recordHintValue) {
			return
		}
	}
	w.recordUnresolved(kind, expr)
}

// recordHintCall records the hints a call produces, reporting whether it did.
//
// Three shapes are recognized, and each is a hint accounted for somewhere
// else. toolutil.HintAction composes the one form this whole command exists to
// ask for, and in the served walk its ID is judged as an ID at this same call
// site, so there is nothing the prose rule can add. toolutil.ListHints is a
// list of hints spelled as its arguments. And a call handed nothing but hints
// recorded elsewhere, read off fields or handed on by a parameter named for
// them, is read the way [walker.isListCopy] reads a list of IDs
// ([walker.carriesRecordedHints]).
//
// A quotation is the exception to the first shape. The suite walk dispatches
// through [walker.visitSuite], which never reaches [walker.visitCall], so a
// needle built with HintAction has its ID judged nowhere unless it is judged
// here. It is recorded as the quotation's own site, the ID alone: the purpose
// after it is prose the served walk does not judge either, and binding the
// whole sentence is not possible from the suite, whose load leaves toolutil's
// declarations out, so a fold would report the call as unreadable rather than
// read what it names.
func (w *walker) recordHintCall(kind string, call *ast.CallExpr) bool {
	callee, ok := w.callee(call)
	if ok && callee.Pkg() != nil && callee.Pkg().Path() == goprogram.ToolutilPath {
		switch callee.Name() {
		case hintActionFunc:
			if kind == kindAssertion {
				w.recordErrorHint(kind, call.Args[0])
			}
			return true
		case listHintsFunc:
			w.recordErrorHintArgs(kind, call, 0)
			return true
		}
	}
	return w.carriesRecordedHints(kind, call)
}

// carriesRecordedHints reports whether a call was handed nothing but hints
// recorded where they are written: hints read off fields this walk records,
// and string or list parameters named as carriers of this kind of prose,
// which are followed out to the callers that write them. Nothing is recorded
// or followed until every argument has qualified.
//
// What the call is then depends on what it was handed, on the terms
// [walker.isListCopy] sets for a list of IDs. Handed a recorded read, alone or
// beside carriers, it is a copy or a merge of prose recorded where it was
// written, and passed over with the hole those shapes carry: a sentence the
// body adds of its own is not read. Handed carriers alone it merges nothing
// recorded, so it is followed into as well ([walker.followCarrierCallee]),
// which is where a helper appending a sentence of its own to what it was
// handed writes it; passing it over dropped that sentence without a word.
// toolutil's list footer is that shape, WriteHints(b,
// withoutPreserveLinks(hints)...), and its filter is read through the range
// over what it was handed.
func (w *walker) carriesRecordedHints(kind string, call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	recorded := false
	var carriers []*ast.Ident
	for _, arg := range call.Args {
		if selector, isSelector := ast.Unparen(arg).(*ast.SelectorExpr); isSelector && w.readsARecordedHint(kind, selector) {
			recorded = true
			continue
		}
		param, followable := w.hintCarrierParam(kind, arg)
		if !followable {
			return false
		}
		carriers = append(carriers, param)
	}
	for _, param := range carriers {
		w.followValues(kind, param, hintRecorderFor(w.pkg.TypesInfo.TypeOf(param)))
	}
	if !recorded {
		w.followCarrierCallee(kind, call)
	}
	return true
}

// followCarrierCallee reads what a call handed carriers alone returns, from
// the body of the function it calls.
//
// The carriers are followed already, so what this adds is the callee's own
// prose: a literal it appends to what it was handed, a format it spells the
// carrier into. A callee whose body the load does not hold is another
// module's (strings.TrimSpace), or toolutil read from export data by a run
// narrowed to one domain, and writes no sentence of this repository's; a call
// with no callee to name, of a function value, could do anything with what it
// is handed and is listed with the sites nothing folds.
func (w *walker) followCarrierCallee(kind string, call *ast.CallExpr) {
	if _, named := w.callee(call); !named {
		w.recordUnresolved(kind, call)
		return
	}
	w.followReturns(kind, call, hintRecorderFor(w.pkg.TypesInfo.TypeOf(call)))
}

// hintCarrierParam reports whether an argument is a string or list parameter
// whose name says it carries this kind of prose, the shape
// [walker.followParameter] follows out to the callers that write it.
func (w *walker) hintCarrierParam(kind string, arg ast.Expr) (*ast.Ident, bool) {
	ident, isIdent := ast.Unparen(arg).(*ast.Ident)
	if !isIdent {
		return nil, false
	}
	variable, known := variableOf(w.pkg, ident)
	if !known {
		return nil, false
	}
	if _, isParam := w.prog.params[variable]; !isParam {
		return nil, false
	}
	if !isString(variable.Type()) && !isStringSlice(variable.Type()) {
		return nil, false
	}
	return ident, followableParamName(kind, variable.Name())
}

// hintRecorderFor is the recording rule for a carrier of the given type: a
// list of hints for a list, one hint for a string.
func hintRecorderFor(typ types.Type) recordFunc {
	if isStringSlice(typ) {
		return recordHintListValue
	}
	return recordHintValue
}

// readsARecordedHint reports whether a selector reads a hint field whose
// writes this walk records, which makes the read a copy of prose judged where
// it was written rather than a site of its own.
//
// The suite walk records no field write at all: it reads the calls of the
// assertion helpers and nothing else. So a quotation read off a test table's
// hint field has been recorded nowhere, and treating it as a copy would pass
// it in silence. For an assertion the read is its own site, and reported as
// one when nothing folds it.
func (w *walker) readsARecordedHint(kind string, selector *ast.SelectorExpr) bool {
	return kind != kindAssertion && w.isHintRead(selector)
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
//
// The half a concatenation leaves unfolded is recorded on its own, under the
// kind the whole is recorded as, the way [walker.recordErrorHint] records a
// whole: a name is followed to the values it carries, a read of a hint field
// is left to where the field was written, and anything else is a site nothing
// folds. It is text the reader of the rendered sentence reads, and a value it
// is handed at run time may be the spelling the rule refuses, so dropping it
// left a blind spot that neither the findings nor the unfolded count showed. A
// whole that folds nowhere records nothing here: the caller records it as one
// site.
func (w *walker) foldProse(kind string, expr ast.Expr) (string, bool) {
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
		left, leftFolded := w.foldProse(kind, typed.X)
		right, rightFolded := w.foldProse(kind, typed.Y)
		if !leftFolded && !rightFolded {
			return "", false
		}
		if !leftFolded {
			w.recordErrorHint(kind, typed.X)
		}
		if !rightFolded {
			w.recordErrorHint(kind, typed.Y)
		}
		return left + " " + right, true
	case *ast.CallExpr:
		if w.isFormatCall(typed) {
			return w.foldFormat(kind, typed)
		}
		return w.foldCall(typed, nil, 0, true)
	default:
		return "", false
	}
}

// isFormatCall reports whether a call is one of [formatFuncs].
func (w *walker) isFormatCall(call *ast.CallExpr) bool {
	callee, ok := w.callee(call)
	if !ok {
		return false
	}
	_, isFormat := formatFuncs[callee.FullName()]
	return isFormat
}

// foldFormat folds a call of a format function to the sentence it writes: the
// format as it is written, then every argument that is a constant, each after
// a space.
//
// The format is kept verbatim rather than rendered, for the fixer's sake:
// -fix-hints admits a literal only when its text is contained in a value the
// walk folded, and a format with its verbs filled in contains no literal of
// its own. The verbs are masked where the value is judged instead
// ([maskVerbs]). A constant argument is text the model reads too, so it is
// appended the way a concatenation's halves are joined, which is what reads a
// tool name handed to a sentence as its argument.
//
// Every other argument is one of two things. A name that says it carries hint
// prose, a parameter named for a message where this kind follows one, a read
// of a field this walk records, or, in a Usage line alone, a call of a helper
// declared in the load, is prose the sentence is assembled from, and is
// followed or left to where it is written, as a hint is. Anything else
// is a value the sentence reports: an ID, a path, a count, GitLab's own
// message, the error it wraps. Those are passed over and counted, which is a
// deliberate exception to the rule that a blind spot is listed: a format is
// how a handler reports what it was given, and with some four hundred of them
// in the served tree the list would be GitLab data from end to end. A local
// or a field named for a message is a value here although it would be
// followed as a message's argument, because in a format it is almost always
// GitLab's message spelled into a sentence the server writes around it
// (glMsg); a parameter so named is the server's own sentence handed on by a
// helper, and passing it over would leave every caller's sentence unread. No
// helper in the tree spells one into a format today (files' missingProjectMsg
// and projects' missingUserMsg go to errors.New), so the shape is the one the
// fixture pins, requireProject(op, missingProjectMsg).
//
// A format that does not fold is a sentence the walk cannot read, and the
// call is left to be recorded as one site nothing folds.
func (w *walker) foldFormat(kind string, call *ast.CallExpr) (string, bool) {
	// A format function takes its format first, so a call that type-checks has
	// one argument at least.
	format, ok := w.foldProse(kind, call.Args[0])
	if !ok {
		return "", false
	}
	parts := []string{format}
	for _, arg := range call.Args[1:] {
		if value, isConstant := w.constantString(arg); isConstant {
			parts = append(parts, value)
			continue
		}
		if w.formatArgIsProse(kind, arg) {
			continue
		}
		w.passOver(kind, arg)
	}
	return strings.Join(parts, " "), true
}

// formatArgIsProse reports whether one argument of a format is prose the
// sentence is assembled from rather than a value it reports, recording it
// where it is: a name that says it carries a hint, or a parameter named for a
// message where this kind follows one, is followed to what it is given, a
// read of a field this walk records is a copy of prose recorded where it was
// written, and a call of a sink is prose its own visit reads.
//
// The sink is not a nicety. An error wrapped in the error it causes,
// fmt.Errorf("...: %w", fmt.Errorf("...")), is visited outer first, and the
// inner call is the very expression its own visit records, so passing it over
// here would record it first and leave the inner sentence unread.
//
// A Usage line's format is the one kind whose other calls are prose as well:
// a call of a function declared in the load is followed to every branch it
// returns from, as a whole Usage line handed to such a helper is
// ([walker.recordErrorHint]). badges assembles its twelve lines from a format
// whose last argument is badgeScopeBoundary(scope), a helper returning one of
// two sentences, and both named a meta tool while the call was passed over as
// a value the line reports. A hint's or a message's format is left alone,
// since its calls are the values it reports (err.Error(), a joined list, an
// escaped field), and following those would read toolutil's escaping as
// sentences nothing folds, which is why a whole hint is not followed either.
func (w *walker) formatArgIsProse(kind string, arg ast.Expr) bool {
	switch typed := ast.Unparen(arg).(type) {
	case *ast.Ident:
		if !isHintName(typed.Name) && !w.isMessageParam(kind, typed) {
			return false
		}
		w.recordErrorHint(kind, typed)
		return true
	case *ast.SelectorExpr:
		return w.readsARecordedHint(kind, typed)
	case *ast.CallExpr:
		callee, ok := w.callee(typed)
		if !ok {
			return false
		}
		if _, isSink := proseSinks[callee.FullName()]; isSink {
			return true
		}
		return kind == kindUsage && w.followReturns(kind, typed, recordHintValue)
	default:
		return false
	}
}

// isMessageParam reports whether a name is a parameter named for a message
// that this kind of site follows out to its callers.
//
// The parameter is the whole distinction. A helper taking missingProjectMsg
// and spelling it into a format hands on a sentence each of its callers
// wrote, which is prose; a local or a field of the same name holds what a
// response carried (glMsg), which is a value. A name that is no variable at
// all is no parameter either, which the lookup answers without a question of
// its own: variableOf yields nil for it, and nil indexes no parameter.
func (w *walker) isMessageParam(kind string, ident *ast.Ident) bool {
	if !isMessageName(ident.Name) || !followableParamName(kind, ident.Name) {
		return false
	}
	variable, _ := variableOf(w.pkg, ident)
	_, isParam := w.prog.params[variable]
	return isParam
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
		if !w.readsARecordedHint(kind, expr) {
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
// being grown and whose rest are elements; a conversion, which is the list it
// converts ([]string(nil) is where a copy of a guidance list starts); or one
// of the shapes [walker.recordHintCall] recognizes, which is where
// toolutil.ListHints lands.
func (w *walker) recordHintListCall(kind string, call *ast.CallExpr) {
	switch {
	case w.isBuiltin(call, "make"):
	case w.isAppend(call):
		w.recordHintAppend(kind, call)
	case w.pkg.TypesInfo.Types[call.Fun].IsType():
		// A conversion that compiles takes exactly one operand, as in
		// [walker.recordListCall].
		w.recordHintList(kind, call.Args[0])
	case w.recordHintCall(kind, call):
	case w.followReturns(kind, call, recordHintListValue):
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

// isHintRead reports whether an expression reads a field that is itself
// served prose, a hint, a message or a parameter's guidance, which makes it a
// copy of prose recorded where it was written rather than a new one.
func (w *walker) isHintRead(selector *ast.SelectorExpr) bool {
	field, ok := w.selectedField(selector)
	if !ok {
		return false
	}
	if !isString(field.Type()) && !isStringSlice(field.Type()) {
		return false
	}
	_, isProse := proseFieldKind(field.Name())
	return isProse || proseFieldNames[strings.ToLower(field.Name())] == kindUsage
}

// recordUnresolved records a site the audit could not fold.
func (w *walker) recordUnresolved(kind string, expr ast.Expr) {
	w.addSite(site{Kind: kind, Expr: types.ExprString(expr)}, expr)
}

// passOver records a value a sentence reports rather than prose it writes,
// which is counted and not judged; see [site].
func (w *walker) passOver(kind string, expr ast.Expr) {
	w.addSite(site{Kind: kind, Expr: types.ExprString(expr), PassedOver: true}, expr)
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
