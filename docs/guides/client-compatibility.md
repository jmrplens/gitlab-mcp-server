# Client Compatibility

The server ships its full MCP surface — tool icons, content annotations, `structuredContent`, `outputSchema`, completions — to every client. MCP clients are required to ignore fields they do not understand, and a survey of the ecosystem (Cursor, Windsurf, Zed, Cline, Continue, VS Code Copilot, JetBrains, Gemini CLI, Goose, opencode, Crush, Claude Code, and others) confirms none of them reject unknown fields. One client needs an exception, applied automatically per session.

## The Codex profile

The OpenAI Codex builds bundled with ChatGPT.app (verified on `codex-cli 0.148.0-alpha.9`; Codex's `main` pins `rmcp` 3.3.0 since 2026-09-30, a release without the fix) fail to parse any MCP result whose annotations carry a **non-integer `priority`** (for example `0.6`, which the MCP specification allows as a number from 0 to 1). `rmcp` types the field correctly; the defect is Codex's build, where Cargo feature unification turns on `serde_json`'s `arbitrary_precision` for the whole binary, so a decimal reaches the float field as `serde_json`'s private number map, the field refuses it and the result falls through to `rmcp`'s catch-all variant. A literal `1.0` fails the same way; only `1` and `0` pass. Every affected `tools/call` is reported as:

```text
tool call error: tool call failed for `gitlab/<tool>`
Caused by: Unexpected response type
```

Since this server annotates its markdown content blocks with float priorities, every successful tool call used to fail in Codex.

The fix is a per-session compatibility middleware (`internal/clientcompat`):

- **Detection**: the `clientInfo` the session reports is matched case-insensitively for `codex` in the name or title: the one in `initialize` on protocol 2025-11-25 and earlier, and the one in the request's `_meta` on 2026-07-28, where the SDK reads it from. Codex has identified itself as `codex-mcp-client` / `Codex` since v0.20.
- **Reach**: the profile applies wherever the session knows its client, which is stdio in either protocol era, HTTP with `--stateless=false`, and any HTTP session at 2026-07-28. It does **not** apply to a Codex client speaking 2025-11-25 or earlier to the default stateless HTTP transport: there every POST gets a session of its own that never saw `initialize`, so there is no `clientInfo` to read and that client is sent the fractional priority. Such a deployment needs `--stateless=false` for the profile to reach an older Codex. `test/e2e/http` and `test/e2e/stdio` pin both halves.
- **Rewrite**: only the `priority` field is rounded to the nearest spec-legal integer (0 or 1, both of which Codex parses) in `tools/call` results, `resources/list`, `resources/templates/list`, and `prompts/get`; a priority rounded to 0 is left out, as the field is optional. It works because Go writes an integral number as `1` and never as `1.0`, a wire form a unit test and `test/e2e/stdio` both pin. Audience annotations, markdown text, `structuredContent`, `outputSchema`, icons, and the tool-level annotations Codex's approval policy reads (`readOnlyHint`, `destructiveHint`) are all delivered unchanged.
- **Isolation**: results are cloned before rewriting, so in HTTP mode concurrent non-Codex sessions of the same pooled server keep the full response.
- **OpenAI's hosted client**: it reports `clientInfo` `openai-mcp (Responses API)` or `openai-mcp (Realtime API)`, which the `codex` match does not catch, and it needs no profile: measured on 2026-09-28 ([issue 1044](https://github.com/jmrplens/gitlab-mcp-server/issues/1044)), it reads a fractional priority without error. One label is unmeasured, the `openai-mcp/1.0.0 (Codex)` User-Agent of a ChatGPT web tool call; [row 17](../development/upstream-bugs.md#a-non-integer-annotation-priority-breaks-a-tool-call) records both.

Every other client — including Claude Code, Claude Desktop, and every client in the survey above — receives the complete, unmodified response with the exact float priorities.

### A deliberate deviation from the specification

MCP 2026-07-28 says `clientInfo` is self-reported and that implementations "SHOULD NOT use them to change the behavior of the client or server, and SHOULD NOT rely on them for security decisions" ([basic](https://modelcontextprotocol.io/specification/2026-07-28/basic)). The Codex profile departs from the first half, knowingly, and [issue 959](https://github.com/jmrplens/gitlab-mcp-server/issues/959) decided to keep it:

- **It is a formatting choice and nothing more.** It changes how one number is written and nothing a model reads. It never decides who a caller is or what it may do: identity and authority come from the credential on each request, never from what a client says about itself, so the second half of the note is met.
- **It can be turned off.** `GITLAB_MCP_CLIENT_COMPAT=off` (below) gives every client the same response.
- **It retires with the defect.** The fix is merged in `rmcp` ([modelcontextprotocol/rust-sdk#1300](https://github.com/modelcontextprotocol/rust-sdk/pull/1300)) and released in `rmcp` 3.5.0, and it reaches users through a chain of three: an `rmcp` release carrying it, a Codex release built on that `rmcp`, which does not exist yet, and a ChatGPT.app that bundles that Codex. The profile goes once that Codex is widely deployed, not merely released, since users of the bundled build do not choose their version. [Row 17 of the upstream register](../development/upstream-bugs.md#a-non-integer-annotation-priority-breaks-a-tool-call) tracks each link, with [openai/codex#38979](https://github.com/openai/codex/issues/38979).

### Kill switch

| Variable                   | Default | Description                                                                                                                                                                                                                                  |
| -------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_MCP_CLIENT_COMPAT` | `auto`  | Set to `off` to disable per-client response rewriting (all clients then receive identical responses). Read from the process environment in both stdio and HTTP modes; the `--client-compat` flag sets the same variable and wins when passed |

## MCP gateway validators

An MCP gateway sits between clients and servers and validates a server's
catalog before admitting it, under rules the gateway operator chooses. One
production gateway (IBM [mcp-context-forge](https://github.com/IBM/mcp-context-forge)
before 0.7.0) rejected any tool whose description contained a semicolon:

```text
All N tools failed validation ... Description contains unsafe characters
```

This server responds on two fronts:

- **Its own text is kept clean.** Everything served by `tools/list` (on any
  surface), `prompts/list`, `resources/list`, and `resources/templates/list`
  is pure ASCII prose with no semicolons — descriptions, titles, and the
  schema-embedded descriptions included. A class is stronger than a list: a
  validator that refuses "unsafe characters" usually matches a character
  class, and ASCII-only is the one clean the next rule cannot surprise.
  `make check-gateway-chars` gates this in CI, and
  `go run ./cmd/audit_gateway_chars/` prints any offender with context. The
  same style is applied to payload prose (prompt bodies, result markdown),
  though the gate measures the listed catalog.
- **The next rule is yours to meet without waiting for a release.** The
  substitution knob rewrites listed catalog text on the way out.

### Description substitutions

| Variable                               | Default | Description                                                                                                                 |
| -------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS` | empty   | Comma-separated `old=new` pairs applied in order to every listed description and title (flag `--description-substitutions`) |

A backslash escapes a literal comma, equals sign, or backslash inside either
half. Whitespace is significant. Examples:

```bash
# Replace every semicolon with a period
GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS=';=.'

# Replace semicolons with commas (the comma must be escaped)
GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS=';=\,'

# Two ordered substitutions: "; " becomes ". ", then ":" becomes "-"
GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS='; =. ,:=-'
```

The rewrite covers what a gateway validates and nothing else: tool
descriptions, titles, annotation titles, and the `description`/`title` keys
embedded in input and output schemas, plus prompt, prompt-argument, resource,
and resource-template descriptions and titles. Names, URIs, schema
constraints (`pattern`, `const`, `enum` values, `default`) and tool-call
payloads are never touched. A malformed value refuses to start the server
rather than serving an unrewritten catalog to the gateway the operator
configured it for.

To verify a substitution config clears a character the audit knows about, run
the audit with the substitutions applied:

```bash
GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS=';=.' go run ./cmd/audit_gateway_chars/ -apply -check
```

If the gateway in front of you is mcp-context-forge, upgrading to 0.7.0 or
later also resolves the semicolon rejection at the gateway
([issue 3770](https://github.com/IBM/mcp-context-forge/issues/3770)); its
`TOOL_DESCRIPTION_FORBIDDEN_PATTERNS` setting tunes the rule, and
`VALIDATION_STRICT=false` downgrades rejection to a warning.

## Client limits worth knowing

These are client-side constraints, not server behavior. The default `dynamic` surface (2 tools) fits every client; the `meta` surface (32 to 50 tools, by tier) fits everywhere except Cursor's cap; the `individual` surface only suits clients without tool caps.

| Client                                                                      | Limit                                                                                                                                                                                                                                                                                                         |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cursor                                                                      | 40 tools per server; silently drops tools whose server + tool name exceeds ~60 characters                                                                                                                                                                                                                     |
| Windsurf                                                                    | 100 tools total                                                                                                                                                                                                                                                                                               |
| OpenAI-backed clients (Codex, Copilot CLI, VS Code Copilot with GPT models) | 128 tools per model request                                                                                                                                                                                                                                                                                   |
| Codex                                                                       | Reads only the first `tools/list` page in its default protocol mode; input schemas over ~5 KB get descriptions silently stripped (keep `GITLAB_MCP_META_PARAM_SCHEMA=opaque`); when `structuredContent` is present, only that JSON reaches the model ([#10334](https://github.com/openai/codex/issues/10334)) |
| Gemini CLI                                                                  | Tool names over 63 characters truncated; Gemini API rejects `$ref`/`$defs` in input schemas                                                                                                                                                                                                                   |
| JetBrains AI Assistant                                                      | Rejects the whole `tools/list` if any tool's `outputSchema` root type is not `object` ([LLM-30555](https://youtrack.jetbrains.com/issue/LLM-30555))                                                                                                                                                           |

## Subscription behavior per client

Client handling of `resources/subscribe` varies more than any other
capability: VS Code attempts to subscribe to every resource it reads (this server accepts only the 26 subscribable kinds and refuses the rest) and routes
`resources/updated` into its file-change pipeline (not into chat); Cursor
sends `resources/subscribe` even to servers advertising `subscribe: false`;
and the Go SDK's client fires `subscriptions/listen` without awaiting the
response, so a server-side refusal never surfaces. Details and the
notification `_meta` contract:
[subscriptions reference](../reference/capabilities/subscriptions.md).

## See Also

- [IDE Configuration](ide-configuration.md) — per-client setup, including the OpenAI Codex section
- [Configuration](../reference/configuration.md) — all environment variables
- [Tool Surfaces](../development/tool-surfaces-and-action-core.md) — how the dynamic, meta, and individual catalogs are projected
