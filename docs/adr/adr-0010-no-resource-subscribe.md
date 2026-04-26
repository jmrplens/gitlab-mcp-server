# ADR-0010: No Resource Subscribe Capability

## Status

Accepted — 2026-04-26

## Context

The MCP `resources/subscribe` capability lets clients ask the server to push
`notifications/resources/updated` events when a resource changes. To honor a
subscription, the server must observe the upstream system in real time and
deliver notifications back to the connected client without polling.

Per the [MCP specification][spec-resources], `resources.subscribe` is
**OPTIONAL** — servers advertise it explicitly in `ServerCapabilities`. We
considered enabling it for resources backed by GitLab (projects, issues,
merge requests, pipelines, …) so AI assistants could react to state changes
without re-listing.

## Decision

`gitlab-mcp-server` **does not** advertise `resources.subscribe`. The
`Subscribe` field of every `ResourceCapabilities` block is left at its zero
value (`false`), and we reject `resources/subscribe` requests via the SDK
default behavior.

## Rationale

1. **No GitLab push channel from inside the binary.** GitLab notifies
   external systems via webhooks, which require a publicly reachable HTTPS
   endpoint registered per project. The MCP server is typically:
   - A local stdio binary launched by the IDE — has no listening socket.
   - A shared HTTP MCP endpoint behind authentication — registering it as a
     webhook target on every project the user can access is impractical and
     leaks the MCP token surface.

   Even if a webhook receiver were added, fan-out from one webhook to N
   subscribed MCP sessions belongs in a separate service, not in the tool
   server.

2. **Polling is the alternative, and it is wrong here.** Implementing
   `subscribe` by polling `/projects/:id`, `/merge_requests/:iid`,
   `/pipelines/:id`, etc. for every active subscription would:
   - Multiply API traffic against GitLab and accelerate rate-limit hits.
   - Burn quota that should fund explicit user-driven tool calls.
   - Produce inconsistent latency (poll interval) that does not match the
     "push when changed" semantics clients expect from the capability.

3. **No demonstrated demand.** The current design supports re-querying via
   tools (`gitlab_get_merge_request`, `gitlab_get_pipeline`, …) which the
   LLM can invoke whenever it needs fresh state. No real-world client has
   asked for resource subscriptions, and most MCP clients do not consume
   them today.

4. **Spec compliance is preserved.** Declaring `subscribe: false` (the
   default) is the sanctioned way to opt out. Clients see the absence of the
   capability during initialization and do not attempt to subscribe.

## Consequences

### Positive

- POS-001: No background polling against GitLab from the MCP server.
- POS-002: No webhook receiver to deploy, secure, or rotate secrets for.
- POS-003: Predictable resource consumption — every API call corresponds to
  an explicit tool invocation.
- POS-004: Fully spec-compliant: `subscribe` is OPTIONAL and we honor the
  negotiation contract by advertising `false`.

### Negative

- NEG-001: Clients that want push semantics must re-query via tools. The LLM
  has to decide when state is "stale" rather than receive an event.
- NEG-002: Real-time dashboards built on top of the MCP server have to
  implement their own change detection (e.g., diff between two list calls).

### Neutral

- NEU-001: Future revisit is cheap. If GitLab ships a streaming events API
  reachable from the MCP process (or if a hosted webhook fan-out service
  becomes part of the deployment), we can opt into `subscribe` without
  breaking existing clients.

## Alternatives Considered

- **A. Polling-based subscribe.** Rejected: amplifies rate-limit pressure
  and produces poor latency; better delivered as a separate service if ever
  needed.
- **B. Webhook receiver inside the MCP binary.** Rejected for stdio mode
  (no socket) and impractical for HTTP mode (per-project registration,
  shared-token leakage, and N-to-1 fan-out concerns).
- **C. Sidecar webhook fan-out service.** Rejected as out of scope: would
  introduce its own deployment, auth, and storage concerns that do not
  belong in a stateless tool server.

## References

- [MCP Specification — Resources][spec-resources]
- ADR-0008: Universal Identity (rationale for keeping the server stateless)

[spec-resources]: https://modelcontextprotocol.io/specification/2025-11-25/server/resources
