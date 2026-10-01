// declarations_test.go holds the declaration table to what an entry has to
// say, and the validation to every way an entry can fail to say it.
package main

import (
	"slices"
	"testing"
)

// TestAcceptedAdvisories_EveryEntryIsReviewable holds the real table to a key
// that names an advisory and a module, a defined category and a reason in
// words. The table is empty, which is the state it is meant to stay in.
func TestAcceptedAdvisories_EveryEntryIsReviewable(t *testing.T) {
	t.Parallel()

	if invalid := invalidDeclarations(acceptedAdvisories); len(invalid) != 0 {
		t.Fatalf("declarations that cannot be read as written: %v", invalid)
	}
}

// TestCategories_EachSaysWhatItMeans: a category is an excuse a reviewer
// reads, so one without a description excuses nothing anyone agreed to.
func TestCategories_EachSaysWhatItMeans(t *testing.T) {
	t.Parallel()

	for _, name := range []string{categoryNotLinked, categoryFixNotYetAdoptable} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if categories[name] == "" {
				t.Fatalf("category %q has no description", name)
			}
		})
	}
	if len(categories) != 2 {
		t.Errorf("categories = %v, want the two this file names", categories)
	}
}

// TestDeclarationKey_IsTheAdvisoryAndTheModule: the key a finding is looked up
// under.
func TestDeclarationKey_IsTheAdvisoryAndTheModule(t *testing.T) {
	t.Parallel()

	if got := declarationKey("GO-2026-5932", "golang.org/x/crypto"); got != "GO-2026-5932 golang.org/x/crypto" {
		t.Fatalf("declarationKey = %q, want the id and the module separated by one space", got)
	}
}

// TestInvalidDeclarations_NamesEachProblemInOrder covers every way an entry
// can fail to be read, and an entry that reads cleanly beside them.
func TestInvalidDeclarations_NamesEachProblemInOrder(t *testing.T) {
	t.Parallel()

	reason := "the reason"
	got := invalidDeclarations(map[string]declaration{
		"GO-2026-5932 golang.org/x/crypto":      {category: categoryNotLinked, reason: reason},
		"GO-2026-5932":                          {category: categoryNotLinked, reason: reason},
		"GO-2026-5932 golang.org/x/crypto more": {category: categoryNotLinked, reason: reason},
		"GO-2026-5933  golang.org/x/crypto":     {category: categoryNotLinked, reason: reason},
		"CVE-2026-1 golang.org/x/crypto":        {category: categoryNotLinked, reason: reason},
		"GO-2026-5934 golang.org/x/crypto":      {category: "invented", reason: reason},
		"GO-2026-5935 golang.org/x/crypto":      {category: categoryFixNotYetAdoptable, reason: "   "},
	})
	want := []string{
		`"CVE-2026-1 golang.org/x/crypto": the key is not an advisory id and a module separated by one space`,
		`"GO-2026-5932 golang.org/x/crypto more": the key is not an advisory id and a module separated by one space`,
		`"GO-2026-5932": the key is not an advisory id and a module separated by one space`,
		`"GO-2026-5933  golang.org/x/crypto": the key is not an advisory id and a module separated by one space`,
		`"GO-2026-5934 golang.org/x/crypto": the category "invented" is not one this command defines`,
		`"GO-2026-5935 golang.org/x/crypto": gives no reason`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("invalidDeclarations =\n%q\nwant\n%q", got, want)
	}
}
