# Claude Desktop Extension (.mcpb)

The server ships as a one-click [Desktop Extension](https://www.anthropic.com/engineering/desktop-extensions)
(MCPB bundle) for Claude Desktop on macOS and Windows. The bundle contains a
macOS universal binary (arm64 + amd64) and a Windows amd64 executable — no
Docker, Node.js, or Python required.

## Install

1. Download `gitlab-mcp-server.mcpb` from the
   [latest release](https://github.com/jmrplens/gitlab-mcp-server/releases/latest).
2. Open the file with Claude Desktop (double-click, or drag it onto the
   window). Claude shows an install dialog with the extension details.
3. Fill in the settings:

| Setting                      | Required | Default              | Maps to                  |
| ---------------------------- | -------- | -------------------- | ------------------------ |
| GitLab URL                   | Yes      | `https://gitlab.com` | `GITLAB_URL`             |
| GitLab Personal Access Token | Yes      | —                    | `GITLAB_TOKEN`           |
| Tool surface                 | No       | `dynamic`            | `TOOL_SURFACE`           |
| GitLab tier                  | No       | auto-detect          | `GITLAB_TIER`            |
| Read-only mode               | No       | off                  | `GITLAB_READ_ONLY`       |
| Safe mode                    | No       | off                  | `GITLAB_SAFE_MODE`       |
| Skip TLS verification        | No       | off                  | `GITLAB_SKIP_TLS_VERIFY` |
| Log level                    | No       | `info`               | `LOG_LEVEL`              |

   The token is stored in the operating system keychain by Claude Desktop.

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
make mcpb          # builds dist/gitlab-mcp-server.mcpb (requires macOS lipo + zip)
make check-mcpb    # validates mcpb/manifest.json with the official CLI
```

`make mcpb` cross-compiles the darwin arm64/amd64 binaries, merges them with
`lipo`, cross-compiles the Windows amd64 binary, and packs everything into the
bundle with `scripts/build-mcpb.sh`.

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

| Path                    | Purpose                                                                      |
| ----------------------- | ---------------------------------------------------------------------------- |
| `mcpb/manifest.json`    | MCPB manifest (source of truth; version stamped per release)                 |
| `mcpb/icon.png`         | 512×512 icon rendered from `site/public/favicon.svg` by `make brand-rasters` |
| `scripts/build-mcpb.sh` | Bundle assembly + `mcpb pack`                                                |
| `PRIVACY.md`            | Privacy policy referenced by the manifest                                    |

## Privacy and directory submission

The manifest's `privacy_policies` points to [PRIVACY.md](../../PRIVACY.md) and
the [GitLab Privacy Statement](https://about.gitlab.com/privacy/). Directory
submissions for desktop extensions go through Anthropic's
[submission form](https://claude.com/docs/connectors/building/submission),
which requires the documentation URL, the privacy policy, the icon, and test
credentials.
