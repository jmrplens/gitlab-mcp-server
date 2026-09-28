// Package clientcompat applies per-client response compatibility profiles to
// MCP results. Most MCP clients ignore fields they do not understand, so the
// server ships its full surface (icons, content annotations, structured
// content) unconditionally. OpenAI Codex is the exception: the Codex builds
// bundled with ChatGPT.app (verified on codex-cli 0.148.0-alpha.9, which pins
// rmcp 3.2.0) fail any result whose annotations carry a non-integer priority,
// and every affected call surfaces as "Unexpected response type". rmcp types
// the field correctly; the defect is Codex's build, where Cargo feature
// unification turns serde_json's arbitrary_precision on for the whole binary,
// so a buffered decimal reaches the float field as serde_json's private number
// map, the field refuses it and the untagged result falls through to rmcp's
// CustomResult (row 17 of docs/development/upstream-bugs.md). This package
// detects Codex from the clientInfo the session reports and writes the
// priority as the nearest spec-legal integer (0 or 1) for that session;
// audience, structuredContent, outputSchema, icons, and every other field are
// preserved, and every other client keeps the exact float values.
//
// Choosing a response from clientInfo is a deliberate deviation from MCP
// 2026-07-28, which says implementations SHOULD NOT use it "to change the
// behavior of the client or server" (issue 959, register row IDN-013). It is
// kept because it changes how one number is written and nothing a model
// reads, and it never decides who a caller is or what it may do, which is the
// note's second half and is met. GITLAB_MCP_CLIENT_COMPAT=off removes it, and
// it retires once a Codex built on an rmcp release carrying the fix is widely
// deployed, not merely released.
//
// The rounding holds only because encoding/json writes an integral float64 as
// 1 and never as 1.0, which Codex rejects as well; a test pins that wire form.
package clientcompat
