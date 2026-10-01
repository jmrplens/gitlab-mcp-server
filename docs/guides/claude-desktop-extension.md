# Claude Desktop Extension (.mcpb)

The server ships as a one-click [Desktop Extension](https://www.anthropic.com/engineering/desktop-extensions)
(MCPB bundle) for Claude Desktop on macOS, Windows and Linux, one bundle per
operating system. No Docker, Node.js, or Python is required.

| Your system                     | Bundle                                                                                                                                    | Download    | On disk      | Carries                                                    |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ----------- | ------------ | ---------------------------------------------------------- |
| macOS (Apple silicon and Intel) | [`gitlab-mcp-server-darwin.mcpb`](https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-darwin.mcpb)   | about 30 MB | about 115 MB | the universal binary (arm64 + amd64)                       |
| Windows (x64)                   | [`gitlab-mcp-server-windows.mcpb`](https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-windows.mcpb) | about 15 MB | about 59 MB  | the amd64 executable                                       |
| Linux (x64 and arm64)           | [`gitlab-mcp-server-linux.mcpb`](https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server-linux.mcpb)     | about 31 MB | about 125 MB | both Linux binaries and a launcher that picks between them |

Each bundle lists only its own system in its manifest, so Claude Desktop
refuses one opened on another system with a message instead of installing a
server that cannot start there. All three install as the same extension,
`gitlab-mcp-server`, and each carries the `LICENSE` it is distributed under.

[`gitlab-mcp-server.mcpb`](https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server.mcpb)
is the universal bundle: all three systems' servers in one file, about 77 MB
to download and 299 MB on disk. Every release still publishes it under that
name so that links to it keep working, and it installs as the same extension,
but the bundle for your system is the same server in a fifth to two fifths of
the download. Releases up to 3.1.0 publish only the universal bundle.

## Install

1. Download the bundle for your system from the table above, or from the
   [latest release](https://github.com/jmrplens/gitlab-mcp-server/releases/latest).
2. Open the file with Claude Desktop. On macOS and Windows, double-click it or
   drag it onto the window. On Linux, use **Extensions > Install Extension...**
   and select the file: the Linux app registers no handler for `.mcpb` files,
   so a double-click does not open it. Claude shows an install dialog with the
   extension details.
3. Fill in the settings:

| Setting                      | Required | Default              | Maps to                      |
| ---------------------------- | -------- | -------------------- | ---------------------------- |
| GitLab URL                   | Yes      | `https://gitlab.com` | `GITLAB_URL`                 |
| GitLab Personal Access Token | Yes      | —                    | `GITLAB_TOKEN`               |
| Tool surface                 | No       | `dynamic`            | `GITLAB_MCP_TOOL_SURFACE`    |
| GitLab tier                  | No       | auto-detect          | `GITLAB_MCP_TIER`            |
| Read-only mode               | No       | off                  | `GITLAB_MCP_READ_ONLY`       |
| Safe mode                    | No       | off                  | `GITLAB_MCP_SAFE_MODE`       |
| Skip TLS verification        | No       | off                  | `GITLAB_MCP_SKIP_TLS_VERIFY` |
| Log level                    | No       | `info`               | `GITLAB_MCP_LOG_LEVEL`       |

   Claude Desktop encrypts the token before saving it, with a key protected by
   the operating system: the Keychain on macOS, DPAPI under your Windows login
   on Windows, and the desktop keyring (such as GNOME Keyring or KWallet) on
   Linux. A Linux desktop without a keyring leaves the token without that
   protection.

Updates arrive as new extension versions. The server never replaces its own
binary, on any distribution channel.

### Verify the bundle you downloaded

The bundles are built outside GoReleaser, so they are not listed in
`checksums.txt`. The release workflow attests each of them on its own instead,
the universal one included, which lets you confirm the file came from this
repository's release run:

```bash
gh attestation verify gitlab-mcp-server-linux.mcpb -R jmrplens/gitlab-mcp-server
```

Use the name of the bundle you downloaded.

The release binaries are covered by `checksums.txt`, its keyless Cosign
signature, and their own provenance attestation — see
[release integrity](https://jmrp.io/docs/gitlab-mcp-server/operations/security/#verifying-release-integrity).

## Build locally

```bash
make mcpb          # builds the three per-OS bundles and the universal one in dist/ (requires macOS lipo, zip, unzip and jq)
make check-mcpb    # validates mcpb/manifest.json and the three manifests derived from it with the official CLI
```

`make mcpb` cross-compiles the darwin arm64/amd64 binaries, merges them with
`lipo`, cross-compiles the Windows amd64 binary and the Linux amd64 and arm64
binaries, and packs the four bundles with `scripts/build-mcpb.sh`. It still
needs macOS for `lipo`.

Every bundle uses the same layout and carries the entries its system needs, in
this order:

```text
manifest.json                                 every bundle
icon.png                                      every bundle
LICENSE                                       every bundle
server/gitlab-mcp-server                      macOS universal binary: darwin, universal
server/gitlab-mcp-server.exe                  Windows amd64: windows, universal
server/linux/launch.sh                        Linux launcher: linux, universal
server/linux/gitlab-mcp-server-linux-amd64    linux, universal
server/linux/gitlab-mcp-server-linux-arm64    linux, universal
```

The universal bundle packs `mcpb/manifest.json` as it is: its base command is
the macOS universal binary, with overrides for `win32` and `linux`. A per-OS
bundle packs the manifest `mcpb/platform.jq` derives from it, which lists only
that platform in `compatibility.platforms`, makes that platform's command the
base command, carries no override, and names that command's path as the
server's `entry_point`. Nothing else differs, the extension name above all, so
any of the four installs over any other.

After packing, the script reads each archive back and refuses the whole set
(deleting every bundle) unless each carries exactly its list, the launcher and
the Unix binaries are recorded as Unix files with mode 0755, and each packed
manifest agrees with its archive: every `${__dirname}` path its command, its
args and each platform override name is present, every override is keyed
`darwin`, `win32` or `linux` and listed in `compatibility.platforms`, that
list names exactly the platforms the archive carries a server for and gives
every one but the base command's an override, and no override declares `env`.
The platform rule runs both ways because a platform listed without an
override would be handed the base command, and a platform left out of the
list is one Claude Desktop marks the bundle incompatible on. The `env` rule
exists because Claude Desktop applies an override's `env` in place of the base
`env` rather than merging the two, so an override carrying one would start the
server without `GITLAB_TOKEN`. A check that fails, or a step that stops the
script before the checks finish, leaves no bundle behind.

The script also prints each bundle's download and unpacked size, and writes
them to the job summary in CI. It fails on Claude Desktop's own limits, read
from its extension runtime (2.7032.0): an entry over 512 MiB, more than
2048 MiB unpacked, an archive that unpacks to more than 50 times its own size,
or more than 100,000 entries, since Claude Desktop refuses to install such a
bundle. It warns, and does not fail, when a per-OS bundle passes 50 MiB to
download or 256 MiB unpacked: directories that score MCP servers stop reading
a bundle past those sizes and then report its provenance, licence and
maintenance as unverified, which is what happened to the single bundle 3.1.0
shipped. The universal bundle is over both by design and declared nowhere a
directory reads, so it is measured and not warned about.

### How the Linux entry starts

Manifest overrides are chosen by operating system only, so the `linux`
override cannot name a binary per architecture. It runs
`/bin/sh ${__dirname}/server/linux/launch.sh` instead, and the launcher
(`mcpb/linux/launch.sh`, POSIX `sh`):

- picks `gitlab-mcp-server-linux-amd64` for `x86_64`/`amd64` and
  `gitlab-mcp-server-linux-arm64` for `aarch64`/`arm64` from `uname -m`, and
  refuses any other machine type with a message on stderr;
- finds the binary next to itself, never through the working directory, and
  quotes every path, since the extension directory sits under
  `Claude Extensions/` in Claude Desktop's data directory (`~/.config/Claude`
  by default) and so contains a space;
- sets the owner execute bit if the binary lacks it;
- runs it with `exec`, so the process Claude Desktop spawned becomes the server
  and the stop signals Claude Desktop sends to that PID reach it;
- writes nothing to stdout, which carries the server's JSON-RPC.

Because `/bin/sh` reads the launcher, the launcher itself needs no execute
bit. Claude Desktop's Linux build (read from 2.7032.0) extracts every file
with mode 0600 and restores 0700 only on entries whose recorded mode has the
owner execute bit, which is why the build records 0755 on the binaries; the
launcher's own `chmod` covers an archive that lost those bits.
`scripts/mcpb_launch_sh_test.py` drives the launcher with stub binaries under
`/bin/sh`, `dash`, `busybox sh` and `bash --posix`, whichever the machine has.
A busybox built with standalone applets, like Ubuntu's `busybox-static` on
GitHub's runner, runs its own `uname` and `chmod` whatever `PATH` says, so under
it the cases that need a stub check the binary for the real machine instead, and
the two that cannot (an unsupported machine type, a failing `chmod`) are skipped
with the reason.
`scripts/mcpb_manifest_test.py` pins the manifest values this layout depends
on and what `mcpb/platform.jq` derives from them, and
`scripts/build_mcpb_sh_test.py` runs the build script over stand-in binaries,
checks that it refuses, and removes, each bundle its rules and size limits
exist for, and checks that each server entry of each bundle is packed from the
right file of both the `dist/` that `make mcpb` leaves and the one the release
job builds from.
All three run in CI's supply-chain job. On a machine without `shellcheck`, or
without the `bash`, `jq`, `zip` and `unzip` the build script needs, the cases
that need them skip; in CI, where GitHub sets `CI`, they fail instead, so the
job cannot pass with them unrun.

A `.mcpb` is a plain zip with `manifest.json` at its root, so the script builds
it with `zip` and fixed entry timestamps: the same inputs produce the same
bytes, which matters because `server.json` records each declared bundle's
SHA-256. It used to shell out to `npx --yes @anthropic-ai/mcpb@<pin>`, which
pinned the CLI but resolved its dependency tree fresh from the registry on
every release, inside the job that holds this project's signing and
publishing identities.

In CI, the release workflow builds the four bundles from the GoReleaser
artifacts (including the `universal_binaries` darwin build), uploads them as
release assets and attests each one. `scripts/update-server-json-sha.sh`
declares the three per-OS bundles in `server.json`, one `mcpb` entry each with
its own identifier and SHA-256, and refuses a set that does not serve `darwin`,
`win32` and `linux` exactly once: a registry entry has no platform field, so
the universal bundle beside them would make each system installable two ways
with nothing to choose by. The same script stamps the manifest version from
the git tag, the flow that versions `server.json`.

## Files

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

## Privacy and directory submission

The manifest's `privacy_policies` points to [PRIVACY.md](../../PRIVACY.md) and
the [GitLab Privacy Statement](https://about.gitlab.com/privacy/). Directory
submissions for desktop extensions go through Anthropic's
[submission form](https://claude.com/docs/connectors/building/submission),
which requires the documentation URL, the privacy policy, the icon, and test
credentials.
