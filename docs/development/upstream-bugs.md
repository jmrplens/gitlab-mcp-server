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

A defect we **fix upstream** in a project this server depends on belongs here
too, even when we found it somewhere else. The register's second job is to say
what is open in our name and what it is waiting on, and a contribution left out
of it is one nobody here can see the state of. Such an entry says plainly how it
was found, and says what it costs this server, which for one found elsewhere is
usually nothing.

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
| 8 | go-sdk | [No SSE keep-alive option](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions) | Yes, [#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262) | Yes, theirs, [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232), merged; ours follows it | Partly, by [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232), not ours, unreleased | No | Yes |
| 9 | go-sdk | [A malformed message ends the session](#a-malformed-message-ends-the-session-instead-of-answering--32700) | Yes, by another user | Yes, theirs, open | No | Was yes | Yes |
| 10 | go-sdk | [Cannot send `notifications/cancelled` for a listen stream](#application-code-cannot-send-notificationscancelled-for-a-listen-stream) | Yes, [#1263](https://github.com/modelcontextprotocol/go-sdk/issues/1263) | No, proposal first | No | No | None possible |
| 11 | go-sdk | [Declared, not negotiated, version selects MRTR](#the-declared-protocol-version-not-the-negotiated-one-selects-mrtr) | Yes, [#1258](https://github.com/modelcontextprotocol/go-sdk/issues/1258) | Yes, [#1266](https://github.com/modelcontextprotocol/go-sdk/pull/1266), open | No | No | None taken |
| 12 | go-sdk | [A cancelled call is still answered](#a-cancelled-incoming-call-is-still-answered) | Yes, [#1259](https://github.com/modelcontextprotocol/go-sdk/issues/1259) | Yes, [#1267](https://github.com/modelcontextprotocol/go-sdk/pull/1267), open | No | No | Partial |
| 13 | go-sdk | [The cancellation reason is discarded](#the-cancellation-reason-is-discarded-before-any-handler-sees-it) | Yes | Yes, [#1255](https://github.com/modelcontextprotocol/go-sdk/pull/1255), merged | **Yes, unreleased** | No | Yes, until it ships |
| 14 | go-sdk | [`Mcp-Name` compared without decoding](#mcp-name-is-compared-without-decoding-the-base64-sentinel) | Yes, by another user, [modelcontextprotocol/go-sdk#1234](https://github.com/modelcontextprotocol/go-sdk/issues/1234) | Yes, theirs, [modelcontextprotocol/go-sdk#1242](https://github.com/modelcontextprotocol/go-sdk/pull/1242), merged | **Yes, unreleased** | No | None taken |
| 15 | go-sdk | [Protocol version classified by string ordering](#the-protocol-version-is-classified-by-string-ordering) | Yes, [#1260](https://github.com/modelcontextprotocol/go-sdk/issues/1260) | Yes, [#1268](https://github.com/modelcontextprotocol/go-sdk/pull/1268), merged | **Yes, unreleased** | No | None taken |
| 16 | go-selfupdate | [Deprecated `x/crypto/openpgp`](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp) | Yes | Yes, open | No | No | Retired |
| 17 | codex | [Non-integer `priority` breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call) | Yes | No | No | Was yes | Yes |
| 18 | go-sdk | [A receiving middleware cannot read the JSON-RPC id](#a-receiving-middleware-cannot-read-the-json-rpc-request-id) | Yes, [#1264](https://github.com/modelcontextprotocol/go-sdk/issues/1264) | No, proposal first | No | No | None possible |
| 19 | client-go | [Security mutations discard GraphQL errors](#the-security-attribute-and-category-mutations-discard-graphql-errors) | No | No | No | No | Yes |
| 20 | client-go | [Dependency Firewall lacks `operation` and the enablement endpoint](#the-dependency-firewall-wrapper-is-missing-an-attribute-and-an-endpoint) | No | No | No | No | None |
| 21 | go-sdk | [A middleware cannot ask whether a request carries params](#a-middleware-cannot-ask-whether-a-request-carries-params) | Yes, [#1261](https://github.com/modelcontextprotocol/go-sdk/issues/1261) | Yes, [#1269](https://github.com/modelcontextprotocol/go-sdk/pull/1269), merged | **Yes, unreleased** | No | Yes |
| 22 | client-go | [Enum constants lag the documented value sets](#enum-constants-lag-the-documented-value-sets) | No | No | No | No | Yes |
| 23 | go-sdk | [No per-session resource-updated delivery](#a-resource-update-cannot-be-delivered-to-one-session) | Yes, [#1265](https://github.com/modelcontextprotocol/go-sdk/issues/1265) | No, proposal first | No | No | Yes |
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
| 34 | client-go | [Response structs that miss a field GitLab sends unconditionally](#response-structs-that-miss-a-field-gitlab-sends-unconditionally) | Yes | Yes, 2 open | **12 of 14; all 12 released, v3.1.0 to v3.11.0** | No | Retired for the 12; the 2 open ones keep theirs |
| 35 | client-go | [The Geo structs model a fraction of a site and its status, and the repair method names the wrong entity](#the-geo-structs-model-a-fraction-of-a-site-and-its-status-and-the-repair-method-names-the-wrong-entity) | In part, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Partial |
| 36 | client-go | [The merge request structs miss six keys, unevenly, and two methods name an entity they do not answer with](#the-merge-request-structs-miss-six-keys-unevenly-and-two-methods-name-an-entity-they-do-not-answer-with) | In part, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Partial |
| 37 | client-go | [The User struct models one user entity and GitLab serves six](#the-user-struct-models-one-user-entity-and-gitlab-serves-six) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Yes |
| 38 | gitlab-org/gitlab | [Three job token scope endpoints declare a response entity they do not send](#three-job-token-scope-endpoints-declare-a-response-entity-they-do-not-send) | Yes | Yes, [gitlab-org/gitlab!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698), merged | **Yes, 19.4.0** | No | Yes, until the live record is taken from 19.4 or later |
| 39 | gitlab-org/gitlab | [Two project group listings are annotated with the whole Group entity](#two-project-group-listings-are-annotated-with-the-whole-group-entity) | Yes | Yes, [gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699), open and approved | No | No | Yes |
| 40 | client-go | [Ten modelled fields that no Grape entity exposes](#ten-modelled-fields-that-no-grape-entity-exposes-removed-from-this-servers-output) | No | No | No | No | Not needed |
| 41 | client-go | [IssueRelation models an issue basic where GitLab renders a whole issue](#issuerelation-models-an-issue-basic-where-gitlab-renders-a-whole-issue) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Yes |
| 42 | client-go | [MemberRole models twenty of the forty-five permissions GitLab sends](#memberrole-models-twenty-of-the-forty-five-permissions-gitlab-sends) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Yes |
| 43 | client-go | [PipelineInfo decodes two entities and models only the smaller one](#pipelineinfo-decodes-two-entities-and-models-only-the-smaller-one) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Yes |
| 44 | client-go | [Group, Project and Issue each model one entity where GitLab renders two](#group-project-and-issue-each-model-one-entity-where-gitlab-renders-two) | In part, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) | No | No | No | Yes |
| 45 | client-go | [The work item get, create and update documents select licensed fields](#the-work-item-get-create-and-update-documents-select-licensed-fields) | No | No | No | Yes, on Community Edition | None possible |
| 46 | gitlab-org/gitlab | [Cancelling an auto-merge answers a status hash under a merge request annotation](#cancelling-an-auto-merge-answers-a-status-hash-under-a-merge-request-annotation) | Yes | Yes, [!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704), open; [!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239) closed unmerged | No | Was yes | Yes |
| 47 | gitlab-org/gitlab | [A revoked GPG UID still verifies commits](#a-revoked-gpg-uid-is-still-offered-for-verification-and-still-verifies-commits) | Yes, by another user, [gitlab-org/gitlab#24572](https://gitlab.com/gitlab-org/gitlab/-/work_items/24572) | Yes, [gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300), merged | **Yes, unreleased** | No | None possible |
| 48 | go-sdk | [Two listens on one URI leave a session receiving neither](#a-sessions-second-listen-on-a-uri-overwrites-the-firsts-subscription-and-its-close-deletes-both) | No | No | No | No | Partial |
| 49 | go-sdk | [Three methods served before the initialize handshake](#three-methods-are-served-on-a-legacy-session-before-the-initialize-handshake) | Yes, [#1271](https://github.com/modelcontextprotocol/go-sdk/issues/1271) | Yes, [#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273), merged | **Yes, unreleased** | No | None taken |
| 50 | go-sdk | [The negotiated version is recorded on one path of four](#the-negotiated-protocol-version-is-recorded-on-one-path-of-four) | Yes, [#1272](https://github.com/modelcontextprotocol/go-sdk/issues/1272) | Yes, [#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274), merged | **Yes, unreleased** | No | None taken |
| 51 | client-go | [A WithOptions delegation sends `null` as the request body](#a-withoptions-delegation-sends-null-as-the-request-body) | No | No | No | No | None taken |
| 52 | client-go | [`UpdatePackageProtectionRulesOptions` lacks `omitempty`](#updatepackageprotectionrulesoptions-sends-two-explicit-nulls-on-every-partial-update) | No | No | No | Partly | Partial |
| 53 | gitlab-org/gitlab | [No endpoint reports the instance plan to a non-administrator](#no-endpoint-reports-the-instance-plan-to-a-non-administrator) | Yes, [gitlab-org/gitlab#630305](https://gitlab.com/gitlab-org/gitlab/-/issues/630305) | Yes, [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936), open | No | No | Yes |
| 54 | client-go | [Seven more option structs send an optional param on every call](#seven-more-option-structs-send-an-optional-param-on-every-call) | No | No | No | Not measured | None |
| 55 | gitlab-org/gitlab | [A permission refusal is answered 401 rather than 403](#a-permission-refusal-is-answered-401-rather-than-403) | No | No | No | No | Yes |
| 56 | gitlab-org/gitlab | [Deleting an external status check without the role answers 204 and deletes nothing](#deleting-an-external-status-check-without-the-role-answers-204-and-deletes-nothing) | No | No | No | No | Partial |
| 57 | gitlab-org/gitlab | [Creating an external status check without the role answers 500](#creating-an-external-status-check-without-the-role-answers-500) | No | No | No | No | Yes |

States verified against the upstream trackers on 2026-09-12, and rows 8 to 23
again on 2026-09-13 when the go-sdk batch was filed. Rows 39 to 44 were added
on the 12th: each entry existed with its five fields and the table had never
listed it, which is the drift this table exists to prevent. Rows 45 and 46 are
what the e2e rebuild found, the first from the EE port and the second from the
CE coverage that closed the gap against the old suite's baseline. Row 47 was
added on the 14th and is the first entry not found from this codebase, on the
terms the next paragraph sets out.

Re-verified in full on 2026-09-22, every merge request, pull request and issue
the file links, against the trackers rather than against memory. One row moved:
`modelcontextprotocol/go-sdk#1274` merged on the 21st and row 50 still read
open. Four of the nine
documentation merge requests had also landed since the last check, which is
recorded in row 34's section rather than in the table, since that row counts
the client-go structs and not the pages. A merged pull request was held to the
tags that contain its merge commit rather than to its merge date, which is the
rule the client-go section already states and which matters here: go-sdk
v1.8.0 was tagged on 2026-09-14 and contains **none** of the six merges,
`modelcontextprotocol/go-sdk#1242` from the 6th included, so every one of them
is merged and unreleased.

Every `client-go` row was then re-read against the **v3.12.0** source on
2026-09-19, when the pin moved there, rather than against the tracker: for each
one the struct, the method or the route it names was opened in the module cache
and compared with what the row claims. Only row 34 changed, and only for the
twelve of its fourteen that had merged. Rows 5, 6, 7, 19, 20, 22, 25, 26, 27,
28, 29, 30, 31, 32, 33, 35, 36, 37, 40, 41, 42, 43, 44 and 45 are unchanged,
which the diff of the two versions says structurally as well: twenty-one
non-test files differ between v3.0.0 and v3.12.0, no exported symbol was
removed or renamed, no field changed its Go type or its `omitempty`, and no
route a service method builds changed. Row 52 was added on the 19th as well,
and is read against that same v3.12.0 source.

Row 53 was added on the 22nd. It is the register's first entry opened upstream
as a feature rather than a defect, and the only one whose evidence is a
measurement against two running instances rather than a reading of source.

Re-verified on 2026-09-24 against the trackers and the tags: every merge
request, pull request and issue the file links, and every merge request and
issue the maintainer's gitlab.com account has opened, in any state. That second
pass found the closed
[gitlab-org/api/client-go!3033](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3033)
to
[gitlab-org/api/client-go!3039](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3039),
which the file had never named and row 34's section now does. Row 38 moved:
[gitlab-org/gitlab!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698)
is in `v19.4.0-ee` and `v19.4.1-ee`, the first `gitlab-org/gitlab` row of this
table to reach a release, so its declaration now waits only on the live record
being taken again from a 19.4 image, which is deferred until the work in flight
has landed. Row 8 moved as well: the keep-alive option it asked for was merged
upstream by somebody else, for the listen stream alone, and ours follows on the
shape agreed on the issue. Rows 8 and 14 now count another user's merged pull
request as In review, and row 14 the issue that pull request fixed as Reported,
which is the reading row 9 already took and the one the Merged field implies.
Row 17 had read In review on the strength of its open issue, which the schema
does not count, and now reads no, since no fix has been opened upstream. Rows
35 to 37 and 41 to 44 now read Reported through the umbrella issue, whose field
lists carry a block for each of those structs,
which is the reading row 34 already took; the table had said no since the rows
were added. The sections of rows 15 and 21 each carried two Merged bullets that
contradicted each other, and row 14's still named v1.8.0-pre.2 as the newest
tag; all three are corrected. client-go v3.13.0, tagged on 2026-09-21, adds
nothing any row waits on, so the v3.12.0 pin still carries every client-go merge
the file records; go-sdk v1.8.0 is still that SDK's newest tag, so every go-sdk
merge here is still unreleased. Rows 34, 46 and 53 keep their fields, row 39's
In review cell now records the approval, and the four sections now carry the
state as of the end of that day, most of which is what was done upstream the
same day: the writer's suggestions applied on row 53's merge request, the
nudge thread and a GitLab Duo finding resolved on
[gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540),
the router snapshot merged and the reviewer's suggestion applied on row 46's,
and the umbrella issue's description rewritten. No open merge request the
file follows has merged since, and each now waits on its reviewers, or on
another merge request, rather than on us.

## GitLab (`gitlab-org/gitlab`)

### No endpoint reports the instance plan to a non-administrator

- **Reported**: yes,
  [gitlab-org/gitlab#630305](https://gitlab.com/gitlab-org/gitlab/-/issues/630305),
  opened 2026-09-22 with the measurements below. A GitLab team member routed
  it to `group::entitlements` on 2026-09-23 (`Category:Plan Provisioning`),
  and it carries no milestone.
- **In review**: yes,
  [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936),
  open, from this repository's maintainer. The same team member routed it with
  the issue and set milestone 19.5 on it, which is a target and not a release.
  It was readied on 2026-09-24 with a bare `@gitlab-bot ready`, the form every
  earlier backend merge request here had been readied with, except
  gitlab-org/gitlab!254698, which went through the contributor platform's
  review request: the bot requested a backend coach, and seconds later the
  documentation automation requested the page's technical writer and took the
  coach off the reviewer field in the same step. Read the same day: the fork
  pipeline on `b183f4fa` is green, it has no conflicts, and it has no approval
  yet against the six maintainer code-owner rules its files fall under. The
  bot's reply also turned the ready into an unresolved thread. The writer
  reviewed it the same day, at 11:03 UTC, with four suggestions that reword the
  `plan` description in the GraphQL type, the GraphQL reference and both
  introspection files. At 11:16 UTC the writer requested the backend coach
  again, so the reviewer field now carries both. All four suggestions were
  applied the same day in one commit, `83c6e83ebe63`, each thread answered
  "Applied in 83c6e83ebe63.", and the ready's thread was resolved; the four
  suggestion threads are left for the writer to resolve. As of 2026-09-24 it
  waits on the reviewers, the writer and the backend coach, and nothing is
  pending on us.
- **Merged**: no.
- **Blocking**: no. The tier can always be pinned with `--tier` or
  `GITLAB_MCP_TIER`, which skips detection entirely.
- **Workaround**: yes, the detection cascade in `internal/gitlab/client.go`.
  `GET /license` first, then the namespace plans, then Free with a warning.
  The namespace step answers only on gitlab.com, where a namespace plan is the
  subscription; on a self-managed instance it reads `default` for everyone, so
  a non-administrator there falls through both steps to Free, and that caller
  is the one the merge request is for. Once a release carries it, a step
  reading `plan` from `GET /metadata` answers that caller, which needs
  client-go's `Metadata` struct (`version`, `revision`, `kas` and `enterprise`
  in v3.12.0) to gain the field, or a captured-response read under
  [ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)
  until it does.

**Where**: `GET /api/v4/license`, the only endpoint that reports the plan an
instance is licensed for.

**What**: it requires an administrator. Measured on 2026-09-21 against a
licensed GitLab EE 19.3.1 in Docker with an administrator and an ordinary user
on the same instance, and against gitlab.com with an ordinary account:

| Endpoint          | Administrator             | Ordinary user             |
| ----------------- | ------------------------- | ------------------------- |
| `GET /license`    | `200`, `plan: ultimate`   | `403 Forbidden`           |
| `GET /version`    | `200`, `enterprise: true` | `200`, `enterprise: true` |
| `GET /namespaces` | `200`, `plan: default`    | `200`, `plan: default`    |
| `GET /features`   | not asked                 | `403 Forbidden`           |

On gitlab.com the namespace plans carry real values for a namespace the caller
administers, under the `can_admin_namespace || has_gitlab_subscription`
condition in `ee/lib/ee/api/entities/namespace.rb`. On a self-managed instance
they read `default` for everyone, because a namespace subscription is a
gitlab.com concept, so they say nothing about the instance licence there.

**Consequence for us**: `DetectTier` resolved Free for every non-administrator
credential, so a caller on an Ultimate self-managed instance was served the
Free catalogue with nothing in the listing explaining what was withheld. That
is [issue 899](https://github.com/jmrplens/gitlab-mcp-server/issues/899).

**What is proposed**: a `plan` field added to the instance metadata that is
already served to every authenticated caller, in its REST and GraphQL forms
alike. It would report the licence plan, `free` where there is no licence, and
nothing where subscriptions are held per namespace rather than per instance.
Nothing commercial would move: the licensee, the seats, the expiry and the
subscription identifier stay behind the administrator check on `/license`.
The merge request is open and unmerged, so none of this is served yet. Two issues have
asked for this since 2020,
[#247915](https://gitlab.com/gitlab-org/gitlab/-/issues/247915) and
[#219732](https://gitlab.com/gitlab-org/gitlab/-/issues/219732), neither with a
design objection recorded and both with their owning group archived.

**Effort**: small, following the CE and EE split
`Gitlab::Tracking::StandardContext` already uses for exactly this. The merge
request is seventeen files: five source files; five spec files and the
response schema fixture the request spec validates against; four under `doc/`,
the page, the GraphQL reference and both OpenAPI documents; and the two
regenerated GraphQL introspection files.

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

### Two project group listings are annotated with the whole Group entity

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699),
  open and approved: by a technical writer, by a reviewer, and on 2026-09-15 by
  the backend maintainer @hustewart, who set auto-merge the same day. It has
  not merged because the pipelines since have failed on breaks from `master`
  rather than on the change, the last of them the fork pipeline of the
  2026-09-22 rebase, whose one blocking failure was
  `generate-apollo-graphql-schema` on a schema clash that `master` fixed
  twelve minutes later, when gitlab-org/gitlab!256904 merged, after the rebase
  had landed on `e52599d0`. Auto-merge is no longer set. A follow-up of 2026-09-24 said so and
  asked @hustewart for a fresh pipeline and auto-merge; posted as a reply
  under our own note of 2026-09-22, it turned that note into an unresolved
  thread and so added a `DISCUSSIONS_NOT_RESOLVED` blocker to a merge request
  that had none, and we resolved the thread an hour later. It now fails only
  `CI_MUST_PASS`, and waits on @hustewart to start a pipeline in the canonical
  project and set auto-merge again.
- **Merged**: no. Its milestone still says 19.4, which was released without
  it, so 19.5 is the earliest release that can carry it.
- **Blocking**: no. Our output type is already the right shape; only the audit
  was misled.
- **Workaround**: yes, a declaration. `cmd/audit_1to1/internal/paths/sent_declarations.go`
  answers the 49 findings this raised against `projects.ProjectGroupOutput`
  under `documented-response-is-not-the-one-sent`. It retires when
  `cmd/gen_api_live` is run against a release that carries the merge, since
  the stale-declaration check then fails on it.

**Where**: `lib/api/projects.rb`, the `desc` blocks for
`GET :id/share_locations` and `GET :id/invited_groups`.

**What**: both descriptions say `success Entities::Group`, and both handlers
call `present_groups`, defined a few hundred lines above in the same file,
which presents `with: Entities::PublicGroupDetails`. `PublicGroupDetails` is
`BasicGroupDetails` plus `avatar_url`, `full_name` and `full_path`, so six keys
in total, against roughly seventy for `Group`.

Their sibling `GET :id/groups` calls the same helper and is annotated
`Entities::PublicGroupDetails`, correctly, so three adjacent routes share one
helper and two of them disagree with it.

**Confirmed against the API**, not by reading alone. All three endpoints answer
with exactly six keys on GitLab.com: `id`, `name`, `avatar_url`, `web_url`,
`full_name`, `full_path`.

**Root cause**: the same class as the three job token scope annotations
recorded above. A `desc` block naming an entity the handler does not present is
invisible to every test, because Grape uses it for documentation only.

**How we found it**: the R-PATH sent dimension read `Entities::Group` off the
route annotation and reported all 49 of that entity's fields as missing from
`ProjectGroupOutput`, which publishes the six the endpoint really sends. It was
the largest single block left in the backlog, 49 of 117, and it was not work at
all.

**Effort**: small. One line in `lib/api/projects.rb` plus the regenerated
OpenAPI document, where the change is a single `$ref`, because
`APIEntitiesPublicGroupDetails` is already a component of the document.

## GitLab client (`gitlab.com/gitlab-org/api/client-go`)

### Panic unmarshalling an issue with no id

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!3006](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3006).
- **Merged**: **yes**, on 2026-08-25 into `main`, shipped in **v2.59.1**.
- **Blocking**: it was. The panic took the process down rather than failing one
  call.
- **Workaround**: retired. The local guard went with the move to v2.62.0, and
  every v3 tag carries the fix as well; the pin is now `client-go/v3` v3.12.0.

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
- **Workaround**: yes, in two places. `internal/tools/features.Set` builds the
  request body itself, and so does `fixture.setFeature` in the end-to-end
  suite, which pins a flag as a scenario's precondition.

**What**: the option struct's fields carry no `omitempty`, so empty strings are
serialized and GitLab rejects the request with a "mutually exclusive" error.

**Reach**: every call, not a corner. Grape counts a param that is present as
given, so the body's empty `key`, `feature_group` and `user` collide whatever
the caller asked for: GitLab answers `400 {error: key, feature_group are
mutually exclusive, key, user are mutually exclusive}` for a plain
instance-wide set. The method is therefore unusable as shipped, which is what
made the second workaround necessary: the fixture called it and the licensed
suite failed on the flag it was setting rather than on its own subject.

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

- **Reported**: yes, one merge request per struct, linked below, plus one
  umbrella issue,
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which publishes the method and the whole measurement and answers
  [issue 2269](https://gitlab.com/gitlab-org/api/client-go/-/issues/2269),
  where the maintainers had said there was no good way to detect this drift.
  Every merge request references it with a non-closing `Related to`, so the
  first merge does not close the umbrella.
- **In review**: two are open, both with a green pipeline and no conflicts.
  [gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
  (the seven `Hook` fields) is approved and mergeable: a reviewer approved it
  on 2026-09-14 and again on 2026-09-16, after the test push that reset the
  first approval, both of its threads are answered and resolved, one of them
  by adding the `custom_webhook_template` assertion to the edit test, and the
  reviewer handed the maintainer review to @fforster on 2026-09-14, who has
  not answered since. A note of 2026-09-24 in the reviewer's documentation
  thread records that the documentation half,
  [gitlab-org/gitlab!254538](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254538),
  merged and is live, and asks him for that review.
  [gitlab-org/api/client-go!3052](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3052)
  (the `Package` fields) was handed to @PatrickRice on 2026-09-14. He asked on
  2026-09-16 whether `CreatorID` should be a primitive rather than a pointer,
  which it became the same day (`2cc7e540`), and that thread, still unresolved
  and his to close, is the one thing between it and his approval. It was not
  asked again from here on 2026-09-24, because the bot had reminded him of it
  that morning.
- **Merged**: `gitlab-org/api/client-go!3042` (`BroadcastMessage.Color`) in
  **v3.1.0**, tagged on 2026-09-09 eighteen minutes after the merge; then
  `gitlab-org/api/client-go!3040` (`Appearance.SiteName`) and
  `gitlab-org/api/client-go!3046` (the `GroupSCIMIdentity` json tag) in
  **v3.2.0** the same day; then `gitlab-org/api/client-go!3043`
  (`Agent.IsReceptive`) and `gitlab-org/api/client-go!3045`
  (`SecureFile.FileExtension`) in **v3.3.0**, and
  `gitlab-org/api/client-go!3047` (`GroupServiceAccount.PublicEmail` and
  `UnconfirmedEmail`) in **v3.4.0**, all on 2026-09-10; then
  `gitlab-org/api/client-go!3053` (the four `Snippet` fields) in **v3.5.0**
  and `gitlab-org/api/client-go!3049` (`LastUsedAt` and `UsageType` on both
  deploy key structs) in **v3.6.0**, both on 2026-09-11; then
  `gitlab-org/api/client-go!3044` (`LicenseTemplate.Popular`) in **v3.7.0**
  and `gitlab-org/api/client-go!3041` (`Topic.OrganizationID`) in **v3.9.0**,
  merged on 2026-09-12 and 2026-09-13; then `gitlab-org/api/client-go!3050`
  (`Imported`, `ImportedFrom` and `WikiPage` on both event structs) in
  **v3.10.0** on 2026-09-14; and finally `gitlab-org/api/client-go!3051` (the
  eight `Namespace` fields) in **v3.11.0** on 2026-09-16.
  `gitlab-org/api/client-go!3051` was merged on the 14th, nineteen minutes
  after v3.10.0 was cut, and this register recorded it as in no tag until the
  next release carried it. Do not read a merge as a release:
  `gitlab-org/api/client-go!3040` sat merged and in no tag for hours, so the
  version is read from which tags contain the merge commit rather than from the
  newest tag. Eleven releases in eight days is why: the newest tag was wrong
  for five of the first six, and when this was first written
  `gitlab-org/api/client-go!3044` was in three tags while
  `gitlab-org/api/client-go!3041` was in one.
- **What review asked for, and what it cost**: `gitlab-org/api/client-go!3051`
  was merged on the second push. A maintainer asked for the five numeric
  fields as plain `int64` rather than pointers, on the convention that a response struct uses
  primitives unless a null carries something the zero value does not, and the
  three date fields stayed pointers. The merge request's own reasoning had
  argued the opposite, so the description was corrected along with the code
  rather than left contradicting it. One case is worth keeping in mind here,
  and is recorded on the merge request: a null `shared_runners_minutes_limit`
  is how GitLab says there is no limit, which as an `int64` reads as a limit of
  zero. It is only sent to a caller allowed `:update_subscription_limit`, who
  is reading the namespace to set those limits rather than to enforce one.
- **Blocking**: no.
- **Workaround**: retired for the twelve that merged, at the **v3.12.0** pin.
  Each field was read from the captured response beside the SDK's decode,
  through the readers in `internal/toolutil/sent_shapes.go`; the pin was
  deliberately not moved once per merge, since these landed in quick
  succession, so the bump and the workarounds it retires were taken together.
  Ten packages now read the field straight off the SDK struct and their capture
  is gone (`appearance`, `broadcastmessages`, `clusteragents`, `events`,
  `groupscim`, `groupserviceaccounts`, `licensetemplates`, `securefiles`,
  `snippets`, `topics`), and with them the shapes and readers they used.
  Two retire only in part, each for a reason the merge request could not carry:

  - `deploykeys` takes `last_used_at` and `usage_type` from the SDK on both
    structs, and keeps a capture for `projects_with_write_access` and
    `projects_with_readonly_access`, which client-go models on
    `InstanceDeployKey` alone. That half was never
    `gitlab-org/api/client-go!3049`'s: the bullet below
    records why `ProjectDeployKey` must not gain them.
  - `namespaces` takes `projects_count`, `root_repository_size`,
    `additional_purchased_storage_ends_on`, `max_seats_used_changed_at` and
    `end_date` from the SDK, and keeps a capture for the three limits
    `shared_runners_minutes_limit`, `extra_shared_runners_minutes_limit` and
    `additional_purchased_storage_size`. Review asked for those as plain
    `int64` rather than pointers, so the SDK cannot tell a null from a zero,
    and a null `shared_runners_minutes_limit` is exactly how GitLab says there
    is no limit. Reading them from the struct would publish "no limit" as
    "limited to zero minutes", which is the one way this bump could have made
    the surface less true rather than more.

  The two open merge requests keep their workarounds whole: `systemhooks`
  still reads the seven `Hook` fields of `gitlab-org/api/client-go!3048` off
  the capture, and `packages` the `Package` fields of
  `gitlab-org/api/client-go!3052`. Both were checked against the v3.12.0
  source rather than against the tracker, and neither struct carries them.
  `projectserviceaccounts` keeps its read of `public_email` too, since
  `gitlab-org/api/client-go!3047` added the pair to `GroupServiceAccount` and
  `ProjectServiceAccount` was outside it.

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
- `GroupServiceAccount` (`group_serviceaccounts.go`) has neither
  `public_email` nor `unconfirmed_email`. All four group service account
  endpoints render `Entities::ServiceAccount`, which inherits `UserSafe`:
  [lib/api/entities/user_safe.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/user_safe.rb)
  exposes `public_email` on line 10 unconditionally, and
  [lib/api/entities/service_account.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/service_account.rb)
  exposes `unconfirmed_email` on line 7 when one is pending.
  [gitlab-org/api/client-go!3047](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3047).
  This one is worth reading for what it says about the SDK rather than about
  the field: the same GitLab entity is modelled by three structs, and they
  disagree. The project-scope `ServiceAccount` already carries
  `unconfirmed_email` while the group-scope one carries neither, which is why
  `internal/tools/projectserviceaccounts` reads that key from the SDK and
  `internal/tools/groupserviceaccounts` reads it from the capture. Two gaps
  next to it are not covered by that merge request and are worth a second:
  `ProjectServiceAccount` and the instance-scope `ServiceAccount` are both
  missing `public_email` still. The second half of that lead is closed:
  `GroupsService.GetServiceAccount` and
  `ProjectsService.GetProjectServiceAccount` wrap
  `GET /…/service_accounts/:user_id` as of **v3.12.0**
  ([!3056](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3056),
  somebody else's), which this server does not yet publish an action for. That
  is new surface for the 1:1 review (R-ACTION) rather than a workaround.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), whose
oracle is `docs/development/gitlab-api-live.json`, a record of what a booted
GitLab says each of its Grape entities exposes. Each finding carries
`sdk_models: false`, which is what says the gap is upstream rather than ours.

**Effort**: trivial per struct. One additive field with a `json` tag, and one
key added to the fixture of an existing test.

**The gap is usually in GitLab's own documentation too, and that is a second
merge request.** A code owner asked for it on
[!3045](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3045)
rather than opening it himself, so cross-checking all eight fields against
`doc/api/` was worth doing: two of the eight, `file_extension` and
`is_receptive`, appeared nowhere on their page, and a third page showed
`public_email` in none of its fourteen example responses. Nine documentation
merge requests have gone to `gitlab-org/gitlab` from its own
[community fork](https://gitlab.com/gitlab-community/gitlab-org/gitlab):
[!254507](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254507),
[!254511](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254511),
[!254519](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254519),
[!254538](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254538),
[!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540),
[!254542](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254542),
[!254543](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254543),
[!254547](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254547) and
[!254552](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254552).
Eight are merged into `master`: `gitlab-org/gitlab!254507` on 2026-09-10,
`gitlab-org/gitlab!254519` on 2026-09-11, `gitlab-org/gitlab!254511` and
`gitlab-org/gitlab!254542` on 2026-09-14 and 2026-09-15,
`gitlab-org/gitlab!254538`, `gitlab-org/gitlab!254543` and
`gitlab-org/gitlab!254547` together on 2026-09-22, and
`gitlab-org/gitlab!254552` (the snippet clone URLs) on 2026-09-23. Held to the
tags that contain each merge commit, read on 2026-09-23 and again on
2026-09-24, three of them have shipped: `gitlab-org/gitlab!254507`,
`gitlab-org/gitlab!254511` and `gitlab-org/gitlab!254519` are in `v19.4.0-ee`
and `v19.4.1-ee`, and the other five are in no tag yet. Until the first of
those reads the paragraph said none had shipped, a claim that had not been
checked against the tags. `gitlab-org/gitlab!254538` is the one whose merge had
been blocked by a `pre-merge-checks` race rather than by anything in the
change.

The one still open is
[gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540)
(the deploy key fields), in no milestone and no tag, and what held it up was
how it was readied rather than anything in it. The machine-learning labeller
had put the `Technical Writing` label on it on 2026-09-10, before the first
ready. On a documentation-only merge request a bare `@gitlab-bot ready`
requests no coach when `.gitlab/CODEOWNERS` names a writer for the page, and
leaves the writer to the automation that asks one, which does nothing once
that label is present. Its three bare readies, on 2026-09-12, 2026-09-14 and
2026-09-22, therefore set `workflow::ready for review` and asked nobody, and
`gitlab-org/gitlab!254538` and `gitlab-org/gitlab!254552`, readied twice the
same way, waited ten days until the bot's inactivity nudge of 2026-09-22 drew
a reviewer to each. On 2026-09-24 it was readied naming the writer
`.gitlab/CODEOWNERS` lists for `doc/api/deploy_keys.md`, @rsarangadharan, who
was on the reviewer field within seconds. The same day it gained one commit,
`ce04dc8e`, which makes `last_used_at` null in the `POST /deploy_keys`
example: that call always creates the key it answers with, so the key has
never been used, and the example had shown a timestamp eleven days after the
key's own `created_at`. Its fork pipeline on that commit is green. The
inactivity-nudge thread the ready was posted in, its one failing check, was
resolved the same day. The writer then asked GitLab Duo to check the change's
technical accuracy, and its one finding, an `expires_at` older than the
`last_used_at` beside it in another deploy key example, was applied in
`b2b900f7271b` and answered in its thread, and every thread was resolved again.
As of 2026-09-24 it needs no further approval and waits on the writer.

`.github/skills/upstream-contribution/SKILL.md` carries the procedure and the
traps: every example on a page rather than the one that prompted it, the
response attribute tables as well as the examples, the other entities sharing
the page that must not gain the field, and the ready that names the page's
writer.

Three of those nine fixed a documentation defect found on the way rather than
the field that prompted them: three `fingerprint_sha256` keys on the deploy
keys page had lost their name and sat as `""`, one snippet example's `raw_url`
pointed at a different snippet, and the packages page showed a `pipelines`
array inside `versions` where the entity exposes a single `pipeline` and omits
`tags`.

**The second batch, and the cadence a maintainer asked for.** Six more merge
requests carry the structs behind the fields this server published in the
member and Geo tranches:
[!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
(`Hook`),
[!3049](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3049)
(both deploy key structs),
[!3050](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3050)
(both event structs),
[!3051](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3051)
(`Namespace`),
[!3052](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3052)
(`Package`, plus the `GetProjectPackage` wrapper the versions field needs and
which the library did not have) and
[!3053](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3053)
(`Snippet`).

On the last of those, a code owner asked to stop opening one merge request per
struct: keep an issue with the missing fields, update it, and send larger merge
requests once a chunk is pre-approved, so three maintainers validate a chunk
rather than a stream. That is now the rule, and the umbrella issue they were
asking for already existed as issue 2300, referenced from every one of these in
a `Related to` line at the bottom where a reviewer never looks.

They settled the chunks on the umbrella itself, on 2026-09-09 and 2026-09-10:
no list maintained upstream and no generator, but one merge request carrying
every remaining field this server consumes, after which contributions go back
to being made on demand like anybody else's. That merge request is agreed and
not yet opened, and it waits until release 3.1.0 of this server is published.
The umbrella's description was brought up to date on 2026-09-24: its title now
gives the distinct count, 897 fields, its table of merge requests reads as the
tracker does, it counts the fields that have landed since the measurement, and
it says the rest will follow as that one merge request without saying when.

**Four findings from that batch that are not merge requests**, because sending
them would have been wrong:

- `projects_with_write_access` and `projects_with_readonly_access` are gated on
  presenter options that `lib/api/deploy_keys.rb` passes on `GET /deploy_keys`
  alone, so the project-scope routes never send them. `InstanceDeployKey` is
  already correct and `ProjectDeployKey` must not gain them.
- `Package.project_id` and `project_path` are already modelled, on
  `GroupPackage`, which embeds `Package` and is what `ListGroupPackages`
  returns. Adding them to `Package` would shadow those.
- `ContributionEvent.Title`, `ProjectEvent.Title` and `ProjectEvent.Data` are
  phantoms: `API::Entities::Event` exposes no `title` and no `data`. Removing
  them is a breaking change, so it is recorded rather than done.
- `PackagePipeline` is missing `iid`, `project_id` and `source` of the eleven
  keys `API::Entities::Package::Pipeline` exposes.

**Three more gaps are recorded and not yet sent**, held back by the batching
the maintainer asked for above. Each is a field this server now reads from the
captured response, so each carries a live workaround:

- `PendingInvite` has no `invite_token`, which
  `lib/api/entities/invitation.rb` exposes with no condition. The same struct
  declares an `ID` the entity does not expose, which the audit already reports
  as a phantom in the other direction, so one merge request settles both.
- `BillableGroupMember` has no `public_email` and no `locked`, both exposed
  unconditionally through the `UserBasic` that
  `ee/lib/api/entities/billable_member.rb` inherits (`locked` through
  `access_locked?`).
- `AccessRequest` is missing six unconditional fields, `public_email`,
  `locked`, `avatar_url` and `web_url` from the merged `UserBasic`, and
  `expires_at` and `membership_state` from the `Member` that
  `lib/api/entities/access_requester.rb` inherits. The conditional ones,
  `created_by`, `email`, both identities, `override` and `member_role`, belong
  in the same merge request as a second group.

That lead has since been measured and is
[its own entry](#memberrole-models-twenty-of-the-forty-five-permissions-gitlab-sends):
the record's `API::Entities::MemberRole` carries 50 keys, of which 45 are
permissions, and `gl.MemberRole` models 20 of them.

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
from the community fork and closing the old with a note. The first seven went
through exactly that:
[gitlab-org/api/client-go!3033](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3033)
to
[gitlab-org/api/client-go!3039](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3039),
opened from the personal fork on the morning of 2026-09-09, were closed within
half an hour in favour of the first seven linked in the list above, one for
one and each with the same change, and
[gitlab-org/api/client-go!3047](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3047)
was opened from the community fork from the start.

### Ten modelled fields that no Grape entity exposes, removed from this server's output

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: not needed. This server simply stopped publishing them. The
  cost is that `audit_1to1`'s R-OUTPUT diff now reports ten `missing_output`
  rows against these SDK structs, which is this entry's reason for existing:
  they are answered here, not gaps to fill.

**What**: ten struct fields the SDK models, on structs this server's output
types pair with, that no Grape entity renders. They carry eight distinct names
across five structs; `group_mention_events` and its confidential twin are
declared on `Integration` and again on `GroupDatadogIntegration`, which is why
the field count and the name count differ. Every one of them was published
**without `omitempty`**, so each asserted a value on every response rather than
being merely dead:

| SDK struct                               | Field                                                       | What we emitted                                                                                        |
| ---------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `Integration`, `GroupDatadogIntegration` | `group_mention_events`, `group_confidential_mention_events` | `false` on every integration listed                                                                    |
| `Project`                                | `ci_opt_in_jwt`                                             | `false` on the busiest output type this server has                                                     |
| `PendingInvite`                          | `id`                                                        | `0`, and the Markdown rendered it as `(ID: 0)`                                                         |
| `WeightEvent`                            | `resource_type`, `resource_id`, `state`                     | `""`, `0` and `""`, and two of them rendered a whole Markdown column as an empty type followed by `#0` |
| `BasicMergeRequest`                      | `label_details`                                             | `null` on every related merge request row                                                              |

**Evidence**, against the 19.4.0-pre tree at `/opt/gitlab-source/gitlab`:

- `group_mention_events` and its confidential twin appear nowhere under
  `lib/api` or `ee/lib/api`. They are columns on `web_hooks` that no entity
  renders.
- `ci_opt_in_jwt` appears nowhere under `lib/api`, `ee/lib/api` or `doc/api`.
- `lib/api/entities/invitation.rb` exposes exactly `access_level`,
  `created_at`, `expires_at`, `invite_email`, `invite_token`, `user_name` (if
  the member has a user) and `created_by_name`. There is no `id`. The list
  example in `doc/api/invitations.md` prints one, which Grape cannot have
  produced.
- `ee/lib/api/entities/resource_weight_event.rb` exposes exactly `id`, `user`,
  `created_at`, `issue_id` and `weight`. Its siblings (iteration, state and
  milestone events) do expose `resource_type` and `resource_id`, which is
  where the SDK's three came from; the weight entity is the odd one out.
- `label_details` is not a key GitLab sends at all: `merge_request_basic.rb`
  renders the `labels` array as `LabelBasic` objects when
  `with_labels_details` is set. `MergeRequest.UnmarshalJSON` re-keys that into
  `label_details`, and it is defined on `MergeRequest` only. The two endpoints
  behind `issues.RelatedMROutput` return `[]*BasicMergeRequest`, which has the
  field and no unmarshaller, so it could never be filled.

**Effort**: small per field and breaking for the SDK, since removing a public
field is a signature change. `PendingInvite.id` travels with the
`invite_token` gap already recorded above, so one merge request settles that
struct in both directions. The rest are candidates for the v4 line rather than
work to do now, which is why they are recorded rather than sent.

**A second batch followed**, of fields that were dead rather than fabricated:
each carried `omitempty`, so nothing was asserted, but each advertised an
output field that could never be filled. Nine more `missing_output` rows come
from these, plus four in the R-INPUT direction:

| SDK struct                          | Field                     | Why no response carries it                                                                                                                                      |
| ----------------------------------- | ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ContributionEvent`, `ProjectEvent` | `title`                   | `lib/api/entities/event.rb` exposes `target_title` and no `title`                                                                                               |
| `ProjectEvent`                      | `data`                    | the same entity carries a push payload under `push_data`; `doc/api/events.md` still prints `"data": null` from the API v3 era                                   |
| `Project`                           | `build_coverage_regex`    | absent from `lib/api`, `ee/lib/api` and `doc/api` alike, as a parameter as well as a field                                                                      |
| `Project`                           | `operations_access_level` | GitLab renamed it to `monitor_access_level`, which the entity exposes and `projects_helpers.rb` accepts; the old name survives only as a database column        |
| `Issue`                             | `issue_link_id`           | belongs to `API::Entities::RelatedIssue`, which only `GET /projects/:id/issues/:iid/links` presents, decoded into `IssueRelation` and served by another package |
| `PipelineTrigger`                   | `deleted_at`              | `lib/api/entities/trigger.rb` exposes eight keys without it, and `ci_triggers` has no such column                                                               |
| `ReleaseLink`                       | `external`                | removed from the API in 16.0 per `doc/update/deprecations.md`; the 19.4 entity has no trace of it                                                               |
| `AuditEvent`                        | `event_type`              | the entity sends `event_name`, which this server already publishes                                                                                              |

Two of that batch produce no audit row at all, because the SDK never modelled
them either: `two_factor_enabled` on a project member, which
`config/authz/roles/owner.yml` grants at group scope alone, and a job's
`source`, which this server was reading from a raw fetch on the strength of a
`doc/api/jobs.md` example that prints several keys no entity exposes.

`build_coverage_regex` and `operations_access_level` are the only two removed
from the **request** as well: GitLab accepts neither, so offering them was
inviting a caller to send a parameter that is discarded.

### The Geo structs model a fraction of a site and its status, and the repair method names the wrong entity

- **Reported**: in part, in the Geo block of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which names the site's four missing settings and explains the status matrix.
  `repositories_count`, `storage_shards`, the `namespaces` type and the
  `RepairGeoSite` return type are not in it. No merge request of its own, held
  back by the batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no, except for the third item below, which makes two endpoints
  unusable on any site with selective sync by namespace.
- **Workaround**: partial. The site's four missing fields, the status's
  `repositories_count` and `storage_shards`, and the whole per-replicable
  matrix are read from the captured response beside the SDK's decode
  (ADR-0021), through the readers in `internal/tools/geo/sent_shapes.go`.
  Neither the `namespaces` type nor the repair return type can be worked
  around without leaving the SDK method behind, so both stand.

**What**: four gaps in client-go v3.0.0's `geo_sites.go`, measured against the
`v19.3.1-ee` entities the committed live record was taken from.

- `GeoSite` carries 19 of the 23 keys
  [ee/lib/api/entities/geo_site.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/api/entities/geo_site.rb)
  exposes. It has no `blob_download_timeout`, no
  `checksum_mismatch_report_threshold` and no
  `checksum_mismatch_self_heal_cooldown_minutes`, each an `Integer` exposed
  with no condition at all, and no `selective_sync_organization_ids`, exposed
  when `::Gitlab::Geo.geo_selective_sync_by_organizations_enabled?`. The
  create and edit option structs are missing the same settings, so the SDK can
  neither read them nor set them.
- `GeoSiteStatus` carries 211 of the 605 distinct keys
  [ee/lib/api/entities/geo_site_status.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/api/entities/geo_site_status.rb)
  exposes. Most of that entity is generated: it loops over
  `GeoNodeStatus::RESOURCE_STATUS_FIELDS`, which is thirteen metrics for each
  of the 44 replicator classes, flattened into key names, and the struct spells
  out fifteen of the 44 by hand and none of the per-replicable
  `oldest_unsynced_time`. It is also missing two singular keys,
  `repositories_count` and `storage_shards`. This is the one gap where a
  hand-written struct was never going to keep up: the field list is a function
  of which replicators the instance has enabled, and this server therefore
  publishes it as a map keyed by the replicable rather than as 573 fields.
- `GeoSiteStatus.Namespaces` is declared `[]string` and the entity exposes
  `namespaces, using: ::API::Entities::NamespaceBasic`, an array of objects.
  On a site with selective sync by namespace the SDK's own decode therefore
  fails and both `GetStatusOfGeoSite` and `ListStatusOfAllGeoSites` return an
  error instead of a status. Nothing here caught it either, because our own
  fixture spelled the array the way the struct does.
- `RepairGeoSite` returns `*GeoSite`, and `POST /geo_sites/:id/repair` is
  annotated `success Entities::GeoSiteStatus` and presents the site's status.
  Every field of the struct it hands back is therefore the zero value, and
  this server's `geo.repair` answers with a site GitLab did not send. It reads
  no captured extras for that reason, which is written down beside the call.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), whose
oracle is `docs/development/gitlab-api-live.json`. Geo was 1001 of its 1388
findings, and the repair return type is why 603 of those were filed against
`geo.Output`, a site type, under the status entity: the audit reads which
endpoints answer with `*GeoSite` out of the SDK's own source, and that method
puts the repair route in the set.

**Effort**: additive and small for the first item and for `repositories_count`
and `storage_shards`. The replicable matrix is a design question rather than a
field list, since the names depend on the instance. The `namespaces` type and
the `RepairGeoSite` return type are both breaking changes to exported API, so
they belong to a major version or to a new method beside the old one.

### The merge request structs miss six keys, unevenly, and two methods name an entity they do not answer with

- **Reported**: in part, as the `BasicMergeRequest` and
  `MergeRequestDependency` blocks of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which list the six keys and `blocked_merge_request`. Its `MergeRequest`
  block counts the keys of `ApprovalState` and `MergeRequestChanges` against
  that struct, which is the two return types below seen from the other side,
  without naming them as return types. No merge request of its own, held back
  by the batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Every gap is worked around, and the two return types cost
  this repository four declarations rather than a defect.
- **Workaround**: partial. The six keys are read from the captured response
  beside the SDK's decode (ADR-0021), through `toolutil.CapturedMergeRequest`
  and `CapturedMergeRequests` and, for the dependency, a shape local to
  `internal/tools/mergerequests`. The two return types cannot be worked around
  without leaving the SDK methods behind, so both stand and are answered by
  declarations in `cmd/audit_1to1/internal/paths/sent_declarations.go`.

**What**: three gaps in client-go v3.0.0's `merge_requests.go` and
`merge_request_approvals.go`, measured against the `v19.3.1-ee` entities the
committed live record was taken from.

- `BasicMergeRequest` carries 50 of the 55 keys
  [lib/api/entities/merge_request_basic.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/merge_request_basic.rb)
  exposes. It has no `merge_status`, no `reference`, no
  `approvals_before_merge` and no `work_in_progress`, all four exposed with no
  condition at all, and no `title_html` or `description_html`, which the entity
  exposes under the `render_html` presenter option. Four of the six are the
  older spelling of a key the entity also sends under a newer name, and GitLab
  keeps sending both, so a client reading a merge request through the SDK
  silently loses whichever half its caller expected.
- The gap is uneven between the two structs, which is what makes it a modelling
  bug rather than a deliberate omission: `MergeRequest` embeds
  `BasicMergeRequest` and adds `WorkInProgress` back, marked deprecated, so a
  single-merge-request GET carries the key and every list of the same objects
  does not. `BlockingMergeRequest`, a third struct over the same entity,
  carries all four of the unconditional ones. Three spellings of one response
  shape, agreeing on nothing.
- `MergeRequestDependency` carries `blocking_merge_request` and not
  `blocked_merge_request`, though
  [lib/api/entities/merge_request_dependency.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/api/entities/merge_request_dependency.rb)
  exposes both, each rendered with `MergeRequestBasic` and each sent to a
  caller allowed to read that merge request. Half of a dependency is therefore
  invisible to the SDK.
- Two methods declare a return type whose endpoint answers with something else.
  `MergeRequestApprovalsService.ChangeApprovalConfiguration` returns
  `*MergeRequest` and sends `POST /projects/:id/merge_requests/:iid/approvals`,
  which is annotated `Entities::ApprovalState`;
  `MergeRequestsService.GetMergeRequestChanges` returns `*MergeRequest` and
  sends `GET /projects/:id/merge_requests/:iid/changes`, annotated
  `Entities::MergeRequestChanges`. In both cases the struct the caller is
  handed can hold almost nothing of what arrives.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), whose
oracle is `docs/development/gitlab-api-live.json`. The merge request family was
51 of its findings, and the last item above is why 34 of those were filed
against merge request output types under two entities they never receive: the
audit reads which endpoints answer with a struct out of the SDK's own source,
and those two methods put the approval and changes routes in the set. Neither
endpoint is called anywhere in this repository, which is what settled them as
artefacts rather than gaps.

**Effort**: additive and small for the six struct fields, which is the same
change [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
already opened for another set of structs. The two return types are breaking
changes to exported API, so they belong to a major version or to a new method
beside the old one.

### The User struct models one user entity and GitLab serves six

- **Reported**: yes, as the `User` block of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which lists sixteen keys, each marked as sent always or under a condition:
  the fifteen below and `avatar_path`, which, as **How we found it** below
  says, no route sends. No merge request of its own, held back by the batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Every gap is worked around.
- **Workaround**: yes. The fifteen keys are read from the captured response
  beside the SDK's decode (ADR-0021), through `toolutil.CapturedUser` for the
  ten every route sends and `toolutil.CapturedInstanceUser` for those plus the
  five only the instance-wide routes reach.

**What**: `User` in client-go v3.0.0's `users.go` carries 48 keys and is the
one struct every user-returning method decodes into, measured against the
`v19.3.1-ee` entities the committed live record was taken from. GitLab serves
six different user entities through those methods, and the struct models the
union of none of them.

- Ten keys are missing from
  [lib/api/entities/user_public.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/user_public.rb)
  and the `User` it inherits, which together are what `GET /user`,
  `GET /groups/:id/enterprise_users`, `GET /groups/:id/provisioned_users` and
  `GET /groups/:id/saml_users` all answer with: `commit_email`,
  `preferred_language`, `discord`, `github`, `local_time`, `pronouns` and
  `work_information` are exposed with no condition at all, and `followers`,
  `following` and `is_followed` under `Ability.allowed?(current_user,
  :read_user_profile, user)`. Seven keys sent on every user of every one of
  those four endpoints, and a caller reading a user through the SDK sees none
  of them.
- `bio_html` is missing from
  [lib/api/entities/user_profile.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/user_profile.rb),
  which is what `GET /users/:id` answers with. It is the same entity plus the
  `Users::BioHtml` concern, exposed with no condition.
- Three keys are missing from
  [ee/lib/ee/api/entities/user_with_admin.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/ee/api/entities/user_with_admin.rb),
  which `POST /users` and `PUT /users/:id` answer with: `provisioned_by_group_id`
  under `License.feature_available?(:group_saml)`, and `enterprise_group_id`
  and `enterprise_group_associated_at` under
  `License.feature_available?(:domain_verification)`. Both features resolve to
  Premium in the record's own licensed-feature table, so an administrator on a
  licensed instance creates a user and is handed back a struct that cannot say
  which enterprise group owns it.
- `unconfirmed_email` is missing on this path for a different reason:
  `CreateServiceAccountUser` returns `*User` and sends
  `POST /service_accounts`, which answers with
  [lib/api/entities/service_account.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/service_account.rb),
  a six-key object rather than a user. The SDK does model that key, on
  `ServiceAccount`, which the same file's `UpdateInstanceServiceAccount`
  returns; so one endpoint's response is modelled twice upstream and only the
  narrower struct carries the key. That unevenness is the same shape
  [entry 36](#the-merge-request-structs-miss-six-keys-unevenly-and-two-methods-name-an-entity-they-do-not-answer-with)
  records for the merge request structs.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), whose
oracle is `docs/development/gitlab-api-live.json`. The user family was 64 of
its findings across four output types, and the split is the interesting part:
because all four pair with this one struct, the audit unions all eleven
endpoints its methods reach in front of every one of them, so each type was
held to the same sixteen keys. Only ten of the sixteen are reachable on the
three group-scoped types, whose endpoints GitLab presents `with:
::API::Entities::UserPublic`; the other five are answered by declarations in
`cmd/audit_1to1/internal/paths/sent_declarations.go` rather than published
there. One key, `avatar_path`, is reachable on none of them: `UserBasic`
exposes it under the `only_path` presenter option, which is not a request
parameter, and no route in the whole 2110-route record declares it.

**Effort**: additive and small. Fifteen fields on one struct, all of them
`omitempty`-safe, which is the same change
[entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
already opened for another set of structs. The conditional ones want pointers
rather than values, since a zero follower count and a profile the caller may
not read are different answers.

### IssueRelation models an issue basic where GitLab renders a whole issue

- **Reported**: yes, as the `IssueRelation` block of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which lists twenty-five keys: the twenty-four below and `subscribed`, which
  the listing route never sends, so the umbrella overstates this struct by
  one. No merge request of its own, held back by the batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Every gap is worked around.
- **Workaround**: yes. The twenty-four keys are read from the captured
  response beside the SDK's decode (ADR-0021), through
  `issuelinks.capturedRelations`.

**What**: `IssueRelation` in client-go v3.0.0's `issue_links.go` carries 23
keys and is what `ListIssueRelations` decodes
`GET /projects/:id/issues/:issue_iid/links` into. That endpoint presents
[lib/api/entities/related_issue.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/related_issue.rb),
which is `API::Entities::Issue` plus four link keys, and the struct models
roughly the shape of `IssueBasic` instead. Measured against the `v19.3.1-ee`
entities the committed live record was taken from, twenty-four keys are
missing and only one key of the entity is genuinely absent from that response.

- Nineteen are exposed with no condition, so every relation of every response
  carries them: `_links`, `blocking_issues_count`, `closed_at`, `closed_by`,
  `discussion_locked`, `downvotes`, `has_tasks`, `imported`, `imported_from`,
  `issue_type`, `merge_requests_count`, `moved_to_id`,
  `service_desk_reply_to`, `severity`, `start_date`, `task_completion_status`,
  `time_stats`, `type` and `upvotes`. `blocking_issues_count` is the one of
  the nineteen that comes from the Enterprise module prepended onto the
  entity,
  [ee/lib/ee/api/entities/issue_basic.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/ee/api/entities/issue_basic.rb),
  and it is exposed there under no licensed feature, so an Enterprise instance
  sends it on every relation whatever its plan and a Community one sends
  nothing. It wants no pointer for that reason: the key is present or the
  whole edition is absent.
- Four are licensed, all from
  [ee/lib/ee/api/entities/issue.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/ee/api/entities/issue.rb):
  `epic` and `epic_iid` under `epics`, `iteration` under `iterations` and
  `health_status` under `issuable_health_status`. The first three resolve to
  Premium in the record's own licensed-feature table and the last to Ultimate.
- One, `task_status`, is gated on the issue's own content rather than on a
  licence or a permission.

`epic` is worth a sentence of its own, because the obvious modelling of it is
wrong. It is not `gl.Epic`: the entity renders it `using: EpicBaseEntity`,
[ee/app/serializers/epic_base_entity.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/app/serializers/epic_base_entity.rb),
which is `id`, `iid`, `title`, `url` and `group_id`, plus two human-readable
date strings when the epic has the dates behind them. A struct reusing the
full epic here would advertise twenty keys the endpoint has never sent, and
`EpicBaseEntity` is not an `API::Entities` class, so the generated record
cannot describe it and the Ruby is the only oracle.

The one key of the entity that is genuinely not on this response is
`subscribed`, and it is instructive. `lib/api/entities/issue.rb` exposes it
under `options.fetch(:include_subscribed, true)`, a presenter option whose
default is to **send**, so reading the condition the way the other presenter
options in this register are read gives the wrong answer. `lib/api/issue_links.rb`
settles it: the route's `present` call passes `include_subscribed: false`,
because computing the flag renders Markdown and GitLab will not do that for
every row of a list. The key belongs on the single-issue endpoints, which do
send it and where the SDK's `Issue` already models it.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), whose
oracle is `docs/development/gitlab-api-live.json`. Every finding carries
`sdk_models: false`, which is what says the gap is upstream rather than ours.

**Effort**: additive and small for the scalars, two new sub-structs for `epic`
and the `_links` object. `_links` is a fifth key wider than `gl.IssueLinks`:
the same nested block renders `closed_as_duplicate_of` for an issue closed as
a duplicate, which no struct in the SDK carries.

### MemberRole models twenty of the forty-five permissions GitLab sends

- **Reported**: yes, as the `MemberRole` block of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which lists all twenty-five permissions. No merge request of its own, held
  back by the batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Every gap is worked around.
- **Workaround**: yes. The twenty-five permissions are read from the captured
  response beside the SDK's decode (ADR-0021), through
  `memberroles.capturedRole` and `capturedRoles`.

**What**: `MemberRole` in client-go v3.0.0's `member_roles.go` carries 25 keys,
20 of them permission flags, and is what all four member role routes decode
into. `API::Entities::MemberRole` at `v19.3.1-ee` carries 50 keys, 45 of them
permissions, every one exposed with `default: false` and no condition. So 25
permissions are on every response of every one of those routes and the SDK
drops all of them: `admin_ai_catalog_item`, `admin_ai_catalog_item_consumer`,
`admin_integrations`, `admin_protected_branch`,
`admin_protected_environments`, `admin_runners`, `admin_security_attributes`,
`apply_security_scan_profiles`, `create_security_scan_profiles`,
`delete_security_scan_profiles`, `destroy_package`, `read_admin_cicd`,
`read_admin_groups`, `read_admin_monitoring`, `read_admin_projects`,
`read_admin_subscription`, `read_admin_users`, `read_agent_artifacts`,
`read_compliance_dashboard`, `read_crm_contact`, `read_security_attribute`,
`read_security_scan_profiles`, `read_virtual_registry`,
`update_sec_ai_workflow_settings` and `update_security_scan_profiles`.

**This one could not have been found by reading GitLab's source, and that is
the point of it.**
[ee/lib/api/entities/member_role.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/api/entities/member_role.rb)
exposes its permissions by looping over
`::MemberRole.all_customizable_permissions`, a constant the running
application assembles, so the file says "expose the loop variable" and names
none of the forty-five. A scanner reading that Ruby sees zero permission keys;
the committed live record, taken from a booted GitLab that was asked what the
entity exposes, has all of them. This is the same class as the Geo status
matrix in
[entry 35](#the-geo-structs-model-a-fraction-of-a-site-and-its-status-and-the-repair-method-names-the-wrong-entity),
and it is why `cmd/gen_api_live` evaluates rather than parses.

The documentation cannot stand in for the entity either.
[doc/api/member_roles.md](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/doc/api/member_roles.md)
prints four permission keys across its example bodies and refers the reader to
the abilities page under `doc/user` for the rest, so a contributor working from
the page would model four.

**What must not be copied across with it**: `CreateMemberRoleOptions` accepts
the same twenty the response struct models, and this server's create inputs are
built from those options. Whether GitLab's `POST` accepts the other twenty-five
is a separate question this register does not answer, so the twenty-five stay
off the input fragment here and belong in a separate change upstream if they
are added to the options at all.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`). It had
been recorded as an unmeasured lead in
[entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
on the strength of the record's key count alone; the set difference against the
SDK struct's own json names is what turned it into 25 named fields.

**Effort**: trivial and mechanical. Twenty-five additive `bool` fields with
`json` tags on one struct, and twenty-five keys added to an existing test
fixture. Pointers are not needed upstream the way they are here: this server
keeps them nil to distinguish a permission an older instance never had from one
it denies, and a struct field decoding a body is under no such obligation.

### PipelineInfo decodes two entities and models only the smaller one

- **Reported**: yes, as the `PipelineInfo` block of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which lists all twelve keys. No merge request of its own, held back by the
  batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Every gap is worked around.
- **Workaround**: yes. The twelve keys are read from the captured response
  beside the SDK's decode (ADR-0021), through `pipelines.CapturedOutput` on
  the one route that sends them.

**What**: `PipelineInfo` in client-go v3.0.0's `pipelines.go` carries 11 keys
and is the return type of three methods that do not answer with the same
thing. `ListProjectPipelines` and `ListMergeRequestPipelines` reach endpoints
GitLab presents
[lib/api/entities/ci/pipeline_basic.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/ci/pipeline_basic.rb)
with, ten keys, and the struct models those. `CreateMergeRequestPipeline`
reaches `POST /projects/:id/merge_requests/:merge_request_iid/pipelines`, which
`lib/api/merge_requests.rb` presents `::API::Entities::Ci::Pipeline` with:
[lib/api/entities/ci/pipeline.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/ci/pipeline.rb)
inherits the basic entity and adds twelve keys, every one exposed with no
condition. So a created merge request pipeline always carries `before_sha`,
`tag`, `yaml_errors`, `user`, `started_at`, `finished_at`, `committed_at`,
`duration`, `queued_duration`, `coverage`, `detailed_status` and `archived`,
and the struct decoding it drops all twelve.

The SDK already models eleven of them, on `Pipeline`, which is what the
single-pipeline endpoints decode into. Only `archived` is on neither, and this
register's
[entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
is the wider statement of that half. So the fix is not new modelling: it is
either widening `PipelineInfo` or giving `CreateMergeRequestPipeline` the
return type its endpoint's entity already matches, which would be breaking.

**Two of the twelve are objects, and neither takes the obvious struct.**
`user` is `API::Entities::UserBasic`, which sends `public_email` and `locked`
and no `created_at`; `gl.BasicUser` declares `created_at` and neither of the
other two, so decoding this key into it loses two keys and offers one GitLab
never sends. `detailed_status` is not an `API::Entities` class at all but
[app/serializers/detailed_status_entity.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/app/serializers/detailed_status_entity.rb),
so the generated record cannot describe it and the Ruby is the only oracle.

**A second gap sits one level inside that object and is recorded rather than
closed here**, because the audit's nested comparison cannot reach a type the
record does not hold. `DetailedStatusEntity` renders an `action` object when
the status has one, six keys (`icon`, `title`, `path`, `method`,
`button_title`, `confirmation_message`), and its `illustration` merges the
status's own `size`, `title` and `content` beside the `image` path.
`gl.DetailedStatus` has neither the action nor those three, and
`doc/api/merge_requests.md` documents all of them on `head_pipeline`, so the
documentation is ahead of the struct. This server does not publish them
either: `pipelines.StatusOutput` is filled from the SDK on the single-pipeline
routes, and adding a key there that only the captured path could fill would
leave it empty on every other one. Closing it means reading `detailed_status`
from the capture everywhere, which is the next layer's work rather than this
one's.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), which
unions the endpoints every method returning the struct reaches and holds the
output type against all of them. That union is what makes this finding
readable: the twelve appear against a type whose own package never receives
them, and the route that does is in another package entirely.

**Two more things this endpoint's oracles disagree about**, both recorded and
neither acted on:

- The
  [create merge request pipeline](https://docs.gitlab.com/api/merge_requests/#create-merge-request-pipeline)
  section's example body prints eleven of the twelve keys and omits
  `queued_duration`, which `lib/api/entities/ci/pipeline.rb` exposes on the
  line after `duration` with no condition. That is a documentation merge
  request of the kind
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, held back by the same batching.
- `PipelineInfo` and `Pipeline` both declare a `name`, and neither
  `Ci::PipelineBasic` nor `Ci::Pipeline` exposes one, so the field decodes on
  none of these endpoints. The audit reports it in the other direction, as a
  phantom on this server's own output, where it is undeclared and part of that
  backlog. Removing an exported field is breaking, so it is recorded here the
  way `LicenseTemplate.Featured` and the two event `Title` fields are.

**Effort**: small for the eleven scalars and the timestamps. The two objects
want the shapes above rather than the nearest existing struct, and the nested
`action` is a struct that does not exist upstream yet.

### Group, Project and Issue each model one entity where GitLab renders two

- **Reported**: in part. Every missing key this entry names is in the `Group`,
  `Project`, `Issue`, `ProjectUser`, `ProjectApprovalRule` and `GroupHook`
  blocks of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  and the point of the entry, that each struct decodes two entities, is not.
  Thirteen of that `Group` block's thirty-one are project keys the job token
  allowlist's wrong annotation put there, which
  [the job token entry](#three-job-token-scope-endpoints-declare-a-response-entity-they-do-not-send)
  records, so the eighteen left are this entry's. No merge request of its own,
  held back by the batching
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  records.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. Every key is worked around.
- **Workaround**: yes. All of them are read from the captured response beside
  the SDK's decode (ADR-0021), through `toolutil.CapturedGroup`,
  `CapturedProject`, `CapturedIssue` and their list siblings.

**What**: three of the structs this server uses most model fewer keys than the
Grape entity their routes render, and in each case the gap is bigger than a
missing field because GitLab renders **two** entities through the one struct:
a narrow one on the routes that answer with a page, and a wider one that
inherits it on the routes that answer with a single object.

| Struct    | Narrow entity                             | Wider entity                   | Keys neither models |
| --------- | ----------------------------------------- | ------------------------------ | ------------------- |
| `Group`   | `Entities::Group`                         | `Entities::GroupDetail`        | 10 + 8              |
| `Project` | `Entities::BasicProjectDetails` (24 keys) | `Entities::Project` (159 keys) | 17                  |
| `Issue`   | `Entities::IssueBasic`                    | `Entities::Issue`              | 3 + 6               |

The project row is the one worth reading twice. `BasicProjectDetails` is not a
subset a caller opts into: it is what
[lib/api/search.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/search.rb)'s
`SCOPE_ENTITY` maps the `projects` scope to, what the job token allowlist
answers with, and what every route through `present_projects` narrows to when
the caller passes `simple=true`. One Go struct decodes both, so a client cannot
tell from the type which of the two it is holding.

**The seventeen on `Project`**: `description_html`, `repository_object_format`,
`show_diff_preview_in_email`, `warn_about_potentially_unwanted_characters`,
`secret_push_protection_enabled`, `web_based_commit_signing_enabled`,
`merge_train_enforcement`, `max_pipelines_per_merge_train`,
`duo_remote_flows_enabled`, `duo_foundational_flows_enabled`,
`only_allow_merge_if_all_status_checks_passed`, `duo_sast_fp_detection_enabled`,
`duo_sast_vr_workflow_enabled`, `duo_secret_detection_fp_enabled`,
`duo_dependency_bump_breaking_changes_enabled`,
`security_policy_pipeline_must_succeed` and `spp_repository_pipeline_access`.
Four are unconditional on the entity; the rest are gated on a licensed feature,
on the `read_secret_push_protection_info` ability, or on GitLab.com.
`secret_push_protection_enabled` is not new data: it is the newer spelling of
`pre_receive_secret_detection_enabled`, which the SDK does model, exposed twice
from `project.security_setting`.

**The nine on `Issue`**: `blocking_issues_count`, `start_date` and `type` are
on `IssueBasic` and therefore on every issue GitLab renders anywhere;
`epic_iid`, `has_tasks`, `imported`, `imported_from`, `severity` and
`task_status` are added by `Entities::Issue` and so reach only the issues API's
own routes. `type` and `issue_type` are the same attribute exposed twice, once
with `format_with: :upcase`, so a client that derives one from the other is
guessing at a spelling GitLab owns.

**Three smaller ones in the same batch**, each a single struct and a single
key, all found the same way:

- `ProjectUser` misses `locked` and `public_email`.
  `GET /projects/:id/users` presents `Entities::UserBasic`, which sends eight
  keys unconditionally, and the struct declares six of them.
- `ProjectApprovalRule` misses `coverage_minimum_threshold`, which
  [ee/lib/api/entities/project_approval_rule.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/ee/lib/api/entities/project_approval_rule.rb)
  exposes on a rule whose `report_type` is `code_coverage`.
- `GroupHook` misses `repository_update_events`, which
  [lib/api/entities/group_hook.rb](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.3.1-ee/lib/api/entities/group_hook.rb)
  sends on every group hook. The SDK already declares that field on
  `ProjectHook` and on the system hook, so this is the cheapest of the set to
  land and the hardest to argue with.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), read
against `docs/development/gitlab-api-live.json`, which is taken from a booted
GitLab and carries the field list per entity with the condition gating each.
Every finding here is marked `sdk_models: false`, which is what says the fix is
upstream rather than local.

**One caution for whoever writes the merge request.** `gen_api_live` captures a
condition as source text over a line range, and in
`ee/lib/ee/api/entities/project.rb` the text of each `expose` runs on into the
next one, so two of the seventeen read as gated by their neighbour's feature.
`max_pipelines_per_merge_train` gates on `merge_trains` (Premium) and
`duo_foundational_flows_enabled` on `ai_workflows` (Premium); the record
resolves both to Ultimate from the next expose's `external_status_checks` and
`ai_features`. This server's struct tags carry the corrected tiers.

**Effort**: small per struct and mechanical. The partition is the work: a
struct that keeps serving both entities cannot answer the pointer question,
which is why this server split each of the three into one output type per
entity before surfacing anything.

### The work item get, create and update documents select licensed fields

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: yes, on Community Edition. `issue.work_item_get`,
  `issue.work_item_create` and `issue.work_item_update` cannot answer there:
  GitLab refuses the whole document, so the three actions fail on every call
  a Free instance is given, while the listing and the type listing work.
- **Workaround**: none possible without replacing the SDK's documents. The
  selection set is built inside `GetWorkItem`, `CreateWorkItem` and
  `UpdateWorkItem` from one template, and nothing a caller passes changes it.
  The e2e suite records the state instead: `test/e2e/gitlab/common`'s work
  item lifecycle skips on a Free runtime naming this entry, and its type
  listing runs on both.

**Where**: `workitems.go`, `workItemTemplate`, which `getWorkItemTemplate`,
`createWorkItemTemplate` and `updateWorkItemTemplate` clone.

**What**: the shared template selects five widgets that exist only in the
Enterprise schema: `color`, `healthStatus`, `iteration`, `status` and
`weight`. A Community Edition instance answers the whole document with
`Field 'color' doesn't exist on type 'WorkItemFeatures'` and the four
siblings, and the SDK surfaces that as `Mutation.workItemCreate failed`, so a
caller on Free cannot read, create or update a work item at all. The SDK
knows the problem: `ListWorkItemsOptions.ReturnedFields` exists precisely so
the listing can leave those five out, its comment says the five "error
against Community Edition instances", and `WorkItemDefaultListFields` is
documented as CE-safe. The other three operations were given no such
selector and select everything.

**How we found it**: the rebuilt e2e suite drove the work item lifecycle on
the Community runtime for the first time (the old suite kept it in its
Enterprise half), and `work_item_create` failed on all three surfaces with
the five field errors above; verified against v3.0.0 and against `main` on
2026-09-12, where the three functions still execute the full template.

**Effort**: small. Either the three operations take the same `ReturnedFields`
the listing takes, with the same CE-safe default, or the template drops the
five widgets into fragments the caller opts into. The decoder already
tolerates their absence, since the listing runs without them today.

### UpdatePackageProtectionRulesOptions sends two explicit nulls on every partial update

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: partly. An update that names the pattern and the type works;
  an update that changes only an access level is refused by GitLab, and there
  is no spelling of that call this server can send.
- **Workaround**: partial, and it cannot be complete. The struct tag is what
  decides, so no option a handler passes suppresses the nulls.
  `internal/tools/protectedpackages.Update` sets each pointer only when the
  caller named a value, which is the safe side of that choice, and answers the
  refusal with a hint telling the caller to name `package_name_pattern` and
  `package_type` on every update. The protection rule lifecycle in
  `test/e2e/gitlab/common` sends the type with every update for the same
  reason. All of it retires the day the tag changes.

**Where**: `protected_packages.go`, `UpdatePackageProtectionRulesOptions`.

**What**: the struct's two `*string` fields carry no `omitempty`.

```go
type UpdatePackageProtectionRulesOptions struct {
	PackageNamePattern          *string                             `url:"package_name_pattern" json:"package_name_pattern"`
	PackageType                 *string                             `url:"package_type" json:"package_type"`
	MinimumAccessLevelForDelete Nullable[ProtectionRuleAccessLevel] `url:"minimum_access_level_for_delete,omitempty" json:"minimum_access_level_for_delete,omitempty"`
	MinimumAccessLevelForPush   Nullable[ProtectionRuleAccessLevel] `url:"minimum_access_level_for_push,omitempty" json:"minimum_access_level_for_push,omitempty"`
}
```

The two `Nullable` fields beside them do carry it, and there it works, because
`Nullable[T]` is a `map[bool]T` and an empty map is empty to `encoding/json`. A
nil `*string` is not, so `PATCH /projects/:id/packages/protection/rules/:id`
always carries both keys: an update that changes only an access level sends
`{"package_name_pattern": null, "package_type": null, ...}`.

GitLab reads that null as blank rather than as "leave this one alone", and the
rebuilt e2e suite measured it against a live instance: an update naming only
the pattern is answered `422 Package type can't be blank`, which is why
`TestPackage_ProtectionRules_Lifecycle` sends the type with every update. The
mirror case was not measured, but it is the same null through the same
whole-rule validation.

GitLab's own record marks both parameters optional on the PATCH and required on
the POST (`docs/development/gitlab-api-live.json`, `PATCH
/api/:version/projects/:id/packages/protection/rules/:package_protection_rule_id`),
so `CreatePackageProtectionRulesOptions`, which carries the same two tags, is
unharmed: a caller who omits either one is refused for omitting it whether the
key arrives as null or not at all. The minimal fix is the update struct alone,
although a reviewer may prefer both changed for symmetry.

**How we found it**: the `protectedpackages` sweep, reading what the handler can
and cannot control. The consequence came from the e2e scenario's own comment,
which had recorded the 422 without connecting it to the tag. Verified identical
in v3.0.0, v3.10.0 and v3.12.0, so a dependency bump does not retire it.

**Effort**: small, two struct tags and a test, like
[`SetFeatureFlagOptions`](#setfeatureflagoptions-fields-lack-omitempty).

## MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)

Nine of the entries here were filed upstream together on 2026-09-13, one issue
each so that a maintainer can triage, label and close them apart, with
[modelcontextprotocol/go-sdk#1257](https://github.com/modelcontextprotocol/go-sdk/issues/1257)
as an index over the set. The four that need no new exported API carry a pull
request; the four that do are proposals waiting on a decision about the shape,
because a maintainer chooses their own API and a pull request that assumes the
answer wastes both sides' time. The tenth, `Mcp-Name`, was fixed upstream by
somebody else before we got to it.

### No keep-alive interval for SSE streams on StreamableHTTPOptions

- **Reported**: yes, in three pieces, two of them by other users.
  [modelcontextprotocol/go-sdk#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262),
  ours, filed on 2026-09-13, is the third: an SSE response keeps
  `http.Server.WriteTimeout` armed, so a long-lived stream fails at its first
  write after the deadline, which defeats the other two fixes on any server
  that sets it. It names those two as open elsewhere: the headers a streamed
  POST never commits
  ([modelcontextprotocol/go-sdk#1155](https://github.com/modelcontextprotocol/go-sdk/issues/1155),
  with the pull request
  [modelcontextprotocol/go-sdk#1197](https://github.com/modelcontextprotocol/go-sdk/pull/1197),
  both still open) and the keep-alive comment
  ([modelcontextprotocol/go-sdk#1229](https://github.com/modelcontextprotocol/go-sdk/issues/1229),
  since closed by the merge below).
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232),
  merged on 2026-09-21. Ours is not opened yet. The shape was settled on the
  issue on 2026-09-18, where a contributor prefers extending the write
  deadline on every write, added as an option on top of
  [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232)
  once that merged, and we said we would open that pull request then. It has
  merged, so the next step is ours.
- **Merged**: in part, and by somebody else:
  [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232)
  added `StreamableHTTPOptions.StreamKeepAlive` on 2026-09-21, in no tag yet,
  since v1.8.0 predates it. It keeps the `subscriptions/listen` response
  stream alone alive, every 30s by default: every other streamed POST
  response, a slow tool call's for one, and the standalone GET stream are
  untouched, so it covers one stream of the case this entry describes and not
  the rest.
- **Blocking**: no.
- **Workaround**: yes, and it covers more than the requested option would.
  `sseAwareWriter` in `cmd/server/main.go` emits a comment frame every 25s on
  **any** response that commits to `text/event-stream`, guarding its writes with
  the same mutex the handler's writes take. It stays now that the option has
  merged, since that option covers the listen stream alone.

**What**: the SDK emits keep-alives only on the standalone GET stream, not on
streamed POST responses, and offers no option to configure the interval. An idle
SSE response therefore puts no bytes on the wire, and a proxy's read timeout
severs it; nginx's `proxy_read_timeout` is 60s by default. Worse, with nothing
written the response headers are not flushed either, so the client hangs before
the first read rather than after.

**How we found it**: writing `TestSSEKeepAlive_IdleStreamKeepsBytesOnTheWire`.
The test hung instead of failing, which is how the header-flush half surfaced.

### A malformed message ends the session instead of answering -32700

- **Reported**: yes, by another user:
  [modelcontextprotocol/go-sdk#1209](https://github.com/modelcontextprotocol/go-sdk/issues/1209)
  (2026-08-29) describes the same stdio behavior.
- **In review**: yes, theirs:
  [modelcontextprotocol/go-sdk#1210](https://github.com/modelcontextprotocol/go-sdk/pull/1210),
  open with no maintainer response as of 2026-09-12. We open no second one;
  if it stalls, the evidence here (the e2e case and the stdio filter) goes on
  that thread rather than into a new pull request.
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

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1259](https://github.com/modelcontextprotocol/go-sdk/issues/1259),
  on 2026-09-13.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1267](https://github.com/modelcontextprotocol/go-sdk/pull/1267):
  `Connection.CancelFromPeer` records that the peer cancelled a given id, which
  is what the jsonrpc2 layer could not tell apart from an ordinary context
  ending, and the response is then suppressed for that id alone. The streamable
  transport implements `ResponseDropper` so the POST's stream is released
  rather than left hanging, and `loggingConn` forwards it, since that wrapper
  would otherwise swallow the interface and reinstate the response. The
  narrowest reading of the clause was chosen deliberately: `notDone` still
  writes a response when the caller's own context ended for any other reason.
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

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1254](https://github.com/modelcontextprotocol/go-sdk/issues/1254),
  on 2026-09-12.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1255](https://github.com/modelcontextprotocol/go-sdk/pull/1255):
  `Connection.CancelCause` in the internal jsonrpc2 package, the canceller
  cancelling with an error that carries the reason and unwraps to
  `context.Canceled`, a debug log line with the id and the reason, and two
  tests. Chosen as the first contribution to that SDK because the maintainers
  had already accepted the cause plumbing it builds on (their
  [modelcontextprotocol/go-sdk#1100](https://github.com/modelcontextprotocol/go-sdk/issues/1100)),
  it adds no exported API, and it answers a SHOULD of the specification.
- **Merged**: **yes, upstream, on 2026-09-14, and in no released version yet.**
  v1.8.0 was published that morning and the merge landed after it, so the
  released SDK still drops the reason: its `canceller.Preempt` reads
  `params.RequestID`, calls `conn.Cancel(id)` and never looks at
  `params.Reason` (`mcp/transport.go`). The first release carrying it retires
  the note below.
- **Blocking**: no.
- **Workaround**: yes, until that release. The field is dropped inside the SDK
  and there is no seam to read it from, so we log what remains — that the call
  was cancelled and how long it ran. Once the fix ships, `context.Cause(ctx)` in
  a handler reads `request cancelled by the peer: <reason>`, and the
  classification in `internal/toolutil` can carry the reason into the log line.

**What**: "Implementations SHOULD log cancellation reasons for debugging."
`mcp/transport.go` unmarshals `CancelledParams`, uses `params.RequestID` to
cancel the call, and discards `params.Reason`. A hook, or even a logger line at
the preempter, would be enough.

**How we found it**: the interaction-pattern audit. Sending a cancellation with
reason "User requested cancellation" left no trace of that string anywhere in
the server's output.

### The declared protocol version, not the negotiated one, selects MRTR

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1258](https://github.com/modelcontextprotocol/go-sdk/issues/1258),
  on 2026-09-13.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1266](https://github.com/modelcontextprotocol/go-sdk/pull/1266):
  an unexported `ServerSession.protocolVersion` that answers with
  `NegotiatedProtocolVersion` and falls back to the declared value for a
  session that never ran `initialize`, read by both the MRTR check and the
  server-initiated-request assertion. Fixing only the first makes the symptom
  worse rather than better, which the pull request shows by running the test
  against exactly that half. Two more sites read the declared value the same
  way, `Server.notifySessions` and `Server.ResourceUpdated`, and are left out
  on purpose: the second has to decide what happens to a session that
  subscribed through the legacy method and is routed into the new branch, which
  is a judgement rather than a swap. The pull request says so and offers it.
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

### Three methods are served on a legacy session before the initialize handshake

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1271](https://github.com/modelcontextprotocol/go-sdk/issues/1271),
  on 2026-09-15.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273),
  which moves the gate out of the switch so it covers every method rather
  than only the ones without a `case`.
- **Merged**: yes, 2026-09-17,
  [modelcontextprotocol/go-sdk#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273),
  unreleased: the merge is `826e653c` and it closed the issue, but the newest
  tag is v1.8.0, cut on 2026-09-14, so it predates the merge and the pin in
  our `go.mod` does not carry the fix. What retires this entry is a release
  and a version bump, not the merge, and until then the three methods are
  still served before the handshake by the SDK this server compiles against.
- **Blocking**: no, but it reaches our surface: `resources/subscribe` is one
  of the three, and ADR-0015 makes the first read the authorization check, so
  the watcher a pre-handshake subscribe starts is one this server created.
- **Workaround**: none taken. The refusal belongs in the SDK, and disagreeing
  with it here would mean this server rejecting a call the SDK accepts.

**What**: `ServerSession.handle` refuses a call made before `initialize` on a
legacy session, and the refusal sat in the `default` branch of its method
switch. Any method with a `case` of its own skipped it, and three are served
on a session that never handshook: `logging/setLevel`, `resources/subscribe`
and `resources/unsubscribe`.

`resources/subscribe` is the one that matters, because it reaches state rather
than merely answering: it starts a watcher and registers a subscriber, so a
client that subscribes before `initialize` is delivered
`notifications/resources/updated` for the lifetime of a session the server
never agreed to.

**Why it is the SDK disagreeing with itself** rather than with the
specification: the lifecycle page is what motivates the gate the `default`
branch applies, and these three sat outside it for no stated reason. The
discriminator is the request rather than the session, which is the condition
the `default` branch already applied: a call carrying no
`_meta.protocolVersion` on a session with no recorded handshake. A SEP-2575
session legitimately has no `initialize` and stays served through the same
`usesNewProtocol` exemption.

**Two review points settled on the pull request**, both worth keeping because
each is a rule about the gate rather than about this change. The comment was
shortened to the rule and the exemption, since how the gap came to exist
belongs in the commit message. And a proposal to replace `req.IsCall()` with
`req.Method != notificationInitialized` was declined with evidence: that
widens the gate from calls to notifications, and `notifications/cancelled` has
no `case` of its own, so a client giving up on a slow `initialize` would have
its cancellation refused and the in-flight call never cancelled. The SDK's own
client sends exactly that message (`client.go:353`) and the transport has a
dedicated path for it (`transport.go:261`).

### The negotiated protocol version is recorded on one path of four

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1272](https://github.com/modelcontextprotocol/go-sdk/issues/1272),
  on 2026-09-15.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274),
  merged.
- **Merged**: yes, on 2026-09-21, and in no tag: v1.8.0 was cut on 2026-09-14
  and does not contain the merge commit.
- **Blocking**: no, and the concrete failure is on the transport this server
  leads with. `ioConn.sessionUpdated` reads only `NegotiatedProtocolVersion`,
  so a SEP-2575 session over **stdio** is treated as `2025-03-26` and accepts
  JSON-RPC batches. Batching was removed in `2025-06-18`, and the streamable
  handler already refuses them by reading the header, so the two transports
  disagree about the same session shape.
- **Workaround**: none taken.

**What**: four places record a protocol version on a `ServerSession` and only
`ServerSession.initialize` records it in `NegotiatedProtocolVersion`. The other
three (`handle` for a new-protocol client's `_meta`, `server/discover`, and the
streamable handler synthesizing from the `MCP-Protocol-Version` header) record
only `InitializeParams`, so a reader asking what a session speaks gets a
different answer depending on how the session began.

The support check those three apply **is** all the negotiation SEP-2575 has:
with no handshake response there is nowhere to communicate a downgrade, so a
version they accept is one the server supports, and recording it as negotiated
states what happened. `initialize` stays the one path that downgrades.

**A reviewer proposal here was refuted by a test, and the refutation is the
useful part.** Validating an incoming new-protocol request against
`Session.supportedVersions` rather than `Server.protocolVersions` looks
obviously right, and its premise is: `server/discover` answers with the
session's list, so validating against the server's means advertising one set
and accepting another. But `StreamableServerTransport.SupportsProtocolVersion`
returns `t.Stateless && …` for any version at or above `2026-07-28`, so on a
**stateful** session the session's list excludes `2026-07-28` entirely, and the
change refuses the one request whose job is to ask what the server supports.
`TestStreamableStateful_AcceptsDiscover` turns from 200 to 400. A discover
probe cannot be required to speak the version it is asking about. The shape
that does work, verified against the package with `-race`, judges
`methodDiscover` against the server's list and every other new-protocol method
against the session's.

### Application code cannot send notifications/cancelled for a listen stream

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1263](https://github.com/modelcontextprotocol/go-sdk/issues/1263),
  on 2026-09-13, as a proposal: closing it needs a method the SDK does not
  have. It is the one finding of the nine that names a MUST rather than a
  SHOULD.
- **In review**: no. Waiting on the maintainers to say which shape they want.
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

- **Reported**: yes, by another user, not by us:
  [modelcontextprotocol/go-sdk#1234](https://github.com/modelcontextprotocol/go-sdk/issues/1234),
  opened 2026-09-03 and fixed upstream before we got to it.
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1242](https://github.com/modelcontextprotocol/go-sdk/pull/1242),
  merged on 2026-09-06.
- **Merged**: **yes**, by another contributor:
  [modelcontextprotocol/go-sdk#1242](https://github.com/modelcontextprotocol/go-sdk/pull/1242)
  on 2026-09-06 decodes the header before the comparison, and
  [modelcontextprotocol/go-sdk#1246](https://github.com/modelcontextprotocol/go-sdk/pull/1246)
  makes the SDK's own client encode a name that is not header-safe. Neither is
  in a tag yet: the newest is v1.8.0, published on 2026-09-14 on the same
  commit as v1.8.0-pre.2 of 2026-09-04, so the v1.8.0 pin does not carry it.
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

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1261](https://github.com/modelcontextprotocol/go-sdk/issues/1261),
  on 2026-09-13.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1269](https://github.com/modelcontextprotocol/go-sdk/pull/1269),
  merged:
  `mcp.HasParams(req Request) bool`. This is the one of the four pull requests
  that adds an exported symbol, and the body says so and offers the smaller
  answer instead, which is the `getRequestMeta` fix plus the guarantee written
  into the `AddReceivingMiddleware` doc comments and no new function. It is a
  function over `Request` rather than an exported `isNil`, because a params
  type declared the documented way, `struct{ mcp.ParamsBase }`, promotes
  `isNil` through a field selector and dereferences the nil outer pointer
  before the body runs, so exporting the predicate would ship one that is
  itself unsafe. The same pull request stops three accessors panicking on that
  value.
- **Merged**: yes, on 2026-09-14 (merge commit `3f43bcf0`), and in no tag:
  v1.8.0 was published that morning on the commit of v1.8.0-pre.2 and does not
  contain it.
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

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1260](https://github.com/modelcontextprotocol/go-sdk/issues/1260),
  on 2026-09-13.
- **In review**: yes,
  [modelcontextprotocol/go-sdk#1268](https://github.com/modelcontextprotocol/go-sdk/pull/1268),
  merged:
  classify by membership in the supported set rather than by comparing strings,
  so an unrecognised version is refused with the error the versioning page
  requires instead of being read as a legacy handshake. Writing that pull
  request corrected this entry's own reach: the window is **not** stdio-only.
  The streamable header gate is
  `!slices.Contains(supported, v) && v < "2026-07-28"`, so an unknown version
  sorting **above** that revision passes it and lands in the same
  classification, and `mcp/sse.go` has no header check at all. Every transport
  can reach it.
- **Merged**: yes, on 2026-09-14 (merge commit `a82c86a6`), and in no tag:
  v1.8.0 was published that morning on the commit of v1.8.0-pre.2 and does not
  contain it.
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

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1265](https://github.com/modelcontextprotocol/go-sdk/issues/1265),
  on 2026-09-13, as a proposal offering both shapes below. The earlier note
  here said reporting was deferred because the workaround is complete; it went
  out with the rest of the batch, since a complete workaround is a reason not
  to be blocked and not a reason to keep the gap to ourselves.
- **In review**: no. Waiting on the maintainers to say which shape they want.
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

### A session's second listen on a URI overwrites the first's subscription, and its close deletes both

- **Reported**: no, not yet. It shares a root cause with the entry above, whose
  proposal is still waiting on a maintainer decision, and the shape of the fix
  here depends on what they choose.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. It needs a client that opens two `subscriptions/listen`
  covering one URI, or mixes a legacy `resources/subscribe` with a listen, on
  stdio or `--stateless=false`. The SDK's own client never holds two at once,
  but it reaches the same state when it unsubscribes a URI and subscribes it
  again before the server has run the first listen's teardown: on 2026-07-28
  `Unsubscribe` only cancels the listen and returns, so the second listen's
  subscribe can land first, is acknowledged, and is then deleted by the first
  one's deferred unsubscribe. The e2e harness is written around that
  (`Session.TrySubscribe` in `test/e2e/internal/harness/verbs.go`).
- **Workaround**: partial. `sessionBridge.holds` in
  `cmd/server/subscriptions.go` already keeps the watch alive for the surviving
  stream, which is the half this side owns. The delivery half cannot be repaired
  here, because the table that lost the request id is the SDK's.

**What**: `resourceSubscriptions` is keyed by URI and session
(`map[string]map[*ServerSession]jsonrpc.ID`, `mcp/server.go`), so a session's
second listen on a URI overwrites the first's request id, and that second
listen's deferred unsubscribe deletes the entry outright. After either stream
closes the session is subscribed in its own view and reachable in neither: no
notification is delivered, and no ending is delivered either, so the client sees
a stream it believes is live and never hears from again. Meanwhile the watch
this server correctly kept for the surviving stream goes on polling GitLab on
the subscriber's token until its lifetime expires.

SEP-2575 makes the listen request the subscription identity and allows several
per session, which the SDK's own comment cites as "multiple concurrent
subscriptions", so the table's key is one level coarser than the protocol it
implements.

**The fix belongs upstream**: key the table by request id, or refuse a second
listen for a URI a session already holds. The second is a smaller change and
would cost a client one working subscription instead of two half-working ones.

**Still present in v1.8.0**, checked against that release's source rather than
the pinned one. It ships several subscription fixes, including pruning
completed listen request ids from the session, and the table's key is unchanged.

**How we found it**: auditing every declared capability against the
specification and the SDK. [ADR-0015](adr/adr-0015-polled-resource-subscriptions.md)
recorded the overwrite half and understated it, saying a session with two
listens sees the notification "on one of them"; the state after either close is
neither, and the ADR now says so.

## OpenAI Codex (`openai/codex`)

### A non-integer annotation priority breaks a tool call

- **Reported**: yes,
  [openai/codex#38979](https://github.com/openai/codex/issues/38979).
- **In review**: no. The issue is open and labelled `bug`, `mcp`, `CLI`,
  `tool-calls`, and no fix has been proposed upstream.
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

## GitLab (`gitlab-org/gitlab`)

### Three job token scope endpoints declare a response entity they do not send

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/gitlab!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698),
  merged.
- **Merged**: **yes**, into `master` on 2026-09-14 (milestone 19.4, merge
  commit `1dd45490`), and released: held to the tags that contain that commit,
  read on 2026-09-24, it is in `v19.4.0-ee`, tagged on 2026-09-16, and in
  `v19.4.1-ee`, so 19.4.0 is the first GitLab whose annotations say what the
  three endpoints send. The declaration below still stands, because the record
  this repository commits was taken from `19.3.1-ee` and still carries the
  three wrong annotations. It retires when `cmd/gen_api_live` is run against a
  19.4 or later image, which waits until the work in flight has landed; the
  stale-declaration check then fails on it until it is removed.
- **Blocking**: no.
- **Workaround**: yes, a declaration. The 13 findings this produces against
  `internal/tools/groups`' output are answered under
  `documented-response-is-not-the-one-sent`, which is the category built for
  exactly this: the record is right about what GitLab says and wrong about what
  GitLab sends, because GitLab itself is wrong about it.

Three Grape `desc` blocks in `lib/api/project_job_token_scope.rb` name a
response entity the handler beneath them does not present. The wrong line and
the `present` it contradicts landed in the same commit each time, so this is a
copy-paste at introduction rather than drift.

| Endpoint                                    | Declared              | Actually presented  |
| ------------------------------------------- | --------------------- | ------------------- |
| `GET :id/job_token_scope/groups_allowlist`  | `BasicProjectDetails` | `BasicGroupDetails` |
| `POST :id/job_token_scope/groups_allowlist` | `BasicGroupDetails`   | `GroupScopeLink`    |
| `POST :id/job_token_scope/allowlist`        | `BasicProjectDetails` | `ProjectScopeLink`  |

**How it was settled.** Not by reading the source, which is what found it, but
by calling the three endpoints against a fixture project and comparing the keys
that came back. The groups allowlist returns three keys, `id`, `web_url` and
`name`; the projects allowlist, which really is annotated with the project
entity, returns seventeen. The two creations return a two-key link object with
no `id` at all. Both links created for the measurement were deleted afterwards
and the original state verified restored.

**Why the reading alone was not enough.** Every way the finding could have been
wrong was tried first. None of the presented entities descends from the declared
one, so the annotation is not merely loose. No Enterprise override reopens the
class, and the route is defined once. `doc/development/api_styleguide.md`
defines `model` as the entity returned in the response body and its own example
pairs the two, so a mismatch is not house style. It went upstream as the merge
request above rather than as an issue of its own.

**What it costs GitLab.** Both committed OpenAPI specifications carry all three
wrong schemas, and `ProjectScopeLink` and `GroupScopeLink` have no schema at all
in either, because no `desc` has ever referenced them.

**What the fix is.** Three lines of Ruby, and **not** a documentation change:
`doc/api/project_job_token_scopes.md` already documents all three correctly, so
the page and the annotation contradict each other and the page is the one that
matches the wire. That puts the fix on the backend review path rather than the
documentation one this project has used so far. A `lefthook` hook regenerates
`doc/api/openapi/openapi_v3.yaml` from `lib/api/**/*.rb`, and CI checks its
freshness, so the change cannot be sent from outside a GDK checkout without that
regenerated file.

Two adjacent observations were left out of the finding on purpose, so that the
three lines stay reviewable on their own: both list endpoints are missing
`is_array: true`, which is real but repository-wide, and `success status:`
against the styleguide's `code:` is the prevailing idiom rather than a defect.

### Cancelling an auto-merge answers a status hash under a merge request annotation

- **Reported**: yes, as the merge request below rather than as an issue of its
  own, the way the job token scope annotations went.
- **In review**: yes, as **two** merge requests from the community fork,
  [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702)
  (adds `cancel_auto_merge`, which as reviewed answers `201` with the merge
  request when the cancel goes through, `409 Conflict` with the service's
  "Can't cancel the automatic merge" when it does not, and `403` to a caller
  without the permission) and
  [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)
  (corrects the old endpoint's documentation to what it actually sends, and
  deprecates it).

  **Where they stand on 2026-09-24.** The backend review of 2026-09-16 settled
  `201` for success, `409` for a refusal and a commit message without the
  short reference to its own merge request, which had turned `danger-review`
  red, and all three went in that day. On 2026-09-24 the branch of
  `gitlab-org/gitlab!255702` was rebuilt on current `master` as one commit,
  `ff12f93a`, whose message states the final contract and whose only change
  beyond the reviewed diff is `"bot": false` under `author` in the new
  example, since
  [gitlab-org/gitlab!256381](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256381)
  had added that attribute to every merge request response; the push reset
  the technical writer's approval. The rebase also made
  `cells-routes:router-in-sync` blocking on this branch; it had run here as a
  non-blocking failure since 2026-09-15, and
  [gitlab-org/gitlab!257152](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/257152)
  made it blocking on `master` on 2026-09-23. It compares GitLab's routes
  with the HTTP Router's committed snapshot, so the new route needs that
  snapshot refreshed. The paired refresh is
  [gitlab-org/cells/http-router!1354](https://gitlab.com/gitlab-org/cells/http-router/-/merge_requests/1354):
  the route falls under the router's existing project rule, so nothing but the
  snapshot changes, and @marcogreg, asked for it on 2026-09-24, approved and
  merged it the same day (merge commit `8337f3f5`). That job was the one
  failure of the fork pipeline on `ff12f93a`, whose predictive rspec runs
  passed, and the next pipeline runs it against the refreshed snapshot. The
  merge request's description and a correction in the `danger-review` thread
  set out the router's timeline, and it was readied the same day naming
  @marc_shaw for the re-review. He approved it at 12:57 UTC, with one
  non-blocking suggestion, that no spec covered the `author == current_user`
  half of `can_cancel_auto_merge?`, and asked @egrieff for a second review;
  @uchandran approved it at 13:07 UTC. The suggestion was applied at 18:39 UTC
  in one commit, `839df5e1572f`, which specs the author path, and every thread
  was answered and resolved. That push reset @marc_shaw's approval and started
  a fork pipeline on `839df5e1572f`. As of 2026-09-24 it carries @uchandran's
  approval, and waits on @egrieff's review and on @marc_shaw approving again;
  it still needs a maintainer approval for each of the `/config/`, `/lib/` and
  `/spec/` code-owner rules, and nothing is pending on us.
  `gitlab-org/gitlab!255704` waits on it in turn: it is rebased onto
  `gitlab-org/gitlab!255702` once that merges, and gains a link to the new
  section then.

  **The first attempt,
  [!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239), was
  closed unmerged, and why is the useful part.** It changed the handler to match
  the documentation, on the reasoning that unlike
  [!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698), which
  corrected annotations to match the code, here the page and the annotation
  agree with each other and only the handler disagrees. @phikai answered that
  this is a breaking change whichever way it is argued, since it changes the
  response of a stable endpoint, and @marc_shaw proposed the shape that was
  taken instead: leave the old endpoint exactly as it behaves, deprecate it in
  the documentation, and add a new one under the current naming. His reason is
  worth recording because it applies to every future contribution of this shape:
  "we basically can't deprecate our API, by introducing another endpoint, we are
  now maintaining the old and the new". That is why the deprecation is a
  documentation notice and **not** an entry in `doc/api/rest/deprecations.md`,
  which promises removals, and why a symmetric `add_to_auto_merge` was declined
  in the same breath.
- **Merged**: no.
- **Blocking**: it was, for the action. `merge_request.cancel_auto_merge`
  answered a model with an object carrying no IID, no state and no title, so a
  caller could not tell a cancelled auto-merge from a broken call.
- **Workaround**: yes, and it is the right shape whatever GitLab does:
  `CancelAutoMerge` in `internal/tools/mergerequests/merge_requests.go` reads
  the merge request back when the answer carries no IID, which is what
  `toggleSubscription` beside it already does when a subscription toggle is
  answered 304 with an empty body. Pinned by
  `TestMRCancelAutoMerge_StatusHashIsReadBack`.

**What**: `POST /projects/:id/merge_requests/:iid/cancel_merge_when_pipeline_succeeds`
is annotated `success Entities::MergeRequest` in `lib/api/merge_requests.rb`,
which is what the API documentation publishes and what `cmd/gen_api_live`
records in `docs/development/gitlab-api-live.json`, since that record reads the
`desc` block. The endpoint body is `AutoMergeService.new(...).cancel(mr)` with
no `present` after it, and `AutoMerge::BaseService#cancel` returns
`::BaseService#success`, so what goes on the wire is `{"status":"success"}`.
A `desc` annotation documents a response; it does not render one, and this is a
route where the two disagree.

**The failure path is lost the same way**, which writing the merge request
turned up: `clear_auto_merge` rescues and returns
`error("Can't cancel the automatic merge", 406)`, and Grape has no reason to
read `http_status` out of a plain hash, so a cancellation that fails is
answered `201` with the error hash as its body while the page documents `406`.
Both halves of the documented contract were lost in the same missing `present`.

**Why it survived for years, measured rather than guessed while writing the
replacement.** The endpoint's own request spec never arms an auto-merge and
cannot notice that it does not: it sets up with
`AutoMergeService#execute(merge_request, STRATEGY_MERGE_WHEN_CHECKS_PASS)`, and
on that factory the call is a no-op, because
`MergeWhenChecksPassService#availability_details` returns an error when the
merge request is already mergeable with no pipeline in progress, which is
exactly the fixture's state. Against the real fixture: `mergeable? true`,
`available_for? false`, `execute :failed`, `auto_merge_enabled false`. The
endpoint answers `201` either way and the spec asserts only `:created`, so it
passes whatever the handler does.

**The false contract was published in a third place, and that one is CI-gated**:
`doc/api/openapi/openapi_v3.yaml` is generated from these `desc` blocks, is
committed, and `scripts/static-analysis` holds it through
`gitlab:openapi:v3:check_docs`. Its `201` carried an `APIEntitiesMergeRequest`
schema for an endpoint that returns no merge request. Two smaller findings from
the same reading: the real body key order is `message`, `status`, `http_status`,
and the `406` in the endpoint's `desc` failure list can never fire, because
`not_acceptable!` is called in exactly one place in the whole codebase
(`lib/api/repositories.rb`, hotlink detection). Both are evidence that the
documentation was written from the annotation rather than from the behaviour,
which is the pattern this whole record exists to catch.

**Why it is worth recording rather than just working around**: the annotation
is an oracle three of our own audit rules read. R-PATH's type grain joins an
output type to the endpoints client-go's methods name and then asks this record
what those endpoints send, so a route whose annotation is wrong quietly teaches
every rule downstream the wrong answer. This is the first case found where the
generated record is confidently wrong rather than merely silent, which is the
class the record's own documentation says a scan cannot catch either.

**How we found it**: the e2e rebuild. The old CE suite called the action and
threw the answer away, and its own note recorded that a live 19.3 instance had
replied with a body carrying no IID, so the evidence had been written down and
never acted on. Asserting the answer is what turned it into a failure. The
handler's unit test mocked a full merge request body, which is why the server's
contract was wrong in the same direction as the record and no gate could see
the disagreement.

### A revoked GPG UID is still offered for verification and still verifies commits

- **Reported**: yes, by another user, before us, as
  [gitlab-org/gitlab#24572](https://gitlab.com/gitlab-org/gitlab/-/work_items/24572).
  The merge request names it as the issue it closes, and it was still open on
  2026-09-23, after the merge.
- **In review**: yes,
  [gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300),
  from the community fork, merged.
- **Merged**: **yes**, on 2026-09-23 into `master` (milestone 19.5, merge
  commit `4033c4fa`). Held to the tags that contain that commit, read the same
  day, it is in no release yet.
- **Blocking**: no.
- **Workaround**: none possible. The verdict is computed inside GitLab and
  served as one string; nothing on this side can tell a signature verified
  under a live identity from one verified under a revoked one. An instance is
  fixed once it runs a release that carries the merge.

**Where**: `Gitlab::Gpg.user_infos_from_key` in `lib/gitlab/gpg.rb`.

**What**: revoking a UID does not remove it from the key. GnuPG records the
revocation as another signature on the same UID, so the UID is still in the
key and still comes back from GPGME. GitLab listed every one of them: the
revoked addresses were offered for verification in User Settings > GPG keys,
and `GpgKey#verified_and_belongs_to_email?` accepted them, which is what
decides the Verified badge on a signed commit. Deleting the key and adding it
again does not help, because the UID is inside the key.

**What it costs this server**: `repository.commit_signature` publishes GitLab's
`verification_status` verbatim (`internal/tools/commits/commits.go`), so a
commit signed under an address its owner revoked is served to a model as
`verified`, which is the one thing that field exists to say. There is nothing
to work around: the field is GitLab's verdict, and a second opinion computed
here would be a different answer to the same question rather than a better one.

**The fix**: skip the UIDs GPGME reports as revoked. That matches what
revocation already means elsewhere in GitLab, where revoking a key withdraws
the verification of the commits signed with it while removing one leaves them
untouched. A commit signed under a revoked identity then lands on
`same_user_different_email` or `other_user`, and a key whose every UID is
revoked verifies nothing.

Two details are worth keeping, because both were nearly got wrong:

- GPGME's `invalid` flag is deliberately **not** checked, unlike the earlier
  attempt at
  [gitlab-org/gitlab!36315](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/36315).
  GnuPG drops a UID with no valid self-signature at import, so such a UID never
  reaches the listing and no fixture can be built for one through the import
  path GitLab uses. A check nothing can exercise is a check nobody can trust.
- The change **does** re-derive signatures that were already verified, which
  the first reading of it said it would not. `GpgKeys::DestroyService` nulls
  `gpg_key_id`, and a subkey-signed row never carries one, so
  `InvalidGpgSignatureUpdater` reaches exactly those rows: the remedy the
  original reporter was told to apply, deleting the key and adding it again, is
  what puts a row in that state. The specs pin both halves.

**How we found it**: not from this codebase. The maintainer brought the report
in from elsewhere and the investigation was done here, against a GitLab checkout
with fixtures generated by GnuPG 2.2.40. It is recorded under the rule above for
a fix we carry upstream in our name.

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

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1264](https://github.com/modelcontextprotocol/go-sdk/issues/1264),
  on 2026-09-13, as a proposal: `GetID()` on the `Request` interface or a field
  on `RequestExtra`, with the trade-off between them stated, since adding a
  method to an exported interface breaks anything outside the package that
  implements it.
- **In review**: no. Waiting on the maintainers to say which shape they want.
- **Merged**: no.
- **Blocking**: no. One Conditionally Required attribute is omitted; the span is
  otherwise complete and the metric does not carry the attribute at all.
- **Workaround**: none possible. Nothing in the public API exposes the value.

### A WithOptions delegation sends null as the request body

`Client.NewRequestToURL` decides whether a request carries a body with
`if opt != nil`, where `opt` is an `any`. A typed nil pointer held in an
interface is not equal to `nil`, so any caller that reaches it with a nil
`*SomeOptions` marshals that pointer instead of sending nothing, and
`json.Marshal` of a nil pointer is the four bytes `null`.

v3.12.0 reached it from inside the SDK for the first time. It gave two methods
a `WithOptions` sibling and made the old name delegate to it with a nil options
pointer:

```go
func (s *DraftNotesService) PublishAllDraftNotes(pid any, mergeRequest int64, options ...RequestOptionFunc) (*Response, error) {
	return s.PublishAllDraftNotesWithOptions(pid, mergeRequest, nil, options...)
}
```

The sibling passes that nil through `withAPIOpts(opt)`, so
`POST /projects/:id/merge_requests/:iid/draft_notes/bulk_publish` went from a
body-less request under v3.0.0 to one carrying `null` with `Content-Length: 4`.
Measured against an `httptest` server with both versions:

| Method                            | v3.0.0                         | v3.12.0                            |
| --------------------------------- | ------------------------------ | ---------------------------------- |
| `DraftNotes.PublishAllDraftNotes` | body `""`, `Content-Length: 0` | body `"null"`, `Content-Length: 4` |
| `Jobs.GetJobArtifactsWithOptions` | query `""`                     | query `""`                         |

`GetJobArtifacts` took the same delegation and is unharmed only by accident:
its request is a GET, so the nil goes to `query.Values`, which returns early on
a nil pointer, and the empty `RawQuery` adds no `?`. The defect is confined to
the methods whose verb makes `NewRequestToURL` take the marshalling branch.

Nothing is expected to break at GitLab, which is why it is not blocking:
`Grape::Middleware::Formatter` sets the form hash only `if body.is_a?(Hash)`,
and `null` parses to `nil`, so the endpoint sees the same empty parameter set
either way. That is read from Grape's formatter rather than measured against a
live instance, so it is the reason this is not urgent and not a claim that the
bytes are identical. It is still a request the SDK did not mean to send, and it
will reach any future delegation of the same shape on a POST, PUT or PATCH.

The fix is a nil-pointer check where the decision is made, in
`NewRequestToURL`, rather than at each delegation, since the next one will be
written the same way.

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none taken. Reaching the body-less shape again would mean
  calling `PublishAllDraftNotesWithOptions`, which is marked `Deprecated:`
  upstream and would trip `staticcheck` SA1019 under this repository's
  `checks: all`, to buy bytes GitLab ignores.
  `TestDraftNotePublishAll_SendsNoPublishParameters` in
  `internal/tools/mrdraftnotes` pins what the call sends instead, and accepts
  either spelling of an empty body so an upstream fix does not fail the suite.

### Seven more option structs send an optional param on every call

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: not measured. The tag defect is certain and read from the
  source below; whether GitLab minds the null is per endpoint and was measured
  for none of these seven. Entry 52's was measured and GitLab does mind it
  (`422 Package type can't be blank`), so the class is not theoretical.
- **Workaround**: none. Nothing here carries one, and none is possible from a
  handler: the struct tag is what decides, so no option a caller passes
  suppresses the key.

**Where**: seven option structs across client-go.

**What**: the same defect as entries 6 and 52, found systematically rather
than one at a time. `audit_1to1 -scope=paths` compares the keys
`encoding/json` writes whatever a handler set against the params GitLab's live
record marks optional, and reports eleven fields in eight option types. One of
the eight is entry 52. These are the other seven:

| Option type                           | Field        | Param         | Endpoint                                                  |
| ------------------------------------- | ------------ | ------------- | --------------------------------------------------------- |
| `CreateDependencyListExportOptions`   | `ExportType` | `export_type` | `POST /pipelines/:id/dependency_list_exports`             |
| `CreateGroupIssueBoardListOptions`    | `LabelID`    | `label_id`    | `POST /groups/:id/boards/:id/lists`                       |
| `AddGroupMemberOptions`               | `ExpiresAt`  | `expires_at`  | `POST /groups/:id/members`                                |
| `AddProjectMemberOptions`             | `ExpiresAt`  | `expires_at`  | `POST /projects/:id/members`                              |
| `ShareWithGroupOptions`               | `ExpiresAt`  | `expires_at`  | `POST /projects/:id/share`, `POST /groups/:id/share`      |
| `CreateIssueLinkOptions`              | `LinkType`   | `link_type`   | `POST /projects/:id/issues/:iid/links`                    |
| `EditPipelineScheduleVariableOptions` | `Value`      | `value`       | `PUT /projects/:id/pipeline_schedules/:id/variables/:key` |

**Two of them are a split tag, not a missing one**, which is worth separating
because it reads as a fix somebody began and did not finish:

```go
ExpiresAt *string `url:"expires_at,omitempty" json:"expires_at"`
```

That is `AddGroupMemberOptions` and `AddProjectMemberOptions`. The `url` half
omits the key and the `json` half does not, so the same field is absent from a
query-encoded call and present as `null` in a JSON body. Every other field of
both structs carries `omitempty` on both halves.

The remaining five simply lack it.
`EditPipelineScheduleVariableOptions` has the shape entry 52 has, one field
with the tag and one without (`VariableType` carries it, `Value` does not).
`CreateIssueLinkOptions` carries no `url` tags at all, so only the JSON body
is affected. `ShareWithGroupOptions` is the one reached from three packages,
since `projects`, `groups` and `groupmembers` all call it.

**Why entry 6 is not in this list.** `SetFeatureFlagOptions` is the same
defect and does not appear, because the audit reads the request this server
**recorded** and `internal/tools/features.Set` builds its body by hand to
avoid the bug. The workaround hides the defect from the check that would have
found it, which is a property worth knowing before trusting the count: the
eight are the ones we still send through the SDK, not the eight that exist.

**Effort**: small, and one merge request covers all nine of entries 6, 52 and
these seven. Every case is a struct tag plus a test that the key is absent
when the field is nil.

### A permission refusal is answered 401 rather than 403

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The call is correctly refused and nothing is served that
  should not be. What breaks is the explanation a client can give.
- **Workaround**: yes, in two places that read one rule,
  `UnauthorizedNamesCredential` in `internal/gitlab/credential_refusal.go`: a
  401 names the credential when its REST body carries the RFC 6750 code
  `invalid_token`, which only GitLab's API guard writes, or when the GraphQL
  endpoint answered it, which has no permission 401 to confuse it with.
  `internal/toolutil/errors.go` reads it to describe a 401:
  `ClassifyHTTPStatus(401)` names both causes and how to tell them apart, and
  `ClassifyError` names the credential alone when the rule says GitLab did.
  The HTTP pool (`internal/serverpool`) reads it to decide what a refused call
  means for the caller's pooled credential: a 401 naming the credential ends
  the entry at once, and one naming nothing is first put to the credential
  probe (`GET /api/v4/user`, at most once per 30 seconds per credential),
  which keeps the entry when GitLab still accepts the token. A handler's hint
  then names the permission, keyed on `toolutil.IsPermissionRefusal`, which
  reads the same file's `RefusalMayBePermission`: a REST 401 or 403 whose body
  carries no RFC 6750 error code and whose message is not the API guard's
  refusal of an account it will not serve (blocked, deactivated and the
  like), so it is never true of an answer the rule above says names the
  credential, and a hint keyed on it never follows the verdict that the token
  itself was refused
  ([issue 908](https://github.com/jmrplens/gitlab-mcp-server/issues/908)).
  Two kinds of action are not keyed on it. The four group SAML link actions
  still add their hint to every error, a rejected token included, because the
  licensed end-to-end suite quotes that wording; and the two self-rotations
  keep a 401 hint about the calling token, which agrees with that verdict
  rather than contradicting it.
  `TestPermissionRefusedWith401_EveryServedUnauthorizedRoute_CarriesAHint` in
  `internal/tools/action_catalog_test.go` drives every served action of the
  Where list below, one row per action and site: all of them must carry a
  suggestion on Grape's refusal, and all but those six must carry none after
  a revoked token, the six being exempt from that half. What retires it is
  GitLab answering 403 at these sites, after which a REST 401 without that
  code would again mean an unusable credential alone and the probe would have
  nothing left to tell apart. A handler that keys one hint on the predicate
  alone needs no change then, since the predicate reads a plain 403 the same
  way. The handlers that pair it with a status do, because each reads the
  status as the cause: the three security settings routes read a 403 first as
  the license, an archive or an enforced setting, so a role refusal moved to
  403 would get the license hint; the three external status check merge
  request routes read a 403 as the role, so the license refusal moved to 403
  would get the role hint; the fork link would give a target namespace refusal
  the Owner hint; and the merge train add would lose its hint. They are the
  ones to revisit when this entry is retired.

**Where**: `lib/api/merge_request_approvals.rb:105` and 148,
`lib/api/merge_requests.rb:896` and 945, `lib/api/remote_mirrors.rb:12`,
`lib/api/resource_access_tokens.rb:32`, 63 and 200,
`lib/api/resource_access_tokens/self_rotation.rb:46`,
`lib/api/personal_access_tokens.rb:73` and 107,
`lib/api/helpers/personal_access_tokens_helpers.rb:80`,
`lib/api/award_emoji.rb:124`, `lib/api/groups.rb:90`,
`lib/api/projects.rb:925`, `ee/lib/api/status_checks.rb:16` and 67,
`ee/app/services/external_status_checks/update_service.rb:40`,
`ee/lib/api/merge_trains.rb:168`, `ee/lib/api/security_scans.rb:54`,
`ee/lib/api/project_security_settings.rb:30` and 53,
`ee/lib/api/group_security_settings.rb:36`, `ee/lib/api/saml_group_links.rb`
(four sites), `ee/lib/ee/api/helpers.rb:193`, and
`lib/api/ml/mlflow/api_helpers.rb:15` and 23. Read at 19.4.0-pre
(`b183f4fad4bd`, 2026-09-22). Of these thirty, this server serves an action
for every one but `security_scans.rb:54` (client-go has no wrapper for it),
`ee/lib/ee/api/helpers.rb:193` (defined, and called from nowhere at that
commit) and the two MLflow helpers.

**What**: GitLab's API helper `unauthorized!` (`lib/api/helpers.rb`)
renders 401 through Grape's `error!`, and these sites call it to
refuse an **authenticated** user who lacks a permission. Eighteen of them
guard a `can?` or `can_*?` predicate on `current_user`; the approve endpoint
calls it on a falsy service result, which is the same thing one layer down.
The three sites read on the second pass, `resource_access_tokens.rb:200`
and `personal_access_tokens.rb:73` and 107 (rotating a resource access token,
and reading or rotating a personal access token by id), refuse the same way
behind `Ability.allowed?`, and tell only an administrator `not_found!`
instead. The eight read on the third pass, while fixing the handlers
([issue 908](https://github.com/jmrplens/gitlab-mcp-server/issues/908)),
are the same refusal under other guards: `personal_access_tokens_helpers.rb:80`
behind `Ability.allowed?` for a `user_id` naming someone else; `groups.rb:90`
behind the group permission check a runner administrator passes only for the
runner setting; `award_emoji.rb:124` for an award somebody else gave;
`projects.rb:925` for a fork target namespace, with the reason
`Target Namespace`; `merge_trains.rb:168` for a service that said
`:forbidden`; `resource_access_tokens/self_rotation.rb:46` for a bot token
that is not one of the resource's; `update_service.rb:40`, a service that
builds its own 401 for a missing role and hands it to `render_api_error!`;
and `status_checks.rb:16`, a before-block that answers every external status
check route with 401 when the project's namespace lacks the licensed feature,
which is a license rather than a role and is reachable on GitLab.com, where
the plan is the namespace's. RFC 9110 gives 401 for a request that lacks
valid authentication credentials and 403 for one the server understood and
refuses to authorize, so every one of these is the second answered as the
first.

It is not accidental, at least at the approve endpoint, whose own `desc` block
declares the failure:

```ruby
failure [
  { code: 404, message: 'Not found' },
  { code: 401, message: 'Unauthorized' }
]
```

So this is a design complaint rather than a bug report, which is the honest
way to file it. GitLab's own REST API is not consistent with itself here:
`forbidden!` appears 221 times against `unauthorized!`'s 94, and the
neighbouring endpoints of several of these sites use it.

**What it costs a client.** A refusal that says 401 is indistinguishable from
an expired token unless the reader knows the endpoint, so a generic client
tells its user to check their credentials when the real answer is "you wrote
this merge request". Measured here: the licensed end-to-end run approves a
merge request with the credential that opened it, a licensed instance ships
"Prevent approval by author" on, and GitLab answers

```text
POST /api/v4/projects/109/merge_requests/1/approve: 401 {message: 401 Unauthorized}
```

which this server rendered as `authentication failed: GITLAB_TOKEN may be
invalid or expired` followed by the hint that contradicts it. The list above
is not a corner: it covers merge, cancel auto-merge, approve, reset approvals,
adding to a merge train, project mirrors, access token reads, lists and
rotation, external status checks, security settings, group SAML links, award
emoji removal, group updates and fork links, all of which this server serves.

**Our half of it.** `httpStatusDescriptions` in `internal/toolutil/errors.go`
mapped 401 to a sentence about the token, which was right for a genuine
authentication failure and wrong for every site above. It was fixed without
waiting for upstream in
[issue 905](https://github.com/jmrplens/gitlab-mcp-server/issues/905), as the
Workaround field describes, since it is the half a model actually reads. Two
local consequences of the same upstream choice were tracked apart. HTTP mode
evicted a valid credential's pool entry on a permission 401, which ended its
subscriptions with a false "re-authenticate" and counted a revocation that
never happened; that is fixed
([issue 907](https://github.com/jmrplens/gitlab-mcp-server/issues/907)) by
confirming a 401 that names nothing with the credential probe before the
entry goes, as the Workaround field describes. The handlers' own hints were
the third: most of the handlers behind these sites scoped their permission
hint to the 403 GitLab never sends there, or carried none, and the one that
hinted on any 401 (the approve) followed the rejected-token verdict with a
self-approval suggestion. They are fixed
([issue 908](https://github.com/jmrplens/gitlab-mcp-server/issues/908)) by
keying each hint on `toolutil.IsPermissionRefusal`, except the group SAML
links and the self-rotations the Workaround field names, and reading the fix
against GitLab's source corrected what several hints and served usages said
as well: push mirrors are available on every tier, external status checks
need Ultimate rather than Premium, a group's security settings need
Maintainer or Security Manager rather than Owner, the approval reset is
refused with 401 for a person's token rather than 404 and admits a service
account's, a rotation by id is refused whatever the role when the calling
token is itself a project or group access token, and the reads and rotations
of project and group access tokens are all refused whatever the role when an
administrator has disabled personal access tokens on the instance. The same
reading found two status check routes whose role refusal never arrives as a
401: the delete discards it and answers 204 (entry 56), and the create
answers it with 500 (entry 57).

### Deleting an external status check without the role answers 204 and deletes nothing

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no, but the answer is false: a caller told the check was
  deleted goes on as if it were.
- **Workaround**: partial. Nothing on the wire tells this refusal apart from
  a real deletion, so the handler cannot report it. The action's served usage
  says so instead, and asks the caller to confirm a deletion with
  `external_status_check.list_project`; and
  `DeleteProjectExternalStatusCheck` in
  `internal/tools/externalstatuschecks/external_status_checks.go` hints only
  the license on a refusal, never the role, since GitLab never refuses the
  role there. What retires it is GitLab rendering the service's refusal.

**What**: `DELETE /projects/:id/external_status_checks/:check_id`
(`ee/lib/api/status_checks.rb:122-132`) wraps
`ExternalStatusChecks::DestroyService#execute` in `destroy_conditionally!`.
The service refuses a caller without `delete_external_status_check`, which
is the Maintainer role, by returning an error response with
`http_status: :unauthorized`
(`ee/app/services/external_status_checks/destroy_service.rb:8` and 27-33).
`destroy_conditionally!` (`lib/api/helpers.rb:53-65`) sets the status to 204
and the body to empty **before** it yields, and discards what the block
returns, so the refusal never reaches the response: a Developer's delete is
answered 204 and the check is still there. The update beside it hands the
same kind of service error to `render_api_error!` and answers 401 correctly
(`status_checks.rb:106-110`), which is the shape the delete is missing. Read
at 19.4.0-pre (`b183f4fad4bd`, 2026-09-22).

**How we found it**: reading every 401 the status check routes can answer
while fixing their hints
([issue 908](https://github.com/jmrplens/gitlab-mcp-server/issues/908)). The
delete's service builds a 401 like the update's, and following it to the
response showed it goes nowhere.

### Creating an external status check without the role answers 500

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The check is correctly not created. What breaks is the
  answer: a refusal of the caller arrives as a fault of the instance, which a
  client retries or reports as an outage.
- **Workaround**: yes. `CreateProjectExternalStatusCheck` in
  `internal/tools/externalstatuschecks/external_status_checks.go` reads a 500
  whose message carries the service's `Not allowed` as the role refusal it is
  (`createRefusedForRole`) and hints the Maintainer role, and
  `TestStatusChecks_CreateRefusedWith500NotAllowed_NamesTheMaintainerRole`
  holds it, beside a 500 without that message, which names no role. What
  retires it is the service giving its refusal a status.

**Where**: `ee/app/services/external_status_checks/create_service.rb:32-38`,
rendered by `ee/lib/api/status_checks.rb:53`. Read at 19.4.0-pre
(`b183f4fad4bd`, 2026-09-22).

**What**: `POST /projects/:id/external_status_checks` runs
`ExternalStatusChecks::CreateService#execute`, which refuses a caller without
`create_external_status_check`, granted to the Maintainer role
(`config/authz/roles/maintainer.yml:84`), with `access_denied_error`: a `ServiceResponse.error`
carrying `reason: :access_denied`, the errors `['Not allowed']`, and no
`http_status`, which `ServiceResponse.error` defaults to `nil`
(`app/services/service_response.rb:13`). The route hands that status to
`render_api_error!(response.payload[:errors], response.http_status)`, which
reaches Grape's `error!` with a `nil` status (`lib/api/helpers.rb:720-733`),
and Grape 2.4.0 answers a `nil` status with its default error status, 500.
So a Developer's create is answered `500 {"message":["Not allowed"]}`. The
update beside it builds its refusal with `http_status: :unauthorized` and is
answered 401 (`ee/app/services/external_status_checks/update_service.rb:40`),
and the delete loses it the other way (entry 56). The route's own spec
(`ee/spec/requests/api/status_checks_spec.rb`, "when feature is disabled,
unlicensed or user has permission") drives only the owner and a user who is
not a member, whom `user_project` answers 404 before the service runs, so no
test of GitLab's reaches the service's refusal.

**How we found it**: reviewing the fix for
[issue 908](https://github.com/jmrplens/gitlab-mcp-server/issues/908), by
reading where each status check route's role refusal ends up, after entry 56
had shown one of them going nowhere.
