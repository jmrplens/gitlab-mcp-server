// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
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
	// it (`Type.Method` for a method), `func` for one inside a function
	// literal no function declaration encloses, and empty for one at package
	// scope. It is part of the constant's identity in the declaration table:
	// a local constant and a package-level one may share a name, and a
	// declaration excusing the one must not excuse the other.
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
// the summary of every tested package. A package a load left Go files out of
// is what a build constraint this platform does not satisfy looks like.
func (s *scanner) observe(loaded []*packages.Package) {
	underTest := goprogram.PackagesUnderTest(loaded)
	for _, pkg := range loaded {
		if goprogram.IsTestMain(pkg, underTest) {
			continue
		}
		s.packages[goprogram.TrimModulePath(goprogram.VariantName(pkg.PkgPath))] = struct{}{}
		if len(goprogram.IgnoredGoFiles(pkg)) > 0 {
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
			switch open[len(open)-1].(type) {
			case *ast.FuncDecl, *ast.FuncLit:
				funcs = funcs[:len(funcs)-1]
			}
			open = open[:len(open)-1]
			return true
		}
		open = append(open, node)
		switch fn := node.(type) {
		case *ast.FuncDecl:
			funcs = append(funcs, sourcegate.FuncDeclName(fn))
		case *ast.FuncLit:
			funcs = append(funcs, funcLitName(funcs))
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
				Package:   goprogram.TrimModulePath(goprogram.VariantName(pkg.PkgPath)),
				File:      goprogram.RelativePath(at.file, s.root),
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

// funcLitName is what a function literal's constants are keyed under: the
// function around the literal when there is one, since a closure's constants
// are that function's locals, and `func` when the literal sits in a
// package-level declaration. `func` is a keyword, so it is no function's name
// and no constant's, and a constant in such a literal can share its key with
// neither.
func funcLitName(enclosing []string) string {
	if len(enclosing) > 0 {
		return enclosing[len(enclosing)-1]
	}
	return "func"
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
