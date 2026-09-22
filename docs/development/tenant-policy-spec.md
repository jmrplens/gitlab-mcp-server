# Tenant policy: what identifies a caller, and what it may hold

Specification for [issue 565](https://github.com/jmrplens/gitlab-mcp-server/issues/565). It settles the
questions that issue says have to be settled before any code, and proposes nothing
else: **no limit's value and no current behaviour changes here.**

Status: draft, for review. Nothing in this document has been implemented.

## The question

Six layers decide what a caller may hold, and each invented its own criterion. The
issue lists them; all six were read on 2026-09-22 and are where it says:

| Decision                                                       | Where                                   | Keyed on                    |
| -------------------------------------------------------------- | --------------------------------------- | --------------------------- |
| Rate-limit bucket                                              | `cmd/server/credential.go:312`          | pool entry                  |
| `subscriptions/listen` ceiling, per credential and per process | `cmd/server/subscriptions.go:1544`      | pool entry, then process    |
| Watcher cap                                                    | `internal/subscriptions/manager.go:172` | manager, one per credential |
| Eviction under size pressure, and the busy-skip                | `internal/serverpool/pool.go:1346`      | pool entry                  |
| Read-only narrowing from token scopes                          | `internal/gitlab/scopes.go:62`          | token                       |
| Tier                                                           | `internal/gitlab/client.go:476`         | instance licence            |

Four of the six are keyed on a pool entry, and a pool entry is keyed on
`sessionKey(token, gitlabURL)`, which is `SHA-256(token || 0x00 || gitlabURL)`
(`internal/serverpool/pool.go:1369`).

**That key is minted by the caller.** A second personal access token produces a
second entry, and with it a second rate-limit bucket, a second watcher allowance
and a second listen ceiling. This is the fact that broke all four designs in
[issue 540](https://github.com/jmrplens/gitlab-mcp-server/issues/540) and then
decided [issue 561](https://github.com/jmrplens/gitlab-mcp-server/issues/561).
It has never been written down anywhere a seventh layer would find it.

## What GitLab considers a principal

Checked against GitLab's own documentation on 2026-09-22.

**GitLab counts authenticated API requests per user, not per token**
([User and IP rate limits](https://docs.gitlab.com/administration/settings/user_and_ip_rate_limits/)):
"Maximum authenticated API requests per rate limit period per user", 7200 requests
per 3600 seconds by default. Unauthenticated requests are counted per IP, 3600 per
3600 seconds. A request over the limit is answered `429` with `Retry-After`.

So GitLab has already answered "what is a tenant", and its answer is the **user**.
A user who creates ten personal access tokens still has one budget with GitLab.

**This server is therefore more generous to an attacker than GitLab is.** Ten
tokens buy one GitLab budget and ten of everything here. That asymmetry is the
defect the issue is circling, stated plainly.

### What it costs to become another principal

The hierarchy matters more than the fact, because it is what a policy can lean on:

| Identity       | Cost of minting another                                                                                                                                                                                                                                                                                                                             |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Token          | None. Any user creates another personal access token in seconds.                                                                                                                                                                                                                                                                                    |
| GitLab user    | A project or group access token creates a bot user with its own user ID, and needs the **Maintainer or Owner** role on that project ([Project access tokens](https://docs.gitlab.com/user/project/settings/project_access_tokens/)); on GitLab.com it needs Premium or Ultimate. Otherwise: register another account, which an instance can forbid. |
| Client address | Depends on the deployment, and is already bounded separately by the authentication budgets.                                                                                                                                                                                                                                                         |

A bot user is a real second principal with a real cost, so keying on the user is
not a perfect answer. It is a much better one than keying on the token, and it is
the same answer GitLab gave.

## What this server already knows

**The non-mintable identifier already exists and is already computed.** The pool
resolves `UserIdentity{UserID, Username}` for each credential through
`client.CurrentUser(ctx)` (`internal/serverpool/pool.go:831-846`), and the OAuth
verifier resolves the same user id (`internal/oauth/verifier.go:528`). ADR-0008 is
where that identity was introduced.

Nothing keys a limit on it. Every limit that could be shared across a user's tokens
is instead keyed on the token.

That is the finding this specification exists to record: the four designs in issue
540 each failed on the mintable key, and a key that is not mintable was sitting one
field away the whole time.

## What the MCP specification and the SDK allow

Read on 2026-09-22 against revision 2026-07-28, and it settles more than expected.

**Rate limiting is normative.** The Tools page's security considerations say
servers **MUST** "Rate limit tool invocations", alongside validating inputs,
implementing access controls and sanitising outputs
([Tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)).
So a policy here is required rather than merely allowed. What the specification
does not say is how: **no page of it mentions `429`, `Retry-After` or backoff at
all**, and the authorization page's error table lists only `401`, `403` and `400`
([Authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)).
The transport pages say nothing either. That is not an oversight to work around;
it means the shape of a refusal is left to HTTP, and the deployment is free to
use `429` with `Retry-After` as it already does.

**A smaller listing is not an available channel, and that is a rule rather than a
limitation of the SDK.** The same Tools page requires that the tool set "**MUST
NOT** vary per-connection or as a side effect of other requests on the
connection", while permitting it to "vary by the authorization presented on the
request, for example, returning only the tools the caller's granted scopes
permit, since credentials are per-request input, not connection state."

So the read-only narrowing this server already does is expressly allowed, because
its cause is the authorization. **Shrinking a listing because a caller has spent
its allowance is expressly forbidden**, because the cause is other requests on
the connection. A policy module must not offer that as an option, and an earlier
draft of this document listed it as one.

- **A `tools/call` refusal has a channel the listing does not.** A tool execution
  error (`isError: true`) carries actionable text, and clients **SHOULD** give
  those to the model so it can self-correct, where protocol errors are described
  as "less likely to result in successful recovery". A refusal a model should
  react to belongs there rather than in a JSON-RPC error.
- **A `tools/list` result carries no error flag**, and per the rule above it must
  not be narrowed to express one either, which leaves the transport-level refusal
  as the only way to answer a listing the caller may not have.
- **A closed stateful session carries nothing at all.** ADR-0015 records that
  ending a watch has to be expressed by cancelling the handler's context so the
  SDK emits the result, because `SubscriptionsListenResult` cannot be constructed
  by application code.
- **`429` with `Retry-After` is the channel that does exist**, and is what both
  the HTTP gate and GitLab itself use.

A policy module therefore decides, and the layers keep enforcing, which is what
the issue already proposed. This specification adds two constraints on what it may
decide: the decision has to be expressible in the channels above, so a policy that
can only be applied by closing a session is one this server cannot state to a
client; and it may never be expressed by narrowing a listing, because the
specification reserves that for the authorization presented on the request.

## What this repository already decided

Recorded here so the specification captures existing policy rather than proposing
a different one by accident:

- **ADR-0018**: admission asks for the minimum scope, and writes are gated per
  action. A `read_api` token is admitted and served a read-only surface.
- **ADR-0019**: audience binding is unavailable at the authorization server, so
  `--oauth-client-uid` is the specification's "otherwise verify" alternative.
- **ADR-0020**: one MCP server per configuration shape, with the credential bound
  per request; what is per credential is the pool entry.
- **Issue 540**: a share granted to a mintable key gives the honest tenant one unit
  and the attacker one per credential.
- **Issue 561**: refusing a newcomer is worse than evicting an incumbent, for the
  same reason.
- **PR #789 and issue 790**: a credential the pool already holds is exempt from an
  address block, and the distinct-credential budget counts what a caller cannot
  avoid producing. The second is the first limit here already keyed on something
  closer to a principal than a token.

## Proposed shape

A module that **decides and does not enforce**, as the issue asks. Three things
belong to it:

1. **`Principal`**: what identifies this caller. Resolved from the credential, with
   the GitLab user id where it is known and the token digest where it is not, and
   carrying which of the two it is, because a limit may legitimately want to refuse
   to apply when the identity is the weaker one.
2. **`Allowance`**: what that principal may hold, per resource. The current values,
   unchanged, expressed once.
3. **`Charge`** and **`Verdict`**: what a request costs and what happens when it
   exceeds the allowance, restricted to the channels above.

The layers ask and apply. A stream ceiling is still applied where streams are
counted, a token bucket where the method is dispatched.

### What must not be assumed

- **The user id is not always available.** A credential the instance refused has
  none, and an unreachable GitLab yields none. The module has to answer with the
  weaker identity rather than block, or a GitLab outage becomes a lockout.
- **Two tokens of one user are not interchangeable.** They may carry different
  scopes, so they get different surfaces; sharing an allowance between them must
  not merge anything else about them.
- **A shared allowance is a behaviour change**, and this specification does not
  make it. Moving a limit from the token to the user reduces what an honest user
  holding several tokens may do, which is the kind of change the issue puts out of
  scope. It belongs in its own change, with its own measurement.

## The three questions, answered

The first draft left these open. They were worked through on 2026-09-22 against
the code and against the two issues whose reasoning binds this one.

### 1. Eviction keeps the entry, and is the one decision that should

**No, it does not need the principal.** What the pool bounds is memory, and
memory is held per entry, not per principal. `lruVictimLocked`
(`internal/serverpool/pool.go:1344`) walks the list from the tail for the first
entry the caller does not report busy, and takes the tail when every entry is
busy. Nothing in that needs to know who owns an entry.

A principal would buy one thing only: **fairness between principals**, so an
address holding a hundred quiet entries would be preferred over a stranger
holding one. Issue 561 already refused the remedy that would need
("**Do not add a per-credential or per-busy-entry share**", because it is what
issue 540 rejected, keyed on the same mintable key), and it refused it on
evidence rather than taste: with a pool of three and one busy subscriber, fifty
quiet arrivals produced forty-eight evictions and never touched the subscriber,
and the entry taken next was the attacker's own freshest quiet one.

A non-mintable key would reopen that door, since the reason for the refusal was
the key. But **acting on it is a behaviour change** and belongs to its own
change, not to this one. Eviction stays keyed on the entry.

### 2. A single-user deployment is the argument for keeping this small

Every limit collapses onto one principal, and the per-process ceilings become the
only real bound: 512 listen streams (`maxListenStreamsPerProcess`) and 512
watchers (`maxWatchersPerProcess`, added by issue 561's fourth point). Nothing
breaks.

What it settles is a matter of proportion. Stdio, the default and by far the
commonest deployment, has one credential and one user, so a per-principal policy
does nothing there at all. The module has to be worth its weight in the HTTP
multi-tenant case alone, and a design that complicates the single-user path to
serve the shared one has the trade backwards.

### 3. The tier is not a policy decision, and it is not a property of the caller either, but today it behaves like one

Conceptually it belongs to the instance. **In practice it varies by token, and by
accident.** `DetectTier` (`internal/gitlab/client.go:476`) resolves it from
`GET /license`, which is **admin-only on self-managed**, and falls back to Free on
any error: a non-admin token, a CE instance, or an API failure. There is no other
detection path in the tree.

Two consequences follow that nothing documents:

- A non-admin token on a self-managed **Ultimate** instance resolves **Free**, and
  is served the Free catalogue.
- On **GitLab.com**, where `/license` is not available to ordinary users at all,
  detection always falls back to Free, so Premium and Ultimate surfaces are
  reachable only by setting the tier explicitly.

That is not a policy decision to move into the module. It is a detection defect,
filed as [issue 899](https://github.com/jmrplens/gitlab-mcp-server/issues/899)
rather than absorbed here. The tier stays where it is.

## Not in scope

Changing any limit's value or any current behaviour, as the issue states. This
document records where the decisions live and what they are allowed to lean on.
