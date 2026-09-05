// Command gen_lhm_manifest regenerates the tools, prompts, and resources arrays
// in lhm.plugin.json, the manifest published to the LobeHub Marketplace.
//
// LobeHub derives a listing's capability badges from the manifest's own
// tools/prompts/resources arrays — its scanner cannot introspect a server that
// ships as a Go binary or a Docker image, so a manifest without them lists the
// server as having zero tools and zero prompts no matter what the server
// actually registers. The arrays are therefore data we owe the marketplace, and
// this command derives them from a real tools/list, prompts/list, and
// resources/list round-trip against an in-memory server rather than from a
// hand-written copy that would drift on the next release.
//
// The declared tool surface is the default one, dynamic, pinned explicitly
// rather than read from TOOL_SURFACE: the manifest describes what a user gets
// with no configuration, and reading the environment would make the committed
// file depend on the machine that generated it.
//
// Every other field the manifest carries is preserved, version included — the
// release stamp owns that one.
//
// Usage:
//
//	go run ./cmd/gen_lhm_manifest/
//	go run ./cmd/gen_lhm_manifest/ --check
package main
