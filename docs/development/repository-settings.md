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
That trade is not worth taking for two items that change roughly never, so the
checks below are run by hand.

The one exception is release immutability, which is visible in the public
release metadata and is already checked (see below).

## SC-04: enable immutable releases

**What to set.** Repository → Settings → General → Releases → enable
**Immutable releases**.

**Why.** Every other artifact this project publishes is pinned by a hash or by
an immutable registry version: the `.mcpb` bundle by `fileSha256` in
`server.json`, the container image by `@sha256:` digest, npm and PyPI by version
(both registries forbid republishing one). The GitHub Release is the exception.
Without this setting, anyone holding `contents: write` can replace a published
binary **and** `checksums.txt` together, and both installers accept the
consistent pair. The cosign signature over `checksums.txt` is what travels to a
verifier who checks it; this setting is what stops the substitution happening at
the source.

**How to check.**

```bash
gh api repos/jmrplens/gitlab-mcp-server/releases/latest --jq '{tag: .tag_name, immutable}'
```

**Current state.** Not enabled. `v2.7.5` and every release before it report
`"immutable": false`.

**Caveat.** The setting is not retroactive. Releases published before it is
turned on stay mutable forever, so enabling it protects future releases only.
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

## Checking all of it at once

```bash
gh api repos/jmrplens/gitlab-mcp-server/releases/latest --jq '"immutable_release=\(.immutable)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets --jq '.[] | "ruleset=\(.name) target=\(.target)"'
gh api repos/jmrplens/gitlab-mcp-server/rulesets/16049728 --jq '.bypass_actors[] | "bypass=\(.actor_type)"'
```

Run these after any change to repository settings, and before a release when one
of the items above is still open.
