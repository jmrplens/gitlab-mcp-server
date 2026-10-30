// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/ast"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// helperDataSource is a tool package of helpers alone, the shape of
// labeldata and iterationdata: functions that build an options struct out of
// scalars, one that hands its parameter on to another, and a package-level
// variable holding one, which is a value rather than a function.
const helperDataSource = `package widgetdata

import gl "` + fixtureSDKPath + `"

// ListOptions builds the options out of scalars.
func ListOptions(page int, search string) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{Search: &search}
	opts.Page = int64(page)
	return opts
}

// Chain hands its parameter on, two helpers deep.
func Chain(search string) *gl.ListWidgetsOptions { return Deeper(search) }

// Deeper hands its parameter on again.
func Deeper(search string) *gl.ListWidgetsOptions { return ListOptions(1, search) }

// Builder holds a helper as a value.
var Builder = ListOptions
`

// helperCallerSource is the tool package whose handlers call those helpers,
// in every shape the pairing has to tell apart.
const helperCallerSource = `package widgets

import (
	gl "` + fixtureSDKPath + `"
	"example.com/x/internal/tools/widgetdata"
)

type ListInput struct {
	Page   int    ~json:"page"~
	Search string ~json:"search"~
}

// List hands the helper of another package its input's fields.
func List(c *gl.Client, in ListInput) error {
	_, err := c.Widgets.ListWidgets(widgetdata.ListOptions(in.Page, in.Search))
	return err
}

type ChainInput struct {
	Search string ~json:"search"~
}

// ListChain reaches the literal through two helpers.
func ListChain(c *gl.Client, in ChainInput) error {
	_, err := c.Widgets.ListWidgets(widgetdata.Chain(in.Search))
	return err
}

type ConstInput struct {
	Search string ~json:"search"~
}

// ListConst hands the helper constants, so nothing of its input.
func ListConst(c *gl.Client, in ConstInput) error {
	_, err := c.Widgets.ListWidgets(widgetdata.ListOptions(1, "x"))
	return err
}

type LookupInput struct {
	Search string ~json:"search"~
}

// Lookup builds the options and sends nothing that takes them, beside a copy
// of its input, which makes the input no intermediate of itself.
func Lookup(c *gl.Client, in LookupInput) error {
	again := LookupInput{Search: in.Search}
	_ = widgetdata.ListOptions(1, in.Search)
	_ = again
	return c.Widgets.Run(gl.Query{})
}

type OwnInput struct {
	Search string ~json:"search"~
}

// Own fills the options from its input and hands a field of them to a
// helper: a client-go struct is the request, never a carrier of the input.
func Own(c *gl.Client, in OwnInput) error {
	opts := &gl.ListWidgetsOptions{Search: &in.Search}
	_ = widgetdata.Chain(*opts.Search)
	return c.Widgets.Run(gl.Query{})
}

// args is a struct of arguments several handlers fill for one body.
type args struct {
	search string
}

// options builds the options off the receiver.
func (a args) options() *gl.ListWidgetsOptions { return widgetdata.ListOptions(0, a.search) }

type lister func(opt *gl.ListWidgetsOptions, options ...gl.RequestOptionFunc) ([]*gl.Widget, error)

// run is the shared body, which takes the struct of arguments.
func run(list lister, a args) error {
	_, err := list(a.options())
	return err
}

type RunInput struct {
	Search string ~json:"search"~
}

// Run fills the struct of arguments and hands on the method as a value.
func Run(c *gl.Client, in RunInput) error {
	return run(c.Widgets.ListWidgets, args{search: in.Search})
}

type ValueInput struct {
	Page   int    ~json:"page"~
	Search string ~json:"search"~
}

type holder struct {
	build func(int, string) *gl.ListWidgetsOptions
}

// Values calls the helper through a local, a field, a package variable, a
// method expression and a literal, none of which the pairing follows, and
// converts a field, which calls nothing.
func Values(c *gl.Client, in ValueInput) error {
	local := widgetdata.ListOptions
	h := holder{build: widgetdata.ListOptions}
	_ = local(in.Page, in.Search)
	_ = h.build(in.Page, in.Search)
	_ = widgetdata.Builder(in.Page, in.Search)
	_ = args.options(args{search: in.Search})
	_ = func() int { return in.Page }()
	_ = int64(in.Page)
	_, err := c.Widgets.ListWidgets(nil)
	return err
}

type unlisted struct {
	Search string ~json:"search"~
}

// listUnlisted takes an input no model sends.
func listUnlisted(c *gl.Client, in unlisted) error {
	_, err := c.Widgets.ListWidgets(widgetdata.ListOptions(1, in.Search))
	return err
}

type UnnamedInput struct {
	Search string ~json:"search"~
}

// shared is a client no handler takes as a parameter.
var shared *gl.Client

// fixed is a constant a handler hands a helper.
const fixed = "x"

// Unnamed binds its input to no name and hands the helper a constant
// through a package-level client, which reads nothing of an input.
func Unnamed(*gl.Client, UnnamedInput) error {
	_, err := shared.Widgets.ListWidgets(widgetdata.ListOptions(1, fixed))
	return err
}

// filter is an intermediate: a struct of arguments a handler fills from its
// input and hands on whole.
type filter struct {
	search string
}

// optionsOf builds the options out of the intermediate.
func optionsOf(f filter) *gl.ListWidgetsOptions { return widgetdata.ListOptions(0, f.search) }

// scoped hands the intermediate to the function its caller passes.
func scoped(f filter, list func(filter) error) error { return list(f) }

type ScopedInput struct {
	Search string ~json:"search"~
}

// Scoped fills the intermediate from its input and builds the options in a
// closure whose parameter is the intermediate.
func Scoped(c *gl.Client, in ScopedInput) error {
	return scoped(filter{search: in.Search}, func(f filter) error {
		_, err := c.Widgets.ListWidgets(optionsOf(f))
		return err
	})
}

// read carries an intermediate, the helper that builds the options from it,
// and the method that sends them.
type read struct {
	f     filter
	build func(filter) *gl.ListWidgetsOptions
	list  func(*gl.ListWidgetsOptions, ...gl.RequestOptionFunc) ([]*gl.Widget, error)
}

// runRead is the shared body every read hands its struct to.
func runRead(r read) error {
	_, err := r.list(r.build(r.f))
	return err
}

type BuildInput struct {
	Search string ~json:"search"~
}

// Build names the helper as a value beside the intermediate it builds from,
// and hands on the method as a value too.
func Build(c *gl.Client, in BuildInput) error {
	return runRead(read{f: filter{search: in.Search}, build: optionsOf, list: c.Widgets.ListWidgets})
}

// token is an intermediate a helper both builds the options from and sends.
type token struct {
	search string
}

// sendToken builds the options out of the intermediate and sends them.
func sendToken(c *gl.Client, t token) error {
	_, err := c.Widgets.ListWidgets(widgetdata.ListOptions(0, t.search))
	return err
}

// withToken hands the intermediate to the function its caller passes.
func withToken(t token, create func(token) error) error { return create(t) }

type TokenInput struct {
	Search string ~json:"search"~
}

// Token hands the intermediate, from a closure, to a helper that sends the
// options itself, so the handler names no method that takes them.
func Token(c *gl.Client, in TokenInput) error {
	return withToken(token{search: in.Search}, func(t token) error { return sendToken(c, t) })
}

type TokenLitInput struct {
	Search string ~json:"search"~
}

// TokenLit hands that helper a literal of the intermediate filled in place.
func TokenLit(c *gl.Client, in TokenLitInput) error { return sendToken(c, token{search: in.Search}) }

type TokenConstInput struct {
	Search string ~json:"search"~
}

// TokenConst fills the intermediate with a constant, so the struct its
// closure hands on is no intermediate of its input.
func TokenConst(c *gl.Client, in TokenConstInput) error {
	if in.Search == "" {
		return nil
	}
	return withToken(token{search: "x"}, func(t token) error { return sendToken(c, t) })
}

// protectOf builds the protection out of the intermediate.
func protectOf(f filter) *gl.ProtectOptions { return &gl.ProtectOptions{Name: &f.search} }

// protectRead carries an intermediate and the helper that builds the
// protection from it.
type protectRead struct {
	f     filter
	build func(filter) *gl.ProtectOptions
}

type MixedInput struct {
	Search string ~json:"search"~
}

// Mixed fills the intermediate from its input for the list, and with
// constants for the protection: in a literal, in a local, beside a closure
// whose parameter is the intermediate and beside the helper named as a
// value. Only the list is built for its input.
func Mixed(c *gl.Client, in MixedInput) error {
	if _, err := c.Widgets.ListWidgets(optionsOf(filter{search: in.Search})); err != nil {
		return err
	}
	fixed := filter{search: "x"}
	_ = c.Widgets.Protect(1, protectOf(fixed))
	_ = c.Widgets.Protect(1, protectOf(filter{search: "y"}))
	_ = protectRead{f: filter{search: "v"}, build: protectOf}
	return scoped(filter{search: "z"}, func(f filter) error { return c.Widgets.Protect(1, protectOf(f)) })
}

// split hands back the intermediate it is handed and another it fills with
// constants.
func split(f filter) (filter, filter) { return f, filter{search: "x"} }

type SplitInput struct {
	Search string ~json:"search"~
}

// Split takes two intermediates from one call: only the first holds its
// input, so the protection built from the second is built for none of it.
func Split(c *gl.Client, in SplitInput) error {
	held, fixed := split(filter{search: in.Search})
	if _, err := c.Widgets.ListWidgets(optionsOf(held)); err != nil {
		return err
	}
	return c.Widgets.Protect(1, protectOf(fixed))
}

// flip hands its two intermediates back in each other's place.
func flip(x, y filter) (filter, filter) { return y, x }

// swap hands its two intermediates back in each other's place, through
// another function's results and through named results and a bare return.
func swap(a, b filter) (first, second filter) {
	if a.search == "" {
		return flip(a, b)
	}
	first, second = b, a
	return
}

// pick hands back what the function it is handed does, which it cannot
// read, or what it does itself when handed none, or two intermediates of
// constants.
func pick(f filter, from func(filter) (filter, filter)) (filter, filter) {
	if f.search == "" {
		return split(filter{search: "y"})
	}
	if from == nil {
		return pick(f, split)
	}
	return from(f)
}

type PickInput struct {
	Search string ~json:"search"~
}

// Pick takes two intermediates whose places nothing it reads tells apart,
// so each holds its input.
func Pick(c *gl.Client, in PickInput) error {
	_, fixed := pick(filter{search: in.Search}, nil)
	return c.Widgets.Protect(1, protectOf(fixed))
}

type SwapInput struct {
	Search string ~json:"search"~
}

// Swap hands its input in the first place and takes it back in the second.
func Swap(c *gl.Client, in SwapInput) error {
	fixed, held := swap(filter{search: in.Search}, filter{search: "x"})
	if err := c.Widgets.Protect(1, protectOf(fixed)); err != nil {
		return err
	}
	_, err := c.Widgets.ListWidgets(optionsOf(held))
	return err
}

// constant calls the function its caller passes with an intermediate of its
// own, filled with constants, whatever it is handed.
func constant(f filter, protect func(filter) error) error {
	if f.search == "" {
		return nil
	}
	return protect(filter{search: "w"})
}

type ConstantInput struct {
	Search string ~json:"search"~
}

// Constant fills the intermediate from its input beside a closure the
// function it is handed to calls with constants alone, so the protection is
// built for none of its input.
func Constant(c *gl.Client, in ConstantInput) error {
	return constant(filter{search: in.Search}, func(f filter) error { return c.Widgets.Protect(1, protectOf(f)) })
}

// relay hands the function its caller passes on to another, which calls it.
func relay(f filter, list func(filter) error) error { return scoped(f, list) }

type RelayInput struct {
	Search string ~json:"search"~
}

// Relay hands its closure to a function that hands it on, so whoever calls
// it is out of sight and the closure holds the intermediate beside it.
func Relay(c *gl.Client, in RelayInput) error {
	return relay(filter{search: in.Search}, func(f filter) error {
		_, err := c.Widgets.ListWidgets(optionsOf(f))
		return err
	})
}

type IndirectInput struct {
	Search string ~json:"search"~
}

// Indirect hands its closure to a function held in a variable, whose body
// is out of sight, so the closure holds the intermediate beside it.
func Indirect(c *gl.Client, in IndirectInput) error {
	call := scoped
	return call(filter{search: in.Search}, func(f filter) error {
		_, err := c.Widgets.ListWidgets(optionsOf(f))
		return err
	})
}

// ignore names neither of its parameters, so it calls neither.
func ignore(filter, func(filter) error) error { return nil }

type IgnoredInput struct {
	Search string ~json:"search"~
}

// Ignored hands its closure to a function that never calls it, so the
// protection it builds is built for none of its input.
func Ignored(c *gl.Client, in IgnoredInput) error {
	return ignore(filter{search: in.Search}, func(f filter) error { return c.Widgets.Protect(1, protectOf(f)) })
}

type HeldInput struct {
	Search string ~json:"search"~
}

// Held fills the intermediate from its input into a local, copies it into
// another, and hands the protection helper each one a loop over them binds.
func Held(c *gl.Client, in HeldInput) error {
	held := filter{search: in.Search}
	var copied = held
	for _, each := range []filter{copied} {
		if err := c.Widgets.Protect(1, protectOf(each)); err != nil {
			return err
		}
	}
	return nil
}

type ForkInput struct {
	Search string ~json:"search"~
}

// forkOptions takes an input and references no method: it builds the
// options for whichever function calls it.
func forkOptions(in ForkInput) *gl.ListWidgetsOptions { return widgetdata.ListOptions(0, in.Search) }

// Forks builds its options in a function that takes its input whole.
func Forks(c *gl.Client, in ForkInput) error {
	_, err := c.Widgets.ListWidgets(forkOptions(in))
	return err
}

type GroupForkInput struct {
	Search string ~json:"search"~
}

// GroupForks hands that builder a literal of another input, filled from its
// own.
func GroupForks(c *gl.Client, in GroupForkInput) error {
	_, err := c.Widgets.ListWidgets(forkOptions(ForkInput{Search: in.Search}))
	return err
}

type DelegateInput struct {
	Search string ~json:"search"~
}

// Delegate calls a handler, which sends its own request and so is no
// helper, and sends the options it builds itself as nil.
func Delegate(c *gl.Client, in DelegateInput) error {
	if err := List(c, ListInput{Search: in.Search}); err != nil {
		return err
	}
	_, err := c.Widgets.ListWidgets(nil)
	return err
}
`

// helperTree checks the two helper fixture packages and returns them,
// helpers first.
func helperTree(t *testing.T) []*packages.Package {
	t.Helper()
	return checkedTree(t, pairingSDK,
		toolSource{path: "example.com/x/internal/tools/widgetdata", source: helperDataSource},
		toolSource{path: "example.com/x/internal/tools/widgets", source: helperCallerSource},
	)
}

// callerPairs runs collectCallerInputs over pkg and returns what it recorded,
// as "<input> <sdk struct> <via>".
func callerPairs(pkg *packages.Package) []string {
	pairs := map[[2]string]structPair{}
	collectCallerInputs(pkg, pairs)
	out := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, pair.mcpName+" "+pair.sdkName+" "+pair.via)
	}
	slices.Sort(out)
	return out
}

// TestCollectCallerInputs_AHelperBuildsTheOptions_PairsThemWithTheInputOfItsCaller
// verifies the pairing through a call: a literal a helper builds out of
// scalars is paired with the input of each handler that hands the helper its
// input's fields, when the helper is in another tool package, two helpers
// deep, or a struct of arguments the handler fills and a shared body reads,
// and each pair says it was found through the caller. An intermediate the
// handler fills from its input carries the input on: into the parameter of a
// closure, into a helper named as a value beside it, into a helper that
// sends the options itself, and through a local, a copy of it and a loop
// over them. A function of the package that hands back several results is
// read through its body, so only the result it hands the intermediate back
// in holds it, whether its value, a named result or another function's
// result, and each does when it hands back what nothing read tells apart (a
// function value's results, its own); and a closure it is handed holds the
// intermediate only where it calls the closure with it, never when it names
// no parameter to call, and wherever it hands the closure on or is itself
// out of sight. A function that takes an input and references no
// method builds for its caller, whether the caller hands it its own input
// or a literal of the builder's. A handler that hands the helper constants,
// or an
// intermediate filled with them (even one whose type the same handler fills
// from its input elsewhere, in a literal, a local, beside a closure or
// beside a helper named as a value), one that sends nothing taking the
// struct, one that calls a handler, one whose input no model sends, one
// whose input has no name, and the calls that name no function pair nothing.
func TestCollectCallerInputs_AHelperBuildsTheOptions_PairsThemWithTheInputOfItsCaller(t *testing.T) {
	pkgs := helperTree(t)

	got := callerPairs(pkgs[1])

	want := []string{
		"BuildInput v3.ListWidgetsOptions caller",
		"ChainInput v3.ListWidgetsOptions caller",
		"ForkInput v3.ListWidgetsOptions caller",
		"GroupForkInput v3.ListWidgetsOptions caller",
		"HeldInput v3.ProtectOptions caller",
		"IndirectInput v3.ListWidgetsOptions caller",
		"ListInput v3.ListWidgetsOptions caller",
		"MixedInput v3.ListWidgetsOptions caller",
		"PickInput v3.ProtectOptions caller",
		"RelayInput v3.ListWidgetsOptions caller",
		"RunInput v3.ListWidgetsOptions caller",
		"ScopedInput v3.ListWidgetsOptions caller",
		"SplitInput v3.ListWidgetsOptions caller",
		"SwapInput v3.ListWidgetsOptions caller",
		"TokenInput v3.ListWidgetsOptions caller",
		"TokenLitInput v3.ListWidgetsOptions caller",
	}
	if !slices.Equal(got, want) {
		t.Errorf("collectCallerInputs = %v, want %v", got, want)
	}
	pairs := map[[2]string]structPair{}
	collectCallerInputs(pkgs[1], pairs)
	pair := pairs[[2]string{"ListInput", "ListWidgetsOptions"}]
	if pair.mcpNamed == nil || pair.mcpNamed.Obj().Name() != "ListInput" || pair.sdkNamed == nil || pair.sdkNamed.Obj().Name() != "ListWidgetsOptions" || !pair.sdkURLTags {
		t.Errorf("the ListInput pair = %+v, want both named sides and url tags first", pair)
	}
	if st, _ := pair.sdkNamed.Underlying().(*types.Struct); pair.sdkType != st {
		t.Error("the ListInput pair compares a struct other than the one its SDK side names")
	}
	if helpersOnly := callerPairs(pkgs[0]); len(helpersOnly) != 0 {
		t.Errorf("collectCallerInputs(widgetdata) = %v, want nothing from a package of helpers alone", helpersOnly)
	}
}

// TestCollectCallerInputs_ADirectPair_IsKeptOverTheCallerOne verifies the
// order discoverPairs records the kinds in: a struct a handler both builds
// itself and builds through a helper keeps the pair of its own literal.
func TestCollectCallerInputs_ADirectPair_IsKeptOverTheCallerOne(t *testing.T) {
	pkgs := checkedTree(t, pairingSDK,
		toolSource{path: "example.com/x/internal/tools/widgetdata", source: helperDataSource},
		toolSource{path: "example.com/x/internal/tools/widgets", source: `package widgets

import (
	gl "` + fixtureSDKPath + `"
	"example.com/x/internal/tools/widgetdata"
)

type ListInput struct {
	Search string ~json:"search"~
}

func List(c *gl.Client, in ListInput) error {
	opts := &gl.ListWidgetsOptions{Search: &in.Search}
	if in.Search == "" {
		opts = widgetdata.ListOptions(1, in.Search)
	}
	_, err := c.Widgets.ListWidgets(opts)
	return err
}
`})

	inputs, _ := discoverPairs(pkgs[1])

	if pair := inputs[[2]string{"ListInput", "ListWidgetsOptions"}]; len(inputs) != 1 || pair.via != "" {
		t.Errorf("discoverPairs = %+v, want the one pair the handler's own literal found", inputs)
	}
}

// TestHelpersOf_APackage_IsWalkedOnce verifies the memo: a package's helpers
// are read by every package that imports it, and each read after the first
// is answered with the same map.
func TestHelpersOf_APackage_IsWalkedOnce(t *testing.T) {
	pkgs := helperTree(t)

	first := helpersOf(pkgs[0])
	second := helpersOf(pkgs[0])

	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("helpersOf walked the package again")
	}
	var built []string
	for key, structs := range first {
		for _, named := range structs {
			built = append(built, key[strings.LastIndex(key, ".")+1:]+" "+named.Obj().Name())
		}
	}
	slices.Sort(built)
	if want := []string{"Chain ListWidgetsOptions", "Deeper ListWidgetsOptions", "ListOptions ListWidgetsOptions"}; !slices.Equal(built, want) {
		t.Errorf("helpersOf(widgetdata) = %v, want %v", built, want)
	}
}

// TestImportedToolPackages_AnImportGraph_KeepsTheToolPackagesWithSyntax
// verifies which imports a package's helpers are read from: a tool package
// loaded with its syntax and its types, and never the SDK, a tool package
// without type information or one without syntax.
func TestImportedToolPackages_AnImportGraph_KeepsTheToolPackagesWithSyntax(t *testing.T) {
	loaded := &packages.Package{PkgPath: "example.com/x/internal/tools/loaded", TypesInfo: &types.Info{}, Syntax: []*ast.File{{}}}
	pkg := &packages.Package{Imports: map[string]*packages.Package{
		loaded.PkgPath:                          loaded,
		fixtureSDKPath:                          {PkgPath: fixtureSDKPath, TypesInfo: &types.Info{}, Syntax: []*ast.File{{}}},
		"example.com/x/internal/tools/untyped":  {PkgPath: "example.com/x/internal/tools/untyped", Syntax: []*ast.File{{}}},
		"example.com/x/internal/tools/nosyntax": {PkgPath: "example.com/x/internal/tools/nosyntax", TypesInfo: &types.Info{}},
	}}

	got := importedToolPackages(pkg)

	if len(got) != 1 || got[0] != loaded {
		t.Errorf("importedToolPackages = %v, want the one tool package loaded with syntax and types", got)
	}
}

// TestCollectHelpers_ADeclarationWithNoObjectOrNoBody_IsPassedOver verifies
// the two declarations the helper scan skips, which a type-checked package
// never produces: a function without a body, declared outside Go, and one
// whose name the type information does not define.
func TestCollectHelpers_ADeclarationWithNoObjectOrNoBody_IsPassedOver(t *testing.T) {
	bodyless := &ast.FuncDecl{Name: ast.NewIdent("external"), Type: &ast.FuncType{Params: &ast.FieldList{}}}
	unknown := &ast.FuncDecl{Name: ast.NewIdent("unknown"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{}}
	pkg := &packages.Package{
		PkgPath:   "example.com/x/internal/tools/p",
		TypesInfo: &types.Info{Defs: map[*ast.Ident]types.Object{}},
		Syntax:    []*ast.File{{Decls: []ast.Decl{bodyless, unknown, &ast.GenDecl{}}}},
	}

	if got := collectHelpers(pkg); len(got) != 0 {
		t.Errorf("collectHelpers = %v, want no helper", got)
	}
	pairs := map[[2]string]structPair{}
	collectCallerInputs(pkg, pairs)
	collectNestedInputs(pkg, pairs)
	if len(pairs) != 0 {
		t.Errorf("collectCallerInputs and collectNestedInputs = %v, want no pair", pairs)
	}
}

// TestBoundParams_ANameTheTypeInformationDoesNotDefine_IsLeftOut verifies
// the variables a helper's receiver and parameters bind are the ones the type
// information defines, an unnamed parameter and a name it has no variable
// for left out.
func TestBoundParams_ANameTheTypeInformationDoesNotDefine_IsLeftOut(t *testing.T) {
	recv, defined, undefined := ast.NewIdent("w"), ast.NewIdent("page"), ast.NewIdent("search")
	recvVar := types.NewParam(0, nil, "w", types.Typ[types.Int])
	pageVar := types.NewParam(0, nil, "page", types.Typ[types.Int])
	fn := &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{recv}}}},
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{defined, undefined}}, {}}}},
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{recv: recvVar, defined: pageVar}}

	got := boundParams(info, fn)

	if want := map[*types.Var]bool{recvVar: true, pageVar: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("boundParams = %v, want %v", got, want)
	}
}

// TestSentStructs_ASelectorTheTypeCheckerDidNotResolve_SendsNothing verifies
// a selector on a service interface whose method the type information does
// not name, which a type-checked package never holds, adds no struct.
func TestSentStructs_ASelectorTheTypeCheckerDidNotResolve_SendsNothing(t *testing.T) {
	sdk := types.NewPackage(fixtureSDKPath, "gitlab")
	service := types.NewNamed(types.NewTypeName(0, sdk, "WidgetsServiceInterface", nil), types.NewInterfaceType(nil, nil), nil)
	receiver := ast.NewIdent("widgets")
	body := &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.SelectorExpr{X: receiver, Sel: ast.NewIdent("ListWidgets")}}}}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{receiver: {Type: service}}, Uses: map[*ast.Ident]types.Object{}}

	if sent := sentStructs(info, body); len(sent) != 0 {
		t.Errorf("sentStructs = %v, want nothing", sent)
	}
}

// TestCollectPairs_Repository_PairsWhatAHelperBuildsWithTheInputOfItsCaller
// is the witness of the pairing through a call on the tree as it stands: the
// options structs a helper with no input of its own builds were compared with
// no input at all, and each of these is now compared with the input of the
// handler that hands the helper its fields, across a package boundary
// (labeldata, iterationdata) and through the struct of arguments every
// search fills. The intermediate a handler fills and hands on whole carries
// its input into a closure (the runner lists), into a helper named as a
// value (the issue statistics) and into a helper that sends the options
// itself (the access token creates), and a function that takes an input and
// sends nothing builds for its caller (the project lists, the group member
// role, the SSH key a user is given).
func TestCollectPairs_Repository_PairsWhatAHelperBuildsWithTheInputOfItsCaller(t *testing.T) {
	assertRepositoryPairs(t, []string{
		"accessrequests ListProjectInput ListAccessRequestsOptions",
		"accesstokens GroupCreateInput CreateGroupAccessTokenOptions",
		"accesstokens ProjectCreateInput CreateProjectAccessTokenOptions",
		"auditevents ListProjectInput ListAuditEventsOptions",
		"dorametrics ProjectInput GetDORAMetricsOptions",
		"groupiterations ListInput ListGroupIterationsOptions",
		"grouplabels ListInput ListGroupLabelsOptions",
		"issuestatistics GetGroupInput GetGroupIssuesStatisticsOptions",
		"issuestatistics GetProjectInput GetProjectIssuesStatisticsOptions",
		"labels ListInput ListLabelsOptions",
		"memberroles CreateGroupInput CreateMemberRoleOptions",
		"projectiterations ListInput ListProjectIterationsOptions",
		"projects ListForksInput ListProjectsOptions",
		"projects ListInput ListProjectsOptions",
		"runners ListGroupInput ListGroupsRunnersOptions",
		"runners ListProjectInput ListProjectRunnersOptions",
		"search MergeRequestsInput SearchOptions",
		"search NotesInput SearchOptions",
		"users AddSSHKeyForUserInput AddSSHKeyOptions",
	})
}

// assertRepositoryPairs checks that every "<package> <input> <sdk struct>"
// in want is among the input pairs CollectPairs lists for the tree.
func assertRepositoryPairs(t *testing.T, want []string) {
	t.Helper()
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	listed := map[string]bool{}
	for _, pkg := range pkgs {
		for _, pair := range CollectPairs(pkg) {
			if pair.Kind == "input" {
				listed[shared.ShortPackage(pkg.PkgPath)+" "+pair.MCPName+" "+sdkLeaf(pair.SDKName)] = true
			}
		}
	}
	for _, pair := range want {
		t.Run(pair, func(t *testing.T) {
			if !listed[pair] {
				t.Errorf("CollectPairs lists no input pair %q", pair)
			}
		})
	}
}
