// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Which client-go functions this server can run: the ones its own source names
// and everything they call inside client-go.

package main

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"
)

// reachedFunctions returns, for every function of the client-go program that a
// call in ours can run, the import paths of our packages that reach it, sorted.
//
// The roots are every client-go function or method our source names, which is
// what a handler calling a service does and also what one handing a method
// value on does. A method named through a service interface, the way a handler
// holds client-go's Client, is every implementation client-go declares of
// it. From the roots the walk follows every function a client-go body names,
// through interfaces in the same way, with no depth bound: a service method
// whose GraphQL send sits in a helper (the work item ID lookup a work item
// update makes first) is reached through the helper.
//
// The two programs are separate loads, in which one function is two objects:
// ours sees client-go through export data and the other load type-checked it
// from source. Functions are therefore compared by full name, which carries
// the package path and the receiver and is the same in both, and an interface
// named in ours is looked up by name in the load that declares its
// implementations.
func reachedFunctions(ours, sdk []*packages.Package) map[string][]string {
	index := newSDKIndex(sdk)
	roots := map[string]map[string]bool{}
	for _, pkg := range ours {
		for _, object := range pkg.TypesInfo.Uses {
			for _, name := range index.resolve(object) {
				if roots[name] == nil {
					roots[name] = map[string]bool{}
				}
				roots[name][pkg.PkgPath] = true
			}
		}
	}
	reached := map[string]map[string]bool{}
	for root, callers := range roots {
		index.walk(root, func(name string) {
			if reached[name] == nil {
				reached[name] = map[string]bool{}
			}
			for caller := range callers {
				reached[name][caller] = true
			}
		})
	}
	sorted := make(map[string][]string, len(reached))
	for name, callers := range reached {
		for caller := range callers {
			sorted[name] = append(sorted[name], caller)
		}
		slices.Sort(sorted[name])
	}
	return sorted
}

// sdkIndex is the client-go program arranged for the walk: its packages by
// path, the concrete types that may implement an interface, and every function
// with the functions its body names.
type sdkIndex struct {
	packages    map[string]*packages.Package
	concrete    []*types.Named
	calls       map[string][]string
	implemented map[string][]string
}

// newSDKIndex indexes a loaded client-go program.
//
// A generic type is not a candidate implementation: go/types leaves what
// Implements answers for an uninstantiated type unspecified, and client-go's
// generic types (its connection and nullable wrappers) implement none of its
// service interfaces. An alias names a type declared elsewhere, which is a
// candidate under its own name.
func newSDKIndex(sdk []*packages.Package) *sdkIndex {
	index := &sdkIndex{packages: map[string]*packages.Package{}, calls: map[string][]string{}, implemented: map[string][]string{}}
	for _, pkg := range sdk {
		index.packages[pkg.PkgPath] = pkg
	}
	for _, pkg := range sdk {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			declared, isType := scope.Lookup(name).(*types.TypeName)
			if !isType || declared.IsAlias() || types.IsInterface(declared.Type()) {
				continue
			}
			if named, _ := declared.Type().(*types.Named); named.TypeParams().Len() == 0 {
				index.concrete = append(index.concrete, named)
			}
		}
	}
	for _, pkg := range sdk {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
					index.recordCalls(pkg, fn)
				}
			}
		}
	}
	return index
}

// recordCalls notes every client-go function a function's body names.
func (x *sdkIndex) recordCalls(pkg *packages.Package, fn *ast.FuncDecl) {
	from := declaredFunc(pkg, fn).FullName()
	ast.Inspect(fn, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			x.calls[from] = append(x.calls[from], x.resolve(pkg.TypesInfo.Uses[ident])...)
		}
		return true
	})
}

// resolve names the client-go functions an object stands for: itself when it
// is a function or a method of client-go, every implementation client-go
// declares when it is an interface method, and nothing when it is neither.
func (x *sdkIndex) resolve(object types.Object) []string {
	fn, isFunc := object.(*types.Func)
	if !isFunc || fn.Pkg() == nil || x.packages[fn.Pkg().Path()] == nil {
		return nil
	}
	recv := fn.Signature().Recv()
	if recv == nil || !types.IsInterface(recv.Type()) {
		return []string{fn.Origin().FullName()}
	}
	return x.implementations(recv.Type(), fn.Name())
}

// implementations names the method every concrete client-go type declares for
// an interface method.
//
// A named interface is read in the client-go load, whichever load named it,
// since an interface from export data and a type checked from source are
// never identical even when they are the same declaration. An interface
// written in place has no name to look up, and is only ever met inside
// client-go's own bodies, where it is already the source-checked type. The
// answer for one interface method is the same however often a body names it,
// so it is worked out once.
func (x *sdkIndex) implementations(iface types.Type, method string) []string {
	key := types.TypeString(iface, nil) + "." + method
	if found, done := x.implemented[key]; done {
		return found
	}
	target, _ := iface.Underlying().(*types.Interface)
	if named, isNamed := iface.(*types.Named); isNamed {
		declared, _ := x.packages[named.Obj().Pkg().Path()].Types.Scope().Lookup(named.Obj().Name()).(*types.TypeName)
		target, _ = declared.Type().Underlying().(*types.Interface)
	}
	found := []string{}
	for _, concrete := range x.concrete {
		pointer := types.NewPointer(concrete)
		if !types.Implements(pointer, target) {
			continue
		}
		object, _, _ := types.LookupFieldOrMethod(pointer, true, concrete.Obj().Pkg(), method)
		implementation, _ := object.(*types.Func)
		found = append(found, implementation.FullName())
	}
	x.implemented[key] = found
	return found
}

// walk visits every function reachable from a root, the root included, once
// each.
func (x *sdkIndex) walk(root string, visit func(string)) {
	seen := map[string]bool{root: true}
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		visit(current)
		for _, next := range x.calls[current] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
}
