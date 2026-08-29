# Upstream bugs and gaps

Defects and missing capabilities found in projects this server depends on, kept
here so they are contributed back rather than only worked around.

An entry earns its place by being **found from this codebase** — a workaround we
carry, a behaviour a test had to accommodate, a spec clause we cannot satisfy
because the dependency does not expose what it needs. Each one records where the
evidence is, so a contributor does not have to rediscover it.

See the [upstream contribution skill](../../.github/skills/upstream-contribution/)
for the fork → branch → fix → test → MR workflow.

## Status legend

| Status    | Meaning                                                             |
| --------- | ------------------------------------------------------------------- |
| Open      | Reported or ready to report; no upstream fix yet                    |
| Proposed  | We have an MR or issue open                                         |
| Merged    | Fixed upstream; our workaround can be retired at the stated version |
| Declined  | Upstream decided against it; the workaround is permanent            |
| Not a bug | Investigated and found to be correct behaviour                      |

## GitLab (`gitlab-org/gitlab`)

### 403 responses carry no `WWW-Authenticate` header

- **Status**: Open — not yet reported by us.
- **Where**: `lib/api/api_guard.rb`, around the `ForbiddenError` handling.
- **What**: GitLab's own comment states it:
  `# FIXME: ForbiddenError (inherited from Bearer::Forbidden of Rack::Oauth2)
  does not include WWW-Authenticate header, which breaks the standard.`
  RFC 6750 §3 requires a protected resource that refuses a request for
  insufficient scope to return `WWW-Authenticate: Bearer
  error="insufficient_scope"`. GitLab returns the error in the JSON body only.
- **Root cause**: in `rack-oauth2`, only the `Unauthorized` class builds the
  challenge; `Forbidden` does not. Fixable in GitLab's own handler without
  changing the gem.
- **How we found it**: implementing insufficient-scope detection
  (`internal/oauth.isInsufficientScope`). The obvious implementation — parse
  `error="insufficient_scope"` out of the challenge — would have compiled,
  passed a hand-written fake that emitted the header, and never once fired
  against a real GitLab. We detect on the response body instead.
- **Effort**: small. Observable API behaviour change, so it needs a maintainer
  from the authentication area and a changelog entry.
- **Value to us**: none — our detection does not depend on it. This is a
  contribution, not a fix we need.

### No `resource_indicators_supported` in authorization-server metadata

- **Status**: Open — a feature request rather than a defect.
- **Where**: `/.well-known/oauth-authorization-server`.
- **What**: verified live on 2026-08-29 against gitlab.com, the document
  advertises seventeen fields and not this one, so RFC 8707 resource indicators
  are unavailable and a client cannot request an audience-restricted token.
- **Consequence for us**: the MCP authorization specification's audience-binding
  MUST cannot be met by its named mechanism. Recorded as
  [ADR-0019](adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md),
  which implements the specification's "or otherwise verify" alternative.
- **Effort**: large, and it is a product decision rather than a patch.

## GitLab client (`gitlab.com/gitlab-org/api/client-go`)

### Panic unmarshalling an issue with a non-object `milestone`

- **Status**: **Merged** — upstream !3006, shipped in v2.59.1.
- **Retired**: the workaround is gone; the dependency is pinned at v2.60.0.

## MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)

### No keep-alive interval for SSE streams on `StreamableHTTPOptions`

- **Status**: Open — not yet reported by us.
- **What**: the SDK emits keep-alives only on the standalone GET stream
  (`streamable.go`), not on streamed POST responses, and offers no option to
  configure the interval. An idle SSE response therefore puts no bytes on the
  wire, and a proxy's read timeout — nginx's `proxy_read_timeout` is 60s by
  default — severs it. Worse, with nothing written the response headers are not
  flushed either, so the client hangs before the first read rather than after.
- **How we found it**: writing `TestSSEKeepAlive_IdleStreamKeepsBytesOnTheWire`.
  The test hung instead of failing, which is how the header-flush half surfaced.
- **Our workaround**: `sseAwareWriter` in `cmd/server/main.go` emits a comment
  frame every 25s on any response that commits to `text/event-stream`, guarding
  its writes with the same mutex the handler's writes take.
- **Effort**: small for the option; the behaviour change is opt-in.
- **Value to us**: low — the workaround is at our own middleware layer and
  covers every stream, which is more than the requested option would.

## Other

### `go-selfupdate` depends on the deprecated `x/crypto/openpgp`

- **Status**: **Proposed** — issue #57 and PR #58 open upstream.
- **Consequence for us**: the `GO-2026-5932` govulncheck allowlist entry stays
  until it merges. Retire the entry then.
