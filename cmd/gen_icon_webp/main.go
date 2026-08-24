// Command gen_icon_webp rasterizes the SVG icon constants declared in
// internal/toolutil/icons.go into lossless WebP fallbacks for MCP clients
// that reject image/svg+xml (VS Code Copilot's mcpIcons.ts MIME allowlist,
// for example, admits image/webp but not SVG).
//
// For every svg<Name> = `<svg ...>` constant it emits two 16x16 lossless
// WebP files under internal/toolutil/icons/webp/:
//
//	<name>-light.webp — near-black glyph (#1A1A1A), for Icon.Theme "light"
//	<name>-dark.webp  — near-white glyph (#FAFAFA), for Icon.Theme "dark"
//
// It requires rsvg-convert (librsvg) and cwebp (libwebp) on PATH. This is a
// maintainer-only, occasional regeneration step: the generated .webp files
// are committed to the repository, so ordinary builds and CI never invoke
// this tool. Run it after adding or editing an icon in icons.go.
//
// Usage:
//
//	go run ./cmd/gen_icon_webp/
//	go run ./cmd/gen_icon_webp/ --check
package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	sourceFile = "internal/toolutil/icons.go"
	outDir     = "internal/toolutil/icons/webp"
	rasterSize = "16"

	colorLight = "#1A1A1A" // near-black, for Icon.Theme "light" (light background)
	colorDark  = "#FAFAFA" // near-white, for Icon.Theme "dark" (dark background)
)

type iconSource struct {
	name string // lowercase icon name, e.g. "branch"
	svg  string
}

type variant struct {
	suffix string
	color  string
}

func variants() []variant {
	return []variant{
		{suffix: "-light", color: colorLight},
		{suffix: "-dark", color: colorDark},
	}
}

func main() {
	check := len(os.Args) > 1 && os.Args[1] == "--check"

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}

	icons, err := extractIcons(filepath.Join(root, sourceFile))
	if err != nil {
		fatal(err)
	}
	if len(icons) == 0 {
		fatal(fmt.Errorf("no svg<Name> constants found in %s", sourceFile))
	}

	if toolErr := requireTools("rsvg-convert", "cwebp"); toolErr != nil {
		fatal(toolErr)
	}

	dir := filepath.Join(root, outDir)
	if check {
		runCheck(dir, icons)
		return
	}
	runGenerate(dir, icons)
}

func runCheck(dir string, icons []iconSource) {
	if err := checkAll(dir, icons); err != nil {
		fatal(err)
	}
	fmt.Printf("icon webp assets are up to date (%d icons, %d files)\n", len(icons), len(icons)*2)
}

func runGenerate(dir string, icons []iconSource) {
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		fatal(mkErr)
	}
	written := 0
	for _, ic := range icons {
		for _, v := range variants() {
			path := filepath.Join(dir, ic.name+v.suffix+".webp")
			data, rasterErr := rasterize(ic.svg, v.color)
			if rasterErr != nil {
				fatal(fmt.Errorf("%s: %w", ic.name, rasterErr))
			}
			if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil { //nolint:gosec // generated asset, not sensitive
				fatal(writeErr)
			}
			written++
		}
	}
	fmt.Printf("wrote %d webp files for %d icons into %s\n", written, len(icons), outDir)
}

func checkAll(dir string, icons []iconSource) error {
	var stale []string
	for _, ic := range icons {
		for _, v := range variants() {
			path := filepath.Join(dir, ic.name+v.suffix+".webp")
			want, err := rasterize(ic.svg, v.color)
			if err != nil {
				return fmt.Errorf("%s: %w", ic.name, err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, want) {
				stale = append(stale, filepath.Base(path))
			}
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		return fmt.Errorf("icon webp assets are stale or missing, run `go run ./cmd/gen_icon_webp/`: %s", strings.Join(stale, ", "))
	}
	return nil
}

// rasterize renders svg (with every "currentColor" replaced by color) to a
// 16x16 PNG via rsvg-convert, then re-encodes it as lossless WebP via cwebp.
func rasterize(svg, color string) ([]byte, error) {
	colored := strings.ReplaceAll(svg, "currentColor", color)
	ctx := context.Background()

	rsvg := exec.CommandContext(ctx, "rsvg-convert", "-w", rasterSize, "-h", rasterSize, "--format=png")
	rsvg.Stdin = strings.NewReader(colored)
	var png bytes.Buffer
	rsvg.Stdout = &png
	var rsvgErr bytes.Buffer
	rsvg.Stderr = &rsvgErr
	if err := rsvg.Run(); err != nil {
		return nil, fmt.Errorf("rsvg-convert: %w: %s", err, rsvgErr.String())
	}

	cwebp := exec.CommandContext(ctx, "cwebp", "-lossless", "-z", "9", "-quiet", "-o", "-", "--", "-")
	cwebp.Stdin = bytes.NewReader(png.Bytes())
	var webp bytes.Buffer
	cwebp.Stdout = &webp
	var cwebpErr bytes.Buffer
	cwebp.Stderr = &cwebpErr
	if err := cwebp.Run(); err != nil {
		return nil, fmt.Errorf("cwebp: %w: %s", err, cwebpErr.String())
	}
	return webp.Bytes(), nil
}

// extractIcons parses sourceFile's AST and returns every constant declared
// as svg<Name> = `<svg ...>`, in declaration order.
func extractIcons(path string) ([]iconSource, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var icons []iconSource
	for _, decl := range file.Decls {
		icons = append(icons, constDeclIcons(decl)...)
	}
	return icons, nil
}

// constDeclIcons returns every svg<Name> = `<svg ...>` constant declared by
// decl, or nil if decl is not a const block.
func constDeclIcons(decl ast.Decl) []iconSource {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.CONST {
		return nil
	}
	var icons []iconSource
	for _, spec := range gen.Specs {
		icons = append(icons, valueSpecIcons(spec)...)
	}
	return icons
}

// valueSpecIcons returns every svg<Name> = `<svg ...>` constant declared by
// a single `name = value` spec (specs can declare several names at once).
func valueSpecIcons(spec ast.Spec) []iconSource {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok || len(vs.Names) != len(vs.Values) {
		return nil
	}
	var icons []iconSource
	for i, name := range vs.Names {
		if ic, found := svgConstIcon(name.Name, vs.Values[i]); found {
			icons = append(icons, ic)
		}
	}
	return icons
}

// svgConstIcon reports whether expr is a string literal holding SVG markup
// assigned to a svg<Name> identifier, returning the resolved iconSource.
func svgConstIcon(name string, expr ast.Expr) (iconSource, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return iconSource{}, false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		// Raw (backtick) string literals are not valid input to
		// strconv.Unquote; strip the surrounding backticks directly since
		// our SVG bodies never contain one.
		value = strings.Trim(lit.Value, "`")
	}
	if !strings.HasPrefix(name, "svg") || !strings.HasPrefix(value, "<svg") {
		return iconSource{}, false
	}
	return iconSource{name: iconFileName(strings.TrimPrefix(name, "svg")), svg: value}, true
}

// iconFileName converts a Go identifier suffix like "MR" or "Branch" into a
// lowercase, filesystem-safe asset name ("mr", "branch").
func iconFileName(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s))
}

func requireTools(names ...string) error {
	var missing []string
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tool(s) not found on PATH: %s (install librsvg and libwebp, e.g. `brew install librsvg webp`)", strings.Join(missing, ", "))
	}
	return nil
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", wd)
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen_icon_webp:", err)
	os.Exit(1)
}
