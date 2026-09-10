# ADR-0022: An operator-named instance is exempt from the outbound destination guard

## Status

Accepted, 2026-09-10.

## Context

This server makes outbound HTTP requests on a caller's behalf, which is the shape every server-side request forgery starts from. [GHSA-fcj2-hj27-hj26](https://github.com/jmrplens/gitlab-mcp-server/security/advisories/GHSA-fcj2-hj27-hj26) closed the largest half of it: HTTP mode now refuses to start without `--gitlab-url`, so a deployment names the instances it serves and a `GITLAB-URL` header selects among them rather than naming a host of its own. Item 3 of the reporter's remediation list, an outbound destination guard, was left open, and two reachable shapes remained.

**The hatch.** `--allow-any-gitlab-url` restores per-request selection for a single-user local deployment, on a loopback listener only. The caller is nominally the operator, but the thing choosing that header is a model that has read issue bodies, merge request descriptions and CI logs. A model persuaded to walk `http://10.0.0.1`, `http://10.0.0.2` and so on gets a distinguishable answer per host, and the upstream body is returned to it in both `content` and `structuredContent`. That is a network scanner, driven by untrusted text, on the operator's own network.

**The redirect hop, which fires on an ordinary pinned deployment.** `credentialSafeRedirect` follows a cross-host 302 with the credential headers stripped, because six shipped read-only actions exist only because of it: GitLab answers artifact, trace and package reads with a redirect to object storage whenever object storage is configured, which is GitLab.com and most self-managed instances. The credential policy decides what the hop may carry and cannot decide where it goes: the destination is chosen by whatever answered, and `job.trace` returns up to 100 KiB of the body raw. A compromised GitLab, or a cleartext leg where a response can be injected, therefore reached a cloud metadata endpoint and reflected it, on a deployment nobody misconfigured and that the instance allow-list had nothing to say about.

The obvious remedy is the one that breaks everybody. A great many people run this server against a GitLab on `localhost`, on `10.x`, on `192.168.x`, or behind a VPN on CGNAT `100.64.0.0/10`. A guard that refuses private destinations refuses every one of those deployments, and the usual answer to that, an exception list the operator maintains, is a second copy of the configuration they already wrote.

## Decision

A guard in two tiers, installed as `net.Dialer.ControlContext` inside `newBaseTransport`, with a policy carried on the request context by a thin `RoundTripper` (`internal/gitlab/destination.go`).

**Tier A, always on.** The cloud metadata addresses are refused on every hop, for every client, whatever the configuration says: `169.254.169.254`, `169.254.170.2`, `fd00:ec2::254` and `100.100.100.200`. They are named one by one rather than derived from their enclosing ranges, because a rule that applies to every deployment must be as narrow as it can be. Nothing legitimate serves a GitLab API or a presigned object-storage URL from one of them.

**Tier B, for destinations the operator did not choose.** Loopback, RFC 1918, CGNAT, link-local, unique-local and unspecified addresses are refused in exactly two situations: the instance itself was named by a `GITLAB-URL` header under `--allow-any-gitlab-url`, or a redirect hop left the instance's own host. The opt-out is `--allow-private-instances` (`GITLAB_MCP_ALLOW_PRIVATE_INSTANCES`).

**There is never a tier C.** An address reached because `--gitlab-url` or `GITLAB_URL` named it is exempt, whatever it resolves to, with no exception list and no configuration. That is the decision this record exists for, and the reasoning is: the guard constrains destinations the operator did **not** choose, and DNS rebinding is only a threat when the attacker controls the name. Here the operator wrote the name into their own configuration file. Refusing it would mean telling every self-hosted user that their GitLab is now a security risk to their own server, and the flag that undoes it would be passed by all of them, which is a guard that protects nobody and annoys everybody.

**One deployment shape needs care and gets it by construction.** A self-managed GitLab redirecting an artifact download to MinIO on the same private network is ordinary, and tier B's redirect rule would refuse it. So a redirect to a private address is allowed whenever the configured instance itself resolves private, because a deployment whose instance is inside a private network is already inside that network. Tier A still applies there: no object store is served from a metadata address.

**Placement.** `ControlContext` runs after resolution and once per candidate address. A name that resolves to something other than what it claims is therefore judged by what it resolved to, with no window between the check and the connection, because this is the connection. The same hook covers the first request and every redirect hop, since both reach the same dialer. The policy travels on the request context rather than on a transport of its own because `sharedBaseTransport` is one process-wide transport and a transport per client would give each of up to `--max-http-clients` pool entries its own idle-connection set. Connection reuse cannot walk past the guard: Go keys idle connections on scheme, host and port, so a connection to one destination is never handed to a request for another.

**Who says an instance was caller-named.** The server pool, and only it. A client is built from a URL string, and the string looks identical whether `--gitlab-url` published it or a header did; the pool is where the difference is known, so `buildEntry` calls `Client.MarkInstanceCallerNamed` when the deployment published no instance. The default is the other way round, because a client nobody marked is one the operator configured.

**The gate refuses early.** Under `--allow-any-gitlab-url`, a header naming a literal address the guard would decline is refused at the HTTP gate with a 400 that names the flag, through the same predicate (`CheckCallerNamedInstance`). The dialer remains the authority; this only spares an operator a server that admits their credential, builds a pool entry, and then fails every action. The gate resolves nothing: a host spelled as a name is left entirely to the dialer, which is the only check that can see what the name resolved to.

**The refusal names the flag.** `ErrDestinationRefused` is classified by `toolutil.ClassifyError` as a request that never left the process, and `WrapErr`, `WrapErrWithMessage` and `WrapErrWithHint` all route it through the hint that names `--allow-private-instances`. A handler's own hint is replaced rather than kept, for the reason an unattributed request drops one altogether: a handler's hint advises about GitLab state, and GitLab was never asked.

## Consequences

**Positive.**

- POS-001: The metadata endpoint is unreachable from every deployment shape, including the pinned one the allow-list had already secured, and including the redirect hop that no request of ours chose.
- POS-002: The self-hosted deployment is protected by construction rather than by a list somebody has to maintain, so the common case needs no configuration at all and cannot be broken by forgetting an entry.
- POS-003: The check is after resolution, so DNS rebinding is not a separate problem to solve, and one hook covers the first request and every redirect.
- POS-004: A refusal is a named error with a message that says nothing was sent and names the one flag that changes it, rather than a timeout or an unreachable-host diagnosis that sends an operator to check DNS.

**Negative.**

- NEG-001: `--allow-any-gitlab-url` against a private GitLab stops working until `--allow-private-instances` is passed. It is a breaking change, it is the one deliberate cost, and it is the argument for shipping it in a major release. The local hatch is now two flags: one saying a caller may name the instance, one saying that instance may be on this machine.
- NEG-002: A redirect from a **publicly resolving** instance to a **private** address is refused. Self-managed installs whose GitLab resolves publicly while its object store does not are the one combination with a real false-positive cost, and nothing here measures how many there are. The opt-out is documented and the first bug report is the measurement.
- NEG-003: Two clients in one process that target the same host and port share an idle connection, so a redirect destination vetted for one client's policy can be reused by another's without a second dial. It is reachable only between two pinned clients of different instances where one instance resolves private and the other does not, and only for the exact same host and port. Tier A is unaffected, since no client of any policy can establish the connection in the first place.
- NEG-004: A proxy named by `HTTPS_PROXY` is what gets dialed, so the guard judges the proxy rather than the destination behind it. A deployment that proxies its outbound traffic has delegated the decision to that proxy, which is where a rule about destinations belongs in that topology.
- NEG-005: Tier A has no opt-out at all. The P1 design allowed one and this record departs from it: the only flag it could be is `--allow-private-instances`, whose meaning is "my instance is on a private network", which is not a claim about instance credential endpoints. A single flag that meant both would let a deployment that needs the first silently acquire the second.
- NEG-006: The private-ness of the configured instance is resolved on the refusal path, so a redirect that is about to be refused may cost one DNS lookup, bounded at two seconds and memoized per client. A resolver that cannot answer leaves the destination refused rather than permitted.

## Compliance

- `internal/gitlab/destination_test.go`: `TestDestinationGuard_OperatorNamedInstance_IsExempt` is the row that protects the ordinary self-hosted deployment and is written first; `TestBaseTransport_RefusesPrivateDestinations` covers each address class through the real transport, including a hostname that resolves to loopback, which only a post-resolution check can refuse.
- `internal/gitlab/redirect_test.go`: `TestCredentialSafeRedirect_HopToMetadataAddress_Refused` pairs the refused metadata hop with a hop to ordinary object storage on the instance's own private network, which still succeeds.
- `test/e2e/http/gate_test.go`: `TestAllowAnyGitLabURL_LinkLocalHeader_Refused` drives the real binary and reads the policy from four sides, with no connection attempted to any of the addresses it names.
- `internal/serverpool/pool_test.go`: `TestBuildEntry_UnpublishedInstance_IsCallerNamed` holds the address constant and varies only who named it.
