package finegrained

import (
	"slices"
	"strings"
)

// Description is one action's requirement in GitLab's words: what the detail
// of an action serves a model and what the documentation site's fine-grained
// permissions page prints
// (https://jmrp.io/docs/gitlab-mcp-server/reference/fine-grained-permissions/),
// built here once so the two never word a requirement differently.
type Description struct {
	// GitLabVersion is the release the requirement was recorded at.
	GitLabVersion string `json:"gitlab_version"`
	// AnyOf are the ways of running the action, any one of which is enough:
	// the action sends a different request depending on its input.
	AnyOf []Way `json:"any_of,omitempty"`
	// DeniedWays are the ways of running the action no fine-grained token
	// passes, beside the ways in AnyOf that it can: an input that selects one
	// of these meets the denial, every other input runs.
	DeniedWays []Denial `json:"denied_ways,omitempty"`
	// Denied says why no fine-grained token runs the action at this release.
	Denied *Denial `json:"denied,omitempty"`
	// AlwaysEmpty are parts of the answer GitLab leaves empty for every
	// fine-grained token, while the rest is served, each written as a
	// [Selection].
	AlwaysEmpty []string `json:"always_empty,omitempty"`
	// EmptyWithout are parts of the answer GitLab leaves empty unless the
	// grant also holds what each names.
	EmptyWithout []Position `json:"empty_without,omitempty"`
}

// Way is one way of running an action: every need, each at one of its
// boundaries.
type Way struct {
	Needs []Need `json:"needs,omitempty"`
	// NotJudged is set when a request on this way opts out of the grant
	// check, so GitLab judges it by the token's other properties.
	NotJudged bool `json:"not_judged_by_grant,omitempty"`
}

// Need is one requirement GitLab checks on its own: every permission, held at
// one of the boundaries.
type Need struct {
	Permissions []string `json:"permissions"`
	At          []string `json:"at"`
}

// Position is a part of an answer, written as a [Selection], and what a grant
// needs for it to be served.
type Position struct {
	Selection string `json:"selection"`
	Needs     []Need `json:"needs"`
}

// Selection writes a position of an answer as the GraphQL selection that
// reaches it from the field the action asks for, `project { vulnerabilities {
// nodes } }` for the dotted path a table element holds. It is how a reader
// meets the position in a query, and it is the one spelling of a position
// this package hands out: a dotted path reads as a canonical action ID
// wherever its first field shares a domain's name, as `vulnerability.project`
// does, to a model reading the detail and to the documentation's name check
// reading the fine-grained permissions page alike.
func Selection(path string) string {
	fields := strings.Split(path, ".")
	return strings.Join(fields, " { ") + strings.Repeat(" }", len(fields)-1)
}

// Describe words one row. A nil row describes nothing and yields nil.
func (t *Table) Describe(row *Requirement) *Description {
	if t == nil || row == nil {
		return nil
	}
	description := &Description{GitLabVersion: t.DisplayVersion()}
	if row.Denied != nil {
		denied := *row.Denied
		description.Denied = &denied
		return description
	}
	for _, path := range row.Paths {
		way := t.way(path)
		if !slices.ContainsFunc(description.AnyOf, func(other Way) bool { return sameWay(other, way) }) {
			description.AnyOf = append(description.AnyOf, way)
		}
	}
	description.DeniedWays = slices.Clone(row.DeniedWays)
	for _, element := range row.Degraded {
		description.AlwaysEmpty = append(description.AlwaysEmpty, Selection(t.Elements[element].Path))
	}
	seen := map[uint32]bool{}
	for _, path := range row.Paths {
		for _, op := range path {
			for _, element := range t.Operations[op].OffSpine {
				if seen[element] {
					continue
				}
				seen[element] = true
				description.EmptyWithout = append(description.EmptyWithout, Position{
					Selection: Selection(t.Elements[element].Path), Needs: t.needs(t.Elements[element].Groups),
				})
			}
		}
	}
	return description
}

// way words one path: the groups of every operation on it, the spine's
// included, each once. An operation that opts out of the check leaves its own
// requirement to GitLab, but the objects its answer is made of are still
// checked, so its spine is worded with the rest.
func (t *Table) way(path []uint32) Way {
	var way Way
	var groups []uint32
	for _, index := range path {
		op := &t.Operations[index]
		if op.Skip {
			way.NotJudged = true
		} else {
			groups = append(groups, op.Groups...)
		}
		for _, element := range op.Spine {
			groups = append(groups, t.Elements[element].Groups...)
		}
	}
	way.Needs = t.needs(groups)
	return way
}

// needs words groups, dropping any that words the same as one before it.
func (t *Table) needs(groups []uint32) []Need {
	var needs []Need
	for _, index := range groups {
		need := t.need(&t.Groups[index])
		if !slices.ContainsFunc(needs, func(other Need) bool { return sameNeed(other, need) }) {
			needs = append(needs, need)
		}
	}
	return needs
}

// need words one group in the words GitLab's token page shows, falling back to
// the raw name for a permission no assignable carries.
func (t *Table) need(group *Group) Need {
	need := Need{At: group.Any.Names()}
	for _, perm := range group.Perms {
		word := t.displayOf(perm)
		if !slices.Contains(need.Permissions, word) {
			need.Permissions = append(need.Permissions, word)
		}
	}
	return need
}

// sameWay reports whether two ways word the same.
func sameWay(a, b Way) bool {
	return a.NotJudged == b.NotJudged && slices.EqualFunc(a.Needs, b.Needs, sameNeed)
}

// sameNeed reports whether two needs word the same.
func sameNeed(a, b Need) bool {
	return slices.Equal(a.Permissions, b.Permissions) && slices.Equal(a.At, b.At)
}
