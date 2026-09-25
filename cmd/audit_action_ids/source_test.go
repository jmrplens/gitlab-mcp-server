package main

import (
	"cmp"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// fixtureDir is the directory the in-memory fixture package pretends to live
// in. Nothing is written there: the package exists only in the loader overlay,
// which keeps generated Go source out of the repository while still
// type-checking it against the real toolutil.
const fixtureDir = "cmd/audit_action_ids/fixture"

// fixturePatterns load the fixture together with the package that owns
// HintAction, ActionSpec and ActionSpecOptions, since the walk resolves all of
// them by object identity rather than by the text of a selector.
//
// toolutil is walked as well, which is why every assertion below is scoped to
// the fixture's own package: what toolutil publishes is the repository's
// business and not this test's.
var fixturePatterns = []string{"./" + fixtureDir + "/...", "./internal/toolutil"}

// repoRoot walks up from the test's working directory to the module root, so
// the overlay can name absolute paths inside the module and the loader
// resolves the module's own import paths.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// collectFixture walks one fixture source and returns the sites it produced in
// the fixture's own package.
func collectFixture(t *testing.T, source string) []site {
	t.Helper()
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(source),
	}
	sites, err := collectSites(root, fixturePatterns, overlay)
	if err != nil {
		t.Fatalf("collect sites: %v", err)
	}
	var own []site
	for _, at := range sites {
		if at.Package == fixtureDir {
			own = append(own, at)
		}
	}
	return own
}

// valuesOfKind is every folded value the walk recorded for one kind, sorted so
// a comparison does not depend on the order files were walked in.
func valuesOfKind(sites []site, kind string) []string {
	var values []string
	for _, at := range sites {
		if at.Kind == kind && at.Resolved {
			values = append(values, at.Value)
		}
	}
	sort.Strings(values)
	return values
}

// unresolvedExprs is every expression the walk could not fold, sorted. A value
// the walk passed over is not one of them: it was declined on purpose, and
// [passedOverExprs] lists those.
func unresolvedExprs(sites []site) []string {
	var exprs []string
	for _, at := range sites {
		if !at.Resolved && !at.PassedOver {
			exprs = append(exprs, at.Expr)
		}
	}
	sort.Strings(exprs)
	return exprs
}

// TestCollectSites_EveryPublishingShape_IsFolded drives the walk over one
// fixture carrying each shape a published ID is written in, and asserts the
// folded values.
//
// The shapes are the ones the tree uses, and each is here because a simpler
// reading missed it: constants rather than literals, a concatenated prefix, an
// unexported metadata table copied onto the options, a list handed in as a
// parameter, a list grown in a local, and a one-line helper that builds an ID
// from a domain constant and a name.
func TestCollectSites_EveryPublishingShape_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const (
	domain     = "demo"
	actionGet  = "demo.get"
	actionList = "demo.list"
	nameCreate = "create"
)

func canonicalID(name string) string { return domain + "." + name }

type metaEntry struct {
	usage       string
	related     []string
	description string
}

var table = map[string]metaEntry{
	"demo": {
		usage:       "Use action 'demo.from_usage' to do the thing.",
		related:     []string{actionGet, "demo.from_table"},
		description: "Returns the thing. See also demo.from_description.",
	},
}

func fromLiteral() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{actionList, "demo.from_literal"}}
}

func fromAssignment() toolutil.ActionSpecOptions {
	var options toolutil.ActionSpecOptions
	options.RelatedActions = append([]string(nil), table["demo"].related...)
	options.RelatedActions = []string{canonicalID(nameCreate)}
	return options
}

func fromParameter(related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: related}
}

func callsFromParameter() toolutil.ActionSpecOptions {
	return fromParameter([]string{"demo.from_parameter"})
}

func fromLocal() toolutil.ActionSpecOptions {
	related := []string{"demo.from_local"}
	related = append(related, "demo.from_local_append")
	return toolutil.ActionSpecOptions{RelatedActions: related}
}

func hints() []string {
	return []string{
		toolutil.HintAction(actionGet, "read one"),
		toolutil.HintAction(canonicalID(nameCreate), "make one"),
		toolutil.HintAction(domain+".from_concatenation", "and this"),
	}
}
`)

	relatedWant := []string{
		"demo.create",
		"demo.from_literal",
		"demo.from_local",
		"demo.from_local_append",
		"demo.from_parameter",
		"demo.from_table",
		"demo.get",
		"demo.list",
	}
	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, relatedWant) {
		t.Errorf("related values = %v, want %v", got, relatedWant)
	}

	hintWant := []string{"demo.create", "demo.from_concatenation", "demo.get"}
	if got := valuesOfKind(sites, kindHint); !slices.Equal(got, hintWant) {
		t.Errorf("hint values = %v, want %v", got, hintWant)
	}

	if got := valuesOfKind(sites, kindUsage); len(got) != 1 || !strings.Contains(got[0], "demo.from_usage") {
		t.Errorf("usage values = %v, want the one usage line", got)
	}
	if got := valuesOfKind(sites, kindDescription); len(got) != 1 || !strings.Contains(got[0], "demo.from_description") {
		t.Errorf("description values = %v, want the one description", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectSites_Site_IsStampedWithItsOwnFileAndLine holds the sites of one
// small fixture as whole records. Every other test here reads a site's kind
// and value and filters on its package, so the file and the line, which are
// what a reader opens, were asserted by nothing: a site stamped with the
// column where its line belongs, or with the package where its file belongs,
// would have passed them all.
//
// The fixture also writes a string field this rule does not name, and the
// whole-list assertion is what says no site came of it: a rule reading every
// string field as prose would judge a tag line as an invitation to call what
// it mentions.
func TestCollectSites_Site_IsStampedWithItsOwnFileAndLine(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var Spec = toolutil.ActionSpecOptions{
	RelatedActions: []string{"demo.get"},
	Usage:          "Chain demo.list after this.",
	ContentKind:    "demo.not_prose",
}
`)

	want := []site{
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 6, Kind: kindRelated, Value: "demo.get", Resolved: true},
		{Package: fixtureDir, File: fixtureDir + "/fixture.go", Line: 7, Kind: kindUsage, Value: "Chain demo.list after this.", Resolved: true},
	}
	if !slices.Equal(sites, want) {
		t.Errorf("sites = %+v, want %+v", sites, want)
	}
}

// TestFoldCall_TwoParameters_AreBoundByPosition holds that a helper's
// parameters are bound to the arguments in the order the call passes them.
// Every other folded helper here takes one parameter, so a fold that bound
// the last argument to the first parameter would have produced the same ID.
func TestFoldCall_TwoParameters_AreBoundByPosition(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func join(domain, name string) string { return domain + "." + name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{join("demo", "get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("related values = %v, want demo.get with the domain in front", got)
	}
}

// TestCollectSites_RelatedParameter_IsReadAtItsOwnPosition holds that a
// parameter is followed to the argument at its own index in every call, and
// not to the first argument. The one parameter-following fixture elsewhere
// takes the list as its only parameter, where the two cannot differ.
func TestCollectSites_RelatedParameter_IsReadAtItsOwnPosition(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func build(usage string, related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: usage, RelatedActions: related}
}

func options() toolutil.ActionSpecOptions {
	return build("Plain prose.", []string{"demo.from_second_argument"})
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.from_second_argument"}) {
		t.Errorf("related values = %v, want the list passed as the second argument", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none: the prose argument is not the list", got)
	}
}

// TestCollectSites_FieldsNamedRightAndTypedWrong_AreNotRead holds the half of
// the field rule that reads the type. The names are common, so a related that
// is a list of something else, a usage that is not text and a description
// that is a table belong to some other struct, and reading any of them would
// put whatever they hold in front of a model as an action ID.
func TestCollectSites_FieldsNamedRightAndTypedWrong_AreNotRead(t *testing.T) {
	sites := collectFixture(t, `package fixture

type counters struct {
	related     []int
	usage       int
	description map[string]string
}

var table = counters{related: []int{1, 2}, usage: 3, description: map[string]string{"a": "demo.get"}}
`)

	if len(sites) != 0 {
		t.Errorf("sites = %+v, want none: neither field carries text", sites)
	}
}

// TestCollectSites_ACallerThatPassesNoList_ContributesNothing holds the one
// thing a caller can do that leaves a followed parameter with no argument at
// all: a variadic list nobody filled. A rule that read the argument at the
// parameter's position regardless would be reaching past the end of what the
// call was written with.
func TestCollectSites_ACallerThatPassesNoList_ContributesNothing(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func spec(related ...string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: related}
}

func withNone() toolutil.ActionSpecOptions { return spec() }

func withOne() toolutil.ActionSpecOptions {
	ids := []string{"demo.get"}
	return spec(ids...)
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("related values = %v, want the one call that passed a list", got)
	}
}

// TestCollectSites_AnonymousStructTable_IsRead holds the rule that accepts an
// anonymous struct. internal/tools/integrations keeps its metadata table as a
// map to one, and a rule that asked for a named type read neither its lists
// nor its prose while reporting the copy taken from it as unfoldable.
func TestCollectSites_AnonymousStructTable_IsRead(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

var anonymous = map[string]struct {
	related []string
	usage   string
}{
	"demo": {related: []string{"demo.anonymous"}, usage: "Chain demo.anonymous_usage after this."},
}

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: append([]string(nil), anonymous["demo"].related...)}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.anonymous"}) {
		t.Errorf("related values = %v, want the anonymous table's entry", got)
	}
	if got := valuesOfKind(sites, kindUsage); len(got) != 1 {
		t.Errorf("usage values = %v, want the anonymous table's usage", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none: the copy is a read of a recorded list", got)
	}
}

// TestCollectSites_OnlyTheHintHelper_IsReadAsAHint holds both halves of the
// call rule, which is the package and the name together. A call of another
// toolutil function is not a hint although the package matches, and a local
// function of the same name is not one although the name matches: either
// half alone would read an ordinary string argument as an ID a model is
// invited to call.
func TestCollectSites_OnlyTheHintHelper_IsReadAsAHint(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func HintAction(id, purpose string) string { return id + purpose }

func hints() []string {
	return []string{
		toolutil.HintAction("demo.real_hint", "read one"),
		toolutil.EscapeMdTableCell("demo.not_a_hint"),
		HintAction("demo.local_helper", "not the toolutil one"),
	}
}
`)

	if got := valuesOfKind(sites, kindHint); !slices.Equal(got, []string{"demo.real_hint"}) {
		t.Errorf("hint values = %v, want only the toolutil helper's argument", got)
	}
}

// TestCollectSites_AValueOfNoPackage_IsNotAStructThisModuleWrites holds the
// guard in front of the module test. A universe type such as error is named
// and belongs to no package at all, so asking for its path before asking
// whether it has one is a crash rather than a wrong answer, and the only
// values reaching that question are whatever a package hands a helper.
func TestCollectSites_AValueOfNoPackage_IsNotAStructThisModuleWrites(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func published(reason error) []string { return []string{reason.Error()} }

func options(reason error) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: published(reason)}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing read out of a value of no package", got)
	}
	if got := unresolvedExprs(sites); len(got) == 0 {
		t.Error("a list built from a value of no package was passed over silently")
	}
}

// TestCollectSites_ValuesNoRuleCanFollow_AreReported walks the shapes that
// reach a rule and hand it nothing to read: a call through a function value,
// which names no declared function to follow into, and an identifier declared
// and never given a value. Each lands in the unresolved bucket, which is the
// one thing a reporting audit owes a reader about what it could not see.
//
// A list built by a function of another module is here too, since this walk
// has no body for one: what a package outside the walk returns is a hole in
// the audit rather than a clean answer.
//
// The fixture also declares a function under the blank name, which no call
// can name and which the walk therefore never follows into.
func TestCollectSites_ValuesNoRuleCanFollow_AreReported(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func _(name string) string { return name }

var makeID = func() string { return "demo.from_a_function_value" }

var makeList = func() []string { return []string{"demo.from_a_list_value"} }

var unassigned string

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{makeID(), unassigned}}
}

func more() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: makeList()}
}

func fromAnotherModule() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: strings.Split("demo.a,demo.b", ",")}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a value no rule can follow", got)
	}
	want := []string{"makeID()", "makeList()", `strings.Split("demo.a,demo.b", ",")`, "unassigned"}
	if got := unresolvedExprs(sites); !slices.Equal(got, want) {
		t.Errorf("unresolved = %v, want %v", got, want)
	}
}

// TestCollectSites_AHelperReachedTwice_IsReadOnce holds the guard that keeps
// one helper's literals out of the report twice. Two specs sharing a defaults
// helper is an ordinary shape here, and the same guard is what stops a helper
// that calls itself from running forever.
func TestCollectSites_AHelperReachedTwice_IsReadOnce(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func defaults() []string { return []string{"demo.shared"} }

func first() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: defaults()}
}

func second() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: defaults()}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.shared"}) {
		t.Errorf("related values = %v, want the helper's list read once", got)
	}
}

// TestCollectSites_AMethodValue_IsNotAFieldRead holds the other half of what
// makes a selector a read of an ID list: it has to select a field. A method
// value selects a method, so the call it is handed to is followed like any
// other rather than passed over as a copy of a list recorded elsewhere.
func TestCollectSites_AMethodValue_IsNotAFieldRead(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

type entry struct{ Name string }

func (e entry) Related() []string { return []string{"demo.from_a_method"} }

func published(read func() []string) []string { return read() }

func options(e entry) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: published(e.Related)}
}
`)

	if got := unresolvedExprs(sites); len(got) == 0 {
		t.Error("a list built from a method value was passed over silently")
	}
}

// TestCollectSites_UnfoldableValue_IsReportedRatherThanSkipped holds the one
// property a reporting audit must have: what it could not see is named. A
// value the type checker cannot fold and no rule here can follow has to land
// in the unresolved bucket, or the run reads clean over source nobody read.
func TestCollectSites_UnfoldableValue_IsReportedRatherThanSkipped(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{os.Getenv("DEMO_ACTION")}}
}
`)

	got := unresolvedExprs(sites)
	if len(got) != 1 || !strings.Contains(got[0], "os.Getenv") {
		t.Errorf("unresolved = %v, want the environment read named", got)
	}
	if values := valuesOfKind(sites, kindRelated); len(values) != 0 {
		t.Errorf("related values = %v, want none folded", values)
	}
}

// TestCollectSites_SharedListHelperParameter_IsNotFollowed holds the
// restriction on parameter following. A parameter is followed out to every
// call of its function, so following one that is merely a list of strings
// judges whatever any caller passes: here the alias list handed to a shared
// clone helper would read as a cross-link that resolves to nothing.
//
// The price of the restriction is the unresolved entry this also asserts: a
// list that reaches RelatedActions through a generic helper is named rather
// than guessed at, which is the trade the audit makes everywhere.
func TestCollectSites_SharedListHelperParameter_IsNotFollowed(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func cloneStrings(values []string) []string {
	out := make([]string, 0, len(values))
	out = append(out, values...)
	return out
}

func aliases() []string { return cloneStrings([]string{"list the demo things"}) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: cloneStrings([]string{"demo.get"})}
}
`)

	if slices.Contains(valuesOfKind(sites, kindRelated), "list the demo things") {
		t.Errorf("the shared clone helper leaked an alias into the related rule: %v",
			valuesOfKind(sites, kindRelated))
	}
	if got := unresolvedExprs(sites); len(got) == 0 {
		t.Error("a list reaching RelatedActions through a generic helper was passed over silently")
	}
}

// TestCollectSites_AMergeWithARelatedParameter_IsFollowedToItsCallers holds
// the third shape the pass-through rule accepts: a call handed a recorded list
// and a parameter named as a carrier of related actions, which merges the two
// through a helper whose body folds to nothing. toolutil's
// ActionRoute.WithRelatedActions is the shape. The parameter is followed out
// to the calls that fill it, where the IDs are written, and the helper's body
// is not reported; read the old way, the make and the append inside it were
// two sites nothing folds on every run that loaded toolutil.
//
// The other cases are what keeps the rule to that shape: an argument that is
// not a parameter, even one named like a carrier, a parameter under another
// name, and a name that is no variable at all each leave the call to be
// followed into, which reports the body it cannot fold rather than passing it.
// So does a call handed carriers alone, which merges nothing recorded and is
// where a helper appending an ID of its own writes it, and a carrier that is
// a string rather than a list, whose callers each pass one ID. Passed over,
// the first dropped the helper's ID without a word and the second reported
// every caller's constant as a list nothing folds. The last case pins the
// hole the merge shares with the copy: an ID the merging helper adds of its
// own is not read.
func TestCollectSites_AMergeWithARelatedParameter_IsFollowedToItsCallers(t *testing.T) {
	const merge = `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func normalize(existing []string, values ...string) []string {
	merged := make([]string, 0, len(existing)+len(values))
	merged = append(merged, existing...)
	return append(merged, values...)
}

`
	cases := []struct {
		name       string
		body       string
		related    []string
		unresolved bool
	}{
		{
			name: "a related parameter",
			body: `func withRelated(opts toolutil.ActionSpecOptions, related ...string) toolutil.ActionSpecOptions {
	opts.RelatedActions = normalize(opts.RelatedActions, related...)
	return opts
}

func options() toolutil.ActionSpecOptions {
	return withRelated(toolutil.ActionSpecOptions{RelatedActions: []string{"demo.base"}}, "demo.merged")
}
`,
			related: []string{"demo.base", "demo.merged"},
		},
		{
			name: "a local named like one",
			body: `func options() toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{RelatedActions: []string{"demo.base"}}
	related := []string{"demo.local"}
	opts.RelatedActions = normalize(opts.RelatedActions, related...)
	return opts
}
`,
			related:    []string{"demo.base"},
			unresolved: true,
		},
		{
			name: "a parameter under another name",
			body: `func withValues(opts toolutil.ActionSpecOptions, values ...string) toolutil.ActionSpecOptions {
	opts.RelatedActions = normalize(opts.RelatedActions, values...)
	return opts
}

func options() toolutil.ActionSpecOptions {
	return withValues(toolutil.ActionSpecOptions{RelatedActions: []string{"demo.base"}}, "demo.unread")
}
`,
			related:    []string{"demo.base"},
			unresolved: true,
		},
		{
			name: "a constant",
			body: `const relatedConstant = "demo.constant"

func options() toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{RelatedActions: []string{"demo.base"}}
	opts.RelatedActions = normalize(opts.RelatedActions, relatedConstant)
	return opts
}
`,
			related:    []string{"demo.base"},
			unresolved: true,
		},
		{
			name: "related parameters alone",
			body: `func withCommon(related []string) []string {
	return append(related, "demo.common")
}

func build(related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: withCommon(related)}
}

func options() toolutil.ActionSpecOptions {
	return build([]string{"demo.caller"})
}
`,
			related: []string{"demo.caller", "demo.common"},
		},
		{
			name: "string parameters named like carriers",
			body: `const idA = "demo.first"

func pair(related1, related2 string) []string {
	return []string{related1, related2}
}

func build(relatedFirst, relatedSecond string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: pair(relatedFirst, relatedSecond)}
}

func options() toolutil.ActionSpecOptions {
	return build(idA, "demo.second")
}
`,
			related: []string{"demo.first", "demo.second"},
		},
		{
			name: "a string parameter named like a carrier beside a recorded list",
			body: `func withOne(relatedList []string, related string) []string {
	return append(relatedList, related)
}

func build(related string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{RelatedActions: []string{"demo.base"}}
	opts.RelatedActions = withOne(opts.RelatedActions, related)
	return opts
}

func options() toolutil.ActionSpecOptions {
	return build("demo.one")
}
`,
			related: []string{"demo.base", "demo.one"},
		},
		{
			name: "a merge whose helper adds an ID of its own",
			body: `func withRelatedAndCommon(opts toolutil.ActionSpecOptions, related ...string) toolutil.ActionSpecOptions {
	opts.RelatedActions = normalizeWithCommon(opts.RelatedActions, related...)
	return opts
}

func normalizeWithCommon(existing []string, values ...string) []string {
	return append(normalize(existing, values...), "demo.unread")
}

func options() toolutil.ActionSpecOptions {
	return withRelatedAndCommon(toolutil.ActionSpecOptions{RelatedActions: []string{"demo.base"}}, "demo.merged")
}
`,
			related: []string{"demo.base", "demo.merged"},
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			sites := collectFixture(t, merge+one.body)
			if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, one.related) {
				t.Errorf("related values = %v, want %v", got, one.related)
			}
			if got := unresolvedExprs(sites); (len(got) > 0) != one.unresolved {
				t.Errorf("unresolved = %v, want some: %t", got, one.unresolved)
			}
		})
	}
}

// TestCollectSites_AnIDHeldInALocal_IsFollowedToItsValue holds the one-ID
// half of variable following: an element of a list that names a local rather
// than a constant is followed to the value the local is given. This was
// reached only through toolutil's own normalizing helper until that helper
// stopped being followed into, so it is planted here rather than left to
// whichever package the fixture load happens to walk.
func TestCollectSites_AnIDHeldInALocal_IsFollowedToItsValue(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func options() toolutil.ActionSpecOptions {
	id := "demo.local"
	return toolutil.ActionSpecOptions{RelatedActions: []string{id}}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.local"}) {
		t.Errorf("related values = %v, want the value the local is given", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none: the local folds to its one value", got)
	}
}

// TestCollectSites_AListNarrowedByAProjection_IsPassedOver holds the second
// shape the pass-through rule accepts: a call handed the value an ID list
// hangs off, returning the subset one caller may be shown. The dynamic
// registry narrows its cross-links that way before publishing them, and the
// IDs it narrows are declared in the catalog and judged there.
//
// The negative half is what keeps the rule from being a blanket silence: a
// call handed a value carrying no ID list has had nothing recorded for it, so
// the list it returns lands in the unresolved bucket like any other. Its
// struct carries a list of strings under another name and a related under
// another type, since what makes a field an ID list is the name and the type
// together and either alone is a field of some other kind.
func TestCollectSites_AListNarrowedByAProjection_IsPassedOver(t *testing.T) {
	t.Run("the value carries an ID list", func(t *testing.T) {
		sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

type entry struct {
	ID             string
	RelatedActions []string
}

func published(e entry) []string {
	out := make([]string, 0, len(e.RelatedActions))
	for _, id := range e.RelatedActions {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func options(e entry) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: published(e)}
}
`)

		if got := unresolvedExprs(sites); len(got) != 0 {
			t.Errorf("unresolved = %v, want none: the narrowed list is declared and judged where it was written", got)
		}
	})

	t.Run("the value carries none", func(t *testing.T) {
		sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

type label struct {
	Name    string
	Tags    []string
	related int
}

func published(l label) []string {
	out := make([]string, 0, 1)
	out = append(out, l.Name)
	return out
}

func options(l label) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: published(l)}
}
`)

		if got := unresolvedExprs(sites); len(got) == 0 {
			t.Error("a list built from a value carrying no action IDs was passed over silently")
		}
	})
}

// TestCollectSites_AReadThatIsNoIDList_DoesNotSilenceTheCall holds what makes
// a call a pass-through: it is handed a list this walk records where it is
// written. A field named related that holds something else, and a selector
// that names no field of this module at all, are neither, so the call is
// followed into its body as any other and what it publishes is read.
func TestCollectSites_AReadThatIsNoIDList_DoesNotSilenceTheCall(t *testing.T) {
	for name, source := range map[string]string{
		"a related field that is not a list of strings": `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

type entry struct {
	related map[string]string
}

func published(names map[string]string) []string { return []string{"demo.from_helper"} }

func options(e entry) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: published(e.related)}
}
`,
		"a selector that names no field of this module": `package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func published(names []string) []string { return []string{"demo.from_helper"} }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: published(os.Args)}
}
`,
	} {
		t.Run(name, func(t *testing.T) {
			sites := collectFixture(t, source)
			if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.from_helper"}) {
				t.Errorf("related values = %v, want the helper's own literal read", got)
			}
		})
	}
}

// TestCollectSites_ShapesWithNoIDInThem_AreHandled walks the shapes that
// carry no action ID and must not be mistaken for one: a struct literal
// written positionally, a list taken out of a slice of lists, and a read of a
// field that holds something other than action IDs.
//
// The last is the one worth stating. A copy of an ID list is passed over
// because it was recorded where it was written; a read of a field that is not
// an ID list has been recorded nowhere, so it belongs in the unresolved bucket
// and not in the same silence.
func TestCollectSites_ShapesWithNoIDInThem_AreHandled(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

type metaEntry struct {
	usage   string
	related []string
	aliases []string
}

var positional = metaEntry{"Plain prose with no identifier.", []string{"demo.positional"}, nil}

var lists = [][]string{{"demo.indexed"}}

func fromIndex() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: lists[0]}
}

func fromOtherField() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: positional.aliases}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.positional"}) {
		t.Errorf("related values = %v, want the positional literal's list only", got)
	}
	got := unresolvedExprs(sites)
	if !slices.Contains(got, "lists[0]") {
		t.Errorf("unresolved = %v, want the indexed list named", got)
	}
	if !slices.Contains(got, "positional.aliases") {
		t.Errorf("unresolved = %v, want the read of a non-ID field named", got)
	}
}

// TestCollectSites_HelperWithANonStringArgument_IsNotFolded holds that only a
// string constant binds a parameter. A helper taking a count folds to nothing
// rather than to a rendering of the number.
func TestCollectSites_HelperWithANonStringArgument_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"strconv"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const domain = "demo"

func numbered(name string, index int) string { return domain + "." + name + strconv.Itoa(index) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{numbered("get", 2)}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestCollectSites_NoPatternMatch_IsAnError holds that a pattern matching
// nothing is refused rather than reported as a clean run over no source.
func TestCollectSites_NoPatternMatch_IsAnError(t *testing.T) {
	if _, err := collectSites(repoRoot(t), []string{"./cmd/audit_action_ids/nothing/..."}, nil); err == nil {
		t.Fatal("collectSites over a pattern that matches nothing returned no error")
	}
}

// TestTrimModulePath_ModulePrefix_IsRemoved holds that a package is named the
// way the repository names it.
func TestTrimModulePath_ModulePrefix_IsRemoved(t *testing.T) {
	cases := map[string]string{
		"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues": "internal/tools/issues",
		"github.com/jmrplens/gitlab-mcp-server/v3":                       "",
		"golang.org/x/tools/go/packages":                                 "golang.org/x/tools/go/packages",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := trimModulePath(input); got != want {
				t.Errorf("trimModulePath(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestRelativePath_OutsideTheRoot_KeepsTheWholePath holds that a file the root
// does not contain is named in full rather than as a climb out of the
// repository.
func TestRelativePath_OutsideTheRoot_KeepsTheWholePath(t *testing.T) {
	root := filepath.FromSlash("/repo")
	inside := filepath.Join(root, "internal", "tools", "issues", "action_specs.go")
	if got := relativePath(inside, root); got != "internal/tools/issues/action_specs.go" {
		t.Errorf("relativePath inside the root = %q", got)
	}
	outside := filepath.FromSlash("/elsewhere/other.go")
	if got := relativePath(outside, root); got != "/elsewhere/other.go" {
		t.Errorf("relativePath outside the root = %q", got)
	}
	// A relative path against an absolute root is the one pair filepath.Rel
	// refuses outright rather than answering with a climb, which is the other
	// way into this branch and the one the case above takes. Both are answered
	// with the whole path, rendered with slashes like every other answer: a
	// finding reads the same wherever the audit ran, so the separator is the
	// report's and never the platform's.
	//
	// The root is a real temporary directory rather than the invented one
	// above, because what makes a path absolute is per platform: Windows calls
	// a path absolute only with a volume in front of it.
	absoluteRoot := t.TempDir()
	unrelatable := filepath.FromSlash("some/other.go")
	if got := relativePath(unrelatable, absoluteRoot); got != "some/other.go" {
		t.Errorf("relativePath of a path no root can reach = %q, want %q", got, "some/other.go")
	}
}

// TestIsRelatedParamName_NamedSpellings_AreFollowed holds the naming rule the
// parameter restriction leans on, numbered spellings included.
func TestIsRelatedParamName_NamedSpellings_AreFollowed(t *testing.T) {
	cases := map[string]bool{
		"related":        true,
		"relatedActions": true,
		"related1":       true,
		"RelatedActions": true,
		"values":         false,
		"aliases":        false,
		"tags":           false,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isRelatedParamName(name); got != want {
				t.Errorf("isRelatedParamName(%q) = %t, want %t", name, got, want)
			}
		})
	}
}

// TestReadsAsHintProse_Kinds_AreRead holds which kinds are folded as a
// sentence: the six kinds of served prose and the suite's quotation of it, and
// none of the four that publish an ID.
func TestReadsAsHintProse_Kinds_AreRead(t *testing.T) {
	cases := map[string]bool{
		kindErrorHint:         true,
		kindHintField:         true,
		kindMessage:           true,
		kindNextStep:          true,
		kindParamGuidance:     true,
		kindSchemaDescription: true,
		kindAssertion:         true,
		kindRelated:           false,
		kindHint:              false,
		kindUsage:             false,
		kindDescription:       false,
	}
	for kind, want := range cases {
		t.Run(kind, func(t *testing.T) {
			if got := readsAsHintProse(kind); got != want {
				t.Errorf("readsAsHintProse(%q) = %t, want %t", kind, got, want)
			}
		})
	}
	if isHintKind(kindAssertion) {
		t.Error("isHintKind(assertion) = true, which would put the suite in the hint section and in -fix-hints' reach")
	}
}

// TestFollowableParamName_EachKind_FollowsItsOwnNames holds the parameter
// names each kind of site is followed out to its callers under. An assertion
// takes the hint names and the helper table's names besides, and neither the
// hint sites nor the ID sites take the table's: a served hint parameter named
// contains is not prose by that name.
func TestFollowableParamName_EachKind_FollowsItsOwnNames(t *testing.T) {
	cases := []struct {
		kind, name string
		want       bool
	}{
		{kind: kindAssertion, name: "substrings", want: true},
		{kind: kindAssertion, name: "hint", want: true},
		{kind: kindAssertion, name: "wants", want: false},
		{kind: kindAssertion, name: "related", want: false},
		{kind: kindErrorHint, name: "hint", want: true},
		{kind: kindErrorHint, name: "substrings", want: false},
		{kind: kindErrorHint, name: "missingProjectMsg", want: false},
		{kind: kindHintField, name: "needles", want: false},
		{kind: kindMessage, name: "missingProjectMsg", want: true},
		{kind: kindMessage, name: "emptyMessage", want: true},
		{kind: kindMessage, name: "hint", want: true},
		{kind: kindMessage, name: "valueSource", want: false},
		{kind: kindMessage, name: "substrings", want: false},
		{kind: kindParamGuidance, name: "valueSource", want: true},
		{kind: kindParamGuidance, name: "commonConfusions", want: true},
		{kind: kindParamGuidance, name: "hints", want: true},
		{kind: kindParamGuidance, name: "message", want: false},
		{kind: kindNextStep, name: "hints", want: true},
		{kind: kindNextStep, name: "message", want: false},
		{kind: kindNextStep, name: "valueSource", want: false},
		{kind: kindSchemaDescription, name: "description", want: true},
		{kind: kindSchemaDescription, name: "levelDescription", want: true},
		{kind: kindSchemaDescription, name: "hint", want: true},
		{kind: kindSchemaDescription, name: "usage", want: false},
		{kind: kindSchemaDescription, name: "message", want: false},
		{kind: kindUsage, name: "usage", want: true},
		{kind: kindUsage, name: "leadUsage", want: true},
		{kind: kindUsage, name: "hint", want: false},
		{kind: kindUsage, name: "description", want: false},
		{kind: kindUsage, name: "related", want: false},
		{kind: kindRelated, name: "related", want: true},
		{kind: kindRelated, name: "hint", want: false},
		{kind: kindRelated, name: "contains", want: false},
	}
	for _, one := range cases {
		t.Run(one.kind+" "+one.name, func(t *testing.T) {
			if got := followableParamName(one.kind, one.name); got != one.want {
				t.Errorf("followableParamName(%q, %q) = %t, want %t", one.kind, one.name, got, one.want)
			}
		})
	}
}

// TestCollectSites_EveryErrorHintShape_IsFolded drives the walk over one
// fixture carrying each shape corrective prose is written in, and asserts the
// folded hints.
//
// The shapes are the ones the tree uses. The three error helpers take their
// hint at a different argument each and NotFoundResult takes several; a
// constant, a struct field and a list of them are how nineteen domains reach
// those helpers; a hint handed in as a parameter and one grown in a local are
// the indirections a handler writes; and toolutil.ListHints and
// toolutil.HintAction are toolutil's own hint constructors, the second of
// which is judged as an ID at the same call site and so adds no prose.
func TestCollectSites_EveryErrorHintShape_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const hintConst = "read it back with gitlab_demo_get"

var errDemo = errors.New("demo")

type notFoundOutput struct {
	Resource     string
	Hints        []string
	notFoundHint string
}

var output = notFoundOutput{
	Hints:        []string{"grow with gitlab_demo_grow", toolutil.HintAction("demo.get", "read one")},
	notFoundHint: "field hint naming gitlab_demo_field",
}

func fromHintArgument() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, "list them with gitlab_demo_list")
}

func fromStatusHintArgument() error {
	return toolutil.WrapErrWithStatusHint("demo_get", errDemo, 404, "verify it with gitlab_demo_verify")
}

func fromNotFoundResult() {
	_ = toolutil.NotFoundResult("Demo", "id", "first gitlab_demo_first", "second gitlab_demo_second")
}

func fromConstant() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, hintConst)
}

func fromField(out notFoundOutput) {
	_ = toolutil.NotFoundResult(out.Resource, "id", out.notFoundHint)
}

func fromEscapedField(out notFoundOutput) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, toolutil.EscapeMdTableCell(out.notFoundHint))
}

func fromListHints() notFoundOutput {
	return notFoundOutput{Hints: toolutil.ListHints("listed gitlab_demo_listed")}
}

func fromParameter(hint string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, hint)
}

func callsFromParameter() error {
	return fromParameter("passed gitlab_demo_passed")
}

func fromLocal() {
	hints := []string{"local gitlab_demo_local"}
	hints = append(hints, "appended gitlab_demo_appended")
	_ = toolutil.NotFoundResult("Demo", "id", hints...)
}
`)

	argumentWant := []string{
		"appended gitlab_demo_appended",
		"first gitlab_demo_first",
		"list them with gitlab_demo_list",
		"local gitlab_demo_local",
		"passed gitlab_demo_passed",
		"read it back with gitlab_demo_get",
		"second gitlab_demo_second",
		"verify it with gitlab_demo_verify",
	}
	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, argumentWant) {
		t.Errorf("error hint values = %v, want %v", got, argumentWant)
	}

	fieldWant := []string{
		"field hint naming gitlab_demo_field",
		"grow with gitlab_demo_grow",
		"listed gitlab_demo_listed",
	}
	if got := valuesOfKind(sites, kindHintField); !slices.Equal(got, fieldWant) {
		t.Errorf("hint field values = %v, want %v", got, fieldWant)
	}

	// The hint read off a field at the call site is the one already recorded
	// where it was written, escaped on its way through a formatter or not, so
	// it is passed over rather than counted twice.
	if got := valuesOfKind(sites, kindHint); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("hint IDs = %v, want the HintAction argument judged as an ID", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectSites_HintAssembledAtRunTime_KeepsItsLiteralHalves holds the fold
// that matters most here, since a capability name is spelled in a literal half
// or nowhere: dorametrics writes exactly this shape, and a rule that only read
// what the type checker folds whole would see none of it.
//
// The seam is a space rather than nothing, so two halves cannot be read as one
// token neither of them spells.
func TestCollectSites_HintAssembledAtRunTime_KeepsItsLiteralHalves(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func fromConcatenation(scope string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, "list with gitlab_demo"+scope+"list_after")
}
`)

	folded := valuesOfKind(sites, kindErrorHint)
	if len(folded) != 1 {
		t.Fatalf("error hint values = %v, want the one concatenation", folded)
	}
	for _, want := range []string{"gitlab_demo", "list_after"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(folded[0], want) {
				t.Errorf("folded hint %q left out the literal half %q", folded[0], want)
			}
		})
	}
	if strings.Contains(folded[0], "gitlab_demolist_after") {
		t.Errorf("folded hint %q joined two halves into a token neither spells", folded[0])
	}
}

// TestCollectSites_HintNothingCanFold_IsReported holds this rule's blind spot
// as a row rather than as a silence. A hint built by a helper that branches
// carries no literal the fold can pick, but a rule whose blind spot says
// nothing is one a future site steps into, so the site is named.
func TestCollectSites_HintNothingCanFold_IsReported(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func build(value string) string {
	if value == "" {
		return "nothing"
	}
	return value
}

func unfoldable(value string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, build(value))
}
`)

	want := []string{`build(value)`}
	if got := unresolvedExprs(sites); !slices.Equal(got, want) {
		t.Errorf("unresolved = %v, want %v", got, want)
	}
	if got := valuesOfKind(sites, kindErrorHint); len(got) != 0 {
		t.Errorf("error hint values = %v, want nothing folded", got)
	}
}

// TestCollectSites_FormatSentence_FoldsToItsFormatAndCountsItsValues holds
// the format fold: a sentence built by fmt.Sprintf or fmt.Errorf is read as
// the format it is written as, a constant argument is read with it, a name
// that says it carries a hint is followed, and every other argument is a value
// the sentence reports, counted and not listed.
//
// The format is kept verbatim, verbs and all, because the fixer finds the
// literal to rewrite by its being contained in the folded value; a rendered
// format contains no literal of its own. The verbs are masked where the value
// is judged, which [TestClassify_FormatVerbs_AreMaskedBeforeJudging] holds.
func TestCollectSites_FormatSentence_FoldsToItsFormatAndCountsItsValues(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const listTool = "list with gitlab_demo_list"

var errDemo = errors.New("demo")

func fromSprintf(value string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, fmt.Sprintf("built from %s, then %s", value, listTool))
}

func fromErrorf(id int, err error) error {
	return fmt.Errorf("demo %d: use gitlab_demo_get: %w", id, err)
}

func fromCarrier(hint string, err error) error {
	return fmt.Errorf("demo: %w%s", err, hint)
}

func callsCarrier() error {
	return fromCarrier(" (see gitlab_demo_carrier)", errDemo)
}

func fromFormatNothingFolds(format string) error {
	return fmt.Errorf(format, 1)
}
`)

	if got, want := valuesOfKind(sites, kindErrorHint), []string{"built from %s, then %s list with gitlab_demo_list"}; !slices.Equal(got, want) {
		t.Errorf("error hint values = %v, want the format and the constant it is handed: %v", got, want)
	}
	messages := valuesOfKind(sites, kindMessage)
	for _, want := range []string{"demo %d: use gitlab_demo_get: %w", "demo: %w%s", " (see gitlab_demo_carrier)"} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(messages, want) {
				t.Errorf("message values = %q, want %q", messages, want)
			}
		})
	}
	passed := 0
	for _, at := range sites {
		if at.PassedOver {
			passed++
		}
	}
	// value, id, err and the err handed to fromCarrier's format.
	if passed != 4 {
		t.Errorf("values passed over = %d, want the four arguments that are values", passed)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"fmt.Errorf(format, 1)"}) {
		t.Errorf("unresolved = %v, want only the format nothing folds", got)
	}
}

// TestCollectSites_VariadicHintParameter_IsReadAsItsCallersSpellIt holds the
// one parameter shape that means two different things at the call site.
//
// A caller that spreads a slice passes the whole list, and a caller that
// spells its hints passes one value each. Reading the second as the first is
// what the parameter rule got away with while the only variadic lists it met
// were the spread lists of IDs, and it is not how a hint is written.
func TestCollectSites_VariadicHintParameter_IsReadAsItsCallersSpellIt(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func takesHints(hints ...string) {
	_ = toolutil.NotFoundResult("Demo", "id", hints...)
}

func spellsTheElements() {
	takesHints("first gitlab_demo_first", "second gitlab_demo_second")
}

func spreadsAList() {
	takesHints([]string{"spread gitlab_demo_spread"}...)
}
`)

	want := []string{"first gitlab_demo_first", "second gitlab_demo_second", "spread gitlab_demo_spread"}
	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, want) {
		t.Errorf("error hint values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectSites_HintShapesNothingCanRead_AreReportedOneByOne holds every
// shape the hint rules decline, so that declining is a row a reader can see
// rather than a silence.
//
// Two of them are declined on purpose and produce nothing: a nil list and a
// make, which allocate no prose between them. A helper declared here that
// returns a list is not declined: its returns are followed, which is how a
// formatter's hint builder is read, and this one returns an empty list. The
// rest are values this walk cannot follow, and each is named with the
// expression as it was written, since naming what could not be read is the
// only honest alternative to passing over it.
func TestCollectSites_HintShapesNothingCanRead_AreReportedOneByOne(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

type carrier struct {
	Hints        []string
	Notes        []string
	notFoundHint string
	Label        string
}

type odd struct {
	hint int
}

var table = map[string][]string{"a": {"table gitlab_demo_table"}}

var (
	source    = carrier{Hints: []string{"source gitlab_demo_source"}}
	copied    = carrier{Hints: source.Hints}
	empty     = carrier{Hints: nil}
	allocated = carrier{Hints: make([]string, 0, 2)}
	indexed   = carrier{Hints: table["a"]}
	called    = carrier{Hints: build()}
	split     = carrier{Hints: strings.Fields("split gitlab_demo_split")}
	noted     = carrier{Hints: other.Notes}
	named     = carrier{notFoundHint: other.Label}
	other     = carrier{Label: "label gitlab_demo_label"}
)

func build() []string {
	out := []string{}
	return out
}

func describe(value string) string {
	trimmed := value
	return trimmed
}

func render(value int) string {
	shown := value
	return string(rune(shown))
}

func fromPlainListParameter(values []string) carrier {
	return carrier{Hints: values}
}

func fromUnfoldableConcatenation(left, right string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, left+right)
}

func fromQualifiedConstant() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, describe(toolutil.HintPreserveLinks))
}

func fromFieldOfAnotherType(o odd) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, render(o.hint))
}
`)

	// The copy of a hint list, the nil and the make publish nothing: the first
	// is recorded where it was written and the other two hold no prose.
	if got := valuesOfKind(sites, kindHintField); !slices.Equal(got, []string{"source gitlab_demo_source"}) {
		t.Errorf("hint field values = %v, want the one list written as a literal", got)
	}
	want := []string{
		"describe(toolutil.HintPreserveLinks)",
		"left + right",
		"other.Label",
		"other.Notes",
		"render(o.hint)",
		`strings.Fields("split gitlab_demo_split")`,
		`table["a"]`,
		"values",
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, want) {
		t.Errorf("unresolved = %v, want %v", got, want)
	}
}

// TestIsHintName_NamedSpellings_AreRead holds the naming rule the hint field
// and parameter rules lean on. A suffix rather than a substring, or every name
// that merely mentions hints would be read as carrying one.
func TestIsHintName_NamedSpellings_AreRead(t *testing.T) {
	cases := map[string]bool{
		"hint":           true,
		"hints":          true,
		"Hints":          true,
		"notFoundHint":   true,
		"badRequestHint": true,
		"NextSteps":      true,
		"NextStep":       true,
		"hintAction":     false,
		"usage":          false,
		"related":        false,
		"nextStepCount":  false,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isHintName(name); got != want {
				t.Errorf("isHintName(%q) = %t, want %t", name, got, want)
			}
		})
	}
}

// TestProseFieldKind_EachName_IsRecordedUnderItsOwnKind holds the field rule's
// three carriers and the order they are asked in: the hint name first, since a
// field named for a hint is server prose wherever it is written, and a message
// last, since a message field is GitLab's text as often as the server's.
func TestProseFieldKind_EachName_IsRecordedUnderItsOwnKind(t *testing.T) {
	cases := map[string]string{
		"Hint":              kindHintField,
		"NextSteps":         kindHintField,
		"messageHint":       kindHintField,
		"ValueSource":       kindParamGuidance,
		"CommonConfusions":  kindParamGuidance,
		"Message":           kindMessage,
		"missingProjectMsg": kindMessage,
		"EmptyMessage":      kindMessage,
		"SemanticRole":      "",
		"ExampleBinding":    "",
		"Messages":          "",
		"Label":             "",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got, carries := proseFieldKind(name)
			if got != want || carries != (want != "") {
				t.Errorf("proseFieldKind(%q) = %q, %t; want %q", name, got, carries, want)
			}
		})
	}
}

// TestCollectSites_APairHandedAsTheWholeArgumentList_RecordsWhatWasWritten
// holds the one shape where a parameter has no argument of its own in a call
// that compiles: Go lets a call of a multi-valued function stand for the whole
// argument list, so a two-parameter helper can be called with one expression.
//
// The first parameter is then recorded as that call, which folds to nothing
// and is named; the second has no argument to read at all. Reading it anyway
// would index past the list, which is a crash rather than a wrong answer, and
// the shape is rare enough in a tree of hint helpers that nothing else here
// would have produced one.
func TestCollectSites_APairHandedAsTheWholeArgumentList_RecordsWhatWasWritten(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func pairOfHints() (string, string) {
	return "left gitlab_demo_left", "right gitlab_demo_right"
}

func takesTwoHints(firstHint, secondHint string) {
	_ = toolutil.WrapErrWithHint("demo_get", errDemo, firstHint)
	_ = toolutil.WrapErrWithHint("demo_get", errDemo, secondHint)
}

func spellsBothHints() {
	takesTwoHints("first gitlab_demo_first", "second gitlab_demo_second")
}

func handsThePairAsTheWholeList() {
	takesTwoHints(pairOfHints())
}
`)

	want := []string{"first gitlab_demo_first", "second gitlab_demo_second"}
	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, want) {
		t.Errorf("error hint values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"pairOfHints()"}) {
		t.Errorf("unresolved = %v, want the pair named once", got)
	}
}

// TestCollectSites_HintCallsThatAreNoToolutilHelper_AreReadAsProse holds the
// three answers the toolutil test gives for a call standing where a hint is
// expected, and it exists because two of them are reached by no other fixture
// here.
//
// A conversion resolves to a type rather than to a function, so there is no
// callee to ask a package of; a method of the universe scope resolves to a
// function whose package is nil. Both sit in front of the path comparison and
// each is the only thing standing between it and a nil dereference. The third
// is an ordinary function of this package, which resolves whole and is simply
// not the helper being looked for, so the fold gets it and reads the prose it
// returns.
func TestCollectSites_HintCallsThatAreNoToolutilHelper_AreReadAsProse(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func spell(name string) string { return name }

func fromAConversion(raw []byte) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, string(raw))
}

func fromAMethodOfTheUniverse() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, errDemo.Error())
}

func fromAFunctionOfThisPackage() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, spell("gitlab_demo_list"))
}
`)

	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, []string{"gitlab_demo_list"}) {
		t.Errorf("error hint values = %v, want the one call the fold reaches", got)
	}
	want := []string{`errDemo.Error()`, `string(raw)`}
	if got := unresolvedExprs(sites); !slices.Equal(got, want) {
		t.Errorf("unresolved = %v, want %v", got, want)
	}
}

// TestCollectSites_HintListGrownByAppend_ReadsASpreadAsAListAndTheRestAsProse
// holds the two ways an append hands hints on. The list being grown is the
// first argument and is followed as a list; an element spelled in place is one
// hint; and a spread is a list again, which is the half that differs, since
// reading it as one hint would record a slice where prose belongs and lose
// every line in it.
func TestCollectSites_HintListGrownByAppend_ReadsASpreadAsAListAndTheRestAsProse(t *testing.T) {
	sites := collectFixture(t, `package fixture

type carrier struct {
	Hints []string
}

var (
	baseOfTheGrown  = []string{"base gitlab_demo_base"}
	baseOfTheSpread = []string{"other gitlab_demo_other"}
	extra           = []string{"extra gitlab_demo_extra"}

	grown  = carrier{Hints: append(baseOfTheGrown, "grown gitlab_demo_grown")}
	spread = carrier{Hints: append(baseOfTheSpread, extra...)}
)
`)

	want := []string{
		"base gitlab_demo_base",
		"extra gitlab_demo_extra",
		"grown gitlab_demo_grown",
		"other gitlab_demo_other",
	}
	if got := valuesOfKind(sites, kindHintField); !slices.Equal(got, want) {
		t.Errorf("hint field values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestIndexFuncs_DeclarationsTheWalkCannotRead_AreSkipped holds the three
// shapes the function index passes over, asked of it directly.
//
// It is called directly because two of them are states a loaded package cannot
// be in: a function with no body does not type-check without assembly beside
// it, and a function the checker never defined cannot come out of a package
// the loader accepted. The walk is given a file the parser produced and a
// package whose definitions are empty, which is exactly those two states and
// nothing else.
func TestIndexFuncs_DeclarationsTheWalkCannotRead_AreSkipped(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package fixture

var notAFunctionAtAll = 1

func bodyless()

func withABody() {}
`, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	prog := indexProgram("", nil)
	prog.indexFuncs(&packages.Package{TypesInfo: &types.Info{Defs: map[*ast.Ident]types.Object{}}}, file)

	if len(prog.decls) != 0 {
		t.Errorf("declarations indexed = %d, want none of the three", len(prog.decls))
	}
	if len(prog.params) != 0 {
		t.Errorf("parameters indexed = %d, want none", len(prog.params))
	}
}

// TestStructType_NoTypeAtAll_IsNoStruct holds the guard in front of the type
// switch. An expression the checker recorded no type for yields a nil here,
// and reading a nil type is a crash rather than an answer.
func TestStructType_NoTypeAtAll_IsNoStruct(t *testing.T) {
	if _, ok := (&walker{}).structType(nil); ok {
		t.Error("structType(nil) resolved a struct, want none")
	}
}

// TestFollowValues_AnIdentifierThatNamesNoVariable_IsNotFollowed holds the
// first question following a value asks. Every identifier the walk reaches in
// a loaded package names a variable, a constant the fold already read, or a
// name the checker refused, so the miss is reachable only by handing the
// method an identifier from nowhere.
func TestFollowValues_AnIdentifierThatNamesNoVariable_IsNotFollowed(t *testing.T) {
	w := &walker{pkg: &packages.Package{TypesInfo: &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
	}}}
	if w.followValues(kindErrorHint, ast.NewIdent("nothing"), recordHintValue) {
		t.Error("followValues followed an identifier that names no variable")
	}
}

// TestCollectSites_AHintReadOffAParameterNothingNames_IsReportedAndItsHalvesKept
// holds the two answers a hint assembled from a parameter gets.
//
// A parameter whose name says nothing about hints is not followed out to its
// callers, on purpose: following every string parameter judges whatever any
// caller ever passes. So a hint that is nothing but such a parameter is named
// as unread rather than guessed at, and one that concatenates a literal onto
// it keeps the literal, which is where a capability would be spelled, and
// names the parameter as well: the value it is handed is text the model reads
// beside the literal, and a run that counted only the literal would report the
// sentence read whole.
func TestCollectSites_AHintReadOffAParameterNothingNames_IsReportedAndItsHalvesKept(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func fromAPlainParameter(text string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, text)
}

func fromHalfAParameter(text string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, text+" then run demo.list")
}
`)

	if got := valuesOfKind(sites, kindErrorHint); len(got) != 1 || !strings.Contains(got[0], "then run demo.list") {
		t.Errorf("error hint values = %q, want the literal half kept", got)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"text", "text"}) {
		t.Errorf("unresolved = %v, want the bare parameter and the unfolded half each named once", got)
	}
}

// TestCollectSites_AHalfNamedAsAHint_IsFollowedToWhatItIsHanded holds the
// half of a concatenation a name can answer for: it is read the way a whole
// hint in that name would be, out to the values it is handed, so a tool name a
// caller passes is judged where it is written. awardemoji handed its note
// deletes the list tool's name through exactly this shape, and the fold kept
// the sentence around it and dropped the name.
func TestCollectSites_AHalfNamedAsAHint_IsFollowedToWhatItIsHanded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func deleteThrough(listHint string) error {
	return toolutil.WrapErrWithHint("demo_delete", errDemo, "list them with "+listHint+" first")
}

func deleteDemo() error { return deleteThrough("gitlab_demo_list") }
`)

	got := valuesOfKind(sites, kindErrorHint)
	if len(got) != 2 || got[0] != "gitlab_demo_list" || !strings.HasPrefix(got[1], "list them with ") || !strings.HasSuffix(got[1], " first") {
		t.Errorf("error hint values = %q, want the value the half is handed and the sentence around it", got)
	}
	if unresolved := unresolvedExprs(sites); len(unresolved) != 0 {
		t.Errorf("unresolved = %v, want nothing: the half is read where it is written", unresolved)
	}
}

// TestCollectSites_AVariadicRelatedParameter_ReadsEachElementAsAnID holds the
// other half of the rule that reads a variadic parameter the way its callers
// spell it. A list of hints spelled element by element is prose each; a list of
// IDs spelled the same way is an ID each, and the two are told apart by the
// kind the site publishes rather than by the shape of the call.
func TestCollectSites_AVariadicRelatedParameter_ReadsEachElementAsAnID(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func specWith(relatedActions ...string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: relatedActions}
}

func spellsTheElements() toolutil.ActionSpecOptions {
	return specWith("demo.get", "demo.list")
}
`)

	want := []string{"demo.get", "demo.list"}
	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, want) {
		t.Errorf("related values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// passedOverExprs is every expression the walk passed over as a value, sorted.
func passedOverExprs(sites []site) []string {
	var exprs []string
	for _, at := range sites {
		if at.PassedOver {
			exprs = append(exprs, at.Expr)
		}
	}
	sort.Strings(exprs)
	return exprs
}

// TestCollectSites_ErrorConstructorsAndRefusals_AreReadAsMessages holds the
// message sinks: an error a handler returns reaches a model as the sentence it
// was built with, and so does the refusal ErrorResult answers with.
// ErrorResultAnnotated is not one, since its callers hand it Markdown another
// sink already wrote, so its argument is no site at all.
func TestCollectSites_ErrorConstructorsAndRefusals_AreReadAsMessages(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo: use gitlab_demo_list to find one")

func refuse() any { return toolutil.ErrorResult("refused: use gitlab_demo_get first") }

func declined() any { return toolutil.CancelledResult("declined: ask before gitlab_demo_delete") }

func annotated(md string) any { return toolutil.ErrorResultAnnotated(md, nil) }

func wrapped(id int) error { return fmt.Errorf("demo %d: use gitlab_demo_get: %w", id, errDemo) }
`)

	want := []string{
		"declined: ask before gitlab_demo_delete",
		"demo %d: use gitlab_demo_get: %w",
		"demo: use gitlab_demo_list to find one",
		"refused: use gitlab_demo_get first",
	}
	if got := valuesOfKind(sites, kindMessage); !slices.Equal(got, want) {
		t.Errorf("message values = %v, want %v", got, want)
	}
	if got := passedOverExprs(sites); !slices.Equal(got, []string{"errDemo", "id"}) {
		t.Errorf("passed over = %v, want the format's two values", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none: the annotated refusal's Markdown is no site", got)
	}
}

// nextStepFixture writes every shape a result's next steps are written in: the
// three toolutil writers called directly, a domain wrapper that hands its own
// hints parameter to one, a hint escaped on its way through, and a helper
// that builds the list and returns it from either branch.
const nextStepFixture = `package fixture

import (
	"errors"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func list(b *strings.Builder, p toolutil.PaginationOutput) {
	toolutil.WriteListFooter(b, p, false, "footer gitlab_demo_footer")
}

func card(b *strings.Builder) {
	toolutil.NewCard(b, "Demo").End("card gitlab_demo_card")
}

func written(b *strings.Builder) {
	toolutil.WriteHints(b, "written gitlab_demo_written")
}

func wrapper(b *strings.Builder, hints ...string) {
	toolutil.NewCard(b, "Demo").End(hints...)
}

func callsWrapper(b *strings.Builder) {
	wrapper(b, "wrapped gitlab_demo_wrapped")
}

func builder(on bool) []string {
	if on {
		return []string{"on gitlab_demo_on"}
	}
	return []string{"off gitlab_demo_off"}
}

func fromBuilder(b *strings.Builder) {
	toolutil.WriteHints(b, builder(true)...)
}

func escaped(hint string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, toolutil.EscapeMdTableCell(hint))
}

func callsEscaped() error {
	return escaped("escaped gitlab_demo_escaped")
}
`

// TestCollectSites_NextStepWriters_AreReadWhereTheyAreWritten holds the
// next-step sinks, and holds them twice: with toolutil loaded from source, and
// narrowed to the one domain package, which loads toolutil from export data.
//
// The second run is why WriteListFooter and Card.End are sinks of their own
// rather than read through their forwarding to WriteHints: the forwarding is
// a body, and a narrowed run has no body to follow, so it would read none of
// a formatter's next steps. The first run is why the filter the list footer
// hands WriteHints is no site nothing folds: its argument is the footer's own
// hints parameter, a carrier recorded where its callers write it, and the
// filter's body, which the call is followed into, ranges over that list.
func TestCollectSites_NextStepWriters_AreReadWhereTheyAreWritten(t *testing.T) {
	want := []string{
		"card gitlab_demo_card",
		"footer gitlab_demo_footer",
		"off gitlab_demo_off",
		"on gitlab_demo_on",
		"wrapped gitlab_demo_wrapped",
		"written gitlab_demo_written",
	}

	t.Run("toolutil from source", func(t *testing.T) {
		root := repoRoot(t)
		overlay := map[string][]byte{filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(nextStepFixture)}
		sites, err := collectSites(root, fixturePatterns, overlay)
		if err != nil {
			t.Fatalf("collect sites: %v", err)
		}
		var own []site
		for _, at := range sites {
			if at.Package == fixtureDir {
				own = append(own, at)
			}
			if at.Package == "internal/toolutil" && !at.Resolved && strings.Contains(at.Expr, "withoutPreserveLinks") {
				t.Errorf("the list footer's filter is a site nothing folds: %+v", at)
			}
		}
		if got := valuesOfKind(own, kindNextStep); !slices.Equal(got, want) {
			t.Errorf("next step values = %v, want %v", got, want)
		}
		if got := valuesOfKind(own, kindErrorHint); !slices.Equal(got, []string{"escaped gitlab_demo_escaped"}) {
			t.Errorf("error hint values = %v, want the escaped parameter followed to its caller", got)
		}
		if got := unresolvedExprs(own); len(got) != 0 {
			t.Errorf("unresolved = %v, want none", got)
		}
	})

	t.Run("narrowed to the domain package", func(t *testing.T) {
		root := repoRoot(t)
		overlay := map[string][]byte{filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(nextStepFixture)}
		sites, err := collectSites(root, []string{"./" + fixtureDir + "/..."}, overlay)
		if err != nil {
			t.Fatalf("collect sites: %v", err)
		}
		if got := valuesOfKind(sites, kindNextStep); !slices.Equal(got, want) {
			t.Errorf("next step values = %v, want %v", got, want)
		}
	})
}

// TestCollectSites_ASinkArgument_KeepsTheSinksKindWhateverTheWalkMeetsFirst
// holds the kind of a site reached two ways.
//
// toolutil's sink bodies forward their prose to other sinks: WrapErrWithHint
// hands its hint to a format through hintedError, and NotFoundResult hands its
// hints to Card.End. Followed back out, those parameters reach every caller's
// argument a second time under the other sink's kind, and the first route to
// arrive kept its kind, so the answer depended on which package the walk met
// first. The walk here meets toolutil first, which is the order that used to
// file an error hint as a message.
func TestCollectSites_ASinkArgument_KeepsTheSinksKindWhateverTheWalkMeetsFirst(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func hinted() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, "hinted gitlab_demo_hinted")
}

func notFound() any {
	return toolutil.NotFoundResult("Demo", "id", "missing gitlab_demo_missing")
}
`),
	}
	loaded, err := goprogram.Load(root, fixturePatterns, overlay)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	slices.SortStableFunc(loaded, func(left, right *packages.Package) int {
		return cmp.Compare(boolRank(left.PkgPath != goprogram.ToolutilPath), boolRank(right.PkgPath != goprogram.ToolutilPath))
	})
	collect, err := newCollector(root, loaded)
	if err != nil {
		t.Fatalf("collector: %v", err)
	}
	collect.walk((*walker).visit)

	var own []site
	for _, at := range collect.sites {
		if at.Package == fixtureDir {
			own = append(own, at)
		}
	}
	if got := valuesOfKind(own, kindErrorHint); !slices.Equal(got, []string{"hinted gitlab_demo_hinted", "missing gitlab_demo_missing"}) {
		t.Errorf("error hint values = %v, want both arguments under the sink's own kind", got)
	}
	for _, kind := range []string{kindMessage, kindNextStep} {
		t.Run(kind, func(t *testing.T) {
			for _, value := range valuesOfKind(own, kind) {
				if strings.Contains(value, "gitlab_demo_") {
					t.Errorf("%s holds %q, which a sink argument reached through another sink's body", kind, value)
				}
			}
		})
	}
}

// boolRank orders false before true.
func boolRank(value bool) int {
	if value {
		return 1
	}
	return 0
}

// TestCollectSites_MessageCarriers_AreFollowedAndGitLabValuesPassedOver holds
// the message's carriers and what a message field is not.
//
// A parameter or field named for a message is followed to what it is given,
// which is how a helper shared by twenty handlers takes their sentence. A read
// of another module's field into a message field is GitLab's text (a commit's
// message, a title dereferenced from its option), counted as passed over and
// not listed; a read of a message field this walk records is a copy of prose
// judged where it was written.
func TestCollectSites_MessageCarriers_AreFollowedAndGitLabValuesPassedOver(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

type args struct {
	missingProjectMsg string
}

type output struct {
	Message      string
	AwardMessage string
}

var shared = args{missingProjectMsg: "mr: project_id is required. Use gitlab_demo_list first"}

func missing(missingMsg string) error { return errors.New(missingMsg) }

func callsMissing() error { return missing("project_id is required. Use gitlab_demo_list") }

func fromArgs(a args) error { return errors.New(a.missingProjectMsg) }

func emptyOf(emptyMessage string) output { return output{Message: emptyMessage} }

func callsEmpty() output { return emptyOf("No demos found. Use gitlab_demo_create") }

func formatted(name string) output {
	return output{Message: fmt.Sprintf("Demo %s deleted. Use gitlab_demo_restore", name)}
}

func fromCommit(c *gl.Commit) output { return output{Message: c.Message} }

func fromOption(o *gl.CreateIssueOptions) output { return output{AwardMessage: *o.Title} }

func copied(o output) output { return output{Message: o.Message} }
`)

	want := []string{
		"Demo %s deleted. Use gitlab_demo_restore",
		"No demos found. Use gitlab_demo_create",
		"mr: project_id is required. Use gitlab_demo_list first",
		"project_id is required. Use gitlab_demo_list",
	}
	if got := valuesOfKind(sites, kindMessage); !slices.Equal(got, want) {
		t.Errorf("message values = %v, want %v", got, want)
	}
	if got := passedOverExprs(sites); !slices.Equal(got, []string{"*o.Title", "c.Message", "name"}) {
		t.Errorf("passed over = %v, want GitLab's two values and the format's argument", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectSites_GuidanceAndNextStepFields_AreRead holds the two field rules
// the served prose added: a parameter's guidance, which every surface serves
// beside the schema, and the next steps a meta tool's JSON carries.
//
// The guidance constructor in toolutil takes the domain's sentence under the
// field's own name, so the parameter is followed out to the domain that wrote
// it; SemanticRole is a token rather than a sentence and is not read; and a
// copy of a guidance list through a conversion and an append is a copy, not a
// site nothing folds.
func TestCollectSites_GuidanceAndNextStepFields_AreRead(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

type result struct {
	toolutil.HintableOutput
}

func guidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"id": {SemanticRole: "demo_id", ValueSource: "From gitlab_demo_list.", CommonConfusions: []string{"Not gitlab_demo_other."}},
	}
}

func viaConstructor() toolutil.ParameterGuidance {
	return toolutil.DiscussionIDParamGuidance("Thread id from gitlab_demo_threads.")
}

func withSteps(r result) result {
	r.NextSteps = []string{"then gitlab_demo_next"}
	return r
}

func copiedGuidance(g toolutil.ParameterGuidance) toolutil.ParameterGuidance {
	g.CommonConfusions = append([]string(nil), g.CommonConfusions...)
	return g
}
`)

	want := []string{"From gitlab_demo_list.", "Not gitlab_demo_other.", "Thread id from gitlab_demo_threads."}
	if got := valuesOfKind(sites, kindParamGuidance); !slices.Equal(got, want) {
		t.Errorf("guidance values = %v, want %v", got, want)
	}
	if got := valuesOfKind(sites, kindHintField); !slices.Equal(got, []string{"then gitlab_demo_next"}) {
		t.Errorf("hint field values = %v, want the next step", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectSites_SchemaTags_AreReadAsServed holds the schema description: a
// field's jsonschema tag is read as the text every surface serves beside it,
// with the required marker the schema builder strips stripped here too, and a
// field whose tag describes nothing is no site.
func TestCollectSites_SchemaTags_AreReadAsServed(t *testing.T) {
	sites := collectFixture(t, `package fixture

type Input struct {
	ID    int    `+"`json:\"id\" jsonschema:\"Demo ID. Use gitlab_demo_list to find it,required\"`"+`
	Name  string `+"`json:\"name\"`"+`
	Blank string `+"`json:\"blank\" jsonschema:\"\"`"+`
	Plain int
}

var anonymous = struct {
	Scope string `+"`jsonschema:\"Scope, see gitlab_demo_scopes\"`"+`
}{}
`)

	want := []string{"Demo ID. Use gitlab_demo_list to find it", "Scope, see gitlab_demo_scopes"}
	if got := valuesOfKind(sites, kindSchemaDescription); !slices.Equal(got, want) {
		t.Errorf("schema descriptions = %v, want %v", got, want)
	}
	if got := len(sites); got != len(want) {
		t.Errorf("sites = %+v, want one per described field", sites)
	}
}

// TestCollectSites_AFormatInsideAOneLineHelper_FoldsForProseAndNotForAnID
// holds the prose flag the fold carries. A one-line helper returning a format
// is a sentence when prose asks for it, and the constant its caller hands it is
// read with it, which is the shape of dynamic's queryTooLongMessage. The same
// helper building an ID stays a site nothing folds: judged as its format, the
// ID would be "demo.%s", which is neither true nor useful.
func TestCollectSites_AFormatInsideAOneLineHelper_FoldsForProseAndNotForAnID(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func tooLong(tool, query string) string {
	return fmt.Sprintf("%s: query is too long (%d characters)", tool, len(query))
}

func refuse(q string) any { return toolutil.ErrorResult(tooLong("gitlab_demo_find", q)) }

func idOf(name string) string { return fmt.Sprintf("demo.%s", name) }

var Spec = toolutil.ActionSpecOptions{RelatedActions: []string{idOf("get")}}
`)

	if got := valuesOfKind(sites, kindMessage); !slices.Equal(got, []string{"%s: query is too long (%d characters) gitlab_demo_find"}) {
		t.Errorf("message values = %v, want the format and the tool name its caller handed it", got)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{`idOf("get")`}) {
		t.Errorf("unresolved = %v, want the ID a format builds reported", got)
	}
}

// TestCollectSites_AnErrorWrappedInAFormat_IsReadWhereItIsWritten holds the
// format argument that is a sink call of its own. The outer format is visited
// first, and the inner call is the expression the inner sink's visit records,
// so the outer fold must leave it to that visit rather than pass it over, or
// the inner sentence is never read. A call that is no sink is a value, and so
// is one whose callee is a function value the type checker cannot name.
func TestCollectSites_AnErrorWrappedInAFormat_IsReadWhereItIsWritten(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"
	"strconv"
)

var lookup = func() string { return "x" }

func wrapped() error {
	return fmt.Errorf("outer: %w", fmt.Errorf("inner: use gitlab_demo_inner"))
}

func withConstructor() error {
	return fmt.Errorf("outer %s: %w", strconv.Itoa(1), errors.New("constructed: use gitlab_demo_constructed"))
}

func withFunctionValue() error {
	return fmt.Errorf("outer %s", lookup())
}
`)

	want := []string{"constructed: use gitlab_demo_constructed", "inner: use gitlab_demo_inner", "outer %s", "outer %s: %w", "outer: %w"}
	if got := valuesOfKind(sites, kindMessage); !slices.Equal(got, want) {
		t.Errorf("message values = %v, want %v", got, want)
	}
	if got := passedOverExprs(sites); !slices.Equal(got, []string{"lookup()", "strconv.Itoa(1)"}) {
		t.Errorf("passed over = %v, want the two calls that are values", got)
	}
}

// TestCollectSites_ALocalHandedToAHelper_IsNoCarrier holds the carrier rule's
// one requirement besides the name: a carrier is a parameter, followed out to
// the callers that write it. A local handed to a helper is not one, and the
// helper's answer is a site nothing folds.
func TestCollectSites_ALocalHandedToAHelper_IsNoCarrier(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func trimmed(input string) error {
	hint := input + " "
	return toolutil.WrapErrWithHint("demo_get", errDemo, strings.TrimSpace(hint))
}
`)

	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"strings.TrimSpace(hint)"}) {
		t.Errorf("unresolved = %v, want the helper's answer reported", got)
	}
}

// TestCollectSites_AFormatAHelperCannotFold_LeavesTheCallUnfolded holds the
// format fold inside a one-line helper when the format itself is what the
// caller does not bind: nothing is read, and the call is a site nothing folds.
func TestCollectSites_AFormatAHelperCannotFold_LeavesTheCallUnfolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func render(format string) string { return fmt.Sprintf(format, 1) }

func refuse(format string) any { return toolutil.ErrorResult(render(format)) }
`)

	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"render(format)"}) {
		t.Errorf("unresolved = %v, want the helper call reported", got)
	}
}

// TestAddSite_OneExpressionReachedTwice_IsOneSite holds the invariant every
// route of the walk leans on: an expression is one site however many routes
// reach it. None of today's shapes reaches one twice, since a sink's own
// parameters are no longer followed out, and the guard is what keeps a route
// added tomorrow from counting a sentence twice.
func TestAddSite_OneExpressionReachedTwice_IsOneSite(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte("package fixture\n\nconst hint = \"one sentence\"\n"),
	}
	loaded, err := goprogram.Load(root, []string{"./" + fixtureDir + "/..."}, overlay)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	collect, err := newCollector(root, loaded)
	if err != nil {
		t.Fatalf("collector: %v", err)
	}
	w := &walker{collector: collect, pkg: loaded[0]}
	var literal ast.Expr
	ast.Inspect(loaded[0].Syntax[0], func(node ast.Node) bool {
		if lit, ok := node.(*ast.BasicLit); ok {
			literal = lit
		}
		return true
	})

	w.addSite(site{Kind: kindMessage, Value: "one sentence", Resolved: true}, literal)
	w.addSite(site{Kind: kindNextStep, Value: "one sentence", Resolved: true}, literal)
	if len(collect.sites) != 1 || collect.sites[0].Kind != kindMessage {
		t.Errorf("sites = %+v, want the first route's one site", collect.sites)
	}
}

// TestCollectSites_ARootItCannotResolve_IsReported holds the one failure
// between loading a program and walking it. filepath.Abs fails only when the
// process has no working directory, which a test cannot arrange, so the seam
// is what makes the branch reachable; the branch matters because a walk rooted
// at nothing stamps every finding with a path a reader cannot open.
func TestCollectSites_ARootItCannotResolve_IsReported(t *testing.T) {
	restore := absolutePath
	absolutePath = func(string) (string, error) { return "", errors.New("no working directory") }
	defer func() { absolutePath = restore }()

	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte("package fixture\n"),
	}
	sites, err := collectSites(root, fixturePatterns, overlay)
	if err == nil {
		t.Fatalf("collectSites() = %v, want the root failure reported", sites)
	}
	if !strings.Contains(err.Error(), "no working directory") {
		t.Errorf("error = %v, want the reason the root could not be resolved", err)
	}
}

// TestCollectSites_SchemaMaps_AreReadAsServed holds the half of the served
// schema a tag cannot carry: an input schema override and a hand-built
// schema are maps, and the description one gives a property is served
// exactly as a tag's is.
//
// The entry is found by its constant key and read only where it is text. A
// property that is itself named description holds a schema rather than one,
// and a guidance table keyed by that parameter holds its guidance; a key that
// is not a constant, and a map keyed by anything but a string, describe
// nothing. A description handed to a helper under a name ending in
// description is followed out to the callers that write it, a read of another
// module's field is GitLab's text and passed over (securityattributes hands an
// attribute's own description to a mutation that way), and a read of a field
// this walk records is a copy of prose judged where it was written.
func TestCollectSites_SchemaMaps_AreReadAsServed(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type output struct {
	Hint string
}

var override = toolutil.SchemaPropertyOverride("links.url", map[string]any{
	"description": "Use the URL gitlab_demo_publish returns",
	"format":      "uri",
})

var nested = map[string]any{
	"properties": map[string]any{
		"description": map[string]any{"type": "string"},
	},
}

var guidance = map[string]toolutil.ParameterGuidance{
	"description": {SemanticRole: "label"},
}

var byNumber = map[int]string{1: "description"}

func keyed(key string) map[string]string {
	return map[string]string{key: "not read gitlab_demo_key"}
}

func levelSchema(name, levelDescription string) toolutil.InputSchemaOverride {
	return toolutil.SchemaPropertyOverride(name, map[string]any{"description": levelDescription})
}

var level = levelSchema("level", "Level, see gitlab_demo_levels")

func fromGitLab(c *gl.Commit) map[string]any {
	return map[string]any{"description": c.Title}
}

func copied(o output) map[string]any {
	return map[string]any{"description": o.Hint}
}
`)

	want := []string{"Level, see gitlab_demo_levels", "Use the URL gitlab_demo_publish returns"}
	if got := valuesOfKind(sites, kindSchemaDescription); !slices.Equal(got, want) {
		t.Errorf("schema descriptions = %v, want %v", got, want)
	}
	if got := passedOverExprs(sites); !slices.Equal(got, []string{"c.Title"}) {
		t.Errorf("passed over = %v, want GitLab's own text", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestIsStringKeyedMap_NoTypeAtAll_IsNoMap holds the one input the walk hands
// it that no fixture can: a composite literal the type checker recorded no
// type for.
func TestIsStringKeyedMap_NoTypeAtAll_IsNoMap(t *testing.T) {
	if isStringKeyedMap(nil) {
		t.Error("a missing type was read as a map keyed by a string")
	}
}

// TestCollectSites_ACallHandedCarriersAlone_IsFollowedIntoItsCallee holds
// what a call handed only a parameter named for a hint is: the carrier is
// followed out to the callers that write it, and the callee is followed into,
// which is where a helper appending a sentence of its own writes it. Passing
// the call over as a copy dropped that sentence without a word.
//
// A call whose callee this load holds no body for, another module's, writes
// no sentence of this repository's and adds nothing; a call of a function
// value could do anything with what it is handed and is listed. A call handed
// a recorded read beside the carrier is a merge, passed over with the hole a
// merge carries: the sentence its body adds is not read, which is pinned here
// so that the day it is read is a day this test is changed on purpose.
func TestCollectSites_ACallHandedCarriersAlone_IsFollowedIntoItsCallee(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

type output struct {
	Hint string
}

func withList(hint string) string { return hint + ". Use gitlab_demo_list to find it" }

func wrapped(hint string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, withList(hint))
}

func callsWrapped() error { return wrapped("demo not found") }

func merged(first, hint string) string { return first + hint + " (merge adds gitlab_demo_merged)" }

func withMerge(o output, hint string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, merged(o.Hint, hint))
}

func callsWithMerge(o output) error { return withMerge(o, "merge carrier") }

func trimmed(hint string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, strings.TrimSpace(hint))
}

func callsTrimmed() error { return trimmed("trimmed gitlab_demo_trimmed") }

func throughValue(transform func(string) string, hint string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, transform(hint))
}

func callsThroughValue() error { return throughValue(strings.ToUpper, "value carrier") }
`)

	want := []string{
		" . Use gitlab_demo_list to find it",
		"demo not found",
		"merge carrier",
		"trimmed gitlab_demo_trimmed",
		"value carrier",
	}
	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, want) {
		t.Errorf("error hint values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"transform(hint)"}) {
		t.Errorf("unresolved = %v, want the call of a function value", got)
	}
}

// TestCollectSites_ARangeOverAList_IsFollowedToTheList holds the value
// variable of a range over a list of strings: it holds each element in turn,
// so it is followed to the list, read as one. A filter over the hints it is
// handed (toolutil's withoutPreserveLinks is the shape) is read through its
// range to its callers rather than listed, and an element of a list of IDs is
// an ID.
//
// Nothing else is read that way: a range over a map hands its variable a
// map's value, a range naming no value variable has nothing to follow, and a
// blank one names no variable at all.
func TestCollectSites_ARangeOverAList_IsFollowedToTheList(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func keep(hints []string) []string {
	out := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint != "" {
			out = append(out, hint)
		}
	}
	return out
}

func footer(b *strings.Builder, hints ...string) {
	toolutil.WriteHints(b, keep(hints)...)
}

func callsFooter(b *strings.Builder) {
	footer(b, "kept gitlab_demo_kept")
}

func build(relatedIDs []string) toolutil.ActionSpecOptions {
	var out []string
	for _, id := range relatedIDs {
		out = append(out, id)
	}
	return toolutil.ActionSpecOptions{RelatedActions: out}
}

func callsBuild() toolutil.ActionSpecOptions {
	return build([]string{"demo.from_a_range"})
}

func overAMap(b *strings.Builder, table map[string]string) {
	for _, hint := range table {
		toolutil.WriteHints(b, hint)
	}
}

func keysOnly(b *strings.Builder, hints []string) {
	for index := range hints {
		toolutil.WriteHints(b, hints[index])
	}
}

func blank(hints []string) int {
	count := 0
	for _, _ = range hints {
		count++
	}
	return count
}
`)

	if got := valuesOfKind(sites, kindNextStep); !slices.Equal(got, []string{"kept gitlab_demo_kept"}) {
		t.Errorf("next step values = %v, want the filtered hint read at its caller", got)
	}
	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.from_a_range"}) {
		t.Errorf("related values = %v, want the ranged ID read at its caller", got)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"hint", "hints[index]"}) {
		t.Errorf("unresolved = %v, want the map's value and the index read", got)
	}
}

// TestCollectSites_AUsageLineAssembledAtRunTime_IsFoldedAsProse holds a Usage
// line the type checker cannot fold whole. It used to be passed over in
// silence, which the tool-name rule it is now judged by cannot afford: its
// literal halves are where a tool name is spelled.
//
// So it is folded as a hint is. A local is followed to every value it is
// given, a concatenation keeps its literal halves, a format its format, a
// helper that picks a line by name is followed to every branch it returns
// from, and a parameter named for a Usage line to its callers. A read of
// another Usage field is a copy of a line recorded where it was written. What
// still folds nowhere is listed, and a value a format reports is passed over.
func TestCollectSites_AUsageLineAssembledAtRunTime_IsFoldedAsProse(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type meta struct {
	usage string
}

var table = map[string]meta{
	"list": {usage: "List demos. Prefer gitlab_demo_search"},
}

func fromLocal(list bool) toolutil.ActionSpecOptions {
	usage := "Get one demo."
	if list {
		usage = "List demos with gitlab_demo_list."
	}
	return toolutil.ActionSpecOptions{Usage: usage}
}

func fromConcatenation(name string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: "Use gitlab_demo_get for " + name}
}

func fromFormat(name string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: fmt.Sprintf("Use demo.%s after gitlab_demo_list", name)}
}

func pick(action string) string {
	switch action {
	case "list":
		return "List demos, then gitlab_demo_get."
	default:
		return "Get a demo."
	}
}

func fromHelper(action string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: pick(action)}
}

func fromParameter(leadUsage string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: leadUsage}
}

func callsParameter() toolutil.ActionSpecOptions { return fromParameter("Lead with gitlab_demo_lead.") }

func fromCopy(options *toolutil.ActionSpecOptions, m meta) {
	options.Usage = m.usage
}

func fromLookup(leads map[string]string, action string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: leads[action]}
}

func fromAnotherModule(raw string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: strings.TrimSpace(raw)}
}
`)

	want := []string{
		"Get a demo.",
		"Get one demo.",
		"Lead with gitlab_demo_lead.",
		"List demos with gitlab_demo_list.",
		"List demos, then gitlab_demo_get.",
		"List demos. Prefer gitlab_demo_search",
		"Use demo.%s after gitlab_demo_list",
		"Use gitlab_demo_get for  ",
	}
	if got := valuesOfKind(sites, kindUsage); !slices.Equal(got, want) {
		t.Errorf("usage values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"leads[action]", "name", "strings.TrimSpace(raw)"}) {
		t.Errorf("unresolved = %v, want the map lookup, the concatenated value and the call no body answers for", got)
	}
	if got := passedOverExprs(sites); !slices.Equal(got, []string{"name"}) {
		t.Errorf("passed over = %v, want the format's value", got)
	}
}

// TestCollectSites_AUsageFormatHandedAHelperCall_ReadsEveryBranch holds the
// one kind whose format follows a call argument into the helper it names: a
// Usage line assembled as fmt.Sprintf("%s ... %s", describe(verb),
// boundary(scope)), which is how badges writes its twelve lines. Both helpers
// pick a sentence by a parameter, and each branch they return is read, the
// constant whole and the format folded with its own value passed over. The
// call used to be passed over as a value the line reports, and both of the
// boundary's sentences named a meta tool that way.
//
// Two calls stay values. One into another module has no body to follow, and
// the same helper handed to a hint's format is passed over, because a hint's
// calls are what it reports: following them read toolutil's escaping as
// sentences nothing folds.
func TestCollectSites_AUsageFormatHandedAHelperCall_ReadsEveryBranch(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func boundary(scope string) string {
	if scope == "group" {
		return "Group badges only. Project badges belong to gitlab_demo_project."
	}
	return "Do not use gitlab_demo_group for project badges."
}

func describe(verb string) string {
	switch verb {
	case "add":
		return fmt.Sprintf("Add a %s badge.", verb)
	default:
		return "Manage badges."
	}
}

func hintBoundary(scope string) string {
	return "not read gitlab_demo_hint " + scope
}

func fromHelpers(verb, scope, idParam string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: fmt.Sprintf("%s Use %s. %s", describe(verb), idParam, boundary(scope))}
}

func fromAnotherModule(name string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{Usage: fmt.Sprintf("Use %s.", strings.ToUpper(name))}
}

func hinted(scope string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, fmt.Sprintf("retry: %s", hintBoundary(scope)))
}
`)

	want := []string{
		"%s Use %s. %s",
		"Add a %s badge.",
		"Do not use gitlab_demo_group for project badges.",
		"Group badges only. Project badges belong to gitlab_demo_project.",
		"Manage badges.",
		"Use %s.",
	}
	if got := valuesOfKind(sites, kindUsage); !slices.Equal(got, want) {
		t.Errorf("usage values = %v, want %v", got, want)
	}
	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, []string{"retry: %s"}) {
		t.Errorf("error hint values = %v, want the hint's format alone", got)
	}
	wantPassed := []string{"hintBoundary(scope)", "idParam", "strings.ToUpper(name)", "verb"}
	if got := passedOverExprs(sites); !slices.Equal(got, wantPassed) {
		t.Errorf("passed over = %v, want %v", got, wantPassed)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectSites_AMessageParameterInAFormat_IsFollowedToItsCallers holds
// the one name a format argument is followed under besides a hint's: a
// parameter named for a message, which is a helper handing on the sentence
// each of its callers wrote. A local of the same name is GitLab's message
// spelled into a sentence the server writes around it, and a parameter so
// named where the site's kind follows no message (a hint's format) is a value
// on the same terms; both are passed over, and the sentence handed to the
// second is not read.
func TestCollectSites_AMessageParameterInAFormat_IsFollowedToItsCallers(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func requireProject(op, missingProjectMsg string) error {
	return fmt.Errorf("%s: %s", op, missingProjectMsg)
}

func callsRequire() error {
	return requireProject("demo_get", "project_id is required. Use gitlab_demo_list")
}

func fromGitLab(err error) error {
	glMsg := err.Error()
	return fmt.Errorf("demo failed: %s", glMsg)
}

func hinted(detailMsg string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, fmt.Sprintf("retry: %s", detailMsg))
}

func callsHinted() error { return hinted("not read gitlab_demo_detail") }
`)

	want := []string{"%s: %s", "demo", "demo failed: %s", "project_id is required. Use gitlab_demo_list"}
	if got := valuesOfKind(sites, kindMessage); !slices.Equal(got, want) {
		t.Errorf("message values = %v, want %v", got, want)
	}
	if got := valuesOfKind(sites, kindErrorHint); !slices.Equal(got, []string{"retry: %s"}) {
		t.Errorf("error hint values = %v, want the format alone", got)
	}
	if got := passedOverExprs(sites); !slices.Equal(got, []string{"detailMsg", "glMsg", "op"}) {
		t.Errorf("passed over = %v, want the operation, GitLab's message and the hint's value", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestListRecorder_EachKind_ReadsTheListItsElementBelongsTo holds how the list
// a range walks is read: as sentences for every prose kind, a Usage line
// included although it is judged in both sections, and as IDs for the
// published IDs.
//
// The element is a literal concatenated with a value, which is where the two
// readings part: a sentence keeps its literal half and lists the value on its
// own, and an ID folds whole or not at all, so it is one site nothing folds.
func TestListRecorder_EachKind_ReadsTheListItsElementBelongsTo(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte("package fixture\n\nfunc list(name string) []string { return []string{\"read \" + name} }\n"),
	}
	loaded, err := goprogram.Load(root, []string{"./" + fixtureDir + "/..."}, overlay)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var list ast.Expr
	ast.Inspect(loaded[0].Syntax[0], func(node ast.Node) bool {
		if lit, ok := node.(*ast.CompositeLit); ok {
			list = lit
		}
		return true
	})
	asSentence := []site{{Value: "read  ", Resolved: true}, {Expr: "name"}}
	asID := []site{{Expr: `"read " + name`}}
	for kind, want := range map[string][]site{
		kindUsage:     asSentence,
		kindNextStep:  asSentence,
		kindAssertion: asSentence,
		kindRelated:   asID,
		kindHint:      asID,
	} {
		t.Run(kind, func(t *testing.T) {
			collect, collectErr := newCollector(root, loaded)
			if collectErr != nil {
				t.Fatalf("collector: %v", collectErr)
			}
			listRecorder(kind)(&walker{collector: collect, pkg: loaded[0]}, kind, list)
			got := make([]site, 0, len(collect.sites))
			for _, at := range collect.sites {
				got = append(got, site{Value: at.Value, Expr: at.Expr, Resolved: at.Resolved})
			}
			slices.SortFunc(got, func(left, right site) int { return cmp.Compare(left.Expr, right.Expr) })
			if !slices.Equal(got, want) {
				t.Errorf("sites = %+v, want %+v", got, want)
			}
		})
	}
}
