package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// oauthTestGrants is a table of three operations, two of which a test
// declares refused to an OAuth token.
func oauthTestGrants() *finegrained.Table {
	return &finegrained.Table{Operations: []finegrained.Operation{
		{Name: "GET /a"}, {Name: "GET /tokens/self"}, {Name: "DELETE /tokens/self"},
	}}
}

// TestOAuthRefusedSet_EveryDeclaredRouteIsHeld_FormsTheSet verifies the
// declared routes become the set the actions are read against, and that a
// route the table holds no operation for stops generation, since the table
// holds a route only while some action sends it.
func TestOAuthRefusedSet_EveryDeclaredRouteIsHeld_FormsTheSet(t *testing.T) {
	grants := oauthTestGrants()
	set, err := oauthRefusedSet(grants, []oauthRefusedRoute{{Route: "GET /tokens/self"}, {Route: "DELETE /tokens/self"}})
	if err != nil || len(set) != 2 || !set["GET /tokens/self"] || !set["DELETE /tokens/self"] || set["GET /a"] {
		t.Errorf("oauthRefusedSet() = %v, %v, want the two declared routes", set, err)
	}
	_, err = oauthRefusedSet(grants, []oauthRefusedRoute{{Route: "GET /tokens/self"}, {Route: "POST /gone"}})
	if err == nil || !strings.Contains(err.Error(), "POST /gone names no operation of the action grants table") {
		t.Errorf("oauthRefusedSet() error = %v, want the route the table does not hold named", err)
	}
}

// TestAssemble_AnOAuthRefusalTheTableDoesNotHold_StopsGeneration verifies the
// stale declaration stops assemble before anything is folded.
func TestAssemble_AnOAuthRefusalTheTableDoesNotHold_StopsGeneration(t *testing.T) {
	_, err := assemble(testBuilds(t), testSurfaces(), nil, testGrants(), []oauthRefusedRoute{{Route: "DELETE /tokens/self"}})
	if err == nil || !strings.Contains(err.Error(), "DELETE /tokens/self names no operation") {
		t.Errorf("assemble() error = %v, want the stale declaration named", err)
	}
}

// TestOAuthRefusal_ReadsEachWay verifies an action names every refused route
// any of its ways sends, sorted and each once, and is refused on every way
// only when each way sends one; an action whose ways send none, or that has
// no way at all, names nothing.
func TestOAuthRefusal_ReadsEachWay(t *testing.T) {
	grants := oauthTestGrants()
	refused := map[string]bool{"GET /tokens/self": true, "DELETE /tokens/self": true}
	tests := []struct {
		name     string
		paths    [][]uint32
		routes   []string
		everyWay bool
	}{
		{name: "every way", paths: [][]uint32{{1}, {2, 1}}, routes: []string{"DELETE /tokens/self", "GET /tokens/self"}, everyWay: true},
		{name: "some ways", paths: [][]uint32{{0}, {1}}, routes: []string{"GET /tokens/self"}},
		{name: "a way that sends one beside another", paths: [][]uint32{{0, 1}}, routes: []string{"GET /tokens/self"}, everyWay: true},
		{name: "no way sends one", paths: [][]uint32{{0}}},
		{name: "no way at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, everyWay := oauthRefusal(grants, &finegrained.Requirement{Paths: tt.paths}, refused)
			if !slices.Equal(routes, tt.routes) || everyWay != tt.everyWay {
				t.Errorf("oauthRefusal() = %v, %t, want %v, %t", routes, everyWay, tt.routes, tt.everyWay)
			}
		})
	}
}

// TestOAuthRefusedRoutes_RealCatalog_NameTheSelfTokenActions verifies the
// declarations reach the actions they were written for in the catalog this
// binary builds: the four that act on the calling token alone, which no OAuth
// token can run, and the personal token read, which reads the calling token
// only when it is given no id.
func TestOAuthRefusedRoutes_RealCatalog_NameTheSelfTokenActions(t *testing.T) {
	ref := mustRealReference(t)
	everyWay, some := []string{}, []string{}
	for _, group := range ref.groups {
		for _, action := range group.actions {
			switch {
			case action.oauthRefusedEveryWay:
				everyWay = append(everyWay, action.id)
			case len(action.oauthRefused) > 0:
				some = append(some, action.id)
			}
		}
	}
	slices.Sort(everyWay)
	slices.Sort(some)
	wantEveryWay := []string{"access.token_group_rotate_self", "access.token_personal_revoke_self", "access.token_personal_rotate_self", "access.token_project_rotate_self"}
	if !slices.Equal(everyWay, wantEveryWay) {
		t.Errorf("actions refused to an OAuth token on every way = %v, want %v", everyWay, wantEveryWay)
	}
	if want := []string{"access.token_personal_get"}; !slices.Equal(some, want) {
		t.Errorf("actions refused to an OAuth token on some ways = %v, want %v", some, want)
	}
}
