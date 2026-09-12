package main

import (
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// deadExports lists the exported symbols of the harness that no package
// outside it uses: package-level functions, types, constants and variables,
// the exported methods of exported types, and the exported fields of exported
// structs.
//
// The harness's own tests do not count as a consumer: a symbol only the
// harness tests use is one the harness keeps for itself, and the question is
// whether the suite it exists for calls it.
func deadExports(harness *packages.Package, selected []*packages.Package, harnessPath string) []string {
	exported := exportedSymbols(harness.Types)
	used := map[string]bool{}
	for _, pkg := range selected {
		if strings.TrimSuffix(pkg.PkgPath, "_test") == harnessPath {
			continue
		}
		markUses(pkg, harnessPath, used)
	}
	closeOverTypes(harness.Types, used)
	var dead []string
	for _, name := range exported {
		if !used[name] {
			dead = append(dead, name)
		}
	}
	sort.Strings(dead)
	return dead
}

// closeOverTypes marks the types a used symbol implies: a type a used
// function takes or returns, the type of a used constant or variable, the
// type a used method or field belongs to, and the types those in turn name in
// their signatures.
//
// A test that calls New(t) and then Session() on what it got never spells
// Env, and a list that reported Env dead would be reporting the type the
// whole harness hands out. Being named is one way to be used and not the
// only one.
func closeOverTypes(pkg *types.Package, used map[string]bool) {
	scope := pkg.Scope()
	for {
		changed := false
		for _, name := range scope.Names() {
			if markImplied(scope.Lookup(name), used) {
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

// markImplied marks what one symbol implies, and reports whether anything
// new was marked.
func markImplied(obj types.Object, used map[string]bool) bool {
	marked := false
	mark := func(key string) {
		if !used[key] {
			used[key] = true
			marked = true
		}
	}
	name := obj.Name()
	switch o := obj.(type) {
	case *types.TypeName:
		named, isNamed := o.Type().(*types.Named)
		if !isNamed {
			return false
		}
		for method := range named.Methods() {
			if used[name+"."+method.Name()] {
				mark(name)
				markSignatureTypes(method.Signature(), obj.Pkg(), mark)
			}
		}
		if structType, isStruct := named.Underlying().(*types.Struct); isStruct {
			for field := range structType.Fields() {
				if used[name+"."+field.Name()] {
					mark(name)
				}
			}
		}
	case *types.Func:
		if used[name] {
			markSignatureTypes(o.Signature(), obj.Pkg(), mark)
		}
	case *types.Const, *types.Var:
		if used[name] {
			markNamedTypes(obj.Type(), obj.Pkg(), mark, 0)
		}
	}
	return marked
}

// markSignatureTypes marks the package's named types a signature names.
func markSignatureTypes(sig *types.Signature, pkg *types.Package, mark func(string)) {
	for param := range sig.Params().Variables() {
		markNamedTypes(param.Type(), pkg, mark, 0)
	}
	for result := range sig.Results().Variables() {
		markNamedTypes(result.Type(), pkg, mark, 0)
	}
}

// typeDepth bounds the walk into a type's structure.
const typeDepth = 4

// markNamedTypes marks the package's named types a type is built from.
func markNamedTypes(t types.Type, pkg *types.Package, mark func(string), depth int) {
	if depth > typeDepth {
		return
	}
	switch u := t.(type) {
	case *types.Named:
		if u.Obj().Pkg() == pkg {
			mark(u.Obj().Name())
		}
	case *types.Pointer:
		markNamedTypes(u.Elem(), pkg, mark, depth+1)
	case *types.Slice:
		markNamedTypes(u.Elem(), pkg, mark, depth+1)
	case *types.Map:
		markNamedTypes(u.Key(), pkg, mark, depth+1)
		markNamedTypes(u.Elem(), pkg, mark, depth+1)
	case *types.Signature:
		for param := range u.Params().Variables() {
			markNamedTypes(param.Type(), pkg, mark, depth+1)
		}
		for result := range u.Results().Variables() {
			markNamedTypes(result.Type(), pkg, mark, depth+1)
		}
	}
}

// exportedSymbols names every exported symbol of a package, as
// [symbolKey] spells them.
func exportedSymbols(pkg *types.Package) []string {
	var names []string
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		names = append(names, name)
		typeName, isType := obj.(*types.TypeName)
		if !isType {
			continue
		}
		names = append(names, exportedMembers(typeName)...)
	}
	sort.Strings(names)
	return names
}

// exportedMembers names the exported methods and fields of a named type.
func exportedMembers(typeName *types.TypeName) []string {
	named, isNamed := typeName.Type().(*types.Named)
	if !isNamed {
		return nil
	}
	var names []string
	for method := range named.Methods() {
		if method.Exported() {
			names = append(names, typeName.Name()+"."+method.Name())
		}
	}
	if structType, isStruct := named.Underlying().(*types.Struct); isStruct {
		for field := range structType.Fields() {
			if field.Exported() {
				names = append(names, typeName.Name()+"."+field.Name())
			}
		}
	}
	return names
}

// markUses records every harness symbol a package uses.
func markUses(pkg *packages.Package, harnessPath string, used map[string]bool) {
	for _, obj := range pkg.TypesInfo.Uses {
		if obj.Pkg() == nil || obj.Pkg().Path() != harnessPath {
			continue
		}
		used[symbolKey(obj)] = true
	}
	for _, selection := range pkg.TypesInfo.Selections {
		obj := selection.Obj()
		if obj.Pkg() == nil || obj.Pkg().Path() != harnessPath {
			continue
		}
		if named := receiverNamed(selection.Recv()); named != nil {
			used[named.Obj().Name()+"."+obj.Name()] = true
		}
	}
}

// symbolKey spells an object the way [exportedSymbols] does: a method as
// Type.Method, anything else by its name.
func symbolKey(obj types.Object) string {
	if fn, isFunc := obj.(*types.Func); isFunc {
		if recv := fn.Signature().Recv(); recv != nil {
			if named := receiverNamed(recv.Type()); named != nil {
				return named.Obj().Name() + "." + fn.Name()
			}
		}
	}
	return obj.Name()
}

// receiverNamed returns the named type behind a receiver or selection
// receiver, through one pointer.
func receiverNamed(t types.Type) *types.Named {
	if pointer, isPointer := t.(*types.Pointer); isPointer {
		t = pointer.Elem()
	}
	named, isNamed := t.(*types.Named)
	if !isNamed {
		return nil
	}
	return named
}
