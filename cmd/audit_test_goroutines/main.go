// Command audit_test_goroutines detects testing.T abort calls made from
// goroutines other than the one running the test.
//
// The testing package documents that FailNow — and therefore t.Fatal and
// t.Fatalf — must be called from the test goroutine. Inside an HTTP mock
// handler, a go statement, an errgroup task, or an MCP tool handler, the call
// instead terminates only that goroutine: the response is truncated or never
// written, the client observes a transport error, and the test continues
// against the wreckage. go vet's testinggoroutine analyzer flags bare
// `go func() { t.Fatal() }()` but cannot see that a function literal passed
// to http.HandlerFunc or mcp.AddTool crosses a goroutine boundary, which is
// how these sites accumulate unnoticed.
//
// The tool reports every t.Fatal/t.Fatalf/t.FailNow call inside such a
// literal, classifies each site as category A (the abort is in tail position:
// nothing else would have run) or category B (the handler still had work to
// do, so the response is observably truncated), and separately reports
// t.Error/t.Errorf calls whose enclosing block does not return afterwards —
// the conversion contract requires an explicit return so a later edit cannot
// silently reintroduce dead code paths (the defect class behind PR #270's
// nil-dereference panic).
//
// Usage:
//
//	go run ./cmd/audit_test_goroutines [-json out.json] [-check] [dirs...]
//
// With no directories the module's test files under ./cmd, ./internal, and
// ./test are scanned. -check exits non-zero when any finding exists, so the
// tool can gate CI once the sweep lands.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding describes one abort or missing-return site inside a non-test
// goroutine literal.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Call     string `json:"call"`               // t.Fatal, t.Fatalf, t.FailNow, t.Error, t.Errorf
	Kind     string `json:"kind"`               // fatal | errorf_no_return
	Category string `json:"category,omitempty"` // A (tail position) or B (work remained) for fatal sites
	Boundary string `json:"boundary"`           // what makes the literal run off the test goroutine
}

// Report is the JSON work list consumed by the conversion batches.
type Report struct {
	Fatal          []Finding `json:"fatal"`
	ErrorfNoReturn []Finding `json:"errorf_no_return"`
	Summary        Summary   `json:"summary"`
}

// Summary aggregates the counts the sweep plan tracks.
type Summary struct {
	FatalSites     int `json:"fatal_sites"`
	CategoryA      int `json:"category_a"`
	CategoryB      int `json:"category_b"`
	ErrorfNoReturn int `json:"errorf_no_return"`
	Files          int `json:"files"`
}

// abortNames are the testing.T methods that call FailNow under the hood.
var abortNames = map[string]bool{"Fatal": true, "Fatalf": true, "FailNow": true}

// errorNames are the non-aborting assertion methods checked for the
// missing-return contract.
var errorNames = map[string]bool{"Error": true, "Errorf": true}

func main() {
	jsonPath := flag.String("json", "", "write the JSON work list to this path")
	check := flag.Bool("check", false, "exit non-zero when any abort (Fatal/FailNow) site exists; errorf sites stay advisory")
	flag.Parse()

	os.Exit(run(flag.Args(), *jsonPath, *check, os.Stdout, os.Stderr))
}

// run scans dirs (the module's cmd, internal and test trees when empty),
// prints the human report to stdout, writes the JSON work list when jsonPath
// is set, and returns the process exit code: 2 when the scan or the write
// fails, 1 when check is set and an abort site exists, 0 otherwise.
func run(dirs []string, jsonPath string, check bool, stdout, stderr io.Writer) int {
	if len(dirs) == 0 {
		dirs = []string{"cmd", "internal", "test"}
	}

	report, err := scan(dirs)
	if err != nil {
		fmt.Fprintf(stderr, "audit_test_goroutines: %v\n", err)
		return 2
	}

	printHuman(stdout, report)

	if jsonPath != "" {
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			fmt.Fprintf(stderr, "audit_test_goroutines: marshal: %v\n", marshalErr)
			return 2
		}
		if writeErr := os.WriteFile(jsonPath, append(data, '\n'), 0o600); writeErr != nil {
			fmt.Fprintf(stderr, "audit_test_goroutines: write %s: %v\n", jsonPath, writeErr)
			return 2
		}
		fmt.Fprintf(stdout, "work list written to %s\n", jsonPath)
	}

	// Pilot amendment (2026-08-17): only abort sites gate. The
	// errorf-without-return list stays advisory — the sweep found that most
	// of those sites are the legitimate assert-then-respond shape, where the
	// handler still writes its canned response and no invalid state is used;
	// rule 2 of the contract applies to converted Fatal guards, which review
	// and the testutil helpers cover.
	if check && len(report.Fatal) > 0 {
		fmt.Fprintf(stdout, "check: FAIL. %d abort site(s) off the test goroutine (%d advisory errorf sites not gated)\n",
			len(report.Fatal), len(report.ErrorfNoReturn))
		return 1
	}
	if check {
		fmt.Fprintf(stdout, "check: PASS. No testing.T aborts off the test goroutine (%d advisory errorf site(s))\n",
			len(report.ErrorfNoReturn))
	}
	return 0
}

// scan walks every _test.go file under dirs and collects findings.
func scan(dirs []string) (*Report, error) {
	report := &Report{}
	files := map[string]bool{}
	fset := token.NewFileSet()

	for _, dir := range dirs {
		if walkErr := filepath.WalkDir(dir, collectFindings(fset, report, files)); walkErr != nil {
			return nil, walkErr
		}
	}

	sortFindings(report.Fatal)
	sortFindings(report.ErrorfNoReturn)
	report.Summary = Summary{
		FatalSites:     len(report.Fatal),
		ErrorfNoReturn: len(report.ErrorfNoReturn),
		Files:          len(files),
	}
	for _, f := range report.Fatal {
		if f.Category == "A" {
			report.Summary.CategoryA++
		} else {
			report.Summary.CategoryB++
		}
	}
	return report, nil
}

// collectFindings returns the WalkDir callback that parses each _test.go
// file and appends its findings to the report.
func collectFindings(fset *token.FileSet, report *Report, files map[string]bool) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "node_modules" || name == "testdata" || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		for _, f := range scanFile(fset, path, file) {
			files[f.File] = true
			if f.Kind == "fatal" {
				report.Fatal = append(report.Fatal, f)
			} else {
				report.ErrorfNoReturn = append(report.ErrorfNoReturn, f)
			}
		}
		return nil
	}
}

// scanFile finds goroutine-boundary literals in one file and audits their
// bodies.
func scanFile(fset *token.FileSet, path string, file *ast.File) []Finding {
	var findings []Finding
	seen := map[*ast.FuncLit]bool{}

	audit := func(lit *ast.FuncLit, boundary string) {
		if lit == nil || seen[lit] {
			return
		}
		seen[lit] = true
		findings = append(findings, auditLiteral(fset, path, lit, boundary)...)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GoStmt:
			if lit, ok := node.Call.Fun.(*ast.FuncLit); ok {
				audit(lit, "go statement")
			}
		case *ast.CallExpr:
			boundary, argIdx := boundaryCall(node)
			if boundary == "" {
				return true
			}
			if argIdx < len(node.Args) {
				if lit, ok := node.Args[argIdx].(*ast.FuncLit); ok {
					audit(lit, boundary)
				}
			}
		case *ast.KeyValueExpr:
			key, ok := node.Key.(*ast.Ident)
			if !ok || !strings.HasSuffix(key.Name, "Handler") {
				return true
			}
			if lit, isLit := node.Value.(*ast.FuncLit); isLit {
				audit(lit, "handler field "+key.Name)
			}
		}
		return true
	})
	return findings
}

// boundaryCall reports whether call hands a function literal to another
// goroutine, returning a label and the argument index that carries the
// literal. Conversions like http.HandlerFunc(lit) have the literal at index
// 0; mux.HandleFunc(pattern, lit) at index 1.
func boundaryCall(call *ast.CallExpr) (label string, argIndex int) {
	switch name := calleeName(call.Fun); name {
	case "HandlerFunc": // the http.HandlerFunc conversion around a literal
		return "http.HandlerFunc", 0
	case "HandleFunc": // ServeMux registration: pattern first, literal second
		return "HandleFunc", 1
	case "Go": // errgroup.Group.Go and friends
		return ".Go(...)", 0
	case "AddTool", "AddResource", "AddResourceTemplate", "AddPrompt":
		// MCP server registrations: handlers run on the serving session's
		// goroutine. The handler is the last argument.
		return "mcp " + name, len(call.Args) - 1
	case "AddReceivingMiddleware", "AddSendingMiddleware":
		return "mcp " + name, 0
	default:
		return "", 0
	}
}

// calleeName extracts the terminal identifier of a call target.
func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.IndexExpr: // generic instantiation
		return calleeName(f.X)
	case *ast.IndexListExpr:
		return calleeName(f.X)
	default:
		return ""
	}
}

// auditLiteral reports abort calls and missing-return Errorf calls in the
// literal's body, including nested literals (they inherit the goroutine).
func auditLiteral(fset *token.FileSet, path string, lit *ast.FuncLit, boundary string) []Finding {
	var findings []Finding
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok || (recv.Name != "t" && recv.Name != "b" && recv.Name != "tb") {
			return true
		}
		pos := fset.Position(call.Pos())
		switch {
		case abortNames[sel.Sel.Name]:
			category := "A"
			if hasWorkAfter(lit, call) {
				category = "B"
			}
			findings = append(findings, Finding{
				File: path, Line: pos.Line,
				Call: recv.Name + "." + sel.Sel.Name,
				Kind: "fatal", Category: category, Boundary: boundary,
			})
		case errorNames[sel.Sel.Name]:
			if !returnsAfter(lit, call) {
				findings = append(findings, Finding{
					File: path, Line: pos.Line,
					Call: recv.Name + "." + sel.Sel.Name,
					Kind: "errorf_no_return", Boundary: boundary,
				})
			}
		}
		return true
	})
	return findings
}

// hasWorkAfter reports whether any statement in the literal begins after the
// call ends — category B: the abort truncates work the handler still owed.
func hasWorkAfter(lit *ast.FuncLit, call *ast.CallExpr) bool {
	work := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if work || n == nil {
			return false
		}
		if stmt, ok := n.(ast.Stmt); ok && stmt.Pos() > call.End() {
			work = true
			return false
		}
		return true
	})
	return work
}

// returnsAfter reports whether the statement list that DIRECTLY contains the
// call has an explicit return after it — the contract's rule 2. Only the
// innermost block matters: a compliant guard is `t.Errorf; respond; return`
// inside its own if-body, regardless of what the outer block does next.
func returnsAfter(lit *ast.FuncLit, call *ast.CallExpr) bool {
	found := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range block.List {
			expr, isExpr := stmt.(*ast.ExprStmt)
			if !isExpr || expr.X != ast.Expr(call) {
				continue
			}
			for _, later := range block.List[i+1:] {
				if _, isReturn := later.(*ast.ReturnStmt); isReturn {
					found = true
					break
				}
			}
			return false
		}
		return true
	})
	return found
}

// sortFindings orders findings by file then line for stable output.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}

// printHuman writes the per-file tallies and the summary to w.
func printHuman(w io.Writer, report *Report) {
	perFile := map[string][2]int{}
	for _, f := range report.Fatal {
		c := perFile[f.File]
		c[0]++
		perFile[f.File] = c
	}
	for _, f := range report.ErrorfNoReturn {
		c := perFile[f.File]
		c[1]++
		perFile[f.File] = c
	}
	files := make([]string, 0, len(perFile))
	for f := range perFile {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		c := perFile[f]
		fmt.Fprintf(w, "%-72s fatal=%-3d errorf_no_return=%d\n", f, c[0], c[1])
	}
	fmt.Fprintf(w, "\nsummary: %d fatal sites (A=%d tail-position, B=%d truncating) + %d advisory errorf-without-return across %d files\n",
		report.Summary.FatalSites, report.Summary.CategoryA, report.Summary.CategoryB,
		report.Summary.ErrorfNoReturn, report.Summary.Files)
}
