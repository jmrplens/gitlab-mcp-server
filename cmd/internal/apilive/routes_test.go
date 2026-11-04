// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apilive

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
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

// artifactRoutes are the record's routes for what [RouteIndex.MatchAcross]
// is about: two splats and a declared placeholder of the job artifacts, the
// generic package route whose optional group holds a splat, a route with no
// splat that shares their head, and Grape's catch-all, under a verb a request
// can have and under the one the record mounts it with.
func artifactRoutes() []Route {
	return []Route{
		mounted("GET", "/projects/:id/jobs/artifacts/:ref_name/download"),
		mounted("GET", "/projects/:id/jobs/artifacts/:ref_name/raw/*artifact_path"),
		mounted("GET", "/projects/:id/jobs/:job_id/artifacts/*artifact_path"),
		mounted("GET", "/projects/:id/jobs/:job_id/artifacts"),
		mounted("GET", "/projects/:id/packages/generic/:package_name/*package_version/(*path/):file_name"),
		mounted("GET", "/*path"),
		mounted("*", "/*path"),
		{Method: "GET", Path: "/elsewhere/*path"},
	}
}

// slashedRef declares the placeholder artifactRoutes' download route takes a
// slash in.
var slashedRef = SlashPlaceholder{Route: "GET /projects/:id/jobs/artifacts/:ref_name/download", Param: "ref_name", Reason: "a requirement"}

// TestRouteIndex_MatchAcross_LetsASplatTakeTheSegmentsItHolds verifies a splat
// takes one segment, several and all that are left, before a literal, an
// optional group either way and a splat beside another; a declared placeholder
// takes several and says it was the declaration that placed the path, only
// when no splat did; and what stays unmatched: a path only the catch-all
// meets, an extra segment where the route holds no splat, a placeholder no
// declaration names, a requested placeholder where the route has a literal, a
// splat that would have to take nothing, a route outside the API prefix and a
// path of another verb.
func TestRouteIndex_MatchAcross_LetsASplatTakeTheSegmentsItHolds(t *testing.T) {
	const (
		jobFile     = "GET /projects/:id/jobs/:job_id/artifacts/*artifact_path"
		refFile     = "GET /projects/:id/jobs/artifacts/:ref_name/raw/*artifact_path"
		packageFile = "GET /projects/:id/packages/generic/:package_name/*package_version/(*path/):file_name"
	)
	index := NewRouteIndex(artifactRoutes())
	cases := []struct {
		name    string
		method  string
		path    string
		slashes []SlashPlaceholder
		want    string
		through bool
	}{
		{name: "splat_takes_one", path: "/projects/:project_id/jobs/:job_id/artifacts/report.txt", want: jobFile},
		{name: "splat_takes_several", path: "/projects/:project_id/jobs/:job_id/artifacts/build/output/report.txt", want: jobFile},
		{name: "a_placeholder_and_a_splat", path: "/projects/:project_id/jobs/artifacts/main/raw/dist/v1.2/app.txt", want: refFile},
		{name: "optional_group_left_out", path: "/projects/:p/packages/generic/pkg/1.0.0/tool.bin", want: packageFile},
		{name: "optional_group_taken", path: "/projects/:p/packages/generic/pkg/1.0.0/dist/linux-amd64/tool.bin", want: packageFile},
		{name: "declared_placeholder", path: "/projects/:p/jobs/artifacts/feature/my-branch/download", slashes: []SlashPlaceholder{slashedRef}, want: slashedRef.Route, through: true},
		{name: "declared_second", path: "/projects/:p/jobs/artifacts/feature/my-branch/download", slashes: []SlashPlaceholder{{Route: "GET /other", Param: "x"}, slashedRef}, want: slashedRef.Route, through: true},
		{name: "a_splat_before_the_declaration", path: "/projects/:p/jobs/artifacts/main/raw/a/b", slashes: []SlashPlaceholder{slashedRef}, want: refFile},
		{name: "undeclared_placeholder", path: "/projects/:p/jobs/artifacts/feature/my-branch/download"},
		{name: "declared_on_another_route", path: "/projects/:p/jobs/artifacts/feature/my-branch/download", slashes: []SlashPlaceholder{{Route: "GET /other", Param: "ref_name"}}},
		{name: "declared_another_placeholder", path: "/projects/:p/jobs/artifacts/feature/my-branch/download", slashes: []SlashPlaceholder{{Route: slashedRef.Route, Param: "id"}}},
		{name: "only_the_catch_all", path: "/anything/at/all"},
		{name: "no_splat_extra_segment", path: "/projects/:p/jobs/artifacts/main/download/extra"},
		{name: "no_splat_in_the_route", path: "/projects/:p/jobs/:j"},
		{name: "same_length_without_a_splat", path: "/projects/:p/jobs/:j/artifacts"},
		{name: "requested_placeholder_for_a_literal", path: "/projects/:p/jobs/:j/:artifacts/report.txt"},
		{name: "a_splat_takes_something", path: "/projects/:p/packages/generic/pkg/tool.bin"},
		{name: "outside_the_prefix", path: "/elsewhere/a/b"},
		{name: "another_verb", method: "POST", path: "/projects/:p/jobs/:j/artifacts/a/b"},
		{name: "the_catch_all_verb", method: "*", path: "/anything/at/all"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			method := testCase.method
			if method == "" {
				method = "GET"
			}
			route, through := index.MatchAcross(method, testCase.path, testCase.slashes)
			if (route != nil) != (testCase.want != "") || route != nil && RouteName(route) != testCase.want {
				t.Errorf("MatchAcross(%q) = %v; want %q", testCase.path, route, testCase.want)
			}
			if (through != nil) != testCase.through {
				t.Errorf("MatchAcross(%q) went through %v; want a declaration: %t", testCase.path, through, testCase.through)
			}
		})
	}
}

// TestRouteIndex_MatchAcross_TakesTheClosestSpelling verifies the spelling with
// the most literal segments wins whichever comes first in the record, and the
// first in the record's order among equally many.
func TestRouteIndex_MatchAcross_TakesTheClosestSpelling(t *testing.T) {
	cases := []struct {
		name   string
		routes []Route
		want   string
	}{
		{
			name:   "the_closer_one_first",
			routes: []Route{mounted("GET", "/packages/*path/index"), mounted("GET", "/packages/*path")},
			want:   "GET /packages/*path/index",
		},
		{
			name:   "the_closer_one_later",
			routes: []Route{mounted("GET", "/packages/*path"), mounted("GET", "/packages/*path/index")},
			want:   "GET /packages/*path/index",
		},
		{
			name:   "the_first_of_equal_ones",
			routes: []Route{mounted("GET", "/packages/*first"), mounted("GET", "/packages/*second")},
			want:   "GET /packages/*first",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			route, _ := NewRouteIndex(testCase.routes).MatchAcross("GET", "/packages/a/b/index", nil)
			if route == nil || RouteName(route) != testCase.want {
				t.Errorf("MatchAcross = %v, want %q", route, testCase.want)
			}
		})
	}
}

// TestSlashPlaceholders_EachNamesAPlaceholderOfItsRoute holds the declarations
// to the record's own spelling: each names a route of the committed record and
// a placeholder that route holds, and gives its reason. That the row still
// answers a recorded request is R-GRANT's question.
func TestSlashPlaceholders_EachNamesAPlaceholderOfItsRoute(t *testing.T) {
	document, err := Read(filepath.Join("..", "..", "..", DefaultDir))
	if err != nil {
		t.Fatalf("read the committed record: %v", err)
	}
	for _, declared := range SlashPlaceholders {
		t.Run(declared.Route, func(t *testing.T) {
			index := slices.IndexFunc(document.Routes, func(route Route) bool { return RouteName(&route) == declared.Route })
			if index < 0 {
				t.Fatalf("the record mounts no %s", declared.Route)
			}
			if !strings.Contains(document.Routes[index].Path, ":"+declared.Param) {
				t.Errorf("%s holds no :%s", declared.Route, declared.Param)
			}
			if declared.Reason == "" {
				t.Errorf("%s gives no reason", declared.Route)
			}
		})
	}
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
