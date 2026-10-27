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

// forwarder is a function that hands a raw request its own verb or options
// parameter: the site it is, the index of each parameter it forwards (-1
// for one it does not), and what the request carries in place of one it
// does not.
type forwarder struct {
	site                    string
	verb, options           int
	verbValue, optionsValue string
}

// rawReads is what the walk of the tool packages reads around the
// workarounds it finds, keyed by site: the types a raw request's answer is
// decoded into and the ones it sends as options, the functions that forward
// a raw request their verb or options and each call of them, the type
// arguments a generic one is instantiated with, the client-go methods each
// function's result is handed to, and the strings each function's body
// holds.
type rawReads struct {
	sdk        waits.SDK
	decodes    map[string][]types.Type
	sends      map[string][]types.Type
	forwarders map[types.Object]forwarder
	callers    map[string][]rawCaller
	instances  map[string][][]types.Type
	passedTo   map[string]map[string]bool
	strings    map[string][]string
}

// newRawReads returns an empty rawReads for clientGo.
func newRawReads(clientGo *types.Package) *rawReads {
	return &rawReads{
		sdk:        waits.SDK{Types: clientGo},
		decodes:    map[string][]types.Type{},
		sends:      map[string][]types.Type{},
		forwarders: map[types.Object]forwarder{},
		callers:    map[string][]rawCaller{},
		instances:  map[string][][]types.Type{},
		passedTo:   map[string]map[string]bool{},
		strings:    map[string][]string{},
	}
}

// read records what one function body holds: each string, each raw
// request's options and whether the function forwards its own to it, each
// answer a raw request decodes, and each call of a client-go service method
// with the functions whose results it is handed.
func (r *rawReads) read(scan bodyScan, fn funcBody, site string) {
	r.strings[site] = append(r.strings[site], syntaxStrings(fn.body)...)
	ast.Inspect(fn.body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall {
			r.readCall(scan, fn, site, call)
		}
		return true
	})
}

// readCall records one call of the function body at site.
func (r *rawReads) readCall(scan bodyScan, fn funcBody, site string, call *ast.CallExpr) {
	info := scan.info
	if constructor, verb, opt := scan.rawRequestArgs(call); constructor != nil {
		forward := forwarder{
			site: site, verb: paramIndex(info, fn.signature, verb, false), options: paramIndex(info, fn.signature, opt, true),
			verbValue: constantString(info, verb), optionsValue: spellOptions(info, opt),
		}
		if forward.verb >= 0 || forward.options >= 0 {
			r.forwarders[fn.object] = forward
		}
		// A forwarded parameter's own type is an interface, which is no
		// superset; what it carries is read off each caller.
		r.sends[site] = append(r.sends[site], info.TypeOf(opt))
		return
	}
	sel, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if decoded := doTarget(info, sel, call); decoded != nil {
		r.decodes[site] = append(r.decodes[site], decoded)
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

// readCaller records one call, made at site, when it calls a forwarder.
func (r *rawReads) readCaller(info *types.Info, site string, call *ast.CallExpr) {
	forward, isForwarder := r.forwarders[calledObject(info, call)]
	if !isForwarder {
		return
	}
	caller := rawCaller{Site: site, Verb: forward.verbValue, Options: forward.optionsValue}
	if forward.verb >= 0 {
		caller.Verb = constantString(info, call.Args[forward.verb])
	}
	if forward.options >= 0 {
		caller.Options = spellOptions(info, call.Args[forward.options])
		r.sends[forward.site] = append(r.sends[forward.site], info.TypeOf(call.Args[forward.options]))
	}
	r.callers[forward.site] = append(r.callers[forward.site], caller)
	if instance, isGeneric := info.Instances[calledIdent(call)]; isGeneric {
		r.instances[forward.site] = append(r.instances[forward.site], slices.Collect(instance.TypeArgs.Types()))
	}
}

// attach fills in each raw request of found what was read around it: the
// calls of its function when it forwards, and the supersets it decodes its
// answer into and sends.
func (r *rawReads) attach(found []workaround) {
	for i := range found {
		if found[i].Form != formRawRequest {
			continue
		}
		site := found[i].Site
		found[i].Callers = sortedCallers(r.callers[site])
		found[i].Decodes = r.supersets(site, r.decodes[site])
		found[i].Sends = r.supersets(site, r.sends[site])
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
