package join

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestExpandOptional_SpellsEveryWayAGrapePathCanBeWritten verifies a Grape
// path is spelled with each optional group taken and left out, nested groups
// one inside the other and a later group after an earlier one, a group at the
// very start, an escaped parenthesis and a closing one outside every group
// kept as the literals they are, and a group that never closes left alone
// rather than guessed at.
func TestExpandOptional_SpellsEveryWayAGrapePathCanBeWritten(t *testing.T) {
	cases := []struct {
		path string
		want []string
	}{
		{path: "/projects/:id/issues", want: []string{"/projects/:id/issues"}},
		{path: "/groups/:id(/-)/epics", want: []string{"/groups/:id/epics", "/groups/:id/-/epics"}},
		{path: "/a(/b(/c))", want: []string{"/a", "/a/b", "/a/b/c"}},
		{path: "/a(/b)/c(/d)", want: []string{"/a/c", "/a/b/c", "/a/c/d", "/a/b/c/d"}},
		{path: "(/a)/b", want: []string{"/b", "/a/b"}},
		{path: `/a\(b\)/c(/d)`, want: []string{`/a\(b\)/c`, `/a\(b\)/c/d`}},
		{path: `/a(/b\)c)`, want: []string{"/a", `/a/b\)c`}},
		{path: "/a)b(/c)", want: []string{"/a)b", "/a)b/c"}},
		{path: "/a(/b", want: []string{"/a(/b"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			if got := expandOptional(testCase.path); !slices.Equal(got, testCase.want) {
				t.Errorf("expandOptional(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestSegmentsOf_CollapsesEveryIdentifier verifies a named parameter and a
// splat both read as the placeholder, and the slashes around a path do not
// make empty segments.
func TestSegmentsOf_CollapsesEveryIdentifier(t *testing.T) {
	got := segmentsOf("/projects/:id/repository/files/*file_path/raw/")
	want := []string{"projects", ":", "repository", "files", ":", "raw"}
	if !slices.Equal(got, want) {
		t.Errorf("segmentsOf = %q, want %q", got, want)
	}
}

// TestAgree_ARecordPlaceholderStandsForADerivedLiteral verifies the match
// rule: a record placeholder may stand for a literal the derivation folded,
// a derived placeholder never stands for a record literal, and two spellings
// of different lengths never meet. The score counts the literals shared.
func TestAgree_ARecordPlaceholderStandsForADerivedLiteral(t *testing.T) {
	cases := []struct {
		name            string
		record, derived []string
		score           int
		ok              bool
	}{
		{name: "equal", record: []string{"projects", ":", "issues"}, derived: []string{"projects", ":", "issues"}, score: 2, ok: true},
		{name: "record placeholder", record: []string{"projects", ":", ":"}, derived: []string{"projects", ":", "slack"}, score: 1, ok: true},
		{name: "derived placeholder", record: []string{"projects", ":", "slack"}, derived: []string{"projects", ":", ":"}},
		{name: "literal differs", record: []string{"groups", ":"}, derived: []string{"projects", ":"}},
		{name: "length differs", record: []string{"projects"}, derived: []string{"projects", ":"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			score, ok := agree(testCase.record, testCase.derived)
			if score != testCase.score || ok != testCase.ok {
				t.Errorf("agree = %d, %t; want %d, %t", score, ok, testCase.score, testCase.ok)
			}
		})
	}
}

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

// TestRouteIndex_Match_TakesTheClosestSpellingAndTheFirstOfEqualOnes
// verifies a derived route is matched to the spelling sharing the most
// literal segments with it, a later one only when it is closer, the first in
// the record's order among equally close ones, a spelling of placeholders
// alone when nothing else meets it, and nothing when no spelling does.
func TestRouteIndex_Match_TakesTheClosestSpellingAndTheFirstOfEqualOnes(t *testing.T) {
	index := newRouteIndex([]apilive.Route{
		route("GET", "/projects/:id/:kind", nil),
		route("GET", "/projects/:id/things", nil),
		route("GET", "/projects/:project_id/things", nil),
		route("GET", "/:id", nil),
	})
	cases := []struct {
		path string
		want string
	}{
		{path: "/projects/:/things", want: "GET /projects/:id/things"},
		{path: "/projects/:/stuff", want: "GET /projects/:id/:kind"},
		{path: "/:", want: "GET /:id"},
		{path: "/groups/:/things"},
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			got, ok := index.match("GET", testCase.path)
			if ok != (testCase.want != "") || ok && liveName(got) != testCase.want {
				t.Errorf("match(%q) = %v, %t; want %q", testCase.path, got, ok, testCase.want)
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
