package main

import (
	"fmt"
	"go/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkRefs is G5: an Enforce site that names a register constant in Reads
// refers to it in its body, directly or through a declared Alias of it, and
// every register function a row names in Functions is called from one of that
// row's Enforce sites. A promoted rule nobody consults is an answer that
// decides nothing.
func (g *gate) checkRefs() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		found = append(found, g.readFindings(d)...)
		found = append(found, g.functionFindings(d)...)
	}
	return found
}

// readFindings judges the Enforce sites of one row that name what they read.
func (g *gate) readFindings(d tenancy.Decision) []Finding {
	var found []Finding
	for _, s := range d.Sites {
		if s.Role != tenancy.Enforce || s.Reads == "" {
			continue
		}
		decl, err := g.p.lookup(s)
		if err != nil {
			continue
		}
		body := decl.body()
		switch {
		case g.leafConst(s.Reads) == nil:
			found = append(found, Finding{
				Rule: "G5", Subject: d.ID, Position: decl.where(g.p),
				Message: fmt.Sprintf("%s is declared to read %s, which is not a constant of the register", keyOf(s), s.Reads),
			})
		case body == nil || !g.refersTo(decl, body, s.Reads):
			found = append(found, Finding{
				Rule: "G5", Subject: d.ID, Position: decl.where(g.p),
				Message: fmt.Sprintf("%s does not refer to %s, or to a declared alias of it", keyOf(s), s.Reads),
			})
		}
	}
	return found
}

// refersTo reports whether node refers to the register constant reads, or to
// a declared Alias of it.
func (g *gate) refersTo(decl *declaration, node ast.Node, reads string) bool {
	refers := false
	ast.Inspect(node, func(n ast.Node) bool {
		if id, isIdent := n.(*ast.Ident); isIdent && g.isRegisterValue(decl.info().Uses[id], reads) {
			refers = true
		}
		return !refers
	})
	return refers
}

// functionFindings judges that each register function a row names is called
// from one of its Enforce sites.
func (g *gate) functionFindings(d tenancy.Decision) []Finding {
	var found []Finding
	for _, name := range d.Functions {
		fn := g.leafFunc(name)
		if fn == nil {
			found = append(found, Finding{Rule: "G5", Subject: d.ID, Message: fmt.Sprintf("names %s, which is not a function of the register", name)})
			continue
		}
		if !g.calledFromEnforce(d, fn.Name()) {
			found = append(found, Finding{
				Rule: "G5", Subject: d.ID,
				Message: fmt.Sprintf("names %s, and none of its Enforce sites calls it", name),
			})
		}
	}
	return found
}

// calledFromEnforce reports whether one of the row's Enforce sites calls the
// register function named name.
func (g *gate) calledFromEnforce(d tenancy.Decision, name string) bool {
	want := siteKey(g.reg.leaf, name)
	for _, s := range d.Sites {
		if s.Role != tenancy.Enforce {
			continue
		}
		decl, err := g.p.lookup(s)
		if err != nil || decl.body() == nil {
			continue
		}
		called := false
		ast.Inspect(decl.body(), func(n ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall {
				if fn := calleeOf(decl.info(), call); fn != nil && objectKey(fn) == want {
					called = true
				}
			}
			return !called
		})
		if called {
			return true
		}
	}
	return false
}
