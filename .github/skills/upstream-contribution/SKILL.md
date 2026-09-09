---
name: upstream-contribution
description: "Contribute bug fixes or features to upstream projects (gitlab.com/gitlab-org/api/client-go). Use when: API gap found, client-go bug, missing endpoint wrapper."
---

# Upstream Contribution — GitLab client-go

Contribute bug fixes, missing endpoint wrappers, or missing struct fields to the upstream GitLab API client library this project depends on.

Every rule below was read off the upstream project's own configuration, its triage automation or its merged history, and the source is named beside it. Where two readings disagreed, the one that survived an adversarial pass is the one written here.

## Before starting

1. Identify the specific gap or bug in `gitlab.com/gitlab-org/api/client-go/v3`, with the GitLab API source or documentation that proves it.
2. Verify it is not already fixed in the latest release.
3. **Search the upstream project's open merge requests for the same struct, field or file, from any author.** If one already covers it, do not open a second: read it, judge whether it is complete (does it carry every key the entity exposes, does it change the options struct where the endpoint accepts the parameter, does its test actually decode the field), and if something is missing, say so in a comment on that merge request with the same evidence a new one would have carried. A duplicate costs a maintainer more than it saves.
4. Record the finding in `docs/development/upstream-bugs.md`, the permanent register of upstream defects and gaps found from this codebase: follow its entry schema, link the tracker item with a full URL (never a bare `!NNN` or `#NN`, since the repository is mirrored between GitHub and GitLab), and keep the entry when the fix lands, marking it merged with the version that carries it.

## Upstream project

- **Repository**: <https://gitlab.com/gitlab-org/api/client-go> (project id 65271576)
- **Community fork**: <https://gitlab.com/gitlab-community/gitlab-org/api/client-go> (project id 65275361)
- **Language**: Go
- **Branch model**: `main` (default)
- **Reviewers**: `.gitlab/CODEOWNERS` is one line naming four people
- **Style**: `.gitlab/duo/mr-review-instructions.yaml` is the reviewer's actual checklist; `AGENTS.md` and `docs/CONTRIBUTING.md` carry the rest

## Opening a merge request

### 1. Branch in the community fork

Push to the community fork, never to a personal one. `docs/CONTRIBUTING.md` recommends it, and GitLab's triage bot posts a "did you know about our community forks" nudge on every merge request opened from a personal fork. The pipeline runs either way, so CI is not the reason.

The fork is shared by many contributors, so prefix branch names with the account: `jmrp-<topic>`.

```bash
git push https://oauth2:$GITLAB_COM_TOKEN@gitlab.com/gitlab-community/gitlab-org/api/client-go.git HEAD:refs/heads/jmrp-<topic>
```

Then open the merge request from project `65275361` with `target_project_id=65271576`.

**A merge request's source project and branch cannot be changed after it is opened** — the update endpoint accepts neither parameter and answers with the list of ones it does accept. Moving one means opening a replacement and closing the original with a note naming it.

### 2. Target `main`

Target a `release-client-N.0` branch only when the change breaks the public API. A new struct field or a corrected json tag does not.

### 3. Write a conventional-commit title, and prefix every commit

Title: `feat(topics): add OrganizationID to the Topic struct`. `feat` for a new field or endpoint, `fix` for a wrong tag or a bug.

**The scope is the API file, and it is expected even though conventional-commit does not require one.** `fix: correct the json tag` parses as conventional and reaches the release notes, so a check for "has a prefix" passes it, and the project's merged history is overwhelmingly file-scoped (`fix(group_boards)`, `fix(feature_flags)`, `fix(issues)`). Write the scope.

The release-notes generator strips the prefix and **omits a non-conventional title entirely**, so an unprefixed title merges and then never appears in the changelog. Of 279 changelog entries, none differs from its merge request title.

`AddingAPISupport.md` step 9 asks for the prefix on **every commit**, not only the merge request title.

Never use a `(no-release)` scope, and never put `[delay release]` in the description: the first marks work that deliberately cuts no version, the second delays the release job.

### 4. Use the project's description headings

Three `##` headings, in this order: "What does this MR do?", "Is this a breaking change?", "How was this tested?". The repository has issue templates and no merge request template, so this is convention read off merged requests rather than something enforced, which is exactly why it gets skipped: seven merge requests opened from here carried ad-hoc headings or none until they were rewritten.

Answer the breaking-change heading honestly rather than with a flat "no". Correcting a json tag is source-compatible and still changes the value a caller decodes, which is worth one sentence.

Say the gap was found while developing this MCP server and link the repository: the backlink is the point of contributing from here.

**Always reference the drift issue.** Every merge request that comes out of the field-by-field review carries this line, on a line of its own, at the end of the description:

```text
Related to https://gitlab.com/gitlab-org/api/client-go/-/issues/2300
```

Issue 2300 is the umbrella: it publishes the method, the per-struct lists of fields GitLab sends that the library does not model, and the table of what has been sent so far. `Related to` is deliberate and `Closes` would be wrong, because the umbrella has to survive the first merge rather than be closed by it.

Two separate mechanisms read that line, and confusing them is what produced the wrong rule this replaces.

- **`apply_labels_from_related_issue.rb`** copies the first `type::` label off the referenced issue, and only onto a merge request that carries none, so a label already asked for by comment is not overwritten. It re-fires whenever the description is edited. The reference must start a line with one of `related to`, `relates to`, `relate to`, `contributes to`, `contribute to`, `closes`, `close`, `see`; `fixes`, `resolves` and `updates` are **not** in that list, and merge requests using them merged with no `type::` label ever. Issue 2300 carries no `type::` label of its own, having been opened by an account that cannot set labels, so referencing it copies nothing.
- **The contributor platform** applies a `linked-issue` label of its own a few minutes after creation, when the merge request's GraphQL `linked_work_items` connection is non-empty. A `MENTIONED` link is enough; it does not have to be `CLOSES`. That label is what carries the linked-issue point bonus, which is why one umbrella issue referenced from every merge request scores nearly the same as opening a throwaway issue per merge request, and reads as evidence rather than as noise.

`CONTRIBUTING.md` asks for no issue before a merge request, and a maintainer has said per-merge-request issues are noise. One well-evidenced umbrella is the shape that satisfies both.

### 5. Do not set fields the account cannot set

`MergeRequests::BaseService#filter_reviewer` deletes `reviewer_ids` **silently** unless the caller can administer the merge request, at creation and after. The same holds for assignee, labels and milestone: a non-member account has `adminMergeRequest: false` and `createLabel: false`. Setting them through the API looks like it worked and changes nothing. Section "Moving an open merge request" below is how those fields are actually reached.

Do not open as a draft; the ready command issues `/ready` regardless.

Do not request `@GitLabDuo` yourself. That request is what produces the `DCR4003` warning ("you don't have permission to create a pipeline for Code Review Flow"), and it is not the review that counts: on merged requests `gitlab-bot` requests Duo after the ready command and Duo then posts a real review.

**This one cannot be undone.** Removing a reviewer means `reviewer_ids`, which is exactly the field dropped silently for a non-member, so a Duo request made at creation stays on the merge request with its warning comment for the life of it. Seven merge requests opened from here carry one permanently.

Do not write a `Changelog:` trailer and do not touch `CHANGELOG.md`: semantic-release writes it.

## Code rules

From `.gitlab/duo/mr-review-instructions.yaml`, which is what the reviewer checks against:

- `int64` never `int`, and `any` never `interface{}` — inside slices and maps too.
- Pointer structs for requests, non-pointer for responses.
- `PathEscape()` on every path parameter interpolated into a route. Query values are not path segments: encode those with `url.Values.Encode()`, or `url.QueryEscape()` for a single value, since `url.PathEscape` leaves a `+` alone and form-style decoding then reads it as a space.
- Project and group ids typed `any`, parsed with `parseID()`.
- lowerCamelCase locals.
- "GitLab", never "Gitlab" or "gitlab", in comments, log messages, test names and any other text.
- Put a new struct field where the API documentation puts it, **not** at the end of the struct.

From `AGENTS.md`, and the single most-repeated demand in the project's own documents: every public function, type and method carries a comment beginning with its own name, in the present tense, wrapped under 80 characters, ending in a `// GitLab API docs: <url>` line.

Tests:

- Assert the new field specifically.
- Start with `t.Parallel()`, use `setup(t)`, `testMethod(t, r, ...)` and testify `assert`/`require`; never `reflect.DeepEqual`.
- Inline JSON for a small response, `mustWriteHTTPResponse(t, w, "testdata/...")` for a large one.
- GIVEN/WHEN/THEN comments are for complex tests and may be omitted.

Run `make reviewable` (setup, generate, fmt, lint, test) before opening, and regenerate mocks whenever a signature changes.

Expect `danger-review` and `autolabels` to fail: both are `allow_failure: true`. This matters beyond the noise — `autolabels` is the job that would have derived the `type::` label from the conventional prefix, so the title-to-label route is dead and the label has to be asked for by comment.

## Moving an open merge request

In this order.

1. **Fix the title** if it has no conventional prefix, or has one with no file scope. Title and description are the two fields the author may still edit.
2. **Fix the description** if it is missing the three headings, the backlink or the `Related to` line for issue 2300, keeping every piece of evidence it already carries. Do it **before** the label command: editing a description re-fires `apply_labels_from_related_issue.rb`, and although that script only labels a merge request carrying no `type::` label at all, and so cannot overwrite one already asked for, doing the edit first removes the question entirely.
3. **Ask for the label**: post `@gitlab-bot label ~"type::feature"` (or `~"type::bug"`) as a comment, with the command at the start of its own line. `command_mr_label.rb` accepts it from the resource author, and its allowed scopes include `type`. The rate limit is the module default of 60 per hour keyed on the actor.
   Pick by what should ship: `type::feature` cuts a minor, `type::bug` a patch, `type::maintenance` cuts nothing. There is no urgency — an unlabelled merge request still merges and still ships under "Other Changes", and the analyzer reads labels through the API at release time, so a label added later still counts.
4. **Ask for review, naming a code owner**: post `@gitlab-bot ready @<code owner>`. `command_mr_request_review.rb` reacts to the author's own note and emits `/ready`, the `workflow::ready for review` label and `/request_review`; with arguments it requests exactly those users. This is the only mechanism by which a non-member gets a chosen name onto the reviewer field. Posted **bare** it picks a random GitLab-wide merge request coach, who is usually not one of the four code owners.
   One ready command per merge request per hour: the limit is 1 for a non-member and the cache key is the actor **plus** the merge request path, so several merge requests can be readied within the same hour and a misfire costs an hour on that one only.
5. **Then stop.** One approval is required and this account cannot approve; the fork pipeline is already green and needs no maintainer to start it. Do not re-post `ready` if nothing happens — it is rate-limited and would only re-request the same person, and `gitlab-bot` nudges an unattended merge request on its own.

## The same gap is usually in GitLab's own documentation

A field client-go does not model is often a field GitLab never documented either, and the merge request is not finished until both are sent. This is not a guess: cross-checking the eight fields of the first tranche against `doc/api/`, two of them, `is_receptive` and `file_extension`, appeared nowhere on their page. A code owner asked for exactly this on [!3045](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3045) rather than opening it himself, so offering it unprompted saves a round trip.

**Check before assuming.** Read the page from `gitlab-org/gitlab` (project `278964`) and grep for the field. Do not trust the page's title to name the field's home: the service account page is `doc/api/service_accounts.md`, not `user_service_accounts.md`, and a wrong path answers 404 rather than "absent".

**Where the change goes.** Branch on the community fork `gitlab-community/gitlab-org/gitlab` (project `41372369`, default branch `master`, developer access), and open the merge request against `gitlab-org/gitlab` `master` with `target_project_id`. One file, one commit, `remove_source_branch` and `allow_collaboration` both on.

Four rules the two merge requests sent so far were shaped by.

1. **Every example, not the one that prompted it.** `doc/api/secure_files.md` had four example responses and all four predated the field. Fixing one and leaving three is a worse page than before, because now they disagree.
2. **The attribute table counts as documentation.** `doc/api/cluster_agents.md` documents its responses twice, in a table and in an example, and a field missing from the table is missing whichever the reader trusts. That page needed three tables and four examples.
3. **Other entities share the page.** The cluster agents page also documents agent tokens and receptive URL configurations, which are different Grape entities and must not gain the field. Identify the right objects by something structural (the agent examples are the ones carrying `config_project`), never by position.
4. **Put the field where the entity exposes it, and keep the table aligned.** Read the order off `gitlab-api-live.json`. In a table, keep the new cell no wider than the column already is, or every other row has to be re-padded and a three-line change becomes a whole-table diff.

**Two traps in that record, both of which have already produced a wrong published claim.** Its per-field gate key is `conditions`, a **list**, not `condition`: reading it as `condition` reports every field as unconditional and never errors, which is how upstream issue 2300 came to claim that all six `API::Entities::ServiceAccount` fields are unconditional when `unconfirmed_email` is gated on `unconfirmed_email.present?`. And the record is a pin taken at one release, so it lags master and will not know a field or route added since. Before writing a conditionality claim into a public description, confirm it against GitLab's current Ruby, and say "sent only when …" rather than nothing when a field is gated: a documented field a reader cannot find in a response is a bug report waiting to be filed.

Validate before pushing: parse every ` ```json ` block on the page and assert the field is present in exactly the objects that should have it and absent from the rest. A trailing comma left behind by hand-editing is invisible in review and breaks the example for anyone who copies it.

**A new merge request there reports empty for a while.** `prepared_at` is null until a background job fetches the fork ref and builds the diff, and until then the API answers 0 commits and 0 changed files. That is preparation lag on a repository that size, not a failed push: confirm the work from the branch tip on the fork instead.

Finally, link both ways: a note on the client-go merge request naming the documentation one, and the client-go merge request named in the documentation description as where the gap was found.

## Keep issue 2300 current

The umbrella issue carries a table of what has been sent, and a table that lags is worse than no table: a maintainer reading "open" about something merged a week ago learns that nothing here is maintained. Update it at both moments, not only at the end.

**When the merge request is published**, add its row: the merge request link, the struct, the field or fields, and `open`. A merge request that exists and is not in the table is invisible to anyone reading the issue.

**When it merges**, change the row to say which release carries it, and read that from the repository rather than assuming. A merge and a tag are different events: `!3040` was merged and in no tag for hours, and saying "released in v3.1.0" because that was the newest tag would have been false. Ask which tags contain the merge commit:

```bash
curl -s --header "PRIVATE-TOKEN: $TOKEN" \
  "https://gitlab.com/api/v4/projects/gitlab-org%2Fapi%2Fclient-go/repository/commits/<merge_commit_sha>/refs?type=tag"
```

An empty answer means merged and unreleased, which the row should say in those words.

Update the issue by rewriting its description through the API (`PUT /projects/:id/issues/2300`), not by commenting: the table is the document, and a correction buried in a comment thread leaves the wrong table at the top.

**Read, modify, write. Never PUT a locally held copy.** Contributor success prepends an `<!--IssueSummary start-->` block to the description shortly after the issue is created, and it is not visible in anything written here. A blind PUT of the local file deletes it. Fetch the description, replace the exact region being changed, refuse to write if the anchor does not appear exactly once, and check the block survived in the response. The first correction made from here did wipe it, and the only reason nothing was lost is that the automation put it back.

Re-read the state of every merge request in the table while doing this, not just the one that prompted the update. Two of them changed state during the single session that opened the issue.

**Nothing in the issue sidebar is ours to set, and the labels arrive on their own.** Labels, assignees, milestone, weight, iteration, dates and health status are all dropped silently for a non-member exactly as `reviewer_ids` is on a merge request: the PUT answers 200 and `labels` comes back empty. There is no issue equivalent of the merge request escape hatch either, since `triage/processor/community/` holds `command_mr_label.rb` and no `command_issue_label.rb`; `command_issue_help.rb` is the only issue command.

Do not reach for the "Label this issue" link in the `IssueSummary` block either, at least not first. An `untriaged-issues` triage policy runs a model over the issue and labels it about fifteen minutes after creation: issue 2300 was opened at 14:08 and had `type::maintenance`, `backend` and `automation:ml` at 14:23, with no action from anybody. It publishes its confidence per label and applies only the high ones, so it took type at 80 and backend at 75 and declined `group::source code` at 40, `Category:Source Code Management` at 35 and `maintenance::refactor` at 55, which is the right call: the first two would have routed a Go client library issue to the source code management team. **Wait for it, then correct only what it got wrong.** Expect `ai-quick-win-labeller-gitlab-org` to comment "Could not start processing due to this error: You have insufficient permissions"; that is a failure on their side and costs nothing but the `quick win` label.

Subscription needs nothing: the author is subscribed on creation.

## What actually moves a merge request

The merge depends on one of four code owners deciding to look, and nothing above makes that decision happen. Ready state does not predict a merge (one sampled request was ready within half an hour and merged 54.7 days later), the `type::` label does not gate one, and the pipeline is already green. Ranked honestly:

1. **The one lever**: `@gitlab-bot ready @<code owner>`. It is the only route a non-member has to the reviewer field.
2. **Prevents a second wait, does not shorten the first**: a diff that satisfies the review rubric on sight. A round trip costs another wait of the same length as the first, not a comment.
3. **Bookkeeping, cheap enough to do**: title prefix, description headings, `type::` label. They decide what the release looks like once the merge happens, not whether it happens.
4. **Worth nothing**: re-posting commands, requesting Duo, chasing the failing `danger-review` and `autolabels` jobs, or opening more merge requests hoping to raise the odds. Several open at once divides the same scarce attention rather than multiplying it.

## After the merge

```bash
go get gitlab.com/gitlab-org/api/client-go/v3@latest
go mod tidy
```

Then retire the workaround this project carried for the gap (a captured-response read under [ADR-0021](../../../docs/development/adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md), or a request built by hand), mark the `docs/development/upstream-bugs.md` entry merged with the version that carries it, and update the row in issue 2300 as "Keep issue 2300 current" describes.

Wait before bumping: several merge requests land across several releases, so batch the version bump rather than raising the pin per merge.

## Validation checklist

- [ ] No open upstream merge request already covers this
- [ ] Branch pushed to the community fork, merge request opened against project 65271576
- [ ] Conventional prefix **with the API file as scope** on the title, and a prefix on every commit
- [ ] Description uses the project's three `##` headings, names this MCP server as where the gap was found, and links it
- [ ] Description ends with `Related to https://gitlab.com/gitlab-org/api/client-go/-/issues/2300` on its own line, never `Closes`
- [ ] Row added to issue 2300's table once the merge request is published, and corrected with the release once it merges
- [ ] No reviewer requested by hand, `@GitLabDuo` included: that one is permanent once made
- [ ] Field placed in API-documentation order, `int64`/`any`, `PathEscape` where a path parameter is interpolated
- [ ] Every public symbol carries a name-first comment ending in a `// GitLab API docs:` link
- [ ] Test asserts the new field, starts with `t.Parallel()`, uses testify
- [ ] `make reviewable` passes
- [ ] `type::` label asked for by comment, then `@gitlab-bot ready @<code owner>`
- [ ] `doc/api/` checked for the same field, and a documentation merge request opened against `gitlab-org/gitlab` when it is missing, covering every example **and** every attribute table on the page
- [ ] The two merge requests link to each other
