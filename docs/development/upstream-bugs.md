# Upstream bugs and gaps

Defects and missing capabilities found in projects this server depends on, kept
here so they are contributed back rather than only worked around.

**This register is permanent.** An entry is never deleted. When a fix lands
upstream it is marked merged with the version that carries it, and the entry
stays as the record of why the workaround existed and when it could go.

An entry earns its place by being **found from this codebase**: a workaround we
carry, a behaviour a test had to accommodate, a spec clause we cannot satisfy
because the dependency does not expose what it needs. Each one records where the
evidence is, so a contributor does not have to rediscover it.

See the [upstream contribution skill](../../.github/skills/upstream-contribution/)
for the fork, branch, fix, test and MR workflow.

## Writing an entry

**Always link a tracker item, never write a bare reference.** This repository
lives on GitHub and is mirrored to GitLab, and each forge autolinks the other's
reference syntax against *itself*. A bare `!2996` renders on the mirror as a
merge request of the mirror, and a bare `#58` renders on GitHub as an issue of
this repository. Both point at something that does not exist, or worse, at
something unrelated that does.

A markdown link is not reprocessed as a reference by either forge, so put the
project path in the link text and the full URL in the target:

```markdown
[gitlab-org/api/client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996)
[creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58)
```

That stays readable as a mention, resolves from either forge, and creates no
cross-reference anywhere.

## What does not belong here

Our own misconfiguration. An entry was opened here for the SDK keepalive that
pinged sessions serving protocol 2026-07-28, where `ping` is a removed method,
and it was wrong: `ServerOptions.KeepAlive` defaults to zero and the SDK only
starts a keepalive when it is set to something. This project set it to 30
seconds. The defect was real and is fixed, but it was ours, and filing it
upstream would have sent someone looking for a bug in code that was doing what
it was told.

The test to apply before adding an entry: would the behavior happen to a caller
who never configured it? If it takes a setting of ours to reach, it is a bug in
this repository whatever it looks like from inside the debugger.

## What each entry records

Every entry carries the same five facts, so the state of a contribution is
readable without opening the tracker:

| Field          | Meaning                                                                                                         |
| -------------- | --------------------------------------------------------------------------------------------------------------- |
| **Reported**   | Whether it has been raised upstream at all, with a link to the issue                                            |
| **In review**  | Whether an upstream change was opened for review, with a link. Historical: it stays yes after the change merges |
| **Merged**     | Whether it has landed, and **in which version**. Merged implies Reported and In review are both yes             |
| **Blocking**   | Whether it blocks this MCP server, or only costs us a workaround                                                |
| **Workaround** | Whether we carry one while waiting, where it lives, and what retires it                                         |

## Summary

| # | Project | Issue | Reported | In review | Merged | Blocking | Workaround |
| - | ------- | ----- | -------- | --------- | ------ | -------- | ---------- |
| 1 | gitlab-org/gitlab | [403 carries no `WWW-Authenticate`](#403-responses-carry-no-www-authenticate-header) | No | No | No | No | Yes |
| 2 | gitlab-org/gitlab | [No `resource_indicators_supported`](#no-resource_indicators_supported-in-authorization-server-metadata) | No | No | No | No | Yes |
| 3 | client-go | [Panic unmarshalling an issue](#panic-unmarshalling-an-issue-with-no-id) | Yes | Yes | **Yes, v2.59.1** | Was yes | Retired |
| 4 | client-go | [`UpdateIssueBoardList` cannot decode its own response](#updateissueboardlist-cannot-decode-a-successful-response) | Yes | Yes | **Yes, v3.0.0** | No | Retired |
| 5 | client-go | [`GetNamespace` breaks on a path lookup](#getnamespace-cannot-decode-a-path-based-lookup) | No | No | No | No | Yes |
| 6 | client-go | [`SetFeatureFlagOptions` lacks `omitempty`](#setfeatureflagoptions-fields-lack-omitempty) | No | No | No | No | Yes |
| 7 | client-go | [`ApplicationStatistics` assumes numeric JSON](#applicationstatistics-assumes-numeric-json) | No | No | No | No | Yes |
| 8 | go-sdk | [No SSE keep-alive option](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions) | No | No | No | No | Yes |
| 9 | go-sdk | [A malformed message ends the session](#a-malformed-message-ends-the-session-instead-of-answering--32700) | No | No | No | Was yes | Yes |
| 10 | go-sdk | [Cannot send `notifications/cancelled` for a listen stream](#application-code-cannot-send-notificationscancelled-for-a-listen-stream) | No | No | No | No | None possible |
| 11 | go-sdk | [Declared, not negotiated, version selects MRTR](#the-declared-protocol-version-not-the-negotiated-one-selects-mrtr) | No | No | No | No | None taken |
| 12 | go-sdk | [A cancelled call is still answered](#a-cancelled-incoming-call-is-still-answered) | No | No | No | No | Partial |
| 13 | go-sdk | [The cancellation reason is discarded](#the-cancellation-reason-is-discarded-before-any-handler-sees-it) | No | No | No | No | None possible |
| 14 | go-sdk | [`Mcp-Name` compared without decoding](#mcp-name-is-compared-without-decoding-the-base64-sentinel) | No | No | No | No | None taken |
| 15 | go-sdk | [Protocol version classified by string ordering](#the-protocol-version-is-classified-by-string-ordering) | No | No | No | No | None taken |
| 16 | go-selfupdate | [Deprecated `x/crypto/openpgp`](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp) | Yes | Yes, open | No | No | Retired |
| 17 | codex | [Non-integer `priority` breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call) | Yes | Yes, open | No | Was yes | Yes |
| 18 | go-sdk | [A receiving middleware cannot read the JSON-RPC id](#a-receiving-middleware-cannot-read-the-json-rpc-request-id) | No | No | No | No | None possible |
| 19 | client-go | [Security mutations discard GraphQL errors](#the-security-attribute-and-category-mutations-discard-graphql-errors) | No | No | No | No | Yes |
| 20 | client-go | [Dependency Firewall lacks `operation` and the enablement endpoint](#the-dependency-firewall-wrapper-is-missing-an-attribute-and-an-endpoint) | No | No | No | No | None |
| 21 | go-sdk | [A middleware cannot ask whether a request carries params](#a-middleware-cannot-ask-whether-a-request-carries-params) | No | No | No | No | Yes |
| 22 | client-go | [Enum constants lag the documented value sets](#enum-constants-lag-the-documented-value-sets) | No | No | No | No | Yes |
| 23 | go-sdk | [No per-session resource-updated delivery](#a-resource-update-cannot-be-delivered-to-one-session) | No | No | No | No | Yes |
| 24 | gitlab-org/gitlab | [Approvals page documents the POST's response under the GET](#the-merge-request-approvals-page-documents-the-deprecated-posts-response-under-the-get) | No | No | No | No | Yes |
| 25 | client-go | [`CreateProjectForkRelation` declares a response GitLab does not send](#createprojectforkrelation-declares-a-response-gitlab-does-not-send) | No | No | No | No | Yes |
| 26 | client-go | [The invitations wrapper is missing two parameters and a response field](#the-invitations-wrapper-is-missing-two-parameters-and-a-response-field) | No | No | No | No | Yes |
| 27 | client-go | [The achievements fragments select less than the schema offers](#the-achievements-fragments-select-less-than-the-schema-offers) | No | No | No | No | None possible |
| 28 | client-go | [The epics wrapper is missing two filters and twelve response fields](#the-epics-wrapper-is-missing-two-filters-and-twelve-response-fields) | No | No | No | Was yes | Yes |
| 29 | client-go | [The note and discussion structs miss what GitLab sends and declare what it does not](#the-note-and-discussion-structs-miss-what-gitlab-sends-and-declare-what-it-does-not) | No | No | No | No | Yes |
| 30 | client-go | [The member structs, options and services miss what GitLab sends, accepts and serves](#the-member-structs-options-and-services-miss-what-gitlab-sends-accepts-and-serves) | No | No | No | No | Partial |
| 31 | client-go | [Six response structs miss a field GitLab sends on every object](#six-response-structs-miss-a-field-gitlab-sends-on-every-object) | No | No | No | No | Yes |
| 32 | client-go | [No token struct carries the granular fields, and the impersonation and resource ones carry less still](#no-token-struct-carries-the-granular-fields-and-the-impersonation-and-resource-ones-carry-less-still) | No | No | No | No | Yes |
| 33 | client-go | [The four Sidekiq routes carry a leading slash](#the-four-sidekiq-routes-carry-a-leading-slash-and-send-a-double-slash) | No | No | No | No | None |
| 34 | client-go | [Response structs that miss a field GitLab sends unconditionally](#response-structs-that-miss-a-field-gitlab-sends-unconditionally) | Yes | Yes, open | No | No | Yes |

States verified against the upstream trackers on 2026-09-05, except entry 34,
whose merge requests were opened on 2026-09-09.

## GitLab (`gitlab-org/gitlab`)

### 403 responses carry no WWW-Authenticate header

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Our detection does not depend on the header, so this is a
  contribution rather than a fix we need.
- **Workaround**: yes. `internal/oauth.isInsufficientScope` reads the response
  body instead of the challenge. It is not a stopgap: the body is the only place
  the distinction appears, so it stays whatever upstream does.

**Where**: `lib/api/api_guard.rb`, around the `ForbiddenError` handling.

**What**: GitLab's own comment states it.
`# FIXME: ForbiddenError (inherited from Bearer::Forbidden of Rack::Oauth2)
does not include WWW-Authenticate header, which breaks the standard.`
RFC 6750 section 3 requires a protected resource that refuses a request for
insufficient scope to return `WWW-Authenticate: Bearer
error="insufficient_scope"`. GitLab returns the error in the JSON body only.

**Root cause**: in `rack-oauth2`, only the `Unauthorized` class builds the
challenge; `Forbidden` does not. Fixable in GitLab's own handler without
changing the gem.

**How we found it**: implementing insufficient-scope detection. The obvious
implementation, parsing `error="insufficient_scope"` out of the challenge, would
have compiled, passed a hand-written fake that emitted the header, and never
once fired against a real GitLab.

**Effort**: small. Observable API behaviour change, so it needs a maintainer
from the authentication area and a changelog entry.

### No resource_indicators_supported in authorization-server metadata

- **Reported**: no. It is a feature request rather than a defect.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, the specification's own alternative. `--oauth-client-uid`
  pins the OAuth applications whose tokens are admitted, compared against
  `application.uid`. Off by default, since enabling it refuses personal access
  tokens.

**Where**: `/.well-known/oauth-authorization-server`.

**What**: verified live on 2026-08-29 against gitlab.com, the document
advertises seventeen fields and not this one, so RFC 8707 resource indicators
are unavailable and a client cannot request an audience-restricted token.

**Consequence for us**: the MCP authorization specification's audience-binding
MUST cannot be met by its named mechanism. Recorded as
[ADR-0019](adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md).

**Effort**: large, and it is a product decision rather than a patch.

### The merge request approvals page documents the deprecated POST's response under the GET

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no, now that we read the generated document instead.
- **Workaround**: yes. The carve-outs in
  `cmd/audit_1to1/internal/structs/analyze.go` cite
  GitLab's generated OpenAPI document rather than the prose page, which is
  the only entry in that table that does. It retires when the page is corrected.

**Where**: `doc/api/merge_request_approvals.md`, the section for
`GET /projects/:id/merge_requests/:merge_request_iid/approvals`.

**What**: the example response printed under the GET carries `id`, `iid`,
`project_id`, `title`, `description`, `state`, `created_at`, `updated_at`,
`merge_status`, `approvals_required`, `approvals_left` and more. GitLab renders
that endpoint with `::API::Entities::MergeRequestApprovals`
(`lib/api/entities/merge_request_approvals.rb`), which exposes four fields:
`user_has_approved`, `user_can_approve`, `approved` and `approved_by`. The wider
body is the response of the `POST` at the same path, deprecated in GitLab 16.0.
GitLab's own generated OpenAPI document already separates the two.

**How we found it**: `gitlab_mr_approval_config` published all twenty-four
fields of `gl.MergeRequestApprovals` and every one of the twenty extra arrived
as a zero. The 1:1 audit was green throughout, because it compares our type
against the SDK type and the SDK type models the POST. It surfaced when
a record of GitLab's own gave the audit an oracle that speaks for it, and the
e2e suite had recorded the live CE observation months earlier without anyone
connecting it to the output type.

**Effort**: small. A documentation correction, though the deprecated POST's
example needs to stay reachable for callers still using it.

## GitLab client (`gitlab.com/gitlab-org/api/client-go`)

### Panic unmarshalling an issue with no id

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!3006](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3006).
- **Merged**: **yes**, on 2026-08-25 into `main`, shipped in **v2.59.1**.
- **Blocking**: it was. The panic took the process down rather than failing one
  call.
- **Workaround**: retired. The dependency is pinned at v2.62.0 and the local
  guard is gone.

Kept here as the record: this is what the round trip looks like when it works.

### UpdateIssueBoardList cannot decode a successful response

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996),
  targeting `release-client-3.0`.
- **Merged**: **yes**, on 2026-09-02 into `release-client-3.0`, and released in
  v3.0.0 on 2026-09-07.
- **Blocking**: no.
- **Workaround**: retired. The check the entry asked for was made against the
  release actually adopted rather than against the bump: v3.0.0 declares
  `UpdateIssueBoardList(gid any, board, list int64, opt *UpdateGroupIssueBoardListOptions, ...) (*BoardList, *Response, error)`,
  so `internal/tools/groupboards.UpdateGroupBoardList` calls the wrapper again
  and the `acceptedMissingMethods` entry in
  `cmd/audit_1to1/internal/actions/analyze.go` is gone with it.

**What**: the group-level wrapper declared `[]*BoardList`, while GitLab returns
the single updated list object, so the wrapper could never unmarshal a
successful response. The project-level equivalent already returned
`*BoardList`.

### GetNamespace cannot decode a path-based lookup

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/namespaces.Get` issues the request
  directly.

**What**: `GetNamespace` expects a single JSON object, but some GitLab versions
answer a path-based lookup with an array.

**Before reporting**: establish which GitLab versions return the array, so the
report names a reproduction rather than a symptom.

### SetFeatureFlagOptions fields lack omitempty

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/features.Set` builds the request body
  itself.

**What**: the option struct's fields carry no `omitempty`, so empty strings are
serialized and GitLab rejects the request with a "mutually exclusive" error.

**Effort**: small, struct tags plus a test. A good first contribution.

### ApplicationStatistics assumes numeric JSON

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/appstatistics.Get` decodes the response
  itself.

**What**: the struct uses `int64` fields, while some GitLab versions return the
counts as JSON strings, so decoding fails.

**Before reporting**: as with `GetNamespace`, pin down which versions send
strings.

### The security attribute and category mutations discard GraphQL errors

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. It is why the eight mutations stay on raw GraphQL, not a
  fix we need to ship.
- **Workaround**: yes. `internal/tools/securityattributes` and
  `internal/tools/securitycategories` issue the same mutations themselves and
  read the errors array through `toolutil.GraphQLTopLevelError`. It retires
  when the wrappers check that array, at which point the handlers can move onto
  them unchanged: the field selections already match exactly.

**Where**: `security_attributes.go` and `security_categories.go`, in all eight
of `CreateSecurityAttributes`, `UpdateSecurityAttribute`,
`DestroySecurityAttribute`, `ProjectUpdateSecurityAttribute`,
`BulkUpdateSecurityAttributes`, `CreateSecurityCategory`,
`UpdateSecurityCategory` and `DestroySecurityCategory`.

**What**: each one unmarshals into a struct that embeds `GenericGraphQLErrors`
and then never reads it. Only the mutation payload's own `errors` field is
checked. `GraphQL.Do` returns an error solely for a non-2xx status, and GitLab
answers a query-level failure with HTTP 200 and a top-level `errors` array, so
a refused mutation reaches the caller as a success: `DestroySecurityAttribute`
and `BulkUpdateSecurityAttributes` return a nil error, `CreateSecurityAttributes`
returns an empty slice with no error, and the update methods degrade to a bare
`ErrNotFound` that throws GitLab's message away.

**Root cause**: an omission rather than a design choice, and the same file set
shows what the fix looks like. `WorkItems.ListWorkItems` checks
`len(result.Errors) != 0` and returns `&GraphQLResponseError{...}`; these eight
need the same three lines.

**How we found it**: auditing whether the wrappers could replace this server's
raw mutations. The field selections match ours character for character, so the
migration looked mechanical until the error paths were compared.

**Effort**: small. Three lines per method plus a test each, and no signature
changes: every one of them already returns an `error`.

### The Dependency Firewall wrapper is missing an attribute and an endpoint

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none. `internal/tools/dependencyfirewall` sends what the
  options struct can carry and documents the omission.

**What**: two gaps against
[the Dependency Firewall API](https://docs.gitlab.com/api/dependency_firewall/).
`EvaluatePackageOptions` carries `Ecosystem`, `Name` and `Version`, but not the
documented optional `operation` attribute (`download` or `upload`, defaulting to
`download`), and nothing on the options struct can carry a body field the type
does not declare. Separately, `SecurityDependencyFirewallService` wraps only
`POST /projects/:id/dependency_firewall/evaluate`, while the same page documents
`GET /projects/:id/dependency_firewall/enablement`, which reports whether the
firewall is on for a project.

**Found from**: exposing the evaluate endpoint as an MCP action
(`project.dependency_firewall_evaluate`), where 1:1 fidelity means every option
field becomes an input field and there was one fewer than the API documents.

**Effort**: small for the attribute (one field plus a test). The enablement
endpoint is a new method with its own result type, and would let a tool answer
"is the firewall even on here" without inferring it from a 404.

### Enum constants lag the documented value sets

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The handlers forward the string a caller passes, so the
  schema enums offer the documented values whether or not a constant exists,
  and each such value is recorded in `acceptedEnumGaps` in
  `cmd/audit_1to1/internal/enums/exemptions.go` so the enum rule can tell a
  documented extra from an invented one. Each entry retires when the constant
  lands upstream: the rule then reports the exemption as stale.

**What**: four value types in `types.go` and `todos.go` declare fewer
constants than the GitLab API documents for the parameters they type, and one
parameter is typed with the wrong value type altogether.

- `EventTypeValue` lacks `approved`, which
  [the user contribution events](https://docs.gitlab.com/user/profile/contributions_calendar/#user-contribution-events)
  list among the action types the events API filters on.
- `EventTargetTypeValue` lacks `epic`, which the
  [events API](https://docs.gitlab.com/api/events/) lists as a `target_type`
  since GitLab 17.3.
- `TodoAction` lacks `unmergeable`, `merge_train_removed` and
  `member_access_requested`, all listed by the
  [to-do items API](https://docs.gitlab.com/api/todos/) as `action` filter
  values.
- `DeploymentStatusValue` lacks `blocked`, which the
  [deployments API](https://docs.gitlab.com/api/deployments/) lists as a
  `status` filter value (the list options type the filter as `*string`, so it
  is only the constant that is missing).
- `EditProjectOptions.CIRestrictPipelineCancellationRole` is typed
  `*AccessControlValue`, whose constants are the feature-visibility set
  (`disabled`, `private`, `enabled`, `public`), while the
  [projects API](https://docs.gitlab.com/api/projects/) documents the parameter
  as `developer`, `maintainer` or `no_one`. A caller using the constants sends
  a value GitLab rejects.

**Found from**: the 1:1 audit's enum rule (`make audit-1to1-enums`), which
holds every schema enum to the constants of the SDK type behind the field and
reported these as values offered that the SDK does not declare.

**Effort**: small. Six constants across the four types (one on
`EventTypeValue`, one on `EventTargetTypeValue`, three on `TodoAction`, one on
`DeploymentStatusValue`), and a dedicated value type for the cancellation role;
none of them changes a signature.

### CreateProjectForkRelation declares a response GitLab does not send

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/projects.CreateForkRelation` issues the
  `POST` directly and decodes into `gl.Project`. Retire it, and the
  `acceptedMissingMethods` entry in `cmd/audit_1to1/internal/actions/analyze.go`,
  once the wrapper returns what the endpoint answers with.

**What**: `CreateProjectForkRelation` decodes `POST /projects/:id/fork/:forked_from_id`
into `ProjectForkRelation`, a `{id, forked_to_project_id, forked_from_project_id,
created_at, updated_at}` struct. GitLab answers that endpoint with the downstream
project, which shares only `id` with that shape, so every other field decodes to
its zero value and `id` decodes to the project's rather than a relation's. The
call therefore succeeds and returns a struct that says nothing true.

**Evidence**: GitLab's generated OpenAPI document records the response of
that operation as a project (`_links`, `namespace`, `forked_from_project` and
the rest of the project body), and the CE end-to-end suite observed the same
against a live GitLab 19.3.

**Effort**: small. The method returns `*Project` and the `ProjectForkRelation`
type retires with it; it is a signature change, so the v3 line is where it goes.

### The invitations wrapper is missing two parameters and a response field

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `internal/tools/invites.postInvitation` issues the `POST`
  directly, with the body built from `gl.InvitesOptions` plus the two fields it
  does not carry, and decodes into a superset of `gl.InvitesResult`. Retire it,
  and the two `acceptedMissingMethods` entries in
  `cmd/audit_1to1/internal/actions/analyze.go`, once the wrapper carries all
  three.

**What**: `InvitesOptions` models `id`, `email`, `user_id`, `access_level` and
`expires_at`, while
[the invitations API](https://docs.gitlab.com/api/invitations/) documents
`invite_source` and `member_role_id` beside them, the second of which is how an
Ultimate instance assigns a custom role at invitation time. `InvitesResult`
models `status` and `message`, and the same page documents a `queued_users` map
on the response of an instance with member promotion management enabled, which
is the case where nobody was invited outright and the caller most needs to be
told.

**Effort**: small. Two fields on the options struct and one on the result; all
three are additive.

### The achievements fragments select less than the schema offers

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none possible. The selection is a private constant inside
  `achievements.go` and every method builds its document from it, so a caller
  cannot ask for a field the fragment omits; reaching the missing data means
  leaving the service and issuing raw GraphQL, which would duplicate the
  service rather than extend it.

**What**: `achievementFields` and `userAchievementFields` in `achievements.go`
select a strict subset of what the GraphQL schema declares for the two types,
and the `Achievement` and `UserAchievement` structs carry only what those
fragments ask for.

- `UserAchievement.awardMessageHtml` is never selected, so the rendered form of
  the award message is unavailable while the raw one is.
- `achievement.namespace`, `userAchievement.user`, `.awardedByUser` and
  `.revokedByUser` are each selected as `{ id }` alone, and the structs keep the
  numeric id. GitLab returns a `Namespace` and a `UserCore` there, so a caller
  that wants a name, a path or an avatar has to make a second request per id.

**Found from**: this server mirrors what the SDK exposes, one field for one
field, so `internal/tools/achievements` publishes `namespace_id`, `user_id`,
`awarded_by_user_id` and `revoked_by_user_id` where every other list tool in the
repository publishes a user object, and publishes no HTML award message at all.
The gap was measured against the pinned GitLab schema in
`internal/graphqlschema/gitlab-schema.graphql` (types `Achievement` and
`UserAchievement`).

**Effort**: small for the message (one line in the fragment plus a field on the
struct). Larger for the user objects, because widening them changes the shape of
a published struct: the ids would stay and a `*BasicUser` would join them, which
is the same accretion the SDK already makes elsewhere.

### The epics wrapper is missing two filters and twelve response fields

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: was yes. `with_labels_details` is a parameter this server
  publishes, and through the wrapper it could not be answered at all.
- **Workaround**: yes, two of them. The two filters are sent through the Work
  Items GraphQL query instead, which is why naming either one routes an epic
  list away from the REST endpoint (`usesWorkItemsPath` in
  `internal/tools/epics/epics.go`). The response fields come from a raw REST
  fetch into the `epicAPI` superset in the same file, the shape this repository
  already uses for `jobs`, `boards` and `groupepicboards`. Both retire when the
  wrapper carries them.

**What**: five gaps, four of them in `epics.go`, all of them fields GitLab
really sends or really accepts.

- `ListGroupEpicsOptions` declares no `AuthorUsername` and no `Confidential`.
  Both are listed as parameters of `GET /api/v4/groups/{id}/-/epics` in the
  OpenAPI document GitLab generates from its own Grape definitions; the prose page
  [doc/api/epics.md](https://docs.gitlab.com/api/epics/#list-all-group-epics)
  lists `author_username` in the list endpoint's parameter table and in that
  table's `not` row, and prints `confidential` in the example bodies of list,
  create and update while naming it in the parameter tables of create and
  update only, which is a gap in the page rather than in the endpoint. There is
  no way to send either one through the wrapper.
- `Epic` declares twelve fewer fields than the endpoint returns: `parent_iid`,
  `color`, `text_color`, `web_edit_url`, `work_item_id`, `references`,
  `imported`, `imported_from`, `_links`, `end_date`,
  `start_date_from_inherited_source` and `due_date_from_inherited_source`. The
  same OpenAPI record lists every one of them on all five epic GETs, live
  gitlab.com GETs on 2026-09-07 carried all twelve, and the documentation
  page's example bodies print all but `web_edit_url`, which it never mentions,
  and `text_color`, which appears only in its `with_labels_details` parameter
  row.

  `subscribed` and `reference` were counted here too, on the strength of the
  record alone, and neither belongs. A Grape entity's conditional expose is
  invisible to the generator that writes that record, so the record is the
  upper bound of what an entity can render rather than a statement about a
  route: `ee/lib/api/entities/epic.rb` exposes `subscribed` under
  `options.fetch(:include_subscribed, false)`, which `ee/lib/api/epics.rb`
  passes on `GET :id/epics/:epic_iid` alone, and `reference` under
  `with_reference`, which nothing sets and which GitLab deprecated in favour of
  `references`. The wrapper is missing neither, because the endpoints this
  server calls do not send them.
- `Epic.Labels` is typed `[]string`, and the documented `with_labels_details`
  parameter makes GitLab answer with an array of label objects instead. A caller
  who sends it gets a JSON decode failure rather than epics, so the parameter
  cannot be used through the wrapper at all.
- `EpicAuthor` declares six of the eight keys GitLab's user entity sends on an
  epic: `locked` and `public_email` are on the live response and on neither
  `EpicAuthor` nor `BasicUser`.
- On the Work Items side of the same domain, `workitems.go` selects
  `color { color textColor }` and `workItemWidgetColorGQL.unwrap` returns the
  colour alone, so `WorkItem` has no field for a value the query already paid
  for. That is why `text_color` reaches an epic only on the REST path here.

**Also**: `Epic` declares `UserNotesCount` and `URL`, and no epic endpoint sends
either. They are absent from the OpenAPI record, from every example body on the
documentation page, and from a live response. Anything reading them off a
decoded `Epic` reads a zero.

**How we found it**: the 1:1 output reconciliation for the epics domain, which
compares what we publish against what GitLab's generated OpenAPI record says
each endpoint returns. The `with_labels_details` failure was found by sending
the parameter to gitlab.com and reading the array back.

**Effort**: small for the filters and the fields (struct members with `url` and
`json` tags). The labels type is the one breaking change: a dual-shape field
needs either its own type with an `UnmarshalJSON`, as this repository carries,
or a second `LabelDetails` field beside the names.

### The note and discussion structs miss what GitLab sends and declare what it does not

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The missing fields are read from the captured response
  beside the SDK's own decode ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)):
  `internal/toolutil/note_capture.go` decodes a note, a discussion or a list
  of either into `NoteExtra`, `DiscussionExtra` and `NoteUserExtra`, and the
  converters in `internal/toolutil/note_shapes.go` take both halves. Seven
  packages carry it: `issuenotes`, `mrnotes`, `snippetnotes`,
  `commitdiscussions`, `mrdiscussions`, `issuediscussions` and
  `snippetdiscussions`. Retire the extras, the readers and the capture in
  those handlers once the wrapper carries the fields; the phantoms need no
  workaround, since this server's output shapes simply do not name them.

**What**: three structs in `notes.go` and `discussions.go`, held against the
Grape entities that render them at the pinned commit `1c8ac034` of
gitlab-org/gitlab.

- `Note` declares four fewer fields than `lib/api/entities/note.rb` exposes:
  `imported` and `imported_from`, sent on every note; `commands_changes`, sent
  on every note and carrying the quick actions a create or update applied,
  which is the one place a caller learns that `/label ~bug` in the body did
  something; and `suggestions`, an array of `lib/api/entities/suggestion.rb`
  objects (`id`, `from_line`, `to_line`, `appliable`, `applied`,
  `from_content`, `to_content`) sent on a merge request diff note, which is
  the only way to read a suggestion's id in order to apply it.
- `Note` also declares four fields no note endpoint sends: `attachment`,
  `title`, `file_name` and `expires_at`. None is exposed by the entity, none
  is in the OpenAPI record for any note operation, and anything reading them
  off a decoded `Note` reads a zero. They look like the residue of an older
  snippet shape.
- `NoteAuthor` and `NoteResolvedBy` both render through
  `lib/api/entities/user_basic.rb`, which exposes `id`, `username`,
  `public_email`, `name`, `state`, `locked`, `avatar_url` and `web_url`
  (`avatar_path` and `custom_attributes` too, under options no note endpoint
  passes). Both structs declare `email`, which the entity never sends and
  which the example bodies of
  [doc/api/notes.md](https://docs.gitlab.com/api/notes/) print on every
  author, so the page is where the field came from; neither declares
  `public_email` or `locked`.
- `Discussion` declares `id`, `individual_note` and `notes`, and
  `lib/api/entities/discussion.rb` exposes `resolvable` beside them on every
  discussion and `resolved` on a resolvable one. A caller listing the threads
  of a merge request cannot tell an open thread from a resolved one without
  walking its notes.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), which
holds each output type against the entity its endpoints render, with the
entity's own condition on each field; the four phantoms came from the same
run's type grain, and the `email` one from reading `user_basic.rb`.

**Effort**: small for the eight missing fields, all additive struct members
with `json` tags, and `suggestions` needs one new type. The four phantoms and
`email` are removals, so a deprecation note is the likely upstream shape for
them.

### The member structs, options and services miss what GitLab sends, accepts and serves

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: partial. The response fields are read from the captured
  response beside the SDK's decode ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)):
  `toolutil.CapturedMember` and `CapturedMembers` in
  `internal/toolutil/member_capture.go`, taken by the converters of
  `groupmembers`, `members` and the member list of `groups`, which retire
  when the structs carry the fields. The missing parameters and the missing
  endpoints have no workaround yet: this server does not publish them, and
  the action pass of the field-by-field review is where they are picked up.

**What**: three kinds of gap in `group_members.go` and `project_members.go`,
held against `lib/api/members.rb`, `ee/lib/ee/api/members.rb` and
`lib/api/entities/member.rb` at the pinned commit `1c8ac034` of
gitlab-org/gitlab.

- **Response fields.** `lib/api/entities/member.rb` exposes `locked` on every
  member and, on an Enterprise instance, `membership_state`; it exposes
  `two_factor_enabled` to a caller allowed to read it (the pages say group
  owners and administrators), `group_scim_identity` (`extern_uid`,
  `group_id`, `active`) to the owners of an SSO-enabled group, and `override`,
  the LDAP override flag, on an LDAP member of a group. `GroupMember` declares
  none of the five. `ProjectMember` declares none of them either, and also
  lacks `public_email`, which the entity sends on every member, and
  `group_saml_identity`, which `GroupMember` carries: a project member
  inherited from a group renders through the same entity, so the project
  struct is the group struct minus three fields for no reason in GitLab.
  `avatar_path` and `custom_attributes` are exposed too, under presenter
  options that `present_members` in `lib/api/helpers/members_helpers.rb`
  never passes, so they are correctly absent from both structs.
- **Parameters.** `ListGroupMembersOptions` and `ListProjectMembersOptions`
  lack `skip_users`, which `GET /:source/:id/members` takes on both, and the
  `state` filter (`awaiting` or `active`, Premium) the Enterprise prepend adds
  to the same list. `AddGroupMemberOptions` and `AddProjectMemberOptions` lack
  `invite_source`. `DeleteProjectMember` takes no options at all, while the
  endpoint accepts `skip_subresources` and `unassign_issuables` the way the
  group one does, which `RemoveGroupMemberOptions` models.
- **Endpoints.** `ee/lib/ee/api/members.rb` serves six group endpoints no
  service method reaches: `POST` and `DELETE /groups/:id/members/:user_id/override`,
  `PUT /groups/:id/members/:member_id/approve`,
  `POST /groups/:id/members/approve_all`, `GET /groups/:id/pending_members`,
  `PUT /groups/:id/members/:user_id/state` and
  `GET /groups/:id/billable_members/:user_id/indirect`. All are documented
  on [doc/api/group_members.md](https://docs.gitlab.com/api/group_members/).

**How we found it**: the sent dimension of the 1:1 audit for the fields
(`shapes.typed.unsurfaced` and `shapes.sent.unsurfaced` in
`go run ./cmd/audit_1to1/ -scope=paths`, with the entity condition on each);
the parameters and the endpoints by reading the two Ruby files beside the
service methods while writing the workaround.

**Effort**: small for the fields, all additive struct members with `json`
tags plus one type for the SCIM identity; small for the parameters, additive
fields on the option structs and one new options struct for the project
delete; medium for the endpoints, six methods with their option and result
types.

### Six response structs miss a field GitLab sends on every object

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. Each field is read from the captured response beside
  the SDK's decode ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)),
  through the readers in `internal/toolutil/sent_shapes.go`, in `keys`,
  `runners`, `cilint`, `labeldata` with `labels` and `grouplabels`,
  `pipelines` and `pipelinetriggers`. They retire when the structs carry the
  fields.

**What**: one gap per struct, each a field the rendering entity exposes with
no condition at the pinned commit `1c8ac034` of gitlab-org/gitlab, except where
noted.

- `Key` (`keys.go`) declares `id`, `title`, `key`, `created_at` and `user`,
  and `lib/api/entities/ssh_key.rb` exposes `expires_at`, `last_used_at` and
  `usage_type` beside them; `SSHKey` in `users.go` carries the first and the
  third and not `last_used_at`. The
  [keys page](https://docs.gitlab.com/api/keys/) prints all three.
- `Runner` and `RunnerDetails` (`runners.go`) declare neither `created_at`,
  nor `created_by`, nor `job_execution_status`, which
  `lib/api/entities/ci/runner.rb` exposes on every runner, the second one to
  a caller allowed to read the creating user. The
  [runners page](https://docs.gitlab.com/api/runners/) prints
  `job_execution_status` on every example.
- `ProjectLintResult` (`validate.go`) declares no `jobs`, the array
  `lib/api/entities/ci/lint/result.rb` exposes when the request set
  `include_jobs`, which both `ProjectLintOptions` and
  `ProjectNamespaceLintOptions` already model, so the option is accepted and
  its one effect on the answer is dropped. The
  [lint page](https://docs.gitlab.com/api/lint/) prints the array.
- `Label` and `GroupLabel` (`labels.go`, `group_labels.go`) declare no
  `description_html`, which `lib/api/entities/label.rb` exposes on every
  label and both label pages print in every example body.
- `Pipeline` (`pipelines.go`) declares no `archived`, which
  `lib/api/entities/ci/pipeline.rb` exposes on every pipeline rendered whole
  and the pipelines page prints on every single-pipeline example. The list
  rows render through `Ci::PipelineBasic`, which does not carry it, so
  `PipelineInfo` is complete.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`, with
the entity condition on each field), during the field-by-field review.

**Effort**: small. Every field is an additive struct member with a `json`
tag; the lint jobs need one type for the job object, and the runner's creator
can reuse whichever basic-user struct the wrapper settles on.

### No token struct carries the granular fields, and the impersonation and resource ones carry less still

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The fields are read from the captured response beside
  the SDK's decode ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)),
  through the three token readers in `internal/toolutil/sent_shapes.go`, in
  every package that presents a token: `accesstokens`,
  `impersonationtokens`, `users`, `groupserviceaccounts` and
  `projectserviceaccounts`. They retire when the structs carry the fields.

**What**: `PersonalAccessToken` in `personal_access_tokens.go` and the three
types built on it, held against the entities that render them at the pinned
commit `1c8ac034` of gitlab-org/gitlab.

- **Every token is missing three fields.**
  `lib/api/entities/personal_access_token.rb` exposes `granular` on every
  token, and the two entities inheriting it add `granular_scopes`, the
  array of `{access, permissions, project_id, group_id}` objects a granular
  token carries, and `last_used_ips`, the addresses the token authenticated
  from. `PersonalAccessToken` declares none of the three, so a caller
  cannot tell a granular token from a classic one, cannot read the scopes a
  granular one actually holds, and cannot see where a token has been used.
  The granular scopes are not a nicety: `lib/api/personal_access_tokens.rb`
  passes `with_granular_scopes: true` on list, get and rotate, so GitLab
  sends them and the decode drops them.
- **`ImpersonationToken` is missing six.** It renders through
  `lib/api/entities/impersonation_token.rb`, which inherits the personal
  access token entity and adds `impersonation`. The struct declares neither
  that flag nor the `description` and `user_id` the parent entity sends,
  which `PersonalAccessToken` does model, so the impersonation type is the
  personal one minus two fields for no reason in GitLab, plus the three
  above.
- **The resource token is missing five.** `ProjectAccessToken` and
  `GroupAccessToken` are both the unexported `resourceAccessToken`, which
  embeds `PersonalAccessToken` and adds `access_level` alone, while
  `lib/api/entities/resource_access_token.rb` also exposes `resource_type`
  (`project` or `group`) and `resource_id`. A caller listing tokens across
  scopes cannot tell which resource each belongs to.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` and `shapes.sent.unsurfaced` in
`go run ./cmd/audit_1to1/ -scope=paths`) reported the granular trio on the
shared shape; reading the four entities beside it during the field-by-field
review turned up the impersonation and resource fields, which the generated
OpenAPI record does not list because the entity exposes them through blocks
rather than documented attributes.

**Effort**: small for the fields, additive struct members with `json` tags
plus one type for the granular scope object. The `granular_scopes` field on
`resourceAccessToken` comes free with the embed once
`PersonalAccessToken` carries it.

### The four Sidekiq routes carry a leading slash and send a double slash

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. `gitlab.com` answers the malformed path with a redirect.
- **Workaround**: none. There is nowhere to put one: the path is built inside
  the SDK from a constant, and the handlers in `internal/tools/sidekiq` only
  call the service methods.

**What**: `sidekiq_metrics.go:24-27` declares its four routes as
`route("/sidekiq/compound_metrics")` and the three beside it, with a leading
slash. `NewRequest` joins the path onto `/api/v4`, so this server sends
`GET /api/v4//sidekiq/queue_metrics`.

**Evidence that it is a defect rather than a convention**: of the 699 `route(…)`
declarations in client-go v3.0.0, **695 have no leading slash and the 4 that do
are all in this one file**. The malformed paths are visible in this
repository's own committed record of what it sends,
`docs/development/request-inventory.json` (`//sidekiq/compound_metrics`,
`//sidekiq/job_stats`, `//sidekiq/process_metrics`, `//sidekiq/queue_metrics`),
and `internal/tools/sidekiq/sidekiq_test.go` registers its mock handler under
the double slash because that is what arrives.

**Why it matters even though nothing is broken today**: `gitlab.com` answers a
double slash with a 308 to the collapsed path, so the call succeeds after a
redirect. A self-managed front end is not obliged to do that, and a proxy that
normalizes differently, or a deployment that refuses a redirect on an
authenticated request, would see four tools stop working for a reason nothing
in this repository could explain.

**Effort**: trivial, four characters. A good first contribution, and the kind
of change whose test is one assertion on the built URL.

### Response structs that miss a field GitLab sends unconditionally

- **Reported**: yes, one merge request per struct, linked below.
- **In review**: yes, the same merge requests.
- **Merged**: not yet.
- **Blocking**: no.
- **Workaround**: yes. Each field is read from the captured response beside
  the SDK's decode, through the readers in `internal/toolutil/sent_shapes.go`.
  Each retires when its merge request lands and the pin moves.

**What**: one field per struct, each exposed by the rendering entity with no
condition at all, so every response of every endpoint that renders it carries
the field and the SDK drops it. Every entity reference is to the tag
`v19.3.1-ee`, which is the release the record was taken from.

- `Topic` (`topics.go`) has no `organization_id`, which
  [lib/api/entities/projects/topic.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/projects/topic.rb)
  exposes on line 12.
  [gitlab-org/api/client-go!3041](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3041).
- `Appearance` (`appearance.go`) has no `site_name`, which
  [lib/api/entities/appearance.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/appearance.rb)
  exposes on line 44. The same merge request adds it to
  `ChangeAppearanceOptions`, because `lib/api/appearance.rb` declares
  `site_name` as an accepted parameter of the `PUT` on line 58, so the SDK
  could neither read it nor set it.
  [gitlab-org/api/client-go!3040](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3040).
- `BroadcastMessage` (`broadcast_messages.go`) has no `color`, which
  [lib/api/entities/system/broadcast_message.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/system/broadcast_message.rb)
  exposes on line 11 and the model defaults to `#E75E40`, so it is never
  blank. The same merge request adds it to both option structs, since
  `lib/api/admin/broadcast_messages.rb` accepts `color` on create and update.
  The SDK's own test fixtures already carried the key with no field to decode
  it into, which is as clear a statement of the gap as the entity is.
  [gitlab-org/api/client-go!3042](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3042).
  Worth knowing when reading that merge request: the
  [broadcast messages page](https://docs.gitlab.com/api/broadcast_messages/)
  omits `color` from both the response examples and the parameter tables while
  still documenting `font`, so the documentation is the one source that does
  not show it. A live `GET /broadcast_messages` on gitlab.com does, on every
  message.
- `Agent` (`cluster_agents.go`) has no `is_receptive`. The exposure is not in
  the Community Edition entity but in the Enterprise module prepended onto it,
  [ee/lib/ee/api/entities/clusters/agent.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/ee/api/entities/clusters/agent.rb)
  line 11, so an Enterprise instance sends it on every agent and a Community
  one sends nothing. The register-agent options need no change: the route
  declares `requires :name` and nothing else.
  [gitlab-org/api/client-go!3043](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3043).
- `LicenseTemplate` (`license_templates.go`) has no `popular`, which
  [lib/api/entities/license.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/license.rb)
  exposes on line 7 and a live gitlab.com sends on both licence endpoints. Not
  to be confused with `ListLicenseTemplatesOptions.Popular`, which is the
  request filter and already exists.
  [gitlab-org/api/client-go!3044](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3044).
  The struct's `Featured` is the other half of this: no GitLab sends
  `featured`, the entity does not expose it, and the field has therefore never
  decoded. It survives because removing an exported field is breaking, and the
  [licences page](https://docs.gitlab.com/api/templates/licenses/) still shows
  `featured` in its example response, which is what hid the real key.
- `SecureFile` (`secure_files.go`) has no `file_extension`, which
  [lib/api/entities/ci/secure_file.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/ci/secure_file.rb)
  exposes on line 15 and the list, show and create endpoints all render.
  [gitlab-org/api/client-go!3045](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3045).
- `GroupSCIMIdentity` (`group_scim.go`) is the odd one out, and the worst of
  them: it declares the tag `external_uid` and GitLab sends `extern_uid`, so
  the field never decodes and is the empty string on every call.
  [ee/lib/api/entities/identity_detail.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/api/entities/identity_detail.rb)
  exposes `extern_uid` on line 6, the SCIM page documents that spelling, and
  every other struct in the SDK carrying this key already spells it that way.
  The SDK's own fixtures used the wrong key too, so its tests were green
  against a response GitLab never sends.
  [gitlab-org/api/client-go!3046](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3046).

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), whose
oracle is `docs/development/gitlab-api-live.json`, a record of what a booted
GitLab says each of its Grape entities exposes. Each finding carries
`sdk_models: false`, which is what says the gap is upstream rather than ours.

**Effort**: trivial per struct. One additive field with a `json` tag, and one
key added to the fixture of an existing test.

**Where these merge requests come from**: the
[community fork](https://gitlab.com/gitlab-community/gitlab-org/api/client-go),
which `CONTRIBUTING.md` recommends over a personal one. It is worth doing:
GitLab's triage bot posts a "did you know about our community forks" nudge on
every merge request opened from a personal fork and does not on one opened
from the community fork, and the pipeline runs either way. What it does not
fix is the `DCR4003` warning the Duo reviewer leaves, which comes from the
automatic review request the upstream project makes on the author's behalf and
fails because the author is not a member there. A merge request's source
cannot be re-pointed after it is opened, so moving one means opening a new one
from the community fork and closing the old with a note.

## MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)

### No keep-alive interval for SSE streams on StreamableHTTPOptions

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, and it covers more than the requested option would.
  `sseAwareWriter` in `cmd/server/main.go` emits a comment frame every 25s on
  **any** response that commits to `text/event-stream`, guarding its writes with
  the same mutex the handler's writes take. It would stay even if the option
  landed.

**What**: the SDK emits keep-alives only on the standalone GET stream, not on
streamed POST responses, and offers no option to configure the interval. An idle
SSE response therefore puts no bytes on the wire, and a proxy's read timeout
severs it; nginx's `proxy_read_timeout` is 60s by default. Worse, with nothing
written the response headers are not flushed either, so the client hangs before
the first read rather than after.

**How we found it**: writing `TestSSEKeepAlive_IdleStreamKeepsBytesOnTheWire`.
The test hung instead of failing, which is how the header-flush half surfaced.

### A malformed message ends the session instead of answering -32700

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: it was, on stdio. One client lost its session and its
  accumulated context to a single unparseable line; there was no cross-tenant
  effect, since stdio is one process per client.
- **Workaround**: yes. `resilientStdio` in `cmd/server/stdio.go` filters stdin
  ahead of the SDK, answering a line the read loop would choke on and dropping
  it. This entry first said no workaround was available, on the reasoning that
  the decision sits inside the SDK's read loop; that was wrong. The SDK exposes
  `mcp.IOTransport`, which takes any `Reader` and `Writer`, so the loop can be
  fed a stream that never contains the input it cannot handle. Anything parsing
  as a JSON object with `"jsonrpc":"2.0"` is passed through untouched, since
  deciding what a valid message means is the SDK's job.

**What**: `internal/jsonrpc2/conn.go`'s `readIncoming` breaks its loop on *any*
error from `reader.Read`, so a message that fails to parse is treated exactly
like a closed pipe. The session ends, and on stdio the process exits. Nothing is
written to the client, which sees EOF on a stream it can still write to.

JSON-RPC 2.0 defines `-32700 Parse error` for this case, and the framing here is
one message per line, so the next line is an independent message and
resynchronizing is trivial. Both a line that is not JSON (`{not json`) and one
that parses but carries no `"jsonrpc":"2.0"` (`{"hello":"world"}`) produce it; a
request the server understands but cannot serve — an unknown method, a
nonexistent tool — is correctly answered with an error and the session
continues.

**How we found it**: writing `test/e2e/stdio`. The case was written expecting
the session to survive, and it did not.

**Also fixed by the workaround, unintentionally**: with `mcp.StdioTransport`,
EOF on stdin — a client closing its pipe, which is how every session ends —
produced an error from `server.Run`, and the process exited 1. A clean shutdown
reported failure to whatever supervises it. Under `IOTransport` it exits 0. A
unit test had been passing because of that error rather than the condition it
named, which is how this surfaced.

**Pinned by**: `TestMalformedInput_IsAnsweredAndTheSessionSurvives` in
`test/e2e/stdio`, and `TestResilientStdio_*` in `cmd/server`. The e2e case
asserts the client-visible contract rather than the workaround, so it keeps
passing if the SDK ever fixes this and the filter is removed.

### A cancelled incoming call is still answered

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: partial and honest rather than a fix. The response cannot be
  suppressed, so what we do is stop it lying: a cancellation is classified as
  "the request was cancelled by the client" instead of falling through to
  "unexpected error", and logged at INFO rather than ERROR. The client behaviour
  is documented in the HTTP guide so a client can expect the late response.

**What**: "Servers receiving cancellation notifications SHOULD ... not send a
response for the cancelled request." `internal/jsonrpc2/conn.go` writes the
response with `c.write(notDone{req.ctx}, response)`, where `notDone` deliberately
strips the cancellation from the context so the write proceeds. Nothing at
application level runs between the handler returning and that write, so no
server built on this SDK can satisfy the clause.

**How we found it**: the interaction-pattern audit. Captured on stdio, two
seconds into a hanging GitLab call: the client's `notifications/cancelled` at
6.306s, and the server's response to that same request id at 6.307s.

### The cancellation reason is discarded before any handler sees it

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none possible. The field is dropped inside the SDK; there is
  no seam to read it from. We log what remains — that the call was cancelled and
  how long it ran.

**What**: "Implementations SHOULD log cancellation reasons for debugging."
`mcp/transport.go` unmarshals `CancelledParams`, uses `params.RequestID` to
cancel the call, and discards `params.Reason`. A hook, or even a logger line at
the preempter, would be enough.

**How we found it**: the interaction-pattern audit. Sending a cancellation with
reason "User requested cancellation" left no trace of that string anywhere in
the server's output.

### The declared protocol version, not the negotiated one, selects MRTR

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none taken, deliberately. Disagreeing with the SDK here would
  be worse than matching it: our gate and `clientSupportsMultiRoundTrip` would
  choose differently for the same session, and the SDK labels the result.

**What**: `initialize` is deprecated in 2026-07-28, so `negotiatedVersion`
(`mcp/shared.go`) caps that handshake at `2025-11-25` — while
`InitializeParams.ProtocolVersion` keeps whatever the client asked for.
`clientSupportsMultiRoundTrip` (`mcp/mrtr.go`) reads the latter, so a client
that sends `initialize` requesting `2026-07-28` is negotiated down to
`2025-11-25` and served multi round-trip requests regardless. The negotiated
version is the one that describes what the session can actually do, and it
should drive both.

**Blast radius is small.** The SDK's own client middleware fulfills
`inputRequests` whatever version it negotiated, so an SDK-based client never
notices. It takes a hand-written client that implements `2025-11-25` strictly,
claims `2026-07-28` in its handshake, and ignores `inputRequests` to be harmed.

**How we found it**: the interaction-pattern specification audit. Observed on
stdio: `initialize -> protocolVersion='2025-11-25'`, and the next `tools/call`
answered `resultType: 'input_required'`.

**Documented in**: `docs/reference/capabilities/elicitation.md`, so the
behaviour is stated where someone writing a client would look.

### Application code cannot send notifications/cancelled for a listen stream

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The client still receives the completion result the SDK
  writes when the stream's handler returns, which tells a conforming client the
  subscription has ended.
- **Workaround**: none, and unlike the malformed-message entry this one really
  has none: the type cannot be constructed at all, so there is nothing to
  interpose.

**What**: 2026-07-28 says a server "MUST send `notifications/cancelled`
referencing a `subscriptions/listen` request ID when it tears down that
subscription stream". `SubscriptionsListenResult` embeds an unexported type, so
application code cannot build the message, and the SDK offers no method that
sends one. A server that ends a subscription can therefore satisfy the graceful
half of the contract and not this one.

**How we found it**: the interaction-pattern specification audit. Reproduced on
stdio at 2026-07-28: a watcher retired after its resource began returning 404,
and the only output was the listen request's own result — no
`notifications/cancelled`, before or after, with the connection still usable.

**Recorded in**: ADR-0015, so the gap is stated where the design is rather than
only here.

### `Mcp-Name` is compared without decoding the base64 sentinel

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Nothing on this server's surface forces the encoded form:
  every tool name is `gitlab_*`, every prompt name is ASCII, and a resource URI
  is a URI, so non-ASCII arrives percent-encoded and matches a plain header.
- **Workaround**: none taken. The header is the SDK's to validate and this
  repository reads `Mcp-Name` nowhere outside tests, so intercepting it would
  mean a second implementation of a rule the SDK already has fifteen lines
  further down.

**What**: SEP-2575 defines a `=?base64?…?=` sentinel for header values that
cannot be sent as plain ASCII, and the transport binding says servers **MUST**
decode encoded values before comparing them to the body. In
`mcp/streamable_headers.go`, `validateParamHeaders` calls `decodeHeaderValue`
(line 425) and `validateMcpHeaders` does not (line 381): it compares
`nameInHeader != nameInBody` raw. A client that encodes a name is answered
`-32020 "header mismatch"` even though its header and body agree.

The direction of the failure is the notable part. Encoding an ASCII-safe value
is permitted, not forbidden, so a client that does it is conforming and is
refused, while a client that sends a non-ASCII name as raw UTF-8 bytes passes:
Go's `net/textproto` hands those through and the string comparison succeeds.
Only the conforming client is rejected.

**How we found it**: the transports specification audit. Reproduced against the
shipped HTTP default with a base64 `Mcp-Name` on `tools/call`, `resources/read`
and `prompts/get`, all three answered `-32020` where the plain-ASCII control was
served. It is upstream by this file's own test: it happens to any caller of
`StreamableHTTPHandler` with no setting of ours.

### A middleware cannot ask whether a request carries params

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no, once known. The check is three lines of `reflect` and this
  server now makes it in one place.
- **Workaround**: `internal/mcpotel.paramsOf` returns `nil` for a `Params`
  interface holding a nil pointer, and the three consumers in that package ask
  through it rather than calling `GetParams` themselves.

**What**: `Params` (mcp/shared.go) declares `GetMeta`, `SetMeta`, `isParams` and
`isNil`. The last exists because a receiving middleware is handed a typed nil
whenever the wire omitted the params member, which `serverMethodInfos` permits
for every list method and for `notifications/initialized`. It is unexported, so
only the SDK can ask. Outside the package, `req.GetParams() != nil` is true for
that value, because an interface holding a nil pointer is not a nil interface,
and `GetMeta` on it dereferences the pointer.

The shape is ordinary: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` is a
complete, valid request that clients really send. Nothing in the SDK's own
documentation for `AddReceivingMiddleware` mentions it, and the accessor named
after the question is the one thing a middleware cannot reach.

**How we found it**: a hosted deployment logged "recovered a panic while
handling a request" a hundred times in a day, on `tools/list`, `prompts/list`,
`resources/list` and `notifications/initialized`, with the stack ending in
`(*ListToolsParams).GetMeta` called from this repository's trace-context
carrier. The bug in the carrier is ours, and a comment in it had asserted the
wire could not produce such a request. Exporting `isNil`, or documenting the
guarantee, would keep the next middleware from writing the same line.

### The protocol version is classified by string ordering

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The window contains only malformed version strings.
- **Workaround**: none taken. The clean seam is a wrapping `mcp.Transport` that
  inspects the decoded request before the SDK session sees it; the stdio filter
  in `cmd/server/stdio.go` is deliberately not that seam, since it exists to do
  the least it can and parsing `params._meta` there would be a second, divergent
  implementation of the protocol. Receiving middleware cannot serve either: the
  initialization gate returns at `mcp/server.go:1901`, before `handleReceive`
  runs the middleware chain at :1927.

**What**: `mcp/shared.go:549` decides whether a request is modern by comparing
version strings with `<`. Anything sorting below `2026-07-28` is treated as a
legacy handshake, so a request carrying an unrecognised version in per-request
`_meta` is answered `method "tools/list" is invalid during session
initialization` with wire code `0`, rather than the
`UnsupportedProtocolVersionError` the versioning page requires. Published
revisions are unaffected, since they are all in the supported list; the window
holds `2026-01-01`, `2025-11-24`, `1900-01-01`, `1.0`, `0` and the empty string.

`1900-01-01` is the versioning page's own worked example of this error, so the
literal illustration in the specification is the case that comes back wrong.

**How we found it**: the lifecycle specification audit, on stdio. HTTP cannot
reach it: the header check rejects a `_meta`-only version with `-32020`, and
`protocolVersionMiddleware` in this repository answers the header case itself.

### A resource update cannot be delivered to one session

- **Reported**: no. Deferred: the workaround is complete and no upstream change
  is being asked for yet.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `sessionOwners.sendingMiddleware` in
  `cmd/server/session_owner.go` stamps the owning pool entry into the
  notification's `_meta`, filters delivery per session on the way out, and
  strips the private key from a clone before the frame is written. It retires
  the day either fix below lands.

**What**: `Server.ResourceUpdated(ctx, params)` is the only exported delivery
for `notifications/resources/updated`, and it notifies **every** session
subscribed to that URI. `ServerSession` exposes no equivalent of its own: its
senders are `NotifyProgress`, `Log`, `Ping`, `ListRoots`, `CreateMessage` and
`Elicit`. Application code cannot construct the 2026-07-28 form either, because
that one needs the listen request id the SDK stamps into `_meta` from its own
subscription table.

**Why it matters here**: one `mcp.Server` now serves every credential of a
configuration shape (ADR-0020), and the SDK's subscription table is keyed by URI
and session with no notion of a credential. Two credentials subscribed to the
same resource therefore each receive the other's notifications, carrying the
other's watch state, and a credential whose access was revoked goes on being
told the resource changed by somebody else's polling.

**Either of two changes would close it.** A per-session sender,
`ServerSession.ResourceUpdated(ctx, params)`, reading that session's own request
id from the table, so a caller can deliver to the sessions it knows about. Or
propagating the caller's context to the sending middleware: both delivery paths
build a fresh `context.Background()` with a ten second timeout
(`notifySessions` in `shared.go`, `notifySubscribedSessions` in `server.go`), so
nothing a caller of `ResourceUpdated` puts on its context can reach the
middleware, which is why the owner has to travel in the params instead.

**How we found it**: making the pool share one server per configuration shape.
The delivery end was the only part of the design with no per-credential seam.

## OpenAI Codex (`openai/codex`)

### A non-integer annotation priority breaks a tool call

- **Reported**: yes,
  [openai/codex#38979](https://github.com/openai/codex/issues/38979).
- **In review**: yes, the issue is open and labelled `bug`, `mcp`, `CLI`,
  `tool-calls`. No fix has been proposed upstream.
- **Merged**: no.
- **Blocking**: it was. Every tool call failed with "Unexpected response type",
  so the server was unusable from Codex rather than degraded.
- **Workaround**: yes, and it is load-bearing. `internal/clientcompat` detects
  Codex from `clientInfo` and rounds annotation priorities to 0 or 1, which is
  spec-legal and parseable by both. `GITLAB_MCP_CLIENT_COMPAT=off` disables it. Retire it
  only once the fixed Codex is widely deployed, not merely released: the
  affected build ships inside ChatGPT.app, so users do not choose their version.

**What**: the Codex builds bundled with ChatGPT.app reject any MCP result whose
`annotations.priority` is a non-integer float. `0.6` fails; `1` or an
audience-only annotation passes. The specification places no such restriction,
and crates.io `rmcp` 3.0.0 parses floats correctly (`Option<f32>`), so the defect
is in the patched bundle rather than in the library.

**How we found it**: every tool call from Codex failed with "Unexpected response
type" and nothing else. Bisected with a Python fake server replaying canned
`CallToolResult` values until the float was the only variable left.

**Not the cause, though it looks like it**: unknown fields. Neither Codex
generation used `deny_unknown_fields`, and that hypothesis cost a day before it
was ruled out.

**Related, and deliberately not worked around**:
[openai/codex#10334](https://github.com/openai/codex/issues/10334) — Codex sends
only `structuredContent` to its model when both are present, dropping the
markdown. We keep emitting both.

## Other

### go-selfupdate depends on the deprecated x/crypto/openpgp

- **Reported**: yes,
  [creativeprojects/go-selfupdate#57](https://github.com/creativeprojects/go-selfupdate/issues/57).
- **In review**: yes,
  [creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58),
  still open.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: retired. It was the `GO-2026-5932` entry in the govulncheck
  allowlist; removing the self-update subsystem removed the dependency, so the
  advisory no longer reaches any binary and the allowlist is empty again. The
  upstream PR stays worth merging for the module's other users.

### A receiving middleware cannot read the JSON-RPC request id

`mcp.Request` exposes `GetSession`, `GetParams` and `GetExtra`, and nothing that
returns the JSON-RPC `id` of the message being handled. A receiving middleware
therefore cannot see it, and neither can anything built on one.

That is what stops this server from emitting `jsonrpc.request.id`, which the MCP
semantic convention marks Conditionally Required "When the client executes a
request". The attribute is what distinguishes a request span from a notification
span, and the Go semantic-conventions package already ships the key
(`semconv.JSONRPCRequestID`), so the only missing piece is an accessor.

The id is not secret and is already on the wire in both directions; the SDK
decodes it to route the response and then discards it before application code
runs, the same shape as
[the cancellation reason](#the-cancellation-reason-is-discarded-before-any-handler-sees-it). Adding `GetID()` to the `Request` interface, or a
field on `RequestExtra`, would close it.

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. One Conditionally Required attribute is omitted; the span is
  otherwise complete and the metric does not carry the attribute at all.
- **Workaround**: none possible. Nothing in the public API exposes the value.
