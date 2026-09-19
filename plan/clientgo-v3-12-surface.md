# What client-go v3.12.0 opens, and what the one-to-one promise now owes

Twelve minor versions of `gitlab.com/gitlab-org/api/client-go` landed while the pin
stood at v3.0.0. This is the work list the bump to v3.12.0 opens, ordered by what a
caller loses rather than by count.

**It is a report, not a change.** Nothing here is implemented. Acting on it is the
one-to-one campaign's own work, tracked separately.

## How it was measured

Two runs of `cmd/audit_1to1` on **one tree**, differing only in `go.mod` and `go.sum`.
The bump commit was cherry-picked, the two files were reverted to the parent's, the
audit was run, then they were restored and it was run again. Everything else, this
repository's own source included, is byte-identical between the runs, so the whole
delta is the SDK.

```bash
go run ./cmd/audit_1to1/ -gaps-only -output <file>   # R-INPUT/R-OUTPUT/R-ACTION/R-META/R-ENUM
go run ./cmd/audit_1to1/ -scope=sdk    -output <file>  # R-SERVICE/R-GRAPHQL/R-ENUM
go run ./cmd/audit_1to1/ -scope=paths  -output <file>  # R-PATH and R-PAGE
```

Three oracles were consulted beyond the audit: the SDK source in the module cache
(`/root/go/pkg/mod/gitlab.com/gitlab-org/api/client-go/v3@v3.12.0`), its `CHANGELOG.md`,
and the committed live GitLab record `docs/development/gitlab-api-live.json`
(`gitlab/gitlab-ee:latest`, 19.3.1-ee, retrieved 2026-09-09). GitLab's own API
documentation was read where the live record and the SDK disagreed.

## The short answer

The bump costs this server six parameters across four existing actions, one output
field across three output types, and two endpoints it has no action for. It also
retires twelve packages' captured-response workarounds, which are ours upstream.

Every item on the work list is somebody else's contribution to client-go. Every field
this project contributed upstream is already published here, which is why the bump's
net effect on the audit is that thirty-one findings disappear.

## The audit delta

`make audit-1to1`, summary block, before and after:

| Counter                  | v3.0.0 | v3.12.0 | Change |
| ------------------------ | -----: | ------: | -----: |
| `packages`               |     59 |      60 |     +1 |
| `struct_missing_input`   |      6 |      10 |     +4 |
| `struct_missing_output`  |    323 |     325 |     +2 |
| `struct_extra_output`    |    108 |      77 |    -31 |
| `action_missing_methods` |      4 |      17 |    +13 |
| `meta_*` (all four)      |      0 |       0 |      0 |
| `enum_missing_values`    |      0 |       0 |      0 |
| `enum_extra_values`      |      0 |       0 |      0 |

At field grain that is **7 new findings and 32 retired**. The seven:

| Stream   | Package           | Type          | Paired with                       | Field                      |
| -------- | ----------------- | ------------- | --------------------------------- | -------------------------- |
| R-INPUT  | appearance        | `UpdateInput` | `v3.ChangeAppearanceOptions`      | `site_name`                |
| R-INPUT  | broadcastmessages | `CreateInput` | `v3.CreateBroadcastMessageOptions`| `color`                    |
| R-INPUT  | broadcastmessages | `UpdateInput` | `v3.UpdateBroadcastMessageOptions`| `color`                    |
| R-INPUT  | projects          | `UpdateInput` | `v3.EditProjectOptions`           | `automatic_rebase_enabled` |
| R-OUTPUT | projects          | `Output`      | `v3.Project`                      | `automatic_rebase_enabled` |
| R-OUTPUT | projects          | `BasicOutput` | `v3.Project`                      | `automatic_rebase_enabled` |
| R-OUTPUT | environments      | `ProjectOutput`| `v3.Project`                     | `automatic_rebase_enabled` |

`action_missing_methods` moves by 13 rows but by only **4 distinct methods**: the
stream reports one row per package that touches the service, so `GetProjectServiceAccount`
appears six times and `GetServiceAccount` five. The four rows that were already there
(`SubscribeToMergeRequest` and `UnsubscribeFromMergeRequest`, in `mergerequests` and
`mrchanges`) predate the bump and are not part of this work list.

`-scope=sdk` is **identical** on both sides: 169 services, 157 covered, 12 declared,
0 undeclared, 17 GraphQL operations all adjudicated, 649 enum fields with 0 gaps,
0 stale declarations. `-scope=paths` is identical on every summary counter, including
`sdk_graphql_documents` 42, `sdk_graphql_refused` 0 and `typed_unsurfaced_not_in_sdk`
1243; the only textual difference in its report is the module version string and the
source positions inside the `sdk_graphql` section.

## The work list

| # | What a caller cannot do today                                     | Kind        | Where               | Tier |
| - | ----------------------------------------------------------------- | ----------- | ------------------- | ---- |
| 1 | Ask a job for one report instead of the whole archive             | input       | `job.artifacts`     | Free |
| 2 | Finish a review: set the reviewer state and the summary note      | input       | `mr_review.draft_note_publish_all` | Free |
| 3 | Read one service account by id                                     | **action**  | group and project   | Free |
| 4 | See whether a project rebases the source branch automatically      | output      | three types         | unknown |
| 5 | Set the instance site name                                         | input       | `admin.appearance_update` | Free |
| 6 | Set a broadcast message's deprecated background colour             | input       | `admin.broadcast_message_*` | Free |
| 7 | (housekeeping) twelve packages still read a field from the capture | workaround  | twelve packages     | n/a  |

### 1. `job.artifacts` cannot ask for one report

`JobsService.GetJobArtifactsWithOptions` takes `GetJobArtifactsOptions{FileType *ArtifactFileTypeValue}`
on the route `job.artifacts` already calls, `GET /projects/:id/jobs/:job_id/artifacts`.
The SDK declares 24 values for it, a type it did not express at all before:
`accessibility`, `api_fuzzing`, `archive`, `browser_performance`, `cluster_image_scanning`,
`cobertura`, `codequality`, `container_scanning`, `cyclonedx`, `dast`, `dependency_scanning`,
`dotenv`, `jacoco`, `junit`, `license_scanning`, `load_performance`, `lsif`, `metrics`,
`performance`, `requirements`, `requirements_v2`, `sarif`, `sast`, `secret_detection`.

This is first on the list because the loss is not a nuisance, it is the capability.
`readArtifactContent` in `internal/tools/jobs/jobs.go` caps the response at
`maxArtifactBytes`, which is **1 MiB**, base64-encodes it and sets `truncated`. A model
that wants a JUnit or SAST report out of an archive larger than 1 MiB cannot get it
through any call this server offers: it receives a truncated zip it cannot open, and
no sibling action helps, because `job.download_single_artifact` wants a path inside the
archive rather than a report type. With `file_type` the server asks GitLab for that one
report and the 1 MiB cap stops mattering for the common case.

Cost: one optional input field, one enum, and swapping the call for its `WithOptions`
sibling. `GetJobArtifacts` is kept upstream and delegates, so nothing else moves.

### 2. `mr_review.draft_note_publish_all` cannot finish the review

`DraftNotesService.PublishAllDraftNotesWithOptions` takes `PublishAllDraftNotesOptions`
with three fields, on the route the action already calls:

| Field           | Kind              | What it is                                                    |
| --------------- | ----------------- | ------------------------------------------------------------- |
| `note`          | content           | summary note posted on the merge request with the batch       |
| `internal`      | behaviour switch  | makes that summary note internal                              |
| `reviewer_state`| behaviour switch  | sets the reviewer's review state: `reviewed` or `requested_changes` |

`PublishAllInput` in `internal/tools/mrdraftnotes/mr_draft_notes.go` carries
`project_id` and `merge_request_iid` and nothing else, and the handler calls
`PublishAllDraftNotes` with no options.

`reviewer_state` is the one that matters. Nothing in this repository sets a reviewer
state: `grep -rn reviewer_state --include=*.go internal/` returns nothing,
`merge_request.reviewers` only lists them and `merge_request.update` sets
`reviewer_ids`. Approval is not a substitute, and GitLab says so in its own parameter
text: "Does not record a formal approval". So a model can post a review's comments and
cannot say it requested changes. The bulk-publish call is the only REST route to that
state.

Our own live record already declares all three params on that route, so this one needs
no new oracle:

```text
POST /api/:version/projects/:id/merge_requests/:merge_request_iid/draft_notes/bulk_publish
  internal       => {"type": "Grape::API::Boolean", "default": "false", "desc": "If true, the summary note is internal"}
  note           => {"type": "String", "desc": "Summary note body to post on the merge request"}
  reviewer_state => {"type": "String", "desc": "Set reviewer review state after publishing. Does not record a formal approval"}
```

The two `reviewer_state` values come from GitLab's documentation rather than from the
SDK, which types the field `*string` with no constants. They therefore need an
`InputSchemaOverride`, the way `project.service_account_pat_list` declares `state`.

### 3. Two endpoints with no action: read one service account

| SDK method                                  | Endpoint                                     | Action it would be          | Individual tool |
| ------------------------------------------- | -------------------------------------------- | --------------------------- | --------------- |
| `GroupsService.GetServiceAccount`           | `GET /groups/:id/service_accounts/:user_id`  | `group.service_account_get` | `gitlab_group_service_account_get` |
| `ProjectsService.GetProjectServiceAccount`  | `GET /projects/:id/service_accounts/:user_id`| `project.service_account_get` | `gitlab_project_service_account_get` |

Both packages have `list`, `create`, `update`, `delete` and the four PAT actions, and
neither has a `get`. Checked against the catalog rather than guessed:
`go run ./cmd/server --tool-search service_account` returns 19 actions, and no `_get`
among them at either scope.

This is a capability the one-to-one promise claims and the server does not have, but it
ranks below the two above because a workaround exists: list the collection and filter.
That workaround costs pages, and it fails outright for an account past whatever page a
model stops at, since the list is offset-paginated at 20.

Both would slot into their existing packages with `NewReadActionSpec`, reusing the
`Output` type the list and create actions already publish. The output type is already
1:1 with `GroupServiceAccount` as of this bump, `public_email` and `unconfirmed_email`
included.

### 4. `automatic_rebase_enabled`: three output types, and one input to verify first

`Project.AutomaticRebaseEnabled` is the only genuinely new response field in the bump.
Nothing in this repository declares `automatic_rebase_enabled` at all. It is missing
from:

- `internal/tools/projects` `Output` (the busiest output type here)
- `internal/tools/projects` `BasicOutput`
- `internal/tools/environments` `ProjectOutput`

The output half is a straightforward 1:1 addition. **The input half is a candidate
phantom and should not be added on the SDK's word alone.** client-go also added
`EditProjectOptions.AutomaticRebaseEnabled`, and GitLab documents the attribute only as
a project *response* attribute, in "Retrieve a project", "List all projects" and "List
all personal projects for a user", introduced in GitLab 19.4. A reading of the "Edit a
project" attribute table could not be completed (the page is long enough that the fetch
truncated before reaching it), and the committed live record declares
`automatic_rebase_enabled` as a param on **zero** routes. Publishing the input before
that is settled would hand a model a knob GitLab may ignore, which is the failure mode
`mrapprovals.ConfigOutput` was built as a fixture against.

The check that settles it is `make gen-api-live` against a 19.4 image: it reads the
Grape param declarations out of the booted application, so it answers the input
question and the output question together.

### 5. `admin.appearance_update` cannot set `site_name`

`ChangeAppearanceOptions.SiteName`. `UpdateInput` in `internal/tools/appearance/appearance.go`
carries eighteen fields and not this one. The surface is asymmetric today: the
`Appearance` entity sends `site_name` unconditionally in the live record, and
`admin.appearance_get` already publishes it, so a caller can read the value and cannot
write it. `PUT /api/:version/application/appearance` declares the param in the same
record, with the description "Last part of the webpage title. Defaults to empty."
Nothing else is needed to act on this one.

### 6. `admin.broadcast_message_create` and `_update` cannot set `color`

`CreateBroadcastMessageOptions.Color` and `UpdateBroadcastMessageOptions.Color`. Last on
the list because GitLab deprecates it in the parameter's own text, and the replacement
is a field this server already accepts:

```text
POST /api/:version/broadcast_messages
  color => {"type": "String", "desc": "Background color (Deprecated. Use \"theme\" instead.)"}
```

`CreateInput` already carries `theme` with its ten values. The read side is already
1:1: `MessageItem` publishes `color`, which is one of the thirty-one findings the bump
retired. So the decision here is whether strict 1:1 means offering a deprecated
parameter beside its replacement, or answering it with a declaration. Either is
defensible; what is not defensible is leaving it unexamined.

### 7. Twelve packages can stop reading the capture

The bump commit deliberately kept every `WithResponseCapture` read. client-go now models
each of those fields, so both decoders read the same key and the workaround is dead
weight. This is the "quitar workarounds" half of the request, and it is the whole of the
thirty-one retired `extra_output` findings plus one retired `missing_output`:

| Package                | Output type               | Fields the SDK now models                                                                 |
| ---------------------- | ------------------------- | ----------------------------------------------------------------------------------------- |
| appearance             | `Item`                    | `site_name`                                                                                 |
| broadcastmessages      | `MessageItem`             | `color`                                                                                     |
| clusteragents          | `AgentItem`               | `is_receptive`                                                                              |
| deploykeys             | `Output`, `InstanceOutput`| `last_used_at`, `usage_type` on both                                                        |
| events                 | `ContributionEventOutput`, `ProjectEventOutput` | `imported`, `imported_from`, `wiki_page` on both                      |
| groupscim              | `Output`                  | `extern_uid` (the json tag fix; `external_uid` was never sent)                              |
| groupserviceaccounts   | `Output`                  | `public_email`, `unconfirmed_email`                                                         |
| licensetemplates       | `LicenseItem`             | `popular`                                                                                   |
| namespaces             | `Output`                  | eight storage, seat and runner-minute fields                                                |
| securefiles            | `SecureFileItem`          | `file_extension`                                                                            |
| snippets               | `Output`                  | `http_url_to_repo`, `ssh_url_to_repo`, `imported`, `imported_from`                          |
| topics                 | `TopicItem`               | `organization_id`                                                                           |

Retiring a read means deleting the capture wrapper, the small decode type, and the
negative test's reason for existing. The eleven tests that moved to
`testutil.AssertUnreadableBodyRefused` in the bump commit go with it: once the handler
stops reading a capture there is no second decoder, and the strict assertion is the
wrong shape either way.

`docs/development/upstream-bugs.md` row 34 records this family as "12 of 14; 11
released, v3.1.0 to v3.10.0". That is now one release behind: the twelfth, the eight
namespace fields (`!3051`), shipped in v3.11.0. The row wants a pass when the captures
are retired.

## Answers to the four questions

### 1. New services or methods with no action

**No new service.** `-scope=sdk` reports 169 services, 157 covered and 12 declared on
both sides of the bump, with zero undeclared and zero stale declarations. The service
universe question is green and the bump did not touch it.

**Four newly uncovered methods**, of which two are capability gaps and two are the same
routes with an options struct:

| Method                                        | Route                                          | Gap                         |
| --------------------------------------------- | ---------------------------------------------- | --------------------------- |
| `GroupsService.GetServiceAccount`             | `GET groups/%s/service_accounts/%d`            | no action (item 3)          |
| `ProjectsService.GetProjectServiceAccount`    | `GET projects/%s/service_accounts/%d`          | no action (item 3)          |
| `JobsService.GetJobArtifactsWithOptions`      | `projects/%s/jobs/%d/artifacts`                | action exists, options do not (item 1) |
| `DraftNotesService.PublishAllDraftNotesWithOptions` | `POST projects/%s/merge_requests/%d/draft_notes/bulk_publish` | action exists, options do not (item 2) |

The fifth added method, `EventWikiPage.String`, is a stringer with no route.

### 2. SDK structs that gained fields we do not publish

One field, three of our types: `Project.AutomaticRebaseEnabled`, item 4.

That is the whole answer, and it is worth saying why the number is one when the SDK
gained 44 response fields. Every other one of the 44 is a field **this server already
publishes** through a captured response, so the bump moved it from `extra_output`
(we publish it, the SDK does not model it) to matched. That is the entire `-31`.

The nested case was checked too: the new `EventWikiPage` type carries `format`, `slug`,
`title` and `wiki_page_meta_id`, and `events.WikiPageOutput` already carries the same
four under the same keys.

### 3. SDK option structs that gained parameters we do not accept

Six parameters across four actions. Four of them the audit sees; two of them it
structurally cannot, which is why they are described here rather than read off the
report.

| Parameter        | Action                             | Kind             | What a caller loses without it |
| ---------------- | ---------------------------------- | ---------------- | ------------------------------ |
| `file_type`      | `job.artifacts`                    | field selector   | the report itself, past 1 MiB  |
| `reviewer_state` | `mr_review.draft_note_publish_all` | behaviour switch | the review's verdict           |
| `note`           | `mr_review.draft_note_publish_all` | content          | the batch's summary comment    |
| `internal`       | `mr_review.draft_note_publish_all` | behaviour switch | keeping that comment internal  |
| `site_name`      | `admin.appearance_update`          | field selector   | one setting it can already read |
| `color`          | `admin.broadcast_message_create` and `_update` | field selector | a deprecated alias of `theme` |
| `automatic_rebase_enabled` | `project.update`         | behaviour switch | verify first: see item 4       |

A field selector on a list that already paginates would be a nuisance. `file_type` is
not that: it is the difference between receiving a report and receiving a truncated
archive. `reviewer_state` is not that either: it is a state nothing else here can set.

### 4. Which of it is licensed surface

| Item                                          | Tier        | Evidence |
| --------------------------------------------- | ----------- | -------- |
| `group.service_account_get`, `project.service_account_get` | **Free** | Service Accounts API page badge: "Free, Premium, Ultimate". Matches how this repository already treats the eight sibling actions: neither package is passed through `editionTaggedSpecs`, and both declare "Available on all tiers" in their usage text |
| `file_type` on job artifacts                  | **Free**    | Job Artifacts API page badge: "Free, Premium, Ultimate" |
| `note`, `internal`, `reviewer_state`          | **Free**    | Draft Notes API page badge: "Free, Premium, Ultimate". The live record's licensed-feature table has `review_merge_request => premium`, but nothing ties it to this route's params, and the page badge is unambiguous |
| `site_name`                                   | **Free**    | The live record's `API::Entities::Appearance` exposes `site_name` unconditionally, and no licensed feature names it |
| `color`                                       | **Free**    | `API::Entities::System::BroadcastMessage` exposes `color` unconditionally |
| `automatic_rebase_enabled`                    | **unknown** | Not sent by any entity in the live record and declared on no route there, so there is no condition text to read and no licensed feature to resolve. GitLab's documentation carries no per-attribute badge for it. Stated as unknown rather than guessed |

None of the six Free items needs a `tier:` struct tag. Item 4 cannot get one until the
oracle can answer.

## The prerequisite: the live oracle predates this surface

`docs/development/gitlab-api-live.json` is `gitlab/gitlab-ee:latest` at **19.3.1-ee**,
retrieved 2026-09-09. Measured against it today:

- no `GET` on `/groups/:id/service_accounts/:user_id` or `/projects/:id/service_accounts/:user_id`.
  Those paths carry only `DELETE` and `PATCH`. The record does capture GET-by-id routes
  in general (`/projects/:id/hooks/:hook_id`, `/projects/:id/deploy_keys/:key_id` are
  both there), so this is an absence rather than a scanning artifact
- no `file_type` param on `GET /projects/:id/jobs/:job_id/artifacts`
- no `automatic_rebase_enabled`, on any entity or as any route's param

GitLab's own documentation dates `file_type` and `automatic_rebase_enabled` to **19.4**
and documents both service-account retrieve endpoints with the exact paths the SDK
builds. So the oracle is not wrong, it is one release behind the surface client-go
now wraps.

**Consequence for the work list.** Items 1, 3 and 4 are 19.4 surface. Build any of them
against today's record and R-PATH's shape readers will report the new field as one
GitLab does not send, the endpoint gate would have nothing to match, and the Docker
e2e instance may answer 404 until its image moves. Regenerate the record once 19.4 is
the released `gitlab-ee` image (`make gen-api-live`), and the same run settles the
`automatic_rebase_enabled` input question. Item 2 needs none of this: its three params
are already in the committed record. Items 5 and 6 likewise.

## Two blind spots this exercise exposed

Both are worth recording, because between them they hide the two highest-value items on
the list.

**R-INPUT cannot see an option struct no action passes.** It compares an action's input
type with the SDK option struct that action's handler hands to the SDK. A brand new
option struct that no handler passes pairs with nothing, so it produces no input
finding at all. `GetJobArtifactsOptions` and `PublishAllDraftNotesOptions` raised
**zero** R-INPUT findings between them, and surfaced only as R-ACTION rows saying a
method is not covered. The four parameters they carry are items 1 and 2, the top of
this list. The report shape is misleading in the same move: "missing method" reads as a
capability we lack, when the capability is registered and it is the parameters that are
missing.

**R-ENUM keys on the same pairing, so a new enum with no paired field is invisible.**
The SDK gained 24 `ArtifactFileTypeValue` constants, a value set it did not express
before. `enum_fields` reads 649 on both sides of the bump and no row in the report
mentions the type. `reviewer_state` is the harder half of the same problem: client-go
types it `*string` with no constants at all, so even a paired action would learn no
values from the SDK, and the two would have to be declared here from the documentation.

The shared cause is that all five surface rules pair through an action. A gap that
exists *because* this server calls the older method is a gap the pairing cannot reach.
Worth considering: hold every exported `*Options` struct of the SDK against the option
structs our handlers actually construct, which needs no action pairing and would have
named both of these on the day the bump landed.

## Attribution

Four contributions by three people outside this project account for the entire work
list. Every field this project contributed upstream between v3.1.0 and v3.11.0 is
already published here, which is why our side of the bump is subtraction.

| Merge request | Author                        | Released | What it opens here |
| ------------- | ----------------------------- | -------- | ------------------ |
| `!3025`       | Anton Kuligin                 | v3.7.0   | item 4             |
| `!3031`       | Andreas Kunze                 | v3.8.0   | item 1             |
| `!3056`       | Andreas Kunze                 | v3.12.0  | item 3             |
| `!3057`       | Andrei Zubov                  | v3.12.0  | item 2             |

The one other change by an outside contributor in this range, `!3054` by Oleksandr
Redko, rewrites documentation examples from `gitlab.Ptr` to `new` and moves no surface.

Ours, all of them field additions that retire a capture read here: `!3042` (v3.1.0),
`!3040` and `!3046` (v3.2.0), `!3043` and `!3045` (v3.3.0), `!3047` (v3.4.0), `!3053`
(v3.5.0), `!3049` (v3.6.0), `!3044` (v3.7.0), `!3041` (v3.9.0), `!3050` (v3.10.0),
`!3051` (v3.11.0).
