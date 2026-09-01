# Installation

Every way to install gitlab-mcp-server, what each one needs, and how to verify, upgrade and remove it.

> **Diátaxis type**: How-to · **Audience**: 👤 Users & 🔧 operators
> 📖 **User documentation**: the same material, one page per channel, is on the documentation site under [Installation](https://jmrp.io/docs/gitlab-mcp-server/install/overview/).

---

## Choose a channel

Every channel ends with the same program: one `gitlab-mcp-server` binary that your MCP client starts over stdio. The channels differ in what they need on the machine and in who is responsible for upgrading it.

| Channel                                         | What you get                                      | Needs              | Upgrade with                                        | Platforms                                       |
| ----------------------------------------------- | ------------------------------------------------- | ------------------ | --------------------------------------------------- | ----------------------------------------------- |
| [Native binary](#native-binary-github-releases) | The release binary, placed by a script or by hand | Nothing            | Re-run the script, or download again                | Linux, macOS, Windows; amd64 and arm64          |
| [Homebrew](#homebrew)                           | The same binary from a tap formula                | Homebrew           | `brew upgrade`                                      | macOS and Linux; amd64 and arm64                |
| [Windows (winget)](#windows-winget)             | The same binary as a portable package             | winget             | `winget upgrade`                                    | Windows; x64 and arm64                          |
| [Docker](#docker)                               | A container image, stdio or HTTP                  | Docker             | Pull a newer tag                                    | linux/amd64 and linux/arm64                     |
| [npm and npx](#npm-and-npx)                     | A launcher package carrying the binary            | Node.js 18+        | `npm update -g`                                     | Linux (glibc), macOS, Windows; x64 and arm64    |
| [PyPI, uvx and pipx](#pypi-uvx-and-pipx)        | A platform wheel carrying the binary              | Python 3.9+ or uv  | `pipx upgrade`, `uv tool upgrade`, `pip install -U` | Linux (glibc), macOS, Windows; x86_64 and arm64 |
| [Claude Desktop (.mcpb)](#claude-desktop-mcpb)  | A desktop extension with the binary inside        | Claude Desktop     | Install the newer extension version                 | macOS (universal) and Windows (x64 binary)      |
| [Agent Plugins](#agent-plugins)                 | A plugin manifest that runs the container image   | A host + Docker    | Pull a newer tag                                    | Wherever the host and Docker run                |
| [Hosted endpoint](#hosted-endpoint)             | Nothing installed; an HTTP endpoint on GitLab.com | A GitLab.com token | Nothing to upgrade                                  | Any HTTP-capable client                         |

Not sure? Docker or the one-line installer for a first try, Homebrew or winget if you already use them, npm or PyPI if your client launches servers with `npx` or `uvx`, the `.mcpb` for Claude Desktop, and the hosted endpoint to look before installing anything.

---

## What every channel shares

- **The binary is the same everywhere.** Homebrew and winget point at the GitHub Release assets, pinned by the SHA-256 values in that release's `checksums.txt`; the npm and PyPI packages and the `.mcpb` are assembled in the release job from the same build outputs. `gitlab-mcp-server --version` prints `gitlab-mcp-server <version> (commit: <commit>)` on every channel, which is the quickest way to see what a client is actually running.
- **Two values configure it.** `GITLAB_TOKEN` is the only required setting: a Personal Access Token (`glpat-...`) with the `api` scope. A `read_api` token also works and is served a read-only tool surface; pair it with `GITLAB_READ_ONLY=true` for an explicit read-only setup. `GITLAB_URL` defaults to `https://gitlab.com`, so set it only for a self-managed instance. Everything else is optional and listed in the [configuration reference](../reference/configuration.md).
- **The server never updates itself.** There is no update check and no in-place binary replacement on any channel. Upgrades come from whichever channel installed it: `brew upgrade`, `winget upgrade`, `npm update -g`, a newer image tag, a newer Claude Desktop extension, or a fresh download. An earlier self-update subsystem was removed; package managers own the files they install.
- **There is no setup wizard.** Started in a terminal, or double-clicked on Windows, with no `GITLAB_TOKEN` set, the binary prints what it is and the two values it needs to stderr, then waits for Enter so a console window does not vanish before you read it. An MCP client never sees that screen, because a client connects pipes rather than a terminal. Configuration lives in the client's own JSON; see [Configure your client](#configure-your-client).
- **The current release is v2.7.5** (2026-08-27). Every registry below carries that version, and the Docker `latest` tag resolves to it. Facts in this guide that depend on the live registries were checked on 2026-09-01.

---

## Native binary (GitHub Releases)

Release binaries are built by GoReleaser with `CGO_ENABLED=0`, `-trimpath` and `-buildmode=pie`, so each one is a single static file with no runtime dependency.

### Assets

| Asset                                 | Platform                                                    |
| ------------------------------------- | ----------------------------------------------------------- |
| `gitlab-mcp-server-linux-amd64`       | Linux x86_64                                                |
| `gitlab-mcp-server-linux-arm64`       | Linux arm64                                                 |
| `gitlab-mcp-server-darwin-amd64`      | macOS Intel                                                 |
| `gitlab-mcp-server-darwin-arm64`      | macOS Apple Silicon                                         |
| `gitlab-mcp-server-darwin-all`        | macOS universal (arm64 + amd64 in one file)                 |
| `gitlab-mcp-server-windows-amd64.exe` | Windows x64                                                 |
| `gitlab-mcp-server-windows-arm64.exe` | Windows arm64                                               |
| `checksums.txt`                       | SHA-256 of each of the seven binaries                       |
| `checksums.txt.sigstore.json`         | Keyless Cosign signature of `checksums.txt`                 |
| `gitlab-mcp-server.mcpb`              | Claude Desktop extension, see [below](#claude-desktop-mcpb) |

`gitlab-mcp-server-darwin-all` exists because the `.mcpb` manifest overrides the command per operating system, not per architecture, so its macOS entry point has to run on both; it is just as usable on its own.

Download URLs have two forms: `https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/<asset>` always resolves to the newest release, and `https://github.com/jmrplens/gitlab-mcp-server/releases/download/v<version>/<asset>` pins one.

### Install script (Linux and macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.sh | sh
```

The script detects Linux or macOS on amd64 or arm64 (`x86_64` and `aarch64` are mapped), refuses anything else and tells Windows users to use the PowerShell installer. It needs `curl` or `wget`, downloads the asset for the platform, and **verifies its SHA-256 against the release's `checksums.txt`** with `sha256sum` or `shasum`; a mismatch, a missing entry or an unreachable `checksums.txt` aborts the install (`ALLOW_UNVERIFIED=1` bypasses that, at your own risk). It removes any existing binary first so re-installing over a running server does not fail with "Text file busy", installs the file with mode `0755` as `$INSTALL_DIR/gitlab-mcp-server`, and warns when the directory is not on your `PATH`.

| Variable      | Default                      | Meaning                               |
| ------------- | ---------------------------- | ------------------------------------- |
| `INSTALL_DIR` | `$HOME/.local/bin`           | Where the binary is written           |
| `VERSION`     | `latest`                     | A release tag such as `v2.7.5` to pin |
| `REPO`        | `jmrplens/gitlab-mcp-server` | The repository to download from       |

Its printed next step is the Claude Code registration, `claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server` (add `--env GITLAB_URL=https://gitlab.example.com` for a self-managed instance), or the hand configuration in [Configure your client](#configure-your-client).

### PowerShell installer (Windows)

```powershell
irm https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.ps1 | iex
```

The script detects AMD64 or ARM64 (reading `PROCESSOR_ARCHITEW6432` first, so a 32-bit PowerShell on a 64-bit machine still picks the right build) and refuses 32-bit x86, for which no build is published. It downloads `gitlab-mcp-server-windows-<arch>.exe`, verifies it against `checksums.txt` with `Get-FileHash` (same `ALLOW_UNVERIFIED=1` bypass), installs it as `gitlab-mcp-server.exe` under `%LOCALAPPDATA%\Programs\gitlab-mcp-server` by default, and appends that directory to your user-scope `PATH`. Open a new PowerShell or Windows Terminal window before relying on the new `PATH`. `-Version`, `-InstallDir` and `-Repo` (or the `VERSION`, `INSTALL_DIR` and `REPO` environment variables) override the defaults.

### Manual download

Download the asset for your platform, then on Linux or macOS:

```bash
chmod +x gitlab-mcp-server-*
sudo mv gitlab-mcp-server-linux-amd64 /usr/local/bin/gitlab-mcp-server   # or ~/.local/bin/gitlab-mcp-server
```

On Windows, put the `.exe` in a directory on your `PATH`, or reference its full path in the client configuration.

### Verify a download

Every release ships `checksums.txt` and `checksums.txt.sigstore.json`, and both can be checked for every release including v2.7.5. Install [Cosign](https://docs.sigstore.dev/cosign/installation/), download the binary and the two files into one directory, then:

```bash
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp "^https://github.com/jmrplens/gitlab-mcp-server/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

`cosign` prints `Verified OK` and `sha256sum` prints `<asset>: OK`. On macOS, `shasum` has no `--ignore-missing`, so filter the line first: `grep "$(ls gitlab-mcp-server-*)" checksums.txt | shasum -a 256 -c`. To pin the exact release rather than the repository, replace the regexp with `--certificate-identity "https://github.com/jmrplens/gitlab-mcp-server/.github/workflows/release.yml@refs/tags/v<version>"`, which is the form each release's notes print. If verification fails, do not run the binary.

Two further artifacts are produced by the release workflow for **releases after v2.7.5**: one SPDX SBOM per binary (`<asset>.sbom.json`) and a SLSA build provenance attestation stored by GitHub, which `gh attestation verify <file> -R jmrplens/gitlab-mcp-server` checks. Both steps landed after the v2.7.5 tag, so that release carries neither; on v2.7.5 the checksum and signature above are the verification. Steps and background are in [release integrity](https://jmrp.io/docs/gitlab-mcp-server/operations/security/#verifying-release-integrity).

Releases are created as drafts and published only after every asset is attached, so a release you can see is never a partial one.

### Upgrade and uninstall

Re-run the install script, or download the newer asset over the old file: the scripts replace the existing binary in place. To remove it, delete the file: `$HOME/.local/bin/gitlab-mcp-server` for the shell script, `%LOCALAPPDATA%\Programs\gitlab-mcp-server\gitlab-mcp-server.exe` for the PowerShell one (and drop that directory from your user `PATH` if you want it gone entirely), or wherever you placed a manual download. Then remove the server entry from your client's configuration.

---

## Homebrew

```bash
brew install jmrplens/tap/gitlab-mcp-server
```

The tap is [`jmrplens/homebrew-tap`](https://github.com/jmrplens/homebrew-tap) and the formula is `Formula/gitlab-mcp-server.rb`, a binary formula: it downloads the release asset for your OS and architecture (`on_macos`/`on_linux` × `on_arm`/`on_intel`) pinned by the SHA-256 from that release's `checksums.txt`, and installs it as `bin/gitlab-mcp-server`. The formula is regenerated from `checksums.txt` by `scripts/update-homebrew-tap.sh` on every release, its `test` block runs `gitlab-mcp-server --version`, and its `livecheck` follows the latest GitHub release. The live formula is at 2.7.5 with hashes matching that release.

Configure your client with the command `$(brew --prefix)/bin/gitlab-mcp-server` (or just `gitlab-mcp-server` when Homebrew's bin directory is on your `PATH`), `GITLAB_TOKEN`, and `GITLAB_URL` for a self-managed instance.

The formula published for 2.7.5 still mentions `AUTO_UPDATE=false` and `--setup` in its caveats. Neither exists any more; the tap's generator no longer emits those lines, so the next release's formula will not carry them.

**Upgrade and uninstall.** `brew upgrade gitlab-mcp-server` (or a plain `brew upgrade`) moves to the newest formula; `brew uninstall gitlab-mcp-server` removes it, and `brew untap jmrplens/tap` drops the tap as well. These are ordinary Homebrew commands, not project-specific ones.

---

## Windows (winget)

### Install

```powershell
winget install --id jmrplens.gitlab-mcp-server -e
```

The package identifier is `jmrplens.gitlab-mcp-server` (moniker `gitlab-mcp-server`), published in `microsoft/winget-pkgs` under the name "GitLab MCP Server". It is a **portable** package: the installer manifest points straight at `gitlab-mcp-server-windows-amd64.exe` and `gitlab-mcp-server-windows-arm64.exe` from the GitHub Release, with SHA-256 values equal to the entries in that release's `checksums.txt`, so what winget installs is byte-for-byte the release asset.

### Verify

```powershell
winget show --id jmrplens.gitlab-mcp-server -e     # package details and the installer winget selected
winget list --id jmrplens.gitlab-mcp-server -e     # installed version, plus "Available" when a newer one exists
gitlab-mcp-server --version                        # what actually runs
```

Open a new PowerShell or Windows Terminal window after the install: `PATH` changes only reach new sessions.

### Where the binary and its alias land

winget installs a user-scope portable package into a per-package folder under `%LOCALAPPDATA%\Microsoft\WinGet\Packages\` (`%PROGRAMFILES%\WinGet\Packages\` for machine scope). It then creates a symlink in `%LOCALAPPDATA%\Microsoft\WinGet\Links\` (`%PROGRAMFILES%\WinGet\Links\` for machine scope), adds that `Links` directory to `PATH` if it is not there already, and names the symlink after the manifest's command, which is `gitlab-mcp-server`. That is the same command name every other channel installs, so `claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server` works unchanged, and so does any client JSON that names the command without a path. `winget install --rename <name>` (`-r`) chooses a different alias.

### Upgrade

```powershell
winget upgrade --id jmrplens.gitlab-mcp-server -e
```

`winget update` is an alias. winget downloads the new executable to a temporary location, replaces the existing file, and refreshes the symlink and the Apps & Features entry. If the server is running at that moment winget says so; `--force` terminates it.

### Uninstall

```powershell
winget uninstall --id jmrplens.gitlab-mcp-server -e
```

`remove` and `rm` are aliases. Uninstalling removes the executable, its symlink and the Apps & Features entry, and deletes the package directory once it is empty. Two portable-only options: `--purge` deletes everything in the package directory, `--preserve` keeps files the package created.

### Version lag

A winget release is not published by this repository directly. The release workflow opens a version pull request against `microsoft/winget-pkgs` through the `jmrplens/winget-pkgs` fork, and the new version becomes installable only after Microsoft's validation merges it. The newest version can therefore lag the GitHub Release; check `winget show` before assuming it is there. As of 2026-09-01 the newest published version, 2.7.5, matches the repository. The published locale manifest still describes an interactive `--setup` wizard and points at an old documentation host; both are stale text carried forward by the publishing tool and not a feature of the binary.

---

## Docker

The image is published to two registries with identical content: `ghcr.io/jmrplens/gitlab-mcp-server` and the mirror `docker.io/jmrplens/gitlab-mcp-server` (shown as `jmrplens/gitlab-mcp-server` on Docker Hub). Each release pushes the `<version>` tag and moves `latest`, for `linux/amd64` and `linux/arm64`, with provenance and SBOM attestations and a Cosign signature on the digest. The image runs as the non-root user `appuser` (uid 10001) on Alpine, has the binary at `/usr/local/bin/gitlab-mcp-server`, exposes port 8080, and carries a `HEALTHCHECK` against `http://localhost:8080/health`.

Its `ENTRYPOINT` is `gitlab-mcp-server` and its `CMD` is `--http --http-addr 0.0.0.0:8080`, which is why the two ways to run it look different.

### stdio: a client starts the container

Any argument after the image name replaces the `CMD` wholesale, so pass `--http=false` to get the stdio transport a desktop client expects. Without it the container starts an HTTP listener and the client waits forever for the `initialize` response. Keep `-i` so stdin stays open, and do not publish port 8080 in this mode.

```bash
docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest --http=false
```

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "-e", "GITLAB_TOKEN", "ghcr.io/jmrplens/gitlab-mcp-server:latest", "--http=false"],
      "env": { "GITLAB_TOKEN": "glpat-xxxxxxxxxxxx" }
    }
  }
}
```

`-e GITLAB_TOKEN` with no value forwards the variable from the client's `env` block into the container. For a self-managed instance add `-e GITLAB_URL` to `args` and `GITLAB_URL` to `env`. Claude Code takes the same command line: `claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx --transport stdio -- docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest --http=false`. The image is pulled on first use. The one-click buttons for VS Code, Cursor, LM Studio and Kiro register exactly this configuration, which is why they need Docker installed.

### HTTP: a long-running container on a port

The container's default mode serves HTTP on `:8080`, where each client sends its own token per request. The simplest form is:

```bash
docker run --rm -p 8080:8080 ghcr.io/jmrplens/gitlab-mcp-server:latest
```

That starts the server **without** `--gitlab-url`, so it is in multi-instance mode: a client chooses the instance with a `GITLAB-URL` header and gets `https://gitlab.com` when it sends none. To fix the instance, and for the hardening flags a shared deployment should have, pass the flags explicitly:

```bash
docker run -d --name gitlab-mcp \
  --read-only --tmpfs /tmp:rw,size=64m --cap-drop=ALL --security-opt=no-new-privileges:true \
  -p 8080:8080 \
  ghcr.io/jmrplens/gitlab-mcp-server:latest \
  --http --http-addr=0.0.0.0:8080 --gitlab-url=https://gitlab.com
```

A client then uses `type: "http"` with a URL such as `http://localhost:8080/mcp` and its own token. The root `docker-compose.yml` runs the same shape, with `image: ${DOCKER_REGISTRY:-ghcr.io/jmrplens/gitlab-mcp-server}:${VERSION:-latest}` and the surface, tier, TLS, read-only and safe-mode flags as `command` arguments. Everything about that mode, including OAuth, TLS termination and the server pool, is in [HTTP Server Mode](http-server-mode.md).

### Environment variables

The container reads the same variables as the binary. The Agent Plugins config (root `mcp.json`) and `server.json` forward this set with `-e`: `GITLAB_URL`, `GITLAB_TOKEN`, `GITLAB_SKIP_TLS_VERIFY`, `TOOL_SURFACE`, `CAPABILITY_SURFACE`, `META_PARAM_SCHEMA`, `GITLAB_TIER`, `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE`, `EMBEDDED_RESOURCES`, `EXCLUDE_TOOLS`, `GITLAB_IGNORE_SCOPES`, `UPLOAD_MAX_FILE_SIZE`, `GITLAB_MCP_ALLOWED_IMPORT_DIRS`, `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`, `CLIENT_COMPAT` and `LOG_LEVEL`. In stdio mode each one you use needs its own `-e NAME` in `args` and its value in the client's `env` block. In HTTP mode the flags in the command line take their place; see the [configuration reference](../reference/configuration.md).

For a self-signed certificate, mount the CA into the container and set `SSL_CERT_FILE=/path/to/ca-bundle.crt` rather than reaching for `GITLAB_SKIP_TLS_VERIFY=true`, which disables verification outright and which OAuth mode refuses for any non-loopback instance.

### Tags, registries and updating

Pin `ghcr.io/jmrplens/gitlab-mcp-server:<version>` in anything that has to be reproducible; `latest` tracks the newest release. As of 2026-09-01, `2.7.5` and `latest` resolve to the same digest on both registries, `sha256:8eec1825b266712cd544bf1b2144e55c1eb711b4540def40e963a664c4e97168`, which is also the digest `server.json` pins for the MCP Registry.

Updating is a pull, then a restart: `docker pull ghcr.io/jmrplens/gitlab-mcp-server:latest` (a standard Docker command; the repository's docs rely on `docker run` pulling on first use) and, for a stdio setup, nothing more, since `docker run --rm` starts a fresh container from the updated image on the next client launch. A long-running HTTP container keeps serving the image it started from until you recreate it. To remove the image, `docker rmi ghcr.io/jmrplens/gitlab-mcp-server:latest` after stopping any container using it.

---

## npm and npx

```bash
npx -y @jmrp.io/gitlab-mcp-server           # zero install; clients launch it directly
npm install -g @jmrp.io/gitlab-mcp-server   # global install
pnpm add -g @jmrp.io/gitlab-mcp-server      # global install with pnpm
```

[`@jmrp.io/gitlab-mcp-server`](https://www.npmjs.com/package/@jmrp.io/gitlab-mcp-server) is a launcher package (the esbuild and biome model): its `bin` is `gitlab-mcp-server`, mapped to a small `cli.js`, and six optional dependencies, `@jmrp.io/gitlab-mcp-server-linux-x64`, `-linux-arm64`, `-darwin-x64`, `-darwin-arm64`, `-win32-x64` and `-win32-arm64`, each carry the release binary for one platform, gated by `os` and `cpu` (the Linux ones also by `libc: ["glibc"]`) and pinned to the launcher's exact version. npm installs only the one that matches, so nothing compiles and nothing runs at install time: `--ignore-scripts` works, and so does a proxy. Node.js 18 or newer is required. `cli.js` resolves the platform package, spawns the binary with inherited stdio, forwards every argument verbatim and mirrors the exit code, so `gitlab-mcp-server --version` after a global install prints the binary's own version.

A client launches it with `npx`:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "npx",
      "args": ["-y", "@jmrp.io/gitlab-mcp-server"],
      "env": { "GITLAB_URL": "https://gitlab.com", "GITLAB_TOKEN": "glpat-…" }
    }
  }
}
```

After a global install the command is plain `gitlab-mcp-server`. On the registry, 2.7.5 is both `latest` and the only published version.

If the launcher reports that the platform package is missing, the usual causes are `npm install --no-optional`, a lockfile generated on another operating system, or a musl-based distribution such as Alpine: the Linux packages are skipped there on purpose, because the PIE binaries need the glibc loader. Use the [Docker image](#docker), which is musl-based, or build from source. On an unsupported platform the launcher exits with a message pointing at the release binaries.

**Upgrade and uninstall.** `npm update -g @jmrp.io/gitlab-mcp-server` (documented) or `pnpm update -g @jmrp.io/gitlab-mcp-server`; `npx -y` resolves the newest version on each cold run, subject to the npx cache. Remove a global install with `npm uninstall -g @jmrp.io/gitlab-mcp-server` or `pnpm remove -g @jmrp.io/gitlab-mcp-server`. These are ordinary package-manager commands.

---

## PyPI, uvx and pipx

```bash
uvx jmrplens-gitlab-mcp-server              # zero install; clients launch it directly
pipx install jmrplens-gitlab-mcp-server     # isolated global install
pip install jmrplens-gitlab-mcp-server      # into the active environment
```

[`jmrplens-gitlab-mcp-server`](https://pypi.org/project/jmrplens-gitlab-mcp-server/) ships six platform wheels (the uv and ruff model): `manylinux_2_17` x86_64 and aarch64, `macosx_11_0` x86_64 and arm64, `win_amd64` and `win_arm64`, for Python 3.9 or newer. Each wheel places the native binary in its `.data/scripts` directory, so the installer puts it on the environment's scripts path as the `gitlab-mcp-server` command itself, executable bit set, and no Python runs when the server does. The wheel also declares a console script named `jmrplens-gitlab-mcp-server`, a wrapper that execs the binary; that second command is what lets `uvx` resolve the distribution by name, and `python -m gitlab_mcp_server` works too. On PyPI, 2.7.5 is the only released version.

The distribution name carries the author prefix because the unprefixed `gitlab-mcp-server` project on PyPI is an empty registration held by an unrelated account, under a PEP 541 reclamation request. If it is reclaimed, the rename is a constant in `scripts/build_pypi.py` and `scripts/validate_pypi.py`, the `server.json` identifier and the docs.

A client launches it with `uvx`:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "uvx",
      "args": ["jmrplens-gitlab-mcp-server"],
      "env": { "GITLAB_URL": "https://gitlab.com", "GITLAB_TOKEN": "glpat-…" }
    }
  }
}
```

The Linux wheels need glibc; on musl systems such as Alpine use the [Docker image](#docker) instead.

**Upgrade and uninstall.** `pipx upgrade jmrplens-gitlab-mcp-server`, `pip install -U jmrplens-gitlab-mcp-server`, or `uv tool upgrade jmrplens-gitlab-mcp-server` if you installed with `uv tool install`; `uvx` resolves the newest version on each cold run, subject to uv's cache. Remove with `pipx uninstall jmrplens-gitlab-mcp-server`, `pip uninstall jmrplens-gitlab-mcp-server` or `uv tool uninstall jmrplens-gitlab-mcp-server`. These are ordinary package-manager commands.

---

## Claude Desktop (.mcpb)

Download [`gitlab-mcp-server.mcpb`](https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server.mcpb), open it with Claude Desktop (double-click it, or drag it onto the Settings window), review the install dialog and fill in the settings. The bundle contains a macOS universal binary and a Windows executable, so it needs no Docker, Node.js or Python; its manifest (v0.4, `server.type: binary`) declares `darwin` and `win32` and Claude Desktop 0.10.0 or newer. There is no Linux entry, and the Windows binary is amd64 only, so a Windows arm64 machine receives the x64 executable.

The settings map to environment variables: GitLab URL (`GITLAB_URL`, default `https://gitlab.com`) and Personal Access Token (`GITLAB_TOKEN`, `api` scope) are required; Tool surface (`TOOL_SURFACE`, default `dynamic`), GitLab tier (`GITLAB_TIER`, empty for auto-detect), Read-only mode (`GITLAB_READ_ONLY`), Safe mode (`GITLAB_SAFE_MODE`), Skip TLS verification (`GITLAB_SKIP_TLS_VERIFY`) and Log level (`LOG_LEVEL`, written to Claude Desktop's MCP log files) are optional. Claude Desktop stores the token in the operating system keychain, not in a plain-text file.

To check the install, ask Claude "What GitLab user am I authenticated as?"; on the default dynamic surface it calls `gitlab_find_action` and then `gitlab_execute_action` with the `user.current` action and returns your username.

**Verify the bundle.** The `.mcpb` is built outside GoReleaser, so it is not in `checksums.txt`. For v2.7.5, compare its SHA-256 with the `fileSha256` recorded in `server.json`, `c224f7dca31ca5b3e53d450413ea46155798f82f9cada8c4e34e01642f60e4fe`. Releases after v2.7.5 attest the bundle separately, which `gh attestation verify gitlab-mcp-server.mcpb -R jmrplens/gitlab-mcp-server` checks. Building and packing are covered in the [Claude Desktop Extension guide](claude-desktop-extension.md).

**Upgrade and uninstall.** Updates arrive as new extension versions published with each release; the installed one runs until Claude Desktop installs a newer extension. Uninstall it from Claude Desktop's extension settings; that step belongs to Claude Desktop and is not documented in this repository.

---

## Agent Plugins

```bash
/plugin install jmrplens/gitlab-mcp-server
```

The repository ships an [Agent Plugins](https://agent-plugins.org/) 1.0 manifest at the root (`plugin.json` and `mcp.json`) and keeps the legacy Open Plugins manifest (`.plugin/plugin.json`) for older hosts, so a conformant host (Cursor, Claude Code, VS Code, OpenCode, when your version supports it) installs the server in one step. `make check-openplugin` validates both.

The bundled `mcp.json` has a single stdio entry that runs `docker run -i --rm -e <variables> ghcr.io/jmrplens/gitlab-mcp-server:latest --http=false`, forwarding the variable set listed under [Docker](#environment-variables), so Docker must be installed and running. The spec starts every entry automatically and has no runtime variants, which is why there is one entry and it is Docker.

**The token cannot travel inside `mcp.json`.** Agent Plugins expands only `${PLUGIN_ROOT}` and `${PLUGIN_DATA}`; `"${GITLAB_TOKEN}"` reaches the server as that literal string. The spec also lets a host inherit, omit or sanitize ambient variables (§9.1), so how `GITLAB_TOKEN` arrives is decided by the host: set it the way the host documents, or put it in the `env` block of the installed plugin's local `mcp.json` (commonly under `.agents/plugins/gitlab-mcp-server/`), a file that is yours and should stay out of version control.

To use a native binary instead of Docker, edit that same local `mcp.json` and replace `command` and `args`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "gitlab": {
      "type": "stdio",
      "command": "/usr/local/bin/gitlab-mcp-server"
    }
  }
}
```

For a detached HTTP deployment do not use a stdio entry at all: run the image in [HTTP mode](#http-a-long-running-container-on-a-port) and configure the client with `type: "http"` and a URL such as `http://localhost:8080/mcp`.

**Upgrade and uninstall.** The plugin runs `latest`, so `docker pull ghcr.io/jmrplens/gitlab-mcp-server:latest` picks up a new release; removing the plugin is done in the host's plugin UI, and the image with `docker rmi`.

---

## Hosted endpoint

Nothing to install: a public instance runs at `https://mcp.jmrp.io/gitlab`, fixed to GitLab.com.

```json
{
  "mcpServers": {
    "gitlab": {
      "type": "http",
      "url": "https://mcp.jmrp.io/gitlab",
      "headers": { "Authorization": "Bearer glpat-xxxxxxxxxxxx" }
    }
  }
}
```

It runs `--auth-mode=oauth`. A client that speaks the OAuth flow needs no header: the `401` carries an RFC 6750 challenge pointing at `https://mcp.jmrp.io/.well-known/oauth-protected-resource/gitlab`, from which it discovers `https://gitlab.com` as the authorization server and authorizes in the browser. Anything that cannot open a browser sends a GitLab.com personal access token as `Authorization: Bearer glpat-...`, verified exactly like an OAuth token, per request, never stored server-side. `PRIVATE-TOKEN` is the legacy-mode header and is not accepted; `GITLAB-URL` is ignored because the instance is fixed, so a self-managed GitLab cannot be reached through it. An `api` token gets the full surface; a `read_api` token is admitted and served a read-only one.

The transport is stateless streamable HTTP on the default dynamic surface (`gitlab_find_action` and `gitlab_execute_action`): `POST` is the transport, an authenticated `GET` or `DELETE` answers `405`, and with no credential any method answers `401` with `WWW-Authenticate: Bearer … resource_metadata=…`, so a bare `curl` getting `401` is the endpoint working. `GET https://mcp.jmrp.io/gitlab/health` needs no credential and answers `200` with `{"status":"ok",…}`.

Three companion pages: the [server card](https://mcp.jmrp.io/servers/gitlab/) lists the whole catalog unauthenticated and carries copy-paste config for Claude Code, Cursor and VS Code, including the OAuth client ID those clients need (without it they fall back to dynamic registration and receive a scope the server cannot use); the [browser inspector](https://mcp.jmrp.io/inspector/?server=gitlab) signs in with OAuth and makes read-only calls with nothing installed; and [mcp.jmrp.io](https://mcp.jmrp.io/) is the directory, with `https://mcp.jmrp.io/servers.json` as its machine-readable form. The MCP server card is at `/gitlab/server-card`.

It is a personal service run by one person, as-is: no SLA, no support channel, no promise it is unchanged next week. Your token and every request pass through someone else's machine, which is the reason to run the server locally once you have decided to keep using it, and the only sensible choice for a private self-managed instance. It adds no quota of its own (every call spends GitLab.com's limits under your token), it tracks the latest release rather than a pinned version, and it is deployed out of band from the [mcp.jmrp.io](https://github.com/jmrplens/mcp.jmrp.io) host; this repository deploys nothing. The full property table is in [HTTP Server Mode](http-server-mode.md#public-hosted-endpoint), and self-hosting the same setup is `--auth-mode=oauth --gitlab-url=https://gitlab.com --public-url=https://mcp.example.com/mcp`, with `--public-url` exactly the URL clients are configured with; see [OAuth App Setup](oauth-app-setup.md).

There is nothing to upgrade or uninstall: remove the entry from your client's configuration.

---

## Configure your client

Every stdio channel is configured the same way: the client starts `gitlab-mcp-server` (or `npx`, `uvx`, or `docker`) and passes `GITLAB_TOKEN` in its environment. The generic shape, used by Claude Desktop, Cursor, Windsurf, Kiro and Cline, is:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": { "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx" }
    }
  }
}
```

Add `"GITLAB_URL": "https://gitlab.example.com"` to `env` only for a self-managed instance. VS Code uses `.vscode/mcp.json` with a `servers` map and `"type": "stdio"`, and its `promptString` inputs keep the token out of plain text; Claude Code takes `claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server`. Per-client file locations and shapes are in [IDE Configuration](ide-configuration.md) and on the site's [Quick Start](https://jmrp.io/docs/gitlab-mcp-server/getting-started/#manual-configuration).

To keep the token out of client JSON entirely, put it in `~/.gitlab-mcp-server.env` (one `KEY=value` per line): an explicit environment variable beats a `.env` in the working directory, which beats that file. Then ask your assistant "List my GitLab projects." or "Who am I on GitLab?" to confirm the connection.

---

## See also

- [Getting Started](../getting-started.md) for the end-to-end tutorial
- [Configuration reference](../reference/configuration.md) for every variable and flag
- [HTTP Server Mode](http-server-mode.md) for shared deployments
- [Troubleshooting](troubleshooting.md) when a client does not connect
