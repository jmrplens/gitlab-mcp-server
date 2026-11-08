// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// declaration is one top-level declaration of a source file as the record
// knows it: the key it is named by, where its text starts and ends, and the
// SHA-256 of that text, the doc comment left out, so a comment reworded or a
// line added above it leaves it the declaration the runs measured.
type declaration struct {
	key        string
	start, end position
	sum        string
}

// declarationsOf parses one source file and returns its top-level
// declarations in source order. A function is keyed by its name, a method by
// its receiver type and name, a type, var, const or import group by every
// name or path it declares, and a key the file repeats (init declared twice,
// two var _) is numbered from its second occurrence on.
func declarationsOf(name string, src []byte) ([]declaration, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	decls := make([]declaration, 0, len(file.Decls))
	repeats := map[string]int{}
	for _, decl := range file.Decls {
		var key string
		switch d := decl.(type) {
		case *ast.FuncDecl:
			key = funcKey(fset, src, d)
		case *ast.GenDecl:
			key = genKey(d)
		}
		repeats[key]++
		if n := repeats[key]; n > 1 {
			key = fmt.Sprintf("%s #%d", key, n)
		}
		from, to := fset.Position(decl.Pos()), fset.Position(decl.End())
		digest := sha256.Sum256(src[from.Offset:to.Offset])
		decls = append(decls, declaration{
			key:   key,
			start: position{line: from.Line, col: from.Column},
			end:   position{line: to.Line, col: to.Column},
			sum:   hex.EncodeToString(digest[:]),
		})
	}
	return decls, nil
}

// funcKey names a function by its name and a method by its receiver's type
// as the source spells it, then its name.
func funcKey(fset *token.FileSet, src []byte, d *ast.FuncDecl) string {
	if d.Recv == nil {
		return "func " + d.Name.Name
	}
	recv := d.Recv.List[0].Type
	from, to := fset.Position(recv.Pos()).Offset, fset.Position(recv.End()).Offset
	return fmt.Sprintf("func (%s) %s", src[from:to], d.Name.Name)
}

// genKey names a type, var, const or import declaration by its keyword and
// every name, or path, it declares.
func genKey(d *ast.GenDecl) string {
	var names []string
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.ImportSpec:
			names = append(names, s.Path.Value)
		case *ast.TypeSpec:
			names = append(names, s.Name.Name)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				names = append(names, n.Name)
			}
		}
	}
	return d.Tok.String() + " " + strings.Join(names, ", ")
}

// holds reports whether the block at s lies inside the declaration's text,
// from its first position to its last, which is where every block go test
// measures in it lies.
func (d declaration) holds(s span) bool {
	return !s.start.before(d.start) && !d.end.before(s.end)
}

// holding returns the declaration whose text holds a block that starts at
// p, which is every block go test measures: it instruments function bodies,
// the literals of a var among them, and nothing between declarations.
func holding(decls []declaration, p position) (declaration, bool) {
	for _, d := range decls {
		if !p.before(d.start) && p.before(d.end) {
			return d, true
		}
	}
	return declaration{}, false
}
