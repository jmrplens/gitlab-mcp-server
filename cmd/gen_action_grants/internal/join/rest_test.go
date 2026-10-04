package join

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// TestLooselyAgree_APlaceholderOnEitherSideStandsForALiteral verifies the
// rule a slug declaration is held by: every record route its placeholders
// could be, which is a placeholder on either side meeting a literal.
func TestLooselyAgree_APlaceholderOnEitherSideStandsForALiteral(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{name: "placeholder on the left", a: []string{"x", ":"}, b: []string{"x", "y"}, want: true},
		{name: "placeholder on the right", a: []string{"x", "y"}, b: []string{"x", ":"}, want: true},
		{name: "literals differ", a: []string{"x", "y"}, b: []string{"x", "z"}},
		{name: "lengths differ", a: []string{"x"}, b: []string{"x", "y"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := looselyAgree(testCase.a, testCase.b); got != testCase.want {
				t.Errorf("looselyAgree = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestRestRequirements_CarriesTheRecordsReadingIntoTheJoinsForm verifies the
// join takes a route's demand as apilive reads it, group for group and in
// order, with its skip and its denial: an authorized route with a scope gives
// its two groups, and a skipped and a deferred route give none.
func TestRestRequirements_CarriesTheRecordsReadingIntoTheJoinsForm(t *testing.T) {
	authorized := &apilive.Route{Authorization: &apilive.RouteAuthorization{
		Permissions: []string{"update_issue", "read_issue"}, BoundaryType: "project",
		AdditionalScopes: []apilive.AdditionalScope{{Permissions: []string{"read_namespace"}, BoundaryType: "group"}},
	}}
	groups, skip, denied := restRequirements(authorized)
	want := []requirement{
		{perms: []string{"read_issue", "update_issue"}, any: finegrained.BoundaryProject},
		{perms: []string{"read_namespace"}, any: finegrained.BoundaryGroup},
	}
	if !reflect.DeepEqual(groups, want) || skip || denied != "" {
		t.Errorf("restRequirements(authorized) = %+v, %t, %q; want %+v, false, none", groups, skip, denied, want)
	}
	groups, skip, denied = restRequirements(&apilive.Route{Authorization: &apilive.RouteAuthorization{Skip: "public"}})
	if groups != nil || !skip || denied != "" {
		t.Errorf("restRequirements(skipped) = %+v, %t, %q; want no group, a skip, no denial", groups, skip, denied)
	}
	groups, skip, denied = restRequirements(&apilive.Route{Authorization: &apilive.RouteAuthorization{Todo: "later"}})
	if groups != nil || skip || denied != finegrained.CauseRESTTodo {
		t.Errorf("restRequirements(todo) = %+v, %t, %q; want no group, no skip, %q", groups, skip, denied, finegrained.CauseRESTTodo)
	}
}
