# Enterprise deployments

> **Diátaxis type**: How-to
> **Audience**: 🔧 Operators running one deployment for a large number of people

[Remote Deployment](remote-deployment.md) stands the server up for other people:
a service unit, a container, a reverse proxy, a certificate, and a first pass at
several instances. This page is what changes when "other people" is hundreds of
them: how to size an instance from measurements rather than guesses, how to
balance across instances that come and go, what an MCP gateway in front of the
server does to the catalog and to the credential, how to terminate and rotate
TLS without a restart, and what to watch once it is running.

Everything here is HTTP mode. Read [HTTP Server Mode](http-server-mode.md) first
if the pool, the instance allow-list and the authentication modes are not
already familiar.

## Two quantities, not one

Sizing this server used to be one question, because holding a credential and
serving a call cost the same kind of thing. It is now two, and they have
different answers.

**Holding a credential is nearly free.** Up to version 2.8.0 an HTTP deployment
built one MCP server per credential, so the resident set was a straight line in
the number of pooled credentials and the slope was one registered tool catalog.
The catalog, its schemas, the discovery index and the tool manifest are now
shared per configuration, and the MCP server itself is shared per configuration
**shape**, so what a credential costs is its GitLab client, its rate-limit
bucket, its listen counter, its watchers and its sessions.
[ADR-0020](../development/adr/adr-0020-one-server-per-configuration-shape.md)
records the design and its invariants.

**Serving a call costs exactly what it did.** The sharing work did not touch the
per-call path, and per-call processor time is where an instance actually runs
out. Everything in the sizing section below follows from keeping those two
apart.

## Sizing an instance

### What a credential costs at rest

Measured at rest with no connection open, a pooled credential costs **7.7 KiB on
the dynamic surface, 8.3 KiB on meta and 8.5 KiB on individual**. It is the same
figure three times over, because what a credential holds is its GitLab client
and its bookkeeping rather than its tools. Those figures come from the
[resource record](../reference/resource-benchmark.md).

The shipped regression probe takes the same reading with the test harness's
connections kept alive: run from this tree on an AMD Ryzen 5 3550H (8 threads,
60.8 GiB, Go 1.27.1), the live heap grows 0.14 MiB between one credential and
twenty on the dynamic surface and 0.16 MiB on individual, which is what the
record reports for it.

Three things bound that measurement, and all three matter when you apply it:

- It is the **live heap** after a collection, not the resident set. The resident
  set of the process is larger and does not shrink promptly, because Go returns
  memory to the operating system lazily.
- It is **with no connection open**. A credential holding one also costs the
  buffers behind it, which the resource benchmark measures separately and
  reports as a larger per-credential slope.
- It is **at rest**. A credential with requests in flight costs what those
  requests allocate, which is the term that actually dominates. Any
  per-credential figure measured under load is a mixture of tenancy and load and
  cannot be multiplied by a credential count.

### What a configuration shape costs, once

A **shape** is everything that decides what a server registers: tool surface,
capability surface, meta parameter-schema mode, tier and whether it was pinned,
GitLab.com or self-managed, read-only including the narrowing a `read_api` token
causes, safe mode, excluded tools, token scopes, and whether the transport is
stateless. Every credential hashing to one shape is served by one MCP server.

A process holding one registered shape and one credential has a live heap of
about **38 MiB on the dynamic surface and 88 MiB on individual**, from the same
run on the same AMD Ryzen 5 3550H, at rest. An HTTP process with no credential
at all, and therefore no shape registered, holds about 35 MiB resident, from the
[resource record](../reference/resource-benchmark.md)'s reference host, an Intel
i5-14400 with 16 threads.

How many shapes a deployment produces is small and bounded, and you control most
of it:

| Input               | Values in one deployment                                                               |
| ------------------- | -------------------------------------------------------------------------------------- |
| Tier                | 1 with `--tier` pinned; up to 3 (free, premium, ultimate) when detected per credential |
| Token scopes        | 2, because only `admin_mode` changes the catalog                                       |
| Scope detection     | 2, or 1 with `--ignore-scopes`                                                         |
| Read-only narrowing | 2 when the operator did not set `--read-only`, 1 when they did                         |
| GitLab.com or not   | 1, or 2 for a deployment publishing both                                               |
| Everything else     | 1 each; they are process settings                                                      |

A single-instance deployment with `--tier` pinned and `--ignore-scopes` produces
exactly one shape. A typical one produces two to eight. The worst case is under
fifty, and each costs one catalog build (measured at 1.8 seconds on the dynamic
surface and 3.0 on individual) paid once for the process rather than once per
credential.

This is the direct consequence worth naming: **pinning `--tier` is a sizing
decision, not only a correctness one.** It removes the tier from the shape key
and collapses up to three catalogs into one.

### What a call costs

Per call, on the reference host (Intel i5-14400, 16 threads), under the
concurrency series' load of four requests in flight per credential on the
dynamic and meta surfaces and two on individual:

| Surface      | Processor time per call | What dominates it                                  |
| ------------ | ----------------------- | -------------------------------------------------- |
| `dynamic`    | about 8 ms              | The SDK's result marshalling and schema validation |
| `meta`       | about 8 ms              | The same                                           |
| `individual` | about 130 ms            | Marshalling a 3 MB `tools/list` response           |

The arithmetic that follows is the only one that matters for capacity:

```text
calls per second ceiling  =  usable threads  /  processor seconds per call
```

On the reference host, `16 / 0.008` is 2,000 calls a second in theory. The
measured series plateaus at about **1,650 calls a second** on the dynamic
surface, reached by five credentials each keeping four requests in flight, and
never exceeded no matter how many more credentials are added; that plateau is
thirteen of the host's sixteen threads.
The individual surface plateaus at about **106 calls a second** on the same
host, because one `tools/list` there costs sixteen times what a whole dynamic
call does.

Use it to size, not as a promise. Your instance's GitLab is on the other side of
a network this measurement did not cross, and a real call waits on it.

### The worked example

Five hundred developers, an editor each, on the default dynamic surface, against
one Ultimate self-managed instance, with `--tier=ultimate` pinned.

**Memory.** One shape, so one catalog: about 38 MiB of live heap. Five hundred
credentials at 7.7 KiB is under 4 MiB. Tenancy is therefore not the term that
sizes this deployment, and the peak resident set is decided by how many calls
are in flight at once. Size for the concurrency, then add a comfortable margin
for Go's lazy return of memory to the operating system. A 512 MiB floor is the
right starting point, and the first thing to measure is the peak resident set
under your own traffic rather than the credential count.

**Processor.** Five hundred editors do not make five hundred concurrent calls.
Take the number actually in flight: if the busy hour sees fifty calls a second,
that is `50 × 0.008 = 0.4` of a thread on the reference host, and the deployment
is bound by nothing. If it sees a thousand, that is eight threads and one
instance of the reference host's size is at about two thirds of its ceiling; add
the second instance for headroom rather than for memory.

**Reach for a second instance for availability first.** A single instance holds
a large population cheaply now. The ceiling that usually binds a busy deployment
is GitLab's own rate limit against one token, which no number of instances
raises.

### The knobs, and what to set them to

| Setting                                   | Default     | What it bounds                                                           | At scale                                                                                                                                                                                                                           |
| ----------------------------------------- | ----------- | ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--max-http-clients`                      | `100`       | Pooled credentials, evicted least-recently-used first; upper bound 10000 | Set it above your expected simultaneous population. Eviction is no longer cheap: it ends that credential's subscriptions and closes its listen streams                                                                             |
| `--pool-idle-timeout`                     | `1h`        | How long an unused entry is kept                                         | Leave it. The sweep runs every quarter of the timeout with a one-minute floor, so an entry can outlive the timeout by that much. An entry with live watchers or open listen streams is never idle-evicted                          |
| `--revalidate-interval`                   | `15m`       | How often pooled credentials are re-checked against GitLab               | The sweep is serial with a ten second timeout per entry, so a large pool against a slow instance costs up to `entries × 10s` per round. Raise it, or accept the lag; an entry older than an hour is rebuilt on next use regardless |
| `--rate-limit-rps` / `--rate-limit-burst` | `10` / `40` | Calls a second per credential that reach GitLab                          | Per credential **per process**. Several instances multiply it unless affinity pins each caller to one. See [The limiter multiplies](#the-limiter-multiplies)                                                                       |
| `--action-timeout`                        | `65m`       | How long one action may run                                              | Above the longest wait any action offers. Lower it only if you would rather fail a pipeline wait than hold a goroutine                                                                                                             |
| `GITLAB_MCP_MAX_LISTEN_STREAMS`           | `64`        | Open `subscriptions/listen` streams per credential                       | A second ceiling of 512 per process is deliberately not configurable, because the per-credential one multiplies by however many tokens one caller holds                                                                            |
| `--session-timeout`                       | `30m`       | Idle MCP session lifetime                                                | Only under `--stateless=false`. Under the default transport a session ends with its POST                                                                                                                                           |
| `--http-idle-timeout`                     | `0`         | Idle connection closure                                                  | Leave it disabled. A positive value cuts long-lived streams                                                                                                                                                                        |
| `--drain-delay`                           | `0`         | How long `/health` answers `503 draining` before the listener closes     | Set it to at least one probe interval. See [Health-driven ejection](#health-driven-ejection-and-the-drain-window)                                                                                                                  |

Two further limits are not configurable and are worth knowing: ten resource
watchers per credential, and ten failed authentications a minute per client
address before that address is answered `429` for a minute. The second one has a
trap behind a proxy, described in [Operations at scale](#what-to-monitor).

### What does not scale with the credential count

- **The registered catalog**, now paid once per shape.
- **The catalog build**, likewise. A credential arriving at a shape another
  credential already built waits for nothing.
- **Goroutines**, which are per watcher, per open listen stream and per live
  session rather than per credential. A credential doing nothing has none.

## Load balancing beyond one worked example

The [remote deployment guide](remote-deployment.md#several-instances-behind-one-proxy)
explains the three distributions (round robin, address hash, token hash) and
carries a worked nginx balancer. This section is what a deployment with many
users needs on top of it.

### What affinity is still worth

Affinity used to be close to mandatory, because a caller that moved paid a whole
catalog build on the instance it moved to. It no longer does: the instance it
lands on has the shape built already, and building a pool entry there is a
credential probe, a licensing lookup and a client. What affinity still buys:

- **One rate-limit bucket per caller instead of one per instance.**
- **One licensing and identity probe per caller instead of one per instance.**
- **A pool entry that stays warm**, rather than being rebuilt on each instance
  in turn and evicted from each in turn.

And two things that are not preferences. A stateful session (`--stateless=false`)
lives in the process that minted its `Mcp-Session-Id`, and another instance
answers that id with `404`. Resource subscriptions are watchers held by one
process. If you run either, affinity is a requirement.

### Consistent hashing across a changing instance set

Use consistent hashing, not a plain modulo. Removing one of three instances then
relocates roughly a third of the callers instead of reshuffling all of them,
which is the difference between a rolling update that warms one cold pool and
one that warms every pool. Both configurations below say `consistent`.

Hash a **salted digest of the credential**, never the credential. An affinity key
ends up in the balancer's memory, in its access log if the log format names the
variable, and in whatever upstream selection it drives; a raw bearer token in any
of those is a credential leak with extra steps. The salt is what does the
security work: without it the key is a token fingerprint anyone holding a
candidate token could confirm.

Three details decide whether the affinity holds at all:

- **Normalize before hashing.** Strip the `Bearer` scheme prefix and surrounding
  whitespace. The same token spelled two ways hashes two ways.
- **Fall back to the address, not to nothing.** An empty key makes every
  credential-less request hash identically onto one instance.
- **Prove it rather than assume it.** A hash key the balancer cannot resolve
  makes it fall back to round robin silently, which looks exactly like a working
  deployment until someone counts.

### nginx

nginx hashes a string containing variables itself, so the salt goes into the key
expression without ever becoming a loggable variable of its own. This needs no
third-party module.

```nginx
# Keep the salt out of the repository: include it from a 0600 file.
map $host $mcp_salt {
    default "a-long-random-per-deployment-salt";
}

# The bearer credential, without its scheme prefix.
map $http_authorization $mcp_bearer {
    default                     "";
    "~*^Bearer[ ]+(?<tok>\S+)$" $tok;
}

# Legacy-mode callers send PRIVATE-TOKEN instead.
map $mcp_bearer $mcp_credential {
    default $mcp_bearer;
    ""      $http_private_token;
}

# No credential: fall back to the client address.
map $mcp_credential $mcp_affinity {
    default $mcp_credential;
    ""      $remote_addr;
}

upstream gitlab_mcp {
    hash "$mcp_salt$mcp_affinity" consistent;
    server 10.0.0.11:8080 max_fails=2 fail_timeout=10s;
    server 10.0.0.12:8080 max_fails=2 fail_timeout=10s;
    server 10.0.0.13:8080 max_fails=2 fail_timeout=10s;
    keepalive 32;
}

server {
    listen 443 ssl;
    server_name mcp.example.com;

    location / {
        proxy_pass http://gitlab_mcp;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 1h;

        # Retry only what was never delivered.
        proxy_next_upstream error timeout;
        proxy_next_upstream_tries 2;
    }

    location /.well-known/oauth-protected-resource {
        proxy_pass http://gitlab_mcp;
        proxy_http_version 1.1;
    }
}
```

Do not log `$mcp_affinity` or `$mcp_credential`.

**nginx open source has no active health check.** `max_fails` and `fail_timeout`
are passive: an instance is taken out only after real requests to it fail, which
means the requests that discover the failure are the ones that failed. A drain
window buys nginx nothing, because nothing polls `/health`. If that matters to
you, use a balancer that polls, or drive the upstream from outside by removing
the instance and reloading before you signal it.

### HAProxy

HAProxy computes the digest in its own configuration, so nothing has to be
trusted to keep the raw credential out of a routing key, and it polls `/health`,
which is what makes the drain window mean anything.

```haproxy
global
  daemon
defaults
  mode http
  timeout connect 2s
  timeout client 1h
  timeout server 1h
  retries 1
  option http-server-close

frontend mcp
  bind *:443 ssl crt /etc/ssl/private/mcp.example.com.pem
  # The credential, from either header, with the Bearer prefix removed.
  http-request set-var(txn.cred) req.hdr(PRIVATE-TOKEN)
  http-request set-var(txn.cred) req.hdr(Authorization),regsub(^[Bb]earer\ +,) if !{ req.hdr(PRIVATE-TOKEN) -m found }
  # No credential: the client address, so anonymous requests spread.
  http-request set-var(txn.cred) src,ipmask(32,128) if !{ var(txn.cred) -m found }
  default_backend mcp_servers

backend mcp_servers
  # Route on a salted digest, never on the credential itself.
  balance hash var(txn.cred),concat(a-long-random-per-deployment-salt),sha1,hex
  hash-type consistent
  option httpchk
  http-check send meth GET uri /health
  http-check expect status 200
  server one 10.0.0.11:8080 check inter 2s fall 2 rise 2
  server two 10.0.0.12:8080 check inter 2s fall 2 rise 2
  server three 10.0.0.13:8080 check inter 2s fall 2 rise 2
```

`option httpchk` sends no `Host` header unless one is configured, and until
version 2.8.0 this server answered such a request with `403` when
`--http-addr` named a specific host, which marked every instance permanently
down. It is now served: a request naming no host is not the DNS-rebinding attack
that check exists for, since a browser always sends one. Adding
`hdr Host mcp.example.com` to the `http-check send` line is still good practice
and is required against an older server.

`retries 1` with no `option redispatch` is the deliberate half of this. HAProxy
retries a connection failure and never a request it already delivered, so a
`tools/call` that created an issue and then lost its response is not replayed on
a second instance.

### Health-driven ejection and the drain window

On `SIGTERM` the process marks itself draining before anything else. From that
moment `/health` answers `503` with `"status": "draining"` and
`Cache-Control: no-store`, **while the listener stays open and keeps serving
real requests**. By default it stays open for no time at all, so a balancer
usually discovers the shutdown from a failed request rather than from the flip.

`--drain-delay` is the window. Set it to at least one full detection interval of
your balancer: for the HAProxy configuration above, `inter 2s fall 2` detects in
four seconds, so `--drain-delay=10s` is comfortable. The sequence then is: the
signal arrives, `/health` flips to `503`, the balancer stops sending new work
while the instance still answers the work it has, the window elapses, the
listener closes, and in-flight requests get fifteen seconds to finish.

Upper bound five minutes. It applies to HTTP mode only, since stdio has no
listener to hold open.

### Retries that never replay a delivered request

An MCP `tools/call` can create an issue, merge a request or delete a branch. A
balancer that retries a delivered POST on a second instance turns one mutating
call into two, and neither the client nor the server can tell.

- **nginx**: `proxy_next_upstream error timeout` and nothing else. Never add
  `non_idempotent`, and never add `http_500`: both make nginx replay a request
  that was delivered and answered.
- **HAProxy**: leave `option redispatch` off and `retry-on` at its default of
  connection failures.
- **Anything else**: the rule is that a retry is safe only when the request
  provably never reached an instance. Nothing in the MCP protocol makes a
  delivered call idempotent for you.

### When a gateway hides the client

Behind a CDN, an API gateway or a service mesh, the connection the balancer sees
comes from the gateway and the credential may have been replaced. Two
consequences:

- **Affinity has to key on something the gateway preserves.** If the gateway
  terminates authentication and issues its own credential downstream, hash that
  one. If it forwards the original, hash the original. If it does neither, you
  have no key and round robin is the honest choice.
- **The rate limiter must be told the real address**, or it will charge every
  caller's failures to the gateway. Set `--trusted-proxy-header` to the header
  your gateway sets and `--trusted-proxies` to its addresses; the header is
  believed only on a connection from a listed address, and each flag is refused
  without the other. For `X-Forwarded-For` the value is read from the right,
  skipping hops that are themselves listed, so the first hop nobody vouches for
  is the client.

### The limiter multiplies

`--rate-limit-rps` is per pooled credential inside one process. Three instances
mean up to three buckets for one caller unless affinity pins it to one. With
token affinity the configured number is the number; under round robin, either
divide it by the instance count or put the real limit on the balancer.

The same multiplication applies to `--revalidate-interval`: the same token is
re-checked once per instance holding it.

## MCP gateways

An MCP gateway sits between clients and this server, validates the catalog it
serves, re-exposes it under names of its own, and may add authentication,
rate limiting and observability. Five things change.

### The catalog is validated before it is admitted

Gateways refuse catalogs under rules their operator chooses, and a refusal is
usually all-or-nothing: one bad description and none of the tools are admitted.
One production gateway rejected any tool whose description contained a
semicolon.

Everything this server lists (`tools/list` on any surface, `prompts/list`,
`resources/list`, `resources/templates/list`) is pure ASCII prose with no
semicolons, descriptions, titles and schema-embedded descriptions included.
`make check-gateway-chars` gates it in CI.

For the next rule, which is yours and not ours,
`GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS` rewrites listed text on the way out:

```bash
# Replace every semicolon with a period, for a gateway that refuses them.
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com \
  --description-substitutions=';=.'
```

It covers descriptions and titles across tools, prompts, resources and resource
templates, and never touches names, URIs, `pattern`, `const`, enum values or
tool-call payloads. A malformed value refuses startup rather than serving the
gateway the unrewritten catalog it already rejected. Verify a configuration
against the character audit before deploying it:

```bash
GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS=';=.' go run ./cmd/audit_gateway_chars/ -apply -check
```

See [Client Compatibility](client-compatibility.md#mcp-gateway-validators) for
the full rule set.

### The credential has to reach the server

This server holds no credential of its own: every request carries the caller's,
as `PRIVATE-TOKEN` or `Authorization: Bearer`. A gateway must therefore either
forward the caller's credential or hold one of its own and present it.

Forwarding the caller's is the only arrangement that preserves per-user identity,
per-user tiering and per-user rate limiting. A gateway holding one shared
credential collapses the whole population onto a single pool entry, a single
rate-limit bucket and a single GitLab identity: every action is attributed to
that one user in GitLab's audit log, and `--rate-limit-rps` becomes a limit for
the entire deployment rather than per caller.

If the deployment publishes several instances with repeated `--gitlab-url`, the
gateway must also forward `GITLAB-URL`, and a request without it is refused
rather than resolved to a default.

### Sessions and subscriptions may not survive

Run gateways against the default `--stateless=true`. Each POST is then
self-contained, no `Mcp-Session-Id` has to survive the gateway, and a gateway
that pools connections or fans out across instances cannot break a session it
does not know it is carrying.

Do not assume subscriptions pass through. A gateway re-advertises its own
capabilities to its clients, and it is free to advertise fewer than the servers
behind it. The worked configuration below negotiates
`resources.subscribe: true` with this server and then advertises
`subscribe: false` to its own clients, so no client behind it can subscribe at
all. Check what your gateway advertises before promising subscriptions to
anyone.

### What a gateway may cache

Every cacheable result carries SEP-2549 hints. Almost everything is `private`,
because catalogs and resource content are filtered by the caller's token scopes
and licensing tier and must never be served from a shared intermediary cache.
The prompt catalog is the one exception and is marked `public`.

A gateway that ignores `cacheScope` and caches a `tools/list` across callers
will serve one caller's tier-filtered catalog to another. That is a gateway
misconfiguration this server cannot prevent, and it is worth checking for
explicitly: pinning `--tier` and `--ignore-scopes` makes every caller's catalog
identical, which removes the hazard at the cost of the per-caller filtering.

### A worked gateway configuration

Verified against [mcp-context-forge](https://github.com/IBM/mcp-context-forge)
0.9.0. Register this server as an upstream gateway peer with the credential
travelling in a forwarded header:

```bash
curl -sS -X POST https://gateway.example.com/gateways \
  -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "gitlab_mcp",
        "url": "https://mcp.example.com/mcp",
        "transport": "STREAMABLEHTTP",
        "auth_type": "authheaders",
        "auth_headers": [{"key": "PRIVATE-TOKEN", "value": "glpat-..."}]
      }'
```

What that produced, exactly:

- The gateway reported `reachable: true`, negotiated `prompts`, `resources`
  (with `subscribe: true`), `tools` and `completions`, and fetched the catalog.
  On the default dynamic surface that is two tools.
- **The tool names change.** `gitlab_find_action` is re-exposed as
  `gitlab-mcp-gitlab-find-action`: underscores become dashes and the gateway's
  own name for the server is prefixed. Any prompt, skill or documentation that
  names a tool has to be written against the gateway's names, not this server's.
- **Descriptions pass through unchanged**, which is the ASCII property doing its
  job.
- **The gateway advertises `resources.subscribe: false` to its own clients**,
  having negotiated `true` with this server.
- Its own `/rpc` convenience endpoint re-validated a successful tool result and
  reported it as an error while carrying the real answer in
  `structuredContent`; its MCP endpoint at `/mcp/` did not. Prefer the MCP
  endpoint, and treat a gateway's non-MCP facade as a separate thing to test.

The credential shown above is one held by the gateway, which is the shape that
collapses every caller onto one GitLab identity. Use the gateway's per-user
credential pass-through if it has one.

### Routing by action

On the dynamic surface, the `action` property of `gitlab_execute_action` carries
the SEP-2243 `x-mcp-header` annotation, which the SDK turns into the wire header
`Mcp-Param-Action`. A gateway or a balancer can therefore route, rate-limit and
observe calls by canonical action ID without parsing the JSON-RPC body:

```nginx
# Send everything that mutates through a stricter limiter, by reading a header.
map $http_mcp_param_action $mcp_action_zone {
    default            "read";
    "~^issue\.create$" "write";
    "~^project\."      "write";
}
```

The header is advisory: it reflects what the client declared, so use it for
routing and observability, never as an authorization decision. The server's own
per-action gating is what decides whether a call is allowed.

## TLS

### Where to terminate

| Where the proxy is      | What to do                                                                                     |
| ----------------------- | ---------------------------------------------------------------------------------------------- |
| On the same machine     | `--http-addr=/run/gitlab-mcp/server.sock`. No network hop to encrypt, no certificate to rotate |
| On another machine      | `--tls-cert` and `--tls-key` on the listener, with the proxy verifying it                      |
| Terminating for clients | The proxy's own certificate, plus one of the two rows above for the hop behind it              |

The unix socket removes the hop rather than encrypting it, and under Docker that
hop is not "just loopback": the path runs through docker-proxy and a bridge
network. It has one cost at scale, described below.

### The listener's own TLS

```bash
gitlab-mcp-server --http --http-addr=:8443 \
  --gitlab-url=https://gitlab.example.com \
  --tls-cert=/etc/ssl/mcp.crt --tls-key=/etc/ssl/mcp.key
```

Both flags or neither: a certificate without its key is a deployment that
believes it is encrypting and is not, and startup refuses it. TLS 1.2 is the
floor and no maximum is set, so a current client negotiates TLS 1.3 and 1.2 is
reached only by a client that cannot go higher. Anything below 1.2 is refused.
Both properties are pinned by tests that drive the real binary.

### Certificate rotation without downtime

**Write the new pair over the old paths. There is nothing else to do.** The
certificate is served through a callback that checks the two files on each
handshake and re-reads them when either has changed, so the next handshake
presents the new certificate. Connections already open keep the old one until
they are replaced, which is correct and is how every rotation works.

Concretely, with certbot or any renewal tool that writes the same paths:

```bash
# The renewal hook needs no signal and no restart.
certbot renew --deploy-hook "true"
```

Three properties of that path, each covered by a test:

- **A half-written rotation keeps serving.** Writing a certificate and its key
  is two writes, and between them the pair on disk does not match. The
  previously loaded certificate stays in service until the pair is whole again,
  and the failure is logged once rather than once per handshake.
- **Unreadable files keep serving.** A renewal that unlinks before it writes, or
  a mount briefly absent, does not take the listener down.
- **Nothing is re-read while nothing changed.** The staleness check is two
  `stat` calls; the parse happens only when the size or modification time moved.

There is no reload signal and none is needed. `SIGHUP` is not handled and, under
its default disposition, terminates the process; do not send it.

The first load is still strict: at startup a path that does not exist or a key
that does not match its certificate is a named startup error, because that is
the one moment there is nothing to fall back to and an operator is watching.

### Client certificates

**The listener does not do mTLS.** It requests no client certificate and
verifies none; authentication on it is the bearer credential, in either
supported mode. Mutual TLS at the edge is therefore a property of the proxy in
front, which is where it belongs for a deployment with many clients anyway,
since the proxy is what holds the client CA and the revocation list:

```nginx
server {
    listen 443 ssl;
    server_name mcp.example.com;

    ssl_client_certificate /etc/ssl/clients-ca.pem;
    ssl_verify_client on;
    ssl_verify_depth 2;

    location / {
        proxy_pass http://gitlab_mcp;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        # Pass the verified identity on for logging, never for authorization:
        # the server authorizes on the GitLab credential.
        proxy_set_header X-Client-DN $ssl_client_s_dn;
    }
}
```

For the proxy-to-server hop, `proxy_ssl_verify on` with `proxy_ssl_trusted_certificate`
pointed at your private CA is the pairing for `--tls-cert` on the listener.

### The unix socket, and what it costs at scale

A unix socket has no peer address. The authentication failure budget is keyed on
the caller's address, so on a socket **every caller shares one budget**: ten
failed authentications a minute from anywhere behind the proxy answer `429` to
everybody for a minute. `--trusted-proxy-header` cannot repair it either, since
the header is believed only from a peer that parses as an address, and no socket
peer does.

The alternative is one line: bind loopback TCP instead and tell the server who
the proxy is.

```bash
gitlab-mcp-server --http --http-addr=127.0.0.1:8080 \
  --gitlab-url=https://gitlab.example.com \
  --trusted-proxy-header=X-Real-IP --trusted-proxies=127.0.0.1,::1
```

Keep the socket where the caller population is small or entirely trusted, and
where removing the hop matters more than per-caller failure accounting.

## Operations at scale

### Rolling updates

1. Start every instance with `--drain-delay` at or above one balancer detection
   interval.
2. Roll one instance at a time. Under consistent hashing the callers pinned to
   it move to a neighbour, build a pool entry there, and come back when it
   returns.
3. Compare `config_digest` across the fleet after the roll, before declaring it
   done.

A build from a different version with the same settings reports the same
`config_digest`, so an upgrade does not trip the comparison. That is deliberate:
the digest answers "do these instances serve the same catalog", not "are these
instances the same build". `build` answers the second.

### `/health` and `config_digest` across a fleet

`GET /health` needs no credential and performs no GitLab round trip. It answers
`200` while serving and `503` once shutdown was requested, with `status`,
`version`, `commit`, `build`, `config_digest`, `started_at` and
`uptime_seconds`.

```bash
for host in 10.0.0.11 10.0.0.12 10.0.0.13; do
  printf '%s ' "$host"
  curl -fsS "http://$host:8080/health" |
    python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["build"], d["config_digest"], d["status"])'
done
```

Every instance behind one balancer must report the same `config_digest`, or one
of them serves a different catalog to whichever clients reach it and nothing
else notices: the calls all succeed, and only the set of available actions
differs. The digest covers tool surface, capability surface, meta parameter
schema, tier and whether it was pinned, scope detection, read-only, safe mode
and excluded tools. It does **not** cover TLS, addresses, rate limits or pool
sizes, so it will not catch an instance that differs only in those.

`/health` deliberately does not test GitLab reachability, so a balancer must not
read a `200` as "GitLab is up", and there is no separate readiness endpoint.

### Telemetry with many users

Telemetry is off by default and goes to a collector you configure. Two settings
matter once the caller population is large:

- **`--telemetry-identity`** decides what is recorded about who made a call:
  `none` (the default, records nobody), `pseudonymous` (a keyed digest that
  correlates one caller's calls without naming them), or `full`. Identity never
  reaches a metric under any policy.
- **`GITLAB_MCP_TELEMETRY_IDENTITY_KEY`** is what makes `pseudonymous` agree
  across replicas. Left empty, each process generates its own key, so one caller
  is a different pseudonym on each instance and a distinct-user count is
  meaningless across a fleet. Set it when replicas must agree, and keep it away
  from wherever the telemetry lands: GitLab user ids are small enough to
  enumerate against a known key, which makes the key the "additional
  information" that turns a pseudonym back into a person.
- **`--telemetry-tool-name`** defaults to `auto`, which keeps
  `gen_ai.tool.name` as a metric dimension on the dynamic and meta surfaces and
  drops it on individual, where a thousand tool names would exhaust the SDK's
  cardinality limit and collapse the long tail into one overflow bucket.

See [Telemetry](telemetry.md) for the full picture.

### Fixed egress for GitLab allow-lists

GitLab applies its own rate limits and any IP allow-lists per source address.
Instances behind a NAT gateway with a stable address are one caller to GitLab;
instances with ephemeral public addresses are several unpredictable ones, and an
allow-list cannot be written for them. Give the fleet a fixed egress address
before you ask a GitLab administrator to allow-list it.

### Secrets

There is no server credential to protect: in HTTP mode every request carries the
caller's own token and `/health` needs none. What the deployment does hold:

- **The affinity salt.** A file the balancer reads, mode 0600, not in the
  repository. It is a distribution function's salt rather than a security
  primitive, but leaking it turns the routing key into a confirmable token
  fingerprint.
- **The TLS private key**, if the listener terminates TLS. Now rotatable in
  place without a restart.
- **`GITLAB_MCP_TELEMETRY_IDENTITY_KEY`**, if pseudonymous telemetry is on. It
  has no flag on purpose: process arguments are readable through `/proc` by any
  local principal. `GITLAB_TOKEN` has no flag for the same reason.

Pooled tokens are held in memory for as long as their entry lives, because a
client cannot call GitLab without one, and the pool is keyed by digest so a scan
of its lookup structures yields no credential. Nothing is written to disk. Logs
carry the last four characters of a token and nothing more.

### What to monitor

| Signal                                    | Where         | Why it matters                                                                                |
| ----------------------------------------- | ------------- | --------------------------------------------------------------------------------------------- |
| `config_digest` equality across instances | `/health`     | The only detector of an instance serving a different catalog                                  |
| `status` and the HTTP code                | `/health`     | `503 draining` is the balancer's cue, not a failure                                           |
| `build` per instance                      | `/health`     | Which version each instance is actually running                                               |
| Peak resident set                         | Host metrics  | The term that sizes the process is calls in flight, not credentials                           |
| Processor time per call                   | Telemetry     | The ceiling is threads divided by this                                                        |
| Pool evictions                            | Logs          | Frequent eviction means `--max-http-clients` is below the population                          |
| `429` rate                                | Balancer logs | Distinguish the per-credential limiter from the authentication budget                         |
| Authentication failures per address       | Logs          | Ten a minute blocks an address; behind a proxy without `--trusted-proxies` it blocks everyone |

That last row is the one that bites hardest at scale. Without
`--trusted-proxy-header` and `--trusted-proxies`, every caller's failures are
charged to the balancer's address, so ten bad tokens a minute from anywhere in
the population answers `429` to the entire deployment for a minute. Configure
both flags on any instance behind a proxy.

## Further reading

- [Remote Deployment](remote-deployment.md) for service units, containers, the individual proxies, and the first pass at several instances
- [HTTP Server Mode](http-server-mode.md) for every flag, the pool, the instance allow-list and the authentication modes
- [Resource Benchmark](../reference/resource-benchmark.md) for the measurements this page sizes from, and the host they were taken on
- [ADR-0020](../development/adr/adr-0020-one-server-per-configuration-shape.md) for why a credential is now nearly free and what that cost in isolation guarantees
- [Client Compatibility](client-compatibility.md) for gateway validators and per-client limits
- [Telemetry](telemetry.md) for what an instance reports about itself and its callers
- [Security](../concepts/security.md) for the threat model and the hardening checklist
