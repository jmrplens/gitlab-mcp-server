// Audit finds missing or malformed Go doc comments (formerly audit_godocs).

package main

import (
	"cmp"
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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
)

const (
	categoryPackageDocMissing  = "pkgdoc_missing"
	categoryPackageDocForm     = "pkgdoc_form"
	categoryPackageDocMultiple = "pkgdoc_multiple"
	categoryPackageDocLocation = "pkgdoc_location"
	categoryFuncMissing        = "func_missing"
	categoryFuncForm           = "func_form"
	categoryMethodMissing      = "method_missing"
	categoryMethodForm         = "method_form"
	categoryTypeMissing        = "type_missing"
	categoryTypeForm           = "type_form"
	categoryConstMissing       = "const_missing"
	categoryConstForm          = "const_form"
	categoryConstGroupMissing  = "const_group_missing"
	categoryVarMissing         = "var_missing"
	categoryVarForm            = "var_form"
	categoryVarGroupMissing    = "var_group_missing"
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

// Seams over the toolchain and standard library, so a test can drive the
// failure branches a toolchain and filesystem the tests own, and run as root,
// never produce on their own: a `go list` whose rows are blank or missing a
// tab, a doc build that rejects its options, and a JSON marshal of a report
// that cannot be encoded.
var (
	goListOutput    = defaultGoListOutput
	newDocFromFiles = doc.NewFromFiles
	marshalIndent   = json.MarshalIndent
)

// defaultGoListOutput runs `go list` and returns its raw output. It is the
// production value of the goListOutput seam.
func defaultGoListOutput(ctx context.Context) ([]byte, error) {
	// #nosec G204 -- golist.Executable returns the fixed Go tool path from the active runtime installation.
	cmd := exec.CommandContext(ctx, golist.Executable(), "list", "-f", golist.Format, "./...")
	return cmd.Output()
}

// options controls how the documentation audit scans and reports packages.
//
// format is the requested output format ("markdown" or "json"). outputPath,
// when non-empty, redirects the report to a file; otherwise the report is
// written to stdout. includeTests enables Test/Benchmark/Fuzz/Example
// validation; failOnFindings turns findings into a non-zero exit code;
// ignoreInternal skips packages whose import path contains "/internal/".
type options struct {
	format         string
	outputPath     string
	includeTests   bool
	failOnFindings bool
	ignoreInternal bool
}

// finding describes one documentation issue found in a package.
//
// Category is one of the constants declared at the top of this file (e.g.
// "func_missing"). ImportPath and Package identify the affected package;
// File and Name pinpoint the symbol. Detail carries the human-readable
// explanation rendered in the report.
type finding struct {
	Category   string `json:"category"`
	ImportPath string `json:"import_path"`
	Package    string `json:"package"`
	File       string `json:"file,omitempty"`
	Name       string `json:"name"`
	Detail     string `json:"detail"`
}

// report is the complete machine-readable audit result.
//
// GeneratedAt is the RFC3339 timestamp recorded when the audit finished.
// Packages is the count of audited packages; Findings is the unsorted list
// of every finding collected during the walk. ByCategory and ByPackage
// expose pre-computed counts so consumers can render summary tables
// without re-aggregating Findings.
type report struct {
	GeneratedAt  string         `json:"generated_at"`
	IncludeTests bool           `json:"include_tests"`
	Packages     int            `json:"packages"`
	Findings     []finding      `json:"findings"`
	ByCategory   map[string]int `json:"by_category"`
	ByPackage    map[string]int `json:"by_package"`
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
//
// The 30-second bound is this command's own: the audit lists the whole module
// once and then reads it, so a toolchain that never answers should end the run
// rather than hang it.
func listPackages() ([]golist.PackageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := goListOutput(ctx)
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	return golist.ParseRows(out)
}

// auditPackage parses source files for one package and checks documentation.
func auditPackage(pkg golist.PackageInfo, includeTests bool) ([]finding, error) {
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

func parsePackageFiles(pkg golist.PackageInfo) (parsedPackage, error) {
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

func checkPackageDocs(pkg golist.PackageInfo, files map[string]*ast.File, findings *[]finding) {
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
			// A package comment has one home. Anywhere else it is one file
			// header away from being mistaken for one, and a second file's
			// header away from being duplicated; `fix -move-package-doc`
			// moves it.
			if filepath.Base(path) != packageDocFile {
				*findings = append(*findings, newFinding(categoryPackageDocLocation, pkg, path, pkg.Name, fmt.Sprintf("package comment lives in %s; keep it in %s", filepath.Base(path), packageDocFile)))
			}
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

func checkExportedDocs(pkg golist.PackageInfo, parsed parsedPackage, findings *[]finding) error {
	files := make([]*ast.File, 0, len(parsed.sourceFiles))
	for _, file := range parsed.sourceFiles {
		files = append(files, file)
	}
	docPackage, err := newDocFromFiles(parsed.fset, files, pkg.ImportPath)
	if err != nil {
		return fmt.Errorf("build doc package %s: %w", pkg.ImportPath, err)
	}
	funcRule := symbolRule{missing: categoryFuncMissing, form: categoryFuncForm, kind: "func"}
	typeRule := symbolRule{missing: categoryTypeMissing, form: categoryTypeForm, kind: "type"}
	methodRule := symbolRule{missing: categoryMethodMissing, form: categoryMethodForm, kind: "method"}
	constRule := valueRule{missing: categoryConstMissing, form: categoryConstForm, kind: "const", group: categoryConstGroupMissing}
	varRule := valueRule{missing: categoryVarMissing, form: categoryVarForm, kind: "var", group: categoryVarGroupMissing}
	for _, fn := range docPackage.Funcs {
		checkNamedDoc(pkg, funcRule, fn.Name, fn.Name, fn.Doc, findings)
	}
	for _, typ := range docPackage.Types {
		checkNamedDoc(pkg, typeRule, typ.Name, typ.Name, typ.Doc, findings)
		for _, fn := range typ.Funcs {
			checkNamedDoc(pkg, funcRule, fn.Name, fn.Name, fn.Doc, findings)
		}
		// A method is named with its receiver type: a symbol finding carries
		// no file, so "Run" alone could be any of the package's Run methods.
		for _, method := range typ.Methods {
			checkNamedDoc(pkg, methodRule, typ.Name+"."+method.Name, method.Name, method.Doc, findings)
		}
		for _, value := range typ.Consts {
			checkValueDoc(pkg, constRule, value, findings)
		}
		for _, value := range typ.Vars {
			checkValueDoc(pkg, varRule, value, findings)
		}
	}
	for _, value := range docPackage.Consts {
		checkValueDoc(pkg, constRule, value, findings)
	}
	for _, value := range docPackage.Vars {
		checkValueDoc(pkg, varRule, value, findings)
	}
	return nil
}

// symbolRule is how one kind of symbol is reported: the category of a missing
// comment, the category of a malformed one, and the word naming the kind.
type symbolRule struct {
	missing, form, kind string
}

// checkNamedDoc reports a missing or malformed comment on one symbol. label
// names the symbol in the finding; name is the identifier the comment has to
// open with, which for a method is label without its receiver type.
func checkNamedDoc(pkg golist.PackageInfo, rule symbolRule, label, name, docText string, findings *[]finding) {
	docText = strings.TrimSpace(docText)
	if docText == "" {
		*findings = append(*findings, newFinding(rule.missing, pkg, "", label, fmt.Sprintf("missing %s documentation", rule.kind)))
		return
	}
	if !strings.HasPrefix(docText, name) {
		*findings = append(*findings, newFinding(rule.form, pkg, "", label, fmt.Sprintf("%s comment must start with %q; got %q", rule.kind, name, firstLine(docText))))
	}
}

// valueRule is how one kind of const or var declaration is reported: the
// categories and word of [symbolRule], and the category of a declaration of
// several exported names that leaves some of them with no comment at all.
type valueRule struct {
	symbolRule
	group string
}

// checkValueDoc reports a missing or malformed comment on one const or var
// declaration, which go/doc hands over whole: a parenthesized group, or a
// single spec that may declare several names.
//
// Where a declaration's comment is looked for follows revive's exported rule,
// which counts a trailing line comment no more than this audit does, with one
// difference: revive never judges the form of a parenthesized group's own
// comment, while a group of one exported name is held to it here, since go doc
// shows that comment as the name's documentation.
//
// A declaration of one exported name needs a comment opening with that name:
// the group's own, or, in a group that has none, the doc comment of the spec
// declaring the name, which go/doc does not hand over and which used to leave
// such a name reported as undocumented however it was commented. A declaration
// of several needs a comment on the group, which describes the set and so is
// not judged by its form, or else a doc comment of its own on every exported
// name that opens with that name.
func checkValueDoc(pkg golist.PackageInfo, rule valueRule, value *doc.Value, findings *[]finding) {
	exportedNames := exportedNames(value.Names)
	if len(exportedNames) == 0 {
		return
	}
	docText := strings.TrimSpace(value.Doc)
	nameList := strings.Join(exportedNames, ",")
	specDoc := specDocs(value.Decl)
	if len(exportedNames) > 1 {
		if docText != "" {
			return
		}
		if undocumented := withoutDoc(exportedNames, specDoc); len(undocumented) > 0 {
			*findings = append(*findings, newFinding(rule.group, pkg, "", nameList, fmt.Sprintf("%s group has no group comment, and these exported names have none of their own: %s", rule.kind, strings.Join(undocumented, ", "))))
		}
		for _, name := range exportedNames {
			if text := specDoc[name]; text != "" && !strings.HasPrefix(text, name) {
				*findings = append(*findings, newFinding(rule.form, pkg, "", name, fmt.Sprintf("%s comment must start with %q; got %q", rule.kind, name, firstLine(text))))
			}
		}
		return
	}
	if docText == "" {
		docText = specDoc[exportedNames[0]]
	}
	if docText == "" {
		*findings = append(*findings, newFinding(rule.missing, pkg, "", nameList, fmt.Sprintf("missing %s documentation", rule.kind)))
		return
	}
	for _, name := range exportedNames {
		if strings.HasPrefix(docText, name) {
			return
		}
	}
	*findings = append(*findings, newFinding(rule.form, pkg, "", nameList, fmt.Sprintf("%s comment must start with one exported name in the group; got %q", rule.kind, firstLine(docText))))
}

// specDocs maps every name a const or var declaration declares to the doc
// comment of the spec declaring it, "" for none. A trailing line comment is not
// one: go doc prints it inside the declaration's code, not as documentation. A
// spec that declares no value names nothing, although go/doc only ever hands
// over value specs here.
func specDocs(decl *ast.GenDecl) map[string]string {
	docs := map[string]string{}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		text := strings.TrimSpace(valueSpec.Doc.Text())
		for _, name := range valueSpec.Names {
			docs[name.Name] = text
		}
	}
	return docs
}

// withoutDoc returns the names, in their order, for which docs holds no
// comment.
func withoutDoc(names []string, docs map[string]string) []string {
	var out []string
	for _, name := range names {
		if docs[name] == "" {
			out = append(out, name)
		}
	}
	return out
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

func checkTestDocs(pkg golist.PackageInfo, files map[string]*ast.File, findings *[]finding) {
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

func checkTestFunctionDoc(pkg golist.PackageInfo, path string, fn *ast.FuncDecl, findings *[]finding) {
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

// testFamilies are the functions of a test file that go test runs, by the
// prefix of their name, each with the categories a missing and a malformed doc
// comment of one is reported under. TestMain carries the Test prefix, so it is
// one of the first family.
var testFamilies = []struct{ prefix, missing, form string }{
	{prefix: "Test", missing: categoryTestMissing, form: categoryTestForm},
	{prefix: "Benchmark", missing: categoryBenchmarkMissing, form: categoryBenchmarkForm},
	{prefix: "Fuzz", missing: categoryFuzzMissing, form: categoryFuzzForm},
	{prefix: "Example", missing: categoryExampleMissing, form: categoryExampleForm},
}

func testDocCategories(name string) (missingCategory, formCategory string, ok bool) {
	for _, family := range testFamilies {
		if strings.HasPrefix(name, family.prefix) {
			return family.missing, family.form, true
		}
	}
	return "", "", false
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

func newFinding(category string, pkg golist.PackageInfo, path, name, detail string) finding {
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

// sortFindings orders the findings by package, file, category and name, and
// keeps the order they were found in among findings that share all four.
func sortFindings(findings []finding) {
	slices.SortStableFunc(findings, func(left, right finding) int {
		return cmp.Or(
			strings.Compare(left.ImportPath, right.ImportPath),
			strings.Compare(left.File, right.File),
			strings.Compare(left.Category, right.Category),
			strings.Compare(left.Name, right.Name),
		)
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
		out, err := marshalIndent(report, "", "  ")
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
	slices.SortFunc(rows, func(left, right row) int {
		return cmp.Or(cmp.Compare(right.count, left.count), strings.Compare(left.key, right.key))
	})
	if limit > 0 {
		rows = rows[:min(len(rows), limit)]
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
