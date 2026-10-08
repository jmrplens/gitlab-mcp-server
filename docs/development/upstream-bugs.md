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

## Contents

- [Writing an entry](#writing-an-entry)
- [What does not belong here](#what-does-not-belong-here)
- [What each entry records](#what-each-entry-records)
- [Summary](#summary)
- [GitLab client (`gitlab.com/gitlab-org/api/client-go`)](#gitlab-client-gitlabcomgitlab-orgapiclient-go)
  - [Panic unmarshalling an issue with no id](#panic-unmarshalling-an-issue-with-no-id)
  - [UpdateIssueBoardList cannot decode a successful response](#updateissueboardlist-cannot-decode-a-successful-response)
  - [GetNamespace cannot decode a path-based lookup](#getnamespace-cannot-decode-a-path-based-lookup)
  - [SetFeatureFlagOptions fields lack omitempty](#setfeatureflagoptions-fields-lack-omitempty)
  - [ApplicationStatistics assumes numeric JSON](#applicationstatistics-assumes-numeric-json)
  - [The security attribute and category mutations discard GraphQL errors](#the-security-attribute-and-category-mutations-discard-graphql-errors)
  - [The Dependency Firewall wrapper is missing an attribute and an endpoint](#the-dependency-firewall-wrapper-is-missing-an-attribute-and-an-endpoint)
  - [Enum constants lag the documented value sets](#enum-constants-lag-the-documented-value-sets)
  - [CreateProjectForkRelation declares a response GitLab does not send](#createprojectforkrelation-declares-a-response-gitlab-does-not-send)
  - [The invitations wrapper is missing two parameters and a response field](#the-invitations-wrapper-is-missing-two-parameters-and-a-response-field)
  - [The achievements fragments select less than the schema offers](#the-achievements-fragments-select-less-than-the-schema-offers)
  - [The epics wrapper is missing two filters and twelve response fields](#the-epics-wrapper-is-missing-two-filters-and-twelve-response-fields)
  - [The note and discussion structs miss what GitLab sends and declare what it does not](#the-note-and-discussion-structs-miss-what-gitlab-sends-and-declare-what-it-does-not)
  - [The member structs, options and services miss what GitLab sends, accepts and serves](#the-member-structs-options-and-services-miss-what-gitlab-sends-accepts-and-serves)
  - [Six response structs miss a field GitLab sends on every object](#six-response-structs-miss-a-field-gitlab-sends-on-every-object)
  - [No token struct carries the granular fields, and the impersonation and resource ones carry less still](#no-token-struct-carries-the-granular-fields-and-the-impersonation-and-resource-ones-carry-less-still)
  - [The four Sidekiq routes carry a leading slash and send a double slash](#the-four-sidekiq-routes-carry-a-leading-slash-and-send-a-double-slash)
  - [Response structs that miss a field GitLab sends unconditionally](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  - [Ten modelled fields that no Grape entity exposes, removed from this server's output](#ten-modelled-fields-that-no-grape-entity-exposes-removed-from-this-servers-output)
  - [The Geo structs model a fraction of a site and its status, and the repair method names the wrong entity](#the-geo-structs-model-a-fraction-of-a-site-and-its-status-and-the-repair-method-names-the-wrong-entity)
  - [The merge request structs miss six keys, unevenly, and two methods name an entity they do not answer with](#the-merge-request-structs-miss-six-keys-unevenly-and-two-methods-name-an-entity-they-do-not-answer-with)
  - [The User struct models one user entity and GitLab serves six](#the-user-struct-models-one-user-entity-and-gitlab-serves-six)
  - [IssueRelation models an issue basic where GitLab renders a whole issue](#issuerelation-models-an-issue-basic-where-gitlab-renders-a-whole-issue)
  - [MemberRole models twenty of the forty-five permissions GitLab sends](#memberrole-models-twenty-of-the-forty-five-permissions-gitlab-sends)
  - [PipelineInfo decodes two entities and models only the smaller one](#pipelineinfo-decodes-two-entities-and-models-only-the-smaller-one)
  - [Group, Project and Issue each model one entity where GitLab renders two](#group-project-and-issue-each-model-one-entity-where-gitlab-renders-two)
  - [The work item get, create and update documents select licensed fields](#the-work-item-get-create-and-update-documents-select-licensed-fields)
  - [A WithOptions delegation sends null as the request body](#a-withoptions-delegation-sends-null-as-the-request-body)
  - [UpdatePackageProtectionRulesOptions sends two explicit nulls on every partial update](#updatepackageprotectionrulesoptions-sends-two-explicit-nulls-on-every-partial-update)
  - [Seven more option structs send an optional param on every call](#seven-more-option-structs-send-an-optional-param-on-every-call)
  - [Five response keys and three parameters GitLab 19.4 added that v3.14.0 does not model](#five-response-keys-and-three-parameters-gitlab-194-added-that-v3140-does-not-model)
  - [PlanLimit models eight of the twenty-nine limits GitLab sends and accepts](#planlimit-models-eight-of-the-twenty-nine-limits-gitlab-sends-and-accepts)
  - [JobPipeline models five of the ten keys a job's pipeline carries](#jobpipeline-models-five-of-the-ten-keys-a-jobs-pipeline-carries)
  - [AwardEmoji does not model the image URL of a custom emoji](#awardemoji-does-not-model-the-image-url-of-a-custom-emoji)
  - [Diff does not model why a file diff arrives without its text](#diff-does-not-model-why-a-file-diff-arrives-without-its-text)
  - [ApproveOrRejectProjectDeployment discards the approval GitLab records](#approveorrejectprojectdeployment-discards-the-approval-gitlab-records)
  - [ShareProjectWithGroup discards the link GitLab creates](#shareprojectwithgroup-discards-the-link-gitlab-creates)
  - [GroupRelationStatus does not model the object count, and a relation's status does not decode](#grouprelationstatus-does-not-model-the-object-count-and-a-relations-status-does-not-decode)
  - [Commit declares extended_trailers a map of strings, and GitLab sends lists](#commit-declares-extended_trailers-a-map-of-strings-and-gitlab-sends-lists)
  - [The Orbit schema format is sent as `format`, and its llm answer is not modelled](#the-orbit-schema-format-is-sent-as-format-and-its-llm-answer-is-not-modelled)
  - [OrbitGraphStatusProjects does not model the projects the indexer gave up on](#orbitgraphstatusprojects-does-not-model-the-projects-the-indexer-gave-up-on)
  - [GroupMilestone does not model the milestone's web URL](#groupmilestone-does-not-model-the-milestones-web-url)
  - [CreateGroupIssueBoardListOptions models only label_id where GitLab takes four list types](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types)
  - [UpdateGroupIssueBoardOptions models neither list switch the group board update takes](#updategroupissueboardoptions-models-neither-list-switch-the-group-board-update-takes)
  - [The board structs miss keys GitLab sends on a board and its lists](#the-board-structs-miss-keys-gitlab-sends-on-a-board-and-its-lists)
  - [No client-go helper returns the RFC 6750 fields of a token refusal](#no-client-go-helper-returns-the-rfc-6750-fields-of-a-token-refusal)
- [MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)](#mcp-go-sdk-githubcommodelcontextprotocolgo-sdk)
  - [No keep-alive interval for SSE streams on StreamableHTTPOptions](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions)
  - [A malformed message ends the session instead of answering -32700](#a-malformed-message-ends-the-session-instead-of-answering--32700)
  - [A cancelled incoming call is still answered](#a-cancelled-incoming-call-is-still-answered)
  - [The cancellation reason is discarded before any handler sees it](#the-cancellation-reason-is-discarded-before-any-handler-sees-it)
  - [The declared protocol version, not the negotiated one, selects MRTR](#the-declared-protocol-version-not-the-negotiated-one-selects-mrtr)
  - [Three methods are served on a legacy session before the initialize handshake](#three-methods-are-served-on-a-legacy-session-before-the-initialize-handshake)
  - [The negotiated protocol version is recorded on one path of four](#the-negotiated-protocol-version-is-recorded-on-one-path-of-four)
  - [Application code cannot send notifications/cancelled for a listen stream](#application-code-cannot-send-notificationscancelled-for-a-listen-stream)
  - [`Mcp-Name` is compared without decoding the base64 sentinel](#mcp-name-is-compared-without-decoding-the-base64-sentinel)
  - [A receiving middleware cannot read the JSON-RPC request id](#a-receiving-middleware-cannot-read-the-json-rpc-request-id)
  - [A middleware cannot ask whether a request carries params](#a-middleware-cannot-ask-whether-a-request-carries-params)
  - [The protocol version is classified by string ordering](#the-protocol-version-is-classified-by-string-ordering)
  - [A resource update cannot be delivered to one session](#a-resource-update-cannot-be-delivered-to-one-session)
  - [A session's second listen on a URI overwrites the first's subscription, and its close deletes both](#a-sessions-second-listen-on-a-uri-overwrites-the-firsts-subscription-and-its-close-deletes-both)
  - [A tool, prompt or resource result a middleware makes carries no `resultType`](#a-tool-prompt-or-resource-result-a-middleware-makes-carries-no-resulttype)
  - [A Go SDK client never sees a `subscriptions/listen` refusal](#a-go-sdk-client-never-sees-a-subscriptionslisten-refusal)
  - [The Go SDK client starts no new session after a 404](#the-go-sdk-client-starts-no-new-session-after-a-404)
- [OpenAI Codex (`openai/codex`)](#openai-codex-openaicodex)
  - [A non-integer annotation priority breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call)
- [GitLab (`gitlab-org/gitlab`)](#gitlab-gitlab-orggitlab)
  - [No endpoint reports the instance plan to a non-administrator](#no-endpoint-reports-the-instance-plan-to-a-non-administrator)
  - [403 responses carry no WWW-Authenticate header](#403-responses-carry-no-www-authenticate-header)
  - [No resource_indicators_supported in authorization-server metadata](#no-resource_indicators_supported-in-authorization-server-metadata)
  - [The merge request approvals GET answers 24 keys on EE under a four-key annotation](#the-merge-request-approvals-get-answers-24-keys-on-ee-under-a-four-key-annotation)
  - [Two project group listings are annotated with the whole Group entity](#two-project-group-listings-are-annotated-with-the-whole-group-entity)
  - [The context commit list is annotated with Commit and presents CommitWithLink](#the-context-commit-list-is-annotated-with-commit-and-presents-commitwithlink)
  - [Three job token scope endpoints declare a response entity they do not send](#three-job-token-scope-endpoints-declare-a-response-entity-they-do-not-send)
  - [Cancelling an auto-merge answers a status hash under a merge request annotation](#cancelling-an-auto-merge-answers-a-status-hash-under-a-merge-request-annotation)
  - [The REST API page does not say a non-GET request to a moved project's old path is answered 405](#the-rest-api-page-does-not-say-a-non-get-request-to-a-moved-projects-old-path-is-answered-405)
  - [A revoked GPG UID is still offered for verification and still verifies commits](#a-revoked-gpg-uid-is-still-offered-for-verification-and-still-verifies-commits)
  - [The transfer API pages do not say the answer precedes the move, or how a failure is reported](#the-transfer-api-pages-do-not-say-the-answer-precedes-the-move-or-how-a-failure-is-reported)
  - [A saved view create or subscribe from a token answers 500, and the create has already saved the view](#a-saved-view-create-or-subscribe-from-a-token-answers-500-and-the-create-has-already-saved-the-view)
  - [The Orbit API page's query examples predate version 12 of the query DSL](#the-orbit-api-pages-query-examples-predate-version-12-of-the-query-dsl)
  - [An unknown severity on a pipeline's findings list answers 500](#an-unknown-severity-on-a-pipelines-findings-list-answers-500)
  - [An unknown report type on a pipeline's findings list is dropped and filters out every finding](#an-unknown-report-type-on-a-pipelines-findings-list-is-dropped-and-filters-out-every-finding)
  - [The scan profile attach mutation drops the reason it refused a name](#the-scan-profile-attach-mutation-drops-the-reason-it-refused-a-name)
  - [A permission refusal is answered 401 rather than 403](#a-permission-refusal-is-answered-401-rather-than-403)
  - [Deleting an external status check without the role answers 204 and deletes nothing](#deleting-an-external-status-check-without-the-role-answers-204-and-deletes-nothing)
  - [Creating an external status check without the role answers 500](#creating-an-external-status-check-without-the-role-answers-500)
  - [The admin token route takes no granular scopes, and no client-go create option carries them](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them)
  - [A token's own description omits its granular scopes](#a-tokens-own-description-omits-its-granular-scopes)
  - [The fine-grained refusal names the missing permissions only as display labels in prose](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose)
  - [The fine-grained refusal can name a deprecated permission's label](#the-fine-grained-refusal-can-name-a-deprecated-permissions-label)
  - [`available_for_permission` ignores `available_for`](#available_for_permission-ignores-available_for)
  - [GraphQL types and mutations this server reaches declare no fine-grained permission](#graphql-types-and-mutations-this-server-reaches-declare-no-fine-grained-permission)
  - [A declared mutation whose payload type declares nothing commits the write and answers null](#a-declared-mutation-whose-payload-type-declares-nothing-commits-the-write-and-answers-null)
  - [WorkItem declares the project boundary only, so a group's work item is null to a fine-grained token](#workitem-declares-the-project-boundary-only-so-a-groups-work-item-is-null-to-a-fine-grained-token)
  - [The pending-permission check exempts every type named `*Edge` or `*Payload`](#the-pending-permission-check-exempts-every-type-named-edge-or-payload)
  - [A board name GitLab cannot save is answered as a success](#a-board-name-gitlab-cannot-save-is-answered-as-a-success)
  - [The roles and permissions page does not say when a Guest or a Planner can view pipelines and merge requests](#the-roles-and-permissions-page-does-not-say-when-a-guest-or-a-planner-can-view-pipelines-and-merge-requests)
  - [The roles and permissions page gives Guest, Planner and Reporter the pipeline security report](#the-roles-and-permissions-page-gives-guest-planner-and-reporter-the-pipeline-security-report)
- [GitLab Orbit (`gitlab-org/orbit/knowledge-graph`)](#gitlab-orbit-gitlab-orgorbitknowledge-graph)
  - [The DSL schema says a path query may omit `rel_types`](#the-dsl-schema-says-a-path-query-may-omit-rel_types)
  - [The DSL schema says the default neighbors direction is `both`](#the-dsl-schema-says-the-default-neighbors-direction-is-both)
- [Other](#other)
  - [go-selfupdate depends on the deprecated x/crypto/openpgp](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp)
  - [gobco type-checks every file of a package directory, whatever its build constraints say](#gobco-type-checks-every-file-of-a-package-directory-whatever-its-build-constraints-say)
  - [gobco cannot instrument a package whose export_test.go feeds its external test package](#gobco-cannot-instrument-a-package-whose-export_testgo-feeds-its-external-test-package)
  - [go/types reads an imported generic instance another checker is expanding](#gotypes-reads-an-imported-generic-instance-another-checker-is-expanding)

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

That stays readable as a mention and resolves from either forge. It does not
keep the linked item from hearing about it. A link in a pull request body, an
issue body or a commit message still adds a cross-referenced or referenced
event to the GitHub issue it names, and the GitLab mirror adds a "mentioned in
commit" note to a gitlab.com issue or merge request that a commit message
names. Read on 2026-10-05:
[golang/go#81122](https://github.com/golang/go/issues/81122) carries
cross-referenced events from
[pull request 1083](https://github.com/jmrplens/gitlab-mcp-server/pull/1083),
[pull request 1122](https://github.com/jmrplens/gitlab-mcp-server/pull/1122)
and [pull request 1127](https://github.com/jmrplens/gitlab-mcp-server/pull/1127),
whose bodies name it only through that link, and
[gitlab-org/gitlab#630541](https://gitlab.com/gitlab-org/gitlab/-/issues/630541)
gained a "mentioned in commit" note from this repository's commit `072520b0`,
whose message names it through the same link form. So in this repository's
commit messages and pull request or issue bodies, name the register row rather
than the upstream item whenever the text is about our side (a workaround, a
refresh of this file): each mention is an entry in a timeline the upstream
reviewers read. Inside a committed file, this one included, the link form
stays, since the content of a file adds no such event.

**A new entry goes in three places**: its section under the project it belongs
to, or under [Other](#other), a row in the [Summary](#summary) table, and a
line in the [Contents](#contents) list under the same heading. markdownlint
checks that every link in the table and the list reaches a heading, not that
every heading is listed.

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
| 8 | go-sdk | [No SSE keep-alive option](#no-keep-alive-interval-for-sse-streams-on-streamablehttpoptions) | Yes, [modelcontextprotocol/go-sdk#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262) | Yes, theirs, [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232), merged; ours, [modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293), open | Partly, by [modelcontextprotocol/go-sdk#1232](https://github.com/modelcontextprotocol/go-sdk/pull/1232), not ours, unreleased | No | Yes |
| 9 | go-sdk | [A malformed message ends the session](#a-malformed-message-ends-the-session-instead-of-answering--32700) | Yes, by another user, [modelcontextprotocol/go-sdk#1209](https://github.com/modelcontextprotocol/go-sdk/issues/1209) | Yes, theirs, [modelcontextprotocol/go-sdk#1210](https://github.com/modelcontextprotocol/go-sdk/pull/1210), open | No | Was yes | Yes |
| 10 | go-sdk | [Cannot send `notifications/cancelled` for a listen stream](#application-code-cannot-send-notificationscancelled-for-a-listen-stream) | Yes, [modelcontextprotocol/go-sdk#1263](https://github.com/modelcontextprotocol/go-sdk/issues/1263) | No, proposal first | No | No | None possible |
| 11 | go-sdk | [Declared, not negotiated, version selects MRTR](#the-declared-protocol-version-not-the-negotiated-one-selects-mrtr) | Yes, [modelcontextprotocol/go-sdk#1258](https://github.com/modelcontextprotocol/go-sdk/issues/1258) | Yes, [modelcontextprotocol/go-sdk#1266](https://github.com/modelcontextprotocol/go-sdk/pull/1266), open | No | No | None taken |
| 12 | go-sdk | [A cancelled call is still answered](#a-cancelled-incoming-call-is-still-answered) | Yes, [modelcontextprotocol/go-sdk#1259](https://github.com/modelcontextprotocol/go-sdk/issues/1259) | Yes, [modelcontextprotocol/go-sdk#1267](https://github.com/modelcontextprotocol/go-sdk/pull/1267), open | No | No | Partial |
| 13 | go-sdk | [The cancellation reason is discarded](#the-cancellation-reason-is-discarded-before-any-handler-sees-it) | Yes, [modelcontextprotocol/go-sdk#1254](https://github.com/modelcontextprotocol/go-sdk/issues/1254) | Yes, [modelcontextprotocol/go-sdk#1255](https://github.com/modelcontextprotocol/go-sdk/pull/1255), merged | **Yes, unreleased** | No | Yes, until it ships |
| 14 | go-sdk | [`Mcp-Name` compared without decoding](#mcp-name-is-compared-without-decoding-the-base64-sentinel) | Yes, by another user, [modelcontextprotocol/go-sdk#1234](https://github.com/modelcontextprotocol/go-sdk/issues/1234) | Yes, theirs, [modelcontextprotocol/go-sdk#1242](https://github.com/modelcontextprotocol/go-sdk/pull/1242), merged | **Yes, unreleased** | No | None taken |
| 15 | go-sdk | [Protocol version classified by string ordering](#the-protocol-version-is-classified-by-string-ordering) | Yes, [modelcontextprotocol/go-sdk#1260](https://github.com/modelcontextprotocol/go-sdk/issues/1260) | Yes, [modelcontextprotocol/go-sdk#1268](https://github.com/modelcontextprotocol/go-sdk/pull/1268), merged | **Yes, unreleased** | No | None taken |
| 16 | go-selfupdate | [Deprecated `x/crypto/openpgp`](#go-selfupdate-depends-on-the-deprecated-xcryptoopenpgp) | Yes | Yes, [creativeprojects/go-selfupdate#58](https://github.com/creativeprojects/go-selfupdate/pull/58), open | No | No | Retired |
| 17 | codex | [Non-integer `priority` breaks a tool call](#a-non-integer-annotation-priority-breaks-a-tool-call) | Yes, [openai/codex#38979](https://github.com/openai/codex/issues/38979), and the cause in rmcp, [modelcontextprotocol/rust-sdk#1299](https://github.com/modelcontextprotocol/rust-sdk/issues/1299) | Yes, [modelcontextprotocol/rust-sdk#1300](https://github.com/modelcontextprotocol/rust-sdk/pull/1300), merged | **Yes, rmcp 3.5.0**; Codex's `main` pins 3.3.0 since 2026-09-30, which predates it, and so does its newest release, 0.161.0 | Was yes | Yes, until a Codex built on the fix is widely deployed, not merely released |
| 18 | go-sdk | [A receiving middleware cannot read the JSON-RPC id](#a-receiving-middleware-cannot-read-the-json-rpc-request-id) | Yes, [modelcontextprotocol/go-sdk#1264](https://github.com/modelcontextprotocol/go-sdk/issues/1264) | No, proposal first | No | No | None possible |
| 19 | client-go | [Security mutations discard GraphQL errors](#the-security-attribute-and-category-mutations-discard-graphql-errors) | Yes | Yes, [gitlab-org/api/client-go!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066), open | No | No | Yes |
| 20 | client-go | [Dependency Firewall lacks `operation` and the enablement endpoint](#the-dependency-firewall-wrapper-is-missing-an-attribute-and-an-endpoint) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | None |
| 21 | go-sdk | [A middleware cannot ask whether a request carries params](#a-middleware-cannot-ask-whether-a-request-carries-params) | Yes, [modelcontextprotocol/go-sdk#1261](https://github.com/modelcontextprotocol/go-sdk/issues/1261) | Yes, [modelcontextprotocol/go-sdk#1269](https://github.com/modelcontextprotocol/go-sdk/pull/1269), merged | **Yes, unreleased** | No | Yes |
| 22 | client-go | [Enum constants lag the documented value sets](#enum-constants-lag-the-documented-value-sets) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 23 | go-sdk | [No per-session resource-updated delivery](#a-resource-update-cannot-be-delivered-to-one-session) | Yes, [modelcontextprotocol/go-sdk#1265](https://github.com/modelcontextprotocol/go-sdk/issues/1265) | No, proposal first | No | No | Yes |
| 24 | gitlab-org/gitlab | [Approvals GET answers 24 keys on EE under a four-key annotation](#the-merge-request-approvals-get-answers-24-keys-on-ee-under-a-four-key-annotation) | Yes, by the merge request; in part before it, by GitLab, [gitlab-org/gitlab#408183](https://gitlab.com/gitlab-org/gitlab/-/issues/408183) | Yes, [gitlab-org/gitlab!259766](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259766), open, the page only | No | No upstream block; this server publishes what each edition sends on the GET and both POSTs | Yes, a shape declaration per EE key against the CE annotation; the carve-outs were the defect |
| 25 | client-go | [`CreateProjectForkRelation` declares a response GitLab does not send](#createprojectforkrelation-declares-a-response-gitlab-does-not-send) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 26 | client-go | [The invitations wrapper is missing two parameters and a response field](#the-invitations-wrapper-is-missing-two-parameters-and-a-response-field) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 27 | client-go | [The achievements fragments select less than the schema offers](#the-achievements-fragments-select-less-than-the-schema-offers) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | None possible |
| 28 | client-go | [The epics wrapper is missing two filters and twelve response fields](#the-epics-wrapper-is-missing-two-filters-and-twelve-response-fields) | Yes | Yes, in part, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | Was yes | Yes |
| 29 | client-go | [The note and discussion structs miss what GitLab sends and declare what it does not](#the-note-and-discussion-structs-miss-what-gitlab-sends-and-declare-what-it-does-not) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 30 | client-go | [The member structs, options and services miss what GitLab sends, accepts and serves](#the-member-structs-options-and-services-miss-what-gitlab-sends-accepts-and-serves) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 31 | client-go | [Six response structs miss a field GitLab sends on every object](#six-response-structs-miss-a-field-gitlab-sends-on-every-object) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 32 | client-go | [No token struct carries the granular fields, and the impersonation and resource ones carry less still](#no-token-struct-carries-the-granular-fields-and-the-impersonation-and-resource-ones-carry-less-still) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 33 | client-go | [The four Sidekiq routes carry a leading slash](#the-four-sidekiq-routes-carry-a-leading-slash-and-send-a-double-slash) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | None |
| 34 | client-go | [Response structs that miss a field GitLab sends unconditionally](#response-structs-that-miss-a-field-gitlab-sends-unconditionally) | Yes | Yes, the gaps held back in [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | **14 of 14; all released, v3.1.0 to v3.15.0** | No | Retired for all 14, the last, `systemhooks`, at the v3.15.0 pin |
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
| 46 | gitlab-org/gitlab | [Cancelling an auto-merge answers a status hash under a merge request annotation](#cancelling-an-auto-merge-answers-a-status-hash-under-a-merge-request-annotation) | Yes | Yes, [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704), both merged; [gitlab-org/gitlab!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239) closed unmerged | **Yes, unreleased**: both halves, in milestone 19.5 | Was yes | Yes |
| 47 | gitlab-org/gitlab | [A revoked GPG UID still verifies commits](#a-revoked-gpg-uid-is-still-offered-for-verification-and-still-verifies-commits) | Yes, by another user, [gitlab-org/gitlab#24572](https://gitlab.com/gitlab-org/gitlab/-/work_items/24572) | Yes, [gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300), merged | **Yes, unreleased** | No | None possible |
| 48 | go-sdk | [Two listens on one URI leave a session receiving neither](#a-sessions-second-listen-on-a-uri-overwrites-the-firsts-subscription-and-its-close-deletes-both) | No issue; named as a known limitation of [modelcontextprotocol/go-sdk#1275](https://github.com/modelcontextprotocol/go-sdk/pull/1275) by another user | No | No | No | Partial |
| 49 | go-sdk | [Three methods served before the initialize handshake](#three-methods-are-served-on-a-legacy-session-before-the-initialize-handshake) | Yes, [modelcontextprotocol/go-sdk#1271](https://github.com/modelcontextprotocol/go-sdk/issues/1271) | Yes, [modelcontextprotocol/go-sdk#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273), merged | **Yes, unreleased** | No | None taken |
| 50 | go-sdk | [The negotiated version is recorded on one path of four](#the-negotiated-protocol-version-is-recorded-on-one-path-of-four) | Yes, [modelcontextprotocol/go-sdk#1272](https://github.com/modelcontextprotocol/go-sdk/issues/1272) | Yes, [modelcontextprotocol/go-sdk#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274), merged | **Yes, unreleased** | No | None taken |
| 51 | client-go | [A WithOptions delegation sends `null` as the request body](#a-withoptions-delegation-sends-null-as-the-request-body) | Yes | Yes, [gitlab-org/api/client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065), open | No | No | Yes |
| 52 | client-go | [`UpdatePackageProtectionRulesOptions` lacks `omitempty`](#updatepackageprotectionrulesoptions-sends-two-explicit-nulls-on-every-partial-update) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | Partly | Partial |
| 53 | gitlab-org/gitlab | [No endpoint reports the instance plan to a non-administrator](#no-endpoint-reports-the-instance-plan-to-a-non-administrator) | Yes, [gitlab-org/gitlab#630305](https://gitlab.com/gitlab-org/gitlab/-/issues/630305) | Yes, [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936), open; the backend review approved it, and the backend maintainer's review of 2026-10-06 holds it on a product decision asked of `group::entitlements` on the issue | No | No | Yes |
| 54 | client-go | [Seven more option structs send an optional param on every call](#seven-more-option-structs-send-an-optional-param-on-every-call) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No; one latent, one narrows an action | Not needed for five; two handlers require the field, one of them until the tag changes |
| 55 | gitlab-org/gitlab | [A permission refusal is answered 401 rather than 403](#a-permission-refusal-is-answered-401-rather-than-403) | No | No | No | No | Yes |
| 56 | gitlab-org/gitlab | [Deleting an external status check without the role answers 204 and deletes nothing](#deleting-an-external-status-check-without-the-role-answers-204-and-deletes-nothing) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486), open, with row 57 | No | No | Partial |
| 57 | gitlab-org/gitlab | [Creating an external status check without the role answers 500](#creating-an-external-status-check-without-the-role-answers-500) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486), open, with row 56 | No | No | Yes |
| 58 | client-go | [Five response keys and three parameters GitLab 19.4 added](#five-response-keys-and-three-parameters-gitlab-194-added-that-v3140-does-not-model) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 59 | client-go | [PlanLimit models eight of the twenty-nine limits GitLab sends and accepts](#planlimit-models-eight-of-the-twenty-nine-limits-gitlab-sends-and-accepts) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Partial |
| 60 | client-go | [JobPipeline models five of the ten keys a job's pipeline carries](#jobpipeline-models-five-of-the-ten-keys-a-jobs-pipeline-carries) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 61 | client-go | [`AwardEmoji` does not model the image URL of a custom emoji](#awardemoji-does-not-model-the-image-url-of-a-custom-emoji) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 62 | client-go | [`Diff` does not model why a file diff arrives without its text](#diff-does-not-model-why-a-file-diff-arrives-without-its-text) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 63 | client-go | [`ApproveOrRejectProjectDeployment` discards the approval GitLab records](#approveorrejectprojectdeployment-discards-the-approval-gitlab-records) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 64 | client-go | [`ShareProjectWithGroup` discards the link GitLab creates](#shareprojectwithgroup-discards-the-link-gitlab-creates) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 65 | client-go | [`GroupRelationStatus` misses the object count, and a relation's status does not decode](#grouprelationstatus-does-not-model-the-object-count-and-a-relations-status-does-not-decode) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 66 | go-sdk | [A tool, prompt or resource result a middleware makes carries no `resultType`](#a-tool-prompt-or-resource-result-a-middleware-makes-carries-no-resulttype) | Yes, by another user, [modelcontextprotocol/go-sdk#1225](https://github.com/modelcontextprotocol/go-sdk/issues/1225) | Yes, theirs, [modelcontextprotocol/go-sdk#1226](https://github.com/modelcontextprotocol/go-sdk/pull/1226), merged | **Yes, unreleased** | No; without the workaround it breaks a MUST | Yes, until the bump that carries it |
| 67 | go-sdk | [A Go SDK client never sees a listen refusal](#a-go-sdk-client-never-sees-a-subscriptionslisten-refusal) | Yes, by another user, [modelcontextprotocol/go-sdk#1169](https://github.com/modelcontextprotocol/go-sdk/issues/1169), closed as working as intended; the rest is the proposal [modelcontextprotocol/go-sdk#1284](https://github.com/modelcontextprotocol/go-sdk/issues/1284) | Yes, theirs, [modelcontextprotocol/go-sdk#1170](https://github.com/modelcontextprotocol/go-sdk/pull/1170), closed unmerged; and, for the second `Subscribe`, [modelcontextprotocol/go-sdk#1283](https://github.com/modelcontextprotocol/go-sdk/pull/1283), merged | Partly, by [modelcontextprotocol/go-sdk#1283](https://github.com/modelcontextprotocol/go-sdk/pull/1283), not ours, unreleased: the second `Subscribe` asks again; `Subscribe` still returns nil on a refusal | No | None possible |
| 68 | go-sdk | [The client starts no new session after a 404](#the-go-sdk-client-starts-no-new-session-after-a-404) | Yes, [modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299) | Yes, theirs, [modelcontextprotocol/go-sdk#1300](https://github.com/modelcontextprotocol/go-sdk/pull/1300), open | No | No | None taken |
| 69 | client-go | [Commit declares `extended_trailers` a map of strings, and GitLab sends lists](#commit-declares-extended_trailers-a-map-of-strings-and-gitlab-sends-lists) | No; an additive change is drafted and not sent | No | No | Was yes, for `repository.commit_list` with `trailers` | Yes for every action that publishes `extended_trailers`; not for the readers of an embedded commit, resources, prompts and completions included |
| 70 | client-go | [The Orbit schema format is sent as `format`, and its llm answer is not modelled](#the-orbit-schema-format-is-sent-as-format-and-its-llm-answer-is-not-modelled) | Yes | Yes, [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), open | No | No | Yes |
| 71 | gitlab-org/gitlab | [The transfer API pages do not say the answer precedes the move, or how a failure is reported](#the-transfer-api-pages-do-not-say-the-answer-precedes-the-move-or-how-a-failure-is-reported) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260143](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260143), open | No | No | Yes |
| 72 | gitlab-org/gitlab | [A saved view create or subscribe from a token answers 500, and the create has already saved the view](#a-saved-view-create-or-subscribe-from-a-token-answers-500-and-the-create-has-already-saved-the-view) | Yes, by the merge request | Yes, [gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074), merged | **Yes, unreleased** | Yes | Partial |
| 73 | gobco | [gobco type-checks every file of a package directory](#gobco-type-checks-every-file-of-a-package-directory-whatever-its-build-constraints-say) | Yes, [rillig/gobco#40](https://github.com/rillig/gobco/issues/40) | Yes, [rillig/gobco#41](https://github.com/rillig/gobco/pull/41), open | No | No; it keeps the condition gate from measuring the e2e harness | Partial |
| 74 | gitlab-org/gitlab | [The Orbit API page's query examples predate version 12 of the query DSL](#the-orbit-api-pages-query-examples-predate-version-12-of-the-query-dsl) | Yes, by the merge request | Yes, [gitlab-org/gitlab!258241](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258241), merged | **Yes, unreleased**: in milestone 19.5 | No | Yes, `orbit.query`'s own guidance and the site's Orbit page teach version 12, with [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031) |
| 75 | gitlab-org/orbit/knowledge-graph | [The DSL schema says a path query may omit `rel_types`](#the-dsl-schema-says-a-path-query-may-omit-rel_types) | Yes, [gitlab-org/orbit/knowledge-graph#1329](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1329), closed by the merge | Yes, [gitlab-org/orbit/knowledge-graph!2650](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2650), merged; the follow-up making the schema require it, [gitlab-org/orbit/knowledge-graph!2691](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2691), merged | **Yes, Orbit v0.137.0**, which GitLab.com served on 2026-10-05 | No | Not needed: GitLab.com serves the corrected schema, and `orbit.query`'s guidance states the rule too, with [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031) |
| 76 | gitlab-org/orbit/knowledge-graph | [The DSL schema says the default neighbors direction is `both`](#the-dsl-schema-says-the-default-neighbors-direction-is-both) | Yes, [gitlab-org/orbit/knowledge-graph#1330](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1330), closed by the merge | Yes, [gitlab-org/orbit/knowledge-graph!2651](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2651), merged | **Yes, Orbit v0.137.0**, which GitLab.com served on 2026-10-05 | No; a result could be silently incomplete until then | Not needed: GitLab.com serves the corrected schema, and `orbit.query`'s guidance says the default is `outgoing` too, with [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031) |
| 77 | gitlab-org/gitlab | [The context commit list is annotated with `Commit` and presents `CommitWithLink`](#the-context-commit-list-is-annotated-with-commit-and-presents-commitwithlink) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260487](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260487), open | No | No | Yes |
| 78 | gitlab-org/gitlab | [An unknown severity on a pipeline's findings list answers 500](#an-unknown-severity-on-a-pipelines-findings-list-answers-500) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260459](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260459), open, with row 79 | No | No | Yes |
| 79 | gitlab-org/gitlab | [An unknown report type on a pipeline's findings list is dropped and filters out every finding](#an-unknown-report-type-on-a-pipelines-findings-list-is-dropped-and-filters-out-every-finding) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260459](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260459), open, with row 78 | No | No | Yes |
| 80 | gitlab-org/gitlab | [The scan profile attach mutation drops the reason it refused a name](#the-scan-profile-attach-mutation-drops-the-reason-it-refused-a-name) | No | No | No | No | Yes |
| 81 | golang/go | [go/types reads an imported generic instance another checker is expanding](#gotypes-reads-an-imported-generic-instance-another-checker-is-expanding) | Yes, by another user, [golang/go#81122](https://github.com/golang/go/issues/81122) | Yes, [golang/go#81871](https://github.com/golang/go/pull/81871), imported as [go.dev/cl/841585](https://go.dev/cl/841585), open | No | No; without the workaround it fails race runs of the tooling tests at random | Yes |
| 82 | gitlab-org/gitlab, then client-go | [The admin token route takes no granular scopes, and no client-go create option carries them](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them) | GitLab half yes, by GitLab, [gitlab-org/gitlab#630541](https://gitlab.com/gitlab-org/gitlab/-/issues/630541); client-go half no | No; attempts by other users, [gitlab-org/gitlab!245585](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/245585) and [gitlab-org/api/client-go!2978](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2978), closed unmerged | No | No | None taken; the five token create actions send classic scopes only, tracked in [issue 1115](https://github.com/jmrplens/gitlab-mcp-server/issues/1115) |
| 83 | gitlab-org/gitlab | [The fine-grained refusal names the missing permissions only as display labels in prose](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose) | No, drafted in its section; goes before row 84 | No | No | No | None taken; the labels are not parsed, on purpose |
| 84 | client-go | [No client-go helper returns the RFC 6750 fields of a token refusal](#no-client-go-helper-returns-the-rfc-6750-fields-of-a-token-refusal) | No, drafted in its section; waits on this project deciding to adopt the helper | No | No | No | Not needed; this server decodes the body itself |
| 85 | gitlab-org/gitlab | [The fine-grained refusal can name a deprecated permission's label](#the-fine-grained-refusal-can-name-a-deprecated-permissions-label) | No, not yet reproduced on a running instance | No | No | No | None taken |
| 86 | gitlab-org/gitlab | [A token's own description omits its granular scopes](#a-tokens-own-description-omits-its-granular-scopes) | Yes, by GitLab, [gitlab-org/gitlab#629849](https://gitlab.com/gitlab-org/gitlab/-/issues/629849) | Yes, [gitlab-org/gitlab!259764](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259764), merged; its two follow-ups for the other token routes, [gitlab-org/gitlab!260276](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260276) and [gitlab-org/gitlab!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277), open | **Yes, unreleased**: merged on 2026-10-07, in milestone 19.5, and deployed to GitLab.com the same day | No | Yes, a second request by the token's id, until a release carries the merge |
| 87 | gitlab-org/gitlab | [GraphQL types and mutations this server reaches declare no fine-grained permission](#graphql-types-and-mutations-this-server-reaches-declare-no-fine-grained-permission) | GitLab tracks them on its own pending list, and plans part of them in [gitlab-org/gitlab#631631](https://gitlab.com/gitlab-org/gitlab/-/issues/631631); nothing raised by us | No, by us; GitLab's own changes for `WorkItemType` are open, and another contributor's for `CustomEmoji`, [gitlab-org/gitlab!260586](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260586) | No | Yes, for a fine-grained token: 37 actions withheld, 8 served with parts empty | Withheld with the reason, and a note on the parts; [issue 1054](https://github.com/jmrplens/gitlab-mcp-server/issues/1054) |
| 88 | gitlab-org/gitlab | [A declared mutation whose payload type declares nothing commits the write and answers null](#a-declared-mutation-whose-payload-type-declares-nothing-commits-the-write-and-answers-null) | No | No | No | Yes, for a fine-grained token, on 20 writes | Withheld with the reason |
| 89 | gitlab-org/gitlab | [WorkItem declares the project boundary only, so a group's work item is null to a fine-grained token](#workitem-declares-the-project-boundary-only-so-a-groups-work-item-is-null-to-a-fine-grained-token) | Yes, by another user, [gitlab-org/gitlab#630483](https://gitlab.com/gitlab-org/gitlab/-/issues/630483) | Yes, [gitlab-org/gitlab!259765](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259765), open, closing that issue | No | Yes, for a fine-grained token, on the epic actions | Withheld with the reason |
| 90 | gitlab-org/gitlab | [The pending-permission check exempts every type named `*Edge` or `*Payload`](#the-pending-permission-check-exempts-every-type-named-edge-or-payload) | No | No | No | No | Not needed; the live record computes the undeclared set itself |
| 91 | gitlab-org/gitlab | [`available_for_permission` ignores `available_for`](#available_for_permission-ignores-available_for) | No | No | No | No | Not needed; the live record names the first permission a token can be granted |
| 92 | gitlab-org/gitlab | [The REST API page does not say a non-GET request to a moved project's old path is answered 405](#the-rest-api-page-does-not-say-a-non-get-request-to-a-moved-projects-old-path-is-answered-405) | Yes, by the merge request | Yes, [gitlab-org/gitlab!259297](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259297), merged | **Yes, unreleased**: in milestone 19.5 | No | Not yet, with [issue 1133](https://github.com/jmrplens/gitlab-mcp-server/issues/1133) |
| 93 | client-go | [`OrbitGraphStatusProjects` does not model the projects the indexer gave up on](#orbitgraphstatusprojects-does-not-model-the-projects-the-indexer-gave-up-on) | No | No | No | No | Yes |
| 94 | gitlab-org/gitlab | [A board name GitLab cannot save is answered as a success](#a-board-name-gitlab-cannot-save-is-answered-as-a-success) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260458](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260458), open | No | No | None taken |
| 95 | client-go | [`GroupMilestone` does not model the milestone's web URL](#groupmilestone-does-not-model-the-milestones-web-url) | No | No | No | No | Yes |
| 96 | client-go | [`CreateGroupIssueBoardListOptions` models only `label_id` where GitLab takes four list types](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types) | No | No | No | No; it narrows `group.group_board_create_list` to label lists | None, by decision |
| 97 | gobco | [gobco cannot instrument a package whose `export_test.go` feeds its external test package](#gobco-cannot-instrument-a-package-whose-export_testgo-feeds-its-external-test-package) | No; a fix and a pull request are decided, after the release row 73 waits on | No | No | No; the condition gate cannot measure a package it reaches | None, by decision |
| 98 | client-go | [`UpdateGroupIssueBoardOptions` models neither list switch the group board update takes](#updategroupissueboardoptions-models-neither-list-switch-the-group-board-update-takes) | No; it joins row 96 for the joint client-go merge request | No | No | No; a group board's Open and Closed lists cannot be hidden or shown through this server | Not yet, with [issue 1241](https://github.com/jmrplens/gitlab-mcp-server/issues/1241) |
| 99 | gitlab-org/gitlab | [The roles and permissions page does not say when a Guest or a Planner can view pipelines and merge requests](#the-roles-and-permissions-page-does-not-say-when-a-guest-or-a-planner-can-view-pipelines-and-merge-requests) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260353](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260353), open, in milestone 19.5, rebased onto row 100's merge and waiting for the technical writer's approval again | No | No | Not needed; `pipeline.latest`'s refusal names the role the policy names |
| 100 | gitlab-org/gitlab | [The roles and permissions page gives Guest, Planner and Reporter the pipeline security report](#the-roles-and-permissions-page-gives-guest-planner-and-reporter-the-pipeline-security-report) | Yes, by the merge request | Yes, [gitlab-org/gitlab!260471](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260471), merged | **Yes, unreleased**: in milestone 19.5 | No | Not needed; no hint of this server names who can view a pipeline's findings |
| 101 | client-go | [The board structs miss keys GitLab sends on a board and its lists](#the-board-structs-miss-keys-gitlab-sends-on-a-board-and-its-lists) | No; it joins rows 96 and 98 for the joint client-go merge request | No | No | No; the keys are read around the SDK | Partial; six requests of our own and the captured response, and `limit_metric` is missing from six project board answers |

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

Re-verified on 2026-10-02 against the trackers and the tags: every merge
request, pull request and issue the file links, the tags of each dependency,
and every merge request and issue this project's accounts touched upstream on
2026-09-30, 2026-10-01 and 2026-10-02, read from the account's event list so
that nothing done there is missing here. Two rows moved furthest.
[gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)
merged at 12:25 UTC on 2026-10-02, so both halves of row 46 are merged and
unreleased, and
[gitlab-org/orbit/knowledge-graph!2650](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2650)
merged at 10:15 UTC the same day and closed row 75's issue, with the schema
change its review suggested opened afterwards as a merge request of its own.
Row 92 is new: a GitLab Duo finding on row 46's merge request turned out to be
a refusal the REST API page did not describe, and the merge request documenting
it was opened and merged the same day. It is numbered 92 because rows 86 to 91
are written on the branches for
[issue 952](https://github.com/jmrplens/gitlab-mcp-server/issues/952) and
arrive with them. Row 17 moved a step without moving its
state: Codex's `main` pins rmcp 3.3.0 since 2026-09-30, a release that predates
the fix. Rows 19, 34, 47, 51, 53, 68, 74 and 76 each moved a step, recorded in
their sections: the client-go merge request that row 19's section proposed
merged and the project's integration job runs its 132 tests again; row 34's
deploy key documentation merge request had its technical review, and a second
merge request took the two older problems on the page that it leaves alone; a
documentation change by one of row 47's approvers merged;
row 51's merge request had its first review and its changes are in; row 53's
was approved by its backend reviewer; row 68's pull request had a review from
another contributor; row 74's was approved by its writer and by the Orbit
team; and row 76's was rebased, handed to another reviewer, and asked by that
reviewer for one more rebase, which is owed from here. The go-sdk pull
requests of rows 8, 11 and 12 are nine commits behind `main` again, unchanged
since they were brought up to date on 2026-09-29. No release moved anything:
GitLab 19.4.1, client-go v3.15.0 and go-sdk v1.8.0 are still each project's
newest release, go-sdk's `main` is 39 commits ahead of v1.8.0, and the Orbit
knowledge graph's newest tag, v0.136.0 of 2026-09-30, predates its merge, so
every merge recorded here as unreleased still is. The Go change of row 81,
gobco, go-selfupdate and the client-go umbrella and 4.0 issues are where their
sections left them.

Rows 86 to 91 were added on 2026-10-02 for
[issue 952](https://github.com/jmrplens/gitlab-mcp-server/issues/952), which
reads a fine-grained personal access token's grant and judges every action
against what GitLab declares. They are what that work found in GitLab itself,
read from the `v19.4.1-ee` source (`26212baa`), the release the live record and
the permission table are taken from, and measured on a 19.4.1 instance by the
end-to-end suite's direct probes where a section says so. They follow rows 82 to
85, which the register gained on 2026-09-30 for the same issue, and none of them
repeats those four: creating such a token
([row 82](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them)),
the refusal naming the missing permissions only as labels in prose
([row 83](#the-fine-grained-refusal-names-the-missing-permissions-only-as-display-labels-in-prose)),
the client-go helper for the RFC 6750 fields of a refusal
([row 84](#no-client-go-helper-returns-the-rfc-6750-fields-of-a-token-refusal))
and the refusal naming a deprecated permission's label
([row 85](#the-fine-grained-refusal-can-name-a-deprecated-permissions-label)).
Nothing has been raised upstream for any of the six;
contributing the missing GraphQL declarations of rows 87 to 89 was
[issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055) until
2026-10-07, and is held by row 87's section since, and
serving over REST what GraphQL cannot serve a fine-grained token is
[issue 1054](https://github.com/jmrplens/gitlab-mcp-server/issues/1054).

Re-verified on 2026-10-04 against the trackers and the tags: every merge
request and pull request the file links, read for anything merged, closed or
reopened after 2026-10-02, and the tags of each dependency. One merged:
[gitlab-org/gitlab!259375](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259375),
the second of row 34's deploy key documentation merge requests, at 03:00 UTC on
2026-10-04, in milestone 19.5. Rows 74, 75 and 76 each moved a step, recorded
in their sections: row 74's merge request has no open thread left and waits
only on a maintainer, row 76's was rebased onto the `main` its reviewer named
the same evening it was asked for and now waits on that reviewer, and the
`lint:prose` regression rows 75 and 76 meet has a fix in review from another
contributor. No release moved anything: GitLab 19.4.1, client-go v3.15.0 and
go-sdk v1.8.0 are still each project's newest release, and the Orbit knowledge
graph's newest tag is still v0.136.0 of 2026-09-30, so every merge recorded
here as unreleased still is.

On 2026-10-05 the client-go pin moved from v3.14.0 to v3.15.0 by hand, a week
before the Dependabot cooldown for a Go minor would have proposed it, which
retires the last of row 34's workarounds: `systemhooks` reads the seven `Hook`
fields off client-go's struct and its capture is gone, as row 34's section
records. Between the two tags only `system_hooks.go`, its test and the
changelog differ, which is
[gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
and nothing else, so no struct, method or route another row names changed, and
every other `client-go` row reads as it did at v3.14.0.

Re-verified on 2026-10-05 against the trackers, the tags and Gerrit, for every
merge request, pull request, issue and change the file links and every item
the maintainer's accounts have open upstream, after a status review of all of
them; this file is brought to what that review found and to what was posted
upstream the same day. No release moved anything: go-sdk v1.8.0 is still that
SDK's newest tag, with `main` 42 commits ahead of it and no milestone for a
next version; client-go v3.15.0, GitLab 19.4.1 (no 19.5 tag yet) and the Orbit
knowledge graph's v0.136.0 are still each project's newest release, so every
merge recorded here as unreleased still is. The merges of rows 39, 46, 47, 72
and 92, which no release carries yet, are deployed on GitLab.com, which the
release tooling records with `workflow::production`:
[gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300)
on 2026-09-24,
[gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699)
on 2026-09-29,
[gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702)
and
[gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074)
on 2026-09-30, and
[gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)
and
[gitlab-org/gitlab!259297](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259297)
on 2026-10-05. Two merges moved rows: row 75's follow-up and the `lint:prose`
fix rows 75 and 76 waited on both merged in the Orbit knowledge graph, and row
76's merge request was rebased onto them and passes its fork pipeline. Posted
upstream the same day, each recorded in its row: the descriptions of row 11's
and row 12's pull requests corrected and a status note on the go-sdk index
issue (rows 8, 10, 11, 12, 18 and 23), a note on row 53's merge request, which
its reviewer answered by asking a backend maintainer for the review, a note on
row 17's Codex issue, and row 76's rebase with its note. Reminders are planned,
not posted: on row 74's merge request on 2026-10-06, on row 51's and on the
joint client-go merge request on 2026-10-08, and on row 81's Gerrit change not
before 2026-10-14. Readings that moved no state, each recorded in its row: row
9's pull request has stalled by the measure its section set, which leaves to
the maintainer whether its evidence goes upstream now; the umbrella issue of
row 34 is behind again in its documentation paragraph; row 73 says why the
gobco release its maintainer offered has not come; GitLab.com does not run
row 75's fixes yet and still serves Orbit 0.135.0; rows 85 and 89 read the
same on GitLab's `master`; and rows 39 and 72 record their deployment in
their sections as well. Rows corrected rather than moved: row 24 is
rewritten, since an EE build answers the GET it describes with 24 keys, and
the approve and unapprove POSTs that share its helper and its annotation with
the same entity, where the entry had taken the four-key CE answer for
GitLab's; row 86 is reported, by GitLab itself; row 81 counts the packages its
workaround reaches again and names two other projects that keep the same race
out of their gates; row 87 records a related blind spot GitLab has filed, and
row 83 the milestone of the rollout issue it cites; row 11 names the predicate
by its current name; row 17's sentences on
[issue 1043](https://github.com/jmrplens/gitlab-mcp-server/issues/1043) say
what is to be done rather than what is done; and the summary cells of rows 9,
13 and 16 link what they name. The rule in
[Writing an entry](#writing-an-entry) about cross-references is corrected too:
a full link renders right and still tells the linked item it was mentioned.

Read again later on 2026-10-05, for the change that closes
[issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031). Row
76's merge request merged at 16:21 UTC, and the Orbit knowledge graph cut
v0.137.0 at 17:08 UTC carrying it and both of row 75's merges; GitLab.com
serves that release (read at 21:06 UTC: Orbit 0.137.0, query DSL 12.1.10), so
rows 75 and 76 are merged, released and deployed. The same change settles the
Workaround cells of rows 74, 75 and 76, since `orbit.query` now teaches
version 12 of the DSL, and adds row 93, which the re-recording of the Orbit
response record found. Row 74's merge request reads as it did that morning.

Read on 2026-10-06 for the three rows whose merge requests to
`gitlab-org/gitlab` were opened from the community fork on the evening of the
5th, each readied for review with a green fork pipeline:
[gitlab-org/gitlab!259766](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259766),
the approvals page of row 24,
[gitlab-org/gitlab!259764](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259764),
the self route of row 86, and
[gitlab-org/gitlab!259765](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259765),
the group boundary of row 89. Each row's Reported and In review fields now say
so. Row 89 also names the issue another user had filed for the same defect,
which its merge request closes, and row 24 the other issue its merge request
cites; rows 86 and 89 carry a Fix in review paragraph where their
proposal was. None of the three has a review yet. Row 74's merge request
reads as it did on the 5th: both approvals, nothing open, not merged, and the
note asking for the merge not posted. No other row was read.

Read again late on 2026-10-06 for rows 24, 34, 53, 67, 71, 74, 82, 86 and 89,
on a day four of the changes they follow merged upstream. Row 74's merge
request merged at 09:59 UTC through the merge train, in milestone 19.5, after
the note asking for the merge went out that morning, and reached GitLab.com's
canary stage that evening. Row 67 moved furthest: a go-sdk maintainer closed
its issue and the first pull request on the 5th as working as intended,
leaving the visibility of a refusal to an open proposal, and on the 6th merged
the second pull request with a commit of his own. No go-sdk release carries
it; the entry says which half of the defect that release retires, and which
test to re-read at the bump that brings it in. Row 71's GitLab change merged
and its issue closed: from 19.5 a project's name or path collision is refused
before the answer, and a failed transfer's reason is served on its to-do item
over GraphQL alone, which
[issue 1222](https://github.com/jmrplens/gitlab-mcp-server/issues/1222) takes
up here; the documentation merge request the entry proposes is being
prepared and is not open, so the row still reads no. Row 82's section records
that GitLab's fix for the service account tokens merged, exempting bot users
from enforcement, while its issue stays open, and that the shared builder's
merge request waits on an authentication review. Rows 34, 53, 86 and 89 each
moved a step in review, recorded in their sections: row 34's deploy key merge
request was approved by its backend reviewer and waits on its writer; row
53's is held by its backend maintainer on a product decision, asked of
`group::entitlements` on the issue the same day, with both of his other
suggestions taken as commits; row 86's was reviewed, both suggestions were
answered with commits, and two follow-up issues were filed and assigned to
this project's maintainer; and row 89's commit now carries the `EE: true`
trailer and its page is approved. Row 24's merge request reads as it did that
morning. No release moved anything: GitLab 19.4.1, with no 19.5 tag yet, and
go-sdk v1.8.0 are still each project's newest release, so every merge
recorded here as unreleased still is.

Row 95 was added on 2026-10-07, for the change that closes
[issue 1169](https://github.com/jmrplens/gitlab-mcp-server/issues/1169), which
gives the group milestone resource the web URL the group milestone tools
already read from the captured response. The gap behind both had no row.
client-go's `main`, read the same day at `000e81ea` (2026-10-01), still
declares no such field, and no merge request or issue of that project
proposes one. No other row was read.

Rows 96 and 97 were added on 2026-10-07, when five issues of this repository
whose remaining work waited only on an upstream change were moved here and
closed, since an upstream finding is tracked in this register rather than as
an issue of our own. Each entry that replaces one names it, and now holds what
the issue knew: its measurements, the decisions recorded in its comments, and
what this server does on the day the upstream change lands.
[Issue 966](https://github.com/jmrplens/gitlab-mcp-server/issues/966) went to
rows 6, 52 and 54, the option fields client-go writes on every call;
[issue 1026](https://github.com/jmrplens/gitlab-mcp-server/issues/1026) to row
69, the trailer lists;
[issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055) to row
87, with its two side findings in row 86 and in row 87's section;
[issue 1099](https://github.com/jmrplens/gitlab-mcp-server/issues/1099) to row
73 and to row 97, the second gobco limit, which had no entry; and
[issue 1101](https://github.com/jmrplens/gitlab-mcp-server/issues/1101) to row
96, the group board list types client-go does not model, which had none
either. The upstream items those rows follow were read again the same day.
client-go v3.16.0, tagged at 00:42 UTC, carries a topics option, a dependency
update and row 19's CI change
([gitlab-org/api/client-go!3067](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3067)),
and none of the struct tags or fields these rows wait on; the joint merge
request has had no review activity since 2026-09-30. gobco has tagged nothing
since v1.3.4. The GitLab items of rows 86 to 89 were read as well, and one
moved: row 86's merge request merged at 05:39 UTC and reached GitLab.com's
canary stage; row 89's reads as it did on the 6th. No other row was read.

Read again on the evening of 2026-10-07, at 19:56 and 20:04 UTC, after eight
merge requests to `gitlab-org/gitlab` were opened from the community fork that
day and readied for review, the two follow-ups of row 86's among them. Rows
56, 57, 77, 78, 79 and 94 moved from no to in review:
[gitlab-org/gitlab!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486)
refuses the status check create and delete of rows 56 and 57 with 403 before
the service runs, which rewrites what those entries said would retire them;
[gitlab-org/gitlab!260487](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260487)
annotates the context commit list of row 77 with the entity it presents;
[gitlab-org/gitlab!260459](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260459)
refuses the unknown filter values of rows 78 and 79 in the resolver; and
[gitlab-org/gitlab!260458](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260458)
has the board routes of row 94 answer 400. Row 55 records that the first of
these leaves the update and list status check routes at 401. Rows 98 to 101
are new: the two list switches client-go's group board update options do not
model, which
[issue 1241](https://github.com/jmrplens/gitlab-mcp-server/issues/1241) needs,
and the keys its board structs leave out, which this server reads around them,
both joining row 96 for the joint client-go merge request; and two
corrections of GitLab's roles and permissions page,
[gitlab-org/gitlab!260353](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260353)
and
[gitlab-org/gitlab!260471](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260471),
both approved by their technical writer and set to milestone 19.5. Row 86's
merge request reached GitLab.com's production stage at 12:15 UTC, and its two
follow-ups,
[gitlab-org/gitlab!260276](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260276)
and
[gitlab-org/gitlab!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277),
have their documentation approval and wait on a backend reviewer; row 32
records what the second means for client-go's impersonation token struct. Two
of the eight need one more commit before a review can finish: row 99's, whose
Danger check refuses the short references in one commit message, and row
78's, whose fork pipeline asks for the GraphQL introspection results to be
regenerated. No other row was read, and no release moved anything: GitLab
19.4.1, with no 19.5 tag yet, and client-go v3.16.0 are still each project's
newest release.

Read once more at 21:40 UTC the same evening: the eight merge requests, and
those of rows 24, 71 and 89. Row 56's merge request had its fork pipeline go
green at 20:09 UTC and was moved back to `workflow::in dev` for want of a
reviewer at the same minute, and row 77's pipeline went green at 20:28. Row
100's was set to join the merge train at 19:30 UTC, which GitLab aborted at
19:34 when the technical writer's suggestion commit updated the branch, and
nothing has set it again. Row 89's was set at 19:33 UTC to join the merge
train once its two code-owner approvals land. Row 71's had been readied on
2026-10-06 at 20:31 UTC, not left unreadied as its row said, and its
technical writer requested changes at 16:23 UTC. Row 24's had a
documentation review at 20:47 UTC that also asks a subject matter expert to
verify the page. Row 86's follow-ups read as they did at 19:56, and the merge
requests of rows 78 and 79, 94 and 99 as they did at 20:04. No other row was
read, and GitLab 19.4.1 and client-go v3.16.0 are still each project's newest
release.

Re-verified on 2026-10-08, between 05:00 and 05:30 UTC, against the trackers,
the tags and Gerrit: every merge request, pull request, issue and change the
file links, the tags of each dependency, and every merge request the
maintainer's gitlab.com account touched since 2026-10-01, all read through
GET requests only. No release moved anything: client-go v3.16.0, go-sdk
v1.8.0 (with `main` 45 commits ahead), gobco v1.3.4 and GitLab 19.4.1 are
still each project's newest release, GitLab has no 19.5 tag, release
candidate or stable branch yet (its 19.5 milestone ends on 2026-10-09 and the
release is due on 2026-10-15) and GitLab.com reports `19.5.0-pre`, so every
merge recorded here as unreleased still is. The Orbit knowledge graph tagged
v0.137.1 and v0.138.0, and GitLab.com still serves 0.137.0. Codex's newest
stable release is now 0.161.0, built on rmcp 3.3.0 and so without row 17's
fix. One merge request of ours merged: row 100's, at 23:26 UTC on
2026-10-07, ahead of row 99's, which was rebased onto it at 08:03 UTC and
waits for its approval again. Row 82's
section records that GitLab's shared granular scope builder merged without
the authentication review it had waited on, which is the shape that row's
GitLab half now builds on; row 86's merge request was given milestone 19.5,
where its row and summary said it had none; and GitLab's fix for the bot
tokens of rows 82 to 84, row 74's merge request and row 71's GitLab change
reached GitLab.com's production stage. Rows 45 and 87 changed through other
people's work: the client-go draft row 45 said would collide with its commit
was closed as a proof of concept, and its replacement leaves the GraphQL
service alone, and another contributor opened a GitLab merge request that
declares `CustomEmoji`, one line of row 87's table. Late on 2026-10-07 most
of our open GitLab merge requests that waited on us or on a reviewer being
asked were answered or sent to one, each recorded in its row: rows 24 and 71
answered their reviews with commits and replies, rows 56 and 57, 77, 78 and
79, 86's two follow-ups, 89 and 94 had a backend reviewer requested, and rows
78 and 79 had the regenerated introspection results their pipeline asked
for. Three rows moved after their last reading, each true when it was read:
row 71's In review field listed what its merge request documented as it was
readied, and after the writer's review our commits that evening took three
of those things out; row 24's said none of its review comments had been
answered, as was so at 21:40 UTC; and row 17's called `rust-v0.160.0` the
newest stable Codex release, as it was when read on 2026-10-05. Rows
corrected rather than moved: row 12 never named another user's issue for the
same defect, closed on 2026-10-05 as a duplicate of ours; and row 34 said the
to-do item's `updated_at` was in no merge request without recording our
comment of 2026-10-05 proposing it on another contributor's, which the
client-go maintainer has since accepted. Readings that moved no state, each
recorded in its row: the go-sdk pull requests of rows 8, 9, 11, 12 and 68 are
three commits further behind `main`; the triage bot labelled the joint
client-go merge request and row 19's `idle`, and the reminders planned on it
and on row 51's for 2026-10-08 had not been posted when they were read; row
19's integration job ran its 132 tests on every `main` pipeline that ran it
after the failed one of 2026-10-03; GitLab's
pending GraphQL list lost one entry that none of row 87's lines is; row 32
notes that the impersonation token follow-up now has its backend review
requested; the GitLab merge request row 46's router job waited on is still
open; and row 75's follow-up had a question answered after its merge. Rows 1
to 5, 7, 10, 13 to 16, 18, 20 to 23, 25 to 31, 33, 35 to 44, 47 to 50, 52 to
55, 58 to 66, 69, 70, 72, 73, 76, 80, 81, 85, 88, 90 to 93, 95 to 98 and 101
were read and are unchanged. Rows 24, 45, 87 and 99 were read once more at
05:52 UTC: row 45's draft had a new commit, pushed at 05:38 and recorded in
its row.

## GitLab client (`gitlab.com/gitlab-org/api/client-go`)

### Panic unmarshalling an issue with no id

- **Reported**: yes.
- **In review**: yes,
  [gitlab-org/api/client-go!3006](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3006).
- **Merged**: **yes**, on 2026-08-25 into `main`, shipped in **v2.59.1**.
- **Blocking**: it was. The panic took the process down rather than failing one
  call.
- **Workaround**: retired. The local guard went with the move to v2.62.0, and
  every v3 tag carries the fix as well; the pin is now `client-go/v3` v3.15.0.

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
- **In review**: yes, open. The commit, `418162ec`, gives the eight optional
  fields (`key`, `feature_group`, `user`, `group`, `namespace`, `project`,
  `repository` and `force`) `omitempty` on both halves of the tag and pins
  that a field the caller set is still sent; `Value` keeps its tags, since
  GitLab requires it. Read on 2026-10-08 the merge request is open with 35
  commits, `418162ec` still the 32nd, and has had no review activity since
  2026-09-30; the triage bot labelled it `idle` at 00:20 UTC on 2026-10-08.
- **Merged**: no. The struct is unchanged at v3.15.0, the version `go.mod`
  pins, and at v3.16.0, tagged on 2026-10-07 (`feature_flags.go:115-125`).
- **Blocking**: no.
- **Workaround**: yes, in two places. `internal/tools/features.Set` builds the
  request body itself, and so does `setFeature` in
  `test/e2e/internal/fixture/feature.go`, which pins a flag as a scenario's
  precondition. Both retire at the client-go bump to the release that
  carries the commit, when each can call `Features.SetFeatureFlag` again. No
  test goes red on that bump, since neither path sends the struct, so this
  entry is what triggers the change. Until
  [issue 966](https://github.com/jmrplens/gitlab-mcp-server/issues/966)
  closed on 2026-10-07, that issue tracked the retirement of both beside the
  eleven always-sent findings of
  [entries 52](#updatepackageprotectionrulesoptions-sends-two-explicit-nulls-on-every-partial-update)
  and [54](#seven-more-option-structs-send-an-optional-param-on-every-call);
  entry 54 now holds what closes all three.

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
  proposed the one-line fix, passing the same token under both names, and
  @PatrickRice merged it at 15:01 UTC on 2026-09-30 (merge commit
  `b7187981`). It changes only the CI configuration, so no release carries it
  or needs to: the `main` pipeline on that commit and the scheduled ones since
  report `DONE 132 tests` in `tests:integration`, none skipped, all but the
  scheduled pipeline of 2026-10-03 (2909832640), whose `tests:integration`
  failed installing its tools, `mise` timing out on `api.github.com` for
  `buf`, before any test ran. Read again on 2026-10-08, that is still the
  only exception: the scheduled pipelines of 2026-10-04 to 2026-10-07 and the
  two `main` pipelines of 2026-10-07's pushes that ran all report
  `DONE 132 tests`. The pipeline of that day's third push, the release
  commit of v3.16.0, was skipped and ran no job.
  `gitlab-org/api/client-go!3066` itself still waits on its review: nobody
  has commented on it since it was readied on 2026-09-30, @aharadon gave it
  its group label on 2026-10-01, and the triage bot labelled it `idle` at
  00:20 UTC on 2026-10-08.
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
  routes do not ask for them, so that key never arrives there. That changes
  with
  [gitlab-org/gitlab!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277),
  opened on 2026-10-07, which has the impersonation token routes present a
  granular token's scopes as the personal access token routes do
  ([row 86](#a-tokens-own-description-omits-its-granular-scopes)); once a
  GitLab release carries it, `ImpersonationToken` wants the field too. Read on
  2026-10-08 it is open and waits on the backend review requested that
  evening.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The fields are read from the captured response beside
  the SDK's decode ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)),
  through the three token readers in `internal/toolutil/sent_shapes.go`, in
  every package that presents a token: `accesstokens`,
  `impersonationtokens`, `users`, `groupserviceaccounts` and
  `projectserviceaccounts`. `gitlab.ReadGrant` (`internal/gitlab/grant.go`)
  carries the capture as well: it reads a fine-grained token's own
  `granular_scopes` from `GET /personal_access_tokens/:id` to judge what the
  token is served (issue 952), since
  `GetSinglePersonalAccessTokenByID` decodes the answer and drops them. They
  retire when the structs carry the fields.

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
  [gitlab-org/api/client-go#2269](https://gitlab.com/gitlab-org/api/client-go/-/issues/2269),
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
- **Workaround**: retired for all fourteen: twelve at the **v3.12.0** pin,
  the thirteenth, `packages`, at **v3.14.0**, and the fourteenth,
  `systemhooks`, at **v3.15.0**, as the end of this bullet says.
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

  `systemhooks` followed when the pin moved to **v3.15.0** on 2026-10-05,
  checked against that release's `system_hooks.go` in the module cache: the
  list, get, add and edit handlers read the seven `Hook` fields of
  `gitlab-org/api/client-go!3048` (`organization_id`, `alert_status`,
  `disabled_until`, `push_events_branch_filter`, `branch_filter_strategy`,
  `custom_webhook_template` and `custom_headers`) off the struct, and the
  capture, its shape and its two readers are gone. The published hook is the
  one the capture produced, field for field, and a custom header still
  publishes its name alone. The same merge request added
  `custom_webhook_template` to both option structs, which GitLab accepts on
  `POST /hooks` and `PUT /hooks/:hook_id` (the live record declares it on
  both routes), so `admin.system_hook_add` and `admin.system_hook_edit` now
  offer it too. At the v3.14.0 pin the system hook was the one webhook of the
  three whose template this server could read and not set, since client-go's
  project and group hook options already carried it.
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
  ([gitlab-org/api/client-go!3056](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3056),
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
[gitlab-org/api/client-go!3045](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3045)
rather than opening it themselves, so cross-checking all eight fields against
`doc/api/` was worth doing: two of the eight, `file_extension` and
`is_receptive`, appeared nowhere on their page, and a third page showed
`public_email` in none of its fourteen example responses. Ten documentation
merge requests have gone to `gitlab-org/gitlab` from its own
[community fork](https://gitlab.com/gitlab-community/gitlab-org/gitlab):
[gitlab-org/gitlab!254507](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254507),
[gitlab-org/gitlab!254511](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254511),
[gitlab-org/gitlab!254519](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254519),
[gitlab-org/gitlab!254538](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254538),
[gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540),
[gitlab-org/gitlab!254542](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254542),
[gitlab-org/gitlab!254543](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254543),
[gitlab-org/gitlab!254547](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254547),
[gitlab-org/gitlab!254552](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254552) and
[gitlab-org/gitlab!259375](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259375).
Nine are merged into `master`: `gitlab-org/gitlab!254507` on 2026-09-10,
`gitlab-org/gitlab!254519` on 2026-09-11, `gitlab-org/gitlab!254511` and
`gitlab-org/gitlab!254542` on 2026-09-14 and 2026-09-15,
`gitlab-org/gitlab!254538`, `gitlab-org/gitlab!254543` and
`gitlab-org/gitlab!254547` together on 2026-09-22,
`gitlab-org/gitlab!254552` (the snippet clone URLs) on 2026-09-23, and
`gitlab-org/gitlab!259375` (the deploy keys page's Retrieve and Update
sections) on 2026-10-04. Held to the tags that contain each merge commit, read
on 2026-09-23, on 2026-09-24, again on 2026-09-27 and on 2026-10-04, three of
them have shipped: `gitlab-org/gitlab!254507`, `gitlab-org/gitlab!254511` and
`gitlab-org/gitlab!254519` are in `v19.4.0-ee` and `v19.4.1-ee`, and the other
six are in no tag yet, so 19.5 is the first release that can carry them; once
it is cut, this paragraph and the umbrella's documentation paragraph can say
they shipped. Until the first of
those reads the paragraph said none had shipped, a claim that had not been
checked against the tags. `gitlab-org/gitlab!254538` is the one whose merge had
been blocked by a `pre-merge-checks` race rather than by anything in the
change.

The one still open is
[gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540)
(the deploy key fields), in milestone 19.5 since the writer set it at 12:24
UTC on 2026-09-24 and in no tag, and what held it up was
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

Later that day, at 11:37 UTC, the writer asked @narendran-kannan, a backend
reviewer, for a review of its technical accuracy, and after the bot's
inactivity nudge of 2026-09-30 it came at 13:05 UTC on 2026-10-02. One finding
blocked: a deploy key's `usage_type` is always `auth_and_signing`, the
column's default, since no deploy key create path accepts the parameter, so
the eleven `auth` values the merge request had added were wrong. `auth` is the
example of `API::Entities::SSHKey`, which deploy keys share with user keys, and
`doc/api/keys.md` carried the same mistake. A second, non-blocking note listed
seven problems already on the page, for a follow-up. The answer went in at
16:11 UTC as five commits on top of `b2b900f7`: `c19b91b4` sets all eleven to
`auth_and_signing`; `b2e394bc` corrects `doc/api/keys.md`, whose example now
shows `auth_and_signing` and whose table no longer offers `auth`; `5338f331`
removes the unlabelled example response the note listed in "List project
deploy keys for user", a block this merge request already edits, so a
follow-up removing it would conflict; and two found while checking what the
entity sends, `2cc91577`, which adds the `fingerprint` and
`fingerprint_sha256` that the add, update and enable examples and the deploy
key table and example of `doc/api/keys.md` left out, and `cf255773`, which
drops the `SHA256:` prefix every `fingerprint_sha256` on both pages carried
and the API never returns, since `Key#generate_fingerprint` strips it before
saving. The title became "Make the deploy key example responses match what
the API returns", and the description was rewritten to match. Of the seven
problems, four were already fixed on `master` by another contributor's
[gitlab-org/gitlab!256496](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256496),
merged on 2026-09-21 after this branch was cut, and one is the block above.
The other two went to
[gitlab-org/gitlab!259375](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259375),
"Fix the Retrieve and Update sections of the deploy keys API page", opened
from the community fork at 16:08 UTC: the Retrieve example now requests key
`1`, the key its response shows, and the Update table gains its `key_id` row
and a note that the request needs at least one of `can_push` or `title`,
which the route enforces with `at_least_one_of`. The two change different
lines of the page, so they merge in either order. Its first
`@gitlab-bot ready`, seconds after it opened, was undone when the bot
labelled it a community contribution; the second, at 17:13 UTC, set
`workflow::ready for review`, and the documentation automation requested
@rsarangadharan, the page's writer. Read at the end of 2026-10-02:
`gitlab-org/gitlab!254540` is on head `cf255773` with a green fork pipeline,
both review threads answered and waiting on the reviewer, and a
`@gitlab-bot ready @narendran-kannan` asking for the re-review;
`gitlab-org/gitlab!259375` has a green fork pipeline and no milestone. Neither
needs an approval, and each waits on its reviewer.

Read on 2026-10-04: @rsarangadharan approved `gitlab-org/gitlab!259375` at
02:51 UTC that day and started the merge train that merged it at 03:00 UTC, as
squash commit `b1d0b26b` (merge commit `5bbb1b15`), in milestone 19.5. Two of
the scheduled `master` pipelines that ran on its merge commit failed and opened
broken `master` incidents, which name it only as the commit they ran on: it
changes `doc/api/deploy_keys.md` alone, one incident was closed as a job
timeout that could not be reproduced, and the other is open with its cause
undetermined. `gitlab-org/gitlab!254540` is where the end of 2026-10-02 left
it, on head `cf255773` and waiting for the re-review.

Read on 2026-10-05: at 10:15 UTC @narendran-kannan, a reviewer of
`gitlab-org/gitlab!254540` since 2026-09-29, re-requested a review from
himself, so the re-review is on his side; the head is still `cf255773`, with
its green fork pipeline.

Read on 2026-10-06: @narendran-kannan approved `gitlab-org/gitlab!254540` at
17:36 UTC ("Overall changes LGTM"), with two non-blocking notes. One proposed
a wording for the `fingerprint_sha256` line of the attribute list, saying
that the value comes without the `SHA256:` prefix and that a lookup by
fingerprint through the keys API needs it added; it went in as commit
`4eab7848` at 18:23 UTC, whose fork pipeline passed. The other pointed at the
example of `fingerprint_sha256` in `lib/api/entities/deploy_key.rb`, which
still carries the prefix, as a follow-up; our answer agreed, noted that the
generated OpenAPI v2 and v3 documents repeat it, and promised one follow-up
merge request for all three once this one merges. Both threads are resolved.
It needs no approval and waits on the writer, @rsarangadharan, to merge it.

Read on 2026-10-08: at 07:49 UTC on 2026-10-07 @narendran-kannan re-requested
a review from @rsarangadharan. `gitlab-org/gitlab!254540` is still open on
`4eab7848`, mergeable, with his approval alone and milestone 19.5, and still
waits on the writer.

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
[gitlab-org/api/client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)
(`Hook`),
[gitlab-org/api/client-go!3049](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3049)
(both deploy key structs),
[gitlab-org/api/client-go!3050](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3050)
(both event structs),
[gitlab-org/api/client-go!3051](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3051)
(`Namespace`),
[gitlab-org/api/client-go!3052](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3052)
(`Package`, plus the `GetProjectPackage` wrapper the versions field needs and
which the library did not have) and
[gitlab-org/api/client-go!3053](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3053)
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
approval; each new one waits on its review. Read on 2026-10-05 nothing has
moved on it since 2026-09-30: head `a9dbd2b5`, milestone 19.5 (set on
2026-09-29), no approval of the one required, and both questions of
[note 3929048251](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063#note_3929048251)
unanswered. A reminder in that thread is planned for 2026-10-08, the same day
as one on row 51's merge request. Read at 05:18 UTC on 2026-10-08 it is where
2026-10-05 left it, and the triage bot labelled it `idle` at 00:20 UTC on
2026-10-08; neither reminder had been posted yet.

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

**Behind again on 2026-10-05**, in its documentation paragraph alone. The
description, last changed on 2026-09-30 for the split, still counts nine
documentation merge requests, says eight of them are merged, and puts
[gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540)
in review with the page's technical writer. Since then
[gitlab-org/gitlab!259375](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259375),
opened on 2026-10-02, merged on 2026-10-04, so ten went out and nine are
merged, and
[gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540)
has had a backend reviewer, @narendran-kannan, since 2026-09-29, whose
re-review it waits on. Its table and count paragraph are current: the three
merge requests open in client-go read open and in review, and 32 of the 290
fields read released. The rewrite is an upstream write and waits on the
maintainer's yes; whether
[gitlab-org/gitlab!259375](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259375),
which fixes problems the page already had, belongs in the umbrella at all is
part of that decision. Read on 2026-10-06, @narendran-kannan approved
`gitlab-org/gitlab!254540` and it waits on its writer, so the umbrella's
sentence on it, in review with the page's technical writer, reads right
again; its documentation paragraph is now behind only in counting nine merge
requests with eight merged, where ten went out and nine are merged. Read on
2026-10-08 the description is unchanged since 2026-09-30 and has no new
comment, so that count is still the one place it is behind. The 4.0 issue,
[gitlab-org/api/client-go#2301](https://gitlab.com/gitlab-org/api/client-go/-/work_items/2301),
is cited since 19:41 UTC on 2026-10-07 by another contributor's draft,
[gitlab-org/api/client-go!3079](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3079),
which discourages pointer-to-slice fields in the project's style guidance
and defers converting the 35 existing ones to 4.0, "similar to how breaking
changes are being collected" there.

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
  routes create belongs to a project and never carries `group`. Read on
  2026-10-05, `group` is in review in somebody else's merge request,
  [gitlab-org/api/client-go!3074](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3074),
  opened on 2026-10-02 to retire a local wrapper type in glab
  ([gitlab-org/cli!3255](https://gitlab.com/gitlab-org/cli/-/merge_requests/3255)),
  which decodes it as a `ProjectNamespace`, the shape of the `NamespaceBasic`
  GitLab sends. `updated_at` is in no merge request yet, but it is proposed
  in that one: at 19:52 UTC on 2026-10-05 a comment of ours there
  ([note 3958082267](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3074#note_3958082267))
  suggested adding it beside `created_at` while `Todo` is open, citing the
  entity and the API page, which this entry did not record until 2026-10-08.
  At 16:41 UTC on 2026-10-07 @PatrickRice agreed
  ([note 3972179181](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3074#note_3972179181):
  "as long as we have the model open let's add that one missing attribute,
  then I think we are good to merge"), said he may open a documentation merge
  request to update the API page's example, requested his own review and
  assigned the merge request to its author, @jhebden.
  Read on 2026-10-08 its head is still `d7b77a74`, one commit adding `Group`
  alone, so `updated_at` has not been pushed. The capture in
  `internal/tools/todos` stays until a release carries both.
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

  Read on 2026-10-05, another change touches the same code:
  [gitlab-org/api/client-go!3064](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3064),
  a draft opened on 2026-09-29 by @phil-wong of `group::work items`, moves
  `WorkItemsService` to the work items REST API behind the
  `work_item_rest_api` feature flag, whose rollout issue,
  [gitlab-org/gitlab#588874](https://gitlab.com/gitlab-org/gitlab/-/issues/588874),
  has no milestone. It deletes the GraphQL documents commit 8 rebuilds,
  declares the public API unchanged and `internal/graphqlfields` unused, and
  drops `linkedItems` from `WorkItemDefaultListFields`, the set this entry's
  actions are to pass on Free. Commit 8 adds public API (`ReturnedFields` on
  two option structs, `GetWorkItemWithOptions`, `GetWorkItemOptions` and a
  regenerated `testing/workitems_mock.go`) and new users of
  `internal/graphqlfields`, so whichever of the two merges second has to carry
  or drop those. @PatrickRice asked on it at 23:19 UTC on 2026-09-29 whether
  it is a breaking change until the flag is rolled out, which has no answer
  yet. It fixes nothing for anyone while the flag is off, so the summary row
  does not count it, and whether its REST routes refuse anything on Community
  Edition is unverified.

  Read on 2026-10-08, that conflict is gone. On 2026-10-06 @PatrickRice set
  out two ways for client-go to offer the REST API, falling back to GraphQL
  when REST is off or a separate REST service so that GraphQL callers are
  unaffected, preferring the second, and @phil-wong agreed. He opened the
  replacement,
  [gitlab-org/api/client-go!3078](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3078),
  as a draft at 22:00 UTC that day: a new `WorkItemsRESTService`
  (`client.WorkItemsREST`) whose one method, `GetWorkItem`, reads
  `GET /namespaces/:id/-/work_items/:work_item_iid` behind the same flag, with
  the GraphQL `WorkItemsService` left unchanged. At 22:27 UTC he closed
  `gitlab-org/api/client-go!3064` unmerged as the proof of concept. The new
  draft leaves the documents commit 8 rebuilds and
  `WorkItemDefaultListFields` as they are, so the two no longer collide and
  the breaking change question asked on the closed draft no longer applies.
  @PatrickRice reviewed the new draft, and @phil-wong answered the review
  with `183e3c44` at 05:17 UTC on 2026-10-07. At 05:02 UTC that day
  @PatrickRice asked on it whether the REST API keeps the GraphQL behaviour
  this entry describes, an error for an Enterprise field requested on
  Community Edition; nobody has answered that yet. At 04:32 UTC on
  2026-10-08 he agreed to a slice of user pointers rather than of values, for
  consistency with the merge request structs, and at 05:38 UTC @phil-wong
  pushed `b84f0ca8`, which leaves the REST service's opt-in fields nil when
  they are not requested. That push also edits `workItemCEFields` in `workitems.go`, the
  field list commit 8 builds its documents from, but only by adding lint
  comments to three of its entries, on lines commit 8 does not change, so the
  two still do not conflict. Read at 05:53 UTC, its pipeline passed at 05:49
  and the draft has no approval. The rollout issue still has no milestone.
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
  now names that merge request as the one carrying the option. At 02:11 UTC
  on 2026-10-01 @PatrickRice reviewed it and requested changes, with two
  suggestions and nothing else: a comment at the call site saying where
  `WithNilOptionsOmitted` sets the behavior, and the helper inverted from
  `isNilOptions` to `isNonNilOptions`, since its one caller always negates
  it. Both went in at 20:36 UTC as one commit on top,
  `3dafd822`, the comment typed in the reviewer's words rather than applied
  as a suggestion so that it is tab-indented and wrapped as the project's
  `AGENTS.md` asks; no test names the helper, so none changed. Each thread
  was answered, the description names `isNonNilOptions`, its fork pipeline
  passed, and a `@gitlab-bot ready @PatrickRice` at 20:44 UTC put it back at
  `workflow::ready for review`. It waits on that review, whose
  requested-changes state stays until the reviewer clears it, and on its one
  required approval. Read on 2026-10-05 nothing has moved since: the reviewer
  field still shows @PatrickRice at requested changes, since the ready of
  2026-10-01 drew the bot's ready note but no new review request, unlike the
  first ready of 2026-09-30, and a reminder is planned for 2026-10-08. Read
  at 05:18 UTC on 2026-10-08 it is still where 2026-10-01 left it, with no
  `idle` label, unlike the joint merge request and row 19's, and the reminder
  had not been posted yet.
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

### UpdatePackageProtectionRulesOptions sends two explicit nulls on every partial update

- **Reported**: yes, as commit 33 of
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  opened on 2026-09-27, the joint merge request
  [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes.
- **In review**: yes, open. The commit, `707ff4be`, gives both fields
  `omitempty` on both halves of the tag and pins a body carrying the one field
  the caller set; `CreatePackageProtectionRulesOptions` keeps its tags, since
  GitLab requires both there. Read on 2026-10-07 it is still the 33rd of the
  merge request's 35 commits, and the merge request is open.
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
  Neither is carried here, and that is a decision rather than a gap. GitLab
  refuses these nulls, so by the rule
  [issue 966](https://github.com/jmrplens/gitlab-mcp-server/issues/966) set
  (a request built here only for a field GitLab is shown to mishandle) this
  field qualified for one; the maintainer chose to keep the hint below and
  wait for the client-go release that carries the commit, in the issue's
  status comment of 2026-09-30 (against `7baa4a40b`), which names this hint
  among the workarounds that retire at that bump.
  `internal/tools/protectedpackages.Update` sets each
  pointer only when the caller named a value, which is the safe side of that
  choice, and answers the refusal with a hint telling the caller to name
  `package_name_pattern` and `package_type` on every update
  (`protected_packages.go:165-175`). The protection rule lifecycle in
  `test/e2e/gitlab/common` (`TestPackage_ProtectionRules_Lifecycle`) sends the
  type with every update for the same reason. All of it retires at the bump
  to the client-go release that carries the commit:
  `TestUpdate_LeavesAnUnnamedPatternNullRatherThanEmpty` is written to go red
  the day the tag changes, which is the signal to drop the 422 hint and its
  comment and let the e2e lifecycle update a single field. Issue 966 tracked
  this entry until it closed on 2026-10-07;
  [entry 54](#seven-more-option-structs-send-an-optional-param-on-every-call)
  holds the measurement that closes it.

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
in v3.0.0, v3.10.0, v3.12.0 and v3.14.0, and on 2026-10-07 in v3.15.0, the
version `go.mod` pins, and v3.16.0, tagged that day
(`protected_packages.go:86-87`), so a dependency bump does not retire it.

**Effort**: small, two struct tags and a test, like
[`SetFeatureFlagOptions`](#setfeatureflagoptions-fields-lack-omitempty).

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
  entries 6 and 52 as commits 32 and 33. Read on 2026-10-07 the merge request
  is open with 35 commits and this one, `26736d2b`, is still the 34th; its
  diff adds `omitempty` to each of the seven fields in the first table below,
  and the merge request adds no field to `CreateGroupIssueBoardListOptions`.
- **Merged**: no. Issue 966 confirmed each of the seven tags at v3.14.0, and
  read on 2026-10-07 each is unchanged, on the same line, at v3.15.0, the
  version `go.mod` pins, and v3.16.0, tagged that day:
  `dependency_list_export.go:70`,
  `group_boards.go:216`, `group_members.go:198`, `project_members.go:152`,
  `projects.go:1361`, `issue_links.go:121` and `pipeline_schedules.go:267`.
- **Blocking**: no, field by field in the second table below. For five of the
  seven, GitLab reads a null exactly as it reads the key left out, or never
  receives one. `label_id` is latent: a null would be refused beside another
  board list type, which neither the struct nor the handler offers today. It
  stops being latent the day client-go models the other three list types
  ([entry 96](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types)),
  so that change has to ship with this one or after it, never before.
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
  stays until client-go models the milestone, iteration and assignee lists,
  which is
  [entry 96](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types). `pipelineschedules.EditVariable` has required `value` since
  the domain was written, and the requirement is what keeps the null from
  reaching GitLab and what costs the action an edit of the type alone. It
  retires with the tag: `value` then becomes optional in the handler, in its
  check (`pipeline_schedules.go:522`) and in the `required` of its input
  schema (the `jsonschema` tag at `:507`, and the golden snapshots that carry
  it), as GitLab declares it. A
  handler could also get past the tag today, contrary to what this entry first
  said, in two ways: a request option, since client-go runs the request
  options after it has marshalled the body (`NewRequestToURL` in `gitlab.go`)
  and an option can read the body and replace it, which is how client-go's own
  GraphQL pagination option works; or a request the handler builds itself, as
  `features.Set` does for entry 6. Neither is carried, by decision: GitLab
  mishandles two of these nulls (entry 52's, which it refuses, and `value`,
  which it stores), which by
  [issue 966](https://github.com/jmrplens/gitlab-mcp-server/issues/966)'s own
  rule qualified each for a request built here, and the maintainer chose
  instead to keep entry 52's hint and this required value until the client-go
  release that carries the fix, in the issue's status comment of 2026-09-30
  (against `7baa4a40b`). Building either request here is new work of its own
  and would be an issue of its own; it is not what the two wait on.

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
| `ShareWithGroupOptions`               | `ExpiresAt`  | `expires_at`  | `POST /groups/:id/share`                                  | none since issue 1027 (below)      |
| `ShareWithGroupOptions`               | `ExpiresAt`  | `expires_at`  | `POST /projects/:id/share`                                | `projects.ShareProjectWithGroup`   |
| `CreateIssueLinkOptions`              | `LinkType`   | `link_type`   | `POST /projects/:id/issues/:iid/links`                    | `issuelinks.Create`                |
| `EditPipelineScheduleVariableOptions` | `Value`      | `value`       | `PUT /projects/:id/pipeline_schedules/:id/variables/:key` | `pipelineschedules.EditVariable`   |

Five of the seven handlers set the pointer only when the caller named a value,
which is the safe side of the choice they have. For four of them the null is
what the tag adds when the caller did not; for `dependencies.CreateExport` it
is not, because client-go fills in `"sbom"` when the pointer is nil, so no null
is ever sent there. The other two, `groupboards.CreateGroupBoardList` and
`pipelineschedules.EditVariable`, require the field and always set it. The
`POST /groups/:id/share` row has no handler here any more: this table was
written when `groupmembers.ShareGroup` sent `ShareWithGroupOptions`, and since
[issue 1027](https://github.com/jmrplens/gitlab-mcp-server/issues/1027),
closed by pull request 1071 (`e30abf5cd`), it sends
`ShareGroupWithGroupOptions` through `Groups.ShareGroupWithGroup`
(`internal/tools/groupmembers/group_members.go:451-489`), whose fields all
carry `omitempty`.
`TestGroupMemberWrites_OptionalParametersReachTheRequestOnlyWhenGiven/share`
pins that a share without an expiry carries no `expires_at` key.

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
is affected. `ShareWithGroupOptions` is sent by one package today:
`projects` passes it to `Projects.ShareProjectWithGroup`, and
`TestShareProjectWithGroup_SendsExactlyWhatTheCallerSet` pins the
`"expires_at":null` a share without an expiry carries. The audit lists it
under `groupmembers` and under `groups` as well, and both are the join rather
than callers: `group.share_with_group` (`internal/tools/groups`) has always
sent `ShareGroupWithGroupOptions`, and `group.group_member_share` has sent the
same since issue 1027, both to `POST /groups/:id/share`, the route
`GroupMembers.ShareWithGroup` reaches with `ShareWithGroupOptions`; the check
pairs option types with a recorded route (`optionTypesByRoute`) rather than
with the handler that sent the request, and so asks about every option struct
whose method reaches that route.
[Issue 1104](https://github.com/jmrplens/gitlab-mcp-server/issues/1104), open,
owns that limit of the check. Those two rows, and the `export_type` one
above, which client-go never sends as null, are the rule's reading of a tag
rather than a request this server sends wrongly; the two share rows leave the
report with issue 1104 or with the `ShareWithGroupOptions` tag, whichever
comes first, and the `export_type` row with its tag.

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

**What closes it here, and how it is measured.** This entry, with entries 6
and 52, replaces
[issue 966](https://github.com/jmrplens/gitlab-mcp-server/issues/966), closed
on 2026-10-07: what was left of it was a client-go bump once a release carries
commits 32 to 34 of the joint merge request, and the workarounds retiring
with it. The measure is R-PATH's always-sent check,
`go run ./cmd/audit_1to1/ -scope=paths -gaps-only`, whose
`always_sent.optional_but_always_sent` listed 11 rows over nine fields in
eight option types when read on 2026-10-07 (37 always-sent fields GitLab
requires beside them, and two the route does not declare), and reads empty
after that bump; the sentence of `CLAUDE.md`'s Request paths section that
counts the 11 and the 37 moves with it. The bump layer does, beside what
entries 6 and 52 list:

- `pipelineschedules.EditVariable` makes `value` optional in the handler, in
  its check and in its schema, with the golden snapshots regenerated. No test
  goes red on the bump, so this entry is what triggers it.
- Three tests that pin the null today go red and are rewritten to pin its
  absence: `TestMemberAdd_SendsTheIdentityAndOptionsItWasGiven`
  (`internal/tools/members/members_test.go:1500`, `fieldExpiresAt: nil`),
  `TestShareProjectWithGroup_SendsExactlyWhatTheCallerSet` and
  `TestIssueLinkCreate_SendsNoLinkTypeWhenUnset`.
- `groupboards.CreateGroupBoardList` keeps requiring `label_id` until
  [entry 96](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types)
  lands as well.

Two open issues of this repository build on these nulls:
[issue 1100](https://github.com/jmrplens/gitlab-mcp-server/issues/1100), on
which parameters of `issue.link_create` the individual tools and the catalog
route schema call required, `link_type` among them, and
[issue 1104](https://github.com/jmrplens/gitlab-mcp-server/issues/1104), the
check's route join above.

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

- **Reported**: no. A change is drafted and has not been sent: an additive
  one, which keeps `ExtendedTrailers map[string]string`, marks it
  deprecated, and adds `ExtendedTrailerValues map[string][]string` beside it,
  filled from `extended_trailers` by a `Commit.UnmarshalJSON`, so every
  method answering with a `Commit` decodes the lists and no caller breaks. Its
  test, `TestCommitsService_ListCommits_ExtendedTrailers` in client-go's own
  style, answers a commit shaped like GitLab's request spec fixture. The plain
  retype to `map[string][]string` stays the alternative, for a major version
  or for a minor if client-go's maintainers accept it there. Read on
  2026-10-07, no client-go merge request or issue proposes either, and
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
  does not touch the field. It goes either as a commit of a client-go merge
  request of field syncs or as one of its own from the community fork, after
  the maintainer has reviewed its text, and not before the three merge
  requests of ours already open in that project (rows 19, 34 and 51) have
  been reviewed.
- **In review**: no.
- **Merged**: no.
- **Blocking**: was yes, for `repository.commit_list` with `trailers` set: a
  page holding one commit with a trailer failed as a whole, with client-go's
  decode error in place of the page. It returns the page since
  [issue 1026](https://github.com/jmrplens/gitlab-mcp-server/issues/1026).
- **Workaround**: yes for every action that publishes `extended_trailers`, and
  not yet for the readers of an embedded commit. Every handler that publishes
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
  `branches.capturedBranch`), the single merge request diff version
  (`mr_review.diff_version_get`, through `mrchanges.capturedDiffVersion`), and
  the two other lists that publish `commits.Output` rows,
  `merge_request.commits` (`mergerequests.Commits`) and `search.commits` at
  each of its three scopes (`search.Commits`), through
  `commits.CapturedOutputs` and `commits.MisreadByClientGo`: each page is read
  the way `repository.commit_list` reads its own, so a commit carrying a
  trailer no longer fails it as a whole. The handlers that decode a `Commit`
  inside an answer and publish no trailers (the commit of a tag, a release, a
  group release or a job, and the deployable commit of a deployment or an
  environment) fail as `repository.commit_list` did the day their route parses
  trailers, and so do the readers outside the tools that take a commit from
  client-go: the commit, branch and tag resources
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
(`lib/gitlab/git/commit.rb`) builds it that way,
`(hash[trailer.key] ||= []) << encode!(trailer.value)`. `Commit` in client-go's
`commits.go` declares `ExtendedTrailers map[string]string`, at v3.14.0 and,
read again on 2026-10-05, at v3.15.0 and on `main` (`000e81ea`, line 159), and
on 2026-10-07 at v3.16.0, tagged that day, on the same line. The
field came with that type from
[gitlab-org/api/client-go!1957](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/1957),
merged on 2024-06-18 and titled for snippets, whose only change is that field
in `commits.go` and whose description reports a `cannot unmarshal array`
error and names this key. So decoding a commit whose `extended_trailers` is
not empty fails with `json: cannot unmarshal array into Go struct
field Commit.extended_trailers.Signed-off-by of type string`, and the SDK
returns that error in place of the whole answer. Only one route fills the key
today, which is why nothing else has failed: measured on gitlab.com on
2026-09-27, `GET /projects/278964/repository/commits?per_page=40&trailers=true`
answers commit `9f1632e2` with `"Reviewed-by"` mapped to four values and
`"Co-authored-by"` to two, while the same
commit read singly (`/repository/commits/9f1632e2...`) and as the last commit
of a tree entry (`/repository/tree?with_last_commit=true`) answers `{}` for
both trailer keys, as does the list without `trailers`. Measured again on
2026-10-05: the same list with `trailers=true` answers commit `b4a3d0eb` with
`Approved-by` mapped to three values and `Reviewed-by` to two, and the commit
read singly and the list without `trailers` answer `{}` for both keys. Only
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

**What closes it here.** This entry replaces
[issue 1026](https://github.com/jmrplens/gitlab-mcp-server/issues/1026),
closed on 2026-10-07. Everything that issue asked of this repository is done:
pull request 1072 (`852a653ee`) typed the commit, branch, compare, merge base
and diff version outputs as lists read from the capture, and pull request 1200
(`c39571322`) moved `merge_request.commits` and `search.commits` onto the same
path, each tested with GitLab-shaped fixtures
(`internal/tools/mergerequests/merge_requests_test.go`,
`internal/tools/search/search_test.go`) and the one route GitLab fills held
by `test/e2e/gitlab/common/commits_test.go`. What is left is the client-go
change above, its release, and the layer that moves the client-go pin to that
release. That layer:

- switches every capture reader the Workaround bullet lists to the SDK field:
  `commits.Captured`, `commits.CapturedOutput` and `commits.CapturedOutputs`
  with `commits.MisreadByClientGo`, `repository.capturedCompare` and
  `repository.treeCommit`, `branches.capturedBranch`,
  `mrchanges.capturedDiffVersion`, `mrcontextcommits.capturedCommit` and
  `mrcontextcommits.misreadByClientGo`, the `misreadByClientGo` of `branches`
  and of `mrchanges`, `repositorysubmodules.submoduleCommitExtra`,
  `mergerequests.Commits` and
  `search.Commits`, keeping a capture wherever it still reads another field
  client-go does not model (the tree entry's last commit and the submodule
  commit, for instance);
- retires, by the maintainer's decision recorded in the issue's status comment
  of 2026-09-30 and in pull request 1200, the latent failure of the readers of
  an embedded commit: the commit of a tag, a release, a group release or a
  job, the deployable commit of a deployment or an environment, the commit,
  branch and tag resources, the compare and branch prompts, and the branch,
  tag and commit completions. None of them publishes trailers, each fails only
  the day its route starts parsing them, and none is worked around before the
  bump;
- moves two fixtures of `internal/tools/tags/tags_test.go` (`:323` and `:394`)
  that spell `extended_trailers` as a map of strings, a shape GitLab never
  sends (it answers `{}` or lists), to `{}` or to lists, since a field that
  decodes lists would refuse them.

The sent audits compare key names and never a value's shape, so no audit here
reports this class; `docs/development/cmd-utilities.md` says so beside the
sent check. The issue's milestone (3.2.0) no longer tracks the bump once the
issue is closed; this entry does.

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

### OrbitGraphStatusProjects does not model the projects the indexer gave up on

- **Reported**: no. It joins the gaps held for the next joint client-go merge
  request, the one [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, and like every item here it waits on the maintainer's approval.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. `orbit.GraphStatus` (`internal/tools/orbit/orbit.go`)
  reads `projects.gaps` from the captured response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  into `GraphStatusProjects.Gaps`, and the graph status card shows it as the
  projects out of indexing attempts. R-PATH holds the field to the Orbit
  response record, which carries the key. `TestGraphStatus_Success_ByFullPath` holds the read and
  `TestGraphStatus_CapturedBodyThatDoesNotDecode_ReturnsAnError` its failure.
  It retires when the SDK models the field.

**Where**: `OrbitGraphStatusProjects` in client-go v3.15.0's `orbit.go`, which
models `indexed` and `total_known`.

**What**: GitLab's graph status answer carries a third count, `gaps`, on every
structured answer. `map_projects_status` in
`ee/lib/analytics/knowledge_graph/grpc_client.rb` sends
`{ indexed:, total_known:, gaps: }`, zeros included when the service sends no
projects; GitLab master gained the key with `9a5ee3ee` ("Add Orbit indexing
status and item counts endpoints", 2026-10-01). The Knowledge Graph service
defines it as the projects that used every indexing attempt without producing
an index (the `gaps` field of the `ProjectsStatus` message in the service's
protobuf contract under `crates/orbit-server/proto/`, v0.137.0). GitLab.com
sent it on 2026-10-05, as `0` for the fixture namespace.

**How we found it**: re-recording the Orbit response record for
[issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031). Its
`orbit.graph_status (raw)` call gained `projects.gaps`, which R-PATH then
reported as a key GitLab sends that the output did not publish.

**Effort**: small. Add `Gaps int64` with `json:"gaps"` to
`OrbitGraphStatusProjects`, and the key to the graph status decode test.

### GroupMilestone does not model the milestone's web URL

- **Reported**: no. It joins the gaps held for the next joint client-go merge
  request, the one [entry 34](#response-structs-that-miss-a-field-gitlab-sends-unconditionally)
  describes, and like every item here it waits on the maintainer's approval.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: yes. The field is read from the captured response beside
  the SDK's decode
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md)),
  through `toolutil.CapturedMilestone` and `toolutil.CapturedMilestones`
  (`internal/toolutil/sent_shapes.go`), in two places: the four group
  milestone handlers that return a milestone (`List`, `Get`, `Create` and
  `Update` in `internal/tools/groupmilestones/group_milestones.go`), and the
  `gitlab://group/{group_id}/milestone/{milestone_iid}` resource
  (`registerGroupMilestoneResource` in `internal/resources/resources.go`).
  `TestGroupMilestones_UnreadableCapturedWebURL` holds the handlers' failure,
  and `TestGroupMilestoneResource_PublishesTheWebURLGitLabSends` and
  `TestGroupMilestoneResource_AnAnswerTheCaptureCannotDecode_IsAnError` the
  resource's read and its failure. Both retire when the SDK models the field.

**Where**: `GroupMilestone` in client-go v3.15.0's `group_milestones.go`,
which models `id`, `iid`, `group_id`, `title`, `description`, `start_date`,
`due_date`, `state`, `updated_at`, `created_at` and `expired`. The project
milestone struct, `Milestone` in `milestones.go`, does carry `web_url`.

**What**: every group milestone route (`lib/api/group_milestones.rb`, through
`lib/api/milestone_responses.rb`) presents `Entities::Milestone`, and
`lib/api/entities/milestone.rb` exposes `web_url` with no condition, built by
`Gitlab::UrlBuilder` (lines 17 to 19 at `v19.4.1-ee`). The live record agrees:
`API::Entities::Milestone` carries `web_url` with no condition. So every group
milestone GitLab sends carries its page, and the decode drops it.

**How we found it**: the sent dimension of the 1:1 audit
(`shapes.typed.unsurfaced` in `go run ./cmd/audit_1to1/ -scope=paths`) during
the field-by-field review, which put the group milestone tools on the
captured read without a row here; and again for
[issue 1169](https://github.com/jmrplens/gitlab-mcp-server/issues/1169),
whose group milestone resource described a web URL it always left empty.

**Effort**: trivial. Add `WebURL string` with `json:"web_url"` to
`GroupMilestone`, and the key to the group milestone decode tests.

### CreateGroupIssueBoardListOptions models only label_id where GitLab takes four list types

- **Reported**: no. Its home is the joint client-go merge request,
  [gitlab-org/api/client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063),
  as one more commit, and like every item here it waits on the maintainer's
  approval. Read on 2026-10-07, no client-go merge request or issue proposes
  it, and neither that merge request's branch nor any other branch of the
  community fork adds the fields; the merge request changes only the tag of
  `LabelID` (commit 34, `26736d2b`,
  [entry 54](#seven-more-option-structs-send-an-optional-param-on-every-call)).
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. It narrows `group.group_board_create_list` to label
  lists, where GitLab also takes assignee, milestone and iteration lists on a
  licensed instance, which the 1:1 policy asks this server to offer.
- **Workaround**: none, by decision. The action offers only what the SDK
  struct can send, and no request is built outside the SDK for the other
  three types (a request option rewriting the body, or a request of the
  handler's own):
  [issue 1101](https://github.com/jmrplens/gitlab-mcp-server/issues/1101)
  decided that they follow a client-go change, and pull requests 1218 and
  1226, which kept that decision, made the refusal hints speak of `label_id`
  alone and key the role hint on the refusal GitLab gives
  (`createGroupBoardListError` in
  `internal/tools/groupboards/group_boards.go`, whose comment records the
  decision; `CreateGroupBoardList` refuses an input without `label_id` before
  it sends anything). This entry replaces issue 1101, closed on 2026-10-07.

**Where**: client-go v3.15.0, the version `go.mod` pins, and v3.16.0, tagged
on 2026-10-07, `group_boards.go:215-217`:

```go
type CreateGroupIssueBoardListOptions struct {
	LabelID *int64 `url:"label_id" json:"label_id"`
}
```

The project side of the same API already models all four, each with
`omitempty`, and is the template for the change: `CreateIssueBoardListOptions`
in `boards.go:260-265` carries `LabelID`, `AssigneeID`, `MilestoneID` and
`IterationID`.

**What**: `POST /groups/:id/boards/:board_id/lists` takes `label_id`,
`assignee_id`, `milestone_id` and `iteration_id`. At `v19.4.1-ee` the
Enterprise build makes them `exactly_one_of`
([`ee/lib/ee/api/boards_responses.rb:11-17`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/ee/lib/ee/api/boards_responses.rb#L11-17)),
the Community build `requires :label_id`
([`lib/api/boards_responses.rb:79-80`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/boards_responses.rb#L79-80)),
and the live record lists all four as optional params of the route
(`docs/development/gitlab-api-live.json`). A list type the licence
lacks is refused with a 400, not a 403:
`EE::Boards::Lists::CreateService#license_validation_error`
([`ee/app/services/ee/boards/lists/create_service.rb:24-37`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/ee/app/services/ee/boards/lists/create_service.rb#L24-37))
answers "Assignee lists not available with your current license" and its
milestone and iteration twins, and `API::BoardsResponses#create_list` renders
the service's first error with status 400
([`lib/api/boards_responses.rb:46-57`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/boards_responses.rb#L46-57)).
That sentence is written in the language the caller set in GitLab's
preferences, which the project-side `createBoardListError`
(`internal/tools/boards/boards.go`) already accounts for since pull request
1226. The API page, `doc/api/group_boards.md` at the same tag, documents all
four parameters, the last three as Premium and Ultimate only.

**It needs entry 54's tag to ship with it or before it.** Without `omitempty`
on `LabelID`, a request for another list type would carry `"label_id": null`,
and Grape's `exactly_one_of` counts a key present whatever its value
(`MultipleParamsBase#keys_in_common`), so GitLab would refuse it as two list
types at once. That is entry 54's latent case.

**What this server does when a release carries it**: in
`internal/tools/groupboards/group_boards.go`, `CreateGroupBoardListInput`
gains `assignee_id`, `milestone_id` and `iteration_id`, tagged
`tier:"premium"` as the other Premium board inputs have been since
[issue 1233](https://github.com/jmrplens/gitlab-mcp-server/issues/1233);
`label_id` loses its requirement in the handler's check and in the schema;
the handler asks for exactly one of the four; and `createGroupBoardListError`
gains a hint for the licence 400, as the project-side `CreateBoardList` has.
The action's description, usage and aliases ("from a group label") widen with
it, `TestCreateGroupBoardList_Refused_HintNamesOnlyWhatTheInputOffers`, which
asserts that no hint names an assignee, a milestone, an iteration, a tier or
a licence, is rewritten, Enterprise e2e scenarios are added for the three
list types (`test/e2e/gitlab/common/groupboards_test.go` creates label lists
only), and the tool reference is regenerated.

**How we found it**: checking pull request 1033, which recorded what GitLab
does with each null an always-sent option writes (entry 54's second table) and
noted, without changing them, the board hints that named `assignee_id` and
`milestone_id`; issue 1101 then took the hints, which pull requests 1218
(`5cbacf96d`) and 1226 (`d6627395b`) fixed, and the three list types, which
are this entry.

**Effort**: small. Three fields with `omitempty` on both halves of the tag,
copied from `CreateIssueBoardListOptions`, and a test that each one reaches
the body only when set.

### UpdateGroupIssueBoardOptions models neither list switch the group board update takes

- **Reported**: no. Its home is the joint client-go merge request, beside
  [row 96](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types)'s
  list types, and like every item here it waits on the maintainer's approval.
  Read on 2026-10-07, client-go's `main` (`53c03417`) and v3.16.0 still
  declare neither field, and no merge request or issue of that project
  proposes them for the group options:
  [gitlab-org/api/client-go!2780](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2780),
  merged on 2026-02-19, added them to the project options and changed only
  the project side (`boards.go` and its test).
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. A group board's Open and Closed lists cannot be hidden or
  shown through this server, while a project board's can.
- **Workaround**: not yet.
  [Issue 1241](https://github.com/jmrplens/gitlab-mcp-server/issues/1241) is
  this server's side: `groupboards.UpdateGroupBoard`
  (`internal/tools/groupboards/group_boards.go`) already sends a request of its
  own, `rawUpdateGroupBoard`, so that the board it answers with carries what
  `gl.GroupIssueBoard` does not model, and the two switches can go in a body
  struct of this server's own rather than in `gl.UpdateGroupIssueBoardOptions`.
  That struct retires when a release models them.

**Where**: client-go v3.15.0, the version `go.mod` pins, and v3.16.0, tagged
on 2026-10-07, `group_boards.go:158-164`:

```go
type UpdateGroupIssueBoardOptions struct {
	Name        *string       `url:"name,omitempty" json:"name,omitempty"`
	AssigneeID  *int64        `url:"assignee_id,omitempty" json:"assignee_id,omitempty"`
	MilestoneID *int64        `url:"milestone_id,omitempty" json:"milestone_id,omitempty"`
	Labels      *LabelOptions `url:"labels,omitempty" json:"labels,omitempty"`
	Weight      *int64        `url:"weight,omitempty" json:"weight,omitempty"`
}
```

The project side carries both and is the template for the change:
`UpdateIssueBoardOptions` in `boards.go:180-188` adds `HideBacklogList` and
`HideClosedList`, each a `*bool` with `omitempty`.

**What**: `PUT /groups/:id/boards/:board_id` takes `hide_backlog_list` and
`hide_closed_list`, both optional booleans that every tier serves:
`update_params_ce` in
[`lib/api/boards_responses.rb:83-87`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/boards_responses.rb#L83-87)
at `v19.4.1-ee` declares them for the project and the group board update
alike, the live record lists them among the route's params
(`docs/development/gitlab-api-live.json`), and `doc/api/group_boards.md`
documents them under "Update a group issue board" with no tier, where the
four scope fields beside them are marked Premium and Ultimate only. The
answer struct leaves them out as well: `GroupIssueBoard` models neither switch,
nor the licensed `assignee` and `weight`, which `Entities::Board` exposes, and
that is why `groupboards` reads every group board through a request of its
own; that half is
[row 101](#the-board-structs-miss-keys-gitlab-sends-on-a-board-and-its-lists).

**How we found it**: reviewing the change for
[issue 1233](https://github.com/jmrplens/gitlab-mcp-server/issues/1233), where
the next step a group board card offers had to leave the list visibility out,
since `UpdateGroupBoardInput` offers only `name` and the four scope fields; issue
1241 records the gap on this server's side.

**Effort**: trivial. Two fields with `omitempty` on both halves of the tag,
copied from `UpdateIssueBoardOptions`, and a test that each reaches the body
only when set.

### The board structs miss keys GitLab sends on a board and its lists

- **Reported**: no. Its home is the joint client-go merge request, beside
  [row 96](#creategroupissueboardlistoptions-models-only-label_id-where-gitlab-takes-four-list-types)
  and
  [row 98](#updategroupissueboardoptions-models-neither-list-switch-the-group-board-update-takes),
  and like every item here it waits on the maintainer's approval. Read on
  2026-10-07, client-go's `main` (`53c03417`) and v3.16.0 still declare
  `GroupIssueBoard` and `BoardList` as v3.15.0 does, and no merge request or
  issue of that project proposes the keys:
  [gitlab-org/api/client-go!2780](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2780)
  added the two list switches to `IssueBoard` and its update options only.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. This server reads the keys around the SDK, through
  requests of its own and the captured response.
- **Workaround**: partial. `groupboards` lists, reads, creates and updates
  every group board through a request of its own (`rawListGroupBoards`,
  `rawGetGroupBoard`, `rawCreateGroupBoard` and `rawUpdateGroupBoard` in
  `internal/tools/groupboards/group_boards.go`), decoded into
  `groupIssueBoardAPI`, which adds the four board keys and each list's
  `limit_metric`; its four list reads and writes take `limit_metric` off the
  captured response (ADR-0021, `toolutil.CapturedBoardList` and
  `CapturedBoardLists` in `internal/toolutil/sent_shapes.go`). On the project
  side, `boards.GetBoard` and `boards.ListBoardLists` send requests of their
  own (`rawGetBoard`, `rawListBoardLists`) for each list's `limit_metric`, the
  first for the board's `group` as well, and `ListBoards`, `CreateBoard` and
  `UpdateBoard` take `group` off the captured response (`CapturedBoard`,
  `CapturedBoards`). The partial is ours: the lists in the project board
  list, create and update answers, and the project's own list get, create and
  update, read no `limit_metric`, so it is empty there.
  What retires the workaround is a release that models the keys: the six
  requests and the board readers of `sent_shapes.go` go, and the handlers
  call the SDK's methods.

**Where**: client-go v3.15.0, the version `go.mod` pins, and v3.16.0,
`group_boards.go:97-104` and `boards.go:133-142`:

```go
type GroupIssueBoard struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	Group     *Group        `json:"group"`
	Milestone *Milestone    `json:"milestone"`
	Labels    []*GroupLabel `json:"labels"`
	Lists     []*BoardList  `json:"lists"`
}
```

```go
type BoardList struct {
	ID             int64              `json:"id"`
	Assignee       *BoardListAssignee `json:"assignee"`
	Iteration      *ProjectIteration  `json:"iteration"`
	Label          *Label             `json:"label"`
	MaxIssueCount  int64              `json:"max_issue_count"`
	MaxIssueWeight int64              `json:"max_issue_weight"`
	Milestone      *Milestone         `json:"milestone"`
	Position       int64              `json:"position"`
}
```

The project board struct beside them, `IssueBoard` in `boards.go:113-124`,
carries `Assignee`, `Weight`, `HideBacklogList` and `HideClosedList`, and is
the template for the group one.

**What**: every group board route (`GET`, `POST` and `PUT` on
`/groups/:id/boards`) presents `Entities::Board`, which exposes
`hide_backlog_list` and `hide_closed_list` on every board
([`lib/api/entities/board.rb:8-9`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/lib/api/entities/board.rb#L8-9)),
and `assignee` and `weight` where the board's parent has the
`scoped_issue_board` feature
([`ee/lib/ee/api/entities/board.rb:12-24`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/ee/lib/ee/api/entities/board.rb#L12-24)).
Every list route, and the lists inside a board, present `Entities::List`,
which exposes `limit_metric` beside `max_issue_count` and `max_issue_weight`
where the list has work-in-progress limits available
([`ee/lib/ee/api/entities/list.rb:13-18`](https://gitlab.com/gitlab-org/gitlab/-/blob/v19.4.1-ee/ee/lib/ee/api/entities/list.rb#L13-18));
`BoardList` models the two counts and not the metric they are counted by. The
live record lists the same exposures with the same conditions
(`docs/development/gitlab-api-live.json`). `IssueBoard` does not model
`group` either, which the Enterprise build exposes on every board
(`ee/lib/ee/api/entities/board.rb:10`) and sends as null on a project's, so
that key costs nothing.

**How we found it**: reviewing row 98, whose answer struct is this entry's
`GroupIssueBoard`; the same reading found `BoardList`'s metric behind the
readers `internal/toolutil/sent_shapes.go` already carried, whose comment says
every gap it reads is recorded here, and none of the board ones was.

**Effort**: small. Four fields on `GroupIssueBoard` copied from `IssueBoard`,
`LimitMetric` on `BoardList`, and a decoding test for each.

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
  `errorCode` and `carriesInvalidToken` (`internal/gitlab/credential_refusal.go`),
  which `UnauthorizedNamesCredential` and `RefusalMayBePermission` read
  ([row 55](#a-permission-refusal-is-answered-401-rather-than-403)). The first
  is one rule with two readers that the file says must not disagree: the
  transport, which reads a raw prefix of a 401's body before client-go builds
  any error (`classifyUnauthorized` in `internal/gitlab/response_limit.go`),
  and `internal/toolutil/errors.go`, which holds the `*ErrorResponse`
  (`gitLabResponseOf`, and `IsPermissionRefusal` through
  `RefusalMayBePermission`). The functions are named rather than their lines,
  which moved when issue 952's work merged on 2026-10-04. The helper replaces
  the two decoders only if this project adopts it, which is the decision the
  issue waits on. `isInsufficientScope` in `internal/oauth/verifier.go`
  decodes a response of the OAuth verifier's own HTTP client and stays either
  way.

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
GitLab.com whose classic token reaches such a group can see that code. GitLab's
fix for that issue,
[gitlab-org/gitlab!257280](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/257280),
merged on 2026-10-06 for 19.5, reached GitLab.com's canary stage that
evening and its production stage at 00:17 UTC on 2026-10-07. It exempts every
token whose owner is not a human user, so from then on the classic token that
meets the code is a human user's personal access token. On a self-managed
instance, enforcement only stops legacy tokens from
being created or rotated (`doc/auth/tokens/fine_grained_access_tokens.md`,
lines 134 to 153;
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
GitLab gives it. That issue closed on 2026-10-04 with this server's own
decoders in place, so adopting the helper would replace them rather than
unblock it. Whether it does is the maintainer's decision, and the draft's
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
- On GitLab.com the owner of a top-level group can require fine-grained tokens. After the enforcement date a user's legacy personal access token is refused in that group with `insufficient_granular_scope` (service account, group and project access tokens are exempt since gitlab-org/gitlab!257280), so a caller holding a classic personal access token can meet that code without ever creating a fine-grained one. On a self-managed instance, enforcement only stops legacy tokens being created or rotated.

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
[modelcontextprotocol/go-sdk#1268](https://github.com/modelcontextprotocol/go-sdk/pull/1268),
[modelcontextprotocol/go-sdk#1269](https://github.com/modelcontextprotocol/go-sdk/pull/1269),
[modelcontextprotocol/go-sdk#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273) and
[modelcontextprotocol/go-sdk#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274)), did not
name
[modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293)
as the pull request for
[modelcontextprotocol/go-sdk#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262),
and did not list
[modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299).
It was rewritten that evening (21:51 UTC): each pull request reads merged or
open as it stands,
[modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293)
is named under
[modelcontextprotocol/go-sdk#1262](https://github.com/modelcontextprotocol/go-sdk/issues/1262),
and a "Filed since" section lists
[modelcontextprotocol/go-sdk#1299](https://github.com/modelcontextprotocol/go-sdk/issues/1299),
with the closing sentence narrowed to the nine issues of the original audit,
which are the ones that link back to it.

On 2026-10-05 (14:09 UTC) a status note went on the index
([comment 5996173838](https://github.com/modelcontextprotocol/go-sdk/issues/1257#issuecomment-5996173838)),
addressed to @guglielmo-san, its first comment: the three pull requests still
open,
[modelcontextprotocol/go-sdk#1266](https://github.com/modelcontextprotocol/go-sdk/pull/1266),
[modelcontextprotocol/go-sdk#1267](https://github.com/modelcontextprotocol/go-sdk/pull/1267)
and
[modelcontextprotocol/go-sdk#1293](https://github.com/modelcontextprotocol/go-sdk/pull/1293)
(rows 11, 12 and 8), each with what it now does, that all three were green
when last updated and still merge into `main` without conflicts, an offer to
update or rebase any of them on request, and that the three proposals,
[modelcontextprotocol/go-sdk#1263](https://github.com/modelcontextprotocol/go-sdk/issues/1263),
[modelcontextprotocol/go-sdk#1264](https://github.com/modelcontextprotocol/go-sdk/issues/1264)
and
[modelcontextprotocol/go-sdk#1265](https://github.com/modelcontextprotocol/go-sdk/issues/1265)
(rows 10, 18 and 23), still wait on a choice of shape. The descriptions of the
first two were corrected seconds before it (14:09:10 and 14:09:12 UTC), since
the note asks the reviewer to read them; rows 11 and 12 say what changed. The
body of the index is as the rewrite of 2026-09-27 left it, and every state it
gives is still current.

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
  since closed by the merge below). Read on 2026-10-05 the pull request still
  conflicts with `main`, 64 commits behind it, with no check run on its head;
  on 2026-10-08 it is 67 behind and otherwise the same.
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
  still had no review. Read on 2026-10-02 it is nine commits behind `main`
  again, unchanged since. Read on 2026-10-05 it is twelve commits behind
  `main`, still merges without conflicts, its nine checks green on
  `4bb3af60`, and still unreviewed; the status note on the index that day
  names it. Read on 2026-10-08 it is 15 commits behind `main` and otherwise
  the same.
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
  since 2026-08-29, and so again on 2026-10-05, 56 commits behind `main`, and
  on 2026-10-08, 59 behind. A
  comment on the issue on 2026-09-27, not ours, confirms the
  defect in v1.8.0-pre.2 and reports that the pull request as it stands
  (`3906b01`) leaves part of it open: it answers a syntax error with `-32700`
  twice, and a frame that is valid JSON but not a JSON-RPC message (`42`,
  `{}`, `[]`) still ends the session, taking with it the answers owed to
  requests that arrived before it. That is not ours to answer, and nothing
  here waits on us. We open no second one; if it stalls, the evidence here
  (the e2e case and the stdio filter) goes on that thread rather than into a
  new pull request. Read on 2026-10-05 it has stalled by that measure: the
  issue has had no answer from a maintainer, no label and no type in the 37
  days since it was opened, and the pull request has had no review at all
  since it was opened on 2026-08-29 and still conflicts with `main`. The
  maintainer decided on 2026-10-05 to hold: go-sdk's one reviewer already has
  three pull requests and three proposals of ours waiting, the comment of
  2026-09-27 already confirms the defect on a recent pre-release and names
  what the pull request leaves open, and the workaround below makes nothing
  here urgent. Once that reviewer has reviewed our open pull requests, the
  missing cases (the doubled `-32700`, a valid JSON frame that is not a
  JSON-RPC message) go on the pull request's thread as tests offered to its
  author; if the author does not answer within about two weeks, a pull request
  of ours that covers every case, credits the original one and closes the
  issue replaces it.
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
request the server understands but cannot serve (an unknown method, a
nonexistent tool) is correctly answered with an error and the session
continues.

**How we found it**: writing `test/e2e/stdio`. The case was written expecting
the session to survive, and it did not.

**Also fixed by the workaround, unintentionally**: with `mcp.StdioTransport`,
EOF on stdin (a client closing its pipe, which is how every session ends)
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
  on 2026-09-13. Another user had reported the same defect ten days earlier,
  in
  [modelcontextprotocol/go-sdk#1235](https://github.com/modelcontextprotocol/go-sdk/issues/1235)
  (2026-09-03), which this entry did not name until 2026-10-08; a maintainer
  closed it at 14:22 UTC on 2026-10-05 as a duplicate of ours, which carries
  the pull request below.
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
  `761b52b2`) and its ten checks passed. The thread still waits. Read on
  2026-10-02 it is nine commits behind `main` again, unchanged since. Read on
  2026-10-05 it is twelve commits behind `main`, still merges without
  conflicts, its ten checks green on `761b52b2`, and the thread still waits
  on the reviewer. Its description was corrected that day (14:09 UTC), before
  the status note on the index named it: its tests paragraph described only
  the SSE run, and one bullet of what a user sees still described the JSON
  response mode as it stood before the review of 2026-09-20 (a 200 carrying
  `Content-Type: application/json` and no body, an offer of 202, no test of
  its own); both now say the POST is answered 204 with no `Content-Type` in
  both modes, as the code and its test do.
  [modelcontextprotocol/go-sdk#1328](https://github.com/modelcontextprotocol/go-sdk/issues/1328),
  opened by another user on 2026-10-01, proposes forwarders on `loggingConn`
  beside the `DropResponse` one this pull request adds, and says itself that
  the two touch neighbouring lines of `mcp/transport.go`. Read on 2026-10-08
  the pull request is 15 commits behind `main`, still merges without
  conflicts, its ten checks green on `761b52b2`, and the thread still waits on
  the reviewer.
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
  and there is no seam to read it from, so we log what remains: that the call
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
  `ServerSession.negotiatedLegacyProtocol` (the name `b6f3c555` gave it; only
  the test `TestSpeaksLegacyProtocol_NoHandshakeIsNotLegacy` keeps the old
  one), so a client negotiated down to 2025-11-25 by `initialize` is notified
  on the shared session channel rather than on a `subscriptions/listen` stream
  its version cannot open, and is no longer sent a per-session subscription id
  in `_meta`. A session that has
  recorded no version at all moves from legacy to new-protocol, which is what
  SEP-2575 says such a session is. No reviewer has answered since, and `main`
  gained three commits on 2026-09-28, so the branch was behind again. At 09:15
  UTC on 2026-09-29 @guglielmo-san updated it from `main` (merge commit
  `85f855fd`), and all nine checks passed on it. `main` gained two more
  commits the same morning, and at 12:42 UTC I updated it again with GitHub's
  Update branch (merge commit `bb0e7c5a`), which merges `main` in and so kept
  `85f855fd`, where a rebase would have dropped it; its nine checks passed
  again. It has had no review since 2026-09-15. Read on 2026-10-02 it is nine
  commits behind `main` again, unchanged since. Read on 2026-10-05 it is
  twelve commits behind `main`, still merges without conflicts, its nine
  checks green on `bb0e7c5a`, and has had no review since 2026-09-15. Its
  description was corrected that day (14:09 UTC), before the status note on
  the index named it: it had said a client that reaches 2026-07-28 through
  `server/discover` records no negotiated version, which has been false since
  [modelcontextprotocol/go-sdk#1274](https://github.com/modelcontextprotocol/go-sdk/pull/1274)
  (row 50); that all four sites read `negotiatedLegacyProtocol`, where all
  four read the version through `protocolVersion` and three of them through
  that predicate; and that `handle` does not gate a `resources/subscribe`
  sent before `initialize`, which
  [modelcontextprotocol/go-sdk#1273](https://github.com/modelcontextprotocol/go-sdk/pull/1273)
  (row 49) now refuses. Read on 2026-10-08 it is 15 commits behind `main` and
  otherwise the same. A downstream project tracks it,
  [vriesdemichael/bitbucket-data-center-cli#692](https://github.com/vriesdemichael/bitbucket-data-center-cli/issues/692)
  (open, "fix(mcp): upgrade go-sdk to v1.9.0 when it ships"), which names this
  pull request as the open fix for the issue above and plans a test of its own
  if the release after v1.8.0 carries it.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: none taken, deliberately. Disagreeing with the SDK here would
  be worse than matching it: our gate and `clientSupportsMultiRoundTrip` would
  choose differently for the same session, and the SDK labels the result.

**What**: `initialize` is deprecated in 2026-07-28, so `negotiatedVersion`
(`mcp/shared.go`) caps that handshake at `2025-11-25`, while
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

**Documented in**: the site's
[elicitation page](https://jmrp.io/docs/gitlab-mcp-server/capabilities/elicitation/#which-protocol-version-decides),
so the behaviour is stated where someone writing a client would look.

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
  [modelcontextprotocol/go-sdk#1328](https://github.com/modelcontextprotocol/go-sdk/issues/1328),
  opened by another user on 2026-10-01, reports that `LoggingTransport`
  forwards no `sessionUpdated`, so the fix does not reach a stdio transport
  wrapped in it. This server serves stdio over `mcp.IOTransport` itself
  (`cmd/server/main.go`) and wraps no transport, so it is not affected.
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
  Read on 2026-10-05 the issue has no comment, no label and no type; the
  status note on the index that day asked again.
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
and the only output was the listen request's own result: no
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
  Read on 2026-10-05 the issue has no comment, no label and no type; the
  status note on the index that day asked again.
- **Merged**: no.
- **Blocking**: no. One Conditionally Required attribute is omitted; the span is
  otherwise complete and the metric does not carry the attribute at all.
- **Workaround**: none possible. Nothing in the public API exposes the value.

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
  to be blocked and not a reason to keep the gap to ourselves. Its only
  triage since is the issue type Enhancement, which @guglielmo-san set on
  2026-09-14; read on 2026-10-05 it has no comment and no label, and go-sdk
  has no `proposal` label to give it.
- **In review**: no. Waiting on the maintainers to say which shape they want;
  the status note on the index on 2026-10-05 asked again.
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
  to open an issue with tests, and none had been opened when read on
  2026-10-05. On that pull request
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
  tag exists as of 2026-10-01, nor as of 2026-10-05.
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
  three tool results a receiving middleware here makes: the rate limiter's
  refusal, where it leaves the middleware (`attachRateLimitFunc` in
  `internal/toolutil/rate_limit.go`, the ToolError channel of row RTC-001), the
  held-call ceiling's (`heldRequestsRefusal` in `cmd/server/held.go`, row
  HLD-011), whose 2026-07-28 calls the gate counts, or refuses, before the SDK
  reads them, so its label is defensive, and the withheld refusal of issue 952,
  which the call middleware makes of a fine-grained session's call to a
  registered tool whose action it may not run, before the SDK decodes the
  arguments (`CallMiddleware` in `internal/tools/toolvisibility/fine_grained.go`,
  the Withheld channel of rows AUT-007 and AUT-008). The same refusal made by dynamic
  execute, inside its handler, is labeled by the dispatcher like any served
  call. No middleware here makes a `prompts/get` or `resources/read` result.
  Each refusal keeps its channel, its text and its error flag: row RTC-001
  declares the ToolError channel for `tools/call` so that a model reads the
  refusal as a tool result it can back off from, which moving it to the
  `-42900` JSON-RPC error the limiter writes for the other metered methods
  would have traded away, and rows AUT-007 and AUT-008 declare the Withheld channel, which
  a model reads as a tool result saying why and what to do. This entry used to say
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
existed. The third site, the withheld refusal of issue 952, was found reviewing
that layer against the binary, by reading the result a withheld call answers at
2026-07-28.

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
`TestCallMiddleware_WithheldCall_CarriesTheResultTypeOfItsRevision` in
`internal/tools/toolvisibility` and
`TestFineGrained_WithheldCall_EachRevision_CarriesTheResultTypeTheServedCallDoes`
in `test/e2e/http` hold the withheld refusal of issue 952 the same way, the
second beside a call the same fine-grained credential is served.

### A Go SDK client never sees a `subscriptions/listen` refusal

- **Reported**: yes, by another user:
  [modelcontextprotocol/go-sdk#1169](https://github.com/modelcontextprotocol/go-sdk/issues/1169),
  opened on 2026-08-13, reports it from `Client.Connect`'s side, and a comment
  on it of 2026-08-25, also not ours, reproduces it on an explicit
  `ClientSession.Subscribe` whose server refuses the URI, which is this
  server's case. It needs no issue of ours. Read on 2026-10-06: a maintainer,
  @guglielmo-san, closed it at 16:54 UTC on 2026-10-05 "as working as
  intended", for the listen `Connect` opens: that listen asks only for
  list-changed notifications and is optional, the session stays usable when
  a server refuses it, and failing `Connect` would break the clients of the
  servers that do. He called seeing that a subscription was refused or has
  ended "a fair need" and left it to the proposal
  [modelcontextprotocol/go-sdk#1284](https://github.com/modelcontextprotocol/go-sdk/issues/1284),
  an optional handler, which he said can be extended to the list-changed
  listen. That proposal is what is left of this entry upstream, and read on
  2026-10-06 it is open, with no comment on it and no label, and so again on
  2026-10-08.
- **In review**: yes, theirs,
  [modelcontextprotocol/go-sdk#1170](https://github.com/modelcontextprotocol/go-sdk/pull/1170),
  open since 2026-08-13 and conflicting with `main`. Its one review, on
  2026-08-17, points out that it checks for a refusal without waiting for one,
  so over any transport slower than an in-process one the client still reports
  success; nothing has moved since. As written it observes only a refusal that
  has already arrived, which covers a server answering the listen with a
  non-2xx status and not a refusal delivered on a 200 event stream. Read on
  2026-10-05 it still conflicts with `main`, in `mcp/transport.go`, 71
  commits behind it, and the review of 2026-08-17 is unanswered. A second
  pull request by another contributor,
  [modelcontextprotocol/go-sdk#1283](https://github.com/modelcontextprotocol/go-sdk/pull/1283),
  open since 2026-09-20, awaits the listen on a goroutine and clears the
  client's record of a URI when its listen ends without the client cancelling
  it, a refusal included, so a later `Subscribe` asks the server again;
  `Subscribe` itself still returns nil on the refusal, so it retires the half
  of the defect about a second `Subscribe` and not the rest. Its one review,
  of 2026-09-21, says it bypasses the middleware a user may have added, and
  is unanswered; it merges into `main` without conflicts, 27 commits behind
  it. The optional handler it leaves out is the proposal
  [modelcontextprotocol/go-sdk#1284](https://github.com/modelcontextprotocol/go-sdk/issues/1284),
  by the same contributor, which has had no response. Read on 2026-10-06:
  @guglielmo-san closed
  [modelcontextprotocol/go-sdk#1170](https://github.com/modelcontextprotocol/go-sdk/pull/1170)
  unmerged at 16:54 UTC on 2026-10-05, for the reason he gave on the issue,
  and at 17:28 UTC the same day added his own commit to
  [modelcontextprotocol/go-sdk#1283](https://github.com/modelcontextprotocol/go-sdk/pull/1283),
  which gathers the listen into one `awaitSubscriptionsListen` method,
  approved it at 17:34 UTC and said so at 17:35 UTC; a second maintainer
  approved it at 11:50 UTC on 2026-10-06.
  The review of 2026-09-21 about the middleware is answered by the merged
  code, which sends the listen through `handleSend` and so through the
  sending middleware.
- **Merged**: partly, by
  [modelcontextprotocol/go-sdk#1283](https://github.com/modelcontextprotocol/go-sdk/pull/1283),
  not ours: @guglielmo-san merged it at 13:51 UTC on 2026-10-06 (merge commit
  `6737e73d2d`). No release carries it: v1.8.0, of 2026-09-14, is still the
  newest tag, and `main` is 45 commits ahead of it on 2026-10-08. It retires
  the half about
  a second `Subscribe`, on the release that carries it, and not the half
  about seeing a refusal; What changes on the next go-sdk release, below,
  says how.
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

**What changes on the next go-sdk release**, read in the merged source on
2026-10-06 (`mcp/client.go`, `mcp/shared.go` and `mcp/transport.go` at
`6737e73d2d`): `callSubscriptionsListen` is gone from `mcp/transport.go`, and
`defaultSendingMethodHandler` in `mcp/shared.go` awaits the listen like any
other call; `Subscribe` starts `awaitSubscriptionsListen` on a goroutine, which
sends the listen through `handleSend` and so through the client's sending
middleware, and awaits its end. When the listen ends while the client has not
cancelled it, which a refusal is, the URI's `resourceSubs` entry is cleared,
so a later `Subscribe` sends a new listen instead of returning nil and sending
nothing. `Subscribe` itself still returns nil whatever the server answers
("Subscribe stays non-blocking"), and the stream's error is discarded once it
has passed the middleware, so the client application is still not told of a
refusal unless it installs a sending middleware of its own, which now sees the
listen's real result or error rather than an empty result. Seeing a refusal or
an ended subscription through the SDK's own API is the proposal named under
Reported.

**How we found it**: the issue 565 SDK probe, a go-sdk client subscribing
through a listen the server refused, which returned `err=<nil>`.

**Pinned by**: `TestListenRefusal_BeforeTheAcknowledgment_UnseenByTheModernSDKClient`
in `internal/tenancy/channels_integration_test.go`, which asserts over the three
transports that the refusal reaches the wire before any acknowledgment, that a
modern SDK client's `Subscribe` still returns nil, and that a legacy one sees
the code. The release that reports the refusal fails it. The release that
carries
[modelcontextprotocol/go-sdk#1283](https://github.com/modelcontextprotocol/go-sdk/pull/1283)
should not, since its `Subscribe` still returns nil, but the reason the test's
failure message gives, that the client does not wait for the listen it
opens, stops being the reason: re-read the test, and this entry, at the go-sdk
bump that brings that release in.

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
  it. The conflict is the author's to resolve. At 13:46 UTC on 2026-10-02
  another contributor reviewed it, in a comment marked AI-assisted and checked
  against `3e21beb4`: `reinitialize()` keeps the new session whenever the
  fresh `initialize` negotiates the same protocol version, and updates only
  the session ID, so a restarted server advertising other capabilities,
  server info or instructions on that version leaves the client deciding from
  the old result. It asks that the new session be adopted only when the result
  stays compatible, or that the fresh result reach the `ClientSession`. Read
  the same day the pull request still conflicts with `main`, now 15 commits
  behind it, and no maintainer has reviewed it. Read on 2026-10-05 it still
  conflicts with `main`, now 18 commits behind it, in the same two
  `mcpgodebug` documents, which
  [modelcontextprotocol/go-sdk#1315](https://github.com/modelcontextprotocol/go-sdk/pull/1315)
  also appended to on 2026-09-29; no workflow has run on its current head,
  the author has not answered the review of 2026-10-02, whose reading of
  `reinitialize()` holds against `3e21beb4`, and no maintainer has reviewed
  it. Read on 2026-10-08 it is 21 commits behind `main` and otherwise the
  same.
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
| the same head with `MCPGODEBUG=noprotocolerrorbody=1`                                                            | new session; the call resent and served            | new session; later calls served              | new session; notification resent and served |

A call was measured both ways the gate refuses one, under another credential on
a live session and under the owner's own after the idle timeout, and the two
agree. No configuration of v1.8.0 sends a second `initialize`.

So the pull request as proposed reaches this server's 404 wherever the 404 names
no request, which covers a default client whose session expired or was evicted:
its stream meets the 404 first, and it starts a new session before its next
call. It does not reach a call, because the body is decoded first and the error
never wraps `ErrSessionMissing`, and the switch that turns the decoding off
shows the body is the only thing in the way there. One more property of the
pull request is read from its source and was not measured: a session that
holds server state (a resource subscription or a logging level) is
deliberately not replaced, and the 404 stays terminal for it.

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
client re-initializes. Against the pull request's head the stream and notification
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
  `codex-rs/Cargo.toml` on its `main` still pinned `rmcp = "=3.2.0"` on
  2026-09-28, and the comment owed once 3.5.0 was tagged was posted the same day
  ([openai/codex#38979, comment 5874158831](https://github.com/openai/codex/issues/38979#issuecomment-5874158831)),
  naming the release and what the bump from 3.2.0 involves. On 2026-09-30 a
  Codex change for another reason,
  [openai/codex#49473](https://github.com/openai/codex/pull/49473) ("Use the
  rmcp SDK for enterprise-managed token exchanges", commit `c9b3924a`), moved
  the pin to rmcp 3.3.0, taken from git at the `rmcp-v3.3.0` release commit
  (`3e636cab`, 2026-09-10) for the enterprise authorization helpers that
  release added. That commit is 31 commits behind the fix's merge and
  carries none of it, so read on 2026-10-02 Codex's `main` still builds
  without the fix. So do its releases: `rust-v0.160.0`, the newest stable one
  (2026-10-01), still pins 3.2.0, and every alpha of the 0.161 and 0.162
  lines from `rust-v0.161.0-alpha.4` (2026-09-30) on pins 3.3.0, while the
  hotfix alphas of older lines published after it, `rust-v0.160.0-alpha.6.2`
  and `rust-v0.159.0-alpha.12.1`, still pin 3.2.0. Read again on 2026-10-05,
  all of that holds through `rust-v0.162.0-alpha.15`, published that day. A
  short note on the issue that day
  ([comment 5996186056](https://github.com/openai/codex/issues/38979#issuecomment-5996186056))
  said that the move to the 3.3.0 release commit still leaves `main`, the
  0.162 alphas and the latest release without the fix, and pointed again to
  the earlier comment on what the bump involves. Read on 2026-10-08,
  `rust-v0.160.0` is no longer the newest stable release: `rust-v0.160.1`
  (2026-10-05 18:29 UTC) still pins 3.2.0, and `rust-v0.161.0` (2026-10-07
  15:58 UTC), now the newest stable one, pins 3.3.0 from the same `3e636cab`,
  so it is built without the fix as well. Every alpha published since
  2026-10-05, through `rust-v0.162.0-alpha.20` (2026-10-08 02:25 UTC), pins
  3.3.0, and so does `main`. rmcp itself tagged `rmcp-v3.5.1` on 2026-10-05;
  3.5.0 is still the first release carrying the fix. No Codex maintainer has
  commented on the issue since it was opened on 2026-08-17.
- **Blocking**: it was. Every tool call failed with "Unexpected response type",
  so the server was unusable from Codex rather than degraded.
- **Workaround**: yes, and it is load-bearing. `internal/clientcompat` detects
  Codex and rounds annotation priorities to 0 or 1, which is spec-legal and
  parseable by both. `GITLAB_MCP_CLIENT_COMPAT=off` disables it. Retire it
  only once a Codex built on an rmcp carrying the fix is widely deployed, not
  merely released: the affected build ships inside ChatGPT.app, so users do not
  choose their version. Choosing a response from `clientInfo` departs from MCP
  2026-07-28, which says it SHOULD NOT change behavior; that departure is kept
  on purpose and stated in the security and client compatibility pages
  ([issue 959](https://github.com/jmrplens/gitlab-mcp-server/issues/959),
  register row `IDN-013`). The session's `clientInfo` decides whenever it has
  one, matched to the two spellings Codex has used since v0.20, a
  case-insensitive `codex-mcp-client` name prefix or a `title` of exactly
  `Codex`; that reaches stdio in either protocol era, HTTP with
  `--stateless=false`, and any session at 2026-07-28, whose requests each
  carry `clientInfo`. A Codex client on 2025-11-25 or earlier against the
  default stateless HTTP transport has none, because each POST there is a
  session of its own that never saw `initialize`, so a session with no
  `clientInfo` falls back to a case-insensitive `codex-mcp-client/` prefix of
  the request's User-Agent, which Codex's MCP client sends on every Streamable
  HTTP request (`codex-rs/rmcp-client/src/utils.rs`)
  ([issue 1043](https://github.com/jmrplens/gitlab-mcp-server/issues/1043),
  recorded on `IDN-013` beside issue 959 as a widening of the same
  deviation). `test/e2e/http` holds the fallback and its negatives against the
  binary. The `openai-mcp` question is settled
  ([issue 1044](https://github.com/jmrplens/gitlab-mcp-server/issues/1044),
  measured 2026-09-28): OpenAI's hosted MCP client, reached through the
  Responses API and the Realtime API, reports `clientInfo`
  `openai-mcp (Responses API)` or `(Realtime API)` and reads
  `annotations.priority: 0.6` without error on every model tried, so it does
  not have this defect and needs no profile. It sends no `title`, and its tool
  calls carry only the User-Agent `openai-mcp/1.0.0 (...)`. One label stays
  unmeasured, a ChatGPT web tool call's `openai-mcp/1.0.0 (Codex)`. The
  `codex` substring the profile matched until issue 1043 would have caught it
  if its `clientInfo` carries the same word; neither of the two matches it
  uses now does, and `test/e2e/http` pins both negatives.

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
[openai/codex#10334](https://github.com/openai/codex/issues/10334). Codex sends
only `structuredContent` to its model when both are present, dropping the
markdown. We keep emitting both.

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
  [gitlab-org/gitlab!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698),
  which went through the contributor platform's
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

  **On 2026-09-30** @narendran-kannan approved it for the backend (07:19 UTC),
  after accepting the answer on the experiment marker ("let's leave it
  as-is") and passing on the AppSec reviewer's one finding: that
  `doc/api/openapi/openapi_v2.yaml` listed `plan` as required while
  GitLab.com answers it `null`. The answer, at 09:45 UTC, agreed that the v2
  document described a string that is never `null`
  and fixed that at its source in `be9abac4`, where `expose :plan` now
  documents `x: { nullable: true }` and the regenerated document carries
  `x-nullable: true`. It kept `plan` in `required`, since the key is in every
  response and v2's `required` says only that, as the request specs' own
  response schema already does for `plan` and for two `kas` keys, and offered
  `required: false` instead if the reviewer prefers it. The fork pipeline on
  `be9abac4` failed one Duo Agentic Chat spec in `jest-msw-integration vue3`,
  which this change does not touch. Read on 2026-10-02 it carries that one
  approval, under the `All Members` rule, and still needs a maintainer's for
  each of the `/app/`, `/ee/`, `/ee/spec/`, `/lib/`, `/spec/` and `/public/`
  code-owner rules; the AppSec thread, answered by us, waits on the reviewer.

  **On 2026-10-05**, with nothing new since 2026-09-30 and no maintainer yet
  requested, a note to @narendran-kannan (14:09 UTC,
  [note 3956032195](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936#note_3956032195))
  said that his approval at 07:19 UTC on 2026-09-30 had come before our
  answer to the AppSec finding at 09:45, summed that answer up, offered again
  to take `plan` out of `required`, asked him to resolve his AppSec and trial
  license threads if the answers suit him, and asked him to pass the merge
  request to a maintainer, since each of the six code-owner rules above still
  needs one approval and a backend maintainer can give all six; it also said
  that the one failed job of the last pipeline, `jest-msw-integration vue3`,
  failed in a spec this change does not touch. Our own ready thread of
  2026-09-28 was resolved with it. At 14:40 UTC he answered in the trial
  license thread ("Codewise LGTM"), leaving that thread for @rsarangadharan
  to resolve on the documentation side, and under the note, leaving whether
  `plan` stays in `required` to the maintainer, and at 14:42 he asked @ck3g
  for the backend maintainer review and requested it. Read later that
  afternoon it carries his approval under `All Members` and none of the six
  code-owner approvals, on head `be9abac4`, with four threads open: the bot's
  nudge, the AppSec finding, the trial license question and our note.

  **On 2026-10-06** @ck3g reviewed it (09:41 UTC) with one blocking question
  and three non-blocking notes. The blocking one: the merge request lets
  every signed-in user read the instance plan, which today only an
  administrator can through `/license`, `/metadata` also accepts a token
  carrying only `read_user`, and the issue records no product decision about
  that, so he asked for a confirmation from the `group::entitlements` product
  manager on the issue before it merges. At 11:25 UTC we asked
  @ppalanikumar for that decision on
  [gitlab-org/gitlab#630305](https://gitlab.com/gitlab-org/gitlab/-/issues/630305)
  ([note 3962127074](https://gitlab.com/gitlab-org/gitlab/-/issues/630305#note_3962127074)),
  offering three answers: any authenticated user, as the merge request does
  now; only a token carrying `read_api` or `api`, as the GraphQL field
  already behaves; or administrators only, which `/license` already serves
  and which would make the merge request unnecessary. Our answer in his
  thread, posted with it, says where the question now stands and what the
  second answer would change in the merge request. Both of his non-blocking
  suggestions were taken two minutes later as commits: `da9e9d24` adds
  `License#trial_expired?`, which
  `License#feature_available?` and the `plan` of the instance metadata now
  both call, with a spec over the four trial and expiry combinations, and
  `eaf69493` says in `doc/api/metadata.md` and in the GraphQL field
  description that an expired paid license still reports its plan, with the
  GraphQL reference and both introspection files regenerated. His third note
  answered the question the AppSec finding had left to the maintainer:
  `plan` stays in `required` with `x-nullable: true`, which our reply accepts
  unless the product decision limits which tokens receive the key; the
  AppSec thread itself is still open. The push kept @narendran-kannan's
  approval under `All Members`, and the fork pipeline on `eaf69493` passed.
  Read that evening, none of the six code-owner approvals has been given,
  the threads of the blocking question and of both suggestions are answered
  and open, and the issue has no answer from the product manager. It waits
  on that decision.
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
  at the v3.15.0 pin) to gain the field, or a captured-response read under
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
[gitlab-org/gitlab#247915](https://gitlab.com/gitlab-org/gitlab/-/issues/247915) and
[gitlab-org/gitlab#219732](https://gitlab.com/gitlab-org/gitlab/-/issues/219732), neither with a
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
Re-read on 2026-10-05, it advertises eighteen fields, still not this one.

**Consequence for us**: the MCP authorization specification's audience-binding
MUST cannot be met by its named mechanism. Recorded as
[ADR-0019](adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md).

**Effort**: large, and it is a product decision rather than a patch.

### The merge request approvals GET answers 24 keys on EE under a four-key annotation

This entry was titled "The merge request approvals page documents the
deprecated POST's response under the GET" until 2026-10-05, and said the page
was wrong and the annotation right. That holds for Community Edition only; on
an Enterprise build, licensed or not, it is the other way round, which the
sections below record.

- **Reported**: in part, by GitLab, not by us:
  [gitlab-org/gitlab#408183](https://gitlab.com/gitlab-org/gitlab/-/issues/408183)
  ("Move merge request approvals API code to EE", opened 2023-04-20, open,
  milestone Backlog, `group::source code`) proposes merging the CE class into
  the EE module and verifying the documentation of the approval responses,
  "as it can be incorrect", and the EE override below cites it in a comment.
  Ours is the merge request below, opened 2026-10-05; a documentation-only
  change needs no issue first. It is related to that issue and to
  [gitlab-org/gitlab#602776](https://gitlab.com/gitlab-org/gitlab/-/issues/602776)
  ("Merge request approvals API returns approved as true in EE Free with no
  approvals", opened 2026-06-12 by another user), whose quoted answer from an
  unlicensed EE instance carries `approvals_required` and `approvals_left`,
  which only the EE entity exposes.
- **In review**: yes,
  [gitlab-org/gitlab!259766](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259766),
  opened 2026-10-05 from the community fork, documentation only. It says on
  the page which keys each edition answers the GET with, adds a response
  attribute table that marks the 20 keys only Enterprise Edition sends and the
  three deprecated ones, gives a complete example answer for each edition, and
  says that the approve and unapprove POSTs answer `201 Created` with the same
  keys. It leaves the routes' `desc` annotations, and so the OpenAPI document,
  as they are, and asks the reviewer which of two directions they want for
  them: an EE entity prepended onto the CE one, or the move
  [gitlab-org/gitlab#408183](https://gitlab.com/gitlab-org/gitlab/-/issues/408183)
  proposes. It was readied naming @uchandran, whom the bot set as reviewer and
  asked for the documentation review. Read on 2026-10-06 it has no approval,
  no milestone, a green fork pipeline and no thread open but the ready note's,
  and waits on that review. It retires nothing of ours: the shape declarations
  under Workaround wait on the annotation, which it leaves. Read again at
  21:40 UTC on 2026-10-07: @uchandran set milestone 19.5 at 13:50 UTC and
  asked GitLab Duo for a review, which raised one minor point (a link the page
  already carries) and requested changes; at 20:47 UTC @uchandran left eleven
  comments on the page, most of them suggestions, asked @Saahmed to verify the
  change as a subject matter expert, and asked @idurham whether the `.rb`
  files need the same change, which is the annotation question the merge
  request leaves open. None of the comments was answered at that read. They
  were that evening: at 22:22 UTC the ten suggestions went in as one commit
  (`7b412cff`); at 22:36 UTC the two questions were answered in their
  threads (every EE build sends both approval-rule availability attributes,
  `false` without the license, so the suggestion was applied with its twin
  on `multiple_approval_rules_available`; `require_password_to_approve` stays,
  since EE still sends it and REST removes it only in v5), and a note thanked
  @uchandran and answered @idurham that the page is written by hand, so the
  `.rb` files need no change for it, and that their annotations disagree with
  it through the OpenAPI document generated from them, a change that needs a
  backend review and is offered as a follow-up; at 22:39 UTC
  `f3aebdb0` dropped the unsupported version from
  `require_password_to_approve` and review was requested again from
  @uchandran and GitLab Duo, which answered that it could not run for this
  project (`DCR4003`). Read on 2026-10-08 the fork pipeline on `f3aebdb0` is
  green, and no reviewer holds requested changes any more: requesting GitLab
  Duo again reset its state, which reads reviewed after its `DCR4003`
  answer, and the reviews of @uchandran and @Saahmed are pending. There is no
  approval, the milestone is 19.5, and @Saahmed has not answered.
- **Merged**: no.
- **Blocking**: no upstream block. Trusting the CE annotation cost this
  server data until the change under Workaround:
  `merge_request.approval_config` published the four keys of `ConfigOutput`
  on every tier, so on GitLab.com and on every EE instance a model was given
  4 of the 24 keys GitLab sends; `merge_request.approve` published five of
  them, with `approvals_required` reading 0 on CE, where GitLab does not send
  it; and `merge_request.unapprove` published nothing of its answer. The
  approve and unapprove POSTs answer through the same helper (below).
- **Workaround**: yes, for the annotation, and what this entry used to call
  one was a defect of ours. `configToOutput` in
  `internal/tools/mrapprovals/mr_approvals.go` cut `ConfigOutput` down to the
  four keys a Community Edition instance sends
  ([issue 580](https://github.com/jmrplens/gitlab-mcp-server/issues/580)),
  and twenty `docOmittedFields` entries in
  `cmd/audit_1to1/internal/structs/analyze.go`, citing GitLab's generated
  OpenAPI document, excused the cut by declaring those keys absent on every
  tier. The three actions now read the twenty EE keys off the captured
  response
  ([ADR-0021](adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md))
  into `mrapprovals.EnterpriseApprovalState`: each key is absent when GitLab
  did not send it (a pointer, or for the two timestamps a string left empty)
  and each list is published only when GitLab sent one, so a CE answer
  carries none of them and an EE one keeps its zeros, such as
  `approvals_left: 0`.
  client-go's `MergeRequestApprovals` cannot serve for them: its value-typed
  fields read zero whether or not the key was sent, it has no
  `invalid_approvers_rules`, it types `approval_rules_left` with the whole
  rule where GitLab sends the short reference, and `UnapproveMergeRequest`
  discards the body. `approvals_before_merge`, which it still models and no
  edition sends here, stays out. What remains is one shape declaration per
  key on `mrapprovals.ConfigOutput` in
  `cmd/audit_1to1/internal/paths/shape_declarations.go`, because the live
  record reads the CE annotation of the three routes; they retire when GitLab
  annotates the EE answer with `ApprovalState`.

**Where**: `lib/api/merge_request_approvals.rb` presents
`GET /projects/:id/merge_requests/:merge_request_iid/approvals` through
`present_approval`, which renders `::API::Entities::MergeRequestApprovals`,
and its `desc` names that entity. `ee/lib/ee/api/merge_request_approvals.rb`
prepends a `present_approval` that renders `merge_request.approval_state` with
`::API::Entities::ApprovalState` (`ee/lib/api/entities/approval_state.rb`),
under a comment, "Overrides helper from CE", that links
[gitlab-org/gitlab#408183](https://gitlab.com/gitlab-org/gitlab/-/issues/408183).
The same `present_approval` renders the answers of
`POST /projects/:id/merge_requests/:merge_request_iid/approve` and
`POST /projects/:id/merge_requests/:merge_request_iid/unapprove` in the CE
file, whose `desc` blocks name `MergeRequestApprovals` as well
(`success code: 201, model: ::API::Entities::MergeRequestApprovals`), so on an
EE build all three routes answer `ApprovalState`, and
`docs/development/gitlab-api-live.json` records both POSTs under
`MergeRequestApprovals` too. The page is `doc/api/merge_request_approvals.md`,
"Retrieve approval state for a merge request"; its "Approve merge request"
section shows an example of 12 of the EE keys and its "Unapprove a merge
request" section none. The two source files and the page were read on
`master` on 2026-10-05.

**What**: Community Edition answers the GET with the four keys
`MergeRequestApprovals` exposes: `user_has_approved`, `user_can_approve`,
`approved` and `approved_by`. An Enterprise build answers with `ApprovalState`,
which merges in the `IssuableEntity` of the merge request and exposes each of
its own keys with no condition: measured on GitLab.com on 2026-10-05,
`GET /projects/278964/merge_requests/259297/approvals` answers 24 keys (`id`,
`iid`, `project_id`, `title`, `description`, `state`, `created_at`,
`updated_at`, `merge_status`, `approved`, `approvals_required`,
`approvals_left`, `require_password_to_approve`, `approved_by`,
`suggested_approvers`, `approvers`, `approver_groups`, `user_has_approved`,
`user_can_approve`, `approval_rules_left`, `has_approval_rules`,
`merge_request_approvers_available`, `multiple_approval_rules_available` and
`invalid_approvers_rules`). The route's `desc` names `MergeRequestApprovals` in
both editions, so `doc/api/openapi/openapi_v3.yaml` and this repository's
`docs/development/gitlab-api-live.json`, taken from an EE image, both describe
the four-key answer, which no EE build sends. The page's example under the GET
shows 13 of the 24 EE keys and never says that Community Edition answers four,
though it does say that `approved` means something different in each
edition.

**How we found it**: `gitlab_mr_approval_config` published all twenty-four
fields of `gl.MergeRequestApprovals` and every one of the twenty extra arrived
as a zero. The 1:1 audit was green throughout, because it compares our type
against the SDK type and the SDK type models the POST. It surfaced when
a record of GitLab's own gave the audit an oracle that speaks for it, and the
e2e suite had recorded the live CE observation months earlier without anyone
connecting it to the output type. That reading was half right: the zeros were
the CE answer, and the record speaks for the annotation rather than for what an
EE build presents. A status review on 2026-10-05 read the same GET on
GitLab.com and found the 24 keys, which the EE prepend above explains.

**Effort**: small upstream, a documentation and annotation correction that
says what each edition answers, covering the GET and the approve and
unapprove POSTs, which share the helper and the annotation, or the move
[gitlab-org/gitlab#408183](https://gitlab.com/gitlab-org/gitlab/-/issues/408183)
proposes. The documentation half is in review and the annotation half is the
question it asks (In review); the change on this side is made (Workaround).

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
  twelve minutes later, when
  [gitlab-org/gitlab!256904](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256904)
  merged, after the rebase
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
  without it, so 19.5 is the first release that will. Read on 2026-10-05 that
  still holds, and it is deployed on GitLab.com: the release tooling labelled
  it `workflow::production` at 10:26 UTC on 2026-09-29.
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
recorded below. A `desc` block naming an entity the handler does not present is
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

- **Reported**: yes, by the merge request below; a change to an annotation and
  its documentation needs no issue first.
- **In review**: yes,
  [gitlab-org/gitlab!260487](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260487),
  "Annotate the context commit list with the entity it presents", opened from
  the community fork at 19:43 UTC on 2026-10-07 and readied at 19:49 (Fix in
  review, below). The documentation automation requested the code owner of
  the page, @uchandran, and no backend reviewer was assigned until 22:10 UTC,
  when a `@gitlab-bot ready @egrieff` with the `backend`, `type::bug` and
  `group::code review` labels drew a request to @egrieff; its fork pipeline,
  still running at 20:04 UTC, passed at 20:28. Read on 2026-10-08 its
  reviewers are @egrieff and @uchandran, and it has no review yet, no
  approval and no milestone.
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

**Fix in review**: the `desc` of the list route declares
`Entities::CommitWithLink`; both OpenAPI documents point its `200` at that
entity and gain its definition and `UserPath`'s, spelled
`APIEntitiesCommitWithLink` and `APIEntitiesUserPath` in v3 and
`API_Entities_CommitWithLink` and `API_Entities_UserPath` in v2
(the v3 document regenerated whole, the v2 document given only the hunks this
route causes, since a full v2 run rewrites about 12,400 lines that have drifted
on `master`); `doc/api/merge_request_context_commits.md` gets an example
request and an example response with every key the route sends, recorded in a
request spec run; and a new example in `spec/requests/api/merge_requests_spec.rb`
expects the keys of the response to be exactly the declared entity's
exposures, so the two cannot drift apart again without a failure. Against
`master` without the annotation change, that example fails on exactly the ten
keys `CommitWithLink` adds. The four keys that are always null are documented
as they arrive and left on the route, as a second question the merge request
names and does not answer. When a release carries it, this entry's Workaround
field says what follows: the four shape declarations go stale and
`cmd/gen_api_live` against that release turns the six keys this server does
not publish into findings of the sent direction.

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
  and it waits on that pipeline. Of the two merge requests still open that day,
  [gitlab-org/gitlab!254189](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254189)
  was closed unmerged by its author at 11:36 UTC on 2026-09-30, and
  [gitlab-org/gitlab!256404](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256404)
  was still open when read on 2026-10-05, and on 2026-10-08, after a rebase
  and a maintainer review requested on 2026-10-07.

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

  **It merged on 2026-10-02.** @marc_shaw approved it at 09:45 UTC on
  2026-09-30, with one non-blocking suggestion: delete the auto-merge
  `before` block of the endpoint's spec and the original example, now that
  the block was known to do nothing, rather than describe them in a comment.
  @uchandran approved the documentation half again at 13:48 UTC on
  2026-10-01, and @marc_shaw resolved every thread and set it to join the
  merge train at 16:10. The suggestion was taken at 20:40 UTC in `78b2ba7a`,
  with `45697162` after it: the block had been creating a merge request in
  every example, which moved the id sequence along before `returns 404 if the
  merge request id is used instead of iid` ran, and without it that example,
  run on its own against a fresh test database, creates a merge request whose
  id and iid are both `1` and answers `201`, so it now creates one in another
  project first. Both commits touch only
  `spec/requests/api/merge_requests_spec.rb`, so @uchandran's approval stood;
  the push reset @marc_shaw's and took it off the train, which the ready note
  that followed said, asking for the approval, a pipeline and the train
  again. On 2026-10-02 @marc_shaw asked GitLab Duo to review it, whose one
  finding (09:18 UTC) was that the rewritten status table leaves out the
  `405 Method not allowed` the endpoint's `failure` list declares. Our answer
  (10:32 UTC) kept the `405` in the `failure` list and out of the table: it
  is not a routing answer but `find_project!` refusing a non-`GET` request
  that names a moved project by its old path, which most project endpoints do
  the same way, so a row on this endpoint alone would read as particular to
  it, and the REST API page, which describes only the `GET` redirect, is where
  it belongs; that is
  [row 92](#the-rest-api-page-does-not-say-a-non-get-request-to-a-moved-projects-old-path-is-answered-405).
  @marc_shaw approved it again at 11:21 UTC with one non-blocking note, that
  the other "id is used instead of iid" examples in that spec file share the
  same dependence on a fresh sequence (three he tried answer `201` after
  restarting `merge_requests_id_seq`), so the guard fixes one of thirteen,
  offered as a follow-up if we want one; it has not been opened. He resolved
  every thread at 11:24 and started a merge train at 12:23, and it merged at
  12:25 UTC (merge commit `acc7dd86`, squash commit `d1cd3e49`), in milestone
  19.5.

  **The first attempt,
  [gitlab-org/gitlab!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239), was
  closed unmerged, and why is the useful part.** It changed the handler to match
  the documentation, on the reasoning that unlike
  [gitlab-org/gitlab!254698](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254698), which
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
- **Merged**: yes, both halves, and unreleased.
  [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702),
  the new `cancel_auto_merge` endpoint, merged into `master` on 2026-09-29
  (milestone 19.5, merge commit `043d425d`), and
  [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704),
  the old endpoint's documentation and deprecation, on 2026-10-02 (milestone
  19.5, merge commit `acc7dd86`). Held to the tags that contain those
  commits, read on 2026-10-02, neither is in a release yet, the newest being
  `v19.4.1-ee`; 19.5 is due on 2026-10-15. Read on 2026-10-05, that still
  holds, and both are deployed on GitLab.com: the release tooling labelled
  `gitlab-org/gitlab!255702` `workflow::production` at 05:48 UTC on
  2026-09-30 and `gitlab-org/gitlab!255704` at 04:28 UTC on 2026-10-05.
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

### The REST API page does not say a non-GET request to a moved project's old path is answered 405

- **Reported**: yes, by the merge request below; a documentation-only change
  needs no issue first.
- **In review**: yes,
  [gitlab-org/gitlab!259297](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259297),
  "Document the 405 for non-GET requests to a moved project", opened from the
  community fork at 10:32 UTC on 2026-10-02 and readied at 10:36. The
  documentation automation requested the technical writer @fneill, who
  approved it at 14:41 UTC and set it to join the merge train.
- **Merged**: yes, at 14:50 UTC on 2026-10-02 (merge commit `804725ee`), in
  milestone 19.5. Held to the tags that contain that commit, read the same
  day, it is in no release yet; 19.5 is due on 2026-10-15. Read on
  2026-10-05 it is still in no tag, and it is deployed: the release tooling
  labelled it `workflow::production` at 04:28 UTC that day, and the REST API
  page on docs.gitlab.com already carries the 405 sentence.
- **Blocking**: no. A request that names the project by its ID or by its
  current path is never refused this way.
- **Workaround**: not yet.
  [Issue 1133](https://github.com/jmrplens/gitlab-mcp-server/issues/1133) is
  for this server to describe the refusal, so that a model retries with the
  project's ID instead of reading the generic 405 sentence of
  `internal/toolutil/errors.go`, "the action cannot be performed on this
  resource in its current state", as a statement about the merge request,
  issue or branch it was acting on.

**Where**: the [Redirects](https://docs.gitlab.com/api/rest/#redirects)
section of `doc/api/rest/_index.md`, and `find_project!` in
`lib/api/helpers.rb`.

**What**: the page said that after a path change the API answers with a
redirect to the new location. That holds for `GET` only. For any other method,
`find_project!` answers a caller who can read a moved project and names it by
its old path with `405 Method Not Allowed` and the message
`Non GET methods are not allowed for moved projects`, which
`spec/requests/api/projects_spec.rb` pins for the archive endpoint. A caller
who cannot read the project is answered `404` before that check, which the
same spec pins.
`project_moved?` is true only when `:id` is a path other than the project's
current one. Most project endpoints look `:id` up through that helper; a few
use `find_project`, which follows the redirect without the check, so a
non-`GET` request with the old path is processed there as usual
(`POST /projects/:id/trigger/pipeline` is one). `HEAD` is refused too, since
the helper's `get?` check is false for it whether Grape serves it through a
`GET` route, as for a raw file, or through one declared with `head`, as for a
file's metadata, and its refusal has no body.

**Fix merged**: the merge request adds to the section which methods are
refused, the response body and what to send instead. It leaves `HEAD` out of
the list of methods because its answer has no body to show, and says most
endpoints rather than all of them, for the `find_project` exceptions above.

**What it costs this server**: it passes `project_id` to GitLab as given, so a
model that names a project by a path it saw before the move can read the
project and then fail every write to it, and two reads as well,
`repository.file_metadata` and `repository.file_raw_metadata`, which are
`HEAD` requests. The error leads with the generic 405 sentence, with GitLab's
message after it, and three handlers that key on the status alone give a wrong
cause instead: `merge_request.merge`, `merge_request.cancel_auto_merge` and
`access.token_project_rotate_self`. Issue 1133 records each.

**How we found it**: answering GitLab Duo's one finding on
[gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)
([row 46](#cancelling-an-auto-merge-answers-a-status-hash-under-a-merge-request-annotation)),
that its status table left out the `405` the endpoint's `failure` list
declares. Tracing where that `405` comes from showed it was this helper, not
the endpoint, and that the REST API page did not describe it.

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
  permission close it. Read again on 2026-10-02 nothing of that has changed.
  Read on 2026-10-05 the issue is still open (milestone Backlog,
  `group::repository services`) and no tag carries `4033c4fa`, but GitLab.com
  runs the fix: the release tooling labelled the merge request
  `workflow::production` at 07:46 UTC on 2026-09-24, so GitLab.com no longer
  offers a revoked UID for verification; a signature it had already verified
  under a key that stays in place keeps its stored result, since the fix
  re-derives only subkey-signed rows and rows of a key removed and added
  again. A self-managed instance waits for 19.5.
  The documentation followed from somebody else: @brendan777, one of the
  merge request's three approvers, opened
  [gitlab-org/gitlab!255357](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255357),
  "Distinguish revoking a GPG key from revoking a user ID", which cites it
  and was merged on 2026-10-01 (milestone 19.5, merge commit `612f032e`). It
  adds to the "Revoke a GPG key" section of
  `doc/user/project/repository/signed_commits/gpg.md` that revoking a key in
  GitLab marks every commit signed with it unverified while revoking one of
  its user IDs with GnuPG affects only the commits using that ID's address,
  and corrects the procedure's last step.
- **Blocking**: no.
- **Workaround**: none possible. The verdict is computed inside GitLab and
  served as one string; nothing on this side can tell a signature verified
  under a live identity from one verified under a revoked one. An instance is
  fixed once it runs a release that carries the merge; GitLab.com has run it
  since 2026-09-24.

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

- **Reported**: yes, by the merge request below; a documentation-only change
  needs no issue first.
- **In review**: yes,
  [gitlab-org/gitlab!260143](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260143),
  opened on 2026-10-06 from the community fork, against `master` after the
  change below merged: both transfer sections say the move runs in the
  background, what the request returns (`200 OK` for a project, `201 Created`
  for a group, the object before the move), how to confirm the move, which
  checks refuse with `400` before it and which can only fail in the worker,
  how a failure is reported and where its reason is readable, and what a
  resend does, and the to-do page's `action` and `type` lists gain every
  value the route accepts. It suggests the stages' technical writers without
  mentioning them. It was readied at 20:31 UTC on 2026-10-06, and the bot
  requested @z_painter for the documentation review. At 16:23 UTC on
  2026-10-07 @z_painter requested changes, with suggestions for the history
  lines, footnotes in place of the long `action` and `type` value lists, and
  a shorter confirmation text without the GraphQL part, and stopped the review
  to ask @shubhamkrai about GraphQL work on transfers for 19.5 and whether the
  user documentation needs a change too. Read at 21:40 UTC, none of it had
  been answered. It was that evening. At 22:08 UTC a note answered on the user
  documentation: the transfer sections of `doc/user/group/manage.md` and
  `doc/user/project/working_with_projects.md` already say the transfer is
  asynchronous, the change below updated them for 19.5, and the only gap is a
  19.5 history item for the to-do item's reason, offered for both pages. Two
  suggestions went in at 22:22 UTC (`f9433e72`), and at 22:43 UTC each thread
  was answered and the rest committed by hand in `fe758318` (22:46 UTC):
  the footnotes under one label, since the suggested reference and definition
  did not match; the blank lines markdownlint asks for kept; the history
  change made on the project page too; and the confirmation text moved to the
  end of each section. That changes what the merge request documents, so the
  sentence above that lists it describes the merge request as it was
  readied: after the review, both
  transfer sections say what the request returns, how to confirm the move and
  how a failure is reported, with a history item for the 19.5 name and path
  check on the project page, and leave out the checks that answer 400, what a
  resend does and the GraphQL reason, which its description still sets out
  as evidence. Two questions went to @shubhamkrai at 22:43 UTC: whether REST
  is to serve the failure reason too, since the change below added it to the
  GraphQL to-do type only, and whether a resend before the first worker
  starts, which this entry's What paragraph describes, is expected to leave a
  `transfer_failed` item for a transfer that succeeded. Read on 2026-10-08
  the fork pipeline on `fe758318` is green, the state is still
  `requested_changes` (@z_painter's), and it has no approval and no
  milestone; it waits on @z_painter and @shubhamkrai.
- **Merged**: no.
- **Blocking**: no. The transfer is accepted and applied; what misleads is the
  answer and the pages that describe it: the object in the answer (a 200 for
  a project, a 201 for a group) is where it still is, a failure after that answer reaches the caller only as a to-do
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
  reference pages do not say so. From 19.5 the failure's reason is readable
  over GraphQL (below), and reading it when the wait finds the move failed,
  so that the error carries GitLab's reason rather than the list of checks
  the hints give, is
  [issue 1222](https://github.com/jmrplens/gitlab-mcp-server/issues/1222).

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
in the 19.4 milestone. In 18.11 it gated only the web controllers: the two
REST routes have no `enqueue_async_transfer` at `v18.11.12-ee` and read the
flag from `v19.0.0-ee`, so through the API 19.0 to 19.3 behave the same way
when it is enabled, and 18.11 transfers inline (corrected on 2026-10-06; this
paragraph had said 18.11 to 19.3).

**What**: both routes run the checks of `ensure_allowed_transfer`
synchronously, move the namespace's state machine to `transfer_scheduled`,
enqueue the worker, and answer with the object as it stands, before the
worker moves it: `200 OK` for a project and `201 Created` for a group. For a project that synchronous half is only the blank
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

**Read on 2026-10-05, GitLab is changing two of the facts above**, so the
proposal waits on that change. In
[gitlab-org/gitlab!251614](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/251614)
("Surface why a group or project transfer failed", open, milestone 19.5, by
@rymai), for
[gitlab-org/gitlab#597297](https://gitlab.com/gitlab-org/gitlab/-/issues/597297)
(`severity::1`, `priority::1`, milestone 19.5, `workflow::in review`), the
project name and path conflict check moves into
`Projects::TransferService#ensure_allowed_transfer`, so that collision
becomes a synchronous 400 for a project as it already is for a group, and the
reason a transfer failed is kept and served on the to-do item as the new
GraphQL field `transferFailureReason`. It touches neither `doc/api/projects.md`,
`doc/api/groups.md` nor `doc/api/todos.md`, nor `lib/api/entities/todo.rb`, so
the reason is readable through GraphQL only: `waitpoll.TransferFailed`, which
lists to-do items over REST, would need a GraphQL read to report it.

**Read on 2026-10-06, that change is merged.**
[gitlab-org/gitlab!251614](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/251614)
merged at 14:17 UTC (merge commit `87449438`, squash commit `d33044b9`),
in milestone 19.5 and in no tag yet, and
[gitlab-org/gitlab#597297](https://gitlab.com/gitlab-org/gitlab/-/issues/597297)
was closed at 14:54 UTC (`workflow::complete`). Its final shape is the one
described above. `Projects::TransferService#ensure_allowed_transfer` raises
"Project with same name or path in target namespace already exists" itself,
so from 19.5 a project's name or path collision is a synchronous 400 as a
group's is, and the check is gone from `#transfer`; the pending deletion,
container registry and npm checks stay in the worker. The worker keeps the
message of the error it failed with on the namespace that carries the
transfer state (the group, or the project's project namespace), and
`Todo#transfer_failure_reason` serves it on a `transfer_failed` item as the
GraphQL field `Todo.transferFailureReason`, new in 19.5, beside
`transferFailedRetryUrl`. The merge request changed
`doc/user/group/manage.md` and `doc/user/project/working_with_projects.md`,
which now say that a failed transfer sends no email and leaves the user who
started it a to-do item with the reason, and the GraphQL reference; it did
not touch `lib/api/entities/todo.rb`, `doc/api/projects.md`,
`doc/api/groups.md` or `doc/api/todos.md`, so the REST to-do item still
carries no reason and every gap of the three API pages above stands. The
proposal no longer waits: the docs merge request was opened the same day as
gitlab-org/gitlab!260143 (In review, above), written against 19.5's behaviour
(the project collision refused before the answer, and the reason on the to-do
item over GraphQL). Reading the reason on our side is
[issue 1222](https://github.com/jmrplens/gitlab-mcp-server/issues/1222),
opened on 2026-10-06. The release tooling labelled
[gitlab-org/gitlab!251614](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/251614)
`workflow::production` at 05:55 UTC on 2026-10-07, so GitLab.com runs it; no
tag carries it yet.

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
  carries it yet; 19.5 is the first release that will. Read on 2026-10-05 that
  still holds, and it is deployed on GitLab.com: the release tooling labelled
  it `workflow::production` at 05:47 UTC on 2026-09-30.
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
  writer @fneill, whom the bot set as reviewer. @fneill approved it at 10:26
  UTC on 2026-09-30 and asked @aalgutifan, of the Orbit team, for the
  technical review, who approved it at 17:22 UTC on 2026-10-02 without a
  comment. Read that evening it has both approvals, needs no other, has a
  green fork pipeline and no milestone, and its one open thread is the ready
  note's; it waits on being merged. That thread was resolved at 00:51 UTC on
  2026-10-03, and read on 2026-10-04 GitLab reports it mergeable, with nothing
  left but a maintainer to merge it. Read on 2026-10-05 nothing has changed
  since the approvals, and a note asking for the merge is planned for
  2026-10-06. Read again late on 2026-10-05, it is unchanged. The note went
  to @fneill at 08:15 UTC on 2026-10-06, and said too that Danger had noted
  the missing milestone and `documentation` label; at 09:50 UTC @fneill set
  milestone 19.5 and the label, and set it to join the merge train.
- **Merged**: yes, at 09:59 UTC on 2026-10-06, by @fneill through the merge
  train (merge commit `ff55853d`, squash commit `82920a47`), in milestone
  19.5. No release tag carries it yet, so 19.5 is the first release that
  will; it reached GitLab.com's canary stage at 17:14 UTC the same day
  (`workflow::canary`), the staging stage at 23:42 UTC and the production
  stage at 00:17 UTC on 2026-10-07 (`workflow::production`).
- **Blocking**: no.
- **Workaround**: yes, with
  [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031).
  This server never points a model at the page's examples, and what it does
  teach is version 12: the schema description of `orbit.query`'s `query`,
  its parameter guidance and the site's Orbit page, whose four examples, one
  per query type, GitLab.com compiled and answered on 2026-10-05. Nothing
  here waits on the merge request; it retires nothing of ours.

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

**Fix merged**: the merge request rewrites the five examples in version 12
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

- **Reported**: yes, by the merge request below, which covers
  [row 79](#an-unknown-report-type-on-a-pipelines-findings-list-is-dropped-and-filters-out-every-finding)
  as well.
- **In review**: yes,
  [gitlab-org/gitlab!260459](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260459),
  "Refuse unknown filter values on pipeline security findings", opened from the
  community fork at 18:28 UTC on 2026-10-07 and readied at 18:33 (Fix in
  review, below). The documentation automation requested @sarahwatt for the
  regenerated GraphQL reference, and no backend reviewer was assigned that
  evening. Its fork pipeline failed on `graphql-verify`, which asks for the two
  introspection results under `public/-/graphql/` to be regenerated after the
  argument descriptions changed, so the branch needed one more change before a
  review. It had it that evening: at 22:17 UTC the single commit was amended
  as `c16e389d`, now carrying `public/-/graphql/introspection_result.json`
  and `introspection_result_no_deprecated.json` as well, and its fork
  pipeline, 2924272652, passed. At 22:35 UTC a
  `@gitlab-bot ready @uokeadu`, since the finder and the models belong to
  Security Insights, drew a backend review request to @uokeadu; @sarahwatt
  stays the documentation reviewer. Read on 2026-10-08 it has no review yet,
  no approval and no milestone.
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

**Fix in review**: the resolver validates `severity` and `reportType` before
the finder runs, against the enums the finder reads
(`Security::Finding.severities` and `Security::Scan.scan_types`), and answers an
unknown value with a `Gitlab::Graphql::Errors::ArgumentError` naming the
argument, the value and the values it takes; the argument descriptions list
those values and the GraphQL reference is regenerated. The argument types stay
`[String!]`: typing them with enums, which the proposal here preferred, changes
a type every client declaring the variables as strings is refused for, the
pipeline security tab's own query among them, and GitLab's deprecation process
counts that as a breaking change, so the merge request leaves the migration to
the issue GitLab already has open for it. A list mixing a known
and an unknown value is refused whole, and the values stay case sensitive, as
they are today; the merge request asks the reviewer about both choices. When a
release carries it, `findingSeverities` in `securityfindings` only duplicates
what GitLab refuses itself.

### An unknown report type on a pipeline's findings list is dropped and filters out every finding

- **Reported**: yes, by the merge request of
  [row 78](#an-unknown-severity-on-a-pipelines-findings-list-answers-500).
- **In review**: yes,
  [gitlab-org/gitlab!260459](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260459),
  open, the one row 78 follows.
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

**Fix in review**: the resolver check of
[row 78](#an-unknown-severity-on-a-pipelines-findings-list-answers-500)
refuses a `reportType` value `Security::Scan.scan_types` does not have, in the
resolver rather than in the finder, because the finder's REST caller,
`GET /projects/:id/vulnerability_findings`, defaults `report_type` to every
vulnerability report type, `generic` and `container_scanning_for_registry`
included, and relies on `sanitize_scan_types` to drop them. That REST route
keeps dropping either one asked for alone, which the merge request names and
offers to fix there or in a follow-up. When a release carries it,
`findingReportTypes` in `securityfindings` only duplicates GitLab's refusal on
the releases that have it, and still decides on the older ones, `SARIF` before
18.11 among them.

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
answers it with 500 (entry 57). Their merge request,
[gitlab-org/gitlab!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486),
opened on 2026-10-07, makes both answer 403 and leaves the update and the list
at 401, which are this entry's `update_service.rb:40` and `status_checks.rb:67`;
it offers to give them `authorize!` too, in the same merge request or a
follow-up, which would be the first sites of this entry to move.

### Deleting an external status check without the role answers 204 and deletes nothing

- **Reported**: yes, by the merge request below, which covers
  [entry 57](#creating-an-external-status-check-without-the-role-answers-500)
  as well.
- **In review**: yes,
  [gitlab-org/gitlab!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486),
  "Answer external status check create and delete role refusals with 403",
  opened from the community fork at 19:43 UTC on 2026-10-07 and readied at
  19:48 (Fix in review, below). No reviewer was assigned, so at 20:09 the
  triage bot moved it back to `workflow::in dev`, where it waited for a
  reviewer to be asked. One was at 22:16 UTC: a
  `@gitlab-bot ready @sashi_kumar` with the `backend`, `type::bug` and
  `group::policy management` labels, whose note says an earlier fix to these
  routes was reviewed and merged by Security Policies, put it back at
  `workflow::ready for review`, and the bot requested @sashi_kumar.
  Read on 2026-10-08 the bot has added the section, stage and category
  labels of security governance (00:37 UTC), its fork pipeline is green, and
  it has no review yet, no approval and no milestone.
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
  role there. What retires it is the route refusing the role before the
  service runs, below; the handler then hints the Maintainer role on that 403
  and keeps the license hint for the 401 the license check still answers, and
  the usage stops asking the caller to confirm the deletion with
  `external_status_check.list_project`.

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

**Fix in review**: the fix is in the route, not in the service. The delete
and the create routes of `ee/lib/api/status_checks.rb` call `authorize!` on the
ability and subject the service checks (`:delete_external_status_check` on
the check, `:create_external_status_check` on the project) before the service
runs, so a caller without the Maintainer role is answered
`403 {"message":"403 Forbidden"}` and nothing changes, the way the merge
request routes of the same file already refuse through
`find_merge_request_with_access`. The delete also renders the service's error
from inside the `destroy_conditionally!` block, so a destroy that fails is
answered 422 with the model's errors where it was answered 204 too. The
services are left as they are, since the route now asks their question first,
and so are the update and list routes, which still answer the same missing
role with 401: that is
[entry 55](#a-permission-refusal-is-answered-401-rather-than-403), and the
merge request offers to align them here or in a follow-up, since moving a
status a client may depend on is a breaking change under GitLab's API style
guide while these two answered wrongly outright.

### Creating an external status check without the role answers 500

- **Reported**: yes, by the merge request of
  [entry 56](#deleting-an-external-status-check-without-the-role-answers-204-and-deletes-nothing).
- **In review**: yes,
  [gitlab-org/gitlab!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486),
  open, the one entry 56 follows; its Fix in review paragraph describes both
  halves.
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
  retires it is the route refusing the role with 403 before the service runs,
  as the merge request does, rather than the service giving its refusal a
  status; the handler then hints the Maintainer role on that 403, where it
  hints the license on every permission refusal today, and keeps
  `createRefusedForRole` for the releases before it.

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

### The admin token route takes no granular scopes, and no client-go create option carries them

- **Reported**: the GitLab half yes, by GitLab itself,
  [gitlab-org/gitlab#630541](https://gitlab.com/gitlab-org/gitlab/-/issues/630541)
  ("Support granular scopes when admins create a personal access token for a
  user via REST"), opened on 2026-09-23 by @alexbuijs of
  `group::authorization`. Read on 2026-09-30 it is unassigned, has no
  milestone, carries `automation:quick-win-judged`, and has no comment, and
  read again on 2026-10-05 it is still unassigned, with no milestone and no
  comment, as it is on 2026-10-06 and on 2026-10-08, when its only new entry
  is a "mentioned in" note of 10:30 UTC on 2026-10-07 from our merge request
  for the impersonation token routes,
  [gitlab-org/gitlab!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277)
  (row 86). The client-go half no.
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
the merge request below; GitLab's fix for it,
[gitlab-org/gitlab!257280](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/257280)
("Skip fine-grained token enforcement for bot users", milestone 19.5), was
open when read on 2026-10-05. So was
[gitlab-org/gitlab!259286](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259286)
(milestone 19.5), which moves the builder behind `build_granular_scopes` into
a shared `Authz::GranularScopes::Builder` and changes one hunk of
`lib/api/helpers/personal_access_tokens_helpers.rb`; the line numbers this
section cites are those of the 19.4.1 pin, and the merge request below is to
build on whichever shape `master` has by then. Read on 2026-10-06:
`gitlab-org/gitlab!257280` merged at 08:33 UTC, in milestone 19.5 and in no
tag yet, and reached GitLab.com's canary stage at 17:14 UTC
(`workflow::canary`), then the production stage at 00:17 UTC on 2026-10-07
(`workflow::production`). It does not give the service account routes a granular
parameter: it exempts a token whose owner is not a human user from
fine-grained enforcement, in `AuthorizeGranularScopesService` and in the
personal access token create and rotate services, until those tokens can be
fine-grained, which is why
[gitlab-org/gitlab#616442](https://gitlab.com/gitlab-org/gitlab/-/issues/616442),
which the merge request lists as a reference and does not close, is still
open. `gitlab-org/gitlab!259286`, by @eugielimpin, is still open: its backend
reviewer and @alexbuijs, as maintainer, approved it, and a minute after
setting it to join the merge train @alexbuijs cancelled that, writing that
it still needs an `authentication` review; the one approval rule not yet met
is the code-owner rule for `lib/api/helpers/personal_access_tokens_helpers.rb`.
Read on 2026-10-08: that review was not given. @eugielimpin resolved the
threads and set it to join the merge train at 00:59 UTC, removed the review
request for @atevans at 01:04 UTC, after the triage bot's reminder of 08:29
UTC on 2026-10-07, and started the merge train at 01:44 UTC; it merged at
01:47 UTC (merge commit `e4e3e8e3`, squash commit `c32b7f3e`), in milestone
19.5, with @jayswain's and @alexbuijs's approvals on every rule, the
helper's code-owner rule included. So `Authz::GranularScopes::Builder` is on
`master`, and is the shape the GitLab half of this entry builds on.

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
Nobody had answered on 2026-09-30, nor on 2026-10-05.

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

### A token's own description omits its granular scopes

- **Reported**: yes, by GitLab itself, before us:
  [gitlab-org/gitlab#629849](https://gitlab.com/gitlab-org/gitlab/-/issues/629849)
  ("Improve fine-grained PAT self-introspection"), opened on 2026-09-18 in
  `group::authorization`, says that the self-inform response "does not appear
  to provide the same information as `GET /personal_access_tokens/:id`,
  including `granular_scopes`", and the third of its open questions asks
  whether `granular_scopes` is missing from it. Read on 2026-10-05 it is open,
  unassigned and has no milestone, and read again on 2026-10-06 it is
  unchanged. This entry used to read no: the issue was cited under row 83 and
  never here. Ours is the merge request below, which answers that third
  question and nothing else, so it is related to the issue rather than closing
  it: whether self-introspection should need the permission at all, and the
  behaviour matrix the issue asks for, are for the group to decide.
- **In review**: yes,
  [gitlab-org/gitlab!259764](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259764),
  opened 2026-10-05 from the community fork (Fix merged, below). It was
  readied naming @eduardosanz, who merged the change that gave the other three
  routes the field, and @idurham for the page; the bot removed the first
  request and kept @idurham, whom it asked for the documentation review, so no
  backend reviewer is requested. Read on 2026-10-06 it needs three approvals
  and has none, has no milestone, a green fork pipeline and no thread open but
  the ready note's, and waits on a backend reviewer. Its merge would retire
  nothing here on its own: the read by id stays for every release before the
  one that carries it.

  Later on 2026-10-06 it had its review. @eduardosanz (11:07 UTC) left two
  suggestions, that the history line under "Self-inform" name 19.5 rather
  than 19.6, since `master` is building 19.5, and a shared helper instead of
  the repeated preload, approved, and asked @alexbuijs for the maintainer
  review; he also filed two follow-up issues,
  [gitlab-org/gitlab#632314](https://gitlab.com/gitlab-org/gitlab/-/issues/632314)
  (`granular_scopes` in the token create and rotate responses) and
  [gitlab-org/gitlab#632315](https://gitlab.com/gitlab-org/gitlab/-/issues/632315)
  (the impersonation token responses), both assigned to @jmrp, this
  project's maintainer. @alexbuijs (11:25 UTC) asked for the open threads to
  be addressed before the merge, and @idurham approved the page at 13:14
  UTC. At 17:49 UTC two commits answered both suggestions: `31e2643f` moves
  the history line to 19.5, made by hand with @eduardosanz as co-author
  since the suggestion's fence did not render as one, and `5b98f89d` moves the
  preload into `granular_scopes_options_for` in
  `lib/api/helpers/personal_access_tokens_helpers.rb`, which the self route
  and the list, get by id and rotate routes now all call, and which the two
  follow-ups can call as it is. Each thread was answered and all were
  resolved; the reply under the follow-ups says one merge request each will
  be opened once this one merges, and both are prepared. The push reset both
  approvals, and @idurham approved again at 17:58 UTC. Read that evening, on
  head `5b98f89d` with a green fork pipeline and no thread open, it carries
  that one approval and none of the five code-owner approvals its code and
  specs need. Those belong to two sections: the `Authentication` code owners
  of `lib/api/personal_access_tokens.rb`, `lib/api/personal_access_tokens/`
  and the helpers file, among them @eduardosanz, whose approval the push
  reset, and the backend maintainers of `/lib/` and `/spec/`, among them
  @alexbuijs. Neither of the two can approve the other's section (seven of
  the nine `Authentication` code owners are maintainers too and could approve
  both), so it waits on an approval from each and then on the merge.
- **Merged**: yes, unreleased, in milestone 19.5. @eduardosanz merged it at
  05:39 UTC on 2026-10-07 (squash commit `f11064df`, merge commit
  `7982d44f`), with no milestone set, and the release tooling labelled it
  `workflow::canary` at 11:03 UTC, `workflow::staging` at 11:41 and
  `workflow::production` at 12:15 the same day, so GitLab.com serves it.
  @gitlab-bot set milestone 19.5 at 01:14 UTC on 2026-10-08, so the "no
  milestone" this field and the summary row gave until that day is no
  longer true. No 19.5 tag or stable branch exists yet to carry it. The two
  follow-ups were opened that morning from the
  community fork and readied at 10:31 UTC, each building on the helper this
  merge request added:
  [gitlab-org/gitlab!260276](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260276)
  (`POST /user/personal_access_tokens`, `POST /personal_access_tokens/self/rotate`
  and the GitLab.com route that rotates an enterprise user's or service
  account's token, closing
  [gitlab-org/gitlab#632314](https://gitlab.com/gitlab-org/gitlab/-/issues/632314))
  and
  [gitlab-org/gitlab!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277)
  (the list, get and create routes of the impersonation tokens, closing
  [gitlab-org/gitlab#632315](https://gitlab.com/gitlab-org/gitlab/-/issues/632315)).
  The documentation automation requested @idurham on both, whose
  suggestions, two on the first and three on the second, were applied as
  commits at 13:24 UTC; @idurham approved both at 18:33, noting that each still
  needs a backend approval. Read at 19:56 UTC, and again at 21:40, neither
  had a backend reviewer or a milestone, and both fork pipelines were green.
  Both were asked for one that evening: a
  `@gitlab-bot ready @eduardosanz @alexbuijs` with the `backend` label at
  22:10 UTC on the first and 22:16 UTC on the second, after which the bot
  requested @alexbuijs and @eduardosanz on each. Read on 2026-10-08 neither
  has a backend review yet; each carries @idurham's approval alone, no
  milestone, and a green fork pipeline. GitLab's own issue is still open,
  unassigned and with no milestone. Until 2026-10-07 this finding was also
  carried by
  [issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055),
  as a side finding of
  [row 87](#graphql-types-and-mutations-this-server-reaches-declare-no-fine-grained-permission).
- **Blocking**: no. It costs every fine-grained session one request more
  each time its grant is read: once when the session starts and on every
  revalidation.
- **Workaround**: yes. `gitlab.DetectToken` (`internal/gitlab/scopes.go`)
  reads the token's id from `GET /personal_access_tokens/self`, and
  `gitlab.ReadGrant` (`internal/gitlab/grant.go`) then asks
  `GET /personal_access_tokens/:id` for the grant, through the captured
  response, since client-go does not model the field either
  ([row 32](#no-token-struct-carries-the-granular-fields-and-the-impersonation-and-resource-ones-carry-less-still)).
  Both routes need the same permission of a fine-grained token, Personal
  Access Token: Read, so the second request is the only cost. What retires it
  is the self route presenting the field. The request side of the same field,
  creating a token with a grant, is
  [row 82](#the-admin-token-route-takes-no-granular-scopes-and-no-client-go-create-option-carries-them).

**Where**: `lib/api/personal_access_tokens/self_information.rb` at
`v19.4.1-ee`, `get 'self'`, presents the token with
`Entities::PersonalAccessToken` and no options, while
`lib/api/entities/personal_access_token.rb` exposes `granular_scopes` only
`if: ->(token, options) { token.granular? && options[:with_granular_scopes] }`.
The list, the get by id and the rotate routes in
`lib/api/personal_access_tokens.rb` pass `with_granular_scopes: true`; the
self route does not.

**What**: a fine-grained token asking about itself is told it is granular
(`granular: true`, `scopes: ["granular"]`) and not what it was granted, which
is the one question a client holding such a token needs answered. It has to
ask again by its own id, through a route meant for listing a user's tokens.

**How we found it**: issue 952's research into how this server could read a
grant, reading the two routes side by side, and confirmed against client-go,
whose `GetSinglePersonalAccessToken` decodes the self answer.

**Fix merged**: for a granular token, the self route preloads the token's
granular scopes with their namespaces and presents it with
`with_granular_scopes: true` and the `project_ids_by_namespace_id` the entity
reads, as the get by id does; a legacy token is presented as before and runs
no query for scopes it does not have. Since the review of 2026-10-06 the
preload and those options come from one helper,
`granular_scopes_options_for`, which the list, get by id and rotate routes
call as well, and five examples in
`spec/lib/api/helpers/personal_access_tokens_helpers_spec.rb` pin the helper
itself. Five request spec examples pin the route: two
for a legacy token (no `granular_scopes` key, and no query that touches
them), a fine-grained token holding only Personal Access Token: Read, the same
token with a project scope as well, and an N+1 guard. A history line under
"Self-inform" in `doc/api/personal_access_tokens.md` records the release that
starts returning the field, since the page already promised it. The OpenAPI
documents do not change: the route's success model is the same entity.

### The fine-grained refusal names the missing permissions only as display labels in prose

- **Reported**: no. The issue is drafted below and goes before row 84's, which
  cites it. It needs nothing from client-go, and waits only on the
  maintainer's approval and an independent review of its text. Re-read on
  `master` on 2026-10-05, the refusal carries the same sentence and records
  the same denied permissions, while the code around it changed: the helper
  no longer iterates over `granular_token_requirements`, and the service picks
  the refused boundary in a `denied_boundary` method. The lines the draft
  cites from the 19.4.1 pin have moved with it,
  `authorize_granular_token_scopes!` to `lib/api/helpers.rb:1343-1356` and
  `access_denied_error` to
  `app/services/authz/tokens/authorize_granular_scopes_service.rb:141-152`,
  so the draft is to say which of the two it cites when it is posted.
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
is open and labelled as missed in every milestone from 19.0 to 19.4; read on
2026-10-05 it carries milestone 19.5, moved there on 2026-09-12. The error
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
  showing it for a service account's legacy token. GitLab's fix for that
  issue,
  [gitlab-org/gitlab!257280](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/257280),
  merged on 2026-10-06 for 19.5, reached GitLab.com's canary stage that
  evening and its production stage at 00:17 UTC on 2026-10-07. It exempts
  every token whose owner is not a human user, so from then on the classic
  token that meets the code is a human user's personal access token. On a
  self-managed instance enforcement refuses no existing
  legacy token (`doc/auth/tokens/fine_grained_access_tokens.md`, lines 134 to
  153).
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
`for_permission`. Read again on 2026-10-05 (`master` at `f5d0cd38`), it still
does, at line 144: the last commit to touch the service, `f2904340` of
2026-09-21, changed only its boundary check.

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

### `available_for_permission` ignores `available_for`

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no, and nothing at 19.4.1 reaches it.
- **Workaround**: not needed. `cmd/gen_api_live` records, for each raw
  permission, the first assignable definition a fine-grained token can be
  granted (not deprecated and `available_for?(:granular_access_token)`), and
  that is the name this server prints.

**Where**: `lib/authz/permission_groups/assignable.rb` at `v19.4.1-ee`:
`available_for_permission(permission)` filters `available_definitions`,
which rejects deprecated definitions only, while `available_for?(consumer)`
beside it is what tells a definition a token may hold from one only a role
may.

**What**: the method meant to name a permission a user can grant can name
one no token can be granted. At 19.4.1 one assignable is role-only,
`system_access/enterprise_user/read_email.yml` (`available_for: [role]`),
and no route or directive declares its permission, so nothing prints it
today. It matters because the fix
[row 85](#the-fine-grained-refusal-can-name-a-deprecated-permissions-label)
proposes for the refusal's label switches to this method, and so would carry
the gap into every refusal the day a role-only definition shares a raw
permission with a route.

**How we found it**: defining the name the withheld answers and the
reference page print, when the live record gained its fine-grained half
(issue 952).

**Proposal**: filter on `available_for?(:granular_access_token)` as well
where the caller is about a token, or give the method the consumer as an
argument, and carry it into row 85's change.

### GraphQL types and mutations this server reaches declare no fine-grained permission

- **Reported**: GitLab tracks every one of them on its own pending list,
  `config/authz/graphql/authorization_todo.txt`, which at `v19.4.1-ee` names
  862 types and 61 mutations and forbids new entries; nothing has been raised
  by us. Contributing the declarations this server needs was
  [issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055)
  until 2026-10-07, and is this section's since (What this asks of GitLab,
  below). Read on `master` on 2026-10-05, the list holds 905 entries (863
  types and 42 mutations) against the 923 of `v19.4.1-ee`, and every type and
  mutation in the table below is still on it; read again on 2026-10-07
  (`631ac4bc`), the same. Read on 2026-10-08 (`df1d6875`) it holds 904
  entries (862 types and 42 mutations): the one removed is `DuoWorkflow`,
  declared by a merge request of GitLab's own that merged on 2026-10-07,
  which is none of this table's, and every line of the table is still
  listed. GitLab plans part of it itself:
  [gitlab-org/gitlab#631631](https://gitlab.com/gitlab-org/gitlab/-/issues/631631)
  ("W1.2: Add granular token directives to GraphQL types reached by MCP
  tools", `group::authorization`, opened 2026-10-01, milestone 19.6, open and
  unassigned on 2026-10-07 and 2026-10-08) names `Namespace`, `WorkItemType` and
  `WorkItemSavedViewType` for its first merge request and the nested
  `Vulnerability*` types and the mutation `SecurityScanProfileAttach`, but not
  `securityScanProfileDetach`, for its second. Two older GitLab issues frame
  the same work:
  [gitlab-org/gitlab#593878](https://gitlab.com/gitlab-org/gitlab/-/issues/593878)
  (supporting the remaining REST endpoints and GraphQL queries and mutations
  with fine-grained tokens, or deprecating them; open, milestone "Next 4-6
  releases") and
  [gitlab-org/gitlab#626742](https://gitlab.com/gitlab-org/gitlab/-/issues/626742)
  (four types answered `null` to a fine-grained token, `WorkItemType` among
  them; open).
- **In review**: no, by us. GitLab's own attempt to declare
  `PipelineSecurityReportFinding`,
  [gitlab-org/gitlab!243768](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/243768),
  was closed unmerged on 2026-09-25 by GitLab's triage bot for inactivity, not
  rejected: it had been approved on 2026-07-06, then hit an N+1 failure and a
  conflict. [gitlab-org/gitlab!251840](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/251840)
  (open, milestone 19.5) declares four types, two of which this server's
  generated table records, `WorkItemType` and `WorkItemStatus`, as
  [row 88](#a-declared-mutation-whose-payload-type-declares-nothing-commits-the-write-and-answers-null)
  describes, and
  [gitlab-org/gitlab!254455](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254455)
  (open, by another contributor, no milestone, `workflow::in dev`, resolving
  the second of those two issues) declares `WorkItemType` among four types
  too. Both were read on 2026-10-07, and are unchanged on 2026-10-08. One
  line of the table is in review since 04:09 UTC on 2026-10-08, by another
  contributor:
  [gitlab-org/gitlab!260586](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260586)
  ("Authorize reading CustomEmoji for granular tokens", by @splattael, a
  community contribution related to
  [gitlab-org/gitlab#593878](https://gitlab.com/gitlab-org/gitlab/-/issues/593878))
  adds a `read_custom_emoji` permission on the group boundary, beside the
  create and delete ones that already exist, declares it on `CustomEmoji` and
  takes `CustomEmoji` and `CustomEmojiPermissions` off the pending list,
  because a fine-grained token read a group's `customEmoji`
  connection with the right count and no nodes, and no error. That is this
  table's `CustomEmoji` line, which withholds `custom_emoji.list`. Read the
  same morning it is ready for review, with @alexbuijs for the backend,
  @idurham for the documentation and @deepika.guliani as coach requested, a
  green pipeline, and no milestone; GitLab Duo found nothing and approved it
  at 04:58 UTC, which leaves all four required approvals missing.
- **Merged**: no.
- **Blocking**: yes, for a fine-grained personal access token, whatever its
  grant: 37 actions cannot be served it and 8 are served with parts of their
  answer empty. A classic token is unaffected.
- **Workaround**: yes. The 37 are withheld from every fine-grained session
  with the reason, the GitLab release the verdict comes from and the way out,
  a classic token (register row `AUT-007`), so a model is never handed an
  empty answer that reads as "nothing there"; the 8 are served with a note
  naming each part GitLab leaves empty. `cmd/gen_action_grants` derives both
  sets from the handlers and the live record, and `make audit-1to1-grants-report`
  prints this list as the worklist of this entry's contributions, each element
  with the REST routes that already declare a permission for the same
  resource. Serving over REST what GraphQL cannot serve such a token is
  [issue 1054](https://github.com/jmrplens/gitlab-mcp-server/issues/1054).
  Each declaration GitLab adds retires its line, once the live record is taken
  from the release that carries it (Adopting a declaration, below).

**Where**: GitLab's GraphQL authorization of a fine-grained token
(`lib/gitlab/graphql/authz/granular_scope_authorization.rb` at `v19.4.1-ee`):
an enforced object type or a mutation whose class carries no
`authorize_granular_token` directive denies every fine-grained token
(`return error(default_error_message) if token.granular?`), and a denied
object is answered `null`, or left out of a connection, with no error
(`app/graphql/types/base_object.rb`; the schema keeps graphql-ruby's
`unauthorized_object`).

**What**: the undeclared types and mutations on a path this server's
documents reach, as `go run ./cmd/audit_1to1/ -scope=grants` reports them at
19.4.1:

| Type or mutation                                                                                           | What a fine-grained token gets                    | Actions                                                                                                                      |
| ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `Namespace`                                                                                                | `null` for every root `namespace(fullPath:)` read | 24: 14 epic actions, the work item reads and writes and the saved view reads, `achievement.list` and two more                |
| `UserAchievement`, `CustomEmoji`, `BranchRule`, `ProjectTargetBranchRule`, `PipelineSecurityReportFinding` | The items removed from the list                   | `achievement.user_list`, `custom_emoji.list`, `branch.rule_list`, `project.target_branch_rule_list`, `security_finding.list` |
| `CiCatalogResource`, `SecurityReportSummary`, `VulnerabilitySeveritiesCount`                               | `null`                                            | `ci_catalog.get`, `ci_catalog.list`, `vulnerability.pipeline_security_summary`, `vulnerability.severity_count`               |
| `ScanProfileProjectStatus`                                                                                 | A list of non-null items answered `null` whole    | `security_scan_profile.list_project_statuses`                                                                                |
| `bulkUpdateSecurityAttributes`, `securityScanProfileAttach`, `securityScanProfileDetach`                   | Refused, nothing runs                             | `security_attribute.bulk_update`, `security_scan_profile.attach`, `security_scan_profile.detach`                             |
| `VulnerabilityIssueLink`, `VulnerabilityFindingTokenStatus`, `VulnerableKubernetesResource`                | Those fields empty, the rest served               | `vulnerability.get`, `vulnerability.list` and the four state mutations                                                       |
| `QuickActionsStatus`                                                                                       | That payload field empty, the note served         | `group.epic_note_update`, `group.epic_discussion_update_note`                                                                |

The probes of `test/e2e/gitlab` measured the shapes on a 19.4.1 instance: a
`namespace(fullPath:)` read answers `null`, a project's `branchRules` loses
its items and `vulnerabilitySeveritiesCount` answers `null`, each with no
error, where a classic token reads them
(`TestFineGrainedProbes_UndeclaredGraphQLTypes_AreEmptyWithNoError`,
`TestFineGrainedProbes_SeverityCount_IsNullWithNoError`). For four of the
types a REST route of the same resource does declare a permission (the
protected branches routes for `BranchRule`, the namespace routes for
`Namespace`, the issue links routes for `VulnerabilityIssueLink`, the epic
routes for `WorkItem` in row 89), which is where the declaration GitLab would
add can start from.

**A related blind spot**, read on 2026-10-05:
[gitlab-org/gitlab#627165](https://gitlab.com/gitlab-org/gitlab/-/issues/627165)
(open, `group::authorization`, opened 2026-09-01) records that a mutation
which calls `Authz::Tokens::AuthorizeGranularScopesService` in its resolver,
because the boundary it needs is not a top-level argument, is invisible to
GitLab's documentation generator, to `gitlab:permissions:validate` and to its
spec scanner, all of which read only the directives. `cmd/gen_api_live` reads
the same directives, so a requirement checked that way is invisible to
R-GRANT as well, and the derivation would understate what such an action
needs. No action of this server reaches the issue's three examples
(`promoteToEpic`, `epicTreeReorder` and `projectSubscriptionCreate`) today.

**How we found it**: the fine-grained derivation of issue 952, which walks
every GraphQL document an action sends against the authorization a booted
19.4.1 records (`cmd/gen_api_live`), and the direct probes above.

**Proposal**: declare `authorize_granular_token` on each, starting with
`Namespace`, which alone withholds 24 actions, at the boundary and with the
permission the matching REST route already declares where one exists.

**What this asks of GitLab**, element by element, as
[issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055) set it
out before this entry replaced it on 2026-10-07. For each type or mutation of
the table: first establish what GitLab intends, whether an issue or epic of
its fine-grained token rollout already covers it (the W1.2 issue named under
Reported covers part), whether a merge request is in flight, or whether it is
left undeclared on purpose (a `skip_reason`, or a statement in the tracking
issue). Where it is pending and nobody has it, prepare a GitLab merge request
that declares it with the assignable permission and boundary GitLab already
uses for the equivalent REST route, removes its line from
`config/authz/graphql/authorization_todo.txt` (issue 1055 placed it under
`lib/tasks/gitlab/permissions/graphql/`, which is wrong), carries request
specs under `spec/requests/api/graphql` or
`ee/spec/requests/api/graphql` with GitLab's shared examples ("authorizing
granular token permissions for GraphQL") and an N+1 guard for a list
position, and passes `gitlab:permissions:graphql:compile_docs` and
`gitlab:permissions:validate`. Then record each one here with the release
that carries its declaration, so the withheld set shrinks by recorded release
and never by assumption. Nothing is posted to gitlab.com, merge request,
issue or comment, before the maintainer has reviewed the list of what is
proposed.

**One blocker for `Namespace`**: its assignable permission, `read_namespace`
(`config/authz/permission_groups/assignable_permissions/groups/namespace/read.yml`),
allows the `user` and `project` boundaries only, at `v19.4.1-ee` and on
`master` read on 2026-10-07, so a declaration at the group boundary, which
the W1.2 issue asks for beside the project one, needs that assignable amended
in the same change.

**Adopting a declaration**: once a release carries it, the live record is
taken from that release (`make gen-api-live`) and the grants table derived
again (`make gen-action-grants`), which drops the element from the withheld
set with no code of its own. Two end-to-end probes pin today's `null` and flip
for their elements,
`TestFineGrainedProbes_UndeclaredGraphQLTypes_AreEmptyWithNoError` and
`TestFineGrainedProbes_SeverityCount_IsNullWithNoError`, and the
`group-work-item` declarations of `cmd/gen_action_grants/grant_declarations.go`
go with row 89. The cost is ADR-0024's NEG-001: the withheld set is the newest
record's, so an instance older than the release that carries a declaration is
served an action it answers with a silent `null`. Re-pinning the record at
each minor is
[issue 1023](https://github.com/jmrplens/gitlab-mcp-server/issues/1023), and
how GitLab could fail visibly instead is
[issue 1103](https://github.com/jmrplens/gitlab-mcp-server/issues/1103).

**The two other findings issue 1055 carried.** The self route omitting a
token's granular scopes is
[row 86](#a-tokens-own-description-omits-its-granular-scopes), whose fix merged
on 2026-10-07. The other was that `validate_unique_namespace` in
`app/services/authz/granular_scope_service.rb` appeared to accept two scopes
with the same namespace in one creation request, recorded only from the `201`
of that creation and so to be measured again before it was reported. GitLab
is fixing it itself:
[gitlab-org/gitlab!259531](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259531)
("Reject duplicate granular scopes within one request", `group::authorization`,
milestone 19.5, opened 2026-10-05, open on 2026-10-07) describes the same gap,
incoming scopes never compared with each other, and calls the result
redundant rows rather than a security hole. Read on 2026-10-08 it is still
open, approved by @alexbuijs at 09:19 UTC on 2026-10-07 and waiting on its
database review. It costs this server nothing and
leaves nothing for us to do, so it has no row of its own.

### A declared mutation whose payload type declares nothing commits the write and answers null

- **Reported**: no.
- **In review**: no, by us. For one of the payload types below GitLab's own
  change is open:
  [gitlab-org/gitlab!251840](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/251840)
  (milestone 19.5, last updated 2026-09-30) adds
  `authorize_granular_token skip_reason: :parent_authorizes` to
  `WorkItemType`, `WorkItemStatus`, `WorkItemFeatures` and
  `NamespaceAvailableFeatures` and takes them off GitLab's pending list. It
  would retire the `WorkItemType` line below and the work item status this
  server's generated table records as served empty on four work item paths
  (create, update, a work item read and the listing), which a report by
  another user,
  [gitlab-org/gitlab#631882](https://gitlab.com/gitlab-org/gitlab/-/issues/631882)
  (opened 2026-10-02, `group::work items`), measured from the outside: a
  fine-grained token reads the status widget's `status` as `null`. Read on
  2026-10-07, GitLab's W1.2 issue
  ([gitlab-org/gitlab#631631](https://gitlab.com/gitlab-org/gitlab/-/issues/631631),
  in [row 87](#graphql-types-and-mutations-this-server-reaches-declare-no-fine-grained-permission))
  plans `WorkItemType` and `WorkItemSavedViewType` as well, and
  [gitlab-org/gitlab!254455](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254455),
  open, declares `WorkItemType` too; these payload types close through row
  87's declarations.
- **Merged**: no.
- **Blocking**: yes, for a fine-grained personal access token: 20 writes
  cannot be served it.
- **Workaround**: yes. The 20 are withheld from every fine-grained session
  with the reason (register row `AUT-007`), and the reason says that GitLab
  commits such a write and answers null. Serving them would be worse than
  withholding them: a client that reads a null payload as "not done" and
  retries repeats a write GitLab already made.

**Where**: `app/graphql/mutations/base_mutation.rb` and
`lib/gitlab/graphql/authz/granular_scope_authorization.rb` at `v19.4.1-ee`. A
mutation is authorized before it resolves, against its own directive, so a
grant that holds the mutation's permission passes and the write runs; the
objects of the payload are authorized after, each against its own type, and
an enforced type with no directive denies every fine-grained token, so the
payload field comes back `null` with no error.

**What**: the payload types that declare nothing and the writes that answer
with them: `Achievement` (`achievement.create`, `achievement.update`,
`achievement.delete`), `UserAchievement` (`achievement.award`,
`achievement.revoke` and the three `achievement.user_achievement_*` writes),
`CustomEmoji` (`custom_emoji.create`, `custom_emoji.delete`),
`WorkItemType` (`issue.work_item_create`, through `WorkItem.workItemType`, a
non-null field whose null takes the work item with it),
`WorkItemSavedViewType` (the four saved view writes),
`ProjectTargetBranchRule` (`project.target_branch_rule_create`),
`SecurityAttribute` (`security_attribute.create`, `security_attribute.update`)
and `SecurityCategory` (`security_category.create`,
`security_category.update`). The write is not refused, so a token that holds
the mutation's permission changes GitLab and is told nothing it can tell from
a write that did not happen.

Read from the source. The same answer, a committed write and a `null`
payload with no error, was measured on a 19.4.1 instance for the group work
item of row 89, which reaches it by another path.

**How we found it**: the fine-grained derivation of issue 952, which judges a
mutation's payload as part of its answer.

**Proposal**: the declarations of row 87 for the payload types close it for
these; the class closes when a payload object no fine-grained token may read
is refused with an `errors[]` entry rather than nulled, or when a mutation is
refused before it runs if its payload type declares nothing. Establishing
whether GitLab means a GraphQL read to fail silently for such a token, and
proposing a visible error upstream, is
[issue 1103](https://github.com/jmrplens/gitlab-mcp-server/issues/1103), open.

### WorkItem declares the project boundary only, so a group's work item is null to a fine-grained token

- **Reported**: yes, by another user, before us:
  [gitlab-org/gitlab#630483](https://gitlab.com/gitlab-org/gitlab/-/issues/630483),
  opened on 2026-09-23 in `group::work items` as a `type::bug`, reports a
  group's `workItems` filtered to epics answering `nodes: []` to a
  fine-grained token where a classic token reads the epics. Read on
  2026-10-06 it is open, unassigned and has no milestone. This entry read no
  until then: the issue was not cited here, and the merge request below is
  what joins the two, since it closes it.
- **In review**: yes,
  [gitlab-org/gitlab!259765](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259765),
  opened 2026-10-05 from the community fork (Fix in review, below). It was
  readied naming no reviewer; the bot requested @deepika.guliani and then
  replaced that request with @idurham, whom it asked for the documentation
  review, so no backend reviewer is requested. Read on 2026-10-06 it needs two
  approvals and has none, has no milestone, a green fork pipeline and no
  thread open but the ready note's, and waits on a backend reviewer; the
  Danger review suggests an `EE: true` trailer on its changelog commit, since
  its specs are under `ee/`. On its own it would not serve the 14 of the 15
  epic actions that `Namespace` (row 87) decides first, as Blocking says.
  Read again that evening: the commit, amended with the `EE: true` trailer,
  was pushed at 11:27 UTC as `8bd419c3`, whose fork pipeline passed, and
  @idurham approved the page at 13:20 UTC and resolved the ready note's
  thread. It still needs the two code-owner approvals of `/app/` and
  `/ee/spec/`, has no milestone and no backend reviewer requested, and waits
  on one. On 2026-10-07 at 19:33 UTC @idurham set it to join the merge train
  when its checks pass, and the canonical pipeline that started then was green
  at 20:45; read at 21:40 UTC it still lacks both code-owner approvals, and
  joins the merge train as soon as they land. A backend reviewer was asked
  for that evening: the backend request had been dropped when the
  documentation review was requested, so at 22:16 UTC a
  `@gitlab-bot ready @imand3r` asked for a reviewer of the authorization
  group, and the bot requested @imand3r. Read on 2026-10-08 the merge train
  setting stands, both code-owner approvals are still missing, there is no
  milestone, and the pipeline of 20:45 UTC is still the latest, green. The
  issue it closes is still open, unassigned and with no milestone.
- **Merged**: no.
- **Blocking**: yes, for a fine-grained personal access token: the 15 epic
  actions it withholds, and the GraphQL way of `group.epic_list`, whose REST
  way is served. `Namespace` (row 87) decides 14 of the 15 and that way
  first, so declaring `Namespace` alone would not serve them.
- **Workaround**: yes. Every epic position the epic actions reach is declared
  in `cmd/gen_action_grants/grant_declarations.go` (category
  `group-work-item`) as one no fine-grained token passes, and the actions are
  withheld with the reason (register row `AUT-007`).

**Where**: `app/graphql/types/work_item_type.rb` at `v19.4.1-ee` declares
`authorize_granular_token permissions: :read_work_item, boundary: :project,
boundary_type: :project`; re-read on `master` on 2026-10-05 (`f5d0cd38`), line
13 still declares only the project boundary. An epic is a group's work item,
whose boundary `lib/gitlab/graphql/authz/boundary_extractor.rb` cannot resolve
to a project, so the extractor returns no boundary and the service refuses
(`app/services/authz/tokens/authorize_granular_scopes_service.rb`). The
mutation `workItemUpdate` declares both the project and the group boundary
(`app/graphql/mutations/work_items/update.rb`), so the write itself passes.

**What**: measured on a 19.4.1 instance with a token granted Work Item: Read
and Work Item: Update on the epic's group: `workItem(id:)` of the epic
answers `null` with no error where a classic token reads it, and
`workItemUpdate` on it renames the epic and answers `workItem: null` with no
error (`TestFineGrainedProbes_GroupWorkItem_ResolvesNoBoundary` in
`test/e2e/gitlab/ee`). A grant on the group can therefore change an epic and
never read it.

**How we found it**: issue 952's derivation, read from the source, then the
probe above, which also showed the write committing where the source reading
had expected a refusal.

**Fix in review**: `WorkItem` declares `read_work_item` at the group boundary
beside the project one, as `workItemUpdate` already does, naming the
`namespace` association for the group and the `project` association for the
project rather than `resource_parent`, so that the boundary preloader keeps a
group's list of work items at the query count of `master` for either kind of
token. EE request specs pin the single work item of a group read with a token
granted on the group, the issue's query, a group's list with its projects'
work items, a token without Work Item: Read, and N+1 guards for a
fine-grained and a classic token; the fine-grained GraphQL documentation page
is regenerated with the new row. A work item in a personal namespace still
resolves no boundary, which the merge request leaves as a separate question.

### The pending-permission check exempts every type named `*Edge` or `*Payload`

- **Reported**: no.
- **In review**: no.
- **Merged**: no.
- **Blocking**: no. No action of this server reaches any of the three types
  below.
- **Workaround**: not needed. `cmd/gen_api_live` records which types GitLab
  enforces and which declare a fine-grained permission, and the derivation
  reads that rather than GitLab's list, so these three would be judged like
  any other undeclared type.

**Where**: `graphql_object_type?` in
`lib/tasks/gitlab/permissions/graphql/schema_directives.rb` at `v19.4.1-ee`,
which the validation task (`validate_task.rb`, `current_entry_sources`) uses
to decide which types must either declare a permission or sit on
`authorization_todo.txt`, returns false for every name ending in `Payload`,
`Connection` or `Edge`.

**What**: the suffixes are meant to pass over the framing graphql-ruby
generates, but a hand-written type can carry one of those names too. Three
enforced object types that declare no permission are exempted by name and so
are on neither side of the check: `DependencyPathEdge`
(`ee/app/graphql/types/sbom/dependency_path_edge.rb`, a `BaseObject` with
the comment that authorization is on the parent),
`NamespaceWorkItemChangesPayload` and `PushEventPayload`
(`app/graphql/types/users/push_event_payload_type.rb`). A fine-grained token
reaching any of them is denied as any undeclared type is, and nothing in
GitLab's tooling lists it as pending.

**How we found it**: when the live record gained its fine-grained half,
comparing the enforced types that declare nothing with the set GitLab's own
rule computes, which the record holds beside `authorization_todo.txt`.

**Proposal**: exempt the generated connection and edge types by their class
(the types graphql-ruby builds for a connection) rather than by their name,
so a hand-written type ending in `Edge` or `Payload` is checked like any
other.

### A board name GitLab cannot save is answered as a success

- **Reported**: yes, by the merge request below.
- **In review**: yes,
  [gitlab-org/gitlab!260458](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260458),
  "Return a 400 when a board name fails validation in the boards API", opened
  from the community fork at 18:28 UTC on 2026-10-07 and readied at 18:33 (Fix
  in review, below). No reviewer was assigned, so at 19:11 the triage bot moved
  it back to `workflow::in dev`, where it waited for a reviewer to be asked.
  One was at 22:09 UTC: a `@gitlab-bot ready @brytannia` with the `backend`,
  `type::bug` and `group::planning views` labels, since the boards moved to
  that group, set `workflow::ready for review`, and the bot requested
  @brytannia. Read on 2026-10-08 its fork pipeline is green, and it has no
  review yet, no approval and no milestone.
- **Merged**: no.
- **Blocking**: no, but the answer is false: a caller told the board was
  created or renamed goes on as if it were.
- **Workaround**: none taken. Neither answer says the write was dropped, and
  a length check of this server's own would copy a model validation the API
  pages do not state. The handlers report what GitLab answers: `UpdateBoard`
  in `internal/tools/boards/boards.go` and `UpdateGroupBoard` in
  `internal/tools/groupboards/group_boards.go` return the board as GitLab
  kept it, and `CreateBoard` and `CreateGroupBoard` beside them return a
  board whose ID is 0.
  `TestUpdateGroupBoard_NameGitLabCannotSave_ReturnsTheBoardGitLabKept` holds
  the update half. What retires it is GitLab sending the 400 `update_board`
  already has for a board that does not validate, and `create_board`
  rendering the service's error.

**Where**: `lib/api/boards_responses.rb` at `v19.4.1-ee` (the `board`,
`create_board` and `update_board` helpers), which `lib/api/boards.rb` and
`lib/api/group_boards.rb` both include; `app/models/board.rb:14`;
`app/services/boards/create_service.rb` and
`app/services/boards/update_service.rb`.

**What**: `Board` validates a changed name at 255 characters at most
(`validates :name, presence: true, length: { maximum: 255, if:
:name_changed? }`), and nothing else checks it: the routes declare `name` a
plain `String`, the column is an unbounded `character varying`, and neither
API page states a limit. A longer name therefore fails only in the model, and
every route that sets it answers as if it had not:

- `PUT /projects/:id/boards/:board_id` and `PUT /groups/:id/boards/:board_id`
  run `update_board`, which hands `board` to `Boards::UpdateService#execute`
  (an `update` of the record, which returns false and saves nothing), then
  asks `board.valid?` and presents `board`. But `board` is a helper,
  `board_parent.boards.find(params[:board_id])`, that is not memoized, and
  `has_many :boards` (`app/models/project.rb:233`, `app/models/group.rb:111`)
  loads a new record on each `find`, so the check and the answer each read
  the stored board, which is valid because its name did not change. GitLab
  answers 200 with the board as it was, every other field of the request
  dropped with the name since the update is one save, and the
  `bad_request!("Failed to save board ...")` branch is reached by nothing the
  route accepts.
- `POST /projects/:id/boards` and `POST /groups/:id/boards` run
  `create_board`, which presents `response.payload[:board]` whatever the
  response says. `Boards::CreateService#create_board!` builds the board with
  a `create` on the parent's board collection and, when it was not persisted,
  returns `ServiceResponse.error` with the unsaved board in its payload, so
  GitLab answers 201 with a board whose `id` is `null`, and nothing is
  created.

No spec of GitLab's covers either. `spec/requests/api/boards_spec.rb` drives a
create 400 only for a missing name, which Grape answers before the helper
runs, and `spec/requests/api/group_boards_spec.rb` and the shared board
examples (`spec/support/shared_examples/requests/api/boards_shared_examples.rb`)
only a list create 400. The GraphQL board mutations report both failures in
their `errors` field (`app/graphql/mutations/boards/update.rb` and
`create.rb`), since the update reads the errors of the record it updated and
the create checks the service's response.

**How we found it**: reviewing the fix for
[issue 1213](https://github.com/jmrplens/gitlab-mcp-server/issues/1213), which
had given the group board update a hint for a 400 naming the name length.
Following that 400 back through `update_board` showed GitLab never sends it,
and `create_board` beside it presents the unsaved record the same way.

**Fix in review**: all in `lib/api/boards_responses.rb`. The `board` helper is
memoized, so `update_board` presents and reports on the record the service
updated, and branches on the result of the update rather than validating the
record a second time; both helpers answer a failed write with
`render_validation_error!`, so the four routes answer
`400 {"message":{"name":["is too long (maximum is 255 characters)"]}}`, the
shape other routes use for a model that does not validate, and still save
nothing. The merge request treats the changed status as a bug fix, since only
a request GitLab did not save gets the new answer, and leaves the 255-character
limit off both API pages, offering to add it there or in a follow-up. When a
release carries it, the four board handlers here report GitLab's 400 as it
arrives, and
`TestUpdateGroupBoard_NameGitLabCannotSave_ReturnsTheBoardGitLabKept` is
rewritten for that release.

### The roles and permissions page does not say when a Guest or a Planner can view pipelines and merge requests

- **Reported**: yes, by the merge request below; a documentation-only change
  needs no issue first.
- **In review**: yes,
  [gitlab-org/gitlab!260353](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260353),
  "Say when Guests and Planners can view pipelines and merge requests", opened
  from the community fork at 14:17 UTC on 2026-10-07 and readied at 14:22
  with one commit; three more were added at 19:14 UTC, one commit per
  correction below. The technical writer the
  documentation automation requested, @idurham, approved it at 19:32 UTC and
  set milestone 19.5. Its pipeline stopped at `danger-review`, which refused
  the short merge request references in the message of the fourth commit,
  `e78ae440`, since GitLab's commit message rules ask for full URLs there.
  That commit was reworded at 22:21 UTC as `945301ce`, with the same diff, and
  a note at 22:32 UTC asked @idurham to approve again and to run the merged
  results pipeline, which passed at 23:29 UTC. At 23:23 UTC @idurham set it to
  join the merge train when its checks pass; at 23:27 UTC he wrote that it
  conflicts and asked for the conflict to be resolved so that he can merge
  it, and cancelled the merge train. The conflict is the one row 100 foresaw:
  row 100's merge request merged at 23:26 UTC, changing the lines of
  `doc/user/permissions.md` next to these. Read at 05:30 UTC on 2026-10-08,
  and again at 05:52, the head was still `945301ce` and GitLab still listed
  @idurham's approval, although the note of 22:32 UTC had said the reword
  reset it: GitLab keeps an approval across a push that leaves the patch id
  unchanged, and the reword changed only a message. At 08:03 UTC the four
  commits were pushed rebased onto `master` with row 100's merge in it, as
  `376357723`: the only conflict was the View vulnerabilities in a pipeline
  row and the footnote list around it, resolved by taking that row as row
  100 merged it and keeping this merge request's footnotes, with the added
  and removed lines of the diff unchanged. That push did reset his approval,
  the description was updated for the rebase, and a reply in his thread asks
  him to approve again and merge it once the pipeline passes. The thread is
  left for him to resolve and is the one open thread; the milestone is 19.5
  and the pipeline of `376357723` was running at 08:05 UTC.
- **Merged**: no.
- **Blocking**: no.
- **Workaround**: not needed. The hint this server gives on a refused
  pipeline read follows the policy rather than the page: `hintReadPipelines`
  in `internal/tools/pipelines/latest.go` names Reporter as the lowest role
  that reads pipelines unconditionally and says when a Guest or a Planner
  reads them, and its comment records that the page says otherwise.

**Where**: `doc/user/permissions.md`, the Project CI/CD and Project merge
requests tables, and `doc/ci/pipelines/settings.md`, the description of
Project-based pipeline visibility.

**What**: the pages say what the policy does not, in four places.

- The Project CI/CD table gives the Planner role a check on seven view rows
  (the list of jobs, job logs, pipelines, viewing and downloading artifacts,
  environments, and the pipelines tab of a merge request), and no footnote
  names Planner. `config/authz/roles/planner.yml` inherits from Guest and adds
  no CI/CD read permission, so a Planner reads jobs, pipelines and artifacts
  only through `_read_public_build` and `_read_public_pipeline`, which
  `app/policies/project_policy.rb` prevents while Project-based pipeline
  visibility (`public_builds`) is off, and environments only on a public
  project.
- The View existing artifacts row gives Planner a check with no condition,
  and its footnote says a Guest sees existing artifacts only in a public
  project, while on a private one a Guest or a Planner learns of them from
  the pages the setting opens.
- The settings page said that with the setting cleared, the pipelines of an
  internal project stay visible to every authenticated user except external
  users. The pipeline list and the pipeline pages need the permission the
  setting withholds, so for those users an internal project behaves like a
  public one.
- The Project merge requests table gives Planner a check on viewing,
  searching and approving merge requests, adding internal notes and
  commenting, and nothing says a Planner loses all five when a project sets
  merge requests to Only Project Members, for which
  `ProjectFeature::PRIVATE_FEATURES_MIN_ACCESS_LEVEL` makes Reporter the
  minimum role, as the change that added the Planner role intended. The
  footnote on viewing and searching also said that an external user needs at
  least the Reporter role on an internal project, where `planner.yml` grants
  `read_merge_request` and the Planner role is enough.

The setting is on by default, so with it on a Planner does see the jobs,
pipelines and artifacts the table promises and loses only the environments of
a private project; the page was wrong for every project that turns the
setting off.

**Fix in review**: the merge request adds Planner to the footnotes of the
seven CI/CD view rows with the conditions the policy applies, gives the
environments, merge request pipelines tab and existing artifacts rows
footnotes of their own, corrects the internal-project bullet of the settings
page, adds the Only Project Members condition for Planner to five merge
request rows, and says that an external user on an internal project needs at
least the Planner role, not Reporter, to view and search merge requests. No
check mark changes.

**What it costs this server**: nothing at run time; a hint written from the
page would have told a Planner of a project with the setting off that it can
read the pipelines GitLab refuses it.

**How we found it**: writing the refusal hint of `pipeline.latest`'s fallback
for [issue 1234](https://github.com/jmrplens/gitlab-mcp-server/issues/1234):
the CI/CD table put the lowest role that reads pipelines at Planner, and the
policy puts it at Reporter. The other statements were found reading the same
tables and the setting's page while preparing the merge request.

### The roles and permissions page gives Guest, Planner and Reporter the pipeline security report

- **Reported**: yes, by the merge request below; a documentation-only change
  needs no issue first.
- **In review**: yes,
  [gitlab-org/gitlab!260471](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260471),
  "Correct who can view vulnerabilities in a pipeline", opened from the
  community fork at 19:10 UTC on 2026-10-07 and readied at 19:16. The
  technical writer the documentation automation requested, @idurham, applied
  one suggestion of their own as a commit (`c59f03bc`), approved it at 19:30
  UTC, set milestone 19.5, and set the merge request to be added to the merge
  train when its checks pass; applying the suggestion at 19:34 updated the
  branch, and GitLab aborted that. Read at 20:04 UTC, and again at 21:40, it
  was approved with a green pipeline and nothing had set it to merge again.
  It edits the lines next to
  [row 99](#the-roles-and-permissions-page-does-not-say-when-a-guest-or-a-planner-can-view-pipelines-and-merge-requests)'s
  in `doc/user/permissions.md`, so whichever of the two merges second needs a
  rebase that keeps both changes. This one merged first, so row 99's was the
  one rebased, at 08:03 UTC on 2026-10-08, keeping this row as merged.
- **Merged**: yes, unreleased, in milestone 19.5. @idurham started a merge
  train at 23:24 UTC on 2026-10-07, and it merged at 23:26 UTC (merge commit
  `977b8a30`, squash commit `ec0a9371`). The release tooling labelled it
  `workflow::staging-canary` at 03:45 UTC on 2026-10-08. No 19.5 tag or
  stable branch exists yet to carry it.
- **Blocking**: no.
- **Workaround**: not needed. The one hint this server gives on a refused
  pipeline findings read, in `securityfindings.List`, names the Ultimate
  license and no role.

**Where**: `doc/user/permissions.md`, the View vulnerabilities in a pipeline
row of the Project CI/CD table and its footnote, and
`doc/ci/pipelines/settings.md`, the list of what Project-based pipeline
visibility changes.

**What**: the row gives Guest (through a footnote that makes it depend on
Project-based pipeline visibility), Planner and Reporter a check, while
everything that shows a pipeline's findings checks `read_security_resource`:
the pipeline Security tab (`expose_security_dashboard?` in
`ee/app/presenters/ee/ci/pipeline_presenter.rb`), the
`Pipeline.securityReportSummary` and `Pipeline.securityReportFindings` GraphQL
fields, and `GET /projects/:id/vulnerability_findings`. Among the default
roles only Security Manager and Developer grant that ability, Maintainer and
Owner through Developer, besides auditors; the setting gates the
`_read_public_*` abilities and never this one; and the page the row links
already names the right roles. The row came in with GitLab 14.8, when the
policy already granted the ability only from Developer up, so the Guest and
Reporter checks were never right, and the Planner check followed with the
Planner column in 17.7. The settings page made the same claim from the
setting's side.

**Fix merged**: the merge request removes the three checks and the
footnote only this row used, takes pipeline security results out of the
setting's list and out of its sentence about public projects on the settings
page, and adds a sentence there saying the setting does not change who can
view the pipeline security report, with a link to the section that lists the
roles.

**What it costs this server**: nothing.

**How we found it**: it is the row the merge request of
[row 99](#the-roles-and-permissions-page-does-not-say-when-a-guest-or-a-planner-can-view-pipelines-and-merge-requests)
left out of its Planner changes to the same table, read in the same pass,
since its Guest, Planner and Reporter cells are wrong for a reason that has
nothing to do with the Planner role.

## GitLab Orbit (`gitlab-org/orbit/knowledge-graph`)

Both entries here were filed as an issue first and then a merge request that
closes it, because the project's `CONTRIBUTING.md` asks for an issue before any
non-trivial merge request. Each merge request bumps the Orbit skill to
`0.32.3`, as every change under `skills/orbit/` must, so whichever lands second
needs `0.32.4`. Nothing in the project's pipeline will flag it: the two
branches merge cleanly, since both make the same change to the version, and
`skill-version-bump-check` compares a branch with its merge request's diff
base, where the skill stays at `0.32.2` until the branch is rebased. The
commit that updated each branch's tests (below) says so in its message. The
numbers have moved since: on 2026-10-01 both branches were rebased onto a
`main` where
[gitlab-org/orbit/knowledge-graph!2672](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2672),
merged that day, had taken the skill to `0.33.0` and added a GQL copy,
`SKILL.gql.md`, versioned as the JSON skill's version plus
`+gql`, and they took `0.33.1` and `0.33.2`, each with the four tests that pin
the two versions. Row 75's merged at `0.33.1`, and row 76's was rebased onto
that on 2026-10-02 and keeps `0.33.2`, one above `main`. On 2026-10-02
[gitlab-org/orbit/knowledge-graph!2692](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2692)
stopped those tests hardcoding the version, so a bump no longer edits them.

### The DSL schema says a path query may omit `rel_types`

- **Reported**: yes,
  [gitlab-org/orbit/knowledge-graph#1329](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1329),
  opened 2026-09-28, mentioning the author of the rule and of the current text,
  and closed by the merge on 2026-10-02.
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

  On 2026-09-30 @kerrizor passed the review to @dgruzd and had GitLab Duo
  review it, which found nothing to comment on. @dgruzd approved it at 13:05
  UTC on 2026-10-01, asked @michaelusa for the maintainer review, and
  suggested in a non-blocking note that the schema itself mark `rel_types`
  required, in `PathConfig.required` and with `"minItems": 1`, so that
  schema-driven clients and `get_query_dsl` stop getting a mixed signal. The
  reply (20:36 UTC) agreed and offered it as a follow-up once this merged,
  noting that a JSON path query without `rel_types` would then get the
  schema's required-property error instead of `check_path`'s fan-out message.
  The rebase that evening (21:51 UTC, above) reset the approval. @dgruzd
  approved again at 10:04 UTC on 2026-10-02, set it to merge when its checks
  pass, leaving any feedback from @michaelusa to a follow-up, and started the
  pipeline in the canonical project, which passed; it merged at 10:15.

  The follow-up is
  [gitlab-org/orbit/knowledge-graph!2691](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2691),
  "fix(dsl): require rel_types in the path_finding schema", opened from the
  community fork at 17:13 UTC on 2026-10-02 and readied naming @dgruzd at
  17:43. It adds `rel_types` to `PathConfig.required` with `"minItems": 1`,
  bumps `query_dsl` from 12.1.9 to 12.1.10 and pins the new shape in a test.
  Its description discloses one narrowing beyond what was agreed: the
  top-level `path` references `PathConfig` for every query type, so a
  traversal or neighbors query carrying an unused `path` without `rel_types`,
  which compiled before, is now refused by the schema; it keeps the patch bump
  because that `path` has no effect on those queries, and offers another bump
  if the maintainers read the narrowing as a change of contract. Three
  integration tests whose JSON `path` had no `rel_types` gain `["*"]`, which
  is what the untyped edge of their GQL counterparts lowers to. Like this
  merge request it needs a pipeline in the canonical project, which only a
  project member can start.

  Read again on 2026-10-02, the two pipeline defects above are where they
  were, and `pinned-version-check`'s turned out to depend on the branch
  rather than on the fork. `lint:prose` failed in every fork pipeline since,
  on this branch, on
  row 76's and on the follow-up's, with the same empty revision.
  `pinned-version-check` passed in each of those pipelines: on the two
  branches that carry the marker it skipped before looking for a merge base,
  and on the follow-up's, which carries none, the depth 1 fetch of `main` held
  the freshly rebased branch's base and the check found the bump. It stops at
  `no merge base` on a branch whose base `main` has moved past and whose
  description carries no marker.

  Read on 2026-10-04, the follow-up has had no review from a person and its
  fork pipeline fails only `lint:prose`. The `lint:prose` regression has a fix
  in review from another contributor:
  [gitlab-org/orbit/knowledge-graph!2676](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2676),
  "fix(linting): fall back to HEAD when the MR source SHA is empty", opened
  from the community fork on 2026-09-30, makes `prose_lint.py` read an empty
  `CI_MERGE_REQUEST_SOURCE_BRANCH_SHA` as `HEAD`, which is what the
  `base...head` form did before `b7bce6fb`. Its own fork pipeline passes,
  `lint:prose` included. @peterhegman handed its review to @dgruzd on
  2026-10-02, and it has no approval. The lint runs from the branch's own tree, so once it merges, the
  follow-up and row 76's merge request pass `lint:prose` in a fork pipeline
  only after a rebase onto it.

  Read on 2026-10-05, both merged. @dgruzd wrote that the follow-up looks
  good at 12:47 UTC, resolved its threads, approved it, set it to merge when
  its checks pass and started pipeline 2913575818 in the canonical project,
  which passed with `lint:prose` and `pinned-version-check`; it merged at
  12:59 UTC. He approved
  [gitlab-org/orbit/knowledge-graph!2676](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2676)
  at 12:55 UTC the same way, and it merged at 13:07 UTC (squash commit
  `ba81b335`, merge commit `7450d865`), so a fork branch rebased onto `main`
  now passes `lint:prose`, which row 76's merge request did the same
  afternoon. After the merge, on 2026-10-06, @michaelangeloio asked on the
  follow-up whether the change would help the GQL form of a query too, and
  @aalgutifan answered at 17:39 UTC that it is "already handled by GQL";
  nothing there asks anything of us.
- **Merged**: yes, at 10:15 UTC on 2026-10-02, by @dgruzd, as squash commit
  `374c457c` (merge commit `edfa149d`), which closed
  [gitlab-org/orbit/knowledge-graph#1329](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1329).
  The follow-up that makes the schema itself require `rel_types`,
  [gitlab-org/orbit/knowledge-graph!2691](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2691),
  merged at 12:59 UTC on 2026-10-05, by @dgruzd, as squash commit `fb2e95a3`
  (merge commit `66657589`), with `query_dsl` at 12.1.10. Read at 15:43 UTC
  that day, GitLab.com still ran Orbit 0.135.0 and `/orbit/schema/dsl` still
  served the old `PathConfig.rel_types` description. Both merges are in
  **v0.137.0**, cut the same day (release commit `1872b6f9`, 17:08 UTC), whose
  `config/versions.yaml` carries `query_dsl: 12.1.10`, and GitLab.com serves
  it: read at 21:06 UTC on 2026-10-05, `/orbit/status` reports Orbit 0.137.0
  and `/orbit/schema/dsl` serves version 12.1.10, with `rel_types` in
  `PathConfig.required`, `"minItems": 1`, and the description "Required,
  including when both endpoints use node_ids". The project has tagged
  v0.137.1 (2026-10-06) and v0.138.0 (2026-10-07) since; read at 05:09 UTC on
  2026-10-08, GitLab.com still reports Orbit 0.137.0.
- **Blocking**: no.
- **Workaround**: not needed. `orbit.dsl` hands the model GitLab's schema text
  verbatim, which now states the rule, and `orbit.query`'s own guidance states
  it too since
  [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031): its
  parameter guidance names `rel_types` as required, `["*"]` for any type, and
  the Orbit live suite holds a path query naming them to a 200.

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

**Fix merged**: the merge request states the rule in the schema, the guide,
the skill copy and the recipe, turns the guide's example round, and bumps the
skill. It changes no accepted query, so it left the `query_dsl` version
alone and put that choice, and making `rel_types` required in the schema
itself, to the maintainers in the issue. Its review suggested the second,
which the follow-up above did; it merged on 2026-10-05.

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
  `workflow::ready for review`. At 19:44 UTC on 2026-10-01 @ms.mondrian
  handed the review to @aalgutifan. The branch was rebased that evening with
  row 75's, taking the skill to `0.33.2`, and again at 17:12 UTC on
  2026-10-02 onto the `main` that carries row 75's merge, where only the two
  version lines and the four tests that pin them conflicted and the neighbors
  change stayed the same. The note posted with the second rebase (17:43 UTC)
  tells @aalgutifan that it needs a pipeline in the canonical project, which
  only a project member can start, since a fork pipeline cannot pass
  `lint:prose`. Its fork pipeline on that head fails only that job. At 17:53
  UTC @aalgutifan replied that
  [gitlab-org/orbit/knowledge-graph!2692](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2692),
  merged a minute earlier, stops the project's tests hardcoding the skill
  version, asked for one more rebase onto it, and said the change looks good
  otherwise. That rebase went up at 19:03 UTC the same day, dropping the
  commit that had moved the four version assertions, since the server's skill
  tests now read the version from the skill itself, so what is left is the
  documentation commit, unchanged, with the skill one patch above `main`. The
  note posted with it (19:33 UTC) says so, that the unit tests of
  `orbit-server`, `orbit-cli` and `orbit-prompts` pass locally on the rebased
  tree with no test change, and that a pipeline in the canonical project is still
  needed for `lint:prose`. Read on 2026-10-04, its fork pipeline on that head
  (`a75651db`) fails only `lint:prose`, nobody has answered since, it has no
  approval, and the next step is the reviewer's. The `lint:prose` regression
  had a fix in review from another contributor, named under row 75, which
  merged at 13:07 UTC on 2026-10-05.

  On 2026-10-05 the branch was rebased onto `main` again at 14:10 UTC, taking
  that fix and the follow-up of row 75, whose change to
  `graph_query.schema.json` is in other lines and did not conflict; the skill
  stays one patch above `main` and the documentation commit is unchanged,
  head `f1db93a2`. Its fork pipeline, 2913907361, passed, `lint:prose`
  included, the first fork pipeline of this merge request in which that job
  ran to the end. A note at 14:42 UTC in the thread our rebase note of
  2026-10-02 opened, where @aalgutifan had asked for the last rebase
  ([note 3956267581](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2651#note_3956267581)),
  told @aalgutifan so, that the job no longer needs a pipeline in the
  canonical project, and that the change is ready for another look.
  @aalgutifan approved it at 16:20 UTC and merged it a minute later, which
  closed
  [gitlab-org/orbit/knowledge-graph#1330](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/work_items/1330).
- **Merged**: yes, at 16:21 UTC on 2026-10-05, by @aalgutifan, as squash
  commit `20080819` (merge commit `fc57ab46`). It is in **v0.137.0**, cut the
  same day (release commit `1872b6f9`, 17:08 UTC), and GitLab.com serves it:
  read at 21:06 UTC on 2026-10-05, `/orbit/status` reports Orbit 0.137.0 and
  `NeighborsConfig.direction` in `/orbit/schema/dsl` declares
  `"default": "outgoing"` and describes the default as `outgoing`.
- **Blocking**: no. Until the release a result could be silently incomplete,
  to a client that read the schema's default.
- **Workaround**: not needed. GitLab.com's schema states the default now, and
  `orbit.query`'s own guidance says it too since
  [issue 1031](https://github.com/jmrplens/gitlab-mcp-server/issues/1031):
  the schema description of `query`, its parameter guidance and the site's
  Orbit page each say that `direction` defaults to `outgoing` and to pass
  `both` for every relationship of the center, and the Orbit live suite asks
  its neighbors queries that way.

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

**Fix merged**: the merge request documents `outgoing` in the schema and
adds `direction` and `rel_types` to the guide's neighbors section and its
skill copy. Making `both` the compiler's default instead would change what
existing queries return, so it was left in the issue as the maintainers'
choice, and the merge closed the issue without taking it.

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
  allowlist. Removing the self-update subsystem in 3.0.0 took the openpgp
  import out of every binary, so the source scan has reported nothing since
  and the allowlist is empty again. The module itself stayed in the binaries
  of 3.0.0 and 3.1.0, through the HKDF import in `internal/telemetry`, and a
  scanner that reads a binary's module list rather than its symbols
  (`govulncheck -mode binary -scan module`, and every directory working from
  an SBOM) still reported the advisory against both. The release after 3.1.0
  derives those keys with the standard library's `crypto/hkdf` and names no
  `golang.org/x/crypto` module in its build information, and
  `make check-binary-vulns` holds every release binary to the database at
  that grain. The upstream PR stays worth merging for the module's other
  users.

### gobco type-checks every file of a package directory, whatever its build constraints say

- **Reported**: yes,
  [rillig/gobco#40](https://github.com/rillig/gobco/issues/40), opened
  2026-09-28 with a minimal reproduction of the panic and of four related cases.
- **In review**: yes,
  [rillig/gobco#41](https://github.com/rillig/gobco/pull/41), opened the same
  day from a fork: two commits, one per half of the fix, each with its tests.
  The project's CI has not run on it, which for a first contribution waits on
  the maintainer's approval.
- **Merged**: no. Up to 2024 the maintainer fixed an issue that carried a
  reproduction directly, within days, with a commit of his own, and one pull
  request has ever been merged as sent, in 2019, so the fix may well arrive
  as the maintainer's own commit. The 2026 record is slower: the fix for
  [rillig/gobco#38](https://github.com/rillig/gobco/issues/38) landed on
  `master` three days after the report, on 2026-05-03, with the maintainer
  offering a release once the snapshot worked for its reporter. It did not:
  on 2026-05-04 the reporter showed the snapshot (`7a099954`, the commit read
  under Where below) still panicking in `resolveTypes`, with "could not
  import" errors from the source importer, the type-checking step v1.3.4
  introduced and this entry's issue is about, and the maintainer reopened the
  issue on 2026-05-06, so the release offered there waits on the same path
  as this entry. No release has been tagged since v1.3.4 (2024-03-08),
  `master` being 9 commits ahead of it when read on 2026-10-05, when this
  entry's issue and pull request still had no response. Read again on
  2026-10-07, nothing has moved: v1.3.4 is still the newest tag, `master` is
  still 9 commits ahead at `7a099954`, and the issue and the pull request are
  open with no comment. Either way the entry retires on the release that
  carries the fix, and it is re-read at each gobco tag.
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
  `scripts/check-spec-conditions.sh`, measures `cmd/server` at 2236 of 2236
  conditions and `internal/toolutil` at 3870 of 3870 on linux/amd64. The
  harness is not measured: every file of it sits behind `e2e`, so blanking
  its constraints fails its own test that each file carries exactly that
  line, and the script says the figure it printed is not a measurement. The
  platform halves the copy leaves out (four source files of `cmd/server`, two
  of `internal/toolutil`) are measured on the platforms that build them: the
  Windows and macOS legs of CI's cross-platform job run the same script on
  both packages and fail on a condition left one-way in a file Linux does not
  build. It retires when a gobco release carries the fix and the pin,
  `github.com/rillig/gobco@v1.3.4` in `scripts/coverage-conditions.sh`
  (line 124), moves to it. That change, once the release exists, does four
  things. It moves the pin and measures `test/e2e/internal/harness` directly
  with `TAGS=e2e`, which under the pull request's build gave 863 of 1162
  conditions; once measurable, the harness is held to the gate like any
  package a change touches, which means closing its one-way conditions. It
  retires the staged module copy and its constraint blanking from
  `scripts/coverage-conditions.sh`, with the text that describes them: the
  script's header (line 73 says the harness waits for gobco to honour build
  tags), the staging note of `scripts/check-spec-conditions.sh`, and the two
  paragraphs of `docs/development/testing/README.md` that begin "gobco reads
  a package as every `.go` file" and "The e2e harness stays unmeasured until
  gobco honours build tags". It exempts the `gobco_*.go` files gobco generates
  from `TestSources_EveryFile_CarriesExactlyTheE2EConstraint`
  (`test/e2e/internal/harness/sources_test.go`), which is ours to do and would
  be dead code before the release, since the staged copy blanks the
  constraint lines that test reads. And it marks this entry merged with the
  gobco version, keeping it. Moving the coverage recipes into a Go command
  ([issue 1114](https://github.com/jmrplens/gitlab-mcp-server/issues/1114),
  open) carries the pin with them; if gobco releases first, that command need
  not stage at all.

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

**Where it was tracked**: issue 1017's Linux half was done by pull request
1064 and its Windows and macOS legs by pull request 1206; what waited on gobco
was split into
[issue 1099](https://github.com/jmrplens/gitlab-mcp-server/issues/1099), a
watch re-read at each gobco tag, which this entry replaced when it closed on
2026-10-07. That issue also carried a second gobco limit with the same owner,
which had no entry until then and is
[entry 97](#gobco-cannot-instrument-a-package-whose-export_testgo-feeds-its-external-test-package).

### gobco cannot instrument a package whose export_test.go feeds its external test package

- **Reported**: no. It is neither the fourth case of
  [rillig/gobco#40](https://github.com/rillig/gobco/issues/40), which is the
  build-tag one
  ([entry 73](#gobco-type-checks-every-file-of-a-package-directory-whatever-its-build-constraints-say)),
  nor [rillig/gobco#33](https://github.com/rillig/gobco/issues/33) with its
  open fix [rillig/gobco#39](https://github.com/rillig/gobco/pull/39), which
  are about finding the import path of a black box test's package. Searched
  on 2026-10-07, gobco's tracker has no issue or pull request about
  `export_test.go` or an external test package's symbols.
- **In review**: no. Decided on 2026-10-06, in the maintainer's comment on
  [issue 1099](https://github.com/jmrplens/gitlab-mcp-server/issues/1099):
  this gets a fix and a pull request to gobco, not a workaround in this
  repository's staging script, and the comment schedules both for when the
  gobco release carrying entry 73's fix has landed, so nothing new goes to
  that project while our pull request there is unreviewed.
- **Merged**: no.
- **Blocking**: no for the server. It keeps the condition gate from measuring
  a package it reaches. `scripts/check-spec-conditions.sh` reports such a
  package as not measured and does not fail on it, and
  `docs/development/testing/README.md` says the same; both count 25 of the
  169 packages carrying an `action_specs.go` as in that state, a figure from
  when they were written and not measured again since (read from the tree on
  2026-10-07, 22 of the 150 `action_specs.go` files sit beside an
  `export_test.go` whose package also has an external test package).
  `internal/clientcompat` is held by mutation testing instead (pull request
  1047), and `internal/tools/issues` was measured once, 368 of 368
  conditions, only through a workaround applied by hand to a copy of the
  module outside this repository (pull request 1067), which the
  repository's tooling does not carry.
- **Workaround**: none, by the decision above. When the fix ships in a gobco
  release and the pin of entry 73 moves to it, the "reported and does not
  fail" bound of `scripts/check-spec-conditions.sh` retires with its
  paragraph in `docs/development/testing/README.md`, and `internal/clientcompat`,
  `internal/tools/issues` and the `action_specs.go` packages are measured and
  closed like any other.

**Where**: `rillig/gobco` at `7a09995` (`master`, read on 2026-10-07),
`resolveTypes` (`instrumenter.go:113-150`). It type-checks each package of the
directory with `importer.ForCompiler(fset, "source", nil)`. When it checks the
external `x_test` package, that importer loads `x` from `build.Default`,
which reads the non-test files of a directory only, so a symbol `x`'s
`export_test.go` declares for the black box test does not exist there, and
`ok(err)` aborts the run before anything is instrumented. Type resolution
arrived in v1.3.4 (`09623a7`), so v1.3.3 is presumably unaffected; that is a
reading of the history, not a measurement.

**What**: `export_test.go` is the usual Go idiom for handing an unexported
symbol to a package's black box tests: a file of `package x`, compiled only
into the test binary, that `x_test` reads. `go test` builds it and gobco
cannot: on `internal/tools/todos` it dies with
`action_specs_test.go:49:27: undefined: todos.PublishedActionIDs` before it
writes a single counter.

**How we found it**: the condition gate, first on `internal/clientcompat`
(pull request 1047) and then on `internal/tools/issues` (pull request 1067),
and its cause by reading `resolveTypes`. It was recorded in issue 1099 beside
entry 73's adoption until that issue closed on 2026-10-07.

### go/types reads an imported generic instance another checker is expanding

- **Reported**: yes, by another user, as
  [golang/go#81122](https://github.com/golang/go/issues/81122) on 2026-08-26.
  Read on 2026-09-29 it was open, labelled NeedsInvestigation, with no
  milestone, and read on 2026-10-05 it still is, with no assignee; its
  reporter added that it reproduces with every package loaded from source
  too. Our
  [comment of 2026-09-30](https://github.com/golang/go/issues/81122#issuecomment-5901488429)
  adds a reproducer that needs no x/tools, the go1.26 read, and the change
  below with the source-loaded case it also covers. Two other projects keep
  the race out of their gates and cite the issue:
  [Hikyo-Org/Hikyo commit `93e47b8335`](https://github.com/Hikyo-Org/Hikyo/commit/93e47b8335)
  (2026-09-25) excludes a package from its race shard and calls go1.26
  race-clean for it, where the reproducer below races on go1.26.8 too, and
  [rfizzle/astimate commit `3f1f41b23e`](https://github.com/rfizzle/astimate/commit/3f1f41b23e)
  (2026-09-27) runs one package's tests with `GOMAXPROCS=1`, the lever the
  workaround below pulls.
- **In review**: yes, [golang/go#81871](https://github.com/golang/go/pull/81871),
  opened on 2026-09-29 and imported to Gerrit as
  [go.dev/cl/841585](https://go.dev/cl/841585), open. Patch set 2
  (2026-09-30) holds both constructions in its test, the importer's and a
  package checked from source. Gerrit's automatic assignment named Robert
  Findley and Robert Griesemer; just before midnight UTC on 2026-09-29 Alan
  Donovan replaced Robert Findley with Mark Freeman and added himself in
  copy, so its reviewers are Robert Griesemer and Mark Freeman. On 2026-09-30
  Mark Freeman asked for a shorter commit message, patch set 3 of the same
  day carries it with the code unchanged, and that
  thread is resolved; no vote yet. Read on 2026-10-05 through Gerrit's REST
  API, nothing has moved since patch set 3: both reviewers are in the
  attention set, no label carries a vote, and no TryBot has run, which takes
  a maintainer's `Commit-Queue+1`. Three submit requirements are unmet:
  `Code-Review` wants a `+2` from someone other than the uploader,
  `Review-Enforcement` wants two trusted contributors to approve, and the
  upload, made by GerritBot, does not count as one, and `TryBots-Pass` wants
  the TryBots; `No-Unresolved-Comments` is met. The GitHub pull request is
  open on the head patch set 3 imports (`87fa66c4`). A reminder on the change
  is planned for no earlier than 2026-10-14, for the maintainer to post
  himself.
- **Merged**: no. No Go release carries a fix, and `master` still reads the
  field the same way.
- **Blocking**: no for the server, which type-checks nothing. It failed the
  race gate of a release rehearsal, in
  `TestAccess_OnlyTheSanctionedPackagesReadTheKey` of
  `internal/testutil/modelcorpus`, and any race run of a test that
  type-checks two or more packages in one go/packages load can fail the same
  way at random. Measured on 2026-09-29, 20 test packages did: the audits under
  `cmd/` that type-check Go source with go/packages, `cmd/internal/goprogram`,
  `cmd/internal/graphqldocs` and `internal/testutil/modelcorpus`. Read from
  the sources of `origin/main` on 2026-10-05, the packages added since that
  load programs in their tests are `cmd/gen_action_grants`, its
  `internal/derive` and `cmd/internal/actionrequests`, which belong on that
  list; `cmd/audit_1to1/internal/grants`, `cmd/gen_action_grants/internal/join`
  and `cmd/internal/sdkroutes` link go/packages only through the packages
  they import and load nothing, and neither does `cmd/audit_binary_vulns`,
  added on 2026-10-01. That is a reading of the
  sources, not the measurement of 2026-09-29, which has not been repeated.
- **Workaround**: yes. `internal/testutil/serialtypecheck` is imported blank by
  one test file of each of the 29 packages whose test binary links
  go/packages, counted on `origin/main` on 2026-10-05 (23 when this was last
  written, on 2026-10-01; the six since are those of the fine-grained grant
  derivation, `cmd/audit_1to1/internal/grants`, `cmd/gen_action_grants` with
  its `internal/derive` and `internal/join`, `cmd/internal/actionrequests`
  and `cmd/internal/sdkroutes`). In a race build its initializer sets
  GOMAXPROCS to one before go/packages sizes the semaphore it type-checks
  under, so the checkers run one after another; in any other build it changes
  nothing. Its tests hold
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
