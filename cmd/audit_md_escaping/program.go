package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/goprogram"
)

// modulePath is this repository's module path, trimmed off an import path so a
// report names a package the way the repository does.
const modulePath = "github.com/jmrplens/gitlab-mcp-server/v2"

// toolutilPath is the import path of the package that owns the escaping
// helpers. Resolution keys on the path rather than on the package name so an
// import alias cannot fool it.
const toolutilPath = modulePath + "/internal/toolutil"

// program is the loaded, indexed source the audit reasons over.
type program struct {
	fset  *token.FileSet
	order []*packages.Package
	// decls maps a declared function to its body and the package it was
	// written in, so a return expression can be resolved in its own scope.
	decls map[*types.Func]*funcDecl
	// callers maps a declared function to every argument list it is called
	// with, which is how a parameter is resolved to the values that reach it.
	callers map[*types.Func][]callSite
}

// funcDecl is one declared function the audit may need to look inside.
type funcDecl struct {
	obj  *types.Func
	pkg  *packages.Package
	decl *ast.FuncDecl
}

// callSite is one call of a declared function, kept with the package it was
// written in so its arguments resolve in their own scope.
type callSite struct {
	call *ast.CallExpr
	pkg  *packages.Package
}

// loadProgram loads and indexes the packages named by patterns, rooted at dir.
//
// The load itself, including the refusal of a package that did not type-check,
// belongs to [goprogram.Load]; what is here is the indexing this audit needs.
// The overlay is passed straight through: it is how a test supplies source
// that is not on disk, so a fixture package written in the test file itself
// type-checks against the real toolutil and the classifier is exercised on the
// shapes it has to handle rather than on a mock of them. Production passes nil.
func loadProgram(dir string, patterns []string, overlay map[string][]byte) (*program, error) {
	loaded, err := goprogram.Load(dir, patterns, overlay)
	if err != nil {
		return nil, err
	}
	prog := &program{
		fset:    loaded[0].Fset,
		order:   loaded,
		decls:   make(map[*types.Func]*funcDecl),
		callers: make(map[*types.Func][]callSite),
	}
	for _, pkg := range prog.order {
		prog.indexDecls(pkg)
	}
	for _, pkg := range prog.order {
		prog.indexCalls(pkg)
	}
	return prog, nil
}

// indexDecls records every declared function's body.
func (p *program) indexDecls(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if obj, isFunc := pkg.TypesInfo.Defs[fn.Name].(*types.Func); isFunc {
				p.decls[obj] = &funcDecl{obj: obj, pkg: pkg, decl: fn}
			}
		}
	}
}

// indexCalls records where each declared function is called, so a parameter
// can be resolved to every value that reaches it.
func (p *program) indexCalls(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callee := calleeOf(pkg, call); callee != nil {
				p.callers[callee] = append(p.callers[callee], callSite{call: call, pkg: pkg})
			}
			return true
		})
	}
}

// calleeOf returns the function a call names, or nil when the call is of a
// value rather than of a declared function.
func calleeOf(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	var ident *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return nil
	}
	fn, _ := pkg.TypesInfo.Uses[ident].(*types.Func)
	return fn
}

// position renders a source position.
func (p *program) position(pos token.Pos) token.Position {
	return p.fset.Position(pos)
}
