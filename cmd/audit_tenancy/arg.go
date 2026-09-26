package main

import (
	"fmt"
	"go/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkArgs is G3: inside an Arg site's function, the calls to the callee it
// names pass the register constant at the argument index it names, and there
// are exactly as many of them as it says. A literal in that position, or a
// call added or removed, fails.
//
// It is deferred for a pending row, whose values have not moved yet.
func (g *gate) checkArgs() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		if !g.pending[d.ID] {
			found = append(found, g.argFindings(d)...)
		}
	}
	return found
}

// argFindings judges every Arg site of one row.
func (g *gate) argFindings(d tenancy.Decision) []Finding {
	var found []Finding
	for _, s := range d.Sites {
		if s.Role != tenancy.Arg {
			continue
		}
		if decl, err := g.p.lookup(s); err == nil {
			found = append(found, g.argSiteFindings(d.ID, s, decl)...)
		}
	}
	return found
}

// argSiteFindings judges one Arg site.
func (g *gate) argSiteFindings(id string, s tenancy.Site, decl *declaration) []Finding {
	var found []Finding
	fail := func(at, format string, args ...any) {
		found = append(found, Finding{Rule: "G3", Subject: id, Position: at, Message: keyOf(s) + " " + fmt.Sprintf(format, args...)})
	}
	if g.leafConst(s.Reads) == nil {
		fail(decl.where(g.p), "passes %s, which is not a constant of the register", s.Reads)
		return found
	}
	if decl.fn == nil {
		fail(decl.where(g.p), "is not a function, so it has no call to %s", s.Call)
		return found
	}
	calls := callsTo(decl, s.Call)
	if len(calls) != s.Count {
		fail(decl.where(g.p), "calls %s %d times, and the register says %d", s.Call, len(calls), s.Count)
	}
	for _, call := range calls {
		if s.Arg < 0 || s.Arg >= len(call.Args) {
			fail(g.p.position(call.Pos()), "calls %s with %d arguments, so it has none at index %d", s.Call, len(call.Args), s.Arg)
			continue
		}
		if msg := g.readsRegister(decl.info(), call.Args[s.Arg], s.Reads); msg != "" {
			fail(g.p.position(call.Args[s.Arg].Pos()), "argument %d of %s %s", s.Arg, s.Call, msg)
		}
	}
	return found
}

// callsTo are the calls inside a function to the callee a site names, "Func"
// or "Type.Method".
func callsTo(decl *declaration, callee string) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(decl.fn, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			if fn := calleeOf(decl.info(), call); fn != nil && objectName(fn) == callee {
				calls = append(calls, call)
			}
		}
		return true
	})
	return calls
}
