// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command release_packages builds the Claude Desktop extension bundles a
// release ships (mcpb), derives the manifest each per-OS bundle carries
// (mcpb-manifest), and checks that every package server.json declares is
// published and is what it claims to be (server-json-packages). It is item 2
// of issue 1114: scripts/build-mcpb.sh, mcpb/platform.jq and
// scripts/validate-server-json-packages.sh, moved into Go, each held by its
// tests to a record of what the script did (testdata/build_mcpb.json,
// testdata/platform_jq.json and testdata/server_json_packages.json, taken at
// fe8773534, the revision of main this port started from).
//
// It also builds and validates the npm, PyPI and NuGet packages a release
// publishes (npm, pypi, nuget, and validate-npm, validate-pypi and
// validate-nuget), item 13 of issue 1114: scripts/build-npm.mjs,
// scripts/build_pypi.py, scripts/build_nuget.py and their three validators,
// moved into Go, the builders held by their tests to a record of what the
// scripts packed (testdata/build_packages.json, taken at 1c26b3b04).
//
// # mcpb
//
// One run builds four bundles from the release binaries GoReleaser produced,
// the committed manifest (mcpb/manifest.json), the 512x512 icon
// (mcpb/icon.png), the Linux launcher (mcpb/linux/launch.sh), the
// repository's LICENSE and <dist>/THIRD_PARTY_NOTICES, the license, notice and
// patent texts of what the binaries link, which GoReleaser writes beside them
// in a release and `make mcpb` beside its own (cmd/gen_third_party_notices):
//
//	gitlab-mcp-server-darwin.mcpb    macOS: the universal binary (arm64 + amd64)
//	gitlab-mcp-server-windows.mcpb   Windows: the amd64 executable
//	gitlab-mcp-server-linux.mcpb     Linux: the launcher and both Linux binaries
//	gitlab-mcp-server.mcpb           all three servers, the universal bundle
//
// The three per-OS bundles are what server.json declares, one registry entry
// each. A user downloads only the server their system runs, and each
// manifest lists only its own platform, so Claude Desktop refuses one opened
// on another system with a clear message instead of starting a binary that is
// not there. The universal bundle keeps the name it has always had and is
// still published, because links to it exist outside this repository;
// server.json does not declare it, since a registry entry has no platform
// field and a client could not tell it from the three that each serve one
// system. Every bundle uses the same layout, carrying the entries its system
// needs, so a path means the same thing in all four:
//
//	manifest.json                       (version stamped to VERSION)
//	icon.png
//	LICENSE
//	THIRD_PARTY_NOTICES
//	server/gitlab-mcp-server            (darwin universal: arm64 + amd64)
//	server/gitlab-mcp-server.exe        (windows amd64)
//	server/linux/launch.sh              (picks the binary below by uname -m)
//	server/linux/gitlab-mcp-server-linux-amd64
//	server/linux/gitlab-mcp-server-linux-arm64
//
// Each binary is found under the dist directory by its build directory
// (GoReleaser's <id>_<goos>_<goarch>[_<variant>], `make mcpb`'s
// local_<goos>_<goarch>, darwin_all for the universal binary), and exactly
// one match is accepted, since with two the bundle would carry whichever a
// walk of the directory happened to find first.
//
// A bundle is a plain zip with manifest.json first, so a reader that streams
// it finds the manifest before the multi-megabyte binaries, and the same
// inputs make the same bytes: every entry is recorded as a Unix file with its
// mode set rather than inherited (0755 for the servers, which Claude Desktop
// restores the execute bit of only from a recorded owner-execute bit, 0644
// for the rest), at 1980-01-01 00:00:00, deflated, with no extra field and no
// directory entry. server.json carries each declared bundle's SHA-256, and a
// hash that changes because the clock moved tells a verifier nothing. That is
// what zip -X -D wrote from the files the script touched to the zip epoch,
// entry for entry. The archive around the entries is archive/zip's rather
// than Info-ZIP's (other deflated bytes, a data descriptor after each entry,
// zip 2.0 rather than 3.0 as the version that made them, no text flag, and
// no entry left stored), so the bundles' hashes changed once, with this port.
//
// Each archive is read back after it is written, because the archive is what
// ships: exactly the expected entries in order, the servers recorded 0755,
// and a manifest that can start on every platform it lists (its version, its
// entry point, its icon and every path a command line names under
// ${__dirname} carried; every override a listed platform Claude Desktop
// reports, and every listed platform but the base command's overridden;
// exactly the platforms the archive carries a server for; no override that
// carries env, which would replace the base env and drop GITLAB_TOKEN, nor
// args beside non-empty base args, which it would replace). Each bundle's
// download and unpacked size is printed, and appended to the job summary
// GitHub Actions names; a bundle past one of Claude Desktop's own limits
// (100,000 entries, 512 MiB an entry, 2048 MiB unpacked, 50 times its own
// size) cannot be installed and fails the run, and a declared bundle past the
// sizes directories stop reading at (50 MiB to download, 256 MiB unpacked) is
// a warning.
//
// The previous run's bundles are removed before anything can refuse a run,
// and every bundle is removed when any check fails: a bundle that did not
// pass its own checks must not be left behind, and they go as a set, since a
// later step handed two of the three declared ones would publish a registry
// entry short of a system.
//
// # mcpb-manifest
//
// Prints the manifest of the bundle for one platform, derived from the
// committed manifest, which describes the universal bundle: only that
// platform listed, its override promoted to the base command and every
// override dropped, and as the entry point the first path that command line
// names inside the bundle. Everything else, the name above all, stays as it
// is, so the bundles are one extension and installing one replaces another
// in place. It is what mcpb/platform.jq derived, byte for byte: the members
// in their order, two spaces of indent, every string escaped as jq escapes
// it and every number as written. `make check-mcpb` validates each derived
// manifest with the mcpb CLI.
//
// # server-json-packages
//
// Downloads or looks up every package server.json declares, the mcpb
// bundles, the OCI images on ghcr.io and Docker Hub, and the npm, PyPI and
// NuGet versions, and holds each to what the MCP Registry checks only at
// publish time: a bundle's SHA-256, that it is a zip whose manifest names
// only paths it carries, and that the declared bundles serve darwin, win32
// and linux exactly once between them; an image tag that resolves to the
// digest it pins, every runnable platform's ownership label, and an entry
// that reaches stdio; each registry's ownership token or mcpName; and the
// fields the registry rejects for each type. It reports whether the GitHub
// release of the declared npm version is immutable, as a warning rather
// than a failure. Each request is bounded and retried as curl
// --retry 2 --retry-connrefused retried it, and a failed one is described on
// stderr. A bundle or a NuGet package is written to a temporary file as it
// arrives, as the script's curl -o wrote it, its SHA-256 taken on the way,
// and read as a zip from that file, so no artifact is held in memory.
//
// Three things differ from the script on purpose. Every field is held
// against the entry that carries it, where the script held each entry to the
// fields of every entry sharing its identifier. A registry that answers a
// token, a manifest or a tag lookup with a failure fails that package, where
// the script ended the whole run under set -e with no line naming it. And
// the warning about a mutable release carries no em dash.
//
// # npm, pypi and nuget
//
// Each builds one registry's packages from a directory of release assets
// under their published names (gitlab-mcp-server-linux-amd64, ...), the
// release's notices and its checksums.txt, which cosign signed at build time.
// Every asset is held to the digest checksums.txt names, through
// cmd/internal/releaseassets, before any package is written, so a refused run
// leaves nothing behind; a dist without checksums.txt is refused unless
// --allow-unverified is given, and never is in a release. What was verified
// is recorded beside the packages, in verified-binaries.json, for the
// validator to hold the packed bytes to. The six platforms meet their four
// vocabularies (GOOS and GOARCH, Node's platform and architecture, the wheel
// tag, the runtime identifier) in one table, releasePlatforms.
//
//	npm     npm/packages/<platform>/ for the six platform packages, the
//	        launcher's version and pins synced in npm/gitlab-mcp-server, and
//	        the license texts copied in beside it; --sync-only syncs alone
//	pypi    pypi/dist/, one wheel per platform with the binary in
//	        .data/scripts (the uv and ruff model) and PEP 639 license metadata
//	nuget   nuget/dist/, a pointer package and six runtime-identifier
//	        packages in the .NET 10 tool layout, with .mcp/server.json derived
//	        from server.json
//
// Every package is equal to the script's file for file and entry for entry.
// The wheels and the NuGet packages are zips whose entries are regular Unix
// files at the zip epoch, deflated, with their sizes in the local header and
// no data descriptor, which nuget.org's repository signature needs to be
// removable byte for byte; the deflated bytes are compress/flate's, so the
// archives differ from the ones Python's zipfile wrote. On purpose, too: no
// message carries an em dash, a missing flag is a usage error, an
// unreadable line in checksums.txt is refused rather than skipped, and the
// wheels name this command as their generator.
//
// # validate-npm, validate-pypi and validate-nuget
//
// Each checks what a builder wrote before the irreversible publish: a
// published npm version is permanent, a PyPI file name is burned forever, and
// a NuGet version can be unlisted but never replaced. Every problem is
// collected and named, rather than the first ending the run. The structural
// checks open what ships (the tarball npm pack makes, the wheels, the
// packages): the exact file set, the license texts (the repository's LICENSE
// byte for byte, the notices by the generator's header and the recorded
// digest), every binary's execute bit, size floor, format and machine as
// debug/elf, debug/macho or debug/pe read them, and its recorded digest, which
// is what tells a wrong but plausible file apart for the five platforms
// nothing runs. Each registry adds its own: package.json gates and pins; the
// wheel's RECORD, METADATA, license declaration, Project-URLs, regular-file
// modes and glibc floor, and twine check --strict when twine is installed;
// the OPC skeleton, nuspec, tool settings, ownership token, .mcp/server.json
// and the layout nuget.org's signature leaves intact. Without --no-install
// each then installs the package for the host the way a user does (npm
// install, a virtual environment's pip, dotnet tool install and dnx), drives
// an MCP initialize handshake that must answer with the version over pure
// JSON-RPC, and on Linux checks that a SIGTERM to npx alone ends the server
// in stdio and HTTP mode. The validators shell out to npm, pip and dotnet for
// that tier, which is what they check.
//
// # The standard library and this module only
//
// The mcpb subcommand runs in release.yml's release job, which holds the
// release's signing identity, and the package subcommands run in the npm,
// pypi and nuget jobs, which hold the registries' publishing identities, so
// rule 2 of cmd/audit_supply_chain allows the command the standard library
// and this module and nothing else. That is why it carries its own ordered
// JSON reader and printer rather than a library. The package jobs take the
// command as an artifact built once per run and checked against its
// SHA-256.
//
// # Usage
//
//	go run ./cmd/release_packages/ mcpb 3.4.0 [dist]
//	go run ./cmd/release_packages/ mcpb-manifest linux [mcpb/manifest.json]
//	go run ./cmd/release_packages/ server-json-packages [server.json]
//	go run ./cmd/release_packages/ npm --binaries dist --version 3.4.0
//	go run ./cmd/release_packages/ npm --sync-only --version 3.4.0
//	go run ./cmd/release_packages/ pypi --binaries dist --version 3.4.0
//	go run ./cmd/release_packages/ nuget --binaries dist --version 3.4.0
//	go run ./cmd/release_packages/ validate-npm --version 3.4.0
//	go run ./cmd/release_packages/ validate-pypi --version 3.4.0
//	go run ./cmd/release_packages/ validate-nuget --version 3.4.0
//
// Each runs from the repository root and exits 0 when everything passes, 1
// when something does not or could not be checked, and 2 for arguments it
// cannot use.
package main
