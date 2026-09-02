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

### The four traps in the standard variables

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

**`OTEL_EXPORTER_OTLP_INSECURE=true` overrides an `https` endpoint, for traces
and metrics.** The specification says the variable "only applies to OTLP/gRPC
when an endpoint is provided without the http or https scheme". The Go trace and
metric exporters do not read it that way: their environment configuration
applies the scheme first and the `INSECURE` options after, so the later one
wins and an `https` endpoint is downgraded to plaintext, taking whatever
credential you set with it. The newer log exporters resolve the scheme first and
are correct, which is why one deployment can end up exporting two signals in the
clear and the third over TLS.

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://collector.example.com:4318
OTEL_EXPORTER_OTLP_INSECURE=true   # traces and metrics go out in plaintext anyway
```

The precedence runs the other way too: an `INSECURE=false` beside an `http`
endpoint upgrades traces and metrics to TLS. This server emulates the deviation
per signal rather than reading the scheme off the URL, so the plaintext warning
below names the signals that are actually affected. It does not refuse the
configuration: the endpoint is yours to choose.

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

This server does not refuse that configuration, because a collector on a trusted
private network reached over plaintext is a real deployment and the endpoint is
yours to choose. It does say so once at startup, naming the signals affected:

```text
level=WARN msg="a collector credential is configured against a plaintext endpoint
on another host; it crosses the network in the clear on every export" signals=[traces]
```

Loopback is exempt: a credential that never leaves the machine cannot be observed
on a network, so a sidecar collector raises nothing.

**The encoding matters and is the mistake this syntax invites.** The value is
W3C Baggage format, so it is percent-decoded:

- A space must be written `%20`. `Authorization=Bearer abc` is a malformed pair,
  dropped silently, and your collector sees an unauthenticated export.
- A literal percent must be written `%25`.
- A literal comma separates pairs. To carry one inside a value, write `%2C`.
- Base64 padding is safe. `=` separates a key from its value at the **first**
  occurrence only, so the trailing `==` of a basic credential survives.

This server never transforms that variable. Whatever you set is what your
collector receives, and it is never logged: a test asserts the credential never
appears in this server's own log output, including when an export fails.

It is read, once, and only in order to keep it out of that output. The SDK's own
header parser prints what it could not parse, and the log exporter hands the
*entire* variable to the error handler, so one malformed non-credential pair is
enough to print a perfectly well formed credential beside it. This server reads
the same variables the exporters do at startup, and redacts those values, and
their percent-decoded spellings, out of every SDK diagnostic it emits. The
variable name is left in place, because an operator whose configuration is
malformed needs to know which one to fix.

## What is recorded

### Spans

One span per MCP request, plus one child span per GitLab API call it caused.

| Attribute                   | On                       | Meaning                                                                                                        |
| --------------------------- | ------------------------ | -------------------------------------------------------------------------------------------------------------- |
| `mcp.method.name`           | every span               | `tools/call`, `resources/read`, `prompts/get`, …                                                               |
| `gen_ai.tool.name`          | tool calls               | the tool the client named                                                                                      |
| `gitlab_mcp.action`         | tool calls               | the canonical catalog action, such as `issue.list`                                                             |
| `gitlab_mcp.tool_surface`   | every span               | `dynamic`, `meta` or `individual`                                                                              |
| `network.transport`         | every span               | `pipe` for stdio, `tcp` for HTTP                                                                               |
| `gen_ai.prompt.name`        | `prompts/get`            | the prompt the client named                                                                                    |
| `mcp.protocol.version`      | when it is known         | the MCP revision the request is speaking                                                                       |
| `mcp.session.id`            | stateful sessions only   | absent under the default stateless HTTP transport                                                              |
| `error.type`                | failures only            | a JSON-RPC code, or `tool_error`                                                                               |
| `rpc.response.status_code`  | when a code was returned | the JSON-RPC code                                                                                              |
| `gitlab_mcp.domain`         | tool calls               | the catalog domain, such as `issue`; bounded, so it survives on metrics where the action id is too many values |
| `gitlab_mcp.refusal_reason` | refusals                 | why the call was declined without reaching GitLab                                                              |

`gitlab_mcp.action` is the one worth understanding. On the default dynamic
surface every tool call names the same two tools, so `gen_ai.tool.name` is
`gitlab_execute_action` whether the server listed issues or deleted a branch.
The action attribute is the only thing that tells them apart.

### Metrics

| Instrument                      | Unit | What it answers                                                   |
| ------------------------------- | ---- | ----------------------------------------------------------------- |
| `mcp.server.operation.duration` | `s`  | how long requests take, by method and action                      |
| `mcp.client.operation.duration` | `s`  | how long this server waits on the client (elicitation, sampling)  |
| `http.client.request.duration`  | `s`  | how long GitLab takes                                             |
| `mcp.server.session.duration`   | `s`  | how long a client stays connected (stdio and `--stateless=false`) |

`mcp.server.session.duration` is deliberately not recorded under the default
stateless HTTP transport, where each POST is its own session: the histogram
would be a copy of `mcp.server.operation.duration` under a name promising
something else.

One attribute the convention asks for is missing, and it cannot be supplied:
`jsonrpc.request.id`. The Go SDK gives a receiving middleware no access to the
JSON-RPC id of the message it is handling, so there is nothing to record. It is
[registered upstream](../development/upstream-bugs.md).

Seconds, not milliseconds. The convention fixes that, and it is the opposite of
the `duration` field in this server's log records.

Two of those dimensions carry a name the caller chose, `gen_ai.tool.name` and
`gen_ai.prompt.name`. On a metric they are bounded: a call that named something
this server does not have is recorded as `_OTHER` rather than under the name it
sent, so the number of stored time series follows what is registered rather than
what a client types. The span keeps the name verbatim, which is where you look
when a client reports that its calls are failing.

A refusal is not a malfunction, which is why it has an attribute of its own:
`error.type` cannot tell a call this server declined from one that broke. The
call arrived well formed and this server chose not to run it:

| `gitlab_mcp.refusal_reason` | Why                                           |
| --------------------------- | --------------------------------------------- |
| `safe_mode`                 | safe mode answered a write with a preview     |
| `needs_confirmation`        | a destructive action was not confirmed        |
| `invalid_params`            | the parameters do not fit the action          |
| `unknown_action`            | no action by that name exists on this surface |
| `rate_limited`              | the deployment's own limit rejected the call  |

Those five are the whole set, which is what makes the attribute affordable as a
metric dimension as well as a span attribute: a deployment refusing every third
call looks exactly like a healthy one on a duration histogram alone.

Some of them carry `error.type` as well, and that is not a contradiction. A
refusal the caller can fix is answered with an error result, so the model is
told plainly that its call failed; safe mode answers with a preview instead,
which is a successful result and sets no `error.type`. Group by the refusal
reason rather than by `error.type` when you want the declined calls.

`rate_limited` is the one worth alerting on, because it is the only refusal that
is not the caller's doing. Its log line is throttled to one every ten seconds,
so a client in a retry loop cannot flood the terminal; the metric counts them
all.

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

The identity policy governs these records as well, and the two legs differ on
purpose. Your terminal keeps `user` and `user_id` whatever the policy says: you
are reading your own server's output, and a setting about what leaves the
process has no business editing that. The exported copy carries what the policy
allows and under the same `user.*` names the spans use, so one query joins them.

That last paragraph was untrue until recently, and a live deployment is what
showed it: running on `pseudonymous`, which names nobody, its spans carried a
digest and its logs carried the username in the clear. The policy had been
applied where it was written and never to the log bridge.

By default the digest is keyed with a secret generated at startup and written
nowhere, so it identifies a caller within one process and nowhere else. A
deployment running several replicas gives the same person a different digest on
each, at the same time, and a restart renumbers everybody. For a single
instance that is usually what you want: nothing to store, nothing to leak.

Two settings change it, and which one you reach for follows from how you run
the server.

| Setting                                  | What it does                                                                                                                   |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `GITLAB_MCP_TELEMETRY_IDENTITY_KEY`      | a secret every replica derives its keys from, so one caller carries one digest across the whole deployment and across restarts |
| `GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION` | how long a generated key lives, such as `24h`; empty keeps it for the life of the process                                      |

Set the key when you run more than one replica, or when a count of distinct
users has to survive a deploy. Rotation then does not apply: a key you supplied
is yours to rotate, on your schedule, and this server says so at startup rather
than rotating it underneath you.

Set the rotation interval when you run one instance and want the pseudonym to
stop correlating after a while. It is off by default because replicas start at
different moments, so they would rotate out of phase and a distinct-user count
would churn without anybody asking for it.

Nothing prescribes either answer. The OpenTelemetry registry defines `user.hash`
as a value "to correlate information for a user in anonymized form" and says
nothing about how long it should hold, and neither the specification nor ENISA
offers guidance on the lifetime of a pseudonymisation secret. What the field
shows is two coherent designs, and Matomo ships both at once: an
installation-wide salt that never rotates where a pseudonym must persist, and a
seed discarded every day where it must not.

### What setting the key means

Say it plainly, because it is a real change in what the export is. A stable
pseudonym is what the EDPB calls a person pseudonym, which "requires long-term
storage of the pseudonymisation secrets" and whose "risk of unauthorised
attribution is comparatively high". Under GDPR Article 4(5) that key is the
"additional information" that allows attribution, so it has to be kept
separately from the data it protects.

Concretely: GitLab user ids are small integers, so anyone holding both the key
and an export recovers the mapping by enumeration, in about two minutes on one
core. The key must not live where the telemetry lands. An environment variable
is where this server reads it, with the caveat every environment secret carries:
it is visible to anything that can read the process environment.

The key is expanded with HKDF-SHA256 into two independent keys, one for callers
and one for resource URIs, so the value you supply is never used as a key
itself and a digest of a user cannot be compared against a digest of a resource.

### What is never recorded

Not by default and not by any setting, because there is no setting:

- Tool arguments and tool results. The conventions define `Opt-In` attributes
  for both, and this server declines them: they carry project paths, issue
  bodies and search queries.
- Resource contents. The URI that named the resource follows the identity
  policy: a keyed per-process digest by default, and the URI itself only under
  `full`, which already exports the caller's real name.
- Your GitLab token, or any header a client sent.
- GitLab response bodies, and GitLab error messages. A failure records a
  classification such as `-32603`, never the text, which can name private paths.
- Full URLs of GitLab calls. The child span records the method, the host and the
  status, and the parent span already names the action, which identifies the
  endpoint family more legibly than a URL would.
- `token_suffix`, the last four characters of an HTTP client's credential. It
  keeps being written to stderr, where it is what an operator correlates a
  refusal by on their own terminal, and it is stripped from the exported copy of
  every log record at any depth: it authenticates nothing and correlates
  everything, which is not a distinction a telemetry backend can make for you.

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

The policy governs one more thing than its name suggests: **which resource a
request named**. A resource URI here embeds project and group ids, so it says
what a caller is working on, which is the same class of disclosure as saying who
they are. Under `none` and `pseudonymous` a span carries
`gitlab_mcp.resource.ref`, a keyed per-process digest that correlates reads and
polls of one resource without naming it. Under `full` it carries
`mcp.resource.uri`, the convention's own attribute, with the URI in it.

One flag rather than two, because a deployment that recorded project paths while
claiming to record nobody would be the predictable result of letting the two
settings drift apart.

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
