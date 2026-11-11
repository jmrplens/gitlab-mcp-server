// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package structs

import (
	"go/ast"
	"go/types"
	"maps"
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

// twice hands the intermediate to the function its caller passes, two times.
func twice(f filter, list func(filter) error) error {
	if err := list(f); err != nil {
		return err
	}
	return list(f)
}

type TwiceInput struct {
	Search string ~json:"search"~
}

// Twice hands its closure to a function that calls it twice with the
// intermediate.
func Twice(c *gl.Client, in TwiceInput) error {
	return twice(filter{search: in.Search}, func(f filter) error {
		_, err := c.Widgets.ListWidgets(optionsOf(f))
		return err
	})
}

// firstOf hands back the first of the intermediates it is handed.
func firstOf(fs []filter) filter { return fs[0] }

type FieldInput struct {
	Search string ~json:"search"~
}

// Field holds the intermediate in a field of a local, hands the field to the
// helper, and hands on a list of intermediates whose last is that field;
// the same field of a local filled with a constant carries nothing.
func Field(c *gl.Client, in FieldInput) error {
	r := read{f: filter{search: in.Search}}
	fixedRead := read{f: filter{search: fixed}}
	_ = firstOf([]filter{{search: in.Search}, {search: in.Search}, r.f})
	_ = optionsOf(fixedRead.f)
	_, err := c.Widgets.ListWidgets(optionsOf(r.f))
	return err
}

// keep hands the intermediate back in its first named result, by a bare
// return.
func keep(f filter) (kept, other filter) {
	kept = f
	return
}

type KeepInput struct {
	Search string ~json:"search"~
}

// Keep takes the intermediate back from the first of two named results.
func Keep(c *gl.Client, in KeepInput) error {
	kept, _ := keep(filter{search: in.Search})
	_, err := c.Widgets.ListWidgets(optionsOf(kept))
	return err
}

// optionsBeside builds the options out of the intermediate it takes after a
// struct its callers do not fill from their input.
func optionsBeside(a args, f filter) *gl.ListWidgetsOptions {
	return widgetdata.ListOptions(0, f.search+a.search)
}

// applyBeside hands the intermediate to the builder its caller passes.
func applyBeside(f filter, build func(args, filter) *gl.ListWidgetsOptions) *gl.ListWidgetsOptions {
	return build(args{}, f)
}

type BesideInput struct {
	Search string ~json:"search"~
}

// Beside names as a value, beside the intermediate, a helper whose first
// parameter is a struct it does not fill from its input.
func Beside(c *gl.Client, in BesideInput) error {
	_, err := c.Widgets.ListWidgets(applyBeside(filter{search: in.Search}, optionsBeside))
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
// closure, the closure called once or twice with it, into a helper named as
// a value beside it, one whose first parameter is a struct the handler does
// not fill among them, into a helper that sends the options itself, and
// through a local, a copy of it, a loop over them and a field of a local,
// whether the field is handed on alone or after other intermediates in a
// list. A function of the package that hands back several results is
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
		"BesideInput v3.ListWidgetsOptions caller",
		"BuildInput v3.ListWidgetsOptions caller",
		"ChainInput v3.ListWidgetsOptions caller",
		"FieldInput v3.ListWidgetsOptions caller",
		"ForkInput v3.ListWidgetsOptions caller",
		"GroupForkInput v3.ListWidgetsOptions caller",
		"HeldInput v3.ProtectOptions caller",
		"IndirectInput v3.ListWidgetsOptions caller",
		"KeepInput v3.ListWidgetsOptions caller",
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
		"TwiceInput v3.ListWidgetsOptions caller",
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

// throughSource is a tool package whose handlers reach their options through
// the struct an options builder takes, the shape of projects.userProjectFilter
// and notifications.eventFields: the builder's own pair is answered by a
// declaration written on that struct, and the inputs that go through it lack
// the same field for the same reason.
const throughSource = `package fixturepkg

import gl "` + fixtureSDKPath + `"

// Output is what every handler here answers with.
type Output struct {
	ID int64 ~json:"id"~
}

// filter is the struct the list handlers hand the options builder. It
// offers no page.
type filter struct {
	Search string ~json:"search"~
}

// optionsOf builds the options out of the filter.
func optionsOf(f filter) *gl.ListWidgetsOptions { return &gl.ListWidgetsOptions{Search: &f.Search} }

type FirstInput struct {
	Search string ~json:"search"~
}

// First fills the filter in a literal.
func First(c *gl.Client, in FirstInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsOf(filter{Search: in.Search}))
	return Output{}, err
}

type SecondInput struct {
	filter
}

// Second hands the builder the filter it embeds.
func Second(c *gl.Client, in SecondInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsOf(in.filter))
	return Output{}, err
}

// defaults is a struct of constants a builder takes before the filter:
// nothing of an input goes through it.
type defaults struct {
	page int64
}

// optionsBeside builds the options out of the filter and the constants.
func optionsBeside(d defaults, f filter) *gl.ListWidgetsOptions {
	opts := optionsOf(f)
	opts.Page = d.page
	return opts
}

type ThirdInput struct {
	Search string ~json:"search"~
}

// Third hands that builder the filter beside a literal of constants.
func Third(c *gl.Client, in ThirdInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsBeside(defaults{page: 1}, filter{Search: in.Search}))
	return Output{}, err
}

// guard is the struct the protection builder takes.
type guard struct {
	Name string ~json:"name"~
}

// protectOf builds the protection out of the guard.
func protectOf(g guard) *gl.ProtectOptions { return &gl.ProtectOptions{Name: &g.Name} }

type GuardInput struct {
	Name string ~json:"name"~
}

// Guard hands the protection builder a guard of its own.
func Guard(c *gl.Client, in GuardInput) (Output, error) {
	return Output{}, c.Widgets.Protect(1, protectOf(guard{Name: in.Name}))
}

// complete offers every field the options carry, so its own pair lacks
// nothing.
type complete struct {
	Search string ~json:"search"~
	Page   int64  ~json:"page"~
}

// optionsOfComplete builds the options out of a complete.
func optionsOfComplete(k complete) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{Search: &k.Search}
	opts.Page = k.Page
	return opts
}

// optionsSearching hands the complete it is given on to another builder and
// sets the search from a scalar.
func optionsSearching(k complete, search string) *gl.ListWidgetsOptions {
	opts := optionsOfComplete(k)
	opts.Search = &search
	return opts
}

type FourthInput struct {
	Search string ~json:"search"~
}

// Fourth hands that builder a complete of constants beside its search:
// nothing of the input goes through complete, at the call or down the
// chain.
func Fourth(c *gl.Client, in FourthInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsSearching(complete{Search: "x", Page: 1}, in.Search))
	return Output{}, err
}

// optionsOfEither builds the options out of the first filter with a search.
func optionsOfEither(a, b filter) *gl.ListWidgetsOptions {
	if a.Search == "" {
		return optionsOf(b)
	}
	return optionsOf(a)
}

type SixthInput struct {
	Search string ~json:"search"~
}

// Sixth hands that builder two filters it fills from its input, so the
// input goes through the filter once.
func Sixth(c *gl.Client, in SixthInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsOfEither(filter{Search: in.Search}, filter{Search: in.Search}))
	return Output{}, err
}

// scope carries a filter to a method that builds from it.
type scope struct {
	f filter
}

// options hands the filter its receiver carries to the builder.
func (s scope) options() *gl.ListWidgetsOptions { return optionsOf(s.f) }

type FifthInput struct {
	Search string ~json:"search"~
}

// Fifth calls that method on a scope it fills from its input, so the input
// goes through the scope and, down the chain, through the filter.
func Fifth(c *gl.Client, in FifthInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(scope{f: filter{Search: in.Search}}.options())
	return Output{}, err
}

// listAndProtect builds the options out of the filter, and protects with
// constants beside.
func listAndProtect(c *gl.Client, f filter) (*gl.ListWidgetsOptions, error) {
	return optionsOf(f), c.Widgets.Protect(1, &gl.ProtectOptions{})
}

type SeventhInput struct {
	Search string ~json:"search"~
}

// Seventh hands that builder a filter it fills from its input: the input
// goes through the filter into the options and into nothing of the
// protection.
func Seventh(c *gl.Client, in SeventhInput) (Output, error) {
	opts, err := listAndProtect(c, filter{Search: in.Search})
	if err != nil {
		return Output{}, err
	}
	_, err = c.Widgets.ListWidgets(opts)
	return Output{}, err
}

// relayProtect hands the filter on to the builder that protects with
// constants beside.
func relayProtect(c *gl.Client, f filter) (*gl.ListWidgetsOptions, error) {
	return listAndProtect(c, f)
}

type TenthInput struct {
	Search string ~json:"search"~
}

// Tenth hands the relay a filter it fills from its input: down the chain
// too, the protection reads nothing of it.
func Tenth(c *gl.Client, in TenthInput) (Output, error) {
	opts, err := relayProtect(c, filter{Search: in.Search})
	if err != nil {
		return Output{}, err
	}
	_, err = c.Widgets.ListWidgets(opts)
	return Output{}, err
}

// optionsRelayed builds options of its own, or hands the guard on to the
// builder below, which writes it into what another builder hands back. It is
// declared before that builder on purpose: what the guard reaches there is
// known only after a pass over both, so it is read on the next one.
func optionsRelayed(g guard, own bool) *gl.ListWidgetsOptions {
	if own {
		return &gl.ListWidgetsOptions{}
	}
	return optionsNamed(filter{Search: "x"}, g)
}

type TwelfthInput struct {
	Name string ~json:"name"~
}

// Twelfth hands that relay a guard it fills from its input.
func Twelfth(c *gl.Client, in TwelfthInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsRelayed(guard{Name: in.Name}, false))
	return Output{}, err
}

// optionsNamed builds the options out of the filter, and writes the guard's
// name into what that builder hands back.
func optionsNamed(f filter, g guard) *gl.ListWidgetsOptions {
	opts := optionsOf(f)
	opts.Search = &g.Name
	return opts
}

type EighthInput struct {
	Name string ~json:"name"~
}

// Eighth hands that builder a filter of constants beside a guard it fills
// from its input, so the input reaches the options through the guard alone.
func Eighth(c *gl.Client, in EighthInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsNamed(filter{Search: "x"}, guard{Name: in.Name}))
	return Output{}, err
}

// optionsDeclared is optionsNamed with the options it writes into declared.
func optionsDeclared(f filter, g guard) *gl.ListWidgetsOptions {
	var opts = optionsOf(f)
	opts.Search = &g.Name
	return opts
}

type EleventhInput struct {
	Name string ~json:"name"~
}

// Eleventh hands that builder a filter of constants beside a guard it fills
// from its input.
func Eleventh(c *gl.Client, in EleventhInput) (Output, error) {
	_, err := c.Widgets.ListWidgets(optionsDeclared(filter{Search: "x"}, guard{Name: in.Name}))
	return Output{}, err
}

// listOrProtect builds the options out of the filter and the protection out
// of the guard.
func listOrProtect(f filter, g guard) (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	return optionsOf(f), protectOf(g)
}

type NinthInput struct {
	Search string ~json:"search"~
	Name   string ~json:"name"~
}

// Ninth hands that builder a filter and a guard it fills from its input:
// the input goes through the filter into the options and through the guard
// into the protection.
func Ninth(c *gl.Client, in NinthInput) (Output, error) {
	list, protect := listOrProtect(filter{Search: in.Search}, guard{Name: in.Name})
	if err := c.Widgets.Protect(1, protect); err != nil {
		return Output{}, err
	}
	_, err := c.Widgets.ListWidgets(list)
	return Output{}, err
}

// optionsPaired builds the options and the protection out of the filter and
// a guard of constants, and writes the guard it is handed into the options
// alone, the first of the two that builder hands back.
func optionsPaired(f filter, g guard) (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	list, protect := listOrProtect(f, guard{Name: "x"})
	list.Search = &g.Name
	return list, protect
}

type ThirteenthInput struct {
	Search string ~json:"search"~
	Name   string ~json:"name"~
}

// Thirteenth hands that builder a filter and a guard it fills from its
// input: the guard reaches the options and nothing of the protection.
func Thirteenth(c *gl.Client, in ThirteenthInput) (Output, error) {
	list, protect := optionsPaired(filter{Search: in.Search}, guard{Name: in.Name})
	if err := c.Widgets.Protect(1, protect); err != nil {
		return Output{}, err
	}
	_, err := c.Widgets.ListWidgets(list)
	return Output{}, err
}

// optionsVia has the builder its caller passes build the options out of the
// filter, a call whose body is out of sight.
func optionsVia(f filter, build func(filter) *gl.ListWidgetsOptions) *gl.ListWidgetsOptions {
	opts := build(f)
	return opts
}

// defaultOptions builds options of constants.
func defaultOptions() *gl.ListWidgetsOptions { return &gl.ListWidgetsOptions{} }

// optionsDefaulted writes the guard's name into the options of constants
// another builder hands back, which no entry of its own reaches by a call.
// It is the last builder here, and the declaration reaches nothing.
func optionsDefaulted(g guard) *gl.ListWidgetsOptions {
	var opts = defaultOptions()
	opts.Search = &g.Name
	return opts
}
`

// routesSource is a tool package whose handlers reach one options struct by
// more than one route.
const routesSource = `package fixturepkg

import gl "` + fixtureSDKPath + `"

type filter struct {
	Search string ~json:"search"~
}

// optionsOf builds the options out of the filter.
func optionsOf(f filter) *gl.ListWidgetsOptions { return &gl.ListWidgetsOptions{Search: &f.Search} }

type guard struct {
	Name string ~json:"name"~
}

// optionsGuarded builds the options out of a guard, beside a struct of
// client-go's own it copies the page from.
func optionsGuarded(base *gl.ListOptions, g guard) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{Search: &g.Name}
	opts.Page = base.Page
	return opts
}

// optionsOfEach builds the options out of a prefix and every filter it is
// handed.
func optionsOfEach(prefix string, fs ...filter) *gl.ListWidgetsOptions {
	search := prefix
	for _, f := range fs {
		search += f.Search
	}
	return &gl.ListWidgetsOptions{Search: &search}
}

type DirectInput struct {
	Search string ~json:"search"~
}

// Direct builds the options itself, and through a builder when the search
// is empty.
func Direct(c *gl.Client, in DirectInput) error {
	opts := &gl.ListWidgetsOptions{Search: &in.Search}
	if in.Search == "" {
		opts = optionsOf(filter{Search: in.Search})
	}
	_, err := c.Widgets.ListWidgets(opts)
	return err
}

type BothInput struct {
	Search string ~json:"search"~
	Page   int64  ~json:"page"~
}

// Both reaches the options through two builders, one of which it hands a
// struct of client-go's own beside the guard.
func Both(c *gl.Client, in BothInput) error {
	if _, err := c.Widgets.ListWidgets(optionsOf(filter{Search: in.Search})); err != nil {
		return err
	}
	_, err := c.Widgets.ListWidgets(optionsGuarded(&gl.ListOptions{Page: in.Page}, guard{Name: in.Search}))
	return err
}

type EachInput struct {
	Search string ~json:"search"~
}

// Each hands a builder that takes any number of filters two of them.
func Each(c *gl.Client, in EachInput) error {
	_, err := c.Widgets.ListWidgets(optionsOfEach(in.Search, filter{Search: in.Search}, filter{Search: in.Search}))
	return err
}

type LastInput struct {
	Search string ~json:"search"~
}

// Last hands that builder a filter of constants and, after it, one it fills
// from its input, so only a value past the first of the list carries it.
func Last(c *gl.Client, in LastInput) error {
	_, err := c.Widgets.ListWidgets(optionsOfEach("", filter{Search: "x"}, filter{Search: in.Search}))
	return err
}

// applyEach hands the filter to the builder its caller passes.
func applyEach(f filter, build func(string, ...filter) *gl.ListWidgetsOptions) *gl.ListWidgetsOptions {
	return build("", f)
}

type ValueEachInput struct {
	Search string ~json:"search"~
}

// ValueEach names as a value, beside a filter it fills from its input, the
// builder that takes any number of them.
func ValueEach(c *gl.Client, in ValueEachInput) error {
	_, err := c.Widgets.ListWidgets(applyEach(filter{Search: in.Search}, optionsOfEach))
	return err
}
`

// TestRecordInputPair_TheRoutesToOnePair_KeepTheDirectOneAndJoinTheCallerOnes
// verifies what one pair found by several routes goes through: a handler's
// own literal keeps its pair however a builder reaches the same struct, and
// goes through nothing; a pair found through two builders goes through the
// struct each takes, and never through a struct of client-go's handed
// beside one. A builder that takes any number of filters goes through the
// filter, the struct of each value its last parameter takes, whether it is
// called with them or named as a value beside one.
func TestRecordInputPair_TheRoutesToOnePair_KeepTheDirectOneAndJoinTheCallerOnes(t *testing.T) {
	pkg := checkedTree(t, pairingSDK, toolSource{path: "example.com/x/internal/tools/fixturepkg", source: routesSource})[0]

	inputs, _ := discoverPairs(pkg)

	cases := []struct {
		input, via string
		through    []string
	}{
		{input: "DirectInput"},
		{input: "BothInput", via: ViaCaller, through: []string{"fixturepkg.filter", "fixturepkg.guard"}},
		{input: "EachInput", via: ViaCaller, through: []string{"fixturepkg.filter"}},
		{input: "LastInput", via: ViaCaller, through: []string{"fixturepkg.filter"}},
		{input: "ValueEachInput", via: ViaCaller, through: []string{"fixturepkg.filter"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			pair, found := inputs[[2]string{tc.input, "ListWidgetsOptions"}]
			if !found || pair.via != tc.via || !slices.Equal(pair.through, tc.through) {
				t.Errorf("the %s pair = found %t, via %q, through %#v; want via %q, through %#v", tc.input, found, pair.via, pair.through, tc.via, tc.through)
			}
		})
	}
}

// inputGaps runs the field diff over pkg and returns each input pair it
// reports, as "<input> <sdk struct> <via> <missing tags>", and the keys of
// acceptedMissingInputs the run left unanswered among keys.
func inputGaps(pkg *packages.Package, keys ...string) (gaps, stale []string) {
	run := newDiffRun()
	pr, _ := run.analyzePackage(pkg)
	for _, g := range pr.Gaps {
		if g.Kind != "input" || len(g.MissingFields) == 0 {
			continue
		}
		tags := make([]string, 0, len(g.MissingFields))
		for _, field := range g.MissingFields {
			tags = append(tags, field.Tag)
		}
		gaps = append(gaps, g.MCPType+" "+sdkLeaf(g.SDKType)+" "+g.Via+" "+strings.Join(tags, ","))
	}
	for _, entry := range run.staleDeclarations([]*declarationTable{acceptedMissingInputs}) {
		if slices.Contains(keys, entry.Key) {
			stale = append(stale, entry.Key)
		}
	}
	return gaps, stale
}

// TestAnalyzePackage_ACallerPairThroughADeclaredStruct_IsAnsweredByItsDeclaration
// verifies that a pair found through the caller of a builder is answered by a
// declaration written on the struct the builder takes: the builder's own
// pair and every input handed through it lack the field for one reason,
// whether the input fills the struct in a literal, embeds it, hands it
// beside a struct of constants, hands it twice or fills a receiver whose
// method hands it on.
// A declaration answers only the inputs whose values went through its
// struct, at the call and at every step down the chain: one on a struct of
// constants a builder takes beside the input answers nothing and is stale,
// and so is one on a struct of constants that builder hands on to another,
// while the input handed beside it in a scalar still reports what it lacks.
// An input handed through a struct no declaration names reports too. The
// whole-type form answers every field the same way.
//
// A struct a builder takes goes through into what it reaches of what the
// builder builds, and only that: through a helper further down, and through
// what such a helper hands back when the builder writes the struct into it,
// in the result it writes it into alone. A request the builder makes of
// constants beside, or out of another struct it takes, at the call or down
// the chain, goes through nothing the caller hands, so a declaration on the
// struct answers nothing of it, and neither does one of several results of
// a call that nothing of the caller's is written into or handed back in.
func TestAnalyzePackage_ACallerPairThroughADeclaredStruct_IsAnsweredByItsDeclaration(t *testing.T) {
	pkg := checkedTree(t, pairingSDK, toolSource{path: "example.com/x/internal/tools/fixturepkg", source: throughSource})[0]
	fourthGap := "FourthInput ListWidgetsOptions caller page"

	t.Run("a per-field declaration on the struct a builder takes", func(t *testing.T) {
		declare(t, acceptedMissingInputs, "fixturepkg.filter.page", "the filter offers no page")
		declare(t, acceptedMissingInputs, "fixturepkg.defaults.page", "nothing of an input goes through the constants")
		declare(t, acceptedMissingInputs, "fixturepkg.complete.page", "nothing of an input goes through the complete")

		gaps, stale := inputGaps(pkg, "fixturepkg.filter.page", "fixturepkg.defaults.page", "fixturepkg.complete.page")

		wantGaps := []string{
			"EighthInput ListWidgetsOptions caller page,search",
			"EleventhInput ListWidgetsOptions caller page,search",
			fourthGap,
			"GuardInput ProtectOptions caller allowed,by_user,levels,page,since",
			"NinthInput ProtectOptions caller allowed,by_user,levels,page,since",
			"ThirteenthInput ProtectOptions caller allowed,by_user,levels,page,since",
			"TwelfthInput ListWidgetsOptions caller page,search",
			"guard ProtectOptions  allowed,by_user,levels,page,since",
		}
		if !slices.Equal(gaps, wantGaps) {
			t.Errorf("input gaps = %q, want only %q", gaps, wantGaps)
		}
		if want := []string{"fixturepkg.complete.page", "fixturepkg.defaults.page"}; !slices.Equal(stale, want) {
			t.Errorf("stale declarations = %q, want %q", stale, want)
		}
	})

	t.Run("a whole-type declaration on the struct a builder takes", func(t *testing.T) {
		declare(t, acceptedMissingInputs, "fixturepkg.guard", "the guard offers the name alone")

		gaps, stale := inputGaps(pkg, "fixturepkg.guard")

		want := []string{
			"FifthInput ListWidgetsOptions caller page",
			"FirstInput ListWidgetsOptions caller page",
			fourthGap,
			"NinthInput ListWidgetsOptions caller page",
			"SecondInput ListWidgetsOptions caller page",
			"SeventhInput ListWidgetsOptions caller page",
			"SixthInput ListWidgetsOptions caller page",
			"TenthInput ListWidgetsOptions caller page",
			"ThirdInput ListWidgetsOptions caller page",
			"ThirteenthInput ProtectOptions caller allowed,by_user,levels,page,since",
			"filter ListWidgetsOptions  page",
		}
		if !slices.Equal(gaps, want) {
			t.Errorf("input gaps = %q, want %q", gaps, want)
		}
		if len(stale) != 0 {
			t.Errorf("stale declarations = %q, want none", stale)
		}
	})

	t.Run("what each input goes through", func(t *testing.T) {
		inputs, _ := discoverPairs(pkg)

		for _, tc := range []struct {
			input, sdk string
			want       []string
		}{
			{input: "FifthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter", "fixturepkg.scope"}},
			{input: "FourthInput", sdk: "ListWidgetsOptions", want: []string{}},
			{input: "SixthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter"}},
			{input: "ThirdInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter"}},
			{input: "SeventhInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter"}},
			{input: "SeventhInput", sdk: "ProtectOptions", want: []string{}},
			{input: "TenthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter"}},
			{input: "TenthInput", sdk: "ProtectOptions", want: []string{}},
			{input: "EighthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.guard"}},
			{input: "EleventhInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.guard"}},
			{input: "TwelfthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.guard"}},
			{input: "NinthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter"}},
			{input: "NinthInput", sdk: "ProtectOptions", want: []string{"fixturepkg.guard"}},
			{input: "ThirteenthInput", sdk: "ListWidgetsOptions", want: []string{"fixturepkg.filter", "fixturepkg.guard"}},
			{input: "ThirteenthInput", sdk: "ProtectOptions", want: []string{}},
		} {
			t.Run(tc.input+" "+tc.sdk, func(t *testing.T) {
				pair, found := inputs[[2]string{tc.input, tc.sdk}]
				if !found || !slices.Equal(pair.through, tc.want) {
					t.Errorf("%s %s found %t, goes through %q; want %q", tc.input, tc.sdk, found, pair.through, tc.want)
				}
			})
		}
	})
}

// passesSource is a tool package whose handlers hold the input in ways only
// a later pass over the body, a method's receiver, a field of a local or the
// target of an assignment tells apart.
const passesSource = `package passes

import gl "` + fixtureSDKPath + `"

// filter is an intermediate the handlers fill from their input.
type filter struct {
	search string
}

// optionsOf builds the options out of the intermediate.
func optionsOf(f filter) *gl.ListWidgetsOptions { return &gl.ListWidgetsOptions{Search: &f.search} }

// protectOf builds the protection out of the intermediate.
func protectOf(f filter) *gl.ProtectOptions { return &gl.ProtectOptions{Name: &f.search} }

type LoopInput struct {
	Search string ~json:"search"~
}

// Loop fills the intermediate at the end of a loop whose start copies it,
// so the copy holds the input only on the second pass over the body.
func Loop(c *gl.Client, in LoopInput) error {
	var first, later filter
	for range 2 {
		later = first
		first = filter{search: in.Search}
	}
	_, err := c.Widgets.ListWidgets(optionsOf(later))
	return err
}

type DiscardInput struct {
	Search string ~json:"search"~
}

// Discard assigns an intermediate filled from its input to the blank
// identifier and builds the protection from one filled with a constant.
func Discard(c *gl.Client, in DiscardInput) error {
	_ = filter{search: in.Search}
	return c.Widgets.Protect(1, protectOf(filter{search: "x"}))
}

// drop hands back an intermediate of constants, whatever it is handed.
func drop(filter) filter { return filter{search: "x"} }

type SingleInput struct {
	Search string ~json:"search"~
}

// Single takes one result from a function that drops what it is handed.
func Single(c *gl.Client, in SingleInput) error {
	kept := drop(filter{search: in.Search})
	return c.Widgets.Protect(1, protectOf(kept))
}

// pair hands back its receiver first and an intermediate of constants
// second.
func (f filter) pair() (filter, filter) { return f, filter{search: "x"} }

type MethodInput struct {
	Search string ~json:"search"~
}

// Method takes two results from a method called on an intermediate filled
// from its input: only the first holds the input.
func Method(c *gl.Client, in MethodInput) error {
	held := filter{search: in.Search}
	first, second := held.pair()
	if _, err := c.Widgets.ListWidgets(optionsOf(first)); err != nil {
		return err
	}
	return c.Widgets.Protect(1, protectOf(second))
}

// valid reads the intermediate it is handed.
func valid(f filter) bool { return f.search != "" }

// checked hands the intermediate it is handed to another function and calls
// the function its caller passes with constants alone.
func checked(f filter, protect func(filter) error) error {
	if !valid(f) {
		return nil
	}
	return protect(filter{search: "w"})
}

type CheckedInput struct {
	Search string ~json:"search"~
}

// Checked hands its closure to a function that calls it with constants
// alone, after handing the intermediate to another function.
func Checked(c *gl.Client, in CheckedInput) error {
	return checked(filter{search: in.Search}, func(f filter) error { return c.Widgets.Protect(1, protectOf(f)) })
}

// apply calls the closure it is handed with the search of the intermediate
// it is handed and an intermediate of constants.
func apply(f filter, use func(string, filter) error) error {
	return use(f.search, filter{search: "z"})
}

type NarrowInput struct {
	Search string ~json:"search"~
}

// Narrow hands apply a closure whose first parameter, which apply hands the
// search, is no intermediate, and whose second has no name: neither holds
// the input, so the literal the closure fills from the first carries
// nothing.
func Narrow(c *gl.Client, in NarrowInput) error {
	return apply(filter{search: in.Search}, func(s string, _ filter) error { return c.Widgets.Protect(1, protectOf(filter{search: s})) })
}

// wrapper holds an intermediate in a field; no handler fills one in a
// literal.
type wrapper struct {
	f filter
}

type WrappedInput struct {
	Search string ~json:"search"~
}

// Wrapped fills the field of a local from its input and hands the field to
// the helper.
func Wrapped(c *gl.Client, in WrappedInput) error {
	var w wrapper
	w.f = filter{search: in.Search}
	_, err := c.Widgets.ListWidgets(optionsOf(w.f))
	return err
}
`

// TestCollectCallerInputs_WhatOnlyALaterPassOrAPlaceTellsApart_IsPairedAsItHolds
// verifies the pairing through an intermediate where the order or the place
// it is held in decides: a copy taken at a loop's start holds the input once
// the loop's end fills what it copies, which takes another pass over the
// body; a method called on a held intermediate holds it in its receiver, so
// only the result it hands the receiver back in carries it; a field of a
// local carries it although the local's own type is filled in no literal;
// and a single target holds whatever its value reads, the one result of a
// function that drops what it is handed included. An intermediate assigned
// to the blank identifier is held by nothing, so a literal of constants
// beside it carries nothing; a closure a function calls with constants
// alone holds nothing, whatever else the function hands its intermediate to;
// and a closure's parameter that is no intermediate, or that has no name,
// holds nothing even where the function hands it something of the input.
func TestCollectCallerInputs_WhatOnlyALaterPassOrAPlaceTellsApart_IsPairedAsItHolds(t *testing.T) {
	pkgs := checkedTree(t, pairingSDK, toolSource{path: "example.com/x/internal/tools/passes", source: passesSource})

	got := callerPairs(pkgs[0])

	want := []string{
		"LoopInput v3.ListWidgetsOptions caller",
		"MethodInput v3.ListWidgetsOptions caller",
		"SingleInput v3.ProtectOptions caller",
		"WrappedInput v3.ListWidgetsOptions caller",
	}
	if !slices.Equal(got, want) {
		t.Errorf("collectCallerInputs = %v, want %v", got, want)
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
		for _, b := range structs {
			built = append(built, key[strings.LastIndex(key, ".")+1:]+" "+b.named.Obj().Name())
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

// handBacksSource is a tool package of helpers that hand back what they
// build in each shape a return can take, and one that writes an entry into
// one of several results another helper hands back.
const handBacksSource = `package backs

import gl "` + fixtureSDKPath + `"

// filter is the struct the builders take.
type filter struct {
	search string
}

// guard is the struct a builder writes into what another hands back.
type guard struct {
	name string
}

// optionsOf builds the options out of the filter, in its one result.
func optionsOf(f filter) *gl.ListWidgetsOptions { return &gl.ListWidgetsOptions{Search: &f.search} }

// protectOf builds the protection out of the guard, in its one result.
func protectOf(g guard) *gl.ProtectOptions { return &gl.ProtectOptions{Name: &g.name} }

// pair hands back the options a builder builds first, and a protection it
// builds in place second.
func pair(f filter) (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	return optionsOf(f), &gl.ProtectOptions{}
}

// swapped takes both results of pair and hands them back the other way
// round.
func swapped(f filter) (*gl.ProtectOptions, *gl.ListWidgetsOptions) {
	list, protect := pair(f)
	return protect, list
}

// relayed hands back the results of pair whole.
func relayed(f filter) (*gl.ListWidgetsOptions, *gl.ProtectOptions) { return pair(f) }

// both assigns each of two targets a value of its own, the second a call,
// and hands them back in order.
func both(g guard) (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	list, protect := &gl.ListWidgetsOptions{}, protectOf(g)
	return list, protect
}

// named builds the options into its first named result and hands both back
// by a bare return.
func named(f filter) (list *gl.ListWidgetsOptions, err error) {
	list = optionsOf(f)
	return
}

// kept hands back the options it is handed, and builds none.
func kept(opts *gl.ListWidgetsOptions) (*gl.ListWidgetsOptions, error) { return opts, nil }

// viaKept hands back the results of kept, handed the options it builds.
func viaKept() (*gl.ListWidgetsOptions, error) {
	opts := &gl.ListWidgetsOptions{}
	return kept(opts)
}

// viaValue hands back the results of the function its caller passes,
// handed the options and the protection it builds in place.
func viaValue(split func(*gl.ListWidgetsOptions, *gl.ProtectOptions) (*gl.ListWidgetsOptions, error)) (*gl.ListWidgetsOptions, error) {
	return split(&gl.ListWidgetsOptions{}, &gl.ProtectOptions{})
}

// closed builds the protection in a closure, whose return is the closure's
// own, and hands back what the closure answers second.
func closed() (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	protect := func() *gl.ProtectOptions { return &gl.ProtectOptions{} }
	return &gl.ListWidgetsOptions{}, protect()
}

// declared declares the options before it builds them, and builds a
// protection it hands back nowhere.
func declared() (*gl.ListWidgetsOptions, error) {
	var opts *gl.ListWidgetsOptions
	opts = &gl.ListWidgetsOptions{}
	_ = &gl.ProtectOptions{}
	return opts, nil
}

// defaultOptions builds options of constants.
func defaultOptions() *gl.ListWidgetsOptions { return &gl.ListWidgetsOptions{} }

// defaulted writes the guard's name into the options a builder it hands
// nothing builds, and so builds nothing itself.
func defaulted(g guard) *gl.ListWidgetsOptions {
	opts := defaultOptions()
	opts.Search = &g.name
	return opts
}

// pairNamed writes the guard's name into the first of the two results of
// pair, and hands both back.
func pairNamed(f filter, g guard) (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	list, protect := pair(f)
	list.Search = &g.name
	return list, protect
}

// second hands back the second of the two values it is handed.
func second(_ *gl.ListWidgetsOptions, p *gl.ProtectOptions) *gl.ProtectOptions { return p }

// forwarded hands both results of pair on to second, and hands back what
// second answers.
func forwarded(f filter) *gl.ProtectOptions { return second(pair(f)) }

// forwardedNamed writes the guard's name into what forwarded hands back.
func forwardedNamed(f filter, g guard) *gl.ProtectOptions {
	protect := forwarded(f)
	protect.Name = &g.name
	return protect
}

// fill writes the filter's search into the guard through its pointer
// receiver.
func (g *guard) fill(f filter) { g.name = f.search }

// guarded fills a guard from the filter through that method, and hands it
// back first of two results.
func guarded(f filter) (guard, error) {
	g := guard{}
	g.fill(f)
	return g, nil
}

// filled builds the options out of the guard guarded hands back.
func filled(f filter) *gl.ListWidgetsOptions {
	g, _ := guarded(f)
	return &gl.ListWidgetsOptions{Search: &g.name}
}
`

// lateBacksSource is a tool package of three helpers alone, declared so
// that where the second hands back what it builds is known only on a pass
// that finds nothing else, which the first reads.
const lateBacksSource = `package lates

import gl "` + fixtureSDKPath + `"

// filter is the struct the builders take.
type filter struct {
	search string
}

// guard is the struct the first builder writes into what another hands
// back.
type guard struct {
	name string
}

// pairLate builds options of its own, or writes the guard's name into the
// first result of a builder declared after it that it hands nothing of its
// own. Where that builder hands the options back is known only once the one
// it relays is read, a pass after this body is, and that pass finds nothing
// else.
func pairLate(g guard, fresh bool) *gl.ListWidgetsOptions {
	if fresh {
		return &gl.ListWidgetsOptions{}
	}
	list, _ := pairRelay(filter{search: "x"})
	list.Search = &g.name
	return list
}

// pairRelay hands back the results of pairBuilt whole.
func pairRelay(f filter) (*gl.ListWidgetsOptions, *gl.ProtectOptions) { return pairBuilt(f) }

// pairBuilt builds the options and the protection in place.
func pairBuilt(f filter) (*gl.ListWidgetsOptions, *gl.ProtectOptions) {
	return &gl.ListWidgetsOptions{Search: &f.search}, &gl.ProtectOptions{}
}
`

// TestCollectHelpers_WhatAHelperHandsBack_IsRecordedByTheResultItHoldsIt
// verifies the places among a helper's results that each struct it builds
// is recorded as handed back in. A return of as many values as there are
// results hands it back where a value builds it, through a literal or a
// helper that hands it back in its one result, or reads a variable that
// holds it, which is a target assigned a value of its own or the place a
// call's result takes, or is a call that is handed it, the results of a
// builder forwarded whole to another function included, whatever place the
// builder hands it back in; a return of one call's results, where that
// helper hands it back, or everywhere when the call builds none of it and is
// handed it, or is a function value; a bare return, in each named result
// holding it; and a closure's return in nowhere of the helper's. A struct
// built and handed back nowhere is in no place, and a helper that writes
// into what a builder it hands nothing builds builds nothing. An entry
// written into one of several results of a call reaches only the struct
// handed back in that place, and the entry handed to the call reaches the
// struct the call's own route says it does, and no other; it does so too
// when where the call hands the struct back is known only on a pass that
// finds nothing else, which another pass then reads. An entry written into
// what a forwarding call hands back reaches every struct that call is
// handed, and an entry reaches a literal built out of a variable that one of
// several results of a call holds when the callee fills the result from it
// through a pointer.
func TestCollectHelpers_WhatAHelperHandsBack_IsRecordedByTheResultItHoldsIt(t *testing.T) {
	pkgs := checkedTree(t, pairingSDK,
		toolSource{path: "example.com/x/internal/tools/backs", source: handBacksSource},
		toolSource{path: "example.com/x/internal/tools/lates", source: lateBacksSource})
	builds := collectHelpers(pkgs[0])
	maps.Copy(builds, collectHelpers(pkgs[1]))
	recorded := func(helper string, read func(helperBuild) []int) map[string][]int {
		out := map[string][]int{}
		for name, built := range builds["example.com/x/internal/tools/"+helper] {
			out[name.Name()] = read(built)
		}
		return out
	}
	results := func(built helperBuild) []int { return slices.Sorted(maps.Keys(built.results)) }

	for _, tc := range []struct {
		helper string
		want   map[string][]int
	}{
		{helper: "backs.optionsOf", want: map[string][]int{"ListWidgetsOptions": {0}}},
		{helper: "backs.pair", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {1}}},
		{helper: "backs.swapped", want: map[string][]int{"ListWidgetsOptions": {1}, "ProtectOptions": {0}}},
		{helper: "backs.relayed", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {1}}},
		{helper: "backs.both", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {1}}},
		{helper: "backs.named", want: map[string][]int{"ListWidgetsOptions": {0}}},
		{helper: "backs.viaKept", want: map[string][]int{"ListWidgetsOptions": {0, 1}}},
		{helper: "backs.viaValue", want: map[string][]int{"ListWidgetsOptions": {0, 1}, "ProtectOptions": {0, 1}}},
		{helper: "backs.closed", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {1}}},
		{helper: "backs.declared", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": nil}},
		{helper: "backs.defaulted", want: map[string][]int{}},
		{helper: "backs.pairNamed", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {1}}},
		{helper: "backs.forwarded", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {0}}},
		{helper: "backs.forwardedNamed", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {0}}},
		{helper: "backs.filled", want: map[string][]int{"ListWidgetsOptions": {0}}},
		{helper: "lates.pairLate", want: map[string][]int{"ListWidgetsOptions": {0}}},
		{helper: "lates.pairRelay", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": {1}}},
	} {
		t.Run(tc.helper, func(t *testing.T) {
			if got := recorded(tc.helper, results); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s hands back %v, want %v", tc.helper, got, tc.want)
			}
		})
	}

	entries := func(built helperBuild) []int { return slices.Sorted(maps.Keys(built.through)) }
	for _, tc := range []struct {
		helper string
		want   map[string][]int
	}{
		{helper: "backs.pairNamed", want: map[string][]int{"ListWidgetsOptions": {0, 1}, "ProtectOptions": nil}},
		{helper: "backs.forwardedNamed", want: map[string][]int{"ListWidgetsOptions": {0, 1}, "ProtectOptions": {0, 1}}},
		{helper: "backs.filled", want: map[string][]int{"ListWidgetsOptions": {0}}},
		{helper: "lates.pairRelay", want: map[string][]int{"ListWidgetsOptions": {0}, "ProtectOptions": nil}},
		{helper: "lates.pairLate", want: map[string][]int{"ListWidgetsOptions": {0}}},
	} {
		t.Run("the entries that reach what "+tc.helper+" builds", func(t *testing.T) {
			if got := recorded(tc.helper, entries); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s is reached by %v, want %v", tc.helper, got, tc.want)
			}
		})
	}
}

// TestBoundEntries_ANameTheTypeInformationDoesNotDefine_IsLeftOut verifies
// the variables a helper's receiver and parameters bind are the ones the type
// information defines, each with its entry, an unnamed parameter and a name
// it has no variable for left out without moving the position of the others.
func TestBoundEntries_ANameTheTypeInformationDoesNotDefine_IsLeftOut(t *testing.T) {
	recv, defined, undefined := ast.NewIdent("w"), ast.NewIdent("page"), ast.NewIdent("search")
	recvVar := types.NewParam(0, nil, "w", types.Typ[types.Int])
	pageVar := types.NewParam(0, nil, "page", types.Typ[types.Int])
	fn := &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{recv}}}},
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{}, {Names: []*ast.Ident{undefined, defined}}}}},
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{recv: recvVar, defined: pageVar}}

	got := boundEntries(info, fn)

	if want := map[*types.Var]int{recvVar: receiverEntry, pageVar: 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("boundEntries = %v, want %v", got, want)
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

// lookupsSource is a tool package of functions that are no handler, each
// taking an input a model sends and building options that read nothing of
// it, beside helpers of the package that send options of the same type: the
// shapes that tell a literal such a function sends, a lookup of its own,
// from one it builds for whoever calls it.
const lookupsSource = `package lookups

import gl "` + fixtureSDKPath + `"

// listWith sends the options it is handed.
func listWith(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	_, err := c.Widgets.ListWidgets(opts)
	return err
}

// warm lists with options of its own, and is handed none.
func warm(c *gl.Client) error {
	search := "warm"
	return listWith(c, &gl.ListWidgetsOptions{Search: &search})
}

// inspect reads the options it is handed and sends nothing.
func inspect(opts *gl.ListWidgetsOptions) bool { return opts.Search != nil }

// relay hands the options it is handed to itself, and then to listWith.
func relay(c *gl.Client, opts *gl.ListWidgetsOptions, n int) error {
	if n > 0 {
		return relay(c, opts, max(n-1, 0))
	}
	return listWith(c, opts)
}

// spin hands the options it is handed to itself alone.
func spin(opts *gl.ListWidgetsOptions, n int) bool { return n > 0 && spin(opts, n-1) }

// lister holds the client a list goes out through.
type lister struct {
	c *gl.Client
}

// blind gives the options it is handed no name and lists with its own.
func (l lister) blind(*gl.ListWidgetsOptions) error {
	return listWith(l.c, &gl.ListWidgetsOptions{})
}

// blindValue gives the options it is handed no name, and hands on beside
// options of its own the service method that sends them, as a value.
func (l lister) blindValue(*gl.ListWidgetsOptions) error {
	return through(l.c.Widgets.ListWidgets, &gl.ListWidgetsOptions{})
}

// blankValue is blindValue with the options it is handed named by the blank
// identifier.
func (l lister) blankValue(_ *gl.ListWidgetsOptions) error {
	return through(l.c.Widgets.ListWidgets, &gl.ListWidgetsOptions{})
}

// query embeds the options beside the client that sends them.
type query struct {
	gl.ListWidgetsOptions
	c *gl.Client
}

// send hands the options the query embeds on to listWith.
func (q *query) send() error { return listWith(q.c, &q.ListWidgetsOptions) }

// holder carries the options in a field.
type holder struct {
	opts *gl.ListWidgetsOptions
}

// listHeld sends the options the holder carries.
func listHeld(c *gl.Client, h holder) error { return listWith(c, h.opts) }

// own is handed options, and lists with options of its own.
func own(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	_, err := c.Widgets.ListWidgets(&gl.ListWidgetsOptions{})
	return err
}

// protectWith hands what the options it is handed search for to a service
// method that takes another type.
func protectWith(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	return c.Widgets.Protect(opts.Search, nil)
}

// listCopy sends a copy of the options it is handed.
func listCopy(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	next := *opts
	_, err := c.Widgets.ListWidgets(&next)
	return err
}

// listFunc is the shape of a service method that lists widgets.
type listFunc func(*gl.ListWidgetsOptions, ...gl.RequestOptionFunc) ([]*gl.Widget, error)

// through calls the list it is handed with the options it is handed.
func through(list listFunc, opts *gl.ListWidgetsOptions) error {
	_, err := list(opts)
	return err
}

// listValue hands the options it is handed on beside the service method that
// sends them, as a value.
func listValue(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	return through(c.Widgets.ListWidgets, opts)
}

type DetachedInput struct {
	Search string ~json:"search"~
}

// detached lists with options of its own through a helper, and builds
// others its caller sends.
func detached(c *gl.Client, in DetachedInput) *gl.ListWidgetsOptions {
	search := "own"
	own := &gl.ListWidgetsOptions{Search: &search}
	if listWith(c, own) != nil {
		return nil
	}
	return &gl.ListWidgetsOptions{}
}

type InspectedInput struct {
	Search string ~json:"search"~
}

// inspected hands the options it builds to a helper that sends nothing, and
// warms beside them.
func inspected(c *gl.Client, in InspectedInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	if !inspect(opts) && warm(c) != nil {
		return nil
	}
	return opts
}

type SpunInput struct {
	Search string ~json:"search"~
}

// spun hands the options it builds to a helper that hands them only to
// itself, and warms beside them.
func spun(c *gl.Client, in SpunInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	if spin(opts, 2) || warm(c) != nil {
		return nil
	}
	return opts
}

type BlindInput struct {
	Search string ~json:"search"~
}

// blinded hands the options it builds to a helper that drops them, and
// drops what the helper answers.
func blinded(c *gl.Client, in BlindInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	_ = (lister{c: c}).blind(opts)
	return opts
}

type BlindValueInput struct {
	Search string ~json:"search"~
}

// blindValued hands the options it builds to a helper that drops them
// beside a method value of the service.
func blindValued(c *gl.Client, in BlindValueInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	_ = (lister{c: c}).blindValue(opts)
	return opts
}

type BlankValueInput struct {
	Search string ~json:"search"~
}

// blankValued hands the options it builds to a helper that names them by
// the blank identifier and drops them beside a method value of the service.
func blankValued(c *gl.Client, in BlankValueInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	_ = (lister{c: c}).blankValue(opts)
	return opts
}

type InPlaceInput struct {
	Search string ~json:"search"~
}

// inPlace hands the options it builds to listWith in place.
func inPlace(c *gl.Client, in InPlaceInput) error {
	return listWith(c, &gl.ListWidgetsOptions{})
}

type RelayedInput struct {
	Search string ~json:"search"~
}

// relayed hands the options it builds, through a variable declared before
// it is assigned them, to a helper that sends them through itself and
// listWith, and then, beside it, to one that reads them.
func relayed(c *gl.Client, in RelayedInput) bool {
	var opts *gl.ListWidgetsOptions
	opts = &gl.ListWidgetsOptions{}
	return relay(c, opts, 2) != nil || inspect(opts)
}

type QueriedInput struct {
	Search string ~json:"search"~
}

// queried calls a method on the query it builds, which hands what its
// receiver embeds on to listWith.
func queried(c *gl.Client, in QueriedInput) error {
	q := &query{c: c}
	return q.send()
}

type HeldInput struct {
	Search string ~json:"search"~
}

// held builds the options inside a holder it hands to a helper that sends
// them.
func held(c *gl.Client, in HeldInput) error {
	h := holder{opts: &gl.ListWidgetsOptions{}}
	return listHeld(c, h)
}

type OwnInput struct {
	Search string ~json:"search"~
}

// owned hands the options it builds to a helper that lists with options of
// its own.
func owned(c *gl.Client, in OwnInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	if own(c, opts) != nil {
		return nil
	}
	return opts
}

type ProtectedInput struct {
	Search string ~json:"search"~
}

// protected hands the options it builds to a helper that hands a field of
// them to a service method of another type.
func protected(c *gl.Client, in ProtectedInput) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{}
	if protectWith(c, opts) != nil {
		return nil
	}
	return opts
}

type CopiedInput struct {
	Search string ~json:"search"~
}

// copied hands the options it builds to a helper that sends a copy of them.
func copied(c *gl.Client, in CopiedInput) error {
	return listCopy(c, &gl.ListWidgetsOptions{})
}

type ValuedInput struct {
	Search string ~json:"search"~
}

// valued hands the options it builds to a helper that hands them on beside
// the service method that sends them.
func valued(c *gl.Client, in ValuedInput) error {
	return listValue(c, &gl.ListWidgetsOptions{})
}

type DirectInput struct {
	Search string ~json:"search"~
}

// direct lists with options of its own and builds others it sends nowhere.
func direct(c *gl.Client, in DirectInput) *gl.ListWidgetsOptions {
	search := "direct"
	if _, err := c.Widgets.ListWidgets(&gl.ListWidgetsOptions{Search: &search}); err != nil {
		return nil
	}
	return &gl.ListWidgetsOptions{}
}

// guard is what a lookup fills from its input before it builds the options
// out of it.
type guard struct {
	name string
}

// set writes the name into the guard through its pointer receiver.
func (g *guard) set(name string) { g.name = name }

type FilledInput struct {
	Search string ~json:"search"~
}

// guardOf fills a guard from the input through that method, and hands it
// back first of two results.
func guardOf(in FilledInput) (guard, error) {
	g := guard{}
	g.set(in.Search)
	return g, nil
}

// filled builds the options out of the guard guardOf hands back, and hands
// them to a helper that sends them.
func filled(c *gl.Client, in FilledInput) *gl.ListWidgetsOptions {
	g, _ := guardOf(in)
	opts := &gl.ListWidgetsOptions{Search: &g.name}
	_ = listWith(c, opts)
	return opts
}
`

// TestSentThrough_ALiteralOfAFunctionThatIsNoHandler_IsSentOnlyWhereItIsHandedOn
// verifies when a literal that reads nothing of the input of a function
// that is no handler is a lookup of the function's own, and so paired with
// nothing. It is when the function references a service method that takes
// the literal's type itself, as direct does whatever it sends, and when it
// hands the literal on to a helper of its package that sends it: in place,
// through a variable, inside another value, as the receiver of a method, as
// a copy the helper makes, beside a method value of the service that the
// helper hands on with it, and however many calls down, a helper that hands
// it to itself first included, whatever the function hands it to
// afterwards. A helper that sends options of the same type it is never
// handed is no send of the literal, whether the helper builds them or the
// function hands it other options of its own, nor is one it is handed to
// that reads it, that hands it only to itself, that gives it no name or
// names it by the blank identifier (even beside a method value of the
// service that sends its type, which can be called with nothing the helper
// was handed there), that lists with options of its own beside it or that
// hands a field of it to a service method of another type, so each of those
// builders pairs its literal with its input, as one that sends nothing does.
// A literal that reads the input pairs with it wherever it is sent, one that
// reads it through the first of two results a function of the package fills
// from it through a pointer included.
func TestSentThrough_ALiteralOfAFunctionThatIsNoHandler_IsSentOnlyWhereItIsHandedOn(t *testing.T) {
	pkg := checkedTree(t, pairingSDK, toolSource{path: "example.com/x/internal/tools/lookups", source: lookupsSource})[0]

	inputs, _ := discoverPairs(pkg)

	cases := []struct {
		name   string
		input  string
		paired bool
	}{
		{name: "beside other options of its own a helper sends", input: "DetachedInput", paired: true},
		{name: "handed to a helper that reads it", input: "InspectedInput", paired: true},
		{name: "handed to a helper that hands it only to itself", input: "SpunInput", paired: true},
		{name: "handed to a helper that gives it no name", input: "BlindInput", paired: true},
		{name: "handed to a helper that gives it no name beside a method value that sends", input: "BlindValueInput", paired: true},
		{name: "handed to a helper that names it blank beside a method value that sends", input: "BlankValueInput", paired: true},
		{name: "handed to a helper that lists with options of its own", input: "OwnInput", paired: true},
		{name: "handed to a helper that hands a field of it to a method of another type", input: "ProtectedInput", paired: true},
		{name: "reading the input through a result filled from it through a pointer, handed to a helper that sends it", input: "FilledInput", paired: true},
		{name: "handed in place to a helper that sends it", input: "InPlaceInput"},
		{name: "handed to a helper that sends a copy of it", input: "CopiedInput"},
		{name: "handed to a helper that hands it on beside a method value that sends it", input: "ValuedInput"},
		{name: "handed through a variable, two calls down", input: "RelayedInput"},
		{name: "the receiver of a method that hands it on", input: "QueriedInput"},
		{name: "inside a value handed to a helper that sends it", input: "HeldInput"},
		{name: "built by a function that sends its type itself", input: "DirectInput"},
	}
	paired := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pair, found := inputs[[2]string{tc.input, "ListWidgetsOptions"}]
			if found != tc.paired {
				t.Errorf("%s paired with ListWidgetsOptions = %t, want %t", tc.input, found, tc.paired)
			}
			if found && pair.via != "" {
				t.Errorf("%s is paired via %q, want the function's own literal", tc.input, pair.via)
			}
		})
		if tc.paired {
			paired++
		}
	}
	if len(inputs) != paired {
		t.Errorf("discoverPairs = %v, want only the %d pairs the cases name", slices.Collect(maps.Keys(inputs)), paired)
	}
}

// TestHelperValue_ANameThatIsNotAFunction_BuildsNothing verifies a name the
// type information resolves to something other than a function, such as the
// package-level variable holding a helper that helperDataSource declares,
// builds nothing as a helper named as a value. The intermediates record
// every name beside an intermediate, variables included, and helperValue is
// what keeps the functions among them.
func TestHelperValue_ANameThatIsNotAFunction_BuildsNothing(t *testing.T) {
	ident := ast.NewIdent("Builder")
	held := types.NewVar(0, nil, "Builder", types.Typ[types.Int])
	info := &types.Info{Uses: map[*ast.Ident]types.Object{ident: held}}

	if built := helperValue(info, ident, helperBuilds{"Builder": nil}, nil); built != nil {
		t.Errorf("helperValue = %v, want nothing for a variable", built)
	}
}

// TestEnter_AReceiverWithNoNameOrNoExpression_HoldsNothing verifies that a
// function read through its body holds nothing in its receiver when the
// receiver has no name to hold it under, or when the call names no
// receiver expression to hand it, however much of the input the caller
// holds. A type-checked package reaches the second only through a method
// value, which always names one, so the case is built by hand.
func TestEnter_AReceiverWithNoNameOrNoExpression_HoldsNothing(t *testing.T) {
	named := ast.NewIdent("w")
	cases := []struct {
		name  string
		names []*ast.Ident
	}{
		{name: "a receiver with no name"},
		{name: "a call with no receiver expression", names: []*ast.Ident{named}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			callee := ast.NewIdent("options")
			fn := types.NewFunc(0, nil, "options", types.NewSignatureType(nil, nil, nil, nil, nil, false))
			decl := &ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{{Names: tc.names}}},
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			}
			receiver := types.NewParam(0, nil, "w", types.Typ[types.Int])
			input := types.NewParam(0, nil, "in", types.Typ[types.Int])
			info := &types.Info{
				Uses: map[*ast.Ident]types.Object{callee: fn},
				Defs: map[*ast.Ident]types.Object{named: receiver},
			}
			held := &intermediates{
				info: info, input: input, from: map[*types.Var]bool{input: true},
				beside: map[*ast.Ident]bool{}, decls: map[*types.Func]*ast.FuncDecl{fn: decl}, reading: map[*types.Func]bool{},
			}

			inner, entered := held.enter(&ast.CallExpr{Fun: callee})

			if inner == nil || entered != decl {
				t.Fatalf("enter = %v, %v, want the declaration read", inner, entered)
			}
			if len(inner.from) != 0 {
				t.Errorf("enter held %v, want nothing", inner.from)
			}
		})
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
