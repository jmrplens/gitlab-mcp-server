// Command gen_brand emits every vector brand asset from one parametric
// geometry, so the mark cannot drift between its surfaces.
//
// The mark is "the fan-out": a source node projecting three branch arcs,
// each terminating in a node. It reads as a git graph and as the project's
// actual architecture — one canonical action catalog projected to the
// dynamic, meta, and individual tool surfaces. The geometry lives here as
// constants; every emitter (site logo, mono mark, favicon, in-binary MCP
// brand mark) renders the same arcs at its own scale, so editing a curve
// edits every asset in the same run.
//
// Outputs (relative to the repository root):
//
//	site/src/assets/logo.svg           canonical classed mark, no fills — painted by site CSS tokens
//	.github/brand/logo-mono.svg        single-color currentColor variant
//	site/public/favicon.svg            mark on its own dark ground, self-contained colors
//	internal/toolutil/brandmark_gen.go the 24×24 currentColor MCP brand mark (Go const)
//
// Usage:
//
//	go run ./cmd/gen_brand/            # write all assets
//	go run ./cmd/gen_brand/ --check    # byte-compare against disk, exit 1 on drift
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Palette: the "circuit violet" identity on the GitHub neutral ramp (see
// plan/2026-08-brand-redesign.md §2). The favicon embeds the dark values —
// the art brings its own ground — while the canonical mark carries no color
// at all and is painted by the site's CSS tokens.
const (
	darkGround = "#0d1117" // GitHub dark black: the mark's own ground
	darkNode   = "#d6c9ff" // accent-ink: the source node
	darkBranch = "#a78bfa" // accent: the three branch arcs
	darkTip    = "#fc8a3d" // secondary (GitLab-orange family, saturated): terminal nodes
)

// Geometry of the fan-out at the canonical 64×64 viewBox. The three arcs
// leave the source node horizontally and arrive at their tips horizontally
// (cubic Béziers with axis-aligned tangents — the git-graph curve).
const (
	canvas = 64.0

	srcX = 14.0 // source node center
	srcY = 32.0
	srcR = 5.0

	branchStartX = 18.0 // arcs start at the node's right edge
	branchEndX   = 46.0 // arcs end where the tip circles begin
	branchWidth  = 3.5

	tipX    = 49.5 // terminal node centers
	tipR    = 3.5
	tipSpan = 18.0 // vertical distance from the center branch to the outer tips
)

// arcPath returns the cubic Bézier for one branch: horizontal tangent out
// of the source, horizontal tangent into the tip. dy is the tip's vertical
// offset from the source (negative up, 0 straight, positive down).
func arcPath(dy float64) string {
	if dy == 0 {
		return fmt.Sprintf("M %g %g H %g", branchStartX, srcY, branchEndX)
	}
	midX := (branchStartX + branchEndX) / 2
	return fmt.Sprintf("M %g %g C %g %g, %g %g, %g %g",
		branchStartX, srcY,
		midX, srcY,
		midX, srcY+dy,
		branchEndX, srcY+dy)
}

// markBody renders the shared geometry with per-part attribute strings, so
// each emitter chooses classes, currentColor, or literal colors without
// duplicating a single coordinate.
func markBody(nodeAttrs, branchAttrs, tipAttrs string) string {
	var body strings.Builder
	for _, dy := range []float64{-tipSpan, 0, tipSpan} {
		fmt.Fprintf(&body, "  <path d=%q fill=\"none\" stroke-width=\"%g\" stroke-linecap=\"round\" %s/>\n",
			arcPath(dy), branchWidth, branchAttrs)
	}
	for _, dy := range []float64{-tipSpan, 0, tipSpan} {
		fmt.Fprintf(&body, "  <circle cx=\"%g\" cy=\"%g\" r=\"%g\" %s/>\n", tipX, srcY+dy, tipR, tipAttrs)
	}
	fmt.Fprintf(&body, "  <circle cx=\"%g\" cy=\"%g\" r=\"%g\" %s/>\n", srcX, srcY, srcR, nodeAttrs)
	return body.String()
}

// canonicalSVG is the site mark: classes only, no fills, painted entirely
// by the site's CSS tokens (--gm-mark and friends) so mark and palette
// cannot drift and the mark follows the theme toggle.
func canonicalSVG() string {
	return fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %g %g\" role=\"img\" aria-label=\"gitlab-mcp-server\">\n%s</svg>\n",
		canvas, canvas,
		markBody(`class="m-node"`, `class="m-branch"`, `class="m-tip"`))
}

// monoSVG is the single-color variant: everything currentColor, for
// contexts that bring their own color (README on GitHub, embeds).
func monoSVG() string {
	return fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %g %g\" role=\"img\" aria-label=\"gitlab-mcp-server\">\n%s</svg>\n",
		canvas, canvas,
		markBody(`fill="currentColor"`, `stroke="currentColor"`, `fill="currentColor"`))
}

// faviconSVG is self-grounding: the mark in its dark colors on a rounded
// dark plate, identical under any browser theme (phonometry's one-card
// rule — the art brings its own ground).
func faviconSVG() string {
	return fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %g %g\" role=\"img\" aria-label=\"gitlab-mcp-server\">\n"+
		"  <rect width=\"%g\" height=\"%g\" rx=\"14\" fill=\"%s\"/>\n%s</svg>\n",
		canvas, canvas, canvas, canvas, darkGround,
		markBody(
			fmt.Sprintf("fill=%q", darkNode),
			fmt.Sprintf("stroke=%q", darkBranch),
			fmt.Sprintf("fill=%q", darkTip),
		))
}

// brandMark24 is the in-binary MCP brand mark: the same fan-out at a 24×24
// viewBox, currentColor throughout, sized to sit beside the 16×16 domain
// icons the server ships. Scaled from the canonical geometry rather than
// redrawn, so the two cannot diverge.
func brandMark24() string {
	const s = 24.0 / canvas
	var body strings.Builder
	for _, dy := range []float64{-tipSpan, 0, tipSpan} {
		var d string
		if dy == 0 {
			d = fmt.Sprintf("M %.3g %.3g H %.3g", branchStartX*s, srcY*s, branchEndX*s)
		} else {
			midX := (branchStartX + branchEndX) / 2
			d = fmt.Sprintf("M %.3g %.3g C %.3g %.3g, %.3g %.3g, %.3g %.3g",
				branchStartX*s, srcY*s, midX*s, srcY*s, midX*s, (srcY+dy)*s, branchEndX*s, (srcY+dy)*s)
		}
		fmt.Fprintf(&body, `<path d=%q fill="none" stroke="currentColor" stroke-width="%.3g" stroke-linecap="round"/>`, d, branchWidth*s)
	}
	for _, dy := range []float64{-tipSpan, 0, tipSpan} {
		fmt.Fprintf(&body, `<circle cx="%.3g" cy="%.3g" r="%.3g" fill="currentColor"/>`, tipX*s, (srcY+dy)*s, tipR*s)
	}
	fmt.Fprintf(&body, `<circle cx="%.3g" cy="%.3g" r="%.3g" fill="currentColor"/>`, srcX*s, srcY*s, srcR*s)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">%s</svg>`, body.String())
}

// brandMarkGo renders the generated Go source carrying the in-binary mark.
func brandMarkGo() string {
	return fmt.Sprintf(`// Code generated by cmd/gen_brand; DO NOT EDIT.

package toolutil

// svgBrand is the project mark — "the fan-out": one source node projecting
// three branch arcs, the canonical action catalog reaching its three tool
// surfaces. Original artwork for this project (no GitLab trademark
// geometry); the canonical vector lives in cmd/gen_brand and is scaled to
// this 24×24 currentColor variant for MCP clients. Regenerate with
// `+"`make brand`"+`, then refresh the WebP fallbacks with cmd/gen_icon_webp.
const svgBrand = `+"`%s`"+`
`, brandMark24())
}

// bannerSVG is the repository banner: 1280x400, self-grounding on the dark
// plate (phonometry's one-card rule — the art brings its own ground, so it
// reads identically under GitHub's light and dark themes). The wordmark is
// set in DejaVu Sans Mono — the project's name is an identifier, and the
// identity sets identifiers in mono — and the raster is produced from this
// file by `make brand-rasters` (rsvg-convert + cwebp, the same maintainer
// tooling cmd/gen_icon_webp already requires). No counts appear here on
// purpose: a figure would go stale between releases; figures live on
// surfaces the generators re-stamp.
func bannerSVG() string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 400" role="img" aria-label="gitlab-mcp-server — GitLab for your AI assistant">
  <rect width="1280" height="400" fill="%s"/>
  <rect x="0.5" y="0.5" width="1279" height="399" fill="none" stroke="#21262d"/>
  <g transform="translate(96,56) scale(4.5)">
%s  </g>
  <text x="500" y="196" font-family="DejaVu Sans Mono, monospace" font-weight="bold" font-size="58" fill="#e6edf3">gitlab-mcp-server</text>
  <text x="502" y="248" font-family="DejaVu Sans, sans-serif" font-size="26" fill="#8b949e">GitLab for your AI assistant — one action catalog,</text>
  <text x="502" y="284" font-family="DejaVu Sans, sans-serif" font-size="26" fill="#8b949e">three MCP tool surfaces, REST + GraphQL.</text>
  <text x="502" y="336" font-family="DejaVu Sans Mono, monospace" font-size="20" fill="%s">dynamic · meta · individual</text>
</svg>
`, darkGround,
		markBody(
			fmt.Sprintf("fill=%q", darkNode),
			fmt.Sprintf("stroke=%q", darkBranch),
			fmt.Sprintf("fill=%q", darkTip)),
		darkBranch)
}

// ogSVG is the Open Graph / social card: 1200x630, the same composition
// stacked and centered. Rendered to site/public/og-image.png by
// `make brand-rasters`.
func ogSVG() string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 630" role="img" aria-label="gitlab-mcp-server — GitLab for your AI assistant">
  <rect width="1200" height="630" fill="%s"/>
  <rect x="0.5" y="0.5" width="1199" height="629" fill="none" stroke="#21262d"/>
  <g transform="translate(444,88) scale(4.875)">
%s  </g>
  <text x="600" y="470" text-anchor="middle" font-family="DejaVu Sans Mono, monospace" font-weight="bold" font-size="64" fill="#e6edf3">gitlab-mcp-server</text>
  <text x="600" y="524" text-anchor="middle" font-family="DejaVu Sans, sans-serif" font-size="27" fill="#8b949e">GitLab for your AI assistant — one catalog, three MCP surfaces</text>
  <text x="600" y="574" text-anchor="middle" font-family="DejaVu Sans Mono, monospace" font-size="21" fill="%s">jmrplens.github.io/gitlab-mcp-server</text>
</svg>
`, darkGround,
		markBody(
			fmt.Sprintf("fill=%q", darkNode),
			fmt.Sprintf("stroke=%q", darkBranch),
			fmt.Sprintf("fill=%q", darkTip)),
		darkBranch)
}

type asset struct {
	path    string
	content string
}

func assets() []asset {
	return []asset{
		{filepath.Join("site", "src", "assets", "logo.svg"), canonicalSVG()},
		{filepath.Join(".github", "brand", "logo-mono.svg"), monoSVG()},
		{filepath.Join("site", "public", "favicon.svg"), faviconSVG()},
		{filepath.Join("internal", "toolutil", "brandmark_gen.go"), brandMarkGo()},
		{filepath.Join(".github", "brand", "banner.svg"), bannerSVG()},
		{filepath.Join(".github", "brand", "og.svg"), ogSVG()},
	}
}

// run writes (or, with check, verifies) every asset under root. root is
// the repository root; tests point it at a temporary directory.
func run(root string, check bool) error {
	drift := false
	for _, a := range assets() {
		path := filepath.Join(root, a.path)
		if check {
			existing, err := os.ReadFile(path)
			if err != nil || string(existing) != a.content {
				fmt.Fprintf(os.Stderr, "gen_brand: %s is stale\n", a.path)
				drift = true
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(a.content), 0o644); err != nil { //nolint:gosec // generated asset, not sensitive
			return fmt.Errorf("write %s: %w", a.path, err)
		}
		fmt.Printf("gen_brand: wrote %s\n", a.path)
	}
	if drift {
		return errors.New("brand assets are stale; run `make brand`")
	}
	return nil
}

func main() {
	check := flag.Bool("check", false, "verify committed assets match the geometry instead of writing")
	flag.Parse()
	if err := run("", *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
