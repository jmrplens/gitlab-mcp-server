// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"go/ast"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// Names the capture walk recognizes things by: the package that captures a
// response, the capture's type, the function that starts one, the method
// that decodes it, and the option that hands a request its context.
const (
	gitlabPathSuffix        = "/internal/gitlab"
	responseCaptureName     = "ResponseCapture"
	withResponseCaptureName = "WithResponseCapture"
	captureDecodeName       = "Decode"
	// answerResults is how many results a service method that decodes a
	// value answers with: the value, the response and the error.
	answerResults = 3
)

// What a captured key came to. A key the client-go struct beside it also
// models is redundant unless a declaration says why it is read twice. A
// declaration some of whose predicates stopped holding is one to edit, since
// the key is still read twice for the rest; one none of whose predicates
// holds is the SDK now reading the key the way the capture does, so the
// capture is to retire.
const (
	captureRedundant      = "redundant_capture"
	captureKept           = "kept"
	captureStalePredicate = "stale_predicate"
	captureObsolete       = "obsolete_capture"
)

// What a struct a capture is decoded into comes to when the walk pairs it
// with no client-go struct. One each of whose captures had a service method
// sent under it, none answering with a struct, has no SDK half to compare:
// the method answers with the response and the error alone, or with a value
// that is no struct. Any other is a pairing the walk does not see, so a key
// the SDK models beside it would be read twice with nothing to say so.
const (
	captureAnswersNoStruct = "answers_no_struct"
	captureUnpaired        = "unpaired_capture"
)

// capturesReport is the captures section of the report.
type capturesReport struct {
	// Types are the structs a tool package decodes off a captured response,
	// each with the client-go structs decoded from the same response.
	Types []capturedType `json:"types"`
	// Keys are the keys a captured struct reads that a client-go struct it
	// is paired with models too, each with what answers it.
	Keys []capturedKey `json:"keys"`
	// Unpaired are the structs a capture is decoded into that the walk pairs
	// with no client-go struct, each with what that comes to.
	Unpaired []unpairedCapture `json:"unpaired"`
}

// unpairedCapture is one struct a capture is decoded into that the walk
// pairs with no client-go struct.
type unpairedCapture struct {
	Type string `json:"type"`
	// Sites are the functions it is decoded in.
	Sites  []string `json:"sites"`
	Status string   `json:"status"`
}

// capturedType is one struct decoded off a captured response.
type capturedType struct {
	// Type is the struct, spelled with its package's name, or, for a struct
	// literal, with the function it is written in and its fields' names.
	Type string `json:"type"`
	// Pairs are the client-go structs decoded from the same response,
	// spelled as client-go names them.
	Pairs []string `json:"pairs"`
	// Sites are the functions the pairings were read in: a converter that
	// takes both, or a function that sends the request under the capture
	// and decodes it.
	Sites []string `json:"sites"`
}

// capturedKey is one key a captured struct reads that the client-go struct
// beside it models too.
type capturedKey struct {
	Type string `json:"type"`
	Key  string `json:"key"`
	// Captured spells the captured field's type, Pairs are the client-go
	// structs paired with the type that model the key, and Models what
	// models it on each, at the key's depth.
	Captured string         `json:"captured"`
	Pairs    []string       `json:"pairs"`
	Models   []waits.Shared `json:"models"`
	Status   string         `json:"status"`
	// Declared, Category, Predicates and Reason are the declaration's, when
	// one answers the key: Declared is the key it is written on, the key
	// itself or an object above it.
	Declared   string          `json:"declared,omitempty"`
	Category   string          `json:"category,omitempty"`
	Predicates []waits.Verdict `json:"predicates,omitempty"`
	Reason     string          `json:"reason,omitempty"`
}

// capturePair is one captured struct, a named struct or a struct literal,
// and one client-go struct decoded from the same response.
type capturePair struct {
	captured types.Type
	sdk      *types.Named
}

// captureWalk finds what the tool packages and the shared toolutil package
// decode off a captured response and what each is paired with.
type captureWalk struct {
	sdk waits.SDK
	// module is the import path every package of the repository's own
	// starts with.
	module string
	// capture is the capture's type, nil when no tool package captures.
	capture *types.Named
	// responses holds every client-go struct a service method answers
	// with, and every one such a struct holds, which is what a response is
	// decoded into.
	responses map[*types.Named]bool
	// decoders maps a function, by its generic origin, to the types each of
	// its capture parameters is decoded into, by the parameter's index. A
	// type may be one of the function's own type parameters, read through
	// the instantiation at each call.
	decoders map[types.Object]map[int]map[types.Type]bool
	// literals spells each struct literal a capture is decoded into, by the
	// function it is written in.
	literals map[*types.Struct]string
	// decodedIn maps every struct a capture is decoded into, a named struct
	// of the repository's own or a struct literal, to the functions it is
	// decoded in.
	decodedIn map[types.Type]map[string]bool
	// flows counts, for each such struct, the captures a function started
	// and decoded into it, and answeredNone those of them a service method
	// sent under answered with no struct.
	flows, answeredNone map[types.Type]int
	// pairs maps each pairing to the functions it was read in.
	pairs map[capturePair]map[string]bool
}

// analyzeCaptures walks pkgs and the shared toolutil package for the structs
// decoded off a captured response, pairs each with the client-go structs
// decoded from the same response, and holds every key both read to a
// declaration.
func analyzeCaptures(pkgs []*packages.Package, clientGo *types.Package, declared []captureDeclaration) (report capturesReport, stale []string) {
	walk := newCaptureWalk(pkgs, clientGo)
	return buildCaptures(walk, declared)
}

// newCaptureWalk walks pkgs and the bodies of the shared toolutil package
// they import, which the load carries from its source.
func newCaptureWalk(pkgs []*packages.Package, clientGo *types.Package) *captureWalk {
	walk := &captureWalk{
		sdk:          waits.SDK{Types: clientGo},
		responses:    responseStructs(clientGo),
		decoders:     map[types.Object]map[int]map[types.Type]bool{},
		literals:     map[*types.Struct]string{},
		decodedIn:    map[types.Type]map[string]bool{},
		flows:        map[types.Type]int{},
		answeredNone: map[types.Type]int{},
		pairs:        map[capturePair]map[string]bool{},
	}
	if gitlab := importedPackage(pkgs, gitlabPathSuffix); gitlab != nil {
		walk.capture = captureType(gitlab)
		walk.module = strings.TrimSuffix(gitlab.Path(), gitlabPathSuffix)
	}
	if walk.capture == nil {
		return walk
	}
	bodies := packageBodies(pkgs)
	if toolutil := toolutilSource(pkgs); toolutil != nil {
		bodies = append(bodies, packageBodies([]*packages.Package{toolutil})...)
	}
	walk.readDecoders(bodies)
	readers := walk.readerOutputs()
	for _, body := range bodies {
		decoded := walk.decodedVars(body)
		walk.recordDecoded(body.site, decoded)
		walk.pairSignature(body.fn.signature, body.site, readers, walk.decoders[body.fn.object])
		walk.pairFlow(body, decoded)
	}
	return walk
}

// toolutilSource returns the shared toolutil package the tool packages
// import, which the load carries with its syntax, nil when none imports it.
func toolutilSource(pkgs []*packages.Package) *packages.Package {
	for _, pkg := range pkgs {
		for path, imported := range pkg.Imports {
			if strings.HasSuffix(path, toolutilPathSuffix) {
				return imported
			}
		}
	}
	return nil
}

// captureType returns the capture's type, nil when the package that captures
// a response declares none.
func captureType(gitlab *types.Package) *types.Named {
	obj, _ := gitlab.Scope().Lookup(responseCaptureName).(*types.TypeName)
	if obj == nil {
		return nil
	}
	named, _ := obj.Type().(*types.Named)
	return named
}

// responseStructs returns every client-go struct a method of a service the
// Client publishes answers with, and every client-go struct one of those
// holds through a field, at any depth.
func responseStructs(clientGo *types.Package) map[*types.Named]bool {
	out := map[*types.Named]bool{}
	var queue []*types.Named
	add := func(t types.Type) {
		if named, isStruct := clientGoStruct(captureElement(t)); isStruct && !out[named] {
			out[named] = true
			queue = append(queue, named)
		}
	}
	client, _ := shared.ClientStruct(clientGo)
	for field := range client.Fields() {
		service, isService := shared.ClientGoServiceInterface(field.Type())
		if !isService {
			continue
		}
		iface, _ := service.Underlying().(*types.Interface)
		for method := range iface.Methods() {
			// A method answers with its value, the response and the error;
			// one that answers with the response and the error alone
			// decodes nothing.
			if results := method.Signature().Results(); shared.SignatureIsAPICall(method.Signature()) && results.Len() == answerResults {
				add(results.At(0).Type())
			}
		}
	}
	for len(queue) > 0 {
		st, _ := queue[0].Underlying().(*types.Struct)
		queue = queue[1:]
		for field := range st.Fields() {
			add(field.Type())
		}
	}
	return out
}

// captureElement returns what a value of type t holds one of: the type a
// pointer points to, the element of a slice, an array or a map, or t itself.
func captureElement(t types.Type) types.Type {
	switch typed := t.Underlying().(type) {
	case *types.Pointer:
		return captureElement(typed.Elem())
	case *types.Slice:
		return captureElement(typed.Elem())
	case *types.Array:
		return captureElement(typed.Elem())
	case *types.Map:
		return captureElement(typed.Elem())
	}
	return t
}

// toolBody is one function body of a tool package or of the shared toolutil
// package, with what reading it needs.
type toolBody struct {
	fn   funcBody
	info *types.Info
	site string
}

// packageBodies lists every body of pkgs.
func packageBodies(pkgs []*packages.Package) []toolBody {
	var out []toolBody
	for _, pkg := range pkgs {
		for _, fn := range functionBodies(pkg) {
			out = append(out, toolBody{fn: fn, info: pkg.TypesInfo, site: shared.ShortPackage(pkg.PkgPath) + "." + fn.name})
		}
	}
	return out
}

// isCapture reports whether t is a pointer to the capture.
func (w *captureWalk) isCapture(t types.Type) bool {
	pointer, isPointer := t.(*types.Pointer)
	return isPointer && types.Identical(pointer.Elem(), w.capture)
}

// addDecoded records that fn decodes its i-th parameter into decoded, and
// reports whether that is new.
func (w *captureWalk) addDecoded(fn types.Object, i int, decoded types.Type) bool {
	byIndex := w.decoders[fn]
	if byIndex == nil {
		byIndex = map[int]map[types.Type]bool{}
		w.decoders[fn] = byIndex
	}
	if byIndex[i] == nil {
		byIndex[i] = map[types.Type]bool{}
	}
	if byIndex[i][decoded] {
		return false
	}
	byIndex[i][decoded] = true
	return true
}

// readDecoders reads, to a fixed point, which capture parameter of each body
// is decoded into what: directly through Decode, or by handing it to a
// function that decodes it.
func (w *captureWalk) readDecoders(bodies []toolBody) {
	for changed := true; changed; {
		changed = false
		for _, body := range bodies {
			decoded := w.decodedVars(body)
			for i := range body.fn.signature.Params().Len() {
				for t := range decoded[body.fn.signature.Params().At(i)] {
					changed = w.addDecoded(body.fn.object, i, t) || changed
				}
			}
		}
	}
}

// decodedVars returns, for each capture variable of body, the types it is
// decoded into in body. A struct literal is spelled the first time one is
// read, which is in the function it is written in: a call hands one on only
// once that function has decoded into it.
func (w *captureWalk) decodedVars(body toolBody) map[*types.Var]map[types.Type]bool {
	out := map[*types.Var]map[types.Type]bool{}
	add := func(v *types.Var, t types.Type) {
		if v == nil {
			return
		}
		if literal, isLiteral := t.(*types.Struct); isLiteral && w.literals[literal] == "" {
			w.literals[literal] = literalName(body.site, literal)
		}
		if out[v] == nil {
			out[v] = map[types.Type]bool{}
		}
		out[v][t] = true
	}
	ast.Inspect(body.fn.body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if receiver, target, isDecode := w.decodeCall(body.info, call); isDecode {
			add(receiver, captureElement(target))
			return true
		}
		for i, into := range w.decoders[calledObject(body.info, call)] {
			// A call handed the values of another call, f(g()), names no
			// variable.
			if i >= len(call.Args) {
				continue
			}
			arg, _ := ast.Unparen(call.Args[i]).(*ast.Ident)
			for t := range into {
				add(identVar(body.info, arg), instantiated(body.info, call, t))
			}
		}
		return true
	})
	return out
}

// literalName spells a struct literal written in the function site names by
// that function and its fields' names.
func literalName(site string, literal *types.Struct) string {
	names := make([]string, 0, literal.NumFields())
	for field := range literal.Fields() {
		names = append(names, field.Name())
	}
	return site + ".struct{" + strings.Join(names, ", ") + "}"
}

// decodeCall reports whether call decodes a capture variable, and returns the
// variable and the type of what it decodes into.
func (w *captureWalk) decodeCall(info *types.Info, call *ast.CallExpr) (*types.Var, types.Type, bool) {
	sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != captureDecodeName || len(call.Args) != 1 || !w.isCapture(info.TypeOf(sel.X)) {
		return nil, nil, false
	}
	receiver, _ := ast.Unparen(sel.X).(*ast.Ident)
	return identVar(info, receiver), info.TypeOf(call.Args[0]), true
}

// instantiated returns t, a type parameter of the function call calls
// replaced by what call instantiates it with. A method of a generic type
// records no instance of its own, so its type parameter is left as it is,
// which is no captured struct.
func instantiated(info *types.Info, call *ast.CallExpr, t types.Type) types.Type {
	param, isTypeParam := t.(*types.TypeParam)
	if !isTypeParam {
		return t
	}
	args := info.Instances[calledIdent(call)].TypeArgs
	if param.Index() >= args.Len() {
		return t
	}
	return captureElement(args.At(param.Index()))
}

// recordDecoded records that what each capture variable of the function site
// names is decoded into, in decoded, is decoded there, when it is a struct
// of the repository's own.
func (w *captureWalk) recordDecoded(site string, decoded map[*types.Var]map[types.Type]bool) {
	for _, into := range decoded {
		for t := range into {
			if !w.ownStruct(t) {
				continue
			}
			if w.decodedIn[t] == nil {
				w.decodedIn[t] = map[string]bool{}
			}
			w.decodedIn[t][site] = true
		}
	}
}

// ownStruct reports whether t is a struct a capture is decoded into for the
// keys client-go does not read: a named struct client-go does not declare,
// or a struct literal. A type parameter left unresolved is neither.
func (w *captureWalk) ownStruct(t types.Type) bool {
	switch typed := t.(type) {
	case *types.Named:
		_, isStruct := typed.Underlying().(*types.Struct)
		return isStruct && typed.Obj().Pkg() != w.sdk.Types
	case *types.Struct:
		return true
	}
	return false
}

// spell spells a captured struct: a named one with its package's name, a
// struct literal by the function it is written in.
func (w *captureWalk) spell(t types.Type) string {
	if named, isNamed := t.(*types.Named); isNamed {
		return spellNamed(named)
	}
	literal, _ := t.(*types.Struct)
	return w.literals[literal]
}

// readerOutputs returns every struct a function decodes a capture it is
// handed into, which is what a reader hands its caller.
func (w *captureWalk) readerOutputs() map[types.Type]bool {
	out := map[types.Type]bool{}
	for _, byIndex := range w.decoders {
		for _, decoded := range byIndex {
			for t := range decoded {
				out[t] = w.ownStruct(t)
			}
		}
	}
	return out
}

// pairSignature pairs each struct a reader decodes a capture into that a
// function of signature takes, or that it decodes a capture parameter into,
// with each response struct it takes: a converter that takes both reads the
// two halves of one response. A struct decoded off a capture the function
// that decodes it started is paired by what that function sends, and a
// converter it hands the struct to beside a client-go struct may be handed
// an object inside the answer (a package file's extra beside the file's
// pipelines), so it is not paired again there.
func (w *captureWalk) pairSignature(signature *types.Signature, site string, readers map[types.Type]bool, decodes map[int]map[types.Type]bool) {
	var responses []*types.Named
	var own []types.Type
	for i := range signature.Params().Len() {
		t := captureElement(signature.Params().At(i).Type())
		if named, isStruct := clientGoStruct(t); isStruct && w.responses[named] {
			responses = append(responses, named)
		}
		if readers[t] {
			own = append(own, t)
		}
		for decoded := range decodes[i] {
			if w.ownStruct(decoded) {
				own = append(own, decoded)
			}
		}
	}
	w.pair(own, responses, site)
}

// pair records every pairing of own with responses as read at site.
func (w *captureWalk) pair(own []types.Type, responses []*types.Named, site string) {
	for _, captured := range own {
		for _, response := range responses {
			key := capturePair{captured: captured, sdk: response}
			if w.pairs[key] == nil {
				w.pairs[key] = map[string]bool{}
			}
			w.pairs[key][site] = true
		}
	}
}

// pairFlow pairs, in body, what each capture is decoded into with what each
// call sent under it answers with: a service method handed, through
// client-go's WithContext, a context a capture started on, or a function
// handed that context itself, which sends for its caller and whose answer is
// what it sent, by the last capture written before the call on that context.
// ast.Inspect visits the body in the order it is written, so the capture a
// context holds when a call is reached is the one started last above it. It
// then counts, for what each capture started here is decoded into, whether
// a service method sent under the capture answered with no struct. One
// nothing visible was sent under counts as not, since its request may have
// gone where the walk does not follow (a context kept in a field); one also
// sent under a call that answered with a struct has paired all it is
// decoded into, so what the count says of it is never read.
func (w *captureWalk) pairFlow(body toolBody, decoded map[*types.Var]map[types.Type]bool) {
	current := map[*types.Var]*types.Var{}
	// answeredNone holds each capture started here, and whether a service
	// method sent under it answered with no struct.
	answeredNone := map[*types.Var]bool{}
	ast.Inspect(body.fn.body, func(node ast.Node) bool {
		if assign, isAssign := node.(*ast.AssignStmt); isAssign {
			if capture := w.recordStart(body.info, assign, current); capture != nil {
				answeredNone[capture] = false
			}
			return true
		}
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		capture, response, service := w.sentUnder(body.info, call, current)
		if capture == nil {
			return true
		}
		if response == nil {
			answeredNone[capture] = answeredNone[capture] || service
			return true
		}
		var own []types.Type
		for t := range decoded[capture] {
			if w.ownStruct(t) {
				own = append(own, t)
			}
		}
		w.pair(own, []*types.Named{response}, body.site)
		return true
	})
	for capture, none := range answeredNone {
		for t := range decoded[capture] {
			w.flows[t]++
			if none {
				w.answeredNone[t]++
			}
		}
	}
}

// recordStart records in current, when assign starts a capture on a context
// held in a variable, that the variable's requests are captured by it from
// here on, and returns the capture's variable, nil for none. The function
// answers a context and a capture, so an assignment of its call has those
// two on its left and the call alone on its right.
func (w *captureWalk) recordStart(info *types.Info, assign *ast.AssignStmt, current map[*types.Var]*types.Var) *types.Var {
	call, isCall := ast.Unparen(assign.Rhs[0]).(*ast.CallExpr)
	if !isCall || !w.startsCapture(calledObject(info, call)) {
		return nil
	}
	ctx := assignedVar(info, assign.Lhs[0])
	if ctx == nil {
		return nil
	}
	current[ctx] = assignedVar(info, assign.Lhs[1])
	return current[ctx]
}

// sentUnder returns the capture call sends under, nil for none, the
// client-go struct a response is decoded into that call answers with, nil
// for none, and whether call is a service method. A service method sends
// under the capture of the context client-go's WithContext hands it; any
// other call sends under the capture of a context it is handed itself.
func (w *captureWalk) sentUnder(info *types.Info, call *ast.CallExpr, current map[*types.Var]*types.Var) (capture *types.Var, response *types.Named, service bool) {
	sel, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if method := serviceMethod(info, sel); method != nil {
		return w.captureOf(info, call, current), w.answered(method.Signature().Results()), true
	}
	for _, arg := range call.Args {
		ident, _ := ast.Unparen(arg).(*ast.Ident)
		if handed := current[identVar(info, ident)]; handed != nil {
			return handed, w.answered(info.TypeOf(call)), false
		}
	}
	return nil, nil, false
}

// answered returns the client-go struct a response is decoded into that a
// call answering with t answers with first, or a list of, nil for none. t is
// the call's results, or its one result.
func (w *captureWalk) answered(t types.Type) *types.Named {
	if results, isTuple := t.(*types.Tuple); isTuple {
		if results.Len() == 0 {
			return nil
		}
		t = results.At(0).Type()
	}
	named, _ := clientGoStruct(captureElement(t))
	if !w.responses[named] {
		return nil
	}
	return named
}

// startsCapture reports whether obj is the function that starts a capture.
func (w *captureWalk) startsCapture(obj types.Object) bool {
	fn, isFunc := obj.(*types.Func)
	return isFunc && fn.Name() == withResponseCaptureName && fn.Pkg() == w.capture.Obj().Pkg()
}

// assignedVar returns the variable an assignment's left-hand side declares or
// assigns, nil for anything else.
func assignedVar(info *types.Info, lhs ast.Expr) *types.Var {
	ident, _ := ast.Unparen(lhs).(*ast.Ident)
	if ident == nil {
		return nil
	}
	if defined, isVar := info.Defs[ident].(*types.Var); isVar {
		return defined
	}
	return identVar(info, ident)
}

// captureOf returns the capture call sends under: the one the context it
// hands its request through client-go's WithContext holds, nil for none.
func (w *captureWalk) captureOf(info *types.Info, call *ast.CallExpr, current map[*types.Var]*types.Var) *types.Var {
	for _, arg := range call.Args {
		option, isCall := ast.Unparen(arg).(*ast.CallExpr)
		if !isCall || !w.isWithContext(calledObject(info, option)) {
			continue
		}
		ctx, _ := ast.Unparen(option.Args[0]).(*ast.Ident)
		if capture := current[identVar(info, ctx)]; capture != nil {
			return capture
		}
	}
	return nil
}

// isWithContext reports whether obj is client-go's WithContext.
func (w *captureWalk) isWithContext(obj types.Object) bool {
	fn, isFunc := obj.(*types.Func)
	return isFunc && fn.Name() == withContextName && fn.Pkg() == w.sdk.Types
}

// buildCaptures holds every key a captured struct shares with a client-go
// struct it is paired with to the declarations, and lists the declarations
// that no longer describe the tree.
func buildCaptures(walk *captureWalk, declared []captureDeclaration) (report capturesReport, stale []string) {
	report = capturesReport{Types: make([]capturedType, 0), Keys: make([]capturedKey, 0), Unpaired: unpairedCaptures(walk)}
	byType := map[types.Type]*capturedType{}
	in := captureJudging{sdk: walk.sdk, keys: map[string]*capturedKey{}, pairs: map[string][]string{}, lacks: map[string][]waits.Lack{}}
	for pair, sites := range walk.pairs {
		entry := byType[pair.captured]
		if entry == nil {
			entry = &capturedType{Type: walk.spell(pair.captured)}
			byType[pair.captured] = entry
		}
		entry.Pairs = append(entry.Pairs, pair.sdk.Obj().Name())
		entry.Sites = append(entry.Sites, slices.Collect(maps.Keys(sites))...)
		own, _ := pair.captured.Underlying().(*types.Struct)
		comparison := waits.Compare(walk.sdk, walk.module, own, pair.sdk)
		in.lacks[entry.Type] = append(in.lacks[entry.Type], comparison.Lacks...)
		for _, shared := range comparison.Shared {
			key := in.keys[entry.Type+"."+shared.Key]
			if key == nil {
				key = &capturedKey{Type: entry.Type, Key: shared.Key, Captured: shared.Captured, Status: captureRedundant}
				in.keys[entry.Type+"."+shared.Key] = key
			}
			key.Pairs = append(key.Pairs, pair.sdk.Obj().Name())
			key.Models = append(key.Models, shared)
		}
	}
	for _, entry := range byType {
		entry.Pairs = sortedUnique(entry.Pairs)
		entry.Sites = sortedUnique(entry.Sites)
		in.pairs[entry.Type] = entry.Pairs
		report.Types = append(report.Types, *entry)
	}
	slices.SortFunc(report.Types, func(a, b capturedType) int { return strings.Compare(a.Type, b.Type) })
	for _, key := range in.keys {
		key.Pairs = sortedUnique(key.Pairs)
		slices.SortFunc(key.Models, func(a, b waits.Shared) int { return cmp.Compare(a.Owner, b.Owner) })
		report.Keys = append(report.Keys, *key)
	}
	slices.SortFunc(report.Keys, func(a, b capturedKey) int {
		return cmp.Or(strings.Compare(a.Type, b.Type), strings.Compare(a.Key, b.Key))
	})
	return report, judgeCaptures(in, report.Keys, declared)
}

// unpairedCaptures lists every struct a capture is decoded into that the
// walk pairs with nothing, sorted: one each of whose captures was sent under
// a service method that answered with no struct has no SDK half, and any
// other is a pairing the walk does not see.
func unpairedCaptures(walk *captureWalk) []unpairedCapture {
	paired := map[types.Type]bool{}
	for pair := range walk.pairs {
		paired[pair.captured] = true
	}
	out := make([]unpairedCapture, 0)
	for t, sites := range walk.decodedIn {
		if paired[t] {
			continue
		}
		status := captureUnpaired
		if walk.flows[t] > 0 && walk.answeredNone[t] == walk.flows[t] {
			status = captureAnswersNoStruct
		}
		out = append(out, unpairedCapture{Type: walk.spell(t), Sites: slices.Sorted(maps.Keys(sites)), Status: status})
	}
	slices.SortFunc(out, func(a, b unpairedCapture) int { return strings.Compare(a.Type, b.Type) })
	return out
}

// spellNamed spells a named type with its package's name.
func spellNamed(named *types.Named) string {
	return types.TypeString(named, func(pkg *types.Package) string { return pkg.Name() })
}

// sortedUnique sorts list and drops its repeats.
func sortedUnique(list []string) []string {
	slices.Sort(list)
	return slices.Compact(list)
}

// summarizeCaptures fills the captures counters of summary.
func summarizeCaptures(summary *reportSummary, report capturesReport) {
	summary.CapturedTypes = len(report.Types)
	summary.CapturedKeysShared = len(report.Keys)
	counters := map[string]*int{
		captureKept: &summary.CapturesKept, captureRedundant: &summary.CapturesRedundant,
		captureStalePredicate: &summary.CapturesStalePredicate, captureObsolete: &summary.CapturesObsolete,
		captureAnswersNoStruct: &summary.CapturesAnswerNoStruct, captureUnpaired: &summary.CapturesUnpaired,
	}
	for _, key := range report.Keys {
		*counters[key.Status]++
	}
	for _, unpaired := range report.Unpaired {
		*counters[unpaired.Status]++
	}
}

// keepCaptureFindings keeps only the keys and the unpaired captures that are
// a finding.
func keepCaptureFindings(report capturesReport) capturesReport {
	keys := make([]capturedKey, 0)
	for _, key := range report.Keys {
		if key.Status != captureKept {
			keys = append(keys, key)
		}
	}
	unpaired := make([]unpairedCapture, 0)
	for _, capture := range report.Unpaired {
		if capture.Status == captureUnpaired {
			unpaired = append(unpaired, capture)
		}
	}
	return capturesReport{Types: make([]capturedType, 0), Keys: keys, Unpaired: unpaired}
}
