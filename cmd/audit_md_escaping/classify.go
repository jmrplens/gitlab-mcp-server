package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// verdict is what the classifier concluded about one value reaching a Markdown
// construct.
type verdict int

const (
	// safe means the value cannot carry a character that changes the
	// document's shape, either because of what it is or because it has already
	// been through an escaper.
	safe verdict = iota
	// unescaped means the value is text this server did not write and no
	// escaper stands between it and the page.
	unescaped
	// unresolved means the classifier could not follow the value to either
	// answer. It is reported separately: a gate that silently counted these as
	// safe would be a gate with a hole in it.
	unresolved
)

// escapers are the toolutil functions whose result is safe to interpolate.
// Each neutralizes a superset of what the weakest construct needs, so a value
// that has been through any of them is safe in any context.
//
// StripControlBytes is deliberately absent. It drops the C0 and C1 bytes and
// leaves both the pipe and the angle bracket, so accepting it as an answer
// would pass exactly the values this audit exists to find.
var escapers = map[string]bool{
	"EscapeMdTableCell":       true,
	"EscapeMdHeading":         true,
	"EscapeMdLinkLabel":       true,
	"EscapeMdLinkDestination": true,
	"MdTitleLink":             true,
	"FormatTarget":            true,
	"WrapGFMBody":             true,
}

// externalSafe names functions outside the audited packages whose result
// cannot carry GitLab-authored text. Their bodies are not loaded, so they are
// judged by what they are documented to return: a number's decimal form, a
// duration, a formatted instant.
var externalSafe = map[string]bool{
	"strconv.Itoa":         true,
	"strconv.FormatInt":    true,
	"strconv.FormatUint":   true,
	"strconv.FormatFloat":  true,
	"strconv.FormatBool":   true,
	"time.Time.Format":     true,
	"time.Time.String":     true,
	"time.Duration.String": true,
}

// passThroughArgs names functions outside the audited packages whose result is
// made of the arguments listed, so the question passes to those arguments
// instead of stopping at a body the audit never loaded.
//
// It is what lets the escaping helpers be recognized through the transforms
// wrapped around them: the prompts package renders a value as
// DefuseHintsHeading(EscapeMdTableCell(s)), and DefuseHintsHeading is a
// strings.ReplaceAll of its argument.
var passThroughArgs = map[string][]int{
	"strings.Join":       {0},
	"strings.Map":        {1},
	"strings.Repeat":     {0},
	"strings.Replace":    {0, 2},
	"strings.ReplaceAll": {0, 2},
	"strings.ToLower":    {0},
	"strings.ToTitle":    {0},
	"strings.ToUpper":    {0},
	"strings.Trim":       {0},
	"strings.TrimLeft":   {0},
	"strings.TrimPrefix": {0},
	"strings.TrimRight":  {0},
	"strings.TrimSpace":  {0},
	"strings.TrimSuffix": {0},
}

// classifier answers whether one expression can carry text that changes the
// shape of the Markdown around it.
type classifier struct {
	prog *program
	// assigns maps a local variable to every expression assigned to it, so a
	// value escaped into a variable and then written is recognized.
	assigns map[*types.Var][]ast.Expr
	// pkgOf maps an assigned expression to the package it was written in, so
	// it is resolved against its own type information.
	pkgOf map[ast.Expr]*packages.Package
	// params maps a parameter to the function that declares it and its
	// position in the signature, which is how a parameter with no binding is
	// resolved to what every caller passes.
	params map[*types.Var]paramRef
	// fieldWritten records the variables a field was assigned on. Such a
	// variable no longer answers for its fields through the literal that
	// built it: a struct built empty and filled afterwards would otherwise
	// read as one that left every field at its zero value, which is the one
	// way this audit could call a raw value safe.
	fieldWritten map[*types.Var]bool
	// returnMemo caches a declared function's verdict, for the calls whose
	// answer does not depend on what the caller passed.
	returnMemo map[resultKey]verdict
	// inProgress guards the recursion against a function that reaches itself.
	inProgress map[resultKey]bool
	// inVar guards it against a variable assigned from itself.
	inVar map[*types.Var]bool
	// bindHits counts how often a parameter was answered from a caller's
	// argument. A result reached through one is not cached, because it holds
	// for that call site rather than for the function.
	bindHits int
}

// paramRef is one parameter: the function that declares it and its index in
// the flattened parameter list.
type paramRef struct {
	fn    *types.Func
	index int
}

// resultKey names one result of one declared function.
type resultKey struct {
	fn    *types.Func
	index int
}

// env binds a called function's parameters to the expressions its caller
// passed, together with the environment those expressions are meaningful in.
//
// It is what makes a finding belong to a call site. toolutil.FormatTime
// returns its argument verbatim when neither layout parses, and it is called
// from about a hundred and fifty places: without a binding, the one caller
// that passes an unescaped field would condemn every other caller too, and the
// report would name the wrong lines.
type env struct {
	binds map[*types.Var]bound
}

// bound is one argument a caller passed, kept with everything needed to judge
// it where it was written.
type bound struct {
	expr ast.Expr
	pkg  *packages.Package
	env  *env
}

// lookup returns the argument bound to v, if this environment binds it.
func (e *env) lookup(v *types.Var) (bound, bool) {
	if e == nil || e.binds == nil {
		return bound{}, false
	}
	b, ok := e.binds[v]
	return b, ok
}

// newClassifier indexes every local assignment and parameter in the loaded
// packages.
func newClassifier(prog *program) *classifier {
	c := &classifier{
		prog:         prog,
		assigns:      make(map[*types.Var][]ast.Expr),
		pkgOf:        make(map[ast.Expr]*packages.Package),
		params:       make(map[*types.Var]paramRef),
		fieldWritten: make(map[*types.Var]bool),
		returnMemo:   make(map[resultKey]verdict),
		inProgress:   make(map[resultKey]bool),
		inVar:        make(map[*types.Var]bool),
	}
	for _, pkg := range prog.order {
		c.indexPackage(pkg)
	}
	return c
}

// indexPackage records the assignments and parameters of one package.
func (c *classifier) indexPackage(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.AssignStmt:
				c.indexAssign(pkg, typed)
			case *ast.ValueSpec:
				c.indexValueSpec(pkg, typed)
			case *ast.RangeStmt:
				c.indexRange(pkg, typed)
			}
			return true
		})
	}
	for obj, decl := range c.prog.decls {
		if decl.pkg != pkg {
			continue
		}
		c.indexParams(pkg, obj, decl.decl)
	}
}

// indexAssign records the right-hand sides of an assignment against the
// variables they land in.
//
// A multi-value call assigned to several names is recorded as an unfollowable
// assignment rather than skipped, so a variable that receives one is reported
// unresolved instead of being judged on its other assignments alone.
func (c *classifier) indexAssign(pkg *packages.Package, stmt *ast.AssignStmt) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		c.indexUnbalancedAssign(pkg, stmt)
		return
	}
	for i, lhs := range stmt.Lhs {
		c.noteFieldWrite(pkg, lhs)
		if v := c.localVar(pkg, lhs); v != nil {
			c.assigns[v] = append(c.assigns[v], stmt.Rhs[i])
			c.pkgOf[stmt.Rhs[i]] = pkg
		}
	}
}

// noteFieldWrite records that a field was assigned on a variable, when the
// assignment target is a field of one.
func (c *classifier) noteFieldWrite(pkg *packages.Package, lhs ast.Expr) {
	sel, ok := ast.Unparen(lhs).(*ast.SelectorExpr)
	if !ok {
		return
	}
	base, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return
	}
	if v, isVar := pkg.TypesInfo.Uses[base].(*types.Var); isVar {
		c.fieldWritten[v] = true
	}
}

// indexRange records a loop's key and value variables against the collection
// they come from, so a value interpolated straight out of a range is judged by
// what it was ranged over.
func (c *classifier) indexRange(pkg *packages.Package, stmt *ast.RangeStmt) {
	for _, target := range []ast.Expr{stmt.Key, stmt.Value} {
		if target == nil {
			continue
		}
		if v := c.localVar(pkg, target); v != nil {
			c.assigns[v] = append(c.assigns[v], stmt.X)
			c.pkgOf[stmt.X] = pkg
		}
	}
}

// indexUnbalancedAssign records an assignment whose right-hand side does not
// line up with its left.
//
// The comma-ok forms are followed, because the value they produce is the
// element the operand holds: a lookup in a table of status emoji answers for
// the string it yields. Everything else is recorded as an assignment the audit
// cannot follow, so a variable that receives one is reported unresolved rather
// than judged on its other assignments alone.
func (c *classifier) indexUnbalancedAssign(pkg *packages.Package, stmt *ast.AssignStmt) {
	value := ast.Expr(nil)
	if len(stmt.Lhs) == 2 && len(stmt.Rhs) == 1 && commaOK(stmt.Rhs[0]) {
		value = stmt.Rhs[0]
		c.pkgOf[value] = pkg
	}
	for i, lhs := range stmt.Lhs {
		v := c.localVar(pkg, lhs)
		if v == nil {
			continue
		}
		if i == 0 {
			c.assigns[v] = append(c.assigns[v], value)
			continue
		}
		c.assigns[v] = append(c.assigns[v], nil)
	}
}

// commaOK reports whether an operand is one of the forms that yield a value
// and a boolean: a map index, a type assertion or a channel receive.
func commaOK(expr ast.Expr) bool {
	switch typed := ast.Unparen(expr).(type) {
	case *ast.IndexExpr, *ast.TypeAssertExpr:
		return true
	case *ast.UnaryExpr:
		return typed.Op == token.ARROW
	default:
		return false
	}
}

// indexValueSpec records the initialisers of a var declaration. A declaration
// with no initialiser leaves the variable holding whatever a later assignment
// put there, which the assignments above cover, so the zero value is recorded
// as unfollowable rather than as safe.
func (c *classifier) indexValueSpec(pkg *packages.Package, spec *ast.ValueSpec) {
	if len(spec.Values) == 0 {
		for _, name := range spec.Names {
			if v := c.localVar(pkg, name); v != nil {
				c.assigns[v] = append(c.assigns[v], nil)
			}
		}
		return
	}
	if len(spec.Names) != len(spec.Values) {
		return
	}
	for i, name := range spec.Names {
		if v := c.localVar(pkg, name); v != nil {
			c.assigns[v] = append(c.assigns[v], spec.Values[i])
			c.pkgOf[spec.Values[i]] = pkg
		}
	}
}

// indexParams records each parameter of a declared function against its
// position, so a parameter no caller bound can still be resolved to the
// arguments every caller passes.
func (c *classifier) indexParams(pkg *packages.Package, obj *types.Func, decl *ast.FuncDecl) {
	index := 0
	for _, field := range decl.Type.Params.List {
		if len(field.Names) == 0 {
			index++
			continue
		}
		for _, name := range field.Names {
			if v, ok := pkg.TypesInfo.Defs[name].(*types.Var); ok {
				c.params[v] = paramRef{fn: obj, index: index}
			}
			index++
		}
	}
}

// localVar returns the variable an assignment target names, when that target is
// a plain identifier bound to a variable the classifier can follow. A field, an
// index and the blank identifier are all skipped: none of them is a value the
// classifier can follow backwards, and a struct field is where it stops
// looking by design.
//
// A package-level variable is followed like a local one, so a formatter
// interpolating a package's own default label is answered by the literal that
// declared it rather than reported as a value nobody assigns.
func (c *classifier) localVar(pkg *packages.Package, expr ast.Expr) *types.Var {
	ident, ok := expr.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return nil
	}
	obj := pkg.TypesInfo.Defs[ident]
	if obj == nil {
		obj = pkg.TypesInfo.Uses[ident]
	}
	v, ok := obj.(*types.Var)
	if !ok || v.Parent() == nil {
		return nil
	}
	return v
}
