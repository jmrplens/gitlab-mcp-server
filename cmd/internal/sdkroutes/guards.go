// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkroutes

import (
	"go/ast"
	"go/token"
	"maps"
	"slices"
)

// NilGuard says a route of a method is sent only under a nil check of one of
// the method's own parameters: the label update and delete send
// /projects/:/labels/: when lid is not nil and /projects/:/labels when it is,
// so what a caller hands lid decides which of the two the call can send.
type NilGuard struct {
	// Param is the parameter's name, and Index its position among the method's
	// parameters, the first counted 0.
	Param string
	Index int
	// Nil is true for a route sent only while the parameter is nil, and false
	// for one sent only while it is not.
	Nil bool
}

// readNilChecks records, for each call in a function's body, the nil checks of
// a parameter whose branch encloses it, outermost first. A check it cannot read
// as exactly one parameter against nil (a condition of several terms, nil
// written first), and one of a parameter the body assigns, whose value then
// need not be the one the caller handed, decides nothing, which leaves a route
// they enclose read as sent on every call.
func readNilChecks(fn *function, body *ast.BlockStmt) {
	assigned := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		if assign, isAssign := node.(*ast.AssignStmt); isAssign {
			for _, target := range assign.Lhs {
				if ident, isIdent := target.(*ast.Ident); isIdent {
					assigned[ident.Name] = true
				}
			}
		}
		return true
	})
	ast.Inspect(body, func(node ast.Node) bool {
		if check, isIf := node.(*ast.IfStmt); isIf {
			fn.addNilCheck(check, assigned)
		}
		return true
	})
}

// addNilCheck records the two sides of an if whose condition is a nil check of
// a parameter of fn the body does not assign, and nothing for any other if.
func (fn *function) addNilCheck(check *ast.IfStmt, assigned map[string]bool) {
	name, nilWhenTrue := nilCheckOf(check.Cond)
	index := slices.Index(fn.params, name)
	if index < 0 || assigned[name] {
		return
	}
	fn.guardCalls(check.Body, NilGuard{Param: name, Index: index, Nil: nilWhenTrue})
	if check.Else != nil {
		fn.guardCalls(check.Else, NilGuard{Param: name, Index: index, Nil: !nilWhenTrue})
	}
}

// guardCalls records a guard on every call one side of a check encloses.
func (fn *function) guardCalls(branch ast.Node, guard NilGuard) {
	ast.Inspect(branch, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall {
			fn.guards[call] = append(fn.guards[call], guard)
		}
		return true
	})
}

// nilCheckOf reads a condition that compares one name with nil: the name, ""
// for any other condition, and whether it holds while the name is nil.
func nilCheckOf(condition ast.Expr) (name string, nilWhenTrue bool) {
	binary, isBinary := condition.(*ast.BinaryExpr)
	if !isBinary || (binary.Op != token.EQL && binary.Op != token.NEQ) {
		return "", false
	}
	operand, isName := binary.X.(*ast.Ident)
	against, isIdent := binary.Y.(*ast.Ident)
	if !isName || !isIdent || against.Name != "nil" {
		return "", false
	}
	return operand.Name, binary.Op == token.EQL
}

// routeGuards folds what guards each way a method reaches a route: only a
// check every way passes through keeps the route from being sent, so a route
// also sent outside a check, or under another one, is not held to it.
type routeGuards map[Route][]NilGuard

// note folds one more way of reaching a route into what the earlier ones hold.
func (g routeGuards) note(route Route, guards []NilGuard) {
	earlier, seen := g[route]
	if !seen {
		g[route] = guards
		return
	}
	g[route] = slices.DeleteFunc(slices.Clone(earlier), func(guard NilGuard) bool { return !slices.Contains(guards, guard) })
}

// held is the routes left with a guard, nil when none is, so a method with no
// nil check reads the same however it was built.
func (g routeGuards) held() map[Route][]NilGuard {
	maps.DeleteFunc(g, func(_ Route, guards []NilGuard) bool { return len(guards) == 0 })
	if len(g) == 0 {
		return nil
	}
	return g
}

// ownGuards is what a use of a template written in fn is guarded by, as a
// method entered at entry sees it: a check in the body of a function the method
// delegates to is of that function's parameters, none of which a caller hands.
func ownGuards(fn, entry *function, use templateUse) []NilGuard {
	if fn != entry {
		return nil
	}
	return use.guards
}
