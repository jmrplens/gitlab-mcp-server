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

// doName is the method of client-go's Client that sends a request and
// decodes its answer into its second argument.
const doName = "Do"

// rawCaller is one call of a function that forwards its own verb or options
// parameter to a raw request it builds (doProjectRequest[T],
// rawMutateApprovalRule, groupboards.newRawRequest): where the call is, and
// the verb and the static type of the options the request then carries,
// empty when not a constant and for nil.
type rawCaller struct {
	Site    string `json:"site"`
	Verb    string `json:"verb,omitempty"`
	Options string `json:"options,omitempty"`
}

// rawRequest names one raw request: the site of the function that builds it
// and its place among the raw requests that site builds, in source order,
// which is the order the walk lists them in.
type rawRequest struct {
	site  string
	place int
}

// forwarder is a raw request that a function hands its own verb or options
// parameter: the request, the index of each parameter it forwards (-1 for
// one it does not), and what the request carries in place of one it does
// not.
type forwarder struct {
	request                 rawRequest
	verb, options           int
	verbValue, optionsValue string
}

// rawReads is what the walk of the tool packages reads around the
// workarounds it finds: by raw request, the types its answer is decoded
// into, the ones it sends as options and each call of the function that
// forwards it a verb or options; by site, the answers decoded from a request
// the walk cannot tell (one handed in, or built by another function), the
// type arguments a generic function is instantiated with, the client-go
// methods each function's result is handed to, and the strings each
// function's body holds; and the forwarding requests of each function.
type rawReads struct {
	sdk         waits.SDK
	decodes     map[rawRequest][]types.Type
	siteDecodes map[string][]types.Type
	sends       map[rawRequest][]types.Type
	forwarders  map[types.Object][]forwarder
	callers     map[rawRequest][]rawCaller
	instances   map[string][][]types.Type
	passedTo    map[string]map[string]bool
	strings     map[string][]string
	// placed counts the raw requests read at each site so far, so two bodies
	// a site names (a package's init functions) number theirs in turn.
	placed map[string]int
}

// newRawReads returns an empty rawReads for clientGo.
func newRawReads(clientGo *types.Package) *rawReads {
	return &rawReads{
		sdk:         waits.SDK{Types: clientGo},
		decodes:     map[rawRequest][]types.Type{},
		siteDecodes: map[string][]types.Type{},
		sends:       map[rawRequest][]types.Type{},
		forwarders:  map[types.Object][]forwarder{},
		callers:     map[rawRequest][]rawCaller{},
		instances:   map[string][][]types.Type{},
		passedTo:    map[string]map[string]bool{},
		strings:     map[string][]string{},
		placed:      map[string]int{},
	}
}

// read records what one function body holds: each string, each raw
// request's options and whether the function forwards its own to it, each
// answer a raw request decodes, and each call of a client-go service method
// with the functions whose results it is handed.
func (r *rawReads) read(scan bodyScan, fn funcBody, site string) {
	r.strings[site] = append(r.strings[site], syntaxStrings(fn.body)...)
	body := scan.requestsOf(fn.body, site, r.placed[site])
	r.placed[site] += len(body.places)
	ast.Inspect(fn.body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall {
			r.readCall(scan, fn, body, call)
		}
		return true
	})
}

// bodyRequests are the raw requests one function body builds: the place of
// each, and the place of the one each variable is assigned, -1 for a
// variable assigned more than one.
type bodyRequests struct {
	site   string
	places map[*ast.CallExpr]int
	vars   map[*types.Var]int
}

// requestAlias is one variable assigned another: to holds the request from
// holds, beside any it is assigned elsewhere.
type requestAlias struct {
	to, from *types.Var
}

// requestsOf numbers the raw requests body builds in source order, from
// first, and maps each variable one is assigned to to its place, directly
// or through another variable it is assigned.
func (s bodyScan) requestsOf(body *ast.BlockStmt, site string, first int) bodyRequests {
	found := bodyRequests{site: site, places: map[*ast.CallExpr]int{}, vars: map[*types.Var]int{}}
	ast.Inspect(body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall {
			if constructor, _ := s.rawConstructor(call); constructor != nil {
				found.places[call] = first + len(found.places)
			}
		}
		return true
	})
	var aliases []requestAlias
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.AssignStmt:
			found.bind(s.info, typed.Lhs, typed.Rhs)
			aliases = append(aliases, variableAliases(s.info, typed.Lhs, typed.Rhs)...)
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(typed.Names))
			for i, name := range typed.Names {
				names[i] = name
			}
			found.bind(s.info, names, typed.Values)
			aliases = append(aliases, variableAliases(s.info, names, typed.Values)...)
		}
		return true
	})
	found.follow(aliases)
	return found
}

// bind maps the variable the first of lhs names to the place of the raw
// request rhs builds, when rhs is the one call that builds one.
func (b bodyRequests) bind(info *types.Info, lhs, rhs []ast.Expr) {
	if len(lhs) == 0 || len(rhs) != 1 {
		return
	}
	call, _ := ast.Unparen(rhs[0]).(*ast.CallExpr)
	place, isRaw := b.places[call]
	if variable := assignedVar(info, lhs[0]); isRaw && variable != nil {
		b.assign(variable, place)
	}
}

// assign records that variable is assigned the request at place, and
// reports whether that changed what it holds: a variable assigned two
// requests holds -1, which no later assignment changes.
func (b bodyRequests) assign(variable *types.Var, place int) bool {
	bound, isBound := b.vars[variable]
	if isBound && bound != place {
		place = -1
	}
	if isBound && bound == place {
		return false
	}
	b.vars[variable] = place
	return true
}

// variableAliases returns each variable of lhs assigned the variable rhs
// names in its place, one value to each target.
func variableAliases(info *types.Info, lhs, rhs []ast.Expr) []requestAlias {
	if len(lhs) != len(rhs) {
		return nil
	}
	var out []requestAlias
	for i, target := range lhs {
		source, isIdent := ast.Unparen(rhs[i]).(*ast.Ident)
		if !isIdent {
			continue
		}
		to, from := assignedVar(info, target), identVar(info, source)
		if to != nil && from != nil && to != from {
			out = append(out, requestAlias{to: to, from: from})
		}
	}
	return out
}

// follow gives each variable assigned another the request the other holds,
// until a pass changes nothing, which it reaches because a variable's place
// changes at most twice: so first = second, with first assigned a request
// of its own, makes first a variable of two requests, and a call of Do
// reading it is counted against both.
func (b bodyRequests) follow(aliases []requestAlias) {
	for changed := true; changed; {
		changed = false
		for _, alias := range aliases {
			if place, isBound := b.vars[alias.from]; isBound {
				changed = b.assign(alias.to, place) || changed
			}
		}
	}
}

// sent returns the raw request expr, the request a call of Do sends, is: the
// one request every variable it reads is assigned, and false when it reads
// none, or several, or one assigned more than one.
func (b bodyRequests) sent(info *types.Info, expr ast.Expr) (rawRequest, bool) {
	read := map[int]bool{}
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			if place, isRequest := b.vars[identVar(info, ident)]; isRequest {
				read[place] = true
			}
		}
		return true
	})
	places := slices.Collect(maps.Keys(read))
	if len(places) != 1 || places[0] < 0 {
		return rawRequest{}, false
	}
	return rawRequest{site: b.site, place: places[0]}, true
}

// readCall records one call of the function body.
func (r *rawReads) readCall(scan bodyScan, fn funcBody, body bodyRequests, call *ast.CallExpr) {
	info := scan.info
	if constructor, verb, opt := scan.rawRequestArgs(call); constructor != nil {
		request := rawRequest{site: body.site, place: body.places[call]}
		forward := forwarder{
			request: request, verb: paramIndex(info, fn.signature, verb, false), options: paramIndex(info, fn.signature, opt, true),
			verbValue: constantString(info, verb), optionsValue: spellOptions(info, opt),
		}
		if forward.verb >= 0 || forward.options >= 0 {
			r.forwarders[fn.object] = append(r.forwarders[fn.object], forward)
		}
		// A forwarded parameter's own type is an interface, which is no
		// superset; what it carries is read off each caller.
		r.sends[request] = append(r.sends[request], info.TypeOf(opt))
		return
	}
	sel, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if decoded := doTarget(info, sel, call); decoded != nil {
		if request, isTold := body.sent(info, call.Args[0]); isTold {
			r.decodes[request] = append(r.decodes[request], decoded)
		} else {
			r.siteDecodes[body.site] = append(r.siteDecodes[body.site], decoded)
		}
		return
	}
	if method := serviceMethod(info, sel); method != nil {
		r.recordPassed(info, sel, method, call.Args)
	}
}

// paramIndex returns the index of the parameter of signature that expr
// names, and -1 when expr names none (nil included) or, with interfaceOnly,
// names one whose type is not an interface, whose value's static type the
// caller already knows.
func paramIndex(info *types.Info, signature *types.Signature, expr ast.Expr, interfaceOnly bool) int {
	ident, _ := expr.(*ast.Ident)
	variable := identVar(info, ident)
	params := signature.Params()
	for i := range params.Len() {
		if variable != nil && params.At(i) == variable && (!interfaceOnly || types.IsInterface(variable.Type())) {
			return i
		}
	}
	return -1
}

// doTarget returns the type a call of client-go's Client.Do decodes the
// answer into, and nil for any other call, a call that decodes into nil
// included.
func doTarget(info *types.Info, sel *ast.SelectorExpr, call *ast.CallExpr) types.Type {
	if sel == nil || sel.Sel.Name != doName || len(call.Args) != 2 || info.Types[call.Args[1]].IsNil() {
		return nil
	}
	selection := info.Selections[sel]
	if selection == nil || selection.Kind() != types.MethodVal {
		return nil
	}
	if recv, isClient := clientGoStruct(selection.Recv()); !isClient || recv.Obj().Name() != clientStructName {
		return nil
	}
	return info.TypeOf(call.Args[1])
}

// recordPassed records each function whose result a call of a client-go
// service method is handed, as one of its arguments.
func (r *rawReads) recordPassed(info *types.Info, sel *ast.SelectorExpr, method *types.Func, args []ast.Expr) {
	named, _ := shared.ClientGoServiceInterface(info.TypeOf(sel.X))
	key := shared.ServiceName(named) + "." + method.Name()
	for _, arg := range args {
		argCall, isCall := ast.Unparen(arg).(*ast.CallExpr)
		if !isCall {
			continue
		}
		site := r.siteOf(calledObject(info, argCall))
		if site == "" {
			continue
		}
		if r.passedTo[site] == nil {
			r.passedTo[site] = map[string]bool{}
		}
		r.passedTo[site][key] = true
	}
}

// siteOf spells the site a function is, the way the walk names it, and
// returns the empty string for what is no site: a method, a variable, a
// function of client-go.
func (r *rawReads) siteOf(obj types.Object) string {
	fn, isFunc := obj.(*types.Func)
	if !isFunc || fn.Pkg() == r.sdk.Types || fn.Signature().Recv() != nil {
		return ""
	}
	if inToolutil(fn) {
		return toolutilSitePrefix + fn.Name()
	}
	return shared.ShortPackage(fn.Pkg().Path()) + "." + fn.Name()
}

// readCallers records each call of a forwarder in pkgs, with what the raw
// request it builds then carries, and the type arguments a generic one is
// instantiated with.
func (r *rawReads) readCallers(pkgs []*packages.Package) {
	for _, pkg := range pkgs {
		short := shared.ShortPackage(pkg.PkgPath)
		for _, fn := range functionBodies(pkg) {
			site := short + "." + fn.name
			ast.Inspect(fn.body, func(node ast.Node) bool {
				if call, isCall := node.(*ast.CallExpr); isCall {
					r.readCaller(pkg.TypesInfo, site, call)
				}
				return true
			})
		}
	}
}

// readCaller records one call, made at site, when it calls a forwarder: for
// each raw request the function forwards its verb or options to, what that
// request then carries.
func (r *rawReads) readCaller(info *types.Info, site string, call *ast.CallExpr) {
	forwards := r.forwarders[calledObject(info, call)]
	if len(forwards) == 0 {
		return
	}
	for _, forward := range forwards {
		caller := rawCaller{Site: site, Verb: forward.verbValue, Options: forward.optionsValue}
		if forward.verb >= 0 {
			caller.Verb = constantString(info, call.Args[forward.verb])
		}
		if forward.options >= 0 {
			caller.Options = spellOptions(info, call.Args[forward.options])
			r.sends[forward.request] = append(r.sends[forward.request], info.TypeOf(call.Args[forward.options]))
		}
		r.callers[forward.request] = append(r.callers[forward.request], caller)
	}
	if instance, isGeneric := info.Instances[calledIdent(call)]; isGeneric {
		forwarderSite := forwards[0].request.site
		r.instances[forwarderSite] = append(r.instances[forwarderSite], slices.Collect(instance.TypeArgs.Types()))
	}
}

// attach fills in each raw request of found what was read around it: the
// calls of its function when it forwards it, and the supersets it decodes
// its answer into and sends. An answer decoded from a request the walk
// cannot tell is counted against every raw request of its site.
func (r *rawReads) attach(found []workaround) {
	placed := map[string]int{}
	for i := range found {
		if found[i].Form != formRawRequest {
			continue
		}
		site := found[i].Site
		request := rawRequest{site: site, place: placed[site]}
		placed[site]++
		found[i].Callers = sortedCallers(r.callers[request])
		found[i].Decodes = r.supersets(site, slices.Concat(r.decodes[request], r.siteDecodes[site]))
		found[i].Sends = r.supersets(site, r.sends[request])
	}
}

// sortedCallers sorts callers and drops a repeated one, returning nil for
// none.
func sortedCallers(callers []rawCaller) []rawCaller {
	slices.SortFunc(callers, func(a, b rawCaller) int {
		return cmp.Or(strings.Compare(a.Site, b.Site), strings.Compare(a.Verb, b.Verb), strings.Compare(a.Options, b.Options))
	})
	return slices.Compact(callers)
}

// supersets returns the supersets among the types site carries, each type
// parameter of its function read as every type argument the function is
// instantiated with, sorted by type and each once.
func (r *rawReads) supersets(site string, carried []types.Type) []waits.Superset {
	byType := map[string]waits.Superset{}
	for _, t := range carried {
		for _, resolved := range r.resolve(site, t) {
			if superset, isSuperset := waits.SupersetOf(r.sdk, resolved); isSuperset {
				byType[superset.Type] = superset
			}
		}
	}
	var out []waits.Superset
	for _, key := range slices.Sorted(maps.Keys(byType)) {
		out = append(out, byType[key])
	}
	return out
}

// resolve returns t, or, when t is a type parameter of site's function or a
// pointer to one, every type argument the function is instantiated with in
// its place.
func (r *rawReads) resolve(site string, t types.Type) []types.Type {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	param, isParam := t.(*types.TypeParam)
	if !isParam {
		return []types.Type{t}
	}
	var out []types.Type
	for _, args := range r.instances[site] {
		out = append(out, args[param.Index()])
	}
	return out
}

// calledIdent returns the identifier a call calls by: the function's name,
// the selected method's or function's name, its generic origin's for an
// instantiation, and nil for a call of anything else.
func calledIdent(call *ast.CallExpr) *ast.Ident {
	fun := ast.Unparen(call.Fun)
	if index, isIndex := fun.(*ast.IndexExpr); isIndex {
		fun = index.X
	}
	if list, isList := fun.(*ast.IndexListExpr); isList {
		fun = list.X
	}
	if sel, isSel := fun.(*ast.SelectorExpr); isSel {
		return sel.Sel
	}
	ident, _ := fun.(*ast.Ident)
	return ident
}
