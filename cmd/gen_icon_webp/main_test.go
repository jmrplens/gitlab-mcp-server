// main_test.go covers icon-asset generation: extracting svg<Name> constants
// from icons.go's AST, naming their WebP files, and the check/generate
// pipeline that reads or writes them. Every test here runs without
// rsvg-convert/cwebp on PATH by injecting a fake rasterizer — production
// code's real rasterize (the only function that shells out) is exercised by
// a single test gated on those tools actually being present, since CI does
// not install them (this generator is a maintainer-only, occasional step;
// its output is committed).
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeRasterizer returns deterministic, cheap-to-compare bytes for (svg,
// color) without touching rsvg-convert/cwebp, so checkAll/generateAll/runIn
// can be tested in pure Go.
func fakeRasterizer(svg, color string) ([]byte, error) {
	return []byte(svg + "|" + color), nil
}

// failingRasterizer always errors, for exercising error propagation.
func failingRasterizer(_, _ string) ([]byte, error) {
	return nil, errors.New("boom")
}

const fixtureIconsGo = `package toolutil

const (
	svgBranch = ` + "`<svg>branch</svg>`" + `
	svgMR     = ` + "`<svg>mr</svg>`" + `
)

// svgMIME is not SVG markup, so it must be skipped even though its name
// starts with "svg".
const svgMIME = "image/svg+xml"

// notPrefixed starts with "<svg" but its identifier does not start with
// "svg", so it must be skipped too.
const notPrefixed = ` + "`<svg>ignored</svg>`" + `

const (
	svgWeird = 42
)

// svgComposed holds SVG markup, but as a concatenation rather than a single
// literal, so the AST hands over a binary expression and it must be skipped.
const svgComposed = ` + "`<svg>com`" + ` + ` + "`posed</svg>`" + `

const (
	svgInherited = ` + "`<svg>first</svg>`" + `
	svgInheritedTwo
)

var IconBranch = icon("branch", svgBranch)

func helper() string { return "not a const decl" }
`

const fixtureEmptyIconsGo = `package toolutil

const notAnIcon = "hello"
`

// fixtureBrandGo stands in for the generated brand mark, the second source the
// command reads, so a test can drive both without the real repository.
const fixtureBrandGo = `package toolutil

const svgBrand = ` + "`<svg>brand</svg>`" + `
`

// writeFixture writes content to name inside a fresh temp directory and
// returns that directory.
func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

// --- iconFileName ---

func TestIconFileName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Branch", "branch"},
		{"MR", "mr"},
		{"MergeRequest", "mergerequest"},
		{"Vulnerability2", "vulnerability2"},
		{"Merge_Request-2", "mergerequest2"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := iconFileName(tt.in); got != tt.want {
				t.Errorf("iconFileName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- extractIcons ---

func TestExtractIcons_FindsSVGConstantsInDeclarationOrder(t *testing.T) {
	dir := writeFixture(t, "icons.go", fixtureIconsGo)

	icons, err := extractIcons(filepath.Join(dir, "icons.go"))
	if err != nil {
		t.Fatalf("extractIcons() error: %v", err)
	}

	// svgMIME (wrong content), notPrefixed (wrong name), svgWeird (not a
	// string literal), svgComposed (a concatenation rather than one literal)
	// and svgInheritedTwo (no value of its own) must all be excluded; only
	// svgBranch, svgMR, and svgInherited qualify.
	var names []string
	for _, ic := range icons {
		names = append(names, ic.name)
	}
	want := []string{"branch", "mr", "inherited"}
	if len(names) != len(want) {
		t.Fatalf("extractIcons() names = %v, want %v", names, want)
	}
	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if names[i] != name {
				t.Fatalf("extractIcons() names = %v, want %v", names, want)
			}
		})
	}
	if icons[0].svg != "<svg>branch</svg>" {
		t.Errorf("icons[0].svg = %q, want %q", icons[0].svg, "<svg>branch</svg>")
	}
}

func TestExtractIcons_NoIconConstants(t *testing.T) {
	dir := writeFixture(t, "icons.go", fixtureEmptyIconsGo)

	icons, err := extractIcons(filepath.Join(dir, "icons.go"))
	if err != nil {
		t.Fatalf("extractIcons() error: %v", err)
	}
	if len(icons) != 0 {
		t.Fatalf("extractIcons() = %v, want empty", icons)
	}
}

func TestExtractIcons_MalformedSource(t *testing.T) {
	dir := writeFixture(t, "icons.go", "package toolutil\n\nconst svgBroken = `<svg>\n")

	if _, err := extractIcons(filepath.Join(dir, "icons.go")); err == nil {
		t.Fatal("extractIcons() error = nil, want a parse error for malformed Go source")
	}
}

func TestExtractIcons_MissingFile(t *testing.T) {
	if _, err := extractIcons(filepath.Join(t.TempDir(), "does-not-exist.go")); err == nil {
		t.Fatal("extractIcons() error = nil, want an error for a missing file")
	}
}

// TestValueSpecIcons_NotAValueSpec_IsSkipped pins the type guard its own
// caller makes unreachable: constDeclIcons hands over the specs of a const
// block, which are always value specs, so only a direct call can establish
// that another kind of spec is skipped rather than panicking on the assertion.
func TestValueSpecIcons_NotAValueSpec_IsSkipped(t *testing.T) {
	if icons := valueSpecIcons(&ast.ImportSpec{}); icons != nil {
		t.Errorf("valueSpecIcons(*ast.ImportSpec) = %v, want nil", icons)
	}
}

// strLit builds the AST node go/parser produces for a string constant's value,
// so a test can hand svgConstIcon a literal no valid source file could carry.
func strLit(text string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: text}
}

// TestSvgConstIcon_AcceptsOnlyAQuotedSVGLiteral states the whole per-constant
// filter in one table, and asserts the resolved iconSource as a whole, so a
// name and a markup body that changed places would fail rather than pass on
// two separate checks. The unquotable literal is the case no fixture can
// reach: go/parser accepts only literals strconv.Unquote can read back, so
// that arm is reachable from a synthetic node alone.
func TestSvgConstIcon_AcceptsOnlyAQuotedSVGLiteral(t *testing.T) {
	tests := []struct {
		name      string
		ident     string
		expr      ast.Expr
		want      iconSource
		wantFound bool
	}{
		{
			name:      "an svg-prefixed name holding SVG markup",
			ident:     "svgMergeRequest",
			expr:      strLit("`<svg>mr</svg>`"),
			want:      iconSource{name: "mergerequest", svg: "<svg>mr</svg>"},
			wantFound: true,
		},
		{
			name:  "a concatenation rather than one literal",
			ident: "svgComposed",
			expr:  &ast.BinaryExpr{X: strLit("`<svg>com`"), Op: token.ADD, Y: strLit("`posed</svg>`")},
		},
		{name: "a literal that is not a string", ident: "svgWeird", expr: &ast.BasicLit{Kind: token.INT, Value: "42"}},
		{name: "a literal whose text is not a quoted string", ident: "svgBroken", expr: strLit("`<svg>unterminated")},
		{name: "a name that does not begin with svg", ident: "notPrefixed", expr: strLit("`<svg>ignored</svg>`")},
		{name: "a quoted string that is not SVG markup", ident: "svgMIME", expr: strLit(`"image/svg+xml"`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := svgConstIcon(tt.ident, tt.expr)
			if found != tt.wantFound {
				t.Fatalf("svgConstIcon(%q) found = %v, want %v", tt.ident, found, tt.wantFound)
			}
			if got != tt.want {
				t.Errorf("svgConstIcon(%q) = %+v, want %+v", tt.ident, got, tt.want)
			}
		})
	}
}

// --- requireTools ---

func TestRequireTools_AllPresent(t *testing.T) {
	if err := requireTools(); err != nil {
		t.Fatalf("requireTools() error = %v, want nil for an empty tool list", err)
	}
}

func TestRequireTools_ReportsMissingByName(t *testing.T) {
	err := requireTools("definitely-not-a-real-tool-gitlab-mcp-server-xyz")
	if err == nil {
		t.Fatal("requireTools() error = nil, want an error naming the missing tool")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-real-tool-gitlab-mcp-server-xyz") {
		t.Errorf("requireTools() error = %v, want it to name the missing tool", err)
	}
}

// TestRequireTools_NamesOnlyWhatIsMissing drives the resolved half of the
// lookup, which nothing else does: asked about a tool it finds on PATH and one
// it does not, the refusal names the absent one alone, so a caller is told
// what to install rather than what it already has.
func TestRequireTools_NamesOnlyWhatIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tool is a POSIX shell script")
	}
	bin := t.TempDir()
	writeFakeTool(t, bin, "present-tool", "exit 0\n")
	t.Setenv("PATH", bin)

	if err := requireTools("present-tool"); err != nil {
		t.Fatalf("requireTools(\"present-tool\") error = %v, want nil for a tool on PATH", err)
	}
	err := requireTools("present-tool", "absent-tool")
	if err == nil {
		t.Fatal("requireTools() error = nil, want an error for the tool that is not on PATH")
	}
	if !strings.Contains(err.Error(), "absent-tool") {
		t.Errorf("requireTools() error = %v, want it to name the absent tool", err)
	}
	if strings.Contains(err.Error(), "present-tool") {
		t.Errorf("requireTools() error = %v, want it to leave out the tool it resolved", err)
	}
}

// --- repoRoot ---

func TestRepoRoot_FindsAncestorGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	t.Chdir(nested)

	got, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot() error: %v", err)
	}
	// Resolve both sides through EvalSymlinks: on macOS, t.TempDir() lives
	// under /tmp, which is itself a symlink to /private/tmp.
	wantResolved, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved {
		t.Errorf("repoRoot() = %q, want %q", gotResolved, wantResolved)
	}
}

func TestRepoRoot_NoGoModAnywhereAbove(t *testing.T) {
	// A fresh temp dir has no go.mod above it up to the filesystem root.
	t.Chdir(t.TempDir())

	if _, err := repoRoot(); err == nil {
		t.Fatal("repoRoot() error = nil, want an error when no go.mod is found")
	}
}

// --- variants ---

// hexBrightness reads a "#RRGGBB" color as the integer its digits spell, so
// the two theme colors can be compared without this test repeating either
// value and passing on its own copy.
func hexBrightness(t *testing.T, color string) uint64 {
	t.Helper()
	value, err := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	if err != nil {
		t.Fatalf("parse color %q: %v", color, err)
	}
	return value
}

// TestVariants_TheLightThemeGlyphIsTheDarkerOne pins which color belongs to
// which suffix, which nothing else states as a property: Icon.Theme "light"
// names a light background, so its glyph has to be the darker of the pair, and
// the two constants trading places would otherwise only change bytes no
// assertion reads.
func TestVariants_TheLightThemeGlyphIsTheDarkerOne(t *testing.T) {
	vs := variants()

	if len(vs) != 2 {
		t.Fatalf("variants() = %+v, want one light and one dark variant", vs)
	}
	if vs[0].suffix != "-light" || vs[1].suffix != "-dark" {
		t.Fatalf("variants() suffixes = %q, %q, want %q, %q", vs[0].suffix, vs[1].suffix, "-light", "-dark")
	}
	if light, dark := hexBrightness(t, vs[0].color), hexBrightness(t, vs[1].color); light >= dark {
		t.Errorf("variants(): the light-theme color %s is not darker than the dark-theme color %s", vs[0].color, vs[1].color)
	}
}

// --- generateAll ---

func TestGenerateAll_WritesEveryVariant(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "webp")
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}, {name: "mr", svg: "<svg>mr</svg>"}}

	written, err := generateAll(dir, icons, fakeRasterizer)
	if err != nil {
		t.Fatalf("generateAll() error: %v", err)
	}
	if written != 4 {
		t.Fatalf("generateAll() written = %d, want 4 (2 icons x 2 variants)", written)
	}

	for _, name := range []string{"branch-light.webp", "branch-dark.webp", "mr-light.webp", "mr-dark.webp"} {
		t.Run(name, func(t *testing.T) {
			data, readErr := os.ReadFile(filepath.Join(dir, name))
			if readErr != nil {
				t.Fatalf("expected %s to exist: %v", name, readErr)
			}
			if len(data) == 0 {
				t.Errorf("%s is empty", name)
			}
		})
	}
	// Both files are read, because each names the color the other must not
	// carry: with only one asserted, a variant table whose two colors had
	// traded places would still be caught, but one whose second entry
	// repeated the first would not.
	branchLight, _ := os.ReadFile(filepath.Join(dir, "branch-light.webp"))
	if string(branchLight) != "<svg>branch</svg>|"+colorLight {
		t.Errorf("branch-light.webp = %q, want the fake rasterizer's deterministic output", branchLight)
	}
	branchDark, _ := os.ReadFile(filepath.Join(dir, "branch-dark.webp"))
	if string(branchDark) != "<svg>branch</svg>|"+colorDark {
		t.Errorf("branch-dark.webp = %q, want the dark variant's color", branchDark)
	}
}

func TestGenerateAll_PropagatesRasterizerError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "webp")
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}

	_, err := generateAll(dir, icons, failingRasterizer)
	if err == nil {
		t.Fatal("generateAll() error = nil, want the rasterizer's error propagated")
	}
	if !strings.Contains(err.Error(), "branch") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("generateAll() error = %v, want it to name the icon and the underlying error", err)
	}
}

func TestGenerateAll_MkdirFailsWhenParentIsAFile(t *testing.T) {
	// A regular file where a directory component is expected makes
	// MkdirAll fail deterministically, regardless of OS/permissions.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	dir := filepath.Join(blocker, "webp")
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}

	if _, err := generateAll(dir, icons, fakeRasterizer); err == nil {
		t.Fatal("generateAll() error = nil, want an error when the output directory cannot be created")
	}
}

func TestGenerateAll_WriteFileFailsWhenTargetIsADirectory(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a directory at the exact path generateAll needs to write
	// a file to, so os.WriteFile fails.
	if err := os.MkdirAll(filepath.Join(dir, "branch-light.webp"), 0o750); err != nil {
		t.Fatalf("mkdir collision path: %v", err)
	}
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}

	written, err := generateAll(dir, icons, fakeRasterizer)
	if err == nil {
		t.Fatal("generateAll() error = nil, want an error when a target path is a directory")
	}
	if written != 0 {
		t.Errorf("generateAll() written = %d, want 0 since the first write failed", written)
	}
}

// --- checkAll ---

func TestCheckAll_MatchesUpToDateAssets(t *testing.T) {
	dir := t.TempDir()
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}
	if _, err := generateAll(dir, icons, fakeRasterizer); err != nil {
		t.Fatalf("seed generateAll() error: %v", err)
	}

	if err := checkAll(dir, icons, fakeRasterizer); err != nil {
		t.Fatalf("checkAll() error = %v, want nil for freshly generated assets", err)
	}
}

func TestCheckAll_ReportsMissingAsset(t *testing.T) {
	dir := t.TempDir()
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}

	err := checkAll(dir, icons, fakeRasterizer)
	if err == nil {
		t.Fatal("checkAll() error = nil, want an error for missing assets")
	}
	if !strings.Contains(err.Error(), "branch-dark.webp") {
		t.Errorf("checkAll() error = %v, want it to name the missing file", err)
	}
}

func TestCheckAll_ReportsStaleAsset(t *testing.T) {
	dir := t.TempDir()
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}
	if _, err := generateAll(dir, icons, fakeRasterizer); err != nil {
		t.Fatalf("seed generateAll() error: %v", err)
	}
	// Edit the source SVG so the committed asset no longer matches.
	icons[0].svg = "<svg>changed</svg>"

	err := checkAll(dir, icons, fakeRasterizer)
	if err == nil {
		t.Fatal("checkAll() error = nil, want an error for a stale asset")
	}
	if !strings.Contains(err.Error(), "branch-light.webp") {
		t.Errorf("checkAll() error = %v, want it to name the stale file", err)
	}
}

// TestCheckAll_NamesEveryStaleFileInOneSortedList verifies the refusal a
// maintainer actually reads: every file that has to be regenerated is named,
// in one sorted list, rather than in whatever order the icons happen to be
// declared in.
func TestCheckAll_NamesEveryStaleFileInOneSortedList(t *testing.T) {
	icons := []iconSource{{name: "mr", svg: "<svg>mr</svg>"}, {name: "branch", svg: "<svg>branch</svg>"}}

	err := checkAll(t.TempDir(), icons, fakeRasterizer)
	if err == nil {
		t.Fatal("checkAll() error = nil, want an error for missing assets")
	}
	const want = "branch-dark.webp, branch-light.webp, mr-dark.webp, mr-light.webp"
	if !strings.HasSuffix(err.Error(), want) {
		t.Errorf("checkAll() error = %v, want it to end with the sorted list %q", err, want)
	}
}

func TestCheckAll_PropagatesRasterizerError(t *testing.T) {
	dir := t.TempDir()
	icons := []iconSource{{name: "branch", svg: "<svg>branch</svg>"}}

	err := checkAll(dir, icons, failingRasterizer)
	if err == nil {
		t.Fatal("checkAll() error = nil, want the rasterizer's error propagated")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("checkAll() error = %v, want the underlying rasterizer error", err)
	}
}

// --- runIn ---

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written to it. os.Stdout is the one standard stream `go test -json`
// leaves alone, so this reads the same file the command writes to under CI.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close the write end: %v", closeErr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read the captured output: %v", err)
	}
	return string(out)
}

// TestRunIn_SummarisesWhatItWroteAndWhatItChecked reads the two lines the
// command prints, which are the whole report a maintainer running it gets and
// which nothing else asserts. The fixture's icon count and file count differ,
// so neither line can have its two figures trade places and stay green.
func TestRunIn_SummarisesWhatItWroteAndWhatItChecked(t *testing.T) {
	root := writeFixture(t, "icons.go", fixtureIconsGo) // three icons, six files
	var genErr, checkErr error

	generated := captureStdout(t, func() {
		genErr = runIn(root, []string{"icons.go"}, "webp", false, fakeRasterizer)
	})
	checked := captureStdout(t, func() {
		checkErr = runIn(root, []string{"icons.go"}, "webp", true, fakeRasterizer)
	})

	if genErr != nil || checkErr != nil {
		t.Fatalf("runIn() errors: generate %v, check %v", genErr, checkErr)
	}
	if want := "wrote 6 webp files for 3 icons into webp\n"; generated != want {
		t.Errorf("runIn(generate) printed %q, want %q", generated, want)
	}
	if want := "icon webp assets are up to date (3 icons, 6 files)\n"; checked != want {
		t.Errorf("runIn(check) printed %q, want %q", checked, want)
	}
}

func TestRunIn_GenerateThenCheckRoundTrips(t *testing.T) {
	root := writeFixture(t, "icons.go", fixtureIconsGo)

	if err := runIn(root, []string{"icons.go"}, "webp", false, fakeRasterizer); err != nil {
		t.Fatalf("runIn(generate) error: %v", err)
	}
	if err := runIn(root, []string{"icons.go"}, "webp", true, fakeRasterizer); err != nil {
		t.Fatalf("runIn(check) error = %v, want nil right after generating", err)
	}
}

func TestRunIn_PropagatesExtractIconsError(t *testing.T) {
	root := writeFixture(t, "icons.go", "package toolutil\n\nconst svgBroken = `<svg>\n")

	if err := runIn(root, []string{"icons.go"}, "webp", true, fakeRasterizer); err == nil {
		t.Fatal("runIn() error = nil, want extractIcons()'s parse error propagated")
	}
}

func TestRunIn_NoIconsFoundIsAnError(t *testing.T) {
	root := writeFixture(t, "icons.go", fixtureEmptyIconsGo)

	err := runIn(root, []string{"icons.go"}, "webp", true, fakeRasterizer)
	if err == nil {
		t.Fatal("runIn() error = nil, want an error when icons.go declares no svg<Name> constants")
	}
	if !strings.Contains(err.Error(), "icons.go") {
		t.Errorf("runIn() error = %v, want it to name the source file", err)
	}
}

func TestRunIn_CheckFailsBeforeGenerate(t *testing.T) {
	root := writeFixture(t, "icons.go", fixtureIconsGo)

	if err := runIn(root, []string{"icons.go"}, "webp", true, fakeRasterizer); err == nil {
		t.Fatal("runIn(check) error = nil, want an error when no assets have been generated yet")
	}
}

func TestRunIn_GeneratePropagatesRasterizerError(t *testing.T) {
	root := writeFixture(t, "icons.go", fixtureIconsGo)

	if err := runIn(root, []string{"icons.go"}, "webp", false, failingRasterizer); err == nil {
		t.Fatal("runIn(generate) error = nil, want the rasterizer's error propagated")
	}
}

// --- run ---

// writeUnder writes content at path, taken relative to root and spelled with
// forward slashes, creating the directories above it.
func writeUnder(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeFakeRepo lays out a repository root holding the two icon sources the
// command really reads, and returns it.
//
// The three paths below are spelled out rather than taken from sourceFile,
// brandFile and outDir: a fixture written at the constant the assertion then
// reads moves with it, so it would hold nothing at all. Spelled, they say
// where the icons live, and a deliberate move of one is answered by editing
// this line.
func writeFakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	writeUnder(t, root, "internal/toolutil/icons.go", fixtureIconsGo)
	writeUnder(t, root, "internal/toolutil/brandmark_gen.go", fixtureBrandGo)
	return root
}

// generatedAssetDir is where the command writes, as a reader of the
// repository sees it, for the same reason writeFakeRepo spells its sources.
const generatedAssetDir = "internal/toolutil/icons/webp"

// TestRun_ReadsBothIconSourcesRelativeToTheRepositoryRoot drives run's success
// path, which no other test reaches. The three paths it resolves are constants
// nothing else reads, so icons.go, the generated brand mark beside it and the
// output directory would each be free to move without a test noticing until a
// maintainer ran the command; the brand mark matters most, since it is the one
// source a reader would not think to look for.
func TestRun_ReadsBothIconSourcesRelativeToTheRepositoryRoot(t *testing.T) {
	root := writeFakeRepo(t)
	nested := filepath.Join(root, "cmd", "gen_icon_webp")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	t.Chdir(nested)

	out := captureStdout(t, func() {
		if err := run(false, fakeRasterizer); err != nil {
			t.Errorf("run() error: %v", err)
		}
	})

	for _, name := range []string{"branch-light.webp", "mr-dark.webp", "inherited-light.webp", "brand-dark.webp"} {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(generatedAssetDir), name)); err != nil {
				t.Errorf("expected %s under %s: %v", name, generatedAssetDir, err)
			}
		})
	}
	if want := "wrote 8 webp files for 4 icons into " + generatedAssetDir + "\n"; out != want {
		t.Errorf("run() printed %q, want %q", out, want)
	}
}

func TestRun_PropagatesRepoRootError(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := run(true, fakeRasterizer); err == nil {
		t.Fatal("run() error = nil, want repoRoot()'s error propagated when no go.mod is found")
	}
}

// --- rasterize integration ---

// TestRasterize_ToolNotOnPath exercises rasterize's exec.LookPath failure
// branch deterministically, without needing rsvg-convert/cwebp installed:
// an empty PATH can never resolve either one.
func TestRasterize_ToolNotOnPath(t *testing.T) {
	t.Setenv("PATH", "")

	if _, err := rasterize("<svg></svg>", colorLight); err == nil {
		t.Fatal("rasterize() error = nil, want an error when rsvg-convert cannot be resolved on PATH")
	}
}

// TestRasterize_InvalidSVGFailsAtRsvgConvert exercises rasterize's
// rsvg-convert error branch. Skipped without the tool for the same reason
// as TestRun_CheckModeAcceptsCommittedAssets.
func TestRasterize_InvalidSVGFailsAtRsvgConvert(t *testing.T) {
	if err := requireTools("rsvg-convert"); err != nil {
		t.Skip("skipping: " + err.Error())
	}

	if _, err := rasterize("not valid currentColor svg markup", colorLight); err == nil {
		t.Fatal("rasterize() error = nil, want rsvg-convert to reject non-SVG input")
	}
}

// TestRun_CheckModeAcceptsCommittedAssets verifies the real, committed WebP
// assets under internal/toolutil/icons/webp/ still match icons.go, using the
// real rasterize (rsvg-convert + cwebp) rather than a fake. This is the same
// gate `make check-icon-webp` and CI would run if those tools were
// installed; it is skipped here because they are a maintainer-only,
// non-CI dependency (see the package doc comment).
func TestRun_CheckModeAcceptsCommittedAssets(t *testing.T) {
	if err := requireTools("rsvg-convert", "cwebp"); err != nil {
		t.Skip("skipping: " + err.Error())
	}

	if err := run(true, rasterize); err != nil {
		t.Fatalf("run(true) error: %v", err)
	}
}

// writeFakeTool writes an executable shell script named name into dir whose
// body is script, so a test can stand in for rsvg-convert or cwebp on PATH
// without the real tool.
func writeFakeTool(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // the fake tool must be executable
		t.Fatalf("write fake %s: %v", name, err)
	}
}

// TestRasterize_CwebpUnusable_ReturnsCwebpError exercises the two cwebp
// branches of rasterize by putting a fake rsvg-convert that succeeds on a
// private PATH: with no cwebp beside it the lookup fails, and with a cwebp
// that exits non-zero the run fails and carries the tool's stderr. Neither
// case needs the real tools installed.
func TestRasterize_CwebpUnusable_ReturnsCwebpError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are POSIX shell scripts")
	}
	tests := []struct {
		name    string
		cwebp   string
		wantErr []string
	}{
		{name: "cwebp not on PATH", cwebp: "", wantErr: []string{"cwebp", "not found"}},
		{name: "cwebp exits non-zero", cwebp: "echo 'bad png' >&2\nexit 3\n", wantErr: []string{"cwebp: exit status 3", "bad png"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			writeFakeTool(t, bin, "rsvg-convert", "cat >/dev/null\nexit 0\n")
			if tt.cwebp != "" {
				writeFakeTool(t, bin, "cwebp", tt.cwebp)
			}
			t.Setenv("PATH", bin)

			_, err := rasterize("<svg fill=\"currentColor\"></svg>", colorLight)
			if err == nil {
				t.Fatal("rasterize() error = nil, want a cwebp failure")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("rasterize() error = %v, want containing %q", err, want)
				}
			}
		})
	}
}

// TestRepoRoot_RemovedWorkingDirectory_ReturnsError verifies repoRoot reports
// the os.Getwd failure when the process's working directory was removed:
// getcwd(3) then fails with ENOENT regardless of privilege, which makes the
// branch reproducible without depending on permission enforcement.
func TestRepoRoot_RemovedWorkingDirectory_ReturnsError(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(gone)
	// Windows refuses to remove a process's working directory, and macOS
	// keeps answering getcwd from the path it remembers, so on neither can
	// the failure be produced this way: both skip rather than report the
	// operating system's design as a defect here.
	if err := os.RemoveAll(gone); err != nil {
		t.Skipf("this platform will not remove the working directory: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed")
	}

	if _, err := repoRoot(); err == nil || strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("repoRoot() error = %v, want the working-directory error, not a go.mod walk result", err)
	}
}

// recordingTools puts fake rsvg-convert and cwebp scripts on a private PATH
// and returns the directory each one writes its arguments and its standard
// input into, so what rasterize sends them can be read back without librsvg or
// libwebp installed.
func recordingTools(t *testing.T) string {
	t.Helper()
	bin, rec := t.TempDir(), t.TempDir()
	record := func(tool, output string) string {
		return fmt.Sprintf("printf '%%s\\n' \"$@\" >'%s'\ncat >'%s'\nprintf '%s'\n",
			filepath.Join(rec, tool+".args"), filepath.Join(rec, tool+".stdin"), output)
	}
	writeFakeTool(t, bin, "rsvg-convert", record("rsvg", "FAKEPNG"))
	writeFakeTool(t, bin, "cwebp", record("cwebp", "FAKEWEBP"))
	// Prepended rather than substituted: the scripts read their standard
	// input with cat, which a PATH holding nothing but this directory could
	// not resolve, and the redirection would then leave an empty file behind
	// rather than fail. The fakes still win, being first.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rec
}

// readRecord reads back one of the files the fake tools wrote.
func readRecord(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestRasterize_SendsEachToolWhatItsOutputDependsOn is the only test that says
// what the two commands are actually run with, and every value here decides
// what a committed icon looks like: the glyph color substituted for every
// occurrence of currentColor, the 16x16 the raster is sized to, and cwebp's
// -lossless, without which each icon would be a lossy approximation of itself.
// It pins the direction of the pipe too, since it is rsvg-convert's PNG
// reaching cwebp's standard input that makes the second stage encode the
// first's output rather than the markup.
func TestRasterize_SendsEachToolWhatItsOutputDependsOn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are POSIX shell scripts")
	}
	rec := recordingTools(t)

	webp, err := rasterize(`<svg fill="currentColor"><path stroke="currentColor"/></svg>`, colorDark)
	if err != nil {
		t.Fatalf("rasterize() error: %v", err)
	}

	if string(webp) != "FAKEWEBP" {
		t.Errorf("rasterize() = %q, want what cwebp wrote to standard output", webp)
	}
	if got, want := readRecord(t, rec, "rsvg.stdin"), `<svg fill="`+colorDark+`"><path stroke="`+colorDark+`"/></svg>`; got != want {
		t.Errorf("rsvg-convert read %q, want %q", got, want)
	}
	if got, want := readRecord(t, rec, "rsvg.args"), "-w\n16\n-h\n16\n--format=png\n"; got != want {
		t.Errorf("rsvg-convert ran with %q, want %q", got, want)
	}
	if got := readRecord(t, rec, "cwebp.stdin"); got != "FAKEPNG" {
		t.Errorf("cwebp read %q, want the PNG rsvg-convert wrote", got)
	}
	if got, want := readRecord(t, rec, "cwebp.args"), "-lossless\n-z\n9\n-quiet\n-o\n-\n--\n-\n"; got != want {
		t.Errorf("cwebp ran with %q, want %q", got, want)
	}
}

// TestRasterize_RsvgConvertExitsNonZero_ReportsItsStderr covers the first
// stage's failure on a machine without librsvg, where the tool-gated test
// above can only skip. The message has to carry rsvg-convert's own complaint,
// since that is all a maintainer is told about an icon it refused.
func TestRasterize_RsvgConvertExitsNonZero_ReportsItsStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are POSIX shell scripts")
	}
	bin := t.TempDir()
	writeFakeTool(t, bin, "rsvg-convert", "echo 'bad svg' >&2\nexit 5\n")
	writeFakeTool(t, bin, "cwebp", "exit 0\n")
	t.Setenv("PATH", bin)

	_, err := rasterize("<svg></svg>", colorLight)
	if err == nil {
		t.Fatal("rasterize() error = nil, want the rsvg-convert failure reported")
	}
	if !strings.Contains(err.Error(), "rsvg-convert: exit status 5") {
		t.Errorf("rasterize() error = %v, want it to name the tool and its status", err)
	}
	if !strings.Contains(err.Error(), "bad svg") {
		t.Errorf("rasterize() error = %v, want it to carry the tool's own stderr", err)
	}
}

// --- runMain ---

// TestRunMain_FlagParsing_ExitsCleanOnHelpAndTwoOnANameItRefuses verifies the
// flag set is the command's own rather than the package-level one: it writes
// to the stream it was handed, names the command rather than whichever binary
// drove it, and turns a parse failure into an exit code instead of an os.Exit
// nothing can observe.
func TestRunMain_FlagParsing_ExitsCleanOnHelpAndTwoOnANameItRefuses(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "help", args: []string{"-h"}, want: 0},
		{name: "a flag it does not define", args: []string{"-nope"}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer

			if got := runMain(tt.args, &stderr); got != tt.want {
				t.Errorf("runMain(%v) = %d, want %d", tt.args, got, tt.want)
			}
			if !strings.Contains(stderr.String(), "Usage of "+toolName) {
				t.Errorf("runMain(%v) stderr = %q, want the usage naming the command", tt.args, stderr.String())
			}
			if !strings.Contains(stderr.String(), "-check") {
				t.Errorf("runMain(%v) stderr = %q, want it to list the one flag there is", tt.args, stderr.String())
			}
		})
	}
}

// TestRunMain_WithoutTheExternalTools_ExitsOneNamingThem verifies the command
// refuses before it reads or writes anything when librsvg and libwebp are
// missing, and says which, since that refusal is the whole reason this tool
// checks PATH at all.
func TestRunMain_WithoutTheExternalTools_ExitsOneNamingThem(t *testing.T) {
	t.Setenv("PATH", "")
	var stderr bytes.Buffer

	if got := runMain(nil, &stderr); got != 1 {
		t.Errorf("runMain() = %d, want 1 when neither tool is on PATH", got)
	}
	if !strings.HasPrefix(stderr.String(), toolName+": ") {
		t.Errorf("runMain() stderr = %q, want it to begin with the command's name", stderr.String())
	}
	if !strings.Contains(stderr.String(), "rsvg-convert") || !strings.Contains(stderr.String(), "cwebp") {
		t.Errorf("runMain() stderr = %q, want both missing tools named", stderr.String())
	}
}

// TestRunMain_GeneratesEveryAssetAndExitsZero drives the whole command end to
// end against a repository of the test's own: the flags, the PATH check, the
// root walk, both icon sources and the real rasterize, which here reaches the
// fake tools. It is what proves the pieces are wired to each other and not
// only correct apart, and the last markup rsvg-convert received says the brand
// mark is read after icons.go rather than instead of it.
func TestRunMain_GeneratesEveryAssetAndExitsZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are POSIX shell scripts")
	}
	rec := recordingTools(t)
	root := writeFakeRepo(t)
	t.Chdir(root)
	var stderr bytes.Buffer
	code := 1

	out := captureStdout(t, func() { code = runMain(nil, &stderr) })

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("runMain() = %d with stderr %q, want 0 and nothing written", code, stderr.String())
	}
	if want := "wrote 8 webp files for 4 icons into " + generatedAssetDir + "\n"; out != want {
		t.Errorf("runMain() printed %q, want %q", out, want)
	}
	if got := readRecord(t, rec, "rsvg.stdin"); got != "<svg>brand</svg>" {
		t.Errorf("the last markup rsvg-convert read was %q, want the brand mark, the second source", got)
	}
	asset := filepath.Join(root, filepath.FromSlash(generatedAssetDir), "brand-dark.webp")
	if got, err := os.ReadFile(asset); err != nil || string(got) != "FAKEWEBP" {
		t.Errorf("brand-dark.webp = %q (err %v), want what cwebp emitted", got, err)
	}
}

// TestRunMain_CheckMode_ExitsOneAndWritesNothing verifies the gating mode: it
// reports the assets that are missing and creates no file, which is what lets
// CI run it against a checkout it must not modify.
func TestRunMain_CheckMode_ExitsOneAndWritesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are POSIX shell scripts")
	}
	recordingTools(t)
	root := writeFakeRepo(t)
	t.Chdir(root)
	var stderr bytes.Buffer
	code := 0

	out := captureStdout(t, func() { code = runMain([]string{"-check"}, &stderr) })

	if code != 1 {
		t.Errorf("runMain(-check) = %d, want 1 when the assets have never been generated", code)
	}
	if out != "" {
		t.Errorf("runMain(-check) printed %q on stdout, want nothing when it refuses", out)
	}
	if !strings.Contains(stderr.String(), "stale or missing") || !strings.Contains(stderr.String(), "brand-dark.webp") {
		t.Errorf("runMain(-check) stderr = %q, want it to name the assets to regenerate", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(generatedAssetDir))); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) err = %v, want the output directory not to exist: check mode writes nothing", generatedAssetDir, err)
	}
}

// TestMainEntry_ExitsWithTheCodeRunMainReturns verifies main hands the process
// arguments to runMain and exits with what it returns, through the osExit seam
// so the failing invocation does not end the test process. Without it the one
// line main carries is reachable from no test, and a main that ignored that
// code would exit 0 on every failure.
func TestMainEntry_ExitsWithTheCodeRunMainReturns(t *testing.T) {
	originalArgs, originalExit, originalStderr := os.Args, osExit, os.Stderr
	t.Cleanup(func() { os.Args, osExit, os.Stderr = originalArgs, originalExit, originalStderr })
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devnull.Close() })
	os.Stderr = devnull
	// An empty PATH makes the tool check refuse, so main reaches its exit
	// without reading a repository or writing an asset.
	t.Setenv("PATH", "")
	os.Args = []string{toolName}
	gotCode, called := 0, false
	osExit = func(code int) { gotCode, called = code, true }

	main()

	if !called {
		t.Fatal("main() returned without reaching osExit: the process would exit 0 whatever runMain reported")
	}
	if gotCode != 1 {
		t.Errorf("main() exit code = %d, want 1 when the external tools are missing", gotCode)
	}
}
