package main

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkPins is G4: a Pin site keeps its own literal, and the value and type it
// folds to equal the register constant it names. It is how the second
// statement of a default stated twice (issue 958) is held to the first
// without aliasing it, so drift between the two is a finding rather than a
// surprise.
//
// It applies to every row, pending or not: a pin moves nothing, so there is
// nothing for a later layer to do before it holds.
func (g *gate) checkPins() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		for _, s := range d.Sites {
			if s.Role != tenancy.Pin {
				continue
			}
			decl, err := g.p.lookup(s)
			if err != nil {
				continue
			}
			if msg := g.pinProblem(decl, s.Reads); msg != "" {
				found = append(found, Finding{Rule: "G4", Subject: d.ID, Position: decl.where(g.p), Message: keyOf(s) + " " + msg})
			}
		}
	}
	return found
}

// pinProblem says how a pinned declaration differs from the register
// constant reads, or returns empty when it does not.
func (g *gate) pinProblem(decl *declaration, reads string) string {
	reg := g.leafConst(reads)
	if reg == nil {
		return fmt.Sprintf("is pinned to %s, which is not a constant of the register", reads)
	}
	value, typ := pinnedValue(decl)
	if value == nil {
		return "does not fold to a constant, so it cannot be held equal to " + reads
	}
	if value.Kind() != reg.Val().Kind() || !constant.Compare(value, token.EQL, reg.Val()) {
		return fmt.Sprintf("is %s where %s is %s", value.ExactString(), reads, reg.Val().ExactString())
	}
	if !types.Identical(typ, reg.Type()) {
		return fmt.Sprintf("is typed %s where %s is %s", typ, reads, reg.Type())
	}
	return ""
}

// pinnedValue is the constant a pinned declaration holds and its type: a
// const's own, or a var's initializer folded.
func pinnedValue(decl *declaration) (constant.Value, types.Type) {
	if c, isConst := decl.obj.(*types.Const); isConst {
		return c.Val(), c.Type()
	}
	init := decl.initializer()
	if init == nil {
		return nil, nil
	}
	return decl.info().Types[init].Value, decl.obj.Type()
}
