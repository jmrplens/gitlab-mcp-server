package derive

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// nodeKind is how a node's children combine.
type nodeKind uint8

const (
	// seqNode runs every child.
	seqNode nodeKind = iota
	// altNode runs one child.
	altNode
	// optNode may run its one child, or not.
	optNode
	// leafNode is one thing a body names that can lead to a request.
	leafNode
)

// node is one part of a body's structure.
type node struct {
	kind nodeKind
	kids []*node
	leaf *leaf
	// directive is the directive qualifying the statement this node stands
	// for.
	directive *Directive
	// failing marks an arm that ends returning an error: a failed call, which
	// is not a way the action runs when it sends nothing.
	failing bool
}

// leafKind is what a leaf names.
type leafKind uint8

const (
	// sdkLeaf names a client-go service method.
	sdkLeaf leafKind = iota
	// docLeaf names a GraphQL document.
	docLeaf
	// callLeaf names a function of the loaded source.
	callLeaf
	// rawLeaf builds a request through client-go's request constructor.
	rawLeaf
)

// leaf is one name in a body that can lead to a request.
type leaf struct {
	kind leafKind
	// site is the function whose body the leaf is written in, which is where
	// a reader looks for the request.
	site *types.Func
	pkg  *packages.Package
	// sdk is a client-go method key.
	sdk string
	// document is a GraphQL request.
	document Request
	// callee is the function a call leaf names, and call the call expression
	// when the leaf is called right there, whose arguments bind the callee's
	// parameters.
	callee *types.Func
	call   *ast.CallExpr
	// verb and path are a raw request's arguments, and toURL marks one built
	// from an absolute URL, which names no route.
	verb, path ast.Expr
	toURL      bool
}

// emptyNode returns a node with nothing in it.
func emptyNode() *node { return &node{kind: seqNode} }

// seqOf returns a sequence of the nodes that hold anything.
func seqOf(parts ...*node) *node {
	seq := emptyNode()
	for _, part := range parts {
		if part != nil && !part.isEmpty() {
			seq.kids = append(seq.kids, part)
		}
	}
	return seq
}

// isEmpty reports whether a node holds no leaf at all.
func (n *node) isEmpty() bool {
	if n.kind == leafNode {
		return false
	}
	for _, kid := range n.kids {
		if !kid.isEmpty() {
			return false
		}
	}
	return n.directive == nil
}

// builder reads one body into its structure.
type builder struct {
	d *deriver
	// site is the function the body belongs to, and fn its index entry.
	site *types.Func
	fn   *actionrequests.Function
	pkg  *packages.Package
}

// block reads a list of statements in order, taking the rest of the list as
// the arm that did not return wherever a branching statement has an arm that
// does.
func (b *builder) block(stmts []ast.Stmt) *node {
	seq := emptyNode()
	for i, stmt := range stmts {
		br, ok := b.branching(stmt)
		if !ok {
			seq.kids = append(seq.kids, b.stmt(stmt))
			continue
		}
		directive := b.d.forStatement(stmt)
		if !br.anyTerminates(b) {
			whole := b.fromBranch(br)
			whole.directive = directive
			seq.kids = append(seq.kids, whole)
			continue
		}
		rest := b.block(stmts[i+1:])
		alt := &node{kind: altNode}
		for _, arm := range br.arms {
			armNode := b.arm(arm)
			if !b.terminates(arm) {
				armNode = seqOf(armNode, rest)
			}
			alt.kids = append(alt.kids, armNode)
		}
		if br.implicit {
			alt.kids = append(alt.kids, rest)
		}
		whole := seqOf(br.prefix, alt)
		whole.directive = directive
		seq.kids = append(seq.kids, whole)
		return seq
	}
	return seq
}

// branch is a statement whose arms are alternatives.
type branch struct {
	// prefix is what the statement evaluates on every call: an init
	// statement, a condition, a switch tag.
	prefix *node
	arms   [][]ast.Stmt
	// implicit is set when no arm may be taken: an if with no else, a switch
	// with no default, a select.
	implicit bool
}

// anyTerminates reports whether some arm ends the function.
func (br *branch) anyTerminates(b *builder) bool {
	return slices.ContainsFunc(br.arms, b.terminates)
}

// branching reads a statement whose arms are alternatives, and reports false
// for any other statement.
func (b *builder) branching(stmt ast.Stmt) (branch, bool) {
	switch typed := stmt.(type) {
	case *ast.IfStmt:
		br := branch{prefix: seqOf(b.stmt(typed.Init), b.expr(typed.Cond)), arms: [][]ast.Stmt{typed.Body.List}}
		switch elseStmt := typed.Else.(type) {
		case nil:
			br.implicit = true
		case *ast.BlockStmt:
			br.arms = append(br.arms, elseStmt.List)
		default:
			br.arms = append(br.arms, []ast.Stmt{elseStmt})
		}
		return br, true
	case *ast.SwitchStmt:
		br := branch{prefix: seqOf(b.stmt(typed.Init), b.expr(typed.Tag)), implicit: true}
		br.addClauses(typed.Body)
		return br, true
	case *ast.TypeSwitchStmt:
		br := branch{prefix: seqOf(b.stmt(typed.Init), b.stmt(typed.Assign)), implicit: true}
		br.addClauses(typed.Body)
		return br, true
	case *ast.SelectStmt:
		br := branch{prefix: emptyNode(), implicit: true}
		for _, clause := range typed.Body.List {
			comm, _ := clause.(*ast.CommClause)
			br.arms = append(br.arms, append(stmtsOf(comm.Comm), comm.Body...))
		}
		return br, true
	}
	return branch{}, false
}

// addClauses adds a switch's case clauses as arms; a default clause means
// some arm is always taken.
func (br *branch) addClauses(body *ast.BlockStmt) {
	for _, clause := range body.List {
		cases, _ := clause.(*ast.CaseClause)
		if cases.List == nil {
			br.implicit = false
		}
		br.arms = append(br.arms, cases.Body)
	}
}

// stmtsOf wraps one statement as a list, nothing for none.
func stmtsOf(stmt ast.Stmt) []ast.Stmt {
	if stmt == nil {
		return nil
	}
	return []ast.Stmt{stmt}
}

// arm reads one arm, marking it failing when it ends returning an error.
func (b *builder) arm(stmts []ast.Stmt) *node {
	armNode := b.block(stmts)
	armNode.failing = b.failing(stmts)
	return armNode
}

// stmt reads one statement.
func (b *builder) stmt(stmt ast.Stmt) *node {
	if stmt == nil {
		return emptyNode()
	}
	built := b.statement(stmt)
	if directive := b.d.forStatement(stmt); directive != nil {
		built = &node{kind: seqNode, kids: []*node{built}, directive: directive}
	}
	return built
}

// statement reads one statement's structure, without its directive.
func (b *builder) statement(stmt ast.Stmt) *node {
	switch typed := stmt.(type) {
	case *ast.ExprStmt:
		return b.expr(typed.X)
	case *ast.AssignStmt:
		return seqOf(b.exprs(typed.Rhs), b.exprs(typed.Lhs))
	case *ast.DeclStmt:
		return b.decl(typed)
	case *ast.ReturnStmt:
		return b.exprs(typed.Results)
	case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		br, _ := b.branching(stmt)
		return b.fromBranch(br)
	case *ast.ForStmt:
		return seqOf(b.stmt(typed.Init), b.expr(typed.Cond), &node{kind: optNode, kids: []*node{seqOf(b.block(typed.Body.List), b.stmt(typed.Post))}})
	case *ast.RangeStmt:
		return seqOf(b.expr(typed.X), &node{kind: optNode, kids: []*node{b.block(typed.Body.List)}})
	case *ast.BlockStmt:
		return b.block(typed.List)
	case *ast.LabeledStmt:
		return b.stmt(typed.Stmt)
	case *ast.GoStmt:
		return b.expr(typed.Call)
	case *ast.DeferStmt:
		return b.expr(typed.Call)
	case *ast.SendStmt:
		return seqOf(b.expr(typed.Chan), b.expr(typed.Value))
	case *ast.IncDecStmt:
		return b.expr(typed.X)
	}
	return emptyNode()
}

// fromBranch reads a branching statement none of whose arms returns: its
// prefix, then its arms as alternatives, with an arm that sends nothing when
// no arm may be taken.
func (b *builder) fromBranch(br branch) *node {
	alt := &node{kind: altNode}
	for _, arm := range br.arms {
		alt.kids = append(alt.kids, b.arm(arm))
	}
	if br.implicit {
		alt.kids = append(alt.kids, emptyNode())
	}
	return seqOf(br.prefix, alt)
}

// decl reads a declaration statement's initializers.
func (b *builder) decl(stmt *ast.DeclStmt) *node {
	seq := emptyNode()
	gen, ok := stmt.Decl.(*ast.GenDecl)
	if !ok {
		return seq
	}
	for _, spec := range gen.Specs {
		if value, isValue := spec.(*ast.ValueSpec); isValue {
			seq.kids = append(seq.kids, b.exprs(value.Values))
		}
	}
	return seq
}

// exprs reads expressions in order.
func (b *builder) exprs(list []ast.Expr) *node {
	seq := emptyNode()
	for _, expr := range list {
		seq.kids = append(seq.kids, b.expr(expr))
	}
	return seq
}

// expr reads one expression.
func (b *builder) expr(expr ast.Expr) *node {
	switch typed := expr.(type) {
	case *ast.Ident:
		return b.ident(typed, nil)
	case *ast.BasicLit:
		return b.literal(typed)
	case *ast.CompositeLit:
		return b.composite(typed)
	case *ast.FuncLit:
		// A closure runs where it is written: the handler that defines one
		// hands it to a helper that calls it.
		return b.block(typed.Body.List)
	case *ast.ParenExpr:
		return b.expr(typed.X)
	case *ast.SelectorExpr:
		return seqOf(b.expr(typed.X), b.ident(typed.Sel, nil))
	case *ast.IndexExpr:
		return seqOf(b.expr(typed.X), b.expr(typed.Index))
	case *ast.IndexListExpr:
		return seqOf(b.expr(typed.X), b.exprs(typed.Indices))
	case *ast.SliceExpr:
		return seqOf(b.expr(typed.X), b.expr(typed.Low), b.expr(typed.High), b.expr(typed.Max))
	case *ast.TypeAssertExpr:
		return b.expr(typed.X)
	case *ast.CallExpr:
		return b.callExpr(typed)
	case *ast.StarExpr:
		return b.expr(typed.X)
	case *ast.UnaryExpr:
		return b.expr(typed.X)
	case *ast.BinaryExpr:
		if typed.Op == token.LAND || typed.Op == token.LOR {
			return seqOf(b.expr(typed.X), &node{kind: optNode, kids: []*node{b.expr(typed.Y)}})
		}
		return seqOf(b.expr(typed.X), b.expr(typed.Y))
	case *ast.KeyValueExpr:
		return seqOf(b.expr(typed.Key), b.expr(typed.Value))
	}
	return emptyNode()
}

// callExpr reads a call: what it calls, and its arguments, two or more of
// which, when they name functions, are the variants the callee chooses
// between.
func (b *builder) callExpr(call *ast.CallExpr) *node {
	var callee *node
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		callee = b.ident(fun, call)
	case *ast.SelectorExpr:
		callee = seqOf(b.expr(fun.X), b.ident(fun.Sel, call))
	case *ast.IndexExpr:
		callee = b.instantiated(fun.X, fun, call)
	case *ast.IndexListExpr:
		callee = b.instantiated(fun.X, fun, call)
	default:
		callee = b.expr(fun)
	}
	return seqOf(callee, b.choices(call.Args))
}

// instantiated reads a call through an index expression: a generic function
// instantiated in place, which is called right there, or a table looked up by
// a key, which is not.
func (b *builder) instantiated(x, index ast.Expr, call *ast.CallExpr) *node {
	if ident, ok := ast.Unparen(x).(*ast.Ident); ok {
		if _, isFunc := b.pkg.TypesInfo.Uses[ident].(*types.Func); isFunc {
			return b.ident(ident, call)
		}
	}
	return b.expr(index)
}

// choices reads a list of expressions, two or more of which naming functions
// are alternatives: a helper handed a project, a group and an instance variant
// calls one.
func (b *builder) choices(list []ast.Expr) *node {
	var refs, rest []ast.Expr
	for _, expr := range list {
		if b.namesFunction(expr) {
			refs = append(refs, expr)
		} else {
			rest = append(rest, expr)
		}
	}
	if len(refs) < 2 {
		return b.exprs(list)
	}
	alt := &node{kind: altNode}
	for _, ref := range refs {
		alt.kids = append(alt.kids, b.expr(ref))
	}
	return seqOf(b.exprs(rest), alt)
}

// namesFunction reports whether an expression names a function value without
// calling it: a client-go method value or a function of the loaded source.
func (b *builder) namesFunction(expr ast.Expr) bool {
	var ident *ast.Ident
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		ident = typed
	case *ast.SelectorExpr:
		ident = typed.Sel
	default:
		return false
	}
	fn, ok := b.pkg.TypesInfo.Uses[ident].(*types.Func)
	if !ok {
		return false
	}
	if _, isService := actionrequests.ServiceMethod(fn); isService {
		return true
	}
	_, ours := b.d.prog.Function(fn.Origin())
	return ours
}

// composite reads a composite literal; two or more function elements are the
// entries of a dispatch table, of which one runs.
func (b *builder) composite(lit *ast.CompositeLit) *node {
	var funcs, rest []ast.Expr
	for _, element := range lit.Elts {
		value := element
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			value = pair.Value
		}
		if _, isLit := ast.Unparen(value).(*ast.FuncLit); isLit || b.namesFunction(value) {
			funcs = append(funcs, value)
		} else {
			rest = append(rest, element)
		}
	}
	if len(funcs) < 2 {
		return b.exprs(lit.Elts)
	}
	alt := &node{kind: altNode}
	for _, fn := range funcs {
		alt.kids = append(alt.kids, b.expr(fn))
	}
	return seqOf(b.exprs(rest), alt)
}

// literal reads a string literal that is a GraphQL document written inline.
func (b *builder) literal(lit *ast.BasicLit) *node {
	value := b.pkg.TypesInfo.Types[lit].Value
	if value == nil || value.Kind() != constant.String {
		return emptyNode()
	}
	text := constant.StringVal(value)
	if !graphqldocs.LooksLikeDocument(text) {
		return emptyNode()
	}
	return b.leaf(&leaf{kind: docLeaf, document: Request{Kind: KindGraphQL, Document: text}})
}

// ident reads what one name names. call is the call expression when the name
// is called right there.
func (b *builder) ident(ident *ast.Ident, call *ast.CallExpr) *node {
	obj := b.pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return emptyNode()
	}
	if text, ok := b.d.prog.Document(obj); ok {
		return b.leaf(&leaf{kind: docLeaf, document: Request{Kind: KindGraphQL, Document: text, Name: obj.Name()}})
	}
	switch typed := obj.(type) {
	case *types.Func:
		return b.function(typed.Origin(), call)
	case *types.Var:
		return b.variable(typed, call)
	}
	return emptyNode()
}

// function reads a name of a function.
func (b *builder) function(fn *types.Func, call *ast.CallExpr) *node {
	// The GraphQL transport is a request only with a document, which the body
	// names on its own: a document named where the transport is reached is
	// the request, and the transport adds nothing to it.
	if actionrequests.SendsGraphQL(fn) {
		return emptyNode()
	}
	if key, ok := actionrequests.ServiceMethod(fn); ok {
		return b.leaf(&leaf{kind: sdkLeaf, sdk: key})
	}
	if call != nil && isRequestConstructor(fn) {
		return b.raw(fn, call, 0)
	}
	if _, ours := b.d.prog.Function(fn); ours {
		return b.leaf(&leaf{kind: callLeaf, callee: fn, call: call})
	}
	return emptyNode()
}

// variable reads a name of a variable: a package-level one whose initializer
// was indexed, or a parameter of a route helper a handler literal calls
// through.
func (b *builder) variable(variable *types.Var, call *ast.CallExpr) *node {
	if stand, ok := b.d.prog.Initializer(variable); ok {
		if fn, isSeam := b.d.requestSeam(stand); isSeam && call != nil {
			return b.raw(fn, call, 1)
		}
		return b.leaf(&leaf{kind: callLeaf, callee: stand, call: call})
	}
	bound := b.fn.Bound(variable)
	if len(bound) == 0 {
		return emptyNode()
	}
	alt := &node{kind: altNode}
	for _, fn := range bound {
		alt.kids = append(alt.kids, b.leaf(&leaf{kind: callLeaf, callee: fn}))
	}
	return alt
}

// raw reads a call of client-go's request constructor, whose verb and path
// are its arguments after offset (one for a method expression held in a
// variable, whose first argument is the client).
func (b *builder) raw(fn *types.Func, call *ast.CallExpr, offset int) *node {
	built := &leaf{kind: rawLeaf, toURL: fn.Name() == "NewRequestToURL"}
	if len(call.Args) > offset {
		built.verb = call.Args[offset]
	}
	if len(call.Args) > offset+1 {
		built.path = call.Args[offset+1]
	}
	return b.leaf(built)
}

// leaf wraps a leaf in a node, recording where it was written.
func (b *builder) leaf(built *leaf) *node {
	built.site = b.site
	built.pkg = b.pkg
	return &node{kind: leafNode, leaf: built}
}

// requestConstructors are client-go Client's methods that build a request a
// handler then sends itself.
var requestConstructors = map[string]bool{"NewRequest": true, "UploadRequest": true, "NewRequestToURL": true}

// isRequestConstructor reports whether fn is one of client-go Client's request
// constructors.
func isRequestConstructor(fn *types.Func) bool {
	if !requestConstructors[fn.Name()] {
		return false
	}
	recv := fn.Signature().Recv()
	if recv == nil {
		return false
	}
	typ := recv.Type()
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	named, ok := typ.(*types.Named)
	return ok && named.Obj().Name() == "Client" && named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == actionrequests.ClientGoPath
}

// terminates reports whether a list of statements ends the function: a
// return, a panic, or an if whose arms both end it.
func (b *builder) terminates(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch last := stmts[len(stmts)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.ExprStmt:
		return b.isPanic(last.X)
	case *ast.BlockStmt:
		return b.terminates(last.List)
	case *ast.IfStmt:
		return last.Else != nil && b.terminates(last.Body.List) && b.terminates(stmtsOf(last.Else))
	}
	return false
}

// isPanic reports whether an expression is a call of the builtin panic.
func (b *builder) isPanic(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, isBuiltin := b.pkg.TypesInfo.Uses[ident].(*types.Builtin)
	return isBuiltin && builtin.Name() == "panic"
}

// failing reports whether an arm ends returning an error: a panic, or a
// return whose last result is an error value other than nil. A return whose
// one result is a call answering several values hands on what the call
// answers, which is not a failure of its own.
func (b *builder) failing(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch last := stmts[len(stmts)-1].(type) {
	case *ast.ExprStmt:
		return b.isPanic(last.X)
	case *ast.ReturnStmt:
		if len(last.Results) == 0 {
			return false
		}
		result := ast.Unparen(last.Results[len(last.Results)-1])
		if ident, ok := result.(*ast.Ident); ok && ident.Name == "nil" {
			return false
		}
		return isError(b.pkg.TypesInfo.TypeOf(result))
	}
	return false
}

// errorType is the predeclared error interface.
var errorType = types.Universe.Lookup("error").Type()

// isError reports whether a type is the error interface.
func isError(typ types.Type) bool {
	return typ != nil && types.Identical(typ, errorType)
}
