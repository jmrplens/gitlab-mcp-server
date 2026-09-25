package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"net/textproto"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// refusalType is a composite literal type a refusal is built from, and the
// names of its fields: the JSON-RPC error, the coded error that carries one,
// and the gate's own failure.
type refusalType struct {
	// name is the type's full name, "github.com/.../cmd/server.gateFailure".
	name string
	// code, status, message and header name its fields; a type without one
	// leaves it empty.
	code, status, message, header string
}

// refusalLit is one composite literal of a refusal type, with its fields by
// name whether it was written keyed or positional.
type refusalLit struct {
	lit    *ast.CompositeLit
	typ    refusalType
	fields map[string]ast.Expr
}

// typeName is the full name of a named type, through an alias and a pointer.
func typeName(t types.Type) string {
	t = types.Unalias(t)
	if ptr, isPtr := t.(*types.Pointer); isPtr {
		t = types.Unalias(ptr.Elem())
	}
	named, isNamed := t.(*types.Named)
	if !isNamed || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

// refusalLiteral reads lit as a refusal literal, or reports that it is not
// one of the refusal types.
func (g *gate) refusalLiteral(info *types.Info, lit *ast.CompositeLit) (refusalLit, bool) {
	t := info.TypeOf(lit)
	name := typeName(t)
	i := slices.IndexFunc(g.rules.refusalTypes, func(rt refusalType) bool { return rt.name == name })
	if i < 0 {
		return refusalLit{}, false
	}
	fields := map[string]ast.Expr{}
	st, _ := types.Unalias(t).Underlying().(*types.Struct)
	for pos, elt := range lit.Elts {
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			if id, isIdent := kv.Key.(*ast.Ident); isIdent {
				fields[id.Name] = kv.Value
			}
			continue
		}
		// A positional struct literal names every field in order.
		if st != nil {
			fields[st.Field(pos).Name()] = elt
		}
	}
	return refusalLit{lit: lit, typ: g.rules.refusalTypes[i], fields: fields}, true
}

// refusalLiterals are the refusal literals of the named type inside a
// declaration, in source order; an empty name takes every refusal type.
func (g *gate) refusalLiterals(decl *declaration, only string) []refusalLit {
	body := decl.body()
	if body == nil {
		return nil
	}
	var out []refusalLit
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, isLit := n.(*ast.CompositeLit); isLit {
			if rl, ok := g.refusalLiteral(decl.info(), lit); ok && (only == "" || rl.typ.name == only) {
				out = append(out, rl)
			}
		}
		return true
	})
	return out
}

// intField is the integer a field of the literal folds to. A type without
// the field names it as empty, which no literal sets.
func (r refusalLit) intField(info *types.Info, field string) (int, bool) {
	expr, ok := r.fields[field]
	if !ok {
		return 0, false
	}
	return foldInt(info, expr)
}

// foldInt is the integer an expression folds to, and false for one that folds
// to nothing or to no integer.
func foldInt(info *types.Info, expr ast.Expr) (int, bool) {
	value := info.Types[expr].Value
	if value == nil {
		return 0, false
	}
	v, exact := constant.Int64Val(constant.ToInt(value))
	return int(v), exact
}

// constString is the string an expression folds to.
func constString(info *types.Info, expr ast.Expr) (string, bool) {
	tv, ok := info.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// leadingText is the text an expression is known to begin with: all of it
// when it folds, its left operand's when it is a concatenation, and a format
// string up to its first verb when it is a formatting call. It is empty when
// nothing at the start folds.
func (g *gate) leadingText(info *types.Info, expr ast.Expr) string {
	expr = ast.Unparen(expr)
	if s, ok := constString(info, expr); ok {
		return s
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		// A string is built by one operator, so this is a concatenation.
		return g.leadingText(info, e.X)
	case *ast.CallExpr:
		if format := g.formatArg(info, e); format != nil {
			if s, ok := constString(info, format); ok {
				return beforeVerb(s)
			}
		}
	}
	return ""
}

// formatArg is the format argument of a call to one of the formatting
// functions, or nil.
func (g *gate) formatArg(info *types.Info, call *ast.CallExpr) ast.Expr {
	fn := calleeOf(info, call)
	if fn == nil || !slices.Contains(g.rules.formatters, calleeName(fn)) {
		return nil
	}
	return call.Args[0]
}

// beforeVerb is a format string up to its first verb, with an escaped percent
// read as the percent it prints.
func beforeVerb(format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteByte('%')
			i++
			continue
		}
		break
	}
	return b.String()
}

// foldedTexts are the string constants a declaration folds, each as far as
// it is known: a format string only up to its first verb, since what follows
// the verb is not what the caller receives.
func (g *gate) foldedTexts(decl *declaration) []string {
	body := decl.body()
	if body == nil {
		return nil
	}
	info := decl.info()
	var texts []string
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			if format := g.formatArg(info, call); format != nil {
				// The format is read only up to its verb, and never whole:
				// a prefix running past the verb is text no caller sees.
				if s, ok := constString(info, format); ok {
					texts = append(texts, beforeVerb(s))
				}
				for _, arg := range call.Args[1:] {
					ast.Inspect(arg, visit)
				}
				return false
			}
		}
		if expr, isExpr := n.(ast.Expr); isExpr {
			if s, ok := constString(info, expr); ok {
				texts = append(texts, s)
			}
		}
		return true
	}
	ast.Inspect(body, visit)
	return texts
}

// beginsSome reports whether one of texts begins with prefix.
func beginsSome(texts []string, prefix string) bool {
	return slices.ContainsFunc(texts, func(t string) bool { return strings.HasPrefix(t, prefix) })
}

// checkRefusals is G8: every refusal's stable text still begins a string the
// code it names folds, and the literal that carries it still has the status,
// the code and the headers the row declares. It is what makes a reworded
// refusal, or a changed status, code or header (CON-003), fail until the row
// changes with it.
func (g *gate) checkRefusals() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		for i, r := range d.Refusals {
			subject := fmt.Sprintf("%s refusal %d (%s %s)", d.ID, i+1, strings.Join(r.Methods, ","), r.Channel)
			found = append(found, g.refusalFindings(subject, r)...)
		}
	}
	return found
}

// refusalFindings judges one refusal.
func (g *gate) refusalFindings(subject string, r tenancy.Refusal) []Finding {
	at, err := g.p.lookup(r.At)
	if err != nil {
		return nil
	}
	g.read.refusals++
	fail := func(where, format string, args ...any) []Finding {
		return []Finding{{Rule: "G8", Subject: subject, Position: where, Message: fmt.Sprintf(format, args...)}}
	}
	if r.Prefix != "" && !beginsSome(g.foldedTexts(at), r.Prefix) {
		return fail(at.where(g.p), "no string %s folds begins with %q", keyOf(r.At), r.Prefix)
	}
	holder := at
	if r.Via != (tenancy.Site{}) {
		if holder, err = g.p.lookup(r.Via); err != nil {
			return nil
		}
	}
	var msg string
	switch r.Channel {
	case tenancy.Gate:
		msg = g.gateProblem(r, at, holder)
	case tenancy.RPC:
		msg = g.rpcProblem(r, holder)
	case tenancy.ToolError:
		msg = g.toolErrorProblem(holder, map[string]bool{})
	}
	if msg != "" {
		return fail(holder.where(g.p), "%s %s", holder.key, msg)
	}
	return nil
}

// gateProblem says how the gate literal carrying r differs from it, or
// returns empty when one carries it exactly.
//
// The literal is looked for in holder: the status must match, and the text
// must begin with the prefix where the literal holds the text itself. Where
// several literals qualify, one that carries the refusal exactly is enough;
// which return each of them is, is G7's question.
func (g *gate) gateProblem(r tenancy.Refusal, at, holder *declaration) string {
	info := holder.info()
	var candidates []refusalLit
	for _, rl := range g.refusalLiterals(holder, g.rules.gateType) {
		status, _ := rl.intField(info, rl.typ.status)
		if status != r.Status {
			continue
		}
		if holder == at && r.Prefix != "" && !strings.HasPrefix(g.leadingText(info, rl.fields[rl.typ.message]), r.Prefix) {
			continue
		}
		candidates = append(candidates, rl)
	}
	if holder != at {
		// The text is built in At and handed to this literal: prefer the
		// literal whose message is At's, where one is.
		if narrowed := g.messageFrom(candidates, info, at); len(narrowed) > 0 {
			candidates = narrowed
		}
	}
	if len(candidates) == 0 {
		return fmt.Sprintf("holds no gate refusal with status %d whose text begins %q", r.Status, r.Prefix)
	}
	var first string
	for _, rl := range candidates {
		problems := g.gateLiteralProblems(r, rl, holder)
		if len(problems) == 0 {
			return ""
		}
		if first == "" {
			first = strings.Join(problems, "; ")
		}
	}
	return first
}

// messageFrom keeps the literals whose message refers to at's declaration.
func (g *gate) messageFrom(lits []refusalLit, info *types.Info, at *declaration) []refusalLit {
	var out []refusalLit
	for _, rl := range lits {
		msg, ok := rl.fields[rl.typ.message]
		if !ok {
			continue
		}
		refers := false
		ast.Inspect(msg, func(n ast.Node) bool {
			if id, isIdent := n.(*ast.Ident); isIdent && objectKey(info.Uses[id]) == at.key {
				refers = true
			}
			return !refers
		})
		if refers {
			out = append(out, rl)
		}
	}
	return out
}

// gateLiteralProblems lists how one gate literal differs from the refusal.
func (g *gate) gateLiteralProblems(r tenancy.Refusal, rl refusalLit, holder *declaration) []string {
	info := holder.info()
	var problems []string
	if code, ok := rl.intField(info, rl.typ.code); !ok || code != r.Code {
		problems = append(problems, fmt.Sprintf("its code is %s, and the register says %d", describeInt(code, ok), r.Code))
	}
	headers, readable := g.headers(info, rl.fields[rl.typ.header])
	if !readable {
		return append(problems, "its headers are built where the gate cannot read them")
	}
	retry, hasRetry := headers[textproto.CanonicalMIMEHeaderKey(g.rules.headerRetryAfter)]
	switch {
	case hasRetry != (r.RetryAfter != tenancy.RetryAfterNone):
		problems = append(problems, fmt.Sprintf("it carries Retry-After %t, and the register says %t", hasRetry, r.RetryAfter != tenancy.RetryAfterNone))
	case hasRetry:
		read := g.symbolsRead(holder, retry)
		for _, want := range g.rules.retryAfterReads[r.RetryAfter] {
			if !read[want] {
				problems = append(problems, "its Retry-After does not read "+want)
			}
		}
	}
	if _, hasChallenge := headers[textproto.CanonicalMIMEHeaderKey(g.rules.headerChallenge)]; hasChallenge != r.Challenge {
		problems = append(problems, fmt.Sprintf("it carries WWW-Authenticate %t, and the register says %t", hasChallenge, r.Challenge))
	}
	return problems
}

// describeInt renders a folded integer, or says it did not fold.
func describeInt(v int, ok bool) string {
	if !ok {
		return "not a constant"
	}
	return strconv.Itoa(v)
}

// headers reads the header expression of a gate literal: a call whose
// arguments alternate a name that folds and the value it is given. A literal
// with no header has none, and anything else cannot be read.
func (g *gate) headers(info *types.Info, expr ast.Expr) (map[string]ast.Expr, bool) {
	out := map[string]ast.Expr{}
	if expr == nil {
		return out, true
	}
	call, isCall := ast.Unparen(expr).(*ast.CallExpr)
	if !isCall {
		return nil, info.Types[expr].IsNil()
	}
	for i := 0; i+1 < len(call.Args); i += 2 {
		name, ok := constString(info, call.Args[i])
		if !ok {
			return nil, false
		}
		out[textproto.CanonicalMIMEHeaderKey(name)] = call.Args[i+1]
	}
	return out, true
}

// symbolsRead are the names of every object an expression refers to, through
// the local variables it reads: each assignment to such a variable inside the
// declaration is read too, so `delay` resolves to what was assigned to it.
func (g *gate) symbolsRead(decl *declaration, expr ast.Expr) map[string]bool {
	info := decl.info()
	names := map[string]bool{}
	visited := map[types.Object]bool{}
	var walk func(ast.Node)
	walk = func(node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			id, isIdent := n.(*ast.Ident)
			if !isIdent {
				return true
			}
			obj := info.Uses[id]
			if obj == nil {
				return true
			}
			names[obj.Name()] = true
			if v, isVar := obj.(*types.Var); isVar && isLocal(v) && !visited[v] {
				visited[v] = true
				for _, rhs := range assignedTo(decl, v) {
					walk(rhs)
				}
			}
			return true
		})
	}
	walk(expr)
	return names
}

// isLocal reports whether a variable is declared inside a function: not a
// field, and not in its package's own scope.
func isLocal(v *types.Var) bool {
	return !v.IsField() && v.Parent() != v.Pkg().Scope()
}

// assignedTo are the expressions a local variable is given inside the
// declaration: its initializer and every plain assignment to it.
func assignedTo(decl *declaration, v *types.Var) []ast.Expr {
	info := decl.info()
	var out []ast.Expr
	target := func(lhs ast.Expr) bool {
		id, isIdent := ast.Unparen(lhs).(*ast.Ident)
		return isIdent && (info.Defs[id] == v || info.Uses[id] == v)
	}
	ast.Inspect(decl.body(), func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				if target(lhs) {
					out = append(out, s.Rhs[min(i, len(s.Rhs)-1)])
				}
			}
		case *ast.ValueSpec:
			for i, name := range s.Names {
				if target(name) && len(s.Values) > 0 {
					out = append(out, s.Values[min(i, len(s.Values)-1)])
				}
			}
		}
		return true
	})
	return out
}

// rpcProblem says why no JSON-RPC literal in holder carries r's code, or
// returns empty when one does. A code held in a variable counts as every
// constant the function assigns to it.
func (g *gate) rpcProblem(r tenancy.Refusal, holder *declaration) string {
	info := holder.info()
	lits := slices.DeleteFunc(g.refusalLiterals(holder, ""), func(rl refusalLit) bool { return rl.typ.name == g.rules.gateType })
	if len(lits) == 0 {
		return "builds no JSON-RPC error"
	}
	var seen []int
	for _, rl := range lits {
		expr, ok := rl.fields[rl.typ.code]
		if !ok {
			continue
		}
		if code, folds := foldInt(info, expr); folds {
			seen = append(seen, code)
			continue
		}
		if v, isVar := referenced(info, expr).(*types.Var); isVar && isLocal(v) {
			for _, rhs := range assignedTo(holder, v) {
				if code, folds := foldInt(info, rhs); folds {
					seen = append(seen, code)
				}
			}
		}
	}
	if slices.Contains(seen, r.Code) {
		return ""
	}
	return fmt.Sprintf("builds no JSON-RPC error carrying code %d (it carries %v)", r.Code, seen)
}

// toolErrorProblem says why a tool-error refusal's result is not flagged as
// an error, or returns empty when every result holder returns is.
//
// It reads a holder that returns a tool result, following the calls it
// returns into other functions of the program; a holder that returns no tool
// result (a message constant, an error) has no result literal to read.
func (g *gate) toolErrorProblem(holder *declaration, visited map[string]bool) string {
	if holder.fn == nil || visited[holder.key] {
		return ""
	}
	visited[holder.key] = true
	index := resultIndex(holder.function(), g.rules.toolResult)
	if index < 0 {
		return ""
	}
	info := holder.info()
	var problems []string
	forEachReturn(holder.fn, func(ret *ast.ReturnStmt) {
		if len(ret.Results) <= index {
			return
		}
		expr := ast.Unparen(ret.Results[index])
		if unary, isUnary := expr.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
			expr = ast.Unparen(unary.X)
		}
		switch e := expr.(type) {
		case *ast.CompositeLit:
			if !g.setsIsError(info, e) {
				problems = append(problems, fmt.Sprintf("returns a tool result at %s without %s set", g.p.position(e.Pos()), g.rules.isErrorField))
			}
		case *ast.CallExpr:
			var decl *declaration
			if callee := calleeOf(info, e); callee != nil {
				decl = g.p.decls[objectKey(callee)]
			}
			if decl == nil {
				problems = append(problems, fmt.Sprintf("returns a tool result from %s, which the gate cannot read", types.ExprString(e.Fun)))
				return
			}
			if msg := g.toolErrorProblem(decl, visited); msg != "" {
				problems = append(problems, msg)
			}
		default:
			if !info.Types[expr].IsNil() {
				problems = append(problems, fmt.Sprintf("returns %s, which the gate cannot read as a tool result", types.ExprString(expr)))
			}
		}
	})
	return strings.Join(problems, "; ")
}

// setsIsError reports whether a tool result literal sets its error flag to a
// constant true.
func (g *gate) setsIsError(info *types.Info, lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		// A struct literal's keys are its field names, and the flag is a bool.
		if types.ExprString(kv.Key) == g.rules.isErrorField {
			value := info.Types[kv.Value].Value
			return value != nil && constant.BoolVal(value)
		}
	}
	return false
}

// resultIndex is the index of the first result of a function whose type is the
// named type or a pointer to it, or -1.
func resultIndex(fn *types.Func, name string) int {
	results := fn.Signature().Results()
	for i := range results.Len() {
		if typeName(results.At(i).Type()) == name {
			return i
		}
	}
	return -1
}

// forEachReturn calls visit on every return statement of a function,
// leaving out the returns of the function literals inside it.
func forEachReturn(fn *ast.FuncDecl, visit func(*ast.ReturnStmt)) {
	ast.Inspect(fn, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			visit(s)
		}
		return true
	})
}
