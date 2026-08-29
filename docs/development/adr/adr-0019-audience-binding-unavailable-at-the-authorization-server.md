# ADR-0019: Audience binding is unavailable at the authorization server

## Status

Accepted, 2026-08-29.

## Context

The 2026-07-28 security-considerations page carries three sentences that bear
on this, quoted verbatim.

On audience binding, with its own condition attached:

> RFC 8707 Resource Indicators provide critical security benefits by binding
> tokens to their intended audiences **when the Authorization Server supports
> the capability**.

On what a server must accept:

> MCP servers **MUST** only accept tokens specifically intended for themselves
> and **MUST** reject tokens that do not include them in the audience claim or
> otherwise verify that they are the intended recipient of the token.

And on passthrough, with no condition attached at all:

> If the MCP server makes requests to upstream APIs, it may act as an OAuth
> client to them. The access token used at the upstream API is a separate
> token, issued by the upstream authorization server. The MCP server **MUST
> NOT** pass through the token it received from the MCP client.

[ADR-0018](adr-0018-authorization-admits-per-action-gating.md) settled who is
admitted and what they may do. This ADR settles the question one layer below
it: whether the credential was meant for this server at all, and what to do
about the fact that the authorization server cannot say.

The setting is unusual and it is what makes the clauses hard to read
mechanically. GitLab is simultaneously **this resource's authorization
server**, `internal/oauth/metadata.go` publishes it as such in the RFC 9728
document, and **its upstream API**. There is no third party. A token this
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
`id_token_signing_alg_values_supported`, `claims_supported`, `claim_types_supported`,
`token_endpoint_auth_methods_supported` and `issuer`, eighteen keys in all, and
**no `resource_indicators_supported`**. A client cannot send a `resource`
parameter GitLab will honor, so no token it issues carries an audience
restriction to check. Nothing this server does can change that.

The specification anticipates this: the audience-binding sentence conditions its
benefit on the authorization server supporting the capability, and the
requirement it places on servers offers "or otherwise verify that they are the
intended recipient" as the alternative. Decision 2 is that alternative.

**2. The specification's alternative, "or otherwise verify that they are the
intended recipient", is implemented, opt-in, and off by default.**

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
outright, a PAT belongs to no application, and a PAT is a supported credential
here, the one that makes the server usable from CI and from anything that cannot
open a browser.

**3. The token IS passed through. That is a deviation, accepted on the grounds
below, and not a claim of conformance.**

This server sends the client's own bearer to GitLab on every call. The sentence
prohibiting that carries no condition, so there is nothing to argue about at the
level of the letter: we do not comply with it, and an earlier draft of this ADR
claiming otherwise was wrong. It reached that conclusion by answering the
passthrough definition's second conjunct, about the harm, while labelling the
answer as the first, about validation. Decision 4 concedes the first conjunct
outright, so the two could not both stand.

What is true is why the deviation is acceptable here, and it is worth stating
precisely because it does not generalize:

- **The prohibition exists to stop a credential being laundered into a service
  that never issued it.** The sentence's own framing is that the upstream token
  "is a separate token, issued by the upstream authorization server". In this
  deployment the upstream authorization server and the downstream API are the
  same GitLab, and the token was issued by it. Sending it back is not laundering
  it anywhere; it is returning it to its issuer.
- **No authority is gained.** What this server can do with the token is exactly
  what its holder can already do at GitLab: no scope is added, nothing is
  elevated, and every action is performed as that user.
- **Every token is verified against the instance it will be used against**
  before any use (`NewGitLabVerifierFor`), and the cache is keyed on instance
  and token together, so a credential verified against one published instance
  cannot pass as identity on another.

The alternative would be for this server to hold a service credential of its
own and act under it, which is the shape the prohibition protects. That is
worse here: it makes the server an authority, performing actions no user
authorized, and it destroys the per-user GitLab authorization that makes
ADR-0018's read-only surface mean anything.

`docs/concepts/security.md` states the same thing in operator-facing terms,
under "OAuth mode and audience binding (documented deviation)". The two must
keep agreeing; if one is edited, edit both.

**4. The residual risk is accepted and stated.**

With the pin unset (the default), this server cannot distinguish a token
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
- POS-003: The refusal is unambiguous at the wire, and honest. It wraps
  `oauth.ErrUnacceptedRecipient` rather than `auth.ErrInvalidToken`, so the
  guard answers `401` with the RFC 6750 challenge (section 3.1 gives
  `invalid_token` to a token "invalid for other reasons") while saying what is
  actually true: the instance accepts this credential, this deployment does not
  admit the application it belongs to. It is also kept off the
  authentication-failure budget, so a client holding a token the deployment
  does admit cannot be locked out of the address by someone else's
  misconfigured one.

### Negative

- NEG-001: Setting the pin refuses personal access tokens. That is the intended
  meaning of the setting ("only tokens minted for my application"), but it
  removes the credential most non-browser clients use, so it is not a default
  and should not become one.
- NEG-002: The pin is only as good as `/oauth/token/info`. An instance that
  restricts that endpoint makes every token unverifiable, and a pinned
  deployment then admits nobody. This is the correct failure direction, and it
  is a real way to lock a deployment out of itself.
- NEG-003: Nothing new is published in the protected-resource metadata. RFC 9728
  defines no field for a recipient pin, so a client cannot discover the
  requirement in advance and learns about it from the `401`. That answer now
  names the cause and points at `resource_documentation`, which is as close to
  discovery as the RFC allows.

### Neutral

- NEU-001: If GitLab adds `resource_indicators_supported`, decisions 1 and 2
  are superseded: the `resource` parameter becomes the mechanism and the
  application pin becomes redundant. That is the revisit trigger: watch the
  authorization-server metadata document, not a release note.

## Alternatives Considered

- **Require the pin by default.** Rejected: it silently breaks every PAT-based
  client, which is most of them, in exchange for a property the specification
  itself offers as the fallback rather than the requirement.
- **Refuse to forward the user's token upstream, and act under a service
  credential instead.** Rejected: it makes the server an authority of its own,
  performing actions no user authorized, and loses per-user GitLab
  authorization, the property that makes the read-only surface in ADR-0018
  meaningful.
- **Publish the pin in the RFC 9728 metadata as a custom field.** Rejected:
  inventing a field is not discovery, and a client that does not know it exists
  is no better off than one that reads the `401`.

## References

- [ADR-0018](adr-0018-authorization-admits-per-action-gating.md), admission at
  the minimum scope, writes gated per action.
- [Security](../../concepts/security.md), the operator-facing statement of the
  same deviation.
- [OAuth application setup](../../guides/oauth-app-setup.md), where an
  application's uid is found.
