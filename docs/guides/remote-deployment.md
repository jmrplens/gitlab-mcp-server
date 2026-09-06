# Remote Deployment

[HTTP Server Mode](http-server-mode.md) documents the transport flag by flag.
This guide is the other half: how to stand one up for other people, on a machine
that stays running, reachable from somewhere other than the operator's laptop.

Everything here assumes HTTP mode. There is no remote form of stdio: stdio is a
pipe between one client process and one server process on the same machine, and
the moment a second person needs to reach it the answer is HTTP.

One property shapes every section, so it is worth stating before the first
snippet. **In HTTP mode the server holds no credential of its own.** Each
request carries the caller's GitLab token, and the server keeps a pooled MCP
server per `(token, GitLab URL)` pair. That is why there is no server token to
hide in a unit file, why `/health` can be unauthenticated, and why running
several instances is a caching problem rather than a state-replication one.

## Pick a shape first

| Situation                           | Shape                                                                                                    |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------- |
| One host, proxy and server together | Unix socket: `--http-addr=/run/gitlab-mcp/server.sock`                                                   |
| Proxy on another host               | `--tls-cert` and `--tls-key` on the listener                                                             |
| No proxy at all                     | `--tls-cert` and `--tls-key` plus `--auth-mode=oauth`; read [Direct exposure](#direct-exposure-no-proxy) |
| Containers                          | The published image, read-only root filesystem, digest pin                                               |
| More than one instance              | A balancer with token affinity: [Several instances](#several-instances-behind-one-proxy)                 |

## Run it as a service

### systemd

Two shapes, and what separates them is whether a same-host proxy has to reach a
unix socket.

**Loopback TCP with a dynamic user** is the least privileged form, and the one
to use when the proxy talks to `127.0.0.1`:

```ini
# /etc/systemd/system/gitlab-mcp-server.service
[Unit]
Description=GitLab MCP Server
Documentation=https://jmrp.io/docs/gitlab-mcp-server/operations/http-server/
After=network-online.target
Wants=network-online.target

[Service]
Type=exec
DynamicUser=yes
EnvironmentFile=/etc/gitlab-mcp-server/env
ExecStart=/usr/local/bin/gitlab-mcp-server \
    --http \
    --http-addr=127.0.0.1:8080 \
    --gitlab-url=https://gitlab.example.com \
    --auth-mode=oauth \
    --public-url=https://mcp.example.com \
    --trusted-proxy-header=X-Real-IP \
    --trusted-proxies=127.0.0.1
Restart=on-failure
RestartSec=2s
StandardInput=null

NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
PrivateTmp=yes
PrivateDevices=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
CapabilityBoundingSet=
AmbientCapabilities=
MemoryMax=512M

[Install]
WantedBy=multi-user.target
```

`ProtectSystem=strict` mounts the whole filesystem read-only and the server is
fine with that: it writes nothing to disk. Logs go to standard error, which
`journald` collects, and the only file it ever creates is the unix socket in the
second shape below. Reads are unaffected, so TLS certificates and the
environment file stay readable.

`RestrictAddressFamilies=` deliberately omits `AF_NETLINK`. The released binary
is built with `CGO_ENABLED=0`, so it uses Go's own resolver, which reaches DNS
over UDP and TCP and asks the kernel nothing through netlink. Verified against
this unit: a credential check that had to resolve and reach `gitlab.com` was
answered by GitLab with and without `AF_NETLINK` in the list.

`MemoryMax=512M` is the documented floor for HTTP mode rather than a target. The
base process is around 50 MB and each pooled token adds roughly 130 KB, so the
default `--max-http-clients=100` lands near 63 MB. See
[Resource Consumption](../concepts/resource-consumption.md) before sizing for
hundreds of callers.

**A unix socket needs a fixed user.** `DynamicUser=yes` allocates a transient
user and group per start, so there is no group the proxy can be added to. Create
a system user and share a real group:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin gitlab-mcp
sudo usermod -aG gitlab-mcp www-data   # or nginx, or caddy
```

Then replace the identity and address lines:

```ini
User=gitlab-mcp
Group=gitlab-mcp
RuntimeDirectory=gitlab-mcp
RuntimeDirectoryMode=0750
ExecStart=/usr/local/bin/gitlab-mcp-server \
    --http \
    --http-addr=/run/gitlab-mcp/server.sock \
    --http-socket-mode=0660 \
    --gitlab-url=https://gitlab.example.com
```

`RuntimeDirectory=` is what makes `/run/gitlab-mcp` exist, be owned by the
service, and be removed on stop. It also has to be **writable**, which it is:
the server does not bind the published path directly. It creates a `0700`
staging directory beside it, binds and chmods the socket there, then links it
into place, so a socket only ever appears with its final permissions. A
read-only parent directory therefore fails, even though the process writes
nothing else.

Four things about a unit file that are specific to this server:

- **State the transport.** `--transport auto` reads file descriptor 0 and picks
  HTTP only when stdin is `/dev/null`. systemd's default `StandardInput=null`
  happens to give the same answer, but a unit that sets `StandardInput=socket`
  or `tty` gets stdio instead. `--http` says what you mean and cannot be moved
  by how the supervisor wires a file descriptor.
- **Socket activation is not supported.** A `.socket` unit hands the listening
  socket to the service on file descriptor 0, and this server reads a socket
  there as "a supervisor handed me a stdio channel", so it would speak JSON-RPC
  down it instead of accepting connections. Bind the socket yourself with
  `--http-addr` and `RuntimeDirectory=`.
- **Secrets belong in `EnvironmentFile=`, never in `ExecStart=`.** A unit's
  command line is readable through `systemctl show` and `/proc` by any local
  user. HTTP mode has no GitLab token to hide there, but two real secrets exist:
  `GITLAB_MCP_TELEMETRY_IDENTITY_KEY`, which has no flag for exactly this
  reason, and whatever `OTEL_EXPORTER_OTLP_HEADERS` carries. Keep
  `/etc/gitlab-mcp-server/env` at mode `0600` owned by root; systemd reads it
  before dropping privileges.
- **A leftover socket is not always cleaned up for you.** On start the server
  probes an existing socket and removes it only when the connection is refused,
  which proves nothing is listening. A socket that is live is refused rather
  than stolen, a path that is not a socket is never deleted, and a probe that
  fails for any other reason (a permission-restricted directory, for instance)
  refuses to start and tells you to remove the file by hand.

Install and check:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gitlab-mcp-server
curl -fsS http://127.0.0.1:8080/health
journalctl -u gitlab-mcp-server -f
```

### launchd

macOS, as a system daemon under `/Library/LaunchDaemons/`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.github.jmrplens.gitlab-mcp-server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/gitlab-mcp-server</string>
        <string>--http</string>
        <string>--http-addr=127.0.0.1:8080</string>
        <string>--gitlab-url=https://gitlab.example.com</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>UserName</key>
    <string>_gitlabmcp</string>
    <key>StandardOutPath</key>
    <string>/usr/local/var/log/gitlab-mcp-server.log</string>
    <key>StandardErrorPath</key>
    <string>/usr/local/var/log/gitlab-mcp-server.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>GITLAB_MCP_LOG_LEVEL</key>
        <string>info</string>
    </dict>
</dict>
</plist>
```

```bash
sudo launchctl load -w /Library/LaunchDaemons/io.github.jmrplens.gitlab-mcp-server.plist
sudo launchctl print system/io.github.jmrplens.gitlab-mcp-server
```

The same two cautions apply. State `--http` rather than relying on `auto`, and
do not use launchd's `Sockets` key: it is socket activation by another name and
lands in the same misreading. A plist is world-readable, so a secret does not
belong in `EnvironmentVariables`; put it in a file the daemon reads instead,
named by `GITLAB_MCP_ENV_FILE` with an absolute path.

### Windows service

`sc.exe create` on its own does not work. The Service Control Manager expects
the executable it starts to register a service control handler and report back,
and a plain console binary does not, so the start fails with error 1053, "did
not respond to the start request in a timely fashion". A service wrapper bridges
that gap:

```powershell
nssm install GitLabMcpServer "C:\Program Files\gitlab-mcp-server\gitlab-mcp-server.exe"
nssm set GitLabMcpServer AppParameters "--http --http-addr=127.0.0.1:8080 --gitlab-url=https://gitlab.example.com"
nssm set GitLabMcpServer AppStdout "C:\ProgramData\gitlab-mcp-server\out.log"
nssm set GitLabMcpServer AppStderr "C:\ProgramData\gitlab-mcp-server\err.log"
nssm set GitLabMcpServer Start SERVICE_AUTO_START
nssm start GitLabMcpServer
```

`winget install --id jmrplens.gitlab-mcp-server -e` installs a **user-scope**
portable package under `%LOCALAPPDATA%\Microsoft\WinGet\Packages\`, reachable
through a symlink in a per-user `Links` directory. A service does not run as
that user and must not depend on that path, so for a service install either add
`--scope machine`, which installs under `%PROGRAMFILES%\WinGet\`, or copy the
executable to a machine-wide directory and point the wrapper at the real file
rather than at the symlink.

`--http-socket-mode` is accepted on Windows and not enforced: the server logs a
warning and lets the directory ACL decide. Use a TCP listener there.

## Run it with Docker

The image and its command line are described in
[Installation](installation.md#docker). What follows is the deployment shape.

### One container

```bash
docker run -d --name gitlab-mcp \
  --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  --read-only \
  --tmpfs /tmp:rw,size=64m,mode=1777 \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --memory 512m \
  -e GITLAB_URL=https://gitlab.example.com \
  ghcr.io/jmrplens/gitlab-mcp-server:2.7.5@sha256:8eec1825b266712cd544bf1b2144e55c1eb711b4540def40e963a664c4e97168
```

Five details worth a sentence each.

**The instance arrives as an environment variable, not an argument.** Any
argument after the image name replaces `CMD` wholesale, and `CMD` is
`--transport auto --http-addr 0.0.0.0:8080`. Writing
`docker run <image> --gitlab-url=…` would silently drop both, leaving the server
on its own `:8080` default with the transport inferred rather than stated.
`-e GITLAB_URL=` fills the instance list without touching the command.

**No `-i`.** `auto` serves HTTP precisely when stdin is `/dev/null`, which is
what `docker run` without `-i` gives it. Adding `-i` hands the container a pipe,
which means a client, and the container speaks stdio to nobody.

**`-p 127.0.0.1:8080:8080` binds the published port to loopback.** `-p
8080:8080` publishes on every interface and, with Docker's default iptables
integration, that goes around the host firewall.

**`--read-only` works because the image writes nothing at runtime.** The one
exception is a unix socket: binding one needs a writable parent directory for
the staging step described above, so give it a `--tmpfs` or a volume. The
container already runs as `appuser` (uid 10001), so `--user` is redundant, and
`--cap-drop ALL` is safe because nothing binds a privileged port.

**The image is pinned by digest, not only by tag.** A tag can be re-pushed; a
digest is what a client actually resolves. `2.7.5` and `latest` resolved to the
digest above on 2026-09-01, on `ghcr.io` and on the Docker Hub mirror.

### Compose

```yaml
services:
  gitlab-mcp-server:
    image: ghcr.io/jmrplens/gitlab-mcp-server:2.7.5
    restart: unless-stopped
    networks: [mcp]
    ports:
      - "127.0.0.1:8080:8080"
    command:
      - "--http"
      - "--http-addr=0.0.0.0:8080"
      - "--gitlab-url=https://gitlab.example.com"
      - "--auth-mode=oauth"
      - "--public-url=https://mcp.example.com"
      - "--trusted-proxy-header=X-Real-IP"
      - "--trusted-proxies=172.28.0.1"
    read_only: true
    tmpfs:
      - /tmp:rw,size=64m,mode=1777
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    deploy:
      resources:
        limits: { memory: 512M, cpus: "2.0" }
    healthcheck:
      test: ["CMD", "gitlab-mcp-server", "--probe"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    logging:
      driver: json-file
      options: { max-size: "10m", max-file: "3" }

networks:
  mcp:
    ipam:
      config:
        - subnet: 172.28.0.0/24
```

A `command:` list replaces `CMD`, which is why `--http` and `--http-addr` are
spelled out again. Naming an instance is not optional: HTTP mode exits with
`--gitlab-url is required in HTTP mode` when none is given, because a deployment
that names no instance would send whatever token a caller supplied to whatever
host that caller put in `GITLAB-URL`.

`--trusted-proxies` names where the proxy connects from, and that has to be an
address only the proxy can hold. The example declares its own network with a
fixed subnet and trusts `172.28.0.1` alone: the gateway, which is where a proxy
running on the host arrives from through the published port. A proxy that is
itself a container on that network gets a fixed `ipv4_address`, and that is the
value to trust instead. Docker's default pools (`172.16.0.0/12`) are the wrong
answer: every container on the host lives in them, and any of them could then
name the address a caller's failures are charged to.

Several flags have **no environment-variable equivalent** and must be arguments
here: `--http-addr`, `--http-socket-mode`, `--tls-cert`, `--tls-key`,
`--trusted-proxy-header`, `--trusted-proxies`, `--stateless`, `--json-response`,
`--http-idle-timeout`, `--max-request-body-bytes` and `--allow-any-gitlab-url`.
Most of the rest can come from the environment, `--rate-limit-rps` and
`--rate-limit-burst` included, where an explicitly passed flag still wins.

The image already carries a `HEALTHCHECK`, and from 2.8.0 it is
`gitlab-mcp-server --probe`: the binary finds the server process in the
container, reads
`--http-addr`, `--tls-cert` and the transport off its own command line, and
asks `/health` where it is actually served. Move the listener to another port,
to a unix socket, or behind `--tls-cert`, and the check follows without being
told. The Compose block restates it only so a change to the image's interval
does not silently change the deployment's. A container a client runs over
stdio (`docker run -i`) has no listener; the probe reports it healthy while the
process runs. `--probe <url>`, `--probe unix:<path>` or `--probe host:port`
skips the discovery, for a check run from outside the container.

The Compose block above restates the check, which is what an image up to 2.7.5
needs: those carry `wget` against `http://localhost:8080/health`, so any other
listener is reported unhealthy while it serves. Pinned to 2.8.0 or later the
`healthcheck:` block can be dropped and the image's own used.

### Secrets

HTTP mode has no GitLab token to inject: every request brings its own. What is
left is telemetry credentials, and those do not belong in `environment:`, which
`docker inspect` prints in full. Mount the file and name it:

```yaml
    secrets:
      - source: mcp_env
        target: /run/secrets/mcp.env
    environment:
      GITLAB_MCP_ENV_FILE: /run/secrets/mcp.env

secrets:
  mcp_env:
    file: ./secrets/mcp.env
```

`GITLAB_MCP_ENV_FILE` is resolved once from the process environment before any
dotenv file is loaded, so a loaded file cannot nominate another one, and it must
be an absolute path. It composes with `read_only: true`, because the secret
arrives as a read-only mount.

### One instance or several

`--gitlab-url` may be given more than once, or once comma-separated. One
instance pins the deployment and a `GITLAB-URL` header is ignored. Several
publish an allow-list, the header becomes required, and a header naming anything
outside the list is refused rather than honored. `--allow-any-gitlab-url` starts
with none published and lets the header name any host; a request that then omits
the header is refused with `400` rather than resolved to a default. It warns at
startup and belongs on a single-user local deployment only. The full table is in
[Publishing more than one instance](http-server-mode.md#publishing-more-than-one-instance).

## Behind a reverse proxy

### What every proxy has to get right

| Requirement                     | Why                                                                                                                                                                                           |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| No response buffering           | Streamable HTTP answers with `text/event-stream`. A buffering proxy turns a live stream into one delivery at the end                                                                          |
| Long read timeout               | A `subscriptions/listen` stream is silent between notifications. The server's SSE keep-alive fires every 25 seconds, under nginx's 60-second default, but a quiet stream still wants headroom |
| HTTP/1.1 upstream               | HTTP/1.0 to the upstream has no chunked transfer, which is what an SSE body uses                                                                                                              |
| Forward `Authorization`         | OAuth mode reads the bearer token from it; stripping it turns every request into a `401`                                                                                                      |
| Forward `PRIVATE-TOKEN`         | Legacy mode's header                                                                                                                                                                          |
| Forward `GITLAB-URL`            | Selects the instance when several are published                                                                                                                                               |
| Real client address             | `--trusted-proxy-header` names the header the proxy sets and `--trusted-proxies` the addresses it connects from, so the authentication-failure limiter counts callers rather than the proxy   |
| Route `/.well-known/` unchanged | In OAuth mode the RFC 9728 metadata lives at the **host root**, not under the path prefix                                                                                                     |
| Add no CORS headers             | The server answers preflight itself. Two `Access-Control-Allow-Origin` headers is a CORS failure, not a merge                                                                                 |

Three of those repay a longer look.

**The metadata path is at the host root.** `--public-url` is the RFC 9728
resource identifier, and the metadata URL is derived by inserting the well-known
segment between host and path. Started with
`--public-url=https://mcp.example.com/gitlab`, the server answers:

| Path                                                  | Result |
| ----------------------------------------------------- | ------ |
| `/gitlab/health`, `/health`                           | `200`  |
| `/gitlab/server-card`, `/server-card`                 | `200`  |
| `/.well-known/oauth-protected-resource`               | `404`  |
| `/.well-known/oauth-protected-resource/gitlab`        | `200`  |
| `/gitlab/.well-known/oauth-protected-resource/gitlab` | `404`  |

`/health` and the server card are mounted under the prefix as well as at the
root; the OAuth metadata is served at exactly one path, the derived one, and
the `401` challenge points at that host-root form. The path-less document
belongs to a resource that is the origin itself, so a server under a prefix
neither serves it nor should have it routed. A proxy that routes only `/gitlab`
will answer `404` for the document every OAuth client fetches first, and
discovery fails before the MCP endpoint is ever reached. Route the derived
path to the same upstream without rewriting it. The derivation itself is
documented under [OAuth Mode](http-server-mode.md#oauth-mode).

**Do not let the proxy speak CORS.** The server answers its own preflight from
`--trusted-origins`, to which the `--public-url` origin is added automatically.
A proxy that also emits `Access-Control-Allow-Origin` produces two of them.
Browsers reject that outright while `curl` reports a cheerful `200`, so it is a
failure visible only in the one client that matters. `test/e2e/http` pins it
with a real nginx; the remedy is in [Security](../concepts/security.md).

**Anything the proxy does not route is a `404`, not a `401`.** The MCP endpoint
is mounted on specific patterns rather than as a catch-all, so a misrouted path
answers `{"error":"not found"}` without authentication. A `404` where you
expected a `401` usually means the path never reached the handler.

### nginx

```nginx
upstream gitlab_mcp {
    server 127.0.0.1:8080;
    keepalive 16;
}

server {
    listen 443 ssl;
    http2 on;
    server_name mcp.example.com;

    ssl_certificate     /etc/letsencrypt/live/mcp.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/mcp.example.com/privkey.pem;

    # RFC 9728 metadata: host root, never rewritten.
    location /.well-known/oauth-protected-resource {
        proxy_pass http://gitlab_mcp;
        proxy_http_version 1.1;
    }

    location / {
        proxy_pass http://gitlab_mcp;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Server-sent events must not be held.
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
    }
}
```

Pair it with `--trusted-proxy-header=X-Real-IP --trusted-proxies=127.0.0.1`.
The list is what makes the header worth reading: it is believed only on a
connection from one of those addresses, and a request from anywhere else is
charged to its own peer address whatever header it carries. Without the list a
caller who reaches the listener directly would write the header themselves and
choose the address their failures are charged to, which is why the server
refuses to start with one flag and not the other.

Either header works. With `X-Forwarded-For` the server walks the value from the
right, skipping every hop that is itself in the list, and charges the first hop
that is not: with one nginx in front that is the client, and with a second proxy
in front of nginx it is still the client, provided both proxies are listed. A
value the client invented on the left is never reached, and a hop that is not
an address charges the peer instead. `X-Real-IP` set from `$remote_addr` at the
hop nearest the server carries one address and needs no walk, and it is the
header the project's own end-to-end proxy test uses.

nginx passes client request headers upstream by default, so `Authorization`,
`PRIVATE-TOKEN` and `GITLAB-URL` arrive with no `proxy_set_header` line. The one
nginx habit to know is that it drops headers containing underscores unless
`underscores_in_headers on` is set; none of this server's headers contain one.

### Caddy

```text
# Caddyfile
mcp.example.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1
        transport http {
            read_timeout 1h
        }
    }
}
```

Caddy obtains and renews the certificate itself and forwards request headers
unchanged, so most of the list above needs no configuration. `flush_interval -1`
disables response buffering explicitly: Caddy already flushes immediately for
`text/event-stream`, and `-1` means the behaviour no longer depends on the
upstream getting its content type right.

Caddy sets `X-Forwarded-For` by default, so
`--trusted-proxy-header=X-Forwarded-For --trusted-proxies=127.0.0.1` pairs with
this. A single site block covers the whole host, so the well-known path is
already routed.

### Traefik

With the Docker provider, on the container from the Compose block above:

```yaml
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.mcp.rule=Host(`mcp.example.com`)"
      - "traefik.http.routers.mcp.entrypoints=websecure"
      - "traefik.http.routers.mcp.tls.certresolver=le"
      - "traefik.http.services.mcp.loadbalancer.server.port=8080"
```

Traefik does not buffer responses unless a `buffering` middleware is added, so
the fix here is to not add one. What does need attention is the entry point's
responding timeout, which is global rather than per route:

```yaml
# traefik static configuration
entryPoints:
  websecure:
    address: ":443"
    transport:
      respondingTimeouts:
        readTimeout: 0
        idleTimeout: 3m
```

`readTimeout: 0` removes the limit on how long a request may take, which is what
a long-lived stream needs. Traefik sets `X-Forwarded-For` when it is the edge,
or when the previous hop is listed in `forwardedHeaders.trustedIPs`.

A `Host()` rule matches every path on the host, so the well-known document is
covered. A `PathPrefix()` rule is where the trap opens: add a second router for
`PathPrefix('/.well-known/oauth-protected-resource')` pointing at the same
service.

### Apache

```apache
<VirtualHost *:443>
    ServerName mcp.example.com

    SSLEngine on
    SSLCertificateFile    /etc/letsencrypt/live/mcp.example.com/fullchain.pem
    SSLCertificateKeyFile /etc/letsencrypt/live/mcp.example.com/privkey.pem

    ProxyPreserveHost On
    ProxyPass        / http://127.0.0.1:8080/ flushpackets=on timeout=3600 connectiontimeout=10
    ProxyPassReverse / http://127.0.0.1:8080/

    RequestHeader set X-Forwarded-Proto "https"
</VirtualHost>
```

`flushpackets=on` is the one that matters: without it `mod_proxy_http` buffers
the response body and the stream arrives all at once. `timeout=3600` is this
route's read timeout, separate from the server-wide `Timeout`.

Leave authentication to the MCP server, so nothing here should sit behind
`Require valid-user`; Apache forwards `Authorization` upstream when it is not
consuming it itself. `mod_remoteip` with `RemoteIPHeader X-Forwarded-For` fixes
the address in Apache's own logs, and the MCP server still needs
`--trusted-proxy-header` and `--trusted-proxies` for its own limiter.

Modules required: `proxy`, `proxy_http`, `ssl`, `headers`, and `remoteip` if you
use it.

### Cloudflare Tunnel

A tunnel replaces the inbound listener entirely: `cloudflared` dials out, so the
host needs no open port and no certificate of its own.

```yaml
# ~/.cloudflared/config.yml
tunnel: <tunnel-uuid>
credentials-file: /etc/cloudflared/<tunnel-uuid>.json

ingress:
  - hostname: mcp.example.com
    service: http://127.0.0.1:8080
    originRequest:
      connectTimeout: 10s
      disableChunkedEncoding: false
      noTLSVerify: false
  - service: http_status:404
```

Leave `disableChunkedEncoding` false. Setting it removes chunked transfer
encoding to the origin, which is what an SSE body needs.

Cloudflare sets `CF-Connecting-IP`, so name that one:
`--trusted-proxy-header=CF-Connecting-IP --trusted-proxies=127.0.0.1`, since
`cloudflared` connects from the same host. Prefer it over `X-Forwarded-For`
here, because Cloudflare rewrites `CF-Connecting-IP` on every request and a
client cannot forge it through the edge.

Two Cloudflare behaviours to check before blaming the server. The proxy applies
an inactivity limit to a response that a quiet `subscriptions/listen` stream can
reach even while the server's keep-alive holds the connection open at the TCP
level; and Cloudflare's own CORS and caching rules, if enabled for the hostname,
land in exactly the same collision as a proxy-level `Access-Control-Allow-Origin`.

## TLS between the proxy and the server

When the proxy shares the machine, remove the hop rather than encrypting it:
`--http-addr=/run/gitlab-mcp/server.sock`. When the proxy is elsewhere, the
binary terminates TLS itself and no sidecar is needed:

```bash
gitlab-mcp-server --http --http-addr=:8443 \
  --tls-cert=/etc/ssl/mcp.crt --tls-key=/etc/ssl/mcp.key \
  --gitlab-url=https://gitlab.example.com
```

Both flags or neither: `--tls-cert requires --tls-key` is a startup error, not a
warning. TLS 1.2 is the floor with no ceiling, so a current proxy negotiates
1.3 and anything below 1.2 is refused.

The pair is loaded at startup, and that has an operational consequence: **a
renewed certificate does not take effect until the process restarts.** There is
no reload signal; the only signals handled are `SIGINT` and `SIGTERM`, and both
mean shutdown. With certbot, that is a deploy hook:

```bash
# /etc/letsencrypt/renewal-hooks/deploy/gitlab-mcp-server.sh
#!/bin/sh
systemctl restart gitlab-mcp-server
```

A restart drops the pool and cuts open streams, so schedule renewal where a
brief reconnect is acceptable, or terminate TLS at a proxy that does reload in
place. The mechanics of both listeners are in
[Listening on a unix socket, or on TLS](http-server-mode.md#listening-on-a-unix-socket-or-on-tls).

## Direct exposure, no proxy

The binary answers its own CORS preflights, its own `404`s, its own security
headers, and can terminate TLS or listen on a unix socket. Nothing here needs a
proxy to be correct. A deployment without one should still be deliberate about
five things:

- **`--auth-mode=oauth`.** Bearer verification against the instance the request
  selected, with RFC 9728 metadata so clients discover it. `--public-url` is
  required and must be the externally reachable https origin.
- **Certificates and their renewal.** Loaded at startup, so renewal means a
  restart. See above.
- **Rate limiting.** `--rate-limit-rps` defaults to 10 in HTTP mode with a burst
  of 40, counted per pooled token rather than per address, and there is a
  separate per-address budget for authentication failures, which is the one that
  answers `429` with `Retry-After`. A throttled `tools/call` is not a `429`: it
  is a normal MCP result carrying an error, so it will not show up in HTTP-level
  monitoring.
- **Bounds.** `--max-http-clients` caps pooled entries, not sessions or
  concurrent requests. `--pool-idle-timeout` reclaims unused ones, except an
  entry with a live subscription, which is not idle however long it has been
  since it made a request. `--session-timeout` applies to stateful mode only.
- **The token passes through the box.** Every caller's GitLab token reaches this
  process, authenticates one request, and is never persisted. That is a property
  of the software. Whether the people whose tokens they are consider the machine
  trustworthy is a question the software cannot answer, so ask them rather than
  answering for them.

## Several instances behind one proxy

### Affinity is structural, not a preference

Each instance keeps two caches. Both are per process, both are keyed on the
caller, and neither can be shared:

- The **server pool**, keyed on `SHA-256(token + "\x00" + gitlabURL)`. On a miss
  the entry is built: the token's scopes are probed, the instance's licensing
  tier is detected, and a whole tool catalog is registered and pruned to that
  tier. The catalog build is the entire cost, measured at **1.8 seconds on the
  dynamic surface and 3.0 seconds on individual**, and it is paid on the first
  request of every credential rather than once per process the way stdio pays
  it. The handshake itself is answered immediately from a shell, so the cost
  lands on the first tool call rather than on `initialize`.
- The **OAuth identity cache**, keyed on instance and token, with
  `--oauth-cache-ttl` at 15 minutes by default. A miss is a round trip to GitLab
  to verify the credential.

Both hold live objects rather than serializable state, so there is no version of
this where a second instance reads the first one's cache. The question for a
balancer is therefore not whether requests will work anywhere. Under the default
`--stateless=true` every POST is self-contained and every instance answers
correctly. The question is how many times you are willing to pay for a cache
designed to be paid once.

Two things do not merely cost extra when a caller moves, they break. A stateful
session (`--stateless=false`) lives in the process that minted its
`Mcp-Session-Id`, and another instance answers that id with `404`, which a client
reads as its session having been terminated. Resource subscriptions are watchers
held by one process and disappear with it. If you run either, affinity is not an
optimization.

### The three distributions

| Distribution | Key                               | What it costs                                                                                                                                                                                                                                                                                                                              |
| ------------ | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Round robin  | none                              | Every instance eventually holds an entry for every caller, so pool memory multiplies by the instance count and each caller pays a 1.8 to 3.0 second catalog build on each instance it first touches. Token verification against GitLab goes from once per cache TTL to roughly once per TTL per instance. Correct, and pays N times over   |
| IP hash      | the client address                | Free, one directive, no secret to keep. The key is the wrong grain in both directions: an office, a VPN concentrator or a CI runner fleet is one address, so all of it collapses onto one instance, while one caller roaming between networks changes key and lands cold each time. Behind a CDN it needs the real address recovered first |
| Token hash   | a salted digest of the credential | Matches the grain of what is cached, because the pool key is derived from the token. Costs one secret to manage, a fallback for credential-less requests, and one cold entry whenever a caller rotates its token. This is what the hosted endpoint runs                                                                                    |

Token hash is the right answer here, and the reason is narrow enough to state
exactly: the thing being cached is keyed on the token, so the routing key should
be too. Neither of the others is wrong, and both are fine wherever one instance
serves the whole caller population, which is most deployments. Reach for a second
instance for availability, not for throughput: the ceiling that usually binds is
GitLab's own rate limit against one token, which no number of instances raises.

### Hashing the token without routing on it

The credential must not become the routing key. An affinity key ends up in the
balancer's memory, in its access log if the log format names the variable, and
in whatever upstream selection it drives. A raw bearer token in any of those is
a credential leak with extra steps.

The fix is a salted digest: hash a per-deployment secret together with the
credential, and route on that. The digest is a distribution function rather than
a security primitive; the salt is what does the security work. Without it the
key is a token fingerprint that anyone holding a candidate token could confirm.
With it the key is meaningless outside this deployment and cannot be replayed as
a credential.

Three details decide whether the affinity actually holds:

- **Normalize before hashing.** Strip the `Bearer` scheme prefix and surrounding
  whitespace. The same token spelled two ways hashes two ways and lands on two
  instances.
- **Fall back to the address, not to nothing.** A request with no credential has
  no key, and an empty key makes every anonymous request hash identically onto
  one instance. `$remote_addr` spreads them.
- **Use consistent hashing.** Removing one of three instances then relocates
  roughly a third of the callers instead of reshuffling all of them, which is
  the difference between a rolling update that warms one cold pool and one that
  warms every pool.

### A worked nginx balancer

`hash` accepts a string with variables and hashes it itself, so the salt can go
into the key expression without ever becoming a loggable variable of its own.
This needs no third-party module:

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

Do not log `$mcp_affinity` or `$mcp_credential`. Where an explicit digest is
preferred, `set_md5 $key "$mcp_salt$mcp_affinity"` from
`ngx_http_set_misc_module` (OpenResty, or Debian and Ubuntu's `nginx-extras`)
produces one, and njs does the same in a few lines. The hosted endpoint at
`mcp.jmrp.io` computes `md5(salt + bearer)` that way.

**Never add `non_idempotent` to `proxy_next_upstream`.** Without it, nginx
retries a `POST` on another instance only when the request was never sent, which
is exactly the guarantee wanted here: a `tools/call` that created an issue and
then lost its response must not be replayed on a second instance. `error
timeout` on its own is connection-level. Adding `http_500` or `non_idempotent`
turns one delivered mutating call into two.

Prove the affinity rather than assuming it. Two instances, eight tokens, six
requests each, reading the backend out of the access log:

```bash
for t in alpha bravo charlie delta echo foxtrot golf hotel; do
  for _ in $(seq 1 6); do
    curl -s -o /dev/null -H "Authorization: Bearer token-$t" \
      https://mcp.example.com/health
  done
done
```

Every token must show exactly one backend across its six requests, and the eight
tokens must not all show the same one. Run the same loop with no `Authorization`
header and with `PRIVATE-TOKEN` instead, to confirm both fallbacks pin as well.

### Rolling updates, probes and egress

**Announce the drain before the listener closes.** On `SIGTERM` the process
marks itself draining first: from that moment `/health` answers `503` with
`"status": "draining"` and `Cache-Control: no-store`. By default the listener
closes right after, so a balancer polling `/health` usually notices the closed
listener rather than the `503`, one probe later, and every request it sent in
that window failed. Start each instance with `--drain-delay` set to at least
one probe interval (`--drain-delay=10s` for a 5-second probe that tolerates two
failures): the listener then stays open that long answering `503`, the balancer
removes the backend, and only then does the listener close and the in-flight
requests get their 15 seconds to finish before the remaining connections are
closed. Under consistent hashing the callers pinned to it move to a neighbour,
warm an entry there, and come back when it returns. A balancer that cannot poll
still works the old way: remove the backend by hand, let streams drain, then
signal.

**`/health` is the probe, and it is honest about what it knows.** No credential,
no GitLab round trip, `200` with `status`, `version`, `commit`, `build`,
`config_digest`, `started_at` and `uptime_seconds`. `status` is `ok` while
serving and `draining` once shutdown was requested; the HTTP status carries the
same verdict. `build` is the label to put on a dashboard, the closest release
plus the short commit, so a release binary and a build from `main` read alike.
`config_digest` is the fleet check: every instance behind one balancer must
report the same one, or one of them serves a different catalog to whichever
clients reach it and nothing else notices. Compare it in the same loop that
proves the affinity. The endpoint deliberately does not test GitLab
reachability, so a balancer must not read a `200` as "GitLab is up", and there
is no separate readiness endpoint.

**Give the fleet a fixed egress address.** GitLab applies its own rate limits and
any IP allow-lists per source address. Instances behind a NAT gateway with a
stable address are one caller to GitLab; instances with ephemeral public
addresses are several unpredictable ones, and an allow-list cannot be written
for them.

**Remember the limiter multiplies.** `--rate-limit-rps` is per pooled token
entry inside one process, so three instances mean up to three buckets for one
caller unless affinity pins it to one. With token affinity the configured number
is the number. Under round robin, either divide it by the instance count or put
the real limit on the balancer.

**Revalidation keeps running per instance.** `--revalidate-interval` re-checks
pooled credentials every 15 minutes by default and evicts the ones GitLab now
rejects; setting it to `0` does not make a revoked token last forever, because
an entry older than an hour is rebuilt on next use anyway. That is per instance,
so the same token is re-checked once per instance holding it, which is one more
thing affinity keeps to one.

## Further reading

- [HTTP Server Mode](http-server-mode.md) for every flag, the server pool, and the OAuth derivation rules
- [OAuth App Setup](oauth-app-setup.md) for creating the GitLab application clients authorize against
- [Security](../concepts/security.md) for the threat model, CORS, and the hardening checklist
- [Resource Consumption](../concepts/resource-consumption.md) for sizing an instance
- [Telemetry](telemetry.md) for what an instance can report about itself
- [Troubleshooting](troubleshooting.md) for symptoms and their usual causes
