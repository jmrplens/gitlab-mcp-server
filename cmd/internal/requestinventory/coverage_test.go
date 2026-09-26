package requestinventory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// makeToolsPackage creates internal/tools/<name> under root, which is how the
// classification tells an owner that recorded nothing from an owner that is no
// package at all.
func makeToolsPackage(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(ToolsDir), name), 0o750); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", name, err)
	}
}

// TestClassify_Owners_AreSplitThreeWays verifies the three counts stay
// disjoint and mean what they say: an owner that recorded something, one that
// exists and recorded nothing, and one that is not a package under
// internal/tools at all.
//
// Every count is given a value no other count shares, since two that agree in
// the fixture can be exchanged in the code without a test noticing, and the
// whole record is compared so an owner filed under the wrong list shows too.
// The unmapped owner shares its name with a package that recorded a request
// outside internal/tools, which is not an owner the catalog can mean.
func TestClassify_Owners_AreSplitThreeWays(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	makeToolsPackage(t, root, "adminspecs")
	rows := []Row{
		{Package: "internal/tools/issues", Path: "/projects/:id/issues"},
		{Package: "internal/completions", Path: "/groups"},
	}
	actions := []Action{
		{ID: "issue.list", Owner: "issues"},
		{ID: "topic.list", Owner: "adminspecs"},
		{ID: "issue.get", Owner: "issues"},
		{ID: "completion.list", Owner: "completions"},
		{ID: "topic.get", Owner: "adminspecs"},
		{ID: "issue.create", Owner: "issues"},
	}

	coverage := Classify(root, rows, actions)

	want := Coverage{
		Total:          6,
		Covered:        3,
		Silent:         2,
		Unmapped:       1,
		SilentOwners:   []Owner{{Package: "adminspecs", Actions: []string{"topic.get", "topic.list"}}},
		UnmappedOwners: []Owner{{Package: "completions", Actions: []string{"completion.list"}}},
	}
	if !reflect.DeepEqual(coverage, want) {
		t.Errorf("coverage = %+v, want %+v", coverage, want)
	}
}

// TestClassify_TheRootOwner_ResolvesToTheOrchestrationPackage verifies the one
// owner that is not a directory under internal/tools.
//
// The catalog gives a spec group that declares none the owner "tools", and
// TestCollectedActionSpecs_DeclareCatalogOwnership admits it beside the domain
// names, so it has to resolve to internal/tools itself. Calling it unmapped
// would report a catalog-metadata defect that is not one, and the gate now
// fails on unmapped owners.
func TestClassify_TheRootOwner_ResolvesToTheOrchestrationPackage(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")

	t.Run("covered when the root package recorded something", func(t *testing.T) {
		coverage := Classify(root,
			[]Row{{Package: ToolsDir, Path: "/projects/:project_id"}},
			[]Action{{ID: "discover.project", Owner: RootOwner}})

		want := Coverage{Total: 1, Covered: 1, SilentOwners: []Owner{}, UnmappedOwners: []Owner{}}
		if !reflect.DeepEqual(coverage, want) {
			t.Errorf("coverage = %+v, want the root owner covered: %+v", coverage, want)
		}
	})

	t.Run("silent, not unmapped, when it recorded nothing", func(t *testing.T) {
		coverage := Classify(root,
			[]Row{{Package: "internal/tools/issues", Path: "/projects/:project_id/issues"}},
			[]Action{{ID: "discover.project", Owner: RootOwner}})

		want := Coverage{
			Total:          1,
			Silent:         1,
			SilentOwners:   []Owner{{Package: RootOwner, Actions: []string{"discover.project"}}},
			UnmappedOwners: []Owner{},
		}
		if !reflect.DeepEqual(coverage, want) {
			t.Errorf("coverage = %+v, want the root owner silent rather than unmapped: %+v", coverage, want)
		}
	})
}

// TestRootOwner_IsWhatTheCatalogCallsAGroupThatNamesNoOwner verifies the
// spelling [RootOwner] claims against the catalog that assigns it.
//
// The classification tests spell the root owner with this constant on both
// sides, so any value of it passes them, and no action of the real catalog is
// owned by the orchestration package today, so the real catalog cannot pin it
// either. A group that declares no owner is the case the constant exists for,
// so one is added here: were the two spellings to part, the first such group
// the catalog gained would be counted as owned by nothing, and the gate would
// fail on a metadata defect that is not one.
func TestRootOwner_IsWhatTheCatalogCallsAGroupThatNamesNoOwner(t *testing.T) {
	probe := tools.ActionSpecGroup{
		ToolName:   "gitlab_requestinventory_probe",
		BaseDomain: "requestinventoryprobe",
		Actions: []toolutil.ActionSpec{toolutil.NewActionSpec("probe", toolutil.RouteAction(nil,
			func(context.Context, *gitlabclient.Client, struct{}) (struct{}, error) { return struct{}{}, nil }),
			toolutil.ActionSpecOptions{ReadOnly: true})},
	}
	original := buildCatalog
	buildCatalog = func(client *gitlabclient.Client, opts tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		opts.SpecGroups = []tools.ActionSpecGroup{probe}
		return original(client, opts)
	}
	t.Cleanup(func() { buildCatalog = original })

	actions, err := Actions()
	if err != nil {
		t.Fatalf("Actions() error = %v", err)
	}
	found := slices.IndexFunc(actions, func(action Action) bool { return action.ID == "requestinventoryprobe.probe" })
	if found < 0 {
		t.Fatalf("the catalog holds no requestinventoryprobe.probe among its %d actions", len(actions))
	}
	if owner := actions[found].Owner; owner != RootOwner {
		t.Fatalf("the catalog gave a group that names no owner the owner %q, and RootOwner spells it %q", owner, RootOwner)
	}
	coverage := Classify(repoRoot(t), nil, actions[found:found+1])
	if coverage.Silent != 1 || coverage.Unmapped != 0 {
		t.Errorf("coverage = %+v, want the action resolved to this repository's %s and counted silent", coverage, ToolsDir)
	}
}

// TestClassify_SeveralOwnersAndActions_AreSortedForAStableReport verifies the
// order, since a report that reshuffles between runs turns every audit into a
// diff nobody can read.
//
// The owners are handed over in reverse order, and there are three of them: a
// map this small tends to iterate in the order its keys went in, turned at a
// random point, and with two owners a report that forgot to sort came back
// sorted in 5 runs of 40. No turn of three keys in reverse order is sorted.
func TestClassify_SeveralOwnersAndActions_AreSortedForAStableReport(t *testing.T) {
	root := t.TempDir()

	coverage := Classify(root, nil, []Action{
		{ID: "zeta.get", Owner: "third"},
		{ID: "alpha.get", Owner: "third"},
		{ID: "gamma.get", Owner: "second"},
		{ID: "beta.get", Owner: "first"},
	})

	if !slices.Equal(Packages(coverage.UnmappedOwners), []string{"first", "second", "third"}) {
		t.Fatalf("owners = %v, want them sorted", Packages(coverage.UnmappedOwners))
	}
	if !slices.Equal(coverage.UnmappedOwners[2].Actions, []string{"alpha.get", "zeta.get"}) {
		t.Errorf("actions = %v, want them sorted", coverage.UnmappedOwners[2].Actions)
	}
}

// TestActions_TheRealCatalog_NamesAnOwnerForEveryAction verifies the default
// implementation against the catalog this binary carries: every action has an
// owning package, or every count built on it is counting something else.
func TestActions_TheRealCatalog_NamesAnOwnerForEveryAction(t *testing.T) {
	actions, err := Actions()
	if err != nil {
		t.Fatalf("Actions() error = %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("Actions() returned nothing")
	}
	for _, action := range actions {
		if action.Owner == "" {
			t.Fatalf("action %q of the %d in the catalog has no owning package", action.ID, len(actions))
		}
	}
}

// TestActions_TheRealCatalog_CarriesTheRouteOfEveryAction verifies the half of
// an action the pagination rule reads. A route with no output type publishes
// nothing a comparison can see, so an action arriving without one would be
// skipped silently rather than judged, and the catalog is where that has to
// hold rather than at the reader.
func TestActions_TheRealCatalog_CarriesTheRouteOfEveryAction(t *testing.T) {
	actions, err := Actions()
	if err != nil {
		t.Fatalf("Actions() error = %v", err)
	}
	withOutput := 0
	for _, action := range actions {
		if action.Route.OutputType != nil {
			withOutput++
		}
	}
	if withOutput == 0 {
		t.Fatalf("no action of the %d in the catalog carried an output type", len(actions))
	}
}

// TestPackageName_Owner_IsSpelledTheWayARowSpellsIt verifies the join key
// itself. The inventory records a package path and the catalog records an owner
// name, and this is the one place the two are made to meet; getting the root
// owner wrong here would silently detach every action the orchestration package
// owns from the requests it made.
func TestPackageName_Owner_IsSpelledTheWayARowSpellsIt(t *testing.T) {
	cases := []struct {
		owner string
		want  string
	}{
		{owner: "issues", want: ToolsDir + "/issues"},
		{owner: RootOwner, want: ToolsDir},
	}
	for _, testCase := range cases {
		t.Run(testCase.owner, func(t *testing.T) {
			if got := PackageName(testCase.owner); got != testCase.want {
				t.Errorf("PackageName(%q) = %q, want %q", testCase.owner, got, testCase.want)
			}
		})
	}
}

// TestActions_EveryField_ComesFromTheCatalogFieldItNames verifies the
// projection against a catalog built here, where no two values agree.
//
// Nothing else holds it: the two tests above read the real catalog and ask only
// whether a field is non-empty, which an action carrying its owner as its ID
// and its ID as its owner answers just as well. That exchange would leave every
// action classified unmapped, since no dotted ID is a directory, and the tier
// asked for is invisible the same way: a catalog built at Free would count a
// narrower surface and read as complete. ReadOnly is the third: nothing reads
// it here, and R-PAGE judges pagination on reads alone, so one stuck at false
// empties that comparison in silence. The route is compared whole, since
// R-PAGE reads its input schema as well as its output type.
func TestActions_EveryField_ComesFromTheCatalogFieldItNames(t *testing.T) {
	readRoute := toolutil.ActionRoute{
		InputType:   reflect.TypeFor[Row](),
		OutputType:  reflect.TypeFor[Coverage](),
		InputSchema: map[string]any{"type": "object", "title": "read input"},
	}
	writeRoute := toolutil.ActionRoute{
		InputType:   reflect.TypeFor[Inventory](),
		OutputType:  reflect.TypeFor[Owner](),
		InputSchema: map[string]any{"type": "object", "title": "write input"},
	}
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{
		ToolName:   "gitlab_fixture",
		BaseDomain: "fixture",
	})
	group.SetAction(actioncatalog.Action{
		Name:         "read",
		ReadOnly:     true,
		OwnerPackage: "readerpkg",
		Route:        readRoute,
	})
	group.SetAction(actioncatalog.Action{
		Name:         "write",
		ReadOnly:     false,
		OwnerPackage: "writerpkg",
		Route:        writeRoute,
	})
	catalog := actioncatalog.NewCatalog()
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup error = %v", err)
	}

	var asked tools.ActionCatalogOptions
	original := buildCatalog
	buildCatalog = func(_ *gitlabclient.Client, opts tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		asked = opts
		return catalog, nil
	}
	t.Cleanup(func() { buildCatalog = original })

	actions, err := Actions()
	if err != nil {
		t.Fatalf("Actions() error = %v", err)
	}

	want := []Action{
		{ID: "fixture.read", Owner: "readerpkg", ReadOnly: true, Route: readRoute},
		{ID: "fixture.write", Owner: "writerpkg", ReadOnly: false, Route: writeRoute},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Errorf("Actions() = %+v, want %+v", actions, want)
	}
	if asked.Tier != edition.Ultimate {
		t.Errorf("the catalog was asked for at tier %q, want %q so the counts are of the whole surface", asked.Tier, edition.Ultimate)
	}
}

// TestActions_ACatalogThatWillNotBuild_IsReported verifies that a caller is
// told rather than handed an empty action list, which would score every
// package as covering nothing it owns, and told what the catalog said, since
// that is the line the generator prints in place of its coverage count.
func TestActions_ACatalogThatWillNotBuild_IsReported(t *testing.T) {
	broken := errors.New("catalog is broken")
	original := buildCatalog
	buildCatalog = func(*gitlabclient.Client, tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return nil, broken
	}
	t.Cleanup(func() { buildCatalog = original })

	actions, err := Actions()

	if !errors.Is(err, broken) {
		t.Fatalf("Actions() error = %v, want the catalog's own failure", err)
	}
	if actions != nil {
		t.Errorf("Actions() = %v, want nothing on failure", actions)
	}
}

// TestDirectoryExists_Path_IsADirectoryOrNot verifies the check that tells an
// owner package that recorded nothing from a name that is no package.
func TestDirectoryExists_Path_IsADirectoryOrNot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"a directory", root, true},
		{"a regular file", file, false},
		{"nothing at all", filepath.Join(root, "absent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := directoryExists(tt.path); got != tt.want {
				t.Errorf("directoryExists(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}
