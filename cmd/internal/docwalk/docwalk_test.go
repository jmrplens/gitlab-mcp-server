// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// docwalk_test.go holds the walk to what a documentation root holds: the
// files it yields and in what order, the directories it leaves out below a
// root and the roots it enters whatever they are called, and the failures it
// reports rather than reading as an empty tree.
package docwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// plant writes content to name below root, creating its directories.
func plant(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //#nosec G703 -- a path below the test's own t.TempDir
		t.Fatal(err)
	}
	return path
}

// relative names files relative to root, with slashes, in the order given.
func relative(t *testing.T, root string, files []string) []string {
	t.Helper()
	names := make([]string, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.ToSlash(rel))
	}
	return names
}

// TestFiles_ATree_YieldsItsDocumentsInLexicalOrder verifies the files a
// directory root holds: every .md and .mdx file below it, in lexical order,
// and nothing of another extension.
func TestFiles_ATree_YieldsItsDocumentsInLexicalOrder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plant(t, root, "docs/b.md", "")
	plant(t, root, "docs/a.mdx", "")
	plant(t, root, "docs/nested/c.md", "")
	plant(t, root, "docs/notes.txt", "")
	plant(t, root, "docs/UPPER.MD", "")
	files, err := Files([]string{filepath.Join(root, "docs")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/a.mdx", "docs/b.md", "docs/nested/c.md"}
	if got := relative(t, root, files); !slices.Equal(got, want) {
		t.Errorf("files %q, want %q", got, want)
	}
}

// TestFiles_BelowARoot_LeavesOutWhatIsNotTheRepositorysDocumentation
// verifies every directory a walk leaves out below its root, each holding a
// page it would otherwise read: the dependencies, a build's output, the
// untracked working area, a dot-directory and another checkout.
func TestFiles_BelowARoot_LeavesOutWhatIsNotTheRepositorysDocumentation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plant(t, root, "kept.md", "")
	for _, name := range []string{"node_modules/pkg/README.md", "dist/modeleval/run.md", "plan/draft.md", ".cache/page.md", "site/dist/page.md"} {
		plant(t, root, name, "")
	}
	plant(t, root, "worktree/.git", "gitdir: /elsewhere/.git/worktrees/one")
	plant(t, root, "worktree/copy.md", "")
	files, err := Files([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if got := relative(t, root, files); !slices.Equal(got, []string{"kept.md"}) {
		t.Errorf("files %q, want kept.md alone", got)
	}
}

// TestFiles_ARoot_IsEnteredWhateverItIs verifies that the rules judge the
// directories a walk descends into and never a root: a dot-directory named
// as a root is read, a root that is itself a checkout is read, and a root
// named for a directory a walk leaves out below a root is read too.
func TestFiles_ARoot_IsEnteredWhateverItIs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dot := plant(t, root, ".github/page.md", "")
	plant(t, root, "checkout/.git", "gitdir: /elsewhere")
	checkout := plant(t, root, "checkout/page.md", "")
	ignored := plant(t, root, "dist/page.md", "")
	files, err := Files([]string{filepath.Dir(dot), filepath.Dir(checkout), filepath.Dir(ignored)})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{dot, checkout, ignored}; !slices.Equal(files, want) {
		t.Errorf("files %q, want %q", files, want)
	}
}

// TestFiles_FileAndMissingRoots_AreThemselvesAndNothing verifies the two
// roots that are not trees: a file is itself whatever its extension, and a
// root that is not there holds nothing and is no error.
func TestFiles_FileAndMissingRoots_AreThemselvesAndNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := plant(t, root, "llms-install.txt", "")
	files, err := Files([]string{filepath.Join(root, "absent"), file})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{file}) {
		t.Errorf("files %q, want %q", files, file)
	}
}

// TestFiles_ADanglingLink_IsYielded verifies that a page whose link points
// nowhere is still a page: the walk names it and its reader reports what
// reading it found, rather than the walk passing it over in silence.
func TestFiles_ADanglingLink_IsYielded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	link := filepath.Join(root, "broken.md")
	if err := os.Symlink(filepath.Join(root, "absent.md"), link); err != nil {
		t.Skipf("this platform does not let the test make a link: %v", err)
	}
	files, err := Files([]string{root})
	if err != nil || !slices.Equal(files, []string{link}) {
		t.Errorf("files %q, error %v, want the link alone", files, err)
	}
}

// TestFiles_ARootThatCannotBeStated_Fails verifies that only a root that is
// not there is passed over: one the walk could not look at may hold anything.
func TestFiles_ARootThatCannotBeStated_Fails(t *testing.T) {
	t.Parallel()

	if _, err := Files([]string{"docs\x00"}); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Files of an unstattable root = %v, want an error other than its absence", err)
	}
}

// TestFiles_TheWalksOwnError_IsReturned verifies that the error the walk
// hands its callback, which a directory that cannot be listed produces, ends
// the walk with that error. The walk is replaced because a process that
// ignores directory permissions cannot be made to produce it, and
// TestFiles_ADirectoryThatCannotBeListed_Fails skips for such a process.
func TestFiles_TheWalksOwnError_IsReturned(t *testing.T) {
	restore := walkDir
	t.Cleanup(func() { walkDir = restore })
	unlisted := errors.New("unlisted")
	walkDir = func(root string, fn fs.WalkDirFunc) error { return fn(root, nil, unlisted) }

	if _, err := Files([]string{t.TempDir()}); !errors.Is(err, unlisted) {
		t.Errorf("Files = %v, want the walk's own error", err)
	}
}

// TestFiles_ADirectoryThatCannotBeListed_Fails verifies a subtree the walk
// cannot list stops it instead of being read as a subtree that holds
// nothing. Directory permissions are a POSIX mechanism a privileged process
// ignores, so the case skips when the chmod did not in fact deny the read.
func TestFiles_ADirectoryThatCannotBeListed_Fails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory read permission is not a Windows file mode")
	}
	root := t.TempDir()
	plant(t, root, "kept.md", "")
	closed := filepath.Join(root, "closed")
	if err := os.Mkdir(closed, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Skipf("this platform does not let the test close a directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, 0o700) }) //#nosec G302 -- a directory needs its search bit back or t.TempDir cannot remove it
	if _, err := os.ReadDir(closed); err == nil {
		t.Skip("this process lists a directory with no permissions on it, so the walk cannot be made to fail")
	}
	if _, err := Files([]string{root}); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Files = %v, want the permission failure the walk reported", err)
	}
}

// TestIsDocument_TheTwoExtensions_InTheirSpelling verifies which names are
// documents.
func TestIsDocument_TheTwoExtensions_InTheirSpelling(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]bool{"a.md": true, "d/b.mdx": true, "c.MD": false, "e.txt": false, "md": false} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			if got := IsDocument(path); got != want {
				t.Errorf("IsDocument(%q) = %v, want %v", path, got, want)
			}
		})
	}
}
