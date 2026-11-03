// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package apilive

import (
	"slices"
	"strings"
)

// This file is how a request this server sends is found among the record's
// routes, and how a route is named once it is found. Two readers ask it: the
// fine-grained derivation (cmd/gen_action_grants), which places every route a
// handler derives, and R-GRANT (cmd/audit_1to1 -scope=grants), which places
// every route the unit suite recorded so the two can be held to each other. A
// second copy of the rule in either would let the two disagree about which
// route a request is, which is the one thing their comparison assumes they
// agree on. A recorded path holds what a route's splat only stands for, so
// R-GRANT and R-PATH also ask [RouteIndex.MatchAcross] of what no spelling of
// the path's own length meets; the derivation places spellings of its own,
// which never need it.

// RouteName spells a record route the way the fine-grained table and the
// request record name it: the verb and the path under the API prefix, with
// parameters named as GitLab names them ("GET /projects/:id/issues").
func RouteName(route *Route) string {
	return route.Method + " " + strings.TrimPrefix(route.Path, EndpointPrefix)
}

// RouteIndex finds the record's route for a path a request was sent to.
type RouteIndex struct {
	// byMethod maps a verb to every spelling of every route with that verb.
	byMethod map[string][]spelled
}

// spelled is one spelling of a record route: its path with optional segments
// taken or left out, as written and with identifiers collapsed.
type spelled struct {
	written  []string
	segments []string
	route    *Route
}

// NewRouteIndex indexes routes, every spelling Grape's optional groups allow
// of each. A route mounted outside the API prefix is no route a request of
// this server's reaches, and is left out.
func NewRouteIndex(routes []Route) *RouteIndex {
	index := &RouteIndex{byMethod: map[string][]spelled{}}
	for i := range routes {
		route := &routes[i]
		path, mounted := strings.CutPrefix(route.Path, EndpointPrefix)
		if !mounted {
			continue
		}
		for _, variant := range ExpandOptional(path) {
			index.byMethod[route.Method] = append(index.byMethod[route.Method], spelled{
				written:  strings.Split(strings.Trim(variant, "/"), "/"),
				segments: PathSegments(variant),
				route:    route,
			})
		}
	}
	return index
}

// ExpandOptional spells a Grape path with each optional group, written in
// parentheses, taken and left out: `:id/(-/)epics` is both `:id/epics` and
// `:id/-/epics`. An escaped parenthesis is a literal, not a group, a closing
// one outside every group is a literal too, and a group that never closes
// leaves the path as written.
func ExpandOptional(path string) []string {
	open, depth, escaped := 0, 0, false
	// The cases are a sequence of ifs rather than an untagged switch so the
	// mutation tool can measure each condition; it cannot see a case
	// expression.
	for i := range len(path) {
		if escaped {
			escaped = false
			continue
		}
		if path[i] == '\\' {
			escaped = true
			continue
		}
		if path[i] == '(' {
			if depth == 0 {
				open = i
			}
			depth++
			continue
		}
		if path[i] != ')' || depth == 0 {
			continue
		}
		depth--
		if depth == 0 {
			return spellGroup(path[:open], path[open+1:i], path[i+1:])
		}
	}
	return []string{path}
}

// spellGroup spells a path whose first optional group, inner, sits between
// head and rest: every spelling of rest with the group left out, and with
// each spelling of the group taken.
func spellGroup(head, inner, rest string) []string {
	var out []string
	for _, tail := range ExpandOptional(rest) {
		out = append(out, head+tail)
		for _, middle := range ExpandOptional(inner) {
			out = append(out, head+middle+tail)
		}
	}
	return out
}

// PathSegments splits a path into segments, each identifier or splat
// collapsed to the placeholder ":", so two spellings of one endpoint that
// name their parameters differently meet.
func PathSegments(path string) []string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			segments[i] = ":"
		}
	}
	return segments
}

// Match finds the record route a request's path is: the one spelling whose
// segments equal the request's, a record placeholder standing for a literal
// of the request's where it must; the closest, by literal segments in
// common, when several do, and the first in the record's order among equally
// close ones. It reports false for a path no route of the verb meets.
func (index *RouteIndex) Match(method, path string) (*Route, bool) {
	requested := PathSegments(path)
	var best *Route
	bestScore := 0
	for _, candidate := range index.byMethod[method] {
		score, ok := agree(candidate.segments, requested)
		if ok && (best == nil || score > bestScore) {
			best, bestScore = candidate.route, score
		}
	}
	return best, best != nil
}

// SlashPlaceholder names a placeholder of a record route that GitLab lets hold
// a slash. Grape says so in the route's requirements, which the record does
// not carry, so a route reads as though every placeholder of it took one
// segment and only a declaration can say otherwise.
type SlashPlaceholder struct {
	// Route is the route as [RouteName] spells it, and Param the placeholder
	// without its colon.
	Route string
	Param string
	// Reason says where GitLab declares it.
	Reason string
}

// SlashPlaceholders are the placeholders of the live record known to take a
// slash and that a recorded request sends one to. R-GRANT fails on a row no
// recorded request is placed through, which is why the raw route beside this
// one has none: lib/api/ci/job_artifacts.rb mounts it with the same
// requirement, and no recorded request sends it a slashed ref.
var SlashPlaceholders = []SlashPlaceholder{{
	Route:  "GET /projects/:id/jobs/artifacts/:ref_name/download",
	Param:  "ref_name",
	Reason: "lib/api/ci/job_artifacts.rb at v19.5.0-ee mounts it with requirements: { ref_name: /.+/ }, and job.download_artifacts sends a branch named feature/x as the two segments toolutil.EscapePathSegments escapes one by one",
}}

// MatchAcross finds the record route a concrete path is when a segment of the
// route may take more than one of the path's, which [RouteIndex.Match] cannot
// say: a splat takes one or more (*artifact_path holds build/output/report.txt),
// and so does a placeholder slashes names. It answers the declaration the path
// was placed through, nil when a splat alone placed it, and no route for a path
// that meets none. A spelling takes part only when it holds such a segment, an
// optional group taken or left out, and the one with the most literal segments
// wins, the first in the record's order among equally many. A spelling with no
// literal segment never meets anything: that is Grape's catch-all, which any
// path would meet and which says nothing about the path.
func (index *RouteIndex) MatchAcross(method, path string, slashes []SlashPlaceholder) (*Route, *SlashPlaceholder) {
	requested := PathSegments(path)
	if route := index.closestAcross(method, requested, nil); route != nil {
		return route, nil
	}
	for i := range slashes {
		if route := index.closestAcross(method, requested, &slashes[i]); route != nil {
			return route, &slashes[i]
		}
	}
	return nil, nil
}

// closestAcross is the route of the closest spelling that meets a requested
// path, with one declaration's placeholder taking more than one segment when
// slash names one.
func (index *RouteIndex) closestAcross(method string, requested []string, slash *SlashPlaceholder) *Route {
	var best *Route
	bestLiterals := 0
	for _, candidate := range index.byMethod[method] {
		literals := candidate.literals()
		wide := candidate.wide(slash)
		if literals > bestLiterals && slices.Contains(wide, true) && across(candidate.segments, wide, requested) {
			best, bestLiterals = candidate.route, literals
		}
	}
	return best
}

// literals counts the segments of a spelling that are no placeholder.
func (s spelled) literals() int {
	count := 0
	for _, segment := range s.segments {
		if segment != ":" {
			count++
		}
	}
	return count
}

// wide says, by position, which segments of a spelling may take more than one
// of a path's: its splats, and the placeholder a declaration of its route
// names.
func (s spelled) wide(slash *SlashPlaceholder) []bool {
	declared := slash != nil && slash.Route == RouteName(s.route)
	wide := make([]bool, len(s.written))
	for i, segment := range s.written {
		wide[i] = strings.HasPrefix(segment, "*") || (declared && segment == ":"+slash.Param)
	}
	return wide
}

// across reports whether a record spelling meets a requested path, each wide
// segment taking one or more of the path's and any other exactly one, as
// [agree] reads one.
func across(record []string, wide []bool, requested []string) bool {
	if len(record) == 0 || len(requested) == 0 {
		return len(record) == len(requested)
	}
	if !wide[0] {
		return (record[0] == requested[0] || record[0] == ":") && across(record[1:], wide[1:], requested[1:])
	}
	for taken := 1; taken <= len(requested); taken++ {
		if across(record[1:], wide[1:], requested[taken:]) {
			return true
		}
	}
	return false
}

// Placeholders names, by the position of each segment of a path a request
// was sent to, the placeholder or splat of a record route that segment fills:
// read from the first spelling of the route, optional groups taken and left
// out, that the path meets as [RouteIndex.Match] meets one. It names nothing
// for a path no spelling of the route meets. A route mounted outside the API
// prefix is read as written.
func Placeholders(route *Route, path string) map[int]string {
	requested := PathSegments(path)
	for _, variant := range ExpandOptional(NormalizePath(route.Path)) {
		if _, meets := agree(PathSegments(variant), requested); !meets {
			continue
		}
		names := map[int]string{}
		for i, segment := range strings.Split(strings.Trim(variant, "/"), "/") {
			if name, isParam := placeholderOf(segment); isParam {
				names[i] = name
			}
		}
		return names
	}
	return nil
}

// placeholderOf reports the parameter a path segment names when it is a
// placeholder (`:id`) or a splat (`*file_path`).
func placeholderOf(segment string) (string, bool) {
	for _, prefix := range []string{":", "*"} {
		if name, isParam := strings.CutPrefix(segment, prefix); isParam {
			return name, true
		}
	}
	return "", false
}

// agree reports whether a record spelling meets a requested path, and how
// many literal segments they share. A record placeholder may stand for a
// requested literal; a requested placeholder never stands for a record
// literal.
func agree(record, requested []string) (int, bool) {
	if len(record) != len(requested) {
		return 0, false
	}
	score := 0
	for i := range record {
		switch record[i] {
		case requested[i]:
			if record[i] != ":" {
				score++
			}
		case ":":
		default:
			return 0, false
		}
	}
	return score, true
}
