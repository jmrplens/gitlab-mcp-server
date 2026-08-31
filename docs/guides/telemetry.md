# OpenTelemetry

This server can export traces, metrics and logs over OTLP to a collector you
run. It is **off by default**, and this page is about turning it on, pointing it
somewhere, and knowing exactly what leaves the process.

Two sentences to set expectations before the detail:

- Telemetry goes to **your** collector. Nothing is ever sent to the maintainer
  of this project, and there is no address you have to opt out of.
- What is recorded describes **operations**, never their contents. The action
  called, the outcome, the duration. Not the project you named, not the issue
  body, not your search query, not your token.

## Turning it on

```bash
gitlab-mcp-server --telemetry
```

or, for a stdio client whose configuration is JSON rather than a command line:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "glpat-…",
        "GITLAB_MCP_TELEMETRY": "true",
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318"
      }
    }
  }
}
```

That is the whole surface this project owns. Everything else, the endpoint, the
credentials, the sampling, the batching, the resource attributes, comes from the
standard `OTEL_*` environment variables the OpenTelemetry exporters read
themselves. This is deliberate: reinventing that surface would mean maintaining
a second, worse copy of configuration you already know, and it would break the
ordinary case of a host that exports `OTEL_EXPORTER_OTLP_ENDPOINT` once for
every service running on it.

### The three traps in the standard variables

These catch people, they are not this server's doing, and each one fails
quietly.

**`http://` is not the default.** The Go exporters default to
`https://localhost:4318`, not `http`. Turning telemetry on with no endpoint set
attempts TLS against a plaintext local collector and every batch fails, for all
three signals. Write the scheme:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Writing bare `localhost:4318` also yields https.

**Durations are integer milliseconds, never Go durations.** The specification is
explicit: "Any value that represents a timeout MUST be an integer representing a
number of milliseconds." So `OTEL_EXPORTER_OTLP_TIMEOUT=30s` does not mean
thirty seconds. It parses as nothing, and the 10s default stands.

```bash
OTEL_EXPORTER_OTLP_TIMEOUT=30000   # thirty seconds
OTEL_EXPORTER_OTLP_TIMEOUT=30s     # nothing; the default is kept
```

Every `OTEL_BSP_*`, `OTEL_BLRP_*`, `OTEL_METRIC_EXPORT_INTERVAL` and
`OTEL_METRIC_EXPORT_TIMEOUT` behaves the same way. Note that every flag *this*
server defines takes a Go duration, which makes the inconsistency worse rather
than better. It is worth reading twice.

**`http/json` is refused, and the reason is narrower than it looks.** Since
v1.46.0 the Go trace exporter implements it; the metric and log exporters do
not. Honoring it would give one deployment two encodings at once, JSON spans
beside protobuf metrics, from a single setting that reads as though it selects
one thing. So it fails at startup, by name, rather than silently downgrading or
silently splitting.

## Authenticating to your collector

There is **no username and password option**, and none is missing: the
specification defines no such variable. Every scheme is a header.

| What your collector wants | What to set                                                                                              |
| ------------------------- | -------------------------------------------------------------------------------------------------------- |
| Nothing (the default)     | Set nothing. This is a supported configuration, not a fallback.                                          |
| A bearer token            | `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer%20YOUR_TOKEN`                                           |
| Basic auth                | `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic%20$(printf 'user:pass' \| base64)`                       |
| A vendor API key          | `OTEL_EXPORTER_OTLP_HEADERS=api-key=YOUR_KEY`                                                            |
| Several at once           | `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer%20tok,x-tenant=acme`                                    |
| Mutual TLS                | `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE=/path/cert.pem` and `OTEL_EXPORTER_OTLP_CLIENT_KEY=/path/key.pem` |
| A private CA              | `OTEL_EXPORTER_OTLP_CERTIFICATE=/path/ca.pem`                                                            |

**The encoding matters and is the mistake this syntax invites.** The value is
W3C Baggage format, so it is percent-decoded:

- A space must be written `%20`. `Authorization=Bearer abc` is a malformed pair,
  dropped silently, and your collector sees an unauthenticated export.
- A literal percent must be written `%25`.
- A literal comma separates pairs. To carry one inside a value, write `%2C`.
- Base64 padding is safe. `=` separates a key from its value at the **first**
  occurrence only, so the trailing `==` of a basic credential survives.

This server never reads, logs or transforms that variable. Whatever you set is
what your collector receives, and a test asserts the credential never appears in
this server's own log output, including when an export fails.

## What is recorded

### Spans

One span per MCP request, plus one child span per GitLab API call it caused.

| Attribute                  | On                       | Meaning                                            |
| -------------------------- | ------------------------ | -------------------------------------------------- |
| `mcp.method.name`          | every span               | `tools/call`, `resources/read`, `prompts/get`, …   |
| `gen_ai.tool.name`         | tool calls               | the tool the client named                          |
| `gitlab_mcp.action`        | tool calls               | the canonical catalog action, such as `issue.list` |
| `gitlab_mcp.tool_surface`  | every span               | `dynamic`, `meta` or `individual`                  |
| `network.transport`        | every span               | `pipe` for stdio, `tcp` for HTTP                   |
| `error.type`               | failures only            | a JSON-RPC code, or `tool_error`                   |
| `rpc.response.status_code` | when a code was returned | the JSON-RPC code                                  |

`gitlab_mcp.action` is the one worth understanding. On the default dynamic
surface every tool call names the same two tools, so `gen_ai.tool.name` is
`gitlab_execute_action` whether the server listed issues or deleted a branch.
The action attribute is the only thing that tells them apart.

### Metrics

| Instrument                      | Unit | What it answers                                                  |
| ------------------------------- | ---- | ---------------------------------------------------------------- |
| `mcp.server.operation.duration` | `s`  | how long requests take, by method and action                     |
| `mcp.client.operation.duration` | `s`  | how long this server waits on the client (elicitation, sampling) |
| `http.client.request.duration`  | `s`  | how long GitLab takes                                            |

Seconds, not milliseconds. The convention fixes that, and it is the opposite of
the `duration` field in this server's log records.

### Logs

The third signal is this server's own structured records: the same lines it
writes to stderr, exported from `INFO` upward. `LOG_LEVEL` still governs the
terminal; the export floor is separate, so running at `debug` does not send a
record per GitLab round trip to your collector on top of the span that already
describes it.

Every record written while serving a request carries the trace and span id of
that request, so a backend can jump from a slow span straight to the lines
written inside it. Startup, shutdown and background records carry none, because
there is no span to belong to.

### What is never recorded

Not by default and not by any setting, because there is no setting:

- Tool arguments and tool results. The conventions define `Opt-In` attributes
  for both, and this server declines them: they carry project paths, issue
  bodies and search queries.
- Resource contents, and resource URIs, which embed project and group ids.
- Your GitLab token, or any header a client sent.
- GitLab response bodies, and GitLab error messages. A failure records a
  classification such as `-32603`, never the text, which can name private paths.
- Full URLs of GitLab calls. The child span records the method, the host and the
  status, and the parent span already names the action, which identifies the
  endpoint family more legibly than a URL would.

An end-to-end test drives real traffic carrying a distinctive project path, a
search query and a token, then searches every exported payload for all three.

## Recording who made a call

Off by default, and configurable in three steps:

```bash
--telemetry-identity none          # the default: nothing about the caller
--telemetry-identity pseudonymous  # a per-process digest, correlatable, not readable
--telemetry-identity full          # user.id and user.name
```

or `GITLAB_MCP_TELEMETRY_IDENTITY`.

**`none`** is the default because whether to record a person's identity is your
decision about your own users, and a default that decides for you is wrong
whichever way it points. The conventions agree: these attributes are `Opt-In`,
whose rule is "Instrumentations SHOULD populate the attribute if and only if the
user configures the instrumentation to do so".

**`pseudonymous`** emits `user.hash`, an HMAC-SHA256 digest under a 32-byte key
generated at startup and never written anywhere. It gives you the one thing a
shared endpoint genuinely needs, telling one caller's traffic from another's, so
a burst can be attributed and a session followed, without naming anybody.

Two properties to understand before relying on it. The digest is **stable within
a process and different after a restart**, which is deliberate: a digest stable
across processes would let anyone holding two deployments' exports link the same
person across both. And pseudonymous is not anonymous: somebody who can
correlate a known person's activity with a digest can link the two. That is
inherent to the technique, not to this implementation.

**`full`** emits `user.id` and `user.name`. What an organization auditing its own
users wants, and what it is entitled to: those are its people and its collector.

None of these ever reach a metric, under any policy. A span carrying a user id
costs one span; a metric dimension carrying one is a time series per person,
growing without bound with the number of people using the deployment.

## Turning it off

Three ways, in order of scope:

```bash
# Do not turn it on. This is the default.
--telemetry=false

# Veto it from the environment, whatever the flag says.
OTEL_SDK_DISABLED=true
```

`OTEL_SDK_DISABLED` is the specification's own kill switch and this server
honors it, which nothing beneath us does: the string appears in no OpenTelemetry
Go module. It is a veto rather than a switch, because its specified default
means "enabled" while telemetry here is off until asked for. Setting it to
`true` forces no-op providers even when `--telemetry` was passed, which is what
lets you disable telemetry across a fleet without editing every unit file.

Its boolean grammar is the specification's, which is stricter than Go's: only
the case-insensitive string `true` disables. `1`, `t` and `yes` do not, and an
empty value counts as unset.

## Seeing whether it is on

The server card reports it:

```bash
curl -s http://localhost:8080/server-card | jq .telemetry
```

```json
{
  "enabled": true,
  "signals": ["traces", "metrics", "logs"],
  "protocol": "http/protobuf",
  "conventions": "OpenTelemetry, following the MCP semantic convention",
  "recorded": "the method called, the tool and catalog action, the outcome and the duration",
  "not_recorded": "tool arguments, tool results, resource contents, queries, tokens, and GitLab response bodies"
}
```

The block is absent when telemetry is off, rather than present and saying
`false`: you should not have to parse a negation to learn that nothing is being
recorded.

**The collector address is deliberately not published there.** It names your
infrastructure, and the card is served to anyone who asks for it.

## When it goes wrong

Telemetry failures never affect a request. A collector that is down, refusing
connections, or rejecting your credential produces log lines on stderr and
changes nothing else: the same status, the same body, the same latency for the
client. This is a decision the specification leaves open ("The API or SDK MAY
fail fast ... but MUST NOT cause the application to fail later at runtime"), and
the choice here is that a server which can talk to GitLab keeps doing so when it
cannot talk to a collector.

To see what the SDK thinks is wrong:

```bash
LOG_LEVEL=debug gitlab-mcp-server --telemetry 2>telemetry.log
```

Export failures, malformed header pairs, unparseable durations and protocol
warnings all arrive there as structured records.

### On stdio, never use a console exporter

`OTEL_LOGS_EXPORTER=console` and the stdout exporters write to **stdout**, which
on the stdio transport carries JSON-RPC and nothing else. One stray line ends
the session. This server does not ship a console exporter for that reason, and
an end-to-end test asserts stdout stays pure JSON-RPC while every export is
failing.

## Standards this follows

| Area                         | Source                                                                                                                |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Configuration variables      | [OTLP Exporter Configuration](https://opentelemetry.io/docs/specs/otel/protocol/exporter/)                            |
| Span and metric shape        | [MCP semantic conventions](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/mcp.md) |
| Attribute requirement levels | [Attribute Requirement Level](https://opentelemetry.io/docs/specs/semconv/general/attribute-requirement-level/)       |
| Error recording              | [Recording Errors](https://opentelemetry.io/docs/specs/semconv/general/recording-errors/)                             |
| Identity attributes          | [User attributes](https://opentelemetry.io/docs/specs/semconv/registry/attributes/user/)                              |
| Trace context in `_meta`     | [MCP specification](https://modelcontextprotocol.io/specification/)                                                   |

The MCP semantic convention is **Development** status, which its own maturity
table describes as "SHOULD NOT be used in production" and "MAY be removed
without prior notice". It is adopted anyway, on the grounds that it affects
operators rather than MCP clients: a convention change costs a dashboard edit
and costs consumers of this server nothing. It lives in a separate repository
and not on `opentelemetry.io`, whose `/docs/specs/semconv/mcp/` returns 404.
