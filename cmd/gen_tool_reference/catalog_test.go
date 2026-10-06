package main

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// schemaOf builds an input schema with the named properties, the ones in
// required listed as required.
func schemaOf(required []string, names ...string) map[string]any {
	props := map[string]any{}
	for _, name := range names {
		props[name] = map[string]any{"type": "string", "description": name + " value"}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// testGrants is the table the synthetic actions' rows point into: one read
// of a widget, granted at project, as GitLab 19.4.1 records it.
func testGrants() *finegrained.Table {
	return &finegrained.Table{
		Version:     "19.4.1-ee",
		Permissions: []string{"read_widget"},
		Displays:    []string{"", "Widget: Read"},
		Display:     []uint16{1},
		Groups:      []finegrained.Group{{Perms: []uint16{0}, Any: finegrained.BoundaryProject}},
		Operations:  []finegrained.Operation{{Name: "GET /widgets", Groups: []uint32{0}, Classic: finegrained.ClassicReadAPI}},
	}
}

// testAction is an action named name whose individual tool is tool, with the
// changes edit makes. Unless edit gives it a row of its own, its row in the
// grants table reads a widget and needs read_api when the action reads and
// api when it writes, which is what the generator derives for most actions.
func testAction(name, tool string, edit func(*actioncatalog.Action)) actioncatalog.Action {
	action := actioncatalog.Action{
		Name:           name,
		Route:          toolutil.ActionRoute{InputSchema: schemaOf(nil)},
		IndividualTool: toolutil.IndividualToolSpec{Name: tool, Description: "Does " + name + "."},
	}
	if edit != nil {
		edit(&action)
	}
	if action.FineGrained == nil {
		classic := finegrained.ClassicAPI
		if action.ReadOnly {
			classic = finegrained.ClassicReadAPI
		}
		action.FineGrained = &finegrained.Requirement{Classic: classic, Paths: [][]uint32{{0}}}
	}
	return action
}

// testGroup is a group named tool of kind holding actions.
func testGroup(tool string, kind actioncatalog.SurfaceKind, capabilities []string, actions ...actioncatalog.Action) actioncatalog.Group {
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: tool, SurfaceKind: kind, CapabilityRequirements: capabilities})
	for _, action := range actions {
		group.SetAction(action)
	}
	return group
}

// testCatalog assembles a catalog of groups, failing the test on any error.
func testCatalog(t *testing.T, groups ...actioncatalog.Group) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	for _, group := range groups {
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup(%s) error = %v", group.ToolName, err)
		}
	}
	return catalog
}

// widgetList, widgetCreate and widgetGraph are the actions of the synthetic
// widget group: one served everywhere, one from Premium that gains a
// parameter at Ultimate, and one GitLab.com serves alone from Premium.
func widgetList() actioncatalog.Action {
	return testAction("list", "gitlab_widget_list", func(a *actioncatalog.Action) {
		a.ReadOnly, a.Idempotent = true, true
		a.Route.InputSchema = schemaOf([]string{"project_id"}, "project_id", "page")
		a.IndividualTool.Description = "List widgets. See also: gitlab_widget_create, gitlab_unknown_tool."
	})
}

func widgetCreate(withWeight bool) actioncatalog.Action {
	return testAction("create", "gitlab_widget_create", func(a *actioncatalog.Action) {
		a.Destructive = true
		a.Route.InputSchema = schemaOf([]string{"project_id"}, "project_id")
		if withWeight {
			a.Route.InputSchema = schemaOf([]string{"project_id"}, "project_id", "weight")
		}
	})
}

func widgetGraph() actioncatalog.Action {
	return testAction("graph", "gitlab_widget_graph", func(a *actioncatalog.Action) {
		a.ReadOnly = true
		a.IndividualTool.Description = ""
		a.Usage = "Reads the graph."
	})
}

// helperResolve is the one action of the synthetic standalone group.
func helperResolve() actioncatalog.Action {
	return testAction("resolve", "gitlab_helper_resolve", func(a *actioncatalog.Action) {
		a.ReadOnly, a.Idempotent = true, true
	})
}

// testBuilds are six synthetic builds in the order defaultBuilds makes them.
func testBuilds(t *testing.T) []build {
	t.Helper()
	helper := func() actioncatalog.Group {
		return testGroup("gitlab_helper", actioncatalog.SurfaceKindRuntimeUtility, []string{"elicitation"}, helperResolve())
	}
	free := func() *actioncatalog.Catalog {
		return testCatalog(t, testGroup("gitlab_widget", actioncatalog.SurfaceKindMetaGroup, nil, widgetList()), helper())
	}
	premium := func(dotcom, weight bool) *actioncatalog.Catalog {
		actions := []actioncatalog.Action{widgetList(), widgetCreate(weight)}
		if dotcom {
			actions = append(actions, widgetGraph())
		}
		return testCatalog(t, testGroup("gitlab_widget", actioncatalog.SurfaceKindMetaGroup, nil, actions...), helper())
	}
	return []build{
		{dotcom: false, tier: edition.Free, catalog: free()},
		{dotcom: true, tier: edition.Free, catalog: free()},
		{dotcom: false, tier: edition.Premium, catalog: premium(false, false)},
		{dotcom: true, tier: edition.Premium, catalog: premium(true, false)},
		{dotcom: false, tier: edition.Ultimate, catalog: premium(false, true)},
		{dotcom: true, tier: edition.Ultimate, catalog: premium(true, true)},
	}
}

// testSurfaces registers every tool the synthetic builds name.
func testSurfaces() surfaceNames {
	return surfaceNames{
		meta:       map[string]bool{"gitlab_widget": true, "gitlab_helper_resolve": true},
		individual: map[string]bool{"gitlab_widget_list": true, "gitlab_widget_create": true, "gitlab_widget_graph": true, "gitlab_helper_resolve": true},
	}
}

// testReference assembles the synthetic builds, failing the test on an error.
func testReference(t *testing.T) reference {
	t.Helper()
	ref, err := assemble(testBuilds(t), testSurfaces(), map[string][]string{"gitlab_widget": {"admin_mode"}}, testGrants(), nil)
	if err != nil {
		t.Fatalf("assemble() error = %v", err)
	}
	return ref
}

// actionByID finds an assembled action, failing the test when it is absent.
func actionByID(t *testing.T, ref reference, id string) *refAction {
	t.Helper()
	for _, group := range ref.groups {
		for _, action := range group.actions {
			if action.id == id {
				return action
			}
		}
	}
	t.Fatalf("no action %s in the reference", id)
	return nil
}

func TestAssemble_SyntheticBuilds_FoldsGroupsInNameOrder(t *testing.T) {
	ref := testReference(t)
	var tools, slugs []string
	for _, group := range ref.groups {
		tools = append(tools, group.tool)
		slugs = append(slugs, group.slug)
	}
	if !slices.Equal(tools, []string{"gitlab_helper", "gitlab_widget"}) {
		t.Errorf("group tools = %v, want gitlab_helper then gitlab_widget", tools)
	}
	if !slices.Equal(slugs, []string{"helper", "widget"}) {
		t.Errorf("group slugs = %v", slugs)
	}
	helper, widget := ref.groups[0], ref.groups[1]
	if !helper.standalone || widget.standalone {
		t.Errorf("standalone = %t, %t, want true for the runtime utility only", helper.standalone, widget.standalone)
	}
	if !slices.Equal(widget.scopes, []string{"admin_mode"}) || helper.scopes != nil {
		t.Errorf("scopes = %v, %v", widget.scopes, helper.scopes)
	}
	if !slices.Equal(helper.capabilities, []string{"elicitation"}) {
		t.Errorf("capabilities = %v", helper.capabilities)
	}
	var ids []string
	for _, action := range widget.actions {
		ids = append(ids, action.id)
	}
	if !slices.Equal(ids, []string{"widget.create", "widget.graph", "widget.list"}) {
		t.Errorf("widget actions = %v, want them in ID order", ids)
	}
	if got := ref.actionCount(); got != 4 {
		t.Errorf("actionCount() = %d, want 4", got)
	}
}

func TestAssemble_SyntheticBuilds_CountsWhatEachBuildServes(t *testing.T) {
	ref := testReference(t)
	widget := ref.groups[1]
	want := [2][3]int{{1, 2, 2}, {1, 3, 3}}
	if widget.served != want {
		t.Errorf("widget served = %v, want %v", widget.served, want)
	}
	if wantTotal := [2][3]int{{2, 3, 3}, {2, 4, 4}}; ref.served != wantTotal {
		t.Errorf("reference served = %v, want %v", ref.served, wantTotal)
	}
}

func TestAssemble_SyntheticBuilds_TierIsTheLowestBuildThatServes(t *testing.T) {
	ref := testReference(t)
	tests := []struct {
		id         string
		tier       edition.Tier
		dotcomOnly bool
	}{
		{id: "widget.list", tier: edition.Free},
		{id: "widget.create", tier: edition.Premium},
		{id: "widget.graph", tier: edition.Premium, dotcomOnly: true},
		{id: "helper.resolve", tier: edition.Free},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			action := actionByID(t, ref, tt.id)
			if action.tier != tt.tier || action.dotcomOnly != tt.dotcomOnly {
				t.Errorf("tier, dotcomOnly = %v, %t, want %v, %t", action.tier, action.dotcomOnly, tt.tier, tt.dotcomOnly)
			}
		})
	}
}

func TestAssemble_SyntheticBuilds_NamesEachToolAndParameterTier(t *testing.T) {
	ref := testReference(t)
	create := actionByID(t, ref, "widget.create")
	if create.metaTool != "gitlab_widget" || create.name != "create" || create.individual != "gitlab_widget_create" || create.standalone {
		t.Errorf("create tools = %q %q %q %t", create.metaTool, create.name, create.individual, create.standalone)
	}
	if !create.destructive || create.readOnly || create.idempotent {
		t.Errorf("create annotations = destructive %t, read-only %t, idempotent %t", create.destructive, create.readOnly, create.idempotent)
	}
	var tiersByName []string
	for _, p := range create.params {
		tiersByName = append(tiersByName, p.name+"="+tierNames[p.tier])
	}
	if !slices.Equal(tiersByName, []string{"project_id=Premium", "weight=Ultimate"}) {
		t.Errorf("create params = %v, want the weight served from Ultimate only", tiersByName)
	}
	resolve := actionByID(t, ref, "helper.resolve")
	if resolve.metaTool != "gitlab_helper_resolve" || !resolve.standalone {
		t.Errorf("standalone meta tool = %q (standalone %t), want the action's own tool", resolve.metaTool, resolve.standalone)
	}
	list := actionByID(t, ref, "widget.list")
	if !list.readOnly || !list.idempotent {
		t.Errorf("list annotations = read-only %t, idempotent %t", list.readOnly, list.idempotent)
	}
	if want := "List widgets. See also: widget.create."; list.description != want {
		t.Errorf("list description = %q, want %q", list.description, want)
	}
	if graph := actionByID(t, ref, "widget.graph"); graph.description != "Reads the graph." {
		t.Errorf("graph description = %q, want the usage line", graph.description)
	}
}

func TestAssemble_UnregisteredTool_IsRefused(t *testing.T) {
	tests := []struct {
		name    string
		surface func(surfaceNames)
		want    string
	}{
		{name: "meta tool", surface: func(s surfaceNames) { delete(s.meta, "gitlab_widget") }, want: `the meta surface registers no tool "gitlab_widget"`},
		{name: "standalone tool", surface: func(s surfaceNames) { delete(s.meta, "gitlab_helper_resolve") }, want: `the meta surface registers no tool "gitlab_helper_resolve"`},
		{name: "individual tool", surface: func(s surfaceNames) { delete(s.individual, "gitlab_widget_graph") }, want: `the individual surface registers no tool "gitlab_widget_graph"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			surfaces := testSurfaces()
			tt.surface(surfaces)
			_, err := assemble(testBuilds(t), surfaces, nil, testGrants(), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("assemble() error = %v, want it to say %s", err, tt.want)
			}
		})
	}
}

func TestAssemble_ActionWithoutIndividualTool_IsServedOnTheOtherSurfaces(t *testing.T) {
	probe := testAction("probe", "", nil)
	catalog := testCatalog(t, testGroup("gitlab_widget", actioncatalog.SurfaceKindMetaGroup, nil, probe))
	ref, err := assemble([]build{{tier: edition.Free, catalog: catalog}}, surfaceNames{meta: map[string]bool{"gitlab_widget": true}}, nil, testGrants(), nil)
	if err != nil {
		t.Fatalf("assemble() error = %v", err)
	}
	if got := ref.groups[0].actions[0].individual; got != "" {
		t.Errorf("individual = %q, want none", got)
	}
}

// TestAssemble_ActionTheGrantsTableHoldsNoClassicScopeFor_IsRefused verifies
// an action whose row is missing, or holds no classic scope, stops the run
// and says which command fills it in, rather than printing a token line that
// would be a guess: the catalog gains an action before the table is
// regenerated for it, and the pages are generated from both.
func TestAssemble_ActionTheGrantsTableHoldsNoClassicScopeFor_IsRefused(t *testing.T) {
	cases := []struct {
		name string
		row  *finegrained.Requirement
	}{
		{name: "no row"},
		{name: "unknown scope", row: &finegrained.Requirement{Paths: [][]uint32{{0}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := actioncatalog.Action{
				Name:           "probe",
				Route:          toolutil.ActionRoute{InputSchema: schemaOf(nil)},
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_widget_probe"},
				FineGrained:    tc.row,
			}
			catalog := testCatalog(t, testGroup("gitlab_widget", actioncatalog.SurfaceKindMetaGroup, nil, probe))
			surfaces := surfaceNames{meta: map[string]bool{"gitlab_widget": true}, individual: map[string]bool{"gitlab_widget_probe": true}}
			_, err := assemble([]build{{tier: edition.Free, catalog: catalog}}, surfaces, nil, testGrants(), nil)
			if err == nil || !strings.Contains(err.Error(), "widget.probe: the action grants table holds no classic scope for it; run make gen-action-grants first") {
				t.Errorf("assemble() error = %v, want the action named and the generator to run", err)
			}
		})
	}
}

// TestAssemble_SyntheticBuilds_ReadsEachActionsTokenRequirements verifies an
// action carries what its row says a token needs: the classic scope, the
// scopes its group demands beside it, the fine-grained description of its
// row in the table, and the domain it is filed under on the permissions page,
// and the reference carries the release the table was recorded from.
func TestAssemble_SyntheticBuilds_ReadsEachActionsTokenRequirements(t *testing.T) {
	ref := testReference(t)
	if ref.version != "19.4.1" {
		t.Errorf("version = %q, want the table's release without its edition", ref.version)
	}
	tests := []struct {
		id      string
		classic finegrained.ClassicScope
		scopes  []string
		domain  string
	}{
		{id: "widget.list", classic: finegrained.ClassicReadAPI, scopes: []string{"admin_mode"}, domain: "widget"},
		{id: "widget.create", classic: finegrained.ClassicAPI, scopes: []string{"admin_mode"}, domain: "widget"},
		{id: "helper.resolve", classic: finegrained.ClassicReadAPI, domain: "helper"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			action := actionByID(t, ref, tt.id)
			if action.classic != tt.classic || !slices.Equal(action.scopes, tt.scopes) || action.domain != tt.domain {
				t.Errorf("classic, scopes, domain = %s, %v, %q, want %s, %v, %q", action.classic, action.scopes, action.domain, tt.classic, tt.scopes, tt.domain)
			}
			if action.fineGrained == nil || len(action.fineGrained.AnyOf) != 1 || action.fineGrained.AnyOf[0].Needs[0].Permissions[0] != "Widget: Read" {
				t.Errorf("fineGrained = %+v, want the widget read the row names", action.fineGrained)
			}
		})
	}
}

func TestClassIndex_EachInstanceClass_HasItsOwnIndex(t *testing.T) {
	if classIndex(false) != 0 || classIndex(true) != 1 {
		t.Errorf("classIndex = %d, %d, want 0 for self-managed and 1 for GitLab.com", classIndex(false), classIndex(true))
	}
}

func TestDefaultBuilds_CatalogFails_NamesTheBuild(t *testing.T) {
	original := buildCatalog
	t.Cleanup(func() { buildCatalog = original })
	buildCatalog = func(*gitlabclient.Client, *config.ServerConfig) (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
		return nil, gitlabtools.WithheldActions{}, errors.New("boom")
	}
	_, err := defaultBuilds()
	if err == nil || !strings.Contains(err.Error(), "build the free catalog (GitLab.com false): boom") {
		t.Errorf("defaultBuilds() error = %v, want the failing build named", err)
	}
}

// realReference is the reference of the catalog this binary builds, made
// once: building it costs seconds, and every test that needs it only reads it.
var realReference = sync.OnceValues(func() (reference, error) {
	builds, err := defaultBuilds()
	if err != nil {
		return reference{}, err
	}
	return assemble(builds, defaultSurfaces(), gitlabtools.MetaToolScopes, actiongrants.Table(), oauthRefusedRoutes)
})

// mustRealReference is realReference, failing the test on an error.
func mustRealReference(t *testing.T) reference {
	t.Helper()
	ref, err := realReference()
	if err != nil {
		t.Fatalf("real reference: %v", err)
	}
	return ref
}

func TestDefaultBuilds_RealCatalog_BuildsEachClassAtEachTierInOrder(t *testing.T) {
	builds, err := defaultBuilds()
	if err != nil {
		t.Fatalf("defaultBuilds() error = %v", err)
	}
	var shape []string
	for _, b := range builds {
		shape = append(shape, tierNames[b.tier]+map[bool]string{false: "/self-managed", true: "/gitlab.com"}[b.dotcom])
	}
	want := []string{"Free/self-managed", "Free/gitlab.com", "Premium/self-managed", "Premium/gitlab.com", "Ultimate/self-managed", "Ultimate/gitlab.com"}
	if !slices.Equal(shape, want) {
		t.Errorf("builds = %v, want %v", shape, want)
	}
	_, selfManaged := builds[4].catalog.Action("orbit.status")
	_, dotcom := builds[5].catalog.Action("orbit.status")
	if selfManaged || !dotcom {
		t.Errorf("orbit.status served self-managed %t, GitLab.com %t, want GitLab.com only", selfManaged, dotcom)
	}
	if _, ok := builds[0].catalog.Action("discover_project.resolve"); !ok {
		t.Error("the Free build lacks the standalone discover_project.resolve")
	}
}

func TestDefaultSurfaces_RealCatalog_ListsBothInstanceClasses(t *testing.T) {
	surfaces := defaultSurfaces()
	for _, name := range []string{"gitlab_branch", "gitlab_orbit", "gitlab_discover_project", "gitlab_interactive_issue_create"} {
		t.Run("meta "+name, func(t *testing.T) {
			if !surfaces.meta[name] {
				t.Errorf("meta surface lacks %s", name)
			}
		})
	}
	for _, name := range []string{"gitlab_branch_list", "gitlab_orbit_status", "gitlab_server_status"} {
		t.Run("individual "+name, func(t *testing.T) {
			if !surfaces.individual[name] {
				t.Errorf("individual surface lacks %s", name)
			}
		})
	}
}

func TestAssemble_RealCatalog_ServerHealthCheckHasNoIndividualTool(t *testing.T) {
	ref := mustRealReference(t)
	health := actionByID(t, ref, "server.health_check")
	if health.individual != "" || health.metaTool != "gitlab_server" {
		t.Errorf("server.health_check tools = meta %q, individual %q", health.metaTool, health.individual)
	}
	if orbit := actionByID(t, ref, "orbit.query"); !orbit.dotcomOnly {
		t.Error("orbit.query is not GitLab.com only")
	}
}
