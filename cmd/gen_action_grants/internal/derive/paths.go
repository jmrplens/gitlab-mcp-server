package derive

import (
	"slices"
)

// pathSet is a disjunction of paths, each a sorted set of request indices: the
// ways one part of a body can run, each with the requests it makes. It is kept
// minimal, so no path holds another; a path that holds another needs more for
// nothing, since whatever lets the larger one run lets the smaller one too.
type pathSet [][]int

// maxPaths bounds the paths one action is expanded into. The tree's actions
// need a handful; the bound keeps a body that multiplies alternatives from
// running away, and an action that meets it is reported rather than cut.
const maxPaths = 256

// unit is the one path that makes no request: a part that sends nothing.
func unit() pathSet { return pathSet{{}} }

// single is the one path making one request.
func single(index int) pathSet { return pathSet{{index}} }

// isUnit reports whether a set is the one empty path, which is what a part
// that sends nothing expands to.
func (s pathSet) isUnit() bool { return len(s) == 1 && len(s[0]) == 0 }

// product runs both parts: every path of a joined with every path of b. It
// reports false when the result would pass [maxPaths].
func product(a, b pathSet) (pathSet, bool) {
	if a.isUnit() {
		return b, true
	}
	if b.isUnit() {
		return a, true
	}
	var out pathSet
	for _, left := range a {
		for _, right := range b {
			out = append(out, merge(left, right))
		}
	}
	out = minimize(out)
	return out, len(out) <= maxPaths
}

// merge joins two index sets into one sorted set.
func merge(a, b []int) []int {
	out := slices.Concat(a, b)
	slices.Sort(out)
	return slices.Compact(out)
}

// minimize drops repeated paths and every path holding another, and orders
// what is left shortest first, then by its indices.
func minimize(paths pathSet) pathSet {
	slices.SortFunc(paths, func(a, b []int) int {
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return slices.Compare(a, b)
	})
	var out pathSet
	for _, path := range paths {
		kept := true
		for _, shorter := range out {
			if contains(path, shorter) {
				kept = false
				break
			}
		}
		if kept {
			out = append(out, path)
		}
	}
	return out
}

// contains reports whether the sorted set a holds every index of b.
func contains(a, b []int) bool {
	i := 0
	for _, want := range b {
		for i < len(a) && a[i] < want {
			i++
		}
		if i == len(a) || a[i] != want {
			return false
		}
		i++
	}
	return true
}
