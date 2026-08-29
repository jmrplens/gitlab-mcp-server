# Upstream bugs and gaps

Defects and missing capabilities found in projects this server depends on, kept
here so they are contributed back rather than only worked around.

**This register is permanent.** An entry is never deleted. When a fix lands
upstream it is marked merged with the version that carries it, and the entry
stays as the record of why the workaround existed and when it could go.

An entry earns its place by being **found from this codebase**: a workaround we
carry, a behaviour a test had to accommodate, a spec clause we cannot satisfy
because the dependency does not expose what it needs. Each one records where the
evidence is, so a contributor does not have to rediscover it.

See the [upstream contribution skill](../../.github/skills/upstream-contribution/)
for the fork, branch, fix, test and MR workflow.

## Writing an entry

**Always link a tracker item, never write a bare reference.** This repository
lives on GitHub and is mirrored to GitLab, and each forge autolinks the other's
reference syntax against *itself*. A bare `!2996` renders on the mirror as a
merge request of the mirror, and a bare `#58` renders on GitHub as an issue of
this repository. Both point at something that does not exist, or worse, at
something unrelated that does.

A markdown link is not reprocessed as a reference by either forge, so put the
project path in the link text and the full URL in the target:

```markdown
[gitlab-org/api/client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996)
[creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58)
```

That stays readable as a mention, resolves from either forge, and creates no
cross-reference anywhere.

## What each entry records

Every entry carries the same five facts, so the state of a contribution is
readable without opening the tracker:

| Field          | Meaning                                                                                                         |
| -------------- | --------------------------------------------------------------------------------------------------------------- |
| **Reported**   | Whether it has been raised upstream at all, with a link to the issue                                            |
| **In review**  | Whether an upstream change was opened for review, with a link. Historical: it stays yes after the change merges |
| **Merged**     | Whether it has landed, and **in which version**. Merged implies Reported and In review are both yes             |
| **Blocking**   | Whether it blocks this MCP server, or only costs us a workaround                                                |
| **Workaround** | Whether we carry one while waiting, where it lives, and what retires it                                         |

## Summary

| # | Project | Issue | Reported | In review | Merged | Blocking | Workaround |
| - | ------- | ----- | -------- | --------- | ------ | -------- | ---------- |
| 1 | gitlab-org/gitlab | [403 carries no `WWW-Authenticate`](#403-responses-carry-no-www-authenticate-header) | No | No | No | No | Yes |
| 2 | gitlab-org/gitlab | [No `resource_indicators_supported`](#no-resource_indicators_supported-in-authorization-server-metadata) | No | No | No | No | Yes |
| 3 | client-go | [Panic unmarshalling an issue](#panic-unmarshalling-an-issue-with-no-id) | Yes | Yes | **Yes, v2.59.1** | Was yes | Retired |
| 4 | client-go | [`UpdateIssueBoardList` cannot decode its own response](#updateissueboardlist-cannot-decode-a-successful-response) | Yes | Yes, open | No | No | Yes |
| 5 | client-go | [`GetNamespace` breaks on a path lookup](#getnamespace-cannot-decode-a-path-based-lookup) | No | No | No | No | Yes |
| 6 | client-go | [`SetFeatureFlagOptions` lacks `omitempty`](#setfeatureflagoptions-fields-lack-omitempty) | No | No | No | No | Yes |
| 7 | client-go | [`ApplicationStatistics` assumes numeric JSON](#applicationstatistics-assumes-numeric-json) | No | No | No | No | Yes |
| 8 | go-sdk | [No SSE keep-alive option](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions) | No | No | No | No | Yes |
| 9 | go-sdk | [A malformed message ends the session](#a-malformed-message-ends-the-session-instead-of-answering--32700) | No | No | No | Was yes | Yes |
| 10 | go-sdk | [The keepalive pings a revision that removed `ping`](#the-keepalive-pings-a-session-serving-a-revision-that-removed-ping) | No | No | No | Was yes | Yes |
| 11 | go-selfupdate | [Deprecated `x/crypto/openpgp`](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp) | Yes | Yes, open | No | No | Yes |
| 12 | codex | [Non-integer `priority` breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call) | Yes | Yes, open | No | Was yes | Yes |

States verified against the upstream trackers on 2026-08-29.

## GitLab (`gitlab-org/gitlab`)

### 403 responses carry no WWW-Authenticate header

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Our detection does not depend on the header, so this is a
  contribution rather than a fix we need.
- **Workaround**: yes. `internal/oauth.isInsufficientScope` reads the response
  body instead of the challenge. It is not a stopgap: the body is the only place
  the distinction appears, so it stays whatever upstream does.

**Where**: `lib/api/api_guard.rb`, around the `ForbiddenError` handling.

**What**: GitLab's own comment states it.
`# FIXME: ForbiddenError (inherited from Bearer::Forbidden of Rack::Oauth2)
does not include WWW-Authenticate header, which breaks the standard.`
RFC 6750 section 3 requires a protected resource that refuses a request for
insufficient scope to return `WWW-Authenticate: Bearer
error="insufficient_scope"`. GitLab returns the error in the JSON body only.

**Root cause**: in `rack-oauth2`, only the `Unauthorized` class builds the
challenge; `Forbidden` does not. Fixable in GitLab's own handler without
changing the gem.

**How we found it**: implementing insufficient-scope detection. The obvious
implementation, parsing `error="insufficient_scope"` out of the challenge, would
have compiled, passed a hand-written fake that emitted the header, and never
once fired against a real GitLab.

**Effort**: small. Observable API behaviour change, so it needs a maintainer
from the authentication area and a changelog entry.

### No resource_indicators_supported in authorization-server metadata

- **Reported**: no. It is a feature request rather than a defect.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, the specification's own alternative. `--oauth-client-uid`
  pins the OAuth applications whose tokens are admitted, compared against
  `application.uid`. Off by default, since enabling it refuses personal access
  tokens.

**Where**: `/.well-known/oauth-authorization-server`.

**What**: verified live on 2026-08-29 against gitlab.com, the document
advertises seventeen fields and not this one, so RFC 8707 resource indicators
are unavailable and a client cannot request an audience-restricted token.

**Consequence for us**: the MCP authorization specification's audience-binding
MUST cannot be met by its named mechanism. Recorded as
[ADR-0019](adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md).

**Effort**: large, and it is a product decision rather than a patch.

## GitLab client (`gitlab.com/gitlab-org/api/client-go`)

### Panic unmarshalling an issue with no id

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!3006](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3006).
- **Merged**: **yes**, on 2026-08-25 into `main`, shipped in **v2.59.1**.
- **Blocking**: it was. The panic took the process down rather than failing one
  call.
- **Workaround**: retired. The dependency is pinned at v2.60.0 and the local
  guard is gone.

Kept here as the record: this is what the round trip looks like when it works.

### UpdateIssueBoardList cannot decode a successful response

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996),
  **still open**, targeting `release-client-3.0`.
- **Merged**: no. It is accepted for the v3 line, so it will not appear in any
  v2 release.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/groupboards.UpdateGroupBoardList` issues
  the `PUT` directly instead of calling the wrapper. Retire it, and the
  `acceptedMissingMethods` entry in `cmd/audit_1to1/internal/actions/analyze.go`,
  once the client-go version this project depends on actually contains the fix,
  not merely when the v3 bump happens. The MR is still open, so it may land in a
  later v3 minor, or not at all; check the release before removing either.

**What**: the group-level wrapper declares `[]*BoardList`, while GitLab returns
the single updated list object, so the wrapper can never unmarshal a successful
response. The project-level equivalent already returns `*BoardList`.

**Note**: the major-version policy in `CLAUDE.md` ties this project's major to
client-go's, so the v3 bump is the moment to *check*. The condition is the fix
being present, not the bump having happened.

### GetNamespace cannot decode a path-based lookup

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/namespaces.Get` issues the request
  directly.

**What**: `GetNamespace` expects a single JSON object, but some GitLab versions
answer a path-based lookup with an array.

**Before reporting**: establish which GitLab versions return the array, so the
report names a reproduction rather than a symptom.

### SetFeatureFlagOptions fields lack omitempty

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/features.Set` builds the request body
  itself.

**What**: the option struct's fields carry no `omitempty`, so empty strings are
serialized and GitLab rejects the request with a "mutually exclusive" error.

**Effort**: small, struct tags plus a test. A good first contribution.

### ApplicationStatistics assumes numeric JSON

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/appstatistics.Get` decodes the response
  itself.

**What**: the struct uses `int64` fields, while some GitLab versions return the
counts as JSON strings, so decoding fails.

**Before reporting**: as with `GetNamespace`, pin down which versions send
strings.

## MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)

### No keep-alive interval for SSE streams on StreamableHTTPOptions

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, and it covers more than the requested option would.
  `sseAwareWriter` in `cmd/server/main.go` emits a comment frame every 25s on
  **any** response that commits to `text/event-stream`, guarding its writes with
  the same mutex the handler's writes take. It would stay even if the option
  landed.

**What**: the SDK emits keep-alives only on the standalone GET stream, not on
streamed POST responses, and offers no option to configure the interval. An idle
SSE response therefore puts no bytes on the wire, and a proxy's read timeout
severs it; nginx's `proxy_read_timeout` is 60s by default. Worse, with nothing
written the response headers are not flushed either, so the client hangs before
the first read rather than after.

**How we found it**: writing `TestSSEKeepAlive_IdleStreamKeepsBytesOnTheWire`.
The test hung instead of failing, which is how the header-flush half surfaced.

### A malformed message ends the session instead of answering -32700

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: it was, on stdio. One client lost its session and its
  accumulated context to a single unparseable line; there was no cross-tenant
  effect, since stdio is one process per client.
- **Workaround**: yes. `resilientStdio` in `cmd/server/stdio.go` filters stdin
  ahead of the SDK, answering a line the read loop would choke on and dropping
  it. This entry first said no workaround was available, on the reasoning that
  the decision sits inside the SDK's read loop; that was wrong. The SDK exposes
  `mcp.IOTransport`, which takes any `Reader` and `Writer`, so the loop can be
  fed a stream that never contains the input it cannot handle. Anything parsing
  as a JSON object with `"jsonrpc":"2.0"` is passed through untouched, since
  deciding what a valid message means is the SDK's job.

**What**: `internal/jsonrpc2/conn.go`'s `readIncoming` breaks its loop on *any*
error from `reader.Read`, so a message that fails to parse is treated exactly
like a closed pipe. The session ends, and on stdio the process exits. Nothing is
written to the client, which sees EOF on a stream it can still write to.

JSON-RPC 2.0 defines `-32700 Parse error` for this case, and the framing here is
one message per line, so the next line is an independent message and
resynchronizing is trivial. Both a line that is not JSON (`{not json`) and one
that parses but carries no `"jsonrpc":"2.0"` (`{"hello":"world"}`) produce it; a
request the server understands but cannot serve — an unknown method, a
nonexistent tool — is correctly answered with an error and the session
continues.

**How we found it**: writing `test/e2e/stdio`. The case was written expecting
the session to survive, and it did not.

**Also fixed by the workaround, unintentionally**: with `mcp.StdioTransport`,
EOF on stdin — a client closing its pipe, which is how every session ends —
produced an error from `server.Run`, and the process exited 1. A clean shutdown
reported failure to whatever supervises it. Under `IOTransport` it exits 0. A
unit test had been passing because of that error rather than the condition it
named, which is how this surfaced.

**Pinned by**: `TestMalformedInput_IsAnsweredAndTheSessionSurvives` in
`test/e2e/stdio`, and `TestResilientStdio_*` in `cmd/server`. The e2e case
asserts the client-visible contract rather than the workaround, so it keeps
passing if the SDK ever fixes this and the filter is removed.

### The keepalive pings a session serving a revision that removed ping

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: it was. A stdio session speaking 2026-07-28 was closed by the
  server after 45 idle seconds.
- **Workaround**: yes, and it is total: `keepAliveFor` in `cmd/server/main.go`
  returns 0 on every transport, so this server never runs the SDK keepalive.

**What**: two clauses of 2026-07-28 are broken by the keepalive. The revision
removes `ping`, so a conformant client cannot answer one; with
`KeepAliveFailureThreshold` at 1 the SDK then closes the session. And on the
ping's timeout `mcp/transport.go`'s `cancelCall` emits
`notifications/cancelled` referencing the ping's own request ID, where the same
revision says a server "MUST NOT send `notifications/cancelled` for any other
purpose" than tearing down a `subscriptions/listen` stream.

The version cannot be gated on from application code either: the SDK starts the
keepalive when the session is created, before any request has revealed which
revision the client speaks. So the SDK is the only place this can be decided.

**How we found it**: the interaction-pattern specification audit. Confirmed on
the wire against a build of the previous code — an unprompted
`{"jsonrpc":"2.0","id":1,"method":"ping"}` on a stdio session that declared
2026-07-28.

**Pinned by**: `TestIdleSession_IsNotClosedByTheServer` in `test/e2e/stdio`,
which fails with a broken pipe when the keepalive is restored.

## OpenAI Codex (`openai/codex`)

### A non-integer annotation priority breaks a tool call

- **Reported**: yes,
  [openai/codex#38979](https://github.com/openai/codex/issues/38979).
- **In review**: yes, the issue is open and labelled `bug`, `mcp`, `CLI`,
  `tool-calls`. No fix has been proposed upstream.
- **Merged**: no.
- **Blocking**: it was. Every tool call failed with "Unexpected response type",
  so the server was unusable from Codex rather than degraded.
- **Workaround**: yes, and it is load-bearing. `internal/clientcompat` detects
  Codex from `clientInfo` and rounds annotation priorities to 0 or 1, which is
  spec-legal and parseable by both. `CLIENT_COMPAT=off` disables it. Retire it
  only once the fixed Codex is widely deployed, not merely released: the
  affected build ships inside ChatGPT.app, so users do not choose their version.

**What**: the Codex builds bundled with ChatGPT.app reject any MCP result whose
`annotations.priority` is a non-integer float. `0.6` fails; `1` or an
audience-only annotation passes. The specification places no such restriction,
and crates.io `rmcp` 3.0.0 parses floats correctly (`Option<f32>`), so the defect
is in the patched bundle rather than in the library.

**How we found it**: every tool call from Codex failed with "Unexpected response
type" and nothing else. Bisected with a Python fake server replaying canned
`CallToolResult` values until the float was the only variable left.

**Not the cause, though it looks like it**: unknown fields. Neither Codex
generation used `deny_unknown_fields`, and that hypothesis cost a day before it
was ruled out.

**Related, and deliberately not worked around**:
[openai/codex#10334](https://github.com/openai/codex/issues/10334) — Codex sends
only `structuredContent` to its model when both are present, dropping the
markdown. We keep emitting both.

## Other

### go-selfupdate depends on the deprecated x/crypto/openpgp

- **Reported**: yes,
  [creativeprojects/go-selfupdate#57](https://github.com/creativeprojects/go-selfupdate/issues/57).
- **In review**: yes,
  [creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58),
  still open.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, the `GO-2026-5932` entry in the govulncheck allowlist.
  Retire it when the PR merges.
