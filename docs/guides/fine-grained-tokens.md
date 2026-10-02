# Fine-grained Personal Access Tokens

How to run gitlab-mcp-server with a GitLab fine-grained personal access token: what
to grant it, what the server does with it, and how to read what it tells you when an
action is out of the token's reach.

> **Diátaxis type**: How-to
> **Audience**: 👤 Users and 🔧 operators who hand the server a fine-grained token
> **Prerequisites**: GitLab 19.2 or later, where fine-grained tokens are generally
> available (they were a beta from 18.10); the permissions this server knows are the
> ones GitLab 19.4.1 declares

---

## What a fine-grained token is

A classic personal access token carries scopes (`api`, `read_api`, `read_user` and the
rest), and a scope opens a whole class of endpoints. A fine-grained token carries none
of them: its scope list is the single value `granular`, and what it may do is a
**grant** of named permissions (Project: Read, Merge Request: Approve, Pipeline: Read),
each held at a boundary (a project, a group, the user, the instance). GitLab
authenticates the token first and then judges every request against the grant.

Two facts about the grant decide most of this guide:

- **A grant cannot be changed after the token is created.** GitLab 19.4 has no route,
  GraphQL mutation or settings page that edits one, and rotating a token copies its
  grant into the new one. A missing permission therefore means a new token.
- **A grant is judged per request.** A token can be refused one call and served the
  next, and a GraphQL query can come back partly empty, with no error, where the grant
  does not reach part of the answer.

The server reads a fine-grained token as **unknown authority**, never as read-only:
its `granular` scope says nothing about writes, so it is not narrowed to the read-only
surface a `read_api` token gets, and the five `admin_mode` groups stay in its catalog.
What it is shown is decided by its grant instead, as the rest of this guide describes
([ADR-0024](../development/adr/adr-0024-fine-grained-token-authority-per-action.md)).

## The permissions the server needs to start

Grant these besides whatever your work needs. Each is optional, and the table says
what is lost without it.

| Permission (GitLab's words) | Boundary | What the server uses it for                                                                                 | Without it                                                                                                                       |
| --------------------------- | -------- | ----------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| User: Read                  | user     | `GET /api/v4/user`: the HTTP door's check of every new credential, and the identity on stdio                | HTTP mode refuses the token at the door with `403` (see [HTTP mode](#http-mode-the-door)); stdio starts without knowing the user |
| Metadata: Read              | instance | `GET /api/v4/version`: the instance version the grant is judged at, and the edition                         | The grant is not evaluated (the server falls back as below); stdio logs one warning naming the permission and starts anyway      |
| Personal Access Token: Read | user     | Reading the token's own grant (`GET /api/v4/personal_access_tokens/self` and `/personal_access_tokens/:id`) | The grant is not evaluated, and the server withholds only what no fine-grained token can reach                                   |
| Namespace: Read             | user     | The namespace plans the licensing tier is detected from, which is the subscription on GitLab.com            | The tier falls back to what the license says, or Free; set `GITLAB_MCP_TIER` (`--tier` in HTTP mode) to pin it                   |
| License: Read               | instance | The license of a self-managed instance, which only an administrator may read                                | Nothing for a non-administrator, who cannot read it with any token                                                               |

The first four are what a session needs to be served exactly what its grant reaches,
and they are the grant the end-to-end suite starts its fine-grained sessions on. Without
Metadata: Read or Personal Access Token: Read the server still starts and serves the
token, and withholds only the actions no fine-grained token can reach, saying in each
refusal why the grant was not evaluated.

## Create the token

**In the UI**: select your avatar, **Edit profile**, then **Access** > **Personal
access tokens**, and from **Generate token** choose **Fine-grained token**. Under
**Add resource permissions**, the **Group and project**, **User** and **Global** tabs
are the project or group, user and instance boundaries the table above names. GitLab's
own page describes every step:
[Fine-grained personal access tokens](https://docs.gitlab.com/auth/tokens/fine_grained_access_tokens/).

**Through the API**: `POST /api/v4/user/personal_access_tokens` creates a token for
the user whose credential sends the request (`$CREATING_TOKEN` below, one of your own),
with `granular_scopes` in place of `scopes`. Each scope names an access level, the
permissions it grants by GitLab's identifier for each (`read_project`, which the UI
calls Project: Read), and, for `selected_memberships`, the projects or groups it
covers:

```bash
curl --request POST "$GITLAB_URL/api/v4/user/personal_access_tokens" \
  --header "PRIVATE-TOKEN: $CREATING_TOKEN" \
  --header "Content-Type: application/json" \
  --data '{
    "name": "gitlab-mcp-server",
    "expires_at": "2027-01-31",
    "granular_scopes": [
      {"access": "user", "permissions": ["read_user", "read_namespace", "read_personal_access_token"]},
      {"access": "instance", "permissions": ["read_metadata"]},
      {"access": "selected_memberships", "project_ids": [42],
       "permissions": ["read_project", "read_work_item", "create_work_item", "read_merge_request"]}
    ]
  }'
```

Send every scope in the one request: a grant cannot be added to afterwards. The access
levels are `personal_projects` (the projects in your own namespace),
`selected_memberships` (the projects and groups you list, and everything under a listed
group), `all_memberships` (every project and group you are a member of), `user` and
`instance`. Every project or group id becomes a scope of its own. The administrator
route that creates a token for another user takes no grant at 19.4, so this server's
own token creation actions create classic tokens only.

## Grants for common uses

Each row is what the actions it names need at the project boundary, beside the startup
permissions above. [Fine-grained Permissions](../reference/fine-grained-permissions.md)
lists every action this server offers with what it needs, and `gitlab://tools/{id}`
serves the same for one action in its `fine_grained` block.

| You want the assistant to                  | Grant at the project                                                     | Actions this covers, for example                                                           |
| ------------------------------------------ | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------ |
| Read a project's issues and merge requests | Project: Read, Work Item: Read, Merge Request: Read                      | `project.get`, `issue.list`, `issue.get`, `merge_request.list`                             |
| Triage and comment on issues               | the row above, plus Work Item: Create and Work Item: Update              | `issue.create`, `issue.update`, `issue.note_create`                                        |
| Review merge requests                      | Merge Request: Read, Merge Request: Create, Merge Request: Approve       | `mr_review.changes_get`, `mr_review.discussion_create`, `merge_request.approve`            |
| Follow CI                                  | Pipeline: Read, Job: Read                                                | `pipeline.list`, `pipeline.get`, `job.list`, `job.trace`                                   |
| Edit files on a new branch                 | Repository: Read, Repository: Create, Repository: Update, Branch: Create | `repository.file_get`, `repository.file_create`, `repository.file_update`, `branch.create` |

Some pairings are GitLab's rather than this server's: a note on an issue or a merge
request is created with Work Item: Create, and a merge request discussion with Merge
Request: Create, because that is what the routes declare at 19.4.1.

## What the server does with the token

### The grant decides what the session is shown

When the token may read its own grant and the instance runs GitLab 19.4, the release
the server's permission table records, the server reads the grant with the token
itself and judges every action against it. A session is then **listed** the actions its
grant reaches, on every surface (`tools/list`, `gitlab_find_action`, `gitlab://tools`),
and a call to any other is answered with the permission it needs, in the words the
token creation page uses, without a request to GitLab. Two kinds of call are let
through even though the listing leaves them out, so GitLab judges them: a read GitLab
would serve on a public project or group whatever the grant, and, on a prerelease
instance (below), every call that is not withheld from all fine-grained tokens.

The grant is read once when the session starts and again on every revalidation (every
15 minutes by default in HTTP mode, and on the same timer on stdio), so an upgraded
instance moves the session to what its new release decides. A re-read that fails keeps
what the session was shown rather than narrowing it, so what an assistant sees never
changes with a GitLab outage. The server reads at most 1 MiB and 1000 scopes of a
grant; a larger grant is not evaluated.

### When the grant is not evaluated

The session falls back to withholding only what no fine-grained token can reach (the
next section), and every refusal says why the grant was not evaluated:

| What the refusal says                                                          | Cause                                                                                         | What to do                                                                   |
| ------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| The token cannot read its own grant                                            | The token lacks Personal Access Token: Read                                                   | Create a token that also grants it                                           |
| The instance did not report a version this server can read                     | The token lacks Metadata: Read, or the instance answered with no release number               | Create a token that also grants it                                           |
| The instance reports GitLab x.y and the permissions are recorded for 19.4 only | The instance runs a release the table does not record                                         | Nothing on your side; the table moves with the server's releases             |
| The token's grant names a permission GitLab 19.4.1 does not define             | A permission GitLab renamed or added after 19.4                                               | As above                                                                     |
| The token's grant is larger than this server reads                             | More than 1 MiB or 1000 scopes                                                                | Grant at a group rather than project by project                              |
| The token's grant holds a scope this server cannot read without guessing       | An access level the server does not know, or a scope naming no project or group where it must | Report it as an issue of this project, quoting the reason the log line names |
| The instance did not answer the request for the token's grant or its version   | GitLab was unreachable at that moment                                                         | The next revalidation reads it again                                         |

The server writes one line at `INFO` when a session starts in this state, naming the
reason (`grant-unreadable`, `version-unreadable`, `version-outside-record` and so on)
and never the token, its id or its grant.

### What no fine-grained token can reach

Some of what this server offers goes through GitLab GraphQL types or mutations that
declare no fine-grained permission at 19.4.1, and GitLab refuses or empties those for
every fine-grained token, whatever its grant. At 19.4.1 that is 58 actions of 1098, all
through GraphQL, each in one of four ways:

| How GitLab answers                                                              | Actions | Examples                                                                                                                                                                        |
| ------------------------------------------------------------------------------- | ------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A type on the answer's path declares nothing: null, or a list emptied or nulled |      34 | the epic, work item and saved view reads (`group.epic_get`, `issue.work_item_list`), `branch.rule_list`, `ci_catalog.list`, `custom_emoji.list`, `vulnerability.severity_count` |
| The write commits, and the answer is null                                       |      20 | the achievement writes, `custom_emoji.create`, `issue.work_item_create`, `security_attribute.create`                                                                            |
| The mutation declares nothing and is refused                                    |       3 | `security_attribute.bulk_update`, `security_scan_profile.attach`, `security_scan_profile.detach`                                                                                |
| The object never resolves to the boundary GitLab declares                       |       1 | `group.epic_create`                                                                                                                                                             |

These are withheld from every fine-grained session, with the reason, the GitLab release
the verdict comes from and the way out, which is a classic token. One more action,
`group.epic_list`, runs over REST and is refused only with an input that makes it send
its GraphQL request instead. The second row is withheld for a reason beyond the empty
answer: an assistant that reads a null as "not done" and tries again repeats a write
GitLab already committed.

[Fine-grained Permissions](../reference/fine-grained-permissions.md) names the reason
for each; making GitLab declare the missing permissions is
[issue 1055](https://github.com/jmrplens/gitlab-mcp-server/issues/1055), and serving
over REST what GraphQL cannot reach is
[issue 1054](https://github.com/jmrplens/gitlab-mcp-server/issues/1054).

### GitLab.com and other releases

The permission table is recorded from GitLab 19.4.1, and the grant is judged only on an
instance that reports a 19.4 release. An instance on another release falls back as
above, naming the version it reported and the one recorded. One exception is made for
the prerelease of the minor right after the recorded one (`19.5.0-pre`), which is what
GitLab.com and a nightly image report: such a session is **listed** what its grant
reaches at 19.4.1, and every call the fallback would allow is passed to GitLab, so a
permission GitLab changed in that one milestone surfaces as GitLab's own refusal rather
than as a refusal here with a stale name. Once GitLab.com moves past that prerelease,
it falls back like any other release until the table is recorded again.

## Reading a withheld answer

A call to an action the session may not run is answered as a tool error, before
anything reaches GitLab and before a confirmation or a safe-mode preview is offered.
It names the action by its canonical ID and opens with one of two stable texts:

- `action "branch.create" exists but this fine-grained personal access token was not
  granted what it needs: the project permission [Branch: Create], as GitLab 19.4.1
  declares it. Create a fine-grained token that grants it, or use a classic token with
  the api scope (...)`: the grant does not reach the action. The permissions are the
  ones the token creation page offers, grouped by the boundary they are held at.
- `action "custom_emoji.list" exists but is not available to a fine-grained personal
  access token: GitLab 19.4.1 declares no fine-grained permission on the GraphQL type
  CustomEmoji this action reads, and removes the items from such a list. Use a classic
  personal access token with read_api for reads or api for writes (...)`: no
  fine-grained token reaches the action at that release.

Both end with `Do not report the capability as missing.`, since the action exists and
only the credential cannot run it. On the default dynamic surface the text is prefixed
with `gitlab_execute_action:` and a space. The way out to a classic token reads "(an
existing one, on an instance that no longer lets you create them), where the group does
not refuse classic tokens", because GitLab can enforce fine-grained tokens in two ways
([below](#when-an-organization-enforces-fine-grained-tokens)).

Where to look next:

- `gitlab://tools/{id}` serves the detail of a withheld action with a `withheld` block
  carrying the cause and the same words, rather than answering not found, and a
  `fine_grained` block for every session saying what the action needs
  ([Resources](../reference/resources.md#tool-manifest-detail)).
- `gitlab_find_action` leaves out of a fine-grained session's results what it may not
  run, so the next best match takes its place, while `gitlab_execute_action` still
  answers a withheld action with the reason.
- Each refusal is logged at `INFO` with the reason class `fine_grained`, which the
  telemetry guide lists, and never with the token or its grant.

## What an empty answer can mean

Over REST, a call outside the grant is GitLab's own `403`, which names the missing
permission. Over GraphQL it is not: a position the grant does not reach comes back
`null`, and a connection drops the items it does not reach, with no error either way.
So a fine-grained session is given a next step beside three kinds of answer:

- An answer GitLab always leaves partly empty for a fine-grained token, or leaves empty
  unless the grant holds more: the note names each part as the GraphQL selection that
  reaches it (`vulnerability { issueLinks { nodes } }`) and says "Empty there does not
  mean there is nothing." `vulnerability.list` and `vulnerability.get` are served this
  way.
- A not-found answer of an action that reads GraphQL: GitLab answers null for an object
  the token is not granted or that sits outside its grant, so not found may mean the
  token cannot see it.
- An empty list from an action whose answer is a GraphQL list: GitLab leaves out the
  items the token is not granted, so empty may mean the token cannot see them.

## GitLab's own refusals

A call the server lets through can still be refused by GitLab, and the server quotes
GitLab's sentence with what to do. GitLab has four of them, described in
[Troubleshooting](troubleshooting.md#fine-grained-personal-access-tokens); the one you
will meet most is "Access denied: This operation requires a fine-grained personal
access token with the following project permissions: [...]", whose answer is a new
token that grants what it lists.

## HTTP mode: the door

In HTTP mode every new credential is checked with `GET /api/v4/user` before it is
served, and a fine-grained token reaches that route only when it grants User: Read. A
token without it is answered `403` (not `401`), is **not** charged to the address's
failure budget, since GitLab accepted it, and is remembered for five minutes so the same
token is answered from memory rather than checked again. The body quotes GitLab's
sentence and says the way out: a token that grants User: Read, or a classic token. In
OAuth mode a fine-grained token meets the `read_api` minimum the door asks for, and a
deployment that pins its OAuth applications (`--oauth-client-uid`) refuses every
personal access token, fine-grained ones included. See
[HTTP Server Mode](http-server-mode.md#fine-grained-personal-access-tokens).

## When an organization enforces fine-grained tokens

GitLab can require fine-grained tokens after a date, in two forms:

- **On GitLab.com**, the Owner of a top-level group enforces them for the group, its
  subgroups and projects. A classic token is then refused there with the same text a
  fine-grained token gets ("Access denied: This operation requires a fine-grained
  personal access token with the following ... permissions"), so a classic token can
  meet that refusal too. Classic tokens keep working outside the group.
- **On a self-managed instance**, an administrator enforces them for the whole
  instance: users can no longer create or rotate classic tokens, and existing ones keep
  working until they expire.

That is why every way out this server offers reads "a classic token (an existing one, on
an instance that no longer lets you create them), where the group does not refuse
classic tokens".

## `--ignore-scopes` and a fine-grained token

`--ignore-scopes` (`GITLAB_MCP_IGNORE_SCOPES=true`) skips the scope filter and the
read-only narrowing. It does not skip reading what kind of token the server holds, and
it does not turn off anything this guide describes: the grant is a different question
from the scopes, and skipping it would serve a fine-grained session the whole catalog
with no word of what its grant leaves out.

## See Also

- [Fine-grained Permissions](../reference/fine-grained-permissions.md): what every
  action needs, generated from the handlers and from GitLab 19.4.1
- [Troubleshooting](troubleshooting.md#fine-grained-personal-access-tokens): GitLab's
  four refusal texts and what each means
- [HTTP Server Mode](http-server-mode.md#fine-grained-personal-access-tokens): the door
  and the pool
- [Security](../concepts/security.md) and [GraphQL Integration](../concepts/graphql.md)
- [ADR-0024](../development/adr/adr-0024-fine-grained-token-authority-per-action.md):
  why the server decides this way
