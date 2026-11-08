// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestCountConditions_EachConstruct_CountsAsGobcoDoes holds the count to
// gobco v1.3.4's instrumenter, construct by construct and in both modes: in
// condition mode the operands of && and || taken apart, the operand of ! and
// every comparison wherever it is written, a parenthesized one passing its
// mark on; in both the condition of an if and of a for that has one, every
// case of a switch, tagged or not, and of a type switch; a case expression
// counted once though it is a comparison too; nothing in a constant
// declaration, a range, a select or a bitwise or; and nothing a //line
// directive moves out of a .go file.
func TestCountConditions_EachConstruct_CountsAsGobcoDoes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		body      string
		condition int
		branch    int
	}{
		{name: "an if comparing", body: "func f(x int) { if x > 0 {} }", condition: 1, branch: 1},
		{name: "an if of two operands", body: "func f(a, b bool) { if a && b {} }", condition: 2, branch: 1},
		{name: "an if and its else if", body: "func f(a, b bool) { if a {} else if b {} }", condition: 2, branch: 2},
		{name: "an or with a negation, returned", body: "func f(a, b bool) bool { return a || !b }", condition: 2, branch: 0},
		{name: "a negation alone", body: "func f(a bool) bool { return !a }", condition: 1, branch: 0},
		{name: "a negated comparison", body: "func f(a, b int) bool { return !(a == b) }", condition: 1, branch: 0},
		{name: "a parenthesized if condition", body: "func f(a bool) { if (a) {} }", condition: 1, branch: 1},
		{name: "a parenthesized operand", body: "func f(a, b bool) bool { return (a) && b }", condition: 2, branch: 0},
		{name: "an unmarked parenthesis", body: "func f(a int) int { return (a) }", condition: 0, branch: 0},
		{name: "a comparison assigned", body: "func f(a, b int) { _ = a == b }", condition: 1, branch: 0},
		{name: "each comparison operator", body: "func f(a, b int) []bool { return []bool{a == b, a != b, a < b, a <= b, a > b, a >= b} }", condition: 6, branch: 0},
		{name: "a comparison inside a comparison", body: "func g(bool) bool { return true }\nfunc f(a, b, c bool) bool { return g(a == b) == c }", condition: 2, branch: 0},
		{name: "a for with a condition", body: "func f(n int) { for i := 0; i < n; i++ {} }", condition: 1, branch: 1},
		{name: "a for without one", body: "func f() { for { break } }", condition: 0, branch: 0},
		{name: "a range", body: "func f(s []int) { for range s {} }", condition: 0, branch: 0},
		{name: "a switch without a tag", body: "func f(x int) { switch { case x > 1, x < -1: default: } }", condition: 2, branch: 2},
		{name: "a switch with a tag", body: "func f(x int) { switch x { case 1, 2: case 3: default: } }", condition: 3, branch: 3},
		{name: "a tag that compares", body: "func f(x int) { switch x > 0 { case true: } }", condition: 2, branch: 1},
		{name: "a case that compares", body: "func f(x int, b bool) { switch b { case x > 0: } }", condition: 1, branch: 1},
		{name: "a type switch with an assignment", body: "func f(v any) { switch t := v.(type) { case int, string: _ = t; case nil: default: } }", condition: 3, branch: 3},
		{name: "a type switch without one", body: "func f(v any) { switch v.(type) { case int: } }", condition: 1, branch: 1},
		{name: "a constant declaration", body: "const c = 1 == 1\nfunc f() { const d = 2 > 1; _ = d }", condition: 0, branch: 0},
		{name: "a variable declaration", body: "var v = 1 == 1", condition: 1, branch: 0},
		{name: "a select", body: "func f(c chan int) { select { case <-c: default: } }", condition: 0, branch: 0},
		{name: "a bitwise or and a constraint", body: "type N interface{ ~int | ~string }\nfunc f(a, b int) int { return a | b }", condition: 0, branch: 0},
		{name: "a line directive into another .go file", body: "func f(a, b int) bool {\n//line other.go:10\n\treturn a == b\n}", condition: 1, branch: 0},
		{name: "a line directive out of Go", body: "func f(a, b int) bool {\n//line generated.y:10\n\tif a > 0 {}\n\treturn a == b\n}", condition: 0, branch: 0},
		{name: "a type switch a line directive moves", body: "func f(v any) {\n//line generated.y:10\n\tswitch v.(type) { case int: }\n}", condition: 0, branch: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "planted.go", "package p\n"+tc.body+"\n", parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			if got := countConditions(fset, []*ast.File{file}, false); got != tc.condition {
				t.Errorf("countConditions(condition mode) = %d, want %d", got, tc.condition)
			}
			if got := countConditions(fset, []*ast.File{file}, true); got != tc.branch {
				t.Errorf("countConditions(branch mode) = %d, want %d", got, tc.branch)
			}
		})
	}
}

// TestCountConditions_SeveralFiles_AreSummed verifies that every file of a
// package is counted, each with marks of its own.
func TestCountConditions_SeveralFiles_AreSummed(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var files []*ast.File
	var failures []error
	for i, body := range []string{"func f(a, b bool) bool { return a && b }", "func g(x int) { if x > 0 {} }"} {
		file, err := parser.ParseFile(fset, fmt.Sprintf("f%d.go", i), "package p\n"+body+"\n", 0)
		failures = append(failures, err)
		files = append(files, file)
	}
	if err := errors.Join(failures...); err != nil {
		t.Fatal(err)
	}
	if got := countConditions(fset, files, false); got != 3 {
		t.Errorf("countConditions(condition mode) = %d, want 3", got)
	}
	if got := countConditions(fset, files, true); got != 1 {
		t.Errorf("countConditions(branch mode) = %d, want 1", got)
	}
}

// TestGobcoRelease_IsTheOneTheScriptPins holds the release the count follows
// to the one scripts/coverage-conditions.sh runs, so a bump of the script's
// pin fails here until the counting rule has been read against it.
func TestGobcoRelease_IsTheOneTheScriptPins(t *testing.T) {
	t.Parallel()
	script := readFile(t, filepath.Join("..", "..", filepath.FromSlash(coverageScript)))
	match := regexp.MustCompile(`(?m)^readonly GOBCO="github\.com/rillig/gobco@(v[^"]+)"$`).FindStringSubmatch(script)
	if match == nil {
		t.Fatalf("%s pins no gobco release in the line this test reads", coverageScript)
	}
	if match[1] != gobcoRelease {
		t.Errorf("%s pins gobco %s and the count follows %s; read markConds, prepareSwitchStmt and prepareTypeSwitchStmt of %s and bring countConditions and gobcoRelease to it", coverageScript, match[1], gobcoRelease, match[1])
	}
}

// listCallOf is a lister answering every call with out or err, recording
// what it was asked into calls.
func listCallOf(out string, err error, calls *[]listCall) lister {
	return func(_ context.Context, dir string, env []string, args ...string) ([]byte, error) {
		*calls = append(*calls, listCall{dir: dir, env: env, args: args})
		return []byte(out), err
	}
}

// TestCountPackage_Listing_CountsTheFilesTheGoCommandBuilds verifies that
// the package's files are asked of the go command in the module root, with
// the script's environment, and that every Go and cgo file it names is read,
// in the mode the measurer counts in.
func TestCountPackage_Listing_CountsTheFilesTheGoCommandBuilds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var failures []error
	for name, body := range map[string]string{
		"a.go":     "package p\nfunc f(a, b bool) bool { return a && b }\n",
		"c.go":     "package p\nimport \"C\"\nfunc g(x int) { if x > 0 {} }\n",
		"other.go": "package p\nfunc h(x int) { if x > 0 {} }\n",
	} {
		failures = append(failures, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	if err := errors.Join(failures...); err != nil {
		t.Fatal(err)
	}
	listing := fmt.Sprintf(`{"Dir": %q, "GoFiles": ["a.go"], "CgoFiles": ["c.go"]}`, dir)
	for mode, want := range map[string]int{"condition": 3, "branch": 1} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			var calls []listCall
			m := measurer{list: listCallOf(listing, nil, &calls), dir: "root", env: []string{"GOBCO_GATE=all"}, mode: mode}
			got, err := countPackage(context.Background(), m, "./p")
			if err != nil || got.count != want || !reflect.DeepEqual(got.files, []string{"a.go", "c.go"}) {
				t.Errorf("countPackage() = %+v, %v; want %d conditions in a.go and c.go", got, err, want)
			}
			wantCalls := []listCall{{dir: "root", env: []string{"GOBCO_GATE=all"}, args: []string{"list", "-json=Dir,GoFiles,CgoFiles", "./p"}}}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Errorf("lister calls = %+v, want %+v", calls, wantCalls)
			}
		})
	}
}

// TestCountPackage_Failures_AreNoCount verifies that every way the count can
// fail is an error and never a count: the go command failing, a listing that
// is not one package's JSON, and a file it names that cannot be read or
// parsed.
func TestCountPackage_Failures_AreNoCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package p\nfunc {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		out     string
		err     error
		wantErr string
	}{
		{name: "the go command fails", err: errors.New("planted"), wantErr: "planted"},
		{name: "not JSON", out: "example.com/m/p", wantErr: "go list for ./p: invalid character"},
		{name: "two packages", out: `{"Dir": "a"} {"Dir": "b"}`, wantErr: "go list for ./p: invalid character"},
		{name: "a file that does not parse", out: fmt.Sprintf(`{"Dir": %q, "GoFiles": ["broken.go"]}`, dir), wantErr: filepath.Join(dir, "broken.go") + ":2:6: expected 'IDENT'"},
		{name: "a file that is not there", out: fmt.Sprintf(`{"Dir": %q, "CgoFiles": ["gone.go"]}`, dir), wantErr: "open " + filepath.Join(dir, "gone.go")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls []listCall
			got, err := countPackage(context.Background(), measurer{list: listCallOf(tc.out, tc.err, &calls), mode: "condition"}, "./p")
			if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr) || !reflect.DeepEqual(got, packageConditions{}) {
				t.Errorf("countPackage() = %+v, %v; want no count and an error starting %q", got, err, tc.wantErr)
			}
		})
	}
}

// TestCountPackage_PlantedModule_CountsWhatTheGoCommandLists counts the
// planted module's packages through the toolchain's own go command: a test
// file and a file a constraint leaves out are not counted, a comparison
// returned is a condition and no branch, and a package of test files alone
// has no file to count.
func TestCountPackage_PlantedModule_CountsWhatTheGoCommandLists(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pkg       string
		files     []string
		condition int
		branch    int
	}{
		{pkg: "./cmd/none", files: []string{"none.go"}, condition: 0, branch: 0},
		{pkg: "./internal/some", files: []string{"some.go"}, condition: 1, branch: 1},
		{pkg: "./internal/branchless", files: []string{"branchless.go"}, condition: 1, branch: 0},
		{pkg: "./internal/testonly", condition: 0, branch: 0},
	}
	for _, tc := range cases {
		for mode, want := range map[string]int{"condition": tc.condition, "branch": tc.branch} {
			t.Run(tc.pkg+" in "+mode+" mode", func(t *testing.T) {
				t.Parallel()
				got, err := countPackage(context.Background(), measurer{list: goList, dir: plantedModule, mode: mode}, tc.pkg)
				if err != nil || got.count != want || !reflect.DeepEqual(got.files, tc.files) {
					t.Errorf("countPackage() = %+v, %v; want %d in %q", got, err, want, tc.files)
				}
			})
		}
	}
}

// plantedModule is the module the planted packages of the count's tests sit
// in, below cmd/ and internal/ as the tree's packages do.
var plantedModule = filepath.Join("testdata", "conditions")
