// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package clientcompat applies per-client response compatibility profiles to
// MCP results. Most MCP clients ignore fields they do not understand, so the
// server ships its full surface (icons, content annotations, structured
// content) unconditionally. OpenAI Codex is the exception: the Codex builds
// bundled with ChatGPT.app (verified on codex-cli 0.148.0-alpha.9) fail any
// result whose annotations carry a non-integer priority, and every affected
// call surfaces as "Unexpected response type". rmcp types the field correctly,
// at 3.2.0 and 3.3.0, which Codex 0.162.1 and earlier pin; the defect is
// Codex's build, where Cargo feature unification turns serde_json's
// arbitrary_precision on for the whole binary, so a buffered decimal reaches
// the float field as serde_json's private number map, the field refuses it and
// the untagged result falls through to rmcp's CustomResult
// (https://github.com/openai/codex/issues/38979). The fix is released in rmcp
// 3.5.0, and Codex 0.163.0, released on 2026-10-15, is the first Codex built on
// it (rmcp 3.5.1). The defect was seen in the Codex ChatGPT.app bundles, a
// bundle that moves on its own schedule, so a Codex release alone does not end
// it. This package detects Codex and writes the priority as the nearest
// spec-legal integer (0 or 1) for that session; audience, structuredContent,
// outputSchema, icons, and every other field are preserved, and every other
// client keeps the exact float values.
//
// Codex is recognized by the clientInfo the session reports, matched to the
// two spellings Codex has used since v0.20: a case-insensitive
// "codex-mcp-client" name prefix, or a title of exactly "Codex". That reaches
// stdio in either protocol era, HTTP with --stateless=false, and any session
// at 2026-07-28, whose requests each carry clientInfo in _meta. Over the
// default stateless HTTP transport a client on 2025-11-25 or earlier reports
// clientInfo only in an initialize no later POST's session saw, so a session
// with no clientInfo falls back to the request's User-Agent, matched to a
// case-insensitive "codex-mcp-client/" prefix, which Codex's MCP client sends
// on every Streamable HTTP request (issue 1043). The clientInfo decides
// whenever the session has one.
//
// The match is held to Codex's own spellings rather than a "codex" substring
// because OpenAI's hosted client needs no profile: measured on 2026-09-28
// (issue 1044), it reports clientInfo "openai-mcp (Responses API)" or
// "(Realtime API)" with no title and reads a fractional priority without
// error. A ChatGPT web tool call carries the User-Agent
// "openai-mcp/1.0.0 (Codex)", whose clientInfo is the one label still
// unmeasured; neither match catches the word in it.
//
// Choosing a response from clientInfo is a deliberate deviation from MCP
// 2026-07-28, which says implementations SHOULD NOT use it "to change the
// behavior of the client or server" (issue 959, register row IDN-013). It is
// kept because it changes how one number is written and nothing a model
// reads, and it never decides who a caller is or what it may do, which is the
// note's second half and is met. The User-Agent fallback widens the same
// deviation to a second self-reported label, read only where the clientInfo
// is absent and for the same one number (issue 1043, recorded on the same
// row). GITLAB_MCP_CLIENT_COMPAT=off removes both. The profile does not read
// the Codex version a session reports, and will not: it is retired outright,
// the package, IDN-013 and the setting with it, once ChatGPT.app bundles Codex
// 0.163.0 or later, which is all it waits on (decided on issue 1404 on
// 2026-11-01).
//
// The rounding holds only because encoding/json writes an integral float64 as
// 1 and never as 1.0, which Codex rejects as well; a test pins that wire form.
package clientcompat
