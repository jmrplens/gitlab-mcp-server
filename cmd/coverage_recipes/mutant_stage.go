// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// staged is the copy of a package gremlins is run over, and is empty for one
// it is run over where it is: the copy's directory, and whether it sits below
// the package rather than beside it.
type staged struct {
	dir   string
	below bool
}

// needsStaging reports whether gremlins cannot resolve the package where it
// is: whether its directory does not end in its package clause's name. One
// that does is run where it is.
func needsStaging(pkg golist.PackageFiles) bool {
	return !strings.HasSuffix(path.Base(pkg.Dir), pkg.Name)
}

// importsOwnInternal reports whether the package, or either kind of its tests,
// imports the package's own internal subtree. An import path is read whole,
// never as a prefix of a longer element: X/internalx is no internal subtree.
func importsOwnInternal(pkg golist.PackageFiles) bool {
	own := pkg.ImportPath + "/internal"
	return slices.ContainsFunc(slices.Concat(pkg.Imports, pkg.TestImports, pkg.XTestImports), func(imported string) bool {
		return imported == own || strings.HasPrefix(imported, own+"/")
	})
}

// stage copies the package where gremlins can resolve it, and returns what
// removes the copy, whatever happened. Left behind it is a second copy of the
// package inside the module, which every tree-wide `go build ./...` and every
// gate that loads ./... would then read as real.
//
// The copy sits next to the package rather than somewhere tidy like dist/,
// because Go's internal rule is about the path: a copy of a cmd/ command
// staged under dist/ cannot import cmd/internal/... and fails at setup. Beside
// the package, every import of an internal subtree rooted above it is still
// legal, and so is every relative path its tests climb out by, since the copy
// is at the same depth. The package's own internal subtree is the exception:
// X/internal/... is importable from X and below it and from nowhere beside it,
// so a package importing it, from its own files or from either kind of test
// file, is staged one level below itself. Every other internal subtree the
// package may import is rooted at one of its ancestors, and so at an ancestor
// of the place beside it too, which is why its own is the one that decides.
//
// ".mutants-" is in the name so the path cannot be one somebody meant to keep,
// and an existing one is refused rather than removed: the recipe deletes what
// it creates and nothing else, and a leftover means a previous run was killed
// hard enough to skip its own cleanup, which a person should see rather than
// have quietly overwritten. Both places are looked at, since a leftover of
// either is a second copy of the package in the module whichever place this
// run would use.
func (m *mutation) stage(ctx context.Context) (cleanup func(), err error) {
	cleanup = func() {}
	if !needsStaging(m.pkg) {
		return cleanup, nil
	}
	name := path.Base(m.pkg.Dir) + ".mutants-" + m.pkg.Name
	beside, inside := path.Dir(m.pkg.Dir)+"/"+name, m.pkg.Dir+"/"+name
	for _, leftover := range []string{beside, inside} {
		if _, statErr := os.Lstat(leftover); !errors.Is(statErr, os.ErrNotExist) {
			return cleanup, fmt.Errorf("%s is already there, which means a previous staged run did not clean up after itself; remove it and try again", m.fromRoot(leftover))
		}
	}
	at, where := staged{dir: beside}, ""
	if importsOwnInternal(m.pkg) {
		at = staged{dir: inside, below: true}
		where = ", one level below it, since it imports its own internal/ subtree, which nothing beside it may import"
	}
	nested, err := m.nestedPackages(ctx)
	if err != nil {
		return cleanup, fmt.Errorf("go list could not name the packages below %s: %w; refusing to measure", m.name, err)
	}
	m.sayf("%s is package %s in a directory that does not end in \"%s\", which gremlins cannot resolve (issue 872); measuring a staged copy at %s%s", m.name, m.pkg.Name, m.pkg.Name, m.fromRoot(at.dir), where)
	m.staged = at
	cleanup = func() { _ = os.RemoveAll(at.dir) }
	err = recipe.CopyTree(m.pkg.Dir, at.dir, func(rel string) bool {
		// The copy itself, when it sits below the package it copies.
		return rel == name || slices.Contains(nested, rel)
	})
	if err != nil {
		return cleanup, fmt.Errorf("the staged copy of %s could not be made: %w; refusing to measure", m.name, err)
	}
	return cleanup, nil
}

// nestedPackages are the packages below this one, as go names them, by their
// path relative to it, which the copy leaves out: a copy of them would be a
// second package that nothing imports and that may not build where it lands,
// and its tests would be timed into every mutant's deadline although none of
// its files is mutated. They are listed from the package's own directory,
// which names its subtree whether or not it is the module root, and before
// the copy exists, which would otherwise be listed as one of them.
func (m *mutation) nestedPackages(ctx context.Context) ([]string, error) {
	out, err := m.goOutput(ctx, filepath.FromSlash(m.pkg.Dir), slices.Concat([]string{"list", "-e"}, m.flags.tagArgs(), []string{"-f", "{{.Dir}}", "./..."})...)
	if err != nil {
		return nil, err
	}
	var nested []string
	for line := range strings.Lines(strings.ReplaceAll(out, `\`, "/")) {
		if rel, below := strings.CutPrefix(strings.TrimSpace(line), m.pkg.Dir+"/"); below {
			nested = append(nested, rel)
		}
	}
	return nested, nil
}

// fromRoot is a path below the module root relative to it, which is how the
// recipe names a directory to a reader, to go test and to gremlins. The root
// and the package directory are both written with slashes, so the cut works on
// Windows as well; a strip expecting a slash on a native path once stripped
// nothing there and left a pattern naming a directory that does not exist,
// which go test fails and the recipe would then report as a failing suite.
func (m *mutation) fromRoot(dir string) string {
	return strings.TrimPrefix(dir, m.root+"/")
}

// patterns returns the two patterns a run over dir names, both from the module
// root. scan is the run gremlins times, its coverage step, over the
// directory's subtree or over the whole module under --integration. each is
// the command gremlins runs against each mutant: the package itself, since the
// default exclusion keeps every mutant in it, or the whole subtree when the
// caller's own exclusion may leave packages below it to be mutated, or the
// whole module under --integration.
func (m *mutation) patterns(dir string) (scan, each string) {
	scan, each = "./...", "."
	if dir != m.root {
		each = "./" + m.fromRoot(dir)
		if !m.flags.integration {
			scan = each + "/..."
		}
	}
	if m.flags.integration || m.flags.excluded {
		each = scan
	}
	return scan, each
}
