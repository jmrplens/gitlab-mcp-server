# Distribution

**How the release artifacts are built, checked and published**: the Claude
Desktop bundles in detail, and where each other channel is assembled,
validated and pushed.

> **Diátaxis type**: Reference & Explanation · **Audience**: 🛠️ Contributors & maintainers

What a user installs, and how they verify it, is on the documentation site:
[Installation](https://jmrp.io/docs/gitlab-mcp-server/install/overview/), one
page per channel, and the
[Claude Desktop Extension](https://jmrp.io/docs/gitlab-mcp-server/claude-desktop/)
page. This page is the other half: what produces those artifacts and what
refuses a bad one. The release workflow's own sequence (draft, attest,
publish, read back) is described in the Release process section of
[CLAUDE.md](../../CLAUDE.md), and the settings it depends on that live on
GitHub rather than in the tree are in
[Repository Settings](repository-settings.md).

---

## The Claude Desktop bundles

### Build locally

```bash
make mcpb          # the three per-OS bundles and the universal one, in dist/
make check-mcpb    # validates mcpb/manifest.json and the three manifests derived from it with the official MCPB CLI
```

`make mcpb` needs macOS, because it merges the two darwin binaries with
`lipo` and stops at once without it; `scripts/build-mcpb.sh` then needs `jq`,
`zip` and `unzip`. The target cross-compiles the darwin arm64 and amd64
binaries and merges them, cross-compiles the Windows amd64 binary and the
Linux amd64 and arm64 binaries, each into a `dist/local_<goos>_<goarch>/`
directory, writes their `dist/THIRD_PARTY_NOTICES` with
`cmd/gen_third_party_notices` (from the module cache), and packs the four
bundles with `scripts/build-mcpb.sh`.

`make check-mcpb` runs `npx @anthropic-ai/mcpb@$(MCPB_CLI_VERSION) validate`
over `mcpb/manifest.json`, then over the manifest `mcpb/platform.jq` derives
for each of `darwin`, `win32` and `linux`, each in a temporary directory with
a copy of the icon, since the CLI looks for the icon beside the manifest it
validates. CI runs it in the Markdown job.

### What a bundle carries

Every bundle uses the same layout and carries the entries its system needs,
in this order:

```text
manifest.json                                 every bundle
icon.png                                      every bundle
LICENSE                                       every bundle
THIRD_PARTY_NOTICES                           every bundle
server/gitlab-mcp-server                      macOS universal binary: darwin, universal
server/gitlab-mcp-server.exe                  Windows amd64: windows, universal
server/linux/launch.sh                        Linux launcher: linux, universal
server/linux/gitlab-mcp-server-linux-amd64    linux, universal
server/linux/gitlab-mcp-server-linux-arm64    linux, universal
```

`manifest.json` comes first, so a reader that streams the archive finds it
before the binaries. The entries are named to `zip` one by one rather than
recursed into, so their order is the list's and not the filesystem's.

The script finds each binary under the distribution directory by pattern
(`*darwin_all*`, `*windows_amd64*`, `*linux_amd64*`, `*linux_arm64*`), which
matches both the `dist/<id>_<goos>_<goarch>[_<variant>]/` directories
GoReleaser writes and the `dist/local_<goos>_<goarch>/` ones `make mcpb`
writes. Exactly one match is accepted for each: with two, which one `find`
lists first is up to the filesystem, so the script names them and stops.

### The per-OS manifests

The universal bundle packs `mcpb/manifest.json` as it is: its base command is
the macOS universal binary, with overrides for `win32` and `linux`. A per-OS
bundle packs the manifest `mcpb/platform.jq` derives from it, which lists only
that platform in `compatibility.platforms`, makes that platform's command the
base command, carries no override, and names as the server's `entry_point` the
first path that command line names inside the bundle (the binary, or the
launcher `/bin/sh` runs). Nothing else differs, the extension name above all,
so any of the four installs over any other. The script stamps the packed
manifest's `version` with the version it was given.

### What the build refuses

The previous run's bundles are removed before anything else, and until every
check has passed any exit removes this run's too, so a refused set never
leaves a bundle behind for a later step or a developer to take for this
version. They go as a set: a release declares three of them, and a later step
handed two would publish a registry entry short of a system.

After packing, the script reads each archive back and refuses the whole set
unless:

- each archive carries exactly its list of entries, in order;
- the launcher and the Unix binaries are recorded as Unix files with mode
  `0755` (`-rwxr-xr-x unx`). Every mode is set rather than inherited, since
  the zip records it and a mode that followed the builder's umask would make
  the same inputs pack differently;
- each packed manifest agrees with its archive: its `version` is the one
  given, the server's `entry_point` and the `icon` are in the archive, and every
  `${__dirname}` path that its command, its arguments and each platform
  override name is present;
- every override is keyed `darwin`, `win32` or `linux` and listed in
  `compatibility.platforms`, and every listed platform but the one the base
  command serves has an override;
- `compatibility.platforms` names exactly the platforms the archive carries a
  server for;
- no override declares `env`, and none declares `args` when the base `args`
  are not empty.

The platform rules run both ways because a platform listed without an
override would be handed the base command, the macOS binary in the universal
bundle, and a platform left out of the list is one Claude Desktop marks the
bundle incompatible on. The `env` rule exists because Claude Desktop applies
an override's `env` in place of the base one rather than merging the two, so
an override carrying one would start the server without `GITLAB_TOKEN`; an
override's `args` replace the base `args` the same way.

### Size limits

The script prints each bundle's download size, unpacked size, entry count,
largest entry and compression ratio, and in GitHub Actions writes the sizes to
the job summary. It fails on Claude Desktop's own limits, read from its
extension runtime (2.7032.0), since Claude Desktop refuses to install a bundle
past any of them:

| Limit                       | Value    |
| --------------------------- | -------- |
| Entries in the archive      | 100,000  |
| One entry, unpacked         | 512 MiB  |
| The whole archive, unpacked | 2048 MiB |
| Unpacked size over archive  | 50 times |

It warns, and does not fail, when a per-OS bundle passes 50 MiB to download or
256 MiB unpacked: directories that score MCP servers stop reading a bundle
past those sizes and then report its provenance, licence and maintenance as
unverified, which is what happened to the single bundle 3.1.0 shipped. The
universal bundle is over both by design and declared nowhere a directory
reads, so it is measured and not warned about.

### How the Linux entry is held to its rules

What the launcher does, and why it runs through `/bin/sh` and ends in `exec`,
is on the site under
[How the Linux entry starts](https://jmrp.io/docs/gitlab-mcp-server/claude-desktop/#how-the-linux-entry-starts).
Two details belong here. The launcher (`mcpb/linux/launch.sh`) exits `127`
when the binary for the machine is missing from the extension and `126` when
it is not executable and `chmod` cannot make it so. And Claude Desktop's Linux
build (read from 2.7032.0) extracts every file with mode `0600` and restores
`0700` only on entries whose recorded mode has the owner execute bit, which is
why the build records `0755` on the binaries; the launcher's own `chmod`
covers an archive that lost those bits.

### Tests

| Script                           | What it holds                                                                                                                                                                                                        |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `scripts/mcpb_launch_sh_test.py` | Drives the launcher with stub binaries under `/bin/sh`, `dash`, `busybox sh` and `bash --posix`, whichever the machine has, and runs `shellcheck` on it                                                              |
| `scripts/mcpb_manifest_test.py`  | Pins the manifest values the bundle layout depends on, and what `mcpb/platform.jq` derives from them                                                                                                                 |
| `scripts/build_mcpb_sh_test.py`  | Runs the build script over stand-in binaries, checks that it refuses, and removes, each bundle its rules and size limits exist for, and that each server entry is packed from the right file of both `dist/` layouts |

A busybox built with standalone applets, like the `busybox-static` on GitHub's
runner, runs its own `uname` and `chmod` whatever `PATH` says, so under it the
cases that need a stub check the binary for the real machine instead, and the
two that cannot (an unsupported machine type, a failing `chmod`) are skipped
with the reason.

All three run in CI's supply-chain job, in the step that runs every
`scripts/*_test.py` (`python3 -m unittest discover -s scripts -p "*_test.py"`).
On a machine without `shellcheck`, or without the `bash`, `jq`, `zip` and
`unzip` the build script needs, the cases that need them skip; in CI, where
GitHub sets `CI`, they fail instead, so the job cannot pass with them unrun.

### Why `zip`, with fixed timestamps

A `.mcpb` is a plain zip with `manifest.json` at its root, so the script
builds it with `zip` and sets every entry's timestamp to the zip epoch
(`touch -t 198001010000`, a form both GNU and BSD `touch` accept, since
`make mcpb` runs on macOS). The same inputs produce the same bytes, which
matters because `server.json` records each declared bundle's SHA-256, and a
hash that changes because the clock moved tells a verifier nothing.

It used to shell out to `npx --yes @anthropic-ai/mcpb@<pin>`, which pinned the
CLI but resolved its dependency tree fresh from the registry on every release,
inside the job that holds this project's signing and publishing identities.
The pinned CLI (`MCPB_CLI_VERSION` in the Makefile) is now run only by
`make check-mcpb`, which validates and packs nothing.

### In a release

The release job builds the four bundles from the GoReleaser artifacts (the
`universal_binaries` darwin build included) with `scripts/build-mcpb.sh`,
uploads them as release assets and attests each one with
`actions/attest-build-provenance`, the universal one included. A rehearsal
builds them and uploads nothing. They are built before the stamp because
GoReleaser does not produce them, so they are absent from the signed
`checksums.txt`, and appending them there would invalidate the Cosign
signature over it.

`scripts/update-server-json-sha.sh` takes the bundles to declare as its third
argument, comma-separated: in a release, the three per-OS ones. It writes one
`mcpb` entry per bundle in `server.json`, each with its own identifier and a
`fileSha256` hashed from the file, and refuses, before writing anything, a set
that does not serve `darwin`, `win32` and `linux` exactly once: a registry
entry has no platform field, so the universal bundle beside them would make
each system installable two ways with nothing to choose by. The same script
stamps the version of `mcpb/manifest.json`, the flow that versions
`server.json`, and the stamped manifests are committed back to `main`.

### Files

| Path                             | Purpose                                                                                             |
| -------------------------------- | --------------------------------------------------------------------------------------------------- |
| `mcpb/manifest.json`             | MCPB manifest (source of truth; version stamped per release)                                        |
| `mcpb/platform.jq`               | Derives the manifest of each per-OS bundle from `mcpb/manifest.json`                                |
| `mcpb/icon.png`                  | 512×512 icon rendered from `site/public/favicon.svg` by `make brand-rasters`                        |
| `mcpb/linux/launch.sh`           | Linux entry point: picks the amd64 or arm64 binary by `uname -m`                                    |
| `scripts/build-mcpb.sh`          | Bundle assembly, deterministic `zip` packing, the size report, and the checks on each archive       |
| `scripts/mcpb_launch_sh_test.py` | Tests of the Linux launcher, run in CI                                                              |
| `scripts/mcpb_manifest_test.py`  | Tests of the manifest values the bundle layout depends on and of their per-OS derivation, run in CI |
| `scripts/build_mcpb_sh_test.py`  | Tests of the build script's checks, its size limits and its removal of a refused set                |
| `PRIVACY.md`                     | Privacy policy referenced by the manifest                                                           |

### Privacy policy and directory submission

The manifest's `privacy_policies` names [PRIVACY.md](../../PRIVACY.md), by its
URL on GitHub, and the [GitLab Privacy Statement](https://about.gitlab.com/privacy/).
Directory submissions for desktop extensions go through Anthropic's
[submission form](https://claude.com/docs/connectors/building/submission),
which requires the documentation URL, the privacy policy, the icon, and test
credentials.

---

## The other channels

Each channel publishes from a job of its own in
`.github/workflows/release.yml`. The npm, PyPI and NuGet jobs take the
binaries the release already published, check them against the signed
`checksums.txt`, assemble their packages from them and validate what they
built before anything is pushed; the Homebrew formula and the winget
manifests point at the release assets themselves. The `make` targets run the
same scripts by hand, and each package target takes the directory of release
binaries as a variable (`NPM_BINARIES`, `PYPI_BINARIES`, `NUGET_BINARIES`).

| Channel  | Assemble                                    | Validate                                                                                                  | Publish                                                                                       | Release job    |
| -------- | ------------------------------------------- | --------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | -------------- |
| npm      | `make gen-npm` (`scripts/build-npm.mjs`)    | `make validate-npm` in a `node:22` container, `make validate-npm-local` on a runner                       | `scripts/publish-npm.sh`; `make publish-npm-dry` and `make publish-npm` by hand               | `npm`          |
| PyPI     | `make gen-pypi` (`scripts/build_pypi.py`)   | `make validate-pypi` in a `python:3.14-slim` container, `make validate-pypi-local` on a runner            | `pypa/gh-action-pypi-publish`; `make publish-pypi-dry` and `make publish-pypi` by hand        | `pypi`         |
| NuGet    | `make gen-nuget` (`scripts/build_nuget.py`) | `make validate-nuget` in the .NET SDK container pinned by digest, `make validate-nuget-local` on a runner | `scripts/publish-nuget.sh`; `make publish-nuget-dry` and `make publish-nuget` by hand         | `nuget`        |
| Homebrew | `scripts/update-homebrew-tap.sh`            | The script's `--dry-run`, in a rehearsal                                                                  | The same script pushes the formula to `jmrplens/homebrew-tap`                                 | `homebrew`     |
| winget   | `vedantmgoyal9/winget-releaser`             | Microsoft's validation of the pull request                                                                | A version pull request to `microsoft/winget-pkgs` through the `jmrplens/winget-pkgs` fork     | `winget`       |
| Registry | `scripts/update-server-json-sha.sh`         | `make check-server-json`; `make check-server-json-packages` downloads every declared artifact             | `mcp-publisher publish`                                                                       | `mcp-registry` |
| LobeHub  | `make gen-lhm-manifest`                     | `make check-lhm-manifest`                                                                                 | `make publish-lobehub`, by hand from a machine with `lhm login` and `lhm github connect` done | none           |

`scripts/verify_published_packages.py` (the `verify-published` job) reads the
npm, PyPI and NuGet packages back out of their registries and compares the
binaries inside them with the signed `checksums.txt`, and the Homebrew tap,
the winget pull request, the MCP Registry entry and the manifest commit to
`main` all wait on it. It runs minutes after the pushes, so a version a
registry does not serve yet is waited for rather than reported, from two
allowances. npm and PyPI share a budget of seconds slept between attempts
(`--retry-budget`, 480 in the release job). NuGet has a deadline of its own
(`--nuget-deadline`, 40 minutes), counted from the start of the run, because
nuget.org validates every push before it serves it, and does so while the
other two are waited for: on 3.1.0 the last package was served 23 minutes
after the step started, which one shared budget of eight minutes could not
cover. The pointer's index lists the versions already published until the
new one is served, so an index that does not list it yet is waited for too.
The job's timeout (50 minutes) is sized to outlast the deadline and the
downloads after it. A digest that does not match is reported at once,
whatever is left to wait, and `--retry-budget 0`, the out-of-band run days
later, waits for nothing.

Two details that the per-channel pages on the site do not carry:

- **The PyPI distribution name is a constant.** The unprefixed
  `gitlab-mcp-server` name on PyPI is held by an unrelated account under a
  PEP 541 reclamation request. If it is reclaimed, the rename is `DIST_NAME`
  in `scripts/build_pypi.py` and `scripts/validate_pypi.py`, the PyPI
  identifier in `server.json`, and the documentation.
- **The Agent Plugins manifests are checked by `make check-openplugin`**
  (`scripts/check-openplugin.sh`), which validates the root `plugin.json` and
  `mcp.json` and the legacy Open Plugins manifest `.plugin/plugin.json`. CI
  runs it in the generated-artifacts job, beside `make check-server-json`;
  `make check-server-json-packages` runs only on a push to `main`, since it
  downloads every declared artifact.
