// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// plant writes a file below dir, creating its directory.
func plant(t *testing.T, dir, rel, content string, mode fs.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// keepAll skips nothing.
func keepAll(string) bool { return false }

// holdCopied holds the files of a copy to their content, and what it left out
// to not being there.
func holdCopied(t *testing.T, dst string, want map[string]string, leftOut ...string) {
	t.Helper()
	for rel, content := range want {
		if data, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel))); err != nil || string(data) != content {
			t.Errorf("%s = %q, %v; want %q", rel, data, err, content)
		}
	}
	for _, rel := range leftOut {
		if _, err := os.Lstat(filepath.Join(dst, filepath.FromSlash(rel))); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was copied (%v)", rel, err)
		}
	}
}

// TestCopyTree_ATree_IsCopiedWithoutWhatIsSkipped verifies the copy: every
// file with its content and modification time, a directory left out with
// everything below it and a file alone, and the copy itself when it sits
// inside the tree it copies.
func TestCopyTree_ATree_IsCopiedWithoutWhatIsSkipped(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	stamp := time.Date(2026, 11, 24, 1, 2, 3, 0, time.UTC)
	if err := os.Chtimes(plant(t, src, "a.txt", "a", 0o600), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	plant(t, src, "sub/b.txt", "b", 0o600)
	plant(t, src, "left/c.txt", "c", 0o600)
	plant(t, src, "sub/left.txt", "d", 0o600)
	dst := filepath.Join(src, "copy")
	skip := func(rel string) bool { return rel == "left" || rel == "sub/left.txt" || rel == "copy" }
	if err := CopyTree(src, dst, skip); err != nil {
		t.Fatalf("CopyTree() = %v", err)
	}
	holdCopied(t, dst, map[string]string{"a.txt": "a", "sub/b.txt": "b"}, "left", "sub/left.txt", "copy")
	info, err := os.Stat(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Errorf("a.txt's time = %v, want %v", info.ModTime(), stamp)
	}
}

// TestCopyTree_WhatOnlyAUnixTreeHolds_IsCopiedOrLeftOut verifies, where a
// platform has them, that a file keeps its mode, but for a file nobody could
// write, whose copy its owner can, that a directory its owner could not write
// in can be written in, that a link is copied as a link, and that a socket is
// left out.
func TestCopyTree_WhatOnlyAUnixTreeHolds_IsCopiedOrLeftOut(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has neither the modes nor the links")
	}
	// A short path, which a socket's address has to fit in.
	src, err := os.MkdirTemp("", "copy") //nolint:usetesting // a t.TempDir is too long for a socket's address on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(src) })
	plant(t, src, "sub/run.sh", "r", 0o700)
	plant(t, src, "sub/read.txt", "o", 0o400)
	if err = os.Symlink("sub/run.sh", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", filepath.Join(src, "s"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	locked := filepath.Join(src, "locked")
	if err = os.Mkdir(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	plant(t, src, "locked/in.txt", "i", 0o600)
	if err = os.Chmod(locked, 0o500); err != nil { //#nosec G302 -- a directory whose owner may not write in it, the case under test
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) //#nosec G302 -- so the test's own directory can be removed
	dst := filepath.Join(t.TempDir(), "copy")
	if err = CopyTree(src, dst, keepAll); err != nil {
		t.Fatalf("CopyTree() = %v", err)
	}
	holdCopied(t, dst, map[string]string{"sub/run.sh": "r", "sub/read.txt": "o", "link": "r", "locked/in.txt": "i"}, "s")
	for rel, mode := range map[string]fs.FileMode{"sub/run.sh": 0o700, "sub/read.txt": 0o600, "locked": fs.ModeDir | 0o700, "link": fs.ModeSymlink | 0o777} {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			info, statErr := os.Lstat(filepath.Join(dst, rel))
			if statErr != nil {
				t.Error(statErr)
			} else if info.Mode() != mode && info.Mode().Type() != fs.ModeSymlink {
				t.Errorf("%s's mode = %v, want %v", rel, info.Mode(), mode)
			}
		})
	}
}

// TestCopyTree_WhatCannotBeCopied_IsAnError verifies that a tree that is not
// there, a copy that is already there, and a file that cannot be written are
// each the copy's error rather than a copy with something missing.
func TestCopyTree_WhatCannotBeCopied_IsAnError(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	plant(t, src, "a.txt", "a", 0o600)
	if err := CopyTree(filepath.Join(src, "absent"), filepath.Join(t.TempDir(), "x"), keepAll); err == nil {
		t.Error("CopyTree(absent) = nil, want an error")
	}
	if err := CopyTree(src, t.TempDir(), keepAll); !errors.Is(err, fs.ErrExist) {
		t.Errorf("CopyTree(onto a directory) = %v, want that it exists", err)
	}
}

// goneEntry is an entry of a directory whose file went after it was listed.
type goneEntry struct{ fs.DirEntry }

// Info is the error the file's absence gives.
func (goneEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

// TestCopyEntry_AFileThatWent_IsAnError verifies the three places a file or a
// link can go between being listed and being copied, and the two a copy can
// be refused in.
func TestCopyEntry_AFileThatWent_IsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := plant(t, dir, "a.txt", "a", 0o600)
	info, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}
	absent := filepath.Join(dir, "absent")
	if err = copyEntry(file, absent, goneEntry{}); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("copyEntry(listed and gone) = %v", err)
	}
	if err = copyFile(absent, filepath.Join(dir, "copy"), info); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("copyFile(gone) = %v", err)
	}
	if err = copyFile(file, file, info); !errors.Is(err, fs.ErrExist) {
		t.Errorf("copyFile(onto a file) = %v", err)
	}
	if err = copyFile(dir, filepath.Join(dir, "copy"), info); err == nil {
		t.Error("copyFile(a directory) = nil, want the read's error")
	}
	if runtime.GOOS == "windows" {
		return
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink("a.txt", link); err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if err = copyEntry(absent, filepath.Join(dir, "x"), fs.FileInfoToDirEntry(linkInfo)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("copyEntry(a link that went) = %v", err)
	}
}
