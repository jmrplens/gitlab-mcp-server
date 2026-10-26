# Refusing Plain HTTP off Loopback

This page weighs one change and decides nothing: refusing to start an HTTP
listener that serves plain HTTP on an address other machines can reach, unless
TLS or an explicit opt-in is given. The maintainer's position is that such a
refusal, if it happens, ships in 3.4.0 rather than waiting for a 4.0.0, and
only after this analysis has been read. It is the second half of the work the
OpenSSF Best Practices issue 1269 opened for the gold criterion
`crypto_used_network`, recorded as finding 6 of the
[Security Review, October 2026](security-review-2026-10.md#findings).

The first half is in the tree: the warning and the opt-in described below.
Everything this page says about a refusal is a proposal.

## What the server does today

The binary serves stdio unless it is told otherwise, so a client install
(Homebrew, winget, npm, PyPI, NuGet, the Claude Desktop extension, `docker run
-i`) opens no listener at all. HTTP mode needs `--http`, `--transport=http`, or
`--transport=auto` with stdin on the null device, and then `--http-addr` decides
the listener:

| `--http-addr` and TLS                                                                                                                     | What startup writes                                       |
| ----------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| A unix socket path                                                                                                                        | Nothing about the transport                               |
| A loopback address, or a name that resolves only to loopback                                                                              | Nothing about the transport                               |
| Any TCP address with `--tls-cert` and `--tls-key`                                                                                         | Nothing about the transport                               |
| Any other TCP address: a wildcard bind (the default `:8080`, `0.0.0.0`, `[::]`), a routable address, a name that resolves off the machine | `WARN`, naming the address and the three remedies         |
| The same, under `--allow-plaintext-http` or `GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true`                                                        | `INFO`, saying the listener serves plain HTTP as declared |

The classification is `cmd/server.listenerIsHostLocal`, the same test that
admits `--allow-any-gitlab-url` (register row DST-002), and the lines are
`plaintextListenerWarning` and `plaintextListenerAllowed` in
`cmd/server/listen.go`. `announcePlaintextListener` runs in `runHTTP` after the
TLS pair is validated and before the bind. Nothing refuses: a listener that
warned serves exactly as it did before the warning existed.

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
The silver form is a SHOULD and is met already: stdio needs no network, and
GitLab is reached over https by default. The gold reading turns on the image:
its default command serves plain HTTP on every interface, so a
`docker run -d -e GITLAB_URL=…` deployment has plain HTTP enabled without
configuring it. The warning does not change that reading, since it reports a
default rather than changing it.

Gold is out of reach while the project has one maintainer: `bus_factor`,
`contributors_unassociated` and `two_person_review` need a second person and
allow no N/A. A refusal closes one criterion of a level the project cannot
reach for other reasons, so the criterion sets no deadline. What it buys is the
protection itself: an operator who starts the binary with its defaults, or
publishes the image on every interface, stops sending tokens across the
network in the clear by accident.

## Who a refusal would stop

The Today column is what startup writes now. Under a refusal, a row that warns
today exits `1` at startup instead.

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
| The hosted endpoint, `https://mcp.jmrp.io/gitlab`                                                                                                                                                                                                                                                                          | The image behind nginx on the same host, over the Docker bridge                                                                            | `WARN`                             | Refused                               | The variable in its deployment, set before that host takes the refusing release, or a unix socket shared with nginx. Its configuration lives in the mcp.jmrp.io repository, not here                                                                                                      |
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

## What the opt-in would look like

The opt-in exists already, and a refusal would change what it does without
renaming it: under `--allow-plaintext-http` or
`GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true` the listener would start and write the
same INFO line it writes now, and without either it would exit `1` with an
error built like the warning, naming the address and the three ways out: TLS on
the listener, a loopback address or unix socket, or the opt-in in both
spellings.

The change in code is small. `announcePlaintextListener` returns an error
where it now warns, `runHTTP` returns that error before the bind, the unit
cases of `TestAnnouncePlaintextListener_SaysWhatTheListenerExposes` and
`TestRunHTTP_AnnouncesThePlaintextListenerItWasConfiguredFor` expect the error,
and the three plaintext cases of `test/e2e/http/listener_test.go` expect a
refused start and an opt-in that still serves. It needs no row in
`internal/tenancy`: the register holds decisions about who a caller is and
what it may hold, spend or be told, and this one is about how the operator's
process listens. DST-002 is in the register because its startup refusal decides
which instance a caller may name; a plaintext refusal decides nothing about any
caller.

The image would not set the opt-in. Setting it there keeps every container
working and also keeps plain HTTP the image's default, which is the one thing
the criterion asks to change.

The rest of the change is documentation, and it is most of the work: every
example in the table above, the HTTP Server, Security, Command Line,
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

Warn in 3.4.0, the release this change is planned for, and refuse in the minor
release after it, with the 3.4.0 notes announcing the refusal and naming the
variable. Before the refusing release is tagged: the hosted endpoint sets
`GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true` or moves to a unix socket,
`docker-compose.yml` publishes on loopback and sets the variable,
`examples/telemetry/compose.yaml` sets it, the examples published on every
interface move to loopback, and every example in the table gains the variable
or TLS. The reasons are the precedent of the retired names, warned about in
3.0.0 and broken in 3.1.0, the compose file that pulls `latest` on every
start, and the absence of a deadline: gold stays out of reach whatever this
decides, so one release of notice costs nothing the criterion would have paid
for.

If the refusal goes into 3.4.0 instead, the same list holds, with two
additions: the hosted endpoint has to set the variable before 3.4.0 is
published rather than before it is deployed, and the summary at the top of the
3.4.0 release names the refusal as the one configuration change, the way
3.1.0's named the variables it stopped reading.

Either way, the decision is the maintainer's, and nothing here is implemented
until he makes it.
