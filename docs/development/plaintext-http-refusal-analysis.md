# Refusing Plain HTTP off Loopback

This page weighed one change: refusing, by default, to start an HTTP listener
that serves plain HTTP on an address other machines can reach, unless TLS or an
explicit opt-in is given. It is the second half of the work the OpenSSF Best
Practices issue 1269 opened for the gold criterion `crypto_used_network`,
recorded as finding 6 of the
[Security Review, October 2026](security-review-2026-10.md#findings).

**The maintainer decided on 27 October 2026, after reading it, against a
default refusal** ([Decision](#decision)). Plain HTTP stays served by default,
with the warning, and the refusal is the hardened mode an operator asks for by
setting the opt-in to `false`. The analysis below is kept as the record of what
a default refusal would have cost.

## What the server does

The binary serves stdio unless it is told otherwise, so a client install
(Homebrew, winget, npm, PyPI, NuGet, the Claude Desktop extension, `docker run
-i`) opens no listener at all. HTTP mode needs `--http`, `--transport=http`, or
`--transport=auto` with stdin on the null device, and then `--http-addr` decides
the listener, with `--allow-plaintext-http` (or
`GITLAB_MCP_ALLOW_PLAINTEXT_HTTP`, which the flag wins over) saying what becomes
of one that serves plain HTTP to the network:

| `--http-addr` and TLS                                                                                                                     | Setting unset (the default)                                                        | Setting `true`                                                      | Setting `false` (the hardened mode)                                 |
| ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------- |
| A unix socket path                                                                                                                        | Served, nothing written                                                            | Served, nothing written                                             | Served, nothing written                                             |
| A loopback address, or a name that resolves only to loopback                                                                              | Served, nothing written                                                            | Served, nothing written                                             | Served, nothing written                                             |
| Any TCP address with `--tls-cert` and `--tls-key`                                                                                         | Served, nothing written                                                            | Served, nothing written                                             | Served, nothing written                                             |
| Any other TCP address: a wildcard bind (the default `:8080`, `0.0.0.0`, `[::]`), a routable address, a name that resolves off the machine | Served, with a `WARN` naming the address, the three remedies and the hardened mode | Served, with an `INFO` line saying it serves plain HTTP as declared | Refused: exit `1` with an error naming the address and the remedies |

The classification is `cmd/server.listenerIsHostLocal`, the same test that
admits `--allow-any-gitlab-url` (register row DST-002), the lines are
`plaintextListenerWarning` and `plaintextListenerAllowed` in
`cmd/server/listen.go`, and the refusal is register row ADM-015.
`checkPlaintextListener` runs in `runHTTP` after the TLS pair is validated and
before the bind. Unset or `true`, a listener serves exactly as it did before
the setting existed.

Two defaults put plain HTTP on a network without anyone asking for it. The
binary's `--http-addr` defaults to `:8080`, every interface, so
`gitlab-mcp-server --http --gitlab-url=…` serves plain HTTP to the network.
The container image's command is `--transport auto --http-addr 0.0.0.0:8080`,
and `auto` serves HTTP whenever the container has no stdin, which is every way
of running it as a service. Wherever the container has a network namespace of
its own (a Docker bridge network, a Kubernetes pod), loopback is not an option:
a published port, a Kubernetes Service and another container all arrive at the
container's own address, so the listener binds `0.0.0.0`, or that address,
which warns the same since it is routable. A container listener therefore
warns even when its port is published on loopback only. Loopback serves only
where the proxy shares the namespace: a container on `--network host`, or a
sidecar in the same pod.

## What the criterion asks

At gold, `crypto_used_network` is a MUST: the software must support secure
protocols for all its network communications, and insecure protocols such as
HTTP "MUST be disabled by default, and only enabled if the user specifically
configures it" ([criteria](https://www.bestpractices.dev/en/criteria/2#2.crypto_used_network)).
The silver form is a SHOULD and is met already: stdio, the default transport,
needs no network, HTTP mode is served only when `--http` or a transport asks
for it, and GitLab is reached over https by default. Passing has no such
criterion. The gold reading turns on the image: its default command serves
plain HTTP on every interface, so a `docker run -d -e GITLAB_URL=…` deployment
has plain HTTP enabled without configuring it. Neither the warning nor the
hardened mode changes that reading, since one reports a default and the other
is asked for, so the gold criterion stays unmet by the decision below.

Gold is out of reach while the project has one maintainer: `bus_factor`,
`contributors_unassociated` and `two_person_review` need a second person and
allow no N/A. A refusal closes one criterion of a level the project cannot
reach for other reasons, so the criterion sets no deadline. What it buys is the
protection itself: an operator who starts the binary with its defaults, or
publishes the image on every interface, stops sending tokens across the
network in the clear by accident.

## Who a default refusal would have stopped

The Today column is what startup writes by default. Under a default refusal, a
row that warns today would exit `1` at startup instead, which is what each of
them does now only in the hardened mode.

| Deployment                                                                                                                                                                                                                                                                                                                 | Listener                                                                                                                                   | Today                              | Under a refusal                       | What it would have to change                                                                                                                                                                                                                                                              |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Any client install over stdio                                                                                                                                                                                                                                                                                              | None                                                                                                                                       | Nothing                            | Unaffected                            | Nothing                                                                                                                                                                                                                                                                                   |
| The binary behind a proxy on the same machine, on loopback or a unix socket (the systemd, launchd and NSSM units of Remote Deployment, the Cloudflare Tunnel example)                                                                                                                                                      | `127.0.0.1:8080` or a socket path                                                                                                          | Nothing                            | Unaffected                            | Nothing                                                                                                                                                                                                                                                                                   |
| The binary with TLS on the listener, for a proxy on another machine (Enterprise TLS)                                                                                                                                                                                                                                       | `:8443` with `--tls-cert`                                                                                                                  | Nothing                            | Unaffected                            | Nothing                                                                                                                                                                                                                                                                                   |
| The binary started with the default address (the first commands of HTTP Server's Starting the server)                                                                                                                                                                                                                      | `:8080`                                                                                                                                    | `WARN`                             | Refused                               | `--http-addr=127.0.0.1:8080` for a local client or proxy, TLS for a remote one, or the opt-in. This is the deployment the refusal protects                                                                                                                                                |
| Replicas on other hosts behind one balancer (the worked nginx examples of Remote Deployment and Enterprise Load Balancing, which `proxy_pass http://` to `10.0.0.11:8080` and its siblings, and the HAProxy example of Enterprise Load Balancing, which balances and health-checks the same three addresses in plain HTTP) | A routable address, plain                                                                                                                  | `WARN`                             | Refused                               | TLS on each replica, with `proxy_pass https://` and `proxy_ssl_verify on` in nginx as Enterprise TLS recommends, or `ssl verify required` and a `ca-file` on each HAProxy `server` line, or the opt-in where the network between them is trusted. All three examples would have to change |
| The image with its default command (`docker run -p … -e GITLAB_URL=…`)                                                                                                                                                                                                                                                     | `0.0.0.0:8080` in the container                                                                                                            | `WARN`                             | Refused                               | `-e GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true` behind a proxy that terminates TLS. TLS on the listener or a socket in a shared volume means replacing the command, since `--tls-cert` and `--http-addr` have no variable                                                                       |
| The image published on loopback for a proxy on the host (the `docker run` and Compose examples of HTTP Server and Remote Deployment)                                                                                                                                                                                       | `0.0.0.0:8080` or `:8080` in the container, `127.0.0.1` on the host                                                                        | `WARN`                             | Refused                               | The variable, as above, or a unix socket in a directory shared with the proxy. Every such example would gain the variable                                                                                                                                                                 |
| The image published on every interface of the host (the `docker run` examples of the Docker install page, its hardened Single instance and Multi-instance ones among them, and the HTTP alternative in Troubleshooting, each with `-p 8080:8080`)                                                                          | `0.0.0.0:8080` in the container, every interface on the host                                                                               | `WARN`                             | Refused                               | Publishing on `127.0.0.1` for a proxy on the host, and then the variable or the flag behind a proxy that terminates TLS, or TLS on the listener. Every such example would have to change, and the page's paragraph on the warning with them                                               |
| The repository's `docker-compose.yml`                                                                                                                                                                                                                                                                                      | `0.0.0.0:8080`, published as `8080:8080` on every interface                                                                                | `WARN`                             | Refused                               | The file has to change in the refusing release, because it pulls `latest` with `pull_policy: always`: a `docker compose up` after that release pulls the refusal. It should also publish on `127.0.0.1` only                                                                              |
| `examples/telemetry/compose.yaml`                                                                                                                                                                                                                                                                                          | `0.0.0.0:8080` in the container                                                                                                            | `WARN`                             | Refused                               | The variable in its environment; it is a local demo, so declaring the hop is the honest setting                                                                                                                                                                                           |
| Kubernetes (no manifest ships here; every one is the operator's)                                                                                                                                                                                                                                                           | `0.0.0.0:8080` in the pod                                                                                                                  | `WARN`                             | Refused                               | The variable in the container spec behind an ingress that terminates TLS, or TLS from a mounted Secret with the ingress talking HTTPS to the backend; the listener re-reads a rotated pair without a restart                                                                              |
| A service mesh whose sidecar terminates mTLS                                                                                                                                                                                                                                                                               | Whatever the pod binds                                                                                                                     | `WARN` unless loopback             | Refused unless loopback               | The opt-in, unless the sidecar reaches the container on loopback, which depends on the mesh                                                                                                                                                                                               |
| The hosted endpoint, `https://mcp.jmrp.io/gitlab`                                                                                                                                                                                                                                                                          | Whatever its own deployment binds; that is configured in the mcp.jmrp.io repository                                                        | `WARN` off loopback without TLS    | Refused off loopback without TLS      | The variable in its deployment, set before it takes the refusing release, or a listener the refusal starts: TLS, loopback or a unix socket. Its configuration lives in the mcp.jmrp.io repository, not here                                                                               |
| This repository's own suites                                                                                                                                                                                                                                                                                               | A loopback address, except the plaintext cases of `test/e2e/http/listener_test.go` and the wildcard starts of `test/e2e/http/gate_test.go` | Nothing, the plaintext cases aside | Unaffected, the plaintext cases aside | The plaintext cases would assert the refusal and the opt-in instead of the warning. `test/e2e/gitlab` drives stdio, and HTTP on a loopback address; the wildcard starts of `gate_test.go` are refused by DST-002 before the listener is judged; `cmd/bench_resources` binds `127.0.0.1`   |

Three properties of the containers in that table decide most of the cost. A
container with a network namespace of its own cannot bind loopback and be
reached, so it binds `0.0.0.0` or its own address, which warns the same; only
`--network host` and a sidecar in the same pod reach it on loopback. An
environment variable is the only way a container adds a setting without
replacing the image's command, which is why the opt-in has one when no other
listener setting does. And a container that starts into the refusal exits,
and under a restart policy starts and exits again, with the reason in its log
and nowhere a dashboard looks first.

## What a default refusal would have looked like

The opt-in would have changed what it does without being renamed: under
`--allow-plaintext-http` or `GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true` the listener
would start and write the INFO line, and without either it would exit `1` with
an error built like the warning, naming the address and the three ways out: TLS
on the listener, a loopback address or unix socket, or the opt-in in both
spellings.

The code is the hardened mode's, which is in the tree now with the opposite
default: `checkPlaintextListener` returns the refusal before the bind when the
setting is `false`, and a default refusal would have returned it when the
setting is unset as well. The refusal is register row ADM-015, since a refusal
of startup is a decision of the register like DST-002's: that row refuses a
deployment that would let any caller name any instance, and this one a
deployment that would carry every caller's token across the network in the
clear.

The image would not have set the opt-in. Setting it there keeps every
container working and also keeps plain HTTP the image's default, which is the
one thing the criterion asks to change.

The rest of the change would have been documentation, and most of the work:
every example in the table above, the HTTP Server, Security, Command Line,
Environment, Configuration, Docker and Remote Deployment pages in both
languages, the assurance case's cleartext section and residual risk, finding 6,
and the version policy of the changelog page, which today names 3.1.0 as the
one minor release of the 3.x line that broke configuration.

## The alternatives

**Keep the warning and refuse nothing.** Nothing breaks, the INFO line under
the opt-in still records which deployments declared their hop, and the
criterion stays unmet. Finding 6 stays open, honestly described as a design
choice.

**Refuse in 3.4.0, in the same release as the warning.** This is what the
maintainer's position allows. Its cost is that no operator ever sees the
warning before the refusal: whoever upgrades to 3.4.0 meets the line and the
exit at once. The 3.x line has broken configuration in a minor release once,
and it warned a release ahead: 3.0.0 read the old variable spellings and
warned; 3.1.0 stopped reading them, warned about each one still set, and
refused to start only for the three that withhold something
(`GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE` and `EXCLUDE_TOOLS`). It came one
release later than planned, because 2.7.5 updated itself into 3.0.0 with
nobody reading a release note, not because the two-step form was chosen in
advance.

**Warn in 3.4.0 and refuse in the next minor.** The shape the retired names
took, warn one release and break the next: the warning reaches every log
first, the 3.4.0 notes announce the refusal, and the hosted endpoint and the
repository's compose files move to the opt-in before it lands.

**Change the default `--http-addr` to `127.0.0.1:8080`.** It removes the
binary's default exposure without refusing anything, and leaves the image
untouched, since the image names `0.0.0.0:8080`. It is also a silent break: a
binary deployment that relies on the default being reachable from another
machine stops answering it, with a connection refused and no message, which is
worse than a refusal that names its remedy. It fits beside a refusal, not
instead of one.

**Set the opt-in in the image.** Covered above: it keeps the default the
criterion objects to, and it would also silence the warning for a container
published on every interface, the one container deployment the warning most
needs to reach.

**Generate a self-signed certificate when no TLS is configured.** Every proxy
and client would have to skip verification or pin a certificate that changes
on each start, which teaches the habit `GITLAB_MCP_SKIP_TLS_VERIFY` exists to
discourage. Not proposed.

**Relax the rule inside a container.** Detecting one (`/.dockerenv`, cgroup
names) is guesswork, and a bridge network is a network: other containers on it
reach the listener. Not proposed.

## Recommendation

The page recommended warning in 3.4.0 and refusing in the minor release after
it, for the precedent of the retired names, warned about in 3.0.0 and broken in
3.1.0, the compose file that pulls `latest` on every start, and the absence of
a deadline, since gold stays out of reach whatever this decides.

## Decision

On 27 October 2026 the maintainer decided against a default refusal, in 3.4.0
or later, and for a hardened mode an operator asks for:

- **`--allow-plaintext-http` and `GITLAB_MCP_ALLOW_PLAINTEXT_HTTP` default to
  `true`**, so plain HTTP is served as it was. The flag wins over the variable,
  which wins over the default, and a value neither reads as a boolean fails
  startup, as for every HTTP setting.
- **Unset**, a plaintext listener off loopback is served with the warning, which
  now names the hardened mode as well as the three remedies.
- **`true`** is the declaration that something in front of the listener
  encrypts the hop: served, with the INFO line, as before.
- **`false`** is the hardened mode: startup refuses a listener that would serve
  plain HTTP off loopback, and starts only with `--tls-cert` and `--tls-key`, a
  loopback address or a name that resolves only to loopback, or a unix socket.
  The error names the address and the remedies (register row ADM-015).

Nothing in the table above that starts today stops starting, so no example,
compose file or hosted deployment has to change, and no release note announces
a break. What it costs is the criterion: at gold, `crypto_used_network` asks
that plain HTTP be off by default, and it stays unmet by design, beside the
three gold criteria a single maintainer cannot meet. The silver form, a SHOULD,
is met (above), and passing has no such criterion. Finding 6 of the security
review is recorded as a design choice.

## The hardened mode and a container's network

Whether a container starts in the hardened mode depends on its network, since
`listenerIsHostLocal` reads the address the server binds and a container's
loopback is its own. HTTP Server's
[The listener rule and the container's network](https://docs.jmrp.io/gitlab-mcp-server/operations/http-server/#the-listener-rule-and-the-containers-network)
is the operator's version of this list, in both languages:

- **Its own bridge network**: a proxy on the host arrives through a published
  port, which Docker forwards to the bridge address, so the server binds
  `0.0.0.0` and is off loopback even when the port is published on
  `127.0.0.1`. Refused in the hardened mode, warned about by default.
- **The host's network** (`network_mode: host`, Linux): the host's loopback is
  the container's, so `--http-addr=127.0.0.1:<port>` starts. `ports:` is
  ignored, each replica needs its own port, the proxy's peer is `127.0.0.1`,
  so `--trusted-proxies` names `127.0.0.1/32` or the authentication budgets
  charge every caller to it, and the container loses its network isolation.
- **Another container's network** (`network_mode: service:<other>`, a
  sidecar): loopback reaches only that namespace, whose owner publishes the
  port; a proxy on the host still arrives off loopback.
- **A unix socket in a directory bind-mounted from the host**: works across
  namespaces, with `--http-socket-mode` for the proxy's user, a directory
  writable by UID 10001 (a mount of its own under `--read-only`) and
  `--trusted-proxies=unix`. `--probe` reads the socket path off the server's
  command line (`discoveredTarget`) and dials it, so the image's health check
  keeps working.
- **TLS on the listener**, with the proxy verifying the certificate: starts on
  any of them.
