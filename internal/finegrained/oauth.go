// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package finegrained

import "slices"

// OAuthRefusal names the routes of an action's ways GitLab refuses to an
// OAuth access token ([Operation.OAuthRefused]), sorted and each once, and
// reports whether every way the action can run sends one, which is when no
// input lets an OAuth token run it (issue 1238).
//
// It is the one reading of the table both consumers make: the server
// withholds an action refused on every way from a session it knows to be
// authenticated by an OAuth token, and the tool reference names the refused
// routes on each action's token lines. An action with no row, or a row with
// no way, is refused nothing.
func (t *Table) OAuthRefusal(row *Requirement) (routes []string, everyWay bool) {
	if t == nil || row == nil {
		return nil, false
	}
	everyWay = len(row.Paths) > 0
	for _, path := range row.Paths {
		sends := false
		for _, op := range path {
			if operation := t.Operations[op]; operation.OAuthRefused {
				sends = true
				routes = append(routes, operation.Name)
			}
		}
		everyWay = everyWay && sends
	}
	slices.Sort(routes)
	return slices.Compact(routes), everyWay
}
