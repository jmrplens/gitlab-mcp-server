// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package goprogram

import (
	"maps"
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestPackagesUnderTest_ReadsTheVariantsNotTheExecutable verifies the packages
// under test are read off the variants' ForTest, and that neither a plain
// package nor the synthesized executable, which carry none, adds one.
func TestPackagesUnderTest_ReadsTheVariantsNotTheExecutable(t *testing.T) {
	underTest := PackagesUnderTest([]*packages.Package{
		{PkgPath: "example.com/p"},
		{PkgPath: "example.com/p [example.com/p.test]", ForTest: "example.com/p"},
		{PkgPath: "example.com/p_test [example.com/p.test]", ForTest: "example.com/p"},
		{PkgPath: "example.com/p.test", Name: "main"},
		{PkgPath: "example.com/q [example.com/q.test]", ForTest: "example.com/q"},
	})
	if got, want := slices.Sorted(maps.Keys(underTest)), []string{"example.com/p", "example.com/q"}; !slices.Equal(got, want) {
		t.Errorf("PackagesUnderTest = %v, want %v", got, want)
	}
	if got := PackagesUnderTest(nil); got == nil || len(got) != 0 {
		t.Errorf("PackagesUnderTest(nil) = %v, want an empty set a caller can read", got)
	}
}

// TestIsTestMain_BelievesTheSuffixOnlyBesideAVariant verifies the synthesized
// test executable is told apart from a package whose directory is named like
// one: the suffix counts only for a main package, and only when the load also
// holds a variant of the package the suffix names.
func TestIsTestMain_BelievesTheSuffixOnlyBesideAVariant(t *testing.T) {
	underTest := PackagesUnderTest([]*packages.Package{
		{PkgPath: "example.com/p [example.com/p.test]", ForTest: "example.com/p"},
		{PkgPath: "example.com/p"},
	})
	for _, tt := range []struct {
		name string
		pkg  *packages.Package
		want bool
	}{
		{name: "the executable built for a package with tests", pkg: &packages.Package{Name: "main", PkgPath: "example.com/p.test"}, want: true},
		{name: "a main package in a directory named like one", pkg: &packages.Package{Name: "main", PkgPath: "example.com/q.test"}},
		{name: "a library in a directory named like one", pkg: &packages.Package{Name: "p", PkgPath: "example.com/p.test"}},
		{name: "a main package", pkg: &packages.Package{Name: "main", PkgPath: "example.com/cmd"}},
		{name: "the package under test itself", pkg: &packages.Package{Name: "main", PkgPath: "example.com/p"}},
		{name: "the external test package", pkg: &packages.Package{Name: "p_test", PkgPath: "example.com/p_test"}},
		{name: "a package named after tests", pkg: &packages.Package{Name: "testutil", PkgPath: "example.com/testutil"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTestMain(tt.pkg, underTest); got != tt.want {
				t.Errorf("IsTestMain(%s) = %v, want %v", tt.pkg.PkgPath, got, tt.want)
			}
		})
	}
}

// TestVariantName_TakesTheDecorationOffAndNothingElse verifies a test
// variant's decoration is taken off, a plain path is left as it is, and the
// external test package keeps the name that makes it a package of its own.
func TestVariantName_TakesTheDecorationOffAndNothingElse(t *testing.T) {
	for _, tt := range []struct{ name, path, want string }{
		{"a test variant", "example.com/p [example.com/p.test]", "example.com/p"},
		{"the external test package", "example.com/p_test [example.com/p.test]", "example.com/p_test"},
		{"a plain package", "example.com/p", "example.com/p"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := VariantName(tt.path); got != tt.want {
				t.Errorf("VariantName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
