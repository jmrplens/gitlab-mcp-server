package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// clientGoPath is the import path of the SDK whose request options this gate
// reads. It is matched on the path the type checker resolves, never on the
// name a file imports it under, so `gl`, `gitlab` and any other alias are the
// same package here.
const clientGoPath = "gitlab.com/gitlab-org/api/client-go/v3"

// retryablePath is the import path of the request type client-go's request
// builders return, whose own WithContext method is the other way a request
// can be given the caller's context after it was built. Its own constructors
// are judged as well: (*gl.Client).Do sends whatever request it is handed,
// and one this package built from context.Background() reaches GitLab through
// it as surely as a builder's does.
const retryablePath = "github.com/hashicorp/go-retryablehttp"

// The reasons a call is reported, one per way the context fails to arrive.
const (
	// reasonMissing is a call none of whose request options carries a
	// context, which client-go answers by building the request from
	// context.Background().
	reasonMissing = "passes no gl.WithContext option"
	// reasonDetached is a call one of whose options is gl.WithContext of a
	// context that never ends, whatever else it passes:
	// gl.WithContext(context.Background()) or context.TODO() compiles, reads
	// like the fix, and bounds nothing, and client-go applies the options in
	// order, so one placed after the caller's context replaces it.
	reasonDetached = "passes gl.WithContext a context that never ends"
	// reasonUnbound is a request built by hand with no context among its
	// options and never rebound to one.
	reasonUnbound = "builds a request without the caller's context and never rebinds it to one"
)

// Finding is one call that reaches client-go without the caller's context.
type Finding struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	// Func is the declaration the call sits in, as the file spells it: the
	// bare name, `Type.Method` for a method, or the first variable of a
	// package-level declaration, which is where a function literal assigned
	// at package scope lives. A call inside a closure belongs to the
	// declaration around the closure, which is what a reader of the file
	// would name.
	Func string `json:"func"`
	// Callee is the called expression as written, so a finding names the
	// service method the way the source spells it.
	Callee string `json:"callee"`
	Reason string `json:"reason"`
}

// outcome is what the request options handed to one call were seen to do with
// its context, as a set: the options of one call, the elements of a literal,
// the arguments of an append and the assignments of a variable are combined by
// union. A set rather than a rank, because a context that never ends is not a
// weaker form of one that can: client-go applies the options in order and a
// later WithContext replaces an earlier one, so a detached option beside the
// caller's context may be the one the request ends up with, and taking the
// better of the two would pass exactly that call.
type outcome uint8

// outcomeMissing: nothing among the options carries a context.
const outcomeMissing outcome = 0

const (
	// outcomeCarried: one of the options is gl.WithContext of a context that
	// can end.
	outcomeCarried outcome = 1 << iota
	// outcomeForwarded: one of the options is a request-option parameter of
	// the enclosing function, handed on, so whoever calls that function is
	// judged instead.
	outcomeForwarded
	// outcomeDetached: one of the options is gl.WithContext of a context that
	// never ends.
	outcomeDetached
)

// has reports whether an outcome includes another.
func (o outcome) has(other outcome) bool {
	return o&other != 0
}

// scanner accumulates what one load of the tree says about its client-go
// calls.
type scanner struct {
	root string
	// overlay is the source a test supplies instead of the disk, consulted
	// when a file the load left out is read for its imports.
	overlay  map[string][]byte
	findings []Finding
	// unjudged are the files a load left out that import client-go: a build
	// constraint this load did not satisfy hid every call in them, and a gate
	// that said nothing about that would be clean over source it never read.
	unjudged []string
	// packages are the packages this run looked at, named the way the
	// repository names them. The declaration table is held against this set,
	// so a run over one package does not report every declaration elsewhere
	// as stale.
	packages map[string]struct{}
	// calls counts every call judged, which is the population the findings
	// are drawn from: those that reached a request-option parameter, and the
	// request constructors of go-retryablehttp.
	calls int
	// forwarded and rebound count the calls accepted by the two rules that
	// are not an option in the call itself, so a reader can see how much of
	// the clean answer rests on each.
	forwarded int
	rebound   int
}

// newScanner returns a scanner rooted at an absolute repository path.
func newScanner(root string, overlay map[string][]byte) *scanner {
	return &scanner{root: root, overlay: overlay, packages: map[string]struct{}{}}
}

// observe judges every call in every package of one load.
func (s *scanner) observe(loaded []*packages.Package) {
	for _, pkg := range loaded {
		name := trimModulePath(pkg.PkgPath)
		s.packages[name] = struct{}{}
		s.noteUnjudged(pkg)
		facts := collectFacts(pkg)
		for _, file := range pkg.Syntax {
			s.observeFile(pkg, name, facts, file)
		}
	}
}

// noteUnjudged records the files this load left out that import client-go.
//
// Only the imports are read, which is all the question needs: whether a
// hidden file could hold a call this gate would judge. A file that cannot be
// read is recorded too, since nothing can be said about what it calls. A test
// file is passed over whatever its constraints say, because the gate reads no
// test file under any constraint: naming one here would report the stated
// blind spot as though a build tag had caused it.
func (s *scanner) noteUnjudged(pkg *packages.Package) {
	for _, path := range pkg.IgnoredFiles {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		var src any
		if content, overlaid := s.overlay[path]; overlaid {
			src = content
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err == nil && !importsClientGo(file) {
			continue
		}
		s.unjudged = append(s.unjudged, relativePath(path, s.root))
	}
}

// importsClientGo reports whether a file imports the SDK, under any name.
func importsClientGo(file *ast.File) bool {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, "\"`") == clientGoPath {
			return true
		}
	}
	return false
}

// observeFile judges every call in one file, each against the declaration it
// sits in.
func (s *scanner) observeFile(pkg *packages.Package, pkgName string, facts *facts, file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			s.observeNode(pkg, pkgName, facts, d, funcDeclName(d))
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if value, isValue := spec.(*ast.ValueSpec); isValue {
					s.observeNode(pkg, pkgName, facts, value, value.Names[0].Name)
				}
			}
		}
	}
}

// observeNode judges every call below one node, closures included, naming
// each finding after the declaration the node is.
func (s *scanner) observeNode(pkg *packages.Package, pkgName string, facts *facts, node ast.Node, enclosing string) {
	ast.Inspect(node, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		reason := s.judge(pkg.TypesInfo, facts, call)
		if reason == "" {
			return true
		}
		at := pkg.Fset.Position(call.Pos())
		s.findings = append(s.findings, Finding{
			Package: pkgName,
			File:    relativePath(at.Filename, s.root),
			Line:    at.Line,
			Func:    enclosing,
			Callee:  types.ExprString(call.Fun),
			Reason:  reason,
		})
		return true
	})
}

// judge decides one call, and returns why it reaches client-go without the
// caller's context, or nothing when it passes the context or is not a call
// this gate judges at all.
//
// A detached option is decided before anything else the call passes, so it is
// reported beside the caller's context as much as instead of it: placed after
// that context it replaces it, and placed before it, it does nothing and reads
// as though it did.
func (s *scanner) judge(info *types.Info, facts *facts, call *ast.CallExpr) string {
	if ctx, built := retryableConstructor(info, call); built {
		s.calls++
		if ctx != nil && !neverEnds(info, ctx) {
			return ""
		}
		return s.unlessRebound(facts, call)
	}
	options, signature := optionArguments(info, call)
	if signature == nil {
		return ""
	}
	s.calls++
	seen := outcomeMissing
	for _, option := range options {
		seen |= facts.outcomeOf(info, option)
	}
	switch {
	case seen.has(outcomeDetached):
		return reasonDetached
	case seen.has(outcomeCarried):
		return ""
	case seen.has(outcomeForwarded):
		s.forwarded++
		return ""
	}
	if !returnsRequest(signature) {
		return reasonMissing
	}
	return s.unlessRebound(facts, call)
}

// unlessRebound decides a call that built a request with no context of its
// own: clean when the variable holding the request is rebound to one, and
// reported otherwise.
func (s *scanner) unlessRebound(facts *facts, call *ast.CallExpr) string {
	if facts.rebound[facts.requestOf[call]] {
		s.rebound++
		return ""
	}
	return reasonUnbound
}

// retryableConstructor reports whether a call is one of go-retryablehttp's own
// request constructors, with the context it was handed: none for NewRequest,
// which builds from context.Background(), and the first argument of
// NewRequestWithContext.
//
// These are judged because nothing else here would see them. A request they
// build takes no client-go option, so no rule above reaches it, and
// (*gl.Client).Do sends it as it is. Judging the constructor rather than the
// send is what keeps a request a helper built from being reported where it is
// sent, as the four group board handlers send one: every request starts at a
// constructor, and each one that can start it without the caller's context is
// judged where it is called, client-go's builders by the option rules, these
// two here, and the standard library's by noctx, which is what covers the
// http.Request that FromRequest wraps. Only the direct call is recognized, as
// with the contexts that never end.
func retryableConstructor(info *types.Info, call *ast.CallExpr) (ast.Expr, bool) {
	fn := calledFunc(info, call.Fun)
	if fn == nil || fn.Pkg().Path() != retryablePath {
		return nil, false
	}
	switch fn.Name() {
	case "NewRequest":
		return nil, true
	case "NewRequestWithContext":
		return call.Args[0], true
	}
	return nil, false
}

// optionArguments returns the argument expressions a call hands to a request
// option parameter of its callee, with the callee's signature, which is nil
// when the callee has no such parameter and the call is not judged at all.
//
// The callee's signature is the type checker's, so a service method, a method
// value stored in a variable or a field, a function-typed parameter and a
// helper of this repository's own are all judged alike. A variadic
// `...RequestOptionFunc` contributes each of its arguments, or the one slice
// spread into it; a plain `[]RequestOptionFunc` parameter, which is how
// client-go's request builders take their options, contributes its argument.
//
// Two shapes contribute nothing although they reach a parameter. A builtin is
// passed over entirely: append's recorded signature is that of the one call,
// so appending options to a slice would otherwise read as a request. And a
// call handed the results of another call, `f(g())`, has no expression per
// parameter to read, so it is judged with no options, which reports it.
func optionArguments(info *types.Info, call *ast.CallExpr) ([]ast.Expr, *types.Signature) {
	fun := info.Types[call.Fun]
	if fun.IsType() || fun.IsBuiltin() {
		return nil, nil
	}
	signature, isSignature := fun.Type.Underlying().(*types.Signature)
	if !isSignature {
		return nil, nil
	}
	params := signature.Params()
	spreadsResults := len(call.Args) == 1 && isTuple(info.TypeOf(call.Args[0]))
	var options []ast.Expr
	reaches := false
	for i := range params.Len() {
		if !isOptionSlice(params.At(i).Type()) {
			continue
		}
		reaches = true
		if spreadsResults {
			continue
		}
		if signature.Variadic() && i == params.Len()-1 && !call.Ellipsis.IsValid() {
			options = append(options, call.Args[i:]...)
			continue
		}
		options = append(options, call.Args[i])
	}
	if !reaches {
		return nil, nil
	}
	return options, signature
}

// isTuple reports whether a type is the result list of a call returning more
// than one value.
func isTuple(t types.Type) bool {
	_, tuple := t.(*types.Tuple)
	return tuple
}

// facts are what a package's assignments say about its variables, gathered
// before any call is judged so a call may use a variable defined anywhere.
//
// Everything is keyed by the type checker's object, which is what makes a
// captured variable in a closure, a shadowed name and an import alias all
// resolve to what they really are.
type facts struct {
	// assigned are the expressions each option-typed variable was given, in
	// any assignment. A variable whose any assignment carries the context is
	// taken to carry it: the shape this exists for is a slice initialized
	// with gl.WithContext and appended to on a branch.
	assigned map[types.Object][]ast.Expr
	// params are the request-option parameters of every function literal and
	// declaration in the package. An object is unique to the function that
	// declares it, so a use of one is lexically inside that function or a
	// closure within it, and handing it on is forwarding.
	params map[types.Object]bool
	// requestOf names the variable a call's first result was assigned to,
	// which the rebinding rule reads for a request builder or constructor.
	requestOf map[*ast.CallExpr]types.Object
	// rebound are the request variables rebound through their WithContext
	// method to a context that can end, in a way that reaches the send: the
	// result assigned back to the variable, or handed straight to a call.
	// Where the rebinding sits is not read, so one after the send counts as
	// much as one before it; that is a stated limit of the rule, the same one
	// the option variables have.
	rebound map[types.Object]bool
	// outcomes memoises what each variable carries, and guards the
	// recursion: a variable assigned from itself is answered from here rather
	// than followed again.
	outcomes map[types.Object]outcome
}

// collectFacts reads a package's assignments, parameters and rebindings.
func collectFacts(pkg *packages.Package) *facts {
	f := &facts{
		assigned:  map[types.Object][]ast.Expr{},
		params:    map[types.Object]bool{},
		requestOf: map[*ast.CallExpr]types.Object{},
		rebound:   map[types.Object]bool{},
		outcomes:  map[types.Object]outcome{},
	}
	info := pkg.TypesInfo
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncType:
				f.noteParams(info, n)
			case *ast.AssignStmt:
				f.noteAssignment(info, n.Lhs, n.Rhs)
			case *ast.ValueSpec:
				f.noteAssignment(info, identsAsExprs(n.Names), n.Values)
			case *ast.CallExpr:
				f.noteReboundArguments(info, n)
			}
			return true
		})
	}
	return f
}

// noteParams records the request-option parameters one function declares.
// A function type with no body declares names no body can use, so recording
// them as well changes nothing. The type checker defines an object for every
// parameter name, a blank one included, so each name has one to read.
func (f *facts) noteParams(info *types.Info, fn *ast.FuncType) {
	for _, field := range fn.Params.List {
		for _, name := range field.Names {
			if obj := info.Defs[name]; isOptionSlice(obj.Type()) {
				f.params[obj] = true
			}
		}
	}
}

// noteAssignment records what one assignment gives its left-hand side: the
// option values a variable is built from, the variable a call's result lands
// in, and a request rebound to itself.
func (f *facts) noteAssignment(info *types.Info, lhs, rhs []ast.Expr) {
	if len(rhs) == 1 && len(lhs) > 1 {
		// `req, err := client.NewRequest(...)`: only the first result is a
		// request, and it is the one a rebinding would name.
		if call, isCall := ast.Unparen(rhs[0]).(*ast.CallExpr); isCall {
			f.requestOf[call] = objectOf(info, lhs[0])
		}
		return
	}
	for i := range min(len(lhs), len(rhs)) {
		obj := objectOf(info, lhs[i])
		if obj == nil {
			continue
		}
		value := ast.Unparen(rhs[i])
		if isOption(obj.Type()) || isOptionSlice(obj.Type()) {
			f.assigned[obj] = append(f.assigned[obj], value)
		}
		call, isCall := value.(*ast.CallExpr)
		if !isCall {
			continue
		}
		f.requestOf[call] = obj
		if receiver := requestRebinding(info, call); receiver == obj {
			f.rebound[obj] = true
		}
	}
}

// noteReboundArguments records a request rebound inside a call's arguments,
// `client.Do(req.WithContext(ctx), &out)`, which sends the rebound request
// directly.
func (f *facts) noteReboundArguments(info *types.Info, call *ast.CallExpr) {
	for _, arg := range call.Args {
		inner, isCall := ast.Unparen(arg).(*ast.CallExpr)
		if !isCall {
			continue
		}
		if receiver := requestRebinding(info, inner); receiver != nil {
			f.rebound[receiver] = true
		}
	}
}

// outcomeOf says what one option expression carries.
func (f *facts) outcomeOf(info *types.Info, expr ast.Expr) outcome {
	switch e := ast.Unparen(expr).(type) {
	case *ast.CallExpr:
		return f.callOutcome(info, e)
	case *ast.CompositeLit:
		seen := outcomeMissing
		for _, element := range e.Elts {
			seen |= f.outcomeOf(info, element)
		}
		return seen
	case *ast.Ident:
		return f.variableOutcome(info, info.Uses[e])
	}
	return outcomeMissing
}

// callOutcome says what a call producing options carries: gl.WithContext
// itself, or append building a slice out of options that do.
func (f *facts) callOutcome(info *types.Info, call *ast.CallExpr) outcome {
	if isClientGoFunc(info, call.Fun, "WithContext") {
		if neverEnds(info, call.Args[0]) {
			return outcomeDetached
		}
		return outcomeCarried
	}
	if !isBuiltin(info, call.Fun, "append") {
		return outcomeMissing
	}
	seen := outcomeMissing
	for _, arg := range call.Args {
		seen |= f.outcomeOf(info, arg)
	}
	return seen
}

// variableOutcome says what a variable carries: forwarded when it is a
// request-option parameter, together with everything it was ever assigned.
//
// Neither half reads control flow, which is a stated limit rather than an
// oversight. A parameter reassigned before the call is still forwarded, and an
// assignment counts wherever it sits, after the call included: the same
// reading that lets a slice initialized with the context and appended to on a
// branch pass without the walk following the branch.
func (f *facts) variableOutcome(info *types.Info, obj types.Object) outcome {
	if known, memoized := f.outcomes[obj]; memoized {
		return known
	}
	seen := outcomeMissing
	if f.params[obj] {
		seen = outcomeForwarded
	}
	f.outcomes[obj] = seen
	for _, value := range f.assigned[obj] {
		seen |= f.outcomeOf(info, value)
	}
	f.outcomes[obj] = seen
	return seen
}

// requestRebinding names the request variable a call rebinds when it is
// `req.WithContext(ctx)` on a request client-go built, with a context that
// can end, and nil for anything else.
func requestRebinding(info *types.Info, call *ast.CallExpr) types.Object {
	selector, isSelector := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "WithContext" {
		return nil
	}
	selection := info.Selections[selector]
	if selection == nil || !isRequest(selection.Recv()) {
		return nil
	}
	receiver, isIdent := ast.Unparen(selector.X).(*ast.Ident)
	if !isIdent || neverEnds(info, call.Args[0]) {
		return nil
	}
	return info.Uses[receiver]
}

// returnsRequest reports whether a callee is one of client-go's request
// builders, whose request can still be given a context after it is built.
func returnsRequest(signature *types.Signature) bool {
	results := signature.Results()
	return results.Len() > 0 && isRequest(results.At(0).Type())
}

// neverEnds reports whether an expression is context.Background() or
// context.TODO(), the two contexts no deadline and no cancellation reaches.
// Only the direct call is recognized: a variable holding one is usually
// derived from before it is used, by a timeout or a cancel, and following it
// would refuse exactly the bounded contexts a command builds for itself.
func neverEnds(info *types.Info, expr ast.Expr) bool {
	call, isCall := ast.Unparen(expr).(*ast.CallExpr)
	if !isCall {
		return false
	}
	fn := calledFunc(info, call.Fun)
	return fn != nil && fn.Pkg().Path() == "context" && (fn.Name() == "Background" || fn.Name() == "TODO")
}

// isClientGoFunc reports whether an expression names the client-go
// package-level function of that name, whatever the import is called.
func isClientGoFunc(info *types.Info, fun ast.Expr, name string) bool {
	fn := calledFunc(info, fun)
	return fn != nil && fn.Pkg().Path() == clientGoPath && fn.Name() == name
}

// calledFunc resolves the package-level function a call expression names,
// through a package qualifier or directly, and nil for anything else: a
// method, a function value, a builtin. A method is left out because every
// question asked of the result here is about a package's own function, and a
// method of the same name would otherwise answer it.
func calledFunc(info *types.Info, fun ast.Expr) *types.Func {
	var ident *ast.Ident
	switch f := ast.Unparen(fun).(type) {
	case *ast.SelectorExpr:
		ident = f.Sel
	case *ast.Ident:
		ident = f
	default:
		return nil
	}
	fn, isFunc := info.Uses[ident].(*types.Func)
	if !isFunc || fn.Signature().Recv() != nil {
		return nil
	}
	return fn
}

// isBuiltin reports whether an expression names the builtin of that name.
func isBuiltin(info *types.Info, fun ast.Expr, name string) bool {
	ident, isIdent := ast.Unparen(fun).(*ast.Ident)
	if !isIdent {
		return false
	}
	builtin, isBuiltinObj := info.Uses[ident].(*types.Builtin)
	return isBuiltinObj && builtin.Name() == name
}

// isOption reports whether a type is client-go's RequestOptionFunc.
func isOption(t types.Type) bool {
	return isNamed(t, clientGoPath, "RequestOptionFunc")
}

// isOptionSlice reports whether a type is a slice of client-go request
// options, which is also the type a variadic `...RequestOptionFunc`
// parameter has.
func isOptionSlice(t types.Type) bool {
	slice, isSlice := t.Underlying().(*types.Slice)
	return isSlice && isOption(slice.Elem())
}

// isRequest reports whether a type is a pointer to the request client-go's
// builders return.
func isRequest(t types.Type) bool {
	pointer, isPointer := types.Unalias(t).(*types.Pointer)
	return isPointer && isNamed(pointer.Elem(), retryablePath, "Request")
}

// isNamed reports whether a type is the named type of that package and name.
func isNamed(t types.Type, pkgPath, name string) bool {
	named, isNamedType := types.Unalias(t).(*types.Named)
	if !isNamedType {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == pkgPath && obj.Name() == name
}

// objectOf resolves the variable an assignment's left-hand side names, new or
// existing, and nil for anything that is not a plain identifier.
func objectOf(info *types.Info, expr ast.Expr) types.Object {
	ident, isIdent := ast.Unparen(expr).(*ast.Ident)
	if !isIdent {
		return nil
	}
	if obj := info.Defs[ident]; obj != nil {
		return obj
	}
	return info.Uses[ident]
}

// identsAsExprs widens a name list so a declaration and an assignment are
// read by one function.
func identsAsExprs(names []*ast.Ident) []ast.Expr {
	exprs := make([]ast.Expr, len(names))
	for i, name := range names {
		exprs[i] = name
	}
	return exprs
}

// funcDeclName spells a function the way its file does: the bare name, or
// `Type.Method` with the receiver's type stripped of its pointer and of its
// type parameters. A receiver that is not a type name once those are gone
// does not type-check, and is spelled by the method's name alone rather than
// guessed at.
func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil {
		return fn.Name.Name
	}
	receiver := ast.Unparen(fn.Recv.List[0].Type)
	if star, isPointer := receiver.(*ast.StarExpr); isPointer {
		receiver = ast.Unparen(star.X)
	}
	switch generic := receiver.(type) {
	case *ast.IndexExpr:
		receiver = generic.X
	case *ast.IndexListExpr:
		receiver = generic.X
	}
	if ident, isIdent := receiver.(*ast.Ident); isIdent {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
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
