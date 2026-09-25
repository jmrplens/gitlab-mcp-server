package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkCharges is G7: the authentication failure table (INV-007) against the
// code, per refusal return.
//
// In each function the table names, every return of a refusal is matched to
// one row by its status and, where its text folds, by the row's prefix, and it
// is charged exactly when a call of the function's charge helper precedes it
// in the same block. A return whose charged state differs from its row's, a
// return no row matches, a row no return matches, and a charge that no return
// follows in its own block all fail. So a charge moved from one branch to its
// sibling fails twice: the branch it left is charged in the table and not in
// the code, and the branch it reached the reverse.
//
// The plumbing half holds the lower charges, the calls that actually spend a
// budget, to the charge helpers: a charge made through anything else would not
// be seen by the per-return half at all.
func (g *gate) checkCharges() []Finding {
	var order []string
	byAt := map[string][]tenancy.Failure{}
	for _, f := range g.reg.failures {
		key := keyOf(f.At)
		if _, seen := byAt[key]; !seen {
			order = append(order, key)
		}
		byAt[key] = append(byAt[key], f)
	}
	var found []Finding
	for _, key := range order {
		found = append(found, g.chargeFindings(byAt[key], byAt)...)
	}
	return append(found, g.plumbingFindings()...)
}

// refusalReturn is one return of a refusal, as G7 reads it.
type refusalReturn struct {
	pos     token.Pos
	status  int
	text    string
	charged bool
}

// chargeWalk reads one function's refusal returns and charge calls.
type chargeWalk struct {
	g      *gate
	decl   *declaration
	helper string
	// delegates are the functions the table names, a return of whose answer
	// is not a refusal of its own.
	delegates map[string][]tenancy.Failure
	index     int
	returns   []refusalReturn
	charges   []*ast.CallExpr
	placed    map[*ast.CallExpr]bool
	problems  []Finding
}

// chargeFindings judges one function of the table against its rows.
func (g *gate) chargeFindings(rows []tenancy.Failure, delegates map[string][]tenancy.Failure) []Finding {
	at := rows[0].At
	subject := keyOf(at)
	decl, err := g.p.lookup(at)
	if err != nil {
		return nil
	}
	if decl.fn == nil {
		return []Finding{{Rule: "G7", Subject: subject, Position: decl.where(g.p), Message: "is not a function, so it returns no refusal"}}
	}
	helper := g.p.decls[siteKey(at.Pkg, at.Call)]
	if helper == nil || helper.fn == nil {
		return []Finding{{Rule: "G7", Subject: subject, Message: fmt.Sprintf("names the charge helper %s, which is not a function of %s", at.Call, at.Pkg)}}
	}
	index := resultIndex(decl.function(), g.rules.gateType)
	if index < 0 {
		return []Finding{{Rule: "G7", Subject: subject, Position: decl.where(g.p), Message: "returns no gate refusal"}}
	}
	w := &chargeWalk{g: g, decl: decl, helper: helper.key, delegates: delegates, index: index, placed: map[*ast.CallExpr]bool{}}
	ast.Inspect(decl.fn, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall && w.isCharge(call) {
			w.charges = append(w.charges, call)
		}
		return true
	})
	w.block(decl.fn.Body.List)
	found := w.problems
	for _, call := range w.charges {
		if !w.placed[call] {
			found = append(found, Finding{
				Rule: "G7", Subject: subject, Position: g.p.position(call.Pos()),
				Message: "charges a failure in a block no refusal return follows, so no row of the table can say it is charged",
			})
		}
	}
	return append(found, g.matchReturns(subject, rows, w.returns)...)
}

// isCharge reports whether a call is the function's charge helper.
func (w *chargeWalk) isCharge(call *ast.CallExpr) bool {
	fn := calleeOf(w.decl.info(), call)
	return fn != nil && objectKey(fn) == w.helper
}

// block walks one statement list: a charge call at this level marks every
// refusal returned later in the same list as charged.
func (w *chargeWalk) block(stmts []ast.Stmt) {
	var charges []*ast.CallExpr
	for _, st := range stmts {
		if expr, isExpr := st.(*ast.ExprStmt); isExpr {
			if call, isCall := ast.Unparen(expr.X).(*ast.CallExpr); isCall && w.isCharge(call) {
				charges = append(charges, call)
				continue
			}
		}
		if ret, isReturn := st.(*ast.ReturnStmt); isReturn {
			if w.refusal(ret, len(charges) > 0) {
				for _, call := range charges {
					w.placed[call] = true
				}
			}
			continue
		}
		w.nested(st)
	}
}

// nested walks the blocks inside one statement. The body of a function
// literal is another function's, and is not read.
func (w *chargeWalk) nested(st ast.Stmt) {
	switch s := st.(type) {
	case *ast.BlockStmt:
		w.block(s.List)
	case *ast.IfStmt:
		w.block(s.Body.List)
		if s.Else != nil {
			w.nested(s.Else)
		}
	case *ast.ForStmt:
		w.block(s.Body.List)
	case *ast.RangeStmt:
		w.block(s.Body.List)
	case *ast.SwitchStmt:
		w.clauses(s.Body)
	case *ast.TypeSwitchStmt:
		w.clauses(s.Body)
	case *ast.SelectStmt:
		w.clauses(s.Body)
	case *ast.LabeledStmt:
		w.nested(s.Stmt)
	}
}

// clauses walks the case bodies of a switch or a select: a switch's are case
// clauses, and a select's are communication clauses.
func (w *chargeWalk) clauses(body *ast.BlockStmt) {
	for _, clause := range body.List {
		if cc, isCase := clause.(*ast.CaseClause); isCase {
			w.block(cc.Body)
			continue
		}
		comm, _ := clause.(*ast.CommClause)
		w.block(comm.Body)
	}
}

// refusal records a return if it returns a refusal of its own, and reports
// whether it did.
func (w *chargeWalk) refusal(ret *ast.ReturnStmt, charged bool) bool {
	info := w.decl.info()
	results := w.decl.function().Signature().Results()
	if len(ret.Results) != results.Len() {
		w.unreadable(ret, "returns what the gate cannot split into its results")
		return false
	}
	expr := ast.Unparen(ret.Results[w.index])
	if info.Types[expr].IsNil() {
		return false
	}
	if unary, isUnary := expr.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
		expr = ast.Unparen(unary.X)
	}
	switch e := expr.(type) {
	case *ast.CompositeLit:
		// The result is typed as the gate's failure, so a literal there is
		// one, and read as one wherever the rules list its type.
		if rl, ok := w.g.refusalLiteral(info, e); ok {
			status, _ := rl.intField(info, rl.typ.status)
			w.returns = append(w.returns, refusalReturn{pos: ret.Pos(), status: status, text: w.g.leadingText(info, rl.fields[rl.typ.message]), charged: charged})
			return true
		}
	case *ast.CallExpr:
		return w.constructed(ret, e, charged)
	}
	w.unreadable(ret, "returns a refusal the gate cannot read")
	return false
}

// constructed records a return of a refusal built by a call: a constructor of
// this program, whose own literal carries the status, or a function the table
// names, whose answer is not a refusal of its own.
func (w *chargeWalk) constructed(ret *ast.ReturnStmt, call *ast.CallExpr, charged bool) bool {
	info := w.decl.info()
	callee := calleeOf(info, call)
	var ctor *declaration
	if callee != nil {
		key := objectKey(callee)
		if _, delegated := w.delegates[key]; delegated {
			return false
		}
		ctor = w.g.p.decls[key]
	}
	if ctor == nil {
		w.unreadable(ret, "returns a refusal from a call the gate cannot follow")
		return false
	}
	lits := w.g.refusalLiterals(ctor, w.g.rules.gateType)
	if len(lits) != 1 {
		w.unreadable(ret, fmt.Sprintf("returns a refusal from %s, which builds %d gate refusals rather than one", ctor.key, len(lits)))
		return false
	}
	ctorInfo := ctor.info()
	status, _ := lits[0].intField(ctorInfo, lits[0].typ.status)
	text := ""
	for _, arg := range call.Args {
		if s, ok := constString(info, arg); ok {
			text = s
			break
		}
	}
	if text == "" {
		text = w.g.leadingText(ctorInfo, lits[0].fields[lits[0].typ.message])
	}
	w.returns = append(w.returns, refusalReturn{pos: ret.Pos(), status: status, text: text, charged: charged})
	return true
}

// unreadable records a return G7 could not read, which fails rather than
// passing silently.
func (w *chargeWalk) unreadable(ret *ast.ReturnStmt, why string) {
	w.problems = append(w.problems, Finding{Rule: "G7", Subject: w.decl.key, Position: w.g.p.position(ret.Pos()), Message: why})
}

// matchReturns pairs each return with a row, in source order, and reports
// every disagreement: a charged state that differs, a return no row matches,
// and a row no return matches.
func (g *gate) matchReturns(subject string, rows []tenancy.Failure, returns []refusalReturn) []Finding {
	var found []Finding
	matched := make([]bool, len(rows))
	for _, ret := range returns {
		best, bestScore := -1, 0
		for i, row := range rows {
			if score := matchScore(row, ret); !matched[i] && score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			found = append(found, Finding{
				Rule: "G7", Subject: subject, Position: g.p.position(ret.pos),
				Message: fmt.Sprintf("returns a %d refusal beginning %q that no row of the failure table matches", ret.status, ret.text),
			})
			continue
		}
		matched[best] = true
		g.read.returns++
		if row := rows[best]; row.Charged != ret.charged {
			found = append(found, Finding{
				Rule: "G7", Subject: subject + " " + row.Kind, Position: g.p.position(ret.pos),
				Message: fmt.Sprintf("is %s here, and the failure table says %s", chargedWord(ret.charged), chargedWord(row.Charged)),
			})
		}
	}
	for i, row := range rows {
		if !matched[i] {
			found = append(found, Finding{
				Rule: "G7", Subject: subject + " " + row.Kind,
				Message: fmt.Sprintf("is a %d refusal beginning %q in the failure table that no return of %s matches", row.Status, row.Prefix, subject),
			})
		}
	}
	return found
}

// matchScore is how well a row matches a return: zero when it does not, one
// for a row with no prefix, and two for a row whose prefix begins the
// return's text, so a return prefers the row that names it.
func matchScore(row tenancy.Failure, ret refusalReturn) int {
	switch {
	case row.Status != ret.status:
		return 0
	case row.Prefix == "":
		return 1
	case strings.HasPrefix(ret.text, row.Prefix):
		return 2
	default:
		return 0
	}
}

// chargedWord renders a charged state.
func chargedWord(charged bool) string {
	if charged {
		return "charged"
	}
	return "uncharged"
}

// plumbingFindings holds the lower charges to the declared callers.
func (g *gate) plumbingFindings() []Finding {
	var found []Finding
	g.p.forEachCall(func(key string, info *types.Info, call *ast.CallExpr) {
		callee := calleeOf(info, call)
		if callee != nil && slices.Contains(g.rules.lowerCharges, calleeName(callee)) && !slices.Contains(g.rules.chargeCallers, key) {
			found = append(found, Finding{
				Rule: "G7", Subject: key, Position: g.p.position(call.Pos()),
				Message: fmt.Sprintf("spends an authentication budget through %s outside the declared charge helpers", calleeName(callee)),
			})
		}
	})
	return found
}
