package main

import (
	"slices"
	"testing"
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
	if got := unknownCategories(usedAfterTheCloseOnPurpose); len(got) != 0 {
		t.Errorf("unknown categories in the table = %v", got)
	}
}

// TestStaleDeclarations_AreScopedToThePackagesLoaded verifies an entry that
// excused something is not stale, one in a loaded package that excused
// nothing is, and one in a package the run never loaded is passed over.
func TestStaleDeclarations_AreScopedToThePackagesLoaded(t *testing.T) {
	declared := map[string]declaration{
		"p:TestUsed":      {category: categoryOnlyAsText},
		"p:TestGone":      {category: categoryOnlyAsText},
		"p:TestAlsoGone":  {category: categoryOnlyAsText},
		"q:TestElsewhere": {category: categoryOnlyAsText},
	}
	excused := map[string]struct{}{"p:TestUsed": {}}
	scanned := map[string]struct{}{"p": {}}
	if got, want := staleDeclarations(declared, excused, scanned), []string{"p:TestAlsoGone", "p:TestGone"}; !slices.Equal(got, want) {
		t.Errorf("stale = %v, want %v", got, want)
	}
}

// TestUnknownCategories_NamesEachUndefinedOneInOrder verifies a category
// nobody defined is reported, sorted, and a defined one is not.
func TestUnknownCategories_NamesEachUndefinedOneInOrder(t *testing.T) {
	declared := map[string]declaration{
		"p:B":       {category: "made-up"},
		"p:A":       {category: "also-made-up"},
		"p:Defined": {category: categoryOnlyAsText},
	}
	if got, want := unknownCategories(declared), []string{"p:A", "p:B"}; !slices.Equal(got, want) {
		t.Errorf("unknown = %v, want %v", got, want)
	}
}

// TestDeclarationKey_IsPackageAndFunction verifies the key a finding is
// looked up under is the one the table is documented to use.
func TestDeclarationKey_IsPackageAndFunction(t *testing.T) {
	if got := declarationKey(Finding{Package: "internal/oauth", Func: "TestX"}); got != "internal/oauth:TestX" {
		t.Errorf("declarationKey = %q, want %q", got, "internal/oauth:TestX")
	}
}
