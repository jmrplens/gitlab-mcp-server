// main_test.go covers the gen_testing_docs generator.
//
// Tests verify the AST-based test counter, naming-pattern classification,
// the catalog-tool-counting heuristics, the coverage parsers, the Markdown
// renderers, and the marker/legacy section replacement used to update
// docs/development/testing/testing.md. The go-list-backed collectors run
// against small fake modules written into temporary directories, so every
// number they produce is known in advance and the real repository is never
// scanned or written.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// goReleaseVersionRE matches the numeric part of a released Go toolchain
// version such as "1.27.0".
var goReleaseVersionRE = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

// legacyDoc is a testing document that still carries the historic unmarked
// Overview section, the shape the first generator run migrates.
const legacyDoc = "# Testing\n\n## Overview\n\nold metrics\n\n## Test Types\n\nManual notes.\n"

// TestParsePackageCoverages_ExtractsGoTestCoverage verifies the parser accepts
// normal and cached go test coverage lines while ignoring packages without
// coverage data.
func TestParsePackageCoverages_ExtractsGoTestCoverage(t *testing.T) {
	output := strings.Join([]string{
		"ok  github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil 0.018s coverage: 96.6% of statements",
		"ok  github.com/jmrplens/gitlab-mcp-server/v2/cmd/server (cached) coverage: 62.5% of statements",
		"?   github.com/jmrplens/gitlab-mcp-server/v2/cmd/add_docs [no test files]",
	}, "\n")

	coverages, err := parsePackageCoverages(output)
	if err != nil {
		t.Fatalf("parsePackageCoverages() error = %v", err)
	}
	if got := coverages["github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"].Percent; got != 96.6 {
		t.Fatalf("toolutil coverage = %.1f, want 96.6", got)
	}
	if got := coverages["github.com/jmrplens/gitlab-mcp-server/v2/cmd/server"].Percent; got != 62.5 {
		t.Fatalf("cmd/server coverage = %.1f, want 62.5", got)
	}
	if _, ok := coverages["github.com/jmrplens/gitlab-mcp-server/v2/cmd/add_docs"]; ok {
		t.Fatal("package without coverage should be ignored")
	}
}

// TestParsePackageCoverages_MalformedPercent_ReturnsError verifies a coverage
// figure the regular expression accepts but strconv rejects surfaces as an
// error naming the offending line instead of a silent zero.
func TestParsePackageCoverages_MalformedPercent_ReturnsError(t *testing.T) {
	_, err := parsePackageCoverages("ok  example.com/pkg 0.1s coverage: 1.2.3% of statements")
	if err == nil || !strings.Contains(err.Error(), "parse coverage line") {
		t.Fatalf("parsePackageCoverages() error = %v, want parse coverage line error", err)
	}
}

// TestParseTotalCoverage_ExtractsTotal verifies go tool cover -func total line parsing.
func TestParseTotalCoverage_ExtractsTotal(t *testing.T) {
	output := "github.com/example/project/file.go:10:\tRun\t80.0%\n" +
		"total:\t\t\t\t(statements)\t93.7%\n"

	got, err := parseTotalCoverage(output)
	if err != nil {
		t.Fatalf("parseTotalCoverage() error = %v", err)
	}
	if got != 93.7 {
		t.Fatalf("parseTotalCoverage() = %.1f, want 93.7", got)
	}
}

// TestParseTotalCoverage_MissingOrMalformed_ReturnsError verifies the total
// parser reports an absent total line and a total it cannot convert.
func TestParseTotalCoverage_MissingOrMalformed_ReturnsError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "no total line", output: "file.go:1:\tRun\t80.0%\n", want: "total coverage line not found"},
		{name: "malformed percent", output: "total:\t(statements)\t1.2.3%\n", want: "parse total coverage line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTotalCoverage(tt.output)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseTotalCoverage() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestCountTests_ClassifiesTestNames verifies AST-based test counting and
// naming pattern classification for Go test entry points.
func TestCountTests_ClassifiesTestNames(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "sample_test.go", `package sample

import "testing"

func TestWidget_Success(t *testing.T) {}
func TestWidget_Create_Success(t *testing.T) {}
func TestWidget(t *testing.T) {}
func TestCovWidgetBranch(t *testing.T) {}
func Testhelper(t *testing.T) {}
func TestMain(m *testing.M) {}
`)

	tests, files, counts, err := countTests(dir)
	if err != nil {
		t.Fatalf("countTests() error = %v", err)
	}
	if tests != 4 {
		t.Fatalf("test function count = %d, want 4", tests)
	}
	if files != 1 {
		t.Fatalf("test file count = %d, want 1", files)
	}
	expected := map[string]int{
		pattern2Part:        1,
		pattern3Part:        1,
		patternNoUnderscore: 1,
		patternTestCov:      1,
	}
	for pattern, want := range expected {
		t.Run(pattern, func(t *testing.T) {
			if got := counts[pattern]; got != want {
				t.Fatalf("pattern %s count = %d, want %d", pattern, got, want)
			}
		})
	}
}

// TestCountTests_MissingDirOrBrokenFile_ReturnsError verifies the counter
// reports an unreadable directory and a test file the parser rejects.
func TestCountTests_MissingDirOrBrokenFile_ReturnsError(t *testing.T) {
	broken := t.TempDir()
	writeFixture(t, broken, "broken_test.go", "package sample\n\nfunc TestBroken(t *testing.T) {\n")
	tests := []struct {
		name string
		dir  string
	}{
		{name: "missing directory", dir: filepath.Join(t.TempDir(), "absent")},
		{name: "unparseable test file", dir: broken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := countTests(tt.dir); err == nil {
				t.Fatal("countTests() error = nil, want error")
			}
		})
	}
}

// TestIsTestFunction_NameShapes_MatchGoRules verifies the Test* entry-point
// rule: a bare Test, an uppercase follower, and rejection of TestMain, a
// lowercase follower, and a non-Test prefix.
func TestIsTestFunction_NameShapes_MatchGoRules(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "Test", want: true},
		{name: "TestWidget", want: true},
		{name: "Test_Widget", want: true},
		{name: "TestMain", want: false},
		{name: "Testwidget", want: false},
		{name: "BenchmarkWidget", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTestFunction(tt.name); got != tt.want {
				t.Fatalf("isTestFunction(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestCountMCPTools_CountsAddToolCalls verifies tool-count extraction from
// mcp.AddTool calls without executing registration code.
func TestCountMCPTools_CountsAddToolCalls(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "register.go", `package sample

func RegisterTools() {
	mcp.AddTool(server, toolA, handlerA)
	mcp.AddTool(server, toolB, handlerB)
	toolutil.AddMetaTool(server, toolC, routes, icons, handlerC)
}
`)
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o750); err != nil {
		t.Fatalf("create nested fixture dir: %v", err)
	}
	writeFixture(t, nested, "register.go", `package nested

func RegisterTools() {
	mcp.AddTool(server, toolNested, handlerNested)
}
`)

	got, err := countMCPTools(dir)
	if err != nil {
		t.Fatalf("countMCPTools() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("countMCPTools() = %d, want 2", got)
	}
}

// TestCountMCPTools_MissingDirOrBrokenFile_ReturnsError verifies the AddTool
// scanner reports an unreadable directory and a source file the parser
// rejects, while ignoring test files and other extensions.
func TestCountMCPTools_MissingDirOrBrokenFile_ReturnsError(t *testing.T) {
	broken := t.TempDir()
	writeFixture(t, broken, "register.go", "package sample\n\nfunc RegisterTools() {\n")
	writeFixture(t, broken, "notes.txt", "not go")
	tests := []struct {
		name string
		dir  string
	}{
		{name: "missing directory", dir: filepath.Join(t.TempDir(), "absent")},
		{name: "unparseable source file", dir: broken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := countMCPTools(tt.dir); err == nil {
				t.Fatal("countMCPTools() error = nil, want error")
			}
		})
	}
}

// TestCountLocalActionSpecTools_CountsPackageOwnedSpecs verifies catalog-backed
// tool counts can be derived from domain-local ActionSpecs without executing handlers.
func TestCountLocalActionSpecTools_CountsPackageOwnedSpecs(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "action_specs.go", `package sample

func ActionSpecs(client *Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		sampleSpec("list"),
		sampleSpec("get"),
	}
	if enterprise {
		specs = append(specs,
			sampleSpec("create"),
			sampleSpec("delete"),
		)
	}
	specs = append(specs, external.ActionSpecs(client)...)
	return specs
}

func GroupActionSpecs(client *Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		sampleSpec("group_list"),
	}
}
`)

	got, err := countLocalActionSpecTools(dir)
	if err != nil {
		t.Fatalf("countLocalActionSpecTools() error = %v", err)
	}
	if got != 5 {
		t.Fatalf("countLocalActionSpecTools() = %d, want 5", got)
	}
}

// TestCountLocalActionSpecTools_BuilderShapes_CountsEachBranch verifies every
// statement shape the ActionSpecs walker understands: literal and append
// assignments, non-identifier targets, surplus right-hand sides, if/else-if/else
// chains, for and range loops, nested blocks, direct literal and append
// returns, opaque call returns, non-ActionSpec literals, pointer element
// types, bodiless declarations, and the CollectActionSpecs exemption.
func TestCountLocalActionSpecTools_BuilderShapes_CountsEachBranch(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "shapes.go", `package sample

func BranchActionSpecs(mode int) []ActionSpec {
	specs := []ActionSpec{spec("a")}
	specs = append(specs, spec("b"), []ActionSpec{spec("c"), spec("d")})
	holder.specs = []ActionSpec{spec("ignored")}
	specs = spec("x"), spec("y")
	if mode == 1 {
		return specs
	} else if mode == 2 {
		return []ActionSpec{spec("e")}
	} else {
		specs = append(specs, spec("f"))
	}
	for i := 0; i < 1; i++ {
		{
			specs = append(specs, spec("g"))
		}
	}
	for range specs {
		specs = append(specs)
	}
	log("noop")
	return specs, other(), Other{}, []string{"h"}, []*ActionSpec{nil}
}

func AppendActionSpecs(base []ActionSpec) []ActionSpec {
	return append(base, spec("y"), spec("z"))
}

func ExternalActionSpecs() []ActionSpec

func CollectActionSpecs() []ActionSpec {
	return []ActionSpec{spec("not counted")}
}

func Helper() []ActionSpec {
	return []ActionSpec{spec("not a builder")}
}
`)
	writeFixture(t, filepath.Join(dir, "nested"), "nested.go", "package nested\n\nfunc NestedActionSpecs() []ActionSpec {\n\treturn []ActionSpec{spec(\"ignored\")}\n}\n")

	got, err := countLocalActionSpecTools(dir)
	if err != nil {
		t.Fatalf("countLocalActionSpecTools() error = %v", err)
	}
	// In BranchActionSpecs, specs starts at 1 and grows by 3 (b, c, d)
	// before the if chain, whose first branch returns it at 4 and whose
	// else-if returns a literal of 1; the else and the loop then add f and
	// g, so the final return reads 6. AppendActionSpecs returns an append of
	// two, and the nested directory is not this package.
	if got != 13 {
		t.Fatalf("countLocalActionSpecTools() = %d, want 13", got)
	}
}

// TestCountLocalActionSpecTools_MissingDirOrBrokenFile_ReturnsError verifies
// the ActionSpecs scanner reports an unreadable directory and a source file
// the parser rejects.
func TestCountLocalActionSpecTools_MissingDirOrBrokenFile_ReturnsError(t *testing.T) {
	broken := t.TempDir()
	writeFixture(t, broken, "action_specs.go", "package sample\n\nfunc ActionSpecs() []ActionSpec {\n")
	tests := []struct {
		name string
		dir  string
	}{
		{name: "missing directory", dir: filepath.Join(t.TempDir(), "absent")},
		{name: "unparseable source file", dir: broken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := countLocalActionSpecTools(tt.dir); err == nil {
				t.Fatal("countLocalActionSpecTools() error = nil, want error")
			}
		})
	}
}

// TestActionSpecToolCounts_RealCatalog_CountsDistinctToolsPerOwner verifies the
// runtime catalog projection yields a positive distinct-tool count for every
// owning package, including a well-known domain.
func TestActionSpecToolCounts_RealCatalog_CountsDistinctToolsPerOwner(t *testing.T) {
	counts := actionSpecToolCounts()
	if len(counts) == 0 {
		t.Fatal("actionSpecToolCounts() returned no owners")
	}
	if counts["issues"] == 0 {
		t.Fatalf("actionSpecToolCounts()[issues] = 0, want > 0 (owners: %d)", len(counts))
	}
	for owner, count := range counts {
		t.Run(owner, func(t *testing.T) {
			if count <= 0 {
				t.Fatalf("owner %s count = %d, want > 0", owner, count)
			}
		})
	}
}

// TestParseCoverageProfileTotal_FiltersMatchingFiles verifies internal-only
// coverage totals can be derived from one combined coverage profile.
func TestParseCoverageProfileTotal_FiltersMatchingFiles(t *testing.T) {
	profile := strings.Join([]string{
		"mode: count",
		"github.com/example/project/internal/a/a.go:1.1,2.1 2 1",
		"github.com/example/project/internal/a/b.go:3.1,4.1 3 0",
		"github.com/example/project/cmd/app/main.go:1.1,2.1 5 1",
	}, "\n")

	got, err := parseCoverageProfileTotal(profile, func(fileName string) bool {
		return strings.Contains(fileName, "/internal/")
	})
	if err != nil {
		t.Fatalf("parseCoverageProfileTotal() error = %v", err)
	}
	if !got.OK {
		t.Fatal("parseCoverageProfileTotal() OK = false, want true")
	}
	if got.Percent != 40 {
		t.Fatalf("parseCoverageProfileTotal() = %.1f, want 40.0", got.Percent)
	}
}

// TestParseCoverageProfileTotal_EdgeLines_HandledOrRejected verifies a profile
// with no matching statements yields an unmeasured value, and that malformed
// field counts, missing locations, and non-numeric counters are rejected.
func TestParseCoverageProfileTotal_EdgeLines_HandledOrRejected(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantErr string
		wantOK  bool
	}{
		{name: "no statements", profile: "mode: count\n\nexample.com/x/cmd/a.go:1.1,2.1 3 1\n"},
		{name: "wrong field count", profile: "example.com/x/internal/a.go:1.1,2.1 3\n", wantErr: "unexpected coverage profile line"},
		{name: "missing location", profile: "example.com/x/internal/a.go 3 1\n", wantErr: "missing location"},
		{name: "bad statement count", profile: "example.com/x/internal/a.go:1.1,2.1 x 1\n", wantErr: "parse statement count"},
		{name: "bad coverage count", profile: "example.com/x/internal/a.go:1.1,2.1 3 y\n", wantErr: "parse coverage count"},
		{name: "measured", profile: "example.com/x/internal/a.go:1.1,2.1 3 2\n", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoverageProfileTotal(tt.profile, func(fileName string) bool {
				return strings.Contains(fileName, "/internal/")
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseCoverageProfileTotal() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCoverageProfileTotal() error = %v", err)
			}
			if got.OK != tt.wantOK {
				t.Fatalf("parseCoverageProfileTotal() OK = %v, want %v", got.OK, tt.wantOK)
			}
		})
	}
}

// TestReplaceGeneratedBlock_MigratesLegacySection verifies first-run migration
// from the historic unmarked Overview/Coverage block to managed markers.
func TestReplaceGeneratedBlock_MigratesLegacySection(t *testing.T) {
	legacy := "# Testing\n\n## Overview\n\nold metrics\n\n## Test Types\n\nmanual content\n"

	updated, err := replaceGeneratedBlock(legacy, "## Overview\n\nnew metrics\n")
	if err != nil {
		t.Fatalf("replaceGeneratedBlock() error = %v", err)
	}
	for _, want := range []string{startMarker, "new metrics", endMarker, "## Test Types", "manual content"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(updated, want) {
				t.Fatalf("updated content missing %q:\n%s", want, updated)
			}
		})
	}
	if strings.Contains(updated, "old metrics") {
		t.Fatalf("legacy metrics should have been replaced:\n%s", updated)
	}
}

// TestReplaceGeneratedBlock_ReplacesMarkedSection verifies subsequent runs only
// rewrite the managed marker block.
func TestReplaceGeneratedBlock_ReplacesMarkedSection(t *testing.T) {
	marked := "# Testing\n\n" + startMarker + "\n\nold\n" + endMarker + "\n\n## Test Types\n\nmanual\n"

	updated, err := replaceGeneratedBlock(marked, "new\n")
	if err != nil {
		t.Fatalf("replaceGeneratedBlock() error = %v", err)
	}
	if !strings.Contains(updated, startMarker+"\n\nnew\n\n"+endMarker) {
		t.Fatalf("marked section not replaced as expected:\n%s", updated)
	}
	if !strings.Contains(updated, "manual") {
		t.Fatalf("manual content should be preserved:\n%s", updated)
	}
}

// TestReplaceGeneratedBlock_MissingAnchors_ReturnsError verifies every anchor
// the replacement needs is reported when absent: a lone end marker, a start
// marker without its end, a document without the legacy Overview heading, and
// one whose Overview never reaches the Test Types heading.
func TestReplaceGeneratedBlock_MissingAnchors_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "end marker only", text: "# T\n\n" + endMarker + "\n", want: "start marker"},
		{name: "start marker only", text: "# T\n\n" + startMarker + "\n", want: "end marker"},
		{name: "no overview heading", text: "# T\n\n## Test Types\n", want: "fallback start heading"},
		{name: "no test types heading", text: "# T\n\n## Overview\n\nold\n", want: "fallback end heading"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := replaceGeneratedBlock(tt.text, "new\n")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("replaceGeneratedBlock() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestRelativePath_UsesRepositoryRoot verifies absolute go list paths are
// converted to module-relative paths before package layer classification.
func TestRelativePath_UsesRepositoryRoot(t *testing.T) {
	root := repositoryRoot()
	got := relativePath(filepath.Join(root, "internal", "toolutil"))
	if got != "internal/toolutil" {
		t.Fatalf("relativePath() = %q, want internal/toolutil", got)
	}
	if layer := classifyLayer(got); layer != "core" {
		t.Fatalf("classifyLayer(%q) = %q, want core", got, layer)
	}
}

// TestRepositoryRoot_NoGoMod_FallsBackToDot verifies that outside any module
// the root is ".", the Go version pin is empty, and the child environment
// carries no GOTOOLCHAIN entry at all, even when the parent had one; an
// absolute path then stays absolute since it cannot be made relative to ".".
func TestRepositoryRoot_NoGoMod_FallsBackToDot(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Chdir(t.TempDir())

	if got := repositoryRoot(); got != "." {
		t.Fatalf("repositoryRoot() = %q, want .", got)
	}
	if got := moduleGoVersion(); got != "" {
		t.Fatalf("moduleGoVersion() = %q, want empty", got)
	}
	for _, item := range goEnvironment() {
		if strings.HasPrefix(item, "GOTOOLCHAIN=") {
			t.Fatalf("goEnvironment() still carries %q", item)
		}
	}
	abs := filepath.Join(t.TempDir(), "elsewhere")
	if got := relativePath(abs); got != filepath.ToSlash(abs) {
		t.Fatalf("relativePath(%q) = %q, want the path unchanged", abs, got)
	}
}

// TestGoEnvironment_ModuleVersion_PinsToolchain verifies the go directive of
// the nearest go.mod becomes the single GOTOOLCHAIN entry handed to child Go
// commands, replacing whatever the parent process had, and that a go.mod
// without a directive pins nothing.
func TestGoEnvironment_ModuleVersion_PinsToolchain(t *testing.T) {
	tests := []struct {
		name        string
		goMod       string
		wantVersion string
		wantPin     string
	}{
		{name: "directive present", goMod: "module example.com/pinned\n\ngo 1.99.0\n", wantVersion: "1.99.0", wantPin: "GOTOOLCHAIN=go1.99.0"},
		{name: "directive absent", goMod: "module example.com/unpinned\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "go.mod", tt.goMod)
			t.Setenv("GOTOOLCHAIN", "auto")
			t.Chdir(root)

			if got := moduleGoVersion(); got != tt.wantVersion {
				t.Fatalf("moduleGoVersion() = %q, want %q", got, tt.wantVersion)
			}
			var pins []string
			for _, item := range goEnvironment() {
				if strings.HasPrefix(item, "GOTOOLCHAIN=") {
					pins = append(pins, item)
				}
			}
			if tt.wantPin == "" && len(pins) != 0 {
				t.Fatalf("goEnvironment() pinned %v, want no pin", pins)
			}
			if tt.wantPin != "" && (len(pins) != 1 || pins[0] != tt.wantPin) {
				t.Fatalf("goEnvironment() pinned %v, want exactly %q", pins, tt.wantPin)
			}
		})
	}
	if got := goExecutable(); !filepath.IsAbs(got) || strings.TrimSuffix(filepath.Base(got), ".exe") != "go" || filepath.Base(filepath.Dir(got)) != "bin" {
		t.Fatalf("goExecutable() = %q, want an absolute .../bin/go path", got)
	}
}

// TestRepositoryRoot_DeletedWorkingDirectory_FallsBackToDot verifies a
// working directory removed underneath the process makes the root "." and
// path resolution fail with the root error rather than a stale path.
func TestRepositoryRoot_DeletedWorkingDirectory_FallsBackToDot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the working directory cannot be removed while in use on Windows")
	}
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatalf("remove working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform still resolves a deleted working directory")
	}

	if got := repositoryRoot(); got != "." {
		t.Fatalf("repositoryRoot() = %q, want .", got)
	}
	if _, err := resolveRepositoryPath("x.md"); err == nil || !strings.Contains(err.Error(), "resolve repository root") {
		t.Fatalf("resolveRepositoryPath() error = %v, want resolve repository root error", err)
	}
}

// TestParseOptions_FlagShapes_ParsedOrRejected verifies the defaults, every
// flag override, and the three rejection paths: an unknown flag, positional
// arguments, and a non-positive table cap.
func TestParseOptions_FlagShapes_ParsedOrRejected(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{
			name: "defaults",
			want: options{docPath: defaultDocPath, topToolRows: 25, timeout: 15 * time.Minute},
		},
		{
			name: "all flags",
			args: []string{"--file", "x.md", "--check", "--skip-coverage", "--top-tool-rows", "3", "--timeout", "1m", "--coverage-dir", "cov", "--include-e2e-run"},
			want: options{docPath: "x.md", check: true, skipCoverage: true, topToolRows: 3, timeout: time.Minute, coverageDir: "cov", includeE2ERun: true},
		},
		{name: "unknown flag", args: []string{"--bogus"}, wantErr: true},
		{name: "positional argument", args: []string{"extra"}, wantErr: true},
		{name: "zero rows", args: []string{"--top-tool-rows", "0"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseOptions() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOptions() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCollectMetrics_FakeModule_CountsEveryLayer verifies the go-list-backed
// collector against a module whose contents are known: package keys, layers,
// summaries, test counts, naming buckets, and the tool-count precedence between
// the legacy AddTool scan, the local ActionSpecs scan, and the runtime catalog.
func TestCollectMetrics_FakeModule_CountsEveryLayer(t *testing.T) {
	root := writeFakeModule(t, fakeModuleFiles())
	t.Chdir(root)

	metrics, err := collectMetrics(t.Context(), options{skipCoverage: true, timeout: time.Minute})
	if err != nil {
		t.Fatalf("collectMetrics() error = %v", err)
	}
	byKey := map[string]packageMetrics{}
	for _, pkg := range metrics.Packages {
		byKey[pkg.Key] = pkg
	}
	if len(byKey) != 6 {
		t.Fatalf("collectMetrics() found %d packages, want 6: %v", len(byKey), byKey)
	}
	catalogIssues := actionSpecToolCounts()["issues"]
	tests := []struct {
		key     string
		layer   string
		summary string
		tests   int
		files   int
		tools   int
	}{
		{key: "cmd/app", layer: layerCmd, summary: "Package main is the fake entry point.", tests: 1, files: 1},
		{key: "core", layer: layerCore, summary: "Package core adds numbers.", tests: 3, files: 1},
		{key: "tools", layer: layerToolsOrchestration, summary: "Package tools orchestrates the fake catalog.", tests: 1, files: 1},
		{key: "issues", layer: layerToolSubpackage, summary: "Package issues mirrors a real catalog owner.", tests: 1, files: 1, tools: catalogIssues},
		{key: "widgets", layer: layerToolSubpackage, summary: "Package documentation unavailable.", tests: 1, files: 1, tools: 3},
		{key: "test/e2e/suite", layer: layerE2E, summary: "Package documentation unavailable.", tests: 2, files: 1},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			pkg, ok := byKey[tt.key]
			if !ok {
				t.Fatalf("package %s not collected", tt.key)
			}
			got := packageMetrics{Key: pkg.Key, Layer: pkg.Layer, Summary: pkg.Summary, TestFunctions: pkg.TestFunctions, TestFiles: pkg.TestFiles, ToolCount: pkg.ToolCount}
			want := packageMetrics{Key: tt.key, Layer: tt.layer, Summary: tt.summary, TestFunctions: tt.tests, TestFiles: tt.files, ToolCount: tt.tools}
			if got != want {
				t.Fatalf("package %s = %+v, want %+v", tt.key, got, want)
			}
			if pkg.Coverage.OK {
				t.Fatalf("package %s has coverage %+v with coverage skipped", tt.key, pkg.Coverage)
			}
		})
	}
	wantNaming := map[string]int{pattern3Part: 5, pattern2Part: 1, patternTestCov: 1, patternNoUnderscore: 2}
	for pattern, want := range wantNaming {
		t.Run(pattern, func(t *testing.T) {
			if got := metrics.NamingCounts[pattern]; got != want {
				t.Fatalf("naming count %s = %d, want %d", pattern, got, want)
			}
		})
	}
	if metrics.AveragePackageCoverage.OK {
		t.Fatalf("average coverage = %+v, want unmeasured", metrics.AveragePackageCoverage)
	}
}

// TestCollectMetrics_FakeModule_RunsCoverageAndE2E verifies the full path:
// unit coverage is measured per package and in total, the internal-only total
// excludes cmd statements, the unweighted average spans measured packages
// only, and the optional E2E run replaces the footnote.
func TestCollectMetrics_FakeModule_RunsCoverageAndE2E(t *testing.T) {
	root := writeFakeModule(t, fakeModuleFiles())
	t.Chdir(root)
	coverageDir := filepath.Join(root, "coverage")

	metrics, err := collectMetrics(t.Context(), options{timeout: 5 * time.Minute, coverageDir: coverageDir, includeE2ERun: true})
	if err != nil {
		t.Fatalf("collectMetrics() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(coverageDir, "cmd-internal.out")); statErr != nil {
		t.Fatalf("coverage profile not kept in the configured directory: %v", statErr)
	}
	byKey := map[string]coverageValue{}
	for _, pkg := range metrics.Packages {
		byKey[pkg.Key] = pkg.Coverage
	}
	tests := []struct {
		key  string
		want string
	}{
		{key: "cmd/app", want: "0.0%"},
		{key: "core", want: "50.0%"},
		{key: "tools", want: "n/a"},
		{key: "issues", want: "100.0%"},
		{key: "widgets", want: "66.7%"},
		{key: "test/e2e/suite", want: "n/a"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := fmtCoverage(byKey[tt.key]); got != tt.want {
				t.Fatalf("coverage %s = %s, want %s", tt.key, got, tt.want)
			}
		})
	}
	if got := fmtCoverage(metrics.OverallCoverage); got != "57.1%" {
		t.Fatalf("overall coverage = %s, want 57.1%%", got)
	}
	if got := fmtCoverage(metrics.InternalCoverage); got != "66.7%" {
		t.Fatalf("internal coverage = %s, want 66.7%%", got)
	}
	if got := fmtCoverage(metrics.AveragePackageCoverage); got != "54.2%" {
		t.Fatalf("average coverage = %s, want 54.2%%", got)
	}
	if !strings.HasPrefix(metrics.E2ENote, "E2E tests were executed") {
		t.Fatalf("E2E note = %q, want the executed variant", metrics.E2ENote)
	}
}

// TestCollectMetrics_FakeModuleFailures_ReturnEachError verifies each failure
// the collector can meet is reported with its own context: a module the
// listing cannot resolve, a warning row the listing cannot parse, a test file
// the counter cannot parse, a tool source the AddTool scan cannot parse, a
// failing unit test under coverage, and a failing E2E run.
func TestCollectMetrics_FakeModuleFailures_ReturnEachError(t *testing.T) {
	failingTest := "package core\n\nimport \"testing\"\n\nfunc TestAdd_Fails_OnPurpose(t *testing.T) {\n\tt.Fatal(\"forced failure\")\n}\n"
	tests := []struct {
		name     string
		mutate   func(files map[string]string)
		opts     options
		wantErr  string
		wantDirs []string
	}{
		{
			name:    "missing e2e tree",
			mutate:  func(files map[string]string) { delete(files, "test/e2e/suite/flow_test.go") },
			opts:    options{skipCoverage: true},
			wantErr: "list packages",
		},
		{
			name:     "warning row",
			mutate:   func(files map[string]string) { delete(files, "test/e2e/suite/flow_test.go") },
			opts:     options{skipCoverage: true},
			wantErr:  "unexpected go list row",
			wantDirs: []string{"test/e2e"},
		},
		{
			name: "unparseable test file",
			mutate: func(files map[string]string) {
				files["internal/core/core_test.go"] = "package core\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) {\n"
			},
			opts:    options{skipCoverage: true},
			wantErr: "count tests in internal/core",
		},
		{
			name: "unparseable tool source",
			mutate: func(files map[string]string) {
				files["internal/tools/widgets/widgets.go"] = "package widgets\n\nfunc Register() {\n"
			},
			opts:    options{skipCoverage: true},
			wantErr: "count MCP tools in internal/tools/widgets",
		},
		{
			name:    "failing unit test",
			mutate:  func(files map[string]string) { files["internal/core/core_test.go"] = failingTest },
			opts:    options{},
			wantErr: "run coverage for ./internal/... ./cmd/...",
		},
		{
			name: "failing e2e test",
			mutate: func(files map[string]string) {
				files["test/e2e/suite/flow_test.go"] = "//go:build e2e\n\n" + strings.Replace(failingTest, "package core", "package suite", 1)
			},
			opts:    options{skipCoverage: true, includeE2ERun: true},
			wantErr: "run e2e tests",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := fakeModuleFiles()
			tt.mutate(files)
			root := writeFakeModule(t, files)
			for _, dir := range tt.wantDirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
					t.Fatalf("create %s: %v", dir, err)
				}
			}
			t.Chdir(root)
			opts := tt.opts
			opts.timeout = 5 * time.Minute
			opts.coverageDir = filepath.Join(root, "coverage")

			_, err := collectMetrics(t.Context(), opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("collectMetrics() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestRunUnitCoverage_UnwritableCoverageDir_ReturnsError verifies a coverage
// directory that cannot be created is reported before any Go command runs.
func TestRunUnitCoverage_UnwritableCoverageDir_ReturnsError(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "file", "not a directory")

	_, _, _, err := runUnitCoverage(t.Context(), options{coverageDir: filepath.Join(root, "file", "sub")})
	if err == nil || !strings.Contains(err.Error(), "create coverage dir") {
		t.Fatalf("runUnitCoverage() error = %v, want create coverage dir error", err)
	}
}

// TestCoverageDirectory_UnusableTempRoot_ReturnsError verifies the
// unconfigured case reports a temporary root that does not exist instead of
// handing back a directory nothing can write to.
func TestCoverageDirectory_UnusableTempRoot_ReturnsError(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")
	t.Setenv("TMPDIR", absent)
	t.Setenv("TMP", absent)
	t.Setenv("TEMP", absent)

	_, cleanup, err := coverageDirectory("")
	if cleanup != nil {
		cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), "create temp coverage dir") {
		t.Fatalf("coverageDirectory() error = %v, want create temp coverage dir error", err)
	}
}

// TestCoverageDirectory_TempDir_CleansUp verifies the unconfigured case
// creates a fresh temporary directory whose cleanup callback removes it.
func TestCoverageDirectory_TempDir_CleansUp(t *testing.T) {
	dir, cleanup, err := coverageDirectory("")
	if err != nil {
		t.Fatalf("coverageDirectory() error = %v", err)
	}
	if cleanup == nil {
		t.Fatal("coverageDirectory() cleanup = nil, want a callback")
	}
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		t.Fatalf("coverageDirectory() dir %q not created: %v", dir, statErr)
	}
	cleanup()
	if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cleanup left %q behind: %v", dir, statErr)
	}
}

// TestRunGo_FailingCommand_IncludesOutputTail verifies a failing Go command is
// wrapped with its arguments and the tail of its combined output, so the
// operator sees the compiler or test message rather than only an exit code.
func TestRunGo_FailingCommand_IncludesOutputTail(t *testing.T) {
	t.Chdir(writeFakeModule(t, map[string]string{"go.mod": fakeGoMod()}))

	_, err := runGo(t.Context(), []string{"list", "./nowhere/..."})
	if err == nil {
		t.Fatal("runGo() error = nil, want error")
	}
	for _, want := range []string{"go list ./nowhere/...", "nowhere"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("runGo() error %q missing %q", err, want)
			}
		})
	}
}

// TestRun_FakeModule_MigratesChecksAndReports verifies the command end to end
// on a fake module: the first run migrates the legacy section and reports the
// update, a second run reports nothing changed, --check accepts the current
// document, and --check rejects a document edited afterwards.
func TestRun_FakeModule_MigratesChecksAndReports(t *testing.T) {
	root := writeFakeModule(t, fakeModuleFiles())
	t.Chdir(root)
	docPath := filepath.Join(root, "docs", "testing.md")
	args := []string{"--skip-coverage", "--file", "docs/testing.md"}

	steps := []struct {
		name    string
		args    []string
		before  func()
		wantOut string
		wantErr string
	}{
		{name: "first run migrates", args: args, wantOut: "Updated docs/testing.md"},
		{name: "second run is idle", args: args, wantOut: "docs/testing.md already up to date"},
		{name: "check accepts current", args: append([]string{"--check"}, args...), wantOut: "docs/testing.md is up to date"},
		{
			name: "check rejects stale",
			args: append([]string{"--check"}, args...),
			before: func() {
				data, readErr := os.ReadFile(docPath)
				if readErr != nil {
					t.Fatalf("read doc: %v", readErr)
				}
				edited := strings.Replace(string(data), "| Total test functions", "| Total test functions (edited)", 1)
				if writeErr := os.WriteFile(docPath, []byte(edited), 0o600); writeErr != nil { //#nosec G703 -- docPath sits inside the temporary module this test created
					t.Fatalf("edit doc: %v", writeErr)
				}
			},
			wantErr: "docs/testing.md is out of date",
		},
	}
	// sequential: each step depends on the document state the previous one left
	for _, step := range steps {
		if step.before != nil {
			step.before()
		}
		var out bytes.Buffer
		err := run(step.args, &out)
		if step.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), step.wantErr) {
				t.Fatalf("%s: run() error = %v, want %q", step.name, err, step.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: run() error = %v", step.name, err)
		}
		if got := strings.TrimSpace(out.String()); got != step.wantOut {
			t.Fatalf("%s: run() output = %q, want %q", step.name, got, step.wantOut)
		}
	}

	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}
	doc := string(data)
	for _, want := range []string{startMarker, endMarker, "Manual notes.", "| issues ", "| widgets "} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(doc, want) {
				t.Fatalf("generated document missing %q:\n%s", want, doc)
			}
		})
	}
	if strings.Contains(doc, "old metrics") {
		t.Fatalf("legacy metrics survived migration:\n%s", doc)
	}
}

// TestRun_FakeModuleWithCoverage_WritesMeasuredTables verifies the coverage
// run lands in the document: the overall, internal, and average figures, the
// per-package snapshot, and the below-target list with its rationale.
func TestRun_FakeModuleWithCoverage_WritesMeasuredTables(t *testing.T) {
	root := writeFakeModule(t, fakeModuleFiles())
	t.Chdir(root)

	var out bytes.Buffer
	if err := run([]string{"--file", "docs/testing.md", "--coverage-dir", filepath.Join(root, "coverage"), "--top-tool-rows", "1"}, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "testing.md"))
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}
	doc := string(data)
	rows := []struct {
		label string
		want  []string
	}{
		{label: "Overall coverage (`go test ./internal/... ./cmd/...`)", want: []string{"57.1%"}},
		{label: "Overall coverage (`go test ./internal/...`)", want: []string{"66.7%"}},
		{label: "Average package coverage", want: []string{"54.2%"}},
		{label: "core", want: []string{"3", "50.0%", "Package core adds numbers."}},
		{label: "cmd/app", want: []string{"0.0%"}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, doc, row.label, row.want)
		})
	}
	wantLines := []string{
		"- **cmd/app** (0.0%) - developer command formatting",
		"- **core** (50.0%) - review this package",
		"- **widgets** (66.7%) - review this package",
	}
	for _, want := range wantLines {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(doc, want) {
				t.Fatalf("generated document missing %q:\n%s", want, doc)
			}
		})
	}
	if got := strings.Count(doc, "| widgets "); got != 2 {
		t.Fatalf("widgets rows = %d, want 2 (complete table and coverage snapshot, not the capped top table):\n%s", got, doc)
	}
}

// TestRun_Failures_ReturnEachError verifies the entry point surfaces flag
// errors, collector errors, and document errors instead of a partial update.
func TestRun_Failures_ReturnEachError(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(files map[string]string)
		args    []string
		wantErr string
	}{
		{name: "bad flag", args: []string{"--top-tool-rows", "0"}, wantErr: "top-tool-rows"},
		{
			name:    "collector failure",
			mutate:  func(files map[string]string) { delete(files, "test/e2e/suite/flow_test.go") },
			args:    []string{"--skip-coverage", "--file", "docs/testing.md"},
			wantErr: "list packages",
		},
		{
			name:    "missing document",
			args:    []string{"--skip-coverage", "--file", "docs/absent.md"},
			wantErr: "read docs/absent.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := fakeModuleFiles()
			if tt.mutate != nil {
				tt.mutate(files)
			}
			t.Chdir(writeFakeModule(t, files))

			err := run(tt.args, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestUpdateManagedSection_PathShapes_WriteOrRefuse verifies the document
// update in check and write mode: an unchanged document reports no change, a
// changed one is rewritten only outside check mode, a path escaping the
// repository is refused, and a document without anchors is rejected.
func TestUpdateManagedSection_PathShapes_WriteOrRefuse(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/docs\n")
	t.Chdir(root)
	current := "# T\n\n" + startMarker + "\n\nsame\n\n" + endMarker + "\n"
	tests := []struct {
		name        string
		path        string
		text        string
		check       bool
		wantChanged bool
		wantErr     string
		wantWritten string
	}{
		{name: "unchanged", path: "same.md", text: current, wantWritten: current},
		{name: "changed in check mode", path: "check.md", text: legacyDoc, check: true, wantChanged: true, wantWritten: legacyDoc},
		{name: "changed and written", path: "write.md", text: legacyDoc, wantChanged: true, wantWritten: startMarker + "\n\nsame\n\n" + endMarker},
		{name: "outside repository", path: filepath.Join("..", "escape.md"), wantErr: "outside repository root"},
		{name: "no anchors", path: "anchorless.md", text: "# T\n\nplain\n", wantErr: "fallback start heading"},
		{name: "missing file", path: "absent.md", wantErr: "read absent.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.text != "" {
				writeFixture(t, root, tt.path, tt.text)
			}
			changed, err := updateManagedSection(tt.path, "same\n", tt.check)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("updateManagedSection() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("updateManagedSection() error = %v", err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("updateManagedSection() changed = %v, want %v", changed, tt.wantChanged)
			}
			data, readErr := os.ReadFile(filepath.Join(root, tt.path))
			if readErr != nil {
				t.Fatalf("read %s: %v", tt.path, readErr)
			}
			if !strings.Contains(string(data), tt.wantWritten) {
				t.Fatalf("%s content = %q, want it to contain %q", tt.path, data, tt.wantWritten)
			}
		})
	}
}

// TestUpdateManagedSection_ReadOnlyDocument_ReturnsWriteError verifies a
// document that can be read but not written is reported as a write failure.
// Root bypasses file permission bits on Unix, so the case is skipped there.
func TestUpdateManagedSection_ReadOnlyDocument_ReturnsWriteError(t *testing.T) {
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		t.Skip("file permission bits do not restrict root")
	}
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/docs\n")
	writeFixture(t, root, "readonly.md", legacyDoc)
	if err := os.Chmod(filepath.Join(root, "readonly.md"), 0o400); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Chdir(root)

	_, err := updateManagedSection("readonly.md", "new\n", false)
	if err == nil || !strings.Contains(err.Error(), "write readonly.md") {
		t.Fatalf("updateManagedSection() error = %v, want write error", err)
	}
}

// TestResolveRepositoryPath_RelativeAndAbsolute_StayInsideRoot verifies
// relative paths are anchored at the module root, absolute paths inside the
// root are accepted as given, and parent traversal is refused.
func TestResolveRepositoryPath_RelativeAndAbsolute_StayInsideRoot(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/docs\n")
	t.Chdir(root)
	absRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "relative", path: filepath.Join("docs", "t.md"), want: filepath.Join(absRoot, "docs", "t.md")},
		{name: "absolute inside", path: filepath.Join(absRoot, "README.md"), want: filepath.Join(absRoot, "README.md")},
		{name: "root itself", path: ".", want: absRoot},
		{name: "parent traversal", path: filepath.Join("..", "x.md"), wantErr: true},
		{name: "absolute outside", path: filepath.Join(filepath.Dir(absRoot), "other.md"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, resolveErr := resolveRepositoryPath(tt.path)
			if tt.wantErr {
				if resolveErr == nil {
					t.Fatalf("resolveRepositoryPath(%q) = %q, want error", tt.path, got)
				}
				return
			}
			if resolveErr != nil {
				t.Fatalf("resolveRepositoryPath(%q) error = %v", tt.path, resolveErr)
			}
			if got != tt.want {
				t.Fatalf("resolveRepositoryPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestPackageSummary_DocShapes_FirstSentenceOrFallback verifies the summary
// comes from the first documented non-test file, skips files without a doc
// comment or with parse errors, and falls back for an unreadable directory.
func TestPackageSummary_DocShapes_FirstSentenceOrFallback(t *testing.T) {
	documented := t.TempDir()
	writeFixture(t, documented, "a_broken.go", "package p\n\nfunc (")
	writeFixture(t, documented, "b_plain.go", "package p\n")
	writeFixture(t, documented, "c_doc_test.go", "// Package p test doc must be ignored.\npackage p\n")
	writeFixture(t, documented, "d_doc.go", "// Package p renders\n// things. It also does more.\npackage p\n")
	undocumented := t.TempDir()
	writeFixture(t, undocumented, "plain.go", "package p\n")
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "documented", dir: documented, want: "Package p renders things."},
		{name: "undocumented", dir: undocumented, want: "Package documentation unavailable."},
		{name: "missing directory", dir: filepath.Join(t.TempDir(), "absent"), want: "Package documentation unavailable."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packageSummary(tt.dir); got != tt.want {
				t.Fatalf("packageSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFirstSentence_CommentShapes_Compacts verifies newlines and runs of
// whitespace collapse and the text stops at the first sentence boundary.
func TestFirstSentence_CommentShapes_Compacts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "multi-line", in: "Package x does\n\tone   thing. Then another.", want: "Package x does one thing."},
		{name: "no boundary", in: "Package x has no period", want: "Package x has no period"},
		{name: "trailing period", in: "Package x ends.", want: "Package x ends."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstSentence(tt.in); got != tt.want {
				t.Fatalf("firstSentence(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPackageKey_PathShapes_ShortLabels verifies the table label for every
// path family the document distinguishes.
func TestPackageKey_PathShapes_ShortLabels(t *testing.T) {
	tests := []struct {
		relPath string
		want    string
	}{
		{relPath: "internal/tools/issues", want: "issues"},
		{relPath: "internal/tools", want: "tools"},
		{relPath: "internal/config", want: "config"},
		{relPath: "cmd/server", want: "cmd/server"},
		{relPath: "test/e2e/suite", want: "test/e2e/suite"},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			if got := packageKey(tt.relPath); got != tt.want {
				t.Fatalf("packageKey(%q) = %q, want %q", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestClassifyLayer_PathShapes_MapsEveryLayer verifies each path family maps
// to its layer, with an unknown prefix falling into the other bucket.
func TestClassifyLayer_PathShapes_MapsEveryLayer(t *testing.T) {
	tests := []struct {
		relPath string
		want    string
	}{
		{relPath: "internal/tools", want: layerToolsOrchestration},
		{relPath: "internal/tools/issues", want: layerToolSubpackage},
		{relPath: "internal/config", want: layerCore},
		{relPath: "cmd/server", want: layerCmd},
		{relPath: "test/e2e/suite", want: layerE2E},
		{relPath: "scripts/tool", want: layerOther},
	}
	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			if got := classifyLayer(tt.relPath); got != tt.want {
				t.Fatalf("classifyLayer(%q) = %q, want %q", tt.relPath, got, tt.want)
			}
		})
	}
}

// TestRenderTestingStats_SampleMetrics_RendersEverySection verifies the
// assembled section opens with the generated-by note, contains each heading
// in order, and ends with exactly one newline.
func TestRenderTestingStats_SampleMetrics_RendersEverySection(t *testing.T) {
	out := renderTestingStats(sampleMetrics(), 25)
	if !strings.HasPrefix(out, "## Overview\n\n> This section is generated by `go run ./cmd/gen_testing_docs/`.") {
		t.Fatalf("unexpected opening:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Fatalf("section must end with exactly one newline: %q", out[len(out)-4:])
	}
	headings := []string{
		"### Naming Convention Stats",
		"## Test Distribution",
		"### Core Packages",
		"### Tool Sub-Packages (Top Domains by Test Count)",
		"### Complete Tool Sub-Package Test Counts",
		"## Coverage Report",
		"Coverage target: **>90%** per package.",
	}
	last := 0
	// sequential: each heading must follow the previous one in the rendered text
	for _, heading := range headings {
		idx := strings.Index(out[last:], heading)
		if idx < 0 {
			t.Fatalf("heading %q missing or out of order:\n%s", heading, out)
		}
		last += idx
	}
	if !strings.Contains(out, "sample e2e note") {
		t.Fatalf("E2E note missing:\n%s", out)
	}
}

// TestRenderOverview_SampleMetrics_TotalsEachRow verifies the top-level table:
// totals across layers, the unit/E2E split, per-prefix test-file counts, the
// tested-package counts, and the three coverage aggregates.
func TestRenderOverview_SampleMetrics_TotalsEachRow(t *testing.T) {
	out := renderOverview(sampleMetrics())
	rows := []struct {
		label string
		want  string
	}{
		{label: "Total test functions", want: "1,684"},
		{label: "Unit test functions", want: "1,677"},
		{label: "E2E test functions", want: "7"},
		{label: "cmd test functions", want: "12"},
		{label: "Test files (internal/)", want: "19"},
		{label: "Test files (cmd/)", want: "3"},
		{label: "Test files (test/e2e/)", want: "5"},
		{label: "Tool sub-packages tested", want: "2"},
		{label: "Core packages tested", want: "2"},
		{label: "Overall coverage (`go test ./internal/... ./cmd/...`)", want: "90.1%"},
		{label: "Overall coverage (`go test ./internal/...`)", want: "93.4%"},
		{label: "Average package coverage", want: "88.0%"},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, out, row.label, []string{row.want})
		})
	}
}

// TestRenderNamingStats_SampleMetrics_SkipsEmptyBuckets verifies each naming
// bucket renders its count and share of the total, and a zero bucket is
// omitted rather than shown as 0.
func TestRenderNamingStats_SampleMetrics_SkipsEmptyBuckets(t *testing.T) {
	out := renderNamingStats(sampleMetrics())
	rows := []struct {
		label string
		want  []string
	}{
		{label: "`TestFunc_Scenario` (2-part)", want: []string{"500", "31.2%"}},
		{label: "`TestFunc` (no underscore)", want: []string{"100", "6.2%"}},
		{label: "`TestFunc_Scenario_Expected` (3+ part)", want: []string{"1,000", "62.3%"}},
		{label: "`TestCovFuncScenario` (coverage helper)", want: []string{"4", "0.2%"}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, out, row.label, row.want)
		})
	}
	if strings.Contains(out, "| Other") {
		t.Fatalf("zero bucket rendered:\n%s", out)
	}
}

// TestRenderDistribution_SampleMetrics_TotalsByLayer verifies the per-layer
// table carries each layer's test and file counts, the tested sub-package
// count in its label, and a bold total row.
func TestRenderDistribution_SampleMetrics_TotalsByLayer(t *testing.T) {
	out := renderDistribution(sampleMetrics())
	rows := []struct {
		label string
		want  []string
	}{
		{label: "Core packages", want: []string{"45", "3"}},
		{label: "Tools orchestration", want: []string{"100", "10"}},
		{label: "Tool sub-packages (2)", want: []string{"1,520", "6"}},
		{label: "E2E integration", want: []string{"7", "5"}},
		{label: "cmd packages", want: []string{"12", "3"}},
		{label: "**Total**", want: []string{"**1,684**", "**27**", ""}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			cells := tableRow(t, out, row.label)
			for i, want := range row.want {
				if cells[i+1] != want {
					t.Fatalf("row %s cell %d = %q, want %q", row.label, i+1, cells[i+1], want)
				}
			}
		})
	}
}

// TestRenderCorePackages_SampleMetrics_ListsTestedOnly verifies core packages
// without tests are omitted, summaries have their pipes escaped, and the
// subtotal sums the listed rows.
func TestRenderCorePackages_SampleMetrics_ListsTestedOnly(t *testing.T) {
	out := renderCorePackages(sampleMetrics())
	if strings.Contains(out, "| untested") {
		t.Fatalf("untested package rendered:\n%s", out)
	}
	if !strings.Contains(out, "Package config loads \\| settings.") {
		t.Fatalf("pipe not escaped in summary:\n%s", out)
	}
	rows := []struct {
		label string
		want  []string
	}{
		{label: "testutil", want: []string{"5", "70.0%", "Package testutil helps."}},
		{label: "**Subtotal**", want: []string{"**45**", "", ""}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, out, row.label, row.want)
		})
	}
}

// TestRenderTopToolPackages_RowCap_OrdersByTestsThenKey verifies the top
// table sorts by test count descending with the key as tie-break, and that
// the cap truncates the list.
func TestRenderTopToolPackages_RowCap_OrdersByTestsThenKey(t *testing.T) {
	metrics := sampleMetrics()
	metrics.Packages = append(metrics.Packages, packageMetrics{Key: "aardvark", Layer: layerToolSubpackage, TestFunctions: 20, TestFiles: 1, ToolCount: 1})
	tests := []struct {
		name string
		cap  int
		want []string
	}{
		{name: "uncapped", cap: 25, want: []string{"issues", "aardvark", "branches", "empty"}},
		{name: "capped", cap: 2, want: []string{"issues", "aardvark"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableKeys(renderTopToolPackages(metrics, tt.cap))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("top table keys = %v, want %v", got, tt.want)
			}
		})
	}
	assertRow(t, renderTopToolPackages(metrics, 25), "issues", []string{"1,500", "99.2%", "30"})
}

// TestRenderCompleteToolPackages_SampleMetrics_TotalsTestedRows verifies the
// collapsible table names the tested count, skips untested sub-packages, and
// totals tests, files, and tools over the listed rows.
func TestRenderCompleteToolPackages_SampleMetrics_TotalsTestedRows(t *testing.T) {
	out := renderCompleteToolPackages(sampleMetrics())
	if !strings.Contains(out, "<summary>All 2 tested sub-packages (click to expand)</summary>") {
		t.Fatalf("summary line missing:\n%s", out)
	}
	if got := tableKeys(out); strings.Join(got, ",") != "branches,issues,**Total**" {
		t.Fatalf("complete table keys = %v", got)
	}
	rows := []struct {
		label string
		want  []string
	}{
		{label: "branches", want: []string{"20", "2", "85.0%", "8"}},
		{label: "**Total**", want: []string{"**1,520**", "**6**", "", "**38**"}},
	}
	for _, row := range rows {
		t.Run(row.label, func(t *testing.T) {
			assertRow(t, out, row.label, row.want)
		})
	}
}

// TestRenderCoverageReport_SampleMetrics_SnapshotsMeasuredPackages verifies
// the snapshot lists only measured packages per layer and labels the
// orchestration package ahead of the sub-packages.
func TestRenderCoverageReport_SampleMetrics_SnapshotsMeasuredPackages(t *testing.T) {
	out := renderCoverageReport(sampleMetrics())
	if strings.Contains(out, "| cmd/gen_x") || strings.Contains(out, "| untested") || strings.Contains(out, "| empty") {
		t.Fatalf("unmeasured package rendered:\n%s", out)
	}
	if got := tableKeys(out); strings.Join(got, ",") != "cmd/server,config,testutil,tools (orch.),branches,issues" {
		t.Fatalf("coverage report keys = %v", got)
	}
	assertRow(t, out, "tools (orch.)", []string{"91.0%"})
}

// TestRenderCoverageExceptions_BelowTarget_SortsAndExplains verifies packages
// under the target are listed from lowest to highest with the rationale their
// key selects, and that an all-green snapshot renders None.
func TestRenderCoverageExceptions_BelowTarget_SortsAndExplains(t *testing.T) {
	out := renderCoverageExceptions(sampleMetrics())
	want := "- **cmd/server** (62.5%) - entry-point glue, signal handling, and transport startup are validated mostly through integration and E2E coverage.\n" +
		"- **testutil** (70.0%) - some helpers are exercised by external packages or the build-tagged E2E suite rather than this package's own tests.\n" +
		"- **branches** (85.0%) - review this package for missing unit coverage or add an explicit exception if the remaining paths are integration-only.\n"
	if !strings.HasSuffix(out, want) {
		t.Fatalf("exceptions = %q, want suffix %q", out, want)
	}
	green := renderCoverageExceptions(repositoryMetrics{Packages: []packageMetrics{{Key: "ok", Coverage: coverageValue{Percent: 95, OK: true}}}})
	if !strings.HasSuffix(green, "- None.\n") {
		t.Fatalf("all-green exceptions = %q, want None", green)
	}
}

// TestCoverageRationale_KeyFamilies_SelectsText verifies the rationale keyed
// on the package label: testutil, the server entry point, other commands, and
// everything else.
func TestCoverageRationale_KeyFamilies_SelectsText(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "testutil", want: "some helpers are exercised"},
		{key: "cmd/server", want: "entry-point glue"},
		{key: "cmd/gen_stats", want: "developer command formatting"},
		{key: "branches", want: "review this package"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := coverageRationale(packageMetrics{Key: tt.key}); !strings.HasPrefix(got, tt.want) {
				t.Fatalf("coverageRationale(%q) = %q, want prefix %q", tt.key, got, tt.want)
			}
		})
	}
}

// TestLowCoveragePackages_Ties_OrderByPercentThenKey verifies the below-target
// filter drops unmeasured and passing packages and orders ties by key.
func TestLowCoveragePackages_Ties_OrderByPercentThenKey(t *testing.T) {
	packages := []packageMetrics{
		{Key: "zeta", Coverage: coverageValue{Percent: 50, OK: true}},
		{Key: "alpha", Coverage: coverageValue{Percent: 50, OK: true}},
		{Key: "unmeasured"},
		{Key: "green", Coverage: coverageValue{Percent: 90, OK: true}},
		{Key: "lowest", Coverage: coverageValue{Percent: 10, OK: true}},
	}
	low := lowCoveragePackages(packages, 90)
	keys := make([]string, 0, len(low))
	for _, pkg := range low {
		keys = append(keys, pkg.Key)
	}
	if got := strings.Join(keys, ","); got != "lowest,alpha,zeta" {
		t.Fatalf("lowCoveragePackages() keys = %s, want lowest,alpha,zeta", got)
	}
}

// TestAverageCoverage_MeasuredSubset_AveragesUnweighted verifies the average
// spans only measured packages and is unmeasured when none are.
func TestAverageCoverage_MeasuredSubset_AveragesUnweighted(t *testing.T) {
	tests := []struct {
		name     string
		packages []packageMetrics
		want     string
	}{
		{name: "none measured", packages: []packageMetrics{{Key: "a"}, {Key: "b"}}, want: "n/a"},
		{
			name: "mixed",
			packages: []packageMetrics{
				{Key: "a", Coverage: coverageValue{Percent: 100, OK: true}},
				{Key: "b"},
				{Key: "c", Coverage: coverageValue{Percent: 50, OK: true}},
			},
			want: "75.0%",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmtCoverage(averageCoverage(tt.packages)); got != tt.want {
				t.Fatalf("averageCoverage() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestFmtInt_Magnitudes_InsertsSeparators verifies thousands separators are
// inserted only from four digits upward.
func TestFmtInt_Magnitudes_InsertsSeparators(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{in: 0, want: "0"},
		{in: 999, want: "999"},
		{in: 1000, want: "1,000"},
		{in: 1234567, want: "1,234,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := fmtInt(tt.in); got != tt.want {
				t.Fatalf("fmtInt(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFmtRatio_ZeroTotal_AvoidsDivision verifies a zero denominator renders
// 0.0% and a normal ratio renders one decimal.
func TestFmtRatio_ZeroTotal_AvoidsDivision(t *testing.T) {
	tests := []struct {
		name  string
		count int
		total int
		want  string
	}{
		{name: "zero total", count: 3, total: 0, want: "0.0%"},
		{name: "third", count: 1, total: 3, want: "33.3%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmtRatio(tt.count, tt.total); got != tt.want {
				t.Fatalf("fmtRatio(%d, %d) = %q, want %q", tt.count, tt.total, got, tt.want)
			}
		})
	}
}

// TestTailLines_LongOutput_KeepsLastLines verifies short output is returned
// whole and long output is cut to its last n lines.
func TestTailLines_LongOutput_KeepsLastLines(t *testing.T) {
	tests := []struct {
		name string
		text string
		n    int
		want string
	}{
		{name: "short", text: "a\nb\n", n: 5, want: "a\nb"},
		{name: "long", text: "a\nb\nc\nd\n", n: 2, want: "c\nd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tailLines(tt.text, tt.n); got != tt.want {
				t.Fatalf("tailLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEscapeTable_Pipe_Escaped verifies table separators inside cells are
// escaped so the row keeps its column count.
func TestEscapeTable_Pipe_Escaped(t *testing.T) {
	if got := escapeTable("a | b"); got != "a \\| b" {
		t.Fatalf("escapeTable() = %q", got)
	}
}

// sampleMetrics builds a repository snapshot spanning every layer, with and
// without coverage, so the renderers can be checked against known totals.
func sampleMetrics() repositoryMetrics {
	cov := func(percent float64) coverageValue { return coverageValue{Percent: percent, OK: true} }
	return repositoryMetrics{
		Packages: []packageMetrics{
			{RelPath: "cmd/server", Key: "cmd/server", Layer: layerCmd, TestFunctions: 12, TestFiles: 3, Coverage: cov(62.5), Summary: "Command server."},
			{RelPath: "cmd/gen_x", Key: "cmd/gen_x", Layer: layerCmd},
			{RelPath: "internal/config", Key: "config", Layer: layerCore, TestFunctions: 40, TestFiles: 2, Coverage: cov(95.5), Summary: "Package config loads | settings."},
			{RelPath: "internal/testutil", Key: "testutil", Layer: layerCore, TestFunctions: 5, TestFiles: 1, Coverage: cov(70), Summary: "Package testutil helps."},
			{RelPath: "internal/untested", Key: "untested", Layer: layerCore, Summary: "Package untested."},
			{RelPath: "internal/tools", Key: "tools", Layer: layerToolsOrchestration, TestFunctions: 100, TestFiles: 10, Coverage: cov(91)},
			{RelPath: "internal/tools/issues", Key: "issues", Layer: layerToolSubpackage, TestFunctions: 1500, TestFiles: 4, ToolCount: 30, Coverage: cov(99.2)},
			{RelPath: "internal/tools/branches", Key: "branches", Layer: layerToolSubpackage, TestFunctions: 20, TestFiles: 2, ToolCount: 8, Coverage: cov(85)},
			{RelPath: "internal/tools/empty", Key: "empty", Layer: layerToolSubpackage, ToolCount: 2},
			{RelPath: "test/e2e/suite", Key: "test/e2e/suite", Layer: layerE2E, TestFunctions: 7, TestFiles: 5},
		},
		NamingCounts:           map[string]int{pattern3Part: 1000, pattern2Part: 500, patternNoUnderscore: 100, patternTestCov: 4, patternOther: 0},
		OverallCoverage:        cov(90.1),
		InternalCoverage:       cov(93.4),
		AveragePackageCoverage: cov(88),
		E2ENote:                "sample e2e note",
	}
}

// fakeModuleFiles returns a self-contained module with one package per layer
// the document distinguishes. Every file compiles, so the same tree serves the
// count-only collectors and a real go test coverage run: core covers one of
// its two statements, issues covers its only one, widgets covers two of three,
// and cmd/app covers none of its one.
func fakeModuleFiles() map[string]string {
	return map[string]string{
		"go.mod":          fakeGoMod(),
		"docs/testing.md": legacyDoc,
		"cmd/app/main.go": "// Package main is the fake entry point.\npackage main\n\nfunc main() {\n\tprintln(\"app\")\n}\n",
		"cmd/app/main_test.go": "package main\n\nimport \"testing\"\n\n" +
			"func TestApp_Start_Succeeds(t *testing.T) {}\n",
		"internal/core/core.go": "// Package core adds numbers. It also subtracts them.\npackage core\n\n" +
			"// Add adds.\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\n" +
			"// Sub subtracts.\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n",
		"internal/core/core_test.go": "package core\n\nimport \"testing\"\n\n" +
			"func TestAdd_TwoAndTwo_ReturnsFour(t *testing.T) {\n\tif Add(2, 2) != 4 {\n\t\tt.Fatal(\"bad sum\")\n\t}\n}\n\n" +
			"func TestAdd_Zero(t *testing.T) {}\n\n" +
			"func TestCovAddBranch(t *testing.T) {}\n",
		"internal/tools/tools.go": "// Package tools orchestrates the fake catalog.\npackage tools\n",
		"internal/tools/tools_test.go": "package tools\n\nimport \"testing\"\n\n" +
			"func TestCatalog_Build_Succeeds(t *testing.T) {}\n",
		"internal/tools/issues/issues.go": "// Package issues mirrors a real catalog owner.\npackage issues\n\n" +
			"// ActionSpec is a fake catalog entry.\ntype ActionSpec struct{ Name string }\n\n" +
			"// ActionSpecs returns the fake issue actions.\nfunc ActionSpecs() []ActionSpec {\n\treturn []ActionSpec{{Name: \"list\"}}\n}\n",
		"internal/tools/issues/issues_test.go": "package issues\n\nimport \"testing\"\n\n" +
			"func TestActionSpecs_Default_ReturnsOne(t *testing.T) {\n\tif len(ActionSpecs()) != 1 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n",
		"internal/tools/widgets/widgets.go": "package widgets\n\n" +
			"// ActionSpec is a fake catalog entry.\ntype ActionSpec struct{ Name string }\n\n" +
			"type registry struct{}\n\n" +
			"func (registry) AddTool(id int) {}\n\n" +
			"var mcp registry\n\n" +
			"// Register mimics the legacy AddTool registration.\nfunc Register() {\n\tmcp.AddTool(1)\n\tmcp.AddTool(2)\n}\n\n" +
			"// ActionSpecs returns the fake widget actions.\nfunc ActionSpecs() []ActionSpec {\n\treturn []ActionSpec{{Name: \"list\"}, {Name: \"get\"}, {Name: \"delete\"}}\n}\n",
		"internal/tools/widgets/widgets_test.go": "package widgets\n\nimport \"testing\"\n\n" +
			"func TestRegister_Default_AddsTools(t *testing.T) {\n\tRegister()\n}\n",
		"test/e2e/suite/flow_test.go": "//go:build e2e\n\npackage suite\n\nimport \"testing\"\n\n" +
			"func TestFullWorkflow(t *testing.T) {}\n\n" +
			"func TestMetaToolWorkflow(t *testing.T) {}\n",
	}
}

// fakeGoMod returns a go.mod whose go directive matches the toolchain running
// the tests, so the GOTOOLCHAIN pin the generator derives from it resolves to
// this installation rather than to a download.
func fakeGoMod() string {
	version := strings.TrimPrefix(runtime.Version(), "go")
	if !goReleaseVersionRE.MatchString(version) {
		return "module example.com/fake\n"
	}
	return "module example.com/fake\n\ngo " + version + "\n"
}

// writeFakeModule writes files (slash-separated paths relative to a fresh
// temporary directory) and isolates the child Go commands from workspace and
// flag settings inherited from the developer's environment.
func writeFakeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	root := t.TempDir()
	for rel, content := range files {
		writeFixture(t, root, filepath.FromSlash(rel), content)
	}
	return root
}

// writeFixture writes one file under dir, creating parent directories.
func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// tableRow returns the trimmed cells of the first Markdown table row whose
// first cell equals label, failing the test when no such row exists.
func tableRow(t *testing.T, rendered, label string) []string {
	t.Helper()
	for line := range strings.Lines(rendered) {
		cells := splitTableRow(line)
		if len(cells) > 0 && cells[0] == label {
			return cells
		}
	}
	t.Fatalf("row %q not found in:\n%s", label, rendered)
	return nil
}

// assertRow checks the cells after the label of the named table row.
func assertRow(t *testing.T, rendered, label string, want []string) {
	t.Helper()
	cells := tableRow(t, rendered, label)
	got := cells[1:]
	if len(got) != len(want) {
		t.Fatalf("row %q has cells %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %q cell %d = %q, want %q (row %v)", label, i+1, got[i], want[i], got)
		}
	}
}

// tableKeys lists the first cell of every body row across the Markdown tables
// in rendered, skipping header and separator rows.
func tableKeys(rendered string) []string {
	keys := []string{}
	for line := range strings.Lines(rendered) {
		cells := splitTableRow(line)
		if len(cells) == 0 || cells[0] == "" || strings.HasPrefix(cells[0], "-") || strings.HasPrefix(cells[0], ":") {
			continue
		}
		if cells[0] == "Package" || cells[0] == "Sub-package" {
			continue
		}
		keys = append(keys, cells[0])
	}
	return keys
}

// splitTableRow splits one Markdown table line into trimmed cells, or returns
// nil when the line is not a table row.
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}
