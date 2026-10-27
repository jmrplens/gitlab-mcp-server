// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// englishCatalogVar is the package-level variable the English catalog is
// written in, the source every translation is held to.
const englishCatalogVar = "english"

// catalog is what the audit reads out of the catalog package's source: the
// English template of every key, where each entry is written, and every key
// constant the package declares.
//
// It is read from the loaded source rather than from the catalog compiled
// into this binary, so a planted tree with a stand-in catalog of its own is
// judged by that stand-in, and so the gate judges the tree it was handed
// rather than the one it was built from.
type catalog struct {
	// path is the catalog package's import path, and name the name its
	// package clause gives it, which a finding spells its functions with.
	path string
	name string
	// templates maps each key of the English catalog, as the string its
	// constant folds to, to its template.
	templates map[string]string
	// entries maps each key of the English catalog to where its entry is
	// written, which is where a dead one is reported.
	entries map[string]token.Pos
	// constants maps the value of every constant of the Key type the package
	// declares to that constant.
	constants map[string]*types.Const
	pkg       *packages.Package
}

// readCatalog reads the catalog package out of a load that holds it.
//
// The load always holds it, because the audit adds the catalog's import path
// to the patterns and a pattern that resolves to no package fails the load.
// What can still go wrong is the shape of the source: an English catalog that
// is not there, is not written as a map literal, or holds an entry the type
// checker could not fold to a constant key and a constant template. Each is
// refused, since the rules would otherwise be asked of a catalog with nothing
// in it, which reads as a clean run.
func readCatalog(root string, loaded []*packages.Package, path string) (*catalog, error) {
	cat := &catalog{
		path:      path,
		templates: map[string]string{},
		entries:   map[string]token.Pos{},
		constants: map[string]*types.Const{},
	}
	for _, pkg := range loaded {
		if pkg.PkgPath == path {
			cat.pkg = pkg
		}
	}
	cat.name = cat.pkg.Name
	value, found := englishValue(cat.pkg)
	if !found {
		return nil, fmt.Errorf("%s declares no English catalog: no package-level variable named %s", path, englishCatalogVar)
	}
	literal, isLiteral := ast.Unparen(value).(*ast.CompositeLit)
	if !isLiteral {
		return nil, fmt.Errorf("the English catalog of %s is not written as a map literal", path)
	}
	if err := cat.readEntries(root, literal); err != nil {
		return nil, err
	}
	cat.readConstants()
	return cat, nil
}

// englishValue finds the expression the English catalog variable is
// initialized with, among the package-level declarations of the catalog
// package. A variable declared without a value is found with none.
func englishValue(pkg *packages.Package) (ast.Expr, bool) {
	for _, file := range pkg.Syntax {
		for _, spec := range valueSpecs(file) {
			for i, name := range spec.Names {
				if name.Name != englishCatalogVar {
					continue
				}
				if i < len(spec.Values) {
					return spec.Values[i], true
				}
				return nil, true
			}
		}
	}
	return nil, false
}

// valueSpecs returns every package-level const and var spec of a file, in
// source order.
func valueSpecs(file *ast.File) []*ast.ValueSpec {
	var specs []*ast.ValueSpec
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen {
			continue
		}
		for _, spec := range gen.Specs {
			if value, isValue := spec.(*ast.ValueSpec); isValue {
				specs = append(specs, value)
			}
		}
	}
	return specs
}

// readEntries reads every key and template of the English catalog literal,
// and refuses the first entry the type checker could not fold.
//
// The pairs are found by walking the literal, which reaches its type and its
// pairs and never anything inside a pair: a map literal holds nothing but
// pairs, and stepping over the type is all the walk has to do to reach them.
func (c *catalog) readEntries(root string, literal *ast.CompositeLit) error {
	info := c.pkg.TypesInfo
	var unfolded []*ast.KeyValueExpr
	ast.Inspect(literal, func(node ast.Node) bool {
		pair, isPair := node.(*ast.KeyValueExpr)
		if !isPair {
			return true
		}
		key, template := info.Types[pair.Key].Value, info.Types[pair.Value].Value
		if key == nil || template == nil {
			unfolded = append(unfolded, pair)
			return false
		}
		c.templates[constant.StringVal(key)] = constant.StringVal(template)
		c.entries[constant.StringVal(key)] = pair.Key.Pos()
		return false
	})
	if len(unfolded) == 0 {
		return nil
	}
	at := c.pkg.Fset.Position(unfolded[0].Pos())
	return fmt.Errorf("the English catalog of %s holds an entry at %s:%d whose key or template is not a constant",
		c.path, goprogram.RelativePath(at.Filename, root), at.Line)
}

// readConstants records every constant of the Key type the catalog package
// declares.
func (c *catalog) readConstants() {
	scope := c.pkg.Types.Scope()
	for _, name := range scope.Names() {
		declared, isConst := scope.Lookup(name).(*types.Const)
		if !isConst || !c.isNamed(declared.Type(), "Key") {
			continue
		}
		c.constants[constant.StringVal(declared.Val())] = declared
	}
}

// owns reports whether an object is declared in the catalog package. An
// object of the universe, such as the Error method of error, belongs to no
// package at all.
func (c *catalog) owns(obj types.Object) bool {
	return obj.Pkg() != nil && obj.Pkg().Path() == c.path
}

// isNamed reports whether t is the named type the catalog package declares
// under name, through an alias.
func (c *catalog) isNamed(t types.Type, name string) bool {
	named, isNamed := types.Unalias(t).(*types.Named)
	return isNamed && c.owns(named.Obj()) && named.Obj().Name() == name
}

// isKey reports whether a constant is a key the catalog package declares.
func (c *catalog) isKey(declared *types.Const) bool {
	return c.owns(declared) && c.isNamed(declared.Type(), "Key")
}
