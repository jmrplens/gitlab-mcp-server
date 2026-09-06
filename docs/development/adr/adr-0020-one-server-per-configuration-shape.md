# ADR-0020: One MCP server per configuration shape, with owner-filtered notification delivery

## Status

Accepted (2026-09-05).

## Context

In HTTP mode the pool built one `mcp.Server` per credential, so the resident
set of the process was a straight line in the number of pooled credentials.
The [resource benchmark](../../reference/resource-benchmark.md) measured the slope
at 130.9 MiB per credential on the dynamic surface, 63.5 on meta and 90.8 on
individual, and the series ran out of a 16 GiB budget at 100 to 200
credentials.

The step before this one froze the catalog and its schemas and shared them per
configuration: one catalog built with a credential-less client
(`gitlabclient.NewUnboundClient`), handlers rebuilt per client through
`ActionRoute.Bind`. That took the slope to 4.10, 5.44 and 11.28 MiB per
credential. Both sets of figures are the ones
[Resource hot spots](../resource-hot-spots.md) publishes, which carries the full
analysis, the host they were measured on and the commits they were measured at.

What was left was the `mcp.Server` itself, and it is irreducible while there is
one per credential: the SDK's tool table with an entry and a closure per
registered tool, the bound catalog descriptors, the meta dispatchers'
descriptions, the sessions and the client. The target is a per-credential cost
that no longer contains a registered surface, so that a thousand credentials
cost a thousand credentials rather than a thousand catalogs. That remainder
therefore has to go, which means one server serving many credentials.

Everything a server holds is decided by configuration, with one exception: the
GitLab client, which is decided by the credential. Two credentials whose
configurations agree can therefore be served by one server as long as every
request runs under its own client. That was already half true, because the
catalog's handlers are rebuilt per client through a recorded binder; it was not
true of the resources, the prompts, the completion handler or the interactive
flows, each of which captured a client at registration.

The part that had no answer at all was resource subscriptions, at the delivery
end. The go-sdk keeps its subscription table per server, keyed by URI and then
by session, and `Server.ResourceUpdated` is the only exported delivery: it
notifies every session subscribed to that URI, whoever they belong to. On a
shared server, credential A's watcher would notify B's session. Each change
would be delivered twice, A's watch state would arrive in B's `_meta`, and after
B's access was revoked, when its own watcher had already stopped on the 401
while the SDK's table still held its session for as long as its listen stayed
open, B would go on learning that the resource had changed from A's polling.
ADR-0015 makes the first read the authorization check precisely because every
session on one manager shares one token; a shared server keeps that at the
polling end and loses it at the delivery end.

## Decision drivers

- What a credential costs at rest should be flat in the number of them, which
  is what makes a shared deployment's memory a property of its configuration
  rather than of its popularity. What the requests in flight cost is a separate
  question and is not addressed here.
- No credential may observe another's data, its watch state, or the existence
  of its traffic.
- Nothing may become correct only by accident of ordering. A request that
  cannot be attributed to a credential must fail, not fall back to whichever
  credential happened to build the shape.
- No fork of the go-sdk, and no dependency on an unreleased change in it.

## Decision

**One `mcp.Server` per configuration shape, shared by every credential whose
configuration hashes to that shape, subscriptions included.**

### The shape

`serverShapeKey` names the configuration a server is built for: tool surface,
capability surface, meta parameter-schema mode, tier and whether it was pinned
or detected, GitLab.com or self-managed, read-only including read-only caused
by token-scope narrowing, safe mode, excluded tools, token scopes, and the
transport's statelessness. Those are exactly the inputs that decide which tools
exist and what shape they are listed in.

The instance URL is deliberately **not** in the key. Two instances of one tier
are served the same catalog, and what differs between them is the client, which
is per credential anyway. Including it would build a full server per instance
for no gain, and a deployment publishing several instances through
`--gitlab-url` is the one that most wants the sharing. Nothing about the
credential is in the key either: a key that depended on the token, the user or
their instance would be a key per credential, which is what this replaces.

`shapeServers` holds one built server per key and builds a shape at most once.
Registration runs on a goroutine behind that server's readiness gate exactly as
it did per credential, so the first caller still does not wait for it; the
difference is that every later credential of the same shape finds a catalog
already built, which used to be the entire cost of a pool entry (1.8 s on the
dynamic surface, 3.0 s on individual).

A registration that fails has failed for every credential of the shape, and
clearing that up has an ordering to respect, because the registration is started
by the shape registry while the pool's factory has not yet returned. The failure
path forgets the shape and then evicts every pool entry pointing at its server.
An entry already filed is found by that eviction; an entry filed afterwards
finds no shape in the registry when the pool's insert hook asks, and evicts
itself. There is no third ordering, since an eviction can only miss an entry by
running before the pool filed it, and running that early means the forget ran
earlier still. Without the second half a registration that failed quickly left a
poisoned entry cached, answering every later request for that credential from a
failed readiness gate until an idle timeout or a revalidation happened to drop
it.

### The credential travels with the request

A shape server is registered with the credential-less client for its instance
class, and every handler resolves the caller's client from the request context,
with that unbound client as the fallback that refuses. The seam is
`gitlab.WithClient` and `(*gitlab.Client).For(ctx)`, read by
`toolutil.WrapAction` and its three siblings (which covers every catalog
action on every surface), by the 38 resource closures, the 37 prompt closures,
the completion handler and the interactive elicitation flows.

The channel from the HTTP layer to the handler is the per-POST carrier that
already exists for cancellation (`cmd/server/carrier.go`), for the reason
recorded there: the header is the only per-request value the SDK exposes, and a
context value would be right in stateless mode and wrong in stateful mode,
where the session is connected with the initialize POST's context. The gate
resolves the pool entry, puts its `credentialState` on the HTTP request
context, and the `bindCredential` middleware reads it back and installs it on
the handler context.

`bindCredential` is added **after** the telemetry, rate-limit, listen-ceiling
and subscription middlewares, which makes it run **before** them: each of those
asks which credential a request belongs to, and a binding applied inside them
would leave them answering for the wrong tenant or for none. What they read
from it is the per-credential rate-limit bucket, the per-credential
listen-stream ceiling and the per-credential watchers.

### Watchers stay per credential; delivery is filtered by owner

The subscription machinery splits in two. A `subscriptionShape` per shape holds
what the configuration decides: the polling options, the listen-stream registry
and the handler index that registration publishes. A `subscriptionRuntime` per
pool entry holds the watchers, because a watcher polls GitLab with a credential
and ADR-0015 makes its first read the authorization check. Two credentials
watching one URI therefore have one watcher each, and each polls with its own
token.

Delivery is where the SDK offers nothing per session, so it is filtered
instead. Every pool entry carries an opaque owner token, minted from
`crypto/rand.Text` when the entry is built and never derived from the
credential, the user or the instance. `serverNotifier` stamps it into the
notification's `_meta` under one private key, and a sending middleware on the
shape server acts on `notifications/resources/updated` alone: it resolves the
receiving session's owner and forwards the notification with the private key
removed from its `_meta`, or drops it when the owner differs, when it carries no
owner tag, or when the session has no recorded owner. Every other method passes
through untouched.

The middleware fails closed on all three counts. Every notification this server
emits is tagged and every session that could hold a subscription was recorded
when its subscribe arrived, so none of the three absences can happen in a
correct wiring; treating any of them as "deliver anyway" would turn one wiring
mistake into cross-tenant delivery.

It reads the params rather than the request, and it puts the key back after the
send. Both are forced by how the SDK delivers. The two paths instantiate the
request generic differently: the legacy one passes concrete params and produces
a `*mcp.ServerRequest[*mcp.ResourceUpdatedNotificationParams]`, while the
2026-07-28 one passes a `func() Params` and produces a
`*mcp.ServerRequest[mcp.Params]`. A middleware that asserts on the request type
therefore matches one path and drops the other in silence, which is what the
first implementation of this did: every notification was sent, none was
delivered, and the server logged that it had sent them. The params interface is
the same on both paths and the concrete params behind it are always the same
type, so that is what the decision reads. And because
`notifySubscribedSessions` shallow-copies the params per session while the
legacy path hands one value to every subscriber in turn, leaving the key
stripped would make every session after the first look untagged; restoring it
after the send is safe because delivery within one `ResourceUpdated` call is
sequential, and because each call builds its own params and its own `_meta`. The
map itself is never mutated: the stripped `_meta` is a new map, which matters
because the per-session copies share one and the SDK stamps its own subscription
id into it.

### Session ownership is recorded, not derived

`mcp.ServerOptions.GetSessionID` takes no request, so a shared server mints
session IDs under one tag for every credential it serves and a tag stops saying
anything. The fact is recorded where it is known instead: an MCP request arrives
already bound to its credential, and `sessionOwners` writes the session, its ID
and the owner down together. It forgets a session when `ServerSession.Wait`
returns, and every session of an entry when the pool evicts it.

Only the sessions that will be asked about are recorded, which is the two cases
that read the map: one with an ID, because the gate checks every later request
against it, and one that subscribes by either method, because that is what makes
a session able to receive a notification the filter has to attribute. On the
default stateless transport every other POST gets a session of its own that
closes with the response, and recording those would cost a map entry and a
goroutine parked on `Wait` per request for a fact nothing reads.

That map serves both readers. The gate refuses a stateful GET, DELETE or POST
presenting a session ID whose recorded owner is not the entry the request
authenticated as, which is the same check as before on a recorded fact instead
of an inferred one. The sending middleware reads it to decide who a
notification is for.

Evicting an entry also ends what that credential owned, on a goroutine of its
own because `Manager.Close` waits for the watchers and the pool's callbacks run
under its write lock. While each credential had a server of its own, eviction
dropped only the pool's reference and a live session kept working; on a shared
server it cannot, because the same eviction forgets which credential that
session belongs to and every later notification for it would be filtered away in
silence.

Ending it has two halves, and the first implementation had only one. Stopping
the watchers is silent by contract: `Manager.Close` is the one stop path that
fires no `OnStop`, since the endings it announces are the ones a subscriber did
not ask for and a closed manager used to mean the whole server going away. So
nothing reached `listenStreams.stoppedFor` and the client's open
`subscriptions/listen` was left neither closed nor completed, which is the
outcome this decision calls the worse one. The eviction therefore closes that
credential's own streams as well, by owner, which is what makes the SDK write
the completion result the specification asks for; the next request
re-initializes.

The other half is not evicting in the first place. `lastUsed` is refreshed by
pool hits, and a credential whose only activity is a subscription produces none:
its watcher polls GitLab directly and its listen is one request the client holds
open rather than repeats. `serverpool.WithInUse` is what the pool asks before
dropping such an entry, and **both** eviction paths ask it. The idle sweep skips
a busy entry outright and moves it to the front of the LRU, so the two clocks
record the same decision; size pressure prefers an entry that is not busy and
takes a busy one only when every entry is busy.

Consulting it on the idle path alone was not a protection, because the two
clocks then disagreed in exactly the attacker's favour. A quiet subscriber sits
at the LRU tail precisely because its work does not pass through the pool, and
`evictLRU` took the tail, so any caller could evict every quiet subscriber in
the pool by presenting `--max-http-clients` credentials of its own, repeatably.
An hour's protection from the sweep is worth little against a second's from a
stranger.

The preference is bounded rather than absolute: an entry is passed over in
favour of another one, never in favour of the pool growing past
`--max-http-clients`, so a pool in which everything is busy still evicts its
oldest, and a credential GitLab has refused is evicted whatever `WithInUse`
says. The teardown above is what tells the client in every one of those cases.

### The invariant

Two credentials listening to the same resource URI each receive exactly their
own watcher's notifications, with their own watch state in `_meta`. A
credential whose access has been revoked receives nothing: its own watcher
stops on the 401 or 404, and the other credentials' notifications are filtered
away from its sessions.

## What in the go-sdk this rests on

Verified against v1.7.0 (`$(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.7.0/mcp/`),
which is the latest tag, and unchanged on the SDK's main branch at the time of
writing.

- `Server.resourceSubscriptions` is keyed by resource URI, then by session,
  then by the listen request id, and `Server.ResourceUpdated` (`server.go`)
  notifies only the sessions subscribed to that exact URI. The only
  cross-credential exposure on a shared server is therefore two credentials
  listening to the same URI.
- Both delivery paths build the notification per session and run it through the
  server's **sending middleware** chain: the legacy `notifySessions`
  (`shared.go`) and the 2026-07-28 `notifySubscribedSessions` (`server.go`)
  each call `newRequest(sess, params)` and then `handleNotify`, which calls
  `req.GetSession().sendingMethodHandler()`, which is the handler
  `Server.AddSendingMiddleware` wraps. The concrete request type is
  `*mcp.ServerRequest[P]` with exported `Session` and `Params` fields, and
  `defaultSendingMethodHandler` reads `req.GetParams()` at send time. A sending
  middleware can therefore inspect the session and the params, decline to send
  by returning without calling the next handler, and substitute the params.
- The context that middleware receives is a fresh `context.Background()` with a
  ten second timeout, created inside those two functions. Nothing the caller of
  `ResourceUpdated` puts on its own context reaches it, which is why the owner
  travels in the params.
- `notifySubscribedSessions` shallow-copies the params per session (`p :=
  *params`) and `injectMetaSubscriptionID` writes into the `Meta` map those
  copies share, so a middleware must never mutate that map.
- `ServerOptions.GetSessionID` takes no request, so a shared server cannot mint
  per-credential session ids.

## Consequences

### Positive

- POS-001: What a credential costs the process stops being a registered tool
  surface. Measured as the live heap of an idle process at 1 and at 20
  credentials, it fell from 434 to 17 KiB per credential on the dynamic surface,
  815 to 73 on meta, and 1,487 to 8 on individual. The residue is the pool
  entry's own state: a GitLab client, a rate-limit bucket, a counter, a watcher
  set, and the sessions the client holds open. What remains of the resident-set
  slope under load is the requests in flight, not the tenancy.
- POS-002: A credential's first request no longer pays for a catalog build
  whenever another credential of the same shape has already paid for it. That
  cost was 1.8 s on the dynamic surface and 3.0 s on individual, and it was
  paid again on every eviction.
- POS-003: The write check, the read check and the identity are all per
  request already, so nothing about ADR-0018 changes: a `read_api` credential
  simply hashes to a different shape than an `api` one and is served a
  different server.
- POS-004: Session ownership is now a recorded fact rather than an inference
  from the shape of an opaque string, and it survives a change in how session
  ids are minted.

### Negative

- NEG-001: A request that cannot be attributed to a credential now fails where
  it used to work. On a shape server the fallback client refuses every request
  and a subscription that resolves to no credential is answered with an
  internal error. That is deliberate, and it is the reason the binding
  middleware runs outermost, but it does mean a future middleware inserted
  above it would silently break every call.
- NEG-002: Notification delivery costs one map lookup per session per
  notification, and a params and `_meta` clone for the sessions that pass. That
  is small next to the poll it reports, and it happens only for
  resource-updated notifications.
- NEG-003: Evicting a pool entry now ends its subscriptions rather than letting
  them run until the session closes, and a client that is evicted is told to
  re-subscribe. The alternative was worse: going quiet without saying so.

  Two things follow from that, and both are load-bearing. A credential whose
  only activity is a subscription no longer looks idle: its watcher polls
  GitLab directly and its listen is one request the client never repeats, so
  `lastUsed` never moves and `--pool-idle-timeout` would have evicted a client
  that was being served correctly. `serverpool.WithInUse` is what the pool asks
  before evicting, on both paths: the idle sweep skips such an entry and moves
  it to the front of the LRU, and size pressure prefers an entry that is not
  busy, falling back to the oldest only when every entry is busy. A revoked
  credential is evicted regardless. And ending the
  watchers is not by itself ending the subscription: `Manager.Close` is the one
  stop path that fires no `OnStop`, so nothing reaches `listenStreams.stoppedFor`
  and the client's stream would stay open and silent. The eviction closes that
  credential's streams itself, which is what produces the completion result.
- NEG-004: The shape registry is a second cache beside the pool, with its own
  lifetime. A shape is dropped when its registration fails and otherwise lives
  for the process, which is bounded by the number of distinct configurations a
  deployment can produce rather than by `--max-http-clients`. That bound is
  small in practice (tier, scope narrowing and instance class), but it is not
  the pool's bound.
- NEG-005: The owner token is process-local and is minted per entry, so a
  rebuilt entry for the same credential is a different owner. A client whose
  entry is rebuilt mid-session is refused on its session id and re-initializes.
  That was already true when the session tag was per server.

### Neutral

- NEU-001: On stdio nothing changes. There is one credential, no binding is
  installed, `For` returns the captured client, and the notification filter is
  not installed at all, because a filter there would drop every notification
  for want of a tag nothing needs to write.
- NEU-002: `ServerPool.GetOrCreate` still returns a `*mcp.Server` and is still
  what the pool's own tests drive. The entry-shaped `GetOrCreateEntry` was
  added beside it rather than replacing it, because a server pointer is no
  longer an answer to "which credential is this" and every caller that needs
  that answer now asks for the entry.

## Alternatives considered

- **Route `subscriptions/listen` to a per-credential subscription server,
  stateless only.** This was the way recorded as most likely when the blocker
  was first written down, and it is rejected here. It needs a second shell per
  credential (templates and a manager, no tools), and it needs the HTTP gate to
  parse the JSON-RPC body itself to route by method, because the SDK's
  `Mcp-Method` header check inspects a single message and a batch skips it. It
  also leaves `--stateless=false` with a full server per credential, since a
  session lives on one server and a `resources/subscribe` arrives on it later.
  So it buys a flat heap on the default transport only, at the cost of a body
  parser in the authentication path.
- **Per-session delivery in the go-sdk.** `ServerSession.ResourceUpdated(ctx,
  params)`, reading the session's own request id from the table, would make the
  filter unnecessary. It is the right long-term shape and is recorded as a
  deferred upstream ask in
  [upstream-bugs.md](../upstream-bugs.md); nothing here can use it until it is
  released, and the filter is what makes the sharing possible today. Propagating
  the caller's context to the sending middleware would be an equivalent fix,
  since the owner could then travel on the context instead of in the params.
- **Accept cross-notification within a shape.** Nothing to build, and it is the
  one option that trades a security property for memory: duplicates, another
  credential's watch state in `_meta`, and delivery after revocation. Rejected.
- **Keep one server per credential and shrink it further.** Recovers part of the
  remainder (the SDK tool table and closures, 1.1 MB on individual; the bound
  descriptors and route maps, about 3 MB on dynamic) and leaves the slope a
  slope. Rejected against the stated target.

## Related

- [ADR-0014](adr-0014-catalog-first-runtime-architecture.md) is what makes a
  shape definable at all: every surface is projected from one catalog, so the
  inputs that decide what a server registers are enumerable.
- [ADR-0015](adr-0015-polled-resource-subscriptions.md) fixes the authorization
  model this preserves: the first read is the check, which is why watchers stay
  per credential.
- [ADR-0018](adr-0018-authorization-admits-per-action-gating.md) makes the
  served surface a property of the credential, which is what puts the token
  scopes and the read-only narrowing into the shape key.
- [Resource hot spots](../resource-hot-spots.md) carries the measurements
  before and after.
