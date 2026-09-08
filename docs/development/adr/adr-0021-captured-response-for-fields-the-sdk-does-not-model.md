# ADR-0021: A field client-go does not model is read from the captured response

## Status

Accepted, 2026-09-08.

## Context

The 1:1 policy ([issue 392](https://github.com/jmrplens/gitlab-mcp-server/issues/392)) leaves no room for a field GitLab sends and this server drops. The sent dimension of the R-PATH audit (`shapes.typed.unsurfaced`, [PR 622](https://github.com/jmrplens/gitlab-mcp-server/pull/622) and [PR 623](https://github.com/jmrplens/gitlab-mcp-server/pull/623)) lists such fields from GitLab's own OpenAPI document and Grape source, and its first worklist held thirty that client-go v2.64.0 does not model: a note's `imported`, `imported_from`, `commands_changes` and `suggestions`; a member's `locked`, `membership_state`, `override`, `two_factor_enabled`, `group_scim_identity`, `custom_attributes` and `avatar_path`; a key's `expires_at`, `last_used_at` and `usage_type`; a runner's `created_at`, `created_by` and `job_execution_status`; a pipeline's `archived`; a lint result's `jobs`; a label's `description_html`; a token's `granular`, `granular_scopes` and `last_used_ips`.

Every handler in this repository calls a client-go service method, which spells the route, encodes the options, drives the retries and decodes the answer into the SDK's struct. A field that struct does not carry is gone by the time the handler sees the answer, and the body was consumed decoding it.

The answer this repository had for that was a request of the handler's own: `internal/tools/invites` decodes the `queued_users` map client-go's `InvitesResult` does not model by building the POST with `client.GL().NewRequest`, spelling the route itself, and decoding into a struct that embeds the SDK's. Seventeen other packages do the same for one endpoint or another. It works, and it costs what it costs: the route is spelled twice, once in client-go and once here; the options are encoded here rather than by the method that knows them; a list loses the pagination the SDK method would have read; and the request inventory records the handler's spelling, so a divergence between the two spellings is invisible to the audit that reads it. The notes alone would need it in some fifty handlers across eight packages.

Contributing the fields to client-go is the right end state and is recorded per gap in `docs/development/upstream-bugs.md`; it is not an answer for this release, since an upstream change lands in a version this server bumps to later, and the surface has to be whole now.

## Decision

The SDK client's transport chain gains a `captureTransport` (`internal/gitlab/capture.go`), placed just outside the response ceiling. A handler that needs a field the SDK does not model wraps its context with `gitlabclient.WithResponseCapture`, passes that context to the SDK call as it always did, and then decodes the captured body into a type of its own:

```go
ctx, captured := gitlabclient.WithResponseCapture(ctx)
key, _, err := client.GL().Keys.GetKeyWithUser(input.KeyID, gl.WithContext(ctx))
if err != nil {
    return Output{}, toolutil.WrapErr("key_get", err)
}
var extra keyExtra
if err := captured.Decode(&extra); err != nil {
    return Output{}, toolutil.WrapErr("key_get", err)
}
```

The transport reads the body once, records it under the capture the request travelled with, and hands the SDK a reader over the same bytes. Nothing about the request changes: the route, the options, the retries and the pagination stay client-go's, and the request inventory records exactly what it recorded before. A request made without a capture pays nothing. A retried request records each answer and keeps the last, which is the one the SDK decoded. An answer over the client's ceiling fails in the capture with the ceiling's own error, since the read passes through the limiter, exactly as the SDK's decoder would have failed.

A body that does not decode into the handler's type is an error the handler returns rather than swallows: the SDK decoded the same bytes, so the fault is in the type, and a test catches it.

The `invites` pattern is not retired. A request client-go has no method for at all is still a request of the handler's own; the capture is for the case where client-go has the method and lacks the field.

## Consequences

**Positive.**

- POS-001: A field client-go does not model reaches the surface in the handler that already makes the call, without a second request and without a second spelling of the route.
- POS-002: The request inventory, and every R-PATH check that reads it, keeps seeing the request client-go makes.
- POS-003: The mechanism is one transport and one type, tested on their own and through the real client, and every handler using it is exercised by the same `httptest` fixtures it already had, with the new field added to the fixture's JSON.

**Negative.**

- NEG-001: A captured body is held in memory twice for the life of the call, once in the capture and once in the SDK's decoder's buffer. Bounded by the response ceiling, and only for the calls that ask.
- NEG-002: The handler's extra type has to name the field the way GitLab spells it, and nothing checks that spelling against GitLab but the sent dimension of the audit, which is where the field came from.
- NEG-003: A field read this way is one more thing an upstream contribution retires, so each gap is recorded in `docs/development/upstream-bugs.md` with the handler that carries the workaround.
