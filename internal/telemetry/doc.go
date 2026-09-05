// Package telemetry is the only place in this server that knows about
// OpenTelemetry.
//
// Everything else reaches observability through the small API here, or through
// the OTel global providers this package installs. That is deliberate: an
// instrumentation library spread across 176 packages is one nobody can remove,
// reconfigure or reason about, and the seams that matter here are few enough to
// be listed. Nothing outside this package imports go.opentelemetry.io.
//
// # Off by default, and why
//
// Telemetry stays off unless an operator turns it on, for privacy rather than
// for cost. Instrumenting a deployment is the operator's decision to make about
// their own users, and a server that traced by default would be making it for
// them. PRIVACY.md's promise is unaffected either way: it is scoped to what the
// maintainer receives, and an exporter an operator points at their own
// collector sends nothing to anyone else.
//
// When it is on, the server says so. [Snapshot] feeds the `observability` block
// of the server card, so somebody connecting to a published endpoint can see
// that their calls are instrumented rather than having to ask.
//
// # Configuration
//
// One switch is ours and the rest is the specification's. `--telemetry` (or
// GITLAB_MCP_TELEMETRY) turns it on; endpoint, headers, timeouts, sampling and
// resource attributes come from the standard OTEL_* environment variables the
// exporters read themselves. Reinventing that surface would mean maintaining a
// second, worse copy of a configuration an operator already knows.
//
// The name is deliberate in both halves. It is not in the OTEL_ namespace:
// nothing forbids that (the OTEL_{LANGUAGE}_{FEATURE} convention carries no RFC
// 2119 keyword and addresses SDKs), but the namespace belongs to the
// specification and to the language SDKs, it is actively occupied
// (OTEL_GO_X_RESOURCE, OTEL_GO_X_OBSERVABILITY, OTEL_GO_X_CARDINALITY_LIMIT), a
// future release could claim a plain name like OTEL_ENABLED and change its
// meaning underneath us, and an operator seeing an OTEL_ prefix will reasonably
// assume the SDK is what reads it. And it carries the GITLAB_MCP_ prefix so it
// cannot collide with whatever else a host has exported; a bare TELEMETRY or
// OBSERVABILITY is the kind of name two programs on one machine will both want.
// Once shipped, the name and its false default cannot move without a major
// version, so it is decided here or not at all.
//
// The standard OTEL_* variables keep their names and must never be given a
// prefix of ours. They are not read by this code at all: the exporters read
// them. Shadowing them under GITLAB_MCP_ would mean passing the value as a
// programmatic option, which in Go is applied after the environment and so
// silently kills the variable it was meant to mirror, and it would break the
// ordinary case of a host that exports OTEL_EXPORTER_OTLP_ENDPOINT once for
// every service running on it.
//
// OTEL_SDK_DISABLED is honored as a veto on top, which is a different thing
// from an off switch; [SDKDisabledByEnv] says why the two cannot be collapsed.
//
// # What an attribute may carry
//
// The same discipline the logs already keep, stated here because a span makes
// it easy to break: record what was called and how it ended, never what was
// passed. Tool names, action ids, outcome, duration and the identity already in
// the log line are in; parameters, queries, tokens and response bodies are out.
// The existing code is the precedent: the dynamic surface logs `query_len` and
// not the query, and the pool logs a token suffix and not the token.
package telemetry
