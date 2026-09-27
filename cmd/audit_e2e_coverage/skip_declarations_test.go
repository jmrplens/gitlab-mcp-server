package main

import (
	"strings"
	"testing"
)

// skipPackages are the packages a complete run tests, which a declaration's
// Package must name.
var skipPackages = map[string]bool{"common": true, "ce": true, "ee": true}

// TestSkipDeclarations_EveryEntry_WellFormed verifies that every skip
// declaration names a run, a package and a test the gate can match, carries a
// fragment of the reason so it cannot excuse the test for another one, and
// says which kind of skip it is and why it stands.
//
// An empty Because would be the dangerous one: every reason contains the empty
// string, so the declaration would excuse the test whatever it skipped for.
func TestSkipDeclarations_EveryEntry_WellFormed(t *testing.T) {
	for index, declaration := range declaredSkips {
		switch {
		case !skipRuntimes[declaration.Runtime]:
			t.Errorf("declaration %d names the run %q, want ce or ee", index, declaration.Runtime)
		case !skipPackages[declaration.Package]:
			t.Errorf("declaration %d names the package %q, want common, ce or ee", index, declaration.Package)
		case !strings.HasPrefix(declaration.Test, "Test") || strings.HasSuffix(declaration.Test, "/"):
			t.Errorf("declaration %d names the test %q, want a Test function or a subtest of one", index, declaration.Test)
		case strings.TrimSpace(declaration.Because) == "":
			t.Errorf("declaration %d of %s carries no fragment of the reason, so it would excuse any skip of that test", index, declaration.Test)
		case !declaredSkipCategories[declaration.Category]:
			t.Errorf("declaration %d of %s has the category %q, which is not a declared one", index, declaration.Test, declaration.Category)
		case strings.TrimSpace(declaration.Reason) == "":
			t.Errorf("declaration %d of %s says nothing about why it stands", index, declaration.Test)
		}
	}
}

// TestSkipDeclarations_NoTwoCoverOneSkip verifies that no two declarations of
// one run name the same test for the same reason, which the gate would report
// only on a run that has that skip: the second declaration covers nothing.
func TestSkipDeclarations_NoTwoCoverOneSkip(t *testing.T) {
	seen := map[skipDeclaration]bool{}
	for _, declaration := range declaredSkips {
		key := skipDeclaration{Runtime: declaration.Runtime, Package: declaration.Package, Test: declaration.Test, Because: declaration.Because}
		if seen[key] {
			t.Errorf("%s %s %s is declared twice for %q", declaration.Runtime, declaration.Package, declaration.Test, declaration.Because)
		}
		seen[key] = true
	}
}
