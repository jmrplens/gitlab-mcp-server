# Claude Desktop Extension (.mcpb)

The server ships as a one-click [Desktop Extension](https://www.anthropic.com/engineering/desktop-extensions)
(MCPB bundle) for Claude Desktop on macOS, Windows and Linux. The bundle
contains a macOS universal binary (arm64 + amd64), a Windows amd64 executable,
and the Linux amd64 and arm64 binaries with a launcher that picks between
them. No Docker, Node.js, or Python is required. Carrying four binaries makes
the download about 75 MB.

## Install

1. Download `gitlab-mcp-server.mcpb` from the
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

   Claude Desktop encrypts the token before saving it, with a key held by the
   operating system's credential store: the Keychain on macOS, DPAPI on
   Windows, and the desktop keyring (such as GNOME Keyring or KWallet) on
   Linux. A Linux desktop without a keyring leaves the token without that
   protection.

Updates arrive as new extension versions. The server never replaces its own
binary, on any distribution channel.

### Verify the bundle you downloaded

The `.mcpb` is built outside GoReleaser, so it is not listed in `checksums.txt`.
The release workflow attests it on its own instead, which lets you confirm the
file came from this repository's release run:

```bash
gh attestation verify gitlab-mcp-server.mcpb -R jmrplens/gitlab-mcp-server
```

The release binaries are covered by `checksums.txt`, its keyless Cosign
signature, and their own provenance attestation — see
[release integrity](https://jmrp.io/docs/gitlab-mcp-server/operations/security/#verifying-release-integrity).

## Build locally

```bash
make mcpb          # builds dist/gitlab-mcp-server.mcpb (requires macOS lipo, zip, unzip and jq)
make check-mcpb    # validates mcpb/manifest.json with the official CLI
```

`make mcpb` cross-compiles the darwin arm64/amd64 binaries, merges them with
`lipo`, cross-compiles the Windows amd64 binary and the Linux amd64 and arm64
binaries, and packs everything into the bundle with `scripts/build-mcpb.sh`.
It still needs macOS for `lipo`.

The archive holds exactly these entries, in this order:

```text
manifest.json
icon.png
server/gitlab-mcp-server                      macOS universal binary
server/gitlab-mcp-server.exe                  Windows amd64
server/linux/launch.sh                        Linux launcher
server/linux/gitlab-mcp-server-linux-amd64
server/linux/gitlab-mcp-server-linux-arm64
```

After packing, the script reads the archive back and refuses it (deleting the
file) unless it carries exactly that list, the launcher and the three Unix
binaries are recorded as Unix files with mode 0755, and the packed manifest
agrees with the archive: every `${__dirname}` path its command, its args and
each platform override name is present, every override is keyed `darwin`,
`win32` or `linux` and listed in `compatibility.platforms`, that list names
all three platforms and gives every one but `darwin` an override, and no
override declares `env`. The platform rule runs both ways because the base
command is the macOS universal binary: a platform listed without an override
would be handed that binary, and a platform left out of the list is one
Claude Desktop marks the bundle incompatible on. The `env` rule exists because
Claude Desktop applies an override's `env` in place of the base `env` rather
than merging the two, so an override carrying one would start the server
without `GITLAB_TOKEN`. A check that fails, or a step that stops the script
before the checks finish, leaves no bundle behind.

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
`scripts/mcpb_manifest_test.py` pins the manifest values this layout depends
on, and `scripts/build_mcpb_test.py` runs the build script over stand-in
binaries and checks that it refuses, and removes, each bundle its rules exist
for. All three run in CI's supply-chain job.

A `.mcpb` is a plain zip with `manifest.json` at its root, so the script builds
it with `zip` and fixed entry timestamps: the same inputs produce the same
bytes, which matters because `server.json` records the bundle's SHA-256. It used
to shell out to `npx --yes @anthropic-ai/mcpb@<pin>`, which pinned the CLI but
resolved its dependency tree fresh from the registry on every release, inside
the job that holds this project's signing and publishing identities.

In CI, the release workflow builds the bundle from the GoReleaser artifacts
(including the `universal_binaries` darwin build) and uploads it as a release
asset. The manifest version is stamped from the git tag by
`scripts/update-server-json-sha.sh`, the same flow that versions `server.json`.

## Files

| Path                             | Purpose                                                                      |
| -------------------------------- | ---------------------------------------------------------------------------- |
| `mcpb/manifest.json`             | MCPB manifest (source of truth; version stamped per release)                 |
| `mcpb/icon.png`                  | 512×512 icon rendered from `site/public/favicon.svg` by `make brand-rasters` |
| `mcpb/linux/launch.sh`           | Linux entry point: picks the amd64 or arm64 binary by `uname -m`             |
| `scripts/build-mcpb.sh`          | Bundle assembly, deterministic `zip` packing, and the checks on the archive  |
| `scripts/mcpb_launch_sh_test.py` | Tests of the Linux launcher, run in CI                                       |
| `scripts/mcpb_manifest_test.py`  | Tests of the manifest values the bundle layout depends on, run in CI         |
| `scripts/build_mcpb_test.py`     | Tests of the build script's checks and its removal of a refused bundle       |
| `PRIVACY.md`                     | Privacy policy referenced by the manifest                                    |

## Privacy and directory submission

The manifest's `privacy_policies` points to [PRIVACY.md](../../PRIVACY.md) and
the [GitLab Privacy Statement](https://about.gitlab.com/privacy/). Directory
submissions for desktop extensions go through Anthropic's
[submission form](https://claude.com/docs/connectors/building/submission),
which requires the documentation URL, the privacy policy, the icon, and test
credentials.
