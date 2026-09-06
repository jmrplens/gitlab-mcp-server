# Resource Subscriptions

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, integrators, operators

`resources.subscribe` lets a client ask to be told when a resource changes,
instead of re-reading it on a hunch. This server honors that by polling
GitLab, under bounds described below.

Advertised only when `GITLAB_MCP_CAPABILITY_SURFACE=full` (the default) — the surface
that registers the GitLab resources a subscription can apply to.

## Protocol

| Protocol       | How a client subscribes                                        | What it gets                                                                                   |
| -------------- | -------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| ≤ `2025-11-25` | `resources/subscribe` / `resources/unsubscribe`                | `notifications/resources/updated`                                                              |
| ≥ `2026-07-28` | `subscriptions/listen` with `resourceSubscriptions` (SEP-2575) | The same notification, plus a result on the open request when the server ends the subscription |

Both are handled. Which one is used is decided by the version negotiated at
`initialize`.

**In stateless HTTP mode — the default — only the newer path works.** The
server gives each stateless POST its own session and closes it when the POST
returns, so a legacy `resources/subscribe` would be cancelled moments after
being acknowledged and could never be notified on. Rather than accept one
and go quiet, the server refuses it and says why. `subscriptions/listen` is
unaffected, because there the subscription *is* an open request. Run with
`--stateless=false` if you need to serve legacy subscribers over HTTP.

## What can be subscribed to

26 kinds of resource: a project or group; a pipeline, its job list, the
latest pipeline on a project, and a single job; a merge request with its
discussions and notes; an issue; a deployment, an environment, a feature
flag; a release, a tag, a branch; a milestone (project or group), a label
(project or group), a board; a deploy key; a snippet (project or personal);
a wiki page; and a repository file.

The list is machine-readable in three places, all derived from the same
whitelist that enforces it (`internal/subscriptions.Templates()`), so none
can drift from the others:

- every subscribable template's **description** ends with the marker
  sentence `Subscribable: subscriptions/listen (protocol 2026-07-28).
  Resources/subscribe on stateful sessions.` — appended mechanically at
  registration, never hand-written (two sentences rather than a semicolon,
  because the served surface stays free of the characters strict gateway
  validators reject);
- every subscribable template also carries the vendor-namespaced
  **`_meta` key** `io.github.jmrplens/subscribable: true` — the spec's
  sanctioned per-object extension point, for generic clients that want to
  filter without knowing this server's manifest (the standard surface has
  no per-resource subscribable field, only the server-wide
  `resources.subscribe` capability);
- the **`gitlab://tools` manifest** carries the full list under
  `subscriptions.subscribable_uri_templates`;
- in HTTP mode, the **Server Card** at `/server-card` (also served at the
  legacy `/.well-known/mcp/server-card.json` path)
  carries a top-level `subscriptions` block with the same list plus a
  per-method `available` boolean (on stateless HTTP,
  `resources/subscribe` is listed with `available: false` and a
  `requires` note, while `subscriptions/listen` stays true), and a
  `capabilities`
  object mirroring the handshake — so a directory can learn
  `resources.subscribe` without connecting.

Three of those — a pipeline's job list, and a merge request's discussions
and notes — are lists, and subscribable on purpose: each is bounded by one
parent object whose lifecycle the subscriber is already following, so "did
this change?" is exactly the question a watch on the parent's activity
answers. What stays excluded is the open-ended, top-level collection:

Subscribing to `gitlab://project/42/issues` would notify on every change
to any issue in the project — most of them nothing the subscriber asked
about — and cost a full page read on every poll. A subscription to a
top-level collection is refused.

A refused subscription starts no watcher and costs no API call. On protocol
2026-07-28 the refusal may not reach the client: the Go SDK's client fires
`subscriptions/listen` without awaiting its response, so a server-side error
is discarded client-side. The refusal still takes effect.

## Polling cadence

| Resource state     | Interval   | Example                                         |
| ------------------ | ---------- | ----------------------------------------------- |
| Work in flight     | 5s (floor) | a `running` pipeline, an `opened` merge request |
| Settled            | 60s        | a `success` pipeline, a `closed` issue          |
| No lifecycle field | 15s        | a wiki page, a file, a label                    |

Nothing retires itself for being finished. GitLab has no terminal state — a
retried pipeline reuses its ID and starts running again, a closed issue
reopens, a merged merge request can still be edited — so a watcher that
stopped at `success` would go blind to a real change its subscriber asked
about.

Latency is cadence-bounded by design: no pull-safe event channel GitLab
offers today changes that — the Events API, GraphQL subscriptions over
ActionCable, and conditional requests were each surveyed with live probes
and declined; see
[ADR-0017](../../development/adr/adr-0017-pull-safe-event-sources-surveyed.md).

`manual` and `scheduled` count as settled: both are waiting on something
outside CI, so polling them at the floor spends budget on a resource that
cannot change until a human or a clock acts.

## Lifetime

A watch ends, or slows, for these reasons and no others:

| Event                                     | Effect                                | Told to the client?                                                                                                            |
| ----------------------------------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `resources/unsubscribe`                   | Stops when the last subscriber leaves | It asked for it                                                                                                                |
| The session disconnects                   | Stops                                 | It is gone                                                                                                                     |
| 30 minutes with no traffic on the session | **Slows to a 10-minute poll**         | `_meta` on the next notification                                                                                               |
| Any request on the session                | Restores full speed                   | —                                                                                                                              |
| The resource returns 401, 403 or 404      | Stops                                 | Stream closed with `resource_gone` (2026-07-28)                                                                                |
| 24 hours, renewals included               | Stops                                 | Stream closed with `lifetime_reached` (2026-07-28)                                                                             |
| Evicted to make room at the cap           | Stops                                 | Stream closed with `watcher_evicted` (2026-07-28)                                                                              |
| The pool evicts this credential's entry   | Stops                                 | Stream closed with `credential_evicted`, `credential_reset` or `credential_revoked`; session terminated on `--stateless=false` |
| The server is shutting down               | Stops                                 | Stream closed with `shutdown` (2026-07-28)                                                                                     |

The reason words are a closed vocabulary, listed in
[Why a subscription ended](#why-a-subscription-ended).

**The lease slows a watch down; it does not end it.** MCP defines no lease,
no TTL and no expiry message, and its one notification means "this changed,
read it again". A watch that retired at a deadline would go silent in a way
no client could tell apart from "nothing has happened yet". Slowing down is
the only claim a server can make honestly here.

Renewal is automatic: any request that reaches the server on that session —
a tool call, a resource read, a prompt — pushes every watch on it back to
full speed. Two things do not renew. Ping and the `initialize` handshake are
excluded deliberately: a ping proves a socket is open, not that anyone is
waiting on the other end of it. And from protocol 2026-07-28 a client may
answer `tools/list`, `prompts/list` or a repeated `resources/read` from its
own cache, in which case nothing arrives here to renew — which is correct,
since a client serving itself from cache is not evidence of anything.

**A watch can also end because the pool dropped the credential it belongs
to.** In HTTP mode the watchers hang off a pool entry, and
`--max-http-clients` bounds how many entries a process holds. Size pressure
prefers an entry that is not serving a subscription and takes one that is
only when every pooled entry is busy, so this is the last thing the pool
does rather than the first. When it does happen, the credential's watchers
stop, its open `subscriptions/listen` requests are completed, and under
`--stateless=false` the sessions that no stream ended are terminated.
Nothing is wrong with the credential: reconnect and subscribe again. The
operator's lever, and the pool size above which the fallback is unreachable,
are in [HTTP Server Mode](../../guides/http-server-mode.md#4-pool-eviction).

## Limits

| Limit                   | Value                      | Why                                                                                                                                          |
| ----------------------- | -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Watchers per credential | 10                         | A watcher polls with the subscriber's own token, so this is the ceiling on one credential's polling                                          |
| Watchers per process    | 512                        | A credential is one API call to mint, so a per-credential number multiplies by however many a caller holds; only this one bounds the process |
| Floor interval          | 5s                         | Ten watchers at 5s would be 120 requests a minute — an entire default budget                                                                 |
| Rate-limit pause        | 30s, doubling to 5 minutes | GitLab enforces limits per user, so one 429 pauses every watcher                                                                             |

At the 15s base, ten watchers cost 40 requests a minute. Once demoted, the
same ten cost one. A self-managed instance's default is 7,200 authenticated
requests an hour (120 a minute), and that limit is disabled unless an
administrator enables it.

When the per-credential cap is reached, a new subscription evicts the
**longest-demoted** watch rather than being refused — a subscriber who is
actively waiting wins the slot over one whose own inactivity already devalued
it, and the watch that has been slow the longest is the one to go. If every
watch is still active, the new subscription is refused with an error.

The process ceiling only ever refuses. Making room there would mean stopping
some other credential's watch to start this one, and nothing about the arriving
subscription makes it worth more than the one already running; within a
credential the demoted-watch eviction above still applies, because that is a
subscriber reclaiming a slot it already holds. Both refusals carry `-32000`: the
process one says `server-wide` in as many words, and the per-credential one
carries that credential's own count and limit, so a client's error says which
ceiling it met and an operator reading it knows whether the number to look at is
that caller's or the deployment's. Neither ceiling is configurable: the
per-credential one multiplies by however many credentials a caller holds, and an
operator who could raise the process one could undo the bound. The lever an
operator does have is `--max-http-clients`, which decides how many credentials
the pool serves at once.

## Error codes

A refused legacy `resources/subscribe` carries a deliberate JSON-RPC error
code, so a generic client never sees the accidental `code: 0` a plain
error would marshal as:

| Refusal                                                                    | Code                                                                        | Retry later?                     |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------- | -------------------------------- |
| URI is deliberately not subscribable                                       | `-32602` (invalid params)                                                   | No — pick a subscribable URI     |
| Resource unreadable on the authorization read (401/403/404)                | `-32602` — the code the SDK itself answers an unknown `resources/read` with | Only if the resource appears     |
| Rate limited, watcher cap with no evictable watch, or server shutting down | `-32000` (implementation-defined server busy)                               | Yes — the condition is transient |
| Watchers per process at 512                                                | `-32000`, with `server-wide` in the message                                 | Yes, the condition is transient  |
| Transient GitLab failure on the first read                                 | `-32603` (internal error)                                                   | Yes                              |
| `resources/subscribe` on stateless HTTP                                    | `-32600` (invalid request for this session state)                           | No — use `subscriptions/listen`  |

The message beside the code preserves the upstream failure detail
verbatim; the retry behavior is defined by the code, per the table. On
protocol 2026-07-28 a `subscriptions/listen` refusal is delivered by the
stream closing instead; the SDK's client discards the response of the
listen call it fires, so the codes above are observable on the legacy
method and in server logs.

## Notification `_meta`

Every `notifications/resources/updated` carries the state of the watch that
produced it, under a vendor key. MCP registers no key for subscription
lifetime, so this is namespaced to this project:

```json
{
  "method": "notifications/resources/updated",
  "params": {
    "uri": "gitlab://project/42/pipeline/99",
    "_meta": {
      "io.github.jmrplens/watch": {
        "state": "active",
        "renewBy": "2026-08-24T19:30:00Z",
        "pollIntervalMs": 5000,
        "renewedByActivity": true
      }
    }
  }
}
```

| Field               | Meaning                                                |
| ------------------- | ------------------------------------------------------ |
| `state`             | `active` at full speed, `slow` once demoted            |
| `renewBy`           | When the lease runs out and the watch slows down       |
| `pollIntervalMs`    | The cadence the watch is running at right now          |
| `renewedByActivity` | Any request on this session pushes `renewBy` out again |

No client is obliged to read any of it, and no client known today does. It
is included because a subscriber has no other way to learn that its watch
slowed down, and because everything it says is true whether or not anyone
looks.

## Why a subscription ended

On protocol 2026-07-28 a subscription is a `subscriptions/listen` request the
client leaves open, and the server ends it by answering it. Every ending the
**server** initiates carries a reason in that result's `_meta`, under a vendor
key namespaced to this project, beside the subscription id the SDK stamps
there:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "_meta": {
      "io.modelcontextprotocol/subscriptionId": 1,
      "io.github.jmrplens/watch-end": {
        "reason": "credential_evicted",
        "detail": "the server released this credential's pooled entry under capacity pressure; reconnect and subscribe again, the credential itself is still valid"
      }
    }
  }
}
```

Without it, all seven endings below are one bare result: a client cannot tell
"the server ran out of room" from "your token was revoked", and the right
response to those two is not the same.

| `reason`             | What happened                                                                                      | What to do                                                             |
| -------------------- | -------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `credential_evicted` | Size pressure took this credential's pooled entry                                                  | Reconnect and subscribe again now; the credential is still valid       |
| `credential_reset`   | The entry was reclaimed for idleness, for the credential-recheck ceiling, or to rebuild its server | Reconnect and subscribe again; nothing is wrong with the credential    |
| `credential_revoked` | GitLab refused the credential, at revalidation or on a call                                        | Re-authenticate first; the same token will be refused again            |
| `resource_gone`      | The watched resource answered 401, 403 or 404, so the watch retired                                | Check access; subscribe again only if the resource comes back          |
| `lifetime_reached`   | The absolute 24-hour watch lifetime ran out                                                        | Subscribe again; expected on a long-lived client                       |
| `watcher_evicted`    | One of this credential's own demoted watches was stopped at the watcher cap                        | Subscribe again, and keep the session busy so the watch is not demoted |
| `shutdown`           | The server is stopping                                                                             | Reconnect; behind a balancer another instance will take you            |

Three things are worth knowing about the shape:

- **`detail` is a sentence, not a restatement.** A client that does not
  recognize a reason word can act on the prose, and so can a person reading a
  frame in a log.
- **`status` appears only on `resource_gone`**, and only when the failed read
  carried an HTTP status. It is relayed, not interpreted: GitLab answers 404
  for a resource the caller may not see, precisely so that it cannot be told
  apart from one that does not exist, so a 404 here does **not** mean deleted.
- **The vocabulary is closed and published.** The `subscriptions.end_reasons`
  array of the [server card](../../guides/http-server-mode.md#server-card)
  lists it, so a
  client can learn the set without meeting each value in production. Treat an
  unrecognized value as "ended for a reason this client does not know", never
  as "any ending".

**A client-initiated ending carries no reason**, deliberately. If the client
cancels its own listen or disconnects, it caused the ending, and the result may
never reach it at all.

**A session-era `resources/subscribe` carries no reason either**, and this is a
real limitation rather than an oversight. That subscription holds no open
request, so there is nothing to answer: on `--stateless=false` the only ending
the protocol offers is terminating the session, which is what the server does.
What such a client observes is its standalone SSE stream ending and its next
request being answered as a new session. The `logging` capability would be the
one place left to carry an advisory string, and this server deliberately does
not declare it, because SEP-2577 deprecated it; declaring a deprecated
capability to carry one sentence is the wrong trade. Use `subscriptions/listen`
if you want the reason.

## Change detection

A watcher re-reads through the same handler `resources/read` dispatches to,
and compares a SHA-256 of the content. "The content changed" therefore means
exactly "what you would read changed" — the notification is never about
something the subscriber cannot observe.

The baseline is taken synchronously when the subscription is accepted, so a
subscriber is never notified about a change that predates it. That first
read doubles as the authorization check: it runs with the subscriber's own
token, so a URI the token cannot read is refused up front rather than
accepted and then silently never firing.

## Client behavior

| Client               | Protocol     | Notes                                                                                              |
| -------------------- | ------------ | -------------------------------------------------------------------------------------------------- |
| VS Code              | `2025-11-25` | Subscribes to every resource it reads; routes updates into its file-change pipeline, not into chat |
| Cursor               | `2025-11-25` | Sends `resources/subscribe` even to servers advertising `subscribe: false`                         |
| Go SDK client        | `2026-07-28` | Discards the response to `subscriptions/listen`, so it never sees a refusal or a graceful close    |
| Your own integration | any          | The `_meta` above is there for you; the notification alone is enough without it                    |

## See also

- [ADR-0015: Polled resource subscriptions](../../development/adr/adr-0015-polled-resource-subscriptions.md) — the decision and its bounds
- [ADR-0010: No resource subscribe capability](../../development/adr/adr-0010-no-resource-subscribe.md) — the decision this replaced
- [Resources reference](../resources.md) — what can be read
