# Governance

This document says how gitlab-mcp-server is run: who decides, how a decision is made and how a disagreement is settled, who holds which role, who has access to what, and what the project depends on to continue. [CONTRIBUTING.md](CONTRIBUTING.md) covers how a change is proposed and reviewed, [SECURITY.md](SECURITY.md) how a vulnerability is reported and handled, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) the behavior expected in every space the project uses.

## Model

The project has a single maintainer, who makes the final decision on every change, every release and every question of direction. There is no committee, no vote and no company behind it: it is an independent project, not affiliated with GitLab Inc. Who holds the role today is in [Roles and responsibilities](#roles-and-responsibilities).

## Decisions

Decisions are made in public, where anyone can read them and comment before they land:

- A change is proposed as an issue or a pull request on GitHub. An issue says what is wrong or missing and what done looks like; a pull request says what changed and how it was checked.
- A decision about the architecture, a boundary of what the server does, or a rule later work has to follow is recorded as an architecture decision record in [`docs/development/adr`](docs/development/adr/README.md), with its context, the decision and its consequences. A later record supersedes or refines an accepted one; the accepted one is not rewritten to say something else.
- Where the project is going is written on the [roadmap](https://docs.jmrp.io/gitlab-mcp-server/roadmap/), and the issues planned for the next release are on its open [milestone](https://github.com/jmrplens/gitlab-mcp-server/milestones).
- A setting that lives outside the files of the repository, such as a branch rule or a registry's trusted publisher, is recorded in [`docs/development/repository-settings.md`](docs/development/repository-settings.md), with how to check it.

The one exception is a vulnerability, which is reported and fixed in private, as [SECURITY.md](SECURITY.md) describes, until its advisory is published.

The maintainer decides each issue and pull request. An issue is placed on a milestone, sent back to its author for what it is missing, or closed with the reason. A pull request is merged once it meets [Code review](CONTRIBUTING.md#code-review), or closed with the reason.

## Disagreements

A disagreement is raised where it arises, in the issue or the pull request, with what supports it: a failing test, GitLab's documentation, a measurement, a reproduction. The maintainer answers in the same thread with the decision and the reason for it. A decision is reopened by new evidence, in an issue that says what changed since it was made. The [Code of Conduct](CODE_OF_CONDUCT.md) applies to the discussion, however strongly it is felt.

## Roles and responsibilities

| Role              | Held today by                                                    | Responsibilities                                                                                                                                                                                                                                                                                                                                                                                                                          |
| ----------------- | ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Maintainer        | José M. Requena Plens ([@jmrplens](https://github.com/jmrplens)) | Triage every issue; review every pull request a contributor opens; merge each pull request that meets Code review; cut each release (the written summary, the signed tag, and watching every channel publish); handle each vulnerability report as SECURITY.md sets out; enforce the Code of Conduct; keep dependencies current; hold the access and the accounts listed under [Continuity](#continuity) and keep their settings recorded |
| Contributor       | Anyone who opens an issue, comments or sends a pull request      | Follow CONTRIBUTING.md and the Code of Conduct; sign off every commit, which certifies the Developer Certificate of Origin for it; answer every review thread on their pull request, with a change or the reason for none; keep its checks passing                                                                                                                                                                                        |
| Security reporter | Anyone who reports a vulnerability                               | Report it privately as SECURITY.md describes, and keep it private for as long as SECURITY.md sets out                                                                                                                                                                                                                                                                                                                                     |

Automation is not a role. Dependabot, the automated code reviewers and the workflows act within the permissions the repository gives them: their pull requests and review comments are read and answered like anyone's, and none of them merges a pull request. The one write to `main` that no pull request carries is the version stamp the release workflow pushes with a deploy key after a release, which [`docs/development/repository-settings.md`](docs/development/repository-settings.md#sc-01-keep-the-deploy-key-ruleset-bypass-declared) records. A fix developed in a security advisory's temporary private fork reaches `main` from a pull request of that fork, merged from the advisory, and GitHub runs no check on it there, as [SECURITY.md](SECURITY.md#coordinated-disclosure) describes.

## Access and two-factor authentication

Collaborator access to the repository, and access to its private vulnerability reports beyond a reporter's access to their own report, is granted only to an account with two-factor authentication enabled through an authenticator app, a security key or a passkey. SMS alone does not qualify. The repository belongs to a personal GitHub account, where GitHub cannot require two-factor authentication of collaborators the way an organization can, and does not show the owner whether a collaborator has it or by which method. So the rule is applied when access is granted: the maintainer asks the account's holder to confirm the method first, and withdraws the access on learning that the account no longer meets the rule.

Today the maintainer's account is the only one with any of that access: it is the repository's one collaborator, with the admin role, and it has two-factor authentication enabled. Besides that account, one deploy key, added by the maintainer, has write access to the repository: the release workflow pushes the version stamp with it.

Three repository rules limit what that access can do:

- **`main`** takes a change through a pull request whose review threads are all resolved and whose required `✅ CI` check passed, merged as a squash, the one merge method the repository allows. No approval count is required while the project has one maintainer. Nothing can push to `main` directly except the release workflow's version stamp. The rule's bypass list holds the repository admin role for pull requests only, so the maintainer cannot push to `main` and can pass over the rule only when merging a pull request, and deploy keys always, so that the release workflow can push the stamp with the repository's one deploy key: the stamp is a commit of the version and digests the release has just published, made in a job that checks out `main` afresh and runs no third-party code, and sending it through a pull request would leave every release waiting on a second human action. A workflow's own token, Dependabot and every other app are held to the rule like a person.
- **Release tags** (`v*`) are created by the repository admin role alone, and no tag can be moved or deleted by anyone. Every release tag since v2.7.0 is signed by the maintainer.
- **The `release` environment**, which holds the identity npm, PyPI and NuGet accept a release from, deploys only from a `v*` tag, with no administrator bypass.

How each rule is set and checked is in [`docs/development/repository-settings.md`](docs/development/repository-settings.md).

## Secrets and credentials

The project's automation holds the credentials below, each as a GitHub Actions secret of this repository. Publishing to npm, PyPI, nuget.org and the MCP Registry needs none, since each trusts the release workflow's OIDC identity, and the image reaches GHCR with the job's own token.

| Secret                                  | What it is                                                                                                                                         | Read by                                                                                    |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` | The Docker Hub account and an access token that pushes the image and its description                                                               | The `docker` and `sign-attest` jobs of `release.yml`                                       |
| `RELEASE_DEPLOY_KEY_B64`                | The private half of `release-manifest-bot`, the repository's one deploy key, which pushes the version stamp to `main`                              | The `commit-manifests` job of `release.yml`, from the copy the `release` environment holds |
| `TAP_DEPLOY_KEY_B64`                    | The private half of a deploy key with write access to `jmrplens/homebrew-tap`                                                                      | The `homebrew` job of `release.yml`                                                        |
| `WINGET_TOKEN`                          | A GitHub token of the maintainer's that pushes branches to the `jmrplens/winget-pkgs` fork and opens pull requests against `microsoft/winget-pkgs` | The `winget` job of `release.yml`                                                          |
| `GITLAB_MIRROR_SSH_KEY`                 | An SSH key that pushes every branch and tag to the GitLab.com mirror                                                                               | The `repository` job of `gitlab-mirror.yml`                                                |
| `GITLAB_API_TOKEN`                      | A GitLab.com token that creates the mirror's releases and their links                                                                              | The `releases` job of `gitlab-mirror.yml`                                                  |
| `SONAR_TOKEN`                           | A SonarCloud token that publishes the analyses                                                                                                     | The `sonarcloud` job of `ci.yml`                                                           |
| `INDEXNOW_KEY`                          | The IndexNow key, which the site also publishes at its root, so it proves nothing a reader of the site lacks                                       | The `indexnow` job of `pages.yml`                                                          |

The key that signs release tags is not among them: tags are signed on the maintainer's machine, and the key never reaches a workflow. The tokens an end-to-end run against a self-hosted GitLab or GitLab.com uses are the maintainer's, kept in the git-ignored `.env` file of the machine that runs it; the end-to-end runs in CI boot a GitLab of their own and hold no GitLab credential.

- **Storage.** A secret is kept as an encrypted GitHub Actions secret and nowhere in the tree. Once set, GitHub shows its value to no one, through its interface or its API; it hands it to a job that names it and masks it in that job's log. The accounts that issue them are in the maintainer's password manager, as [Continuity](#continuity) says, so a lost secret is issued anew rather than recovered.
- **Access.** Only the repository admin role, which the maintainer alone holds, can set, replace or delete a secret. A secret reaches a workflow run of this repository and no run for a pull request from a fork or one Dependabot starts, and only an account with write access, again the maintainer's alone, can push a workflow that names it. The `release` environment, whose copy of `RELEASE_DEPLOY_KEY_B64` is the one the release reads, deploys only from a `v*` tag, which only the admin role can create; every other secret, the repository's own copy of that key included, is a repository secret the environment does not bind ([Repository Settings](docs/development/repository-settings.md#sc-04-deploy-the-release-environment-from-release-tags-only)).
- **Use.** A job reads a secret in the step that needs it, through `env:` or an action's input and never on a command line, and writes a key to a file only its user can read (mode 0600) before using it. No workflow prints a secret, and a rehearsal of the release is given neither deploy key. Every secret a workflow reads is declared in `cmd/audit_supply_chain/secrets.go` with what it can write, and `make check-supply-chain` holds a job that reads one that can write to the rules of a credentialed job: no code resolved at run time.
- **Never committed.** No credential is written into the tree, a fixture, a log or a commit; test fixtures carry invented values. Secret scanning with push protection refuses a push that carries a secret GitHub recognizes, GitGuardian checks every pull request, and `.gitignore` keeps out the `.env` files and exported OpenPGP secret keys. Secret scanning has raised no alert on the repository.
- **Rotation.** There is no fixed schedule. A secret is replaced before the credential behind it expires at its issuer; at once when it may have been exposed, because it appeared anywhere outside the secret or an account or a machine that held it was compromised; when what it must be allowed to do changes; and when a maintainer joins or leaves. To replace one, issue the new credential where it comes from, set it with `gh secret set` (with `--env release` for the environment's copy), run the job that reads it or rehearse the release, and only then revoke the old one.
- **Revocation.** A credential is revoked where it was issued (a deploy key in the settings of the repository it opens, a token on Docker Hub, GitHub, GitLab.com or SonarCloud), not only deleted here, since deleting the secret leaves any copy taken of it valid. A secret found committed is treated as exposed and revoked and replaced at once: rewriting the history does not withdraw a copy already fetched.

## Continuity

The project has to be able to create and close issues, accept changes and publish a release even if any one person stops supporting it. This section records what that takes and where the project stands.

What does not depend on any one person:

- The source, the history, the issues and the pull requests are public, and the code is MIT licensed.
- Everything needed to build, test and release is in the repository and documented: [`docs/development/release-process.md`](docs/development/release-process.md), [`docs/development/distribution.md`](docs/development/distribution.md) and [`docs/development/repository-settings.md`](docs/development/repository-settings.md).
- npm, PyPI and NuGet accept releases through trusted publishers bound to this repository's release workflow, and the MCP Registry through the workflow's GitHub identity, so publishing to them needs no token a person holds: whoever controls the repository controls those publishes.

What depends on the maintainer today:

| What                                                                                   | What it is needed for                                                                 |
| -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| The GitHub account that owns the repository, its admin role and its settings           | Closing issues, merging pull requests, creating release tags                          |
| The key release tags are signed with                                                   | Signed release tags                                                                   |
| The Docker Hub account and the token the release workflow uses for it                  | The image on Docker Hub (the GHCR image is pushed with the workflow's own token)      |
| The npm, PyPI and NuGet accounts that own the packages                                 | Managing the trusted publishers and the package owners                                |
| The Homebrew tap repository and the winget fork, with the credentials the release uses | The Homebrew formula and the winget manifest                                          |
| The `jmrp.io` domain, and its DNS at Cloudflare                                        | The documentation site (`docs.jmrp.io`) and the hosted endpoint                       |
| The SonarCloud project and the GitLab.com mirror, with their credentials               | The quality gate and the mirror                                                       |
| The LobeHub Marketplace account, connected to the maintainer's GitHub account          | The LobeHub listing, updated by hand after each release with `make publish-lobehub`   |
| The `jmrplens/mcp.jmrp.io` repository and the host that serves it                      | The hosted endpoint (`mcp.jmrp.io/gitlab`), deployed out of band from this repository |
| The bestpractices.dev entry, project 15879                                             | The OpenSSF Best Practices badge and the answers behind it                            |

Each item above is held in the maintainer's name, and the repository has no other collaborator, so the project's bus factor is 1.

The keys are in a lockbox. A person the maintainer trusts has emergency access to the maintainer's password manager, which holds every credential and key the project depends on: the GitHub account that owns the repository, with its two-factor recovery; the npm, PyPI, NuGet and Docker Hub accounts, the Homebrew tap and the `io.github.jmrplens` namespace of the MCP Registry; Cloudflare and the `jmrp.io` domain; SonarCloud; and the key that signs release tags, with its passphrase. If the maintainer cannot continue, that access lets the project create and close issues, accept changes and publish a release within a week, following [`docs/development/release-process.md`](docs/development/release-process.md). Neither the person nor any secret is named here.

## When a second maintainer joins

A contributor who has made significant contributions over time and reviewed other people's changes may be invited to become a maintainer. The invitation and its acceptance are a pull request that adds the new maintainer to [Roles and responsibilities](#roles-and-responsibilities), with the roles they take, and access is granted under [Access and two-factor authentication](#access-and-two-factor-authentication).

With two maintainers:

- A maintainer's pull request is reviewed by the other maintainer before it is merged.
- Decisions are made by agreement between the maintainers, in the issue or pull request. When they cannot agree, the maintainer who founded the project decides and says why in the same thread.
- Each item under [Continuity](#continuity) is reachable by both. A repository under a personal account has only one administrator, and only that role creates release tags, so sharing release access means moving the repository out of the personal account or letting a second role create release tags.

This document is changed by a pull request, like any other file in the repository.
