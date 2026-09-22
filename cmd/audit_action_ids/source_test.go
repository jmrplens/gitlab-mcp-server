package main

import (
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

// unresolvedExprs is every expression the walk could not fold, sorted.
func unresolvedExprs(sites []site) []string {
	var exprs []string
	for _, at := range sites {
		if !at.Resolved {
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
// as a row rather than as a silence. A hint built by a formatter carries no
// literal a reader could have got wrong, but a rule whose blind spot says
// nothing is one a future site steps into, so the site is named.
func TestCollectSites_HintNothingCanFold_IsReported(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

func unfoldable(value string) error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, fmt.Sprintf("built from %s", value))
}
`)

	want := []string{`fmt.Sprintf("built from %s", value)`}
	if got := unresolvedExprs(sites); !slices.Equal(got, want) {
		t.Errorf("unresolved = %v, want %v", got, want)
	}
	if got := valuesOfKind(sites, kindErrorHint); len(got) != 0 {
		t.Errorf("error hint values = %v, want nothing folded", got)
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
// make, which allocate no prose between them. The rest are values this walk
// cannot follow, and each is named with the expression as it was written,
// since naming what could not be read is the only honest alternative to
// passing over it.
func TestCollectSites_HintShapesNothingCanRead_AreReportedOneByOne(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"errors"

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
		"build()",
		"describe(toolutil.HintPreserveLinks)",
		"left + right",
		"other.Label",
		"other.Notes",
		"render(o.hint)",
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
		"hintAction":     false,
		"usage":          false,
		"related":        false,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isHintName(name); got != want {
				t.Errorf("isHintName(%q) = %t, want %t", name, got, want)
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
// it keeps the literal, which is where a capability would be spelled.
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
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"text"}) {
		t.Errorf("unresolved = %v, want the bare parameter named once", got)
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
