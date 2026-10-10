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
