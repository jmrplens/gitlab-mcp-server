// Package clientcompat applies per-client response compatibility profiles to
// MCP results. Most MCP clients ignore fields they do not understand, so the
// server ships its full surface (icons, content annotations, structured
// content) unconditionally. OpenAI Codex is the exception: the Codex builds
// bundled with ChatGPT.app (verified on codex-cli 0.148.0-alpha.9, rmcp
// 3.0.0) fail any result whose annotations carry a non-integer priority —
// the response degrades to rmcp's CustomResult and every affected call
// surfaces as "Unexpected response type". This package detects Codex from
// the session's initialize clientInfo and rounds the priority to the nearest
// spec-legal integer (0 or 1) for that session; audience, structuredContent,
// outputSchema, icons, and every other field are preserved, and every other
// client keeps the exact float values.
package clientcompat
