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

// e2eBuildTag is the one build constraint every file of the e2e suite
// carries. The load states it itself, the way cmd/audit_e2e_coverage -static
// does, so neither make check-action-ids nor CI has to pass a flag the command
// would otherwise need and could be run without.
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
// after it alone.
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

// suiteRead is what the walk of the suite hands back: the quotations it
// found, how many times it met each declared helper, and the helpers whose
// function turned out to take no parameter of the declared name.
type suiteRead struct {
	sites      []site
	calls      map[string]int
	mismatches map[string]string
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

// suiteVariants picks one copy of each package to walk: the test variant
// where one was loaded, and the package itself where none was.
//
// A load with tests hands back a tested package twice, as itself and as the
// variant compiled with its tests, and a file that is not a test file is
// parsed into both as two separate trees. A site is one per expression, so
// walking both would read a call in such a file twice. The package itself
// holds nothing its variant does not, so it is the one dropped. The test main
// the go tool synthesizes is kept: it calls no helper, and telling it apart
// costs more than walking it does.
func suiteVariants(loaded []*packages.Package) []*packages.Package {
	tested := map[string]struct{}{}
	for _, pkg := range loaded {
		if pkg.ForTest != "" {
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
func (w *walker) visitSuite(node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.CallExpr:
		w.visitAssertionCall(typed, false)
	case *ast.UnaryExpr:
		if call, isCall := ast.Unparen(typed.X).(*ast.CallExpr); isCall && typed.Op == token.NOT {
			w.visitAssertionCall(call, true)
		}
	}
	return true
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
		w.mismatches[callee.Name()] = helper.param
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
// call, each with the table it sits in.
//
// A mismatch is named on any run, since one call is enough to show the
// function takes no parameter of the declared name. An entry nothing calls is
// named only by a run over the whole suite: over one package every helper
// that package does not use is called nowhere, and reporting it would be an
// answer about the patterns rather than about the table.
func staleHelpers(calls map[string]int, mismatches map[string]string, wholeSuite bool) []string {
	var stale []string
	for name, param := range mismatches {
		stale = append(stale, name+" takes no parameter named "+param+" (servedTextAssertions)")
	}
	if wholeSuite {
		for name := range servedTextAssertions {
			if calls[name] == 0 {
				stale = append(stale, name+" is called nowhere in the suite (servedTextAssertions)")
			}
		}
	}
	sort.Strings(stale)
	return stale
}
