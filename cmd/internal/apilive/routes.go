package apilive

import "strings"

// This file is how a request this server sends is found among the record's
// routes, and how a route is named once it is found. Two readers ask it: the
// fine-grained derivation (cmd/gen_action_grants), which places every route a
// handler derives, and R-GRANT (cmd/audit_1to1 -scope=grants), which places
// every route the unit suite recorded so the two can be held to each other. A
// second copy of the rule in either would let the two disagree about which
// route a request is, which is the one thing their comparison assumes they
// agree on.

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
// taken or left out, and identifiers collapsed.
type spelled struct {
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
			index.byMethod[route.Method] = append(index.byMethod[route.Method], spelled{segments: PathSegments(variant), route: route})
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
	for i := range len(path) {
		switch {
		case escaped:
			escaped = false
		case path[i] == '\\':
			escaped = true
		case path[i] == '(':
			if depth == 0 {
				open = i
			}
			depth++
		case path[i] == ')' && depth > 0:
			depth--
			if depth == 0 {
				return spellGroup(path[:open], path[open+1:i], path[i+1:])
			}
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
