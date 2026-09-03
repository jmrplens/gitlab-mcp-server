// Package main tests the case-loop subtest auditor and its rewriter against
// fixture files covering every table shape, the escape hatch, and the
// control-flow blockers.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureSource exercises: a []string table (fixable, named after the
// element), a struct table with a name field (fixable), a map[string] table
// (fixable, named after the key), a struct table without a name field
// (needs-name), a loop with a bare break (break), a declared-sequential
// loop, a compliant loop already under t.Run, a setup loop that never
// asserts, a single-element literal that is not a table, and a loop inside
// a synctest bubble, where t.Run is not allowed.
const fixtureSource = `package fixture

import (
	"strings"
	"testing"
	"testing/synctest"
)

type step struct {
	label string
	in    string
}

func TestFixture(t *testing.T) {
	out := "alpha beta"
	for _, want := range []string{"alpha", "beta"} { // element
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}

	for _, tc := range []struct {
		name string
		in   string
	}{{"one", "a"}, {"two", "b"}} { // field:name
		if tc.in == "" {
			continue
		}
		if !strings.Contains(out, tc.in) {
			t.Fatalf("%s: missing", tc.name)
		}
	}

	for key, want := range map[string]string{"k1": "alpha", "k2": "beta"} { // key
		if !strings.Contains(out, want) {
			t.Errorf("%s: missing %q", key, want)
		}
	}

	for _, tc := range []struct {
		in   string
		want bool
	}{{"a", true}, {"b", true}} { // needs-name
		if strings.Contains(out, tc.in) != tc.want {
			t.Errorf("%q", tc.in)
		}
	}

	for _, want := range []string{"alpha", "gamma"} { // break
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
			break
		}
	}

	// sequential: every step reads what the previous one wrote
	for _, s := range []step{{"a", "1"}, {"b", "2"}} {
		if s.in == "" {
			t.Fatal("empty")
		}
	}

	for _, want := range []string{"alpha", "beta"} { // compliant
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q", want)
			}
		})
	}

	seen := map[string]bool{}
	for _, name := range []string{"x", "y"} { // setup only
		seen[name] = true
	}

	for _, want := range []string{"alpha"} { // one element: not a table
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}

	synctest.Test(t, func(t *testing.T) { // synctest bubble: t.Run would panic
		for _, want := range []string{"alpha", "beta"} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q", want)
			}
		}
	})
}
`

// writeFixture writes the fixture into a temporary directory and returns
// the file path.
func writeFixture(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture_test.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestScan_ClassifiesEveryTableShape verifies that scan reports each
// asserting case loop with the rewrite it qualifies for, records the
// declared-sequential and synctest-bubble loops separately, counts the
// compliant loop, and skips the setup loop and the single-element literal.
func TestScan_ClassifiesEveryTableShape(t *testing.T) {
	path := writeFixture(t, fixtureSource)
	report, err := scan([]string{filepath.Dir(path)})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	byLine := map[int]Finding{}
	for _, f := range report.Findings {
		byLine[f.Line] = f
	}
	for _, tc := range []struct {
		name string
		line int
		fix  string
	}{
		{"string_slice", 16, "element"},
		{"struct_with_name", 22, "field:name"},
		{"string_map", 34, "key"},
		{"struct_without_name", 40, "needs-name"},
		{"bare_break", 49, "break"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := byLine[tc.line]
			if !ok {
				t.Fatalf("no finding at line %d; findings: %+v", tc.line, report.Findings)
			}
			if got.Fix != tc.fix {
				t.Errorf("finding at line %d: Fix = %q, want %q", tc.line, got.Fix, tc.fix)
			}
			if got.Test != "TestFixture" {
				t.Errorf("finding at line %d: Test = %q, want TestFixture", tc.line, got.Test)
			}
		})
	}

	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"sites", report.Summary.Sites, 5},
		{"fixable", report.Summary.Fixable, 3},
		{"sequential", report.Summary.Sequential, 1},
		{"synctest", report.Summary.Synctest, 1},
		{"compliant", report.Summary.Compliant, 1},
		{"files", report.Summary.Files, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("summary %s = %d, want %d", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestFixFile_RewritesFixableSitesOnly verifies that -fix wraps the three
// unambiguous loops in t.Run with the derived name, turns the bare continue
// into a return, leaves the needs-name and break loops untouched, and yields
// a file that parses and that a second scan reports as only those two.
func TestFixFile_RewritesFixableSitesOnly(t *testing.T) {
	path := writeFixture(t, fixtureSource)
	n, err := fixFile(path)
	if err != nil {
		t.Fatalf("fixFile: %v", err)
	}
	if n != 3 {
		t.Fatalf("fixFile rewrote %d site(s), want 3", n)
	}

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten fixture: %v", err)
	}
	got := string(src)
	if _, parseErr := parser.ParseFile(token.NewFileSet(), path, src, parser.ParseComments); parseErr != nil {
		t.Fatalf("rewritten fixture does not parse: %v\n%s", parseErr, got)
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{"element_name", "t.Run(want, func(t *testing.T) {"},
		{"field_name", "t.Run(tc.name, func(t *testing.T) {"},
		{"key_name", "t.Run(key, func(t *testing.T) {"},
		{"continue_becomes_return", "if tc.in == \"\" {\n\t\t\t\treturn\n\t\t\t}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(got, tc.want) {
				t.Errorf("rewritten fixture lacks %q:\n%s", tc.want, got)
			}
		})
	}
	if strings.Contains(got, "continue") {
		t.Errorf("a bare continue survived the rewrite:\n%s", got)
	}

	report, err := scan([]string{filepath.Dir(path)})
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	remaining := map[string]int{}
	for _, f := range report.Findings {
		remaining[f.Fix]++
	}
	for _, tc := range []struct {
		name string
		want int
	}{{"needs-name", 1}, {"break", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			if remaining[tc.name] != tc.want {
				t.Errorf("after fix, %s sites = %d, want %d (findings %+v)", tc.name, remaining[tc.name], tc.want, report.Findings)
			}
		})
	}
	if report.Summary.Fixable != 0 {
		t.Errorf("after fix, fixable = %d, want 0", report.Summary.Fixable)
	}
	if report.Summary.Compliant != 4 {
		t.Errorf("after fix, compliant = %d, want 4 (one original plus three rewritten)", report.Summary.Compliant)
	}
	if report.Summary.Synctest != 1 {
		t.Errorf("after fix, synctest = %d, want 1 (the bubble loop must stay untouched)", report.Summary.Synctest)
	}
}

// TestFixFile_LeavesCleanFilesAlone verifies that a file with nothing to
// rewrite is neither touched nor counted.
func TestFixFile_LeavesCleanFilesAlone(t *testing.T) {
	const clean = `package fixture

import "testing"

func TestClean(t *testing.T) {
	for _, want := range []string{"a", "b"} {
		t.Run(want, func(t *testing.T) {
			if want == "" {
				t.Fatal("empty")
			}
		})
	}
}
`
	path := writeFixture(t, clean)
	n, err := fixFile(path)
	if err != nil {
		t.Fatalf("fixFile: %v", err)
	}
	if n != 0 {
		t.Fatalf("fixFile rewrote %d site(s) in a clean file, want 0", n)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(src) != clean {
		t.Errorf("clean file was modified:\n%s", src)
	}
}

// TestBodyControlFlow_ClassifiesBranches verifies the control-flow gate:
// a break aimed at the loop blocks the rewrite, a goto or label blocks it,
// a continue inside a nested loop is that loop's business, and a bare
// continue is collected for conversion.
func TestBodyControlFlow_ClassifiesBranches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		reason    string
		continues int
	}{
		{"bare_continue", "for _, x := range xs { if x == 0 { continue }; t.Error(x) }", "", 1},
		{"continue_in_switch", "for _, x := range xs { switch x { case 0: continue }; t.Error(x) }", "", 1},
		{"nested_loop_continue", "for _, x := range xs { for range 2 { continue }; t.Error(x) }", "", 0},
		{"bare_break", "for _, x := range xs { if x == 0 { break }; t.Error(x) }", "break", 0},
		{"break_in_switch_is_fine", "for _, x := range xs { switch x { case 0: break }; t.Error(x) }", "", 0},
		{"goto", "for _, x := range xs { if x == 0 { goto done }; t.Error(x); done: }", "goto", 0},
		{"closure_is_opaque", "for _, x := range xs { f := func() { for { break } }; f(); t.Error(x) }", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\nimport \"testing\"\nfunc TestX(t *testing.T) { xs := []int{1, 2}; " + tc.body + " }\n"
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "x_test.go", src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var loop *ast.RangeStmt
			ast.Inspect(file, func(n ast.Node) bool {
				if rs, ok := n.(*ast.RangeStmt); ok && loop == nil {
					loop = rs
				}
				return loop == nil
			})
			reason, continues := bodyControlFlow(loop.Body)
			if reason != tc.reason {
				t.Errorf("reason = %q, want %q", reason, tc.reason)
			}
			if len(continues) != tc.continues {
				t.Errorf("continues = %d, want %d", len(continues), tc.continues)
			}
		})
	}
}
