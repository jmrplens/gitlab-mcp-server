// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

// TestMeasure_ALinkedFile_IsRewrittenInTheCopyAlone verifies, where a tree
// holds symbolic links, that a file of the package that links to one outside
// the module is blanked in the staged copy and that what it links to keeps
// its bytes: a run that only measures edits nothing of the tree it measures.
func TestMeasure_ALinkedFile_IsRewrittenInTheCopyAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symbolic link takes a privilege there")
	}
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	const rel = "internal/release/release.go"
	original, link := filepath.Join(b.scratch, "original.go"), filepath.Join(b.root, filepath.FromSlash(rel))
	if err := errors.Join(os.Rename(link, original), os.Symlink(original, link)); err != nil {
		t.Fatal(err)
	}
	status, _, stderr := b.measure(t.Context(), "./internal/release")
	if status != 0 || len(b.calls) != 1 {
		t.Fatalf("status = %d, stderr = %q, ran %+v; want one measurement", status, stderr, b.calls)
	}
	if got, want := b.calls[0].Files["release.go"], blankConstraints(rec.Files[rel]); got != want || got == rec.Files[rel] {
		t.Errorf("gobco finds %q, want %q", got, want)
	}
	if got, err := os.ReadFile(original); err != nil || string(got) != rec.Files[rel] {
		t.Errorf("the file linked to reads %q (%v), want %q", got, err, rec.Files[rel])
	}
}

// TestMeasure_AModuleReachedThroughALink_IsLeftAsItIs verifies, where a tree
// holds symbolic links, that a run started in a directory that links to the
// module, which is how the go command then names the module's directory,
// stages a copy of the module and edits nothing of it.
func TestMeasure_AModuleReachedThroughALink_IsLeftAsItIs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symbolic link takes a privilege there")
	}
	rec := readRecord(t)
	b := newBench(t, rec.Files)
	const rel = "internal/release/release.go"
	link := filepath.Join(b.scratch, "link")
	if err := os.Symlink(b.root, link); err != nil {
		t.Fatal(err)
	}
	original := b.root
	b.root, b.environ = link, append(b.environ, "PWD="+link)
	status, _, stderr := b.measure(t.Context(), "./internal/release")
	if status != 0 || len(b.calls) != 1 {
		t.Fatalf("status = %d, stderr = %q, ran %+v; want one measurement", status, stderr, b.calls)
	}
	if got, want := b.calls[0].Files["release.go"], blankConstraints(rec.Files[rel]); got != want {
		t.Errorf("gobco finds %q, want %q", got, want)
	}
	if got, err := os.ReadFile(filepath.Join(original, filepath.FromSlash(rel))); err != nil || string(got) != rec.Files[rel] {
		t.Errorf("the module's own file reads %q (%v), want %q", got, err, rec.Files[rel])
	}
}

// TestMeasure_AModuleReachedThroughALink_IsMeasuredWhereItLeads verifies,
// where a tree holds symbolic links, that gobco is run on a package measured
// where it is in the directory behind the link: it copies the module around
// the directory it is run in without following a link to it, and panicked on
// the empty copy.
func TestMeasure_AModuleReachedThroughALink_IsMeasuredWhereItLeads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symbolic link takes a privilege there")
	}
	b := newBench(t, readRecord(t).Files)
	link := filepath.Join(b.scratch, "link")
	if err := os.Symlink(b.root, link); err != nil {
		t.Fatal(err)
	}
	original := b.root
	b.root, b.environ = link, append(b.environ, "PWD="+link)
	status, _, stderr := b.measure(t.Context(), "./internal/plain")
	if status != 0 || len(b.calls) != 1 {
		t.Fatalf("status = %d, stderr = %q, ran %+v; want one measurement", status, stderr, b.calls)
	}
	if got, want := b.calls[0].Dir, filepath.Join(original, "internal", "plain"); got != want {
		t.Errorf("gobco was run in %s, want %s", got, want)
	}
}

// TestMeasure_APackageTheCopyCannotHold_IsRefused verifies that a package
// whose directory is reached through a symbolic link, which in the copy would
// still lead to the original, and one in a directory the copy leaves out are
// not staged, and that nothing of the original is removed or added.
func TestMeasure_APackageTheCopyCannotHold_IsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symbolic link takes a privilege there")
	}
	rec := readRecord(t)
	cases := []struct {
		name, at, measured, want string
		linked                   bool
	}{
		{name: "a linked package", at: "internal/linked", measured: "internal/linked", linked: true, want: "internal/linked is a symbolic link"},
		{name: "below a linked directory", at: "linked", measured: "linked/scratch", linked: true, want: "linked is a symbolic link"},
		{name: "in a directory left out", at: "dist/scratch", measured: "dist/scratch", want: "could not be staged: lstat "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t, rec.Files)
			source, at := filepath.Join(b.root, "internal"), filepath.Join(b.root, filepath.FromSlash(tc.at))
			if tc.at == tc.measured {
				source = filepath.Join(source, "scratch")
			}
			place := os.Rename
			if tc.linked {
				place = os.Symlink
			}
			if err := place(source, at); err != nil {
				t.Fatal(err)
			}
			status, _, stderr := b.measure(t.Context(), "./"+tc.measured)
			if status != NotMeasured || !strings.Contains(stderr, tc.want) || !strings.HasSuffix(stderr, "; refusing to measure\n") {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, stderr, tc.want)
			}
			entries, err := os.ReadDir(filepath.Join(b.root, filepath.FromSlash(tc.measured)))
			if err != nil || len(entries) != 4 {
				t.Errorf("the package holds %v (%v), want the four files it held", entries, err)
			}
			if len(b.calls) != 0 || len(b.left(t)) != 0 {
				t.Errorf("ran %+v and left %q; want nothing", b.calls, b.left(t))
			}
		})
	}
}

// TestReplaceFile_ANameItCannotFree_IsAnError verifies that a file of the copy
// that cannot take the place of what holds its name is an error.
func TestReplaceFile_ANameItCannotFree_IsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "taken/held", "")
	var refused *fs.PathError
	if err := replaceFile(filepath.Join(dir, "taken"), "text"); !errors.As(err, &refused) || refused.Op != "remove" {
		t.Errorf("replaceFile(a directory that is not empty) = %v, want the removal refused", err)
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
