// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// The files the copy adds to a package whose external test package imports
// nothing of it: gobco's bridge copies an import of the package it finds, so
// the copy writes one, which reads a name a file of the package declares
// behind a constraint gobco does not instrument.
const (
	stagedImportFile = "gobco_staged_import_gobco.go"
	stagedImportTest = "gobco_staged_import_test.go"
)

// uninstrumented opens a file the go command builds and gobco's narrower
// context does not, so gobco leaves it as it is.
const uninstrumented = "//go:build gc\n\n"

// stage says what the package is staged for, if it is, and copies the module
// into a temporary directory with the package brought to what gobco can read.
// It returns the directory gobco is to run in, which is the package's own
// when nothing is staged, and what removes the copy, whatever happened.
func (m *measurement) stage() (target string, cleanup func(), err error) {
	if !m.staged.needed() {
		return m.pkg.Dir, func() {}, nil
	}
	m.announceStaging()
	rel, err := m.packageBelowModule()
	if err != nil {
		return "", func() {}, err
	}
	workdir, err := os.MkdirTemp("", "coverage-conditions-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(workdir) }
	module := filepath.Join(workdir, path.Base(m.pkg.ModuleDir))
	err = recipe.CopyTree(m.pkg.ModuleDir, module, leftOutOfCopy)
	if err != nil {
		return "", cleanup, err
	}
	err = heldByCopy(module, rel)
	if err != nil {
		return "", cleanup, err
	}
	target = filepath.Join(module, filepath.FromSlash(rel))
	return target, cleanup, m.rewrite(target)
}

// heldByCopy reports as an error a package the copy of its module in module
// does not hold: one below a directory the copy leaves out, and one reached
// through a symbolic link. The copy keeps a link as it is, so what the staging
// removes and writes there would be removed from and written to the original.
func heldByCopy(module, rel string) error {
	var at string
	for element := range strings.SplitSeq(rel, "/") {
		at = path.Join(at, element)
		info, err := os.Lstat(filepath.Join(module, filepath.FromSlash(at)))
		if err != nil {
			return err
		}
		if info.Mode().Type() == fs.ModeSymlink {
			return fmt.Errorf("%s is a symbolic link, which the copy would follow back to the original", at)
		}
	}
	return nil
}

// packageBelowModule is the package's directory relative to its module's, and
// "." for the module's own.
func (m *measurement) packageBelowModule() (string, error) {
	if m.pkg.Dir == m.pkg.ModuleDir {
		return ".", nil
	}
	rel, below := strings.CutPrefix(m.pkg.Dir, m.pkg.ModuleDir+"/")
	if !below || m.pkg.ModuleDir == "" {
		return "", fmt.Errorf("%s is not below the directory of its module, %q", m.pkg.Dir, m.pkg.ModuleDir)
	}
	return rel, nil
}

// announceStaging says what gobco cannot read in the package, and how the
// copy answers each.
func (m *measurement) announceStaging() {
	s := m.staged
	if len(s.leftOut) > 0 {
		m.sayf("%s holds %d .go file(s) the go command does not build on %s (%s), which gobco would parse with the rest (issue 1017); measuring it in a staged copy of the module without them. The conditions of the files left out are not measured by this run: only a run on a platform that builds them could measure them",
			m.Package, len(s.leftOut), m.context(), joined(s.leftOut))
		if len(s.constrained) > 0 {
			m.sayf("the copy blanks the build constraint lines of %s, line for line, since each names more than the platform and gobco's own narrower build context would decline to instrument it", joined(s.constrained))
		}
	} else if len(s.constrained) > 0 {
		m.sayf("%s builds %s under a build constraint naming more than the platform, which gobco's own narrower build context would decline to instrument; measuring it in a staged copy of the module with those lines blanked, line for line, so gobco instruments every file the go command builds here", m.Package, joined(s.constrained))
	}
	if len(s.support) > 0 {
		m.sayf("%s has an external test package, which gobco type-checks against the package's non-test files alone, and test files of its own that declare no test (%s), whose names that package may use; measuring it in a staged copy of the module in which those files are part of the package, behind //go:build gc, which the go command builds and gobco's own build context does not instrument, so they are compiled into the test binary as before and stay out of the report as a test file does", m.Package, joined(s.support))
	}
	if s.unimported {
		m.sayf("%s has an external test package that does not import it, and gobco bridges that package to the instrumented one through an import it finds there; measuring it in a staged copy of the module with a test file in that package that imports it, beside a file of the package behind //go:build gc that declares the one name the import reads", m.Package)
	}
}

// rewrite brings the package's copy in target to what gobco can read: the
// files the go command does not build removed, the constraint lines gobco
// would misjudge blanked, the support files made files of the package, and an
// import of the package added where there is none.
func (m *measurement) rewrite(target string) error {
	var failed []error
	for _, name := range m.staged.leftOut {
		// One that went since it was listed is left out already.
		if err := os.Remove(filepath.Join(target, name)); !errors.Is(err, fs.ErrNotExist) {
			failed = append(failed, err)
		}
	}
	for _, name := range m.staged.constrained {
		failed = append(failed, rewriteFile(filepath.Join(target, name), filepath.Join(target, name), blankConstraints))
	}
	// The file builds here, so any constraint of its own already holds, and
	// its lines are blanked rather than combined with the one it opens with.
	for _, name := range m.staged.support {
		from := filepath.Join(target, name)
		to := filepath.Join(target, strings.TrimSuffix(name, "_test.go")+"_gobco.go")
		failed = append(failed, rewriteFile(from, to, func(text string) string { return uninstrumented + blankConstraints(text) }), os.Remove(from))
	}
	if m.staged.unimported {
		name := m.pkg.Name
		declares := uninstrumented + "package " + name + "\n\n// GobcoStagedImport is what " + stagedImportTest + " reads.\nvar GobcoStagedImport struct{}\n"
		imports := "package " + name + "_test\n\nimport " + name + " \"" + m.pkg.ImportPath + "\"\n\nvar _ = " + name + ".GobcoStagedImport\n"
		failed = append(failed, replaceFile(filepath.Join(target, stagedImportFile), declares),
			replaceFile(filepath.Join(target, stagedImportTest), imports))
	}
	return errors.Join(failed...)
}

// rewriteFile writes to to what change makes of the text of from.
func rewriteFile(from, to string, change func(string) string) error {
	text, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return replaceFile(to, change(string(text)))
}

// replaceFile writes a file of the copy anew. What holds its name is removed
// first: the copy keeps symbolic links, and a write through one would change
// what it links to, which may be a file of the tree being measured.
func replaceFile(name, text string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.WriteFile(name, []byte(text), 0o600)
}

// blankConstraints blanks the build constraint lines of a file's header, line
// for line, so every position in it stays where it was.
func blankConstraints(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if isPackageClause(line) {
			break
		}
		if _, constrained := constraintOf(line); constrained {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// leftOutOfCopy reports whether the copy of a module leaves out the entry at
// rel, its path relative to the module's root.
func leftOutOfCopy(rel string) bool {
	switch rel {
	case ".git", ".claude/worktrees", "dist":
		return true
	default:
		return path.Base(rel) == "node_modules"
	}
}
