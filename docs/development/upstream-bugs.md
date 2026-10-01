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
| 5 | client-go | [`GetNamespace` breaks on a path lookup](#getnamespace-cannot-decode-a-path-based-lookup) | No, not reproduced | No | No | No | Retired |
| 6 | client-go | [`SetFeatureFlagOptions` lacks `omitempty`](#setfeatureflagoptions-fields-lack-omitempty) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 7 | client-go | [`ApplicationStatistics` assumes numeric JSON](#applicationstatistics-assumes-numeric-json) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 8 | go-sdk | [No SSE keep-alive option](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions) | Yes, [#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262) | Yes, theirs, [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232), merged; ours, [modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293), open | Partly, by [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232), not ours, unreleased | No | Yes |
| 9 | go-sdk | [A malformed message ends the session](#a-malformed-message-ends-the-session-instead-of-answering--32700) | Yes, by another user | Yes, theirs, open | No | Was yes | Yes |
| 10 | go-sdk | [Cannot send `notifications/cancelled` for a listen stream](#application-code-cannot-send-notificationscancelled-for-a-listen-stream) | Yes, [#1263](https://github.com/modelcontextprotocol/go-sdk/issues/1263) | No, proposal first | No | No | None possible |
| 11 | go-sdk | [Declared, not negotiated, version selects MRTR](#the-declared-protocol-version-not-the-negotiated-one-selects-mrtr) | Yes, [#1258](https://github.com/modelcontextprotocol/go-sdk/issues/1258) | Yes, [#1266](https://github.com/modelcontextprotocol/go-sdk/pull/1266), open | No | No | None taken |
| 12 | go-sdk | [A cancelled call is still answered](#a-cancelled-incoming-call-is-still-answered) | Yes, [#1259](https://github.com/modelcontextprotocol/go-sdk/issues/1259) | Yes, [#1267](https://github.com/modelcontextprotocol/go-sdk/pull/1267), open | No | No | Partial |
| 13 | go-sdk | [The cancellation reason is discarded](#the-cancellation-reason-is-discarded-before-any-handler-sees-it) | Yes | Yes, [#1255](https://github.com/modelcontextprotocol/go-sdk/pull/1255), merged | **Yes, unreleased** | No | Yes, until it ships |
| 14 | go-sdk | [`Mcp-Name` compared without decoding](#mcp-name-is-compared-without-decoding-the-base64-sentinel) | Yes, by another user, [modelcontextprotocol/go-sdk#1234](https://github.com/modelcontextprotocol/go-sdk/issues/1234) | Yes, theirs, [modelcontextprotocol/go-sdk#1242](https://github.com/modelcontextprotocol/go-sdk/pull/1242), merged | **Yes, unreleased** | No | None taken |
| 15 | go-sdk | [Protocol version classified by string ordering](#the-protocol-version-is-classified-by-string-ordering) | Yes, [#1260](https://github.com/modelcontextprotocol/go-sdk/issues/1260) | Yes, [#1268](https://github.com/modelcontextprotocol/go-sdk/pull/1268), merged | **Yes, unreleased** | No | None taken |
| 16 | go-selfupdate | [Deprecated `x/crypto/openpgp`](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp) | Yes | Yes, open | No | No | Retired |
| 17 | codex | [Non-integer `priority` breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call) | Yes, [openai/codex#38979](https://github.com/openai/codex/issues/38979), and the cause in rmcp, [modelcontextprotocol/rust-sdk#1299](https://github.com/modelcontextprotocol/rust-sdk/issues/1299) | Yes, [modelcontextprotocol/rust-sdk#1300](https://github.com/modelcontextprotocol/rust-sdk/pull/1300), merged | **Yes, rmcp 3.5.0**; Codex still pins 3.2.0 | Was yes | Yes, until a Codex built on the fix is widely deployed, not merely released |
| 18 | go-sdk | [A receiving middleware cannot read the JSON-RPC id](#a-receiving-middleware-cannot-read-the-json-rpc-request-id) | Yes, [#1264](https://github.com/modelcontextprotocol/go-sdk/issues/1264) | No, proposal first | No | No | None possible |
| 19 | client-go | [Security mutations discard GraphQL errors](#the-security-attribute-and-category-mutations-discard-graphql-errors) | Yes | Yes, [gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066), open | No | No | Yes |
| 20 | client-go | [Dependency Firewall lacks `operation` and the enablement endpoint](#the-dependency-firewall-wrapper-is-missing-an-attribute-and-an-endpoint) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | None |
| 21 | go-sdk | [A middleware cannot ask whether a request carries params](#a-middleware-cannot-ask-whether-a-request-carries-params) | Yes, [#1261](https://github.com/modelcontextprotocol/go-sdk/issues/1261) | Yes, [#1269](https://github.com/modelcontextprotocol/go-sdk/pull/1269), merged | **Yes, unreleased** | No | Yes |
| 22 | client-go | [Enum constants lag the documented value sets](#enum-constants-lag-the-documented-value-sets) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 23 | go-sdk | [No per-session resource-updated delivery](#a-resource-update-cannot-be-delivered-to-one-session) | Yes, [#1265](https://github.com/modelcontextprotocol/go-sdk/issues/1265) | No, proposal first | No | No | Yes |
| 24 | gitlab-org/gitlab | [Approvals page documents the POST's response under the GET](#the-merge-request-approvals-page-documents-the-deprecated-posts-response-under-the-get) | No | No | No | No | Yes |
| 25 | client-go | [`CreateProjectForkRelation` declares a response GitLab does not send](#createprojectforkrelation-declares-a-response-gitlab-does-not-send) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 26 | client-go | [The invitations wrapper is missing two parameters and a response field](#the-invitations-wrapper-is-missing-two-parameters-and-a-response-field) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 27 | client-go | [The achievements fragments select less than the schema offers](#the-achievements-fragments-select-less-than-the-schema-offers) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | None possible |
| 28 | client-go | [The epics wrapper is missing two filters and twelve response fields](#the-epics-wrapper-is-missing-two-filters-and-twelve-response-fields) | Yes | Yes, in part, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | Was yes | Yes |
| 29 | client-go | [The note and discussion structs miss what GitLab sends and declare what it does not](#the-note-and-discussion-structs-miss-what-gitlab-sends-and-declare-what-it-does-not) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 30 | client-go | [The member structs, options and services miss what GitLab sends, accepts and serves](#the-member-structs-options-and-services-miss-what-gitlab-sends-accepts-and-serves) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 31 | client-go | [Six response structs miss a field GitLab sends on every object](#six-response-structs-miss-a-field-gitlab-sends-on-every-object) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 32 | client-go | [No token struct carries the granular fields, and the impersonation and resource ones carry less still](#no-token-struct-carries-the-granular-fields-and-the-impersonation-and-resource-ones-carry-less-still) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 33 | client-go | [The four Sidekiq routes carry a leading slash](#the-four-sidekiq-routes-carry-a-leading-slash-and-send-a-double-slash) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | None |
| 34 | client-go | [Response structs that miss a field GitLab sends unconditionally](#response-structs-that-miss-a-field-gitlab-sends-unconditionally) | Yes | Yes, the gaps held back in [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | **14 of 14; all released, v3.1.0 to v3.15.0** | No | Retired for 13 at the v3.14.0 pin; `systemhooks` keeps its own until the pin reaches v3.15.0 |
| 35 | client-go | [The Geo structs model a fraction of a site and its status, and the repair method names the wrong entity](#the-geo-structs-model-a-fraction-of-a-site-and-its-status-and-the-repair-method-names-the-wrong-entity) | In part in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300), whole in [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 36 | client-go | [The merge request structs miss six keys, unevenly, and two methods name an entity they do not answer with](#the-merge-request-structs-miss-six-keys-unevenly-and-two-methods-name-an-entity-they-do-not-answer-with) | In part in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300), whole in [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, in part, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 37 | client-go | [The User struct models one user entity and GitLab serves six](#the-user-struct-models-one-user-entity-and-gitlab-serves-six) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) and [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 38 | gitlab-org/gitlab | [Three job token scope endpoints declare a response entity they do not send](#three-job-token-scope-endpoints-declare-a-response-entity-they-do-not-send) | Yes | Yes, [gitlab-org/gitlab!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698), merged | **Yes, 19.4.0** | No | No, retired when the live record was taken from 19.4.1-ee |
| 39 | gitlab-org/gitlab | [Two project group listings are annotated with the whole Group entity](#two-project-group-listings-are-annotated-with-the-whole-group-entity) | Yes | Yes, [gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699), merged | **Yes, unreleased** | No | Yes |
| 40 | client-go | [Ten modelled fields that no Grape entity exposes](#ten-modelled-fields-that-no-grape-entity-exposes-removed-from-this-servers-output) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Not needed |
| 41 | client-go | [IssueRelation models an issue basic where GitLab renders a whole issue](#issuerelation-models-an-issue-basic-where-gitlab-renders-a-whole-issue) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) and [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, in part, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 42 | client-go | [MemberRole models twenty of the forty-five permissions GitLab sends](#memberrole-models-twenty-of-the-forty-five-permissions-gitlab-sends) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) and [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 43 | client-go | [PipelineInfo decodes two entities and models only the smaller one](#pipelineinfo-decodes-two-entities-and-models-only-the-smaller-one) | Yes, in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300) and [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, in part, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 44 | client-go | [Group, Project and Issue each model one entity where GitLab renders two](#group-project-and-issue-each-model-one-entity-where-gitlab-renders-two) | In part in [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300), whole in [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) | Yes, in part, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 45 | client-go | [The work item get, create and update documents select licensed fields](#the-work-item-get-create-and-update-documents-select-licensed-fields) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | Yes, on Community Edition | None possible |
| 46 | gitlab-org/gitlab | [Cancelling an auto-merge answers a status hash under a merge request annotation](#cancelling-an-auto-merge-answers-a-status-hash-under-a-merge-request-annotation) | Yes | Yes, [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) merged and [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704) open; [gitlab-org/gitlab!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239) closed unmerged | **Half, unreleased**: the new endpoint, in milestone 19.5 | Was yes | Yes |
| 47 | gitlab-org/gitlab | [A revoked GPG UID still verifies commits](#a-revoked-gpg-uid-is-still-offered-for-verification-and-still-verifies-commits) | Yes, by another user, [gitlab-org/gitlab#24572](https://gitlab.com/gitlab-org/gitlab/-/work_items/24572) | Yes, [gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300), merged | **Yes, unreleased** | No | None possible |
| 48 | go-sdk | [Two listens on one URI leave a session receiving neither](#a-sessions-second-listen-on-a-uri-overwrites-the-firsts-subscription-and-its-close-deletes-both) | No issue; named as a known limitation of [modelcontextprotocol/go-sdk#1275](https://github.com/modelcontextprotocol/go-sdk/pull/1275) by another user | No | No | No | Partial |
| 49 | go-sdk | [Three methods served before the initialize handshake](#three-methods-are-served-on-a-legacy-session-before-the-initialize-handshake) | Yes, [#1271](https://github.com/modelcontextprotocol/go-sdk/issues/1271) | Yes, [#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273), merged | **Yes, unreleased** | No | None taken |
| 50 | go-sdk | [The negotiated version is recorded on one path of four](#the-negotiated-protocol-version-is-recorded-on-one-path-of-four) | Yes, [#1272](https://github.com/modelcontextprotocol/go-sdk/issues/1272) | Yes, [#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274), merged | **Yes, unreleased** | No | None taken |
| 51 | client-go | [A WithOptions delegation sends `null` as the request body](#a-withoptions-delegation-sends-null-as-the-request-body) | Yes | Yes, [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065), open | No | No | Yes |
| 52 | client-go | [`UpdatePackageProtectionRulesOptions` lacks `omitempty`](#updatepackageprotectionrulesoptions-sends-two-explicit-nulls-on-every-partial-update) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | Partly | Partial |
| 53 | gitlab-org/gitlab | [No endpoint reports the instance plan to a non-administrator](#no-endpoint-reports-the-instance-plan-to-a-non-administrator) | Yes, [gitlab-org/gitlab#630305](https://gitlab.com/gitlab-org/gitlab/-/issues/630305) | Yes, [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936), open | No | No | Yes |
| 54 | client-go | [Seven more option structs send an optional param on every call](#seven-more-option-structs-send-an-optional-param-on-every-call) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No; one latent, one narrows an action | Not needed for five; two handlers require the field, one of them until the tag changes |
| 55 | gitlab-org/gitlab | [A permission refusal is answered 401 rather than 403](#a-permission-refusal-is-answered-401-rather-than-403) | No | No | No | No | Yes |
| 56 | gitlab-org/gitlab | [Deleting an external status check without the role answers 204 and deletes nothing](#deleting-an-external-status-check-without-the-role-answers-204-and-deletes-nothing) | No | No | No | No | Partial |
| 57 | gitlab-org/gitlab | [Creating an external status check without the role answers 500](#creating-an-external-status-check-without-the-role-answers-500) | No | No | No | No | Yes |
| 58 | client-go | [Five response keys and three parameters GitLab 19.4 added](#five-response-keys-and-three-parameters-gitlab-194-added-that-v3140-does-not-model) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 59 | client-go | [PlanLimit models eight of the twenty-nine limits GitLab sends and accepts](#planlimit-models-eight-of-the-twenty-nine-limits-gitlab-sends-and-accepts) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 60 | client-go | [JobPipeline models five of the ten keys a job's pipeline carries](#jobpipeline-models-five-of-the-ten-keys-a-jobs-pipeline-carries) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 61 | client-go | [`AwardEmoji` does not model the image URL of a custom emoji](#awardemoji-does-not-model-the-image-url-of-a-custom-emoji) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 62 | client-go | [`Diff` does not model why a file diff arrives without its text](#diff-does-not-model-why-a-file-diff-arrives-without-its-text) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 63 | client-go | [`ApproveOrRejectProjectDeployment` discards the approval GitLab records](#approveorrejectprojectdeployment-discards-the-approval-gitlab-records) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 64 | client-go | [`ShareProjectWithGroup` discards the link GitLab creates](#shareprojectwithgroup-discards-the-link-gitlab-creates) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 65 | client-go | [`GroupRelationStatus` misses the object count, and a relation's status does not decode](#grouprelationstatus-does-not-model-the-object-count-and-a-relations-status-does-not-decode) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 66 | go-sdk | [A tool, prompt or resource result a middleware makes carries no `resultType`](#a-tool-prompt-or-resource-result-a-middleware-makes-carries-no-resulttype) | Yes, by another user, [modelcontextprotocol/go-sdk#1225](https://github.com/modelcontextprotocol/go-sdk/issues/1225) | Yes, theirs, [modelcontextprotocol/go-sdk#1226](https://github.com/modelcontextprotocol/go-sdk/pull/1226), merged | **Yes, unreleased** | No; without the workaround it breaks a MUST | Yes, until the bump that carries it |
| 67 | go-sdk | [A Go SDK client never sees a listen refusal](#a-go-sdk-client-never-sees-a-subscriptionslisten-refusal) | Yes, by another user, [modelcontextprotocol/go-sdk#1169](https://github.com/modelcontextprotocol/go-sdk/issues/1169) | Yes, theirs, [modelcontextprotocol/go-sdk#1170](https://github.com/modelcontextprotocol/go-sdk/pull/1170), open | No | No | None possible |
| 68 | go-sdk | [The client starts no new session after a 404](#the-go-sdk-client-starts-no-new-session-after-a-404) | Yes, [modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299) | Yes, theirs, [modelcontextprotocol/go-sdk#1300](https://github.com/modelcontextprotocol/go-sdk/pull/1300), open | No | No | None taken |
| 69 | client-go | [Commit declares `extended_trailers` a map of strings, and GitLab sends lists](#commit-declares-extended_trailers-a-map-of-strings-and-gitlab-sends-lists) | No | No | No | Was yes, for `repository.commit_list` with `trailers` | Yes, except `merge_request.commits`, `search.commits` and the readers of an embedded commit, resources, prompts and completions included |
| 70 | client-go | [The Orbit schema format is sent as `format`, and its llm answer is not modelled](#the-orbit-schema-format-is-sent-as-format-and-its-llm-answer-is-not-modelled) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 71 | gitlab-org/gitlab | [The transfer API pages do not say the answer precedes the move, or how a failure is reported](#the-transfer-api-pages-do-not-say-the-answer-precedes-the-move-or-how-a-failure-is-reported) | No | No | No | No | Yes |
| 72 | gitlab-org/gitlab | [A saved view create or subscribe from a token answers 500, and the create has already saved the view](#a-saved-view-create-or-subscribe-from-a-token-answers-500-and-the-create-has-already-saved-the-view) | Yes, by the merge request | Yes, [gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074), merged | **Yes, unreleased** | Yes | Partial |
| 73 | gobco | [gobco type-checks every file of a package directory](#gobco-type-checks-every-file-of-a-package-directory-whatever-its-build-constraints-say) | Yes, [rillig/gobco#40](https://github.com/rillig/gobco/issues/40) | Yes, [rillig/gobco#41](https://github.com/rillig/gobco/pull/41), open | No | No; it keeps the condition gate from measuring the e2e harness | Partial |
| 74 | gitlab-org/gitlab | [The Orbit API page's query examples predate version 12 of the query DSL](#the-orbit-api-pages-query-examples-predate-version-12-of-the-query-dsl) | Yes, by the merge request | Yes, [gitlab-org/gitlab!258241](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258241), open | No | No | Not yet, with issue 1031 |
| 75 | gitlab-org/orbit/knowledge-graph | [The DSL schema says a path query may omit `rel_types`](#the-dsl-schema-says-a-path-query-may-omit-rel_types) | Yes, [gitlab-org/orbit/knowledge-graph#1329](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1329) | Yes, [gitlab-org/orbit/knowledge-graph!2650](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2650), open | No | No | Not yet, with issue 1031 |
| 76 | gitlab-org/orbit/knowledge-graph | [The DSL schema says the default neighbors direction is `both`](#the-dsl-schema-says-the-default-neighbors-direction-is-both) | Yes, [gitlab-org/orbit/knowledge-graph#1330](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1330) | Yes, [gitlab-org/orbit/knowledge-graph!2651](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2651), open | No | No, but results can be silently incomplete | Not yet, with issue 1031 |
| 77 | gitlab-org/gitlab | [The context commit list is annotated with `Commit` and presents `CommitWithLink`](#the-context-commit-list-is-annotated-with-commit-and-presents-commitwithlink) | No | No | No | No | Yes |
| 78 | gitlab-org/gitlab | [An unknown severity on a pipeline's findings list answers 500](#an-unknown-severity-on-a-pipelines-findings-list-answers-500) | No | No | No | No | Yes |
| 79 | gitlab-org/gitlab | [An unknown report type on a pipeline's findings list is dropped and filters out every finding](#an-unknown-report-type-on-a-pipelines-findings-list-is-dropped-and-filters-out-every-finding) | No | No | No | No | Yes |
| 80 | gitlab-org/gitlab | [The scan profile attach mutation drops the reason it refused a name](#the-scan-profile-attach-mutation-drops-the-reason-it-refused-a-name) | No | No | No | No | Yes |
| 81 | golang/go | [go/types reads an imported generic instance another checker is expanding](#gotypes-reads-an-imported-generic-instance-another-checker-is-expanding) | Yes, by another user, [golang/go#81122](https://github.com/golang/go/issues/81122) | Yes, [golang/go#81871](https://github.com/golang/go/pull/81871), imported as [go.dev/cl/841585](https://go.dev/cl/841585), open | No | No; without the workaround it fails race runs of the tooling tests at random | Yes |
| 82 | gitlab-org/gitlab, then client-go | [The admin token route takes no granular scopes, and no client-go create option carries them](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them) | GitLab half yes, by GitLab, [gitlab-org/gitlab#630541](https://gitlab.com/gitlab-org/gitlab/-/issues/630541); client-go half no | No; attempts by other users, [gitlab-org/gitlab!245585](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585) and [gitlab-org/api/client-go!2978](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2978), closed unmerged | No | No | None taken; the five token create actions send classic scopes only, tracked in [issue 1115](https://github.com/jmrplens/gitlab-mcp-server/issues/1115) |
| 83 | gitlab-org/gitlab | [The fine-grained refusal names the missing permissions only as display labels in prose](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose) | No, drafted in its section; goes before row 84 | No | No | No | None taken; the labels are not parsed, on purpose |
| 84 | client-go | [No client-go helper returns the RFC 6750 fields of a token refusal](#no-client-go-helper-returns-the-rfc-6750-fields-of-a-token-refusal) | No, drafted in its section; waits on this project deciding to adopt the helper | No | No | No | Not needed; this server decodes the body itself |
| 85 | gitlab-org/gitlab | [The fine-grained refusal can name a deprecated permission's label](#the-fine-grained-refusal-can-name-a-deprecated-permissions-label) | No, not yet reproduced on a running instance | No | No | No | None taken |

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
upstream by somebody else, for the listen stream alone, and ours,
[modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293),
was opened the same day on the shape agreed on the issue. Rows 8 and 14 now count another user's merged pull
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

Re-verified on 2026-09-25 against the trackers and the tags. One row moved:
[gitlab-org/api/client-go!3052](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3052)
merged at 18:46 UTC on 2026-09-24 and is in **v3.14.0**, tagged thirteen
minutes later, so row 34 reads 13 of 14 and every merged client-go row is
released. The pin moved from v3.12.0 to v3.14.0 on 2026-09-25, so it lags no
merge this file records, and the `Package` fields that merge request added are
read off the struct rather than the capture. Every `client-go` row was re-read
against the v3.14.0 source rather than the tracker, and only row 34 moved:
between the two tags the non-test Go sources differ in `gitlab.go`, which
gains `StatusCode`, and in `packages.go` with its generated mock,
`testing/packages_mock.go`, which are that merge request, so no struct, method
or route another row names changed. v3.13.1, tagged at 18:23 the same day, carries
nothing a row waits on; its fix is to the separate `config` module, which this
server does not import. The
fork pipeline of
[gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702)
failed `cells-routes:router-in-sync` again, on routes `master` added after the
branch was rebuilt rather than on the route the router snapshot refresh added,
and row 46's section says what clears it. Six merge requests are open, and no
reviewer has commented on any of them since.

Re-verified on 2026-09-27 against the trackers and the tags, the day the joint
client-go merge request row 34's section describes was opened:
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
36 commits on v3.14.0, one per entry. Every client-go row it carries now
reports it under Reported and In review, 6, 7, 19, 20, 22, 25 to 33, 35 to 37,
40 to 45, 51, 52, 54 and 58 to 65, and row 34 counts the gaps it had held back.
Rows 61 to 65 are new: those five entries were written for
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971) and had a
section here and no row, the drift the first of these paragraphs describes.
Row 5 moved the other way: it could not be reproduced, so the merge request
leaves it out, and its section says what that means for the workaround here.
Rows 22 and 43 each carry a correction the merge request's commits turned up,
and row 7 one about this server's own workaround. Row 46 moved most: its merge
request was approved and put on a merge train, the router job failed it, and
the rebase asked for after that failed it again on routes `master` does not
have yet, so what its section said would clear the job was wrong. Row 34's
open merge request moved a step: a code owner ran its merged-results pipeline
in the canonical project, which passed. Every other open merge request and
pull request the file follows is where its section left it, except that row
11's pull request is behind `main` and owes a comment rewrite promised on
2026-09-15, and row 17's fix is in the release pull request for rmcp 3.5.0,
which is not tagged. Two upstream indexes are behind, the umbrella's
description and the go-sdk index, and row 34's section and the go-sdk section
say where. go-sdk v1.8.0 is still that SDK's newest tag and gitlab-org/gitlab
19.5 is not cut, so every merge here that was unreleased still is.

Rows 66 to 68 were added on 2026-09-27 for
[issue 961](https://github.com/jmrplens/gitlab-mcp-server/issues/961), which
tracks the go-sdk behaviours the issue 565 tenant policy specification recorded
as findings F-20, F-21, F-22 and F-24; F-24 is row 10. Their five fields were
read off the trackers that day, and every link of the other go-sdk rows was read
again with them. One had moved since the 25th: a comment on
[modelcontextprotocol/go-sdk#1209](https://github.com/modelcontextprotocol/go-sdk/issues/1209)
reports that the pull request under row 9 leaves part of that defect open, which
row 9 now says. v1.8.0 is still that SDK's newest tag, 24 commits behind
`main`, so every go-sdk merge here is still unreleased. Row 68's section records
what its tracker does not say, measured the same day: this server's 404 meets
the Go SDK client on three paths with two outcomes, and the pull request
proposed for it reaches two of them. Each of the three, and row 10, is now held
by a test that fails on the change that retires it, named in its section.

Re-verified on 2026-09-28 against the trackers and the tags: every merge
request, pull request and issue the file links, the tags of each dependency,
and every issue, merge request and pull request the maintainer's accounts
touched on the 27th and the 28th, so that nothing done upstream in those two
days is missing here. Rows 73 to 76 are that day's contributions: gobco,
issue and pull request, and the Orbit query DSL documentation, one merge
request to `gitlab-org/gitlab` and an issue with its merge request for each
of the two schema defects in `gitlab-org/orbit/knowledge-graph`. Row 17 moved
furthest: rmcp 3.5.0 was tagged that day and carries the fix, the comment owed
on the Codex issue was posted, and Codex's `main` still pins rmcp 3.2.0, so the
workaround stays. Its section also records the `openai-mcp` measurement of
[issue 1044](https://github.com/jmrplens/gitlab-mcp-server/issues/1044). Row
72's fix changed under review, from `restore_attributes` to
`clear_attribute_changes`, and its section now describes the second. Rows 11,
34, 39, 46, 53 and 68 each moved a step, recorded in their sections: row 11's
pull request carries the two sites it had left out and the promised comment
rewrite, and is behind `main` again; row 68's pull request took our review
change as a commit; row 53's merge request had its first backend review and our
first round of changes; row 46's is approved and waits on routes `master` does
not have yet. Two of them had waited on us, and both were answered that
evening with the maintainer's go-ahead: row 34's
`gitlab-org/api/client-go!3048`, where the maintainer review's one suggestion
is applied and the test updated, and row 39's `gitlab-org/gitlab!254699`, whose
new pipeline had run on the branch's stale base rather than on the merged
result, which the reply explains. Both upstream indexes that had fallen behind, the
client-go umbrella and the go-sdk index, were rewritten on the evening of the
27th. No other row moved: go-sdk v1.8.0, client-go v3.14.0 and GitLab 19.4.1
are still each project's newest release, so every merge recorded here as
unreleased still is, and
[creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58)
has not changed since 2026-08-05. The rows the lanes of the second wave add
are not in this pass; they arrive with those lanes and take the numbers after
76.

Row 77 was added on 2026-09-28 for
[issue 1025](https://github.com/jmrplens/gitlab-mcp-server/issues/1025). It is
the class of rows 38 and 39, read from a 19.5.0-pre checkout and not yet from a
running instance, and nothing has been raised upstream for it; its section
says what a merge request would carry.

Re-verified on 2026-09-29 against the GitLab.com trackers and the tags, for
every merge request and issue this project's account has open or closed since
the last pass; the GitHub trackers were not read again. Two rows moved:
[gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
merged on 2026-09-28 and is in **v3.15.0**, so row 34 reads 14 of 14, and
[gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699)
merged at 23:59 UTC the same day and is in no tag yet. The pin stays at
v3.14.0 until the Dependabot cooldown for a Go minor ends on 2026-10-12. Rows
22, 46, 72, 75 and 76 each moved a step, recorded in their sections.

Later on 2026-09-29 row 72's merge request merged, and the GitHub trackers were
read again, for every go-sdk issue and pull request this file links. go-sdk
has cut no release since v1.8.0 and `main` is 30 commits ahead of it, so every
go-sdk merge here is still unreleased; on
[modelcontextprotocol/go-sdk#1226](https://github.com/modelcontextprotocol/go-sdk/pull/1226)
the maintainer wrote on 2026-09-14 that the next release "will happen in ~2
months". Nothing there asks anything of us. Rows 8, 11, 12, 48 and 68 moved a
step, recorded in their sections: a maintainer updated row 11's pull request
from `main`, row 68's issue was triaged and its pull request now conflicts with
`main`, the pull requests of rows 8, 11 and 12 were brought up to date with
`main` again that afternoon, and row 48's case turned out to be named upstream
as a known limitation of a pull request another contributor opened and a
maintainer merged, whose discussion also showed the entry's
claim about the transports that reach it to be too wide.

Row 81 was added on 2026-09-29 as well, and is the register's first entry
against the Go toolchain itself. It was found from this codebase, by the race
gate of a release rehearsal, and it costs the server nothing, since the server
type-checks nothing. The commands under `cmd/` that type-check this
repository's source are exposed in every build; the race detector is what turns
that into failures of their tests, and the entry says why it changes no answer
outside it.

Rows 19 and 51 moved on 2026-09-30: at the review's request each left
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
for a merge request of its own,
[gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066)
and
[gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065).
The joint merge request has 35 commits since, and every sentence that names
one of its commits as it stands uses the new numbering, while the dated
history keeps the numbers of its day. Row 34's section tells the split, and
row 19's records a finding it turned up in the project itself: its
`tests:integration` job has run no integration test since the 3.0 release.

Rows 82 to 85 were added on 2026-09-30 from a study of what client-go could
tell a caller about the fine-grained permission a request needs, which
[issue 952](https://github.com/jmrplens/gitlab-mcp-server/issues/952) wants
for fine-grained personal access tokens. GitLab sends no machine-readable name
of that permission, on success or on a refusal, so the four rows are what would
move that: GitLab creating fine-grained tokens for a user, and client-go
options to ask for one (row 82); GitLab naming the missing permissions in a
form a program can read (row 83); a client-go helper that returns the
refusal's fields as GitLab sent them (row 84); and a GitLab defect in the label
the refusal prints (row 85). None has been raised by us. Three of the six merge
requests open against client-go that day are this project's,
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
[gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065)
and
[gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066),
and a maintainer has written that the volume weighs on a team of three
([gitlab-org/api/client-go!3053 note 3810461065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3053#note_3810461065)),
so each section records the verified facts, the order, what each step waits on
and the text to send. No client-go merge request from these rows goes out
before those three are reviewed, row 83's issue goes before row 84's, which
cites it, and nothing goes out anywhere without the maintainer's approval and
an independent review of its exact text.

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
  pending on us. Read again on 2026-09-27 it is where that left it: head
  `83c6e83e` with a green fork pipeline, none of the six rules' approvals yet,
  the writer's four threads answered and theirs to resolve, and the coach,
  @narendran-kannan, not yet reviewing, though `gitlab-bot` nudged them on
  2026-09-25. The issue carries only the bot's labelling note, and nobody has
  asked us anything on either. The ready is not to be posted again, so
  nothing is owed from here.

  **On 2026-09-28** @narendran-kannan reviewed it (07:18 UTC) and requested
  changes: two blocking issues, that the GraphQL request spec did not assert
  the new field and that an instance whose Ultimate trial had expired still
  reported `ultimate`, plus a question on trial licenses, a proposal to mark
  the field as an experiment and seven suggestions and nitpicks on the EE
  specs, the REST wording and the changelog trailer, and started the AppSec
  reviewer. Every thread was answered at 10:58 UTC, and the changes went in
  as four commits: `b42a1c1f` asserts `plan` in the GraphQL metadata query
  spec, `64f08e2c` words the REST row the way the GraphQL reference does,
  `97645f2e` simplifies the EE specs as suggested, and `8ee91f87` reports
  `free` for an expired trial license, since `License#feature_available?`
  turns every feature off for one. An active trial keeps reporting its plan,
  which the reply explains is intended, and an expired paid license keeps its
  plan too, since GitLab still grants that plan's features and only blocks
  changes; the reply asks the reviewer to confirm that choice and offers to
  report `free` for any expired license instead. The experiment marker and the
  `EE: true` trailer were answered rather than applied, each with the reason.
  The fork
  pipeline on `8ee91f87` passed, and a second `@gitlab-bot ready` put it back
  in front of @narendran-kannan and @rsarangadharan; the requested-changes
  state stays until the reviewer clears it.
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
  at the v3.14.0 pin) to gain the field, or a captured-response read under
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
  now merged. It was approved by a technical writer, by a reviewer, and on
  2026-09-15 by the backend maintainer @hustewart, who set auto-merge the same
  day. It did not merge then because the pipelines that followed failed on breaks from `master`
  rather than on the change, the last of them the fork pipeline of the
  2026-09-22 rebase, whose one blocking failure was
  `generate-apollo-graphql-schema` on a schema clash that `master` fixed
  twelve minutes later, when gitlab-org/gitlab!256904 merged, after the rebase
  had landed on `e52599d0`. Auto-merge was then no longer set. A follow-up of 2026-09-24 said so and
  asked @hustewart for a fresh pipeline and auto-merge; posted as a reply
  under our own note of 2026-09-22, it turned that note into an unresolved
  thread and so added a `DISCUSSIONS_NOT_RESOLVED` blocker to a merge request
  that had none, and we resolved the thread an hour later. It then failed only
  `CI_MUST_PASS`, and waited on @hustewart to start a pipeline in the canonical
  project and set auto-merge again. Read on 2026-09-27 nothing has moved: head
  `1b339299`, the approvals of @hustewart for `/lib/` and @z_painter for
  `/doc/` standing with none left to give, no unresolved thread, and one
  working day since the note that asked. `cells-routes:router-in-sync`, which
  holds up row 46's merge request, runs only on a change to the routes and so
  not on this one, and the Danger warning about the commit body's 72 columns
  stays, as the maintainer decided on 2026-09-15. On 2026-09-28 @hustewart
  started a pipeline and set the merge train again (14:04 UTC), and it failed
  exactly as the fork pipeline of 2026-09-22 had: `generate-apollo-graphql-schema`
  stopped on "Field `ArtifactRegistryManifestDetails.referrers` already exists
  in the schema. It cannot also be defined in this type extension", the one
  failure among 321 jobs that ran. The reason is where it ran: pipeline
  2889824200 is a detached pipeline on `refs/merge-requests/254699/head`, the
  branch itself, whose base is still `e52599d0` of the 2026-09-22 rebase, which
  carries the client typedef and not its removal in
  [gitlab-org/gitlab!256904](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256904);
  the pipelines of 2026-09-15 and 16 had been merged-results pipelines instead.
  At 16:44 @hustewart asked for the failure to be addressed and for a ping to
  start another pipeline. Our reply in that thread (17:28 UTC) set that out and
  offered two ways through: a pipeline on the merged result, whose ref was
  regenerated at 17:17 UTC on a `master` that carries
  [gitlab-org/gitlab!256904](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256904),
  or a rebase onto
  current `master`, held back because the push would reset the two approvals.
  It waited on @hustewart's choice, and the rebase was the way through: the
  branch went onto current `master` as `fb3ecfdc`, the merge train pipeline
  2891419980 passed, and @hustewart merged it.
- **Merged**: yes, at 23:59 UTC on 2026-09-28 (merge commit `3cbc103f`). No
  release tag carries it yet; its milestone still says 19.4, which was released
  without it, so 19.5 is the first release that will.
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

### The context commit list is annotated with Commit and presents CommitWithLink

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The keys arrive; only the route's description, and
  everything generated from it, says they do not.
- **Workaround**: yes, two halves. `mrcontextcommits.List` reads the commits
  off the captured response (ADR-0021), which it already did for
  `extended_trailers` (row 69), and publishes four of the keys the presented
  entity adds: `author`, `author_gravatar_url`, `description_html` and
  `title_html`, on `mrcontextcommits.CommitItem`, whose doc comment says why
  each of the others is left out. The audit half is four declarations in
  `cmd/audit_1to1/internal/paths/shape_declarations.go` under
  `route-annotation-names-another-entity-than-the-handler-presents`, one per
  key, since the record holds the route under `Commit` and so reports every
  one of them as a key no response carries. They retire when
  `cmd/gen_api_live` is run against a release that carries the fix: the
  stale-declaration check then fails on all four, and the sent direction
  starts reporting the six keys this server does not publish, which will want
  declarations of their own in `sent_declarations.go`.

**Where**: `lib/api/merge_requests.rb`, the `desc` block of
`GET :id/merge_requests/:merge_request_iid/context_commits` (lines 576 to 591
at `f23a2b35383`, a 19.5.0-pre checkout).

**What**: the description says `success Entities::Commit`, and the handler
presents
`with: Entities::CommitWithLink, type: :full, request: merge_request`.
`CommitWithLink` (`lib/api/entities/commit_with_link.rb`) is `Commit` plus
`author` (a `UserPath`), `author_gravatar_url`, `commit_url` and
`commit_path`, and under `type: :full`, which the route passes,
`description_html`, `title_html`, `signature_html`, `prev_commit_id`,
`next_commit_id` and `pipeline_status_path`. The `POST` at the same path is
annotated `Entities::Commit` and presents it, correctly, so two routes on one
path answer with two entities under one annotation. `APIEntitiesCommitWithLink`
is not a component of `doc/api/openapi/openapi_v3.yaml` at all, and
`doc/api/merge_request_context_commits.md` prints the list's example body with
the `Commit` keys only.

**Read from the source, not yet measured.** Four of the ten added keys are
null on every commit this route sends, which is why they are not published
here: `prev_commit_id`, `next_commit_id` and `pipeline_status_path` read
presenter options the route does not pass, and `signature_html` renders only
for a commit with a signature, which a context commit never has, because
`MergeRequestContextCommit#to_commit` rebuilds it with `Commit.from_hash` from
the stored row and `Commit#raw_signature_type` reads the signature off a
Gitaly commit the hash does not carry. `author` is null for an email no
confirmed account holds (`Commit#lazy_author` looks it up with
`User.by_any_email(emails, confirmed: true)`), and its `show_status` is false
on every author, since that lookup does not preload the status association
`UserStatusTooltip` checks. The e2e scenario for the context commits asserts
the author and `title_html` on the list and their absence on the answer to the
`POST`, which is the first time a running instance is asked; this section is
to be amended with what it answers.

**How we found it**: surfacing the context commit keys for
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971) read the
handler rather than the annotation, and
[issue 1025](https://github.com/jmrplens/gitlab-mcp-server/issues/1025)
recorded it. The R-PATH sent report could not have: it reads the entity off
the annotation, so none of the ten keys ever reached it.

**Root cause**: the class of the entry above and of row 38. A `desc` block
naming an entity the handler does not present is invisible to every test,
because Grape uses it for documentation only.

**Effort**: small, and a candidate for the same kind of documentation merge
request as row 39: one line in `lib/api/merge_requests.rb`, the regenerated
OpenAPI document, which gains the `APIEntitiesCommitWithLink` component (and
`APIEntitiesUserPath` under it) rather than a changed `$ref` alone, and the
page's example body. Whether the four keys that are always null belong on this
route at all is a second question, left out of the finding so the annotation
stays reviewable on its own.

## GitLab client (`gitlab.com/gitlab-org/api/client-go`)

### Panic unmarshalling an issue with no id

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!3006](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3006).
- **Merged**: **yes**, on 2026-08-25 into `main`, shipped in **v2.59.1**.
- **Blocking**: it was. The panic took the process down rather than failing one
  call.
- **Workaround**: retired. The local guard went with the move to v2.62.0, and
  every v3 tag carries the fix as well; the pin is now `client-go/v3` v3.14.0.

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

- **Reported**: no, and it will not be: the check this entry asked for was made
  on 2026-09-27 and found nothing to report (below).
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
  leaves it out and says why.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: retired, by
  [issue 1021](https://github.com/jmrplens/gitlab-mcp-server/issues/1021).
  `internal/tools/namespaces.Get` used to call `GetNamespace` and, only when
  that failed with `cannot unmarshal array`, ask again through `getFromArray`
  and answer with the first element of the array. No current GitLab reached
  it for the reason it was written for. Read from the routes rather than
  measured, one request alone could reach it against GitLab 19.4 and 19.5: an
  empty `id`, which the schema's `required` does not refuse and which turns
  the path into `namespaces/`, the listing route, so the element it answered
  with was the first namespace of the caller's list rather than one anybody
  asked for. `Get` now refuses an empty or blank `id` with the
  required-parameter error before any request, and an answer that does not
  decode as one namespace is reported as the error it is; the fallback, the
  second request and the seam that built it are gone.

**What**: recorded as `GetNamespace` expecting a single JSON object while
some GitLab versions answered a path-based lookup with an array. That was
never a defect of the SDK: no version has been found that answers a path
lookup with an array, and the one array the handler ever met came from the
listing route an empty `id` reaches.

**Before reporting** asked which GitLab versions return the array, so that a
report would name a reproduction rather than a symptom. The check was made on
2026-09-27 and named none.

**Not reproduced.** `lib/api/namespaces.rb` mounts `get ':id'` with
`NAMESPACE_OR_PROJECT_REQUIREMENTS`, so a full path reaches it as well as a
numeric id, and answers either with
`present user_namespace, with: Entities::Namespace`, one object. That holds on
the 19.5 development branch read on 2026-09-27 and in the live record taken
from `19.4.1-ee` (`docs/development/gitlab-api-live.json`, `GET
/api/:version/namespaces/:id`, entity `API::Entities::Namespace`, not an
array), and client-go escapes the path into that route, so a path lookup
decodes into `Namespace` with the struct as it is. The one request answered
with an array is the empty `id` above, which is caller input rather than an
SDK defect. No version that sent an array for a path has been named, and none
is recorded anywhere this entry could cite.

### SetFeatureFlagOptions fields lack omitempty

- **Reported**: yes, as commit 32 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit gives the eight optional fields
  (`key`, `feature_group`, `user`, `group`, `namespace`, `project`,
  `repository` and `force`) `omitempty` on both halves of the tag and pins
  that a field the caller set is still sent; `Value` keeps its tags, since
  GitLab requires it.
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

- **Reported**: yes, as commit 1 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit gives `ApplicationStatistics` an
  `UnmarshalJSON` that reads each count as a JSON number or as such a string,
  and keeps the `int64` fields.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, since
  [issue 1019](https://github.com/jmrplens/gitlab-mcp-server/issues/1019).
  `internal/tools/appstatistics.Get` builds the request itself and decodes
  each count into a `delimitedCount`, which reads it the way the commit
  above's `UnmarshalJSON` does: a JSON number, or a string whose space and
  punctuation characters are dropped as group separators, a minus sign in
  the first place kept, and any other character refused. A count that does
  not parse, or does not fit an `int64`, fails the call instead of being
  published as 0. Before that issue the counts were decoded as `json.Number`,
  which refuses a string that is not a JSON number literal (`"999"` decoded
  and `"1,234"` failed with `invalid syntax`, checked on 2026-09-27), so the
  whole answer failed at the first count of a thousand or more, and the one
  error it did not surface, `Int64` on a number past the type, was dropped
  and read as 0. What retires it: a client-go release carrying the commit,
  after which `Get` calls `GetApplicationStatistics` and the type goes.

**What**: the struct uses `int64` fields, while GitLab returns every count as
a JSON string, so decoding fails.

**Which versions send strings**, the question this entry used to leave open
before a report, was answered while writing the commit: every version since
at least 13.0.
`lib/api/entities/application_statistics.rb` renders each count through
`number_with_delimiter`, which returns a string, so
`GET /application/statistics` answers `"issues": "1,234"`, and the separator
is the one the caller's `preferred_language` uses, which is not always a
comma.

**Which separator each language gives**, read on 2026-09-28 from GitLab's
`master` at `e52599d01de0` (2026-09-22) rather than from a booted instance:
ActiveSupport's number helper was run outside GitLab under each of the 27
languages `Gitlab::I18n::AVAILABLE_LANGUAGES` offers, set as
`API::Helpers#current_user` sets it, with the locale files GitLab loads
(rails-i18n 7.0.10, the version its `Gemfile.lock` pins, plus
`config/locales`) and its `config.i18n.fallbacks = [:en]`. Three forms come
out, and a count below a thousand has none:

| Separator             | `1234567` renders as | Languages                                                                                                                                                                                                       |
| --------------------- | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Comma                 | `1,234,567`          | `en`, `ja`, `ko`, and the sixteen GitLab spells with a region (`cs_CZ`, `da_DK`, `fil_PH`, `ga_IE`, `gl_ES`, `id_ID`, `nb_NO`, `nl_NL`, `pl_PL`, `pt_BR`, `ro_RO`, `si_LK`, `tr_TR`, `zh_CN`, `zh_HK`, `zh_TW`) |
| Period                | `1.234.567`          | `de`, `es`, `it`                                                                                                                                                                                                |
| Space (ASCII, U+0020) | `1 234 567`          | `bg`, `eo`, `fr`, `ru`, `uk`                                                                                                                                                                                    |

The regional sixteen group with a comma because rails-i18n spells its locales
with a hyphen (`pt-BR`, `zh-CN`), without the region (`pl`, `nl`) or not at
all (`fil`, `ga`, `si`), while GitLab sets `I18n.locale` to the underscore
form, which matches no locale file and falls back to English; a Polish or
Dutch caller therefore sees a comma where rails-i18n's own `pl` and `nl`
files would give a space and a period. rails-i18n 8.1.0, which GitLab's
`Gemfile` takes when it runs on the next Rails, gives each of the eleven
locales without a region the separator 7.0.10 gives it. A negative count is
possible for one field: `forks` is fork network members less fork networks,
both approximated, and Rails writes the sign in front (`-1,234`).

### The security attribute and category mutations discard GraphQL errors

- **Reported**: yes, first as commit 4 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, and since 2026-09-30 on its own in
  [gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066),
  at the review's request.
- **In review**: yes, open, in
  [gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066).
  Its first commit is the one reviewed in the joint merge request,
  cherry-picked unchanged onto v3.15.0: all eight return a
  `*GraphQLResponseError` carrying the top-level errors, as the work item and
  saved view methods already do, and a caller matching `ErrNotFound` on the
  three that returned it sees a different error. On 2026-09-29 the maintainer
  asked whether the eight now return that error and did not before, and the
  reply said yes, since five of them returned success for a refused mutation
  and three a bare `ErrNotFound`. The answer, at 22:06 UTC the same day, asked
  for the change to be reviewed on its own, with integration tests of the
  mutations working and of one of them refused. The second commit adds them
  in `gitlab_test/`: two lifecycle tests drive all eight mutations and need
  an Ultimate license, through a new `SkipIfNotUltimate`, and eight refusals
  assert that GitLab's error comes back as a `*GraphQLResponseError` carrying
  its message, with the HTTP 200 it arrives on. Seven of the refusals name an
  attribute, category, namespace or project that does not exist and need no
  license, since each mutation resolves what it names through
  `authorized_find!` before anything a license decides; the eighth applies a
  destroyed attribute in bulk. They were run on 2026-09-30 against GitLab EE
  19.4.1-ee in Docker: without a license the seven refusals passed and the
  lifecycle tests skipped, with an Ultimate license on the same instance all
  of them passed, and on v3.15.0 with the test commit alone every refusal
  failed, the three `ErrNotFound` ones with that error and the other five
  with none, the fifth being the attribute lifecycle's last step.
  The project's `tests:integration` job runs none of them, nor any other test
  in `gitlab_test/`: it exports the token as `GITLAB_TOKEN`, while
  `SetupIntegrationClient` has read `GITLAB_TOKEN_TEST` since the 3.0
  release, so the scheduled `main` pipeline of 2026-09-07 was the last to run
  the suite (132 tests) and every `main` pipeline since, scheduled or on a
  merge, reports 70 tests, 70 skipped with
  `GITLAB_TOKEN_TEST environment variable not set`
  ([job 16821904079](https://gitlab.com/gitlab-org/api/client-go/-/jobs/16821904079),
  2026-09-29).
  [gitlab-org/api/client-go!3067](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3067)
  proposes the one-line fix, passing the same token under both names.
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
a refused mutation reaches the caller of five of them as a success:
`DestroySecurityAttribute`, `DestroySecurityCategory` and
`BulkUpdateSecurityAttributes` return a nil error, `CreateSecurityAttributes`
returns an empty slice with no error, and `ProjectUpdateSecurityAttribute`
returns zero counts. The other three, `UpdateSecurityAttribute`,
`CreateSecurityCategory` and `UpdateSecurityCategory`, degrade to a bare
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

- **Reported**: yes, as commit 4 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds `Operation` to
  `EvaluatePackageOptions`, with the two values as constants, and
  `GetDependencyFirewallEnablement` for the enablement route, marked
  experimental like the evaluation. While the feature flag is off that route
  answers 404 with `{"enabled": false}`, and the SDK maps every 404 to
  `ErrNotFound` without reading the body, so the method cannot tell that from a
  project the caller may not read, which its doc comment says.
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

- **Reported**: yes, as commit 17 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit reads each value set off the model the
  route validates against rather than off the pages, and that corrects this
  entry on one value: `EventTargetTypeValue` gains `wiki` and `design` and
  **not** `epic`, because the events routes validate `target_type` against
  `Event.target_types` (`app/models/event.rb`), which has no epic, so a
  request filtering on it is refused whatever the page lists.
  `EventTypeValue` gains `transferred` beside `approved`, `TodoAction` the
  fourteen actions of the twenty it lacked, the seven Enterprise ones marked
  so, and `DeploymentStatusValue` `skipped` beside `blocked`. The cancellation
  role gets `CIRestrictPipelineCancellationRoleValue` with its three values,
  declared at first as an alias of `AccessControlValue` so no caller broke.
  After the review read the old typing as a bug, commit 35 (`a9dbd2b5`, added
  2026-09-29) makes it a type of its own, which stops compiling code that holds
  the role in an `AccessControlValue`; GitLab's Terraform provider is such code
  (see the review of the joint merge request below).
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The handlers forward the string a caller passes, so the
  schema enums offer the accepted values whether or not a constant exists,
  and each such value is recorded in `acceptedEnumGaps` in
  `cmd/audit_1to1/internal/enums/exemptions.go` so the enum rule can tell an
  accepted extra from an invented one. Each entry retires when the constant
  lands upstream: the rule then reports the exemption as stale. The three
  event listings (`user.event_list_project`, `user.event_list_contributions`
  and `user.contribution_events`) serve one pair of value sets,
  `events.FilterSchemaOverrides`, read off `Event.actions` and
  `Event.target_types` at GitLab 19.4.1 rather than off the page: `epic`,
  which they used to offer and GitLab refuses, is gone with its three
  exemptions, and `wiki`, `design` and `transferred` are offered, with nine
  exemptions citing the model
  ([issue 1020](https://github.com/jmrplens/gitlab-mcp-server/issues/1020)).
  GitLab validates only the second set: `event_filter_params` hands Grape
  `Event.actions`, the Rails enum hash, and Grape 2.4 reads a Hash given to
  `values:` as an options hash whose `:value` key it does not hold, so it
  checks no action, and `EventsFinder#by_action` answers an action it does not
  know with the whole feed. The three handlers therefore refuse an action
  outside the set before the request (`events.CheckActionFilter`), since a
  model that misspells one would otherwise read every event as filtered. A
  unit test in `internal/tools/events` pins both sets, and the e2e scenario
  `TestEvents_TargetTypeWiki_EveryListingAcceptsTheFilter` filters all three
  listings on `wiki` against a real instance.

**What**: four value types in `types.go` and `todos.go` declare fewer
constants than the GitLab API documents for the parameters they type, and one
parameter is typed with the wrong value type altogether.

- `EventTypeValue` lacks `approved`, which
  [the user contribution events](https://docs.gitlab.com/user/profile/contributions_calendar/#user-contribution-events)
  list among the action types the events API filters on, and `transferred`,
  which `Event.actions` holds beside it and no page lists.
- `EventTargetTypeValue` lacks `wiki` and `design`, which the events routes
  accept. The [events API](https://docs.gitlab.com/api/events/) page also lists
  `epic` as a `target_type` since GitLab 17.3, but the routes validate against
  `Event.target_types`, which has no epic, so a request filtering on it is
  refused; `epic` is not a value to add.
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

**Effort**: small. Eight constants across the four types (two on
`EventTypeValue`, two on `EventTargetTypeValue`, three on `TodoAction`, one on
`DeploymentStatusValue`), and a dedicated value type for the cancellation role;
none of them changes a signature.

### CreateProjectForkRelation declares a response GitLab does not send

- **Reported**: yes, as commit 18 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit does the half that breaks nothing:
  `CreateProjectForkRelationV2` sends the same request and returns
  `*Project`, and `CreateProjectForkRelation` and `ProjectForkRelation` are
  marked deprecated in its favour. The old name taking the `V2` signature is
  left for 4.0, which the merge request lists.
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

- **Reported**: yes, as commit 26 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds `InviteSource` and `MemberRoleID`
  to `InvitesOptions` and `QueuedUsers` to `InvitesResult`, all three.
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

- **Reported**: yes, as commit 5 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit selects `awardMessageHtml`, the
  namespace's name, paths, avatar and page, and the three users through the
  shared user fragment, and adds them to the structs beside the numeric ids,
  which stay. Measured anonymously against gitlab.com, the widest document
  goes from a complexity of 96 to 188, under the unauthenticated limit of 200.
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

- **Reported**: yes, as commit 9 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open, and in part. The commit gives `Epic` an
  `UnmarshalJSON` that keeps the label names in `Labels` and puts the objects
  in a new `LabelDetails`, as `Issue` and `MergeRequest` do, so `Labels` keeps
  its type; adds ten of the twelve fields, `Subscribed` for the one route that
  sends it, `public_email` and `locked` on the author, and the two list
  filters; marks `UserNotesCount` deprecated; and adds `TextColor` to
  `WorkItem`. `end_date` and `web_edit_url` are left out, since GitLab
  deprecates both in favour of `due_date` and `web_url`, which `Epic` already
  has, and `URL` is kept, because an issue embeds its epic through
  `EpicBaseEntity`, which sends it, and `Issue.Epic` decodes into this struct.
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

- **Reported**: yes, as commit 10 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds all eight missing fields,
  `suggestions` with a new `Suggestion` type and `commands_changes` as a map,
  and marks the four phantom `Note` fields and the author and resolver
  `Email` deprecated rather than removing them, which is left for 4.0.
  `Note` gaining a slice and a map stops it being comparable with `==`, one of
  the four decisions the merge request asked the maintainers for. On
  2026-09-29 the maintainer answered that the library does not count
  comparability as part of its compatibility, so the fields stay.
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

- **Reported**: yes, as commit 28 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit carries all three kinds: the member
  fields on both structs and `public_email` and `locked` on
  `MemberCreatedBy`; `SkipUsers` and `State` (a new `MemberStateValue`) on
  both list options and `InviteSource` on both add options;
  `DeleteProjectMemberWithOptions` beside `DeleteProjectMember`, until 4.0
  folds the options in; and methods for all seven Enterprise routes listed
  below.
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

- **Reported**: yes, as commit 15 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds every field below, the lint jobs
  as a new `ProjectLintJob` and the runner's creator as a `*BasicUser`, and
  one more this entry's list leaves out and its workaround already reads:
  `expires_at` on `PipelineTrigger`, exposed on every trigger token.
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

- **Reported**: yes, as commit 24 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds the three fields to
  `PersonalAccessToken`, with a new type for a granular scope, `ResourceType`
  and `ResourceID` to the resource token, and five fields to
  `ImpersonationToken`: not `granular_scopes`, because the impersonation token
  routes do not ask for them, so that key never arrives there.
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

The request side, the create options, is
[row 82](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them).

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

- **Reported**: yes, as commit 2 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit drops the four slashes, and its test
  records the path each method builds before anything can redirect it, since
  the existing tests passed only because `http.ServeMux` redirects a double
  slash the way gitlab.com does.
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
  Every one of those merge requests references it with a non-closing
  `Related to`, so the first merge does not close the umbrella; the joint one
  that follows them closes it.
- **In review**: yes, all fourteen, every one now merged. The last to merge was
  [gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
  (the seven `Hook` fields), whose history follows: a reviewer approved it
  on 2026-09-14 and again on 2026-09-16, after the test push that reset the
  first approval, both of its threads are answered and resolved, one of them
  by adding the `custom_webhook_template` assertion to the edit test, and the
  reviewer handed the maintainer review to @fforster on 2026-09-14, who has
  not answered since. A note of 2026-09-24 in the reviewer's documentation
  thread records that the documentation half,
  [gitlab-org/gitlab!254538](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254538),
  merged and is live, and asks @fforster for that review. A note of
  2026-09-26 asked @PatrickRice to take it over instead, and on 2026-09-27 at
  16:12 UTC @heidi.berry, a code owner, started the merged-results pipeline in
  the canonical project on `934b97ac`, which passed at 16:22; a code owner
  running that pipeline is usually the step before a merge. Its head was then
  `fb2e5110`, it had no conflicts, and the project merges with a merge commit,
  so being behind `main` needs no rebase. On 2026-09-28 @PatrickRice took the
  maintainer review over (16:05 UTC) and left one finding, "otherwise LGTM":
  `CustomHeaders` on the `Hook` response struct should be
  `[]HookCustomHeader` rather than a slice of pointers, since a nil slice
  already stands for no headers and a slice of nil elements means nothing. It
  came with a suggestion block on `system_hooks.go:108`, which was applied
  through GitLab the same evening as `552678c6`; `2d031b04` followed, updating
  the expected `Hook` in `TestSystemHooksService_GetHook` and realigning the
  struct's tags with `gofmt`, since the suggestion alone left both, and
  `go test -run SystemHook` and `go vet` pass on it. The thread was answered
  and resolved, as was the answered handover thread, so no discussion is open
  and the merge request waited on @PatrickRice's approval, which the new
  commits required. @PatrickRice approved it at 18:34 UTC on 2026-09-28 and
  put it on the merge train, which merged it at 18:55 (merge commit
  `5f0e219a`), and it was tagged ten minutes later in **v3.15.0**, so none of
  the fourteen is open any longer.
  The rest of this entry, the gaps held back below, went out on 2026-09-27 in
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  described further down, which does not touch `system_hooks.go` and so does
  not overlap this one.
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
  **v3.10.0** on 2026-09-14; then `gitlab-org/api/client-go!3051` (the eight
  `Namespace` fields) in **v3.11.0** on 2026-09-16; and
  `gitlab-org/api/client-go!3052` (`ConanPackageName`, `CreatorID` and
  `Versions` on `Package`) in **v3.14.0**, merged by @PatrickRice at 18:46 UTC
  on 2026-09-24 and tagged thirteen minutes later. v3.13.1, cut at 18:23 the
  same day, does not carry it. Last, `gitlab-org/api/client-go!3048` (the
  seven `Hook` fields) in **v3.15.0**, merged by @PatrickRice at 18:55 UTC on
  2026-09-28 and tagged at 19:05.
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
- **Workaround**: retired for thirteen of the fourteen: twelve at the
  **v3.12.0** pin and the thirteenth, `packages`, at **v3.14.0**; the
  fourteenth, `systemhooks`, retires when the pin reaches **v3.15.0**, as the
  end of this bullet says.
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

  `packages` followed when the pin moved to **v3.14.0**, checked against that
  release's `packages.go` in the module cache: both listings read `creator_id`
  and `conan_package_name` off `Package`, and `PackageExtra` now holds only the
  five pipeline keys the `PackagePipeline` gap below records. Those are read on
  every route that renders a package: on the package's own `pipeline` by both
  listings and by `package.get`, the action the bump's `GetProjectPackage` made
  possible, and on the pipeline of each other version by `package.get` alone.
  The rest of what that capture read needed no field of
  `gitlab-org/api/client-go!3052`, and is not lost: the owning project's
  `project_id` and `project_path` are sent only to a group's listing, where
  `GroupPackage` already decodes them, and `versions` only to a request for one
  package, which neither listing is, because the entity leaves them out of a
  collection. The entity's `pipelines` is read by nothing: it renders the
  constant `EMPTY_PIPELINES` whatever the package, deprecated in GitLab 16.1,
  so client-go's `Pipelines` decodes an empty list or none and this server
  publishes no such key.

  `systemhooks` keeps its workaround whole: it still reads the seven `Hook`
  fields of `gitlab-org/api/client-go!3048` off the capture, checked against
  the v3.14.0 source rather than against the tracker, and the struct it pins
  does not carry them. The merge request is merged and released in v3.15.0
  (2026-09-28), so the capture retires when the pin moves there, which the
  Dependabot cooldown for a Go minor holds until 2026-10-12.
  `projectserviceaccounts` keeps its read of `public_email` too, since
  `gitlab-org/api/client-go!3047` added the pair to `GroupServiceAccount` and
  `ProjectServiceAccount` was outside it.

**What**: one field per struct, each exposed by the rendering entity with no
condition at all, so every response of every endpoint that renders it carries
the field and the SDK drops it. Every entity reference is to the tag
`v19.3.1-ee`, the release the live record was taken from when this entry was
written; the record has been re-pinned to 19.4.1-ee since.

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
  missing `public_email` still, and both gain it in commit 27 of
  `gitlab-org/api/client-go!3063`. The second half of that lead is closed:
  `GroupsService.GetServiceAccount` and
  `ProjectsService.GetProjectServiceAccount` wrap
  `GET /…/service_accounts/:user_id` as of **v3.12.0**
  ([!3056](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3056),
  somebody else's), which this server publishes as `group.service_account_get`
  and `project.service_account_get`. GitLab mounts that route from 19.4
  ([gitlab-org/gitlab!250360](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/250360),
  first tagged in `v19.4.0-ee`), so both actions say so, and on an older
  instance every ID answers 404.

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
rather than opening it themselves, so cross-checking all eight fields against
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
tags that contain each merge commit, read on 2026-09-23, on 2026-09-24 and
again on 2026-09-27, three of them have shipped: `gitlab-org/gitlab!254507`,
`gitlab-org/gitlab!254511` and `gitlab-org/gitlab!254519` are in `v19.4.0-ee`
and `v19.4.1-ee`, and the other five are in no tag yet, so 19.5 is the first
release that can carry them; once it is cut, this paragraph and the umbrella's
documentation paragraph can say they shipped. Until the first of
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
As of 2026-09-24 it needs no further approval and waits on the writer. Read
again on 2026-09-27 it is where that left it: head `b2b900f7`, its pipeline
green, every thread resolved, and the writer still to approve and merge it.
Nothing is owed on it from here. Read on 2026-09-29, unchanged: untouched
since 2026-09-24, three working days now, with no approval required and none
given.

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
to being made on demand like anybody else's. @PatrickRice proposed that single
merge request there on 2026-09-09 and @timofurrer agreed. The umbrella's
description was brought up to date on 2026-09-24: its title now gives the
distinct count, 897 fields, its table of merge requests reads as the tracker
does, it counts the fields that have landed since the measurement, and it says
the rest will follow as that one merge request without saying when.

**That merge request is
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)**,
opened on 2026-09-27 at 20:34 UTC from the community fork (branch
`jmrp-recorded-defects`, head `a1a4b996`, 141 files): 36 commits on v3.14.0,
one per recorded entry, each with its own tests and, in its message, the
GitLab source it rests on, so it can be reviewed and bisected one commit at a
time. It carries entries 6, 7, 20, 22, 25 to 33, 35 to 37, 40 to 45, 52, 54
and 58 to 65, this entry's own held-back gaps below together with
`public_email` on both service account structs, and the Orbit schema's
`format` parameter, whose entry,
[entry 70](#the-orbit-schema-format-is-sent-as-format-and-its-llm-answer-is-not-modelled),
arrived with
[issue 972](https://github.com/jmrplens/gitlab-mcp-server/issues/972).
Entries 19 and 51 were in it too, as commits 4 and 2, until 2026-09-30, after
the review asked for each to be reviewed in a merge request of its own
(below); it has had 35 commits since. Three things are left out on purpose:
the seven `Hook` fields, which are
`gitlab-org/api/client-go!3048`; the `action` object and the illustration
fields of `DetailedStatus` that
[entry 43](#pipelineinfo-decodes-two-entities-and-models-only-the-smaller-one)
records one level inside the pipeline, which need a type of their own; and
[entry 5](#getnamespace-cannot-decode-a-path-based-lookup), which could not be
reproduced. No exported name is removed or renamed and no field type or method
signature changes (commit 35, added after the first review as commit 37,
changes the cancellation role fields' type; see below): where the right fix
would, the commit adds a `V2` or `WithOptions` sibling, an `UnmarshalJSON`
or a deprecation note instead, and
the description lists the breaking halves for 4.0. It asked the maintainers for
four decisions, three of them answered on 2026-09-29 (below): whether a nil
options pointer may stop sending a body for every caller (entry 51), that the
eight security mutations return `*GraphQLResponseError` (entry 19), that
`Deployment`, `Note` and `PlanLimit` stop being comparable with `==`, and
whether the work item documents' default selection should become safe for
Community Edition (entry 45). Its last line is `Closes #2300`, so the umbrella
closes when it merges.

**Where it stands on 2026-09-27.** Its fork pipeline on `a1a4b996` passed and
it has no conflicts. It has not been readied: it sits at `workflow::in dev`,
with nobody but GitLab Duo on the reviewer field, which left its usual
`DCR4003` note, and none of its one required approval. `prepared_at` is set
and `changes_count` reads 141, so the bot commands are safe to post, and two
are owed, in the order the upstream contribution skill gives. First the label:
the triage bot copied `type::maintenance` onto it from the umbrella through
the `Closes` line, a label that cuts no release, while the title is `feat:`,
so it wants `type::feature`, asked for by comment. Then a ready naming a code
owner: @PatrickRice, who proposed the single merge request, or @heidi.berry,
the code owner who acted on `gitlab-org/api/client-go!3048` the same day.
Both were posted that evening: `@gitlab-bot label ~"type::feature"` at 21:48
UTC and, a minute later, `@gitlab-bot ready @PatrickRice` with a note that
this is the single merge request proposed on the umbrella and that the four
decisions it needs are at the top of the description. The bot requested that
review, and at 00:12 UTC on 2026-09-28 it added its own labels
(`workflow::ready for review`, `backend`, `linked-issue`) beside
`type::feature`. As of 2026-09-28 it waits on that review, still on head
`a1a4b996` with 36 commits, and nothing is owed on it from here.

**The first review, 2026-09-29.** At 00:13 UTC @PatrickRice answered the list
of breaking halves held for 4.0, addressed to @heidi.berry and copying us. The
review agrees with each deferral: the six `V2` siblings, the two `WithOptions`
siblings (to be deprecated in 4.0 and removed in 5.0), the two
narrow-typed fields kept beside their object fields, and the dead fields kept
deprecated until 4.0. It reads the structs that decode several entities as
routes that should move to their `Basic` struct in 4.0, and would leave the two
deprecated methods alone, since a deprecated GitLab route lives a long time and
older instances still serve it. It asks for an issue tracking the 4.0 halves
against the v4 release, which is how the project keeps such decisions. One
point goes further than the merge request: it calls
`CIRestrictPipelineCancellationRole` a bug rather than a breaking change,
because none of `AccessControlValue`'s four values is one the setting accepts,
which argues for making the new type distinct now instead of an alias of the
old. The four decisions the description asks for are not answered yet, there
is no approval, and @heidi.berry has not replied.

**Answered, 2026-09-29.** Both requests were taken up the same day:

- [gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301)
  tracks the 4.0 halves, with the review's additions: the `WithOptions`
  siblings removed in 5.0 and the narrower structs in 4.0.
- Commit 37 (`a4b189e7`) makes `CIRestrictPipelineCancellationRoleValue` a
  type of its own. Preparing it showed the old typing was not unused: GitLab's
  Terraform provider sets the role through
  [`AccessControlLevelValueToName`](https://gitlab.com/gitlab-org/terraform-provider-gitlab/-/blob/da9cb4f79ab5976ed4b2aa03e416454ca741124e/internal/provider/api/access_level_helpers.go#L118-132),
  which returns an `AccessControlValue` and exists, by its own comment, because
  of this typing, and four lines in three of its test files hold the role the
  same way. That code stops compiling when the provider moves to the release
  carrying the commit. The reply says so and offers to drop the commit.
- Commit 32's `User.Skype` note and message dated the removal of `skype` to
  18.4; GitLab stopped sending it in 18.2
  ([gitlab-org/gitlab@70437e6e](https://gitlab.com/gitlab-org/gitlab/-/commit/70437e6efbdc7bd99201f13ac7dc6fa6dee3472f)).
  The branch was force-pushed from `a1a4b996` to `a4b189e7`, now 37 commits;
  commits 33 to 36 changed only their hashes.
- The [reply](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3922840298)
  asks whether the `WithOptions` siblings, deprecated at birth like the three
  already in the library, should instead stay undeprecated until 4.0 or say
  5.0. The description gained the "Is this a breaking change?" and "How was
  this tested?" headings the project's merge requests use.

**The maintainer's answers, 2026-09-29.** At 15:21 UTC @PatrickRice answered
three of the four decisions, and asked about the fourth, in the thread that
reply opened
([note 3924608467](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3924608467)):

- **The nil options pointer (commit 2)** cannot be changed for every caller,
  since nobody can know who calls the library. It has to be a feature toggle:
  opt-in now, the default in 4.0 and removed in 5.0, so that users have time
  to report what it breaks and update their code.
- **The security mutations (commit 4)** are not decided: the note asks
  whether the eight "now" return the response error and did not before, since
  the question's tense left that unclear.
- **Comparability (commits 12, 16 and 31)** is not something the library
  counts as part of its compatibility, since a response struct can gain a
  field that is not comparable at any time. Nobody has reported it as a
  problem, and if somebody did, the structs would gain a comparison function.
  The fields stay.
- **The work item default selection (commit 10)** stays Enterprise. Either
  default confuses somebody: an Enterprise caller given a Community default
  gets a partly filled object, while a Community caller given the Enterprise
  one at least gets "a sane error". It was decided with the upstream product
  team that added the API for the glab CLI.

The note leaves the `WithOptions` schedule and commit 37 unanswered. What
followed the same afternoon:

- Commit 2 was rewritten as `feat(client): add option to treat nil options
  pointers as no options` (`1db14637`). `WithNilOptionsOmitted()` is a client
  option, off by default and modelled on `WithOnlyIdempotentRetries`. With it,
  a nil options pointer means no options. Without it, a POST, PUT or PATCH
  given one still sends the JSON literal `null`, and a request with any other
  method still has the query already on the URL given to `NewRequestToURL`
  replaced with an empty one. The tests pin both sides,
  `TestPublishAllDraftNotes` is back to its original pin of the `null` body,
  and a further test holds that real options are still sent with the option
  on.
- The branch was force-pushed from `a4b189e7` to `7899ad70` at 16:43 UTC:
  still 37 commits, every one PGP-verified on GitLab, and commits 3 to 37
  unchanged apart from their hashes (`git range-diff` marks each `=`).
  Commits 2 to 37 were each built, vetted and tested again, and the head
  also passed `-race`, ten shuffled runs and golangci-lint v2.13.2. Its fork
  pipeline, 2894203711, passed at 16:50 UTC, and the merge request has no
  conflicts.
- [gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301)
  gained a sentence in its summary saying the nil options change is opt-in in
  `gitlab-org/api/client-go!3063` and its default follows in 4.0, "changes
  what a request sends by default" among the reasons in "Why deferred", and a
  section 6: 4.0 makes the option's behavior the default, where the option
  has no effect, and 5.0 removes the option. The merge request's description
  records each answer, commit 2's new title, and a 4.0 bullet saying a nil
  options pointer still sends `null` by default.
- The [reply](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3925107778),
  posted in the same thread at 16:44 UTC, explains the toggle and answers the
  tense: yes, now, since before this merge request five of the eight returned
  success for a refused mutation and three returned a bare `ErrNotFound`. It
  thanks the maintainer for the answers on comparability and the work item
  default, and asks again about the two open items: whether the five
  `WithOptions` siblings keep their `Deprecated` notes now, reworded to say
  5.0, or lose them until 4.0 deprecates them, and whether commit 37 stays or
  is dropped and its item put back on the 4.0 list.

It now waits on the maintainers for the answer on commit 4, the `WithOptions`
schedule and whether commit 37 stays, and on its one required approval.

**Split in three, 2026-09-30.** At 22:06 UTC on 2026-09-29 @PatrickRice asked,
in the same thread
([note 3926712100](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3926712100)),
for the first two decisions to be reviewed in merge requests of their own,
since they change the library rather than sync fields, which is what the
single merge request had been agreed for, and for integration tests of the
security mutations succeeding and of one of them refused. Six minutes earlier,
in the thread of the first review
([note 3926690837](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3926690837)),
he had set out for @heidi.berry, who had first proposed deprecating the
`WithOptions` siblings now and removing them in 4.0, the three phases of that
change: both methods now, the base method taking the options in 4.0 with the
sibling kept, and the sibling removed in 5.0. On 2026-09-30:

- [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065)
  carries the nil options toggle of
  [entry 51](#a-withoptions-delegation-sends-null-as-the-request-body), the
  commit reviewed as commit 2, cherry-picked unchanged onto v3.15.0.
- [gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066)
  carries the security mutation errors of
  [entry 19](#the-security-attribute-and-category-mutations-discard-graphql-errors),
  the commit reviewed as commit 4, cherry-picked unchanged onto v3.15.0, and a
  second commit of integration tests in `gitlab_test/`. Entry 19 has the tests,
  the run against a live instance, and the finding that the project's
  `tests:integration` job runs no test in that directory.
- The joint merge request's branch was rewritten without the two: 35 commits
  on v3.14.0, the rest unchanged apart from their hashes and numbers (commit
  3 is now 2, and each of 5 to 37 is two lower). Every one of the 35 was
  built, vetted and tested again on top of the ones before it, and the branch
  was force-pushed from `7899ad70` to `a9dbd2b5`. Its description points to
  the two new merge requests and renumbers the rest, and the reply in the
  thread says so and asks again about the two items still open.
- [gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301)
  names
  [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065)
  where it named the joint merge request as the one adding
  `WithNilOptionsOmitted()`, and the umbrella's description counts the joint
  merge request's 35 commits and gains a row for each new one.

The joint merge request now waits on the maintainers for the `WithOptions`
schedule, whether commit 35 (37 before the split) stays, and its one required
approval; each new one waits on its review.

**The umbrella's description is behind again**, read on 2026-09-27, in four
places: its table still reads `gitlab-org/api/client-go!3052` as in review,
though it merged on 2026-09-24 and is in v3.14.0; it has no row for
`gitlab-org/api/client-go!3063`; its count paragraph still says 22 fields
released and ten more in `gitlab-org/api/client-go!3048` and
`gitlab-org/api/client-go!3052`, where 25 are released and the seven left are
in the one still open; and its documentation paragraph says
`gitlab-org/gitlab!254540` is waiting for a reviewer, which it has had since
2026-09-24. Its update paragraph promises the single merge request without
naming it. No comment there waits on us, since the maintainers' last notes, of
2026-09-10, were answered. The skill's rule is to rewrite the description
rather than comment, the moment a merge request is published, and the rewrite
was done at 21:50 UTC the same day: `gitlab-org/api/client-go!3052` reads
merged and released in v3.14.0, `gitlab-org/api/client-go!3063` has its row as
the single merge request agreed in the thread, and the count paragraph says 25
of the 290 fields are released and seven more are in
`gitlab-org/api/client-go!3048`. Nothing is owed on the umbrella now.

**Behind again on 2026-09-29.** The description, then last edited at 21:50
UTC on 2026-09-27, still read
[gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
as open and waiting on a maintainer, still counted 25 fields released with
seven more in that merge request where 32 were released, still gave
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
36 commits, and did not name
[gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301),
the 4.0 issue the first review asked for. It was rewritten at 11:21 UTC on
2026-09-29, in five places and nothing else:
[gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
reads merged and released in v3.15.0; the count paragraph says 32 of the 290
fields are released, the seven on `Hook` among them; the update paragraph says
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
opened with 36 commits, carries a 37th after the first review, and leaves its
4.0 halves to
[gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301);
the service account paragraph, which still said the `public_email` additions
for `ServiceAccount` and `ProjectServiceAccount` had not been sent, says
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
adds them; and
[gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699)
reads merged and due in 19.5. Nothing is owed on the umbrella now.

**Three findings from that batch that are not merge requests**, because sending
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

**Six more gaps were recorded and held back** by the batching the maintainer
asked for above, and went out on 2026-09-27 as commit 29 (27 since the
split) of `gitlab-org/api/client-go!3063`, which also deprecates
`LicenseTemplate.Featured` rather than removing it, and leaves
`BasicUser.CreatedAt`, the key `UserBasic` never sends, as it is. Each is a
field this server now reads from the captured response, so each carries a
live workaround until a release carries the commit:

- `BasicUser` is missing `public_email` and `locked`, both exposed with no
  condition by `lib/api/entities/user_basic.rb`, which is what every route
  decoding into `BasicUser` renders (`locked` through `access_locked?`). The
  participants of an issue and of a merge request, a merge request's reviewers
  (nested under `user`) and approvers (nested under `approved_by[].user`), and
  the author of the to-do an issue or a merge request answers `create_todo`
  with all decode into it. This server publishes both keys on every one of
  them, reading them off the captured answer (`toolutil.CapturedUserBasics`,
  `toolutil.CapturedNestedUserBasics`, and `toolutil.CapturedTodo` for the
  to-do's author, whom `toolutil.UserBasicFrom` completes); the package
  pipeline's user below is the same gap met earlier, completed by the same
  helper.
- `ServiceAccount` has no `public_email`, which the `UserSafe` that
  `lib/api/entities/service_account.rb` inherits exposes with no condition.
  The instance service account list and update in `internal/tools/users` read
  it off the captured answer.
- `PackagePipeline` is missing `iid`, `project_id` and `source` of the eleven
  keys `API::Entities::Package::Pipeline` exposes. The pipeline's user decodes
  into `BasicUser`, which is missing the `public_email` and `locked` of the
  `UserBasic` GitLab renders there, and carries a `created_at` that entity
  never sends. Every route that renders a package renders that pipeline: as
  the package's own `pipeline` on both listings and on a request for one
  package, and as the pipeline of each other version on the last. The same
  entity renders `pipelines` too, but as a constant empty list since GitLab
  16.1, so no pipeline ever arrives there to be short of anything. This server
  publishes all five on every one of them, reading them off
  the captured response beside the SDK's decode (`toolutil.CapturedPackage`
  and `toolutil.CapturedPackages`, under
  [ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)),
  and publishes no `created_at` for the user; a bump carrying the five retires
  the read. A package file renders the same pipeline entity under `pipelines`
  (`lib/api/entities/package_file.rb`, real pipelines this time), and
  client-go's `PackageFile` decodes them into its full `Pipeline`, which
  carries the pipeline's own keys and the same short `BasicUser`;
  `package.file_list` reads the user's two keys off the captured answer, and
  the `BasicUser` fix above retires that read too.
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

**Six more are recorded and not yet sent**: the merge request above does not
carry them, and each is read from the captured response in the meantime.

- `Todo` has no `updated_at`, which `lib/api/entities/todo.rb` exposes with no
  condition, and no `group`, which it exposes on a to-do raised in a group
  (`if: ->(todo, _) { todo.group_id }`). `internal/tools/todos` reads both off
  the to-do list's captured answer (`toolutil.CapturedTodos`), and the issue
  and merge request `create_todo` handlers read `updated_at`; a to-do those two
  routes create belongs to a project and never carries `group`.
- `SubmoduleCommit`, what `UpdateSubmodule` returns, models thirteen of the
  eighteen keys `PUT /projects/:id/repository/submodules/:submodule` sends with
  no condition (`lib/api/submodules.rb` presents the new commit through
  `lib/api/entities/commit_detail.rb`). The other five are `web_url`,
  `trailers` and `extended_trailers`, from the `Commit` entity it extends, and
  `project_id` and `last_pipeline`, its own. `internal/tools/repositorysubmodules`
  reads the five off the captured answer, `extended_trailers` as the map of
  lists GitLab sends rather than as client-go's `Commit` spells it, which is
  [a defect of its own](#commit-declares-extended_trailers-a-map-of-strings-and-gitlab-sends-lists).
- `TreeNode` has no `last_commit` and `ListTreeOptions` no `with_last_commit`,
  the parameter GitLab 19.3 added to `GET /projects/:id/repository/tree` and
  the key `lib/api/entities/tree_object.rb` then exposes on each entry, a
  whole commit. `repository.tree` offers the parameter through a request
  option that adds it to the query client-go encoded, and reads the commit off
  the captured answer, into a type of its own rather than client-go's `Commit`
  for the reason the entry on `extended_trailers` gives.
- `JobTokenAccessSettings`, what `GetProjectJobTokenAccessSettings` returns,
  has no `outbound_enabled`, which `lib/api/entities/project_job_token_scope.rb`
  exposes with no condition beside `inbound_enabled`: the older outbound
  scope, which GitLab deprecated and planned to remove in 18.0 and still
  sends. `job.token_scope_get` reads it off the captured answer.
- `PipelineVariable`, what both `GetPipelineVariables` and the three pipeline
  schedule variable writes decode into, has no `raw`, which
  `lib/api/entities/ci/variable.rb` sends for a `Ci::PipelineVariable` and a
  `Ci::PipelineScheduleVariable` because both tables carry the column. The
  entity's other conditional keys (`hidden`, `protected`, `masked`,
  `environment_scope`, `description`) wait on `respond_to?`, which neither
  model does, so `raw` is the only one the struct is short of on these
  routes. `pipeline.variables` and the schedule variable create and edit read
  it off the captured answer.
- `ImportStatus`, what `ImportFromFile` and `ImportStatus` return, tags its
  timestamp `create_at` where `lib/api/entities/project_identity.rb` sends
  `created_at`, so the field never decodes, and has no `failed_relations`
  (at most a hundred `ProjectImportFailedRelation`s) and no `stats` (a GitHub
  import's fetched and imported counts, null for every other import), both
  exposed with no condition by `lib/api/entities/project_import_status.rb`.
  `internal/tools/projectimportexport` already reads both status routes
  through a raw decode for the timestamp, and now reads the other two keys
  there as well. The failed relation's `exception_message` is rendered by a
  block that returns nil for every relation, so a struct for it should leave
  that key out.

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

- **Reported**: yes, as commit 30 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit removes nothing, since removing a field
  is the breaking half and is listed for 4.0: it gives every phantom of the
  three batches below a `Deprecated` note saying that GitLab does not send it
  and what to read instead, and moves it to the deprecated block of its
  struct. `BasicMergeRequest.LabelDetails` is the one it does not deprecate:
  the methods that list merge requests fill it, since they decode through
  `MergeRequest.UnmarshalJSON`, and it is empty on every other method, which
  the type's comment now says. The `PendingInvite.id` half and the
  `invite_token` gap it travels with are both in that merge request, in this
  commit and in commit 27.
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

**A third batch** came from the type grain of R-PATH (`shapes.typed` in
`go run ./cmd/audit_1to1/ -scope=paths`), which holds an output type against
what the live record says the routes it models send. Each field below is one
client-go models on a struct this server's output types pair with, and one no
entity renders on the routes those types answer. Read against the 19.5.0-pre
tree:

| SDK struct  | Field                                   | Why no response carries it                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ----------- | --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `User`      | `skype`                                 | GitLab dropped it: `app/models/user.rb` ignores the column since 18.4, and nothing under `lib/api` or `ee/lib/api` names it, as a key or as a parameter                                                                                                                                                                                                                                                                                                                                               |
| `User`      | `extern_uid`, `provider`                | `lib/api/entities/identity.rb` exposes both inside the `identities` array; no user entity renders them at the top level                                                                                                                                                                                                                                                                                                                                                                               |
| `User`      | `can_create_organization`               | only `API::Entities::ApplicationSetting` exposes the name. It was published without `omitempty` on three outputs, so every user they answered with said `false`                                                                                                                                                                                                                                                                                                                                       |
| `User`      | `current_sign_in_ip`, `last_sign_in_ip` | exposed by `UserDetailsWithAdmin` alone. The enterprise, provisioned and SAML user routes present `UserPublic` (`ee/lib/api/group_enterprise_users.rb`, `ee/lib/ee/api/groups.rb`), so the two left those three outputs and stay on the one for the instance user routes                                                                                                                                                                                                                              |
| `Group`     | `duo_availability`                      | a create and update parameter (`ee/lib/ee/api/helpers/groups_helpers.rb`); the one entity carrying the name is `ApplicationSetting`                                                                                                                                                                                                                                                                                                                                                                   |
| `Project`   | `public_builds`                         | `lib/api/entities/project.rb` renders the column as `public_jobs` alone, since the 9.0 rename client-go's own comment cites. This server published its own copy of `public_jobs` under the old key, and dropped it rather than keep a key no GitLab answer carries                                                                                                                                                                                                                                    |
| `Issue`     | `external_id`                           | filled by `Issue.UnmarshalJSON` from a string `id`, which only `API::Entities::ExternalIssue` sends, on the closes-issues and related-issues routes of a merge request whose project uses an external tracker. Removed from every issue output. Those two routes now list the tracker's issues apart, as the entity renders them (a `title` and a string `id`), since on the basic issue's row such an issue also carried a zero `id` and `iid`, empty dates and `false` flags the tracker never sent |
| `Issue`     | `label_details`                         | filled by the same unmarshaller only when `labels` arrives as objects, which `IssueBasic` renders under `with_labels_details` alone. Search and the two merge request listings take no such parameter, so the key left the basic issue they answer with and stays on the issues output, whose routes do take it                                                                                                                                                                                       |
| `IssueLink` | `source_issue`, `target_issue`          | typed `*Issue`, while `lib/api/entities/issue_link.rb` renders both `using: IssueBasic`: twelve of the issue's keys never arrive at either position (`external_id`, `health_status`, `moved_to_id`, `label_details`, `references`, `subscribed`, `_links`, `issue_link_id`, `epic_issue_id`, `epic`, `iteration`, `service_desk_reply_to`), and the three IssueBasic keys `Issue` does not model (`type`, `start_date`, `blocking_issues_count`) are read from the captured response instead          |

They add 40 `missing_output` rows to the R-OUTPUT diff, answered here as the
first batch's are.

`skype` survives on the request side of client-go too: `CreateUserOptions` and
`ModifyUserOptions` still send it, and Grape drops it, since `lib/api/users.rb`
declares no such parameter on `POST /users` or `PUT /users/:id`. The same
change took it off this server's two user inputs, where a caller who set it
changed nothing, and the R-INPUT diff's report of the SDK field those inputs
no longer offer is answered in `acceptedMissingInputs`.

### The Geo structs model a fraction of a site and its status, and the repair method names the wrong entity

- **Reported**: in part, in the Geo block of the umbrella issue
  [gitlab-org/api/client-go#2300](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300),
  which names the site's four missing settings and explains the status matrix.
  `repositories_count`, `storage_shards`, the `namespaces` type and the
  `RepairGeoSite` return type are not in it. All four gaps are in commit 31 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, after the batching that entry records had held them back.
- **In review**: yes, open. The commit adds the site's four settings to
  `GeoSite` and to both option structs, with `sync_object_storage` on the edit
  options; gathers the per-replicable keys into a `Replicables` map, as this
  server does, rather than some 400 more fields, and adds
  `repositories_count` and `storage_shards`; decodes the namespace objects
  into a new `NamespaceDetails` beside `Namespaces`, which keeps its type and
  now holds the full paths; and adds `RepairGeoSiteV2`, returning
  `*GeoSiteStatus`, with `RepairGeoSite` deprecated in its favour. Changing
  `Namespaces`' type and the old repair method's signature is the half left
  for 4.0.
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
`v19.3.1-ee` entities the live record was taken from when this entry was
written; the record has been re-pinned to 19.4.1-ee since.

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
  without naming them as return types. The whole entry is in
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes: the keys as commit 12, and the two return types in its
  description.
- **In review**: yes, open, and it closes less than this entry lists, for a
  reason the entry did not weigh. The commit adds `title_html` and
  `description_html` to `BasicMergeRequest` and `blocked_merge_request` to
  `MergeRequestDependency`, as a `*BasicMergeRequest`. It does not add
  `merge_status`, `reference`, `work_in_progress` or `approvals_before_merge`:
  GitLab deprecates each in favour of `detailed_merge_status`, `references`,
  `draft` and the approvals API, which the struct already models, and
  client-go took them out of the merge request structs on purpose, so adding
  them back would add fields deprecated from the day they land. The two return
  types are left as they are, because GitLab deprecated both endpoints and
  client-go both methods; the description says so.
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
live record was taken from when this entry was written; the record has been
re-pinned to 19.4.1-ee since.

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
  says, no route sends. The fifteen are commit 25 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, after the batching that entry records had held them back.
- **In review**: yes, open. The commit adds fourteen fields to `User`, the
  conditional ones as plain values like the conditional fields `User` already
  has, and the fifteenth, `unconfirmed_email`, through
  `CreateServiceAccountUserV2`, which returns the `*ServiceAccount` the route
  answers with; `CreateServiceAccountUser` is deprecated in its favour.
  Splitting `User` into one type per entity is left for 4.0.
- **Merged**: no.
- **Blocking**: no. Every gap is worked around.
- **Workaround**: yes. The fifteen keys are read from the captured response
  beside the SDK's decode (ADR-0021), through `toolutil.CapturedUser` for the
  ten every route sends and `toolutil.CapturedInstanceUser` for those plus the
  five only the instance-wide routes reach.

**What**: `User` in client-go v3.0.0's `users.go` carries 48 keys and is the
one struct every user-returning method decodes into, measured against the
`v19.3.1-ee` entities the live record was taken from when this entry was
written (it has been re-pinned to 19.4.1-ee since). GitLab serves
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
  one. The twenty-four are commit 11 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, after the batching that entry records had held them back.
- **In review**: yes, open, and in part. The commit adds twenty-three of the
  keys, reusing the types `Issue` already has for them, and not `epic_iid`,
  which GitLab deprecates in favour of `epic`. It embeds the epic as `Epic`,
  as `Issue.Epic` already does, and adds the two human-readable date strings
  to that struct, rather than the narrow type this entry recommends below;
  `Epic` already carries `url` because an issue's epic is sent that way. It
  adds `closed_as_duplicate_of` to `IssueLinks`, so `Issue` decodes it too, and
  leaves `subscribed` out, as this entry does.
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
entities the live record was taken from when this entry was written (it has
been re-pinned to 19.4.1-ee since), twenty-four keys are
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
  which lists all twenty-five permissions, and as commit 6 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, after the batching that entry records had held it back.
- **In review**: yes, open. The commit adds the twenty-five fields to
  `MemberRole` and leaves `CreateMemberRoleOptions` for a change of its own,
  which is the caution below.
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
  which lists all twelve keys, and as commit 16 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, after the batching that entry records had held it back.
- **In review**: yes, open, and in part. The commit adds
  `CreateMergeRequestPipelineV2`, which returns the `*Pipeline` the route
  answers with and takes the route's `async` option (a 202 answers with no
  pipeline), and deprecates `CreateMergeRequestPipeline` in its favour;
  `PipelineInfo` is unchanged, since everything else it decodes is rendered
  with the basic entity. The nested `DetailedStatus` gap below stays out,
  because it needs a type of its own, and the merge request says so, so no
  upstream change carries it yet.
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
  **That was wrong**, as writing the commit above found: `GET
  /projects/:id/pipelines` presents `Ci::PipelineBasicWithMetadata`, and the
  single pipeline, latest pipeline and metadata routes present
  `Ci::PipelineWithMetadata`, both of which add `name`, so the field decodes
  there and is empty only on the routes that present the two entities named
  above. Neither struct carries a phantom, the merge request leaves both as
  they are, and this server's pipeline outputs already publish `name` only
  when it is sent.

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
  records, so the eighteen left are this entry's. The keys are commit 20 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, after the batching that entry records had held them back, and
  the point of the entry is in its description, among the breaking halves
  left for 4.0.
- **In review**: yes, open, and in part. The commit adds the keys of all six
  structs as plain values, each conditional one with a comment saying when
  GitLab sends it, and the two objects under `ai_settings` and the Duo
  namespace access rules as new types. It leaves out `epic_iid`, which GitLab
  deprecates in favour of the `epic` `Issue` already models, and it does not
  split the three structs into one type per entity, since that would change
  what the methods return.
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

- **Reported**: yes, as commit 8 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit builds the three documents from the
  listing's field registry and gives them the listing's `ReturnedFields`:
  on `CreateWorkItemOptions` and `UpdateWorkItemOptions`, and through a new
  `GetWorkItemWithOptions`, since `GetWorkItem` takes no options. Left unset,
  it still selects every field, as before, so a caller on Community Edition
  has to pass `WorkItemDefaultListFields()`. Making the default safe there
  was one of the four decisions the merge request asked the maintainers for,
  and on 2026-09-29 the maintainer kept it Enterprise, as decided with the
  upstream product team that added the API for the glab CLI: a Community
  caller given that default at least gets a clear refusal, where an
  Enterprise caller given a Community one would get a partly filled object.
  Once a release carries it, the three actions here can pass the CE-safe set
  on a Free instance, which is then the only way they answer there.
- **Merged**: no.
- **Blocking**: yes, on Community Edition. `issue.work_item_get`,
  `issue.work_item_create` and `issue.work_item_update` cannot answer there:
  GitLab refuses the whole document, so the three actions fail on every call
  a Free instance is given, while the listing and the type listing work.
- **Workaround**: none possible without replacing the SDK's documents. The
  selection set is built inside `GetWorkItem`, `CreateWorkItem` and
  `UpdateWorkItem` from one template, and nothing a caller passes changes it.
  The e2e suite pins the state instead of skipping it: on a Community image
  `test/e2e/gitlab/common`'s work item lifecycle holds the create, the read
  and the retitle to GitLab's refusal of the five fields, naming each, and
  the listing and the delete to working, and its type listing runs on both.
  Taking a client-go release that carries the fix does not change that by
  itself, since the commit leaves the default selecting every field: the
  refused calls answer, and that scenario fails saying to run the whole
  lifecycle on Community Edition again and to record this entry as merged,
  the day the three actions here also pass `WorkItemDefaultListFields()` on
  a Free instance.

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

- **Reported**: yes, as commit 33 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit gives both fields `omitempty` on both
  halves of the tag and pins a body carrying the one field the caller set;
  `CreatePackageProtectionRulesOptions` keeps its tags, since GitLab requires
  both there.
- **Merged**: no.
- **Blocking**: partly. An update that names the pattern and the type works;
  an update that leaves out either of them is refused by GitLab (measured for
  one naming only the pattern, read from the source below for the rest, an
  update that changes only an access level among them), and no spelling of
  that call this server sends today avoids it.
- **Workaround**: partial. The struct tag decides what `encoding/json` writes,
  and a handler gets past it in one of two ways: a request option that
  rewrites the body after client-go has marshalled it (entry 54 says how), or a
  request the handler builds itself, as `internal/tools/features.Set` does for
  [`SetFeatureFlagOptions`](#setfeatureflagoptions-fields-lack-omitempty).
  Neither is carried here. `internal/tools/protectedpackages.Update` sets each
  pointer only when the caller named a value, which is the safe side of that
  choice, and answers the refusal with a hint telling the caller to name
  `package_name_pattern` and `package_type` on every update. The protection
  rule lifecycle in `test/e2e/gitlab/common` sends the type with every update
  for the same reason. All of it retires the day the tag changes.

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
other two cases, an update naming only the type and one changing only an
access level, were not measured, and the source says what they meet. At
GitLab 19.4.1-ee the PATCH hands the service
`declared_params(include_missing: false)`
([lib/api/project_packages_protection_rules.rb:123](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/project_packages_protection_rules.rb#L123)),
so a null key arrives where a missing one would not. `UpdateRuleService`
copies a present `package_name_pattern` into `pattern` as well
([app/services/packages/protection/update_rule_service.rb:33](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/app/services/packages/protection/update_rule_service.rb#L33)),
and the rule validates `package_name_pattern`, `package_type` and `pattern`
for presence
([app/models/packages/protection/rule.rb:35-40](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/app/models/packages/protection/rule.rb#L35-40)).
An update naming only the type therefore fails on the pattern twice, as
`package_name_pattern` and as `pattern`, and one changing only an access level
fails on those and on the type together.

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
in v3.0.0, v3.10.0, v3.12.0 and v3.14.0, so a dependency bump does not retire
it.

**Effort**: small, two struct tags and a test, like
[`SetFeatureFlagOptions`](#setfeatureflagoptions-fields-lack-omitempty).

### Five response keys and three parameters GitLab 19.4 added that v3.14.0 does not model

- **Reported**: yes, as commit 21 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds all five keys and all three
  parameters, the hook one as a `*bool` so `false` can be sent, and the usage
  as a `*NamespaceCIMinutesUsage` that is nil when GitLab leaves the key out.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes for the five keys, which are read from the captured
  response beside the SDK's decode
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)):
  through `toolutil.CapturedGroupHook`, `CapturedProject`,
  `CapturedInstanceUser` and `CapturedNamespace` in
  `internal/toolutil/sent_shapes.go`, and through `projectHookAPI`, the body
  the project hook handlers in `internal/tools/projects` already decode
  themselves. They retire when the structs carry the keys. None for the three
  parameters, which this server does not offer: taking them would mean building
  the request outside the SDK's option struct.

**What**: every key below is absent from the 19.3.1-ee live record and present
in the 19.4.1-ee one (`docs/development/gitlab-api-live.json`), and client-go
v3.14.0 declares none of them.

- `GroupHook` (`group_hooks.go`) and `ProjectHook` (`project_hooks.go`) declare
  no `duo_flow_callback_enabled`, which `ee/lib/api/entities/group_hook.rb` and
  `lib/api/entities/project_hook.rb` expose on every hook. `AddGroupHookOptions`,
  `EditGroupHookOptions`, `AddProjectHookOptions` and `EditProjectHookOptions`
  lack the parameter the four write routes declare, which group_webhooks.md and
  project_webhooks.md put behind the `duo_flow_callback_hooks` feature flag.
- `Project` (`projects.go`) declares no `ci_skip_branch_pipelines_for_mrs`,
  which `lib/api/entities/project.rb` exposes among the CI/CD settings a caller
  holding `admin_project` is sent, and `EditProjectOptions` lacks the parameter
  `PUT /projects/:id` declares for it. The release added
  `automatic_rebase_enabled` beside it, and that one client-go does model, on
  the struct and on the options.
- `User` (`users.go`) declares no `provisioned_by_project_id`, which
  `lib/api/entities/user_with_admin.rb` exposes with no license, so it reaches
  an administrator on every route presenting that entity. users.md dates it to
  19.3; the 19.3.1-ee record does not carry it.
- `Namespace` (`namespaces.go`) declares no `ci_minutes_usage`, the object
  `ee/lib/ee/api/entities/namespace.rb` renders through
  `ee/lib/api/entities/ci/minutes/usage.rb` (`total_minutes_used`,
  `monthly_minutes_used`, `purchased_minutes_used`, all integers) for a
  top-level namespace whose caller holds `admin_ci_minutes`.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), which
reported the five keys as soon as the live record was re-pinned to 19.4.1-ee
([issue 965](https://github.com/jmrplens/gitlab-mcp-server/issues/965)), and
the record's route params for the three parameters.

**Effort**: small. Four scalar members with `json` tags, one struct of three
integers for the usage, and three option fields with `url` and `json` tags.

### PlanLimit models eight of the twenty-nine limits GitLab sends and accepts

- **Reported**: yes, as commit 29 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds the twenty-one limits and
  `LimitsHistory` to `PlanLimit`, and the twenty-one limits to
  `ChangePlanLimitOptions` as pointers, so a limit set to 0 is still sent.
  `PlanLimit` gaining a map stops it being comparable with `==`, one of the
  four decisions the merge request asked the maintainers for. On 2026-09-29
  the maintainer answered that the library does not count comparability as
  part of its compatibility, so the fields stay.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes for the response, whose twenty-one other limits and
  change history are read from the captured response beside the SDK's decode
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  through `planlimits.capturedLimits`; it retires when the struct carries them.
  None for the parameters: `admin.plan_limits_change` offers the eight the
  options struct can send, and taking the other twenty-one would mean building
  the request outside the SDK.

**What**: `PlanLimit` in client-go v3.14.0's `plan_limits.go` carries the eight
package file sizes (`conan`, `generic_packages`, `helm`, `maven`, `npm`,
`nuget`, `pypi`, `terraform_module`), and it is what both plan limit routes
decode into. `lib/api/entities/plan_limit.rb` at 19.4.1-ee exposes twenty-nine
limits and `limits_history`, all with no condition, so both routes answer with
all of them and the SDK drops twenty-two keys: `cargo_max_file_size`,
`ci_instance_level_variables`, `ci_pipeline_size`, `ci_active_jobs`,
`ci_project_subscriptions`, `ci_pipeline_schedules`, `ci_needs_size_limit`,
`ci_registered_group_runners`, `ci_registered_project_runners`,
`dotenv_variables`, `dotenv_size`, `enforcement_limit`, `notification_limit`,
`storage_size_limit`, `pipeline_hierarchy_size`,
`max_pipelines_per_merge_train`, `service_desk_outbound_emails_per_hour`,
`service_desk_outbound_emails_per_day`, `web_hook_calls`,
`web_hook_calls_low`, `web_hook_calls_mid` and `limits_history`, the last an
object keyed by limit name whose entries carry `user_id`, `username`,
`timestamp` and `value` (`app/validators/json_schemas/plan_limits_history.json`).
`ChangePlanLimitOptions` has the same gap on the request side:
`PUT /application/plan_limits` declares the twenty-nine limits as optional
params in `lib/api/admin/plan_limits.rb`, and the options struct sends the
eight file sizes and `plan_name`.

The page is a partial oracle here.
[doc/api/plan_limits.md](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/doc/api/plan_limits.md)
lists every limit among the update parameters except `web_hook_calls_low` and
`web_hook_calls_mid`, which the route describes as GitLab.com only, and neither
of its example bodies carries the three webhook limits or `limits_history`. A
contributor working from the page alone would model twenty-seven limits and no
history.

**How we found it**: the sent dimension of the 1:1 audit
(`go run ./cmd/audit_1to1/ -scope=paths`), which listed the twenty-two keys at
package grain (`shapes.sent.unsurfaced`) and, once
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971) made the
type grain judge a wrapper that embeds its payload (`GetOutput`,
`ChangeOutput`) through that payload, against `PlanLimit` itself
(`shapes.typed.unsurfaced`, `sdk_models: false` on every one); and the route
params of the live record for the options.

**Effort**: small. Twenty-one `int64` fields and one map of a four-field struct
on `PlanLimit`, and twenty-one pointer fields with `url` and `json` tags on
`ChangePlanLimitOptions`.

### JobPipeline models five of the ten keys a job's pipeline carries

- **Reported**: yes, as commit 22 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds all five keys, `Source` as the
  `PipelineSource` type `Pipeline.Source` already uses.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `pipeline.resource_group_upcoming_jobs` reads the other
  five keys from the captured response beside the SDK's decode
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  through `resourcegroups.capturedUpcomingJobs`; it retires when the struct
  carries them.

**What**: `lib/api/entities/ci/job_basic.rb` at 19.4.1-ee exposes a job's
`pipeline` with `Entities::Ci::PipelineBasic`, which sends `id`, `iid`,
`project_id`, `sha`, `ref`, `status`, `source`, `created_at`, `updated_at` and
`web_url`, all with no condition. `JobPipeline` in client-go v3.14.0's
`jobs.go` carries `ID`, `ProjectID`, `Ref`, `Sha` and `Status`, so every
method answering with a `Job`, and the resource group queue's
`ListUpcomingJobsForASpecificResourceGroup`, drops the pipeline's number, what
started it, both timestamps and its page. `doc/api/jobs.md` prints the five
modeled keys in its examples, which is where the struct's shape comes from.

**How we found it**: review of the resource group queue's rows for
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971), which
published the nested pipeline and was held to the entity by hand. No audit
here asks the sent question one level down, since the type grain compares a
nested type only in the unpublished direction.

**Effort**: small. Five fields on `JobPipeline`, `IID` an `int64`, `Source` and
`WebURL` strings and the two timestamps `*time.Time`.

### AwardEmoji does not model the image URL of a custom emoji

- **Reported**: yes, as commit 23 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds `URL`, left empty when GitLab
  sends null for a standard emoji.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. Every `internal/tools/awardemoji` handler returning
  awards reads it from the captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  through `capturedOutput` and `capturedListOutput`; it retires when the struct
  carries it.

**What**: `lib/api/entities/award_emoji.rb` at 19.4.1-ee exposes `url` with no
condition, the image of a custom emoji and null for a standard one.
`AwardEmoji` in client-go v3.14.0's `award_emojis.go` stops at
`awardable_type`, so every award read or created through the SDK arrives
without it, and a custom emoji is a name with nothing to show.

**How we found it**: the package grain of the sent dimension
(`shapes.sent.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`), during
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971).

**Effort**: small. One `string` field.

### Diff does not model why a file diff arrives without its text

- **Reported**: yes, as commit 13 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds the three flags under the names
  and in the order `MergeRequestDiff` uses, which reaches the commit diff, the
  comparison and the merge request diff versions alike.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The commit diff and the comparison read the three flags
  from the captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  into `toolutil.DiffExtra`; it retires when the struct carries them.

**What**: `lib/api/entities/diff.rb` at 19.4.1-ee exposes `collapsed`,
`too_large` and `generated_file` on every file diff, with no condition. `Diff`
in client-go v3.14.0's `commits.go` models none of the three, so a diff GitLab
left out for its size decodes as a change with an empty `diff`, which is
indistinguishable from a file whose content did not change. `MergeRequestDiff`
in `merge_requests.go` already carries all three.

**How we found it**: the package grain of the sent dimension, during
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971).

**Effort**: small. Three `bool` fields.

### ApproveOrRejectProjectDeployment discards the approval GitLab records

- **Reported**: yes, as commit 14 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds
  `ApproveOrRejectProjectDeploymentV2`, which sends the same request and
  returns the approval as a new `DeploymentApproval`, deprecates the old
  method in its favour, and gives `Deployment` the approval count, the
  approvals and the approval summary the Enterprise Edition adds to the
  single deployment routes. `Deployment` gaining a slice stops it being
  comparable with `==`, one of the four decisions the merge request asked the
  maintainers for. On 2026-09-29 the maintainer answered that the library
  does not count comparability as part of its compatibility, so the fields
  stay.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `environment.deployment_approve_or_reject` reads the
  approval from the captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md));
  it retires when the method returns it.

**What**: `POST /projects/:id/deployments/:deployment_id/approval` answers with
`Entities::Deployments::Approval` (`ee/lib/api/entities/deployments/approval.rb`
at 19.4.1-ee: the user, the status, the time and the comment).
`ApproveOrRejectProjectDeployment` in client-go v3.14.0's `deployments.go`
decodes into `none` and returns only the `*Response`, so a caller cannot see
what was recorded without reading the deployment again.

**How we found it**: the type grain of the sent dimension, once
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971) paired
the compact rows a handler builds with the endpoints of the methods it calls.

**Effort**: small. Return a `*DeploymentApproval` (the struct the deployment's
own `approvals` list already decodes into) beside the response.

### ShareProjectWithGroup discards the link GitLab creates

- **Reported**: yes, as commit 19 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit adds `ShareProjectWithGroupV2`, which
  sends the same request and returns the link as a new `ProjectGroupLink`,
  and deprecates `ShareProjectWithGroup` in its favour, so the signature
  change this entry calls for is left for 4.0.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `project.share_with_group` reads the link from the
  captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md));
  it retires when the method returns it.

**What**: `POST /projects/:id/share` answers 201 with `Entities::ProjectGroupLink`
(`lib/api/entities/project_group_link.rb` at 19.4.1-ee: `id`, `project_id`,
`group_id`, `group_access` and `expires_at`, and, from the EE prepend,
`member_role_id` when the project may carry a custom role on the link).
`ShareProjectWithGroup` in client-go v3.14.0's `projects.go` decodes into
`none` and returns only the `*Response`, so the link's id, the one a later
update of the share needs, is not available to a caller.

**How we found it**: the package grain of the sent dimension, during
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971).

**Effort**: small. A `ProjectGroupLink` struct of six fields returned beside
the response, which changes the method's signature and so belongs in a major.

### GroupRelationStatus does not model the object count, and a relation's status does not decode

- **Reported**: yes, as commit 7 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open, by a different route than the one this entry
  proposes. The commit adds `TotalObjectsCount`, and rather than a
  `GetExportStatus` it makes `ListExportStatus` accept either answer and
  return a single status as a list of one, so a caller that sets `Relation`
  gets what it asked for and no signature changes; the method and its options
  now say so, the 404 included.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes for both.
  `group.group_relations_list_status` reads `total_objects_count` from the
  captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  through `grouprelationsexport.capturedExportStatuses`, and with `relation`
  set issues the request itself (`grouprelationsexport.getRelationStatus`,
  with client-go's own options) and decodes the one object. Both retire when
  the SDK carries the field and a method answering with one status.

**What**: two gaps in client-go v3.14.0's `group_relations_export.go`.
`lib/api/entities/bulk_imports/export_status.rb` at 19.4.1-ee exposes
`total_objects_count` with no condition, and `GroupRelationStatus` stops at
`batches`. The second is a defect rather than a gap: `GET
/groups/:id/export_relations/status` answers with an array, and with
`relation` set `lib/api/group_export.rb` presents that one export as an object
(`present export, with: Entities::BulkImports::ExportStatus`), or a 404 when
there is none. `ListExportStatus` decodes every answer into
`[]*GroupRelationStatus`, so the relation filter its own options offer fails
with a decode error on every instance. `lib/api/project_export.rb` answers the
project route the same way, which the SDK's project relations export would
meet too.

**How we found it**: the package grain of the sent dimension named
`total_objects_count` during
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971), and
reading the route to confirm it showed the object answer, which the unit test
had been mocking as an array.

**Effort**: small. One `int64` field, and a `GetExportStatus(gid, relation)`
returning one `*GroupRelationStatus`, with `Relation` dropped from the list's
options or documented as answering an object.

### Commit declares extended_trailers a map of strings, and GitLab sends lists

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: was yes, for `repository.commit_list` with `trailers` set: a
  page holding one commit with a trailer failed as a whole, with client-go's
  decode error in place of the page. It returns the page since
  [issue 1026](https://github.com/jmrplens/gitlab-mcp-server/issues/1026).
- **Workaround**: yes, except for two actions. Every handler that publishes
  `extended_trailers` types it as GitLab sends it, a map of lists, and reads
  the commit from the captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)),
  passing over client-go's own decode failure when a commit carries a trailer:
  `repository.tree` with `with_last_commit` (`repository.treeCommit`), the
  submodule update (`repositorysubmodules.submoduleCommitExtra`), both context
  commit actions (`mrcontextcommits.capturedCommit`,
  `mrcontextcommits.misreadByClientGo`), and since issue 1026 the commit
  actions of `internal/tools/commits` (`repository.commit_list`,
  `repository.commit_get`, `repository.commit_create`,
  `repository.commit_cherry_pick`, `repository.commit_revert`, through
  `commits.Captured` and `commits.MisreadByClientGo`), `repository.compare`
  and `repository.merge_base` (`repository.capturedCompare` and
  `commits.CapturedOutput`), the branch actions that answer with a branch
  (`branch.get`, `branch.list`, `branch.create`, through
  `branches.capturedBranch`), and the single merge request diff version
  (`mr_review.diff_version_get`, through `mrchanges.capturedDiffVersion`).
  Two actions publish `commits.Output` through `commits.ToOutput` from a commit
  client-go decoded, `merge_request.commits` (`internal/tools/mergerequests`)
  and `search.commits` (`internal/tools/search`): they carry
  `extended_trailers` in the right shape, empty, because a commit client-go
  decodes can have none, and their page still fails as a whole the day their
  route parses trailers. The handlers that decode a `Commit` inside an answer
  and publish no trailers (the commit of a tag, a release, a group release or a
  job, and the deployable commit of a deployment or an environment) fail the
  same way that day, and so do the readers outside the tools that take a
  commit from client-go: the commit, branch and tag resources
  (`internal/resources`, through `Commits.GetCommit`, `Branches.ListBranches`,
  `Branches.GetBranch`, `Tags.ListTags` and `Tags.GetTag`), the prompts that
  compare two refs or list branches (`internal/prompts`, through
  `Repositories.Compare` and `Branches.ListBranches`), and the branch, tag and
  commit completions (`internal/completions`, through `Branches.ListBranches`,
  `Tags.ListTags` and `Commits.ListCommits`). All of these retire when the
  struct carries the lists.

**What**: `lib/api/entities/commit.rb` at 19.4.1-ee exposes `extended_trailers`
with no condition, documented as a hash of each trailer to the list of its
values, and `Gitlab::Git::Commit#parse_commit_trailers`
(`lib/gitlab/git/commit.rb`) builds it that way, `(hash[trailer.key] ||= []) <<
value`. `Commit` in client-go v3.14.0's `commits.go` declares `ExtendedTrailers
map[string]string`, unchanged on the community fork's main, so decoding a commit
that carries a trailer fails with `json: cannot unmarshal array into Go struct
field Commit.extended_trailers.Signed-off-by of type string`, and the SDK
returns that error in place of the whole answer. Only one route fills the key
today, which is why nothing else has failed: measured on gitlab.com on
2026-09-27, `GET /projects/278964/repository/commits?trailers=true` answers
commit `9f1632e2` with `"Reviewed-by"` mapped to four values, while the same
commit read singly (`/repository/commits/9f1632e2...`) and as the last commit
of a tree entry (`/repository/tree?with_last_commit=true`) answers `{}` for
both trailer keys, as does the list without `trailers`. Only
`FindCommitsRequest` passes Gitaly `trailers`; `call_find_commit`,
`list_commits_by_oid` and the tree entries pass none. Every method answering
with a `Commit` (`ListCommits`, `GetCommit`, `CreateCommit`, `CherryPickCommit`,
`RevertCommit`, the comparison, the merge base, the context commits) fails the
same way the day its route starts parsing trailers.

**How we found it**: reading the entity while surfacing the context commit and
submodule commit keys for
[issue 971](https://github.com/jmrplens/gitlab-mcp-server/issues/971), then
decoding GitLab's own request spec fixture (`spec/requests/api/commits_spec.rb`,
`'Signed-off-by' => [...]`) into `gl.Commit`, and confirming the one route that
fills the key against gitlab.com. The sent audits compare key names and not
the value's shape, so neither grain can see it.

**Effort**: one field type, `map[string][]string`, which breaks exported API:
a major version, or a second field beside the old one that decodes the lists.

### The Orbit schema format is sent as `format`, and its llm answer is not modelled

- **Reported**: yes, as commit 3 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit tags `GetOrbitSchemaOptions.Format`
  `response_format`, so a caller that sets it gets the format it asked for
  without changing any code, and adds `FormattedText` to `OrbitSchema`.
- **Merged**: no.
- **Blocking**: yes for the SDK's own callers, no here. Through client-go,
  `GetSchema` with `Format` set is refused 406 by GitLab.com, and with
  `response_format=llm` it would decode to an empty `OrbitSchema`.
- **Workaround**: yes for both. `orbit.Schema`
  (`internal/tools/orbit/orbit.go`) leaves `GetOrbitSchemaOptions.Format`
  unset and sets `response_format` itself through a `gl.RequestOptionFunc`
  (`responseFormatQuery`), which client-go applies after it has encoded the
  options, and reads `formatted_text` from the captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  into `SchemaOutput.FormattedText`, which the R-OUTPUT tables declare in
  `docAddedFields`. Both retire when the SDK sends the parameter under
  GitLab's name and models the field.
  `TestSchema_LLMFormat_PublishesTheFormattedText` holds the llm path, the
  schema tests assert that no `format` parameter is sent, and the Orbit live
  suite drives both formats under both input names.

**What**: two gaps in client-go v3.14.0's `orbit.go`, the same on `main` when
read on 2026-09-27. `GetOrbitSchemaOptions.Format` carries
`url:"format,omitempty"`, and GitLab's route declares the parameter as
`response_format` (`ee/lib/api/orbit/data.rb`, `get :schema`,
`optional :response_format, type: String, values: %w[raw llm], default: 'raw'`),
as it does on every Orbit route that takes a format. `format` is the parameter
Grape reserves for the representation it renders, so `?format=llm` asks Grape
for an `llm` representation and is answered
`406 {"error":"The requested format 'llm' is not supported."}` before the route
runs; `?format=raw` is refused the same way. With `response_format=llm` the
route answers `{"formatted_text": "..."}` and nothing else (`get_graph_schema`
in `ee/lib/analytics/knowledge_graph/grpc_client.rb`), a field `OrbitSchema`
does not carry, while `OrbitGraphStatus` and `OrbitStatusSystem` already model
the same key for their own llm answers.

**How we found it**: the Orbit response record of
[issue 972](https://github.com/jmrplens/gitlab-mcp-server/issues/972)
records each Orbit call through the handlers against GitLab.com, and the
schema's llm call was refused with 406. The unit tests had pinned
`format` as the parameter name, so they passed against a request GitLab
refuses. The handler fix comes with the change that closes issue 972.

**Effort**: small. Retag the field as `url:"response_format,omitempty"`, or
add a `ResponseFormat` field with that tag, as the other four Orbit option
structs name it, and keep `Format` as a deprecated alias encoded under the
right name; add `FormattedText string` with `json:"formatted_text,omitempty"`
to `OrbitSchema`; correct the option's doc comment, which names `format` as
the parameter.

### No client-go helper returns the RFC 6750 fields of a token refusal

- **Reported**: no. The issue is drafted below. It goes after row 83's issue,
  which it cites, and only once this project has decided to adopt the helper,
  since a named consumer is what makes it acceptable; like every item here it
  waits on the maintainer's approval and an independent review of its text.
  An issue is not held behind the open merge requests: the maintainer who
  asked for fewer merge requests asked for issues instead.
- **In review**: no. The merge request follows a maintainer's answer on the
  issue, and only after
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065)
  and
  [gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066)
  have been reviewed.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: not needed. This server decodes the body itself, in
  `errorCode` and `carriesInvalidToken`
  (`internal/gitlab/credential_refusal.go:190` and `:183`), which
  `UnauthorizedNamesCredential` and `RefusalMayBePermission` read
  ([row 55](#a-permission-refusal-is-answered-401-rather-than-403)). The first
  is one rule with two readers that the file says must not disagree: the
  transport, which reads a raw prefix of a 401's body before client-go builds
  any error (`classifyUnauthorized`, `internal/gitlab/response_limit.go:130`),
  and `internal/toolutil/errors.go`, which holds the `*ErrorResponse` (`:313`,
  and `:601` through `RefusalMayBePermission`). The helper replaces the two
  decoders only if this project adopts it, which is the decision the issue
  waits on. `isInsufficientScope` in `internal/oauth/verifier.go` decodes a
  response of the OAuth verifier's own HTTP client and stays either way.

**Where**: client-go v3.15.0 (`be72a3fe`), `gitlab.go`. `CheckResponse` (line
1412) keeps the raw body in `ErrorResponse.Body` (1427) and flattens it into
`Message` with `parseError` (1433), so `err.Error()` already ends in
`{error: insufficient_granular_scope}, {error_description: Access denied:
...}`. Nothing returns the fields typed.

**What**: a gap rather than a defect. A caller that wants the RFC 6750 code, to
tell a revoked token (`invalid_token`) from a missing scope
(`insufficient_scope`) or a missing fine-grained permission
(`insufficient_granular_scope`), decodes GitLab's body itself, as this server
does. `HasStatusCode` (1484-1491) and `StatusCode` (1495-1502) cannot tell
those apart, and GitLab answers some permission refusals with a plain 401
(row 55), which the body separates from a revoked or expired credential. A
token GitLab cannot find gets the same plain 401
(`lib/gitlab/auth/auth_finders.rb:414` raises `UnauthorizedError`, which
`lib/api/helpers.rb:1034-1036` answers with `unauthorized!`), which is why row
55's workaround puts such a 401 to the credential probe. A classic token can
meet `insufficient_granular_scope` too: under a group's enforcement on
GitLab.com, GitLab refuses a legacy token with the fine-grained sentence, which
[gitlab-org/gitlab#616442](https://gitlab.com/gitlab-org/gitlab/-/issues/616442)
shows for a service account's legacy token, so a client-go caller on
GitLab.com whose classic token reaches such a group can see that code. On a
self-managed instance, enforcement only stops legacy tokens from being created
or rotated (`doc/auth/tokens/fine_grained_access_tokens.md`, lines 134 to 153;
`NamespaceSetting#granular_tokens_enforced?` is false unless the GitLab.com
flag `granular_personal_access_tokens_enforcement_saas` is on,
`app/models/namespace_setting.rb:246-250`).

**The shape proposed**: package-level functions in the style of `StatusCode`,
which
[gitlab-org/api/client-go!3055](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3055)
added. They decode a body only when it has the shape GitLab's API guard
renders, and return the fields exactly as sent. The name does not end in
`Error`, because the type is not an `error` and client-go's error types are:

```go
// TokenRefusal is the RFC 6750 error GitLab's API guard answers a refused
// token with, as GitLab sent it.
type TokenRefusal struct {
	Code        string   // invalid_token, insufficient_scope, insufficient_granular_scope, ...
	Description string   // error_description, verbatim
	Scope       []string // the scope attribute split on spaces; empty when absent
}

// ParseTokenRefusal decodes a response body in the shape GitLab's API guard
// renders, and returns false for a body of any other shape.
func ParseTokenRefusal(body []byte) (*TokenRefusal, bool)

// TokenRefusalFrom applies ParseTokenRefusal to the body of the first
// *ErrorResponse in err's chain, and returns false when there is none.
func TokenRefusalFrom(err error) (*TokenRefusal, bool)
```

Two entry points to one rule, because this server reads the same body in two
places: a transport deciding what a 401 means before client-go builds an error
has only the bytes, and a handler describing an error has the
`*ErrorResponse`. `CheckResponse` and the type it returns do not change, so
`HasStatusCode`, `StatusCode`, `errors.Is(err, ErrNotFound)` and the type
assertion at `graphql.go:177` behave as today. That avoids the riskier shape
go-github chose, distinct error types returned from its `CheckResponse`
(`TwoFactorAuthError`, `RateLimitError`, `AbuseRateLimitError`, in
`github/github.go` on its master). It does not parse `[Work Item: Read]`:
[row 83](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose)
asks GitLab for that value instead, and if GitLab sends it in `scope`, these
functions return it with no further change.

**Why only with a named consumer**, in the maintainers' words:

- `StatusCode` came with one, and a maintainer asked for it: Timo Furrer
  requested it on a glab merge request
  ([gitlab-org/cli!3898 note 3831430533](https://gitlab.com/gitlab-org/cli/-/merge_requests/3898#note_3831430533))
  so that `glab` could shorten its update check error, opened it himself, and
  when a bot tried to drop the helper wrote "I still want the convenience
  helper"
  ([note 3831750912](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3055#note_3831750912)).
- Patrick Rice wants "the solution to be driven by a given problem as opposed
  to just general golang convention"
  ([gitlab-org/api/client-go#2119 note 2488046687](https://gitlab.com/gitlab-org/api/client-go/-/issues/2119#note_2488046687),
  on the package layout), and asked of an addition nobody used yet, "Is there
  value here until we can change the MergeRequest struct too?"
  ([gitlab-org/api/client-go!3013 note 3755379471](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3013#note_3755379471)).
- The objection to expect is go-github's to a typed header: "we already
  provide the actual response (along with its headers) to the client user of
  this repo, so maybe nothing more needs to be done here"
  ([google/go-github#2886](https://github.com/google/go-github/issues/2886#issuecomment-1701051006)).
  client-go provides `Body` and `Message` just as completely.
- A change at the library level goes in a merge request of its own, with
  integration tests: Patrick Rice split two of this project's changes out of
  the joint merge request because "these are more fundamental changes at the
  library level"
  ([gitlab-org/api/client-go!3063 note 3926712100](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3926712100)).
- Heidi Berry's "Usually, we just return the API response rather than
  attempting to map it like this"
  ([gitlab-org/api/client-go#2175 note 2900268862](https://gitlab.com/gitlab-org/api/client-go/-/issues/2175#note_2900268862))
  is about the `users.go` sentinels below, and supports removing them, not
  adding this helper.

**The consumer, and the decision it waits on**: this server would use
`ParseTokenRefusal` in the transport and `TokenRefusalFrom` in
`internal/toolutil`, retiring `errorCode` and `carriesInvalidToken` while
keeping one rule for both readers, and would read the code of a 403
`insufficient_granular_scope` through it in phase A of
[issue 952](https://github.com/jmrplens/gitlab-mcp-server/issues/952), which
maps that refusal to an uncharged one naming the missing permission when
GitLab gives it. Whether it does is the maintainer's decision, and the draft's
consumer sentence commits it, so the issue goes out only after that decision.
[Issue 1103](https://github.com/jmrplens/gitlab-mcp-server/issues/1103) is the
GraphQL side and outside the helper: a GraphQL refusal of a mutation arrives
with HTTP 200 and an `errors` array, and `GraphQL.Do` builds a
`GraphQLResponseError` only on a non-2xx status (`graphql.go:173-186`), so if
GitLab adopts that issue's upstream change the matching accessor is a
proposal of its own. glab would be the other candidate consumer; it has no
hint for either code, and nobody has asked there.

**Limits the issue states**:

- A project or group hidden from the token's user answers a plain 404,
  `{"message":"404 Not Found"}`, the same bytes as a missing resource, which
  reaches callers as `ErrNotFound` (`gitlab.go:1416-1417`).
- REST only, for the reason above.
- For a classic token `Scope` is the list registered for the API class plus
  the API-wide scopes (`lib/api/api_guard.rb:102-114` in GitLab), not what the
  method needs: the body quoted in
  [gitlab-org/api/client-go#2175](https://gitlab.com/gitlab-org/api/client-go/-/issues/2175)
  for a block call carried `read_user ai_workflows api read_api`.
- The code does not always mean a permission is missing: two refusals carry
  `insufficient_granular_scope` with a sentence and no permission (row 83's
  fourth property), so `Scope` stays empty there even after row 83's change.
- Six user moderation methods replace a 403 with a client-side sentinel, so
  the helper cannot see those refusals: `UnblockUser` (`users.go:1045`),
  `DeactivateUser` (`:1107`), `ActivateUser` (`:1129`), `ApproveUser`
  (`:1151`), `RejectUser` (`:1173`) and `DisableTwoFactor` (`:1360`). Removing
  them was agreed on the same issue (Timo Furrer, "NOT return that specific
  LDAP error from client-go",
  [note 2901967459](https://gitlab.com/gitlab-org/api/client-go/-/issues/2175#note_2901967459);
  Patrick Rice, "I agree",
  [note 2903422993](https://gitlab.com/gitlab-org/api/client-go/-/issues/2175#note_2903422993)),
  `BlockUser` was fixed on main by
  [gitlab-org/api/client-go!2581](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2581)
  (`366f12a8`, v0.161.0), and
  [gitlab-org/api/client-go!2632](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2632)
  (`e56b79e8`, into `release-client-2.0`) removed the rest. On 2026-02-14 the
  merge of main into `release-client-2.0` (`2ba417c9`) brought the sixteen
  returns back, and its fix-up, `0152d148` ("Resolve several merge issues"),
  the ten `ErrUser*` declarations; they ship in v2.0.0, v3.0.0 and v3.15.0, and
  nothing records whether that was deliberate. Wrapping them would still print
  the sentinel's own sentence, the defect that issue reported, and callers have
  compared them since v2.0.0, so restoring the removal belongs in an issue of
  its own for 4.0, beside
  [gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/issues/2301),
  which lists the breaks deferred from the joint merge request, or behind a
  toggle, as Patrick Rice asked for another behaviour change ("implement it as
  a feature toggle with a note saying that in v4 the toggle would become the
  default",
  [gitlab-org/api/client-go!3063 note 3924608467](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3924608467)).
  The merge resolution was a maintainer's, so the wording stays neutral.

**Tests the merge request carries**: unit tests beside `TestCheckResponse`
(`gitlab_test.go:226`) for each body shape, a body that is not JSON, an empty
body, an error that is not an `*ErrorResponse` and one wrapped with
`fmt.Errorf("%w")`; and integration tests with classic tokens, which the
fixture can make today: an impersonation token created with `read_api` and
used for a write (`insufficient_scope`), and the same token revoked
(`invalid_token`, "Token was revoked"). `ImpersonateTestUser`
(`gitlab_test/utils_test.go:145-165`) hard-codes the `api` scope, so the test
calls `CreateImpersonationToken` itself, or the helper gains a scopes
parameter. A fine-grained test waits on
[row 82](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them),
since client-go cannot create such a token until then, and a raw request in a
test would draw a reviewer's question.

**What it costs this server**: nothing beyond the decoding it already carries;
see
[row 83's section](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose)
for what none of rows 82 to 85 gives issue 952.

**Verified, inferred and unverified**:

- Verified at client-go v3.15.0 and GitLab 19.4.1, in this repository at
  `origin/main`, and on gitlab.com and GitHub with GET requests on 2026-09-30:
  every line and every quote above; the sentinel history, read with `git show
  <commit>:users.go` at each commit and tag; and the enforcement case, through
  the issue above.
- Inferred: that a refused GraphQL mutation arrives with HTTP 200 and
  `errors`.
- Unverified: whether client-go's maintainers accept the helper, and in which
  shape; whether glab or the Terraform provider want it; whether the
  restoration of the sentinels was deliberate.

**Before posting**: decide whether this server adopts the helper, since the
draft says it will; decide whether to link row 55 of this register for the 401
case, which is not reported upstream, or keep the draft stating the behaviour,
as it does now; fill `gitlab-org/gitlab#<n>` with row 83's issue, which goes
first; re-read `gitlab.go` and `users.go` on client-go's main.

**Effort**: small, one type and two functions with doc comments, unit tests
and two integration tests, roughly 100 to 200 lines, an estimate. A plain
`feat:` title, as the `StatusCode` merge request had, and the `type::feature`
label.

<details>
<summary>Draft: the gitlab-org/api/client-go issue</summary>

Reviewed twice while it was prepared, and corrected after a third review; not
posted. Native references, first person as the maintainer, one paragraph per
line.

Title: `Expose the RFC 6750 fields GitLab returns when it refuses a token`

```markdown
## Summary

When GitLab's API guard refuses a token it answers with an RFC 6750 error in the body: `error` (`invalid_token`, `insufficient_scope`, `insufficient_granular_scope`, ...), `error_description`, and for `insufficient_scope` a `scope` attribute. client-go keeps that body in `ErrorResponse.Body` and flattens it into `Message`, so every caller that wants to act on the code decodes GitLab's body format itself. I would like to add a small helper, in the style of `StatusCode`, that returns those fields exactly as GitLab sent them.

## Problem

`HasStatusCode(err, 401)` and `HasStatusCode(err, 403)` do not say whether the credential itself was refused. Two cases where that matters to a caller:

- A 401 with `invalid_token` means GitLab found the token and refused it: revoked, expired, or an impersonation token where impersonation is disabled. GitLab also answers some permission refusals with a plain 401 (`{"message":"401 Unauthorized"}`), where the token is fine and only the caller's role is not, and it answers a token it cannot find at all with that same plain 401. Only the body tells a revoked or expired token from a role refusal.
- On GitLab.com the owner of a top-level group can require fine-grained tokens. After the enforcement date a classic token is refused in that group with `insufficient_granular_scope`, as gitlab-org/gitlab#616442 shows for a service account's legacy token, so a caller holding a classic personal access token can meet that code without ever creating a fine-grained one. On a self-managed instance, enforcement only stops legacy tokens being created or rotated.

I maintain gitlab-mcp-server, which is built on client-go and decodes these bodies itself today (`errorCode` and `carriesInvalidToken` in `internal/gitlab/credential_refusal.go`): once in its HTTP transport, on the body of a 401 before client-go builds an error, and again where it describes an error to a model. I would switch both to the functions below, so they have a consumer from the first release.

## Proposal

A body-level function, and a helper that applies it to the first `*ErrorResponse` in the chain. Each decodes the body only when it has that shape. Naming is open, for example:

    type TokenRefusal struct {
        Code        string   // invalid_token, insufficient_scope, insufficient_granular_scope, ...
        Description string   // error_description, verbatim
        Scope       []string // the scope attribute split on spaces; empty when GitLab sends none
    }

    func ParseTokenRefusal(body []byte) (*TokenRefusal, bool)
    func TokenRefusalFrom(err error) (*TokenRefusal, bool)

- The body-level function is for a caller that reads a response before client-go builds an error, such as a transport deciding what a 401 means. Keeping one rule behind both is what stops the two readers from disagreeing.
- `CheckResponse` and the error type it returns do not change, so `HasStatusCode`, `StatusCode`, `errors.Is(err, ErrNotFound)` and the type assertion in `graphql.go` behave exactly as today.
- It returns GitLab's values and nothing of its own. It does not parse the permission names out of the `insufficient_granular_scope` description, which is text GitLab changes.
- I am happy to mark it experimental, like `Routes()`, while the shape is agreed.

I am not proposing to record the fine-grained permission of each method here; that answer belongs in GitLab, and I opened gitlab-org/gitlab#<n> asking GitLab to name the missing permissions in a machine-readable field of that response. If it lands, these functions return them with no further change.

<details>
<summary>What GitLab sends today (19.4.1)</summary>

- Revoked token, `lib/api/api_guard.rb`: `401` with `WWW-Authenticate: Bearer realm="...", error="invalid_token", error_description="..."` and the body `{"error":"invalid_token","error_description":"Token was revoked. You have to re-authorize from the user."}`.
- Classic token missing a scope, same file, documented in the REST authentication page: `403 {"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"..."}`. The `scope` list is the one registered for the API class, not per method: the body quoted in #2175 for a block call listed `read_user ai_workflows api read_api`.
- Fine-grained token missing a permission, same file, text from `app/services/authz/tokens/authorize_granular_scopes_service.rb`: `403 {"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following project permissions: [Work Item: Read]."}`.
- The two 403s carry no `WWW-Authenticate` header (a FIXME in `api_guard.rb` says so), so for them the body is the only carrier; the 401 carries its code in both.

</details>

<details>
<summary>Known limits</summary>

- When a fine-grained token's project or group is hidden from the token's user, GitLab answers a plain 404, `{"message":"404 Not Found"}`, the same bytes as a missing resource, which reaches callers as `ErrNotFound`.
- REST only. GraphQL refusals of a mutation arrive with HTTP 200 and an `errors` array, and a refused query field is `null`.
- `insufficient_granular_scope` also answers two refusals no permission fixes ("Fine-grained personal access tokens are not yet supported." and "This operation doesn't support fine-grained personal access tokens."), so the code alone does not say a permission is missing.
- The user moderation methods in `users.go` (`UnblockUser`, `DeactivateUser` and siblings) still replace a 403 with a client-side error, so the helper cannot see those refusals. Removing those errors was agreed in #2175 and merged in !2632, and they came back in a later merge resolution; since callers have compared them since v2.0.0, I would raise that in an issue of its own for 4.0 rather than in this change.

</details>

<details>
<summary>Tests</summary>

- Unit tests next to the `CheckResponse` tests: each body above, a non-JSON body, an empty body, a non-`ErrorResponse` error, and an `ErrorResponse` wrapped with `fmt.Errorf("%w")`.
- Integration tests against the Docker instance: an impersonation token created with `read_api` and used for a write (`insufficient_scope`), and the same token after revocation (`invalid_token`).

</details>

<details>
<summary>Where this comes from</summary>

I ran into this while maintaining [gitlab-mcp-server](https://github.com/jmrplens/gitlab-mcp-server), an MCP server built on client-go. The project keeps a record of everything done on its dependencies and on sibling projects in [docs/development/upstream-bugs.md](https://github.com/jmrplens/gitlab-mcp-server/blob/main/docs/development/upstream-bugs.md). I looked at recording each method's fine-grained permission here and dropped it: you have left GitLab facts about a method, such as whether it is CE or EE, to its consumers (!2931).

</details>

Two questions before I open a merge request: would you prefer functions like the ones above, or fields on `ErrorResponse` filled in by `CheckResponse`? I lean towards the functions because they add nothing to a struct callers build in their own tests. And should they carry the experimental note?
```

</details>

## MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)

Nine of the entries here were filed upstream together on 2026-09-13, one issue
each so that a maintainer can triage, label and close them apart, with
[modelcontextprotocol/go-sdk#1257](https://github.com/modelcontextprotocol/go-sdk/issues/1257)
as an index over the set. The four that need no new exported API carry a pull
request; the four that do are proposals waiting on a decision about the shape,
because a maintainer chooses their own API and a pull request that assumes the
answer wastes both sides' time. The tenth, `Mcp-Name`, was fixed upstream by
somebody else before we got to it.

The index had fallen behind by 2026-09-27: its body, last edited on
2026-09-15, read every pull request it lists as open while five had merged
([modelcontextprotocol/go-sdk#1255](https://github.com/modelcontextprotocol/go-sdk/pull/1255),
[#1268](https://github.com/modelcontextprotocol/go-sdk/pull/1268),
[#1269](https://github.com/modelcontextprotocol/go-sdk/pull/1269),
[#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273) and
[#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274)), did not
name
[modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293)
as the pull request for
[modelcontextprotocol/go-sdk#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262),
and did not list
[modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299).
It was rewritten that evening (21:51 UTC): each pull request reads merged or
open as it stands, #1293 is named under #1262, and a "Filed since" section
lists #1299, with the closing sentence narrowed to the nine issues of the
original audit, which are the ones that link back to it.

The three added on 2026-09-27 are not part of that batch. Each decides what a
caller of this server sees when it is refused, and two of them were already
reported upstream by other users when this server's specification found them,
so they carry no issue of ours; the third,
[modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299),
was filed on 2026-09-25.

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
  both still open, the pull request conflicting with `main` when read on
  2026-09-29) and the keep-alive comment
  ([modelcontextprotocol/go-sdk#1229](https://github.com/modelcontextprotocol/go-sdk/issues/1229),
  since closed by the merge below).
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232),
  merged on 2026-09-21. Ours is
  [modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293),
  opened on 2026-09-24 on the shape settled on the issue on 2026-09-18, where
  a contributor preferred extending the write deadline on every write once
  [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232)
  had merged: every write to a stream that runs a keep-alive, which today
  means `subscriptions/listen`, moves `http.Server.WriteTimeout`'s deadline
  out by twice the keep-alive interval, extended rather than cleared so a
  peer that stopped reading still fails. Its test fails against `main` every
  time and waits for review. Read on 2026-09-27 it is up to date with `main`,
  its nine checks are green, it has had no review yet, and nothing is owed on
  it from here. On 2026-09-29 it was six commits behind `main`; I updated it
  from `main` (merge commit `4bb3af60`) and its nine checks passed. It has
  still had no review.
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
  open with no review and conflicting with `main` as of 2026-09-27, untouched
  since 2026-08-29. A comment on the issue that day, not ours, confirms the
  defect in v1.8.0-pre.2 and reports that the pull request as it stands
  (`3906b01`) leaves part of it open: it answers a syntax error with `-32700`
  twice, and a frame that is valid JSON but not a JSON-RPC message (`42`,
  `{}`, `[]`) still ends the session, taking with it the answers owed to
  requests that arrived before it. That is not ours to answer, and nothing
  here waits on us. We open no second one; if it stalls, the evidence here
  (the e2e case and the stdio filter) goes on that thread rather than into a
  new pull request.
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
  A POST whose every call was cancelled ends with nothing written, and since
  2026-09-20, on a contributor's review, it is answered 204 No Content in
  both JSON and SSE mode rather than an empty 200. It was rebased on
  2026-09-24 after
  [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232)
  conflicted in `deliverLocked`, and the 204 now keys on the `lastWrite`
  field that pull request added, which says the headers are still
  uncommitted. It is mergeable and waits for review. Read on 2026-09-27 it is
  still up to date with `main`, its ten checks are green, and its one review
  thread, answered on 2026-09-20, waits on the reviewer. On 2026-09-29 it was
  six commits behind `main`; I updated it from `main` (merge commit
  `761b52b2`) and its ten checks passed. The thread still waits.
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
  way, `Server.notifySessions` and `Server.ResourceUpdated`, and were left out
  of the first version on purpose; since 2026-09-27 the pull request carries
  them too (below).

  **Where it stands on 2026-09-29.** The two things owed on 2026-09-27 were
  done that day in one push that also rebuilt the branch on `main` as it then
  stood (`e07f0c9d`). The promise of 2026-09-15 in the reviewer's thread is
  kept: once
  [modelcontextprotocol/go-sdk#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274)
  landed, the doc comment of `protocolVersion()` stopped being true, and
  `da8f87c9` restates it (the fallback to the declared version now covers only
  a caller-supplied `ServerSessionState` and state saved by an older release)
  and cuts every comment the branch adds to the maintainers' three-line rule of
  2026-09-20. `b6f3c555` renames the predicate to `negotiatedLegacyProtocol`,
  and `6759c168` brings the two sites left out into the same decision:
  `notifySessions` and `ResourceUpdated` now go through
  `ServerSession.speaksLegacyProtocol`, so a client negotiated down to
  2025-11-25 by `initialize` is notified on the shared session channel rather
  than on a `subscriptions/listen` stream its version cannot open, and is no
  longer sent a per-session subscription id in `_meta`. A session that has
  recorded no version at all moves from legacy to new-protocol, which is what
  SEP-2575 says such a session is. No reviewer has answered since, and `main`
  gained three commits on 2026-09-28, so the branch was behind again. At 09:15
  UTC on 2026-09-29 @guglielmo-san updated it from `main` (merge commit
  `85f855fd`), and all nine checks passed on it. `main` gained two more
  commits the same morning, and at 12:42 UTC I updated it again with GitHub's
  Update branch (merge commit `bb0e7c5a`), which merges `main` in and so kept
  `85f855fd`, where a rebase would have dropped it; its nine checks passed
  again. It has had no review since 2026-09-15.
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
  has none: no method of the SDK sends the notification for a request the
  client made, so there is nothing to interpose.

**What**: 2026-07-28 says a server "MUST send `notifications/cancelled`
referencing a `subscriptions/listen` request ID when it tears down that
subscription stream". go-sdk sends `notifications/cancelled` only when a call
it made itself is abandoned (`call` and `cancelCall` in `mcp/transport.go`), and
`ServerSession` offers no method that sends one for a request the client made.
A server that ends a subscription can therefore satisfy the graceful half of
the contract and not this one. This entry used to give the reason as
`SubscriptionsListenResult` embedding an unexported type; that type is the
listen's result, not this notification, and application code can build one,
which go-sdk sends only in place of its own handler
(`internal/tenancy/channels_integration_test.go` holds that).

**How we found it**: the interaction-pattern specification audit. Reproduced on
stdio at 2026-07-28: a watcher retired after its resource began returning 404,
and the only output was the listen request's own result — no
`notifications/cancelled`, before or after, with the connection still usable.

**Recorded in**: ADR-0015, so the gap is stated where the design is rather than
only here.

**Pinned by**: `TestListenEnd_WatchedResourceGone_CompletesTheListenAndSendsNoCancellation`
in `test/e2e/stdio`, which is that reproduction on the real binary: it lets a
listen be acknowledged, makes its issue answer 404, and asserts that the listen's
own result carries `resource_gone` with the status and that no
`notifications/cancelled` arrives before it or before the answer to a later call.
The change that starts sending the notification fails it, with a message naming
this entry, ADR-0015 and F-24 of the tenant policy specification.

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
  here depends on what they choose. The case is on record upstream all the
  same:
  [modelcontextprotocol/go-sdk#1275](https://github.com/modelcontextprotocol/go-sdk/pull/1275),
  opened by another contributor and merged on 2026-09-18 (`4608cda9`, in no
  release yet), fixed the listen teardown for the three list-changed
  registries and names this case under "Known limitations": "Two listens
  subscribed to the same URI. `resourceSubscriptions[uri][session]` overwrites
  the same way, and the deferred `unsubscribe` deletes it." Its author offered
  to open an issue with tests, and none has been opened. On that pull request
  @guglielmo-san held on 2026-09-17 that keying by session is fine and only
  the teardown was wrong; the author answered the same day with a failing test
  for the resource path. What we file has to meet that position.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. It needs a client that opens two `subscriptions/listen`
  covering one URI, or mixes a legacy `resources/subscribe` with a listen, on
  stdio. Neither HTTP mode reaches it: v1.8.0's stateful handler refuses every
  2026-07-28 request but `server/discover` ("this server is stateful",
  `mcp/streamable.go`), so `--stateless=false` serves no listen at all, and a
  stateless POST's session ends with its response. The SDK's own client never
  holds two at once, which is why
  [modelcontextprotocol/go-sdk#1275](https://github.com/modelcontextprotocol/go-sdk/pull/1275)
  calls the case unreachable from the Go client, but it reaches the same
  state when it unsubscribes a URI and subscribes it
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

### A tool, prompt or resource result a middleware makes carries no `resultType`

- **Reported**: yes, by another user, not by us:
  [modelcontextprotocol/go-sdk#1225](https://github.com/modelcontextprotocol/go-sdk/issues/1225),
  opened on 2026-09-01, names this server's case exactly: a receiving
  middleware that answers a `tools/call` itself, as an authentication, rate
  limit or entitlement gate does.
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1226](https://github.com/modelcontextprotocol/go-sdk/pull/1226),
  merged on 2026-09-14 as e40f35d.
- **Merged**: **yes, unreleased.** The merge landed thirteen minutes before
  v1.8.0 was published, but v1.8.0 is tagged on the commit v1.8.0-pre.2 was,
  3f3b699, four commits before it, so the v1.8.0 pin does not carry it. No later
  tag exists as of 2026-10-01.
- **Blocking**: no client is known to refuse it, and with the workaround below
  none meets it; without it this server breaks a MUST. The 2026-07-28 schema
  says a server implementing that revision MUST include `resultType`
  (`Result.resultType` in `schema.ts`), and its rule that a client reads an
  absent field as `complete` covers only a result from a server implementing an
  earlier revision. Until the workaround, every `tools/call` this server's rate
  limiter refused at 2026-07-28 was nonconformant, and a client that holds a
  2026-07-28 server to the schema was entitled to reject it. It was "no" in the
  summary on practical grounds alone: the Go SDK client reads an empty
  `resultType` as complete (`CallToolResult.NeedsInput` in `mcp/protocol.go`),
  which is how `channels_integration_test.go` receives the refusal as the tool
  error it is.
- **Workaround**: yes, until the bump to the first tag carrying e40f35d.
  `toolutil.LabelForRevision` (`internal/toolutil/result_type.go`) labels a tool
  result a receiving middleware returns. When the request names 2026-07-28 or
  later in its `_meta`, compared as a string, the result is written with every
  field it has and the `resultType` the SDK's dispatcher would have given it,
  `"input_required"` when it carries `InputRequests` and `"complete"` otherwise,
  and read back through `CallToolResult.UnmarshalJSON`, which is public and
  copies the wire `resultType` into the unexported field (`mcp/protocol.go`);
  the fields decoding rebuilds rather than keeps (the content, the structured
  content, the input requests and the `_meta`) are then put back as they were.
  An earlier revision gets the result as it is. The `_meta` test is the one
  v1.8.0 applies to the results it labels once the middleware chain returns
  (`validateRequestMeta` in `mcp/shared.go`, then `setCompleteResultType`), and
  the one e40f35d applies to every result; v1.8.0's tool dispatcher reads
  instead the revision the session recorded when it began
  (`clientSupportsMultiRoundTrip` in `mcp/mrtr.go`), and the two differ only
  where that revision and a request's `_meta` disagree: a client negotiated down
  from 2026-07-28 gets the dispatcher's label and not this one, which its
  revision does not require, and a client that negotiated an earlier revision
  and then names 2026-07-28 in a request's `_meta`, which v1.8.0 accepts over
  stdio, gets this label and not the dispatcher's, which is what that revision
  requires and what e40f35d sends. The params are read by their concrete type, because a typed
  nil behind the `Params` interface panics in `GetMeta`. It is applied to the
  two tool results a receiving middleware here makes: the rate limiter's
  refusal, where it leaves the middleware (`attachRateLimitFunc` in
  `internal/toolutil/rate_limit.go`, the ToolError channel of row RTC-001), and
  the held-call ceiling's (`heldRequestsRefusal` in `cmd/server/held.go`, row
  HLD-011), whose 2026-07-28 calls the gate counts, or refuses, before the SDK
  reads them, so its label is defensive. No middleware here makes a
  `prompts/get` or `resources/read` result. Each refusal keeps its channel, its
  text and its error flag: row RTC-001 declares the ToolError channel for
  `tools/call` so that a model reads the refusal as a tool result it can back
  off from, which moving it to the `-42900` JSON-RPC error the limiter writes
  for the other metered methods would have traded away. This entry used to say
  the field could not be set from outside the SDK, because the setter is
  unexported; the public decoder sets it, which is how the sibling project
  libgen-mcp labels its own refusals
  ([jmrplens/libgen-mcp@af1cdef](https://github.com/jmrplens/libgen-mcp/commit/af1cdef3cf2b6436087d63f706ab7aac68e143df)).
  The bump to the first tag carrying e40f35d retires the workaround and the
  entry: `TestSDK_MiddlewareToolResult_GoesOutUnlabeled` fails on it and says
  what to delete.

**What**: 2026-07-28 puts `resultType` on every result, and a server
implementing it MUST send it. go-sdk v1.8.0 labels a
`tools/call`, `prompts/get` or `resources/read` result only inside its own
dispatcher, because each of those can also be `input_required`; the result
types that can only be complete embed a labeled field and are labeled after
the middleware chain returns (`setCompleteResultType`), and `CallToolResult` is
not one of them. The rate limiter's refusal of a `tools/call`
(`rateLimitedResult` in `internal/toolutil/rate_limit.go`) is a tool-error
result a middleware returns before that dispatcher runs, so at 2026-07-28 it
reached the client with no `resultType`, while a call the same server serves
carries `"complete"`, until the middleware labeled it. The tenant policy
specification recorded it as F-20, on row RTC-001, which no longer carries it:
the finding is answered, and
[issue 961](https://github.com/jmrplens/gitlab-mcp-server/issues/961) stays
open for F-21, F-22 and F-24.

**How we found it**: writing the tenant policy specification of issue 565, which
read every refusal channel against go-sdk v1.8.0. The upstream issue already
existed.

**Pinned by**: `TestSDK_MiddlewareToolResult_GoesOutUnlabeled` in
`internal/toolutil`, which drives a bare SDK server over the in-memory transport
at 2026-07-28 and asserts that a result the tool dispatcher makes carries
`"complete"` while one a receiving middleware makes carries none. It is a unit
test rather than a case in `test/e2e/http` because what it pins is the SDK
alone, on a server of the SDK's own, and the real binary no longer shows the
defect. It was run on 2026-10-01 against go-sdk at e40f35d
(`v1.8.1-0.20260914075210-e40f35d137b7`) and failed there with the message that
names this entry and what to delete, while the tests of the workaround passed.
`TestLabelForRevision_OverTheSDK_ReachesTheWireAsTheServedCallDoes` beside it
drives the workaround through the same server, and
`TestRateLimitedToolCall_EachRevision_CarriesTheResultTypeTheServedCallDoes` in
`test/e2e/http` holds the answer on the real binary: at 2026-07-28 the
limiter's refusal carries `"complete"`, as the call served before it does, and
at 2025-11-25 neither carries the field. That test failed on the code before
the workaround, with the refusal's `resultType` absent at 2026-07-28, and passed
on a binary built against e40f35d, where the SDK labels the refusal with the
same value; it replaces the pin of the absence that test module held, which
failed on 2026-09-27 against the head of
[modelcontextprotocol/go-sdk#1300](https://github.com/modelcontextprotocol/go-sdk/pull/1300).

### A Go SDK client never sees a `subscriptions/listen` refusal

- **Reported**: yes, by another user:
  [modelcontextprotocol/go-sdk#1169](https://github.com/modelcontextprotocol/go-sdk/issues/1169),
  opened on 2026-08-13, reports it from `Client.Connect`'s side, and a comment
  on it of 2026-08-25, also not ours, reproduces it on an explicit
  `ClientSession.Subscribe` whose server refuses the URI, which is this
  server's case. It needs no issue of ours.
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1170](https://github.com/modelcontextprotocol/go-sdk/pull/1170),
  open since 2026-08-13 and conflicting with `main`. Its one review, on
  2026-08-17, points out that it checks for a refusal without waiting for one,
  so over any transport slower than an in-process one the client still reports
  success; nothing has moved since. As written it observes only a refusal that
  has already arrived, which covers a server answering the listen with a
  non-2xx status and not a refusal delivered on a 200 event stream.
- **Merged**: no.
- **Blocking**: no. The refusal still takes effect: no watcher starts, and
  nothing is spent beyond what came before the refusal, which is nothing for a
  URI refused for its kind or a listen refused by a ceiling or the rate limit,
  and the one authorization read for a URI refused because that read failed.
  What the client loses is being told.
- **Workaround**: none possible. The refusal is already an in-band JSON-RPC
  error answering the listen itself, sent before the acknowledgment, which is
  the channel the specification gives it; the Go SDK client is the one that
  does not read that answer.

**What**: at 2026-07-28, `ClientSession.Subscribe` opens a
`subscriptions/listen` through `callSubscriptionsListen` (`mcp/transport.go`),
which sends the request and returns without awaiting its response, so
`Subscribe` returns nil whatever the server answers. Every refusal this server
makes on the listen path is therefore invisible to that client: the
per-credential rate limit (`-42900`); both listen-stream ceilings, both watcher
caps, a GitLab rate limit on the first read and a server shutting down
(`-32000`); an unsubscribable or unreadable URI (`-32602`); and a transient
GitLab failure on the first read (`-32603`).
`Subscribe` also records the URI as subscribed before it sends the listen, and
nothing removes the record on a refusal, so a second `Subscribe` to the same URI
returns nil and sends nothing until `Unsubscribe` is called. The same
fire-and-forget drops the listen's completion result, so that client never
learns why the server ended a subscription either. A legacy client, whose
`Subscribe` sends `resources/subscribe` and awaits it, sees every one of these
refusals, and so does any client that reads the listen's response. The tenant
policy specification records it as F-21, on rows RTC-001 and HLD-001 to
HLD-004.

**How we found it**: the issue 565 SDK probe, a go-sdk client subscribing
through a listen the server refused, which returned `err=<nil>`.

**Pinned by**: `TestListenRefusal_BeforeTheAcknowledgment_UnseenByTheModernSDKClient`
in `internal/tenancy/channels_integration_test.go`, which asserts over the three
transports that the refusal reaches the wire before any acknowledgment, that a
modern SDK client's `Subscribe` still returns nil, and that a legacy one sees
the code. The release that reports the refusal fails it.

### The Go SDK client starts no new session after a 404

- **Reported**: yes,
  [modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299),
  on 2026-09-25. It offers two shapes, the client starting a new session on its
  own with a hook for the state a new session loses, or the error staying
  terminal and documented as needing one. Either changes or documents an
  existing API, so it was filed for go-sdk's proposal process to choose.
  @guglielmo-san labelled it P2 at 09:53 UTC on 2026-09-28, its first triage.
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1300](https://github.com/modelcontextprotocol/go-sdk/pull/1300),
  opened by another contributor on 2026-09-26 on the first shape. Measured
  below, it reaches this server's 404 on the two paths where the 404 names no
  request, and not on the one where it names a call. On 2026-09-27 (22:00 and
  22:09 UTC) I backed it
  over the documentation-only shape on the issue
  ([comment](https://github.com/modelcontextprotocol/go-sdk/issues/1299#issuecomment-5860217879))
  and reviewed it
  ([review](https://github.com/modelcontextprotocol/go-sdk/pull/1300#pullrequestreview-5332347073))
  with a three-line change to `checkResponse`: a 404 on a request carrying the
  session ID wraps `ErrSessionMissing` beside the decoded error, since the
  session is gone whatever the body says. With it all three paths start a new
  session against this server, go-sdk's own `go test ./...` passes, including
  the 404 `MethodNotFound` case of `TestStreamableClientHandlerErrorPropagation`
  (which carries no session ID), and a two-case test offered with it fails
  without the change on the body that names the request's ID. A body naming
  `"id": null`, the shape the TypeScript SDK server answers an unknown session
  with, already recovers without it. The author took the change and its test
  into the pull request on 2026-09-28 (11:46 UTC), as its second commit
  (`3e21beb4`, "treat a 404 with a JSON-RPC error body as a missing
  session"); the table below was measured with the change as proposed in the
  review, and has not been run again against that commit. Read on 2026-09-29
  it conflicts with `main`, since
  [modelcontextprotocol/go-sdk#1297](https://github.com/modelcontextprotocol/go-sdk/pull/1297)
  appended to the same two `mcpgodebug` documents on 2026-09-28; its checks
  have never run, the workflows of its first head waiting for a maintainer's
  approval and its second head having none; and no maintainer has reviewed
  it. The conflict is the author's to resolve.
- **Merged**: no.
- **Blocking**: no, and only under `--stateless=false`, the one mode that mints
  session IDs. There the gate answers 404 (row ADM-007) to any request carrying
  a session ID it will not serve: one presented by a credential other than its
  owner's, and one the server no longer holds, which is where the everyday cases
  land. The server stops holding a session when it idles past
  `--session-timeout` (30 minutes by default), when its credential's pooled
  entry is evicted, when the process restarts, and on any replica that never
  opened it. An idle client's open standalone stream does not count as activity
  to go-sdk's server, and the Go SDK client pings only when its application sets
  `ClientOptions.KeepAlive`, so every long-lived Go SDK client of a stateful
  deployment meets it, and reconnecting is what recovers it.
- **Workaround**: none taken. The gate could answer a call's 404 with no
  JSON-RPC body naming the call, which would move a v1.8.0 client from the
  first outcome below to the second: the connection fails with
  `ErrSessionMissing`, the error go-sdk documents as needing a reconnect,
  instead of staying on a dead session under a message about the wrong cause.
  Row ADM-007 of the tenant policy register rules that out by declaring the
  refusal's code and text, and it would not start a new session either, since
  v1.8.0 never does. 404 itself is the answer the 2025-11-25 transport
  prescribes, and the new session is the client's to start.

**What**: 2025-11-25 says a client that receives 404 to a request carrying a
session ID MUST start a new session by sending `initialize` without one. The
gate answers exactly that 404 to a session it will not serve
(`checkSessionOwnership` in `cmd/server/auth_gate.go`), with a JSON-RPC error
body like every refusal it writes, and its comment counts on the client to make
the refusal heal itself. go-sdk v1.8.0's streamable client starts no new
session on any path. How it fails instead depends on whether the request that
met the 404 carried an id, because the gate can name only a request whose id it
can read:

- **A call** carries an id, so the body is a JSON-RPC error response naming it.
  `checkResponse` (`mcp/streamable.go`) decodes a non-2xx body before it looks
  at the status, and returns that error wrapped as a rejection that keeps the
  connection. The application is shown the gate's own words, "This session does
  not belong to the presented credential. Start a new session by sending
  initialize without a session ID.", which it is told about its own expired
  session too, since the gate has one message for every session it will not
  serve. The client keeps the dead session ID and sends every later call on it,
  to be refused the same way.
- **The standalone GET stream**, which a default client keeps open, and **a
  notification** carry no id, and neither does the gate's body then
  (`requestIDFromBody` in `cmd/server/jsonrpc_id.go` reads an id only from a
  POST body, a notification's has none, and `jsonRPCError` omits the member
  rather than send `null`). go-sdk's `DecodeMessage` refuses a message
  with neither a method nor an id, so `checkResponse` falls through to its 404
  branch and returns `ErrSessionMissing`, and the client fails the connection
  with it. When the server ends a session, the session's standalone stream ends
  with it and the client reconnects the stream, so that GET is the request that
  meets the 404: a default client whose session expires or is evicted fails
  with `ErrSessionMissing` within a couple of seconds, without making a call.
  `ClientSession.Wait` then returns an error wrapping `ErrSessionMissing`, and
  every later call fails with `ErrConnectionClosed` without being sent, the
  cause formatted into its text rather than wrapped.
  [modelcontextprotocol/go-sdk#715](https://github.com/modelcontextprotocol/go-sdk/issues/715),
  closed, exported that error rather than acting on it, so starting over is the
  application's.

Only the second path tells the application its session is gone. On the first
it sees a refusal that names the wrong cause on a connection that looks alive,
and a client that disabled the standalone stream, or makes a call in the second
or two before its stream reconnects, or holds a live session under another
credential, is on the first. The tenant policy specification records it as
F-22, on row ADM-007.

**Measured on 2026-09-27** with the tests below, against this server's binary
started with `--stateless=false`, a go-sdk client at 2025-11-25, and for the
two paths after the session ends, `--session-timeout=2s`:

| go-sdk client                                                                                                    | A call (the body names it)                         | The standalone stream after the session ends | A notification after the session ends       |
| ---------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- | -------------------------------------------- | ------------------------------------------- |
| v1.8.0                                                                                                           | the body's error; connection kept; on dead session | `ErrSessionMissing`; connection failed       | `ErrSessionMissing`; connection failed      |
| v1.8.0 with `MCPGODEBUG=noprotocolerrorbody=1`                                                                   | `ErrSessionMissing`; connection failed             | `ErrSessionMissing`; connection failed       | `ErrSessionMissing`; connection failed      |
| head of [modelcontextprotocol/go-sdk#1300](https://github.com/modelcontextprotocol/go-sdk/pull/1300) (`d3d7b33`) | the body's error; connection kept; on dead session | new session; later calls served              | new session; notification resent and served |
| head of #1300 with `MCPGODEBUG=noprotocolerrorbody=1`                                                            | new session; the call resent and served            | new session; later calls served              | new session; notification resent and served |

A call was measured both ways the gate refuses one, under another credential on
a live session and under the owner's own after the idle timeout, and the two
agree. No configuration of v1.8.0 sends a second `initialize`.

So the pull request as proposed reaches this server's 404 wherever the 404 names
no request, which covers a default client whose session expired or was evicted:
its stream meets the 404 first, and it starts a new session before its next
call. It does not reach a call, because the body is decoded first and the error
never wraps `ErrSessionMissing`, and the switch that turns the decoding off
shows the body is the only thing in the way there. One more property of #1300 is
read from its source and was not measured: a session that holds server state (a
resource subscription or a logging level) is deliberately not replaced, and the
404 stays terminal for it.

**How we found it**: the issue 565 SDK probe's session-close case, which
answered a go-sdk client from a go-sdk server whose 404 body is plain text and
so saw `ErrSessionMissing`. Driving the same client against this server's gate
showed the call path, and a review of that measurement showed that the stream
and notification paths, which carry no id, still end in `ErrSessionMissing`.

**Pinned by**: `TestSessionNotFound_GoSDKClient_ForeignCredential_KeepsTheRefusedSession`
and `TestSessionNotFound_GoSDKClient_AfterTheSessionExpires` in
`test/e2e/http`, which drive a go-sdk client against the real binary with
`--stateless=false`. The first switches the credential under a live session;
the second lets the session idle out and holds all three paths: a call is
refused on the old session ID with the gate's code and words and not
`ErrSessionMissing`, the standalone stream and a notification each fail the
connection with `ErrSessionMissing` after a GET or a POST answered 404, and in
no case is a second `initialize` sent. Each assertion fails with a message
naming this entry, the gate's comment, ADM-007 and the pages that promise a
client re-initializes. Against the head of #1300 the stream and notification
cases fail on the second `initialize` and the call cases pass, which is the
split the table records. The call cases skip under
`MCPGODEBUG=noprotocolerrorbody=1`, which turns off the decoding they are
about; the table's rows with that switch were measured with a probe of the same
three paths that was not kept.

## OpenAI Codex (`openai/codex`)

### A non-integer annotation priority breaks a tool call

- **Reported**: yes, as the symptom in
  [openai/codex#38979](https://github.com/openai/codex/issues/38979), and as its
  cause in the library Codex builds on, in
  [modelcontextprotocol/rust-sdk#1299](https://github.com/modelcontextprotocol/rust-sdk/issues/1299),
  on 2026-09-26. The Codex issue carries a comment linking both, since
  openai/codex takes no pull requests from outside.
- **In review**: yes,
  [modelcontextprotocol/rust-sdk#1300](https://github.com/modelcontextprotocol/rust-sdk/pull/1300),
  merged.
- **Merged**: **yes, in rmcp 3.5.0**. Merged on 2026-09-27 (merge commit
  `e02efbfc`), which closed
  [modelcontextprotocol/rust-sdk#1299](https://github.com/modelcontextprotocol/rust-sdk/issues/1299)
  as completed, and released in `rmcp-v3.5.0` on 2026-09-28 through the release
  pull request
  [modelcontextprotocol/rust-sdk#1296](https://github.com/modelcontextprotocol/rust-sdk/pull/1296),
  which lists it under Fixed. It reaches users through a chain of three: an rmcp
  release carrying it, which now exists, a Codex release built on that rmcp, and
  a ChatGPT.app that bundles that Codex. Codex has not taken the second step:
  `codex-rs/Cargo.toml` on its `main` still pins `rmcp = "=3.2.0"` on
  2026-09-28, and the comment owed once 3.5.0 was tagged was posted the same day
  ([openai/codex#38979, comment 5874158831](https://github.com/openai/codex/issues/38979#issuecomment-5874158831)),
  naming the release and what the bump from 3.2.0 involves. No Codex maintainer
  has commented on the issue since it was opened on 2026-08-17.
- **Blocking**: it was. Every tool call failed with "Unexpected response type",
  so the server was unusable from Codex rather than degraded.
- **Workaround**: yes, and it is load-bearing. `internal/clientcompat` detects
  Codex from `clientInfo` and rounds annotation priorities to 0 or 1, which is
  spec-legal and parseable by both. `GITLAB_MCP_CLIENT_COMPAT=off` disables it. Retire it
  only once a Codex built on an rmcp carrying the fix is widely deployed, not
  merely released: the affected build ships inside ChatGPT.app, so users do not
  choose their version. Choosing a response from `clientInfo` departs from MCP
  2026-07-28, which says it SHOULD NOT change behavior; that departure is kept
  on purpose and stated in the security concepts and client compatibility
  pages ([issue 959](https://github.com/jmrplens/gitlab-mcp-server/issues/959),
  register row `IDN-013`). It reaches only a session that knows its client:
  stdio in either protocol era, HTTP with `--stateless=false`, and any session
  at 2026-07-28, whose requests each carry `clientInfo`. A Codex client on
  2025-11-25 or earlier against the default stateless HTTP transport is sent
  the fraction, because each POST there is a session of its own that never saw
  `initialize`; `test/e2e/http` pins that limit
  ([issue 1043](https://github.com/jmrplens/gitlab-mcp-server/issues/1043)
  adds a `codex-mcp-client/` User-Agent fallback for it). The `openai-mcp`
  question is settled
  ([issue 1044](https://github.com/jmrplens/gitlab-mcp-server/issues/1044),
  measured 2026-09-28): OpenAI's hosted MCP client, reached through the
  Responses API and the Realtime API, reports `clientInfo`
  `openai-mcp (Responses API)` or `(Realtime API)` and reads
  `annotations.priority: 0.6` without error on every model tried, so it does
  not have this defect and needs no profile. It sends no `title`, and its tool
  calls carry only the User-Agent `openai-mcp/1.0.0 (...)`. One label stays
  unmeasured, a ChatGPT web tool call's `openai-mcp/1.0.0 (Codex)`, which the
  `codex` substring would match if its `clientInfo` carries the same word, so
  issue 1043 also narrows the name match to a `codex-mcp-client` prefix or a
  `title` of `Codex`.

**What**: the Codex builds bundled with ChatGPT.app reject any MCP result whose
`annotations.priority` is a non-integer float. `0.6` fails; `1` or an
audience-only annotation passes. The specification places no such restriction,
and rmcp reads the field as `Option<f32>`, which is right on its own: the defect
appears only in a build where serde_json's `arbitrary_precision` feature is on.

**Why it happens**: serde buffers a value whose type it does not know yet
(untagged and internally tagged enums, `#[serde(flatten)]`), and every JSON-RPC
message rmcp decodes takes that path. With `arbitrary_precision` on, a buffered
decimal is replayed as serde_json's private number map, a plain `f32` or `f64`
field refuses it
([serde-rs/json#721](https://github.com/serde-rs/json/issues/721)), and the
untagged result falls through to its catch-all variant, which is the "Unexpected
response type". Cargo turns the feature on for every crate in a build once any
crate enables it, and codex-cli enables it through `starlark` 0.14.2 and
`codex-exec-server-protocol`, so the same rmcp is correct alone and wrong inside
Codex. The fix reads rmcp's ten float fields through `serde_json::Number`, which
accepts both forms, with a regression test that turns the feature on.

**How we found it**: every tool call from Codex failed with "Unexpected response
type" and nothing else. Bisected with a Python fake server replaying canned
`CallToolResult` values until the float was the only variable left. This entry
first put the defect in a patched rmcp inside the bundle; reading codex-cli's
dependency tree found the feature instead
([issue 959](https://github.com/jmrplens/gitlab-mcp-server/issues/959)).

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
  three endpoints send. The record this repository commits was re-taken from
  `19.4.1-ee` on 2026-09-26 (issue 965), which carries the three corrected
  annotations, and the stale-declaration check then failed on the declaration
  below until it was removed.
- **Blocking**: no.
- **Workaround**: no longer. While the record came from `19.3.1-ee` it was a
  declaration: the 13 findings this produced against `internal/tools/groups`'
  output were answered under `documented-response-is-not-the-one-sent`, which
  is the category built for exactly this: the record was right about what
  GitLab said and wrong about what GitLab sent, because GitLab itself was wrong
  about it.

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
  @marc_shaw for the re-review. @marc_shaw approved it at 12:57 UTC, with one
  non-blocking suggestion, that no spec covered the `author == current_user`
  half of `can_cancel_auto_merge?`, and asked @egrieff for a second review;
  @uchandran approved it at 13:07 UTC. The suggestion was applied at 18:39 UTC
  in one commit, `839df5e1572f`, which specs the author path, and every thread
  was answered and resolved. That push reset @marc_shaw's approval and started
  a fork pipeline on `839df5e1572f`, whose one blocking failure was
  `cells-routes:router-in-sync` again, and not on this route: the six
  templates it reports (five under `/api/:version/orbit/` and
  `/api/:version/integrations/jira_forge/user_delegation`) are routes `master`
  added after the branch was rebuilt and the router's snapshot already
  carries, while `cancel_auto_merge` matches. It clears once the branch is
  rebased onto current `master` or a pipeline runs on the merged result. As of
  2026-09-25 it carries @uchandran's approval, and waits on @egrieff's review
  and on @marc_shaw approving again; it still needs a maintainer approval for
  each of the `/config/`, `/lib/` and `/spec/` code-owner rules. The one step
  that could come from here is that rebase, and it has not been taken, since
  the push would reset the approval it carries.

  **What happened next showed the sentence above about what clears the job to
  be wrong, both halves of it.** At 09:05 UTC on 2026-09-25 @egrieff approved
  it, which satisfies the `/config/`, `/lib/` and `/spec/` rules and leaves no
  approval to give, started the merged-results pipeline 2881689669 and set it
  to join the merge train when its checks pass. That pipeline failed
  `cells-routes:router-in-sync`, and @egrieff asked for a rebase. The branch was
  rebased onto `820afbe6` as `c9a6d805` at 23:28 UTC; the push reset
  @uchandran's approval and aborted the merge-train add, and our reply to the
  rebase request did not mention the train. The fork pipeline on `c9a6d805`
  failed the same job again, this time on four templates the HTTP Router's
  snapshot carries and the branch does not: `/api/:version/orbit/context`,
  `/api/:version/orbit/grep`,
  `/api/:version/integrations/jira_forge/user_delegation` and
  `/o/:organization_path/admin/users/invite_search`. Read against the route
  list `master` commits under `config/routing/`, at `9f1632e2` on 2026-09-27,
  all four are absent there too: they come from router refreshes of
  2026-09-24 and 2026-09-25 (`0bba3aab`, `fee6d2da` and `cfa540bd`) for GitLab
  changes `master` does not carry yet, and `cancel_auto_merge` itself matches.
  No push from here can clear the job, then, and a merged-results pipeline
  passes only once `master` has those routes.

  **Where it stands on 2026-09-27.** Approved, with @egrieff's approval
  covering every code-owner rule; one unresolved thread, the rebase request,
  which we answered; its fork pipeline red on that job; and off the merge
  train. @egrieff's non-blocking suggestion, meant for a later merge request, is
  resolved and needs nothing now. It waits on `master` gaining those routes,
  and then on a maintainer starting a merged-results pipeline and adding it to
  the train again.

  **On 2026-09-28** @egrieff resolved every thread and set it to join the
  merge train when its checks pass (10:45 UTC), and the merged-results
  pipeline failed as the paragraph above predicts: `cells-routes:router-in-sync`
  on the same four templates, which come from GitLab merge requests still open
  ([gitlab-org/gitlab!254189](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254189),
  the Orbit entity context API,
  [gitlab-org/gitlab!256404](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256404),
  the scoped Orbit grep, and
  [gitlab-org/gitlab!255555](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255555),
  the organization invite search) and from the `jira_forge` route the router
  synced in `fee6d2da`, and `rspec:check-ci-partition-pruning`, which is
  `allow_failure` and was raised from a unit shard this change does not reach.
  Our reply the same day (14:47 UTC) set that out, said a rebase cannot change
  a merged-results pipeline, and named `pipeline:skip-router-sync` as the
  documented way through if the reviewer would rather not wait. It is approved, has no
  unresolved thread, and waits on those merge requests or on that label.
  Read on 2026-09-29:
  [gitlab-org/gitlab!255555](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255555)
  merged at 08:51 UTC, so the invite search template is on `master` now;
  [gitlab-org/gitlab!254189](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254189)
  and
  [gitlab-org/gitlab!256404](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256404)
  are still open, so the check would still fail on their route templates. At
  10:02 UTC @egrieff applied `pipeline:skip-router-sync`, and at 10:04 a new
  merged-results pipeline, 2892780713, started; no thread blocks the merge,
  and it waits on that pipeline.

  **It merged on 2026-09-29.** @egrieff added it to the merge train at 11:33
  UTC and it merged at 11:36 UTC, merge commit `043d425d`, in milestone 19.5.
  The `master` pipeline on that commit, 2893091347, failed
  `permissions-verify`, and not on this change: the job runs only when a
  path it watches changes, and `lib/api/` and
  `doc/auth/tokens/fine_grained_access_tokens_rest.md` are two, so it ran
  here and not on the commit before; it reported the REST permissions valid
  and failed because `read_duo_workflow` has seven GraphQL declarations and
  three authorization tests, which this merge request does not touch. The
  broken-`master` incident opened for it,
  [gitlab-org/quality/engineering-productivity/master-broken-incidents#30549](https://gitlab.com/gitlab-org/quality/engineering-productivity/master-broken-incidents/-/issues/30549),
  was closed by gitlab-bot seven seconds later as a duplicate.

  `gitlab-org/gitlab!255704` was waiting on it: it was to be rebased onto
  `gitlab-org/gitlab!255702` once that merged, and to gain a link to the new
  section then, which is what was agreed with @marc_shaw in one of its threads.
  Read on 2026-09-27 it sat at `workflow::in dev` on head `69752099`, its
  fork pipeline green, with @uchandran's approval of the documentation half
  and the `/lib/` and `/spec/` maintainer approvals still to come, and four
  unresolved threads, GitLab Duo's style note and three of @marc_shaw's, each
  last answered by us on 2026-09-16 and so the reviewers' to resolve.

  **The rebase, pushed on 2026-09-30.** The branch was rebuilt on `master`
  at `4f19ee05` as the same six commits, head `a1dd4552`, and did what was
  agreed: the deprecation warning links to `#cancel-auto-merge` in the form
  the technical writing review suggested, and this branch's change to
  `doc/api/merge_trains.md` drops out, since `master` already points that
  link at the new section. Three edits keep each commit true on the new
  base: the first commit's message no longer says `cancel_auto_merge` has
  the contract this endpoint always described, the technical writing
  commit's message says the warning now links to the section, and the first
  commit's warning, which said the new endpoint answers `406 Not Acceptable`
  until the technical writing commit replaces that sentence, says
  `409 Conflict`, which is what the section below it on the new base
  documents. The link and `doc/api/merge_trains.md` aside, what the branch
  leaves on the page is what was reviewed before the rebase. It was pushed
  at 07:46 UTC, which reset @uchandran's approval of the documentation half,
  and at 07:47 the description was rewritten to say what the rebase did,
  @marc_shaw's thread about waiting for `gitlab-org/gitlab!255702` was
  answered inside it, and a `@gitlab-bot ready` note asked for review again,
  naming the approval the push reset and the only documentation changes
  since it was given; gitlab-bot moved it to `workflow::ready for review`
  and pinged @marc_shaw and @uchandran the same minute. It waits on their
  review, on the `/lib/` and `/spec/` maintainer approvals, and on its five
  unresolved threads, the four above and the ready note's, being resolved.

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
  the documentation, and add a new one under the current naming. The reason is
  worth recording because it applies to every future contribution of this shape:
  "we basically can't deprecate our API, by introducing another endpoint, we are
  now maintaining the old and the new". That is why the deprecation is a
  documentation notice and **not** an entry in `doc/api/rest/deprecations.md`,
  which promises removals, and why a symmetric `add_to_auto_merge` was declined
  in the same breath.
- **Merged**: half of it, and unreleased.
  [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702),
  the new `cancel_auto_merge` endpoint, merged into `master` on 2026-09-29
  (milestone 19.5, merge commit `043d425d`). Held to the tags that contain
  that commit, read on 2026-09-30, it is in no release yet; 19.5 is due on
  2026-10-15.
  [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704),
  the old endpoint's documentation and deprecation, is in review.
- **Blocking**: it was, for the action. `merge_request.cancel_auto_merge`
  answered a model with an object carrying no IID, no state and no title, so a
  caller could not tell a cancelled auto-merge from a broken call.
- **Workaround**: yes, and it is the right shape whatever GitLab does:
  `CancelAutoMerge` in `internal/tools/mergerequests/merge_requests.go` reads
  the merge request back when the answer carries no IID, which is what
  `toggleSubscription` beside it already does when a subscription toggle is
  answered 304 with an empty body. Pinned by
  `TestMRCancelAutoMerge_StatusHashIsReadBack`. The action still calls the old
  endpoint, and has to for any instance older than the release that ships
  `cancel_auto_merge`. Calling the new one on a newer instance takes a
  client-go method, which neither v3.14.0 nor v3.15.0 has, or a request of
  the handler's own, the way `invites.postInvitation` makes one.

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
  day and again on 2026-09-27, it is in no release yet; 19.5 is due on
  2026-10-15. The issue it names is still open, since its `Closes` line
  closed nothing, and nothing is owed until 19.5 is tagged. Then one comment
  on the issue saying which release carries the fix lets someone with the
  permission close it.
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

### The transfer API pages do not say the answer precedes the move, or how a failure is reported

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The transfer is accepted and applied; what misleads is the
  answer and the pages that describe it: the object in the 200 is where it
  still is, a failure after that answer reaches the caller only as a to-do
  item, and neither API page says so. The to-do API page's own `action` and
  `type` lists leave out the value that item carries.
- **Workaround**: yes. `projects.Transfer` (`internal/tools/projects/transfer.go`)
  and `groups.TransferSubGroup` (`internal/tools/groups/transfer.go`) read the
  object back through `waitpoll.Until` until it sits where the transfer put
  it, for up to `waitpoll.TransferBound` (45 seconds), and on each read that
  does not find it moved they list the caller's pending `transfer_failed`
  to-do items for the object (`waitpoll.TransferFailed`), dated against the
  transfer's own `Date` header so an item an earlier failure left is not taken
  for this one. A failure found that way ends the wait with an error; a move
  that has neither landed nor failed by the bound is answered with
  `transfer_queued` set. `TestTransfer_MoveLandsLater_AnswersTheMovedProject`,
  `TestTransfer_MoveFailsInTheBackground_AnswersTheFailure` and their group
  twins hold the wait, `TestTransferFailed_CountsOnlyAnItemOfThisTransfer`
  holds the dating, and the two e2e scenarios hold that an empty object's
  move is seen to land. `user.todo_list` offers `transfer_failed` in its
  `action` enum, read from `Todo.action_names` rather than from the API page.
  What retires the workaround is nothing on GitLab's side: the API is
  asynchronous by design since 19.4, and the entry records only that its
  reference pages do not say so.

**Where**: `lib/api/projects.rb` (`put ":id/transfer"`, `enqueue_async_transfer`)
and `lib/api/groups.rb` (`post ':id/transfer'`, `enqueue_async_transfer`),
through `Projects::TransferService#schedule_async_transfer` and
`Groups::TransferService#schedule_async_transfer`; the workers
`Projects::TransferWorker` and `Namespaces::Groups::TransferWorker` with
`Namespaces::TransferWorkerHelper`; `TodoService#transfer_failed`. The pages
are `doc/api/projects.md` ("Transfer a project to a new namespace"),
`doc/api/groups.md` ("Transfer a group") and `doc/api/todos.md` ("Get a list of
to-do items"). Read on `master` on 2026-09-27; the feature flag
`groups_and_projects_async_transfer` (introduced in 18.11, off by default) was
removed by
[gitlab-org/gitlab!250913](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/250913)
in the 19.4 milestone, so 18.11 to 19.3 behave the same way when it is enabled.

**What**: both routes run the checks of `ensure_allowed_transfer`
synchronously, move the namespace's state machine to `transfer_scheduled`,
enqueue the worker, and answer 200 with the object as it stands, before the
worker moves it. For a project that synchronous half is only the blank
namespace, the namespace it is already in and the two permission checks: the
checks that a project with the same name or path is not in the target
namespace, nor one there pending deletion, the container registry checks and
the npm package check all live in `Projects::TransferService#transfer`, which
only the worker runs. A group's `ensure_allowed_transfer` still holds its path
collision check, so a group collision is still a synchronous 400.

What a second transfer sent before the first lands does depends on whether a
worker holds the object's lease. `schedule_async_transfer` first calls
`cancel_stale_transfer_state`, which returns early only while a worker holds
`Gitlab::ExclusiveLease` for the object; otherwise it cancels the scheduled
state and schedules the new transfer, enqueueing a second worker. So a resend
while the first transfer is still waiting in Sidekiq (a backlog, or
`defer_on_database_health_signal` deferring it for a minute) is accepted and
runs as well: with the same destination, the second worker then fails with
"Project is already in this namespace." and leaves a spurious failure to-do.
Only a resend while a worker runs is refused, with 400 "Unable to initiate
transfer. The project may already have a transfer in progress." (or "The group
may already ..."). The same words answer a transfer of an object marked for
deletion, since `schedule_transfer` transitions only from `ancestor_inherited`
or `archived`. Marking the object for deletion while it is scheduled or
transferring is refused with 400 "State cannot transition via \"schedule
deletion\"".

A failure inside the worker is reported only as a to-do item for the user who
asked (`TodoService#transfer_failed`, action `transfer_failed`, target the
project with `project_id` set or the group with `group_id` set, target type
`Project` or `Namespace`), whose body is the destination path and never the
reason, and which a later successful transfer resolves. `TodoService` keeps
one pending item per user, target and action (`transfer_failed` is not in
`Todo::ACTIONS_MULTIPLE_ALLOWED`), so a second failure while the first item
is pending adds nothing, and a client cannot tell from the list that a new
failure happened.

The user documentation says transfers are asynchronous from 19.4
(`doc/user/project/working_with_projects.md`, `doc/user/group/manage.md`); the
API reference pages for the two routes still read as if the answer followed
the move, and give no example of the answer, the 400s above, or how a client
learns that the move landed or failed. `doc/api/todos.md` lists nine `action`
values and ten `type` values, while `lib/api/todos.rb` accepts
`Todo.action_names` and `TodosFinder.todo_types`: the page leaves out
`review_requested`, `review_submitted`, `ssh_key_expired`,
`ssh_key_expiring_soon` and `transfer_failed`, and on EE `okr_checkin_requested`,
`added_approver` and the four `duo_*` actions, and the types `WorkItem`, `Key`
and, on EE, `User`, `ComplianceManagement::Projects::ComplianceViolation` and
`Ai::DuoWorkflows::Workflow`. `POST /groups/:id/projects/:project_id` is not
affected by any of this: it runs `Projects::TransferService#execute` inline
and answers after the move.

**Proposal**: a docs merge request adding to both transfer sections that the
transfer is applied in the background from 19.4, that the answer shows the
object before the move, that a client confirms the move by reading the object
back (`GET /projects/:id` or `GET /groups/:id`) until its namespace or parent
changes, that a failed move is reported as a pending `transfer_failed` to-do
item for the caller (and which checks run only in the worker), and that a
resend before the first transfer lands runs a second transfer rather than
being refused; and completing the `action` and `type` lists of
`doc/api/todos.md` from `Todo.action_names` and `TodosFinder.todo_types`.

### A saved view create or subscribe from a token answers 500, and the create has already saved the view

- **Reported**: yes, by the merge request below; the fix was clear, so no
  issue was opened first.
- **In review**: yes,
  [gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074),
  opened 2026-09-28 from the community fork, now merged. The maintainer's
  first review the same day changed the fix (below), the change was pushed as
  a second commit, and the merge request waited on the reviewers again. Read
  on 2026-09-29: the
  reviewer @jannik_lehmann, who approved it at 08:49 UTC on 2026-09-28, asked
  @c_fons at 19:41 UTC that day to take the maintainer review, and @c_fons
  approved it at 10:19 UTC on 2026-09-29, which gives it both required
  approvals. @c_fons resolved the `KnownSignIn` thread @alipniagov had
  opened, which the second commit answers, and opened one proposing a reworded
  squash commit message and asking whether it reads right to us. We answered
  in that thread at 11:14 UTC that it does. Two merge checks still failed
  then: that thread, and the request for changes that came with
  @alipniagov's first review at 12:55 UTC on 2026-09-28, which GitLab still
  counted although @alipniagov left the reviewers a minute later. Between
  11:49 and 11:51 UTC @c_fons resolved every thread, bypassed that request,
  and added the merge request to the merge train. On the way there, the fork
  pipeline on `5436dc9a` passed, and the merged-results pipeline 2889991113
  was red on infrastructure only: its downstream `rspec:predictive:system-full`
  pipeline, rerun on 2026-09-29, stopped every shard before running a spec
  because the runner skipped the git checkout and `scripts/utils.sh` was not
  there to source, and the red `pajamas_adoption` is allowed to fail. The
  next merged-results pipeline, 2892863818, started at 10:27 UTC and passed.
  Nothing was retried from our side.
- **Merged**: yes, at 11:54 UTC on 2026-09-29 through the merge train (merge
  commit `ed88da02`, squash commit `563dadc2` carrying the message @c_fons
  proposed), and the merge train pipeline, 2893132892, passed. No release tag
  carries it yet; 19.5 is the first release that will.
- **Blocking**: yes, for `issue.work_item_saved_view_create` and
  `issue.work_item_saved_view_subscribe` from any client authenticated with a
  token, which is every client of this server. The other five saved view
  actions work.
- **Workaround**: partial. Nothing on the client side avoids the failure, so
  the handlers only make it readable: `workitemsavedviews.Create` answers the
  500 with a hint that the view may exist already and names
  `issue.work_item_saved_view_list` to look for it before creating it again,
  since a second create adds a duplicate, and `workitemsavedviews.Subscribe`
  answers it with a hint that nothing was recorded. The e2e lifecycle
  (`TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete`) holds
  both answers and finds the saved view through the listing, so it runs on
  every GitLab rather than skipping. What retires the workaround is the fix
  below.

**Where**: `WorkItems::SavedViews::UserSavedView.subscribe`
(`app/models/work_items/saved_views/user_saved_view.rb`), which wraps the
subscription in `with_lock` on the user it is given; it is reached from
`WorkItems::SavedViews::CreateService#auto_subscribe_creator` and from
`Mutations::WorkItems::SavedViews::Subscribe#resolve`. The user it locks is
`current_user`, on which `User#update_tracked_fields!` (`app/models/user.rb`)
has set `sign_in_count`, `current_sign_in_at` and `last_sign_in_at` and saved
them only inside `Gitlab::ExclusiveLease.throttle(id)`, whose period is an
hour. Read in the `gitlab/gitlab-ee:19.4.1` image (revision `26212baacad`) on
2026-09-28; the create mutation is an experiment since 18.7.

**What**: a GraphQL request authenticated with a token signs the user in
without a session (`SessionlessAuthentication#sessionless_sign_in`), and
Devise's trackable hook calls `update_tracked_fields!` on it. That method
assigns the tracked fields on every such request and writes them at most once
an hour, so on every request but the first in the hour `current_user` carries
unsaved changes to those three attributes. Rails 7.2 refuses to lock a record
with unsaved changes (`ActiveRecord::Locking::Pessimistic#lock!`: "Locking a
record with unpersisted changes is not supported"), so `with_lock` raises and
the mutation answers `500 Internal server error`. `workItemSavedViewSubscribe`
changes nothing. `workItemSavedViewCreate` has already saved the view
(`saved_view.save` runs before `auto_subscribe_creator`, in no transaction
with it), so the view exists, its creator is not subscribed to it, and a
client told only that the create failed creates it again. Reproduced on the
licensed complete run of the wave 1 stack: three creates, three 500s, three
`saved_views` rows and no `user_saved_views` row, and a subscribe to one of
those views answered 500 twice in a row. The exception log carries the
`RuntimeError` with the backtrace through `user_saved_view.rb:25`. The same
class of failure was fixed in other places before (issues
[gitlab-org/gitlab#384337](https://gitlab.com/gitlab-org/gitlab/-/issues/384337)
and [gitlab-org/gitlab#419343](https://gitlab.com/gitlab-org/gitlab/-/issues/419343)).

**The same lock breaks a second caller.** `MergeRequests::SavedViews::CreateService#persist`
saves the view inside `current_user.with_lock`, so `mergeRequestSavedViewCreate`
(behind the `mr_dashboard_saved_views` flag) answers the same 500 for the same
reason; it fails before the save and leaves nothing behind. This server does
not expose it.

**Fix merged**: [gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074)
fixes it where the state is left rather than in either caller:
`update_tracked_fields!` clears the dirty state of the tracked attributes after
the throttled block (`clear_attribute_changes` over
`Devise::Models::Trackable.required_fields`), which is a no-op when the write
ran, and when the throttle skipped it leaves this sign-in's values on the record
without a change a later `save` would write, so any later
`current_user.with_lock` works. The first version restored the row values
instead (`restore_attributes`), and the maintainer's review showed why that was
wrong: `KnownSignIn` reads this sign-in's values from `current_user` after the
hook (`last_sign_in_ip` as the previous sign-in's IP, `current_sign_in_at` as
this sign-in's time), so a throttled sign-in would have sent the new-location
email with the previous time and could have treated the previous IP as unseen.
The title changed with it, to "Clear the dirty state of tracked sign-in fields
the throttle did not write". Measured on the branch: the model examples of
`spec/models/user_spec.rb` for `#update_tracked_fields!` pass with the change
and the three about the unsaved state fail without it, the lockable one with the
`RuntimeError` above; the reviewer's `when the sign-in write is throttled`
context, added to the `known sign in` shared examples, gives 12 examples that
fail with `restore_attributes` and pass with `clear_attribute_changes`. On
`gitlab/gitlab-ee:19.4.1-ee.0` with the change copied into the running container
and Puma restarted, both mutations went from 500 to 200, the work item view
created subscribed. Not in the merge request:
`WorkItems::SavedViews::CreateService#execute` still saves the view and
subscribes its creator in two steps, so a failure of the second for any other
reason would still leave a view nobody follows.

### The Orbit API page's query examples predate version 12 of the query DSL

- **Reported**: yes, by the merge request below; a documentation-only change
  needs no issue first.
- **In review**: yes,
  [gitlab-org/gitlab!258241](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258241),
  opened 2026-09-28 from the community fork and readied naming the technical
  writer @fneill, whom the bot set as reviewer.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: not yet. This server never points a model at the page's
  examples, and `orbit.query`'s own guidance moves to the version 12 shape
  with [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031).

**Where**: the *Query examples* section of `doc/api/orbit.md` (master
`c2769a65`, lines 265 to 468, unchanged since `187cd4b6`, blob `e86660fd`).

**What**: the five examples were written for the query DSL of March 2026, and
GitLab.com now serves version 12 (`graph_query/v12`, 12.1.9, Orbit 0.131.0,
read 2026-09-28). Sent verbatim, four are refused with `400 compile_error`: a
`search` query type, a top-level `node`, the old aggregation keys with no
bounded node, `neighbors.node`, and a path query without `rel_types`. The fifth
is accepted, but its documented response is not the shape the API returns,
whose `result` carries `nodes`, `edges` and `pagination`. The path example
also asks for a path between two projects, which no forward chain of
relationships joins, and Orbit's own query corpus expects that query to be
empty
([`fixtures/queries/corpus/sdlc.yaml`, case `q25`](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/blob/1c43d9f0e984be5ebd6825bf477773953b788a3b/fixtures/queries/corpus/sdlc.yaml#L743-770)).
[gitlab-org/gitlab!254743](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254743)
kept the section on purpose, as the only worked examples of the DSL in the
repository.

**Fix in review**: the merge request rewrites the five examples in version 12
and every response in the shape the default `raw` format returns, and adds the
few sentences the examples depend on: what `result` holds, that traversal and
aggregation need a bounded node, that path finding needs `rel_types` and
follows each relationship in its defined direction, and that `neighbors`
defaults to `outgoing`. Each new request body answered `200` on GitLab.com
verbatim; `markdownlint-cli2` and Vale with the repository's configuration
report nothing new.

**How we found it**: checking every published example against GitLab.com
while moving `orbit.query` to version 12 of the DSL for issue 1031.

### An unknown severity on a pipeline's findings list answers 500

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. A severity GitLab knows works; one it does not know fails
  the whole request with nothing saying which value was wrong.
- **Workaround**: yes. `securityfindings.List`
  (`internal/tools/securityfindings/security_findings.go`) refuses a severity
  outside `findingSeverities` before it sends anything, naming the values it
  takes, and the package's tests hold that list to `VulnerabilitySeverity`,
  the enum the finding's own `severity` is typed with in the pinned schema
  (`TestList_UnknownFilterValue_RefusedBeforeTheRequest`,
  `TestFilters_HeldToThePinnedSchema`). What retires it is GitLab validating
  the argument itself, below; the local list then only duplicates the
  schema's.

**Where**: the `severity` argument of `Pipeline.securityReportFindings`
(`ee/app/graphql/resolvers/pipeline_security_report_findings_resolver.rb`),
typed `[GraphQL::Types::String]`, reaches `Security::FindingsFinder#severities`
(`ee/app/finders/security/findings_finder.rb`), which reads it with
`Security::Finding.severities.fetch_values(*params[:severity])`. Read at
19.5.0-pre (`5041f73d695`) on 2026-09-28.

**What**: the schema cannot refuse a value, since the argument is a plain
string, and `fetch_values` raises `KeyError` for a key the enum does not have,
which reaches the caller as `500 Internal server error` naming neither the
argument nor the value. The value is looked up as written, so the lowercase
severities the enum is keyed by work and an uppercase one would not; this
server sends them lowercased, which is why the handler judges them in any case.

**Proposal**: type the argument as `[Types::VulnerabilitySeverityEnum]`, which
is what the finding's own `severity` field is, so GraphQL refuses an unknown
value with an argument error naming it; failing that, validate it in the
resolver before the finder runs.

### An unknown report type on a pipeline's findings list is dropped and filters out every finding

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The request succeeds, which is the problem: the answer is
  an empty page that reads as a pipeline whose scans found nothing.
- **Workaround**: yes. `securityfindings.List` refuses a report type outside
  `findingReportTypes` before it sends anything, naming the values it takes,
  and the package's tests hold that list to `VulnerabilityReportType` less the
  two values `Security::Scan` has no scan type for (`CONTAINER_SCANNING_FOR_REGISTRY`
  and `GENERIC`). `SARIF` is the gap the workaround cannot close: `Security::Scan`
  gained `sarif` at 18.11 while the action needs 18.5, so on 18.5 to 18.10 the
  value passes the check and GitLab drops it, and the input's description says
  so. What retires the workaround is GitLab validating the argument itself.

**Where**: the `report_type` argument of `Pipeline.securityReportFindings`
(`ee/app/graphql/resolvers/pipeline_security_report_findings_resolver.rb`),
typed `[GraphQL::Types::String]`, reaches `Security::FindingsFinder#by_report_types`
(`ee/app/finders/security/findings_finder.rb`), which merges
`Security::Scan.by_scan_types(params[:report_type])`, and that scope filters on
`Security::Scan.sanitize_scan_types` (`ee/app/models/security/scan.rb`):
`scan_types.keys & Array(given_types).map(&:to_s)`. Read at 19.5.0-pre
(`5041f73d695`) on 2026-09-28, and on `18-10-stable-ee` and `18-11-stable-ee`
for `sarif`.

**What**: a value `Security::Scan` has no scan type for is removed by the
intersection without a word, and a filter left empty matches no scan, so the
answer is an empty list rather than an error. A misspelled report type, one
the vulnerability list takes and a pipeline scan never records, and one a
newer GitLab added all read the same way, as a clean pipeline.

**Proposal**: type the argument with an enum of the scan types
`Security::Scan` has, or refuse in the finder a value `sanitize_scan_types`
would drop, so the caller is told which value GitLab does not know.

### The scan profile attach mutation drops the reason it refused a name

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. A name GitLab builds a default profile for attaches; one
  it does not fails with an error that says the resource does not exist or
  the caller may not act on it, which reads as a permission problem.
- **Workaround**: yes. `securityscanprofiles.Attach`
  (`internal/tools/securityscanprofiles/security_scan_profiles.go`) answers a
  failed attach with a hint naming the default profile names attach takes
  (`DefaultProfileNames`), the GitLab release each needs
  (`DefaultProfileFloors`) and the scan types refused by name
  (`RefusedByName`), and the input's description, the usage line and the meta
  group description say the same before the call. The package's tests hold
  the names to `SecurityScanProfileType` in the pinned schema, and the EE
  scenario attaches by `container_scanning` and asserts the hint. What
  retires the hint's guesswork is GitLab passing its own message on.

**Where**: `Mutations::Security::ScanProfiles::Attach#resolve_profile!`
(`ee/app/graphql/mutations/security/scan_profiles/attach.rb`) calls
`Security::ScanProfiles::FindOrCreateService.execute`
(`ee/app/services/security/scan_profiles/find_or_create_service.rb`) and, when
the result is not a success, calls `raise_resource_not_available_error!`. Read
at 19.5.0-pre (`5041f73d695`) on 2026-09-28 and on every stable branch from
18-7 to 19-4.

**What**: the service answers a name that is not a preset key of
`Security::DefaultScanProfilesHelper.default_scan_profiles` with
"Could not find a default scan profile for this type" (and a namespace that is
not a root with "Namespace must be a root namespace"), and the mutation throws
that message away for the generic "The resource that you are attempting to
access does not exist or you don't have permission to perform this action".
`container_scanning` and `business_logic` are values of
`SecurityScanProfileType` no release builds a default profile for, and the
bare `triage_and_remediation` names none of the three presets its profiles are
keyed by, so all three are refused this way, as is any name on an instance
older than the release that added it.

**Proposal**: return the service's message, as an argument error or in the
payload's `errors`, the way the mutation already returns the attach
service's own errors.

### The admin token route takes no granular scopes, and no client-go create option carries them

- **Reported**: the GitLab half yes, by GitLab itself,
  [gitlab-org/gitlab#630541](https://gitlab.com/gitlab-org/gitlab/-/issues/630541)
  ("Support granular scopes when admins create a personal access token for a
  user via REST"), opened on 2026-09-23 by @alexbuijs of
  `group::authorization`. Read on 2026-09-30 it is unassigned, has no
  milestone, carries `automation:quick-win-judged`, and has no comment. The
  client-go half no.
- **In review**: no. Two attempts by other contributors were closed unmerged.
  [gitlab-org/gitlab!245585](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585)
  ("Add granular scope support to admin create-token-for-user endpoint", by
  @abime, reviewed by @alexbuijs, 575 changed lines in 9 files) was closed by
  its author on 2026-08-24. Its first review round, on the audit event it
  emitted when the privilege escalation check refuses a grant (a spec for it,
  and a layer shared with the GraphQL mutation), was answered "Done" in commit
  `c08e1a12`; the second, of 2026-08-21, was not: three user lookups for one
  request
  ([note 3715838735](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585#note_3715838735)),
  an unrelated change to the page's history
  ([note 3715838757](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585#note_3715838757)),
  and a missing link to the fine-grained permissions reference
  ([note 3715838766](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585#note_3715838766)).
  [gitlab-org/api/client-go!2978](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2978)
  (`feat(personal_access_tokens): support granular scopes`, by @jchorl) added
  `GranularScopes` to the impersonation and current-user create options and the
  granular fields to the token struct; its one maintainer request, a couple of
  integration tests, went unanswered, and it was closed with the `stale` label
  on 2026-09-24.
- **Merged**: no.
- **Blocking**: no. This server creates no fine-grained token, and the plans of
  [issue 952](https://github.com/jmrplens/gitlab-mcp-server/issues/952) need
  none created through it.
- **Workaround**: none taken. All five actions that create a personal access
  token send classic `scopes` only and refuse a call without them:
  `user.create_personal_access_token` and `user.create_impersonation_token`
  (`internal/tools/impersonationtokens/impersonation_tokens.go`),
  `user.create_current_user_pat`
  (`internal/tools/users/user_service_accounts.go`),
  `group.service_account_pat_create` and `project.service_account_pat_create`.
  The current-user action can therefore create only a `k8s_proxy` or
  `self_rotate` token, the two classic scopes GitLab accepts on that route.
  `user.create_impersonation_token` also takes no `description`
  (`impersonation_tokens.go:88-93`), because client-go's options have none
  (below). Both gaps on this side are tracked in
  [issue 1115](https://github.com/jmrplens/gitlab-mcp-server/issues/1115),
  and retire with the client-go merge request below for the routes GitLab
  serves them on.

**Where**: GitLab v19.4.1-ee (`26212baa`), `lib/api/users.rb`. The admin
route, `POST /users/:user_id/personal_access_tokens` (`resource
:personal_access_tokens` at line 1291, `before { authenticated_as_admin! }` at
1298), declares `requires :scopes` (1308) and no granular parameter. The
impersonation route, `POST /users/:user_id/impersonation_tokens`, has `use
:granular_scope_params` and `mutually_exclusive :scopes, :granular_scopes`
(1227-1228); the current-user route, `POST /user/personal_access_tokens`, has
`use :granular_scope_params` and `exactly_one_of :scopes, :granular_scopes`
(1851, 1858). Each of the two answers 404 when
`granular_personal_access_tokens` is off for the user the token is for (1234,
1864). The service account token routes declare `requires :scopes` and no
granular parameter either (`lib/api/group_service_accounts.rb:231`,
`lib/api/project_service_accounts.rb:229`), which is
[gitlab-org/gitlab#616442](https://gitlab.com/gitlab-org/gitlab/-/issues/616442),
open since 2026-08-17: under a group's enforcement on GitLab.com its service
accounts are left with no working token. That one is GitLab's own and outside
the merge request below.

**What**:

- `doc/api/user_tokens.md` documents `granular_scopes` for none of the three
  routes (their sections open at lines 17, 77 and 266 of the 19.4.1 page), and
  still lists `scopes` as required for the impersonation route. client-go
  "only supports what is in the public API docs" (`CONTRIBUTING.md:20-21`), so
  GitLab's documentation is what makes the client-go half admissible.
- In client-go v3.15.0 (`be72a3fe`), `CreateImpersonationTokenOptions`
  (`users.go:1224`), `CreatePersonalAccessTokenOptions` (`:1253`) and
  `CreatePersonalAccessTokenForCurrentUserOptions` (`:1274`) carry `Scopes`
  and nothing granular, so a client-go caller cannot create a fine-grained
  token through any of the three routes, the two GitLab already serves
  included.
- The feature is not experimental.
  `config/feature_flags/beta/granular_personal_access_tokens.yml` is
  `default_enabled: true` (type `beta`, milestone 18.7), and
  `doc/auth/tokens/fine_grained_access_tokens.md` records general availability
  in 19.2 on every tier, so the client-go maintainers' reluctance about
  experimental endpoints does not apply.
- Found while reading for this entry: `CreateImpersonationTokenOptions` also
  has no `Description`, which the route accepts (`lib/api/users.rb:1224`) and
  the page documents under "Create an impersonation token". It is recorded
  nowhere else here, and can travel in the same client-go merge request.

**Who is waiting on it**:
[gitlab-org/terraform-provider-gitlab#6868](https://gitlab.com/gitlab-org/terraform-provider-gitlab/-/issues/6868)
asks for fine-grained tokens on the provider's personal access token resource,
which creates a token for a user through the admin route. Heidi Berry, a
client-go code owner (`.gitlab/CODEOWNERS:1`), set it to `workflow::blocked`,
"hopefully can be worked on once the admin endpoint has been updated"
([note 3603251022](https://gitlab.com/gitlab-org/terraform-provider-gitlab/-/issues/6868#note_3603251022)),
and on 2026-09-12 wrote that "it looks like the MR stalled as the assignee is
moving to a different team" and asked @jpr0c "do you know if there are any
plans to continue the work" on the merge request above
([note 3825448075](https://gitlab.com/gitlab-org/terraform-provider-gitlab/-/issues/6868#note_3825448075)).
Nobody had answered on 2026-09-30.

**Row 32 is the other half.**
[Row 32](#no-token-struct-carries-the-granular-fields-and-the-impersonation-and-resource-ones-carry-less-still)
is the response side and is not repeated here: commit 24 of
[gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
(`fff671b1`) gives `PersonalAccessToken` the fields `Granular`,
`GranularScopes` and `LastUsedIPs`, with a new type,
`PersonalAccessTokenGranularScope`, for one scope a token holds. This entry is
the request side. The option type the client-go merge request adds for one
scope to grant (the earlier attempt called it `GranularScopeOptions`, with
typed constants for `access`) is named to sit beside that response type, whose
`project_id` and `group_id` are not the request's names, and if the joint merge
request has not merged by then, the description says which of the two lands
first.

**The order, and what each step waits on**:

1. **The GitLab merge request** for the issue, from the
   [community fork](https://gitlab.com/gitlab-community/gitlab-org/gitlab).
   Nothing in client-go's review queue holds it. It waits on the maintainer's
   approval, an independent review of every text before it goes out, and one
   decision taken first: whether to say on the issue that we are taking it,
   since it is unassigned and the earlier merge request was closed by its own
   author. The note is drafted at the end of this section.
2. **The client-go merge request**, once the GitLab change is merged,
   documented and in a release the client-go integration fixture pulls (it
   runs `gitlab-ee:latest`, `docker-compose.yml:5` and `:29`), and only after
   this project's three open client-go merge requests, which the paragraph
   under the summary table lists, have been reviewed. It needs no issue first.
   `CONTRIBUTING.md:10-12` exempts a merge request that describes the problem
   it solves, and adding fields for documented parameters is what the
   maintainers asked to receive on demand once the joint merge request is in
   ("then go back to having people submit fields on-demand",
   [gitlab-org/api/client-go#2300 note 3811110919](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300#note_3811110919)).
   Row 84 files an issue first because it adds a library-level function
   whose shape is a choice. The merge request links the GitLab issue and the
   provider issue.
3. Both descriptions say plainly that the current-user and impersonation
   fields alone do not unblock the provider; the admin route does. The
   provider's own change is not ours to plan and was not researched.

**What the GitLab merge request carries**:

- In `lib/api/users.rb`, the admin block (1291-1323): `use
  :granular_scope_params`, `scopes` made optional with `exactly_one_of
  :scopes, :granular_scopes`, a granular branch following the impersonation
  one (1232-1241) that looks the target user up once (the block's
  `target_user` helper calls `find_user_by_id(params)` each time, which the
  first open comment counted three times in one request), and a 404 when the
  flag is off for the target user. A request that passes `scopes` works as it
  does today.
- In `doc/api/user_tokens.md`, `granular_scopes` for the admin, current-user
  and impersonation create routes, with a link to the fine-grained permissions
  reference, `scopes` marked optional where it is, and no unrelated history
  edits.
- Request specs in `spec/requests/api/users_spec.rb` in the shape the earlier
  merge request had: every access type, a privilege escalation refused, the
  flag off, legacy tokens unchanged. The OpenAPI document regenerated, and a
  changelog trailer.
- Not by default: the audit event the earlier merge request emitted on a
  refused escalation. 19.4.1 emits none there (`create_granular_token`,
  `lib/api/helpers/personal_access_tokens_helpers.rb:136-147`, and nothing in
  `app/services/authz/tokens/privilege_escalation_check.rb` audits), and it
  goes beyond the issue's proposal, so it is carried only if the issue's
  author wants it, in the layer shared with the GraphQL mutation that the
  review asked for
  ([note 3694332559](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585#note_3694332559)).
- A description that credits the earlier merge request and its author,
  answers its three open comments one by one, links the issue and the provider
  issue, keeps its long parts in collapsed sections, and says it comes from
  maintaining this server, linking this register as the record. The workflow
  is the
  [upstream contribution skill](../../.github/skills/upstream-contribution/).

**What the client-go merge request carries**:

- `GranularScopes` on `CreatePersonalAccessTokenOptions`,
  `CreateImpersonationTokenOptions` and
  `CreatePersonalAccessTokenForCurrentUserOptions`, with the option type for
  one scope as GitLab's request declares it: `access`, `permissions`,
  `project_ids` and `group_ids`, the last two only with `selected_memberships`
  (`lib/api/helpers/personal_access_tokens_helpers.rb:40-50`). Each field's
  comment says how it combines with `Scopes` on that route. And `Description`
  on the impersonation options.
- Unit tests in the package's style, and the integration tests the maintainer
  asked for on the earlier attempt: a fine-grained token created through each
  route against the Docker instance and, once row 32's fields have landed,
  read back to show its scopes.
- A description crediting the earlier client-go merge request and its author,
  and linking the GitLab merge request.

**What the maintainers have said that decides this shape**:

- Patrick Rice, on the earlier client-go attempt: "Since the documentation for
  these APIs doesn't appear to be updated and tokens are a fairly commonly
  used API function, can you create a couple integration tests showing that
  the updates work properly?"
  ([gitlab-org/api/client-go!2978 note 3664553320](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2978#note_3664553320)).
  Hence GitLab's documentation first, and integration tests in client-go.
- Patrick Rice, on 2026-09-09: "with so many coming in to a team of 3
  maintainers, it puts a bit of a burden on us to validate each MR
  individually, where if we had an issue we could approve ahead of time in
  chunks we just have to validate that the MR matches the issue we
  pre-approved"
  ([gitlab-org/api/client-go!3053 note 3810461065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3053#note_3810461065)).
  Hence nothing new in client-go's queue until the three open merge requests
  are reviewed.

**What it costs this server**: the gaps the Workaround field names. What none
of rows 82 to 85 gives issue 952, and the two calls that read a fine-grained
token's own grant, are in
[row 83's section](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose).

**Before posting**: re-read the admin route and the page on `master` (checked
on 2026-09-30 at `3421b9a9`: the route still `requires :scopes`, and
`doc/api/user_tokens.md` still says nothing of `granular_scopes`), and read
the issue's notes again; decide whether to announce on the issue first.

**Verified, inferred and unverified**:

- Verified at the two pins, on `master` where noted, and on gitlab.com with GET
  requests on 2026-09-30: every route, parameter, line, flag and documentation
  fact above, the state of the three issues and both closed merge requests,
  the size of the GitLab one (575 changed lines, 478 added and 97 removed, in 9
  files), and every quote.
- Inferred: that the provider needs only the admin route and client-go's
  option to close its issue.
- Unverified: whether @jpr0c or `group::authorization` plans to pick the issue
  up.

**Effort**: the GitLab merge request is medium, in authentication code owned by
`group::authorization`, with a changelog entry; the client-go one small to
medium.

<details>
<summary>Draft: a note on the GitLab issue, if we decide to announce</summary>

Written for this entry and reviewed with it; not posted. Native references,
first person as the maintainer, one paragraph per line.

```markdown
I would like to take this, following the proposal above and the approach of !245585, whose three open review comments (one user lookup per request, no unrelated history change in the docs, a link to the fine-grained permissions reference) I would address. I would document `granular_scopes` for the impersonation route, as the proposal says, and for the current-user route, which is undocumented too; the Go client only adds a parameter the public API docs describe, and gitlab-org/terraform-provider-gitlab#6868 is waiting on the admin route. @alexbuijs, is anyone on the team already picking it up?
```

</details>

### The fine-grained refusal names the missing permissions only as display labels in prose

- **Reported**: no. The issue is drafted below and goes before row 84's, which
  cites it. It needs nothing from client-go, and waits only on the
  maintainer's approval and an independent review of its text.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The call is refused correctly; what is missing is a value
  a program can act on.
- **Workaround**: none taken. This server passes GitLab's sentence on as it
  passes any GitLab message, and reads nothing out of it. Turning a label back
  into the identifier a token is created with needs a table taken from
  GitLab's permission YAML, which is the coupling this entry asks GitLab to
  make unnecessary.

**Where**: GitLab v19.4.1-ee (`26212baa`). `authorize_granular_token_scopes!`
(`lib/api/helpers.rb:1339-1354`) runs `AuthorizeGranularScopesService`,
records the denied raw permissions with
`Current.add_granular_denied_permissions` (1350), and raises
`Gitlab::Auth::GranularPermissionsError` with the message alone (1352). What
reads the recorded permissions is the `use_pat` tracking event
(`lib/api/track_api_request_from_personal_access_token.rb:23-29`), and only
where `track_api_request_from_personal_access_token` is enabled for the user
(`:15`): a GitLab.com derisk flag, `default_enabled: false` at 19.4.1 and on
`master`, whose rollout issue
[gitlab-org/gitlab#596560](https://gitlab.com/gitlab-org/gitlab/-/issues/596560)
is open and labelled as missed in every milestone from 19.0 to 19.4. The error
class carries nothing but the message (`lib/gitlab/auth/auth_finders.rb:13`),
and the API guard renders it as `Bearer::Forbidden` with the code
`insufficient_granular_scope` and no `scope` (`lib/api/api_guard.rb:235-238`).

**What**: a fine-grained token that lacks a permission gets

```text
403 {"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following project permissions: [Work Item: Read]."}
```

`access_denied_error`
(`app/services/authz/tokens/authorize_granular_scopes_service.rb:127-138`)
builds the description from display labels, each `resource_name: Action`,
sorted and joined. The identifiers a token is created with, the values
`granular_scopes` takes in its `permissions` (such as `read_work_item`), appear
nowhere in the response. The classic refusal a few lines above in the same
guard has a machine-readable part: `insufficient_scope` passes `{ scope:
e.scopes }` (`api_guard.rb:217-223`) and its body carries a `scope`, a shape
the REST authentication page documents (`doc/api/rest/authentication.md`,
lines 200 to 205). That branch's missing `WWW-Authenticate` header is
[row 1](#403-responses-carry-no-www-authenticate-header), a change of its own.

Four properties any such field has to be honest about, read in the same
service:

- The check passes when any one boundary holds every permission (`:85-87`),
  and the sentence names only the first boundary, by priority, that lacks one
  (`:128`). A route that accepts a group or the user, such as
  `POST /import/bitbucket` (`lib/api/import_bitbucket.rb:34-35`), is refused
  with one of them named.
- The label is GitLab's reported denial, not a guaranteed grant. In
  [gitlab-org/gitlab#627693](https://gitlab.com/gitlab-org/gitlab/-/issues/627693)
  a token holding Pipeline: Read is refused with `[Pipeline: Read]` because no
  policy rule enables `read_pipeline_job`; and for sixteen raw permissions the
  label names a deprecated definition,
  [row 85](#the-fine-grained-refusal-can-name-a-deprecated-permissions-label).
- A token whose boundary is hidden from its user gets a 404 instead
  (`helpers.rb:1348`), a plain `{"message":"404 Not Found"}` (`not_found!`,
  `helpers.rb:600-605`), the same bytes as a missing resource. That is
  deliberate and nothing here changes it. At 19.4.1 the service asks, for each
  boundary in priority order, whether the token's user is a member of it or
  can see it (`Authz::BoundaryPolicy`,
  `app/policies/authz/boundary_policy.rb:16-26`; what the token grants plays no
  part); on `master` it asks only about the boundary the sentence would name,
  through that project's or group's own policy for the token's user.
- The code does not always mean a permission is missing.
  `insufficient_granular_scope` also carries "Access denied: Fine-grained
  personal access tokens are not yet supported." and "Access denied: This
  operation doesn't support fine-grained personal access tokens." (service
  `:115-121`, raised through `helpers.rb:1346-1352`). A field naming the
  missing permissions would be absent there, and a client has to read its
  absence as "no permission fixes this", which is what phase A of issue 952
  has to tell a model.

**Why a field of its own, not a change to the sentence**: GitLab made the
sentence human-readable on purpose.
[gitlab-org/gitlab#591681](https://gitlab.com/gitlab-org/gitlab/-/issues/591681)
replaced raw names such as `[read_project]` with the `[Resource: Action]`
form: @alexbuijs wrote that with the old message "users are not able to search
for `read_project` anywhere"
([note 3119035543](https://gitlab.com/gitlab-org/gitlab/-/issues/591681#note_3119035543)),
and the issue's author proposed removing the raw permission and structuring
the required permissions "as Resource: Permission(s)"
([note 3124860631](https://gitlab.com/gitlab-org/gitlab/-/issues/591681#note_3124860631)).
A proposal that put identifiers back into the sentence would reverse that; one
that leaves the sentence alone and adds a field for programs does not. The
names the field carries are the identifiers `granular_scopes[].permissions`
takes and a token's own grant lists. For 718 of the 786 live definitions that
identifier is also a raw permission name (`read_project` among them, from
`projects/project/read.yml`), so the case rests on the field being separate
from the sentence and on each name being one a caller can grant, not on the
strings differing.

**What is proposed** (the draft below):

1. `GranularPermissionsError` carries the missing permissions, named by
   assignable identifier with `Assignable.available_for_permission`
   (`lib/authz/permission_groups/assignable.rb:21-23`), so the names are
   current ones. At 19.4.1 each of the 1251 raw permissions the 857
   assignable definitions declare sits in exactly one live definition, and
   GitLab's own permission checks make that hold for a REST route by
   construction: a route permission with no live definition is a
   `missing_assignable` violation
   (`lib/tasks/gitlab/permissions/routes/validate_task.rb:187-195`), and a raw
   permission in two live definitions a `duplicate_raw_permission` one
   (`lib/tasks/gitlab/permissions/assignable/validate_task.rb:309-318`). The
   mapping therefore always has one answer.
2. The guard passes them as `{ scope: identifiers }`, which rack-oauth2
   2.2.1's `Forbidden` already renders space-joined
   (`lib/rack/oauth2/server/resource/error.rb:26-37` in the gem). The
   description does not change.
3. The example at `doc/auth/tokens/fine_grained_access_tokens.md:126-131` is
   updated, and specs cover both.

The body would become

```text
{"error":"insufficient_granular_scope","error_description":"Access denied: ... [Work Item: Read].","scope":"read_work_item"}
```

and the field would be absent from the two refusals above that name no
permission. Two points are left to the group, and the draft asks them: in the
classic refusal `scope` lists scopes of which any one is accepted, class-wide
(`api_guard.rb:102-114`), while here it would list permissions all missing on
one boundary, so they may prefer a dedicated key through a small `Forbidden`
subclass; and only a dedicated key could list each boundary's missing set as
an alternative. Out of scope, and said so: permission names a token cannot be
granted, the deliberate 404, and GraphQL, where a mutation's errors could
follow with `extensions` and a query field redacts to null, which
[issue 1103](https://github.com/jmrplens/gitlab-mcp-server/issues/1103)
investigates from this side.

**What the group has said that decides this shape**: besides the issue above,
@alexbuijs, asking a user how GitLab could help migrate legacy tokens to
fine-grained ones, pointed at the refusals themselves: "I don't think it's
possible to avoid the whack-a-mole game from our side. Your script could be
adjusted to not exit when an API call fails. That would allow you to see all
error messages with missing permissions"
([gitlab-org/gitlab#553887 note 3109306972](https://gitlab.com/gitlab-org/gitlab/-/issues/553887#note_3109306972)).
The user answered that "setting up my token is basically guessing what
permissions map to what I think my program does and just following the
errors"
([note 3109707685](https://gitlab.com/gitlab-org/gitlab/-/issues/553887#note_3109707685)).
If the refusals are how a caller finds its permissions, a program should be
able to read them. The open
[gitlab-org/gitlab#629849](https://gitlab.com/gitlab-org/gitlab/-/issues/629849),
from the same group, is the token's side of the same need: that a
fine-grained token can read its own grant, `granular_scopes` included, in one
request and without broader access, leaving open whether that means no
`read_personal_access_token` or a dedicated permission. No GitLab issue
proposed the field when this was written.

**What it costs this server.** Issue 952 needs the permission an action
requires **before** the call, so that a fine-grained token is served the
actions its grant allows (its phase B), and none of rows 82 to 85 provides
that. GitLab sends nothing about the permission a request used when it
succeeds (`helpers.rb:1339-1354` computes it from the route's `route_setting
:authorization` and discards it on a pass), and a refusal names what was
missing only after the call. Phase B takes the route-to-permission mapping
from GitLab: each route's `route_setting :authorization`, which
`cmd/gen_api_live` does not record yet and could record from a booted GitLab,
or GitLab's generated reference page
(`doc/auth/tokens/fine_grained_access_tokens_rest.md`). It cannot come from
client-go, whose maintainers leave GitLab facts about a method, such as the
edition, to its consumers
([gitlab-org/api/client-go!2931 note 3504443036](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2931#note_3504443036)).
The fact that helps today is on the token's side: a fine-grained token reads
its own grant in two calls. `GET /personal_access_tokens/self`
(`GetSinglePersonalAccessToken`, which `DetectScopes` in
`internal/gitlab/scopes.go` already calls) gives its id and `granular: true`
but no scopes, because it does not pass `with_granular_scopes`
(`lib/api/personal_access_tokens/self_information.rb:49-51`) and the entity
exposes the scopes only under that option
(`lib/api/entities/personal_access_token.rb:25-27`). `GET
/personal_access_tokens/:id` (`GetSinglePersonalAccessTokenByID`) passes it
and presents them (`lib/api/personal_access_tokens.rb:61-70`). Both routes
need the `read_personal_access_token` permission on the user boundary, and
client-go decodes the scopes only once row 32's commit lands, so the second
call is read from the captured response
([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
as the token tools already do.
[gitlab-org/gitlab#629849](https://gitlab.com/gitlab-org/gitlab/-/issues/629849)
above asks for it in one call, and leaves open whether that means no
`read_personal_access_token` or a dedicated permission. What rows 83 and 85
add is the other direction:
after a refusal, the permission to add, named the way GitLab's token form and
token API name it, which phase A of issue 952 wants for the refusal it passes
on.

**Verified, inferred and unverified**:

- Verified in source at 19.4.1 (every line cited above), on `master` where
  noted, in the rack-oauth2 2.2.1 gem, and on gitlab.com with GET requests on
  2026-09-30 (every quote, and the state of every gitlab-org/gitlab issue this
  section links). Verified as well: a classic token under a group's
  enforcement on GitLab.com gets the same code,
  [gitlab-org/gitlab#616442](https://gitlab.com/gitlab-org/gitlab/-/issues/616442)
  showing it for a service account's legacy token; on a self-managed instance
  enforcement refuses no existing legacy token
  (`doc/auth/tokens/fine_grained_access_tokens.md`, lines 134 to 153).
- The 403 body is quoted from the source and from public issues that show it
  from live instances:
  [gitlab-org/gitlab#602772](https://gitlab.com/gitlab-org/gitlab/-/issues/602772),
  [gitlab-org/gitlab#616442](https://gitlab.com/gitlab-org/gitlab/-/issues/616442),
  [gitlab-org/gitlab#594315](https://gitlab.com/gitlab-org/gitlab/-/issues/594315),
  and the Pipeline: Read case above, from gitlab.com. This project has not
  captured one: issue 952's measurement on `gitlab/gitlab-ee:19.4.1-ee.0`
  (2026-09-28) records the status and the code and quotes no body. Capture one
  on the Docker instance before any text calls a body observed.
- Unverified: whether `group::authorization` accepts the field, and whether it
  prefers `scope` or a dedicated key. The reluctance on record is about raw
  names in the sentence and about exposing raw permissions a caller cannot
  grant; the field names only identifiers a caller can grant, though most are
  spelled like a raw permission (above). Besides the issue above, a note on the
  permission renames says the only mitigation would be to "expose assignable
  permission versions and the raw permissions to customers, which I don't
  think we want to do"
  ([gitlab-org/gitlab#591420 note 3113376776](https://gitlab.com/gitlab-org/gitlab/-/issues/591420#note_3113376776)).

**Before posting**: re-read on `master` every GitLab file the draft cites
(checked on 2026-09-30 at `3421b9a9`: `access_denied_error` still builds the
sentence from `for_permission`, and `hidden_boundary` has moved as described
above, which the draft's out-of-scope 404 sentence still covers); capture a
body on the Docker instance. The flag question is answered in the draft, and
its client-go mention is generic, so this issue waits on nothing from row 84.

**Effort**: small, about five Ruby files plus specs and one documentation
example, an estimate. REST only.

<details>
<summary>Draft: the gitlab-org/gitlab issue ("Feature Proposal - lean" headings)</summary>

Reviewed twice while it was prepared, and corrected after a third review; not
posted. Native references, first person as the maintainer, one paragraph per
line.

Title: `Name the missing fine-grained permissions in a machine-readable field of the insufficient_granular_scope response`

```markdown
### Release notes

When a fine-grained personal access token is refused because it lacks a permission, the API response keeps its human-readable description and also names the missing permissions by the identifiers you grant when you create a token (for example `read_work_item`), so scripts, CLIs and API clients can tell you exactly which permission to add.

### Problem to solve

A fine-grained personal access token that lacks a permission gets this 403 from the REST API:

    {"error":"insufficient_granular_scope","error_description":"Access denied: This operation requires a fine-grained personal access token with the following project permissions: [Work Item: Read]."}

#591681 made that sentence human-readable on purpose, and it should stay that way: people search the token form and the documentation for `Work Item`, not for `read_work_item`. But a program cannot act on the sentence. It names display labels, not the identifiers the token API takes in `granular_scopes[].permissions`, and mapping one to the other needs GitLab's permission YAML. The missing permissions are known at that moment: `authorize_granular_token_scopes!` records them, as raw permission names, with `Current.add_granular_denied_permissions` (`lib/api/helpers.rb`), and the response does not carry them.

The classic refusal a few lines above in `lib/api/api_guard.rb` already has a machine-readable part: `insufficient_scope` passes `{ scope: e.scopes }` and the response carries `"scope": "..."`. GitHub has a comparable header for its fine-grained tokens, `X-Accepted-GitHub-Permissions`, which names the permissions an endpoint accepts.

This is part of the "whack-a-mole" described in #553887, where the suggested way through was to read the error messages for the missing permissions. #629849 asks for a token's own grant to be readable in one request; naming the missing permissions in the refusal is the other half, and it needs no new endpoint.

### Proposal

1. Let `Gitlab::Auth::GranularPermissionsError` carry the missing permissions, the way `InsufficientScopeError` carries `scopes`, named by assignable permission identifiers. Map them with `Authz::PermissionGroups::Assignable.available_for_permission`, not `for_permission`, so the answer uses current names only. The permission checks already make that mapping exact for a REST route: `gitlab:permissions:validate` reports a route permission with no live definition, and a raw permission in two.
2. In the `GranularPermissionsError` branch of `lib/api/api_guard.rb`, pass them as `{ scope: identifiers }` to `Rack::OAuth2::Server::Resource::Bearer::Forbidden`, which rack-oauth2 2.2.1 already renders as a space-separated `scope`. `error_description` does not change.
3. Update the example response in `doc/auth/tokens/fine_grained_access_tokens.md`.

The response would become:

    {"error":"insufficient_granular_scope","error_description":"Access denied: ... [Work Item: Read].","scope":"read_work_item"}

The same `error` code also answers "Fine-grained personal access tokens are not yet supported." and "This operation doesn't support fine-grained personal access tokens.", which no permission fixes. The field would be absent there, and its absence is what tells a client so.

Two points for the group to decide:

- For `insufficient_scope`, `scope` lists scopes of which any one is accepted; here it would list permissions that are all missing on the boundary the description names. The `error` code tells the two apart. If you would rather use a dedicated key, that needs a small subclass of `Forbidden`, and I am happy to do either.
- An endpoint that accepts several boundaries passes when any one of them has every permission, while the description names the first failing one. A dedicated key could list each boundary's missing set as an alternative; `scope` can only carry one set.

Out of scope: permission names a token cannot be granted (the field carries assignable identifiers, which for most definitions are spelled like a raw permission, `read_project` among them); the 404 returned when the boundary is hidden from the token's user, which is deliberate; GraphQL, where mutation errors could follow with `extensions` in a separate change and query fields redact to null.

I would like to contribute the merge request.

### Intended users

- Sasha (Software Developer) and Priyanka (Platform Engineer), who configure fine-grained tokens for automation and today do it by trial and error.
- Authors of API clients and integrations: the Go client (gitlab-org/api/client-go), glab, the Terraform provider, python-gitlab, MCP servers.

### Feature Usage Metrics

The denied permissions are already recorded on the `use_pat` event where `track_api_request_from_personal_access_token` is enabled (a GitLab.com derisk flag, default off); the field adds no event.

### Does this feature require an audit event?

No.

<details>
<summary>Where this comes from</summary>

I ran into this while maintaining [gitlab-mcp-server](https://github.com/jmrplens/gitlab-mcp-server), an MCP server for GitLab built on the Go client, which has to tell a model which permission a refused call needed. The project keeps a record of everything done on its dependencies and on sibling projects in [docs/development/upstream-bugs.md](https://github.com/jmrplens/gitlab-mcp-server/blob/main/docs/development/upstream-bugs.md).

</details>

<details>
<summary>Code references (19.4.1)</summary>

- `lib/api/helpers.rb`, `authorize_granular_token_scopes!`: records `result.payload[:denied_permissions]` and raises `GranularPermissionsError` with the message only.
- `app/services/authz/tokens/authorize_granular_scopes_service.rb`, `access_denied_error`: builds the description from `Assignable.for_permission(permission).first`, for the first boundary with missing permissions.
- `lib/api/api_guard.rb`: the `InsufficientScopeError` branch passes `scope`; the `GranularPermissionsError` branch does not.
- `lib/gitlab/auth/auth_finders.rb`: `InsufficientScopeError` has `attr_reader :scopes`; `GranularPermissionsError` is `Class.new(AuthenticationError)`.
- rack-oauth2 2.2.1, `lib/rack/oauth2/server/resource/error.rb`: `Forbidden` accepts `scope` in its options and renders it space-joined.

</details>
```

</details>

### The fine-grained refusal can name a deprecated permission's label

- **Reported**: no. It is read from the source and not yet reproduced on a
  running instance, which comes first.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. The call is refused correctly; the sentence points at a
  permission the token form does not offer.
- **Workaround**: none taken. This server passes the sentence on as GitLab
  wrote it. The routes whose raw permission is among the sixteen below include
  those behind `project.hook_test`, `group.hook_test` and
  `admin.system_hook_test`, and the import routes behind `admin.import_github`,
  `admin.import_bitbucket`, `admin.import_bitbucket_server`,
  `admin.import_cancel_github` and the bulk import actions, so a refusal of
  one of those would tell the caller to grant a permission the form hides.

**Where**: GitLab v19.4.1-ee (`26212baa`), `access_denied_error` in
`app/services/authz/tokens/authorize_granular_scopes_service.rb`, line 130,
names each missing permission by `Assignable.for_permission(permission).first`.
`for_permission` (`lib/authz/permission_groups/assignable.rb:17-19`) keeps
deprecated definitions; `available_for_permission` (`:21-23`) is the one that
drops them. The definitions are loaded in `Dir.glob` order and keyed by name
(`load_files_to_hash` in `lib/authz/concerns/yaml_permission.rb`), so `.first`
is the definition whose file sorts first. On `master` (`3421b9a9`,
2026-09-30) `access_denied_error` still looks the label up with
`for_permission`.

**What**: 71 of the 857 assignable definitions are deprecated at 19.4.1. 124
raw permissions sit in both a deprecated and a live definition, and for 16 the
deprecated one sorts first: `cancel_bulk_import`, `cancel_github_import`,
`create_bitbucket_import`, `create_bitbucket_server_import`,
`create_bulk_import`, `create_github_gist_import`, `create_github_import`,
`create_group_import`, `increment_usage_data_metric`, `read_bulk_import`,
`read_bulk_import_entity`, `read_bulk_import_entity_failure`,
`read_epic_label_event`, `read_issue_label_event`,
`read_user_project_deploy_key` and `test_webhook`. For those the refusal names
a definition the token form does not offer
(`app/graphql/resolvers/authz/access_tokens/permissions_resolver.rb:27` lists
`available_definitions` only) and the documentation does not print
(`lib/tasks/gitlab/permissions/routes/docs_task.rb:141` uses
`available_for_permission`). Two examples:

- `test_webhook`, which `POST /projects/:id/hooks/:hook_id/test/:trigger`
  declares (`lib/api/hooks/trigger_test.rb:34`), is in
  `integrations/webhook/test.yml` (`deprecated: true`) and in
  `integrations/webhook/trigger.yml`. `test.yml` sorts first, so the refusal
  asks for `[Webhook: Test]`, while
  `doc/auth/tokens/fine_grained_access_tokens_rest.md:1468` lists that route
  under `Trigger` and the form offers only Webhook: Trigger.
- `read_epic_label_event` is in `project_planning/epic_label_event/read.yml`
  (deprecated) and `project_planning/work_item/read.yml`, so the refusal would
  ask for `[Epic Label Event: Read]` where the form offers Work Item: Read.

Swapping the lookup is safe: every raw permission sits in exactly one live
definition at 19.4.1, which GitLab's own permission checks make hold for a REST
route by construction, as
[row 83](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose)
records, so `available_for_permission(permission).first` always has one
answer. The defect ends for a definition once GitLab deletes it, which
`gitlab:permissions:assignable:cleanup_deprecated` does after the definition's
rename migration is finalized (`lib/tasks/gitlab/permissions/permissions.rake`,
lines 23 to 29), and that may make the group prefer to wait.

**How we found it**: while studying what the refusal can tell a client
(row 83), a count over the permission YAML showed deprecated definitions
sorting first. It was then recounted for this entry with Ruby 3.3.11, the
version GitLab's `.ruby-version` names at `26212baa`, running `Dir.glob` over
the 857 files of the 19.4.1 checkout, which gives the counts and the list
above; the label each refusal would print was worked out from the same files.

**Before anything is filed**: reproduce it on the Docker GitLab
(`gitlab/gitlab-ee:19.4.1-ee.0`): a fine-grained token holding a project
permission other than Webhook: Trigger (Project: Read, say) on a project that
has a webhook, then
`POST /api/v4/projects/:id/hooks/:hook_id/test/push_events` with it, expecting
403 and `[Webhook: Test]`. A hidden project answers a plain 404,
`{"message":"404 Not Found"}`, instead. At 19.4.1 that is
`token.can?(:read_boundary, boundary)` (service lines 89-91), which
`Authz::BoundaryPolicy` grants when the token's user is a member of the
project or can see it (`app/policies/authz/boundary_policy.rb:16-26`), whatever
the token grants; on `master` the service asks the same of the token's user
through the project's own policy. Either way, use a token of a member of the
project, such as the user who added the webhook. Paste the body observed into
the draft below in place of its marker. Then search `gitlab-org/gitlab` once
more; a search on 2026-09-30 found nothing on it.

**Proposal**: look the label up with `available_for_permission` at line 130,
the method the documentation task already uses, with a spec for a permission
that sits in a deprecated and a live definition. Filed as the issue drafted
below, or as a small merge request of its own; or, if the group agrees on row
83's issue that the lookup should change for the new field anyway, carried by
that merge request.

**What it costs this server**: see
[row 83's section](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose).
A model relaying `[Webhook: Test]` sends its user looking for an option that
is not there.

**Verified, inferred and unverified**:

- Verified in source at 19.4.1 and on `master` where noted: the lookup, the
  two methods, the load order, the counts and the list (with Ruby 3.3.11's
  `Dir.glob`), the form's and the documentation's lookups, the route
  declarations, the permission checks, and the cleanup task.
- Inferred: the 403 itself. No refusal naming a deprecated label has been
  observed on a running instance.

**Effort**: small, a one-line change and a spec, owned by
`group::authorization`.

<details>
<summary>Draft: the gitlab-org/gitlab issue ("Bug" headings)</summary>

Written for this entry and reviewed with it, not yet in its final form: the
current-behaviour section waits on the reproduction above. Native references,
first person as the maintainer, one paragraph per line.

Title: `The insufficient_granular_scope refusal names a deprecated permission for 16 raw permissions`

```markdown
### Summary

When a fine-grained personal access token is refused, `access_denied_error` in `app/services/authz/tokens/authorize_granular_scopes_service.rb` names each missing permission by `Authz::PermissionGroups::Assignable.for_permission(permission).first`. `for_permission` keeps deprecated definitions, so for 16 raw permissions the refusal names a deprecated definition, which the token form does not offer and the fine-grained permissions reference does not list. For example, `POST /projects/:id/hooks/:hook_id/test/:trigger` requires `test_webhook`, which is in both `integrations/webhook/test.yml` (`deprecated: true`) and `integrations/webhook/trigger.yml`. `test.yml` sorts first, so the refusal asks for `[Webhook: Test]`, while the reference lists the route under `Trigger` and the form offers only Webhook: Trigger.

### Steps to reproduce

1. On GitLab 19.4.1, as a member of a project that has a webhook, create a fine-grained personal access token that does not hold Webhook: Trigger (for example one holding only Project: Read).
2. Call `POST /api/v4/projects/:id/hooks/:hook_id/test/push_events` with that token.

### What is the current *bug* behavior?

TODO before posting: paste the observed 403 body.

### What is the expected *correct* behavior?

The refusal names `[Webhook: Trigger]`, the permission the token form offers and the reference lists for that route.

### Relevant logs and/or screenshots

The 16 raw permissions whose deprecated definition sorts before the live one in `Dir.glob` order at 19.4.1: `cancel_bulk_import`, `cancel_github_import`, `create_bitbucket_import`, `create_bitbucket_server_import`, `create_bulk_import`, `create_github_gist_import`, `create_github_import`, `create_group_import`, `increment_usage_data_metric`, `read_bulk_import`, `read_bulk_import_entity`, `read_bulk_import_entity_failure`, `read_epic_label_event`, `read_issue_label_event`, `read_user_project_deploy_key`, `test_webhook`.

### Possible fixes

Look the label up with `Assignable.available_for_permission`, as `gitlab:permissions:routes:compile_docs` already does (`lib/tasks/gitlab/permissions/routes/docs_task.rb`), with a spec for a permission that sits in a deprecated and a live definition. The permission checks make the lookup exact for a REST route: `gitlab:permissions:validate` reports a route permission with no live definition, and a raw permission in two, so it always has one answer. The defect also ends for each definition that `gitlab:permissions:assignable:cleanup_deprecated` deletes once its rename migration is finalized, if you would rather wait for that. I would like to contribute the merge request.

<details>
<summary>Where this comes from</summary>

I ran into this while maintaining [gitlab-mcp-server](https://github.com/jmrplens/gitlab-mcp-server), an MCP server for GitLab, which passes this refusal on to a model. The project keeps a record of everything done on its dependencies and on sibling projects in [docs/development/upstream-bugs.md](https://github.com/jmrplens/gitlab-mcp-server/blob/main/docs/development/upstream-bugs.md).

</details>
```

</details>

## GitLab Orbit (`gitlab-org/orbit/knowledge-graph`)

Both entries here were filed as an issue first and then a merge request that
closes it, because the project's `CONTRIBUTING.md` asks for an issue before any
non-trivial merge request. Each merge request bumps the Orbit skill to
`0.32.3`, as every change under `skills/orbit/` must, so whichever lands second
needs `0.32.4`. Nothing in the project's pipeline will flag it: the two
branches merge cleanly, since both make the same change to the version, and
`skill-version-bump-check` compares a branch with its merge request's diff
base, where the skill stays at `0.32.2` until the branch is rebased. The
commit that updated each branch's tests (below) says so in its message.

### The DSL schema says a path query may omit `rel_types`

- **Reported**: yes,
  [gitlab-org/orbit/knowledge-graph#1329](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1329),
  opened 2026-09-28, mentioning the author of the rule and of the current text.
- **In review**: yes,
  [gitlab-org/orbit/knowledge-graph!2650](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2650),
  opened the same day from the community fork, which closes the issue; the
  issue was then edited to name it. Read on the morning of 2026-09-29 it has
  no reviewer but GitLab Duo, no approval and no comment from a person, and
  its fork pipeline is red on three jobs. `lint:prose` and
  `pinned-version-check` stop before checking anything in the change.
  `pinned-version-check` exits on
  `fatal: origin/main...HEAD: no merge base`: it fetches `main` at depth 1,
  and in the fork `main` had moved past `74d4a4cb`, where the branch starts,
  so the shallow clone holds no commit the two share. `lint:prose` reports
  `diff base ... is unreachable`, but the git error under it is about an
  empty revision: `prose_lint.py` diffs the base against
  `CI_MERGE_REQUEST_SOURCE_BRANCH_SHA`, which is empty outside merged results
  pipelines, and since `b7bce6fb` (2026-09-28) it passes that variable to
  `git diff` as a revision of its own instead of after `base...`, where an
  empty one meant `HEAD`. Every fork pipeline since has failed it this way.
  The third, `unit-test`, is ours: three `orbit-server` tests pin the skill
  version at `0.32.2` (lines 22 and 74 of
  `crates/orbit-server/src/grpc/service/tests/skills.rs` and one in
  `crates/orbit-server/src/skills/mod.rs`, set to `0.32.2` on `main` by
  `bf510a5d` on 2026-09-28 in the same commit that bumped the skill, which the
  branch is based on), and the change bumps
  `skills/orbit/SKILL.md` to `0.32.3`, so the branch owed those assertions
  the new version. The same three fail on
  [gitlab-org/orbit/knowledge-graph!2651](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2651),
  for the same reasons. `0acf02c0`, pushed at 11:21 UTC on 2026-09-29, sets
  the three to `0.32.3`, and its message tells the maintainers about the
  `0.32.4` the second merge request to land will need. In its fork pipeline,
  2893038521, `unit-test` passed, and the pipeline finished with the two jobs
  that stop before running as its only failures. One of them would fail for
  real in the merged-results pipeline a maintainer runs, where the branch and
  the target share a merge base:
  `pinned-version-check` (`scripts/check-pinned-version.sh`) refuses a change
  to `config/schemas/graph_query.schema.json` that does not bump `query_dsl`
  in `config/versions.yaml`, unless the description carries
  `[skip pinned-version-check]`, which the script asks for when a change does
  not affect the shape. This branch only rewords a description there, so at
  12:44 UTC on 2026-09-29 its description gained the marker, at the top of its
  collapsed details block, where the project's description lint does not count
  it and the 2700 characters a pipeline reads of a description still hold it,
  with an offer of a patch bump instead; the Files list also names the test
  pins now. At 22:37 UTC the same day `@gitlab-bot ready` went up with a note
  giving the two jobs' real causes, and the bot requested review from
  `kerrizor`, a merge request coach, and set `workflow::ready for review`.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: not yet. `orbit.dsl` hands the model GitLab's schema text
  verbatim, so it repeats the wrong condition; `orbit.query`'s own guidance
  will say that every path query needs `rel_types` with
  [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031), and
  the two disagree until this is fixed upstream.

**Where**: `PathConfig.rel_types.description` in
`config/schemas/graph_query.schema.json` (main `1c43d9f0`, line 636), served
verbatim by `GET /orbit/schema/dsl`; the same statement in the query language
guide (`docs/source/remote/queries/query-language.md:566,568-570`), its copy in
the Orbit skill and the skill's path recipe (`recipes.md:419-420`).

**What**: they say `rel_types` is optional when both endpoints use `node_ids`.
The validator (`check_path` in
`crates/query-engine/compiler/src/passes/validate.rs:1221-1231`) has rejected
every path query without it since
[gitlab-org/orbit/knowledge-graph!1621](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/1621)
(2026-06-09), and its own test pins the `node_ids` case. On GitLab.com the
query the schema allows answers `400` with "path_finding requires rel_types to
bound fan-out". The guide's path example also runs from a project to a user
through relationship types that point the other way, so it returns no rows;
reversed, the same query returns two paths.

**Fix in review**: the merge request states the rule in the schema, the guide,
the skill copy and the recipe, turns the guide's example round, and bumps the
skill. It changes no accepted query, so it leaves the `query_dsl` version
alone and puts that choice, and making `rel_types` required in the schema
itself, to the maintainers in the issue.

**How we found it**: sending the query the description allows to GitLab.com
while rewriting `orbit.query` for issue 1031.

### The DSL schema says the default neighbors direction is `both`

- **Reported**: yes,
  [gitlab-org/orbit/knowledge-graph#1330](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1330),
  opened 2026-09-28.
- **In review**: yes,
  [gitlab-org/orbit/knowledge-graph!2651](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2651),
  opened the same day from the community fork, which closes the issue; the
  issue was then edited to name it. Read on the morning of 2026-09-29 it is in
  the state row 75's merge request is in: no reviewer but GitLab Duo, and a
  fork pipeline
  red on the same three jobs, `unit-test` among them because its own bump of
  `skills/orbit/SKILL.md` to `0.32.3` breaks the three tests that pin
  `0.32.2`. `639f3d11`, pushed at 11:21 UTC with `0acf02c0` and carrying
  its message with the other merge request's number, sets them to `0.32.3`,
  and `unit-test` passed in its fork pipeline, 2893038522. Its
  `pinned-version-check` would fail in the merged-results pipeline for the
  reason row 75's would, and this branch also changes the schema's declared
  `default` from `both` to `outgoing`, which is what the compiler already
  applies, so no query is accepted, rejected or answered differently. Its
  description gained the marker at the same time and in the same place as row
  75's, with an offer of a `query_dsl` bump should the maintainers read the
  declared default as part of the contract. At 22:37 UTC `@gitlab-bot ready`
  went up with a note giving the same two causes as row 75's, and the bot
  requested review from `ms.mondrian`, a merge request coach, and set
  `workflow::ready for review`.
- **Merged**: no.
- **Blocking**: no, but a result can be silently incomplete.
- **Workaround**: not yet. `orbit.query`'s own guidance will say that
  `direction` defaults to `outgoing` with
  [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031).

**Where**: `NeighborsConfig.direction` in
`config/schemas/graph_query.schema.json` (main `1c43d9f0`, lines 654 to 658)
declares `"default": "both"` and describes the default as `both`, while the
shared `Direction` definition in the same file and the compiler
(`crates/query-engine/compiler/src/input.rs:446-452` and `882-888`) default to
`outgoing`.

**What**: on GitLab.com a neighbors query without `direction` returns only the
relationships that start at the center node, exactly as `outgoing` does, and
nothing in the answer says the incoming ones were left out: a project's
neighbors are its branches, and its group, creator and labels appear only with
`"direction": "both"`.

**Fix in review**: the merge request documents `outgoing` in the schema and
adds `direction` and `rel_types` to the guide's neighbors section and its
skill copy. Making `both` the compiler's default instead would change what
existing queries return, so it is left in the issue as the maintainers'
choice.

**How we found it**: comparing a neighbors query with and without
`direction` against GitLab.com while checking the schema for issue 1031.

## Other

### go-selfupdate depends on the deprecated x/crypto/openpgp

- **Reported**: yes,
  [creativeprojects/go-selfupdate#57](https://github.com/creativeprojects/go-selfupdate/issues/57).
- **In review**: yes,
  [creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58),
  still open, mergeable, and unreviewed on 2026-09-27. Its last activity is
  the maintainer's answer of 2026-08-05: the change is breaking by Go's
  convention and needs a v2 of the module, which they have not had time to
  prepare. It went unanswered, and deliberately: there is no form of it that
  breaks nothing, since the type of `PGPValidator.KeyRing` is the import
  itself and moving it to a subpackage is breaking too, and this server no
  longer depends on the module, so a reply would only ask them again. Nothing
  is owed on it.
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

The SDK reaches it on its own wherever a method delegates to its `WithOptions`
sibling with a nil options pointer, as `CancelJob` has since v2.32.0,
`GetJobArtifacts` since v3.8.0 and `PublishAllDraftNotes` since v3.12.0:

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
| `Jobs.GetJobArtifacts`            | query `""`                     | query `""`                         |

`GetJobArtifacts` takes the same delegation and is unharmed only by accident:
its request is a GET, so the nil goes to `query.Values`, which returns early on
a nil pointer, and the empty `RawQuery` adds no `?`. That empty `RawQuery` does
replace a query already on the URL handed to `NewRequestToURL`, which is the
query branch's half of the defect, but `NewRequest` adds none to the URL it
builds, so among the SDK's own delegations the defect is confined to the
methods whose verb makes `NewRequestToURL` take the marshalling branch.

Nothing is expected to break at GitLab, which is why it is not blocking:
`Grape::Middleware::Formatter` sets the form hash only `if body.is_a?(Hash)`,
and `null` parses to `nil`, so the endpoint sees the same empty parameter set
either way. That is read from Grape's formatter rather than measured against a
live instance, so it is the reason this is not urgent and not a claim that the
bytes are identical. It is still a request the SDK did not mean to send, and it
will reach any future delegation of the same shape on a POST, PUT or PATCH.

The fix is a nil-pointer check where the decision is made, in
`NewRequestToURL`, rather than at each delegation, since the next one will be
written the same way. It changes what every caller passing a nil options
pointer sends, so the merge request now puts it behind an opt-in client
option, and 4.0 makes it the default.

- **Reported**: yes, first as commit 2 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, and since 2026-09-30 on its own in
  [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065),
  at the review's request.
- **In review**: yes, open, in
  [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065).
  Commit 2 of the joint merge request first applied that check to every
  caller, and on 2026-09-29 the maintainer answered that it has to be a
  toggle, since nobody can know whether a caller relies on the `null`. It is
  now
  `feat(client): add option to treat nil options pointers as no options`:
  `WithNilOptionsOmitted()`, a client option off by default and modelled on
  `WithOnlyIdempotentRetries`, makes a nil pointer in the options mean no
  options, in the query branch as well as the body branch, so with it
  `CancelJob` and `PublishAllDraftNotes` send no body at all. Without it no
  request changes, and the tests pin both sides.
  [gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301)
  carries the rest in its section 6: 4.0 makes the option's behavior the
  default, where the option has no effect, and 5.0 removes the option. At
  22:06 UTC on 2026-09-29 the review asked for the change to be reviewed on
  its own, as a library-level change rather than a field sync, so on
  2026-09-30 it left the joint merge request for
  [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065):
  the commit reviewed there, cherry-picked unchanged onto v3.15.0, and
  [gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301)
  now names that merge request as the one carrying the option.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes, and neither route the SDK sends it on is reached from
  this repository, because both handlers call the `WithOptions` sibling with
  a non-nil options value. `job.cancel` passes a `CancelJobOptions` whether
  or not the caller asks for force, so a call without force sends `{}`; until
  2026-09-29 it called `CancelJob` whenever force was not asked for and sent
  `null`, which this entry had missed.
  `TestJobCancel_Body_CarriesForceOnlyWhenAsked` in `internal/tools/jobs`
  pins the body with and without force. `mr_review.draft_note_publish_all`
  offers the route's `note`, `internal` and `reviewer_state`, which only
  `PublishAllDraftNotesWithOptions` carries, so it passes a non-nil options
  value as before and a call naming none of the three sends `{}`;
  `TestDraftNotePublishAll_Body_CarriesExactlyTheReviewOptionsGiven` in
  `internal/tools/mrdraftnotes` pins the body for each combination. Both
  calls carry a `//nolint:staticcheck` for SA1019, because client-go marks
  the two methods `Deprecated:` only until v4 folds the options into
  `CancelJob` and `PublishAllDraftNotes`, and says to use them meanwhile when
  the options are needed. 4.0, which makes that fold and makes a nil pointer
  send no body, retires the workaround. The defect itself is untouched and
  still reaches any other delegation of the same shape on a POST, PUT or
  PATCH.

### Seven more option structs send an optional param on every call

- **Reported**: yes, as commit 34 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit gives each of the seven fields
  `omitempty` on both halves of the tag, and its message records the one case
  where the null changes what GitLab stores: the pipeline schedule variable
  edit hands every declared param the request carries to the update service,
  so an edit that changes only the variable type also sets the value to null.
  The one merge request the **Effort** below asks for is that one, carrying
  entries 6 and 52 as commits 32 and 33.
- **Merged**: no.
- **Blocking**: no, field by field in the second table below. For five of the
  seven, GitLab reads a null exactly as it reads the key left out, or never
  receives one. `label_id` is latent: a null would be refused beside another
  board list type, which neither the struct nor the handler offers today.
  `value` narrows an action: GitLab would store the null over the variable's
  value, and what keeps it from doing so is that the handler requires a value,
  which GitLab does not, so an edit of the type alone cannot be made. Entries
  [6](#setfeatureflagoptions-fields-lack-omitempty) and
  [52](#updatepackageprotectionrulesoptions-sends-two-explicit-nulls-on-every-partial-update)
  hold the fields of this class GitLab refuses outright: every feature flag
  set sent through the SDK, and a package protection rule update that leaves
  out either of its two fields.
- **Workaround**: none needed for five. Two handlers never leave the field
  unset, because they require it, and neither requirement was written for this
  defect. `groupboards.CreateGroupBoardList` requires `label_id`, the only
  list type the struct models, so that requirement is not a workaround and
  stays until client-go models the milestone, iteration and assignee lists. `pipelineschedules.EditVariable` has required `value` since
  the domain was written, and the requirement is what keeps the null from
  reaching GitLab and what costs the action an edit of the type alone. It
  retires with the tag: `value` then becomes optional in the handler, in its
  check and in the `required` of its input schema, as GitLab declares it. A
  handler could also get past the tag today, contrary to what this entry first
  said, in two ways: a request option, since client-go runs the request
  options after it has marshalled the body (`NewRequestToURL` in `gitlab.go`)
  and an option can read the body and replace it, which is how client-go's own
  GraphQL pagination option works; or a request the handler builds itself, as
  `features.Set` does for entry 6. Neither is carried.

**Where**: seven option structs across client-go.

**What**: the same defect as entries 6 and 52, found systematically rather
than one at a time. `audit_1to1 -scope=paths` compares the keys
`encoding/json` writes whatever a handler set against the params GitLab's live
record marks optional, and reports eleven rows over nine fields in eight option
types. Two of the rows are entry 52. These are the other seven types, with the
handler here that sends each:

| Option type                           | Field        | Param         | Endpoint                                                  | Handler here                       |
| ------------------------------------- | ------------ | ------------- | --------------------------------------------------------- | ---------------------------------- |
| `CreateDependencyListExportOptions`   | `ExportType` | `export_type` | `POST /pipelines/:id/dependency_list_exports`             | `dependencies.CreateExport`        |
| `CreateGroupIssueBoardListOptions`    | `LabelID`    | `label_id`    | `POST /groups/:id/boards/:id/lists`                       | `groupboards.CreateGroupBoardList` |
| `AddGroupMemberOptions`               | `ExpiresAt`  | `expires_at`  | `POST /groups/:id/members`                                | `groupmembers.AddMember`           |
| `AddProjectMemberOptions`             | `ExpiresAt`  | `expires_at`  | `POST /projects/:id/members`                              | `members.Add`                      |
| `ShareWithGroupOptions`               | `ExpiresAt`  | `expires_at`  | `POST /groups/:id/share`                                  | `groupmembers.ShareGroup`          |
| `ShareWithGroupOptions`               | `ExpiresAt`  | `expires_at`  | `POST /projects/:id/share`                                | `projects.ShareProjectWithGroup`   |
| `CreateIssueLinkOptions`              | `LinkType`   | `link_type`   | `POST /projects/:id/issues/:iid/links`                    | `issuelinks.Create`                |
| `EditPipelineScheduleVariableOptions` | `Value`      | `value`       | `PUT /projects/:id/pipeline_schedules/:id/variables/:key` | `pipelineschedules.EditVariable`   |

Six of the eight handlers set the pointer only when the caller named a value,
which is the safe side of the choice they have. For five of them the null is
what the tag adds when the caller did not; for `dependencies.CreateExport` it
is not, because client-go fills in `"sbom"` when the pointer is nil, so no null
is ever sent there. The other two, `groupboards.CreateGroupBoardList` and
`pipelineschedules.EditVariable`, require the field and always set it.

**What GitLab does with the null**, read from the GitLab 19.4.1-ee source (the
release the live record was taken from) and, for `expires_at`, from the
end-to-end suite as well. The sources the table cites are linked below it,
each pinned to that tag.

| Param         | Route                        | How the route reads it                                                                                                                                                                                                                             | Null against the key left out                                                  | Blocking                                                 |
| ------------- | ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | -------------------------------------------------------- |
| `export_type` | pipeline export              | never sent as null: client-go fills in `"sbom"` when the caller named none, which is the route's own default (`ee/lib/api/dependency_list_exports.rb:81`)                                                                                          | not reached                                                                    | No                                                       |
| `label_id`    | group board list create      | EE replaces the CE `requires` with `exactly_one_of :label_id, :milestone_id, :iteration_id, :assignee_id` (`ee/lib/ee/api/boards_responses.rb:16`), and Grape 2.4.0 counts the keys present, a null included (`MultipleParamsBase#keys_in_common`) | differs beside another list type: refused as mutually exclusive                | Latent: the struct models no other list type             |
| `expires_at`  | group and project member add | the raw `params` hash with the source added to it (`lib/api/members.rb:146`), read by the create service as `params[:expires_at]` (`app/services/members/create_service.rb:112`)                                                                   | the same: nil either way, no expiry                                            | No                                                       |
| `expires_at`  | group share                  | `expires_at: params[:expires_at]` (`lib/api/groups.rb:769`)                                                                                                                                                                                        | the same                                                                       | No                                                       |
| `expires_at`  | project share                | `declared_params(include_missing: false)` (`lib/api/projects.rb:985`): the null is passed as nil and the missing key is not passed, and a new link has no expiry either way                                                                        | the same on a create                                                           | No                                                       |
| `link_type`   | issue link create            | `declared_params[:link_type]` (`lib/api/issue_links.rb:68`), nil either way, so both take the documented default, `relates_to`                                                                                                                     | the same                                                                       | No                                                       |
| `value`       | schedule variable edit       | `declared_params(include_missing: false)` (`lib/api/ci/pipeline_schedules.rb:378`), then `variable.assign_attributes(params)` (`app/services/ci/pipeline_schedules/variables_base_save_service.rb:9`), and nothing validates the value             | differs: the null is assigned over the stored value, the missing key leaves it | Narrows `pipeline.schedule_edit_variable` to value edits |

Sources, at `v19.4.1-ee`, and Grape at 2.4.0, the release that tag's
`Gemfile.lock` resolves:
[`ee/lib/api/dependency_list_exports.rb:81`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/ee/lib/api/dependency_list_exports.rb#L81),
[`ee/lib/ee/api/boards_responses.rb:16`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/ee/lib/ee/api/boards_responses.rb#L16),
[`MultipleParamsBase#keys_in_common`](https://github.com/ruby-grape/grape/blob/v2.4.0/lib/grape/validations/validators/multiple_params_base.rb#L22),
[`lib/api/members.rb:146`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/members.rb#L146),
[`app/services/members/create_service.rb:112`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/app/services/members/create_service.rb#L112),
[`lib/api/groups.rb:769`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/groups.rb#L769),
[`lib/api/projects.rb:985`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/projects.rb#L985),
[`lib/api/issue_links.rb:68`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/issue_links.rb#L68),
[`lib/api/ci/pipeline_schedules.rb:378`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/ci/pipeline_schedules.rb#L378)
and
[`app/services/ci/pipeline_schedules/variables_base_save_service.rb:9`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/app/services/ci/pipeline_schedules/variables_base_save_service.rb#L9).

The `expires_at` rows are also measured, on both Docker runtimes: the committed
coverage record (`docs/development/e2e-coverage.json`, 2026-09-26, GitLab
19.4.1 CE and 19.3.1-ee) holds `group.group_member_add`, `project.member_add`,
`group.group_member_share` and `project.share_with_group` at L3, asserted on
all three surfaces, and the five scenarios behind them
(`TestGroupMembers_Lifecycle_AddEditListAndRemove`,
`TestGroupMembers_AddDeveloper_AnswersTheMembership`,
`TestProjectMembers_Lifecycle_AddEditAndRemove`,
`TestGroupSharing_TwoSurfaces_ShareAndUnshare` and
`TestProjectSharing_WithGroup_ListsAndRemoves`) name no expiry, so each call
sent `"expires_at": null` and GitLab created the membership or the share. The
`link_type`, `label_id` and `value` rows are read and not measured: the issue
link scenario always names a type, and the other two handlers always send
their field.

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
is affected. `ShareWithGroupOptions` is the one reached from two packages:
`projects` passes it to `Projects.ShareProjectWithGroup` and `groupmembers`
to `GroupMembers.ShareWithGroup`. The audit also lists it under `groups`,
which is the join and not a third caller: `groups.ShareGroupWithGroup`
passes `ShareGroupWithGroupOptions`, whose
`expires_at` already carries `omitempty`, to the same `POST /groups/:id/share`,
and the check asks about every option struct that reaches a recorded route.
That row, and the `export_type` one above, which client-go never sends as
null, are the rule's reading of a tag rather than a request this server sends
wrongly; both leave the report the day the tags change.

**Why entry 6 is not in this list.** `SetFeatureFlagOptions` is the same
defect and does not appear, because the audit reads the request this server
**recorded** and `internal/tools/features.Set` builds its body by hand to
avoid the bug. The workaround hides the defect from the check that would have
found it, which is a property worth knowing before trusting the count: the
eight are the ones we still send through the SDK, not the eight that exist.

**Effort**: small, and one merge request covers all nine structs of entries 6,
52 and these seven. Every case is a struct tag plus a test that the key is
absent when the field is nil. `omitempty` is the right tag for every one of
them, and `Nullable[T]` for none: the table above finds no field where a null
is how a caller asks GitLab for something. An expiry is cleared by an edit
route, and every `expires_at` field here is on a create route; a schedule
variable's value is emptied with an empty string, which a set pointer still
sends under `omitempty`. Changing a field's type would also break every caller
of the struct, where a tag breaks none.

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

### gobco type-checks every file of a package directory, whatever its build constraints say

- **Reported**: yes,
  [rillig/gobco#40](https://github.com/rillig/gobco/issues/40), opened
  2026-09-28 with a minimal reproduction of the panic and of four related cases.
- **In review**: yes,
  [rillig/gobco#41](https://github.com/rillig/gobco/pull/41), opened the same
  day from a fork: two commits, one per half of the fix, each with its tests.
  The project's CI has not run on it, which for a first contribution waits on
  the maintainer's approval.
- **Merged**: no. The maintainer usually fixes an issue that carries a
  reproduction directly, within days, and rarely merges a pull request as
  sent, so the fix may well arrive as the maintainer's own commit; either way
  the entry retires on the release that carries it.
- **Blocking**: no for the server. It blocked the condition gate on three
  packages: `cmd/server` and `internal/toolutil` panicked before anything was
  instrumented, and `test/e2e/internal/harness` with `-tags=e2e` was never
  instrumented at all. With the workaround below only the harness is left
  unmeasured.
- **Workaround**: partial.
  [Issue 1017](https://github.com/jmrplens/gitlab-mcp-server/issues/1017)
  measures such a package from a copy of the module that leaves out the files
  the go command does not build here and blanks the constraint lines gobco's
  narrow build context would misread, which is what the fix does inside gobco:
  `scripts/coverage-conditions.sh`, run by `make coverage-conditions` and by
  `scripts/check-spec-conditions.sh`, measures `cmd/server` at 1990 of 2032
  conditions and `internal/toolutil` at 3604 of 3782 on linux/amd64. The
  harness is not measured: every file of it sits behind `e2e`, so blanking
  its constraints fails its own test that each file carries exactly that
  line, and the script says the figure it printed is not a measurement. The
  platform halves the copy leaves out (five files of `cmd/server`, two of
  `internal/toolutil`) are measured only on a platform that builds them, and
  nothing runs gobco there today. It retires when a gobco release carries the
  fix and the pin, `github.com/rillig/gobco@v1.3.4` in
  `scripts/coverage-conditions.sh`, moves to it.

**Where**: `rillig/gobco` at `7a09995` (v1.3.4 behaves the same).
`instrumenter.instrument` hands every `.go` file of the directory to
`parser.ParseDir` (`instrumenter.go:87-94`), whose filter only narrows the set
to one file named on the command line, and `resolveTypes` type-checks all of
them as one package (`instrumenter.go:142`), which v1.3.4 introduced
(`09623a7`); v1.3.3 resolved no types. `shouldBuild` matches files against a
build context holding only `GOOS` and `GOARCH` (`instrumenter.go:1001`), so
no build tag and no release tag counts, and the source importer resolves
imports in `build.Default` (`instrumenter.go:114`), which knows nothing of
the tags gobco passes on to `go test`.

**What**: a package that implements a function once per platform, the usual
Go idiom, makes gobco panic with "redeclared in this block" while `go test`
passes on the same package. The same cause shows four more ways: a
`//go:build ignore` generator with `package main` beside the package makes
gobco write a bridge test with `import  ""`; a `TestMain` in another platform's
test file keeps gobco from adding its own, and the report is
`Condition coverage: 0/0` with exit status 0; files built only with `-tags` are
never instrumented, not even with `-test -tags=...`; and a black box test of
such a package panics with "could not import".

**Fix in review**: the pull request selects files with `build.Default.MatchFile`
before parsing, which also skips names starting with `_` or `.` and honours
release tags, and takes the build tags from `GOFLAGS` and the `-test` options
the way the go command does, setting them in `build.Default.BuildTags` while
the package is instrumented, since `importer.ForCompiler` takes no other
build context. No dependency is added, and `sh ./test.sh` passes with Go
1.16.15, 1.20.14 and 1.27.1. With it, gobco measures this repository's three
packages: `cmd/server` 1990 of 2032 conditions and `internal/toolutil` 3604 of
3782, the same figures the copy of issue 1017 gives, and
`test/e2e/internal/harness` with `-tags=e2e` 863 of 1162. That last run fails
one test of ours,
`TestSources_EveryFile_CarriesExactlyTheE2EConstraint`, because the files gobco
generates carry no `//go:build e2e`; exempting gobco's files there is ours to
do.

**How we found it**: the condition gate reported `cmd/server` as not measured
([issue 1017](https://github.com/jmrplens/gitlab-mcp-server/issues/1017)), and
reproducing the panic outside this repository showed where it comes from.

### go/types reads an imported generic instance another checker is expanding

- **Reported**: yes, by another user, as
  [golang/go#81122](https://github.com/golang/go/issues/81122) on 2026-08-26.
  Read on 2026-09-29 it was open, labelled NeedsInvestigation, with no
  milestone; its reporter added that it reproduces with every package loaded
  from source too. Our
  [comment of 2026-09-30](https://github.com/golang/go/issues/81122#issuecomment-5901488429)
  adds a reproducer that needs no x/tools, the go1.26 read, and the change
  below with the source-loaded case it also covers.
- **In review**: yes, [golang/go#81871](https://github.com/golang/go/pull/81871),
  opened on 2026-09-29 and imported to Gerrit as
  [go.dev/cl/841585](https://go.dev/cl/841585), open. Patch set 2
  (2026-09-30) holds both constructions in its test, the importer's and a
  package checked from source. Its reviewers are Robert Griesemer and Mark
  Freeman. On 2026-09-30 Mark Freeman asked for a shorter commit message,
  patch set 3 of the same day carries it with the code unchanged, and that
  thread is resolved; no vote yet.
- **Merged**: no. No Go release carries a fix, and `master` still reads the
  field the same way.
- **Blocking**: no for the server, which type-checks nothing. It failed the
  race gate of a release rehearsal, in
  `TestAccess_OnlyTheSanctionedPackagesReadTheKey` of
  `internal/testutil/modelcorpus`, and any race run of a test that
  type-checks two or more packages in one go/packages load can fail the same
  way at random. Measured on this tree, 20 test packages do: the audits under
  `cmd/` that type-check Go source with go/packages, `cmd/internal/goprogram`,
  `cmd/internal/graphqldocs` and `internal/testutil/modelcorpus`.
- **Workaround**: yes. `internal/testutil/serialtypecheck` is imported blank by
  one test file of each of the 22 packages whose test binary links
  go/packages. In a race build its initializer sets GOMAXPROCS to one before
  go/packages sizes the semaphore it type-checks under, so the checkers run
  one after another; in any other build it changes nothing. Its tests hold
  the initialization order it relies on, and fail when a test binary links
  go/packages without it. Over fresh processes of the test that failed, the
  base raced in 13 of 80 and the workaround in 0 of 80. It retires when the Go
  release `go.mod` pins carries the fix: the package, its blank imports and
  its line in `TestDependencies_TestSupport_NeverReachesTheServerBinary` go
  together. Outside the detector the commands and their tests still
  type-check in parallel, and the read still races, with no effect on this
  tree. An instance expands from its origin's RHS, which for a type read from
  export data is its underlying type, and for every generic type this module
  and client-go v3.14.0 declare is a type literal. So `isComplete` returns
  true whichever value it reads (nil, the new type, or a torn pair of the
  two), which is its answer for any type no checker owns. What would change
  that is a generic type loaded from source whose RHS is another defined
  type, such as `type A[T any] B[T]`: a torn read of its instance
  dereferences a nil `*Named` and the command panics.

**Where**: `go/types` in go1.27.1, and its copy
`cmd/compile/internal/types2`. `(*Checker).isComplete` reads `t.fromRHS` of a
`*Named` without unpacking it (`cycles.go:122`), while `(*Named).unpack`
expands an instance by writing `n.fromRHS = n.expandRHS()` (`named.go:244`).
go1.26 has the same unsynchronized read in `(*Checker).finiteSize`
(`cycles.go:127`, reached from `pendingType`), added with the value
observance check of CL 726580 and carried into `isComplete` by CL 734980 in
1.27. go1.25 has neither read. `hasVarSize` in `builtins.go` reads the same
field and unpacks first, with a comment saying why.

**What**: a package imported from export data is one `*types.Package` shared
by every checker that imports it, and the importer leaves a generic instance
reachable from its API unexpanded, for whichever checker needs it first to
expand. `iter.Seq[string]`, the result of `strings.SplitSeq`, is the one
these tests reach. Two checkers ranging over it race: one expands it through
`rangeKeyVal`, `commonUnder` and `Underlying`, the other reads the field in
`isComplete` from `callExpr`. go/packages type-checks every package it loads
from source in parallel, bounded by a semaphore sized from
`runtime.GOMAXPROCS(0)` when the package is initialized, so any load that
type-checks two packages from source is exposed. `GOMAXPROCS=1` set before
the process starts makes such a load serial and the race goes: 0 of 20 runs
against 2 of 20 at five. `runtime.GOMAXPROCS(1)` inside a test does nothing,
because the semaphore was sized before it ran. The report upstream says
go1.26 is not affected; a reproducer without x/tools, eight packages checked
concurrently against one imported `strings`, says it is: 30 of 30 runs
reported the race on go1.27.1, 10 of 30 on go1.26.8, and 0 of 30 on
go1.25.14.

**Fix upstream**: [golang/go#81871](https://github.com/golang/go/pull/81871)
([go.dev/cl/841585](https://go.dev/cl/841585)). `isComplete` returns true for
a `*Named` that no checker owns (`t.check == nil`) without reading its
`fromRHS`. Such a type comes from an importer, from the API or from a pass
that has finished, and its RHS cannot lead back to an object on this
checker's path, so the walk from it always ended in true:

```diff
 	case *Named:
+		if t.check == nil {
+			return true
+		}
 		obj = t.obj
 		rhs = t.fromRHS
```

On a toolchain built from `master` with the change, each of its test's two
subtests, run on its own, raced in none of 200 runs in each of `go/types`
and `types2`, where it raced in at least 99 of 100 without it. An instance
written as a type in the source is expanded before `Check` returns and does
not race; the source subtest uses one created by substitution
(`var F = Of[string]`), which stays unexpanded and unowned. The reproducer
went from 20 of 20 runs racing to none. A temporary assertion that the old
walk returns true for every such type held through `go build -a std cmd`,
`go vet std cmd` and both packages' tests.
Unpacking such a type first also removes the race: applied to both copies of
`cycles.go` on go1.27.1, it took the reproducer from 20 of 20 to 0 of 20 and
the test that failed here from about 3 in 40 to 0 in 40. But it expands the
instance only to reach an answer already known. Unpacking every named type
is wrong: it expands instances whose origin is still being checked, and
`TestFixedbugs/issue57522.go` panics with "nil typ" in `subst`.

**How we found it**: the race gate of a release rehearsal
(`.github/workflows/race.yml`, called by `release.yml`) failed in
`internal/testutil/modelcorpus`, and the report named two go/packages workers.
Reproducing it outside x/tools showed the read is in go/types and that
serializing the checkers is the only lever this side of the toolchain.
