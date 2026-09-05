// docgo.go moves package comments into a doc.go of their own, which is the
// one place the audit accepts them.

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// packageDocFile is the file a package comment lives in. A package comment
// anywhere else is one file header away from being mistaken for one, and one
// more file away from being duplicated, so the convention gives it one home
// and the audit reports every other.
const packageDocFile = "doc.go"

// Seams over the standard library, so a test can drive the failure branches
// that a filesystem the tests own, and run as root, never produces on its own:
// a walk that reports an error, a format of bytes that were just parsed, and a
// write that cannot land.
var (
	walkDir      = filepath.WalkDir
	formatSource = format.Source
	writeFile    = os.WriteFile
)

// movePackageDocs applies movePackageDoc to a directory and everything below
// it, or to the directory of a file. Hidden directories, testdata and vendor
// trees are skipped, since none of them holds a package of this repository.
func movePackageDocs(path string) error {
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", cleanPath, err)
	}
	if !info.IsDir() {
		return movePackageDoc(filepath.Dir(cleanPath))
	}
	var errs []error
	walkErr := walkDir(cleanPath, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if name := d.Name(); p != cleanPath && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
			return fs.SkipDir
		}
		if moveErr := movePackageDoc(p); moveErr != nil {
			errs = append(errs, moveErr)
		}
		return nil
	})
	return errors.Join(append(errs, walkErr)...)
}

// packageDocHolder is a source file that carries the package comment.
type packageDocHolder struct {
	path string
	file *ast.File
	src  []byte
	fset *token.FileSet
}

// movePackageDoc moves the package comment of the package in dir into
// doc.go, when a well-formed one lives in another file and no doc.go exists.
// The comment is copied verbatim above the package clause of the new file and
// cut from the file it came from, whose build constraint, if any, stays
// where it was. A malformed comment is left for the audit to report rather
// than enshrined as the package's documentation.
func movePackageDoc(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", dir, err)
	}
	var holder *packageDocHolder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == packageDocFile {
			return nil
		}
		if holder != nil {
			continue
		}
		path := filepath.Join(dir, name)
		src, readErr := os.ReadFile(path) //#nosec G304 -- paths come from CLI args, not user input
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, src, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		if file.Doc == nil || !validPackageDoc(file.Name.Name, strings.TrimSpace(file.Doc.Text())) {
			continue
		}
		holder = &packageDocHolder{path: path, file: file, src: src, fset: fset}
	}
	if holder == nil {
		return nil
	}
	return holder.moveToDocGo(dir)
}

// moveToDocGo writes the holder's package comment into doc.go and cuts it
// from the holder.
func (h *packageDocHolder) moveToDocGo(dir string) error {
	docStart := h.fset.Position(h.file.Doc.Pos()).Offset
	docEnd := h.fset.Position(h.file.Doc.End()).Offset
	pkgStart := h.fset.Position(h.file.Package).Offset
	if docStart < 0 || docEnd > pkgStart || pkgStart > len(h.src) {
		return fmt.Errorf("%s: package comment does not sit above the package clause", h.path)
	}

	docGo := append(append([]byte{}, h.src[docStart:docEnd]...), []byte("\npackage "+h.file.Name.Name+"\n")...)
	docGo, err := formatSource(docGo)
	if err != nil {
		return fmt.Errorf("format doc.go for %s: %w", dir, err)
	}
	remaining := append(append([]byte{}, h.src[:docStart]...), h.src[pkgStart:]...)
	remaining, err = formatSource(remaining)
	if err != nil {
		return fmt.Errorf("format %s without its package comment: %w", h.path, err)
	}

	docPath := filepath.Join(dir, packageDocFile)
	if dryRun {
		fmt.Printf("// dry-run: would move the package comment of %s from %s into %s\n", h.file.Name.Name, filepath.Base(h.path), docPath)
		return nil
	}
	if writeErr := writeFile(docPath, docGo, 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", docPath, writeErr)
	}
	if writeErr := writeFile(h.path, remaining, 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", h.path, writeErr)
	}
	fmt.Printf("moved the package comment of %s from %s into %s\n", h.file.Name.Name, filepath.Base(h.path), docPath)
	return nil
}
