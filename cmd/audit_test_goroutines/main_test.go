// Package main tests the goroutine-boundary assertion auditor against a
// fixture file covering every boundary kind and both site categories.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureSource exercises: category A and B t.Fatal sites in an HTTP handler,
// a compliant t.Errorf (return follows), a violating t.Errorf (no return), a
// go-statement abort, an errgroup-style .Go abort, a handler struct field,
// and — as negatives — aborts on the test goroutine plus a helper literal
// that never leaves it.
const fixtureSource = `package fixture

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFixture(t *testing.T) {
	_ = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("category B: handler still writes below") // fatal, B
		}
		w.WriteHeader(http.StatusOK)
	}))

	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("category A: tail position") // fatal, A
	})

	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			t.Errorf("compliant: returns after") // no finding
			http.Error(w, "bad", http.StatusInternalServerError)
			return
		}
		t.Errorf("violating: nothing returns after this") // errorf_no_return
	})

	go func() {
		t.Fatal("go statement abort") // fatal, A
	}()

	var g interface{ Go(func() error) }
	g.Go(func() error {
		t.FailNow() // fatal, B (return follows in source order)
		return nil
	})

	_ = struct{ ElicitationHandler func() }{
		ElicitationHandler: func() {
			t.Fatal("handler field abort") // fatal, A
		},
	}

	// Negatives: test-goroutine aborts must not be flagged.
	t.Fatal("on the test goroutine")
}

func helperOnTestGoroutine(t *testing.T) {
	check := func() {
		t.Fatal("plain literal, never crosses a goroutine boundary") // not flagged
	}
	check()
}
`

// dirtyFixture holds one category A abort and one advisory errorf site, the
// smallest tree that exercises every line of the human report.
const dirtyFixture = `package fixture

import (
	"net/http"
	"testing"
)

func TestDirty(t *testing.T) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("abort off the test goroutine")
	})
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("advisory: no return follows")
	})
}
`

// cleanFixture holds a handler that follows the contract and an abort on the
// test goroutine, so a scan of it must report nothing.
const cleanFixture = `package fixture

import (
	"net/http"
	"testing"
)

func TestClean(t *testing.T) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("wrong method")
			http.Error(w, "wrong method", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	t.Fatal("test goroutine aborts stay legal")
}
`

// TestScan_FixtureClassification verifies the scanner finds exactly the
// planted sites, classifies A/B correctly, honors the compliant
// errorf-then-return shape, and ignores test-goroutine aborts.
func TestScan_FixtureClassification(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture_test.go"), []byte(fixtureSource), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	report, err := scan([]string{dir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if got, want := report.Summary.FatalSites, 5; got != want {
		t.Errorf("fatal sites = %d, want %d: %+v", got, want, report.Fatal)
	}
	if got, want := report.Summary.CategoryA, 3; got != want {
		t.Errorf("category A = %d, want %d", got, want)
	}
	if got, want := report.Summary.CategoryB, 2; got != want {
		t.Errorf("category B = %d, want %d", got, want)
	}
	if got, want := report.Summary.ErrorfNoReturn, 1; got != want {
		t.Errorf("errorf_no_return = %d, want %d: %+v", got, want, report.ErrorfNoReturn)
	}

	boundaries := map[string]bool{}
	for _, f := range report.Fatal {
		boundaries[f.Boundary] = true
	}
	for _, want := range []string{"http.HandlerFunc", "go statement", ".Go(...)", "handler field ElicitationHandler"} {
		t.Run(want, func(t *testing.T) {
			if !boundaries[want] {
				t.Errorf("missing boundary kind %q in %v", want, boundaries)
			}
		})
	}
}

// TestScan_CleanFileHasNoFindings verifies a file whose handlers follow the
// contract produces an empty report.
func TestScan_CleanFileHasNoFindings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clean_test.go"), []byte(cleanFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	report, err := scan([]string{dir})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if report.Summary.FatalSites != 0 || report.Summary.ErrorfNoReturn != 0 {
		t.Fatalf("clean file produced findings: %+v", report.Summary)
	}
}

// expectedSite names one finding by the marker comment on its source line,
// so the fixture can be edited without recounting line numbers.
type expectedSite struct {
	marker   string
	call     string
	kind     string
	category string
	boundary string
}

// TestScan_BoundaryKinds_ReportsExactFindings verifies every registration
// shape the auditor recognizes: ServeMux HandleFunc, generic MCP AddTool,
// resource, template and prompt registrations (handler last), middleware
// (handler first), generic .Go, benchmark and testing.TB receivers, nested
// literals inheriting the goroutine, and the t.Error variant of the
// missing-return rule. The shapes the auditor must ignore are planted as one
// case that expects nothing.
func TestScan_BoundaryKinds_ReportsExactFindings(t *testing.T) {
	testCases := []struct {
		name   string
		source string
		want   []expectedSite
	}{
		{
			name: "HandleFunc registration audits the second argument",
			source: `package fixture

import (
	"net/http"
	"testing"
)

func TestMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("registered handler") // SITE
	})
	mux.HandleFunc("/pattern-only")
}
`,
			want: []expectedSite{{marker: "SITE", call: "t.Fatal", kind: "fatal", category: "A", boundary: "HandleFunc"}},
		},
		{
			name: "generic AddTool audits the last argument",
			source: `package fixture

import "testing"

func TestTool(t *testing.T) {
	mcp.AddTool[In, Out](server, tool, func(ctx context.Context, req *Req, in In) (*Res, Out, error) {
		t.Fatalf("tool handler %d", 1) // SITE
		return nil, Out{}, nil
	})
}
`,
			want: []expectedSite{{marker: "SITE", call: "t.Fatalf", kind: "fatal", category: "B", boundary: "mcp AddTool"}},
		},
		{
			name: "generic Go audits the first argument",
			source: `package fixture

import "testing"

func TestGroup(t *testing.T) {
	g.Go[int](func() error {
		t.FailNow() // SITE
		return nil
	})
}
`,
			want: []expectedSite{{marker: "SITE", call: "t.FailNow", kind: "fatal", category: "B", boundary: ".Go(...)"}},
		},
		{
			name: "resource template and prompt registrations audit the last argument",
			source: `package fixture

import "testing"

func TestRegistrations(t *testing.T) {
	server.AddResource(res, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		t.Fatal("resource") // RESOURCE
	})
	server.AddResourceTemplate(tpl, func() {
		t.Fatal("template") // TEMPLATE
	})
	server.AddPrompt(prompt, func() {
		t.Fatal("prompt") // PROMPT
	})
}
`,
			want: []expectedSite{
				{marker: "RESOURCE", call: "t.Fatal", kind: "fatal", category: "A", boundary: "mcp AddResource"},
				{marker: "TEMPLATE", call: "t.Fatal", kind: "fatal", category: "A", boundary: "mcp AddResourceTemplate"},
				{marker: "PROMPT", call: "t.Fatal", kind: "fatal", category: "A", boundary: "mcp AddPrompt"},
			},
		},
		{
			name: "middleware registrations audit the first argument",
			source: `package fixture

import "testing"

func TestMiddleware(t *testing.T) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		t.Fatal("receiving") // RECEIVING
		return next
	}, other)
	server.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		t.Fatal("sending") // SENDING
		return next
	})
}
`,
			want: []expectedSite{
				{marker: "RECEIVING", call: "t.Fatal", kind: "fatal", category: "B", boundary: "mcp AddReceivingMiddleware"},
				{marker: "SENDING", call: "t.Fatal", kind: "fatal", category: "B", boundary: "mcp AddSendingMiddleware"},
			},
		},
		{
			name: "benchmark and testing.TB receivers are audited",
			source: `package fixture

import (
	"net/http"
	"testing"
)

func BenchmarkHandler(b *testing.B) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.Fatal("bench") // BENCH
	})
}

func helper(tb testing.TB) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tb.FailNow() // TBSITE
	})
}
`,
			want: []expectedSite{
				{marker: "BENCH", call: "b.Fatal", kind: "fatal", category: "A", boundary: "http.HandlerFunc"},
				{marker: "TBSITE", call: "tb.FailNow", kind: "fatal", category: "A", boundary: "http.HandlerFunc"},
			},
		},
		{
			name: "nested literal inherits the handler goroutine",
			source: `package fixture

import (
	"net/http"
	"testing"
)

func TestNested(t *testing.T) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check := func() {
			t.Fatal("nested") // SITE
		}
		check()
	})
}
`,
			want: []expectedSite{{marker: "SITE", call: "t.Fatal", kind: "fatal", category: "B", boundary: "http.HandlerFunc"}},
		},
		{
			name: "t.Error without return is advisory and guarded t.Errorf is not",
			source: `package fixture

import (
	"net/http"
	"testing"
)

func TestErrorf(t *testing.T) {
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "" {
			t.Errorf("guarded: the block returns")
			return
		}
		t.Error("bare") // SITE
		t.Log("logging is never a finding")
	})
}
`,
			want: []expectedSite{{marker: "SITE", call: "t.Error", kind: "errorf_no_return", boundary: "http.HandlerFunc"}},
		},
		{
			name: "shapes outside the contract report nothing",
			source: `package fixture

import (
	"net/http"
	"testing"
)

func TestSkipped(t *testing.T) {
	s := struct{ Fatal func(string) }{}
	_ = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Fatal("not a testing receiver")
		helper()
		t.Log("not an assertion")
	})
	_ = map[string]func(){"OnHandler": func() { t.Fatal("string key") }}
	_ = struct{ Callback func() }{Callback: func() { t.Fatal("key without Handler suffix") }}
	_ = struct{ ElicitationHandler func() }{ElicitationHandler: handler}
	func() { t.Fatal("immediately invoked literal") }()
	http.HandleFunc("/pattern-only")
	other.Register(func() { t.Fatal("unknown registration") })
}
`,
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "shape_test.go")
			if err := os.WriteFile(path, []byte(tc.source), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			report, err := scan([]string{dir})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}

			want := []Finding{}
			for _, site := range tc.want {
				want = append(want, Finding{
					File: path, Line: lineOf(t, tc.source, site.marker),
					Call: site.call, Kind: site.kind, Category: site.category, Boundary: site.boundary,
				})
			}
			got := append(append([]Finding{}, report.Fatal...), report.ErrorfNoReturn...)
			sortFindings(got)
			sortFindings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("findings = %+v\nwant %+v", got, want)
			}
		})
	}
}

// TestScan_SkipsIgnoredTreesAndNonTestFiles verifies node_modules, testdata
// and dot-directories are pruned and that a non-test Go file is never parsed,
// even when each holds an abort inside a handler literal.
func TestScan_SkipsIgnoredTreesAndNonTestFiles(t *testing.T) {
	root := t.TempDir()
	mkdirTree(t, root, []string{"node_modules", "testdata", ".hidden"}, "skip_test.go", dirtyFixture)
	if err := os.WriteFile(filepath.Join(root, "notatest.go"), []byte(dirtyFixture), 0o600); err != nil {
		t.Fatalf("write non-test fixture: %v", err)
	}

	report, err := scan([]string{root})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if want := (Summary{}); report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
	if len(report.Fatal) != 0 || len(report.ErrorfNoReturn) != 0 {
		t.Fatalf("pruned trees produced findings: %+v %+v", report.Fatal, report.ErrorfNoReturn)
	}
}

// TestScan_MissingDirectory_ReturnsError verifies a directory that does not
// exist fails the scan instead of being reported as clean.
func TestScan_MissingDirectory_ReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	report, err := scan([]string{missing})
	if err == nil {
		t.Fatalf("scan(%q) = %+v, want error", missing, report)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("scan error = %q, want it to name the missing directory", err)
	}
}

// TestScan_InvalidSource_ReturnsParseError verifies a test file that does not
// parse aborts the scan with the file named in the error.
func TestScan_InvalidSource_ReturnsParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken_test.go")
	if err := os.WriteFile(path, []byte("package fixture\n\nfunc (\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := scan([]string{dir})
	if err == nil {
		t.Fatal("scan of unparseable source returned nil error")
	}
	if !strings.HasPrefix(err.Error(), "parse "+path+": ") {
		t.Fatalf("scan error = %q, want parse error naming %s", err, path)
	}
}

// TestSortFindings_OrdersByFileThenLine verifies the stable ordering the
// work list relies on: file paths first, line numbers within a file.
func TestSortFindings_OrdersByFileThenLine(t *testing.T) {
	findings := []Finding{
		{File: "b_test.go", Line: 2},
		{File: "a_test.go", Line: 9},
		{File: "b_test.go", Line: 1},
	}
	sortFindings(findings)
	want := []Finding{
		{File: "a_test.go", Line: 9},
		{File: "b_test.go", Line: 1},
		{File: "b_test.go", Line: 2},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Fatalf("sorted = %+v, want %+v", findings, want)
	}
}

// TestRun_ModesAndExitCodes verifies the command's contract end to end: the
// human report, the -check verdict and its exit code, the JSON work list, and
// the exit code 2 paths for a failed scan or an unwritable work list.
func TestRun_ModesAndExitCodes(t *testing.T) {
	testCases := []struct {
		name       string
		fixture    string
		jsonPath   func(dir string) string
		dirs       func(dir string) []string
		check      bool
		wantExit   int
		wantStdout func(dir, file, jsonPath string) string
		wantStderr func(dir, jsonPath string) string
	}{
		{
			name:     "clean tree without check prints only the summary",
			fixture:  cleanFixture,
			wantExit: 0,
			wantStdout: func(_, _, _ string) string {
				return "\nsummary: 0 fatal sites (A=0 tail-position, B=0 truncating) + 0 advisory errorf-without-return across 0 files\n"
			},
		},
		{
			name:     "clean tree with check passes",
			fixture:  cleanFixture,
			check:    true,
			wantExit: 0,
			wantStdout: func(_, _, _ string) string {
				return "\nsummary: 0 fatal sites (A=0 tail-position, B=0 truncating) + 0 advisory errorf-without-return across 0 files\n" +
					"check: PASS. No testing.T aborts off the test goroutine (0 advisory errorf site(s))\n"
			},
		},
		{
			name:     "abort site with check fails with exit code 1",
			fixture:  dirtyFixture,
			check:    true,
			wantExit: 1,
			wantStdout: func(_, file, _ string) string {
				return fmt.Sprintf("%-72s fatal=%-3d errorf_no_return=%d\n", file, 1, 1) +
					"\nsummary: 1 fatal sites (A=1 tail-position, B=0 truncating) + 1 advisory errorf-without-return across 1 files\n" +
					"check: FAIL. 1 abort site(s) off the test goroutine (1 advisory errorf sites not gated)\n"
			},
		},
		{
			name:     "abort site without check only reports",
			fixture:  dirtyFixture,
			wantExit: 0,
			wantStdout: func(_, file, _ string) string {
				return fmt.Sprintf("%-72s fatal=%-3d errorf_no_return=%d\n", file, 1, 1) +
					"\nsummary: 1 fatal sites (A=1 tail-position, B=0 truncating) + 1 advisory errorf-without-return across 1 files\n"
			},
		},
		{
			name:     "json path writes the work list",
			fixture:  dirtyFixture,
			jsonPath: func(dir string) string { return filepath.Join(dir, "work.json") },
			wantExit: 0,
			wantStdout: func(_, file, jsonPath string) string {
				return fmt.Sprintf("%-72s fatal=%-3d errorf_no_return=%d\n", file, 1, 1) +
					"\nsummary: 1 fatal sites (A=1 tail-position, B=0 truncating) + 1 advisory errorf-without-return across 1 files\n" +
					"work list written to " + jsonPath + "\n"
			},
		},
		{
			name:     "unwritable json path exits with code 2",
			fixture:  cleanFixture,
			jsonPath: func(dir string) string { return filepath.Join(dir, "missing", "work.json") },
			wantExit: 2,
			wantStdout: func(_, _, _ string) string {
				return "\nsummary: 0 fatal sites (A=0 tail-position, B=0 truncating) + 0 advisory errorf-without-return across 0 files\n"
			},
			wantStderr: func(_, jsonPath string) string {
				return "audit_test_goroutines: write " + jsonPath + ": "
			},
		},
		{
			name:     "missing directory exits with code 2",
			fixture:  cleanFixture,
			dirs:     func(dir string) []string { return []string{filepath.Join(dir, "absent")} },
			wantExit: 2,
			wantStdout: func(_, _, _ string) string {
				return ""
			},
			wantStderr: func(_, _ string) string {
				return "audit_test_goroutines: "
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "fixture_test.go")
			if err := os.WriteFile(file, []byte(tc.fixture), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			jsonPath := ""
			if tc.jsonPath != nil {
				jsonPath = tc.jsonPath(dir)
			}
			dirs := []string{dir}
			if tc.dirs != nil {
				dirs = tc.dirs(dir)
			}

			var stdout, stderr bytes.Buffer
			got := runOutcome{exit: run(dirs, jsonPath, tc.check, &stdout, &stderr)}
			got.stdout, got.stderr = stdout.String(), stderr.String()

			wantStderr := ""
			if tc.wantStderr != nil {
				wantStderr = tc.wantStderr(dir, jsonPath)
			}
			assertRunOutcome(t, got, tc.wantExit, tc.wantStdout(dir, file, jsonPath), wantStderr)
			if jsonPath != "" && tc.wantExit == 0 {
				assertWorkList(t, jsonPath, file)
			}
		})
	}
}

// runOutcome captures what one run invocation produced.
type runOutcome struct {
	exit           int
	stdout, stderr string
}

// assertRunOutcome compares an invocation against the expected exit code,
// the exact stdout, and a stderr prefix (an empty prefix demands no stderr).
func assertRunOutcome(t *testing.T, got runOutcome, wantExit int, wantStdout, wantStderrPrefix string) {
	t.Helper()
	if got.exit != wantExit {
		t.Errorf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", got.exit, wantExit, got.stdout, got.stderr)
	}
	if got.stdout != wantStdout {
		t.Errorf("stdout = %q\nwant %q", got.stdout, wantStdout)
	}
	if wantStderrPrefix == "" && got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
	if !strings.HasPrefix(got.stderr, wantStderrPrefix) {
		t.Errorf("stderr = %q, want prefix %q", got.stderr, wantStderrPrefix)
	}
}

// TestRun_DefaultDirectories_ScansModuleTrees verifies that with no
// directories the command scans cmd, internal and test relative to the
// working directory, reporting paths relative to it.
func TestRun_DefaultDirectories_ScansModuleTrees(t *testing.T) {
	root := t.TempDir()
	mkdirTree(t, root, []string{"cmd", "internal", "test"}, "", "")
	if err := os.WriteFile(filepath.Join(root, "cmd", "a_test.go"), []byte(dirtyFixture), 0o600); err != nil {
		t.Fatalf("write cmd fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "test", "b_test.go"), []byte(cleanFixture), 0o600); err != nil {
		t.Fatalf("write test fixture: %v", err)
	}
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	if exit := run(nil, "", false, &stdout, &stderr); exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", exit, stderr.String())
	}
	want := fmt.Sprintf("%-72s fatal=%-3d errorf_no_return=%d\n", filepath.Join("cmd", "a_test.go"), 1, 1) +
		"\nsummary: 1 fatal sites (A=1 tail-position, B=0 truncating) + 1 advisory errorf-without-return across 1 files\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q\nwant %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

// assertWorkList decodes the JSON work list at path and checks it names the
// dirty fixture's two sites with the same classification the scan produced.
func assertWorkList(t *testing.T, path, file string) {
	t.Helper()
	data, err := os.ReadFile(path) //#nosec G304 -- test fixture path from t.TempDir.
	if err != nil {
		t.Fatalf("read work list: %v", err)
	}
	var got Report
	if unmarshalErr := json.Unmarshal(data, &got); unmarshalErr != nil {
		t.Fatalf("decode work list: %v\n%s", unmarshalErr, data)
	}
	want := Report{
		Fatal:          []Finding{{File: file, Line: lineOf(t, dirtyFixture, `t.Fatal("abort`), Call: "t.Fatal", Kind: "fatal", Category: "A", Boundary: "http.HandlerFunc"}},
		ErrorfNoReturn: []Finding{{File: file, Line: lineOf(t, dirtyFixture, `t.Errorf("advisory`), Call: "t.Errorf", Kind: "errorf_no_return", Boundary: "http.HandlerFunc"}},
		Summary:        Summary{FatalSites: 1, CategoryA: 1, ErrorfNoReturn: 1, Files: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("work list = %+v\nwant %+v", got, want)
	}
	if !bytes.HasSuffix(data, []byte("}\n")) {
		t.Fatalf("work list does not end with a newline: %q", data[len(data)-4:])
	}
}

// mkdirTree creates every named subdirectory under root and, when name is
// set, writes fixture into each of them under that file name.
func mkdirTree(t *testing.T, root string, subs []string, name, fixture string) {
	t.Helper()
	for _, sub := range subs {
		dir := filepath.Join(root, sub)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		if name == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fixture), 0o600); err != nil {
			t.Fatalf("write %s fixture: %v", sub, err)
		}
	}
}

// lineOf returns the 1-based line of the first source line containing marker.
func lineOf(t *testing.T, src, marker string) int {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			return i + 1
		}
	}
	t.Fatalf("marker %q not found in fixture", marker)
	return 0
}
