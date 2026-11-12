// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package docwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/sourcewalk"
)

// ignoredDirs are the directory names a walk leaves out below a root because
// they hold what the repository ignores rather than documentation it writes.
var ignoredDirs = map[string]bool{"node_modules": true, "dist": true, "plan": true}

// walkDir walks a tree. It is a variable because the walk's own error, a
// directory that cannot be listed, cannot be produced for a process that
// ignores directory permissions, which is what the checks run as.
var walkDir = filepath.WalkDir

// IsDocument reports whether path names a Markdown or MDX file by its
// extension, spelled as this repository spells it: .md or .mdx, lower case,
// which is also the spelling git's *.md and *.mdx pathspecs match.
func IsDocument(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".md" || ext == ".mdx"
}

// SkipDir reports whether a walk leaves out the directory at path, one it
// descends into below its root.
func SkipDir(path string) bool {
	return ignoredDirs[filepath.Base(path)] || sourcewalk.SkipDirBelowRoot(path)
}

// Files returns the documentation each root holds, root by root and each
// tree in lexical order: a root that is a file is itself, a root that is a
// directory is every Markdown and MDX file below it that a walk does not
// leave out, and a root that does not exist holds nothing. A root that
// cannot be stated, or a directory below one that cannot be listed, stops
// the walk with its error, since a tree the walk could not read may hold
// anything.
func Files(roots []string) ([]string, error) {
	var files []string
	for _, root := range roots {
		info, err := os.Stat(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, root)
			continue
		}
		err = walkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if path != root && SkipDir(path) {
					return filepath.SkipDir
				}
				return nil
			}
			if IsDocument(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
