package main

import (
	"slices"
	"testing"
)

// TestWithoutContextOnPurpose_EveryEntryIsReviewable holds the real table to
// what an entry has to say: a category that is defined and a reason in words.
// The table is empty today, which is the state it is meant to stay in.
func TestWithoutContextOnPurpose_EveryEntryIsReviewable(t *testing.T) {
	if unknown := unknownCategories(withoutContextOnPurpose); len(unknown) != 0 {
		t.Fatalf("declarations with an undefined category: %v", unknown)
	}
	for key, entry := range withoutContextOnPurpose {
		t.Run(key, func(t *testing.T) {
			if entry.reason == "" {
				t.Fatal("declaration gives no reason")
			}
		})
	}
}

// TestCategories_EachSaysWhatItMeans: a category is an excuse a reviewer
// reads, so one without a description excuses nothing anyone agreed to.
func TestCategories_EachSaysWhatItMeans(t *testing.T) {
	if len(categories) == 0 {
		t.Fatal("no category is defined")
	}
	for name, meaning := range categories {
		t.Run(name, func(t *testing.T) {
			if meaning == "" {
				t.Fatal("category has no description")
			}
		})
	}
}

// TestDeclarationKey_IsThePackageAndTheFunction: the key a finding is looked
// up under.
func TestDeclarationKey_IsThePackageAndTheFunction(t *testing.T) {
	if got := declarationKey(Finding{Package: "internal/tools/x", Func: "Type.Method"}); got != "internal/tools/x:Type.Method" {
		t.Fatalf("declarationKey = %q, want internal/tools/x:Type.Method", got)
	}
}

// TestUnknownCategories_NamesEachInOrder: every declaration naming a category
// nobody defined is named, sorted, and a defined one is not.
func TestUnknownCategories_NamesEachInOrder(t *testing.T) {
	got := unknownCategories(map[string]declaration{
		"b:F": {category: "invented"},
		"a:F": {category: ""},
		"c:F": {category: categoryOutlivesTheCall},
	})
	if want := []string{"a:F", "b:F"}; !slices.Equal(got, want) {
		t.Fatalf("unknownCategories = %v, want %v", got, want)
	}
}

// TestStaleDeclarations_ScopedToWhatTheRunLoaded: an entry that excused
// nothing in a package the run looked at is stale, one that excused something
// is not, and one naming a package the run never loaded is out of view.
func TestStaleDeclarations_ScopedToWhatTheRunLoaded(t *testing.T) {
	declared := map[string]declaration{
		"loaded:Used":   {},
		"loaded:Unused": {},
		"loaded:Also":   {},
		"elsewhere:Fn":  {},
	}
	got := staleDeclarations(declared,
		map[string]struct{}{"loaded:Used": {}},
		map[string]struct{}{"loaded": {}})
	if want := []string{"loaded:Also", "loaded:Unused"}; !slices.Equal(got, want) {
		t.Fatalf("staleDeclarations = %v, want %v", got, want)
	}
}
