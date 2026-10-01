package join

import (
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

// TestPrimaryBoundary_ReadsWhereARouteMayBeHeld verifies a route's primary
// requirement is held at the boundary it declares, at any of its alternative
// boundaries, at all four when a callable resolves it per request or when it
// declares none, that a callable declared beside a boundary type keeps that
// type, and that an alternative naming neither adds nothing.
func TestPrimaryBoundary_ReadsWhereARouteMayBeHeld(t *testing.T) {
	callable := &apilive.Callable{Callable: true}
	cases := []struct {
		name string
		auth apilive.RouteAuthorization
		want finegrained.Boundary
	}{
		{name: "declared", auth: apilive.RouteAuthorization{BoundaryType: "project"}, want: finegrained.BoundaryProject},
		{
			name: "alternatives",
			auth: apilive.RouteAuthorization{Boundaries: []apilive.Boundary{{BoundaryType: "group"}, {BoundaryType: "user"}}},
			want: finegrained.BoundaryGroup | finegrained.BoundaryUser,
		},
		{name: "callable alone", auth: apilive.RouteAuthorization{Boundaries: []apilive.Boundary{{Boundary: callable}}}, want: finegrained.AllBoundaries},
		{
			name: "callable with a type",
			auth: apilive.RouteAuthorization{Boundaries: []apilive.Boundary{{BoundaryType: "project", Boundary: callable}}},
			want: finegrained.BoundaryProject,
		},
		{name: "none", auth: apilive.RouteAuthorization{}, want: finegrained.AllBoundaries},
		{
			name: "an alternative naming neither",
			auth: apilive.RouteAuthorization{Boundaries: []apilive.Boundary{{BoundaryType: "group"}, {}}},
			want: finegrained.BoundaryGroup,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := primaryBoundary(&testCase.auth); got != testCase.want {
				t.Errorf("primaryBoundary = %s, want %s", got, testCase.want)
			}
		})
	}
}
