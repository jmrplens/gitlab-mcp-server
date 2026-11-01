// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// gobcoRelease is the gobco release whose counting rule countConditions
// follows, which is the one scripts/coverage-conditions.sh pins. A test holds
// the two to each other, so a bump of the script's pin fails until the rule
// below has been read against the new release.
const gobcoRelease = "v1.3.4"

// sourcesListing is the package the go command builds for one directory:
// where it is, and its non-test files, which scripts/coverage-conditions.sh
// reads from the same listing as the files gobco instruments (its sources).
type sourcesListing struct {
	Dir      string
	GoFiles  []string
	CgoFiles []string
}

// packageConditions is how many conditions gobco would count in one package,
// in one mode, and the files it read them from, as named in the listing.
type packageConditions struct {
	files []string
	count int
}

// countPackage counts the conditions gobco would instrument in pkg, in the
// mode m measures in: it asks the go command, with the environment the script
// will run under, for the package's non-test files, and reads each the way
// gobco does. An error means the count could not be taken, which is never a
// count of zero: the script is then run, and says itself why it cannot
// measure, if it cannot.
func countPackage(ctx context.Context, m measurer, pkg string) (packageConditions, error) {
	out, err := m.list(ctx, m.dir, m.env, "list", "-json=Dir,GoFiles,CgoFiles", pkg)
	if err != nil {
		return packageConditions{}, err
	}
	var listed sourcesListing
	if err = json.Unmarshal(out, &listed); err != nil {
		return packageConditions{}, fmt.Errorf("go list for %s: %w", pkg, err)
	}
	fset := token.NewFileSet()
	names := slices.Concat(listed.GoFiles, listed.CgoFiles)
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		file, parseErr := parser.ParseFile(fset, filepath.Join(listed.Dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if parseErr != nil {
			return packageConditions{}, parseErr
		}
		files = append(files, file)
	}
	return packageConditions{files: names, count: countConditions(fset, files, m.mode == "branch")}, nil
}

// countConditions returns how many conditions gobco instruments in files,
// which is half the total of the figure it reports, in branch mode when
// branch is set and in condition mode otherwise. It follows the instrumenter
// of gobcoRelease: the expressions markConds leaves marked, each found where
// its parent holds it, and every case of a switch with a tag and of a type
// switch, which prepareSwitchStmt and prepareTypeSwitchStmt wrap in both
// modes. A case expression markConds marked as well is one condition, as it
// is one replacement there. A condition whose position a //line directive
// moves out of a .go file is not counted, since callCover leaves it
// unwrapped.
func countConditions(fset *token.FileSet, files []*ast.File, branch bool) int {
	count := 0
	for _, file := range files {
		wrapped := map[ast.Expr]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			return markCondition(wrapped, branch, n)
		})
		var typeCases []ast.Expr
		ast.Inspect(file, func(n ast.Node) bool {
			if statement, isSwitch := n.(*ast.SwitchStmt); isSwitch && statement.Tag != nil {
				for _, expr := range caseExpressions(statement.Body) {
					wrapped[expr] = true
				}
			}
			if statement, isTypeSwitch := n.(*ast.TypeSwitchStmt); isTypeSwitch {
				typeCases = append(typeCases, caseExpressions(statement.Body)...)
			}
			return true
		})
		for expr := range wrapped {
			count += inGoFile(fset, expr)
		}
		for _, expr := range typeCases {
			count += inGoFile(fset, expr)
		}
	}
	return count
}

// markCondition is gobco's markConds, case for case and in the order
// ast.Inspect visits the nodes, over the set of marked expressions: in
// condition mode the operands of && and ||, the operand of !, and every
// comparison, a marked parenthesized expression passing its mark to what it
// holds; in both modes the condition of each if and of each for that has
// one, and each case of a switch without a tag. A constant declaration is not
// descended into.
func markCondition(marked map[ast.Expr]bool, branch bool, n ast.Node) bool {
	switch n := n.(type) {
	case *ast.ParenExpr:
		if marked[n] {
			delete(marked, n)
			marked[n.X] = true
		}
	case *ast.UnaryExpr:
		if !branch && n.Op == token.NOT {
			delete(marked, n)
			marked[n.X] = true
		}
	case *ast.BinaryExpr:
		if branch {
			break
		}
		if n.Op == token.LAND || n.Op == token.LOR {
			delete(marked, n)
			marked[n.X] = true
			marked[n.Y] = true
		}
		if n.Op.Precedence() == token.EQL.Precedence() {
			marked[n] = true
		}
	case *ast.IfStmt:
		marked[n.Cond] = true
	case *ast.SwitchStmt:
		if n.Tag == nil {
			for _, expr := range caseExpressions(n.Body) {
				marked[expr] = true
			}
		}
	case *ast.ForStmt:
		if n.Cond != nil {
			marked[n.Cond] = true
		}
	case *ast.GenDecl:
		return n.Tok != token.CONST
	}
	return true
}

// caseExpressions is every expression the case clauses of a switch or a type
// switch list, clause by clause, which is every statement of its body.
func caseExpressions(body *ast.BlockStmt) []ast.Expr {
	var exprs []ast.Expr
	for _, statement := range body.List {
		if clause, isCase := statement.(*ast.CaseClause); isCase {
			exprs = append(exprs, clause.List...)
		}
	}
	return exprs
}

// inGoFile is 1 for an expression whose position, //line directives
// applied, lies in a .go file, and 0 for one a directive moved elsewhere.
func inGoFile(fset *token.FileSet, expr ast.Expr) int {
	if strings.HasSuffix(fset.Position(expr.Pos()).Filename, ".go") {
		return 1
	}
	return 0
}

// nothingPattern is the line a record's output holds in place of the
// script's for a package with no condition to measure in the mode named, and
// the only line it holds: the script is not run for it.
var nothingPattern = regexp.MustCompile(`^measure_conditions: nothing to measure in (condition|branch) mode: `)

// nothingLine says why a package with no condition gobco counts in mode was
// recorded as measured with nothing to measure, naming the files it read.
func nothingLine(pkg, mode string, counted packageConditions) string {
	files := "none but its tests"
	if len(counted.files) > 0 {
		files = strings.Join(counted.files, ", ")
	}
	return fmt.Sprintf("%s: nothing to measure in %s mode: gobco %s counts no condition in the files the go command builds of %s on %s/%s (%s), so %s was not run, and the package is measured with nothing to measure: 0 of 0\n",
		toolName, mode, gobcoRelease, pkg, runtime.GOOS, runtime.GOARCH, files, coverageScript)
}
