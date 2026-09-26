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

| Mode        | Credential source                                         | Admission                                                                            | Where the tenant is resolved                                             | Today's per-caller key |
| ----------- | --------------------------------------------------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------ | ---------------------- |
| stdio       | `GITLAB_TOKEN` from the environment                       | The operator's configuration; no admission refusal exists                            | Once per process, after the handshake starts (`IDN-008`)                 | The process            |
| HTTP legacy | `PRIVATE-TOKEN`, then `Authorization: Bearer` (`IDN-001`) | GitLab did not answer `GET /user` with 401 or 403 (`ADM-001`)                        | When the pool entry is built, and put on the request context by the gate | The entry              |
| HTTP OAuth  | `Authorization: Bearer` only (`IDN-001`)                  | Verified against the selected instance, introspected, `read_api` minimum (`ADM-002`) | The verifier's resolved user, and the pool per entry                     | The entry              |

On stdio, process, entry and tenant are one. Over HTTP one tenant maps to one entry per
credential per instance, one entry to one tenant or to unknown and to one owner for the
life of its build, and one address to many tenants.

### Two axes, and why the tenant does not license a share

Every decision bounds either the **requester** (a tenant, a credential or something one
credential holds) or the **process**. The process is the only unit no caller can
multiply.

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

| Key           | What it is                                            | Axis             | Mint cost to the caller                                                                                                         |
| ------------- | ----------------------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `request`     | One request, argument set or watched URI              | request          | Nothing                                                                                                                         |
| `credential`  | The raw token a request presents                      | requester        | Another credential of the same user: a personal access token through the UI, no administrator, no cap, no creation rate limit   |
| `entry`       | One credential on one instance, the pool entry        | requester        | As a credential; several published instances do not multiply keys                                                               |
| `owner`       | The random name of one entry build                    | requester        | As an entry                                                                                                                     |
| `tenant`      | (canonical instance URL, GitLab user id)              | requester        | Another GitLab user: a project bot on self-managed, a paid namespace or a service account on GitLab.com                         |
| `verified`    | (instance, token) as the OAuth identity cache keys it | requester        | As a credential                                                                                                                 |
| `application` | The OAuth application a token was issued to           | requester        | Nothing: dynamic registration needs no authentication                                                                           |
| `session`     | An SDK session id, recorded to an owner               | requester        | Nothing: any `initialize` opens one                                                                                             |
| `address`     | The charged client address                            | before admission | Address rotation; behind a trusted proxy that copies a caller's header, the header value; one for every caller on a unix socket |
| `source`      | The socket peer, every header ignored                 | before admission | Address rotation only                                                                                                           |
| `refused`     | A credential GitLab refused, held as a digest         | before admission | Nothing: an invented string                                                                                                     |
| `unbound`     | Every request no credential was bound to              | process          | Not mintable                                                                                                                    |
| `process`     | The running binary                                    | process          | Not mintable                                                                                                                    |
| `deployment`  | The operator's configuration                          | process          | Not mintable by a caller                                                                                                        |

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
process partner beside it. `Validate` holds the declared class to that answer. The class
D decisions are `RTC-001` on HTTP, `RTC-003`, `RTC-005` and `HLD-003`: each is keyed
finer than the tenant, so a tenant with several credentials holds several units of it,
and moving any of them to the tenant would still leave it dividable by bots.

A row carries two units for its reason: the unit the quoted reason names in its own
words, and the unit of what it cites. They differ for `RTC-001` alone, whose comment
names GitLab's per-token limits while GitLab keeps the limit it cites per user, and that
misstatement is itself a finding. A ceiling a comment calls "fairness" is not class D on
that account, since fairness names no unit; the defect is `INV-003`'s vocabulary rule,
which `HLD-001` breaks.

## Invariants any limit must satisfy

A new limit, or a change to an existing one, meets every invariant below. Existing code
that does not is a finding, filed as an issue; nothing here requires it to change.
`Validate` refuses a row that breaks `INV-003`, `INV-004`, `INV-005`, `INV-007`, `INV-010`,
`INV-011`, `INV-012`, `INV-015`, `INV-016`, `INV-017` or `INV-018` unless the row carries
a finding recorded for that invariant.

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
  number can undo the bound (`HLD-001` with `HLD-002`, `HLD-003` with `HLD-004`). The pool
  size (`POL-001`) is not such a partner: it bounds memory the operator provisions.
- **INV-005 Across keys, refuse; never take.** A holding that belongs to one key is not
  evicted to admit another key. The pool entry is the one thing taken across keys, the
  quiet one first, never growing past the bound, and the newcomer is never refused
  (`POL-001`, `POL-002`, ADR-0020).
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
  write-capable; a detected tier is the highest paid plan found (ADR-0018).
- **INV-009 The surface varies only with the authorization.** A listing may vary with the
  authorization on the request and never with the connection or other requests on it, and
  a listing is never shrunk to express a refusal.
- **INV-010 Caller-chosen values never grow a structure that does not evict.** A cache or
  table keyed on a caller-chosen value either evicts or refuses new keys at a bound. The
  shape key holds nothing about the credential's identity beyond what changes the catalog.
- **INV-011 Every refusal uses a channel the protocol and the SDK carry** (see
  [Refusal channels](#refusal-channels)).
- **INV-012 One class and one next action per refusal.** A refusal belongs to exactly one
  class of next action and says which. Within one layer, a class uses one code.
- **INV-013 Telemetry names the reason, never the caller.** Refusal telemetry carries the
  reason, never the address or the identity, and `/health` carries no pool state.
- **INV-014 Only the pool decides who chose the destination** (ADR-0022).
- **INV-015 Zero means off.** A limit of zero means no limit and never refuses
  everything; switching a ceiling off keeps any count another decision depends on; and
  switching one limit off does not switch off another.
- **INV-016 Every limit states its key, mint cost and reason, and the units agree.** The
  reason's unit matches the key, or the mismatch is recorded as a finding before the limit
  merges, and the reason states its own unit correctly.
- **INV-017 Configuration goes through the configuration package.** A configurable limit
  is read through `config.Getenv` or `config.TrimmedGetenv` with its name in
  `prefixedNames`, has a flag in HTTP mode, and states its malformed-value policy.
- **INV-018 Bound the process on the process.** A bound whose purpose is to protect the
  process, or an upstream budget the whole deployment shares, is keyed on the process.
- **INV-019 No cross-tenant observation.** No tenant observes another's data, watch state
  or the existence of its traffic; the one-bit disclosure of `credential_evicted` is the
  accepted exception (ADR-0020).
- **INV-020 Endings name their cause from a closed vocabulary**, and a removal path added
  without a decision produces no reason rather than the nearest one.
- **INV-021 A change of policy is its own change.** A change to a limit's key, value,
  channel, configurability or zero meaning is made in a change of its own, with a
  measurement from `make bench-fairness` where it claims to protect one population from
  another.

## Refusal channels

A limit whose refusal no client can see is not the limit its author meant, so the channel
is part of the decision. `tenancy.Carriages()` is the matrix below as data, and it
describes go-sdk as this server builds against it: an SDK upgrade that changes what is
carried edits it in the same change.

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
  and the in-band "retry later" code mirrors 429 the same way. Nothing is emitted in
  `-32020` to `-32099` that MCP does not define there. `-32000` sits in the legacy
  sub-range that new implementations should not use; the listen and watcher ceilings still
  refuse with it, which is a finding.
- **A `subscriptions/listen` refusal is made before the acknowledgment**, and a modern Go
  SDK client never observes it.
- **Ending is not refusing.** A conformant client comes back after a session closes or a
  listen ends, so neither is ever the only answer to a condition the caller must act on.
- **A refusal is never a notification the client did not ask for**, a server-initiated
  request, or `notifications/cancelled`.
- **On stdio there is no status**: only in-band errors, tool errors and empty completions.

### Where a refused caller learns what to do

Every refusal names one class of next action (`tenancy.Answer`):

| Answer           | Channels today                                                                                                                                       |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Retry later      | A gate 429 or 503 with `Retry-After`; in-band `-42900` or `-32000`; a tool error saying to back off; an empty completion                             |
| Reauthorize      | A gate 401 with `invalid_token`; a listen ended with `credential_revoked`                                                                            |
| Widen the scope  | A gate 403 with `insufficient_scope`; a surface narrowed by the credential's scope                                                                   |
| Ask the operator | A surface narrowed by the operator; a gate 400 naming a flag; a gate 403 for an untrusted origin or host; a tool error naming an allow-list variable |
| Fix the request  | In-band `-32602` or `-32600`; a gate 400                                                                                                             |
| Start over       | A gate 404 for a foreign session; a closed session; a listen ended with `credential_evicted` or `credential_reset`                                   |
| None given       | A tier narrowing, answered as an unknown action                                                                                                      |

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

## Validation checklist

A new limit, or a change to an existing one, answers each item in its pull request and in
its register row. `Validate` checks the row; the last two items stay pull-request items,
because a declaration cannot hold them.

| Item    | The question                                                                                           | Answered by                                                              |
| ------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------ |
| VAL-001 | Which of the five questions it answers                                                                 | `Question`                                                               |
| VAL-002 | Its key, and that key's mint cost; a new key amends the vocabulary                                     | `Key`, with `MintCost` and `Evidence`                                    |
| VAL-003 | Its class; a class D limit needs a finding and a reason to ship anyway                                 | `Class`, checked against `Disagrees`; `StatedUnit` against `ReasonUnit`  |
| VAL-004 | Whether it is a ceiling or a share; a share on a mintable key is refused                               | `Kind`                                                                   |
| VAL-005 | If it protects a process resource, its process partner and why that partner is not configurable        | `ProtectsProcess`, `Partner`                                             |
| VAL-006 | What happens at capacity, and why                                                                      | `AtCapacity`, and `Table` for a structure keyed on a caller-chosen value |
| VAL-007 | Its refusal: channel, code, status, `Retry-After`, message and class, per method and era, and on stdio | `Refusals`                                                               |
| VAL-008 | What it charges, and that nothing unjudged is charged                                                  | `Refusals[].Charged`, and `Failures` for an admission path               |
| VAL-009 | Its value, source, bounds, zero meaning and malformed-value policy                                     | `Values`, `Source`, `Zero`, `Malformed`                                  |
| VAL-010 | Its telemetry: the reason only                                                                         | The pull request                                                         |
| VAL-011 | Its behavior on stdio                                                                                  | `StdioKey`                                                               |
| VAL-012 | Its measurement, when it claims to protect one population from another                                 | The pull request, with `make bench-fairness`                             |

## Findings

Each place where today's key disagrees with the tenant definition, or where the answers
disagree with each other, their stated reason or the protocol, is a finding carried by
the rows it concerns (`Decision.Findings`, and `tenancy.FindingIssue` for its issue). None
is fixed by the register. They are filed, grouped where one change would answer several:

| Issue                                                           | Subject                                                                                                                          |
| --------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| [950](https://github.com/jmrplens/gitlab-mcp-server/issues/950) | OAuth verification is unbounded: the identity cache has no size limit and the verifier's round trips have no concurrency ceiling |
| [951](https://github.com/jmrplens/gitlab-mcp-server/issues/951) | The process has no ceiling of its own on held requests, stateful sessions or catalog listings                                    |
| [952](https://github.com/jmrplens/gitlab-mcp-server/issues/952) | Admission and authority read scopes in more than one way                                                                         |
| [953](https://github.com/jmrplens/gitlab-mcp-server/issues/953) | Attribution: an OAuth tool call loses its instance, request state binds no principal, and unattributed requests share one budget |
| [954](https://github.com/jmrplens/gitlab-mcp-server/issues/954) | The authentication budgets key on a whole address, and their stated reason is not what GitLab documents                          |
| [955](https://github.com/jmrplens/gitlab-mcp-server/issues/955) | GitLab's per-user and per-IP budgets are spent through allowances keyed per credential, or not accounted at all                  |
| [956](https://github.com/jmrplens/gitlab-mcp-server/issues/956) | One refusal class, one code: retry-later is answered in several shapes, and a tier-removed action reads as a typo                |
| [957](https://github.com/jmrplens/gitlab-mcp-server/issues/957) | Settings read outside the configuration rule, disagreeing on malformed values                                                    |
| [958](https://github.com/jmrplens/gitlab-mcp-server/issues/958) | Limits disagree on what zero means, keep their defaults in two places, and state no rule for what happens at capacity            |
| [959](https://github.com/jmrplens/gitlab-mcp-server/issues/959) | Where the server stands on rate limiting tool invocations, and on behavior chosen from `clientInfo`                              |
| [960](https://github.com/jmrplens/gitlab-mcp-server/issues/960) | Stale statements about the tenant policy in comments, ADRs, the development guide and the site                                   |
| [961](https://github.com/jmrplens/gitlab-mcp-server/issues/961) | The refusal and ending behaviors of go-sdk that decide what a refused caller sees                                                |
