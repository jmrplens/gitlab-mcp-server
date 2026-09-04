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

## What does not belong here

Our own misconfiguration. An entry was opened here for the SDK keepalive that
pinged sessions serving protocol 2026-07-28, where `ping` is a removed method,
and it was wrong: `ServerOptions.KeepAlive` defaults to zero and the SDK only
starts a keepalive when it is set to something. This project set it to 30
seconds. The defect was real and is fixed, but it was ours, and filing it
upstream would have sent someone looking for a bug in code that was doing what
it was told.

The test to apply before adding an entry: would the behavior happen to a caller
who never configured it? If it takes a setting of ours to reach, it is a bug in
this repository whatever it looks like from inside the debugger.

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
| 4 | client-go | [`UpdateIssueBoardList` cannot decode its own response](#updateissueboardlist-cannot-decode-a-successful-response) | Yes | Yes | **Yes, `release-client-3.0` branch (untagged)** | No | Yes |
| 5 | client-go | [`GetNamespace` breaks on a path lookup](#getnamespace-cannot-decode-a-path-based-lookup) | No | No | No | No | Yes |
| 6 | client-go | [`SetFeatureFlagOptions` lacks `omitempty`](#setfeatureflagoptions-fields-lack-omitempty) | No | No | No | No | Yes |
| 7 | client-go | [`ApplicationStatistics` assumes numeric JSON](#applicationstatistics-assumes-numeric-json) | No | No | No | No | Yes |
| 8 | go-sdk | [No SSE keep-alive option](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions) | No | No | No | No | Yes |
| 9 | go-sdk | [A malformed message ends the session](#a-malformed-message-ends-the-session-instead-of-answering--32700) | No | No | No | Was yes | Yes |
| 10 | go-sdk | [Cannot send `notifications/cancelled` for a listen stream](#application-code-cannot-send-notificationscancelled-for-a-listen-stream) | No | No | No | No | None possible |
| 11 | go-sdk | [Declared, not negotiated, version selects MRTR](#the-declared-protocol-version-not-the-negotiated-one-selects-mrtr) | No | No | No | No | None taken |
| 12 | go-sdk | [A cancelled call is still answered](#a-cancelled-incoming-call-is-still-answered) | No | No | No | No | Partial |
| 13 | go-sdk | [The cancellation reason is discarded](#the-cancellation-reason-is-discarded-before-any-handler-sees-it) | No | No | No | No | None possible |
| 14 | go-sdk | [`Mcp-Name` compared without decoding](#mcp-name-is-compared-without-decoding-the-base64-sentinel) | No | No | No | No | None taken |
| 15 | go-sdk | [Protocol version classified by string ordering](#the-protocol-version-is-classified-by-string-ordering) | No | No | No | No | None taken |
| 16 | go-selfupdate | [Deprecated `x/crypto/openpgp`](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp) | Yes | Yes, open | No | No | Retired |
| 17 | codex | [Non-integer `priority` breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call) | Yes | Yes, open | No | Was yes | Yes |
| 18 | go-sdk | [A receiving middleware cannot read the JSON-RPC id](#a-receiving-middleware-cannot-read-the-json-rpc-request-id) | No | No | No | No | None possible |
| 19 | client-go | [Security mutations discard GraphQL errors](#the-security-attribute-and-category-mutations-discard-graphql-errors) | No | No | No | No | Yes |
| 20 | client-go | [Dependency Firewall lacks `operation` and the enablement endpoint](#the-dependency-firewall-wrapper-is-missing-an-attribute-and-an-endpoint) | No | No | No | No | None |
| 21 | go-sdk | [A middleware cannot ask whether a request carries params](#a-middleware-cannot-ask-whether-a-request-carries-params) | No | No | No | No | Yes |

States verified against the upstream trackers on 2026-09-05.

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
- **Workaround**: retired. The dependency is pinned at v2.62.0 and the local
  guard is gone.

Kept here as the record: this is what the round trip looks like when it works.

### UpdateIssueBoardList cannot decode a successful response

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996),
  targeting `release-client-3.0`.
- **Merged**: **yes**, on 2026-09-02 into `release-client-3.0`. That branch
  carries no tag yet, and it is the v3 line, so no v2 release will contain it.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/groupboards.UpdateGroupBoardList` issues
  the `PUT` directly instead of calling the wrapper. Retire it, and the
  `acceptedMissingMethods` entry in `cmd/audit_1to1/internal/actions/analyze.go`,
  once the client-go version this project depends on actually contains the fix,
  not merely when the v3 bump happens: check that the v3 release being adopted
  really descends from the merge before removing either.

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

### The security attribute and category mutations discard GraphQL errors

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. It is why the eight mutations stay on raw GraphQL, not a
  fix we need to ship.
- **Workaround**: yes. `internal/tools/securityattributes` and
  `internal/tools/securitycategories` issue the same mutations themselves and
  read the errors array through `toolutil.GraphQLTopLevelError`. It retires
  when the wrappers check that array, at which point the handlers can move onto
  them unchanged: the field selections already match exactly.

**Where**: `security_attributes.go` and `security_categories.go`, in all eight
of `CreateSecurityAttributes`, `UpdateSecurityAttribute`,
`DestroySecurityAttribute`, `ProjectUpdateSecurityAttribute`,
`BulkUpdateSecurityAttributes`, `CreateSecurityCategory`,
`UpdateSecurityCategory` and `DestroySecurityCategory`.

**What**: each one unmarshals into a struct that embeds `GenericGraphQLErrors`
and then never reads it. Only the mutation payload's own `errors` field is
checked. `GraphQL.Do` returns an error solely for a non-2xx status, and GitLab
answers a query-level failure with HTTP 200 and a top-level `errors` array, so
a refused mutation reaches the caller as a success: `DestroySecurityAttribute`
and `BulkUpdateSecurityAttributes` return a nil error, `CreateSecurityAttributes`
returns an empty slice with no error, and the update methods degrade to a bare
`ErrNotFound` that throws GitLab's message away.

**Root cause**: an omission rather than a design choice, and the same file set
shows what the fix looks like. `WorkItems.ListWorkItems` checks
`len(result.Errors) != 0` and returns `&GraphQLResponseError{...}`; these eight
need the same three lines.

**How we found it**: auditing whether the wrappers could replace this server's
raw mutations. The field selections match ours character for character, so the
migration looked mechanical until the error paths were compared.

**Effort**: small. Three lines per method plus a test each, and no signature
changes: every one of them already returns an `error`.

### The Dependency Firewall wrapper is missing an attribute and an endpoint

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none. `internal/tools/dependencyfirewall` sends what the
  options struct can carry and documents the omission.

**What**: two gaps against
[the Dependency Firewall API](https://docs.gitlab.com/api/dependency_firewall/).
`EvaluatePackageOptions` carries `Ecosystem`, `Name` and `Version`, but not the
documented optional `operation` attribute (`download` or `upload`, defaulting to
`download`), and nothing on the options struct can carry a body field the type
does not declare. Separately, `SecurityDependencyFirewallService` wraps only
`POST /projects/:id/dependency_firewall/evaluate`, while the same page documents
`GET /projects/:id/dependency_firewall/enablement`, which reports whether the
firewall is on for a project.

**Found from**: exposing the evaluate endpoint as an MCP action
(`project.dependency_firewall_evaluate`), where 1:1 fidelity means every option
field becomes an input field and there was one fewer than the API documents.

**Effort**: small for the attribute (one field plus a test). The enablement
endpoint is a new method with its own result type, and would let a tool answer
"is the firewall even on here" without inferring it from a 404.

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

### A cancelled incoming call is still answered

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: partial and honest rather than a fix. The response cannot be
  suppressed, so what we do is stop it lying: a cancellation is classified as
  "the request was cancelled by the client" instead of falling through to
  "unexpected error", and logged at INFO rather than ERROR. The client behaviour
  is documented in the HTTP guide so a client can expect the late response.

**What**: "Servers receiving cancellation notifications SHOULD ... not send a
response for the cancelled request." `internal/jsonrpc2/conn.go` writes the
response with `c.write(notDone{req.ctx}, response)`, where `notDone` deliberately
strips the cancellation from the context so the write proceeds. Nothing at
application level runs between the handler returning and that write, so no
server built on this SDK can satisfy the clause.

**How we found it**: the interaction-pattern audit. Captured on stdio, two
seconds into a hanging GitLab call: the client's `notifications/cancelled` at
6.306s, and the server's response to that same request id at 6.307s.

### The cancellation reason is discarded before any handler sees it

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none possible. The field is dropped inside the SDK; there is
  no seam to read it from. We log what remains — that the call was cancelled and
  how long it ran.

**What**: "Implementations SHOULD log cancellation reasons for debugging."
`mcp/transport.go` unmarshals `CancelledParams`, uses `params.RequestID` to
cancel the call, and discards `params.Reason`. A hook, or even a logger line at
the preempter, would be enough.

**How we found it**: the interaction-pattern audit. Sending a cancellation with
reason "User requested cancellation" left no trace of that string anywhere in
the server's output.

### The declared protocol version, not the negotiated one, selects MRTR

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none taken, deliberately. Disagreeing with the SDK here would
  be worse than matching it: our gate and `clientSupportsMultiRoundTrip` would
  choose differently for the same session, and the SDK labels the result.

**What**: `initialize` is deprecated in 2026-07-28, so `negotiatedVersion`
(`mcp/shared.go`) caps that handshake at `2025-11-25` — while
`InitializeParams.ProtocolVersion` keeps whatever the client asked for.
`clientSupportsMultiRoundTrip` (`mcp/mrtr.go`) reads the latter, so a client
that sends `initialize` requesting `2026-07-28` is negotiated down to
`2025-11-25` and served multi round-trip requests regardless. The negotiated
version is the one that describes what the session can actually do, and it
should drive both.

**Blast radius is small.** The SDK's own client middleware fulfills
`inputRequests` whatever version it negotiated, so an SDK-based client never
notices. It takes a hand-written client that implements `2025-11-25` strictly,
claims `2026-07-28` in its handshake, and ignores `inputRequests` to be harmed.

**How we found it**: the interaction-pattern specification audit. Observed on
stdio: `initialize -> protocolVersion='2025-11-25'`, and the next `tools/call`
answered `resultType: 'input_required'`.

**Documented in**: `docs/reference/capabilities/elicitation.md`, so the
behaviour is stated where someone writing a client would look.

### Application code cannot send notifications/cancelled for a listen stream

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The client still receives the completion result the SDK
  writes when the stream's handler returns, which tells a conforming client the
  subscription has ended.
- **Workaround**: none, and unlike the malformed-message entry this one really
  has none: the type cannot be constructed at all, so there is nothing to
  interpose.

**What**: 2026-07-28 says a server "MUST send `notifications/cancelled`
referencing a `subscriptions/listen` request ID when it tears down that
subscription stream". `SubscriptionsListenResult` embeds an unexported type, so
application code cannot build the message, and the SDK offers no method that
sends one. A server that ends a subscription can therefore satisfy the graceful
half of the contract and not this one.

**How we found it**: the interaction-pattern specification audit. Reproduced on
stdio at 2026-07-28: a watcher retired after its resource began returning 404,
and the only output was the listen request's own result — no
`notifications/cancelled`, before or after, with the connection still usable.

**Recorded in**: ADR-0015, so the gap is stated where the design is rather than
only here.

### `Mcp-Name` is compared without decoding the base64 sentinel

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Nothing on this server's surface forces the encoded form:
  every tool name is `gitlab_*`, every prompt name is ASCII, and a resource URI
  is a URI, so non-ASCII arrives percent-encoded and matches a plain header.
- **Workaround**: none taken. The header is the SDK's to validate and this
  repository reads `Mcp-Name` nowhere outside tests, so intercepting it would
  mean a second implementation of a rule the SDK already has fifteen lines
  further down.

**What**: SEP-2575 defines a `=?base64?…?=` sentinel for header values that
cannot be sent as plain ASCII, and the transport binding says servers **MUST**
decode encoded values before comparing them to the body. In
`mcp/streamable_headers.go`, `validateParamHeaders` calls `decodeHeaderValue`
(line 425) and `validateMcpHeaders` does not (line 381): it compares
`nameInHeader != nameInBody` raw. A client that encodes a name is answered
`-32020 "header mismatch"` even though its header and body agree.

The direction of the failure is the notable part. Encoding an ASCII-safe value
is permitted, not forbidden, so a client that does it is conforming and is
refused, while a client that sends a non-ASCII name as raw UTF-8 bytes passes:
Go's `net/textproto` hands those through and the string comparison succeeds.
Only the conforming client is rejected.

**How we found it**: the transports specification audit. Reproduced against the
shipped HTTP default with a base64 `Mcp-Name` on `tools/call`, `resources/read`
and `prompts/get`, all three answered `-32020` where the plain-ASCII control was
served. It is upstream by this file's own test: it happens to any caller of
`StreamableHTTPHandler` with no setting of ours.

### A middleware cannot ask whether a request carries params

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no, once known. The check is three lines of `reflect` and this
  server now makes it in one place.
- **Workaround**: `internal/mcpotel.paramsOf` returns `nil` for a `Params`
  interface holding a nil pointer, and the three consumers in that package ask
  through it rather than calling `GetParams` themselves.

**What**: `Params` (mcp/shared.go) declares `GetMeta`, `SetMeta`, `isParams` and
`isNil`. The last exists because a receiving middleware is handed a typed nil
whenever the wire omitted the params member, which `serverMethodInfos` permits
for every list method and for `notifications/initialized`. It is unexported, so
only the SDK can ask. Outside the package, `req.GetParams() != nil` is true for
that value, because an interface holding a nil pointer is not a nil interface,
and `GetMeta` on it dereferences the pointer.

The shape is ordinary: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` is a
complete, valid request that clients really send. Nothing in the SDK's own
documentation for `AddReceivingMiddleware` mentions it, and the accessor named
after the question is the one thing a middleware cannot reach.

**How we found it**: a hosted deployment logged "recovered a panic while
handling a request" a hundred times in a day, on `tools/list`, `prompts/list`,
`resources/list` and `notifications/initialized`, with the stack ending in
`(*ListToolsParams).GetMeta` called from this repository's trace-context
carrier. The bug in the carrier is ours, and a comment in it had asserted the
wire could not produce such a request. Exporting `isNil`, or documenting the
guarantee, would keep the next middleware from writing the same line.

### The protocol version is classified by string ordering

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The window contains only malformed version strings.
- **Workaround**: none taken. The clean seam is a wrapping `mcp.Transport` that
  inspects the decoded request before the SDK session sees it; the stdio filter
  in `cmd/server/stdio.go` is deliberately not that seam, since it exists to do
  the least it can and parsing `params._meta` there would be a second, divergent
  implementation of the protocol. Receiving middleware cannot serve either: the
  initialization gate returns at `mcp/server.go:1901`, before `handleReceive`
  runs the middleware chain at :1927.

**What**: `mcp/shared.go:549` decides whether a request is modern by comparing
version strings with `<`. Anything sorting below `2026-07-28` is treated as a
legacy handshake, so a request carrying an unrecognised version in per-request
`_meta` is answered `method "tools/list" is invalid during session
initialization` with wire code `0`, rather than the
`UnsupportedProtocolVersionError` the versioning page requires. Published
revisions are unaffected, since they are all in the supported list; the window
holds `2026-01-01`, `2025-11-24`, `1900-01-01`, `1.0`, `0` and the empty string.

`1900-01-01` is the versioning page's own worked example of this error, so the
literal illustration in the specification is the case that comes back wrong.

**How we found it**: the lifecycle specification audit, on stdio. HTTP cannot
reach it: the header check rejects a `_meta`-only version with `-32020`, and
`protocolVersionMiddleware` in this repository answers the header case itself.

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
  spec-legal and parseable by both. `GITLAB_MCP_CLIENT_COMPAT=off` disables it. Retire it
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
- **Workaround**: retired. It was the `GO-2026-5932` entry in the govulncheck
  allowlist; removing the self-update subsystem removed the dependency, so the
  advisory no longer reaches any binary and the allowlist is empty again. The
  upstream PR stays worth merging for the module's other users.

### A receiving middleware cannot read the JSON-RPC request id

`mcp.Request` exposes `GetSession`, `GetParams` and `GetExtra`, and nothing that
returns the JSON-RPC `id` of the message being handled. A receiving middleware
therefore cannot see it, and neither can anything built on one.

That is what stops this server from emitting `jsonrpc.request.id`, which the MCP
semantic convention marks Conditionally Required "When the client executes a
request". The attribute is what distinguishes a request span from a notification
span, and the Go semantic-conventions package already ships the key
(`semconv.JSONRPCRequestID`), so the only missing piece is an accessor.

The id is not secret and is already on the wire in both directions; the SDK
decodes it to route the response and then discards it before application code
runs, the same shape as
[the cancellation reason](#the-cancellation-reason-is-discarded-before-any-handler-sees-it). Adding `GetID()` to the `Request` interface, or a
field on `RequestExtra`, would close it.

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. One Conditionally Required attribute is omitted; the span is
  otherwise complete and the metric does not carry the attribute at all.
- **Workaround**: none possible. Nothing in the public API exposes the value.
