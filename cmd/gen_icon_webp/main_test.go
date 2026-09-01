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
	"errors"
	"os"
	"path/filepath"
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
	// string literal), and svgInheritedTwo (no value of its own) must all
	// be excluded; only svgBranch, svgMR, and svgInherited qualify.
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
	branchLight, _ := os.ReadFile(filepath.Join(dir, "branch-light.webp"))
	if string(branchLight) != "<svg>branch</svg>|"+colorLight {
		t.Errorf("branch-light.webp = %q, want the fake rasterizer's deterministic output", branchLight)
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
