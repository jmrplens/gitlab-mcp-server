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
			name: "a variadic parameter no caller fills", pkg: "mdcase",
			expression: "rest[0]", want: unresolved, reason: "shape the audit does not follow",
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

	cases := []struct {
		name   string
		expr   ast.Expr
		depth  int
		reason string
	}{
		{name: "deeper than the walk follows", expr: expr, depth: maxDepth + 1, reason: "deeper than the audit follows"},
		{name: "nothing to judge", expr: nil, reason: "an assignment the audit does not follow"},
		{name: "a shape with no rule", expr: &ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}, reason: "a shape the audit does not follow"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := c.classifyExpr(pkg, tc.expr, nil, tc.depth)
			if got != unresolved {
				t.Errorf("classifyExpr = %v, want unresolved", got)
			}
			if !strings.Contains(why, tc.reason) {
				t.Errorf("reason %q does not say %q", why, tc.reason)
			}
		})
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

// TestClassifyParamField_ShortCall_IsUnresolved checks the guard on a call
// that passes fewer arguments than the parameter being asked about, which a
// variadic signature makes possible.
func TestClassifyParamField_ShortCall_IsUnresolved(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	c := newClassifier(prog)
	ref, ok := paramNamed(c, "FormatVariadic", "rest")
	if !ok {
		t.Fatal("the fixture's variadic parameter was not indexed")
	}

	got, why := c.classifyParamField(ref, "Title", 0)

	if got != unresolved || !strings.Contains(why, "shape the audit does not follow") {
		t.Errorf("classifyParamField = %v (%s), want unresolved", got, why)
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
		{name: "a map of numbers", typ: types.NewMap(num, num), want: false},
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
}
