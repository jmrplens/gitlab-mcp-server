// coverage_test.go covers the summary the command prints beside the artifact:
// how much of the catalog the recording could see, and how honestly it says
// what it could not.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// makeToolsPackage creates internal/tools/<name> under root, which is how the
// summary tells an owner that recorded nothing from an owner that is no
// package at all.
func makeToolsPackage(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, toolsDir, name), 0o750); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", name, err)
	}
}

// TestCoverageOf_Owners_AreSplitThreeWays verifies the three counts stay
// disjoint and mean what they say: an owner that recorded something, one that
// exists and recorded nothing, and one that is not a package under
// internal/tools at all.
func TestCoverageOf_Owners_AreSplitThreeWays(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	makeToolsPackage(t, root, "adminspecs")
	rows := []row{
		{Package: "internal/tools/issues", Path: "/projects/:id/issues"},
		{Package: "internal/completions", Path: "/groups"},
	}

	summary := coverageOf(root, rows, []string{"issues", "issues", "adminspecs", "nowhere"})

	if summary.Total != 4 || summary.Covered != 2 || summary.Silent != 1 || summary.Unmapped != 1 {
		t.Fatalf("summary = %+v, want 4 actions split 2/1/1", summary)
	}
	if !slices.Equal(summary.SilentOwners, []string{"adminspecs"}) {
		t.Errorf("silent owners = %v, want [adminspecs]", summary.SilentOwners)
	}
	if !slices.Equal(summary.UnmappedOwners, []string{"nowhere"}) {
		t.Errorf("unmapped owners = %v, want [nowhere]", summary.UnmappedOwners)
	}
}

// TestSummarize_Rows_CountThePathsAndTheCatalog verifies the line a human
// reads after a recording run, including that -v names the packages behind the
// count rather than only scoring them.
func TestSummarize_Rows_CountThePathsAndTheCatalog(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	makeToolsPackage(t, root, "adminspecs")
	original := buildCatalog
	buildCatalog = func() ([]string, error) { return []string{"issues", "adminspecs", "nowhere"}, nil }
	t.Cleanup(func() { buildCatalog = original })
	rows := []row{
		{Package: "internal/tools/issues", Path: "/projects/:id/issues", Method: "GET"},
		{Package: "internal/tools/issues", Path: "/projects/:id/issues", Method: "POST"},
	}

	var quiet, verbose bytes.Buffer
	summarize(&quiet, root, rows, false)
	summarize(&verbose, root, rows, true)

	for _, want := range []string{"2 rows", "1 distinct paths", "1 packages", "1 of 3 catalog actions"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(quiet.String(), want) {
				t.Errorf("summary = %q, want it to contain %q", quiet.String(), want)
			}
		})
	}
	t.Run("no owner is named without -v", func(t *testing.T) {
		if strings.Contains(quiet.String(), "adminspecs") {
			t.Errorf("summary names an owner without -v: %q", quiet.String())
		}
	})
	for _, want := range []string{"silent: internal/tools/adminspecs", "no package of that name: nowhere"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(verbose.String(), want) {
				t.Errorf("verbose summary = %q, want it to contain %q", verbose.String(), want)
			}
		})
	}
}

// TestSummarize_CatalogFailure_KeepsTheInventory verifies that a catalog that
// will not build costs the coverage line and nothing else, since the artifact
// is complete with or without a count of what is missing from it.
func TestSummarize_CatalogFailure_KeepsTheInventory(t *testing.T) {
	original := buildCatalog
	buildCatalog = func() ([]string, error) { return nil, errors.New("catalog is broken") }
	t.Cleanup(func() { buildCatalog = original })

	var progress bytes.Buffer
	summarize(&progress, t.TempDir(), []row{{Package: "internal/tools/issues", Path: "/projects"}}, true)

	if !strings.Contains(progress.String(), "1 rows") {
		t.Errorf("summary = %q, want it to still count the rows", progress.String())
	}
	if !strings.Contains(progress.String(), "catalog is broken") {
		t.Errorf("summary = %q, want it to name the catalog failure", progress.String())
	}
}

// TestBuildCatalog_RealCatalog_NamesAnOwnerForEveryAction verifies the default
// implementation against the catalog this binary carries: every action has an
// owning package, or the summary would be counting something else.
func TestBuildCatalog_RealCatalog_NamesAnOwnerForEveryAction(t *testing.T) {
	owners, err := buildCatalog()
	if err != nil {
		t.Fatalf("buildCatalog error = %v", err)
	}
	if len(owners) == 0 {
		t.Fatal("buildCatalog returned no actions")
	}
	for _, owner := range owners {
		if owner == "" {
			t.Fatalf("an action of the %d in the catalog has no owning package", len(owners))
		}
	}
}

// TestDirectoryExists_Path_IsADirectoryOrNot verifies the check that tells an
// owner package that recorded nothing from a name that is no package.
func TestDirectoryExists_Path_IsADirectoryOrNot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"a directory", root, true},
		{"a regular file", file, false},
		{"nothing at all", filepath.Join(root, "absent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := directoryExists(tt.path); got != tt.want {
				t.Errorf("directoryExists(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}
