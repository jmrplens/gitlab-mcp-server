// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkroutes

import (
	"cmp"
	"go/ast"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Placement is one path segment a parameter of a method fills in one route
// the method sends.
type Placement struct {
	// Param is the parameter's name in the method's signature, Index its
	// position among the parameters, the first counted 0, and Type its type
	// as the signature spells it ("any", "int64").
	Param string
	Index int
	Type  string
	// Route is the route, spelled as [Method.Routes] spells it.
	Route Route
	// Segment is the position of the segment the parameter fills among the
	// segments of the route's path, the first after the leading slash
	// counted 0.
	Segment int
}

// What the reading of [SDK.Placements] folds a parameter of the method to:
// its name, or the names of the parameters an expression is built out of
// joined by markSeparator, between markOpen and markClose. None of the three
// is a byte a path GitLab routes carries, nor one of the reading's own
// stand-ins.
const (
	markOpen      = "\x02"
	markClose     = "\x03"
	markSeparator = ","
)

// markPattern finds a mark and the names inside it. It is compiled on first
// use rather than at package initialization, so the statement that builds it
// is one a test runs.
var markPattern = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(markOpen + "([^" + markClose + "]*)" + markClose)
})

// Placements returns where the parameters of one method land in the routes
// it sends, sorted, and nothing for a key client-go does not declare.
//
// The method is read a second time with each of its parameters folded to a
// mark that names it. A mark is carried wherever a route's pieces are: into a
// function the method delegates to, through a format, and through what the
// reading cannot name but builds out of a marked parameter (ProjectID{pid},
// the string parseID(uid) answers, PathEscape(project)). A segment that comes
// out as one mark is a segment the parameters it names fill. A mark stands
// where the first reading has an unknown piece, so a path with its marks
// taken out is one the first reading spelled, and a placement names a route
// of [Method.Routes]; a spelling past the bound carries no mark.
func (s *SDK) Placements(key string) []Placement {
	entry, ok := s.entries[key]
	if !ok {
		return nil
	}
	return s.reading.placements(entry)
}

// placements reads entry with its parameters marked and collects the
// segment each mark fills in a route the entry sends.
func (r *reading) placements(entry *function) []Placement {
	env := map[string]string{}
	for _, param := range entry.params {
		env[param] = mark([]string{param})
	}
	found := placementSet{entry: entry, placed: map[Placement]bool{}}
	r.walk(entry, env, map[string]bool{}, func(fn *function, env map[string]string) {
		for _, use := range fn.templates {
			for _, raw := range r.templateSpellings(fn, use, env) {
				found.add(verbOf(fn.verb), raw)
			}
		}
		for _, use := range fn.legacy {
			verb, ok := legacyVerb(fn, use)
			if !ok {
				continue
			}
			for _, raw := range r.fold(fn, use.path, env, map[string]bool{}) {
				found.add(verb, raw)
			}
		}
	})
	return found.sorted()
}

// placementSet is what [reading.placements] collects for one method.
type placementSet struct {
	entry  *function
	placed map[Placement]bool
}

// add records the segments the marks of one formatted path fill. A path that
// names nothing static is no route, as the first reading has it, so a
// parameter handed through whole places nothing.
func (p placementSet) add(verb, raw string) {
	path, literal := shape(markPattern().ReplaceAllLiteralString(raw, unknown))
	if !literal {
		return
	}
	route := Route{Method: verb, Path: path}
	for i, segment := range pathSegments(raw) {
		whole := markPattern().FindStringSubmatch(segment)
		if len(whole) == 0 || whole[0] != segment {
			continue
		}
		for name := range strings.SplitSeq(whole[1], markSeparator) {
			index := slices.Index(p.entry.params, name)
			p.placed[Placement{Param: name, Index: index, Type: p.entry.paramTypes[index], Route: route, Segment: i}] = true
		}
	}
}

// sorted lists the placements by parameter position, route and segment.
func (p placementSet) sorted() []Placement {
	return slices.SortedFunc(maps.Keys(p.placed), func(a, b Placement) int {
		return cmp.Or(cmp.Compare(a.Index, b.Index), strings.Compare(a.Route.String(), b.Route.String()), cmp.Compare(a.Segment, b.Segment))
	})
}

// mark spells the mark of the parameters names.
func mark(names []string) string {
	return markOpen + strings.Join(names, markSeparator) + markClose
}

// mentioned names, sorted, the marked parameters an expression is built out
// of: an identifier bound to a mark, and a local of fn's body assigned an
// expression that is, read through every assignment of it. The field a
// selector names is no identifier of the body, and seen keeps a local
// assigned from itself from being read again.
func (r *reading) mentioned(fn *function, expr ast.Expr, env map[string]string, seen map[string]bool) []string {
	var names []string
	ast.Inspect(expr, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			names = append(names, r.mentioned(fn, typed.X, env, seen)...)
			return false
		case *ast.Ident:
			names = append(names, r.mentionedBy(fn, typed.Name, env, seen)...)
		}
		return true
	})
	return sortedUnique(names)
}

// mentionedBy names the marked parameters one identifier stands for: the
// names its mark carries when it is bound to one, and otherwise what every
// assignment of it in fn's body is built out of.
func (r *reading) mentionedBy(fn *function, name string, env map[string]string, seen map[string]bool) []string {
	if value, bound := env[name]; bound {
		var names []string
		for _, found := range markPattern().FindAllStringSubmatch(value, -1) {
			names = append(names, strings.Split(found[1], markSeparator)...)
		}
		return names
	}
	var names []string
	ast.Inspect(fn.decl.Body, func(node ast.Node) bool {
		if _, isLiteral := node.(*ast.FuncLit); isLiteral {
			return false
		}
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, isIdent := lhs.(*ast.Ident)
			key := fn.key + "@" + strconv.Itoa(int(lhs.Pos()))
			if !isIdent || ident.Name != name || seen[key] {
				continue
			}
			seen[key] = true
			names = append(names, r.mentioned(fn, assignedValue(assign, i), env, seen)...)
		}
		return true
	})
	return names
}

// assignedValue is what one assignment puts in its i-th target: the matching
// right-hand side, or the one call on the right that answers several values.
func assignedValue(assign *ast.AssignStmt, i int) ast.Expr {
	if len(assign.Rhs) == len(assign.Lhs) {
		return assign.Rhs[i]
	}
	return assign.Rhs[0]
}
