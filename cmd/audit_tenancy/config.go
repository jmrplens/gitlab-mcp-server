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

// checkConfig is G14, INV-017's central clause: a setting a row names is read
// the way the configuration package reads one. Its name, without the prefix,
// is on the configuration package's list of prefixed names, and nothing in the
// program hands that name to a direct reader (os.Getenv, os.LookupEnv and
// their syscall twins) or names it in a template os.ExpandEnv reads. A row
// with a variable that fails either carries the finding that records it; a row
// carrying that finding while every variable passes has a finding that no
// longer describes the tree.
//
// Validate cannot hold this, because the register imports nothing and so
// cannot see the list or the calls. A register no row of which names a
// variable has nothing to hold to the list, and the list is not read.
func (g *gate) checkConfig() []Finding {
	if !slices.ContainsFunc(g.reg.decisions, func(d tenancy.Decision) bool { return len(d.Envs) > 0 }) {
		return nil
	}
	names, ok := g.prefixedNames()
	if !ok {
		return []Finding{{
			Rule: "G14", Subject: keyOf(g.rules.envNames),
			Message: "is not a package-level list of string constants, so no setting can be checked against it",
		}}
	}
	readDirectly := g.directEnvReads()
	var found []Finding
	for _, d := range g.reg.decisions {
		var failing []string
		for _, env := range d.Envs {
			g.read.settings++
			suffix, prefixed := strings.CutPrefix(env, g.rules.envPrefix)
			switch {
			case !prefixed || !slices.Contains(names, suffix):
				failing = append(failing, env+" is not on "+keyOf(g.rules.envNames))
			case len(readDirectly[env]) > 0:
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

// directEnvReads maps each variable name an environment read's argument folds
// to onto the places it is read. An expander's argument is a template, and
// every variable it names in $NAME or ${NAME} form is read.
func (g *gate) directEnvReads() map[string][]string {
	reads := map[string][]string{}
	g.p.forEachCall(func(_ string, info *types.Info, call *ast.CallExpr) {
		// Every reader takes the variable's name, and every expander its
		// template, as its first argument.
		name := calleeName(calleeOf(info, call))
		expands := slices.Contains(g.rules.envExpanders, name)
		if !expands && !slices.Contains(g.rules.envReaders, name) {
			return
		}
		arg, folds := constString(info, call.Args[0])
		if !folds {
			return
		}
		names := []string{arg}
		if expands {
			names = expandedNames(arg)
		}
		for _, env := range names {
			reads[env] = append(reads[env], g.p.position(call.Pos()))
		}
	})
	return reads
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
