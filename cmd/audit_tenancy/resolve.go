package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// program is the loaded source, indexed the ways the rules ask for it: by the
// module-relative directory a site names, and from a declared object back to
// the declaration that holds its body, its initializer and its comments.
type program struct {
	root string
	fset *token.FileSet
	// packages are the loaded packages the rules read, in import-path order,
	// with the test-support packages the server never links left out.
	packages []*packages.Package
	byDir    map[string]*packages.Package
	decls    map[string]*declaration
}

// declaration is one package-level declaration of the loaded source: a
// function or method, one name of a const or var spec, or a type.
type declaration struct {
	pkg *packages.Package
	// key is how the register names it, "cmd/server:listenLimits.busy".
	key string
	obj types.Object
	fn  *ast.FuncDecl
	gen *ast.GenDecl
	// value is the spec for a const or var, and index the position of the
	// name in it.
	value *ast.ValueSpec
	index int
	// typ is the spec for a type.
	typ *ast.TypeSpec
}

// newProgram indexes the loaded packages, leaving out those the rules must
// not read.
func newProgram(root string, loaded []*packages.Package, leave []string) *program {
	p := &program{root: root, byDir: map[string]*packages.Package{}, decls: map[string]*declaration{}}
	for _, pkg := range loaded {
		dir := moduleDir(pkg.PkgPath)
		if leftOut(dir, leave) {
			continue
		}
		p.fset = pkg.Fset
		p.packages = append(p.packages, pkg)
		p.byDir[dir] = pkg
		p.index(pkg, dir)
	}
	slices.SortFunc(p.packages, func(a, b *packages.Package) int { return strings.Compare(a.PkgPath, b.PkgPath) })
	return p
}

// leftOut reports whether a module-relative directory is one of leave, or
// below one that ends in "/...".
func leftOut(dir string, leave []string) bool {
	for _, l := range leave {
		if base, tree := strings.CutSuffix(l, "/..."); tree {
			if dir == base || strings.HasPrefix(dir, base+"/") {
				return true
			}
			continue
		}
		if dir == l {
			return true
		}
	}
	return false
}

// moduleDir is an import path relative to this module, the way the register
// names a package.
func moduleDir(path string) string {
	return strings.TrimPrefix(path, goprogram.ModulePath+"/")
}

// index records every package-level declaration of pkg under its key.
func (p *program) index(pkg *packages.Package, dir string) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			// A file's declarations are functions and general declarations;
			// the parser makes a bad one only of source that does not parse,
			// which the loader has already refused.
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
				p.add(&declaration{pkg: pkg, key: siteKey(dir, funcName(fn)), obj: pkg.TypesInfo.Defs[fn.Name], fn: fn})
				continue
			}
			gen, _ := decl.(*ast.GenDecl)
			p.indexGen(pkg, dir, gen)
		}
	}
}

// indexGen records each name of a const, var or type declaration.
func (p *program) indexGen(pkg *packages.Package, dir string, gen *ast.GenDecl) {
	for _, spec := range gen.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			for i, name := range s.Names {
				if name.Name == "_" {
					continue
				}
				p.add(&declaration{pkg: pkg, key: siteKey(dir, name.Name), obj: pkg.TypesInfo.Defs[name], gen: gen, value: s, index: i})
			}
		case *ast.TypeSpec:
			p.add(&declaration{pkg: pkg, key: siteKey(dir, s.Name.Name), obj: pkg.TypesInfo.Defs[s.Name], gen: gen, typ: s})
		}
	}
}

// add keeps the first declaration of a key. Go allows only one package-level
// declaration of a name, except init functions, which no site can name.
func (p *program) add(d *declaration) {
	if _, seen := p.decls[d.key]; !seen {
		p.decls[d.key] = d
	}
}

// siteKey is the key the register and the exemption table use, "dir:Name".
func siteKey(dir, name string) string {
	return dir + ":" + name
}

// keyOf is the key of a register site.
func keyOf(s tenancy.Site) string {
	return siteKey(s.Pkg, s.Name)
}

// funcName is how a function is named in a site: "Func", or "Type.Method"
// with the receiver's base type, its pointer and type parameters stripped.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil {
		return fn.Name.Name
	}
	return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// receiverName is the base type name of a receiver expression.
func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return receiverName(e.X)
	case *ast.IndexExpr:
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	case *ast.ParenExpr:
		return receiverName(e.X)
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
}

// objectName is how a site names a function object, "Func" or "Type.Method".
func objectName(fn *types.Func) string {
	recvVar := fn.Signature().Recv()
	if recvVar == nil {
		return fn.Name()
	}
	recv := types.Unalias(recvVar.Type())
	if ptr, isPtr := recv.(*types.Pointer); isPtr {
		recv = types.Unalias(ptr.Elem())
	}
	if named, isNamed := recv.(*types.Named); isNamed {
		return named.Obj().Name() + "." + fn.Name()
	}
	return fn.Name()
}

// objectKey is the register key of a package-level object or method of a
// package this module holds, and empty for anything else.
func objectKey(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), goprogram.ModulePath+"/") {
		return ""
	}
	name := obj.Name()
	if fn, isFunc := obj.(*types.Func); isFunc {
		name = objectName(fn)
	} else if obj.Parent() != obj.Pkg().Scope() {
		return ""
	}
	return siteKey(moduleDir(obj.Pkg().Path()), name)
}

// lookup returns the declaration a site names, or why it names none.
func (p *program) lookup(s tenancy.Site) (*declaration, error) {
	pkg := p.byDir[s.Pkg]
	if pkg == nil {
		return nil, fmt.Errorf("package %s is not in the loaded program", s.Pkg)
	}
	d := p.decls[keyOf(s)]
	if d == nil {
		return nil, fmt.Errorf("%s declares nothing named %s", s.Pkg, s.Name)
	}
	return d, nil
}

// position renders a node's position relative to the repository root.
func (p *program) position(pos token.Pos) string {
	at := p.fset.Position(pos)
	rel, err := filepath.Rel(p.root, at.Filename)
	if err != nil {
		rel = at.Filename
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), at.Line)
}

// where is the position of a declaration's name.
func (d *declaration) where(p *program) string {
	switch {
	case d.fn != nil:
		return p.position(d.fn.Name.Pos())
	case d.value != nil:
		return p.position(d.value.Names[d.index].Pos())
	default:
		return p.position(d.typ.Name.Pos())
	}
}

// initializer is the expression a const or var name is initialized with, or
// nil when the spec gives it none of its own.
func (d *declaration) initializer() ast.Expr {
	if d.value == nil || len(d.value.Values) != len(d.value.Names) {
		return nil
	}
	return d.value.Values[d.index]
}

// body is what a rule reads inside a declaration: a function's body, a
// const or var's initializer, or nothing for a type.
//
// Every function the load holds has a body: go list refuses a declaration
// without one unless its package carries assembly, and nothing the gate loads
// does.
func (d *declaration) body() ast.Node {
	if d.fn != nil {
		return d.fn.Body
	}
	if init := d.initializer(); init != nil {
		return init
	}
	return nil
}

// comments are the doc comments that describe the declaration: its own, and
// the one on the const, var or type block that encloses it.
func (d *declaration) comments() []*ast.CommentGroup {
	switch {
	case d.fn != nil:
		return []*ast.CommentGroup{d.fn.Doc}
	case d.value != nil:
		return []*ast.CommentGroup{d.value.Doc, d.gen.Doc}
	default:
		return []*ast.CommentGroup{d.typ.Doc, d.gen.Doc}
	}
}

// info is the type information of the declaration's package.
func (d *declaration) info() *types.Info {
	return d.pkg.TypesInfo
}

// function is the declared function or method, for a declaration that is
// one, and nil for anything else.
func (d *declaration) function() *types.Func {
	fn, _ := d.obj.(*types.Func)
	return fn
}

// everySite lists every site the register names, each with what named it, in
// the register's own order: the rows' sites, refusal texts and literals and
// reasons, the keys' derivations, and the failure table's functions.
func everySite(reg register) []namedSite {
	var all []namedSite
	for _, d := range reg.decisions {
		for _, s := range d.Sites {
			all = append(all, namedSite{by: d.ID, site: s})
		}
		for _, r := range d.Refusals {
			all = append(all, namedSite{by: d.ID + " refusal", site: r.At}, namedSite{by: d.ID + " refusal", site: r.Via})
		}
		all = append(all, namedSite{by: d.ID + " reason", site: d.ReasonAt})
	}
	for _, k := range reg.derivations {
		for _, s := range k.sites {
			all = append(all, namedSite{by: "key " + k.key, site: s})
		}
	}
	for _, f := range reg.failures {
		all = append(all, namedSite{by: "failure " + f.Kind, site: f.At})
	}
	return slices.DeleteFunc(all, func(n namedSite) bool { return n.site == (tenancy.Site{}) })
}

// namedSite is a site with what named it.
type namedSite struct {
	by   string
	site tenancy.Site
}

// declaredKeys is the set of every declaration the register names in any
// role. A limit-shaped declaration inside one of them is declared.
func declaredKeys(reg register) map[string]bool {
	keys := map[string]bool{}
	for _, n := range everySite(reg) {
		keys[keyOf(n.site)] = true
	}
	return keys
}

// checkResolve is G1: every site of every row, derivation and failure, and
// every exemption, resolves to exactly one declaration, and no exemption is
// also a site. A declaration that matches nothing is a finding, on the terms
// every declaration table here is held to.
func checkResolve(p *program, reg register, exempt map[string]exemption) []Finding {
	var found []Finding
	seen := map[string]bool{}
	for _, n := range everySite(reg) {
		key := n.by + "\x00" + keyOf(n.site)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := p.lookup(n.site); err != nil {
			found = append(found, Finding{Rule: "G1", Subject: n.by, Message: fmt.Sprintf("names %s, which matches nothing: %v", keyOf(n.site), err)})
		}
	}
	declared := declaredKeys(reg)
	for _, key := range sortedKeys(exempt) {
		dir, name, _ := strings.Cut(key, ":")
		if _, err := p.lookup(tenancy.Site{Pkg: dir, Name: name}); err != nil {
			found = append(found, Finding{Rule: "G1", Subject: key, Message: fmt.Sprintf("exempted, and matches nothing: %v", err)})
		}
		if declared[key] {
			found = append(found, Finding{Rule: "G1", Subject: key, Message: "exempted and also named by the register as a site; a declaration is one or the other"})
		}
	}
	return found
}

// sortedKeys returns a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// forEachTopLevel calls visit with every piece of code the program declares at
// package level, under the key of the declaration it belongs to: a function's
// body, and each const or var name's initializer. A spec that initializes
// several names from one call is the first name's.
func (p *program) forEachTopLevel(visit func(key string, info *types.Info, node ast.Node)) {
	for _, pkg := range p.packages {
		dir := moduleDir(pkg.PkgPath)
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				visitDecl(dir, pkg.TypesInfo, decl, visit)
			}
		}
	}
}

// visitDecl hands visit the code of one top-level declaration.
func visitDecl(dir string, info *types.Info, decl ast.Decl, visit func(key string, info *types.Info, node ast.Node)) {
	if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
		visit(siteKey(dir, funcName(fn)), info, fn)
		return
	}
	gen, _ := decl.(*ast.GenDecl)
	for _, spec := range gen.Specs {
		vs, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		// A spec has one value per name, or one value for them all, so the
		// i'th value always has an i'th name: the first, for the single call
		// that initializes several.
		for i, value := range vs.Values {
			visit(siteKey(dir, vs.Names[i].Name), info, value)
		}
	}
}

// forEachCall calls visit with every call the program makes at package level,
// under the key of the declaration it sits in.
func (p *program) forEachCall(visit func(key string, info *types.Info, call *ast.CallExpr)) {
	p.forEachTopLevel(func(key string, info *types.Info, node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall {
				visit(key, info, call)
			}
			return true
		})
	})
}

// calleeOf is the function or method a call statically names, or nil for a
// call through a value, a conversion or a builtin.
func calleeOf(info *types.Info, call *ast.CallExpr) *types.Func {
	var obj types.Object
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		obj = info.Uses[fun]
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[fun]; ok {
			obj = sel.Obj()
		} else {
			obj = info.Uses[fun.Sel]
		}
	case *ast.IndexExpr:
		return calleeOf(info, &ast.CallExpr{Fun: fun.X})
	case *ast.IndexListExpr:
		return calleeOf(info, &ast.CallExpr{Fun: fun.X})
	}
	fn, _ := obj.(*types.Func)
	if fn != nil {
		fn = fn.Origin()
	}
	return fn
}

// calleeName is the full name of a function, its import path and its
// site-style name, "golang.org/x/time/rate.NewLimiter".
func calleeName(fn *types.Func) string {
	if fn == nil || fn.Pkg() == nil {
		return ""
	}
	return fn.Pkg().Path() + "." + objectName(fn)
}

// referenced is the object an identifier or a selector names, or nil.
func referenced(info *types.Info, expr ast.Expr) types.Object {
	switch e := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return info.Uses[e]
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[e]; ok {
			return sel.Obj()
		}
		return info.Uses[e.Sel]
	default:
		return nil
	}
}
