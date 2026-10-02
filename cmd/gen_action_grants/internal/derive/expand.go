package derive

import (
	"go/ast"
	"go/types"
	"strings"
)

// context is what the expansion carries down a structure: whether a directive
// above said every request here runs, whether one said how they combine, and
// whether one said they may not run.
type context struct {
	forced           bool
	qualified        bool
	declaredOptional bool
	directive        *Directive
}

// expansion is the walk of one action through the structures of every
// function it calls.
type expansion struct {
	d            *deriver
	action       *Action
	index        map[string]int
	stack        map[*types.Func]bool
	declarations []*declared
	// sends is whether the action reaches the GraphQL transport.
	sends    bool
	overflow bool
	// recorded counts the requests recorded so far, which is how a directive
	// learns it qualified one.
	recorded int
}

// call expands a function called with call (nil for a root or a reference),
// its parameters bound in the caller's frame.
func (e *expansion) call(fn *types.Func, call *ast.CallExpr, caller *frame, ctx context) pathSet {
	if e.stack[fn] {
		return unit()
	}
	indexed, ok := e.d.prog.Function(fn)
	if !ok {
		return unit()
	}
	at := &frame{pkg: indexed.Package(), body: indexed.Root()}
	if decl := indexed.Decl(); decl != nil {
		at.body = decl.Body
		at.env = bind(indexed.Package(), decl.Type.Params, call, caller)
	} else if lit, isLit := indexed.Root().(*ast.FuncLit); isLit {
		at.body = lit.Body
		at.env = bind(indexed.Package(), lit.Type.Params, call, caller)
	}
	e.stack[fn] = true
	defer delete(e.stack, fn)
	return e.expand(e.d.body(fn), at, ctx)
}

// expand expands one node in a frame.
func (e *expansion) expand(n *node, at *frame, ctx context) pathSet {
	if n.directive == nil {
		return e.expandKind(n, at, ctx)
	}
	before := e.recorded
	ctx.directive = n.directive
	var result pathSet
	switch n.directive.Kind {
	case DirectiveOptional:
		ctx.declaredOptional = true
		e.expandKind(n, at, ctx)
		result = unit()
	case DirectiveMandatory:
		ctx.forced, ctx.qualified = true, true
		result = e.expandKind(n, at, ctx)
	default:
		ctx.qualified = true
		result = e.alternatives(n, at, ctx)
	}
	if e.recorded > before {
		n.directive.used = true
	}
	return result
}

// expandKind expands a node by its kind.
func (e *expansion) expandKind(n *node, at *frame, ctx context) pathSet {
	switch n.kind {
	case leafNode:
		return e.leaf(n.leaf, at, ctx)
	case optNode:
		inner := e.expand(n.kids[0], at, ctx)
		if ctx.forced {
			return inner
		}
		return unit()
	case altNode:
		var arms pathSet
		for _, kid := range n.kids {
			arm := e.expand(kid, at, ctx)
			if arm.isUnit() && (kid.failing || ctx.forced) {
				continue
			}
			arms = append(arms, arm...)
		}
		if len(arms) == 0 {
			return unit()
		}
		return minimize(arms)
	default:
		paths := unit()
		for _, kid := range n.kids {
			next, ok := product(paths, e.expand(kid, at, ctx))
			if !ok {
				e.overflow = true
				return paths
			}
			paths = next
		}
		return paths
	}
}

// alternatives expands a statement an alternatives directive qualifies: each
// leaf under it is one alternative, and a leaf that sends nothing is not one.
func (e *expansion) alternatives(n *node, at *frame, ctx context) pathSet {
	var arms pathSet
	for _, part := range leaves(n) {
		arm := e.expand(part, at, ctx)
		if !arm.isUnit() {
			arms = append(arms, arm...)
		}
	}
	if len(arms) == 0 {
		return unit()
	}
	return minimize(arms)
}

// leaves lists the leaf nodes under a node, in order.
func leaves(n *node) []*node {
	if n.kind == leafNode {
		return []*node{n}
	}
	var found []*node
	for _, kid := range n.kids {
		found = append(found, leaves(kid)...)
	}
	return found
}

// leaf expands one leaf.
func (e *expansion) leaf(l *leaf, at *frame, ctx context) pathSet {
	switch l.kind {
	case sdkLeaf:
		return e.sdk(l, ctx)
	case docLeaf:
		if !e.sends {
			return unit()
		}
		return e.record(l.document, l, ctx, "")
	case callLeaf:
		return e.call(l.callee, l.call, at, ctx)
	default:
		return e.raw(l, at, ctx)
	}
}

// sdk expands a client-go method into what it sends: one of its routes, and
// every document it posts.
func (e *expansion) sdk(l *leaf, ctx context) pathSet {
	routes, documents, known := e.d.sdk.Requests(l.sdk)
	if !known {
		return e.record(Request{Kind: KindUnresolved, Reason: "sdk-undeclared " + l.sdk}, l, ctx, l.sdk)
	}
	var routeSet pathSet
	for _, route := range routes {
		routeSet = append(routeSet, e.record(route, l, ctx, l.sdk)...)
	}
	paths := unit()
	if len(routeSet) > 0 {
		paths = minimize(routeSet)
	}
	// A method posting several documents posts every one of them on every
	// call (a work item delete reads the item's ID, then deletes it): that is
	// read from client-go, not guessed from a handler's syntax, so no author of
	// ours has a directive to write about it.
	if len(documents) > 1 {
		ctx.qualified = true
	}
	for _, document := range documents {
		next, ok := product(paths, e.record(document, l, ctx, l.sdk))
		if !ok {
			e.overflow = true
			return paths
		}
		paths = next
	}
	return paths
}

// raw expands a raw request into the routes its verb and path fold to, one of
// which is sent.
func (e *expansion) raw(l *leaf, at *frame, ctx context) pathSet {
	if l.toURL {
		return e.record(Request{Kind: KindUnresolved, Reason: "raw-url " + siteName(l.site)}, l, ctx, "")
	}
	var arms pathSet
	for _, verb := range e.d.fold(l.verb, at, 0) {
		for _, path := range e.d.fold(l.path, at, 0) {
			shape, ok := normalizePath(path)
			if !ok || verb == placeholder || strings.Contains(verb, unknownPiece) {
				arms = append(arms, e.record(Request{Kind: KindUnresolved, Reason: "raw-path " + siteName(l.site)}, l, ctx, "")...)
				continue
			}
			arms = append(arms, e.record(Request{Kind: KindREST, Method: strings.ToUpper(verb), Path: shape}, l, ctx, "")...)
		}
	}
	return minimize(arms)
}

// record adds a request to the action and returns the path that makes it. An
// unresolved request a declaration of the action answers is replaced by the
// declared requests. Every unresolved request carries its reason, so a
// declaration replacing nothing never meets one. A request a declaration
// says GitLab redirects is followed by the declared requests on the same
// path, after it, since the client sends them only once GitLab has answered
// it.
func (e *expansion) record(req Request, l *leaf, ctx context, sdk string) pathSet {
	if req.Kind == KindUnresolved {
		for _, entry := range e.declarations {
			if entry.Replaces == req.Reason {
				entry.used = true
				return e.declared(entry, l, ctx)
			}
		}
	}
	sent := single(e.use(req, l, ctx, sdk, ""))
	for _, entry := range e.declarations {
		if entry.Follows != "" && entry.Follows == req.Key() {
			entry.used = true
			// One path times the few a declaration lists never passes the
			// bound.
			followed, _ := product(sent, e.declared(entry, l, ctx))
			return followed
		}
	}
	return sent
}

// declared records the requests a declaration stands for.
func (e *expansion) declared(entry *declared, l *leaf, ctx context) pathSet {
	ctx.qualified = true
	var set pathSet
	if entry.Any {
		for _, req := range entry.Requests {
			set = append(set, single(e.use(req, l, ctx, "", entry.Category))...)
		}
		return minimize(set)
	}
	set = unit()
	for _, req := range entry.Requests {
		set, _ = product(set, single(e.use(req, l, ctx, "", entry.Category)))
	}
	return set
}

// use records one request of the action with where it came from, and returns
// its index.
func (e *expansion) use(req Request, l *leaf, ctx context, sdk, declaration string) int {
	e.recorded++
	key := req.Key()
	index, seen := e.index[key]
	if !seen {
		index = len(e.action.Uses)
		e.index[key] = index
		e.action.Uses = append(e.action.Uses, Use{Request: req})
	}
	use := &e.action.Uses[index]
	use.Sites = append(use.Sites, siteName(l.site))
	if sdk != "" {
		use.SDKMethods = append(use.SDKMethods, sdk)
	}
	if ctx.directive != nil {
		use.Directives = append(use.Directives, ctx.directive.String())
	}
	if declaration != "" {
		use.Declaration = declaration
	}
	use.Qualified = use.Qualified || ctx.qualified
	use.DeclaredOptional = use.DeclaredOptional || ctx.declaredOptional
	return index
}
