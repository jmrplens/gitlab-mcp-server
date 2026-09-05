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
