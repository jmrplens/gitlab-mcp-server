# ADR-0017: Pull-Safe Event Sources Surveyed and Declined

## Status

Accepted — 2026-08-25.

## Context

[ADR-0015](adr-0015-polled-resource-subscriptions.md) shipped
`resources/subscribe` by polling and reserved a seam (NEU-002) for "a
genuinely pull-safe future events API".
[ADR-0016](adr-0016-no-webhook-ingestion.md) declined inbound webhook push
and narrowed that seam to pull-safe sources specifically.

This ADR records the survey of every pull-safe candidate GitLab offers
today — something a process can poll or stream with only the caller's own
token, no inbound endpoint, no server-held secret. The survey combined
documentation, GitLab source reading, and live probes against a self-managed
CE 19.3 instance and GitLab.com (read-only GETs and one GraphQL
introspection query, authenticated with the caller's own tokens).

The economic frame matters as much as the candidates: the shipped watcher
load is small. Ten watchers at the 5s floor — the worst case the bounds
allow — cost 120 requests a minute, which is 6% of GitLab.com's 2,000
requests per minute per user. On self-managed instances the authenticated
API throttle is **disabled by default** (live-confirmed: the probed
instance emits no `RateLimit-*` headers at all). There is no budget problem
to solve; only latency could improve, and only for some kinds.

## Decision

Adopt none of the surveyed sources. Polling through the registered resource
handlers, exactly as ADR-0015 shipped it, remains the only freshness
mechanism. What the survey found is recorded here — including one genuine
discovery held for later — so none of it is re-litigated from scratch.

## Survey Verdicts

### Events API (REST) — declined

`GET /projects/:id/events` could in principle act as a batched change
pre-check: one call per project per tick, re-reading through the
authoritative `Reader` only on a hit. Live probing and source reading
killed it on four grounds:

- **Structurally blind to every busy-floor kind.** GitLab's
  `EventCreateService` records no events for pipelines, jobs, deployments,
  environments, releases, label definitions, or feature flags — precisely
  the resources the 5s floor exists for. A pre-check that cannot see CI
  cannot accelerate the watches that matter most.
- **A false negative is forever.** The manager notifies only on digest
  change, so a change the events feed misses (title edits, description
  edits, and note edits produce no reliable event) is suppressed until a
  backstop re-read — turning a ≤5s detection into minutes on exactly the
  edits people subscribe to an issue to see.
- **The filters are unreliable live.** `after=` is date-granular with
  exclusive end-of-day semantics and 500s on datetime forms;
  `action`/`target_type` filters hit statement timeouts on large projects
  and fail open on invalid values; `GET /groups/:id/events` does not exist
  (404), leaving the group-scoped and personal-snippet kinds unreachable.
- **The arithmetic does not pay.** For realistic watch mixes the pre-check
  saves under 10% of a budget already at 6% utilization; break-even
  requires at least four base-cadence watchers sharing one project, a
  clustering no deployment has demonstrated.

Should that clustering ever materialize, the shape to build is a memoizing
hinter wrapped behind the `Reader` seam (one events call per project,
answers cached for the tick, digest machinery untouched), keyed on the
event-id watermark — comparing oldest-page id against the watermark, since
event ids are instance-wide and page-contiguity checks are incoherent. Any
whitelist of kinds it serves must respect that issues and merge requests
are busy-capable (they reach the 5s floor while open) and cannot be treated
as settled-tier.

### GraphQL subscriptions over ActionCable — the real discovery, held

The one live-proven pull-safe stream: GitLab's `/-/cable` websocket accepts
a PAT as bearer auth (since GitLab 16.7; confirmed working on CE Free
19.3), and the GraphQL schema's `Subscription` root carries GA fields for
issue and merge-request updates and CI job events — exactly the busy-floor
kinds where cadence-bounded latency hurts. Declined today because:

- It is undocumented for API consumers; the batching fields
  (`ciPipelineStatusesUpdated`, `namespaceWorkItemChanges`) are
  Experiment-status.
- There is no replay. A disconnect silently loses events, so polling must
  continue as reconciliation regardless — the stream saves approximately
  zero requests and buys only latency.
- It adds a websocket-per-token lifecycle to the HTTP server pool, a class
  of long-lived state the pool deliberately does not hold.

If adopted later, the integration shape is fixed now: a received event
triggers an immediate re-read through the `Reader` seam. The stream is a
wake-up, never a content source — a websocket frame cannot answer "what
does this subscriber's own token see?", which is the invariant ADR-0015's
change detection rests on.

### Conditional requests (ETag / If-None-Match) — declined, measured

Live-probed against real endpoints: GitLab serves ETags, but a 304 response
**costs one full rate-limit unit** (the `ratelimit-observed` counter
increments on the 304 itself) and a full server-side render (each 304
carries a fresh `correlation_id`). There is no GitHub-style 304 exemption,
no `If-Modified-Since` support (no endpoint emits `Last-Modified`), and
GitLab's Redis-cheap ETag middleware covers only frontend routes that
reject PAT auth outright (302 to sign-in). Best case is ~15 MB/h of egress
saved — a bandwidth win, not a budget one — against a new
stale-replay bug class threaded through every read. Not worth building.

### Prior art — nothing to copy

GitLab's official MCP server is tools-only; no surveyed community GitLab
MCP server implements change detection at all. glab, Renovate and
Sourcegraph poll naively or on fixed timers. The shipped adaptive poller
(5s busy / 15s default / 60s settled / 10min demoted) is already ahead of
every shipped integrator found.

## Consequences

### Positive

- POS-001: No second freshness source that can disagree with the
  authoritative read; the digest/baseline/lease logic keeps its single
  writer.
- POS-002: The dead ends are recorded with live evidence, so "just add
  If-None-Match" and "just use the Events API" are answered without
  re-probing.
- POS-003: The one genuine discovery (PAT-authenticated ActionCable) is
  preserved with its adoption criteria instead of being lost as a footnote.

### Negative

- NEG-001: Subscription latency stays cadence-bounded — ≤5s busy, up to
  60s settled, up to 10min demoted. Nothing GitLab offers today changes
  that within this server's constraints.

### Neutral

- NEU-001: Three code comments overstated the premise "a self-managed
  instance throttles at 120 requests a minute by default" — that throttle
  exists but ships disabled. The wording is corrected alongside this ADR;
  the cadence constants it motivated are untouched, because staying small
  against an *optional* budget is still the right posture for a server
  sharing the user's own token.

## Revisit Triggers

Reopen this decision when any of the following holds:

1. GitLab documents the GraphQL `Subscription` root for API consumers.
2. `ciPipelineStatusesUpdated` or `namespaceWorkItemChanges` graduate to GA.
3. The Events API gains pipeline/job/deployment/environment target types.
4. GitLab exempts 304 responses from rate-limit accounting.
5. A real deployment demonstrates four or more base-cadence watchers
   routinely sharing one project.

## Claims Resting on Unverified Ground

- Event write latency ("seconds, via Sidekiq") is inferred from source
  reading, not a documented contract; the Events API's real freshness floor
  was not measured end to end.
- ActionCable behavior was probed on CE 19.3 and GitLab.com only; older
  supported versions were not probed.

## References

- ADR-0015: Polled Resource Subscriptions (NEU-002 is the seam this
  survey exercised)
- ADR-0016: No Webhook Ingestion (the push half of the same question)
- `internal/subscriptions/manager.go` — the `Reader` seam and cadence
  constants
- GitLab docs: Events API, GraphQL API, user and IP rate limits
