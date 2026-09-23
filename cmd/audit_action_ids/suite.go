package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// e2eBuildTag is the one build constraint every test file of the e2e suite
// carries (the package doc.go files carry none). The load states it itself,
// the way cmd/audit_e2e_coverage -static does, so neither make
// check-action-ids nor CI has to pass a flag the command would otherwise need
// and could be run without.
const e2eBuildTag = "e2e"

// suiteDir is where the e2e suite lives, relative to the repository root. A
// helper is matched only when the function a call resolves to is declared in
// a package under it, so a function of the same name anywhere else in the
// module is not one; see [inSuite].
const suiteDir = "test/e2e/"

// inSuite reports whether an import path names a package under suiteDir.
//
// The prefix is assembled here rather than held in a constant beside suiteDir,
// because a constant expression is evaluated by the compiler and no test run
// executes it, so mutation testing reports every operator in one as
// uncovered whatever the tests assert.
func inSuite(pkgPath string) bool {
	return strings.HasPrefix(pkgPath, goprogram.ModulePath+"/"+suiteDir)
}

// assertionHelper is one function the suite asserts served text through: the
// parameter the asserted substring is passed in, and whether the function is
// a predicate the caller branches on rather than an assertion that fails the
// test itself.
type assertionHelper struct {
	param     string
	predicate bool
}

// servedTextAssertions are the suite's helpers that hold a served text to a
// substring, keyed by function name, each with the parameter that substring
// is passed in.
//
// A name rather than a package and a name, because the suite declares these
// per package: assertMentions is written in common and again in ee, and a
// copy added to ce is read with no entry of its own. The argument is found by
// the parameter's name in the callee's own signature rather than by position,
// which is what reads ExpectToolError's one contains and leaves the options
// after it alone. The price is that one entry names the parameter of every
// copy, so a copy whose parameter differs is named with its package (see
// [helperCopy]): no edit of the table can agree with two copies at once.
//
// A predicate is judged only where its call is negated. `if !mentionsAny(...)`
// fails the test when none of the needles is there, so each needle is a claim
// about what the server wrote; `if containsAny(...)` is the opposite claim, an
// absence check or a classification, and a tool name there is exactly what a
// test asserting that a refusal leaks nothing has to spell.
//
// The table is held to the suite on the terms every declaration table here is
// held to. A run over the whole suite reports an entry nothing calls, which is
// what a renamed helper looks like from here, and any run reports an entry
// whose function has no parameter of the declared name, which is what a
// renamed parameter looks like. Either would otherwise stop the reading of
// every call it declares without a word.
var servedTextAssertions = map[string]assertionHelper{
	"assertMentions":  {param: "substrings"},
	"ExpectToolError": {param: "contains"},
	"mentionsAny":     {param: "substrings", predicate: true},
	"containsAny":     {param: "needles", predicate: true},
}

// isAssertionParamName reports whether a parameter carries the name one of the
// declared helpers takes its substrings under.
func isAssertionParamName(name string) bool {
	for _, helper := range servedTextAssertions {
		if helper.param == name {
			return true
		}
	}
	return false
}

// helperCopy is one declaration of a table helper: the package that declares
// it, as the repository names it, and its name.
//
// A mismatch is a fact about one copy and not about the name. assertMentions
// is declared in common and again in ee, both under the one entry, so a copy
// whose parameter was renamed is the copy that has to change, and a row
// naming the helper alone would say neither which copy that is nor that the
// entry is not the thing to edit.
type helperCopy struct {
	pkg  string
	name string
}

// suiteRead is what the walk of the suite hands back: the quotations it
// found, how many times it met each declared helper, and the copies of a
// helper whose function turned out to take no parameter of the declared name.
type suiteRead struct {
	sites      []site
	calls      map[string]int
	mismatches map[helperCopy]string
}

// collectAssertionSites loads the e2e suite named by patterns, rooted at dir,
// and returns every substring it asserts a served text carries.
//
// The load is the one cmd/audit_e2e_coverage -static makes: the test variants
// under the e2e tag, since the suite is nothing but _test.go files behind
// that tag. The overlay is how a test plants a suite that is not on disk.
func collectAssertionSites(dir string, patterns []string, overlay map[string][]byte) (suiteRead, error) {
	loaded, err := goprogram.LoadWith(dir, patterns, goprogram.Options{
		Tests: true, BuildTags: []string{e2eBuildTag}, Overlay: overlay,
	})
	if err != nil {
		return suiteRead{}, err
	}
	collect, err := newCollector(dir, suiteVariants(loaded))
	if err != nil {
		return suiteRead{}, err
	}
	collect.walk((*walker).visitSuite)
	return suiteRead{sites: collect.sites, calls: collect.calls, mismatches: collect.mismatches}, nil
}

// suiteVariants picks one copy of each package to walk: the internal test
// variant where one was loaded, and the package itself where none was.
//
// A load with tests hands back a package with internal tests twice, as itself
// and as the variant compiled with those tests, and a file that is not a test
// file is parsed into both as two separate trees. A site is one per
// expression, so walking both would read a call in such a file twice. The
// package itself holds nothing its internal variant does not, so it is the one
// dropped.
//
// Only an internal variant displaces it, which is the one whose path is the
// path of the package it tests. An external test package (package p_test)
// names p in ForTest too, but it compiles only its own files, and go list
// builds no internal variant for a package whose tests are all external, so
// there the package itself is the only copy of its non-test files and
// dropping it would leave every helper call in them unread without a word.
// The external package and the test main the go tool synthesizes are kept:
// the first holds files nothing else compiles, and the second calls no helper
// and would cost more to tell apart than to walk.
func suiteVariants(loaded []*packages.Package) []*packages.Package {
	tested := map[string]struct{}{}
	for _, pkg := range loaded {
		if pkg.ForTest != "" && pkg.PkgPath == pkg.ForTest {
			tested[pkg.ForTest] = struct{}{}
		}
	}
	var kept []*packages.Package
	for _, pkg := range loaded {
		if _, hasVariant := tested[pkg.PkgPath]; hasVariant && pkg.ForTest == "" {
			continue
		}
		kept = append(kept, pkg)
	}
	return kept
}

// visitSuite dispatches the two shapes an assertion helper is called in: a
// call, and a call negated by `!`, which is the only position a predicate's
// needles are claims in.
//
// ast.Inspect reaches a negated call twice, once as the operand of the `!`
// and once as a call, which is what lets each visit do one job: the call is
// counted where it is met as a call, and judged where its polarity is known.
//
// A negation inside what a return statement hands back is not judged: there
// the predicate's answer, negated or not, is the wrapper's answer, and
// whether its needles are claims is decided where the wrapper is called, as
// it is for a wrapper returning the answer unnegated. It is marked when the
// return is met, which ast.Inspect does before it reaches the operands.
func (w *walker) visitSuite(node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.ReturnStmt:
		w.markReturnedNegations(typed)
	case *ast.CallExpr:
		w.visitAssertionCall(typed, false)
	case *ast.UnaryExpr:
		if _, isReturned := w.returned[typed]; isReturned {
			return true
		}
		if call, isCall := ast.Unparen(typed.X).(*ast.CallExpr); isCall && typed.Op == token.NOT {
			w.visitAssertionCall(call, true)
		}
	}
	return true
}

// markReturnedNegations marks every unary expression a return statement hands
// back as its answer, so a negated predicate there is not read as a claim.
//
// A function literal inside the result is not part of the answer: it is code
// that runs when called, and a negated predicate in its body is in the
// position every other negation is. Only a `!` is acted on, so marking the
// other unary operators too costs nothing and asks no question.
func (w *walker) markReturnedNegations(ret *ast.ReturnStmt) {
	for _, result := range ret.Results {
		ast.Inspect(result, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.FuncLit:
				return false
			case *ast.UnaryExpr:
				w.returned[typed] = struct{}{}
			}
			return true
		})
	}
}

// visitAssertionCall records what one call of a declared helper asserts.
//
// The parameter is looked up before the polarity is asked about, so a helper
// whose parameter was renamed is named on every call of it, and not only on
// the calls whose polarity would have been read.
func (w *walker) visitAssertionCall(call *ast.CallExpr, negated bool) {
	callee, ok := w.callee(call)
	if !ok || callee.Pkg() == nil || !inSuite(callee.Pkg().Path()) {
		return
	}
	helper, declared := servedTextAssertions[callee.Name()]
	if !declared {
		return
	}
	if !negated {
		w.calls[callee.Name()]++
	}
	signature := callee.Signature()
	index := paramIndex(signature.Params(), helper.param)
	if index < 0 {
		w.mismatches[helperCopy{pkg: trimModulePath(callee.Pkg().Path()), name: callee.Name()}] = helper.param
		return
	}
	if helper.predicate != negated {
		return
	}
	// A call whose arguments stop short of the parameter has handed over one
	// multi-valued call for its whole argument list, which is the case this
	// guard is for; a variadic helper called with no needle at all lands here
	// too, and is reported with it, since a call asserting nothing is as worth
	// a look as one this walk cannot read.
	if index >= len(call.Args) {
		w.recordUnresolved(kindAssertion, call)
		return
	}
	if signature.Variadic() && index == signature.Params().Len()-1 {
		w.recordErrorHintArgs(kindAssertion, call, index)
		return
	}
	// A helper may take its needles as a []string of its own rather than as a
	// variadic tail, and the argument is then one list. Recorded as one needle
	// it would fold to nothing, and a needle nothing folds fails nothing, so
	// every quotation handed to such a helper would pass unread.
	if isStringSlice(signature.Params().At(index).Type()) {
		w.recordHintList(kindAssertion, call.Args[index])
		return
	}
	w.recordErrorHint(kindAssertion, call.Args[index])
}

// paramIndex is the position of the named parameter, or -1 when the signature
// has none by that name.
func paramIndex(params *types.Tuple, name string) int {
	for index := range params.Len() {
		if params.At(index).Name() == name {
			return index
		}
	}
	return -1
}

// staleHelpers names the entries of [servedTextAssertions] that describe no
// call, each with the table it sits in and what would make it describe one.
//
// A mismatch is named on any run, since one call is enough to show the
// function takes no parameter of the declared name. It is named with the
// package of the copy that disagrees, and its remedy is not the entry's: the
// entry holds one parameter for every copy of the helper, so it is this
// copy's parameter that has to take the entry's name, unless the entry is
// renamed together with every copy.
//
// An entry nothing calls is named only by a run over the whole suite: over
// one package every helper that package does not use is called nowhere, and
// reporting it would be an answer about the patterns rather than about the
// table. That one is the entry's to fix, renamed with its helper or removed.
func staleHelpers(calls map[string]int, mismatches map[helperCopy]string, wholeSuite bool) []string {
	var stale []string
	for declared, param := range mismatches {
		stale = append(stale, declared.pkg+": "+declared.name+" takes no parameter named "+param+
			" (servedTextAssertions). The entry names one parameter for every copy of "+declared.name+
			", so rename this copy's parameter to "+param+", or the entry and every copy together")
	}
	if wholeSuite {
		for name := range servedTextAssertions {
			if calls[name] == 0 {
				stale = append(stale, name+" is called nowhere in the suite (servedTextAssertions). Fix the entry")
			}
		}
	}
	sort.Strings(stale)
	return stale
}
