# Client Compatibility

The server ships its full MCP surface — tool icons, content annotations, `structuredContent`, `outputSchema`, completions — to every client. MCP clients are required to ignore fields they do not understand, and a survey of the ecosystem (Cursor, Windsurf, Zed, Cline, Continue, VS Code Copilot, JetBrains, Gemini CLI, Goose, opencode, Crush, Claude Code, and others) confirms none of them reject unknown fields. One client needs an exception, applied automatically per session.

## The Codex profile

The OpenAI Codex builds bundled with ChatGPT.app (verified on `codex-cli 0.148.0-alpha.9`) fail to parse any MCP result whose annotations carry a **non-integer `priority`** (for example `0.6`, which the MCP specification allows as a 0–1 number). The response degrades inside Codex's bundled `rmcp` parser and every affected `tools/call` is reported as:

```text
tool call error: tool call failed for `gitlab/<tool>`
Caused by: Unexpected response type
```

Since this server annotates its markdown content blocks with float priorities, every successful tool call used to fail in Codex.

The fix is a per-session compatibility middleware (`internal/clientcompat`):

- **Detection** — the `initialize` request's `clientInfo` is matched case-insensitively for `codex` in the name or title. Codex has identified itself as `codex-mcp-client` / `Codex` since v0.20.
- **Rewrite** — only the `priority` field is rounded to the nearest spec-legal integer (0 or 1, both of which Codex parses) in `tools/call` results, `resources/list`, `resources/templates/list`, and `prompts/get`. Audience annotations, markdown text, `structuredContent`, `outputSchema`, icons, and the tool-level annotations Codex's approval policy reads (`readOnlyHint`, `destructiveHint`) are all delivered unchanged.
- **Isolation** — results are cloned before rewriting, so in HTTP mode concurrent non-Codex sessions of the same pooled server keep the full response.

Every other client — including Claude Code, Claude Desktop, and every client in the survey above — receives the complete, unmodified response with the exact float priorities.

### Kill switch

| Variable        | Default | Description                                                                                          |
| --------------- | ------- | ---------------------------------------------------------------------------------------------------- |
| `CLIENT_COMPAT` | `auto`  | Set to `off` to disable per-client response rewriting (all clients then receive identical responses) |

## Client limits worth knowing

These are client-side constraints, not server behavior. The default `dynamic` surface (2 tools) fits every client; the `meta` surface (~33–50 tools) fits everywhere except Cursor's cap; the `individual` surface only suits clients without tool caps.

| Client                                                                      | Limit                                                                                                                                                                                                                                                                                              |
| --------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cursor                                                                      | 40 tools per server; silently drops tools whose server + tool name exceeds ~60 characters                                                                                                                                                                                                          |
| Windsurf                                                                    | 100 tools total                                                                                                                                                                                                                                                                                    |
| OpenAI-backed clients (Codex, Copilot CLI, VS Code Copilot with GPT models) | 128 tools per model request                                                                                                                                                                                                                                                                        |
| Codex                                                                       | Reads only the first `tools/list` page in its default protocol mode; input schemas over ~5 KB get descriptions silently stripped (keep `META_PARAM_SCHEMA=opaque`); when `structuredContent` is present, only that JSON reaches the model ([#10334](https://github.com/openai/codex/issues/10334)) |
| Gemini CLI                                                                  | Tool names over 63 characters truncated; Gemini API rejects `$ref`/`$defs` in input schemas                                                                                                                                                                                                        |
| JetBrains AI Assistant                                                      | Rejects the whole `tools/list` if any tool's `outputSchema` root type is not `object` ([LLM-30555](https://youtrack.jetbrains.com/issue/LLM-30555))                                                                                                                                                |

## See Also

- [IDE Configuration](ide-configuration.md) — per-client setup, including the OpenAI Codex section
- [Configuration](../reference/configuration.md) — all environment variables
- [Tool Surfaces](../development/tool-surfaces-and-action-core.md) — how the dynamic, meta, and individual catalogs are projected
