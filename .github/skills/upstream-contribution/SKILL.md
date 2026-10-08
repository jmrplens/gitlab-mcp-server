---
name: upstream-contribution
description: "Contribute fixes, missing fields or documentation to GitLab's own projects on gitlab.com: gitlab-org/api/client-go, gitlab-org/gitlab and any other gitlab-org project, from the community forks. Use when: an API gap, client-go bug, GitLab bug or documentation gap is found; when preparing, opening, readying or answering review on a gitlab.com merge request; when working in a GitLab Hackathon."
---

# Upstream contribution: GitLab projects on gitlab.com

Contribute bug fixes, missing endpoint wrappers, missing struct fields, API fixes and documentation corrections to the GitLab projects this server depends on or documents against: `gitlab-org/api/client-go` (the library), `gitlab-org/gitlab` (the monolith: the API, its OpenAPI and GraphQL documents, and `doc/`), and any other `gitlab-org` project a finding leads to.

Every rule below was read off the upstream project's own configuration, its triage automation, its merged history, or the 65 merge requests this account (`jmrp` on gitlab.com) opened in gitlab-org projects up to 2026-10-08 together with every discussion on them. The source is named beside each rule, and a count such as "(14 MRs)" says how often reviewers or bots raised it. Where two readings disagreed, the one that survived an adversarial pass is the one written here.

## Ground rules for every write

These hold on every gitlab-org project, before anything else in this file.

1. **Ask first.** Nothing is written on gitlab.com or to a fork without the maintainer's yes: no push, no merge request, no comment, no reply, no ready or label command, no applied suggestion, no issue edit. Read everything, lay out the state and the proposed writes as a list, and wait. A push to a community-fork branch counts, because it changes a merge request other people are reviewing. A subagent doing this work keeps branches local and posts nothing.
2. **A second, independent review before anything leaves.** An Opus agent that did not write the artefact reviews the exact diff or text, with the thread it answers and this file, and is briefed to refute it: wrong facts, overclaims, tone, attribution lines, bare short references, em dashes, the target project's rules. Fix what survives, review again, then publish.
3. **Before each approved write, check it is not already done and copy how it was done before.** A bot may already have requested the same reviewer, someone may have restarted the pipeline. Match the form of jmrp's own earlier notes on sibling merge requests.
4. **Pace.** A recent upstream merge request gets at least one more day after its last activity before a nudge is even proposed. Do not fire a round of ready commands in one sitting (see "Batch, do not drip"). A finished local fix is opened even when the project already has several of ours open (maintainer, 2026-10-08); only a real dependency (same files, a promised order) still waits.
5. **Never retry a pipeline** on gitlab.com or in a community fork. Classify the failure once (infrastructure versus a job that reached `step_script`), fix what is ours, report the rest. Maintainers start the canonical pipeline when they review (they said so on [gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699), [!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996), [!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)).
6. **Texts.** English, first person as the maintainer, plain and specific, no em dash, no AI attribution of any kind, no session or tool links. A merge request description is one paragraph per line with no hard wrap; a commit body is the opposite and wraps at 72 (see "Commit messages"). On gitlab.com use native references inside a description or a note; never put a reference in a commit message.
7. **Long descriptions** keep a short visible summary and put the per-commit list, evidence and testing in `<details><summary>...</summary>` blocks with a blank line after `<summary>`. Say the work comes from maintaining [gitlab-mcp-server](https://github.com/jmrplens/gitlab-mcp-server), and link [upstream-bugs.md](https://github.com/jmrplens/gitlab-mcp-server/blob/main/docs/development/upstream-bugs.md) as the place the project records everything done on its dependencies and sibling projects.
8. **GitLab Duo is a bot.** A note in a Duo thread is written for the human reviewers: impersonal, what holds, what changed and why, no thanks, no addressing the bot. Resolving it stays with the human reviewer.
9. **The gitlab.com username is `jmrp`, not `jmrplens`.** An `author_username` query with the wrong one answers `[]`, which reads as "no merge requests".
10. **An upstream finding is answered with the fix where it belongs**, a merge request upstream and, if our code adapts, a pull request here; never with an issue in this repository about somebody else's code.

## The GitLab Hackathon

The October 2026 Hackathon counts merge requests **opened from 2026-10-06 through 2026-10-12 and merged before 2026-11-12** (contributor platform `contributors/app/models/hackathon.rb`, terms version `2026-10-v1`; the leaderboard query compares `merged_date < '2026-11-12'`, so a merge on the 12th itself does not count), and the `Hackathon` label is applied by `triage-ops` to community merge requests created from 2026-10-06T00:00Z to before 2026-10-13T00:00Z (`hackathon_label.rb`; 11 of ours carry it). The maintainer has accepted the rules; without that a contribution scores nothing.

**The rules (terms 2026-10-v1).** Breaking any of them disqualifies without warning.

1. At most 20 merge requests and 20 issues opened per day.
2. **No merge request for an issue unless jmrp is assigned to it first.** The maintainer self-assigns through the contributor platform's issue page, `https://contributors.gitlab.com/manage-issue?projectId=<project id>&issueIid=<iid>` (the platform's `assign` action posts `/assign @jmrp` in a thread on the issue and pings the people it lists), or asks in the issue and waits. Read the issue's assignees before preparing anything. An issue assigned to someone else is out.
3. AI tools are allowed, but every contribution is the maintainer's own: review the code and test it locally before opening.
4. At most one typo-fix merge request per project during the hackathon. Do not propose typo fixes.
5. Issues labelled `quick win::first-time contributor` are reserved for people with no merged merge request in any GitLab project. jmrp has merged ones, so those issues are out. Other `quick win` issues are fine.

**How it scores** (`hackathon_leaderboard_service.rb`, `hackathon_leaderboard_query.rb` and `setting.rb`). A merge request earns nothing until it merges: each merged one is worth 80 points, 110 when it carries the `linked-issue` label, plus 20 for each distinct merged merge request containing your commits. A user with no merged merge request in the window, or who has not accepted the rules, scores 0; once one has merged, the platform's other activity points for the same week (issues, notes and the like) are added. Only merge requests under `gitlab-org`, `gitlab-com`, `gitlab-community` and `components` count. So a hackathon merge request is worth opening only when it can merge before 2026-11-12, and linking an issue is what earns the extra 30: an issue the merge request resolves needs the assignment first (rule 2), while one cited only as related context does not.

**What that means in practice.** Prefer changes a reviewer can approve on sight (the median time to merge for ours was 6.0 days in gitlab-org/gitlab and 1.3 days in client-go). Use the issue's own reference (`Closes #N` for an issue it fixes, `Related to #N` otherwise) so the platform applies `linked-issue`. Every merge request, the ones already open included, cites the issues genuinely related to it even when it does not close them, as context a reviewer benefits from: at most three, one line each on how it relates, never padded with an issue that is only tangential, and never with a closing keyword for an issue it does not resolve. Keep the daily count and the one-typo rule in a list before opening anything.

## Before starting

1. Identify the gap with evidence from GitLab's own source or documentation, and verify it on the project's current default branch (`master` for gitlab-org/gitlab, `main` for client-go). Re-verify right before opening: a fix may have landed meanwhile.
2. **Search the target project's open merge requests and issues for the same struct, field, route, file or behaviour, from any author.** If one already covers it, do not open a second: read it, judge whether it is complete (every key the entity exposes, the options struct where the endpoint accepts the parameter, a test that actually decodes the field), and if something is missing say so in a comment on that merge request with the same evidence a new one would carry. A duplicate costs a maintainer more than it saves.
3. Check our own open merge requests for the same files or topic. Two of ours that touch adjacent lines conflict, and whichever merges second must rebase, which resets its approvals ([gitlab-org/gitlab!260353](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260353) and [!260471](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260471) did, in `doc/user/permissions.md`). Say so in both descriptions.
4. For the Hackathon, assign the issue first (rule 2 above).
5. Record the finding in `docs/development/upstream-bugs.md`, the permanent register of upstream defects and gaps found from this codebase: follow its entry schema (Reported, In review, Merged with the version, Blocking, Workaround), link the tracker item with a full URL rather than a bare short reference (the repository is mirrored between GitHub and GitLab), and keep the entry when the fix lands.

## Projects, forks and branches

| Target | Project id | Default branch | Push to | Fork id |
| ------ | ---------- | -------------- | ------- | ------- |
| `gitlab-org/gitlab` | 278964 | `master` | `gitlab-community/gitlab-org/gitlab` | 41372369 |
| `gitlab-org/api/client-go` | 65271576 | `main` | `gitlab-community/gitlab-org/api/client-go` | 65275361 |
| `gitlab-org/orbit/knowledge-graph` | 77960826 | `main` | `gitlab-community/gitlab-org/orbit/knowledge-graph` | 82768768 |
| `gitlab-org/cells/http-router` | 54957577 | `main` | personal fork `jmrp/http-router` (no community fork exists) | 86845981 |

- **Push to the community fork, never to a personal one, when a community fork exists.** `CONTRIBUTING.md` of client-go recommends it, and gitlab-bot's thank-you note adds "Did you know about our community forks?" to every merge request opened from a personal fork (10 of ours: [client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996), [!3006](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3006), [!3033](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3033) to [!3039](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3039), and [http-router!1354](https://gitlab.com/gitlab-org/cells/http-router/-/merge_requests/1354), where no community fork exists, so the personal fork is the route). The pipeline runs either way, so CI is not the reason.
- **Branch names are `jmrp-<topic>`**, because the forks are shared by many contributors.
- **Push through the one-shot credential helper**, never with the token in a URL or in argv: `git -c credential.https://gitlab.com.helper= -c credential.https://gitlab.com.helper='!<helper>' push --force-with-lease ...`. The fork answers "cannot lock references" for minutes at a time; retry that error only.
- **Open the merge request from the fork's project id with `target_project_id`**, `squash: true`, `remove_source_branch: true`, `allow_collaboration: true`. The maintainer's publisher script (`publish-gitlab-mr.sh <texts-dir> <worktree> <push-remote> <fork-id> <target-id> <target-branch>`, kept with his local tooling beside the credential helper rather than in this repository) does exactly this: it refuses an unsigned head, pushes with a lease, opens the merge request once, waits up to ten minutes for `/diffs` to list files, then posts `ready-note.md` once, and every step is idempotent. The texts directory holds `title.txt`, `description.md` and `ready-note.md`.
- **A merge request's source project and branch cannot be changed after it is opened**: the update endpoint accepts neither and answers with the list of what it does accept. Moving one means opening a replacement from the right fork with the same title and description and closing the original with a note naming it (client-go [!3033](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3033) to [!3039](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3039) became [!3040](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3040) to [!3046](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3046)).
- **Commits are signed with the global identity.** Never pass `-c user.name`, `-c user.email` or `--author`.

## Commit messages (every GitLab-hosted repository)

On gitlab-org/gitlab, Danger lints every commit with the `commit_messages` rule of `gitlab-dangerfiles` (4.12.0 on master), and the rules are exact. client-go's `Dangerfile` does not import that rule and knowledge-graph has no `Dangerfile`, but write every commit to the same shape anyway, with one exception: client-go's conventional prefix wins over the capital-letter rule (see "client-go" below).

- Subject: at least 3 words, at most 72 characters, starts with a capital letter, no leading space, no trailing period. The capital-letter check skips one leading `word:` or `[tag]` prefix and reads the word after it.
- A blank line between subject and body.
- Body lines at most 72 characters; a URL on the line is not counted.
- No Markdown or Unicode emoji.
- **No short references** (`#123`, `!123`, `&123`, `group/project#123`, `%12.3`).
- A commit that touches more than 3 files and more than 30 lines must have a body.

Severity matters. Body line length, a short subject and a missing body only warn. On a squash merge request (both gitlab-org/gitlab and client-go default to squash), the other problems of the first multi-line commit **fail** `danger-review` and those of the other commits only warn; without squash they fail on every commit. A 73-character subject failed it on [gitlab-org/gitlab!254542](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254542) and short references failed it on [!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [!260353](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260353). The warning is still worth avoiding: on [gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699) body lines up to 79 columns drew Danger warnings on two commits, and fixing them once the merge request was approved would have meant a force-push that can reset the approvals, so they stayed.

**Write no references at all in a commit message.** A short reference fails Danger, and a full URL, which Danger accepts, makes GitLab add a "mentioned in commit" system note to the referenced merge request or issue on every push. Links go in the description.

Write the message into a file and check it before committing:

```bash
awk '{ if (length($0) > 72) printf "line %d is %d chars\n", NR, length($0) }' msg.txt
bundle exec ruby scripts/lint/commit_linter.rb msg.txt     # gitlab-org/gitlab; -m "<message>" also works
git commit --amend -F msg.txt
```

Name a long endpoint without its full path rather than letting a line run (`POST /projects/:id/merge_requests/:iid/cancel_merge_when_pipeline_succeeds` is 74 characters alone).

**Trailers (gitlab-org/gitlab).** A change users can observe carries `Changelog: <category>` (`fixed`, `added`, `changed`, ...) on the commit that should reach the changelog; Danger reports "CHANGELOG missing" otherwise (3 MRs). A change touching `ee/` adds `EE: true` to the commit that carries `Changelog:` (Danger warned on 3 MRs and a reviewer asked for it on [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936)), except when the merge request has database changes, where Danger warns against it. On [gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074) the merging maintainer (c_fons) offered to reword our message at merge time and kept `Changelog: fixed` in the version he proposed.

**client-go** additionally wants a conventional-commit prefix on every commit (`docs/guides/AddingAPISupport.md`: "Please make sure any commits you do have semantic commit prefixes"), and does not want a `Changelog:` trailer or a `CHANGELOG.md` edit: semantic-release writes it.

## Titles and descriptions

**gitlab-org/gitlab.** Use the project's templates. Code: `.gitlab/merge_request_templates/Default.md` ("What does this MR do and why?", "References", "Screenshots or screen recordings", "How to set up and validate locally", "MR acceptance checklist"). Documentation only: `Documentation.md` ("What does this MR do?", "Related issues", "Author's checklist", "Reviewer's checklist"). Delete the template's HTML comments (the one at the top of `Default.md` recommends putting the bracketed skip-CI hint in the title, which must never appear in anything we write) and its quick-action lines (`/label`, `/assign me`), which this account cannot apply. Title: a plain sentence of what changes, no prefix. Say what was verified and how (the reviewer of [gitlab-org/gitlab!254507](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254507) merged because "the link to ... secure_file.rb#L15 in the CLI MR helped make it very clear that this was an oversight", the link sitting in the paired client-go merge request).

**client-go.** See "client-go" below: conventional title with the file as scope, and the three headings "What does this MR do?", "Is this a breaking change?", "How was this tested?".

**knowledge-graph.** Conventional titles, as our three merged ones were (`docs(dsl): ...`, `fix(dsl): ...`).

**In every project**, link the issue with a line the automation reads, at the start of a line: `Closes #N` for an issue it fixes, `Related to <issue URL>` otherwise (`apply_labels_from_related_issue.rb` accepts `related to`, `relates to`, `relate to`, `contributes to`, `contribute to`, `closes`, `close`, `see`, and not `fixes`, `resolves` or `updates`). The contributor platform then applies `linked-issue`, which is what the hackathon's linked-issue points read.

## Opening and readying a merge request (all projects)

**Do not set fields the account cannot set.** `MergeRequests::BaseService#filter_reviewer` deletes `reviewer_ids` silently unless the caller can administer the merge request, at creation and after; the same holds for assignee, labels and milestone. A non-member has `adminMergeRequest: false` and `createLabel: false`, so the API call looks like it worked and changes nothing. The bot commands below are how those fields are actually reached.

**Do not open as a draft**; the ready command issues `/ready` regardless.

**Do not request `@GitLabDuo` yourself.** Every one of the 47 requests jmrp made (one per merge request) got the `DCR4003` warning ("you don't have permission to create a pipeline for Code Review Flow") and produced no review. The Duo reviews that did run on ours (nine merge requests) were each requested by a team member or by gitlab-bot. Removing a reviewer needs `reviewer_ids`, which is dropped for a non-member, so a Duo request made from this account stays on the merge request with its warning for good. Danger's "We advise getting a review from GitLab Duo Code. You can assign `@GitLabDuo`" (on all 33 of ours in gitlab-org/gitlab) is written for team members.

**Wait until the diff exists, then post the ready note.** Right after the API create the merge request is still preparing: `prepared_at` is null, `/diffs` and `/commits` are empty, and `changes_count` is 0. On the monolith this once took about four minutes ([gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074)). On [gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300) a ready posted at 09:55 UTC was answered "contains no changes", the delayed job set the label at 10:00, the merge request was back on `workflow::in dev` a minute later, and only a second ready at 10:04, with the diff in place, requested anyone. Confirm a push by the fork's branch head (`/projects/<fork>/repository/branches/<branch>`), not by the merge request's `sha`, which lags. Poll `/diffs` until it lists the files (the publisher does) before posting anything.

**The ready note.** Read off `triage-ops` `command_mr_request_review.rb` and `reactive_command.rb`:

```text
@gitlab-bot ready @reviewer1 @reviewer2
@gitlab-bot label ~"type::bug" ~backend ~"group::<group>"

One or two sentences for the reviewer, when there is something they need to know.
```

- The command reads its arguments from **its own line only**, and every `@word` on that line becomes a reviewer (`@([^@[[:blank:]]]+)`), punctuation included. So the first line holds the command and the names separated by spaces, nothing else.
- The label command is a separate command that matches at the start of any line, so it may follow on the second line.
- What ready does once the diff exists: `/ready`, the `workflow::ready for review` label, and a review request. With names, it requests those and keeps any reviewer already on the merge request. **With no names it re-requests the reviewers already on the merge request** (Duo excepted), and only if there are none does it pick a random merge request coach. On a merge request touching only `doc/` or `docs/`, with no names and no current reviewers, it skips the coach whenever CODEOWNERS names a technical writer for the page, and leaves the writer to `automated_review_request_doc.rb`, which does nothing once the merge request carries `Technical Writing` or any `tw::` label (the ML labeller often adds one first). That combination left [gitlab-org/gitlab!254538](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254538) and [!254552](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254552) ten days and [!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540) twelve days with `workflow::ready for review` and nobody on the reviewer field, until a ready naming the writer put the writer there within seconds.
- **A ready posted before the diff exists requests nobody.** The bot answers "this merge request currently contains no changes ... We will wait 5 minutes before assigning a reviewer" (10 of ours), and the delayed `MarkReadyForReviewJob` then only sets `/ready` and the label. A hygiene policy later notices "ready for review but no reviewer is assigned" and puts the merge request back to `workflow::in dev` ([gitlab-org/gitlab!260458](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260458) and [!260486](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260486)). The fix is a new ready, naming someone, once the diff exists.
- **Rate limit: one ready per hour for a non-member, keyed on the actor, the merge request and whether its diff was ready.** So several merge requests can be readied in the same hour, a ready posted before the diff exists does not block the first one after it, and a second ready on the same prepared merge request within the hour is ignored.
- `@gitlab-bot ready` answers inside the ready's own note whenever it requests someone, and also when it answers "contains no changes", so such a ready becomes a resolvable thread; a ready that skipped the coach on a documentation-only merge request gets no answer and stays an individual note.
- **Read the reviewer field after every ready**, not the workflow label, which is set either way. The field can also lose a reviewer afterwards: on [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936) the documentation automation took the coach a bare ready had requested off the field in the same step that added the page's writer, and on [!259765](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259765) the backend review request was dropped the same way when the documentation review was requested.

**Choosing whom to name.**

- client-go: one of the four in `.gitlab/CODEOWNERS` (`@timofurrer @patrickrice @fforster @heidi.berry`).
- A documentation-only change in gitlab-org/gitlab: the technical writer CODEOWNERS lists for the page, on the most specific line that matches it (`/doc/api/deploy_keys.md @rsarangadharan` is one).
- A backend change in gitlab-org/gitlab: the maintainer Danger's "Reviewer roulette suggestions" table names for the category (17 of ours carried one), or a reviewer from the group that owns the code, or someone involved in the related issue.
- Avoid anyone who already has several of ours waiting: the bot's review nudges say how loaded a reviewer is ("`<user>` currently has N active review requests"). Reviewers hand a merge request on when it is not theirs (about 38 hand-off notes on 25 of ours), so a wrong first name costs a day, not the merge request.

**The label command** (`command_mr_label.rb`, which despite its name reacts to issue notes too): `@gitlab-bot label` and `@gitlab-bot unlabel` accept the scopes `type::`, `group::`, `bug::`, `feature::`, `maintenance::`, `Category:` labels, and a fixed list that includes `backend`, `database`, `documentation`, `frontend`, `UX`, the `workflow::` states and `automation:ml wrong`; 60 per hour per actor. `type::` is scoped, so a new one replaces the old. Danger's "Labels missing" (26 of our 33 in gitlab-org/gitlab) lists `documentation`, `backend` and a `type::`, which this covers, and the `roulette-experiment::*` labels and the milestone (27 of 33), which only reviewers set: leave those to them. The TypeLabelNudger thread ("Please add ~"type::bug" ~"type::feature", or ~"type::maintenance"") stays unresolved after the label is set, and the author may resolve it; left open, it counts against `DISCUSSIONS_NOT_RESOLVED` like any other thread.

**Feature flags.** Danger warns "There were no new or modified feature flag YAML files" on a backend change (once on ours). The `feature flag::` labels are not in the command's list, so say in the description why the change needs no flag. A reviewer once asked whether a new field should be an experiment for a few milestones and then dropped it as "not a major change" ([gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936)).

## gitlab-org/gitlab: preparing the change

### The local environment

Contributions to gitlab-org/gitlab have never used the GDK. The environment is built by hand and is enough for API, request-spec and documentation changes:

- The clone is `/opt/gitlab-source/gitlab`, remotes `origin` (gitlab-org/gitlab) and `fork` (the community fork), with one worktree per merge request at `/opt/gitlab-source/wt-<slug>` on a branch `jmrp-<slug>`.
- **It is a shallow clone.** `git fetch origin` with no depth pulls the whole history and the host's memory guard kills it; use `git fetch --depth=1 origin master`. After such a fetch there is no common ancestor, so rebuild a branch with `git checkout -B <branch> origin/master` and `git cherry-pick <sha>` instead of `git rebase origin/master`. A `Gemfile.lock` that moved needs `bundle install`.
- Ruby from rbenv at the version `.ruby-version` names (3.3.11 on master in 2026-10), then `bundle install` in the worktree.
- Two containers: `gitlab-dev-postgres` (`postgres:17` on 127.0.0.1:5434) and `gitlab-dev-redis` (`redis:7.2-alpine` on 127.0.0.1:6390). **Each worktree gets its own test database** in a hand-written `config/database.yml` (`main` and `ci` both pointing at, for example, `gitlabhq_test_insufficient_scope`) and its own Redis database number in `config/resque.yml`, created with `RAILS_ENV=test bundle exec rake db:create db:schema:load`.
- **When several items run specs at once, wrap every rspec and database command in `flock <lock file> <command>`** on one shared lock file in the job's scratch directory, so two schema loads or two suites never meet. Worktrees and branches in the clone are created under a second lock for the same reason.
- The first rspec in a fresh worktree builds Gitaly (three git builds), Workhorse, the indexer, OpenBao and Zoekt into `tmp/tests`, about 20 minutes; it needs `meson`. Copying `tmp/tests` from a worktree that already built it saves that. Give the first run a 25-minute timeout and edit nothing it loads meanwhile; run long suites with a waiter rather than a foreground wait.
- A spec that goes through `ApplicationController` (GraphQL request specs) dies with `cannot load such file -- sass`; an empty `app/assets/builds/emoji_sprites.css` (gitignored, created before boot) stands in.
- Mixing a `fast_spec_helper` spec with `spec_helper` ones in one rspec call aborts unless run with `-r spec_helper`.
- Prove a fix by running the new spec with and without the change: it must fail on master and pass on the branch.

### Linters and generated files

- `bundle exec rubocop <changed Ruby files>`.
- The commit linter (`scripts/lint/commit_linter.rb`) and the Danger rules above.
- **Documentation**: markdownlint and Vale in the image the `docs-lint markdown` job uses, which is pulled on this host: `registry.gitlab.com/gitlab-org/technical-writing/docs-gitlab-com/lint-markdown:alpine-3.24-vale-3.21.0-markdownlint2-0.23.2-lychee-0.24.2-rumdl-0.2.69` (master's `.tool-versions` pins markdownlint-cli2 0.23.2, vale 3.21.0, lychee 0.24.2). Run it with no network on the worktree: `markdownlint-cli2 <page>` and `vale --minAlertLevel warning --output=line <page>`, and lint the page as the parent commit holds it too, so you report only what you introduced.
- **OpenAPI**: a change to a route's `desc`, `success`, `failure` or an entity's `expose` documentation moves `doc/api/openapi/openapi_v3.yaml`, and `static-analysis` fails with "OpenAPI documentation is outdated! Please update it by running `bin/rake gitlab:openapi:v3:generate`" ([gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699)). Regenerate and check with `gitlab:openapi:v3:check_docs`. The v2 document (`doc/api/openapi/openapi_v2.yaml`, with `gitlab:openapi:v2:generate` and `gitlab:openapi:v2:check_docs`) can move the same way, as it did on [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936); commit only the hunks your change produces.
- **GraphQL**: a field or description change needs the reference and both introspection files regenerated, or `graphql-verify` fails with "One or more GraphQL Introspection Schemas need to be updated! ... bundle exec rake gitlab:graphql:generate_all_introspection_schemas" ([gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936) and [!260459](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260459)). `RAILS_ENV=test bundle exec rake gitlab:graphql:update_all` runs the docs, the schema dump and both introspection schemas; `gitlab:graphql:check_docs` and `gitlab:graphql:check_introspection_sync` check them.
- **Never take a reviewer's wording suggestion on a generated file as written.** rsarangadharan left the same wording on the GraphQL reference, both introspection JSON files and the type on [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936); the change belongs in the source, followed by a regeneration.
- **Routes**: a new API route changes the HTTP Router snapshot and `cells-routes:router-in-sync` fails until a paired merge request to `gitlab-org/cells/http-router` refreshes `test/routes/gitlab_routes.json` ([gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [http-router!1354](https://gitlab.com/gitlab-org/cells/http-router/-/merge_requests/1354)), and the branch is rebased onto a master that has it.

### Documentation conventions, from the technical writers' reviews

The single most frequent request on our merge requests: 57 diff notes on `doc/` across 14 merge requests, 47 of them suggestion blocks. Most of it is the documentation style guide applied line by line, so apply it before the writer has to:

- **History items** name the milestone the change ships in, not the current one: "Updating to reflect the next milestone because 19.4 just ended" (uchandran, [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702)). A response attribute added to an existing endpoint gets its own line (eduardosanz wrote one himself on [gitlab-org/gitlab!259764](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259764)). Deprecations take no history block, only the warning alert (uchandran, [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)). Check the release date before writing the version, and move the line if the merge slips past the cut.
- Imperative first word (`Cancel`, not `Cancels`); "you", not "the user"; a less formal tone; no leading "The" in an attribute description; do not repeat the error message in the reason column; drop words like "often" and "usually" (uchandran, [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702), 6 notes).
- Keep the page's template order: the status table first, explanation below it (uchandran, [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)).
- Link text is lowercase, including API names; minimize links on a page and never link the same target twice (uchandran and Duo, [gitlab-org/gitlab!259766](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259766)).
- Do not explain tiers in an attribute description; "GitLab Enterprise Edition only" is enough, and the API answers the rest (uchandran, [gitlab-org/gitlab!259766](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259766)).
- A long list of values goes to a footnote under the table (z_painter, [gitlab-org/gitlab!260143](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260143)). Check a footnote suggestion's labels match before applying it: the one on [!260143](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260143) had `[^action-values]` against `[^action-value]` and no blank line before the next block (MD031).
- Say what a field is in the response, not on the object: "The attribute is on the response, not the token" (idurham, [gitlab-org/gitlab!260276](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260276) and [!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277)). Notes use `> [!note]` and end `For more information, see [...](...).`
- **Change every occurrence on the page, not only the one that prompted it.** Every example response, every attribute table, every endpoint section that documents the same field (brendan777 and Duo on [gitlab-org/gitlab!254552](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254552); narendran-kannan on [!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540), blocking, for eleven example values). Other entities on the same page must not gain the field; identify them structurally, never by position.
- **Keep a table edit to the rows it changes**; do not realign the columns (amsingh6, [gitlab-org/gitlab!254552](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254552), citing the style guide).
- Keep example data internally consistent (one hostname per example, a `last_used_at` before `expires_at`, an id the request actually asked for) and every JSON block parseable.
- Leave out what the page cannot yet promise: a GraphQL detail on a REST page, behaviour of an unreleased version, an endpoint that is still in review (z_painter, [gitlab-org/gitlab!260143](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260143); marc_shaw, [!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)). Link a section only once it exists on master.
- A documentation fix that reveals more pre-existing problems on the page: fix them in a follow-up rather than widening the merge request, and say so (narendran-kannan's list on [gitlab-org/gitlab!254540](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254540) became [!259375](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259375)).

### Spec conventions, from the reviews

16 notes on 7 merge requests asked for spec changes. The recurring ones:

- Tag every example with two or more expectations `:aggregate_failures` (kerrizor, [gitlab-org/gitlab!255300](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255300)).
- An EE extension spec describes the CE class (`RSpec.describe AppConfig::InstanceMetadata`), uses metadata tags such as `:saas_gitlab_com_subscriptions` instead of stubbing, prefers `it { is_expected.to eq(...) }`, and carries no redundant `let`, unneeded stub or stale comment (narendran-kannan, [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936), 7 notes).
- A GraphQL spec that compares `all_graphql_fields_for(...)` against an exact hash breaks when a field is added; update its expected data in the same change ([gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936), blocking).
- Cover both halves of a permission check (the author and the owner, `can_cancel_auto_merge?`), do not keep a duplicate example that passes only by accident, and do not depend on fresh id sequences in id-versus-iid examples (marc_shaw and egrieff, [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)).
- When a response is the published contract, pin it with a request spec so nobody "fixes" it later (marc_shaw, [gitlab-org/gitlab!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704)).
- When a change can affect a caller you did not touch, find the caller's spec and add the context that shows it (alipniagov supplied a context whose 12 examples failed on the branch, on [gitlab-org/gitlab!258074](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/258074) for `KnownSignIn`).
- No reviewer has asked us for an N+1 spec (0 of 65), but a route that presents more data per row gets a query-count spec before it is opened, as [gitlab-org/gitlab!259764](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259764), [!260276](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260276) and [!260277](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/260277) did.

### API changes

Reviewers scrutinised compatibility more than anything except documentation (18 notes on 7 merge requests):

- **Changing what an existing endpoint answers is a breaking change**, even when the old answer contradicts the documentation (phikai, [gitlab-org/gitlab!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239), with the API style guide's definition). The accepted path was a new endpoint with the documented contract plus a deprecation of the old one and documentation of what it really sends (marc_shaw, [!255239](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255239)), which became [!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702) and [!255704](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255704).
- Follow the family's status codes: a `POST` action on a merge request answers `201`; a refusal because of state is `409 Conflict` through `conflict!`, not `406`, which is content negotiation (marc_shaw, [gitlab-org/gitlab!255702](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/255702)).
- Exposing data to more users than before needs the owning group's product manager on the issue before merge (ck3g, blocking, [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936)).
- Keep one rule in one place: if a new value repeats a rule another method already encodes, extract a small method and call it in both (ck3g, [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936)).
- In `ee/`, prepend with `prepend_mod_with` on the last line, guard overrides with `override`, fall back to `super` (praised on [gitlab-org/gitlab!256936](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/256936)).

### Pipelines

- An MR from the community fork runs its pipeline **in the fork** (102 of the 155 failed job rows on ours ran in 41372369). Ask the merge request for `.head_pipeline.project_id` and read `/projects/<that id>/pipelines/<id>`; `head_pipeline.status` on the merge request lags by minutes.
- A `failed` pipeline with no failed jobs is a failed child pipeline: read `/pipelines/<id>/bridges`.
- Infrastructure failures die in `get_sources` before running anything ("remote: error resource exhausted", "bad pack header", "GitLab is currently unable to handle this request due to load"), whatever the job's name. Real failures reached `step_script`. Report the first kind once; fix the second.
- `pre-merge-checks` failing ("Expected latest pipeline ... to be successful") is a merge-time check the maintainer runs; it is not ours to fix (5 of ours).
- The canonical (merged results) pipeline needs a maintainer; gitlab-bot explains the pipeline tiers to the maintainer who will set auto-merge.

### Pushing to a merge request under review

- **A push resets approvals**: 14 "reset approvals from @X by pushing to the branch" events on 9 of ours. Whether a project resets on push is not readable by this account (403), so read `approved_by` after the push before saying anything about it.
- **A push cancels an automatic add to the merge train** ("aborted automatic add to merge train because the source branch was updated", 4 of ours). Before pushing to an approved merge request, read its system notes for "enabled automatic add to merge train" or "set to auto-merge", and tell the maintainer first.
- After such a push, the ready note asks the reviewer to approve again, start a new pipeline and set the automatic add again, and says exactly what changed since their approval.
- A rebase the reviewer did not ask for buys nothing on `merge_method=merge` and may cost an approval; rebase when a reviewer asks (5 such requests) or a job needs it.

## client-go

### Upstream project

- **Repository**: <https://gitlab.com/gitlab-org/api/client-go> (project id 65271576), community fork <https://gitlab.com/gitlab-community/gitlab-org/api/client-go> (65275361).
- **Language**: Go. **Branch model**: `main`.
- **Reviewers**: `.gitlab/CODEOWNERS` is one line naming four people: `@timofurrer @patrickrice @fforster @heidi.berry`.
- **Style**: `.gitlab/duo/mr-review-instructions.yaml` is the reviewer's actual checklist; `AGENTS.md`, `CONTRIBUTING.md` (at the repository root) and `docs/guides/AddingAPISupport.md` carry the rest. `CONTRIBUTING.md` asks for no issue before a merge request, and says the project only supports what is in the public API documentation.

### Opening a merge request

1. **Target `main`.** Target a `release-client-N.0` branch only when the change breaks the public API, and then add an entry to that release's migration document (PatrickRice on [client-go!2996](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/2996)). A new struct field or a corrected json tag is not breaking ([client-go!3046](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3046): "treating this as a `bug` is fine since there's no compile-time breaking change"). Removing or re-typing a struct field or an attribute is breaking (`.agents/skills/quick-win-instructions/SKILL.md`).
2. **Write a conventional-commit title, and prefix every commit.** Title: `feat(topics): add OrganizationID to the Topic struct`. `feat` for a new field or endpoint, `fix` for a wrong tag or a bug. **The scope is the API file, and it is expected even though conventional-commit does not require one**: `fix: correct the json tag` parses and reaches the release notes, but the merged history is overwhelmingly file-scoped (`fix(group_boards)`, `fix(feature_flags)`, `fix(issues)`). The release-notes generator strips the prefix and **omits a non-conventional title entirely**, so an unprefixed title merges and then never appears in the changelog; of 279 changelog entries, none differs from its merge request title. Never use a `(no-release)` scope, and never put `[delay release]` in the description.
3. **Use the project's description headings**, three `##` headings in this order: "What does this MR do?", "Is this a breaking change?", "How was this tested?". The repository has issue templates and no merge request template, so this is convention, which is why it gets skipped (seven of ours carried ad-hoc headings until they were rewritten). Answer the breaking-change heading honestly rather than with a flat "no": correcting a json tag is source-compatible and still changes the value a caller decodes. Say the gap was found while developing this MCP server and link the repository.
4. **While issue 2300 is open, reference it in the opening line** of "What does this MR do?", and carry the machine-readable form on a line of its own at the end:

   ```text
   Related to https://gitlab.com/gitlab-org/api/client-go/-/issues/2300
   ```

   Issue 2300 is the umbrella: it publishes the method, the per-struct lists of fields GitLab sends that the library does not model, and the table of what has been sent. `Related to` survives the merge; the one exception is [client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063), the single merge request the maintainers asked for, which ends with `Closes #2300`. After it merges, a field found later is an ordinary on-demand contribution with no issue needed. Putting the reference only at the bottom is what went wrong on 2026-09-09: a code owner asked on [client-go!3053](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3053) for exactly the issue every one of those merge requests linked.

Two separate mechanisms read the reference line:

- **`apply_labels_from_related_issue.rb`** copies the first `type::` label off the referenced issue onto a merge request that carries none (and its section, stage and group labels where the merge request has none), and re-fires whenever the description is edited. Issue 2300 carries `type::maintenance` (the ML labeller put it there fifteen minutes after it was opened), so every merge request referencing it is labelled `type::maintenance` at creation, which cuts no release. Replace it with `@gitlab-bot label ~"type::feature"` (or `~"type::bug"`); `type::` is scoped, so the new one replaces the copied one, and the script never re-fires over a merge request that already carries a `type::` label. Pick by what should ship: `type::feature` cuts a minor, `type::bug` a patch, `type::maintenance` nothing. An unlabelled merge request still merges and ships under "Other Changes", and the release reads labels at release time, so a label added later still counts.
- **The contributor platform** applies `linked-issue` when the merge request's `linked_work_items` is non-empty; a `MENTIONED` link is enough. One umbrella issue referenced from every merge request earns it as well as a throwaway issue per merge request would, and reads as evidence rather than noise.

Issue 2300 is ours, opened to answer a question their own [issue 2269](https://gitlab.com/gitlab-org/api/client-go/-/issues/2269) left open. What the maintainers agreed to in it is one merge request, then fields on demand, and no list. Quote that when a description needs it, and do not describe the issue's tables or method as anything a maintainer endorsed.

### Batch, do not drip

**One merge request per struct is the wrong shape, and the maintainers said so.** PatrickRice on [client-go!3053](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3053): "Instead of creating a bunch of individual MRs to add fields to structs ... would you mind creating an issue with the missing fields and updating that until you hit a good point where you're ready to create some MRs that may be a bit larger? I get the impression a lot of these MRs are LLM generated which isn't a big deal but with so many coming in to a team of 3 maintainers, it puts a bit of a burden on us to validate each MR individually ..."

What they settled on in issue 2300 went further: PatrickRice proposed on 2026-09-09 ([note 3811110919](https://gitlab.com/gitlab-org/api/client-go/-/issues/2300#note_3811110919)) a single merge request for every addition and struct change this server needs, then fields on demand with no list kept; timofurrer agreed the next day. That merge request is [client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063). **But batching has a limit:** library-level changes go in merge requests of their own. On [!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) PatrickRice asked: "Can we please separate items 1 and 2 out into separate MRs for each? ... I know we'd initially discussed larger MRs, but that was mostly in regards to syncing fields, whereas these are more fundamental changes at the library level", which became [client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065) and [!3066](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3066). Field syncs batch; behaviour changes split.

**What actually stung was the burst, not the count.** The same maintainer said there was no reason to close the ones already in front of reviewers and that he was reacting to a second large set of pings on top of an overnight one. So the rule is about the rate at which you demand attention: batch the work, and do not fire a round of ready commands in one sitting. Offering to withdraw work already under review was the wrong instinct and was declined.

**Say plainly how the work is produced** when asked, and what that does and does not guarantee: the entity, line and condition are read out of a booted GitLab rather than recalled, and each is checked against current `master`, which narrows what a reviewer must distrust without pretending it removes the review.

**A change that could break callers ships behind an option** with a plan for the next majors (PatrickRice on [client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063): "we have to implement it as a feature toggle with a note saying that in v4 the toggle would become the default, and with v5 the old toggle would be removed"). Deprecated GitLab routes keep their methods ("Deprecated routes live for a _long_ time in GitLab", same review). A method whose options are missing gets a `WithOptions` sibling until the next major folds them in (agreed there, with v4 deprecating the sibling and v5 removing it).

### Code rules

From `.gitlab/duo/mr-review-instructions.yaml`, which is what the reviewer checks against:

- `int64` never `int`, and `any` never `interface{}`, inside slices and maps too.
- Pointer structs for requests, non-pointer for responses. **Response structs use primitives unless `nil` means something** (PatrickRice on [client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048), [!3051](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3051) and [!3052](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3052): "usually we make the 'response' struct use primatives unless the `nil` value means something"; "this should likely be a primitive slice instead of a pointer slice").
- `PathEscape()` on every path parameter interpolated into a route. The same line of the instructions names `url.PathEscape()` for query parameters too; prefer `url.Values.Encode()` for query values, or `url.QueryEscape()` for a single value, since `url.PathEscape` leaves a `+` alone and form-style decoding then reads it as a space.
- Project and group ids typed `any`, parsed with `parseID()`.
- lowerCamelCase locals.
- "GitLab", never "Gitlab" or "gitlab", in comments, log messages, test names and any other text.
- Put a new struct field where the API documentation puts it, **not** at the end of the struct.
- Mark an experimental field with a disclaimer comment (heidi.berry on [client-go!3041](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3041): "we like to have disclaimers on experimental fields/functions in case they need to change").
- Name a helper for what its call site asks: one that is only ever called negated gets the positive name (`isNonNilOptions` rather than `!isNilOptions`), and the branch an option controls carries a comment naming the option (PatrickRice on [client-go!3065](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3065)).

From `AGENTS.md`, the most repeated demand in the project's own documents: every public function, type and method carries a comment beginning with its own name, in the present tense, wrapped under 80 characters, ending in a `// GitLab API docs: <url>` line. `CONTRIBUTING.md`: format with `gofumpt`, comments under 80 columns, code under 100 where sensible.

Tests:

- Assert the new field specifically, in every method that sends or decodes it (ck3g on [client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048): "Can we add the same check to `TestSystemHooksService_EditHook`?").
- Start with `t.Parallel()`, use `setup(t)`, `testMethod(t, r, ...)` and testify `assert`/`require`; never `reflect.DeepEqual`.
- Inline JSON for a small response, `mustWriteHTTPResponse(t, w, "testdata/...")` for a large one.
- GIVEN/WHEN/THEN comments are for complex tests and may be omitted.
- A GraphQL mutation's error handling is shown with integration tests of a success and a refusal (PatrickRice on [client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)).

Run `make reviewable` (setup, generate, fmt, lint, test) before opening, and regenerate mocks whenever a signature changes.

Expect `danger-review` and `autolabels` to fail in the fork's pipeline with `secrets_provider_not_found`: both are `allow_failure: true` and the pipeline still reads green. `autolabels` is the job that would have derived the `type::` label from the conventional prefix, so the label has to be asked for by comment. Never mention those two as a problem to fix.

**A suggestion applied as written can break the build.** On [client-go!3041](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3041) the reviewer's suggestion carried four spaces where gofmt puts a tab, and the commit applying it failed `verify-generated-code` and `golangci-lint`; on [!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048) the commit implementing a suggestion failed the same two jobs. After applying or implementing any suggestion, pull the branch, run `gofmt -l` and `gofumpt -l`, and push a formatting commit before saying it is done.

### The same gap is usually in GitLab's own documentation

A field client-go does not model is often a field GitLab never documented either, and the work is not finished until both are sent. Code owners treat documentation as the contract: "`CONTRIBUTING.md` says this project only supports what is in the public API docs. Right now, none of these seven fields are on <https://docs.gitlab.com/api/system_hooks/> ... Should we wait for that docs MR to merge first?" (ck3g, [client-go!3048](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3048)); "Would you mind updating the documentation as well on the gitlab community fork?" (PatrickRice, [client-go!3045](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3045)). A field already in the OpenAPI document was accepted while its prose documentation was pending ([client-go!3043](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3043)), and one documented nowhere was checked by calling the API ([client-go!3049](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3049)). Offering the documentation merge request unprompted saves a round trip.

**Check before assuming.** Read the page from `gitlab-org/gitlab` (project 278964) and grep for the field. Do not trust the page's title to name the field's home: the service account page is `doc/api/service_accounts.md`, not `user_service_accounts.md`, and a wrong path answers 404 rather than "absent".

**Where the change goes.** Branch on the community fork `gitlab-community/gitlab-org/gitlab` (project 41372369, default branch `master`), and open the merge request against `gitlab-org/gitlab` `master` with `target_project_id`.

Four rules the documentation merge requests sent so far were shaped by:

1. **Every example, not the one that prompted it.** `doc/api/secure_files.md` had four example responses and all four predated the field.
2. **The attribute table counts as documentation.** `doc/api/cluster_agents.md` documents its responses twice, in a table and in an example.
3. **Other entities share the page.** The cluster agents page also documents agent tokens and receptive URL configurations, which are different Grape entities and must not gain the field. Identify the right objects structurally (the agent examples are the ones carrying `config_project`), never by position.
4. **Put the field where the entity exposes it, and keep the table aligned without re-padding it.** Read the order off `gitlab-api-live.json`.

**Two traps in that record**, both of which have already produced a wrong published claim. Its per-field gate key is `conditions`, a **list**, not `condition`: reading it as `condition` reports every field as unconditional and never errors, which is how issue 2300 came to claim that all six `API::Entities::ServiceAccount` fields are unconditional when `unconfirmed_email` is gated on `unconfirmed_email.present?`. And the record is a pin taken at one release, so it lags master. Before writing a conditionality claim into a public description, confirm it against GitLab's current Ruby, and say "sent only when ..." rather than nothing when a field is gated.

Validate before pushing: parse every ` ```json ` block on the page and assert the field is present in exactly the objects that should have it and absent from the rest.

Finally, link both ways: a note on the client-go merge request naming the documentation one, and the client-go merge request named in the documentation description as where the gap was found.

### Keep issue 2300 current

The umbrella issue carries a table of what has been sent, and a table that lags is worse than no table. Update it at both moments. The table is finite: it records what went out up to [client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063) and closes with it, and nothing found after that is added.

- **When the merge request is published**, add its row: the merge request link, the struct, the field or fields, and `open`.
- **When it merges**, change the row to say which release carries it, and read that from the repository rather than assuming: a merge and a tag are different events ([client-go!3040](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3040) was merged and in no tag for hours). The semantic-release bot posts "This MR is included in version X" on the merge request when it is released (17 notes on ours), or ask which tags contain the merge commit with `repository/commits/<merge_commit_sha>/refs?type=tag`; an empty answer means merged and unreleased, which the row should say in those words.
- Update the issue by rewriting its description (`PUT /projects/:id/issues/2300`), not by commenting. **Read, modify, write; never PUT a locally held copy.** Contributor success prepends an `<!--IssueSummary start-->` block that nothing here writes; a blind PUT deletes it. Replace the exact region, refuse to write if the anchor does not appear exactly once, and check the block survived in the response.
- Re-read the state of every merge request in the table while doing it.
- **Nothing in the issue sidebar is ours to set through the API**: labels, assignees, milestone, weight, iteration and dates are dropped silently for a non-member. Two routes reach the labels. The label command's processor (`command_mr_label.rb`) reacts to issue notes as well as merge request notes and accepts the issue's author, so `@gitlab-bot label ~"type::feature"` on our own issue should apply a label from the same allowed list (read from the source, not yet tried); and the contributor platform's issue page has a label action that posts `/label` with a label from its own list. An `untriaged-issues` policy labels a new issue about fifteen minutes after creation and applies only the labels it is confident about; wait for it, then correct only what it got wrong. Expect `ai-quick-win-labeller-gitlab-org` to comment "Could not start processing ... insufficient permissions"; that costs nothing but the `quick win` label.

## Other gitlab-org projects

- **knowledge-graph** (Orbit): conventional titles; a documentation change to the query language also bumps the skill version and its pinned test assertions (Duo verified it on [knowledge-graph!2650](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2650)). The fork's `lint:prose` fails with "diff base ... is unreachable; the lint did not run" and `pinned-version-check` fails in the fork too; a reviewer starts the canonical pipeline. Two open merge requests that each bump the same version conflict by design; whichever merges second rebases (aalgutifan asked for a rebase on [knowledge-graph!2651](https://gitlab.com/gitlab-org/orbit/knowledge-graph/-/merge_requests/2651) after removing the hardcoded version from those tests on main).
- **cells/http-router**: a pipeline runs on a mirror at ops.gitlab.net; a route snapshot refresh is a one-file merge request opened from the personal fork, since no community fork exists.
- **Any other project**: read its `CONTRIBUTING.md`, its merge request templates, its `.gitlab/CODEOWNERS` and its Danger rules before writing, look for a community fork under `gitlab-community/gitlab-org/`, and copy the shape of its recently merged community merge requests.

## Answering review

- **Reply inside the thread**: `POST /projects/:id/merge_requests/:iid/discussions/:discussion_id/notes`, with the full discussion id from a listing (a truncated display id answers 404). Posting to `/notes` creates a separate top-level comment beside the conversation.
- **A `500` from that endpoint is a lie: the note was created anyway.** Never blind-retry a write to it; re-list the discussions and check first. Delete extra copies with `DELETE /notes/:id`, keeping the earliest.
- **Answer every thread, Duo's and Danger's included**, with what changed and the commit that changed it ("Applied in <sha>"), or the evidence for not changing it.
- **Do not resolve threads on somebody else's merge request**; reply and leave them for the reviewer. The exception is a thread we created that answers nobody's question, below.
- **A reply under your own note can block the merge.** A top-level note is an individual note, which is not resolvable; replying to it through the discussions endpoint turns it into a resolvable thread that starts unresolved, and an unresolved thread fails `DISCUSSIONS_NOT_RESOLVED`. That added a merge blocker to the approved [gitlab-org/gitlab!254699](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/254699) at the moment a maintainer was asked to set auto-merge. Read the discussion's `individual_note` first: if the reply only adds information, post it as a new top-level note; if it must sit under the old one, resolve that thread once posted, with the maintainer's yes.
- **Suggestions.** The response of `PUT /suggestions/:id/apply` (or `/suggestions/batch_apply`) still says `applied: false`, because it presents the suggestion as loaded before the commit, and the merge request's `sha` catches up seconds later; read the branch head before quoting a sha. Applying through the API does not resolve the thread (the UI flow does; `Suggestions::ApplyService` does not). A custom `commit_message` drops the reviewer's co-author trailer unless it contains `%{co_authored_by}`, which the default message carries. A suggestion commit on a merge request without squash fails Danger ("If you are applying suggestions, edit the merge request, enable Squash commits ..."). When a suggestion cannot be taken as written (generated file, broken footnote label, missing blank line, gofmt), commit the corrected change and say why in the thread.
- **Follow-ups.** Reviewers often accept a narrower merge request with "fine for a follow-up" (8 notes on 6 of ours). Say you will open it, open it after the merge, and name it in the original thread. A maintainer who files follow-up issues and offers to assign them is asking for more of the same (eduardosanz on [gitlab-org/gitlab!259764](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/259764): "We wish you can implement the other two issues assigned to you"); under the Hackathon rules, be assigned before opening.
- **When a merge request stops showing a pushed commit** for more than a few minutes after an ordinary push, close and reopen it through the API once (`state_event=close`, then `reopen`) before any ready command; after a force-push wait instead. Check the diff (`/merge_requests/<iid>/diffs`), not the commit count.

## What actually moves a merge request

The merge depends on a reviewer and a maintainer deciding to look, and nothing in this file makes that decision happen. Ready state does not predict a merge (one sampled request was ready within half an hour and merged 54.7 days later), labels do not gate one, and the pipeline is usually already green. Ranked honestly:

1. **The one lever**: `@gitlab-bot ready @<the right person>` once the diff exists. It is the only route a non-member has to the reviewer field.
2. **Prevents a second wait, does not shorten the first**: a change that already satisfies the style guide, the spec conventions and the generated-file checks. A round trip costs another wait of the same length as the first.
3. **Bookkeeping, cheap enough to do**: title, description template, changelog trailer, `type::` label. They decide what the release looks like, not whether the merge happens.
4. **Worth nothing**: re-posting commands, requesting Duo, chasing `danger-review` and `autolabels` on client-go, retrying pipelines, or opening more merge requests in the same reviewer's queue hoping to raise the odds.

## After the merge

```bash
go get gitlab.com/gitlab-org/api/client-go/v3@latest
go mod tidy
```

Then retire the workaround this project carried for the gap (a captured-response read under [ADR-0021](../../../docs/development/adr/adr-0021-captured-response-for-fields-the-sdk-does-not-model.md), or a request built by hand), mark the `docs/development/upstream-bugs.md` entry merged with the version that carries it, and update the row in issue 2300 when the merge request came from it. Batch the client-go version bump across several merges rather than raising the pin per merge.

For gitlab-org/gitlab, the version is the milestone the maintainer sets at merge (19.4 or 19.5 on ours); confirm it on the merge request and in the history item before writing it into the register.

## Validation checklist

All projects:

- [ ] The maintainer said yes to this exact write, and a second Opus agent reviewed it
- [ ] No open merge request or issue, from anyone, already covers it; none of ours touches the same lines
- [ ] Hackathon: jmrp is assigned to the linked issue, it is not `quick win::first-time contributor`, it is not a second typo fix in the project, and today's count is under 20
- [ ] Defect re-verified on the current default branch; the new test fails without the change and passes with it
- [ ] Branch `jmrp-<topic>` pushed to the community fork (or the personal fork where none exists) through the credential helper, signed with the global identity
- [ ] Commit messages pass `scripts/lint/commit_linter.rb` and the Danger rules: subject 3 words to 72 characters, capitalised (in client-go, a conventional prefix instead), no period; body at most 72 columns; no references of any kind
- [ ] Description uses the project's template with its comments and quick actions removed, one paragraph per line, says where the gap was found, and links the issue with `Closes #N` or `Related to <URL>` at the start of a line
- [ ] No reviewer, assignee, label or milestone set through the API, and no `@GitLabDuo` request
- [ ] The diff exists before the ready note; the ready note's first line holds only the command and the names; a label command, if any, on its own line
- [ ] Reviewer field read after the ready
- [ ] The register entry in `docs/development/upstream-bugs.md` added or updated

gitlab-org/gitlab:

- [ ] `Changelog:` trailer on an observable change, `EE: true` when `ee/` changed
- [ ] rubocop clean on changed Ruby; specs run under `flock` against the worktree's own database
- [ ] markdownlint and Vale clean in the docs-lint image, compared with the parent page
- [ ] OpenAPI v3 regenerated and `check_docs` clean when a route or entity annotation changed; GraphQL docs and both introspection schemas regenerated when a type changed; router snapshot paired when a route was added
- [ ] Documentation edits follow the style guide items above, cover every example and table on the page, keep tables unrealigned, and carry a history line with the shipping milestone
- [ ] Before any push to an approved merge request: system notes read for auto-merge or merge train, the maintainer told that the push resets approvals

client-go:

- [ ] Branch pushed to the community fork, merge request opened against project 65271576, `main` unless the change breaks the API
- [ ] Conventional prefix **with the API file as scope** on the title, and a prefix on every commit
- [ ] Description uses the three `##` headings, names this MCP server as where the gap was found, and links it
- [ ] While issue 2300 is open and the merge request comes from it, the description ends with `Related to https://gitlab.com/gitlab-org/api/client-go/-/issues/2300` on its own line; `Closes` only on [client-go!3063](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests/3063)
- [ ] Row added to issue 2300's table once published, and corrected with the release once merged
- [ ] Field placed in API-documentation order, `int64`/`any`, primitives in response structs, `PathEscape` where a path parameter is interpolated
- [ ] Every public symbol carries a name-first comment ending in a `// GitLab API docs:` link
- [ ] Test asserts the new field in every method that carries it, starts with `t.Parallel()`, uses testify
- [ ] `make reviewable` passes, and `gofmt -l` and `gofumpt -l` are clean after any applied suggestion
- [ ] `type::` label asked for by comment (replacing the `type::maintenance` a reference to issue 2300 copies), then `@gitlab-bot ready @<code owner>`
- [ ] `doc/api/` checked for the same field, and a documentation merge request opened against `gitlab-org/gitlab` when it is missing, covering every example **and** every attribute table on the page, readied naming the page's writer, linked both ways
