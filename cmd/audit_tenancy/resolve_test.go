package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// resolveSource declares one of each shape a site can name: a function, a
// method on a pointer receiver, a method on a value receiver, a method of a
// generic type, a const, a var and a type.
const resolveSource = siteHeader + `
func Handle() {}

type Gate struct{}

func (g *Gate) Check() {}

func (g Gate) Busy() {}

type Set[T any] struct{ items []T }

func (s *Set[T]) Add(v T) { s.items = append(s.items, v) }

const Ceiling = 5

var Counter = 0
`

// TestCheckResolve_EveryShapeASiteNames_Resolves: a function, both receiver
// kinds, a generic type's method, a const, a var and a type all resolve, in
// every place a site can be named: a row, a refusal's text and literal, a
// reason, a key's derivation and a failure.
func TestCheckResolve_EveryShapeASiteNames_Resolves(t *testing.T) {
	d := row("ROW-001",
		site("Handle", tenancy.Enforce), site("Gate.Check", tenancy.Enforce), site("Gate.Busy", tenancy.Refuse),
		site("Set.Add", tenancy.Enforce), site("Ceiling", tenancy.Pin), site("Counter", tenancy.Enforce), site("Gate", tenancy.Reason))
	d.Refusals = []tenancy.Refusal{{At: site("Gate.Busy", tenancy.Refuse), Via: site("Handle", tenancy.Refuse)}}
	d.ReasonAt = site("Ceiling", tenancy.Reason)
	report := fixture{
		files:  map[string]string{"site/site.go": resolveSource},
		rows:   []tenancy.Decision{d},
		derive: []derivation{{key: "k", sites: []tenancy.Site{site("Handle", tenancy.Derive)}}},
		fails:  []tenancy.Failure{{Kind: "f", At: site("Gate.Check", tenancy.Charge)}},
	}.run(t)
	assertFindings(t, report, "G1")
}

// TestCheckResolve_ADeclarationThatMatchesNothing_IsAFinding: a site in a
// package the program does not hold, a name no declaration has, a method a
// type does not declare, a derivation and a failure naming nothing each fail,
// and each is named once however many times the register names it.
func TestCheckResolve_ADeclarationThatMatchesNothing_IsAFinding(t *testing.T) {
	gone := site("Gone", tenancy.Enforce)
	d := row("ROW-001", gone, gone, site("Gate.Missing", tenancy.Enforce), tenancy.Site{Pkg: "internal/nowhere", Name: "X"})
	report := fixture{
		files:  map[string]string{"site/site.go": resolveSource},
		rows:   []tenancy.Decision{d},
		derive: []derivation{{key: "k", sites: []tenancy.Site{site("Derived", tenancy.Derive)}}},
		fails:  []tenancy.Failure{{Kind: "f", At: site("Charged", tenancy.Charge)}},
	}.run(t)
	assertFindings(t, report, "G1",
		"ROW-001: names "+siteDir+":Gate.Missing, which matches nothing: "+siteDir+" declares nothing named Gate.Missing",
		"ROW-001: names "+siteDir+":Gone, which matches nothing: "+siteDir+" declares nothing named Gone",
		"ROW-001: names internal/nowhere:X, which matches nothing: package internal/nowhere is not in the loaded program",
		"failure f: names "+siteDir+":Charged, which matches nothing: "+siteDir+" declares nothing named Charged",
		"key k: names "+siteDir+":Derived, which matches nothing: "+siteDir+" declares nothing named Derived",
	)
}

// TestCheckResolve_AnExemption_MustNameSomethingAndNotASite: an exemption is
// held like a site, and one naming a declaration the register also names is
// refused, since a declaration is one or the other.
func TestCheckResolve_AnExemption_MustNameSomethingAndNotASite(t *testing.T) {
	report := fixture{
		files: map[string]string{"site/site.go": resolveSource},
		rows:  []tenancy.Decision{row("ROW-001", site("Ceiling", tenancy.Pin))},
		exempt: map[string]exemption{
			siteDir + ":Ceiling": {categoryParsing, "also a site"},
			siteDir + ":Nothing": {categoryParsing, "names nothing"},
		},
	}.run(t)
	assertFindings(t, report, "G1",
		siteDir+":Ceiling: exempted and also named by the register as a site; a declaration is one or the other",
		siteDir+":Nothing: exempted, and matches nothing: "+siteDir+" declares nothing named Nothing",
	)
}

// TestLeftOut_ReadsATreeAndAnExactPackage: "x/..." takes x and everything
// below it and nothing that merely shares its prefix; a bare entry takes that
// package alone.
func TestLeftOut_ReadsATreeAndAnExactPackage(t *testing.T) {
	leave := []string{"internal/testutil/...", "internal/freshness"}
	tests := []struct {
		dir  string
		want bool
	}{
		{"internal/testutil", true},
		{"internal/testutil/fixture", true},
		{"internal/testutilx", false},
		{"internal/freshness", true},
		{"internal/freshness/sub", false},
		{"cmd/server", false},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			if got := leftOut(tt.dir, leave); got != tt.want {
				t.Fatalf("leftOut(%q) = %t, want %t", tt.dir, got, tt.want)
			}
		})
	}
}

// TestNewProgram_LeavesOutWhatTheRulesMustNotRead: a package the rules leave
// out is neither indexed nor counted.
func TestNewProgram_LeavesOutWhatTheRulesMustNotRead(t *testing.T) {
	r := fixtureRules()
	r.leave = []string{siteDir}
	report := fixture{files: map[string]string{"site/site.go": resolveSource}, rules: &r}.run(t)
	if report.Summary.Packages != 1 {
		t.Fatalf("packages = %d, want only the leaf", report.Summary.Packages)
	}
}

// TestReceiverName_ReadsEveryReceiverShape: the base type name under a
// pointer, type parameters and parentheses, and nothing for a shape no
// receiver can have.
func TestReceiverName_ReadsEveryReceiverShape(t *testing.T) {
	gate := ast.NewIdent("Gate")
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{"plain", gate, "Gate"},
		{"pointer", &ast.StarExpr{X: gate}, "Gate"},
		{"one type parameter", &ast.IndexExpr{X: gate}, "Gate"},
		{"two type parameters", &ast.IndexListExpr{X: gate}, "Gate"},
		{"parenthesized", &ast.ParenExpr{X: &ast.StarExpr{X: gate}}, "Gate"},
		{"not a receiver", &ast.ArrayType{Elt: gate}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := receiverName(tt.expr); got != tt.want {
				t.Fatalf("receiverName = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestObjectName_NamesAFunctionAndAMethod: a method is named by its
// receiver's type, whichever receiver kind, and a method whose receiver is no
// named type (an interface literal's) by its own name alone.
func TestObjectName_NamesAFunctionAndAMethod(t *testing.T) {
	pkg := types.NewPackage("example.com/p", "p")
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Gate", nil), types.NewStruct(nil, nil), nil)
	method := func(recv types.Type) *types.Func {
		return types.NewFunc(token.NoPos, pkg, "Check", types.NewSignatureType(types.NewVar(token.NoPos, pkg, "g", recv), nil, nil, nil, nil, false))
	}
	tests := []struct {
		name string
		fn   *types.Func
		want string
	}{
		{"function", types.NewFunc(token.NoPos, pkg, "Handle", types.NewSignatureType(nil, nil, nil, nil, nil, false)), "Handle"},
		{"pointer receiver", method(types.NewPointer(named)), "Gate.Check"},
		{"value receiver", method(named), "Gate.Check"},
		{"unnamed receiver", method(types.NewInterfaceType(nil, nil)), "Check"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := objectName(tt.fn); got != tt.want {
				t.Fatalf("objectName = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestObjectKey_NamesOnlyThisModulesPackageLevelObjects: a key is given to a
// package-level object or a method of this module, and to nothing else: not
// to another module's objects, a local, a field or a builtin.
func TestObjectKey_NamesOnlyThisModulesPackageLevelObjects(t *testing.T) {
	ours := types.NewPackage(sitePath, "site")
	theirs := types.NewPackage("golang.org/x/time/rate", "rate")
	pkgConst := types.NewConst(token.NoPos, ours, "Ceiling", types.Typ[types.UntypedInt], nil)
	ours.Scope().Insert(pkgConst)
	local := types.NewVar(token.NoPos, ours, "n", types.Typ[types.Int])
	types.NewScope(ours.Scope(), token.NoPos, token.NoPos, "f").Insert(local)
	tests := []struct {
		name string
		obj  types.Object
		want string
	}{
		{"nil", nil, ""},
		{"package-level const", pkgConst, siteDir + ":Ceiling"},
		{"function", types.NewFunc(token.NoPos, ours, "Handle", types.NewSignatureType(nil, nil, nil, nil, nil, false)), siteDir + ":Handle"},
		{"another module", types.NewConst(token.NoPos, theirs, "Inf", types.Typ[types.Float64], nil), ""},
		{"local", local, ""},
		{"field", types.NewField(token.NoPos, ours, "status", types.Typ[types.Int], false), ""},
		{"builtin", types.Universe.Lookup("true"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := objectKey(tt.obj); got != tt.want {
				t.Fatalf("objectKey = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPosition_FallsBackToTheFileWhenItIsNotUnderTheRoot: a file the root
// cannot be made relative to is named as the loader named it.
func TestPosition_FallsBackToTheFileWhenItIsNotUnderTheRoot(t *testing.T) {
	fset := token.NewFileSet()
	file := fset.AddFile("/abs/site.go", -1, 10)
	file.SetLines([]int{0, 5})
	p := &program{root: "relative", fset: fset}
	if got := p.position(file.Pos(6)); got != "/abs/site.go:2" {
		t.Fatalf("position = %q, want the absolute file and its line", got)
	}
}

// TestDeclaration_BodyAndComments_ForEveryShape: a function's body and doc,
// a value's initializer and its spec's and block's docs, a type's docs and no
// body, and no body for a value whose spec gives it no initializer of its own.
func TestDeclaration_BodyAndComments_ForEveryShape(t *testing.T) {
	doc := &ast.CommentGroup{List: []*ast.Comment{{Text: "// doc"}}}
	body := &ast.BlockStmt{}
	value := &ast.BasicLit{Kind: token.INT, Value: "1"}
	names := []*ast.Ident{ast.NewIdent("a"), ast.NewIdent("b")}
	tests := []struct {
		name     string
		decl     *declaration
		wantBody ast.Node
		wantDocs int
	}{
		{"function", &declaration{fn: &ast.FuncDecl{Doc: doc, Body: body}}, body, 1},
		{"value", &declaration{gen: &ast.GenDecl{Doc: doc}, value: &ast.ValueSpec{Names: names[:1], Values: []ast.Expr{value}, Doc: doc}}, value, 2},
		{"value repeated from the spec above", &declaration{gen: &ast.GenDecl{}, value: &ast.ValueSpec{Names: names[:1]}}, nil, 2},
		{"several names from one call", &declaration{gen: &ast.GenDecl{}, value: &ast.ValueSpec{Names: names, Values: []ast.Expr{value}}, index: 1}, nil, 2},
		{"type", &declaration{gen: &ast.GenDecl{Doc: doc}, typ: &ast.TypeSpec{Doc: doc}}, nil, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.decl.body(); got != tt.wantBody {
				t.Fatalf("body = %v, want %v", got, tt.wantBody)
			}
			if got := tt.decl.comments(); len(got) != tt.wantDocs {
				t.Fatalf("comments = %d groups, want %d", len(got), tt.wantDocs)
			}
		})
	}
}

// TestDeclaration_Where_NamesTheDeclaredName: the position of a function's,
// a value's and a type's name.
func TestDeclaration_Where_NamesTheDeclaredName(t *testing.T) {
	p := programOf(t, fixture{files: map[string]string{"site/site.go": resolveSource}})
	for key, line := range map[string]string{
		siteDir + ":Handle":  "func Handle() {}",
		siteDir + ":Ceiling": "const Ceiling = 5",
		siteDir + ":Gate":    "type Gate struct{}",
	} {
		t.Run(key, func(t *testing.T) {
			want := fmt.Sprintf("%s/site/site.go:%d", fixtureDir, lineOf(t, resolveSource, line))
			if got := p.decls[key].where(p); got != want {
				t.Fatalf("where = %q, want %q", got, want)
			}
		})
	}
}

// programOf loads a fixture's program, for the cases that inspect it.
func programOf(t *testing.T, f fixture) *program {
	t.Helper()
	p, err := loadProgram(f.config(t))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return p
}

// lineOf is the 1-based line of source that line is.
func lineOf(t *testing.T, source, line string) int {
	t.Helper()
	for i, l := range strings.Split(source, "\n") {
		if l == line {
			return i + 1
		}
	}
	t.Fatalf("no line %q in the fixture", line)
	return 0
}

// TestForEachTopLevel_KeysEveryPieceOfCode: a function body under its name,
// each initializer under its own name, one call initializing several names
// under the first, and nothing for a type or a function without a body.
func TestForEachTopLevel_KeysEveryPieceOfCode(t *testing.T) {
	source := siteHeader + `
func pair() (int, int) { return 1, 2 }

func Handle() { _ = 1 }

var (
	a, b = pair()
	c    = 3
	d    int
)

type T struct{}
`
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source}})
	var keys []string
	p.forEachTopLevel(func(key string, _ *types.Info, _ ast.Node) {
		if strings.HasPrefix(key, siteDir+":") && key != siteDir+":_" {
			keys = append(keys, key)
		}
	})
	slices.Sort(keys)
	want := []string{siteDir + ":Handle", siteDir + ":a", siteDir + ":c", siteDir + ":pair"}
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

// TestCalleeOf_NamesWhatACallStaticallyReaches: a function, a method, a
// generic function with one and two type arguments, and nothing for a call
// through a value or a conversion.
func TestCalleeOf_NamesWhatACallStaticallyReaches(t *testing.T) {
	source := siteHeader + `
type T struct{ fn func() }

func (T) M() {}

func One[A any](a A) {}

func Two[A, B any](a A, b B) {}

func Calls(t T) {
	One(1)
	One[int](1)
	Two[int, string](1, "")
	t.M()
	t.fn()
	_ = int64(1)
}
`
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source}})
	decl := p.decls[siteDir+":Calls"]
	var got []string
	ast.Inspect(decl.fn.Body, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			got = append(got, calleeName(calleeOf(decl.info(), call)))
		}
		return true
	})
	want := []string{sitePath + ".One", sitePath + ".One", sitePath + ".Two", sitePath + ".T.M", "", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("callees = %q, want %q", got, want)
	}
}

// TestReferenced_NamesAnIdentifierOrASelector: an identifier, a qualified
// identifier and a field selection each name an object, and anything else
// names none.
func TestReferenced_NamesAnIdentifierOrASelector(t *testing.T) {
	source := siteHeader + `
type T struct{ f int }

const K = 1

var (
	ident     = K
	qualified = leaf.Limit
	field     = T{}.f
	other     = K + 1
)
`
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source}})
	for name, want := range map[string]string{"ident": "K", "qualified": "Limit", "field": "f", "other": ""} {
		t.Run(name, func(t *testing.T) {
			decl := p.decls[siteDir+":"+name]
			got := ""
			if obj := referenced(decl.info(), decl.initializer()); obj != nil {
				got = obj.Name()
			}
			if got != want {
				t.Fatalf("referenced = %q, want %q", got, want)
			}
		})
	}
}

// TestSortedKeys_OrdersAMapsKeys is the order every table is reported in.
func TestSortedKeys_OrdersAMapsKeys(t *testing.T) {
	if got := sortedKeys(map[string]int{"b": 1, "a": 2}); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("sortedKeys = %v", got)
	}
}
