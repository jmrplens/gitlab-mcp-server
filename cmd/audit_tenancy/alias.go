package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkAliases is G2: an Alias site's initializer is a reference to the
// register constant it names, or to another declared Alias of that constant,
// and the site keeps the constant's own typedness. For a composite literal
// every element is an alias, and each element is declared.
//
// It is deferred for a pending row, whose values have not moved yet.
func (g *gate) checkAliases() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		if !g.pending[d.ID] {
			found = append(found, g.aliasFindings(d)...)
		}
	}
	return found
}

// aliasFindings judges every Alias site of one row.
func (g *gate) aliasFindings(d tenancy.Decision) []Finding {
	var found []Finding
	elements := map[string][]int{}
	for _, s := range d.Sites {
		if s.Role != tenancy.Alias {
			continue
		}
		decl, err := g.p.lookup(s)
		if err != nil {
			continue
		}
		fail := func(format string, args ...any) {
			found = append(found, Finding{Rule: "G2", Subject: d.ID, Position: decl.where(g.p), Message: keyOf(s) + " " + fmt.Sprintf(format, args...)})
		}
		reg := g.leafConst(s.Reads)
		if reg == nil {
			fail("aliases %s, which is not a constant of the register", s.Reads)
			continue
		}
		init := decl.initializer()
		if init == nil {
			fail("is not a const or var with an initializer of its own, so it cannot alias %s", s.Reads)
			continue
		}
		if lit, isLit := ast.Unparen(init).(*ast.CompositeLit); isLit {
			if msg := g.elementProblem(decl, lit, s); msg != "" {
				fail("%s", msg)
			}
			elements[decl.key] = append(elements[decl.key], s.Arg)
			continue
		}
		if msg := g.readsRegister(decl.info(), init, s.Reads); msg != "" {
			fail("%s", msg)
			continue
		}
		// Only a const can change typedness: a var takes its declared type or
		// the constant's default one, exactly as it did from the literal.
		if _, isConst := decl.obj.(*types.Const); isConst && !types.Identical(decl.obj.Type(), reg.Type()) {
			fail("is typed %s where %s is %s: an alias keeps the typedness of the literal it replaced", decl.obj.Type(), s.Reads, reg.Type())
		}
	}
	for _, key := range sortedKeys(elements) {
		decl := g.p.decls[key]
		lit, _ := ast.Unparen(decl.initializer()).(*ast.CompositeLit)
		for i := range lit.Elts {
			if !slices.Contains(elements[key], i) {
				found = append(found, Finding{
					Rule: "G2", Subject: d.ID, Position: g.p.position(lit.Elts[i].Pos()),
					Message: fmt.Sprintf("%s element %d is not declared as an alias: every element of an aliased literal is one", key, i),
				})
			}
		}
	}
	return found
}

// elementProblem judges one element of a composite literal alias, or says
// why it is not one.
//
// An element has no typedness of its own to keep: it takes the literal's
// element type, which an untyped constant converts to exactly as the literal
// it replaced did, and a typed constant is assignable only to its own type,
// which the compiler already holds.
func (g *gate) elementProblem(decl *declaration, lit *ast.CompositeLit, s tenancy.Site) string {
	if s.Arg < 0 || s.Arg >= len(lit.Elts) {
		return fmt.Sprintf("names element %d of a literal with %d elements", s.Arg, len(lit.Elts))
	}
	elem := lit.Elts[s.Arg]
	if kv, isKV := elem.(*ast.KeyValueExpr); isKV {
		elem = kv.Value
	}
	if msg := g.readsRegister(decl.info(), elem, s.Reads); msg != "" {
		return fmt.Sprintf("element %d %s", s.Arg, msg)
	}
	return ""
}

// readsRegister returns empty when expr is a reference to the register
// constant reads, or to a declared Alias of it, and otherwise says what expr
// is instead.
func (g *gate) readsRegister(info *types.Info, expr ast.Expr, reads string) string {
	if g.isRegisterValue(referenced(info, expr), reads) {
		return ""
	}
	switch e := ast.Unparen(expr).(type) {
	case *ast.BasicLit:
		return fmt.Sprintf("is the literal %s rather than %s", e.Value, reads)
	case *ast.Ident, *ast.SelectorExpr:
		return fmt.Sprintf("reads %s rather than %s", types.ExprString(e), reads)
	default:
		return fmt.Sprintf("is the expression %s rather than a reference to %s", types.ExprString(e), reads)
	}
}

// isRegisterValue reports whether obj is the register constant reads, or a
// declaration the register names as an Alias of it.
func (g *gate) isRegisterValue(obj types.Object, reads string) bool {
	if obj == nil {
		return false
	}
	key := objectKey(obj)
	if key == siteKey(g.reg.leaf, reads) {
		_, isConst := obj.(*types.Const)
		return isConst
	}
	return g.aliasOf(key, reads)
}

// aliasOf reports whether the register names key as an Alias of reads.
func (g *gate) aliasOf(key, reads string) bool {
	return g.aliases[key][reads]
}

// aliasIndex maps each declaration the register names as an Alias to the
// register constants it is declared to carry.
func aliasIndex(reg register) map[string]map[string]bool {
	index := map[string]map[string]bool{}
	for _, d := range reg.decisions {
		for _, s := range d.Sites {
			if s.Role != tenancy.Alias {
				continue
			}
			if index[keyOf(s)] == nil {
				index[keyOf(s)] = map[string]bool{}
			}
			index[keyOf(s)][s.Reads] = true
		}
	}
	return index
}

// leafConst is the register constant named name, or nil.
func (g *gate) leafConst(name string) *types.Const {
	leaf := g.p.byDir[g.reg.leaf]
	if leaf == nil || name == "" {
		return nil
	}
	c, _ := leaf.Types.Scope().Lookup(name).(*types.Const)
	return c
}

// leafFunc is the register function named name, or nil.
func (g *gate) leafFunc(name string) *types.Func {
	leaf := g.p.byDir[g.reg.leaf]
	if leaf == nil || name == "" {
		return nil
	}
	fn, _ := leaf.Types.Scope().Lookup(name).(*types.Func)
	return fn
}
