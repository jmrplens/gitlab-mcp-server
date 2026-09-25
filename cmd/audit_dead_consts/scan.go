package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// Constant is one unexported constant declared in this repository.
type Constant struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	// Column is where the name starts on its line. A finding is printed as
	// file:line, so the column is not shown; it is what orders two constants
	// declared on one line, `const a, b = ...`, the way the line declares
	// them, where the order used to come from the map the scan collects into
	// and two runs over one tree printed the report two ways.
	Column int    `json:"column"`
	Name   string `json:"name"`
	// Func is the function a constant is declared inside, as the file spells
	// it (`Type.Method` for a method), and empty for one at package scope. It
	// is part of the constant's identity in the declaration table: a local
	// constant and a package-level one may share a name, and a declaration
	// excusing the one must not excuse the other.
	Func string `json:"func,omitempty"`
	// GroupSize is how many constants the declaration it sits in declares.
	// Anything above one is the shape staticcheck's unused cannot see, and the
	// report says so per finding rather than only in prose, because that
	// number is what tells a reader whether the linter already had its chance.
	GroupSize int `json:"group_size"`
}

// position is a declaration's place in the tree, which is what identity is
// keyed on here rather than the object the type checker made.
//
// One source constant is type-checked once per package variant and once per
// platform load, and each of those runs mints its own *types.Const with its
// own pointer. Two loads do not even share a token.FileSet, so a token.Pos
// from one means nothing in the other. The file, line and column do mean the
// same thing everywhere, which is what lets the uses recorded by every load be
// unioned against the declarations found by every load.
type position struct {
	file string
	line int
	col  int
}

// scanner accumulates what one or more loads of the same tree declared and
// what they read, so the answer is a union rather than one load's view.
type scanner struct {
	root     string
	declared map[position]Constant
	used     map[position]struct{}
	// packages are the packages this run looked at, named the way the
	// repository names them. The declaration table is held against this set
	// rather than against the whole tree, so a run over one package does not
	// report every declaration elsewhere as stale.
	packages map[string]struct{}
	// platformPackages are the packages a load left Go files out of, which is
	// what a GOOS-constrained file looks like from the platform it is not for.
	// They are re-read under the other platforms, since a constant read only
	// by the Windows half of a package is read. Keyed by the plain import
	// path, which is the one name a second load can be asked for and which
	// the test variants share with the package they test: asking for it with
	// Tests set brings the variants with it, and the go tool builds the
	// external test package and the synthesized main fresh, so neither ever
	// carries a left-out file of its own.
	platformPackages map[string]struct{}
}

// newScanner returns a scanner rooted at an absolute repository path.
func newScanner(root string) *scanner {
	return &scanner{
		root:             root,
		declared:         map[position]Constant{},
		used:             map[position]struct{}{},
		packages:         map[string]struct{}{},
		platformPackages: map[string]struct{}{},
	}
}

// observe records the declarations and the uses of one load.
//
// The test main the go tool synthesizes for a package (`p.test`) is passed
// over: its one generated file is nobody's source, declares no constant and
// reads none, and counting it put a package the repository never wrote in
// the summary of every tested package.
func (s *scanner) observe(loaded []*packages.Package) {
	underTest := packagesUnderTest(loaded)
	for _, pkg := range loaded {
		if isTestMain(pkg, underTest) {
			continue
		}
		s.packages[trimModulePath(variantName(pkg.PkgPath))] = struct{}{}
		if hasIgnoredGoFile(pkg) {
			s.platformPackages[pkg.PkgPath] = struct{}{}
		}
		for _, file := range pkg.Syntax {
			s.observeFile(pkg, file)
		}
		for _, obj := range pkg.TypesInfo.Uses {
			if constant, isConst := obj.(*types.Const); isConst {
				s.used[s.positionOf(pkg, constant.Pos())] = struct{}{}
			}
		}
	}
}

// observeFile records every unexported constant one file declares, at package
// scope and inside a function alike: an unread constant in a function body is
// no more read than one beside it, and the compiler refuses neither.
func (s *scanner) observeFile(pkg *packages.Package, file *ast.File) {
	// ast.Inspect announces a node's end with a nil call and names no node, so
	// the path of open nodes is kept here to know which function a
	// declaration sits in and when that function has been left.
	var open []ast.Node
	var funcs []string
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			if _, wasFunc := open[len(open)-1].(*ast.FuncDecl); wasFunc {
				funcs = funcs[:len(funcs)-1]
			}
			open = open[:len(open)-1]
			return true
		}
		open = append(open, node)
		if fn, isFunc := node.(*ast.FuncDecl); isFunc {
			funcs = append(funcs, funcDeclName(fn))
		}
		decl, isDecl := node.(*ast.GenDecl)
		if !isDecl || decl.Tok != token.CONST {
			return true
		}
		enclosing := ""
		if len(funcs) > 0 {
			enclosing = funcs[len(funcs)-1]
		}
		names := constNames(decl)
		for _, name := range names {
			constant, isConst := pkg.TypesInfo.Defs[name].(*types.Const)
			if !isConst || constant.Exported() {
				continue
			}
			at := s.positionOf(pkg, name.Pos())
			s.declared[at] = Constant{
				Package:   trimModulePath(variantName(pkg.PkgPath)),
				File:      relativePath(at.file, s.root),
				Line:      at.line,
				Column:    at.col,
				Name:      constant.Name(),
				Func:      enclosing,
				GroupSize: len(names),
			}
		}
		return true
	})
}

// funcDeclName spells a function the way its file does: the bare name, or
// `Type.Method` with the receiver's type stripped of its pointer.
//
// The receiver is read through its parentheses, as the type checker reads
// it: `(w (walker))` and `(w *(walker))` declare methods of walker as much as
// `(w walker)` does, and a key that dropped the receiver there would excuse
// nothing a reader wrote for it.
func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := ast.Unparen(fn.Recv.List[0].Type)
	if star, isPointer := receiver.(*ast.StarExpr); isPointer {
		receiver = ast.Unparen(star.X)
	}
	switch generic := receiver.(type) {
	case *ast.IndexExpr:
		receiver = generic.X
	case *ast.IndexListExpr:
		receiver = generic.X
	}
	if ident, isIdent := receiver.(*ast.Ident); isIdent {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// dead is every declared constant no load recorded a use of, in source order.
func (s *scanner) dead() []Constant {
	var found []Constant
	for at, constant := range s.declared {
		if _, read := s.used[at]; read {
			continue
		}
		found = append(found, constant)
	}
	sortConstants(found)
	return found
}

// positionOf renders a position the way every load can agree on it.
func (s *scanner) positionOf(pkg *packages.Package, pos token.Pos) position {
	at := pkg.Fset.Position(pos)
	return position{file: at.Filename, line: at.Line, col: at.Column}
}

// constNames are the identifiers one const declaration binds, blanks left out.
// A blank is a real declaration and is never read by anything, so counting it
// would report every iota placeholder in the tree.
func constNames(decl *ast.GenDecl) []*ast.Ident {
	var names []*ast.Ident
	for _, spec := range decl.Specs {
		valueSpec, isValueSpec := spec.(*ast.ValueSpec)
		if !isValueSpec {
			continue
		}
		for _, name := range valueSpec.Names {
			if name.Name == "_" {
				continue
			}
			names = append(names, name)
		}
	}
	return names
}

// hasIgnoredGoFile reports whether a load left a Go file of this package out,
// which is what a build constraint this platform does not satisfy looks like.
func hasIgnoredGoFile(pkg *packages.Package) bool {
	for _, file := range pkg.IgnoredFiles {
		if strings.HasSuffix(file, ".go") {
			return true
		}
	}
	return false
}

// packagesUnderTest collects, from one load, the import paths the loader says
// a test executable was built for. `go list` reports a test variant as
// "p [q.test]" and fills [packages.Package.ForTest] with q for it; the
// executable itself arrives as plain "q.test" with ForTest empty, so the
// variants are where the name of the package under test can be read.
func packagesUnderTest(loaded []*packages.Package) map[string]struct{} {
	underTest := make(map[string]struct{})
	for _, pkg := range loaded {
		if pkg.ForTest != "" {
			underTest[pkg.ForTest] = struct{}{}
		}
	}

	return underTest
}

// isTestMain reports whether a loaded package is the test executable the go
// tool synthesizes for a package under test, "q.test", which is neither source
// of this repository nor a name a second load can be asked for.
//
// The path alone does not answer it. A directory may be named "foo.test", and
// the package in it then carries a path ending exactly as the synthesized
// executable does while being source somebody wrote; passing over it would
// drop every constant it declares and report the package clean. So the path is
// only believed when the loader also produced a test variant for the package
// the suffix claims this was built for, which is a fact about the load rather
// than a guess about a name.
func isTestMain(pkg *packages.Package, underTest map[string]struct{}) bool {
	if pkg.Name != "main" || !strings.HasSuffix(pkg.PkgPath, ".test") {
		return false
	}
	_, built := underTest[strings.TrimSuffix(pkg.PkgPath, ".test")]

	return built
}

// variantName is a package's import path with the test-variant decoration the
// loader adds taken off, so "p", "p [p.test]" and "p_test [p.test]" are one
// package. The suffix is decoration on the path rather than part of it, and
// the external test package keeps its own "_test" name because it really is a
// different package.
func variantName(pkgPath string) string {
	plain, _, _ := strings.Cut(pkgPath, " [")
	return plain
}

// trimModulePath names a package the way the repository does.
func trimModulePath(pkgPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(pkgPath, goprogram.ModulePath), "/")
}

// relativePath renders a file below the repository root, so a finding reads
// the same wherever the audit was run from.
func relativePath(path, root string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}
