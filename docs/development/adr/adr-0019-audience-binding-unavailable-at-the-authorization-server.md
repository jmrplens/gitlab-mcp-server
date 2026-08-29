# ADR-0019: Audience binding is unavailable at the authorization server

## Status

Accepted — 2026-08-29.

## Context

The MCP authorization specification requires a resource server to validate
that an access token was issued for it:

> MCP servers **MUST** validate that access tokens were issued specifically
> for them as the intended audience, according to
> [RFC 8707](https://datatracker.ietf.org/doc/html/rfc8707), or otherwise
> verify that they are the intended recipient of the token.

It also prohibits token passthrough, defined as a server that

> accepts tokens … without validating that the tokens were properly issued
> **to the MCP server** AND passes them through to the downstream API.

[ADR-0018](adr-0018-authorization-admits-per-action-gating.md) settled who is
admitted and what they may do. This ADR settles the question one layer below
it: whether the credential was meant for this server at all, and what to do
about the fact that the authorization server cannot say.

The setting is unusual and it is what makes the clauses hard to read
mechanically. GitLab is simultaneously **this resource's authorization
server** — `internal/oauth/metadata.go` publishes it as such in the RFC 9728
document — and **its upstream API**. There is no third party. A token this
server forwards upstream is going back to the issuer that minted it.

## Decision

**1. RFC 8707 resource indicators are unavailable, so the MUST cannot be met
by its named mechanism.**

Verified live on 2026-08-29 against
`https://gitlab.com/.well-known/oauth-authorization-server`: the document
advertises `authorization_endpoint`, `token_endpoint`, `introspection_endpoint`,
`revocation_endpoint`, `registration_endpoint`, `jwks_uri`, `userinfo_endpoint`,
`scopes_supported`, `code_challenge_methods_supported`, `grant_types_supported`,
`response_types_supported`, `response_modes_supported`, `subject_types_supported`,
`id_token_signing_alg_values_supported`, `claims_supported`, `claim_types_supported`
and `issuer` — and **no `resource_indicators_supported`**. A client cannot send
a `resource` parameter GitLab will honor, so no token it issues carries an
audience restriction to check. Nothing this server does can change that.

**2. The specification's alternative — "or otherwise verify that they are the
intended recipient" — is implemented, opt-in, and off by default.**

`/oauth/token/info` names the OAuth application a token was minted for, in
`application.uid`. `--oauth-client-uid` (env `OAUTH_CLIENT_UID`) pins the
applications a deployment admits; `internal/oauth.acceptedRecipient` refuses
anything else. It is a comma-separated set rather than a single value because
`--gitlab-url` is repeatable and each published instance has its own
application with its own uid.

The check inverts the fail-open behaviour of the introspection around it, on
purpose. Scope introspection falls back to assuming `api` with a debug log when
neither endpoint answers, so that instances with restricted introspection keep
working. Reusing that shape here would make breaking introspection the way
around the pin, so an unanswered introspection, an absent uid and an unmatched
uid are all refusals. The check runs before the token cache is written, so a
refused token never becomes a cached identity.

It is **off by default** because turning it on refuses personal access tokens
outright — a PAT belongs to no application — and a PAT is a supported credential
here, the one that makes the server usable from CI and from anything that cannot
open a browser.

**3. Forwarding the client's token to GitLab is not the prohibited passthrough
pattern.**

The prohibition has two conjuncts, and the first one does not hold here. It
names a server that accepts tokens *"without validating that the tokens were
properly issued to the MCP server"* **and** passes them to *"the downstream
API"* — the harm being that a resource server launders a credential into a
service that never issued it. In this deployment the downstream API and the
authorization server are the same GitLab: the token sent upstream already is
"a token issued by the upstream authorization server", which is the condition
the specification's own accepted pattern describes. Every token is verified
against the instance it will be used against before any use
(`NewGitLabVerifierFor`), and the cache is keyed on instance and token together
so a credential verified against one published instance cannot pass as identity
on another.

**4. The residual risk is accepted and stated.**

With the pin unset — the default — this server cannot distinguish a token
granted to its own OAuth application from any other GitLab credential the user
holds. What it gains from that token is exactly what the token already carries
at GitLab: no scope is added, no authority is elevated, and every action is
performed as the user, against the instance that issued the credential. An
operator who wants the stronger property, and can accept losing PAT support,
sets `--oauth-client-uid`.

## Consequences

### Positive

- POS-001: The deviation is now a decision with evidence rather than a
  paragraph in a security page. The evidence is reproducible: fetch the
  authorization-server metadata and look for `resource_indicators_supported`.
- POS-002: A deployment that wants recipient verification has it, and it is a
  single flag. The browser inspector at mcp.jmrp.io is the concrete case: it
  holds a token minted for one known application, and pinning that application
  makes every other credential inadmissible.
- POS-003: The refusal is unambiguous at the wire. `acceptedRecipient` wraps
  `auth.ErrInvalidToken`, so the guard answers `401` with the RFC 6750
  challenge rather than a bare error.

### Negative

- NEG-001: Setting the pin refuses personal access tokens. That is the intended
  meaning of the setting — "only tokens minted for my application" — but it
  removes the credential most non-browser clients use, so it is not a default
  and should not become one.
- NEG-002: The pin is only as good as `/oauth/token/info`. An instance that
  restricts that endpoint makes every token unverifiable, and a pinned
  deployment then admits nobody. This is the correct failure direction, and it
  is a real way to lock a deployment out of itself.
- NEG-003: Nothing new is published in the protected-resource metadata. RFC 9728
  defines no field for a recipient pin, so a client cannot discover the
  requirement and will learn about it from a `401`.

### Neutral

- NEU-001: If GitLab adds `resource_indicators_supported`, decisions 1 and 2
  are superseded: the `resource` parameter becomes the mechanism and the
  application pin becomes redundant. That is the revisit trigger — watch the
  authorization-server metadata document, not a release note.

## Alternatives Considered

- **Require the pin by default.** Rejected: it silently breaks every PAT-based
  client, which is most of them, in exchange for a property the specification
  itself offers as the fallback rather than the requirement.
- **Refuse to forward the user's token upstream, and act under a service
  credential instead.** Rejected: it makes the server an authority of its own,
  performing actions no user authorized, and loses per-user GitLab
  authorization — the property that makes the read-only surface in ADR-0018
  meaningful.
- **Publish the pin in the RFC 9728 metadata as a custom field.** Rejected:
  inventing a field is not discovery, and a client that does not know it exists
  is no better off than one that reads the `401`.

## References

- [ADR-0018](adr-0018-authorization-admits-per-action-gating.md) — admission at
  the minimum scope, writes gated per action.
- [Security](../../concepts/security.md) — the operator-facing statement of the
  same deviation.
- [OAuth application setup](../../guides/oauth-app-setup.md) — where an
  application's uid is found.
