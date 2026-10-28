---
title: "ADR-0027: Text a person reads directly is localized from a compiled catalog; what the model reads and the listed catalog stay English"
status: "Accepted"
date: "2026-10-28"
authors: "jmrplens"
tags: ["i18n", "elicitation", "progress", "configuration", "decision"]
supersedes: ""
superseded_by: ""
---

# ADR-0027: Text a person reads directly is localized from a compiled catalog; what the model reads and the listed catalog stay English

## Status

Accepted, 2026-10-28, closing [issue 1547](https://github.com/jmrplens/gitlab-mcp-server/issues/1547).
The maintainer's decision of 2026-10-27 was to implement the issue, add Spanish, check the
result against the MCP clients that exist, and keep English the default with no change for
anyone who does not set the new option. The contributor reference is
[Localization](../i18n.md).

## Context

Every text the server produced was an English string literal. Most of it is read by the model,
which answers the person in the person's own language whatever it read. Some of it is not: a
client puts the questions of an elicitation, the titles of its form fields and progress
messages in front of a person directly. The documentation site is written in English and
Spanish; the software was not, and the OpenSSF Best Practices silver criterion
`internationalization` asks that software generating text for people be internationalized.

Three facts shaped the decision:

- **No client sends its user's language.** No revision of the MCP specification carries one,
  the Go SDK (v1.8.0) has no field for it, and none of the clients documented to render
  elicitation (VS Code, Claude Code, OpenAI Codex, Goose) sends one; each shows the server's
  text as it arrives. Cursor lists elicitation as supported and documents neither how it renders
  the questions nor what it sends. SEP-2792 adds a per-request language in `_meta`; the core maintainers accepted
  it on 2026-10-17, and it is not merged, not in a published revision and not in an SDK.
  An HTTP `Accept-Language` header is no signal either, since Node clients send `*` when
  nobody sets it, and the machine's locale is the operator's machine rather than the person's.
- **The listed catalog is a contract.** `tools/list`, `prompts/list`, `resources/list` and
  `resources/templates/list` are what strict MCP gateways validate (pure ASCII, no semicolon,
  `make check-gateway-chars`), the prompt catalog is cached as public (`internal/cachehints`),
  and the generated reference, the token footprint and the surface-quality audit compare the
  listing between deployments.
- **Some texts have two readers.** A destructive confirmation and the questions of the pull
  mirror and work item guards are asked of the person and, when the client cannot ask, become
  the refusal the model reads.

## Decision

**A text a client shows a person without the model reading it is rendered from a message
catalog compiled into the binary, in the language the operator chose; everything the model
reads, the listed catalog and everything an operator reads stay English.**

- **What is localized**: the questions and consent summaries of the four `interactive.*`
  flows, the destructive confirmation of the meta and individual surfaces, the two guards'
  questions, the `title` and `description` of every form property, and every progress message.
  What stays English: tool and schema descriptions, results, hints, refusals, cancellation
  results, `instructions`, prompt bodies, the four listings with their titles, the server cards
  and `serverInfo`, startup errors, `--help` and logs.
- **Errors a client shows as they are stay English.** A JSON-RPC `Error.message` is diagnostic
  text an operator searches for, clients log it and forward it to the model, several are
  written before a request context with a language exists (the HTTP gate, the body and line
  limits), and SEP-2792 leaves it a MAY. A tool result with `isError` is read by the model. If
  `Error.message` is ever localized, it will be per request under SEP-2792, with
  `contentLanguage` in `error.data`.
- **The catalog is Go**: `internal/i18n`, standard library only, typed `Key` constants with
  stable dotted values, one map literal per language (`catalog_en.go`, the source, and
  `catalog_es.go`), named placeholders filled by `S`, `N`, `M` and `L`, `Concat` for summaries
  with optional rows, and one-pass substitution. English is what the server sent before, byte
  for byte. Every translation holds exactly the English keys, the same placeholders and the
  same Markdown constructions, and its language's characters.
- **The sinks are typed**: elicitation, progress, the waits and the destructive confirmations
  take `i18n.Text`, which only the catalog produces, or `i18n.Message` where the same text is
  rendered for the model in English. A literal at a person-facing call site does not compile;
  `make check-i18n` closes the remaining doors (`i18n.Verbatim` outside tests, a key that is not
  a catalog constant, an argument not tied to its placeholder, an entry nothing uses), and
  `make check-md-escaping` judges a render by its English template as it judged the
  `fmt.Sprintf` it replaced.
- **The setting**: `GITLAB_MCP_LANGUAGE` and `--language`, both transports, `en` by default
  and `es`, a BCP 47 tag read without regard to case with its region dropped. Any other value
  refuses the start in stdio and HTTP mode alike. The machine's locale is never read, and the
  bare `LANGUAGE`, gettext's, is never read or reported. The language is per process, carried
  on each request's context by `languageMiddleware` and in the configuration shape key. It has
  no tenant register row: it is presentation per deployment and decides nothing about who a
  caller is or what it may hold.
- **SEP-2792 plugs into one place.** When it lands, `languageMiddleware` reads the request's
  language, picks among the supported ones, falls back to the setting, and marks what it
  localized with `contentLanguage`; nothing is kept on the session, and that change adds a
  register row because a client-declared value then changes the output.

## Alternatives considered

- **Keep the texts English and record why.** The issue allowed it. Not taken: the maintainer
  chose on 2026-10-27 to serve a second language, the site already has Spanish readers, and the
  setting changes nothing for a deployment that does not set it.
- **Translate the listing titles too**, which SEP-2792 puts on its SHOULD list for a server
  that honors a request's language. Rejected for an operator setting: Spanish does not fit the
  gateway character policy without dropping its accents, a listing that varies by language needs
  a cache key only SEP-2792 defines, and the model reads the titles. Revisited with SEP-2792 in
  issue 1013.
- **Translate what the model reads.** Rejected: the model already answers in the person's
  language, so translating its input changes its behavior for nothing, and SEP-2792 says as
  much.
- **An embedded JSON catalog, or `golang.org/x/text/message` with `gotext`.** Rejected: a Go
  map literal makes a duplicate or undeclared key a compile error and has no decoding that
  could fail at startup, and x/text adds code generation and a dependency for plural and number
  formatting the proposal excludes. The site's JSON catalogs exist because Astro reads them;
  nothing outside the binary reads this one.
- **Overrides by environment variable**, the approach of `github/github-mcp-server#25`.
  Rejected: it customizes a description, and does not give a second language whole.
- **Choose the language per session or per credential now**, from a header of our own or from
  the GitLab profile's `preferred_language`. Rejected for this change: SEP-2792 forbids tying the
  language to the session, and the profile needs a field client-go does not model, a language
  per pool entry and a register row, none of which serving Spanish needs. The profile remains an
  open question for an issue of its own.
- **Read the machine's locale.** Rejected: it would change the output of every deployment on a
  Spanish machine, against the rule that nothing changes for anyone who does not set the option.

## Consequences

### Positive

- POS-001: A deployment with `GITLAB_MCP_LANGUAGE=es` asks its users in Spanish in every
  client that renders elicitation forms, and reports progress in Spanish, with no change to what
  the model reads.
- POS-002: A deployment that sets nothing is served exactly what it was served before:
  `TestEnglishCatalog_MatchesTheLiteralsItReplaced` holds every English entry to the literal it
  replaced, and the listings are compared byte for byte under no setting, `en` and `es`.
- POS-003: A translation cannot drift from its source in keys, placeholders, Markdown structure
  or characters without a test failing, and a person-facing text cannot be written outside the
  catalog without the compiler or `make check-i18n` refusing it.
- POS-004: Per-request languages are a change to one middleware when SEP-2792 lands.

### Negative

- NEG-001: A hook, an auto-answer rule or a script that recognizes a question by its `message`
  stops matching under `es`. The property keys, the `inputRequests` keys and the option values
  never change, and the documentation says to match on them. On a `2025-11-25` session or an
  earlier one there are no `inputRequests` keys and every yes/no question has the same
  `confirmed` property, so a hook that answers those questions differently can only tell them
  apart by the `message` or by their order, and has to recognize the message in each language
  the deployment may be set to; the documentation says so too.
- NEG-002: In HTTP mode every credential is asked in the same language, since the language is
  the operator's. A per-user language waits for SEP-2792 or for the profile question.
- NEG-003: The listing titles a person sees in some clients stay English under `es`.
- NEG-004: The Claude Desktop bundle does not offer the setting in its `user_config`
  (`mcpb/manifest.json`), and its entries in `server.json` list only what it forwards. Claude
  Desktop renders no elicitation form, so the questions, summaries and field titles never reach
  a person there; the progress messages would be Spanish wherever it shows them, and a bundle
  user who wants that can set `GITLAB_MCP_LANGUAGE` in `~/.gitlab-mcp-server.env`, which the
  bundled server reads like any other stdio process. Offering it in the bundle is a later
  decision, should Claude Desktop render forms or a user ask for it.
- NEG-005: Every new person-facing text needs its Spanish in the same change, which the
  completeness test enforces, and a Spanish Go file is a second exception to the repository's
  English-only rule, named in the language policy.

## Compliance

| Gate                                                                        | What it holds                                                                                                                                |
| --------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test ./internal/i18n/`                                                  | Completeness, placeholder and structure parity, frozen English, the character policy per language, the Spanish glossary, rendering           |
| `make check-i18n` (`cmd/audit_i18n`)                                        | Every render ties its arguments to its template's placeholders, names a catalog key, and every entry is used; `Verbatim` only in tests       |
| `make check-md-escaping` (`cmd/audit_md_escaping`)                          | A render is judged by its English template, so an unescaped value in a translated summary is found as it was in English                      |
| `TestCreateServer_UnsetEnglishAndSpanish_ListTheSameCatalog` (`cmd/server`) | The four listings are identical under no setting, `en` and `es`, so what `make check-gateway-chars` holds in English holds in every language |
| `test/e2e/stdio`, `test/e2e/http`                                           | The real binary asks in each language, refuses a language without a catalog before serving, and never reads the machine's locale             |

## References

- **REF-001**: [Localization](../i18n.md), the contributor reference.
- **REF-002**: [Issue 1547](https://github.com/jmrplens/gitlab-mcp-server/issues/1547) and
  [issue 1013](https://github.com/jmrplens/gitlab-mcp-server/issues/1013), which tracks
  SEP-2792.
- **REF-003**: [SEP-2792](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2792),
  per-request user language.
- **REF-004**: [ADR-0021](adr-0021-captured-response-for-fields-the-sdk-does-not-model.md), the
  way a GitLab profile field client-go does not model would be read, and
  [ADR-0023](adr-0023-tenant-policy-is-declared-once.md), the register a per-request language
  would add a row to.
- **REF-005**: The site: [In which language are the questions asked?](https://docs.jmrp.io/gitlab-mcp-server/capabilities/elicitation/#in-which-language-are-the-questions-asked)
  and [`GITLAB_MCP_LANGUAGE`](https://docs.jmrp.io/gitlab-mcp-server/reference/environment/#tool-surface-and-behavior).
