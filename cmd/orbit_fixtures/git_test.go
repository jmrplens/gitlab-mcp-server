// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// kgFixture is the kg-fixtures fixture directory, from this package.
var kgFixture = filepath.Join("..", "..", "test", "fixtures", "orbit", "kg-fixtures")

// TestTrackedFiles_TheFixture_ListsWhatGitTracksLicenseFirst checks the list
// a commit and a push are built from is what git tracks in the fixture, with
// the LICENSE first and the rest in order.
func TestTrackedFiles_TheFixture_ListsWhatGitTracksLicenseFirst(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not installed here: %v", err)
	}
	files, err := trackedFiles(t.Context(), runGit, kgFixture)
	if err != nil {
		t.Fatalf("trackedFiles: %v", err)
	}
	if len(files) < 2 || files[0] != "LICENSE" || !slices.IsSorted(files[1:]) {
		t.Errorf("files = %q, want LICENSE first and the rest sorted", files)
	}
	if !slices.Contains(files, "src/acme/orders/fulfillment.py") {
		t.Errorf("files = %q, want the nested fulfillment module", files)
	}
	content, err := committedBytes(t.Context(), runGit, kgFixture, "LICENSE")
	if err != nil || !strings.Contains(string(content), "MIT") {
		t.Errorf("the LICENSE HEAD holds = %q, %v", content, err)
	}
}

// TestTrackedFiles_LicenseOrder_ComparesEitherSide checks the LICENSE goes
// first wherever git lists it.
func TestTrackedFiles_LicenseOrder_ComparesEitherSide(t *testing.T) {
	git := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("b\x00LICENSE\x00a\x00"), nil
	}
	files, err := trackedFiles(t.Context(), git, ".")
	if err != nil || !slices.Equal(files, []string{"LICENSE", "a", "b"}) {
		t.Errorf("files = %q, %v", files, err)
	}
	failing := func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("no repository") }
	if _, err = trackedFiles(t.Context(), failing, "."); err == nil {
		t.Error("a failed ls-tree listed files")
	}
}

// TestTrackedFiles_AStagedFile_IsListedOnlyOnceCommitted checks the list is
// what HEAD holds and not what the index does: a file staged and not
// committed has no content at HEAD to read, so listing it stopped the run
// with "cannot compare" instead of leaving it out.
func TestTrackedFiles_AStagedFile_IsListedOnlyOnceCommitted(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@example.test", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root"},
	} {
		if _, err := runGit(t.Context(), dir, args...); err != nil {
			t.Skipf("git cannot make a repository here: %v", err)
		}
	}
	// sequential: each file is written and staged, the first committed.
	for _, name := range []string{"committed.txt", "staged.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runGit(t.Context(), dir, "add", name); err != nil {
			t.Fatalf("git add %s: %v", name, err)
		}
		if name == "committed.txt" {
			if _, err := runGit(t.Context(), dir, "-c", "user.email=t@example.test", "-c", "user.name=t", "commit", "-q", "-m", "one"); err != nil {
				t.Fatalf("git commit: %v", err)
			}
		}
	}
	files, err := trackedFiles(t.Context(), runGit, dir)
	if err != nil || !slices.Equal(files, []string{"committed.txt"}) {
		t.Errorf("files = %q, %v; want only the committed file", files, err)
	}
	for _, file := range files {
		if _, showErr := committedBytes(t.Context(), runGit, dir, file); showErr != nil {
			t.Errorf("%s is listed and HEAD does not hold it: %v", file, showErr)
		}
	}
}

// TestRunGit_AFailure_NamesTheSubcommandAndNotTheConfiguration checks a git
// that fails is reported by its subcommand and its own words, and never with
// a -c value, which is where the token goes.
func TestRunGit_AFailure_NamesTheSubcommandAndNotTheConfiguration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not installed here: %v", err)
	}
	_, err := runGit(t.Context(), t.TempDir(), "-c", "http.extraHeader=PRIVATE-TOKEN: glpat-secret", "show", "HEAD:./nothing")
	if err == nil {
		t.Fatal("git show outside a repository succeeded")
	}
	if !strings.HasPrefix(err.Error(), "git show:") || strings.Contains(err.Error(), "glpat-secret") {
		t.Errorf("err = %v", err)
	}
}

// TestGitVerb_Args_SkipOptionsAndTheirValues checks the subcommand is found
// past -c and -C and their values, and that a command line of options alone
// names none.
func TestGitVerb_Args_SkipOptionsAndTheirValues(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{"-c", "user.name=x", "-C", "dir", "commit", "-m", "y"}, want: "commit"},
		{args: []string{"--quiet", "push"}, want: "push"},
		{args: []string{"-c", "a=b"}, want: "(no subcommand)"},
		{args: []string{"-C", "push"}, want: "(no subcommand)"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := gitVerb(tc.args); got != tc.want {
				t.Errorf("gitVerb(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
