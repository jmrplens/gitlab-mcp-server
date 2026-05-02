// Command gen_testing_docs regenerates the managed test metrics section in
// docs/development/testing.md.
//
// It discovers Go packages, counts Test* functions by parsing _test.go files,
// runs unit-test coverage for ./internal/... and ./cmd/..., and replaces the
// generated Markdown block in the testing reference document.
//
// Usage:
//
//	go run ./cmd/gen_testing_docs/
//	go run ./cmd/gen_testing_docs/ --check
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	defaultDocPath = "docs/development/testing.md"
	startMarker    = "<!-- START TESTING STATS -->"
	endMarker      = "<!-- END TESTING STATS -->"
	fallbackStart  = "## Overview"
	fallbackEnd    = "## Test Types"

	pattern3Part        = "3-part"
	pattern2Part        = "2-part"
	patternNoUnderscore = "no-underscore"
	patternTestCov      = "TestCov"
	patternOther        = "other"
)

var (
	coverageLineRE = regexp.MustCompile(`^ok\s+(\S+)\s+.*coverage:\s+([0-9.]+)% of statements`)
	totalLineRE    = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)
	covPattern     = regexp.MustCompile(`^TestCov[A-Z]`)
)

// options controls how the generator collects and writes documentation data.
type options struct {
	docPath       string
	check         bool
	skipCoverage  bool
	topToolRows   int
	timeout       time.Duration
	coverageDir   string
	includeE2ERun bool
}

// packageMetrics contains the generated testing facts for one Go package.
type packageMetrics struct {
	ImportPath    string
	Dir           string
	RelPath       string
	Name          string
	Key           string
	Layer         string
	Summary       string
	TestFunctions int
	TestFiles     int
	ToolCount     int
	Coverage      coverageValue
}

// coverageValue stores a per-package or aggregate coverage percentage.
type coverageValue struct {
	Percent float64
	OK      bool
}

// repositoryMetrics is the full generated data model for testing.md.
type repositoryMetrics struct {
	Packages               []packageMetrics
	NamingCounts           map[string]int
	OverallCoverage        coverageValue
	InternalCoverage       coverageValue
	AveragePackageCoverage coverageValue
	E2ENote                string
}

// packageInfo identifies one Go package returned by go list.
type packageInfo struct {
	ImportPath string
	Dir        string
	Name       string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses flags, collects metrics, renders Markdown, and updates testing.md.
func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	metrics, err := collectMetrics(ctx, opts)
	if err != nil {
		return err
	}

	content := renderTestingStats(metrics, opts.topToolRows)
	changed, err := updateManagedSection(opts.docPath, content, opts.check)
	if err != nil {
		return err
	}

	if opts.check {
		if changed {
			return fmt.Errorf("%s is out of date; run go run ./cmd/gen_testing_docs/", opts.docPath)
		}
		_, _ = fmt.Fprintf(stdout, "%s is up to date\n", opts.docPath)
		return nil
	}

	if changed {
		_, _ = fmt.Fprintf(stdout, "Updated %s\n", opts.docPath)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s already up to date\n", opts.docPath)
	}
	return nil
}

// parseOptions validates command-line flags.
func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("gen_testing_docs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := options{}
	fs.StringVar(&opts.docPath, "file", defaultDocPath, "testing documentation file to update")
	fs.BoolVar(&opts.check, "check", false, "fail if the generated section is not current")
	fs.BoolVar(&opts.skipCoverage, "skip-coverage", false, "skip go test coverage execution and update count-only sections")
	fs.IntVar(&opts.topToolRows, "top-tool-rows", 25, "number of high-test-count tool sub-packages to show in the summary table")
	fs.DurationVar(&opts.timeout, "timeout", 15*time.Minute, "timeout for Go test coverage commands")
	fs.StringVar(&opts.coverageDir, "coverage-dir", "", "directory for temporary coverage profiles; defaults to a temp directory")
	fs.BoolVar(&opts.includeE2ERun, "include-e2e-run", false, "also run the build-tagged E2E suite; requires GitLab test environment")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.topToolRows < 1 {
		return options{}, errors.New("top-tool-rows must be greater than zero")
	}
	return opts, nil
}

// collectMetrics discovers packages, counts tests, and runs coverage commands.
func collectMetrics(ctx context.Context, opts options) (repositoryMetrics, error) {
	infos, err := listPackages(ctx)
	if err != nil {
		return repositoryMetrics{}, err
	}

	metrics := repositoryMetrics{
		NamingCounts: map[string]int{},
		E2ENote:      "E2E tests are counted statically from test/e2e/suite because they require a real GitLab fixture to execute.",
	}

	coverageByPackage := map[string]coverageValue{}
	if !opts.skipCoverage {
		combinedCoverage, combinedTotal, coverageErr := runCoverage(ctx, opts, "cmd-internal", []string{"./internal/...", "./cmd/..."})
		if coverageErr != nil {
			return repositoryMetrics{}, coverageErr
		}
		coverageByPackage = combinedCoverage
		metrics.OverallCoverage = combinedTotal

		_, internalTotal, internalErr := runCoverage(ctx, opts, "internal", []string{"./internal/..."})
		if internalErr != nil {
			return repositoryMetrics{}, internalErr
		}
		metrics.InternalCoverage = internalTotal
	}

	for _, info := range infos {
		pkg := packageMetrics{
			ImportPath: info.ImportPath,
			Dir:        info.Dir,
			Name:       info.Name,
			RelPath:    relativePath(info.Dir),
			Layer:      classifyLayer(relativePath(info.Dir)),
			Summary:    packageSummary(info.Dir),
		}
		pkg.Key = packageKey(pkg.RelPath)

		testFunctions, testFiles, namingCounts, countErr := countTests(info.Dir)
		if countErr != nil {
			return repositoryMetrics{}, fmt.Errorf("count tests in %s: %w", pkg.RelPath, countErr)
		}
		pkg.TestFunctions = testFunctions
		pkg.TestFiles = testFiles
		for pattern, count := range namingCounts {
			metrics.NamingCounts[pattern] += count
		}

		if pkg.Layer == "tool-subpackage" {
			toolCount, toolErr := countMCPTools(info.Dir)
			if toolErr != nil {
				return repositoryMetrics{}, fmt.Errorf("count MCP tools in %s: %w", pkg.RelPath, toolErr)
			}
			pkg.ToolCount = toolCount
		}

		if coverage, ok := coverageByPackage[pkg.ImportPath]; ok {
			pkg.Coverage = coverage
		}
		metrics.Packages = append(metrics.Packages, pkg)
	}

	metrics.AveragePackageCoverage = averageCoverage(metrics.Packages)

	if opts.includeE2ERun {
		if _, e2eErr := runGo(ctx, []string{"test", "-tags", "e2e", "-timeout", opts.timeout.String(), "./test/e2e/suite/"}); e2eErr != nil {
			return repositoryMetrics{}, fmt.Errorf("run e2e tests: %w", e2eErr)
		}
		metrics.E2ENote = "E2E tests were executed with -tags e2e during this generation run. Coverage tables still report unit-test coverage for ./internal/... and ./cmd/...."
	}

	return metrics, nil
}

// listPackages returns all packages covered by the testing reference document.
func listPackages(ctx context.Context) ([]packageInfo, error) {
	output, err := runGo(ctx, []string{"list", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{.Name}}", "./cmd/...", "./internal/...", "./test/e2e/suite"})
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}

	infos := []packageInfo{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected go list row: %q", line)
		}
		infos = append(infos, packageInfo{ImportPath: parts[0], Dir: parts[1], Name: parts[2]})
	}
	return infos, nil
}

// runCoverage executes go test with coverage and returns per-package and total percentages.
func runCoverage(ctx context.Context, opts options, name string, patterns []string) (map[string]coverageValue, coverageValue, error) {
	coverageDir, cleanup, err := coverageDirectory(opts.coverageDir)
	if err != nil {
		return nil, coverageValue{}, err
	}
	defer cleanup()

	profilePath := filepath.Join(coverageDir, name+".out")
	args := []string{"test", "-coverprofile=" + profilePath, "-covermode=count"}
	args = append(args, patterns...)
	args = append(args, "-count=1")

	output, err := runGo(ctx, args)
	if err != nil {
		return nil, coverageValue{}, fmt.Errorf("run coverage for %s: %w", strings.Join(patterns, " "), err)
	}

	coverages, err := parsePackageCoverages(string(output))
	if err != nil {
		return nil, coverageValue{}, err
	}

	coverOutput, err := runGo(ctx, []string{"tool", "cover", "-func=" + profilePath})
	if err != nil {
		return nil, coverageValue{}, fmt.Errorf("summarize coverage for %s: %w", strings.Join(patterns, " "), err)
	}
	total, err := parseTotalCoverage(string(coverOutput))
	if err != nil {
		return nil, coverageValue{}, err
	}

	return coverages, coverageValue{Percent: total, OK: true}, nil
}

// coverageDirectory returns the directory for profiles and a cleanup callback.
func coverageDirectory(configured string) (dir string, cleanup func(), err error) {
	if configured != "" {
		if mkdirErr := os.MkdirAll(configured, 0o750); mkdirErr != nil {
			return "", func() {}, fmt.Errorf("create coverage dir: %w", mkdirErr)
		}
		return configured, func() {}, nil
	}
	dir, err = os.MkdirTemp("", "gen-testing-docs-coverage-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp coverage dir: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// countTests parses _test.go files in one package directory.
func countTests(dir string) (testFunctions, testFiles int, namingCounts map[string]int, err error) {
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		return 0, 0, nil, readErr
	}

	namingCounts = map[string]int{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		testFiles++
		path := filepath.Join(dir, entry.Name())
		node, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return 0, 0, nil, parseErr
		}
		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isTestFunction(fn.Name.Name) {
				continue
			}
			testFunctions++
			pattern := classifyTestName(fn.Name.Name)
			namingCounts[pattern]++
		}
	}
	return testFunctions, testFiles, namingCounts, nil
}

// isTestFunction reports whether name follows Go's Test* entry-point rules.
func isTestFunction(name string) bool {
	if name == "TestMain" || !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) == len("Test") {
		return true
	}
	return !unicode.IsLower(rune(name[len("Test")]))
}

// classifyTestName returns the documentation naming-pattern bucket for a test name.
func classifyTestName(name string) string {
	if covPattern.MatchString(name) {
		return patternTestCov
	}
	parts := strings.Split(name, "_")
	switch {
	case len(parts) >= 3:
		return pattern3Part
	case len(parts) == 2:
		return pattern2Part
	default:
		return patternNoUnderscore
	}
}

// countMCPTools counts individual MCP tool registrations in a package directory.
func countMCPTools(dir string) (int, error) {
	count := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		node, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "AddTool" {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if ok && ident.Name == "mcp" {
				count++
			}
			return true
		})
		return nil
	})
	return count, err
}

// parsePackageCoverages extracts coverage percentages from go test output.
func parsePackageCoverages(output string) (map[string]coverageValue, error) {
	coverages := map[string]coverageValue{}
	for line := range strings.SplitSeq(output, "\n") {
		matches := coverageLineRE.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) != 3 {
			continue
		}
		percent, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse coverage line %q: %w", line, err)
		}
		coverages[matches[1]] = coverageValue{Percent: percent, OK: true}
	}
	return coverages, nil
}

// parseTotalCoverage extracts the total coverage from go tool cover -func output.
func parseTotalCoverage(output string) (float64, error) {
	for line := range strings.SplitSeq(output, "\n") {
		matches := totalLineRE.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) != 2 {
			continue
		}
		percent, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return 0, fmt.Errorf("parse total coverage line %q: %w", line, err)
		}
		return percent, nil
	}
	return 0, errors.New("total coverage line not found")
}

// renderTestingStats builds the generated Markdown section.
func renderTestingStats(metrics repositoryMetrics, topToolRows int) string {
	var b strings.Builder
	b.WriteString("## Overview\n\n")
	fmt.Fprintf(&b, "> This section is generated by `go run ./cmd/gen_testing_docs/`. It runs unit coverage for `./internal/...` and `./cmd/...`; %s\n\n", metrics.E2ENote)
	b.WriteString(renderOverview(metrics))
	b.WriteString(renderNamingStats(metrics))
	b.WriteString(renderDistribution(metrics))
	b.WriteString(renderCorePackages(metrics))
	b.WriteString(renderTopToolPackages(metrics, topToolRows))
	b.WriteString(renderCompleteToolPackages(metrics))
	b.WriteString(renderCoverageReport(metrics))
	b.WriteString(renderCoverageExceptions(metrics))
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// renderOverview renders the top-level testing metrics table.
func renderOverview(metrics repositoryMetrics) string {
	var b strings.Builder
	totals := layerTotals(metrics.Packages)
	toolPackages := packagesByLayer(metrics.Packages, "tool-subpackage")
	corePackages := packagesByLayer(metrics.Packages, "core")

	b.WriteString("| Metric | Value |\n")
	b.WriteString("| --- | ---: |\n")
	fmt.Fprintf(&b, "| Total test functions | %s |\n", fmtInt(totalTests(metrics.Packages)))
	fmt.Fprintf(&b, "| Unit test functions | %s |\n", fmtInt(totalTests(metrics.Packages)-totals["e2e"].tests))
	fmt.Fprintf(&b, "| E2E test functions | %s |\n", fmtInt(totals["e2e"].tests))
	fmt.Fprintf(&b, "| cmd test functions | %s |\n", fmtInt(totals["cmd"].tests))
	fmt.Fprintf(&b, "| Test files (internal/) | %s |\n", fmtInt(testFilesWithPrefix(metrics.Packages, "internal/")))
	fmt.Fprintf(&b, "| Test files (cmd/) | %s |\n", fmtInt(testFilesWithPrefix(metrics.Packages, "cmd/")))
	fmt.Fprintf(&b, "| Test files (test/e2e/suite/) | %s |\n", fmtInt(testFilesWithPrefix(metrics.Packages, "test/e2e/suite")))
	fmt.Fprintf(&b, "| Tool sub-packages tested | %s |\n", fmtInt(countTestedPackages(toolPackages)))
	fmt.Fprintf(&b, "| Core packages tested | %s |\n", fmtInt(countTestedPackages(corePackages)))
	fmt.Fprintf(&b, "| Overall coverage (`go test ./internal/... ./cmd/...`) | %s |\n", fmtCoverage(metrics.OverallCoverage))
	fmt.Fprintf(&b, "| Overall coverage (`go test ./internal/...`) | %s |\n", fmtCoverage(metrics.InternalCoverage))
	fmt.Fprintf(&b, "| Average package coverage | %s |\n\n", fmtCoverage(metrics.AveragePackageCoverage))
	return b.String()
}

// renderNamingStats renders Test* naming convention counts.
func renderNamingStats(metrics repositoryMetrics) string {
	total := sumCounts(metrics.NamingCounts)
	var b strings.Builder
	b.WriteString("### Naming Convention Stats\n\n")
	b.WriteString("| Pattern | Count | % |\n")
	b.WriteString("| --- | ---: | ---: |\n")
	for _, row := range []struct {
		Pattern string
		Label   string
	}{
		{pattern2Part, "`TestFunc_Scenario` (2-part)"},
		{patternNoUnderscore, "`TestFunc` (no underscore)"},
		{pattern3Part, "`TestFunc_Scenario_Expected` (3+ part)"},
		{patternTestCov, "`TestCovFuncScenario` (coverage helper)"},
		{patternOther, "Other"},
	} {
		count := metrics.NamingCounts[row.Pattern]
		if count == 0 {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", row.Label, fmtInt(count), fmtRatio(count, total))
	}
	b.WriteString("\n")
	return b.String()
}

// renderDistribution renders layer-level package/test counts.
func renderDistribution(metrics repositoryMetrics) string {
	totals := layerTotals(metrics.Packages)
	var b strings.Builder
	b.WriteString("## Test Distribution\n\n")
	b.WriteString("### By Layer\n\n")
	b.WriteString("| Layer | Test Functions | Test Files | Description |\n")
	b.WriteString("| --- | ---: | ---: | --- |\n")
	rows := []struct {
		Layer       string
		Label       string
		Description string
	}{
		{"core", "Core packages", "shared runtime packages such as config, GitLab client, OAuth, resources, prompts, and utilities"},
		{"tools-orchestration", "Tools orchestration", "registration, meta-tool dispatch, safe mode, validation, markdown, and routing tests"},
		{"tool-subpackage", fmt.Sprintf("Tool sub-packages (%d)", countTestedPackages(packagesByLayer(metrics.Packages, "tool-subpackage"))), "domain-specific GitLab tool handlers"},
		{"e2e", "E2E integration", "build-tagged real GitLab integration suite"},
		{"cmd", "cmd packages", "server entry point and developer command utilities"},
	}
	for _, row := range rows {
		total := totals[row.Layer]
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", row.Label, fmtInt(total.tests), fmtInt(total.files), escapeTable(row.Description))
	}
	fmt.Fprintf(&b, "| **Total** | **%s** | **%s** |  |\n\n", fmtInt(totalTests(metrics.Packages)), fmtInt(totalTestFiles(metrics.Packages)))
	return b.String()
}

// renderCorePackages renders counts and coverage for non-tool internal packages.
func renderCorePackages(metrics repositoryMetrics) string {
	packages := packagesByLayer(metrics.Packages, "core")
	sort.Slice(packages, func(i, j int) bool { return packages[i].Key < packages[j].Key })

	var b strings.Builder
	b.WriteString("### Core Packages\n\n")
	b.WriteString("| Package | Tests | Coverage | Description |\n")
	b.WriteString("| --- | ---: | ---: | --- |\n")
	subtotal := 0
	for _, pkg := range packages {
		if pkg.TestFunctions == 0 {
			continue
		}
		subtotal += pkg.TestFunctions
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", pkg.Key, fmtInt(pkg.TestFunctions), fmtCoverage(pkg.Coverage), escapeTable(pkg.Summary))
	}
	fmt.Fprintf(&b, "| **Subtotal** | **%s** |  |  |\n\n", fmtInt(subtotal))
	return b.String()
}

// renderTopToolPackages renders the most-tested tool sub-packages.
func renderTopToolPackages(metrics repositoryMetrics, topToolRows int) string {
	packages := packagesByLayer(metrics.Packages, "tool-subpackage")
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].TestFunctions != packages[j].TestFunctions {
			return packages[i].TestFunctions > packages[j].TestFunctions
		}
		return packages[i].Key < packages[j].Key
	})
	if len(packages) > topToolRows {
		packages = packages[:topToolRows]
	}

	var b strings.Builder
	b.WriteString("### Tool Sub-Packages (Top Domains by Test Count)\n\n")
	b.WriteString("| Sub-package | Tests | Coverage | Tools |\n")
	b.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, pkg := range packages {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", pkg.Key, fmtInt(pkg.TestFunctions), fmtCoverage(pkg.Coverage), fmtInt(pkg.ToolCount))
	}
	b.WriteString("\n")
	return b.String()
}

// renderCompleteToolPackages renders the full tool sub-package table.
func renderCompleteToolPackages(metrics repositoryMetrics) string {
	packages := packagesByLayer(metrics.Packages, "tool-subpackage")
	sort.Slice(packages, func(i, j int) bool { return packages[i].Key < packages[j].Key })

	tested := countTestedPackages(packages)
	var b strings.Builder
	b.WriteString("### Complete Tool Sub-Package Test Counts\n\n")
	b.WriteString("<details>\n")
	fmt.Fprintf(&b, "<summary>All %d tested sub-packages (click to expand)</summary>\n\n", tested)
	b.WriteString("| Sub-package | Tests | Test Files | Coverage | Tools |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	totalTests := 0
	totalFiles := 0
	totalTools := 0
	for _, pkg := range packages {
		if pkg.TestFunctions == 0 {
			continue
		}
		totalTests += pkg.TestFunctions
		totalFiles += pkg.TestFiles
		totalTools += pkg.ToolCount
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", pkg.Key, fmtInt(pkg.TestFunctions), fmtInt(pkg.TestFiles), fmtCoverage(pkg.Coverage), fmtInt(pkg.ToolCount))
	}
	fmt.Fprintf(&b, "| **Total** | **%s** | **%s** |  | **%s** |\n\n", fmtInt(totalTests), fmtInt(totalFiles), fmtInt(totalTools))
	b.WriteString("</details>\n\n")
	return b.String()
}

// renderCoverageReport renders package coverage snapshots.
func renderCoverageReport(metrics repositoryMetrics) string {
	var b strings.Builder
	b.WriteString("## Coverage Report\n\n")
	b.WriteString("### cmd Package Snapshot\n\n")
	b.WriteString("| Package | Coverage |\n")
	b.WriteString("| --- | ---: |\n")
	for _, pkg := range sortedCoveragePackages(packagesByLayer(metrics.Packages, "cmd")) {
		fmt.Fprintf(&b, "| %s | %s |\n", pkg.Key, fmtCoverage(pkg.Coverage))
	}
	b.WriteString("\n")

	b.WriteString("### Core Packages\n\n")
	b.WriteString("| Package | Coverage |\n")
	b.WriteString("| --- | ---: |\n")
	for _, pkg := range sortedCoveragePackages(packagesByLayer(metrics.Packages, "core")) {
		fmt.Fprintf(&b, "| %s | %s |\n", pkg.Key, fmtCoverage(pkg.Coverage))
	}
	b.WriteString("\n")

	b.WriteString("### Tool Sub-Packages\n\n")
	b.WriteString("| Package | Coverage |\n")
	b.WriteString("| --- | ---: |\n")
	for _, pkg := range sortedCoveragePackages(packagesByLayer(metrics.Packages, "tools-orchestration")) {
		fmt.Fprintf(&b, "| %s | %s |\n", "tools (orch.)", fmtCoverage(pkg.Coverage))
	}
	for _, pkg := range sortedCoveragePackages(packagesByLayer(metrics.Packages, "tool-subpackage")) {
		fmt.Fprintf(&b, "| %s | %s |\n", pkg.Key, fmtCoverage(pkg.Coverage))
	}
	b.WriteString("\n")
	return b.String()
}

// renderCoverageExceptions renders packages that currently miss the target.
func renderCoverageExceptions(metrics repositoryMetrics) string {
	low := lowCoveragePackages(metrics.Packages, 90.0)
	var b strings.Builder
	b.WriteString("Coverage target: **>90%** per package. Packages below the target in the latest generated coverage snapshot:\n\n")
	if len(low) == 0 {
		b.WriteString("- None.\n")
		return b.String()
	}
	for _, pkg := range low {
		fmt.Fprintf(&b, "- **%s** (%s) - %s\n", pkg.Key, fmtCoverage(pkg.Coverage), coverageRationale(pkg))
	}
	return b.String()
}

// packageSummary returns the first sentence of a package comment.
func packageSummary(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "Package documentation unavailable."
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		node, parseErr := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.ParseComments)
		if parseErr != nil || node.Doc == nil {
			continue
		}
		docText := strings.TrimSpace(node.Doc.Text())
		if docText != "" {
			return firstSentence(docText)
		}
	}
	return "Package documentation unavailable."
}

// firstSentence returns a compact one-line summary from a Go doc comment.
func firstSentence(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if idx := strings.Index(s, ". "); idx >= 0 {
		s = s[:idx+1]
	}
	return s
}

// updateManagedSection replaces or creates the generated section in the target file.
func updateManagedSection(path, content string, check bool) (bool, error) {
	docPath, err := resolveRepositoryPath(path)
	if err != nil {
		return false, err
	}

	data, err := os.ReadFile(docPath) //#nosec G304 -- path constrained to repository root by resolveRepositoryPath.
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	text := string(data)

	updated, err := replaceGeneratedBlock(text, content)
	if err != nil {
		return false, err
	}
	if updated == text {
		return false, nil
	}
	if check {
		return true, nil
	}
	if writeErr := os.WriteFile(docPath, []byte(updated), 0o644); writeErr != nil { //#nosec G306,G304,G703 -- path constrained to repository root by resolveRepositoryPath.
		return false, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return true, nil
}

// resolveRepositoryPath returns an absolute path that stays inside the repository.
func resolveRepositoryPath(path string) (string, error) {
	root, err := filepath.Abs(repositoryRoot())
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(root, cleaned)
	}
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return "", fmt.Errorf("compare %s with repository root: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s is outside repository root", path)
	}
	return absPath, nil
}

// replaceGeneratedBlock replaces an existing marker block or migrates the legacy section.
func replaceGeneratedBlock(text, content string) (string, error) {
	if strings.Contains(text, startMarker) || strings.Contains(text, endMarker) {
		return replaceBetweenMarkers(text, content)
	}

	startIdx := strings.Index(text, fallbackStart)
	if startIdx < 0 {
		return "", fmt.Errorf("fallback start heading %q not found", fallbackStart)
	}
	endIdx := strings.Index(text[startIdx:], fallbackEnd)
	if endIdx < 0 {
		return "", fmt.Errorf("fallback end heading %q not found after %q", fallbackEnd, fallbackStart)
	}
	endIdx += startIdx

	before := strings.TrimRight(text[:startIdx], "\n")
	after := strings.TrimLeft(text[endIdx:], "\n")
	return before + "\n\n" + startMarker + "\n\n" + content + "\n" + endMarker + "\n\n" + after, nil
}

// replaceBetweenMarkers replaces content between startMarker and endMarker.
func replaceBetweenMarkers(text, content string) (string, error) {
	startIdx := strings.Index(text, startMarker)
	if startIdx < 0 {
		return "", fmt.Errorf("start marker %s not found", startMarker)
	}
	searchFrom := startIdx + len(startMarker)
	endRel := strings.Index(text[searchFrom:], endMarker)
	if endRel < 0 {
		return "", fmt.Errorf("end marker %s not found after start marker", endMarker)
	}
	endIdx := searchFrom + endRel
	before := text[:searchFrom]
	after := text[endIdx:]
	return before + "\n\n" + content + "\n" + after, nil
}

// runGo executes a Go command with the module toolchain version pinned.
func runGo(ctx context.Context, args []string) ([]byte, error) {
	// #nosec G204 -- args are built by this generator from fixed Go subcommands; no shell is involved.
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Env = goEnvironment()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, tailLines(string(output), 80))
	}
	return output, nil
}

// goEnvironment returns the process environment with GOTOOLCHAIN pinned to go.mod.
func goEnvironment() []string {
	version := moduleGoVersion()
	env := os.Environ()
	filtered := env[:0]
	for _, item := range env {
		if strings.HasPrefix(item, "GOTOOLCHAIN=") {
			continue
		}
		filtered = append(filtered, item)
	}
	if version != "" {
		filtered = append(filtered, "GOTOOLCHAIN=go"+version)
	}
	return filtered
}

// moduleGoVersion reads the module Go version for child Go commands.
func moduleGoVersion() string {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(), "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	return ""
}

// repositoryRoot returns the nearest parent directory containing go.mod.
func repositoryRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, statErr := os.Stat(filepath.Join(wd, "go.mod")); statErr == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "."
		}
		wd = parent
	}
}

// relativePath returns a slash-separated path relative to the repository root.
func relativePath(path string) string {
	rel, err := filepath.Rel(repositoryRoot(), path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// packageKey returns the short name used in generated tables.
func packageKey(relPath string) string {
	switch {
	case strings.HasPrefix(relPath, "internal/tools/"):
		return strings.TrimPrefix(relPath, "internal/tools/")
	case relPath == "internal/tools":
		return "tools"
	case strings.HasPrefix(relPath, "internal/"):
		return strings.TrimPrefix(relPath, "internal/")
	case strings.HasPrefix(relPath, "cmd/"):
		return relPath
	default:
		return relPath
	}
}

// classifyLayer maps a package path to the testing document layer model.
func classifyLayer(relPath string) string {
	switch {
	case relPath == "internal/tools":
		return "tools-orchestration"
	case strings.HasPrefix(relPath, "internal/tools/"):
		return "tool-subpackage"
	case strings.HasPrefix(relPath, "internal/"):
		return "core"
	case strings.HasPrefix(relPath, "cmd/"):
		return "cmd"
	case relPath == "test/e2e/suite":
		return "e2e"
	default:
		return "other"
	}
}

type layerTotal struct {
	tests int
	files int
}

// layerTotals aggregates test counts by layer.
func layerTotals(packages []packageMetrics) map[string]layerTotal {
	totals := map[string]layerTotal{}
	for _, pkg := range packages {
		total := totals[pkg.Layer]
		total.tests += pkg.TestFunctions
		total.files += pkg.TestFiles
		totals[pkg.Layer] = total
	}
	return totals
}

// packagesByLayer filters packages by layer.
func packagesByLayer(packages []packageMetrics, layer string) []packageMetrics {
	filtered := []packageMetrics{}
	for _, pkg := range packages {
		if pkg.Layer == layer {
			filtered = append(filtered, pkg)
		}
	}
	return filtered
}

// sortedCoveragePackages returns packages with coverage, sorted by key.
func sortedCoveragePackages(packages []packageMetrics) []packageMetrics {
	filtered := []packageMetrics{}
	for _, pkg := range packages {
		if pkg.Coverage.OK {
			filtered = append(filtered, pkg)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Key < filtered[j].Key })
	return filtered
}

// lowCoveragePackages returns packages below target, sorted by coverage then name.
func lowCoveragePackages(packages []packageMetrics, target float64) []packageMetrics {
	low := []packageMetrics{}
	for _, pkg := range packages {
		if pkg.Coverage.OK && pkg.Coverage.Percent < target {
			low = append(low, pkg)
		}
	}
	sort.Slice(low, func(i, j int) bool {
		if low[i].Coverage.Percent != low[j].Coverage.Percent {
			return low[i].Coverage.Percent < low[j].Coverage.Percent
		}
		return low[i].Key < low[j].Key
	})
	return low
}

// averageCoverage computes an unweighted package average over packages with coverage.
func averageCoverage(packages []packageMetrics) coverageValue {
	total := 0.0
	count := 0
	for _, pkg := range packages {
		if pkg.Coverage.OK {
			total += pkg.Coverage.Percent
			count++
		}
	}
	if count == 0 {
		return coverageValue{}
	}
	return coverageValue{Percent: total / float64(count), OK: true}
}

// coverageRationale returns a stable rationale for low-coverage packages.
func coverageRationale(pkg packageMetrics) string {
	switch {
	case pkg.Key == "testutil":
		return "some helpers are exercised by external packages or the build-tagged E2E suite rather than this package's own tests."
	case pkg.Key == "autoupdate":
		return "process replacement, platform-specific binary moves, and signal-handling paths cannot be fully exercised in-process."
	case pkg.Key == "wizard":
		return "interactive UI code, browser launch, and OS dialogs require heavy test stubbing."
	case pkg.Key == "cmd/server":
		return "entry-point glue, signal handling, and transport startup are validated mostly through integration and E2E coverage."
	case strings.HasPrefix(pkg.Key, "cmd/"):
		return "developer command formatting and reporting branches are covered by focused unit tests plus manual/CI tooling runs."
	default:
		return "review this package for missing unit coverage or add an explicit exception if the remaining paths are integration-only."
	}
}

// totalTests returns the total Test* function count across packages.
func totalTests(packages []packageMetrics) int {
	total := 0
	for _, pkg := range packages {
		total += pkg.TestFunctions
	}
	return total
}

// totalTestFiles returns the total _test.go file count across packages.
func totalTestFiles(packages []packageMetrics) int {
	total := 0
	for _, pkg := range packages {
		total += pkg.TestFiles
	}
	return total
}

// testFilesWithPrefix counts _test.go files under a path prefix.
func testFilesWithPrefix(packages []packageMetrics, prefix string) int {
	total := 0
	for _, pkg := range packages {
		if strings.HasPrefix(pkg.RelPath, prefix) {
			total += pkg.TestFiles
		}
	}
	return total
}

// countTestedPackages counts packages with at least one Test* function.
func countTestedPackages(packages []packageMetrics) int {
	total := 0
	for _, pkg := range packages {
		if pkg.TestFunctions > 0 {
			total++
		}
	}
	return total
}

// sumCounts sums a count map.
func sumCounts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

// fmtInt formats integers with thousands separators.
func fmtInt(n int) string {
	s := strconv.Itoa(n)
	if n < 1000 {
		return s
	}
	parts := []string{}
	for len(s) > 3 {
		parts = append(parts, s[len(s)-3:])
		s = s[:len(s)-3]
	}
	parts = append(parts, s)
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ",")
}

// fmtCoverage formats a coverage value for Markdown tables.
func fmtCoverage(value coverageValue) string {
	if !value.OK {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", value.Percent)
}

// fmtRatio formats a count as a percentage of total.
func fmtRatio(count, total int) string {
	if total == 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", float64(count)*100/float64(total))
}

// escapeTable escapes Markdown table separators inside cell text.
func escapeTable(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// tailLines returns the last n lines of text for command error messages.
func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
