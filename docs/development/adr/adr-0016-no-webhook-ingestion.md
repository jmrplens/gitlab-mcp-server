# ADR-0016: No Webhook Ingestion

## Status

Accepted — 2026-08-25.

## Context

`client-go` v2.58.2 ships 23 parsed webhook event types
(`event_webhook_types.go`) plus `ParseHook`, `ParseWebhook` and
`ParseSystemhook` (`event_parsing.go`). This repository references none of
them. It manages hooks as data — `gitlab_project_hook_*`,
`gitlab_group_hook_*`, `gitlab_system_hook_*` create, list, edit and delete
them — but has never consumed a delivery.

[ADR-0015](adr-0015-polled-resource-subscriptions.md) shipped
`resources/subscribe` by polling, and left the door open for something
better: NEU-002 says "if GitLab ever ships an events API reachable from
this process, the reader interface is the seam to swap". Webhooks are the
obvious candidate for that events API, and `internal/subscriptions.Reader`
(`Read(ctx, uri) ([]byte, error)`) is a genuinely push-shaped seam — nothing
in `Manager` assumes the content came from a poll.

The question this ADR settles: should this server accept inbound webhook
deliveries, in any transport mode, for any purpose?

## Decision

No. This server does not run a webhook receiver, in stdio mode or HTTP
mode, stateful or stateless, now or as a documented direction. The `Reader`
seam stays exactly as shipped in ADR-0015 — polling only.

Hook management tools (`gitlab_project_hook_*` and friends) are unaffected:
they remain ordinary CRUD over hook configuration, with no special
relationship to subscriptions. A user who wants GitLab to push to their own
external service configures that themselves; this server is never on that
path.

## Rationale

1. **The safe shape doesn't buy enough.** A pull-only design — a tool that
   fetches a hook's own delivery log (`GET .../hooks/:hook_id/events`, which
   `client-go` has no wrapper for) and parses it with `ParseHook` for
   inspection — avoids every security problem below, because it never
   listens for anything and every read is gated by the caller's own token
   like any other tool. But it answers a different, narrower question ("what
   was delivered, historically, to a hook that already exists") than
   "accelerate subscriptions", and it is speculative: whether a delivery
   log's `request_data` round-trips cleanly through `ParseHook`'s exact-field
   unmarshal is unverified, and GitLab's `trigger` field
   (`push_hooks`, snake_case) does not obviously match `client-go`'s
   `EventType` constants (`Push Hook`, Title Case). It may be worth building
   later, on its own merits, as a debugging tool — not as part of this
   decision.

2. **Every push-shaped design has a control-bypass at its core.** The two
   architectures that would actually lower subscription latency — an
   HTTP-mode receiver that auto-provisions a project hook when a client
   subscribes, or the same behind a routing scheme to contain the blast
   radius — both make `resources/subscribe`, a nominally read-only
   operation, silently create a mutating GitLab-side artifact as a side
   effect. Neither routes that mutation through `GITLAB_READ_ONLY`,
   `GITLAB_SAFE_MODE`, or this project's elicitation-consent pattern for
   mutations. That is not a scope-tuning problem to fix with a smaller
   design; it is a policy violation baked into the mechanism itself
   (auto-provisioning a hook at subscribe time). Fixing it — consent,
   permission-failure fallback, a security review of a new public endpoint —
   does not shrink the design; it grows it.

3. **It would be this server's first unauthenticated public inbound
   surface**, secured by GitLab's per-hook secret token compared against
   whatever this process stored for it — meaning a new secret store, the
   first piece of server-owned durable state this codebase has ever needed.
   Every other piece of state here is either the caller's own GitLab token
   (never stored past the request) or purely in-memory and reconstructible
   (`internal/serverpool`'s pool entries, `internal/subscriptions`'
   watchers). A webhook secret is neither: it must survive
   `AUTO_UPDATE`'s routine restarts and pool eviction, or every hook
   silently starts failing signature checks, and losing it does not merely
   degrade a poll — it either accepts unverified deliveries or accepts
   none.

4. **A delivery cannot answer the question a subscription needs answered.**
   `resources/subscribe` is per-subscriber, and each subscriber's
   entitlement is a property of their own GitLab token. A webhook delivery
   arrives with no token at all. Routing one to the right subscribers
   without leaking requires re-deriving entitlement by re-reading through
   that subscriber's own token anyway (`Reader`, exactly as ADR-0015 already
   does it) — at which point the delivery has bought only a wake-up hint,
   not a shortcut past the read. That is a real latency win when there is
   sustained polling load to relieve, and this project has never measured
   whether there is.

5. **Coverage is partial even in the best case.** Fewer than half of the 26
   subscribable resource kinds (`internal/subscriptions/kind.go`) have a
   clean, direct match to a parsed webhook event type. Several more only get
   partial signal — a comment event stands in for a discussion resource, a
   push event stands in for a single file or a branch. Board, label, deploy
   key and environment changes have no corresponding webhook event at all.
   A webhook path would need the polling path to remain fully functional
   for everything it does not cover, so it is additive complexity on top of
   ADR-0015, never a replacement for any part of it.

## Consequences

### Positive

- POS-001: No new inbound HTTP surface, authenticated or not.
- POS-002: No secret store, and therefore nothing new to lose across a
  restart or an eviction.
- POS-003: `resources/subscribe` stays what ADR-0015 built it to be: a
  read-only operation with no GitLab-side side effect.
- POS-004: `internal/subscriptions.Reader` stays exactly as shipped —
  nothing here constrains a future change to it.

### Negative

- NEG-001: Subscription latency stays a poll interval (5s floor, 60s
  settled) rather than near-real-time for the resource kinds a webhook
  could have covered.
- NEG-002: `client-go`'s 23 parsed webhook event types and `ParseHook` /
  `ParseWebhook` / `ParseSystemhook` remain unexercised by this codebase.

### Neutral

- NEU-001: This does not foreclose a pull-only delivery-inspection tool
  (reading a hook's own delivery log on request) as separate, later,
  opt-in work — see Rationale item 1. It answers a different question than
  this ADR and carries no inbound-surface risk.
- NEU-002: If GitLab ever ships a genuinely pull-safe events API — one a
  server can poll or subscribe to without exposing an inbound endpoint or
  holding a shared secret — ADR-0015's NEU-002 seam still applies. This ADR
  narrows that seam's scope to exclude inbound webhook push specifically;
  it does not close it.

## Alternatives Considered

- **A. Pull-only delivery inspection.** A tool that fetches and parses a
  hook's own delivery log. Not disqualified, just out of scope here — see
  Rationale item 1. The correct shape for any future webhook-adjacent tool
  in this codebase: pull-only, per-caller-authorized, no new inbound
  surface.
- **B. Webhook-accelerated subscriptions, contained.** An HTTP-mode receiver
  with a routing scheme designed to limit the blast radius of a bad
  delivery. Rejected: the hook-auto-provisioning mutation at the core of it
  bypasses `GITLAB_READ_ONLY` / `GITLAB_SAFE_MODE` regardless of how well
  the receiver around it is contained.
- **C. Webhook-accelerated subscriptions, full.** The same, without the
  containment scheme. Rejected for the same core defect, plus: the first
  unauthenticated public endpoint this project would ship, hand-rolled
  signature verification with no library support in `client-go`, and a
  dependency on pool-eviction cleanup infrastructure that does not exist.
- **D. Measure current poll load first, then decide.** Reasonable, and
  still available later — nothing here forecloses building any of the above
  once there is evidence sustained polling load actually needs relieving.
  Declining now does not require that measurement, because the
  control-bypass and secret-storage objections apply independently of load.

## References

- ADR-0010: No Resource Subscribe Capability
- ADR-0015: Polled Resource Subscriptions
- `internal/subscriptions/manager.go` — the `Reader` seam
- `internal/subscriptions/kind.go` — the 26 subscribable resource kinds
