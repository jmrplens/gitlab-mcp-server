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

Title: `feat(topics): add OrganizationID to the Topic struct` — the API file is the scope. `feat` for a new field or endpoint, `fix` for a wrong tag or a bug.

The release-notes generator strips the prefix and **omits a non-conventional title entirely**, so an unprefixed title merges and then never appears in the changelog. Of 279 changelog entries, none differs from its merge request title.

`AddingAPISupport.md` step 9 asks for the prefix on **every commit**, not only the merge request title.

Never use a `(no-release)` scope, and never put `[delay release]` in the description: the first marks work that deliberately cuts no version, the second delays the release job.

### 4. Use the project's description headings

"What does this MR do?", "Is this a breaking change?", "How was this tested?". The repository has issue templates and no merge request template, so this is convention read off merged requests rather than something enforced.

Say the gap was found while developing this MCP server and link the repository: the backlink is the point of contributing from here.

Reference a related issue **only** if it already carries a `type::` label, and only at the start of a line with one of `related to`, `relates to`, `relate to`, `contributes to`, `contribute to`, `closes`, `close`, `see`. GitLab's `apply_labels_from_related_issue.rb` copies the first `type::` label off the issue and re-fires when the description is edited. `fixes`, `resolves` and `updates` are **not** in that list, and merge requests using them merged with no `type::` label ever.

### 5. Do not set fields the account cannot set

`MergeRequests::BaseService#filter_reviewer` deletes `reviewer_ids` **silently** unless the caller can administer the merge request, at creation and after. The same holds for assignee, labels and milestone: a non-member account has `adminMergeRequest: false` and `createLabel: false`. Setting them through the API looks like it worked and changes nothing. Section "Moving an open merge request" below is how those fields are actually reached.

Do not open as a draft; the ready command issues `/ready` regardless.

Do not request `@GitLabDuo` yourself. That request is what produces the `DCR4003` warning ("you don't have permission to create a pipeline for Code Review Flow"), and it is not the review that counts: on merged requests `gitlab-bot` requests Duo after the ready command and Duo then posts a real review.

Do not write a `Changelog:` trailer and do not touch `CHANGELOG.md`: semantic-release writes it.

## Code rules

From `.gitlab/duo/mr-review-instructions.yaml`, which is what the reviewer checks against:

- `int64` never `int`, and `any` never `interface{}` — inside slices and maps too.
- Pointer structs for requests, non-pointer for responses.
- `PathEscape()` on path parameters, `url.PathEscape()` on query parameters.
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

1. **Fix the title** if it has no conventional prefix. Title and description are the two fields the author may still edit.
2. **Ask for the label**: post `@gitlab-bot label ~"type::feature"` (or `~"type::bug"`) as a comment, with the command at the start of its own line. `command_mr_label.rb` accepts it from the resource author, and its allowed scopes include `type`. The rate limit is the module default of 60 per hour keyed on the actor.
   Pick by what should ship: `type::feature` cuts a minor, `type::bug` a patch, `type::maintenance` cuts nothing. There is no urgency — an unlabelled merge request still merges and still ships under "Other Changes", and the analyzer reads labels through the API at release time, so a label added later still counts.
3. **Ask for review, naming a code owner**: post `@gitlab-bot ready @<code owner>`. `command_mr_request_review.rb` reacts to the author's own note and emits `/ready`, the `workflow::ready for review` label and `/request_review`; with arguments it requests exactly those users. This is the only mechanism by which a non-member gets a chosen name onto the reviewer field. Posted **bare** it picks a random GitLab-wide merge request coach, who is usually not one of the four code owners.
   One ready command per merge request per hour: the limit is 1 for a non-member and the cache key is the actor **plus** the merge request path, so several merge requests can be readied within the same hour and a misfire costs an hour on that one only.
4. **Then stop.** One approval is required and this account cannot approve; the fork pipeline is already green and needs no maintainer to start it. Do not re-post `ready` if nothing happens — it is rate-limited and would only re-request the same person, and `gitlab-bot` nudges an unattended merge request on its own.

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

Then retire the workaround this project carried for the gap (a captured-response read under [ADR-0021](../../docs/development/adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md), or a request built by hand) and mark the `docs/development/upstream-bugs.md` entry merged with the version that carries it.

## Validation checklist

- [ ] No open upstream merge request already covers this
- [ ] Branch pushed to the community fork, merge request opened against project 65271576
- [ ] Conventional prefix on the title **and** on every commit
- [ ] Description uses the project's three headings, names this MCP server as where the gap was found, and links it
- [ ] Field placed in API-documentation order, `int64`/`any`, `PathEscape` where a path parameter is interpolated
- [ ] Every public symbol carries a name-first comment ending in a `// GitLab API docs:` link
- [ ] Test asserts the new field, starts with `t.Parallel()`, uses testify
- [ ] `make reviewable` passes
- [ ] `type::` label asked for by comment, then `@gitlab-bot ready @<code owner>`
