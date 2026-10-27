// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/sourcewalk"
)

// scriptExtensions are the names a script in another language than Go
// carries: shell, Node, Python, PowerShell, Ruby and Perl. A file with no
// extension at all is a script when it opens with #!.
var scriptExtensions = []string{
	".sh", ".bash", ".zsh", ".mjs", ".cjs", ".js", ".ts", ".mts", ".cts", ".py", ".ps1", ".psm1", ".rb", ".pl",
}

// unreadDirectories are the directories below the root the script rule does
// not read, by their repository-relative path: the site, whose own build
// steps are Node by design, the untracked plan/ working area, the npm
// packages the release assembles from the launcher, and the worktrees Claude
// Code checks out under .claude.
var unreadDirectories = map[string]bool{"site": true, "plan": true, "npm/packages": true, ".claude/worktrees": true}

// trackedDotDirectories are the dot-directories at the root this repository
// tracks files in, which the script rule reads although the other walkers
// skip every dot-directory: a script can be added to any of them and run from
// there. Every other one at the root is a tool's state (.git, a cache, an
// editor's or an agent's), not this repository's source.
var trackedDotDirectories = map[string]bool{".github": true, ".claude": true, ".plugin": true, ".vscode": true}

// unreadNames are the directories the script rule does not read wherever
// they sit: a package manager's install, a build's output, and a test's
// fixtures.
var unreadNames = map[string]bool{"node_modules": true, "dist": true, "testdata": true}

// openFile is a seam over os.Open, so a test can drive the branch a file the
// tests own never produces: a regular file that cannot be opened.
var openFile = os.Open

// scriptsInTree returns, sorted, every script in another language than Go
// below root, repository-relative, reading the tree the way the other walkers
// here do: a dot-directory and another checkout are not this repository's
// source (internal/sourcewalk), except the dot-directories at the root
// trackedDotDirectories names, and neither is any directory unreadDirectories
// or unreadNames names. A symbolic link is not followed.
func scriptsInTree(root string) []string {
	var scripts []string
	scriptsBelow(root, "", &scripts)
	sort.Strings(scripts)
	return scripts
}

// scriptsBelow appends every script in the repository-relative directory dir
// of root, and below it.
func scriptsBelow(root, dir string, scripts *[]string) {
	entries, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	for _, entry := range entries {
		relative := path.Join(dir, entry.Name())
		full := filepath.Join(root, filepath.FromSlash(relative))
		if entry.IsDir() {
			if readsDirectory(relative, full) {
				scriptsBelow(root, relative, scripts)
			}
			continue
		}
		if entry.Type().IsRegular() && isScript(full, entry.Name()) {
			*scripts = append(*scripts, relative)
		}
	}
}

// readsDirectory reports whether the script rule reads a repository-relative
// directory: a dot-directory at the root this repository tracks always, and
// any other one that is not a dot-directory, another checkout, or one
// unreadDirectories or unreadNames names.
func readsDirectory(relative, full string) bool {
	if trackedDotDirectories[relative] {
		return true
	}
	return !unreadDirectories[relative] && !unreadNames[path.Base(relative)] && !sourcewalk.SkipDirBelowRoot(full)
}

// isScript reports whether a regular file is a script: by its extension, or
// for a name with none by the #! it opens with.
func isScript(full, name string) bool {
	if extension := path.Ext(name); extension != "" {
		return slices.Contains(scriptExtensions, extension)
	}
	file, err := openFile(full) //#nosec G304 -- an audit tool reading this repository's own tree.
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 2)
	_, err = io.ReadFull(file, head)
	return err == nil && string(head) == "#!"
}

// scriptProblems holds the scripts in the tree to the table of them: a script
// no declaration names, and every declaration that cannot be read as written,
// gives a category this command does not define for a script, gives no
// reason, names an item of issue 1114 it may not or none it must, or names no
// script in the tree; and an item of the ports table no pending script names.
//
// A script is declared by its own path or by the nearest directory, written
// with a trailing slash, that holds it.
func scriptProblems(found []string, declared tables) []string {
	var problems []string
	used := map[string]bool{}
	for _, script := range found {
		key, ok := scriptDeclarationFor(script, declared.scripts)
		if !ok {
			problems = append(problems, script+": a script that is not Go, which the table in "+declarationsFile+
				" does not declare: repository tooling is a Go command under cmd/, so port it, or declare why it has to stay")
			continue
		}
		used[key] = true
	}
	return append(problems, scriptTableProblems(declared, used)...)
}

// scriptDeclarationFor is the key that declares a script: its own path, or
// the nearest directory above it declared with a trailing slash.
func scriptDeclarationFor(script string, declared map[string]scriptDeclaration) (string, bool) {
	if _, ok := declared[script]; ok {
		return script, true
	}
	for dir := path.Dir(script); dir != "."; dir = path.Dir(dir) {
		if _, ok := declared[dir+"/"]; ok {
			return dir + "/", true
		}
	}
	return "", false
}

// scriptTableProblems names every declaration of a script that is wrong in
// itself or names nothing, in key order, then every item of the ports table
// no pending script names, in item order.
func scriptTableProblems(declared tables, used map[string]bool) []string {
	keys := make([]string, 0, len(declared.scripts))
	for key := range declared.scripts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	pending := map[int]bool{}
	for _, key := range keys {
		entry := declared.scripts[key]
		if trimmed := strings.TrimSuffix(key, "/"); trimmed == "." || path.Clean(trimmed) != trimmed || !filepath.IsLocal(filepath.FromSlash(trimmed)) {
			problems = append(problems, fmt.Sprintf("%s: %q is not a repository-relative path, or a directory of one ending in /", declarationsFile, key))
			continue
		}
		problems = append(problems, scriptEntryProblems(key, entry, declared.ports)...)
		if entry.category == scriptPortPending {
			pending[entry.item] = true
		}
		if !used[key] {
			problems = append(problems, fmt.Sprintf("%s: %q names no script in the tree", declarationsFile, key))
		}
	}
	items := make([]int, 0, len(declared.ports))
	for item := range declared.ports {
		items = append(items, item)
	}
	sort.Ints(items)
	for _, item := range items {
		if !pending[item] {
			problems = append(problems, fmt.Sprintf("%s: item %d of issue 1114 is named by no script pending its port, so it has landed and its line goes",
				declarationsFile, item))
		}
	}
	return problems
}

// scriptEntryProblems names what is wrong with one declaration in itself: a
// category this command does not define for a script, a port pending with no
// item of the ports table, an item named outside a pending port, and no reason
// where the category asks for one. A pending port's reason is its item's.
func scriptEntryProblems(key string, entry scriptDeclaration, ports map[int]string) []string {
	if scriptCategories[entry.category] == "" {
		return []string{fmt.Sprintf("%s: %q: the category %q is not one this command defines for a script", declarationsFile, key, entry.category)}
	}
	if entry.category == scriptPortPending {
		if ports[entry.item] == "" {
			return []string{fmt.Sprintf("%s: %q is pending a port and names no item of issue 1114 that ports it", declarationsFile, key)}
		}
		return nil
	}
	var problems []string
	if entry.item != 0 {
		problems = append(problems, fmt.Sprintf("%s: %q names item %d of issue 1114, which only a script pending its port does", declarationsFile, key, entry.item))
	}
	if strings.TrimSpace(entry.reason) == "" {
		problems = append(problems, fmt.Sprintf("%s: %q gives no reason", declarationsFile, key))
	}
	return problems
}
