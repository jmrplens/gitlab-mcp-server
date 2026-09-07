// coverage_test.go covers the summary the command prints beside the artifact:
// how much of the catalog the recording could see, and how honestly it says
// what it could not.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// makeToolsPackage creates internal/tools/<name> under root, which is how the
// summary tells an owner that recorded nothing from an owner that is no
// package at all.
func makeToolsPackage(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(requestinventory.ToolsDir), name), 0o750); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", name, err)
	}
}

// TestSummarize_Rows_CountThePathsAndTheCatalog verifies the line a human
// reads after a recording run, including that -v names the packages behind the
// count rather than only scoring them.
func TestSummarize_Rows_CountThePathsAndTheCatalog(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	makeToolsPackage(t, root, "adminspecs")
	original := catalogActions
	catalogActions = func() ([]requestinventory.Action, error) {
		return []requestinventory.Action{
			{ID: "issue.list", Owner: "issues"},
			{ID: "topic.list", Owner: "adminspecs"},
			{ID: "ghost.list", Owner: "nowhere"},
		}, nil
	}
	t.Cleanup(func() { catalogActions = original })
	rows := []requestinventory.Row{
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
	original := catalogActions
	catalogActions = func() ([]requestinventory.Action, error) { return nil, errors.New("catalog is broken") }
	t.Cleanup(func() { catalogActions = original })

	var progress bytes.Buffer
	summarize(&progress, t.TempDir(), []requestinventory.Row{{Package: "internal/tools/issues", Path: "/projects"}}, true)

	if !strings.Contains(progress.String(), "1 rows") {
		t.Errorf("summary = %q, want it to still count the rows", progress.String())
	}
	if !strings.Contains(progress.String(), "catalog is broken") {
		t.Errorf("summary = %q, want it to name the catalog failure", progress.String())
	}
}
