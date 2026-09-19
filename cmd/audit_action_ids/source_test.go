package main

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
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
