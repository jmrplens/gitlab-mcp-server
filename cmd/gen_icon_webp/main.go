package main

import (
	"bytes"
	"context"
	"flag"
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
	// brandFile carries the generated project mark (cmd/gen_brand); its
	// svgBrand constant needs the same WebP fallbacks as the domain icons.
	brandFile  = "internal/toolutil/brandmark_gen.go"
	outDir     = "internal/toolutil/icons/webp"
	rasterSize = "16"

	colorLight = "#1A1A1A" // near-black, for Icon.Theme "light" (light background)
	colorDark  = "#FAFAFA" // near-white, for Icon.Theme "dark" (dark background)
)

// iconSource is one svg<Name> constant extracted from icons.go.
type iconSource struct {
	name string // lowercase icon name, e.g. "branch"
	svg  string
}

// variant is one of the two themed WebP files generated per icon.
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

// rasterizer renders svg (with "currentColor" replaced by color) into an
// encoded image and returns its bytes. Production code uses [rasterize]
// (rsvg-convert + cwebp); tests inject a fake so checkAll/generateAll are
// exercised without requiring those external tools on PATH.
type rasterizer func(svg, color string) ([]byte, error)

func main() {
	check := flag.Bool("check", false, "verify the committed WebP assets match icons.go without writing them")
	flag.Parse()

	if err := requireTools("rsvg-convert", "cwebp"); err != nil {
		fatal(err)
	}
	if err := run(*check, rasterize); err != nil {
		fatal(err)
	}
}

// run locates the repository root and delegates to runIn with this
// command's real source/output paths.
func run(check bool, raster rasterizer) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	return runIn(root, []string{sourceFile, brandFile}, outDir, check, raster)
}

// runIn is gen_icon_webp's testable core: given an explicit repo root, the
// icon source paths and a WebP output directory (both relative to root), it
// extracts every svg<Name> constant and either verifies (check) or
// (re)writes the WebP assets for all of them using raster.
func runIn(root string, sources []string, out string, check bool, raster rasterizer) error {
	var icons []iconSource
	for _, source := range sources {
		found, err := extractIcons(filepath.Join(root, source))
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return fmt.Errorf("no svg<Name> constants found in %s", source)
		}
		icons = append(icons, found...)
	}

	dir := filepath.Join(root, out)
	if check {
		if checkErr := checkAll(dir, icons, raster); checkErr != nil {
			return checkErr
		}
		fmt.Printf("icon webp assets are up to date (%d icons, %d files)\n", len(icons), len(icons)*2)
		return nil
	}

	written, genErr := generateAll(dir, icons, raster)
	if genErr != nil {
		return genErr
	}
	fmt.Printf("wrote %d webp files for %d icons into %s\n", written, len(icons), out)
	return nil
}

// generateAll (re)writes every icon's light/dark WebP file under dir,
// returning how many files it wrote.
func generateAll(dir string, icons []iconSource, raster rasterizer) (int, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return 0, err
	}
	written := 0
	for _, ic := range icons {
		for _, v := range variants() {
			path := filepath.Join(dir, ic.name+v.suffix+".webp")
			data, rasterErr := raster(ic.svg, v.color)
			if rasterErr != nil {
				return written, fmt.Errorf("%s: %w", ic.name, rasterErr)
			}
			if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil { //nolint:gosec // generated asset, not sensitive
				return written, writeErr
			}
			written++
		}
	}
	return written, nil
}

// checkAll reports whether every icon's light/dark WebP file under dir is
// present and byte-identical to what raster would produce.
func checkAll(dir string, icons []iconSource, raster rasterizer) error {
	var stale []string
	for _, ic := range icons {
		for _, v := range variants() {
			path := filepath.Join(dir, ic.name+v.suffix+".webp")
			want, err := raster(ic.svg, v.color)
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
// This is the only function that shells out to external tools; production
// code reaches it through the [rasterizer] indirection.
//
// Both tools are invoked by their exec.LookPath-resolved absolute path,
// not by bare name: passing a bare name lets exec.Command re-resolve it
// against PATH at run time, which a writable-PATH-entry attack could hijack
// (Sonar go:S4036 / CWE-427 uncontrolled search path).
//
// Coverage note: the rsvg-convert LookPath and Run failure branches are
// each tested directly; the cwebp LookPath and Run failures are not, since
// forcing "rsvg-convert present, cwebp absent" needs test-machine-specific
// PATH surgery, and forcing cwebp's Run to fail needs a PNG rsvg-convert
// itself refuses to ever emit — not worth it for a maintainer-only tool.
func rasterize(svg, color string) ([]byte, error) {
	colored := strings.ReplaceAll(svg, "currentColor", color)
	ctx := context.Background()

	rsvgPath, err := exec.LookPath("rsvg-convert")
	if err != nil {
		return nil, err
	}
	rsvg := exec.CommandContext(ctx, rsvgPath, "-w", rasterSize, "-h", rasterSize, "--format=png")
	rsvg.Stdin = strings.NewReader(colored)
	var png bytes.Buffer
	rsvg.Stdout = &png
	var rsvgErr bytes.Buffer
	rsvg.Stderr = &rsvgErr
	if runErr := rsvg.Run(); runErr != nil {
		return nil, fmt.Errorf("rsvg-convert: %w: %s", runErr, rsvgErr.String())
	}

	cwebpPath, err := exec.LookPath("cwebp")
	if err != nil {
		return nil, err
	}
	cwebp := exec.CommandContext(ctx, cwebpPath, "-lossless", "-z", "9", "-quiet", "-o", "-", "--", "-")
	cwebp.Stdin = bytes.NewReader(png.Bytes())
	var webp bytes.Buffer
	cwebp.Stdout = &webp
	var cwebpErr bytes.Buffer
	cwebp.Stderr = &cwebpErr
	if runErr := cwebp.Run(); runErr != nil {
		return nil, fmt.Errorf("cwebp: %w: %s", runErr, cwebpErr.String())
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
// a single `name = value` spec (a spec can declare several names at once,
// and a grouped const block can also declare a name with no value of its
// own, inheriting the previous spec's — that case is skipped here since
// Names and Values no longer line up).
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
	// lit.Value is a lexically valid Go string literal (single-quoted,
	// double-quoted, or backquoted) because the parser already accepted it,
	// so Unquote only fails here for a literal this function doesn't want
	// anyway (its content isn't SVG markup).
	value, err := strconv.Unquote(lit.Value)
	if err != nil || !strings.HasPrefix(name, "svg") || !strings.HasPrefix(value, "<svg") {
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

// requireTools reports an actionable error naming every tool in names that
// is missing from PATH, or nil if all are present.
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

// repoRoot walks up from the working directory to the nearest ancestor
// containing a go.mod file.
//
// Coverage note: os.Getwd()'s own error path is not tested — reliably
// forcing it requires deleting the process's current directory out from
// under it, which is racy and OS-dependent.
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
