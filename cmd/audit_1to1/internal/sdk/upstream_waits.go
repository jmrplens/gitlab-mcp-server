// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// The forms of workaround the walk enumerates. Anything else a workaround
// can be (an error hint, a URL a handler builds, a captured response) is
// seen only when an entry declares it.
const (
	// formRawRequest: a tool package builds a request through one of
	// client-go's own constructors and sends it itself.
	formRawRequest = "raw-request"
	// formRequestOption: a function writes a request option by hand, a
	// literal of client-go's RequestOptionFunc or a call of one of the
	// option constructors client-go exports other than WithContext.
	formRequestOption = "request-option"
	// formBodyEdit: a call of toolutil.EditJSONBody, with the keys it
	// writes into or takes out of the body client-go encoded.
	formBodyEdit = "body-edit"
	// formFixtureRawRequest: the e2e fixture builds a request through one
	// of client-go's constructors.
	formFixtureRawRequest = "fixture-raw-request"
	// formScenarioRawRequest: an e2e scenario builds a request through one
	// of client-go's constructors.
	formScenarioRawRequest = "scenario-raw-request"
)

// What an entry came to. An upstream entry holds, has a predicate to edit or
// a workaround to retire; an entry of any other category is declared.
const (
	waitHolds          = "holds"
	waitStalePredicate = "stale_predicate"
	waitObsolete       = "obsolete_workaround"
	waitDeclared       = "declared"
)

// Names the walk recognizes things by.
const (
	clientStructName      = "Client"
	requestOptionFuncName = "RequestOptionFunc"
	withContextName       = "WithContext"
	toolutilPathSuffix    = "/internal/toolutil"
	toolutilSitePrefix    = "toolutil."
	editJSONBodyName      = "EditJSONBody"
	nullWhenZeroName      = "NullWhenZero"
	bodyEditName          = "BodyEdit"
	bodyEditSetName       = "Set"
	glAccessor            = "GL"
	httpMethodPrefix      = "Method"
	verbParam             = "method"
	// uploadTypeParam is the name UploadRequest gives the form field its
	// file is sent under.
	uploadTypeParam = "uploadType"
)

// fixtureDir is the e2e fixture package and scenarioDir the tree of e2e
// scenario packages, relative to the repository root. Their files carry the
// e2e build tag and are read for their syntax alone.
const (
	fixtureDir  = "test/e2e/internal/fixture"
	scenarioDir = "test/e2e/gitlab"
)

// e2eTree is one tree of e2e sources the walk reads: where it is, how a
// finding names it, the form its raw requests take, and whether its
// subdirectories and its test files are read. The fixture is one package,
// whose own tests are its plumbing; a scenario is a test file, in a package
// per runtime.
type e2eTree struct {
	dir, name, form  string
	recursive, tests bool
}

// e2eTrees are the trees of e2e sources the walk reads.
var e2eTrees = []e2eTree{
	{dir: fixtureDir, name: "e2e fixture", form: formFixtureRawRequest},
	{dir: scenarioDir, name: "e2e scenarios", form: formScenarioRawRequest, recursive: true, tests: true},
}

// rawConstructors are the methods of client-go's Client that build a request
// a caller then sends itself, each with the index of its options argument,
// which go/parser reads in the fixture without the types to name it by.
var rawConstructors = map[string]int{"NewRequest": 2, "NewRequestToURL": 2, "UploadRequest": 5}

// workaround is one site the walk found carrying a workaround, with the
// entries that answer it.
type workaround struct {
	Form string `json:"form"`
	Site string `json:"site"`
	// Constructor, Verb and Options describe a raw request: the client-go
	// constructor that builds it, the verb when it is a constant, and the
	// static type of its options (the expression, in the e2e trees), empty
	// for nil. R-PARAM reads the last to walk a raw route's options.
	Constructor string `json:"constructor,omitempty"`
	Verb        string `json:"verb,omitempty"`
	Options     string `json:"options,omitempty"`
	// UploadField is, for an UploadRequest, the form field its file is sent
	// under when the call names it with a constant, which R-PARAM counts as
	// a param the request carries beside its options.
	UploadField string `json:"upload_field,omitempty"`
	// Callers are, for a raw request whose function forwards it its own
	// verb or options parameter, each call of that function with what the
	// request then carries, which R-PARAM reads in place of the forwarder's
	// own verb and options.
	Callers []rawCaller `json:"callers,omitempty"`
	// Decodes and Sends are the supersets a raw request decodes its answer
	// into and sends as its options: structs of the package's own that
	// embed a client-go struct, each with the predicates the keys it carries
	// beside that struct need. UnansweredNeeds are the needs no predicate
	// of an entry naming the site is.
	Decodes         []waits.Superset `json:"decodes,omitempty"`
	Sends           []waits.Superset `json:"sends,omitempty"`
	UnansweredNeeds []string         `json:"unanswered_needs,omitempty"`
	// Keys are the body keys a body edit writes or takes out, and
	// UnansweredKeys the ones no predicate of an entry naming the site
	// explains. KeysUnread marks a body edit whose keys the walk cannot read
	// (built in another package, or more than one call away), which no
	// predicate can be tied to.
	Keys           []string `json:"keys,omitempty"`
	UnansweredKeys []string `json:"unanswered_keys,omitempty"`
	KeysUnread     bool     `json:"keys_unread,omitempty"`
	// PassedTo are the client-go methods a request option is handed to, and
	// UnansweredMethods, once an entry naming the site carries a
	// TakesNoOptions, the ones none of its TakesNoOptions names.
	PassedTo          []string `json:"passed_to,omitempty"`
	UnansweredMethods []string `json:"unanswered_methods,omitempty"`
	// AnsweredBy names the entries that list the site; empty is the
	// finding.
	AnsweredBy []string `json:"answered_by,omitempty"`
}

// waitEntry is one entry of the two tables with what the run made of it.
type waitEntry struct {
	Row        int             `json:"row,omitempty"`
	Category   string          `json:"category"`
	Sites      []string        `json:"sites,omitempty"`
	Unseen     []string        `json:"unseen,omitempty"`
	Replaces   []string        `json:"replaces,omitempty"`
	Status     string          `json:"status"`
	Predicates []waits.Verdict `json:"predicates,omitempty"`
	Reason     string          `json:"reason"`
}

// upstreamWaitsReport is the upstream_waits section of the report.
type upstreamWaitsReport struct {
	Entries     []waitEntry  `json:"entries"`
	Workarounds []workaround `json:"workarounds"`
}

// waitsInput is what the section is built from.
type waitsInput struct {
	// found is every workaround the walks found.
	found []workaround
	// sites is every function the walks can name, which a site an entry
	// lists has to be one of, and strings the constant strings each one's
	// body holds, which an unseen site is held to naming its keys in.
	sites   map[string]bool
	strings map[string][]string
	sdk     waits.SDK
	// rows are the register's rows, each with the text that describes it.
	rows map[int]string
	// called holds the client-go methods ("Service.Method") a handler
	// references.
	called  map[string]bool
	entries []waits.Entry
}

// analyzeUpstreamWaits walks root's tool packages, the toolutil package they
// import and the e2e trees for workarounds, and holds them and entries to
// each other and to the pinned SDK.
func analyzeUpstreamWaits(root string, pkgs []*packages.Package, clientGo *types.Package, usage map[string]*shared.ServiceUsage, entries []waits.Entry) (upstreamWaitsReport, []string, error) {
	found, sites, reads := walkToolWorkarounds(pkgs, clientGo)
	toolutilFound, toolutilSites := toolutilWorkarounds(pkgs, clientGo)
	e2eFound, e2eSites, e2eStrings, err := e2eWorkarounds(root)
	if err != nil {
		return upstreamWaitsReport{}, nil, err
	}
	maps.Copy(sites, toolutilSites)
	maps.Copy(sites, e2eSites)
	maps.Copy(reads.strings, e2eStrings)
	found = slices.Concat(found, toolutilFound, e2eFound)
	for i := range found {
		if found[i].Form == formRequestOption {
			found[i].PassedTo = slices.Sorted(maps.Keys(reads.passedTo[found[i].Site]))
		}
	}
	rows, rowsErr := waits.ReadRows(filepath.Join(root, filepath.FromSlash(waits.RegisterPath)))
	routes, unresolved := sdkRoutes(structs.ClientGoDir(pkgs))
	report, stale := buildUpstreamWaits(waitsInput{
		found:   found,
		sites:   sites,
		strings: reads.strings,
		sdk:     waits.SDK{Types: clientGo, Routes: routes, Unresolved: unresolved},
		rows:    rows,
		called:  calledMethods(usage),
		entries: entries,
	})
	if rowsErr != nil {
		stale = append(stale, "upstream register: "+rowsErr.Error())
	}
	return report, stale, nil
}

// sdkRoutes returns every route a method of the client-go source in dir
// sends, spelled the way a [waits.NoMethodFor] spells one, and every request
// one sends whose path the reading did not fold to a route, which a route
// unsent by the first could still be.
func sdkRoutes(dir string) (routes map[string]bool, unresolved []string) {
	routes = map[string]bool{}
	for _, method := range sdkroutes.Read(dir).Methods() {
		for _, route := range method.Routes {
			routes[route.String()] = true
		}
		unresolved = append(unresolved, method.Unresolved...)
	}
	return routes, unresolved
}

// calledMethods spells every method a handler references "Service.Method".
func calledMethods(usage map[string]*shared.ServiceUsage) map[string]bool {
	called := map[string]bool{}
	for _, use := range usage {
		for method := range use.Called {
			called[use.Service()+"."+method] = true
		}
	}
	return called
}

// walkToolWorkarounds finds the raw requests, the request options written by
// hand and the body edits of every function of pkgs, names every function
// it walked, and reads what is around them: each raw request is given the
// supersets it decodes and sends and, for one whose function forwards it its
// verb or options, the calls of that function.
func walkToolWorkarounds(pkgs []*packages.Package, clientGo *types.Package) (found []workaround, sites map[string]bool, reads *rawReads) {
	optionFunc := requestOptionFunc(clientGo)
	constructors := constructorVars(pkgs)
	sites = map[string]bool{}
	reads = newRawReads(clientGo)
	for _, pkg := range pkgs {
		pkgName := shared.ShortPackage(pkg.PkgPath)
		bodies := functionBodies(pkg)
		byObject := map[types.Object]funcBody{}
		for _, fn := range bodies {
			byObject[fn.object] = fn
		}
		for _, fn := range bodies {
			site := pkgName + "." + fn.name
			sites[site] = true
			scan := bodyScan{info: pkg.TypesInfo, clientGo: clientGo, optionFunc: optionFunc, constructors: constructors, byObject: byObject}
			found = append(found, scan.workarounds(fn, site)...)
			reads.read(scan, fn, site)
		}
	}
	reads.readCallers(pkgs)
	reads.attach(found)
	return found, sites, reads
}

// bodyScan is what scanning one function body needs.
type bodyScan struct {
	info         *types.Info
	clientGo     *types.Package
	optionFunc   types.Type
	constructors map[types.Object]*types.Func
	byObject     map[types.Object]funcBody
}

// workarounds lists what fn carries: each raw request and body edit it makes,
// and one request option when it writes any.
func (s bodyScan) workarounds(fn funcBody, site string) []workaround {
	var out []workaround
	writes := s.isOptionSignature(fn.signature)
	ast.Inspect(fn.body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncLit:
			signature, _ := s.info.TypeOf(typed).(*types.Signature)
			writes = writes || s.isOptionSignature(signature)
		case *ast.CallExpr:
			if raw, isRaw := s.rawRequest(typed); isRaw {
				raw.Site = site
				out = append(out, raw)
				return true
			}
			callee := calledObject(s.info, typed)
			writes = writes || s.isOptionConstructor(callee)
			if isToolutilFunc(callee, editJSONBodyName) {
				out = append(out, workaround{Form: formBodyEdit, Site: site, Keys: s.bodyEditKeys(fn)})
			}
		}
		return true
	})
	if writes {
		out = append(out, workaround{Form: formRequestOption, Site: site})
	}
	return out
}

// rawRequest describes a call of one of client-go's request constructors,
// made on a Client or through a package variable that holds the method
// expression (commits.newRequest), and reports false for any other call.
func (s bodyScan) rawRequest(call *ast.CallExpr) (workaround, bool) {
	constructor, offset := s.rawConstructor(call)
	if constructor == nil {
		return workaround{}, false
	}
	return workaround{
		Form: formRawRequest, Constructor: constructor.Name(),
		Verb:        constantString(s.info, argNamed(constructor, call, offset, verbParam)),
		Options:     spellOptions(s.info, argNamed(constructor, call, offset, rawRequestParam)),
		UploadField: constantString(s.info, argNamed(constructor, call, offset, uploadTypeParam)),
	}, true
}

// rawRequestArgs returns the request constructor a call builds a raw request
// with and the call's verb and options arguments, each nil when the call
// does not pass it, and a nil constructor for any other call.
func (s bodyScan) rawRequestArgs(call *ast.CallExpr) (constructor *types.Func, verb, opt ast.Expr) {
	constructor, offset := s.rawConstructor(call)
	if constructor == nil {
		return nil, nil, nil
	}
	return constructor, argNamed(constructor, call, offset, verbParam), argNamed(constructor, call, offset, rawRequestParam)
}

// rawConstructor returns the request constructor a call builds a raw
// request with, and the offset of the constructor's first parameter among
// the call's arguments: 0 for a call of the method on a Client, 1 for a call
// through a package variable holding its method expression, whose first
// argument is the Client. It returns a nil constructor for any other call.
func (s bodyScan) rawConstructor(call *ast.CallExpr) (constructor *types.Func, offset int) {
	if sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr); isSel {
		if method := clientConstructor(s.info.Selections[sel], types.MethodVal); method != nil {
			return method, 0
		}
	}
	return s.constructors[calledObject(s.info, call)], 1
}

// argNamed returns the argument a call hands the constructor's parameter of
// that name, and nil when the call stops before it or the constructor takes
// none.
func argNamed(constructor *types.Func, call *ast.CallExpr, offset int, name string) ast.Expr {
	params := constructor.Signature().Params()
	for i := range params.Len() {
		if i+offset >= len(call.Args) {
			return nil
		}
		if params.At(i).Name() == name {
			return call.Args[i+offset]
		}
	}
	return nil
}

// clientConstructor returns the client-go request constructor a selection of
// kind names, and nil for any other selection, a missing one included. A
// method selection always selects a function.
func clientConstructor(selection *types.Selection, kind types.SelectionKind) *types.Func {
	if selection == nil || selection.Kind() != kind {
		return nil
	}
	recv, isClient := clientGoStruct(selection.Recv())
	if !isClient || recv.Obj().Name() != clientStructName {
		return nil
	}
	method, _ := selection.Obj().(*types.Func)
	if _, isConstructor := rawConstructors[method.Name()]; !isConstructor {
		return nil
	}
	return method
}

// constructorVars finds the package variables of pkgs that hold one of
// client-go's request constructors as a method expression
// (`var newRequest = (*gl.Client).NewRequest`, a seam a test replaces), so a
// call through one is read as the raw request it builds.
func constructorVars(pkgs []*packages.Package) map[types.Object]*types.Func {
	out := map[types.Object]*types.Func{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				if gen, isGen := decl.(*ast.GenDecl); isGen {
					addConstructorVars(pkg.TypesInfo, gen, out)
				}
			}
		}
	}
	return out
}

// addConstructorVars adds to out each variable a declaration initializes
// with a request constructor's method expression.
func addConstructorVars(info *types.Info, decl *ast.GenDecl, out map[types.Object]*types.Func) {
	for _, spec := range decl.Specs {
		values, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for i, value := range values.Values {
			sel, isSel := ast.Unparen(value).(*ast.SelectorExpr)
			if !isSel {
				continue
			}
			if constructor := clientConstructor(info.Selections[sel], types.MethodExpr); constructor != nil {
				out[info.Defs[values.Names[i]]] = constructor
			}
		}
	}
}

// requestOptionFunc returns client-go's RequestOptionFunc, nil when the SDK
// declares none or declares it as something other than a function type.
func requestOptionFunc(clientGo *types.Package) types.Type {
	obj, _ := clientGo.Scope().Lookup(requestOptionFuncName).(*types.TypeName)
	if obj == nil {
		return nil
	}
	if _, isFunc := obj.Type().Underlying().(*types.Signature); !isFunc {
		return nil
	}
	return obj.Type()
}

// isOptionSignature reports whether a function of this signature can stand
// as a RequestOptionFunc. The two are compared by shape, the spelling of
// each parameter and result type, so neither the parameters' names nor two
// loads holding two objects for the request type change the verdict.
func (s bodyScan) isOptionSignature(signature *types.Signature) bool {
	if signature == nil || s.optionFunc == nil {
		return false
	}
	want, _ := s.optionFunc.Underlying().(*types.Signature)
	return signatureShape(signature) == signatureShape(want)
}

// signatureShape spells a signature's parameter and result types in order,
// without their names.
func signatureShape(signature *types.Signature) string {
	var shape strings.Builder
	for param := range signature.Params().Variables() {
		shape.WriteString(types.TypeString(param.Type(), nil) + ";")
	}
	shape.WriteString(" -> ")
	for result := range signature.Results().Variables() {
		shape.WriteString(types.TypeString(result.Type(), nil) + ";")
	}
	return shape.String()
}

// isOptionConstructor reports whether obj is a package-level function of
// client-go that returns a RequestOptionFunc, other than WithContext, which
// every call passes and which carries no parameter.
func (s bodyScan) isOptionConstructor(obj types.Object) bool {
	fn, isFunc := obj.(*types.Func)
	if !isFunc || s.optionFunc == nil || fn.Pkg() != s.clientGo || fn.Name() == withContextName {
		return false
	}
	return returnsType(fn.Signature(), s.optionFunc)
}

// returnsType reports whether one of signature's results is t, by spelling.
func returnsType(signature *types.Signature, t types.Type) bool {
	want := types.TypeString(t, nil)
	for result := range signature.Results().Variables() {
		if types.TypeString(result.Type(), nil) == want {
			return true
		}
	}
	return false
}

// isToolutilFunc reports whether obj is the function of the shared toolutil
// package called name.
func isToolutilFunc(obj types.Object, name string) bool {
	fn, isFunc := obj.(*types.Func)
	return isFunc && fn.Name() == name && inToolutil(fn)
}

// inToolutil reports whether obj is declared in the shared toolutil package.
// An object of the universe has no package.
func inToolutil(obj types.Object) bool {
	pkg := obj.Pkg()
	return pkg != nil && strings.HasSuffix(pkg.Path(), toolutilPathSuffix)
}

// bodyEditKeys lists the body keys a body edit in fn writes or takes out:
// the key of each toolutil.NullWhenZero, the keys of a map set as a
// BodyEdit's Set and the names in its Drop, read in fn and in the functions
// of its package fn calls, where an update's options are often built.
func (s bodyScan) bodyEditKeys(fn funcBody) []string {
	keys := map[string]bool{}
	bodies := []*ast.BlockStmt{fn.body}
	ast.Inspect(fn.body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall {
			if callee, isLocal := s.byObject[calledObject(s.info, call)]; isLocal {
				bodies = append(bodies, callee.body)
			}
		}
		return true
	})
	for _, body := range bodies {
		s.collectEditKeys(body, keys)
	}
	return slices.Sorted(maps.Keys(keys))
}

// collectEditKeys adds the keys body writes into a BodyEdit to keys. A
// variable handed to Set or Drop is followed to the keys assigned into it
// and the strings appended to it.
func (s bodyScan) collectEditKeys(body *ast.BlockStmt, keys map[string]bool) {
	dropped := s.collectEditValues(body, keys)
	handed := func(expr ast.Expr) bool {
		ident, isIdent := expr.(*ast.Ident)
		return isIdent && dropped[identVar(s.info, ident)]
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			if len(typed.Args) > 0 && handed(typed.Args[0]) {
				s.addAppended(typed, keys)
			}
		case *ast.AssignStmt:
			s.addIndexedKeys(typed.Lhs, handed, keys)
		}
		return true
	})
}

// collectEditValues adds to keys what body writes into a BodyEdit in place:
// the key of each NullWhenZero, the keys and names a literal or an
// assignment sets on its Set or Drop, the key an assignment writes into its
// Set, and the strings an append adds to its Drop. It returns the variables
// handed to one whole, whose keys and appended strings the caller collects.
func (s bodyScan) collectEditValues(body *ast.BlockStmt, keys map[string]bool) map[*types.Var]bool {
	dropped := map[*types.Var]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			// NullWhenZero(value, key, edit): the key is the second
			// argument, which the compiler holds every call to.
			if isToolutilFunc(calledObject(s.info, typed), nullWhenZeroName) {
				s.addKey(typed.Args[1], keys)
			}
		case *ast.CompositeLit:
			s.editLiteral(typed, keys, dropped)
		case *ast.AssignStmt:
			s.editAssign(typed, keys, dropped)
		}
		return true
	})
	return dropped
}

// editLiteral adds what lit sets on its Set and Drop, when it is a BodyEdit.
func (s bodyScan) editLiteral(lit *ast.CompositeLit, keys map[string]bool, dropped map[*types.Var]bool) {
	if !isBodyEdit(s.info.TypeOf(lit)) {
		return
	}
	for _, elt := range lit.Elts {
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			s.editValue(kv.Value, keys, dropped)
		}
	}
}

// editAssign adds what one assignment writes into a BodyEdit: the key
// edit.Set["key"] = value writes whatever the value, and what a value set
// as its Set or Drop names.
func (s bodyScan) editAssign(assign *ast.AssignStmt, keys map[string]bool, dropped map[*types.Var]bool) {
	s.addIndexedKeys(assign.Lhs, func(expr ast.Expr) bool { return s.editField(expr) == bodyEditSetName }, keys)
	for i, lhs := range assign.Lhs {
		if s.editField(lhs) != "" && i < len(assign.Rhs) {
			s.editValue(assign.Rhs[i], keys, dropped)
		}
	}
}

// addIndexedKeys adds to keys the key each of lhs writes into a map, when
// into says the map is one the edit writes.
func (s bodyScan) addIndexedKeys(lhs []ast.Expr, into func(ast.Expr) bool, keys map[string]bool) {
	for _, target := range lhs {
		if index, isIndex := target.(*ast.IndexExpr); isIndex && into(index.X) {
			s.addKey(index.Index, keys)
		}
	}
}

// addAppended adds to keys the strings call appends, when it is a call of
// the builtin append.
func (s bodyScan) addAppended(call *ast.CallExpr, keys map[string]bool) {
	if len(call.Args) < 2 || !isBuiltin(s.info, call.Fun, "append") {
		return
	}
	for _, arg := range call.Args[1:] {
		s.addKey(arg, keys)
	}
}

// editField returns the name of the field of a BodyEdit expr selects, Set
// or Drop, and the empty string for any other expression.
func (s bodyScan) editField(expr ast.Expr) string {
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel || !isBodyEdit(s.info.TypeOf(sel.X)) {
		return ""
	}
	return sel.Sel.Name
}

// editValue adds what one value set into a BodyEdit names: the keys of a map
// literal, the strings of a list literal or of an append, or, for a
// variable, the variable, whose keys and appended strings the caller
// collects.
func (s bodyScan) editValue(value ast.Expr, keys map[string]bool, dropped map[*types.Var]bool) {
	switch typed := value.(type) {
	case *ast.CompositeLit:
		for _, elt := range typed.Elts {
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				elt = kv.Key
			}
			s.addKey(elt, keys)
		}
	case *ast.CallExpr:
		// edit.Drop = append(edit.Drop, "key") appends what follows the
		// list it extends.
		s.addAppended(typed, keys)
	case *ast.Ident:
		if variable := identVar(s.info, typed); variable != nil {
			dropped[variable] = true
		}
	}
}

// addKey adds expr to keys when it is a constant string.
func (s bodyScan) addKey(expr ast.Expr, keys map[string]bool) {
	if key := constantString(s.info, expr); key != "" {
		keys[key] = true
	}
}

// isBodyEdit reports whether t is, or points to, toolutil.BodyEdit.
func isBodyEdit(t types.Type) bool {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	named, isNamed := t.(*types.Named)
	return isNamed && named.Obj().Name() == bodyEditName && inToolutil(named.Obj())
}

// isBuiltin reports whether fun names the builtin called name.
func isBuiltin(info *types.Info, fun ast.Expr, name string) bool {
	ident, isIdent := ast.Unparen(fun).(*ast.Ident)
	if !isIdent {
		return false
	}
	builtin, isBuiltinObj := info.Uses[ident].(*types.Builtin)
	return isBuiltinObj && builtin.Name() == name
}

// constantString returns the value of a constant string expression, and the
// empty string for anything else.
func constantString(info *types.Info, expr ast.Expr) string {
	value := info.Types[expr].Value
	if value == nil || value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(value)
}

// spellOptions spells the static type of a raw request's options, each
// package by its name, and returns the empty string for nil.
func spellOptions(info *types.Info, expr ast.Expr) string {
	tv := info.Types[expr]
	if tv.IsNil() || tv.Type == nil {
		return ""
	}
	return types.TypeString(tv.Type, func(pkg *types.Package) string { return pkg.Name() })
}

// toolutilWorkarounds names every function the shared toolutil package
// exports, and lists as a request option each one that returns a
// RequestOptionFunc, but EditJSONBody, whose every call the walk judges on
// its own. The package is read from the types the tool packages import, so
// it is known by its signatures and not walked. Only what it exports is a
// site: a tool package can call nothing else, and an unexported function is
// reached through an exported one, which is the site an entry names.
func toolutilWorkarounds(pkgs []*packages.Package, clientGo *types.Package) (found []workaround, sites map[string]bool) {
	sites = map[string]bool{}
	toolutil := importedPackage(pkgs, toolutilPathSuffix)
	optionFunc := requestOptionFunc(clientGo)
	if toolutil == nil || optionFunc == nil {
		return nil, sites
	}
	for _, name := range toolutil.Scope().Names() {
		fn, isFunc := toolutil.Scope().Lookup(name).(*types.Func)
		if !isFunc || !fn.Exported() {
			continue
		}
		sites[toolutilSitePrefix+name] = true
		if name != editJSONBodyName && returnsType(fn.Signature(), optionFunc) {
			found = append(found, workaround{Form: formRequestOption, Site: toolutilSitePrefix + name})
		}
	}
	return found, sites
}

// importedPackage returns the types of the first package pkgs import whose
// path ends in suffix, nil when none does.
func importedPackage(pkgs []*packages.Package, suffix string) *types.Package {
	for _, pkg := range pkgs {
		for path, imported := range pkg.Imports {
			if strings.HasSuffix(path, suffix) && imported.Types != nil {
				return imported.Types
			}
		}
	}
	return nil
}

// e2eWorkarounds reads the e2e trees under root for their syntax, lists every
// raw request they build through a client's GL() accessor, names every
// function they declare and collects the strings each one's body holds. A
// tree root lacks has none; one that cannot be read or parsed fails the run.
func e2eWorkarounds(root string) (found []workaround, sites map[string]bool, strs map[string][]string, err error) {
	sites, strs = map[string]bool{}, map[string][]string{}
	fset := token.NewFileSet()
	for _, tree := range e2eTrees {
		dir := filepath.Join(root, filepath.FromSlash(tree.dir))
		files, readErr := e2eFiles(dir, tree)
		// A tree that is not there is skipped, but a name that is there and
		// does not read as a directory is not: Windows reports a regular file
		// read as one as not existing, and so does a dangling link anywhere.
		if errors.Is(readErr, fs.ErrNotExist) {
			if _, statErr := os.Lstat(dir); statErr != nil {
				continue
			}
			readErr = fmt.Errorf("%s is there but cannot be read as a directory", dir)
		}
		if readErr != nil {
			return nil, nil, nil, fmt.Errorf("read the %s: %w", tree.name, readErr)
		}
		for _, path := range files {
			file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				return nil, nil, nil, fmt.Errorf("parse the %s: %w", tree.name, parseErr)
			}
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || fn.Body == nil {
					continue
				}
				site := file.Name.Name + "." + functionName(fn)
				sites[site] = true
				strs[site] = append(strs[site], syntaxStrings(fn.Body)...)
				found = append(found, e2eRawRequests(fn.Body, site, tree.form)...)
			}
		}
	}
	return found, sites, strs, nil
}

// e2eFiles lists the Go files under root the tree reads: test files only
// when the tree's are read, and the subdirectories' only when it is read
// whole, each directory read once in the order it is found.
func e2eFiles(root string, tree e2eTree) ([]string, error) {
	var files []string
	for dirs := []string{root}; len(dirs) > 0; dirs = dirs[1:] {
		dirEntries, err := os.ReadDir(dirs[0])
		if err != nil {
			return nil, err
		}
		for _, dirEntry := range dirEntries {
			path, name := filepath.Join(dirs[0], dirEntry.Name()), dirEntry.Name()
			if dirEntry.IsDir() && tree.recursive {
				dirs = append(dirs, path)
			}
			if dirEntry.IsDir() || !strings.HasSuffix(name, ".go") || (!tree.tests && strings.HasSuffix(name, "_test.go")) {
				continue
			}
			files = append(files, path)
		}
	}
	return files, nil
}

// syntaxStrings lists the string literals body holds.
func syntaxStrings(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(node ast.Node) bool {
		if lit, isLit := node.(*ast.BasicLit); isLit && lit.Kind == token.STRING {
			value, _ := strconv.Unquote(lit.Value)
			out = append(out, value)
		}
		return true
	})
	return out
}

// e2eRawRequests lists the calls in body of a constructor on what a GL()
// call returns, which is how the e2e trees reach client-go's Client, each a
// workaround of form.
func e2eRawRequests(body *ast.BlockStmt, site, form string) []workaround {
	var found []workaround
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel {
			return true
		}
		optIndex, isConstructor := rawConstructors[sel.Sel.Name]
		accessor, isAccessorCall := sel.X.(*ast.CallExpr)
		if !isConstructor || !isAccessorCall || !callsNamed(accessor, glAccessor) {
			return true
		}
		raw := workaround{Form: form, Site: site, Constructor: sel.Sel.Name}
		if len(call.Args) > optIndex {
			raw.Verb = syntaxVerb(call.Args[0])
			if options := types.ExprString(call.Args[optIndex]); options != "nil" {
				raw.Options = options
			}
		}
		found = append(found, raw)
		return true
	})
	return found
}

// callsNamed reports whether call calls a method or function called name.
func callsNamed(call *ast.CallExpr, name string) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name == name
	case *ast.Ident:
		return fun.Name == name
	}
	return false
}

// syntaxVerb reads a verb written as net/http's constant (http.MethodPost)
// or as a string literal, and returns the empty string for anything else.
func syntaxVerb(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		if name, isMethod := strings.CutPrefix(typed.Sel.Name, httpMethodPrefix); isMethod {
			return strings.ToUpper(name)
		}
	case *ast.BasicLit:
		if verb, err := strconv.Unquote(typed.Value); err == nil {
			return verb
		}
	}
	return ""
}

// buildUpstreamWaits ties every workaround found to the entries that list
// its site, evaluates every entry's predicates, and returns the section with
// the entries that no longer describe the tree.
func buildUpstreamWaits(in waitsInput) (report upstreamWaitsReport, stale []string) {
	listedBy := map[string][]int{}
	for i, entry := range in.entries {
		for _, site := range entry.Sites {
			listedBy[site] = append(listedBy[site], i)
		}
	}
	report = upstreamWaitsReport{Entries: make([]waitEntry, 0, len(in.entries)), Workarounds: make([]workaround, 0, len(in.found))}
	bySite := map[string][]workaround{}
	for _, found := range in.found {
		found = answerWorkaround(found, listedBy[found.Site], in)
		bySite[found.Site] = append(bySite[found.Site], found)
		report.Workarounds = append(report.Workarounds, found)
	}
	slices.SortFunc(report.Workarounds, func(a, b workaround) int {
		return cmp.Or(strings.Compare(a.Site, b.Site), strings.Compare(a.Form, b.Form), strings.Compare(a.Constructor+a.Verb, b.Constructor+b.Verb))
	})

	stale = waits.Problems(in.entries)
	for _, entry := range in.entries {
		judged := judgeEntry(entry, in.sdk)
		report.Entries = append(report.Entries, judged)
		stale = append(stale, staleWait(entry, judged, in, bySite)...)
	}
	slices.Sort(stale)
	return report, stale
}

// answerWorkaround ties one workaround to the entries listed (by index into
// in.entries) that name its site, and records what of it they leave
// unexplained: a body key no predicate's field is written under, a need of
// a superset no predicate spells, and, once one of them carries a
// TakesNoOptions, a method the option is handed to that none names. A body
// edit whose keys the walk could not read is marked so.
func answerWorkaround(found workaround, listed []int, in waitsInput) workaround {
	keys, spelled, noOptions := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, i := range listed {
		found.AnsweredBy = append(found.AnsweredBy, in.entries[i].Label())
		for _, predicate := range in.entries[i].Predicates {
			spelled[predicate.String()] = true
			if key, isKey := waits.BodyKey(predicate, in.sdk); isKey {
				keys[key] = true
			}
			if takes, isTakes := predicate.(waits.TakesNoOptions); isTakes {
				noOptions[takes.Method] = true
			}
		}
	}
	found.AnsweredBy = slices.Compact(slices.Sorted(slices.Values(found.AnsweredBy)))
	found.UnansweredKeys = missing(found.Keys, keys)
	for _, superset := range slices.Concat(found.Decodes, found.Sends) {
		found.UnansweredNeeds = append(found.UnansweredNeeds, missing(superset.Needs, spelled)...)
	}
	if len(noOptions) > 0 {
		found.UnansweredMethods = missing(found.PassedTo, noOptions)
	}
	found.KeysUnread = found.Form == formBodyEdit && len(found.Keys) == 0
	return found
}

// missing returns the elements of list set does not hold, nil for none.
func missing(list []string, set map[string]bool) []string {
	var out []string
	for _, element := range list {
		if !set[element] {
			out = append(out, element)
		}
	}
	return out
}

// judgeEntry evaluates an entry's predicates. A predicate that names
// something the SDK lacks does not hold; [staleWait] reports why.
func judgeEntry(entry waits.Entry, sdk waits.SDK) waitEntry {
	judged := waitEntry{
		Row: entry.Row, Category: entry.Category, Sites: entry.Sites, Unseen: entry.Unseen,
		Replaces: entry.Replaces, Status: waitDeclared, Reason: entry.Reason,
	}
	if entry.Category != waits.CategoryUpstream {
		return judged
	}
	holding := 0
	for _, predicate := range entry.Predicates {
		verdict := waits.Evaluate(predicate, sdk)
		judged.Predicates = append(judged.Predicates, verdict)
		if verdict.Holds {
			holding++
		}
	}
	judged.Status = waitStalePredicate
	if holding == len(entry.Predicates) {
		judged.Status = waitHolds
	}
	if holding == 0 {
		judged.Status = waitObsolete
	}
	return judged
}

// siteClause follows an entry's label in what staleWait reports about one of
// its sites.
const siteClause = ": site "

// staleWait lists what of one entry no longer describes the tree: a site
// that no longer exists or that the walk finds no workaround at, an unseen
// site the walk does see or whose strings name no key a field predicate of
// the entry is about, a row the register does not carry or whose text names
// nothing the entry is about, a method client-go does not declare or a
// handler calls, a predicate that names something client-go lacks, a
// predicate on a superset's root naming a key no superset at the entry's
// sites carries, and a TakesNoOptions naming a method no site of the entry
// is handed to.
func staleWait(entry waits.Entry, judged waitEntry, in waitsInput, bySite map[string][]workaround) []string {
	label := "wait " + entry.Label()
	var stale []string
	for _, site := range entry.Sites {
		if !in.sites[site] {
			stale = append(stale, label+siteClause+site+" no longer exists")
			continue
		}
		if len(bySite[site]) == 0 {
			stale = append(stale, label+siteClause+site+" carries no workaround the walk finds")
		}
	}
	for _, site := range entry.Unseen {
		if !in.sites[site] {
			stale = append(stale, label+siteClause+site+" no longer exists")
			continue
		}
		if len(bySite[site]) != 0 {
			stale = append(stale, label+": the walk finds a workaround at "+site+", so it belongs among the sites")
			continue
		}
		stale = append(stale, unnamedKeys(label, site, entry, in)...)
	}
	stale = append(stale, staleRow(label, entry, in.rows)...)
	for _, key := range entry.Replaces {
		if !strings.Contains(key, ".") {
			continue
		}
		if err := in.sdk.DeclaresMethod(key); err != nil {
			stale = append(stale, label+": replaces "+key+": "+err.Error())
			continue
		}
		if in.called[key] {
			stale = append(stale, label+": replaces "+key+", which a handler calls")
		}
	}
	for _, verdict := range judged.Predicates {
		if verdict.Error != "" {
			stale = append(stale, label+": "+verdict.Predicate+" names what client-go lacks: "+verdict.Error)
		}
	}
	return append(stale, unreadReasons(label, entry, in.sdk, bySite)...)
}

// unnamedKeys lists, for an unseen site, each key a field predicate of the
// entry is about that no string of the site's body names: an error hint
// that stopped telling the caller what to send no longer carries the
// workaround the entry says it does.
func unnamedKeys(label, site string, entry waits.Entry, in waitsInput) []string {
	var stale []string
	for _, predicate := range entry.Predicates {
		key, isKey := waits.BodyKey(predicate, in.sdk)
		if isKey && !slices.ContainsFunc(in.strings[site], func(s string) bool { return strings.Contains(s, key) }) {
			stale = append(stale, label+": unseen site "+site+" holds no string naming "+key+", the key "+predicate.String()+" is about")
		}
	}
	return stale
}

// staleRow lists what is wrong with the register row an upstream entry
// waits on: a row the register does not carry, and one whose text names
// nothing the entry is about, which is what a renumbering that left the
// entry pointing at another row's problem looks like.
func staleRow(label string, entry waits.Entry, rows map[int]string) []string {
	if entry.Category != waits.CategoryUpstream || entry.Row <= 0 {
		return nil
	}
	text, hasRow := rows[entry.Row]
	if !hasRow {
		return []string{label + ": " + waits.RegisterPath + " has no row " + strconv.Itoa(entry.Row)}
	}
	if !entry.NamedIn(text) {
		return []string{label + ": row " + strconv.Itoa(entry.Row) + " of " + waits.RegisterPath + " names nothing the entry is about (" + strings.Join(entry.Names(), ", ") + ")"}
	}
	return nil
}

// unreadReasons lists each predicate of the entry that names a reason the
// workarounds at its sites do not have: a LacksKey or a FieldKind on the
// root of a superset at a site naming a key no such superset carries, and a
// TakesNoOptions naming a method no request option at a site is handed to.
// A misspelled key or a method that moved would otherwise hold for nothing.
func unreadReasons(label string, entry waits.Entry, sdk waits.SDK, bySite map[string][]workaround) []string {
	carried, handed := map[string]map[string]bool{}, map[string]bool{}
	for _, site := range entry.Sites {
		for _, found := range bySite[site] {
			for _, superset := range slices.Concat(found.Decodes, found.Sends) {
				if carried[superset.Root] == nil {
					carried[superset.Root] = map[string]bool{}
				}
				for _, key := range superset.Keys {
					carried[superset.Root][key] = true
				}
			}
			for _, method := range found.PassedTo {
				handed[method] = true
			}
		}
	}
	var stale []string
	for _, predicate := range entry.Predicates {
		keys, onRoot := carried[predicate.Subject()]
		if key, isKey := carriedKey(predicate, sdk); onRoot && isKey && !keys[key] {
			stale = append(stale, label+": "+predicate.String()+" names a key no superset at the entry's sites carries beside "+predicate.Subject())
		}
		if takes, isTakes := predicate.(waits.TakesNoOptions); isTakes && !handed[takes.Method] {
			stale = append(stale, label+": "+predicate.String()+" names a method no site of the entry is handed to")
		}
	}
	return stale
}

// carriedKey returns the key a LacksKey or a FieldKind is about, the two
// predicates a superset's keys need, and false for any other predicate.
func carriedKey(predicate waits.Predicate, sdk waits.SDK) (string, bool) {
	if lacks, isLacks := predicate.(waits.LacksKey); isLacks {
		return lacks.Key, true
	}
	if _, isKind := predicate.(waits.FieldKind); isKind {
		return waits.BodyKey(predicate, sdk)
	}
	return "", false
}

// withCoveredRawServices adds to the service declarations the services the
// entries replace, as COVERED_RAW, and lists a service both declare as a
// declaration said twice, and a COVERED_RAW declaredServices writes by hand,
// which no entry holds to the pinned SDK, as one said in the wrong place.
func withCoveredRawServices(declaredBy map[string]declaration, entries []waits.Entry) (out map[string]declaration, twice []string) {
	out = make(map[string]declaration, len(declaredBy))
	maps.Copy(out, declaredBy)
	for name, declared := range declaredBy {
		if declared.Category == coveredRaw {
			twice = append(twice, "service "+name+" (declared COVERED_RAW in declaredServices, which only a wait declares)")
		}
	}
	for name, reason := range waits.CoveredRawServices(entries) {
		if _, declared := out[name]; declared {
			twice = append(twice, "service "+name+" (declared in declaredServices and replaced by a wait)")
			continue
		}
		out[name] = declaration{Category: coveredRaw, Reason: reason}
	}
	slices.Sort(twice)
	return out, twice
}

// summarizeUpstreamWaits counts the section into the summary.
func summarizeUpstreamWaits(summary *reportSummary, report upstreamWaitsReport) {
	summary.UpstreamWaits = len(report.Entries)
	// Ifs rather than a switch: a case expression carries no statement
	// counter of its own, so a mutation tool leaves its mutant unrun.
	for _, entry := range report.Entries {
		if entry.Status == waitHolds {
			summary.WaitsHolding++
		}
		if entry.Status == waitStalePredicate {
			summary.WaitsStalePredicate++
		}
		if entry.Status == waitObsolete {
			summary.WaitsObsolete++
		}
	}
	summary.Workarounds = len(report.Workarounds)
	for _, found := range report.Workarounds {
		if len(found.AnsweredBy) == 0 {
			summary.WorkaroundsUndeclared++
		}
		if found.KeysUnread {
			summary.BodyEditsUnread++
		}
		summary.BodyEditKeysUnanswered += len(found.UnansweredKeys)
		summary.SupersetNeedsUnanswered += len(found.UnansweredNeeds)
		summary.OptionMethodsUnanswered += len(found.UnansweredMethods)
	}
}

// keepWaitFindings keeps the entries a predicate failed in and the
// workarounds nothing answers in full.
func keepWaitFindings(report upstreamWaitsReport) upstreamWaitsReport {
	out := upstreamWaitsReport{Entries: make([]waitEntry, 0), Workarounds: make([]workaround, 0)}
	for _, entry := range report.Entries {
		if entry.Status == waitStalePredicate || entry.Status == waitObsolete {
			out.Entries = append(out.Entries, entry)
		}
	}
	for _, found := range report.Workarounds {
		unexplained := len(found.UnansweredKeys) + len(found.UnansweredNeeds) + len(found.UnansweredMethods)
		if len(found.AnsweredBy) == 0 || unexplained > 0 || found.KeysUnread {
			out.Workarounds = append(out.Workarounds, found)
		}
	}
	return out
}
