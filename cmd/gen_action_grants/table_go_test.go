package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestRenderTable_KeyedLiteralsOfEveryField verifies the table is written as
// keyed composite literals, every field the table carries spelled once and a
// zero one left out, a boundary as the constants that name it, a bit set in
// hexadecimal, and the whole formatted the way gofmt writes it.
func TestRenderTable_KeyedLiteralsOfEveryField(t *testing.T) {
	table := &finegrained.Table{
		Version:        "19.4.1-ee",
		Bucket:         "19.4",
		Permissions:    []string{"read_a", "read_b"},
		Display:        []string{"A: Read", ""},
		RefusalDisplay: []string{"A: Read (old)", ""},
		Assignables: []finegrained.Assignable{
			{Name: "read_a", Permissions: []uint16{0}, Boundaries: finegrained.BoundaryProject | finegrained.BoundaryGroup, Grantable: true},
			{Name: "read_old", Permissions: []uint16{0, 1}, Deprecated: true},
		},
		PublicAnonymous: [2][]uint64{{0x1}, {}},
		PublicKnown:     true,
		Groups: []finegrained.Group{
			{Perms: []uint16{0}, Any: finegrained.AllBoundaries},
		},
		Operations: []finegrained.Operation{
			{Name: "GET /a", Groups: []uint32{0}},
			{Name: "mutation m (x.Y)", Skip: true, Spine: []uint32{0}, OffSpine: []uint32{1}},
		},
		Elements: []finegrained.Element{
			{Path: "m.thing", Type: "Thing", Groups: []uint32{0}, Effect: finegrained.EffectNull},
			{Path: "m.either", Type: "Either", Members: []string{"A", "B"}, Undeclared: true, Effect: finegrained.EffectRemoved},
		},
		Actions: []finegrained.Requirement{
			{ID: "a.denied", Denied: &finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "GET /a", Effect: finegrained.EffectRefused}},
			{ID: "a.read", Paths: [][]uint32{{0}, {0, 1}}, Degraded: []uint32{1}, GraphQL: true, Collection: true},
		},
	}
	want := tableHeader + `	Version: "19.4.1-ee",
	Bucket:  "19.4",
	Permissions: []string{
		"read_a",
		"read_b",
	},
	Display: []string{
		"A: Read",
		"",
	},
	RefusalDisplay: []string{
		"A: Read (old)",
		"",
	},
	Assignables: []finegrained.Assignable{
		{Name: "read_a", Permissions: []uint16{0}, Boundaries: finegrained.BoundaryProject | finegrained.BoundaryGroup, Grantable: true},
		{Name: "read_old", Permissions: []uint16{0, 1}, Boundaries: 0, Deprecated: true},
	},
	PublicAnonymous: [2][]uint64{
		{0x1},
		{},
	},
	PublicKnown: true,
	Groups: []finegrained.Group{
		{Perms: []uint16{0}, Any: finegrained.BoundaryProject | finegrained.BoundaryGroup | finegrained.BoundaryUser | finegrained.BoundaryInstance},
	},
	Operations: []finegrained.Operation{
		{Name: "GET /a", Groups: []uint32{0}},
		{Name: "mutation m (x.Y)", Skip: true, Spine: []uint32{0}, OffSpine: []uint32{1}},
	},
	Elements: []finegrained.Element{
		{Path: "m.thing", Type: "Thing", Groups: []uint32{0}, Effect: "null"},
		{Path: "m.either", Type: "Either", Members: []string{"A", "B"}, Undeclared: true, Effect: "removed-items"},
	},
	Actions: []finegrained.Requirement{
		{ID: "a.denied", Denied: &finegrained.Denial{Cause: "rest-todo", Element: "GET /a", Effect: "refused"}},
		{ID: "a.read", Paths: [][]uint32{{0}, {0, 1}}, Degraded: []uint32{1}, GraphQL: true, Collection: true},
	},
}
`
	if got := string(renderTable(table)); got != want {
		t.Errorf("renderTable =\n%s\nwant\n%s", got, want)
	}
}
