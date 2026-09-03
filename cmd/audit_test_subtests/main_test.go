// Package main tests the case-loop subtest auditor and its rewriter against
// fixture files covering every table shape, the escape hatch, and the
// control-flow blockers.
package main

import (
	"encoding/json"
	"fmt"
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

// fixtureShapes exercises the table shapes the first fixture leaves out: a
// table bound to a local variable, a struct type declared in the file (named
// after its label field), a declared non-struct element type, an undeclared
// element type, a map with a non-string key, a blank map key, a blank slice
// element, a blank struct value, a helper that receives t, a label in the
// body, and a ranged expression that is not a table literal at all.
const fixtureShapes = `package fixture

import "testing"

type step struct {
	label string
	in    string
}

type name string

func check(t *testing.T, s string) {
	if s == "" {
		t.Fatal("empty")
	}
}

func TestShapes(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{{"one", "a"}, {"two", "b"}}
	for _, tc := range cases { // named table: field:name
		if tc.in == "" {
			t.Errorf("%s", tc.name)
		}
	}

	for _, s := range []step{{"a", "1"}, {"b", "2"}} { // declared struct: field:label
		if s.in == "" {
			t.Error(s.label)
		}
	}

	for _, n := range []name{"a", "b"} { // declared non-struct type: needs-name
		if n == "" {
			t.Error(n)
		}
	}

	for _, u := range []undeclared{{}, {}} { // undeclared type: needs-name
		t.Error(u)
	}

	for k := range map[int]string{1: "a", 2: "b"} { // non-string key: needs-name
		t.Error(k)
	}

	for _, v := range map[string]string{"a": "1", "b": "2"} { // blank key: blank-var
		t.Error(v)
	}

	for range []string{"a", "b"} { // blank element: blank-var
		t.Error("x")
	}

	for range []struct{ name string }{{"a"}, {"b"}} { // blank struct value: blank-var
		t.Error("x")
	}

	for _, s := range []string{"a", "b"} { // helper receives t: element
		check(t, s)
	}

	for _, s := range []string{"a", "b"} { // label: goto
	again:
		t.Error(s)
	}

	var holder struct{ cases []string }
	holder.cases = []string{"a", "b"}
	p := struct{ x, y int }{1, 2}
	_ = p
	for _, s := range holder.cases { // unresolved table: not a site
		t.Error(s)
	}
}
`

// cleanSource is a test file whose only case loop already runs under t.Run.
const cleanSource = `package fixture

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

// writeFile writes src under dir at rel, creating parents, and returns the path.
func writeFile(t *testing.T, dir, rel, src string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

// TestScan_TableShapes_ClassifiesEachRewrite verifies the shape fixture: the
// named table and the declared struct are fixable through their name fields,
// the helper call that receives t is a fixable element site, a label blocks
// the rewrite, and the non-string key, blank variables, undeclared and
// non-struct element types are reported as sites that need a hand-written
// name; the ranged field expression is not a table and is not a site.
func TestScan_TableShapes_ClassifiesEachRewrite(t *testing.T) {
	path := writeFixture(t, fixtureShapes)
	report, err := scan([]string{filepath.Dir(path)})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	byFix := map[string]int{}
	tables := map[string]int{}
	for _, f := range report.Findings {
		byFix[f.Fix]++
		tables[f.Table]++
		if f.Test != "TestShapes" {
			t.Errorf("finding at line %d attributed to %q, want TestShapes", f.Line, f.Test)
		}
	}
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"field_name", byFix["field:name"], 1},
		{"field_label", byFix["field:label"], 1},
		{"element", byFix["element"], 1},
		{"goto", byFix["goto"], 1},
		{"needs_name", byFix["needs-name"], 3},
		{"blank_var", byFix["blank-var"], 3},
		{"named_tables", tables["named"], 1},
		{"inline_tables", tables["inline"], 9},
		{"sites", report.Summary.Sites, 10},
		{"fixable", report.Summary.Fixable, 3},
		{"compliant", report.Summary.Compliant, 0},
		{"files", report.Summary.Files, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %d, want %d (findings %+v)", tc.name, tc.got, tc.want, report.Findings)
			}
		})
	}
}

// TestScan_TwoFiles_OrdersFindingsByFileThenLine verifies the work list is
// sorted by file and then by line, so a diff of two runs is stable.
func TestScan_TwoFiles_OrdersFindingsByFileThenLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b_test.go", fixtureShapes)
	writeFile(t, dir, "a_test.go", fixtureSource)
	report, err := scan([]string{dir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(report.Findings) != 15 || report.Summary.Files != 2 {
		t.Fatalf("findings = %d across %d files, want 15 across 2", len(report.Findings), report.Summary.Files)
	}
	for i := 1; i < len(report.Findings); i++ {
		prev, cur := report.Findings[i-1], report.Findings[i]
		if prev.File > cur.File || (prev.File == cur.File && prev.Line >= cur.Line) {
			t.Errorf("findings out of order: %s:%d before %s:%d", prev.File, prev.Line, cur.File, cur.Line)
		}
	}
	if !strings.HasSuffix(report.Findings[0].File, "a_test.go") || !strings.HasSuffix(report.Findings[14].File, "b_test.go") {
		t.Errorf("findings span %s .. %s, want a_test.go first and b_test.go last", report.Findings[0].File, report.Findings[14].File)
	}
}

// TestScan_Failures_ReturnErrors verifies a directory that cannot be walked
// and a test file that does not parse both abort the scan with an error
// rather than a partial report.
func TestScan_Failures_ReturnErrors(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
	}{
		{
			name: "missing_directory",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "absent")
			},
			wantErr: "absent",
		},
		{
			name: "unparsable_test_file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, dir, "broken_test.go", "package fixture\n\nfunc TestBroken(t *testing.T) {\n")
				return dir
			},
			wantErr: "parse",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := scan([]string{tc.setup(t)})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("scan error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestScan_SkippedDirectories_AreNeverEntered verifies node_modules, dist and
// dot-directories are pruned from the walk and non-test Go files are
// ignored, so vendored or generated trees cannot add sites.
func TestScan_SkippedDirectories_AreNeverEntered(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "node_modules/pkg/x_test.go", fixtureSource)
	writeFile(t, dir, "dist/y_test.go", fixtureSource)
	writeFile(t, dir, ".hidden/z_test.go", fixtureSource)
	writeFile(t, dir, "helper.go", fixtureSource)
	writeFile(t, dir, "sub/real_test.go", cleanSource)
	report, err := scan([]string{dir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.Summary.Sites != 0 || report.Summary.Compliant != 1 || report.Summary.Files != 1 {
		t.Errorf("summary = %+v, want only the compliant loop of sub/real_test.go", report.Summary)
	}
}

// TestSkipDir_Names_PruneVendoredAndHiddenTrees verifies the directory
// names the walk prunes and that the current directory itself is kept.
func TestSkipDir_Names_PruneVendoredAndHiddenTrees(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "node_modules", want: true},
		{name: "dist", want: true},
		{name: ".hidden", want: true},
		{name: ".", want: false},
		{name: "internal", want: false},
		{name: "distribution", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipDir(tc.name); got != tc.want {
				t.Errorf("skipDir(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestFixFile_TableShapes_RewritesOnlyTheNamedSites verifies -fix on the
// shape fixture wraps the three unambiguous loops and nothing else: the
// named table gets its name field, the declared struct its label, the helper
// site its element, and the result parses and rescans as three compliant
// loops beside the seven remaining sites.
func TestFixFile_TableShapes_RewritesOnlyTheNamedSites(t *testing.T) {
	path := writeFixture(t, fixtureShapes)
	n, err := fixFile(path)
	if err != nil {
		t.Fatalf("fixFile: %v", err)
	}
	if n != 3 {
		t.Fatalf("fixFile rewrote %d site(s), want 3", n)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(src)
	for _, want := range []string{
		"t.Run(tc.name, func(t *testing.T) {",
		"t.Run(s.label, func(t *testing.T) {",
		"t.Run(s, func(t *testing.T) { // helper receives t: element\n\t\t\tcheck(t, s)",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("rewritten fixture lacks %q:\n%s", want, got)
			}
		})
	}
	report, err := scan([]string{filepath.Dir(path)})
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if report.Summary.Sites != 7 || report.Summary.Fixable != 0 || report.Summary.Compliant != 3 {
		t.Errorf("after fix, summary = %+v, want 7 sites, none fixable, 3 compliant", report.Summary)
	}
}

// TestFixFile_Failures_ReturnErrors verifies a missing file and a file that
// does not parse are reported instead of being rewritten.
func TestFixFile_Failures_ReturnErrors(t *testing.T) {
	cases := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr string
	}{
		{
			name: "missing_file",
			path: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "absent_test.go")
			},
			wantErr: "absent_test.go",
		},
		{
			name: "unparsable_file",
			path: func(t *testing.T) string {
				t.Helper()
				return writeFile(t, t.TempDir(), "broken_test.go", "package fixture\n\nfunc TestBroken(t *testing.T) {\n")
			},
			wantErr: "parse",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := fixFile(tc.path(t))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("fixFile error = %v, want it to mention %q", err, tc.wantErr)
			}
			if n != 0 {
				t.Errorf("fixFile counted %d rewrite(s) on failure, want 0", n)
			}
		})
	}
}

// TestFixAll_Trees_RewritesTestFilesOutsideSkippedDirs verifies the fix walk
// rewrites every test file it reaches, leaves non-test files and the pruned
// directories untouched, and fails on a directory it cannot walk.
func TestFixAll_Trees_RewritesTestFilesOutsideSkippedDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "dist/skipped_test.go", fixtureSource)
	writeFile(t, dir, "helper.go", fixtureSource)
	writeFile(t, dir, "sub/real_test.go", fixtureSource)
	writeFile(t, dir, "sub/clean_test.go", cleanSource)

	n, err := fixAll([]string{dir})
	if err != nil {
		t.Fatalf("fixAll: %v", err)
	}
	if n != 3 {
		t.Errorf("fixAll rewrote %d site(s), want the 3 of sub/real_test.go", n)
	}
	for _, untouched := range []string{"dist/skipped_test.go", "helper.go"} {
		t.Run(untouched, func(t *testing.T) {
			src, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(untouched)))
			if readErr != nil {
				t.Fatalf("read: %v", readErr)
			}
			if string(src) != fixtureSource {
				t.Errorf("%s was rewritten; the fix walk must not enter it", untouched)
			}
		})
	}

	if _, missingErr := fixAll([]string{filepath.Join(dir, "absent")}); missingErr == nil {
		t.Error("fixAll on a missing directory returned nil error")
	}
}

// TestBodyControlFlow_Label_BlocksTheRewrite verifies a labeled statement
// in the body, even without a goto, is reported as a blocker: the closure
// cannot carry a label the loop's other statements may target.
func TestBodyControlFlow_Label_BlocksTheRewrite(t *testing.T) {
	src := "package p\nimport \"testing\"\nfunc TestX(t *testing.T) { xs := []int{1, 2}; for _, x := range xs {\nagain:\n t.Error(x) } }\n"
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
	if reason != "goto" || len(continues) != 0 {
		t.Errorf("bodyControlFlow = (%q, %d continues), want (goto, 0)", reason, len(continues))
	}
}

// TestPrintHuman_Findings_ListsFilesThenSummary verifies the human report
// prints one padded row per file, sorted, with its site and fixable counts,
// followed by the summary line.
func TestPrintHuman_Findings_ListsFilesThenSummary(t *testing.T) {
	report := &Report{
		Findings: []Finding{
			{File: "z_test.go", Line: 3, Fix: "needs-name"},
			{File: "a_test.go", Line: 9, Fix: "element"},
			{File: "a_test.go", Line: 4, Fix: "break"},
		},
		Summary: Summary{Sites: 3, Fixable: 1, Sequential: 2, Synctest: 1, Compliant: 4, Files: 2},
	}
	var out strings.Builder
	printHuman(&out, report)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{
		"a_test.go" + strings.Repeat(" ", 63) + " sites=2   fixable=1",
		"z_test.go" + strings.Repeat(" ", 63) + " sites=1   fixable=0",
		"",
		"summary: 3 case loop(s) assert without a subtest (1 fixable by -fix), 2 declared sequential, 1 inside synctest bubbles, 4 compliant, across 2 file(s)",
	}
	if len(lines) != len(want) {
		t.Fatalf("printHuman wrote %d lines, want %d:\n%s", len(lines), len(want), out.String())
	}
	for i, line := range want {
		t.Run(fmt.Sprintf("line_%d", i), func(t *testing.T) {
			if lines[i] != line {
				t.Errorf("line %d = %q, want %q", i, lines[i], line)
			}
		})
	}
}

// TestRun_Flags_DriveExitCodesAndOutput verifies the command's contract from
// the flags in: -check exits 1 with the failure line while sites remain and 0
// with the pass line otherwise, -json writes the work list and names it,
// -fix rewrites before reporting, and a bad flag, an unwalkable directory, an
// unparsable file, a failed rewrite and an unwritable work list exit 2 with
// the cause on stderr; -h prints usage and exits 0.
func TestRun_Flags_DriveExitCodesAndOutput(t *testing.T) {
	cases := []struct {
		name     string
		args     func(t *testing.T) []string
		wantCode int
		wantOut  []string
		wantErr  string
	}{
		{
			name: "check_fails_on_sites",
			args: func(t *testing.T) []string {
				t.Helper()
				return []string{"-check", filepath.Dir(writeFixture(t, fixtureSource))}
			},
			wantCode: 1,
			wantOut:  []string{"sites=5   fixable=3", "check: FAIL. 5 case loop(s) assert without a subtest (1 declared sequential)\n"},
		},
		{
			name: "check_passes_on_clean_tree",
			args: func(t *testing.T) []string {
				t.Helper()
				return []string{"-check", filepath.Dir(writeFixture(t, cleanSource))}
			},
			wantCode: 0,
			wantOut:  []string{"summary: 0 case loop(s)", "check: PASS. Every case loop runs its cases under t.Run (0 declared sequential)\n"},
		},
		{
			name: "json_writes_the_work_list",
			args: func(t *testing.T) []string {
				t.Helper()
				dir := filepath.Dir(writeFixture(t, fixtureSource))
				return []string{"-json", filepath.Join(dir, "work.json"), dir}
			},
			wantCode: 0,
			wantOut:  []string{"work list written to "},
		},
		{
			name: "json_to_a_directory_fails",
			args: func(t *testing.T) []string {
				t.Helper()
				dir := filepath.Dir(writeFixture(t, cleanSource))
				return []string{"-json", dir, dir}
			},
			wantCode: 2,
			wantErr:  "audit_test_subtests: write ",
		},
		{
			name: "fix_then_check_reports_the_rest",
			args: func(t *testing.T) []string {
				t.Helper()
				return []string{"-fix", "-check", filepath.Dir(writeFixture(t, fixtureSource))}
			},
			wantCode: 1,
			wantOut:  []string{"fix: rewrote 3 site(s)\n", "check: FAIL. 2 case loop(s) assert without a subtest (1 declared sequential)\n"},
		},
		{
			name: "fix_on_missing_directory_fails",
			args: func(t *testing.T) []string {
				t.Helper()
				return []string{"-fix", filepath.Join(t.TempDir(), "absent")}
			},
			wantCode: 2,
			wantErr:  "audit_test_subtests: fix: ",
		},
		{
			name: "scan_of_missing_directory_fails",
			args: func(t *testing.T) []string {
				t.Helper()
				return []string{filepath.Join(t.TempDir(), "absent")}
			},
			wantCode: 2,
			wantErr:  "audit_test_subtests: ",
		},
		{
			name: "unparsable_file_fails",
			args: func(t *testing.T) []string {
				t.Helper()
				return []string{filepath.Dir(writeFile(t, t.TempDir(), "broken_test.go", "package fixture\n\nfunc TestBroken(t *testing.T) {\n"))}
			},
			wantCode: 2,
			wantErr:  "parse",
		},
		{
			name:     "unknown_flag_fails",
			args:     func(_ *testing.T) []string { return []string{"-bogus"} },
			wantCode: 2,
			wantErr:  "flag provided but not defined: -bogus",
		},
		{
			name:     "help_exits_zero",
			args:     func(_ *testing.T) []string { return []string{"-h"} },
			wantCode: 0,
			wantErr:  "Usage of audit_test_subtests",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args(t)
			var out, errOut strings.Builder
			got := run(args, &out, &errOut)
			if got != tc.wantCode {
				t.Errorf("run(%v) = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, got, tc.wantCode, out.String(), errOut.String())
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout lacks %q:\n%s", want, out.String())
				}
			}
			if !strings.Contains(errOut.String(), tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tc.wantErr)
			}
			if tc.wantErr == "" && errOut.Len() != 0 {
				t.Errorf("stderr should stay empty, got %q", errOut.String())
			}
		})
	}
}

// TestRun_JSONWorkList_DecodesToTheReport verifies the -json file is the
// report itself: it decodes back into the same findings and summary the
// scan produced, terminated by a newline.
func TestRun_JSONWorkList_DecodesToTheReport(t *testing.T) {
	dir := filepath.Dir(writeFixture(t, fixtureSource))
	path := filepath.Join(dir, "work.json")
	if got := run([]string{"-json", path, dir}, &strings.Builder{}, &strings.Builder{}); got != 0 {
		t.Fatalf("run = %d, want 0", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read work list: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("work list lacks the trailing newline")
	}
	var report Report
	if unmarshalErr := json.Unmarshal(data, &report); unmarshalErr != nil {
		t.Fatalf("work list is not JSON: %v", unmarshalErr)
	}
	if report.Summary.Sites != 5 || len(report.Findings) != 5 || len(report.Sequential) != 1 || len(report.Synctest) != 1 {
		t.Errorf("decoded report = %+v, want the fixture's 5 sites, 1 sequential, 1 synctest", report.Summary)
	}
}

// writeDefaultTrees seeds one compliant test file in each of the three trees
// the sweep scans by default. It lives outside the Test function so the
// fixture loop is setup rather than a case table.
func writeDefaultTrees(t *testing.T, root string) {
	t.Helper()
	for _, sub := range []string{"cmd/x", "internal/y", "test/z"} {
		writeFile(t, root, sub+"/clean_test.go", cleanSource)
	}
}

// TestRun_NoDirectories_ScansTheModuleDefaults verifies that with no
// directory argument the sweep covers ./cmd, ./internal and ./test relative
// to the working directory, which is how the Makefile invokes it.
func TestRun_NoDirectories_ScansTheModuleDefaults(t *testing.T) {
	root := t.TempDir()
	writeDefaultTrees(t, root)
	writeFile(t, root, "docs/ignored_test.go", fixtureSource)
	t.Chdir(root)

	var out, errOut strings.Builder
	if got := run([]string{"-check"}, &out, &errOut); got != 0 {
		t.Fatalf("run(-check) = %d, want 0\nstdout:\n%s\nstderr:\n%s", got, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "3 compliant, across 3 file(s)") {
		t.Errorf("stdout = %q, want the three default trees scanned and docs/ left alone", out.String())
	}
}
