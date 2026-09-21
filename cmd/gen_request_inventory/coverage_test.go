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
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
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
//
// Every count in the fixture is a different number, which is what makes the
// assertions about them mean anything: five counts are printed and three of
// them share one Fprintf, so a fixture where the rows, the distinct paths and
// the packages all came to one (or where the silent actions, their packages
// and the unmapped actions did) reads identically whichever of them each hole
// is filled from.
func TestSummarize_Rows_CountThePathsAndTheCatalog(t *testing.T) {
	root := t.TempDir()
	// sequential: setup steps building one tree, asserted by the counts below
	for _, pkg := range []string{"issues", "tags", "adminspecs", "systemhooks"} {
		makeToolsPackage(t, root, pkg)
	}
	original := catalogActions
	catalogActions = func() ([]requestinventory.Action, error) {
		return []requestinventory.Action{
			{ID: "issue.list", Owner: "issues"},
			{ID: "issue.get", Owner: "issues"},
			{ID: "tag.list", Owner: "tags"},
			{ID: "tag.get", Owner: "tags"},
			{ID: "topic.list", Owner: "adminspecs"},
			{ID: "topic.create", Owner: "adminspecs"},
			{ID: "hook.list", Owner: "systemhooks"},
			{ID: "ghost.list", Owner: "nowhere"},
		}, nil
	}
	t.Cleanup(func() { catalogActions = original })
	rows := []requestinventory.Row{
		{Package: "internal/tools/issues", Path: "/projects/:id/issues", Method: "GET"},
		{Package: "internal/tools/issues", Path: "/projects/:id/issues", Method: "POST"},
		{Package: "internal/tools/issues", Path: "/projects/:id/issues/:iid", Method: "GET"},
		{Package: "internal/tools/tags", Path: "/projects/:id/repository/tags", Method: "GET"},
		{Package: "internal/tools/tags", Path: "/projects/:id/repository/tags", Method: "POST"},
	}

	written := time.Date(2026, 9, 15, 10, 30, 0, 0, time.UTC)
	var quiet, verbose bytes.Buffer
	summarize(&quiet, root, rows, written, false)
	summarize(&verbose, root, rows, written, true)

	for _, want := range []string{
		"5 rows",
		"3 distinct paths",
		"2 packages",
		"recorded 2026-09-15T10:30:00Z",
		"4 of 8 catalog actions",
		"3 in 2 package(s) recorded none, and 1 are owned by a name that is no package under internal/tools",
	} {
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
	for _, want := range []string{
		"silent: internal/tools/adminspecs",
		"silent: internal/tools/systemhooks",
		"no package of that name: nowhere",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(verbose.String(), want) {
				t.Errorf("verbose summary = %q, want it to contain %q", verbose.String(), want)
			}
		})
	}
	t.Run("a silent package is never listed as no package of that name", func(t *testing.T) {
		if strings.Contains(verbose.String(), "no package of that name: adminspecs") {
			t.Errorf("verbose summary = %q, want the two owner lists kept apart", verbose.String())
		}
	})
}

// TestSummarize_CatalogFailure_KeepsTheInventory verifies that a catalog that
// will not build costs the coverage line and nothing else, since the artifact
// is complete with or without a count of what is missing from it.
func TestSummarize_CatalogFailure_KeepsTheInventory(t *testing.T) {
	original := catalogActions
	catalogActions = func() ([]requestinventory.Action, error) { return nil, errors.New("catalog is broken") }
	t.Cleanup(func() { catalogActions = original })

	var progress bytes.Buffer
	summarize(&progress, t.TempDir(), []requestinventory.Row{{Package: "internal/tools/issues", Path: "/projects"}}, time.Now(), true)

	if !strings.Contains(progress.String(), "1 rows") {
		t.Errorf("summary = %q, want it to still count the rows", progress.String())
	}
	if !strings.Contains(progress.String(), "catalog is broken") {
		t.Errorf("summary = %q, want it to name the catalog failure", progress.String())
	}
}

// TestSummarize_UndatedRecording_SaysSo verifies the summary never implies a
// recording time it does not have: a reader uses that line to tell a check
// made against a fresh run from one made against a week-old one, and an empty
// or invented date answers the question wrongly rather than declining it.
func TestSummarize_UndatedRecording_SaysSo(t *testing.T) {
	original := catalogActions
	catalogActions = func() ([]requestinventory.Action, error) { return nil, nil }
	t.Cleanup(func() { catalogActions = original })

	var progress bytes.Buffer
	summarize(&progress, t.TempDir(), []requestinventory.Row{{Package: "internal/tools/issues", Path: "/projects"}}, time.Time{}, false)

	if !strings.Contains(progress.String(), "recorded at an unknown time") {
		t.Errorf("summary = %q, want it to say the recording time is unknown", progress.String())
	}
}
