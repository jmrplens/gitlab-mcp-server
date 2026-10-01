package sdkroutes

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const (
	// placeholder is what an identifier segment collapses to, client-go's own
	// spelling.
	placeholder = ":"
	// unknown marks a piece of a path the reading cannot name while the path is
	// still being assembled, so a segment made of nothing else becomes the
	// placeholder and one that holds it beside literal text keeps the text.
	unknown = "\x00"
	// fmtPackage and sprintfFunc spell the one formatting call a legacy path is
	// built with.
	fmtPackage  = "fmt"
	sprintfFunc = "Sprintf"
	// maxFolds bounds how many spellings one expression may fold to, so a
	// concatenation of several reassigned variables cannot multiply without
	// end. No client-go path comes near it.
	maxFolds = 16
	// verbFlags are the characters a format verb may carry between its percent
	// sign and its letter.
	verbFlags = "+-# 0123456789."
)

// resolve evaluates one entry: every request the method and the functions it
// delegates to can send.
func (r *reading) resolve(entry *function) Method {
	method := Method{
		Service: strings.TrimSuffix(entry.recvType, serviceSuffix),
		Name:    entry.name,
		Answers: entry.answers,
		Many:    entry.many,
		Options: entry.options,
	}
	r.walk(entry, nil, map[string]bool{}, func(fn *function, env map[string]string) {
		method.GraphQL = method.GraphQL || fn.graphQL
		for _, use := range fn.templates {
			if path, literal := r.templatePath(fn, use, env); literal {
				method.Routes = append(method.Routes, Route{Method: verbOf(fn.verb), Path: path})
			}
		}
		for _, use := range fn.legacy {
			routes, unresolved := r.legacyRoutes(fn, use, env)
			method.Routes = append(method.Routes, routes...)
			method.Unresolved = append(method.Unresolved, unresolved...)
		}
	})
	method.Routes = sortedRoutes(method.Routes)
	method.Unresolved = sortedUnique(method.Unresolved)
	return method
}

// walk visits fn and every function it delegates to, each with the string
// constants its caller handed it, once per call chain so a recursive helper
// cannot loop.
func (r *reading) walk(fn *function, env map[string]string, onPath map[string]bool, visit func(*function, map[string]string)) {
	if onPath[fn.key] {
		return
	}
	onPath[fn.key] = true
	defer delete(onPath, fn.key)
	visit(fn, env)
	for _, call := range fn.calls {
		callee, ok := r.funcs[call.callee]
		if !ok {
			continue
		}
		r.walk(callee, r.bind(fn, callee, call.args, env, map[string]bool{}), onPath, visit)
	}
}

// bind maps a callee's parameters to the first spelling the caller's arguments
// fold to.
func (r *reading) bind(caller, callee *function, args []ast.Expr, env map[string]string, active map[string]bool) map[string]string {
	bound := map[string]string{}
	for i, param := range callee.params {
		if i >= len(args) {
			break
		}
		bound[param] = first(r.fold(caller, args[i], env, active))
	}
	return bound
}

// first is the spelling an argument is taken as when one value is wanted: the
// first that is not empty. A helper's failure branch returns "" beside its
// error (PathEscapeFileName does), and taking that as a path piece would drop
// a segment from every route built with it.
func first(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return values[0]
}

// verbOf is the verb a method sends, GET when it names none.
func verbOf(named string) string {
	if named == "" {
		return defaultVerb
	}
	return named
}

// templatePath formats one withPath template and shapes it.
func (r *reading) templatePath(fn *function, use templateUse, env map[string]string) (string, bool) {
	values := make([]string, 0, len(use.args))
	for _, arg := range use.args {
		values = append(values, first(r.fold(fn, arg, env, map[string]bool{})))
	}
	return shape(substitute(use.template, values))
}

// legacyRoutes reads one NewRequest or UploadRequest: its verb, reassigned or
// not, and every path its argument folds to.
//
// An empty path is the GraphQL transport's, which posts to an endpoint of its
// own, and is no REST route. A path that folds to no static segment at all
// cannot be told from any other route and is recorded as unresolved rather
// than guessed at.
func (r *reading) legacyRoutes(fn *function, use legacyUse, env map[string]string) (routes []Route, unresolved []string) {
	verb := fn.override
	if verb == "" {
		named, ok := httpVerb(use.verb)
		if !ok {
			return nil, []string{fn.key + ": the verb of a request it builds is not a net/http constant"}
		}
		verb = named
	}
	for _, raw := range r.fold(fn, use.path, env, map[string]bool{}) {
		if raw == "" {
			continue
		}
		path, literal := shape(raw)
		if !literal {
			unresolved = append(unresolved, fn.key+": the path of a request it builds folds to no static segment")
			continue
		}
		routes = append(routes, Route{Method: verb, Path: path})
	}
	return routes, unresolved
}

// fold evaluates a string expression to every spelling it can hold, with
// [unknown] standing for any piece the reading cannot name. It never returns
// an empty list.
//
// active holds the assignments and functions being folded on the current path,
// so a variable reassigned from itself (u = fmt.Sprintf("%s.%s", u, x)) reads
// the other assignments for its own name, and a helper that calls itself folds
// the call back to an unknown piece, instead of either recursing.
func (r *reading) fold(fn *function, expr ast.Expr, env map[string]string, active map[string]bool) []string {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		if literal, ok := stringLiteral(typed); ok {
			return []string{literal}
		}
	case *ast.ParenExpr:
		return r.fold(fn, typed.X, env, active)
	case *ast.Ident:
		return r.foldIdent(fn, typed.Name, env, active)
	case *ast.BinaryExpr:
		if typed.Op == token.ADD {
			return concat(r.fold(fn, typed.X, env, active), r.fold(fn, typed.Y, env, active))
		}
	case *ast.CallExpr:
		return r.foldCall(fn, typed, 0, env, active)
	}
	return []string{unknown}
}

// foldIdent evaluates a name: a parameter to what the caller handed it, a
// local variable to everything assigned to it, and a package constant to its
// value.
func (r *reading) foldIdent(fn *function, name string, env map[string]string, active map[string]bool) []string {
	if value, bound := env[name]; bound {
		return []string{value}
	}
	if slices.Contains(fn.params, name) {
		return []string{unknown}
	}
	if assigned := r.foldLocal(fn, name, env, active); len(assigned) > 0 {
		return assigned
	}
	if value, isConst := r.consts[name]; isConst {
		return []string{value}
	}
	return []string{unknown}
}

// foldLocal evaluates every value a local variable is assigned in fn's body,
// including one taken from a call that returns several. It returns nothing for
// a name the body never assigns, and leaves out an assignment already being
// folded on this path, which is the one a self-reference sits inside.
func (r *reading) foldLocal(fn *function, name string, env map[string]string, active map[string]bool) []string {
	var out []string
	ast.Inspect(fn.decl.Body, func(node ast.Node) bool {
		if _, isLiteral := node.(*ast.FuncLit); isLiteral {
			return false
		}
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, isIdent := lhs.(*ast.Ident)
			if !isIdent || ident.Name != name {
				continue
			}
			key := fn.key + "@" + strconv.Itoa(int(ident.Pos()))
			if active[key] {
				continue
			}
			active[key] = true
			out = append(out, r.foldAssigned(fn, assign, i, env, active)...)
			delete(active, key)
		}
		return true
	})
	return limit(out)
}

// foldAssigned evaluates what one assignment puts in its i-th target: the
// matching right-hand side, or the i-th result of the one call on the right.
func (r *reading) foldAssigned(fn *function, assign *ast.AssignStmt, i int, env map[string]string, active map[string]bool) []string {
	if len(assign.Rhs) == len(assign.Lhs) {
		return r.fold(fn, assign.Rhs[i], env, active)
	}
	if call, isCall := assign.Rhs[0].(*ast.CallExpr); isCall {
		return r.foldCall(fn, call, i, env, active)
	}
	return []string{unknown}
}

// foldCall evaluates result index of a call: fmt.Sprintf with its format
// substituted, a function of this package entered with its parameters bound,
// and anything else, PathEscape among them, as one unknown piece.
func (r *reading) foldCall(fn *function, call *ast.CallExpr, index int, env map[string]string, active map[string]bool) []string {
	if isSprintf(call) && len(call.Args) > 0 {
		var values []string
		for _, arg := range call.Args[1:] {
			values = append(values, first(r.fold(fn, arg, env, active)))
		}
		var out []string
		for _, format := range r.fold(fn, call.Args[0], env, active) {
			out = append(out, substitute(format, values))
		}
		return limit(out)
	}
	callee, ok := r.calleeOf(fn, call)
	if !ok || active[callee.key] {
		return []string{unknown}
	}
	inner := r.bind(fn, callee, call.Args, env, active)
	active[callee.key] = true
	defer delete(active, callee.key)
	var out []string
	ast.Inspect(callee.decl.Body, func(node ast.Node) bool {
		if _, isLiteral := node.(*ast.FuncLit); isLiteral {
			return false
		}
		if ret, isReturn := node.(*ast.ReturnStmt); isReturn && index < len(ret.Results) {
			out = append(out, r.fold(callee, ret.Results[index], inner, active)...)
		}
		return true
	})
	if len(out) == 0 {
		return []string{unknown}
	}
	return limit(out)
}

// calleeOf names the function of this package a call enters: a package
// function, or a method called on fn's own receiver.
func (r *reading) calleeOf(fn *function, call *ast.CallExpr) (*function, bool) {
	switch callee := unindex(call.Fun).(type) {
	case *ast.Ident:
		found, ok := r.funcs[callee.Name]
		return found, ok
	case *ast.SelectorExpr:
		if holder, isIdent := callee.X.(*ast.Ident); isIdent && fn.recvVar != "" && holder.Name == fn.recvVar {
			found, ok := r.funcs[fn.recvType+"."+callee.Sel.Name]
			return found, ok
		}
	}
	return nil, false
}

// isSprintf reports whether a call is fmt.Sprintf.
func isSprintf(call *ast.CallExpr) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != sprintfFunc {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	return isIdent && pkg.Name == fmtPackage
}

// concat joins every spelling of a left piece to every spelling of a right
// one, bounded.
func concat(left, right []string) []string {
	var out []string
	for _, l := range left {
		for _, rr := range right {
			out = append(out, l+rr)
		}
	}
	return limit(out)
}

// limit keeps at most [maxFolds] spellings, dropping repeats.
func limit(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] || len(out) == maxFolds {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// substitute replaces every format verb of a format string with the next
// value, [unknown] once the values run out, and unescapes a doubled percent.
func substitute(format string, values []string) string {
	var builder strings.Builder
	next := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			builder.WriteByte(format[i])
			continue
		}
		j := i + 1
		for j < len(format) && strings.IndexByte(verbFlags, format[j]) >= 0 {
			j++
		}
		if j < len(format) && format[j] == '%' {
			builder.WriteByte('%')
			i = j
			continue
		}
		value := unknown
		if next < len(values) {
			value = values[next]
		}
		next++
		builder.WriteString(value)
		i = j
	}
	return builder.String()
}

// shape reduces a formatted path to a route shape: a segment that is nothing
// but an unknown piece becomes the placeholder, and an unknown piece inside a
// longer segment is dropped so the literal part still names the route, which
// is client-go's own normalization of a template (archive%s is archive). The
// separator that joined a dropped piece to the text goes with it, so a format
// suffix written archive.%s names archive too rather than "archive.". It
// reports whether any segment is literal text, since a path made only of
// placeholders names no endpoint anybody could tell from another.
func shape(raw string) (string, bool) {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	segments := strings.Split(strings.Trim(raw, "/"), "/")
	literal := false
	for i, segment := range segments {
		if segment == unknown {
			segments[i] = placeholder
			continue
		}
		segments[i] = separatedUnknown().Replace(segment)
		literal = literal || segments[i] != ""
	}
	return "/" + strings.Join(segments, "/"), literal
}

// separatedUnknown drops an unknown piece, and the separator in front of it
// when there is one. It is built on first use rather than at package
// initialization, so the statement that builds it is one a test runs.
var separatedUnknown = sync.OnceValue(func() *strings.Replacer {
	return strings.NewReplacer("."+unknown, "", "-"+unknown, "", "_"+unknown, "", unknown, "")
})
