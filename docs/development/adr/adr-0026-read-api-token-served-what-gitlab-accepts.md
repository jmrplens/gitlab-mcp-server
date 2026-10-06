---
title: "ADR-0026: A read_api token is served what GitLab accepts from it, derived per action"
status: "Accepted"
date: "2026-10-06"
authors: "jmrplens"
tags: ["authorization", "scopes", "read_api", "catalog", "decision"]
supersedes: ""
superseded_by: ""
---

# ADR-0026: A read_api token is served what GitLab accepts from it, derived per action

## Status

Accepted, 2026-10-06. It refines
[ADR-0018](adr-0018-authorization-admits-per-action-gating.md), whose admission rule it keeps
and whose narrowing it replaces: a classic or OAuth token carrying `read_api` and not `api` used
to be served the actions the catalog classifies as read-only, and is now served the actions
whose requests GitLab accepts from `read_api`. The question that opened it was a smaller one,
whether each action's page of the tool reference could say which token it needs beside its
tier, and answering it for a classic token is what showed the narrowing was keyed on the wrong
fact.

## Context

ADR-0018 admits a token at the minimum scope and narrows what it is served per action. For a
token that cannot write, the narrowing set `ServerConfig.ReadOnly`, the operator's own switch,
so the surface such a token was served was the read-only catalog: every action whose
`ActionSpec` says it does not write.

That classification answers a different question from the one GitLab asks. It says whether an
action changes anything, GitLab or the machine the server runs on. GitLab asks which scope the
request needs, and answers it by the request:

- REST: `read_api` for a GET or a HEAD and `api` for anything else (`lib/api/api.rb:60-61` at
  `v19.4.1-ee`), except a route whose class grants another set, such as `read_api` on every
  method of `POST /markdown` (`lib/api/markdown.rb:7-8`) and of the Orbit data routes
  (`ee/lib/api/orbit/data.rb:24-25`), or every scope on `DELETE /personal_access_tokens/self`
  (`lib/api/personal_access_tokens/self_information.rb:16`). A route GitLab authenticates by
  a credential the caller passes as a parameter, a runner's token or a trigger's, never reads
  the token this server holds: nothing on it asks for the user, which is what validates that
  token (`lib/api/helpers.rb:96`).
- GraphQL: `read_api` for a query and `api` for a mutation, with two query fields GitLab
  answers only from `api` (`Issue.createNoteEmail` and `WorkItem.createNoteEmail`).

`docs/development/action-requests.json` already records every request each of the 1098 actions
sends, so the two answers could be compared. They disagreed on seven actions:

- `template.lint` and `project.dependency_firewall_evaluate` are reads sent as POSTs. They were
  listed to a `read_api` token and refused by GitLab with a 403: `template.lint` with
  `insufficient_scope`, measured on 19.4.1 CE and EE, and the dependency firewall evaluation,
  an Enterprise route, measured on 19.4.1-ee in each state of its flag and setting.
- `package.download` sends GETs only and writes a file on the server's machine, so it was
  withheld from a token GitLab serves it to.
- `access.token_personal_revoke_self` is a DELETE every scope may send.
- `pipeline.trigger_run`, `runner.register` and `runner.delete_by_token` are writes GitLab
  authenticates by the trigger, registration or runner token the caller passes.

Four more agreed by coincidence: `orbit.query`, `repository.markdown_render` and
`runner.verify` POST and are served, and `repository.archive` sends nothing.

The answer to a withheld action had the same flaw at a smaller scale. The dynamic surface told
a narrowed session that GitLab requires `api` for every action a scope filter removed, so a
token holding `api` and not `admin_mode` was told to reauthorize with the scope it already had.

## Decision

**What a classic or OAuth token needs for an action is derived from the requests the action
sends, and a token carrying `read_api` and not `api` is served exactly the actions it reaches.**

- `cmd/gen_action_grants` derives the classic scope beside the fine-grained requirement, from
  the same requests. An operation takes GitLab's general rule, or the value a declaration in
  `cmd/gen_action_grants/classic_declarations.go` gives its route with a category and a reason;
  a way takes the strongest of its mandatory operations; an action takes the weakest of its
  ways, counting the ways no fine-grained token takes. The value is one of `api`, `read_api`,
  `other-credential` (GitLab reads another credential and never this one) and `no-request`, an
  ordered `finegrained.ClassicScope`, written into `internal/tools/actiongrants/table_gen.go`
  and `docs/development/action-requests.json`. At 19.4.1 it is 573 `api`, 520 `read_api`, 4
  `other-credential` and 1 `no-request`, and `read_api` reaches 525 actions.
- The derivation is held to GitLab where the record can speak and to itself where it cannot.
  A credential-not-read declaration must match the route's skip reason in the live record, a
  route with such a skip must be declared, and a declaration that agrees with the general rule
  or that no action sends is a finding. Two gates join `make check-action-grants-derivation`:
  gate 4 fails an action whose ways need different scopes or whose optional request needs more
  than the action, and gate 5 fails an action whose read or write classification disagrees with
  whether `read_api` reaches it, unless `annotationDisagreements` declares why. R-GRANT holds
  the committed table to the record the same way (`classicOperations` in
  `cmd/audit_1to1/internal/grants`).
- `ServerConfig.ReadOnly` is the operator's switch alone. `gitlabclient.NarrowToTokenScope`
  sets `ServerConfig.ReadAPIOnly` for a token whose scopes are known, classic, and carry
  `read_api` without `api`, and `Catalog.FilterReachableWith` keeps the actions whose classic
  scope it reaches. `tools.FilterActionCatalog` runs exclusions, the group scope filter, the
  operator's read-only step and then this one, so an action both would remove is reported as
  the operator's. A meta group stays read-only only when every action it keeps is. The
  standalone guided flows, registered outside the catalog, are removed by the same rule
  (`tools.StandaloneToolsBeyond`, applied by `toolvisibility.Apply`).
- A withheld action is answered with the scopes the credential lacks, read per action, and
  who asks for each: `api`, which GitLab requires for what the action sends, `admin_mode`,
  which this server demands before it serves one of the five administration groups, or both
  (`gitlabclient.MissingClassicScopes`, `actioncatalog.ScopeWithheld`). The two are worded
  apart because only the first is GitLab's rule: GitLab serves some actions of those groups,
  `admin.metadata_get` among them, to any authenticated token.
- The tool reference prints, on every action, the classic or OAuth line and the fine-grained
  line beside its tier, and the fine-grained permissions page sends a classic token there. The
  fine-grained words move into `cmd/internal/grantwords`, so both pages print one sentence. The
  classic line words the `admin_mode` of the administration groups as this server's
  requirement for listing the group, and names the routes GitLab refuses to an OAuth token,
  declared with their source lines in `cmd/gen_tool_reference`.
- The register row `AUT-001` records the narrowing and its two enforcing sites.

**What does not move.** Admission is ADR-0018's: `read_api` or `api`. A fine-grained token is
ADR-0024's: its grant decides, and its scope list narrows nothing. A token whose scopes are
unknown is served everything. `GITLAB_MCP_READ_ONLY`, safe mode and the confirmation a
destructive action asks for still key on the classification, so the five write-annotated
actions a `read_api` token now reaches are still writes to each of them, and
`cmd/audit_readonly_graphql` still guards the switch. A `read_api` token cannot reach a GraphQL
mutation by construction, since every mutation folds to `api`.

## Alternatives considered

- **Keep the classification as the narrowing.** Rejected: it listed two actions GitLab refuses
  and withheld five GitLab serves, and the tool reference would have printed a scope the server
  does not act on.
- **Record each route's scopes in the live GitLab record (schema 5 of `gitlab-api-live.json`)
  instead of declaring the departures.** Not taken in this change. It would make the `read_api`
  grants record-checked rather than declared, but it changes the oracle every reader of the
  record shares and needs a Docker re-record of the 19.4.1-ee image. The declaration table is
  complete as far as a sweep of the 19.4.1 source for every scope grant on a route that is not
  a GET, and the record already corroborates the other-credential class through the routes'
  skip reasons. This remains the way to close NEG-002.
- **Serve a `read_api` token the intersection of the two, read-only and reachable.** Rejected:
  it keeps the promise that such a token never writes, which `GITLAB_MCP_READ_ONLY` already
  makes for a deployment that wants it, and hides from every other deployment five actions
  GitLab serves the token.

## Consequences

### Positive

- POS-001: What a `read_api` token is listed is what GitLab answers it. The two actions it was
  listed and refused are withheld with a sentence naming `api`, and the five it was denied are
  listed.
- POS-002: A withheld action names the scope the credential lacks. A token holding `api` and
  not `admin_mode` is told about `admin_mode`, and one lacking both is told both.
- POS-003: Every action's reference entry says what a classic, OAuth or fine-grained token
  needs for it, generated from the same table the server enforces.
- POS-004: A new action whose classification and reach disagree fails gate 5 until its
  departure is declared with a reason, so the two facts cannot drift apart in silence.

### Negative

- NEG-001: A `read_api` token no longer means a read-only session. It is served
  `package.download`, which writes a local file, `access.token_personal_revoke_self`, which
  revokes the token itself when it is a personal access token (GitLab refuses the route to an
  OAuth token), and three writes GitLab authenticates by another credential. A
  deployment that wants no writes at all sets `GITLAB_MCP_READ_ONLY`, and the documentation says
  so instead of the earlier "exactly as if `GITLAB_MCP_READ_ONLY` were set".
- NEG-002: The `read_api` grants on non-GET routes are declared, not recorded. A new route of
  ours on which GitLab grants `read_api` would be withheld from a token GitLab serves, which is
  the less visible of the two errors, and nothing committed would catch it.
- NEG-003: A deployment in read-only mode keeps recommending `read_api` in its challenge
  (`oauth.RequiredScope`). A token issued on that advice also loses `template.lint` and
  `project.dependency_firewall_evaluate`, which the operator's switch alone would keep listed;
  the withheld answer names `api` for each.
- NEG-004: `orbit.query` on GitLab.com, the one instance that serves Orbit, was not measured
  with a token carrying `read_api` alone, and `runner.register` was not measured at all, since
  registration tokens are off by default on 19.x. Both rest on the 19.4.1 source. If GitLab
  refuses either, a `read_api` session meets GitLab's own 403 on that action.
- NEG-005: The tool annotations of a `read_api` session change with the five writes it is
  served. On the default dynamic surface `gitlab_execute_action` is annotated neither
  read-only nor non-destructive any more (`readOnlyHint` false, `destructiveHint` true), where
  the narrowing this decision replaces served it as read-only. On the meta surface the four
  groups that keep a write lose their read-only hint: `gitlab_package` (the download) and
  `gitlab_pipeline` (the trigger run), and `gitlab_access` (the self-revocation) and
  `gitlab_runner` (the registration and the deletion by token), which are annotated
  destructive as well. On the individual surface each tool keeps its own annotation. A client
  that approves read-only tools without asking therefore asks before every call of the
  dynamic surface in such a session; a deployment that wants the read-only annotations back
  sets `GITLAB_MCP_READ_ONLY`.
- NEG-006: The classic line names `admin_mode` only for the five groups this server filters
  by it. GitLab asks `admin_mode` of every route only an administrator may call once an
  instance turns Admin Mode on, wherever the action sits (`ci_variable.instance_list` and
  `user.create` among them), and the record holds no route's administrator check, so
  the reference says it once per page rather than on each such action.
