# Tenant policy: who a caller is and what it may do here

This is the specification the register in `internal/tenancy` implements, for
[issue 565](https://github.com/jmrplens/gitlab-mcp-server/issues/565). It says once
what a tenant of this server is, what every key a limit can be counted against costs a
caller to mint, which invariants any limit must satisfy, and which refusal channels
exist to say no. [ADR-0023](adr/adr-0023-tenant-policy-is-declared-once.md) records
why the answer lives in one register that the layers read rather than in each layer.

It cites decisions by their register ID (`HLD-001`, `AUB-003`): each ID is one row of
`tenancy.Decisions()`, with its key, class, value source, zero meaning, capacity
behavior, refusals and the symbols that decide and enforce it. The register was built
from a dated record, with line citations and the research behind every claim, that was a
working document kept outside the repository: it is true at the commit it names and at no
other, which is why nothing here cites it.

Nothing in this specification changes a value, a key, a refusal, a message or a
configuration surface. Where the code departs from it today, the departure is a finding,
filed as an issue of its own (see [Findings](#findings)).

## The tenant

**A tenant is the GitLab principal a request runs as: the user GitLab resolves the
presented credential to, on the instance the request selected. Its identity is the pair
(canonical instance URL, GitLab user id).** GitLab authorizes, attributes and throttles
authenticated API traffic per user
([User and IP rate limits](https://docs.gitlab.com/administration/settings/user_and_ip_rate_limits/)),
and MCP leaves the question of what a token denotes to the authorization server, which
here is GitLab. The instance is part of the identity because a user id is unique only
within one instance.

- **Bots and service accounts are tenants of their own.** GitLab makes each project or
  group bot and each service account a user with its own budget
  ([project access tokens](https://docs.gitlab.com/user/project/settings/project_access_tokens/),
  [service accounts](https://docs.gitlab.com/user/profile/service_accounts/)). The server
  never folds a bot into the human who made it: GitLab exposes a bot's owner only to
  administrators, and a generated username is not an API contract.
- **A credential is not a tenant.** Many credentials map to one tenant: a personal access
  token, a fine-grained one, an OAuth access token and an impersonation token all present
  the user they resolve to. A CI job token cannot call `GET /user`, which both admission
  paths ask, and a deploy token cannot call the REST API, so neither is a supported
  credential here.
- **An OAuth application is not a tenant**, nor the key of any allowance: anyone can
  register one without authenticating. `--oauth-client-uid` is an admission filter
  (`ADM-004`) and stays one.
- **Unknown is unknown.** When GitLab returns no user id for a credential, the tenant is
  unknown: not anonymous, not another tenant, and not a reason to refuse, because a GitLab
  outage must not become a lockout. A decision that needs a tenant and meets an unknown
  one falls back to the entry and records that it did. `IDN-008` names the code that
  resolves the identity and applies this rule.
- **Identity comes from the credential on each request**, never from the connection, a
  session id, `clientInfo`, a request id, a mirrored header, an OAuth client id or
  connection history. The address is used only before admission, where no tenant exists
  yet.

### Per mode

| Mode        | Credential source                                         | Admission                                                                                                                                                                                          | Where the tenant is resolved                                                                | Today's per-caller key |
| ----------- | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- | ---------------------- |
| stdio       | `GITLAB_TOKEN` from the environment                       | The operator's configuration, at the `read_api` minimum: a token GitLab accepted below it keeps the process up and every catalog method is answered `-40300` in-band (`ADM-001`)                   | Once per process, after the handshake starts (`IDN-008`)                                    | The process            |
| HTTP legacy | `PRIVATE-TOKEN`, then `Authorization: Bearer` (`IDN-001`) | GitLab did not answer `GET /user` with 401 or 403, at the `read_api` minimum; a 403 refusing a fine-grained token User: Read and a token below the minimum are answered 403, uncharged (`ADM-001`) | When the pool entry is built, and put on the request context by the gate                    | The entry              |
| HTTP OAuth  | `Authorization: Bearer` only (`IDN-001`)                  | Verified against the selected instance, introspected, `read_api` minimum, which a fine-grained token meets (`ADM-002`)                                                                             | The verifier's resolved user, with the instance it verified against, and the pool per entry | The entry              |

On stdio, process, entry and tenant are one. Over HTTP one tenant maps to one entry per
credential per instance, one entry to one tenant or to unknown and to one owner for the
life of its build, and one address to many tenants.

A stdio process reads none of the settings only HTTP mode has, the listener's plain
HTTP opt-in, the pool, the stateful session, the drain, the authentication gate with its budgets, the OAuth verifier, the
trusted origins and the discovery documents' bucket, so
a row of one of them has no stdio key, and a stdio start names each such variable it
finds set instead of refusing a value it would never use (issue 1171). The revalidation
interval is not one of them: the process re-reads its fine-grained token at that
interval and stops with it at zero, so `ADM-009` is keyed on the process there.

### Two axes, and why the tenant does not license a share

Before admission a decision is keyed on the address, the socket or a refused
credential's digest, since no tenant exists yet, and a bound on one request is keyed on
the request; the key table below gives both axes. Every allowance after admission bounds
either the **requester** (a tenant, a credential or something one credential holds) or
the **process**. The process is the only unit no caller can multiply.

**Every key a request yields is mintable, the tenant included.** On a self-managed
instance any user who may create a personal project may create project access tokens on
it, and each is a bot user of its own, with no administrator and no count cap. GitLab has
no per-person key one person cannot multiply. So the tenant is the unit of identity,
attribution and authority, and never a unit a share of a process-wide resource can
safely be granted to. This is what [issue 540](https://github.com/jmrplens/gitlab-mcp-server/issues/540)
found about the token and [issue 561](https://github.com/jmrplens/gitlab-mcp-server/issues/561)
found again about the pool; it holds of the user too, one level up.

The two earlier positions are therefore both right, about different questions: the user
is the right identity, because GitLab counts many tokens of one user as one; and the user
is as dividable as a token when what is at stake is a share, because a bot costs a role
any self-managed user already holds.

## Keys, and what minting one costs

A limit counts against one key of this vocabulary (`tenancy.Key`). A limit that needs a
key the vocabulary lacks adds it there, with its mint cost and the evidence for it, which
is how the mintable-key question gets answered once. `Key.Evidence()` holds the sentence
below with its sources, and a refusal of a share quotes it.

| Key           | What it is                                                                      | Axis             | Mint cost to the caller                                                                                                                                                                                  |
| ------------- | ------------------------------------------------------------------------------- | ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `request`     | One request, argument set or watched URI                                        | request          | Nothing                                                                                                                                                                                                  |
| `credential`  | The raw token a request presents                                                | requester        | Another credential of the same user: a personal access token through the UI, no administrator, no cap, no creation rate limit                                                                            |
| `entry`       | One credential on one instance, the pool entry                                  | requester        | As a credential; several published instances do not multiply keys, but under `--allow-any-gitlab-url` (`DST-002`, loopback only) the caller names the instance and mints one entry per instance it names |
| `owner`       | The random name of one entry build                                              | requester        | As an entry                                                                                                                                                                                              |
| `tenant`      | (canonical instance URL, GitLab user id)                                        | requester        | Another GitLab user: a project bot on self-managed, a paid namespace or a service account on GitLab.com                                                                                                  |
| `verified`    | (instance, token) as the OAuth identity cache keys it                           | requester        | As a credential                                                                                                                                                                                          |
| `application` | The OAuth application a token was issued to                                     | requester        | Nothing: dynamic registration needs no authentication                                                                                                                                                    |
| `session`     | An SDK session id, recorded to an owner                                         | requester        | Nothing: any `initialize` opens one                                                                                                                                                                      |
| `address`     | The charged client address: a whole IPv4 address, an IPv6 address's fixed `/64` | before admission | Address rotation, which inside one `/64` buys nothing; behind a trusted proxy that copies a caller's header, the header value; on a unix socket the one key `unix`, unless `unix` is a trusted proxy     |
| `source`      | The socket peer, every header ignored, keyed as the address is                  | before admission | Address rotation only                                                                                                                                                                                    |
| `network`     | The IPv6 `/48` a charged client address lies in                                 | before admission | Another `/48`, a new assignment; an IPv4 address and a unix-socket peer have none                                                                                                                        |
| `refused`     | A credential GitLab refused, held as a digest                                   | before admission | Nothing: an invented string                                                                                                                                                                              |
| `unbound`     | Every request no credential was bound to                                        | process          | Not mintable                                                                                                                                                                                             |
| `process`     | The running binary                                                              | process          | Not mintable                                                                                                                                                                                             |
| `deployment`  | The operator's configuration                                                    | process          | Not mintable by a caller                                                                                                                                                                                 |

The per-request carrier and the configuration shape are deliberately not keys: nothing
is counted against the first, and the second keys a catalog cache that `INV-010`
governs. The symbols that compute each key (a hash, a host canonicalization, an owner
minted from the operating system's entropy) are declared by `Key.Derivation()` and not
moved: they are identity mechanics rather than policy.

## Alignment classes

Each decision carries one class, which says whether its key agrees with the tenant
definition given the reason the code states for it. The class is that of the HTTP key;
on stdio every per-caller key is the process.

| Class | Meaning                                                                        | Disagrees with the tenant definition? |
| ----- | ------------------------------------------------------------------------------ | ------------------------------------- |
| T     | Keyed on the tenant itself                                                     | No                                    |
| E     | A property of the tenant or instance, resolved with the entry's own credential | No, in effect                         |
| C     | A property of the credential: validity, authority, admission                   | No                                    |
| R     | Bounds or ends something the entry itself holds                                | No                                    |
| Q     | Scoped to a request, a session or a URI                                        | Not a tenant allowance                |
| A     | Before admission, where no tenant exists yet                                   | Not applicable                        |
| P     | The process or the deployment                                                  | Not a tenant allowance, by design     |
| U     | An allowance keyed on the entry whose stated reason names no unit              | No contradiction recorded             |
| D     | An allowance keyed on the entry whose reason is per user or per process        | **Yes, and it carries a finding**     |

The class is computed rather than asserted: `Decision.Disagrees()` compares the unit of
what a row's reason cites with the unit of its key, only for allowances (a ceiling, a
rate, a budget or a share) on a requester key, and a reason about the process is met by a
process partner beside it. `Validate` holds the declared class to that answer. No
decision is class D. `RTC-001` on HTTP, `RTC-005` and `HLD-003` were: each is keyed
finer than the tenant, so a tenant with several credentials holds several units of it,
and each gave a reason about a GitLab user. Moving any of them to the tenant would
still leave it dividable by bots, so issue 955 kept them on the entry and stated each
reason as what the entry bounds, one credential's own requests or watchers, which
makes them class R. `RTC-003`, the listing bucket, was the fourth until `RTC-007` gave
it a process partner (issue 951): its reason is still the processor, and a partner
keyed on the process is what meets it, so it is class R, as `HLD-001` is.

A row carries two units for its reason: the unit the quoted reason names in its own
words, and the unit of what it cites. They differ for no row. They differed for
`RTC-001`, whose comment named GitLab's per-token limits while GitLab keeps the limit
it cited per user, a misstatement that was itself a finding until issue 955 restated
the reason. A ceiling a comment calls "fairness" is not class D on that account, since
fairness names no unit; the defect is `INV-003`'s vocabulary rule, which `HLD-001`
broke until issue 960 stated its reason in the entry's own unit.

## Invariants any limit must satisfy

A new limit, or a change to an existing one, meets every invariant below. Existing code
that does not is a finding, filed as an issue; nothing here requires it to change.
`Validate` holds a row to eleven of them, and to two items of the
[validation checklist](#validation-checklist) that read them for what a row holds and
where its value comes from, in three ways:

- **With no exception**: a share on any mintable key (`INV-003`); a charge to anything but
  a budget before admission (`INV-007`); a refusal on a channel its method cannot carry,
  or with a status, a code, a `Retry-After` or a challenge the channel forbids, a code
  in the legacy `-32000` range among them (`INV-011`); a process partner that is
  configurable or not keyed on the process, and a reason about the process on a row that
  does not say it protects one (`INV-004`); a valued row that does not say what zero means (`INV-015`); a variable without the
  `GITLAB_MCP_` prefix, or a configurable value with no flag, variable or malformed-value
  policy, a value only stdio's environment reaches (`StdioEnvOnly`, issue 957) with
  a flag, with no variable or malformed-value policy, or on a row that does not exist on
  stdio, and a row with no value source of its own, a request bound or a rule, that names
  a flag or a variable and no malformed-value policy (issue 1170) (`INV-017`); a behavior
  at capacity the capacity rule does not allow for the
  row's kind, key and table, which is how `INV-005` and `INV-010` are read for what each
  structure holds (`VAL-006`, issue 958; see [Validation checklist](#validation-checklist));
  and a value no operator setting reaches, a constant or a Go option's, that does not say
  why, or such a reason on a value a setting reaches or one derived from another
  (`VAL-009`, issue 958).
- **With a recorded decision**: a holding taken across keys (`INV-005`); a process
  partner switched off with the per-key limit it stands beside (`INV-015`), which is the
  one zero another row decides that passes without a finding (`RTC-007`, issue 951); a
  zero refused at startup or replaced by the default, the row's own or one of its
  settings' (`INV-015`), which is how the exceptions issue 958 states pass (`POL-001`,
  `RTC-001`'s burst, `ADM-005`, `ADM-008`, `HLD-003`); and a value that does not parse
  and does not refuse startup, warning and keeping the default, skipping the entry or
  reading as off (`INV-017`), which is how the exceptions issue 957 states pass
  (`HLD-001`, `RQB-004`, `IDN-014`, `AUT-006`, `DST-003`). The decision is the one the
  row's `OffWithBy`, `ZeroBy` or `MalformedBy` names, among those it records: a row
  whose decisions are about something else does not pass that way, a `ZeroBy` on a row
  whose every zero means off decides nothing and is refused, and so is a `MalformedBy`
  on a row whose malformed value refuses startup or that has none.
- **With a finding recorded for the invariant**: a per-key ceiling on a process resource
  with no partner (`INV-004`); a table keyed on a mintable value with no capacity
  (`INV-010`); an in-band code of the server's own that mirrors no status the gate
  answers its class of next action with (`INV-012`); any other zero that does not mean
  off (`INV-015`); a reason whose unit differs from its key or misstates its own
  (`INV-016`); a value only an environment variable reaches in HTTP mode, and any other
  malformed value that does not refuse startup (`INV-017`);
  and a ceiling nothing bounds (`INV-018`).

- **INV-001 Identity from the credential.** A limit derives who a caller is from the
  credential on the request, never from the connection, a session id, `clientInfo`, a
  request id, a mirrored header or an OAuth client id. The address may key a budget only
  before admission.
- **INV-002 Unattributed fails closed.** A request no credential was bound to never falls
  back to any tenant's allowance or client (`IDN-007`).
- **INV-003 No share on a mintable key.** No share, reserve, quota guarantee or fairness
  promise is granted to a key a caller can mint, which is every key a request yields. A
  per-key number exists only as a ceiling on what one key consumes, and its code,
  comments and documentation do not describe it as fairness.
- **INV-004 A process partner for every per-key ceiling on a process resource.** A
  per-key ceiling that protects a process-wide resource has a process-wide ceiling beside
  it, and that partner is not configurable, because an operator who can raise the shared
  number can undo the bound (`HLD-001` with `HLD-002`, `HLD-003` with `HLD-004`, `RTC-003`
  with `RTC-007`). The pool size (`POL-001`) is not such a partner: it bounds memory the
  operator provisions. A key before admission is held to this as one after it is, since
  an address is as mintable as a credential: `RTC-008`, the discovery documents' bucket on
  the address, stands alone by issue 1053's decision, which F-36 records.
- **INV-005 Across keys, refuse; never take.** A holding that belongs to one key is not
  evicted to admit another key. Two things are taken across keys, each by a recorded
  decision, never growing past its bound and never refusing the newcomer: the pool
  entry, the quiet one first (`POL-001`, `POL-002`, ADR-0020), and the verified OAuth
  identity, the one used least recently first (`ADM-005`, issue 950). Refusing a
  newcomer there would refuse a credential GitLab has just accepted; what the identity
  taken costs its credential is one more verification, which `ADM-014` bounds, and
  while its address is blocked the exemption a cached identity gives it.
- **INV-006 A refusal costs nothing already held.** Refusing a request never releases,
  evicts or demotes anything the caller already holds.
- **INV-007 What was not judged is not charged.** A failure the server could not
  attribute to the caller's credential (an upstream failure, an unanswered introspection,
  probe saturation, insufficient scope on a genuine credential, an unadmitted application,
  a misaddressed instance, an untrusted origin or an undeclared host) is not charged to any
  budget. `tenancy.Failures()` lists every refusal the authentication paths return and
  which of them are charged; `ValidateFailures` refuses a charged failure the caller did
  not cause.
- **INV-008 Admission at the minimum, authority per action.** A limit does not raise the
  admission minimum; authority is applied per action; unknown scopes count as
  write-capable; a detected tier is the highest paid plan found (ADR-0018). A token
  carrying `read_api` and not `api` is served the actions whose requests GitLab accepts
  from `read_api`, derived per action from what each sends, which is not the operator's
  read-only surface (`AUT-001`, ADR-0026; the operator's is `AUT-004`). The one
  exception, recorded under issue 952, is a tier neither the license, a namespace plan
  nor, on GitLab.com, a group membership answers for, which is Free: that is the truth on
  a CE build and on an unlicensed enterprise one, and an enterprise build that could not
  read its tier warns, naming `GITLAB_MCP_TIER` and `--tier` (`AUT-003`, issue 900). A
  fine-grained token's scope list, the single value `granular`, is unknown scopes, and
  its grant decides per action what it is shown and may call, judged against what GitLab
  declares (`AUT-007`, `AUT-008`, ADR-0024); an action the generated table has no row for
  is unknown authority, listed and callable.
- **INV-009 The surface varies only with the authorization.** A listing may vary with the
  authorization on the request and never with the connection or other requests on it, and
  a listing is never shrunk to express a refusal.
- **INV-010 Caller-chosen values never grow a structure that does not evict.** A cache or
  table keyed on a caller-chosen value either evicts or refuses new keys at a bound, and
  the capacity rule says which it may do (see [Validation checklist](#validation-checklist),
  issue 958). The shape key holds nothing about the credential's identity beyond what
  changes the catalog.
- **INV-011 Every refusal uses a channel the protocol and the SDK carry** (see
  [Refusal channels](#refusal-channels)).
- **INV-012 One class and one next action per refusal, and the code the standard gives
  it.** A refusal belongs to exactly one class of next action and says which. Its code is
  the one the standard assigns to what it says: a refusal telling the caller it made too
  many requests or holds too many is HTTP 429 (RFC 6585) with `Retry-After`, mirrored
  in-band as `-42900`, and one saying the server cannot serve it right now is 503 (RFC
  9110), mirrored as `-50300`. An in-band code of the server's own is the status the gate
  answers the same class with, multiplied by `-100` (issue 956); a code JSON-RPC defines
  (`-32603` for a request no credential was bound to) is GitLab's or the protocol's, not
  the server's, and is exempt.
- **INV-013 Telemetry names the reason, never the caller.** Refusal telemetry carries the
  reason, never the address or the identity, and `/health` carries no pool state.
- **INV-014 Only the pool decides who chose the destination** (ADR-0022).
- **INV-015 Zero means off.** A limit of zero means no limit and never refuses
  everything; switching a ceiling off keeps any count another decision depends on; and
  switching one limit off does not switch off another, unless a recorded decision makes
  a process partner follow the per-key limit it stands beside: the listing bucket's
  partner (`RTC-007`) is off wherever `RTC-003` is, which is when `--rate-limit-rps` is
  `0`, so a deployment that turned rate limiting off has turned all of it off (issue 951).
  The listen and watcher partners keep counting when their per-key ceilings are off.
  Where zero must not mean off, the exception is stated with its reason and the decision
  that made it, never left as a silent normalization (issue 958): the pool size refuses a
  zero, since a pool with no bound would grow with every credential presented and one
  that kept no entry could serve nothing (`POL-001`); the tool-call bucket's burst
  refuses a zero beside a positive rate, the rate being the setting that says off
  (`RTC-001`); the OAuth cache TTL refuses a zero or a negative value through its flag
  and its variable alike, in either authentication mode, since the verifier stamps the
  TTL into every token's expiration, so a zero would expire each token as it is verified
  (`ADM-005`); and the credential age ceiling and the per-credential watcher ceiling,
  which no operator sets, keep their default for a Go option's zero, the first because
  it exists so that every credential is re-checked whatever else is off (`ADM-008`) and
  the second because off would leave one credential's watchers bounded only by the
  process's (`HLD-003`). A row whose settings mean different things by zero states each
  that differs once (`SettingZeros`), and its own `Zero` is the meaning of the rest.
- **INV-016 Every limit states its key, mint cost and reason, and the units agree.** The
  reason's unit matches the key, or the mismatch is recorded as a finding before the limit
  merges, and the reason states its own unit correctly.
- **INV-017 Configuration goes through the configuration package.** A configurable limit
  is read through `config.Getenv` or `config.TrimmedGetenv` with its name in
  `prefixedNames`, has a flag in HTTP mode, and states its malformed-value policy, which
  refuses startup (issue 957, as `internal/config` already refused a value it could not
  parse). Where a value that does not parse must not refuse startup, the exception is
  stated with its reason and the decision that made it (`MalformedBy`), never left
  implicit: the listen ceiling and the stdio line limit warn and keep their default,
  since stdio configuration arrives through the environment of whatever launched the
  process and a mistyped number must not take the client down with the server
  (`HLD-001`, `RQB-004`); the tool name and the action as metric dimensions fall back to
  `auto`, since telemetry observes the server rather than serving its clients
  (`IDN-014`); a directory of a local allow-list that does not resolve is skipped and the
  rest kept, since an entry that names nothing widens nothing (`AUT-006`); and the
  private-instance opt-out reads a value it does not parse as off, so a typo never turns
  the destination guard off (`DST-003`). The flag is asked of a setting in HTTP mode,
  and a setting HTTP mode never reads has none to have: the local allow-lists are
  consulted once a caller-supplied path is admitted, which a server reached over HTTP
  refuses first (`RQB-008`), so their row's source is `StdioEnvOnly`, a value only
  stdio's environment reaches, which departs from nothing (`AUT-006`, issue 957), where
  a value only an environment variable reaches in HTTP mode (`EnvOnly`) still records
  the departure.
- **INV-018 Bound the process on the process.** A bound whose purpose is to protect the
  process, or an upstream budget the whole deployment shares, is keyed on the process.
- **INV-019 No cross-tenant observation.** No tenant observes another's data, watch state
  or the existence of its traffic. Two one-bit disclosures are the accepted exceptions:
  `credential_evicted` (ADR-0020), and the refusal of a bound keyed on the process
  (`HLD-002`, `HLD-004`, `HLD-010`, `HLD-011`, `RTC-007`, `ADM-014`), which tells a
  caller that is still under a bound of its own beside it, or that knows the upstream to
  be healthy, that the process has reached its bound, and so that others are holding,
  spending or verifying against it, and tells a caller refused by `HLD-010` or
  `HLD-011`, which have no such bound, that the process is full (issues 951 and 950,
  ADR-0023 NEG-007). Neither carries a count of
  what others hold or an identity, and neither says more than a caller could infer from
  its own count or its own wait: the stream ceilings name the scope that refused, which a
  caller counting its own streams knows already, `RTC-007` answers in `RTC-003`'s words
  and `ADM-014` in the words `ADM-002` uses for a verification with no verdict, so only
  the log line says which bound refused. `HLD-010` and `HLD-011` have no per-caller
  ceiling beside them, so any refusal of either says the process is full whatever its
  words, and both use the same words, which say only that the server is busy, naming no
  bound, no figure and no caller. That the process is full is all such a refusal
  establishes: one credential can fill `HLD-010` by itself, and `HLD-011` where the
  descriptor limit is small or the rate limit is off, and the refusal names no figure, so
  it is consistent with other callers holding slots without showing that they do, and a
  caller can conclude that they do only from its own count, plainly when it holds none.
  They fall under this exception rather than needing one of their own, which issue
  951 decided: the bit is at most the one the exception already accepts, and no
  per-caller ceiling is set beside them to hide it, since a number on a key a caller can
  mint multiplies with every token it mints (`INV-003`).
- **INV-020 Endings name their cause from a closed vocabulary**, and a removal path added
  without a decision produces no reason rather than the nearest one.
- **INV-021 A change of policy is its own change.** A change to a limit's key, value,
  channel, configurability or zero meaning is made in a change of its own, with a
  measurement from `make bench-fairness` where it claims to protect one population from
  another.

## Refusal channels

A limit whose refusal no client can see is not the limit its author meant, so the channel
is part of the decision. `tenancy.Carriages()` is the matrix below as data, and it
describes go-sdk as this server builds against it. It is checked against the SDK rather
than asserted by it: `internal/tenancy/channels_integration_test.go` drives the pinned
go-sdk in process through every row whose method the SDK owns, in both eras, over three
transports: the in-memory one, which carries what stdio carries, and the streamable HTTP
handler on `httptest` answering with an event stream and with a JSON body. Over HTTP a
2026-07-28 client meets a stateless handler, which is how `cmd/server` serves that
revision by default, and a 2025-11-25 client a stateful session, the only way a client of
that revision receives a notification; the expiry row is driven over HTTP alone, since
only the handler times a session out. In an era a row holds in, every channel it lists
must be carried. In either era, a channel the test attempts that the SDK carries must be
one the matrix lists for that method and era, so a row confined to one era is held in the
other too: the one place go-sdk carries a method outside its row's era,
`subscriptions/listen` on a 2025-11-25 session, which no client of that revision sends, is
declared in the test with that reason, and the declaration fails once the SDK stops bearing
it out. A failure names the row, the era, the transport, the channel expected and the
channel observed, including a failure that kept the row from being observed at all.

That second direction holds only for a channel the test attempts. Every channel is
attempted on every request method except three: a gate status, which is written in front
of the SDK, a refusal to start, which no method carries, and a session close, which the SDK
lets a server send in answer to any request and which the matrix gives to the expiry and
eviction row alone; and a narrowing (`Withheld`, `Absent`, `Unknown`) is attempted on the
two tool methods alone, since the SDK serves whatever surface it is given. For those the
test cannot catch the SDK carrying a channel a row leaves out, and it fails when a channel
of the register is neither attempted nor declared as not attempted, so the list cannot
silently grow. A channel counts as carried only when what the server sent arrives intact,
so the SDK's own `-32601` for a method it removed from an era does not count as the
server's refusal reaching the client. The gate and startup rows describe this server's own
layers and are not driven, and neither is the eviction method, whose row is driven through
expiry. An SDK upgrade that changes what is carried therefore fails its own pull request,
which edits the matrix in the same change. The wording is part of a refusal too: a client may
recognize one only by its stable leading text (a refused `tools/call` by
`toolutil.RateLimitRefusalPrefix`, which `cmd/bench_resources` reads as well), so that
text, a row's `Prefix`, belongs to the wire shape with the code, the status and the
headers.

| Channel (`tenancy.Channel`) | What reaches the caller                                                                        | Carried for                                          |
| --------------------------- | ---------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `Gate`                      | An HTTP status from the gate in front of the SDK, with a JSON-RPC body carrying the request id | Every request, before the SDK sees it                |
| `RPC`                       | An in-band JSON-RPC error                                                                      | Every method a client sends                          |
| `ToolError`                 | A result with `isError`                                                                        | `tools/call` only, the one result with an error flag |
| `EmptyCompletion`           | A completion with no values                                                                    | `completion/complete` only                           |
| `Withheld`                  | A narrowed surface whose answer names the cause                                                | `tools/call`                                         |
| `Absent`                    | A narrowed surface where the action or tool is missing                                         | `tools/list`, `tools/call`                           |
| `Unknown`                   | A narrowed surface whose answer calls the action unknown                                       | `tools/call`                                         |
| `ListenEnd`                 | A `subscriptions/listen` ended with a completion result carrying a watch-end reason            | `subscriptions/listen`, protocol 2026-07-28          |
| `SessionClose`              | A stateful session closed; later requests get 404                                              | Stateful sessions, protocol 2025-11-25               |
| `Silent`                    | No visible refusal: uncounted, delayed or rebuilt                                              | The gate, and resource-updated notifications         |
| `Startup`                   | The process refuses to start                                                                   | Startup                                              |

What the protocol and the SDK leave a server:

- **No "retry later" code exists in MCP.** A delay is HTTP (`Retry-After` on a 429 or a
  503, from the gate only) or prose a model reads.
- **Once the SDK handler runs, the only statuses an in-band error produces are 404 and
  400**, and only on protocol 2026-07-28. Any other status (401, 403, 413, 429, 503) is
  available only in the gate in front of the SDK, whose body is a JSON-RPC error.
- **An in-band error carries a JSON-RPC code**, never a plain Go error, which a client
  receives as code 0. It never uses `-32601` to explain itself, since the SDK replaces that
  message.
- **A code this server allocates sits outside the JSON-RPC reserved range.** The gate's
  codes mirror their status multiplied by -100 (`-40100`, `-40300`, `-42900`, `-50300`),
  and an in-band code mirrors the status the gate answers the same refusal with: `-42900`
  where the caller made or holds too many, `-50300` where the server cannot serve it right
  now. Nothing is emitted in `-32020` to `-32099` that MCP does not define there, nor in
  the legacy sub-range `-32000` to `-32019`, where MCP 2026-07-28 says new codes must not
  be allocated; the listen and watcher ceilings and the readiness gate refused with
  `-32000` until issue 956 retired it.
- **A `subscriptions/listen` refusal is made before the acknowledgment**, and a modern Go
  SDK client never observes it.
- **Ending is not refusing.** A conformant client comes back after a session closes or a
  listen ends, so neither is ever the only answer to a condition the caller must act on.
- **A refusal is never a notification the client did not ask for**, a server-initiated
  request, or `notifications/cancelled`.
- **On stdio there is no status**: only in-band errors, tool errors and empty completions.

The same test holds the half of that list that is about the SDK, over the same three
transports unless this says otherwise: an in-band error takes no status but 404 and 400,
and those at 2026-07-28 alone, whichever kind of HTTP answer carries it; a plain Go error
arrives as code 0, a wrapped JSON-RPC error keeps its code and takes the outer text, and a
`-32601` loses its text to the SDK's own; every in-band refusal the register declares
reaches the client with its code and its text, `Prefix` included; and a listen is refused
before the acknowledgment, which a 2026-07-28 Go SDK client, driven in memory, never sees.
It also holds three facts other decisions rest on without being rows:

- **A typed nil result is not a refusal.** A receiving middleware that answers with one
  and no error gets a `null` result at 2025-11-25, and at 2026-07-28 a panic after the
  middleware chain has returned, which ends the process. No middleware can recover it: the
  test answers it in a child process of its own, in memory, whose outermost middleware
  recovers any panic the chain raises, and the process still ends. A refusal is therefore
  always an error.
- **Load shedding is not a "retry later".** A handler that answers with an empty
  input-request map gets a code 0 `the server is busy, retry later` at 2025-11-25, and at
  2026-07-28 an `input_required` result that a Go SDK client, driven in memory, retries
  three times and then turns into a local error the model never reads. No row carries it.
- **A listen is ended through its handler's context.** Application code can build a
  `SubscriptionsListenResult`, and one a middleware returns instead of calling the SDK's
  handler is sent, but only in place of that handler, before anything is subscribed or
  acknowledged. A stream the handler has acknowledged is answered only when the handler's
  context ends, with the SDK's own result, which a middleware can still stamp with a
  watch-end reason: that is the `ListenEnd` channel.

### Where a refused caller learns what to do

Every refusal names one class of next action (`tenancy.Answer`). The table gives, for each,
every channel a row of the register declares with it; `tenancy.Decisions()` is the list
itself, and a row that adds a channel adds it here.

| Answer           | Channels the rows declare                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Retry later      | A gate 429 with `Retry-After`; a gate 503, with `Retry-After` where GitLab's verification failed, no verification slot came free or the process holds as many requests as it serves at once; in-band `-42900`, `-50300`, or `-32603` for a request no credential was bound to; a tool error saying to back off, or saying the call could not be attributed; an empty completion; a listen ended with `shutdown`                                                                                                                                                                                |
| Reauthorize      | A gate 401 with a challenge, `invalid_token` where GitLab refused the credential; a listen ended with `credential_revoked`                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Widen the scope  | A gate 403 with `insufficient_scope`; a gate 403 for a fine-grained token GitLab accepted and refused User: Read, uncharged and remembered; a gate 403 for a token GitLab accepted below the `read_api` minimum, uncharged and remembered; on stdio, every catalog method refused in-band with `-40300` for a process token below that minimum; a listen ended with `credential_insufficient`; a surface narrowed by the credential's scope; a surface narrowed for a fine-grained token, a call to which names the permission its grant lacks or why no fine-grained token reaches the action |
| Ask the operator | A surface narrowed by the operator, or by the tier, a call to which names the tier the action needs and whether the session's was pinned or detected; a gate 400 refusing a destination the caller named; a gate 403 for an untrusted origin or host, or for an instance the deployment does not publish; a tool error naming an allow-list variable or a refused destination; the process refusing to start                                                                                                                                                                                   |
| Fix the request  | In-band `-32602` or `-32600`; a gate 400 for a missing or invalid instance header; a listen ended with `resource_gone`                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Start over       | A gate 404 for a foreign session; a closed session; a listen ended with `credential_evicted`, `credential_reset`, `lifetime_reached` or `watcher_evicted`                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| None given       | A failure a full tracking table stops counting; a watch's notifications delayed after GitLab's 429                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |

## The five questions

Every decision answers exactly one question, and the register groups its rows by it
(`Decision.Question`).

| Question                                                                     | What the answer carries                                                                                                                                                        |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Identify**: who is this request?                                           | The mode; the tenant or unknown; the entry; the owner; before admission, the address and the source                                                                            |
| **Admit**: may this credential enter, and for how long?                      | The verdict, charged or not; the cache lifetime; revalidation; the age ceiling                                                                                                 |
| **Authorize**: what may it do?                                               | The tier; the scopes or unknown; read-only and its cause; the groups removed; safe mode; exclusions; the destinations it may dial; the local directories; the response profile |
| **Allow**: what may it hold or spend?                                        | The resource; the key; the limit; the value source; the zero meaning; the process partner                                                                                      |
| **End**: what happens when it exceeds, or when the server ends what it held? | The channel, code, status, `Retry-After`, message, class of next action, charged budget and ending reason                                                                      |

The register decides and never enforces: a stream ceiling stays applied where streams
are counted, and a token bucket where the method is dispatched. It works on stdio, where
there is no pool, no gate and one tenant; it is consulted in no way that changes the
middleware order or adds a lock on the request path; and it holds nothing derived from a
credential.

## Two MCP clauses the server meets in part

MCP 2026-07-28 has one mandatory limit and one note on `clientInfo` that this server
meets only in part. Where it stands on each was decided in
[issue 959](https://github.com/jmrplens/gitlab-mcp-server/issues/959), and the rows they
concern record that decision (`Decided`) in place of the findings that asked for it
(F-19 and F-33).

**"Servers MUST: [...] Rate limit tool invocations"**
([server/tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools),
Security Considerations). The clause names no unit, no value and no refusal. `RTC-001`
meets it with a token bucket counted in requests and refilling at the configured rate
per second, drawn on by `tools/call` and by the four other methods that reach GitLab
with the caller's credential; it refuses a tool call as a tool error and the other four
in-band with `-42900`, and it exists to bound a client in a loop, whose volume an HTTP
deployment would otherwise pass on to the instance and to every other caller sharing its
address. Its key is the pool entry in HTTP mode, one token and GitLab URL pair, where it
is on by default at 10 a second with 40 in hand; on stdio it is the process, and it is
off by default there. A stdio process serves one person with their own token on their
own machine, so there is no co-tenant to protect and a limiter there would only refuse
its one user's own calls, while GitLab's own per-user limits still apply to every call it
forwards. The same bucket is switched on for stdio by setting
`GITLAB_MCP_RATE_LIMIT_RPS` above zero, with `GITLAB_MCP_RATE_LIMIT_BURST` beside it.
`--rate-limit-rps` and `--rate-limit-burst` are read in HTTP mode only: stdio ignores
them and says so at startup (issue 1045), so the variable is the stdio switch.
`test/e2e/stdio` holds both halves against the binary: a tool call refused with the
variable set, and none refused without it.

**`clientInfo` "SHOULD NOT" change behavior**
([basic](https://modelcontextprotocol.io/specification/2026-07-28/basic)). The note says
implementations "SHOULD NOT use them to change the behavior of the client or server, and
SHOULD NOT rely on them for security decisions". The second half is met: identity comes
from the credential (`INV-001`). The first is departed from on purpose: `IDN-013` writes
annotation priorities as 0 or 1 for a session whose `clientInfo` names Codex, matched to
Codex's own two spellings (a `codex-mcp-client` name prefix, or a `title` of exactly
`Codex`). Its key is the session, and the session's `clientInfo` decides wherever it has
one: stdio in either protocol era, HTTP with `--stateless=false`, and any session at
2026-07-28, since go-sdk fills `ClientInfo` from each request's `_meta` there. A client on
2025-11-25 or earlier over the default stateless HTTP transport has no session that saw
`initialize`, so its session has no `clientInfo`; for that case issue 1043 decided that
the profile falls back to a `codex-mcp-client/` prefix of the request's User-Agent, which
Codex's MCP client sends on every Streamable HTTP request. That widens the departure to a
second self-reported label, read only where the first is absent and for the same one
number, and `IDN-013` records it beside issue 959; over stateless HTTP the session is the
POST whose header is read, so the key is unchanged (`test/e2e/http` holds the fallback
and its negatives against the binary).
It is the workaround for a Codex build that rejects a fractional priority, it changes how
one number is written and nothing a model reads, it never decides who a caller is or what
it may do, `GITLAB_MCP_CLIENT_COMPAT=off` removes it, and it retires only once a Codex
built on an rmcp carrying the fix is widely deployed, not merely released (row 17 of
[`upstream-bugs.md`](upstream-bugs.md#a-non-integer-annotation-priority-breaks-a-tool-call)).

## Validation checklist

A new limit, or a change to an existing one, answers each item in its pull request and in
its register row. `Validate` checks the row; VAL-010 and VAL-012 stay pull-request items,
because a declaration cannot hold them. It also holds the register as a whole: one row per
requirement, none declared twice and none missing (its rules `unique` and `complete`), and
every row answering VAL-001 to VAL-009 with a value and naming only rows and findings that
exist (`well-formed`).

| Item    | The question                                                                                                | Answered by                                                                                               |
| ------- | ----------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| VAL-001 | Which of the five questions it answers                                                                      | `Question`                                                                                                |
| VAL-002 | Its key, and that key's mint cost; a new key amends the vocabulary                                          | `Key`, with `MintCost` and `Evidence`                                                                     |
| VAL-003 | Its class; a class D limit needs a finding and a reason to ship anyway                                      | `Class`, checked against `Disagrees`; `StatedUnit` against `ReasonUnit`                                   |
| VAL-004 | Whether it is a ceiling or a share; a share on a mintable key is refused                                    | `Kind`                                                                                                    |
| VAL-005 | If it protects a process resource, its process partner and why that partner is not configurable             | `ProtectsProcess`, `Partner`                                                                              |
| VAL-006 | What happens at capacity, and why                                                                           | `AtCapacity`, held to the capacity rule below, and `Table` for a structure keyed on a caller-chosen value |
| VAL-007 | Its refusal: channel, code, status, `Retry-After`, message and class, per method and era, and on stdio      | `Refusals`                                                                                                |
| VAL-008 | What it charges, and that nothing unjudged is charged                                                       | `Refusals[].Charged`, and `Failures` for an admission path                                                |
| VAL-009 | Its value, source, bounds, zero meaning and malformed-value policy, and for a value no setting reaches, why | `Values`, `Source`, `Fixed`, `Zero`, `SettingZeros`, `ZeroBy`, `Malformed`, `MalformedBy`                 |
| VAL-010 | Its telemetry: the reason only                                                                              | The pull request                                                                                          |
| VAL-011 | Its behavior on stdio                                                                                       | `StdioKey`                                                                                                |
| VAL-012 | Its measurement, when it claims to protect one population from another                                      | The pull request, with `make bench-fairness`                                                              |

**The capacity rule** (issue 958, rule `capacity-fits`) says which behavior at capacity
a row may name, read off its kind, its key and whether it holds a record per key, and
`Validate` holds every row to it with no exception. Refusing the newcomer fits anything
that fills, which is a ceiling, a rate, a budget, a share or a table, and nothing else
names a capacity. Waiting for room is a ceiling's, for a slot another request frees
(`ADM-014`, `POL-006`). Reclaiming takes back what the same key holds, so it needs a key a
caller holds (`AUB-002`'s source, `HLD-003`'s entry). The oldest record is evicted only
from a table kept before admission whose records have a lifetime or a refill: before
admission no tenant exists to hold anything, a record there is a count or a verdict
against its key, and evicting one takes nothing from another key (`INV-005`), which is
the rejected-token cache (`ADM-006`) and the discovery documents' buckets (`RTC-008`).
Counting stops only where the newcomer it no longer counts is let through, which the row
declares as a silent refusal: the authentication tables (`AUB-004`), where evicting a
count would release the block it holds and refusing would refuse every new address while
a flood lasts. And a holding is taken across keys only from a table, by the recorded
decision `INV-005` asks for: the pool entry (`POL-001`, `POL-002`, ADR-0020) and the
verified OAuth identity (`ADM-005`, issue 950). Where more than one behavior fits, the
rule does not choose between them: the row names one, and its comment says why, as the
rejected-token cache's says why it evicts rather than refuse and the authentication
tables' why they stop counting rather than refuse.

**A value no setting reaches says why** (issue 958, rule `fixed-reasoned`). Every row
whose value is a constant, or one only a Go option reaches, states in `Fixed` what the
value is set against and what a setting that moved it would cost or undo. `Validate`
refuses, with no exception, such a row that states none, and a `Fixed` reason on a value
a setting reaches or one derived from another. The process partners and the process's own
ceilings had a reason, `INV-004`'s, written beside some of them in the code; the others
had none a reader could tell from an accident, and the next constant copied whichever
neighbor it was written beside. A Go option no operator reaches is not configuration and
departs from nothing in `INV-017`: it is a constant to every operator, and it is held to
its reason as one is (`ADM-008`, the credential age ceiling, whose option exists for the
tests, and the watcher manager's ceiling and cadence, `HLD-003` and `HLD-007`, whose
`Options` fields only the tests set).

## Findings

Each place where today's key disagrees with the tenant definition, or where the answers
disagree with each other, their stated reason or the protocol, is a finding carried by
the rows it concerns (`Decision.Findings`, and `tenancy.FindingIssue` for its issue). None
is fixed by the register. They are filed, grouped where one change would answer several:

| Issue                                                             | Subject                                                                                                                          |
| ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| [950](https://github.com/jmrplens/gitlab-mcp-server/issues/950)   | OAuth verification is unbounded: the identity cache has no size limit and the verifier's round trips have no concurrency ceiling |
| [951](https://github.com/jmrplens/gitlab-mcp-server/issues/951)   | The process has no ceiling of its own on held requests, stateful sessions or catalog listings                                    |
| [952](https://github.com/jmrplens/gitlab-mcp-server/issues/952)   | Admission and authority read scopes in more than one way                                                                         |
| [953](https://github.com/jmrplens/gitlab-mcp-server/issues/953)   | Attribution: an OAuth tool call loses its instance, request state binds no principal, and unattributed requests share one budget |
| [954](https://github.com/jmrplens/gitlab-mcp-server/issues/954)   | The authentication budgets key on a whole address, and their stated reason is not what GitLab documents                          |
| [955](https://github.com/jmrplens/gitlab-mcp-server/issues/955)   | GitLab's per-user and per-IP budgets are spent through allowances keyed per credential, or not accounted at all                  |
| [956](https://github.com/jmrplens/gitlab-mcp-server/issues/956)   | One refusal class, one code: retry-later is answered in several shapes, and a tier-removed action reads as a typo                |
| [957](https://github.com/jmrplens/gitlab-mcp-server/issues/957)   | Settings read outside the configuration rule, disagreeing on malformed values                                                    |
| [958](https://github.com/jmrplens/gitlab-mcp-server/issues/958)   | Limits disagree on what zero means, keep their defaults in two places, and state no rule for what happens at capacity            |
| [959](https://github.com/jmrplens/gitlab-mcp-server/issues/959)   | Where the server stands on rate limiting tool invocations, and on behavior chosen from `clientInfo`                              |
| [960](https://github.com/jmrplens/gitlab-mcp-server/issues/960)   | Stale statements about the tenant policy in comments, ADRs, the development guide and the site                                   |
| [961](https://github.com/jmrplens/gitlab-mcp-server/issues/961)   | The refusal and ending behaviors of go-sdk that decide what a refused caller sees                                                |
| [982](https://github.com/jmrplens/gitlab-mcp-server/issues/982)   | The transport-source budget remembers every (source, key) pair it charges, and only its sweep bounds that record                 |
| [1053](https://github.com/jmrplens/gitlab-mcp-server/issues/1053) | The discovery documents' bucket is kept per address with no process partner beside it, by decision                               |

One finding grew after it was filed, and one was added, when the register was read
against the code as a whole. F-34 ([issue 958](https://github.com/jmrplens/gitlab-mcp-server/issues/958))
also records the idle session timeout's variable, which refuses a zero its flag accepts
(`END-005`), and the tool-call bucket's burst, whose zero beside a positive rate refuses
startup rather than meaning off (`RTC-001`); both are departures from `INV-015` of the
kind the issue is about. Issue 958 then settled four of the zeros F-34 recorded as stated
exceptions, which their rows record through `ZeroBy` and no longer carry the finding for:
the pool size's and the burst's, which its decision of 2026-09-28 keeps refused; the
credential age ceiling's, the exception its own text keeps; and the watcher ceiling's
option's, under its measure that each zero left departing is an exception with its reason
(see `INV-015`). It answered the other four with code, and no row carries F-34 any
longer. The OAuth cache TTL's flag selected the default in oauth mode and went unread in
legacy mode, where its variable refused in both, and now refuses a zero or a negative
value at startup in either mode as the variable does, the fifth stated exception
(`ADM-005`). The fast lockout's window switched the distinct-credential budget
off with it, whose escalation steps by that window; a zero there now switches the
lockout off alone, and the escalation steps by the default window, as the
transport-source budget and the `/48` aggregate already counted in it (`AUB-001`,
`AUB-003`). And the session timeout's variable refused the zero its flag reads as off,
and now reads it as off too (`END-005`), as every duration variable whose zero means off
now reads `0s` as it reads `0`. Each records the issue. F-16, of the same
issue, recorded bounds no setting reaches with no reason stated, and defaults and
fallbacks written in two places. Issue 958 stated a reason for every value no setting
reaches (see [Validation checklist](#validation-checklist)), so the rejected-token
cache, the credential age ceiling, the unexplained-401 interval, the probe bound and the
upstream retries (`ADM-006`, `ADM-008`, `ADM-010`, `POL-006`, `RTC-006`) record the
issue and carry the finding no longer. It then wrote the four written twice once: the
revalidation interval, the pool size and the idle timeout, which the pool stated a
second time and now aliases from the register as `internal/config` does (`ADM-009`,
`POL-001`, `POL-004`), and the transport-source budget's window fallback (`AUB-002`),
which every budget that reads another one's window takes from one helper, so no row
carries F-16 either. F-16, like F-11, records a
departure from no invariant: the `INV-017` departure it covered, a value only a Go option
reached, is none, since a Go option is not configuration. F-35
([issue 982](https://github.com/jmrplens/gitlab-mcp-server/issues/982)) recorded the
transport-source budget's map of charged (source, key) pairs, which only the sweep bounded,
since a source the full failure table never tracks is never blocked (`AUB-002`), a
departure from `INV-010`. The map was first recorded under F-29, whose issue (950) is
about OAuth verification while the map is kept in both authentication modes, and it was
given a finding of its own once it was filed. Issue 982 answered it with two bounds, and
`AUB-002` records the issue and says what it does at capacity rather than carrying the
finding. A source holds at most `AUB-002`'s value in pairs, since a source charged that
many distinct keys is blocked and remembering more teaches nothing, and at that bound a
fresh key reclaims the source's own oldest pair and is charged as before: stopping
instead would let a source just released from its block fail with fresh keys uncharged
until its old pairs lapse, and reclaiming only the source's own pair takes nothing across
sources (`INV-005`). And a pair is kept only for a failure the transport limiter recorded,
so the map holds nothing the limiter did not charge: a pair kept for an unrecorded failure
used to stand in for a charge that never happened, so the same key failing again once the
limiter had room went uncounted (`INV-007` holds either way, since the caller causes each
failure). That alone does not bound the sources, because the limiter's full table lets a
lapsed record go at most an eighth of a window after it lapses and takes a new source in
its place, while the sweep (`AUB-005`) forgets a pair only every five minutes: a new
table's worth of sources stayed for every window between two sweeps, and more the shorter
`AUB-001`'s window. Once the budget indexes as many sources as `AUB-004` lets that limiter
track, a new source first makes it forget every source none of whose pairs is live, at
most once every eighth of a window, as the limiter sweeps its own table. Past that many,
the sources it holds were all charged within one window and an eighth, while the limiter
held a record of each, and a record stays in that table for a window at least, so they
are at most three times what the table holds, whatever the window. What
the issue leaves alone is the saturation itself, a source arriving once the limiter's
table is full being blocked by neither table, which is the `/48` aggregate's (`AUB-006`)
and issue 958's capacity rules'.

F-13 ([issue 957](https://github.com/jmrplens/gitlab-mcp-server/issues/957)) recorded the
settings read around the configuration package, which every row naming one carried
(`HLD-001`, `RQB-004`, `IDN-009`, `IDN-014`, `AUT-006`, `DST-003`). Issue 957 put the twelve
variables it found that way on `prefixedNames` and read each through `config.Getenv` or
`config.TrimmedGetenv` under the same spelling, and gave the listen ceiling the flag
`INV-017` asks of a limit in HTTP mode (`--max-listen-streams`), so no row carries the
finding. The local allow-lists, which HTTP mode never reads, have no flag and need none:
their source is `StdioEnvOnly`. G14 keeps F-13 as the finding a row would have to carry
if one of its variables were read any other way again, and the one variable read around
the package on purpose, the env file's, is declared with its reason rather than recorded.
The same issue made one row's stated policy true. `IDN-009` said a telemetry identity
policy or rotation that does not parse refuses startup, and the server logged it and
recorded nothing about callers instead, the departure issue 1170 also reported. It refuses
startup now on either transport, once telemetry is known to be on and before anything is
built, and a rotation lying outside what a generated key may live is refused with it;
with telemetry off nothing is exported under either, and beside a configured key the
rotation is ignored.

Issue 1170 reported the other half of that departure. `RQB-007`, the upload limit, named
its flag and its variable and stated no malformed-value policy, and HTTP mode warned and
used the default for a value it could not parse and the maximum for one above the 1 TB
ceiling, where stdio refused both. Both transports refuse all of them now, through the
one reader stdio's `Load` and HTTP mode's overlay call, which also refuses a count whose
bytes do not fit in 64 bits (it used to wrap, so `17179869185GB` was read as 1 GiB) and
reads a blank value as unset on both, and the row states the refused start. Three more
rows with no value source of their own named a setting and stated no policy, `RQB-001`,
`RQB-005` and `DST-002`, each of which the flag package or its variable's reader already
refused, so they state the refused start too, and `Validate` holds the next such row to
stating one. The action timeout's reader read a blank variable as a duration that did
not parse on stdio and as unset in HTTP mode, and reads it as unset on both now.

F-36 ([issue 1053](https://github.com/jmrplens/gitlab-mcp-server/issues/1053)) is the one
finding a decision recorded rather than a reading of the code found. The three documents
the server publishes without a credential, the SEP-2127 card at every path it is mounted
at, the legacy enumerating card and in oauth mode the RFC 9728 metadata, draw on one token
bucket per client address (`RTC-008`), keyed as the authentication budgets key an address,
20 requests a second with 200 in hand by default, `0` for either setting switching it off,
refused with the gate's `429`, `-42900` and a fixed `Retry-After` of one second, and
charged to no budget. Every request to those paths draws on it, a revalidation answered
`304` and a CORS preflight included; `/health` and the MCP endpoint do not. No bucket
keyed on the process stands beside it, which departs from `INV-004`: a bucket the whole
process shared would let a flood from many addresses deny discovery to every caller, a
hosted client's egress address first, so the issue decided against one and the finding
records that it did, which is why `Validate` holds a key before admission to `INV-004` as
it holds a requester's. Its table holds `DiscoveryTrackedAddresses` addresses, and a new
one past that takes the place of the one used least recently, which is how `EvictOldest`
reads for a bucket, since a bucket has no expiry; what that forgets is a record against its
own address rather than a holding taken from it (`INV-005`). No rule at the bound limits a
caller holding more addresses than the table, which any IPv6 `/48` does, without denying
discovery to somebody. Under eviction it cycles them and finds each forgotten when it
returns; against a table that stopped counting, it would fill the table and then ask from
one more address uncounted until the sweep; and refusing newcomers would deny every new
address while the flood lasts, the outcome the missing partner avoids. That gap is part of what the finding
accepts. What eviction changes is the cost: an address that keeps asking stays counted, so
a flood has to keep cycling through more addresses than the table holds, drawing about one
burst from each before it is forgotten, where a table that stopped counting would let a few
thousand single requests from distinct `/64`s leave every newcomer uncounted until its
sweep. The measurement it answers VAL-012 with, `make bench-fairness BOUND=discovery` and
`BenchmarkDiscoveryDocuments` in `cmd/server`, is in the pull request that added it: a
discovery request cost the process about 0.1 ms of processor whatever the document, a
refusal about a sixth less (about 92 microseconds against 110), and the legacy card is
141 KB on the dynamic surface; on a host of 16 logical CPUs, in a container held to five,
sixteen addresses flooding at 100 requests a second each kept 0.22 to 0.30 cores busy in
either arm, each refused four requests in five with the bucket in force, and two at 2,000
a second each 0.42 to 0.47 without the bucket and 0.37 with it, the legacy card's egress
falling from 141 MB/s to about 1.6 MB/s, while a hosted client's egress address shared by
eight users fetching the metadata and the card was refused nothing in any arm. Discovery
requests at those rates do not contend for the machine, so the comparison of the quiet
tenant's latency was the saturation gate's `indistinguishable`: what the bucket bounds is
each address's draw.

The question F-25 left open, whether a REST request carrying an invalid token counts
against GitLab's per-IP limit on unauthenticated requests (OQ-11, as issue 954 names it), is
answered by a probe against GitLab 19.5.0 CE,
`TestAuthThrottle_InvalidTokens_SpendTheAddressUnauthenticatedAllowance` in
`test/e2e/gitlab/ce`, which turns the unauthenticated API throttle on (four requests in
two minutes) and runs only when `E2E_AUTH_THROTTLE_PROBE=true`, since it changes an
instance-wide setting and so never runs beside the other packages; a complete CE run
skips it in its main pass, declared under `runs-last`, and runs it alone in a final pass
on the same stack, after every other test and before the one teardown. With the
throttle off, as a self-managed instance ships, nothing counted an
invalid token. With it on, every `GET /api/v4/user` carrying one in `PRIVATE-TOKEN` was
counted once against the address, the server's own legacy verification of an invented
token included, by the `throttle_unauthenticated_api` GitLab names in its `RateLimit-Name`
header, which GitLab 19.5 counts with Labkit in a window that opens with the first request
counted; the fifth request of the window was answered 429, and so was an anonymous request
from the same address; the run's own token was answered 200 and counted by no
unauthenticated limit, and three servers started at the throttled address verified it and
served `user.current`; in the next window an invalid token was answered 401 and counted
afresh, so no ban followed. GitLab.com counted an invalid token sent as a Bearer token, as
the OAuth verification sends it, once in the same way in October 2026, and GitLab 19.5
reads both headers through one finder whose refusal of a token leaves the request
unauthenticated. The distinct-credential budget's reason (`AUB-003`, stated at
`DistinctTokenBudget`) now says what was observed rather than that GitLab rate-limits
failed authentication per address.

Issue 954 then changed the key, and answered F-14 and F-25 with it; the rows it moved
(`IDN-004`, `IDN-005`, `AUB-001` to `AUB-003`) record the issue among their decisions and
carry neither finding, nor F-34, which issue 958 answered on `AUB-001` and `AUB-003`. Every
budget before admission reads its key off the address through `budgetKey`: a whole IPv4
address, an IPv4-mapped one as the IPv4 address it names, and an IPv6 address by the
fixed `/64` it lies in, since an IPv6 subnet is a `/64` (RFC 4291) and a caller on one
rotated the bits below it into a fresh budget per request. The prefix is not
configurable, which the issue decided, and the cost is the one an IPv4 NAT has always
had, met more often: callers sharing a `/64` share its budgets (the hosts of one IPv6
subnet, the customers a provider numbers from one shared `/64`, and every link-local
caller, whose zone is dropped), while a credential already served is exempt from a block.
A stateful NAT64 presents the translator's IPv4 address, keyed whole as before, but a
stateless translator in front of an IPv6-only server (SIIT-DC, RFC 7755) writes each
IPv4 client into its RFC 6052 prefix, so under `64:ff9b::/96`, or any prefix of `/64` or
longer, they all share one `/64`, and under any prefix of `/48` or longer one `/48`,
where on IPv4 each had a key of its own. The transport source (`AUB-002`) is keyed the
same way. A caller holding a `/48` holds 65536 `/64`s, so `AUB-006` counts the `/64`s of
one `/48` that had a credential refused, on a key of its own (`network`, through
`networkKey`), with a register constant of ten no operator sets, in `AUB-003`'s window
and up `AUB-003`'s ladder, the `/64`s held as digests in a table `AUB-004` caps; a
request that carried no credential moves it no more than it moves `AUB-003`. A refused
credential moves it whoever presents it, so an organization whose network is one `/48`
reaches ten when callers on ten of its subnets present an expired or revoked token
inside the window, and the `/48` is then refused for every credential not already
served. Nor is a `/48` always one organization's: a provider that delegates each
subscriber a `/56` or a `/60` puts 256 or 4096 of them in one, and a mobile network
gives each device a `/64` of its own (RFC 6459), so ten unrelated subscribers presenting
an expired token inside the window block the `/48` for every other one. Its windows fall
back to their defaults when a zero switches off the budget that owns them, so it stays
on wherever the gate runs (`INV-015`), and its refusal is every budget's `429`, counted
in telemetry as `network_aggregate`. A unix socket's peer has no address, and RemoteAddr spells it `@`,
an empty string or a path by platform, so `remoteHost` names every connection accepted on
a socket listener `unix`: with nothing trusted every caller on the socket shares that key,
and `unix` in `--trusted-proxies` trusts the socket's peer, so the proxy on it names each
caller as a TCP proxy does, and the host guard reads the same set. The two pairings that
trust nobody refuse startup: a socket listener whose list names addresses and no `unix`,
which used to start with its header inert, and `unix` on a TCP listener.

The listener is judged once more at startup (`ADM-015`). A TCP listener another machine
can reach that serves plain HTTP is served by default, with a warning, and in the hardened
mode, `--allow-plaintext-http=false` or `GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=false`, the process
refuses to start it, so no caller's token crosses that hop in the clear. The row is keyed
on the deployment, since only the operator sets it, and a value neither the flag package
nor the overlay reads as a boolean refuses startup.

Sixteen findings are answered, and stay in the list with their issues. F-03, the listing
bucket with no process partner, is answered by `RTC-007`, the first of issue 951's three
changes: a `tools/list` bucket keyed on the process and counted in the tools a listing
carries, which `RTC-003` names as its partner and which no row carries F-03 for any
longer. The second and third of those changes answer F-31. The second, `HLD-011`, bounds
the calls the process holds open across every credential, not configurable, a
`subscriptions/listen` aside since `HLD-001` and `HLD-002` count it. Its value is derived
from the descriptors the process may open, read once at startup: an eighth of the limit
spare, one descriptor for each of the 512 listen streams, and two for each held call,
which is 192 under a hard limit of 1024 and 229120 under the 524288 a systemd service is
given by default. The hard limit is the one that counts, because the Go runtime raises
the soft limit to it before `main`; where the platform has no limit to read, the value
is the one a limit of 1024 gives. It counts the calls that reach GitLab, the methods
`MeterFor` charges to the tool-call and completion buckets, where the SDK dispatches
them, so each call of a JSON-RPC batch counts, a response the client sends to a request
of the server's own never does, and neither does a listen on any revision; each stateful
session the process keeps holds one slot for its standalone stream, the GET the SDK holds
open for as long as the session lives, taken with the session's own when the session
opens and kept until it ends, so the GET takes none. A POST on protocol 2026-07-28 or later,
whose `Mcp-Method` header the SDK holds to the body, takes its slot in the gate instead
and is refused there, a 503 `-50300` with `Retry-After` and the connection closed; a
call on an older revision is refused where it is dispatched,
on the channel `RTC-001` refuses the same method on (a result flagged with `isError`, an
in-band error, or an empty completion), the in-band code being `-50300`, the one the gate
answers the same refusal with (issue 956). Every refusal says `This server is busy. Retry later.` and is
charged to nothing. The slot is taken after admission, which departs from what issue 951
asks, that the process slot be taken before anything keyed on the credential: a slot
taken first would let a caller with no credential hold one for as long as its
verification takes, and issue 951 accepted the departure for that reason. A
refused newcomer has spent its admission by then, the pool entry its first call builds
in legacy mode (with `POL-006`'s probe, and at the pool's bound the eviction of another
credential's quiet entry, `POL-002`) or one of `ADM-014`'s verification slots in oauth
mode, and none of its rate. Measured through `cmd/bench_resources`' held mode
against a stand-in GitLab that holds every read, a held call costs the process two
descriptors, six goroutines, about 51 KiB of live heap and about 190 KiB of resident
set, linearly to 4000 calls (8010 descriptors, 873 MiB); under a hard limit of 1024 the
process held 503 and stopped accepting connections, `/health` among them. With the
ceiling, under the same limit and 4000 calls offered, from one credential and from a
hundred, it held 192 in 394 descriptors and refused the other 3808; with the limit
inherited (1048576) it held all 4000, as it did without the ceiling. Issue 951 decided
that nothing stands beside it. No per-credential ceiling does: the credential is a key a
caller can mint (`INV-003`), so a per-credential number multiplies with every token a
caller mints and cannot bound what the process holds, and one credential can therefore
fill the ceiling where the limit is small, at the default rate in about fifteen seconds.
No memory cap does either: the figure bounds descriptors, and where the limit is large
the memory held calls take, about 190 KiB each, is bounded by the memory limit the
process runs under (a container's, or a systemd unit's `MemoryMax`) where one is set,
which the operator sets and the server does not repeat; a unit without a `MemoryMax` of
its own is bounded by the slices above it where one of them sets one, and otherwise only
by the host. And Windows keeps the
figure a limit of 1024 gives rather than none, so that platform is not left unbounded by
default. The third,
`HLD-010`, bounds the stateful sessions the process keeps on `--stateless=false` across
every credential, which the SDK keeps until the client deletes one, the pool evicts its
credential or it sits idle for `--session-timeout`, and which nothing bounded while
`initialize` is metered to no bucket (`RTC-004`). The gate takes a session slot for every
POST that would open one, a POST carrying no `Mcp-Session-Id` on a revision before
2026-07-28, once the credential is admitted and before the SDK creates anything, with a
held-call slot for the session's standalone stream beside it, and refuses it past either
ceiling with `HLD-011`'s 503 in `HLD-011`'s words; the session keeps both slots from the
first request the SDK dispatches on it until it ends, however it ends, and a POST whose
session did not survive it gives them back as the gate returns. A POST on 2026-07-28 or
later is left to the SDK, which answers it, `server/discover` included, with the
revisions the stateful transport serves, so the client falls back, and keeps no session
for it. The ceiling is half of `HLD-011`'s, 96 under a hard descriptor limit of 1024,
rather than a figure of its own: with a held slot taken for every session, the sessions
take at most half of the held slots, the descriptor budget `HLD-011` is sized from holds
as it was, a call on an open session is still served with every session slot taken, and
a session the process keeps is never refused its stream, which the SDK's own client,
refused it, gives up without asking again. No flag moves it, for `HLD-011`'s reason.
Measured through `cmd/bench_resources`' sessions mode before the ceiling, an idle session
cost the process three goroutines, 10 to 17 KiB of live heap, 56 to 119 KiB of resident
set and no descriptor, and one holding its stream six goroutines, about 25 KiB of live
heap and one descriptor; under a hard limit of 1024 the process kept every session it
was offered until the streams had taken all 1024 descriptors, at 1012 to 1016 sessions,
and from there `/health` went unanswered and opens failed at the connection. With the
ceiling, under the same limit and 4000 sessions offered with their streams, from one
credential and from a hundred, it kept 96 in 112 and 183 descriptors and refused the
other 3904 in the gate, `/health` answering throughout; with the limit inherited it kept
all 4000, as it did without the ceiling. The session's slots are given back by a
goroutine of its own, which is one goroutine more per session, four idle and seven with
its stream, bounded by the ceiling. What the ceiling bounds is descriptors, and memory
only where the limit is small: an idle session's 88 to 110 KiB of resident set, measured
with the ceiling at 500 to 4000 sessions, is not something the descriptor limit raises,
so the 114560 sessions a hard limit of 524288 allows come to about ten to twelve GiB
idle, and there the memory limit the process runs under (a container's, or a systemd
unit's `MemoryMax`) bounds them first where one is set; a unit without a `MemoryMax` of
its own is bounded by the slices above it where one of them sets one, and otherwise only
by the host. Standing
alone, it is also cheap to fill: `initialize` spends no rate and an idle session holds no
connection, so one credential can take every slot, and a session nobody deletes holds its
slot for `--session-timeout`, half an hour by default and a day at most, and with a
timeout of zero (`END-005`) until the pool evicts its credential, while every other
tenant's `initialize` is refused. Before this ceiling an idle session refused nobody.
Issue 951 decided to leave both costs where they are, for `HLD-011`'s reason. No
per-credential ceiling stands beside it and `initialize` stays unmetered (`RTC-004`),
because the credential is a key a caller can mint (`INV-003`): a per-credential number,
and a price in the opener's own rate, multiply with every token a caller mints. No fixed
cap stands beside the derived figure either, because the memory limit the process runs
under (a container's, or a systemd unit's `MemoryMax`) bounds the memory the figure does
not where one is set, and a fixed cap no flag moves would be sized for one host. The slot is taken after
admission, the departure `HLD-011` records, and it costs more here: at the pool's bound a
newcomer's admission evicts another credential's quiet entry (`POL-002`), an entry
holding only stateful sessions is quiet (`POL-003`), and evicting it ends those sessions,
so a newcomer this row then refuses has taken from another key what the row counts;
issue 951 accepted that cost with the order.
`IDN-010`, the owner record every stateful session carries, is bounded by the same count
and no longer carries F-31, and neither does `HLD-010`; that bound is stated on the row
and nowhere the register can hold it. F-19 and F-33 are answered by issue 959's decision
that what they recorded is the server's position, stated in
[Two MCP clauses the server meets in part](#two-mcp-clauses-the-server-meets-in-part):
`RTC-001` and `IDN-013` record that decision and carry neither any longer. F-29 and F-30
are answered by issue 950. The OAuth identity cache (`ADM-005`) holds at most ten
thousand identities, the largest pool an operator may configure, and a full cache drops
an expired identity or, when none has expired, the one used least recently, to hold the
one GitLab has just verified. What pushes a live identity out is ten thousand other
distinct credentials used since its last request, so on a deployment serving close to
that many, every new verification does. The verification's round trips to GitLab run
under a ceiling of their own keyed on the process (`ADM-014`): sixteen verifications at
once, not configurable, with slots that are not the pool's probe slots (`POL-006`), so
neither kind of work occupies the other's slots. It bounds concurrency and not rate,
about three hundred and twenty requests a second at fifty milliseconds a round trip,
which is above GitLab.com's allowance for unauthenticated traffic from one address. A
token the cache does not hold waits at most five seconds for a slot and is then refused
with a gate 503 `-50300` and `Retry-After`, charged to no budget, in the words `ADM-002`
uses for a verification with no verdict; a caller that knows the instance to be healthy
can still infer from the refusal that others are verifying, the one bit `INV-019`
accepts for a bound keyed on the process. A token the cache holds is answered before a
slot is asked for. The cost falls on one population, and
`make bench-fairness BOUND=oauth-verification` measures it (VAL-012): a legitimate
credential presented for the first time while a flood holds every slot waits in the
flood's queue and is served only when a slot frees before its five seconds do. On a
sixteen-thread host with the run held to five cores, a stand-in GitLab answering each
verification request in 100 ms, a flood of four hundred invented tokens a second from
ninety-six transport sources, and eight quiet credentials presenting four new tokens a
second between them beside twelve requests a second on credentials already cached, the
ceiling served 42 and 40 of the 120 new tokens of a thirty-second phase in two
repetitions, after 5.5 s at the median where they took 0.52 s without it, and refused
the rest; every one of the 360 cached requests was served in both arms, at a median of
14 ms with the ceiling and 13 and 15 ms without it. The phase follows the lead-in on
one clock and opens after the flood has settled into its queue, so these are what a
sustained flood leaves rather than what an empty queue serves in its first seconds.
The share served follows the slots' share of what arrives, which is two in five here: 80
and 82 of 120 at 50 ms, where the slots finish four in five, 17 and 17 at 200 ms, where
they finish one in five, and 17 and 17 at 100 ms under a flood of a thousand a second,
where they finish about one in six. The new credentials' share sat a few points under
the slots' share in every configuration, which a queue model attributes to the driver's
schedule rather than to the ceiling: the flood arrives evenly spaced and the new
credentials in clumps on fixed ticks, and when both arrive at random the two shares
match. What the ceiling buys is the instance's load, which the slots and the round trip
set whatever the flood: at 100 ms the instance received about 160 requests a second
under either flood, sixteen slots over the round trip, never more than 20 at once,
against about 420 and 1,020 a second and up to 56 and 110 at once without it (5,640 and
5,600 requests over the 35 seconds from the phase's first request to the answer of its
last waiter, against 12,600 and 30,600 over 30). The queue moves into the process
instead, since each waiting request holds its connection for up to the five seconds:
the server's resident set peaked at 273 and 277 MiB against 175 and 178 without the
ceiling under the flood of four hundred, and at 436 and 438 against 188 under the flood
of a thousand. Nothing bounds how many requests wait but the rate they arrive at, and
their connections are descriptors outside the held calls `HLD-011` counts, since the
wait comes before the gate admits a request; the runs had a descriptor limit of 1048576
and did not reach it. The same waiters hold memory: each cost the process about 50 KiB
in the measurement (the resident set rose by about 100 MiB for some 2,000 waiting and
250 MiB for some 5,000), so where the memory limit the process runs under is below the
arrival rate times five seconds times that, the flood ends the process and every cached
credential with it. The ceiling trades the admission of new credentials during a flood
for the load the instance receives.
Measured through the verifier
against a stand-in GitLab, a hundred thousand distinct credentials held a hundred
thousand entries and sixty megabytes before, and hold ten thousand and seven megabytes
now; two thousand invented tokens at once put two thousand verification requests in
flight before, and sixteen now. `ADM-002`, `ADM-005` and `POL-006` carry neither finding
any longer, and `ADM-005` and `ADM-014` record the decision. F-20 is answered by a change
of code made under [issue 961](https://github.com/jmrplens/gitlab-mcp-server/issues/961),
which stays open for F-21, F-22 and F-24. go-sdk v1.8.0 labels a `tools/call` result with
the `resultType` 2026-07-28 requires only inside its own tool dispatcher, so `RTC-001`'s
refusal of a `tools/call`, a tool error its middleware returns in the dispatcher's place,
reached a 2026-07-28 client without the field. The middleware now labels it
(`toolutil.LabelForRevision`, row 66 of `docs/development/upstream-bugs.md`): a request
naming 2026-07-28 or later in its `_meta` is refused with `resultType: "complete"`, as a
call the dispatcher serves is answered, and one of an earlier revision with no
`resultType`. That is the test go-sdk v1.8.0 applies to the results it labels once the
middleware chain returns, and the one its fix applies to every result; its tool
dispatcher reads instead the revision the session recorded when it began, and the two
differ only where that revision and a request's `_meta` disagree: a client negotiated
down from 2026-07-28 gets the dispatcher's label and not this one, which its revision
does not require, and a client that negotiated an earlier revision and then names
2026-07-28 in a request's `_meta`, which v1.8.0 accepts over stdio, gets this label and
not the dispatcher's, which is what that revision requires and what the fix sends. The
refusal keeps its channel, its text and its error flag, and
`HLD-011`'s refusal of a `tools/call` is labeled the same way, as is the third tool result
a middleware here makes, the refusal of a withheld call that `AUT-007` and `AUT-008`
declare (below). `RTC-001` records the issue and no longer carries F-20.
F-17, that a fine-grained personal access token was misread, is answered by issue 952
([ADR-0024](adr/adr-0024-fine-grained-token-authority-per-action.md)). Its scope list is
the single value `granular`, which names no authority: read as scopes, it narrowed such a
token to the read-only surface `AUT-001` then served a token that cannot write, and both
doors misread it, the legacy gate
taking the probe's 403 for a refused credential, charged and answered 401, and the OAuth
door refusing it for want of `read_api` (`ADM-001`, `ADM-002`). It is now unknown
authority. The doors answer a token GitLab accepted and refused User: Read with an
uncharged 403 under a stable prefix, its body quoting GitLab's sentence filtered and cut
at 512 bytes and the bearer challenge's description a constant of this server's, and
remember the verdict in `ADM-006`'s cache for its five minutes, since nothing at GitLab
19.4 edits a grant after its token is created; what stays bounded in concurrency alone is
a flood of distinct minted tokens, each a genuine credential. `ADM-003`'s introspection
reads a `403` carrying `insufficient_granular_scope` on the token's own description as an
answer rather than a failure: that route's boundary is the user and names no root
namespace, so only a fine-grained token is refused a grant there, and the refusal is taken
as such a token's one scope, `granular`, without asking `/oauth/token/info`, which knows
nothing of a personal access token. What a fine-grained session
is shown is two rows of their own. `AUT-007` withholds the actions no fine-grained token
can reach at the GitLab release the server's permission table was recorded from, and
`AUT-008` the actions the token's grant does not reach when the server can read the
grant, through the rule `tenancy.CoverableAt`, which says what one granted scope covers
before a call's target is known. Both refuse a
`tools/call` on the `Withheld` channel with a stable prefix and leave the action out of
`tools/list` on the `Absent` channel, answered Widen the scope, charge no failure budget
and key nothing on the grant (`INV-010`), the authority going with the pool entry
`POL-001` and `POL-002` already bound; neither carries F-20, since the call middleware
labels its refusal of a withheld call as `RTC-001`'s middleware labels its own. `RQB-011`
bounds the read of the grant, 1 MiB and 1000 scopes, under a per-request ceiling below
`RQB-009`'s, because every project or group a scope names becomes a scope of its own and
the minter sizes the grant. `AUT-001`, `ADM-001` and `ADM-002` carry F-17 no longer and
record the decision. F-09, that unknown authority resolves wide for scopes and narrow for
the tier, is answered by issue 952's other decision, which kept both: scopes nobody
answered for still count as write-capable (`ADM-003`), and a tier neither the license nor
a namespace plan answers for is still Free, recorded as `INV-008`'s one exception, with
the warning an enterprise build gives naming `GITLAB_MCP_TIER` and `--tier` (`AUT-003`,
issue 900). `ADM-003` records ADR-0018 and issue 952, `AUT-003` issues 900 and 952, and
neither carries F-09 any longer, nor does `AUT-003` carry F-10 (below). `AUT-003` also records issue 1224,
which gave the tier a third question on GitLab.com: GitLab.com reports a namespace's plan
only to a caller who may administer it and serves a licensed feature to every member, so
a member of a paid group who owned no paid namespace resolved Free. Unless a namespace
already answered Ultimate, the probe now reads the top-level groups the caller reaches,
each with whether its plan carries a Premium and an Ultimate feature, in pages of
`TierMembershipPageSize` up to `TierMembershipMaxPages` and stopping at the first
Ultimate, as the namespace pages are bounded; the higher of the namespace and membership
answers wins, a read that fails keeps what the namespaces answered and a page GitLab
answered in part keeps the groups it resolved, the side of the asymmetry `INV-008`
records. The tier is asked when an entry is built and kept while it lives (`POL-007`), so a changed
membership reaches a caller when the entry is next built. F-08, that the legacy door and the
OAuth one admitted different minimum authority, is answered by the same decision, which set
the admission minimum at `read_api` in legacy HTTP and on stdio as well: the legacy door
asks the predicate the OAuth door asks (`MeetsMinimum`), and refuses a token GitLab
accepted below it 403, uncharged and remembered in `ADM-006`'s structure, whether the
probe said so (403 `insufficient_scope`, which it used to answer 401 and charge) or the
token's own description did (`read_user`, which it used to admit to a surface on which
every call failed). On stdio the same predicate reads the description of the process's
token once at start, under `--ignore-scopes` too, and a token below it leaves the process
answering the handshake and `ping` while every catalog method is refused in-band with
`-40300` and the way out; the process does not exit, since a client reads an exit as a
crash with the reason on stderr alone. A start that could not describe the token learns
it on the first round that can and refuses from then, and a version GitLab refuses for
want of a scope (what it answers a token carrying `self_rotate` or `read_repository`
alone) counts as an instance that answered rather than one the start could not reach,
and is a refusal on its own: the version endpoint takes `read_user`, `ai_features`,
`ai_workflows`, `api` and `read_api`, so a token it refuses carries neither of the two,
and the process refuses even when the token's description went unanswered.
`ADM-001` records ADR-0018 and issue 952, and neither it nor `ADM-002` carries F-08 any
longer. F-07 and F-10 are answered by issue 956, whose decision is that each refusal
carries the code the standard assigns to what it says. A refusal telling the caller it
made or holds too many is answered in-band `-42900`, mirroring 429, and one saying the
server cannot serve it right now `-50300`, mirroring 503, so the legacy `-32000` is
answered by nothing: the per-credential listen ceiling (`HLD-001`) and watcher ceiling
(`HLD-003`) and a GitLab 429 on a first read (`RTC-005`) moved to `-42900`, the process
listen and watcher ceilings (`HLD-002`, `HLD-004`), a subscribe during shutdown and the
readiness gate abandoning a request to `-50300`, and `HLD-011`'s in-band refusal from
`-42900` to `-50300`, since the gate answers that same refusal 503. The two listen
ceilings, which shared one sentence naming its scope, now refuse in words and codes of
their own, and the process watcher ceiling refuses with a sentinel of its own
(`ErrServerAtCapacity`), so the code is chosen by what the refusal says and not by the
layer that made it. The process's listing bucket (`RTC-007`) keeps the credential's
words and so 429's code: issue 951 chose those words so that a refusal names no process,
and issue 956 did not reopen that choice. `Validate`
holds that rule as `INV-012` (`code-mirrors-status`): an in-band code of the server's own
must be a status the gate answers the same class of next action with, times `-100`, and
the legacy range is refused with no exception (`INV-011`). The tier narrowing, which
answered an action above the instance's tier as unknown, now answers it as withheld,
naming the tier it needs, the tier the session was built for and whether that tier was
pinned or detected, and asks the operator (`AUT-003`), since the caller cannot change the
licence and an operator can pin the tier when detection was wrong. When read-only mode or
a scope the credential lacks would still withhold the action once the tier served it, the
same answer says so, so the operator is not sent to pin a tier that only uncovers the
next refusal. The meta surface still refuses a paid action of a base group, such as
`gitlab_project`'s `approval_config_get`, through the tool's action enum, as it refuses
an action the scope or the operator withheld; only the dynamic surface answers with the
cause. The dynamic surface
learns which actions the tier removed from the catalog built without the tier filter
(`tools.TierWithheld`), never naming one the operator excluded by name, so the
`related_actions` links that the session dropped at Free (five in three packages) are
published like the links a scope or operator filter withheld. `RTC-005`, `HLD-001` to
`HLD-004`, `HLD-011` and `AUT-003` record the issue, and none carries F-07 or F-10. F-14
and F-25 are answered by issue 954, as its two paragraphs above say: the stated
reason first, from what GitLab 19.5 answered, then the key, the `/48` aggregate
(`AUB-006`) and the named unix-socket key. F-35 is answered by issue 982, as the
paragraph that records it says: the transport-source budget's pairs are bounded per
source, and across sources to three times what its limiter's table holds, and `AUB-002`
records the issue and carries the finding no longer. F-11 is answered by issue 958's
capacity rule (see [Validation checklist](#validation-checklist)): the rejected-token
cache's eviction of its oldest record and the authentication tables' stop at a full
table are behaviors the rule allows for what each holds, each row says why it names the
one it does, and `ADM-006` and `AUB-004` record the issue and carry the finding no
longer. F-16 and F-34 are answered by issue 958 too, as the paragraph that records them
says: every zero that does not mean off is a stated exception with its reason, every
other zero means off through the flag and the variable alike and switches nothing else
off, and each default and fallback is written once. F-04 and F-26 are answered by
issue 960, which restated comments and changed no code: `HLD-001`'s reason calls the
listen ceiling per credential and bounding what one credential holds open, with no
share word, and the pool's comments say that size pressure consults
`serverpool.WithInUse` as the idle sweep does (`POL-002`); both rows record the issue
and carry the finding no longer. F-01, F-02, F-05, F-06 and F-32 are answered by issue
955, whose decision of 2026-09-27 kept on the entry the three allowances whose reasons
were about a GitLab user (F-01) and corrected each reason to what the entry bounds,
which restated comments and changed no code. The tool-call bucket (`RTC-001`) bounds
what one credential sends, where its comment named GitLab's per-token limits (F-02);
the watcher pause on a GitLab 429 (`RTC-005`) holds back what one credential's
watchers send, where it gave GitLab's per-user limit as its reason while another
credential of the same user kept polling (F-06); and the watcher cap (`HLD-003`)
bounds what one credential's watchers send, where it was sized for one user's budget
(F-05). A user holding N credentials holds N of each, all spending the one budget
GitLab keeps for that user, and the comments and the site say so. The upstream
retries (`RTC-006`) keep their values and state their cost: one GitLab request costs
at most three upstream requests, and one tool call the bucket charges once may send
several, which GitLab counts against the user while the bucket sees one (F-32). Those
four rows and the two that resolve the entry and the tenant (`IDN-002`, `IDN-008`),
where no allowance is keyed on the tenant, record the issue and carry none of the
five. F-18 is answered by issue 955 as well, with the observe-only counter the same
decision named. GitLab.com keeps a budget per IP address that every tenant spends through
the deployment's egress address, and that no row records because nothing here is keyed on
it; the process now counts every request it sends upstream, retries and redirect hops
included, from the API client, the OAuth verifier and the health probes alike, keyed on the
process and on the bounded server address (`INV-018`), and exports the count as
`gitlab_mcp.upstream.requests`. The counter bounds nothing, so no row was added for it:
F-18 was carried by no row before and is carried by none now, and is answered by the issue
the rows above record. F-28 is answered by issue 953, which put the instance on the
identity of an OAuth tool call: the handlers read that caller from the verifier's
admission, which carried the user id and the name and no instance, so user 7 of two
published instances was logged as one caller. The admission now carries the instance it
verified the token against, and the telemetry identity policy records the pair, the
instance as `gitlab_mcp.instance` beside the user under `full` and inside the digest under
`pseudonymous`, on spans and exported log records alike. `IDN-008` records the issue and
carries the finding no longer. F-12 is answered by issue 953 too, which bound signed
request state to the principal its request ran under beside its call: the state was
signed with one key per process and bound to the tool and its arguments alone, so on a
server the credentials of one shape share, state issued under one credential verified
under any other for its ten minutes, and the second was handed the first one's answers
(`INV-019`). The credential binding now puts the pool entry's owner token on the request
(`withCredentialState`), the flow reads it (`FlowFromRequest`), and the state carries a
MAC of its payload under a key derived from that token beside a seal under the process
key, so nothing readable on the wire names the principal, nothing derived from the
credential travels at all, and state issued under another credential yields no answers
and the flow asks again (`decodeState`). The token is random and minted per entry, so a rebuilt
entry asks again too, which is the answer the flow already gave state it cannot use; on
stdio the principal is empty for everyone the process serves. `IDN-011` records the issue,
keeps its lifetime and carries the finding no longer. F-15 is answered by issue 953 as
well, which left a request no credential was bound to with no budget to share
(`INV-002`): on a server the credentials of one shape share, such a request used to run
under the shell's default state, which failed closed towards GitLab but held a bucket and
a listen counter of its own, so every unattributed request drew on one budget, and a
`tools/list` or a `subscriptions/listen` asking only for list-changed notifications, which
no handler refuses, was charged there and held open there. A shared shell now has no
default state (`serverShell.defaultCredentialState`, `serverShell.stateFor`), so such a
request holds no bucket, no listen slot and no watcher, and `refuseUnattributed`, which
runs right after the binding (`IDN-006`), refuses a `tools/list` with the internal error
the resource and prompt methods answer it with and every `subscriptions/listen` with the
subscription's. Stdio keeps its one credential as the state every request finds. `IDN-007`
records the issue, declares the listing among its refusals and carries the finding no
longer. Three more have been carried by no row since the register landed,
because each records something no row decides: F-24 a message the SDK gives the server no
way to send, and F-23 and F-27 stale statements, which issue 960 corrected in the comments
and documents that made them. `TestDecisions_AFindingNoRowCarries_IsAnsweredByItsIssue`
holds both sets: a finding no row carries fails unless it is one of those three or a row
records its issue in `Decided`, and the answered set is named, so a finding dropped from a
row by mistake fails too.
