package derive

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
)

// Class is how a request of an action is needed.
type Class string

// The classes.
const (
	// ClassMandatory is a request every path makes.
	ClassMandatory Class = "mandatory"
	// ClassAlternative is a request some paths make and others do not.
	ClassAlternative Class = "alternative"
	// ClassOptional is a request no path needs.
	ClassOptional Class = "optional"
)

// Declaration answers, for one action, what the walk cannot read: an action
// that sends nothing, or the requests behind one unresolved request.
type Declaration struct {
	Action   string
	Category string
	Reason   string
	// Replaces is the [Request.Reason] of the unresolved request the
	// declaration answers, "" for an action declared to send nothing.
	Replaces string
	// Requests are what the unresolved request stands for: all of them made,
	// or one of them when Any is set.
	Requests []Request
	Any      bool
}

// Use is one request of one action, with where it came from and what shaped
// it.
type Use struct {
	Request
	Class Class
	// Sites are the functions whose bodies name it, by package and name.
	Sites []string
	// SDKMethods are the client-go methods that send it.
	SDKMethods []string
	// Directives are the directives it went through.
	Directives []string
	// Declaration is the category of the declaration that supplied it.
	Declaration string
	// Qualified is set when a mandatory or alternatives directive, or a
	// declaration, shaped it: an author said how it combines.
	Qualified bool
	// DeclaredOptional is set when an optional directive made it optional.
	DeclaredOptional bool
}

// Action is what one catalog action can send.
type Action struct {
	ID string
	// Handlers are the functions its route runs.
	Handlers []string
	// Uses are its requests, in the order the walk first met them, which is
	// the order the handler makes them along any one path.
	Uses []Use
	// Paths are the ways it runs, each a set of indices into Uses, minimal and
	// sorted.
	Paths [][]int
	// Declaration is the category of the declaration that decided it whole,
	// for an action declared to send nothing.
	Declaration string
	// Findings are what keeps the derivation of this action from being read
	// as complete.
	Findings []string
}

// Result is the derivation of every action.
type Result struct {
	Actions []Action
	// Findings are about the derivation as a whole: a malformed or stale
	// directive, a declaration that answers nothing.
	Findings []string
}

// deriver holds what every action's derivation shares.
type deriver struct {
	prog       *actionrequests.Program
	sdk        SDK
	directives *directives
	bodies     map[*types.Func]*node
}

// Derive derives every catalog action's requests.
func Derive(prog *actionrequests.Program, actions []actionrequests.Action, sdk SDK, declarations []Declaration) Result {
	d := &deriver{
		prog:       prog,
		sdk:        sdk,
		directives: collectDirectives(prog.Position, prog.Packages()),
		bodies:     map[*types.Func]*node{},
	}
	byAction := map[string][]*declared{}
	var all []*declared
	for i := range declarations {
		entry := &declared{Declaration: declarations[i]}
		byAction[entry.Action] = append(byAction[entry.Action], entry)
		all = append(all, entry)
	}
	sites := prog.Sites()
	result := Result{}
	for _, act := range actions {
		result.Actions = append(result.Actions, d.action(act, sites, byAction[act.ID]))
	}
	result.Findings = append(result.Findings, d.directives.malformed...)
	result.Findings = append(result.Findings, d.directives.unused()...)
	for _, entry := range all {
		if !entry.used {
			result.Findings = append(result.Findings, fmt.Sprintf(
				"%s: the %s declaration answers nothing the walk reaches (%s); remove it, or say what it answers now",
				entry.Action, entry.Category, describeReplaces(entry.Replaces),
			))
		}
	}
	sort.Strings(result.Findings)
	return result
}

// describeReplaces names what a declaration answers, for a finding.
func describeReplaces(replaces string) string {
	if replaces == "" {
		return "the action is declared to send nothing and the walk found a request"
	}
	return "no request reads " + replaces
}

// declared is a declaration with whether some action's walk used it.
type declared struct {
	Declaration
	used bool
}

// action derives one action.
func (d *deriver) action(act actionrequests.Action, sites map[string][]actionrequests.Site, declarations []*declared) Action {
	out := Action{ID: act.ID}
	matched := actionrequests.Match(sites, act)
	if len(matched) != 1 {
		out.Findings = append(out.Findings, fmt.Sprintf("%s meets %d ActionSpec constructions, so its handler cannot be read", act.ID, len(matched)))
		return out
	}
	roots := d.prog.Roots(matched)
	if len(roots) == 0 {
		out.Findings = append(out.Findings, act.ID+": its route resolves to no handler")
		return out
	}
	walk := &expansion{d: d, action: &out, index: map[string]int{}, stack: map[*types.Func]bool{}, declarations: declarations}
	walk.sends = d.sendsGraphQL(roots)
	paths := unit()
	for _, root := range roots {
		out.Handlers = append(out.Handlers, symbol(root))
		next, ok := product(paths, walk.call(root, nil, nil, context{}))
		if !ok {
			walk.overflow = true
		}
		paths = next
	}
	if walk.overflow {
		out.Findings = append(out.Findings, fmt.Sprintf("%s expands into more than %d paths; declare how its requests combine", act.ID, maxPaths))
	}
	d.sendsNothing(&out, declarations)
	out.Paths = paths
	finish(&out)
	return out
}

// sendsNothing applies a declaration that the action sends nothing, which
// holds only while the walk finds nothing it sends.
func (d *deriver) sendsNothing(out *Action, declarations []*declared) {
	for _, entry := range declarations {
		if entry.Replaces != "" {
			continue
		}
		if len(out.Uses) == 0 {
			entry.used = true
			out.Declaration = entry.Category
		}
	}
}

// sendsGraphQL reports whether anything the handlers reach puts a document on
// the GraphQL transport, which is what makes a document a body names a
// request rather than a string.
func (d *deriver) sendsGraphQL(roots []*types.Func) bool {
	for fn := range d.prog.Reachable(roots) {
		if body, ok := d.prog.Function(fn); ok && body.SendsGraphQL {
			return true
		}
	}
	return false
}

// finish orders an action's paths and classifies each request by the paths
// that make it. The requests keep the order the walk first met them in, which
// is the order a handler makes them along any one path: a statement before
// the next, a callee's requests where it is called, and client-go's documents
// in the order it posts them. What a caller meets first on a path is decided
// by that order (a lookup that a fine-grained token cannot pass stops the
// write after it from ever being sent), so a sort by anything else would
// throw away the one fact the join needs to say which refusal comes first.
func finish(out *Action) {
	paths := make([][]int, 0, len(out.Paths))
	for _, path := range out.Paths {
		sorted := slices.Clone(path)
		slices.Sort(sorted)
		paths = append(paths, sorted)
	}
	out.Paths = minimize(paths)
	for i := range out.Uses {
		out.Uses[i].Class = classify(i, out.Paths)
		slices.Sort(out.Uses[i].Sites)
		out.Uses[i].Sites = slices.Compact(out.Uses[i].Sites)
		slices.Sort(out.Uses[i].SDKMethods)
		out.Uses[i].SDKMethods = slices.Compact(out.Uses[i].SDKMethods)
		slices.Sort(out.Uses[i].Directives)
		out.Uses[i].Directives = slices.Compact(out.Uses[i].Directives)
	}
}

// classify reads how a request is needed from the paths that make it.
func classify(index int, paths [][]int) Class {
	in := 0
	for _, path := range paths {
		if slices.Contains(path, index) {
			in++
		}
	}
	if in == 0 {
		return ClassOptional
	}
	if in == len(paths) {
		return ClassMandatory
	}
	return ClassAlternative
}

// symbol names a function by its package and name, with its receiver's type
// for a method, which is how the record names where a request is made.
func symbol(fn *types.Func) string {
	name := fn.Name()
	if recv := fn.Signature().Recv(); recv != nil {
		typ := recv.Type()
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = pointer.Elem()
		}
		if named, ok := typ.(*types.Named); ok {
			name = named.Obj().Name() + "." + name
		}
	}
	if fn.Pkg() == nil {
		return name
	}
	return fn.Pkg().Name() + "." + name
}

// body returns a function's structure, read once.
func (d *deriver) body(fn *types.Func) *node {
	if built, ok := d.bodies[fn]; ok {
		return built
	}
	indexed, _ := d.prog.Function(fn)
	b := &builder{d: d, site: fn, fn: indexed, pkg: indexed.Package()}
	// A body is a declared function's or a literal's block, or the expression
	// a package-level variable is initialized with; the index holds no other.
	var built *node
	if block, isBlock := indexed.Root().(*ast.BlockStmt); isBlock {
		built = b.block(block.List)
	} else {
		initializer, _ := indexed.Root().(ast.Expr)
		built = b.expr(initializer)
	}
	d.bodies[fn] = built
	return built
}

// forStatement returns the directive qualifying a statement, if one does.
func (d *deriver) forStatement(stmt ast.Stmt) *Directive {
	return d.directives.at(d.prog.Position(stmt.Pos()))
}

// requestSeam reports whether a package-level variable's initializer is a
// method expression of client-go's request constructor, and returns the
// constructor: a body calling the variable builds a raw request with the
// client as its first argument.
func (d *deriver) requestSeam(stand *types.Func) (*types.Func, bool) {
	indexed, _ := d.prog.Function(stand)
	selector, ok := indexed.Root().(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	selection, ok := indexed.Package().TypesInfo.Selections[selector]
	if !ok || selection.Kind() != types.MethodExpr {
		return nil, false
	}
	// A method expression selects a method, which is always a function.
	fn, _ := selection.Obj().(*types.Func)
	if !isRequestConstructor(fn) {
		return nil, false
	}
	return fn, true
}

// siteName names the function a leaf is written in.
func siteName(fn *types.Func) string {
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(symbol(fn))
}
