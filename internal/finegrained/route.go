package finegrained

import (
	"net/http"
	"strings"
	"sync"
)

// The two path prefixes a GitLab request this server sends begins with: every
// REST route under the v4 API, and the one GraphQL endpoint beside it.
const (
	restPrefix  = "/api/v4"
	graphQLPath = "/api/graphql"
)

// RouteTemplate names the route a GitLab request was sent to, from its method
// and its escaped path, as the table's own operations spell it: the path
// template of the REST operation it matches, under "/api/v4", or the GraphQL
// endpoint's own path. It reports false for a request to anything else,
// which is a route no action of this table calls.
//
// What it answers is always one of a closed set compiled into the binary, a
// template with every identifier a placeholder, and never a value the request
// carried. That is what lets a client span carry it as url.template without
// carrying a project path, a file name or a search query, which is the reason
// the span records no URL at all (internal/mcpotel's round tripper), and what
// keeps the attribute low in cardinality whatever a caller names.
//
// A request matches a template segment by segment: a literal segment matches
// itself, ":name" any one segment, "*name" one segment or more, and Grape's
// optional groups ("(-/)", "(ref/:ref/)") both with and without what they
// hold. The template with the most literal segments wins, so
// /projects/1/issues/statistics names the statistics route rather than an
// issue. A HEAD request with no HEAD route of its own is matched against the
// GET routes, since Grape answers a HEAD from the GET it mounts beside it and
// checks that GET's authorization, which a fine-grained token on a booted
// GitLab 19.4.1 shows (a HEAD on a raw file is refused naming Repository:
// Read exactly when the GET is).
func (t *Table) RouteTemplate(method, escapedPath string) (string, bool) {
	if escapedPath == graphQLPath {
		return graphQLPath, true
	}
	rest, found := strings.CutPrefix(escapedPath, restPrefix)
	if !found || !strings.HasPrefix(rest, "/") {
		return "", false
	}
	index := t.routes()
	segments := strings.Split(rest, "/")
	if template, matched := bestRoute(index[method], segments); matched {
		return restPrefix + template, true
	}
	if method == http.MethodHead {
		if template, matched := bestRoute(index[http.MethodGet], segments); matched {
			return restPrefix + template, true
		}
	}
	return "", false
}

// routeVariant is one way a route template can be written once its optional
// groups are decided: the template as the table spells it, split into the
// segments a request path is held to.
type routeVariant struct {
	template string
	segments []string
	literals int
}

// routeIndexes holds each table's variants, built the first time a request
// asks: a table is generated data with no code of its own, so the index is
// kept beside it rather than in it.
var routeIndexes sync.Map // *Table -> map[string][]routeVariant

// routes returns the table's REST templates by method, each expanded into
// every variant of its optional groups.
func (t *Table) routes() map[string][]routeVariant {
	if built, ok := routeIndexes.Load(t); ok {
		index, _ := built.(map[string][]routeVariant)
		return index
	}
	index := map[string][]routeVariant{}
	for _, op := range t.Operations {
		method, path, isREST := strings.Cut(op.Name, " ")
		if !isREST || !strings.HasPrefix(path, "/") {
			continue
		}
		for _, expanded := range expandOptional(path) {
			segments := strings.Split(expanded, "/")
			index[method] = append(index[method], routeVariant{template: path, segments: segments, literals: literalCount(segments)})
		}
	}
	actual, _ := routeIndexes.LoadOrStore(t, index)
	stored, _ := actual.(map[string][]routeVariant)
	return stored
}

// expandOptional writes out every way Grape's optional groups in a path can
// be taken: each "(...)" present and absent, nested groups included.
func expandOptional(path string) []string {
	open := strings.IndexByte(path, '(')
	if open < 0 {
		return []string{path}
	}
	depth, end := 0, -1
	for i := open; i < len(path); i++ {
		switch path[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			end = i
			break
		}
	}
	if end < 0 {
		// An unbalanced group is a template this reader cannot expand; it is
		// kept as written, and no request path carries a parenthesis to match
		// it.
		return []string{path}
	}
	before, inner, after := path[:open], path[open+1:end], path[end+1:]
	var out []string
	for _, rest := range expandOptional(after) {
		for _, held := range expandOptional(inner) {
			out = append(out, before+held+rest)
		}
		out = append(out, before+rest)
	}
	return out
}

// literalCount is how many of a variant's segments are written out rather
// than named, which is how specific it is.
func literalCount(segments []string) int {
	count := 0
	for _, segment := range segments {
		if !strings.HasPrefix(segment, ":") && !strings.HasPrefix(segment, "*") {
			count++
		}
	}
	return count
}

// bestRoute is the most specific variant a request's segments match, the
// template it belongs to, and whether any matched. Of two variants as
// specific, the template that sorts first wins, so the answer does not depend
// on the order the table lists its operations in.
func bestRoute(variants []routeVariant, segments []string) (string, bool) {
	best := -1
	template := ""
	for _, variant := range variants {
		if !matchSegments(variant.segments, segments) {
			continue
		}
		if variant.literals > best || variant.literals == best && variant.template < template {
			best, template = variant.literals, variant.template
		}
	}
	return template, best >= 0
}

// matchSegments reports whether a request's segments are a path the
// template's segments describe.
func matchSegments(template, path []string) bool {
	if len(template) == 0 {
		return len(path) == 0
	}
	head := template[0]
	if strings.HasPrefix(head, "*") {
		// A wildcard takes one segment or more, and leaves the rest to the
		// segments after it.
		for taken := 1; taken <= len(path); taken++ {
			if matchSegments(template[1:], path[taken:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if strings.HasPrefix(head, ":") {
		return path[0] != "" && matchSegments(template[1:], path[1:])
	}
	return head == path[0] && matchSegments(template[1:], path[1:])
}
