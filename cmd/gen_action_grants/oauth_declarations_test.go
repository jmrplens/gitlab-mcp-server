// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// TestMarkOAuthRefused_MarksEachDeclaredOperation_AndNamesAStaleOne verifies
// the declarations reach the table they were written for: each declared
// route's operation is marked refused to an OAuth token, the rest are left
// alone, and a declaration whose route the table holds no operation for is a
// finding naming it, since the table holds a route only while some action
// sends it (issue 1238).
func TestMarkOAuthRefused_MarksEachDeclaredOperation_AndNamesAStaleOne(t *testing.T) {
	table := &finegrained.Table{Operations: []finegrained.Operation{
		{Name: "GET /a"}, {Name: "GET /tokens/self"}, {Name: "DELETE /tokens/self"},
	}}
	findings := markOAuthRefused(table, []oauthDeclaration{{Route: "GET /tokens/self"}, {Route: "DELETE /tokens/self"}, {Route: "POST /gone"}})
	for i, want := range []bool{false, true, true} {
		t.Run(table.Operations[i].Name, func(t *testing.T) {
			if got := table.Operations[i].OAuthRefused; got != want {
				t.Errorf("OAuthRefused = %t, want %t", got, want)
			}
		})
	}
	if len(findings) != 1 || !strings.Contains(findings[0], "POST /gone names no operation of the table") {
		t.Errorf("findings = %q, want the stale declaration named", findings)
	}
}

// TestOAuthRefusedRoutes_EveryDeclarationNamesAnOperationOfTheCommittedTable
// holds the declarations to the table the tree carries: each names an
// operation, cites the release it was read at, and is marked in the table,
// so a regeneration that lost a mark or a declaration that outlived its route
// is caught before the server withholds the wrong actions.
func TestOAuthRefusedRoutes_EveryDeclarationNamesAnOperationOfTheCommittedTable(t *testing.T) {
	freshness.SkipIfDeferred(t)
	marked := map[string]bool{}
	for _, op := range actiongrants.Table().Operations {
		marked[op.Name] = op.OAuthRefused
	}
	for _, declaration := range oauthRefusedRoutes {
		t.Run(declaration.Route, func(t *testing.T) {
			refused, held := marked[declaration.Route]
			if !held || !refused {
				t.Errorf("the committed table holds %s = %t, marked %t; want it held and marked", declaration.Route, held, refused)
			}
			if !strings.Contains(declaration.Reason, "at v19.5.0-ee") {
				t.Errorf("the reason %q cites no release", declaration.Reason)
			}
		})
	}
	declared := 0
	for _, refused := range marked {
		if refused {
			declared++
		}
	}
	if declared != len(oauthRefusedRoutes) {
		t.Errorf("the committed table marks %d operations, want the %d declared", declared, len(oauthRefusedRoutes))
	}
}
