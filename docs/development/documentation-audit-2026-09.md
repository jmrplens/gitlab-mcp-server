# Documentation Audit, September 2026

> **Diátaxis type**: Explanation
> **Audience**: Maintainers planning the 2.8.0 documentation work

What the documentation is today, what is wrong with it, what to do about it,
and in what order. Every claim below names a file and a line. Every claim about
behaviour was checked against a running binary rather than read off a page: the
method is at the end, and it is reproducible in about twenty minutes.

This is an audit and a plan. Six local corrections were applied to the tree,
each in its own commit and each listed in
[What was corrected in the tree](#what-was-corrected-in-the-tree). Everything
structural is a plan step, deliberately unexecuted, because several other
branches are editing documentation at the same time and a reorganisation that
lands before the reasoning has been read would collide with all of them.

---

## What the documentation is

| Corpus                                |   Files |       Words | Notes                                                    |
| ------------------------------------- | ------: | ----------: | -------------------------------------------------------- |
| `docs/`                               |     118 |     247,783 | 35,706 lines; 505 fenced blocks, of which 189 are `bash` |
| `site/src/content/docs/` (English)    |      46 |      89,543 | Astro Starlight, published at the docs domain            |
| `site/src/content/docs/es/` (Spanish) |      46 |      98,921 | A filename-for-filename mirror of the English tree       |
| `README.md`                           |       1 |       4,256 | 538 lines, carrying a quickstart of its own              |
| **Total**                             | **211** | **440,503** |                                                          |

Inside `docs/` the split is 44 per-domain tool pages, 17 ADRs, 12 guides, 8
concepts, 12 other reference pages, and the development set.

Three numbers frame everything that follows.

- **440,000 words across three parallel treatments.** `docs/` and the English
  site are two independently written manuals covering the same 19 topics, and
  the Spanish site is a third. On the two largest operator guides they are 58
  and 60 per cent verbatim identical, measured on shared non-blank lines longer
  than forty characters.
- **181 of the 189 shell blocks under `docs/` contain a command somebody could
  run.** Before this audit, no gate ran any of them. Seven of the other eight
  are environment-variable value samples and one is prose.
- **Zero broken links.** 1,946 link, image and `href` occurrences across 215
  files were extracted and resolved: 578 relative file links, 306 site-absolute
  links, 164 same-file anchors, 74 cross-file and site anchors, 64 asset
  references. Nothing is broken, nothing crosses the `docs/` and `site/`
  filesystem boundary, and no English page links into the Spanish tree. Every
  one of the 118 files under `docs/` is reachable within three clicks of
  `docs/README.md`. Findability inside `docs/` is not a problem and does not
  need work.

---

## The five paths

The issue names five paths a reader must be able to execute top to bottom. Each
was executed against the real binary with a stand-in GitLab, a real
OpenTelemetry collector and a real nginx.

| # | Path                            | Page a reader would follow                        | Executes as written? | The blocker                                                        |
| - | ------------------------------- | ------------------------------------------------- | -------------------- | ------------------------------------------------------------------ |
| 1 | Quick start to a first tool call | `docs/getting-started.md`                        | Partly               | The Docker route, which every one-click button uses, hangs today   |
| 2 | stdio, fully configured          | `docs/getting-started.md` + `docs/reference/env.md` | Yes                | No single page; the reader assembles it from four                  |
| 3 | HTTP from scratch                | `docs/guides/http-server-mode.md`                 | Yes                  | Three samples did not match the wire, now corrected                |
| 4 | HTTP with telemetry and a collector | `docs/guides/telemetry.md`                     | Half                 | No collector configuration exists anywhere in the repository       |
| 5 | Reverse proxies, LAN and public  | `docs/guides/remote-deployment.md`                | **No**               | Every published proxy configuration is answered `403`              |

### Path 1: quick start

`docs/getting-started.md` is 496 lines and offers eight installation routes
before its own manual walkthrough. It works as a page, and the path it puts
first does not.

**What was executed.** The stdio route with a downloaded binary: export
`GITLAB_TOKEN`, point `GITLAB_URL` at a stand-in instance, drive `initialize`,
`tools/list` and a `gitlab_find_action` call over pipes. It works. `initialize`
is answered in 0.05 s and `tools/list` in 0.94 s, with the catalog built on a
background goroutine, so the startup race recorded against earlier versions is
gone.

**What fails.** The Docker route at `docs/getting-started.md:39-43`, which is
also what all four one-click buttons at `docs/getting-started.md:30-33`
register, what `mcp.json` ships for Agent Plugins, and what
`docs/guides/ide-configuration.md:46` documents:

```bash
docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest
```

Against the currently published image this writes nothing to stdout and logs
`starting MCP server in HTTP mode, addr 0.0.0.0:8080` to stderr. An MCP client
waits at `initialize` forever. The cause is that the repository's `Dockerfile`
carries `CMD ["--transport", "auto", "--http-addr", "0.0.0.0:8080"]`, while
`docker inspect ghcr.io/jmrplens/gitlab-mcp-server:latest` reports
`["--http","--http-addr","0.0.0.0:8080"]`. The `--transport auto` change has not
been released. The `--http=false` workaround that used to sit in every example
was removed from all of them, including the shipped manifests, on the strength
of that unreleased change.

This is the same class of problem as
[the self-update claim](#d5-the-shipped-image-updates-itself-while-the-page-says-nothing-does),
and both are covered by plan step 2.

**What is missing.** There is no CLI-only path to a first tool call. Every route
in the page ends at "open your AI client", so a reader with no MCP client
cannot confirm the server works. The HTTP guide has such a check at
`docs/guides/http-server-mode.md:966-973`, six hundred lines into a different
document.

### Path 2: stdio, fully configured

**Executed and correct.** The dotenv precedence at
`docs/reference/configuration.md:353-361` behaves exactly as written: a working
directory `.env` is found, refused and reported at `WARN` with its absolute path
and the keys it wanted to set; `GITLAB_MCP_ENV_FILE` and `--env-file` load and
win over `~/.gitlab-mcp-server.env`; a relative value warns about following the
client into every workspace. `GITLAB_URL` really does default to
`https://gitlab.com`. Read-only withholds `issue.create` with a message that
tells the model not to report the capability as missing; safe mode answers with
a `{"status":"blocked","mode":"safe"}` preview. Every documented tool count is
exact: `--tool-surface=individual` registers 854, 1007 and 1073 tools at Free,
Premium and Ultimate, and `--tool-surface=meta` registers 32, 38 and 49.

**What is missing.** There is no stdio page. A reader assembling a full stdio
deployment needs `docs/getting-started.md` for the client JSON,
`docs/reference/env.md` for the variables, `docs/guides/ide-configuration.md`
for their own client, and `docs/concepts/security.md` for the allow-lists.
Nothing tells them that is the set.

**One defect found.** `--tool-search` is documented at
`docs/reference/cli.md:32` as honouring `--tool-surface` and `--tier`. It reads
the flags and ignores the environment variables, which are the stdio spelling,
so a stdio operator searching their own surface gets nothing. See
[D2](#d2-the-tool-search-flag-ignores-the-environment-that-configures-stdio).

### Path 3: HTTP from scratch

**The strongest page in the tree.** Fifteen documented behaviours were driven
against the binary and fifteen matched, including several that are easy to get
wrong:

| Claim                                                                 | Page                                       | Observed                                               |
| --------------------------------------------------------------------- | ------------------------------------------ | ------------------------------------------------------ |
| Three instance-policy shapes start                                    | `docs/guides/http-server-mode.md:32-47`    | All three                                              |
| Several published, header absent                                      | `docs/guides/http-server-mode.md:331`      | `400`, message names no hostnames                      |
| Several published, header names an outsider                           | `docs/guides/http-server-mode.md:331`      | `400`, "an instance this deployment does not serve"    |
| `--allow-any-gitlab-url` with no header                               | `docs/guides/http-server-mode.md:44-46`    | `400`, not resolved to `gitlab.com`                    |
| `--allow-any-gitlab-url` warns at startup                             | `docs/reference/cli.md:62`                 | One `WARN` naming the exposure                         |
| One published, header ignored                                         | `docs/guides/http-server-mode.md:923`      | `200` plus the `ignored_options` warning               |
| The `curl` smoke test, verbatim, no protocol header                   | `docs/guides/http-server-mode.md:637-642`  | `200` and an SSE `tools/list`                          |
| `/health` field set                                                   | `docs/guides/http-server-mode.md:936-946`  | All seven fields, exact names                          |
| `/server-card`                                                        | `docs/guides/http-server-mode.md:983`      | `200`                                                  |
| Unrouted path is `404`, unauthenticated                               | `docs/guides/http-server-mode.md:131`      | `404`                                                  |
| RFC 9728 metadata at the derived path only                            | `docs/guides/remote-deployment.md:435-441` | All five rows exactly                                  |
| The `401` challenge points at the host-root metadata                  | `docs/guides/http-server-mode.md:373`      | Correct `resource_metadata`                            |
| Unix socket listener                                                  | `docs/guides/http-server-mode.md:654`      | Socket at mode `0660`, `/health` over it               |
| TLS on the listener                                                   | `docs/guides/http-server-mode.md:687`      | HTTPS served, plaintext to it gets `400`               |
| `--drain-delay`                                                       | `docs/reference/cli.md:77`                 | `503 draining`, `Cache-Control: no-store`, then closed |
| `--trusted-proxy-header` without `--trusted-proxies` refuses to start | `docs/guides/remote-deployment.md:505`     | Refused, both directions                               |

Three samples in the page did not match the wire and have been corrected: the
`401` body's `id`, the startup and pool log lines, and the unqualified claim
that `GET` and `DELETE` need no credential.

### Path 4: HTTP with telemetry and a collector

**Half of this path does not exist.** `docs/guides/telemetry.md` is 479 lines
and is the best-researched page in the repository: the four traps in the
standard variables at lines 47 to 105 are each a real specification or
implementation detail with the reasoning attached, and the header encoding
section is worth more than the OpenTelemetry documentation it summarises. Every
span attribute and metric instrument it lists at lines 156 to 240 was observed
arriving at a collector.

There is no collector. Grepping `docs/` and `site/src/content/docs/` for
`receivers:`, `exporters:`, `otelcol` or `opentelemetry-collector` returns
nothing. A reader who follows the page has a server exporting OTLP to an
endpoint they have not been shown how to stand up, and no way to see whether
anything arrived.

**What was executed, and what the plan should carry.** An
`otel/opentelemetry-collector-contrib` container with an OTLP receiver on 4317
and 4318, a `batch` processor and a `debug` exporter at `verbosity: detailed`,
with the server started as:

```bash
GITLAB_MCP_TELEMETRY=true \
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
OTEL_SERVICE_NAME=gitlab-mcp-server \
OTEL_METRIC_EXPORT_INTERVAL=3000 \
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --telemetry-identity=pseudonymous
```

All three signals arrive within eight seconds. The collector reports spans named
`tools/list`, `tools/call gitlab_find_action` and `server/discover`; the metrics
`mcp.server.operation.duration`, `mcp.client.operation.duration`,
`http.client.request.duration` and `http.server.request.duration`; and the
server's own log records carrying the trace and span ids of the request that
wrote them. Span attributes observed include `mcp.method.name`,
`mcp.protocol.version`, `gen_ai.tool.name`, `gen_ai.operation.name`,
`gitlab_mcp.tool_surface`, `network.transport` and `rpc.response.status_code`,
which is the set the page documents.

That configuration is fifteen lines and provably works. Plan step 4 is to put it
in the page with a compose file beside it.

### Path 5: reverse proxies, LAN and public

**This path does not work as written, and it is the most serious finding in the
audit.** Every reverse-proxy configuration `docs/guides/remote-deployment.md`
publishes is answered `403 Forbidden` on every request, `/health` included, for
the most common deployment shape.

Reproduced with the real nginx, the documented configuration copied verbatim
from `docs/guides/remote-deployment.md:465-502`:

```text
POST https://mcp.example.com/mcp  ->  403
{"jsonrpc":"2.0","id":1,"error":{"code":-40300,
 "message":"Request refused: the Host header names a host this deployment does not serve."}}
```

There are two host guards, and between them they cover every documented
same-host shape except the unix socket.

| `--http-addr`             | Proxy reaches the server over | Client `Host` preserved | Result | Guard                                     |
| ------------------------- | ----------------------------- | ----------------------- | ------ | ----------------------------------------- |
| `:8080` or `0.0.0.0:8080` | loopback                      | `mcp.example.com`       | `403`  | SDK localhost protection, plain-text body |
| `127.0.0.1:8080`          | loopback                      | `mcp.example.com`       | `403`  | Both guards                               |
| `192.168.1.5:8080`        | that address                  | `mcp.example.com`       | `403`  | `hostValidationMiddleware`                |
| `:8080`                   | a non-loopback address        | `mcp.example.com`       | `200`  | Neither applies                           |
| unix socket               | the socket                    | `mcp.example.com`       | `200`  | Neither applies                           |

The second guard is the project's own: `allowedHosts` at
`cmd/server/main.go:3441` returns `{listen host, localhost, 127.0.0.1, ::1}`
whenever the listen address names a host, and `hostValidationMiddleware` at
`cmd/server/main.go:3822` refuses anything else. The first is the SDK's, at
`mcp/streamable.go:325-331` of go-sdk v1.7.0: when the connection arrived on a
loopback local address and `Host` is not itself loopback, it answers `403` in
plain text before any middleware of ours runs. It is controlled by
`StreamableHTTPOptions.DisableLocalhostProtection`, which this server never
sets, so there is no flag that turns it off.

All four published proxies preserve the client's `Host` and connect over
loopback, which is exactly the combination that fails:

| Proxy             | Configuration                                        | Line                                       |
| ----------------- | ---------------------------------------------------- | ------------------------------------------ |
| nginx             | `proxy_set_header Host $host;` to `127.0.0.1:8080`   | `docs/guides/remote-deployment.md:490`     |
| Caddy             | `reverse_proxy 127.0.0.1:8080`, Host kept by default | `docs/guides/remote-deployment.md:528-538` |
| Apache            | `ProxyPreserveHost On` to `http://127.0.0.1:8080/`   | `docs/guides/remote-deployment.md:598`     |
| Cloudflare Tunnel | `service: http://127.0.0.1:8080`, Host from the edge | `docs/guides/remote-deployment.md:624-637` |
| nginx balancer    | `proxy_set_header Host $host;`                       | `docs/guides/remote-deployment.md:833`     |

`--public-url` does not help: the host it names is not added to the allowed set,
so an OAuth deployment started with `--public-url=https://mcp.example.com` still
refuses `Host: mcp.example.com`. The requirement table at
`docs/guides/remote-deployment.md:416-430` lists nine things a proxy must get
right and `Host` is not among them, and the page's own troubleshooting note at
line 461 covers `404` and `401` but not `403`.

The fix at the proxy is one line, verified: `proxy_set_header Host localhost;`
makes the same nginx configuration answer `200` on both `/mcp` and `/health`.
Whether that is the right fix is a design decision, which is why it is plan step
1 rather than a commit here. See
[D1](#d1-the-host-guards-refuse-every-documented-reverse-proxy).

**Why no test caught it.** `test/e2e/http/proxy_test.go` does drive a real
nginx, and it does use `proxy_set_header Host $host;`, but its client connects
to `127.0.0.1`, so `$host` is a loopback name and both guards pass. The test is
green on a shape no deployment uses.

**What else is missing.** The issue asks for a LAN shape and a public shape. All
five proxy examples are the public shape: `listen 443 ssl`, `mcp.example.com`,
Let's Encrypt paths. The shape table at
`docs/guides/remote-deployment.md:20-26` names a LAN deployment but shows no
configuration for one. A plain-HTTP nginx front, which is what a LAN deployment
behind a proxy that terminates nothing actually looks like, was executed here
and works once the `Host` problem is solved.

---

## Organisation

### Duplication

`docs/` and the site are two complete manuals, and the Spanish site is a third.
Nineteen topics exist as a pair. The verbatim overlap on the two largest
operator guides:

| Pair                                                                            | Verbatim shared long lines | Fuller | Cross-linked? |
| ------------------------------------------------------------------------------- | -------------------------: | ------ | ------------- |
| `docs/guides/telemetry.md` and `site/src/content/docs/operations/telemetry.mdx` |                       60 % | `docs` | **No**        |
| `docs/guides/remote-deployment.md` and `.../operations/remote-deployment.mdx`   |                       58 % | `docs` | **No**        |
| `docs/guides/http-server-mode.md` and `.../operations/http-server.mdx`          |                        5 % | `docs` | Yes           |
| `docs/reference/env.md` and `site/src/content/docs/configuration.mdx`           |                        6 % | `docs` | Partly        |

Only 10 of the 19 `docs/` pages with a site twin carry the
`📖 **User documentation**` banner that points at it, and the two most
duplicated pages are among the nine that do not. A reader who lands on either
has no way to learn the other exists.

The repetition is measurable and it agrees today, which is precisely the danger:

- **The environment-variable table exists five times in English and six with
  Spanish.** `docs/reference/env.md:41-155` is the superset at 44 rows;
  `docs/reference/configuration.md:47-75` and `:216-232` hold 27,
  `site/src/content/docs/configuration.mdx:79-131` holds 29,
  `docs/getting-started.md:325-345` holds 17 and
  `docs/concepts/architecture.md:148-160` holds 7. Every one is a strict subset,
  with zero semantic disagreements in the shared rows.
- **The CLI-flag table exists five times**, with 57, 55, 52, 49 and 38 rows.
  Spot-checked defaults agree across all five.
- **Fourteen distinct English surfaces tell a reader how to install**, 24
  counting Spanish. `docs/getting-started.md` does it twice in one file, at
  lines 22 to 171 and again at 172 to 197.
- **Twenty-seven files carry an `mcpServers` or `servers` JSON block.**
- **Thirty English locations explain dynamic versus meta versus individual**,
  not counting the one-line repeat at line 17 of each of the 44 tool pages.

### Diátaxis placement

Six pages declare a type that contradicts the directory they sit in. The
section indexes declare one type for the whole directory
(`docs/guides/README.md:11` How-to, `docs/reference/README.md:17` Reference,
`docs/concepts/README.md:11` Explanation):

| Page                                      | Declares                      | Sits in      | Belongs in                                |
| ----------------------------------------- | ----------------------------- | ------------ | ----------------------------------------- |
| `docs/concepts/meta-tools.md:5`           | Reference, and its H1 says so | `concepts/`  | `reference/`                              |
| `docs/concepts/resource-consumption.md:5` | Reference                     | `concepts/`  | Split three ways, see below               |
| `docs/concepts/dynamic-tools.md:5`        | "Guide + Reference"           | `concepts/`  | Split                                     |
| `docs/guides/http-server-mode.md:5`       | Explanation                   | `guides/`    | Its 52-flag table belongs in `reference/` |
| `docs/guides/ide-configuration.md:5`      | Reference                     | `guides/`    | `reference/`                              |
| `docs/reference/output-format.md:5`       | Explanation                   | `reference/` | `concepts/`                               |

`docs/concepts/resource-consumption.md` is three documents in 213 lines:
measured tables at 16 to 76, capacity-planning recipes at 165 to 207, and the
explanation in between. Its measured half overlaps
`docs/reference/resource-benchmark.md`, which is the generated and CI-validated
page and is where measurements belong.

`docs/reference/capabilities/README.md:3` says "the 4 MCP capabilities" and the
directory holds five pages. Icons is the fifth, filed in a separate `## Features`
table at line 79 whose numbering restarts at 1, and the site itself says at
`site/src/content/docs/capabilities/icons.mdx:20` that icons are "not a
negotiated MCP protocol capability". The page also carries a 167-line how-to
(`## Building a Custom MCP Client`, line 73) and two contributor sections inside
a reference page.

Four pages under `docs/development/` declare a user audience while the index at
`docs/development/README.md:13` declares the directory is for contributors:
`token-footprint.md:4`, `testing/model-results.md` (no banner at all, linked
three times from the README), `testing/model-evaluation.md:4` and
`testing/README.md:3`. Going the other way,
`docs/guides/examples/generate-release-notes-skill.md:3` declares "Contributors"
inside the user tree.

### Findability

Inside `docs/`, findability is solved: zero orphans, zero broken links, and all
118 files within three clicks of `docs/README.md`.

The gap is at the front door. From `README.md`, 14 files are unreachable at any
depth, and they include `docs/README.md` and all five section indexes.
`README.md:356` deep-links to leaf pages and never to an index, so a GitHub
reader never learns the structure exists.

One pointer is aimed at the wrong page:
`docs/reference/configuration.md:117` sends a reader wanting per-client setup
for eight named clients to `docs/getting-started.md`, which covers three. The
708-line, 13-client `docs/guides/ide-configuration.md` is never linked from the
configuration reference at all.

The site sidebar at `site/astro.config.mjs:576` is explicit rather than
autogenerated, so any page added without editing that file is an orphan. That is
a latent trap, not a current defect.

### Contradictions

Facts checked and found **consistent** across every page that states them: the
default tool surface, the default rate limit and its stdio and HTTP split,
whether `GITLAB_URL` is required, the default transport, the `--stateless`
default, tier detection, the tool and resource and prompt counts, the minimum Go
version, and the version number. Those were the high-risk candidates and they
are clean.

What is not:

1. **Binary size.** `docs/concepts/resource-consumption.md:17` said `~49 MB`;
   `site/src/content/docs/operations/http-server.mdx:592` says `~55 MB`.
   Measured: a stripped `-trimpath` release build is 54.8 MB, and the binary
   inside the published 2.7.5 image is 57.4 MB. **Corrected in the tree.**
2. **Per-credential memory.** `docs/concepts/resource-consumption.md:60-64`
   states 71, 35 and 36 MiB per further credential; twelve lines above, the same
   page states the real figures are 17, 73 and 8 KiB, a factor of about 4,000,
   and says the tables stay until the reference host is re-run.
   `docs/guides/http-server-mode.md:908` says "~50 MB (full process)". A reader
   landing on the concepts page gets a number wrong by three orders of magnitude
   beside an admission that it is wrong. Not corrected here: it needs the
   benchmark re-run, which is plan step 6.
3. **The pool model.** Since one MCP server is built per configuration shape and
   shared, seven statements are stale:
   `docs/guides/http-server-mode.md:16` ("isolated MCP server instance per
   unique token"), `:823`, `:825`, `:844`, `docs/concepts/security.md:457` and
   `:460-462`, `site/src/content/docs/operations/http-server.mdx:11`, `:311`,
   `:334`, `site/src/content/docs/operations/security.mdx:189`, and
   `docs/development/testing/testing.md:75`. The conclusion each draws (one
   rate-limit bucket per credential) is still true; the mechanism each states is
   not. Plan step 5.
4. **The meta-surface count.** `docs/guides/client-compatibility.md:100` said
   "~33 to 50" where every other page and the binary say 32 to 50; the 33 came
   from counting `gitlab_server`, which `docs/concepts/meta-tools.md:11`
   explicitly excludes. **Corrected in the tree.**
5. **Hardcoded counts beside generated ones.**
   `site/src/content/docs/architecture.mdx:165` writes the literal `38` in the
   same sentence as `{stats.meta.self_managed_enterprise}`, because
   `site/src/data/stats.json` has no `meta.premium` key. When the catalog
   changes the site half-corrects itself and `docs/` does not.
6. **A correction applied to one half of a pair.**
   `docs/reference/resource-benchmark.md:358-376` records that "the server idles
   near 181 MB" and "512 MB is the floor" were published and are wrong, and says
   "Both are corrected there", linking to the site.
   `docs/guides/http-server-mode.md` never got a sizing section at all, so the
   docs half lost the guidance rather than gaining the correction.

### Audience banners

The convention is a blockquote after the H1, in two shapes: multi-line (82
pages) and single-line joined with `·` (10 pages).

| Field               | Pages | Share |
| ------------------- | ----: | ----: |
| `**Diátaxis type**` |    92 |  78 % |
| `**Audience**`      |    92 |  78 % |
| `**Prerequisites**` |    29 |  25 % |

The audience field has 29 distinct wordings for what is really four audiences,
differing mostly by an emoji. `docs/development/testing/README.md:3` declares
`**Diátaxis type**: Overview`, which is not one of the four types.

Twenty-six pages carry no banner: all 17 ADRs, plus `docs/README.md` itself,
`docs/guides/telemetry.md`, `docs/guides/remote-deployment.md`,
`docs/guides/client-compatibility.md`,
`docs/guides/claude-desktop-extension.md`,
`docs/reference/resource-benchmark.md`, `docs/reference/tools/README.md`,
`docs/development/upstream-bugs.md` and
`docs/development/testing/model-results.md`.

---

## The two languages

The Spanish site follows the English one closely. All 46 filenames match in both
directions, and for all 46 pairs the H2 and H3 nesting sequence, the fenced
block count, the table block count and every table's row count are identical.
No Spanish page has an untranslated body: the word-count ratio runs from 1.05 to
1.245 with nothing near 1.00.

Two real divergences, in the order a person should work through them. **Do not
fix these here**; another branch is reconciling the pages this stack touches.

1. **`site/src/content/docs/es/tools/orbit.mdx:6` carries an FAQ entry that has
   no English counterpart.** The Spanish `faq:` array has six entries and the
   English has five; the extra one is
   `¿Qué es Orbit en GitLab MCP Server?`. The insertion also reorders the other
   five relative to English. Both sides render `FAQPage` structured data, so
   English readers and crawlers get strictly less than Spanish ones. Either add
   an equivalent English entry before `site/src/content/docs/tools/orbit.mdx:6`
   or drop the Spanish one.
2. **A reserved documentation domain was localised.**
   `site/src/content/docs/es/operations/http-server.mdx:143` and `:318` write
   `gitlab.ejemplo.com` and `self-hosted.ejemplo.com`. RFC 2606 reserves
   `example.com` and guarantees it never resolves; `ejemplo.com` is a real
   registrable name. Every other occurrence in the same file, about fifteen,
   correctly keeps `example.com`.

Cosmetic placeholder translations inside fenced blocks, which are harmless but
break strict code parity, at 25 lines across five files:

| What                                        | Files and lines                                                                                                                             |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `/path/to/…` becomes `/ruta/a/…`            | `es/compatibility.mdx:88`; `es/configuration.mdx:167,191,216,234,254,267,283`; `es/getting-started.mdx:395,420,446,464,484,498,526,545,568` |
| `glpat-your-token` becomes `glpat-tu-token` | `es/install/docker.mdx:106`; `es/operations/http-server.mdx:405,421,434,635`                                                                |
| OAuth placeholders translated               | `es/operations/http-server.mdx:216,239,241`                                                                                                 |
| Tunnel and salt placeholders                | `es/operations/remote-deployment.mdx:590,591,768`                                                                                           |

---

## Facts the pages need and do not carry

Where a page hand-waves, this is the fact and where it comes from, so whoever
writes it does not research it twice.

| Where                                                                   | The fact that is missing                                                                                                                                                                                                             | Source                                                                       |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `docs/guides/remote-deployment.md:416-430`, the proxy requirement table | A reverse proxy must send a `Host` the server accepts, and the two guards that decide accept only the listen host, `localhost`, `127.0.0.1` and `::1`. The SDK's guard is `DisableLocalhostProtection` and this server never sets it | `mcp/streamable.go:325-331` of go-sdk v1.7.0; `cmd/server/main.go:3441,3822` |
| `docs/guides/telemetry.md`, the whole page                              | A minimal collector: an `otlp` receiver on 4317 and 4318, a `batch` processor, a `debug` exporter at `verbosity: detailed`, and the three pipelines. Fifteen lines, executed for this audit                                          | OpenTelemetry Collector configuration reference                              |
| `docs/guides/telemetry.md:106-155`                                      | Which collector authenticator each header form pairs with: `bearertokenauth` for the bearer, `basicauth` for basic, `oidc` for a vendor key                                                                                          | Collector `contrib` extension documentation                                  |
| `docs/guides/remote-deployment.md:465-502`                              | nginx drops headers containing underscores unless `underscores_in_headers on`. The page states this at line 522 for our own headers but not that a client's custom header would be dropped                                           | nginx `ngx_http_core_module` reference                                       |
| `docs/guides/remote-deployment.md:526-549`                              | Caddy's `flush_interval -1` is documented as disabling response buffering; what is not stated is that Caddy already sets `X-Forwarded-Host` and `X-Forwarded-Proto` but keeps the original `Host`, which is what triggers the `403`  | Caddy `reverse_proxy` directive reference                                    |
| `docs/guides/remote-deployment.md:619-651`                              | Cloudflare's 100-second proxy read timeout applies to a response that has not sent bytes; the server's 25-second SSE keep-alive stays under it, and that is why the keep-alive interval matters here                                 | Cloudflare proxy timeout documentation                                       |
| `docs/getting-started.md:216-219`                                       | "Waits for you to read it before closing" holds only when standard input is a terminal. With `/dev/null` on standard input the process logs `GITLAB_TOKEN is required` and exits, which is what a container start looks like         | Observed; `cmd/server/transport.go`                                          |
| `docs/reference/capabilities/subscriptions.md`                          | The MCP specification's own position on `resources/subscribe` under a stateless transport, which is what justifies refusing the legacy method while keeping the capability bit on                                                    | MCP specification 2026-07-28, resources and lifecycle                        |
| `docs/concepts/security.md`                                             | GitLab's dynamic client registration assigns the `mcp` scope, which is why `clientId` must be configured. The claim is in `docs/getting-started.md:482` with no citation                                                             | GitLab OAuth application documentation                                       |

---

## Code defects found

Each was reproduced against a build of `af6023c7`. These are the most valuable
output of the audit and none of them is a documentation change.

### D1: the host guards refuse every documented reverse proxy

| Side | Location                                                                                            |
| ---- | --------------------------------------------------------------------------------------------------- |
| Code | `cmd/server/main.go:3441` (`allowedHosts`), `cmd/server/main.go:3822` (`hostValidationMiddleware`)  |
| Code | `mcp/streamable.go:325-331` of go-sdk v1.7.0, reached because `DisableLocalhostProtection` is unset |
| Docs | `docs/guides/remote-deployment.md:490,528,598,624,833`; requirement table at `:416-430`             |

Reproduced with nginx: the configuration at `docs/guides/remote-deployment.md:465-502`
answers `403` on `/mcp` and on `/health`. The full matrix is in
[Path 5](#path-5-reverse-proxies-lan-and-public). Three candidate fixes, all in
code:

1. Add the `--public-url` host to `allowedHosts`. Smallest change, and it makes
   the documented OAuth deployment work. Does not help legacy mode, and does not
   touch the SDK guard.
2. Set `DisableLocalhostProtection` when `--public-url` or
   `--trusted-proxies` is configured, and add a `--trusted-hosts` flag whose
   value joins the allowed set. Covers every documented shape.
3. Leave the guards and rewrite `Host` in all five proxy examples. One line each,
   verified to work, but it discards the client's `Host` and pushes a security
   guard's workaround into every reader's configuration.

Whichever is chosen, `test/e2e/http/proxy_test.go` needs a case whose nginx
sends a non-loopback `Host`, because the present one cannot fail.

### D2: the tool-search flag ignores the environment that configures stdio

| Side | Location                                                                  |
| ---- | ------------------------------------------------------------------------- |
| Code | `cmd/server/main.go:372-386`                                              |
| Docs | `docs/reference/cli.md:32`; `site/src/content/docs/configuration.mdx:378` |

The dispatch reads `hcfg.toolSurface` and `resolveHTTPTier(&hcfg)`, which are the
HTTP flag values. It never calls `config.Load()` nor the HTTP environment
overlay, so `GITLAB_MCP_TOOL_SURFACE` and `GITLAB_MCP_TIER` are ignored:

```text
GITLAB_MCP_TOOL_SURFACE=individual --tool-search "list issues"  ->  No tools found
--tool-surface=individual          --tool-search "list issues"  ->  Found 100
GITLAB_MCP_TIER=ultimate --tool-surface=individual --tool-search epic  ->  Found 1
--tier=ultimate          --tool-surface=individual --tool-search epic  ->  Found 26
```

Since the environment variables are the stdio spelling and the flags are
documented under HTTP mode, a stdio operator searching their own surface always
searches the default dynamic surface, which has two tools.

### D3: two documented upper bounds are enforced only where the setting is inert

| Side | Location                                                                            |
| ---- | ----------------------------------------------------------------------------------- |
| Code | `internal/config/config.go:76-77` and `:758-766`; `internal/config/http_overlay.go` |
| Docs | `docs/reference/cli.md:74` and `:92`; `docs/reference/env.md:141` and `:152`        |

`GITLAB_MCP_MAX_HTTP_CLIENTS` sizes the HTTP pool and does nothing on stdio.
`GITLAB_MCP_RATE_LIMIT_RPS` has a documented ceiling of 1000. Both bounds live
in `config.Load()`, which only stdio calls:

```text
stdio, GITLAB_MCP_MAX_HTTP_CLIENTS=999999  ->  refused, "exceeds maximum of 10000"
HTTP,  GITLAB_MCP_MAX_HTTP_CLIENTS=999999  ->  starts, max_clients:999999
HTTP,  --max-http-clients=999999           ->  starts, max_clients:999999
stdio, GITLAB_MCP_RATE_LIMIT_RPS=99999     ->  refused, "exceeds maximum of 1000"
HTTP,  GITLAB_MCP_RATE_LIMIT_RPS=99999     ->  starts
```

The overlay does validate other settings the same way stdio does:
`GITLAB_MCP_SESSION_TIMEOUT=99h` is refused for exceeding 24h, and an
unparseable integer refuses startup. These two are the gap. Separately, neither
ceiling is documented for `--rate-limit-rps` at all, so `--rate-limit-rps=2000`
is refused on stdio by a bound no page states.

### D4: `--probe` reports a healthy server unhealthy when the target has a trailing slash

| Side | Location                        |
| ---- | ------------------------------- |
| Code | `cmd/server/probe.go:209`       |
| Docs | `docs/reference/cli.md:196-218` |

`parseProbeTarget` substitutes `/health` when the parsed path is empty, and
takes any other path literally. A URL ending in `/`, which is what copying a
base URL out of a browser or a configuration file gives, has path `"/"`:

```text
--probe http://127.0.0.1:8080/health  ->  exit 0, answered
--probe http://127.0.0.1:8080         ->  exit 0, answered
--probe http://127.0.0.1:8080/        ->  exit 1, "answered 405 Method Not Allowed"
```

The `405` is the MCP endpoint answering a `GET`, so the probe reports a
perfectly healthy server as unhealthy and an orchestrator restarts it. The fix
is `if path == "" || path == "/"`.

Two smaller things in the same area. `cli.md:216` says "each attempt is bounded
to three seconds" and omits `probeBudget` at `cmd/server/probe.go:45`, which
stops the whole run at four seconds however many peers there are; from the
second peer on, the stated bound over-promises. And `cli.md:273` documents exit
2 for "a target that is not a URL, a socket path or `host:port`", but
`parseProbeTarget` falls through to treating an unrecognised string as a socket
path, so `--probe 'ht!tp://x'` exits 1 with a `dial unix` error rather than 2.
Exit 2 is reachable only for an empty target or an unparseable `http://` URL.

### D5: the shipped image updates itself while the page says nothing does

| Side | Location                                                                 |
| ---- | ------------------------------------------------------------------------ |
| Docs | `docs/guides/installation.md:35`                                         |
| Fact | The published `:latest` image logs `autoupdate: starting periodic check` |

The page says, in bold, "**The server never updates itself.** There is no update
check and no in-place binary replacement on any channel… An earlier self-update
subsystem was removed". That is true of the current tree: no `autoupdate`
symbol survives under `cmd/` or `internal/`. It is false of every artifact a
reader can install today. Starting the published image logs:

```json
{"level":"INFO","msg":"autoupdate: starting periodic check","interval":3600000000000,"mode":"true","repository":"jmrplens/gitlab-mcp-server","current_version":"2.7.5"}
```

Same root cause as the hanging Docker quick start in
[Path 1](#path-1-quick-start): pages assert unreleased behaviour as present
tense. Both self-resolve at the 2.8.0 tag, and until then both are wrong for
every reader.

### D6: a safe-mode hint names the wrong spelling in HTTP mode

| Side | Location                             |
| ---- | ------------------------------------ |
| Code | The safe-mode preview's `hint` field |

A blocked write in an HTTP deployment started with `--safe-mode` answers
`"hint":"Set GITLAB_MCP_SAFE_MODE=false to execute this operation"`. The
operator set a flag, and the variable named would not change anything without a
restart with the flag removed. Cosmetic, but it is model-facing text.

### D7: `--oauth-cache-ttl` accepts a value every page says is out of range

| Side | Location                                                                  |
| ---- | ------------------------------------------------------------------------- |
| Code | `cmd/server/main.go:1158-1161` and `:3249-3256`                           |
| Docs | `docs/reference/cli.md:83`; `docs/reference/env.md:147`; four more places |

Six pages state the range as 1m to 2h. `validateOAuthCacheTTL` returns `nil` for
any non-positive value, and `oauthCacheTTL` then substitutes the default with a
warning. So `--oauth-cache-ttl=0` and `--oauth-cache-ttl=-5m` start and run at
15m, which no page describes, while `30s` is correctly refused. Either document
that non-positive means "the default", or refuse it.

### D8: `--action-timeout` is accepted and ignored on stdio

| Side | Location                                              |
| ---- | ----------------------------------------------------- |
| Code | `cmd/server/main.go:272` and `:429-431`               |
| Docs | `docs/reference/cli.md:76`, **corrected in the tree** |

The flag is parsed into the HTTP configuration struct, which stdio discards.
`gitlab-mcp-server --action-timeout=1s` with no `--http` starts normally and the
value has no effect, while `GITLAB_MCP_ACTION_TIMEOUT=99h` is refused for
exceeding the bound. Every flag in cli.md's HTTP table behaves this way, which
is correct by placement; this row was the one whose wording invited the mistake.
The documentation half is fixed. Whether the binary should warn when an
HTTP-only flag is passed to a stdio run is a code decision.

### D9: the SDK's plain-text 403 is the one refusal that is not JSON-RPC

| Side | Location                                 |
| ---- | ---------------------------------------- |
| Code | `mcp/streamable.go:330` of go-sdk v1.7.0 |

`hostValidationMiddleware` carries a comment explaining that a refusal must be
JSON-RPC, because "an unparseable 4xx body reads to a Streamable HTTP client as
a pre-negotiation server". The SDK's own localhost guard, which runs first,
answers `Forbidden: invalid Host header "mcp.example.com"` as `text/plain`. Any
fix for D1 that leaves the SDK guard reachable should also give it a JSON-RPC
body, by checking the same condition in our own middleware first.

---

## The plan, in execution order

Sizes are rough: **S** is under an hour, **M** is a few hours, **L** is a day or
more.

| #  | Step                                                                                                                                                                                                                                                      | Size |
| -- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| 1  | **Decide and fix D1.** Pick one of the three options, implement it, then rewrite the five proxy configurations to match, and add a `test/e2e/http` case whose nginx sends a non-loopback `Host` so the shape a deployment actually uses has a test         | M    |
| 2  | **Stop asserting unreleased behaviour.** Audit every page for a claim true only on `main` (D5, and the Docker quick start). Either restore the `--http=false` workaround with a "until 2.8.0" note, or state the version each claim starts holding in       | S    |
| 3  | **Fix D2, D3, D4, D7 and D9**, each its own commit, each with a test                                                                                                                                                                                       | M    |
| 4  | **Write the collector half of path 4.** The configuration in [Path 4](#path-4-http-with-telemetry-and-a-collector) plus a compose file that starts collector and server together, the three authentication forms mapped to collector extensions, and the identity-policy choice explained where an operator has to make it | M    |
| 5  | **Retire the per-token server model from the prose.** Eleven statements across five files, listed in [Contradictions](#contradictions) item 3. Keep every conclusion, replace every mechanism                                                                | S    |
| 6  | **Re-run `make bench-resources` and replace the stale tables** in `docs/concepts/resource-consumption.md`, removing the warning block that currently apologises for them                                                                                    | M    |
| 7  | **Give `docs/` a stdio page and a CLI-only first-call check.** One page that is the whole of path 2, and a three-line `tools/list` smoke test in `docs/getting-started.md` so a reader can prove the server works before configuring a client               | M    |
| 8  | **Single-source the two repeated tables.** Generate the environment and flag tables from the same source the site's `stats.json` comes from, and let every other page link rather than restate. Five copies of each become one plus four links               | L    |
| 9  | **Decide the `docs/` and site relationship and write it down.** Either the site is the user manual and `docs/` is contributor and reference material, or they are peers that cross-link. Whichever, every one of the 19 paired pages carries the banner naming its twin, not the current 10 | M    |
| 10 | **Move the six misfiled pages** and add the redirects each move needs (below)                                                                                                                                                                              | M    |
| 11 | **Split `docs/concepts/resource-consumption.md` three ways**: measurements into `docs/reference/resource-benchmark.md`, the narrative into `concepts/`, the sizing recipes into `guides/`                                                                   | M    |
| 12 | **Take icons out of `capabilities/`** and reconcile the "4 capabilities" count with the five files. Move `## Building a Custom MCP Client` into `guides/`                                                                                                   | S    |
| 13 | **Add a banner to the 26 pages without one**, and collapse the 29 audience wordings to four. Add `Prerequisites` where a page has any, rather than to all 118                                                                                              | M    |
| 14 | **Link the five section indexes from `README.md:356`**, so the 14 currently unreachable files are reachable from the front door. Repoint `docs/reference/configuration.md:117` at `docs/guides/ide-configuration.md`                                        | S    |
| 15 | **Reconcile the Spanish site**, using [The two languages](#the-two-languages) as the work list                                                                                                                                                              | M    |
| 16 | **Gate the executable blocks.** A harness that runs the 181 runnable `bash` blocks under `docs/` against a stand-in GitLab, in the spirit of `cmd/audit_doc_tool_names`. Start with the five paths' pages, which is about 60 blocks                          | L    |

### Redirects each move needs

The docs domain is path-preserving, so a move under `docs/` changes a URL that
is published. Steps 10 to 12 need these:

| From                                                   | To                                       |
| ------------------------------------------------------ | ---------------------------------------- |
| `docs/concepts/meta-tools.md`                          | `docs/reference/meta-tools.md`           |
| `docs/guides/ide-configuration.md`                     | `docs/reference/ide-configuration.md`    |
| `docs/reference/output-format.md`                      | `docs/concepts/output-format.md`         |
| `docs/reference/capabilities/icons.md`                 | `docs/reference/icons.md`                |
| `docs/concepts/resource-consumption.md`                | Split; keep a stub pointing at all three |
| `docs/guides/examples/generate-release-notes-skill.md` | `docs/development/` or the skills tree   |

`docs/concepts/meta-tools.md` has the widest inbound link surface, including
`CLAUDE.md` and `README.md`, so it is the one whose redirect matters most.

### What is deliberately not in the plan

- **Reorganising `docs/` for findability.** It is already correct: zero orphans,
  everything within three clicks.
- **Fixing the Spanish placeholder translations.** Cosmetic, and another branch
  is in those files.
- **Merging `docs/` into the site or the reverse.** Step 9 asks for a decision,
  not a merge; either answer is defensible and the audit does not have standing
  to pick.

---

## What was corrected in the tree

Six commits, each an unambiguous local correction verified against the binary.
Nothing structural was touched.

| Commit                                                                  | What                                                                                                                          |
| ----------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `docs(concepts): correct the binary size…`                              | `docs/concepts/resource-consumption.md:17`, `~49 MB` to `~55 MB`, measured at 54.8 MB stripped                                |
| `docs(http): show the id the 401 body actually carries`                 | `docs/guides/http-server-mode.md:378`, `"id":null` to `"id":1`, plus a sentence on what happens when the request carries none |
| `docs(http): print the log lines the server really writes`              | The sample log block: nanosecond durations, `tier` and `tier_source` rather than `enterprise_source`, and the elision stated  |
| `docs(http): say that the GET and DELETE exemption is legacy mode only` | Two unqualified statements that are false under `--auth-mode=oauth`, where an unauthenticated `GET` answers `401`             |
| `docs(reference): stop saying HTTP mode ignores the environment`        | `docs/reference/configuration.md` twice, and the `--action-timeout` row of `docs/reference/cli.md` (D8)                       |
| `docs(compat): give the meta surface the count the binary registers`    | `docs/guides/client-compatibility.md:100`, `~33 to 50` to `32 to 50`                                                          |

Every one of these was a single sentence or a single value, contradicted by a
running binary, with an unambiguous correct answer. Everything where the right
answer is a design decision stayed in the plan.

---

## How this was checked

Reproducible in about twenty minutes on a machine with Go, Docker, nginx and
`curl`.

1. **A stand-in GitLab.** A 90-line Go program answering `/api/v4/version`,
   `/api/v4/user`, `/api/v4/personal_access_tokens/self`, `/api/v4/license`,
   `/api/v4/projects` and `/api/graphql`, modelled on `startFakeGitLab` in
   `test/e2e/http/harness_test.go`. Enough for startup, scope detection, tier
   detection and a real tool call.
2. **A stdio driver.** A short Python script that spawns the binary, writes
   JSON-RPC lines to its standard input, reads responses off standard output and
   keeps standard error separate, so "stdout carries nothing but JSON-RPC" is
   observable rather than assumed.
3. **Fourteen HTTP servers**, one per configuration under test: single instance,
   several instances, `--allow-any-gitlab-url`, unix socket, TLS on the
   listener, OAuth with a path-prefixed `--public-url`, `--read-only`,
   `--safe-mode`, `--drain-delay`, `--telemetry`, wildcard and specific and
   non-loopback bind addresses.
4. **A real nginx**, with the documented public configuration copied verbatim,
   a plain-HTTP LAN configuration, a deliberately wrong one, and the corrected
   one, on ports of their own so the host's own nginx was untouched.
5. **A real collector**, `otel/opentelemetry-collector-contrib`, with a `debug`
   exporter, receiving all three signals from the server.
6. **The gates**, before and after: `go run ./cmd/audit_doc_tool_names/ --check`
   reports 1,120 registered tool names across 196 documentation files with no
   unregistered name, and `go run ./cmd/format_md_tables/ --check` reports the
   tables current.

Everything ran under `/tmp` in a directory of its own, and nothing was written
into the repository except the six commits above and this page.
