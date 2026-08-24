# Resource Subscriptions

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, integrators, operators

`resources.subscribe` lets a client ask to be told when a resource changes,
instead of re-reading it on a hunch. This server honors that by polling
GitLab, under bounds described below.

Advertised only when `CAPABILITY_SURFACE=full` (the default) — the surface
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

26 kinds of resource, all of them single objects with a lifecycle worth
watching: a project or group; a pipeline, its job list, the latest pipeline
on a project, and a single job; a merge request with its discussions and
notes; an issue; a deployment, an environment, a feature flag; a release, a
tag, a branch; a milestone (project or group), a label (project or group), a
board; a deploy key; a snippet (project or personal); a wiki page; and a
repository file.

**Collections are deliberately excluded.** Subscribing to
`gitlab://project/42/issues` would notify on every change to any issue in
the project — most of them nothing the subscriber asked about — and cost a
full page read on every poll. A subscription to a collection is refused.

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

| Event                                     | Effect                                | Told to the client?              |
| ----------------------------------------- | ------------------------------------- | -------------------------------- |
| `resources/unsubscribe`                   | Stops when the last subscriber leaves | It asked for it                  |
| The session disconnects                   | Stops                                 | It is gone                       |
| 30 minutes with no traffic on the session | **Slows to a 10-minute poll**         | `_meta` on the next notification |
| Any request on the session                | Restores full speed                   | —                                |
| The resource returns 401, 403 or 404      | Stops                                 | Stream closed (2026-07-28)       |
| 24 hours, renewals included               | Stops                                 | Stream closed (2026-07-28)       |
| Evicted to make room at the cap           | Stops                                 | Stream closed (2026-07-28)       |

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

## Limits

| Limit               | Value                      | Why                                                                          |
| ------------------- | -------------------------- | ---------------------------------------------------------------------------- |
| Watchers per server | 10                         | A server is one token, so this is the ceiling on one user's polling          |
| Floor interval      | 5s                         | Ten watchers at 5s would be 120 requests a minute — an entire default budget |
| Rate-limit pause    | 30s, doubling to 5 minutes | GitLab enforces limits per user, so one 429 pauses every watcher             |

At the 15s base, ten watchers cost 40 requests a minute. Once demoted, the
same ten cost one. A self-managed instance's default is 7,200 authenticated
requests an hour (120 a minute), and that limit is disabled unless an
administrator enables it.

When the cap is reached, a new subscription evicts the **longest-demoted**
watch rather than being refused — a subscriber who is actively waiting wins
the slot over one whose own inactivity already devalued it, and the watch
that has been slow the longest is the one to go. If every watch is still
active, the new subscription is refused with an error.

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
