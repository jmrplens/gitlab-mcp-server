// Command audit_godocs audits Go package and symbol documentation.
//
// It parses the repository with go/parser and go/doc, then reports package
// comments, exported symbol comments, and optional test documentation issues in
// Markdown or JSON format.
//
// Usage:
//
//	go run ./cmd/audit_godocs/ --format=markdown
//	go run ./cmd/audit_godocs/ --include-tests --fail-on-findings
//
// # Reported Categories
//
// Findings are grouped by package comments, exported declarations, and optional
// test declarations. Package findings catch missing, malformed, or duplicated
// package comments; symbol findings catch exported declarations without useful
// comments; and test findings catch Test, Benchmark, Fuzz, and Example functions
// whose comments are missing or do not start with the function name.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	categoryPackageDocMissing  = "pkgdoc_missing"
	categoryPackageDocForm     = "pkgdoc_form"
	categoryPackageDocMultiple = "pkgdoc_multiple"
	categoryFuncMissing        = "func_missing"
	categoryFuncForm           = "func_form"
	categoryMethodMissing      = "method_missing"
	categoryMethodForm         = "method_form"
	categoryTypeMissing        = "type_missing"
	categoryTypeForm           = "type_form"
	categoryConstMissing       = "const_missing"
	categoryConstForm          = "const_form"
	categoryVarMissing         = "var_missing"
	categoryVarForm            = "var_form"
	categoryTestMissing        = "test_missing"
	categoryTestForm           = "test_form"
	categoryBenchmarkMissing   = "benchmark_missing"
	categoryBenchmarkForm      = "benchmark_form"
	categoryFuzzMissing        = "fuzz_missing"
	categoryFuzzForm           = "fuzz_form"
	categoryExampleMissing     = "example_missing"
	categoryExampleForm        = "example_form"
	categoryExampleOutput      = "example_missing_output"

	formatMarkdown = "markdown"
	formatJSON     = "json"
)

// options controls how the documentation audit scans and reports packages.
type options struct {
	format         string
	outputPath     string
	includeTests   bool
	failOnFindings bool
	ignoreInternal bool
}

// packageInfo identifies one Go package returned by go list.
type packageInfo struct {
	Dir        string `json:"dir"`
	ImportPath string `json:"import_path"`
	Name       string `json:"name"`
}

// finding describes one documentation issue found in a package.
type finding struct {
	Category   string `json:"category"`
	ImportPath string `json:"import_path"`
	Package    string `json:"package"`
	File       string `json:"file,omitempty"`
	Name       string `json:"name"`
	Detail     string `json:"detail"`
}

// report is the complete machine-readable audit result.
type report struct {
	GeneratedAt  string         `json:"generated_at"`
	IncludeTests bool           `json:"include_tests"`
	Packages     int            `json:"packages"`
	Findings     []finding      `json:"findings"`
	ByCategory   map[string]int `json:"by_category"`
	ByPackage    map[string]int `json:"by_package"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses CLI arguments, audits packages, writes the report, and optionally
// fails when findings remain.
func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	auditReport, err := auditRepository(opts)
	if err != nil {
		return err
	}

	rendered, err := renderReport(auditReport, opts.format)
	if err != nil {
		return err
	}
	if opts.outputPath != "" {
		// #nosec G304,G703 -- output path is an explicit local developer CLI destination.
		if writeErr := os.WriteFile(filepath.Clean(opts.outputPath), rendered, 0o600); writeErr != nil {
			return fmt.Errorf("write report: %w", writeErr)
		}
	} else if _, writeErr := stdout.Write(rendered); writeErr != nil {
		return fmt.Errorf("write stdout: %w", writeErr)
	}

	if opts.failOnFindings && len(auditReport.Findings) > 0 {
		return fmt.Errorf("godoc audit found %d issue(s)", len(auditReport.Findings))
	}
	return nil
}

// parseOptions validates CLI flags for the audit command.
func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("audit_godocs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := options{}
	fs.StringVar(&opts.format, "format", formatMarkdown, "report format: markdown or json")
	fs.StringVar(&opts.outputPath, "output", "", "write report to this path instead of stdout")
	fs.BoolVar(&opts.includeTests, "include-tests", false, "also audit Test, Benchmark, Fuzz, and Example functions in _test.go files")
	fs.BoolVar(&opts.failOnFindings, "fail-on-findings", false, "exit non-zero when findings are present")
	fs.BoolVar(&opts.ignoreInternal, "ignore-internal", false, "skip packages whose import path contains /internal/")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.format != formatMarkdown && opts.format != formatJSON {
		return options{}, fmt.Errorf("unsupported format %q", opts.format)
	}
	return opts, nil
}

// auditRepository runs go list and audits every discovered package.
func auditRepository(opts options) (report, error) {
	packages, err := listPackages()
	if err != nil {
		return report{}, err
	}

	findings := []finding{}
	auditedPackages := 0
	for _, pkg := range packages {
		if opts.ignoreInternal && strings.Contains(pkg.ImportPath, "/internal/") {
			continue
		}
		pkgFindings, auditErr := auditPackage(pkg, opts.includeTests)
		if auditErr != nil {
			return report{}, auditErr
		}
		auditedPackages++
		findings = append(findings, pkgFindings...)
	}
	sortFindings(findings)

	return report{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		IncludeTests: opts.includeTests,
		Packages:     auditedPackages,
		Findings:     findings,
		ByCategory:   countBy(findings, func(f finding) string { return f.Category }),
		ByPackage:    countBy(findings, func(f finding) string { return f.ImportPath }),
	}, nil
}

// listPackages returns all packages in the current module using go list.
func listPackages() ([]packageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// #nosec G204 -- goExecutable returns the fixed Go tool path from the active runtime installation.
	cmd := exec.CommandContext(ctx, goExecutable(), "list", "-f", "{{.Dir}}\t{{.ImportPath}}\t{{.Name}}", "./...")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	packages := []packageInfo{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		dir, remainder, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("unexpected go list row: %q", line)
		}
		importPath, name, ok := strings.Cut(remainder, "\t")
		if !ok {
			return nil, fmt.Errorf("unexpected go list row: %q", line)
		}
		packages = append(packages, packageInfo{Dir: dir, ImportPath: importPath, Name: name})
	}
	return packages, nil
}

// goExecutable returns the absolute Go tool path from the runtime installation.
func goExecutable() string {
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	//lint:ignore SA1019 command audit must avoid PATH lookup for Sonar go:S4036.
	return filepath.Join(runtime.GOROOT(), "bin", name) //nolint:staticcheck // Avoid PATH lookup for Sonar go:S4036.
}

// auditPackage parses source files for one package and checks documentation.
func auditPackage(pkg packageInfo, includeTests bool) ([]finding, error) {
	parsed, err := parsePackageFiles(pkg)
	if err != nil {
		return nil, err
	}
	if len(parsed.sourceFiles) == 0 {
		return nil, nil
	}

	findings := []finding{}
	checkPackageDocs(pkg, parsed.sourceFiles, &findings)
	if docErr := checkExportedDocs(pkg, parsed, &findings); docErr != nil {
		return nil, docErr
	}
	if includeTests {
		checkTestDocs(pkg, parsed.testFiles, &findings)
	}
	return findings, nil
}

type parsedPackage struct {
	fset        *token.FileSet
	sourceFiles map[string]*ast.File
	testFiles   map[string]*ast.File
}

func parsePackageFiles(pkg packageInfo) (parsedPackage, error) {
	entries, err := os.ReadDir(pkg.Dir)
	if err != nil {
		return parsedPackage{}, fmt.Errorf("read package dir %s: %w", pkg.Dir, err)
	}

	fset := token.NewFileSet()
	parsed := parsedPackage{
		fset:        fset,
		sourceFiles: map[string]*ast.File{},
		testFiles:   map[string]*ast.File{},
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(pkg.Dir, entry.Name())
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parsedPackage{}, fmt.Errorf("parse %s: %w", path, parseErr)
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			parsed.testFiles[path] = file
			continue
		}
		if file.Name.Name == pkg.Name {
			parsed.sourceFiles[path] = file
		}
	}
	return parsed, nil
}

func checkPackageDocs(pkg packageInfo, files map[string]*ast.File, findings *[]finding) {
	packageDocs := packageDocFiles(files)
	if len(packageDocs) == 0 {
		*findings = append(*findings, newFinding(categoryPackageDocMissing, pkg, "", pkg.Name, "missing package documentation"))
		return
	}
	if len(packageDocs) > 1 {
		*findings = append(*findings, newFinding(categoryPackageDocMultiple, pkg, strings.Join(packageDocs, ", "), pkg.Name, "multiple package comments; keep one canonical doc.go comment"))
	}

	for _, path := range packageDocs {
		docText := strings.TrimSpace(files[path].Doc.Text())
		if validPackageDoc(pkg.Name, docText) {
			continue
		}
		want := "Package " + pkg.Name
		if pkg.Name == "main" {
			want = "Command "
		}
		*findings = append(*findings, newFinding(categoryPackageDocForm, pkg, path, pkg.Name, fmt.Sprintf("package comment must start with %q; got %q", want, firstLine(docText))))
	}
}

func packageDocFiles(files map[string]*ast.File) []string {
	paths := make([]string, 0, len(files))
	for path, file := range files {
		if file.Doc != nil && strings.TrimSpace(file.Doc.Text()) != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func validPackageDoc(packageName, docText string) bool {
	docText = strings.TrimSpace(docText)
	if packageName == "main" {
		return strings.HasPrefix(docText, "Command ")
	}
	return strings.HasPrefix(docText, "Package "+packageName)
}

func checkExportedDocs(pkg packageInfo, parsed parsedPackage, findings *[]finding) error {
	files := make([]*ast.File, 0, len(parsed.sourceFiles))
	for _, file := range parsed.sourceFiles {
		files = append(files, file)
	}
	docPackage, err := doc.NewFromFiles(parsed.fset, files, pkg.ImportPath)
	if err != nil {
		return fmt.Errorf("build doc package %s: %w", pkg.ImportPath, err)
	}
	for _, fn := range docPackage.Funcs {
		checkNamedDoc(pkg, categoryFuncMissing, categoryFuncForm, "func", fn.Name, fn.Doc, findings)
	}
	for _, typ := range docPackage.Types {
		checkNamedDoc(pkg, categoryTypeMissing, categoryTypeForm, "type", typ.Name, typ.Doc, findings)
		for _, fn := range typ.Funcs {
			checkNamedDoc(pkg, categoryFuncMissing, categoryFuncForm, "func", fn.Name, fn.Doc, findings)
		}
		for _, method := range typ.Methods {
			checkNamedDoc(pkg, categoryMethodMissing, categoryMethodForm, "method", method.Name, method.Doc, findings)
		}
		for _, value := range typ.Consts {
			checkValueDoc(pkg, categoryConstMissing, categoryConstForm, "const", value.Names, value.Doc, findings)
		}
		for _, value := range typ.Vars {
			checkValueDoc(pkg, categoryVarMissing, categoryVarForm, "var", value.Names, value.Doc, findings)
		}
	}
	for _, value := range docPackage.Consts {
		checkValueDoc(pkg, categoryConstMissing, categoryConstForm, "const", value.Names, value.Doc, findings)
	}
	for _, value := range docPackage.Vars {
		checkValueDoc(pkg, categoryVarMissing, categoryVarForm, "var", value.Names, value.Doc, findings)
	}
	return nil
}

func checkNamedDoc(pkg packageInfo, missingCategory, formCategory, kind, name, docText string, findings *[]finding) {
	docText = strings.TrimSpace(docText)
	if docText == "" {
		*findings = append(*findings, newFinding(missingCategory, pkg, "", name, fmt.Sprintf("missing %s documentation", kind)))
		return
	}
	if !strings.HasPrefix(docText, name) {
		*findings = append(*findings, newFinding(formCategory, pkg, "", name, fmt.Sprintf("%s comment must start with %q; got %q", kind, name, firstLine(docText))))
	}
}

func checkValueDoc(pkg packageInfo, missingCategory, formCategory, kind string, names []string, docText string, findings *[]finding) {
	exportedNames := exportedNames(names)
	if len(exportedNames) == 0 {
		return
	}
	docText = strings.TrimSpace(docText)
	nameList := strings.Join(exportedNames, ",")
	if len(exportedNames) > 1 {
		return
	}
	if docText == "" {
		*findings = append(*findings, newFinding(missingCategory, pkg, "", nameList, fmt.Sprintf("missing %s documentation", kind)))
		return
	}
	for _, name := range exportedNames {
		if strings.HasPrefix(docText, name) {
			return
		}
	}
	*findings = append(*findings, newFinding(formCategory, pkg, "", nameList, fmt.Sprintf("%s comment must start with one exported name in the group; got %q", kind, firstLine(docText))))
}

func exportedNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if ast.IsExported(name) {
			out = append(out, name)
		}
	}
	return out
}

func checkTestDocs(pkg packageInfo, files map[string]*ast.File, findings *[]finding) {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		for _, decl := range files[path].Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			checkTestFunctionDoc(pkg, path, fn, findings)
		}
	}
}

func checkTestFunctionDoc(pkg packageInfo, path string, fn *ast.FuncDecl, findings *[]finding) {
	name := fn.Name.Name
	missingCategory, formCategory, ok := testDocCategories(name)
	if !ok {
		return
	}
	docText := ""
	if fn.Doc != nil {
		docText = strings.TrimSpace(fn.Doc.Text())
	}
	if docText == "" {
		*findings = append(*findings, newFinding(missingCategory, pkg, path, name, "missing test documentation"))
		return
	}
	if !strings.HasPrefix(docText, name) {
		*findings = append(*findings, newFinding(formCategory, pkg, path, name, fmt.Sprintf("test comment must start with %q; got %q", name, firstLine(docText))))
	}
	if strings.HasPrefix(name, "Example") && !hasExampleOutput(docText) {
		*findings = append(*findings, newFinding(categoryExampleOutput, pkg, path, name, "example is missing Output or Unordered output comment"))
	}
}

func testDocCategories(name string) (missingCategory, formCategory string, ok bool) {
	switch {
	case name == "TestMain" || strings.HasPrefix(name, "Test"):
		return categoryTestMissing, categoryTestForm, true
	case strings.HasPrefix(name, "Benchmark"):
		return categoryBenchmarkMissing, categoryBenchmarkForm, true
	case strings.HasPrefix(name, "Fuzz"):
		return categoryFuzzMissing, categoryFuzzForm, true
	case strings.HasPrefix(name, "Example"):
		return categoryExampleMissing, categoryExampleForm, true
	default:
		return "", "", false
	}
}

func hasExampleOutput(docText string) bool {
	for line := range strings.SplitSeq(docText, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Output:") || strings.HasPrefix(line, "Unordered output:") {
			return true
		}
	}
	return false
}

func newFinding(category string, pkg packageInfo, path, name, detail string) finding {
	return finding{
		Category:   category,
		ImportPath: pkg.ImportPath,
		Package:    pkg.Name,
		File:       relativePath(path),
		Name:       name,
		Detail:     detail,
	}
}

func relativePath(path string) string {
	if path == "" {
		return ""
	}
	cleanPath := filepath.Clean(path)
	if rel, err := filepath.Rel(".", cleanPath); err == nil && !strings.HasPrefix(rel, "..") {
		cleanPath = rel
	}
	return filepath.ToSlash(cleanPath)
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	line, _, _ := strings.Cut(text, "\n")
	return line
}

func sortFindings(findings []finding) {
	sort.Slice(findings, func(i, j int) bool {
		left, right := findings[i], findings[j]
		if left.ImportPath != right.ImportPath {
			return left.ImportPath < right.ImportPath
		}
		if left.File != right.File {
			return left.File < right.File
		}
		if left.Category != right.Category {
			return left.Category < right.Category
		}
		return left.Name < right.Name
	})
}

func countBy(findings []finding, key func(finding) string) map[string]int {
	counts := map[string]int{}
	for _, finding := range findings {
		counts[key(finding)]++
	}
	return counts
}

func renderReport(report report, format string) ([]byte, error) {
	switch format {
	case formatMarkdown:
		return []byte(renderMarkdown(report)), nil
	case formatJSON:
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal json: %w", err)
		}
		return append(out, '\n'), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func renderMarkdown(report report) string {
	var b strings.Builder
	b.WriteString("# Godoc Audit Report\n\n")
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Packages audited | %d |\n", report.Packages)
	fmt.Fprintf(&b, "| Findings | %d |\n", len(report.Findings))
	fmt.Fprintf(&b, "| Include test functions | %t |\n", report.IncludeTests)
	b.WriteString("\n")

	writeCountTable(&b, "## Findings By Category", report.ByCategory, 0)
	writeCountTable(&b, "## Top Packages", report.ByPackage, 25)

	b.WriteString("## Findings\n\n")
	if len(report.Findings) == 0 {
		b.WriteString("No findings.\n")
		return b.String()
	}
	b.WriteString("| Category | Package | File | Name | Detail |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, finding := range report.Findings {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			md(finding.Category), md(finding.ImportPath), md(finding.File), md(finding.Name), md(finding.Detail))
	}
	return b.String()
}

func writeCountTable(b *strings.Builder, title string, counts map[string]int, limit int) {
	b.WriteString(title + "\n\n")
	if len(counts) == 0 {
		b.WriteString("No entries.\n\n")
		return
	}
	type row struct {
		key   string
		count int
	}
	rows := make([]row, 0, len(counts))
	for key, count := range counts {
		rows = append(rows, row{key: key, count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count == rows[j].count {
			return rows[i].key < rows[j].key
		}
		return rows[i].count > rows[j].count
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	b.WriteString("| Name | Count |\n")
	b.WriteString("| --- | ---: |\n")
	for _, row := range rows {
		fmt.Fprintf(b, "| %s | %d |\n", md(row.key), row.count)
	}
	b.WriteString("\n")
}

func md(value string) string {
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}
