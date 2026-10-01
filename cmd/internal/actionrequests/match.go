package actionrequests

import (
	"go/ast"
	"go/types"
	"strings"
)

// literalName is what the stand-in for a handler written as a function literal
// is called, which is what a reader printing the function a request is made
// from shows for one.
const literalName = "func literal"

// Match picks the construction sites that declare one catalog action.
//
// The join is on the canonical ID, through what a site can be read for: its
// action name, the individual tool name its options declare, and the package
// it is written in. A site whose tool name was read and is not the action's
// declares another action whatever its name and package, so it is never
// taken; a site whose tool name is the action's is the action's, and when one
// is found nothing else is taken. Only where no site's tool name could be read
// does the package decide, and when none of those is the owner, every site
// with the name is taken, because a wrong guess must not be the quiet one.
func Match(sites map[string][]Site, act Action) []Site {
	tool := strings.TrimSpace(act.Tool)
	var exact, open []Site
	for _, candidate := range sites[act.Name] {
		switch candidate.Tool {
		case "":
			open = append(open, candidate)
		case tool:
			exact = append(exact, candidate)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	var owned []Site
	for _, candidate := range open {
		if candidate.Package == act.Owner {
			owned = append(owned, candidate)
		}
	}
	if len(owned) > 0 {
		return owned
	}
	return open
}

// Roots turns the handlers of matched sites into call-graph roots, one per
// handler.
//
// A handler written as a function literal has no declared function, so its
// body is indexed as a function of its own under a stand-in, and the stand-in
// is the root: what the literal names directly is part of what the action
// does, and so is what its callees do. A literal that calls a parameter of the
// route helper it was written in (awardEmojiDeleteRoute wraps the delete it
// was handed) calls whatever the helper's caller bound to that parameter, and
// that function is recorded as one of the stand-in's calls.
//
// A literal is indexed afresh on every call, because what it calls through a
// parameter depends on the frame it was resolved in: the four guided creation
// flows are one literal in elicitationRoute, each bound to its own flow, and a
// stand-in kept across calls would hand every flow the first one's.
func (p *Program) Roots(matched []Site) []*types.Func {
	res := &resolver{prog: p}
	entered := make(map[*ast.FuncLit]*types.Func)
	var roots []*types.Func
	for _, site := range matched {
		for _, handler := range site.Handlers {
			roots = append(roots, res.handlerRoot(handler, entered))
		}
	}
	return roots
}

// handlerRoot returns the function a handler's walk starts from, indexing a
// literal under a stand-in of its own. A literal reached again while its own
// bound parameters are being resolved is the stand-in already made, so a
// literal bound to itself is not entered twice.
func (r *resolver) handlerRoot(handler Handler, entered map[*ast.FuncLit]*types.Func) *types.Func {
	if handler.Func != nil {
		return handler.Func
	}
	if stand, ok := entered[handler.Lit]; ok {
		return stand
	}
	stand := types.NewFunc(handler.Lit.Pos(), handler.pkg.Types, literalName, types.NewSignatureType(nil, nil, nil, nil, nil, false))
	entered[handler.Lit] = stand
	fn := r.prog.indexBody(handler.pkg, handler.Lit.Body)
	fn.decl = &ast.FuncDecl{Name: ast.NewIdent(literalName)}
	r.prog.link(fn)
	r.prog.funcs[stand] = fn
	for _, bound := range r.boundHandlers(handler) {
		root := r.handlerRoot(bound.handler, entered)
		fn.calls[root] = true
		if fn.bound == nil {
			fn.bound = make(map[*types.Var][]*types.Func)
		}
		fn.bound[bound.variable] = append(fn.bound[bound.variable], root)
	}
	return stand
}

// boundHandler is one handler a literal reaches through a parameter of the
// route helper it was written in, with the parameter it reaches it through.
type boundHandler struct {
	variable *types.Var
	handler  Handler
}

// boundHandlers resolves every function-valued parameter a literal names to
// the handlers its caller bound to it, in the order the literal first names
// each parameter.
func (r *resolver) boundHandlers(handler Handler) []boundHandler {
	seen := make(map[*types.Var]bool)
	var found []boundHandler
	ast.Inspect(handler.Lit.Body, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		variable, isVar := handler.pkg.TypesInfo.Uses[ident].(*types.Var)
		if !isVar || seen[variable] {
			return true
		}
		bound, isBound := handler.at.env[variable]
		if !isBound {
			return true
		}
		seen[variable] = true
		for _, resolved := range r.resolveHandler(bound.expr, bound.frame, 0) {
			found = append(found, boundHandler{variable: variable, handler: resolved})
		}
		return true
	})
	return found
}
