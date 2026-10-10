// Rendering the documents a module writes with holes in them: the format
// strings fmt.Sprintf fills and the text/template shells a template set
// executes, which is how client-go writes six of its documents.

package graphqldocs

import (
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"maps"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

// The calls a shell's holes are filled by, which is what [Assembly.By] names.
const (
	// AssembledByFormat is a document written as the format of a fmt.Sprintf
	// call.
	AssembledByFormat = "fmt.Sprintf"
	// AssembledByTemplate is a document parsed into a text/template set.
	AssembledByTemplate = "text/template"
)

// The two calls a document is rendered at, by their full names as the type
// checker spells them, so a package that imports either under another name is
// read the same way.
const (
	sprintfName = "fmt.Sprintf"
	parseName   = "(*text/template.Template).Parse"
)

// Assembly says how a document the source writes with holes in it becomes the
// text GitLab receives.
//
// A document with no hole carries none. One with holes carries one whether or
// not it was rendered, so a reader holding a document that is still a shell
// learns why it is one rather than only that it is.
type Assembly struct {
	// By names the call that fills the holes: [AssembledByFormat] or
	// [AssembledByTemplate], and "" when no call this walk reads takes the
	// document.
	By string
	// Shell is the document as the source writes it, holes included.
	Shell string
	// Unrendered says why [Document.Text] is still the shell, and is "" when it
	// is the rendering.
	Unrendered string
}

// site is one call that takes a document as the text it fills: the format of a
// fmt.Sprintf call, or the text a template set parses.
type site struct {
	by   string
	pkg  *packages.Package
	call *ast.CallExpr
}

// initializer is the expression a package variable is declared with, in the
// package that declares it, which is how a template set built in one
// declaration and cloned in another is followed.
type initializer struct {
	pkg   *packages.Package
	value ast.Expr
}

// recordSite notes a call that fills a document, keyed the way the document
// itself is found: by the object a named document is declared as, or by the
// position of one written inline at the call.
//
// The first argument is the document for both calls, and both always have one,
// since a call that type-checked passed fmt.Sprintf its format and Parse its
// text.
func (c *collector) recordSite(pkg *packages.Package, call *ast.CallExpr) {
	by := assemblerOf(pkg.TypesInfo, call)
	if by == "" {
		return
	}
	at := site{by: by, pkg: pkg, call: call}
	argument := ast.Unparen(call.Args[0])
	if object := namedObject(pkg.TypesInfo, argument); object != nil {
		c.sitesByObject[object] = append(c.sitesByObject[object], at)
		return
	}
	position := c.fset.Position(argument.Pos())
	c.sitesByPosition[position] = append(c.sitesByPosition[position], at)
}

// namedObject is the object an argument names, or nil for one written in
// place.
//
// A name declared in another package is written as a qualified identifier,
// and the object its selector resolves to is the one that package declares the
// document as, which is what joins a call in one package to a document shared
// from another. Keyed by where the identifier is written, the call would match
// no document and the document no call.
func namedObject(info *types.Info, argument ast.Expr) types.Object {
	switch typed := argument.(type) {
	case *ast.Ident:
		return info.Uses[typed]
	case *ast.SelectorExpr:
		return info.Uses[typed.Sel]
	default:
		return nil
	}
}

// assemblerOf names what a call fills a document with, or "" for a call that
// fills none.
func assemblerOf(info *types.Info, call *ast.CallExpr) string {
	switch calleeName(info, call) {
	case sprintfName:
		return AssembledByFormat
	case parseName:
		return AssembledByTemplate
	default:
		return ""
	}
}

// calleeName is the full name of the function a call reaches, as the type
// checker spells it, or "" when it reaches none statically: a conversion, a
// builtin or a call through a function value.
//
// It is not the call's own spelling in the second case, because a spelling
// can be anything a package names a value: a package variable called fmt with
// a Sprintf field is called as fmt.Sprintf, and read by its spelling it would
// be rendered as the call it only looks like.
func calleeName(info *types.Info, call *ast.CallExpr) string {
	if callee, isFunc := typeutil.Callee(info, call).(*types.Func); isFunc {
		return callee.FullName()
	}
	return ""
}

// receiverOf is the value a method call is made on. A call reaching a method
// is always written as a selector on that value, which is why the assertion is
// not checked.
func receiverOf(call *ast.CallExpr) ast.Expr {
	selector, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	return selector.X
}

// recordInitializers notes every package variable a declaration gives a value
// to, one value per name.
func (c *collector) recordInitializers(pkg *packages.Package, spec *ast.ValueSpec) {
	if len(spec.Values) != len(spec.Names) {
		return
	}
	for i, name := range spec.Names {
		variable, isVar := pkg.TypesInfo.Defs[name].(*types.Var)
		if isVar && variable.Parent() == pkg.Types.Scope() {
			c.initializers[variable] = initializer{pkg: pkg, value: spec.Values[i]}
		}
	}
}

// render turns every shell the walk found into the text GitLab receives where
// the calls that fill it say what that is, and records why where they do not.
func (c *collector) render() {
	for i := range c.documents {
		if IsTemplate(c.documents[i]) {
			c.assemble(&c.documents[i])
		}
	}
}

// assemble renders one shell in place.
func (c *collector) assemble(document *Document) {
	assembly := &Assembly{Shell: document.Text}
	document.Assembly = assembly
	sites := c.sitesByPosition[document.Position]
	if document.Object != nil {
		sites = c.sitesByObject[document.Object]
	}
	if len(sites) == 0 {
		assembly.Unrendered = "no fmt.Sprintf call and no text/template Parse this walk reads takes it"
		return
	}
	assembly.By = sites[0].by
	rendered, err := c.renderAt(sites, document.Text)
	if err != nil {
		assembly.Unrendered = err.Error()
		return
	}
	document.Text = rendered
}

// renderAt renders a shell at every call that takes it, and answers only when
// they all agree and no hole is left.
//
// A shell taken by two calls is rendered at both rather than at the first,
// because the rendering is a claim about what GitLab receives, and two calls
// that render it differently send two different documents.
func (c *collector) renderAt(sites []site, shell string) (string, error) {
	renderings := map[string]bool{}
	var rendered string
	for _, at := range sites {
		text, err := c.renderOne(at, shell)
		if err != nil {
			return "", err
		}
		renderings[text] = true
		rendered = text
	}
	if len(renderings) > 1 {
		return "", fmt.Errorf("its %d call sites render it %d ways", len(sites), len(renderings))
	}
	if hole := templateHole.FindString(rendered); hole != "" {
		return "", fmt.Errorf("its rendering still carries %s", hole)
	}
	return rendered, nil
}

// renderOne renders a shell at one call.
func (c *collector) renderOne(at site, shell string) (string, error) {
	if at.by == AssembledByFormat {
		return renderFormat(at.pkg.TypesInfo, at.call, shell)
	}
	return c.renderTemplate(at, shell)
}

// fmtComplaint matches what fmt writes in place of a verb its value does not
// fit, a value it has no verb for, or a verb it has no value for.
var fmtComplaint = regexp.MustCompile(`%!\w*\([^)]*\)`)

// renderFormat calls fmt.Sprintf with the format and a stand-in for each value
// the call hands it, which is the text the call sends with only the values
// changed.
//
// A value written at the call as a constant is that constant. Any other is a
// stand-in of its type, and a string is spelled as the expression it comes
// from, so a reader of the rendering sees where each value would go: the
// Terraform state query renders with fullPath: "projectFullPath".
func renderFormat(info *types.Info, call *ast.CallExpr, format string) (string, error) {
	if call.Ellipsis.IsValid() {
		return "", errors.New("its values are a slice spread into the call, so no argument says what a hole holds")
	}
	values := make([]any, 0, len(call.Args))
	for _, argument := range call.Args[1:] {
		value, err := standIn(info, argument)
		if err != nil {
			return "", err
		}
		values = append(values, value)
	}
	rendered := fmt.Sprintf(format, values...)
	if complaint := fmtComplaint.FindString(rendered); complaint != "" {
		return "", fmt.Errorf("fmt.Sprintf answers %s, so a verb and its value disagree", complaint)
	}
	return rendered, nil
}

// standIn is the value a hole is filled with for one argument.
func standIn(info *types.Info, argument ast.Expr) (any, error) {
	typed := info.Types[argument]
	if typed.Value != nil {
		return constantValue(typed.Value, argument)
	}
	if basic, isBasic := typed.Type.Underlying().(*types.Basic); isBasic {
		if value := basicStandIn(basic, argument); value != nil {
			return value, nil
		}
	}
	return nil, fmt.Errorf("fmt.Sprintf formats %s, a %s, which this walk has no stand-in for", types.ExprString(argument), typed.Type)
}

// constantValue is a constant argument's own value.
func constantValue(value constant.Value, argument ast.Expr) (any, error) {
	switch value.Kind() {
	case constant.String:
		return constant.StringVal(value), nil
	case constant.Bool:
		return constant.BoolVal(value), nil
	case constant.Int:
		integer, _ := constant.Int64Val(value)
		return integer, nil
	case constant.Float:
		float, _ := constant.Float64Val(value)
		return float, nil
	default:
		return nil, fmt.Errorf("fmt.Sprintf formats %s, a %s constant, which this walk has no stand-in for", types.ExprString(argument), value.Kind())
	}
}

// basicStandIn is the stand-in for a value of a basic type, or nil for a kind
// no GraphQL document is written with.
//
// An unsigned integer takes the signed stand-in: every verb fmt has for an
// integer writes 1 the same whichever of the two it is handed, so a stand-in of
// its own would render nothing different.
//
// Written as a chain of returns rather than a tagless switch, whose case
// expressions carry no statement counter for the mutation gate to see.
func basicStandIn(basic *types.Basic, argument ast.Expr) any {
	info := basic.Info()
	if info&types.IsString != 0 {
		return placeholder(argument)
	}
	if info&types.IsBoolean != 0 {
		return true
	}
	if info&types.IsInteger != 0 {
		return int64(1)
	}
	if info&types.IsFloat != 0 {
		return 1.5
	}
	return nil
}

// notInAName matches a character a GraphQL name cannot hold.
var notInAName = regexp.MustCompile(`[^A-Za-z0-9_]`)

// placeholder spells an expression as a name: every character a GraphQL name
// cannot hold becomes an underscore, so opt.Path is opt_Path and the stand-in
// reads the same in a string literal and wherever a name goes.
func placeholder(argument ast.Expr) string {
	return notInAName.ReplaceAllString(types.ExprString(argument), "_")
}

// templateSet is a text/template set as the source builds one: the text of
// every template it defines, by name, and the template it currently is.
type templateSet struct {
	name  string
	texts map[string]string
}

// renderTemplate executes the template a Parse call defines, inside the set
// its receiver resolves to, with no data: what client-go does for every
// template whose text reads none.
func (c *collector) renderTemplate(at site, shell string) (string, error) {
	set, err := c.resolveSet(at.pkg, receiverOf(at.call))
	if err != nil {
		return "", err
	}
	set.texts[set.name] = shell
	return set.execute()
}

// resolveSet reads the template set an expression evaluates to, following the
// calls a set is built with (template.New and template.Must, and on a set
// Clone, New and Parse with a constant text) back through the package
// variables they start from.
//
// An initialization cycle does not type-check, and an unchecked package is
// refused before this walk runs, so following initializers ends.
func (c *collector) resolveSet(pkg *packages.Package, expr ast.Expr) (templateSet, error) {
	switch typed := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return c.resolveVariable(pkg, typed)
	case *ast.CallExpr:
		return c.resolveCall(pkg, typed)
	default:
		return templateSet{}, fmt.Errorf("its template set comes from %s, which this walk does not follow", types.ExprString(expr))
	}
}

// resolveVariable follows a package variable to the set it is declared with.
//
// An identifier holding a template set is a variable of one kind or another (a
// package variable, a local, a parameter or a named result), which is why the
// assertion is not checked; only the first is declared with a value this walk
// can read.
func (c *collector) resolveVariable(pkg *packages.Package, name *ast.Ident) (templateSet, error) {
	variable, _ := pkg.TypesInfo.Uses[name].(*types.Var)
	if variable.Parent() != variable.Pkg().Scope() {
		return templateSet{}, fmt.Errorf("%s is not a package variable, so the template set it holds is built when a function runs", name.Name)
	}
	declared, found := c.initializers[variable]
	if !found {
		return templateSet{}, fmt.Errorf("%s is declared with no value this walk reads", name.Name)
	}
	return c.resolveSet(declared.pkg, declared.value)
}

// The calls a template set is built with that resolveCall follows, besides
// [parseName].
const (
	mustName  = "text/template.Must"
	newName   = "text/template.New"
	cloneName = "(*text/template.Template).Clone"
	childName = "(*text/template.Template).New"
)

// resolveCall follows one call a template set is built with.
func (c *collector) resolveCall(pkg *packages.Package, call *ast.CallExpr) (templateSet, error) {
	switch callee := calleeName(pkg.TypesInfo, call); callee {
	case mustName:
		return c.resolveSet(pkg, call.Args[0])
	case newName:
		name, err := constantText(pkg, call.Args[0], "named by")
		return templateSet{name: name, texts: map[string]string{}}, err
	case cloneName, childName, parseName:
		return c.resolveMethod(pkg, call, callee)
	default:
		return templateSet{}, fmt.Errorf("its template set is built with %s, a call this walk does not follow", types.ExprString(call.Fun))
	}
}

// resolveMethod follows a call on a template set: a clone is the set it was
// made from, New names the template the set now is, and Parse gives that
// template its text.
func (c *collector) resolveMethod(pkg *packages.Package, call *ast.CallExpr, callee string) (templateSet, error) {
	set, err := c.resolveSet(pkg, receiverOf(call))
	if err != nil || callee == cloneName {
		return set, err
	}
	if callee == childName {
		set.name, err = constantText(pkg, call.Args[0], "named by")
		return set, err
	}
	set.texts[set.name], err = constantText(pkg, call.Args[0], "parsed from")
	return set, err
}

// constantText is the folded string an argument holds, or an error saying it
// holds none. Both calls it reads take a string, so a constant argument is a
// string constant.
func constantText(pkg *packages.Package, argument ast.Expr, role string) (string, error) {
	value := pkg.TypesInfo.Types[argument].Value
	if value == nil {
		return "", fmt.Errorf("a template in its set is %s %s, which is not a constant", role, types.ExprString(argument))
	}
	return constant.StringVal(value), nil
}

// execute parses every template of the set and executes the current one with
// no data, once nothing it reaches reads any.
func (s templateSet) execute() (string, error) {
	root := template.New(s.name)
	for _, name := range slices.Sorted(maps.Keys(s.texts)) {
		if _, err := root.New(name).Parse(s.texts[name]); err != nil {
			return "", fmt.Errorf("the template %q does not parse: %w", name, err)
		}
	}
	if err := readsNoData(root, s.name, map[string]bool{}); err != nil {
		return "", err
	}
	var rendered strings.Builder
	if err := root.ExecuteTemplate(&rendered, s.name, nil); err != nil {
		return "", fmt.Errorf("the template %q does not execute: %w", s.name, err)
	}
	return rendered.String(), nil
}

// readsNoData reports why a template's output depends on data, or nil when it
// is the same text whatever the caller passes: nothing but text and the
// templates it names, each of which reads none either.
//
// That is the condition for a rendering to be a claim about every request: a
// template that reads a value renders what client-go sends for one caller. The
// work item list, which reads the filters a caller sets, never reaches this
// check: its template set is built inside a function, and [resolveVariable]
// stops at that before anything is executed, which is the reason its assembly
// records.
//
// A template is looked at once however often it is named, which is what ends
// the walk of one that names itself; executing that one is what fails.
func readsNoData(set *template.Template, name string, seen map[string]bool) error {
	if seen[name] {
		return nil
	}
	seen[name] = true
	defined := set.Lookup(name)
	if defined == nil {
		return fmt.Errorf("it names the template %q, which nothing this walk reads defines", name)
	}
	for _, node := range defined.Root.Nodes {
		if err := nodeReadsNoData(set, node, seen); err != nil {
			return err
		}
	}
	return nil
}

// nodeReadsNoData is [readsNoData] for one node of a template's text.
func nodeReadsNoData(set *template.Template, node parse.Node, seen map[string]bool) error {
	switch typed := node.(type) {
	case *parse.TextNode:
		return nil
	case *parse.TemplateNode:
		if typed.Pipe != nil {
			return fmt.Errorf("it hands the template %q a value", typed.Name)
		}
		return readsNoData(set, typed.Name, seen)
	default:
		return fmt.Errorf("it reads %s, a value client-go fills when the method runs", node)
	}
}
