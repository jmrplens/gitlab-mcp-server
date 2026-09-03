package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadMode is everything the audit needs from the loader: syntax to walk and
// types to resolve an identifier to the object it names.
//
// NeedDeps is deliberately absent. Every function this audit follows lives in
// the patterns it loads, so type-checking the whole dependency tree from
// source would buy nothing and cost minutes; dependencies come in through
// export data, which still gives each of their objects one identity shared
// with the packages that use them.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports

// toolutilPath is the import path of the package that owns ActionSpec, the
// route constructors, and the shared GraphQL executors. Resolution keys on the
// path rather than on the package name so an import alias cannot fool it.
const toolutilPath = "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

// program is the loaded, indexed source the audit reasons over.
type program struct {
	fset *token.FileSet
	// pkgs is every loaded package keyed by import path.
	pkgs map[string]*packages.Package
	// funcs maps a declared function to its indexed body.
	funcs map[*types.Func]*function
	// documents maps a string constant or package-level string variable to
	// what its value asks GitLab to do.
	documents map[types.Object]documentKind
	// order is the loaded packages in the loader's order.
	order []*packages.Package
}

// docRef is one GraphQL document named inside a function body.
type docRef struct {
	kind documentKind
	// name is the constant this document was declared as, or "" when the
	// document is written inline at the point of use.
	name string
	// pos is where the body names it, which is the line a finding points at.
	pos token.Pos
	// declared is where the document itself is written.
	declared token.Pos
}

// function is one declared function with everything the audit reads from it.
type function struct {
	obj  *types.Func
	pkg  *packages.Package
	decl *ast.FuncDecl
	// calls is every function this body names. A reference counts as a call:
	// a function value handed to a helper is called by that helper, and the
	// audit would rather over-approximate the reachable set than miss a
	// mutation reached through a callback.
	calls map[*types.Func]bool
	// docs is every GraphQL document this body names.
	docs []docRef
	// sendsGraphQL reports whether this body reaches the GraphQL transport,
	// which is what makes a document it names a request rather than a string.
	sendsGraphQL bool
}

// graphQLSenders are the entry points that put a GraphQL document on the wire:
// the client-go GraphQL service method every call here ultimately reaches, and
// the shared toolutil executors that call it with a document their caller
// supplied.
var graphQLSenders = map[string]bool{
	"Do":                      true,
	"ExecGraphQLNoteMutation": true,
	"ExecGraphQLDestroyNote":  true,
}

// loadProgram loads and indexes the packages named by patterns, rooted at dir.
//
// The overlay is how a test supplies source that is not on disk: a fixture
// package written in the test file itself loads and type-checks against the
// real toolutil, so the resolver is exercised on the shapes it has to handle
// rather than on a mock of them. Production passes nil.
func loadProgram(dir string, patterns []string, overlay map[string][]byte) (*program, error) {
	cfg := &packages.Config{Mode: loadMode, Dir: dir, Tests: false, Overlay: overlay}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("no packages matched %s in %s", strings.Join(patterns, " "), dir)
	}
	prog := &program{
		fset:      loaded[0].Fset,
		pkgs:      make(map[string]*packages.Package, len(loaded)),
		funcs:     make(map[*types.Func]*function),
		documents: make(map[types.Object]documentKind),
	}
	for _, pkg := range loaded {
		if loadErr := packageLoadError(pkg); loadErr != nil {
			return nil, loadErr
		}
		prog.pkgs[pkg.PkgPath] = pkg
		prog.order = append(prog.order, pkg)
	}
	for _, pkg := range prog.order {
		prog.indexDocuments(pkg)
	}
	for _, pkg := range prog.order {
		prog.indexFunctions(pkg)
	}
	return prog, nil
}

// packageLoadError turns a package's load errors into one reportable error. A
// partially typed package would make every resolution below silently
// unresolvable, which is exactly the failure mode this audit must not have.
func packageLoadError(pkg *packages.Package) error {
	if len(pkg.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("load %s: %w", pkg.PkgPath, pkg.Errors[0])
}

// indexDocuments records every string constant and package-level string
// variable whose value is a GraphQL document.
//
// Constants are folded by the type checker, so a document assembled from a
// shared fragment constant is indexed with the fragment already spliced in,
// which is how the vulnerability state mutations are written.
func (p *program) indexDocuments(pkg *packages.Package) {
	if pkg.TypesInfo == nil || pkg.Types == nil {
		return
	}
	for ident, obj := range pkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		value, ok := definedStringValue(pkg, obj, ident)
		if !ok {
			continue
		}
		if kind := classifyDocument(value); kind != notADocument {
			p.documents[obj] = kind
		}
	}
}

// definedStringValue returns the constant string an object was defined with,
// for the two shapes a GraphQL document is written in: a string constant, and
// a package-level variable initialized from a constant string expression.
func definedStringValue(pkg *packages.Package, obj types.Object, ident *ast.Ident) (string, bool) {
	if con, ok := obj.(*types.Const); ok {
		return constantString(con.Val())
	}
	variable, ok := obj.(*types.Var)
	if !ok || variable.Parent() != pkg.Types.Scope() {
		return "", false
	}
	init := variableInitializer(pkg, ident)
	if init == nil {
		return "", false
	}
	tv, ok := pkg.TypesInfo.Types[init]
	if !ok || tv.Value == nil {
		return "", false
	}
	return constantString(tv.Value)
}

// variableInitializer finds the expression a package-level variable is
// declared with. A variable declared without one, or declared in a grouped
// spec that assigns fewer values than names, has none.
func variableInitializer(pkg *packages.Package, ident *ast.Ident) ast.Expr {
	var found ast.Expr
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok || len(spec.Names) != len(spec.Values) {
				return true
			}
			for i, name := range spec.Names {
				if name == ident {
					found = spec.Values[i]
					return false
				}
			}
			return true
		})
		if found != nil {
			return found
		}
	}
	return nil
}

// constantString unwraps a string constant value.
func constantString(value constant.Value) (string, bool) {
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

// indexFunctions records every declared function's body.
func (p *program) indexFunctions(pkg *packages.Package) {
	if pkg.TypesInfo == nil {
		return
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}
			obj, ok := pkg.TypesInfo.Defs[funcDecl.Name].(*types.Func)
			if !ok {
				continue
			}
			p.funcs[obj] = p.indexBody(pkg, obj, funcDecl)
		}
	}
}

// indexBody walks one function body. Function literals inside it are walked as
// part of it: a closure a handler defines runs when that handler runs, so
// folding it into the enclosing function keeps the reachable set honest
// without a separate node per literal.
func (p *program) indexBody(pkg *packages.Package, obj *types.Func, decl *ast.FuncDecl) *function {
	fn := &function{obj: obj, pkg: pkg, decl: decl, calls: make(map[*types.Func]bool)}
	seen := make(map[token.Pos]bool)
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			p.recordUse(fn, pkg, typed, seen)
		case *ast.BasicLit:
			if typed.Kind == token.STRING {
				p.recordLiteral(fn, pkg, typed, seen)
			}
		}
		return true
	})
	return fn
}

// recordUse records what one identifier in a body names.
func (p *program) recordUse(fn *function, pkg *packages.Package, ident *ast.Ident, seen map[token.Pos]bool) {
	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return
	}
	if callee, ok := obj.(*types.Func); ok {
		fn.calls[callee] = true
		if graphQLSenders[callee.Name()] && isGraphQLSender(callee) {
			fn.sendsGraphQL = true
		}
		return
	}
	kind, ok := p.documents[obj]
	if !ok || seen[ident.Pos()] {
		return
	}
	seen[ident.Pos()] = true
	fn.docs = append(fn.docs, docRef{kind: kind, name: obj.Name(), pos: ident.Pos(), declared: obj.Pos()})
}

// recordLiteral records a GraphQL document written inline rather than as a
// named constant.
func (p *program) recordLiteral(fn *function, pkg *packages.Package, lit *ast.BasicLit, seen map[token.Pos]bool) {
	tv, ok := pkg.TypesInfo.Types[lit]
	if !ok || tv.Value == nil || seen[lit.Pos()] {
		return
	}
	value, ok := constantString(tv.Value)
	if !ok {
		return
	}
	kind := classifyDocument(value)
	if kind == notADocument {
		return
	}
	seen[lit.Pos()] = true
	fn.docs = append(fn.docs, docRef{kind: kind, pos: lit.Pos(), declared: lit.Pos()})
}

// isGraphQLSender reports whether a method named Do (or one of the shared
// executors) belongs to the GraphQL transport rather than to some unrelated
// type that happens to have a method with the same name.
func isGraphQLSender(callee *types.Func) bool {
	if callee.Pkg() != nil && callee.Pkg().Path() == toolutilPath {
		return true
	}
	sig, ok := callee.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return strings.Contains(sig.Recv().Type().String(), "GraphQL")
}

// reachable returns the transitive closure of functions a handler can run,
// including the handlers themselves.
func (p *program) reachable(roots []*types.Func) map[*types.Func]bool {
	seen := make(map[*types.Func]bool, len(roots))
	queue := append([]*types.Func(nil), roots...)
	for len(queue) > 0 {
		current := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[current] {
			continue
		}
		seen[current] = true
		fn, ok := p.funcs[current]
		if !ok {
			continue
		}
		for callee := range fn.calls {
			if !seen[callee] {
				queue = append(queue, callee)
			}
		}
	}
	return seen
}

// position renders a source position.
func (p *program) position(pos token.Pos) token.Position {
	return p.fset.Position(pos)
}
