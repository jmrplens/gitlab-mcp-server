# Repository settings the release pipeline depends on

Parts of the supply-chain hardening are **settings on the GitHub repository
rather than files in it**, so no commit can apply them and no gate in this
repository can see them. They are recorded here because the workflow changes
that assume them are already merged: `release.yml` publishes the release as a
draft and only marks it public once every artifact is attached and attested,
which is the sequence immutable releases require, and the deploy-key push to
`main` is already isolated in a job that checks out `main` fresh and runs no
third-party code.

> **Diátaxis type**: Reference
> **Audience**: 🛠️ Maintainers with repository admin
> **Prerequisites**: `gh` authenticated as a repository administrator

This page is a checklist. Each item states what to set, why it matters, how to
check the current state, and what still tells you when it drifts.

---

## Why none of this is a merge gate

Every other release invariant in this project is enforced by something that runs
in CI: `make check-supply-chain` reads the workflows, and
`make check-server-json-packages` downloads each declared artifact and inspects
it. Neither can be extended to cover this page.

The repository ruleset and release-immutability APIs need the **Administration**
repository permission. The `GITHUB_TOKEN` an Actions job receives cannot be
granted it: `administration` is not one of the scopes the `permissions:` key
accepts. A scheduled job that checked these settings would therefore need a
personal access token stored as a repository secret, which is one more
long-lived credential to rotate and is itself an unmanaged repository setting.
That trade is not worth taking for items that change roughly never, so the
checks below are run by hand.

The one exception is release immutability, which is visible in the public
release metadata and is already checked (see below). The NuGet.org trusted
publishing policy at the end of this page is not a GitHub setting at all, but it
is recorded here for the same reason: it lives outside the repository, the
release depends on it, and no gate can see it.

## SC-04: enable immutable releases

**What to set.** Repository → Settings → General → Releases → enable
**Immutable releases**.

**Why.** Every other artifact this project publishes is pinned by a hash or by
an immutable registry version: the `.mcpb` bundles by their `fileSha256` in
`server.json`, the container image by `@sha256:` digest, npm, PyPI and NuGet by
version (all three registries forbid republishing one). The GitHub Release is
the exception.
Without this setting, anyone holding `contents: write` can replace a published
binary **and** `checksums.txt` together, and both installers accept the
consistent pair. The cosign signature over `checksums.txt` is what travels to a
verifier who checks it; this setting is what stops the substitution happening at
the source.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/releases/latest --jq '{tag: .tag_name, immutable}'
```

**Current state.** Enabled, as of 2026-09-10. The check above answers about a
release rather than about the setting, so it keeps reporting the old state until
a release is published under it: `v2.7.5` and everything before it predate the
change and stay mutable. 3.0.0 is the first release the setting covers, and it
is the one to check the field on. Nothing in the repository API reports the
toggle itself, so this page is the record.

**Caveat.** The setting is not retroactive. Releases published before it was
turned on stay mutable forever, so it protects future releases only.
Nothing in the workflow needs to change: GoReleaser already creates the release
as a draft and a later step publishes it, which is the shape the feature
requires.

**What tells you it drifted.** `scripts/validate-server-json-packages.sh`
(`make check-server-json-packages`) reads the release metadata for the version
`server.json` declares and warns when it is mutable. It is a drift alarm, not a
gate: it warns rather than failing, because a warning that cannot be fixed by
any commit must not block a merge.

## SC-04: protect release tags

**What to set.** Repository → Settings → Rules → new ruleset, target **Tags**,
pattern `v*`, with **Restrict deletions** and **Restrict updates** enabled.

**Why.** A release is identified by its tag, and `server.json`, the Homebrew
formula, the winget manifest and every installation document reference versions
by tag. A tag that can be moved is a version number that can be pointed at
different code after the fact. The digest pinning in `server.json` closes this
for the container image; the tag ruleset closes it for the release itself.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/rulesets --jq '.[] | {id, name, target, enforcement}'
```

**Current state.** Satisfied. A second ruleset, `Protect tags` (id
`22194732`, enforcement `active`), targets every tag (`~ALL`, which is wider
than the `v*` pattern above and equally acceptable) with the `deletion`,
`update` and `non_fast_forward` rules and no bypass actors. Checked on
2026-09-05 with the command above.

## SC-04: deploy the release environment from release tags only

**What to set.** Two environments, under Repository → Settings →
Environments:

- `release`: under **Deployment branches and tags**, choose **Selected branches
  and tags** and add one rule of type **Tag** with the pattern `v*`; turn off
  **Allow administrators to bypass configured protection rules**.
- `release-rehearsal`: created with no secret, no variable and no protection
  rule, and named by no trusted publisher anywhere.

The same settings through the API:

```bash
gh api -X PUT repos/jmrplens/gitlab-mcp-server/environments/release-rehearsal
gh api -X PUT repos/jmrplens/gitlab-mcp-server/environments/release --input - <<'EOF'
{"deployment_branch_policy": {"protected_branches": false, "custom_branch_policies": true}, "can_admins_bypass": false}
EOF
gh api -X POST repos/jmrplens/gitlab-mcp-server/environments/release/deployment-branch-policies -f name='v*' -f type=tag
```

**Why.** The npm, PyPI and nuget.org trusted publishers each accept an OIDC
token from this repository, the workflow file `release.yml` and the `release`
environment. None of the three can name a ref, so any run whose copy of
`release.yml` puts a job in `release` holds the identity a release publishes
with, and the environment's secrets with it. `release.yml` sends a dispatched
run (a rehearsal) to `release-rehearsal` instead (issue 1322), but that is the
workflow's own decision, and a branch can carry a `release.yml` that decides
otherwise. The deployment policy is what decides it from outside the tree: a
job that names `release` from anything but a `v*` tag is refused before it
starts. Administrator bypass is turned off so the policy holds for an
administrator's run too; it costs a release nothing, since a release runs from
a tag the policy admits.

**Order.** Create `release-rehearsal` before the first rehearsal that names it.
Restrict `release` only once the workflow that sends rehearsals elsewhere is on
`main`: until then every rehearsal runs in `release` and would be refused.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/environments/release --jq '{deployment_branch_policy, can_admins_bypass}'
gh api repos/jmrplens/gitlab-mcp-server/environments/release/deployment-branch-policies --jq '.branch_policies[] | {name, type}'
gh api repos/jmrplens/gitlab-mcp-server/environments/release-rehearsal/secrets --jq '.total_count'
```

The first answers `custom_branch_policies: true`, `protected_branches: false`
and `can_admins_bypass: false`; the second lists exactly one policy,
`{"name": "v*", "type": "tag"}`; the third answers `0`.

**Current state.** Not yet applied. On 2026-10-12 `release` had no deployment
policy (`deployment_branch_policy: null`) and administrator bypass on, and
`release-rehearsal` did not exist.

**Caveat.** The policy binds the environment and nothing outside it. A
repository secret, as opposed to an environment secret, reaches any workflow
run in the repository whatever environment its job names. Every secret the
publisher jobs read that is a repository secret, a repository-level copy of one
the `release` environment also holds included, is outside what this item
protects; moving those into `release` is what would bring them under it.

**What tells you it drifted.** Nothing automatic. A rehearsal still passes with
the policy gone, because it no longer runs in `release`; only the commands above
show it.

## SC-04: restrict who creates a release tag

**What to set.** Repository → Settings → Rules → new ruleset `Create release
tags`, target **Tags**, enforcement **Active**, targeting the pattern `v*`, with
**Restrict creations** enabled and the **Repository admin** role as the only
bypass actor. Through the API:

```bash
gh api -X POST repos/jmrplens/gitlab-mcp-server/rulesets --input - <<'EOF'
{"name": "Create release tags", "target": "tag", "enforcement": "active",
 "conditions": {"ref_name": {"include": ["refs/tags/v*"], "exclude": []}},
 "rules": [{"type": "creation"}],
 "bypass_actors": [{"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always"}]}
EOF
```

**Why.** The deployment policy above admits any tag matching `v*`, so it binds
the `release` identity to release tags only as far as creating such a tag is
restricted. Without this rule, anything that can push a tag can create one on a
commit whose `release.yml` publishes whatever it likes. A deploy key with write
access pushes it, and the push itself runs that workflow in `release`. A tag a
workflow creates with a `GITHUB_TOKEN` holding `contents: write` starts no run
by itself, since GitHub triggers nothing from that token's pushes, but a
`workflow_dispatch` against the tag then runs the tagged commit's
`release.yml`, which can name `release` outright, and the policy admits it
because it judges the ref, a tag matching `v*`. Push access to this repository
is held by administrators alone today, so this item and the one above are
defense in depth rather than the only barrier.

**Why a ruleset of its own.** A ruleset's bypass list applies to every rule in
it. `Protect tags` (id `22194732`) restricts deletion and update of every tag
with no bypass at all; adding the administrator role there so administrators
could still create release tags would also let them delete or move one, which
that ruleset exists to forbid.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/rulesets --jq '.[] | select(.target == "tag") | {id, name, enforcement}'
gh api repos/jmrplens/gitlab-mcp-server/rulesets/<id> --jq '{include: .conditions.ref_name.include, rules: [.rules[].type], bypass: .bypass_actors}'
```

The second command, given the id of `Create release tags`, answers
`include: ["refs/tags/v*"]`, `rules: ["creation"]` and one bypass actor,
`{"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always"}`.

**Current state.** Not yet applied. On 2026-10-12 the only tag ruleset was
`Protect tags`, with the rules `deletion`, `update` and `non_fast_forward`.

**What tells you it drifted.** Nothing automatic; the commands above.

## SC-01: keep the deploy-key ruleset bypass declared

**What to set.** In the `Protect main` ruleset, keep **Deploy keys** on the
bypass list.

**Why.** `release.yml` pushes the version stamp straight to `main` with a deploy
key, which is a write to a protected branch that no pull request reviewed. A
deploy key is not subject to the ruleset, so the push succeeds either way; the
bypass entry is what makes that visible on the settings page instead of leaving
an audit reader to work out why a protected branch has unreviewed commits on it.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/rulesets/16049728 --jq '.bypass_actors'
```

**Current state.** Already satisfied. The ruleset carries
`{"actor_type": "DeployKey", "actor_id": null, "bypass_mode": "always"}` (beside
a `RepositoryRole` entry for the repository administrators), and
the oldest version its history records already carries it, so it predates the
security work that filed this item. The remaining work is to keep it: a standing
requirement to re-check after any ruleset edit, not an outstanding change.

**The alternative, and why it was not taken.** The push could go through a
branch and a pull request instead of bypassing at all. That would remove the
bypass but would also make every release wait on a second human action, in a job
that runs after the artifacts are already published, to commit a version number
the release itself determined. The bypass with the credentialed push isolated in
its own job, which is what `release.yml` does today, is the better trade.

## NuGet.org trusted publishing policy

**What to set.** On nuget.org, under the username menu, **Trusted Publishing**:
one policy that lets the release workflow push the NuGet distribution with no
stored credential.

**Why.** `release.yml`'s `nuget` job holds `id-token: write` and exchanges its
OIDC token for a one-hour nuget.org API key through the `NuGet/login` action,
then pushes the seven packages with `dotnet nuget push --skip-duplicate`. The
exchange succeeds only when the token's claims match a policy on the account
that owns the packages, so the policy is what stands between "any workflow in
any repository" and "this workflow file, in this repository, running under the
`release` environment". No repository secret is involved: `make publish-nuget`
with an API key in `NUGET_API_KEY` is the only tokened path, for an
out-of-band push from a maintainer machine.

**Current state.** Satisfied. The policy exists and is **Active**, with the
repository and owner ids locked in (the repository is public, so nuget.org
resolved them at creation). Its fields, recorded here because nothing in the
repository can read them back:

| Field            | Value                                  |
| ---------------- | -------------------------------------- |
| Package owner    | `jmrplens`                             |
| Scope            | Push new packages and package versions |
| Package glob     | `gitlab-mcp-server*`                   |
| Publisher        | GitHub Actions                         |
| Repository owner | `jmrplens`                             |
| Repository       | `gitlab-mcp-server`                    |
| Workflow file    | `release.yml`                          |
| Environment      | `release`                              |

The glob covers the pointer package `gitlab-mcp-server` and the six
runtime-identifier packages `gitlab-mcp-server.<rid>`, and the "push new
packages" scope is what lets the first release create the seven ids: unlike
npm and PyPI, no bootstrap publish with a token is needed. The workflow's
`NuGet/login` step names the nuget.org profile (`jmrplens`) as its `user`
input; that is the account the policy belongs to, not a secret.

**How to check.** There is no API for it; open nuget.org, the username menu,
Trusted Publishing, and confirm the policy is listed as Active with the fields
above. A release rehearsal no longer exercises the exchange: it runs in the
`release-rehearsal` environment, which this policy does not name and must not,
since a policy naming it would hand every rehearsal a push key, so the
`NuGet login` step is skipped there (issue 1322).

**Caveat.** A policy nuget.org cannot bind to repository and owner ids at
creation starts as **temporarily active for 7 days** and lapses if no publish
happens in that window; if the policy ever shows that state, restart the
window from the same page before the release, and the first successful push
makes it permanent.

**What tells you it drifted.** The `nuget` job of the next release fails at
its login step, before any push, and nothing else in the release is affected.

## Checking all of it at once

```bash
gh api repos/jmrplens/gitlab-mcp-server/releases/latest --jq '"immutable_release=\(.immutable)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets --jq '.[] | "ruleset=\(.name) target=\(.target)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets/16049728 --jq '.bypass_actors[] | "bypass=\(.actor_type)"'
gh api repos/jmrplens/gitlab-mcp-server/environments/release --jq '"release_policy=\(.deployment_branch_policy) admins_bypass=\(.can_admins_bypass)"'
gh api repos/jmrplens/gitlab-mcp-server/environments/release/deployment-branch-policies --jq '.branch_policies[] | "release_ref=\(.type):\(.name)"'
gh api repos/jmrplens/gitlab-mcp-server/environments/release-rehearsal/secrets --jq '"rehearsal_secrets=\(.total_count)"'
```

Run these after any change to repository settings, and before a release when one
of the items above is still open.
