// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package finegrained

import (
	"slices"
	"testing"
)

// TestTable_OAuthRefusal_NamesTheRefusedRoutesAndWhetherEveryWaySendsOne
// verifies the one reading of the table the server and the tool reference
// share (issue 1238): the routes of an action's ways GitLab refuses to an
// OAuth token, sorted and each once, and whether no way avoids them, which is
// when the server withholds the action from an OAuth session. A way that
// sends none of them is enough to keep the action, and an action with no way
// or no row is refused nothing.
func TestTable_OAuthRefusal_NamesTheRefusedRoutesAndWhetherEveryWaySendsOne(t *testing.T) {
	table := &Table{Operations: []Operation{
		{Name: "GET /tokens/self", OAuthRefused: true},
		{Name: "DELETE /tokens/self", OAuthRefused: true},
		{Name: "GET /tokens/:id"},
	}}
	cases := []struct {
		name     string
		row      *Requirement
		routes   []string
		everyWay bool
	}{
		{name: "every way", row: &Requirement{Paths: [][]uint32{{1}, {0, 1}}}, routes: []string{"DELETE /tokens/self", "GET /tokens/self"}, everyWay: true},
		{name: "some ways", row: &Requirement{Paths: [][]uint32{{0}, {2}}}, routes: []string{"GET /tokens/self"}},
		{name: "none", row: &Requirement{Paths: [][]uint32{{2}}}},
		{name: "no way", row: &Requirement{}},
		{name: "no row", row: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routes, everyWay := table.OAuthRefusal(tc.row)
			if !slices.Equal(routes, tc.routes) || everyWay != tc.everyWay {
				t.Errorf("OAuthRefusal() = %v, %t, want %v, %t", routes, everyWay, tc.routes, tc.everyWay)
			}
		})
	}
	t.Run("no table", func(t *testing.T) {
		var none *Table
		if routes, everyWay := none.OAuthRefusal(&Requirement{Paths: [][]uint32{{0}}}); routes != nil || everyWay {
			t.Errorf("OAuthRefusal() on no table = %v, %t, want nothing refused", routes, everyWay)
		}
	})
}
