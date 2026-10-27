// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
)

// How an option struct a request carries is held to an input. The first three
// are the ways a handler fills one; a declared struct is one nothing fills on
// purpose, and an unpaired one is the finding.
const (
	// optionPaired: the field diff compares the struct with an input, paired
	// by the literal a handler builds, by the literal a helper builds out of
	// the fields a handler hands it, or, for a nested struct, by the struct
	// its values are read from.
	optionPaired = "paired"
	// optionConverted: the struct is a conversion of a held struct, which the
	// compiler keeps field for field.
	optionConverted = "converted"
	// optionCopied: the struct is a literal whose values are read field by
	// field off a held struct, and the copy is judged as a copy.
	optionCopied = "copied"
	// optionDeclared: nothing fills the struct, for the reason its
	// declaration gives.
	optionDeclared = "declared"
	// optionUnpaired: nothing fills the struct and nothing says why.
	optionUnpaired = "unpaired"
)

// Where an option struct sits in the request.
const (
	// levelReceived: a client-go service method a tool package references
	// takes it as a parameter.
	levelReceived = "received"
	// levelNested: a held received struct reaches it through a field.
	levelNested = "nested"
)

// optionStruct is one client-go struct a request a handler sends carries,
// with how it is held to an input.
type optionStruct struct {
	// Type is the client-go struct's name.
	Type string `json:"type"`
	// Level is received or nested.
	Level string `json:"level"`
	// Under lists, for a nested struct, the received structs that reach it.
	Under []string `json:"under,omitempty"`
	// Status is paired, converted, copied, declared or unpaired.
	Status string `json:"status"`
	// Inputs lists, for a paired struct, the inputs it is compared with as
	// "<package>.<input>", with how the pair was found in parentheses when it
	// was not by a handler's own literal.
	Inputs []string `json:"inputs,omitempty"`
	// From is, for a converted or copied struct, the struct it is made from.
	From string `json:"from,omitempty"`
	// Sites lists the functions that convert or copy it or, for an unpaired
	// received struct, the functions that send it.
	Sites []string `json:"sites,omitempty"`
	// NilOnly marks an unpaired struct every call sends as nil, so no field
	// of it can ever reach GitLab.
	NilOnly bool `json:"nil_only,omitempty"`
	// Category and Reason are a declared struct's.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Missing lists, for a nested struct, the fields an input it is paired
	// with carries no counterpart of and no declaration answers, as
	// "<package>.<input>.<tag>".
	Missing []string `json:"missing,omitempty"`
}

// optionCopy is one literal that fills a client-go option struct field by
// field from another.
type optionCopy struct {
	// From and To are the struct read and the struct built.
	From string `json:"from"`
	To   string `json:"to"`
	// Site is the function the literal is in.
	Site string `json:"site"`
	// Shared counts the fields both structs declare under one name with one
	// type, and Copied how many of them the literal fills.
	Shared int `json:"shared"`
	Copied int `json:"copied"`
	// Missing lists the shared fields the literal leaves out that no
	// declaration answers, and Dropped the ones a declaration does.
	Missing []string `json:"missing,omitempty"`
	Dropped []string `json:"dropped,omitempty"`
}

// optionConversion is one conversion of a client-go struct into another,
// which the compiler holds field for field and is never a finding. Every
// conversion into a request struct is listed, the ones whose struct no call
// the walk sees sends included: a raw request built in a helper that takes
// its options as any is one, and the struct reaches it by a route the walk
// does not follow.
type optionConversion struct {
	From string `json:"from"`
	To   string `json:"to"`
	Site string `json:"site"`
}

// optionStructsReport is the option_structs section of the report.
type optionStructsReport struct {
	Structs     []optionStruct     `json:"structs"`
	Copies      []optionCopy       `json:"copies"`
	Conversions []optionConversion `json:"conversions"`
}

// Declaration categories. All but the last answer a struct nothing fills,
// and the last a field a copy leaves out.
const (
	// categoryMapInput: the struct is decoded from an input that is a map,
	// whose keys a test of the handler holds to the struct's tags.
	categoryMapInput = "map-input"
	// categorySDKModelsUnused: the struct models parameters the route does
	// not declare, so every call sends it as nil.
	categorySDKModelsUnused = "sdk-models-unused"
	// categoryConstantLookup: the struct is built from constants for a lookup
	// of the server's own, never from what a caller asked for.
	categoryConstantLookup = "constant-lookup"
	// categoryScalarList: the input carries the struct's one field as a list
	// of values, and the handler builds one struct per value, so there is no
	// struct of the input's to pair it with.
	categoryScalarList = "scalar-list"
	// categoryDroppedOnPurpose: a copy leaves the field out because the
	// request it builds must not send it.
	categoryDroppedOnPurpose = "dropped-on-purpose"
)

// structCategories maps each category that answers a struct nothing fills to
// what the struct has to be for the declaration to hold, empty for a
// category that requires nothing the walk can check.
var structCategories = map[string]string{
	categoryMapInput:        "",
	categorySDKModelsUnused: "every call sends it as nil",
	categoryConstantLookup:  "",
	categoryScalarList:      "it carries exactly one field",
}

// declarationHolds reports whether what a category requires of a struct is
// true of entry, the struct named.
func declarationHolds(category string, entry optionStruct, named *types.Named) bool {
	if category == categorySDKModelsUnused {
		return entry.NilOnly
	}
	if category == categoryScalarList {
		st, _ := named.Underlying().(*types.Struct)
		return exportedFields(st) == 1
	}
	_, isStructCategory := structCategories[category]
	return isStructCategory
}

// exportedFields counts the exported fields of a struct, at its top level.
func exportedFields(st *types.Struct) int {
	count := 0
	for field := range st.Fields() {
		if field.Exported() {
			count++
		}
	}
	return count
}

// optionStructDeclaration answers an option struct nothing fills, keyed by
// the struct's name, or a field a copy leaves out, keyed by [copyKey].
type optionStructDeclaration struct {
	Category string
	Reason   string
}

// copyKeySeparator joins the two structs of a copy in its declaration key.
const copyKeySeparator = "->"

// copyKey is the declaration key of a field a copy from one struct into
// another leaves out.
func copyKey(from, to, field string) string { return from + copyKeySeparator + to + "." + field }

// optionUniverse is what the walk of the tool packages finds: the structs the
// service methods they reference take, and every conversion and copy that
// builds one client-go struct out of another.
type optionUniverse struct {
	received    map[*types.TypeName]*receivedUse
	conversions []optionBuild
	copies      []optionBuild
	// forwarders maps a function that hands a raw request one of its own
	// parameters as the options, by its object, to that parameter's index.
	forwarders map[types.Object]int
}

// funcBody is one body the walk reads: a function declaration's, or the
// function literal a package-level variable is initialized with
// (groupboards.newRawRequest, a seam its tests replace).
type funcBody struct {
	// name is the function's name as the section prints it, its receiver
	// type first for a method.
	name string
	// object is the function, or the variable that holds the literal.
	object    types.Object
	signature *types.Signature
	body      *ast.BlockStmt
}

// functionBodies lists the bodies of pkg, declarations without a body
// passed over.
func functionBodies(pkg *packages.Package) []funcBody {
	var out []funcBody
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Body != nil {
					object := pkg.TypesInfo.Defs[typed.Name]
					signature, _ := object.Type().(*types.Signature)
					out = append(out, funcBody{name: functionName(typed), object: object, signature: signature, body: typed.Body})
				}
			case *ast.GenDecl:
				out = append(out, variableBodies(pkg.TypesInfo, typed)...)
			}
		}
	}
	return out
}

// variableBodies lists the function literals a declaration of variables
// initializes them with.
func variableBodies(info *types.Info, decl *ast.GenDecl) []funcBody {
	var out []funcBody
	for _, spec := range decl.Specs {
		values, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for i, value := range values.Values {
			if lit, isLit := value.(*ast.FuncLit); isLit {
				name := values.Names[i]
				signature, _ := info.TypeOf(lit).(*types.Signature)
				out = append(out, funcBody{name: name.Name, object: info.Defs[name], signature: signature, body: lit.Body})
			}
		}
	}
	return out
}

// rawForwarders finds the functions of pkgs that build a raw request with one
// of their own parameters of type any as its options: doProjectRequest[T]
// and rawMutateApprovalRule take the options their callers built and hand
// them on, so the struct a request carries is the static type of what each
// caller passes there.
func rawForwarders(pkgs []*packages.Package) map[types.Object]int {
	forwarders := map[types.Object]int{}
	for _, pkg := range pkgs {
		info := pkg.TypesInfo
		for _, fn := range functionBodies(pkg) {
			params := map[*types.Var]int{}
			signature := fn.signature.Params()
			for i := range signature.Len() {
				if param := signature.At(i); types.IsInterface(param.Type()) {
					params[param] = i
				}
			}
			ast.Inspect(fn.body, func(node ast.Node) bool {
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				sel, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
				opt, _ := rawRequestOption(info, sel, call).(*ast.Ident)
				if param, isParam := params[identVar(info, opt)]; isParam {
					forwarders[fn.object] = param
				}
				return true
			})
		}
	}
	return forwarders
}

// identVar returns the variable an identifier uses, nil for anything else, a
// nil identifier included.
func identVar(info *types.Info, ident *ast.Ident) *types.Var {
	used, _ := info.Uses[ident].(*types.Var)
	return used
}

// calledObject returns what a call calls by name: a function, its generic
// origin for an instantiation, or a variable holding one. Nil for a call of
// anything else.
func calledObject(info *types.Info, call *ast.CallExpr) types.Object {
	fun := ast.Unparen(call.Fun)
	switch typed := fun.(type) {
	case *ast.IndexExpr:
		fun = typed.X
	case *ast.IndexListExpr:
		fun = typed.X
	}
	var ident *ast.Ident
	switch typed := fun.(type) {
	case *ast.Ident:
		ident = typed
	case *ast.SelectorExpr:
		ident = typed.Sel
	}
	obj := info.Uses[ident]
	if fn, isFunc := obj.(*types.Func); isFunc {
		return fn.Origin()
	}
	return obj
}

// receivedUse is one struct a referenced service method takes: its type and,
// for each function that references such a method, whether the function
// sends anything other than nil for it.
type receivedUse struct {
	named *types.Named
	sites map[string]bool
}

// optionBuild is a conversion or a copy literal: the struct built, the struct
// it is built from, the function it is in and, for a copy, the fields the
// literal fills from the other struct.
type optionBuild struct {
	to, from *types.Named
	site     string
	keys     map[string]bool
}

// status is what an option build makes of the struct it builds.
func (b optionBuild) status() string {
	if b.keys == nil {
		return optionConverted
	}
	return optionCopied
}

// collectOptionUniverse walks every function of pkgs, and every function
// literal a package-level variable holds.
func collectOptionUniverse(pkgs []*packages.Package) optionUniverse {
	universe := optionUniverse{received: map[*types.TypeName]*receivedUse{}, forwarders: rawForwarders(pkgs)}
	for _, pkg := range pkgs {
		pkgName := shared.ShortPackage(pkg.PkgPath)
		for _, fn := range functionBodies(pkg) {
			universe.walkFunction(pkg.TypesInfo, fn.body, pkgName+"."+fn.name)
		}
	}
	return universe
}

// walkFunction records what one function body sends, converts and copies.
// A service method called in place is recorded with the arguments of the
// call; one handed on as a value is recorded as sending whatever its caller
// passes, which the walk does not follow. A raw request built through one of
// client-go's own constructors sends the struct its options argument is,
// when that argument's static type is one or embeds one, and a call of a
// function that hands a raw request its parameter of type any sends what the
// call passes there, one call up.
func (u *optionUniverse) walkFunction(info *types.Info, body *ast.BlockStmt, site string) {
	called := map[*ast.SelectorExpr]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			sel, _ := ast.Unparen(typed.Fun).(*ast.SelectorExpr)
			if method := serviceMethod(info, sel); method != nil {
				called[sel] = true
				u.recordReference(info, method, typed.Args, site)
				return true
			}
			// The options parameter is any, so an argument whose static
			// type is a struct is a value of it: a nil handed there has
			// the type of nil and is no struct.
			if opt := rawRequestOption(info, sel, typed); opt != nil {
				u.receiveOptions(info.TypeOf(opt), site)
				return true
			}
			if param, isForwarder := u.forwarders[calledObject(info, typed)]; isForwarder {
				u.receiveOptions(info.TypeOf(typed.Args[param]), site)
				return true
			}
			u.recordConversion(info, typed, site)
		case *ast.SelectorExpr:
			if method := serviceMethod(info, typed); method != nil && !called[typed] {
				u.recordReference(info, method, nil, site)
			}
		case *ast.CompositeLit:
			u.recordCopies(info, typed, site)
		}
		return true
	})
}

// serviceMethod returns the client-go service method a selector names, and
// nil for anything else, a nil selector included. The GraphQL interface is
// not a service: the struct its Do takes carries a document, which the
// GraphQL rules judge.
func serviceMethod(info *types.Info, sel *ast.SelectorExpr) *types.Func {
	if sel == nil {
		return nil
	}
	named, isInterface := shared.ClientGoServiceInterface(info.TypeOf(sel.X))
	if !isInterface || shared.ServiceName(named) == named.Obj().Name() {
		return nil
	}
	selection, isSelection := info.Selections[sel]
	if !isSelection || selection.Kind() != types.MethodVal {
		return nil
	}
	method, _ := selection.Obj().(*types.Func)
	return method
}

// rawRequestParam is the name client-go's request constructors give the
// parameter a request's options are passed in: NewRequest, NewRequestToURL
// and UploadRequest all take them as `opt any`.
const rawRequestParam = "opt"

// rawRequestOption returns the options argument of a call to a method of
// client-go's Client that takes one, which is how a raw request is built, and
// nil for any other call, a call whose function is not a selector included.
func rawRequestOption(info *types.Info, sel *ast.SelectorExpr, call *ast.CallExpr) ast.Expr {
	if sel == nil {
		return nil
	}
	selection, isSelection := info.Selections[sel]
	if !isSelection || selection.Kind() != types.MethodVal {
		return nil
	}
	recv, isClient := clientGoStruct(selection.Recv())
	if !isClient || recv.Obj().Name() != "Client" {
		return nil
	}
	method, _ := selection.Obj().(*types.Func)
	params := method.Signature().Params()
	for i := range params.Len() {
		if params.At(i).Name() == rawRequestParam {
			return call.Args[i]
		}
	}
	return nil
}

// recordReference records every struct method takes, sent from site. args are
// the call's arguments, none for a method value, whose arguments are its
// caller's. The variadic tail of request options is a slice, which is no
// struct, so it is never recorded.
func (u *optionUniverse) recordReference(info *types.Info, method *types.Func, args []ast.Expr, site string) {
	params := method.Signature().Params()
	for i := range params.Len() {
		if named, isStruct := clientGoStruct(params.At(i).Type()); isStruct {
			u.receive(named, site, len(args) <= i || !info.Types[args[i]].IsNil())
		}
	}
}

// receiveOptions records the client-go structs a raw request's options of
// static type t carry, sent from site: the struct t is or points to, or the
// client-go structs a struct of the tool package's own embeds beside the
// fields client-go does not model (groupboards.updateGroupBoardBody), which
// the request carries flattened.
func (u *optionUniverse) receiveOptions(t types.Type, site string) {
	if named, isStruct := clientGoStruct(t); isStruct {
		u.receive(named, site, true)
		return
	}
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		t = ptr.Elem()
	}
	local, isStruct := t.Underlying().(*types.Struct)
	if !isStruct {
		return
	}
	for field := range local.Fields() {
		if named, isClientGo := clientGoStruct(field.Type()); isClientGo && field.Embedded() {
			u.receive(named, site, true)
		}
	}
}

// receive records that site sends named, with something other than nil when
// sends is true.
func (u *optionUniverse) receive(named *types.Named, site string, sends bool) {
	use, seen := u.received[named.Obj()]
	if !seen {
		use = &receivedUse{named: named, sites: map[string]bool{}}
		u.received[named.Obj()] = use
	}
	use.sites[site] = use.sites[site] || sends
}

// recordConversion records a conversion of one client-go struct into
// another, through a pointer or by value.
func (u *optionUniverse) recordConversion(info *types.Info, call *ast.CallExpr, site string) {
	if len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
		return
	}
	to, isTo := clientGoStruct(info.TypeOf(call))
	from, isFrom := clientGoStruct(info.TypeOf(call.Args[0]))
	if isTo && isFrom && to.Obj() != from.Obj() {
		u.conversions = append(u.conversions, optionBuild{to: to, from: from, site: site})
	}
}

// recordCopies records a literal of a client-go struct that fills a field
// with the field of the same name of another client-go struct, once for each
// struct it reads from, with the fields it fills that way.
func (u *optionUniverse) recordCopies(info *types.Info, lit *ast.CompositeLit, site string) {
	to, isStruct := clientGoStruct(info.TypeOf(lit))
	if !isStruct {
		return
	}
	read := map[*types.TypeName]*optionBuild{}
	var order []*types.TypeName
	for _, elt := range lit.Elts {
		from, field, isCopy := copiedField(info, elt)
		if !isCopy || from.Obj() == to.Obj() {
			continue
		}
		build, seen := read[from.Obj()]
		if !seen {
			build = &optionBuild{to: to, from: from, site: site, keys: map[string]bool{}}
			read[from.Obj()] = build
			order = append(order, from.Obj())
		}
		build.keys[field] = true
	}
	for _, obj := range order {
		u.copies = append(u.copies, *read[obj])
	}
}

// copiedField reports whether a literal element is F: x.F with x a client-go
// struct, and returns that struct and F.
func copiedField(info *types.Info, elt ast.Expr) (*types.Named, string, bool) {
	kv, isKV := elt.(*ast.KeyValueExpr)
	if !isKV {
		return nil, "", false
	}
	key, isIdent := kv.Key.(*ast.Ident)
	sel, isSelector := ast.Unparen(kv.Value).(*ast.SelectorExpr)
	if !isIdent || !isSelector || sel.Sel.Name != key.Name {
		return nil, "", false
	}
	if selection, isSelection := info.Selections[sel]; !isSelection || selection.Kind() != types.FieldVal {
		return nil, "", false
	}
	from, isStruct := clientGoStruct(info.TypeOf(sel.X))
	return from, key.Name, isStruct
}

// clientGoStruct returns the named client-go struct t is or points to.
func clientGoStruct(t types.Type) (*types.Named, bool) {
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		t = ptr.Elem()
	}
	named, isNamed := t.(*types.Named)
	if !isNamed || !shared.IsClientGoObject(named.Obj()) {
		return nil, false
	}
	_, isStruct := named.Underlying().(*types.Struct)
	return named, isStruct
}

// sharedFields returns the exported fields two structs both declare at their
// top level, under one name and with identical types, sorted.
func sharedFields(a, b *types.Named) []string {
	aStruct, _ := a.Underlying().(*types.Struct)
	bStruct, _ := b.Underlying().(*types.Struct)
	declared := map[string]*types.Var{}
	for field := range bStruct.Fields() {
		declared[field.Name()] = field
	}
	var out []string
	for field := range aStruct.Fields() {
		other := declared[field.Name()]
		if other == nil || !field.Exported() {
			continue
		}
		if types.Identical(field.Type(), other.Type()) {
			out = append(out, field.Name())
		}
	}
	sort.Strings(out)
	return out
}

// held is how a struct is held: its status, the struct it is made from and
// the functions that make it, for a conversion or a copy.
type held struct {
	status string
	from   string
	sites  []string
}

// buildOptionStructs holds every struct of the universe to an input, judges
// every copy into one, and returns the section and the declarations that no
// longer describe the tree.
func buildOptionStructs(universe optionUniverse, pairs []structs.InputPairDiff, declared map[string]optionStructDeclaration) (report optionStructsReport, stale []string) {
	inputs, missing, paired := indexPairs(pairs)
	resolved := resolveHeld(universe, paired)
	levels, under := optionLevels(universe, resolved)

	answered := map[string]bool{}
	report = optionStructsReport{
		Structs:     make([]optionStruct, 0, len(levels)),
		Copies:      make([]optionCopy, 0),
		Conversions: make([]optionConversion, 0, len(universe.conversions)),
	}
	for obj, level := range levels {
		state := resolved[obj]
		entry := optionStruct{
			Type: obj.Name(), Level: level, Under: under[obj], Inputs: inputs[obj],
			Status: state.status, From: state.from, Sites: state.sites, Missing: missing[obj],
		}
		if level == levelReceived {
			entry.Missing = nil
		}
		if entry.Status == "" {
			named, _ := obj.Type().(*types.Named)
			entry = declareOrFind(entry, named, universe.received[obj], declared, answered)
		}
		report.Structs = append(report.Structs, entry)
	}
	slices.SortFunc(report.Structs, func(a, b optionStruct) int { return strings.Compare(a.Type, b.Type) })

	for _, build := range universe.conversions {
		if _, sent := levels[build.to.Obj()]; !sent && !structs.IsRequestStruct(build.to) {
			continue
		}
		conversion := optionConversion{From: build.from.Obj().Name(), To: build.to.Obj().Name(), Site: build.site}
		if !slices.Contains(report.Conversions, conversion) {
			report.Conversions = append(report.Conversions, conversion)
		}
	}
	slices.SortFunc(report.Conversions, func(a, b optionConversion) int {
		return cmp.Or(strings.Compare(a.From, b.From), strings.Compare(a.To, b.To), strings.Compare(a.Site, b.Site))
	})

	for _, build := range universe.copies {
		if _, sent := levels[build.to.Obj()]; !sent {
			continue
		}
		if judged, isCopy := judgeCopy(build, declared, answered); isCopy {
			report.Copies = append(report.Copies, judged)
		}
	}
	slices.SortFunc(report.Copies, func(a, b optionCopy) int {
		return cmp.Or(strings.Compare(a.From, b.From), strings.Compare(a.To, b.To), strings.Compare(a.Site, b.Site))
	})
	return report, staleOptionDeclarations(report, declared, answered)
}

// indexPairs keys the field diff's input pairs by client-go struct: the
// inputs each is compared with, the fields those inputs lack, and whether
// one of those inputs is a struct a model sends. An unexported struct is an
// intermediate a handler fills from its input (runners.runnerListRequest):
// it is listed among the inputs, but the struct compared with it alone is
// compared with nothing a model sends, so it holds nothing.
func indexPairs(pairs []structs.InputPairDiff) (inputs, missing map[*types.TypeName][]string, paired map[*types.TypeName]bool) {
	inputs = map[*types.TypeName][]string{}
	missing = map[*types.TypeName][]string{}
	paired = map[*types.TypeName]bool{}
	for _, pair := range pairs {
		obj := pair.SDKNamed.Obj()
		name := pair.Package + "." + pair.MCPType
		label := name
		if pair.Via != "" {
			label += " (" + pair.Via + ")"
		}
		inputs[obj] = append(inputs[obj], label)
		if token.IsExported(pair.MCPType) {
			paired[obj] = true
		}
		for _, tag := range pair.Missing {
			missing[obj] = append(missing[obj], name+"."+tag)
		}
	}
	for _, list := range inputs {
		sort.Strings(list)
	}
	for _, list := range missing {
		sort.Strings(list)
	}
	return inputs, missing, paired
}

// resolveHeld resolves which structs are held, and how: a struct paired with
// an input a model sends, then what a conversion or a copy builds out of a
// held struct, pass after pass until one resolves nothing new. Conversions
// come first in every pass, since the compiler holds a conversion and only
// the copy rule holds a copy.
func resolveHeld(universe optionUniverse, paired map[*types.TypeName]bool) map[*types.TypeName]held {
	resolved := map[*types.TypeName]held{}
	for obj := range paired {
		resolved[obj] = held{status: optionPaired}
	}
	builds := slices.Concat(universe.conversions, universe.copies)
	for changed := true; changed; {
		changed = false
		for _, build := range builds {
			to, from := build.to.Obj(), build.from.Obj()
			if resolved[from].status == "" {
				continue
			}
			target, isHeld := resolved[to]
			if !isHeld {
				resolved[to] = held{status: build.status(), from: from.Name(), sites: []string{build.site}}
				changed = true
				continue
			}
			if target.from == from.Name() && target.status == build.status() && !slices.Contains(target.sites, build.site) {
				target.sites = append(target.sites, build.site)
				sort.Strings(target.sites)
				resolved[to] = target
			}
		}
	}
	return resolved
}

// optionLevels places every struct the section judges: each received struct,
// and each struct a held received struct reaches through a field that no
// method receives itself, with the received structs that reach it.
func optionLevels(universe optionUniverse, resolved map[*types.TypeName]held) (levels map[*types.TypeName]string, under map[*types.TypeName][]string) {
	levels = map[*types.TypeName]string{}
	for obj := range universe.received {
		levels[obj] = levelReceived
	}
	under = map[*types.TypeName][]string{}
	for obj, use := range universe.received {
		if resolved[obj].status == "" {
			continue
		}
		for _, nested := range structs.NestedStructs(use.named) {
			if levels[nested.Obj()] == levelReceived {
				continue
			}
			levels[nested.Obj()] = levelNested
			under[nested.Obj()] = append(under[nested.Obj()], obj.Name())
		}
	}
	for _, list := range under {
		sort.Strings(list)
	}
	return levels, under
}

// declareOrFind settles a struct nothing holds: declared when the table
// answers it with a category a struct takes and the struct is what the
// category requires, unpaired otherwise. A received struct carries the
// functions that send it either way.
func declareOrFind(entry optionStruct, named *types.Named, use *receivedUse, declared map[string]optionStructDeclaration, answered map[string]bool) optionStruct {
	entry.Status = optionUnpaired
	if use != nil {
		entry.Sites = slices.Sorted(maps.Keys(use.sites))
		entry.NilOnly = true
		for _, site := range entry.Sites {
			entry.NilOnly = entry.NilOnly && !use.sites[site]
		}
	}
	if declaration, isDeclared := declared[entry.Type]; isDeclared && declarationHolds(declaration.Category, entry, named) {
		answered[entry.Type] = true
		entry.Status, entry.Category, entry.Reason = optionDeclared, declaration.Category, declaration.Reason
	}
	return entry
}

// judgeCopy measures one copy against the fields its two structs share. A
// literal that shares no field with the struct it reads from is not a copy.
func judgeCopy(build optionBuild, declared map[string]optionStructDeclaration, answered map[string]bool) (optionCopy, bool) {
	fields := sharedFields(build.from, build.to)
	if len(fields) == 0 {
		return optionCopy{}, false
	}
	judged := optionCopy{From: build.from.Obj().Name(), To: build.to.Obj().Name(), Site: build.site, Shared: len(fields)}
	for _, field := range fields {
		if build.keys[field] {
			judged.Copied++
			continue
		}
		key := copyKey(judged.From, judged.To, field)
		if declaration, isDeclared := declared[key]; isDeclared && declaration.Category == categoryDroppedOnPurpose {
			answered[key] = true
			judged.Dropped = append(judged.Dropped, field)
			continue
		}
		judged.Missing = append(judged.Missing, field)
	}
	return judged, true
}

// staleOptionDeclarations lists the declarations the run asked nothing of: a
// struct now held or no longer sent, a field a copy now fills or that no
// copy shares, and a category on a key of the other kind.
func staleOptionDeclarations(report optionStructsReport, declared map[string]optionStructDeclaration, answered map[string]bool) []string {
	statuses := map[string]string{}
	for _, entry := range report.Structs {
		statuses[entry.Type] = entry.Status
	}
	stale := make([]string, 0)
	for key, declaration := range declared {
		if answered[key] {
			continue
		}
		if strings.Contains(key, copyKeySeparator) {
			stale = append(stale, staleCopyDeclaration(key, declaration))
			continue
		}
		requirement, isStructCategory := structCategories[declaration.Category]
		if !isStructCategory {
			stale = append(stale, "option struct "+key+" (category "+declaration.Category+" does not answer a struct)")
			continue
		}
		status, isSent := statuses[key]
		if !isSent {
			stale = append(stale, "option struct "+key+" (no call sends it)")
			continue
		}
		if status == optionUnpaired {
			stale = append(stale, "option struct "+key+" (category "+declaration.Category+" requires that "+requirement+")")
			continue
		}
		stale = append(stale, "option struct "+key+" (now "+status+")")
	}
	sort.Strings(stale)
	return stale
}

// staleCopyDeclaration words an unanswered declaration of a field a copy
// leaves out.
func staleCopyDeclaration(key string, declaration optionStructDeclaration) string {
	if declaration.Category != categoryDroppedOnPurpose {
		return "option struct copy " + key + " (category " + declaration.Category + " does not answer a field a copy leaves out)"
	}
	return "option struct copy " + key + " (no copy leaves the field out)"
}

// summarizeOptionStructs counts the section into the summary.
func summarizeOptionStructs(summary *reportSummary, report optionStructsReport) {
	summary.OptionStructs = len(report.Structs)
	for _, entry := range report.Structs {
		summary.OptionNestedMissing += len(entry.Missing)
		if entry.Status == optionPaired {
			summary.OptionPaired++
		}
		if entry.Status == optionConverted {
			summary.OptionConverted++
		}
		if entry.Status == optionCopied {
			summary.OptionCopied++
		}
		if entry.Status == optionDeclared {
			summary.OptionDeclared++
		}
		if entry.Status == optionUnpaired {
			summary.OptionUnpaired++
		}
	}
	for _, judged := range report.Copies {
		summary.OptionCopyMissing += len(judged.Missing)
	}
}

// keepOptionFindings keeps the structs and copies that carry a finding. A
// conversion never does, since the compiler holds it.
func keepOptionFindings(report optionStructsReport) optionStructsReport {
	out := optionStructsReport{Structs: make([]optionStruct, 0), Copies: make([]optionCopy, 0), Conversions: make([]optionConversion, 0)}
	for _, entry := range report.Structs {
		if entry.Status == optionUnpaired || len(entry.Missing) > 0 {
			out.Structs = append(out.Structs, entry)
		}
	}
	for _, judged := range report.Copies {
		if len(judged.Missing) > 0 {
			out.Copies = append(out.Copies, judged)
		}
	}
	return out
}
