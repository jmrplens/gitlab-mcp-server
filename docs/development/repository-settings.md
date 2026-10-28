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

There are two exceptions. Release immutability is visible in the public
release metadata and is already checked (see below), and the documentation
site's response headers are visible in its public responses and are checked
by the `headers` job of `pages.yml` after every deploy (see
[Documentation site response headers](#documentation-site-response-headers)).
The npm trusted
publishers, the NuGet.org trusted publishing policy and the PyPI trusted
publisher at the end of this page are not GitHub settings at all, but they are
recorded here for the same reason: they live outside the repository, the
release depends on them, and no gate can see them. The `Protect main` item is
the rule every pull request merges under rather than something the release
depends on, and it is recorded here because
[GOVERNANCE.md](../../GOVERNANCE.md#access-and-two-factor-authentication)
describes it and no file in the tree can show it. The documentation site's
response headers are a Cloudflare setting, and the release does not depend on
them either; they are here because the OpenSSF Best Practices answer for the
project's website rests on them and only the maintainer can set them. The web
commit sign-off is not a release setting, and it is here because the sign-off
check every pull request runs depends on it for the commits GitHub makes in its
web interface.

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

**Current state.** Applied. `release-rehearsal` exists since 2026-10-17, with
no secret and no protection rule. `release` was restricted on 2026-10-20, once
the workflow that sends rehearsals elsewhere was on `main`; until then it had no
deployment policy and administrator bypass on. Read that day after the change,
the three checks above answer as they should: `custom_branch_policies: true`,
`protected_branches: false`, `can_admins_bypass: false`, the one policy
`{"name": "v*", "type": "tag"}`, and `0` secrets on `release-rehearsal`.

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

**Current state.** Applied on 2026-10-20 as ruleset `25490294`, beside
`Protect tags`. Read that day, the second command answers
`include: ["refs/tags/v*"]`, `rules: ["creation"]` and the one bypass actor
above.

**What tells you it drifted.** Nothing automatic; the commands above.

## Protect main: how a change reaches `main`

**What to set.** Repository → Settings → Rules, the branch ruleset
`Protect main` (id `16049728`), enforcement **Active**, targeting
`refs/heads/main`, with four rules: **Restrict deletions**, **Block force
pushes**, **Require a pull request before merging** with **0** required
approvals and **Require conversation resolution before merging** turned on,
and **Require status checks to pass** with the one check `✅ CI`. Its bypass
list holds **Deploy keys**, for the reason the next item gives, and the
**Repository admin** role. Beside the ruleset, Repository → Settings → General
→ Pull Requests allows **squash merging** alone.

**Why.** [Code Review](../../CONTRIBUTING.md#code-review) says what a pull
request needs before it is merged, and these are the parts of it GitHub can
hold: every review thread answered and resolved, and the one aggregate check
every CI job reports into. No approval is required because GitHub does not let
the author of a pull request approve it, so while the project has one
maintainer a required approval is one only the bypass could satisfy on the
maintainer's own pull requests. Squash is the only merge method because the
squash commit's message is the pull request description, which
`make check-pr-description` holds, so `main` gains one commit per pull request,
with a message that was checked.

**The admin bypass.** With the **Repository admin** role on the bypass list,
the ruleset does not refuse a merge the maintainer makes: what holds that merge
is [What a change needs to be accepted](../../CONTRIBUTING.md#what-a-change-needs-to-be-accepted).
GitHub records each bypass of the ruleset, a version stamp's push included, and
lists them:

```bash
gh api 'repos/jmrplens/gitlab-mcp-server/rulesets/rule-suites?ref=refs/heads/main&rule_suite_result=bypass&time_period=month' \
  --jq '.[] | {pushed_at, actor_name, after_sha}'
```

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/rulesets/16049728 \
  --jq '{include: .conditions.ref_name.include, rules: [.rules[] | {type, parameters}], bypass: .bypass_actors}'
gh api repos/jmrplens/gitlab-mcp-server --jq '{allow_squash_merge, allow_merge_commit, allow_rebase_merge}'
```

The first answers `include: ["refs/heads/main"]`; the rules `deletion` and
`non_fast_forward` with no parameters, `pull_request` with
`required_approving_review_count: 0` and
`required_review_thread_resolution: true`, and `required_status_checks` naming
the one context `✅ CI`; and two bypass actors,
`{"actor_type": "DeployKey", "actor_id": null, "bypass_mode": "always"}` and
`{"actor_type": "RepositoryRole", "actor_id": 5, "bypass_mode": "always"}`. The
second answers `allow_squash_merge: true` and the other two `false`. The
ruleset's own `allowed_merge_methods` lists all three methods; the repository
setting is what narrows them to squash.

**Current state.** Applied. Read on 2026-10-25, both commands answer as above.

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

## npm trusted publishers

**What to set.** On npmjs.com, for each of the seven packages,
`@jmrp.io/gitlab-mcp-server` and the six `@jmrp.io/gitlab-mcp-server-<os>-<cpu>`
packages, the package's settings, **Trusted Publisher**: GitHub Actions, user
`jmrplens`, repository `gitlab-mcp-server`, workflow `release.yml`, environment
`release`.

**Why.** `release.yml`'s `npm` job holds `id-token: write`, and npm exchanges
the job's OIDC token for a publish credential only when its claims match the
publisher configured on the package, so no npm token is stored anywhere and
the registry attaches provenance to each version. npm binds a publisher to one
package rather than to an account, which is why each of the seven has its own;
the first version of each was published with a token, since a package has to
exist before it can have a publisher
([Release Process](release-process.md)). `make publish-npm` with an npm token
is the only tokened path, for an out-of-band publish.

**Current state.** Satisfied for all seven. Read on 2026-10-25, the registry
names `GitHub Actions` as the publisher of each package's 3.3.0, through a
trusted publisher (`trustedPublisher.id: "github"`), and each version's SLSA
provenance names `.github/workflows/release.yml` at `refs/tags/v3.3.0` in this
repository. Neither record names the environment: the package's settings page
on npmjs.com is the only place it shows.

**How to check.**

```bash
for p in gitlab-mcp-server gitlab-mcp-server-darwin-arm64 gitlab-mcp-server-darwin-x64 \
  gitlab-mcp-server-linux-arm64 gitlab-mcp-server-linux-x64 gitlab-mcp-server-win32-arm64 gitlab-mcp-server-win32-x64; do
  curl -s "https://registry.npmjs.org/@jmrp.io%2f$p/3.3.0" \
    | jq -c '{name, publisher: ._npmUser.name, trusted: ._npmUser.trustedPublisher.id}'
done
```

Each line answers `publisher: "GitHub Actions"` and `trusted: "github"`. A
release rehearsal does not exercise the exchange: it runs in the
`release-rehearsal` environment, which no publisher names, and publishes
nothing.

**What tells you it drifted.** The `Publish to npm` step of the next release
fails at the first package npm refuses. `scripts/publish-npm.sh` skips a
version already published, so once the publisher is restored the job can be
re-run.

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

## PyPI trusted publisher

**What to set.** On pypi.org, the project `jmrplens-gitlab-mcp-server`, then
**Manage**, then **Publishing**: one GitHub trusted publisher that lets the
release workflow upload the wheels with no stored credential.

**Why.** `release.yml`'s `pypi` job holds `id-token: write` and runs
`scripts/publish-pypi.sh --trusted-publishing` (issue 1243). The script signs a
PEP 740 publish attestation for each wheel with the job's identity, asks GitHub
for an OIDC token for the audience pypi.org publishes, and posts that token to
`https://pypi.org/_/oidc/mint-token`, which mints an upload token only when the
token's claims match a publisher of the project. PyPI then keeps each
attestation only when its signing identity is that same publisher, and serves it
as the wheel's provenance, which the `verify-published` job holds every wheel
to. No repository secret is involved: `make publish-pypi` with a `PYPI_TOKEN` is
the only tokened path, for an out-of-band upload, and it attests nothing.

**Current state.** Satisfied. The publisher's fields, as PyPI's integrity API
names them in the provenance of 3.3.0's wheels:

| Field       | Value               |
| ----------- | ------------------- |
| Owner       | `jmrplens`          |
| Repository  | `gitlab-mcp-server` |
| Workflow    | `release.yml`       |
| Environment | `release`           |

**How to check.** The project's Publishing page on pypi.org lists the
publisher. The provenance of any released wheel names the publisher that
uploaded it:

```bash
curl -s https://pypi.org/integrity/jmrplens-gitlab-mcp-server/3.3.0/jmrplens_gitlab_mcp_server-3.3.0-py3-none-win_amd64.whl/provenance \
  | jq '.attestation_bundles[].publisher'
```

A release rehearsal does not exercise the exchange: it runs in the
`release-rehearsal` environment, which this publisher does not name and must
not, and the script's `--dry-run` stops before the mint and signs nothing.

**What tells you it drifted.** The `pypi` job of the next release fails at the
mint, after the attestations are signed and before anything is uploaded, with
PyPI's reason (`invalid-publisher` when no publisher matches the token's
claims). A wheel uploaded through any other publisher fails `verify-published`,
which names the publisher its provenance carries.

## Documentation site response headers

**What to set.** In Cloudflare, in the `jmrp.io` zone, a response header
transform rule (**Rules**, **Transform Rules**, **Modify Response Header**)
whose expression matches the site, for example
`http.host eq "docs.jmrp.io" and starts_with(http.request.uri.path, "/gitlab-mcp-server/")`,
and which sets two headers:

```text
Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://img.shields.io https://cursor.com https://kiro.dev https://files.lmstudio.ai; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'
X-Frame-Options: DENY
```

**Why.** The OpenSSF Best Practices gold criterion hardened_site asks the
project's website for four headers with values that are not permissive:
`Content-Security-Policy`, `Strict-Transport-Security`,
`X-Content-Type-Options` as `nosniff`, and `X-Frame-Options`. The site is a
GitHub Pages build, and Pages lets a site set none of its own response headers,
so the two it lacks can only be added by Cloudflare in front of it.

The policy is written for what the built site loads, read from its 216 pages
on 2026-10-26: its own scripts, styles, search index and search worker, data:
images, and the four hosts of the install badges on the getting started page.
`'unsafe-inline'` is there because Starlight writes its theme, sidebar and tab
scripts inline, twelve distinct scripts across the site, and their hashes are
hashes of Starlight's own code, so a policy of hashes set in Cloudflare would
have to change with every Starlight release the dependency updates bring.
`'wasm-unsafe-eval'` is there because Pagefind compiles its search index from
WebAssembly. With the policy and `X-Frame-Options: DENY` injected into every
response, each of the 216 pages loaded in Chromium with no violation, the
theme switch and the search worked, and the five badge images from the four
hosts loaded; the same run with `script-src 'self'` and `img-src 'self' data:`
reported violations, the search returned nothing and the badges did not load.
A page that later loads an image or a script from
another host needs that host added here, in the rule and in
`recommendedPolicy` in `cmd/audit_site_headers/csp.go` together: the check
judges how strict the policy is, and cannot see what it blocks.

**Current state.** Not satisfied. Read on 2026-10-26, the site's responses
carry `Strict-Transport-Security: max-age=31536000; includeSubDomains; preload`
and `X-Content-Type-Options: nosniff`, and neither `Content-Security-Policy`
nor `X-Frame-Options`.

**How to check.**

```bash
make check-site-headers
curl -sI https://docs.jmrp.io/gitlab-mcp-server/ \
  | grep -i -E '^(content-security-policy|strict-transport-security|x-content-type-options|x-frame-options):'
```

`make check-site-headers` reports each header as `ok`, `pending` with the value
to set, or `FAIL`. Once the rule is in place it reports the two headers as
`notice`, served while declared pending: remove their `pending` reason from
`siteHeaders` in `cmd/audit_site_headers/headers.go` then, so that losing them
fails the check from then on. The same change updates the tests that pin
today's declarations (`TestSiteHeaders_Declarations_NameTheFourHeadersAndHoldTheirOwnValues`,
and the cases of `TestRunMain_Arguments_JudgeEachPageOrRefuse` that expect the
two as pending or as a notice), **Current state** above, and the sentence of
[the command's reference](cmd-utilities.md#audit_site_headers) that names the
two pending.

**What tells you it drifted.** The `🛡️ Check the security headers` job of
`pages.yml`, after every deploy: it fails when a header declared required is
lost or turns permissive, and warns with the value to set while one is
pending. A change in Cloudflare between deploys shows at the next deploy, or
at once with `make check-site-headers`.

## Web commit sign-off

**What to set.** Repository → Settings → General, **Require contributors to
sign off on web-based commits**, which the API calls
`web_commit_signoff_required`:

```bash
gh api -X PATCH repos/jmrplens/gitlab-mcp-server -F web_commit_signoff_required=true
```

**Why.** Every commit a pull request brings has to carry a `Signed-off-by`
trailer naming its author, the sign-off of the
[Developer Certificate of Origin](../../CONTRIBUTING.md#developer-certificate-of-origin),
and the `✍️ DCO sign-off` job of `ci.yml` fails a pull request with a commit
that does not ([`audit_dco`](cmd-utilities.md#audit_dco)). A commit made in
GitHub's web interface, an applied review suggestion or a file edited in the
browser, cannot be made with `git commit --signoff`. With this setting on,
GitHub signs such a commit off with its author's identity, as GitHub's
documentation of the setting says, so it passes the job like one made with
`git commit -s`; without it, the trailer has to be typed into the commit
message by hand. The setting reaches only commits made in the web interface:
a commit made with git still needs `--signoff`.

**Current state.** Applied on 2026-10-28, after the DCO check merged in #1562:
the repository answers `web_commit_signoff_required: true`.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server --jq '{web_commit_signoff_required}'
```

It answers `true` once the setting is on.

**What tells you it drifted.** The `✍️ DCO sign-off` job fails the first pull
request that carries a web commit without the trailer.

## Checking all of it at once

```bash
gh api repos/jmrplens/gitlab-mcp-server/releases/latest --jq '"immutable_release=\(.immutable)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets --jq '.[] | "ruleset=\(.name) target=\(.target)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets/16049728 --jq '.bypass_actors[] | "bypass=\(.actor_type)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets/16049728 --jq '.rules[] | "main_rule=\(.type) approvals=\(.parameters.required_approving_review_count) threads=\(.parameters.required_review_thread_resolution) checks=\([.parameters.required_status_checks[]?.context] | join(","))"'
gh api repos/jmrplens/gitlab-mcp-server --jq '"squash=\(.allow_squash_merge) merge_commit=\(.allow_merge_commit) rebase=\(.allow_rebase_merge)"'
gh api repos/jmrplens/gitlab-mcp-server --jq '"web_commit_signoff=\(.web_commit_signoff_required)"'
gh api repos/jmrplens/gitlab-mcp-server/environments/release --jq '"release_policy=\(.deployment_branch_policy) admins_bypass=\(.can_admins_bypass)"'
gh api repos/jmrplens/gitlab-mcp-server/environments/release/deployment-branch-policies --jq '.branch_policies[] | "release_ref=\(.type):\(.name)"'
gh api repos/jmrplens/gitlab-mcp-server/environments/release-rehearsal/secrets --jq '"rehearsal_secrets=\(.total_count)"'
make check-site-headers
```

Run these after any change to repository settings, and before a release when one
of the items above is still open.
