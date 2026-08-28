# ADR-0018: Authorization admits at the minimum scope; writes are gated per action

## Status

Accepted — 2026-08-28.

## Context

HTTP mode in `--auth-mode=oauth` demanded the **deployment's** scope at the
door. `oauth.RequiredScope(readOnly, safeMode)` returned `api` for any
deployment that could write, the bearer guard required that exact scope, and
the SDK's `auth.RequireBearerToken` was configured with it too.

The effect, measured against the public endpoint `https://mcp.jmrp.io/gitlab`
(v2.7.5, `--auth-mode=oauth --gitlab-url=https://gitlab.com`): a GitLab OAuth
application registered with **only `read_api`** completed the whole flow —
PKCE, consent screen showing "READ ONLY", token issued — and was then refused
on `initialize`:

```json
{"jsonrpc":"2.0","id":null,"error":{"code":-40300,
 "message":"This token does not carry the api scope that this deployment requires."}}
```

Not on a write. On `initialize`, before `tools/list`. A credential that could
not break anything had no way to read anything either, and the only remedies
were to hand a browser page a write-capable token or to stand up a second
`--read-only` deployment on its own path.

This mattered because the server already knows which actions write: every
`ActionSpec` carries that classification, `--read-only` projects a catalog
from it, and the documentation site renders it per domain. The check existed;
it was in the wrong place.

Two further questions arrived with the same report and are settled here
because they share the subject:

1. Should `initialize` / `tools/list` be readable without any credential, so
   directories and scanners can describe the server? (VerifyMCP scores six of
   seven categories zero on this endpoint, all with "blocked by
   authentication".)
2. Where should `scopes_supported` sit between "least privilege" and "usable
   by a client that never sees the challenge"?

## Decision

**Admission asks for the minimum any action needs; authority is applied per
action.**

- `oauth.MinimumScope` (`read_api`) is what the door checks, through
  `oauth.SatisfiesMinimum`, which treats `api` as covering `read_api`. The
  only credential refused is one carrying no GitLab API scope at all.
- A token that cannot write gets a read-only tool surface.
  `serverpool.applyScopeReadOnly` sets `ServerConfig.ReadOnly` on the pool
  entry when `gitlabclient.WriteCapable` reports the token's scopes cannot
  mutate. The entry is per token, so one client's `read_api` credential never
  narrows another's.
- `oauth.RequiredScope` keeps its name but changes role: it is what the
  `WWW-Authenticate` challenge **recommends**, not what admission requires.
- OAuth mode hands the pool the scopes the verifier already resolved
  (`GetOrCreateWithScopes`). The PAT self endpoint the pool would otherwise
  query does not answer for an OAuth access token, so without this a
  `read_api` OAuth token would look like "authority unknown" and be served the
  full catalog.
- `scopes_supported` lists `api` then `read_api` for a deployment that can
  write, and `read_api` alone for one that cannot.

**`tools/list` stays authenticated.** The MCP authorization specification
(2025-06-18 and the current draft) states that MCP servers "**MUST** validate
access tokens before processing the request … and take all necessary steps to
ensure no data is returned to unauthorized parties", and its error table
admits only 401, 403 and 400. Authorization is optional for a server as a
whole; a server that requires it has no sanctioned partially anonymous
surface. The catalog is published instead through the mechanism designed for
it: the server card at `/server-card`, unauthenticated, carrying every tool,
resource, template and prompt with its schemas.

## Consequences

### Positive

- POS-001: A read-only OAuth application works. The browser inspector at
  mcp.jmrp.io can hold a `read_api` credential and list and call read actions,
  which is what it was registered for.
- POS-002: Any read-only integration — a dashboard, an index, a CI report —
  becomes possible against a deployment that also serves writes, without a
  second deployment.
- POS-003: The write check has one home. Adding an action does not require
  touching an authorization rule; its existing classification is what gates
  it.
- POS-004: A `read_api` token that reaches a legacy-mode deployment as a PAT
  is narrowed the same way, which reconnects `TokenScopes` — detected since
  the catalog-first refactor, consumed by nothing.

### Negative

- NEG-001: A client holding a `read_api` token sees a smaller `tools/list`
  than one holding `api` against the same URL. That is the intended outcome,
  but it means "the tool list" is now a property of the credential, not only
  of the deployment.
- NEG-002: Unknown scopes (`nil`) count as write-capable. A GitLab too old to
  answer the introspection endpoints keeps the full surface rather than
  silently losing every mutating tool. The failure mode moves to GitLab's own
  403 on the write that is actually attempted.
- NEG-003: `scopes_supported` listing `api` first is a deliberate reading of
  the draft's note that the field "is intended to represent the minimal set of
  scopes necessary for basic functionality". Listing only `read_api` would
  match that sentence more literally, but a client that reaches the metadata
  document without ever seeing a challenge is told to request everything in
  `scopes_supported` — and would then be unable to write at all. Revisit if
  step-up authorization (403 `insufficient_scope` on the mutating call, with
  `scope="api"`) is implemented, which is the flow the draft describes for
  exactly this.
- NEG-004: VerifyMCP's score does not move. Six of its seven categories need
  an unauthenticated `tools/list`, which this ADR declines to provide.

### Neutral

- NEU-001: `RequiredScope` keeps its name for a role that is now advisory.
  Renaming it would touch every caller for no behavioural gain, and its
  doc comment states the distinction.

## Alternatives Considered

- **Grant the inspector's application `api`.** Rejected: that application
  exists precisely to keep a write-capable token out of a web page.
- **Run a second `--read-only` deployment on its own path for read-only
  clients.** Rejected: more deployment machinery than the feature is worth,
  and it does not help any other read-only integration.
- **Keep the deployment-wide check and document it.** Rejected: it is not a
  documentation problem. The server can tell reads from writes per action and
  was refusing on a property of itself.
- **Serve `initialize` and `tools/list` anonymously behind a flag.**
  Rejected on the specification, not on taste — see the Decision. The server
  card already covers the legitimate need it would serve.
