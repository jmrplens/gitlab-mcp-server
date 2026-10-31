// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command release_checks answers two questions release.yml asks of a
// release: before anything publishes it, whether the tag it is cut from may
// be released at all and is the newest release (order), and before anything
// packs its Linux binaries, whether they name the loader every channel that
// ships them promises (elf-interp). The image is pushed before that second
// question is asked, and its binary is not one of them: the Dockerfile links
// it with the musl loader its own base image carries.
//
// # order
//
// The preflight job runs it on the tag a release is cut from, with the tags
// the repository holds on stdin, as `git ls-remote --tags --refs <remote> 'v*'`
// prints them or one name per line: the last field of each line is read and
// a refs/tags/ prefix dropped. The tag being released does not have to be in
// the list, since a rehearsal names the version VERSION carries before that
// tag exists (issue 1323).
//
// It prints three lines for $GITHUB_OUTPUT:
//
//	stable=true          the tag is a plain vX.Y.Z; anything else is refused
//	newest=true|false    no stable tag in the list is higher than it
//	npm_dist_tag=...     latest for the newest release, release-X.Y otherwise
//
// Only those lines go to stdout, so the workflow appends it to $GITHUB_OUTPUT
// whole, and everything else goes to stderr. A prerelease or malformed tag
// exits 1 with an ::error:: line and prints nothing on stdout, so preflight
// fails and nothing downstream of it runs. Before this check existed, a
// prerelease tag pushed the image as :latest and published the GitHub
// release, and only then did the npm job refuse it, which left the release
// half published. A stable tag lower than one already released is a
// backport: it publishes under its own version everywhere, and leaves every
// pointer to the newest release (the image's :latest, the GitHub release
// marked Latest, npm's latest dist-tag, the Homebrew formula, main's
// manifests and the MCP Registry entry) where it is.
//
// Versions are compared number by number, so v3.10.0 is above v3.9.0, and
// prerelease tags already in the list are not compared at all. Two tags
// whose numbers are equal but spelled apart (v3.1.0 and v3.01.0) are ordered
// by their bytes, which is how `sort -V` ordered them when this check was
// scripts/release-order.sh.
//
// # elf-interp
//
// The release builds its binaries with -buildmode=pie, which makes the Linux
// ones dynamically linked, so the kernel starts each through the loader its
// PT_INTERP program header names. Go's linker picks that path by stat-ing the
// host that builds the release (cmd/link/internal/ld/elf.go), and a binary
// naming a loader the target system lacks dies at exec with "not found"
// before a line of Go runs: the image's linux/arm64 binary shipped that way
// once.
//
// Every Linux channel promises glibc: npm's libc field, PyPI's manylinux
// tags, NuGet's linux runtime identifiers and the .mcpb launcher all hand out
// the same two binaries. .goreleaser.yml therefore names the glibc loaders in
// per-target ldflags overrides, and the release job runs this check on the
// staged binaries once, right after GoReleaser, which covers every channel at
// the one point they share. Each binary is read with
// cmd/internal/releaseassets and held to the loader its machine is promised;
// one naming another loader, naming none (a static binary), built for a
// machine the release does not build or not readable as a 64-bit
// little-endian ELF file is named on stderr, and the run fails. Only when
// every binary passes is each one named on stdout.
//
// # The standard library and this module only
//
// elf-interp runs in the release job, which holds the release's signing
// identity, so rule 2 of cmd/audit_supply_chain allows the command the
// standard library and this module and nothing else. That is why the table
// of loaders is written here rather than read from .goreleaser.yml when the
// check runs: reading YAML would take a module from outside. A test holds the
// table to the overrides instead.
//
// # Usage
//
//	go run ./cmd/release_checks/ order v3.4.0 < tags.txt
//	go run ./cmd/release_checks/ elf-interp dist/gitlab-mcp-server-linux-amd64 dist/gitlab-mcp-server-linux-arm64
//
// Each check exits 0 when the release passes it, 1 when it does not or the
// check could not be made, and 2 for arguments it cannot use.
package main
