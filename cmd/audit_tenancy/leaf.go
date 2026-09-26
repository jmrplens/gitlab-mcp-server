package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strconv"
)

// checkLeaf is G12: the register stays a leaf the server can import for free.
// Its non-test files import only the packages the rules allow, each of which
// the server already imports, and they declare no package-level variable.
//
// Those are the conditions the code-identity proof rests on. A change that
// replaces a literal with one of the register's constants claims that its
// binary is the one its parent builds once the parent imports the register
// where the change does; that holds only while importing the register adds no
// package the server did not link and no initialization work, and nothing
// reachable calls a register function the linker would otherwise drop.
func (g *gate) checkLeaf() []Finding {
	leaf := g.p.byDir[g.reg.leaf]
	if leaf == nil {
		return []Finding{{Rule: "G12", Subject: g.reg.leaf, Message: "the register package is not in the loaded program"}}
	}
	server := g.p.byDir[g.rules.server]
	var found []Finding
	for _, file := range leaf.Syntax {
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			switch {
			case !slices.Contains(g.rules.leafImports, path):
				found = append(found, Finding{
					Rule: "G12", Subject: g.reg.leaf, Position: g.p.position(spec.Pos()),
					Message: fmt.Sprintf("imports %s, and the register may import only %v", path, g.rules.leafImports),
				})
			case server == nil || server.Imports[path] == nil:
				found = append(found, Finding{
					Rule: "G12", Subject: g.reg.leaf, Position: g.p.position(spec.Pos()),
					Message: fmt.Sprintf("imports %s, which %s does not import itself", path, g.rules.server),
				})
			}
		}
		for _, decl := range file.Decls {
			if gen, isGen := decl.(*ast.GenDecl); isGen && gen.Tok == token.VAR {
				found = append(found, Finding{
					Rule: "G12", Subject: g.reg.leaf, Position: g.p.position(gen.Pos()),
					Message: "declares a package-level variable, which is initialization work in every binary that imports the register",
				})
			}
		}
	}
	return found
}
