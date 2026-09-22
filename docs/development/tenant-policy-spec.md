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

The constraint the issue names is real and is the reason a policy module cannot
simply refuse everywhere:

- **A `tools/list` result carries no error flag.** A caller over its allowance
  cannot be told so through the listing; the only channels are a smaller listing
  or a transport-level refusal.
- **A closed stateful session carries nothing at all.** ADR-0015 records that
  ending a watch has to be expressed by cancelling the handler's context so the
  SDK emits the result, because `SubscriptionsListenResult` cannot be constructed
  by application code.
- **`429` with `Retry-After` is the channel that does exist**, and is what both
  the HTTP gate and GitLab itself use.

A policy module therefore decides, and the layers keep enforcing, which is what
the issue already proposed. This specification adds only that the decision has to
be expressible in the channels above: a policy that can only be applied by closing
a session is a policy this server cannot state to a client.

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

## Open questions

1. **Does the busy-skip need the principal at all?** Eviction is about the pool's
   size, and the pool holds entries rather than principals. It may be the one
   decision that legitimately stays keyed on the entry.
2. **What does the module do on a self-managed instance where one user is the only
   user?** Every limit collapses onto one principal, which is correct and also
   makes the per-process ceilings the only real bound.
3. **Is the tier a policy decision at all?** It is a property of the instance, not
   of the caller, and the module may be the wrong home for it.

## Not in scope

Changing any limit's value or any current behaviour, as the issue states. This
document records where the decisions live and what they are allowed to lean on.
