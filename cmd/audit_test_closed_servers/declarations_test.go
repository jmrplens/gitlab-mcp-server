// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// TestUsedAfterTheCloseOnPurpose_IsEmptyAndItsCategoriesAreDescribed states
// the table's intent, so an entry is a visible change, and holds every
// category a declaration may name to a description of what it means.
func TestUsedAfterTheCloseOnPurpose_IsEmptyAndItsCategoriesAreDescribed(t *testing.T) {
	if len(usedAfterTheCloseOnPurpose) != 0 {
		t.Errorf("usedAfterTheCloseOnPurpose = %v, want it empty: a test that needs a dead address has netfixture", usedAfterTheCloseOnPurpose)
	}
	for name, meaning := range categories {
		if meaning == "" {
			t.Errorf("category %q has no description", name)
		}
	}
	table := sourcegate.Table{Declared: usedAfterTheCloseOnPurpose, Categories: categories}
	if got := table.UnknownCategories(); len(got) != 0 {
		t.Errorf("unknown categories in the table = %v", got)
	}
}

// TestDeclarationKey_IsPackageAndFunction verifies the key a finding is
// looked up under is the one the table is documented to use.
func TestDeclarationKey_IsPackageAndFunction(t *testing.T) {
	if got := declarationKey(Finding{Package: "internal/oauth", Func: "TestX"}); got != "internal/oauth:TestX" {
		t.Errorf("declarationKey = %q, want %q", got, "internal/oauth:TestX")
	}
}
