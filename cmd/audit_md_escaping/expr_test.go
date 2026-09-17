package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// holeByExpression finds one hole of the fixture by the expression a finding
// would print for it, so a case names a value rather than a position.
func holeByExpression(t *testing.T, prog *program, pkgName, expression string) (*packages.Package, ast.Expr) {
	t.Helper()
	for _, s := range collectSinks(prog) {
		if !strings.HasSuffix(s.pkg.PkgPath, "/"+pkgName) {
			continue
		}
		for _, h := range s.holes {
			if types.ExprString(h.expr) == expression {
				return s.pkg, h.expr
			}
		}
	}
	t.Fatalf("no hole in %s interpolates %s", pkgName, expression)
	return nil, nil
}

// edgeFixture is the fixture for the shapes that sit at the edges of the
// walk: a call whose arguments do not line up with the signature it calls, a
// parameter nobody named, a variable nothing initializes, a template
// declaring more verbs than the call passes, and the two cell-builder calls
// that carry no card row.
//
// It is a fixture set of its own rather than a package added to caseFixture so
// that the want lists the other tests pin do not move when an edge is added.
var edgeFixture = map[string]string{"mdedge/mdedge.go": mdedgeSource}

// mdedgeSource holds one of each edge shape, written the way a formatter that
// hit it would write it.
const mdedgeSource = `package mdedge

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Item is the shape a GitLab response fills.
type Item struct {
	Title string
}

// blank is declared with no initializer, so no literal the audit can see ever
// filled it and a field read from it answers for none.
var blank Item

// pair yields two values at once, which is how a call comes to pass fewer
// arguments than the function it calls declares parameters.
func pair() (string, string) {
	return blank.Title, "fixed"
}

// Render writes two cells and is called once, by a call that spreads both of
// its arguments out of a single operand.
func Render(left, right string) string {
	return fmt.Sprintf("| %s | %s |\n", left, right)
}

// Spread is that call, written into a cell so the call itself is judged.
func Spread() string {
	return fmt.Sprintf("| %s |\n", Render(pair()))
}

// Ignored takes a parameter nobody named, which must not shift the position
// of the one after it.
func Ignored(_ string, item Item) string {
	return fmt.Sprintf("| %s |\n", item.Title)
}

// TooFewOperands writes templates declaring more verbs than the call passes,
// which fmt answers with %!s(MISSING), at both levels the audit reads a
// template at.
func TooFewOperands(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s |\n", item.Title)
	fmt.Fprintf(&b, "| %s |\n", fmt.Sprintf("%s %s", item.Title))
	return b.String()
}

// Cells writes the two cell-builder calls that carry no card row: one with no
// cells at all, and one whose every cell is constant.
func Cells() string {
	return toolutil.MarkdownTableRow() + toolutil.MarkdownTableRow("Name", "fixed")
}

// FromBlank reads a field of the variable nothing initializes.
func FromBlank() string {
	return fmt.Sprintf("| %s |\n", blank.Title)
}
`

// TestClassifyParam_ArgumentsThatDoNotLineUp_AreNotBoundToACallSite checks
// what the audit does with a call that passes one operand for two parameters.
//
// Binding is what makes a finding belong to a call site, so a call the
// signature cannot be laid over must bind nothing: the first parameter is
// still answered by what that operand produces, and the second, which no
// argument of the call reaches at all, is unresolved rather than assumed safe.
// A non-variadic function is also the one shape that reaches the empty-variadic
// guard with nothing variadic about it.
func TestClassifyParam_ArgumentsThatDoNotLineUp_AreNotBoundToACallSite(t *testing.T) {
	prog := loadFixture(t, edgeFixture)
	c := newClassifier(prog)
	pkg, call := holeByExpression(t, prog, "mdedge", "Render(pair())")

	right, ok := paramNamed(c, "Render", "right")
	if !ok {
		t.Fatal("the fixture's second parameter was not indexed")
	}

	got, why := c.classifyParam(right, 0)
	if got != unresolved {
		t.Errorf("classifyParam on the parameter no argument reaches = %v (%s), want unresolved", got, why)
	}
	if !strings.Contains(why, "shape the audit does not follow") {
		t.Errorf("reason %q does not say the call shape could not be read", why)
	}

	// The call itself carries that answer out: with nothing bound, what the
	// callee returns is judged from every caller, and the parameter no
	// argument reaches is what the verdict rests on.
	if got, why = c.classifyExpr(pkg, call, nil, 0); got != unresolved {
		t.Errorf("classifyExpr on the call itself = %v (%s), want unresolved", got, why)
	}
}

// TestClassifyIdent_FieldOfAVariableNothingInitializes_IsReported checks the
// guard on a single assignment the audit was handed nothing for.
//
// A variable declared with no initializer is recorded as assigned nothing, and
// the walk back to a literal must stop there rather than treat that nothing as
// the literal that built the struct: doing so would read every field of it as
// left at its zero value, which is the one way this audit could call a raw
// value safe.
func TestClassifyIdent_FieldOfAVariableNothingInitializes_IsReported(t *testing.T) {
	prog := loadFixture(t, edgeFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdedge", "blank.Title")

	got, why := c.classifyExpr(pkg, expr, nil, 0)

	if got != unescaped {
		t.Errorf("classifyExpr on a field of an uninitialized variable = %v (%s), want unescaped", got, why)
	}
	if !strings.Contains(why, "filled from a GitLab response") {
		t.Errorf("reason %q does not say the field is unaccounted for", why)
	}
}

// TestClassifySprintf_MoreVerbsThanOperands_JudgesTheOnesPassed checks that a
// nested template declaring a hole the call never fills is read as the holes
// it does fill, rather than reaching past the end of the argument list.
func TestClassifySprintf_MoreVerbsThanOperands_JudgesTheOnesPassed(t *testing.T) {
	prog := loadFixture(t, edgeFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdedge", `fmt.Sprintf("%s %s", item.Title)`)

	got, why := c.classifyExpr(pkg, expr, nil, 0)

	if got != unescaped {
		t.Errorf("classifyExpr on a short nested Sprintf = %v (%s), want unescaped from its one operand", got, why)
	}
}

// TestClassifyExpr_Fixture_AnswersEachShape walks one value of every shape the
// classifier has a rule for, and pins both the verdict and the reason it gives,
// since the reason is what the walk through the findings acts on.
func TestClassifyExpr_Fixture_AnswersEachShape(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)

	cases := []struct {
		name       string
		pkg        string
		expression string
		want       verdict
		reason     string
	}{
		{
			name: "a value already through an escaper", pkg: "mdsafe",
			expression: "toolutil.EscapeMdHeading(item.Title)", want: safe, reason: "already through toolutil.EscapeMdHeading",
		},
		{
			name: "a number", pkg: "mdsafe",
			expression: "item.Count", want: safe, reason: "renders as a number",
		},
		{
			name: "a timestamp", pkg: "mdsafe",
			expression: "item.When", want: safe, reason: "renders as a number",
		},
		{
			name: "a standard-library formatter", pkg: "mdsafe",
			expression: "strconv.Itoa(item.Count)", want: safe, reason: "standard-library formatter",
		},
		{
			name: "a strings transform of a safe value", pkg: "mdsafe",
			expression: "strings.TrimSpace(toolutil.EscapeMdTableCell(item.Title))", want: safe, reason: "already safe",
		},
		{
			name: "a helper whose every return is safe", pkg: "mdsafe",
			expression: "label(item)", want: safe, reason: "everything label returns is safe",
		},
		{
			name: "a lookup in a table the server wrote", pkg: "mdsafe",
			expression: "statusIcon(item)", want: safe, reason: "everything statusIcon returns is safe",
		},
		{
			name: "a nested Sprintf of safe halves", pkg: "mdsafe",
			expression: `fmt.Sprintf("%s (%s)", toolutil.EscapeMdTableCell(item.Title), "server text")`,
			want:       safe, reason: "every value the nested Sprintf interpolates is safe",
		},
		{
			name: "a field of an options struct the caller built", pkg: "mdsafe",
			expression: "opts.Title", want: safe, reason: "every caller of FormatItem passes a safe Title",
		},
		{
			name: "a field the caller's literal leaves empty", pkg: "mdsafe",
			expression: "opts.Column", want: safe, reason: "every caller of FormatItem passes a safe Column",
		},
		{
			name: "a conversion of a number", pkg: "mdsafe",
			expression: "string(rune(item.Count))", want: safe, reason: "renders as a number",
		},
		{
			name: "a value bound to a constant through a helper", pkg: "mdsafe",
			expression: `toolutil.FormatTime("2024-01-01")`, want: safe, reason: "everything FormatTime returns is safe",
		},
		{
			name: "a slice allocated and appended to out of escaped values", pkg: "mdsafe",
			expression: `strings.Join(joined(item), ", ")`, want: safe, reason: "made only of values that are already safe",
		},
		{
			name: "the zero value the builtin new allocates", pkg: "mdsafe",
			expression: "*new(string)", want: safe, reason: "an empty value the builtin new allocates",
		},
		{
			name: "a variadic parameter one caller leaves empty", pkg: "mdsafe",
			expression: "hinted()", want: safe, reason: "everything hinted returns is safe",
		},
		{
			name: "a slice allocated and appended to out of raw values", pkg: "mdcase",
			expression: `strings.Join(rawJoined(item), ", ")`, want: unescaped, reason: "handed to FormatShapes",
		},
		{
			name: "a builtin other than append", pkg: "mdcase",
			expression: `min(item.Title, "z")`, want: unresolved, reason: "the builtin min",
		},
		{
			name: "a field of a GitLab response", pkg: "mdcase",
			expression: "item.State", want: unescaped, reason: "handed to FormatOutputMarkdown",
		},
		{
			name: "an element of a field", pkg: "mdcase",
			expression: "item.Labels[0]", want: unescaped, reason: "handed to FormatOutputMarkdown",
		},
		{
			name: "a shared helper one caller passes a raw value to", pkg: "mdcase",
			expression: "title", want: unescaped, reason: "from a call site of FormatRow",
		},
		{
			name: "a recursive helper carrying a raw value", pkg: "mdcase",
			expression: "repeat(item.Title, depth)", want: unescaped, reason: "handed to FormatRecursive",
		},
		{
			name: "a field filled after the struct was built", pkg: "mdcase",
			expression: "pair.Right", want: unescaped, reason: "a field of a value filled from a GitLab response",
		},
		{
			// The helper reads it.Title, and it is bound to the argument
			// FormatDelegated passed, so the answer names FormatDelegated
			// rather than every caller of the helper. Following the binding is
			// what keeps a helper called once from reading as a value the
			// audit cannot resolve.
			name: "a field of a value the caller bound to the helper", pkg: "mdcase",
			expression: "delegated(item)", want: unescaped, reason: "handed to FormatDelegated",
		},
		{
			name: "a call of a function value", pkg: "mdcase",
			expression: "render(item)", want: unresolved, reason: "function value",
		},
		{
			name: "a field of a struct built positionally", pkg: "mdcase",
			expression: "pair.Left", want: unresolved, reason: "built positionally",
		},
		{
			name: "one of several results", pkg: "mdcase",
			expression: "value", want: unresolved, reason: "an assignment the audit does not follow",
		},
		{
			name: "a named result", pkg: "mdcase",
			expression: "namedResult(item)", want: unresolved, reason: "named result",
		},
		{
			// The one caller passes nothing to rest, which is an empty slice
			// and not a shape the audit cannot read, so nothing reaches the
			// hole through it.
			name: "a variadic parameter no caller fills", pkg: "mdcase",
			expression: "rest[0]", want: safe, reason: "every caller of FormatVariadic passes a safe value",
		},
		{
			name: "a package-level variable nothing assigns", pkg: "mdcase",
			expression: "mutableTitle", want: unresolved, reason: "an assignment the audit does not follow",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, expr := holeByExpression(t, prog, tc.pkg, tc.expression)
			got, why := c.classifyExpr(pkg, expr, nil, 0)
			if got != tc.want {
				t.Errorf("classifyExpr(%s) = %v (%s), want %v", tc.expression, got, why, tc.want)
			}
			if !strings.Contains(why, tc.reason) {
				t.Errorf("reason for %s is %q, want one saying %q", tc.expression, why, tc.reason)
			}
		})
	}
}

// TestClassifyExpr_Guards_StopWithoutCallingAValueSafe checks the two ends of
// the walk that are not shapes at all: a chain deeper than it follows, and an
// assignment it was handed nothing for.
func TestClassifyExpr_Guards_StopWithoutCallingAValueSafe(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdcase", "item.State")

	untyped := untypedPackage()
	typeless := ast.NewIdent("typeless")
	untyped.TypesInfo.Types[typeless] = types.TypeAndValue{}

	cases := []struct {
		name   string
		pkg    *packages.Package
		expr   ast.Expr
		depth  int
		reason string
	}{
		{name: "deeper than the walk follows", expr: expr, depth: maxDepth + 1, reason: "deeper than the audit follows"},
		{name: "nothing to judge", expr: nil, reason: "an assignment the audit does not follow"},
		{name: "a shape with no rule", expr: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}, reason: "a shape the audit does not follow"},
		{
			// An expression the type checker recorded with no type at all
			// must not fall through the non-textual test, which would call it
			// safe on the strength of a type nobody knows.
			name: "recorded with no type", pkg: untyped, expr: typeless,
			reason: "names something other than a variable",
		},
		{
			// calleeOf declines a call it cannot resolve, and the shape test
			// in front of it must decline it too rather than read the missing
			// entry as a conversion and judge the first operand instead.
			name: "a call of nothing the type checker knows", pkg: untyped,
			expr:   &ast.CallExpr{Fun: ast.NewIdent("unknown"), Args: []ast.Expr{ast.NewIdent("v")}},
			reason: "of a function value rather than of a named function",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := pkg
			if tc.pkg != nil {
				in = tc.pkg
			}
			got, why := c.classifyExpr(in, tc.expr, nil, tc.depth)
			if got != unresolved {
				t.Errorf("classifyExpr = %v, want unresolved", got)
			}
			if !strings.Contains(why, tc.reason) {
				t.Errorf("reason %q does not say %q", why, tc.reason)
			}
		})
	}
}

// untypedPackage is a package whose type information is empty: every lookup
// misses. It stands for the one state the audit must never read as an answer,
// since a missing entry says nothing about the value rather than saying the
// value is safe.
func untypedPackage() *packages.Package {
	return &packages.Package{
		PkgPath: modulePath + "/" + fixtureDir + "/mdempty",
		TypesInfo: &types.Info{
			Types: map[ast.Expr]types.TypeAndValue{},
			Defs:  map[*ast.Ident]types.Object{},
			Uses:  map[*ast.Ident]types.Object{},
		},
	}
}

// TestClassifySprintf_TemplateTheTypeCheckerDidNotRecord_IsUnresolved checks
// the other end of the nested-template rule: with no recorded value for the
// template there are no holes to walk, and the answer has to be that the audit
// could not read it rather than that everything it interpolates is safe.
func TestClassifySprintf_TemplateTheTypeCheckerDidNotRecord_IsUnresolved(t *testing.T) {
	c := newClassifier(&program{decls: map[*types.Func]*funcDecl{}, callers: map[*types.Func][]callSite{}})
	call := &ast.CallExpr{Fun: ast.NewIdent("Sprintf"), Args: []ast.Expr{ast.NewIdent("tmpl"), ast.NewIdent("v")}}

	got, why := c.classifySprintf(untypedPackage(), call, nil, 0)

	if got != unresolved {
		t.Errorf("classifySprintf on an unrecorded template = %v (%s), want unresolved", got, why)
	}
	if !strings.Contains(why, "not a constant") {
		t.Errorf("reason %q does not say the template is not constant", why)
	}
}

// TestClassifyIdent_RecordedWithNoAssignment_IsUnresolved checks the second
// half of the guard on a variable's assignment list.
//
// An entry holding no expression is not the same as a variable every
// assignment to which is safe, and the loop below it would answer exactly that
// for an empty list, so the emptiness has to be caught before the loop.
func TestClassifyIdent_RecordedWithNoAssignment_IsUnresolved(t *testing.T) {
	c := newClassifier(&program{decls: map[*types.Func]*funcDecl{}, callers: map[*types.Func][]callSite{}})
	pkg := untypedPackage()
	ident := ast.NewIdent("recorded")
	variable := types.NewVar(token.NoPos, nil, "recorded", types.Typ[types.String])
	pkg.TypesInfo.Uses[ident] = variable
	c.assigns[variable] = nil

	got, why := c.classifyIdent(pkg, ident, nil, 0)

	if got != unresolved {
		t.Errorf("classifyIdent on a variable recorded with no assignment = %v (%s), want unresolved", got, why)
	}
	if !strings.Contains(why, "nothing the audit can see assigns recorded") {
		t.Errorf("reason %q does not name the variable nothing assigns", why)
	}
}

// TestClassifyField_KeyThatIsNotAnIdentifier_LeavesTheFieldUnmatched checks
// the guard on a keyed element whose key is not a field name. No struct
// literal has one, so this is what keeps the walk from reading a key it cannot
// compare as though it matched the field being asked about.
func TestClassifyField_KeyThatIsNotAnIdentifier_LeavesTheFieldUnmatched(t *testing.T) {
	c := newClassifier(&program{decls: map[*types.Func]*funcDecl{}, callers: map[*types.Func][]callSite{}})
	lit := &ast.CompositeLit{Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: &ast.BasicLit{Kind: token.STRING, Value: `"Title"`}, Value: ast.NewIdent("raw")},
	}}

	got, why := c.classifyField(untypedPackage(), lit, nil, "Title", 0)

	if got != safe {
		t.Errorf("classifyField over a key it cannot read = %v (%s), want the zero-value answer", got, why)
	}
	if !strings.Contains(why, "zero value") {
		t.Errorf("reason %q does not say the field was left unmatched", why)
	}
}

// TestResolveBase_Depth_StopsAtTheExpressionItHas checks that the guard on the
// walk back to a literal returns the expression it was looking at rather than
// nothing, so the caller still has something to judge.
func TestResolveBase_Depth_StopsAtTheExpressionItHas(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdcase", "item.State")

	base := c.resolveBase(pkg, expr, nil, maxDepth+1)

	if base.expr != expr {
		t.Errorf("resolveBase past the depth guard returned %v, want the expression it was given", base.expr)
	}
	if got := c.resolveBase(pkg, nil, nil, 0); got.expr != nil {
		t.Errorf("resolveBase(nil) returned %v, want nothing", got.expr)
	}
	unknown := ast.NewIdent("nothingNamesThis")
	if got := c.resolveBase(pkg, unknown, nil, 0); got.expr != unknown {
		t.Errorf("resolveBase of an identifier that names nothing returned %v, want the identifier", got.expr)
	}
	if _, isParam := c.paramOf(bound{expr: unknown, pkg: pkg}); isParam {
		t.Error("an identifier that names nothing was read as a parameter")
	}
	if _, isParam := c.paramOf(bound{expr: &ast.CompositeLit{}, pkg: pkg}); isParam {
		t.Error("a literal was read as a parameter")
	}
}

// TestClassifyParamField_ShortCall_TellsAnEmptyVariadicFromAShortCall checks
// the guard on a call that passes fewer arguments than the parameter being
// asked about. A variadic parameter left empty is an empty slice, which
// carries nothing; a call short of a parameter that is not variadic cannot
// type-check, so the guard is exercised with a call injected into the index,
// and it answers unresolved rather than passing the value.
func TestClassifyParamField_ShortCall_TellsAnEmptyVariadicFromAShortCall(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	rest, ok := paramNamed(c, "FormatVariadic", "rest")
	if !ok {
		t.Fatal("the fixture's variadic parameter was not indexed")
	}
	prefix, ok := paramNamed(c, "FormatVariadic", "prefix")
	if !ok {
		t.Fatal("the fixture's first parameter was not indexed")
	}

	if got, why := c.classifyParamField(rest, "Title", 0); got != safe || !strings.Contains(why, "passes a safe Title") {
		t.Errorf("classifyParamField on the empty variadic = %v (%s), want safe", got, why)
	}
	if got, why := c.classifyParam(rest, 0); got != safe || !strings.Contains(why, "passes a safe value") {
		t.Errorf("classifyParam on the empty variadic = %v (%s), want safe", got, why)
	}

	// The injected call goes first: the real caller passes a literal, which
	// the field walk answers for before it reaches a second site.
	pkg := fixturePackage(t, prog, "mdcase")
	original := c.prog.callers[prefix.fn]
	c.prog.callers[prefix.fn] = append([]callSite{{call: &ast.CallExpr{Fun: ast.NewIdent("FormatVariadic")}, pkg: pkg}}, original...)
	t.Cleanup(func() { c.prog.callers[prefix.fn] = original })

	if got, why := c.classifyParamField(prefix, "Title", 0); got != unresolved || !strings.Contains(why, "shape the audit does not follow") {
		t.Errorf("classifyParamField on a short call = %v (%s), want unresolved", got, why)
	}
	if got, why := c.classifyParam(prefix, 0); got != unresolved || !strings.Contains(why, "shape the audit does not follow") {
		t.Errorf("classifyParam on a short call = %v (%s), want unresolved", got, why)
	}

	// The same injected call stops before the parameter in front of the
	// variadic one, so it is not a call that left the variadic empty: it is a
	// call the audit could not lay the signature over, and the variadic
	// parameter must not be excused on the strength of the one that was.
	if got, why := c.classifyParam(rest, 0); got != unresolved || !strings.Contains(why, "shape the audit does not follow") {
		t.Errorf("classifyParam on a call short of the parameters before the variadic = %v (%s), want unresolved", got, why)
	}
}

// TestBuiltinOf_CallShapes_NamesABuiltinAndNothingElse checks the one
// recognition the classifier makes past calleeOf: a call of a builtin, by
// name, against a call through a selector and a call of a plain function,
// neither of which is one.
func TestBuiltinOf_CallShapes_NamesABuiltinAndNothingElse(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	pkg, expr := holeByExpression(t, prog, "mdcase", `min(item.Title, "z")`)
	builtin, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected a call, got %T", expr)
	}

	cases := []struct {
		name string
		call *ast.CallExpr
		want string
	}{
		{name: "a builtin", call: builtin, want: "min"},
		{name: "a call through a selector", call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("item"), Sel: ast.NewIdent("render")}}},
		{name: "a plain function", call: &ast.CallExpr{Fun: ast.NewIdent("nothingNamesThis")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, isBuiltin := builtinOf(pkg, tc.call)
			if isBuiltin != (tc.want != "") || got != tc.want {
				t.Errorf("builtinOf = %q, %v, want %q", got, isBuiltin, tc.want)
			}
		})
	}
}

// TestClassifyParam_RawCallSite_NamesTheCallerInTheReason checks that a
// finding reported at a shared helper says which caller made it fail, by
// package, file and line, since a hundred and seventy-eight packages have a
// markdown.go and the helper's own line is not where the fix goes.
func TestClassifyParam_RawCallSite_NamesTheCallerInTheReason(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdcase", "title")

	got, why := c.classifyExpr(pkg, expr, nil, 0)

	if got != unescaped {
		t.Fatalf("classifyExpr(title) = %v (%s), want unescaped", got, why)
	}
	if !strings.Contains(why, "from a call site of FormatRow ("+fixtureDir+"/mdcase/mdcase.go:") {
		t.Errorf("reason %q does not name the caller's package, file and line", why)
	}
}

// paramNamed finds one indexed parameter by the function and name declaring it.
func paramNamed(c *classifier, fn, name string) (paramRef, bool) {
	for v, ref := range c.params {
		if ref.fn.Name() == fn && v.Name() == name {
			return ref, true
		}
	}
	return paramRef{}, false
}

// TestConstantText_Values_ReadsOnlyAString checks that a constant of another
// kind yields no template, since a numeric constant in the format position is
// not something to parse holes out of.
func TestConstantText_Values_ReadsOnlyAString(t *testing.T) {
	cases := []struct {
		name string
		tv   types.TypeAndValue
		want string
	}{
		{name: "a string", tv: types.TypeAndValue{Value: constant.MakeString("| %s |")}, want: "| %s |"},
		{name: "a number", tv: types.TypeAndValue{Value: constant.MakeInt64(7)}},
		{name: "no constant at all", tv: types.TypeAndValue{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := constantText(tc.tv); got != tc.want {
				t.Errorf("constantText = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestClassifyPassThrough_ShortCall_IsUnresolved checks the guard on a
// transform called with fewer arguments than the audit expects, which is a
// shape it must not read as safe.
func TestClassifyPassThrough_ShortCall_IsUnresolved(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdcase", "item.State")

	got, why := c.classifyPassThrough(pkg, &ast.CallExpr{Fun: ast.NewIdent("f"), Args: []ast.Expr{expr}}, []int{0, 2}, nil, 0)

	if got != unresolved || !strings.Contains(why, "shape the audit does not follow") {
		t.Errorf("classifyPassThrough = %v (%s), want unresolved", got, why)
	}
}

// TestClassifyIdent_NotAVariable_IsUnresolved checks the answer for an
// identifier that names something other than a value.
func TestClassifyIdent_NotAVariable_IsUnresolved(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg := fixturePackage(t, prog, "mdcase")

	got, why := c.classifyIdent(pkg, ast.NewIdent("nothingNamesThis"), nil, 0)

	if got != unresolved || !strings.Contains(why, "other than a variable") {
		t.Errorf("classifyIdent = %v (%s), want unresolved", got, why)
	}
}

// TestClassifySelector_MethodValue_IsUnresolved checks that a method used as a
// value is answered by the bucket for what the audit cannot follow.
func TestClassifySelector_MethodValue_IsUnresolved(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	pkg, expr := holeByExpression(t, prog, "mdsafe", "statusIcon(item)")
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected a call, got %T", expr)
	}

	got, why := c.classifyExpr(pkg, &ast.SelectorExpr{X: call.Args[0], Sel: call.Fun.(*ast.Ident)}, nil, 0)

	if got != unresolved || !strings.Contains(why, "method value") {
		t.Errorf("classifyExpr on a method value = %v (%s), want unresolved", got, why)
	}
}

// TestTextual_Types_TellsTextFromEverythingElse checks the rule that makes the
// numeric verbs free without a whitelist, over the type shapes a formatter's
// output structs are built from.
func TestTextual_Types_TellsTextFromEverythingElse(t *testing.T) {
	str := types.Typ[types.String]
	num := types.Typ[types.Int]
	textStruct := types.NewStruct([]*types.Var{types.NewField(token.NoPos, nil, "Title", str, false)}, nil)
	numStruct := types.NewStruct([]*types.Var{types.NewField(token.NoPos, nil, "Count", num, false)}, nil)
	// Ten slices deep, which is past the bound the walk carries against a
	// type that contains itself in a way the seen set cannot catch.
	deep := types.Type(str)
	for range 10 {
		deep = types.NewSlice(deep)
	}

	cases := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "a string", typ: str, want: true},
		{name: "a number", typ: num, want: false},
		{name: "a boolean", typ: types.Typ[types.Bool], want: false},
		{name: "a pointer to a string", typ: types.NewPointer(str), want: true},
		{name: "a slice of strings", typ: types.NewSlice(str), want: true},
		{name: "a slice of numbers", typ: types.NewSlice(num), want: false},
		{name: "an array of strings", typ: types.NewArray(str, 2), want: true},
		{name: "a map keyed by a string", typ: types.NewMap(str, num), want: true},
		{name: "a map whose values are strings", typ: types.NewMap(num, str), want: true},
		{name: "a map of numbers", typ: types.NewMap(num, num), want: false},
		{name: "a nesting deeper than the walk follows", typ: deep, want: false},
		{name: "a struct holding a string", typ: textStruct, want: true},
		{name: "a struct holding only numbers", typ: numStruct, want: false},
		{name: "an interface, whose contents are not knowable", typ: types.NewInterfaceType(nil, nil), want: true},
		{name: "a signature is not text", typ: types.NewSignatureType(nil, nil, nil, nil, nil, false), want: false},
		{name: "nothing at all", typ: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := textual(tc.typ); got != tc.want {
				t.Errorf("textual(%v) = %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}

// TestTextual_SelfContainingType_Terminates checks the guard against a type
// that holds itself, which a formatter's tree-shaped output can be.
func TestTextual_SelfContainingType_Terminates(t *testing.T) {
	pkg := types.NewPackage("example.invalid/node", "node")
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Node", nil), nil, nil)
	named.SetUnderlying(types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, pkg, "Next", types.NewPointer(named), false),
		types.NewField(token.NoPos, pkg, "Count", types.Typ[types.Int], false),
	}, nil))

	if textual(named) {
		t.Error("a struct of numbers pointing at itself was read as text")
	}
}

// TestRendersAsInstant_Types_CoversTheTimePackageOnly checks which named types
// answer for themselves as a timestamp, since their fields never reach the page.
func TestRendersAsInstant_Types_CoversTheTimePackageOnly(t *testing.T) {
	timePkg := types.NewPackage("time", "time")
	other := types.NewPackage("example.invalid/other", "other")

	cases := []struct {
		name string
		typ  *types.Named
		want bool
	}{
		{name: "time.Time", typ: namedType(timePkg, "Time"), want: true},
		{name: "time.Duration", typ: namedType(timePkg, "Duration"), want: true},
		{name: "time.Month", typ: namedType(timePkg, "Month"), want: true},
		{name: "time.Weekday", typ: namedType(timePkg, "Weekday"), want: true},
		{name: "time.Timer, which is not one", typ: namedType(timePkg, "Timer"), want: false},
		{name: "another package's Time", typ: namedType(other, "Time"), want: false},
		{name: "a type belonging to no package", typ: namedType(nil, "Time"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rendersAsInstant(tc.typ); got != tc.want {
				t.Errorf("rendersAsInstant(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// namedType builds a named type over an empty struct, for the cases above.
func namedType(pkg *types.Package, name string) *types.Named {
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), nil, nil)
	named.SetUnderlying(types.NewStruct(nil, nil))
	return named
}

// TestQualifiedName_Functions_NamesTheReceiverToo checks the name a whitelist
// entry is matched against, so an entry cannot match the wrong method.
func TestQualifiedName_Functions_NamesTheReceiverToo(t *testing.T) {
	pkg := types.NewPackage("time", "time")
	instant := namedType(pkg, "Time")
	recv := types.NewVar(token.NoPos, pkg, "t", instant)
	pointerRecv := types.NewVar(token.NoPos, pkg, "t", types.NewPointer(instant))

	cases := []struct {
		name string
		fn   *types.Func
		want string
	}{
		{
			name: "a function",
			fn:   types.NewFunc(token.NoPos, pkg, "Now", types.NewSignatureType(nil, nil, nil, nil, nil, false)),
			want: "time.Now",
		},
		{
			name: "a method",
			fn:   types.NewFunc(token.NoPos, pkg, "Format", types.NewSignatureType(recv, nil, nil, nil, nil, false)),
			want: "time.Time.Format",
		},
		{
			name: "a method on a pointer",
			fn:   types.NewFunc(token.NoPos, pkg, "String", types.NewSignatureType(pointerRecv, nil, nil, nil, nil, false)),
			want: "time.Time.String",
		},
		{
			name: "a function belonging to no package",
			fn:   types.NewFunc(token.NoPos, nil, "Anonymous", types.NewSignatureType(nil, nil, nil, nil, nil, false)),
			want: ".Anonymous",
		},
		{
			// error is declared in the universe, so its method's receiver
			// renders as a bare name with no package in front of it. Trimming
			// at a dot that is not there would leave nothing of the receiver.
			name: "a method on a type the universe declares",
			fn: types.NewFunc(token.NoPos, nil, "Error", types.NewSignatureType(
				types.NewVar(token.NoPos, nil, "e", types.Universe.Lookup("error").Type()),
				nil, nil, nil, nil, false,
			)),
			want: ".error.Error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := qualifiedName(tc.fn); got != tc.want {
				t.Errorf("qualifiedName = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIsEscaperAndIsSprintf_Functions_KeyOnTheImportPath checks that a helper
// with the right name in the wrong package is not read as an escaper, which is
// what keys the resolution on the path rather than on the package name.
func TestIsEscaperAndIsSprintf_Functions_KeyOnTheImportPath(t *testing.T) {
	impostor := types.NewPackage("example.invalid/toolutil", "toolutil")
	genuine := types.NewPackage(toolutilPath, "toolutil")
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)

	if isEscaper(types.NewFunc(token.NoPos, impostor, "EscapeMdTableCell", signature)) {
		t.Error("a helper from another package was accepted as an escaper")
	}
	if !isEscaper(types.NewFunc(token.NoPos, genuine, "EscapeMdTableCell", signature)) {
		t.Error("the real escaper was not recognized")
	}
	if isEscaper(types.NewFunc(token.NoPos, nil, "EscapeMdTableCell", signature)) {
		t.Error("a function belonging to no package was accepted as an escaper")
	}
	if isSprintf(types.NewFunc(token.NoPos, impostor, "Sprintf", signature)) {
		t.Error("another package's Sprintf was accepted")
	}
	if !isSprintf(types.NewFunc(token.NoPos, types.NewPackage("fmt", "fmt"), "Sprintf", signature)) {
		t.Error("fmt.Sprintf was not recognized")
	}
	if isSprintf(types.NewFunc(token.NoPos, nil, "Sprintf", signature)) {
		t.Error("a function belonging to no package was accepted as fmt.Sprintf")
	}
	if isSprintf(types.NewFunc(token.NoPos, types.NewPackage("fmt", "fmt"), "Sprint", signature)) {
		t.Error("fmt.Sprint, which carries no template, was read as Sprintf")
	}
}

// TestPackageLevel_Variables_TellsAGlobalFromAField checks the rule that
// separates another package's variable from a field of a struct, which decides
// whether the audit keeps looking or starts reporting.
func TestPackageLevel_Variables_TellsAGlobalFromAField(t *testing.T) {
	pkg := types.NewPackage("example.invalid/x", "x")
	scope := types.NewScope(types.Universe, token.NoPos, token.NoPos, "package")
	global := types.NewVar(token.NoPos, pkg, "Global", types.Typ[types.String])
	scope.Insert(global)

	if !packageLevel(global) {
		t.Error("a package-level variable was not recognized as one")
	}
	if packageLevel(types.NewField(token.NoPos, pkg, "Title", types.Typ[types.String], false)) {
		t.Error("a struct field was read as a package-level variable")
	}

	// A local has a scope like a global does; what separates them is whose
	// scope it is, so the parent has to be compared rather than only tested
	// for being there.
	inner := types.NewScope(scope, token.NoPos, token.NoPos, "function")
	local := types.NewVar(token.NoPos, pkg, "local", types.Typ[types.String])
	inner.Insert(local)
	if packageLevel(local) {
		t.Error("a variable declared inside a function was read as a package-level variable")
	}
}
