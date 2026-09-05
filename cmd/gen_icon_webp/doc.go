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
