// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

// TestStage_AModuleThatCannotBeCopied_IsRefusedAndLeavesNothing verifies that
// a package outside its module, one of no module, and a temporary directory
// that is not there are each a run that measured nothing, with the reason and
// no copy left.
func TestStage_AModuleThatCannotBeCopied_IsRefusedAndLeavesNothing(t *testing.T) {
	rec := readRecord(t)
	cases := []struct {
		name     string
		module   func(root string) string
		noTemp   bool
		wantSaid string
	}{
		{name: "in no module", module: func(string) string { return "" }, wantSaid: "is not below the directory of its module, \"\""},
		{name: "outside its module", module: func(root string) string { return root + "x" }, wantSaid: "is not below the directory of its module, "},
		{name: "no temporary directory", module: func(root string) string { return root }, noTemp: true, wantSaid: "absent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			if tc.noTemp {
				for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
					t.Setenv(name, filepath.Join(b.scratch, "absent"))
				}
			}
			root := filepath.ToSlash(b.root)
			b.plant = answering("list", row(root+"/internal/scratch", tc.module(root), "scratch.go", "scratch_test.go", ""), 0, nil)
			status, stdout, stderr := b.measure(t.Context(), "./internal/plain")
			if status != NotMeasured || !strings.HasPrefix(stderr, "gobco: ./internal/plain could not be staged: ") ||
				!strings.Contains(stderr, tc.wantSaid) || !strings.HasSuffix(stderr, "; refusing to measure\n") {
				t.Errorf("status = %d, stderr = %q; want 1 and why it was not staged", status, stderr)
			}
			if !strings.Contains(stdout, "measuring it in a staged copy") || len(b.calls) != 0 || len(b.left(t)) != 0 {
				t.Errorf("said %q, ran %+v and left %q", stdout, b.calls, b.left(t))
			}
		})
	}
}

// TestStage_ACopyThatCannotBeRewritten_IsRemoved verifies that the copy is
// removed when the module cannot be copied, and when the package's copy cannot
// be brought to what gobco reads because a file of it is not there.
func TestStage_ACopyThatCannotBeRewritten_IsRemoved(t *testing.T) {
	rec := readRecord(t)
	for _, tc := range []struct {
		name   string
		staged staging
		module string
	}{
		{name: "left out and gone", staged: staging{leftOut: []string{"gone.go"}}},
		{name: "constrained and gone", staged: staging{constrained: []string{"gone.go"}}},
		{name: "support and gone", staged: staging{support: []string{"gone_test.go"}}},
		{name: "the module gone", staged: staging{unimported: true}, module: "gone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			module := filepath.ToSlash(filepath.Join(b.root, tc.module))
			m := &measurement{staged: tc.staged, pkg: golist.PackageFiles{Dir: module + "/internal/plain", ModuleDir: module, Name: "plain"}}
			var said strings.Builder
			m.Stdout = &said
			_, cleanup, err := m.stage()
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("stage() = %v, want that something is not there", err)
			}
			cleanup()
			if left := b.left(t); len(left) != 0 {
				t.Errorf("left in the temporary directory: %q", left)
			}
		})
	}
}

// TestBlankConstraints_OnlyTheHeader_IsBlanked verifies that a constraint
// below the package clause is a line like any other, that no line moves, and
// that a file that does not end in a newline is not given one.
func TestBlankConstraints_OnlyTheHeader_IsBlanked(t *testing.T) {
	t.Parallel()
	const text = "// c\n//go:build e2e\n// +build e2e\n\npackage p\n\n//go:build e2e\nvar x int"
	if got, want := blankConstraints(text), "// c\n\n\n\npackage p\n\n//go:build e2e\nvar x int"; got != want {
		t.Errorf("blankConstraints() = %q, want %q", got, want)
	}
}

// TestLeftOutOfCopy_EachPath_IsJudgedFromTheModulesRoot verifies what a copy
// of a module leaves out: the root's .git and dist and the worktrees below
// .claude, at the root alone, and node_modules wherever it is.
func TestLeftOutOfCopy_EachPath_IsJudgedFromTheModulesRoot(t *testing.T) {
	t.Parallel()
	for rel, want := range map[string]bool{
		".": false, ".git": true, "sub/.git": false, "dist": true, "docs/dist": false, ".claude": false, ".claude/worktrees": true,
		".claude/settings.json": false, "node_modules": true, "site/node_modules": true, "node_modules_kept": false,
	} {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			if got := leftOutOfCopy(rel); got != want {
				t.Errorf("leftOutOfCopy(%q) = %t, want %t", rel, got, want)
			}
		})
	}
}

// TestRewrite_AnUnwritableCopy_IsAnError verifies that the files an external
// test package without an import is given are written or the run says so.
func TestRewrite_AnUnwritableCopy_IsAnError(t *testing.T) {
	t.Parallel()
	m := &measurement{staged: staging{unimported: true}, pkg: golist.PackageFiles{Name: "p", ImportPath: "example.com/m/p"}}
	if err := m.rewrite(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("rewrite(absent) = %v, want that it is not there", err)
	}
}
