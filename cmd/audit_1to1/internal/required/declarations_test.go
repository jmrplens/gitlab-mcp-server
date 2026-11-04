// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package required

import (
	"strings"
	"testing"
)

// TestDeclaredRequiredness_EveryEntryNamesADirectionACategoryAndAReason
// verifies every declaration answers one of the two directions, under one of
// the categories this file defines, with a reason a reviewer can check. A
// declaration with no reason would excuse a disagreement nobody can judge.
func TestDeclaredRequiredness_EveryEntryNamesADirectionACategoryAndAReason(t *testing.T) {
	directions := map[string]bool{DirectionGitLabRequires: true, DirectionGitLabOptional: true}
	categories := map[string]bool{
		categoryConditional: true, categoryHandlerDefault: true, categoryOneAlternative: true,
		categorySoleAttribute: true, categorySDKPositional: true, categoryModelRequires: true,
		categorySDKSendsNull: true,
	}
	declared := declaredRequiredness()
	if len(declared) == 0 {
		t.Fatal("the declaration table is empty")
	}
	for key, answer := range declared {
		t.Run(key.action+"/"+key.field, func(t *testing.T) {
			if !directions[answer.Direction] || !categories[answer.Category] || strings.TrimSpace(answer.Reason) == "" {
				t.Errorf("declaration = %+v, want a known direction and category and a reason", answer)
			}
		})
	}
}

// TestDeclaredWorkhorseBodies_EveryEntryCitesTheReleaseAndRequiresSomething
// verifies every Workhorse body names a route the request record could
// spell, says where lib/api at the live record's release reads its params,
// and holds at least one param GitLab refuses a request without, since a
// body of optional params alone would place nothing the gate could fail on.
func TestDeclaredWorkhorseBodies_EveryEntryCitesTheReleaseAndRequiresSomething(t *testing.T) {
	declared := declaredWorkhorseBodies()
	if len(declared) == 0 {
		t.Fatal("the Workhorse body table is empty")
	}
	for route, body := range declared {
		t.Run(route, func(t *testing.T) {
			requires := false
			for _, required := range body.Params {
				requires = requires || required
			}
			if !strings.HasPrefix(route, "POST /") && !strings.HasPrefix(route, "PUT /") {
				t.Errorf("route %q is not a POST or PUT the record spells", route)
			}
			if !requires || !strings.HasPrefix(body.Source, "lib/api/") || !strings.Contains(body.Source, " at v19.5.0-ee ") {
				t.Errorf("body = %+v, want a required param and a lib/api citation at v19.5.0-ee", body)
			}
		})
	}
}

// TestFieldAliases_EveryDeclaredAlias_IsHandedOutByActionAndParam verifies
// the accessor another rule reads the aliases through: every declaration,
// filed under its action and its GitLab parameter, naming the field it
// fills the parameter from.
func TestFieldAliases_EveryDeclaredAlias_IsHandedOutByActionAndParam(t *testing.T) {
	declared := declaredAliases()
	byAction := FieldAliases()
	count := 0
	for _, params := range byAction {
		count += len(params)
	}
	if count != len(declared) {
		t.Errorf("FieldAliases holds %d aliases, want the %d declared", count, len(declared))
	}
	for key, alias := range declared {
		t.Run(key.action+"/"+key.param, func(t *testing.T) {
			if got := byAction[key.action][key.param]; got != alias.Field {
				t.Errorf("FieldAliases()[%q][%q] = %q, want %q", key.action, key.param, got, alias.Field)
			}
		})
	}
}

// TestDeclaredAliases_EveryEntryNamesAFieldAndAReason verifies every alias
// names the field it moves a parameter to and why the placement rules cannot
// find it.
func TestDeclaredAliases_EveryEntryNamesAFieldAndAReason(t *testing.T) {
	aliased := declaredAliases()
	if len(aliased) == 0 {
		t.Fatal("the alias table is empty")
	}
	for key, alias := range aliased {
		t.Run(key.action+"/"+key.param, func(t *testing.T) {
			if alias.Field == "" || alias.Field == key.param || strings.TrimSpace(alias.Reason) == "" {
				t.Errorf("alias = %+v, want a field other than the param and a reason", alias)
			}
		})
	}
}
