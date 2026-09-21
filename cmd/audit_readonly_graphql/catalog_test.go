package main

import (
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// TestCatalogActions_CatalogThatCannotBeBuilt_IsReported verifies the run ends
// with the builder's own reason rather than with an empty action list. An
// audit that answered for no action would exit clean while checking nothing,
// which is the one outcome this gate must not have.
func TestCatalogActions_CatalogThatCannotBeBuilt_IsReported(t *testing.T) {
	previous := buildActionCatalog
	t.Cleanup(func() { buildActionCatalog = previous })
	buildActionCatalog = func(*gitlabclient.Client, tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return nil, errFixture
	}

	actions, err := catalogActions()

	if err == nil {
		t.Fatalf("catalogActions() error = nil and returned %d action(s), want the builder's failure", len(actions))
	}
	if !strings.Contains(err.Error(), errFixture.Error()) {
		t.Errorf("catalogActions() error = %q, want it to carry %q", err, errFixture)
	}
}

// TestCatalogActions_ReturnsTheWholeSurface verifies the audit reads the real
// canonical catalog offline: every action arrives with the identity the
// findings quote, and both classifications are present, which is what makes
// the read-only subset meaningful.
func TestCatalogActions_ReturnsTheWholeSurface(t *testing.T) {
	actions, err := catalogActions()
	if err != nil {
		t.Fatalf("catalogActions: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("the canonical catalog produced no actions")
	}

	readOnly, mutating := 0, 0
	for _, item := range actions {
		if item.ID == "" || item.Name == "" || item.Owner == "" {
			t.Fatalf("action %+v is missing the identity a finding has to name", item)
		}
		if item.ReadOnly {
			readOnly++
			continue
		}
		mutating++
	}
	if readOnly == 0 {
		t.Error("no read-only actions, so this audit would check nothing")
	}
	if mutating == 0 {
		t.Error("no mutating actions, which means the classification was not read")
	}
}

// TestCatalogActions_CarriesEachIdentityFieldFromItsOwnSource verifies the
// three names an action arrives with are the three the catalog holds, each
// from its own field: the ID is what a finding is filed under, the name is
// what the construction sites are matched on and the owner is which package's
// site wins. All three are non-empty strings, so a name read into the owner's
// place and the owner into the name's would pass every check that asks only
// whether they are set, and would resolve every action against the wrong
// package with the fallback quietly taking every site of that name.
//
// The maintenance group is checked beside a domain action because it is only
// in the catalog when the builder is asked for it, and an audit that left it
// out would answer for one action fewer without saying so.
func TestCatalogActions_CarriesEachIdentityFieldFromItsOwnSource(t *testing.T) {
	actions, err := catalogActions()
	if err != nil {
		t.Fatalf("catalogActions: %v", err)
	}
	byID := make(map[string]action, len(actions))
	for _, item := range actions {
		byID[item.ID] = item
	}

	cases := []struct {
		id    string
		want  action
		group string
	}{
		{id: "issue.list", want: action{ID: "issue.list", Name: "list", Owner: "issues", ReadOnly: true}, group: "a domain group"},
		{id: "server.status", want: action{ID: "server.status", Name: "status", Owner: "health", ReadOnly: true}, group: "the maintenance group"},
	}
	for _, testCase := range cases {
		t.Run(testCase.group, func(t *testing.T) {
			item, ok := byID[testCase.id]
			if !ok {
				t.Fatalf("action %q is not in the catalog", testCase.id)
			}
			if item != testCase.want {
				t.Errorf("action %q = %+v, want %+v", testCase.id, item, testCase.want)
			}
		})
	}
}

// TestCatalogActions_IncludesKnownReadAndWriteActions verifies the two
// classifications land on actions whose nature is not in doubt, so a catalog
// change that inverted the flag would not pass this file.
func TestCatalogActions_IncludesKnownReadAndWriteActions(t *testing.T) {
	actions, err := catalogActions()
	if err != nil {
		t.Fatalf("catalogActions: %v", err)
	}
	byID := make(map[string]action, len(actions))
	for _, item := range actions {
		byID[item.ID] = item
	}

	cases := []struct {
		id           string
		wantReadOnly bool
	}{
		{id: "issue.list", wantReadOnly: true},
		{id: "project.get", wantReadOnly: true},
		{id: "vulnerability.list", wantReadOnly: true},
		{id: "vulnerability.dismiss", wantReadOnly: false},
		{id: "issue.create", wantReadOnly: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.id, func(t *testing.T) {
			item, ok := byID[testCase.id]
			if !ok {
				t.Fatalf("action %q is not in the catalog", testCase.id)
			}
			if item.ReadOnly != testCase.wantReadOnly {
				t.Errorf("action %q ReadOnly = %t, want %t", testCase.id, item.ReadOnly, testCase.wantReadOnly)
			}
		})
	}
}
