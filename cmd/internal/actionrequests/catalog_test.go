package actionrequests

import (
	"errors"
	"sort"
	"testing"

	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// catalogOnce memoizes the real catalog the identity tests read, since
// building it twice over costs seconds.
var catalogOnce []Action

// realCatalog returns the catalog this repository publishes, keyed by ID.
func realCatalog(t *testing.T) map[string]Action {
	t.Helper()
	if catalogOnce == nil {
		actions, err := Catalog()
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}
		catalogOnce = actions
	}
	byID := make(map[string]Action, len(catalogOnce))
	for _, action := range catalogOnce {
		byID[action.ID] = action
	}
	return byID
}

// TestCatalog_ACatalogThatCannotBeBuilt_IsReported verifies a reader is handed
// the builder's own reason rather than an empty action list. A gate that
// answered for no action would exit clean while checking nothing.
func TestCatalog_ACatalogThatCannotBeBuilt_IsReported(t *testing.T) {
	previous := catalogs
	t.Cleanup(func() { catalogs = previous })
	catalogs = func() ([]*actioncatalog.Catalog, error) { return nil, errFixture }

	actions, err := Catalog()

	if !errors.Is(err, errFixture) || actions != nil {
		t.Errorf("Catalog() = %d action(s), %v; want the builder's failure", len(actions), err)
	}
}

// TestCatalog_CarriesEachIdentityFieldFromItsOwnSource verifies the fields an
// action arrives with are the catalog's, each from its own field: the ID is
// what a finding is filed under, the name is what the sites are matched on,
// the owner which package's site wins and the tool what tells two sites of
// one package apart. All are non-empty strings, so one read into another's
// place would pass every check that only asks whether they are set.
//
// The maintenance group is checked beside a domain action because it is only
// in the catalog when the builder is asked for it, Orbit because only the
// GitLab.com build holds it, and the project discovery because only the
// standalone step adds it.
func TestCatalog_CarriesEachIdentityFieldFromItsOwnSource(t *testing.T) {
	byID := realCatalog(t)

	cases := []struct {
		group string
		want  Action
	}{
		{group: "a domain group", want: Action{ID: "issue.list", Name: "list", Owner: "issues", Tool: "gitlab_issue_list", ReadOnly: true, Group: "gitlab_issue"}},
		{group: "the maintenance group", want: Action{ID: "server.status", Name: "status", Owner: "health", Tool: "gitlab_server_status", ReadOnly: true, Group: "gitlab_server"}},
		{group: "the GitLab.com build", want: Action{ID: "orbit.status", Name: "status", Owner: "orbit", Tool: "gitlab_orbit_status", ReadOnly: true, Group: "gitlab_orbit"}},
		{
			group: "the standalone actions",
			want: Action{
				ID: "discover_project.resolve", Name: "resolve", Owner: "projectdiscovery", Tool: "gitlab_discover_project", ReadOnly: true,
				Group: "gitlab_discover_project",
			},
		},
		{group: "an admin_mode group", want: Action{ID: "admin.metadata_get", Name: "metadata_get", Owner: "metadata", Tool: "gitlab_get_metadata", ReadOnly: true, Group: "gitlab_admin"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.group, func(t *testing.T) {
			got, ok := byID[testCase.want.ID]
			if !ok {
				t.Fatalf("action %q is not in the catalog", testCase.want.ID)
			}
			if got != testCase.want {
				t.Errorf("action %q = %+v, want %+v", testCase.want.ID, got, testCase.want)
			}
		})
	}
}

// TestCatalog_IncludesKnownReadAndWriteActions verifies the classification
// lands on actions whose nature is not in doubt, so a catalog change that
// inverted the flag would not pass this file.
func TestCatalog_IncludesKnownReadAndWriteActions(t *testing.T) {
	byID := realCatalog(t)

	cases := []struct {
		id           string
		wantReadOnly bool
	}{
		{id: "issue.list", wantReadOnly: true},
		{id: "project.get", wantReadOnly: true},
		{id: "vulnerability.list", wantReadOnly: true},
		{id: "vulnerability.dismiss"},
		{id: "issue.create"},
		{id: "interactive.issue_create"},
	}
	for _, testCase := range cases {
		t.Run(testCase.id, func(t *testing.T) {
			got, ok := byID[testCase.id]
			if !ok {
				t.Fatalf("action %q is not in the catalog", testCase.id)
			}
			if got.ReadOnly != testCase.wantReadOnly {
				t.Errorf("action %q ReadOnly = %t, want %t", testCase.id, got.ReadOnly, testCase.wantReadOnly)
			}
		})
	}
}

// catalogOf builds a catalog holding one action, through the catalog's own
// constructor.
func catalogOf(t *testing.T, tool string, action actioncatalog.Action) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	if err := catalog.AddAction(tool, action); err != nil {
		t.Fatalf("build a fixture catalog: %v", err)
	}
	return catalog
}

// TestCatalog_TheBuildsAreFoldedOnceEachAndSorted verifies the union: an
// action both builds hold is read once, from the first build, and the list
// comes back in ID order whatever order the builds held it in.
func TestCatalog_TheBuildsAreFoldedOnceEachAndSorted(t *testing.T) {
	route := toolutil.Route(nil)
	first := catalogOf(t, "gitlab_zeta",
		actioncatalog.Action{ID: "zeta.list", Name: "list", Route: route, OwnerPackage: "first", ReadOnly: true})
	second := catalogOf(t, "gitlab_alpha",
		actioncatalog.Action{ID: "alpha.get", Name: "get", Route: route, OwnerPackage: "second", ReadOnly: true})
	again := catalogOf(t, "gitlab_zeta",
		actioncatalog.Action{ID: "zeta.list", Name: "list", Route: route, OwnerPackage: "again"})
	previous := catalogs
	t.Cleanup(func() { catalogs = previous })
	catalogs = func() ([]*actioncatalog.Catalog, error) {
		return []*actioncatalog.Catalog{first, second, again}, nil
	}

	actions, err := Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}

	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, action.ID)
	}
	if !sort.StringsAreSorted(ids) || len(ids) != 2 {
		t.Fatalf("Catalog() = %v, want the two IDs once each, sorted", ids)
	}
	for _, action := range actions {
		if action.Name == "list" && (action.Owner != "first" || !action.ReadOnly) {
			t.Errorf("the action both builds hold = %+v, want the first build's", action)
		}
	}
}
