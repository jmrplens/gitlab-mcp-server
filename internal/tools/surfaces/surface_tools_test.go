// surface_tools_test.go holds the unit tests for standalone surface
// projection: which visible tools this package publishes outside the ordinary
// GitLab meta-tool dispatchers, what metadata each of them carries, and how
// those specs become catalog groups for the dynamic surface.
package surfaces

import (
	"context"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// surfaceTestInput defines the parameters of the stand-in action used by the
// fixtures below.
type surfaceTestInput struct {
	Value string `json:"value" jsonschema:"value to echo"`
}

// surfaceTestOutput is what the stand-in action answers with.
type surfaceTestOutput struct {
	OK bool `json:"ok" jsonschema:"operation result"`
}

// newProjectionClient returns a GitLab client whose transport fails the test if
// anything reaches it. Projecting the standalone surface is metadata work, and
// a request here would mean the visible tool list cannot be built without an
// instance answering first.
func newProjectionClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected GitLab request while projecting surface tools: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

// testSurfaceSpec returns a valid surface tool spec for the given group and
// action, so a test can vary the one field it is about.
func testSurfaceSpec(groupToolName, baseDomain, name, actionName string) actioncatalog.SurfaceToolSpec {
	return actioncatalog.SurfaceToolSpec{
		Name:          name,
		Title:         "Test Surface",
		Description:   "Test surface utility.",
		GroupToolName: groupToolName,
		BaseDomain:    baseDomain,
		ActionName:    actionName,
		SurfaceKind:   actioncatalog.SurfaceKindRuntimeUtility,
		Route: toolutil.RouteFunc(func(context.Context, surfaceTestInput) (surfaceTestOutput, error) {
			return surfaceTestOutput{OK: true}, nil
		}),
		OwnerPackage: "surfaces",
		ReadOnly:     true,
	}
}

// seedCatalogGroup returns a minimal catalog group a test can plant in a
// catalog before projecting the surface specs over it.
func seedCatalogGroup(toolName, actionName string) actioncatalog.Group {
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: toolName, OwnerPackage: "surfaces"})
	group.SetAction(actioncatalog.Action{Name: actionName, Route: toolutil.ActionRoute{
		Handler:     func(context.Context, map[string]any) (any, error) { return surfaceTestOutput{}, nil },
		InputSchema: map[string]any{"type": "object"},
	}})
	return group
}

// catalogActionIDs returns every action ID the catalog holds, sorted, so a
// test can state the whole projected surface in one comparison.
func catalogActionIDs(catalog *actioncatalog.Catalog) []string {
	ids := make([]string, 0, catalog.CountActions())
	for _, action := range catalog.Actions() {
		ids = append(ids, string(action.ID))
	}
	slices.Sort(ids)
	return ids
}

// surfaceExpectation is the metadata one standalone surface tool must carry.
type surfaceExpectation struct {
	name          string
	groupToolName string
	baseDomain    string
	actionName    string
	surfaceKind   actioncatalog.SurfaceKind
	ownerPackage  string
	icons         []mcp.Icon
	capabilities  []string
	readOnly      bool
}

// assertSurfaceSpec checks one projected surface spec against what its group
// must stamp on it.
func assertSurfaceSpec(t *testing.T, spec actioncatalog.SurfaceToolSpec, want surfaceExpectation) {
	t.Helper()
	if spec.Name != want.name {
		t.Fatalf("Name = %q, want %q", spec.Name, want.name)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want a registrable spec", err)
	}
	if spec.GroupToolName != want.groupToolName || spec.BaseDomain != want.baseDomain || spec.ActionName != want.actionName {
		t.Errorf("group = %q/%q/%q, want %q/%q/%q",
			spec.GroupToolName, spec.BaseDomain, spec.ActionName,
			want.groupToolName, want.baseDomain, want.actionName)
	}
	if spec.SurfaceKind != want.surfaceKind {
		t.Errorf("SurfaceKind = %q, want %q", spec.SurfaceKind, want.surfaceKind)
	}
	if spec.OwnerPackage != want.ownerPackage {
		t.Errorf("OwnerPackage = %q, want %q", spec.OwnerPackage, want.ownerPackage)
	}
	if spec.ReadOnly != want.readOnly {
		t.Errorf("ReadOnly = %t, want %t", spec.ReadOnly, want.readOnly)
	}
	if !slices.Equal(spec.CapabilityRequirements, want.capabilities) {
		t.Errorf("CapabilityRequirements = %v, want %v", spec.CapabilityRequirements, want.capabilities)
	}
	if !reflect.DeepEqual(spec.Icons, want.icons) {
		t.Errorf("Icons = %d icons, want the %d of its domain icon", len(spec.Icons), len(want.icons))
	}
	if spec.SafeModePolicy != surfaceSafeModeGlobalWrapper {
		t.Errorf("SafeModePolicy = %q, want %q", spec.SafeModePolicy, surfaceSafeModeGlobalWrapper)
	}
	if spec.ReadOnlyPolicy != surfaceReadOnlyGlobalFilter {
		t.Errorf("ReadOnlyPolicy = %q, want %q", spec.ReadOnlyPolicy, surfaceReadOnlyGlobalFilter)
	}
	if spec.FormatResult == nil {
		t.Error("FormatResult = nil, want the group's formatter")
	}
}

// assertSurfaceMatchesAction checks that a surface spec republishes the action
// spec behind it rather than metadata of its group.
func assertSurfaceMatchesAction(t *testing.T, spec actioncatalog.SurfaceToolSpec, source toolutil.ActionSpec) {
	t.Helper()
	if spec.Title != source.IndividualTool.Title {
		t.Errorf("Title = %q, want %q", spec.Title, source.IndividualTool.Title)
	}
	if spec.Description != source.IndividualTool.Description {
		t.Errorf("Description = %.40q…, want the action's own description %.40q…", spec.Description, source.IndividualTool.Description)
	}
	if spec.ActionName != source.Name {
		t.Errorf("ActionName = %q, want %q", spec.ActionName, source.Name)
	}
	if !slices.Equal(spec.Aliases, source.Aliases) {
		t.Errorf("Aliases = %v, want %v", spec.Aliases, source.Aliases)
	}
	if !slices.Equal(spec.Tags, source.Tags) {
		t.Errorf("Tags = %v, want %v", spec.Tags, source.Tags)
	}
	if !slices.Equal(spec.RelatedActions, source.RelatedActions) {
		t.Errorf("RelatedActions = %v, want %v", spec.RelatedActions, source.RelatedActions)
	}
	if spec.ReadOnly != source.ReadOnly || spec.Destructive != source.Destructive ||
		spec.Idempotent != source.Idempotent || spec.OpenWorld != source.OpenWorld {
		t.Errorf("annotations = readOnly:%t destructive:%t idempotent:%t openWorld:%t, want %t/%t/%t/%t",
			spec.ReadOnly, spec.Destructive, spec.Idempotent, spec.OpenWorld,
			source.ReadOnly, source.Destructive, source.Idempotent, source.OpenWorld)
	}
	if spec.Route.Handler == nil || spec.Route.InputSchema == nil || spec.Route.OutputSchema == nil {
		t.Errorf("Route = handler:%v input:%v output:%v, want all three",
			spec.Route.Handler != nil, spec.Route.InputSchema != nil, spec.Route.OutputSchema != nil)
	}
}

// catalogGroupExpectation is the metadata one projected catalog group must
// carry once it reaches the catalog.
type catalogGroupExpectation struct {
	name         string
	toolName     string
	baseDomain   string
	surfaceKind  actioncatalog.SurfaceKind
	ownerPackage string
	capabilities []string
	readOnly     bool
	actionCount  int
}

// assertCatalogGroup checks one group of the projected catalog.
func assertCatalogGroup(t *testing.T, group actioncatalog.Group, want catalogGroupExpectation) {
	t.Helper()
	if group.BaseDomain != want.baseDomain {
		t.Errorf("BaseDomain = %q, want %q", group.BaseDomain, want.baseDomain)
	}
	if group.SurfaceKind != want.surfaceKind {
		t.Errorf("SurfaceKind = %q, want %q", group.SurfaceKind, want.surfaceKind)
	}
	if group.OwnerPackage != want.ownerPackage {
		t.Errorf("OwnerPackage = %q, want %q", group.OwnerPackage, want.ownerPackage)
	}
	if !slices.Equal(group.CapabilityRequirements, want.capabilities) {
		t.Errorf("CapabilityRequirements = %v, want %v", group.CapabilityRequirements, want.capabilities)
	}
	// The group is read-only only when every action in it is: that is what
	// decides whether --read-only keeps the whole dispatcher.
	if group.ReadOnly != want.readOnly {
		t.Errorf("ReadOnly = %t, want %t", group.ReadOnly, want.readOnly)
	}
	if got := len(group.ActionsInOrder()); got != want.actionCount {
		t.Errorf("actions = %d, want %d", got, want.actionCount)
	}
	if group.Description == "" {
		t.Error("Description = empty, want the group description a model reads")
	}
	if group.FormatResult == nil {
		t.Error("FormatResult = nil, want the group's formatter")
	}
}

// declaredActionAliases indexes every historical action alias
// [actioncompat.ActionAliases] declares by the canonical action ID it points
// at. It is the oracle the projection has to satisfy, and it lives in another
// package, so an assertion against it cannot be satisfied by the projection
// moving both sides of the comparison.
func declaredActionAliases() map[string][]actioncompat.ActionAlias {
	byCanonical := make(map[string][]actioncompat.ActionAlias)
	for _, alias := range actioncompat.ActionAliases() {
		byCanonical[alias.Canonical] = append(byCanonical[alias.Canonical], alias)
	}
	return byCanonical
}

// assertCarriesDeclaredAliases checks that every alias declared for canonicalID
// is republished in policy, aimed at actionName and carrying the declaration's
// own terms. It returns how many aliases it checked, so a caller can refuse a
// run that asserted nothing because the lookup found no declaration at all.
func assertCarriesDeclaredAliases(t *testing.T, policy toolutil.CompatibilityPolicy, canonicalID, actionName string, declared map[string][]actioncompat.ActionAlias) int {
	t.Helper()
	published := make(map[string]toolutil.ActionAliasSpec, len(policy.ActionAliases))
	for _, alias := range policy.ActionAliases {
		published[alias.Alias] = alias
	}
	want := declared[canonicalID]
	for _, alias := range want {
		got, ok := published[alias.Alias]
		if !ok {
			t.Errorf("declared alias %q is not republished; %q publishes %v",
				alias.Alias, canonicalID, slices.Sorted(maps.Keys(published)))
			continue
		}
		if got.Target != actionName {
			t.Errorf("alias %q targets %q, want the action it was declared for, %q", alias.Alias, got.Target, actionName)
		}
		if got.Source != alias.Source || got.Searchable != alias.Searchable || got.Deprecated != alias.Deprecated ||
			got.RemovalVersion != alias.RemovalVersion || got.Reason != alias.Reason {
			t.Errorf("alias %q = %+v, want the declaration's own terms %+v", alias.Alias, got, alias)
		}
	}
	return len(want)
}

// TestStandaloneToolSpecs_ConfiguredClient_PublishesDiscoveryAndInteractiveSurfaces
// asserts the whole standalone surface: the five visible tools, the group each
// belongs to, and the group-level metadata the projection stamps on them.
//
// The group-level fields are what decide how the runtime treats a tool: the
// surface kind and the capability requirement gate registration, the safe-mode
// and read-only policy names say the tool is handled by the global wrapper and
// filter rather than by a policy of its own, and the owner package is what the
// request-path audit joins recorded requests on.
func TestStandaloneToolSpecs_ConfiguredClient_PublishesDiscoveryAndInteractiveSurfaces(t *testing.T) {
	specs := StandaloneToolSpecs(newProjectionClient(t))

	want := []surfaceExpectation{
		{
			name:          "gitlab_discover_project",
			groupToolName: "gitlab_discover_project",
			baseDomain:    "discover_project",
			actionName:    "resolve",
			surfaceKind:   actioncatalog.SurfaceKindRuntimeUtility,
			ownerPackage:  "projectdiscovery",
			icons:         toolutil.IconProject,
			readOnly:      true,
		},
		{
			name:          "gitlab_interactive_issue_create",
			groupToolName: "gitlab_interactive",
			baseDomain:    "interactive",
			actionName:    "issue_create",
			surfaceKind:   actioncatalog.SurfaceKindInteractiveUtility,
			ownerPackage:  "elicitationtools",
			icons:         toolutil.IconConfig,
			capabilities:  []string{"elicitation"},
		},
		{
			name:          "gitlab_interactive_mr_create",
			groupToolName: "gitlab_interactive",
			baseDomain:    "interactive",
			actionName:    "mr_create",
			surfaceKind:   actioncatalog.SurfaceKindInteractiveUtility,
			ownerPackage:  "elicitationtools",
			icons:         toolutil.IconConfig,
			capabilities:  []string{"elicitation"},
		},
		{
			name:          "gitlab_interactive_project_create",
			groupToolName: "gitlab_interactive",
			baseDomain:    "interactive",
			actionName:    "project_create",
			surfaceKind:   actioncatalog.SurfaceKindInteractiveUtility,
			ownerPackage:  "elicitationtools",
			icons:         toolutil.IconConfig,
			capabilities:  []string{"elicitation"},
		},
		{
			name:          "gitlab_interactive_release_create",
			groupToolName: "gitlab_interactive",
			baseDomain:    "interactive",
			actionName:    "release_create",
			surfaceKind:   actioncatalog.SurfaceKindInteractiveUtility,
			ownerPackage:  "elicitationtools",
			icons:         toolutil.IconConfig,
			capabilities:  []string{"elicitation"},
		},
	}

	if len(specs) != len(want) {
		names := make([]string, 0, len(specs))
		for _, spec := range specs {
			names = append(names, spec.Name)
		}
		t.Fatalf("StandaloneToolSpecs() published %d tools (%v), want %d", len(specs), names, len(want))
	}

	for i, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			assertSurfaceSpec(t, specs[i], tc)
		})
	}
}

// TestStandaloneToolSpecs_EverySurface_CarriesItsOwnActionMetadata asserts that
// the projection copies each action's individual-tool metadata onto its surface
// spec rather than the group's.
//
// The distinction is the whole point of the two-level shape: the group carries
// one short description for the dispatcher, and each action carries the long
// description a model reads in tools/list. A projection that stamped the group
// text on every action would leave four interactive tools describing each other.
func TestStandaloneToolSpecs_EverySurface_CarriesItsOwnActionMetadata(t *testing.T) {
	client := newProjectionClient(t)
	sourceSpecs := append(projectdiscovery.ActionSpecs(client), elicitationtools.ActionSpecs(client)...)
	sources := make(map[string]toolutil.ActionSpec, len(sourceSpecs))
	for _, spec := range sourceSpecs {
		sources[spec.IndividualTool.Name] = spec
	}

	for _, spec := range StandaloneToolSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			source, ok := sources[spec.Name]
			if !ok {
				t.Fatalf("surface tool %q has no action spec behind it", spec.Name)
			}
			assertSurfaceMatchesAction(t, spec, source)
		})
	}
}

// TestAddToolCatalog_NilCatalog_BuildsOneGroupPerSurface asserts that a nil
// catalog is created rather than dereferenced, and that both standalone groups
// arrive with the canonical action IDs and the group metadata the dispatcher
// registers them by.
func TestAddToolCatalog_NilCatalog_BuildsOneGroupPerSurface(t *testing.T) {
	catalog, err := AddToolCatalog(nil, StandaloneToolSpecs(newProjectionClient(t)), CatalogOptions{})
	if err != nil {
		t.Fatalf("AddToolCatalog(nil) error = %v", err)
	}
	if catalog == nil {
		t.Fatal("AddToolCatalog(nil) = nil catalog, want one built for the caller")
	}

	wantIDs := []string{
		"discover_project.resolve",
		"interactive.issue_create",
		"interactive.mr_create",
		"interactive.project_create",
		"interactive.release_create",
	}
	if got := catalogActionIDs(catalog); !slices.Equal(got, wantIDs) {
		t.Fatalf("catalog actions = %v, want %v", got, wantIDs)
	}

	groups := []catalogGroupExpectation{
		{
			name:         "discover project",
			toolName:     "gitlab_discover_project",
			baseDomain:   "discover_project",
			surfaceKind:  actioncatalog.SurfaceKindRuntimeUtility,
			ownerPackage: "projectdiscovery",
			readOnly:     true,
			actionCount:  1,
		},
		{
			name:         "interactive",
			toolName:     "gitlab_interactive",
			baseDomain:   "interactive",
			surfaceKind:  actioncatalog.SurfaceKindInteractiveUtility,
			ownerPackage: "elicitationtools",
			capabilities: []string{"elicitation"},
			actionCount:  4,
		},
	}
	for _, tc := range groups {
		t.Run(tc.name, func(t *testing.T) {
			group, ok := catalog.Group(tc.toolName)
			if !ok {
				t.Fatalf("catalog is missing group %q", tc.toolName)
			}
			assertCatalogGroup(t, group, tc)
		})
	}
}

// TestAddToolCatalog_PopulatedCatalog_KeepsWhatItAlreadyHeld asserts that the
// projection adds to a catalog the caller already filled instead of replacing
// it, which is how dynamic mode folds the standalone tools into the meta
// catalog it has already assembled.
func TestAddToolCatalog_PopulatedCatalog_KeepsWhatItAlreadyHeld(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	if err := catalog.AddGroup(seedCatalogGroup("gitlab_project", "get")); err != nil {
		t.Fatalf("AddGroup(seed) error = %v", err)
	}

	got, err := AddToolCatalog(catalog, StandaloneToolSpecs(newProjectionClient(t)), CatalogOptions{})
	if err != nil {
		t.Fatalf("AddToolCatalog() error = %v", err)
	}
	if _, ok := got.Group("gitlab_project"); !ok {
		t.Error("catalog lost the pre-existing gitlab_project group")
	}
	if _, ok := got.Group("gitlab_discover_project"); !ok {
		t.Error("catalog is missing gitlab_discover_project")
	}
	if _, ok := got.Group("gitlab_interactive"); !ok {
		t.Error("catalog is missing gitlab_interactive")
	}
}

// TestAddToolCatalog_ReadOnlyOnly_DropsTheMutatingSurfaces asserts that the
// read-only projection keeps project discovery and drops the interactive
// creation flows entirely, group and all: a read-only deployment must not
// publish a tool whose only purpose is to create something.
func TestAddToolCatalog_ReadOnlyOnly_DropsTheMutatingSurfaces(t *testing.T) {
	catalog, err := AddToolCatalog(nil, StandaloneToolSpecs(newProjectionClient(t)), CatalogOptions{ReadOnlyOnly: true})
	if err != nil {
		t.Fatalf("AddToolCatalog() error = %v", err)
	}
	if got := catalogActionIDs(catalog); !slices.Equal(got, []string{"discover_project.resolve"}) {
		t.Fatalf("catalog actions = %v, want only discover_project.resolve", got)
	}
	if _, ok := catalog.Group("gitlab_interactive"); ok {
		t.Error("catalog still holds gitlab_interactive in read-only mode")
	}
}

// TestAddToolCatalog_ExcludedToolNames_RemoveWhatTheOperatorNamed asserts that
// --exclude-tools is honored by every spelling the operator can write: the
// group tool name removes the whole dispatcher, and an individual tool name
// or a canonical action ID removes only that action, while a name that
// matches nothing removes nothing.
//
// The canonical ID cases are the ones issue 911 added. This projection
// matched the tool and the group name only, so interactive.issue_create, the
// one spelling that means the same thing on every surface, left the flow
// executable here.
func TestAddToolCatalog_ExcludedToolNames_RemoveWhatTheOperatorNamed(t *testing.T) {
	everyID := []string{
		"discover_project.resolve",
		"interactive.issue_create",
		"interactive.mr_create",
		"interactive.project_create",
		"interactive.release_create",
	}

	testCases := []struct {
		name    string
		exclude []string
		wantIDs []string
	}{
		{
			name:    "group tool name removes the whole group",
			exclude: []string{"gitlab_interactive"},
			wantIDs: []string{"discover_project.resolve"},
		},
		{
			name:    "individual tool name removes one action",
			exclude: []string{"gitlab_interactive_issue_create"},
			wantIDs: []string{
				"discover_project.resolve",
				"interactive.mr_create",
				"interactive.project_create",
				"interactive.release_create",
			},
		},
		{
			name:    "canonical action ID removes one action",
			exclude: []string{"interactive.issue_create"},
			wantIDs: []string{
				"discover_project.resolve",
				"interactive.mr_create",
				"interactive.project_create",
				"interactive.release_create",
			},
		},
		{
			name:    "the discovery action ID removes discovery",
			exclude: []string{"discover_project.resolve"},
			wantIDs: []string{
				"interactive.issue_create",
				"interactive.mr_create",
				"interactive.project_create",
				"interactive.release_create",
			},
		},
		{
			name:    "a prefix of a tool name removes nothing",
			exclude: []string{"gitlab_interactive_issue"},
			wantIDs: everyID,
		},
		{
			name:    "surrounding whitespace still matches",
			exclude: []string{"  gitlab_discover_project  "},
			wantIDs: []string{
				"interactive.issue_create",
				"interactive.mr_create",
				"interactive.project_create",
				"interactive.release_create",
			},
		},
		{
			name:    "blank entries exclude nothing",
			exclude: []string{"", "   "},
			wantIDs: everyID,
		},
		{
			name:    "an unknown name excludes nothing",
			exclude: []string{"gitlab_not_a_tool"},
			wantIDs: everyID,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := AddToolCatalog(nil, StandaloneToolSpecs(newProjectionClient(t)), CatalogOptions{ExcludeToolNames: tc.exclude})
			if err != nil {
				t.Fatalf("AddToolCatalog() error = %v", err)
			}
			if got := catalogActionIDs(catalog); !slices.Equal(got, tc.wantIDs) {
				t.Fatalf("catalog actions = %v, want %v", got, tc.wantIDs)
			}
		})
	}
}

// TestExcludedToolSpecs_EverySpelling_ReportsRemovedNamesAndDeadEntries
// asserts the resolver every surface asks about the standalone utilities: the
// names it returns are the ones the specs register under, whichever spelling
// reached them, and the entries it returns are the ones that reached none, in
// the order the operator wrote them.
//
// The unmatched half is what the exclusion warning is built from, so an entry
// reported there that did remove something accuses a working configuration,
// and one left out hides a dead entry.
func TestExcludedToolSpecs_EverySpelling_ReportsRemovedNamesAndDeadEntries(t *testing.T) {
	flows := []string{
		"gitlab_interactive_issue_create",
		"gitlab_interactive_mr_create",
		"gitlab_interactive_project_create",
		"gitlab_interactive_release_create",
	}

	testCases := []struct {
		name          string
		exclude       []string
		wantExcluded  []string
		wantUnmatched []string
	}{
		{
			name:          "every spelling at once, beside one that names nothing",
			exclude:       []string{"interactive.issue_create", "gitlab_interactive_mr_create", "gitlab_interactive", "gitlab_nope"},
			wantExcluded:  flows,
			wantUnmatched: []string{"gitlab_nope"},
		},
		{
			name:         "a canonical action ID reaches the tool it is registered as",
			exclude:      []string{"interactive.issue_create"},
			wantExcluded: []string{"gitlab_interactive_issue_create"},
		},
		{
			name:         "the discovery ID reaches the discovery tool",
			exclude:      []string{"discover_project.resolve"},
			wantExcluded: []string{"gitlab_discover_project"},
		},
		{
			name:          "entries naming nothing come back in the operator's order",
			exclude:       []string{"gitlab_zzz", "interactive.mr_create", "gitlab_aaa"},
			wantExcluded:  []string{"gitlab_interactive_mr_create"},
			wantUnmatched: []string{"gitlab_zzz", "gitlab_aaa"},
		},
		{
			name:          "a prefix of a tool name reaches nothing",
			exclude:       []string{"gitlab_interactive_issue"},
			wantUnmatched: []string{"gitlab_interactive_issue"},
		},
		{
			name:    "blank entries reach nothing and are not reported",
			exclude: []string{"", "   "},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			excluded, unmatched, err := ExcludedToolSpecs(StandaloneToolSpecs(newProjectionClient(t)), tc.exclude)
			if err != nil {
				t.Fatalf("ExcludedToolSpecs() error = %v", err)
			}
			if got := slices.Sorted(maps.Keys(excluded)); !slices.Equal(got, tc.wantExcluded) {
				t.Errorf("excluded = %v, want %v", got, tc.wantExcluded)
			}
			if !slices.Equal(unmatched, tc.wantUnmatched) {
				t.Errorf("unmatched = %v, want %v", unmatched, tc.wantUnmatched)
			}
		})
	}
}

// TestExcludedToolSpecs_NoEntries_BuildsNothing asserts that an empty
// exclusion list is answered without assembling the specs, and that a list
// with entries is answered by assembling them, so a spec that cannot be
// assembled is reported rather than resolved against.
//
// [AddToolCatalog] asks on every call, and most calls, --tool-search and the
// audit commands among them, exclude nothing. Assembling a catalog there only
// to throw it away is work nobody asked for, and the specs given here would
// make that work fail, which is how the early answer is observed.
func TestExcludedToolSpecs_NoEntries_BuildsNothing(t *testing.T) {
	broken := []actioncatalog.SurfaceToolSpec{
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_first", "surface"),
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_second", "surface"),
	}

	excluded, unmatched, err := ExcludedToolSpecs(broken, nil)
	if err != nil || excluded != nil || unmatched != nil {
		t.Errorf("ExcludedToolSpecs(broken, nil) = %v, %v, %v; want nothing built and nothing reported", excluded, unmatched, err)
	}

	excluded, unmatched, err = ExcludedToolSpecs(broken, []string{"gitlab_test_first"})
	if err == nil || !strings.Contains(err.Error(), "build surface tool group gitlab_test") {
		t.Errorf("ExcludedToolSpecs(broken, entries) error = %v, want the group that would not build named", err)
	}
	if excluded != nil || unmatched != nil {
		t.Errorf("ExcludedToolSpecs(broken, entries) = %v, %v beside the error; want nothing resolved", excluded, unmatched)
	}
}

// TestAddToolCatalog_ExclusionOverSpecsThatDoNotBuild_SaysItWasTheExclusion
// asserts that a projection asked to exclude something from specs that cannot
// be assembled fails naming the exclusion as the step that failed, and hands
// back no catalog.
//
// Without the exclusion the same specs fail later, at the projection itself;
// the prefix is what tells a reader of the startup error which of the two
// assemblies it was.
func TestAddToolCatalog_ExclusionOverSpecsThatDoNotBuild_SaysItWasTheExclusion(t *testing.T) {
	specs := []actioncatalog.SurfaceToolSpec{
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_first", "surface"),
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_second", "surface"),
	}

	catalog, err := AddToolCatalog(nil, specs, CatalogOptions{ExcludeToolNames: []string{"gitlab_test_first"}})
	if err == nil || !strings.HasPrefix(err.Error(), "resolve excluded surface tools: build surface tool group gitlab_test") {
		t.Errorf("AddToolCatalog() error = %v, want the exclusion named as the step that failed", err)
	}
	if catalog != nil {
		t.Errorf("AddToolCatalog() catalog = %v, want nil beside the error", catalog)
	}
}

// TestAddToolCatalog_PaddedSpecName_IsExcludedByTheNameItRegistersUnder
// asserts that a spec declaring its name with stray spaces is still removed
// by the name it registers under, which registration trims.
//
// The resolver answers with registered names, and the projection compares
// them with the specs' declared ones; comparing untrimmed, such a spec would
// be named excluded by the resolver and served anyway.
func TestAddToolCatalog_PaddedSpecName_IsExcludedByTheNameItRegistersUnder(t *testing.T) {
	specs := []actioncatalog.SurfaceToolSpec{
		testSurfaceSpec("gitlab_test", "test", "  gitlab_test_padded  ", "padded"),
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_kept", "kept"),
	}

	catalog, err := AddToolCatalog(nil, specs, CatalogOptions{ExcludeToolNames: []string{"gitlab_test_padded"}})
	if err != nil {
		t.Fatalf("AddToolCatalog() error = %v", err)
	}
	if got := catalogActionIDs(catalog); !slices.Equal(got, []string{"test.kept"}) {
		t.Errorf("catalog actions = %v, want only test.kept", got)
	}
}

// TestAddToolCatalog_StandaloneExclusion_LogsWhatItRemoved asserts the one
// line an operator gets about a standalone exclusion on the dynamic surface,
// which is where this projection runs.
//
// The pass over registered tools sees the two dynamic tools there, never a
// standalone utility, so its count reads zero whatever the exclusion did; a
// working exclusion was indistinguishable from a dead one until this line
// existed. The count is of what the exclusion names, judged before read-only
// mode removes anything, and nothing is logged when it names nothing.
//
// Sequential: the logger it captures is process-wide.
func TestAddToolCatalog_StandaloneExclusion_LogsWhatItRemoved(t *testing.T) {
	testCases := []struct {
		name     string
		opts     CatalogOptions
		wantLine string
	}{
		{name: "the group name counts every flow", opts: CatalogOptions{ExcludeToolNames: []string{"gitlab_interactive"}}, wantLine: `"excluded":4`},
		{name: "one canonical ID counts one", opts: CatalogOptions{ExcludeToolNames: []string{"discover_project.resolve"}}, wantLine: `"excluded":1`},
		{
			name:     "read-only mode does not hide what the exclusion named",
			opts:     CatalogOptions{ReadOnlyOnly: true, ExcludeToolNames: []string{"gitlab_interactive"}},
			wantLine: `"excluded":4`,
		},
		{name: "an entry naming nothing logs nothing", opts: CatalogOptions{ExcludeToolNames: []string{"gitlab_nope"}}},
		{name: "no exclusion logs nothing", opts: CatalogOptions{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logged := testutil.CaptureSlog(t)

			if _, err := AddToolCatalog(nil, StandaloneToolSpecs(newProjectionClient(t)), tc.opts); err != nil {
				t.Fatalf("AddToolCatalog() error = %v", err)
			}

			output := logged.String()
			if tc.wantLine == "" {
				if strings.Contains(output, "excluded standalone actions by configuration") {
					t.Errorf("log = %q, want no exclusion line when nothing was excluded", output)
				}
				return
			}
			if !strings.Contains(output, "excluded standalone actions by configuration") || !strings.Contains(output, tc.wantLine) {
				t.Errorf("log = %q, want the exclusion line with %s", output, tc.wantLine)
			}
		})
	}
}

// TestAddToolCatalog_GroupAlreadyPresent_NamesTheClashingAction asserts that a
// catalog that already holds one of the standalone group names is reported
// rather than silently merged, and that the report names the action the clash
// is about instead of only the tool.
func TestAddToolCatalog_GroupAlreadyPresent_NamesTheClashingAction(t *testing.T) {
	testCases := []struct {
		name string
		seed actioncatalog.Group
		want string
	}{
		{
			name: "discover project",
			seed: seedCatalogGroup("gitlab_discover_project", "resolve"),
			want: "add surface tool group gitlab_discover_project.resolve",
		},
		{
			name: "interactive",
			seed: seedCatalogGroup("gitlab_interactive", "issue_create"),
			want: "add surface tool group gitlab_interactive.issue_create",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := actioncatalog.NewCatalog()
			if err := catalog.AddGroup(tc.seed); err != nil {
				t.Fatalf("AddGroup(seed) error = %v", err)
			}
			got, err := AddToolCatalog(catalog, StandaloneToolSpecs(newProjectionClient(t)), CatalogOptions{})
			if err == nil {
				t.Fatalf("AddToolCatalog() error = nil, want a duplicate group report")
			}
			if got != nil {
				t.Errorf("AddToolCatalog() catalog = %v, want nil beside the error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("AddToolCatalog() error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestAddToolCatalog_TwoSpecsClaimingOneAction_ReportsTheBuildFailure asserts
// that two surface tools projecting into the same group under the same action
// name are refused. Either would answer to the one canonical ID, so admitting
// both would let whichever was projected last shadow the other silently.
func TestAddToolCatalog_TwoSpecsClaimingOneAction_ReportsTheBuildFailure(t *testing.T) {
	specs := []actioncatalog.SurfaceToolSpec{
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_first", "surface"),
		testSurfaceSpec("gitlab_test", "test", "gitlab_test_second", "surface"),
	}

	catalog, err := AddToolCatalog(nil, specs, CatalogOptions{})
	if err == nil {
		t.Fatalf("AddToolCatalog() error = nil, want a duplicate action report")
	}
	if catalog != nil {
		t.Errorf("AddToolCatalog() catalog = %v, want nil beside the error", catalog)
	}
	if !strings.Contains(err.Error(), "build surface tool group gitlab_test") {
		t.Errorf("AddToolCatalog() error = %v, want it to name the group it could not build", err)
	}
	if !strings.Contains(err.Error(), `duplicate action spec "surface"`) {
		t.Errorf("AddToolCatalog() error = %v, want it to name the duplicated action", err)
	}
}

// TestToolGroupSpecs_NoSpecs_ReturnsNoGroups asserts that an empty surface list
// produces no group at all rather than one empty group, which would reach the
// catalog as a dispatcher with nothing to dispatch.
func TestToolGroupSpecs_NoSpecs_ReturnsNoGroups(t *testing.T) {
	if got := ToolGroupSpecs(nil); got != nil {
		t.Fatalf("ToolGroupSpecs(nil) = %v, want nil", got)
	}
	if got := ToolGroupSpecs([]actioncatalog.SurfaceToolSpec{}); got != nil {
		t.Fatalf("ToolGroupSpecs(empty) = %v, want nil", got)
	}
}

// TestToolGroupSpecs_SeveralGroups_SortsByToolNameAndKeepsTheFirstMetadata
// asserts the three properties that make the projection deterministic: groups
// come back in tool-name order whatever order the specs arrived in, the actions
// inside one group keep the order their specs were given in, and a group's
// metadata is taken from the first spec that named it, so a later sibling
// cannot restate the group's owner, kind or capability requirement.
//
// It also asserts the read-only verdict per group, which is what --read-only
// filters on: a group is read-only only when every action in it is.
// TestStandaloneToolSpecs_EachGroup_IntroducesItselfAndNotItsFirstAction
// verifies that a group tool's description is the one written for the group.
//
// A group is assembled from its actions and keeps no record of its own, so
// ToolGroupSpecs builds its options from whichever spec it sees first. Until
// SurfaceToolSpec carried GroupDescription, what it took from that spec was the
// action's description, and the two are not the same sentence: gitlab_interactive
// introduced itself with 1650 characters about creating an issue while also
// creating merge requests, projects and releases, and a model reading tools/list
// was told the dispatcher does one of the four things it does.
//
// The assertion is that the group's text is the group's, and separately that it
// is not any action's, because a group whose first action happened to carry the
// right sentence would satisfy the first half alone.
func TestStandaloneToolSpecs_EachGroup_IntroducesItselfAndNotItsFirstAction(t *testing.T) {
	groups := ToolGroupSpecs(StandaloneToolSpecs(newProjectionClient(t)))

	want := map[string]string{
		"gitlab_discover_project": "Resolve a full git remote URL to a GitLab project and return its project_id and metadata. Read-only; use only for complete git remote URLs from .git/config or git remote -v.",
		"gitlab_interactive":      "Guided interactive creation flows for issues, merge requests, projects, and releases. Mutating; use only when the task explicitly asks for a guided flow.",
	}
	for _, group := range groups {
		t.Run(group.ToolName, func(t *testing.T) {
			wanted, named := want[group.ToolName]
			if !named {
				t.Fatalf("group %q is not in this test's table; add what it should say about itself", group.ToolName)
			}
			if group.Description != wanted {
				t.Errorf("group description = %q,\nwant %q", group.Description, wanted)
			}
			for _, action := range group.Actions {
				if action.IndividualTool.Description == group.Description {
					t.Errorf("group %q describes itself with action %q's own description; the group's sentence is not any one action's",
						group.ToolName, action.Name)
				}
			}
		})
	}
	if len(groups) != len(want) {
		t.Errorf("ToolGroupSpecs() published %d groups, want %d", len(groups), len(want))
	}
}

func TestToolGroupSpecs_SeveralGroups_SortsByToolNameAndKeepsTheFirstMetadata(t *testing.T) {
	definer := testSurfaceSpec("gitlab_alpha", "alpha", "gitlab_alpha_one", "one")
	sibling := testSurfaceSpec("gitlab_alpha", "alpha", "gitlab_alpha_two", "two")
	sibling.SurfaceKind = actioncatalog.SurfaceKindInteractiveUtility
	sibling.OwnerPackage = "somewhere_else"
	sibling.Icons = toolutil.IconConfig
	sibling.CapabilityRequirements = []string{"elicitation"}
	mutating := testSurfaceSpec("gitlab_zulu", "zulu", "gitlab_zulu_one", "one")
	mutating.ReadOnly = false

	// gitlab_zulu is given first and must still come back second.
	groups := ToolGroupSpecs([]actioncatalog.SurfaceToolSpec{mutating, definer, sibling})

	if len(groups) != 2 {
		t.Fatalf("ToolGroupSpecs() = %d groups, want 2", len(groups))
	}
	if groups[0].ToolName != "gitlab_alpha" || groups[1].ToolName != "gitlab_zulu" {
		t.Fatalf("group order = %q, %q, want gitlab_alpha, gitlab_zulu", groups[0].ToolName, groups[1].ToolName)
	}

	alpha := groups[0]
	if alpha.SurfaceKind != actioncatalog.SurfaceKindRuntimeUtility {
		t.Errorf("gitlab_alpha SurfaceKind = %q, want the first spec's %q", alpha.SurfaceKind, actioncatalog.SurfaceKindRuntimeUtility)
	}
	if alpha.OwnerPackage != "surfaces" {
		t.Errorf("gitlab_alpha OwnerPackage = %q, want the first spec's %q", alpha.OwnerPackage, "surfaces")
	}
	if len(alpha.CapabilityRequirements) != 0 {
		t.Errorf("gitlab_alpha CapabilityRequirements = %v, want none from the first spec", alpha.CapabilityRequirements)
	}
	if !alpha.ReadOnly {
		t.Error("gitlab_alpha ReadOnly = false, want true for a group of read-only actions")
	}
	if got := len(alpha.Actions); got != 2 {
		t.Fatalf("gitlab_alpha actions = %d, want 2", got)
	}
	if alpha.Actions[0].Name != "one" || alpha.Actions[1].Name != "two" {
		t.Errorf("gitlab_alpha actions = %q, %q, want one, two", alpha.Actions[0].Name, alpha.Actions[1].Name)
	}

	if groups[1].ReadOnly {
		t.Error("gitlab_zulu ReadOnly = true, want false for a group holding a mutating action")
	}
}

// TestToolGroupSpecs_SpecMissingMetadata_PanicsNamingTheSurface asserts that a
// surface spec the catalog cannot project stops the process instead of being
// skipped. These tools are part of the declared MCP surface, so a malformed one
// is a programming error that must fail startup rather than register a broken
// tool or quietly disappear from tools/list.
func TestToolGroupSpecs_SpecMissingMetadata_PanicsNamingTheSurface(t *testing.T) {
	spec := testSurfaceSpec("gitlab_test", "test", "gitlab_test_surface", "surface")
	spec.Description = ""

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("ToolGroupSpecs() returned normally, want a panic for an unprojectable spec")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("recover() = %v (%T), want an error", recovered, recovered)
		}
		if !strings.Contains(err.Error(), "project surface tool gitlab_test_surface") {
			t.Errorf("panic = %v, want it to name the surface tool", err)
		}
		if !strings.Contains(err.Error(), "description is required") {
			t.Errorf("panic = %v, want it to name the missing metadata", err)
		}
	}()

	ToolGroupSpecs([]actioncatalog.SurfaceToolSpec{spec})
}

// TestSurfaceGroupActionLabel_GroupContents_NameTheFirstActionWhenThereIsOne
// asserts the label a duplicate-group report is written with: the tool name
// plus its first action in deterministic order, whatever order the actions were
// set in.
//
// The empty case cannot arrive through [AddToolCatalog], because a group is
// only built from specs and every spec contributes an action; it is asserted
// here directly so the fallback is known to name the tool rather than emit a
// dangling "tool." prefix.
func TestSurfaceGroupActionLabel_GroupContents_NameTheFirstActionWhenThereIsOne(t *testing.T) {
	withActions := seedCatalogGroup("gitlab_test", "resolve")
	withActions.SetAction(actioncatalog.Action{Name: "archive", Route: toolutil.ActionRoute{
		Handler:     func(context.Context, map[string]any) (any, error) { return surfaceTestOutput{}, nil },
		InputSchema: map[string]any{"type": "object"},
	}})

	testCases := []struct {
		name  string
		group actioncatalog.Group
		want  string
	}{
		{
			name:  "actions are labeled by the first in order",
			group: withActions,
			want:  "gitlab_test.archive",
		},
		{
			name:  "no actions falls back to the tool name",
			group: actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_test"}),
			want:  "gitlab_test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := surfaceGroupActionLabel(tc.group); got != tc.want {
				t.Fatalf("surfaceGroupActionLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReadOnlyGroup_NoActions_ReportsNotReadOnly asserts the defensive answer
// for a group with nothing in it. Every group this package projects carries at
// least one action, so the case is unreachable through [ToolGroupSpecs]; it is
// asserted directly because the safe answer for "nothing to judge" is not
// read-only, and the opposite would admit an unvetted group into a read-only
// surface.
func TestReadOnlyGroup_NoActions_ReportsNotReadOnly(t *testing.T) {
	if readOnlyGroup(nil) {
		t.Error("readOnlyGroup(nil) = true, want false")
	}
	if readOnlyGroup([]toolutil.ActionSpec{}) {
		t.Error("readOnlyGroup(empty) = true, want false")
	}
}

// TestStandaloneToolSpecs_EverySurface_RepublishesItsHistoricalActionAliases
// asserts that each standalone surface carries the historical action aliases
// declared for its canonical ID, aimed at its own action.
//
// Neither projectdiscovery nor elicitationtools declares any compatibility of
// its own, so every alias these five tools publish arrives through the single
// [actioncompat.ApplyToActionSpecs] call the projection makes. That call and
// the field copy carrying its result onto the surface spec were observed by
// nothing: removing either leaves every other assertion in this file passing
// while `gitlab_interactive_mr_create` and the six names beside it stop
// resolving, so a model holding an older ID is told the action does not exist
// rather than being routed to the one that replaced it.
func TestStandaloneToolSpecs_EverySurface_RepublishesItsHistoricalActionAliases(t *testing.T) {
	// Driven from the declarations rather than from what the projection
	// produced, because the produced side is what a rename breaks: iterating it
	// lets a canonical ID that stopped being published go unmentioned, while
	// the other six keep any count of checked aliases above zero and the test
	// passes reporting nothing.
	byCanonical := make(map[string]actioncatalog.SurfaceToolSpec)
	domains := make(map[string]bool)
	for _, spec := range StandaloneToolSpecs(newProjectionClient(t)) {
		byCanonical[spec.BaseDomain+"."+spec.ActionName] = spec
		domains[spec.BaseDomain] = true
	}

	declared := declaredActionAliases()
	wanted := declaredIDsForDomains(t, declared, domains)
	for _, canonicalID := range wanted {
		t.Run(canonicalID, func(t *testing.T) {
			spec, published := byCanonical[canonicalID]
			if !published {
				t.Fatalf("no standalone surface publishes %q, which aliases are declared for; it was renamed or dropped, and a model holding an older ID is now told the action does not exist",
					canonicalID)
			}
			assertCarriesDeclaredAliases(t, spec.Compatibility, canonicalID, spec.ActionName, declared)
		})
	}
}

// declaredIDsForDomains returns the canonical IDs declared for the given base
// domains, sorted, and fails the test if there are none.
//
// The declarations cover the whole tree, and these tests are about the handful
// of standalone surfaces, so the set has to be narrowed by domain rather than
// by what the projection produced: narrowing by the produced side is what lets
// a renamed action go unnoticed, since the survivors keep any count above zero.
func declaredIDsForDomains(t *testing.T, declared map[string][]actioncompat.ActionAlias, domains map[string]bool) []string {
	t.Helper()
	var ids []string
	for canonicalID := range declared {
		domain, _, found := strings.Cut(canonicalID, ".")
		if found && domains[domain] {
			ids = append(ids, canonicalID)
		}
	}
	if len(ids) == 0 {
		t.Fatalf("no alias is declared for any of the standalone domains %v, so this test would assert nothing",
			slices.Sorted(maps.Keys(domains)))
	}
	slices.Sort(ids)
	return ids
}

// TestAddToolCatalog_ProjectedActions_KeepTheHistoricalActionAliases asserts
// that those aliases survive the rest of the trip into the catalog, which is
// where the dynamic surface reads them from.
//
// The spec-level assertion above cannot see this half: the projection clones
// each spec, converts it to an action spec and rebuilds it as a catalog group,
// and a compatibility policy dropped at any of those steps would leave the
// surface spec correct and the catalog a caller actually queries without the
// alias.
func TestAddToolCatalog_ProjectedActions_KeepTheHistoricalActionAliases(t *testing.T) {
	catalog, err := AddToolCatalog(nil, StandaloneToolSpecs(newProjectionClient(t)), CatalogOptions{})
	if err != nil {
		t.Fatalf("AddToolCatalog() error = %v", err)
	}

	// Driven from the declarations, for the reason given on the test above.
	byCanonical := make(map[string]actioncatalog.Action)
	domains := make(map[string]bool)
	for _, action := range catalog.Actions() {
		byCanonical[string(action.ID)] = action
	}
	for _, spec := range StandaloneToolSpecs(newProjectionClient(t)) {
		domains[spec.BaseDomain] = true
	}

	declared := declaredActionAliases()
	for _, canonicalID := range declaredIDsForDomains(t, declared, domains) {
		t.Run(canonicalID, func(t *testing.T) {
			action, projected := byCanonical[canonicalID]
			if !projected {
				t.Fatalf("the catalog holds no action %q, which aliases are declared for; the dynamic surface a caller queries cannot resolve them",
					canonicalID)
			}
			assertCarriesDeclaredAliases(t, action.Compatibility, canonicalID, action.Name, declared)
		})
	}
}
