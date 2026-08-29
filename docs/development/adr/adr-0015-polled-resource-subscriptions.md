# ADR-0015: Polled Resource Subscriptions

## Status

Accepted — 2026-08-24. Supersedes [ADR-0010](adr-0010-no-resource-subscribe.md).

## Context

[ADR-0010](adr-0010-no-resource-subscribe.md) decided not to advertise
`resources.subscribe`, on three grounds: there is no push channel from
GitLab into this process, polling would amplify rate-limit pressure, and no
client had asked for it.

The first ground still holds and always will. The third turned out to be
wrong, and in a way that matters:

- **VS Code subscribes to every resource it reads**, on protocol
  `2025-11-25`, and routes `notifications/resources/updated` into its
  file-change pipeline.
- **Cursor sends `resources/subscribe` even to servers that advertise
  `subscribe: false`.** Declining the capability does not stop a client from
  asking; it only stops the server from answering. Those subscriptions were
  already being made against this server and already going nowhere.
- MCP itself moved: from protocol `2026-07-28` a subscription is a
  long-lived client request (`subscriptions/listen`, SEP-2575) rather than a
  fire-and-forget RPC, which gives a server a sanctioned way to end one.

That leaves the second ground — polling costs API budget — as the real
question. It is a budget question, not a principle, and a budget can be
bounded.

## Decision

Advertise `resources.subscribe` and honor it by polling, under bounds that
make the worst case something an operator can predict.

- **A whitelist decides what is subscribable.** 26 resource kinds — single
  objects with a lifecycle worth watching. Collections are excluded: a
  project's issue list changes for reasons a subscriber did not ask about,
  and polling one costs a full page read per tick.
- **Cadence follows the resource.** A running pipeline polls at a 5s floor;
  a finished one at 60s; anything without a lifecycle field at 15s. GitLab
  has no terminal state — a retried pipeline reuses its ID and starts
  running again — so nothing ever retires itself for being "done".
- **Ten watchers per server, and a server is one token.** That is the
  ceiling on concurrent polling and on concurrent outbound requests.
- **The session bounds the watch.** When the subscribing session
  disconnects, its watches stop. The SDK does not do this for us: it drops a
  disconnected session from its subscriber table without ever calling the
  unsubscribe handler.
- **An unrenewed lease slows a watch down; it does not end it.** After 30
  minutes without traffic on the session, a watch drops to a 10-minute poll.
  Any request that reaches the server on that session restores it. An
  absolute 24-hour cap is the only deadline that stops a watch on time
  alone.

  **An open `subscriptions/listen` stream renews its own watch.** Session
  traffic is a proxy for "is anyone still there"; an open listen request
  answers that question directly and better, since the subscriber is
  demonstrably still connected and still reading. It is also the only answer
  available on the transport this server ships by default: every stateless POST
  is its own session, so a listen stream's session sees one request — the listen
  itself — and nothing would ever renew it. Without this the feature would
  degrade by a factor of forty half an hour in, while the client sat on a stream
  it was still reading. The lease continues to govern the legacy
  `resources/subscribe`, which has no stream to speak for it, and `MaxLifetime`
  still caps every watch regardless.
- **A subscriber is an identity, not a count.** Watches are held by the
  session that asked for them, so subscribing twice is idempotent and one
  session cannot release another's watch.

  **That identity is the session for the legacy `resources/subscribe` and the
  listen stream for `subscriptions/listen`.** This decision was written before
  the 2026-07-28 path existed, and the specification has moved past it: that
  revision makes the listen request the subscription's identity, and a session
  may hold several. The SDK unsubscribes every URI a listen carried when the
  listen ends, so with the session as the only identity, one stream closing
  released a watch a sibling stream was still holding — leaving that sibling
  acknowledged, open, and unable to ever fire. The bridge therefore records
  which streams hold each watch and releases it when the last one goes.
  Delivery remains per-session: `Server.ResourceUpdated` notifies sessions, not
  listen requests, and only one request ID per session per URI survives in the
  SDK's own table, so a session with two listens on a URI sees the notification
  on one of them. That half is upstream.
- **The legacy `resources/subscribe` is refused in stateless HTTP mode**,
  where the session ends with the POST that created it and no notification
  could ever be delivered. `subscriptions/listen` still works there.

  **The refusal must be scoped by request path, not by transport.** The SDK
  routes both methods through the one `SubscribeHandler`: `subscriptions/listen`
  calls it once per resource URI it carries and returns the first error before
  acknowledging anything. A handler that refuses every subscribe in stateless
  mode therefore refuses `subscriptions/listen` too, which is how the feature
  shipped dead in the default configuration while the handshake went on
  advertising `resources.subscribe`. The listen path is marked in its
  middleware and the refusal consults that mark.
- **A torn-down listen stream gets its completion result and no
  `notifications/cancelled`.** The 2026-07-28 cancellation page says a server
  "**MUST** send `notifications/cancelled` referencing a `subscriptions/listen`
  request ID when it tears down that subscription stream". This server does not,
  and cannot: `SubscriptionsListenResult` embeds an unexported type, so
  application code cannot construct the message, and the SDK exposes no other
  way to send one. What a client does get is the graceful completion result the
  SDK writes when the handler's context ends, which tells a conforming client
  the stream is over — so the practical gap is small, and it is recorded here
  rather than left unstated because it is a MUST that nobody in this process
  satisfies. Raised upstream.
- **Rate limiting pauses every watcher on the manager**, with exponential
  back-off, because GitLab enforces its limits per user.
- **A watch that ends for a reason the client did not ask for closes the
  client's subscription stream** on protocol 2026-07-28, rather than going
  quiet.

## Rationale

1. **The subscriptions were already happening.** A client that subscribes to
   a server advertising `subscribe: false` gets a JSON-RPC error, or — for
   Cursor — nothing at all. Either way the user's request to watch something
   was already being made and already failing. Declining the capability did
   not prevent the cost; it prevented the feature.

2. **The budget is bounded and small.** Ten watchers at the 15s base cost 40
   requests a minute; the same ten demoted after their lease cost one. A
   self-managed instance's default budget is 120 requests a minute per user,
   and that default is off unless an administrator enables it. The floor of
   5s exists precisely so that "one urgent watch" cannot become "ten
   watchers at 5s eating the whole budget".

3. **Slowing down is honest; stopping silently is not.** MCP defines no
   lease, no TTL and no expiry message, and its one notification means only
   "this changed, read it again". A watch that retired at a deadline would
   go silent in a way no client can distinguish from "nothing happened yet".
   Demotion makes the only claim the protocol lets a server make truthfully.

4. **Change detection means what a subscriber can verify.** Watchers read
   through the very handler `resources/read` dispatches to, so "the content
   changed" is exactly "what you would read changed".

## Consequences

### Positive

- POS-001: Clients that already subscribe get working notifications instead
  of silent failures.
- POS-002: The worst-case API cost is a small, stated number rather than an
  open question.
- POS-003: A watch is bounded by something real — the session — rather than
  only by a timer.
- POS-004: A subscription that the server ends is answered on the wire from
  protocol 2026-07-28, rather than left hanging.

### Negative

- NEG-001: The server now makes GitLab requests that no tool call asked for,
  which ADR-0010's POS-003 explicitly valued. Every one of them is
  attributable to a subscription a client opened.
- NEG-002: Latency is a poll interval, not a push. A change to a settled
  resource can take a minute to surface, and ten minutes once demoted.
- NEG-003: A revoked token keeps being used until the next poll returns 401,
  403 or 404 — up to one interval, and up to ten minutes for a demoted
  watch. That read is what stops the watch.
- NEG-004: More moving parts: a watcher goroutine per subscription, a
  session bridge, and middleware on the receive path.

### Neutral

- NEU-001: Nothing changes for clients that do not subscribe. The capability
  is advertised only on `CAPABILITY_SURFACE=full`, where the GitLab
  resources it applies to are registered.
- NEU-002: If GitLab ever ships an events API reachable from this process,
  the reader interface is the seam to swap; the cadence, bounds and lease
  logic are independent of where the content comes from. Webhook deliveries
  were investigated and declined as that events source — see
  [ADR-0016](adr-0016-no-webhook-ingestion.md) — but this seam remains open
  for a genuinely pull-safe future events API, not for inbound push. The
  pull-safe candidates GitLab offers today (Events API, GraphQL
  subscriptions over ActionCable, conditional requests) were then surveyed
  with live probes and declined too — see
  [ADR-0017](adr-0017-pull-safe-event-sources-surveyed.md) for the evidence
  and the triggers that would reopen this.

## Alternatives Considered

- **A. Keep declining the capability (ADR-0010).** Rejected because it does
  not stop clients from subscribing — it only guarantees they get nothing.
- **B. Webhooks.** Still rejected, for ADR-0010's reasons, which have not
  changed: stdio has no socket, per-project registration is impractical, and
  fan-out belongs in a service of its own.
- **C. Retire a watch when its resource reaches a terminal state.**
  Rejected on the facts: GitLab has no terminal state. Retrying a pipeline
  reuses its ID, a closed issue reopens, and a merged merge request can
  still be edited.
- **D. End a watch at the lease and say so with a final
  `notifications/resources/updated`.** Rejected as dishonest: the schema
  defines that notification to mean the content changed, and a client acting
  on it would perform a read that finds nothing new.
- **E. A `gitlab_watch_renew` tool.** Rejected as redundant: any request on
  the session already renews every watch on it, so a tool whose only effect
  is renewal would do nothing a tool call had not already done.
- **F. Advertising the capability only when the transport can deliver.**
  Rejected because the capability bit is shared: the SDK requires
  `resources.subscribe` for the `subscriptions/listen` path, which works in
  stateless mode. Refusing the one method that cannot be honored says more,
  and to the client that actually asked.

  The second sentence was true and the third was not, as written. Refusing "the
  one method that cannot be honored" described an intent the code could not
  carry out, because the SDK gives both methods one handler: the refusal
  reached every `subscriptions/listen` naming a resource, and a mixed listen
  lost its list-changed half along with it. So for as long as that held, the
  rejection of this alternative was resting on an outcome that was not
  happening — the capability was advertised and no method could honor it,
  which is the state F exists to avoid. The decision stands now that the
  refusal is scoped by request path; it did not stand before.

## References

- [MCP Specification — Resources (2025-11-25)](https://modelcontextprotocol.io/specification/2025-11-25/server/resources)
- [MCP Specification — Subscriptions (2026-07-28)](https://modelcontextprotocol.io/specification/2026-07-28/basic/patterns/subscriptions)
- [SEP-2575 — Stateless MCP](https://modelcontextprotocol.io/seps/2575-stateless-mcp)
- ADR-0010: No Resource Subscribe Capability (superseded by this ADR)
- [Resource subscriptions](../../reference/capabilities/subscriptions.md)
