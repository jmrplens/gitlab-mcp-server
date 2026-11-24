// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package recipe

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CopyTree copies the directory src into dst, which it creates, with the modes
// and times of its files and its symbolic links as links, but for src itself,
// which is read through one: the go command names a module by the directory it
// is run in, which may be a link to it. skip is asked about every entry below
// src by its path relative to src, written with slashes, and one it answers
// true for is left out, a directory with everything below it; dst may be such
// a directory. What is neither a file, a directory nor a link is left out too:
// a socket or a pipe is nothing a package's tests read.
//
// A copy that cannot be finished is removed, and a dst that is already there is
// an error and left as it is, so a caller removes only a copy this made.
func CopyTree(src, dst string, skip func(rel string) bool) error {
	// A src that cannot be resolved is the walk's to report.
	if linked, err := filepath.EvalSymlinks(src); err == nil {
		src = linked
	}
	created := false
	err := filepath.WalkDir(src, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// The walk names everything from src, so there is always a path from
		// one to the other.
		rel, _ := filepath.Rel(src, name)
		if skip(filepath.ToSlash(rel)) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// The root is the first entry of the walk, and dst its copy.
		err = copyEntry(name, filepath.Join(dst, rel), entry)
		created = created || err == nil
		return err
	})
	if err != nil && created {
		_ = os.RemoveAll(dst)
	}
	return err
}

// copyEntry copies one entry of a tree. A directory stays writable by its
// owner whatever its mode, so its files can be written and the copy removed.
func copyEntry(from, to string, entry fs.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	switch info.Mode().Type() {
	case fs.ModeDir:
		return os.Mkdir(to, info.Mode().Perm()|0o700)
	case fs.ModeSymlink:
		linked, linkErr := os.Readlink(from)
		if linkErr != nil {
			return linkErr
		}
		return os.Symlink(linked, to)
	case 0:
		return copyFile(from, to, info)
	default:
		return nil
	}
}

// copyFile copies one regular file with its mode and its modification time.
func copyFile(from, to string, info fs.FileInfo) (err error) {
	in, err := os.Open(from) //#nosec G304 -- a file of the tree being copied
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()|0o200) //#nosec G304 -- its place in the copy
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}
