// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkConfig is G14, INV-017's central clause: a setting this project
// defines is read the way the configuration package reads one.
//
// It holds three things. A variable a row names is, without the prefix, on
// the configuration package's list of prefixed names, and nothing in the
// program reads it directly; a row with a variable that fails either carries
// the finding that records it, and a row carrying that finding while every
// variable passes has a finding that no longer describes the tree. Every
// variable carrying the prefix that the program reads directly is answered,
// whether or not a row names it (issue 957): by the row that names it, held
// as above, or by a declaration that says why. And every name without the
// prefix that the configuration package reads bare, because the list does
// not hold it, is declared as somebody else's, since a setting of this
// project's that the list left out is read that way too, under a name no
// operator sets. A declaration that answers nothing, or that names a
// variable a row names, is a finding as well.
//
// A variable is read directly when the name it is read under carries the
// prefix: os.Getenv, os.LookupEnv and their syscall twins read it as given,
// os.ExpandEnv reads every variable its template names, and so does any
// function of the program that hands one of its own parameters on to one of
// those as the name, which config.Getenv does for a name outside its list. A
// read through config.Getenv, or through a function that hands it the name,
// config.TrimmedGetenv among them, reads a suffix the list holds under the
// prefix, which keeps it inside the rule, and any other name without the
// prefix bare.
//
// Validate cannot hold this, because the register imports nothing and so
// cannot see the list or the calls. The list is read only when a row names a
// variable, the configuration package is handed a name without the prefix
// or a name is declared as read bare; otherwise there is nothing to hold to
// it.
func (g *gate) checkConfig() []Finding {
	passes := g.parameterPasses()
	direct := g.directEnvReads(passes)
	bare := g.bareEnvReads(passes)
	found := g.checkUnnamedReads(direct)
	if len(bare) == 0 && len(g.rules.envBare) == 0 &&
		!slices.ContainsFunc(g.reg.decisions, func(d tenancy.Decision) bool { return len(d.Envs) > 0 }) {
		return found
	}
	names, ok := g.prefixedNames()
	if !ok {
		return append(found, Finding{
			Rule: "G14", Subject: keyOf(g.rules.envNames),
			Message: "is not a package-level list of string constants, so no setting can be checked against it",
		})
	}
	found = append(found, g.checkRowSettings(names, direct)...)
	return append(found, g.checkBareReads(names, bare)...)
}

// checkRowSettings is the half of G14 that holds the variables the rows name.
func (g *gate) checkRowSettings(names []string, readDirectly map[string][]string) []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		var failing []string
		for _, env := range d.Envs {
			g.read.settings++
			suffix, prefixed := strings.CutPrefix(env, g.rules.envPrefix)
			if !prefixed || !slices.Contains(names, suffix) {
				failing = append(failing, env+" is not on "+keyOf(g.rules.envNames))
				continue
			}
			if len(readDirectly[env]) > 0 {
				failing = append(failing, env+" is read directly at "+strings.Join(readDirectly[env], ", "))
			}
		}
		// A row without the finding answers for each failing variable, and
		// one with it must still have a variable the finding describes.
		if !d.Carries(g.rules.configFinding) {
			for _, why := range failing {
				found = append(found, Finding{
					Rule: "G14", Subject: d.ID,
					Message: fmt.Sprintf("%s, and the row does not carry %s", why, g.rules.configFinding),
				})
			}
			continue
		}
		if len(failing) == 0 {
			found = append(found, Finding{
				Rule: "G14", Subject: d.ID,
				Message: fmt.Sprintf("carries %s, and every variable it names is read through the configuration package", g.rules.configFinding),
			})
		}
	}
	return found
}

// checkUnnamedReads is the half of G14 that holds the variables no row names:
// each one read directly is answered by a declaration, and each declaration
// by a variable read directly that no row names.
func (g *gate) checkUnnamedReads(readDirectly map[string][]string) []Finding {
	namedBy := map[string]string{}
	for _, d := range g.reg.decisions {
		for _, env := range d.Envs {
			namedBy[env] = d.ID
		}
	}
	var found []Finding
	for _, env := range sortedKeys(readDirectly) {
		_, declared := g.rules.envDeclared[env]
		if _, named := namedBy[env]; named || declared {
			continue
		}
		found = append(found, Finding{
			Rule: "G14", Subject: env,
			Message: fmt.Sprintf("is read directly at %s, and no row names it and no declaration answers it", strings.Join(readDirectly[env], ", ")),
		})
	}
	for _, env := range sortedKeys(g.rules.envDeclared) {
		if id, named := namedBy[env]; named {
			found = append(found, Finding{
				Rule: "G14", Subject: env,
				Message: fmt.Sprintf("is declared as read around the configuration package, and %s names it: a row's variable is its row's to answer", id),
			})
			continue
		}
		if len(readDirectly[env]) == 0 {
			found = append(found, Finding{
				Rule: "G14", Subject: env,
				Message: "is declared as read around the configuration package, and nothing reads it that way",
			})
		}
	}
	return found
}

// checkBareReads is the half of G14 that holds the names the configuration
// package reads bare: each name without the prefix it is handed that the list
// does not hold is declared as somebody else's, and each declaration is a
// name it reads that way. A suffix the list holds is read under the prefix,
// so a declaration of one answers nothing.
func (g *gate) checkBareReads(names []string, bare map[string][]string) []Finding {
	var found []Finding
	for _, env := range sortedKeys(bare) {
		_, declared := g.rules.envBare[env]
		if declared || slices.Contains(names, env) {
			continue
		}
		found = append(found, Finding{
			Rule: "G14", Subject: env,
			Message: fmt.Sprintf("is read through the configuration package at %s, which reads it bare since %s does not hold it, and no declaration says it is somebody else's",
				strings.Join(bare[env], ", "), keyOf(g.rules.envNames)),
		})
	}
	for _, env := range sortedKeys(g.rules.envBare) {
		if len(bare[env]) == 0 || slices.Contains(names, env) {
			found = append(found, Finding{
				Rule: "G14", Subject: env,
				Message: "is declared as read bare through the configuration package, and nothing reads it that way",
			})
		}
	}
	return found
}

// prefixedNames folds the configuration package's list of prefixed names out
// of its composite literal.
func (g *gate) prefixedNames() ([]string, bool) {
	decl, err := g.p.lookup(g.rules.envNames)
	if err != nil {
		return nil, false
	}
	lit, isLit := ast.Unparen(decl.initializer()).(*ast.CompositeLit)
	if !isLit {
		return nil, false
	}
	var names []string
	for _, elt := range lit.Elts {
		name, folds := constString(decl.info(), elt)
		if !folds {
			return nil, false
		}
		names = append(names, name)
	}
	return names, true
}

// directEnvReads maps each variable carrying the prefix that a read's name
// argument folds to onto the places it is read, for the readers and
// expanders the rules list, at their first argument, and every function
// [followParameters] makes one of them.
func (g *gate) directEnvReads(passes []parameterPass) map[string][]string {
	readers := map[readerArg]bool{}
	for _, name := range g.rules.envReaders {
		readers[readerArg{fn: name}] = false
	}
	for _, name := range g.rules.envExpanders {
		readers[readerArg{fn: name}] = true
	}
	return g.envReads(followParameters(readers, passes), func(env string) bool {
		return strings.HasPrefix(env, g.rules.envPrefix)
	})
}

// bareEnvReads maps each name without the prefix that a read through the
// configuration package's readers folds to onto the places it is read, for
// those readers, at their first argument, and every function
// [followParameters] makes one of them. Whether the list holds the name is
// [gate.checkBareReads]'s to ask.
func (g *gate) bareEnvReads(passes []parameterPass) map[string][]string {
	readers := map[readerArg]bool{}
	for _, name := range g.rules.envListReaders {
		readers[readerArg{fn: name}] = false
	}
	return g.envReads(followParameters(readers, passes), func(env string) bool {
		return !strings.HasPrefix(env, g.rules.envPrefix)
	})
}

// envReads maps each name a reader's name argument folds to, among those
// keep accepts, onto the places it is read. An expander's argument is a
// template, and every variable it names in $NAME or ${NAME} form is read.
func (g *gate) envReads(readers map[readerArg]bool, keep func(string) bool) map[string][]string {
	reads := map[string][]string{}
	g.p.forEachCall(func(_ string, info *types.Info, call *ast.CallExpr) {
		callee := calleeName(calleeOf(info, call))
		for i, arg := range call.Args {
			expands, isReader := readers[readerArg{fn: callee, arg: i}]
			if !isReader {
				continue
			}
			name, folds := constString(info, arg)
			if !folds {
				continue
			}
			names := []string{name}
			if expands {
				names = expandedNames(name)
			}
			for _, env := range names {
				if keep(env) {
					reads[env] = append(reads[env], g.p.position(call.Pos()))
				}
			}
		}
	})
	return reads
}

// readerArg is one argument of a function that names the variable it reads:
// the function, by full name, and the index of the argument. The readers map
// each one to whether the argument is a template naming several.
type readerArg struct {
	fn  string
	arg int
}

// followParameters adds to readers every parameter of a function of the
// program that the function hands on to a reader's argument, with whether
// that argument expands, followed until no new one turns up. Each parameter
// is followed on its own, so a function that hands two of its parameters to
// readers reads through both, whichever reader each reaches and in whichever
// order the passes are met. A name built inside the function, or taken from
// a loop, is not followed.
func followParameters(readers map[readerArg]bool, passes []parameterPass) map[readerArg]bool {
	// A parameter is added once and never revisited, so the loop ends after
	// at most one round per parameter, whatever the passes say.
	for grew := true; grew; {
		grew = false
		for _, p := range passes {
			caller := readerArg{fn: p.caller, arg: p.param}
			if _, known := readers[caller]; known {
				continue
			}
			expands, reads := readers[readerArg{fn: p.callee, arg: p.arg}]
			if !reads {
				continue
			}
			readers[caller] = expands
			grew = true
		}
	}
	return readers
}

// parameterPass is one call that hands a function's own parameter on: the
// function, by full name, and the index of its parameter, the callee and the
// index of the argument it is passed as.
type parameterPass struct {
	caller string
	param  int
	callee string
	arg    int
}

// parameterPasses lists every call in a function of the program that passes
// one of the function's own parameters as an argument, in the order the
// program declares them.
func (g *gate) parameterPasses() []parameterPass {
	var passes []parameterPass
	for _, pkg := range g.p.packages {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc {
					continue
				}
				passes = append(passes, functionPasses(pkg.TypesInfo, fn)...)
			}
		}
	}
	return passes
}

// functionPasses is [gate.parameterPasses] for one function declaration.
// Every declaration the load holds has a body and a type-checked object (see
// [declaration.body]).
func functionPasses(info *types.Info, fn *ast.FuncDecl) []parameterPass {
	obj, _ := info.Defs[fn.Name].(*types.Func)
	params := obj.Signature().Params()
	caller := calleeName(obj)
	var passes []parameterPass
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		callee := calleeName(calleeOf(info, call))
		for i, arg := range call.Args {
			for p := range params.Len() {
				if referenced(info, arg) == params.At(p) {
					passes = append(passes, parameterPass{caller: caller, param: p, callee: callee, arg: i})
				}
			}
		}
		return true
	})
	return passes
}

// expandedNames are the variables a template names, parsed the way os.Expand
// parses it.
func expandedNames(template string) []string {
	var names []string
	os.Expand(template, func(name string) string {
		names = append(names, name)
		return ""
	})
	return names
}
