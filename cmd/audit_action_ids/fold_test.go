package main

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestFoldCall_DomainPrefixHelper_IsFolded is the case this folding exists
// for. internal/tools/runnercontrollertokens writes every cross-link as
// canonicalID(actionNameTokenGet), where canonicalID puts a domain constant in
// front of a name; a scan over literals sees a bare action name with no domain
// and reports five phantoms that are not there.
func TestFoldCall_DomainPrefixHelper_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const (
	domain    = "demo"
	nameGet   = "get"
	nameList  = "list"
)

func canonicalID(name string) string { return domain + "." + name }

func qualified(name string) string { return canonicalID(name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID(nameGet), qualified(nameList)}}
}
`)

	want := []string{"demo.get", "demo.list"}
	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, want) {
		t.Errorf("related values = %v, want %v", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none: one helper calls the other", got)
	}
}

// TestFoldCall_TheDeepestChainTheBoundAllows_IsFolded holds the bound from
// below: three helpers handing the value on is what maxFoldDepth admits, and
// the tree's own shapes sit inside it. Nothing else here folds deeper than
// two, so a bound one step tighter would have gone unnoticed.
func TestFoldCall_TheDeepestChainTheBoundAllows_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func third(name string) string  { return domain + "." + name }
func second(name string) string { return third(name) }
func first(name string) string  { return second(name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{first("get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("related values = %v, want demo.get folded three helpers deep", got)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none inside the bound", got)
	}
}

// TestFoldCall_PastTheBound_IsReportedRatherThanFolded holds the bound from
// above, in both directions a fold descends: a chain of helpers each handing
// the value to the next, and a call whose own argument is a nest of calls.
// The bound is what stops a pair of helpers that call each other running
// forever, and a fold that stopped counting would fold both of these instead
// of naming what it could not read.
func TestFoldCall_PastTheBound_IsReportedRatherThanFolded(t *testing.T) {
	t.Run("a chain of helpers handing the value on", func(t *testing.T) {
		sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func fourth(name string) string { return domain + "." + name }
func third(name string) string  { return fourth(name) }
func second(name string) string { return third(name) }
func first(name string) string  { return second(name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{first("get")}}
}
`)

		if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
			t.Errorf("related values = %v, want nothing folded past the bound", got)
		}
		if got := unresolvedExprs(sites); !slices.Equal(got, []string{`first("get")`}) {
			t.Errorf("unresolved = %v, want the outermost call named", got)
		}
	})

	t.Run("a nest of calls in the argument", func(t *testing.T) {
		sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func canonicalID(name string) string { return domain + "." + name }
func inner(name string) string       { return name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID(inner(inner(inner("get"))))}}
}
`)

		if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
			t.Errorf("related values = %v, want nothing folded past the bound", got)
		}
		if got := unresolvedExprs(sites); len(got) != 1 {
			t.Errorf("unresolved = %v, want the outermost call named once", got)
		}
	})
}

// TestFoldCall_ASignatureTheArgumentsDoNotFill_IsNotFolded holds the pairing
// a fold rests on: a parameter is bound to the argument written at its own
// index, so a call that passes a different number of arguments than the
// helper declares cannot be folded by binding what happens to line up. A
// variadic helper is the shape that reaches it, since its declared parameters
// and its arguments differ by construction.
func TestFoldCall_ASignatureTheArgumentsDoNotFill_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func variadic(name string, rest ...string) string { return domain + "." + name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{variadic("get", "one", "two")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a signature the call does not fill", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_ABodyThatIsNoReturn_IsNotFolded holds the other half of "one
// return of one expression": a body of one statement that is not a return at
// all. The branching helper above is refused for having two statements and so
// never reaches this rule, which used to be held by nothing.
func TestFoldCall_ABodyThatIsNoReturn_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func panicking(name string) string { panic(name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{panicking("demo.get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a body that returns nothing", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_AReturnWithStatementsAfterIt_IsNotFolded holds the count in
// "one return of one expression". A body whose first statement is a foldable
// return and whose second is anything at all is refused, because a helper
// this rule cannot read whole is one whose value it is guessing at.
func TestFoldCall_AReturnWithStatementsAfterIt_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func trailing(name string) string {
	return domain + "." + name
	panic(name)
}

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{trailing("get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a body of two statements", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_BranchingHelper_IsNotFolded holds the one limit on folding. A
// helper that returns different strings on different paths has no single
// value, and picking one of them would be a guess reported as a fact.
func TestFoldCall_BranchingHelper_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

func branching(project bool) string {
	if project {
		return "demo.project_get"
	}
	return "demo.get"
}

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{branching(true)}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a branching helper", got)
	}
	got := unresolvedExprs(sites)
	if len(got) != 1 || !strings.Contains(got[0], "branching") {
		t.Errorf("unresolved = %v, want the branching call named", got)
	}
}

// TestFoldCall_OneHalfOfTheConcatenationUnknown_IsNotFolded holds that a
// concatenation is folded only when both halves are, since half an ID is a
// phantom: reporting "demo.get" from a helper whose prefix came from the
// environment would name an action nobody published.
func TestFoldCall_OneHalfOfTheConcatenationUnknown_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func joined(prefix, name string) string { return prefix + name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{joined(os.Getenv("PREFIX"), "demo.get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from half a concatenation", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_NonConstantArgument_IsNotFolded holds that a helper called with
// a value only known at run time is reported rather than folded to the prefix
// it would have carried.
func TestFoldCall_NonConstantArgument_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const domain = "demo"

func canonicalID(name string) string { return domain + "." + name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID(os.Getenv("DEMO"))}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_NonConcatenatingExpression_IsNotFolded holds that only the
// shapes a concatenation can take are folded here. Anything else is left to
// the type checker, which already answered no.
func TestFoldCall_NonConcatenatingExpression_IsNotFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const domain = "demo"

func canonicalID(name string) string { return strings.Join([]string{domain, name}, ".") }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID("get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); len(got) != 0 {
		t.Errorf("related values = %v, want nothing folded from a join", got)
	}
	if got := unresolvedExprs(sites); len(got) != 1 {
		t.Errorf("unresolved = %v, want the call named once", got)
	}
}

// TestFoldCall_ArgumentsThatBindNothing_AreSkipped holds that an argument
// which is no string constant binds its parameter to nothing and stops
// neither the fold nor the arguments beside it: a function handed to a helper
// and an arithmetic expression are both values a binding has no spelling for,
// and the name beside them is still read.
func TestFoldCall_ArgumentsThatBindNothing_AreSkipped(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func build() string { return "unused" }

func withOptions(read func() string, count int, name string) string { return domain + "." + name }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{withOptions(build, 4-2, "get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("related values = %v, want demo.get folded from the one argument that binds", got)
	}
}

// TestFoldCall_ParenthesizedConcatenation_IsFolded holds the one shape the
// parser leaves in the way of an otherwise constant expression.
func TestFoldCall_ParenthesizedConcatenation_IsFolded(t *testing.T) {
	sites := collectFixture(t, `package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

const domain = "demo"

func canonicalID(name string) string { return (domain + ".") + (name) }

func options() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{RelatedActions: []string{canonicalID("get")}}
}
`)

	if got := valuesOfKind(sites, kindRelated); !slices.Equal(got, []string{"demo.get"}) {
		t.Errorf("related values = %v, want demo.get", got)
	}
}

// TestSingleReturnExpr_ABodyTheFoldNeverSees_IsRefused holds the two shapes a
// one-line helper is not, asked of the function directly rather than through a
// fixture.
//
// A declaration with no body at all is the one the walk cannot arrange: a Go
// package whose function has no body and no assembly beside it does not type
// check, and the loader refuses such a package before this is ever called. So
// the guard is real and only a direct call can reach it, which is the same
// bargain the loader's own error branches make.
func TestSingleReturnExpr_ABodyTheFoldNeverSees_IsRefused(t *testing.T) {
	cases := map[string]*ast.FuncDecl{
		"no body at all":                {},
		"a body of two":                 {Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{}, &ast.ReturnStmt{}}}},
		"a statement that is no return": {Body: &ast.BlockStmt{List: []ast.Stmt{&ast.EmptyStmt{}}}},
		"a return of two values": {Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("a"), ast.NewIdent("b")}},
		}}},
	}
	for name, decl := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := singleReturnExpr(decl); ok {
				t.Errorf("singleReturnExpr(%s) folded, want refused", name)
			}
		})
	}
}

// TestConstantStringIn_AnExpressionTheCheckerNeverTyped_IsNoConstant holds the
// lookup the fold makes before it reads a value. Every expression a loaded
// package carries is in that map, so the miss is reachable only by handing the
// function an expression from nowhere, which is what this does.
func TestConstantStringIn_AnExpressionTheCheckerNeverTyped_IsNoConstant(t *testing.T) {
	pkg := &packages.Package{TypesInfo: &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}}
	if value, ok := constantStringIn(pkg, &ast.BasicLit{}); ok {
		t.Errorf("constantStringIn of an untyped expression = %q, true; want no value", value)
	}
}
