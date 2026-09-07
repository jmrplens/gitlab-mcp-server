package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// loadMode is everything this audit needs: syntax to walk, and types so a
// constant expression can be folded into the one string GitLab would receive.
//
// NeedDeps is deliberately absent. A document assembled from a fragment is
// assembled inside the package that sends it, so type-checking the whole
// dependency tree from source would cost minutes and change no answer.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports

// document is one GraphQL document found in the source.
type document struct {
	// pkg is the import path of the package that declares it.
	pkg string
	// name is the constant or variable it is declared as, or "" when it is
	// written inline at the point of use.
	name string
	// position is where a reader will find it.
	position token.Position
	// text is the folded value, with any shared fragment already spliced in.
	text string
}

// label names a document for a report line.
func (d document) label() string {
	if d.name == "" {
		return "an inline document"
	}
	return d.name
}

// collector walks the loaded packages and gathers their documents.
type collector struct {
	fset      *token.FileSet
	documents []document
	// claimed holds the positions of expressions already reported under the
	// name they are declared as, so the inline pass does not report them a
	// second time without one.
	claimed map[token.Pos]bool
}

// collect loads the packages named by patterns, rooted at dir, and returns
// every GraphQL document they declare.
//
// The overlay is how a test supplies source that is not on disk: a fixture
// package written in the test file itself is type-checked like any other, so
// the folding of a document assembled from a fragment is exercised for real
// rather than mocked. Production passes nil.
func collect(dir string, patterns []string, overlay map[string][]byte) ([]document, error) {
	// The standalone files are read first because it costs milliseconds and
	// type-checking the tree costs seconds: a run that cannot read one of its
	// own documents should say so before paying for the rest.
	standalone, err := collectFiles(dir, patterns)
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{Mode: loadMode, Dir: dir, Tests: false, Overlay: overlay}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("no packages matched %s in %s", strings.Join(patterns, " "), dir)
	}

	gatherer := &collector{fset: loaded[0].Fset, claimed: map[token.Pos]bool{}}
	for _, pkg := range loaded {
		if loadErr := packageLoadError(pkg); loadErr != nil {
			return nil, loadErr
		}
		gatherer.walk(pkg)
	}

	gatherer.documents = append(gatherer.documents, standalone...)
	sortDocuments(gatherer.documents)
	return gatherer.documents, nil
}

// sortDocuments puts the findings in the order a reader walks a repository:
// by package, then by file, then by position within it. The file is part of the
// key because a package holds documents from several files and, for a
// standalone .graphql document, offset alone says nothing about which file it
// came from.
func sortDocuments(documents []document) {
	sort.Slice(documents, func(i, j int) bool {
		left, right := documents[i], documents[j]
		if left.pkg != right.pkg {
			return left.pkg < right.pkg
		}
		if left.position.Filename != right.position.Filename {
			return left.position.Filename < right.position.Filename
		}
		return left.position.Offset < right.position.Offset
	})
}

// collectFiles gathers the documents that live in .graphql files rather than in
// Go constants.
//
// A constant is the only shape this repository uses today, and it is not the
// only shape it may use tomorrow: moving a long document into its own file and
// pulling it in with an embed directive is the obvious next step for
// readability, and an embedded variable is not a constant, so the type checker
// folds nothing and the document would leave the inventory without a word.
// Reading the files directly closes that door before anybody walks through it.
//
// Every failure below the root is propagated rather than skipped. A directory
// this cannot read is a directory whose documents go unjudged, which is the
// silence the whole command exists to remove; a root that is not there at all
// is a question about the patterns, and the package loader answers that one.
//
// The walk goes through [os.Root], as cmd/format_md_tables does, so a read is
// scoped to the tree being audited rather than to whatever a symlink in it
// points at.
func collectFiles(dir string, patterns []string) ([]document, error) {
	var found []document
	for _, base := range walkRoots(dir, patterns) {
		root, err := os.OpenRoot(base)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", base, err)
		}
		collected, walkErr := documentsUnder(root, base)
		_ = root.Close()
		if walkErr != nil {
			return nil, walkErr
		}
		found = append(found, collected...)
	}
	return found, nil
}

// documentsUnder reads every standalone document in one already-opened tree.
func documentsUnder(root *os.Root, base string) ([]document, error) {
	var found []document
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || path.Ext(name) != graphQLExt || isPinnedSchema(name) {
			return err
		}
		local := filepath.FromSlash(name)
		text, readErr := root.ReadFile(local)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", filepath.Join(base, local), readErr)
		}
		full := filepath.Join(base, local)
		found = append(found, document{
			pkg:      filepath.ToSlash(filepath.Dir(full)),
			name:     path.Base(name),
			position: token.Position{Filename: full, Line: 1},
			text:     string(text),
		})
		return nil
	})
	return found, err
}

// graphQLExt is the extension a standalone document carries.
const graphQLExt = ".graphql"

// isPinnedSchema reports whether path is the pinned schema itself, which is an
// SDL and not a document anybody sends.
func isPinnedSchema(name string) bool {
	return path.Base(name) == graphqlschema.SDLFileName
}

// walkRoots turns the load patterns into the directories to search for
// standalone documents, so the two halves of the inventory cover the same tree.
func walkRoots(dir string, patterns []string) []string {
	roots := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.TrimSuffix(strings.TrimSuffix(pattern, "..."), "/")
		roots = append(roots, filepath.Join(dir, filepath.FromSlash(trimmed)))
	}
	return roots
}

// packageLoadError turns a package's load errors into one reportable error. A
// partially typed package folds no constants, so every document in it would go
// unseen, which is exactly the failure this audit must not have.
func packageLoadError(pkg *packages.Package) error {
	if len(pkg.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("load %s: %w", pkg.PkgPath, pkg.Errors[0])
}

// walk gathers one package's documents.
func (c *collector) walk(pkg *packages.Package) {
	if pkg.TypesInfo == nil {
		return
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool { return c.visit(pkg, node) })
	}
}

// visit records what one node declares or writes, and reports whether the walk
// should descend into it.
func (c *collector) visit(pkg *packages.Package, node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.ImportSpec:
		return false
	case *ast.ValueSpec:
		c.recordNamed(pkg, typed)
		// The walk still descends: a declaration's value may be a function
		// literal with a document inside it, and the values that were just
		// recorded are claimed so they are not reported twice.
		return true
	case *ast.BasicLit:
		c.recordInline(pkg, typed)
		return false
	case *ast.BinaryExpr:
		return !c.recordInline(pkg, typed)
	default:
		return true
	}
}

// recordNamed records every document a declaration gives a name to, covering
// a constant and a variable at package level and inside a function alike.
func (c *collector) recordNamed(pkg *packages.Package, spec *ast.ValueSpec) {
	if len(spec.Names) != len(spec.Values) {
		return
	}
	for i, name := range spec.Names {
		value := spec.Values[i]
		text, ok := constantDocument(pkg, value)
		if !ok {
			continue
		}
		c.claimed[value.Pos()] = true
		c.documents = append(c.documents, document{
			pkg:      pkg.PkgPath,
			name:     name.Name,
			position: c.fset.Position(name.Pos()),
			text:     text,
		})
	}
}

// recordInline records a document written where it is used rather than
// declared under a name. It reports whether it recorded one, which is what
// stops a concatenation from being reported again piece by piece.
func (c *collector) recordInline(pkg *packages.Package, expr ast.Expr) bool {
	if c.claimed[expr.Pos()] {
		return true
	}
	text, ok := constantDocument(pkg, expr)
	if !ok {
		return false
	}
	c.documents = append(c.documents, document{
		pkg:      pkg.PkgPath,
		position: c.fset.Position(expr.Pos()),
		text:     text,
	})
	return true
}

// constantDocument returns the folded string value of expr when it is a
// GraphQL document.
func constantDocument(pkg *packages.Package, expr ast.Expr) (string, bool) {
	typed, ok := pkg.TypesInfo.Types[expr]
	if !ok || typed.Value == nil || typed.Value.Kind() != constant.String {
		return "", false
	}
	text := constant.StringVal(typed.Value)
	if !looksLikeDocument(text) {
		return "", false
	}
	return text, true
}
