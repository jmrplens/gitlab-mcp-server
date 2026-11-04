// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apilive

import (
	"maps"
	"slices"
	"testing"
)

// mounted is a route under the API prefix, the way the record holds every
// route a request of this server's reaches.
func mounted(method, path string) Route {
	return Route{Method: method, Path: EndpointPrefix + path}
}

// TestRouteName_IsTheVerbAndThePathUnderThePrefix verifies a route is named
// by its verb and its path with the mount prefix gone and its parameters as
// GitLab names them, and that a route mounted elsewhere keeps its path whole.
func TestRouteName_IsTheVerbAndThePathUnderThePrefix(t *testing.T) {
	cases := []struct {
		name  string
		route Route
		want  string
	}{
		{name: "mounted", route: mounted("GET", "/projects/:id/issues"), want: "GET /projects/:id/issues"},
		{name: "outside_the_prefix", route: Route{Method: "*", Path: "/oauth/token"}, want: "* /oauth/token"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := RouteName(&testCase.route); got != testCase.want {
				t.Errorf("RouteName = %q, want %q", got, testCase.want)
			}
		})
	}
}

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
			if got := ExpandOptional(testCase.path); !slices.Equal(got, testCase.want) {
				t.Errorf("ExpandOptional(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestPathSegments_CollapsesEveryIdentifier verifies a named parameter and a
// splat both read as the placeholder, and the slashes around a path do not
// make empty segments.
func TestPathSegments_CollapsesEveryIdentifier(t *testing.T) {
	got := PathSegments("/projects/:id/repository/files/*file_path/raw/")
	want := []string{"projects", ":", "repository", "files", ":", "raw"}
	if !slices.Equal(got, want) {
		t.Errorf("PathSegments = %q, want %q", got, want)
	}
}

// TestAgree_ARecordPlaceholderStandsForARequestedLiteral verifies the match
// rule: a record placeholder may stand for a literal the request carries, a
// requested placeholder never stands for a record literal, and two spellings
// of different lengths never meet. The score counts the literals shared.
func TestAgree_ARecordPlaceholderStandsForARequestedLiteral(t *testing.T) {
	cases := []struct {
		name              string
		record, requested []string
		score             int
		ok                bool
	}{
		{name: "equal", record: []string{"projects", ":", "issues"}, requested: []string{"projects", ":", "issues"}, score: 2, ok: true},
		{name: "record placeholder", record: []string{"projects", ":", ":"}, requested: []string{"projects", ":", "slack"}, score: 1, ok: true},
		{name: "requested placeholder", record: []string{"projects", ":", "slack"}, requested: []string{"projects", ":", ":"}},
		{name: "literal differs", record: []string{"groups", ":"}, requested: []string{"projects", ":"}},
		{name: "length differs", record: []string{"projects"}, requested: []string{"projects", ":"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			score, ok := agree(testCase.record, testCase.requested)
			if score != testCase.score || ok != testCase.ok {
				t.Errorf("agree = %d, %t; want %d, %t", score, ok, testCase.score, testCase.ok)
			}
		})
	}
}

// TestRouteIndex_Match_TakesTheClosestSpellingAndTheFirstOfEqualOnes
// verifies a path is matched to the spelling sharing the most literal
// segments with it, a later one only when it is closer, the first in the
// record's order among equally close ones, a spelling of placeholders alone
// when nothing else meets it, an optional group either way, nothing when no
// spelling does, and nothing for a route mounted outside the API prefix.
func TestRouteIndex_Match_TakesTheClosestSpellingAndTheFirstOfEqualOnes(t *testing.T) {
	index := NewRouteIndex([]Route{
		mounted("GET", "/projects/:id/:kind"),
		mounted("GET", "/projects/:id/things"),
		mounted("GET", "/projects/:project_id/things"),
		mounted("GET", "/:id"),
		mounted("GET", "/groups/:id/(-/)epics"),
		{Method: "GET", Path: "/elsewhere/things"},
	})
	cases := []struct {
		path string
		want string
	}{
		{path: "/projects/:/things", want: "GET /projects/:id/things"},
		{path: "/projects/:/stuff", want: "GET /projects/:id/:kind"},
		{path: "/:", want: "GET /:id"},
		{path: "/groups/:group_id/epics", want: "GET /groups/:id/(-/)epics"},
		{path: "/groups/:group_id/-/epics", want: "GET /groups/:id/(-/)epics"},
		{path: "/groups/:/things"},
		{path: "/elsewhere/things"},
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			got, ok := index.Match("GET", testCase.path)
			if ok != (testCase.want != "") || ok && RouteName(got) != testCase.want {
				t.Errorf("Match(%q) = %v, %t; want %q", testCase.path, got, ok, testCase.want)
			}
		})
	}
	t.Run("another_verb", func(t *testing.T) {
		if got, ok := index.Match("POST", "/projects/:/things"); ok {
			t.Errorf("Match(POST) = %v, want no route of that verb", got)
		}
	})
}

// TestPlaceholders_NamesEachSegmentOfThePathAfterTheRoute verifies a path's
// segments are named by the spelling of the route the path meets: a
// placeholder and a splat by the name the record gives them, a literal not at
// all, each position counted from the segment after the leading slash, the
// spelling that leaves an optional group out as well as the one that takes it,
// a record placeholder a requested literal stands in, a route mounted outside
// the API prefix as written, and nothing for a path no spelling meets.
func TestPlaceholders_NamesEachSegmentOfThePathAfterTheRoute(t *testing.T) {
	const generic = "/projects/:id/packages/generic/:package_name/*package_version/(*path/):file_name"
	cases := []struct {
		name  string
		route Route
		path  string
		want  map[int]string
	}{
		{
			name:  "placeholders",
			route: mounted("GET", "/projects/:id/deploy_keys/:key_id"),
			path:  "/projects/:/deploy_keys/:",
			want:  map[int]string{1: "id", 3: "key_id"},
		},
		{
			name:  "a splat",
			route: mounted("GET", "/projects/:id/repository/files/*file_path"),
			path:  "/projects/:/repository/files/:",
			want:  map[int]string{1: "id", 4: "file_path"},
		},
		{
			name:  "the optional group left out",
			route: mounted("PUT", generic),
			path:  "/projects/:/packages/generic/:/:/:",
			want:  map[int]string{1: "id", 4: "package_name", 5: "package_version", 6: "file_name"},
		},
		{
			name:  "the optional group taken",
			route: mounted("PUT", generic),
			path:  "/projects/:/packages/generic/:/:/:/:",
			want:  map[int]string{1: "id", 4: "package_name", 5: "package_version", 6: "path", 7: "file_name"},
		},
		{
			name:  "a literal the record spells as a placeholder",
			route: mounted("PUT", "/projects/:id/integrations/:slug"),
			path:  "/projects/:/integrations/slack",
			want:  map[int]string{1: "id", 3: "slug"},
		},
		{
			name:  "no placeholder at all",
			route: mounted("GET", "/version"),
			path:  "/version",
			want:  map[int]string{},
		},
		{
			name:  "outside the prefix",
			route: Route{Method: "GET", Path: "/elsewhere/:thing"},
			path:  "/elsewhere/:",
			want:  map[int]string{1: "thing"},
		},
		{
			name:  "a path no spelling meets",
			route: mounted("GET", "/projects/:id"),
			path:  "/groups/:",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := Placeholders(&testCase.route, testCase.path)
			if !maps.Equal(got, testCase.want) || (got == nil) != (testCase.want == nil) {
				t.Errorf("Placeholders(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestPlaceholderOf_TellsAParameterFromALiteral verifies both spellings of a
// parameter name it, without the character that marks it, and a literal,
// empty or otherwise, names nothing.
func TestPlaceholderOf_TellsAParameterFromALiteral(t *testing.T) {
	cases := []struct {
		segment string
		name    string
		isParam bool
	}{
		{segment: ":id", name: "id", isParam: true},
		{segment: "*file_path", name: "file_path", isParam: true},
		{segment: "projects"},
		{segment: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.segment, func(t *testing.T) {
			name, isParam := placeholderOf(testCase.segment)
			if name != testCase.name || isParam != testCase.isParam {
				t.Errorf("placeholderOf(%q) = %q, %t; want %q, %t", testCase.segment, name, isParam, testCase.name, testCase.isParam)
			}
		})
	}
}
